// Package job owns the benchmark jobs of bernie (spec §11): the store of
// the jobs, their state machine, the busy check and the typed progress
// events. The front ends call the store and render its answers; they hold
// no state of a job.
package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// The kinds of a job.
const (
	// KindRun is a plain benchmark run.
	KindRun = "run"
	// KindAB is an A/B comparison.
	KindAB = "ab"
)

// The states of a job.
const (
	// StatusRunning means the job runs now.
	StatusRunning = "running"
	// StatusDone means the job finished with a result.
	StatusDone = "done"
	// StatusFailed means the job ended with an error.
	StatusFailed = "failed"
)

// MaxFinished is the number of finished jobs the store keeps. The oldest
// finished job is dropped when a new one finishes.
const MaxFinished = 32

// Request is what a job needs to run.
type Request struct {
	// Kind is the kind of the job: KindRun or KindAB.
	Kind string
	// Options are the flags of a plain run.
	Options bench.Options
	// AB are the two arms of an A/B run.
	AB suite.ABOptions
	// Total is the number of work units of the job. Zero asks the store to
	// count them from the request.
	Total int
	// Run does the work of the job and returns the id of the result it
	// saved. It is nil for the built-in runs: the store runs the benchmark
	// of the request then.
	Run func(ctx context.Context, p *Progress) (string, error)
}

// Doc is the state of a job: the answer of GET /api/bench/{id}.
type Doc struct {
	ID       string      `json:"id"`
	Status   string      `json:"status"`
	Phase    string      `json:"phase"`
	Event    bench.Event `json:"event"`
	Done     int         `json:"done"`
	Total    int         `json:"total"`
	ResultID string      `json:"result_id"`
	Error    string      `json:"error,omitempty"`
}

// Store holds the benchmark jobs of this process.
type Store struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

// NewStore returns an empty store of benchmark jobs.
func NewStore() *Store { return &Store{jobs: map[string]*Job{}} }

// Start puts a new job in the store and runs it in the background. One job
// runs at a time; a second call gets app.ErrBenchBusy: the busy answer of
// every front end that starts a benchmark.
func (s *Store) Start(ctx context.Context, req Request) (*Job, error) {
	s.mu.Lock()
	if s.running() {
		s.mu.Unlock()
		return nil, app.ErrBenchBusy
	}
	j := &Job{
		id:        newID(),
		kind:      req.Kind,
		status:    StatusRunning,
		total:     req.total(),
		startedAt: time.Now().UTC(),
	}
	s.jobs[j.id] = j
	s.mu.Unlock()
	go s.run(context.WithoutCancel(ctx), j, req)
	return j, nil
}

// Get returns the job with this id.
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// List returns the jobs of the store, newest first.
func (s *Store) List() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].startedAt.After(out[k].startedAt) })
	return out
}

// running says whether a job of the store runs now.
func (s *Store) running() bool {
	for _, j := range s.jobs {
		if j.Status() == StatusRunning {
			return true
		}
	}
	return false
}

// run does the work of a job and records its end. The store drops the
// oldest finished job when it keeps too many of them.
func (s *Store) run(ctx context.Context, j *Job, req Request) {
	p := &Progress{job: j}
	id, res, err := j.work(ctx, req, p)
	switch {
	case errors.Is(err, app.ErrAllFailed):
		j.finish(StatusFailed, errors.New(err.Error()+app.FirstCellError(res)))
	case err != nil:
		j.finish(StatusFailed, err)
	default:
		j.setResult(id)
		j.finish(StatusDone, nil)
	}
	s.evict()
}

// evict drops the oldest finished jobs until at most MaxFinished of them
// are left. The store calls it when a job finishes.
func (s *Store) evict() {
	s.mu.Lock()
	defer s.mu.Unlock()
	finished := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		if _, ok := j.finishedTime(); ok {
			finished = append(finished, j)
		}
	}
	if len(finished) <= MaxFinished {
		return
	}
	sort.Slice(finished, func(i, k int) bool {
		a, _ := finished[i].finishedTime()
		b, _ := finished[k].finishedTime()
		if !a.Equal(b) {
			return a.Before(b)
		}
		return finished[i].id < finished[k].id
	})
	for _, j := range finished[:len(finished)-MaxFinished] {
		delete(s.jobs, j.id)
	}
}

// Progress is where a job takes the progress of its run: the text lines
// and the typed events. The lines go to the log of the job, the events
// move its state.
type Progress struct {
	job *Job
}

// Write takes the text of a progress line.
func (p *Progress) Write(b []byte) (int, error) {
	p.job.addLines(string(b))
	return len(b), nil
}

// Event takes a typed progress event.
func (p *Progress) Event(e bench.Event) { p.job.apply(e) }

// total is the number of work units of the request: the cells of a run,
// the cells of a suite run or the visits of an A/B run.
func (r Request) total() int {
	if r.Total > 0 {
		return r.Total
	}
	switch r.Kind {
	case KindAB:
		if len(r.AB.Visits) > 0 {
			return len(r.AB.Visits)
		}
		return len(suite.DefaultVisits)
	}
	if r.Options.Suite != "" {
		s, err := suite.Get(r.Options.Suite)
		if err != nil {
			return 0
		}
		n := 0
		for _, g := range s.Groups {
			n += g.Reps * len(g.Concurrency)
		}
		return n
	}
	return len(r.Options.Concurrency)
}

// resultID returns the id of the first result of an A/B run.
func resultID(res *suite.ABResult) string {
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

// splitLines returns the lines of a text without its empty lines.
func splitLines(text string) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimRight(line, "\r "); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// newID returns the id of a job: "b-" and four random bytes.
func newID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "b-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 16)
	}
	return "b-" + hex.EncodeToString(b)
}
