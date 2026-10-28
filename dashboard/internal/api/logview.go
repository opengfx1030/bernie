package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// The levels of a log line. The rules are the ones of the log doctor, in
// one place, so the page and the doctor never disagree about a line.
const (
	levelAll   = "all"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

// The limits of the log endpoint.
const (
	defaultTail = 2000
	maxTail     = 20000
	logBudget   = 10 * time.Second
)

// logLine is one line of the log: its number in the log, its level and its
// text.
type logLine struct {
	No    int    `json:"no"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// logDoc is the answer of GET /api/runs/{name}/log.
type logDoc struct {
	Name       string    `json:"name"`
	State      string    `json:"state"`
	TotalLines int       `json:"total_lines"`
	Truncated  bool      `json:"truncated"`
	Lines      []logLine `json:"lines"`
}

// findingsDoc is the answer of GET /api/runs/{name}/findings.
type findingsDoc struct {
	Name    string         `json:"name"`
	Matches []logdoc.Match `json:"matches"`
}

// GET /api/runs/{name}/log?tail=2000&level=error&q=oom — the tail of the log
// of a run. Every line carries its number in the log and its level; `level`
// is all, info, warn or error and `q` is a case-insensitive substring. The
// lines are the tail of the log, oldest first.
//
// A line that a finding of the log doctor points at is an error, whatever it
// holds.
func (h *handler) getRunLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tail := defaultTail
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > maxTail {
			httpjson.Error(w, http.StatusBadRequest,
				"tail must be an integer between 0 and "+strconv.Itoa(maxTail))
			return
		}
		tail = n
	}
	level := r.URL.Query().Get("level")
	if level == "" {
		level = levelAll
	}
	switch level {
	case levelAll, levelInfo, levelWarn, levelError:
	default:
		httpjson.Error(w, http.StatusBadRequest, "level must be one of all, info, warn or error")
		return
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	run, err := findRun(r.Context(), name)
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), logBudget)
	defer cancel()
	doc, err := tailLog(ctx, run, tail, level, q)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Redacted(w, http.StatusOK, doc)
}

// GET /api/runs/{name}/findings — the known failures in the log of a run:
// the scan of `bernie doctor --run`, with the same rules.
func (h *handler) getRunFindings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := findRun(r.Context(), name); err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	rep, err := app.RunLog(r.Context(), name)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Redacted(w, http.StatusOK, findingsDoc{Name: name, Matches: rep.Matches})
}

// findRun returns the run with this name and its state.
func findRun(ctx context.Context, name string) (runner.Status, error) {
	runs, err := runner.List(ctx)
	if err != nil {
		return runner.Status{}, err
	}
	for _, r := range runs {
		if r.Name == name {
			return r, nil
		}
	}
	return runner.Status{}, fmt.Errorf("no run named %q", name)
}

// tailLog reads the log of a run and keeps the last tail lines that pass the
// level and the search. The lines that a finding points at are errors.
func tailLog(ctx context.Context, run runner.Status, tail int, level, q string) (logDoc, error) {
	doc := logDoc{Name: run.Name, State: run.State, Lines: []logLine{}}
	found, err := findingLines(ctx, run)
	if err != nil {
		return logDoc{}, err
	}
	rc, err := runner.OpenLog(ctx, run.Record, logBudget)
	if err != nil {
		return logDoc{}, err
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	lines := []logLine{}
	matched := 0
	for sc.Scan() {
		doc.TotalLines++
		text := sc.Text()
		lvl := lineLevel(text)
		if found[doc.TotalLines] {
			lvl = levelError // a known failure is an error, whatever it holds
		}
		if !keepLevel(lvl, level) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(text), q) {
			continue
		}
		matched++
		if tail == 0 {
			continue
		}
		lines = append(lines, logLine{No: doc.TotalLines, Level: lvl, Text: text})
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
	}
	if err := sc.Err(); err != nil {
		return logDoc{}, err
	}
	doc.Truncated = matched > tail
	doc.Lines = lines
	return doc, nil
}

// findingLines returns the numbers of the lines a finding of the log doctor
// points at.
func findingLines(ctx context.Context, run runner.Status) (map[int]bool, error) {
	rules, err := logdoc.Load()
	if err != nil {
		return nil, err
	}
	rc, err := runner.OpenLog(ctx, run.Record, logBudget)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	out := map[int]bool{}
	for _, m := range logdoc.Scan(rc, rules) {
		out[m.LineNo] = true
	}
	return out, nil
}

// lineLevel is the level of a log line.
func lineLevel(text string) string {
	switch {
	case strings.Contains(text, "ERROR"), strings.Contains(text, "Traceback"),
		strings.Contains(text, "RuntimeError"), strings.Contains(text, "CUDA error"),
		strings.Contains(text, "Error:"):
		return levelError
	case strings.Contains(text, "WARNING"), strings.Contains(text, "WARN"):
		return levelWarn
	}
	return levelInfo
}

// keepLevel reports whether a line of this level passes the filter.
func keepLevel(level, want string) bool {
	switch want {
	case levelAll:
		return true
	case levelInfo:
		return level == levelInfo
	case levelWarn:
		return level == levelWarn
	case levelError:
		return level == levelError
	}
	return false
}
