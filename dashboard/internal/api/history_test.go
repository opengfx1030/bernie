package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// fakeTelemetry is a telemetry source with a ring buffer and no stream.
type fakeTelemetry struct {
	hist *telemetry.History
}

func (f fakeTelemetry) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 1)
	return ch, func() {}
}

func (f fakeTelemetry) History() *telemetry.History { return f.hist }

// historyHandler returns the API routes with a telemetry source.
func historyHandler(t *testing.T, h *telemetry.History) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, Options{Telemetry: fakeTelemetry{hist: h}})
	return mux
}

// get calls an endpoint and returns the recorder.
func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

// TestHistoryEndpointShape checks the answer of GET /api/gpus/history: the
// window, the spacing and one series per card.
func TestHistoryEndpointShape(t *testing.T) {
	hist := telemetry.NewHistory()
	now := time.Now()
	for i := range 3 {
		hist.Add(now.Add(-time.Duration(3-i)*2*time.Second), []gpu.Sample{
			{HIPIndex: 0, BusyPct: 12, PowerW: 18.4, VRAMUsed: 24117248},
		}, 56.1)
	}
	h := historyHandler(t, hist)
	w := get(t, h, "/api/gpus/history?window=5m")
	if w.Code != http.StatusOK {
		t.Fatalf("the history: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		Window    string `json:"window"`
		IntervalS int    `json:"interval_s"`
		Points    int    `json:"points"`
		Series    []struct {
			HIPIndex   int       `json:"hip_index"`
			BusyPct    []float64 `json:"busy_pct"`
			PowerW     []float64 `json:"power_w"`
			VRAMUsed   []uint64  `json:"vram_used"`
			TokensPerS []float64 `json:"tokens_per_s"`
		} `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the history is not JSON: %v\n%s", err, w.Body.String())
	}
	if doc.Window != "5m" || doc.IntervalS != 2 || doc.Points != 3 {
		t.Fatalf("the document is %+v", doc)
	}
	if len(doc.Series) != 1 {
		t.Fatalf("the document holds %d series, want 1", len(doc.Series))
	}
	s := doc.Series[0]
	if s.HIPIndex != 0 || len(s.BusyPct) != 3 || len(s.PowerW) != 3 || len(s.VRAMUsed) != 3 || len(s.TokensPerS) != 3 {
		t.Fatalf("the series is %+v", s)
	}
	if s.BusyPct[0] != 12 || s.PowerW[0] != 18.4 || s.VRAMUsed[0] != 24117248 || s.TokensPerS[0] != 56.1 {
		t.Fatalf("the series is %+v", s)
	}
}

// TestHistoryEndpointDefaultsToFifteenMinutes checks the default window.
func TestHistoryEndpointDefaultsToFifteenMinutes(t *testing.T) {
	h := historyHandler(t, telemetry.NewHistory())
	w := get(t, h, "/api/gpus/history")
	if w.Code != http.StatusOK {
		t.Fatalf("the history: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		Window string `json:"window"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Window != "15m" {
		t.Fatalf("the window is %q, want 15m", doc.Window)
	}
}

// TestHistoryEndpointRejectsAnUnknownWindow checks the guard of the window.
func TestHistoryEndpointRejectsAnUnknownWindow(t *testing.T) {
	h := historyHandler(t, telemetry.NewHistory())
	for _, w := range []string{"2m", "1d", "five"} {
		rec := get(t, h, "/api/gpus/history?window="+w)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("the window %q: got %d, want 400", w, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == nil {
			t.Errorf("the window %q: the body is not the error shape: %s", w, rec.Body.String())
		}
	}
}

// TestHistoryEndpointWithoutTelemetry checks the answer of a dashboard
// without a telemetry source.
func TestHistoryEndpointWithoutTelemetry(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Options{})
	w := get(t, mux, "/api/gpus/history")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("the history without telemetry: got %d, want 503", w.Code)
	}
}
