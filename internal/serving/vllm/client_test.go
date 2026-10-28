package vllm_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

func TestParseMetrics(t *testing.T) {
	text := `# HELP vllm:num_requests_running Number of requests.
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{engine="0",model_name="qwen 27b"} 3.0

vllm:generation_tokens_total{model_name="a}b \"c\""} 1.5e3 1730000000000
process_open_fds 12
weird_nan NaN
weird_inf +Inf
`
	got, err := vllm.ParseMetrics([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		`vllm:num_requests_running{engine="0",model_name="qwen 27b"}`: 3,
		`vllm:generation_tokens_total{model_name="a}b \"c\""}`:        1500,
		`process_open_fds`: 12,
		`weird_inf`:        math.Inf(1),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if v, ok := got["weird_nan"]; !ok || !math.IsNaN(v) {
		t.Errorf("weird_nan = %v, %v; want NaN", v, ok)
	}
	if len(got) != 5 {
		t.Errorf("got %d samples, want 5: %v", len(got), got)
	}
}

func TestParseMetricsErrors(t *testing.T) {
	for name, text := range map[string]string{
		"unterminated labels": `m{a="x} 1`,
		"missing value":       `m{a="x"}`,
		"bare name":           `lonely`,
		"bad value":           `m 1,5`,
	} {
		if _, err := vllm.ParseMetrics([]byte(text)); err == nil {
			t.Errorf("%s: no error for %q", name, text)
		}
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]float64
		want vllm.Stats
	}{
		{
			name: "sums label sets, new kv gauge, spec decode",
			m: map[string]float64{
				`vllm:num_requests_running{model_name="a"}`:            2,
				`vllm:num_requests_running{model_name="b"}`:            1,
				`vllm:num_requests_waiting{model_name="a"}`:            4,
				`vllm:kv_cache_usage_perc{engine="0"}`:                 0.2,
				`vllm:kv_cache_usage_perc{engine="1"}`:                 0.6,
				`vllm:gpu_cache_usage_perc`:                            0.9, // ignored: new name present
				`vllm:prompt_tokens_total`:                             1000,
				`vllm:generation_tokens_total`:                         500,
				`vllm:spec_decode_num_drafts_total`:                    100,
				`vllm:spec_decode_num_draft_tokens_total`:              200,
				`vllm:spec_decode_num_accepted_tokens_total`:           150,
				`vllm:spec_decode_num_accepted_tokens_per_pos_total{}`: 9, // different metric
			},
			want: vllm.Stats{RequestsRunning: 3, RequestsWaiting: 4, KVCacheUsage: 0.6,
				PromptTokens: 1000, GenerationTokens: 500, SpecDrafts: 100, SpecDraftTokens: 200,
				SpecAcceptedTokens: 150, SpecAcceptRate: 0.75, SpecMeanAcceptLen: 2.5},
		},
		{
			name: "old kv gauge, no speculation",
			m:    map[string]float64{`vllm:gpu_cache_usage_perc{model_name="a"}`: 0.4},
			want: vllm.Stats{KVCacheUsage: 0.4},
		},
		{name: "empty", m: map[string]float64{}, want: vllm.Stats{}},
	}
	for _, tt := range tests {
		if got := vllm.Summarize(tt.m); got != tt.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tt.name, got, tt.want)
		}
	}
}

func TestClientAgainstFake(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{Model: "m1"}))
	defer srv.Close()
	ctx := context.Background()
	c := vllm.New(srv.URL + "/v1/")
	if c.BaseURL() != srv.URL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), srv.URL)
	}
	if err := c.Health(ctx); err != nil {
		t.Errorf("Health: %v", err)
	}
	ids, err := c.Models(ctx)
	if err != nil || len(ids) != 1 || ids[0] != "m1" {
		t.Errorf("Models = %v, %v", ids, err)
	}
	n, err := c.Tokenize(ctx, "m1", "one two extraordinary")
	if err != nil || n != vllmtest.CountTokens("one two extraordinary") || n != 5 {
		t.Errorf("Tokenize = %d, %v; want 5", n, err)
	}
	m, err := c.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := vllm.Summarize(m); s.KVCacheUsage != 0.25 {
		t.Errorf("KVCacheUsage = %v, want 0.25", s.KVCacheUsage)
	}
}

func TestTokenizeUnsupported(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{NoTokenize: true}))
	defer srv.Close()
	_, err := vllm.New(srv.URL).Tokenize(context.Background(), "fake-model", "x")
	if !errors.Is(err, vllm.ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestStatusErrorAndTokensFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			w.Write([]byte(`{"tokens":[1,2,3,4]}`))
		default:
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := vllm.New(srv.URL)
	if n, err := c.Tokenize(context.Background(), "m", "x"); err != nil || n != 4 {
		t.Errorf("Tokenize = %d, %v; want 4 from tokens", n, err)
	}
	var se *vllm.StatusError
	if err := c.Health(context.Background()); !errors.As(err, &se) || se.Code != 500 {
		t.Errorf("Health err = %v, want StatusError 500", err)
	}
}

func TestContextBoundsCall(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := vllm.New(srv.URL).Health(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("call took %v despite 50ms deadline", d)
	}
}
