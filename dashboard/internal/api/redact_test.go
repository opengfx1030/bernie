package api

// The redaction rule of the dashboard (dashboard/AGENTS.md, hard rule 6):
// nothing that leaves the process carries a secret. This test walks every
// route of the API and checks that the secret is not in the answer. The routes
// come from the router itself, so a route that is added later is covered
// without touching this file.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// theSecret is the value that must never reach the browser.
const theSecret = "hf_abcdefghijklmnopqrstuvwx"

// routePaths are the concrete requests of the routes that name something.
// Every other route gets its placeholders filled with a name that exists
// nowhere: it answers with an error and changes nothing.
var routePaths = map[string]string{
	"GET /api/recipes/{id}/plan":      "/api/recipes/27b-awq/plan?set=env.HF_TOKEN=" + theSecret,
	"GET /api/runs/{name}/logs":       "/api/runs/flashnext/logs",
	"GET /api/runs/{name}/log":        "/api/runs/flashnext/log",
	"GET /api/runs/{name}/findings":   "/api/runs/flashnext/findings",
	"GET /api/bench/{id}":             "/api/bench/nope",
	"GET /api/bench/{id}/events":      "/api/bench/nope/events",
	"GET /api/jobs/{id}":              "/api/jobs/nope",
	"GET /api/jobs/{id}/events":       "/api/jobs/nope/events",
	"POST /api/jobs/{id}/stop":        "/api/jobs/nope/stop",
	"POST /api/onboarding/{step}/run": "/api/onboarding/hardware/run",
	"DELETE /api/runs/{name}":         "/api/runs/nope",
}

// placeholder matches the "{name}" of a route pattern.
var placeholder = regexp.MustCompile(`\{[^}]*\}`)

// routeRequest returns the method and the path of a request that reaches the
// route of this pattern.
func routeRequest(pattern string) (string, string) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		method, path = http.MethodGet, pattern
	}
	if p, known := routePaths[pattern]; known {
		return method, p
	}
	return method, placeholder.ReplaceAllString(path, "nope")
}

// TestEveryRouteRedacts walks every route of the API and checks that the
// secret is not in the answer. The secret is in the log of a run and in the
// environment of the plan, so a route that carries host, run, log or config
// text would answer with it if it did not redact.
func TestEveryRouteRedacts(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{Telemetry: fakeTelemetry{}})
	writeRun(t, "flashnext", "the server starts\nHF_TOKEN="+theSecret+"\nAuthorization: Bearer "+theSecret+"\n")

	if len(routes) == 0 {
		t.Fatal("the API has no route")
	}
	for _, rt := range routes {
		method, path := routeRequest(rt.pattern)
		// The stream routes answer until the request ends, so every request
		// gets a deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		var body *strings.Reader
		if method != http.MethodGet && method != http.MethodHead {
			// A body with an unknown field: the handler refuses it before it
			// changes anything.
			body = strings.NewReader(`{"__redact_probe__":true}`)
		} else {
			body = strings.NewReader("")
		}
		r := httptest.NewRequest(method, path, body).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		cancel()
		if strings.Contains(w.Body.String(), theSecret) {
			t.Errorf("%s %s answers with the secret:\n%s", method, path, w.Body.String())
		}
	}
}

// TestRoutesRedactWhatTheyRead checks that the routes that carry text do
// redact it: the log of the run and the environment of the plan hold the
// secret, and both answers carry the marker of the redaction.
func TestRoutesRedactWhatTheyRead(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{Telemetry: fakeTelemetry{}})
	writeRun(t, "flashnext", "the server starts\nHF_TOKEN="+theSecret+"\nAuthorization: Bearer "+theSecret+"\n")

	w := get(t, mux, "/api/runs/flashnext/log")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "<redacted>") {
		t.Errorf("the log was not redacted:\n%s", w.Body.String())
	}

	w = get(t, mux, "/api/recipes/27b-awq/plan?set=env.HF_TOKEN="+theSecret)
	if w.Code != http.StatusOK {
		t.Fatalf("the plan: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "<redacted>") {
		t.Errorf("the plan was not redacted:\n%s", w.Body.String())
	}
}
