package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opengfx1030/rdnactl/internal/cli"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/opengfx1030/rdnactl/internal/doctor"
	"github.com/opengfx1030/rdnactl/internal/logdoc"
	"github.com/opengfx1030/rdnactl/internal/serving/recipe"
	"github.com/opengfx1030/rdnactl/internal/support/config"
	"github.com/opengfx1030/rdnactl/internal/support/execx"
	"github.com/opengfx1030/rdnactl/internal/support/paths"
)

func init() {
	cmd := &cli.Command{Name: "doctor", Summary: "Run all host checks (exit 3 if one fails)", Usage: "[--category C]... [--venv DIR] | --log FILE|- | --run NAME"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		var cats cli.MultiFlag
		fs.Var(&cats, "category", "limit to a category: "+strings.Join(doctor.Categories, ", ")+" (repeatable)")
		venvFlag := fs.String("venv", "", "venv for the python checks (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV)")
		logFlag := fs.String("log", "", "match a vLLM log file against the known-failure catalog instead of running host checks (- reads stdin)")
		runFlag := fs.String("run", "", "like --log, for the log of the run NAME")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		if *logFlag != "" || *runFlag != "" {
			if *logFlag != "" && *runFlag != "" {
				return cli.Usagef("--log and --run are mutually exclusive")
			}
			if len(cats) > 0 || *venvFlag != "" {
				return cli.Usagef("--category and --venv apply to host checks, not to --log or --run")
			}
			return doctorLog(c, *logFlag, *runFlag)
		}
		for _, cat := range cats {
			if !slices.Contains(doctor.Categories, cat) {
				return cli.Usagef("unknown category %q (have %s)", cat, strings.Join(doctor.Categories, ", "))
			}
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		r := doctor.Run(c.Ctx, doctor.Options{Root: "/", Venv: cfg.ResolveVenv(*venvFlag), Categories: cats, HFHome: cfg.ResolveHFHome(), ValidatedArchs: recipe.ValidatedArchs(recipe.Load())})
		if err := c.Emit(r, func(w io.Writer) { printDoctor(w, r) }); err != nil {
			return err
		}
		if r.Failed() {
			return cli.ChecksFailed("%d checks failed", r.Summary[doctor.Fail])
		}
		return nil
	}
	cli.Register(cmd)
}

var statusMark = map[doctor.Status]string{doctor.OK: "ok  ", doctor.Warn: "WARN", doctor.Fail: "FAIL", doctor.Skip: "skip"}

func printDoctor(w io.Writer, r doctor.Report) {
	cat := ""
	for _, ch := range r.Checks {
		if ch.Category != cat {
			if cat != "" {
				fmt.Fprintln(w)
			}
			cat = ch.Category
			fmt.Fprintf(w, "%s\n", cat)
		}
		line := fmt.Sprintf("  [%s] %s", statusMark[ch.Status], ch.Title)
		if ch.Detail != "" {
			line += ": " + ch.Detail
		}
		fmt.Fprintln(w, line)
		if ch.Status == doctor.Warn || ch.Status == doctor.Fail {
			if ch.Fix != "" {
				fmt.Fprintf(w, "         fix: %s\n", ch.Fix)
			}
			if ch.Doc != "" {
				fmt.Fprintf(w, "         doc: %s\n", ch.Doc)
			}
		}
	}
	s := r.Summary
	fmt.Fprintf(w, "\n%d ok, %d warn, %d fail, %d skip\n", s[doctor.OK], s[doctor.Warn], s[doctor.Fail], s[doctor.Skip])
}

// logReport is the --json document of the log doctor.
type logReport struct {
	Matches []logdoc.Match `json:"matches"`
}

// doctorLog scans one log (a file, stdin or a run's log) and prints the
// matched rules. It fails with exit code 3 when any rule matches.
func doctorLog(c *cli.Context, file, run string) error {
	rules, err := logdoc.Load()
	if err != nil {
		return err
	}
	var src string
	var matches []logdoc.Match
	switch {
	case run != "":
		src = "run " + run
		matches, err = scanRunLog(c, rules, run)
	case file == "-":
		src = "stdin"
		matches = logdoc.Scan(os.Stdin, rules)
	default:
		src = file
		var f *os.File
		if f, err = os.Open(file); err == nil {
			matches = logdoc.Scan(f, rules)
			f.Close()
		}
	}
	if err != nil {
		return err
	}
	rep := logReport{Matches: matches}
	if rep.Matches == nil {
		rep.Matches = []logdoc.Match{}
	}
	if err := c.Emit(rep, func(w io.Writer) { printLogReport(w, src, rep.Matches) }); err != nil {
		return err
	}
	if len(matches) > 0 {
		return cli.ChecksFailed("%d known failure(s) in %s", len(matches), src)
	}
	return nil
}

// scanRunLog scans the log of a run. A host run's log is the file named by the
// "log" field of run.json (serve.log in the run directory when the field is
// empty or the file is gone). A container run's log comes from the engine.
func scanRunLog(c *cli.Context, rules []logdoc.Rule, name string) ([]logdoc.Match, error) {
	dir := filepath.Join(paths.State(), "runs", name)
	b, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no run named %q (looked in %s)", name, dir)
		}
		return nil, err
	}
	var rec struct {
		Engine    string `json:"engine"`
		Container string `json:"container"`
		Log       string `json:"log"`
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, fmt.Errorf("run %s: bad run.json: %w", name, err)
	}
	if rec.Engine != "" && rec.Container != "" {
		res, err := execx.Run(c.Ctx, 60*time.Second, rec.Engine, "logs", rec.Container)
		if err != nil {
			return nil, err
		}
		if !res.OK() {
			return nil, fmt.Errorf("%s logs %s failed: %s", rec.Engine, rec.Container, execx.FirstLine(res.Stderr))
		}
		return logdoc.Scan(io.MultiReader(strings.NewReader(res.Stdout), strings.NewReader(res.Stderr)), rules), nil
	}
	path := rec.Log
	if _, err := os.Stat(path); path == "" || err != nil {
		path = filepath.Join(dir, "serve.log")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return logdoc.Scan(f, rules), nil
}

func printLogReport(w io.Writer, src string, ms []logdoc.Match) {
	if len(ms) == 0 {
		fmt.Fprintf(w, "no known failures found in %s\n", src)
		return
	}
	fmt.Fprintf(w, "%d known failure(s) in %s\n", len(ms), src)
	for _, m := range ms {
		fmt.Fprintf(w, "\n[%s] %s (line %d)\n", m.Severity, m.ID, m.LineNo)
		fmt.Fprintf(w, "  cause: %s\n", m.Cause)
		fmt.Fprintf(w, "  fix:   %s\n", m.Fix)
		fmt.Fprintf(w, "  doc:   %s\n", m.Doc)
		fmt.Fprintf(w, "  line:  %s\n", strings.TrimSpace(m.Line))
	}
}
