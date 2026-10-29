package onboarding

import (
	"errors"
	"testing"

	"github.com/opengfx1030/bernie/internal/setup"
)

func TestPrepareStepReturnsRootCommands(t *testing.T) {
	f := &Facts{Setup: []setup.Step{{
		ID: "groups", Root: true, Commands: []string{"groupadd -r render"},
	}}}
	prepared, err := PrepareStep(f, StepAccess, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.RootCommands) != 1 || prepared.RootCommands[0] != "groupadd -r render" {
		t.Errorf("root commands: %v", prepared.RootCommands)
	}
	if prepared.Action != nil {
		t.Error("a root step carries an action")
	}
}

func TestPrepareStepReturnsAction(t *testing.T) {
	f := &Facts{Recipe: "flashnext"}
	prepared, err := PrepareStep(f, StepServe, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Action == nil || prepared.Action.Kind != ActionServe {
		t.Fatalf("action: %+v", prepared.Action)
	}
}

func TestPrepareStepRejectsUnknownStep(t *testing.T) {
	_, err := PrepareStep(&Facts{}, "nope", Options{})
	if !errors.Is(err, ErrUnknownStep) {
		t.Errorf("unknown step: %v", err)
	}
}
