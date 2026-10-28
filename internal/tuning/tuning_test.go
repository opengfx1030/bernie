package tuning

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/opengfx1030/rdnactl/internal/gpu"
)

const (
	sha714 = "f30bb442e9b5a3c479f6aa60c4d0d0740f67db2957fc0a5c416253e1bf320262" // par1-cs25 venv-7.14.0 wheel
	shaX   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestEmbeddedRegistry(t *testing.T) {
	ps, err := Profiles()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
		if len(p.Ranks) == 0 {
			t.Errorf("%s: no embedded rank files", p.Name)
		}
	}
	if !reflect.DeepEqual(names, []string{"rocm10-rocblas5.6", "rocm7.14-rocblas5.5"}) {
		t.Errorf("profiles = %v", names)
	}
}

func TestProfilesFromRejectsIncompleteEntries(t *testing.T) {
	fsys := fstest.MapFS{"profiles.json": {Data: []byte(`{"profiles": {"p": {"dir": "p", "lib_sha256": "abc"}}}`)}}
	if _, err := profilesFrom(fsys); err == nil || !strings.Contains(err.Error(), `profile "p" needs dir and lib_sha256`) {
		t.Errorf("err = %v", err)
	}
}

func TestSelect(t *testing.T) {
	ps := []Profile{
		{Name: "a", LibSHA256: sha714},
		{Name: "b", LibSHA256: shaX},
	}
	tests := []struct {
		name, sha, explicit string
		want                string // "" = nil profile
		wantErr             string
	}{
		{"auto by prefix", sha714, "", "a", ""},
		{"auto upper case, prefix only", strings.ToUpper(sha714[:12]) + strings.Repeat("0", 52), "", "a", ""},
		{"auto no match: per-user fallback", strings.Repeat("9", 64), "", "", ""},
		{"auto no hash", "", "", "", ""},
		{"explicit matching", shaX, "b", "b", ""},
		{"explicit without hash trusts name", "", "a", "a", ""},
		{"explicit mismatch", shaX, "a", "", "tuned for rocBLAS f30bb442e9b5, but the loaded librocblas.so.5 is 0123456789ab"},
		{"explicit unknown", "", "zzz", "", `"zzz" is not registered`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := selectFrom(ps, tt.sha, tt.explicit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if p != nil {
				got = p.Name
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

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
		{"container rows", p, "/rdnactl/tunableop/a", false, "/rdnactl/tunableop/a/tunableop_results.csv", "0"},
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

func TestPowerStatusAndSet(t *testing.T) {
	// GPU 2 has no hwmon power cap.
	root, gpus := powerTree(t, map[int][3]int{0: {175, 120, 250}, 1: {250, 250, 250}})
	st := powerStatus(root, gpus)
	if len(st) != 2 || st[0].CapW != 175 || st[0].MinW != 120 || st[0].MaxW != 250 {
		t.Fatalf("status = %+v", st)
	}

	tests := []struct {
		name    string
		idx     []int
		watts   float64
		root    bool
		cmds    int
		wantErr string
	}{
		{"non-root returns commands", []int{0}, 150, false, 1, ""},
		{"all GPUs with a cap", nil, 250, false, 2, ""},
		{"below floor", []int{1}, 150, false, 0, "below power1_cap_min 250 W"},
		{"above max", []int{0}, 300, false, 0, "above power1_cap_max 250 W"},
		{"no cap file", []int{2}, 150, false, 0, "GPU 2 has no power cap"},
		{"not positive", []int{0}, 0, false, 0, "positive number of watts"},
		{"root writes", []int{0}, 150, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds, err := setPower(root, gpus, tt.idx, tt.watts, tt.root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(cmds) != tt.cmds {
				t.Fatalf("cmds = %q", cmds)
			}
			if tt.cmds > 0 && !strings.HasPrefix(cmds[0], "echo ") {
				t.Errorf("cmd = %q", cmds[0])
			}
		})
	}
	// Non-root never writes; root writes microwatts.
	b, _ := os.ReadFile(st[0].Path)
	if strings.TrimSpace(string(b)) != "150000000" {
		t.Errorf("power1_cap = %q after root set", b)
	}
	cmds, _ := setPower(root, gpus, []int{0}, 140.5, false)
	if cmds[0] != "echo 140500000 | sudo tee "+st[0].Path {
		t.Errorf("cmd = %q", cmds[0])
	}
}
