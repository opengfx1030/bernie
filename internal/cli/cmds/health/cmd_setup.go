package health

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/setup"
)

type setupFlags struct {
	mode, venv, image, model, rocm *string
}

func addSetupFlags(c *cli.Context, fs *flag.FlagSet) setupFlags {
	return setupFlags{
		mode:  fs.String("mode", "", "host or container (default: config mode, else both)"),
		venv:  &c.Venv,
		image: fs.String("image", "", "image for container mode (default "+setup.DefaultImage+")"),
		model: fs.String("model", "", "Hugging Face repo to download (adds the model.download step)"),
		rocm:  fs.String("rocm", "", "ROCm line to install ("+host.ROCmVersion714+" or "+host.ROCmVersion72+", default "+host.ROCmVersion714+")"),
	}
}

// buildPlan probes the host and returns the setup steps.
func buildPlan(c *cli.Context, f setupFlags) ([]setup.Step, error) {
	if err := validMode(*f.mode); err != nil {
		return nil, err
	}
	return app.SetupPlan(c.Ctx, setupOptionsOf(c, f))
}

// validMode refuses a mode that is not one of the two.
func validMode(mode string) error {
	if mode != "" && mode != "host" && mode != "container" {
		return cli.Usagef("--mode must be host or container, not %q", mode)
	}
	return nil
}

func setupOptions(f setupFlags) app.SetupOptions {
	return app.SetupOptions{Mode: *f.mode, Venv: *f.venv, Image: *f.image, Model: *f.model, ROCm: *f.rocm}
}

// setupOptionsOf adds the root of the host tree the command reads.
func setupOptionsOf(c *cli.Context, f setupFlags) app.SetupOptions {
	o := setupOptions(f)
	o.Root = c.Root
	return o
}

func init() {
	cmd := &cli.Command{Name: "setup", Summary: "Install plan for this host and non-root apply"}

	plan := &cli.Command{Name: "plan", Summary: "Ordered install steps for this host", Usage: "[--mode M] [--venv DIR] [--image I] [--model REPO] [--rocm VERSION]"}
	plan.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(plan)
		f := addSetupFlags(c, fs)
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		steps, err := buildPlan(c, f)
		if err != nil {
			return err
		}
		return c.Emit(steps, func(w io.Writer) { printSteps(w, steps) })
	}

	apply := &cli.Command{Name: "apply", Summary: "Run the steps that do not need root; print the root steps", Usage: "[--mode M] [--step ID]... [--rocm VERSION] [--yes] [-v]"}
	apply.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(apply)
		f := addSetupFlags(c, fs)
		var only cli.MultiFlag
		fs.Var(&only, "step", "run only this step (repeatable)")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		verbose := fs.Bool("v", false, "also stream the raw step output (markers and command echo)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		steps, err := buildPlan(c, f)
		if err != nil {
			return err
		}
		todo := app.SetupPending(steps, only)
		if len(todo) == 0 {
			c.Logf("nothing to run: every selected non-root step is done")
		} else if !*yes {
			printSteps(c.Err, todo)
			if !confirm(c.Err, "run these steps? [y/N] ") {
				return fmt.Errorf("aborted (use --yes to skip the prompt)")
			}
		}
		return runSteps(c, f, steps, only, *verbose)
	}

	verify := &cli.Command{Name: "verify", Summary: "Check the venv again: versions, the fork ops, the libraries", Usage: "[--venv DIR] [-v]"}
	verify.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(verify)
		f := addSetupFlags(c, fs)
		verbose := fs.Bool("v", false, "also stream the raw step output")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		if err := validMode(*f.mode); err != nil {
			return err
		}
		opts := setupOptionsOf(c, f)
		opts.Verbose = *verbose
		// The one verify function: the plan, the forced check and the
		// apply of the check, so the run record is the one of the other
		// front ends.
		return runAndPrint(c, opts, func(o app.SetupOptions) (*app.SetupResult, error) {
			return app.SetupVerify(c.Ctx, o, c.Err)
		})
	}

	cmd.Subs = []*cli.Command{plan, apply, verify, setupReportCommand(), setupLogCommand()}
	cli.Register(cmd)
}

// runSteps applies the selected steps through the one apply function and
// prints the run: one line per step, NDJSON with --json.
func runSteps(c *cli.Context, f setupFlags, steps []setup.Step, only []string, verbose bool) error {
	opts := setupOptionsOf(c, f)
	opts.Verbose = verbose
	return runAndPrint(c, opts, func(o app.SetupOptions) (*app.SetupResult, error) {
		return app.SetupApply(c.Ctx, steps, o, only, c.Err)
	})
}

// runAndPrint runs one setup operation and prints its run: one line per
// step, NDJSON with --json. The operation is the one app function of the
// command: apply for the steps, verify for the check of the venv.
func runAndPrint(c *cli.Context, opts app.SetupOptions, run func(app.SetupOptions) (*app.SetupResult, error)) error {
	// JSON and verbose both bypass the live CR line: JSON streams NDJSON,
	// verbose interleaves raw output that would overwrite the cursor. One
	// line per event is what both want.
	printer := &runPrinter{w: c.Err, tty: isTerminal(os.Stderr) && !opts.Verbose && !c.JSON}
	var enc *json.Encoder
	if c.JSON {
		// NDJSON: one event per line, the record of the run last.
		enc = json.NewEncoder(c.Out)
		enc.SetEscapeHTML(false)
		printer.events = func(ev setup.Event) { _ = enc.Encode(ev) }
	}
	opts.OnEvent = printer.event
	res, err := run(opts)
	printer.finish()
	if c.JSON {
		if res != nil && res.Run != nil {
			_ = enc.Encode(map[string]any{"type": "run", "run": res.Run})
		}
		return err
	}
	if err != nil {
		return err
	}
	return c.Emit(res.Steps, func(w io.Writer) { printSteps(w, res.Steps) })
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
