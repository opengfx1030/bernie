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

// ForkBuildTools are the build backend and the build tools the fork's
// setup.py imports at module level. The fork is built with
// --no-build-isolation, so the packages have to be in the venv before the
// build: without them the build stops with "No module named
// setuptools_scm". They come from PyPI, the AMD index does not carry
// them all.
var ForkBuildTools = []string{
	"setuptools>=77,<81", "setuptools-scm>=8", "setuptools-rust>=1.9",
	"cmake>=3.26", "ninja", "wheel", "jinja2", "packaging>=24.2",
}

// forkBuildTools are the dist-info names of the build tools, used to
// check that the venv holds them.
var forkBuildTools = []string{
	"setuptools_scm-*.dist-info", "setuptools_rust-*.dist-info",
	"cmake-*.dist-info", "ninja-*.dist-info",
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
	cmds = append(cmds, p.pipInstall()+" "+strings.Join(quoted(ForkBuildTools), " "))
	for _, pat := range forkBuildTools {
		if !p.has(pat) {
			done = false
		}
	}
	why += "The build tools of the fork (" + strings.Join(ForkBuildTools, ", ") + ") come from PyPI: the fork is built with --no-build-isolation, so the venv must hold them. "
	why += "Triton is pinned to " + strings.TrimPrefix(TritonPin, "triton==") + "."
	if a, ok := p.tritonDropped(); ok {
		why += " Triton " + a.TritonBelow + " removed " + a.Label() + "."
	}
	return Step{
		ID: "venv.torch", Title: "Install ROCm " + ROCmWheel + " wheels and torch", Mode: "host",
		Done: done, Commands: cmds, Why: why,
	}
}

// quoted quotes every item of a list for the shell.
func quoted(items []string) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, shq(i))
	}
	return out
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

// rocmHome is the install the host actually runs: the tree whose hipcc wins
// on PATH (a TheRock tree nested inside a package install on par1-cs25).
// Falls back to the newest install with hipcc, nested trees included.
func (p planner) rocmHome() string {
	if path := p.r.Tools["hipcc"]; path != "" {
		if in, ok := rocm.InstallForPath(p.r.Installs, path); ok {
			return in.Root
		}
	}
	ins := rocmAllTrees(p.r.Installs)
	sort.SliceStable(ins, func(i, j int) bool { return rocm.CompareVersions(ins[i].Version, ins[j].Version) > 0 })
	for _, in := range ins {
		if isFile(filepath.Join(in.Root, "bin", "hipcc")) {
			return in.Root
		}
	}
	if len(p.r.Installs) > 0 {
		return p.r.Installs[0].Root
	}
	return "/opt/rocm"
}

// rocmAllTrees flattens nested installs (a TheRock tree under a package
// install counts as its own tree for tool lookup).
func rocmAllTrees(ins []rocm.Install) []rocm.Install {
	var out []rocm.Install
	for _, in := range ins {
		out = append(out, in)
		out = append(out, rocmAllTrees(in.Nested)...)
	}
	return out
}

func (p planner) archList() string { return strings.Join(p.targets, ";") }

// hipArchList is the arch list of the HIP compiler. hipcc takes the
// targets separated by commas; a semicolon ends the argument and the
// build fails with "no input files".
func (p planner) hipArchList() string { return strings.Join(p.targets, ",") }

// venvBin is the bin directory of the venv. A build that runs in the venv
// needs it on PATH: cmake and ninja live there.
func (p planner) venvBin() string { return filepath.Join(p.o.Venv, "bin") }

// venvPath is the PATH of a build that runs in the venv.
func (p planner) venvPath() string { return "PATH=" + shq(p.venvBin()) + ":$PATH" }

func (p planner) causalConv1D() Step {
	build := ""
	if len(p.targets) > 0 {
		build = "HIP_ARCHITECTURES=" + shq(p.hipArchList()) + " "
	}
	return Step{
		ID: "causal-conv1d.build", Title: "Build causal-conv1d from source", Mode: "host",
		Done: p.has("causal_conv1d-*.dist-info"),
		Commands: []string{
			"CAUSAL_CONV1D_FORCE_BUILD=TRUE " + build + "ROCM_PATH=" + shq(p.rocmHome()) + " " +
				p.venvPath() + " " + p.pipInstall() + " --no-build-isolation " + shq(CausalConv1DRef),
		},
		Why: "Mamba/GDN models need causal-conv1d; the PyPI wheels have no RDNA kernels. The HIP compiler takes the targets separated by commas.",
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
		// The build runs in the venv: its bin directory holds cmake and
		// ninja, without them the build cannot start.
		"VIRTUAL_ENV="+shq(p.o.Venv),
		p.venvPath(),
		"ROCM_HOME="+shq(rh), "ROCM_PATH="+shq(rh), "HIP_PATH="+shq(rh),
		"CMAKE_HIP_COMPILER="+shq(rh+"/bin/hipcc"),
		// cmake needs ROCM_HOME for FindHIP.cmake and the ROCm cmake
		// modules (amdsmi, hipruntime, hipblas). Without HIP_ROOT_DIR and
		// CMAKE_HIP_COMPILER_ROCM_ROOT the build falls back to /opt/rocm
		// and the wrong headers win.
		"HIP_ROOT_DIR="+shq(rh),
		"CMAKE_HIP_COMPILER_ROCM_ROOT="+shq(rh),
		// RelWithDebInfo is the gfx1030 wiki build mode: enough debug
		// symbols for hipErrorInvalidImage triage, with -O2 for the rest.
		"CMAKE_BUILD_TYPE=RelWithDebInfo",
		"VLLM_PYTHON_EXECUTABLE="+shq(p.python()),
		// The branch carries no tag: setuptools-scm cannot name the
		// package without this.
		"SETUPTOOLS_SCM_PRETEND_VERSION="+shq(ForkVersion),
		fmt.Sprintf("MAX_JOBS=%d", jobs),
	), " ")
	return Step{
		ID: "vllm.build", Title: "Clone and build the vLLM fork (" + ForkBranch + ")", Mode: "host",
		Done: p.vllmBuilt(dir),
		Commands: []string{
			"test -d " + shq(dir+"/.git") + " || git clone -b " + ForkBranch + " " + ForkRepo + " " + shq(dir),
			// The runtime dependencies of the fork: the files in
			// requirements/common.txt. They are installed here, without
			// the ROCm-only extras of requirements/rocm.txt, so the venv
			// holds what vLLM imports and nothing that needs a build.
			"cd " + shq(dir) + " && " + p.pipInstall() + " -r requirements/common.txt",
			"cd " + shq(dir) + " && " + env + " " + p.pipInstall() + " --no-build-isolation --no-deps -e . " + shq(TritonPin),
		},
		Why: "The fork carries the RDNA kernels (_rocm_C rdna ops). Editable install in " + dir + "; the Triton pin is kept. The install takes no dependencies: the runtime ones are in requirements/common.txt, the build ones in the venv.torch step.",
	}
}

// vllmBuilt reports whether the fork of the venv is built and usable: the
// package is installed, the extension is compiled and the checks of the
// gfx1030 wiki do not fail. A half-built fork is not done, so a re-run
// rebuilds it. A probe that could not run at all leaves the step done:
// the state of the venv is then unknown, not bad.
func (p planner) vllmBuilt(dir string) bool {
	if !p.has("vllm-*rdna*.dist-info") && !p.has("vllm-*.dist-info") {
		return false
	}
	so := isFile(filepath.Join(dir, "vllm", "_rocm_C.abi3.so")) || p.has("vllm/_rocm_C.abi3.so")
	return so && !p.venvFailed
}

// StepVenvVerify is the id of the check of the venv.
const StepVenvVerify = "venv.verify"

// venvVerify is the postcondition of the vllm.build step: the fork must
// register its ops, its extension must find every library it needs and
// rocBLAS must come from the wheel of the venv.
func (p planner) venvVerify() Step {
	// The library path is computed when the command runs: the plan is
	// written before the venv holds the wheels. The order is the one of
	// the gfx1030 wiki: the wheel libraries load before the system ones.
	sp := "SP=$(" + shq(p.python()) + " -c 'import sysconfig; print(sysconfig.get_paths()[\"purelib\"])')"
	ld := "export LD_LIBRARY_PATH=\"$SP/_rocm_sdk_libraries/lib:$SP/_rocm_sdk_core/lib" +
		":$SP/_rocm_sdk_core/lib/host-math/lib:$SP/_rocm_sdk_core/lib/rocm_sysdeps/lib" +
		":$SP/_rocm_sdk_core/lib/core/lib:$SP/torch/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}\""
	cmd := sp + "; " + ld + "; BERNIE_VENV_STRICT=1 " + shq(p.python()) + " -I - <<'PY'\n" + rocm.VenvProbeScript + "PY"
	return Step{
		ID: StepVenvVerify, Title: "Verify the vLLM build in the venv", Mode: "host",
		Done:     p.venvOK,
		Commands: []string{cmd},
		Why:      "The checks of the gfx1030 wiki: torch and vLLM import, the fork registers more than ten RDNA ops with an all_reduce, the extension finds every library and rocBLAS comes from the wheel of the venv.",
	}
}
