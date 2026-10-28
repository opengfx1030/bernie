package onboarding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
)

// The kinds of action a step can have.
const (
	ActionSetup  = "setup"
	ActionServe  = "serve"
	ActionVerify = "verify"
)

// ErrNoAction means the step has nothing bernie can run: it needs root, or
// it is already done.
var ErrNoAction = errors.New("this step has nothing bernie can run")

// Action is what a step does when it runs. The dashboard, the jobs and `up`
// all run the same action.
type Action struct {
	Step   string
	Kind   string
	Steps  []setup.Step // the setup steps to apply (kind setup)
	Recipe string
	Mode   string
	Venv   string
	Sets   []string
	Engine string
	Name   string
	// RunName is the run a serve action started. It is empty until the run
	// starts.
	RunName string
	// Limit bounds the wait for a healthy server. 0 = 30 minutes.
	Limit time.Duration
}

// SetupAction returns the action that applies the given setup steps. `up`
// and the jobs run the same action.
func SetupAction(steps []setup.Step) *Action {
	return &Action{Kind: ActionSetup, Steps: steps}
}

// ActionFor returns the action of a step. It returns ErrNoAction when the
// step needs root or has nothing left to do.
func ActionFor(f *Facts, step string, o Options) (*Action, error) {
	a := &Action{Step: step, Venv: o.Venv, Sets: o.Sets, Engine: o.Engine, Name: o.Name}
	switch step {
	case StepPythonEnv:
		pending := pendingSetup(f.Setup, pythonSetupSteps)
		if len(pending) == 0 {
			return nil, ErrNoAction
		}
		a.Kind, a.Steps = ActionSetup, pending
	case StepModel:
		st := setupStep(f.Setup, "model.download")
		if st == nil || st.Done {
			return nil, ErrNoAction
		}
		a.Kind, a.Steps = ActionSetup, []setup.Step{*st}
	case StepServe:
		if f.Recipe == "" {
			return nil, ErrNoAction
		}
		a.Kind, a.Recipe, a.Mode = ActionServe, f.Recipe, f.Mode
	case StepVerify:
		a.Kind = ActionVerify
	default:
		return nil, ErrNoAction
	}
	return a, nil
}

// Run does the action and writes its progress to log.
func (a *Action) Run(ctx context.Context, log io.Writer) error {
	switch a.Kind {
	case ActionSetup:
		ids := make([]string, len(a.Steps))
		for i, s := range a.Steps {
			ids[i] = s.ID
		}
		return setup.Apply(ctx, a.Steps, ids, log)
	case ActionServe:
		return a.serve(ctx, log)
	case ActionVerify:
		return a.verify(ctx, log)
	}
	return ErrNoAction
}

// serve starts the run and waits until it answers /health. The phase
// sentences of the log go to log.
func (a *Action) serve(ctx context.Context, log io.Writer) error {
	p, err := runner.Prepare(ctx, runner.PrepareOptions{
		Recipe: a.Recipe, Mode: a.Mode, Name: a.Name, Venv: a.Venv, Engine: a.Engine, Sets: a.Sets,
	})
	if err != nil {
		return err
	}
	rec, err := runner.Start(ctx, p, runner.StartOptions{Venv: a.Venv, Engine: a.Engine})
	if err != nil {
		return err
	}
	a.RunName = rec.Name
	fmt.Fprintf(log, "started run %s on %s\n", rec.Name, rec.URL())
	return a.waitHealthy(ctx, rec, log)
}

// waitHealthy waits until the run answers /health. A run that exits first is
// an error: the job scans its log for the known failures.
func (a *Action) waitHealthy(ctx context.Context, rec *runner.Record, log io.Writer) error {
	limit := a.Limit
	if limit == 0 {
		limit = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	tick := time.NewTicker(healthPoll)
	defer tick.Stop()
	last := ""
	for {
		runs, err := runner.List(ctx)
		if err == nil {
			for _, r := range runs {
				if r.Name != rec.Name {
					continue
				}
				switch r.State {
				case runner.StateRunning:
					fmt.Fprintf(log, "%s answers /health\n", rec.Name)
					return nil
				case runner.StateExited:
					return fmt.Errorf("run %s exited during startup; see: bernie logs %s", rec.Name, rec.Name)
				}
				if ph, ok := runner.Phase(ctx, r.Record); ok && ph.Sentence != last {
					last = ph.Sentence
					fmt.Fprintln(log, ph.Sentence)
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("run %s did not answer /health in time: %w", rec.Name, ctx.Err())
		case <-tick.C:
		}
	}
}

// verify asks the running server for one token: the smallest request that
// proves the server answers.
func (a *Action) verify(ctx context.Context, log io.Writer) error {
	run, err := LiveRun(ctx)
	if err != nil {
		return err
	}
	res, err := Smoke(ctx, run.URL, "")
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "the server answered: %s\n", res.Reply)
	return nil
}

// healthPoll is the delay between two checks of the run state.
const healthPoll = 3 * time.Second
