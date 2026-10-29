package job

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// Job is one benchmark running in the background. The job outlives the
// request that started it.
type Job struct {
	mu         sync.Mutex
	id         string
	kind       string
	status     string
	phase      string
	event      bench.Event
	done       int
	total      int
	resultID   string
	err        string
	group      string
	level      int
	arm        string
	lines      []string
	startedAt  time.Time
	finishedAt time.Time
	// cancel stops the running job. Set by the store in Start; cleared
	// by run when the work ends. A job of another process has no cancel
	// here.
	cancel context.CancelFunc
	// Incomplete is true when the job ended without measuring every cell
	// of its run (a cancellation, a network failure, an interrupted
	// download): the partial result is kept for the record but the row
	// in the history tells the reader why.
	Incomplete bool
}

// ID returns the id of the job.
func (j *Job) ID() string { return j.id }

// Status returns the state of the job: running, done or failed.
func (j *Job) Status() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// Doc returns the state of the job.
func (j *Job) Doc() Doc {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.doc()
}

// State returns the lines of the job, its document and its status.
func (j *Job) State() ([]string, Doc, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines := make([]string, len(j.lines))
	copy(lines, j.lines)
	return lines, j.doc(), j.status
}

// doc returns the state of the job. The caller holds the lock.
func (j *Job) doc() Doc {
	return Doc{
		ID: j.id, Status: j.status, Phase: j.phase, Event: j.event,
		Done: j.done, Total: j.total, ResultID: j.resultID, Error: j.err,
		Incomplete: j.Incomplete,
	}
}

// work does the work of the job: it runs the benchmark of the request and
// returns the id of the result it saved.
func (j *Job) work(ctx context.Context, req Request, p *Progress) (string, *bench.Result, error) {
	if req.Run != nil {
		id, err := req.Run(ctx, p)
		return id, nil, err
	}
	switch req.Kind {
	case KindAB:
		o := req.AB
		o.Progress = p
		res, paths, err := app.BenchAB(ctx, o)
		if err != nil {
			return "", nil, err
		}
		if len(paths) == 0 {
			return "", nil, nil
		}
		return resultID(res), nil, nil
	default:
		var res *bench.Result
		var err error
		if req.Options.Suite != "" {
			res, _, err = app.BenchSuite(ctx, req.Options.Suite, req.Options, p)
		} else {
			res, _, err = app.Bench(ctx, req.Options, p)
		}
		if err != nil {
			return "", res, err
		}
		if res == nil {
			return "", nil, nil
		}
		return res.ID, res, nil
	}
}

// setResult records the id of the result the run saved.
func (j *Job) setResult(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.resultID = id
}

// finish records the end of the run.
func (j *Job) finish(status string, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = status
	j.finishedAt = time.Now().UTC()
	if err != nil {
		j.err = err.Error()
	}
	if status == StatusDone {
		j.phase = "the run is done"
		j.done = j.total
	}
}

// finishedTime returns the time the job finished and whether it finished.
func (j *Job) finishedTime() (time.Time, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status == StatusRunning {
		return time.Time{}, false
	}
	return j.finishedAt, true
}

// addLines records the progress lines of a run.
func (j *Job) addLines(text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lines = append(j.lines, splitLines(text)...)
}

// apply records a typed event: it moves the phase of the job and its
// counters. The text of the run is not read for the state.
func (j *Job) apply(e bench.Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.event = e
	switch e.Kind {
	case bench.EventVisit:
		j.arm = e.Arm
		j.done = e.Index - 1
		if e.Total > 0 {
			j.total = e.Total
		}
		j.phase = fmt.Sprintf("visit %d of %d: %s", e.Index, j.total, j.arm)
		return
	case bench.EventGroup:
		j.group = e.Group
		if j.kind == KindAB {
			return
		}
	case bench.EventCell:
		j.level = e.Level
		if j.kind == KindAB {
			return
		}
		j.done++
	}
	j.phase = j.cellPhase()
}

// cellPhase names the cell the run is on.
func (j *Job) cellPhase() string {
	cell := j.done + 1
	if cell > j.total {
		cell = j.total
	}
	if j.group == "" {
		return fmt.Sprintf("cell %d of %d at concurrency %d", cell, j.total, j.level)
	}
	return fmt.Sprintf("cell %d of %d: %s at concurrency %d", cell, j.total, j.group, j.level)
}
