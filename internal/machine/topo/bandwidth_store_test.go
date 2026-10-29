package topo

import (
	"os"
	"testing"
)

// The measurement is saved under the state directory and read back: the
// page shows the numbers of the last measurement (ia.md §5 R32).
func TestSaveAndLoadMeasurement(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BERNIE_STATE_DIR", state)

	if m, err := LoadMeasurement(); err != nil || m != nil {
		t.Fatalf("no measurement yet: %v %v", m, err)
	}

	t0 := Topology{
		Pairs: []Pair{
			{A: 0, B: 1, GBps: 24.6},
			{A: 0, B: 2, GBps: 13.1},
			{A: 1, B: 2},
		},
		Bandwidth: BandwidthRBT,
	}
	if err := SaveMeasurement(&t0); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMeasurement()
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("the measurement was not saved")
	}
	if m.Source != BandwidthRBT {
		t.Errorf("source = %q, want %q", m.Source, BandwidthRBT)
	}
	if m.GBps["0-1"] != 24.6 || m.GBps["0-2"] != 13.1 {
		t.Errorf("gbps = %v", m.GBps)
	}
	if _, ok := m.GBps["1-2"]; ok {
		t.Error("a pair without a measurement was saved")
	}
	if m.MeasuredAt.IsZero() {
		t.Error("the measurement has no time")
	}

	// The measurement fills the pairs of a fresh topology.
	fresh := Topology{Pairs: []Pair{{A: 0, B: 1}, {A: 0, B: 2}, {A: 1, B: 2}}}
	ApplyMeasurement(&fresh, m)
	if fresh.Pairs[0].GBps != 24.6 || fresh.Pairs[1].GBps != 13.1 {
		t.Errorf("pairs = %+v", fresh.Pairs)
	}
	if fresh.Pairs[2].GBps != 0 {
		t.Errorf("an unmeasured pair got a number: %+v", fresh.Pairs[2])
	}
	if fresh.MeasuredSource != BandwidthRBT || fresh.MeasuredAt.IsZero() {
		t.Errorf("the topology does not carry the measurement: %+v", fresh)
	}
}

// A topology without a measurement keeps its empty numbers.
func TestApplyMeasurementWithoutOne(t *testing.T) {
	t0 := Topology{Pairs: []Pair{{A: 0, B: 1}}}
	ApplyMeasurement(&t0, nil)
	if t0.Pairs[0].GBps != 0 || !t0.MeasuredAt.IsZero() {
		t.Errorf("the topology changed: %+v", t0)
	}
}

// A topology with no measured pair is not saved: an empty file would say
// "measured" with nothing in it.
func TestSaveMeasurementWithoutNumbers(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	t0 := Topology{Pairs: []Pair{{A: 0, B: 1}}, Bandwidth: BandwidthRBT}
	if err := SaveMeasurement(&t0); err == nil {
		t.Error("a measurement without numbers was saved")
	}
	if _, err := os.Stat(measurementPath()); !os.IsNotExist(err) {
		t.Error("the file was written")
	}
}
