package tuning

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// PowerDesiredVersion is the version of <config>/power.yaml.
const PowerDesiredVersion = 1

// PowerDesired is the desired GPU power state. GPUs maps a selector to
// watts. A selector is "all", a HIP index, a PCI address or a KFD unique id.
type PowerDesired struct {
	Version int                `yaml:"version" json:"version"`
	GPUs    map[string]float64 `yaml:"gpus" json:"gpus"`
}

// NewPowerDesired returns an empty desired state.
func NewPowerDesired() PowerDesired {
	return PowerDesired{Version: PowerDesiredVersion, GPUs: map[string]float64{}}
}

// LoadPowerDesired reads a desired-state file. A missing file is an empty
// desired state. Unknown YAML fields and invalid values are errors.
func LoadPowerDesired(path string) (PowerDesired, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewPowerDesired(), nil
	}
	if err != nil {
		return PowerDesired{}, err
	}
	var d PowerDesired
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return PowerDesired{}, fmt.Errorf("read %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("more than one YAML document")
		}
		return PowerDesired{}, fmt.Errorf("read %s: %w", path, err)
	}
	if d.Version == 0 {
		d.Version = PowerDesiredVersion
	}
	if d.GPUs == nil {
		d.GPUs = map[string]float64{}
	}
	if err := d.Validate(); err != nil {
		return PowerDesired{}, fmt.Errorf("read %s: %w", path, err)
	}
	return d, nil
}

// SavePowerDesired writes the desired state atomically. It creates the parent
// directory and never uses /tmp.
func SavePowerDesired(path string, d PowerDesired) error {
	if d.Version == 0 {
		d.Version = PowerDesiredVersion
	}
	if d.GPUs == nil {
		d.GPUs = map[string]float64{}
	}
	if err := d.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	return writeAtomic(path, b, 0o644)
}

// Validate checks the version, selectors and watt values.
func (d PowerDesired) Validate() error {
	if d.Version != PowerDesiredVersion {
		return fmt.Errorf("power.yaml version must be %d, got %d", PowerDesiredVersion, d.Version)
	}
	for selector, watts := range d.GPUs {
		if !validPowerSelector(selector) {
			return fmt.Errorf("power.yaml has invalid GPU selector %q", selector)
		}
		if watts <= 0 || math.IsNaN(watts) || math.IsInf(watts, 0) {
			return fmt.Errorf("power.yaml value for %q must be a positive number of watts", selector)
		}
	}
	return nil
}

// Set stores one selector. It reports whether the value changed.
func (d *PowerDesired) Set(selector string, watts float64) bool {
	if d.GPUs == nil {
		d.GPUs = map[string]float64{}
	}
	old, ok := d.GPUs[selector]
	d.GPUs[selector] = watts
	return !ok || old != watts
}

// Resolve returns the desired watts for each HIP index. A GPU-specific
// selector overrides "all". Unknown selectors are ignored.
func (d PowerDesired) Resolve(root string, gpus []gpu.GPU) map[int]float64 {
	out := map[int]float64{}
	all, hasAll := d.GPUs["all"]
	for _, g := range gpus {
		if hasAll {
			out[g.HIPIndex] = all
		}
		for _, selector := range gpuPowerSelectors(root, g) {
			if watts, ok := d.GPUs[selector]; ok {
				out[g.HIPIndex] = watts
				break
			}
		}
	}
	return out
}

// gpuPowerSelectors lists the selectors of one GPU, most specific first.
func gpuPowerSelectors(root string, g gpu.GPU) []string {
	var out []string
	props, err := gpu.ReadProperties(filepath.Join(root, gpu.KFDNodesDir, strconv.Itoa(g.KFDNode), "properties"))
	if err == nil && props["unique_id"] > 0 {
		out = append(out, strconv.FormatInt(props["unique_id"], 10))
	}
	if g.PCI != "" {
		out = append(out, g.PCI)
	}
	return append(out, strconv.Itoa(g.HIPIndex))
}

func validPowerSelector(s string) bool {
	if s == "all" {
		return true
	}
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		return true
	}
	return gpu.IsBDF(strings.ToLower(s))
}

// writeAtomic writes a file in its target directory and renames it into
// place. The temporary file is beside the target, never under /tmp.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".bernie-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
