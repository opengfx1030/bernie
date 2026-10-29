package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
)

// GET /api/models — the `bernie models` document: the ids /v1/models
// answers when a run is serving, the repos the Hugging Face cache on this
// host holds, and the hub cache root (spec §11, ia.md R31). A live server
// that fails to answer /v1/models does not fail the endpoint: the cached
// part of the report is still useful and the failure is in `live_error`.
func (h *handler) getModels(w http.ResponseWriter, r *http.Request) {
	rep, err := app.Models(r.Context())
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}
