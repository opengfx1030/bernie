package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestBenchSuiteEndpoints checks that the dashboard lists the built-in
// suites and refuses a comparison without two results.
func TestBenchSuiteEndpoints(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	host := map[string]string{"Host": "127.0.0.1:7860"}
	w := do(t, h, "GET", "/api/bench/suites", host)
	if w.Code != 200 {
		t.Fatalf("GET /api/bench/suites: %d %s", w.Code, w.Body.String())
	}
	var suites []struct {
		ID     string `json:"id"`
		Digest string `json:"digest"`
		Groups []struct {
			Name        string `json:"name"`
			Concurrency []int  `json:"concurrency"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &suites); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range suites {
		if s.ID != "pr" {
			continue
		}
		found = true
		if s.Digest == "" {
			t.Error("the pr suite has no digest")
		}
		cells := 0
		for _, g := range s.Groups {
			cells += len(g.Concurrency)
		}
		if cells != 6 {
			t.Errorf("the pr suite has %d cells, want 6", cells)
		}
	}
	if !found {
		t.Errorf("the suite list has no pr suite: %+v", suites)
	}
	if w := do(t, h, "GET", "/api/bench/compare?a=x", host); w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/bench/compare without b: %d, want 400", w.Code)
	}
	if w := do(t, h, "GET", "/api/bench/compare?a=nope&b=nope", host); w.Code != http.StatusNotFound {
		t.Errorf("GET /api/bench/compare with unknown ids: %d, want 404", w.Code)
	}
}

// view are served: without them the Bench tab loses its verdict chips and
// its length curve.
func TestBenchAssetsAreServed(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	host := map[string]string{"Host": "127.0.0.1:7860"}
	for _, name := range []string{"css/bench.css", "views/benchmark/suites.js"} {
		w := do(t, h, "GET", "/"+name, host)
		if w.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", name, w.Code)
			continue
		}
		if !strings.Contains(w.Body.String(), "{") {
			t.Errorf("GET /%s: the body holds no rule", name)
		}
	}
	w := do(t, h, "GET", "/", host)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `href="css/bench.css"`) {
		t.Error("the page does not link css/bench.css")
	}
	if !strings.Contains(body, `src="views/benchmark/suites.js"`) {
		t.Error("the page does not load views/benchmark/suites.js")
	}
}

// TestTheBenchViewHasNoInnerHTML checks the rule of the dashboard: the
// views build their nodes, they never paste markup.
func TestTheBenchViewHasNoInnerHTML(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	host := map[string]string{"Host": "127.0.0.1:7860"}
	for _, name := range []string{"views/benchmark/benchmark.js", "views/benchmark/suites.js"} {
		w := do(t, h, "GET", "/"+name, host)
		if w.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", name, w.Code)
			continue
		}
		if strings.Contains(w.Body.String(), ".innerHTML") {
			t.Errorf("%s uses innerHTML", name)
		}
	}
}
