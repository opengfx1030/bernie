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

	"github.com/opengfx1030/rdnactl/internal/paths"
	"github.com/opengfx1030/rdnactl/tunableop"
)

// SHAPrefix is the number of hex digits of the librocblas.so.5 sha256 that
// auto-selection compares.
const SHAPrefix = 12

// ContainerRowsDir is where the container runner mounts a profile's rows.
const ContainerRowsDir = "/rdnactl/tunableop"

// rowsFile is the PYTORCH_TUNABLEOP_FILENAME base name. PyTorch inserts the
// device ordinal before the extension (tunableop_results0.csv, ...).
const rowsFile = "tunableop_results.csv"

// Profile is one registered TunableOp row set, valid for one rocBLAS build.
type Profile struct {
	Name        string   `json:"name"`
	Dir         string   `json:"dir"`
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

// registry is the shape of tunableop/profiles.json.
type registry struct {
	Profiles map[string]Profile `json:"profiles"`
}

// Profiles returns the embedded registry, sorted by name.
func Profiles() ([]Profile, error) {
	return profilesFrom(tunableop.FS)
}

func profilesFrom(fsys fs.FS) ([]Profile, error) {
	b, err := fs.ReadFile(fsys, "profiles.json")
	if err != nil {
		return nil, fmt.Errorf("tunableop registry: %w", err)
	}
	var reg registry
	if err := json.Unmarshal(b, &reg); err != nil {
		return nil, fmt.Errorf("tunableop registry: %w", err)
	}
	out := make([]Profile, 0, len(reg.Profiles))
	for name, p := range reg.Profiles {
		p.Name = name
		if p.Dir == "" || len(p.LibSHA256) < SHAPrefix {
			return nil, fmt.Errorf("tunableop registry: profile %q needs dir and lib_sha256", name)
		}
		ranks, _ := fs.Glob(fsys, path.Join(p.Dir, "tunableop_results*.csv"))
		for i := range ranks {
			ranks[i] = path.Base(ranks[i])
		}
		sort.Strings(ranks)
		p.Ranks = ranks
		out = append(out, p)
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

// Select picks a profile.
//   - explicit != "": that profile. It must exist. When libSHA is known, its
//     first 12 hex digits must match the profile (a mismatch is an error:
//     solver IDs are build-specific). An empty libSHA trusts the name
//     (container mode: the recipe names the profile of its pinned image).
//   - explicit == "": the profile whose lib_sha256 prefix equals libSHA's.
//     No match returns nil, nil: use the per-user fallback (PerUserFile).
func Select(libSHA string, explicit string) (*Profile, error) {
	ps, err := Profiles()
	if err != nil {
		return nil, err
	}
	return selectFrom(ps, libSHA, explicit)
}

func selectFrom(ps []Profile, libSHA, explicit string) (*Profile, error) {
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
			return &p, nil
		}
		return nil, fmt.Errorf("tunableop profile %q is not registered", explicit)
	}
	if libSHA == "" {
		return nil, nil
	}
	for i := range ps {
		if sameBuild(ps[i].LibSHA256, libSHA) {
			p := ps[i]
			return &p, nil
		}
	}
	return nil, nil
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
// <data>/tunableop/<name>.
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
