package flow

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

const (
	// PhaseTailBytes is the size of the log tail that Phase reads. The tail
	// holds the last lines of the log, so a multi-gigabyte log costs this
	// much I/O and no more.
	PhaseTailBytes = 256 << 10
	// PhaseBudget bounds one tail read.
	PhaseBudget = 5 * time.Second
	// PhasePrintInterval is the shortest time between two phase sentences.
	PhasePrintInterval = 5 * time.Second
)

// Phase reads the tail of the log of a run and returns what the server is
// doing now. It returns false when the log says nothing known or cannot be
// read.
func Phase(ctx context.Context, r runner.Record) (logdoc.Phase, bool) {
	ctx, cancel := context.WithTimeout(ctx, PhaseBudget)
	defer cancel()
	tail, err := runner.TailBytes(ctx, r, PhaseTailBytes)
	if err != nil {
		return logdoc.Phase{}, false
	}
	return logdoc.PhaseOf(tail)
}

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
