package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/bench/job"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// attachIn is the input of the attach tool: the URL (or port) of an
// already running vLLM server, the name to register it under and the
// recipe it serves (optional).
type attachIn struct {
	URL    string `json:"url" jsonschema:"base URL of the running vLLM server, or its port (loopback only)"`
	Name   string `json:"name,omitempty" jsonschema:"name of the run (default: attached)"`
	Recipe string `json:"recipe,omitempty" jsonschema:"recipe the server serves (optional)"`
	confirmToken
}

// benchCancelIn is the input of the bench_cancel tool: the id of the
// running benchmark job.
type benchCancelIn struct {
	JobID string `json:"job_id" jsonschema:"id of the running benchmark job (bench_run with background returns it)"`
	confirmToken
}

// benchDeleteIn is the input of the bench_delete tool: the id of a saved
// result.
type benchDeleteIn struct {
	ID string `json:"id" jsonschema:"id of the saved benchmark result to delete"`
	confirmToken
}

// configSetIn is the input of the config_set tool: one key of the user
// defaults and its value.
type configSetIn struct {
	Key   string `json:"key" jsonschema:"config key: venv, mode, engine, device_access, hf_home, download_limit, set"`
	Value string `json:"value" jsonschema:"the new value of the key"`
	confirmToken
}

// attach registers an already running vLLM server as a run (R29). The
// health check, the metrics and the models list are checked first. bernie
// never starts or stops the server: `stop` on an attached run only forgets
// it.
func (s *Server) attach(ctx context.Context, c *call, in attachIn) (any, error) {
	if in.URL == "" {
		return nil, fmt.Errorf("attach needs the url of the running server")
	}
	if in.Name == "" {
		in.Name = "attached"
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[app.AttachResult]{tool: "attach", token: token, args: in, lock: &s.mutate,
		plan: func() (any, error) {
			return map[string]string{"url": in.URL, "name": in.Name, "recipe": in.Recipe}, nil
		},
		run: func(ctx context.Context) (*app.AttachResult, error) {
			return app.Attach(ctx, app.AttachOptions{URL: in.URL, Name: in.Name, Recipe: in.Recipe})
		}})
}

// benchCancel stops a running benchmark job of this server (R30). The run
// ends as failed and is marked incomplete: the partial result, when one
// was saved, stays for the record.
func (s *Server) benchCancel(_ context.Context, c *call, in benchCancelIn) (any, error) {
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[map[string]any]{tool: "bench_cancel", token: token, args: in, lock: &s.benchM,
		plan: func() (any, error) {
			j, ok := s.benchJobs.Get(in.JobID)
			if !ok {
				return nil, fmt.Errorf("no benchmark run named %q", in.JobID)
			}
			return j.Doc(), nil
		},
		run: func(ctx context.Context) (*map[string]any, error) {
			cancelled := s.benchJobs.Cancel(in.JobID)
			out := map[string]any{"job_id": in.JobID, "cancelled": cancelled}
			if j, ok := s.benchJobs.Get(in.JobID); ok {
				out["status"] = j.Status()
				out["incomplete"] = j.Doc().Incomplete
			}
			return &out, nil
		}})
}

// benchDelete removes a saved benchmark result (R30). The result is the
// evidence of a measurement: the tool asks for a confirm token first.
func (s *Server) benchDelete(_ context.Context, c *call, in benchDeleteIn) (any, error) {
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[map[string]any]{tool: "bench_delete", token: token, args: in, lock: &s.benchM,
		plan: func() (any, error) {
			r, err := benchGet(in.ID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"id": r.ID, "time": r.Time.String()}, nil
		},
		run: func(ctx context.Context) (*map[string]any, error) {
			id, err := app.BenchDelete(in.ID)
			if err != nil {
				return nil, err
			}
			return &map[string]any{"id": id, "deleted": true}, nil
		}})
}

// configSet changes one key of the user defaults (spec §12). The download
// limit of a model download lives there: `download_limit` takes a size
// like "5MB" and takes effect within a second (R21).
func (s *Server) configSet(_ context.Context, c *call, in configSetIn) (any, error) {
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[map[string]any]{tool: "config_set", token: token, args: in, lock: &s.mutate,
		plan: func() (any, error) {
			return map[string]string{"key": in.Key, "value": in.Value}, nil
		},
		run: func(ctx context.Context) (*map[string]any, error) {
			cfg, err := config.Load()
			if err != nil {
				return nil, err
			}
			if err := cfg.SetKey(in.Key, in.Value); err != nil {
				return nil, err
			}
			if err := config.Save(cfg); err != nil {
				return nil, err
			}
			// The download limit is read by the downloader on every
			// write, so the new value is in force right away.
			if in.Key == "download_limit" {
				setup.LoadDownloadLimitFromConfig()
			}
			out := map[string]any{"key": in.Key, "value": in.Value, "path": config.Path()}
			if in.Key == "download_limit" {
				out["limit"] = setup.CurrentDownloadLimit()
				out["note"] = "Changes take effect within a second."
			}
			return &out, nil
		}})
}

// benchGet reads one saved benchmark result for the plan of a delete.
func benchGet(id string) (*bench.Result, error) {
	r, err := app.BenchList()
	if err != nil {
		return nil, err
	}
	for i := range r {
		if r[i].ID == id {
			return &r[i], nil
		}
	}
	return nil, fmt.Errorf("no saved benchmark result named %q", id)
}

// benchStartIn is the input of the bench_start tool: the same flags as
// bench_run.
type benchStartIn struct {
	URL         string `json:"url,omitempty" jsonschema:"base URL of the server; loopback only (default: the only run, else http://127.0.0.1:8000)"`
	Model       string `json:"model,omitempty" jsonschema:"model name (default: the first id of /v1/models)"`
	InputLen    int    `json:"input_len,omitempty" jsonschema:"prompt tokens (default 1024)"`
	OutputLen   int    `json:"output_len,omitempty" jsonschema:"output tokens (default 512)"`
	Concurrency []int  `json:"concurrency,omitempty" jsonschema:"concurrency levels, one cell each (default [1])"`
	NumPrompts  int    `json:"num_prompts,omitempty" jsonschema:"requests per cell (default max(4, 2 x concurrency))"`
	Seed        int64  `json:"seed,omitempty" jsonschema:"prompt seed"`
	Tag         string `json:"tag,omitempty" jsonschema:"free label stored with the result"`
	Recall      bool   `json:"recall,omitempty" jsonschema:"hide a short random fact in every prompt and check that the reply holds it (default off)"`
	Suite       string `json:"suite,omitempty" jsonschema:"name of the built-in suite to run (default: none, bernie bench suites lists them)"`
	confirmToken
}

// benchStart starts a benchmark in the background and answers the id of
// the job (R30). The job can then be read with bench_status and stopped
// with bench_cancel.
func (s *Server) benchStart(ctx context.Context, c *call, in benchStartIn) (any, error) {
	o := bench.Options{URL: in.URL, Model: in.Model, Tag: in.Tag, InputLen: in.InputLen, OutputLen: in.OutputLen,
		Concurrency: in.Concurrency, NumPrompts: in.NumPrompts, Seed: in.Seed, Recall: in.Recall, Warmup: bench.DefaultWarmup}
	if in.Suite != "" {
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
	return execute(s, c, mutation[map[string]any]{tool: "bench_start", token: token, args: o, lock: &s.benchM,
		busy: app.ErrBenchBusy,
		plan: func() (any, error) { return o, nil },
		run: func(ctx context.Context) (*map[string]any, error) {
			j, err := s.benchJobs.Start(ctx, job.Request{Kind: job.KindRun, Options: o})
			if err != nil {
				return nil, err
			}
			return &map[string]any{"job_id": j.ID(), "status": j.Status()}, nil
		}})
}

// registerJobTools adds the tools of the jobs, the attach and the config
// to the write catalog.
func registerJobTools(s *Server) {
	w := func(name, desc string, destructive, idempotent, openWorld bool) spec {
		return spec{name: name, desc: desc + twoCalls, write: true, destructive: destructive, idempotent: idempotent, openWorld: openWorld}
	}
	addTool[benchStartIn, Outcome[map[string]any]](s, w("bench_start", "Start a benchmark in the background and answer the id of the job (bernie bench --background). The job is read with bench_status and stopped with bench_cancel.", false, false, true),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.benchStart)
	addTool[attachIn, Outcome[app.AttachResult]](s, w("attach", "Register an already running vLLM server as a run (bernie attach). The health check, the metrics and the models list are checked first. bernie never starts or stops the server: stop on an attached run only forgets it.", false, false, true),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.attach)
	addTool[benchCancelIn, Outcome[map[string]any]](s, w("bench_cancel", "Stop a running benchmark job of this server (bernie bench --cancel). The run ends as failed and is marked incomplete.", true, true, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.benchCancel)
	addTool[benchDeleteIn, Outcome[map[string]any]](s, w("bench_delete", "Delete a saved benchmark result (bernie bench delete). The result is the evidence of a measurement: the tool asks for a confirm token first.", true, true, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.benchDelete)
	addTool[configSetIn, Outcome[map[string]any]](s, w("config_set", "Change one key of the user defaults (bernie config set). The download limit of a model download lives there: download_limit takes a size like 5MB and takes effect within a second.", false, true, false),
		func(sc *jsonschema.Schema) { tokenSchema(sc) }, s.configSet)
}
