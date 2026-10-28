package setup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Apply runs, in order, the steps that are not done and do not need root,
// streaming their output to log. Root steps are printed, not run. only
// limits the run to the named step ids. It stops at the first failure.
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error {
	sel := map[string]bool{}
	for _, id := range only {
		found := false
		for _, s := range steps {
			if s.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown step %q", id)
		}
		sel[id] = true
	}
	var rootSteps []Step
	for _, s := range steps {
		if len(sel) > 0 && !sel[s.ID] {
			continue
		}
		switch {
		case s.Done:
			fmt.Fprintf(log, "== %s: done\n", s.ID)
		case s.Root:
			fmt.Fprintf(log, "== %s: needs root, not run\n", s.ID)
			rootSteps = append(rootSteps, s)
		default:
			fmt.Fprintf(log, "== %s: %s\n", s.ID, s.Title)
			if s.Download != nil {
				if err := downloadModel(ctx, newHubClient(), s.Download, log); err != nil {
					return fmt.Errorf("step %s: %w", s.ID, err)
				}
				continue
			}
			for _, c := range s.Commands {
				if strings.HasPrefix(strings.TrimSpace(c), "#") {
					fmt.Fprintf(log, "%s\n", c)
					continue
				}
				fmt.Fprintf(log, "$ %s\n", c)
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c)
				cmd.Stdout, cmd.Stderr = log, log
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("step %s: %q: %w", s.ID, c, err)
				}
			}
		}
	}
	if len(rootSteps) > 0 {
		fmt.Fprintln(log, "\nRun these root steps yourself, then run setup apply again:")
		for _, s := range rootSteps {
			fmt.Fprintf(log, "# %s: %s\n", s.ID, s.Title)
			for _, c := range s.Commands {
				fmt.Fprintf(log, "%s\n", c)
			}
		}
	}
	return nil
}
