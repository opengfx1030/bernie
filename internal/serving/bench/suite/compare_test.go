package suite

import (
	"errors"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// testWorkload is a workload for the compare tests.
func testWorkload() *bench.Workload {
	return &bench.Workload{
		ProtocolID:   bench.ProtocolID,
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
	cases := map[string]func(w *bench.Workload){
		"protocol":    func(w *bench.Workload) { w.ProtocolID = "other" },
		"corpus":      func(w *bench.Workload) { w.CorpusSHA256 = "def456" },
		"input":       func(w *bench.Workload) { w.InputLen = 8192 },
		"output":      func(w *bench.Workload) { w.OutputLen = 256 },
		"concurrency": func(w *bench.Workload) { w.Concurrency = []int{1} },
		"recall":      func(w *bench.Workload) { w.Recall = false },
		"suite":       func(w *bench.Workload) { w.SuiteID = "pr"; w.SuiteDigest = "abc" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := &bench.Result{ID: "a", Workload: testWorkload()}
			b := &bench.Result{ID: "b", Workload: testWorkload()}
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
	a := &bench.Result{ID: "a", Workload: testWorkload()}
	b := &bench.Result{ID: "b", Workload: testWorkload()}
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
	a := &bench.Result{ID: "a", Workload: testWorkload()}
	b := &bench.Result{ID: "b"}
	if _, err := Compare(a, b); !errors.Is(err, ErrWorkloadDiffers) {
		t.Fatalf("compare with a result without workload: %v", err)
	}
}

// TestCompareRefusesOtherProtocol checks that a result of another protocol
// version cannot be compared: its numbers come from another measurement.
func TestCompareRefusesOtherProtocol(t *testing.T) {
	a := &bench.Result{ID: "a", Workload: testWorkload()}
	b := &bench.Result{ID: "b", Workload: testWorkload()}
	b.Workload.ProtocolID = "bernie-bench-1"
	_, err := Compare(a, b)
	if !errors.Is(err, ErrWorkloadDiffers) {
		t.Fatalf("compare of different protocols: %v", err)
	}
	if !strings.Contains(err.Error(), "bernie-bench-1") || !strings.Contains(err.Error(), bench.ProtocolID) {
		t.Errorf("the error %q does not name the two protocols", err)
	}
}

// TestCompareRefusesOtherSuite checks that two runs of different suites
// cannot be compared: the suite digest pins the cells of a run.
func TestCompareRefusesOtherSuite(t *testing.T) {
	a := &bench.Result{ID: "a", Workload: &bench.Workload{ProtocolID: bench.ProtocolID, SuiteID: "pr", SuiteDigest: "one"}}
	b := &bench.Result{ID: "b", Workload: &bench.Workload{ProtocolID: bench.ProtocolID, SuiteID: "pr", SuiteDigest: "two"}}
	_, err := Compare(a, b)
	if !errors.Is(err, ErrWorkloadDiffers) {
		t.Fatalf("compare of different suites: %v", err)
	}
	if !strings.Contains(err.Error(), "suite digest") {
		t.Errorf("the error %q does not name the suite digest", err)
	}
}
