package onboarding

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"testing"

	"github.com/opengfx1030/bernie/internal/setup"
)

// TestButtonAndCLIShareTheRecord: the button of the Setup tab and the CLI
// run the same apply function, so they write the same record of the run
// (spec §6.4).
func TestButtonAndCLIShareTheRecord(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	dir := t.TempDir()
	steps := []setup.Step{{ID: "venv.create", Title: "Create the Python venv",
		Commands: []string{"echo ran > " + dir + "/ran"}}}

	// The button: the action of a checklist step.
	action := &Action{Kind: ActionSetup, Steps: steps, Apply: applyOptions("host", "tiny", "")}
	if err := action.Run(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	button, err := setup.DefaultRunStore().Latest()
	if err != nil {
		t.Fatal(err)
	}

	// The CLI: the same apply function, with the same inputs.
	cli, err := setup.ApplyRun(context.Background(), steps, nil, io.Discard,
		setup.ApplyOptions{Store: setup.DefaultRunStore(), Mode: "host", Recipe: "tiny"})
	if err != nil {
		t.Fatal(err)
	}

	// The two records have the same shape: the same keys in the record
	// and the same keys in every step.
	if got, want := keysOf(t, button), keysOf(t, cli); !equal(got, want) {
		t.Errorf("the record of the button has the keys %v, want %v", got, want)
	}
	if len(button.Steps) != len(cli.Steps) {
		t.Fatalf("the button wrote %d steps, want %d", len(button.Steps), len(cli.Steps))
	}
	for i := range button.Steps {
		if got, want := keysOf(t, button.Steps[i]), keysOf(t, cli.Steps[i]); !equal(got, want) {
			t.Errorf("the step %s of the button has the keys %v, want %v", button.Steps[i].ID, got, want)
		}
	}
	// The step of the button ran and kept its log.
	step := button.Step("venv.create")
	if step == nil {
		t.Fatal("the button wrote no step")
	}
	if step.State != setup.StepDone || step.LogPath == "" {
		t.Errorf("the step of the button is %+v, want a step that ran with a log", step)
	}
}

// keysOf are the JSON keys of a value, sorted.
func keysOf(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// equal compares two lists of keys.
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
