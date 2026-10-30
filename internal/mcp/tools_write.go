package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/netx"
)

const twoCalls = " Two calls: the first, without confirm_token, returns the plan and a confirm_token; repeat the same arguments with the token to run it."

// confirmToken is the field every mutating tool ends with.
type confirmToken struct {
	ConfirmToken string `json:"confirm_token,omitempty" jsonschema:"token from the plan call; omit it to get a plan"`
}

func tokenSchema(s *jsonschema.Schema) {
	t := prop(s, "confirm_token")
	t.MinLength, t.MaxLength = jsonschema.Ptr(32), jsonschema.Ptr(128)
}

type serveIn struct {
	launchIn
	NoPull bool `json:"no_pull,omitempty" jsonschema:"container mode: start the local image without pulling"`
	confirmToken
}

type stopIn struct {
	Name string `json:"name" jsonschema:"name of the run to stop (ps)"`
	confirmToken
}

type benchRunIn struct {
	URL         string `json:"url,omitempty" jsonschema:"base URL of the server; loopback only (default: the only run, else http://127.0.0.1:8000)"`
	Model       string `json:"model,omitempty" jsonschema:"model name (default: the first id of /v1/models)"`
	InputLen    int    `json:"input_len,omitempty" jsonschema:"prompt tokens (default 1024)"`
	OutputLen   int    `json:"output_len,omitempty" jsonschema:"output tokens (default 512)"`
	Concurrency []int  `json:"concurrency,omitempty" jsonschema:"concurrency levels, one cell each (default [1])"`
	NumPrompts  int    `json:"num_prompts,omitempty" jsonschema:"requests per cell (default max(4, 2 x concurrency))"`
	Warmup      *int   `json:"warmup,omitempty" jsonschema:"requests sent and discarded before each cell (default 1)"`
	Seed        int64  `json:"seed,omitempty" jsonschema:"prompt seed"`
	Tag         string `json:"tag,omitempty" jsonschema:"free label stored with the result"`
	Recall      bool   `json:"recall,omitempty" jsonschema:"hide a short random fact in every prompt and check that the reply holds it (default off)"`
	// Suite names a built-in suite. The suite decides the cells and the
	// cell flags above are ignored.
	Suite string `json:"suite,omitempty" jsonschema:"name of the built-in suite to run (default: none, bernie bench suites lists them)"`
	confirmToken
}

type setupIn struct {
	Mode  string   `json:"mode,omitempty" jsonschema:"host or container (default: config mode, else both)"`
	Venv  string   `json:"venv,omitempty" jsonschema:"venv directory for host mode"`
	Image string   `json:"image,omitempty" jsonschema:"image for container mode"`
	Model string   `json:"model,omitempty" jsonschema:"Hugging Face repo to download (adds the model.download step)"`
	Steps []string `json:"steps,omitempty" jsonschema:"run only these step ids (setup_apply plan lists them)"`
	confirmToken
}

// setupVerifyIn is the input of setup_verify: the venv to check and the
// mode of the plan. The check needs a host plan.
type setupVerifyIn struct {
	Mode string `json:"mode,omitempty" jsonschema:"host or container (default: config mode, else both)"`
	Venv string `json:"venv,omitempty" jsonschema:"venv directory for host mode"`
	confirmToken
}

type powerIn struct {
	Watts      float64 `json:"watts" jsonschema:"power cap in watts"`
	GPUIndexes []int   `json:"gpu_indexes,omitempty" jsonschema:"HIP indexes (default: all GPUs)"`
	confirmToken
}

// stopPlan is what stop would act on.
type stopPlan struct {
	Name      string `json:"name"`
	Recipe    string `json:"recipe"`
	State     string `json:"state"`
	Mode      string `json:"mode"`
	PID       int    `json:"pid"`
	Container string `json:"container"`
	URL       string `json:"url"`
}

// setupPlanDoc is what setup_apply would run, and what the operator must run.
type setupPlanDoc struct {
	Run    []setup.Step `json:"run"`    // steps bernie runs after confirmation
	Manual []setup.Step `json:"manual"` // root steps: commands for the operator, never run
}

// setupRunResult is what setup_apply and setup_verify return when they
// ran: the plan as it stands and the record of the run (spec §6.4). The id
// of the run is the handle of the report: setup_report reads it back.
type setupRunResult struct {
	Steps []setup.Step `json:"steps"`
	Run   *setup.Run   `json:"run,omitempty"`
	RunID string       `json:"run_id,omitempty"`
}

// powerPlan is what tune_power_set would write.
type powerPlan struct {
	File       string             `json:"file"`
	Watts      float64            `json:"watts"`
	GPUIndexes []int              `json:"gpu_indexes"`
	Desired    map[string]float64 `json:"desired"`
}

func registerWrite(s *Server) {
	w := func(name, desc string, destructive, idempotent, openWorld bool) spec {
		return spec{name: name, desc: desc + twoCalls, write: true, destructive: destructive, idempotent: idempotent, openWorld: openWorld}
	}

	addTool[serveIn, Outcome[runner.Record]](s, w("serve", "Start a run of a recipe (bernie serve). It returns when the process or container has started, not when /health answers; poll ps.", false, false, true),
		func(sc *jsonschema.Schema) { launchSchema(sc); tokenSchema(sc) }, s.serve)
	addTool[stopIn, Outcome[app.StopResult]](s, w("stop", "Stop a run (bernie stop NAME).", true, true, false),
		func(sc *jsonschema.Schema) { patternOf("name")(sc); tokenSchema(sc) }, s.stop)
	addTool[benchRunIn, Outcome[bench.Result]](s, w("bench_run", "Benchmark a loopback OpenAI-compatible server and save the result (bernie bench). One benchmark at a time; progress per phase. With recall, every prompt hides a short random fact and the reply is checked for it.", false, false, true),
		func(sc *jsonschema.Schema) {
			for _, n := range []string{"input_len", "output_len", "warmup"} {
				prop(sc, n).Minimum = jsonschema.Ptr(0.0)
			}
			prop(sc, "num_prompts").Minimum = jsonschema.Ptr(0.0)
			prop(sc, "concurrency").Items.Minimum = jsonschema.Ptr(1.0)
			tokenSchema(sc)
		}, s.benchRun)
	addTool[benchABIn, Outcome[suite.ABResult]](s, w("bench_ab", "Run a baseline and a candidate back to back (bernie bench ab) and tell the verdict. The visits run in the order base, cand, cand, base and one server runs at a time.", false, false, true),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.benchAB)
	addTool[setupIn, Outcome[setupRunResult]](s, w("setup_apply", "Run the install steps that need no root (bernie setup apply). Steps that need root are returned as commands for the operator. It returns the id of the run: setup_report reads its record.", true, false, true),
		func(sc *jsonschema.Schema) { prop(sc, "mode").Enum = enum("host", "container"); tokenSchema(sc) }, s.setupApply)
	addTool[setupVerifyIn, Outcome[setupRunResult]](s, w("setup_verify", "Check the venv again (bernie setup verify): the versions of the main parts, the fork ops and the libraries. It runs the check even when it passed before, it starts the Python of the venv, and it records a run with the new evidence.", false, false, true),
		func(sc *jsonschema.Schema) { prop(sc, "mode").Enum = enum("host", "container"); tokenSchema(sc) }, s.setupVerify)
	addTool[powerIn, Outcome[app.PowerDesiredResult]](s, w("tune_power_set", "Write the desired GPU power cap (bernie tune power set). It changes the desired-state file; it never writes hwmon.", false, true, false),
		func(sc *jsonschema.Schema) {
			prop(sc, "watts").ExclusiveMinimum = jsonschema.Ptr(0.0)
			g := prop(sc, "gpu_indexes")
			g.Items.Minimum, g.Items.UniqueItems = jsonschema.Ptr(0.0), true
			tokenSchema(sc)
		}, s.powerSet)

	addTool[fleetAddIn, Outcome[config.FleetEntry]](s, w("fleet_add", "Add one peer to the fleet (bernie fleet add). The token, when given, is stored in the config file with 0600 and never appears in the answer.", false, false, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.fleetAdd)

	addTool[fleetRmIn, Outcome[map[string]string]](s, w("fleet_rm", "Forget one peer of the fleet (bernie fleet rm). The token goes with it.", true, true, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.fleetRm)

	registerJobTools(s)
}

// planOf is the plan of a recipe on this host. The tests count the calls: a
// call that the gate refuses must not plan.
var planOf = app.Plan

func (s *Server) serve(ctx context.Context, c *call, in serveIn) (any, error) {
	// The plan is computed once: the digest the token is bound to and the run
	// use the same plan.
	var p *recipe.Plan
	plan := func() (any, error) {
		var err error
		if p, err = planOf(ctx, in.prepare()); err != nil {
			return nil, err
		}
		return p, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[runner.Record]{tool: "serve", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(ctx context.Context) (*runner.Record, error) {
			// The run outlives this request: only a cancel before the start aborts it.
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return app.Serve(context.WithoutCancel(ctx), app.ServeOptions{
				Prepared: p,
				Start:    runner.StartOptions{Venv: in.Venv, Engine: in.Engine, NoPull: in.NoPull, Progress: c.progress()},
			})
		}})
}

func (s *Server) stop(ctx context.Context, c *call, in stopIn) (any, error) {
	var plan *stopPlan
	build := func() (any, error) {
		runs, err := runner.List(ctx)
		if err != nil {
			return nil, err
		}
		plan = nil
		for _, r := range runs {
			if r.Name == in.Name {
				plan = &stopPlan{Name: r.Name, Recipe: r.Recipe, State: r.State, Mode: r.Mode, PID: r.PID, Container: r.Container, URL: r.URL()}
			}
		}
		if plan == nil {
			return nil, fmt.Errorf("no run named %q (ps lists the runs)", in.Name)
		}
		return plan, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[app.StopResult]{tool: "stop", token: token, args: in, plan: build, lock: &s.mutate,
		run: func(ctx context.Context) (*app.StopResult, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			r, err := app.Stop(context.WithoutCancel(ctx), in.Name)
			return &r, err
		}})
}

func (s *Server) benchRun(_ context.Context, c *call, in benchRunIn) (any, error) {
	o := bench.Options{URL: in.URL, Model: in.Model, Tag: in.Tag, InputLen: in.InputLen, OutputLen: in.OutputLen,
		Concurrency: in.Concurrency, NumPrompts: in.NumPrompts, Seed: in.Seed, Recall: in.Recall, Warmup: bench.DefaultWarmup}
	if in.Warmup != nil {
		o.Warmup = *in.Warmup
	}
	if in.Suite != "" {
		// The suite decides the cells, so the cell flags do not apply.
		o.Suite = in.Suite
		o.InputLen, o.OutputLen = 0, 0
		o.Concurrency = nil
		o.Recall = false
	} else {
		var err error
		o, err = bench.Normalize(o)
		if err != nil {
			return nil, err
		}
	}
	if o.URL == "" {
		o.URL = bench.DefaultURL()
	}
	if err := loopbackURL(o.URL); err != nil {
		return nil, err
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[bench.Result]{tool: "bench_run", token: token, args: o, plan: func() (any, error) { return o, nil }, lock: &s.benchM,
		busy: app.ErrBenchBusy,
		run: func(ctx context.Context) (*bench.Result, error) {
			if o.Suite != "" {
				r, _, err := app.BenchSuite(ctx, o.Suite, o, c.progress())
				if errors.Is(err, app.ErrAllFailed) {
					return nil, fmt.Errorf("%w%s", err, app.FirstCellError(r))
				}
				return r, err
			}
			r, _, err := app.Bench(ctx, o, c.progress())
			if errors.Is(err, app.ErrAllFailed) {
				return nil, fmt.Errorf("%w%s", err, app.FirstCellError(r))
			}
			return r, err
		}})
}

// benchABIn is the input of the A/B run: two arms and the suite they run.
type benchABIn struct {
	Base       string `json:"base" jsonschema:"the baseline arm: a recipe id or the URL of a server"`
	Cand       string `json:"cand" jsonschema:"the candidate arm: a recipe id or the URL of a server"`
	Suite      string `json:"suite,omitempty" jsonschema:"the suite every visit runs (default: the cells of the flags)"`
	Model      string `json:"model,omitempty" jsonschema:"model name (default: first id of /v1/models)"`
	NumPrompts int    `json:"num_prompts,omitempty" jsonschema:"requests per cell (default max(4, 2 x concurrency))"`
	Seed       int64  `json:"seed,omitempty" jsonschema:"prompt seed"`
	Tag        string `json:"tag,omitempty" jsonschema:"free label stored with the result"`
	WaitS      int    `json:"wait_s,omitempty" jsonschema:"seconds a started server may take to answer (default 600)"`
	confirmToken
}

func (s *Server) benchAB(_ context.Context, c *call, in benchABIn) (any, error) {
	if in.Base == "" || in.Cand == "" {
		return nil, fmt.Errorf("bench_ab needs a base arm and a candidate arm")
	}
	for _, arm := range []string{in.Base, in.Cand} {
		if strings.HasPrefix(arm, "http://") || strings.HasPrefix(arm, "https://") {
			if err := loopbackURL(arm); err != nil {
				return nil, err
			}
		}
	}
	o := suite.ABOptions{
		Base: in.Base, Cand: in.Cand, Suite: in.Suite, WaitS: in.WaitS,
		Options: bench.Options{Model: in.Model, NumPrompts: in.NumPrompts, Seed: in.Seed, Tag: in.Tag},
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[suite.ABResult]{tool: "bench_ab", token: token, args: o, plan: func() (any, error) { return o, nil }, lock: &s.benchM,
		busy: app.ErrBenchBusy,
		run: func(ctx context.Context) (*suite.ABResult, error) {
			o.Progress = c.progress()
			res, _, err := app.BenchAB(ctx, o)
			return res, err
		}})
}

// loopbackURL accepts only an http(s) URL on this host: a benchmark sends
// load, so the tool never aims it at another machine.
func loopbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("url %q is not an http(s) URL", raw)
	}
	h := u.Hostname()
	if !netx.Loopback(h) {
		return fmt.Errorf("bench_run only targets a loopback server, not %q; use the CLI for another host", h)
	}
	return nil
}

func (s *Server) setupApply(ctx context.Context, c *call, in setupIn) (any, error) {
	opts := app.SetupOptions{Mode: in.Mode, Venv: in.Venv, Image: in.Image, Model: in.Model, Mirror: true}
	steps, err := app.SetupPlan(ctx, opts)
	if err != nil {
		return nil, err
	}
	plan := setupPlanDoc{Run: app.SetupPending(steps, in.Steps), Manual: []setup.Step{}}
	for _, st := range steps {
		if st.Root && !st.Done {
			plan.Manual = append(plan.Manual, st)
		}
	}
	if len(plan.Run) == 0 {
		return planOnly[setupRunResult](c, plan, "nothing to run: every selected non-root step is done"), nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[setupRunResult]{tool: "setup_apply", token: token, args: in, plan: func() (any, error) { return plan, nil }, lock: &s.mutate,
		run: func(ctx context.Context) (*setupRunResult, error) {
			res, err := app.SetupApply(ctx, steps, opts, in.Steps, c.progress())
			out := &setupRunResult{Steps: res.Steps, Run: res.Run}
			if res.Run != nil {
				out.RunID = res.Run.ID
			}
			return out, err
		}})
}

// setupVerify checks the venv again: it runs the check of the venv even
// when its postcondition holds and records the run with the new evidence
// (spec §6.4). The plan is the step the check runs: the operator sees what
// is about to start before the token confirms it.
func (s *Server) setupVerify(ctx context.Context, c *call, in setupVerifyIn) (any, error) {
	opts := app.SetupOptions{Mode: in.Mode, Venv: in.Venv, Mirror: true}
	plan := func() (any, error) {
		steps, err := app.SetupPlan(ctx, opts)
		if err != nil {
			return nil, err
		}
		forced, err := app.SetupForceVerify(steps)
		if err != nil {
			return nil, err
		}
		run := []setup.Step{}
		for _, st := range forced {
			if st.ID == setup.StepVenvVerify {
				run = append(run, st)
			}
		}
		return setupPlanDoc{Run: run, Manual: []setup.Step{}}, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[setupRunResult]{tool: "setup_verify", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(ctx context.Context) (*setupRunResult, error) {
			res, err := app.SetupVerify(ctx, opts, c.progress())
			out := &setupRunResult{}
			if res != nil {
				out.Steps, out.Run = res.Steps, res.Run
				if res.Run != nil {
					out.RunID = res.Run.ID
				}
			}
			return out, err
		}})
}

func (s *Server) powerSet(_ context.Context, c *call, in powerIn) (any, error) {
	desired, err := app.PlanPowerDesired(in.GPUIndexes, in.Watts)
	if err != nil {
		return nil, err
	}
	plan := powerPlan{
		File: desired.File, Watts: desired.Watts,
		GPUIndexes: append([]int{}, desired.GPUIndexes...), Desired: desired.Desired,
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[app.PowerDesiredResult]{tool: "tune_power_set", token: token, args: in, plan: func() (any, error) { return plan, nil }, lock: &s.mutate,
		run: func(context.Context) (*app.PowerDesiredResult, error) {
			r, err := app.PowerDesiredSet(in.GPUIndexes, in.Watts)
			return &r, err
		}})
}
