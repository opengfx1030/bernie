package flow

import (
	"fmt"
	"io"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
)

const (
	// PhasePrintInterval is the shortest time between two phase sentences.
	PhasePrintInterval = 5 * time.Second
)

// phasePrinter writes the phase sentence of a run when it changes. It writes
// at most one line every PhasePrintInterval.
type phasePrinter struct {
	w      io.Writer
	last   string
	lastAt time.Time
}

// print writes the sentence of a run when it differs from the last one and
// the rate limit has passed. A phase that changes faster than the limit is
// printed on the next tick.
func (p *phasePrinter) print(name string, ph logdoc.Phase) {
	if p.w == nil || ph.Sentence == "" || ph.Sentence == p.last {
		return
	}
	if !p.lastAt.IsZero() && time.Since(p.lastAt) < PhasePrintInterval {
		return
	}
	fmt.Fprintf(p.w, "%s: %s\n", name, ph.Sentence)
	p.last, p.lastAt = ph.Sentence, time.Now()
}
