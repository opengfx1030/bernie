package tuning

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// ErrNeedsRoot says that applying a power cap needs a root process.
var ErrNeedsRoot = errors.New("needs root")

// PowerApplyGPU is the outcome for one GPU.
type PowerApplyGPU struct {
	HIPIndex   int     `json:"hip_index"`
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	CurrentW   float64 `json:"current_w"`
	RequestedW float64 `json:"requested_w"`
	EffectiveW float64 `json:"effective_w"`
	MinW       float64 `json:"min_w"`
	MaxW       float64 `json:"max_w"`
	Clamped    bool    `json:"clamped"`
	Changed    bool    `json:"changed"`
	Wrote      bool    `json:"wrote"`
}

// PowerApplyResult is the `tune power apply --json` document.
type PowerApplyResult struct {
	File        string          `json:"file"`
	NeedsRoot   bool            `json:"needs_root"`
	RootCommand string          `json:"root_command,omitempty"`
	GPUs        []PowerApplyGPU `json:"gpus"`
}

// ApplyPowerDesired applies the desired state to the hwmon power caps.
// It never writes anything as a normal user. For every GPU it clamps the
// desired value to the observed range and writes only power1_cap.
func ApplyPowerDesired(root string, gpus []gpu.GPU, file, binary string, isRoot bool) (PowerApplyResult, error) {
	res := PowerApplyResult{File: file, GPUs: []PowerApplyGPU{}}
	if !isRoot {
		res.NeedsRoot = true
		res.RootCommand = PowerRootCommand(binary, file)
		return res, ErrNeedsRoot
	}
	desired, err := LoadPowerDesired(file)
	if err != nil {
		return res, err
	}
	want := desired.Resolve(root, gpus)
	for _, p := range powerStatus(root, gpus) {
		requested, ok := want[p.HIPIndex]
		if !ok {
			continue
		}
		effective, clamped, err := clampPower(requested, p.MinW, p.MaxW)
		if err != nil {
			return res, fmt.Errorf("GPU %d: %w", p.HIPIndex, err)
		}
		one := PowerApplyGPU{
			HIPIndex: p.HIPIndex, Name: p.Name, Path: p.Path,
			CurrentW: p.CapW, RequestedW: requested, EffectiveW: effective,
			MinW: p.MinW, MaxW: p.MaxW, Clamped: clamped,
		}
		one.Changed = math.Abs(p.CapW-effective) > 0.001
		if one.Changed {
			uw := uint64(math.Round(effective * 1e6))
			if err := os.WriteFile(p.Path, []byte(strconv.FormatUint(uw, 10)), 0o644); err != nil {
				return res, fmt.Errorf("GPU %d: write %s: %w", p.HIPIndex, p.Path, err)
			}
			one.Wrote = true
		}
		res.GPUs = append(res.GPUs, one)
	}
	return res, nil
}

// PowerRootCommand returns the one-line command an operator runs as root.
// bernie prints it; it never executes it.
func PowerRootCommand(binary, file string) string {
	if binary == "" {
		binary = "bernie"
	}
	return fmt.Sprintf("sudo %s tune power apply --file %s", shellQuote(binary), shellQuote(file))
}

// clampPower clamps watts to the observed hwmon range.
func clampPower(watts, minW, maxW float64) (float64, bool, error) {
	if minW > 0 && maxW > 0 && minW > maxW {
		return 0, false, fmt.Errorf("invalid hwmon power range %.0f-%.0f W", minW, maxW)
	}
	out := watts
	clamped := false
	if minW > 0 && out < minW {
		out = minW
		clamped = true
	}
	if maxW > 0 && out > maxW {
		out = maxW
		clamped = true
	}
	return out, clamped, nil
}
