package topo

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/support/execx"
)

// iommuItem reads the IOMMU mode from the kernel command line.
func iommuItem(o P2POptions) CheckItem {
	const title = "IOMMU mode"
	switch {
	case cmdlineHas(o.Cmdline, "iommu=pt"):
		return p2pItem("iommu", title, P2POK, "iommu=pt: peer addresses are passed through unchanged").with("", P2PDoc)
	case cmdlineHas(o.Cmdline, "iommu=off") && !cmdlineHas(o.Cmdline, "amd_iommu=on"):
		return p2pItem("iommu", title, P2POK, "iommu=off: no IOMMU translation of peer addresses").with("", P2PDoc)
	case cmdlineHas(o.Cmdline, "iommu=on") || cmdlineHas(o.Cmdline, "amd_iommu=on"):
		return p2pItem("iommu", title, P2PWarn,
			"the IOMMU translates peer addresses, so a peer copy can fall back to the host").with(
			host.KernelArgCommand(o.PkgManager, "iommu=pt"), P2PDoc)
	default:
		return p2pItem("iommu", title, P2PWarn,
			"no iommu=pt on the kernel command line: the IOMMU may translate peer addresses").with(
			host.KernelArgCommand(o.PkgManager, "iommu=pt"), P2PDoc)
	}
}

// acsItem reports the PCIe ACS state. ACS redirection blocks peer copies
// across a switch unless the kernel turns it off.
func acsItem(o P2POptions, t Topology) CheckItem {
	const title = "PCIe ACS"
	hasSwitch := false
	for _, n := range t.Nodes {
		if n.Switch != "" {
			hasSwitch = true
		}
	}
	if !hasSwitch {
		return p2pItem("acs", title, P2POK,
			"the GPUs hang directly off the root ports: no PCIe switch can block a peer copy").with("", TopologyDoc)
	}
	switch {
	case cmdlinePrefix(o.Cmdline, "pcie_acs_override="):
		return p2pItem("acs", title, P2POK,
			"pcie_acs_override is set: the kernel turns ACS redirection off on the PCIe switches").with("", TopologyDoc)
	case cmdlinePrefix(o.Cmdline, "disable_acs_redir="):
		return p2pItem("acs", title, P2POK,
			"disable_acs_redir is set: ACS redirection is off for that bridge").with("", TopologyDoc)
	default:
		return p2pItem("acs", title, P2PWarn,
			"no ACS override on the kernel command line: PCIe ACS can block a peer copy across a switch").with(
			host.KernelArgCommand(o.PkgManager, "pcie_acs_override=downstream,multifunction"), P2PDoc)
	}
}

// linkSpeedItem compares the negotiated PCIe link of every GPU with the
// maximum the slot offers.
func linkSpeedItem(root string, gpus []gpu.GPU) CheckItem {
	const title = "PCIe link speed"
	type link struct {
		g       gpu.GPU
		maxGen  int
		maxWide int
	}
	var links []link
	for _, g := range gpus {
		if g.Link.Gen == 0 && g.Link.Width == 0 {
			continue
		}
		gen, width := maxLink(root, g)
		links = append(links, link{g: g, maxGen: gen, maxWide: width})
	}
	if len(links) == 0 {
		return p2pItem("link_speed", title, P2PSkip, "no PCIe link data").with("", TopologyDoc)
	}
	var slow, degraded []string
	for _, l := range links {
		g := l.g
		cur := fmt.Sprintf("gen%d x%d", g.Link.Gen, g.Link.Width)
		slot := ""
		if l.maxGen > 0 {
			slot = fmt.Sprintf(", slot gen%d x%d", l.maxGen, l.maxWide)
		}
		if g.Link.Gen > 0 && (g.Link.Gen < 4 || g.Link.Width < 8) {
			slow = append(slow, fmt.Sprintf("GPU %d (%s%s)", g.HIPIndex, cur, slot))
		}
		if l.maxGen > 0 && (g.Link.Gen < l.maxGen || g.Link.Width < l.maxWide) {
			degraded = append(degraded, fmt.Sprintf("GPU %d runs at %s, the slot offers gen%d x%d",
				g.HIPIndex, cur, l.maxGen, l.maxWide))
		}
	}
	switch {
	case len(slow) > 0:
		return p2pItem("link_speed", title, P2PWarn,
			fmt.Sprintf("%s: below the gen4 x8 target", strings.Join(slow, "; "))).with(
			"Move the GPUs to slots with PCIe 4 lanes, or change the slot's PCIe generation.", TopologyDoc)
	case len(degraded) > 0:
		return p2pItem("link_speed", title, P2PWarn, strings.Join(degraded, "; ")).with(
			"Check the slot's PCIe generation and lane width in the firmware.", TopologyDoc)
	default:
		return p2pItem("link_speed", title, P2POK,
			fmt.Sprintf("all %d GPUs run at the maximum of their slot (%s)", len(links), linkText(links[0].g.Link))).with("", TopologyDoc)
	}
}

// maxLink reads the maximum PCIe generation and width of the GPU's slot.
func maxLink(root string, g gpu.GPU) (gen, width int) {
	if g.SysfsPath == "" {
		return 0, 0
	}
	dir := filepath.Join(root, g.SysfsPath)
	gen = gpu.LinkGen(readString(filepath.Join(dir, "max_link_speed")))
	width = readIntFile(filepath.Join(dir, "max_link_width"))
	return gen, width
}

// linkClassItem reports the RCCL link classes of the GPU pairs and the
// switch group a tensor-parallel group should use.
func linkClassItem(t Topology) CheckItem {
	const title = "PCIe switch topology"
	if len(t.Pairs) == 0 {
		return p2pItem("link_classes", title, P2PSkip, "no GPU pairs").with("", TopologyDoc)
	}
	counts := map[string]int{}
	for _, p := range t.Pairs {
		counts[p.Class]++
	}
	classes := []string{ClassPIX, ClassPXB, ClassPHB, ClassSYS}
	var parts []string
	for _, c := range classes {
		if counts[c] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", c, counts[c]))
		}
	}
	slow := counts[ClassPHB] + counts[ClassSYS]
	detail := fmt.Sprintf("%s of %d pairs; %s", strings.Join(parts, ", "), len(t.Pairs), groupText(t))
	if slow > 0 {
		return p2pItem("link_classes", title, P2PWarn,
			detail+": a pair across a host bridge or between CPUs is much slower").with(
			"Move the GPUs of a tensor-parallel group under one PCIe switch.", TopologyDoc)
	}
	return p2pItem("link_classes", title, P2POK, detail).with("", TopologyDoc)
}

// groupText names the largest group of GPUs under one PCIe switch.
func groupText(t Topology) string {
	groups := map[string][]int{}
	for _, n := range t.Nodes {
		if n.Switch == "" {
			continue
		}
		groups[n.Switch] = append(groups[n.Switch], n.HIPIndex)
	}
	best, size := "", 0
	for sw, members := range groups {
		if len(members) > size || (len(members) == size && sw < best) {
			best, size = sw, len(members)
		}
	}
	if best == "" {
		return "no PCIe switch holds more than one GPU"
	}
	sort.Ints(groups[best])
	return fmt.Sprintf("GPUs %s share PCIe switch %s", join(groups[best]), best)
}

// bandwidthToolItem reports the optional rocm-bandwidth-test.
func bandwidthToolItem(root string) CheckItem {
	const title = "rocm-bandwidth-test"
	path := execx.Which(BandwidthRBT)
	if path == "" {
		matches, _ := filepath.Glob(filepath.Join(root, "opt/rocm*/bin/"+BandwidthRBT))
		if len(matches) > 0 {
			path = matches[0]
		}
	}
	if path == "" {
		return p2pItem("bandwidth_test", title, P2PSkip,
			"not installed; the test is optional (bernie topo --bandwidth --venv DIR measures with the torch peer-copy probe)").with("", TopologyDoc)
	}
	return p2pItem("bandwidth_test", title, P2POK,
		"installed at "+path+"; bernie topo --bandwidth measures GB/s for every pair").with("", TopologyDoc)
}

// recipeEnvItem shows the communication environment of the chosen recipe.
func recipeEnvItem(r RecipeEnv) CheckItem {
	const title = "Recipe NCCL/RCCL environment"
	if r.ID == "" {
		return p2pItem("recipe_env", title, P2PSkip,
			"no recipe selected (bernie topo --check --recipe ID shows its environment)").with("", P2PDoc)
	}
	keys := make([]string, 0, len(r.Env))
	for k := range r.Env {
		if commEnv[k] || strings.HasPrefix(k, "NCCL_") || strings.HasPrefix(k, "RCCL_") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return p2pItem("recipe_env", title, P2POK,
			"recipe "+r.ID+" sets no NCCL, RCCL or RDNA all-reduce variable (informational)").with("", P2PDoc)
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+r.Env[k])
	}
	return p2pItem("recipe_env", title, P2POK,
		fmt.Sprintf("recipe %s sets %s (informational)", r.ID, strings.Join(parts, ", "))).with("", P2PDoc)
}
