package tuning

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/opengfx1030/bernie/internal/support/paths"
	tunedmoe "github.com/opengfx1030/bernie/tuned-moe"
)

// TunedMoEDir is the root MaterializeTunedMoE writes below: <data>/tuned-moe.
func TunedMoEDir() string { return filepath.Join(paths.Data(), "tuned-moe") }

// MaterializeTunedMoE writes the embedded tuned MoE configs of the
// architecture gfx (for example "gfx1030") into <data>/tuned-moe/<arch> and
// returns that folder. Only files whose content differs are written. It
// returns "" when no embedded folder covers gfx.
func MaterializeTunedMoE(gfx string) (string, error) {
	return materializeTunedMoE(tunedmoe.FS, gfx, TunedMoEDir())
}

func materializeTunedMoE(fsys fs.FS, gfx, root string) (string, error) {
	arch, err := tunedMoEArch(fsys, gfx)
	if err != nil || arch == "" {
		return "", err
	}
	dir := filepath.Join(root, arch)
	if _, err := paths.Ensure(dir); err != nil {
		return "", err
	}
	files, err := fs.Glob(fsys, path.Join(arch, "*.json"))
	if err != nil {
		return "", err
	}
	for _, f := range files {
		want, err := fs.ReadFile(fsys, f)
		if err != nil {
			return "", err
		}
		if err := writeIfChanged(filepath.Join(dir, path.Base(f)), want); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// tunedMoEArch returns the folder of fsys that covers gfx: the longest
// folder name that is a prefix of gfx, or "" when none is.
func tunedMoEArch(fsys fs.FS, gfx string) (string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return "", err
	}
	best := ""
	for _, e := range entries {
		if e.IsDir() && gfx != "" && strings.HasPrefix(gfx, e.Name()) && len(e.Name()) > len(best) {
			best = e.Name()
		}
	}
	return best, nil
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
