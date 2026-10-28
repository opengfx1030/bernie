// Package setup builds the ordered install plan of spec §6 and runs its
// non-root steps. Every step carries a postcondition check (Done) so a
// re-run only does what is missing.
package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opengfx1030/rdnactl/internal/execx"
	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/host"
	"github.com/opengfx1030/rdnactl/internal/paths"
	"github.com/opengfx1030/rdnactl/internal/rocm"
)

// Defaults for the host and container stacks.
const (
	DefaultImage = "docker.io/blivioniag/vllm-rdna:v0.31.0-extras"
	// WheelIndex is AMD's multi-arch ROCm wheel index (TheRock).
	WheelIndex     = "https://repo.amd.com/rocm/whl-multi-arch/"
	ROCmWheel      = "7.14.0"
	TorchPin       = "torch==2.12.0+rocm7.14.0"
	TorchVisionPin = "torchvision==0.27.0+rocm7.14.0"
	TorchAudioPin  = "torchaudio==2.11.0+rocm7.14.0"
	// TritonPin keeps RDNA2 support (Triton 3.9 removed RDNA1/RDNA2).
	TritonPin       = "triton==3.7.1"
	ForkRepo        = "https://github.com/opengfx1030/vllm-rdna.git"
	ForkBranch      = "rdna_extras"
	CausalConv1DRef = "git+https://github.com/Dao-AILab/causal-conv1d.git@4f6ae4e26ae5fe8af9372f8d312ab25cc4595223"
	FlashAttnRepo   = "https://github.com/Dao-AILab/flash-attention.git"
)

// Step is one install step.
type Step struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Mode     string   `json:"mode"` // host | container | both
	Root     bool     `json:"root"`
	Done     bool     `json:"done"`
	Commands []string `json:"commands"`
	Why      string   `json:"why"`
}

// Options select the mode ("host", "container", "" = both), the venv, the
// image, an optional model to download, the source dir for git checkouts
// (default <data>/src) and the Hugging Face cache root (default $HF_HOME,
// else ~/.cache/huggingface). GPUArchs are the host gfx targets (from
// gpu.List), used for device wheels and build targets. Without GPUs the plan
// has no device wheels and no build target.
type Options struct {
	Mode, Venv, Image, ModelRepo string
	SrcDir, HFHome               string
	GPUArchs                     []string
}

// DefaultVenv is the venv used when none is configured.
func DefaultVenv() string { return filepath.Join(paths.Data(), "venv-rocm"+ROCmWheel) }

// Plan returns the ordered steps for the mode, with Done evaluated on this
// host.
func Plan(ctx context.Context, h host.Info, r rocm.Report, o Options) []Step {
	if o.Venv == "" {
		o.Venv = DefaultVenv()
	}
	if o.Image == "" {
		o.Image = DefaultImage
	}
	if o.SrcDir == "" {
		o.SrcDir = filepath.Join(paths.Data(), "src")
	}
	o.GPUArchs = uniq(o.GPUArchs)
	pl := planner{ctx: ctx, h: h, r: r, o: o, site: sitePackages(o.Venv), targets: gpu.CodeTargets(o.GPUArchs)}
	all := []Step{
		pl.groups(), pl.rocmRuntime(), pl.containerEngine(),
		pl.venvCreate(), pl.venvTorch(), pl.causalConv1D(), pl.flashAttn(), pl.vllmBuild(),
		pl.imagePull(), pl.modelDownload(),
	}
	var out []Step
	for _, s := range all {
		if s.ID == "" {
			continue
		}
		if o.Mode == "" || s.Mode == "both" || s.Mode == o.Mode {
			out = append(out, s)
		}
	}
	return out
}

type planner struct {
	ctx  context.Context
	h    host.Info
	r    rocm.Report
	o    Options
	site string
	// targets are the code-object targets of the GPUs (an HSA override
	// target replaces the real one): wheels and build targets use them.
	targets []string
}

// which resolves a tool on PATH; tests replace it.
var which = execx.Which

func (p planner) groups() Step {
	return Step{
		ID: "groups", Title: "Add the user to the video and render groups", Mode: "both", Root: true,
		Done:     p.h.User.InGroup("video") && p.h.User.InGroup("render"),
		Commands: []string{"sudo usermod -aG video,render " + userName(p.h)},
		Why:      "/dev/kfd and /dev/dri/renderD* belong to these groups. Log out and in afterwards.",
	}
}

func userName(h host.Info) string {
	if h.User.Name != "" {
		return h.User.Name
	}
	return "$USER"
}

func (p planner) rocmRuntime() Step {
	s := Step{
		ID: "rocm.runtime", Title: "Install the ROCm runtime and HIP SDK", Mode: "host", Root: true,
		Done: len(p.r.Installs) > 0 && p.r.Tools["hipcc"] != "",
		Why:  "Host mode builds the vLLM fork with hipcc and the ROCm headers; the runtime libraries come from the venv wheels.",
	}
	switch p.h.PkgManager {
	case "dnf":
		s.Commands = []string{
			"sudo rpm --import https://repo.radeon.com/rocm/rocm.gpg.key",
			`printf '[rocm]\nname=ROCm 7.2\nbaseurl=https://repo.radeon.com/rocm/el10/7.2/main\nenabled=1\ngpgcheck=1\npriority=50\ngpgkey=https://repo.radeon.com/rocm/rocm.gpg.key\n' | sudo tee /etc/yum.repos.d/rocm.repo`,
			host.InstallCommand("dnf", host.PkgROCm),
		}
	case "apt":
		s.Commands = []string{
			"sudo mkdir -p -m 0755 /etc/apt/keyrings && wget -qO- https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg >/dev/null",
			`echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/7.2 $(. /etc/os-release && echo $VERSION_CODENAME) main" | sudo tee /etc/apt/sources.list.d/rocm.list`,
			"sudo apt update && " + host.InstallCommand("apt", host.PkgROCm),
		}
	case "pacman", "zypper":
		s.Commands = []string{host.InstallCommand(p.h.PkgManager, host.PkgROCm)}
	default:
		s.Commands = []string{"# Install ROCm: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"}
	}
	return s
}

func (p planner) containerEngine() Step {
	e, ok := host.Preferred(p.h.Engines, "")
	s := Step{
		ID: "container.engine", Title: "Install Docker or Podman with compose", Mode: "container", Root: true,
		Done: ok && e.Usable && e.Compose != "",
		Why:  "Container mode runs the vLLM image with Docker or Podman.",
	}
	switch {
	case ok && e.Name == "docker" && !e.Usable:
		s.Commands = []string{"sudo systemctl enable --now docker", "sudo usermod -aG docker " + userName(p.h)}
	case ok && e.Name == "podman" && !e.Usable:
		s.Commands = []string{"systemctl --user enable --now podman.socket"}
	case ok && e.Compose == "":
		feat := host.PkgDockerCompose
		if e.Name == "podman" {
			feat = host.PkgPodmanCompose
		}
		s.Commands = []string{orSentence(host.InstallCommand(p.h.PkgManager, feat), "# Install compose for "+e.Name)}
	default:
		cmd := orSentence(host.InstallCommand(p.h.PkgManager, host.PkgEngine), "# Install Docker or Podman: https://docs.docker.com/engine/install/")
		s.Commands = []string{cmd}
		if strings.Contains(cmd, "docker") && !strings.HasPrefix(cmd, "#") {
			s.Commands = append(s.Commands, "sudo systemctl enable --now docker", "sudo usermod -aG docker "+userName(p.h))
		}
	}
	return s
}

func orSentence(cmd, sentence string) string {
	if cmd != "" {
		return cmd
	}
	return sentence
}

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
func (p planner) tritonDropped() (gpu.Arch, bool) {
	for _, gfx := range p.o.GPUArchs {
		if a, ok := gpu.LookupArch(gfx); ok && a.TritonBelow != "" {
			return a, true
		}
	}
	return gpu.Arch{}, false
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
		if t, ok := gpu.LookupArch(a); ok && t.FlashAttn {
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

func (p planner) imagePull() Step {
	eng := "docker"
	if e, ok := host.Preferred(p.h.Engines, ""); ok {
		eng = e.Name
	}
	done := false
	if which(eng) != "" {
		res, err := execx.Run(p.ctx, 15*time.Second, eng, "image", "inspect", "--format", "{{.Id}}", p.o.Image)
		done = err == nil && res.OK()
	}
	return Step{
		ID: "image.pull", Title: "Pull the vLLM image", Mode: "container",
		Done:     done,
		Commands: []string{eng + " pull " + p.o.Image},
		Why:      "Container mode runs " + p.o.Image + ". Tags are refreshed in place: pull again to update.",
	}
}

func (p planner) modelDownload() Step {
	if p.o.ModelRepo == "" {
		return Step{}
	}
	hf := "hf"
	if venvHF := filepath.Join(p.o.Venv, "bin", "hf"); which("hf") == "" && isFile(venvHF) {
		hf = shq(venvHF)
	}
	return Step{
		ID: "model.download", Title: "Download " + p.o.ModelRepo, Mode: "both",
		Done:     modelCached(hfHub(p.o.HFHome), p.o.ModelRepo),
		Commands: []string{hf + " download " + shq(p.o.ModelRepo)},
		Why:      "The model goes into the Hugging Face cache (" + hfHub(p.o.HFHome) + "), shared by host and container mode.",
	}
}

// hfHub is the hub directory of the Hugging Face cache.
func hfHub(override string) string {
	if override == "" {
		override = os.Getenv("HF_HOME")
	}
	if override == "" {
		home, _ := os.UserHomeDir()
		override = filepath.Join(home, ".cache", "huggingface")
	}
	return filepath.Join(override, "hub")
}

// modelCached reports a non-empty snapshot of repo in the hub cache.
func modelCached(hub, repo string) bool {
	if filepath.IsAbs(repo) {
		return isDir(repo)
	}
	dir := filepath.Join(hub, "models--"+strings.ReplaceAll(repo, "/", "--"), "snapshots")
	snaps, _ := os.ReadDir(dir)
	for _, s := range snaps {
		files, _ := os.ReadDir(filepath.Join(dir, s.Name()))
		if len(files) > 0 {
			return true
		}
	}
	return false
}

// has reports a glob match in the venv site-packages.
func (p planner) has(pattern string) bool {
	if p.site == "" {
		return false
	}
	m, _ := filepath.Glob(filepath.Join(p.site, pattern))
	return len(m) > 0
}

func sitePackages(venv string) string {
	if sp := rocm.SitePackages(venv); len(sp) > 0 {
		return sp[0]
	}
	return ""
}

// Apply runs, in order, the steps that are not done and do not need root,
// streaming their output to log. Root steps are printed, not run. only
// limits the run to the named step ids. It stops at the first failure.
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error {
	sel := map[string]bool{}
	for _, id := range only {
		found := false
		for _, s := range steps {
			if s.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown step %q", id)
		}
		sel[id] = true
	}
	var rootSteps []Step
	for _, s := range steps {
		if len(sel) > 0 && !sel[s.ID] {
			continue
		}
		switch {
		case s.Done:
			fmt.Fprintf(log, "== %s: done\n", s.ID)
		case s.Root:
			fmt.Fprintf(log, "== %s: needs root, not run\n", s.ID)
			rootSteps = append(rootSteps, s)
		default:
			fmt.Fprintf(log, "== %s: %s\n", s.ID, s.Title)
			for _, c := range s.Commands {
				if strings.HasPrefix(strings.TrimSpace(c), "#") {
					fmt.Fprintf(log, "%s\n", c)
					continue
				}
				fmt.Fprintf(log, "$ %s\n", c)
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c)
				cmd.Stdout, cmd.Stderr = log, log
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("step %s: %q: %w", s.ID, c, err)
				}
			}
		}
	}
	if len(rootSteps) > 0 {
		fmt.Fprintln(log, "\nRun these root steps yourself, then run setup apply again:")
		for _, s := range rootSteps {
			fmt.Fprintf(log, "# %s: %s\n", s.ID, s.Title)
			for _, c := range s.Commands {
				fmt.Fprintf(log, "%s\n", c)
			}
		}
	}
	return nil
}

// shq quotes s for /bin/sh when needed.
func shq(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:+@%,", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
