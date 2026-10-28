package onboarding

import (
	"context"
	"errors"
	"fmt"
	"math"
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

// SmokeResult is the smoke check document: the answer of the server and the
// numbers of the answer.
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
}

// smokePrompt is the prompt of the smoke check: one word.
const smokePrompt = "Hello"

// smokeTokens bounds the answer of the smoke check.
const smokeTokens = 64

// Smoke checks that a server answers: /health first, then one completion of
// the default prompt. It is the smallest request that proves the server works.
func Smoke(ctx context.Context, baseURL, model string) (*SmokeResult, error) {
	return SmokePrompt(ctx, baseURL, model, smokePrompt)
}

// SmokePrompt checks that a server answers the prompt of the caller:
// /health first, then one completion. The result carries the text of the
// answer, the time to the first token, the number of tokens and the output
// speed.
func SmokePrompt(ctx context.Context, baseURL, model, prompt string) (*SmokeResult, error) {
	if prompt == "" {
		prompt = smokePrompt
	}
	c := vllm.New(baseURL)
	if err := c.Health(ctx); err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	if model == "" {
		if ids, err := c.Models(ctx); err == nil && len(ids) > 0 {
			model = ids[0]
		}
	}
	res, err := c.CompleteStream(ctx, model, prompt, smokeTokens)
	if err != nil {
		return nil, fmt.Errorf("completion failed: %w", err)
	}
	out := &SmokeResult{
		URL:          c.BaseURL(),
		Model:        model,
		Health:       true,
		Reply:        strings.TrimSpace(res.Text),
		TTFTMS:       int(res.TTFT.Milliseconds()),
		OutputTokens: res.Tokens,
	}
	if d := res.Total - res.TTFT; d > 0 {
		out.TokS = math.Round(float64(res.Tokens)/d.Seconds()*10) / 10
	}
	return out, nil
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
