package dashboard

import (
	"io/fs"
	"strings"
	"testing"
)

// The Advanced tab carries the two operator tools of the reference: the
// ECC counters and the power cap. The commands are exact and bernie never
// runs them as root (ia.md §5 R36). The checks below read the source of
// the tab: the page has no build step and no JS test runner, so the
// wording and the structure of the file are the contract.

// The tools card holds the commands of the ECC and the power tools.
func TestTheToolsCardHoldsTheExactCommands(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/advanced/doctor.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"cat /sys/class/drm/card*/device/ras/umc_err_count",
		"amdgpu.ras_enable=0",
		"cat /sys/class/drm/card*/device/hwmon/hwmon*/power1_cap",
		"sudo bernie tune power apply",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the tools card holds no %q", want)
		}
	}
	// The card says who runs what: bernie never runs a root command.
	if !strings.Contains(text, "bernie never runs them as root") {
		t.Error("the tools card does not say that bernie never runs them as root")
	}
}

// The tools card is on the Advanced tab, and it reads the doctor report:
// the words and the fixes come from the doctor, not from the page.
func TestTheToolsCardIsOnTheAdvancedTab(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/advanced/console.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "advToolsCard()") {
		t.Error("the Advanced tab does not build the tools card")
	}
	if !strings.Contains(text, `id: "advanced-tools"`) {
		t.Error("the Advanced tab has no section for the tools")
	}
	if !strings.Contains(text, "adv.live.tools.update(adv.doctor)") {
		t.Error("the tools card is not painted with the doctor report")
	}
}
