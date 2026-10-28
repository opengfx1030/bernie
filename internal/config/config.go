// Package config loads and saves the user defaults file <config>/config.yaml
// (spec §12): venv, mode, engine, hf_home and default --set overrides.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/rdnactl/internal/paths"
)

// Config is the content of config.yaml.
type Config struct {
	Venv   string   `json:"venv" yaml:"venv,omitempty"`
	Mode   string   `json:"mode" yaml:"mode,omitempty"`     // host | container | "" (auto)
	Engine string   `json:"engine" yaml:"engine,omitempty"` // docker | podman | "" (auto)
	HFHome string   `json:"hf_home" yaml:"hf_home,omitempty"`
	Set    []string `json:"set" yaml:"set,omitempty"` // applied before --set
}

// Keys are the settable keys, in file order.
var Keys = []string{"venv", "mode", "engine", "hf_home", "set"}

// Path is <config>/config.yaml.
func Path() string { return filepath.Join(paths.Config(), "config.yaml") }

// Load reads config.yaml. A missing file is an empty Config, not an error.
func Load() (Config, error) { return LoadFile(Path()) }

// LoadFile reads one config file. A missing file is an empty Config.
func LoadFile(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, c.validate()
}

func (c Config) validate() error {
	switch c.Mode {
	case "", "host", "container":
	default:
		return fmt.Errorf("config mode must be host or container, not %q", c.Mode)
	}
	switch c.Engine {
	case "", "docker", "podman":
	default:
		return fmt.Errorf("config engine must be docker or podman, not %q", c.Engine)
	}
	for _, s := range c.Set {
		if k, _, ok := strings.Cut(s, "="); !ok || k == "" {
			return fmt.Errorf("config set entry %q: want KEY=VALUE", s)
		}
	}
	return nil
}

// Save writes config.yaml (0644), creating the config directory.
func Save(c Config) error { return SaveFile(Path(), c) }

// SaveFile writes one config file.
func SaveFile(path string, c Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	if _, err := paths.Ensure(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetKey changes one key. For "set", VALUE is one KEY=VALUE override: it
// replaces an entry with the same KEY, and "KEY=" with an empty value after
// "-" ("-KEY") removes it. An empty VALUE clears scalar keys and the list.
func (c *Config) SetKey(key, value string) error {
	switch key {
	case "venv":
		c.Venv = value
	case "mode":
		c.Mode = value
	case "engine":
		c.Engine = value
	case "hf_home":
		c.HFHome = value
	case "set":
		c.Set = setOverride(c.Set, value)
	default:
		return fmt.Errorf("unknown config key %q (keys: %s)", key, strings.Join(Keys, ", "))
	}
	return c.validate()
}

func setOverride(list []string, value string) []string {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "-") {
		k := strings.TrimPrefix(value, "-")
		out := list[:0:0]
		for _, s := range list {
			if sk, _, _ := strings.Cut(s, "="); sk != k {
				out = append(out, s)
			}
		}
		return out
	}
	k, _, _ := strings.Cut(value, "=")
	out := make([]string, 0, len(list)+1)
	replaced := false
	for _, s := range list {
		if sk, _, _ := strings.Cut(s, "="); sk == k {
			if !replaced {
				out = append(out, value)
				replaced = true
			}
			continue
		}
		out = append(out, s)
	}
	if !replaced {
		out = append(out, value)
	}
	return out
}

// ResolveVenv applies the venv order of spec §8.1: flag, $RDNACTL_VENV,
// config venv, $VIRTUAL_ENV. "" when none is set.
func (c Config) ResolveVenv(flag string) string {
	for _, v := range []string{flag, os.Getenv("RDNACTL_VENV"), c.Venv, os.Getenv("VIRTUAL_ENV")} {
		if v != "" {
			return expandHome(v)
		}
	}
	return ""
}

// ResolveHFHome is config hf_home, then $HF_HOME, then ~/.cache/huggingface.
func (c Config) ResolveHFHome() string {
	if c.HFHome != "" {
		return expandHome(c.HFHome)
	}
	if v := os.Getenv("HF_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// Map returns the config as ordered key/value pairs for display. The set
// list keeps its order: later entries win.
func (c Config) Map() [][2]string {
	return [][2]string{
		{"venv", c.Venv}, {"mode", c.Mode}, {"engine", c.Engine},
		{"hf_home", c.HFHome}, {"set", strings.Join(c.Set, " ")},
	}
}
