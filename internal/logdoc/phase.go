package logdoc

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed phase.yaml
var phaseYAML []byte

// Phase is what the server is doing now, read from the tail of its log.
// Done and Total are the progress fraction (shards, graph sizes); they are
// zero when the log does not say how far the step is.
type Phase struct {
	ID       string `json:"id"`
	Sentence string `json:"sentence"`
	Done     int    `json:"done,omitempty"`
	Total    int    `json:"total,omitempty"`
}

// PhaseRule is one row of the phase catalogue.
type PhaseRule struct {
	ID               string `yaml:"id" json:"id"`
	Match            string `yaml:"match" json:"match"`
	Sentence         string `yaml:"sentence" json:"sentence"`
	Progress         string `yaml:"progress" json:"progress,omitempty"`
	ProgressSentence string `yaml:"progress_sentence" json:"progress_sentence,omitempty"`
}

type phaseFile struct {
	Phases []PhaseRule `yaml:"phases"`
}

// LoadPhases parses and validates the embedded phase catalogue.
func LoadPhases() ([]PhaseRule, error) {
	var f phaseFile
	dec := yaml.NewDecoder(bytes.NewReader(phaseYAML))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("logdoc phase catalogue: %w", err)
	}
	if err := validatePhases(f.Phases); err != nil {
		return nil, err
	}
	return f.Phases, nil
}

// validatePhases checks that every phase is complete, its patterns compile and
// its ids are unique.
func validatePhases(rules []PhaseRule) error {
	seen := map[string]bool{}
	var errs []error
	for i, r := range rules {
		who := fmt.Sprintf("phase %d (%q)", i+1, r.ID)
		switch {
		case r.ID == "":
			errs = append(errs, fmt.Errorf("phase %d: empty id", i+1))
		case seen[r.ID]:
			errs = append(errs, fmt.Errorf("%s: duplicate id", who))
		}
		seen[r.ID] = true
		for name, v := range map[string]string{"match": r.Match, "sentence": r.Sentence} {
			if strings.TrimSpace(v) == "" {
				errs = append(errs, fmt.Errorf("%s: empty %s", who, name))
			}
		}
		if _, err := regexp.Compile(r.Match); err != nil {
			errs = append(errs, fmt.Errorf("%s: match: %w", who, err))
		}
		if r.Progress != "" {
			re, err := regexp.Compile(r.Progress)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: progress: %w", who, err))
			} else if re.NumSubexp() != 2 {
				errs = append(errs, fmt.Errorf("%s: progress needs two groups (done and total), has %d", who, re.NumSubexp()))
			}
			if strings.TrimSpace(r.ProgressSentence) == "" {
				errs = append(errs, fmt.Errorf("%s: progress without progress_sentence", who))
			}
		}
	}
	return errors.Join(errs...)
}

// matcher is a phase rule with its patterns compiled.
type matcher struct {
	rule PhaseRule
	re   *regexp.Regexp
	prog *regexp.Regexp
}

// matchers returns the compiled catalogue. It is read once.
var matchers = sync.OnceValues(func() ([]matcher, error) {
	rules, err := LoadPhases()
	if err != nil {
		return nil, err
	}
	out := make([]matcher, 0, len(rules))
	for _, r := range rules {
		re, err := regexp.Compile(r.Match)
		if err != nil {
			continue // LoadPhases rejects such rules
		}
		m := matcher{rule: r, re: re}
		if r.Progress != "" {
			if p, err := regexp.Compile(r.Progress); err == nil {
				m.prog = p
			}
		}
		out = append(out, m)
	}
	return out, nil
})

// PhaseOf returns the phase of the last line of tail that matches the
// catalogue. It returns false when no line matches: a tail with nothing known
// in it has no phase.
func PhaseOf(tail []byte) (Phase, bool) {
	ms, err := matchers()
	if err != nil {
		return Phase{}, false
	}
	lines := bytes.Split(tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		for _, m := range ms {
			if m.re.Match(line) {
				return m.phase(line), true
			}
		}
	}
	return Phase{}, false
}

// phase builds the phase of a matching line. The progress comes from the same
// line; a line without it keeps the plain sentence.
func (m matcher) phase(line []byte) Phase {
	p := Phase{ID: m.rule.ID, Sentence: m.rule.Sentence}
	done, total, ok := m.fraction(line)
	if !ok {
		return p
	}
	p.Sentence = strings.ReplaceAll(m.rule.ProgressSentence, "{done}", strconv.Itoa(done))
	p.Sentence = strings.ReplaceAll(p.Sentence, "{total}", strconv.Itoa(total))
	p.Done, p.Total = done, total
	return p
}

// fraction returns the done and total of the last match of the progress
// pattern in line. A progress bar writes several updates on one line, so the
// last one is the current state.
func (m matcher) fraction(line []byte) (int, int, bool) {
	if m.prog == nil {
		return 0, 0, false
	}
	ms := m.prog.FindAllSubmatch(line, -1)
	if len(ms) == 0 {
		return 0, 0, false
	}
	last := ms[len(ms)-1]
	if len(last) != 3 {
		return 0, 0, false
	}
	done, err := strconv.Atoi(string(last[1]))
	if err != nil {
		return 0, 0, false
	}
	total, err := strconv.Atoi(string(last[2]))
	if err != nil || total <= 0 || done < 0 || done > total {
		return 0, 0, false
	}
	return done, total, true
}
