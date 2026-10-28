package recipe

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadOrderAndErrors(t *testing.T) {
	cfg := t.TempDir()
	env := t.TempDir()
	extra := t.TempDir()
	write := func(dir, name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.MkdirAll(filepath.Join(cfg, "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(cfg, "recipes"), "full.yaml", "schema: 1\nextends: [full]\nname: from-config\n")
	write(filepath.Join(cfg, "recipes"), "mine.yaml", strings.Replace(leaf, "Leaf", "config", 1))
	write(env, "mine.yaml", strings.Replace(leaf, "Leaf", "env", 1))
	write(env, "broken.yml", "schema: 1\nwhat: 1\n")
	extraFile := write(extra, "mine.yaml", strings.Replace(leaf, "Leaf", "extra", 1))
	t.Setenv("RDNACTL_CONFIG_DIR", cfg)
	t.Setenv("RDNACTL_RECIPES", env+string(os.PathListSeparator)+filepath.Join(env, "missing"))

	s := Load(extraFile)
	if got := s.Recipes["mine"].Name; got != "extra" {
		t.Errorf("mine = %q, want the extra file to win", got)
	}
	if !strings.HasPrefix(s.Recipes["27b-awq"].Source, "builtin:") {
		t.Errorf("built-in source = %s", s.Recipes["27b-awq"].Source)
	}
	var srcs []string
	for _, e := range s.Errors {
		srcs = append(srcs, filepath.Base(e.Source))
	}
	if !reflect.DeepEqual(srcs, []string{"broken.yml", "missing"}) {
		t.Errorf("errors = %v", s.Errors)
	}
	// A user file with a built-in id replaces it; extending its own id is a cycle.
	if _, err := s.Resolve("full"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("shadowed self-extend: %v", err)
	}
}

func TestLoadWalksDirectoriesRecursively(t *testing.T) {
	cfg, env := t.TempDir(), t.TempDir()
	t.Setenv("RDNACTL_CONFIG_DIR", cfg)
	t.Setenv("RDNACTL_RECIPES", env)
	write := func(rel, body string) {
		p := filepath.Join(env, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("team/a/deep.yaml", strings.Replace(leaf, "Leaf", "deep", 1))
	write("team/_fragments/net/_net.yml", "schema: 1\nenv: {A: \"1\"}\n")
	write("team/notes.txt", "not a recipe")
	s := Load()
	if r := s.Recipes["deep"]; r == nil || r.Source != filepath.Join(env, "team", "a", "deep.yaml") {
		t.Fatalf("deep = %+v", r)
	}
	if r := s.Recipes["_net"]; r == nil || !r.Fragment {
		t.Fatalf("_net = %+v", r)
	}
	for _, e := range s.Errors {
		t.Errorf("unexpected load error: %s: %s", e.Source, e.Err)
	}
}

func TestLoadDuplicateIDInOneTreeNamesBothPaths(t *testing.T) {
	cfg, env := t.TempDir(), t.TempDir()
	t.Setenv("RDNACTL_CONFIG_DIR", cfg)
	t.Setenv("RDNACTL_RECIPES", env)
	for _, rel := range []string{"a/same.yaml", "b/same.yaml"} {
		p := filepath.Join(env, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Replace(leaf, "Leaf", rel, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := Load()
	var found *LoadError
	for i, e := range s.Errors {
		if strings.Contains(e.Err, "duplicate recipe id") {
			found = &s.Errors[i]
		}
	}
	if found == nil {
		t.Fatalf("no duplicate error in %v", s.Errors)
	}
	first, second := filepath.Join(env, "a", "same.yaml"), filepath.Join(env, "b", "same.yaml")
	if found.Source != second || !strings.Contains(found.Err, first) {
		t.Errorf("error = %+v, want source %s naming %s", *found, second, first)
	}
	if got := s.Recipes["same"].Source; got != first {
		t.Errorf("kept %s, want the first file %s", got, first)
	}
}

func TestValidatedArchs(t *testing.T) {
	got := ValidatedArchs(Load())
	if len(got) == 0 {
		t.Fatal("no validated architecture in the built-in recipes")
	}
	for i, a := range got {
		if i > 0 && got[i-1] >= a {
			t.Errorf("not sorted and distinct: %v", got)
		}
	}
	t.Logf("validated archs: %v", got)
}
