package health

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/setup"
)

// TestPrintRunGolden: the report of a run reads the same on every host.
func TestPrintRunGolden(t *testing.T) {
	run := &setup.Run{
		ID: "20261030T101112Z-abc123", State: setup.RunDone, Mode: "host", Venv: "/v",
		StartedAt: time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC), DurationS: 252,
		Steps: []setup.RunStep{
			{ID: "venv.create", Title: "Create the Python venv", State: setup.StepDone, DurationS: 1.2,
				LogPath: "/state/setup/run/venv.create.log", Summary: "done"},
			{ID: "venv.verify", Title: "Verify the vLLM build in the venv", State: setup.StepDone, DurationS: 30,
				LogPath:  "/state/setup/run/venv.verify.log",
				Summary:  "torch 2.12.0+rocm7.14.0, hip 7.14.60850, vllm 0.31.0+rdna, 144 RDNA ops, all_reduce registered",
				Evidence: json.RawMessage(`{"python":"3.12.14","torch":"2.12.0+rocm7.14.0","hip":"7.14.60850","vllm":"0.31.0+rdna","device_count":8,"rocm_ops":159,"rdna_ops":144,"all_reduce":true,"rocm_c":"/v/lib/vllm/_rocm_C.abi3.so","rocblas":"/v/lib/librocblas.so.5","rocblas_wheel":true}`)},
		},
	}
	var buf bytes.Buffer
	printRun(&buf, run)
	want := `run 20261030T101112Z-abc123  done  mode host  venv /v  in 4m12s

STEP	STATE	DURATION	LOG
venv.create	done	1.2s	/state/setup/run/venv.create.log
venv.verify	done	30.0s	/state/setup/run/venv.verify.log
  torch 2.12.0+rocm7.14.0, hip 7.14.60850, vllm 0.31.0+rdna, 144 RDNA ops, all_reduce registered

evidence of venv.verify:
  python 3.12.14
  torch 2.12.0+rocm7.14.0, hip 7.14.60850, 8 devices
  vllm 0.31.0+rdna
  ops: 144 RDNA of 159, all_reduce true
  extension /v/lib/vllm/_rocm_C.abi3.so
  rocBLAS /v/lib/librocblas.so.5 (from the wheel of the venv)
`
	if got := buf.String(); got != want {
		t.Errorf("the report reads:\n%s\nwant:\n%s", got, want)
	}
}

// TestRunPrinterGolden: the stream of a run is one line per step, the
// line updated while the step runs.
func TestRunPrinterGolden(t *testing.T) {
	start := time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC)
	var buf bytes.Buffer
	p := &runPrinter{w: &buf, now: func() time.Time { return start.Add(12 * time.Second) }}
	p.event(setup.Event{Type: "step_started", At: start, Index: 1, Total: 2,
		Step: &setup.RunStep{ID: "venv.torch", Title: "Install the wheels"}})
	p.event(setup.Event{Type: "progress", At: start, Index: 1, Total: 2,
		Step: &setup.RunStep{ID: "venv.torch", Progress: &setup.Progress{Sentence: "installing the packages"}}})
	p.event(setup.Event{Type: "step_done", At: start, Index: 1, Total: 2,
		Step: &setup.RunStep{ID: "venv.torch", State: setup.StepDone, DurationS: 252, LogPath: "/state/setup/run/venv.torch.log"}})
	p.finish()
	want := "\r[1/2] venv.torch: running... 12.0s" +
		"\r[1/2] venv.torch: running... 12.0s (installing the packages)" +
		"\r[1/2] venv.torch: done in 4m12s, log: /state/setup/run/venv.torch.log\n"
	if got := buf.String(); got != want {
		t.Errorf("the stream reads:\n%q\nwant:\n%q", got, want)
	}
}

// TestRunPrinterFailedStep: a step that failed says why and where its log
// is.
func TestRunPrinterFailedStep(t *testing.T) {
	start := time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC)
	var buf bytes.Buffer
	p := &runPrinter{w: &buf, now: func() time.Time { return start }}
	p.event(setup.Event{Type: "step_done", At: start, Index: 2, Total: 2,
		Step: &setup.RunStep{ID: "venv.verify", State: setup.StepFailed, DurationS: 3,
			Summary: "failed: exit status 1", LogPath: "/state/setup/run/venv.verify.log"}})
	p.finish()
	got := buf.String()
	for _, want := range []string{"venv.verify: failed in 3.0s", "failed: exit status 1", "log: /state/setup/run/venv.verify.log"} {
		if !strings.Contains(got, want) {
			t.Errorf("the line %q does not carry %q", got, want)
		}
	}
}

// TestRunPrinterJSONKeepsTheOrder: with --json the events keep their
// order and the record of the run comes last.
func TestRunPrinterJSONKeepsTheOrder(t *testing.T) {
	var kinds []string
	p := &runPrinter{events: func(ev setup.Event) { kinds = append(kinds, ev.Type) }}
	events := []setup.Event{
		{Type: "run_started"},
		{Type: "step_started"},
		{Type: "progress"},
		{Type: "step_done"},
		{Type: "run_done"},
	}
	for _, ev := range events {
		p.event(ev)
	}
	want := "run_started step_started progress step_done run_done"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("the events are %q, want %q", got, want)
	}
}
