package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/gpu/devices"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/support/execx"
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
	out = append(out, eccChecks(in)...)
	out = append(out, supportChecks(in)...)
	var igpu, dgpu []string
	for _, g := range in.gpus {
		if a, _ := devices.LookupArch(g.GFX); a.Integrated {
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

// docRAS is the kernel documentation of the amdgpu RAS counters.
const docRAS = "https://docs.kernel.org/gpu/amdgpu/ras.html"

// eccChecks reports the ECC state of every GPU (gpu.ecc.<hip index>). A card
// with uncorrectable memory errors fails: they corrupt data silently.
func eccChecks(in inputs) []Check {
	const cat = "gpu"
	var out []Check
	for i, g := range in.gpus {
		id := fmt.Sprintf("ecc.%d", g.HIPIndex)
		title := fmt.Sprintf("GPU %d ECC", g.HIPIndex)
		if i >= len(in.ecc) {
			out = append(out, check(cat, id, Skip, title, "the ECC state was not read").with("", docRAS))
			continue
		}
		out = append(out, eccCheck(in, g, in.ecc[i]))
	}
	return out
}

// eccCheck turns the ECC state of one card into a check.
func eccCheck(in inputs, g gpu.GPU, e gpu.ECC) Check {
	const cat = "gpu"
	id := fmt.Sprintf("ecc.%d", g.HIPIndex)
	title := fmt.Sprintf("GPU %d ECC", g.HIPIndex)
	src := ""
	if e.Source != "" && e.Source != "sysfs" {
		src = ", " + e.Source
	}
	switch e.State {
	case gpu.ECCEnabled:
		switch {
		case e.UE > 0:
			return check(cat, id, Fail, title,
				fmt.Sprintf("ECC is on, uncorrectable memory errors (ue=%d ce=%d de=%d%s): they corrupt data silently", e.UE, e.CE, e.DE, src)).
				with("Reset the card and replace it when the count comes back: stop the workload, then `sudo rmmod amdgpu && sudo modprobe amdgpu` (root). A card that counts uncorrectable errors again after the reset is faulty: replace it (RMA).", docRAS)
		case e.CE > 0 || e.DE > 0:
			return check(cat, id, Warn, title,
				fmt.Sprintf("ECC is on, corrected or degraded memory errors (ue=%d ce=%d de=%d%s)", e.UE, e.CE, e.DE, src)).
				with("Watch the counters: they must stop growing. A card that keeps correcting errors goes bad: replace it.", docRAS)
		default:
			return check(cat, id, OK, title,
				fmt.Sprintf("ECC is on, no memory errors (ue=%d ce=%d de=%d%s)", e.UE, e.CE, e.DE, src)).with("", docRAS)
		}
	case gpu.ECCDisabled:
		return check(cat, id, Warn, title,
			"ECC is off on a card that has it: uncorrectable memory errors then stay silent and can corrupt inference. ECC reserves VRAM for the error correction and costs some memory bandwidth.").
			with(host.KernelArgCommand(in.host.PkgManager, "amdgpu.ras_enable=1")+"  # then reboot. bernie does not change the ECC state.", docRAS)
	case gpu.ECCSupported:
		return check(cat, id, Skip, title,
			"this card has ECC but the kernel exposes no RAS counters: the ECC state is unknown").with("", docRAS)
	default:
		return check(cat, id, Skip, title,
			"no ECC counters: this card does not count memory errors").with("", docRAS)
	}
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
		a, known := devices.LookupArch(gfx)
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

// deviceGroups are the groups that grant access to /dev/kfd and /dev/dri.
var deviceGroups = []string{"video", "render"}

// getentTimeout bounds the getent fallback (LDAP/SSSD lookups can hang).
const getentTimeout = 3 * time.Second

// resolveGroups returns the gid of each named group. The binary is static
// (CGO_ENABLED=0), so os/user reads only /etc/group and cannot see groups that
// come from LDAP or SSSD: look in <root>/etc/group first and, on the live host
// only, fall back to `getent group <name>`. Groups found by neither are absent
// from the result.
func resolveGroups(ctx context.Context, root string, names []string) map[string]int {
	data, _ := os.ReadFile(filepath.Join(root, "etc/group"))
	out := map[string]int{}
	for _, name := range names {
		gid, ok := groupGID(string(data), name)
		if !ok && root == "/" {
			if res, err := execx.Run(ctx, getentTimeout, "getent", "group", name); err == nil && res.OK() {
				gid, ok = groupGID(execx.FirstLine(res.Stdout), name)
			}
		}
		if ok {
			out[name] = gid
		}
	}
	return out
}

// groupGID finds name in group(5) formatted text (name:passwd:gid:members).
func groupGID(text, name string) (int, bool) {
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) < 3 || parts[0] != name {
			continue
		}
		gid, err := strconv.Atoi(parts[2])
		if err == nil {
			return gid, true
		}
	}
	return 0, false
}

func hasGID(gids []int, gid int) bool {
	for _, g := range gids {
		if g == gid {
			return true
		}
	}
	return false
}

// p2pChecks turns the P2P readiness checklist into doctor checks. The
// checklist is the same one `bernie topo --check` prints.
func p2pChecks(in inputs) []Check {
	const cat = "topo.p2p"
	items, err := topo.P2PChecklist(in.root, in.gpus, topo.P2POptions{
		Cmdline: in.host.Cmdline, PkgManager: in.host.PkgManager, Recipe: in.recipe,
	})
	if err != nil {
		return []Check{check(cat, "error", Skip, "P2P readiness",
			"the PCIe topology could not be read: "+err.Error()).with("", topo.P2PDoc)}
	}
	out := make([]Check, 0, len(items))
	for _, it := range items {
		out = append(out, check(cat, it.ID, Status(it.Status), it.Title, it.Detail).with(it.Fix, it.Doc))
	}
	return out
}
