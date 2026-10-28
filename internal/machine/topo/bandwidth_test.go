package topo

import (
	"os"
	"reflect"
	"testing"
)

func TestParseRBT(t *testing.T) {
	cases := []struct {
		file string
		want map[[2]int]float64
	}{
		// TransferBench p2p layout (RBT 2.6+): CPU rows/columns and "<-" rows are ignored.
		{"testdata/rbt-transferbench-p2p.txt", map[[2]int]float64{
			{0, 1}: 26.21, {0, 2}: 12.40, {1, 0}: 26.08, {1, 2}: 12.31, {2, 0}: 12.55, {2, 1}: 12.47,
		}},
		// Legacy layout: device 0 is the CPU, GPUs are devices 1-3 (ordinals 0-2).
		{"testdata/rbt-legacy.txt", map[[2]int]float64{
			{0, 1}: 26.23, {0, 2}: 13.11, {1, 0}: 26.18, {1, 2}: 13.05, {2, 0}: 13.12, {2, 1}: 13.07,
		}},
	}
	for _, c := range cases {
		b, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		if got := ParseRBT(string(b)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %v\nwant %v", c.file, got, c.want)
		}
	}
	if got := ParseRBT("usage: rocm_bandwidth_test [options]\n"); len(got) != 0 {
		t.Errorf("help text parsed: %v", got)
	}
}

func TestApplyBandwidth(t *testing.T) {
	topo := Topology{Pairs: []Pair{{A: 0, B: 1}, {A: 0, B: 2}, {A: 1, B: 2}}}
	apply(&topo, map[[2]int]float64{{0, 1}: 26.21, {1, 0}: 26.08, {0, 2}: 12.4})
	want := []float64{26.15, 12.4, 0}
	for i, p := range topo.Pairs {
		if p.GBps != want[i] {
			t.Errorf("pair %d-%d = %v, want %v", p.A, p.B, p.GBps, want[i])
		}
	}
}

func TestParseTorchProbe(t *testing.T) {
	got, err := parseTorchProbe("warning: noise\nRDNACTL_JSON:{\"0-1\": 25.5, \"1-0\": 24.9}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[[2]int]float64{{0, 1}: 25.5, {1, 0}: 24.9}) {
		t.Errorf("got %v", got)
	}
	if _, err := parseTorchProbe("Traceback"); err == nil {
		t.Error("missing result accepted")
	}
}
