package vllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// ChatDelta is one piece of a streamed chat answer: the text of the answer
// and, when the model sends one, the reasoning behind it.
type ChatDelta struct {
	Text      string // the text of the answer
	Reasoning string // the reasoning of the model, when it sends one
}

// ChatResult carries the numbers of a streamed chat answer.
type ChatResult struct {
	Text      string        // the whole answer
	Reasoning string        // the whole reasoning
	TTFT      time.Duration // the time to the first token of the answer
	Tokens    int           // the number of tokens the server sent
	Total     time.Duration // the time of the whole answer
	TokS      float64       // the tokens per second after the first token
}

// ChatStream is an open streamed chat completion.
type ChatStream struct {
	resp  *http.Response
	start time.Time
	model string
}

// ChatStream opens a streamed chat completion on the server. The model may
// be empty: the first id of /v1/models is then used. The call is bounded by
// the context of the caller, not by DefaultTimeout: an answer takes as long
// as it takes. The caller closes the stream.
func (c *Client) ChatStream(ctx context.Context, model, prompt string, maxTokens int) (*ChatStream, error) {
	if maxTokens < 1 {
		maxTokens = 1
	}
	if model == "" {
		ids, err := c.Models(ctx)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, errors.New("the server serves no model")
		}
		model = ids[0]
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens, "temperature": 0,
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, &StatusError{
			Method: http.MethodPost, URL: c.base + "/v1/chat/completions",
			Code: resp.StatusCode, Body: snippet(b),
		}
	}
	return &ChatStream{resp: resp, start: start, model: model}, nil
}

// Model is the model the stream asks for.
func (s *ChatStream) Model() string { return s.model }

// Close releases the stream. It ends the request of the server.
func (s *ChatStream) Close() error { return s.resp.Body.Close() }

// Read reads the answer of the server and calls onDelta for every piece as
// it comes. An error of the callback stops the read and is returned.
func (s *ChatStream) Read(onDelta func(ChatDelta) error) (ChatResult, error) {
	var out ChatResult
	var text, reasoning strings.Builder
	sc := bufio.NewScanner(s.resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	chunks, usage := 0, 0
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue // blank separators, comments, event and id fields
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if string(data) == "[DONE]" {
			break
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(data, &ch); err != nil {
			return out, fmt.Errorf("parse the chat stream: %w", err)
		}
		if len(ch.Error) > 0 && string(ch.Error) != "null" {
			return out, errors.New("the server failed while it was answering: " + snippet(data))
		}
		if ch.Usage != nil && ch.Usage.CompletionTokens > 0 {
			usage = ch.Usage.CompletionTokens
		}
		for _, c := range ch.Choices {
			d := ChatDelta{Text: c.Delta.Content, Reasoning: c.Delta.ReasoningContent}
			if d.Reasoning == "" {
				d.Reasoning = c.Delta.Reasoning
			}
			if d.Text == "" && d.Reasoning == "" {
				continue
			}
			if d.Text != "" {
				if chunks == 0 {
					out.TTFT = time.Since(s.start)
				}
				chunks++
				text.WriteString(d.Text)
			}
			reasoning.WriteString(d.Reasoning)
			if err := onDelta(d); err != nil {
				return out, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	out.Text = text.String()
	out.Reasoning = reasoning.String()
	out.Total = time.Since(s.start)
	out.Tokens = usage
	if out.Tokens == 0 {
		out.Tokens = chunks
	}
	if d := out.Total - out.TTFT; d > 0 {
		out.TokS = math.Round(float64(out.Tokens)/d.Seconds()*10) / 10
	}
	return out, nil
}
