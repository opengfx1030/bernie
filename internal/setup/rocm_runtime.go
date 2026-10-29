package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/host"
)

// rocmRuntime builds the rocm.runtime step: the AMD repository of the
// distribution, the packages for the code targets of the host and the loader
// configuration that the packages do not ship.
//
// The commands are printed, never run: every one of them needs root.
func (p planner) rocmRuntime() Step {
	s := Step{
		ID: "rocm.runtime", Title: "Install the ROCm runtime and HIP SDK", Mode: "host", Root: true,
		Done: p.rocmRuntimeReady(),
		Why:  "Host mode builds the vLLM fork with hipcc and the ROCm headers; the runtime libraries come from the venv wheels.",
	}
	repo, ok := host.ROCmRepository(p.h.OS, p.o.ROCm)
	if !ok {
		s.Commands = []string{"# Install ROCm " + p.o.ROCm + ": " + host.ROCmDocs}
		s.Why = "bernie has no AMD repository for this distribution. Install ROCm " + p.o.ROCm + " by hand, following the AMD guide."
		return s
	}
	pkgs := host.ROCmPackages(p.o.ROCm, p.targets)
	name := "AMD ROCm " + p.o.ROCm + " (" + repo.Label + ")"
	if repo.MultiArch {
		name = "AMD ROCm " + p.o.ROCm + " multi-arch (" + repo.Label + ")"
	}
	switch repo.Family {
	case "rhel":
		s.Commands = []string{
			"sudo rpm --import " + repo.KeyURL,
			`printf '[rocm]\nname=` + name + `\nbaseurl=` + repo.Base +
				`\nenabled=1\ngpgcheck=1\npriority=50\ngpgkey=` + repo.KeyURL + `\n' | sudo tee /etc/yum.repos.d/rocm.repo`,
			"sudo dnf install -y " + strings.Join(pkgs, " "),
		}
	case "deb":
		s.Commands = []string{
			"sudo mkdir -p -m 0755 /etc/apt/keyrings && curl -fsSL " + repo.KeyURL + " | gpg --dearmor | sudo tee /etc/apt/keyrings/rocm.gpg >/dev/null",
			`echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] ` + repo.Base + ` ` + repo.Suite + ` ` + repo.Component +
				`" | sudo tee /etc/apt/sources.list.d/rocm.list`,
			"sudo apt update && sudo apt install -y " + strings.Join(pkgs, " "),
		}
	}
	if c := p.ldSoConf(); c != "" {
		s.Commands = append(s.Commands, c)
	}
	s.Why += " The packages install ROCm " + p.o.ROCm + " for " + p.pkgTargets() + "."
	return s
}

// pkgTargets names the code targets the packages are built for.
func (p planner) pkgTargets() string {
	if len(p.targets) == 0 {
		return "every architecture"
	}
	return strings.Join(p.targets, " and ")
}

// rocmRuntimeReady reports whether the host has a ROCm runtime with hipcc.
// The three layouts are recognised: the versioned tree (/opt/rocm-7.2.0),
// the TheRock tree (/opt/rocm/core-7.14) and the plain tree (/opt/rocm).
func (p planner) rocmRuntimeReady() bool {
	if p.r.Tools["hipcc"] != "" {
		return true
	}
	for _, in := range p.r.Installs {
		if isFile(filepath.Join(p.o.Root, in.Root, "bin", "hipcc")) {
			return true
		}
	}
	return false
}

// ldSoConf returns the command that puts the ROCm libraries on the loader
// search path, or "" when the loader already knows them.
//
// The multi-arch packages ship /etc/ld.so.conf.d/10-rocm-<version>.conf.
// The per-release packages do not, so bernie writes the file only when it is
// missing.
func (p planner) ldSoConf() string {
	if p.ldSoConfPresent() {
		return ""
	}
	dir := p.rocmLibDir()
	if dir == "" {
		return ""
	}
	return "echo '" + dir + "' | sudo tee /etc/ld.so.conf.d/10-rocm-" + p.o.ROCm + ".conf && sudo ldconfig"
}

// ldSoConfPresent reports whether a loader configuration already names the
// ROCm libraries.
func (p planner) ldSoConfPresent() bool {
	m, _ := filepath.Glob(filepath.Join(p.o.Root, "etc", "ld.so.conf.d", "*"))
	for _, f := range m {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), "/rocm") {
			return true
		}
	}
	return false
}

// rocmLibDir returns the library directory of the first ROCm tree, as the
// host sees it (without the root of a fixture).
func (p planner) rocmLibDir() string {
	for _, in := range p.r.Installs {
		if isDir(filepath.Join(p.o.Root, in.Root, "lib")) {
			return in.Root + "/lib"
		}
	}
	return ""
}
