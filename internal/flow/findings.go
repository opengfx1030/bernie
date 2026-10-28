package flow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// FindingsBudget bounds one scan of run logs. The scan of an exited run never
// holds up a caller (the wait loop of `up`, `ps`, `status`, the dashboard)
// for longer.
const FindingsBudget = 2 * time.Second

// Run is a run with its state, the known failures found in the log of an
// exited run (spec §5.3) and the phase the log of a live run names
// (spec §3.1).
type Run struct {
	runner.Status
	Findings []logdoc.Match `json:"findings,omitempty"`
	Phase    *logdoc.Phase  `json:"phase,omitempty"`
}

// StartupExit is the error of a run that exited before it answered /health.
// Findings are the known failures in its log; it is empty when the log
// matches no rule or could not be read.
type StartupExit struct {
	Name     string
	Findings []logdoc.Match
}

func (e *StartupExit) Error() string {
	return fmt.Sprintf("%s exited during startup; see: bernie logs %s", e.Name, e.Name)
}

var rules = sync.OnceValues(logdoc.Load)

// findingsCache holds the findings of exited runs. The log of an exited run
// does not change, so one scan serves every later call of the same run.
var findingsCache sync.Map // string -> []logdoc.Match

func findingsKey(r runner.Record) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", r.Name, r.StartedAt.Format(time.RFC3339), r.PID, r.Container, r.Log)
}

// Findings scans the log of an exited run with the embedded catalog and
// returns the matches in rule order. It gives up after FindingsBudget (or
// when ctx ends) and then returns what it found so far; an unreadable log or
// a broken catalog yields none.
func Findings(ctx context.Context, r runner.Record) []logdoc.Match {
	key := findingsKey(r)
	if v, ok := findingsCache.Load(key); ok {
		return v.([]logdoc.Match)
	}
	rs, err := rules()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, FindingsBudget)
	defer cancel()
	rc, err := runner.OpenLog(ctx, r, FindingsBudget)
	if err != nil {
		return nil
	}
	defer rc.Close()
	ms := logdoc.Scan(rc, rs)
	if ctx.Err() == nil { // a cut-off scan is not final
		findingsCache.Store(key, ms)
	}
	return ms
}

// Runs lists the runs like runner.List and adds the findings of every exited
// run. All scans share one FindingsBudget.
func Runs(ctx context.Context) ([]Run, error) {
	runs, err := runner.List(ctx)
	if err != nil {
		return nil, err
	}
	sctx, cancel := context.WithTimeout(ctx, FindingsBudget)
	defer cancel()
	pctx, pcancel := context.WithTimeout(ctx, PhaseBudget)
	defer pcancel()
	out := make([]Run, len(runs))
	for i, s := range runs {
		out[i].Status = s
		if s.State == runner.StateExited {
			out[i].Findings = Findings(sctx, s.Record)
			continue
		}
		if p, ok := Phase(pctx, s.Record); ok {
			out[i].Phase = &p
		}
	}
	return out, nil
}
