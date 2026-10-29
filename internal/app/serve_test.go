package app

import (
	"context"
	"errors"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

type fakeServeRunner struct {
	started bool
	runs    []runner.Status
}

func (f *fakeServeRunner) Start(context.Context, *recipe.Plan, runner.StartOptions) (*runner.Record, error) {
	f.started = true
	return &runner.Record{Name: "run"}, nil
}

func (f *fakeServeRunner) List(context.Context) ([]runner.Status, error) { return f.runs, nil }

func TestServeRefusesActiveName(t *testing.T) {
	f := &fakeServeRunner{runs: []runner.Status{{Record: runner.Record{Name: "run"}, State: runner.StateRunning}}}
	_, err := Serve(context.Background(), ServeOptions{
		Prepared: &recipe.Plan{Name: "run"},
		Runner:   f,
	})
	if !errors.Is(err, ErrRunActive) {
		t.Fatalf("active name: %v", err)
	}
	if f.started {
		t.Error("the run started despite the active name")
	}
}

func TestServeStartsPreparedPlan(t *testing.T) {
	f := &fakeServeRunner{}
	rec, err := Serve(context.Background(), ServeOptions{
		Prepared: &recipe.Plan{Name: "run"},
		Runner:   f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "run" || !f.started {
		t.Errorf("record %+v, started %v", rec, f.started)
	}
}
