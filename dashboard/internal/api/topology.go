package api

import (
	"context"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/topo — `bernie topo --json` (no bandwidth test: it loads the GPUs).
func (h *handler) getTopo(w http.ResponseWriter, r *http.Request) {
	val, err := h.topo.get(r.Context(), "", func(ctx context.Context) (any, error) {
		return app.Topo(ctx, app.TopoOptions{NoKFDOK: true})
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}

// GET /api/topo/check — `bernie topo --check --json`.
func (h *handler) getTopoCheck(w http.ResponseWriter, r *http.Request) {
	recipe := r.URL.Query().Get("recipe")
	val, err := h.check.get(r.Context(), recipe, func(ctx context.Context) (any, error) {
		return app.P2PChecklist(ctx, app.P2PChecklistOptions{Recipe: recipe})
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}
