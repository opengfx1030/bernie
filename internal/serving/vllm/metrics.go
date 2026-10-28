package vllm

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// vLLM metric names. Older releases expose the KV cache gauge as
// gpu_cache_usage_perc; newer ones as kv_cache_usage_perc.
const (
	MetricRequestsRunning   = "vllm:num_requests_running"
	MetricRequestsWaiting   = "vllm:num_requests_waiting"
	MetricKVCacheUsage      = "vllm:kv_cache_usage_perc"
	MetricKVCacheUsageOld   = "vllm:gpu_cache_usage_perc"
	MetricPromptTokens      = "vllm:prompt_tokens_total"
	MetricGenerationTokens  = "vllm:generation_tokens_total"
	MetricSpecDrafts        = "vllm:spec_decode_num_drafts_total"
	MetricSpecDraftTokens   = "vllm:spec_decode_num_draft_tokens_total"
	MetricSpecAcceptedToken = "vllm:spec_decode_num_accepted_tokens_total"
)

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
}

// Summarize extracts Stats from flattened samples (see ParseMetrics).
func Summarize(m map[string]float64) Stats {
	s := Stats{
		RequestsRunning:    Sum(m, MetricRequestsRunning),
		RequestsWaiting:    Sum(m, MetricRequestsWaiting),
		PromptTokens:       Sum(m, MetricPromptTokens),
		GenerationTokens:   Sum(m, MetricGenerationTokens),
		SpecDrafts:         Sum(m, MetricSpecDrafts),
		SpecDraftTokens:    Sum(m, MetricSpecDraftTokens),
		SpecAcceptedTokens: Sum(m, MetricSpecAcceptedToken),
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
	return s
}

// Sum adds the samples of metric name over all its label sets.
func Sum(m map[string]float64, name string) float64 {
	var t float64
	for k, v := range m {
		if metricName(k) == name {
			t += v
		}
	}
	return t
}

// Max returns the largest sample of metric name and whether one exists.
func Max(m map[string]float64, name string) (float64, bool) {
	var best float64
	found := false
	for k, v := range m {
		if metricName(k) == name && (!found || v > best) {
			best, found = v, true
		}
	}
	return best, found
}

func metricName(key string) string {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		return key[:i]
	}
	return key
}

// ParseMetrics parses the Prometheus text exposition format. Comment and
// blank lines are skipped. Keys are `name{labels}` with the label block kept
// verbatim, or `name` alone. Timestamps are ignored.
func ParseMetrics(data []byte) (map[string]float64, error) {
	out := make(map[string]float64)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		key, rest, err := splitSample(line)
		if err != nil {
			return nil, fmt.Errorf("metrics line %d: %w", ln, err)
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return nil, fmt.Errorf("metrics line %d: missing value", ln)
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return nil, fmt.Errorf("metrics line %d: bad value %q", ln, fields[0])
		}
		out[key] = v
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// splitSample splits `name{labels} value ts` into key and the remainder.
// Label values may contain spaces, braces and escaped quotes.
func splitSample(line string) (key, rest string, err error) {
	i := strings.IndexAny(line, "{ \t")
	if i < 0 {
		return "", "", fmt.Errorf("missing value")
	}
	if line[i] != '{' {
		return line[:i], line[i:], nil
	}
	inQuote, esc := false, false
	for j := i + 1; j < len(line); j++ {
		ch := line[j]
		switch {
		case esc:
			esc = false
		case inQuote && ch == '\\':
			esc = true
		case ch == '"':
			inQuote = !inQuote
		case !inQuote && ch == '}':
			return line[:j+1], line[j+1:], nil
		}
	}
	return "", "", fmt.Errorf("unterminated label set")
}
