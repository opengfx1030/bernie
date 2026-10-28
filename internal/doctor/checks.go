package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/host"
	"github.com/opengfx1030/rdnactl/internal/rocm"
	"github.com/opengfx1030/rdnactl/internal/tuning"
)

// WikiURL is the gfx1030 community wiki used for doc links.
const WikiURL = "https://blivioniag.github.io/gfx1030-wiki/"

var (
	docOS       = WikiURL + "setup/installing-rocm.html#supported-operating-systems"
	docInstall  = WikiURL + "setup/installing-rocm.html"
	docGotchas  = WikiURL + "setup/installing-rocm.html#notes--gotchas"
	docROCmPin  = WikiURL + "setup/installing-rocm.html#multi-gpu-pin-rocm-720-or-7140"
	docGeneral  = WikiURL + "troubleshooting/general.html"
	docGroups   = WikiURL + "troubleshooting/vllm.html#failed-to-infer-device-type--amdsmi_status_not_init"
	docHardware = WikiURL + "setup/hardware.html"
	docHSA      = WikiURL + "setup/hsa-override.html"
	docP2P      = WikiURL + "tuning/p2p.html"
	docTopology = WikiURL + "tuning/p2p.html#host-topology"
	docHostVenv = WikiURL + "vllm/host-venv.html"
	docTunable  = WikiURL + "vllm/serve-scripts.html#tunableop"
	docMismatch = WikiURL + "troubleshooting/vllm.html#tunableop-rocblas-mismatch"
	docRecipes  = WikiURL + "vllm/recipes.html"
	docCompose  = WikiURL + "vllm/configuration.html#docker-compose-example-gptq--mtp--cuda-graphs"
	docRunning  = WikiURL + "vllm/running.html"
	docPower    = WikiURL + "tuning/power.html"
)

const gib = 1 << 30

// V620PowerAdviceW is the power cap advised for Radeon Pro V620.
const V620PowerAdviceW = 180

// evaluate runs every check on the gathered inputs.
func evaluate(in inputs) []Check {
	var out []Check
	for _, f := range []func(inputs) []Check{osChecks, gpuChecks, kernelChecks, rocmChecks, pythonChecks, containerChecks, resourceChecks, tuningChecks} {
		out = append(out, f(in)...)
	}
	return out
}

func check(cat, id string, st Status, title, detail string) Check {
	return Check{ID: cat + "." + id, Category: cat, Status: st, Title: title, Detail: detail}
}

func (c Check) with(fix, doc string) Check {
	c.Fix, c.Doc = fix, doc
	return c
}

// install returns the package-manager command, else a sentence.
func install(pm, feature, sentence string) string {
	if cmd := host.InstallCommand(pm, feature); cmd != "" {
		return cmd
	}
	return sentence
}

// ---- os ----

var knownDistros = map[string]bool{
	"fedora": true, "rhel": true, "centos": true, "rocky": true, "almalinux": true, "ol": true,
	"ubuntu": true, "debian": true, "linuxmint": true, "pop": true,
	"arch": true, "manjaro": true, "endeavouros": true, "cachyos": true,
	"opensuse-leap": true, "opensuse-tumbleweed": true, "sles": true,
}

func osChecks(in inputs) []Check {
	const cat = "os"
	h := in.host
	var out []Check
	switch {
	case h.OS.ID == "":
		out = append(out, check(cat, "distro", Warn, "Distribution is known", "no /etc/os-release: unknown distribution").with("", docOS))
	case knownDistros[h.OS.ID]:
		out = append(out, check(cat, "distro", OK, "Distribution is known", h.OS.Pretty).with("", docOS))
	default:
		out = append(out, check(cat, "distro", Warn, "Distribution is known",
			fmt.Sprintf("%s (id %q) is not a tested distribution", h.OS.Pretty, h.OS.ID)).with("", docOS))
	}
	if h.PkgManager != "" {
		out = append(out, check(cat, "pkgmanager", OK, "Package manager is known", h.PkgManager))
	} else {
		out = append(out, check(cat, "pkgmanager", Warn, "Package manager is known",
			"none of dnf, apt, pacman, zypper; fixes are given as sentences").with("", docInstall))
	}
	kv := rocm.ParseVersion(h.Kernel)
	switch {
	case h.Kernel == "":
		out = append(out, check(cat, "kernel", Skip, "Kernel version", "unknown"))
	case len(kv) > 0 && kv[0] < 6:
		out = append(out, check(cat, "kernel", Warn, "Kernel version", h.Kernel+": ROCm 7 expects kernel 6.x or newer").
			with("Upgrade to a kernel supported by your ROCm release.", docInstall))
	default:
		out = append(out, check(cat, "kernel", OK, "Kernel version", h.Kernel))
	}
	return out
}

// ---- gpu ----

// rdna2Cousins run gfx1030 code objects with HSA_OVERRIDE_GFX_VERSION=10.3.0.
var rdna2Cousins = map[string]bool{"gfx1031": true, "gfx1032": true, "gfx1033": true, "gfx1034": true, "gfx1035": true, "gfx1036": true}

// integrated lists APU (iGPU) targets; they shift HIP indices.
var integrated = map[string]bool{"gfx902": true, "gfx90c": true, "gfx1033": true, "gfx1035": true, "gfx1036": true, "gfx1103": true, "gfx1150": true, "gfx1151": true, "gfx1152": true}

// archKnown reports architectures ROCm supports (RDNA2-4, CDNA).
func archKnown(gfx string) bool {
	switch {
	case gfx == "gfx1030", rdna2Cousins[gfx]:
		return true
	case strings.HasPrefix(gfx, "gfx11"), strings.HasPrefix(gfx, "gfx12"):
		return true
	case gfx == "gfx908", gfx == "gfx90a", gfx == "gfx942", gfx == "gfx950":
		return true
	}
	return false
}

func gpuChecks(in inputs) []Check {
	const cat = "gpu"
	d := in.devices
	groupsFix := "sudo usermod -aG video,render $USER  # then log out and in"
	var out []Check
	switch {
	case !d.kfdExists:
		out = append(out, check(cat, "kfd", Fail, "/dev/kfd is readable and writable", "/dev/kfd is missing (amdgpu KFD not loaded)").
			with("sudo modprobe amdgpu", docGeneral))
	case !d.kfdRW:
		out = append(out, check(cat, "kfd", Fail, "/dev/kfd is readable and writable", "permission denied").with(groupsFix, docGeneral))
	default:
		out = append(out, check(cat, "kfd", OK, "/dev/kfd is readable and writable", ""))
	}
	switch {
	case len(d.render) == 0:
		out = append(out, check(cat, "render", Fail, "/dev/dri/renderD* exist", "no render nodes").with("sudo modprobe amdgpu", docGeneral))
	case !d.renderRW:
		out = append(out, check(cat, "render", Fail, "/dev/dri/renderD* exist", fmt.Sprintf("%d render nodes, not all accessible", len(d.render))).with(groupsFix, docGeneral))
	default:
		out = append(out, check(cat, "render", OK, "/dev/dri/renderD* exist", fmt.Sprintf("%d render nodes", len(d.render))))
	}
	u := in.host.User
	if u.Name == "" {
		out = append(out, check(cat, "groups", Skip, "User is in video and render", "no user information for this root"))
	} else {
		var missing []string
		for _, g := range []string{"video", "render"} {
			if !u.InGroup(g) {
				missing = append(missing, g)
			}
		}
		switch {
		case len(missing) == 0:
			out = append(out, check(cat, "groups", OK, "User is in video and render", u.Name))
		case d.kfdRW && d.renderRW:
			out = append(out, check(cat, "groups", Warn, "User is in video and render",
				fmt.Sprintf("%s is not in %s; devices are accessible now (world-writable), containers with --group-add still need the groups", u.Name, strings.Join(missing, ", "))).with(groupsFix, docGroups))
		default:
			out = append(out, check(cat, "groups", Fail, "User is in video and render",
				fmt.Sprintf("%s is not in %s", u.Name, strings.Join(missing, ", "))).with(groupsFix, docGroups))
		}
	}
	if in.gpuErr != nil || len(in.gpus) == 0 {
		detail := "no KFD GPU nodes"
		if in.gpuErr != nil {
			detail = in.gpuErr.Error()
		}
		out = append(out, check(cat, "count", Fail, "At least one GPU", detail).with("sudo modprobe amdgpu", docHardware))
		return out
	}
	out = append(out, check(cat, "count", OK, "At least one GPU", fmt.Sprintf("%d GPUs: %s", len(in.gpus), archSummary(in.gpus))).with("", docHardware))
	for _, gfx := range archOrder(in.gpus) {
		id := "arch." + gfx
		title := gfx + " is supported"
		switch {
		case rdna2Cousins[gfx]:
			out = append(out, check(cat, id, Warn, title, "RDNA2 part without its own ROCm code objects; it runs gfx1030 code").
				with("Set HSA_OVERRIDE_GFX_VERSION=10.3.0 in the serve environment (recipe env or --set env.HSA_OVERRIDE_GFX_VERSION=10.3.0).", docHSA))
		case archKnown(gfx):
			out = append(out, check(cat, id, OK, title, ""))
		default:
			out = append(out, check(cat, id, Warn, title, "unknown architecture for ROCm/vLLM").with("", docHardware))
		}
	}
	var igpu, dgpu []string
	for _, g := range in.gpus {
		if integrated[g.GFX] {
			igpu = append(igpu, fmt.Sprint(g.HIPIndex))
		} else {
			dgpu = append(dgpu, fmt.Sprint(g.HIPIndex))
		}
	}
	if len(igpu) > 0 && len(dgpu) > 0 {
		out = append(out, check(cat, "igpu", Warn, "No integrated GPU mixed with discrete GPUs",
			"integrated GPU at HIP index "+strings.Join(igpu, ",")+" shifts HIP indices").
			with("export HIP_VISIBLE_DEVICES="+strings.Join(dgpu, ","), docGotchas))
	}
	return out
}

func archSummary(gpus []gpu.GPU) string {
	n := map[string]int{}
	for _, g := range gpus {
		n[g.GFX]++
	}
	var parts []string
	for _, gfx := range archOrder(gpus) {
		parts = append(parts, fmt.Sprintf("%d× %s", n[gfx], gfx))
	}
	return strings.Join(parts, ", ")
}

// archOrder lists distinct gfx names in HIP order.
func archOrder(gpus []gpu.GPU) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range gpus {
		if !seen[g.GFX] {
			seen[g.GFX] = true
			out = append(out, g.GFX)
		}
	}
	return out
}

// ---- kernel ----

func kernelChecks(in inputs) []Check {
	const cat = "kernel"
	var out []Check
	if in.amdgpu {
		out = append(out, check(cat, "amdgpu", OK, "amdgpu is loaded", ""))
	} else {
		out = append(out, check(cat, "amdgpu", Fail, "amdgpu is loaded", "amdgpu module not found in /sys/module or /proc/modules").
			with("sudo modprobe amdgpu", docGotchas))
	}
	args := strings.Fields(in.host.Cmdline)
	has := func(a string) bool {
		for _, x := range args {
			if x == a {
				return true
			}
		}
		return false
	}
	iommuFix := host.KernelArgCommand(in.host.PkgManager, "iommu=pt")
	switch {
	case in.host.Cmdline == "":
		out = append(out, check(cat, "iommu", Skip, "IOMMU mode suits P2P", "no /proc/cmdline"))
	case has("iommu=pt"):
		out = append(out, check(cat, "iommu", OK, "IOMMU mode suits P2P", "iommu=pt"))
	case len(in.gpus) < 2:
		out = append(out, check(cat, "iommu", OK, "IOMMU mode suits P2P", "single GPU: P2P not used"))
	default:
		out = append(out, check(cat, "iommu", Warn, "IOMMU mode suits P2P",
			"iommu=pt is not set: GPU peer-to-peer copies go through IOMMU translation (do not disable the IOMMU instead)").with(iommuFix, docP2P))
	}
	if len(in.gpus) > 1 {
		var slow []string
		for _, g := range in.gpus {
			if g.Link.Gen > 0 && (g.Link.Gen <= 3 || g.Link.Width < 8) {
				slow = append(slow, fmt.Sprintf("GPU %d gen%d x%d", g.HIPIndex, g.Link.Gen, g.Link.Width))
			}
		}
		if len(slow) > 0 {
			out = append(out, check(cat, "pcie", Warn, "PCIe links are gen4 x8 or better", strings.Join(slow, "; ")).
				with("Move the GPUs to CPU root ports or a PCIe switch slot with more lanes.", docTopology))
		} else {
			out = append(out, check(cat, "pcie", OK, "PCIe links are gen4 x8 or better", linkSummary(in.gpus)))
		}
	}
	return out
}

func linkSummary(gpus []gpu.GPU) string {
	n := map[gpu.Link]int{}
	var order []gpu.Link
	for _, g := range gpus {
		if n[g.Link] == 0 {
			order = append(order, g.Link)
		}
		n[g.Link]++
	}
	var parts []string
	for _, l := range order {
		parts = append(parts, fmt.Sprintf("%d× gen%d x%d", n[l], l.Gen, l.Width))
	}
	return strings.Join(parts, ", ")
}

func amdgpuLoaded(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "sys/module/amdgpu")); err == nil {
		return true
	}
	b, _ := os.ReadFile(filepath.Join(root, "proc/modules"))
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "amdgpu ") {
			return true
		}
	}
	return false
}

// ---- rocm ----

func rocmChecks(in inputs) []Check {
	const cat = "rocm"
	pm := in.host.PkgManager
	r := in.rocm
	var out []Check
	alt := containerUsable(in.host) || in.site != "" && dirExists(filepath.Join(in.site, "_rocm_sdk_core"))
	switch len(r.Installs) {
	case 0:
		st, detail := Fail, "no ROCm under /opt/rocm*, /opt/rocm/core-* or /usr"
		if alt {
			st, detail = Warn, detail+"; container mode or the venv ROCm wheels still work"
		}
		out = append(out, check(cat, "installs", st, "ROCm is installed", detail).with(install(pm, host.PkgROCm, "Install ROCm: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"), docInstall))
	case 1:
		out = append(out, check(cat, "installs", OK, "ROCm is installed", installList(r.Installs)).with("", docInstall))
	default:
		out = append(out, check(cat, "installs", Warn, "ROCm is installed",
			fmt.Sprintf("%d installs: %s; tools and libraries may mix versions", len(r.Installs), installList(r.Installs))).
			with("Keep one ROCm userspace on PATH/LD_LIBRARY_PATH (host mode uses the venv wheels first).", docInstall))
	}
	if len(in.gpus) > 1 {
		var bad []string
		for _, i := range r.Installs {
			if deadlockRange(i.Version) {
				bad = append(bad, i.Root+" "+i.Version)
			}
		}
		if v := wheelROCm(in.venvRep.Torch); deadlockRange(v) {
			bad = append(bad, "venv torch ROCm "+v)
		}
		if len(bad) > 0 {
			out = append(out, check(cat, "version", Warn, "ROCm version is safe for multi-GPU",
				strings.Join(bad, "; ")+": ROCm 7.2.1-7.13 has the RCCL tensor-parallel deadlock").
				with("Use ROCm 7.2.0 or 7.14 (venv wheels or image).", docROCmPin))
		} else if len(r.Installs) > 0 {
			out = append(out, check(cat, "version", OK, "ROCm version is safe for multi-GPU", "outside 7.2.1-7.13").with("", docROCmPin))
		}
	}
	switch {
	case r.Tools["rocminfo"] == "":
		out = append(out, check(cat, "rocminfo", Skip, "rocminfo lists the GPUs", "rocminfo not found").with(install(pm, host.PkgROCmInfo, "Install rocminfo."), docInstall))
	case in.root != "/":
		out = append(out, check(cat, "rocminfo", Skip, "rocminfo lists the GPUs", "not run for a fixture root"))
	case len(r.Agents) == 0 && len(in.gpus) > 0:
		out = append(out, check(cat, "rocminfo", Fail, "rocminfo lists the GPUs", "rocminfo lists no GPU agents").with("sudo usermod -aG video,render $USER  # then log out and in", docGeneral))
	case len(r.Agents) < len(in.gpus):
		out = append(out, check(cat, "rocminfo", Warn, "rocminfo lists the GPUs", fmt.Sprintf("%d of %d GPUs (check ROCR_VISIBLE_DEVICES/HIP_VISIBLE_DEVICES)", len(r.Agents), len(in.gpus))).with("", docGeneral))
	default:
		out = append(out, check(cat, "rocminfo", OK, "rocminfo lists the GPUs", fmt.Sprintf("%d GPU agents", len(r.Agents))))
	}
	if p := r.Tools["hipcc"]; p != "" {
		d := p
		if r.HIPVersion != "" {
			d += " (HIP " + r.HIPVersion + ")"
		}
		out = append(out, check(cat, "hipcc", OK, "hipcc is installed", d))
	} else {
		out = append(out, check(cat, "hipcc", Warn, "hipcc is installed", "needed to build vLLM in host mode").with(install(pm, host.PkgHIPCC, "Install the HIP SDK (hipcc)."), docHostVenv))
	}
	smi := r.Tools["amd-smi"]
	if smi == "" {
		smi = r.Tools["rocm-smi"]
	}
	if smi != "" {
		out = append(out, check(cat, "smi", OK, "amd-smi or rocm-smi is installed", smi))
	} else {
		out = append(out, check(cat, "smi", Warn, "amd-smi or rocm-smi is installed", "GPU monitoring tools missing").with(install(pm, host.PkgSMI, "Install rocm-smi."), docInstall))
	}
	switch {
	case in.rocblas != "":
		out = append(out, check(cat, "rocblas", OK, "librocblas.so.5 is found", in.rocblas))
	case alt:
		out = append(out, check(cat, "rocblas", Warn, "librocblas.so.5 is found", "no system or venv rocBLAS; container mode still works").
			with(install(pm, host.PkgROCBLAS, "Install rocBLAS."), docTunable))
	default:
		out = append(out, check(cat, "rocblas", Fail, "librocblas.so.5 is found", "no system or venv rocBLAS").
			with(install(pm, host.PkgROCBLAS, "Install rocBLAS."), docTunable))
	}
	return out
}

func installList(in []rocm.Install) string {
	var s []string
	for _, i := range in {
		s = append(s, fmt.Sprintf("%s (%s, %s)", i.Root, i.Version, i.Kind))
	}
	return strings.Join(s, ", ")
}

// deadlockRange reports ROCm 7.2.1 <= v <= 7.13.x.
func deadlockRange(v string) bool {
	if v == "" {
		return false
	}
	return rocm.CompareVersions(v, "7.2.1") >= 0 && rocm.CompareVersions(v, "7.14") < 0
}

// wheelROCm extracts "7.14.0" from a torch version "2.12.0+rocm7.14.0".
func wheelROCm(torch string) string {
	_, v, ok := strings.Cut(torch, "+rocm")
	if !ok {
		return ""
	}
	return v
}

func containerUsable(h host.Info) bool {
	for _, e := range h.Engines {
		if e.Usable {
			return true
		}
	}
	return false
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ---- python ----

func pythonChecks(in inputs) []Check {
	const cat = "python"
	if in.venv == "" {
		return []Check{check(cat, "venv", Skip, "Python venv", "no venv: pass --venv DIR or set venv in the config").with("", docHostVenv)}
	}
	v := in.venvRep
	setupFix := "rdnactl setup apply --mode host"
	if v.Python == "" && v.Torch == "" {
		return []Check{check(cat, "venv", Fail, "Python venv", v.Err).with(setupFix, docHostVenv)}
	}
	var out []Check
	if strings.HasPrefix(v.Python, "3.12") {
		out = append(out, check(cat, "version", OK, "Python version", v.Python+" in "+in.venv))
	} else {
		out = append(out, check(cat, "version", Warn, "Python version", v.Python+": the wheels and images target 3.12").with("uv venv --python 3.12 "+in.venv, docHostVenv))
	}
	if v.Torch == "" {
		out = append(out, check(cat, "torch", Fail, "torch imports", v.Err).with("rdnactl setup apply --step venv.torch", docHostVenv))
		return out
	}
	out = append(out, check(cat, "torch", OK, "torch imports", v.Torch))
	if v.HIP == "" {
		out = append(out, check(cat, "torch_hip", Fail, "torch.version.hip is set", "torch "+v.Torch+" is not a ROCm build").with("rdnactl setup apply --step venv.torch", docGeneral))
	} else {
		out = append(out, check(cat, "torch_hip", OK, "torch.version.hip is set", "HIP "+v.HIP))
	}
	switch {
	case v.DeviceCount == 0:
		out = append(out, check(cat, "gpus", Fail, "torch sees the GPUs", "torch.cuda.device_count() = 0").with("", docGeneral))
	case v.DeviceCount != len(in.gpus):
		out = append(out, check(cat, "gpus", Warn, "torch sees the GPUs", fmt.Sprintf("torch sees %d, KFD has %d (HIP_VISIBLE_DEVICES set?)", v.DeviceCount, len(in.gpus))).with("", docGeneral))
	default:
		out = append(out, check(cat, "gpus", OK, "torch sees the GPUs", fmt.Sprintf("%d GPUs", v.DeviceCount)))
	}
	if in.site != "" && dirExists(filepath.Join(in.site, "_rocm_sdk_core")) {
		var missing []string
		for _, gfx := range archOrder(in.gpus) {
			if m, _ := filepath.Glob(filepath.Join(in.site, "rocm_sdk_device_"+gfx+"-*.dist-info")); len(m) == 0 {
				missing = append(missing, gfx)
			}
		}
		if len(missing) > 0 {
			out = append(out, check(cat, "device_wheels", Warn, "ROCm device wheels match the GPUs",
				"missing rocm-sdk-device wheels for "+strings.Join(missing, ", ")+" (hipErrorInvalidImage at kernel launch)").
				with("rdnactl setup apply --step venv.torch", docHostVenv))
		} else {
			out = append(out, check(cat, "device_wheels", OK, "ROCm device wheels match the GPUs", strings.Join(archOrder(in.gpus), ", ")))
		}
	}
	if tv := tritonVersion(in.site); tv != "" {
		if rocm.CompareVersions(tv, "3.9") >= 0 && hasRDNA2(in.gpus) {
			out = append(out, check(cat, "triton", Fail, "Triton supports RDNA2", "triton "+tv+" removed RDNA1/RDNA2 support").
				with(in.venv+"/bin/pip install triton==3.7.1", docHostVenv))
		} else {
			out = append(out, check(cat, "triton", OK, "Triton supports RDNA2", "triton "+tv))
		}
	}
	if v.VLLM == "" {
		out = append(out, check(cat, "vllm", Fail, "vllm imports", v.Err).with("rdnactl setup apply --step vllm.build", docHostVenv))
		return out
	}
	out = append(out, check(cat, "vllm", OK, "vllm imports", v.VLLM))
	if v.RDNAOps > 10 {
		out = append(out, check(cat, "rdna_ops", OK, "vllm-rdna ops are registered", fmt.Sprintf("%d _rocm_C rdna ops", v.RDNAOps)))
	} else {
		out = append(out, check(cat, "rdna_ops", Warn, "vllm-rdna ops are registered",
			fmt.Sprintf("%d _rocm_C rdna ops (need > 10 for vllm-rdna recipes)", v.RDNAOps)).
			with("rdnactl setup apply --step vllm.build", docHostVenv))
	}
	return out
}

func tritonVersion(site string) string {
	if site == "" {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(site, "triton-*.dist-info"))
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m[len(m)-1]), "triton-"), ".dist-info")
	return v
}

func hasRDNA2(gpus []gpu.GPU) bool {
	for _, g := range gpus {
		if strings.HasPrefix(g.GFX, "gfx103") || strings.HasPrefix(g.GFX, "gfx101") {
			return true
		}
	}
	return false
}

// ---- container ----

func containerChecks(in inputs) []Check {
	const cat = "container"
	pm := in.host.PkgManager
	if in.root != "/" {
		return []Check{check(cat, "engine", Skip, "Docker or Podman is installed", "not probed for a fixture root")}
	}
	e, ok := host.Preferred(in.host.Engines, "")
	if !ok {
		return []Check{check(cat, "engine", Skip, "Docker or Podman is installed", "no container engine: container mode is unavailable, host mode works").
			with(install(pm, host.PkgEngine, "Install Docker or Podman."), docRecipes)}
	}
	var out []Check
	out = append(out, check(cat, "engine", OK, "Docker or Podman is installed", strings.TrimSpace(e.Name+" "+e.Version)+" ("+e.Path+")"))
	if e.Usable {
		out = append(out, check(cat, "daemon", OK, e.Name+" info works for this user", ""))
	} else {
		fix := "sudo systemctl enable --now docker && sudo usermod -aG docker $USER"
		if e.Name == "podman" {
			fix = "systemctl --user enable --now podman.socket"
		}
		out = append(out, check(cat, "daemon", Warn, e.Name+" info works for this user", "`"+e.Name+" info` failed: container mode is unavailable").with(fix, docRecipes))
	}
	if e.Compose != "" {
		out = append(out, check(cat, "compose", OK, "Compose is installed", e.Compose))
	} else {
		feat := host.PkgDockerCompose
		if e.Name == "podman" {
			feat = host.PkgPodmanCompose
		}
		out = append(out, check(cat, "compose", Warn, "Compose is installed", "rdnactl compose files need docker compose, podman compose or podman-compose").with(install(pm, feat, "Install compose."), docCompose))
	}
	mode := "rootful"
	if e.Rootless {
		mode = "rootless (rdnactl uses --group-add keep-groups)"
	}
	out = append(out, check(cat, "rootless", OK, "Rootless state", mode))
	switch in.host.SELinux {
	case "":
		out = append(out, check(cat, "selinux", Skip, "SELinux mode", "no SELinux"))
	case "enforcing":
		out = append(out, check(cat, "selinux", OK, "SELinux mode", "enforcing: rdnactl runs containers with --security-opt label=disable").with("", docCompose))
	default:
		out = append(out, check(cat, "selinux", OK, "SELinux mode", in.host.SELinux))
	}
	return out
}

// ---- resources ----

func resourceChecks(in inputs) []Check {
	const cat = "resources"
	var out []Check
	ram := in.host.RAMBytes
	switch {
	case ram == 0:
		out = append(out, check(cat, "ram", Skip, "Host RAM", "unknown"))
	case ram < 32*gib:
		out = append(out, check(cat, "ram", Warn, "Host RAM", fmt.Sprintf("%.0f GiB: below 32 GiB, model loading may swap", float64(ram)/gib)))
	default:
		out = append(out, check(cat, "ram", OK, "Host RAM", fmt.Sprintf("%.0f GiB", float64(ram)/gib)))
	}
	switch free := in.hfFree; {
	case free == 0:
		out = append(out, check(cat, "disk", Skip, "Free disk in the Hugging Face cache", in.hfCache+": unknown"))
	case free < 20*gib:
		out = append(out, check(cat, "disk", Fail, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free", in.hfCache, float64(free)/gib)).
			with("Free space or set hf_home in the config (rdnactl config set hf_home DIR).", ""))
	case free < 100*gib:
		out = append(out, check(cat, "disk", Warn, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free (large models need 100+ GiB)", in.hfCache, float64(free)/gib)).
			with("Free space or set hf_home in the config (rdnactl config set hf_home DIR).", ""))
	default:
		out = append(out, check(cat, "disk", OK, "Free disk in the Hugging Face cache", fmt.Sprintf("%s: %.0f GiB free", in.hfCache, float64(free)/gib)))
	}
	switch shm := in.host.ShmBytes; {
	case shm == 0:
		out = append(out, check(cat, "shm", Skip, "/dev/shm size", "unknown"))
	case shm < 8*gib:
		out = append(out, check(cat, "shm", Warn, "/dev/shm size", fmt.Sprintf("%.1f GiB: tensor parallel in host mode needs 8+ GiB", float64(shm)/gib)).
			with("sudo mount -o remount,size=16G /dev/shm", docRunning))
	default:
		out = append(out, check(cat, "shm", OK, "/dev/shm size", fmt.Sprintf("%.0f GiB", float64(shm)/gib)))
	}
	return out
}

// ---- tuning ----

func tuningChecks(in inputs) []Check {
	const cat = "tuning"
	var out []Check
	switch {
	case in.profErr != nil:
		out = append(out, check(cat, "tunableop", Fail, "TunableOp profile matches the loaded rocBLAS", "profile registry: "+in.profErr.Error()))
	case in.rocblas == "":
		out = append(out, check(cat, "tunableop", Skip, "TunableOp profile matches the loaded rocBLAS", "no librocblas.so.5 found").with("", docTunable))
	case in.libSHA == "":
		out = append(out, check(cat, "tunableop", Skip, "TunableOp profile matches the loaded rocBLAS", "cannot read "+in.rocblas).with("", docTunable))
	case in.match != nil:
		out = append(out, check(cat, "tunableop", OK, "TunableOp profile matches the loaded rocBLAS",
			fmt.Sprintf("%s (rocBLAS %s, sha %s) for %s", in.match.Name, in.match.ROCBLAS, in.libSHA[:tuning.SHAPrefix], in.rocblas)).with("", docTunable))
	default:
		out = append(out, check(cat, "tunableop", Warn, "TunableOp profile matches the loaded rocBLAS",
			fmt.Sprintf("no registered profile for %s (sha %s, %d profiles known); rows go to %s", in.rocblas, in.libSHA[:tuning.SHAPrefix], len(in.profiles), tuning.PerUserFile())).
			with("rdnactl tune tunableop env --tuning  # tune rows once for this rocBLAS build", docMismatch))
	}
	var high []string
	var caps []string
	var idx []string
	for _, g := range in.gpus {
		caps = append(caps, fmt.Sprintf("%d:%.0fW", g.HIPIndex, g.PowerCapW))
		if isV620(g) && g.PowerCapW > V620PowerAdviceW {
			high = append(high, fmt.Sprintf("GPU %d %.0f W", g.HIPIndex, g.PowerCapW))
			idx = append(idx, fmt.Sprint(g.HIPIndex))
		}
	}
	switch {
	case len(in.gpus) == 0:
		out = append(out, check(cat, "power", Skip, "Power cap per GPU", "no GPUs"))
	case len(high) > 0:
		out = append(out, check(cat, "power", Warn, "Power cap per GPU",
			fmt.Sprintf("V620 above %d W: %s", V620PowerAdviceW, strings.Join(high, ", "))).
			with(fmt.Sprintf("rdnactl tune power set %d --gpu %s", V620PowerAdviceW, strings.Join(idx, " --gpu ")), docPower))
	default:
		out = append(out, check(cat, "power", OK, "Power cap per GPU", strings.Join(caps, " ")).with("", docPower))
	}
	return out
}

func isV620(g gpu.GPU) bool { return g.DeviceID == "0x73a1" || g.DeviceID == "0x73ae" }
