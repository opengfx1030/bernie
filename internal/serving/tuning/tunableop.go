// Package tuning selects and materializes TunableOp profiles (spec §9.1) and
// reads or sets the GPU power cap (spec §9.2). It never prints.
package tuning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/support/paths"
	"github.com/opengfx1030/bernie/tunableop"
)

// SHAPrefix is the number of hex digits of the librocblas.so.5 sha256 that
// auto-selection compares.
const SHAPrefix = 12

// ContainerRowsDir is where the container runner mounts a profile's rows.
const ContainerRowsDir = "/bernie/tunableop"

// rowsFile is the PYTORCH_TUNABLEOP_FILENAME base name. PyTorch inserts the
// device ordinal before the extension (tunableop_results0.csv, ...).
const rowsFile = "tunableop_results.csv"

// Profile is one TunableOp row set, valid for one rocBLAS build on the
// architectures in Arch. It is read from the profile's profile.json
// (tunableop/<arch>/<name>/profile.json).
type Profile struct {
	Name string `json:"name"`
	// Dir is the profile folder inside the embedded tree (<arch>/<name>).
	Dir string `json:"-"`
	// Arch lists the gfx names or prefixes the rows were tuned for.
	Arch        []string `json:"arch"`
	ROCBLAS     string   `json:"rocblas"`
	ROCm        string   `json:"rocm"`
	Torch       string   `json:"torch"`
	LibSHA256   string   `json:"lib_sha256"`
	Status      string   `json:"status"`
	RowsPerRank int      `json:"rows_per_rank"`
	Venv        string   `json:"venv,omitempty"`
	Covers      []string `json:"covers"`
	// Ranks lists the embedded rank files (tunableop_results<N>.csv).
	Ranks []string `json:"ranks"`
}

// manifest is the profile manifest file name.
const manifest = "profile.json"

// Profiles returns the embedded profiles, sorted by name. They are found by
// walking the embedded tree for profile.json manifests.
func Profiles() ([]Profile, error) {
	return profilesFrom(tunableop.FS)
}

func profilesFrom(fsys fs.FS) ([]Profile, error) {
	var out []Profile
	seen := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != manifest {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var pr Profile
		if err := json.Unmarshal(b, &pr); err != nil {
			return fmt.Errorf("tunableop profile %s: %w", p, err)
		}
		pr.Dir = path.Dir(p)
		switch {
		case pr.Name == "" || pr.Name != path.Base(pr.Dir):
			return fmt.Errorf("tunableop profile %s: name %q must equal the folder name %q", p, pr.Name, path.Base(pr.Dir))
		case len(pr.LibSHA256) < SHAPrefix:
			return fmt.Errorf("tunableop profile %s: lib_sha256 is required", p)
		case len(pr.Arch) == 0:
			return fmt.Errorf("tunableop profile %s: arch is required (gfx names or prefixes)", p)
		}
		if first, dup := seen[pr.Name]; dup {
			return fmt.Errorf("tunableop profile %q is defined twice: %s and %s", pr.Name, first, p)
		}
		seen[pr.Name] = p
		ranks, _ := fs.Glob(fsys, path.Join(pr.Dir, "tunableop_results*.csv"))
		for i := range ranks {
			ranks[i] = path.Base(ranks[i])
		}
		sort.Strings(ranks)
		pr.Ranks = ranks
		out = append(out, pr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// HashLib returns the hex sha256 of a file (librocblas.so.5).
func HashLib(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Select picks a profile. archs are the gfx targets of the selected GPUs;
// a profile fits when every one of them is named by the profile's Arch. An
// empty archs does not filter by architecture.
//   - explicit != "": that profile. It must exist and fit archs. When libSHA
//     is known, its first 12 hex digits must match the profile (a mismatch
//     is an error: solver IDs are build-specific). An empty libSHA trusts
//     the name (container mode: the recipe names the profile of its pinned
//     image).
//   - explicit == "": the profile whose lib_sha256 prefix equals libSHA's and
//     whose Arch fits archs. No match returns nil, nil: use the per-user
//     fallback (PerUserFile).
func Select(libSHA, explicit string, archs []string) (*Profile, error) {
	ps, err := Profiles()
	if err != nil {
		return nil, err
	}
	return selectFrom(ps, libSHA, explicit, archs)
}

func selectFrom(ps []Profile, libSHA, explicit string, archs []string) (*Profile, error) {
	libSHA = strings.ToLower(libSHA)
	if explicit != "" {
		for i := range ps {
			if ps[i].Name != explicit {
				continue
			}
			p := ps[i]
			if libSHA != "" && !sameBuild(p.LibSHA256, libSHA) {
				return nil, fmt.Errorf("tunableop profile %s was tuned for rocBLAS %s, but the loaded librocblas.so.5 is %s; solver IDs are build-specific",
					p.Name, short(p.LibSHA256), short(libSHA))
			}
			if !p.Fits(archs) {
				return nil, fmt.Errorf("tunableop profile %s was tuned for %s, but the selected GPUs are %s",
					p.Name, strings.Join(p.Arch, "/"), strings.Join(archs, "/"))
			}
			return &p, nil
		}
		return nil, fmt.Errorf("tunableop profile %q is not registered", explicit)
	}
	if libSHA == "" {
		return nil, nil
	}
	for i := range ps {
		if sameBuild(ps[i].LibSHA256, libSHA) && ps[i].Fits(archs) {
			p := ps[i]
			return &p, nil
		}
	}
	return nil, nil
}

// Fits reports whether every gfx target in archs is named by p.Arch. An
// empty archs fits every profile.
func (p Profile) Fits(archs []string) bool {
	for _, a := range archs {
		if !gpu.ArchMatch(p.Arch, a) {
			return false
		}
	}
	return true
}

func sameBuild(a, b string) bool {
	return len(a) >= SHAPrefix && len(b) >= SHAPrefix && strings.EqualFold(a[:SHAPrefix], b[:SHAPrefix])
}

func short(s string) string {
	if len(s) > SHAPrefix {
		return s[:SHAPrefix]
	}
	return s
}

// RowsDir is the materialized rows directory of a profile:
// <data>/tunableop/<name>. Profile names are unique across architectures.
func RowsDir(name string) string {
	return filepath.Join(paths.Data(), "tunableop", name)
}

// PerUserFile is the fallback rows file when no profile matches:
// $HOME/.cache/tunableop/tunableop_results.csv.
func PerUserFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".cache", "tunableop", rowsFile)
}

// Materialize writes the embedded rank files of p into RowsDir(p.Name). It
// rewrites a file only when its content differs from the embedded rows, then
// verifies every file. It returns the directory.
func Materialize(p *Profile) (string, error) {
	if p == nil {
		return "", fmt.Errorf("materialize: no profile")
	}
	return materialize(tunableop.FS, p, RowsDir(p.Name))
}

func materialize(fsys fs.FS, p *Profile, dir string) (string, error) {
	if _, err := paths.Ensure(dir); err != nil {
		return "", err
	}
	files, err := fs.Glob(fsys, path.Join(p.Dir, "*"))
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("tunableop profile %s: no embedded rows under %s", p.Name, p.Dir)
	}
	for _, f := range files {
		want, err := fs.ReadFile(fsys, f)
		if err != nil {
			return "", err
		}
		if err := writeIfChanged(filepath.Join(dir, path.Base(f)), want); err != nil {
			return "", fmt.Errorf("tunableop profile %s: %w", p.Name, err)
		}
	}
	return dir, nil
}

// Env returns the TunableOp environment (spec §9.1). rowsDir is the
// directory that holds p's rows as the vLLM process sees it (RowsDir on the
// host, ContainerRowsDir in a container). With p == nil or tuning, the rows
// file is the per-user fallback, where online tuning writes.
func Env(p *Profile, rowsDir string, tuning bool) map[string]string {
	env := map[string]string{
		"PYTORCH_TUNABLEOP_ENABLED":           "1",
		"PYTORCH_TUNABLEOP_TUNING":            "0",
		"PYTORCH_TUNABLEOP_HIPBLASLT_ENABLED": "0",
	}
	if tuning {
		env["PYTORCH_TUNABLEOP_TUNING"] = "1"
	}
	if p == nil || tuning || rowsDir == "" {
		env["PYTORCH_TUNABLEOP_FILENAME"] = PerUserFile()
	} else {
		env["PYTORCH_TUNABLEOP_FILENAME"] = path.Join(filepath.ToSlash(rowsDir), rowsFile)
	}
	return env
}
