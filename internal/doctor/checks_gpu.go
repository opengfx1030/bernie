package doctor

import (
	"fmt"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

func gpuChecks(in inputs) []Check {
	const cat = "gpu"
	d := in.devices
	groupsFix := "sudo usermod -aG video,render $USER  # then log out and in"
	var out []Check
	switch {
	case !d.kfdExists:
		out = append(out, check(cat, "kfd", Fail, "/dev/kfd is readable and writable", "/dev/kfd is missing (amdgpu KFD not loaded)").
			with("sudo modprobe amdgpu", docGeneral))
	case !d.kfdRW:
		out = append(out, check(cat, "kfd", Fail, "/dev/kfd is readable and writable", "permission denied").with(groupsFix, docGeneral))
	default:
		out = append(out, check(cat, "kfd", OK, "/dev/kfd is readable and writable", ""))
	}
	switch {
	case len(d.render) == 0:
		out = append(out, check(cat, "render", Fail, "/dev/dri/renderD* exist", "no render nodes").with("sudo modprobe amdgpu", docGeneral))
	case !d.renderRW:
		out = append(out, check(cat, "render", Fail, "/dev/dri/renderD* exist", fmt.Sprintf("%d render nodes, not all accessible", len(d.render))).with(groupsFix, docGeneral))
	default:
		out = append(out, check(cat, "render", OK, "/dev/dri/renderD* exist", fmt.Sprintf("%d render nodes", len(d.render))))
	}
	u := in.host.User
	if u.Name == "" {
		out = append(out, check(cat, "groups", Skip, "User is in video and render", "no user information for this root"))
	} else {
		// Compare gids: names do not resolve for LDAP/SSSD groups.
		var missing, unknown []string
		for _, g := range deviceGroups {
			if gid, ok := in.groupGIDs[g]; !ok {
				unknown = append(unknown, g)
			} else if !hasGID(u.GIDs, gid) {
				missing = append(missing, g)
			}
		}
		switch {
		case len(missing) == 0 && len(unknown) > 0:
			out = append(out, check(cat, "groups", Warn, "User is in video and render",
				fmt.Sprintf("cannot tell: group %s not found in /etc/group or by getent", strings.Join(unknown, ", "))).with(groupsFix, docGroups))
		case len(missing) == 0:
			out = append(out, check(cat, "groups", OK, "User is in video and render", u.Name))
		case d.kfdRW && d.renderRW:
			out = append(out, check(cat, "groups", Warn, "User is in video and render",
				fmt.Sprintf("%s is not in %s; devices are accessible now (world-writable), containers with --group-add still need the groups", u.Name, strings.Join(missing, ", "))).with(groupsFix, docGroups))
		default:
			out = append(out, check(cat, "groups", Fail, "User is in video and render",
				fmt.Sprintf("%s is not in %s", u.Name, strings.Join(missing, ", "))).with(groupsFix, docGroups))
		}
	}
	if in.gpuErr != nil || len(in.gpus) == 0 {
		detail := "no KFD GPU nodes"
		if in.gpuErr != nil {
			detail = in.gpuErr.Error()
		}
		out = append(out, check(cat, "count", Fail, "At least one GPU", detail).with("sudo modprobe amdgpu", docHardware))
		return out
	}
	out = append(out, check(cat, "count", OK, "At least one GPU", fmt.Sprintf("%d GPUs: %s", len(in.gpus), archSummary(in.gpus))).with("", docHardware))
	out = append(out, supportChecks(in)...)
	var igpu, dgpu []string
	for _, g := range in.gpus {
		if a, _ := gpu.LookupArch(g.GFX); a.Integrated {
			igpu = append(igpu, fmt.Sprint(g.HIPIndex))
		} else {
			dgpu = append(dgpu, fmt.Sprint(g.HIPIndex))
		}
	}
	if len(igpu) > 0 && len(dgpu) > 0 {
		out = append(out, check(cat, "igpu", Warn, "No integrated GPU mixed with discrete GPUs",
			"integrated GPU at HIP index "+strings.Join(igpu, ",")+" shifts HIP indices").
			with("export HIP_VISIBLE_DEVICES="+strings.Join(dgpu, ","), docGotchas))
	}
	return out
}

func archSummary(gpus []gpu.GPU) string {
	n := map[string]int{}
	for _, g := range gpus {
		n[g.GFX]++
	}
	var parts []string
	for _, gfx := range archOrder(gpus) {
		parts = append(parts, fmt.Sprintf("%d× %s", n[gfx], gfx))
	}
	return strings.Join(parts, ", ")
}

// archOrder lists distinct gfx names in HIP order.
func archOrder(gpus []gpu.GPU) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range gpus {
		if !seen[g.GFX] {
			seen[g.GFX] = true
			out = append(out, g.GFX)
		}
	}
	return out
}

// supportChecks reports, per distinct gfx target of the host, whether the
// device tables know it and whether recipes are validated on it
// (gpu.supported.<gfx>), and the HSA override the table prescribes
// (gpu.hsa_override.<gfx>). The device facts come from internal/machine/gpu/devices.
func supportChecks(in inputs) []Check {
	const cat = "gpu"
	var out []Check
	for _, gfx := range archOrder(in.gpus) {
		id, title := "supported."+gfx, gfx+" is supported"
		a, known := gpu.LookupArch(gfx)
		switch {
		case !known:
			out = append(out, check(cat, id, Warn, title, gfx+" is not in the device tables: its ROCm and vLLM support is unchecked").with("", docHardware))
		case gpu.ArchMatch(in.validated, gfx):
			out = append(out, check(cat, id, OK, title, a.Label()+" target; recipes are validated on it"))
		default:
			out = append(out, check(cat, id, Warn, title,
				fmt.Sprintf("%s target; recipes are validated on %s", a.Label(), validatedSentence(in.validated))))
		}
		if known && a.HSAOverride != "" {
			out = append(out, check(cat, "hsa_override."+gfx, Warn, gfx+" needs an HSA override",
				fmt.Sprintf("%s target without its own validated code objects; it runs %s code objects", a.Label(), a.CodeTarget())).
				with(fmt.Sprintf("Set HSA_OVERRIDE_GFX_VERSION=%[1]s in the serve environment (recipe env or --set env.HSA_OVERRIDE_GFX_VERSION=%[1]s).", a.HSAOverride), docHSA))
		}
	}
	return out
}

func validatedSentence(archs []string) string {
	switch len(archs) {
	case 0:
		return "no target yet"
	case 1:
		return archs[0] + " only"
	}
	return strings.Join(archs, ", ") + " only"
}
