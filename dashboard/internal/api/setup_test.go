package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/setup"
)

// setupHandler returns the API routes with a state directory of its own and
// without a venv on this host: the check of the venv fails fast.
func setupHandler(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	t.Setenv("BERNIE_DATA_DIR", t.TempDir())
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")
	mux := http.NewServeMux()
	Register(mux, Options{})
	return mux
}

// waitJob waits until the job ends and returns its document.
func waitJob(t *testing.T, h http.Handler, id string) onboarding.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		w := get(t, h, "/api/jobs/"+id)
		if w.Code != http.StatusOK {
			t.Fatalf("the job %s: %d %s", id, w.Code, w.Body.String())
		}
		var job onboarding.Job
		if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
			t.Fatalf("the job %s is not a document: %v\n%s", id, err, w.Body.String())
		}
		if job.State != onboarding.JobRunning {
			return job
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the job %s did not end", id)
	return onboarding.Job{}
}

// TestSetupVerifyRouteStartsTheJob: the route starts the check of the venv
// in the background, and the job leaves the record of the run in the store
// of the host: the same record the CLI writes and the MCP server returns
// (spec §6.4).
func TestSetupVerifyRouteStartsTheJob(t *testing.T) {
	h := setupHandler(t)
	w := post(t, h, "/api/setup/verify", `{"mode":"host"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("the verify: %d %s", w.Code, w.Body.String())
	}
	var job onboarding.Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatalf("the answer is not a job document: %v\n%s", err, w.Body.String())
	}
	if job.ID == "" || job.Step != setup.StepVenvVerify {
		t.Fatalf("the job is %+v", job)
	}
	waitJob(t, h, job.ID)

	runs := app.SetupRuns(1)
	if len(runs) != 1 {
		t.Fatalf("the store holds %d runs, want the one of the verify", len(runs))
	}
	if len(runs[0].Steps) != 1 {
		t.Fatalf("the run has %d steps, want only the check", len(runs[0].Steps))
	}
	if got := runs[0].Steps[0].ID; got != setup.StepVenvVerify {
		t.Errorf("the run has the step %q, want %q", got, setup.StepVenvVerify)
	}
}

// TestSetupVerifyRouteIsBusyWhileAJobRuns: one job runs at a time, so the
// route answers 409 while a job of the host runs.
func TestSetupVerifyRouteIsBusyWhileAJobRuns(t *testing.T) {
	h := setupHandler(t)
	release := make(chan struct{})
	action := &onboarding.Action{
		Step: setup.StepVenvVerify, Kind: onboarding.ActionSetupVerify,
		Verify: func(ctx context.Context, log io.Writer) (*setup.Run, error) {
			<-release
			return nil, nil
		},
	}
	job, err := onboarding.StartJob(context.Background(), &onboarding.PreparedStep{Step: setup.StepVenvVerify, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	w := post(t, h, "/api/setup/verify", `{"mode":"host"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("the verify while a job runs: got %d, want 409\n%s", w.Code, w.Body.String())
	}
	close(release)
	waitJob(t, h, job.ID)
}

// TestSetupVerifyRouteBody: the route refuses a body it does not know.
func TestSetupVerifyRouteBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"an unknown field", `{"bogus":true}`, http.StatusBadRequest},
		{"a broken body", `{`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := setupHandler(t)
			w := post(t, h, "/api/setup/verify", c.body)
			if w.Code != c.want {
				t.Errorf("the body %s: got %d, want %d\n%s", c.body, w.Code, c.want, w.Body.String())
			}
			if runs := app.SetupRuns(1); len(runs) != 0 {
				t.Errorf("the refused body left %d runs behind", len(runs))
			}
		})
	}
}
