// Package sse writes the dashboard's server-sent event streams.
package sse

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// heartbeat keeps proxies from closing idle streams and surfaces client
// disconnects as write errors.
const heartbeat = 15 * time.Second

// Interval is the delay between heartbeat comments.
const Interval = heartbeat

// Stream writes Server-Sent Events. It is safe for concurrent use.
type Stream struct {
	mu sync.Mutex
	w  http.ResponseWriter
	rc *http.ResponseController
}

// New sends the stream headers. The caller owns the request lifetime.
func New(w http.ResponseWriter) *Stream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &Stream{w: w, rc: http.NewResponseController(w)}
	s.mu.Lock()
	_ = s.rc.Flush()
	s.mu.Unlock()
	return s
}

// Event writes one event. data must not contain newlines; callers split.
// An empty name sends the default "message" event.
func (s *Stream) Event(name, data string) error {
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

// EventID writes one event with an id. The id lets a client that
// re-attaches ask for the events after the last one it saw: it sends the id
// back in the Last-Event-ID header.
func (s *Stream) EventID(id, name, data string) error {
	var b bytes.Buffer
	b.Grow(len(id) + len(name) + len(data) + 24)
	b.WriteString("id: ")
	b.WriteString(id)
	b.WriteByte('\n')
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

// Comment writes an SSE comment.
func (s *Stream) Comment(text string) error {
	return s.write([]byte(": " + text + "\n\n"))
}

func (s *Stream) write(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(p); err != nil {
		return err
	}
	return s.rc.Flush()
}

// LineWriter turns a byte stream (a run log) into one SSE event per line.
// Carriage returns (progress bars) also end a line.
type LineWriter struct {
	Stream *Stream
	buf    []byte
}

func (l *LineWriter) Write(p []byte) (int, error) {
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
		if err := l.Stream.Event("", line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush emits a trailing partial line.
func (l *LineWriter) Flush() error {
	if len(l.buf) == 0 {
		return nil
	}
	line := strings.TrimRight(string(l.buf), "\r\n")
	l.buf = l.buf[:0]
	return l.Stream.Event("", line)
}

// Heartbeat sends an SSE comment periodically. A failed write means the
// client is gone, so it cancels the stream.
func Heartbeat(ctx context.Context, stream *Stream, cancel context.CancelFunc) {
	t := time.NewTicker(heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if stream.Comment("ping") != nil {
				cancel()
				return
			}
		}
	}
}

// OneLine replaces line breaks so a value fits in one SSE event.
func OneLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
}
