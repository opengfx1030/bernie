// Package paths resolves rdnactl's on-disk locations (XDG layout). Nothing is
// ever placed under /tmp: hosts reboot and /tmp is wiped.
//
//	config  $RDNACTL_CONFIG_DIR  | $XDG_CONFIG_HOME/rdnactl | ~/.config/rdnactl
//	state   $RDNACTL_STATE_DIR   | $XDG_STATE_HOME/rdnactl  | ~/.local/state/rdnactl
//	data    $RDNACTL_DATA_DIR    | $XDG_DATA_HOME/rdnactl   | ~/.local/share/rdnactl
//	cache   $RDNACTL_CACHE_DIR   | $XDG_CACHE_HOME/rdnactl  | ~/.cache/rdnactl
package paths

import (
	"os"
	"path/filepath"
)

func resolve(override, xdg, fallback string) string {
	if v := os.Getenv(override); v != "" {
		return v
	}
	if v := os.Getenv(xdg); v != "" {
		return filepath.Join(v, "rdnactl")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, fallback, "rdnactl")
}

// Config holds user settings and user recipes (Config()/recipes).
func Config() string { return resolve("RDNACTL_CONFIG_DIR", "XDG_CONFIG_HOME", ".config") }

// State holds run records (pid, logs, plans) and benchmark results.
func State() string { return resolve("RDNACTL_STATE_DIR", "XDG_STATE_HOME", ".local/state") }

// Data holds materialized assets (TunableOp rows, generated compose files).
func Data() string { return resolve("RDNACTL_DATA_DIR", "XDG_DATA_HOME", ".local/share") }

// Cache holds vLLM/Triton/Inductor compile caches per run.
func Cache() string { return resolve("RDNACTL_CACHE_DIR", "XDG_CACHE_HOME", ".cache") }

// Ensure creates dir (0755) and returns it.
func Ensure(dir string) (string, error) {
	return dir, os.MkdirAll(dir, 0o755)
}
