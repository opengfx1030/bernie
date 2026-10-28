package setup

import (
	"strings"
	"time"

	"github.com/opengfx1030/rdnactl/internal/machine/host"
	"github.com/opengfx1030/rdnactl/internal/support/execx"
)

func (p planner) groups() Step {
	return Step{
		ID: "groups", Title: "Add the user to the video and render groups", Mode: "both", Root: true,
		Done:     p.h.User.InGroup("video") && p.h.User.InGroup("render"),
		Commands: []string{"sudo usermod -aG video,render " + userName(p.h)},
		Why:      "/dev/kfd and /dev/dri/renderD* belong to these groups. Log out and in afterwards.",
	}
}

func userName(h host.Info) string {
	if h.User.Name != "" {
		return h.User.Name
	}
	return "$USER"
}

func (p planner) rocmRuntime() Step {
	s := Step{
		ID: "rocm.runtime", Title: "Install the ROCm runtime and HIP SDK", Mode: "host", Root: true,
		Done: len(p.r.Installs) > 0 && p.r.Tools["hipcc"] != "",
		Why:  "Host mode builds the vLLM fork with hipcc and the ROCm headers; the runtime libraries come from the venv wheels.",
	}
	switch p.h.PkgManager {
	case "dnf":
		s.Commands = []string{
			"sudo rpm --import https://repo.radeon.com/rocm/rocm.gpg.key",
			`printf '[rocm]\nname=ROCm 7.2\nbaseurl=https://repo.radeon.com/rocm/el10/7.2/main\nenabled=1\ngpgcheck=1\npriority=50\ngpgkey=https://repo.radeon.com/rocm/rocm.gpg.key\n' | sudo tee /etc/yum.repos.d/rocm.repo`,
			host.InstallCommand("dnf", host.PkgROCm),
		}
	case "apt":
		s.Commands = []string{
			"sudo mkdir -p -m 0755 /etc/apt/keyrings && wget -qO- https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg >/dev/null",
			`echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/7.2 $(. /etc/os-release && echo $VERSION_CODENAME) main" | sudo tee /etc/apt/sources.list.d/rocm.list`,
			"sudo apt update && " + host.InstallCommand("apt", host.PkgROCm),
		}
	case "pacman", "zypper":
		s.Commands = []string{host.InstallCommand(p.h.PkgManager, host.PkgROCm)}
	default:
		s.Commands = []string{"# Install ROCm: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"}
	}
	return s
}

func (p planner) containerEngine() Step {
	e, ok := host.Preferred(p.h.Engines, "")
	s := Step{
		ID: "container.engine", Title: "Install Docker or Podman with compose", Mode: "container", Root: true,
		Done: ok && e.Usable && e.Compose != "",
		Why:  "Container mode runs the vLLM image with Docker or Podman.",
	}
	switch {
	case ok && e.Name == "docker" && !e.Usable:
		s.Commands = []string{"sudo systemctl enable --now docker", "sudo usermod -aG docker " + userName(p.h)}
	case ok && e.Name == "podman" && !e.Usable:
		s.Commands = []string{"systemctl --user enable --now podman.socket"}
	case ok && e.Compose == "":
		feat := host.PkgDockerCompose
		if e.Name == "podman" {
			feat = host.PkgPodmanCompose
		}
		s.Commands = []string{orSentence(host.InstallCommand(p.h.PkgManager, feat), "# Install compose for "+e.Name)}
	default:
		cmd := orSentence(host.InstallCommand(p.h.PkgManager, host.PkgEngine), "# Install Docker or Podman: https://docs.docker.com/engine/install/")
		s.Commands = []string{cmd}
		if strings.Contains(cmd, "docker") && !strings.HasPrefix(cmd, "#") {
			s.Commands = append(s.Commands, "sudo systemctl enable --now docker", "sudo usermod -aG docker "+userName(p.h))
		}
	}
	return s
}

func orSentence(cmd, sentence string) string {
	if cmd != "" {
		return cmd
	}
	return sentence
}

func (p planner) imagePull() Step {
	eng := "docker"
	if e, ok := host.Preferred(p.h.Engines, ""); ok {
		eng = e.Name
	}
	done := false
	if which(eng) != "" {
		res, err := execx.Run(p.ctx, 15*time.Second, eng, "image", "inspect", "--format", "{{.Id}}", p.o.Image)
		done = err == nil && res.OK()
	}
	return Step{
		ID: "image.pull", Title: "Pull the vLLM image", Mode: "container",
		Done:     done,
		Commands: []string{eng + " pull " + p.o.Image},
		Why:      "Container mode runs " + p.o.Image + ". Tags are refreshed in place: pull again to update.",
	}
}
