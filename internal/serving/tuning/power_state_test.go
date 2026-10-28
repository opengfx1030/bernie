package tuning

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

func TestPowerDesiredRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "power.yaml")
	want := PowerDesired{Version: 1, GPUs: map[string]float64{
		"all": 180, "0": 160, "0000:47:00.0": 150,
	}}
	if err := SavePowerDesired(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPowerDesired(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || got.GPUs["all"] != 180 || got.GPUs["0"] != 160 || got.GPUs["0000:47:00.0"] != 150 {
		t.Fatalf("round trip = %+v", got)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Errorf("power.yaml mode = %o, want 644", st.Mode().Perm())
	}
	if err := os.WriteFile(path, []byte("version: 1\ngpus:\n  all: 180\nextra: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPowerDesired(path); err == nil || !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("unknown field error = %v", err)
	}
	if err := os.WriteFile(path, []byte("version: 1\ngpus:\n  all: -1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPowerDesired(path); err == nil || !strings.Contains(err.Error(), "positive number") {
		t.Fatalf("invalid watts error = %v", err)
	}
}

func TestPowerDesiredResolve(t *testing.T) {
	root := t.TempDir()
	node := filepath.Join(root, gpu.KFDNodesDir, "4")
	if err := os.MkdirAll(node, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(node, "properties"), []byte("unique_id 123456789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gpus := []gpu.GPU{
		{HIPIndex: 0, KFDNode: 4, PCI: "0000:47:00.0"},
		{HIPIndex: 1, KFDNode: 5, PCI: "0000:48:00.0"},
	}
	d := PowerDesired{Version: 1, GPUs: map[string]float64{
		"all": 180, "0": 160, "0000:47:00.0": 150, "123456789": 140,
	}}
	got := d.Resolve(root, gpus)
	if got[0] != 140 || got[1] != 180 {
		t.Fatalf("resolved = %v", got)
	}
}

func TestPowerUnitsGolden(t *testing.T) {
	u, err := GeneratePowerUnits("/usr/local/bin/bernie", "/etc/bernie/power.yaml", "/var/lib/bernie/power")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ got, golden string }{
		{u.Service, "testdata/bernie-power.service.golden"},
		{u.Path, "testdata/bernie-power.path.golden"},
	} {
		want, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatal(err)
		}
		if tc.got != string(want) {
			t.Fatalf("unit differs from %s:\n got:\n%s\nwant:\n%s", tc.golden, tc.got, want)
		}
	}
	if u.ServiceFile != "/var/lib/bernie/power/bernie-power.service" || u.PathFile != "/var/lib/bernie/power/bernie-power.path" {
		t.Fatalf("unit paths = %+v", u)
	}
	cmds := u.InstallCommands()
	if len(cmds) != 1 || !strings.Contains(cmds[0], "install -m644") || !strings.Contains(cmds[0], "systemctl enable --now") {
		t.Fatalf("install commands = %q", cmds)
	}
	for _, want := range []string{"NoNewPrivileges=yes", "ProtectSystem=strict", "ReadWritePaths=/sys/class/drm /sys/class/hwmon", "RestrictAddressFamilies=AF_UNIX", "IPAddressDeny=any"} {
		if !strings.Contains(u.Service, want) {
			t.Errorf("service unit misses %q", want)
		}
	}
}

func TestApplyPowerDesiredFixture(t *testing.T) {
	root, gpus := powerTree(t, map[int][3]int{0: {175, 120, 250}})
	file := filepath.Join(t.TempDir(), "power.yaml")
	if err := SavePowerDesired(file, PowerDesired{Version: 1, GPUs: map[string]float64{"all": 100}}); err != nil {
		t.Fatal(err)
	}
	res, err := ApplyPowerDesired(root, gpus, file, "/usr/local/bin/bernie", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GPUs) != 1 {
		t.Fatalf("result = %+v", res)
	}
	got := res.GPUs[0]
	if got.RequestedW != 100 || got.EffectiveW != 120 || !got.Clamped || !got.Changed || !got.Wrote {
		t.Fatalf("result = %+v", got)
	}
	b, _ := os.ReadFile(got.Path)
	if strings.TrimSpace(string(b)) != "120000000" {
		t.Fatalf("power1_cap = %q", b)
	}
	minBefore, _ := os.ReadFile(filepath.Join(filepath.Dir(got.Path), "power1_cap_min"))
	maxBefore, _ := os.ReadFile(filepath.Join(filepath.Dir(got.Path), "power1_cap_max"))
	res, err = ApplyPowerDesired(root, gpus, file, "/usr/local/bin/bernie", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.GPUs[0].Changed || res.GPUs[0].Wrote {
		t.Fatalf("second apply changed the cap: %+v", res.GPUs[0])
	}
	minAfter, _ := os.ReadFile(filepath.Join(filepath.Dir(got.Path), "power1_cap_min"))
	maxAfter, _ := os.ReadFile(filepath.Join(filepath.Dir(got.Path), "power1_cap_max"))
	if string(minBefore) != string(minAfter) || string(maxBefore) != string(maxAfter) {
		t.Fatal("apply changed a file other than power1_cap")
	}
}

func TestApplyPowerDesiredNeedsRoot(t *testing.T) {
	root, gpus := powerTree(t, map[int][3]int{0: {175, 120, 250}})
	file := filepath.Join(t.TempDir(), "power.yaml")
	if err := SavePowerDesired(file, PowerDesired{Version: 1, GPUs: map[string]float64{"all": 150}}); err != nil {
		t.Fatal(err)
	}
	res, err := ApplyPowerDesired(root, gpus, file, "/usr/local/bin/bernie", false)
	if !errors.Is(err, ErrNeedsRoot) {
		t.Fatalf("err = %v", err)
	}
	if !res.NeedsRoot || !strings.Contains(res.RootCommand, "sudo ") || !strings.Contains(res.RootCommand, "tune power apply") {
		t.Fatalf("result = %+v", res)
	}
	status := powerStatus(root, gpus)
	if len(status) != 1 {
		t.Fatalf("status = %+v", status)
	}
	b, _ := os.ReadFile(status[0].Path)
	if strings.TrimSpace(string(b)) != "175000000" {
		t.Fatalf("non-root apply changed power1_cap to %q", b)
	}
}
