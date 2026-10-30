package mcp

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/app"
)

// setup_verify starts the Python of the venv and writes the record of the
// run: it is a mutating tool, so the write gate holds it back.
func TestSetupVerifyIsGated(t *testing.T) {
	isolate(t)
	for _, n := range names(listTools(t, connect(t, Options{}))) {
		if n == "setup_verify" {
			t.Error("read-only mode lists setup_verify")
		}
	}
	found := false
	for _, n := range names(listTools(t, connect(t, Options{Write: true}))) {
		if n == "setup_verify" {
			found = true
		}
	}
	if !found {
		t.Error("write mode does not list setup_verify")
	}
}

// The plan of setup_verify is the step the check runs, and the run lands in
// the store of the host: the report reads it back.
func TestSetupVerifyPlanThenConfirm(t *testing.T) {
	isolate(t)
	// No venv on this host: the check fails, and the run is recorded.
	t.Setenv("HOME", t.TempDir())
	cs := connect(t, Options{Write: true})
	args := func(tok string) map[string]any {
		m := map[string]any{"mode": "host"}
		if tok != "" {
			m["confirm_token"] = tok
		}
		return m
	}

	res := callTool(t, cs, "setup_verify", args(""))
	m, _ := res.StructuredContent.(map[string]any)
	tok, _ := m["confirm_token"].(string)
	if res.IsError || m["executed"] != false || len(tok) < 32 {
		t.Fatalf("plan call: %s", resultText(res))
	}
	plan, _ := m["plan"].(map[string]any)
	run, _ := plan["run"].([]any)
	if len(run) != 1 {
		t.Fatalf("the plan runs %d steps, want only the check", len(run))
	}
	step, _ := run[0].(map[string]any)
	if step["id"] != "venv.verify" {
		t.Errorf("the plan runs %v, want the check of the venv", step["id"])
	}

	// The token works once: the check runs and the record of the run is in
	// the store of the host.
	res = callTool(t, cs, "setup_verify", args(tok))
	if !res.IsError {
		out, _ := res.StructuredContent.(map[string]any)
		if out["executed"] != true {
			t.Fatalf("the confirm call did not run: %s", resultText(res))
		}
		result, _ := out["result"].(map[string]any)
		if result["run_id"] == "" {
			t.Error("the result has no run id")
		}
	}
	runs := app.SetupRuns(1)
	if len(runs) != 1 {
		t.Fatalf("the store holds %d runs, want the one of the verify", len(runs))
	}
	if len(runs[0].Steps) != 1 || runs[0].Steps[0].ID != "venv.verify" {
		t.Errorf("the run has the steps %v, want only the check", runs[0].Steps)
	}
}

// A token of another tool does not open the check.
func TestSetupVerifyRefusesAForeignToken(t *testing.T) {
	isolate(t)
	t.Setenv("HOME", t.TempDir())
	cs := connect(t, Options{Write: true})
	res := callTool(t, cs, "setup_verify", map[string]any{"mode": "host", "confirm_token": "0123456789abcdef0123456789abcdef0123456789abcdef"})
	if !res.IsError {
		t.Errorf("a foreign token was accepted: %s", resultText(res))
	}
}
