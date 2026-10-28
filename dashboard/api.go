package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/redact"
)

// root is the host filesystem root the probes read ("/" in production).
const root = "/"

// listGPUs returns the KFD GPUs; a host without KFD has none (not an error).
func listGPUs() ([]gpu.GPU, error) {
	vis, err := gpu.Visible(root)
	if gpu.Restricted(err) {
		return []gpu.GPU{}, err // the restriction names a GPU this host does not have
	}
	gpus := vis.GPUs
	if errors.Is(err, fs.ErrNotExist) {
		return []gpu.GPU{}, nil
	}
	if gpus == nil {
		gpus = []gpu.GPU{}
	}
	return gpus, err
}

// GET /api/hw — `bernie hw --json`.
func (s *server) getHW(w http.ResponseWriter, r *http.Request) {
	rep, err := app.HW(r.Context())
	if err == nil && rep.GPUErr != nil && !errors.Is(rep.GPUErr, fs.ErrNotExist) {
		err = rep.GPUErr // a host without KFD has no GPUs; other read errors are failures
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// GET /api/topo — `bernie topo --json` (no bandwidth test: it loads the GPUs).
func (s *server) getTopo(w http.ResponseWriter, r *http.Request) {
	t, err := app.Topo(r.Context(), app.TopoOptions{NoKFDOK: true})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// GET /api/topo/check — `bernie topo --check --json`.
func (s *server) getTopoCheck(w http.ResponseWriter, r *http.Request) {
	rep, err := app.P2PChecklist(r.Context(), app.P2PChecklistOptions{Recipe: r.URL.Query().Get("recipe")})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// GET /api/doctor — `bernie doctor --json` (all categories, configured venv).
func (s *server) getDoctor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rep, err := app.Doctor(r.Context(), q["category"], q.Get("venv"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// GET /api/recipes — `bernie recipe list --all --json`.
func (s *server) getRecipes(w http.ResponseWriter, r *http.Request) {
	l, err := app.RecipeList(true)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// GET /api/recipes/{id}/plan?set=K=V&mode=M — `bernie recipe plan --json`.
func (s *server) getPlan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := runner.Prepare(r.Context(), runner.PrepareOptions{
		Recipe: r.PathValue("id"), Mode: q.Get("mode"), Name: q.Get("name"), Sets: q["set"],
	})
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeRedacted(w, http.StatusOK, p)
}

// GET /api/runs — `bernie ps --json`.
func (s *server) getRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := flow.Runs(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeRedacted(w, http.StatusOK, runs)
}

// runRequest is the body of POST /api/runs.
type runRequest struct {
	Recipe string   `json:"recipe"`
	Set    []string `json:"set"`
	Mode   string   `json:"mode"`
	Name   string   `json:"name"`
}

// POST /api/runs — `bernie serve <recipe> --set … --mode … --name …`.
func (s *server) postRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Recipe == "" {
		httpError(w, http.StatusBadRequest, "body field recipe is required")
		return
	}
	p, err := runner.Prepare(r.Context(), runner.PrepareOptions{Recipe: req.Recipe, Mode: req.Mode, Name: req.Name, Sets: req.Set})
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The run outlives this request; only a cancelled request before start aborts it.
	rec, err := runner.Start(context.WithoutCancel(r.Context()), p, runner.StartOptions{Progress: s.o.Log})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Fprintf(s.o.Log, "bernie dash: started run %s (recipe %s, port %d)\n", rec.Name, rec.Recipe, rec.Port)
	writeRedacted(w, http.StatusCreated, rec)
}

// DELETE /api/runs/{name} — `bernie stop <name>`.
func (s *server) deleteRun(w http.ResponseWriter, r *http.Request) {
	res, err := app.Stop(context.WithoutCancel(r.Context()), r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}
	fmt.Fprintf(s.o.Log, "bernie dash: stopped run %s\n", res.Name)
	writeJSON(w, http.StatusOK, res)
}

// GET /api/runs/{name}/logs?tail=N — `bernie logs <name> -f`, one SSE event
// per line. The stream ends with an "end" event when the log stops (the run
// exited); a failure sends an "error" event with the message.
func (s *server) getLogs(w http.ResponseWriter, r *http.Request) {
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httpError(w, http.StatusBadRequest, "tail must be a non-negative integer")
			return
		}
		tail = n
	}
	name := r.PathValue("name")
	if !runExists(r.Context(), name) {
		httpError(w, http.StatusNotFound, "no run named "+strconv.Quote(name))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := newSSE(w)
	go heartbeat(ctx, st, cancel)
	lw := &lineEvents{s: st}
	err := runner.Logs(ctx, name, true, tail, lw)
	if ctx.Err() != nil {
		return // client left
	}
	_ = lw.flush()
	if err != nil {
		_ = st.event("error", oneLine(err.Error()))
		return
	}
	_ = st.event("end", "")
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

// heartbeat sends an SSE comment periodically; a failed write means the client
// is gone, so it cancels the stream.
func heartbeat(ctx context.Context, st *sseStream, cancel context.CancelFunc) {
	t := time.NewTicker(sseHeartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if st.comment("ping") != nil {
				cancel()
				return
			}
		}
	}
}

// GET /api/telemetry — 1 Hz SSE; every event is one telemetry JSON document.
func (s *server) getTelemetry(w http.ResponseWriter, r *http.Request) {
	ch, unsubscribe := s.tele.subscribe()
	defer unsubscribe()
	st := newSSE(w)
	ctx := r.Context()
	ping := time.NewTicker(sseHeartbeat)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ch:
			if st.event("", string(b)) != nil {
				return
			}
		case <-ping.C:
			if st.comment("ping") != nil {
				return
			}
		}
	}
}

// GET /api/bench — `bernie bench list --json`.
func (s *server) getBench(w http.ResponseWriter, r *http.Request) {
	rs, err := app.BenchList()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

// benchRequest mirrors the `bernie bench` flags. Absent fields take the CLI
// defaults; concurrency is a list or a comma string ("1,8").
type benchRequest struct {
	URL         string          `json:"url"`
	Model       string          `json:"model"`
	InputLen    *int            `json:"input_len"`
	OutputLen   *int            `json:"output_len"`
	Concurrency json.RawMessage `json:"concurrency"`
	NumPrompts  int             `json:"num_prompts"`
	Warmup      *int            `json:"warmup"`
	Seed        int64           `json:"seed"`
	Tag         string          `json:"tag"`
	Recall      bool            `json:"recall"`
}

func (b benchRequest) options() (bench.Options, error) {
	o := bench.Options{URL: b.URL, Model: b.Model, Tag: b.Tag, NumPrompts: b.NumPrompts, Seed: b.Seed,
		Recall:   b.Recall,
		InputLen: bench.DefaultInputLen, OutputLen: bench.DefaultOutputLen, Warmup: bench.DefaultWarmup}
	if b.InputLen != nil {
		o.InputLen = *b.InputLen
	}
	if b.OutputLen != nil {
		o.OutputLen = *b.OutputLen
	}
	if b.Warmup != nil {
		o.Warmup = *b.Warmup
	}
	conc, err := parseConcurrency(b.Concurrency)
	if err != nil {
		return o, err
	}
	o.Concurrency = conc
	if o.InputLen < 1 || o.OutputLen < 1 {
		return o, errors.New("input_len and output_len must be positive")
	}
	if o.NumPrompts < 0 || o.Warmup < 0 {
		return o, errors.New("num_prompts and warmup must not be negative")
	}
	return o, nil
}

func parseConcurrency(raw json.RawMessage) ([]int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []int{1}, nil
	}
	var list []int
	if err := json.Unmarshal(raw, &list); err != nil {
		var str string
		if json.Unmarshal(raw, &str) != nil {
			return nil, errors.New(`concurrency must be a list of integers or a string like "1,8"`)
		}
		for _, f := range strings.Split(str, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				return nil, fmt.Errorf("concurrency: %q is not a positive integer", f)
			}
			list = append(list, n)
		}
	}
	if len(list) == 0 {
		return []int{1}, nil
	}
	for _, n := range list {
		if n < 1 {
			return nil, fmt.Errorf("concurrency: %d is not a positive integer", n)
		}
	}
	return list, nil
}

// POST /api/bench — `bernie bench …`: runs, saves and returns the result.
// One benchmark runs at a time; a second request gets 409.
func (s *server) postBench(w http.ResponseWriter, r *http.Request) {
	var req benchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	o, err := req.options()
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, _, err := app.Bench(r.Context(), o, s.o.Log)
	var saveErr *app.SaveError
	switch {
	case errors.Is(err, app.ErrBenchBusy):
		httpError(w, http.StatusConflict, err.Error())
	case errors.Is(err, app.ErrAllFailed):
		httpError(w, http.StatusBadGateway, err.Error()+app.FirstCellError(res))
	case errors.As(err, &saveErr):
		httpError(w, http.StatusInternalServerError, err.Error())
	case err != nil:
		httpError(w, http.StatusBadGateway, err.Error())
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

// decodeBody reads a JSON body (max 1 MiB) with unknown fields rejected.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		httpError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeRedacted(w http.ResponseWriter, code int, v any) {
	g, err := redact.JSON(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, code, g)
}

func oneLine(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ") }
