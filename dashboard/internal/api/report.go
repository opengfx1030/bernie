package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/report — the redacted share report of this host.
func (h *handler) getReport(w http.ResponseWriter, r *http.Request) {
	rep, err := app.BuildShareReport(r.Context())
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}
