package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/execx"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Doctor runs the host checks of the given categories (all when empty).
// venv is the --venv flag value ("" = config, then environment).
func Doctor(ctx context.Context, categories []string, venv string) (doctor.Report, error) {
	cfg, err := config.Load()
	if err != nil {
		return doctor.Report{}, err
	}
	set := recipe.Load()
	v := cfg.ResolveVenvInfo(venv)
	opts := doctor.Options{
		Root: root, Venv: v.Path, VenvSource: v.Source, VenvCandidates: v.Candidates,
		Categories: categories, HFHome: cfg.ResolveHFHome(), ValidatedArchs: recipe.ValidatedArchs(set),
	}
	if len(categories) == 0 || slices.Contains(categories, "topo.p2p") {
		vis, _ := gpu.Visible(root)
		env, _ := recipeEnv(set, "", vis.GPUs)
		opts.RecipeEnv = env
	}
	return doctor.Run(ctx, opts), nil
}

// LogReport is the `doctor --log` and `doctor --run` document (spec §5.3).
type LogReport struct {
	Matches []logdoc.Match `json:"matches"`
}

// NewLogReport wraps matches; no match is an empty list, never null.
func NewLogReport(matches []logdoc.Match) LogReport {
	if matches == nil {
		matches = []logdoc.Match{}
	}
	return LogReport{Matches: matches}
}

// RunLog matches the log of the run name against the known-failure catalog.
// A host run's log is the file named by the "log" field of run.json
// (serve.log in the run directory when the field is empty or the file is
// gone). A container run's log comes from the engine.
func RunLog(ctx context.Context, name string) (LogReport, error) {
	rules, err := logdoc.Load()
	if err != nil {
		return LogReport{}, err
	}
	matches, err := scanRunLog(ctx, rules, name)
	if err != nil {
		return LogReport{}, err
	}
	return NewLogReport(matches), nil
}

func scanRunLog(ctx context.Context, rules []logdoc.Rule, name string) ([]logdoc.Match, error) {
	dir := filepath.Join(paths.State(), "runs", name)
	b, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no run named %q (looked in %s)", name, dir)
		}
		return nil, err
	}
	var rec struct {
		Engine    string `json:"engine"`
		Container string `json:"container"`
		Log       string `json:"log"`
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, fmt.Errorf("run %s: bad run.json: %w", name, err)
	}
	if rec.Engine != "" && rec.Container != "" {
		res, err := execx.Run(ctx, 60*time.Second, rec.Engine, "logs", rec.Container)
		if err != nil {
			return nil, err
		}
		if !res.OK() {
			return nil, fmt.Errorf("%s logs %s failed: %s", rec.Engine, rec.Container, execx.FirstLine(res.Stderr))
		}
		return logdoc.Scan(io.MultiReader(strings.NewReader(res.Stdout), strings.NewReader(res.Stderr)), rules), nil
	}
	path := rec.Log
	if _, err := os.Stat(path); path == "" || err != nil {
		path = filepath.Join(dir, "serve.log")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return logdoc.Scan(f, rules), nil
}
