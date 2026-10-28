package tunecmd

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
)

// powerCommand builds the `tune power` command group.
func powerCommand() *cli.Command {
	power := &cli.Command{Name: "power", Summary: "GPU power caps"}

	status := &cli.Command{Name: "status", Summary: "Actual power cap per GPU"}
	status.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(status), args); err != nil {
			return err
		}
		ps, err := app.PowerStatus()
		if err != nil {
			return err
		}
		return c.Emit(ps, func(w io.Writer) {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "GPU\tNAME\tCAP W\tMIN W\tMAX W")
			for _, p := range ps {
				fmt.Fprintf(tw, "%d\t%s\t%.0f\t%.0f\t%.0f\n", p.HIPIndex, p.Name, p.CapW, p.MinW, p.MaxW)
			}
			tw.Flush()
		})
	}

	set := &cli.Command{Name: "set", Summary: "Write the desired power cap", Usage: "<WATTS> [--gpu IDX]..."}
	set.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(set)
		var idxFlags cli.MultiFlag
		fs.Var(&idxFlags, "gpu", "HIP index, repeatable (default: all GPUs)")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("tune power set needs WATTS")
		}
		watts, err := strconv.ParseFloat(pos[0], 64)
		if err != nil || watts <= 0 {
			return cli.Usagef("WATTS: %q is not a positive number", pos[0])
		}
		idx, err := parsePowerIndexes(idxFlags)
		if err != nil {
			return err
		}
		res, err := app.PowerDesiredSet(idx, watts)
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			target := "all GPUs"
			if len(idx) > 0 {
				target = "GPU " + strings.Join(res.Selectors, ", GPU ")
			}
			fmt.Fprintf(w, "desired power cap: %.0f W (%s)\n", watts, target)
			fmt.Fprintf(w, "file: %s\n", res.File)
		})
	}

	apply := &cli.Command{Name: "apply", Summary: "Apply the desired caps to hwmon", Usage: "[--file FILE]"}
	apply.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(apply)
		file := fs.String("file", "", "desired-state file (default: <config>/power.yaml)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		return applyPower(c, *file, app.PowerApply)
	}

	service := powerServiceCommand()
	power.Subs = []*cli.Command{status, set, apply, service}
	return power
}

func applyPower(c *cli.Context, file string, apply func(string) (tuning.PowerApplyResult, error)) error {
	res, err := apply(file)
	if errors.Is(err, tuning.ErrNeedsRoot) {
		if emitErr := c.Emit(res, func(w io.Writer) { fmt.Fprintln(w, res.RootCommand) }); emitErr != nil {
			return emitErr
		}
		return cli.ChecksFailed("needs root: run the command above")
	}
	if err != nil {
		return err
	}
	return c.Emit(res, func(w io.Writer) { printPowerApply(w, res) })
}

func powerServiceCommand() *cli.Command {
	service := &cli.Command{Name: "service", Summary: "Manage the power-cap systemd service"}

	printCmd := &cli.Command{Name: "print", Summary: "Print the generated systemd units"}
	printCmd.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(printCmd), args); err != nil {
			return err
		}
		plan, err := app.GeneratePowerServicePlan()
		if err != nil {
			return err
		}
		return c.Emit(plan, func(w io.Writer) {
			fmt.Fprintf(w, "# %s\n%s\n", plan.ServiceFile, plan.Service)
			fmt.Fprintf(w, "# %s\n%s\n", plan.PathFile, plan.Path)
		})
	}

	install := &cli.Command{Name: "install-plan", Summary: "Print the root commands that install the service"}
	install.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(install), args); err != nil {
			return err
		}
		plan, err := app.GeneratePowerServicePlan()
		if err != nil {
			return err
		}
		return c.Emit(plan, func(w io.Writer) {
			fmt.Fprintln(w, "# run as root")
			for _, command := range plan.InstallCommands {
				fmt.Fprintln(w, command)
			}
		})
	}

	status := &cli.Command{Name: "status", Summary: "Service state, desired caps and drift"}
	status.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(status), args); err != nil {
			return err
		}
		st, err := app.ReadPowerServiceStatus(c.Ctx)
		if err != nil {
			return err
		}
		return c.Emit(st, func(w io.Writer) { printPowerStatus(w, st) })
	}

	remove := &cli.Command{Name: "remove-plan", Summary: "Print the root commands that remove the service"}
	remove.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(remove), args); err != nil {
			return err
		}
		plan, err := app.GeneratePowerServicePlan()
		if err != nil {
			return err
		}
		return c.Emit(plan, func(w io.Writer) {
			fmt.Fprintln(w, "# run as root")
			for _, command := range plan.RemoveCommands {
				fmt.Fprintln(w, command)
			}
		})
	}

	service.Subs = []*cli.Command{printCmd, install, status, remove}
	return service
}

func parsePowerIndexes(flags cli.MultiFlag) ([]int, error) {
	var idx []int
	for _, s := range flags {
		for _, f := range strings.Split(s, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil || n < 0 {
				return nil, cli.Usagef("--gpu: %q is not a HIP index", f)
			}
			idx = append(idx, n)
		}
	}
	return idx, nil
}

func printPowerApply(w io.Writer, res tuning.PowerApplyResult) {
	for _, g := range res.GPUs {
		if !g.Changed {
			fmt.Fprintf(w, "GPU %d: %.0f W (unchanged)\n", g.HIPIndex, g.EffectiveW)
			continue
		}
		note := ""
		if g.Clamped {
			note = fmt.Sprintf(" (clamped to %.0f-%.0f W)", g.MinW, g.MaxW)
		}
		fmt.Fprintf(w, "GPU %d: %.0f W -> %.0f W%s\n", g.HIPIndex, g.CurrentW, g.EffectiveW, note)
	}
}

func printPowerStatus(w io.Writer, st app.PowerServiceStatus) {
	fmt.Fprintf(w, "state:     %s\n", st.State)
	fmt.Fprintf(w, "service:   installed=%t enabled=%t active=%t (%s)\n", st.Installed, st.Enabled, st.Active, st.Service.ActiveState)
	fmt.Fprintf(w, "path:      installed=%t enabled=%t active=%t (%s)\n", st.Path.Installed, st.Path.Enabled, st.Path.Active, st.Path.ActiveState)
	fmt.Fprintf(w, "desired:   %s (%s)\n", st.DesiredFile, presentMissing(st.DesiredExists))
	fmt.Fprintf(w, "drift:     %d GPU(s)\n", st.DriftCount)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "GPU\tNAME\tCAP W\tWANT W\tEFFECTIVE W\tRANGE W\tDRIFT")
	for _, g := range st.GPUs {
		want, effective := "–", "–"
		if g.DesiredW != nil {
			want = fmt.Sprintf("%.0f", *g.DesiredW)
		}
		if g.EffectiveW != nil {
			effective = fmt.Sprintf("%.0f", *g.EffectiveW)
		}
		drift := "no"
		if g.Drift {
			drift = "yes"
		}
		fmt.Fprintf(tw, "%d\t%s\t%.0f\t%s\t%s\t%.0f-%.0f\t%s\n", g.HIPIndex, g.Name, g.CapW, want, effective, g.MinW, g.MaxW, drift)
	}
	tw.Flush()
}

func presentMissing(present bool) string {
	if present {
		return "present"
	}
	return "missing"
}
