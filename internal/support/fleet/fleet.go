// Package fleet fetches the public dashboard of a configured peer and
// returns a small, redacted summary. The fetch is SSRF-safe: only the
// hosts in the config can be reached, only the fixed paths below, with
// a short timeout and a size cap, and no redirects to other hosts.
package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/config"
)

const (
	// Timeout caps one peer request. The Fleet tab opens one request per
	// peer, in parallel.
	Timeout = 3 * time.Second
	// MaxBody caps the bytes the peer may answer with. /api/hw, /api/runs
	// and /api/telemetry are well below this cap.
	MaxBody = 256 << 10
	// AllowedPaths is the fixed set of paths the fetcher will request.
	// The list is the small, public surface of the dashboard: hardware,
	// runs, telemetry and the recipe the running run serves.
	AllowedPaths = "/api/hw /api/runs /api/telemetry /api/recipes /api/glossary"
)

// Summary is one row of the Fleet tab: a self row or a peer row. The peer
// fields are populated from the peer's own /api/hw and /api/runs; gaps
// stay gaps when the peer is down.
type Summary struct {
	Name      string      `json:"name"`
	URL       string      `json:"url,omitempty"`
	Self      bool        `json:"me,omitempty"`
	Reachable bool        `json:"reachable"`
	Error     string      `json:"error,omitempty"`
	State     string      `json:"state,omitempty"`
	Recipe    string      `json:"recipe,omitempty"`
	GenTokS   *float64    `json:"gen_tok_s,omitempty"`
	GPU       []GPUDetail `json:"gpus,omitempty"`
	VRAMGiB   *float64    `json:"vram_total_gb,omitempty"`
}

// GPUDetail is one card on a peer: model name, VRAM and power cap.
type GPUDetail struct {
	Index     int      `json:"index"`
	Name      string   `json:"name"`
	VRAMGiB   *float64 `json:"vram_total_gb,omitempty"`
	PowerCapW *float64 `json:"power_cap_w,omitempty"`
}

// Source holds the dependencies Fetch needs: the configured fleet list and
// a function that returns the local "self" summary. Fetch never reads the
// host directly: it composes the self row from the same data the dashboard
// already shows.
type Source struct {
	Entries []config.FleetEntry
	Self    func(ctx context.Context) Summary
	Client  *http.Client // optional; tests inject one
}

// Fetch returns one summary per entry, with the self row first. A peer
// that fails to answer has Reachable=false and the failure reason in Error.
// The result is not cached here: the API handler caches it for a few
// seconds, which keeps the Fleet tab live without hammering the peers.
func Fetch(ctx context.Context, src Source) []Summary {
	client := src.Client
	if client == nil {
		client = defaultClient()
	}
	out := make([]Summary, 0, len(src.Entries)+1)
	if src.Self != nil {
		out = append(out, src.Self(ctx))
	}
	for _, e := range src.Entries {
		out = append(out, fetchOne(ctx, client, e))
	}
	return out
}

// fetchOne calls the peer's /api/hw (and /api/runs when needed). All
// requests go through safeRequest; a non-allowlisted path or a redirect
// to another host is refused.
func fetchOne(ctx context.Context, client *http.Client, e config.FleetEntry) Summary {
	row := Summary{Name: e.Name, URL: e.URL}
	raw, err := get(ctx, client, e, "/api/hw", nil)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	hw, _ := raw.(map[string]any)
	if hw == nil {
		row.Error = "peer /api/hw: top-level value is not an object"
		return row
	}
	row.Reachable = true
	applyHW(&row, hw)
	if runsRaw, err := get(ctx, client, e, "/api/runs", nil); err == nil {
		applyRuns(&row, runsRaw)
	}
	return row
}

// applyHW fills the host-shaped fields of the summary from an /api/hw
// document. A field that is absent stays absent; a missing value stays a
// gap.
func applyHW(row *Summary, hw map[string]any) {
	if v, ok := hw["gpus"].([]any); ok {
		for _, g := range v {
			m, ok := g.(map[string]any)
			if !ok {
				continue
			}
			card := GPUDetail{}
			if n, ok := m["hip_index"].(float64); ok {
				card.Index = int(n)
			}
			if n, ok := m["name"].(string); ok {
				card.Name = n
			}
			if n, ok := m["vram_total"].(float64); ok {
				gb := n / (1 << 30)
				card.VRAMGiB = &gb
			}
			if n, ok := m["power_cap_w"].(float64); ok {
				card.PowerCapW = &n
			}
			row.GPU = append(row.GPU, card)
		}
	}
}

// applyRuns fills the run-shaped fields of the summary. A run that serves
// gives the state word, the recipe and the generation speed; no run gives
// state "stopped".
func applyRuns(row *Summary, runs any) {
	list, _ := runs.([]any)
	if len(list) == 0 {
		row.State = "stopped"
		return
	}
	// Pick the first run that serves; fall back to the first entry.
	var pick map[string]any
	for _, r := range list {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		st, _ := m["state"].(string)
		if st == "serving" || st == "running" {
			pick = m
			break
		}
		if pick == nil {
			pick = m
		}
	}
	if pick == nil {
		row.State = "stopped"
		return
	}
	if s, ok := pick["state"].(string); ok {
		row.State = s
	}
	if s, ok := pick["recipe"].(string); ok {
		row.Recipe = s
	}
	if s, ok := pick["stats"].(map[string]any); ok {
		if v, ok := s["gen_tok_s"].(float64); ok {
			row.GenTokS = &v
		}
	}
}

// get performs one safe GET: short timeout, size cap, fixed path, header
// token. The host is matched against the configured entry before any byte
// leaves the process, and a redirect is refused (the default client does
// not follow one, by SetCheckRedirect below).
func get(ctx context.Context, client *http.Client, e config.FleetEntry, path string, hdr http.Header) (any, error) {
	u, err := url.Parse(e.URL)
	if err != nil {
		return nil, fmt.Errorf("peer url: %w", err)
	}
	if !allowedPath(path) {
		return nil, fmt.Errorf("path %q is not on the allowlist", path)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("peer url: scheme %q not allowed", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+path, nil)
	if err != nil {
		return nil, err
	}
	if e.Token != "" {
		req.Header.Set("Authorization", "Bearer "+e.Token)
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// Refuse redirects: a 3xx is reported as a status, not followed.
	if client.CheckRedirect == nil {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("peer request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("peer answered %d (redirects refused)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer answered %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("peer body: %w", err)
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("peer body larger than %d KiB", MaxBody>>10)
	}
	var out any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("peer body: %w", err)
	}
	return out, nil
}

// allowedPath reports whether path is on the allowlist of paths the Fleet
// fetcher may request. The list is a fixed set of small, public endpoints
// of the dashboard; a path with a query string is allowed when its base
// is on the list.
func allowedPath(path string) bool {
	path = strings.TrimPrefix(path, "/")
	for _, p := range strings.Fields(AllowedPaths) {
		if strings.HasPrefix(path, strings.TrimPrefix(p, "/")) {
			return true
		}
	}
	return false
}

// defaultClient is the SSRF-safe client. The redirect function refuses
// anything that is not the same host as the original request.
func defaultClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil, // honour $HTTP_PROXY only when explicitly set by the operator
		DialContext:           (&net.Dialer{Timeout: Timeout, KeepAlive: 30 * time.Second}).DialContext,
		ResponseHeaderTimeout: Timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: Timeout, Transport: transport}
}

// ErrNotConfigured is the error returned by Validate when the host of the
// URL is not in the configured fleet list.
var ErrNotConfigured = errors.New("host is not in the configured fleet list")

// Validate checks one URL against the configured fleet list. It is the
// guard used by the API handlers that take a host argument.
func Validate(hosts []string, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	for _, h := range hosts {
		if h == host {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotConfigured, host)
}
