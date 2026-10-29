package dashsvc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// withStateDir redirects the XDG state directory for a single test.
// We restore the previous value at the end so other tests stay isolated.
// The cleanup runs even when the test fails; t.Cleanup is the standard
// hook for that.
func withStateDir(t *testing.T, dir string) {
	t.Helper()
	prev := os.Getenv("BERNIE_STATE_DIR")
	t.Setenv("BERNIE_STATE_DIR", dir)
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("BERNIE_STATE_DIR")
		} else {
			_ = os.Setenv("BERNIE_STATE_DIR", prev)
		}
	})
}

// TestServicePathsAreUnderState verifies the XDG rule of spec §12: no
// file lands under /tmp. The state dir is the override or the default.
func TestServicePathsAreUnderState(t *testing.T) {
	withStateDir(t, t.TempDir())
	p := ServicePaths()
	if !strings.HasPrefix(p.State, os.Getenv("BERNIE_STATE_DIR")) {
		t.Errorf("state dir %q not under BERNIE_STATE_DIR", p.State)
	}
	if !strings.HasPrefix(p.Token, p.State) {
		t.Errorf("token %q not under state dir %q", p.Token, p.State)
	}
	if !strings.HasPrefix(p.Pid, p.State) {
		t.Errorf("pid %q not under state dir %q", p.Pid, p.State)
	}
	if !strings.HasPrefix(p.Log, p.State) {
		t.Errorf("log %q not under state dir %q", p.Log, p.State)
	}
	if !strings.HasPrefix(p.Startup, p.State) {
		t.Errorf("startup %q not under state dir %q", p.Startup, p.State)
	}
	// Default state dir is under HOME, never /tmp.
	def := ServicePaths()
	if strings.Contains(def.State, "/tmp/") {
		t.Errorf("default state dir %q must not be under /tmp", def.State)
	}
	// Also: paths.State() must agree (sanity).
	if got, want := def.State, filepath.Join(paths.State(), DirName); got != want {
		t.Errorf("paths.State()=%q want %q", got, want)
	}
}

// TestEnsureDirCreatesDir0700 verifies the state dir is created with a
// mode that does not let other users read the token file.
func TestEnsureDirCreatesDir0700(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(p.State)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("state dir mode = %o, want 0700", perm)
	}
}

// TestReadTokenPersistsAndReuses verifies the persistent token file:
// missing → generate, present → reuse the same value, mode is 0600.
func TestReadTokenPersistsAndReuses(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("first ReadToken: %v", err)
	}
	if tok1 == "" {
		t.Fatalf("first ReadToken returned empty")
	}
	info, err := os.Stat(ServicePaths().Token)
	if err != nil {
		t.Fatalf("stat token: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 0600", perm)
	}
	tok2, _, err := ReadToken()
	if err != nil {
		t.Fatalf("second ReadToken: %v", err)
	}
	if tok1 != tok2 {
		t.Errorf("token not persisted: %q vs %q", tok1, tok2)
	}
}

// TestWriteTokenRotates verifies WriteToken replaces the persisted value.
func TestWriteTokenRotates(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	tok2 := "0123456789abcdef0123456789abcdef"
	if _, err := WriteToken(tok2); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}
	got, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken after rotate: %v", err)
	}
	if got != tok2 {
		t.Errorf("after rotate got %q, want %q", got, tok2)
	}
	if tok1 == tok2 {
		t.Errorf("rotate did not change token")
	}
}

// TestIsAliveReportsDeadPID verifies the liveness check.
func TestIsAliveReportsDeadPID(t *testing.T) {
	if IsAlive(0) {
		t.Errorf("IsAlive(0) = true, want false")
	}
	if IsAlive(-1) {
		t.Errorf("IsAlive(-1) = true, want false")
	}
	// Pick a PID that almost certainly does not exist: high int.
	if IsAlive(999_999_999) {
		t.Errorf("IsAlive(high) = true, want false")
	}
}

// TestPidRoundTrip verifies the PID file helpers.
func TestPidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WritePid(p, 12345); err != nil {
		t.Fatalf("WritePid: %v", err)
	}
	got, err := ReadPid(p)
	if err != nil {
		t.Fatalf("ReadPid: %v", err)
	}
	if got != 12345 {
		t.Errorf("ReadPid = %d, want 12345", got)
	}
	if err := RemovePid(p); err != nil {
		t.Fatalf("RemovePid: %v", err)
	}
	if _, err := ReadPid(p); !os.IsNotExist(err) {
		t.Errorf("ReadPid after remove err = %v, want not-exist", err)
	}
	if err := RemovePid(p); err != nil {
		t.Errorf("RemovePid idempotent: %v", err)
	}
}

// TestStartupRoundTrip verifies the startup document survives JSON.
func TestStartupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	want := StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/?token=abc",
		PID:       4242,
		StartedAt: now,
		Version:   "v0.1.0-test",
		Manager:   "process",
	}
	if err := WriteStartup(p, want); err != nil {
		t.Fatalf("WriteStartup: %v", err)
	}
	info, err := ReadStartup(p)
	if err != nil {
		t.Fatalf("ReadStartup: %v", err)
	}
	if info.Listen != want.Listen || info.URL != want.URL || info.PID != want.PID || info.Version != want.Version || info.Manager != want.Manager {
		t.Errorf("ReadStartup = %+v, want %+v", info, want)
	}
	if !info.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", info.StartedAt, now)
	}
}

// TestStartupReadMissing is the not-exist path of ReadStartup.
func TestStartupReadMissing(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if _, err := ReadStartup(p); !os.IsNotExist(err) {
		t.Errorf("ReadStartup missing = %v, want not-exist", err)
	}
}

// TestGetStatusStopped verifies the "not running" path of GetStatus.
func TestGetStatusStopped(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
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
	dir := t.TempDir()
	withStateDir(t, dir)
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

// TestGetStatusRunning drops a startup document, leaves the pidfile empty
// and expects "stopped" — without a live PID, GetStatus cannot prove a
// dashboard is running.
func TestGetStatusRunningNeedsPID(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WriteStartup(p, StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/",
		PID:       os.Getpid(),
		StartedAt: time.Now(),
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
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WriteStartup(p, StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/",
		PID:       os.Getpid(),
		StartedAt: time.Now(),
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
	dir := t.TempDir()
	withStateDir(t, dir)
	err := Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("Stop without pidfile err = %v, want ErrNotRunning", err)
	}
}

// TestStopStalePidfile verifies Stop clears a stale pidfile.
func TestStopStalePidfile(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
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
		if got := loopbackURL(c.listen, nil); got != c.want {
			t.Errorf("loopbackURL(%q) = %q, want %q", c.listen, got, c.want)
		}
	}
}

// TestLANURLsNonLoopbackOnly verifies lanURLs returns nothing when the
// listener is already bound to one address.
func TestLANURLsNonLoopbackOnly(t *testing.T) {
	if urls := lanURLs("192.168.1.5:7860", nil); len(urls) != 0 {
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
	urls := lanURLs("0.0.0.0:7860", nil)
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
	dir := t.TempDir()
	withStateDir(t, dir)
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
	dir := t.TempDir()
	withStateDir(t, dir)
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
	dir := t.TempDir()
	withStateDir(t, dir)
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
