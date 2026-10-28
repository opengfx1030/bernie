package bench

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

const ms = time.Millisecond

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func TestMeasure(t *testing.T) {
	tests := []struct {
		name        string
		s           sample
		ttft        time.Duration
		itl         []float64
		decode      float64
		prefill     float64
		completion  int
		chunkCounts int
	}{
		{
			name: "one token per chunk, continuous usage",
			s: sample{
				chunks:       []chunk{{100 * ms, 1}, {110 * ms, 2}, {130 * ms, 3}, {140 * ms, 4}},
				promptTokens: 50, completionTokens: 4,
			},
			ttft: 100 * ms, itl: []float64{10, 20, 10},
			decode: 3 / 0.040, prefill: 500, completion: 4, chunkCounts: 4,
		},
		{
			name: "MTP chunks: per-token ITL weighted by chunk tokens",
			s: sample{
				chunks:       []chunk{{200 * ms, 1}, {220 * ms, 3}, {250 * ms, 6}},
				promptTokens: 100, completionTokens: 6,
			},
			ttft: 200 * ms, itl: []float64{10, 10, 10, 10, 10},
			decode: 5 / 0.050, prefill: 500, completion: 6, chunkCounts: 3,
		},
		{
			name: "first chunk with several tokens is excluded from decode",
			s: sample{
				chunks:       []chunk{{100 * ms, 2}, {120 * ms, 4}},
				promptTokens: 10, completionTokens: 4,
			},
			ttft: 100 * ms, itl: []float64{10, 10},
			decode: 2 / 0.020, prefill: 100, completion: 4, chunkCounts: 2,
		},
		{
			name: "usage only at the end: tokens spread evenly over chunks",
			s: sample{
				chunks:       []chunk{{100 * ms, -1}, {120 * ms, -1}, {160 * ms, -1}},
				promptTokens: 20, completionTokens: 6,
			},
			ttft: 100 * ms, itl: []float64{10, 20},
			decode: 5 / 0.060, prefill: 200, completion: 6, chunkCounts: 3,
		},
		{
			name: "no usage at all: one token per chunk, no prefill rate",
			s:    sample{chunks: []chunk{{50 * ms, -1}, {60 * ms, -1}}},
			ttft: 50 * ms, itl: []float64{10},
			decode: 1 / 0.010, prefill: 0, completion: 2, chunkCounts: 2,
		},
		{
			name: "single chunk: no decode rate, no ITL",
			s:    sample{chunks: []chunk{{80 * ms, 1}}, promptTokens: 8, completionTokens: 1},
			ttft: 80 * ms, itl: nil, decode: 0, prefill: 100, completion: 1, chunkCounts: 1,
		},
	}
	for _, tt := range tests {
		m := measure(tt.s)
		if m.ttft != tt.ttft {
			t.Errorf("%s: ttft %v, want %v", tt.name, m.ttft, tt.ttft)
		}
		if len(m.itlMS) != len(tt.itl) {
			t.Errorf("%s: itl %v, want %v", tt.name, m.itlMS, tt.itl)
		} else {
			for i := range tt.itl {
				if !near(m.itlMS[i], tt.itl[i]) {
					t.Errorf("%s: itl %v, want %v", tt.name, m.itlMS, tt.itl)
					break
				}
			}
		}
		if !near(m.decodeTokS, tt.decode) {
			t.Errorf("%s: decode %v, want %v", tt.name, m.decodeTokS, tt.decode)
		}
		if !near(m.prefillTokS, tt.prefill) {
			t.Errorf("%s: prefill %v, want %v", tt.name, m.prefillTokS, tt.prefill)
		}
		if m.completion != tt.completion || m.chunks != tt.chunkCounts {
			t.Errorf("%s: completion %d chunks %d, want %d %d", tt.name, m.completion, m.chunks, tt.completion, tt.chunkCounts)
		}
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		xs   []float64
		want Dist
	}{
		{nil, Dist{}},
		{[]float64{7}, Dist{Mean: 7, P50: 7, P95: 7, Min: 7, Max: 7}},
		{[]float64{4, 1, 3, 2}, Dist{Mean: 2.5, P50: 2.5, P95: 3.85, Min: 1, Max: 4}},
		{[]float64{1, 2, 3, 4, 5}, Dist{Mean: 3, P50: 3, P95: 4.8, Min: 1, Max: 5}},
		{[]float64{10, 20}, Dist{Mean: 15, P50: 15, P95: 19.5, Min: 10, Max: 20}},
	}
	for _, tt := range tests {
		if got := Summarize(tt.xs); !sameDist(&got, tt.want) {
			t.Errorf("Summarize(%v) = %+v, want %+v", tt.xs, got, tt.want)
		}
	}
	// The helper does not change the order of the numbers it summarizes.
	xs := []float64{3, 1, 2}
	Summarize(xs)
	if xs[0] != 3 {
		t.Errorf("Summarize sorted the slice in place: %v", xs)
	}
}

func TestAggregate(t *testing.T) {
	ok := func(ttft time.Duration) sample {
		return sample{chunks: []chunk{{ttft, 1}, {ttft + 10*ms, 2}}, promptTokens: 100, completionTokens: 2}
	}
	samples := []sample{
		ok(100 * ms), ok(200 * ms), ok(300 * ms),
		{err: errors.New("HTTP 500: boom")},
		{err: errors.New("HTTP 500: boom")},
		{err: errors.New("no output tokens streamed")},
	}
	c := aggregate(4, samples, 2*time.Second)
	if c.Completed != 3 || c.Failed != 3 || c.NumPrompts != 6 {
		t.Errorf("counts: %+v", c)
	}
	if !near(c.TTFTP95, 290) || !near(c.ITLP50, 10) || !near(c.DecodeTokS, 100) || !near(c.OutputTokS, 3) {
		t.Errorf("flat fields: %+v", c)
	}
	if !near(c.TTFTP50, 200) || !near(c.ITLP95, 10) || !near(c.PrefillTokS, (1000+500+1000.0/3)/3) {
		t.Errorf("flat fields: %+v", c)
	}
	// The groups hold the same numbers in seconds and milliseconds.
	if got, want := c.TTFTs, (Dist{Mean: 0.2, P50: 0.2, P95: 0.29, Min: 0.1, Max: 0.3}); !sameDist(got, want) {
		t.Errorf("ttft_s = %+v, want %+v", got, want)
	}
	if got, want := c.ITLms, (Dist{Mean: 10, P50: 10, P95: 10, Min: 10, Max: 10}); !sameDist(got, want) {
		t.Errorf("itl_ms = %+v, want %+v", got, want)
	}
	// Prefill: the window runs from the first request to the last first
	// token (0.3 s) and holds 300 prompt tokens.
	if c.Prefill == nil || !near(c.Prefill.WindowS, 0.3) || !near(c.Prefill.AggTokS, 1000) {
		t.Errorf("prefill = %+v", c.Prefill)
	}
	if c.Prefill.Req == nil || !near(c.Prefill.Req.Mean, (1000+500+1000.0/3)/3) {
		t.Errorf("prefill per request = %+v", c.Prefill.Req)
	}
	// Decode: the three requests decode 0.01 s each, so the union of the
	// decode intervals is 0.03 s for 3 tokens.
	if c.Decode == nil || !near(c.Decode.WindowS, 0.03) || !near(c.Decode.AggTokS, 100) {
		t.Errorf("decode = %+v", c.Decode)
	}
	if c.Decode.Req == nil || !near(c.Decode.Req.Mean, 100) {
		t.Errorf("decode per request = %+v", c.Decode.Req)
	}
	if !c.TokensExact {
		t.Errorf("tokens_exact = false, want true: every request had usage")
	}
	want := []string{"HTTP 500: boom", "no output tokens streamed"}
	if !reflect.DeepEqual(c.Errors, want) {
		t.Errorf("errors = %v, want %v", c.Errors, want)
	}
}

func TestAggregateCapsErrorSamples(t *testing.T) {
	var samples []sample
	for i := range 20 {
		samples = append(samples, sample{err: errors.New(strings.Repeat("e", i+1))})
	}
	c := aggregate(1, samples, time.Second)
	if c.Failed != 20 || c.Completed != 0 || len(c.Errors) != maxErrorSamples || c.OutputTokS != 0 {
		t.Errorf("got failed %d completed %d errors %d out %v", c.Failed, c.Completed, len(c.Errors), c.OutputTokS)
	}
}

func TestReadStream(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		chunks  int
		prompt  int
		compl   int
		wantErr string
	}{
		{
			name: "vLLM stream with continuous usage, empty-text and usage-only chunks",
			body: "data: {\"choices\":[{\"text\":\" a\"}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":1}}\n\n" +
				": keep-alive comment\n\n" +
				"data: {\"choices\":[{\"text\":\"\"}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2}}\n\n" +
				"data: {\"choices\":[{\"text\":\" bc\"}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":3}}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":3}}\n\n" +
				"data: [DONE]\n\n",
			chunks: 2, prompt: 9, compl: 3,
		},
		{
			name:    "error event mid stream",
			body:    "data: {\"choices\":[{\"text\":\" a\"}]}\n\ndata: {\"error\":{\"message\":\"engine dead\"}}\n\n",
			wantErr: "engine dead",
		},
		{
			name:    "no text",
			body:    "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":0}}\n\ndata: [DONE]\n\n",
			wantErr: "no output tokens",
		},
		{
			name:    "garbage",
			body:    "data: {not json}\n\n",
			wantErr: "bad stream chunk",
		},
	}
	for _, tt := range tests {
		s := readStream(strings.NewReader(tt.body), time.Now(), false)
		if tt.wantErr != "" {
			if s.err == nil || !strings.Contains(s.err.Error(), tt.wantErr) {
				t.Errorf("%s: err %v, want %q", tt.name, s.err, tt.wantErr)
			}
			continue
		}
		if s.err != nil || len(s.chunks) != tt.chunks || s.promptTokens != tt.prompt || s.completionTokens != tt.compl {
			t.Errorf("%s: got err %v chunks %d prompt %d compl %d", tt.name, s.err, len(s.chunks), s.promptTokens, s.completionTokens)
		}
		// The second text chunk carries the token hidden in the empty chunk.
		if tt.chunks == 2 && (s.chunks[0].cum != 1 || s.chunks[1].cum != 3) {
			t.Errorf("%s: cum %v", tt.name, s.chunks)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	for body, want := range map[string]string{
		`{"error":{"message":"The model x does not exist.","code":404}}`: "The model x does not exist.",
		`{"error":"plain"}`:      "plain",
		`{"message":"top"}`:      "top",
		`{"detail":"Not Found"}`: "Not Found",
		"  upstream timeout \n":  "upstream timeout",
	} {
		if got := errorMessage([]byte(body)); got != want {
			t.Errorf("errorMessage(%q) = %q, want %q", body, got, want)
		}
	}
}
