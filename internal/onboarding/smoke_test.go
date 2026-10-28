package onboarding

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// TestSmokePromptMeasuresTheAnswer checks the smoke check against a fake
// server: the reply, the time to the first token, the token count and the
// output speed.
func TestSmokePromptMeasuresTheAnswer(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{
		Model: "fake-model", TTFT: 20 * time.Millisecond,
		ChunkDelay: 2 * time.Millisecond, TokensPerChunk: 2,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	res, err := SmokePrompt(context.Background(), srv.URL, "", "Hello")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Health {
		t.Error("the smoke check says the server is not healthy")
	}
	if res.Model != "fake-model" {
		t.Errorf("the model is %q, want fake-model", res.Model)
	}
	if res.URL != srv.URL {
		t.Errorf("the url is %q, want %q", res.URL, srv.URL)
	}
	if res.Reply == "" {
		t.Error("the reply is empty")
	}
	if res.TTFTMS < 20 {
		t.Errorf("the time to the first token is %d ms, want at least 20", res.TTFTMS)
	}
	if res.OutputTokens != 64 {
		t.Errorf("the token count is %d, want 64", res.OutputTokens)
	}
	if res.TokS <= 0 {
		t.Errorf("the output speed is %v, want more than 0", res.TokS)
	}
	if prompts := fake.Prompts(); len(prompts) != 1 || prompts[0] != "Hello" {
		t.Errorf("the server got the prompts %v", prompts)
	}
}

// TestSmokePromptUsesTheDefaultPrompt checks that an empty prompt asks the
// default one.
func TestSmokePromptUsesTheDefaultPrompt(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{Model: "fake-model"})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	if _, err := SmokePrompt(context.Background(), srv.URL, "fake-model", ""); err != nil {
		t.Fatal(err)
	}
	prompts := fake.Prompts()
	if len(prompts) != 1 || prompts[0] != "Hello" {
		t.Fatalf("the server got the prompts %v, want [Hello]", prompts)
	}
}

// TestSmokeFailsWhenTheServerIsDown checks the failure of the smoke check.
func TestSmokeFailsWhenTheServerIsDown(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{}))
	url := srv.URL
	srv.Close()
	_, err := Smoke(context.Background(), url, "")
	if err == nil {
		t.Fatal("the smoke check of a dead server succeeded")
	}
	if !strings.Contains(err.Error(), "health check failed") {
		t.Errorf("the error is %q, want the health check", err)
	}
}
