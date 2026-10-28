package config

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/support/execx"
	"github.com/opengfx1030/bernie/internal/support/paths"
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
// conventional locations. A candidate needs bin/python and a Python that can
// find the vllm module. The probes run in parallel: a Python with a large
// site-packages directory takes seconds to start.
func conventionalVenvs() []string {
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
	ok := make([]bool, len(cands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, dir := range cands {
		wg.Add(1)
		go func(i int, dir string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok[i] = venvCanImportVLLM(dir)
		}(i, dir)
	}
	wg.Wait()
	var out []string
	for i, dir := range cands {
		if ok[i] {
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

// venvCanImportVLLM is replaced by tests. The real probe asks the venv's
// isolated Python whether it can find vllm; it does not import the module.
var venvCanImportVLLM = func(venv string) bool {
	py := filepath.Join(venv, "bin", "python")
	const script = `import importlib.util; raise SystemExit(0 if importlib.util.find_spec("vllm") else 1)`
	res, err := execx.Run(context.Background(), 10*time.Second, py, "-I", "-c", script)
	return err == nil && res.OK()
}
