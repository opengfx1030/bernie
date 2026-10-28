package benchcmd

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

func TestFingerprintLineShowsGPUScope(t *testing.T) {
	line := fingerprintLine(map[string]string{
		"host":      "gpu-box",
		"gpus":      "1x AMD Radeon PRO V620 (gfx1030)",
		"gpu_scope": "run",
		"run":       "flash",
	})
	for _, want := range []string{"host gpu-box", "gpus 1x AMD Radeon PRO V620 (gfx1030) (run)", "run flash"} {
		if !strings.Contains(line, want) {
			t.Errorf("fingerprint line %q lacks %q", line, want)
		}
	}
}

// testCell is a cell with the five groups of numbers of spec §10.
func testCell() *bench.Cell {
	return &bench.Cell{
		Concurrency: 1, NumPrompts: 4, Completed: 4, Failed: 0,
		Measurement: bench.Measurement{
			DurationS:      3.21,
			TokensPerChunk: 1, MeanPromptTokens: 512, MeanOutputTokens: 64, OutputTokS: 48.2,
			Prefill: &bench.Rate{AggTokS: 1234.5, WindowS: 1.6,
				Req: &bench.Dist{Mean: 900.1, P50: 890, P95: 950.2, Min: 800, Max: 950.2}},
			Decode: &bench.Rate{AggTokS: 45.6, WindowS: 2,
				Req: &bench.Dist{Mean: 44.9, P50: 44.5, P95: 46, Min: 43, Max: 46}},
			TTFTs: &bench.Dist{Mean: 0.512, P50: 0.5, P95: 0.56, Min: 0.49, Max: 0.56},
			ITLms: &bench.Dist{Mean: 21.4, P50: 21, P95: 24, Min: 20, Max: 24},
		},
	}
}

// TestPrintCellTable checks the table of a cell: the five groups, their
// units and the numbers of the result.
func TestPrintCellTable(t *testing.T) {
	var b strings.Builder
	printCell(&b, testCell())
	want := `cell c=1   4 ok / 0 failed   wall 3.21 s   1.00 tok/chunk   in/out 512/64 tok
metric                agg  req mean   req p50   req p95   req min   req max
prefill tok/s      1234.5     900.1     890.0     950.2     800.0     950.2
decode tok/s         45.6      44.9      44.5      46.0      43.0      46.0
TTFT s                  –     0.512     0.500     0.560     0.490     0.560
ITL ms                  –     21.40     21.00     24.00     20.00     24.00
output tok/s         48.2         –         –         –         –         –
`
	if b.String() != want {
		t.Errorf("the cell table is:\n%s\nwant:\n%s", b.String(), want)
	}
}

// TestPrintCellTableWithoutNumbers checks that a cell without measured
// numbers shows a dash and not a zero.
func TestPrintCellTableWithoutNumbers(t *testing.T) {
	var b strings.Builder
	printCell(&b, &bench.Cell{Concurrency: 1, NumPrompts: 2, Completed: 0, Failed: 2, Measurement: bench.Measurement{DurationS: 1}})
	lines := strings.Split(b.String(), "\n")
	if len(lines) < 3 {
		t.Fatalf("the cell table is short:\n%s", b.String())
	}
	// The rows of the table hold no number: a cell that measured nothing
	// shows a dash everywhere.
	for _, line := range lines[2:] {
		if strings.ContainsAny(line, "0123456789") {
			t.Errorf("the cell table shows a zero instead of a dash:\n%s", b.String())
		}
	}
	if !strings.Contains(b.String(), "TTFT s") || !strings.Contains(b.String(), "–") {
		t.Errorf("the cell table lacks the units or the dash:\n%s", b.String())
	}
}

// TestPrintResultExplainsTheMetrics checks that the text says how the
// prefill and the decode numbers are measured.
func TestPrintResultExplainsTheMetrics(t *testing.T) {
	var b strings.Builder
	printBenchResult(&b, &bench.Result{ID: "x", Model: "m", URL: "http://127.0.0.1:8000",
		Cells: []bench.Cell{*testCell()}})
	for _, want := range []string{"prefill per request", "decode per request", "prompt tokens", "time to first token"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the result lacks %q:\n%s", want, b.String())
		}
	}
}
