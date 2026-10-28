// Package flow composes the domain packages into the two product-level
// operations of spec §3.1: Snapshot (`status`) and Up (`up`). It decides
// nothing about formatting; front ends print the structs.
package flow

import (
	"context"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// Summary is a compact, human-sized view of one host (spec §3.1).
type Summary struct {
	Host     host.Info             `json:"host"`
	GPUs     []gpu.GPU             `json:"gpus"`
	Doctor   map[doctor.Status]int `json:"doctor"`
	Problems []doctor.Check        `json:"problems"` // fail and warn checks
	Best     string                `json:"best_recipe"`
	Fits     []string              `json:"fitting_recipes"`
	Runs     []runner.Status       `json:"runs"`
	Next     []string              `json:"next"`
}

// Snapshot probes the host and returns the status document. It changes
// nothing. venv is the host-mode venv flag value ("" = config/env).
func Snapshot(ctx context.Context, venv string) (*Summary, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	h, _ := host.Probe(ctx, "/")
	gpus, _ := gpu.List("/")
	rep := doctor.Run(ctx, doctor.Options{Root: "/", Venv: cfg.ResolveVenv(venv), HFHome: cfg.ResolveHFHome(), ValidatedArchs: recipe.ValidatedArchs(recipe.Load())})
	var listing recipe.Listing
	if gpus != nil {
		listing = recipe.List(recipe.Load(), gpus, h.RAMBytes, false)
	}
	runs, _ := runner.List(ctx)

	s := &Summary{Host: h, GPUs: gpus, Doctor: rep.Summary, Runs: runs,
		Problems: []doctor.Check{}, Fits: []string{}}
	if s.GPUs == nil {
		s.GPUs = []gpu.GPU{}
	}
	if s.Runs == nil {
		s.Runs = []runner.Status{}
	}
	for _, ch := range rep.Checks {
		if ch.Status == doctor.Fail || ch.Status == doctor.Warn {
			s.Problems = append(s.Problems, ch)
		}
	}
	for _, e := range listing.Recipes {
		if e.Fit.Status == recipe.FitYes {
			s.Fits = append(s.Fits, e.ID)
		}
	}
	s.Best = recipe.Best(listing)
	s.Next = next(s)
	return s, nil
}

// next returns the commands the operator should type, most important first.
func next(s *Summary) []string {
	var n []string
	if len(s.GPUs) == 0 {
		return []string{"bernie doctor   # no AMD GPU found: check /dev/kfd and the amdgpu driver"}
	}
	for _, ch := range s.Problems {
		if ch.Status == doctor.Fail {
			n = append(n, "bernie doctor   # "+ch.Title)
			break
		}
	}
	running := false
	for _, r := range s.Runs {
		if r.State == "running" || r.State == "starting" {
			running = true
		}
	}
	switch {
	case running:
		n = append(n, "bernie dash", "bernie bench")
	case s.Best != "":
		n = append(n, "bernie up   # serves "+s.Best)
	default:
		n = append(n, "bernie recipe list --all   # no recipe fits this host")
	}
	return n
}
