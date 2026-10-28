package tuning

import (
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
