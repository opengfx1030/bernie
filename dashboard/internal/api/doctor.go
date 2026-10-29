package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/doctor — `bernie doctor --json` (all categories, configured venv).
func (h *handler) getDoctor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	categories := q["category"]
	venv := q.Get("venv")
	key := venv + "\x00" + strings.Join(categories, "\x00")
	val, err := h.doc.get(r.Context(), key, func(ctx context.Context) (any, error) {
		return app.Doctor(ctx, categories, venv)
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, val)
}
