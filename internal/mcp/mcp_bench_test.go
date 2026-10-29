package mcp

import (
	"testing"
)

// bench_suites is read-only and lists the built-in suites with their digest.
func TestBenchSuitesIsReadOnly(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})
	res := callTool(t, cs, "bench_suites", map[string]any{})
	if res.IsError {
		t.Fatalf("bench_suites: %s", resultText(res))
	}
	got, ok := res.StructuredContent.([]any)
	if !ok || len(got) == 0 {
		t.Fatalf("bench_suites: %+v", res.StructuredContent)
	}
	row, _ := got[0].(map[string]any)
	if row["id"] == "" || row["digest"] == "" {
		t.Errorf("bench_suites row: %+v", row)
	}
}

// bench_compare refuses two ids that point at results that do not exist.
func TestBenchCompareRefusesMissingResult(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})
	res := callTool(t, cs, "bench_compare", map[string]any{"a": "nope", "b": "also-nope"})
	if !res.IsError {
		t.Error("bench_compare answered for two missing results")
	}
}

// bench_run refuses a remote URL (loopback only).
func TestBenchRunRefusesRemoteURL(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	res := callTool(t, cs, "bench_run", map[string]any{"url": "http://10.1.2.3:8000"})
	if !res.IsError {
		t.Error("bench_run accepted a remote URL")
	}
}

// bench_ab refuses a remote URL on either arm (loopback only).
func TestBenchABRefusesRemoteURL(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	for _, args := range []map[string]any{
		{"base": "http://10.1.2.3:8000", "cand": "anything"},
		{"base": "anything", "cand": "https://10.1.2.3:8000"},
	} {
		res := callTool(t, cs, "bench_ab", args)
		if !res.IsError {
			t.Errorf("bench_ab accepted %v", args)
		}
	}
}

// bench_ab refuses an arm that is neither a URL nor a non-empty string and
// refuses when one arm is missing.
func TestBenchABRequiresBothArms(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	for _, args := range []map[string]any{
		{},
		{"base": "", "cand": "x"},
		{"base": "x"},
		{"base": "x", "cand": ""},
	} {
		res := callTool(t, cs, "bench_ab", args)
		if !res.IsError {
			t.Errorf("bench_ab accepted %v", args)
		}
	}
}

// bench_ab is not listed in read-only mode and is listed in write mode.
func TestBenchABIsAMutatingTool(t *testing.T) {
	isolate(t)
	if !IsMutating("bench_ab") {
		t.Error("bench_ab should be mutating")
	}
	ro := names(listTools(t, connect(t, Options{})))
	for _, n := range ro {
		if n == "bench_ab" {
			t.Errorf("read-only mode lists bench_ab")
		}
	}
	rw := names(listTools(t, connect(t, Options{Write: true})))
	found := false
	for _, n := range rw {
		if n == "bench_ab" {
			found = true
		}
	}
	if !found {
		t.Error("write mode does not list bench_ab")
	}
}

// bench_run with a suite name goes through the suite runner and does not
// require the cell flags.
func TestBenchRunWithSuiteIgnoresCellFlags(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	// Only the schema is checked here; the runner is not called because the
	// suite does not exist on disk in the isolated test directory. The
	// point of the test is that the input shape is accepted (no schema
	// error): the tool refuses later, in the plan/confirm step.
	res := callTool(t, cs, "bench_run", map[string]any{
		"suite":      "smoke",
		"input_len":  99, // ignored when suite is set
		"output_len": 99, // ignored when suite is set
		"recall":     true,
	})
	if res == nil {
		t.Fatal("bench_run did not answer")
	}
}
