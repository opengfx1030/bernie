package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"os"
	"time"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/app/dashsvc"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
)

func init() {
	cmd := &cli.Command{Name: "up", Summary: "Doctor, setup, best recipe, serve and dashboard in one command",
		Usage: "[RECIPE] [--set KEY=VALUE]... [--mode host|container] [--yes] [--no-dash] [-v]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		var sets cli.MultiFlag
		fs.Var(&sets, "set", "override: INPUT=VALUE or section.key=VALUE, repeatable")
		mode := fs.String("mode", "", "host or container (default: config; else container when the recipe and an engine allow it)")
		engine := fs.String("engine", "", "docker or podman")
		name := fs.String("name", "", "run name (default: recipe id)")
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		verbose := fs.Bool("v", false, "also stream the raw step output (markers and command echo)")
		noDash := fs.Bool("no-dash", false, "do not start the dashboard after the server is healthy")
		listen := fs.String("listen", "", "dashboard listen address (default 127.0.0.1:7860; --host binds 0.0.0.0:7860)")
		wait := fs.Duration("wait", 30*time.Minute, "how long to wait for /health")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return cli.Usagef("up takes at most one recipe id")
		}
		// `auto_yes: true` in config is the persistent form of --yes for
		// this command. --yes on the CLI still wins; both are OR'd.
		if !*yes {
			if cfg, err := config.Load(); err == nil && cfg.AutoYes {
				*yes = true
			}
		}
		o := flow.UpOptions{Sets: sets, Mode: *mode, Venv: c.Venv, Engine: *engine, Name: *name, Yes: *yes, Wait: *wait, Verbose: *verbose}
		// The setup steps of the path stream one line per step (spec §6.4).
		// Live CR line on a TTY; one line per event in JSON or verbose
		// (verbose interleaves raw output that would overwrite the cursor).
		printer := &runPrinter{w: c.Err, tty: isTerminal(os.Stderr) && !*verbose && !c.JSON}
		if c.JSON {
			enc := json.NewEncoder(c.Out)
			enc.SetEscapeHTML(false)
			printer.events = func(ev setup.Event) { _ = enc.Encode(ev) }
		}
		o.OnEvent = printer.event
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
			*listen = dashboard.ResolveListen(*listen, c.Host)
			opts := dashboard.Options{Listen: *listen, Log: c.Err, Version: cli.Version,
				Ready: func(url string) { fmt.Fprintf(c.Out, "dashboard: %s\n", url) }}
			if !dashboard.IsLoopback(*listen) {
				// The persisted token keeps the URL the same across
				// restarts (spec §11.1).
				if tok, _, err := dashsvc.ReadToken(); err == nil && tok != "" {
					opts.Token = tok
				}
			}
			dash = dashboard.NewStarter(opts)
			o.Dashboard = dash
		}
		res, err := flow.Up(c.Ctx, o, c.Err)
		printer.finish()
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
			// NDJSON: the events of the setup run, the result last.
			enc := json.NewEncoder(c.Out)
			enc.SetEscapeHTML(false)
			return enc.Encode(res)
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
