package config

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/opengfx1030/bernie/internal/support/paths"
	"github.com/opengfx1030/bernie/internal/support/pyenv"
)

// Venv resolution sources (spec §6.2).
const (
	VenvSourceFlag         = "flag"
	VenvSourceBernieEnv    = "$BERNIE_VENV"
	VenvSourceConfig       = "config"
	VenvSourceVirtualEnv   = "$VIRTUAL_ENV"
	VenvSourceConventional = "conventional"
	VenvSourceAmbiguous    = "ambiguous"
	VenvSourceNone         = "none"
)

// VenvResolution is the venv bernie will use and where that choice came
// from. Candidates is non-empty only when several conventional venvs exist.
type VenvResolution struct {
	Path       string   `json:"path,omitempty"`
	Source     string   `json:"source"`
	Candidates []string `json:"candidates,omitempty"`
}

// ResolveVenvInfo applies the order of spec §6.2: flag, $BERNIE_VENV,
// config, $VIRTUAL_ENV, then one unambiguous conventional venv.
func (c Config) ResolveVenvInfo(flag string) VenvResolution {
	for _, v := range []struct {
		path, source string
	}{
		{flag, VenvSourceFlag},
		{os.Getenv("BERNIE_VENV"), VenvSourceBernieEnv},
		{c.Venv, VenvSourceConfig},
		{os.Getenv("VIRTUAL_ENV"), VenvSourceVirtualEnv},
	} {
		if v.path != "" {
			return VenvResolution{Path: expandHome(v.path), Source: v.source}
		}
	}
	candidates := conventionalVenvs()
	switch len(candidates) {
	case 0:
		return VenvResolution{Source: VenvSourceNone}
	case 1:
		return VenvResolution{Path: candidates[0], Source: VenvSourceConventional}
	default:
		return VenvResolution{Source: VenvSourceAmbiguous, Candidates: candidates}
	}
}

// conventionalVenvs returns the valid vLLM venvs in the short list of
// conventional locations. A candidate needs bin/python and a site-packages
// that holds the vllm module. The result is cached: one process reads the
// candidates once.
func conventionalVenvs() []string {
	venvMu.Lock()
	defer venvMu.Unlock()
	if venvCached {
		return venvCache
	}
	out := probeConventionalVenvs()
	venvCache, venvCached = out, true
	return out
}

var (
	venvMu     sync.Mutex
	venvCache  []string
	venvCached bool
)

// resetVenvCache forgets the probed candidates. Only the tests need it.
func resetVenvCache() {
	venvMu.Lock()
	venvCache, venvCached = nil, false
	venvMu.Unlock()
}

func probeConventionalVenvs() []string {
	home, _ := os.UserHomeDir()
	patterns := []string{filepath.Join(paths.Data(), "venv")}
	if home != "" {
		patterns = append(patterns,
			filepath.Join(home, ".venvs", "vllm*"),
			filepath.Join(home, "Apps", "vllm", "venv*"))
	}
	seen := map[string]bool{}
	var cands []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, dir := range matches {
			dir = filepath.Clean(dir)
			key := dir
			if real, err := filepath.EvalSymlinks(dir); err == nil {
				key = real
			}
			if seen[key] || !venvHasPython(dir) {
				continue
			}
			seen[key] = true
			cands = append(cands, dir)
		}
	}
	var out []string
	for _, dir := range cands {
		if venvCanImportVLLM(dir) {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

func venvHasPython(venv string) bool {
	st, err := os.Stat(filepath.Join(venv, "bin", "python"))
	return err == nil && st.Mode().IsRegular()
}

// venvCanImportVLLM is replaced by tests. The real probe reads the venv's
// site-packages: it does not start the venv's Python, which takes seconds
// when the venv imports vLLM at startup.
var venvCanImportVLLM = func(venv string) bool { return pyenv.HasModule(venv, "vllm") }
