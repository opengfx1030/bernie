package rocm

import "testing"

// TestRuntimeInstallPicksTheNestedTree checks the rule of the par1-cs25
// layout: the ROCm install the host runs is the TheRock tree nested in the
// package install, not the package around it.
func TestRuntimeInstallPicksTheNestedTree(t *testing.T) {
	root := par1Tree(t)
	got := RuntimeInstall(root)
	if got.Root != "/opt/rocm-7.2.0/core-7.14" {
		t.Errorf("RuntimeInstall = %+v, want the nested 7.14.1 tree", got)
	}
	if got.Version != "7.14.1" {
		t.Errorf("RuntimeInstall version = %q, want 7.14.1", got.Version)
	}
}

// TestRuntimeInstallWithoutATree checks that a host without ROCm has no
// install to point at.
func TestRuntimeInstallWithoutATree(t *testing.T) {
	if got := RuntimeInstall(t.TempDir()); got.Root != "" {
		t.Errorf("RuntimeInstall = %+v, want the zero install", got)
	}
}
