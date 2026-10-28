package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

var dirs = []struct {
	name     string
	fn       func() string
	override string
	xdg      string
	fallback string // under $HOME
}{
	{"config", Config, "BERNIE_CONFIG_DIR", "XDG_CONFIG_HOME", ".config/bernie"},
	{"state", State, "BERNIE_STATE_DIR", "XDG_STATE_HOME", ".local/state/bernie"},
	{"data", Data, "BERNIE_DATA_DIR", "XDG_DATA_HOME", ".local/share/bernie"},
	{"cache", Cache, "BERNIE_CACHE_DIR", "XDG_CACHE_HOME", ".cache/bernie"},
}

// clearEnv blanks every variable the resolver reads and sets HOME.
func clearEnv(t *testing.T, home string) {
	t.Helper()
	for _, d := range dirs {
		t.Setenv(d.override, "")
		t.Setenv(d.xdg, "")
	}
	t.Setenv("HOME", home)
}

func TestResolutionOrder(t *testing.T) {
	for _, d := range dirs {
		t.Run(d.name, func(t *testing.T) {
			clearEnv(t, "/home/op")
			if got, want := d.fn(), filepath.Join("/home/op", d.fallback); got != want {
				t.Errorf("default = %q, want %q", got, want)
			}
			t.Setenv(d.xdg, "/xdg")
			if got, want := d.fn(), "/xdg/bernie"; got != want {
				t.Errorf("XDG = %q, want %q", got, want)
			}
			t.Setenv(d.override, "/srv/bernie-"+d.name)
			if got, want := d.fn(), "/srv/bernie-"+d.name; got != want {
				t.Errorf("override = %q, want %q (used as is, no /bernie suffix)", got, want)
			}
		})
	}
}

func TestOverrideIsPerDirectory(t *testing.T) {
	clearEnv(t, "/home/op")
	t.Setenv("BERNIE_STATE_DIR", "/var/lib/bernie")
	if Config() != "/home/op/.config/bernie" || State() != "/var/lib/bernie" {
		t.Errorf("config %q state %q", Config(), State())
	}
}

// Without a usable home the paths stay relative to the working directory;
// they are never invented under /tmp.
func TestNeverTmp(t *testing.T) {
	clearEnv(t, "")
	for _, d := range dirs {
		got := d.fn()
		if filepath.IsAbs(got) || strings.HasPrefix(got, "/tmp") {
			t.Errorf("%s with no HOME = %q, want a relative path", d.name, got)
		}
	}
}

func TestEnsure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	got, err := Ensure(dir)
	if err != nil || got != dir {
		t.Fatalf("Ensure = %q, %v", got, err)
	}
	if _, err := Ensure(dir); err != nil {
		t.Errorf("second Ensure: %v", err)
	}
}
