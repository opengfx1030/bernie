package rocm

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// par1Tree mirrors the ROCm layout of par1-cs25: /opt/rocm-7.2.0 (7.2.0),
// a TheRock tree nested at /opt/rocm-7.2.0/core-7.14 (7.14.1), and
// /opt/rocm and /opt/rocm/core-7.14 pointing at it through absolute
// alternatives symlinks.
func par1Tree(t *testing.T) string {
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		"opt/rocm-7.2.0/.info/version":           "7.2.0-43\n",
		"opt/rocm-7.2.0/bin/hipcc":               "",
		"opt/rocm-7.2.0/core-7.14/.info/version": "7.14.1\n",
		"opt/rocm-7.2.0/core-7.14/bin/hipcc":     "",
		"opt/rocm-7.2.0/core-7.14/bin/rocminfo":  "",
	})
	link := func(target, name string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	link("/opt/rocm-7.2.0/core-7.14", "etc/alternatives/rocm")
	link("/etc/alternatives/rocm", "opt/rocm")
	return root
}

func TestFindInstalls(t *testing.T) {
	root := par1Tree(t)
	got := FindInstalls(root)
	want := []Install{{
		Root: "/opt/rocm-7.2.0", Version: "7.2.0", Kind: "opt",
		Nested: []Install{{Root: "/opt/rocm-7.2.0/core-7.14", Version: "7.14.1", Kind: "therock"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindInstalls = %+v, want %+v", got, want)
	}
	// Tools fall back to the newest install's bin dir.
	if p := installTool(root, got, "hipcc"); p != filepath.Join(root, "opt/rocm-7.2.0/core-7.14/bin/hipcc") {
		t.Errorf("hipcc = %s", p)
	}
}

func TestFindInstallsDistro(t *testing.T) {
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		"usr/lib64/libamdhip64.so.6":                      "",
		"usr/include/hip/hip_version.h":                   "#define HIP_VERSION_MAJOR 6\n#define HIP_VERSION_MINOR 3\n#define HIP_VERSION_PATCH 42131\n",
		"opt/rocm-6.4.1/include/rocm-core/rocm_version.h": "#define ROCM_VERSION_MAJOR 6\n#define ROCM_VERSION_MINOR 4\n#define ROCM_VERSION_PATCH 1\n",
		"opt/rocm-6.4.1/bin/rocminfo":                     "",
	})
	want := []Install{
		{Root: "/opt/rocm-6.4.1", Version: "6.4.1", Kind: "opt"},
		{Root: "/usr", Version: "6.3.42131", Kind: "distro"},
	}
	if got := FindInstalls(root); !reflect.DeepEqual(got, want) {
		t.Errorf("FindInstalls = %+v, want %+v", got, want)
	}
}

func TestParseRocminfoAgents(t *testing.T) {
	b, err := os.ReadFile("testdata/rocminfo-par1-cs25.txt")
	if err != nil {
		t.Fatal(err)
	}
	// CPU agent and ISA names (amdgcn-amd-amdhsa--gfx...) are not agents.
	want := []string{"gfx1100", "gfx1100", "gfx1030"}
	if got := ParseRocminfoAgents(string(b)); !reflect.DeepEqual(got, want) {
		t.Errorf("agents = %v", got)
	}
}

func TestParseHIPVersion(t *testing.T) {
	out := "HIP version: 7.14.60850-0000000\nAMD clang version 23.0.0git\n"
	if got := ParseHIPVersion(out); got != "7.14.60850" {
		t.Errorf("got %q", got)
	}
	if got := ParseHIPVersion("clang version 18"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"7.2.0", "7.2.1", -1},
		{"7.14.1", "7.2.0", 1},
		{"7.13", "7.13.0", 0},
		{"7.2.0-43", "7.2.0", 0},
		{"2.12.0+rocm7.14.0", "2.12.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s,%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVenvOutput(t *testing.T) {
	out := "INFO 10-28 vllm noise on stdout\nBERNIE_JSON:{\"python\": \"3.12.13\", \"torch\": \"2.12.0+rocm7.14.0\", \"hip\": \"7.14.60850\", \"device_count\": 10, \"vllm\": \"0.31.0+rdna\", \"rdna_ops\": 42}\n"
	rep := VenvReport{Venv: "/v"}
	if !parseVenvOutput(out, &rep) {
		t.Fatal("not parsed")
	}
	want := VenvReport{Venv: "/v", Python: "3.12.13", Torch: "2.12.0+rocm7.14.0", HIP: "7.14.60850", VLLM: "0.31.0+rdna", DeviceCount: 10, RDNAOps: 42}
	if rep != want {
		t.Errorf("got %+v", rep)
	}
	if parseVenvOutput("Traceback ...\n", &rep) {
		t.Error("garbage parsed")
	}
}
