package dashsvc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// This file owns the systemd --user path of the service. When the user
// manager answers, the dashboard runs as a `bernie-dash.service` unit
// instead of a detached process. The unit outlives the session only
// after the operator runs `loginctl enable-linger $USER` as root;
// bernie prints that command and never runs it.

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
