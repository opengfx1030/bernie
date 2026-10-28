package app

import (
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

// hostArchs returns the gfx targets of the GPUs bernie may use (the
// environment can limit them, spec §7.7). It returns nil when the GPUs
// cannot be listed.
func hostArchs() []string {
	vis, err := gpu.Visible(root)
	if err != nil {
		return nil
	}
	var idx []int
	for _, g := range vis.GPUs {
		idx = append(idx, g.HIPIndex)
	}
	return gpu.Archs(vis.GPUs, idx)
}
