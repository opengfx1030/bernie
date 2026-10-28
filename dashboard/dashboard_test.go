package dashboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "s3cret-token"

func do(t *testing.T, h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		if k == "Host" {
			r.Host = v
			continue
		}
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:7860": true, "localhost:7860": true, "[::1]:7860": true, "127.0.0.2:1": true,
		":7860": false, "0.0.0.0:7860": false, "[::]:7860": false, "192.168.1.5:7860": false, "example.com:80": false,
	} {
		if got := IsLoopback(addr); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestLoopbackMode(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()

	if w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"}); w.Code != 200 || !strings.Contains(w.Body.String(), "<title>bernie</title>") {
		t.Fatalf("GET / on loopback: %d %q", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/app.js", map[string]string{"Host": "localhost:7860"}); w.Code != 200 {
		t.Fatalf("GET /app.js: %d", w.Code)
	}
	// DNS rebinding: a foreign Host header never reaches the API without a token.
	if w := do(t, h, "GET", "/api/hw", map[string]string{"Host": "evil.example:7860"}); w.Code != http.StatusForbidden {
		t.Fatalf("foreign Host: got %d, want 403", w.Code)
	}
	// Mutations need X-Bernie: 1.
	for _, m := range []string{"POST /api/runs", "DELETE /api/runs/x", "POST /api/bench"} {
		method, path, _ := strings.Cut(m, " ")
		if w := do(t, h, method, path, map[string]string{"Host": "127.0.0.1:7860"}); w.Code != http.StatusForbidden {
			t.Errorf("%s without X-Bernie: got %d, want 403", m, w.Code)
		}
	}
	// With the header the request reaches the handler (which rejects the empty body).
	w := do(t, h, "POST", "/api/runs", map[string]string{"Host": "127.0.0.1:7860", "X-Bernie": "1"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "recipe is required") {
		t.Fatalf("POST /api/runs with header: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/api/nope", map[string]string{"Host": "127.0.0.1:7860"}); w.Code != 404 {
		t.Fatalf("unknown endpoint: %d", w.Code)
	}
}

func TestTokenMode(t *testing.T) {
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	host := map[string]string{"Host": "gpu-box:7860"}

	w := do(t, h, "GET", "/api/runs", host)
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("no token: %d, WWW-Authenticate %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == nil {
		t.Fatalf("401 body is not the error shape: %s", w.Body.String())
	}
	for _, auth := range []string{"Bearer wrong", "Bearer " + testToken + "x", "Basic " + testToken, "Bearer "} {
		if w := do(t, h, "GET", "/", map[string]string{"Host": "gpu-box", "Authorization": auth}); w.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: got %d, want 401", auth, w.Code)
		}
	}
	if w := do(t, h, "GET", "/", map[string]string{"Host": "gpu-box", "Authorization": "Bearer " + testToken}); w.Code != 200 {
		t.Fatalf("valid bearer: %d", w.Code)
	}
	if w := do(t, h, "GET", "/", map[string]string{"Host": "gpu-box", "Authorization": "bearer " + testToken}); w.Code != 200 {
		t.Fatalf("scheme is case-insensitive: %d", w.Code)
	}
	// A valid token does not lift the header requirement.
	if w := do(t, h, "POST", "/api/runs", map[string]string{"Host": "gpu-box", "Authorization": "Bearer " + testToken}); w.Code != http.StatusForbidden {
		t.Fatalf("POST with token, no X-Bernie: %d", w.Code)
	}
	w = do(t, h, "POST", "/api/runs", map[string]string{"Host": "gpu-box", "Authorization": "Bearer " + testToken, "X-Bernie": "1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST with token and header should reach the handler: %d %s", w.Code, w.Body.String())
	}
	// The header alone is not a credential.
	if w := do(t, h, "POST", "/api/runs", map[string]string{"Host": "gpu-box", "X-Bernie": "1"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("POST with header, no token: %d", w.Code)
	}
}

func TestTokenCookieFlow(t *testing.T) {
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()

	if w := do(t, h, "GET", "/?token=nope", map[string]string{"Host": "gpu-box"}); w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("bad query token: %d, cookies %v", w.Code, w.Result().Cookies())
	}

	w := do(t, h, "GET", "/?tab=1&token="+testToken, map[string]string{"Host": "gpu-box"})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("query token: got %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("redirect must go to / without any query: Location %q", loc)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("want one cookie, got %v", cs)
	}
	c := cs[0]
	if c.Name != cookieName || c.Value != testToken || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie attributes: %+v", c)
	}

	r := httptest.NewRequest("GET", "/app.css", nil)
	r.Host = "gpu-box"
	r.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("request with cookie: %d", rec.Code)
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "forged"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie: %d", rec.Code)
	}
}

func TestServeGeneratesTokenOffLoopback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	urls := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(ctx, Options{Listen: "0.0.0.0:0", Ready: func(u string) { urls <- u }})
	}()
	select {
	case u := <-urls:
		if !strings.Contains(u, "/?token=") || len(u[strings.Index(u, "token=")+6:]) != 32 {
			t.Fatalf("non-loopback URL must carry a generated token: %q", u)
		}
	case err := <-errc:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not become ready")
	}
	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Serve after cancel: %v", err)
	}
}

func TestLineEventsFraming(t *testing.T) {
	rec := httptest.NewRecorder()
	st := newSSE(rec)
	lw := &lineEvents{s: st}
	for _, chunk := range []string{"first li", "ne\nsecond\r\n", "bar 10%\rbar 20%\r", "\ntail"} {
		if _, err := lw.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := lw.flush(); err != nil {
		t.Fatal(err)
	}
	if err := st.event("end", ""); err != nil {
		t.Fatal(err)
	}
	want := "data: first line\n\ndata: second\n\ndata: bar 10%\n\ndata: bar 20%\n\ndata: tail\n\nevent: end\ndata: \n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("framing:\n got %q\nwant %q", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
}

// TestTelemetryStream checks the SSE framing of /api/telemetry and that the
// shared sampler stops when the last subscriber leaves.
func TestTelemetryStream(t *testing.T) {
	s := newServer(Options{Listen: "127.0.0.1:0"})
	n := 0
	s.tele = newHub(5*time.Millisecond, func(context.Context) []byte {
		n++
		b, _ := json.Marshal(map[string]int{"seq": n})
		return b
	})
	ts := httptest.NewServer(s.handler())
	defer ts.Close()

	read := func() (*http.Response, *bufio.Reader, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/telemetry", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("telemetry: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		return resp, bufio.NewReader(resp.Body), cancel
	}
	event := func(br *bufio.Reader) map[string]int {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("event line %q", line)
		}
		blank, _ := br.ReadString('\n')
		if blank != "\n" {
			t.Fatalf("event must end with a blank line, got %q", blank)
		}
		var v map[string]int
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	r1, b1, c1 := read()
	r2, b2, c2 := read()
	a, b := event(b1), event(b2)
	if a["seq"] == 0 || b["seq"] == 0 {
		t.Fatalf("events %v %v", a, b)
	}
	if next := event(b1); next["seq"] <= a["seq"] {
		t.Fatalf("sequence must advance: %v then %v", a, next)
	}
	if !s.tele.running() {
		t.Fatal("sampler must run while subscribed")
	}
	c1()
	r1.Body.Close()
	c2()
	r2.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for s.tele.running() {
		if time.Now().After(deadline) {
			t.Fatal("sampler still running after the last subscriber left")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRedact(t *testing.T) {
	plan := map[string]any{
		"name": "x",
		"env": map[string]string{"HF_TOKEN": "hf_abc", "OPENAI_API_KEY": "sk-1", "VLLM_RDNA_AR": "1", "MAX_NUM_BATCHED_TOKENS": "8192",
			"TOKENIZERS_PARALLELISM": "false"},
		"exec": map[string]any{
			"argv": []string{"podman", "run", "-e", "HF_TOKEN=hf_abc", "-e", "DB_PASSWORD=pw", "-e", "VLLM_RDNA_AR=1",
				"img", "serve", "m", "--api-key", "sk-2", "--max-num-batched-tokens", "8192", "--hf-token=hf_x", "--enable-prefix-caching", "--port", "8000",
				"--long-prefill-token-threshold", "512", "--ssl-keyfile", "/k.pem"},
			"env": []string{"AWS_SECRET_ACCESS_KEY=a", "PATH=/usr/bin"},
		},
	}
	g, err := redactJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(g)
	out := string(b)
	for _, leak := range []string{"hf_abc", "sk-1", "sk-2", "hf_x", `"pw"`, "=pw", "AWS_SECRET_ACCESS_KEY=a"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked: %s", leak, out)
		}
	}
	for _, keep := range []string{`"VLLM_RDNA_AR":"1"`, `"MAX_NUM_BATCHED_TOKENS":"8192"`, `"TOKENIZERS_PARALLELISM":"false"`, `"--max-num-batched-tokens","8192"`,
		`"--enable-prefix-caching","--port","8000"`, `"PATH=/usr/bin"`, `"VLLM_RDNA_AR=1"`, `"--long-prefill-token-threshold","512"`, `"--ssl-keyfile","/k.pem"`} {
		if !strings.Contains(out, keep) {
			t.Errorf("non-secret %q was changed: %s", keep, out)
		}
	}
}

func TestParseConcurrency(t *testing.T) {
	for in, want := range map[string]string{"": "[1]", "null": "[1]", "[1,8]": "[1 8]", `"1, 4,16"`: "[1 4 16]", "[]": "[1]"} {
		got, err := parseConcurrency(json.RawMessage(in))
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if s := strings.Join(strings.Fields(strings.Trim(strings.ReplaceAll(fmtInts(got), ",", " "), "[]")), " "); "["+s+"]" != want {
			t.Errorf("%s: got %v, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"[0]", `"1,x"`, "{}", "[-2]"} {
		if _, err := parseConcurrency(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func fmtInts(v []int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Every response, including errors, redirects and the HTML, is uncacheable.
func TestNoStoreEverywhere(t *testing.T) {
	tok := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	loop := newServer(Options{}).handler()
	bearer := map[string]string{"Host": "gpu-box", "Authorization": "Bearer " + testToken}
	for _, tc := range []struct {
		name string
		h    http.Handler
		path string
		hdr  map[string]string
		code int
	}{
		{"html", tok, "/", bearer, 200},
		{"asset", tok, "/app.js", bearer, 200},
		{"api json", tok, "/api/runs", bearer, 200},
		{"api 404", tok, "/api/nope", bearer, 404},
		{"api 401", tok, "/api/runs", map[string]string{"Host": "gpu-box"}, 401},
		{"bootstrap redirect", tok, "/?token=" + testToken, map[string]string{"Host": "gpu-box"}, 303},
		{"loopback html", loop, "/", map[string]string{"Host": "localhost"}, 200},
		{"loopback api", loop, "/api/runs", map[string]string{"Host": "localhost"}, 200},
		{"rebinding 403", loop, "/", map[string]string{"Host": "evil.example"}, 403},
	} {
		w := do(t, tc.h, "GET", tc.path, tc.hdr)
		if w.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.name, w.Code, tc.code)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s: Cache-Control %q, want no-store", tc.name, cc)
		}
	}
}

// The token travels in the URL only to bootstrap the cookie on "/".
func TestTokenRefusedInAPIQuery(t *testing.T) {
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	host := map[string]string{"Host": "gpu-box"}
	for _, target := range []string{"/api/runs?token=" + testToken, "/api/hw?x=1&token=" + testToken, "/api/runs?token=wrong"} {
		w := do(t, h, "GET", target, host)
		if w.Code != http.StatusBadRequest || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: %d, cookies %v; want 400 and no cookie", target, w.Code, w.Result().Cookies())
		}
		if strings.Contains(w.Body.String(), testToken) {
			t.Errorf("%s: response echoes the token: %s", target, w.Body.String())
		}
	}
	// A valid bearer does not make a URL token acceptable either.
	w := do(t, h, "GET", "/api/runs?token="+testToken, map[string]string{"Host": "gpu-box", "Authorization": "Bearer " + testToken})
	if w.Code != http.StatusBadRequest {
		t.Errorf("bearer + query token: %d, want 400", w.Code)
	}
	// Off "/" a query token is no credential and sets no cookie.
	w = do(t, h, "GET", "/app.js?token="+testToken, host)
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Errorf("/app.js?token: %d, cookies %v; want 401 and no cookie", w.Code, w.Result().Cookies())
	}
	// Loopback mode has no token, but still refuses one in /api URLs.
	if w := do(t, newServer(Options{}).handler(), "GET", "/api/runs?token=x", map[string]string{"Host": "localhost"}); w.Code != http.StatusBadRequest {
		t.Errorf("loopback /api?token: %d, want 400", w.Code)
	}
}

// The server writes no per-request lines: a URL's query string (the token)
// never reaches Options.Log.
func TestRequestsNeverLogQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var log bytes.Buffer
	urls := make(chan string, 1)
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(ctx, Options{Listen: "127.0.0.1:0", Token: testToken, Log: &log, Ready: func(u string) { urls <- u }})
	}()
	var base string
	select {
	case u := <-urls:
		base = u[:strings.Index(u, "?")]
	case err := <-errc:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not become ready")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, q := range []string{"?token=" + testToken, "?token=wrong&a=b", "api/runs?token=" + testToken} {
		resp, err := client.Get(base + q)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	cancel()
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.String(), "token") || strings.Contains(log.String(), "wrong") {
		t.Errorf("log leaks a query string:\n%s", log.String())
	}
}
