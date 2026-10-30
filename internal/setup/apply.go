package setup

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Apply runs, in order, the steps that are not done and do not need root,
// streaming their output to log. Root steps are printed, not run. only
// limits the run to the named step ids. It stops at the first failure.
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error {
	_, err := ApplyRun(ctx, steps, only, log, ApplyOptions{Mirror: true})
	return err
}

// ApplyOptions say how a run is recorded and how its output is streamed.
type ApplyOptions struct {
	// Mode, Recipe and Venv describe the run.
	Mode, Recipe, Venv string
	// Store keeps the record of the run. A nil store keeps no record:
	// the steps run as they always did.
	Store *RunStore
	// Mirror writes the raw output of the steps to log, next to the log
	// of the step. The dashboard and `up` show the output; the CLI shows
	// the progress and keeps the output in the log of the step.
	// The BERNIE_JSON evidence line is never mirrored: it goes to the
	// record only, `setup report` prints it.
	Mirror bool
	// Verbose streams the markers ("== step: title") and the command
	// echo ("$ cmd") to log, on top of the progress printer. Without it
	// the CLI prints nothing but the running/done lines.
	Verbose bool
	// Quiet leaves the markers ("== step: title") out of log.
	Quiet bool
	// Event receives the events of the run. The CLI streams them.
	Event func(Event)
	// Clock is the clock of the run. The tests replace it.
	Clock func() time.Time
}

// Event is one thing that happened in a run. The CLI streams the events
// as NDJSON, the record of the run is the last one.
type Event struct {
	// Type is the kind of event: run_started, step_started, progress,
	// step_done or run_done.
	Type  string    `json:"type"`
	RunID string    `json:"run_id"`
	At    time.Time `json:"at"`
	// Index and Total place the step in the run: "[3/7]".
	Index int      `json:"index,omitempty"`
	Total int      `json:"total,omitempty"`
	Step  *RunStep `json:"step,omitempty"`
	Run   *Run     `json:"run,omitempty"`
	Error string   `json:"error,omitempty"`
}

// ApplyRun runs the steps of the plan like Apply and records the run: one
// record per run, one log per step, the state, the duration and the
// evidence of every step. The record is written after every step and
// every two seconds while a step runs, so a crash or a reload still shows
// the truth.
func ApplyRun(ctx context.Context, steps []Step, only []string, log io.Writer, o ApplyOptions) (*Run, error) {
	sel, err := selectSteps(steps, only)
	if err != nil {
		return nil, err
	}
	a := &applier{opt: o, log: log}
	a.run = &Run{ID: NewRunID(), Mode: o.Mode, Recipe: o.Recipe, Venv: o.Venv, State: RunRunning, StartedAt: a.now()}
	if o.Store != nil {
		if err := o.Store.Create(a.run); err != nil {
			return nil, err
		}
	}
	a.emit(Event{Type: "run_started", RunID: a.run.ID, At: a.run.StartedAt})
	var rootSteps []Step
	var runErr error
	total := runnableCount(steps, sel)
	index := 0
loop:
	for _, s := range steps {
		if !sel[s.ID] {
			continue
		}
		switch {
		case s.Done:
			a.skip(s, "already done")
		case s.Root:
			a.block(s, "needs root: run the commands yourself")
			rootSteps = append(rootSteps, s)
		default:
			index++
			if err := a.step(ctx, s, index, total); err != nil {
				runErr = err
				break loop
			}
		}
	}
	if len(rootSteps) > 0 {
		a.marker("\nRun these root steps yourself, then run setup apply again:")
		for _, s := range rootSteps {
			a.marker("# " + s.ID + ": " + s.Title)
			for _, c := range s.Commands {
				a.marker(c)
			}
		}
	}
	end := a.now()
	a.run.EndedAt = &end
	switch {
	case runErr != nil:
		a.run.State = RunFailed
		a.run.Error = runErr.Error()
	case total == 0 && len(rootSteps) > 0:
		a.run.State = RunBlocked
	default:
		a.run.State = RunDone
	}
	a.save()
	a.emit(Event{Type: "run_done", RunID: a.run.ID, At: end, Run: a.run, Error: a.run.Error})
	return a.run, runErr
}

// selectSteps resolves the step ids the run covers. An unknown id is an
// error.
func selectSteps(steps []Step, only []string) (map[string]bool, error) {
	sel := map[string]bool{}
	if len(only) == 0 {
		for _, s := range steps {
			sel[s.ID] = true
		}
		return sel, nil
	}
	for _, id := range only {
		found := false
		for _, s := range steps {
			if s.ID == id {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown step %q", id)
		}
		sel[id] = true
	}
	return sel, nil
}

// runnableCount counts the steps of the run that will run: the selected
// ones that are not done and need no root.
func runnableCount(steps []Step, sel map[string]bool) int {
	n := 0
	for _, s := range steps {
		if sel[s.ID] && !s.Done && !s.Root {
			n++
		}
	}
	return n
}
