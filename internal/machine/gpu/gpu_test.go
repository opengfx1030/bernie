package gpu

import (
	"path/filepath"
	"testing"

	"github.com/opengfx1030/rdnactl/internal/support/fixture"
)

func TestGFXName(t *testing.T) {
	cases := map[int]string{
		100300: "gfx1030",
		100301: "gfx1031",
		100302: "gfx1032",
		110000: "gfx1100",
		110001: "gfx1101",
		120001: "gfx1201",
		90010:  "gfx90a",
		90402:  "gfx942",
		0:      "",
	}
	for v, want := range cases {
		if got := GFXName(v); got != want {
			t.Errorf("GFXName(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestLinkGen(t *testing.T) {
	cases := map[string]int{
		"16.0 GT/s PCIe": 4,
		"32.0 GT/s PCIe": 5,
		"8.0 GT/s PCIe":  3,
		"5.0 GT/s PCIe":  2,
		"2.5 GT/s PCIe":  1,
		"Unknown":        0,
		"":               0,
	}
	for in, want := range cases {
		if got := LinkGen(in); got != want {
			t.Errorf("LinkGen(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestListPar1(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	gpus, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 10 {
		t.Fatalf("got %d GPUs, want 10", len(gpus))
	}
	// KFD node 10 sorts after 9 (numeric order, not lexical).
	want := []struct {
		node     int
		gfx, pci string
		minor    int
		device   string
	}{
		{1, "gfx1100", "0000:c7:00.0", 128, "0x7449"},
		{2, "gfx1100", "0000:83:00.0", 129, "0x7449"},
		{3, "gfx1030", "0000:47:00.0", 130, "0x73a1"},
		{4, "gfx1030", "0000:4a:00.0", 131, "0x73a1"},
		{5, "gfx1030", "0000:52:00.0", 132, "0x73a1"},
		{6, "gfx1030", "0000:55:00.0", 133, "0x73a1"},
		{7, "gfx1030", "0000:07:00.0", 134, "0x73a1"},
		{8, "gfx1030", "0000:0a:00.0", 135, "0x73a1"},
		{9, "gfx1030", "0000:12:00.0", 136, "0x73a1"},
		{10, "gfx1030", "0000:15:00.0", 137, "0x73a1"},
	}
	for i, w := range want {
		g := gpus[i]
		if g.HIPIndex != i || g.KFDNode != w.node || g.GFX != w.gfx || g.PCI != w.pci || g.RenderMinor != w.minor || g.DeviceID != w.device {
			t.Errorf("gpu %d = %+v, want %+v", i, g, w)
		}
		if g.Link != (Link{Gen: 4, Width: 16}) {
			t.Errorf("gpu %d link = %+v", i, g.Link)
		}
	}
	w7800 := gpus[0]
	if w7800.Name != "Radeon PRO W7800 48GB" || w7800.VRAMBytes != 48301604864 || w7800.PowerCapW != 180 {
		t.Errorf("W7800 = %+v", w7800)
	}
	if w7800.SysfsPath != "/sys/devices/pci0000:c0/0000:c0:03.1/0000:c5:00.0/0000:c6:00.0/0000:c7:00.0" {
		t.Errorf("sysfs path = %s", w7800.SysfsPath)
	}
	v620 := gpus[2]
	if v620.Name != "AMD RADEON PRO V620 Azure" || v620.VRAMBytes != 32195477504 || v620.PowerCapW != 175 || v620.NUMANode != -1 {
		t.Errorf("V620 = %+v", v620)
	}
}

func TestListLocationFallbackAndNameTable(t *testing.T) {
	// A node without DRM sysfs: PCI comes from location_id/domain, name from
	// the device id table.
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		KFDNodesDir + "/0/properties": "cpu_cores_count 16\nsimd_count 0\n",
		KFDNodesDir + "/1/properties": "simd_count 120\ngfx_target_version 100302\ndevice_id 29695\nlocation_id 768\ndomain 1\ndrm_render_minor 0\n",
	})
	gpus, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 1 {
		t.Fatalf("got %d GPUs", len(gpus))
	}
	g := gpus[0]
	if g.PCI != "0001:03:00.0" || g.GFX != "gfx1032" || g.DeviceID != "0x73ff" || g.Name != "Radeon RX 6600 / 6600 XT" {
		t.Errorf("gpu = %+v", g)
	}
}

func TestListSingleGPUFixtures(t *testing.T) {
	cases := []struct {
		fixture, name, gfx, id string
		vram                   uint64
		capW                   float64
	}{
		{"w7800-single", "Radeon PRO W7800 48GB", "gfx1100", "0x7449", 48301604864, 180},
		{"rx7900xtx-synthetic", "Radeon RX 7900 XT / 7900 XTX / 7900 GRE", "gfx1100", "0x744c", 24 << 30, 355},
		{"rx6800xt-synthetic", "Radeon RX 6800 / 6800 XT / 6900 XT", "gfx1030", "0x73bf", 16 << 30, 300},
	}
	for _, c := range cases {
		gpus, err := List(fixture.Root(t, c.fixture))
		if err != nil || len(gpus) != 1 {
			t.Fatalf("%s: List = %v, %v", c.fixture, gpus, err)
		}
		g := gpus[0]
		if g.Name != c.name || g.GFX != c.gfx || g.DeviceID != c.id || g.VRAMBytes != c.vram || g.PowerCapW != c.capW || g.HIPIndex != 0 || g.Link != (Link{Gen: 4, Width: 16}) {
			t.Errorf("%s: gpu = %+v", c.fixture, g)
		}
	}
}

func TestListUnknownDeviceNamedByGFX(t *testing.T) {
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		KFDNodesDir + "/1/properties": "simd_count 8\ngfx_target_version 100306\ndevice_id 5000\ndrm_render_minor 0\n",
	})
	gpus, err := List(root)
	if err != nil || len(gpus) != 1 {
		t.Fatalf("List = %v, %v", gpus, err)
	}
	if g := gpus[0]; g.Name != "AMD GPU gfx1036" || g.DeviceID != "0x1388" {
		t.Errorf("gpu = %+v", g)
	}
}

func TestTelemetry(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	gpus, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	s := Telemetry(root, gpus)
	if len(s) != 10 {
		t.Fatalf("got %d samples", len(s))
	}
	if s[0].PowerCapW != 180 || s[0].VRAMTotal != 48301604864 || s[0].VRAMUsed == 0 || s[0].TempC <= 0 {
		t.Errorf("sample 0 = %+v", s[0])
	}
	// edge temp is temp1 on amdgpu; must be read in degrees, not millidegrees.
	if s[2].TempC > 150 {
		t.Errorf("temp not scaled: %+v", s[2])
	}
}

func TestIsBDF(t *testing.T) {
	for in, want := range map[string]bool{
		"0000:c7:00.0": true,
		"0000:C7:00.0": false,
		"pci0000:c0":   false,
		filepath.Base("/sys/devices/pci0000:00/0000:00:01.1"): true,
	} {
		if IsBDF(in) != want {
			t.Errorf("IsBDF(%q) != %v", in, want)
		}
	}
}
