package gpu

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// eccDev is the device directory of the synthetic GPU trees.
const eccDev = "/sys/devices/pci0000:00/0000:00:01.2/0000:00:02.0"

// eccTree writes a synthetic GPU tree and returns the root and the GPU.
func eccTree(t *testing.T, deviceID string, files map[string]string) (string, GPU) {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{"sys/module/amdgpu/parameters/ras_enable": "-1\n"}
	for k, v := range files {
		all[k] = v
	}
	fixture.Write(t, root, all)
	return root, GPU{HIPIndex: 0, DeviceID: deviceID, SysfsPath: eccDev}
}

func TestECCOfCapturedV620(t *testing.T) {
	root := fixture.Root(t, "v620-ecc")
	gpus, err := List(root)
	if err != nil || len(gpus) != 1 {
		t.Fatalf("List = %v, %v", gpus, err)
	}
	e := ECCOf(root, gpus[0])
	if e.State != ECCEnabled || !e.Counters {
		t.Fatalf("ecc = %+v", e)
	}
	if e.UE != 0 || e.CE != 0 || e.DE != 0 {
		t.Errorf("counters = %+v", e)
	}
	if e.Source != "sysfs" || e.Mask != "feature mask: 0x101" || e.RASParam != "-1" {
		t.Errorf("source/mask/param = %+v", e)
	}
}

func TestECCOfSyntheticErrors(t *testing.T) {
	root := fixture.Root(t, "ecc-synthetic")
	gpus, err := List(root)
	if err != nil || len(gpus) != 1 {
		t.Fatalf("List = %v, %v", gpus, err)
	}
	e := ECCOf(root, gpus[0])
	if e.State != ECCEnabled || e.UE != 2 || e.CE != 5 || e.DE != 1 {
		t.Errorf("ecc = %+v", e)
	}
}

func TestECCStates(t *testing.T) {
	cases := []struct {
		name     string
		deviceID string
		files    map[string]string
		want     ECCState
	}{
		{"counters read", "0x73a1", map[string]string{
			eccDev + "/ras/umc_err_count": "ue: 0\nce: 0\nde: 0\n",
		}, ECCEnabled},
		{"card has ECC, kernel counts nothing", "0x73a1", map[string]string{
			eccDev + "/ras/features": "feature mask: 0x100\n",
		}, ECCDisabled},
		{"card has ECC, host has no RAS interface", "0x73a1", nil, ECCSupported},
		{"card has no ECC", "0x73bf", nil, ECCUnsupported},
		{"card has no ECC but the kernel counts", "0x73bf", map[string]string{
			eccDev + "/ras/umc_err_count": "ue: 1\nce: 0\nde: 0\n",
		}, ECCEnabled},
	}
	for _, c := range cases {
		root, g := eccTree(t, c.deviceID, c.files)
		if got := ECCOf(root, g).State; got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseUMCCounters(t *testing.T) {
	cases := []struct {
		text       string
		ce, ue, de uint64
		ok         bool
	}{
		{"ue: 0\nce: 0\nde: 0\n", 0, 0, 0, true},
		{"ue: 2\nce: 5\nde: 1\n", 5, 2, 1, true},
		{"ue: 7\n", 0, 7, 0, true},
		{"", 0, 0, 0, false},
		{"no counters here\n", 0, 0, 0, false},
		{"ue: x\n", 0, 0, 0, false},
	}
	for _, c := range cases {
		ce, ue, de, ok := ParseUMCCounters(c.text)
		if ce != c.ce || ue != c.ue || de != c.de || ok != c.ok {
			t.Errorf("%q: got ce=%d ue=%d de=%d ok=%v, want %d %d %d %v", c.text, ce, ue, de, ok, c.ce, c.ue, c.de, c.ok)
		}
	}
}

// The amd-smi documents of the reference host (ROCm 7.14).
const smiListDoc = `[{"gpu": 0, "bdf": "0000:07:00.0", "uuid": "e30073a1-0000-1000-8044-9e19a70d44c7",
 "kfd_id": 56424, "node_id": 7, "partition_id": 0},
 {"gpu": 8, "bdf": "0000:83:00.0", "uuid": "ed007449-0000-1000-8094-df1c40516659",
 "kfd_id": 18860, "node_id": 2, "partition_id": 0}]`

const smiMetricDoc = `{"gpu_data": [{"gpu": 0, "ecc": {"total_correctable_count": 3,
 "total_uncorrectable_count": 1, "total_deferred_count": 2, "cache_correctable_count": 0,
 "cache_uncorrectable_count": 0}, "ecc_blocks": {"UMC": {"correctable_count": 3,
 "uncorrectable_count": 1, "deferred_count": 2}}}]}`

func TestSMIIndexAndCounters(t *testing.T) {
	idx := smiIndex(smiListDoc)
	if len(idx) != 2 || idx["0000:07:00.0"] != 0 || idx["0000:83:00.0"] != 8 {
		t.Fatalf("index = %v", idx)
	}
	if smiIndex("not json") != nil {
		t.Error("a broken document must give no index")
	}
	c := smiCounters(smiMetricDoc)
	if got := c[0]; got.ce != 3 || got.ue != 1 || got.de != 2 {
		t.Errorf("counters = %+v", got)
	}
	if len(c) != 1 {
		t.Errorf("counters = %v", c)
	}
}

func TestSMICountersTolerateNA(t *testing.T) {
	doc := `{"gpu_data": [{"gpu": 0, "ecc": {"total_correctable_count": "N/A",
	 "total_uncorrectable_count": "4", "total_deferred_count": null}}]}`
	c := smiCounters(doc)
	if got := c[0]; got.ce != 0 || got.ue != 4 || got.de != 0 {
		t.Errorf("counters = %+v", got)
	}
}
