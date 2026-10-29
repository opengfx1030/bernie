package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/auth"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// chatRun writes a run record that is alive and points at the given address.
func chatRun(t *testing.T, name, host string, port int) {
	t.Helper()
	dir := runner.RunDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	engine := filepath.Join(t.TempDir(), "engine")
	if err := os.WriteFile(engine, []byte("#!/bin/sh\necho true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(runner.Record{
		Name: name, Mode: "container", Engine: engine, Container: "fake",
		Host: host, Port: port, StartedAt: time.Now(),
	})
	if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// chatEvents reads the events of a server-sent event stream.
func chatEvents(body string) map[string][]string {
	out := map[string][]string{}
	for _, block := range strings.Split(body, "\n\n") {
		name, data := "", ""
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		if name == "" {
			continue
		}
		out[name] = append(out[name], data)
	}
	return out
}

// TestChatGuards checks the answers of the guards of POST /api/chat.
func TestChatGuards(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		setup  func(t *testing.T, port int)
		status int
		says   string
	}{
		{
			name: "nothing serves", body: `{"prompt":"Hello"}`,
			status: http.StatusServiceUnavailable, says: "no server is running",
		},
		{
			name: "the prompt is empty", body: `{"prompt":"   "}`,
			status: http.StatusBadRequest, says: "the prompt is empty",
		},
		{
			name: "the prompt is too long", body: `{"prompt":"` + strings.Repeat("x", maxChatPromptBytes+1) + `"}`,
			status: http.StatusBadRequest, says: "longer than",
		},
		{
			name: "the body names a server", body: `{"prompt":"Hello","url":"http://example.com"}`,
			status: http.StatusBadRequest, says: "invalid JSON body",
		},
		{
			name: "the target is not on this host", body: `{"prompt":"Hello"}`,
			setup: func(t *testing.T, _ int) {
				chatRun(t, "flashnext", "192.168.1.5", 8000)
			},
			status: http.StatusBadRequest, says: "only talks to a server of this host",
		},
		{
			name: "the server does not answer", body: `{"prompt":"Hello"}`,
			setup: func(t *testing.T, port int) {
				chatRun(t, "flashnext", "127.0.0.1", port)
			},
			status: http.StatusBadGateway, says: "injected failure",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := vllmtest.New(vllmtest.Config{Model: "fake-model", FailEvery: 1})
			srv := httptest.NewServer(fake)
			defer srv.Close()
			port := srv.Listener.Addr().(*net.TCPAddr).Port

			h := smokeHandler(t)
			if c.setup != nil {
				c.setup(t, port)
			}
			w := post(t, h, "/api/chat", c.body)
			if w.Code != c.status {
				t.Fatalf("the answer is %d %s", w.Code, w.Body.String())
			}
			var doc struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
				t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
			}
			if !strings.Contains(doc.Error, c.says) {
				t.Fatalf("the error is %q, it does not say %q", doc.Error, c.says)
			}
		})
	}
}

// TestChatStreamsTheAnswer checks the stream of POST /api/chat: the text of
// the answer comes piece by piece, the reasoning comes apart from it and the
// last event carries the numbers.
func TestChatStreamsTheAnswer(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{
		Model: "fake-model", TTFT: 10 * time.Millisecond, Reasoning: true,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	h := smokeHandler(t)
	chatRun(t, "flashnext", "127.0.0.1", port)

	w := post(t, h, "/api/chat", `{"prompt":"How fast are you?"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("the answer is %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("the content type is %q", ct)
	}
	events := chatEvents(w.Body.String())
	if len(events["reasoning"]) != 1 {
		t.Fatalf("the stream has the events %v", events)
	}
	if len(events["delta"]) < 2 {
		t.Fatalf("the answer did not come piece by piece: %v", events["delta"])
	}
	var text strings.Builder
	for _, d := range events["delta"] {
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(d), &doc); err != nil {
			t.Fatalf("the event is not JSON: %v\n%s", err, d)
		}
		text.WriteString(doc.Text)
	}
	if !strings.Contains(text.String(), "fake server") {
		t.Fatalf("the answer is %q", text.String())
	}
	if len(events["done"]) != 1 {
		t.Fatalf("the stream has no done event: %v", events)
	}
	var done struct {
		TTFTMS       int     `json:"ttft_ms"`
		OutputTokens int     `json:"output_tokens"`
		TokS         float64 `json:"tok_s"`
	}
	if err := json.Unmarshal([]byte(events["done"][0]), &done); err != nil {
		t.Fatalf("the done event is not JSON: %v\n%s", err, events["done"][0])
	}
	if done.TTFTMS < 10 || done.OutputTokens < 1 || done.TokS <= 0 {
		t.Fatalf("the numbers are %+v", done)
	}
	if prompts := fake.ChatPrompts(); len(prompts) != 1 || prompts[0] != "How fast are you?" {
		t.Fatalf("the server got the prompts %v", prompts)
	}
}

// TestChatCarriesTheAnswerAsText checks that an answer with markup in it
// travels as text: the page writes it with textContent and never with
// innerHTML.
func TestChatCarriesTheAnswerAsText(t *testing.T) {
	hostile := "<script>alert('x')</script>"
	fake := vllmtest.New(vllmtest.Config{Model: "fake-model", ChatReply: hostile})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	h := smokeHandler(t)
	chatRun(t, "flashnext", "127.0.0.1", port)

	w := post(t, h, "/api/chat", `{"prompt":"Hello"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("the answer is %d %s", w.Code, w.Body.String())
	}
	events := chatEvents(w.Body.String())
	if len(events["delta"]) == 0 {
		t.Fatalf("the stream has no answer: %v", events)
	}
	var doc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(events["delta"][0]), &doc); err != nil {
		t.Fatalf("the event is not JSON: %v\n%s", err, events["delta"][0])
	}
	if strings.TrimSpace(doc.Text) != hostile {
		t.Fatalf("the answer is %q", doc.Text)
	}
}

// TestChatCancelsUpstreamWhenTheClientGoesAway checks that the request to
// the server ends when the page goes away.
func TestChatCancelsUpstreamWhenTheClientGoesAway(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list", "data": []map[string]any{{"id": "fake-model"}},
			})
		case "/v1/chat/completions":
			// The body is read so that the server watches the
			// connection and ends the request when the page goes away.
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
				close(cancelled)
			case <-time.After(10 * time.Second):
			}
		}
	}))
	defer upstream.Close()
	port := upstream.Listener.Addr().(*net.TCPAddr).Port

	h := smokeHandler(t)
	chatRun(t, "flashnext", "127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"Hello"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the request to the server was not cancelled")
	}
	<-done
}

// TestChatStreamEndsCleanly checks the end of the stream of POST /api/chat
// on the wire: the final event comes first, the chunked terminator follows
// it, and the reader that reads the body to the end sees no error. A body
// that ends without the terminator, or an event after it, is what a browser
// reports as net::ERR_ABORTED.
func TestChatStreamEndsCleanly(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{
		Model: "fake-model", TTFT: 5 * time.Millisecond,
	})
	upstream := httptest.NewServer(fake)
	defer upstream.Close()
	upPort := upstream.Listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	Register(mux, Options{Telemetry: fakeTelemetry{}})
	chatRun(t, "flashnext", "127.0.0.1", upPort)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// The request goes over a real socket, so the answer is the bytes on the
	// wire and not the recorder of a handler.
	addr := srv.Listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	body := `{"prompt":"Say one word."}`
	fmt.Fprintf(conn, "POST /api/chat HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Content-Type: application/json\r\n"+
		"X-Bernie: 1\r\n"+
		"Content-Length: %d\r\n"+
		"Connection: close\r\n"+
		"\r\n%s", addr, len(body), body)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("the answer was cut: %v", err)
	}
	resp := string(raw)

	if !strings.Contains(resp, "Transfer-Encoding: chunked") {
		t.Fatalf("the answer is not chunked-encoded:\n%s", resp)
	}
	// The final event comes before the terminator: the reader gets it.
	atDone := strings.Index(resp, "event: done")
	atEnd := strings.Index(resp, "0\r\n\r\n")
	if atDone < 0 {
		t.Fatalf("the stream has no done event:\n%s", resp)
	}
	if atEnd < 0 {
		t.Fatalf("the stream has no chunked terminator:\n%s", resp)
	}
	if atDone > atEnd {
		t.Errorf("the done event comes after the terminator (%d > %d)", atDone, atEnd)
	}
	// Nothing comes after the terminator: the body ends there.
	if tail := resp[atEnd+len("0\r\n\r\n"):]; strings.TrimSpace(tail) != "" {
		t.Errorf("the body continues after the terminator: %q", tail)
	}
	// The reader of the browser sees the whole body and no error.
	client := &http.Client{}
	req, err := http.NewRequest("POST", srv.URL+"/api/chat", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bernie", "1")
	answer, err := client.Do(req)
	if err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("the answer is %d", answer.StatusCode)
	}
	read, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatalf("the body was cut: %v", err)
	}
	if !strings.Contains(string(read), "event: done") {
		t.Errorf("the body has no done event:\n%s", read)
	}
}

// TestChatNeedsTheToken checks that the guard of spec §11 holds for the
// chat endpoint: without the token the answer is 401.
func TestChatNeedsTheToken(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{})
	guarded := auth.New("tok", mux)

	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"Hello"}`))
	r.Header.Set("X-Bernie", "1")
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("the answer is %d %s", w.Code, w.Body.String())
	}
}

// TestChatRefusesASecondAnswer checks that one answer runs at a time: the
// second request waits for nothing, it is refused with 409.
func TestChatRefusesASecondAnswer(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list", "data": []map[string]any{{"id": "fake-model"}},
			})
		case "/v1/chat/completions":
			close(started)
			<-release
		}
	}))
	defer upstream.Close()
	port := upstream.Listener.Addr().(*net.TCPAddr).Port

	h := smokeHandler(t)
	chatRun(t, "flashnext", "127.0.0.1", port)

	done := make(chan struct{})
	go func() {
		defer close(done)
		r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"Hello"}`))
		h.ServeHTTP(httptest.NewRecorder(), r)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first answer did not reach the server")
	}
	w := post(t, h, "/api/chat", `{"prompt":"Hello"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("the answer is %d %s", w.Code, w.Body.String())
	}
	close(release)
	<-done
}
