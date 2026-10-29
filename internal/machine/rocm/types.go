// Package rocm finds ROCm installs, tools and libraries on the host and
// probes a Python venv (torch, vllm, fork ops).
package rocm

// Install is one ROCm installation.
type Install struct {
	Root    string `json:"root"`
	Version string `json:"version"`
	Kind    string `json:"kind"` // opt | therock | distro | wheel
	// Nested holds the trees inside this install (a TheRock tree under a
	// package install). They are part of the same install, not extra ones.
	Nested []Install `json:"nested,omitempty"`
}

// Report is the host-level ROCm probe.
type Report struct {
	Installs   []Install         `json:"installs"`
	Tools      map[string]string `json:"tools"` // rocminfo, hipcc, amd-smi, rocm-smi, rocm-bandwidth-test -> path ("" = missing)
	OnPath     map[string]bool   `json:"on_path,omitempty"`
	ROCBLAS    string            `json:"rocblas"`
	HIPVersion string            `json:"hip_version"`
	Agents     []string          `json:"agents"` // gfx names listed by rocminfo
}

// VenvReport is the result of running a probe script in a venv.
type VenvReport struct {
	Venv        string `json:"venv"`
	Python      string `json:"python"`
	Torch       string `json:"torch"`
	HIP         string `json:"hip"`
	VLLM        string `json:"vllm"`
	DeviceCount int    `json:"device_count"`
	// ROCMOps counts the ops the fork registered in the _rocm_C
	// namespace, RDNAOps the RDNA ones among them.
	ROCMOps int `json:"rocm_ops"`
	RDNAOps int `json:"rdna_ops"`
	// AllReduce is true when the fork registered an all_reduce op.
	AllReduce bool `json:"all_reduce"`
	// ROCMC is the file of the compiled extension of the fork.
	ROCMC string `json:"rocm_c"`
	// MissingLibs are the libraries the extension needs and the loader
	// does not find.
	MissingLibs []string `json:"missing_libs"`
	// ROCBLAS is the rocBLAS a torch process loads, ROCBLASWheel true
	// when it is the copy of the wheel of the venv.
	ROCBLAS      string `json:"rocblas"`
	ROCBLASWheel bool   `json:"rocblas_wheel"`
	// VerifyErrors are the checks of the gfx1030 wiki that failed.
	VerifyErrors []string `json:"verify_errors"`
	// Probed is true when the probe script ran and reported. It is
	// false when the probe could not run at all (a timeout, a missing
	// Python): the state of the venv is then unknown, not bad.
	Probed bool   `json:"-"`
	Err    string `json:"error,omitempty"`
}

// Verified reports whether the venv passes every check of the gfx1030
// wiki: torch and vLLM import, the RDNA ops of the fork are registered,
// the libraries of the extension resolve and rocBLAS comes from the wheel.
func (r VenvReport) Verified() bool {
	return r.Err == "" && len(r.VerifyErrors) == 0
}
