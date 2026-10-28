package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/doctor — `bernie doctor --json` (all categories, configured venv).
func (h *handler) getDoctor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rep, err := app.Doctor(r.Context(), q["category"], q.Get("venv"))
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}
