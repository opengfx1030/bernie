package vllm

// Stats are the vLLM gauges and counters bernie shows, summed over all label
// sets (for example several model_name values).
type Stats struct {
	RequestsRunning  float64 `json:"requests_running"`
	RequestsWaiting  float64 `json:"requests_waiting"`
	KVCacheUsage     float64 `json:"kv_cache_usage"` // fraction 0..1
	PromptTokens     float64 `json:"prompt_tokens_total"`
	GenerationTokens float64 `json:"generation_tokens_total"`
	// Speculative decoding (MTP, EAGLE, ngram). Zero without speculation.
	SpecDrafts         float64 `json:"spec_drafts_total"`
	SpecDraftTokens    float64 `json:"spec_draft_tokens_total"`
	SpecAcceptedTokens float64 `json:"spec_accepted_tokens_total"`
	// SpecAcceptRate is accepted / draft tokens (0..1).
	SpecAcceptRate float64 `json:"spec_accept_rate"`
	// SpecMeanAcceptLen is the mean tokens emitted per draft step
	// (1 + accepted / drafts).
	SpecMeanAcceptLen float64 `json:"spec_mean_accept_len"`
	// The prefix cache: the tokens the server looked up and the tokens it
	// could serve from the cache.
	PrefixCacheQueries float64 `json:"prefix_cache_queries_total"`
	PrefixCacheHits    float64 `json:"prefix_cache_hits_total"`
	// PrefixCacheHitRate is hits / queries since the server started (0..1).
	// It is 0 when the server looked up no token yet.
	PrefixCacheHitRate float64 `json:"prefix_cache_hit_rate"`
	// The cache of another instance (the KV connector).
	ExternalPrefixCacheQueries float64 `json:"external_prefix_cache_queries_total"`
	ExternalPrefixCacheHits    float64 `json:"external_prefix_cache_hits_total"`
	// PromptTokensCached is the prompt tokens the server took from a cache.
	PromptTokensCached float64 `json:"prompt_tokens_cached_total"`
	// PromptTokensBySource counts the prompt tokens per source
	// (local_compute, local_cache_hit, external_kv_transfer).
	PromptTokensBySource map[string]float64 `json:"prompt_tokens_by_source"`
	// Preemptions counts the requests the engine took back and ran again.
	Preemptions float64 `json:"num_preemptions_total"`
	// WaitingByReason counts the waiting requests per reason (capacity,
	// deferred).
	WaitingByReason map[string]float64 `json:"waiting_by_reason"`
	// FinishedByReason counts the finished requests per reason (stop,
	// length, abort, error, repetition).
	FinishedByReason map[string]float64 `json:"finished_by_reason"`
	// The latencies the server measured since it started, in seconds. A
	// latency the server does not measure, or has measured nothing of
	// yet, is absent.
	TTFT  *Latency `json:"ttft"`
	ITL   *Latency `json:"itl"`
	E2E   *Latency `json:"e2e"`
	Queue *Latency `json:"queue"`
}

// Latency is one distribution of the server: the median, the 95th
// percentile, the mean and the number of observations behind them. The unit
// is the unit of the histogram the numbers come from (seconds for the
// latencies of vLLM).
type Latency struct {
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	Mean  float64 `json:"mean"`
	Count float64 `json:"count"`
}

// Window is what the server did between two readings of its metrics: the
// counters it advanced and the latencies it measured in that time.
type Window struct {
	// Seconds is the length of the window.
	Seconds float64 `json:"seconds"`
	// The prefix cache counters of the window.
	PrefixCacheQueries float64 `json:"prefix_cache_queries"`
	PrefixCacheHits    float64 `json:"prefix_cache_hits"`
	// PrefixCacheHitRate is hits / queries in the window. It is absent
	// when the window holds no query: a rate without a query is not a rate.
	PrefixCacheHitRate *float64 `json:"prefix_cache_hit_rate"`
	// The prompt tokens the server read and the ones it took from a cache.
	PromptTokens       float64 `json:"prompt_tokens"`
	PromptTokensCached float64 `json:"prompt_tokens_cached"`
	// GenerationTokens counts the tokens the server wrote in the window.
	GenerationTokens float64 `json:"generation_tokens"`
	// Finished counts the requests the server finished in the window.
	Finished float64 `json:"finished"`
	// The latencies the server measured in the window, in seconds.
	TTFT  *Latency `json:"ttft"`
	ITL   *Latency `json:"itl"`
	E2E   *Latency `json:"e2e"`
	Queue *Latency `json:"queue"`
}

// Summarize extracts Stats from flattened samples (see ParseMetrics).
func Summarize(m map[string]float64) Stats {
	s := Stats{
		RequestsRunning:            Sum(m, MetricRequestsRunning),
		RequestsWaiting:            Sum(m, MetricRequestsWaiting),
		PromptTokens:               Sum(m, MetricPromptTokens),
		GenerationTokens:           Sum(m, MetricGenerationTokens),
		SpecDrafts:                 Sum(m, MetricSpecDrafts),
		SpecDraftTokens:            Sum(m, MetricSpecDraftTokens),
		SpecAcceptedTokens:         Sum(m, MetricSpecAcceptedToken),
		PrefixCacheQueries:         sumAny(m, MetricPrefixCacheQueries, MetricPrefixCacheQueriesOld),
		PrefixCacheHits:            sumAny(m, MetricPrefixCacheHits, MetricPrefixCacheHitsOld),
		ExternalPrefixCacheQueries: Sum(m, MetricExternalPrefixCacheQueries),
		ExternalPrefixCacheHits:    Sum(m, MetricExternalPrefixCacheHits),
		PromptTokensCached:         Sum(m, MetricPromptTokensCached),
		Preemptions:                Sum(m, MetricNumPreemptions),
		PromptTokensBySource:       byLabel(m, MetricPromptTokensBySource, "source"),
		WaitingByReason:            byLabel(m, MetricWaitingByReason, "reason"),
		FinishedByReason:           byLabel(m, MetricRequestSuccess, "finished_reason"),
	}
	// The KV cache gauge is a fraction per engine; report the highest one.
	kv, ok := Max(m, MetricKVCacheUsage)
	if !ok {
		kv, _ = Max(m, MetricKVCacheUsageOld)
	}
	s.KVCacheUsage = kv
	if s.SpecDraftTokens > 0 {
		s.SpecAcceptRate = s.SpecAcceptedTokens / s.SpecDraftTokens
	}
	if s.SpecDrafts > 0 {
		s.SpecMeanAcceptLen = 1 + s.SpecAcceptedTokens/s.SpecDrafts
	}
	if s.PrefixCacheQueries > 0 {
		s.PrefixCacheHitRate = s.PrefixCacheHits / s.PrefixCacheQueries
	}
	hs := Histograms(m, MetricTimeToFirstToken, MetricInterTokenLatency, MetricE2ELatency, MetricQueueTime)
	s.TTFT = latencyOf(hs, MetricTimeToFirstToken)
	s.ITL = latencyOf(hs, MetricInterTokenLatency)
	s.E2E = latencyOf(hs, MetricE2ELatency)
	s.Queue = latencyOf(hs, MetricQueueTime)
	return s
}

// sumAny adds the samples of the first name of names that the server
// exposes. A server that publishes two names of the same counter (an old
// and a new release) is counted once.
func sumAny(m map[string]float64, names ...string) float64 {
	for _, n := range names {
		if _, ok := Max(m, n); ok {
			return Sum(m, n)
		}
	}
	return 0
}

// byLabel sums the samples of a metric per the value of one label (the
// `reason` of a waiting request, the `source` of a prompt token). The
// result is nil when the server does not expose the metric: a breakdown
// without a server is a gap, not an empty table.
func byLabel(m map[string]float64, name, label string) map[string]float64 {
	out := map[string]float64{}
	found := false
	for key, v := range m {
		base, labels := splitMetric(key)
		if base != name {
			continue
		}
		found = true
		value, ok := labelValue(labels, label)
		if !ok {
			continue
		}
		out[value] += v
	}
	if !found {
		return nil
	}
	return out
}

// latencyOf returns the distribution of one latency histogram of the
// server. It is nil when the server does not measure that latency or has
// measured nothing of it yet.
func latencyOf(hs map[string]Histogram, name string) *Latency {
	h, ok := hs[name]
	if !ok || h.Count <= 0 {
		return nil
	}
	return &Latency{P50: h.Quantile(0.5), P95: h.Quantile(0.95), Mean: h.Mean(), Count: h.Count}
}

// counters are the counters of the server that a window compares.
type counters struct {
	prefixCacheQueries         float64
	prefixCacheHits            float64
	externalPrefixCacheQueries float64
	externalPrefixCacheHits    float64
	promptTokens               float64
	promptTokensCached         float64
	generationTokens           float64
	finished                   float64
}

// window returns the increase of the counters between two readings. It
// reports false when a counter went down: the server restarted, and nothing
// can be measured across a restart.
func (c counters) window(after counters) (counters, bool) {
	d := counters{
		prefixCacheQueries:         after.prefixCacheQueries - c.prefixCacheQueries,
		prefixCacheHits:            after.prefixCacheHits - c.prefixCacheHits,
		externalPrefixCacheQueries: after.externalPrefixCacheQueries - c.externalPrefixCacheQueries,
		externalPrefixCacheHits:    after.externalPrefixCacheHits - c.externalPrefixCacheHits,
		promptTokens:               after.promptTokens - c.promptTokens,
		promptTokensCached:         after.promptTokensCached - c.promptTokensCached,
		generationTokens:           after.generationTokens - c.generationTokens,
		finished:                   after.finished - c.finished,
	}
	if d.prefixCacheQueries < 0 || d.prefixCacheHits < 0 || d.externalPrefixCacheQueries < 0 ||
		d.externalPrefixCacheHits < 0 || d.promptTokens < 0 || d.promptTokensCached < 0 ||
		d.generationTokens < 0 || d.finished < 0 {
		return counters{}, false
	}
	return d, true
}

// checkpoint is what one reading of the metrics keeps for the window: the
// counters of the server and the latency histograms it measured.
type checkpoint struct {
	counters   counters
	histograms map[string]Histogram
}

// checkpointOf returns what a reading of the metrics keeps for the window.
func checkpointOf(m map[string]float64) checkpoint {
	s := Summarize(m)
	finished := 0.0
	for _, n := range s.FinishedByReason {
		finished += n
	}
	return checkpoint{
		counters: counters{
			prefixCacheQueries:         s.PrefixCacheQueries,
			prefixCacheHits:            s.PrefixCacheHits,
			externalPrefixCacheQueries: s.ExternalPrefixCacheQueries,
			externalPrefixCacheHits:    s.ExternalPrefixCacheHits,
			promptTokens:               s.PromptTokens,
			promptTokensCached:         s.PromptTokensCached,
			generationTokens:           s.GenerationTokens,
			finished:                   finished,
		},
		histograms: Histograms(m, MetricTimeToFirstToken, MetricInterTokenLatency, MetricE2ELatency, MetricQueueTime),
	}
}

// windowBetween returns what the server did between two readings. A server
// that restarted empties the window: nothing can be measured across a
// restart.
func windowBetween(before, after checkpoint, seconds float64) Window {
	w := Window{Seconds: seconds}
	d, ok := before.counters.window(after.counters)
	if !ok {
		return w
	}
	w.PrefixCacheQueries = d.prefixCacheQueries
	w.PrefixCacheHits = d.prefixCacheHits
	w.PromptTokens = d.promptTokens
	w.PromptTokensCached = d.promptTokensCached
	w.GenerationTokens = d.generationTokens
	w.Finished = d.finished
	if d.prefixCacheQueries > 0 {
		rate := d.prefixCacheHits / d.prefixCacheQueries
		w.PrefixCacheHitRate = &rate
	}
	w.TTFT = latencyDelta(before.histograms, after.histograms, MetricTimeToFirstToken)
	w.ITL = latencyDelta(before.histograms, after.histograms, MetricInterTokenLatency)
	w.E2E = latencyDelta(before.histograms, after.histograms, MetricE2ELatency)
	w.Queue = latencyDelta(before.histograms, after.histograms, MetricQueueTime)
	return w
}

// latencyDelta returns the distribution the server measured between two
// readings of one latency histogram. It is nil when the server measured
// nothing in that time or when it restarted.
func latencyDelta(before, after map[string]Histogram, name string) *Latency {
	b, ok := before[name]
	if !ok {
		return nil
	}
	a, ok := after[name]
	if !ok {
		return nil
	}
	d, ok := b.Delta(a)
	if !ok || d.Count <= 0 {
		return nil
	}
	return &Latency{P50: d.Quantile(0.5), P95: d.Quantile(0.95), Mean: d.Mean(), Count: d.Count}
}
