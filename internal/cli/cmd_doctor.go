package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/doctor"
)

func init() {
	cmd := &Command{Name: "doctor", Summary: "Run all host checks (exit 3 if one fails)", Usage: "[--category C]... [--venv DIR]"}
	cmd.Run = func(c *Context, args []string) error {
		fs := c.Flags(cmd)
		var cats MultiFlag
		fs.Var(&cats, "category", "limit to a category: "+strings.Join(doctor.Categories, ", ")+" (repeatable)")
		venvFlag := fs.String("venv", "", "venv for the python checks (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV)")
		if _, err := Parse(fs, args); err != nil {
			return err
		}
		for _, cat := range cats {
			if !slices.Contains(doctor.Categories, cat) {
				return Usagef("unknown category %q (have %s)", cat, strings.Join(doctor.Categories, ", "))
			}
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		r := doctor.Run(c.Ctx, doctor.Options{Root: "/", Venv: cfg.ResolveVenv(*venvFlag), Categories: cats, HFHome: cfg.ResolveHFHome()})
		if err := c.Emit(r, func(w io.Writer) { printDoctor(w, r) }); err != nil {
			return err
		}
		if r.Failed() {
			return ChecksFailed("%d checks failed", r.Summary[doctor.Fail])
		}
		return nil
	}
	Register(cmd)
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
