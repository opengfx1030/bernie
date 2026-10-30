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

// This file prints the progress of a setup run (spec §6.4).

// runPrinter prints the progress of a setup run: one line per step, the
// line updated while the step runs.
type runPrinter struct {
	w io.Writer
	// events receives the events of the run when the output is JSON.
	events func(setup.Event)
	mu     sync.Mutex
	// width is the length of the line on the screen: the next line pads
	// it, so the text does not leave pieces behind.
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
		p.line(p.running())
	case "progress":
		if !p.active || ev.Step == nil || ev.Step.Progress == nil {
			return
		}
		p.phase = ev.Step.Progress.Sentence
		p.line(p.running())
	case "step_done":
		p.active = false
		p.done(ev)
	case "step_skipped":
		p.line(fmt.Sprintf("       %s: skipped (%s)", ev.Step.ID, ev.Step.Summary))
	case "step_blocked":
		p.line(fmt.Sprintf("       %s: blocked (%s)", ev.Step.ID, ev.Step.Summary))
	}
}

// running is the line of the step that runs now.
func (p *runPrinter) running() string {
	line := fmt.Sprintf("[%d/%d] %s: running... %s", p.index, p.total, p.stepID(), setup.FormatDuration(p.clock().Sub(p.start).Seconds()))
	if p.phase != "" {
		line += " (" + p.phase + ")"
	}
	return line
}

// stepID is the id of the step that runs now.
func (p *runPrinter) stepID() string {
	if p.current == "" {
		return "step"
	}
	return p.current
}

// done prints the last line of a step.
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

// line writes a line and pads it, so the next line covers it.
func (p *runPrinter) line(text string) {
	if p.width > len(text) {
		text += strings.Repeat(" ", p.width-len(text))
	}
	fmt.Fprintf(p.w, "\r%s", text)
	p.width = len(text)
}

// finish ends the last line of the run.
func (p *runPrinter) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.width > 0 {
		fmt.Fprintln(p.w)
		p.width = 0
	}
}
