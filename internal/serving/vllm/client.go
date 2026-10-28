// Package vllm is a small client for the HTTP endpoints of a vLLM
// OpenAI-compatible server: /health, /v1/models, /tokenize and /metrics.
// Every call is bounded: when the context has no deadline, DefaultTimeout
// applies.
package vllm

import (
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

// DefaultTimeout bounds a call whose context has no deadline.
const DefaultTimeout = 10 * time.Second

// maxBody caps how much of a response body is read (metrics pages of large
// deployments stay well below this).
const maxBody = 16 << 20

// ErrUnsupported reports that the server does not implement an endpoint
// (HTTP 404, 405 or 501).
var ErrUnsupported = errors.New("endpoint not supported by server")

// StatusError is a non-2xx answer from the server.
type StatusError struct {
	Method, URL string
	Code        int
	Body        string // first bytes of the body, trimmed
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.URL, e.Code)
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.URL, e.Code, e.Body)
}

// Client talks to one server. The zero value is not usable; call New.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for baseURL (for example "http://127.0.0.1:8000").
// A trailing slash or "/v1" suffix is removed.
func New(baseURL string) *Client {
	b := strings.TrimRight(baseURL, "/")
	b = strings.TrimSuffix(b, "/v1")
	return &Client{base: b, http: &http.Client{}}
}

// BaseURL returns the normalized server base URL (no trailing slash).
func (c *Client) BaseURL() string { return c.base }

// Health returns nil when GET /health answers 2xx.
func (c *Client) Health(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/health", nil)
	return err
}

// Models returns the model ids of GET /v1/models, in server order.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	body, err := c.do(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse /v1/models: %w", err)
	}
	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// Tokenize returns the token count of text as the server's tokenizer sees a
// completion prompt (special tokens included). ErrUnsupported when the server
// has no /tokenize endpoint.
func (c *Client) Tokenize(ctx context.Context, model, text string) (int, error) {
	req, _ := json.Marshal(map[string]string{"model": model, "prompt": text})
	body, err := c.do(ctx, http.MethodPost, "/tokenize", req)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Count  *int  `json:"count"`
		Tokens []int `json:"tokens"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse /tokenize: %w", err)
	}
	if resp.Count != nil {
		return *resp.Count, nil
	}
	if resp.Tokens != nil {
		return len(resp.Tokens), nil
	}
	return 0, fmt.Errorf("parse /tokenize: no count or tokens in response")
}

// Metrics returns the flattened Prometheus samples of GET /metrics, keyed by
// `name{labels}` exactly as exposed (just `name` when there are no labels).
func (c *Client) Metrics(ctx context.Context) (map[string]float64, error) {
	body, err := c.do(ctx, http.MethodGet, "/metrics", nil)
	if err != nil {
		return nil, err
	}
	return ParseMetrics(body)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	url := c.base + path
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound,
		resp.StatusCode == http.StatusMethodNotAllowed,
		resp.StatusCode == http.StatusNotImplemented:
		return nil, fmt.Errorf("%s %s: %w", method, path, ErrUnsupported)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		snippet := strings.TrimSpace(string(data))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return nil, &StatusError{Method: method, URL: url, Code: resp.StatusCode, Body: snippet}
	}
	return data, nil
}
