package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
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

func TestConventionalVenvProbesRunInParallel(t *testing.T) {
	resetVenvCache()
	home := t.TempDir()
	data := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("BERNIE_DATA_DIR", data)
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")

	old := venvCanImportVLLM
	defer func() { venvCanImportVLLM = old }()
	var mu sync.Mutex
	running, peak := 0, 0
	venvCanImportVLLM = func(string) bool {
		mu.Lock()
		running++
		if running > peak {
			peak = running
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return true
	}
	for _, dir := range []string{
		filepath.Join(home, ".venvs", "vllm-a"),
		filepath.Join(home, ".venvs", "vllm-b"),
		filepath.Join(home, ".venvs", "vllm-c"),
		filepath.Join(home, ".venvs", "vllm-d"),
	} {
		makeVenv(t, dir)
	}
	_ = (Config{}).ResolveVenvInfo("")
	if peak < 2 {
		t.Errorf("probes ran %d at a time, want more than one", peak)
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

func TestVenvPythonProbe(t *testing.T) {
	venv := t.TempDir()
	makeVenv(t, venv)
	if !venvCanImportVLLM(venv) {
		t.Error("python that can find vllm was rejected")
	}
	py := filepath.Join(venv, "bin", "python")
	if err := os.WriteFile(py, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if venvCanImportVLLM(venv) {
		t.Error("python that cannot find vllm was accepted")
	}
}
