package app

import (
	"context"
	"io"
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

// SetupApply runs the pending steps of a plan in order, stopping at the first
// failure, and returns the plan as it stands afterwards. Root steps are never
// run. Progress goes to log.
func SetupApply(ctx context.Context, steps []setup.Step, o SetupOptions, only []string, log io.Writer) ([]setup.Step, error) {
	if err := setup.Apply(ctx, steps, only, log); err != nil {
		return nil, err
	}
	return SetupPlan(ctx, o)
}
