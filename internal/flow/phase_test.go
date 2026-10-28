package flow

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
)

// A log with real vLLM lines (from the build box).
const phaseLog = `# bernie 2026-10-28T10:00:00Z: python -m vllm.entrypoints.cli.main serve org/m
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:   0% Completed | 0/5 [00:00<?, ?it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  20% Completed | 1/5 [00:00<00:01,  2.80it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:03<00:00,  1.57it/s]
`

func TestRunsCarriesThePhaseOfALiveRun(t *testing.T) {
	stateEnv(t)
	pid, port := liveVLLM(t)
	writeRun(t, "live", phaseLog, pid, port)
	runs, err := Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.Name != "live" {
			continue
		}
		if r.Phase == nil {
			t.Fatal("live run has no phase")
		}
		if r.Phase.ID != "loading-weights" {
			t.Errorf("phase = %q", r.Phase.ID)
		}
		return
	}
	t.Fatal("live run not found")
}

func TestPhasePrinterWritesOneLinePerChange(t *testing.T) {
	var buf bytes.Buffer
	var p phasePrinter
	p.w = &buf
	p.print("run", logdoc.Phase{ID: "loading-weights", Sentence: "Loading the model weights"})
	if buf.String() != "run: Loading the model weights\n" {
		t.Fatalf("first line = %q", buf.String())
	}
	p.print("run", logdoc.Phase{ID: "loading-weights", Sentence: "Loading the model weights"})
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("the same sentence was printed again: %q", buf.String())
	}
	p.print("run", logdoc.Phase{ID: "compiling", Sentence: "Compiling the model"})
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("a change inside the rate limit was printed: %q", buf.String())
	}
	p.lastAt = time.Now().Add(-time.Minute) // the rate limit has passed
	p.print("run", logdoc.Phase{ID: "compiling", Sentence: "Compiling the model"})
	if strings.Count(buf.String(), "\n") != 2 {
		t.Errorf("the change was not printed: %q", buf.String())
	}
}

func TestPhasePrinterWithoutAWriterPrintsNothing(t *testing.T) {
	var p phasePrinter
	p.print("run", logdoc.Phase{ID: "ready", Sentence: "The server is ready"})
}
