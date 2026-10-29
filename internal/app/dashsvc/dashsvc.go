// Package dashsvc manages the bernie dashboard as a long-running service.
// The same package backs `bernie dash start|stop|status`, the installer
// `--start` flag and the MCP tools `dash_start`, `dash_stop` and
// `dash_status`: one app function, three front ends (spec §11, §13).
//
// The service state lives under `<state>/dashboard/`:
//
//	token       0600; the bearer token the guard reads (spec §11). The
//	            service generates one when it first starts and keeps it
//	            across restarts; --rotate-token replaces it.
//	dash.pid    the PID of the dashboard process. The child writes it
//	            after it forks; `stop` reads it and sends the signal.
//	dash.log    combined stdout+stderr of the child.
//	startup.json the URL the dashboard ended up listening on. The child
//	            writes it once the socket binds, so the parent knows the
//	            URL even though it has already returned. `status` reads
//	            it to print the URLs.
//
// Nothing goes under /tmp; the state dir is XDG. The files live in
// dashsvc_state.go, the child in dashsvc_child.go and the systemd unit
// in dashsvc_systemd.go.
package dashsvc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// dashDefaultListen is the loopback address of spec §11. The dashboard
// package owns this constant; we inline it here to avoid a dashboard →
// mcp → dashsvc → dashboard cycle (the dashboard server wires /mcp).
const dashDefaultListen = "127.0.0.1:7860"

// Errors returned by the service functions.
var (
	// ErrAlreadyRunning is returned by Start when a dashboard is already
	// up; the caller should print `bernie dash status`.
	ErrAlreadyRunning = errors.New("dashboard is already running")
	// ErrNotRunning is returned by Stop when no dashboard is up.
	ErrNotRunning = errors.New("dashboard is not running")
)

// StartOptions configure Start.
type StartOptions struct {
	// Listen is the bind address; empty falls back to dashDefaultListen.
	Listen string
	// Token is the explicit token to use; empty means "use or generate".
	Token string
	// RotateToken replaces the persisted token before starting.
	RotateToken bool
	// Version is the bernie version the child reports to MCP clients.
	Version string
	// NoSystemd skips the systemd --user path even when it is available.
	NoSystemd bool
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Status describes the dashboard service. Token is redacted unless the
// caller asks for it (the CLI shows it with --show-token).
type Status struct {
	State       string    `json:"state"` // "running", "stopped", "stale"
	PID         int       `json:"pid,omitempty"`
	Listen      string    `json:"listen,omitempty"`
	Token       string    `json:"token,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	Version     string    `json:"version,omitempty"`
	Manager     string    `json:"manager,omitempty"`
	LoopbackURL string    `json:"loopback_url,omitempty"`
	LANURLs     []string  `json:"lan_urls,omitempty"`
	Hostname    string    `json:"hostname,omitempty"`
	LogPath     string    `json:"log_path,omitempty"`
	TokenPath   string    `json:"token_path,omitempty"`
}

// Start spawns a detached dashboard service. The call returns when the
// child has bound its socket (or when ctx ends); the caller prints the
// URLs. The function is idempotent: a second call returns
// ErrAlreadyRunning.
func Start(ctx context.Context, o StartOptions) (*Status, error) {
	p, err := EnsureDir()
	if err != nil {
		return nil, err
	}
	// Resolve the token (use, generate or rotate).
	tok, err := resolveToken(p, o)
	if err != nil {
		return nil, err
	}
	// Reject a second start. A stale pidfile is removed.
	if cur, perr := ReadPid(p); perr == nil && IsAlive(cur) {
		return nil, fmt.Errorf("%w (pid %d): run `bernie dash stop` first", ErrAlreadyRunning, cur)
	}
	_ = RemovePid(p)
	_ = RemoveStartup(p)

	listen := o.Listen
	if listen == "" {
		listen = dashDefaultListen
	}

	if !o.NoSystemd && systemdUserAvailable() {
		if err := startSystemd(ctx, p, listen, tok, o.Version); err != nil {
			return nil, err
		}
		return waitReady(ctx, p, "systemd")
	}
	if err := startDetached(ctx, p, listen, tok, o.Version); err != nil {
		return nil, err
	}
	return waitReady(ctx, p, "process")
}

// resolveToken picks the token Start should hand to the child.
func resolveToken(p Paths, o StartOptions) (string, error) {
	if o.Token != "" {
		if err := os.WriteFile(p.Token, []byte(o.Token+"\n"), 0o600); err != nil {
			return "", err
		}
		return o.Token, nil
	}
	if o.RotateToken {
		tok := dashNewToken()
		if err := os.WriteFile(p.Token, []byte(tok+"\n"), 0o600); err != nil {
			return "", err
		}
		return tok, nil
	}
	tok, _, err := ReadToken()
	return tok, err
}

// Stop sends SIGTERM to the running dashboard, waits up to five seconds,
// then SIGKILL if it is still alive. The pidfile is removed. The function
// returns ErrNotRunning when no dashboard is up.
func Stop(ctx context.Context) error {
	p, err := EnsureDir()
	if err != nil {
		return err
	}
	pid, err := ReadPid(p)
	if errors.Is(err, os.ErrNotExist) {
		// Try systemd --user too.
		if !stopSystemd() {
			return fmt.Errorf("%w", ErrNotRunning)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !IsAlive(pid) {
		_ = RemovePid(p)
		_ = stopSystemd()
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		_ = RemovePid(p)
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			_ = RemovePid(p)
			_ = stopSystemd()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = RemovePid(p)
	_ = stopSystemd()
	return nil
}

// GetStatus reads the current service state and returns it. It never
// errors: a stopped service is a normal answer. The token is redacted; the
// CLI passes it through with --show-token.
func GetStatus() (*Status, error) {
	p, err := EnsureDir()
	if err != nil {
		return nil, err
	}
	st := &Status{State: "stopped", LogPath: p.Log, TokenPath: p.Token}
	pid, perr := ReadPid(p)
	info, serr := ReadStartup(p)
	switch {
	case perr == nil && IsAlive(pid):
		st.State = "running"
		st.PID = pid
		if serr == nil {
			st.Listen = info.Listen
			st.StartedAt = info.StartedAt
			st.Version = info.Version
			st.Manager = info.Manager
		}
		st.Hostname, _ = os.Hostname()
		st.LoopbackURL = loopbackURL(st.Listen)
		st.LANURLs = lanURLs(st.Listen)
	case perr == nil:
		st.State = "stale"
		st.PID = pid
	default:
		// No pidfile. A systemd unit might still be running; fall through.
		if info, serr := ReadStartup(p); serr == nil {
			if runningSystemd() {
				st.State = "running"
				st.Listen = info.Listen
				st.StartedAt = info.StartedAt
				st.Version = info.Version
				st.Manager = info.Manager
				st.Hostname, _ = os.Hostname()
				st.LoopbackURL = loopbackURL(st.Listen)
				st.LANURLs = lanURLs(st.Listen)
			}
		}
	}
	return st, nil
}

// loopbackURL returns the URL on the loopback interface of the running
// dashboard. Empty when the listener is not known.
func loopbackURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// lanURLs returns the LAN URLs of the running dashboard, one per non-loopback
// interface address. Empty when the listener is loopback only or unknown.
func lanURLs(listen string) []string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return nil // bound to one address
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var urls []string
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			if ip4.IsLinkLocalUnicast() {
				continue
			}
			if seen[ip4.String()] {
				continue
			}
			seen[ip4.String()] = true
			urls = append(urls, "http://"+net.JoinHostPort(ip4.String(), port)+"/")
		}
	}
	return urls
}

// ReadyTimeout is how long Start waits for the child to write startup.json.
const ReadyTimeout = 8 * time.Second

// waitReady polls startup.json until the child writes it or ctx ends.
func waitReady(ctx context.Context, p Paths, manager string) (*Status, error) {
	deadline := time.Now().Add(ReadyTimeout)
	for {
		info, err := ReadStartup(p)
		if err == nil {
			return &Status{
				State:       "running",
				PID:         info.PID,
				Listen:      info.Listen,
				StartedAt:   info.StartedAt,
				Version:     info.Version,
				Manager:     manager,
				LoopbackURL: loopbackURL(info.Listen),
				LANURLs:     lanURLs(info.Listen),
				LogPath:     p.Log,
				TokenPath:   p.Token,
			}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("dashboard child did not become ready in %s; see %s", ReadyTimeout, p.Log)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
