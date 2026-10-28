package tuning

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// Power is the power cap state of one GPU. Watts; Path is the power1_cap file.
type Power struct {
	HIPIndex int     `json:"hip_index"`
	Name     string  `json:"name"`
	CapW     float64 `json:"cap_w"`
	MinW     float64 `json:"min_w"`
	MaxW     float64 `json:"max_w"`
	Path     string  `json:"path"`
}

// PowerStatus reads power1_cap, power1_cap_min and power1_cap_max of each GPU.
// GPUs without a hwmon power cap are omitted.
func PowerStatus(gpus []gpu.GPU) []Power {
	return powerStatus("/", gpus)
}

func powerStatus(root string, gpus []gpu.GPU) []Power {
	out := []Power{}
	for _, g := range gpus {
		if g.SysfsPath == "" {
			continue
		}
		hw := gpu.HwmonDir(filepath.Join(root, g.SysfsPath))
		if hw == "" {
			continue
		}
		capPath := filepath.Join(hw, "power1_cap")
		uw, err := readMicro(capPath)
		if err != nil {
			continue
		}
		minW, _ := readMicro(filepath.Join(hw, "power1_cap_min"))
		maxW, _ := readMicro(filepath.Join(hw, "power1_cap_max"))
		out = append(out, Power{HIPIndex: g.HIPIndex, Name: g.Name, CapW: uw, MinW: minW, MaxW: maxW, Path: capPath})
	}
	return out
}

func readMicro(p string) (float64, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(v) / 1e6, nil
}

// SetPower sets power1_cap of the GPUs idx (all GPUs when idx is empty) to
// watts. As root it writes the files and returns no commands. Otherwise it
// writes nothing and returns the exact commands for the operator. A target
// outside [power1_cap_min, power1_cap_max] is an error; the error quotes the
// range observed on that GPU.
func SetPower(gpus []gpu.GPU, idx []int, watts float64) ([]string, error) {
	return setPower("/", gpus, idx, watts, os.Geteuid() == 0)
}

// PowerCommands returns the commands that set the cap, as SetPower prints them
// for a non-root user, whatever the caller's uid. It writes nothing and
// validates the target like SetPower.
func PowerCommands(gpus []gpu.GPU, idx []int, watts float64) ([]string, error) {
	return setPower("/", gpus, idx, watts, false)
}

func setPower(root string, gpus []gpu.GPU, idx []int, watts float64, isRoot bool) ([]string, error) {
	if watts <= 0 || math.IsNaN(watts) || math.IsInf(watts, 0) {
		return nil, fmt.Errorf("power cap must be a positive number of watts")
	}
	status := powerStatus(root, gpus)
	byIdx := map[int]Power{}
	for _, p := range status {
		byIdx[p.HIPIndex] = p
	}
	targets := idx
	if len(targets) == 0 {
		for _, p := range status {
			targets = append(targets, p.HIPIndex)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no GPU with a power cap (hwmon power1_cap) found")
	}
	uw := uint64(math.Round(watts * 1e6))
	var sel []Power
	for _, i := range targets {
		p, ok := byIdx[i]
		if !ok {
			return nil, fmt.Errorf("GPU %d has no power cap (hwmon power1_cap)", i)
		}
		if p.MinW > 0 && watts < p.MinW {
			return nil, fmt.Errorf("GPU %d: %.0f W is below power1_cap_min %.0f W (allowed range %.0f-%.0f W); the driver rejects it (Invalid argument)",
				i, watts, p.MinW, p.MinW, p.MaxW)
		}
		if p.MaxW > 0 && watts > p.MaxW {
			return nil, fmt.Errorf("GPU %d: %.0f W is above power1_cap_max %.0f W (allowed range %.0f-%.0f W)", i, watts, p.MaxW, p.MinW, p.MaxW)
		}
		sel = append(sel, p)
	}
	if !isRoot {
		cmds := make([]string, 0, len(sel))
		for _, p := range sel {
			cmds = append(cmds, fmt.Sprintf("echo %d | sudo tee %s", uw, p.Path))
		}
		return cmds, nil
	}
	for _, p := range sel {
		if err := os.WriteFile(p.Path, []byte(strconv.FormatUint(uw, 10)), 0o644); err != nil {
			return nil, fmt.Errorf("GPU %d: write %s: %w", p.HIPIndex, p.Path, err)
		}
	}
	return nil, nil
}
