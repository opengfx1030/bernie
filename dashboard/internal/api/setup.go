package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/setup"
)

// setupRunLimit is the number of setup runs the list answers.
const setupRunLimit = 10

// setupLogLimit is the number of lines of a step log the page reads. The
// log of a build is long: the page shows its tail.
const setupLogLimit = 400

// setupRunsDoc is the answer of GET /api/setup/runs: the records of the
// runs of this host and the newest one in full.
type setupRunsDoc struct {
	Runs []*setup.Run `json:"runs"`
	Run  *setup.Run   `json:"run,omitempty"`
}

// GET /api/setup/runs — the records of the setup runs of this host, newest
// first, and the newest one in full. The page reads the same record the
// CLI prints with `bernie setup report` (spec §6.4).
func (h *handler) getSetupRuns(w http.ResponseWriter, r *http.Request) {
	runs := app.SetupRuns(setupRunLimit)
	doc := setupRunsDoc{Runs: runs}
	if len(runs) > 0 {
		doc.Run = runs[0]
	}
	httpjson.Write(w, http.StatusOK, doc)
}

// GET /api/setup/runs/{id}/log/{step} — the log of one step of a run: the
// command line first, then the output with a timestamp on every line.
func (h *handler) getSetupStepLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	step := r.PathValue("step")
	b, err := app.SetupStepLog(id, step)
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	truncated := false
	if len(lines) > setupLogLimit {
		lines = lines[len(lines)-setupLogLimit:]
		truncated = true
	}
	httpjson.Write(w, http.StatusOK, map[string]any{
		"run": id, "step": step, "lines": lines, "truncated": truncated,
	})
}

// setupVerifyRequest is the body of POST /api/setup/verify: the venv to
// check and the mode of the plan.
type setupVerifyRequest struct {
	Mode string `json:"mode"`
	Venv string `json:"venv"`
}

// POST /api/setup/verify — check the venv again in the background: the job
// runs the venv.verify step through the one verify function and leaves the
// record of the run in the store of the host, so the page reads the same
// record the CLI prints (spec §6.4). The answer is the job document.
//
// A job runs at a time: a second one answers 409.
func (h *handler) postSetupVerify(w http.ResponseWriter, r *http.Request) {
	var req setupVerifyRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	job, err := app.RunSetupVerify(r.Context(), app.SetupOptions{Mode: req.Mode, Venv: req.Venv})
	switch {
	case errors.Is(err, onboarding.ErrJobBusy):
		httpjson.Error(w, http.StatusConflict, err.Error())
	case err != nil:
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
	default:
		httpjson.Write(w, http.StatusCreated, job)
	}
}
