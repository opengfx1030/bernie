package setup

import (
	"os"
	"path/filepath"
	"strings"
)

func (p planner) modelDownload() Step {
	if p.o.ModelRepo == "" {
		return Step{}
	}
	hf := "hf"
	if venvHF := filepath.Join(p.o.Venv, "bin", "hf"); which("hf") == "" && isFile(venvHF) {
		hf = shq(venvHF)
	}
	return Step{
		ID: "model.download", Title: "Download " + p.o.ModelRepo, Mode: "both",
		Done:     modelCached(hfHub(p.o.HFHome), p.o.ModelRepo),
		Commands: []string{hf + " download " + shq(p.o.ModelRepo)},
		Why:      "The model goes into the Hugging Face cache (" + hfHub(p.o.HFHome) + "), shared by host and container mode.",
	}
}

// hfHub is the hub directory of the Hugging Face cache.
func hfHub(override string) string {
	if override == "" {
		override = os.Getenv("HF_HOME")
	}
	if override == "" {
		home, _ := os.UserHomeDir()
		override = filepath.Join(home, ".cache", "huggingface")
	}
	return filepath.Join(override, "hub")
}

// modelCached reports a non-empty snapshot of repo in the hub cache.
func modelCached(hub, repo string) bool {
	if filepath.IsAbs(repo) {
		return isDir(repo)
	}
	dir := filepath.Join(hub, "models--"+strings.ReplaceAll(repo, "/", "--"), "snapshots")
	snaps, _ := os.ReadDir(dir)
	for _, s := range snaps {
		files, _ := os.ReadDir(filepath.Join(dir, s.Name()))
		if len(files) > 0 {
			return true
		}
	}
	return false
}
