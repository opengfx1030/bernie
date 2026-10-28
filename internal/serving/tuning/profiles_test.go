package tuning

import (
	"io/fs"
	"path"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/opengfx1030/bernie/tunableop"
	tunedmoe "github.com/opengfx1030/bernie/tuned-moe"
)

const (
	sha714 = "f30bb442e9b5a3c479f6aa60c4d0d0740f67db2957fc0a5c416253e1bf320262" // par1-cs25 venv-7.14.0 wheel
	shaX   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestEmbeddedProfiles(t *testing.T) {
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
		if !reflect.DeepEqual(p.Arch, []string{"gfx1030"}) {
			t.Errorf("%s: arch = %v", p.Name, p.Arch)
		}
		if !strings.HasPrefix(p.Dir, "gfx1030/") {
			t.Errorf("%s: dir = %s", p.Name, p.Dir)
		}
	}
	if !reflect.DeepEqual(names, []string{"rocm10-rocblas5.6", "rocm7.14-rocblas5.5"}) {
		t.Errorf("profiles = %v", names)
	}
}

func manifestFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, body := range files {
		m[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

func TestProfilesFromWalksAndValidates(t *testing.T) {
	good := `{"name": "p", "arch": ["gfx11"], "lib_sha256": "` + sha714 + `"}`
	ps, err := profilesFrom(manifestFS(map[string]string{
		"gfx1100/p/profile.json":           good,
		"gfx1100/p/tunableop_results1.csv": "x",
		"gfx1100/p/tunableop_results0.csv": "x",
		"gfx1100/p/README.md":              "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Dir != "gfx1100/p" || !reflect.DeepEqual(ps[0].Ranks, []string{"tunableop_results0.csv", "tunableop_results1.csv"}) {
		t.Fatalf("profiles = %+v", ps)
	}
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"name differs from folder", map[string]string{"gfx1100/q/profile.json": good}, `name "p" must equal the folder name "q"`},
		{"no arch", map[string]string{"gfx1100/p/profile.json": `{"name": "p", "lib_sha256": "` + sha714 + `"}`}, "arch is required"},
		{"short hash", map[string]string{"gfx1100/p/profile.json": `{"name": "p", "arch": ["gfx11"], "lib_sha256": "abc"}`}, "lib_sha256 is required"},
		{"bad json", map[string]string{"gfx1100/p/profile.json": `{`}, "gfx1100/p/profile.json"},
		{"duplicate name", map[string]string{"gfx1100/p/profile.json": good, "gfx1030/p/profile.json": good}, `profile "p" is defined twice`},
	}
	for _, tt := range tests {
		if _, err := profilesFrom(manifestFS(tt.files)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestSelect(t *testing.T) {
	ps := []Profile{
		{Name: "a", LibSHA256: sha714, Arch: []string{"gfx1030"}},
		{Name: "b", LibSHA256: shaX, Arch: []string{"gfx103"}},
		{Name: "c", LibSHA256: shaX, Arch: []string{"gfx11"}},
	}
	g1030, g1100 := []string{"gfx1030"}, []string{"gfx1100"}
	tests := []struct {
		name, sha, explicit string
		archs               []string
		want                string // "" = nil profile
		wantErr             string
	}{
		{"auto by prefix", sha714, "", g1030, "a", ""},
		{"auto upper case, prefix only", strings.ToUpper(sha714[:12]) + strings.Repeat("0", 52), "", g1030, "a", ""},
		{"auto no match: per-user fallback", strings.Repeat("9", 64), "", g1030, "", ""},
		{"auto no hash", "", "", g1030, "", ""},
		{"auto same build, arch picks the profile", shaX, "", g1100, "c", ""},
		{"auto same build, arch prefix", shaX, "", []string{"gfx1031"}, "b", ""},
		{"auto hash of another arch: per-user fallback", sha714, "", g1100, "", ""},
		{"auto mixed arch fits nothing", shaX, "", []string{"gfx1030", "gfx1100"}, "", ""},
		{"auto unknown arch does not filter", sha714, "", nil, "a", ""},
		{"explicit matching", shaX, "b", g1030, "b", ""},
		{"explicit without hash trusts name", "", "a", g1030, "a", ""},
		{"explicit mismatch", shaX, "a", g1030, "", "tuned for rocBLAS f30bb442e9b5, but the loaded librocblas.so.5 is 0123456789ab"},
		{"explicit wrong arch", "", "a", g1100, "", "tuned for gfx1030, but the selected GPUs are gfx1100"},
		{"explicit unknown", "", "zzz", g1030, "", `"zzz" is not registered`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := selectFrom(ps, tt.sha, tt.explicit, tt.archs)
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

// TestDataTreeLayout enforces spec §2.1 on the tuning data: the tree is
// keyed by architecture, and a folder holds one role (a profile folder with
// its manifest and its rows, or an architecture folder with its configs).
func TestDataTreeLayout(t *testing.T) {
	for name, fsys := range map[string]fs.FS{"tunableop": tunableop.FS, "tuned-moe": tunedmoe.FS} {
		err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			es, err := fs.ReadDir(fsys, p)
			if err != nil {
				return err
			}
			if p != "." && !strings.Contains(p, "/") && !strings.HasPrefix(p, "gfx") {
				t.Errorf("%s/%s: a top-level folder must be a gfx architecture", name, p)
			}
			for _, e := range es {
				if e.IsDir() {
					continue
				}
				switch path.Ext(e.Name()) {
				case ".json", ".csv", ".md":
					// a manifest, a row of the profile, or its README
				default:
					t.Errorf("%s/%s/%s: a profile folder holds a manifest, rows and a README", name, p, e.Name())
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
