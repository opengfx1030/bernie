package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/topo — `bernie topo --json` (no bandwidth test: it loads the GPUs).
func (h *handler) getTopo(w http.ResponseWriter, r *http.Request) {
	t, err := app.Topo(r.Context(), app.TopoOptions{NoKFDOK: true})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, t)
}

// GET /api/topo/check — `bernie topo --check --json`.
func (h *handler) getTopoCheck(w http.ResponseWriter, r *http.Request) {
	rep, err := app.P2PChecklist(r.Context(), app.P2PChecklistOptions{Recipe: r.URL.Query().Get("recipe")})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}
