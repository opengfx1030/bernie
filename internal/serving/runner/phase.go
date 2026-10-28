package runner

import (
	"context"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
)

const (
	// PhaseTailBytes is the size of the log tail that Phase reads. The tail
	// holds the last lines of the log, so a multi-gigabyte log costs this
	// much I/O and no more.
	PhaseTailBytes = 256 << 10
	// PhaseBudget bounds one tail read.
	PhaseBudget = 5 * time.Second
)

// Phase reads the tail of the log of a run and returns what the server is
// doing now (spec §5.4). It returns false when the log says nothing known
// or cannot be read.
func Phase(ctx context.Context, r Record) (logdoc.Phase, bool) {
	ctx, cancel := context.WithTimeout(ctx, PhaseBudget)
	defer cancel()
	tail, err := TailBytes(ctx, r, PhaseTailBytes)
	if err != nil {
		return logdoc.Phase{}, false
	}
	return logdoc.PhaseOf(tail)
}
