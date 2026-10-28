// Package logdoc matches a vLLM serve log against a catalog of known
// failures. Each rule has an id, a regular expression, a cause, a fix and a
// documentation link. The catalog is embedded YAML (catalog.yaml).
//
// The package returns data and never prints.
package logdoc

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severities a rule may carry.
const (
	SeverityError = "error" // the launch failed or cannot work
	SeverityWarn  = "warn"  // the server runs but loses speed or stability
)

const (
	// MaxLineBytes caps the bytes of one log line that are matched. The rest
	// of a longer line is skipped.
	MaxLineBytes = 1 << 20
	// maxShownLine caps the matched line kept in a Match.
	maxShownLine = 400
)

//go:embed catalog.yaml
var catalogYAML []byte

// Rule is one known failure.
type Rule struct {
	ID       string `yaml:"id" json:"id"`
	Severity string `yaml:"severity" json:"severity"`
	Pattern  string `yaml:"pattern" json:"pattern"` // Go (RE2) regular expression, matched against one line
	Cause    string `yaml:"cause" json:"cause"`
	Fix      string `yaml:"fix" json:"fix"`
	Doc      string `yaml:"doc" json:"doc"`
}

// Match is the first line of a log that matched a rule. Line is cut to 400
// bytes; LineNo is 1-based.
type Match struct {
	Rule
	Line   string `json:"line"`
	LineNo int    `json:"line_no"`
}

type catalogFile struct {
	Rules []Rule `yaml:"rules"`
}

// Load parses and validates the embedded catalog. Rule order is file order.
func Load() ([]Rule, error) {
	var f catalogFile
	dec := yaml.NewDecoder(bytes.NewReader(catalogYAML))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("logdoc catalog: %w", err)
	}
	if err := Validate(f.Rules); err != nil {
		return nil, err
	}
	return f.Rules, nil
}

// Validate checks that every rule is complete, its pattern compiles and ids
// are unique.
func Validate(rules []Rule) error {
	seen := map[string]bool{}
	var errs []error
	for i, r := range rules {
		who := fmt.Sprintf("rule %d (%q)", i+1, r.ID)
		switch {
		case r.ID == "":
			errs = append(errs, fmt.Errorf("rule %d: empty id", i+1))
		case seen[r.ID]:
			errs = append(errs, fmt.Errorf("%s: duplicate id", who))
		}
		seen[r.ID] = true
		for name, v := range map[string]string{"pattern": r.Pattern, "cause": r.Cause, "fix": r.Fix, "doc": r.Doc} {
			if strings.TrimSpace(v) == "" {
				errs = append(errs, fmt.Errorf("%s: empty %s", who, name))
			}
		}
		if r.Severity != SeverityError && r.Severity != SeverityWarn {
			errs = append(errs, fmt.Errorf("%s: severity %q is not %s or %s", who, r.Severity, SeverityError, SeverityWarn))
		}
		if r.Pattern != "" {
			if _, err := regexp.Compile(r.Pattern); err != nil {
				errs = append(errs, fmt.Errorf("%s: pattern: %w", who, err))
			}
		}
	}
	return errors.Join(errs...)
}

// compiled is a rule ready to match. lits is a set of case-sensitive literals
// of which every matching line contains at least one; it lets Scan skip most
// lines without running the regular expression. Empty means no prefilter.
type compiled struct {
	idx  int
	re   *regexp.Regexp
	lits [][]byte
}

func (c compiled) matches(line []byte) bool {
	if len(c.lits) > 0 && !slices.ContainsFunc(c.lits, func(l []byte) bool { return bytes.Contains(line, l) }) {
		return false
	}
	return c.re.Match(line)
}

func compile(rules []Rule) []compiled {
	out := make([]compiled, 0, len(rules))
	for i, r := range rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			continue // Load rejects such rules; hand-built rules are skipped
		}
		c := compiled{idx: i, re: re}
		if sx, err := syntax.Parse(r.Pattern, syntax.Perl); err == nil {
			for _, l := range requiredLiterals(sx.Simplify()) {
				c.lits = append(c.lits, []byte(l))
			}
		}
		out = append(out, c)
	}
	return out
}

// requiredLiterals returns case-sensitive literals of which every match of re
// contains at least one, or nil when there is no such set.
func requiredLiterals(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase == 0 {
			return []string{string(re.Rune)}
		}
	case syntax.OpCapture, syntax.OpPlus:
		return requiredLiterals(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return requiredLiterals(re.Sub[0])
		}
	case syntax.OpAlternate:
		var all []string
		for _, s := range re.Sub {
			l := requiredLiterals(s)
			if l == nil {
				return nil
			}
			all = append(all, l...)
		}
		return all
	case syntax.OpConcat:
		// Keep the child whose shortest literal is longest.
		var best []string
		bestLen := 0
		for _, s := range re.Sub {
			l := requiredLiterals(s)
			if l == nil {
				continue
			}
			n := len(l[0])
			for _, x := range l {
				n = min(n, len(x))
			}
			if n > bestLen {
				best, bestLen = l, n
			}
		}
		return best
	}
	return nil
}

// Scan reads r line by line and returns, for each rule that matches, its first
// matching line. Matches come in rule order. Scan holds one line in memory.
// It matches the first MaxLineBytes of each line. It stops reading once every
// rule has matched.
func Scan(r io.Reader, rules []Rule) []Match {
	live := compile(rules)
	found := make([]*Match, len(rules))
	br := bufio.NewReaderSize(r, 64<<10)
	var line []byte
	for lineNo := 1; len(live) > 0; lineNo++ {
		var ok bool
		if line, ok = readLine(br, line[:0]); !ok {
			break
		}
		kept := live[:0]
		for _, c := range live {
			if !c.matches(line) {
				kept = append(kept, c)
				continue
			}
			found[c.idx] = &Match{Rule: rules[c.idx], Line: show(line), LineNo: lineNo}
		}
		live = kept
	}
	var out []Match
	for _, m := range found {
		if m != nil {
			out = append(out, *m)
		}
	}
	return out
}

// readLine appends the next line (without the line break, capped at
// MaxLineBytes) to buf. ok is false at the end of input.
func readLine(br *bufio.Reader, buf []byte) (line []byte, ok bool) {
	for {
		frag, more, err := br.ReadLine()
		if len(frag) > 0 || err == nil {
			ok = true
		}
		if room := MaxLineBytes - len(buf); room > 0 {
			buf = append(buf, frag[:min(len(frag), room)]...)
		}
		if err != nil || !more {
			return buf, ok
		}
	}
}

func show(line []byte) string {
	s := strings.ToValidUTF8(string(line[:min(len(line), maxShownLine)]), "\uFFFD")
	if len(line) > maxShownLine {
		s += "…"
	}
	return s
}

// ScanFile scans the file at path with the embedded catalog.
func ScanFile(path string) ([]Match, error) {
	rules, err := Load()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Scan(f, rules), nil
}
