package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func makeVenv(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "python"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestConventionalVenvDetection(t *testing.T) {
	resetVenvCache()
	home := t.TempDir()
	data := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("BERNIE_DATA_DIR", data)
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")

	old := venvCanImportVLLM
	defer func() { venvCanImportVLLM = old }()
	valid := map[string]bool{}
	venvCanImportVLLM = func(dir string) bool { return valid[dir] }

	paths := []string{
		filepath.Join(data, "venv"),
		filepath.Join(home, ".venvs", "vllm-one"),
		filepath.Join(home, "Apps", "vllm", "venv-two"),
	}
	for _, dir := range paths {
		makeVenv(t, dir)
		valid[dir] = true
	}
	valid[paths[1]] = false
	valid[paths[2]] = false
	// A directory without bin/python is not a candidate.
	makeVenv(t, filepath.Join(home, ".venvs", "vllm-no-python"))
	os.Remove(filepath.Join(home, ".venvs", "vllm-no-python", "bin", "python"))

	got := (Config{}).ResolveVenvInfo("")
	if got.Path != paths[0] || got.Source != VenvSourceConventional {
		t.Fatalf("one candidate = %+v, want %q", got, paths[0])
	}

	valid[paths[1]] = false
	resetVenvCache()
	got = (Config{}).ResolveVenvInfo("")
	if got.Path != paths[0] || got.Source != VenvSourceConventional {
		t.Fatalf("one valid candidate = %+v, want %q", got, paths[0])
	}

	valid[paths[1]] = true
	valid[paths[2]] = true
	resetVenvCache()
	got = (Config{}).ResolveVenvInfo("")
	want := []string{paths[1], paths[2], paths[0]}
	if got.Path != "" || got.Source != VenvSourceAmbiguous || !reflect.DeepEqual(got.Candidates, want) {
		t.Fatalf("ambiguous candidates = %+v, want %v", got, want)
	}
}

func TestConventionalVenvProbeDoesNotStartPython(t *testing.T) {
	resetVenvCache()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BERNIE_DATA_DIR", filepath.Join(home, "data"))
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")

	// A candidate whose Python would write a marker when it starts. The
	// probe must not start it: a venv that imports vLLM at startup takes
	// seconds to start.
	dir := filepath.Join(home, ".venvs", "vllm-a")
	makeVenv(t, dir)
	marker := filepath.Join(home, "started")
	py := filepath.Join(dir, "bin", "python")
	if err := os.WriteFile(py, []byte("#!/bin/sh\ntouch "+marker+"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = (Config{}).ResolveVenvInfo("")
	if _, err := os.Stat(marker); err == nil {
		t.Error("the candidate probe started the venv's Python")
	}
}

func TestConventionalVenvProbeIsCached(t *testing.T) {
	resetVenvCache()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BERNIE_DATA_DIR", filepath.Join(home, "data"))
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")

	old := venvCanImportVLLM
	defer func() { venvCanImportVLLM = old }()
	calls := 0
	venvCanImportVLLM = func(string) bool {
		calls++
		return true
	}
	makeVenv(t, filepath.Join(home, ".venvs", "vllm-a"))
	(Config{}).ResolveVenvInfo("")
	(Config{}).ResolveVenvInfo("")
	if calls != 1 {
		t.Errorf("the candidates were probed %d times, want 1", calls)
	}
}

func TestVenvModuleProbe(t *testing.T) {
	venv := t.TempDir()
	makeVenv(t, venv)
	if venvCanImportVLLM(venv) {
		t.Error("a venv without vllm was accepted")
	}
	site := filepath.Join(venv, "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(filepath.Join(site, "vllm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !venvCanImportVLLM(venv) {
		t.Error("a venv with the vllm package was rejected")
	}
}
