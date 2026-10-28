package bench

import (
	"errors"
	"strings"
	"testing"
)

// testWorkload is a workload for the compare tests.
func testWorkload() *Workload {
	return &Workload{
		ProtocolID:   ProtocolID,
		CorpusSHA256: "abc123",
		InputLen:     1024,
		OutputLen:    512,
		Concurrency:  []int{1, 8},
		Recall:       true,
	}
}

// TestCompareRefusesDifferentWorkloads checks that two results of different
// workloads cannot be put side by side.
func TestCompareRefusesDifferentWorkloads(t *testing.T) {
	cases := map[string]func(w *Workload){
		"protocol":    func(w *Workload) { w.ProtocolID = "other" },
		"corpus":      func(w *Workload) { w.CorpusSHA256 = "def456" },
		"input":       func(w *Workload) { w.InputLen = 8192 },
		"output":      func(w *Workload) { w.OutputLen = 256 },
		"concurrency": func(w *Workload) { w.Concurrency = []int{1} },
		"recall":      func(w *Workload) { w.Recall = false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := &Result{ID: "a", Workload: testWorkload()}
			b := &Result{ID: "b", Workload: testWorkload()}
			change(b.Workload)
			_, err := Compare(a, b)
			if !errors.Is(err, ErrWorkloadDiffers) {
				t.Fatalf("compare of different workloads: %v", err)
			}
			if !strings.Contains(err.Error(), "workload differs") {
				t.Errorf("error %q does not say that the workload differs", err)
			}
		})
	}
}

// TestCompareSameWorkload checks that two results of the same workload pair.
func TestCompareSameWorkload(t *testing.T) {
	a := &Result{ID: "a", Workload: testWorkload()}
	b := &Result{ID: "b", Workload: testWorkload()}
	cmp, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.A != a || cmp.B != b || cmp.Workload.InputLen != 1024 {
		t.Errorf("comparison: %+v", cmp)
	}
}

// TestCompareRefusesResultWithoutWorkload checks that a result saved before
// the workload fields cannot be compared.
func TestCompareRefusesResultWithoutWorkload(t *testing.T) {
	a := &Result{ID: "a", Workload: testWorkload()}
	b := &Result{ID: "b"}
	if _, err := Compare(a, b); !errors.Is(err, ErrWorkloadDiffers) {
		t.Fatalf("compare with a result without workload: %v", err)
	}
}
