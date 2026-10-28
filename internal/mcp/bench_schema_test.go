package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The bench tools describe the metric groups of spec §10 in their output
// schema, so an agent sees the same fields as the CLI.
func TestBenchToolSchemasCarryTheMetricGroups(t *testing.T) {
	isolate(t)
	tools := listTools(t, connect(t, Options{Write: true}))
	seen := 0
	for _, tl := range tools {
		switch tl.Name {
		case "bench_list", "bench_show", "bench_run", "bench_compare", "bench_ab":
		default:
			continue
		}
		seen++
		b, err := json.Marshal(tl.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		doc := string(b)
		// bench_compare and bench_ab carry the per-cell metric verdicts; the
		// other three carry the full cell with the server_side histogram and
		// the tokens_exact flag.
		wants := []string{"prefill", "decode", "ttft_s", "itl_ms"}
		if tl.Name != "bench_compare" && tl.Name != "bench_ab" {
			wants = append(wants, "server_side", "tokens_exact")
		}
		for _, want := range wants {
			if !strings.Contains(doc, `"`+want+`"`) {
				t.Errorf("%s: the output schema has no %q", tl.Name, want)
			}
		}
	}
	if seen != 5 {
		t.Fatalf("found %d bench tools, want 5", seen)
	}
}
