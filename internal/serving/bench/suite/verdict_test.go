package suite

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// verdictCell builds a cell whose numbers follow one speed factor. The
// repetitions are the speeds of the repetitions.
func verdictCell(speed float64, reps []float64) bench.Cell {
	c := bench.Cell{Concurrency: 1, Group: "g", Completed: 4, NumPrompts: 4}
	c.Prefill = &bench.Rate{AggTokS: 1000 * speed, WindowS: 1, Req: &bench.Dist{Mean: 1000 * speed}}
	c.Decode = &bench.Rate{AggTokS: 50 * speed, WindowS: 1, Req: &bench.Dist{Mean: 50 * speed}}
	c.TTFTs = &bench.Dist{P50: 1 / speed}
	c.ITLms = &bench.Dist{P50: 20 / speed}
	c.OutputTokS = 50 * speed
	lo, hi := 0.0, 0.0
	for i, r := range reps {
		rate, ttft := 50*r, 1/r
		if i == 0 {
			lo, hi = rate, rate
		}
		lo, hi = min(lo, rate), max(hi, rate)
		c.Reps = append(c.Reps, bench.Rep{NumPrompts: 4, Completed: 4, Measurement: bench.Measurement{
			Prefill:    &bench.Rate{AggTokS: 1000 * r},
			Decode:     &bench.Rate{AggTokS: rate},
			TTFTs:      &bench.Dist{P50: ttft},
			ITLms:      &bench.Dist{P50: 20 / r},
			OutputTokS: rate,
		}})
	}
	if len(reps) > 0 {
		spread := lo / hi
		c.Spread = map[string]float64{
			"prefill.agg_tok_s": spread, "decode.agg_tok_s": spread,
			"ttft_s.p50": spread, "itl_ms.p50": spread, "output_tok_s": spread,
		}
	}
	return c
}

// verdictResult builds a result of one cell.
func verdictResult(id string, speed float64, reps []float64) *bench.Result {
	return &bench.Result{
		ID: id, Model: "m",
		Workload: &bench.Workload{
			ProtocolID: bench.ProtocolID, SuiteID: "pr", SuiteDigest: "d",
			Kind: KindThroughput, CompareBand: DefaultBand, CompareSteady: DefaultSteady,
		},
		Cells: []bench.Cell{verdictCell(speed, reps)},
	}
}

// metricVerdict returns the verdict of one metric of the first cell.
func metricVerdict(t *testing.T, v *Verdict, name string) MetricVerdict {
	t.Helper()
	for _, cv := range v.Cells {
		for _, m := range cv.Metrics {
			if m.Metric == name {
				return m
			}
		}
	}
	t.Fatalf("the verdict has no metric %q", name)
	return MetricVerdict{}
}

// TestJudgeFindsTheRegression checks that a candidate that lost five per
// cent is worse, and that the comparison says so with its exit code.
func TestJudgeFindsTheRegression(t *testing.T) {
	cmp, err := Compare(verdictResult("a", 1.0, []float64{1.0}), verdictResult("b", 0.95, []float64{0.95}))
	if err != nil {
		t.Fatal(err)
	}
	v := Judge(cmp, 0)
	if !v.Valid {
		t.Fatalf("the comparison is invalid: %v", v.Reasons)
	}
	if got := metricVerdict(t, v, "decode.agg_tok_s").Verdict; got != VerdictWorse {
		t.Errorf("the decode rate is %q, want worse", got)
	}
	if got := metricVerdict(t, v, "itl_ms.p50").Verdict; got != VerdictWorse {
		t.Errorf("the inter-token latency is %q, want worse", got)
	}
	if v.Summary.Worse == 0 {
		t.Error("the summary counts no worse metric")
	}
	if v.ExitCode != 3 {
		t.Errorf("the exit code is %d, want 3", v.ExitCode)
	}
}

// TestJudgeCallsNoiseTheSame checks that a change inside the band is the
// same, not a regression.
func TestJudgeCallsNoiseTheSame(t *testing.T) {
	cmp, err := Compare(verdictResult("a", 1.0, []float64{1.0}), verdictResult("b", 0.99, []float64{0.99}))
	if err != nil {
		t.Fatal(err)
	}
	v := Judge(cmp, 0)
	if got := metricVerdict(t, v, "decode.agg_tok_s").Verdict; got != VerdictSame {
		t.Errorf("the decode rate is %q, want same", got)
	}
	if v.ExitCode != 0 {
		t.Errorf("the exit code is %d, want 0", v.ExitCode)
	}
}

// TestJudgeCallsNoisyRepetitionsNoisy checks that an arm whose repetitions
// disagree gets no verdict: the numbers of that arm say nothing.
func TestJudgeCallsNoisyRepetitionsNoisy(t *testing.T) {
	cmp, err := Compare(verdictResult("a", 1.0, []float64{1.0, 0.85}), verdictResult("b", 1.0, []float64{1.0}))
	if err != nil {
		t.Fatal(err)
	}
	v := Judge(cmp, 0)
	if got := metricVerdict(t, v, "decode.agg_tok_s").Verdict; got != VerdictNoisy {
		t.Errorf("the decode rate is %q, want noisy", got)
	}
	if v.ExitCode != 1 {
		t.Errorf("the exit code is %d, want 1", v.ExitCode)
	}
}

// TestJudgeRefusesAFailedRecall checks that a run whose server did not
// repeat the hidden fact gets no verdict at all.
func TestJudgeRefusesAFailedRecall(t *testing.T) {
	a := verdictResult("a", 1.0, []float64{1.0})
	a.Cells[0].Recall = &bench.Recall{Asked: 4, Passed: 3}
	cmp, err := Compare(a, verdictResult("b", 1.0, []float64{1.0}))
	if err != nil {
		t.Fatal(err)
	}
	v := Judge(cmp, 0)
	if v.Valid {
		t.Error("the comparison of a failed recall is valid")
	}
	if v.ExitCode != 1 {
		t.Errorf("the exit code is %d, want 1", v.ExitCode)
	}
}

// TestJudgeRefusesAnIncompleteCell checks that a cell that lost a request
// gets no verdict.
func TestJudgeRefusesAnIncompleteCell(t *testing.T) {
	a := verdictResult("a", 1.0, []float64{1.0})
	a.Cells[0].Incomplete = true
	cmp, err := Compare(a, verdictResult("b", 1.0, []float64{1.0}))
	if err != nil {
		t.Fatal(err)
	}
	v := Judge(cmp, 0)
	if v.Valid {
		t.Error("the comparison of an incomplete cell is valid")
	}
	if v.ExitCode != 1 {
		t.Errorf("the exit code is %d, want 1", v.ExitCode)
	}
}
