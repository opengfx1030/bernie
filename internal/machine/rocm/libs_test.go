package rocm

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/opengfx1030/rdnactl/internal/support/fixture"
)

// venvTree mirrors the venv-7.14.0 layout on par1-cs25 (TheRock wheels:
// no _rocm_sdk_core/lib/core/lib directory).
func venvTree(t *testing.T) string {
	venv := t.TempDir()
	sp := "lib/python3.12/site-packages/"
	fixture.Write(t, venv, map[string]string{
		"pyvenv.cfg": "version = 3.12.13\n",
		sp + "_rocm_sdk_libraries/lib/librocblas.so.5":                    "wheel",
		sp + "_rocm_sdk_core/lib/host-math/lib/librocm-openblas.so.0":     "x",
		sp + "_rocm_sdk_core/lib/rocm_sysdeps/lib/librocm_sysdeps_z.so.1": "x",
		sp + "_rocm_sdk_core/lib/libamdhip64.so.7":                        "x",
		sp + "torch/lib/libtorch_hip.so":                                  "x",
	})
	return venv
}

func TestVenvLibPaths(t *testing.T) {
	venv := venvTree(t)
	sp := filepath.Join(venv, "lib/python3.12/site-packages")
	want := []string{
		filepath.Join(sp, "_rocm_sdk_libraries/lib"),
		filepath.Join(sp, "_rocm_sdk_core/lib"), // wheel HIP runtime (libamdhip64.so.7)
		filepath.Join(sp, "_rocm_sdk_core/lib/host-math/lib"),
		filepath.Join(sp, "_rocm_sdk_core/lib/rocm_sysdeps/lib"),
		filepath.Join(sp, "torch/lib"),
	}
	if got := VenvLibPaths(venv); !reflect.DeepEqual(got, want) {
		t.Errorf("VenvLibPaths =\n%v\nwant\n%v", got, want)
	}
	if got := VenvLibPaths(""); got != nil {
		t.Errorf("empty venv: %v", got)
	}
}

func TestFindROCBLAS(t *testing.T) {
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{"opt/rocm/lib/librocblas.so.5": "system"})
	venv := venvTree(t)
	plain := t.TempDir() // venv without ROCm wheels (system torch build)
	fixture.Write(t, plain, map[string]string{"lib/python3.11/site-packages/torch/lib/libtorch.so": "x"})

	cases := []struct {
		name, root, venv, want string
	}{
		{"wheel wins over system", root, venv, filepath.Join(venv, "lib/python3.12/site-packages/_rocm_sdk_libraries/lib/librocblas.so.5")},
		{"system when venv has none", root, plain, filepath.Join(root, "opt/rocm/lib/librocblas.so.5")},
		{"system without venv", root, "", filepath.Join(root, "opt/rocm/lib/librocblas.so.5")},
		{"nothing", t.TempDir(), "", ""},
	}
	for _, c := range cases {
		if got := FindROCBLAS(c.root, c.venv); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
