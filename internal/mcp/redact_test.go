package mcp

// The redaction rule of the MCP server (spec §13.7): every result leaves the
// process without a secret. The tools that return host, run, log or config
// text are the ones where the rule has something to remove.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// theSecret is the value that must never leave the process.
const theSecret = "hf_abcdefghijklmnopqrstuvwx"

// TestToolResultsCarryNoSecret calls the tools that return text and checks
// that the secret is in none of the answers: the log of the run holds one,
// and the plan of a recipe carries it in its environment.
func TestToolResultsCarryNoSecret(t *testing.T) {
	dir := isolate(t)
	logPath := filepath.Join(dir, "serve.log")
	text := "Failed to infer device type: Authorization: Bearer " + theSecret + "\n"
	if err := os.WriteFile(logPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(os.Getenv("BERNIE_STATE_DIR"), "runs", "r1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec, _ := json.Marshal(runner.Record{Name: "r1", Mode: "host", Log: logPath})
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), rec, 0o644); err != nil {
		t.Fatal(err)
	}

	cs := connect(t, Options{})
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"logs", map[string]any{"name": "r1"}},
		{"doctor_log", map[string]any{"run": "r1"}},
		{"recipe_plan", map[string]any{"id": "27b-awq", "set": []string{"env.HF_TOKEN=" + theSecret}}},
		{"ps", map[string]any{}},
	} {
		res := callTool(t, cs, c.tool, c.args)
		if res.IsError {
			t.Fatalf("%s: %s", c.tool, resultText(res))
		}
		if text := resultText(res); strings.Contains(text, theSecret) {
			t.Errorf("%s answers with the secret: %s", c.tool, text)
		}
	}
}
