package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// ModelsReport is the `models --json` document (spec §11, ia.md R31). It
// names the ids the running server serves and the repos the Hugging Face
// cache on this host holds, with the snapshot commit each repo points at
// and the path of the cache that was read.
type ModelsReport struct {
	// Running is the list of model ids /v1/models answered when a run is
	// serving on this host. Empty when nothing serves; RunningURL then names
	// the server bernie tried.
	Running    []string `json:"running"`
	RunningURL string   `json:"running_url,omitempty"`
	// Cached is the repos the Hugging Face cache on this host holds, in
	// "org/name" form. Each entry carries the snapshot commit and the path
	// of the hub cache it came from.
	Cached []CachedModel `json:"cached"`
	// HubCache is the resolved hub cache root the report was read from:
	// config hf_home, then $HF_HOME, then ~/.cache/huggingface.
	HubCache string `json:"hub_cache"`
	// HubSource says where the resolved value came from: "config",
	// "$HF_HOME" or "default".
	HubSource string `json:"hub_source"`
	// LiveRun is the name and url of the run that served Running, when one
	// is up. Empty when nothing serves.
	LiveRun string `json:"live_run,omitempty"`
	// LiveError is why nothing was read from the live server, when there
	// is a server but /v1/models did not answer. Empty on success.
	LiveError string `json:"live_error,omitempty"`
}

// CachedModel is one repo the Hugging Face cache holds: the id, the
// snapshot commit and the hub cache root.
type CachedModel struct {
	Repo        string `json:"repo"`
	SnapshotSHA string `json:"snapshot_sha"`
	HubCache    string `json:"hub_cache"`
}

// Models answers the question of R31: the ids the running server serves and
// the repos the HF cache on this host holds. A failure to read the running
// server's /v1/models is reported in LiveError and does not stop the rest.
func Models(ctx context.Context) (ModelsReport, error) {
	cfg, err := config.Load()
	if err != nil {
		return ModelsReport{}, err
	}
	hub, src := resolveHFHome(&cfg)
	rep := ModelsReport{
		Running:   []string{},
		Cached:    []CachedModel{},
		HubCache:  hub,
		HubSource: src,
	}
	if run, err := onboarding.LiveRun(ctx); err == nil {
		rep.LiveRun = run.Name
		rep.RunningURL = run.URL
		ids, mErr := vllm.New(run.URL).Models(ctx)
		if mErr == nil {
			rep.Running = ids
		} else {
			rep.LiveError = mErr.Error()
		}
	} else if !errors.Is(err, onboarding.ErrNoLiveRun) {
		// A real failure (not just "nothing serves") goes into LiveError so
		// the cached part of the report is still useful.
		rep.LiveError = err.Error()
	}
	if hub != "" {
		if cs, err := readCachedModels(hub); err == nil {
			rep.Cached = cs
		}
	}
	return rep, nil
}

// resolveHFHome returns the path the report was read from and the source of
// that value: "config" (config hf_home), "$HF_HOME" (the environment) or
// "default" (~/.cache/huggingface).
func resolveHFHome(cfg *config.Config) (string, string) {
	if cfg.HFHome != "" {
		return cfg.HFHome, "config"
	}
	if v := os.Getenv("HF_HOME"); v != "" {
		return v, "$HF_HOME"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface"), "default"
}

// readCachedModels lists the repos the HF cache holds. A repo lives under
// <hub>/hub/models--<org>--<name>/; the current snapshot commit is the
// entry under refs/main; the snapshot directory of that commit is the one
// the loader reads. Repos with no snapshot or no commit are skipped.
func readCachedModels(hub string) ([]CachedModel, error) {
	root := filepath.Join(hub, "hub")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []CachedModel{}, nil
		}
		return nil, err
	}
	var out []CachedModel
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "models--") {
			continue
		}
		raw := strings.TrimPrefix(e.Name(), "models--")
		repo := strings.ReplaceAll(raw, "--", "/")
		dir := filepath.Join(root, e.Name())
		commit, err := os.ReadFile(filepath.Join(dir, "refs", "main"))
		if err != nil {
			continue
		}
		sha := strings.TrimSpace(string(commit))
		if sha == "" {
			continue
		}
		snap := filepath.Join(dir, "snapshots", sha)
		if _, err := os.Stat(snap); err != nil {
			continue
		}
		out = append(out, CachedModel{Repo: repo, SnapshotSHA: sha, HubCache: hub})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Repo < out[j].Repo })
	return out, nil
}
