package host

import "strings"

// Package features rdnactl can ask the operator to install.
const (
	PkgROCm          = "rocm"           // ROCm runtime + HIP SDK
	PkgHIPCC         = "hipcc"          // HIP compiler
	PkgROCmInfo      = "rocminfo"       // rocminfo
	PkgSMI           = "smi"            // rocm-smi / amd-smi
	PkgROCBLAS       = "rocblas"        // librocblas
	PkgEngine        = "engine"         // a container engine with compose
	PkgDockerCompose = "docker-compose" // compose for docker
	PkgPodmanCompose = "podman-compose" // compose for podman
	PkgPython        = "python"         // Python 3.12 with venv
	PkgGit           = "git"            // git
)

// packages maps feature -> package manager -> package names.
var packages = map[string]map[string][]string{
	PkgROCm: {
		"dnf": {"rocm", "rocm-hip-runtime-devel"}, "apt": {"rocm"},
		"pacman": {"rocm-hip-sdk", "rocminfo"}, "zypper": {"rocm"},
	},
	PkgHIPCC: {
		"dnf": {"hipcc"}, "apt": {"hipcc"}, "pacman": {"rocm-hip-sdk"}, "zypper": {"hipcc"},
	},
	PkgROCmInfo: {
		"dnf": {"rocminfo"}, "apt": {"rocminfo"}, "pacman": {"rocminfo"}, "zypper": {"rocminfo"},
	},
	PkgSMI: {
		"dnf": {"rocm-smi"}, "apt": {"rocm-smi"}, "pacman": {"rocm-smi-lib"}, "zypper": {"rocm-smi"},
	},
	PkgROCBLAS: {
		"dnf": {"rocblas"}, "apt": {"rocblas"}, "pacman": {"rocblas"}, "zypper": {"rocblas"},
	},
	PkgEngine: {
		"dnf": {"podman", "podman-compose"}, "apt": {"docker.io", "docker-compose-v2"},
		"pacman": {"docker", "docker-compose"}, "zypper": {"podman", "podman-compose"},
	},
	PkgDockerCompose: {
		"dnf": {"docker-compose"}, "apt": {"docker-compose-v2"}, "pacman": {"docker-compose"}, "zypper": {"docker-compose"},
	},
	PkgPodmanCompose: {
		"dnf": {"podman-compose"}, "apt": {"podman-compose"}, "pacman": {"podman-compose"}, "zypper": {"podman-compose"},
	},
	PkgPython: {
		"dnf": {"python3.12"}, "apt": {"python3.12", "python3.12-venv"}, "pacman": {"python"}, "zypper": {"python312"},
	},
	PkgGit: {
		"dnf": {"git"}, "apt": {"git"}, "pacman": {"git"}, "zypper": {"git"},
	},
}

var installVerb = map[string]string{
	"dnf":    "sudo dnf install -y",
	"apt":    "sudo apt install -y",
	"pacman": "sudo pacman -S --needed",
	"zypper": "sudo zypper install -y",
}

// InstallCommand returns the install command for a feature with the
// package manager pm, or "" when pm or the feature is unknown.
func InstallCommand(pm, feature string) string {
	verb, ok := installVerb[pm]
	pkgs := packages[feature][pm]
	if !ok || len(pkgs) == 0 {
		return ""
	}
	return verb + " " + strings.Join(pkgs, " ")
}

// KernelArgCommand returns the command (or one instruction) that adds a
// kernel command-line argument persistently.
func KernelArgCommand(pm, arg string) string {
	switch pm {
	case "dnf":
		return `sudo grubby --update-kernel=ALL --args="` + arg + `"`
	case "apt":
		return "Add " + arg + " to GRUB_CMDLINE_LINUX_DEFAULT in /etc/default/grub, run sudo update-grub, reboot."
	case "pacman":
		return "Add " + arg + " to GRUB_CMDLINE_LINUX_DEFAULT in /etc/default/grub, run sudo grub-mkconfig -o /boot/grub/grub.cfg, reboot."
	case "zypper":
		return "Add " + arg + " to GRUB_CMDLINE_LINUX_DEFAULT in /etc/default/grub, run sudo grub2-mkconfig -o /boot/grub2/grub.cfg, reboot."
	}
	return "Add " + arg + " to the kernel command line and reboot."
}
