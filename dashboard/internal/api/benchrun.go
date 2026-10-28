package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/sse"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// benchPoll is the delay between two reads of a running benchmark.
const benchPoll = 300 * time.Millisecond

// The kinds and the states of a benchmark job.
const (
	benchKindRun = "run"
	benchKindAB  = "ab"

	benchRunning = "running"
	benchDone    = "done"
	benchFailed  = "failed"
)

// benchJobDoc is the answer of GET /api/bench/{id}: the state of the run.
type benchJobDoc struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Phase    string `json:"phase"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	ResultID string `json:"result_id"`
	Error    string `json:"error,omitempty"`
}

// benchJob is one benchmark running in the background. The job outlives the
// request that started it.
type benchJob struct {
	mu       sync.Mutex
	id       string
	kind     string
	status   string
	phase    string
	done     int
	total    int
	resultID string
	err      string
	lines    []string
	group    string
	level    int
	visit    int
	arm      string
}

// benchJobs holds the benchmark runs of this process.
type benchJobs struct {
	mu   sync.Mutex
	jobs map[string]*benchJob
}

// newBenchJobs returns an empty store of benchmark runs.
func newBenchJobs() *benchJobs { return &benchJobs{jobs: map[string]*benchJob{}} }

// add puts a job in the store.
func (s *benchJobs) add(j *benchJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.id] = j
}

// get returns the job with this id.
func (s *benchJobs) get(id string) (*benchJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// POST /api/bench/run — the same body as POST /api/bench, but the run
// happens in the background: the answer is the id of the job. The state of
// the run is GET /api/bench/{id}, its events are GET /api/bench/{id}/events.
func (h *handler) postBenchRun(w http.ResponseWriter, r *http.Request) {
	var req benchRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	o, err := req.options()
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	job := h.newJob(benchKindRun, benchCells(o))
	ctx := context.WithoutCancel(r.Context())
	go h.runBench(ctx, job, o)
	httpjson.Write(w, http.StatusAccepted, map[string]string{"id": job.id})
}

// abRequest is the body of POST /api/bench/ab: the two arms of the run.
type abRequest struct {
	Base       string `json:"base"`
	Cand       string `json:"cand"`
	Suite      string `json:"suite"`
	Model      string `json:"model"`
	NumPrompts int    `json:"num_prompts"`
	Seed       int64  `json:"seed"`
	Tag        string `json:"tag"`
}

// POST /api/bench/ab — a baseline and a candidate, one after the other: the
// same run as `bernie bench ab`. The answer is the id of the job.
func (h *handler) postBenchAB(w http.ResponseWriter, r *http.Request) {
	var req abRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if req.Base == "" || req.Cand == "" {
		httpjson.Error(w, http.StatusBadRequest, "base and cand must name the two arms of the run")
		return
	}
	for _, arm := range []string{req.Base, req.Cand} {
		if strings.HasPrefix(arm, "http://") || strings.HasPrefix(arm, "https://") {
			if !loopbackURL(arm) {
				httpjson.Error(w, http.StatusBadRequest, "the run only talks to a server of this host: "+arm)
				return
			}
		}
	}
	job := h.newJob(benchKindAB, len(suite.DefaultVisits))
	o := suite.ABOptions{
		Base: req.Base, Cand: req.Cand, Suite: req.Suite,
		Options: bench.Options{Model: req.Model, NumPrompts: req.NumPrompts, Seed: req.Seed, Tag: req.Tag},
	}
	ctx := context.WithoutCancel(r.Context())
	go h.runBenchAB(ctx, job, o)
	httpjson.Write(w, http.StatusAccepted, map[string]string{"id": job.id})
}

// GET /api/bench/{id} — the state of a benchmark run.
func (h *handler) getBenchJob(w http.ResponseWriter, r *http.Request) {
	job, ok := h.bench.get(r.PathValue("id"))
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no benchmark run named "+strconv.Quote(r.PathValue("id")))
		return
	}
	httpjson.Write(w, http.StatusOK, job.doc())
}

// GET /api/bench/{id}/events — the progress of a benchmark run: one "log"
// event per line, a "progress" event with the state and a last "done" or
// "failed" event with the job document.
func (h *handler) getBenchEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := h.bench.get(r.PathValue("id"))
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no benchmark run named "+strconv.Quote(r.PathValue("id")))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := sse.New(w)
	go sse.Heartbeat(ctx, st, cancel)
	sent := 0
	for {
		lines, doc, status := job.state()
		for ; sent < len(lines); sent++ {
			if st.Event("log", lines[sent]) != nil {
				return
			}
		}
		b, err := json.Marshal(doc)
		if err != nil {
			_ = st.Event("error", sse.OneLine(err.Error()))
			return
		}
		if st.Event("progress", string(b)) != nil {
			return
		}
		if status != benchRunning {
			name := benchDone
			if status == benchFailed {
				name = benchFailed
			}
			_ = st.Event(name, string(b))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(benchPoll):
		}
	}
}

// newJob puts a new benchmark run in the store and returns it.
func (h *handler) newJob(kind string, total int) *benchJob {
	j := &benchJob{id: newBenchID(), kind: kind, status: benchRunning, total: total}
	h.bench.add(j)
	return j
}

// runBench runs a benchmark in the background and records its end.
func (h *handler) runBench(ctx context.Context, job *benchJob, o bench.Options) {
	w := &benchProgress{job: job}
	var res *bench.Result
	var err error
	if o.Suite != "" {
		res, _, err = app.BenchSuite(ctx, o.Suite, o, w)
	} else {
		res, _, err = app.Bench(ctx, o, w)
	}
	switch {
	case errors.Is(err, app.ErrAllFailed):
		job.finish(benchFailed, errors.New(err.Error()+app.FirstCellError(res)))
	case err != nil:
		job.finish(benchFailed, err)
	default:
		if res != nil {
			job.setResult(res.ID)
		}
		job.finish(benchDone, nil)
	}
}

// runBenchAB runs an A/B comparison in the background and records its end.
func (h *handler) runBenchAB(ctx context.Context, job *benchJob, o suite.ABOptions) {
	o.Progress = &benchProgress{job: job}
	res, paths, err := app.BenchAB(ctx, o)
	if err != nil {
		job.finish(benchFailed, err)
		return
	}
	if len(paths) > 0 {
		job.setResult(resultIDOf(res))
	}
	job.finish(benchDone, nil)
}

// resultIDOf returns the id of the first result of an A/B run.
func resultIDOf(res *suite.ABResult) string {
	if res == nil {
		return ""
	}
	for _, v := range res.Visits {
		if v.Result != nil && v.Result.ID != "" {
			return v.Result.ID
		}
	}
	return ""
}

// benchProgress writes the progress of a benchmark into its job.
type benchProgress struct {
	job *benchJob
}

// Write takes the progress of a benchmark.
func (p *benchProgress) Write(b []byte) (int, error) {
	p.job.addLines(string(b))
	return len(b), nil
}

// addLines records the progress lines of a run and updates its phase.
func (j *benchJob) addLines(text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			continue
		}
		j.lines = append(j.lines, line)
		j.progress(line)
	}
}

// progress reads one progress line and updates the phase of the run.
func (j *benchJob) progress(line string) {
	if tag, ok := between(line, "bench: visit ", ""); ok {
		j.visit++
		j.arm, _, _ = strings.Cut(tag, ":")
		if i := strings.LastIndex(tag, ":"); i >= 0 {
			if n, err := strconv.Atoi(tag[i+1:]); err == nil {
				j.visit = n
			}
		}
		j.done = j.visit - 1
		j.phase = fmt.Sprintf("visit %d of %d: %s", j.visit, j.total, j.arm)
		return
	}
	if g, ok := between(line, " group ", ":"); ok {
		j.group = g
	}
	if c, ok := between(line, "c=", " "); ok {
		if n, err := strconv.Atoi(c); err == nil {
			j.level = n
		}
	}
	if strings.Contains(line, " done in ") {
		j.done++
	}
	j.phase = j.cellPhase()
}

// cellPhase names the cell the run is on.
func (j *benchJob) cellPhase() string {
	cell := j.done + 1
	if cell > j.total {
		cell = j.total
	}
	if j.group == "" {
		return fmt.Sprintf("cell %d of %d at concurrency %d", cell, j.total, j.level)
	}
	return fmt.Sprintf("cell %d of %d: %s at concurrency %d", cell, j.total, j.group, j.level)
}

// doc returns the state of the job.
func (j *benchJob) doc() benchJobDoc {
	j.mu.Lock()
	defer j.mu.Unlock()
	return benchJobDoc{
		ID: j.id, Status: j.status, Phase: j.phase, Done: j.done, Total: j.total,
		ResultID: j.resultID, Error: j.err,
	}
}

// state returns the lines of the job, its document and its status.
func (j *benchJob) state() ([]string, benchJobDoc, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines := make([]string, len(j.lines))
	copy(lines, j.lines)
	doc := benchJobDoc{
		ID: j.id, Status: j.status, Phase: j.phase, Done: j.done, Total: j.total,
		ResultID: j.resultID, Error: j.err,
	}
	return lines, doc, j.status
}

// setResult records the id of the result the run saved.
func (j *benchJob) setResult(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.resultID = id
}

// finish records the end of the run.
func (j *benchJob) finish(status string, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = status
	if err != nil {
		j.err = err.Error()
	}
	if status == benchDone {
		j.phase = "the run is done"
		j.done = j.total
	}
}

// benchCells is the number of cells a run measures.
func benchCells(o bench.Options) int {
	if o.Suite != "" {
		s, err := suite.Get(o.Suite)
		if err != nil {
			return 0
		}
		n := 0
		for _, g := range s.Groups {
			n += g.Reps * len(g.Concurrency)
		}
		return n
	}
	return len(o.Concurrency)
}

// between returns the text between a marker and the next occurrence of
// another one.
func between(line, from, to string) (string, bool) {
	i := strings.Index(line, from)
	if i < 0 {
		return "", false
	}
	rest := line[i+len(from):]
	if to == "" {
		return rest, true
	}
	j := strings.Index(rest, to)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// newBenchID returns the id of a benchmark run.
func newBenchID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "b-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 16)
	}
	return "b-" + hex.EncodeToString(b)
}

// writeBenchMarkdown writes the receipt of a comparison as markdown.
func writeBenchMarkdown(w http.ResponseWriter, v *suite.Verdict) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, suite.Receipt(v))
}
