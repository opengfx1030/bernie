package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A log with real vLLM lines (from the build box).
const phaseLog = `# bernie 2026-10-28T10:00:00Z: python -m vllm.entrypoints.cli.main serve org/m
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:   0% Completed | 0/5 [00:00<?, ?it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  20% Completed | 1/5 [00:00<00:01,  2.80it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:03<00:00,  1.57it/s]
`

// writeRun records a host run named name with the given log.
func writeRun(t *testing.T, name, log string) *Record {
	t.Helper()
	dir := RunDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "serve.log")
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &Record{Name: name, Recipe: "r", Mode: "host", Host: "127.0.0.1", Port: 18000,
		StartedAt: time.Now().UTC().Truncate(time.Second), Log: logPath}
	if err := writeRecord(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestPhaseReadsTheTailOfARunLog(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	rec := writeRun(t, "one", phaseLog)
	p, ok := Phase(context.Background(), *rec)
	if !ok {
		t.Fatal("no phase")
	}
	if p.ID != "loading-weights" {
		t.Errorf("phase = %q, want loading-weights", p.ID)
	}
	if p.Sentence != "Loading the model weights, 5 of 5 shards" {
		t.Errorf("sentence = %q", p.Sentence)
	}
	if p.Done != 5 || p.Total != 5 {
		t.Errorf("progress = %d/%d, want 5/5", p.Done, p.Total)
	}
}

func TestPhaseOfAnUnknownLogIsNothing(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	rec := writeRun(t, "two", "hello\nworld\n")
	if p, ok := Phase(context.Background(), *rec); ok {
		t.Errorf("unknown log gave phase %q", p.ID)
	}
}
