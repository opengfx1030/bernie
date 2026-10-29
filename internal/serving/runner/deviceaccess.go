package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// resolveDeviceAccess decides how the container gets the GPU of the host
// and fills the plan: the mode, the CDI device names, the HIP indices
// inside the container and the reason. all is every GPU of the host: the
// index of a CDI device is the rank of a GPU in the PCI-address order of
// the host, so the mapping needs the whole host, not the selected GPUs.
func resolveDeviceAccess(ctx context.Context, p *recipe.Plan, eng host.Engine, all []gpu.GPU) {
	root := hostroot.Root()
	cdi := host.ProbeCDI(ctx, root, eng.Name)
	stale := host.SpecStale(root, cdi.Spec, len(all))
	_, why := host.EngineCDISupport(eng, root)
	da := host.ResolveDeviceAccess(p.DeviceAccessRequest, eng, cdi, stale, why)
	p.DeviceAccess, p.DeviceAccessReason = da.Mode, da.Reason
	if da.Mode != host.DeviceAccessCDI {
		return
	}
	names, err := cdiDeviceNames(all, p.GPUs)
	if err != nil {
		p.Warnings = append(p.Warnings, err.Error())
		p.DeviceAccess = host.DeviceAccessClassic
		p.DeviceAccessReason = "the GPUs of this host cannot be mapped to CDI devices: " + err.Error()
		return
	}
	p.CDIDevices = names
	// A container that only sees the injected GPUs numbers them from 0.
	// The renumbering is not confirmed on a live host yet, so the plan
	// keeps both index sets and warns when they differ.
	inside := containerHIPIndices(len(p.GPUs))
	p.ContainerHIPDevices = joinInts(inside)
	if p.ContainerHIPDevices != p.HIPDevices {
		p.Env["HIP_VISIBLE_DEVICES"] = p.ContainerHIPDevices
		p.Warnings = append(p.Warnings, fmt.Sprintf(
			"the HIP indices inside the container are renumbered (%s -> %s): the renumbering of a CDI container is not confirmed on a live host yet",
			p.HIPDevices, p.ContainerHIPDevices))
	}
}

// cdiDeviceNames returns the CDI device names of the selected GPUs. The
// index of a CDI device is the rank of the GPU in the PCI-address order of
// the host, not the HIP order: the toolkit walks the PCI devices of the
// host in path order. When every GPU of the host is selected, the single
// device "all" is used.
func cdiDeviceNames(all []gpu.GPU, selected []int) ([]string, error) {
	if len(selected) == len(all) && len(all) > 0 {
		return []string{host.CDIKind + "=" + host.CDIAllDevice}, nil
	}
	byHIP := map[int]gpu.GPU{}
	for _, g := range all {
		byHIP[g.HIPIndex] = g
	}
	out := make([]string, 0, len(selected))
	for _, hip := range selected {
		g, ok := byHIP[hip]
		if !ok {
			return nil, fmt.Errorf("GPU %d is not a GPU of this host: it cannot be mapped to a CDI device", hip)
		}
		if g.PCI == "" {
			return nil, fmt.Errorf("GPU %d has no PCI address: it cannot be mapped to a CDI device", hip)
		}
		out = append(out, fmt.Sprintf("%s=%d", host.CDIKind, pciRank(all, g.PCI)))
	}
	return out, nil
}

// pciRank is the rank of a PCI address in the PCI-address order of the
// GPUs of the host.
func pciRank(all []gpu.GPU, pci string) int {
	order := make([]string, 0, len(all))
	for _, g := range all {
		if g.PCI != "" {
			order = append(order, g.PCI)
		}
	}
	sort.Strings(order)
	for i, p := range order {
		if p == pci {
			return i
		}
	}
	return 0
}

// containerHIPIndices are the HIP indices the selected GPUs have inside a
// container that only sees them: their ranks, from 0.
func containerHIPIndices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// joinInts is a list of indices as a comma separated string.
func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ",")
}

// deviceArgs returns the --device flags of a container run: the CDI
// device names in CDI mode, the device nodes of the host in classic mode.
// A run never mixes the two: the classic form passes every render node of
// the host, which would defeat the isolation of CDI.
func deviceArgs(p *recipe.Plan) []string {
	if p.DeviceAccess == host.DeviceAccessCDI && len(p.CDIDevices) > 0 {
		out := make([]string, 0, 2*len(p.CDIDevices))
		for _, d := range p.CDIDevices {
			out = append(out, "--device", d)
		}
		return out
	}
	return []string{"--device", "/dev/kfd", "--device", "/dev/dri"}
}

// deviceList returns the devices of a Compose file: the CDI device names
// in CDI mode, the device nodes of the host in classic mode.
func deviceList(p *recipe.Plan) []string {
	if p.DeviceAccess == host.DeviceAccessCDI && len(p.CDIDevices) > 0 {
		return p.CDIDevices
	}
	return []string{"/dev/kfd", "/dev/dri"}
}
