package pyenv

import (
	"os"
	"path/filepath"
	"testing"
)

// makeVenv builds a venv tree with a site-packages directory.
func makeVenv(t *testing.T, dir string) string {
	t.Helper()
	site := filepath.Join(dir, "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	return site
}

func TestSitePackages(t *testing.T) {
	venv := t.TempDir()
	makeVenv(t, venv)
	got := SitePackages(venv)
	if len(got) != 1 || filepath.Base(got[0]) != "site-packages" {
		t.Fatalf("site-packages = %v, want one directory", got)
	}
	if SitePackages("") != nil {
		t.Error("an empty venv has site-packages")
	}
}

func TestHasModule(t *testing.T) {
	venv := t.TempDir()
	site := makeVenv(t, venv)
	if HasModule(venv, "vllm") {
		t.Error("a venv without vllm was accepted")
	}
	if err := os.MkdirAll(filepath.Join(site, "vllm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !HasModule(venv, "vllm") {
		t.Error("a venv with the vllm package was rejected")
	}
}

func TestHasModuleFile(t *testing.T) {
	venv := t.TempDir()
	site := makeVenv(t, venv)
	if err := os.WriteFile(filepath.Join(site, "vllm.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasModule(venv, "vllm") {
		t.Error("a venv with the vllm module file was rejected")
	}
}

func TestHasModuleEditableInstall(t *testing.T) {
	venv := t.TempDir()
	site := makeVenv(t, venv)
	name := "__editable__.vllm-0.31.0+rdna.pth"
	if err := os.WriteFile(filepath.Join(site, name), []byte("import finder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasModule(venv, "vllm") {
		t.Error("a venv with an editable vllm install was rejected")
	}
}

func TestHasModuleIgnoresOtherPackages(t *testing.T) {
	venv := t.TempDir()
	site := makeVenv(t, venv)
	if err := os.MkdirAll(filepath.Join(site, "vllm-0.31.0.dist-info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "other.pth"), []byte("import x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if HasModule(venv, "vllm") {
		t.Error("a venv without vllm was accepted")
	}
}

func TestHasModuleWithSystemSitePackages(t *testing.T) {
	venv := t.TempDir()
	makeVenv(t, venv)
	base := t.TempDir()
	site := filepath.Join(base, "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(filepath.Join(site, "vllm"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "home = " + filepath.Join(base, "bin") + "\ninclude-system-site-packages = true\n"
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasModule(venv, "vllm") {
		t.Error("a venv with the system site-packages was rejected")
	}
}

func TestHasModuleWithoutSystemSitePackages(t *testing.T) {
	venv := t.TempDir()
	makeVenv(t, venv)
	base := t.TempDir()
	site := filepath.Join(base, "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(filepath.Join(site, "vllm"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "home = " + filepath.Join(base, "bin") + "\ninclude-system-site-packages = false\n"
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if HasModule(venv, "vllm") {
		t.Error("a venv without the system site-packages was accepted")
	}
}
