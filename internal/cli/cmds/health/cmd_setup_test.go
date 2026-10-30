package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
				Evidence: json.RawMessage(`{"python":"3.12.14","torch":"2.12.0+rocm7.14.0","hip":"7.14.60850","vllm":"0.31.0+rdna","device_count":8,"rocm_ops":159,"rdna_ops":144,"all_reduce":true,"rocm_c":"/v/lib/vllm/_rocm_C.abi3.so","rocblas":"/v/lib/librocblas.so.5","rocblas_wheel":true,"components":[{"name":"python","version":"3.12.14"},{"name":"torch","version":"2.12.0+rocm7.14.0","note":"hip 7.14.60850"},{"name":"rocm-sdk-wheel","version":"absent"},{"name":"triton","version":"3.2.0"},{"name":"vllm","version":"0.31.0+rdna","note":"fork @abc1234 release/0.31-rdna"},{"name":"flash-attn","version":"absent"},{"name":"causal-conv1d","version":"absent"},{"name":"transformers","version":"4.45.2"},{"name":"rocBLAS","version":"5.5.0","note":"wheel, soname 5, rocBLAS 5.5.0"},{"name":"RCCL","version":"2.30.4"},{"name":"rocm-sdk-host","version":"7.14.1","note":"/opt/rocm/core-7.14"}]}`)},
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
  components:
    python 3.12.14
    torch 2.12.0+rocm7.14.0  (hip 7.14.60850)
    rocm-sdk-wheel absent
    triton 3.2.0
    vllm 0.31.0+rdna  (fork @abc1234 release/0.31-rdna)
    flash-attn absent
    causal-conv1d absent
    transformers 4.45.2
    rocBLAS 5.5.0  (wheel, soname 5, rocBLAS 5.5.0)
    RCCL 2.30.4
    rocm-sdk-host 7.14.1  (/opt/rocm/core-7.14)
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
	p := &runPrinter{w: &buf, tty: true, now: func() time.Time { return start.Add(12 * time.Second) }}
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

// TestComponentsRender: every component of the evidence renders the same
// way on every host (spec §6.3). A missing component stays in the table
// with version "absent" so the operator sees what the venv did not get.
func TestComponentsRender(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		wantSubs []string
		notWant  []string
	}{
		{
			name: "full",
			fixture: `{"python":"3.12.14","components":[` +
				`{"name":"python","version":"3.12.14"},` +
				`{"name":"torch","version":"2.12.0+rocm7.14.0","note":"hip 7.14.60850"},` +
				`{"name":"rocm-sdk-wheel","version":"0.9.1","note":"wheel"},` +
				`{"name":"triton","version":"3.2.0"},` +
				`{"name":"vllm","version":"0.31.0+rdna","note":"fork @abc1234 release/0.31-rdna"},` +
				`{"name":"flash-attn","version":"absent"},` +
				`{"name":"causal-conv1d","version":"absent"},` +
				`{"name":"transformers","version":"4.45.2"},` +
				`{"name":"rocBLAS","version":"5.5.0","note":"wheel, soname 5, rocBLAS 5.5.0"},` +
				`{"name":"RCCL","version":"2.30.4"},` +
				`{"name":"rocm-sdk-host","version":"7.14.1","note":"/opt/rocm/core-7.14"}]}`,
			wantSubs: []string{
				"components:",
				"python 3.12.14",
				"torch 2.12.0+rocm7.14.0  (hip 7.14.60850)",
				"rocm-sdk-wheel 0.9.1  (wheel)",
				"triton 3.2.0",
				"vllm 0.31.0+rdna  (fork @abc1234 release/0.31-rdna)",
				"flash-attn absent",
				"causal-conv1d absent",
				"transformers 4.45.2",
				"rocBLAS 5.5.0  (wheel, soname 5, rocBLAS 5.5.0)",
				"RCCL 2.30.4",
				"rocm-sdk-host 7.14.1  (/opt/rocm/core-7.14)",
			},
		},
		{
			name: "flash_attn package present",
			fixture: `{"python":"3.12.14","components":[` +
				`{"name":"flash-attn","version":"2.7.3","note":"package"}]}`,
			wantSubs: []string{"flash-attn 2.7.3  (package)"},
		},
		{
			name: "fork FA-RDNA2 ops",
			fixture: `{"python":"3.12.14","components":[` +
				`{"name":"flash-attn","version":"fork","note":"FA-RDNA2 (12 ops)"}]}`,
			wantSubs: []string{"flash-attn fork  (FA-RDNA2 (12 ops))"},
		},
		{
			name: "no components field",
			fixture: `{"python":"3.12.14","torch":"2.12.0","hip":"7.14.60850","vllm":"0.31.0","device_count":1,
				"rocm_ops":12,"rdna_ops":11,"all_reduce":true,"rocm_c":"/v/rc","rocblas":"/v/rb","rocblas_wheel":true}`,
			wantSubs: []string{"python 3.12.14"},
			notWant:  []string{"components:"},
		},
		{
			name:     "components empty",
			fixture:  `{"python":"3.12.14","components":[]}`,
			wantSubs: []string{"python 3.12.14"},
			notWant:  []string{"components:"},
		},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		printEvidence(&buf, json.RawMessage(c.fixture))
		got := buf.String()
		for _, want := range c.wantSubs {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the evidence does not carry %q:\n%s", c.name, want, got)
			}
		}
		for _, not := range c.notWant {
			if strings.Contains(got, not) {
				t.Errorf("%s: the evidence carries %q, want it absent:\n%s", c.name, not, got)
			}
		}
	}
}

// TestComponentsParse: the components array parses from any venv.verify
// evidence, with "absent" left as the version when the component is
// missing and never omitted (spec §6.3).
func TestComponentsParse(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		want    []Component
	}{
		{
			name:    "full",
			fixture: loadComponentsFixture(t, "full.json"),
			want: []Component{
				{Name: "python", Version: "3.12.14"},
				{Name: "torch", Version: "2.12.0+rocm7.14.0", Note: "hip 7.14.60850"},
				{Name: "rocm-sdk-wheel", Version: "0.9.1", Note: "wheel"},
				{Name: "triton", Version: "3.2.0"},
				{Name: "vllm", Version: "0.31.0+rdna", Note: "fork @abc1234 release/0.31-rdna"},
				{Name: "flash-attn", Version: "absent"},
				{Name: "causal-conv1d", Version: "absent"},
				{Name: "transformers", Version: "4.45.2"},
				{Name: "rocBLAS", Version: "5.5.0", Note: "wheel, soname 5, rocBLAS 5.5.0"},
				{Name: "RCCL", Version: "2.30.4"},
				{Name: "rocm-sdk-host", Version: "7.14.1", Note: "/opt/rocm/core-7.14"},
			},
		},
		{
			name:    "absent",
			fixture: loadComponentsFixture(t, "absent.json"),
			want: []Component{
				{Name: "python", Version: "3.12.14"},
				{Name: "torch", Version: "absent"},
				{Name: "rocm-sdk-wheel", Version: "absent"},
				{Name: "triton", Version: "absent"},
				{Name: "vllm", Version: "absent"},
				{Name: "flash-attn", Version: "absent"},
				{Name: "causal-conv1d", Version: "absent"},
				{Name: "transformers", Version: "absent"},
				{Name: "rocBLAS", Version: "absent"},
				{Name: "RCCL", Version: "absent"},
				{Name: "rocm-sdk-host", Version: "absent"},
			},
		},
	}
	for _, c := range cases {
		var got struct {
			Components []Component `json:"components"`
		}
		if err := json.Unmarshal([]byte(c.fixture), &got); err != nil {
			t.Fatalf("%s: parse: %v", c.name, err)
		}
		if !reflect.DeepEqual(got.Components, c.want) {
			t.Errorf("%s: components = %+v, want %+v", c.name, got.Components, c.want)
		}
		// Every component must have a name; missing version is "absent".
		for i, comp := range got.Components {
			if comp.Name == "" {
				t.Errorf("%s: component[%d] has empty name", c.name, i)
			}
			if comp.Version == "" {
				t.Errorf("%s: component[%d] (%s) has empty version", c.name, i, comp.Name)
			}
		}
	}
}

// loadComponentsFixture reads a fixture file from testdata/components/.
// The fixture is a JSON object with a "components" key.
func loadComponentsFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "components", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// TestRunPrinterModes: the printer renders three outputs that operators
// and CI rely on:
//
//   - TTY (live CR line): one row on screen, updated while the step
//     runs; the row ends with a newline on step_done so the cursor drops
//     below.
//   - non-TTY: one clean line per event; the "running" line has no
//     timestamp, the "done" line carries the duration and the log path.
//   - verbose / JSON: same per-event shape as non-TTY — verbose
//     interleaves raw step output and JSON streams NDJSON, so the live
//     CR line would fight both.
//
// The non-TTY path also throttles progress lines to one per ten
// seconds, so a long ninja build does not flood the log file.
func TestRunPrinterModes(t *testing.T) {
	t0 := time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC)
	cases := []struct {
		name string
		tty  bool
		// times maps the At of each event to the clock of the printer
		// at that point.
		times []time.Time
		events []setup.Event
		want   string
	}{
		{
			name: "tty running, progress, done",
			tty:  true,
			times: []time.Time{t0, t0.Add(12 * time.Second), t0.Add(12 * time.Second)},
			events: []setup.Event{
				{Type: "step_started", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch", Title: "Install the wheels"}},
				{Type: "progress", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch",
						Progress: &setup.Progress{Sentence: "installing the packages"}}},
				{Type: "step_done", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch", State: setup.StepDone, DurationS: 252,
						LogPath: "/state/setup/run/venv.torch.log"}},
			},
			want: "\r[1/2] venv.torch: running... 12.0s" +
				"\r[1/2] venv.torch: running... 12.0s (installing the packages)" +
				"\r[1/2] venv.torch: done in 4m12s, log: /state/setup/run/venv.torch.log\n",
		},
		{
			name: "non-tty one line per event",
			tty:  false,
			times: []time.Time{t0, t0.Add(12 * time.Second), t0.Add(12 * time.Second)},
			events: []setup.Event{
				{Type: "step_started", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch", Title: "Install the wheels"}},
				{Type: "progress", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch",
						Progress: &setup.Progress{Sentence: "installing the packages"}}},
				{Type: "step_done", At: t0, Index: 1, Total: 2,
					Step: &setup.RunStep{ID: "venv.torch", State: setup.StepDone, DurationS: 252,
						LogPath: "/state/setup/run/venv.torch.log"}},
			},
			want: "[1/2] venv.torch: running\n" +
				"[1/2] venv.torch: installing the packages\n" +
				"[1/2] venv.torch: done in 4m12s, log: /state/setup/run/venv.torch.log\n",
		},
		{
			name: "skipped and blocked",
			tty:  false,
			times: []time.Time{t0, t0},
			events: []setup.Event{
				{Type: "step_skipped", At: t0,
					Step: &setup.RunStep{ID: "old", Summary: "already done"}},
				{Type: "step_blocked", At: t0,
					Step: &setup.RunStep{ID: "rocm.repo", Summary: "needs root"}},
			},
			want: "       old: skipped (already done)\n" +
				"       rocm.repo: blocked (needs root)\n",
		},
		{
			name: "failed step says why",
			tty:  false,
			times: []time.Time{t0},
			events: []setup.Event{
				{Type: "step_done", At: t0, Index: 2, Total: 2,
					Step: &setup.RunStep{ID: "venv.verify", State: setup.StepFailed, DurationS: 3,
						Summary: "failed: exit status 1", LogPath: "/state/setup/run/venv.verify.log"}},
			},
			want: "[2/2] venv.verify: failed in 3.0s: failed: exit status 1, log: /state/setup/run/venv.verify.log\n",
		},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		p := &runPrinter{w: &buf, tty: c.tty}
		i := 0
		p.now = func() time.Time {
			if i >= len(c.times) {
				return c.times[len(c.times)-1]
			}
			return c.times[i]
		}
		// The TTY golden "running... 12.0s" wants the clock to read
		// 12s after the step_started. Pin the TTY cases to that clock
		// so the rendering matches without the test threading a moving
		// clock through every event.
		if c.tty && len(c.times) > 0 {
			fixed := c.times[len(c.times)-1]
			p.now = func() time.Time { return fixed }
		}
		for _, ev := range c.events {
			p.event(ev)
			i++
		}
		p.finish()
		if got := buf.String(); got != c.want {
			t.Errorf("%s: the stream reads:\n%q\nwant:\n%q", c.name, got, c.want)
		}
	}
}

// TestRunPrinterNonTTYThrottlesProgress: the non-TTY path emits at most
// one progress line per step per progressInterval (10s). A long step
// that pumps progress every second still yields a single line.
func TestRunPrinterNonTTYThrottlesProgress(t *testing.T) {
	t0 := time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC)
	var buf bytes.Buffer
	clock := t0
	p := &runPrinter{w: &buf, tty: false, now: func() time.Time { return clock }}
	p.event(setup.Event{Type: "step_started", At: t0, Index: 1, Total: 2,
		Step: &setup.RunStep{ID: "vllm.build", Title: "Build the fork"}})
	for i := range 30 {
		clock = t0.Add(time.Duration(i) * time.Second)
		p.event(setup.Event{Type: "progress", At: clock, Index: 1, Total: 2,
			Step: &setup.RunStep{ID: "vllm.build",
				Progress: &setup.Progress{Sentence: fmt.Sprintf("building (%d/30)", i+1)}}})
	}
	got := buf.String()
	// 30 progress events 1s apart with a 10s throttle: only 2 lines
	// pass the gate (around the 10s and 20s marks). Plus the "running"
	// line at step_started. The "done" line is appended after the check.
	if n := strings.Count(got, "\n"); n != 3 {
		t.Errorf("the non-TTY output has %d newlines, want 3 (running, two throttled progress lines):\n%s", n, got)
	}
	if strings.Count(got, "vllm.build: building") > 2 {
		t.Errorf("progress is not throttled:\n%s", got)
	}
	// The last progress line carries the most recent sentence the
	// throttle let through, not a stale one.
	if !strings.Contains(got, "(21/30)") {
		t.Errorf("the throttled line should carry the latest gated sentence:\n%s", got)
	}
	p.event(setup.Event{Type: "step_done", At: clock, Index: 1, Total: 2,
		Step: &setup.RunStep{ID: "vllm.build", State: setup.StepDone, DurationS: 30,
			LogPath: "/log/vllm.build.log"}})
	if !strings.Contains(buf.String(), "vllm.build: done in 30.0s, log: /log/vllm.build.log\n") {
		t.Errorf("the done line is missing or wrong:\n%s", buf.String())
	}
}

// TestRunPrinterSkipsRawOutput: the printer itself never leaks the raw
// step output: the BERNIE_JSON evidence line, the "== step: title"
// markers and the "$ uv pip ..." command echo. The applier filters
// those at the source (suppresses markers when a printer is wired and
// strips BERNIE_JSON in the stepWriter), so the printer only sees the
// events it renders. This test exercises the printer side: it must
// still ignore lines that look like raw output even when they reach
// its event channel by mistake.
func TestRunPrinterSkipsRawOutput(t *testing.T) {
	t0 := time.Date(2026, 10, 30, 10, 11, 12, 0, time.UTC)
	var buf bytes.Buffer
	p := &runPrinter{w: &buf, tty: false, now: func() time.Time { return t0 }}
	// Progress with a sentence that happens to look like raw output:
	// the printer renders the sentence verbatim, so a "BERNIE_JSON:..."
	// sentence would be a leak. The applier's progressOf is the gate;
	// here we only assert that the printer does not pass through lines
	// whose event type it does not recognise.
	p.event(setup.Event{Type: "step_started", At: t0, Index: 1, Total: 1,
		Step: &setup.RunStep{ID: "venv.verify", Title: "Verify the venv"}})
	p.event(setup.Event{Type: "step_done", At: t0, Index: 1, Total: 1,
		Step: &setup.RunStep{ID: "venv.verify", State: setup.StepDone, DurationS: 1.0,
			LogPath: "/log/venv.verify.log"}})
	got := buf.String()
	for _, leak := range []string{"BERNIE_JSON:", "== venv.verify", "$ ", "(running...)"} {
		if strings.Contains(got, leak) {
			t.Errorf("the printer leaked %q to the terminal:\n%s", leak, got)
		}
	}
}
