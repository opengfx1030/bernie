package recipe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/gpu"
)

// Fit statuses.
const (
	FitYes     = "fits"
	FitNo      = "no"
	FitUnknown = "unknown"
)

// Fit is how a recipe fits this host (spec §7.8). Modes are the modes the
// recipe supports (host always; container when it names an image).
type Fit struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
	Modes   []string `json:"modes"`
}

// CheckFit compares the recipe's requires block with the host. gpus == nil
// means no GPU data (unknown). ramBytes == 0 skips the RAM check.
func CheckFit(r *Recipe, gpus []gpu.GPU, ramBytes uint64) Fit {
	f := Fit{Status: FitYes, Reasons: []string{}, Modes: Modes(r)}
	rq := r.Requires
	if gpus == nil {
		f.Status = FitUnknown
		f.Reasons = append(f.Reasons, "no GPU data")
	} else {
		var match, vramOK int
		for _, g := range gpus {
			if !archMatch(rq.Arch, g.GFX) {
				continue
			}
			match++
			if rq.VRAMGiB <= 0 || float64(g.VRAMBytes) >= rq.VRAMGiB*gib {
				vramOK++
			}
		}
		switch {
		case match == 0:
			f.Reasons = append(f.Reasons, fmt.Sprintf("arch: needs %s; host has %s", strings.Join(rq.Arch, "/"), archSummary(gpus)))
		case match < rq.GPUs:
			f.Reasons = append(f.Reasons, fmt.Sprintf("gpus: needs %d %s GPUs; host has %d", rq.GPUs, strings.Join(rq.Arch, "/"), match))
		case vramOK < rq.GPUs:
			f.Reasons = append(f.Reasons, fmt.Sprintf("vram: needs %g GiB per GPU; %d of %d matching GPUs have it", rq.VRAMGiB, vramOK, match))
		}
	}
	if rq.RAMGiB > 0 && ramBytes > 0 && float64(ramBytes) < rq.RAMGiB*gib {
		f.Reasons = append(f.Reasons, fmt.Sprintf("ram: needs %g GiB host RAM; host has %.0f GiB", rq.RAMGiB, float64(ramBytes)/gib))
	}
	if len(f.Reasons) > 0 && f.Status == FitYes {
		f.Status = FitNo
	}
	if f.Status == FitUnknown && len(f.Reasons) > 1 {
		f.Status = FitNo // RAM alone already rules it out
	}
	return f
}

// ListEntry is one row of `recipe list` (and the dashboard's /api/recipes).
type ListEntry struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	Priority   int              `json:"priority"`
	Model      string           `json:"model"`
	Entrypoint string           `json:"entrypoint"`
	GPUs       int              `json:"gpus"`
	Arch       []string         `json:"arch"`
	Tags       []string         `json:"tags"`
	Source     string           `json:"source"`
	Inputs     map[string]Input `json:"inputs"`
	Modes      []string         `json:"modes"`
	Fit        Fit              `json:"fit"`
}

// Listing is the `recipe list` document.
type Listing struct {
	Recipes []ListEntry `json:"recipes"`
	Errors  []LoadError `json:"errors"`
}

// List resolves every launchable recipe of s and checks its fit. Without
// all, recipes that do not fit are left out (unknown fit is kept). Resolve
// errors join the load errors. Entries are sorted by id.
func List(s Set, gpus []gpu.GPU, ramBytes uint64, all bool) Listing {
	l := Listing{Recipes: []ListEntry{}, Errors: append([]LoadError{}, s.Errors...)}
	for _, id := range s.IDs(false) {
		r, err := s.Resolve(id)
		if err == nil {
			if errs := Validate(r); len(errs) > 0 {
				err = fmt.Errorf("%s", strings.Join(errs, "; "))
			}
		}
		if err != nil {
			l.Errors = append(l.Errors, LoadError{Source: s.Recipes[id].Source, Err: err.Error()})
			continue
		}
		fit := CheckFit(r, gpus, ramBytes)
		if !all && fit.Status == FitNo {
			continue
		}
		e := ListEntry{
			ID: r.ID, Name: r.Name, Status: r.Status, Priority: r.Priority,
			Model: r.Model.Source, Entrypoint: r.Entrypoint, GPUs: r.Requires.GPUs,
			Arch: r.Requires.Arch, Tags: r.Tags, Source: r.Source, Inputs: r.Inputs,
			Modes: fit.Modes, Fit: fit,
		}
		if e.Entrypoint == "" {
			e.Entrypoint = "cli"
		}
		if e.Inputs == nil {
			e.Inputs = map[string]Input{}
		}
		if e.Tags == nil {
			e.Tags = []string{}
		}
		l.Recipes = append(l.Recipes, e)
	}
	return l
}

// Best returns the id of the best fitting recipe of a listing: fit, then
// priority (higher first), then id. "" when no recipe fits.
func Best(l Listing) string {
	var c []ListEntry
	for _, e := range l.Recipes {
		if e.Fit.Status == FitYes {
			c = append(c, e)
		}
	}
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Priority != c[j].Priority {
			return c[i].Priority > c[j].Priority
		}
		return c[i].ID < c[j].ID
	})
	if len(c) == 0 {
		return ""
	}
	return c[0].ID
}
