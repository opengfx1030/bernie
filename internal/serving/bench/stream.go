package bench

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

// requestTimeout bounds one benchmark request end to end.
const requestTimeout = 30 * time.Minute

// chunk is one streamed text chunk.
type chunk struct {
	at  time.Duration // since the request was sent
	cum int           // cumulative completion tokens (continuous usage), -1 when unknown
}

// sample is the raw record of one request.
type sample struct {
	chunks           []chunk // non-empty text chunks only
	promptTokens     int     // from usage; 0 when absent
	completionTokens int     // from usage; 0 when absent
	reply            string  // the streamed text; only kept for the recall check
	expect           string  // the pass key the reply must hold; empty without the check
	err              error
}

type streamChunk struct {
	Choices []struct {
		Text string `json:"text"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// streamCompletion sends one streaming /v1/completions request and records
// chunk arrival times. With collect, the reply text is kept so the pass key
// check can look at it.
func streamCompletion(ctx context.Context, hc *http.Client, baseURL, model, prompt string, maxTokens int, collect bool) sample {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{
		"model":       model,
		"prompt":      prompt,
		"max_tokens":  maxTokens,
		"temperature": 0,
		"ignore_eos":  true,
		"stream":      true,
		"stream_options": map[string]bool{
			"include_usage":          true,
			"continuous_usage_stats": true,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/completions", bytes.NewReader(body))
	if err != nil {
		return sample{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	start := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		return sample{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return sample{err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorMessage(b))}
	}
	return readStream(resp.Body, start, collect)
}

// readStream parses a server-sent event stream of completion chunks. With
// collect, the text of the chunks is kept in the sample.
func readStream(r io.Reader, start time.Time, collect bool) sample {
	var s sample
	var reply strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue // blank separators, comments, event/id fields
		}
		at := time.Since(start)
		data := bytes.TrimSpace(line[len("data:"):])
		if string(data) == "[DONE]" {
			break
		}
		var ch streamChunk
		if err := json.Unmarshal(data, &ch); err != nil {
			s.err = fmt.Errorf("bad stream chunk: %w", err)
			return s
		}
		if len(ch.Error) > 0 && string(ch.Error) != "null" {
			s.err = fmt.Errorf("stream error: %s", errorMessage(data))
			return s
		}
		cum := -1
		if ch.Usage != nil {
			s.promptTokens = ch.Usage.PromptTokens
			s.completionTokens = ch.Usage.CompletionTokens
			cum = ch.Usage.CompletionTokens
		}
		text := false
		for _, c := range ch.Choices {
			if c.Text != "" {
				text = true
				if collect {
					reply.WriteString(c.Text)
				}
			}
		}
		if text {
			s.chunks = append(s.chunks, chunk{at: at, cum: cum})
		}
	}
	if err := sc.Err(); err != nil {
		s.err = err
		return s
	}
	if len(s.chunks) == 0 {
		s.err = errors.New("no output tokens streamed")
	}
	s.reply = reply.String()
	return s
}

// errorMessage extracts a readable message from an OpenAI-style error body
// ({"error": {"message": ...}}, {"error": "..."}, {"message": ...} or
// {"detail": ...}); otherwise the trimmed body.
func errorMessage(b []byte) string {
	var v struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Detail  any             `json:"detail"`
	}
	if json.Unmarshal(b, &v) == nil {
		var inner struct {
			Message string `json:"message"`
		}
		var str string
		switch {
		case len(v.Error) > 0 && json.Unmarshal(v.Error, &inner) == nil && inner.Message != "":
			return inner.Message
		case len(v.Error) > 0 && json.Unmarshal(v.Error, &str) == nil && str != "":
			return str
		case v.Message != "":
			return v.Message
		case v.Detail != nil:
			return fmt.Sprint(v.Detail)
		}
	}
	msg := strings.TrimSpace(string(b))
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}
