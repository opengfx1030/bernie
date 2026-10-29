package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// powerIsRoot is a variable so tests can exercise the non-root path.
var powerIsRoot = func() bool { return os.Geteuid() == 0 }

// PowerStatus returns the actual power cap of every GPU with a hwmon cap.
func PowerStatus() ([]tuning.Power, error) {
	vis, err := gpu.Visible(root)
	if gpu.Restricted(err) {
		return nil, err
	}
	return tuning.PowerStatus(vis.GPUs), nil
}

// PowerDesiredPath is the desired-state file under the config directory.
func PowerDesiredPath() string { return filepath.Join(paths.Config(), "power.yaml") }

// PowerStateDir is the directory of the generated systemd units.
func PowerStateDir() string { return filepath.Join(paths.State(), "power") }

// PowerDesiredPlan is the proposed desired-state change.
type PowerDesiredPlan struct {
	File       string             `json:"file"`
	Watts      float64            `json:"watts"`
	GPUIndexes []int              `json:"gpu_indexes"`
	Selectors  []string           `json:"selectors"`
	Desired    map[string]float64 `json:"desired"`
	// Clamped names the cards whose own range did not hold the wanted
	// value: they take the clamped value instead.
	Clamped []int `json:"clamped,omitempty"`
}

// PowerDesiredResult is the `tune power set --json` document.
type PowerDesiredResult struct {
	Changed    bool     `json:"changed"`
	File       string   `json:"file"`
	Watts      float64  `json:"watts"`
	GPUIndexes []int    `json:"gpu_indexes"`
	Selectors  []string `json:"selectors"`
	// Clamped names the cards whose own range did not hold the wanted
	// value: they take the clamped value instead.
	Clamped []int `json:"clamped,omitempty"`
}

// PowerGPUStatus is the desired and actual power state of one GPU.
type PowerGPUStatus struct {
	HIPIndex   int      `json:"hip_index"`
	Name       string   `json:"name"`
	PCI        string   `json:"pci"`
	CapW       float64  `json:"cap_w"`
	MinW       float64  `json:"min_w"`
	MaxW       float64  `json:"max_w"`
	DesiredW   *float64 `json:"desired_w,omitempty"`
	EffectiveW *float64 `json:"effective_w,omitempty"`
	Drift      bool     `json:"drift"`
}

// PowerServiceStatus is the `tune power service status --json` document.
type PowerServiceStatus struct {
	DesiredFile     string                `json:"desired_file"`
	DesiredExists   bool                  `json:"desired_exists"`
	State           string                `json:"state"`
	Installed       bool                  `json:"installed"`
	Enabled         bool                  `json:"enabled"`
	Active          bool                  `json:"active"`
	Service         tuning.PowerUnitState `json:"service"`
	Path            tuning.PowerUnitState `json:"path"`
	GPUs            []PowerGPUStatus      `json:"gpus"`
	DriftCount      int                   `json:"drift_count"`
	InstallCommands []string              `json:"install_commands"`
	RemoveCommands  []string              `json:"remove_commands"`
}

// PowerServicePlan is the generated unit pair and its root commands.
type PowerServicePlan struct {
	tuning.PowerUnits
	InstallCommands []string `json:"install_commands"`
	RemoveCommands  []string `json:"remove_commands"`
}

// PlanPowerDesired computes the desired-state change without writing it.
func PlanPowerDesired(idx []int, watts float64) (PowerDesiredPlan, error) {
	if watts <= 0 || math.IsNaN(watts) || math.IsInf(watts, 0) {
		return PowerDesiredPlan{}, fmt.Errorf("power cap must be a positive number of watts")
	}
	file := PowerDesiredPath()
	d, err := tuning.LoadPowerDesired(file)
	if err != nil {
		return PowerDesiredPlan{}, err
	}
	next := copyPowerDesired(d)
	selectors := powerSelectors(idx)
	// One value has to fit every card it touches: the wanted value is
	// clamped to the range the cards share, and the cards that could not
	// take it are named.
	written, clamped, err := clampPowerForCards(idx, watts)
	if err != nil {
		return PowerDesiredPlan{}, err
	}
	for _, selector := range selectors {
		next.Set(selector, written)
	}
	return PowerDesiredPlan{
		File: file, Watts: written, GPUIndexes: append([]int{}, idx...),
		Selectors: selectors, Desired: next.GPUs, Clamped: clamped,
	}, nil
}

// clampPowerForCards fits the wanted watts to the cards the command touches.
// It reads the power range of the cards of this host.
func clampPowerForCards(idx []int, watts float64) (float64, []int, error) {
	gpus, err := gpu.List(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, nil, err
	}
	if gpus == nil {
		gpus = []gpu.GPU{}
	}
	return clampPowerForRows(tuning.PowerStatus(gpus), idx, watts)
}

// clampPowerForRows fits the wanted watts to the range the cards share and
// names the cards whose own range did not hold it. One value has to be one
// every card can take: the range is the highest minimum and the lowest
// maximum of the cards the command touches.
func clampPowerForRows(rows []tuning.Power, idx []int, watts float64) (float64, []int, error) {
	picked := map[int]bool{}
	for _, i := range idx {
		picked[i] = true
	}
	lo, hi := 0.0, 0.0
	clamped := []int{}
	for _, r := range rows {
		if len(picked) > 0 && !picked[r.HIPIndex] {
			continue
		}
		if r.MinW > 0 && (lo == 0 || r.MinW > lo) {
			lo = r.MinW
		}
		if r.MaxW > 0 && (hi == 0 || r.MaxW < hi) {
			hi = r.MaxW
		}
		if (r.MinW > 0 && watts < r.MinW) || (r.MaxW > 0 && watts > r.MaxW) {
			clamped = append(clamped, r.HIPIndex)
		}
	}
	if lo > 0 && hi > 0 && lo > hi {
		return 0, nil, fmt.Errorf("the cards of this host share no power range: %.0f W to %.0f W", lo, hi)
	}
	out, _, err := tuning.ClampPower(watts, lo, hi)
	if err != nil {
		return 0, nil, err
	}
	return out, clamped, nil
}

// PowerDesiredSet writes the desired power state. It needs no root.
func PowerDesiredSet(idx []int, watts float64) (PowerDesiredResult, error) {
	plan, err := PlanPowerDesired(idx, watts)
	if err != nil {
		return PowerDesiredResult{}, err
	}
	old, err := tuning.LoadPowerDesired(plan.File)
	if err != nil {
		return PowerDesiredResult{}, err
	}
	changed := false
	for _, selector := range plan.Selectors {
		if before, ok := old.GPUs[selector]; !ok || before != plan.Watts {
			changed = true
		}
	}
	if err := tuning.SavePowerDesired(plan.File, tuning.PowerDesired{Version: tuning.PowerDesiredVersion, GPUs: plan.Desired}); err != nil {
		return PowerDesiredResult{}, err
	}
	return PowerDesiredResult{
		Changed: changed, File: plan.File, Watts: plan.Watts,
		GPUIndexes: plan.GPUIndexes, Selectors: plan.Selectors, Clamped: plan.Clamped,
	}, nil
}

// PowerApply applies the desired state to hwmon. It needs root.
func PowerApply(file string) (tuning.PowerApplyResult, error) {
	if file == "" {
		file = PowerDesiredPath()
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return tuning.PowerApplyResult{}, err
	}
	binary, err := os.Executable()
	if err != nil {
		return tuning.PowerApplyResult{}, err
	}
	var gpus []gpu.GPU
	if powerIsRoot() {
		gpus, err = gpu.List(root)
		if err != nil {
			return tuning.PowerApplyResult{}, err
		}
	}
	return tuning.ApplyPowerDesired(root, gpus, abs, binary, powerIsRoot())
}

// GeneratePowerServicePlan generates the unit pair, writes it under the state
// directory and returns the root commands.
func GeneratePowerServicePlan() (PowerServicePlan, error) {
	u, err := powerUnits()
	if err != nil {
		return PowerServicePlan{}, err
	}
	if err := u.Write(); err != nil {
		return PowerServicePlan{}, err
	}
	return PowerServicePlan{PowerUnits: u, InstallCommands: u.InstallCommands(), RemoveCommands: tuning.RemoveCommands()}, nil
}

// ReadPowerServiceStatus reports the systemd units, desired caps and drift.
func ReadPowerServiceStatus(ctx context.Context) (PowerServiceStatus, error) {
	gpus, err := gpu.List(root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return PowerServiceStatus{}, err
	}
	if gpus == nil {
		gpus = []gpu.GPU{}
	}
	file := PowerDesiredPath()
	desired, err := tuning.LoadPowerDesired(file)
	if err != nil {
		return PowerServiceStatus{}, err
	}
	want := desired.Resolve(root, gpus)
	actual := tuning.PowerStatus(gpus)
	out := PowerServiceStatus{
		DesiredFile: file, GPUs: []PowerGPUStatus{},
		Service: tuning.ReadPowerUnitState(ctx, tuning.PowerServiceUnit),
		Path:    tuning.ReadPowerUnitState(ctx, tuning.PowerPathUnit),
	}
	if _, err := os.Stat(file); err == nil {
		out.DesiredExists = true
	}
	units, err := powerUnits()
	if err != nil {
		return out, err
	}
	out.InstallCommands = units.InstallCommands()
	out.RemoveCommands = tuning.RemoveCommands()
	out.Installed = out.Service.Installed && out.Path.Installed
	out.Enabled = out.Service.Enabled && out.Path.Enabled
	out.Active = out.Path.Active
	switch {
	case !out.Service.Available || !out.Path.Available:
		out.State = "skip"
	case !out.Installed:
		out.State = "warn"
	case !out.Enabled || !out.Active:
		out.State = "warn"
	default:
		out.State = "ok"
	}
	for _, p := range actual {
		row := PowerGPUStatus{HIPIndex: p.HIPIndex, Name: p.Name, CapW: p.CapW, MinW: p.MinW, MaxW: p.MaxW}
		for _, g := range gpus {
			if g.HIPIndex == p.HIPIndex {
				row.PCI = g.PCI
				break
			}
		}
		if requested, ok := want[p.HIPIndex]; ok {
			effective, _, err := tuning.ClampPower(requested, p.MinW, p.MaxW)
			if err != nil {
				return out, err
			}
			row.DesiredW = &requested
			row.EffectiveW = &effective
			row.Drift = math.Abs(p.CapW-effective) > 0.001
			if row.Drift {
				out.DriftCount++
			}
		}
		out.GPUs = append(out.GPUs, row)
	}
	return out, nil
}

func powerUnits() (tuning.PowerUnits, error) {
	binary, err := os.Executable()
	if err != nil {
		return tuning.PowerUnits{}, err
	}
	return tuning.GeneratePowerUnits(binary, PowerDesiredPath(), PowerStateDir())
}

func powerSelectors(idx []int) []string {
	if len(idx) == 0 {
		return []string{"all"}
	}
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

func copyPowerDesired(d tuning.PowerDesired) tuning.PowerDesired {
	out := tuning.NewPowerDesired()
	for selector, watts := range d.GPUs {
		out.GPUs[selector] = watts
	}
	return out
}
