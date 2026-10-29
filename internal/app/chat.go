package app

import (
	"context"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// ChatDelta is one piece of a streamed chat answer: the text of the answer
// and, when the model sends one, the reasoning behind it.
type ChatDelta = vllm.ChatDelta

// ChatResult carries the numbers of a streamed chat answer.
type ChatResult = vllm.ChatResult

// ChatOptions are the inputs of one chat request.
type ChatOptions struct {
	URL       string // base URL of the server; "" = the running run
	Model     string // model name; "" = the first id of /v1/models
	Prompt    string // the prompt to send
	MaxTokens int    // the cap on the answer; 0 = the default of the server
}

// ChatTarget returns the URL of the run a chat request talks to: the run
// that is serving. It fails with onboarding.ErrNoLiveRun when nothing
// serves.
func ChatTarget(ctx context.Context) (string, error) {
	run, err := onboarding.LiveRun(ctx)
	if err != nil {
		return "", err
	}
	return run.URL, nil
}

// ChatStream opens a streamed chat answer from a server: the answer comes
// piece by piece while the server writes it.
func ChatStream(ctx context.Context, o ChatOptions) (*vllm.ChatStream, error) {
	url := o.URL
	if url == "" {
		run, err := onboarding.LiveRun(ctx)
		if err != nil {
			return nil, err
		}
		url = run.URL
	}
	return vllm.New(url).ChatStream(ctx, o.Model, o.Prompt, o.MaxTokens)
}
