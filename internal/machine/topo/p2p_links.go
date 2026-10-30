package topo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// kfdLinksItem reports the KFD peer links of every GPU pair and the link
// counts of every node.
func kfdLinksItem(root string, gpus []gpu.GPU, t Topology) CheckItem {
	const title = "KFD peer links"
	if len(gpus) < 2 {
		return p2pItem("kfd_links", title, P2PSkip, "one GPU: no peer link is needed")
	}
	linked := 0
	for _, p := range t.Pairs {
		if p.P2P {
			linked++
		}
	}
	p2p, io := kfdLinkCounts(root, gpus)
	detail := fmt.Sprintf("%d of %d GPU pairs have a KFD P2P link; %s",
		linked, len(t.Pairs), linkCountText(p2p, io))
	switch linkStatus(linked, len(t.Pairs), missingIOLink(io)) {
	case P2POK:
		return p2pItem("kfd_links", title, P2POK, detail).with("", P2PDoc)
	case P2PFail:
		return p2pItem("kfd_links", title, P2PFail,
			detail+": no pair can copy peer to peer").with("Check the amdgpu module and the KFD topology.", P2PDoc)
	default:
		return p2pItem("kfd_links", title, P2PWarn,
			detail+": the other pairs stage their copies through the host").with("", P2PDoc)
	}
}

// linkStatus is the status of the peer links: every pair linked is ok, no
// linked pair at all is a fail, a missing pair or io_link is a warning.
func linkStatus(linked, pairs int, missingIO bool) P2PStatus {
	switch {
	case linked == pairs && !missingIO:
		return P2POK
	case linked == 0:
		return P2PFail
	default:
		return P2PWarn
	}
}

// kfdLinkCounts returns the p2p_links and io_links count of every GPU node.
func kfdLinkCounts(root string, gpus []gpu.GPU) (p2p, io map[int]int) {
	p2p, io = map[int]int{}, map[int]int{}
	for _, g := range gpus {
		props, err := gpu.ReadProperties(filepath.Join(root, gpu.KFDNodesDir, strconv.Itoa(g.KFDNode), "properties"))
		if err != nil {
			continue
		}
		p2p[g.HIPIndex] = int(props["p2p_links_count"])
		io[g.HIPIndex] = int(props["io_links_count"])
	}
	return p2p, io
}

// linkCountText describes the per-GPU link counts and names the outliers.
func linkCountText(p2p, io map[int]int) string {
	if len(p2p) == 0 {
		return "the KFD nodes do not report link counts"
	}
	p2pMin, p2pMax := countRange(p2p)
	ioMin, ioMax := countRange(io)
	text := fmt.Sprintf("every GPU has %d p2p_links and %d io_link", p2pMax, ioMax)
	if p2pMin != p2pMax || ioMin != ioMax {
		text = fmt.Sprintf("p2p_links per GPU: %d-%d, io_links per GPU: %d-%d", p2pMin, p2pMax, ioMin, ioMax)
	}
	var out []string
	for idx, n := range p2p {
		if n == 0 {
			out = append(out, fmt.Sprintf("GPU %d has no p2p_link", idx))
		}
	}
	for idx, n := range io {
		if n == 0 {
			out = append(out, fmt.Sprintf("GPU %d has no io_link", idx))
		}
	}
	if len(out) > 0 {
		sort.Strings(out)
		text += "; " + strings.Join(out, ", ")
	}
	return text
}

func countRange(m map[int]int) (min, max int) {
	first := true
	for _, v := range m {
		if first {
			min, max = v, v
			first = false
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func missingIOLink(io map[int]int) bool {
	for _, n := range io {
		if n == 0 {
			return true
		}
	}
	return false
}

// largeBARItem compares the largest PCIe BAR of every GPU with its VRAM.
// A small BAR is not a P2P blocker: the host may pin the firmware BAR on
// purpose with pci=realloc=off.
func largeBARItem(root string, gpus []gpu.GPU, cmdline string) CheckItem {
	const title = "PCIe large BAR"
	type bar struct {
		g     gpu.GPU
		bytes uint64
	}
	var bars []bar
	for _, g := range gpus {
		if b := largestBAR(root, g); b > 0 {
			bars = append(bars, bar{g: g, bytes: b})
		}
	}
	if len(bars) == 0 {
		return p2pItem("large_bar", title, P2PSkip,
			"the largest PCIe BAR is unknown (no PCI resource file)").with("", TopologyDoc)
	}
	var short []bar
	for _, b := range bars {
		if b.g.VRAMBytes > 0 && b.bytes < b.g.VRAMBytes {
			short = append(short, b)
		}
	}
	if len(short) == 0 {
		smallest := bars[0]
		for _, b := range bars[1:] {
			if b.bytes < smallest.bytes {
				smallest = b
			}
		}
		detail := fmt.Sprintf("every GPU maps its full VRAM through the largest BAR (smallest: GPU %d, %s BAR, %s VRAM)",
			smallest.g.HIPIndex, sizeText(smallest.bytes), sizeText(smallest.g.VRAMBytes))
		if hasPCIOpt(cmdline, "realloc=off") {
			detail += "; pci=realloc=off on the kernel command line keeps the firmware BAR"
		}
		return p2pItem("large_bar", title, P2POK, detail).with("", TopologyDoc)
	}
	b := short[0]
	detail := fmt.Sprintf("the largest BAR of GPU %d is %s, below its %s of VRAM",
		b.g.HIPIndex, sizeText(b.bytes), sizeText(b.g.VRAMBytes))
	if len(short) < len(bars) {
		detail = fmt.Sprintf("%d of %d GPUs have a small BAR: %s", len(short), len(bars), detail)
	}
	if hasPCIOpt(cmdline, "realloc=off") {
		return p2pItem("large_bar", title, P2POK,
			detail+"; pci=realloc=off on the kernel command line keeps the firmware BAR").with("", TopologyDoc)
	}
	return p2pItem("large_bar", title, P2PWarn,
		detail+"; the kernel command line does not pin a BAR size").with(
		"Check the firmware for a large BAR setting: a GPU needs a PCI window above 4G for its full VRAM.", TopologyDoc)
}

// maxDeviceBARs is the number of lines of the PCI resource file that describe
// the BARs of the device. The next lines are the expansion ROM, the bridge
// windows and, on an SR-IOV card such as the V620, the VF BARs: their size is
// the size of one VF times the number of VFs, so they can pass for a large BAR.
const maxDeviceBARs = 6

// largestBAR reads the biggest BAR of the device from the PCI resource file.
func largestBAR(root string, g gpu.GPU) uint64 {
	if g.SysfsPath == "" {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(root, g.SysfsPath, "resource"))
	if err != nil {
		return 0
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > maxDeviceBARs {
		lines = lines[:maxDeviceBARs]
	}
	var max uint64
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		start, err1 := strconv.ParseUint(f[0], 0, 64)
		end, err2 := strconv.ParseUint(f[1], 0, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		if size := end - start + 1; size > max {
			max = size
		}
	}
	return max
}
