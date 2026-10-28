package health

import (
	"errors"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"os"
	"time"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/flow"
)

func init() {
	cmd := &cli.Command{Name: "up", Summary: "Doctor, setup, best recipe, serve and dashboard in one command",
		Usage: "[RECIPE] [--set KEY=VALUE]... [--mode host|container] [--yes] [--no-dash]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		var sets cli.MultiFlag
		fs.Var(&sets, "set", "override: INPUT=VALUE or section.key=VALUE, repeatable")
		mode := fs.String("mode", "", "host or container (default: config; else container when the recipe and an engine allow it)")
		venv := fs.String("venv", "", "host-mode venv")
		engine := fs.String("engine", "", "docker or podman")
		name := fs.String("name", "", "run name (default: recipe id)")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		noDash := fs.Bool("no-dash", false, "do not start the dashboard after the server is healthy")
		listen := fs.String("listen", "127.0.0.1:7860", "dashboard listen address")
		wait := fs.Duration("wait", 30*time.Minute, "how long to wait for /health")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return cli.Usagef("up takes at most one recipe id")
		}
		o := flow.UpOptions{Sets: sets, Mode: *mode, Venv: *venv, Engine: *engine, Name: *name, Yes: *yes, Wait: *wait}
		if len(pos) == 1 {
			o.Recipe = pos[0]
		}
		if !*yes && !c.JSON && isTerminal(os.Stdin) {
			o.Confirm = flow.ConfirmFrom(os.Stdin, c.Err)
		}
		// The dashboard starts while the server loads the model, so the
		// operator can watch the log in the browser (spec §3.1).
		var dash *dashboard.Starter
		if !c.JSON && !*noDash {
			dash = dashboard.NewStarter(dashboard.Options{Listen: *listen, Log: c.Err, Version: cli.Version,
				Ready: func(url string) { fmt.Fprintf(c.Out, "dashboard: %s\n", url) }})
			o.Dashboard = dash
		}
		res, err := flow.Up(c.Ctx, o, c.Err)
		var b *flow.Blocked
		if errors.As(err, &b) {
			if c.JSON {
				_ = c.Emit(map[string]any{"blocked": b}, nil)
			} else {
				fmt.Fprintf(c.Err, "up: stopped: %s\n", b.Reason)
				for _, cmd := range b.Commands {
					fmt.Fprintf(c.Err, "  %s\n", cmd)
				}
			}
			return &cli.ExitError{Code: cli.ExitChecks, Err: errors.New("")}
		}
		var se *flow.StartupExit
		if errors.As(err, &se) && len(se.Findings) > 0 {
			return c.ReportExit(se.Name, se.Findings)
		}
		if err != nil {
			return err
		}
		if c.JSON {
			return c.Emit(res, func(w io.Writer) { printUp(w, res) })
		}
		printUp(c.Out, res)
		if dash != nil {
			return dash.Wait() // Ctrl-C stops the dashboard; the run keeps running
		}
		return nil
	}
	cli.Register(cmd)
}

func printUp(w io.Writer, r *flow.UpResult) {
	fmt.Fprintf(w, "%s is serving (%s mode) at %s\n", r.Recipe, r.Mode, r.URL)
	fmt.Fprintf(w, "  logs: bernie logs %s -f   stop: bernie stop %s   bench: bernie bench\n", r.Run.Name, r.Run.Name)
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
