package dashboard

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sseHeartbeat keeps proxies from closing idle streams and surfaces client
// disconnects as write errors.
const sseHeartbeat = 15 * time.Second

// sseStream writes Server-Sent Events. It is safe for concurrent use.
type sseStream struct {
	mu sync.Mutex
	w  http.ResponseWriter
	rc *http.ResponseController
}

// newSSE sends the stream headers. The caller owns the request lifetime.
func newSSE(w http.ResponseWriter) *sseStream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &sseStream{w: w, rc: http.NewResponseController(w)}
	s.mu.Lock()
	_ = s.rc.Flush()
	s.mu.Unlock()
	return s
}

// event writes one event. data must not contain newlines; callers split.
// An empty name sends the default "message" event.
func (s *sseStream) event(name, data string) error {
	var b bytes.Buffer
	b.Grow(len(name) + len(data) + 16)
	if name != "" {
		b.WriteString("event: ")
		b.WriteString(name)
		b.WriteByte('\n')
	}
	b.WriteString("data: ")
	b.WriteString(data)
	b.WriteString("\n\n")
	return s.write(b.Bytes())
}

func (s *sseStream) comment(text string) error {
	return s.write([]byte(": " + text + "\n\n"))
}

func (s *sseStream) write(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(p); err != nil {
		return err
	}
	return s.rc.Flush()
}

// lineEvents is an io.Writer that turns a byte stream (a run log) into one
// SSE event per line. Carriage returns (progress bars) also end a line.
type lineEvents struct {
	s   *sseStream
	buf []byte
}

func (l *lineEvents) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexAny(l.buf, "\r\n")
		if i < 0 {
			break
		}
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		if line == "" && i == 0 {
			continue // \r\n pair or blank line from a progress bar redraw
		}
		if err := l.s.event("", line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// flush emits a trailing partial line.
func (l *lineEvents) flush() error {
	if len(l.buf) == 0 {
		return nil
	}
	line := strings.TrimRight(string(l.buf), "\r\n")
	l.buf = l.buf[:0]
	return l.s.event("", line)
}
