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
	RDNAOps     int    `json:"rdna_ops"`
	Err         string `json:"error,omitempty"`
}
