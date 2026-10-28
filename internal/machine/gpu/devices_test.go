package gpu

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

var (
	idRE  = regexp.MustCompile(`^0x[0-9a-f]{4}$`)
	gfxRE = regexp.MustCompile(`^gfx1[0-2][0-9a-f]{2}$`)
	hsaRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

func TestTablesAreValid(t *testing.T) {
	files, err := fs.Glob(devicesFS, "devices/*.yaml")
	if err != nil || len(files) < 4 {
		t.Fatalf("glob = %v, %v", files, err)
	}
	seenID := map[string]string{}
	for _, f := range files {
		// Each file loads alone: its ids are unique and its devices use its arches.
		one := fstest.MapFS{f: {Data: mustRead(t, f)}}
		tb, err := loadTables(one, "devices")
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		stem := strings.TrimSuffix(path.Base(f), ".yaml")
		for gfx, a := range tb.arches {
			if a.Generation != stem {
				t.Errorf("%s: arch %s has generation %q", f, gfx, a.Generation)
			}
			if !gfxRE.MatchString(gfx) {
				t.Errorf("%s: bad gfx name %q", f, gfx)
			}
			if a.WaveSize != 32 && a.WaveSize != 64 {
				t.Errorf("%s: %s wave_size %d", f, gfx, a.WaveSize)
			}
			if a.BF16 != "" && a.BF16 != "native" && a.BF16 != "none" {
				t.Errorf("%s: %s bf16 %q", f, gfx, a.BF16)
			}
			if a.HSAOverride != "" {
				if !hsaRE.MatchString(a.HSAOverride) {
					t.Errorf("%s: %s hsa_override %q", f, gfx, a.HSAOverride)
				}
				if _, ok := tb.arches[a.CodeTarget()]; !ok {
					t.Errorf("%s: %s overrides to %s, which is not an arch of the same file", f, gfx, a.CodeTarget())
				}
			}
		}
		for id, d := range tb.devices {
			if !idRE.MatchString(id) || d.Name == "" || d.GFX == "" {
				t.Errorf("%s: bad row %+v", f, d)
			}
			if prev, dup := seenID[id]; dup {
				t.Errorf("id %s is in %s and %s", id, prev, f)
			}
			seenID[id] = f
		}
	}
	// The embedded set loads as a whole.
	if len(tables().devices) != len(seenID) {
		t.Errorf("embedded table has %d devices, files have %d", len(tables().devices), len(seenID))
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fs.ReadFile(devicesFS, name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLookup(t *testing.T) {
	cases := []struct {
		id, name, gfx, gen string
		capW               int
	}{
		{"0x73a1", "Radeon PRO V620", "gfx1030", "rdna2", 180},
		{"0x73A1", "Radeon PRO V620", "gfx1030", "rdna2", 180},
		{"73bf", "Radeon RX 6800 / 6800 XT / 6900 XT", "gfx1030", "rdna2", 0},
		{"0x73df", "Radeon RX 6700 / 6700 XT / 6750 XT", "gfx1031", "rdna2", 0},
		{"0x7449", "Radeon PRO W7800 48GB", "gfx1100", "rdna3", 0},
		{"0x744c", "Radeon RX 7900 XT / 7900 XTX / 7900 GRE", "gfx1100", "rdna3", 0},
		{"0x7550", "Radeon RX 9070 / 9070 XT / 9070 GRE", "gfx1201", "rdna4", 0},
		{"0x7590", "Radeon RX 9060 XT / 9050", "gfx1200", "rdna4", 0},
	}
	for _, c := range cases {
		d, ok := Lookup(c.id)
		if !ok || d.Name != c.name || d.GFX != c.gfx || d.Generation != c.gen || d.PowerCapAdviceW != c.capW {
			t.Errorf("Lookup(%s) = %+v, %v", c.id, d, ok)
		}
	}
	if _, ok := Lookup("0xffff"); ok {
		t.Error("unknown id found")
	}
	if _, ok := Lookup(""); ok {
		t.Error("empty id found")
	}
}

func TestPowerCapAdviceIsV620Only(t *testing.T) {
	for id, d := range tables().devices {
		if d.PowerCapAdviceW != 0 && id != "0x73a1" {
			t.Errorf("%s (%s) has power cap advice", id, d.Name)
		}
	}
}

func TestLookupArch(t *testing.T) {
	a, ok := LookupArch("gfx1032")
	if !ok || a.HSAOverride != "10.3.0" || a.CodeTarget() != "gfx1030" || a.BF16 != "none" || a.TritonBelow != "3.9" || a.Label() != "RDNA2" {
		t.Errorf("gfx1032 = %+v, %v", a, ok)
	}
	a, _ = LookupArch("gfx1100")
	if a.HSAOverride != "" || a.CodeTarget() != "gfx1100" || a.BF16 != "native" || !a.FlashAttn || a.Integrated {
		t.Errorf("gfx1100 = %+v", a)
	}
	if a, _ := LookupArch("gfx1030"); a.FlashAttn || a.CodeTarget() != "gfx1030" {
		t.Errorf("gfx1030 = %+v", a)
	}
	if a, _ := LookupArch("gfx1035"); !a.Integrated {
		t.Errorf("gfx1035 = %+v", a)
	}
	if a, _ := LookupArch("gfx1151"); !a.Integrated || a.Generation != "rdna3.5" {
		t.Errorf("gfx1151 = %+v", a)
	}
	if _, ok := LookupArch("gfx908"); ok {
		t.Error("gfx908 is not an RDNA target")
	}
}

func TestLoadTablesRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"duplicate id":  "generation: g\nwave_size: 32\narches: {gfx1030: {}}\ndevices:\n  - {ids: [\"0x0001\", \"0x0001\"], name: A, gfx: gfx1030}\n",
		"unknown gfx":   "generation: g\nwave_size: 32\narches: {gfx1030: {}}\ndevices:\n  - {ids: [\"0x0001\"], name: A, gfx: gfx9999}\n",
		"unknown field": "generation: g\nwave_size: 32\nbogus: 1\n",
		"upper-case id": "generation: g\nwave_size: 32\narches: {gfx1030: {}}\ndevices:\n  - {ids: [\"0x00AB\"], name: A, gfx: gfx1030}\n",
	}
	for name, doc := range cases {
		if _, err := loadTables(fstest.MapFS{"devices/x.yaml": {Data: []byte(doc)}}, "devices"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	two := fstest.MapFS{
		"devices/a.yaml": {Data: []byte("generation: a\nwave_size: 32\narches: {gfx1030: {}}\n")},
		"devices/b.yaml": {Data: []byte("generation: b\nwave_size: 32\narches: {gfx1030: {}}\n")},
	}
	if _, err := loadTables(two, "devices"); err == nil {
		t.Error("an arch in two files was accepted")
	}
}

func TestCodeTargets(t *testing.T) {
	got := CodeTargets([]string{"gfx1031", "gfx1100", "gfx1030", "gfx1032", "gfx908", "", "gfx1100"})
	if want := "gfx1030 gfx1100 gfx908"; strings.Join(got, " ") != want {
		t.Errorf("CodeTargets = %v, want %s", got, want)
	}
	if len(CodeTargets(nil)) != 0 {
		t.Error("nil input gave targets")
	}
}
