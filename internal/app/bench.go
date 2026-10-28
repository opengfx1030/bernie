package app

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// ErrAllFailed means every measured request of a benchmark failed. Bench
// returns it together with the unsaved result.
var ErrAllFailed = errors.New("all requests failed; result not saved")

// SaveError is a benchmark that ran but could not be stored.
type SaveError struct{ Err error }

func (e *SaveError) Error() string { return "save result: " + e.Err.Error() }
func (e *SaveError) Unwrap() error { return e.Err }

// ErrBenchBusy means another benchmark of this process is running.
var ErrBenchBusy = errors.New("a benchmark is already running")

var benchMu sync.Mutex // one benchmark at a time per process

// Bench runs a benchmark and saves the result under <state>/bench. Progress
// lines go to progress. It returns the result and the file it was saved to.
// When no request succeeded it saves nothing and returns the result with
// ErrAllFailed. A second concurrent call returns ErrBenchBusy.
func Bench(ctx context.Context, o bench.Options, progress io.Writer) (*bench.Result, string, error) {
	if !benchMu.TryLock() {
		return nil, "", ErrBenchBusy
	}
	defer benchMu.Unlock()
	r, err := bench.Run(ctx, o, progress)
	if err != nil {
		return nil, "", err
	}
	if r.Completed() == 0 {
		return r, "", ErrAllFailed
	}
	path, err := bench.Save(r)
	if err != nil {
		return r, "", &SaveError{err}
	}
	return r, path, nil
}

// FirstCellError is ": <message>" of the first error sample of a result, or
// "" when no cell has one.
func FirstCellError(r *bench.Result) string {
	for _, c := range r.Cells {
		if len(c.Errors) > 0 {
			return ": " + c.Errors[0]
		}
	}
	return ""
}

// BenchList is the `bench list --json` document: saved results, never null.
func BenchList() ([]bench.Result, error) {
	rs, err := bench.List()
	if rs == nil {
		rs = []bench.Result{}
	}
	return rs, err
}
