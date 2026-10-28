package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

// testRoot is a small command tree: a group with two leaves and one leaf with
// flags (a boolean, a value flag and positionals).
func testRoot() *Command {
	var leaf *Command
	leaf = &Command{Name: "leaf", Summary: "A leaf", Usage: "[--all] [--name N] [file]...", Run: func(c *Context, args []string) error {
		fs := c.Flags(leaf)
		fs.Bool("all", false, "all of it")
		name := fs.String("name", "", "a name")
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		return c.Emit(map[string]any{"name": *name, "pos": pos, "venv": c.Venv}, func(w io.Writer) { fmt.Fprintf(w, "name=%q pos=%v\n", *name, pos) })
	}}
	return &Command{Name: "bernie", Subs: []*Command{
		{Name: "ok", Summary: "Succeeds", Run: func(c *Context, args []string) error {
			return c.Emit(map[string]any{"ok": true, "json": c.JSON}, func(w io.Writer) { io.WriteString(w, "fine\n") })
		}},
		{Name: "fail", Summary: "Fails", Run: func(*Context, []string) error { return errors.New("boom") }},
		{Name: "usage", Summary: "Bad usage", Run: func(*Context, []string) error { return Usagef("need an argument") }},
		{Name: "checks", Summary: "Checks fail", Run: func(*Context, []string) error { return ChecksFailed("2 checks failed") }},
		{Name: "quiet", Summary: "Already explained", Run: func(*Context, []string) error { return &ExitError{Code: ExitFailure, Err: errors.New("")} }},
		{Name: "grp", Summary: "A group", Subs: []*Command{{Name: "sub", Summary: "A sub", Run: func(c *Context, _ []string) error {
			return c.Emit(map[string]any{"path": c.Path}, func(w io.Writer) { io.WriteString(w, c.Path+"\n") })
		}}}},
		leaf,
	}}
}

func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(context.Background(), &out, &errb, testRoot(), args)
	return code, out.String(), errb.String()
}

func TestExitCodesAndStreams(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		code     int
		out, err string // substrings; "" means the stream must be empty
		outExact string
	}{
		{name: "ok", args: []string{"ok"}, code: ExitOK, outExact: "fine\n"},
		{name: "runtime failure", args: []string{"fail"}, code: ExitFailure, err: "bernie: boom\n"},
		{name: "usage error", args: []string{"usage"}, code: ExitUsage, err: "bernie: need an argument\n"},
		{name: "checks failed", args: []string{"checks"}, code: ExitChecks, err: "bernie: 2 checks failed\n"},
		{name: "already explained", args: []string{"quiet"}, code: ExitFailure},
		{name: "unknown command", args: []string{"nope"}, code: ExitUsage, err: `unknown command "nope"`},
		{name: "group without subcommand", args: []string{"grp"}, code: ExitUsage, err: "missing subcommand"},
		{name: "nested", args: []string{"grp", "sub"}, code: ExitOK, outExact: "bernie grp sub\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errs := exec(t, tt.args...)
			if code != tt.code {
				t.Fatalf("exit = %d, want %d (stdout %q, stderr %q)", code, tt.code, out, errs)
			}
			if tt.outExact != "" && out != tt.outExact {
				t.Errorf("stdout = %q, want %q", out, tt.outExact)
			}
			if tt.outExact == "" && out != "" {
				t.Errorf("stdout must be empty, got %q", out)
			}
			if tt.err == "" && errs != "" {
				t.Errorf("stderr must be empty, got %q", errs)
			}
			if !strings.Contains(errs, tt.err) {
				t.Errorf("stderr = %q, want it to contain %q", errs, tt.err)
			}
		})
	}
}

func TestJSONErrorEnvelope(t *testing.T) {
	tests := []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"--json", "fail"}, ExitFailure, "boom"},
		{[]string{"usage", "--json"}, ExitUsage, "need an argument"},
		{[]string{"--json", "leaf", "--bogus"}, ExitUsage, "flag provided but not defined: -bogus"},
	}
	for _, tt := range tests {
		code, out, errs := exec(t, tt.args...)
		if code != tt.code || errs != "" {
			t.Fatalf("%v: exit %d stderr %q", tt.args, code, errs)
		}
		var env struct {
			Error    string `json:"error"`
			ExitCode int    `json:"exit_code"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("%v: stdout is not one JSON document: %q", tt.args, out)
		}
		if env.Error != tt.msg || env.ExitCode != tt.code {
			t.Errorf("%v: envelope = %+v", tt.args, env)
		}
	}
}

// Failed checks already printed their report; the JSON envelope is not added.
func TestChecksFailedHasNoEnvelope(t *testing.T) {
	code, out, errs := exec(t, "--json", "checks")
	if code != ExitChecks || out != "" || !strings.Contains(errs, "2 checks failed") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errs)
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}, {"grp", "--help"}, {"grp", "help"}, {"leaf", "--help"}, {"leaf", "-h"}} {
		code, out, errs := exec(t, args...)
		if code != ExitOK || errs != "" || !strings.Contains(out, "usage: bernie") {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, out, errs)
		}
	}
	_, out, _ := exec(t, "leaf", "--help")
	for _, want := range []string{"A leaf", "usage: bernie leaf [--all]", "-all", "-name", "-json"} {
		if !strings.Contains(out, want) {
			t.Errorf("leaf help lacks %q:\n%s", want, out)
		}
	}
}

func TestBadFlagPrintsOnce(t *testing.T) {
	code, out, errs := exec(t, "leaf", "--bogus")
	if code != ExitUsage || out != "" {
		t.Fatalf("exit %d stdout %q", code, out)
	}
	if n := strings.Count(errs, "flag provided but not defined"); n != 1 {
		t.Errorf("error printed %d times:\n%s", n, errs)
	}
	if strings.Contains(errs, "usage:") {
		t.Errorf("bad flag must not dump usage:\n%s", errs)
	}
}

func TestGlobalVenvBeforeOrAfterCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--venv", "/v", "leaf"},
		{"--json", "leaf", "--venv", "/v"},
		{"--json", "grp", "--venv", "/v", "sub"},
	} {
		code, out, errs := exec(t, args...)
		if code != ExitOK || errs != "" {
			t.Fatalf("%v: exit %d stderr %q", args, code, errs)
		}
		if args[len(args)-1] == "sub" {
			continue
		}
		var got struct {
			Venv string `json:"venv"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil || got.Venv != "/v" {
			t.Errorf("%v: venv = %+v, err %v", args, got, err)
		}
	}
	code, _, errs := exec(t, "--venv")
	if code != ExitUsage || !strings.Contains(errs, "--venv needs a directory") {
		t.Errorf("missing value: exit %d stderr %q", code, errs)
	}
}

func TestSplitJSON(t *testing.T) {
	tests := []struct {
		args []string
		rest []string
		json bool
	}{
		{[]string{"hw"}, []string{"hw"}, false},
		{[]string{"--json", "hw"}, []string{"hw"}, true},
		{[]string{"hw", "--json"}, []string{"hw"}, true},
		{[]string{"a", "--json", "--json"}, []string{"a"}, true},
		// After "--" everything is positional.
		{[]string{"serve", "--", "--json"}, []string{"serve", "--", "--json"}, false},
		{[]string{"serve", "--json", "--", "--json"}, []string{"serve", "--", "--json"}, true},
		// Behind a flag-like token the command's FlagSet decides.
		{[]string{"leaf", "--name", "--json"}, []string{"leaf", "--name", "--json"}, false},
		{[]string{"leaf", "--name=x", "--json"}, []string{"leaf", "--name=x"}, true},
		{[]string{"leaf", "x", "--json"}, []string{"leaf", "x"}, true},
	}
	for _, tt := range tests {
		rest, j := splitJSON(tt.args)
		if !reflect.DeepEqual(rest, tt.rest) || j != tt.json {
			t.Errorf("splitJSON(%v) = %v, %v; want %v, %v", tt.args, rest, j, tt.rest, tt.json)
		}
	}
}

func TestJSONValueAndBooleanFlag(t *testing.T) {
	// A value flag whose value is "--json" keeps it as a value, output stays human.
	code, out, errs := exec(t, "leaf", "--name", "--json")
	if code != ExitOK || errs != "" || strings.HasPrefix(out, "{") {
		t.Fatalf("value case: exit %d stdout %q stderr %q", code, out, errs)
	}
	// After a boolean flag the same token is the global flag.
	code, out, errs = exec(t, "leaf", "--all", "--json", "f")
	if code != ExitOK || errs != "" {
		t.Fatalf("bool case: exit %d stderr %q", code, errs)
	}
	var got struct{ Pos []string }
	if err := json.Unmarshal([]byte(out), &got); err != nil || !reflect.DeepEqual(got.Pos, []string{"f"}) {
		t.Errorf("bool case: stdout %q", out)
	}
	// Positionals after "--" are verbatim.
	code, out, _ = exec(t, "--json", "leaf", "--", "--json")
	if err := json.Unmarshal([]byte(out), &got); code != ExitOK || err != nil || !reflect.DeepEqual(got.Pos, []string{"--json"}) {
		t.Errorf("-- case: exit %d stdout %q", code, out)
	}
}

func TestFlagValueJSONInEnvelope(t *testing.T) {
	// --json parsed by the FlagSet (after a boolean flag) also switches the error envelope.
	code, out, _ := exec(t, "leaf", "--all", "--json", "--bogus")
	if code != ExitUsage || !strings.HasPrefix(out, `{"error"`) {
		t.Errorf("exit %d stdout %q", code, out)
	}
}
