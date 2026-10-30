package setup

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestStepWriterStripsBernieJSON: the stepWriter mirrors the raw step
// output to the log when Mirror is true, but never the BERNIE_JSON
// evidence line. The line is the venv probe's single JSON document: it
// belongs in the per-step log and the record, never on the terminal.
func TestStepWriterStripsBernieJSON(t *testing.T) {
	var logBuf bytes.Buffer
	sw := &stepWriter{mirror: true, log: &logBuf}
	lines := []string{
		"== venv.verify: Verify the vLLM build in the venv",
		"$ BERNIE_VENV_STRICT=1 python -I - <<PY",
		"Collecting torch==2.12.0+rocm7.14.0",
		"BERNIE_JSON:{\"python\":\"3.12.14\",\"torch\":\"2.12.0+rocm7.14.0\"}",
		"Successfully installed torch-2.12.0+rocm7.14.0",
	}
	for _, l := range lines {
		sw.line(l)
	}
	got := logBuf.String()
	for _, want := range []string{
		"== venv.verify: Verify the vLLM build in the venv",
		"Collecting torch",
		"Successfully installed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the mirror is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "BERNIE_JSON:") {
		t.Errorf("the mirror leaked the BERNIE_JSON evidence line:\n%s", got)
	}
}

// TestApplierSuppressesMarkersWhenPrinterWired: the applier skips its
// own "== step: title" and "$ cmd" markers when a printer is wired and
// Verbose is off, so the live CR line of the printer is not fought by
// a second stream of plain text. With Verbose on, the markers come
// through; without a printer at all, they also come through (the
// dashboard and the legacy Apply helper rely on them).
func TestApplierSuppressesMarkersWhenPrinterWired(t *testing.T) {
	dir := t.TempDir()
	store := &RunStore{Root: filepath.Join(dir, "setup")}
	steps := []Step{
		{ID: "noop", Title: "noop", Commands: []string{"echo hi"}},
	}
	t.Run("no printer -> markers reach the log", func(t *testing.T) {
		var logBuf bytes.Buffer
		if _, err := ApplyRun(context.Background(), steps, nil, &logBuf, ApplyOptions{Store: store}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(logBuf.String(), "== noop: noop") {
			t.Errorf("the markers were suppressed without a printer:\n%s", logBuf.String())
		}
		if !strings.Contains(logBuf.String(), "$ echo hi") {
			t.Errorf("the command echo was suppressed without a printer:\n%s", logBuf.String())
		}
	})
	t.Run("printer without verbose -> markers suppressed", func(t *testing.T) {
		var logBuf bytes.Buffer
		if _, err := ApplyRun(context.Background(), steps, nil, &logBuf, ApplyOptions{Store: store, Event: func(Event) {}}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(logBuf.String(), "== noop: noop") {
			t.Errorf("the markers leaked with a printer:\n%s", logBuf.String())
		}
		if strings.Contains(logBuf.String(), "$ echo hi") {
			t.Errorf("the command echo leaked with a printer:\n%s", logBuf.String())
		}
	})
	t.Run("printer with verbose -> markers reach the log", func(t *testing.T) {
		var logBuf bytes.Buffer
		if _, err := ApplyRun(context.Background(), steps, nil, &logBuf, ApplyOptions{Store: store, Verbose: true, Event: func(Event) {}}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(logBuf.String(), "== noop: noop") {
			t.Errorf("the markers were suppressed with verbose:\n%s", logBuf.String())
		}
		if !strings.Contains(logBuf.String(), "$ echo hi") {
			t.Errorf("the command echo was suppressed with verbose:\n%s", logBuf.String())
		}
	})
}
