package dashsvc

import (
	"context"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// This file owns the child of the service: the detached process that
// serves the dashboard, and the two files the child writes (the pidfile
// and the startup document).

// startDetached spawns a bernie child process with setsid so it outlives
// the parent shell, redirects stdio to the log file and waits for the
// child to write its pidfile.
func startDetached(ctx context.Context, p Paths, listen, token, version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logF, err := os.OpenFile(p.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	// The child must outlive this process: exec.Command (not
	// CommandContext) keeps it alive when the parent returns and the
	// CLI context is cancelled.
	cmd := exec.Command(exe, dashChildArgs(listen, token, version)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = logF
	cmd.Stderr = logF
	// The child reads the same directories as the parent: the BERNIE_*
	// and XDG_* variables travel with it.
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
// the same args (the internal `--child` flag) and is the only thing that
// knows how to run the dashboard service.
func dashChildArgs(listen, token, version string) []string {
	return []string{
		"dash", "--child",
		"--listen", listen,
		"--token", token,
	}
}

// childEnv is the env passed to the detached child. The child must see the
// same bernie directories as the parent: BERNIE_STATE_DIR, BERNIE_CONFIG_DIR,
// BERNIE_DATA_DIR, BERNIE_CACHE_DIR and BERNIE_ROOT travel with it, else the
// child writes its pidfile and its token into another tree than the parent
// reads. The XDG_* variables travel too, because they are the fallback of
// the paths package.
func childEnv() []string {
	keep := []string{"PATH", "HOME", "USER", "LANG", "LC_ALL", "TERM", "TZ",
		"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"BERNIE_CONFIG_DIR", "BERNIE_STATE_DIR", "BERNIE_DATA_DIR", "BERNIE_CACHE_DIR",
		"BERNIE_ROOT", "BERNIE_VENV", "BERNIE_RECIPES", "BERNIE_DASH_TOKEN", "BERNIE_MCP_WRITE"}
	out := make([]string, 0, len(keep))
	for _, k := range keep {
		if v := os.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// ChildOptions configure the child mode (`bernie dash --child`). The
// service mode is for `bernie dash --child` only; the foreground
// `bernie dash` does not go through RunChild.
type ChildOptions struct {
	Listen  string
	Token   string
	Version string
	Now     func() time.Time
}

// RunChild writes the pidfile of the dashboard service child, then
// returns. The actual serving of HTTP is done by the caller (the
// `bernie dash --child` command in cmd_dash.go calls dashboard.Serve
// directly); we only own the lifecycle files here, to break the
// dashboard → mcp → dashsvc → dashboard cycle.
//
// The startup document is written later, when the socket binds: the
// caller calls WriteChildStartup from the Ready callback of the
// dashboard.
//
// Callers should defer RunChildCleanup to remove the pidfile on exit.
func RunChild(_ context.Context, o ChildOptions) error {
	p, err := EnsureDir()
	if err != nil {
		return err
	}
	return WritePid(p, os.Getpid())
}

// WriteChildStartup writes the startup document of the child with the URL
// the dashboard bound to. The parent polls for this file to learn the URL.
func WriteChildStartup(listen, url, version string) error {
	p, err := EnsureDir()
	if err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(listen)
	bound := net.JoinHostPort(host, port)
	return WriteStartup(p, StartupInfo{
		Listen:    bound,
		URL:       url,
		PID:       os.Getpid(),
		StartedAt: time.Now(),
		Version:   version,
		Manager:   "process",
	})
}

// RunChildCleanup removes the pidfile when the child exits. Callers
// invoke this in a defer after RunChild.
func RunChildCleanup() {
	p := ServicePaths()
	_ = RemovePid(p)
}
