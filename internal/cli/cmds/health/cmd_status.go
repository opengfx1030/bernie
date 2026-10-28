package health

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/onboarding"
)

func init() {
	cmd := &cli.Command{Name: "status", Summary: "Overview of this host and the next command to type", Usage: "[--venv DIR] [--steps]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		steps := fs.Bool("steps", false, "print the onboarding checklist instead of the overview")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		s, err := flow.Snapshot(c.Ctx, c.Venv)
		if err != nil {
			return err
		}
		return c.Emit(s, func(w io.Writer) {
			if *steps {
				printChecklist(w, s.Onboarding)
				return
			}
			printStatus(w, s)
		})
	}
	cli.Register(cmd)
}

// printChecklist prints the onboarding checklist: every step, its state and the
// command that does it.
func printChecklist(w io.Writer, doc *onboarding.Document) {
	if doc == nil {
		fmt.Fprintln(w, "onboarding: the state of this host is unknown")
		return
	}
	fmt.Fprintln(w, "onboarding")
	for _, f := range doc.Facts {
		fmt.Fprintf(w, "  %-8s %s\n", f.Label, f.Value)
		if f.Why != "" {
			fmt.Fprintf(w, "           %s\n", f.Why)
		}
	}
	for _, s := range doc.Steps {
		fmt.Fprintf(w, "  %-8s %-12s %s\n", s.State, s.ID, s.Summary)
		if s.Progress != nil && s.Progress.Phase != "" {
			fmt.Fprintf(w, "           %s\n", s.Progress.Phase)
		}
		for _, p := range s.Problems {
			fmt.Fprintf(w, "           [%s] %s\n", p.Status, p.Title)
			if p.Fix != "" {
				fmt.Fprintf(w, "           fix: %s\n", p.Fix)
			}
		}
		for _, cmd := range s.RootCommands {
			fmt.Fprintf(w, "           root: %s\n", cmd)
		}
		if s.Command != "" {
			fmt.Fprintf(w, "           run:  %s\n", s.Command)
		}
	}
	if doc.Next != "" {
		fmt.Fprintf(w, "next: %s\n", doc.Next)
	} else {
		fmt.Fprintln(w, "next: the host is ready")
	}
}

func printStatus(w io.Writer, s *flow.Summary) {
	h := s.Host
	fmt.Fprintf(w, "host    %s, %s, %d GiB RAM\n", cli.OrDash(h.OS.Pretty), cli.OrDash(h.Kernel), h.RAMBytes>>30)
	if len(s.GPUs) == 0 {
		fmt.Fprintln(w, "gpus    none found")
	} else {
		counts := map[string]int{}
		var order []string
		for _, g := range s.GPUs {
			k := fmt.Sprintf("%s %s %d GiB", g.GFX, g.Name, g.VRAMBytes>>30)
			if counts[k] == 0 {
				order = append(order, k)
			}
			counts[k]++
		}
		parts := make([]string, 0, len(order))
		for _, k := range order {
			parts = append(parts, fmt.Sprintf("%d× %s", counts[k], k))
		}
		fmt.Fprintf(w, "gpus    %s\n", strings.Join(parts, "; "))
	}
	if v := s.Venv; v != nil {
		if v.Path != "" {
			fmt.Fprintf(w, "venv    %s (from %s)\n", v.Path, v.Source)
		} else {
			fmt.Fprintf(w, "venv    none (from %s)\n", v.Source)
		}
		if len(v.Candidates) > 0 {
			fmt.Fprintf(w, "        candidates: %s\n", strings.Join(v.Candidates, ", "))
			fmt.Fprintln(w, "        choose one: bernie config set venv DIR")
		}
	}
	if s.Restriction != "" {
		fmt.Fprintf(w, "%s\n", s.Restriction)
	}
	d := s.Doctor
	fmt.Fprintf(w, "doctor  %d ok, %d warn, %d fail, %d skip\n", d[doctor.OK], d[doctor.Warn], d[doctor.Fail], d[doctor.Skip])
	for _, ch := range s.Problems {
		line := fmt.Sprintf("          [%s] %s", statusMark[ch.Status], ch.Title)
		if ch.Fix != "" {
			line += "  → " + ch.Fix
		}
		fmt.Fprintln(w, line)
	}
	if s.Best != "" {
		fmt.Fprintf(w, "recipe  best fit: %s (%d fit this host)\n", s.Best, len(s.Fits))
	}
	if len(s.Runs) > 0 {
		fmt.Fprintln(w, "runs")
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, r := range s.Runs {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", r.Name, r.State, r.Mode, r.URL())
		}
		tw.Flush()
		for _, r := range s.Runs {
			if len(r.Findings) > 0 {
				ids := make([]string, len(r.Findings))
				for i, m := range r.Findings {
					ids[i] = m.ID
				}
				fmt.Fprintf(w, "    %s log: %s (bernie ps shows cause and fix)\n", r.Name, strings.Join(ids, ", "))
			}
		}
	}
	fmt.Fprintln(w, "\nnext:")
	for _, n := range s.Next {
		fmt.Fprintf(w, "  %s\n", n)
	}
}
