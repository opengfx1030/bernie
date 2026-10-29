package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The AMD Container Toolkit (amd-ctk) hands the GPU to a container through
// a CDI spec: a file that names the device nodes of every GPU. The engine
// reads the spec and injects the nodes into the container.
// `sudo amd-ctk cdi generate` writes the spec; `amd-ctk cdi list` does not
// read it, it only enumerates the hardware.
const (
	// CDIKind is the CDI kind of the AMD GPU devices.
	CDIKind = "amd.com/gpu"
	// CDIAllDevice is the name of the device that stands for every GPU.
	CDIAllDevice = "all"
	// dockerCDIMin is the first Docker version that knows CDI.
	dockerCDIMin = "25.0.0"
	// dockerCDIDefault is the Docker version from which CDI is on by
	// default.
	dockerCDIDefault = "28.3.0"
	// podmanCDIMin is the first Podman version that knows CDI.
	podmanCDIMin = "4.0.0"
)

// cdiSpecDirs are the directories Docker and Podman read CDI specs from.
var cdiSpecDirs = []string{"/etc/cdi", "/var/run/cdi"}

// Device access modes of a container run.
const (
	// DeviceAccessClassic passes the device nodes of the host into the
	// container (/dev/kfd and /dev/dri).
	DeviceAccessClassic = "classic"
	// DeviceAccessCDI injects the GPUs of the host through the CDI spec.
	DeviceAccessCDI = "cdi"
	// DeviceAccessAuto uses CDI when the host can, else the classic form.
	DeviceAccessAuto = "auto"
)

// DeviceAccessModes are the modes a recipe, a flag or the config may ask
// for.
var DeviceAccessModes = []string{DeviceAccessClassic, DeviceAccessCDI, DeviceAccessAuto}

// CDIDevice is one device of a CDI spec.
type CDIDevice struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

// CDISpec is a parsed CDI spec of kind amd.com/gpu.
type CDISpec struct {
	Path    string      `json:"path"`
	Kind    string      `json:"kind"`
	Version string      `json:"version,omitempty"`
	Devices []CDIDevice `json:"devices"`
}

// CDI holds the container device interface facts of the host.
type CDI struct {
	// Toolkit is true when amd-ctk is installed.
	Toolkit bool `json:"toolkit"`
	// ToolkitPath and ToolkitVersion say which amd-ctk was found.
	ToolkitPath    string `json:"toolkit_path,omitempty"`
	ToolkitVersion string `json:"toolkit_version,omitempty"`
	// SpecDirs are the directories that were scanned for a spec.
	SpecDirs []string `json:"spec_dirs,omitempty"`
	// Spec is the amd.com/gpu spec of the host (nil when there is none).
	Spec *CDISpec `json:"spec,omitempty"`
}

// DeviceAccess is the resolved device access mode of a container run.
type DeviceAccess struct {
	// Mode is classic or cdi.
	Mode string `json:"mode"`
	// Reason says why that mode was chosen, in one sentence.
	Reason string `json:"reason"`
}

// cdiDoc is the part of a CDI spec bernie reads.
type cdiDoc struct {
	Kind    string      `json:"kind" yaml:"kind"`
	Version string      `json:"cdiVersion" yaml:"cdiVersion"`
	Devices []cdiDevDoc `json:"devices" yaml:"devices"`
}

type cdiDevDoc struct {
	Name  string   `json:"name" yaml:"name"`
	Edits cdiEdits `json:"containerEdits" yaml:"containerEdits"`
}

type cdiEdits struct {
	Nodes []cdiNode `json:"deviceNodes" yaml:"deviceNodes"`
}

type cdiNode struct {
	Path string `json:"path" yaml:"path"`
}

// ProbeCDI finds amd-ctk and the amd.com/gpu CDI spec of the host. The
// engine names the CDI spec directories of the daemon ("" = the default
// ones). The probe reads the host only.
func ProbeCDI(ctx context.Context, root, engine string) CDI {
	return probeCDI(ctx, root, engine, defaultRunner)
}

func probeCDI(ctx context.Context, root, engine string, r runner) CDI {
	var c CDI
	if p := r.Which("amd-ctk"); p != "" {
		c.Toolkit = true
		c.ToolkitPath = p
		if v := r.Run(ctx, "amd-ctk", "version"); v.OK() {
			c.ToolkitVersion = toolkitVersion(v.Stdout)
		}
	}
	c.SpecDirs = cdiSpecDirsOf(ctx, r, engine)
	c.Spec = findCDISpec(root, c.SpecDirs)
	return c
}

// toolkitVersion is the version line of `amd-ctk version`
// ("Version: v1.3.0").
func toolkitVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// cdiSpecDirsOf returns the CDI spec directories of the engine: the ones
// the daemon reports, else the default ones.
func cdiSpecDirsOf(ctx context.Context, r runner, engine string) []string {
	if engine == "docker" {
		if v := r.Run(ctx, "docker", "info", "--format", "{{json .CDISpecDirs}}"); v.OK() {
			var dirs []string
			if err := json.Unmarshal([]byte(strings.TrimSpace(v.Stdout)), &dirs); err == nil && len(dirs) > 0 {
				return dirs
			}
		}
	}
	return cdiSpecDirs
}

// findCDISpec returns the first amd.com/gpu spec of the directories. The
// paths of the result are the paths of the host, not of the root.
func findCDISpec(root string, dirs []string) *CDISpec {
	for _, dir := range dirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext != ".json" && ext != ".yaml" && ext != ".yml" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, dir, e.Name()))
			if err != nil {
				continue
			}
			spec, err := parseCDISpec(filepath.Join(dir, e.Name()), b, ext != ".json")
			if err != nil || spec.Kind != CDIKind {
				continue
			}
			return spec
		}
	}
	return nil
}

// parseCDISpec parses a CDI spec file (JSON or YAML).
func parseCDISpec(path string, b []byte, yamlFormat bool) (*CDISpec, error) {
	var d cdiDoc
	if yamlFormat {
		if err := yaml.Unmarshal(b, &d); err != nil {
			return nil, err
		}
	} else if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	spec := &CDISpec{Path: path, Kind: d.Kind, Version: d.Version}
	for _, dev := range d.Devices {
		cd := CDIDevice{Name: dev.Name}
		for _, n := range dev.Edits.Nodes {
			if n.Path != "" {
				cd.Nodes = append(cd.Nodes, n.Path)
			}
		}
		spec.Devices = append(spec.Devices, cd)
	}
	return spec, nil
}

// GPUCount is the number of GPUs the spec names (the "all" device does not
// count).
func (s *CDISpec) GPUCount() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, d := range s.Devices {
		if d.Name != CDIAllDevice {
			n++
		}
	}
	return n
}

// NodePaths is every device node path of the spec.
func (s *CDISpec) NodePaths() []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, d := range s.Devices {
		out = append(out, d.Nodes...)
	}
	return out
}

// SpecStale says why the spec does not match this host, or "" when it
// does. gpuCount is the number of GPUs of the host. A spec is stale when it
// names another number of GPUs, or when it names a device node that this
// host does not have.
func SpecStale(root string, s *CDISpec, gpuCount int) string {
	if s == nil {
		return ""
	}
	if n := s.GPUCount(); n != gpuCount {
		return fmt.Sprintf("the spec names %d GPUs, this host has %d", n, gpuCount)
	}
	for _, p := range s.NodePaths() {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			return fmt.Sprintf("the spec names %s, which does not exist on this host", p)
		}
	}
	return ""
}

// dockerDaemon is the part of /etc/docker/daemon.json bernie reads.
type dockerDaemon struct {
	Features struct {
		CDI *bool `json:"cdi"`
	} `json:"features"`
	SpecDirs []string `json:"cdi-spec-dirs"`
}

// readDockerDaemon reads the daemon.json of the host ("" when there is
// none).
func readDockerDaemon(root string) dockerDaemon {
	var d dockerDaemon
	b, err := os.ReadFile(filepath.Join(root, "etc/docker/daemon.json"))
	if err != nil {
		return d
	}
	_ = json.Unmarshal(b, &d)
	return d
}

// EngineCDISupport says whether an engine can use a CDI spec, and why not.
// Docker knows CDI from 25.0 on and reads it by default from 28.3.0 on;
// before that the daemon must enable it. Podman knows CDI from 4.0 on.
func EngineCDISupport(e Engine, root string) (bool, string) {
	switch e.Name {
	case "docker":
		if !versionAtLeast(e.Version, dockerCDIMin) {
			return false, fmt.Sprintf("docker %s does not support CDI (%s or later is needed)", e.Version, dockerCDIMin)
		}
		d := readDockerDaemon(root)
		if d.Features.CDI != nil && !*d.Features.CDI {
			return false, "/etc/docker/daemon.json disables CDI (features.cdi is false)"
		}
		if !versionAtLeast(e.Version, dockerCDIDefault) && (d.Features.CDI == nil || !*d.Features.CDI) {
			return false, fmt.Sprintf("docker %s needs CDI enabled in /etc/docker/daemon.json (features.cdi true), or %s or later", e.Version, dockerCDIDefault)
		}
		return true, ""
	case "podman":
		if !versionAtLeast(e.Version, podmanCDIMin) {
			return false, fmt.Sprintf("podman %s does not support CDI (%s or later is needed)", e.Version, podmanCDIMin)
		}
		return true, ""
	}
	return false, e.Name + " does not support CDI"
}

// versionAtLeast reports whether the version v is at least min. An unknown
// version is not.
func versionAtLeast(v, min string) bool {
	a, b := versionParts(v), versionParts(min)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for i := 0; i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		y = b[i]
		if x != y {
			return x > y
		}
	}
	return true
}

// versionParts are the numbers of a version string ("29.8.1" -> 29, 8, 1).
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil
	}
	var out []int
	for _, f := range strings.Split(v, ".") {
		n, err := strconv.Atoi(f)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// ResolveDeviceAccess decides how a container gets the GPU of the host.
// The request is classic, cdi or auto. cdi needs a valid amd.com/gpu spec
// (stale is the reason the spec does not fit this host) and an engine that
// can use it (why is the reason the engine cannot). auto uses CDI when the
// host can and the classic form otherwise.
func ResolveDeviceAccess(req string, e Engine, c CDI, stale, why string) DeviceAccess {
	if req == "" {
		req = DeviceAccessAuto
	}
	usable := c.Spec != nil && stale == "" && why == ""
	switch req {
	case DeviceAccessClassic:
		return DeviceAccess{Mode: DeviceAccessClassic, Reason: "the classic device pass-through was asked for"}
	case DeviceAccessCDI:
		if usable {
			return DeviceAccess{Mode: DeviceAccessCDI, Reason: "CDI was asked for and " + c.Spec.Path + " fits this host"}
		}
	default:
		if usable {
			return DeviceAccess{Mode: DeviceAccessCDI, Reason: c.Spec.Path + " fits this host and " + e.Name + " can use it"}
		}
	}
	return DeviceAccess{Mode: DeviceAccessClassic, Reason: cdiReason(c, stale, why)}
}

// cdiReason says why the host cannot use CDI, in one sentence.
func cdiReason(c CDI, stale, why string) string {
	switch {
	case c.Spec == nil && !c.Toolkit:
		return "amd-ctk is not installed, so the classic device pass-through is used"
	case c.Spec == nil:
		return "amd-ctk is installed but there is no " + CDIKind + " spec on this host"
	case stale != "":
		return "the " + CDIKind + " spec does not fit this host: " + stale
	case why != "":
		return why
	}
	return "the " + CDIKind + " spec is used"
}
