// Package bench benchmarks an OpenAI-compatible server (spec §10): streaming
// /v1/completions requests with calibrated prompt lengths, one cell per
// concurrency level, and a result store under <state>/bench.
package bench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opengfx1030/rdnactl/internal/paths"
	"github.com/opengfx1030/rdnactl/internal/vllm"
)

// Defaults of spec §10.
const (
	DefaultInputLen  = 1024
	DefaultOutputLen = 512
	DefaultWarmup    = 1
	FallbackURL      = "http://127.0.0.1:8000"
)

// Options are the bench flags. Zero values take the spec defaults in Run,
// except Warmup, which is used as given (the CLI default is DefaultWarmup).
type Options struct {
	URL         string `json:"url"`
	Model       string `json:"model"`
	Tag         string `json:"tag,omitempty"`
	InputLen    int    `json:"input_len"`
	OutputLen   int    `json:"output_len"`
	Warmup      int    `json:"warmup"`
	Concurrency []int  `json:"concurrency"`
	// NumPrompts is the measured requests per cell; 0 means
	// max(4, 2 × concurrency) for each cell.
	NumPrompts int   `json:"num_prompts"`
	Seed       int64 `json:"seed"`
}

// Cell is the result for one concurrency level. Latencies are milliseconds.
type Cell struct {
	Concurrency int     `json:"concurrency"`
	NumPrompts  int     `json:"num_prompts"`
	Completed   int     `json:"completed"`
	Failed      int     `json:"failed"`
	TTFTP50     float64 `json:"ttft_p50_ms"`
	TTFTP95     float64 `json:"ttft_p95_ms"`
	ITLP50      float64 `json:"itl_p50_ms"`
	ITLP95      float64 `json:"itl_p95_ms"`
	// DecodeTokS is the mean per-request decode rate.
	DecodeTokS float64 `json:"decode_tok_s"`
	// PrefillTokS is the mean per-request prompt tokens / TTFT.
	PrefillTokS float64 `json:"prefill_tok_s"`
	// OutputTokS is all completion tokens of the cell / cell wall time.
	OutputTokS float64 `json:"output_tok_s"`
	DurationS  float64 `json:"duration_s"`
	// MeanPromptTokens and MeanOutputTokens come from the server's usage;
	// they show how close the prompt calibration landed.
	MeanPromptTokens float64 `json:"mean_prompt_tokens"`
	MeanOutputTokens float64 `json:"mean_output_tokens"`
	// TokensPerChunk is completion tokens per streamed chunk; above 1 with
	// speculative decoding (MTP).
	TokensPerChunk float64  `json:"tokens_per_chunk"`
	Errors         []string `json:"errors,omitempty"` // distinct error samples
}

// Result is one saved benchmark.
type Result struct {
	ID          string            `json:"id"`
	Tag         string            `json:"tag,omitempty"`
	URL         string            `json:"url"`
	Model       string            `json:"model"`
	Time        time.Time         `json:"time"`
	Options     Options           `json:"options"`
	Fingerprint map[string]string `json:"fingerprint"`
	// PromptWords is the corpus word count per prompt; Calibrated tells
	// whether /tokenize set it (else it is an estimate).
	PromptWords int    `json:"prompt_words"`
	Calibrated  bool   `json:"calibrated"`
	Cells       []Cell `json:"cells"`
}

// Completed is the number of successful measured requests over all cells.
func (r *Result) Completed() int {
	n := 0
	for _, c := range r.Cells {
		n += c.Completed
	}
	return n
}

// Normalize fills the defaults that do not need the server: input and output
// length, concurrency [1], negative warmup to 0. It rejects invalid values.
func Normalize(o Options) (Options, error) {
	if o.InputLen == 0 {
		o.InputLen = DefaultInputLen
	}
	if o.OutputLen == 0 {
		o.OutputLen = DefaultOutputLen
	}
	if len(o.Concurrency) == 0 {
		o.Concurrency = []int{1}
	}
	if o.Warmup < 0 {
		o.Warmup = 0
	}
	switch {
	case o.InputLen < 1:
		return o, fmt.Errorf("input length must be positive, got %d", o.InputLen)
	case o.OutputLen < 1:
		return o, fmt.Errorf("output length must be positive, got %d", o.OutputLen)
	case o.NumPrompts < 0:
		return o, fmt.Errorf("num prompts must not be negative, got %d", o.NumPrompts)
	}
	for _, c := range o.Concurrency {
		if c < 1 {
			return o, fmt.Errorf("concurrency must be positive, got %d", c)
		}
	}
	return o, nil
}

// prompts per cell when Options.NumPrompts is 0.
func cellPrompts(o Options, concurrency int) int {
	if o.NumPrompts > 0 {
		return o.NumPrompts
	}
	return max(4, 2*concurrency)
}

// Run benchmarks the server. Empty URL resolves with DefaultURL, empty Model
// to the first id of /v1/models. Progress lines go to progress (may be nil).
// Run returns an error only when the benchmark cannot start or ctx ends;
// failed requests are counted in the cells.
func Run(ctx context.Context, o Options, progress io.Writer) (*Result, error) {
	if progress == nil {
		progress = io.Discard
	}
	o, err := Normalize(o)
	if err != nil {
		return nil, err
	}
	if o.URL == "" {
		o.URL = DefaultURL()
	}
	client := vllm.New(o.URL)
	o.URL = client.BaseURL()
	if err := client.Health(ctx); err != nil && !errors.Is(err, vllm.ErrUnsupported) {
		return nil, fmt.Errorf("server %s not ready: %w", o.URL, err)
	}
	if o.Model == "" {
		ids, err := client.Models(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("server %s serves no models", o.URL)
		}
		o.Model = ids[0]
	}

	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	p := prompts{seed: o.Seed, nonce: binary.BigEndian.Uint32(nonce[:])}
	words, calibrated, err := calibrateCached(ctx, client, o.Model, p, o.InputLen)
	if err != nil {
		return nil, err
	}
	how := "calibrated with /tokenize"
	if !calibrated {
		how = "estimated, server has no /tokenize"
	}
	fmt.Fprintf(progress, "bench: %s model %s, prompt %d words for %d tokens (%s)\n", o.URL, o.Model, words, o.InputLen, how)

	now := time.Now().UTC()
	res := &Result{
		ID:          newID(now, o, p.nonce),
		Tag:         o.Tag,
		URL:         o.URL,
		Model:       o.Model,
		Time:        now,
		Options:     o,
		Fingerprint: fingerprint(paths.State(), o.URL),
		PromptWords: words,
		Calibrated:  calibrated,
	}

	maxC := 1
	for _, c := range o.Concurrency {
		maxC = max(maxC, c, o.Warmup)
	}
	hc := &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        maxC,
		MaxIdleConnsPerHost: maxC,
		IdleConnTimeout:     90 * time.Second,
	}}
	defer hc.CloseIdleConnections()

	var seq atomic.Int64
	send := func(ctx context.Context) sample {
		prompt := p.build(int(seq.Add(1)), words)
		return streamCompletion(ctx, hc, o.URL, o.Model, prompt, o.OutputLen)
	}

	for _, c := range o.Concurrency {
		n := cellPrompts(o, c)
		if o.Warmup > 0 {
			fmt.Fprintf(progress, "bench: c=%d warmup %d request(s)\n", c, o.Warmup)
			warm := runPool(ctx, o.Warmup, o.Warmup, send)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for _, s := range warm {
				if s.err != nil {
					fmt.Fprintf(progress, "bench: c=%d warmup failed: %v\n", c, s.err)
					break
				}
			}
		}
		fmt.Fprintf(progress, "bench: c=%d running %d requests\n", c, n)
		start := time.Now()
		samples := runPool(ctx, c, n, send)
		wall := time.Since(start)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cell := aggregate(c, samples, wall)
		fmt.Fprintf(progress, "bench: c=%d done in %.1fs: %d ok, %d failed, %.1f output tok/s\n",
			c, cell.DurationS, cell.Completed, cell.Failed, cell.OutputTokS)
		res.Cells = append(res.Cells, cell)
	}
	return res, nil
}

// runPool sends n requests with at most workers in flight and returns the
// samples in request order.
func runPool(ctx context.Context, workers, n int, send func(context.Context) sample) []sample {
	out := make([]sample, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = send(ctx)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

// newID is the UTC timestamp plus a short hash, e.g. 20261028T153012Z-1a2b3c.
func newID(t time.Time, o Options, nonce uint32) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%d", t.UnixNano(), o.URL, o.Model, nonce)
	return t.Format("20060102T150405Z") + "-" + hex.EncodeToString(h.Sum(nil))[:6]
}
