package dashsvc

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// This file owns the tests of the service: the states of GetStatus, the
// stop, the URLs and the token resolution. The state on disk is tested
// in dashsvc_state_test.go.

// TestGetStatusStopped verifies the "not running" path of GetStatus.
func TestGetStatusStopped(t *testing.T) {
	withStateDir(t, t.TempDir())
	st, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.State != "stopped" {
		t.Errorf("State = %q, want stopped", st.State)
	}
	if st.PID != 0 {
		t.Errorf("PID = %d, want 0", st.PID)
	}
}

// TestGetStatusStale writes a pidfile for a dead PID and expects the
// status to come back as "stale".
func TestGetStatusStale(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	// Pick a PID that almost certainly does not exist.
	if err := WritePid(p, 999_999_999); err != nil {
		t.Fatalf("WritePid: %v", err)
	}
	st, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.State != "stale" {
		t.Errorf("State = %q, want stale", st.State)
	}
	if st.PID != 999_999_999 {
		t.Errorf("PID = %d, want 999_999_999", st.PID)
	}
}

// TestGetStatusRunningNeedsPID writes a startup document, leaves the
// pidfile empty and expects "stopped" — without a live PID, GetStatus
// cannot prove a dashboard is running.
func TestGetStatusRunningNeedsPID(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WriteStartup(p, StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/",
		PID:       os.Getpid(),
		StartedAt: nowFunc(),
		Version:   "v0.0.0-test",
		Manager:   "process",
	}); err != nil {
		t.Fatalf("WriteStartup: %v", err)
	}
	st, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.State == "running" {
		t.Errorf("State = running without a pidfile; want stopped or stale")
	}
}

// TestGetStatusRunningLivePID writes a startup document and a live PID
// (this process), then expects "running" with the listen URL.
func TestGetStatusRunningLivePID(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WriteStartup(p, StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/",
		PID:       os.Getpid(),
		StartedAt: nowFunc(),
		Version:   "v0.0.0-test",
		Manager:   "process",
	}); err != nil {
		t.Fatalf("WriteStartup: %v", err)
	}
	if err := WritePid(p, os.Getpid()); err != nil {
		t.Fatalf("WritePid: %v", err)
	}
	t.Cleanup(func() { _ = RemovePid(p) })
	st, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.State != "running" {
		t.Errorf("State = %q, want running", st.State)
	}
	if st.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", st.PID, os.Getpid())
	}
	if st.LoopbackURL != "http://127.0.0.1:7860/" {
		t.Errorf("LoopbackURL = %q, want http://127.0.0.1:7860/", st.LoopbackURL)
	}
}

// TestStopNotRunning verifies Stop without a pidfile returns
// ErrNotRunning.
func TestStopNotRunning(t *testing.T) {
	withStateDir(t, t.TempDir())
	err := Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("Stop without pidfile err = %v, want ErrNotRunning", err)
	}
}

// TestStopStalePidfile verifies Stop clears a stale pidfile.
func TestStopStalePidfile(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WritePid(p, 999_999_999); err != nil {
		t.Fatalf("WritePid: %v", err)
	}
	if err := Stop(context.Background()); err != nil {
		t.Errorf("Stop stale: %v", err)
	}
	if _, err := ReadPid(p); !os.IsNotExist(err) {
		t.Errorf("stale pidfile not removed: %v", err)
	}
}

// TestLoopbackURL verifies the loopback URL rendering.
func TestLoopbackURL(t *testing.T) {
	cases := []struct {
		listen string
		want   string
	}{
		{"127.0.0.1:7860", "http://127.0.0.1:7860/"},
		{"0.0.0.0:7860", "http://127.0.0.1:7860/"},
		{"[::]:7860", "http://127.0.0.1:7860/"},
		{":7860", "http://127.0.0.1:7860/"},
		{"192.168.1.5:7860", "http://192.168.1.5:7860/"},
		{"broken", ""},
	}
	for _, c := range cases {
		if got := loopbackURL(c.listen); got != c.want {
			t.Errorf("loopbackURL(%q) = %q, want %q", c.listen, got, c.want)
		}
	}
}

// TestLANURLsNonLoopbackOnly verifies lanURLs returns nothing when the
// listener is already bound to one address.
func TestLANURLsNonLoopbackOnly(t *testing.T) {
	if urls := lanURLs("192.168.1.5:7860"); len(urls) != 0 {
		t.Errorf("lanURLs(192.168.1.5:7860) = %v, want empty", urls)
	}
}

// TestLANURLsSkipsLoopback verifies the interface list is filtered.
func TestLANURLsSkipsLoopback(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}
	if len(ifaces) == 0 {
		t.Skip("no interfaces")
	}
	urls := lanURLs("0.0.0.0:7860")
	for _, u := range urls {
		if strings.Contains(u, "127.") {
			t.Errorf("lanURLs returned loopback URL: %s", u)
		}
	}
}

// TestSystemdUnitRender verifies the unit body carries the expected
// fields and points at the executable.
func TestSystemdUnitRender(t *testing.T) {
	body := renderSystemdUnit("/usr/local/bin/bernie", "0.0.0.0:7860", "tok")
	for _, want := range []string{
		"[Unit]", "[Service]", "[Install]",
		"ExecStart=/usr/local/bin/bernie dash --child --listen 0.0.0.0:7860 --token tok",
		"Restart=on-failure",
		"Type=simple",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("unit body missing %q\n---%s---", want, body)
		}
	}
}

// TestResolveTokenKeepsPersisted verifies resolveToken returns the
// persisted token when no override is given.
func TestResolveTokenKeepsPersisted(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	got, err := resolveToken(p, StartOptions{})
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != tok1 {
		t.Errorf("resolveToken = %q, want %q", got, tok1)
	}
}

// TestResolveTokenRotates verifies --rotate-token replaces the value.
func TestResolveTokenRotates(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	got, err := resolveToken(p, StartOptions{RotateToken: true})
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got == tok1 {
		t.Errorf("resolveToken with rotate = %q, want new value", got)
	}
}

// TestResolveTokenOverrides verifies an explicit --token wins and is
// persisted.
func TestResolveTokenOverrides(t *testing.T) {
	withStateDir(t, t.TempDir())
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	override := "0123456789abcdef0123456789abcdef"
	got, err := resolveToken(p, StartOptions{Token: override})
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != override {
		t.Errorf("resolveToken = %q, want %q", got, override)
	}
	// Persisted too.
	cur, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	if cur != override {
		t.Errorf("persisted token = %q, want %q", cur, override)
	}
}

// nowFunc is the clock the tests use.
func nowFunc() time.Time { return time.Now() }
