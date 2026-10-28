package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu/devices"
)

// KFDNodesDir is the KFD topology node directory, relative to the root.
const KFDNodesDir = "sys/class/kfd/kfd/topology/nodes"

// List returns the GPUs of the KFD topology under root, in KFD node order.
// HIPIndex is the position among GPU nodes (simd_count > 0).
func List(root string) ([]GPU, error) {
	root = canonRoot(root)
	nodesDir := filepath.Join(root, KFDNodesDir)
	entries, err := os.ReadDir(nodesDir)
	if err != nil {
		return nil, fmt.Errorf("read KFD topology: %w", err)
	}
	var ids []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			ids = append(ids, n)
		}
	}
	sort.Ints(ids)

	var gpus []GPU
	for _, id := range ids {
		props, err := ReadProperties(filepath.Join(nodesDir, strconv.Itoa(id), "properties"))
		if err != nil || props["simd_count"] <= 0 {
			continue
		}
		g := GPU{
			HIPIndex:         len(gpus),
			KFDNode:          id,
			GFXTargetVersion: int(props["gfx_target_version"]),
			RenderMinor:      int(props["drm_render_minor"]),
			NUMANode:         -1,
		}
		g.GFX = devices.GFXName(g.GFXTargetVersion)
		g.PCI = locationBDF(props["domain"], props["location_id"])
		if props["device_id"] > 0 {
			g.DeviceID = fmt.Sprintf("0x%04x", props["device_id"])
		}
		fillDevice(root, &g)
		if g.Name == "" {
			g.Name = displayName(g)
		}
		gpus = append(gpus, g)
	}
	return gpus, nil
}

// fillDevice adds the DRM/PCI sysfs data of the GPU's render node.
func fillDevice(root string, g *GPU) {
	if g.RenderMinor <= 0 {
		return
	}
	link := filepath.Join(root, "sys/class/drm", fmt.Sprintf("renderD%d", g.RenderMinor), "device")
	dev, err := filepath.EvalSymlinks(link)
	if err != nil {
		return
	}
	g.SysfsPath = relToRoot(root, dev)
	if bdf := filepath.Base(dev); IsBDF(bdf) {
		g.PCI = bdf
	}
	if id := readString(filepath.Join(dev, "device")); id != "" {
		g.DeviceID = strings.ToLower(id)
	}
	g.VRAMBytes = readUint(filepath.Join(dev, "mem_info_vram_total"))
	g.Link.Gen = LinkGen(readString(filepath.Join(dev, "current_link_speed")))
	g.Link.Width = int(readInt(filepath.Join(dev, "current_link_width"), 0))
	g.NUMANode = int(readInt(filepath.Join(dev, "numa_node"), -1))
	if hw := HwmonDir(dev); hw != "" {
		g.PowerCapW = microWatts(readUint(filepath.Join(hw, "power1_cap")))
	}
	g.Name = readString(filepath.Join(dev, "product_name"))
	if d, ok := devices.Lookup(g.DeviceID); ok {
		g.TempLimitsC = d.TempLimitsC
	}
}

// Telemetry reads one sample per GPU. Missing files read as zero.
func Telemetry(root string, gpus []GPU) []Sample {
	root = canonRoot(root)
	out := make([]Sample, 0, len(gpus))
	for _, g := range gpus {
		s := Sample{HIPIndex: g.HIPIndex, VRAMTotal: g.VRAMBytes}
		if g.SysfsPath != "" {
			dev := filepath.Join(root, g.SysfsPath)
			s.BusyPct = float64(readInt(filepath.Join(dev, "gpu_busy_percent"), 0))
			s.VRAMUsed = readUint(filepath.Join(dev, "mem_info_vram_used"))
			if t := readUint(filepath.Join(dev, "mem_info_vram_total")); t > 0 {
				s.VRAMTotal = t
			}
			if hw := HwmonDir(dev); hw != "" {
				s.TempC = edgeTemp(hw)
				s.TempJunctionC = tempByLabel(hw, "junction")
				s.TempMemC = tempByLabel(hw, "mem")
				p := readUint(filepath.Join(hw, "power1_average"))
				if p == 0 {
					p = readUint(filepath.Join(hw, "power1_input"))
				}
				s.PowerW = microWatts(p)
				s.PowerCapW = microWatts(readUint(filepath.Join(hw, "power1_cap")))
			}
			s.SclkMHz = dpmClock(filepath.Join(dev, "pp_dpm_sclk"))
			s.MclkMHz = dpmClock(filepath.Join(dev, "pp_dpm_mclk"))
		}
		out = append(out, s)
	}
	return out
}

// edgeTemp returns the temperature of the sensor labelled "edge", else the
// first sensor of the hwmon node.
func edgeTemp(hw string) float64 {
	if t := tempByLabel(hw, "edge"); t != nil {
		return *t
	}
	return float64(readUint(filepath.Join(hw, "temp1_input"))) / 1000
}

// HwmonDir returns the first hwmon directory of a PCI device dir, or "".
func HwmonDir(dev string) string {
	m, _ := filepath.Glob(filepath.Join(dev, "hwmon", "hwmon*"))
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// LinkGen maps a sysfs link speed ("16.0 GT/s PCIe") to a PCIe generation.
func LinkGen(speed string) int {
	f := strings.Fields(speed)
	if len(f) == 0 {
		return 0
	}
	gts, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	switch {
	case gts >= 64:
		return 6
	case gts >= 32:
		return 5
	case gts >= 16:
		return 4
	case gts >= 8:
		return 3
	case gts >= 5:
		return 2
	case gts >= 2.5:
		return 1
	}
	return 0
}

// IsBDF reports whether s looks like a PCI address "dddd:bb:dd.f".
func IsBDF(s string) bool {
	if len(s) != 12 || s[4] != ':' || s[7] != ':' || s[10] != '.' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 || i == 10 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// locationBDF builds a PCI address from KFD domain and location_id
// (bus<<8 | device<<3 | function).
func locationBDF(domain, loc int64) string {
	return fmt.Sprintf("%04x:%02x:%02x.%x", domain, (loc>>8)&0xff, (loc>>3)&0x1f, loc&7)
}

// displayName names a GPU the DRM product name did not: the device table,
// else the gfx target.
func displayName(g GPU) string {
	if d, ok := devices.Lookup(g.DeviceID); ok {
		return d.Name
	}
	switch {
	case g.GFX != "":
		return "AMD GPU " + g.GFX
	case g.DeviceID != "":
		return "AMD GPU " + g.DeviceID
	}
	return "AMD GPU"
}

// ReadProperties parses a KFD "key value" properties file. Values that do
// not fit int64 are skipped.
func ReadProperties(path string) (map[string]int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	props := map[string]int64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if v, err := strconv.ParseInt(f[1], 10, 64); err == nil {
			props[f[0]] = v
		}
	}
	return props, nil
}

// canonRoot resolves symlinks in root so that resolved sysfs paths can be
// made relative to it again.
func canonRoot(root string) string {
	if root == "" {
		root = "/"
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		return r
	}
	return root
}

// relToRoot turns an absolute resolved path into "/"-rooted host path.
func relToRoot(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return "/" + filepath.ToSlash(rel)
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUint(path string) uint64 {
	v, err := strconv.ParseUint(readString(path), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func readInt(path string, def int64) int64 {
	v, err := strconv.ParseInt(readString(path), 10, 64)
	if err != nil {
		return def
	}
	return v
}

func microWatts(v uint64) float64 { return float64(v) / 1e6 }
