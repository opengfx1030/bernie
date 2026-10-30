package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// TestDefaultRunStatusOnly: when the default flow is overridden to just
// "status", bare `bernie` prints the status document and exits 0. This
// pins the routing (status is the first default command) and proves the
// JSON branch works without the dashboard binding a port in the test.
func TestDefaultRunStatusOnly(t *testing.T) {
	saved := defaultCommands
	defaultCommands = []string{"status"}
	defer func() { defaultCommands = saved }()

	savedReg := registry
	registry = map[string]*Command{}
	var gotJSON bool
	registry["status"] = &Command{Name: "status", Summary: "test status", Run: func(c *Context, args []string) error {
		gotJSON = c.JSON
		return c.Emit(map[string]any{"ok": true, "json": c.JSON}, func(w io.Writer) { io.WriteString(w, "status-ok\n") })
	}}
	defer func() { registry = savedReg }()

	var out, errb bytes.Buffer
	// No args, not --json: human output, the status command runs.
	code := defaultRun(context.Background(), &out, &errb, &Command{Name: "bernie", Subs: topLevel()}, false, nil)
	if code != ExitOK {
		t.Fatalf("defaultRun code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "status-ok") {
		t.Errorf("stdout = %q, want status-ok", out.String())
	}
	if gotJSON {
		t.Errorf("json flag leaked to the non-JSON branch")
	}

	// --json with no command: the same command runs but in JSON mode.
	out.Reset()
	errb.Reset()
	code = defaultRun(context.Background(), &out, &errb, &Command{Name: "bernie", Subs: topLevel()}, true, nil)
	if code != ExitOK {
		t.Fatalf("defaultRun --json code = %d, want 0", code)
	}
	if !gotJSON {
		t.Errorf("json flag did not reach the command")
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, out.String())
	}
	if v, _ := doc["ok"].(bool); !v {
		t.Errorf("doc = %v, want ok=true", doc)
	}
	if v, _ := doc["json"].(bool); !v {
		t.Errorf("doc = %v, want json=true", doc)
	}
}
