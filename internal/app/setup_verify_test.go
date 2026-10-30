package app

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/setup"
)

// TestSetupForceVerify: `setup verify` runs the venv check even when its
// postcondition holds, leaves the other steps alone and does not change the
// plan it was given.
func TestSetupForceVerify(t *testing.T) {
	plan := []setup.Step{
		{ID: "venv.create", Done: true},
		{ID: setup.StepVenvVerify, Done: true},
		{ID: "model.download", Done: true},
	}
	cases := []struct {
		name    string
		steps   []setup.Step
		wantErr bool
	}{
		{"done check is forced", plan, false},
		{"container plan has no check", []setup.Step{{ID: "image.pull", Done: true}}, true},
		{"empty plan", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := SetupForceVerify(tc.steps)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			for _, s := range out {
				if s.ID == setup.StepVenvVerify && s.Done {
					t.Errorf("%s is still done", s.ID)
				}
				if s.ID != setup.StepVenvVerify && !s.Done {
					t.Errorf("%s was changed", s.ID)
				}
			}
			if !tc.steps[1].Done {
				t.Errorf("the plan given was modified")
			}
			pending := SetupPending(out, []string{setup.StepVenvVerify})
			if len(pending) != 1 || pending[0].ID != setup.StepVenvVerify {
				t.Errorf("pending = %v, want only the check", pending)
			}
		})
	}
}
