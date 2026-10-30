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
// server: the reply, the time to the first token, the token count, the
// output speed and the verdict of the check.
func TestSmokePromptMeasuresTheAnswer(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{
		Model: "fake-model", TTFT: 20 * time.Millisecond,
		ChunkDelay: 2 * time.Millisecond, ChatReply: "4",
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	res, err := SmokePrompt(context.Background(), srv.URL, "", smokePrompt)
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
	if res.Reply != "4" {
		t.Errorf("the reply is %q, want 4", res.Reply)
	}
	if !res.OK || res.Check != "ok: contains 4" {
		t.Errorf("the check is %q (ok %v), want %q", res.Check, res.OK, "ok: contains 4")
	}
	if res.TTFTMS < 20 {
		t.Errorf("the time to the first token is %d ms, want at least 20", res.TTFTMS)
	}
	if res.OutputTokens != 1 {
		t.Errorf("the token count is %d, want 1", res.OutputTokens)
	}
	if res.TokS <= 0 {
		t.Errorf("the output speed is %v, want more than 0", res.TokS)
	}
	// The smoke asks through the chat completions endpoint: the model
	// applies its chat template to the prompt.
	prompts := fake.ChatPrompts()
	if len(prompts) != 1 || prompts[0] != smokePrompt {
		t.Errorf("the server got the chat prompts %v, want [%q]", prompts, smokePrompt)
	}
	if raw := fake.Prompts(); len(raw) != 0 {
		t.Errorf("the server got the raw completion prompts %v, want none", raw)
	}
}

// TestSmokePromptUsesTheDefaultPrompt checks that an empty prompt asks
// the default one, and that the default one is a question with a known
// answer.
func TestSmokePromptUsesTheDefaultPrompt(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{Model: "fake-model", ChatReply: "4"})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	if _, err := SmokePrompt(context.Background(), srv.URL, "fake-model", ""); err != nil {
		t.Fatal(err)
	}
	prompts := fake.ChatPrompts()
	if len(prompts) != 1 || prompts[0] != smokePrompt {
		t.Fatalf("the server got the chat prompts %v, want [%q]", prompts, smokePrompt)
	}
	if !strings.Contains(smokePrompt, "2+2") {
		t.Errorf("the default prompt %q asks no arithmetic question", smokePrompt)
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

// TestSmokeCheckAnswer: the verdict of the check says whether the answer
// carried the number the prompt asked for. A server that answers
// something else still answers: the verdict says so without failing.
func TestSmokeCheckAnswer(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		reply  string
		wantOK bool
		want   string
	}{
		{name: "the number", prompt: smokePrompt, reply: "4", wantOK: true, want: "ok: contains 4"},
		{name: "in a sentence", prompt: smokePrompt, reply: "The answer is 4.", wantOK: true, want: "ok: contains 4"},
		{name: "another number", prompt: smokePrompt, reply: "5", wantOK: false, want: "answered"},
		{name: "a longer number", prompt: smokePrompt, reply: "42", wantOK: false, want: "answered"},
		{name: "no number", prompt: smokePrompt, reply: "I cannot answer that.", wantOK: false, want: "answered"},
		{name: "a prompt of the caller", prompt: "What is the sky?", reply: "blue", wantOK: false, want: "answered"},
	}
	for _, c := range cases {
		ok, got := checkAnswer(c.prompt, c.reply)
		if ok != c.wantOK || got != c.want {
			t.Errorf("%s: checkAnswer(%q, %q) = %v, %q, want %v, %q",
				c.name, c.prompt, c.reply, ok, got, c.wantOK, c.want)
		}
	}
}
