package doctor

import (
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
		return []Check{check(cat, "engine", Skip, "Docker or Podman is installed", "no container engine: container mode is unavailable, host mode works").
			with(install(pm, host.PkgEngine, "Install Docker or Podman."), docRecipes)}
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
	return out
}
