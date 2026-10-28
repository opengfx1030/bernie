package tunecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
)

func TestApplyPowerNeedsRootExit(t *testing.T) {
	apply := func(string) (tuning.PowerApplyResult, error) {
		return tuning.PowerApplyResult{
			File: "/config/power.yaml", NeedsRoot: true,
			RootCommand: "sudo '/usr/local/bin/bernie' tune power apply --file '/config/power.yaml'",
		}, tuning.ErrNeedsRoot
	}
	var buf bytes.Buffer
	c := &cli.Context{Ctx: context.Background(), Out: &buf, Err: &buf}
	err := applyPower(c, "/config/power.yaml", apply)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitChecks {
		t.Fatalf("err = %v, want exit %d", err, cli.ExitChecks)
	}
	if !strings.Contains(buf.String(), "tune power apply") || !strings.Contains(err.Error(), "needs root") {
		t.Fatalf("output = %q, err = %v", buf.String(), err)
	}
}

func TestApplyPowerNeedsRootJSON(t *testing.T) {
	apply := func(string) (tuning.PowerApplyResult, error) {
		return tuning.PowerApplyResult{File: "/config/power.yaml", NeedsRoot: true, RootCommand: "sudo bernie tune power apply"}, tuning.ErrNeedsRoot
	}
	var buf bytes.Buffer
	c := &cli.Context{Ctx: context.Background(), Out: &buf, Err: &buf, JSON: true}
	err := applyPower(c, "/config/power.yaml", apply)
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitChecks {
		t.Fatalf("err = %v, want exit %d", err, cli.ExitChecks)
	}
	var doc struct {
		NeedsRoot   bool   `json:"needs_root"`
		RootCommand string `json:"root_command"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if !doc.NeedsRoot || !strings.Contains(doc.RootCommand, "tune power apply") {
		t.Fatalf("document = %+v", doc)
	}
}
