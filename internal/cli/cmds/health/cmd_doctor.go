package health

import (
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/logdoc"
)

func init() {
	cmd := &cli.Command{Name: "doctor", Summary: "Run all host checks (exit 3 if one fails)", Usage: "[--category C]... [--venv DIR] | --log FILE|- | --run NAME"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		var cats cli.MultiFlag
		fs.Var(&cats, "category", "limit to a category: "+strings.Join(doctor.Categories, ", ")+" (repeatable)")
		venvFlag := fs.String("venv", "", "venv for the python checks (default: $BERNIE_VENV, config venv, $VIRTUAL_ENV)")
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
		r, err := app.Doctor(c.Ctx, cats, *venvFlag)
		if err != nil {
			return err
		}
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

// doctorLog scans one log (a file, stdin or a run's log) and prints the
// matched rules. It fails with exit code 3 when any rule matches.
func doctorLog(c *cli.Context, file, run string) error {
	var src string
	var rep app.LogReport
	var err error
	if run != "" {
		src = "run " + run
		rep, err = app.RunLog(c.Ctx, run)
	} else {
		rep, src, err = scanLogFile(file)
	}
	if err != nil {
		return err
	}
	if err := c.Emit(rep, func(w io.Writer) { printLogReport(w, src, rep.Matches) }); err != nil {
		return err
	}
	if len(rep.Matches) > 0 {
		return cli.ChecksFailed("%d known failure(s) in %s", len(rep.Matches), src)
	}
	return nil
}

// scanLogFile matches a log file, or stdin for "-".
func scanLogFile(file string) (app.LogReport, string, error) {
	rules, err := logdoc.Load()
	if err != nil {
		return app.LogReport{}, "", err
	}
	if file == "-" {
		return app.NewLogReport(logdoc.Scan(os.Stdin, rules)), "stdin", nil
	}
	f, err := os.Open(file)
	if err != nil {
		return app.LogReport{}, "", err
	}
	defer f.Close()
	return app.NewLogReport(logdoc.Scan(f, rules)), file, nil
}

func printLogReport(w io.Writer, src string, ms []logdoc.Match) {
	if len(ms) == 0 {
		fmt.Fprintf(w, "no known failures found in %s\n", src)
		return
	}
	fmt.Fprintf(w, "%d known failure(s) in %s\n", len(ms), src)
	cli.WriteMatches(w, ms)
}
