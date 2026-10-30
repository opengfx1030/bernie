package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	listBody  = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	statusReq = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"status","arguments":{}}}`
	stopReq   = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"stop","arguments":{"name":"x"}}}`
)

// mcpPost sends one JSON-RPC message to /mcp with the given headers.
func mcpPost(t *testing.T, h http.Handler, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", target, strings.NewReader(body))
	r.Host = "127.0.0.1:7860"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range hdr {
		if k == "Host" {
			r.Host = v
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// toolNames reads the tool names out of a tools/list response (JSON or SSE).
func toolNames(t *testing.T, body string) []string {
	t.Helper()
	body = strings.TrimSpace(body)
	if i := strings.Index(body, "data: "); i >= 0 {
		body = strings.TrimSpace(strings.SplitN(body[i+6:], "\n", 2)[0])
	}
	var r struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("tools/list response: %v: %s", err, body)
	}
	var n []string
	for _, tl := range r.Result.Tools {
		n = append(n, tl.Name)
	}
	return n
}

func isolateState(t *testing.T) {
	t.Helper()
	d := t.TempDir()
	for _, k := range []string{"BERNIE_STATE_DIR", "BERNIE_CONFIG_DIR", "BERNIE_DATA_DIR", "BERNIE_CACHE_DIR"} {
		t.Setenv(k, d+"/"+strings.ToLower(k))
	}
}

func TestMCPLoopbackReadOnly(t *testing.T) {
	isolateState(t)
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()

	// A read call needs no X-Bernie header and no token on a tokenless loopback dashboard.
	w := mcpPost(t, h, "/mcp", listBody, nil)
	if w.Code != 200 {
		t.Fatalf("tools/list: %d %s", w.Code, w.Body.String())
	}
	names := toolNames(t, w.Body.String())
	if len(names) == 0 {
		t.Fatal("no tools listed")
	}
	for _, n := range names {
		if n == "serve" || n == "stop" || n == "bench_run" || n == "setup_apply" || n == "setup_verify" || n == "tune_power_set" {
			t.Errorf("read-only /mcp lists %s", n)
		}
	}
	if w := mcpPost(t, h, "/mcp", statusReq, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "structuredContent") {
		t.Errorf("tools/call status: %d %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-") {
		t.Errorf("Cache-Control = %q, want a no-store or no-cache value", cc)
	}
	// A mutating tool is refused without X-Bernie, and absent as a tool with it.
	if w := mcpPost(t, h, "/mcp", stopReq, nil); w.Code != http.StatusForbidden {
		t.Errorf("stop without X-Bernie: %d", w.Code)
	}
	w = mcpPost(t, h, "/mcp", stopReq, map[string]string{"X-Bernie": "1"})
	if strings.Contains(w.Body.String(), `"executed"`) || (w.Code == 200 && !strings.Contains(w.Body.String(), "nknown tool")) {
		t.Errorf("stop in read-only mode: %d %s", w.Code, w.Body.String())
	}
	// Only POST; no CORS; the cookie is not a credential.
	if w := do(t, h, "GET", "/mcp", map[string]string{"Host": "127.0.0.1:7860"}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp: %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("/mcp sets a CORS header")
	}
}

func TestMCPHostAndOrigin(t *testing.T) {
	isolateState(t)
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	for name, hdr := range map[string]map[string]string{
		"foreign host":                      {"Host": "evil.example:7860"},
		"foreign origin":                    {"Origin": "https://evil.example"},
		"null origin":                       {"Origin": "null"},
		"origin with a loopback look-alike": {"Origin": "http://127.0.0.1.evil.example"},
	} {
		if w := mcpPost(t, h, "/mcp", listBody, hdr); w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", name, w.Code)
		}
	}
	for _, o := range []string{"http://localhost:3000", "http://127.0.0.1:7860", "http://[::1]:1"} {
		if w := mcpPost(t, h, "/mcp", listBody, map[string]string{"Origin": o}); w.Code != 200 {
			t.Errorf("origin %s: got %d, want 200", o, w.Code)
		}
	}
}

func TestMCPToken(t *testing.T) {
	isolateState(t)
	h := newServer(Options{Listen: "127.0.0.1:7860", Token: testToken}).handler()

	w := mcpPost(t, h, "/mcp", listBody, nil)
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("no token: %d", w.Code)
	}
	if w := mcpPost(t, h, "/mcp", listBody, map[string]string{"Authorization": "Bearer nope"}); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", w.Code)
	}
	// The dashboard cookie does not authenticate /mcp.
	if w := mcpPost(t, h, "/mcp", listBody, map[string]string{"Cookie": cookieName + "=" + testToken}); w.Code != http.StatusUnauthorized {
		t.Errorf("cookie: %d", w.Code)
	}
	if w := mcpPost(t, h, "/mcp?token="+testToken, listBody, map[string]string{"Authorization": "Bearer " + testToken}); w.Code != http.StatusBadRequest {
		t.Errorf("token in the URL: %d", w.Code)
	}
	if w := mcpPost(t, h, "/mcp", listBody, map[string]string{"Authorization": "Bearer " + testToken}); w.Code != 200 {
		t.Errorf("good token: %d %s", w.Code, w.Body.String())
	}
	// The dashboard guard is unchanged: the same token and no X-Bernie still fails on /api POST.
	if w := do(t, h, "POST", "/api/runs", map[string]string{"Authorization": "Bearer " + testToken, "Host": "127.0.0.1:7860"}); w.Code != http.StatusForbidden {
		t.Errorf("POST /api/runs without X-Bernie: %d", w.Code)
	}
}

func TestMCPWriteNeedsAToken(t *testing.T) {
	isolateState(t)
	has := func(o Options) bool {
		w := mcpPost(t, newServer(o).handler(), "/mcp", listBody, map[string]string{"Authorization": "Bearer " + testToken})
		for _, n := range toolNames(t, w.Body.String()) {
			if n == "stop" {
				return true
			}
		}
		return false
	}
	if has(Options{Listen: "127.0.0.1:7860", MCPWrite: true}) {
		t.Error("write tools listed on a tokenless dashboard")
	}
	if !has(Options{Listen: "127.0.0.1:7860", MCPWrite: true, Token: testToken}) {
		t.Error("write tools missing with a token and MCPWrite")
	}
	if has(Options{Listen: "127.0.0.1:7860", Token: testToken}) {
		t.Error("write tools listed without MCPWrite")
	}
}

func TestMCPOffLoopbackIs404(t *testing.T) {
	isolateState(t)
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	w := mcpPost(t, h, "/mcp", listBody, map[string]string{"Authorization": "Bearer " + testToken})
	if w.Code != http.StatusNotFound {
		t.Errorf("/mcp on a non-loopback dashboard: %d", w.Code)
	}
}
