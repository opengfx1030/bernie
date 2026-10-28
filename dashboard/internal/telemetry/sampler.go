package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// telemetry is one event of GET /api/telemetry.
type telemetry struct {
	Time int64        `json:"time"` // Unix milliseconds
	GPUs []gpu.Sample `json:"gpus"`
	Runs []runMetrics `json:"runs"`
}

// runMetrics is the live vLLM state of one run that is not exited.
type runMetrics struct {
	Name  string      `json:"name"`
	URL   string      `json:"url"`
	State string      `json:"state"`
	Stats *vllm.Stats `json:"stats,omitempty"`
	// Token rates from counter deltas between two samples (0 on the first).
	GenTokS    float64 `json:"gen_tok_s"`
	PromptTokS float64 `json:"prompt_tok_s"`
	Error      string  `json:"error,omitempty"`
}

// counters remembers the previous sample of a run for rate computation.
type counters struct {
	at          time.Time
	gen, prompt float64
}

// sampler holds state that lives as long as one hub sampler goroutine.
type sampler struct {
	mu   sync.Mutex
	gpus []gpu.GPU
	gpuT time.Time
	prev map[string]counters
}

// sampleTelemetry builds one telemetry document. It is the hub's sample func;
// the hub calls it from a single goroutine.
func (s *server) sampleTelemetry(ctx context.Context) []byte {
	sp := &s.sampler
	sp.mu.Lock()
	defer sp.mu.Unlock()
	now := time.Now()
	// The GPU list changes only with hardware; re-read it every 30 s.
	if sp.gpus == nil || now.Sub(sp.gpuT) > 30*time.Second {
		g, _ := listGPUs()
		sp.gpus, sp.gpuT = g, now
	}
	t := telemetry{Time: now.UnixMilli(), GPUs: gpu.Telemetry(root, sp.gpus), Runs: []runMetrics{}}
	if t.GPUs == nil {
		t.GPUs = []gpu.Sample{}
	}

	runs, _ := runner.List(ctx)
	live := make([]runner.Status, 0, len(runs))
	for _, r := range runs {
		if r.State != runner.StateExited && r.Port > 0 {
			live = append(live, r)
		}
	}
	t.Runs = make([]runMetrics, len(live))
	var wg sync.WaitGroup
	for i, r := range live {
		wg.Add(1)
		go func(i int, r runner.Status) {
			defer wg.Done()
			t.Runs[i] = scrape(ctx, r)
		}(i, r)
	}
	wg.Wait()

	seen := make(map[string]bool, len(t.Runs))
	if sp.prev == nil {
		sp.prev = map[string]counters{}
	}
	for i := range t.Runs {
		m := &t.Runs[i]
		seen[m.Name] = true
		if m.Stats == nil {
			continue
		}
		cur := counters{at: now, gen: m.Stats.GenerationTokens, prompt: m.Stats.PromptTokens}
		if p, ok := sp.prev[m.Name]; ok && now.Sub(p.at) < 5*time.Second { // stale after a sampler pause
			m.GenTokS = rate(p.gen, cur.gen, now.Sub(p.at))
			m.PromptTokS = rate(p.prompt, cur.prompt, now.Sub(p.at))
		}
		sp.prev[m.Name] = cur
	}
	for name := range sp.prev {
		if !seen[name] {
			delete(sp.prev, name)
		}
	}
	b, err := json.Marshal(t)
	if err != nil {
		return []byte(`{"error":` + strconv.Quote(err.Error()) + `}`)
	}
	return b
}

// rate is the per-second increase of a counter; a reset (restart) gives 0.
func rate(prev, cur float64, dt time.Duration) float64 {
	if dt <= 0 || cur < prev {
		return 0
	}
	return (cur - prev) / dt.Seconds()
}

func scrape(ctx context.Context, r runner.Status) runMetrics {
	u := r.URL()
	m := runMetrics{Name: r.Name, URL: u, State: r.State}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	samples, err := vllm.New(u).Metrics(ctx)
	if err != nil {
		if r.State != runner.StateStarting { // vLLM serves /metrics only once it is up
			m.Error = fmt.Sprint(err)
		}
		return m
	}
	st := vllm.Summarize(samples)
	m.Stats = &st
	return m
}
