package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/opengfx1030/bernie/internal/app"
)

// The structured result of setup_plan is the document of
// `bernie setup plan --json` (spec §13.4).
func TestSetupPlanIsTheCLIDocument(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})

	res := callTool(t, cs, "setup_plan", map[string]any{})
	if res.IsError {
		t.Fatalf("setup_plan: %s", resultText(res))
	}
	want, err := app.SetupPlan(context.Background(), app.SetupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asJSON(t, res.StructuredContent), asJSON(t, want)) {
		t.Errorf("setup_plan structuredContent differs from app.SetupPlan:\n%v\n%v",
			asJSON(t, res.StructuredContent), asJSON(t, want))
	}
	if !reflect.DeepEqual(asJSON(t, res.StructuredContent), asJSON(t, json.RawMessage(resultText(res)))) {
		t.Error("the text content is not the structured document")
	}
}

// setup_plan is read-only: the tool is listed without the write gate.
func TestSetupPlanIsReadOnly(t *testing.T) {
	isolate(t)
	tools := listTools(t, connect(t, Options{}))
	for _, tl := range tools {
		if tl.Name != "setup_plan" {
			continue
		}
		a := tl.Annotations
		if a == nil || !a.ReadOnlyHint {
			t.Errorf("setup_plan annotations %+v, want read-only", a)
		}
		in := asJSON(t, tl.InputSchema).(map[string]any)
		if in["type"] != "object" || in["additionalProperties"] != false {
			t.Errorf("setup_plan input schema %v must be a closed object", in)
		}
		return
	}
	t.Fatal("setup_plan is not listed")
}
