package rocm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVenv writes a venv whose Python prints the probe line and counts its
// runs in the file at counter.
func fakeVenv(t *testing.T, venv, counter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(venv, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `BERNIE_JSON:{"python": "3.12.13", "torch": "2.12.0+rocm7.14.0", "hip": "7.14.60850", "device_count": 2, "vllm": "0.31.0+rdna", "rdna_ops": 42}`
	script := "#!/bin/sh\necho run >> " + counter + "\necho '" + line + "'\n"
	if err := os.WriteFile(filepath.Join(venv, "bin", "python"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runs counts the runs of the fake Python.
func runs(t *testing.T, counter string) int {
	t.Helper()
	b, err := os.ReadFile(counter)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "run")
}

func TestProbeVenvCachesTheResult(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	venv := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs")
	fakeVenv(t, venv, counter)

	rep := ProbeVenv(context.Background(), venv)
	if rep.Torch != "2.12.0+rocm7.14.0" {
		t.Fatalf("the probe = %+v, want the torch version", rep)
	}
	rep = ProbeVenv(context.Background(), venv)
	if rep.Torch != "2.12.0+rocm7.14.0" {
		t.Fatalf("the cached probe = %+v, want the torch version", rep)
	}
	if n := runs(t, counter); n != 1 {
		t.Errorf("the probe ran %d times, want 1", n)
	}
}

func TestProbeVenvProbesAgainWhenTheVenvChanges(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	venv := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs")
	fakeVenv(t, venv, counter)

	ProbeVenv(context.Background(), venv)
	// A rebuilt venv has a new pyvenv.cfg: the fact is a miss.
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte("version = 3.12.13\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ProbeVenv(context.Background(), venv)
	if n := runs(t, counter); n != 2 {
		t.Errorf("the probe ran %d times, want 2", n)
	}
}

func TestProbeVenvFailureIsNotCached(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	venv := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs")
	fakeVenv(t, venv, counter)
	// The Python fails.
	script := "#!/bin/sh\necho run >> " + counter + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(venv, "bin", "python"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	rep := ProbeVenv(context.Background(), venv)
	if rep.Err == "" {
		t.Error("a failed probe has no error")
	}
	rep = ProbeVenv(context.Background(), venv)
	if rep.Err == "" {
		t.Error("the second failed probe has no error")
	}
	if n := runs(t, counter); n != 2 {
		t.Errorf("the failed probe ran %d times, want 2: a failure is not a fact", n)
	}
}
