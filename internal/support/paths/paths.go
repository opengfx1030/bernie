// Package paths resolves bernie's on-disk locations (XDG layout). Nothing is
// ever placed under /tmp: hosts reboot and /tmp is wiped.
//
//	config  $BERNIE_CONFIG_DIR  | $XDG_CONFIG_HOME/bernie | ~/.config/bernie
//	state   $BERNIE_STATE_DIR   | $XDG_STATE_HOME/bernie  | ~/.local/state/bernie
//	data    $BERNIE_DATA_DIR    | $XDG_DATA_HOME/bernie   | ~/.local/share/bernie
//	cache   $BERNIE_CACHE_DIR   | $XDG_CACHE_HOME/bernie  | ~/.cache/bernie
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
		return filepath.Join(v, "bernie")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, fallback, "bernie")
}

// Config holds user settings and user recipes (Config()/recipes).
func Config() string { return resolve("BERNIE_CONFIG_DIR", "XDG_CONFIG_HOME", ".config") }

// State holds run records (pid, logs, plans) and benchmark results.
func State() string { return resolve("BERNIE_STATE_DIR", "XDG_STATE_HOME", ".local/state") }

// Data holds materialized assets (TunableOp rows, generated compose files).
func Data() string { return resolve("BERNIE_DATA_DIR", "XDG_DATA_HOME", ".local/share") }

// Cache holds vLLM/Triton/Inductor compile caches per run.
func Cache() string { return resolve("BERNIE_CACHE_DIR", "XDG_CACHE_HOME", ".cache") }

// Ensure creates dir (0755) and returns it.
func Ensure(dir string) (string, error) {
	return dir, os.MkdirAll(dir, 0o755)
}
