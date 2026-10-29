package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// root is the host filesystem root the probes read. It is "/" on the real
// host; BERNIE_ROOT points it at a captured host tree (spec §4).
var root = hostroot.Root()

// slowEvery is the time between two slow passes over the cards. The
// temperatures, the caps and the clocks move slowly, and every read of an
// amdgpu attribute talks to the card: on a host with ten cards the slow
// pass costs as much as four fast passes.
//
// The cadence is a multiple of the step of the history (two seconds): every
// slow pass then lands on a point of the ring, and the points in between
// carry no slow value. The history never carries a slow value forward: a
// point without a slow measurement holds a gap.
const slowEvery = 4 * time.Second

// document is one event of GET /api/telemetry.
type document struct {
	Time int64        `json:"time"` // Unix milliseconds
	GPUs []gpu.Sample `json:"gpus"`
	Runs []runMetrics `json:"runs"`
	// SlowAgeS is the age in seconds of the slow fields (the temperatures,
	// the caps, the clocks). The fast fields are one second old at most.
	SlowAgeS float64 `json:"slow_age_s"`
	// SlowIntervalS is the time between two slow passes: the slow fields
	// are that old at most.
	SlowIntervalS float64 `json:"slow_interval_s"`
}

// runMetrics is the live vLLM state of one run that is not exited.
type runMetrics struct {
	Name  string      `json:"name"`
	URL   string      `json:"url"`
	State string      `json:"state"`
	Stats *vllm.Stats `json:"stats,omitempty"`
	// Window is what the server did in the last minute. It is absent
	// until the sampler has read the server twice: a window needs two
	// readings.
	Window *vllm.Window `json:"window,omitempty"`
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

// Sampler holds state that lives as long as one hub sampler goroutine.
type Sampler struct {
	mu   sync.Mutex
	gpus []gpu.GPU
	gpuT time.Time
	prev map[string]counters
	// watchers read the server of every run and keep the readings of the
	// last minute, so the stream can say what the server did in it.
	watchers map[string]*runWatcher
	history  *History // the ring buffer of the last minutes (nil = no history)
	// slow is the last slow pass over the cards; it is merged into the
	// fast samples until the next one.
	slow   []gpu.Sample
	slowAt time.Time
	// now is the clock of the sampler. The test sets it to drive the tier
	// schedule without waiting.
	now func() time.Time
	// readFast and readSlow read the two tiers of the attributes of the
	// cards. The test counts the calls.
	readFast func(root string, gpus []gpu.GPU) []gpu.Sample
	readSlow func(root string, gpus []gpu.GPU) []gpu.Sample
}

// clock returns the time of the sampler.
func (s *Sampler) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// fast reads the fast tier of the attributes of the cards.
func (s *Sampler) fast(gpus []gpu.GPU) []gpu.Sample {
	if s.readFast != nil {
		return s.readFast(root, gpus)
	}
	return gpu.TelemetryFast(root, gpus)
}

// slowPass reads the slow tier of the attributes of the cards.
func (s *Sampler) slowPass(gpus []gpu.GPU) []gpu.Sample {
	if s.readSlow != nil {
		return s.readSlow(root, gpus)
	}
	return gpu.TelemetrySlow(root, gpus)
}

// SetHistory gives the sampler the ring buffer it fills. The hub owns it.
func (s *Sampler) SetHistory(h *History) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = h
}

// Sample builds one telemetry document. It is the hub's sample function;
// the hub calls it from a single goroutine.
func (s *Sampler) Sample(ctx context.Context) []byte {
	sp := s
	sp.mu.Lock()
	defer sp.mu.Unlock()
	now := sp.clock()
	// The GPU list changes only with hardware; re-read it every 30 s.
	if sp.gpus == nil || now.Sub(sp.gpuT) > 30*time.Second {
		g, _ := listGPUs()
		sp.gpus, sp.gpuT = g, now
	}
	// The fast tier is read every second, the slow tier every slowEvery.
	// The slow values are merged into the fast samples: the answer holds
	// one value per field, and the age of the slow ones says how old they
	// are.
	cards := sp.fast(sp.gpus)
	slowFresh := false
	if sp.slow == nil || now.Sub(sp.slowAt) >= slowEvery {
		sp.slow = sp.slowPass(sp.gpus)
		sp.slowAt = now
		slowFresh = true
	}
	for i := range cards {
		if i < len(sp.slow) {
			gpu.MergeSlow(&cards[i], &sp.slow[i])
		}
	}
	t := document{Time: now.UnixMilli(), GPUs: cards, Runs: []runMetrics{}}
	t.SlowAgeS = now.Sub(sp.slowAt).Seconds()
	t.SlowIntervalS = slowEvery.Seconds()
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
		w := sp.watcher(r.Name, r.URL())
		wg.Add(1)
		go func(i int, r runner.Status, w *vllm.Watcher) {
			defer wg.Done()
			t.Runs[i] = scrape(ctx, r, w)
		}(i, r, w)
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
	for name := range sp.watchers {
		if !seen[name] {
			delete(sp.watchers, name)
		}
	}
	if sp.history != nil {
		// The output and prefill rates of the fleet: the rates are
		// properties of the server, so every card of the history carries
		// them.
		fleet, fleetPrompt := 0.0, 0.0
		for i := range t.Runs {
			fleet += t.Runs[i].GenTokS
			fleetPrompt += t.Runs[i].PromptTokS
		}
		sp.history.Add(now, t.GPUs, fleet, fleetPrompt, slowFresh)
	}
	b, err := json.Marshal(t)
	if err != nil {
		return []byte(`{"error":` + strconv.Quote(err.Error()) + `}`)
	}
	return b
}

// listGPUs returns the KFD GPUs; a host without KFD has none (not an error).
func listGPUs() ([]gpu.GPU, error) {
	vis, err := gpu.Visible(root)
	if gpu.Restricted(err) {
		return []gpu.GPU{}, err
	}
	gpus := vis.GPUs
	if errors.Is(err, fs.ErrNotExist) {
		return []gpu.GPU{}, nil
	}
	if gpus == nil {
		gpus = []gpu.GPU{}
	}
	return gpus, err
}

// rate is the per-second increase of a counter; a reset (restart) gives 0.
func rate(prev, cur float64, dt time.Duration) float64 {
	if dt <= 0 || cur < prev {
		return 0
	}
	return (cur - prev) / dt.Seconds()
}

func scrape(ctx context.Context, r runner.Status, w *vllm.Watcher) runMetrics {
	u := r.URL()
	m := runMetrics{Name: r.Name, URL: u, State: r.State}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	reading, err := w.Sample(ctx)
	if err != nil {
		if r.State != runner.StateStarting { // vLLM serves /metrics only once it is up
			m.Error = fmt.Sprint(err)
		}
		return m
	}
	m.Stats = &reading.Stats
	if reading.Window.Seconds > 0 {
		m.Window = &reading.Window
	}
	return m
}

// watcher returns the watcher of a run: it reads the server of the run and
// keeps the readings of the last minute. A run that moved to another
// address gets a new watcher.
func (sp *Sampler) watcher(name, url string) *vllm.Watcher {
	if sp.watchers == nil {
		sp.watchers = map[string]*runWatcher{}
	}
	rw, ok := sp.watchers[name]
	if !ok || rw.url != url {
		rw = &runWatcher{url: url, Watcher: vllm.NewWatcher(url, vllm.DefaultWindow)}
		sp.watchers[name] = rw
	}
	return rw.Watcher
}

// runWatcher reads the server of one run. It keeps the address of the run
// beside the watcher: a run that moved gets a new watcher.
type runWatcher struct {
	url string
	*vllm.Watcher
}
