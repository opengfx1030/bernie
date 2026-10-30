package setup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSteps are the steps of the fake plan of the tests: a root step, a
// step that is done, a step that runs, a step that fails and a step that
// never runs.
func fakeSteps(dir string) []Step {
	return []Step{
		{ID: "groups", Title: "Add the user to the groups", Root: true, Commands: []string{"sudo usermod -aG video op"}},
		{ID: "old", Title: "A step that is done", Done: true, Commands: []string{"echo SHOULD-NOT-RUN"}},
		{ID: "a", Title: "The step that runs", Commands: []string{"echo ran-a > " + dir + "/a"}},
		{ID: "b", Title: "The step that fails", Commands: []string{"exit 3"}},
		{ID: "c", Title: "The step after the failure", Commands: []string{"echo SHOULD-NOT-RUN"}},
	}
}

// TestApplyRunRecordsEveryStep: the record of a run holds the state, the
// time, the exit code and the log of every step the run touched.
func TestApplyRunRecordsEveryStep(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	var events []Event
	run, err := ApplyRun(context.Background(), fakeSteps(dir), nil, io.Discard, ApplyOptions{
		Store: store,
		Event: func(e Event) { events = append(events, e) },
	})
	if err == nil {
		t.Fatal("the failing step must stop the run")
	}
	if run.State != RunFailed {
		t.Errorf("the run is %s, want %s", run.State, RunFailed)
	}
	if run.Error == "" {
		t.Error("the run must say why it failed")
	}
	// The states of the steps, in the order of the plan.
	want := []struct {
		id    string
		state string
	}{
		{"groups", StepBlocked},
		{"old", StepSkipped},
		{"a", StepDone},
		{"b", StepFailed},
	}
	if len(run.Steps) != len(want) {
		t.Fatalf("the run holds %d steps, want %d: %+v", len(run.Steps), len(want), run.Steps)
	}
	for i, w := range want {
		got := run.Steps[i]
		if got.ID != w.id || got.State != w.state {
			t.Errorf("step %d is %s/%s, want %s/%s", i, got.ID, got.State, w.id, w.state)
		}
		if got.Summary == "" {
			t.Errorf("step %s has no summary", got.ID)
		}
	}
	// The step that ran: its times, its exit code and its log.
	ran := run.Step("a")
	if ran.StartedAt == nil || ran.EndedAt == nil {
		t.Error("the step that ran has no times")
	}
	if ran.ExitCode != 0 {
		t.Errorf("the step that ran exited %d, want 0", ran.ExitCode)
	}
	if ran.LogPath == "" {
		t.Fatal("the step that ran has no log")
	}
	log, err := os.ReadFile(ran.LogPath)
	if err != nil {
		t.Fatalf("the log of the step is missing: %v", err)
	}
	text := string(log)
	if !strings.Contains(text, "$ echo ran-a") {
		t.Errorf("the log does not carry the command line:\n%s", text)
	}
	if !strings.Contains(text, "ran-a") {
		t.Errorf("the log does not carry the output:\n%s", text)
	}
	if !strings.Contains(text, "exit=0") {
		t.Errorf("the log does not carry the exit code:\n%s", text)
	}
	// The step that failed: the exit code of the command.
	failed := run.Step("b")
	if failed.ExitCode != 3 {
		t.Errorf("the failed step exited %d, want 3", failed.ExitCode)
	}
	if !strings.Contains(failed.Summary, "failed") {
		t.Errorf("the summary of the failed step is %q", failed.Summary)
	}
	// The step after the failure never ran.
	if run.Step("c") != nil {
		t.Error("the step after the failure must not be in the record")
	}
	// The file of the record is there and no temporary file is left.
	if _, err := os.Stat(filepath.Join(store.Dir(run.ID), "run.json")); err != nil {
		t.Errorf("the record of the run is missing: %v", err)
	}
	entries, err := os.ReadDir(store.Dir(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("the atomic write left %s behind", e.Name())
		}
	}
}

// TestApplyRunEventOrder: the events of a run come in the order of the
// run: the start, the steps, the end.
func TestApplyRunEventOrder(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	var events []Event
	_, err := ApplyRun(context.Background(), fakeSteps(dir), nil, io.Discard, ApplyOptions{
		Store: store,
		Event: func(e Event) { events = append(events, e) },
	})
	if err == nil {
		t.Fatal("the failing step must stop the run")
	}
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Type)
	}
	want := "run_started step_blocked step_skipped step_started step_done step_started step_done run_done"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("the events are %q, want %q", got, want)
	}
	for _, e := range events {
		if e.RunID == "" {
			t.Errorf("the event %s has no run id", e.Type)
		}
	}
}

// TestApplyRunOnly: a run covers only the steps it was asked for.
func TestApplyRunOnly(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	run, err := ApplyRun(context.Background(), fakeSteps(dir), []string{"a"}, io.Discard, ApplyOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Steps) != 1 || run.Steps[0].ID != "a" {
		t.Fatalf("the run holds %+v, want the step a only", run.Steps)
	}
	if run.State != RunDone {
		t.Errorf("the run is %s, want %s", run.State, RunDone)
	}
	if _, err := os.Stat(store.StepLog(run.ID, "a")); err != nil {
		t.Errorf("the log of the step is missing: %v", err)
	}
}

// TestApplyRunUnknownStep: an unknown step id is an error and nothing runs.
func TestApplyRunUnknownStep(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	run, err := ApplyRun(context.Background(), fakeSteps(dir), []string{"nope"}, io.Discard, ApplyOptions{Store: store})
	if err == nil || !strings.Contains(err.Error(), "unknown step") {
		t.Fatalf("err = %v, want the unknown step", err)
	}
	if run != nil {
		t.Error("a run that never started must not be recorded")
	}
}

// TestApplyRunEvidence: the check of the venv leaves its JSON in the
// record of the step.
func TestApplyRunEvidence(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	probe := `{"python": "3.12.14", "torch": "2.12.0+rocm7.14.0", "hip": "7.14.60850", ` +
		`"vllm": "0.31.0+rdna", "rdna_ops": 144, "all_reduce": true, "rocblas": "/v/lib/librocblas.so.5", "rocblas_wheel": true}`
	steps := []Step{{ID: "venv.verify", Title: "Verify the vLLM build in the venv",
		Commands: []string{"echo BERNIE_JSON:" + probe}}}
	run, err := ApplyRun(context.Background(), steps, nil, io.Discard, ApplyOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	step := run.Step("venv.verify")
	if len(step.Evidence) == 0 {
		t.Fatal("the step has no evidence")
	}
	var got map[string]any
	if err := json.Unmarshal(step.Evidence, &got); err != nil {
		t.Fatalf("the evidence is not JSON: %v", err)
	}
	if got["rdna_ops"] != float64(144) {
		t.Errorf("the evidence holds %v, want 144 RDNA ops", got["rdna_ops"])
	}
	if !strings.Contains(step.Summary, "torch 2.12.0+rocm7.14.0") {
		t.Errorf("the summary is %q, want the versions of the venv", step.Summary)
	}
}

// TestStoreKeepsTenRuns: the store keeps the last ten runs.
func TestStoreKeepsTenRuns(t *testing.T) {
	store := &RunStore{Root: filepath.Join(t.TempDir(), "setup")}
	for i := range 12 {
		run := &Run{ID: NewRunID(), State: RunDone, StartedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute)}
		if err := store.Create(run); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != KeepRuns {
		t.Errorf("the store holds %d runs, want %d", len(runs), KeepRuns)
	}
	latest, err := store.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != runs[0].ID {
		t.Errorf("the latest run is %s, want %s", latest.ID, runs[0].ID)
	}
	// The list is newest first.
	for i := 1; i < len(runs); i++ {
		if runs[i].StartedAt.After(runs[i-1].StartedAt) {
			t.Errorf("the runs are not newest first: %s before %s", runs[i].ID, runs[i-1].ID)
		}
	}
}

// TestStoreResumeAfterACrash: the record of a run that died is still
// there: a reader sees the state the run had.
func TestStoreResumeAfterACrash(t *testing.T) {
	store := &RunStore{Root: filepath.Join(t.TempDir(), "setup")}
	run := &Run{ID: NewRunID(), State: RunRunning, StartedAt: time.Now().UTC()}
	if err := store.Create(run); err != nil {
		t.Fatal(err)
	}
	// The run dies: nothing writes the record again.
	got, err := store.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != RunRunning {
		t.Errorf("the record of the dead run is %s, want %s", got.State, RunRunning)
	}
	if len(got.Steps) != 0 {
		t.Errorf("the record of the dead run holds %d steps", len(got.Steps))
	}
}

// TestStoreWriteIsAtomic: a reader never sees a half-written record.
func TestStoreWriteIsAtomic(t *testing.T) {
	store := &RunStore{Root: filepath.Join(t.TempDir(), "setup")}
	run := &Run{ID: NewRunID(), State: RunRunning, StartedAt: time.Now().UTC()}
	if err := store.Create(run); err != nil {
		t.Fatal(err)
	}
	run.Steps = []RunStep{{ID: "a", State: StepRunning}}
	if err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(store.Dir(run.ID), "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Run
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("the record is not JSON: %v", err)
	}
	if len(got.Steps) != 1 || got.Steps[0].ID != "a" {
		t.Errorf("the record holds %+v, want the step a", got.Steps)
	}
}

// TestFormatDuration: the durations read the way the CLI shows them.
func TestFormatDuration(t *testing.T) {
	for in, want := range map[float64]string{
		0:    "0.0s",
		1.25: "1.2s",
		59.9: "59.9s",
		60:   "1m00s",
		252:  "4m12s",
		3661: "1h01m01s",
		-5:   "0.0s",
	} {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
