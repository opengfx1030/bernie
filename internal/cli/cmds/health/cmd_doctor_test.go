package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/opengfx1030/rdnactl/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	failingLog = "INFO starting\n(Worker pid=1) ERROR RuntimeError: HIP error: hipErrorInvalidImage\n"
	healthyLog = "INFO starting\nINFO:     Application startup complete.\n"
)

func logDoctor(t *testing.T, json bool, file, run string) (out string, code int, err error) {
	t.Helper()
	var buf bytes.Buffer
	c := &cli.Context{Ctx: context.Background(), Out: &buf, Err: &buf, JSON: json}
	err = doctorLog(c, file, run)
	var ee *cli.ExitError
	if errors.As(err, &ee) {
		code = ee.Code
	}
	return buf.String(), code, err
}

func writeFile(t *testing.T, path, data string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDoctorLogFile(t *testing.T) {
	dir := t.TempDir()
	bad := writeFile(t, filepath.Join(dir, "bad.log"), failingLog)
	ok := writeFile(t, filepath.Join(dir, "ok.log"), healthyLog)

	out, code, _ := logDoctor(t, false, bad, "")
	if code != cli.ExitChecks {
		t.Errorf("failing log: exit %d, want %d", code, cli.ExitChecks)
	}
	for _, want := range []string{"kernels.invalid-image", "(line 2)", "fix:", "doc:", "hipErrorInvalidImage"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output lacks %q:\n%s", want, out)
		}
	}

	out, code, err := logDoctor(t, false, ok, "")
	if err != nil || code != 0 || !strings.Contains(out, "no known failures") {
		t.Errorf("healthy log: exit %d err %v out %q", code, err, out)
	}

	out, code, _ = logDoctor(t, true, bad, "")
	var rep struct {
		Matches []struct {
			ID     string `json:"id"`
			LineNo int    `json:"line_no"`
			Fix    string `json:"fix"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if code != cli.ExitChecks || len(rep.Matches) != 1 || rep.Matches[0].ID != "kernels.invalid-image" || rep.Matches[0].LineNo != 2 || rep.Matches[0].Fix == "" {
		t.Errorf("json report = %+v (exit %d)", rep, code)
	}

	out, code, _ = logDoctor(t, true, ok, "")
	if code != 0 || strings.TrimSpace(strings.Join(strings.Fields(out), "")) != `{"matches":[]}` {
		t.Errorf("healthy json = %q (exit %d)", out, code)
	}

	if _, _, err := logDoctor(t, false, filepath.Join(dir, "missing.log"), ""); err == nil {
		t.Error("missing file: no error")
	}
}

func TestDoctorLogRun(t *testing.T) {
	state := t.TempDir()
	t.Setenv("RDNACTL_STATE_DIR", state)
	run := func(name string) string { return filepath.Join(state, "runs", name) }

	// run.json names the log file.
	elsewhere := writeFile(t, filepath.Join(t.TempDir(), "custom.log"), failingLog)
	rec, _ := json.Marshal(map[string]string{"name": "a", "log": elsewhere})
	writeFile(t, filepath.Join(run("a"), "run.json"), string(rec))
	if _, code, _ := logDoctor(t, false, "", "a"); code != cli.ExitChecks {
		t.Errorf("run a: exit %d, want %d", code, cli.ExitChecks)
	}

	// The recorded file is gone: fall back to serve.log in the run directory.
	rec, _ = json.Marshal(map[string]string{"name": "b", "log": filepath.Join(state, "gone.log")})
	writeFile(t, filepath.Join(run("b"), "run.json"), string(rec))
	writeFile(t, filepath.Join(run("b"), "serve.log"), healthyLog)
	if out, code, err := logDoctor(t, false, "", "b"); err != nil || code != 0 {
		t.Errorf("run b: exit %d err %v out %q", code, err, out)
	}

	if _, _, err := logDoctor(t, false, "", "nope"); err == nil || !strings.Contains(err.Error(), "no run named") {
		t.Errorf("unknown run: err = %v", err)
	}
}
