package fixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The snapshots of the captured hosts travel in the binary: the demo mode
// of the dashboard serves one of them on a host with no GPU.
//
//go:embed testdata/*.tar.gz
var snapshots embed.FS

// Extract unpacks the snapshot <name> into dir and returns the root path
// of the tree.
func Extract(name, dir string) (string, error) {
	b, err := snapshots.ReadFile("testdata/" + name + ".tar.gz")
	if err != nil {
		return "", fmt.Errorf("snapshot %s: %w", name, err)
	}
	if err := unpack(b, dir); err != nil {
		return "", fmt.Errorf("snapshot %s: %w", name, err)
	}
	return filepath.Join(dir, name), nil
}

// Names lists the snapshots that travel in the binary.
func Names() []string {
	entries, err := fs.ReadDir(snapshots, "testdata")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			out = append(out, strings.TrimSuffix(e.Name(), ".tar.gz"))
		}
	}
	sort.Strings(out)
	return out
}

// unpack writes the tarball into dir. A path that leaves dir is skipped.
func unpack(archive []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()
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
