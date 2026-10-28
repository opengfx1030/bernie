package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// TunableOpState is the `tune tunableop status --json` document.
type TunableOpState struct {
	Venv    string `json:"venv"`
	ROCBLAS string `json:"rocblas"`
	SHA256  string `json:"sha256"`
	// Archs are the gfx targets of the GPUs the profile must fit: the host's
	// GPUs, or the ones HIP_VISIBLE_DEVICES lists. Empty: no GPU data.
	Archs   []string `json:"archs"`
	Profile string   `json:"profile"`
	RowsDir string   `json:"rows_dir"`
	PerUser string   `json:"per_user_file"`
}

// TunableOpStatus finds the loaded rocBLAS build and its matching profile.
// venv is the --venv flag value ("" = config, then environment).
func TunableOpStatus(venv string) (TunableOpState, error) {
	cfg, err := config.Load()
	if err != nil {
		return TunableOpState{}, err
	}
	st := TunableOpState{Venv: cfg.ResolveVenv(venv), PerUser: tuning.PerUserFile(), Archs: hostArchs()}
	st.ROCBLAS = rocm.FindROCBLAS(root, st.Venv)
	if st.ROCBLAS == "" {
		return st, nil
	}
	if st.SHA256, err = tuning.HashLib(st.ROCBLAS); err != nil {
		return st, err
	}
	p, err := tuning.Select(st.SHA256, "", st.Archs)
	if err != nil {
		return st, err
	}
	if p != nil {
		st.Profile, st.RowsDir = p.Name, tuning.RowsDir(p.Name)
	}
	return st, nil
}

// hostArchs returns the gfx targets of the host's GPUs, limited to the
// HIP_VISIBLE_DEVICES indices when that variable lists indices. It returns
// nil when the GPUs cannot be listed.
func hostArchs() []string {
	gpus, err := gpu.List(root)
	if err != nil {
		return nil
	}
	var idx []int
	if v := os.Getenv("HIP_VISIBLE_DEVICES"); v != "" {
		for _, f := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				idx = nil // not an index list (UUIDs): do not restrict
				break
			}
			idx = append(idx, n)
		}
	}
	if idx == nil {
		for _, g := range gpus {
			idx = append(idx, g.HIPIndex)
		}
	}
	return gpu.Archs(gpus, idx)
}

// PowerStatus is the `tune power status --json` document.
func PowerStatus() ([]tuning.Power, error) {
	gpus, err := gpu.List(root)
	if err != nil {
		return nil, err
	}
	return tuning.PowerStatus(gpus), nil
}

// PowerResult is the `tune power set --json` document. Commands is empty
// when the caps were written; else they are the commands for the operator.
type PowerResult struct {
	Applied  bool     `json:"applied"`
	Commands []string `json:"commands"`
	Watts    float64  `json:"watts"`
}

// PowerPlan validates a power cap and returns the commands that set it. It
// writes nothing, as root too.
func PowerPlan(idx []int, watts float64) ([]string, error) {
	gpus, err := gpu.List(root)
	if err != nil {
		return nil, err
	}
	return tuning.PowerCommands(gpus, idx, watts)
}

// PowerSet sets the power cap of the GPUs idx (all when empty). Without root
// it writes nothing and returns the exact commands.
func PowerSet(idx []int, watts float64) (PowerResult, error) {
	gpus, err := gpu.List(root)
	if err != nil {
		return PowerResult{}, err
	}
	cmds, err := tuning.SetPower(gpus, idx, watts)
	if err != nil {
		return PowerResult{}, err
	}
	return PowerResult{Applied: cmds == nil, Commands: cmds, Watts: watts}, nil
}
