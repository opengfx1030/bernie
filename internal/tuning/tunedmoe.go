package tuning

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/opengfx1030/rdnactl/internal/paths"
	tunedmoe "github.com/opengfx1030/rdnactl/tuned-moe"
)

// TunedMoEDir is where MaterializeTunedMoE writes: <data>/tuned-moe.
func TunedMoEDir() string { return filepath.Join(paths.Data(), "tuned-moe") }

// MaterializeTunedMoE writes the embedded tuned MoE configs into
// TunedMoEDir (only files whose content differs) and returns the directory.
func MaterializeTunedMoE() (string, error) {
	return materializeFlat(tunedmoe.FS, TunedMoEDir())
}

// materializeFlat copies the top-level files of fsys into dir idempotently.
func materializeFlat(fsys fs.FS, dir string) (string, error) {
	if _, err := paths.Ensure(dir); err != nil {
		return "", err
	}
	files, err := fs.Glob(fsys, "*")
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if filepath.Ext(f) == ".go" {
			continue
		}
		want, err := fs.ReadFile(fsys, f)
		if err != nil {
			return "", err
		}
		if err := writeIfChanged(filepath.Join(dir, f), want); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// writeIfChanged writes want to dst unless dst already holds it, then
// verifies the content.
func writeIfChanged(dst string, want []byte) error {
	if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, want) {
		return nil
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, want, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	if have, err := os.ReadFile(dst); err != nil || !bytes.Equal(have, want) {
		return fmt.Errorf("verify %s failed", dst)
	}
	return nil
}
