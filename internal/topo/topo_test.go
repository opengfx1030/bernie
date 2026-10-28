package topo

import (
	"reflect"
	"strings"
	"testing"

	"github.com/opengfx1030/rdnactl/internal/fixture"
	"github.com/opengfx1030/rdnactl/internal/gpu"
)

func par1(t *testing.T) Topology {
	t.Helper()
	root := fixture.Root(t, "par1-cs25")
	gpus, err := gpu.List(root)
	if err != nil {
		t.Fatal(err)
	}
	topo, err := Discover(root, gpus)
	if err != nil {
		t.Fatal(err)
	}
	return topo
}

func TestDiscoverPar1Nodes(t *testing.T) {
	topo := par1(t)
	if len(topo.Nodes) != 10 || len(topo.Pairs) != 45 {
		t.Fatalf("nodes=%d pairs=%d", len(topo.Nodes), len(topo.Pairs))
	}
	w := topo.Nodes[0]
	if w.Root != "pci0000:c0" || w.RootPort != "0000:c0:03.1" || w.Switch != "" {
		t.Errorf("W7800 node = %+v (GPU-integrated switch must not count)", w)
	}
	v := topo.Nodes[2]
	wantPath := []string{"0000:40:03.1", "0000:41:00.0", "0000:42:00.0", "0000:43:00.0", "0000:44:00.0", "0000:45:00.0", "0000:46:00.0", "0000:47:00.0"}
	if !reflect.DeepEqual(v.Path, wantPath) {
		t.Errorf("V620 path = %v", v.Path)
	}
	// Both PEX88096 levels fold into the chip's top upstream port.
	for i, want := range []string{"", "", "0000:41:00.0", "0000:41:00.0", "0000:41:00.0", "0000:41:00.0", "0000:01:00.0", "0000:01:00.0", "0000:01:00.0", "0000:01:00.0"} {
		if topo.Nodes[i].Switch != want {
			t.Errorf("node %d switch = %q, want %q", i, topo.Nodes[i].Switch, want)
		}
	}
}

func TestDiscoverPar1Classes(t *testing.T) {
	topo := par1(t)
	cases := []struct {
		a, b  int
		class string
		hops  int
	}{
		{2, 3, ClassPIX, 8},  // same leaf of one PEX88096
		{2, 4, ClassPIX, 12}, // other leaf, same chip
		{6, 9, ClassPIX, 12},
		{2, 6, ClassPHB, 18}, // different root complexes, one socket
		{0, 1, ClassPHB, 10}, // W7800s on separate root complexes
		{0, 2, ClassPHB, 14},
	}
	for _, c := range cases {
		p, ok := topo.PairOf(c.a, c.b)
		if !ok {
			t.Fatalf("pair %d-%d missing", c.a, c.b)
		}
		if p.Class != c.class || p.Hops != c.hops {
			t.Errorf("pair %d-%d = %s/%d hops, want %s/%d", c.a, c.b, p.Class, p.Hops, c.class, c.hops)
		}
		if !p.P2P {
			t.Errorf("pair %d-%d: KFD p2p_links say P2P", c.a, c.b)
		}
	}
}

func TestClassify(t *testing.T) {
	s := func(ids ...string) []sw {
		var out []sw
		for _, id := range ids {
			out = append(out, sw{US: id})
		}
		return out
	}
	n := func(root string) Node { return Node{Root: root} }
	cases := []struct {
		name         string
		a, b         Node
		ca, cb       []sw
		numaA, numaB int
		want         string
	}{
		{"same switch", n("pci0000:00"), n("pci0000:00"), s("A"), s("A"), 0, 0, ClassPIX},
		{"nested switch", n("pci0000:00"), n("pci0000:00"), s("A"), s("A", "B"), 0, 0, ClassPXB},
		{"sibling switches", n("pci0000:00"), n("pci0000:00"), s("A", "B"), s("A", "C"), 0, 0, ClassPXB},
		{"root port only", n("pci0000:00"), n("pci0000:00"), nil, nil, 0, 0, ClassPHB},
		{"different switches", n("pci0000:00"), n("pci0000:00"), s("A"), s("C"), 0, 0, ClassPHB},
		{"roots same numa", n("pci0000:00"), n("pci0000:40"), s("A"), s("A"), 0, 0, ClassPHB},
		{"roots no numa info", n("pci0000:00"), n("pci0000:40"), nil, nil, -1, -1, ClassPHB},
		{"roots across sockets", n("pci0000:00"), n("pci0000:80"), nil, nil, 0, 1, ClassSYS},
	}
	for _, c := range cases {
		if got := classify(c.a, c.b, c.ca, c.cb, c.numaA, c.numaB); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestP2PFlags(t *testing.T) {
	root := t.TempDir()
	d := gpu.KFDNodesDir
	fixture.Write(t, root, map[string]string{
		d + "/1/p2p_links/0/properties": "node_from 1\nnode_to 2\nflags 3\n",
		d + "/1/p2p_links/1/properties": "node_from 1\nnode_to 3\nflags 19\n", // NO_PEER_TO_PEER_DMA
		d + "/1/p2p_links/2/properties": "node_from 1\nnode_to 4\nflags 0\n",  // not enabled
		d + "/1/io_links/0/properties":  "node_from 1\nnode_to 5\n",           // XGMI, no flags file field
	})
	got := p2pLinks(root)
	want := map[[2]int]bool{{1, 2}: true, {1, 5}: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("p2pLinks = %v, want %v", got, want)
	}
}

func TestBestGroup(t *testing.T) {
	topo := par1(t)
	cases := []struct {
		name   string
		cand   []int
		n      int
		want   []int
		reason string
	}{
		{"four V620 on first PLX", []int{2, 3, 4, 5, 6, 7, 8, 9}, 4, []int{2, 3, 4, 5}, "PIX"},
		{"skip a busy GPU", []int{3, 4, 5, 6, 7, 8, 9}, 4, []int{6, 7, 8, 9}, "0000:01:00.0"},
		{"two from the lowest PIX group", []int{9, 8, 3, 2}, 2, []int{2, 3}, "PIX"},
		{"no switch holds 3", []int{2, 3, 6, 7}, 3, []int{2, 3, 6}, "lowest HIP indices"},
		{"W7800 pair has no switch", []int{0, 1}, 2, []int{0, 1}, "lowest HIP indices"},
		{"too few", []int{0, 1}, 4, nil, "need 4"},
		{"single", []int{5, 3}, 1, []int{3}, "lowest HIP index"},
	}
	for _, c := range cases {
		got, reason := BestGroup(topo, c.cand, c.n)
		if !reflect.DeepEqual(got, c.want) || !strings.Contains(reason, c.reason) {
			t.Errorf("%s: got %v (%q), want %v (%q)", c.name, got, reason, c.want, c.reason)
		}
	}
}
