package suite

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// ms is a millisecond.
const ms = time.Millisecond

// TestRunSuiteMeasuresEveryGroup runs the smoke suite against the fake
// server and checks that the result holds the cells of the group with their
// lens and the five groups of numbers of spec §10.
func TestRunSuiteMeasuresEveryGroup(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{Model: "fake", TTFT: 5 * ms, ChunkDelay: ms})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s, err := Get("smoke")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(context.Background(), s, bench.Options{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cells) != 1 {
		t.Fatalf("the smoke suite measured %d cells, want 1", len(r.Cells))
	}
	c := r.Cells[0]
	if c.Group != "short" || c.InputLen != 512 || c.OutputLen != 128 {
		t.Errorf("the cell carries the group %q and the lens %d/%d", c.Group, c.InputLen, c.OutputLen)
	}
	if c.Completed == 0 || c.Failed != 0 {
		t.Errorf("the cell completed %d requests and failed %d", c.Completed, c.Failed)
	}
	if c.Prefill == nil || c.Prefill.AggTokS <= 0 || c.Prefill.Req == nil {
		t.Errorf("the cell has no prefill numbers: %+v", c.Prefill)
	}
	if c.Decode == nil || c.Decode.AggTokS <= 0 || c.Decode.Req == nil {
		t.Errorf("the cell has no decode numbers: %+v", c.Decode)
	}
	if c.TTFTs == nil || c.TTFTs.P50 <= 0 {
		t.Errorf("the cell has no time to first token: %+v", c.TTFTs)
	}
	if c.ITLms == nil || c.ITLms.P50 <= 0 {
		t.Errorf("the cell has no inter-token latency: %+v", c.ITLms)
	}
	if r.Suite == nil || r.Suite.ID != "smoke" || r.Suite.Digest != s.Digest() {
		t.Errorf("the result names the suite %+v", r.Suite)
	}
	if r.Workload == nil || r.Workload.SuiteID != "smoke" || r.Workload.SuiteDigest != s.Digest() {
		t.Errorf("the workload names the suite %+v", r.Workload)
	}
	if r.Workload.InputLen != 0 || r.Workload.OutputLen != 0 {
		t.Errorf("the workload of a suite run carries a lens: %+v", r.Workload)
	}
	if r.Options.Suite != "smoke" {
		t.Errorf("the options name the suite %q", r.Options.Suite)
	}
}

// TestRunSuiteRepeatsEveryCell checks that a cell holds the medians of its
// repetitions, the repetitions themselves and the spread of the metrics.
func TestRunSuiteRepeatsEveryCell(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{Model: "fake", TTFT: 5 * ms, ChunkDelay: ms})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s := &Suite{
		Schema: Schema, ID: "t", Name: "T", Reps: 3,
		Compare: Thresholds{Band: DefaultBand, Steady: DefaultSteady},
		Groups: []Group{{
			Name: "g", Kind: KindThroughput, InputLen: 64, OutputLen: 8,
			Concurrency: []int{1}, Reps: 3, Source: Source{Kind: SourceSynthetic},
		}},
	}
	r, err := Run(context.Background(), s, bench.Options{URL: srv.URL, NumPrompts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cells) != 1 {
		t.Fatalf("the suite measured %d cells, want 1", len(r.Cells))
	}
	c := r.Cells[0]
	if len(c.Reps) != 3 {
		t.Fatalf("the cell holds %d repetitions, want 3", len(c.Reps))
	}
	if c.Spread == nil {
		t.Fatal("the cell has no spread")
	}
	lo, hi := 0.0, 0.0
	for i, rep := range c.Reps {
		v := rep.OutputTokS
		if i == 0 {
			lo, hi = v, v
		}
		lo, hi = min(lo, v), max(hi, v)
	}
	if got, want := c.Spread["output_tok_s"], hi/lo; got != want {
		t.Errorf("the spread of the output rate is %v, want %v", got, want)
	}
	if c.OutputTokS < lo || c.OutputTokS > hi {
		t.Errorf("the output rate %v is not between the repetitions %v and %v", c.OutputTokS, lo, hi)
	}
}

// TestRunSuiteTurnsOnTheRecallGate checks that the recall gate of a suite
// reaches the cells: a suite that asks for it checks every reply.
func TestRunSuiteTurnsOnTheRecallGate(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{Model: "fake", TTFT: 2 * ms, ChunkDelay: ms, RecallMode: "correct"})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s := &Suite{
		Schema: Schema, ID: "t", Name: "T", Reps: 1, Recall: true,
		Compare: Thresholds{Band: DefaultBand, Steady: DefaultSteady},
		Groups: []Group{{
			Name: "g", Kind: KindThroughput, InputLen: 64, OutputLen: 8,
			Concurrency: []int{1}, Reps: 1, Source: Source{Kind: SourceSynthetic},
		}},
	}
	r, err := Run(context.Background(), s, bench.Options{URL: srv.URL, NumPrompts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cells) != 1 || r.Cells[0].Recall == nil {
		t.Fatalf("the cell has no recall verdict: %+v", r.Cells)
	}
	rec := r.Cells[0].Recall
	if rec.Asked == 0 || rec.Passed != rec.Asked {
		t.Errorf("the recall check is %d/%d", rec.Passed, rec.Asked)
	}
	if !r.Workload.Recall {
		t.Error("the workload does not say that the recall gate ran")
	}
}
