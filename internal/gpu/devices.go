package gpu

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Arch is what the device tables know about one gfx target. A target
// belongs to one generation (devices/<generation>.yaml).
type Arch struct {
	GFX        string // gfx1030
	Generation string // rdna2, rdna3, rdna3.5, rdna4
	Family     string // die codename (navi21), "" when not sourced
	WaveSize   int
	// BF16 is "native" (BF16 matrix instructions), "none", or "" when the
	// tables do not say.
	BF16 string
	// HSAOverride is the HSA_OVERRIDE_GFX_VERSION that makes the target run
	// the code objects of a sibling target ("10.3.0"), or "".
	HSAOverride string
	// TritonBelow is set when Triton from this version on dropped the
	// generation.
	TritonBelow string
	Integrated  bool // APU target: shares memory with the CPU
	FlashAttn   bool // on the FlashAttention ROCm allowlist
}

// Device is one PCI device id of the tables.
type Device struct {
	Arch
	ID   string // lower-case "0x73a1"
	Name string
	// PowerCapAdviceW is the recommended power cap, 0 when there is no advice.
	PowerCapAdviceW int
}

// CodeTarget is the gfx target whose code objects the GPU runs once
// HSAOverride is set; it is GFX when there is no override.
func (a Arch) CodeTarget() string {
	var maj, min, step int
	if _, err := fmt.Sscanf(a.HSAOverride, "%d.%d.%d", &maj, &min, &step); err != nil {
		return a.GFX
	}
	return GFXName(maj*10000 + min*100 + step)
}

// Label is the generation as written in advice ("RDNA2").
func (a Arch) Label() string { return strings.ToUpper(a.Generation) }

// Lookup returns the table row of a PCI device id ("0x73a1", case and 0x
// prefix are optional).
func Lookup(deviceID string) (Device, bool) {
	d, ok := tables().devices[normalizeID(deviceID)]
	return d, ok
}

// LookupArch returns the table entry of a gfx target ("gfx1030").
func LookupArch(gfx string) (Arch, bool) {
	a, ok := tables().arches[gfx]
	return a, ok
}

func normalizeID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if n, err := strconv.ParseUint(strings.TrimPrefix(id, "0x"), 16, 32); err == nil {
		return fmt.Sprintf("0x%04x", n)
	}
	return id
}

type table struct {
	devices map[string]Device
	arches  map[string]Arch
}

// tables loads and validates the embedded files once. The files ship in the
// binary, so a malformed one is a build defect that the tests catch: it
// panics instead of returning an error to every caller.
var tables = sync.OnceValue(func() *table {
	t, err := loadTables(devicesFS, "devices")
	if err != nil {
		panic("gpu: device tables: " + err.Error())
	}
	return t
})

// tableFile is the YAML document of one generation.
type tableFile struct {
	Generation  string `yaml:"generation"`
	WaveSize    int    `yaml:"wave_size"`
	TritonBelow string `yaml:"triton_below"`
	Arches      map[string]struct {
		Family      string `yaml:"family"`
		BF16        string `yaml:"bf16"`
		HSAOverride string `yaml:"hsa_override"`
		Integrated  bool   `yaml:"integrated"`
		FlashAttn   bool   `yaml:"flash_attn"`
	} `yaml:"arches"`
	Devices []struct {
		IDs             []string `yaml:"ids"`
		Name            string   `yaml:"name"`
		GFX             string   `yaml:"gfx"`
		PowerCapAdviceW int      `yaml:"power_cap_advice_w"`
	} `yaml:"devices"`
}

// loadTables walks dir of fsys for *.yaml generation files.
func loadTables(fsys fs.FS, dir string) (*table, error) {
	t := &table{devices: map[string]Device{}, arches: map[string]Arch{}}
	err := fs.WalkDir(fsys, dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || path.Ext(p) != ".yaml" {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		var f tableFile
		if err := dec.Decode(&f); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if err := t.add(f); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		return nil
	})
	return t, err
}

func (t *table) add(f tableFile) error {
	for gfx, a := range f.Arches {
		if _, dup := t.arches[gfx]; dup {
			return fmt.Errorf("duplicate arch %s", gfx)
		}
		t.arches[gfx] = Arch{GFX: gfx, Generation: f.Generation, Family: a.Family, WaveSize: f.WaveSize, BF16: a.BF16,
			HSAOverride: a.HSAOverride, TritonBelow: f.TritonBelow, Integrated: a.Integrated, FlashAttn: a.FlashAttn}
	}
	for _, d := range f.Devices {
		arch, ok := t.arches[d.GFX]
		if !ok || arch.Generation != f.Generation {
			return fmt.Errorf("%s: gfx %q is not an arch of %s", d.Name, d.GFX, f.Generation)
		}
		for _, id := range d.IDs {
			if n := normalizeID(id); n != id {
				return fmt.Errorf("%s: id %q must be written as %q", d.Name, id, n)
			}
			if _, dup := t.devices[id]; dup {
				return fmt.Errorf("duplicate device id %s", id)
			}
			t.devices[id] = Device{Arch: arch, ID: id, Name: d.Name, PowerCapAdviceW: d.PowerCapAdviceW}
		}
	}
	return nil
}
