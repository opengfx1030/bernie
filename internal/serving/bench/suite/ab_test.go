package suite

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// TestABVisitsTheArmsInOrder checks that an A/B run visits the arms in the
// ABBA order, labels every visit and tells the verdict of the two arms.
func TestABVisitsTheArmsInOrder(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{Model: "fake", TTFT: 2 * ms, ChunkDelay: ms})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	o := ABOptions{
		Base: srv.URL, Cand: srv.URL,
		Options: bench.Options{InputLen: 64, OutputLen: 8, Concurrency: []int{1}, NumPrompts: 1},
	}
	res, err := AB(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Visits) != 4 {
		t.Fatalf("the run made %d visits, want 4", len(res.Visits))
	}
	want := []struct {
		arm string
		tag string
	}{
		{ArmBase, "ab:base:1"},
		{ArmCand, "ab:cand:2"},
		{ArmCand, "ab:cand:3"},
		{ArmBase, "ab:base:4"},
	}
	for i, v := range res.Visits {
		if v.Arm != want[i].arm {
			t.Errorf("the visit %d is the arm %q, want %q", i+1, v.Arm, want[i].arm)
		}
		if v.Tag != want[i].tag {
			t.Errorf("the visit %d has the tag %q, want %q", i+1, v.Tag, want[i].tag)
		}
		if v.Result == nil || len(v.Result.Cells) != 1 {
			t.Fatalf("the visit %d measured nothing", i+1)
		}
	}
	if res.Verdict == nil {
		t.Fatal("the run has no verdict")
	}
	if len(res.Verdict.Cells) != 1 {
		t.Errorf("the verdict holds %d cells, want 1", len(res.Verdict.Cells))
	}
}

// TestABRefusesAnUnknownArm checks that a visit of an unknown arm is
// refused before anything runs.
func TestABRefusesAnUnknownArm(t *testing.T) {
	o := ABOptions{Base: "http://127.0.0.1:1", Cand: "http://127.0.0.1:2", Visits: []string{"other"}}
	if _, err := AB(context.Background(), o); err == nil {
		t.Fatal("the run accepted an unknown arm")
	}
}
