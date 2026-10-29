package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
)

// POST /api/refresh — drop the host caches so the next read of /api/hw,
// /api/topo, /api/doctor, /api/onboarding and /api/power is a fresh probe.
// The Re-check buttons of the Setup, Topology, Advanced and Logs tabs call
// this before they reload, so a cable that changed or a check that just
// resolved shows up without a page reload (spec §11, ia.md R27).
func (h *handler) postRefresh(w http.ResponseWriter, r *http.Request) {
	h.hw.dropAll()
	h.topo.dropAll()
	h.check.dropAll()
	h.doc.dropAll()
	h.power.dropAll()
	h.runs.dropAll()
	httpjson.Write(w, http.StatusOK, map[string]any{"refreshed": true})
}
