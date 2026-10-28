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
