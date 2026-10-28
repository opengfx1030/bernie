package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/setup"
)

// fakeAction is an action that runs shell commands: the setup steps of the
// host plan run them the same way.
func fakeAction(id string, commands ...string) *Action {
	return &Action{Step: StepModel, Kind: ActionSetup,
		Steps: []setup.Step{{ID: id, Title: "fake", Commands: commands}}}
}

// waitJob waits until the job is not running any more.
func waitJob(t *testing.T, id string) *Job {
	t.Helper()
	for range 200 {
		j, err := GetJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State != JobRunning {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s is still running", id)
	return nil
}

func TestAJobRunsTheActionAndKeepsItsLines(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	job, err := startJob(context.Background(), StepModel, fakeAction("model.download", "echo one", "echo two"))
	if err != nil {
		t.Fatal(err)
	}
	got := waitJob(t, job.ID)
	if got.State != JobDone {
		t.Fatalf("job = %+v", got)
	}
	lines, err := JobLog(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "one") || !strings.Contains(joined, "two") {
		t.Errorf("lines = %v", lines)
	}
	if got.Phase == "" {
		t.Error("the job has no phase")
	}
}

func TestAFailingJobKeepsItsError(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	job, err := startJob(context.Background(), StepModel, fakeAction("model.download", "exit 3"))
	if err != nil {
		t.Fatal(err)
	}
	got := waitJob(t, job.ID)
	if got.State != JobFailed {
		t.Fatalf("job = %+v", got)
	}
	if got.Error == "" {
		t.Error("the job has no error")
	}
	if got.FinishedAt == nil {
		t.Error("the job has no end time")
	}
}

func TestOneJobRunsAtATime(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	job, err := startJob(context.Background(), StepModel, fakeAction("model.download", "sleep 2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startJob(context.Background(), StepModel, fakeAction("model.download", "echo x")); err != ErrJobBusy {
		t.Fatalf("second job = %v, want ErrJobBusy", err)
	}
	waitJob(t, job.ID)
}

func TestJobsSurviveAReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BERNIE_STATE_DIR", dir)
	job, err := startJob(context.Background(), StepModel, fakeAction("model.download", "echo one"))
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, job.ID)
	// The state of the job is on disk: a new process finds it.
	if _, err := os.Stat(filepath.Join(dir, "jobs", job.ID, "job.json")); err != nil {
		t.Fatalf("the job is not on disk: %v", err)
	}
	jobs := RecentJobs()
	if len(jobs) != 1 || jobs[0].ID != job.ID || jobs[0].State != JobDone {
		t.Fatalf("jobs = %+v", jobs)
	}
	// A job that was running when the process ended is failed, not running.
	other := filepath.Join(dir, "jobs", "20260101T000000Z-abcd")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"id": "20260101T000000Z-abcd", "step": "model", "state": "running", "started_at": "2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(other, "job.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs = RecentJobs()
	var found bool
	for _, j := range jobs {
		if j.ID != "20260101T000000Z-abcd" {
			continue
		}
		found = true
		if j.State != JobFailed {
			t.Errorf("the job of a dead process is %q, want failed", j.State)
		}
	}
	if !found {
		t.Error("the job of a dead process is gone")
	}
}
