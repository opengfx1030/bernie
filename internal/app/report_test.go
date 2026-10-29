package app

// The share report (ia.md R24) must carry the facts that make a bug
// reproducible and nothing that identifies the host: no hostname, no
// username, no token and no path with the name of the user.

import (
	"os"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
)

func TestReportCarriesNoIdentity(t *testing.T) {
	name := "bernieuser"
	t.Setenv("USER", name)
	home := t.TempDir()
	t.Setenv("HOME", home)

	h := host.Info{
		Hostname: "secret-host-name",
		Kernel:   "6.9.0",
		CPU:      host.CPU{Model: "AMD EPYC", Threads: 64},
		RAMBytes: 128 << 30,
		User:     host.User{Name: name, Home: home},
	}
	gpus := []gpu.GPU{{Name: "Radeon PRO W7800", GFX: "gfx1100", VRAMBytes: 32 << 30}}
	rep := doctor.Report{
		Summary: map[doctor.Status]int{doctor.OK: 40, doctor.Warn: 1, doctor.Fail: 1, doctor.Skip: 2},
		Checks: []doctor.Check{
			{ID: "gpu.kfd", Category: "gpu", Status: doctor.Fail, Title: "/dev/kfd is readable",
				Detail: home + "/venv/lib/python3.12/site-packages", Fix: "sudo modprobe amdgpu"},
			{ID: "rocm.tools", Category: "rocm", Status: doctor.Warn, Title: "ROCm tools",
				Detail: "no /opt/rocm for " + name},
			{ID: "os.kernel", Category: "os", Status: doctor.OK, Title: "Kernel is new enough",
				Detail: "6.9.0"},
		},
	}
	doc := buildShareReport(h, gpus, rep)

	for _, bad := range []string{"secret-host-name", name, home, "secret"} {
		if strings.Contains(doc.Report, bad) {
			t.Errorf("the report carries %q:\n%s", bad, doc.Report)
		}
	}
	if !strings.Contains(doc.Report, "gfx1100") {
		t.Errorf("the report lost the shape of the cards:\n%s", doc.Report)
	}
	if !strings.Contains(doc.Report, "1 fail") {
		t.Errorf("the report lost the summary:\n%s", doc.Report)
	}
	if !strings.Contains(doc.Report, "/dev/kfd") {
		t.Errorf("the report lost the failing check:\n%s", doc.Report)
	}
	if strings.Contains(doc.Report, "Kernel is new enough") {
		t.Errorf("the report carries a check that passed:\n%s", doc.Report)
	}
}

func TestReportRedactsThePathOfTheUser(t *testing.T) {
	name := "operator"
	t.Setenv("USER", name)
	s := redactReportText("venv /home/" + name + "/venv/lib is missing")
	if strings.Contains(s, name) {
		t.Errorf("the path of the user survived: %q", s)
	}
	if !strings.Contains(s, "<user>") {
		t.Errorf("the path does not carry the marker: %q", s)
	}
}

func TestReportRedactsTheHomeDirectory(t *testing.T) {
	home := "/tmp/bernie-home-test"
	t.Setenv("HOME", home)
	os.Setenv("HOME", home)
	s := redactReportText("file " + home + "/x is missing")
	if strings.Contains(s, home) {
		t.Errorf("the home directory survived: %q", s)
	}
	if !strings.Contains(s, "~/x") {
		t.Errorf("the home directory is not shortened: %q", s)
	}
}
