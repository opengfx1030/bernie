package mcp

import (
	"context"
	"errors"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
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
	// A valid token verifies and gives back the digest of the plan it is
	// bound to.
	bound, err := ts.verify(issue(), "stop", "a1")
	if err != nil {
		t.Errorf("valid token: %v", err)
	}
	if bound != "p1" {
		t.Errorf("the plan digest is %q, want p1", bound)
	}
	// A token works once.
	tok := issue()
	if _, err := ts.verify(tok, "stop", "a1"); err != nil {
		t.Errorf("valid token: %v", err)
	}
	if _, err := ts.verify(tok, "stop", "a1"); !errors.Is(err, errTokenUnknown) {
		t.Errorf("reused token: %v", err)
	}
	// A token is bound to the tool and to the arguments.
	for _, c := range [][3]string{{"serve", "a1", "p1"}, {"stop", "a2", "p1"}} {
		if _, err := ts.verify(issue(), c[0], c[1]); !errors.Is(err, errTokenBinding) {
			t.Errorf("token for %v: %v", c, err)
		}
	}
	tok = issue()
	now = now.Add(TokenTTL + time.Second)
	if _, err := ts.verify(tok, "stop", "a1"); !errors.Is(err, errTokenExpired) {
		t.Errorf("expired token: %v", err)
	}
	if _, err := ts.verify("nope", "stop", "a1"); !errors.Is(err, errTokenUnknown) {
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

// TestRefusedCallDoesNotPlan checks that a call the gate refuses never
// computes a plan: the plan of a recipe writes the TunableOp rows of the
// host, so a call without a valid token must not have that side effect.
func TestRefusedCallDoesNotPlan(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{Write: true})
	old := planOf
	calls := 0
	planOf = func(ctx context.Context, o runner.PrepareOptions) (*recipe.Plan, error) {
		calls++
		return old(ctx, o)
	}
	t.Cleanup(func() { planOf = old })

	// A token that was never issued: the call is refused, and nothing is
	// planned.
	res := callTool(t, cs, "serve", map[string]any{"id": "27b-awq", "confirm_token": strings.Repeat("a", 43)})
	if !res.IsError {
		t.Fatalf("a call with an unknown token was accepted: %s", resultText(res))
	}
	if calls != 0 {
		t.Errorf("the plan was computed %d times for a refused call", calls)
	}

	// The plan call plans once and gives a token.
	res = callTool(t, cs, "serve", map[string]any{"id": "27b-awq"})
	m, _ := res.StructuredContent.(map[string]any)
	tok, _ := m["confirm_token"].(string)
	if res.IsError || tok == "" {
		t.Fatalf("plan call: %s", resultText(res))
	}
	if calls != 1 {
		t.Errorf("the plan was computed %d times for the plan call", calls)
	}

	// The token is bound to the arguments: a call with other arguments is
	// refused, and it plans nothing.
	res = callTool(t, cs, "serve", map[string]any{"id": "other", "confirm_token": tok})
	if !res.IsError {
		t.Fatalf("a token was accepted for other arguments: %s", resultText(res))
	}
	if calls != 1 {
		t.Errorf("the plan was computed %d times for a refused call", calls)
	}
}
