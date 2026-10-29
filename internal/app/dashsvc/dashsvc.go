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
//	startup.json the URL the dashboard ended up listening on (token
//	            redacted). The child writes it once the socket binds,
//	            so the parent knows the URL even though it has already
//	            returned. `status` reads it to print the URLs.
//
// Nothing goes under /tmp; the state dir is XDG.
package dashsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// dashDefaultListen is the loopback address of spec §11. The dashboard
// package owns this constant; we inline it here to avoid a dashboard →
// mcp → dashsvc → dashboard cycle (the dashboard server wires /mcp).
const dashDefaultListen = "127.0.0.1:7860"

// dashNewToken is the dashboard token generator (a 128-bit hex string).
func dashNewToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("dashsvc: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// dashIsLoopback reports whether the listen address binds only to
// loopback.
func dashIsLoopback(listen string) bool {
	h, _, err := net.SplitHostPort(listen)
	if err != nil {
		h = listen
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// File names in the state directory.
const (
	DirName     = "dashboard"
	TokenFile   = "token"
	PidFile     = "dash.pid"
	LogFile     = "dash.log"
	StartupFile = "startup.json"
	UnitFile    = "bernie-dash.service"
	UnitName    = "bernie-dash.service"
)

// Errors returned by the service functions.
var (
	// ErrAlreadyRunning is returned by Start when a dashboard is already
	// up; the caller should print `bernie dash status`.
	ErrAlreadyRunning = errors.New("dashboard is already running")
	// ErrNotRunning is returned by Stop when no dashboard is up.
	ErrNotRunning = errors.New("dashboard is not running")
)

// Paths holds the file paths the service uses. The fields are absolute and
// always live under the XDG state dir (spec §12). Nothing is ever under
// /tmp.
type Paths struct {
	State   string // <state>/dashboard
	Token   string // <state>/dashboard/token
	Pid     string // <state>/dashboard/dash.pid
	Log     string // <state>/dashboard/dash.log
	Startup string // <state>/dashboard/startup.json
	Unit    string // <state>/dashboard/bernie-dash.service
}

// ServicePaths returns the file paths the service uses. The state dir is
// created lazily by EnsureDir; this function does not touch the disk.
func ServicePaths() Paths {
	base := filepath.Join(paths.State(), DirName)
	return Paths{
		State:   base,
		Token:   filepath.Join(base, TokenFile),
		Pid:     filepath.Join(base, PidFile),
		Log:     filepath.Join(base, LogFile),
		Startup: filepath.Join(base, StartupFile),
		Unit:    filepath.Join(base, UnitFile),
	}
}

// EnsureDir creates the state dir (mode 0700) if it does not exist and
// returns the Paths.
func EnsureDir() (Paths, error) {
	p := ServicePaths()
	if err := os.MkdirAll(p.State, 0o700); err != nil {
		return p, err
	}
	return p, nil
}

// ReadToken returns the persisted token, generating one and persisting it
// when the file is missing. The file is mode 0600.
func ReadToken() (string, Paths, error) {
	p, err := EnsureDir()
	if err != nil {
		return "", p, err
	}
	data, err := os.ReadFile(p.Token)
	if err == nil {
		tok := strings.TrimSpace(string(data))
		if tok != "" {
			return tok, p, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", p, err
	}
	tok := dashNewToken()
	if err := os.WriteFile(p.Token, []byte(tok+"\n"), 0o600); err != nil {
		return "", p, err
	}
	return tok, p, nil
}

// WriteToken overwrites the persisted token.
func WriteToken(tok string) (Paths, error) {
	p, err := EnsureDir()
	if err != nil {
		return p, err
	}
	if err := os.WriteFile(p.Token, []byte(tok+"\n"), 0o600); err != nil {
		return p, err
	}
	return p, nil
}

// IsAlive reports whether pid is a live process (signal 0 succeeds).
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ReadPid reads the pidfile.
func ReadPid(p Paths) (int, error) {
	data, err := os.ReadFile(p.Pid)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("dash pidfile: %w", err)
	}
	return pid, nil
}

// WritePid writes the PID file with the running dashboard's PID.
func WritePid(p Paths, pid int) error {
	return os.WriteFile(p.Pid, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// RemovePid removes the PID file when present.
func RemovePid(p Paths) error {
	err := os.Remove(p.Pid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// StartupInfo is the document the child writes when it starts listening.
// Status reads it back to print the URLs.
type StartupInfo struct {
	Listen    string    `json:"listen"`
	URL       string    `json:"url"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
	Manager   string    `json:"manager"` // "process" or "systemd"
}

// WriteStartup writes the startup document (mode 0600).
func WriteStartup(p Paths, info StartupInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(p.Startup, data, 0o600)
}

// ReadStartup reads the startup document. Returns os.ErrNotExist when the
// child has not written one yet.
func ReadStartup(p Paths) (*StartupInfo, error) {
	data, err := os.ReadFile(p.Startup)
	if err != nil {
		return nil, err
	}
	var info StartupInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("dash startup: %w", err)
	}
	return &info, nil
}

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

// RemoveStartup deletes the startup file when present.
func RemoveStartup(p Paths) error {
	err := os.Remove(p.Startup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
		st.LoopbackURL = loopbackURL(st.Listen, info)
		st.LANURLs = lanURLs(st.Listen, info)
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
				st.LoopbackURL = loopbackURL(st.Listen, info)
				st.LANURLs = lanURLs(st.Listen, info)
			}
		}
	}
	return st, nil
}

// loopbackURL returns the URL on the loopback interface of the running
// dashboard. Empty when the listener is not known.
func loopbackURL(listen string, info *StartupInfo) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port) + "/"
	if info != nil {
		// Token is not embedded in startup.json; status is redacted by
		// default. The CLI re-renders with --show-token.
	}
	return url
}

// lanURLs returns the LAN URLs of the running dashboard, one per non-loopback
// interface address. Empty when the listener is loopback only or unknown.
func lanURLs(listen string, _ *StartupInfo) []string {
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
				LoopbackURL: loopbackURL(info.Listen, info),
				LANURLs:     lanURLs(info.Listen, info),
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

// --- detached process ---------------------------------------------------

// startDetached spawns a bernie child process with setsid so it outlives
// the parent shell, redirects stdio to the log file and writes the pidfile
// before exec.
func startDetached(ctx context.Context, p Paths, listen, token, version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logF, err := os.OpenFile(p.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, dashChildArgs(listen, token, version)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = logF
	cmd.Stderr = logF
	// Detach from the parent's environment except for a clean PATH/TERM.
	cmd.Env = childEnv()
	if err := cmd.Start(); err != nil {
		_ = logF.Close()
		return err
	}
	_ = logF.Close()
	// Wait briefly for the child to write its pidfile; failure here
	// just means status will fall back to the startup document.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pid, err := ReadPid(p); err == nil && pid == cmd.Process.Pid {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil
}

// dashChildArgs is the argument list of the detached child. The CLI sees
// the same args (foreground mode) and is the only thing that knows how to
// run the dashboard service.
func dashChildArgs(listen, token, version string) []string {
	return []string{
		"dash", "--child",
		"--listen", listen,
		"--token", token,
	}
}

// childEnv is the env passed to the detached child.
func childEnv() []string {
	keep := []string{"PATH", "HOME", "USER", "LANG", "LC_ALL", "TERM", "TZ"}
	out := make([]string, 0, len(keep))
	for _, k := range keep {
		if v := os.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// --- systemd --user ------------------------------------------------------

// SystemdUserAvailable reports whether the user systemd manager can be
// reached. It probes with `systemctl --user show-environment`: the command
// is harmless, has no side effects, and exits non-zero when the user
// manager is not running. CLI callers and MCP tools use it to decide
// whether to print the enable-linger hint.
func SystemdUserAvailable() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	cmd := exec.Command("systemctl", "--user", "show-environment")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// An empty answer is also a sign the manager is not running.
	return len(strings.TrimSpace(string(out))) > 0
}

// systemdUserAvailable is the package-internal alias.
func systemdUserAvailable() bool { return SystemdUserAvailable() }

// runningSystemd reports whether the bernie-dash --user unit is active.
func runningSystemd() bool {
	cmd := exec.Command("systemctl", "--user", "is-active", UnitName)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// startSystemd writes a --user unit, links and starts it. The unit runs
// `bernie dash --child` with the same args as the detached process.
func startSystemd(ctx context.Context, p Paths, listen, token, version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	unit := renderSystemdUnit(exe, listen, token)
	if err := os.WriteFile(p.Unit, []byte(unit), 0o644); err != nil {
		return err
	}
	// Make the unit known to the user manager. `link` copies it into
	// XDG_CONFIG_HOME/systemd/user; the start command works without it
	// but the user can then `enable --user` it to outlive the session.
	link := exec.CommandContext(ctx, "systemctl", "--user", "link", p.Unit)
	link.Env = os.Environ()
	if out, err := link.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user link: %w: %s", err, strings.TrimSpace(string(out)))
	}
	reload := exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload")
	reload.Env = os.Environ()
	_ = reload.Run()
	start := exec.CommandContext(ctx, "systemctl", "--user", "start", UnitName)
	start.Env = os.Environ()
	if out, err := start.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user start: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stopSystemd stops the user unit. Returns true when the unit was active.
func stopSystemd() bool {
	cmd := exec.Command("systemctl", "--user", "stop", UnitName)
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

// renderSystemdUnit returns the --user unit body that runs the dashboard
// service. The unit inherits the user's environment so $BERNIE_STATE_DIR
// and friends point at the user's XDG tree.
func renderSystemdUnit(exe, listen, token string) string {
	return fmt.Sprintf(`[Unit]
Description=bernie dashboard
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s dash --child --listen %s --token %s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, exe, listen, token)
}

// --- service child -------------------------------------------------------

// ChildOptions configure the child mode (`bernie dash --child`). The
// service mode is for `bernie dash --child` only; the foreground
// `bernie dash` does not go through RunChild.
type ChildOptions struct {
	Listen  string
	Token   string
	Version string
	Now     func() time.Time
}

// RunChild writes the pidfile and startup document for the dashboard
// service child, then returns. The actual serving of HTTP is done by
// the caller (the `bernie dash --child` command in cmd_dash.go calls
// dashboard.Serve directly); we only own the lifecycle files here, to
// break the dashboard → mcp → dashsvc → dashboard cycle.
//
// Callers should defer RunChildCleanup to remove the pidfile on exit.
func RunChild(_ context.Context, o ChildOptions) error {
	p, err := EnsureDir()
	if err != nil {
		return err
	}
	if err := WritePid(p, os.Getpid()); err != nil {
		return err
	}
	oNow := o.Now
	if oNow == nil {
		oNow = time.Now
	}
	host, port, _ := net.SplitHostPort(o.Listen)
	listen := net.JoinHostPort(host, port)
	_ = WriteStartup(p, StartupInfo{
		Listen:    listen,
		URL:       "http://" + listen + "/",
		PID:       os.Getpid(),
		StartedAt: oNow(),
		Version:   o.Version,
		Manager:   "process",
	})
	return nil
}

// RunChildCleanup removes the pidfile when the child exits. Callers
// invoke this in a defer after RunChild.
func RunChildCleanup() {
	p := ServicePaths()
	_ = RemovePid(p)
}
