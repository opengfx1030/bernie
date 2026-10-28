package doctor

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/rocm"
)

func TestNestedROCmInstallIsOneInstall(t *testing.T) {
	in := base()
	in.rocm.Installs = []rocm.Install{{Root: "/opt/rocm-7.2.0", Version: "7.2.0", Kind: "opt",
		Nested: []rocm.Install{{Root: "/opt/rocm-7.2.0/core-7.14", Version: "7.14.1", Kind: "therock"}}}}
	got := find(t, evaluate(in), "rocm.installs")
	if got.Status != Warn || !strings.Contains(got.Detail, "1 install with nested layout") {
		t.Fatalf("rocm.installs = %+v", got)
	}
}

func TestROCmPathWinnerFix(t *testing.T) {
	r := rocm.Report{
		Installs: []rocm.Install{{Root: "/opt/rocm-7.2.0", Version: "7.2.0", Kind: "opt",
			Nested: []rocm.Install{{Root: "/opt/rocm-7.2.0/core-7.14", Version: "7.14.1", Kind: "therock"}}}},
		Tools:  map[string]string{"hipcc": "/opt/rocm-7.2.0/core-7.14/bin/hipcc"},
		OnPath: map[string]bool{"hipcc": true},
	}
	fix := pathWinnerFix(r)
	for _, want := range []string{"PATH uses ROCm 7.14.1", "/opt/rocm-7.2.0/core-7.14", "which hipcc"} {
		if !strings.Contains(fix, want) {
			t.Errorf("fix %q lacks %q", fix, want)
		}
	}
}
