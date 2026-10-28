package vllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Completion is the answer of a streamed completion: the text of the reply,
// the time to the first token, the number of tokens the server sent and the
// time of the whole answer.
type Completion struct {
	Text   string
	TTFT   time.Duration
	Tokens int
	Total  time.Duration
}

// CompleteStream asks the server for a completion and reads the answer while
// it comes. The token count comes from the usage of the server when it sends
// one; otherwise it is the number of chunks that carried text.
func (c *Client) CompleteStream(ctx context.Context, model, prompt string, maxTokens int) (Completion, error) {
	if maxTokens < 1 {
		maxTokens = 1
	}
	body := map[string]any{
		"prompt": prompt, "max_tokens": maxTokens, "temperature": 0,
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
	}
	if model != "" {
		body["model"] = model
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Completion{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/completions", bytes.NewReader(raw))
	if err != nil {
		return Completion{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return Completion{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Completion{}, &StatusError{
			Method: http.MethodPost, URL: c.base + "/v1/completions",
			Code: resp.StatusCode, Body: snippet(b),
		}
	}
	res, err := readCompletion(resp.Body, start)
	if err != nil {
		return Completion{}, err
	}
	return res, nil
}

// readCompletion parses the server-sent events of a completion.
func readCompletion(r io.Reader, start time.Time) (Completion, error) {
	var out Completion
	var text strings.Builder
	sc := bufio.NewScanner(r)
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
				Text string `json:"text"`
			} `json:"choices"`
			Usage *struct {
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(data, &ch); err != nil {
			return out, fmt.Errorf("parse the completion stream: %w", err)
		}
		if len(ch.Error) > 0 && string(ch.Error) != "null" {
			return out, errors.New("the server failed while it was answering: " + snippet(data))
		}
		if ch.Usage != nil && ch.Usage.CompletionTokens > 0 {
			usage = ch.Usage.CompletionTokens
		}
		for _, c := range ch.Choices {
			if c.Text == "" {
				continue
			}
			if chunks == 0 {
				out.TTFT = time.Since(start)
			}
			chunks++
			text.WriteString(c.Text)
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	out.Text = text.String()
	out.Total = time.Since(start)
	out.Tokens = usage
	if out.Tokens == 0 {
		out.Tokens = chunks
	}
	return out, nil
}

// snippet cuts a body to the size of an error message.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
