package flow

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// Dashboard is the dashboard that `up` starts while the server loads the
// model (spec §3.1). The CLI implements it with the dashboard package.
type Dashboard interface {
	// Start starts the dashboard. It returns when the dashboard listens, or
	// with an error when it cannot start (the port is in use, for example).
	Start(ctx context.Context) error
}

// Runner starts a run and lists the runs. The runner package is the real
// one; a test can fake it.
type Runner interface {
	// Start starts the plan and returns the record of the run.
	Start(ctx context.Context, p *recipe.Plan, o runner.StartOptions) (*runner.Record, error)
	// List returns the recorded runs with their live state.
	List(ctx context.Context) ([]runner.Status, error)
}

// liveRunner is the runner package behind the Runner interface.
type liveRunner struct{}

func (liveRunner) Start(ctx context.Context, p *recipe.Plan, o runner.StartOptions) (*runner.Record, error) {
	return runner.Start(ctx, p, o)
}

func (liveRunner) List(ctx context.Context) ([]runner.Status, error) { return runner.List(ctx) }

// runner returns the runner of the options: the real one unless a test set
// another.
func (o UpOptions) runner() Runner {
	if o.Runner != nil {
		return o.Runner
	}
	return liveRunner{}
}

// serveAndWait starts the run, starts the dashboard and waits until the run
// answers /health (spec §3.1). It is the tail of Up. A dashboard that cannot
// start is reported and does not stop the wait.
func serveAndWait(ctx context.Context, o UpOptions, log io.Writer, p *recipe.Plan, res *UpResult) error {
	run := o.runner()
	for _, w := range p.Warnings {
		fmt.Fprintf(log, "up: warning: %s\n", w)
	}
	rec, err := run.Start(ctx, p, runner.StartOptions{Venv: o.Venv, Engine: o.Engine, Progress: log})
	if err != nil {
		return err
	}
	res.Run = rec
	res.URL = rec.URL()
	if o.Dashboard != nil {
		fmt.Fprintf(log, "up: starting the dashboard (Ctrl-C stops it; the server keeps running)\n")
		if err := o.Dashboard.Start(ctx); err != nil {
			fmt.Fprintf(log, "up: dashboard: %v\n", err)
		}
	}
	fmt.Fprintf(log, "up: waiting for %s to answer /health (logs: bernie logs %s -f)\n", rec.Name, rec.Name)
	if err := waitHealthy(ctx, rec.Name, o.Wait, log, run.List); err != nil {
		return err
	}
	res.Healthy = true
	return nil
}

// healthPoll is the delay between two checks of the run state in the wait
// loop. A test shortens it.
var healthPoll = 3 * time.Second

// waitHealthy waits until the run answers /health (limit 0 = 30 minutes).
// list gives the runs with their live state.
func waitHealthy(ctx context.Context, name string, limit time.Duration, progress io.Writer,
	list func(context.Context) ([]runner.Status, error)) error {
	if limit == 0 {
		limit = 30 * time.Minute
	}
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var printer phasePrinter
	var rec runner.Record
	next := time.Time{}
	for {
		if time.Now().After(next) {
			next = time.Now().Add(healthPoll)
			runs, err := list(ctx)
			if err != nil {
				return err
			}
			for _, s := range runs {
				if s.Name != name {
					continue
				}
				rec = s.Record
				switch s.State {
				case runner.StateRunning:
					return nil
				case runner.StateExited:
					return &StartupExit{Name: name, Findings: Findings(ctx, s.Record)}
				}
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s is not healthy after %s; see: bernie logs %s", name, limit, name)
			}
		}
		if ph, ok := runner.Phase(ctx, rec); ok {
			printer.print(name, ph)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
