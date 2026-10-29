package vllm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestLiveServer checks the measures of bernie against a running vLLM
// server. It runs only when BERNIE_LIVE_VLLM names the server (for example
// "http://127.0.0.1:18096"): the test prints the raw lines of /metrics it
// reads next to the numbers bernie makes of them.
func TestLiveServer(t *testing.T) {
	url := os.Getenv("BERNIE_LIVE_VLLM")
	if url == "" {
		t.Skip("no live server: set BERNIE_LIVE_VLLM")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 0, 1<<20)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil {
			break
		}
	}
	m, err := ParseMetrics(body)
	if err != nil {
		t.Fatal(err)
	}
	s := Summarize(m)
	lines := strings.Split(string(body), "\n")
	for _, name := range []string{
		"vllm:prefix_cache_queries_total", "vllm:prefix_cache_hits_total",
		"vllm:prompt_tokens_total", "vllm:prompt_tokens_cached_total",
	} {
		for _, line := range lines {
			if strings.HasPrefix(line, name+"{") || strings.HasPrefix(line, name+" ") {
				t.Logf("raw   %s", line)
			}
		}
	}
	t.Logf("stats queries %v hits %v rate %v cached %v prompt %v",
		s.PrefixCacheQueries, s.PrefixCacheHits, s.PrefixCacheHitRate, s.PromptTokensCached, s.PromptTokens)
	if s.TTFT != nil {
		t.Logf("stats ttft p50 %v p95 %v mean %v count %v", s.TTFT.P50, s.TTFT.P95, s.TTFT.Mean, s.TTFT.Count)
	} else {
		t.Logf("stats ttft none")
	}
	// The percentiles are the bounds of the buckets: the p50 lies in the
	// bucket that holds the middle of the observations.
	if s.TTFT == nil {
		return
	}
	bucket := regexp.MustCompile(`vllm:time_to_first_token_seconds_bucket\{[^}]*le="([^"]+)"\} (\S+)`)
	var cum float64
	for _, line := range lines {
		m := bucket.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		t.Logf("raw   %s", line)
		var n float64
		fmt.Sscanf(m[2], "%g", &n)
		cum = n
		if cum >= 0.5*s.TTFT.Count {
			t.Logf("stats ttft p50 is the bucket of %v s", m[1])
			break
		}
	}
}
