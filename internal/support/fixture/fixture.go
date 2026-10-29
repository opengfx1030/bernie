// Package fixture unpacks host snapshots made by scripts/capture-fixture.sh
// for tests and for the demo mode of the dashboard. A snapshot is
// testdata/<name>.tar.gz next to this file; Root extracts it into a
// per-test temp dir and returns the root path to pass to gpu.List,
// topo.Discover, host.Probe and friends. Snapshots are tarballs because
// sysfs paths contain ':' which Go module zips reject.
package fixture

import (
	"os"
	"path/filepath"
)

// TB is what the snapshot helpers need from their caller: the helper
// marker, a temp dir and the two ways to stop. *testing.T satisfies it,
// and so does the demo mode, which unpacks a snapshot without a test.
type TB interface {
	Helper()
	TempDir() string
	Fatalf(format string, args ...any)
	Fatal(args ...any)
}

// Root extracts the snapshot <name> and returns its root path.
func Root(tb TB, name string) string {
	tb.Helper()
	dir := tb.TempDir()
	root, err := Extract(name, dir)
	if err != nil {
		tb.Fatalf("fixture %s: %v", name, err)
	}
	return root
}

// Write creates files under root for synthetic test trees: path -> content.
// It also writes the SYNTHETIC marker at the root of the tree, so that a
// hand-written tree is never mistaken for a captured one (spec §2.1).
func Write(tb TB, root string, files map[string]string) {
	tb.Helper()
	for p, content := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	marker := filepath.Join(root, "SYNTHETIC")
	if _, err := os.Stat(marker); err != nil {
		if err := os.WriteFile(marker, []byte("Synthetic tree, written by hand in a test.\n"), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}
