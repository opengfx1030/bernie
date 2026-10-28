// Package fixture unpacks host snapshots made by scripts/capture-fixture.sh
// for tests. A snapshot is testdata/<name>.tar.gz next to this file; Root
// extracts it into a per-test temp dir and returns the root path to pass to
// gpu.List, topo.Discover, host.Probe and friends. Snapshots are tarballs
// because sysfs paths contain ':' which Go module zips reject.
package fixture

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Root extracts testdata/<name>.tar.gz and returns the snapshot root.
func Root(tb testing.TB, name string) string {
	tb.Helper()
	_, self, _, _ := runtime.Caller(0)
	archive := filepath.Join(filepath.Dir(self), "testdata", name+".tar.gz")
	dir := tb.TempDir()
	if err := extract(archive, dir); err != nil {
		tb.Fatalf("fixture %s: %v", name, err)
	}
	return filepath.Join(dir, name)
}

func extract(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(h.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}

// Write creates files under root for synthetic test trees: path -> content.
// It also writes the SYNTHETIC marker at the root of the tree, so that a
// hand-written tree is never mistaken for a captured one (spec §2.1).
func Write(tb testing.TB, root string, files map[string]string) {
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
