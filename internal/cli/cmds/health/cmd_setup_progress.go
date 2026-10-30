package health

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/setup"
)

// This file prints the progress of a setup run (spec §6.4). The printer
// has two modes:

//
//   - TTY (the live line): one line on screen, updated while the step
//     runs. step_started writes a "running" line with the elapsed time
//     and the latest phase; progress events update the line in place;
//     step_done replaces it with the "done" line and ends with a
//     newline so the next line covers it.
//
//   - non-TTY (CI, agents, files): one line per event. step_started
//     writes "[N/M] step: running", step_done writes
//     "done in T, log: PATH", progress events write a throttled
//     "[N/M] step: sentence" line at most once every ten seconds so a
//     log file stays readable.
//
// The printer never prints the raw step output ("== step: title",
// "$ uv pip ...", BERNIE_JSON evidence) on its own: the applier
// suppresses the markers when a printer is wired and the stepWriter
// strips BERNIE_JSON. Pass --verbose to stream the raw output anyway;
// the printer then falls back to the non-TTY line style so the lines
// interleave cleanly instead of fighting the live CR line.

// progressInterval is the minimum time between two non-TTY progress
// lines for the same step. The throttle keeps a log file readable
// while a long step runs.
const progressInterval = 10 * time.Second

// runPrinter prints the progress of a setup run: one line per step, the
// line updated while the step runs on a TTY; one line per event on
// non-TTY outputs.
type runPrinter struct {
	w io.Writer
	// events receives the events of the run when the output is JSON.
	events func(setup.Event)
	mu     sync.Mutex
	// width is the length of the last live line: the next line pads it
	// on a TTY so the text does not leave pieces behind.
	width int
	// start is when the step that runs now started.
	start time.Time
	// phase is the progress sentence of the step that runs now.
	phase string
	// index and total place the step in the run.
	index, total int
	// current is the id of the step that runs now.
	current string
	// active says that a step runs now.
	active bool
	// tty says the printer updates a single live line. False means one
	// line per event.
	tty bool
	// lastProgress is the last time the non-TTY path wrote a progress
	// line for the step that runs now. The throttle keeps the output
	// readable while the step measures bytes, ninja targets, etc.
	lastProgress time.Time
	// now is the clock of the printer. The tests fix it.
	now func() time.Time
}

// clock is the clock of the printer.
func (p *runPrinter) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// event prints one event of a run.
func (p *runPrinter) event(ev setup.Event) {
	if p.events != nil {
		p.events(ev)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch ev.Type {
	case "step_started":
		p.start = ev.At
		p.phase = ""
		p.index, p.total = ev.Index, ev.Total
		if ev.Step != nil {
			p.current = ev.Step.ID
		}
		p.active = true
		p.lastProgress = ev.At
		p.line(p.running())
	case "progress":
		if !p.active || ev.Step == nil || ev.Step.Progress == nil {
			return
		}
		p.phase = ev.Step.Progress.Sentence
		if p.tty {
			p.line(p.running())
			return
		}
		if p.clock().Sub(p.lastProgress) < progressInterval {
			return
		}
		p.lastProgress = p.clock()
		p.line(fmt.Sprintf("[%d/%d] %s: %s", p.index, p.total, p.stepID(), p.phase))
	case "step_done":
		p.active = false
		p.done(ev)
	case "step_skipped":
		p.line(fmt.Sprintf("       %s: skipped (%s)", ev.Step.ID, ev.Step.Summary))
	case "step_blocked":
		p.line(fmt.Sprintf("       %s: blocked (%s)", ev.Step.ID, ev.Step.Summary))
	}
}

// running is the line of the step that runs now. The live TTY path
// keeps it on screen while the step measures its work.
func (p *runPrinter) running() string {
	elapsed := setup.FormatDuration(p.clock().Sub(p.start).Seconds())
	if p.tty {
		line := fmt.Sprintf("[%d/%d] %s: running... %s", p.index, p.total, p.stepID(), elapsed)
		if p.phase != "" {
			line += " (" + p.phase + ")"
		}
		return line
	}
	return fmt.Sprintf("[%d/%d] %s: running", p.index, p.total, p.stepID())
}

// stepID is the id of the step that runs now.
func (p *runPrinter) stepID() string {
	if p.current == "" {
		return "step"
	}
	return p.current
}

// done prints the last line of a step. On a TTY the done line replaces
// the running line and the row closes with a newline so the cursor sits
// at the start of the next one.
func (p *runPrinter) done(ev setup.Event) {
	if ev.Step == nil {
		return
	}
	took := setup.FormatDuration(ev.Step.DurationS)
	switch ev.Step.State {
	case setup.StepFailed:
		p.line(fmt.Sprintf("[%d/%d] %s: failed in %s: %s, log: %s",
			ev.Index, ev.Total, ev.Step.ID, took, ev.Step.Summary, cli.OrDash(ev.Step.LogPath)))
	default:
		p.line(fmt.Sprintf("[%d/%d] %s: done in %s, log: %s",
			ev.Index, ev.Total, ev.Step.ID, took, cli.OrDash(ev.Step.LogPath)))
	}
}

// line writes a line. The TTY path pads it and returns the cursor to
// the start of the row so the next call covers it; non-TTY writes the
// line and ends with a newline so the output is one event per line.
func (p *runPrinter) line(text string) {
	if p.tty {
		if p.width > len(text) {
			text += strings.Repeat(" ", p.width-len(text))
		}
		fmt.Fprintf(p.w, "\r%s", text)
		p.width = len(text)
		return
	}
	fmt.Fprintln(p.w, text)
}

// finish ends the last line of the run. TTY: a final newline so the
// cursor drops below the running line. Non-TTY: nothing to do.
func (p *runPrinter) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty && p.width > 0 {
		fmt.Fprintln(p.w)
		p.width = 0
	}
}
