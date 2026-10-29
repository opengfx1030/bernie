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
	resetVenvCache()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BERNIE_DATA_DIR", filepath.Join(home, "data"))
	tests := []struct {
		name, flag, envVenv, cfg, virtualEnv, want, source string
	}{
		{"flag first", "/flag", "/env", "/cfg", "/active", "/flag", VenvSourceFlag},
		{"then BERNIE_VENV", "", "/env", "/cfg", "/active", "/env", VenvSourceBernieEnv},
		{"then config", "", "", "~/cfg", "/active", filepath.Join(home, "cfg"), VenvSourceConfig},
		{"then VIRTUAL_ENV", "", "", "", "/active", "/active", VenvSourceVirtualEnv},
		{"none", "", "", "", "", "", VenvSourceNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BERNIE_VENV", tt.envVenv)
			t.Setenv("VIRTUAL_ENV", tt.virtualEnv)
			got := (Config{Venv: tt.cfg}).ResolveVenvInfo(tt.flag)
			if got.Path != tt.want || got.Source != tt.source {
				t.Errorf("got %+v, want path %q from %q", got, tt.want, tt.source)
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

func TestFleetRoundTrip(t *testing.T) {
	// add then load then re-add then remove; the token stays on the entry
	// that already has one and rides with the file.
	c := Config{}
	if err := c.SetKey("fleet", "add gpu-box-2 http://127.0.0.1:7860 abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetKey("fleet", "add gpu-box-3 http://127.0.0.1:7861"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetKey("fleet", "add gpu-box-2 http://127.0.0.1:7862"); err != nil {
		t.Fatal(err)
	} // no token keeps the old one
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := SaveFile(p, c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("with a fleet token, file mode = %v, want 0600", info.Mode().Perm())
	}
	back, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Fleet) != 2 {
		t.Fatalf("got %d entries, want 2", len(back.Fleet))
	}
	if back.Fleet[0].Name != "gpu-box-2" || back.Fleet[0].URL != "http://127.0.0.1:7862" || back.Fleet[0].Token != "abcdef" {
		t.Errorf("gpu-box-2: %+v", back.Fleet[0])
	}
	if back.Fleet[1].Name != "gpu-box-3" || back.Fleet[1].URL != "http://127.0.0.1:7861" || back.Fleet[1].Token != "" {
		t.Errorf("gpu-box-3: %+v", back.Fleet[1])
	}
	if !back.RemoveFleet("gpu-box-2") {
		t.Errorf("remove: got false")
	}
	if back.RemoveFleet("gpu-box-2") {
		t.Errorf("second remove: got true")
	}
	// A config without a token goes back to 0644.
	if err := SaveFile(p, back); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("without a token, file mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestFleetValidate(t *testing.T) {
	cases := []struct {
		name    string
		entry   FleetEntry
		wantErr string
	}{
		{"empty name", FleetEntry{URL: "http://h"}, "name is required"},
		{"empty url", FleetEntry{Name: "a"}, "url is required"},
		{"bad scheme", FleetEntry{Name: "a", URL: "ftp://h"}, "url must be http or https"},
		{"no host", FleetEntry{Name: "a", URL: "http://"}, "url must name a host"},
		{"good", FleetEntry{Name: "a", URL: "http://h:7860", Token: "t"}, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFleetEntry(tt.entry)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("got %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestFleetDuplicateName(t *testing.T) {
	c := Config{}
	if err := c.SetKey("fleet", "add box http://h:1"); err != nil {
		t.Fatal(err)
	}
	err := c.SetKey("fleet", "add box http://h:2")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err = %v, want duplicate", err)
	}
}

func TestFleetMapHidesURLAndToken(t *testing.T) {
	c := Config{Fleet: []FleetEntry{
		{Name: "box", URL: "http://h:1", Token: "secret-token"},
	}}
	seen := map[string]bool{}
	for _, kv := range c.Map() {
		seen[kv[0]] = true
		if strings.Contains(kv[1], "secret-token") || strings.Contains(kv[1], "http://") {
			t.Errorf("fleet row %q carries the secret or the URL: %q", kv[0], kv[1])
		}
	}
	if !seen["fleet"] {
		t.Errorf("Map: no fleet row")
	}
}
