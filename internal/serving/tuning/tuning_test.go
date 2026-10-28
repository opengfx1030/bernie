package tuning

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

func TestEnv(t *testing.T) {
	p := &Profile{Name: "a"}
	fallback := PerUserFile()
	tests := []struct {
		name   string
		p      *Profile
		dir    string
		tuning bool
		file   string
		tune   string
	}{
		{"host rows", p, "/data/tunableop/a", false, "/data/tunableop/a/tunableop_results.csv", "0"},
		{"container rows", p, "/bernie/tunableop/a", false, "/bernie/tunableop/a/tunableop_results.csv", "0"},
		{"no profile", nil, "", false, fallback, "0"},
		{"tuning writes per-user file", p, "/data/tunableop/a", true, fallback, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := Env(tt.p, tt.dir, tt.tuning)
			want := map[string]string{
				"PYTORCH_TUNABLEOP_ENABLED":           "1",
				"PYTORCH_TUNABLEOP_TUNING":            tt.tune,
				"PYTORCH_TUNABLEOP_HIPBLASLT_ENABLED": "0",
				"PYTORCH_TUNABLEOP_FILENAME":          tt.file,
			}
			if !reflect.DeepEqual(env, want) {
				t.Errorf("Env = %v, want %v", env, want)
			}
		})
	}
}

func TestMaterializeIdempotent(t *testing.T) {
	fsys := fstest.MapFS{
		"p/tunableop_results0.csv": {Data: []byte("Validator,ROCBLAS_VERSION,x\n")},
		"p/README.md":              {Data: []byte("readme\n")},
	}
	p := &Profile{Name: "p", Dir: "p"}
	dir := filepath.Join(t.TempDir(), "rows", "p")
	if _, err := materialize(fsys, p, dir); err != nil {
		t.Fatal(err)
	}
	rows := filepath.Join(dir, "tunableop_results0.csv")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(rows, old, old); err != nil {
		t.Fatal(err)
	}
	// Same content: the file is not rewritten.
	if _, err := materialize(fsys, p, dir); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(rows); !st.ModTime().Equal(old) {
		t.Error("unchanged rows file was rewritten")
	}
	// Changed content: the embedded rows are restored.
	if err := os.WriteFile(rows, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := materialize(fsys, p, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(rows); string(b) != "Validator,ROCBLAS_VERSION,x\n" {
		t.Errorf("rows = %q", b)
	}
	if _, err := materialize(fsys, &Profile{Name: "q", Dir: "q"}, t.TempDir()); err == nil {
		t.Error("profile without rows materialized")
	}
}

// powerTree builds sysfs hwmon files for GPUs with the given caps (watts).
func powerTree(t *testing.T, caps map[int][3]int) (string, []gpu.GPU) {
	t.Helper()
	root := t.TempDir()
	var gpus []gpu.GPU
	for i := range 3 {
		dev := filepath.Join("sys", "devices", "pci0000:00", "0000:0"+string(rune('a'+i))+":00.0")
		g := gpu.GPU{HIPIndex: i, Name: "V620", SysfsPath: "/" + dev}
		gpus = append(gpus, g)
		c, ok := caps[i]
		if !ok {
			continue
		}
		hw := filepath.Join(root, dev, "hwmon", "hwmon3")
		if err := os.MkdirAll(hw, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, w := range map[string]int{"power1_cap": c[0], "power1_cap_min": c[1], "power1_cap_max": c[2]} {
			if err := os.WriteFile(filepath.Join(hw, name), []byte(strconv.Itoa(w*1000000)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root, gpus
}

func TestPowerStatus(t *testing.T) {
	// GPU 2 has no hwmon power cap.
	root, gpus := powerTree(t, map[int][3]int{0: {175, 120, 250}, 1: {250, 120, 250}})
	st := powerStatus(root, gpus)
	if len(st) != 2 || st[0].CapW != 175 || st[0].MinW != 120 || st[0].MaxW != 250 {
		t.Fatalf("status = %+v", st)
	}
}
