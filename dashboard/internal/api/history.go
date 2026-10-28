package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
)

// GET /api/gpus/history?window=5m|15m|1h — the samples of the last minutes,
// one series per card. The ring buffer of the telemetry hub holds them: it
// keeps 30 minutes at one point every 2 s, and the 1 h window holds one
// point every 10 s.
//
// The window is 15 m when the request names none. A window the server does
// not know answers with 400.
func (h *handler) getHistory(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("window")
	if name == "" {
		name = telemetry.Window15m
	}
	hist := h.history()
	if hist == nil {
		httpjson.Error(w, http.StatusServiceUnavailable, "the telemetry history is not available")
		return
	}
	doc, err := hist.Window(name)
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpjson.Redacted(w, http.StatusOK, doc)
}

// history returns the ring buffer of the samples, or nil when the dashboard
// has no telemetry source.
func (h *handler) history() *telemetry.History {
	if h.telemetry == nil {
		return nil
	}
	return h.telemetry.History()
}
