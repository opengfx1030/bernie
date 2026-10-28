package app

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// HWReport is the `hw --json` document (spec §4.3).
type HWReport struct {
	Host host.Info `json:"host"`
	GPUs []gpu.GPU `json:"gpus"`
	// GPUErr is why GPUs is empty when the KFD topology could not be read.
	// It is not part of the document: the front end decides what to do.
	GPUErr error `json:"-"`
}

// HW probes the host and its GPUs. A GPU list that cannot be read is not an
// error: the report has no GPUs and GPUErr says why.
func HW(ctx context.Context) (HWReport, error) {
	h, err := host.Probe(ctx, root)
	if err != nil {
		return HWReport{}, err
	}
	gpus, gerr := gpu.List(root)
	if gpus == nil {
		gpus = []gpu.GPU{}
	}
	return HWReport{Host: h, GPUs: gpus, GPUErr: gerr}, nil
}

// TopoOptions select the `topo` variant.
type TopoOptions struct {
	// Bandwidth runs GPU-to-GPU copies (Venv feeds the torch probe).
	Bandwidth bool
	Venv      string
	// NoKFDOK turns a host without a KFD topology into an empty topology.
	NoKFDOK bool
	// Progress receives one line before the bandwidth test. Nil discards.
	Progress io.Writer
}

// Topo is the `topo --json` document.
func Topo(ctx context.Context, o TopoOptions) (topo.Topology, error) {
	gpus, err := gpu.List(root)
	if err != nil {
		if !o.NoKFDOK || !errors.Is(err, fs.ErrNotExist) {
			return topo.Topology{}, err
		}
		gpus = []gpu.GPU{}
	}
	t, err := topo.Discover(root, gpus)
	if err != nil {
		return topo.Topology{}, err
	}
	if o.Bandwidth {
		cfg, err := config.Load()
		if err != nil {
			return topo.Topology{}, err
		}
		logf(o.Progress, "measuring bandwidth...")
		if err := topo.MeasureBandwidth(ctx, &t, cfg.ResolveVenv(o.Venv)); err != nil {
			return topo.Topology{}, err
		}
	}
	return t, nil
}
