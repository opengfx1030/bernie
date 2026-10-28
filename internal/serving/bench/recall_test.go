package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// TestRecallPromptDepth checks that the hidden fact sits halfway in the
// prompt, and that every request of a run has its own salt and its own key.
func TestRecallPromptDepth(t *testing.T) {
	p := prompts{seed: 7, nonce: 42, recall: true}
	const words = 200
	prompt := p.build(3, words)
	key := p.passKey(3)
	if !strings.Contains(prompt, key) {
		t.Fatalf("prompt does not hold the pass key %q", key)
	}
	at := strings.Index(prompt, passKeySentence(key))
	if at < 0 {
		t.Fatalf("prompt has no pass key sentence")
	}
	ratio := float64(at) / float64(len(prompt))
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("pass key at %.0f%% of the prompt, want about 50%%", ratio*100)
	}
	if !strings.HasSuffix(prompt, recallQuestion) {
		t.Errorf("prompt does not ask for the fact")
	}
	salts, keys := map[string]bool{}, map[string]bool{}
	for seq := range 200 {
		s := p.salt(seq)
		if salts[s] {
			t.Fatalf("salt %q repeats", s)
		}
		salts[s] = true
		k := p.passKey(seq)
		if keys[k] {
			t.Fatalf("pass key %q repeats", k)
		}
		keys[k] = true
		if len(k) != 6 {
			t.Errorf("pass key %q is not six digits", k)
		}
	}
}

// TestRecallTokenBudget checks that a recall prompt stays inside the token
// budget, with the server's tokenizer and with the fallback estimate.
func TestRecallTokenBudget(t *testing.T) {
	p := prompts{seed: 3, nonce: 9, recall: true}
	fake := func(ctx context.Context, text string) (int, error) { return vllmtest.CountTokens(text), nil }
	for _, target := range []int{64, 256, 1024, 4096} {
		words, ok, err := calibrate(context.Background(), p, target, fake)
		if err != nil || !ok {
			t.Fatalf("target %d: %v %v", target, ok, err)
		}
		if got := vllmtest.CountTokens(p.build(0, words)); got > target {
			t.Errorf("target %d: %d words = %d tokens, over target", target, words, got)
		}
	}
	// Without /tokenize the estimate keeps room for the fact and the question.
	unsupported := func(context.Context, string) (int, error) { return 0, fmt.Errorf("x: %w", vllm.ErrUnsupported) }
	words, ok, err := calibrate(context.Background(), p, 1024, unsupported)
	if err != nil || ok {
		t.Fatalf("estimate: %v %v", ok, err)
	}
	if want := 1024 - saltOverheadEstimate - recallOverheadEstimate; words != want {
		t.Errorf("estimate %d words, want %d", words, want)
	}
}

// TestRecallVerdict checks the rule: a reply passes when it holds the key.
func TestRecallVerdict(t *testing.T) {
	samples := []sample{
		{expect: "123456", reply: "The pass key is 123456."},
		{expect: "123456", reply: "I do not know."},
		{expect: "123456", err: errors.New("boom")},
		{},
	}
	asked, passed := recallVerdict(samples)
	if asked != 2 || passed != 1 {
		t.Errorf("asked %d passed %d, want 2 and 1", asked, passed)
	}
}

// TestRecallJSONShape checks the JSON of a cell with and without the check.
func TestRecallJSONShape(t *testing.T) {
	cell := Cell{Concurrency: 1, Recall: &Recall{Asked: 4, Passed: 3, Depth: 0.5, Method: recallMethodTokenize}}
	data, err := json.Marshal(cell)
	if err != nil {
		t.Fatal(err)
	}
	want := `"recall":{"asked":4,"passed":3,"depth":0.5,"method":"tokenize"}`
	if !strings.Contains(string(data), want) {
		t.Errorf("cell JSON %s, want it to hold %s", data, want)
	}
	plain, err := json.Marshal(Cell{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "recall") {
		t.Errorf("cell without the check has a recall field: %s", plain)
	}
}

// TestOldResultLoads checks that a result saved before the recall and the
// workload fields still loads, with no recall in its cells.
func TestOldResultLoads(t *testing.T) {
	old := `{"id":"20260101T000000Z-abcdef","url":"http://127.0.0.1:8000","model":"m",` +
		`"time":"2026-01-01T00:00:00Z","options":{"url":"http://127.0.0.1:8000","model":"m",` +
		`"input_len":1024,"output_len":512,"warmup":1,"concurrency":[1]},"fingerprint":{},` +
		`"prompt_words":1000,"calibrated":true,"cells":[{"concurrency":1,"num_prompts":4,` +
		`"completed":4,"failed":0,"ttft_p50_ms":10}]}`
	path := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Workload != nil {
		t.Errorf("old result has a workload: %+v", r.Workload)
	}
	if len(r.Cells) != 1 || r.Cells[0].Recall != nil {
		t.Errorf("old result cells: %+v", r.Cells)
	}
	if r.Options.Recall {
		t.Errorf("old result has the recall check on")
	}
}

// TestRunRecallAgainstFakeServer runs the check against a fake that repeats
// the hidden fact and against one that answers another number.
func TestRunRecallAgainstFakeServer(t *testing.T) {
	for _, mode := range []string{"correct", "wrong"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BERNIE_STATE_DIR", t.TempDir())
			fake := vllmtest.New(vllmtest.Config{Model: "fake-model", RecallMode: mode})
			srv := httptest.NewServer(fake)
			defer srv.Close()
			r, err := Run(context.Background(), Options{
				URL: srv.URL, InputLen: 200, OutputLen: 8, Warmup: 0,
				Concurrency: []int{1}, NumPrompts: 2, Recall: true,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			c := r.Cells[0]
			if c.Recall == nil {
				t.Fatal("cell has no recall")
			}
			if c.Recall.Asked != 2 {
				t.Errorf("asked %d, want 2", c.Recall.Asked)
			}
			want := 0
			if mode == "correct" {
				want = 2
			}
			if c.Recall.Passed != want {
				t.Errorf("passed %d, want %d", c.Recall.Passed, want)
			}
			if c.Recall.Depth != recallDepth || c.Recall.Method != recallMethodTokenize {
				t.Errorf("recall %+v, want depth %v and method %s", c.Recall, recallDepth, recallMethodTokenize)
			}
			if r.Workload == nil || !r.Workload.Recall || r.Workload.CorpusSHA256 == "" {
				t.Errorf("workload: %+v", r.Workload)
			}
			// The prompt carries the key the fake answers.
			prompts := fake.Prompts()
			if len(prompts) == 0 || !strings.Contains(prompts[0], "pass key is") {
				t.Errorf("prompt has no hidden fact: %q", prompts)
			}
		})
	}
}
