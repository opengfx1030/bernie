package gpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/fixture"
)

func TestTelemetryReadsJunctionMemoryAndClocks(t *testing.T) {
	root := fixture.Root(t, "v620-ecc")
	gpus, err := List(root)
	if err != nil || len(gpus) != 1 {
		t.Fatalf("List = %v, %v", gpus, err)
	}
	s := Telemetry(root, gpus)[0]
	if s.TempC != 33 {
		t.Errorf("edge temp = %v", s.TempC)
	}
	if s.TempJunctionC == nil || *s.TempJunctionC != 35 {
		t.Errorf("junction temp = %v", s.TempJunctionC)
	}
	if s.TempMemC == nil || *s.TempMemC != 34 {
		t.Errorf("memory temp = %v", s.TempMemC)
	}
	// The card was idle: the shader clock was off, the memory clock low.
	if s.SclkMHz == nil || *s.SclkMHz != 0 {
		t.Errorf("sclk = %v", s.SclkMHz)
	}
	if s.MclkMHz == nil || *s.MclkMHz != 96 {
		t.Errorf("mclk = %v", s.MclkMHz)
	}
}

func TestTempByLabelMissingSensor(t *testing.T) {
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		"hw/temp1_input": "33000\n",
		"hw/temp1_label": "edge\n",
	})
	if got := tempByLabel(filepath.Join(root, "hw"), "junction"); got != nil {
		t.Errorf("junction = %v, want nil", *got)
	}
	if got := tempByLabel(filepath.Join(root, "hw"), "edge"); got == nil || *got != 33 {
		t.Errorf("edge = %v", got)
	}
}

func TestDPMClockTakesTheStarredLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pp_dpm_sclk")
	// The captured file of an idle card: the shader clock is off.
	if err := os.WriteFile(path, []byte("S: 0Mhz *\n1: 500Mhz \n2: 2570Mhz \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dpmClock(path); got == nil || *got != 0 {
		t.Errorf("clock = %v, want 0", got)
	}
	if err := os.WriteFile(path, []byte("S: 0Mhz \n1: 500Mhz \n2: 2570Mhz *\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dpmClock(path); got == nil || *got != 2570 {
		t.Errorf("clock = %v, want 2570", got)
	}
	if got := dpmClock(filepath.Join(dir, "pp_dpm_mclk")); got != nil {
		t.Errorf("missing clock = %v, want nil", *got)
	}
	if err := os.WriteFile(path, []byte("S: 0Mhz \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dpmClock(path); got != nil {
		t.Errorf("clock without a star = %v, want nil", *got)
	}
}
