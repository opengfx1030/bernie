package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
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
	addTool[setupIn, Outcome[[]setup.Step]](s, w("setup_apply", "Run the install steps that need no root (bernie setup apply). Steps that need root are returned as commands for the operator.", true, false, true),
		func(sc *jsonschema.Schema) { prop(sc, "mode").Enum = enum("host", "container"); tokenSchema(sc) }, s.setupApply)
	addTool[powerIn, Outcome[app.PowerDesiredResult]](s, w("tune_power_set", "Write the desired GPU power cap (bernie tune power set). It changes the desired-state file; it never writes hwmon.", false, true, false),
		func(sc *jsonschema.Schema) {
			prop(sc, "watts").ExclusiveMinimum = jsonschema.Ptr(0.0)
			g := prop(sc, "gpu_indexes")
			g.Items.Minimum, g.UniqueItems = jsonschema.Ptr(0.0), true
			tokenSchema(sc)
		}, s.powerSet)
}

func (s *Server) serve(ctx context.Context, c *call, in serveIn) (any, error) {
	p, err := runner.Prepare(ctx, in.prepare())
	if err != nil {
		return nil, err
	}
	runs, err := runner.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		if r.Name == p.Name && r.State != runner.StateExited {
			return nil, fmt.Errorf("run %s is %s; stop it or choose another name", r.Name, r.State)
		}
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[runner.Record]{tool: "serve", token: token, args: in, plan: p, lock: &s.mutate,
		run: func(ctx context.Context) (*runner.Record, error) {
			// The run outlives this request: only a cancel before the start aborts it.
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return runner.Start(context.WithoutCancel(ctx), p, runner.StartOptions{Venv: in.Venv, Engine: in.Engine, NoPull: in.NoPull, Progress: c.progress()})
		}})
}

func (s *Server) stop(ctx context.Context, c *call, in stopIn) (any, error) {
	runs, err := runner.List(ctx)
	if err != nil {
		return nil, err
	}
	var plan *stopPlan
	for _, r := range runs {
		if r.Name == in.Name {
			plan = &stopPlan{Name: r.Name, Recipe: r.Recipe, State: r.State, Mode: r.Mode, PID: r.PID, Container: r.Container, URL: r.URL()}
		}
	}
	if plan == nil {
		return nil, fmt.Errorf("no run named %q (ps lists the runs)", in.Name)
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[app.StopResult]{tool: "stop", token: token, args: in, plan: plan, lock: &s.mutate,
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
	o, err := bench.Normalize(o)
	if err != nil {
		return nil, err
	}
	if o.URL == "" {
		o.URL = bench.DefaultURL()
	}
	if err := loopbackURL(o.URL); err != nil {
		return nil, err
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[bench.Result]{tool: "bench_run", token: token, args: o, plan: o, lock: &s.benchM,
		run: func(ctx context.Context) (*bench.Result, error) {
			r, _, err := app.Bench(ctx, o, c.progress())
			if err == app.ErrAllFailed {
				return nil, fmt.Errorf("%w%s", err, app.FirstCellError(r))
			}
			return r, err
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
	if ip := net.ParseIP(h); h != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("bench_run only targets a loopback server, not %q; use the CLI for another host", h)
	}
	return nil
}

func (s *Server) setupApply(ctx context.Context, c *call, in setupIn) (any, error) {
	opts := app.SetupOptions{Mode: in.Mode, Venv: in.Venv, Image: in.Image, Model: in.Model}
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
		return planOnly[[]setup.Step](c, plan, "nothing to run: every selected non-root step is done"), nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[[]setup.Step]{tool: "setup_apply", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(ctx context.Context) (*[]setup.Step, error) {
			after, err := app.SetupApply(ctx, steps, opts, in.Steps, c.progress())
			return &after, err
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
	return execute(s, c, mutation[app.PowerDesiredResult]{tool: "tune_power_set", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(context.Context) (*app.PowerDesiredResult, error) {
			r, err := app.PowerDesiredSet(in.GPUIndexes, in.Watts)
			return &r, err
		}})
}
