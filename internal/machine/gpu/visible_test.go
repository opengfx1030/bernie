package gpu

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// The fixture host has 8 V620 (gfx1030) at HIP 2-9 and 2 W7800 (gfx1100) at
// HIP 0-1.
func TestVisibleLimitsTheGPUs(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	t.Setenv("HIP_VISIBLE_DEVICES", "8")
	vis, err := Visible(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.GPUs) != 1 {
		t.Fatalf("gpus = %d, want 1", len(vis.GPUs))
	}
	if vis.GPUs[0].HIPIndex != 8 || vis.GPUs[0].GFX != "gfx1030" {
		t.Errorf("gpu = %+v", vis.GPUs[0])
	}
	if vis.Restriction == nil {
		t.Fatal("no restriction")
	}
	if got := vis.Restriction.Note(); got != "GPUs limited to 8 by HIP_VISIBLE_DEVICES" {
		t.Errorf("note = %q", got)
	}
}

func TestVisibleWithoutRestriction(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	t.Setenv("HIP_VISIBLE_DEVICES", "")
	t.Setenv("ROCR_VISIBLE_DEVICES", "")
	vis, err := Visible(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.GPUs) != 10 {
		t.Errorf("gpus = %d, want 10", len(vis.GPUs))
	}
	if vis.Restriction != nil {
		t.Errorf("restriction = %v", vis.Restriction)
	}
}

func TestVisibleWithROCRVisibleDevices(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	t.Setenv("HIP_VISIBLE_DEVICES", "")
	t.Setenv("ROCR_VISIBLE_DEVICES", "2,3")
	vis, err := Visible(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.GPUs) != 2 || vis.GPUs[0].HIPIndex != 2 || vis.GPUs[1].HIPIndex != 3 {
		t.Fatalf("gpus = %+v", vis.GPUs)
	}
	if got := vis.Restriction.Note(); got != "GPUs limited to 2,3 by ROCR_VISIBLE_DEVICES" {
		t.Errorf("note = %q", got)
	}
}

// A restriction that names a GPU the host does not have is an error.
func TestVisibleRejectsAMissingGPU(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	t.Setenv("HIP_VISIBLE_DEVICES", "42")
	_, err := Visible(root)
	if err == nil {
		t.Fatal("want an error")
	}
	if !Restricted(err) {
		t.Errorf("err = %v, want a restriction error", err)
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("err = %v", err)
	}
}

// The restriction of the environment is known even when the host has no GPU
// list: the note is still worth showing.
func TestVisibleKeepsTheRestrictionWithoutGPUs(t *testing.T) {
	t.Setenv("HIP_VISIBLE_DEVICES", "8")
	vis, err := Visible(t.TempDir())
	if err == nil {
		t.Fatal("want the error of the missing KFD topology")
	}
	if vis.Restriction == nil {
		t.Fatal("no restriction")
	}
	if got := vis.Restriction.Note(); got != "GPUs limited to 8 by HIP_VISIBLE_DEVICES" {
		t.Errorf("note = %q", got)
	}
}

// A value that is not a list of indices (a GPU UUID) is ignored with a
// warning: bernie uses every GPU.
func TestVisibleIgnoresAUuid(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	t.Setenv("HIP_VISIBLE_DEVICES", "GPU-1234")
	vis, err := Visible(root)
	if err != nil {
		t.Fatal(err)
	}
	if vis.Restriction != nil {
		t.Errorf("restriction = %v", vis.Restriction)
	}
	if !strings.Contains(vis.Warning, "GPU-1234") {
		t.Errorf("warning = %q", vis.Warning)
	}
	if len(vis.GPUs) != 10 {
		t.Errorf("gpus = %d, want 10", len(vis.GPUs))
	}
}
