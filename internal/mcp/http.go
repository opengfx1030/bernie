package mcp

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxBody bounds a JSON-RPC request body.
const maxBody = 1 << 20

// Handler serves Streamable HTTP, stateless: every POST is one request. It
// applies the guard of spec §13.10: Host and Origin must be loopback, the
// bearer token is required when token is not empty, only POST is served, and
// a call of a mutating tool needs `X-Bernie: 1`. Cookies never authenticate.
// The caller mounts it on /mcp and serves it on a loopback address only.
func (s *Server) Handler(token string) http.Handler {
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.srv }, &sdk.StreamableHTTPOptions{Stateless: true})
	return guard{token: sha256.Sum256([]byte(token)), auth: token != "", next: h}
}

type guard struct {
	token [32]byte
	auth  bool
	next  http.Handler
}

func (g guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Has("token") {
		deny(w, http.StatusBadRequest, "the token is not accepted in the URL: send Authorization: Bearer <token>")
		return
	}
	if !loopbackHost(r.Host) {
		deny(w, http.StatusForbidden, "host header is not a loopback name")
		return
	}
	if o := r.Header.Get("Origin"); o != "" && !loopbackOrigin(o) {
		deny(w, http.StatusForbidden, "origin is not a loopback origin")
		return
	}
	if g.auth && !g.bearer(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="bernie"`)
		deny(w, http.StatusUnauthorized, "token required: send Authorization: Bearer <token>")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		deny(w, http.StatusMethodNotAllowed, "only POST is served")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		deny(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if callsMutatingTool(body) && r.Header.Get("X-Bernie") != "1" {
		deny(w, http.StatusForbidden, "a mutating tool needs the header X-Bernie: 1")
		return
	}
	g.next.ServeHTTP(w, r)
}

// bearer compares in constant time; hashing first hides the token length.
func (g guard) bearer(r *http.Request) bool {
	const p = "Bearer "
	a := r.Header.Get("Authorization")
	if len(a) <= len(p) || !strings.EqualFold(a[:len(p)], p) {
		return false
	}
	s := sha256.Sum256([]byte(a[len(p):]))
	return subtle.ConstantTimeCompare(s[:], g.token[:]) == 1
}

// callsMutatingTool reports whether a JSON-RPC body (one message or a batch)
// contains a tools/call of a mutating tool. A body that does not parse is
// left to the SDK, which rejects it.
func callsMutatingTool(body []byte) bool {
	var msgs []struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	b := bytes.TrimSpace(body)
	if len(b) > 0 && b[0] != '[' {
		b = append(append([]byte{'['}, b...), ']')
	}
	if json.Unmarshal(b, &msgs) != nil {
		return false
	}
	for _, m := range msgs {
		if m.Method == "tools/call" && IsMutating(m.Params.Name) {
			return true
		}
	}
	return false
}

func loopbackHost(hostport string) bool {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	return loopbackName(h)
}

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && loopbackName(u.Hostname())
}

func loopbackName(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// deny writes the CLI's --json failure shape, like the dashboard.
func deny(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg, "exit_code": 1})
}
