package topo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/gpu"
)

// KFD io_link flag bits (linux kfd_crat.h).
const (
	linkEnabled  = 1 << 0
	linkNoP2PDMA = 1 << 4
)

// hierarchical lists PCIe switches that present one physical chip as two
// switch levels (Broadcom/PLX PEX88xxx Gen4 and Gen5). Like RCCL/NCCL, the
// second level is folded into the first so all GPUs on one chip are PIX.
var hierarchical = map[string]bool{
	"0x1000:0xc010": true, // PEX88000 Gen4 (PEX88096 ...)
	"0x1000:0xc030": true, // PEX89000 Gen5
}

// sw is one PCIe switch on a GPU's path: its top upstream port and id.
type sw struct {
	US string
	ID string // vendor:device of the upstream port
}

// Discover builds the PCIe position of each GPU and classifies every pair.
func Discover(root string, gpus []gpu.GPU) (Topology, error) {
	root = canonRoot(root)
	t := Topology{Nodes: []Node{}, Pairs: []Pair{}}
	chains := make([][]sw, len(gpus))
	numa := make([]int, len(gpus))
	for i, g := range gpus {
		n, chain, err := node(root, g)
		if err != nil {
			return t, err
		}
		t.Nodes = append(t.Nodes, n)
		chains[i] = chain
		numa[i] = g.NUMANode
	}
	links := p2pLinks(root)
	for i := range gpus {
		for j := i + 1; j < len(gpus); j++ {
			a, b := t.Nodes[i], t.Nodes[j]
			p := Pair{A: a.HIPIndex, B: b.HIPIndex}
			p.Class = classify(a, b, chains[i], chains[j], numa[i], numa[j])
			p.Hops = hops(a, b)
			ka, kb := gpus[i].KFDNode, gpus[j].KFDNode
			p.P2P = links[[2]int{ka, kb}] || links[[2]int{kb, ka}]
			t.Pairs = append(t.Pairs, p)
		}
	}
	return t, nil
}

// node resolves the PCI device of g in /sys/devices and walks its path.
func node(root string, g gpu.GPU) (Node, []sw, error) {
	n := Node{HIPIndex: g.HIPIndex, PCI: g.PCI, Path: []string{}}
	real, err := filepath.EvalSymlinks(filepath.Join(root, "sys/bus/pci/devices", g.PCI))
	if err != nil && g.SysfsPath != "" {
		real, err = filepath.EvalSymlinks(filepath.Join(root, g.SysfsPath))
	}
	if err != nil {
		return n, nil, fmt.Errorf("gpu %d (%s): resolve PCI device: %w", g.HIPIndex, g.PCI, err)
	}
	rel, err := filepath.Rel(filepath.Join(root, "sys/devices"), real)
	if err != nil || strings.HasPrefix(rel, "..") {
		return n, nil, fmt.Errorf("gpu %d (%s): %s is not under /sys/devices", g.HIPIndex, g.PCI, real)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "pci") {
		return n, nil, fmt.Errorf("gpu %d (%s): unexpected device path %s", g.HIPIndex, g.PCI, rel)
	}
	n.Root = parts[0]
	for _, p := range parts[1:] {
		if gpu.IsBDF(p) {
			n.Path = append(n.Path, p)
		}
	}
	if len(n.Path) > 1 {
		n.RootPort = n.Path[0]
	}
	chain := switches(root, n)
	if len(chain) > 0 {
		n.Switch = chain[len(chain)-1].US
	}
	return n, chain, nil
}

// switches returns the effective PCIe switches between the root port and
// the GPU, top down. A switch is an upstream port (a bridge whose parent is
// a bridge) followed by a downstream port. GPU-integrated switches (AMD
// vendor bridges directly above the GPU) are dropped, and hierarchical
// switch chips are folded into one switch.
func switches(root string, n Node) []sw {
	dir := filepath.Join(root, "sys/devices", n.Root)
	dirs := make([]string, len(n.Path))
	for i, p := range n.Path {
		dir = filepath.Join(dir, p)
		dirs[i] = dir
	}
	var chain []sw
	// Path[0] is the root port and Path[len-1] the GPU; the bridges between
	// them come in (upstream port, downstream port) pairs.
	for i := 1; i+1 <= len(n.Path)-2; i += 2 {
		us, ds := dirs[i], dirs[i+1]
		if !isBridge(us) || !isBridge(ds) {
			break
		}
		chain = append(chain, sw{US: n.Path[i], ID: readString(filepath.Join(us, "vendor")) + ":" + readString(filepath.Join(us, "device"))})
	}
	// Drop GPU-integrated switches (AMD upstream/downstream port pair).
	for len(chain) > 0 && strings.HasPrefix(chain[len(chain)-1].ID, "0x1002:") {
		chain = chain[:len(chain)-1]
	}
	// Fold hierarchical chips.
	var out []sw
	for _, s := range chain {
		if len(out) > 0 && hierarchical[s.ID] && out[len(out)-1].ID == s.ID {
			continue
		}
		out = append(out, s)
	}
	return out
}

func isBridge(dir string) bool {
	return strings.HasPrefix(readString(filepath.Join(dir, "class")), "0x0604")
}

// classify gives the RCCL link class of two GPUs. Different root complexes
// on the same NUMA node meet in the CPU's I/O die (PHB, as in RCCL); SYS
// needs different NUMA nodes.
func classify(a, b Node, ca, cb []sw, numaA, numaB int) string {
	if a.Root != b.Root {
		if numaA >= 0 && numaB >= 0 && numaA != numaB {
			return ClassSYS
		}
		return ClassPHB
	}
	common := 0
	for common < len(ca) && common < len(cb) && ca[common].US == cb[common].US {
		common++
	}
	switch {
	case common == 0:
		return ClassPHB
	case common == len(ca) && common == len(cb):
		return ClassPIX
	default:
		return ClassPXB
	}
}

// hops counts PCIe links between two GPUs in the device tree. Root
// complexes connect through one virtual CPU node.
func hops(a, b Node) int {
	if a.Root != b.Root {
		return len(a.Path) + len(b.Path) + 2
	}
	c := 0
	for c < len(a.Path) && c < len(b.Path) && a.Path[c] == b.Path[c] {
		c++
	}
	return len(a.Path) - c + len(b.Path) - c
}

// p2pLinks reads enabled peer links (from, to KFD node) from io_links and
// p2p_links of every KFD node.
func p2pLinks(root string) map[[2]int]bool {
	out := map[[2]int]bool{}
	nodes, _ := filepath.Glob(filepath.Join(root, gpu.KFDNodesDir, "*"))
	for _, nd := range nodes {
		for _, kind := range []string{"io_links", "p2p_links"} {
			props, _ := filepath.Glob(filepath.Join(nd, kind, "*", "properties"))
			for _, pf := range props {
				p, err := gpu.ReadProperties(pf)
				if err != nil {
					continue
				}
				flags, hasFlags := p["flags"]
				if hasFlags && (flags&linkEnabled == 0 || flags&linkNoP2PDMA != 0) {
					continue
				}
				out[[2]int{int(p["node_from"]), int(p["node_to"])}] = true
			}
		}
	}
	return out
}

// BestGroup selects n GPUs from candidates (HIP indices): a set below one
// PCIe switch (PIX) first, then the lowest HIP indices.
func BestGroup(t Topology, candidates []int, n int) ([]int, string) {
	cand := append([]int(nil), candidates...)
	sort.Ints(cand)
	switch {
	case n <= 0:
		return nil, "no GPUs requested"
	case len(cand) < n:
		return nil, fmt.Sprintf("need %d GPUs, %d candidates", n, len(cand))
	case n == 1:
		return cand[:1], fmt.Sprintf("GPU %d: lowest HIP index", cand[0])
	}
	bySwitch := map[string][]int{}
	for _, idx := range cand {
		for _, nd := range t.Nodes {
			if nd.HIPIndex == idx && nd.Switch != "" {
				bySwitch[nd.Switch] = append(bySwitch[nd.Switch], idx)
			}
		}
	}
	var best []int
	bestSwitch := ""
	for s, members := range bySwitch {
		if len(members) < n {
			continue
		}
		sel := members[:n]
		if best == nil || less(sel, best) {
			best, bestSwitch = sel, s
		}
	}
	if best != nil {
		return append([]int(nil), best...), fmt.Sprintf("GPUs %s share PCIe switch %s (PIX)", join(best), bestSwitch)
	}
	sel := cand[:n]
	return append([]int(nil), sel...), fmt.Sprintf("GPUs %s: no PCIe switch holds %d candidates; lowest HIP indices", join(sel), n)
}

func less(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func join(idx []int) string {
	s := make([]string, len(idx))
	for i, v := range idx {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, ",")
}

// PairOf returns the pair of HIP indices a and b (any order).
func (t Topology) PairOf(a, b int) (Pair, bool) {
	if a > b {
		a, b = b, a
	}
	for _, p := range t.Pairs {
		if p.A == a && p.B == b {
			return p, true
		}
	}
	return Pair{}, false
}

func canonRoot(root string) string {
	if root == "" {
		root = "/"
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		return r
	}
	return root
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
