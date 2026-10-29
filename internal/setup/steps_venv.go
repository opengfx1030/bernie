package setup

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu/devices"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
)

func (p planner) python() string { return filepath.Join(p.o.Venv, "bin", "python") }

// pipInstall is the install command prefix for the venv (uv when present).
func (p planner) pipInstall() string {
	if which("uv") != "" {
		return "uv pip install --python " + shq(p.python())
	}
	return shq(p.python()) + " -m pip install"
}

func (p planner) venvCreate() Step {
	cmd := "python3 -m venv " + shq(p.o.Venv)
	switch {
	case which("uv") != "":
		cmd = "uv venv --python 3.12 " + shq(p.o.Venv)
	case which("python3.12") != "":
		cmd = "python3.12 -m venv " + shq(p.o.Venv)
	}
	return Step{
		ID: "venv.create", Title: "Create the Python venv", Mode: "host",
		Done:     isFile(p.python()),
		Commands: []string{cmd},
		Why:      "Host mode runs vLLM from a venv at " + p.o.Venv + " (Python 3.12, the wheels target cp312).",
	}
}

func (p planner) venvTorch() Step {
	done := p.has("torch-*+rocm*.dist-info")
	cmds := []string{
		p.pipInstall() + " --pre --index-url " + WheelIndex + " " + strings.Join([]string{shq(TorchPin), shq(TorchVisionPin), shq(TorchAudioPin), shq(TritonPin)}, " "),
	}
	var devices []string
	for _, gfx := range p.targets {
		devices = append(devices, shq("rocm-sdk-device-"+gfx+"=="+ROCmWheel), shq("amd-torch-device-"+gfx+"=="+strings.TrimPrefix(TorchPin, "torch==")))
		if !p.has("rocm_sdk_device_" + gfx + "-*.dist-info") {
			done = false
		}
	}
	why := "torch and the ROCm runtime come from AMD's multi-arch index; "
	if len(devices) > 0 {
		cmds = append(cmds, p.pipInstall()+" --index-url "+WheelIndex+" "+strings.Join(devices, " "))
		why += "the device wheels add the code objects for " + strings.Join(p.targets, ", ") + ". "
	} else {
		why += "no GPU was found, so no device wheels are planned. "
	}
	why += "Triton is pinned to " + strings.TrimPrefix(TritonPin, "triton==") + "."
	if a, ok := p.tritonDropped(); ok {
		why += " Triton " + a.TritonBelow + " removed " + a.Label() + "."
	}
	return Step{
		ID: "venv.torch", Title: "Install ROCm " + ROCmWheel + " wheels and torch", Mode: "host",
		Done: done, Commands: cmds, Why: why,
	}
}

// tritonDropped returns the arch of a host GPU whose generation newer Triton
// releases dropped (the device tables say which).
func (p planner) tritonDropped() (devices.Arch, bool) {
	for _, gfx := range p.o.GPUArchs {
		if a, ok := devices.LookupArch(gfx); ok && a.TritonBelow != "" {
			return a, true
		}
	}
	return devices.Arch{}, false
}

// rocmHome is the newest install with hipcc (TheRock trees preferred on
// ties), else /opt/rocm.
func (p planner) rocmHome() string {
	ins := append([]rocm.Install(nil), p.r.Installs...)
	sort.SliceStable(ins, func(i, j int) bool { return rocm.CompareVersions(ins[i].Version, ins[j].Version) > 0 })
	for _, in := range ins {
		if isFile(filepath.Join(in.Root, "bin", "hipcc")) {
			return in.Root
		}
	}
	if len(ins) > 0 {
		return ins[0].Root
	}
	return "/opt/rocm"
}

func (p planner) archList() string { return strings.Join(p.targets, ";") }

func (p planner) causalConv1D() Step {
	build := ""
	if len(p.targets) > 0 {
		build = "HIP_ARCHITECTURES=" + shq(p.archList()) + " "
	}
	return Step{
		ID: "causal-conv1d.build", Title: "Build causal-conv1d from source", Mode: "host",
		Done: p.has("causal_conv1d-*.dist-info"),
		Commands: []string{
			"CAUSAL_CONV1D_FORCE_BUILD=TRUE " + build + "ROCM_PATH=" + shq(p.rocmHome()) + " " +
				p.pipInstall() + " --no-build-isolation " + shq(CausalConv1DRef),
		},
		Why: "Mamba/GDN models need causal-conv1d; the PyPI wheels have no RDNA kernels.",
	}
}

func (p planner) flashAttn() Step {
	var archs []string
	for _, a := range p.o.GPUArchs {
		if t, ok := devices.LookupArch(a); ok && t.FlashAttn {
			archs = append(archs, a)
		}
	}
	if len(archs) == 0 {
		return Step{} // no allowlisted GPU: the step does not apply
	}
	dir := filepath.Join(p.o.SrcDir, "flash-attention")
	return Step{
		ID: "flash-attn.build", Title: "Build FlashAttention (Triton AMD backend)", Mode: "host",
		Done: p.has("flash_attn-*.dist-info"),
		Commands: []string{
			"test -d " + shq(dir+"/.git") + " || git clone " + FlashAttnRepo + " " + shq(dir),
			"cd " + shq(dir) + " && git submodule update --init && ENABLE_CK=0 PREBUILD_KERNELS=0 FLASH_ATTENTION_TRITON_AMD_ENABLE=TRUE GPU_ARCHS=" +
				shq(strings.Join(archs, ";")) + " " + p.pipInstall() + " --no-build-isolation .",
		},
		Why: "FlashAttention for " + strings.Join(archs, ", ") + "; targets that are not on its ROCm allowlist are left out.",
	}
}

func (p planner) vllmBuild() Step {
	dir := filepath.Join(p.o.SrcDir, "vllm-rdna")
	rh := p.rocmHome()
	jobs := p.h.CPU.Cores
	if jobs <= 0 || jobs > 16 {
		jobs = 16
	}
	vars := []string{"VLLM_TARGET_DEVICE=rocm", "VLLM_USE_PRECOMPILED=0", "ENABLE_CK=0", "FLASH_ATTENTION_TRITON_AMD_ENABLE=TRUE"}
	if len(p.targets) > 0 {
		vars = append(vars, "PYTORCH_ROCM_ARCH="+shq(p.archList()))
	}
	env := strings.Join(append(vars,
		"ROCM_HOME="+shq(rh), "ROCM_PATH="+shq(rh), "HIP_PATH="+shq(rh),
		"CMAKE_HIP_COMPILER="+shq(rh+"/bin/hipcc"),
		"VLLM_PYTHON_EXECUTABLE="+shq(p.python()),
		// The branch carries no tag: setuptools-scm cannot name the
		// package without this.
		"SETUPTOOLS_SCM_PRETEND_VERSION="+shq(ForkVersion),
		fmt.Sprintf("MAX_JOBS=%d", jobs),
	), " ")
	return Step{
		ID: "vllm.build", Title: "Clone and build the vLLM fork (" + ForkBranch + ")", Mode: "host",
		Done: p.has("vllm-*rdna*.dist-info") || p.has("vllm-*.dist-info") && isFile(filepath.Join(dir, "vllm", "_rocm_C.abi3.so")),
		Commands: []string{
			"test -d " + shq(dir+"/.git") + " || git clone -b " + ForkBranch + " " + ForkRepo + " " + shq(dir),
			"cd " + shq(dir) + " && " + env + " " + p.pipInstall() + " --no-build-isolation -e . " + shq(TritonPin),
		},
		Why: "The fork carries the RDNA kernels (_rocm_C rdna ops). Editable install in " + dir + "; the Triton pin is kept.",
	}
}
