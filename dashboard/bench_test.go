package dashboard

import (
	"encoding/json"
	"io/fs"
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
	// The page carries the shell and the Setup tab. The files of the other
	// tabs are named in the manifest of js/boot.js and come when the tab
	// opens: the Benchmark tab brings css/bench.css and the two files of
	// the view.
	web, _ := fs.Sub(webFS, "web")
	boot, err := fs.ReadFile(web, "js/boot.js")
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(boot)
	for _, name := range []string{"css/bench.css", "views/benchmark/suites.js"} {
		if !strings.Contains(manifest, `"`+name+`"`) {
			t.Errorf("the manifest of js/boot.js does not name %s", name)
		}
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
