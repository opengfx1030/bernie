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

// TestBenchSuiteIgnoresTheCellFlags checks the contract of the run form: a
// suite decides the cells, so the cell flags of the request do not apply.
// A half-overridden cell is no cell.
func TestBenchSuiteIgnoresTheCellFlags(t *testing.T) {
	in := 4096
	warm := 3
	req := benchRequest{Suite: "pr", InputLen: &in, OutputLen: &in, Warmup: &warm, Recall: true, NumPrompts: 7}
	o, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if o.Suite != "pr" {
		t.Errorf("the options name the suite %q", o.Suite)
	}
	if o.InputLen != 0 || o.OutputLen != 0 || len(o.Concurrency) != 0 {
		t.Errorf("the suite run keeps a lens: %+v", o)
	}
	if o.Recall {
		t.Error("the suite run keeps the recall flag of the request")
	}
	if o.NumPrompts != 7 {
		t.Errorf("the suite run drops the request count: %+v", o)
	}
}

// TestBenchRequestKeepsTheCellFlags checks that a run without a suite keeps
// the flags of the request.
func TestBenchRequestKeepsTheCellFlags(t *testing.T) {
	in := 4096
	warm := 3
	req := benchRequest{InputLen: &in, OutputLen: &in, Warmup: &warm, Recall: true, Concurrency: json.RawMessage(`"1,8"`)}
	o, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if o.Suite != "" {
		t.Errorf("the options name the suite %q", o.Suite)
	}
	if o.InputLen != 4096 || o.OutputLen != 4096 || o.Warmup != 3 || !o.Recall {
		t.Errorf("the options lost a flag: %+v", o)
	}
	if len(o.Concurrency) != 2 || o.Concurrency[0] != 1 || o.Concurrency[1] != 8 {
		t.Errorf("the options hold the levels %v", o.Concurrency)
	}
}

// TestParseConcurrency checks the parsing of the concurrency field of the
// run form: a list or a comma string.
func TestParseConcurrency(t *testing.T) {
	for in, want := range map[string]string{"": "[1]", "null": "[1]", "[1,8]": "[1 8]", `"1, 4,16"`: "[1 4 16]", "[]": "[1]"} {
		got, err := parseConcurrency(json.RawMessage(in))
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if s := strings.Join(strings.Fields(strings.Trim(strings.ReplaceAll(fmtInts(got), ",", " "), "[]")), " "); "["+s+"]" != want {
			t.Errorf("%s: got %v, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"[0]", `"1,x"`, "{}", "[-2]"} {
		if _, err := parseConcurrency(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func fmtInts(v []int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestBenchAssetsAreServed checks that the bench stylesheet and the suite
// view are served: without them the Bench tab loses its verdict chips and
// its length curve.
func TestBenchAssetsAreServed(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	host := map[string]string{"Host": "127.0.0.1:7860"}
	for _, name := range []string{"bench.css", "views/bench-suite.js"} {
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
	if !strings.Contains(body, `href="bench.css"`) {
		t.Error("the page does not link bench.css")
	}
	if !strings.Contains(body, `src="views/bench-suite.js"`) {
		t.Error("the page does not load views/bench-suite.js")
	}
}

// TestTheBenchViewHasNoInnerHTML checks the rule of the dashboard: the
// views build their nodes, they never paste markup.
func TestTheBenchViewHasNoInnerHTML(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	host := map[string]string{"Host": "127.0.0.1:7860"}
	for _, name := range []string{"views/bench.js", "views/bench-suite.js"} {
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
