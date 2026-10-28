package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doPower(t *testing.T, h http.Handler, method, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/power", strings.NewReader(body))
	r.Host = "127.0.0.1:7860"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPowerAPI(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	if w := doPower(t, h, http.MethodGet, "", nil); w.Code != http.StatusOK {
		t.Fatalf("GET /api/power: %d %s", w.Code, w.Body.String())
	}
	if w := doPower(t, h, http.MethodPut, `{"gpu":0,"watts":180}`, nil); w.Code != http.StatusForbidden {
		t.Fatalf("PUT without X-Bernie: %d", w.Code)
	}
	for _, body := range []string{`{`, `{}`, `{"gpu":0,"watts":0}`, `{"gpu":-1,"watts":180}`} {
		w := doPower(t, h, http.MethodPut, body, map[string]string{"X-Bernie": "1"})
		if w.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: %d, want 400", body, w.Code)
		}
	}
	// The real host has no bernie power unit, so a valid write is refused
	// until the operator installs it.
	w := doPower(t, h, http.MethodPut, `{"gpu":0,"watts":180}`, map[string]string{"X-Bernie": "1"})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not installed") {
		t.Fatalf("PUT without service: %d %s", w.Code, w.Body.String())
	}
}
