package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// liveRun writes a run record that is alive and points at the given port.
// The engine is a script that answers "true" to the inspect of the runner,
// so the run counts as serving on every platform.
func liveRun(t *testing.T, name string, port int) {
	t.Helper()
	dir := runner.RunDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	engine := filepath.Join(t.TempDir(), "engine")
	if err := os.WriteFile(engine, []byte("#!/bin/sh\necho true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(runner.Record{
		Name: name, Mode: "container", Engine: engine, Container: "fake",
		Host: "127.0.0.1", Port: port, StartedAt: time.Now(),
	})
	if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// smokeHandler returns the API routes with an empty state directory.
func smokeHandler(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{})
	return mux
}

// post sends a JSON body to an endpoint.
func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TestSmokeEndpointAnswersThePrompt checks the answer of POST /api/smoke:
// the text of the reply and the numbers of the answer.
func TestSmokeEndpointAnswersThePrompt(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{Model: "fake-model", TTFT: 10 * time.Millisecond})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	h := smokeHandler(t)
	liveRun(t, "flashnext", port)

	w := post(t, h, "/api/smoke", `{"prompt":"How fast are you?","run":"flashnext"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("the smoke check: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		URL          string  `json:"url"`
		Model        string  `json:"model"`
		Health       bool    `json:"health"`
		Reply        string  `json:"reply"`
		TTFTMS       int     `json:"ttft_ms"`
		OutputTokens int     `json:"output_tokens"`
		TokS         float64 `json:"tok_s"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	if !res.Health || res.Model != "fake-model" || res.Reply == "" {
		t.Fatalf("the answer is %+v", res)
	}
	if res.TTFTMS < 10 || res.OutputTokens < 1 {
		t.Fatalf("the numbers are %+v", res)
	}
	if prompts := fake.ChatPrompts(); len(prompts) != 1 || prompts[0] != "How fast are you?" {
		t.Fatalf("the server got the chat prompts %v", prompts)
	}
}

// TestSmokeEndpointWithoutARun checks the answer when nothing serves.
func TestSmokeEndpointWithoutARun(t *testing.T) {
	h := smokeHandler(t)
	w := post(t, h, "/api/smoke", `{"prompt":"Hello"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("the smoke check without a run: got %d, want 404: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == nil {
		t.Fatalf("the body is not the error shape: %s", w.Body.String())
	}
}

// TestSmokeEndpointWithAnUnknownRun checks the answer for a name that does
// not exist.
func TestSmokeEndpointWithAnUnknownRun(t *testing.T) {
	h := smokeHandler(t)
	w := post(t, h, "/api/smoke", `{"run":"nope"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("the smoke check of an unknown run: got %d, want 404: %s", w.Code, w.Body.String())
	}
}

// TestSmokeEndpointRejectsABadBody checks the guard of the body.
func TestSmokeEndpointRejectsABadBody(t *testing.T) {
	h := smokeHandler(t)
	for _, body := range []string{`{"nope":1}`, `{`, `[]`} {
		w := post(t, h, "/api/smoke", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("the body %q: got %d, want 400: %s", body, w.Code, w.Body.String())
		}
	}
}

// TestSmokeEndpointFailsWhenTheServerIsDown checks the answer when the run
// does not answer.
func TestSmokeEndpointFailsWhenTheServerIsDown(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{}))
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Close()

	h := smokeHandler(t)
	liveRun(t, "flashnext", port)
	w := post(t, h, "/api/smoke", `{"run":"flashnext"}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("the smoke check of a dead server: got %d, want 502: %s", w.Code, w.Body.String())
	}
}
