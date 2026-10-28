package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// GET /api/runs is `ps --json`: an exited run whose log matches a rule has
// findings; one that matches none has no findings key.
func TestRunsAPIFindings(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	for name, log := range map[string]string{
		"oom":   "start\n(EngineCore pid=1) ERROR torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 1 GiB\n",
		"plain": "start\nKilled\n",
	} {
		dir := runner.RunDir(name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		lp := filepath.Join(dir, "serve.log")
		if err := os.WriteFile(lp, []byte(log), 0o644); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(runner.Record{Name: name, Mode: "host", PID: 0, Port: 18000, Log: lp})
		if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := do(t, newServer(Options{}).handler(), "GET", "/api/runs", map[string]string{"Host": "localhost"})
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var runs []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %v", runs)
	}
	for _, r := range runs {
		f, has := r["findings"].([]any)
		switch r["name"] {
		case "oom":
			if !has || len(f) != 1 || f[0].(map[string]any)["id"] != "memory.gpu-oom" {
				t.Errorf("oom findings = %v", r["findings"])
			}
		case "plain":
			if _, ok := r["findings"]; ok {
				t.Errorf("plain run has findings %v", r["findings"])
			}
		}
		if r["state"] != "exited" {
			t.Errorf("%v state = %v", r["name"], r["state"])
		}
	}
}
