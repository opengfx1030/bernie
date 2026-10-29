package suite

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// The arms of an A/B run.
const (
	ArmBase = "base"
	ArmCand = "cand"
)

// DefaultVisits is the order of the visits: the arms alternate, so a
// linear drift of the machine hits both arms the same way.
var DefaultVisits = []string{ArmBase, ArmCand, ArmCand, ArmBase}

// ABOptions are the options of an A/B run.
type ABOptions struct {
	// Base and Cand are the arms: a recipe id or the URL of a server.
	Base string
	Cand string
	// Suite is the suite every visit runs. Empty means the cells of
	// Options.
	Suite string
	// Visits is the order of the visits. The default is base, cand, cand,
	// base.
	Visits []string
	// Options are the bench options of a visit.
	Options bench.Options
	// WaitS is how long a started server may take to answer.
	WaitS int
	// Band replaces the noise band of the suite.
	Band float64
	// Progress is where the progress lines go.
	Progress io.Writer
	// Starter starts a recipe arm. It is required for a recipe arm.
	Starter Starter
}

// Starter starts the server of a recipe arm. The shared app operation
// supplies it.
type Starter func(context.Context, string, string) (*runner.Record, error)

// ABResult is the result of an A/B run.
type ABResult struct {
	// Visits holds the visits in the order they ran.
	Visits []ABVisit `json:"visits"`
	// Verdict is the verdict of the two arms.
	Verdict *Verdict `json:"verdict,omitempty"`
}

// ABVisit is one visit of an A/B run.
type ABVisit struct {
	// Arm is the arm of the visit: base or cand.
	Arm string `json:"arm"`
	// Index is the number of the visit, starting at one.
	Index int `json:"index"`
	// Tag labels the result of the visit.
	Tag string `json:"tag"`
	// Result is what the visit measured.
	Result *bench.Result `json:"result"`
}

// AB runs the visits of an A/B comparison and returns their results with
// the verdict of the two arms.
//
// One server runs at a time: when an arm is a recipe, its server starts
// before the visit and stops after it.
func AB(ctx context.Context, o ABOptions) (*ABResult, error) {
	if o.Progress == nil {
		o.Progress = io.Discard
	}
	if o.Base == "" || o.Cand == "" {
		return nil, fmt.Errorf("an A/B run needs a base arm and a candidate arm")
	}
	visits := o.Visits
	if len(visits) == 0 {
		visits = DefaultVisits
	}
	res := &ABResult{}
	for i, arm := range visits {
		switch arm {
		case ArmBase, ArmCand:
		default:
			return nil, fmt.Errorf("the visit %d names the unknown arm %q (base or cand)", i+1, arm)
		}
		bench.SendEvent(o.Progress, bench.Event{Kind: bench.EventVisit, Arm: arm, Index: i + 1, Total: len(visits)})
		recipe := o.Base
		if arm == ArmCand {
			recipe = o.Cand
		}
		url, stop, err := armServer(ctx, arm, recipe, o)
		if err != nil {
			return nil, err
		}
		opts := o.Options
		opts.URL = url
		opts.Tag = fmt.Sprintf("ab:%s:%d", arm, i+1)
		r, err := runVisit(ctx, o, opts)
		stop()
		if err != nil {
			return nil, err
		}
		res.Visits = append(res.Visits, ABVisit{Arm: arm, Index: i + 1, Tag: opts.Tag, Result: r})
	}
	base, cand := mergeArm(res.Visits, ArmBase), mergeArm(res.Visits, ArmCand)
	if base != nil && cand != nil {
		cmp := &Comparison{Workload: *base.Workload, A: base, B: cand}
		res.Verdict = Judge(cmp, o.Band)
	}
	return res, nil
}

// runVisit runs the cells of one visit.
func runVisit(ctx context.Context, o ABOptions, opts bench.Options) (*bench.Result, error) {
	fmt.Fprintf(o.Progress, "bench: visit %s\n", opts.Tag)
	if o.Suite == "" {
		return bench.Run(ctx, opts, o.Progress)
	}
	s, err := Get(o.Suite)
	if err != nil {
		return nil, err
	}
	return Run(ctx, s, opts, o.Progress)
}

// armServer returns the URL of the server of an arm. When the arm is a
// recipe, its server starts and the returned function stops it.
func armServer(ctx context.Context, arm, spec string, o ABOptions) (string, func(), error) {
	stop := func() {}
	if isURL(spec) {
		return spec, stop, nil
	}
	name := "ab-" + arm
	if o.Starter == nil {
		return "", stop, fmt.Errorf("the A/B run has no starter for the recipe %s", spec)
	}
	rec, err := o.Starter(ctx, spec, name)
	if err != nil {
		return "", stop, err
	}
	stop = func() { _ = runner.Stop(context.WithoutCancel(ctx), rec.Name) }
	url := rec.URL()
	if err := waitHealthy(ctx, url, o.WaitS); err != nil {
		stop()
		return "", stop, err
	}
	return url, stop, nil
}

// isURL reports whether a spec names a server instead of a recipe.
func isURL(spec string) bool {
	return strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://")
}

// waitHealthy waits until the server answers its health check.
func waitHealthy(ctx context.Context, url string, seconds int) error {
	if seconds <= 0 {
		seconds = 600
	}
	client := vllm.New(url)
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		if err := client.Health(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server %s did not answer within %d s", url, seconds)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// mergeArm merges the visits of one arm into one result. Its cells hold
// the medians over the visits, so the verdict sees the same shape as a
// plain run.
func mergeArm(visits []ABVisit, arm string) *bench.Result {
	type key struct {
		group string
		level int
	}
	cells := map[key][]bench.Cell{}
	var order []key
	var out *bench.Result
	for _, v := range visits {
		if v.Arm != arm || v.Result == nil {
			continue
		}
		if out == nil {
			out = &bench.Result{ID: arm, Model: v.Result.Model, Workload: v.Result.Workload}
		}
		for _, c := range v.Result.Cells {
			k := key{c.Group, c.Concurrency}
			if _, ok := cells[k]; !ok {
				order = append(order, k)
			}
			cells[k] = append(cells[k], c)
		}
	}
	if out == nil {
		return nil
	}
	for _, k := range order {
		reps := cells[k]
		cell := medianCell(reps)
		for _, r := range reps {
			cell.Reps = append(cell.Reps, repOf(r))
		}
		cell.Spread = mergeSpread(reps)
		out.Cells = append(out.Cells, cell)
	}
	return out
}

// mergeSpread is the spread of an arm: the spread of the visit medians and
// the worst spread of its visits.
func mergeSpread(visits []bench.Cell) map[string]float64 {
	out := spreadOf(visits)
	for _, v := range visits {
		for name, s := range v.Spread {
			if cur, ok := out[name]; !ok || s < cur {
				out[name] = s
			}
		}
	}
	return out
}
