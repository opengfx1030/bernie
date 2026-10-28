package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/host"
	"github.com/opengfx1030/rdnactl/internal/rocm"
	"github.com/opengfx1030/rdnactl/internal/setup"
)

type setupFlags struct {
	mode, venv, image, model *string
}

func addSetupFlags(fs *flag.FlagSet) setupFlags {
	return setupFlags{
		mode:  fs.String("mode", "", "host or container (default: config mode, else both)"),
		venv:  fs.String("venv", "", "venv for host mode (default: $RDNACTL_VENV, config venv, $VIRTUAL_ENV, else "+setup.DefaultVenv()+")"),
		image: fs.String("image", "", "image for container mode (default "+setup.DefaultImage+")"),
		model: fs.String("model", "", "Hugging Face repo to download (adds the model.download step)"),
	}
}

// buildPlan probes the host and returns the setup steps.
func buildPlan(c *Context, f setupFlags) ([]setup.Step, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	mode := *f.mode
	if mode == "" {
		mode = cfg.Mode
	}
	if mode != "" && mode != "host" && mode != "container" {
		return nil, Usagef("--mode must be host or container, not %q", mode)
	}
	h, err := host.Probe(c.Ctx, "/")
	if err != nil {
		return nil, err
	}
	gpus, _ := gpu.List("/")
	var archs []string
	for _, g := range gpus {
		archs = append(archs, g.GFX)
	}
	r := rocm.Probe(c.Ctx, "/")
	return setup.Plan(c.Ctx, h, r, setup.Options{
		Mode: mode, Venv: cfg.ResolveVenv(*f.venv), Image: *f.image, ModelRepo: *f.model,
		HFHome: cfg.ResolveHFHome(), GPUArchs: archs,
	}), nil
}

func init() {
	cmd := &Command{Name: "setup", Summary: "Install plan for this host and non-root apply"}

	plan := &Command{Name: "plan", Summary: "Ordered install steps for this host", Usage: "[--mode M] [--venv DIR] [--image I] [--model REPO]"}
	plan.Run = func(c *Context, args []string) error {
		fs := c.Flags(plan)
		f := addSetupFlags(fs)
		if _, err := Parse(fs, args); err != nil {
			return err
		}
		steps, err := buildPlan(c, f)
		if err != nil {
			return err
		}
		return c.Emit(steps, func(w io.Writer) { printSteps(w, steps) })
	}

	apply := &Command{Name: "apply", Summary: "Run the steps that do not need root; print the root steps", Usage: "[--mode M] [--step ID]... [--yes]"}
	apply.Run = func(c *Context, args []string) error {
		fs := c.Flags(apply)
		f := addSetupFlags(fs)
		var only MultiFlag
		fs.Var(&only, "step", "run only this step (repeatable)")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		if _, err := Parse(fs, args); err != nil {
			return err
		}
		steps, err := buildPlan(c, f)
		if err != nil {
			return err
		}
		todo := pending(steps, only)
		if len(todo) == 0 {
			c.Logf("nothing to run: every selected non-root step is done")
		} else if !*yes {
			printSteps(c.Err, todo)
			if !confirm(c.Err, "run these steps? [y/N] ") {
				return fmt.Errorf("aborted (use --yes to skip the prompt)")
			}
		}
		if err := setup.Apply(c.Ctx, steps, only, c.Err); err != nil {
			return err
		}
		after, err := buildPlan(c, f)
		if err != nil {
			return err
		}
		return c.Emit(after, func(w io.Writer) { printSteps(w, after) })
	}

	cmd.Subs = []*Command{plan, apply}
	Register(cmd)
}

// pending lists the selected non-root steps that are not done.
func pending(steps []setup.Step, only []string) []setup.Step {
	var out []setup.Step
	for _, s := range steps {
		if s.Done || s.Root {
			continue
		}
		if len(only) > 0 && !slices.Contains(only, s.ID) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func confirm(w io.Writer, prompt string) bool {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprintln(w, "stdin is not a terminal")
		return false
	}
	fmt.Fprint(w, prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func printSteps(w io.Writer, steps []setup.Step) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STEP\tMODE\tROOT\tDONE\tTITLE")
	for _, s := range steps {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Mode, yesNo(s.Root), yesNo(s.Done), s.Title)
	}
	tw.Flush()
	for _, s := range steps {
		if s.Done {
			continue
		}
		fmt.Fprintf(w, "\n# %s: %s\n", s.ID, s.Why)
		for _, cmd := range s.Commands {
			fmt.Fprintf(w, "%s\n", cmd)
		}
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
