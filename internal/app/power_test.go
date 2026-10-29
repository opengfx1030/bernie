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

func TestClampPowerForRows(t *testing.T) {
	rows := []tuning.Power{
		{HIPIndex: 0, MinW: 138, MaxW: 230},
		{HIPIndex: 2, MinW: 150, MaxW: 175},
		{HIPIndex: 3, MinW: 150, MaxW: 175},
	}
	cases := []struct {
		name    string
		idx     []int
		watts   float64
		want    float64
		clamped []int
		wantErr bool
	}{
		{"every card takes it", nil, 170, 170, nil, false},
		{"above the common maximum", nil, 230, 175, []int{2, 3}, false},
		{"below the common minimum", nil, 100, 150, []int{0, 2, 3}, false},
		{"one card takes it", []int{0}, 200, 200, nil, false},
		{"one card above its maximum", []int{2}, 230, 175, []int{2}, false},
		{"two cards share their range", []int{2, 3}, 230, 175, []int{2, 3}, false},
		{"above every card", nil, 300, 175, []int{0, 2, 3}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, clamped, err := clampPowerForRows(rows, c.idx, c.watts)
			if c.wantErr {
				if err == nil {
					t.Fatalf("err = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("watts = %v, want %v", got, c.want)
			}
			if len(clamped) != len(c.clamped) {
				t.Fatalf("clamped = %v, want %v", clamped, c.clamped)
			}
			for i, v := range c.clamped {
				if clamped[i] != v {
					t.Fatalf("clamped = %v, want %v", clamped, c.clamped)
				}
			}
		})
	}
	// Cards with no power range are not clamped.
	got, clamped, err := clampPowerForRows([]tuning.Power{{HIPIndex: 0}}, nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	if got != 300 || len(clamped) != 0 {
		t.Fatalf("watts = %v, clamped = %v", got, clamped)
	}
	// Cards that share no range have no common value.
	if _, _, err := clampPowerForRows([]tuning.Power{
		{HIPIndex: 0, MinW: 200, MaxW: 230},
		{HIPIndex: 1, MinW: 100, MaxW: 175},
	}, nil, 180); err == nil {
		t.Fatal("err = nil, want an error")
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
