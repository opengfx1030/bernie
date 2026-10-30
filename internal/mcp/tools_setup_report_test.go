package mcp

import (
	"reflect"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/setup"
)

// The record of a setup run is the document of `bernie setup report
// --json` (spec §13.4): the tool reads the same record the CLI prints.
func TestSetupReportIsTheCLIDocument(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})

	store := setup.DefaultRunStore()
	run := &setup.Run{
		ID: setup.NewRunID(), State: setup.RunDone, Mode: "host",
		StartedAt: time.Now().UTC(), DurationS: 1.5,
		Steps: []setup.RunStep{{ID: "venv.verify", Title: "Verify the vLLM build in the venv",
			State: setup.StepDone, DurationS: 1.5, ExitCode: 0, Summary: "done"}},
	}
	if err := store.Create(run); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, cs, "setup_report", map[string]any{})
	if res.IsError {
		t.Fatalf("setup_report: %s", resultText(res))
	}
	want, err := app.SetupRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asJSON(t, res.StructuredContent), asJSON(t, want)) {
		t.Errorf("setup_report differs from the record of the run:\n%v\n%v",
			asJSON(t, res.StructuredContent), asJSON(t, want))
	}

	// The tool takes the id of a run too.
	res = callTool(t, cs, "setup_report", map[string]any{"run": run.ID})
	if res.IsError {
		t.Fatalf("setup_report with an id: %s", resultText(res))
	}
	if !reflect.DeepEqual(asJSON(t, res.StructuredContent), asJSON(t, want)) {
		t.Error("setup_report with an id returns another document")
	}
}

// setup_report is read-only: the tool is listed without the write gate.
func TestSetupReportIsReadOnly(t *testing.T) {
	isolate(t)
	tools := listTools(t, connect(t, Options{}))
	for _, tl := range tools {
		if tl.Name != "setup_report" {
			continue
		}
		a := tl.Annotations
		if a == nil || !a.ReadOnlyHint {
			t.Errorf("setup_report annotations %+v, want read-only", a)
		}
		return
	}
	t.Fatal("setup_report is not listed")
}
