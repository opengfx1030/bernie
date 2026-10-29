package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// ErrRunActive means that a run with the requested name is still active.
var ErrRunActive = errors.New("a run with that name is already active")

// ErrPrepare means that the launch plan could not be prepared.
var ErrPrepare = errors.New("cannot prepare run")

// Runner starts plans and lists runs. The runner package is the real
// implementation; tests can provide another one.
type Runner interface {
	Start(ctx context.Context, p *recipe.Plan, o runner.StartOptions) (*runner.Record, error)
	List(ctx context.Context) ([]runner.Status, error)
}

type liveRunner struct{}

func (liveRunner) Start(ctx context.Context, p *recipe.Plan, o runner.StartOptions) (*runner.Record, error) {
	return runner.Start(ctx, p, o)
}

func (liveRunner) List(ctx context.Context) ([]runner.Status, error) { return runner.List(ctx) }

// ServeOptions are the inputs of Serve. Prepare holds the launch inputs;
// Start holds the process inputs. Prepared is an already built plan, used
// when the caller had to show it before confirmation.
type ServeOptions struct {
	Prepare  runner.PrepareOptions
	Start    runner.StartOptions
	Prepared *recipe.Plan
	Runner   Runner
}

// serveOnboarding starts the run of an onboarding serve action through the
// same shared operation as every front end.
func serveOnboarding(ctx context.Context, o onboarding.ServeOptions) (*runner.Record, error) {
	return Serve(ctx, ServeOptions{
		Prepare: runner.PrepareOptions{Recipe: o.Recipe, Mode: o.Mode, Name: o.Name, Venv: o.Venv, Engine: o.Engine, Sets: o.Sets},
		Start:   runner.StartOptions{Venv: o.Venv, Engine: o.Engine},
	})
}

// Plan prepares a recipe for this host. It is the shared entry of `recipe
// plan`, `serve --dry-run`, and the plan calls of the other front ends.
func Plan(ctx context.Context, o runner.PrepareOptions) (*recipe.Plan, error) {
	return runner.Prepare(ctx, o)
}

// Serve prepares and starts a run. It refuses an active run name before
// the process starts. A prepared plan lets a front end show that exact plan
// before the operator confirms it.
func Serve(ctx context.Context, o ServeOptions) (*runner.Record, error) {
	p := o.Prepared
	if p == nil {
		var err error
		p, err = runner.Prepare(ctx, o.Prepare)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPrepare, err)
		}
	}
	r := o.Runner
	if r == nil {
		r = liveRunner{}
	}
	if p.Name != "" {
		runs, err := r.List(ctx)
		if err == nil {
			for _, run := range runs {
				if run.Name == p.Name && run.State != runner.StateExited {
					return nil, fmt.Errorf("%w: run %s is %s; stop it first or choose another name", ErrRunActive, p.Name, run.State)
				}
			}
		}
	}
	return r.Start(ctx, p, o.Start)
}
