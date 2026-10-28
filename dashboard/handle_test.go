package dashboard

import (
	"context"
	"net"
	"strings"
	"testing"
)

// Start serves the dashboard in the background: the URL comes back and the
// server stops with the context.
func TestStartServesInTheBackground(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := ""
	h, err := Start(ctx, Options{Listen: "127.0.0.1:0", Ready: func(url string) { ready = url }})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ready, "http://127.0.0.1:") {
		t.Errorf("ready = %q", ready)
	}
	if h.URL() != ready {
		t.Errorf("url = %q, want %q", h.URL(), ready)
	}
	cancel()
	if err := h.Wait(); err != nil {
		t.Errorf("wait: %v", err)
	}
}

// A busy port is an error: no dashboard runs then.
func TestStartFailsOnABusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	h, err := Start(context.Background(), Options{Listen: ln.Addr().String()})
	if err == nil {
		t.Fatal("want a bind error")
	}
	if h != nil {
		t.Error("no dashboard must run")
	}
	if !strings.Contains(err.Error(), "bind") {
		t.Errorf("err = %v", err)
	}
}

// A starter that never started waits without an error.
func TestStarterWaitWithoutStart(t *testing.T) {
	s := NewStarter(Options{Listen: "127.0.0.1:0"})
	if err := s.Wait(); err != nil {
		t.Errorf("wait: %v", err)
	}
}
