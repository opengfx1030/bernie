package telemetry

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// The sampler reads the cards in two tiers: the fast tier (the load, the
// memory in use, the power) every second, the slow tier (the temperatures,
// the caps, the clocks) every slowEvery. The test drives the schedule with
// a fake clock: no second of the plan is waited for.
func TestTierSchedule(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	var fast, slow int
	s := &Sampler{
		now:  func() time.Time { return now },
		gpus: []gpu.GPU{{HIPIndex: 0, SysfsPath: "sys/class/drm/card0/device"}},
		gpuT: now,
		readFast: func(string, []gpu.GPU) []gpu.Sample {
			fast++
			return []gpu.Sample{{HIPIndex: 0, BusyPct: 42, PowerW: 30, VRAMUsed: 7}}
		},
		readSlow: func(string, []gpu.GPU) []gpu.Sample {
			slow++
			clk := 1000 + slow
			return []gpu.Sample{{HIPIndex: 0, TempC: float64(50 + slow), SclkMHz: &clk, VRAMTotal: 99}}
		},
	}

	// The first sample reads both tiers.
	doc := sampleDoc(t, s)
	if fast != 1 || slow != 1 {
		t.Fatalf("after the first sample: fast=%d slow=%d, want 1 and 1", fast, slow)
	}
	if doc.SlowAgeS != 0 {
		t.Errorf("slow age %v s, want 0", doc.SlowAgeS)
	}
	if len(doc.GPUs) != 1 {
		t.Fatalf("the sample holds %d cards, want 1", len(doc.GPUs))
	}
	g := doc.GPUs[0]
	if g.BusyPct != 42 || g.PowerW != 30 || g.VRAMUsed != 7 {
		t.Errorf("the fast fields are %+v, want the values of the fast pass", g)
	}
	if g.TempC != 51 {
		t.Errorf("the temperature is %v, want 51 (the slow pass)", g.TempC)
	}

	// The seconds between two slow passes read the fast tier only.
	for i := 1; i < int(slowEvery/time.Second); i++ {
		now = now.Add(time.Second)
		doc = sampleDoc(t, s)
		if fast != i+1 {
			t.Fatalf("after %d s: fast=%d, want %d", i, fast, i+1)
		}
		if slow != 1 {
			t.Fatalf("after %d s: slow=%d, want 1 (the slow pass waits)", i, slow)
		}
		if doc.SlowAgeS != float64(i) {
			t.Errorf("after %d s: the slow age is %v s, want %d", i, doc.SlowAgeS, i)
		}
		// The slow value is the one of the last slow pass, and the fast
		// values are fresh.
		if doc.GPUs[0].TempC != 51 {
			t.Errorf("after %d s: the temperature is %v, want 51 (the last slow pass)", i, doc.GPUs[0].TempC)
		}
		if doc.GPUs[0].BusyPct != 42 {
			t.Errorf("after %d s: the load is %v, want 42 (the fast pass)", i, doc.GPUs[0].BusyPct)
		}
	}

	// The next second opens the slow tier again.
	now = now.Add(time.Second)
	doc = sampleDoc(t, s)
	if slow != 2 {
		t.Fatalf("after the slow pass: slow=%d, want 2", slow)
	}
	if doc.SlowAgeS != 0 {
		t.Errorf("after the slow pass: the slow age is %v s, want 0", doc.SlowAgeS)
	}
	if doc.GPUs[0].TempC != 52 {
		t.Errorf("after the slow pass: the temperature is %v, want 52", doc.GPUs[0].TempC)
	}
}

// sampleDoc runs one sample of the sampler and reads the document back.
func sampleDoc(t *testing.T, s *Sampler) document {
	t.Helper()
	b := s.Sample(context.Background())
	var d document
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("the sample is not JSON: %v", err)
	}
	return d
}

// The history holds the slow fields at their own cadence: a point without a
// slow measurement carries a gap, never a value carried forward.
func TestHistoryHoldsTheSlowGaps(t *testing.T) {
	h := NewHistory()
	// The window is built from the points of the last minutes, so the
	// points of the test start now.
	now := time.Now()
	for i := range 6 {
		at := now.Add(time.Duration(i) * time.Second)
		gpus := []gpu.Sample{{HIPIndex: 0, BusyPct: float64(i), TempC: float64(50 + i/4)}}
		// The slow pass runs every fourth second.
		h.Add(at, gpus, 0, 0, i%4 == 0)
	}
	doc, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Series) != 1 {
		t.Fatalf("the window holds %d series, want 1", len(doc.Series))
	}
	s := doc.Series[0]
	if len(s.BusyPct) != 3 {
		t.Fatalf("the series holds %d points, want 3 (one every two seconds)", len(s.BusyPct))
	}
	if len(s.TempC) != len(s.BusyPct) {
		t.Fatalf("the slow series holds %d points, want %d", len(s.TempC), len(s.BusyPct))
	}
	if s.TempC[0] == nil || *s.TempC[0] != 50 {
		t.Errorf("the first slow point is %v, want 50", s.TempC[0])
	}
	if s.TempC[1] != nil {
		t.Errorf("the second slow point is %v, want a gap", *s.TempC[1])
	}
	if s.TempC[2] == nil {
		t.Error("the third slow point is a gap, want a value")
	}
}

// The sampler reads the server of every run once a second and keeps the
// readings of the last minute: the window of the run holds what the server
// did in between.
func TestScrapeCarriesTheWindow(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{Model: "m1"}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	st := runner.Status{
		Record: runner.Record{Name: "run", Host: u.Hostname(), Port: port},
		State:  runner.StateRunning,
	}
	w := vllm.NewWatcher(srv.URL, vllm.DefaultWindow)
	m := scrape(context.Background(), st, w)
	if m.Stats == nil {
		t.Fatalf("no stats: %v", m.Error)
	}
	if m.Window != nil {
		t.Errorf("the first reading has no window: %+v", m.Window)
	}
	// A request gives the server something to measure.
	if _, err := vllm.New(srv.URL).CompleteStream(context.Background(), "m1", "hello there", 4); err != nil {
		t.Fatal(err)
	}
	m = scrape(context.Background(), st, w)
	if m.Window == nil {
		t.Fatalf("no window: %v", m.Error)
	}
	if m.Window.TTFT == nil {
		t.Error("the window holds no time to the first token")
	}
	if m.Window.PrefixCacheQueries == 0 {
		t.Error("the window holds no prefix cache query")
	}
}
