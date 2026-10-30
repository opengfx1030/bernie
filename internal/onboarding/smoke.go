package onboarding

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// ErrNoLiveRun means no run of this host is serving.
var ErrNoLiveRun = errors.New("no server is running")

// ErrRunGone means the run asked for is not serving.
var ErrRunGone = errors.New("the run is not serving")

// ErrNoRun means there is no run with the name asked for.
var ErrNoRun = errors.New("no run with that name")

// SmokeResult is the smoke check document: the answer of the server, the
// numbers of the answer and the verdict of the check (spec §3.1).
type SmokeResult struct {
	URL    string `json:"url"`
	Model  string `json:"model"`
	Health bool   `json:"health"`
	Reply  string `json:"reply"`
	// TTFTMS is the time to the first token, in milliseconds.
	TTFTMS int `json:"ttft_ms"`
	// OutputTokens is the number of tokens the server sent.
	OutputTokens int `json:"output_tokens"`
	// TokS is the output speed: the tokens per second after the first one.
	TokS float64 `json:"tok_s"`
	// OK says the answer carried the number the prompt asked for.
	OK bool `json:"ok"`
	// Check is the verdict: "ok: contains 4" when the answer has the
	// expected number, "answered" when the server answered something
	// else. The smoke does not fail on the verdict; it is information.
	Check string `json:"check"`
}

// smokePrompt is the prompt of the smoke check: a short question with a
// known answer. It rides through the chat completions endpoint so the
// model applies its chat template; a raw completion has no template
// and the answer reads as unrelated text.
const smokePrompt = "What is 2+2? Answer with one number."

// smokeAnswer is the number the prompt asks for. The check looks for it
// as a whole word so "4" matches but "42" or "14" do not.
const smokeAnswer = "4"

// smokeAnswerRE matches the expected answer as a whole token.
var smokeAnswerRE = regexp.MustCompile(`\b` + regexp.QuoteMeta(smokeAnswer) + `\b`)

// smokeTokens bounds the answer of the smoke check.
const smokeTokens = 64

// Smoke checks that a server answers: /health first, then one chat
// completion of the default prompt. It is the smallest request that
// proves the server works.
func Smoke(ctx context.Context, baseURL, model string) (*SmokeResult, error) {
	return SmokePrompt(ctx, baseURL, model, smokePrompt)
}

// SmokePrompt checks that a server answers the prompt of the caller
// through the chat completions endpoint: /health first, then one
// streamed chat completion. The result carries the text of the answer,
// the time to the first token, the number of tokens, the output speed
// and the verdict of the check.
func SmokePrompt(ctx context.Context, baseURL, model, prompt string) (*SmokeResult, error) {
	if prompt == "" {
		prompt = smokePrompt
	}
	c := vllm.New(baseURL)
	if err := c.Health(ctx); err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	stream, err := c.ChatStream(ctx, model, prompt, smokeTokens)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}
	defer stream.Close()
	res, err := stream.Read(func(vllm.ChatDelta) error { return nil })
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}
	out := &SmokeResult{
		URL:          c.BaseURL(),
		Model:        stream.Model(),
		Health:       true,
		Reply:        strings.TrimSpace(res.Text),
		TTFTMS:       int(res.TTFT.Milliseconds()),
		OutputTokens: res.Tokens,
	}
	if d := res.Total - res.TTFT; d > 0 {
		out.TokS = math.Round(float64(res.Tokens)/d.Seconds()*10) / 10
	}
	out.OK, out.Check = checkAnswer(prompt, out.Reply)
	return out, nil
}

// checkAnswer says whether the answer carried the number the prompt
// asked for. Only the default prompt has a known answer; a prompt of
// the caller has none, so the check says the server answered without
// claiming a specific value.
func checkAnswer(prompt, reply string) (bool, string) {
	if prompt != smokePrompt {
		return false, "answered"
	}
	if smokeAnswerRE.MatchString(reply) {
		return true, "ok: contains " + smokeAnswer
	}
	return false, "answered"
}

// LiveRun returns the run that is not exited.
func LiveRun(ctx context.Context) (*Run, error) {
	list, err := runner.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.State != runner.StateExited {
			return &Run{Name: r.Name, Recipe: r.Recipe, State: r.State, Mode: r.Mode, URL: r.URL()}, nil
		}
	}
	return nil, ErrNoLiveRun
}

// RunByName returns the run with this name when it is serving. It fails with
// ErrRunGone when the run has exited and with a plain error when there is no
// such run.
func RunByName(ctx context.Context, name string) (*Run, error) {
	list, err := runner.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.Name != name {
			continue
		}
		if r.State == runner.StateExited {
			return nil, fmt.Errorf("%w: %s", ErrRunGone, name)
		}
		return &Run{Name: r.Name, Recipe: r.Recipe, State: r.State, Mode: r.Mode, URL: r.URL()}, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoRun, name)
}
