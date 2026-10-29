package api

import (
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// GET /api/prefs — bernie config show with the source of each value.
func (h *handler) getPrefs(w http.ResponseWriter, r *http.Request) {
	rep, err := app.Prefs(r.Context())
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}

type prefWrite struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PUT /api/prefs — write one KEY/VALUE pair. The auth middleware checks
// X-Bernie; httpjson.Write redacts the token out.
func (h *handler) putPrefs(w http.ResponseWriter, r *http.Request) {
	var in prefWrite
	if !httpjson.Decode(w, r, &in) {
		return
	}
	if in.Key == "" {
		httpjson.Error(w, http.StatusBadRequest, "missing key")
		return
	}
	if !knownKey(in.Key) {
		httpjson.Error(w, http.StatusBadRequest, "unknown key "+in.Key)
		return
	}
	rep, err := app.SetPref(r.Context(), in.Key, in.Value)
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rep)
}

func knownKey(k string) bool {
	for _, key := range config.Keys {
		if key == k {
			return true
		}
	}
	return false
}
