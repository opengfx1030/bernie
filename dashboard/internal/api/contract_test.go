package api

// The contract between the dashboard UI and the JSON documents of spec §11.
//
// The web UI reads named fields of these documents. When a field disappears
// from a document the page shows a dash (or, worse, a wrong number) instead
// of an error, so nothing fails until a reader sees it. This test is the
// guard: it marshals the Go type behind every document the UI reads and
// checks that the fields the UI depends on are still there.
//
// The names in the table are the names in the web files. A field that moves
// or that goes away breaks the UI, so the table breaks with it.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// TestUIFieldContract checks that the JSON documents of the API still carry
// the fields the web UI reads. One table, one check per document.
func TestUIFieldContract(t *testing.T) {
	cases := []struct {
		name string
		doc  any
		want []string
	}{
		{
			name: "telemetry gpu sample",
			doc: gpu.Sample{
				HIPIndex: 8, BusyPct: 12.5, TempC: 49, PowerW: 16, PowerCapW: 175,
				VRAMUsed: 1 << 30, VRAMTotal: 32 << 30,
				TempJunctionC: new(52.0), TempMemC: new(58.0),
				SclkMHz: new(2515), MclkMHz: new(96),
			},
			want: []string{
				"hip_index", "busy_pct", "temp_c", "power_w", "power_cap_w",
				"vram_used", "vram_total", "temp_junction_c", "temp_mem_c",
				"sclk_mhz", "mclk_mhz",
			},
		},
		{
			name: "vllm stats",
			doc:  vllm.Stats{},
			want: []string{
				"requests_running", "requests_waiting", "kv_cache_usage",
				"prompt_tokens_total", "generation_tokens_total",
				"spec_drafts_total", "spec_draft_tokens_total",
				"spec_accepted_tokens_total", "spec_accept_rate",
				"spec_mean_accept_len",
			},
		},
		{
			name: "run",
			doc: flow.Run{
				Status: runner.Status{
					Record: runner.Record{
						Name: "flashnext", Recipe: "flashnext", Mode: "container",
						Host: "127.0.0.1", Port: 8000, StartedAt: time.Now(),
					},
					State: "running",
				},
				Phase:    &logdoc.Phase{ID: "load", Sentence: "The server is loading the model.", Done: 1, Total: 2},
				Findings: []logdoc.Match{{}},
			},
			want: []string{
				"name", "recipe", "mode", "host", "port", "started_at", "state",
				"phase.id", "phase.sentence", "phase.done", "phase.total",
				"findings",
			},
		},
		{
			name: "power service",
			doc: app.PowerServiceStatus{
				State: "active", Installed: true, Enabled: true, Active: true,
				GPUs: []app.PowerGPUStatus{{HIPIndex: 8, Name: "V620", CapW: 175, MinW: 150, MaxW: 175,
					DesiredW: new(175.0), EffectiveW: new(175.0)}},
			},
			want: []string{
				"desired_file", "desired_exists", "state", "installed", "enabled",
				"active", "drift_count", "install_commands",
				"gpus[].hip_index", "gpus[].name", "gpus[].pci", "gpus[].cap_w",
				"gpus[].min_w", "gpus[].max_w", "gpus[].desired_w",
				"gpus[].effective_w", "gpus[].drift",
			},
		},
		{
			name: "power result",
			doc: app.PowerDesiredResult{
				Changed: true, File: "/config/power.yaml", Watts: 175,
				GPUIndexes: []int{0, 2}, Selectors: []string{"all"},
				Clamped: []int{2, 3},
			},
			want: []string{
				"changed", "file", "watts", "gpu_indexes", "selectors", "clamped",
			},
		},
		{
			name: "gpus history",
			doc: telemetry.HistoryDocument{
				Window: "5m", IntervalS: 2, Points: 1,
				Series: []telemetry.HistorySeries{{HIPIndex: 8}},
			},
			want: []string{
				"window", "interval_s", "points",
				"series[].hip_index", "series[].busy_pct", "series[].power_w",
				"series[].vram_used", "series[].tokens_per_s", "series[].prompt_tok_s",
			},
		},
		{
			name: "host gpu",
			doc: gpu.GPU{HIPIndex: 8, KFDNode: 9, Name: "V620", GFX: "gfx1030",
				TempLimitsC: map[string]int{"edge": 100}},
			want: []string{
				"hip_index", "kfd_node", "name", "gfx", "pci", "vram_bytes",
				"power_cap_w", "temp_limits_c",
			},
		},
		{
			name: "doctor report",
			doc: doctor.Report{
				Summary: map[doctor.Status]int{doctor.Fail: 1},
				Checks:  []doctor.Check{{ID: "gpu.ecc.8", Category: "gpu", Status: doctor.OK, Detail: "d", Fix: "f"}},
			},
			want: []string{
				"summary", "checks[].id", "checks[].category", "checks[].status",
				"checks[].title", "checks[].detail", "checks[].fix",
			},
		},
		{
			name: "bench result",
			doc: bench.Result{
				ID: "r1", Suite: &bench.SuiteRef{ID: "short", Digest: "abc", Version: 1},
				Fingerprint: map[string]string{"gpus": "2x W7800"},
				Cells: []bench.Cell{{
					Concurrency: 1, NumPrompts: 4, Completed: 4, Group: "short",
					InputLen: 128, OutputLen: 64,
					Measurement: bench.Measurement{
						Prefill: &bench.Rate{AggTokS: 1200, Req: &bench.Dist{P50: 1100}},
						Decode:  &bench.Rate{AggTokS: 30, Req: &bench.Dist{P50: 29}},
						TTFTs:   &bench.Dist{P50: 0.1, P95: 0.2, Max: 0.3},
						ITLms:   &bench.Dist{P50: 25, P95: 30, Max: 40},
						ServerSide: &bench.ServerSide{
							Source: "vllm", TTFTs: &bench.Dist{P50: 0.11}, ITLms: &bench.Dist{P50: 26},
						},
						OutputTokS: 30,
						Recall:     &bench.Recall{Asked: 4, Passed: 4},
						Check:      &bench.Check{Asked: 4, Passed: 4},
					},
					Reps:   []bench.Rep{{NumPrompts: 4, Completed: 4}},
					Spread: map[string]float64{"output_tok_s": 1},
				}},
			},
			want: []string{
				"id", "time", "fingerprint", "suite.id", "suite.digest", "suite.version",
				"cells[].group", "cells[].concurrency", "cells[].num_prompts",
				"cells[].completed", "cells[].input_len", "cells[].output_len",
				"cells[].prefill.agg_tok_s", "cells[].prefill.req.p50",
				"cells[].decode.agg_tok_s", "cells[].decode.req.p50",
				"cells[].ttft_s.p50", "cells[].ttft_s.p95", "cells[].ttft_s.max",
				"cells[].itl_ms.p50", "cells[].itl_ms.p95", "cells[].itl_ms.max",
				"cells[].output_tok_s", "cells[].spread", "cells[].reps",
				"cells[].server_side.ttft_s.p50", "cells[].server_side.itl_ms.p50",
				"cells[].size_valid", "cells[].output_valid", "cells[].stream_incremental",
				"cells[].recall", "cells[].check",
			},
		},
		{
			name: "onboarding checklist",
			doc: onboarding.Document{
				Next: "hardware",
				Steps: []onboarding.Step{{
					ID: "hardware", Title: "Check the cards", State: "ready",
					CanRun: true, JobID: "job-1", JobState: "running",
					Command: "bernie hw", RootCommands: []string{"sudo modprobe amdgpu"},
					Problems: []onboarding.Problem{{ID: "gpu.ecc.8"}},
					Progress: &onboarding.Progress{Phase: "Reading the cards.", Done: 1, Total: 2},
				}},
			},
			want: []string{
				"next", "steps[].id", "steps[].title", "steps[].state",
				"steps[].summary", "steps[].can_run", "steps[].command",
				"steps[].root_commands", "steps[].job_id", "steps[].job_state",
				"steps[].progress.phase", "steps[].progress.done", "steps[].progress.total",
				"steps[].problems[].id", "facts_age_s", "stale",
			},
		},
		{
			name: "topology",
			doc: topo.Topology{
				Nodes: []topo.Node{{HIPIndex: 8, PCI: "0000:12:00.0"}},
				Pairs: []topo.Pair{{A: 7, B: 8, Class: topo.ClassPIX, Hops: 1, P2P: true, GBps: 12}},
			},
			want: []string{
				"nodes[].hip_index", "nodes[].pci", "nodes[].path", "nodes[].switch",
				"nodes[].root_port", "nodes[].root",
				"pairs[].a", "pairs[].b", "pairs[].class", "pairs[].hops",
				"pairs[].p2p", "pairs[].gbps",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.doc)
			if err != nil {
				t.Fatal(err)
			}
			var v any
			if err := json.Unmarshal(b, &v); err != nil {
				t.Fatal(err)
			}
			for _, p := range c.want {
				if !hasJSONPath(v, p) {
					t.Errorf("the field %q is gone from the document of %s:\n%s", p, c.name, b)
				}
			}
		})
	}
}

// hasJSONPath reports whether the JSON value carries the field at the path.
// A segment with the "[]" suffix is the first element of an array.
func hasJSONPath(v any, path string) bool {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		first := false
		if strings.HasSuffix(seg, "[]") {
			seg = strings.TrimSuffix(seg, "[]")
			first = true
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		val, ok := m[seg]
		if !ok {
			return false
		}
		if first {
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				return false
			}
			val = arr[0]
		}
		cur = val
	}
	return true
}
