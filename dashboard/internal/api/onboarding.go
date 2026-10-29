package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/sse"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
)

// jobPoll is the delay between two reads of a running job.
const jobPoll = 300 * time.Millisecond

// GET /api/onboarding — the onboarding checklist of this host (spec §11).
// It is the same document `bernie status --steps` prints. The document comes
// from the cache when it is fresh enough; a document past its time to live
// is served as it is and refreshed in the background (spec §3.3).
func (h *handler) getOnboarding(w http.ResponseWriter, r *http.Request) {
	doc, err := app.OnboardingCached(r.Context(), app.OnboardingOptions{})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, doc)
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
func (h *handler) postOnboardingRun(w http.ResponseWriter, r *http.Request) {
	var req onboardingRunRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	step := r.PathValue("step")
	res, err := app.RunOnboardingStep(r.Context(), step, req.options())
	switch {
	case errors.Is(err, onboarding.ErrUnknownStep), errors.Is(err, onboarding.ErrNoAction):
		httpjson.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, onboarding.ErrJobBusy):
		httpjson.Error(w, http.StatusConflict, err.Error())
	case err != nil:
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
	case res.RootRequired:
		httpjson.Write(w, http.StatusOK, map[string]any{
			"step": step, "root_required": true, "commands": res.Commands,
		})
	default:
		httpjson.Write(w, http.StatusCreated, res.Job)
	}
}

// GET /api/jobs — the jobs of this host, newest first. The page re-attaches
// to a job with its id. The answer is capped; `limit` takes 1 to 200.
func (h *handler) getJobs(w http.ResponseWriter, r *http.Request) {
	limit := onboarding.DefaultJobLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpjson.Error(w, http.StatusBadRequest, "limit must be an integer between 1 and 200")
			return
		}
		limit = n
	}
	httpjson.Write(w, http.StatusOK, app.OnboardingJobs(limit))
}

// GET /api/jobs/{id} — the state of a job.
func (h *handler) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := app.OnboardingJob(r.PathValue("id"))
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, job)
}

// GET /api/jobs/{id}/events — the output of a job: one event per line, then
// a "done" or "failed" event with the job document.
//
// The stream replays the buffered lines from the beginning, so a page that
// reloads finds the whole output again. Every line event carries the number
// of the line as its id: a client that re-attaches sends it back in the
// Last-Event-ID header and gets the lines after it.
func (h *handler) getJobEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := app.OnboardingJob(id); err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := sse.New(w)
	go sse.Heartbeat(ctx, st, cancel)
	sent := lastEventID(r)
	for {
		lines, err := app.OnboardingJobLog(id)
		if err != nil {
			_ = st.Event("error", sse.OneLine(err.Error()))
			return
		}
		for ; sent < len(lines); sent++ {
			if st.EventID(strconv.Itoa(sent+1), "line", lines[sent]) != nil {
				return
			}
		}
		job, err := app.OnboardingJob(id)
		if err != nil {
			_ = st.Event("error", sse.OneLine(err.Error()))
			return
		}
		if job.State != onboarding.JobRunning {
			name := "done"
			if job.State == onboarding.JobFailed {
				name = "failed"
			}
			b, err := json.Marshal(job)
			if err != nil {
				_ = st.Event("error", sse.OneLine(err.Error()))
				return
			}
			_ = st.Event(name, string(b))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(jobPoll):
		}
	}
}

// lastEventID is the number of lines the client already has: the header of
// a re-attaching stream, or the query of a client that cannot set it.
func lastEventID(r *http.Request) int {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("last_event_id")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
