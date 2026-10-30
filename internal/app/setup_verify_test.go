package app

import (
	"context"
	"io"
	"testing"

	"github.com/opengfx1030/bernie/internal/setup"
)

// isolateVerify points every bernie directory at a fresh temp dir, so the
// test finds no venv and writes no record outside the temp dir.
func isolateVerify(t *testing.T) {
	t.Helper()
	for _, k := range []string{"BERNIE_STATE_DIR", "BERNIE_CONFIG_DIR", "BERNIE_DATA_DIR", "BERNIE_CACHE_DIR"} {
		t.Setenv(k, t.TempDir())
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")
}

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

// TestSetupVerifyRunsTheCheckAndRecordsIt: the verify function runs the
// check of the venv and nothing else, and it leaves the record of the run
// in the store of the host, so the CLI, the MCP server and the dashboard
// read the same one.
func TestSetupVerifyRunsTheCheckAndRecordsIt(t *testing.T) {
	isolateVerify(t)
	res, err := SetupVerify(context.Background(), SetupOptions{Mode: "host"}, io.Discard)
	if res == nil || res.Run == nil {
		t.Fatalf("verify returned %+v (%v)", res, err)
	}
	if len(res.Run.Steps) != 1 {
		t.Fatalf("the run has %d steps, want only the check", len(res.Run.Steps))
	}
	if got := res.Run.Steps[0].ID; got != setup.StepVenvVerify {
		t.Errorf("the run has the step %q, want %q", got, setup.StepVenvVerify)
	}
	runs := SetupRuns(1)
	if len(runs) != 1 || runs[0].ID != res.Run.ID {
		t.Errorf("the store holds %d runs, want the one of the verify", len(runs))
	}
}

// TestSetupVerifyNeedsAHostPlan: a plan without a check of the venv (the
// container mode) fails before anything runs.
func TestSetupVerifyNeedsAHostPlan(t *testing.T) {
	isolateVerify(t)
	res, err := SetupVerify(context.Background(), SetupOptions{Mode: "container"}, io.Discard)
	if err == nil {
		t.Fatal("the verify of a container plan did not fail")
	}
	if res != nil {
		t.Errorf("the verify returned %+v, want nothing", res)
	}
	if runs := SetupRuns(1); len(runs) != 0 {
		t.Errorf("the store holds %d runs, want none", len(runs))
	}
}
