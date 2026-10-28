// Package vllmtest is a deterministic fake of the vLLM OpenAI-compatible
// server for tests: /health, /v1/models, /tokenize, /metrics and streaming
// /v1/completions with controlled delays, multi-token chunks (as with MTP
// speculative decoding) and failure injection.
package vllmtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config controls the fake. Zero values give an instant, healthy server.
type Config struct {
	Model string // served model id; default "fake-model"
	// TTFT is the delay before the first chunk; PrefillPerToken adds a delay
	// per prompt token on top of it.
	TTFT            time.Duration
	PrefillPerToken time.Duration
	// ChunkDelay is the delay before every chunk after the first.
	ChunkDelay time.Duration
	// TokensPerChunk is the token count of every chunk after the first
	// (the first chunk always carries one token). Default 1.
	TokensPerChunk int
	// NoTokenize makes /tokenize answer 404.
	NoTokenize bool
	// NoContinuousUsage ignores stream_options.continuous_usage_stats, so
	// usage arrives only in the final chunk.
	NoContinuousUsage bool
	// FailEvery makes every Nth completion request (counted from 1 in
	// arrival order) answer HTTP 500.
	FailEvery int
	// FailMidStreamEvery makes every Nth completion request send an error
	// event after its first chunk.
	FailMidStreamEvery int
}

// Server is an http.Handler. Use it with httptest.NewServer.
type Server struct {
	cfg Config

	mu       sync.Mutex
	prompts  []string
	requests int
	running  int
	genTotal int
}

// New returns a fake server.
func New(cfg Config) *Server {
	if cfg.Model == "" {
		cfg.Model = "fake-model"
	}
	if cfg.TokensPerChunk <= 0 {
		cfg.TokensPerChunk = 1
	}
	return &Server{cfg: cfg}
}

// CountTokens is the fake tokenizer: one BOS token, plus one token per
// whitespace-separated word, plus one more for words longer than 7 bytes.
func CountTokens(text string) int {
	n := 1
	for _, w := range strings.Fields(text) {
		n++
		if len(w) > 7 {
			n++
		}
	}
	return n
}

// Prompts returns the prompts of all completion requests in arrival order.
func (s *Server) Prompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.prompts...)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health" && r.Method == http.MethodGet:
		w.WriteHeader(http.StatusOK)
	case r.URL.Path == "/v1/models" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": []map[string]any{
			{"id": s.cfg.Model, "object": "model", "owned_by": "vllm", "max_model_len": 32768},
		}})
	case r.URL.Path == "/tokenize" && r.Method == http.MethodPost && !s.cfg.NoTokenize:
		s.tokenize(w, r)
	case r.URL.Path == "/metrics" && r.Method == http.MethodGet:
		s.metrics(w)
	case r.URL.Path == "/v1/completions" && r.Method == http.MethodPost:
		s.completions(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "Not Found"})
	}
}

func (s *Server) tokenize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	n := CountTokens(req.Prompt)
	writeJSON(w, http.StatusOK, map[string]any{"count": n, "max_model_len": 32768, "tokens": make([]int, n)})
}

func (s *Server) metrics(w http.ResponseWriter) {
	s.mu.Lock()
	running, gen := s.running, s.genTotal
	s.mu.Unlock()
	m := s.cfg.Model
	fmt.Fprintf(w, "# HELP vllm:num_requests_running Number of requests in model execution batches.\n")
	fmt.Fprintf(w, "# TYPE vllm:num_requests_running gauge\n")
	fmt.Fprintf(w, "vllm:num_requests_running{engine=\"0\",model_name=\"%s\"} %d\n", m, running)
	fmt.Fprintf(w, "vllm:num_requests_waiting{engine=\"0\",model_name=\"%s\"} 0\n", m)
	fmt.Fprintf(w, "vllm:kv_cache_usage_perc{engine=\"0\",model_name=\"%s\"} 0.25\n", m)
	fmt.Fprintf(w, "vllm:generation_tokens_total{engine=\"0\",model_name=\"%s\"} %d\n", m, gen)
}

type completionRequest struct {
	Model         string  `json:"model"`
	Prompt        string  `json:"prompt"`
	MaxTokens     int     `json:"max_tokens"`
	Stream        bool    `json:"stream"`
	Temperature   float64 `json:"temperature"`
	IgnoreEOS     bool    `json:"ignore_eos"`
	StreamOptions *struct {
		IncludeUsage         bool `json:"include_usage"`
		ContinuousUsageStats bool `json:"continuous_usage_stats"`
	} `json:"stream_options"`
}

func (s *Server) completions(w http.ResponseWriter, r *http.Request) {
	var req completionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	s.mu.Lock()
	s.requests++
	n := s.requests
	s.prompts = append(s.prompts, req.Prompt)
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
	failMid := s.cfg.FailMidStreamEvery > 0 && n%s.cfg.FailMidStreamEvery == 0
	usage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage
	continuous := usage && req.StreamOptions.ContinuousUsageStats && !s.cfg.NoContinuousUsage
	promptTokens := CountTokens(req.Prompt)

	s.mu.Lock()
	s.running++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running--
		s.mu.Unlock()
	}()

	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	if fl != nil {
		fl.Flush()
	}
	id := fmt.Sprintf("cmpl-fake-%d", n)
	send := func(v any) {
		b, _ := json.Marshal(v)
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

	if !sleep(s.cfg.TTFT + time.Duration(promptTokens)*s.cfg.PrefillPerToken) {
		return
	}
	done := 0
	for done < req.MaxTokens {
		k := s.cfg.TokensPerChunk
		if done == 0 {
			k = 1
		} else if !sleep(s.cfg.ChunkDelay) {
			return
		}
		if k > req.MaxTokens-done {
			k = req.MaxTokens - done
		}
		done += k
		var finish any
		if done == req.MaxTokens {
			finish = "length"
		}
		chunk := map[string]any{
			"id": id, "object": "text_completion", "created": 0, "model": s.cfg.Model,
			"choices": []map[string]any{{"index": 0, "text": strings.Repeat(" x", k), "logprobs": nil, "finish_reason": finish, "stop_reason": nil}},
			"usage":   nil,
		}
		if continuous {
			chunk["usage"] = usageBody(promptTokens, done)
		}
		send(chunk)
		s.mu.Lock()
		s.genTotal += k
		s.mu.Unlock()
		if failMid {
			send(map[string]any{"error": map[string]any{"message": "injected mid-stream failure", "type": "InternalServerError", "code": 500}})
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
	}
	if usage {
		send(map[string]any{"id": id, "object": "text_completion", "created": 0, "model": s.cfg.Model,
			"choices": []any{}, "usage": usageBody(promptTokens, done)})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
}

func usageBody(prompt, completion int) map[string]int {
	return map[string]int{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": prompt + completion}
}

func errBody(msg string) map[string]any {
	return map[string]any{"error": map[string]any{"message": msg, "type": "BadRequestError", "code": 400}}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
