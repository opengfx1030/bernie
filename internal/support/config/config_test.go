package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		body    *string // nil = no file
		want    Config
		wantErr string
	}{
		{"missing file", nil, Config{}, ""},
		{"empty file", ptr(""), Config{}, ""},
		{"full", ptr("venv: /v\nmode: host\nengine: podman\nhf_home: /hf\nset: [\"env.A=1\", \"MAXLEN=4096\"]\n"),
			Config{Venv: "/v", Mode: "host", Engine: "podman", HFHome: "/hf", Set: []string{"env.A=1", "MAXLEN=4096"}}, ""},
		{"unknown key", ptr("vnev: /v\n"), Config{}, "field vnev not found"},
		{"bad mode", ptr("mode: vm\n"), Config{}, "mode must be host or container"},
		{"bad engine", ptr("engine: lxc\n"), Config{}, "engine must be docker or podman"},
		{"bad set", ptr("set: [novalue]\n"), Config{}, "want KEY=VALUE"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(dir, strings.Repeat("c", i+1)+".yaml")
			if tt.body != nil {
				if err := os.WriteFile(p, []byte(*tt.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			c, err := LoadFile(p)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c, tt.want) {
				t.Errorf("got %+v, want %+v", c, tt.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestSetKeyAndRoundTrip(t *testing.T) {
	var c Config
	steps := []struct {
		key, value string
		wantSet    []string
		wantErr    string
	}{
		{"set", "env.A=1", []string{"env.A=1"}, ""},
		{"set", "MAXLEN=4096", []string{"env.A=1", "MAXLEN=4096"}, ""},
		{"set", "env.A=2", []string{"env.A=2", "MAXLEN=4096"}, ""}, // same key replaced in place
		{"set", "-env.A", []string{"MAXLEN=4096"}, ""},
		{"set", "-nothere", []string{"MAXLEN=4096"}, ""},
		{"mode", "container", []string{"MAXLEN=4096"}, ""},
		{"mode", "vm", nil, "mode must be host or container"},
		{"colour", "x", nil, "unknown config key \"colour\""},
	}
	for _, s := range steps {
		err := c.SetKey(s.key, s.value)
		if s.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), s.wantErr) {
				t.Fatalf("SetKey(%s, %s) err = %v, want %q", s.key, s.value, err, s.wantErr)
			}
			c.Mode = "container" // undo the rejected value
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.Set, s.wantSet) {
			t.Fatalf("after SetKey(%s, %s): set = %v, want %v", s.key, s.value, c.Set, s.wantSet)
		}
	}
	p := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := SaveFile(p, c); err != nil {
		t.Fatal(err)
	}
	back, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, c) {
		t.Errorf("round trip: %+v != %+v", back, c)
	}
	if err := c.SetKey("set", ""); err != nil || c.Set != nil {
		t.Errorf("empty set value must clear the list: %v %v", c.Set, err)
	}
}

func TestResolveVenv(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		name, flag, envVenv, cfg, virtualEnv, want string
	}{
		{"flag first", "/flag", "/env", "/cfg", "/active", "/flag"},
		{"then BERNIE_VENV", "", "/env", "/cfg", "/active", "/env"},
		{"then config", "", "", "~/cfg", "/active", filepath.Join(home, "cfg")},
		{"then VIRTUAL_ENV", "", "", "", "/active", "/active"},
		{"none", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BERNIE_VENV", tt.envVenv)
			t.Setenv("VIRTUAL_ENV", tt.virtualEnv)
			if got := (Config{Venv: tt.cfg}).ResolveVenv(tt.flag); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveHFHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("HF_HOME", "/env/hf")
	if got := (Config{HFHome: "/cfg/hf"}).ResolveHFHome(); got != "/cfg/hf" {
		t.Errorf("config: %q", got)
	}
	if got := (Config{}).ResolveHFHome(); got != "/env/hf" {
		t.Errorf("env: %q", got)
	}
	t.Setenv("HF_HOME", "")
	if got := (Config{}).ResolveHFHome(); got != filepath.Join(home, ".cache", "huggingface") {
		t.Errorf("default: %q", got)
	}
}
