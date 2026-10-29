package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// downloadLimitRequest is the body of POST /api/download/limit: the cap in
// bytes per second. 0 means no cap. A size string ("5MB") is accepted too.
type downloadLimitRequest struct {
	Limit int64  `json:"limit"`
	Size  string `json:"size,omitempty"`
}

// POST /api/download/limit — set the speed limit of the model download
// (R21). The new value is in force within a second for every running
// download: the downloader reads the cap on every write. The value is
// saved in the user config, so it survives a restart.
func (h *handler) postDownloadLimit(w http.ResponseWriter, r *http.Request) {
	var req downloadLimitRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	limit := req.Limit
	if req.Size != "" {
		n, err := config.ParseBytesPerSecond(req.Size)
		if err != nil {
			httpjson.Error(w, http.StatusBadRequest, "size: "+err.Error())
			return
		}
		limit = n
	}
	if limit < 0 {
		httpjson.Error(w, http.StatusBadRequest, "limit must not be negative")
		return
	}
	prev, err := setup.SetDownloadLimit(limit)
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	// The value is saved so it survives a restart. A config that cannot
	// be saved is reported: the live limit is still in force.
	cfg, err := config.Load()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg.DownloadLimit = limit
	if err := config.Save(cfg); err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{
		"limit": limit, "previous": prev,
		"note": "Changes take effect within a second.",
	})
}

// GET /api/download/limit — the current cap in bytes per second.
func (h *handler) getDownloadLimit(w http.ResponseWriter, r *http.Request) {
	limit := setup.CurrentDownloadLimit()
	httpjson.Write(w, http.StatusOK, map[string]any{
		"limit": limit,
		"note":  "Changes take effect within a second.",
	})
}
