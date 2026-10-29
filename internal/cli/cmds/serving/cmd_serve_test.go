package serving

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// runStopCmd invokes runStop with a hand-built cli.Context. The state dir
// (where bernie looks for runs) is the test temp dir; the host root is the
// same dir, so an empty state means "no runs".
func runStopCmd(t *testing.T, args ...string) (out string, err error) {
	t.Helper()
	state := t.TempDir()
	t.Setenv(hostroot.EnvVar, state)
	cmd := &cli.Command{Name: "stop", Summary: "Stop a run (the only running one if NAME is absent)", Usage: "[NAME]"}
	var buf bytes.Buffer
	c := &cli.Context{Ctx: context.Background(), Out: &buf, Err: &buf, JSON: false}
	err = runStop(c, cmd, args)
	return buf.String(), err
}

// TestStopHintWhenRunIsGone covers D2: `bernie stop <name>` says "the run
// already exited; nothing to stop" when the run record is gone (its parent
// shell died, another worker cleaned up, the run was already stopped).
func TestStopHintWhenRunIsGone(t *testing.T) {
	out, err := runStopCmd(t, "missing-run")
	if err == nil {
		t.Fatalf("expected an error for an unknown run, got nil; out=%q", out)
	}
	if !strings.Contains(err.Error(), "no run named") {
		t.Errorf("error should mention the missing run, got %v", err)
	}
	if !strings.Contains(out, "the run already exited; nothing to stop") {
		t.Errorf("expected the already-exited hint in the output, got %q", out)
	}
}

// TestStopHintNotEmittedOnUsageError covers the corner case where the
// operator types `bernie stop a b` (two names): the hint must not be
// printed, because the failure is a usage error, not a missing run.
func TestStopHintNotEmittedOnUsageError(t *testing.T) {
	out, err := runStopCmd(t, "a", "b")
	if err == nil {
		t.Fatalf("expected a usage error for two run names, got nil; out=%q", out)
	}
	if strings.Contains(out, "the run already exited") {
		t.Errorf("the hint must not appear on a usage error, got %q", out)
	}
}
