package bench

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// sameDist compares two distributions.
func sameDist(got *Dist, want Dist) bool {
	if got == nil {
		return false
	}
	return near(got.Mean, want.Mean) && near(got.P50, want.P50) && near(got.P95, want.P95) &&
		near(got.Min, want.Min) && near(got.Max, want.Max)
}

// scripted builds the sample of a request: it starts at the given offset of
// the cell and its chunks arrive at the given times after the request, one
// token each.
func scripted(start time.Duration, at ...time.Duration) sample {
	chunks := make([]chunk, 0, len(at))
	for i, a := range at {
		chunks = append(chunks, chunk{at: a, cum: i + 1})
	}
	return sample{chunks: chunks, promptTokens: 100, completionTokens: len(chunks), started: start}
}

// TestAggregateWindowsSequential checks the windows of a cell with one
// request after the other.
func TestAggregateWindowsSequential(t *testing.T) {
	samples := []sample{
		scripted(0, 100*ms, 200*ms, 300*ms),
		scripted(350*ms, 100*ms, 200*ms, 300*ms),
	}
	c := aggregate(1, samples, 700*ms)
	// The prefill window runs from the first request (0) to the last first
	// token (0.45 s) and holds the 200 prompt tokens of the cell.
	if c.Prefill == nil || !near(c.Prefill.WindowS, 0.45) {
		t.Fatalf("prefill window = %+v, want 0.45 s", c.Prefill)
	}
	if !near(c.Prefill.AggTokS, 200/0.45) {
		t.Errorf("prefill agg = %v tok/s, want %v", c.Prefill.AggTokS, 200/0.45)
	}
	// The decode window is the union of the decode intervals: 0.2 s plus
	// 0.2 s, and it holds the 4 tokens that came after the first token.
	if c.Decode == nil || !near(c.Decode.WindowS, 0.4) {
		t.Fatalf("decode window = %+v, want 0.4 s", c.Decode)
	}
	if !near(c.Decode.AggTokS, 10) {
		t.Errorf("decode agg = %v tok/s, want 10", c.Decode.AggTokS)
	}
	// Per request: (3 − 1) ÷ (0.3 − 0.1) = 10 tok/s, and 100 ÷ 0.1 = 1000.
	if !near(c.Decode.Req.Mean, 10) || !near(c.Decode.Req.P50, 10) {
		t.Errorf("decode per request = %+v, want 10 tok/s", c.Decode.Req)
	}
	if !near(c.Prefill.Req.Mean, 1000) {
		t.Errorf("prefill per request = %+v, want 1000 tok/s", c.Prefill.Req)
	}
	// The time to first token is in seconds.
	if !near(c.TTFTs.Mean, 0.1) || !near(c.TTFTs.P50, 0.1) {
		t.Errorf("ttft_s = %+v, want 0.1 s", c.TTFTs)
	}
}

// TestAggregateWindowsConcurrent checks the windows of a cell with four
// requests that overlap.
func TestAggregateWindowsConcurrent(t *testing.T) {
	samples := []sample{
		scripted(0, 100*ms, 200*ms, 300*ms),
		scripted(0, 100*ms, 200*ms, 300*ms),
		scripted(0, 150*ms, 250*ms, 350*ms),
		scripted(0, 150*ms, 250*ms, 350*ms),
	}
	c := aggregate(4, samples, 400*ms)
	// The prefill window runs from the first request (0) to the last first
	// token (0.15 s) and holds 400 prompt tokens.
	if c.Prefill == nil || !near(c.Prefill.WindowS, 0.15) {
		t.Fatalf("prefill window = %+v, want 0.15 s", c.Prefill)
	}
	if !near(c.Prefill.AggTokS, 400/0.15) {
		t.Errorf("prefill agg = %v tok/s, want %v", c.Prefill.AggTokS, 400/0.15)
	}
	// The decode intervals overlap: the union is 0.1 s to 0.35 s, and it
	// holds the 8 tokens that came after the first token of every request.
	if c.Decode == nil || !near(c.Decode.WindowS, 0.25) {
		t.Fatalf("decode window = %+v, want 0.25 s", c.Decode)
	}
	if !near(c.Decode.AggTokS, 32) {
		t.Errorf("decode agg = %v tok/s, want 32", c.Decode.AggTokS)
	}
	// Every request had the same speed.
	if !near(c.Decode.Req.Mean, 10) || !near(c.Decode.Req.P95, 10) {
		t.Errorf("decode per request = %+v, want 10 tok/s", c.Decode.Req)
	}
}

// TestMultiTokenChunkITL checks that a chunk with several tokens gives one
// latency per token.
func TestMultiTokenChunkITL(t *testing.T) {
	// Chunks of 1, 2 and 2 tokens, 20 ms apart: every token waits 10 ms.
	s := sample{chunks: []chunk{{100 * ms, 1}, {120 * ms, 3}, {140 * ms, 5}}, promptTokens: 50, completionTokens: 5}
	c := aggregate(1, []sample{s}, 200*ms)
	if c.ITLms == nil || !near(c.ITLms.Mean, 10) || !near(c.ITLms.P50, 10) || !near(c.ITLms.P95, 10) {
		t.Errorf("itl_ms = %+v, want 10 ms", c.ITLms)
	}
	// (5 − 1) ÷ (0.14 − 0.1) = 100 tok/s, and 50 ÷ 0.1 = 500 tok/s.
	if !near(c.Decode.Req.Mean, 100) {
		t.Errorf("decode per request = %+v, want 100 tok/s", c.Decode.Req)
	}
	if !near(c.Prefill.Req.Mean, 500) {
		t.Errorf("prefill per request = %+v, want 500 tok/s", c.Prefill.Req)
	}
}

// TestOldResultLoadsAndConverts checks that a result saved before the
// metric groups existed still loads and shows its numbers.
func TestOldResultLoadsAndConverts(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	old := `{"id":"20261028T120000Z-old01","time":"2026-10-28T12:00:00Z","model":"m","url":"http://127.0.0.1:8000",` +
		`"cells":[{"concurrency":1,"num_prompts":4,"completed":4,"failed":0,"ttft_p50_ms":120,"ttft_p95_ms":180,` +
		`"itl_p50_ms":20,"itl_p95_ms":25,"decode_tok_s":48.5,"prefill_tok_s":850,"output_tok_s":50.1,` +
		`"duration_s":5.1,"mean_prompt_tokens":512,"mean_output_tokens":64,"tokens_per_chunk":1}]}`
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), "20261028T120000Z-old01.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Get("20261028T120000Z-old01")
	if err != nil {
		t.Fatal(err)
	}
	c := r.Cells[0]
	// The milliseconds of the old result become seconds.
	if c.TTFTs == nil || !near(c.TTFTs.P50, 0.12) || !near(c.TTFTs.P95, 0.18) {
		t.Errorf("ttft_s = %+v, want p50 0.12 s and p95 0.18 s", c.TTFTs)
	}
	if c.TTFTs != nil && c.TTFTs.Mean != 0 {
		t.Errorf("ttft_s mean = %v, want 0: the old result did not measure it", c.TTFTs.Mean)
	}
	if c.ITLms == nil || !near(c.ITLms.P50, 20) || !near(c.ITLms.P95, 25) {
		t.Errorf("itl_ms = %+v, want p50 20 ms and p95 25 ms", c.ITLms)
	}
	// The old flat rates are the mean of the per-request rates.
	if c.Prefill == nil || c.Prefill.Req == nil || !near(c.Prefill.Req.Mean, 850) {
		t.Errorf("prefill = %+v, want the mean of the per-request rates 850", c.Prefill)
	}
	if c.Decode == nil || c.Decode.Req == nil || !near(c.Decode.Req.Mean, 48.5) {
		t.Errorf("decode = %+v, want the mean of the per-request rates 48.5", c.Decode)
	}
	// What the old result did not measure stays absent.
	if c.Prefill.AggTokS != 0 || c.Prefill.WindowS != 0 || c.Decode.AggTokS != 0 {
		t.Errorf("the old result has aggregate rates it never measured: %+v %+v", c.Prefill, c.Decode)
	}
	if c.ServerSide != nil {
		t.Errorf("server side = %+v, want none: the old result has none", c.ServerSide)
	}
}

// TestServerSideCrossCheck runs a benchmark against the fake server and
// checks the numbers the server measured over the same requests.
func TestServerSideCrossCheck(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{Model: "fake", TTFT: 40 * ms, ChunkDelay: 10 * ms, TokensPerChunk: 2})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	r, err := Run(context.Background(), Options{
		URL: srv.URL, InputLen: 64, OutputLen: 7, Concurrency: []int{1}, NumPrompts: 3, Warmup: 0,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := r.Cells[0]
	if c.ServerSide == nil {
		t.Fatal("the cell has no server side numbers")
	}
	// The fake server waited 40 ms for the first token of every request.
	if c.ServerSide.Count != 3 {
		t.Errorf("server side count = %d, want 3", c.ServerSide.Count)
	}
	if c.ServerSide.TTFTs == nil || !near(c.ServerSide.TTFTs.Mean, 0.04) {
		t.Errorf("server side ttft = %+v, want a mean of 0.04 s", c.ServerSide.TTFTs)
	}
	// The gap of a chunk holds two tokens: 10 ms ÷ 2 = 5 ms per token.
	if c.ServerSide.ITLms == nil || !near(c.ServerSide.ITLms.Mean, 5) {
		t.Errorf("server side itl = %+v, want a mean of 5 ms", c.ServerSide.ITLms)
	}
	// The client measured the same requests: its time to first token is at
	// least the 40 ms the server waited.
	if c.TTFTs == nil || c.TTFTs.Mean < 0.04 {
		t.Errorf("ttft_s = %+v, want at least 0.04 s", c.TTFTs)
	}
	if c.ITLms == nil || c.ITLms.Mean < 4 {
		t.Errorf("itl_ms = %+v, want about 5 ms", c.ITLms)
	}
	if !c.TokensExact {
		t.Errorf("tokens_exact = false, want true: the fake sent usage")
	}
	// The JSON of the cell carries the new groups and the old flat fields.
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"prefill", "decode", "ttft_s", "itl_ms", "server_side", "tokens_exact",
		"ttft_p50_ms", "itl_p50_ms", "decode_tok_s", "prefill_tok_s", "output_tok_s"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the cell JSON has no %q: %s", key, data)
		}
	}
	pf, _ := doc["prefill"].(map[string]any)
	if _, ok := pf["agg_tok_s"]; !ok {
		t.Errorf("the prefill group has no aggregate rate: %s", data)
	}
	if _, ok := pf["req"].(map[string]any); !ok {
		t.Errorf("the prefill group has no per-request rates: %s", data)
	}
}
