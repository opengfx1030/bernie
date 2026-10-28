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

func TestPercentile(t *testing.T) {
	tests := []struct {
		xs   []float64
		p    float64
		want float64
	}{
		{nil, 50, 0},
		{[]float64{7}, 95, 7},
		{[]float64{4, 1, 3, 2}, 50, 2.5},
		{[]float64{1, 2, 3, 4, 5}, 50, 3},
		{[]float64{1, 2, 3, 4, 5}, 95, 4.8},
		{[]float64{10, 20}, 100, 20},
		{[]float64{10, 20}, 0, 10},
	}
	for _, tt := range tests {
		xs := append([]float64(nil), tt.xs...)
		if got := percentile(xs, tt.p); !near(got, tt.want) {
			t.Errorf("percentile(%v, %v) = %v, want %v", tt.xs, tt.p, got, tt.want)
		}
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
	want := Cell{
		Concurrency: 4, NumPrompts: 6, Completed: 3, Failed: 3,
		TTFTP50: 200, TTFTP95: 290, ITLP50: 10, ITLP95: 10,
		DecodeTokS: 100, OutputTokS: 3, DurationS: 2,
		MeanPromptTokens: 100, MeanOutputTokens: 2, TokensPerChunk: 1,
		Errors: []string{"HTTP 500: boom", "no output tokens streamed"},
	}
	// Prefill is the mean of 100/0.1, 100/0.2, 100/0.3.
	want.PrefillTokS = (1000 + 500 + 1000.0/3) / 3
	if !near(c.TTFTP95, want.TTFTP95) || !near(c.PrefillTokS, want.PrefillTokS) || !near(c.ITLP50, want.ITLP50) {
		t.Errorf("percentiles/prefill: got %+v", c)
	}
	c.TTFTP95, want.TTFTP95 = 0, 0
	c.PrefillTokS, want.PrefillTokS = 0, 0
	c.ITLP50, c.ITLP95, want.ITLP50, want.ITLP95 = 0, 0, 0, 0
	if !reflect.DeepEqual(c, want) {
		t.Errorf("aggregate:\n got %+v\nwant %+v", c, want)
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
		s := readStream(strings.NewReader(tt.body), time.Now())
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
