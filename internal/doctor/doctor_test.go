package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

func find(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %s missing; have %v", id, ids(checks))
	return Check{}
}

func ids(checks []Check) []string {
	var s []string
	for _, c := range checks {
		s = append(s, c.ID)
	}
	return s
}

func v620s(n int, capW float64) []gpu.GPU {
	var g []gpu.GPU
	for i := range n {
		g = append(g, gpu.GPU{HIPIndex: i, GFX: "gfx1030", DeviceID: "0x73a1", PowerCapW: capW, Link: gpu.Link{Gen: 4, Width: 16}})
	}
	return g
}

// base is a healthy par1-like host; cases mutate it.
func base() inputs {
	return inputs{
		root: "/",
		host: host.Info{
			OS: host.OSRelease{ID: "fedora", VersionID: "43", Pretty: "Fedora Linux 43"}, PkgManager: "dnf",
			Kernel: "7.1.7", Cmdline: "ro iommu=pt", RAMBytes: 512 * gib, ShmBytes: 64 * gib, SELinux: "enforcing",
			User:    host.User{Name: "op", Groups: []string{"op", "video", "render"}, GIDs: []int{1000, 44, 105}},
			Engines: []host.Engine{{Name: "docker", Version: "29.8.1", Usable: true, Compose: "docker compose"}},
		},
		gpus:      v620s(4, 180),
		validated: []string{"gfx1030"},
		rocm: rocm.Report{
			Installs: []rocm.Install{{Root: "/opt/rocm-7.14.0", Version: "7.14.0", Kind: "opt"}},
			Tools:    map[string]string{"rocminfo": "/usr/bin/rocminfo", "hipcc": "/usr/bin/hipcc", "amd-smi": "/usr/bin/amd-smi"},
			Agents:   []string{"gfx1030", "gfx1030", "gfx1030", "gfx1030"},
		},
		rocblas: "/opt/rocm/lib/librocblas.so.5",
		libSHA:  "f30bb442e9b5a3c479f6aa60c4d0d0740f67db2957fc0a5c416253e1bf320262",
		match:   &tuning.Profile{Name: "rocm7.14-rocblas5.5", ROCBLAS: "5.5"},
		hfCache: "/data/hf", hfFree: 500 * gib,
		amdgpu:    true,
		groupGIDs: map[string]int{"video": 44, "render": 105},
		devices:   devices{kfdExists: true, kfdRW: true, render: []string{"/dev/dri/renderD128"}, renderRW: true},
	}
}

func TestEvaluateStatuses(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*inputs)
		id     string
		status Status
		detail string // substring of detail or fix
	}{
		{"healthy host", func(*inputs) {}, "gpu.count", OK, "4× gfx1030"},
		{"no os-release is a warn", func(in *inputs) { in.host.OS = host.OSRelease{} }, "os.distro", Warn, "no /etc/os-release"},
		{"two installs warn", func(in *inputs) {
			in.rocm.Installs = append(in.rocm.Installs, rocm.Install{Root: "/opt/rocm-7.2.0", Version: "7.2.0", Kind: "opt"})
		}, "rocm.installs", Warn, "2 installs"},
		{"no install, no alternative", func(in *inputs) {
			in.rocm.Installs = nil
			in.host.Engines = nil
		}, "rocm.installs", Fail, "sudo dnf install -y rocm rocm-hip-runtime-devel"},
		{"no install but container works", func(in *inputs) { in.rocm.Installs = nil }, "rocm.installs", Warn, "container mode"},
		{"RCCL deadlock range multi-GPU", func(in *inputs) {
			in.rocm.Installs[0].Version = "7.2.1"
		}, "rocm.version", Warn, "deadlock"},
		{"deadlock range from venv torch", func(in *inputs) {
			in.venvRep.Torch = "2.10.0+rocm7.13.0"
		}, "rocm.version", Warn, "venv torch ROCm 7.13.0"},
		{"7.2.0 is safe", func(in *inputs) { in.rocm.Installs[0].Version = "7.2.0" }, "rocm.version", OK, ""},
		{"rocminfo partial", func(in *inputs) { in.rocm.Agents = in.rocm.Agents[:2] }, "rocm.rocminfo", Warn, "2 of 4"},
		{"rocminfo missing skip", func(in *inputs) { delete(in.rocm.Tools, "rocminfo") }, "rocm.rocminfo", Skip, "sudo dnf install -y rocminfo"},
		{"groups missing but world-rw devices", func(in *inputs) { in.host.User.GIDs = []int{1000, 44} }, "gpu.groups", Warn, "render"},
		{"groups held under other names (LDAP gid match)", func(in *inputs) {
			in.host.User.Groups = []string{"op", "105", "44"}
		}, "gpu.groups", OK, ""},
		{"group not found on host is a warn, not a fail", func(in *inputs) {
			delete(in.groupGIDs, "render")
			in.host.User.GIDs = []int{1000, 44}
			in.devices.kfdRW = false
		}, "gpu.groups", Warn, "group render not found"},
		{"groups missing and kfd denied", func(in *inputs) {
			in.host.User.GIDs = []int{1000}
			in.devices.kfdRW = false
		}, "gpu.groups", Fail, "usermod -aG video,render"},
		{"no kfd", func(in *inputs) { in.devices = devices{} }, "gpu.kfd", Fail, "modprobe amdgpu"},
		{"gfx1031 override advice", func(in *inputs) { in.gpus[0].GFX = "gfx1031" }, "gpu.hsa_override.gfx1031", Warn, "HSA_OVERRIDE_GFX_VERSION=10.3.0"},
		{"gfx1031 unvalidated", func(in *inputs) { in.gpus[0].GFX = "gfx1031" }, "gpu.supported.gfx1031", Warn, "RDNA2 target; recipes are validated on gfx1030 only"},
		{"validated arch", func(*inputs) {}, "gpu.supported.gfx1030", OK, "recipes are validated on it"},
		{"known but unvalidated arch", func(in *inputs) { in.gpus[0].GFX = "gfx1100" }, "gpu.supported.gfx1100", Warn, "RDNA3 target; recipes are validated on gfx1030 only"},
		{"unknown arch", func(in *inputs) { in.gpus[0].GFX = "gfx908" }, "gpu.supported.gfx908", Warn, "not in the device tables"},
		{"no validated recipes", func(in *inputs) { in.validated = nil }, "gpu.supported.gfx1030", Warn, "validated on no target yet"},
		{"two validated archs", func(in *inputs) { in.validated = []string{"gfx1030", "gfx1100"}; in.gpus[0].GFX = "gfx1102" }, "gpu.supported.gfx1102", Warn, "validated on gfx1030, gfx1100 only"},
		{"igpu mixed", func(in *inputs) { in.gpus = append(in.gpus, gpu.GPU{HIPIndex: 4, GFX: "gfx1036"}) }, "gpu.igpu", Warn, "HIP_VISIBLE_DEVICES=0,1,2,3"},
		{"iommu=pt missing on apt", func(in *inputs) {
			in.host.Cmdline = "ro quiet"
			in.host.PkgManager = "apt"
		}, "kernel.iommu", Warn, "update-grub"},
		{"iommu=pt missing on dnf", func(in *inputs) { in.host.Cmdline = "ro quiet" }, "kernel.iommu", Warn, `grubby --update-kernel=ALL --args="iommu=pt"`},
		{"slow link", func(in *inputs) { in.gpus[1].Link = gpu.Link{Gen: 3, Width: 4} }, "kernel.pcie", Warn, "GPU 1 gen3 x4"},
		{"no engine is skip, not fail", func(in *inputs) { in.host.Engines = nil }, "container.engine", Skip, "host mode works"},
		{"daemon denied", func(in *inputs) { in.host.Engines[0].Usable = false }, "container.daemon", Warn, "usermod -aG docker"},
		{"podman without compose", func(in *inputs) {
			in.host.Engines = []host.Engine{{Name: "podman", Usable: true, Rootless: true}}
		}, "container.compose", Warn, "podman-compose"},
		{"small shm", func(in *inputs) { in.host.ShmBytes = 2 * gib }, "resources.shm", Warn, "2.0 GiB"},
		{"tiny disk", func(in *inputs) { in.hfFree = 5 * gib }, "resources.disk", Fail, "5 GiB"},
		{"tunableop skipped without venv", func(*inputs) {}, "tuning.tunableop", Skip, "pass --venv to check the rocBLAS your server loads"},
		{"tunableop match", func(in *inputs) { in.venv = "/v" }, "tuning.tunableop", OK, "rocm7.14-rocblas5.5"},
		{"tunableop no match", func(in *inputs) { in.venv = "/v"; in.match = nil }, "tuning.tunableop", Warn, "no registered profile"},
		{"V620 cap above advice", func(in *inputs) { in.gpus[2].PowerCapW = 250 }, "tuning.power", Warn, "tune power set 180 --gpu 2"},
		{"no venv skips python", func(*inputs) {}, "python.venv", Skip, "--venv"},
		{"venv without vllm", func(in *inputs) {
			in.venv = "/v"
			in.venvRep = rocm.VenvReport{Python: "3.12.13", Torch: "2.12.0+rocm7.14.0", HIP: "7.14.60850", DeviceCount: 4, Err: "import vllm: No module named 'vllm'"}
		}, "python.vllm", Fail, "No module named"},
		{"venv sees fewer GPUs", func(in *inputs) {
			in.venv = "/v"
			in.venvRep = rocm.VenvReport{Python: "3.12.13", Torch: "2.12.0+rocm7.14.0", HIP: "7.14.60850", DeviceCount: 2, VLLM: "0.31.0", RDNAOps: 68}
		}, "python.gpus", Warn, "torch sees 2, KFD has 4"},
		{"CUDA torch", func(in *inputs) {
			in.venv = "/v"
			in.venvRep = rocm.VenvReport{Python: "3.12.13", Torch: "2.12.0+cu128", DeviceCount: 0}
		}, "python.torch_hip", Fail, "not a ROCm build"},
		{"upstream vllm without fork ops", func(in *inputs) {
			in.venv = "/v"
			in.venvRep = rocm.VenvReport{Python: "3.12.13", Torch: "2.12.0+rocm7.14.0", HIP: "7.14", DeviceCount: 4, VLLM: "0.31.0", RDNAOps: 0}
		}, "python.rdna_ops", Warn, "need > 10"},
	}
	for _, c := range cases {
		in := base()
		c.mutate(&in)
		got := find(t, evaluate(in), c.id)
		if got.Status != c.status || !strings.Contains(got.Detail+" | "+got.Fix, c.detail) {
			t.Errorf("%s: %s = %s %q fix=%q, want %s containing %q", c.name, c.id, got.Status, got.Detail, got.Fix, c.status, c.detail)
		}
	}
}

func TestHealthyHostHasNoFailures(t *testing.T) {
	r := newReport(evaluate(base()))
	if r.Failed() {
		for _, c := range r.Checks {
			if c.Status == Fail {
				t.Errorf("unexpected fail: %+v", c)
			}
		}
	}
	total := 0
	for _, n := range r.Summary {
		total += n
	}
	if total != len(r.Checks) {
		t.Errorf("summary %v does not add up to %d checks", r.Summary, len(r.Checks))
	}
}

func TestRunFixtureCategories(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	r := Run(context.Background(), Options{Root: root, Categories: []string{"gpu", "kernel"}})
	for _, c := range r.Checks {
		if c.Category != "gpu" && c.Category != "kernel" {
			t.Errorf("category filter leaked %s", c.ID)
		}
	}
	if c := find(t, r.Checks, "gpu.count"); c.Status != OK || !strings.Contains(c.Detail, "2× gfx1100, 8× gfx1030") {
		t.Errorf("gpu.count = %+v", c)
	}
	if c := find(t, r.Checks, "kernel.amdgpu"); c.Status != OK {
		t.Errorf("kernel.amdgpu = %+v", c)
	}
	if c := find(t, r.Checks, "kernel.iommu"); c.Status != OK {
		t.Errorf("kernel.iommu = %+v", c)
	}
	if c := find(t, r.Checks, "kernel.pcie"); c.Status != OK || c.Detail != "10× gen4 x16" {
		t.Errorf("kernel.pcie = %+v", c)
	}
}

func TestRunEmptyRoot(t *testing.T) {
	r := Run(context.Background(), Options{Root: t.TempDir(), Categories: []string{"os", "kernel"}})
	for _, id := range []string{"os.distro", "os.pkgmanager"} {
		if c := find(t, r.Checks, id); c.Status != Warn {
			t.Errorf("%s = %+v, want warn", id, c)
		}
	}
	if c := find(t, r.Checks, "os.kernel"); c.Status != Skip {
		t.Errorf("os.kernel = %+v, want skip", c)
	}
}

func TestSupportChecksMixedHost(t *testing.T) {
	in := base()
	in.gpus = append([]gpu.GPU{{HIPIndex: 0, GFX: "gfx1100", DeviceID: "0x7449"}, {HIPIndex: 1, GFX: "gfx1100", DeviceID: "0x7449"}},
		v620s(2, 180)...)
	checks := supportChecks(in)
	got := map[string]Status{}
	for _, c := range checks {
		got[c.ID] = c.Status
	}
	want := map[string]Status{"gpu.supported.gfx1100": Warn, "gpu.supported.gfx1030": OK}
	if len(got) != len(want) || got["gpu.supported.gfx1100"] != Warn || got["gpu.supported.gfx1030"] != OK {
		t.Errorf("checks = %v, want %v (no override check for gfx1100 or gfx1030)", got, want)
	}
}

func TestPowerAdviceFollowsDeviceTable(t *testing.T) {
	in := base()
	// A W7800 at any cap has no advice; the V620 above 180 W does.
	in.gpus = []gpu.GPU{{HIPIndex: 0, GFX: "gfx1100", DeviceID: "0x7449", PowerCapW: 300}, {HIPIndex: 1, GFX: "gfx1030", DeviceID: "0x73a1", PowerCapW: 250}}
	c := find(t, evaluate(in), "tuning.power")
	if c.Status != Warn || !strings.Contains(c.Detail, "Radeon PRO V620 above 180 W: GPU 1 250 W") || strings.Contains(c.Detail+c.Fix, "GPU 0") || c.Fix != "bernie tune power set 180 --gpu 1" {
		t.Errorf("tuning.power = %+v", c)
	}
	in.gpus = in.gpus[:1]
	if c := find(t, evaluate(in), "tuning.power"); c.Status != OK {
		t.Errorf("W7800 power = %+v", c)
	}
}

func TestTritonCheckUsesGenerationData(t *testing.T) {
	in := base()
	in.venv = "/v"
	if c := tritonCheck(in, "3.9.0"); c.Status != Fail || !strings.Contains(c.Detail, "RDNA2") {
		t.Errorf("rdna2 + triton 3.9 = %+v", c)
	}
	if c := tritonCheck(in, "3.7.1"); c.Status != OK {
		t.Errorf("rdna2 + triton 3.7.1 = %+v", c)
	}
	in.gpus = []gpu.GPU{{GFX: "gfx1100"}}
	if c := tritonCheck(in, "3.9.0"); c.Status != OK {
		t.Errorf("rdna3 + triton 3.9 = %+v", c)
	}
}

func TestRunFixtureSupportedChecks(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	r := Run(context.Background(), Options{Root: root, Categories: []string{"gpu"}, ValidatedArchs: []string{"gfx1030"}})
	if c := find(t, r.Checks, "gpu.supported.gfx1030"); c.Status != OK {
		t.Errorf("gfx1030 = %+v", c)
	}
	if c := find(t, r.Checks, "gpu.supported.gfx1100"); c.Status != Warn || !strings.Contains(c.Detail, "validated on gfx1030 only") {
		t.Errorf("gfx1100 = %+v", c)
	}
	for _, fx := range []string{"w7800-single", "rx7900xtx-synthetic"} {
		r := Run(context.Background(), Options{Root: fixture.Root(t, fx), Categories: []string{"gpu"}, ValidatedArchs: []string{"gfx1030"}})
		if c := find(t, r.Checks, "gpu.supported.gfx1100"); c.Status != Warn {
			t.Errorf("%s: %+v", fx, c)
		}
		if c := find(t, r.Checks, "gpu.count"); !strings.Contains(c.Detail, "1 GPUs: 1× gfx1100") {
			t.Errorf("%s: %+v", fx, c)
		}
	}
	r = Run(context.Background(), Options{Root: fixture.Root(t, "rx6800xt-synthetic"), Categories: []string{"gpu"}, ValidatedArchs: []string{"gfx1030"}})
	if c := find(t, r.Checks, "gpu.supported.gfx1030"); c.Status != OK {
		t.Errorf("rx6800xt = %+v", c)
	}
}

func TestP2PCategory(t *testing.T) {
	r := Run(context.Background(), Options{Root: fixture.Root(t, "par1-cs25"), Categories: []string{"topo.p2p"}})
	if len(r.Checks) != 8 {
		t.Fatalf("checks = %d, want 8: %v", len(r.Checks), ids(r.Checks))
	}
	if c := find(t, r.Checks, "topo.p2p.kfd_links"); c.Status != OK || !strings.Contains(c.Detail, "45 of 45") {
		t.Errorf("topo.p2p.kfd_links = %+v", c)
	}
	if c := find(t, r.Checks, "topo.p2p.large_bar"); c.Status != OK || !strings.Contains(c.Detail, "32.0 GiB") {
		t.Errorf("topo.p2p.large_bar = %+v", c)
	}
	if c := find(t, r.Checks, "topo.p2p.link_classes"); c.Status != Warn || c.Fix == "" {
		t.Errorf("topo.p2p.link_classes = %+v", c)
	}
	// The synthetic host without peer links, without iommu=pt and without an
	// ACS override warns on every item that can fail.
	r = Run(context.Background(), Options{Root: fixture.Root(t, "p2p-degraded-synthetic"), Categories: []string{"topo.p2p"}})
	for _, id := range []string{"topo.p2p.kfd_links", "topo.p2p.large_bar", "topo.p2p.iommu", "topo.p2p.acs", "topo.p2p.link_speed"} {
		if c := find(t, r.Checks, id); c.Status != Warn {
			t.Errorf("%s = %+v, want warn", id, c)
		}
	}
}

func TestECCChecksOnFixtures(t *testing.T) {
	r := Run(context.Background(), Options{Root: fixture.Root(t, "v620-ecc"), Categories: []string{"gpu"}})
	c := find(t, r.Checks, "gpu.ecc.0")
	if c.Status != OK || !strings.Contains(c.Detail, "ue=0 ce=0 de=0") {
		t.Errorf("v620-ecc: gpu.ecc.0 = %+v", c)
	}
	r = Run(context.Background(), Options{Root: fixture.Root(t, "ecc-synthetic"), Categories: []string{"gpu"}})
	c = find(t, r.Checks, "gpu.ecc.0")
	if c.Status != Fail || !strings.Contains(c.Detail, "ue=2 ce=5 de=1") || !strings.Contains(c.Fix, "replace it") {
		t.Errorf("ecc-synthetic: gpu.ecc.0 = %+v", c)
	}
}

func TestECCStates(t *testing.T) {
	cases := []struct {
		name   string
		ecc    gpu.ECC
		status Status
		detail string
	}{
		{"clean", gpu.ECC{State: gpu.ECCEnabled, Counters: true}, OK, "ue=0 ce=0 de=0"},
		{"corrected errors", gpu.ECC{State: gpu.ECCEnabled, CE: 5, Counters: true}, Warn, "corrected"},
		{"degraded pages", gpu.ECC{State: gpu.ECCEnabled, DE: 2, Counters: true}, Warn, "degraded"},
		{"uncorrectable errors", gpu.ECC{State: gpu.ECCEnabled, UE: 2, Counters: true}, Fail, "uncorrectable"},
		{"off on a card with ECC", gpu.ECC{State: gpu.ECCDisabled}, Warn, "ECC is off"},
		{"card with ECC, no counters", gpu.ECC{State: gpu.ECCSupported}, Skip, "no RAS counters"},
		{"card without ECC", gpu.ECC{State: gpu.ECCUnsupported}, Skip, "does not count memory errors"},
	}
	for _, c := range cases {
		in := base()
		in.gpus = v620s(1, 180)
		in.ecc = []gpu.ECC{c.ecc}
		got := find(t, evaluate(in), "gpu.ecc.0")
		if got.Status != c.status || !strings.Contains(got.Detail, c.detail) {
			t.Errorf("%s: gpu.ecc.0 = %s %q, want %s containing %q", c.name, got.Status, got.Detail, c.status, c.detail)
		}
	}
}
