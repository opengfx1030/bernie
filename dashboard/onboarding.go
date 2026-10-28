package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
)

// jobPoll is the delay between two reads of a running job.
const jobPoll = 300 * time.Millisecond

// GET /api/onboarding — the onboarding checklist of this host (spec §11).
// It is the same document `bernie status --steps` prints. The document comes
// from the cache when it is fresh enough; a document past its time to live
// is served as it is and refreshed in the background (spec §3.3).
func (s *server) getOnboarding(w http.ResponseWriter, r *http.Request) {
	doc, err := app.OnboardingCached(r.Context(), app.OnboardingOptions{})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// onboardingRunRequest is the optional body of POST /api/onboarding/{step}/run: the
// same launch flags as `serve`.
type onboardingRunRequest struct {
	Recipe string   `json:"recipe"`
	Set    []string `json:"set"`
	Mode   string   `json:"mode"`
	Venv   string   `json:"venv"`
}

// options returns the options of the request.
func (b onboardingRunRequest) options() app.OnboardingOptions {
	return app.OnboardingOptions{Recipe: b.Recipe, Sets: b.Set, Mode: b.Mode, Venv: b.Venv}
}

// POST /api/onboarding/{step}/run — run the action of a step in the
// background. A step that needs root answers with the commands to run, and
// runs nothing.
func (s *server) postOnboardingRun(w http.ResponseWriter, r *http.Request) {
	var req onboardingRunRequest
	if !decodeBody(w, r, &req) {
		return
	}
	step := r.PathValue("step")
	doc, err := app.Onboarding(r.Context(), req.options())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	found := findStep(doc.Steps, step)
	if found == nil {
		httpError(w, http.StatusBadRequest, "no onboarding step named "+step)
		return
	}
	if len(found.RootCommands) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"step": step, "root_required": true, "commands": found.RootCommands,
		})
		return
	}
	if !found.CanRun {
		httpError(w, http.StatusBadRequest, "this step has nothing bernie can run")
		return
	}
	job, err := app.StartOnboardingJob(r.Context(), step, req.options())
	switch {
	case errors.Is(err, onboarding.ErrJobBusy):
		httpError(w, http.StatusConflict, err.Error())
	case err != nil:
		httpError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusCreated, job)
	}
}

// GET /api/jobs/{id} — the state of a job.
func (s *server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := app.OnboardingJob(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// GET /api/jobs/{id}/events — the output of a job: one event per line, then
// a "done" or "failed" event with the job document.
func (s *server) getJobEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := app.OnboardingJob(id); err != nil {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := newSSE(w)
	go heartbeat(ctx, st, cancel)
	sent := 0
	for {
		lines, err := app.OnboardingJobLog(id)
		if err != nil {
			_ = st.event("error", oneLine(err.Error()))
			return
		}
		for ; sent < len(lines); sent++ {
			if st.event("line", lines[sent]) != nil {
				return
			}
		}
		job, err := app.OnboardingJob(id)
		if err != nil {
			_ = st.event("error", oneLine(err.Error()))
			return
		}
		if job.State != onboarding.JobRunning {
			name := "done"
			if job.State == onboarding.JobFailed {
				name = "failed"
			}
			b, err := json.Marshal(job)
			if err != nil {
				_ = st.event("error", oneLine(err.Error()))
				return
			}
			_ = st.event(name, string(b))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(jobPoll):
		}
	}
}

// findStep returns the onboarding step with the given id, or nil.
func findStep(steps []onboarding.Step, id string) *onboarding.Step {
	for i := range steps {
		if steps[i].ID == id {
			return &steps[i]
		}
	}
	return nil
}
