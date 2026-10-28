// Package topo discovers the PCIe tree between GPUs, classifies GPU pairs
// with RCCL link classes (PIX, PXB, PHB, SYS), reads KFD P2P links and runs
// optional bandwidth tests.
package topo

// Link classes, from closest to farthest.
const (
	ClassPIX = "PIX" // same PCIe switch
	ClassPXB = "PXB" // several switches, no root complex
	ClassPHB = "PHB" // through one root complex
	ClassSYS = "SYS" // across root complexes
)

// Node is the PCIe position of one GPU.
type Node struct {
	HIPIndex int      `json:"hip_index"`
	PCI      string   `json:"pci"`
	Path     []string `json:"path"`      // PCI addresses from the root port down to the GPU
	Switch   string   `json:"switch"`    // upstream port of the nearest PCIe switch, "" if none
	RootPort string   `json:"root_port"` // first PCI address under the host bridge
	Root     string   `json:"root"`      // PCI domain:bus of the root complex, e.g. "pci0000:00"
}

// Pair is the relation of two GPUs (A < B).
type Pair struct {
	A     int     `json:"a"`
	B     int     `json:"b"`
	Class string  `json:"class"`
	Hops  int     `json:"hops"`
	P2P   bool    `json:"p2p"`
	GBps  float64 `json:"gbps,omitempty"`
}

// Topology is the full result of Discover (+ MeasureBandwidth).
type Topology struct {
	Nodes     []Node `json:"nodes"`
	Pairs     []Pair `json:"pairs"`
	Bandwidth string `json:"bandwidth,omitempty"` // "rocm-bandwidth-test" | "torch" | ""
}
