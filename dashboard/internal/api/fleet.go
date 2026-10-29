package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/fleet"
)

const (
	// fleetCacheTTL holds the answer for a short time, so the page
	// reading it again and again does not hammer the peers.
	fleetCacheTTL = 5 * time.Second
)

// fleetCache is the short-lived cache of the Fleet summary. The key is
// empty: the cache holds one document per process.
type fleetCache struct {
	val []fleet.Summary
	at  time.Time
}

func newFleetCache() *fleetCache { return &fleetCache{} }

func (c *fleetCache) get(ctx context.Context, build func(context.Context) ([]fleet.Summary, error)) ([]fleet.Summary, error) {
	if !c.at.IsZero() && time.Since(c.at) < fleetCacheTTL {
		return c.val, nil
	}
	v, err := build(ctx)
	if err != nil {
		return nil, err
	}
	c.val = v
	c.at = time.Now()
	return v, nil
}

// GET /api/fleet — one summary per host: the local self first, then every
// configured peer. The token stays in the file: the answer never carries
// it (dashboard/AGENTS.md hard rule 6).
func (h *handler) getFleet(w http.ResponseWriter, r *http.Request) {
	val, err := h.fleet.get(r.Context(), func(ctx context.Context) ([]fleet.Summary, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		return fleet.Fetch(ctx, fleet.Source{
			Entries: cfg.Fleet,
			Self:    selfSummary,
		}), nil
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}

// fleetAddRequest is the body of POST /api/fleet.
type fleetAddRequest struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

// POST /api/fleet — add one entry to the configured fleet. The token is
// stored in the file (0600 when a token is set) and is never echoed in the
// response. A bad URL answers 400; a name that already exists answers 409.
func (h *handler) postFleet(w http.ResponseWriter, r *http.Request) {
	var req fleetAddRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	entry := config.FleetEntry{Name: req.Name, URL: req.URL, Token: req.Token}
	if err := config.ValidateFleetEntry(entry); err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := config.Load()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, e := range cfg.Fleet {
		if e.Name == req.Name {
			httpjson.Error(w, http.StatusConflict, "a fleet entry named "+req.Name+" already exists")
			return
		}
	}
	cfg.UpsertFleet(entry)
	if err := config.Save(cfg); err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{
		"name": entry.Name, "url": entry.URL,
	})
}

// DELETE /api/fleet/{name} — forget one entry. The token is dropped.
func (h *handler) deleteFleet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		httpjson.Error(w, http.StatusBadRequest, "fleet name is required")
		return
	}
	cfg, err := config.Load()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cfg.RemoveFleet(name) {
		httpjson.Error(w, http.StatusNotFound, "no fleet entry named "+name)
		return
	}
	if err := config.Save(cfg); err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"removed": name})
}

// selfSummary is the local row of the Fleet tab: the same numbers the
// Dashboard tab already shows.
func selfSummary(ctx context.Context) fleet.Summary {
	row := fleet.Summary{Name: hostName(), Self: true, Reachable: true}
	rep, err := app.HW(ctx)
	if err == nil {
		for _, g := range rep.GPUs {
			card := fleet.GPUDetail{
				Index: g.HIPIndex,
				Name:  g.Name,
			}
			if g.VRAMBytes > 0 {
				gb := float64(g.VRAMBytes) / (1 << 30)
				card.VRAMGiB = &gb
			}
			if g.PowerCapW > 0 {
				w := g.PowerCapW
				card.PowerCapW = &w
			}
			row.GPU = append(row.GPU, card)
		}
	}
	runs, err := flow.Runs(ctx)
	if err == nil {
		applySelfRuns(&row, runs)
	}
	return row
}

// applySelfRuns picks the live run of the local host and fills the
// run-shaped fields of the self row.
func applySelfRuns(row *fleet.Summary, runs any) {
	list, ok := runs.([]any)
	if !ok {
		row.State = "stopped"
		return
	}
	var pick map[string]any
	for _, r := range list {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		st, _ := m["state"].(string)
		if st == "serving" || st == "running" {
			pick = m
			break
		}
		if pick == nil {
			pick = m
		}
	}
	if pick == nil {
		row.State = "stopped"
		return
	}
	if s, ok := pick["state"].(string); ok {
		row.State = s
	}
	if s, ok := pick["recipe"].(string); ok {
		row.Recipe = s
	}
	if s, ok := pick["stats"].(map[string]any); ok {
		if v, ok := s["gen_tok_s"].(float64); ok {
			row.GenTokS = &v
		}
	}
}

// hostName reads the OS hostname once; cached by the OS after the first
// syscall.
func hostName() string {
	if name, err := os.Hostname(); err == nil {
		return name
	}
	return "this box"
}
