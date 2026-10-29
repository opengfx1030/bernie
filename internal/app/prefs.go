package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// PrefsReport is the `bernie config show` view (spec §3.2, ia.md R28).
type PrefsReport struct {
	Path  string   `json:"path"`
	Keys  []string `json:"keys"`
	Items []Pref   `json:"items"`
}

// Pref is one row of the preferences panel.
type Pref struct {
	Key      string   `json:"key"`
	Value    string   `json:"value"`
	Source   string   `json:"source"`
	Resolved string   `json:"resolved,omitempty"`
	Choices  []string `json:"choices,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// Prefs reads the user defaults with their source (config, env, default).
// Secrets (the fleet token) are stripped before they leave the file.
func Prefs(ctx context.Context) (PrefsReport, error) {
	cfg, err := config.Load()
	if err != nil {
		return PrefsReport{}, err
	}
	rep := PrefsReport{Path: config.Path(), Keys: append([]string{}, config.Keys...)}
	for _, k := range config.Keys {
		rep.Items = append(rep.Items, prefOf(k, &cfg))
	}
	return rep, nil
}

func prefOf(key string, cfg *config.Config) Pref {
	switch key {
	case "venv":
		resolved, src := resolveVenv(cfg)
		return Pref{Key: key, Value: cfg.Venv, Source: src, Resolved: resolved,
			Hint: "Path of the host-mode Python venv (bernie uses it for vLLM and the doctor)."}
	case "mode":
		return Pref{Key: key, Value: cfg.Mode, Source: sourceOf(cfg.Mode), Resolved: effectiveMode(cfg.Mode),
			Choices: []string{"host", "container"}, Hint: "Default run mode for bernie serve (--mode)."}
	case "engine":
		return Pref{Key: key, Value: cfg.Engine, Source: sourceOf(cfg.Engine), Resolved: cfg.Engine,
			Choices: []string{"docker", "podman"}, Hint: "Default container engine for --mode container."}
	case "device_access":
		return Pref{Key: key, Value: cfg.DeviceAccess, Source: sourceOf(cfg.DeviceAccess), Resolved: cfg.DeviceAccess,
			Choices: []string{"classic", "cdi", "auto"}, Hint: "How a container gets the GPU (--device-access)."}
	case "hf_home":
		resolved := cfg.ResolveHFHome()
		src := "default"
		if cfg.HFHome != "" {
			src = "config"
		} else if os.Getenv("HF_HOME") != "" {
			src = "$HF_HOME"
		}
		return Pref{Key: key, Value: cfg.HFHome, Source: src, Resolved: resolved,
			Hint: "Root of the Hugging Face cache (HF_HOME)."}
	case "set":
		src := "default"
		if len(cfg.Set) > 0 {
			src = "config"
		}
		return Pref{Key: key, Value: joinList(cfg.Set), Source: src, Resolved: joinList(cfg.Set),
			Hint: "Default --set overrides (KEY=VALUE, -KEY removes)."}
	case "fleet":
		names := make([]string, len(cfg.Fleet))
		for i, e := range cfg.Fleet {
			names[i] = e.Name
		}
		return Pref{Key: key, Value: joinList(names), Source: sourceOfList(names),
			Resolved: joinList(names), Hint: "Other bernie hosts the Fleet tab watches."}
	}
	return Pref{Key: key, Value: "", Source: "none"}
}

// SetPref validates and saves one KEY/VALUE pair through Config.SetKey.
func SetPref(ctx context.Context, key, value string) (PrefsReport, error) {
	cfg, err := config.Load()
	if err != nil {
		return PrefsReport{}, err
	}
	if err := cfg.SetKey(key, value); err != nil {
		return PrefsReport{}, fmt.Errorf("%v", err)
	}
	if err := config.Save(cfg); err != nil {
		return PrefsReport{}, err
	}
	return Prefs(ctx)
}

func sourceOf(v string) string {
	if v == "" {
		return "default"
	}
	return "config"
}
func sourceOfList(v []string) string {
	if len(v) == 0 {
		return "default"
	}
	return "config"
}
func effectiveMode(m string) string {
	if m == "" {
		return "auto"
	}
	return m
}
func joinList(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}

func resolveVenv(cfg *config.Config) (string, string) {
	if cfg.Venv != "" {
		return cfg.Venv, "config"
	}
	if v := os.Getenv("BERNIE_VENV"); v != "" {
		return v, "$BERNIE_VENV"
	}
	if v := os.Getenv("VIRTUAL_ENV"); v != "" {
		return v, "$VIRTUAL_ENV"
	}
	cands := conventionalVenvs()
	switch len(cands) {
	case 0:
		return "", "none"
	case 1:
		return cands[0], "conventional"
	default:
		return "", "ambiguous"
	}
}

func conventionalVenvs() []string {
	var out []string
	for _, rel := range []string{"venv", ".venv"} {
		if p, err := filepath.Abs(rel); err == nil && isVenv(p) {
			out = append(out, p)
		}
	}
	share := filepath.Join(paths.State(), "venvs")
	if entries, err := os.ReadDir(share); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(share, e.Name())
			if isVenv(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func isVenv(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return false
	}
	if st, err := os.Stat(filepath.Join(p, "bin", "python")); err == nil && !st.IsDir() {
		return true
	}
	if st, err := os.Stat(filepath.Join(p, "Scripts", "python.exe")); err == nil && !st.IsDir() {
		return true
	}
	return false
}
