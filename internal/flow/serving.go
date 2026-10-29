package flow

import (
	"context"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// servingTimeout bounds the read of the metrics of one server: the status
// must not wait for a server that does not answer.
const servingTimeout = time.Second

// Serving is what the server of one live run measures: the numbers of the
// server since it started (spec §11). The measures the server does not
// publish are absent.
type Serving struct {
	Name  string      `json:"name"`
	URL   string      `json:"url"`
	State string      `json:"state"`
	Stats *vllm.Stats `json:"stats,omitempty"`
	// Error says why the server published nothing.
	Error string `json:"error,omitempty"`
}

// serving reads the metrics of the server of every run that serves. A
// server that does not answer is a gap with its reason, not a failure of
// the status.
func serving(ctx context.Context, runs []Run) []Serving {
	out := make([]Serving, 0, len(runs))
	for _, r := range runs {
		if r.State == runner.StateExited || r.Port == 0 {
			continue
		}
		s := Serving{Name: r.Name, URL: r.URL(), State: r.State}
		ctx, cancel := context.WithTimeout(ctx, servingTimeout)
		reading, err := vllm.NewWatcher(s.URL, vllm.DefaultWindow).Sample(ctx)
		cancel()
		if err != nil {
			s.Error = err.Error()
		} else {
			s.Stats = &reading.Stats
		}
		out = append(out, s)
	}
	return out
}
