package onboarding

import (
	"context"
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/setup"
)

// ActionPath runs every step of the path that bernie can do itself, in the
// order of the path: the setup steps (the Python environment and the model
// download), then the server, then the question that proves it answers.
//
// It is the action of the one button of the Setup tab and of `bernie up`:
// the same setup.Apply, the same serve and the same check, in one run.
const ActionPath = "path"

// PathAction returns the action of the whole path: every setup step that is
// missing, then the server, then the check.
func PathAction(f *Facts, o Options) *Action {
	a := &Action{Kind: ActionPath, Recipe: f.Recipe, Mode: f.Mode, Venv: o.Venv,
		Sets: o.Sets, Engine: o.Engine, Name: o.Name}
	for _, s := range f.Setup {
		if s.Done || s.Root {
			continue
		}
		a.Steps = append(a.Steps, s)
	}
	return a
}

// StepPath is the id of the job that runs the whole path.
const StepPath = "path"

// PreparePath prepares the action of the whole path from facts that were
// already gathered. It returns the root commands when the operator must act
// first, and the action when bernie can do everything that is missing.
func PreparePath(f *Facts, o Options) (*PreparedStep, error) {
	for _, s := range f.Setup {
		if s.Done || !s.Root {
			continue
		}
		return &PreparedStep{Step: StepPath, RootCommands: s.Commands}, nil
	}
	if f.Recipe == "" {
		return nil, ErrNoAction
	}
	return &PreparedStep{Step: StepPath, Action: PathAction(f, o)}, nil
}

// pathJob is the job of the whole path when one runs now, or when one has
// just finished. The page re-attaches to it with the id.
func pathJob(f *Facts) *PathJob {
	j := lastJob(f.Jobs, StepPath)
	if j == nil {
		return nil
	}
	if j.State != JobRunning && !recentJob(j) {
		return nil
	}
	p := &PathJob{ID: j.ID, State: j.State, Phase: j.Phase, Progress: j.Progress}
	if j.Progress != nil {
		p.Step = j.Progress.Step
	}
	return p
}

// runPath does the whole path: the setup steps, then the server, then the
// check that proves the server answers. Every step it starts says so on a
// line of its own, so the page can show which one runs now.
func (a *Action) runPath(ctx context.Context, log io.Writer) error {
	if len(a.Steps) > 0 {
		ids := make([]string, len(a.Steps))
		for i, s := range a.Steps {
			ids[i] = s.ID
		}
		fmt.Fprintf(log, "== %s: %s\n", StepPythonEnv, "Build the Python environment")
		if err := setup.Apply(ctx, a.Steps, ids, log); err != nil {
			return err
		}
		fmt.Fprintf(log, "== %s: %s\n", StepModel, "Download the model")
	}
	if a.Recipe == "" {
		return ErrNoAction
	}
	fmt.Fprintf(log, "== %s: %s\n", StepServe, "Start the server")
	if err := a.serve(ctx, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "== %s: %s\n", StepVerify, "Ask the server a question")
	return a.verify(ctx, log)
}
