package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/tuning"
)

func TestPowerDesiredPlanAndSet(t *testing.T) {
	config := t.TempDir()
	t.Setenv("BERNIE_CONFIG_DIR", config)
	plan, err := PlanPowerDesired(nil, 180)
	if err != nil {
		t.Fatal(err)
	}
	if plan.File != filepath.Join(config, "power.yaml") || plan.Desired["all"] != 180 {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := os.Stat(plan.File); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plan wrote the desired file: %v", err)
	}
	if _, err := PowerDesiredSet(nil, 180); err != nil {
		t.Fatal(err)
	}
	res, err := PowerDesiredSet([]int{2}, 160)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.File != plan.File || len(res.Selectors) != 1 || res.Selectors[0] != "2" {
		t.Fatalf("result = %+v", res)
	}
	d, err := tuning.LoadPowerDesired(plan.File)
	if err != nil {
		t.Fatal(err)
	}
	if d.GPUs["all"] != 180 || d.GPUs["2"] != 160 {
		t.Fatalf("desired = %+v", d)
	}
	res, err = PowerDesiredSet([]int{2}, 160)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatalf("same write reported a change: %+v", res)
	}
}

func TestPowerApplyNeedsRoot(t *testing.T) {
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	old := powerIsRoot
	powerIsRoot = func() bool { return false }
	defer func() { powerIsRoot = old }()
	res, err := PowerApply("")
	if !errors.Is(err, tuning.ErrNeedsRoot) {
		t.Fatalf("err = %v", err)
	}
	if !res.NeedsRoot || !strings.Contains(res.RootCommand, "tune power apply") || !strings.Contains(res.RootCommand, "power.yaml") {
		t.Fatalf("result = %+v", res)
	}
}

func TestPowerServicePlanWritesUnits(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BERNIE_STATE_DIR", state)
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	plan, err := GeneratePowerServicePlan()
	if err != nil {
		t.Fatal(err)
	}
	if plan.ServiceFile != filepath.Join(state, "power", tuning.PowerServiceUnit) {
		t.Fatalf("service file = %q", plan.ServiceFile)
	}
	for _, file := range []string{plan.ServiceFile, plan.PathFile} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty unit file %s", file)
		}
	}
	if len(plan.InstallCommands) != 1 || !strings.Contains(plan.InstallCommands[0], "systemctl enable --now") {
		t.Fatalf("install commands = %q", plan.InstallCommands)
	}
}
