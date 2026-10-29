package doctor

import (
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/host"
)

func containerChecks(in inputs) []Check {
	const cat = "container"
	pm := in.host.PkgManager
	if in.root != "/" {
		return []Check{check(cat, "engine", Skip, "Docker or Podman is installed", "not probed for a fixture root")}
	}
	e, ok := host.Preferred(in.host.Engines, "")
	if !ok {
		return append([]Check{check(cat, "engine", Skip, "Docker or Podman is installed", "no container engine: container mode is unavailable, host mode works").
			with(install(pm, host.PkgEngine, "Install Docker or Podman."), docRecipes)}, cdiChecks(in)...)
	}
	var out []Check
	out = append(out, check(cat, "engine", OK, "Docker or Podman is installed", strings.TrimSpace(e.Name+" "+e.Version)+" ("+e.Path+")"))
	if e.Usable {
		out = append(out, check(cat, "daemon", OK, e.Name+" info works for this user", ""))
	} else {
		fix := "sudo systemctl enable --now docker && sudo usermod -aG docker $USER"
		if e.Name == "podman" {
			fix = "systemctl --user enable --now podman.socket"
		}
		out = append(out, check(cat, "daemon", Warn, e.Name+" info works for this user", "`"+e.Name+" info` failed: container mode is unavailable").with(fix, docRecipes))
	}
	if e.Compose != "" {
		out = append(out, check(cat, "compose", OK, "Compose is installed", e.Compose))
	} else {
		feat := host.PkgDockerCompose
		if e.Name == "podman" {
			feat = host.PkgPodmanCompose
		}
		out = append(out, check(cat, "compose", Warn, "Compose is installed", "bernie compose files need docker compose, podman compose or podman-compose").with(install(pm, feat, "Install compose."), docCompose))
	}
	mode := "rootful"
	if e.Rootless {
		mode = "rootless (bernie uses --group-add keep-groups)"
	}
	out = append(out, check(cat, "rootless", OK, "Rootless state", mode))
	switch in.host.SELinux {
	case "":
		out = append(out, check(cat, "selinux", Skip, "SELinux mode", "no SELinux"))
	case "enforcing":
		out = append(out, check(cat, "selinux", OK, "SELinux mode", "enforcing: bernie runs containers with --security-opt label=disable").with("", docCompose))
	default:
		out = append(out, check(cat, "selinux", OK, "SELinux mode", in.host.SELinux))
	}
	return append(out, cdiChecks(in)...)
}

// cdiChecks are the checks of the container device access: the CDI spec of
// the AMD Container Toolkit, whether it still fits this host, whether the
// engine can read it and the mode a container run would use.
func cdiChecks(in inputs) []Check {
	const cat = "container"
	pm := in.host.PkgManager
	var out []Check
	switch {
	case !in.cdi.Toolkit:
		out = append(out, check(cat, "cdi", Skip, "The AMD Container Toolkit is installed",
			"amd-ctk is not installed: the container gets the GPU the classic way").
			with(install(pm, host.PkgAMDCTK, "Install the AMD Container Toolkit (amd-container-toolkit)."), docInstall))
	case in.cdi.Spec == nil:
		out = append(out, check(cat, "cdi", Warn, "The AMD Container Toolkit is installed", "amd-ctk installed, no spec").
			with("sudo amd-ctk cdi generate --output=/etc/cdi/amd.json", docInstall))
	default:
		out = append(out, check(cat, "cdi", OK, "The AMD Container Toolkit is installed",
			in.cdi.Spec.Path+" ("+in.cdi.ToolkitVersion+", "+strconv.Itoa(in.cdi.Spec.GPUCount())+" GPUs)"))
	}
	switch {
	case in.cdi.Spec == nil:
		out = append(out, check(cat, "cdi.valid", Skip, "The CDI spec fits this host", "no spec to check"))
	case in.cdiStale != "":
		out = append(out, check(cat, "cdi.valid", Warn, "The CDI spec fits this host", in.cdiStale).
			with("sudo amd-ctk cdi generate --output=/etc/cdi/amd.json && sudo amd-ctk cdi validate", docInstall))
	default:
		out = append(out, check(cat, "cdi.valid", OK, "The CDI spec fits this host", "the spec matches the GPUs of this host"))
	}
	eng, ok := host.Preferred(in.host.Engines, "")
	switch {
	case !ok:
		out = append(out, check(cat, "cdi.engine", Skip, "The container engine can use CDI", "no container engine"))
	case in.cdiWhy != "":
		fix := install(pm, host.PkgDocker, "Upgrade the container engine to a version that reads CDI.")
		if strings.Contains(in.cdiWhy, "features.cdi") {
			fix = `remove "cdi": false from the features block of /etc/docker/daemon.json and run sudo systemctl restart docker`
		}
		out = append(out, check(cat, "cdi.engine", Warn, "The container engine can use CDI", in.cdiWhy).with(fix, docInstall))
	default:
		out = append(out, check(cat, "cdi.engine", OK, "The container engine can use CDI", eng.Name+" "+eng.Version+" reads CDI"))
	}
	if !ok {
		out = append(out, check(cat, "device_access", Skip, "The device access mode of a container run", "no container engine"))
		return out
	}
	da := host.ResolveDeviceAccess(host.DeviceAccessAuto, eng, in.cdi, in.cdiStale, in.cdiWhy)
	out = append(out, check(cat, "device_access", OK, "The device access mode of a container run", da.Mode+": "+da.Reason))
	return out
}
