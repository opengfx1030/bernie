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
		case "bench_list", "bench_show", "bench_run":
		default:
			continue
		}
		seen++
		b, err := json.Marshal(tl.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		doc := string(b)
		for _, want := range []string{"prefill", "decode", "ttft_s", "itl_ms", "server_side", "tokens_exact"} {
			if !strings.Contains(doc, `"`+want+`"`) {
				t.Errorf("%s: the output schema has no %q", tl.Name, want)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("found %d bench tools, want 3", seen)
	}
}
