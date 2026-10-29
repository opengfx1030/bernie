package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

const (
	// fleetCacheTTL holds the answer for a short time, so the page
	// reading it again and again does not hammer the peers.
	fleetCacheTTL = 5 * time.Second
)

// fleetCache is the short-lived cache of the Fleet summary. The key is
// empty: the cache holds one document per process.
type fleetCache struct {
	val app.FleetList
	at  time.Time
}

func newFleetCache() *fleetCache { return &fleetCache{} }

func (c *fleetCache) get(ctx context.Context, build func(context.Context) (app.FleetList, error)) (app.FleetList, error) {
	if !c.at.IsZero() && time.Since(c.at) < fleetCacheTTL {
		return c.val, nil
	}
	v, err := build(ctx)
	if err != nil {
		return app.FleetList{}, err
	}
	c.val = v
	c.at = time.Now()
	return v, nil
}

// GET /api/fleet — one summary per host: the local self first, then every
// configured peer. The token stays in the file: the answer never carries
// it (dashboard/AGENTS.md hard rule 6).
func (h *handler) getFleet(w http.ResponseWriter, r *http.Request) {
	val, err := h.fleet.get(r.Context(), app.Fleet)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val.Hosts)
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
	entry, err := app.FleetAdd(req.Name, req.URL, req.Token)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			httpjson.Error(w, http.StatusConflict, err.Error())
			return
		}
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.fleet.at = time.Time{}
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
	if err := app.FleetRemove(name); err != nil {
		if strings.Contains(err.Error(), "no fleet entry") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.fleet.at = time.Time{}
	httpjson.Write(w, http.StatusOK, map[string]any{"removed": name})
}
