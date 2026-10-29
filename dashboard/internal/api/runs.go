package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/sse"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// GET /api/runs — `bernie ps --json`.
func (h *handler) getRuns(w http.ResponseWriter, r *http.Request) {
	val, err := h.runs.get(r.Context(), "", func(ctx context.Context) (any, error) {
		return flow.Runs(ctx)
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Redacted(w, http.StatusOK, val)
}

// runRequest is the body of POST /api/runs.
type runRequest struct {
	Recipe string   `json:"recipe"`
	Set    []string `json:"set"`
	Mode   string   `json:"mode"`
	Name   string   `json:"name"`
}

// POST /api/runs — `bernie serve <recipe> --set … --mode … --name …`.
func (h *handler) postRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if req.Recipe == "" {
		httpjson.Error(w, http.StatusBadRequest, "body field recipe is required")
		return
	}
	// The run outlives this request; only a cancelled request before start aborts it.
	rec, err := app.Serve(context.WithoutCancel(r.Context()), app.ServeOptions{
		Prepare: runner.PrepareOptions{Recipe: req.Recipe, Mode: req.Mode, Name: req.Name, Sets: req.Set},
		Start:   runner.StartOptions{Progress: h.log},
	})
	switch {
	case errors.Is(err, app.ErrPrepare):
		httpjson.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, app.ErrRunActive):
		httpjson.Error(w, http.StatusConflict, err.Error())
	case err != nil:
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
	default:
		fmt.Fprintf(h.log, "bernie dash: started run %s (recipe %s, port %d)\n", rec.Name, rec.Recipe, rec.Port)
		httpjson.Redacted(w, http.StatusCreated, rec)
	}
}

// DELETE /api/runs/{name} — `bernie stop <name>`.
func (h *handler) deleteRun(w http.ResponseWriter, r *http.Request) {
	res, err := app.Stop(context.WithoutCancel(r.Context()), r.PathValue("name"))
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	fmt.Fprintf(h.log, "bernie dash: stopped run %s\n", res.Name)
	httpjson.Write(w, http.StatusOK, res)
}

// GET /api/runs/{name}/logs?tail=N — `bernie logs <name> -f`, one SSE event
// per line. The stream ends with an "end" event when the log stops (the run
// exited); a failure sends an "error" event with the message.
func (h *handler) getLogs(w http.ResponseWriter, r *http.Request) {
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httpjson.Error(w, http.StatusBadRequest, "tail must be a non-negative integer")
			return
		}
		tail = n
	}
	name := r.PathValue("name")
	if !runExists(r.Context(), name) {
		httpjson.Error(w, http.StatusNotFound, "no run named "+strconv.Quote(name))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := sse.New(w)
	go sse.Heartbeat(ctx, st, cancel)
	lw := &sse.LineWriter{Stream: st}
	err := runner.Logs(ctx, name, true, tail, lw)
	if ctx.Err() != nil {
		return // client left
	}
	_ = lw.Flush()
	if err != nil {
		_ = st.Event("error", sse.OneLine(err.Error()))
		return
	}
	_ = st.Event("end", "")
}

func runExists(ctx context.Context, name string) bool {
	runs, err := runner.List(ctx)
	if err != nil {
		return true // let Logs report the real error
	}
	for _, r := range runs {
		if r.Name == name {
			return true
		}
	}
	return false
}
