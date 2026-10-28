package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// jobHandler returns the API routes with a state directory of its own.
func jobHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("BERNIE_STATE_DIR", state)
	mux := http.NewServeMux()
	Register(mux, Options{})
	return mux, state
}

// writeJob writes a job document and the log of its output.
func writeJob(t *testing.T, state, id, step, jobState string, lines []string, started time.Time, finished bool) {
	t.Helper()
	dir := filepath.Join(state, "jobs", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	job := map[string]any{"id": id, "step": step, "state": jobState, "started_at": started}
	if finished {
		job["finished_at"] = started.Add(time.Minute)
	}
	b, _ := json.Marshal(job)
	if err := os.WriteFile(filepath.Join(dir, "job.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	for _, l := range lines {
		log.WriteString(l + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "job.log"), []byte(log.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestJobsList checks the list of the jobs: an array, newest first, with the
// fields the page needs to re-attach.
func TestJobsList(t *testing.T) {
	h, state := jobHandler(t)
	old := time.Now().UTC().Add(-time.Hour)
	writeJob(t, state, "old-job", "model", "done", []string{"done"}, old, true)
	writeJob(t, state, "new-job", "serve", "running", []string{"busy"}, time.Now().UTC(), false)

	w := get(t, h, "/api/jobs")
	if w.Code != http.StatusOK {
		t.Fatalf("the jobs: %d %s", w.Code, w.Body.String())
	}
	var jobs []struct {
		ID         string `json:"id"`
		Step       string `json:"step"`
		State      string `json:"state"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
		Phase      string `json:"phase"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("the jobs are not a JSON array: %v\n%s", err, w.Body.String())
	}
	if len(jobs) != 2 {
		t.Fatalf("the list holds %d jobs, want 2", len(jobs))
	}
	if jobs[0].ID != "new-job" || jobs[1].ID != "old-job" {
		t.Fatalf("the list is not newest first: %+v", jobs)
	}
	if jobs[0].Step != "serve" || jobs[0].StartedAt == "" {
		t.Errorf("the first job is %+v", jobs[0])
	}
	// A job that was running when the process that wrote it ended is
	// marked failed: the list never shows a job that nobody runs.
	if jobs[0].State != "failed" {
		t.Errorf("the state of the first job is %q, want failed", jobs[0].State)
	}
	if jobs[1].FinishedAt == "" {
		t.Errorf("the finished job has no end: %+v", jobs[1])
	}
}

// TestJobsListGuards checks the guard of the limit.
func TestJobsListGuards(t *testing.T) {
	h, _ := jobHandler(t)
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=1000"} {
		if w := get(t, h, "/api/jobs"+q); w.Code != http.StatusBadRequest {
			t.Errorf("the query %q: got %d, want 400", q, w.Code)
		}
	}
}

// TestJobEventsReplayTheLines checks that the stream of a job replays its
// buffered lines from the beginning: a page that reloads finds the whole
// output again.
func TestJobEventsReplayTheLines(t *testing.T) {
	h, state := jobHandler(t)
	writeJob(t, state, "job-1", "model", "failed", []string{"first", "second", "third"}, time.Now().UTC(), true)
	w := get(t, h, "/api/jobs/job-1/events")
	if w.Code != http.StatusOK {
		t.Fatalf("the events: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for i, want := range []string{"first", "second", "third"} {
		id := "id: " + strconv.Itoa(i+1)
		if !strings.Contains(body, id) {
			t.Errorf("the stream lacks the event %s:\n%s", id, body)
		}
		if !strings.Contains(body, want) {
			t.Errorf("the stream lacks the line %q:\n%s", want, body)
		}
	}
	// The lines come in the order of the log.
	if strings.Index(body, "first") > strings.Index(body, "second") ||
		strings.Index(body, "second") > strings.Index(body, "third") {
		t.Errorf("the lines are out of order:\n%s", body)
	}
	// The stream ends with the state of the job.
	if !strings.Contains(body, "event: failed") {
		t.Errorf("the stream does not end with the state of the job:\n%s", body)
	}
}

// TestJobEventsReloadReplaysTheWholeOutput checks that a second reader of the
// same job gets the same lines: the page reloads and finds the output again.
func TestJobEventsReloadReplaysTheWholeOutput(t *testing.T) {
	h, state := jobHandler(t)
	writeJob(t, state, "job-2", "model", "running", []string{"one", "two"}, time.Now().UTC(), false)
	first := jobEvents(t, h, "/api/jobs/job-2/events")
	second := jobEvents(t, h, "/api/jobs/job-2/events")
	for _, want := range []string{"one", "two"} {
		if !strings.Contains(first, want) || !strings.Contains(second, want) {
			t.Fatalf("the output %q is missing:\n%s\n---\n%s", want, first, second)
		}
	}
	if !strings.Contains(first, "id: 1") || !strings.Contains(second, "id: 1") {
		t.Errorf("the replay does not start at the first line:\n%s\n---\n%s", first, second)
	}
}

// TestJobEventsLastEventID checks that a client that re-attaches gets the
// lines after the last one it saw.
func TestJobEventsLastEventID(t *testing.T) {
	h, state := jobHandler(t)
	writeJob(t, state, "job-3", "model", "failed", []string{"one", "two", "three"}, time.Now().UTC(), true)
	r := httptest.NewRequest(http.MethodGet, "/api/jobs/job-3/events", nil)
	r.Header.Set("Last-Event-ID", "2")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if strings.Contains(body, "one") || strings.Contains(body, "two") {
		t.Errorf("the stream repeats the lines the client has:\n%s", body)
	}
	if !strings.Contains(body, "three") {
		t.Errorf("the stream lacks the new line:\n%s", body)
	}
	if !strings.Contains(body, "id: 3") {
		t.Errorf("the stream lacks the id of the line:\n%s", body)
	}
}

// jobEvents reads the events of a job with a deadline: a job that still runs
// keeps the stream open.
func jobEvents(t *testing.T, h http.Handler, target string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Body.String()
}

// TestJobNotFound checks the answer of an unknown job.
func TestJobNotFound(t *testing.T) {
	h, _ := jobHandler(t)
	if w := get(t, h, "/api/jobs/nope"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown job: got %d, want 404", w.Code)
	}
	if w := get(t, h, "/api/jobs/nope/events"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown job: got %d, want 404", w.Code)
	}
}
