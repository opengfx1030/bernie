package app

import (
	"context"
	"io"
	"os"
	"slices"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// SetupOptions are the flags of `setup plan` and `setup apply`. Mode, Venv
// and Image may be empty (config, then defaults). ROCm is the ROCm line to
// install, empty for the newest one. Root is the host tree to read, empty
// for the default ("/" or BERNIE_ROOT).
type SetupOptions struct {
	Mode, Venv, Image, Model, ROCm string
	Root                           string
	// OnEvent receives the events of a run (the CLI streams them). It
	// is not a flag.
	OnEvent func(setup.Event)
	// Mirror writes the raw output of the steps to the log. The CLI
	// leaves it off: the output is in the log of the step.
	Mirror bool
	// Verbose streams the markers ("== step: title", "$ cmd") on top of
	// the progress printer. The CLI leaves it off: the progress line is
	// the only live line on a TTY; the per-step log carries everything.
	Verbose bool
}

// SetupPlan probes the host and returns the ordered install steps.
func SetupPlan(ctx context.Context, o SetupOptions) ([]setup.Step, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if o.Mode == "" {
		o.Mode = cfg.Mode
	}
	tree := o.Root
	if tree == "" {
		tree = root
	}
	h, err := host.Probe(ctx, tree)
	if err != nil {
		return nil, err
	}
	vis, _ := gpu.Visible(tree)
	var archs []string
	for _, g := range vis.GPUs {
		archs = append(archs, g.GFX)
	}
	return setup.Plan(ctx, h, rocm.Probe(ctx, tree), setup.Options{
		Mode: o.Mode, Venv: cfg.ResolveVenv(o.Venv), Image: o.Image, ModelRepo: o.Model,
		HFHome: cfg.ResolveHFHome(), GPUArchs: archs, ROCm: o.ROCm, Root: tree,
	}), nil
}

// SetupPending lists the selected steps that apply would run: not done and
// no root needed. only limits the selection to step ids (all when empty).
func SetupPending(steps []setup.Step, only []string) []setup.Step {
	var out []setup.Step
	for _, s := range steps {
		if s.Done || s.Root {
			continue
		}
		if len(only) > 0 && !slices.Contains(only, s.ID) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// SetupResult is what a setup apply produced: the plan as it stands
// afterwards and the record of the run (spec §6.4).
type SetupResult struct {
	Steps []setup.Step `json:"steps"`
	Run   *setup.Run   `json:"run,omitempty"`
}

// SetupApply runs the pending steps of a plan in order, stopping at the first
// failure, and returns the plan as it stands afterwards and the record of the
// run. Root steps are never run. Progress goes to log.
//
// The run is recorded in the store of the host, so the CLI, the MCP server
// and the dashboard read the same record.
func SetupApply(ctx context.Context, steps []setup.Step, o SetupOptions, only []string, log io.Writer) (*SetupResult, error) {
	resolved := setup.ResolveRunVenv(o.Venv)
	run, err := setup.ApplyRun(ctx, steps, only, log, setup.ApplyOptions{
		Mode: o.Mode, Venv: resolved, Store: setup.DefaultRunStore(), Mirror: o.Mirror, Verbose: o.Verbose, Event: o.OnEvent,
	})
	if err != nil {
		return &SetupResult{Run: run}, err
	}
	after, err := SetupPlan(ctx, o)
	if err != nil {
		return &SetupResult{Run: run}, err
	}
	return &SetupResult{Steps: after, Run: run}, nil
}

// SetupStore is the store of the setup runs of this host: <state>/setup.
func SetupStore() *setup.RunStore { return setup.DefaultRunStore() }

// SetupRuns returns the setup runs of this host, newest first. A limit below
// one means every run the store keeps.
func SetupRuns(limit int) []*setup.Run {
	runs, err := SetupStore().List()
	if err != nil {
		return nil
	}
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs
}

// SetupRun returns the record of one setup run.
func SetupRun(id string) (*setup.Run, error) { return SetupStore().Load(id) }

// SetupLatestRun returns the record of the newest setup run.
func SetupLatestRun() (*setup.Run, error) { return SetupStore().Latest() }

// SetupStepLog returns the log of one step of a run.
func SetupStepLog(runID, stepID string) ([]byte, error) {
	return os.ReadFile(SetupStore().StepLog(runID, stepID))
}
