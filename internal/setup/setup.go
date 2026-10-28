// Package setup builds the ordered install plan of spec §6 and runs its
// non-root steps. Every step carries a postcondition check (Done) so a
// re-run only does what is missing.
package setup

import (
	"context"
	"path/filepath"

	"github.com/opengfx1030/rdnactl/internal/machine/gpu"
	"github.com/opengfx1030/rdnactl/internal/machine/host"
	"github.com/opengfx1030/rdnactl/internal/machine/rocm"
	"github.com/opengfx1030/rdnactl/internal/support/execx"
	"github.com/opengfx1030/rdnactl/internal/support/paths"
)

// Defaults for the host and container stacks.
const (
	DefaultImage = "docker.io/blivioniag/vllm-rdna:v0.31.0-extras"
	// WheelIndex is AMD's multi-arch ROCm wheel index (TheRock).
	WheelIndex     = "https://repo.amd.com/rocm/whl-multi-arch/"
	ROCmWheel      = "7.14.0"
	TorchPin       = "torch==2.12.0+rocm7.14.0"
	TorchVisionPin = "torchvision==0.27.0+rocm7.14.0"
	TorchAudioPin  = "torchaudio==2.11.0+rocm7.14.0"
	// TritonPin keeps RDNA2 support (Triton 3.9 removed RDNA1/RDNA2).
	TritonPin       = "triton==3.7.1"
	ForkRepo        = "https://github.com/opengfx1030/vllm-rdna.git"
	ForkBranch      = "rdna_extras"
	CausalConv1DRef = "git+https://github.com/Dao-AILab/causal-conv1d.git@4f6ae4e26ae5fe8af9372f8d312ab25cc4595223"
	FlashAttnRepo   = "https://github.com/Dao-AILab/flash-attention.git"
)

// Step is one install step.
type Step struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Mode     string   `json:"mode"` // host | container | both
	Root     bool     `json:"root"`
	Done     bool     `json:"done"`
	Commands []string `json:"commands"`
	Why      string   `json:"why"`
}

// Options select the mode ("host", "container", "" = both), the venv, the
// image, an optional model to download, the source dir for git checkouts
// (default <data>/src) and the Hugging Face cache root (default $HF_HOME,
// else ~/.cache/huggingface). GPUArchs are the host gfx targets (from
// gpu.List), used for device wheels and build targets. Without GPUs the plan
// has no device wheels and no build target.
type Options struct {
	Mode, Venv, Image, ModelRepo string
	SrcDir, HFHome               string
	GPUArchs                     []string
}

// DefaultVenv is the venv used when none is configured.
func DefaultVenv() string { return filepath.Join(paths.Data(), "venv-rocm"+ROCmWheel) }

// Plan returns the ordered steps for the mode, with Done evaluated on this
// host.
func Plan(ctx context.Context, h host.Info, r rocm.Report, o Options) []Step {
	if o.Venv == "" {
		o.Venv = DefaultVenv()
	}
	if o.Image == "" {
		o.Image = DefaultImage
	}
	if o.SrcDir == "" {
		o.SrcDir = filepath.Join(paths.Data(), "src")
	}
	o.GPUArchs = uniq(o.GPUArchs)
	pl := planner{ctx: ctx, h: h, r: r, o: o, site: sitePackages(o.Venv), targets: gpu.CodeTargets(o.GPUArchs)}
	all := []Step{
		pl.groups(), pl.rocmRuntime(), pl.containerEngine(),
		pl.venvCreate(), pl.venvTorch(), pl.causalConv1D(), pl.flashAttn(), pl.vllmBuild(),
		pl.imagePull(), pl.modelDownload(),
	}
	var out []Step
	for _, s := range all {
		if s.ID == "" {
			continue
		}
		if o.Mode == "" || s.Mode == "both" || s.Mode == o.Mode {
			out = append(out, s)
		}
	}
	return out
}

type planner struct {
	ctx  context.Context
	h    host.Info
	r    rocm.Report
	o    Options
	site string
	// targets are the code-object targets of the GPUs (an HSA override
	// target replaces the real one): wheels and build targets use them.
	targets []string
}

// which resolves a tool on PATH; tests replace it.
var which = execx.Which

// has reports a glob match in the venv site-packages.
func (p planner) has(pattern string) bool {
	if p.site == "" {
		return false
	}
	m, _ := filepath.Glob(filepath.Join(p.site, pattern))
	return len(m) > 0
}

func sitePackages(venv string) string {
	if sp := rocm.SitePackages(venv); len(sp) > 0 {
		return sp[0]
	}
	return ""
}
