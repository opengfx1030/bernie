package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/recipe"
	"github.com/opengfx1030/rdnactl/internal/runner"
)

func init() {
	serve := &Command{Name: "serve", Summary: "Start a run of a recipe",
		Usage: "<id> [--set KEY=VALUE]... [--mode host|container] [--name N] [--venv DIR] [--engine E] [--foreground] [--no-pull] [--dry-run]"}
	serve.Run = func(c *Context, args []string) error {
		fs := c.Flags(serve)
		var o runner.PrepareOptions
		var sets MultiFlag
		prepareFlags(fs, &o, &sets)
		fg := fs.Bool("foreground", false, "stay attached; Ctrl-C stops the server")
		noPull := fs.Bool("no-pull", false, "container mode: start the local image without pulling")
		dry := fs.Bool("dry-run", false, "print the plan and start nothing (same as recipe plan)")
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return Usagef("serve needs exactly one recipe id")
		}
		o.Recipe, o.Sets = pos[0], sets
		p, err := runner.Prepare(c.Ctx, o)
		if err != nil {
			return err
		}
		if *dry {
			return c.Emit(p, func(w io.Writer) { printPlan(w, p) })
		}
		for _, w := range p.Warnings {
			c.Logf("warning: %s", w)
		}
		so := runner.StartOptions{Venv: o.Venv, Engine: o.Engine, Foreground: *fg, NoPull: *noPull, Progress: c.Err}
		if *fg && !c.JSON {
			so.Progress = c.Out
		}
		rec, err := runner.Start(c.Ctx, p, so)
		if err != nil {
			return err
		}
		return c.Emit(rec, func(w io.Writer) {
			if *fg {
				fmt.Fprintf(w, "run %s stopped\n", rec.Name)
				return
			}
			who := fmt.Sprintf("pid %d", rec.PID)
			if rec.Container != "" {
				who = fmt.Sprintf("container %s (%s)", rec.Container, short(rec.ImageDigest))
			}
			fmt.Fprintf(w, "started %s: %s, %s mode, GPUs %s\n", rec.Name, who, rec.Mode, p.HIPDevices)
			fmt.Fprintf(w, "url:  %s (rdnactl ps shows when /health answers)\n", rec.URL())
			fmt.Fprintf(w, "log:  rdnactl logs %s -f\n", rec.Name)
			fmt.Fprintf(w, "stop: rdnactl stop %s\n", rec.Name)
		})
	}
	Register(serve)

	ps := &Command{Name: "ps", Summary: "Runs, with state and health"}
	ps.Run = func(c *Context, args []string) error {
		if _, err := Parse(c.Flags(ps), args); err != nil {
			return err
		}
		runs, err := runner.List(c.Ctx)
		if err != nil {
			return err
		}
		return c.Emit(runs, func(w io.Writer) { printRuns(w, runs) })
	}
	Register(ps)

	stop := &Command{Name: "stop", Summary: "Stop a run (the only running one if NAME is absent)", Usage: "[NAME]"}
	stop.Run = func(c *Context, args []string) error {
		pos, err := Parse(c.Flags(stop), args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return Usagef("stop takes at most one run name")
		}
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		if err := runner.Stop(c.Ctx, name); err != nil {
			return err
		}
		return c.Emit(map[string]any{"stopped": true, "name": name}, func(w io.Writer) {
			fmt.Fprintln(w, "stopped")
		})
	}
	Register(stop)

	logs := &Command{Name: "logs", Summary: "Show the log of a run", Usage: "[NAME] [-f] [--tail N]"}
	logs.Run = func(c *Context, args []string) error {
		fs := c.Flags(logs)
		follow := fs.Bool("f", false, "follow the log")
		tail := fs.Int("tail", 100, "last N lines (0 = all)")
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return Usagef("logs takes at most one run name")
		}
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		// The log is text; --json does not apply to a stream.
		return runner.Logs(c.Ctx, name, *follow, *tail, c.Out)
	}
	Register(logs)

	compose := &Command{Name: "compose", Summary: "Write a compose.yml for a container plan",
		Usage: "<id> [--set KEY=VALUE]... [--name N] [-o FILE]"}
	compose.Run = func(c *Context, args []string) error {
		fs := c.Flags(compose)
		var o runner.PrepareOptions
		var sets MultiFlag
		prepareFlags(fs, &o, &sets)
		out := fs.String("o", "", "write to FILE (default: stdout)")
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return Usagef("compose needs exactly one recipe id")
		}
		if o.Mode != "" && o.Mode != recipe.ModeContainer {
			return Usagef("compose writes a container plan; drop --mode %s", o.Mode)
		}
		o.Recipe, o.Sets, o.Mode = pos[0], sets, recipe.ModeContainer
		p, err := runner.Prepare(c.Ctx, o)
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		rows := ""
		if p.TunableOpRows != "" {
			rows = filepath.Dir(p.TunableOpRows)
		}
		b, err := runner.Compose(p, rows, cfg.ResolveHFHome())
		if err != nil {
			return err
		}
		for _, w := range p.Warnings {
			c.Logf("warning: %s", w)
		}
		if *out != "" {
			if err := os.WriteFile(*out, b, 0o644); err != nil {
				return err
			}
			return c.Emit(map[string]string{"file": *out}, func(w io.Writer) {
				fmt.Fprintf(w, "wrote %s (start: docker compose -f %s up -d)\n", *out, *out)
			})
		}
		return c.Emit(map[string]string{"compose": string(b)}, func(w io.Writer) { w.Write(b) })
	}
	Register(compose)
}

func short(digest string) string {
	if len(digest) > 19 {
		return digest[:19]
	}
	return digest
}

func printRuns(w io.Writer, runs []runner.Status) {
	if len(runs) == 0 {
		fmt.Fprintln(w, "no runs")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tRECIPE\tMODE\tSTATE\tPID/CONTAINER\tURL\tSTARTED")
	for _, r := range runs {
		who := fmt.Sprint(r.PID)
		if r.Container != "" {
			who = r.Container
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Recipe, r.Mode, r.State, who, r.URL(),
			r.StartedAt.Local().Format(time.DateTime))
	}
	tw.Flush()
}
