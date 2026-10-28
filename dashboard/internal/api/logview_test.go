package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// logHandler returns the API routes with a state directory of its own.
func logHandler(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{})
	return mux
}

// writeRun writes a run record with a log of the given text.
func writeRun(t *testing.T, name, log string) {
	t.Helper()
	dir := runner.RunDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lp := filepath.Join(dir, "serve.log")
	if err := os.WriteFile(lp, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(runner.Record{Name: name, Mode: "host", PID: 0, Port: 18000, Log: lp})
	if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// theLog is the log of the test run.
const theLog = "the server starts\n" +
	"WARNING the cache is small\n" +
	"ERROR the engine died\n" +
	"RuntimeError: boom\n" +
	"the server waits\n"

// TestRunLogLevelsAndNumbers checks the answer of GET /api/runs/{name}/log:
// the number of every line, its level and the number of lines of the log.
func TestRunLogLevelsAndNumbers(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", theLog)
	w := get(t, h, "/api/runs/flashnext/log")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		Name       string `json:"name"`
		State      string `json:"state"`
		TotalLines int    `json:"total_lines"`
		Truncated  bool   `json:"truncated"`
		Lines      []struct {
			No    int    `json:"no"`
			Level string `json:"level"`
			Text  string `json:"text"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the log is not JSON: %v\n%s", err, w.Body.String())
	}
	if doc.Name != "flashnext" || doc.State != "exited" {
		t.Fatalf("the document is %+v", doc)
	}
	if doc.TotalLines != 5 || doc.Truncated {
		t.Fatalf("the document holds %d lines, truncated %v", doc.TotalLines, doc.Truncated)
	}
	want := []struct {
		no    int
		level string
	}{
		{1, "info"}, {2, "warn"}, {3, "error"}, {4, "error"}, {5, "info"},
	}
	if len(doc.Lines) != len(want) {
		t.Fatalf("the log holds %d lines, want %d", len(doc.Lines), len(want))
	}
	for i, w := range want {
		if doc.Lines[i].No != w.no || doc.Lines[i].Level != w.level {
			t.Errorf("the line %d is %+v, want %d %s", i, doc.Lines[i], w.no, w.level)
		}
	}
}

// TestRunLogFilters checks the level filter and the search.
func TestRunLogFilters(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", theLog)
	var doc struct {
		TotalLines int `json:"total_lines"`
		Lines      []struct {
			No    int    `json:"no"`
			Level string `json:"level"`
			Text  string `json:"text"`
		} `json:"lines"`
	}
	w := get(t, h, "/api/runs/flashnext/log?level=error")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.TotalLines != 5 {
		t.Errorf("the log holds %d lines, want 5", doc.TotalLines)
	}
	if len(doc.Lines) != 2 || doc.Lines[0].No != 3 || doc.Lines[1].No != 4 {
		t.Fatalf("the error lines are %+v", doc.Lines)
	}
	w = get(t, h, "/api/runs/flashnext/log?q=WAITS")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Lines) != 1 || doc.Lines[0].No != 5 {
		t.Fatalf("the searched lines are %+v", doc.Lines)
	}
}

// TestRunLogTail checks the tail: the last lines of the log, and the mark
// that lines were dropped.
func TestRunLogTail(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", theLog)
	var doc struct {
		Truncated bool `json:"truncated"`
		Lines     []struct {
			No int `json:"no"`
		} `json:"lines"`
	}
	w := get(t, h, "/api/runs/flashnext/log?tail=2")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Truncated {
		t.Error("the log is not marked as truncated")
	}
	if len(doc.Lines) != 2 || doc.Lines[0].No != 4 || doc.Lines[1].No != 5 {
		t.Fatalf("the tail is %+v", doc.Lines)
	}
}

// TestRunLogGuards checks the guards of the endpoint.
func TestRunLogGuards(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", theLog)
	for _, q := range []string{"?level=loud", "?tail=-1", "?tail=abc", "?tail=99999"} {
		if w := get(t, h, "/api/runs/flashnext/log"+q); w.Code != http.StatusBadRequest {
			t.Errorf("the query %q: got %d, want 400", q, w.Code)
		}
	}
	if w := get(t, h, "/api/runs/nope/log"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown run: got %d, want 404", w.Code)
	}
	if w := get(t, h, "/api/runs/nope/findings"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown run: got %d, want 404", w.Code)
	}
}

// TestRunFindings checks the findings of a run: the known failures of the
// log, with the fix of every one.
func TestRunFindings(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", "start\n(EngineCore pid=1) ERROR torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 1 GiB\n")
	w := get(t, h, "/api/runs/flashnext/findings")
	if w.Code != http.StatusOK {
		t.Fatalf("the findings: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		Name    string `json:"name"`
		Matches []struct {
			ID       string `json:"id"`
			Severity string `json:"severity"`
			Cause    string `json:"cause"`
			Fix      string `json:"fix"`
			Doc      string `json:"doc"`
			Line     string `json:"line"`
			LineNo   int    `json:"line_no"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the findings are not JSON: %v\n%s", err, w.Body.String())
	}
	if doc.Name != "flashnext" || len(doc.Matches) != 1 {
		t.Fatalf("the document is %+v", doc)
	}
	m := doc.Matches[0]
	if m.ID != "memory.gpu-oom" || m.Severity != "error" {
		t.Fatalf("the match is %+v", m)
	}
	if m.Fix == "" || m.Cause == "" || m.Doc == "" {
		t.Fatalf("the match is %+v", m)
	}
	if m.LineNo != 2 {
		t.Errorf("the match is on the line %d, want 2", m.LineNo)
	}
}

// TestRunLogMarksTheLinesOfAFinding checks that a line a finding points at
// is an error, whatever it holds.
func TestRunLogMarksTheLinesOfAFinding(t *testing.T) {
	h := logHandler(t)
	writeRun(t, "flashnext", "start\n(EngineCore pid=1) ERROR torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 1 GiB\n")
	var doc struct {
		Lines []struct {
			No    int    `json:"no"`
			Level string `json:"level"`
		} `json:"lines"`
	}
	w := get(t, h, "/api/runs/flashnext/log")
	if w.Code != http.StatusOK {
		t.Fatalf("the log: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Lines) != 2 {
		t.Fatalf("the log holds %d lines, want 2", len(doc.Lines))
	}
	if doc.Lines[1].Level != "error" {
		t.Errorf("the line of the finding is %q, want error", doc.Lines[1].Level)
	}
}
