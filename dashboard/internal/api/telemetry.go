package api

import (
	"net/http"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/sse"
)

// GET /api/telemetry — 1 Hz SSE; every event is one telemetry JSON document.
func (h *handler) getTelemetry(w http.ResponseWriter, r *http.Request) {
	ch, unsubscribe := h.telemetry.Subscribe()
	defer unsubscribe()
	st := sse.New(w)
	ctx := r.Context()
	ping := time.NewTicker(sse.Interval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ch:
			if st.Event("", string(b)) != nil {
				return
			}
		case <-ping.C:
			if st.Comment("ping") != nil {
				return
			}
		}
	}
}
