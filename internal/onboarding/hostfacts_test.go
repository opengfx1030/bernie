package onboarding

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
)

// TestFactsOfTheHost checks the facts of the document: every fact has a key,
// a label, a value and the reason why it matters.
func TestFactsOfTheHost(t *testing.T) {
	f := &Facts{
		Host:   host.Info{RAMBytes: 64 << 30},
		GPUs:   []gpu.GPU{{HIPIndex: 0, Name: "Radeon PRO V620", GFX: "gfx1030"}, {HIPIndex: 1, Name: "Radeon PRO V620", GFX: "gfx1030"}},
		Recipe: "flashnext",
		Best:   "flashnext",
		Fits:   []string{"flashnext"},
		Venv:   "/opt/venv", VenvSource: "config",
		Model: "Qwen/Qwen3-8B",
	}
	facts := factsOf(f)
	if len(facts) == 0 {
		t.Fatal("the host has no facts")
	}
	seen := map[string]Fact{}
	for _, fact := range facts {
		if fact.ID == "" || fact.Label == "" || fact.Value == "" {
			t.Errorf("the fact %+v is incomplete", fact)
		}
		if fact.Why == "" {
			t.Errorf("the fact %q has no reason", fact.ID)
		}
		seen[fact.ID] = fact
	}
	gpus, ok := seen["gpus"]
	if !ok {
		t.Fatalf("the facts hold no gpus: %+v", facts)
	}
	if gpus.Value != "2× gfx1030 Radeon PRO V620" {
		t.Errorf("the gpus are %q", gpus.Value)
	}
	if gpus.Why != "Every card is gfx1030, so one recipe family fits them all." {
		t.Errorf("the reason of the gpus is %q", gpus.Why)
	}
	if mem := seen["memory"]; mem.Value != "64.0 GiB" {
		t.Errorf("the memory is %q", mem.Value)
	}
	if py := seen["python"]; py.Value != "/opt/venv (config)" {
		t.Errorf("the python is %q", py.Value)
	}
	if m := seen["model"]; m.Value != "Qwen/Qwen3-8B" {
		t.Errorf("the model is %q", m.Value)
	}
}

// TestBuildCarriesTheFacts checks that the document of the onboarding holds
// the facts.
func TestBuildCarriesTheFacts(t *testing.T) {
	doc := Build(&Facts{Host: host.Info{RAMBytes: 32 << 30}})
	if len(doc.Facts) == 0 {
		t.Fatal("the document holds no facts")
	}
}

// TestStepsCarryTheJobOfTheStep checks that a step names the job that made
// it: the page re-attaches to it with the id.
func TestStepsCarryTheJobOfTheStep(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	f := &Facts{Jobs: []Job{
		{ID: "job-old", Step: StepModel, State: JobDone, StartedAt: old, FinishedAt: &old},
		{ID: "job-new", Step: StepModel, State: JobRunning, StartedAt: now},
		{ID: "job-serve", Step: StepServe, State: JobDone, StartedAt: now, FinishedAt: &now},
	}}
	doc := Build(f)
	steps := map[string]Step{}
	for _, s := range doc.Steps {
		steps[s.ID] = s
	}
	model := steps[StepModel]
	if model.JobID != "job-new" || model.JobState != JobRunning {
		t.Errorf("the model step carries %q (%q)", model.JobID, model.JobState)
	}
	serve := steps[StepServe]
	if serve.JobID != "job-serve" || serve.JobState != JobDone {
		t.Errorf("the serve step carries %q (%q)", serve.JobID, serve.JobState)
	}
	// A job that finished long ago belongs to no step.
	if hw := steps[StepHardware]; hw.JobID != "" {
		t.Errorf("the hardware step carries the job %q", hw.JobID)
	}
}

// TestJobProgressReadsTheDownload checks the progress of a job: the phase
// and the numbers of a download.
func TestJobProgressReadsTheDownload(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j1", Step: "model", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s")
	if job.Progress == nil {
		t.Fatal("the job has no progress")
	}
	p := job.Progress
	if p.Phase != "model.download" {
		t.Errorf("the phase is %q", p.Phase)
	}
	if p.BytesDone != 1_200_000_000 || p.BytesTotal != 2_300_000_000 {
		t.Errorf("the bytes are %d of %d", p.BytesDone, p.BytesTotal)
	}
	if p.RateBPS != 45_200_000 {
		t.Errorf("the rate is %d", p.RateBPS)
	}
	if p.ETAS != 24 {
		t.Errorf("the time to finish is %d s, want 24", p.ETAS)
	}
}

// TestJobProgressOfAnotherStep checks that a step that counts no bytes
// still carries its phase.
func TestJobProgressOfAnotherStep(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j2", Step: "serve", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("the server answers /health")
	if job.Progress == nil || job.Progress.Phase != "serve" {
		t.Fatalf("the progress is %+v", job.Progress)
	}
	if job.Progress.BytesTotal != 0 {
		t.Errorf("the progress holds bytes: %+v", job.Progress)
	}
}
