package app

import (
	"errors"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

func TestBenchCompare(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	workload := bench.Workload{ProtocolID: bench.ProtocolID, Concurrency: []int{1}}
	for _, id := range []string{"a", "b"} {
		r := &bench.Result{ID: id, Workload: &workload, Cells: []bench.Cell{{Concurrency: 1}}}
		if _, err := bench.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	verdict, err := BenchCompare("a", "b", 0)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.A.ID != "a" || verdict.B.ID != "b" {
		t.Errorf("the verdict names %q and %q", verdict.A.ID, verdict.B.ID)
	}
	if _, err := BenchCompare("missing", "b", 0); !errors.Is(err, ErrBenchResultNotFound) {
		t.Errorf("a missing result: %v", err)
	}
}
