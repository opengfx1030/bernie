package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/rdnactl/internal/doctor"
	"github.com/opengfx1030/rdnactl/internal/flow"
)

func init() {
	cmd := &Command{Name: "status", Summary: "Overview of this host and the next command to type", Usage: "[--venv DIR]"}
	cmd.Run = func(c *Context, args []string) error {
		fs := c.Flags(cmd)
		venv := fs.String("venv", "", "host-mode venv (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV)")
		if _, err := Parse(fs, args); err != nil {
			return err
		}
		s, err := flow.Snapshot(c.Ctx, *venv)
		if err != nil {
			return err
		}
		return c.Emit(s, func(w io.Writer) { printStatus(w, s) })
	}
	Register(cmd)
}

func printStatus(w io.Writer, s *flow.Summary) {
	h := s.Host
	fmt.Fprintf(w, "host    %s, %s, %d GiB RAM\n", orDash(h.OS.Pretty), orDash(h.Kernel), h.RAMBytes>>30)
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
	}
	fmt.Fprintln(w, "\nnext:")
	for _, n := range s.Next {
		fmt.Fprintf(w, "  %s\n", n)
	}
}
