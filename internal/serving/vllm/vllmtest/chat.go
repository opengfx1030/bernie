package vllmtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// chatRequest is the body of a chat completion request.
type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	MaxTokens     int  `json:"max_tokens"`
	Stream        bool `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

// chatAnswer is the reply of the fake to a chat prompt.
const chatAnswer = "The fake server answers one sentence."

// chat streams a chat completion: the words of the answer come one by one.
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	prompt := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			prompt = m.Content
		}
	}
	s.mu.Lock()
	s.requests++
	n := s.requests
	s.chatPrompts = append(s.chatPrompts, prompt)
	s.mu.Unlock()

	switch {
	case req.Model != s.cfg.Model:
		writeJSON(w, http.StatusNotFound, errBody(fmt.Sprintf("The model `%s` does not exist.", req.Model)))
		return
	case !req.Stream:
		writeJSON(w, http.StatusBadRequest, errBody("fake server supports stream=true only"))
		return
	case req.MaxTokens <= 0:
		writeJSON(w, http.StatusBadRequest, errBody("max_tokens must be positive"))
		return
	case s.cfg.FailEvery > 0 && n%s.cfg.FailEvery == 0:
		writeJSON(w, http.StatusInternalServerError, errBody("injected failure"))
		return
	}

	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	if fl != nil {
		fl.Flush()
	}
	id := fmt.Sprintf("chatcmpl-fake-%d", n)
	send := func(delta map[string]any) {
		b, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": 0, "model": s.cfg.Model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": nil}},
			"usage":   nil,
		})
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
	}
	sleep := func(d time.Duration) bool {
		if d <= 0 {
			return r.Context().Err() == nil
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return true
		case <-r.Context().Done():
			return false
		}
	}
	if !sleep(s.cfg.TTFT) {
		return
	}
	if s.cfg.Reasoning {
		send(map[string]any{"role": "assistant", "reasoning_content": "The fake thinks about it."})
	}
	words := strings.Fields(s.cfg.ChatReply)
	if len(words) == 0 {
		words = strings.Fields(chatAnswer)
	}
	if len(words) > req.MaxTokens {
		words = words[:req.MaxTokens]
	}
	for i, word := range words {
		if i > 0 && !sleep(s.cfg.ChunkDelay) {
			return
		}
		send(map[string]any{"content": word + " "})
	}
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		b, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": 0, "model": s.cfg.Model,
			"choices": []any{}, "usage": usageBody(CountTokens(prompt), len(words)),
		})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
}

// ChatPrompts returns the prompts of all chat requests in arrival order.
func (s *Server) ChatPrompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.chatPrompts...)
}
