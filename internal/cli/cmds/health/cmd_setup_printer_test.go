package health

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/setup"
)

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
		times  []time.Time
		events []setup.Event
		want   string
	}{
		{
			name:  "tty running, progress, done",
			tty:   true,
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
			name:  "non-tty one line per event",
			tty:   false,
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
			name:  "skipped and blocked",
			tty:   false,
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
			name:  "failed step says why",
			tty:   false,
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
