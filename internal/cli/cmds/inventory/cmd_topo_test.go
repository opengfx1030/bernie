package inventory

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

func TestPrintP2PChecklist(t *testing.T) {
	rep := app.P2PChecklistReport{
		Ready:   true,
		Summary: map[string]int{"ok": 1, "warn": 1, "fail": 0, "skip": 1},
		Checklist: []topo.CheckItem{
			{ID: "kfd_links", Title: "KFD peer links", Status: topo.P2POK, Detail: "45 of 45 GPU pairs have a KFD P2P link"},
			{ID: "iommu", Title: "IOMMU mode", Status: topo.P2PWarn, Detail: "no iommu=pt on the kernel command line",
				Fix: `sudo grubby --update-kernel=ALL --args="iommu=pt"`},
			{ID: "bandwidth_test", Title: "rocm-bandwidth-test", Status: topo.P2PSkip, Detail: "not installed"},
		},
	}
	var out strings.Builder
	printP2PChecklist(&out, rep)
	got := out.String()
	for _, want := range []string{
		"P2P readiness checklist",
		"[ok  ] KFD peer links: 45 of 45 GPU pairs",
		"[WARN] IOMMU mode:",
		`fix: sudo grubby --update-kernel=ALL --args="iommu=pt"`,
		"[skip] rocm-bandwidth-test: not installed",
		"1 ok, 1 warn, 0 fail, 1 skip",
		"ready:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}
