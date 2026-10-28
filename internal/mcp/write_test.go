package mcp

import (
	"errors"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"slices"
	"testing"
	"time"
)

func TestWriteToolsOnlyInWriteMode(t *testing.T) {
	isolate(t)
	ro := names(listTools(t, connect(t, Options{})))
	rw := names(listTools(t, connect(t, Options{Write: true})))
	for _, n := range mutatingTools {
		if slices.Contains(ro, n) {
			t.Errorf("read-only mode lists %s", n)
		}
		if !slices.Contains(rw, n) {
			t.Errorf("write mode does not list %s", n)
		}
	}
	if len(rw) != len(ro)+len(mutatingTools) {
		t.Errorf("write mode lists %d tools, read-only %d", len(rw), len(ro))
	}
	cs := connect(t, Options{})
	if res, err := cs.CallTool(t.Context(), mustParams("stop", map[string]any{"name": "x"})); err == nil && !res.IsError {
		t.Error("stop answered in read-only mode")
	}
}

func TestWriteSchemaValidation(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"stop", map[string]any{}},                                      // name required
		{"stop", map[string]any{"name": "x", "confirm_token": "short"}}, // token too short
		{"tune_power_set", map[string]any{"watts": -5}},
		{"bench_run", map[string]any{"bogus": 1}},
	} {
		if res, err := cs.CallTool(t.Context(), mustParams(c.tool, c.args)); err == nil && !res.IsError {
			t.Errorf("%s %v was accepted", c.tool, c.args)
		}
	}
}

func TestBenchRunRefusesRemoteURL(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	res := callTool(t, cs, "bench_run", map[string]any{"url": "http://10.1.2.3:8000"})
	if !res.IsError {
		t.Error("bench_run accepted a non-loopback URL")
	}
}

func TestTunePowerSetPlanThenConfirm(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	args := func(tok string) map[string]any {
		m := map[string]any{"watts": 180, "gpu_indexes": []int{0}}
		if tok != "" {
			m["confirm_token"] = tok
		}
		return m
	}
	res := callTool(t, cs, "tune_power_set", args(""))
	plan, _ := res.StructuredContent.(map[string]any)
	tok, _ := plan["confirm_token"].(string)
	if res.IsError || plan["executed"] != false || len(tok) < 32 {
		t.Fatalf("plan call: %+v", res.StructuredContent)
	}
	res = callTool(t, cs, "tune_power_set", args(tok))
	out, _ := res.StructuredContent.(map[string]any)
	if res.IsError || out["executed"] != true {
		t.Fatalf("confirm call: %+v", res.StructuredContent)
	}
	result, _ := out["result"].(map[string]any)
	if result["changed"] != true || result["watts"] != float64(180) {
		t.Fatalf("result = %+v", result)
	}
}

func TestConfirmToken(t *testing.T) {
	now := time.Now()
	ts := tokenStore{dir: t.TempDir(), now: func() time.Time { return now }}
	issue := func() string {
		tok, _, err := ts.issue("stop", "a1", "p1")
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	if err := ts.consume(issue(), "stop", "a1", "p1"); err != nil {
		t.Errorf("valid token: %v", err)
	}
	tok := issue()
	if err := ts.consume(tok, "stop", "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	if err := ts.consume(tok, "stop", "a1", "p1"); !errors.Is(err, errTokenUnknown) {
		t.Errorf("reused token: %v", err)
	}
	for _, c := range [][3]string{{"serve", "a1", "p1"}, {"stop", "a2", "p1"}, {"stop", "a1", "p2"}} {
		if err := ts.consume(issue(), c[0], c[1], c[2]); !errors.Is(err, errTokenBinding) {
			t.Errorf("token for %v: %v", c, err)
		}
	}
	tok = issue()
	now = now.Add(TokenTTL + time.Second)
	if err := ts.consume(tok, "stop", "a1", "p1"); !errors.Is(err, errTokenExpired) {
		t.Errorf("expired token: %v", err)
	}
	if err := ts.consume("nope", "stop", "a1", "p1"); !errors.Is(err, errTokenUnknown) {
		t.Errorf("unknown token: %v", err)
	}
	if es, _ := os.ReadDir(ts.dir); len(es) != 0 {
		t.Errorf("%d token files left", len(es))
	}
}

func mustParams(name string, args map[string]any) *sdk.CallToolParams {
	return &sdk.CallToolParams{Name: name, Arguments: args}
}

func TestServePlanThenConfirm(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	args := func(tok string) map[string]any {
		m := map[string]any{"id": "27b-awq", "mode": "host"}
		if tok != "" {
			m["confirm_token"] = tok
		}
		return m
	}
	res := callTool(t, cs, "serve", args(""))
	m, _ := res.StructuredContent.(map[string]any)
	tok, _ := m["confirm_token"].(string)
	if res.IsError || m["executed"] != false || len(tok) < 32 {
		t.Fatalf("plan call: %+v", res.StructuredContent)
	}
	// other arguments: the token no longer fits, and it is burnt.
	other := args(tok)
	other["id"] = "other"
	if res := callTool(t, cs, "serve", other); !res.IsError {
		t.Error("a token was accepted for other arguments")
	}
	if res := callTool(t, cs, "serve", args(tok)); !res.IsError {
		t.Error("a burnt token was accepted")
	}
}
