package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/machine/gpu"
	"github.com/opengfx1030/rdnactl/internal/machine/host"
	"github.com/opengfx1030/rdnactl/internal/machine/rocm"
)

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
