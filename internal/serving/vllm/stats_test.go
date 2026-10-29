package vllm

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// sample reads a metrics file of the testdata folder.
func sample(t *testing.T, name string) map[string]float64 {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseMetrics(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestSummarizeLiveSample reads the metrics of a running server and checks
// the numbers bernie shows against the raw lines of the file.
func TestSummarizeLiveSample(t *testing.T) {
	s := Summarize(sample(t, "metrics_live.txt"))
	if s.PrefixCacheQueries != 14536 {
		t.Errorf("PrefixCacheQueries = %v, want 14536", s.PrefixCacheQueries)
	}
	if s.PrefixCacheHits != 416 {
		t.Errorf("PrefixCacheHits = %v, want 416", s.PrefixCacheHits)
	}
	want := 416.0 / 14536.0
	if math.Abs(s.PrefixCacheHitRate-want) > 1e-12 {
		t.Errorf("PrefixCacheHitRate = %v, want %v", s.PrefixCacheHitRate, want)
	}
	if s.PromptTokens != 14536 || s.PromptTokensCached != 416 {
		t.Errorf("prompt tokens = %v, cached = %v; want 14536 and 416", s.PromptTokens, s.PromptTokensCached)
	}
	if s.ExternalPrefixCacheQueries != 0 || s.ExternalPrefixCacheHits != 0 {
		t.Errorf("external cache = %v queries, %v hits; want 0 and 0", s.ExternalPrefixCacheQueries, s.ExternalPrefixCacheHits)
	}
	if s.Preemptions != 0 {
		t.Errorf("Preemptions = %v, want 0", s.Preemptions)
	}
	wantSource := map[string]float64{"local_compute": 14120, "local_cache_hit": 416, "external_kv_transfer": 0}
	for k, v := range wantSource {
		if s.PromptTokensBySource[k] != v {
			t.Errorf("PromptTokensBySource[%q] = %v, want %v", k, s.PromptTokensBySource[k], v)
		}
	}
	wantWaiting := map[string]float64{"capacity": 0, "deferred": 0}
	for k, v := range wantWaiting {
		if s.WaitingByReason[k] != v {
			t.Errorf("WaitingByReason[%q] = %v, want %v", k, s.WaitingByReason[k], v)
		}
	}
	wantFinished := map[string]float64{"stop": 1, "length": 54, "abort": 0, "error": 0, "repetition": 0}
	for k, v := range wantFinished {
		if s.FinishedByReason[k] != v {
			t.Errorf("FinishedByReason[%q] = %v, want %v", k, s.FinishedByReason[k], v)
		}
	}
	// The TTFT histogram of the sample: 55 requests, 30.42 s in total.
	// The median lies in the bucket of 0.06 s, the 95th percentile in the
	// bucket of 0.25 s.
	if s.TTFT == nil {
		t.Fatal("no TTFT in the summary")
	}
	if s.TTFT.Count != 55 {
		t.Errorf("TTFT.Count = %v, want 55", s.TTFT.Count)
	}
	if s.TTFT.P50 != 0.06 {
		t.Errorf("TTFT.P50 = %v, want 0.06", s.TTFT.P50)
	}
	if s.TTFT.P95 != 0.25 {
		t.Errorf("TTFT.P95 = %v, want 0.25", s.TTFT.P95)
	}
	if want := 30.421223402023315 / 55; math.Abs(s.TTFT.Mean-want) > 1e-12 {
		t.Errorf("TTFT.Mean = %v, want %v", s.TTFT.Mean, want)
	}
	if s.ITL == nil || s.ITL.Count != 3044 || s.ITL.P50 != 0.025 || s.ITL.P95 != 0.05 {
		t.Errorf("ITL = %+v, want 3044 observations, p50 0.025 and p95 0.05", s.ITL)
	}
	if s.E2E == nil || s.E2E.Count != 55 || s.E2E.P50 != 1.5 || s.E2E.P95 != 5 {
		t.Errorf("E2E = %+v, want 55 observations, p50 1.5 and p95 5", s.E2E)
	}
	if s.Queue == nil || s.Queue.Count != 55 {
		t.Errorf("Queue = %+v, want 55 observations", s.Queue)
	}
}

// TestSummarizeOldSample reads the metrics of an old server: the prefix
// cache counters have the old names and the newer counters are absent.
func TestSummarizeOldSample(t *testing.T) {
	s := Summarize(sample(t, "metrics_old.txt"))
	if s.PrefixCacheQueries != 14536 || s.PrefixCacheHits != 416 {
		t.Errorf("prefix cache = %v queries, %v hits; want 14536 and 416", s.PrefixCacheQueries, s.PrefixCacheHits)
	}
	if s.PromptTokens != 14536 {
		t.Errorf("PromptTokens = %v, want 14536", s.PromptTokens)
	}
	// The old server measures less: a missing counter is a gap, not a zero.
	if s.PromptTokensBySource != nil {
		t.Errorf("PromptTokensBySource = %v, want nil (the old server has no such counter)", s.PromptTokensBySource)
	}
	if s.WaitingByReason != nil {
		t.Errorf("WaitingByReason = %v, want nil (the old server has no such gauge)", s.WaitingByReason)
	}
	if s.PromptTokensCached != 0 || s.ExternalPrefixCacheHits != 0 {
		t.Errorf("cached = %v, external hits = %v; want 0 and 0", s.PromptTokensCached, s.ExternalPrefixCacheHits)
	}
	if s.TTFT == nil || s.TTFT.Count != 55 {
		t.Errorf("TTFT = %+v, want 55 observations", s.TTFT)
	}
}

// TestSummarizeEmptyServer checks that a server that measures nothing
// leaves every measure absent.
func TestSummarizeEmptyServer(t *testing.T) {
	s := Summarize(map[string]float64{})
	if s.TTFT != nil || s.ITL != nil || s.E2E != nil || s.Queue != nil {
		t.Errorf("latencies = %+v, %+v, %+v, %+v; want nil", s.TTFT, s.ITL, s.E2E, s.Queue)
	}
	if s.PrefixCacheHitRate != 0 {
		t.Errorf("PrefixCacheHitRate = %v, want 0 (no query yet)", s.PrefixCacheHitRate)
	}
	if s.PromptTokensBySource != nil || s.FinishedByReason != nil {
		t.Error("a server without counters has no breakdown")
	}
}

// TestWindowBetween checks what the server did between two readings.
func TestWindowBetween(t *testing.T) {
	before := map[string]float64{
		"vllm:prefix_cache_queries_total":                      100,
		"vllm:prefix_cache_hits_total":                         10,
		"vllm:prompt_tokens_total":                             100,
		"vllm:prompt_tokens_cached_total":                      10,
		"vllm:generation_tokens_total":                         50,
		"vllm:request_success_total{finished_reason=\"stop\"}": 4,
		"vllm:time_to_first_token_seconds_bucket{le=\"0.1\"}":  2,
		"vllm:time_to_first_token_seconds_bucket{le=\"1\"}":    4,
		"vllm:time_to_first_token_seconds_count":               4,
		"vllm:time_to_first_token_seconds_sum":                 0.5,
	}
	after := map[string]float64{
		"vllm:prefix_cache_queries_total":                      200,
		"vllm:prefix_cache_hits_total":                         60,
		"vllm:prompt_tokens_total":                             200,
		"vllm:prompt_tokens_cached_total":                      60,
		"vllm:generation_tokens_total":                         90,
		"vllm:request_success_total{finished_reason=\"stop\"}": 7,
		"vllm:time_to_first_token_seconds_bucket{le=\"0.1\"}":  3,
		"vllm:time_to_first_token_seconds_bucket{le=\"1\"}":    9,
		"vllm:time_to_first_token_seconds_count":               9,
		"vllm:time_to_first_token_seconds_sum":                 1.5,
	}
	w := windowBetween(checkpointOf(before), checkpointOf(after), 60)
	if w.Seconds != 60 {
		t.Errorf("Seconds = %v, want 60", w.Seconds)
	}
	if w.PrefixCacheQueries != 100 || w.PrefixCacheHits != 50 {
		t.Errorf("prefix cache = %v queries, %v hits; want 100 and 50", w.PrefixCacheQueries, w.PrefixCacheHits)
	}
	if w.PrefixCacheHitRate == nil || math.Abs(*w.PrefixCacheHitRate-0.5) > 1e-12 {
		t.Errorf("PrefixCacheHitRate = %v, want 0.5", w.PrefixCacheHitRate)
	}
	if w.PromptTokens != 100 || w.PromptTokensCached != 50 {
		t.Errorf("prompt tokens = %v, cached = %v; want 100 and 50", w.PromptTokens, w.PromptTokensCached)
	}
	if w.Finished != 3 {
		t.Errorf("Finished = %v, want 3", w.Finished)
	}
	if w.TTFT == nil || w.TTFT.Count != 5 {
		t.Fatalf("TTFT = %+v, want 5 observations", w.TTFT)
	}
	// The window holds the observations between the two readings: three in
	// the bucket of 0.1 s and two more in the bucket of 1 s.
	if w.TTFT.P50 != 1 {
		t.Errorf("TTFT.P50 = %v, want 1", w.TTFT.P50)
	}
	if w.TTFT.P95 != 1 {
		t.Errorf("TTFT.P95 = %v, want 1", w.TTFT.P95)
	}
}

// TestWindowWithoutQueries checks that a window without a query has no hit
// rate: a rate without a query is not a rate.
func TestWindowWithoutQueries(t *testing.T) {
	before := map[string]float64{"vllm:prefix_cache_queries_total": 100}
	after := map[string]float64{"vllm:prefix_cache_queries_total": 100}
	w := windowBetween(checkpointOf(before), checkpointOf(after), 10)
	if w.PrefixCacheHitRate != nil {
		t.Errorf("PrefixCacheHitRate = %v, want nil", *w.PrefixCacheHitRate)
	}
}

// TestWindowAfterRestart checks that a server that restarted empties the
// window: nothing can be measured across a restart.
func TestWindowAfterRestart(t *testing.T) {
	before := map[string]float64{
		"vllm:prefix_cache_queries_total":        100,
		"vllm:prefix_cache_hits_total":           10,
		"vllm:time_to_first_token_seconds_count": 4,
	}
	after := map[string]float64{
		"vllm:prefix_cache_queries_total":        5,
		"vllm:prefix_cache_hits_total":           0,
		"vllm:time_to_first_token_seconds_count": 1,
	}
	w := windowBetween(checkpointOf(before), checkpointOf(after), 10)
	if w.PrefixCacheQueries != 0 || w.PrefixCacheHits != 0 {
		t.Errorf("prefix cache = %v queries, %v hits; want 0 and 0", w.PrefixCacheQueries, w.PrefixCacheHits)
	}
	if w.PrefixCacheHitRate != nil {
		t.Errorf("PrefixCacheHitRate = %v, want nil", *w.PrefixCacheHitRate)
	}
	if w.TTFT != nil {
		t.Errorf("TTFT = %+v, want nil", w.TTFT)
	}
}

// TestWatcher checks the readings of a watcher: the first reading has no
// window, the second one holds what the server did in between.
func TestWatcher(t *testing.T) {
	first := "# TYPE vllm:prefix_cache_queries_total counter\n" +
		"vllm:prefix_cache_queries_total{engine=\"0\"} 100\n" +
		"vllm:prefix_cache_hits_total{engine=\"0\"} 10\n"
	second := "# TYPE vllm:prefix_cache_queries_total counter\n" +
		"vllm:prefix_cache_queries_total{engine=\"0\"} 300\n" +
		"vllm:prefix_cache_hits_total{engine=\"0\"} 60\n"
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, first)
			return
		}
		io.WriteString(w, second)
	}))
	defer srv.Close()
	w := NewWatcher(srv.URL, DefaultWindow)
	r1, err := w.sample(context.Background(), time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if r1.Stats.PrefixCacheQueries != 100 {
		t.Errorf("queries = %v, want 100", r1.Stats.PrefixCacheQueries)
	}
	if r1.Window.Seconds != 0 || r1.Window.PrefixCacheHitRate != nil {
		t.Errorf("the first reading has no window: %+v", r1.Window)
	}
	r2, err := w.sample(context.Background(), time.Unix(1060, 0))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Window.Seconds != 60 {
		t.Errorf("Seconds = %v, want 60", r2.Window.Seconds)
	}
	if r2.Window.PrefixCacheQueries != 200 || r2.Window.PrefixCacheHits != 50 {
		t.Errorf("prefix cache = %v queries, %v hits; want 200 and 50",
			r2.Window.PrefixCacheQueries, r2.Window.PrefixCacheHits)
	}
	if r2.Window.PrefixCacheHitRate == nil || math.Abs(*r2.Window.PrefixCacheHitRate-0.25) > 1e-12 {
		t.Errorf("PrefixCacheHitRate = %v, want 0.25", r2.Window.PrefixCacheHitRate)
	}
}

// TestWatcherDropsOldReadings checks that the window holds the last minute
// and not every reading of the server.
func TestWatcherDropsOldReadings(t *testing.T) {
	text := "# TYPE vllm:prefix_cache_queries_total counter\n" +
		"vllm:prefix_cache_queries_total{engine=\"0\"} 100\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, text)
	}))
	defer srv.Close()
	w := NewWatcher(srv.URL, 30*time.Second)
	at := time.Unix(1000, 0)
	for range 5 {
		if _, err := w.sample(context.Background(), at); err != nil {
			t.Fatal(err)
		}
		at = at.Add(10 * time.Second)
	}
	if len(w.points) != 4 {
		t.Errorf("the watcher keeps %d readings, want 4 (the last 30 s)", len(w.points))
	}
}

// TestFakeServerMetrics checks that the fake server publishes the series
// bernie reads and that the summary holds them.
func TestFakeServerMetrics(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{Model: "m1"}))
	defer srv.Close()
	c := New(srv.URL)
	ctx := context.Background()
	// The first prompt fills the cache, the second one hits it.
	for _, p := range []string{"the quick brown fox jumps", "the quick brown fox runs"} {
		if _, err := c.CompleteStream(ctx, "m1", p, 4); err != nil {
			t.Fatal(err)
		}
	}
	m, err := c.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := Summarize(m)
	if s.PrefixCacheQueries <= 0 {
		t.Errorf("PrefixCacheQueries = %v, want more than 0", s.PrefixCacheQueries)
	}
	if s.PrefixCacheHits <= 0 {
		t.Errorf("PrefixCacheHits = %v, want more than 0", s.PrefixCacheHits)
	}
	if s.PromptTokensCached != s.PrefixCacheHits {
		t.Errorf("PromptTokensCached = %v, want the same as the hits (%v)", s.PromptTokensCached, s.PrefixCacheHits)
	}
	if s.FinishedByReason["length"] != 2 {
		t.Errorf("FinishedByReason = %v, want 2 requests with the reason length", s.FinishedByReason)
	}
	if s.TTFT == nil || s.TTFT.Count != 2 {
		t.Errorf("TTFT = %+v, want 2 observations", s.TTFT)
	}
	if s.E2E == nil || s.E2E.Count != 2 {
		t.Errorf("E2E = %+v, want 2 observations", s.E2E)
	}
	if s.Queue == nil || s.Queue.Count != 2 {
		t.Errorf("Queue = %+v, want 2 observations", s.Queue)
	}
}
