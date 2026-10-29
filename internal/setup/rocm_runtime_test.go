package setup

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// rocmStepOf plans the host steps of a distribution and returns the
// rocm.runtime step.
func rocmStepOf(t *testing.T, osr host.OSRelease, rocmVersion string, archs []string) Step {
	t.Helper()
	h := host.Info{OS: osr, PkgManager: "dnf", User: host.User{Name: "op", Groups: []string{"op"}}}
	if osr.ID == "ubuntu" || osr.ID == "debian" {
		h.PkgManager = "apt"
	}
	if len(archs) == 0 {
		archs = []string{"gfx1030"}
	}
	steps := Plan(context.Background(), h, rocm.Report{Tools: map[string]string{}},
		Options{Mode: "host", Venv: filepath.Join(t.TempDir(), "venv"), ROCm: rocmVersion, GPUArchs: archs})
	s, ok := stepMap(steps)["rocm.runtime"]
	if !ok {
		t.Fatalf("no rocm.runtime step for %s %s", osr.ID, osr.VersionID)
	}
	return s
}

// TestROCmRuntimeDistros checks the repository and the packages of the
// rocm.runtime step for every distribution bernie knows.
func TestROCmRuntimeDistros(t *testing.T) {
	const key = "https://repo.amd.com/rocm/packages-multi-arch/gpg/rocm.gpg"
	tests := []struct {
		name    string
		os      host.OSRelease
		version string
		archs   []string
		want    []string
	}{
		{
			name: "fedora 43 uses the rhel10 repository",
			os:   host.OSRelease{ID: "fedora", VersionID: "43"},
			want: []string{
				"sudo rpm --import " + key,
				"baseurl=https://repo.amd.com/rocm/packages-multi-arch/rhel10/x86_64",
				"gpgkey=" + key,
				"sudo dnf install -y amdrocm-core-sdk7.14-gfx1030 amdrocm-core7.14-gfx1030 amdrocm-blas7.14-gfx1030",
			},
		},
		{
			name: "fedora 41 uses the rhel9 repository",
			os:   host.OSRelease{ID: "fedora", VersionID: "41"},
			want: []string{
				"baseurl=https://repo.amd.com/rocm/packages-multi-arch/rhel9/x86_64",
				"sudo dnf install -y amdrocm-core-sdk7.14-gfx1030",
			},
		},
		{
			name: "rhel 10 uses the rhel10 repository",
			os:   host.OSRelease{ID: "rhel", VersionID: "10.0"},
			want: []string{"baseurl=https://repo.amd.com/rocm/packages-multi-arch/rhel10/x86_64"},
		},
		{
			name: "rocky 9 uses the rhel9 repository",
			os:   host.OSRelease{ID: "rocky", IDLike: "rhel fedora", VersionID: "9.4"},
			want: []string{"baseurl=https://repo.amd.com/rocm/packages-multi-arch/rhel9/x86_64"},
		},
		{
			name: "ubuntu 22.04 uses the ubuntu2204 repository",
			os:   host.OSRelease{ID: "ubuntu", VersionID: "22.04"},
			want: []string{
				"curl -fsSL " + key + " | gpg --dearmor",
				`deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.amd.com/rocm/packages-multi-arch/ubuntu2204 stable main`,
				"sudo apt update && sudo apt install -y amdrocm-core-sdk7.14-gfx1030 amdrocm-core7.14-gfx1030 amdrocm-blas7.14-gfx1030",
			},
		},
		{
			name: "ubuntu 24.04 uses the ubuntu2404 repository",
			os:   host.OSRelease{ID: "ubuntu", VersionID: "24.04"},
			want: []string{"https://repo.amd.com/rocm/packages-multi-arch/ubuntu2404 stable main"},
		},
		{
			name: "ubuntu 26.04 uses the ubuntu2604 repository",
			os:   host.OSRelease{ID: "ubuntu", VersionID: "26.04"},
			want: []string{"https://repo.amd.com/rocm/packages-multi-arch/ubuntu2604 stable main"},
		},
		{
			name: "debian 12 uses the debian12 repository",
			os:   host.OSRelease{ID: "debian", VersionID: "12"},
			want: []string{"https://repo.amd.com/rocm/packages-multi-arch/debian12 stable main"},
		},
		{
			name: "debian 13 uses the debian13 repository",
			os:   host.OSRelease{ID: "debian", VersionID: "13"},
			want: []string{"https://repo.amd.com/rocm/packages-multi-arch/debian13 stable main"},
		},
		{
			name:  "two targets get a package each",
			os:    host.OSRelease{ID: "fedora", VersionID: "43"},
			archs: []string{"gfx1030", "gfx1100"},
			want: []string{
				"sudo dnf install -y amdrocm-core-sdk7.14-gfx1030 amdrocm-core7.14-gfx1030 amdrocm-blas7.14-gfx1030 " +
					"amdrocm-core-sdk7.14-gfx1100 amdrocm-core7.14-gfx1100 amdrocm-blas7.14-gfx1100",
			},
		},
		{
			name:    "rocm 7.2 uses the per-release repository",
			os:      host.OSRelease{ID: "fedora", VersionID: "43"},
			version: host.ROCmVersion72,
			want: []string{
				"sudo rpm --import https://repo.radeon.com/rocm/rocm.gpg.key",
				"baseurl=https://repo.radeon.com/rocm/el10/7.2/main",
				"sudo dnf install -y rocm-core rocm-hip-sdk hipcc rocminfo rocm-smi-lib amd-smi-lib rocblas rocm-device-libs rocm-developer-tools",
			},
		},
		{
			name:    "rocm 7.2 on ubuntu 24.04 uses the noble suite",
			os:      host.OSRelease{ID: "ubuntu", VersionID: "24.04"},
			version: host.ROCmVersion72,
			want: []string{
				"curl -fsSL https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor",
				"https://repo.radeon.com/rocm/apt/7.2 noble main",
				"sudo apt update && sudo apt install -y rocm-core rocm-hip-sdk hipcc rocminfo",
			},
		},
		{
			name:    "rocm 7.2 has no apt suite for ubuntu 26.04",
			os:      host.OSRelease{ID: "ubuntu", VersionID: "26.04"},
			version: host.ROCmVersion72,
			want:    []string{"# Install ROCm 7.2: " + host.ROCmDocs},
		},
		{
			name: "arch has no AMD repository",
			os:   host.OSRelease{ID: "arch", VersionID: "20260101"},
			want: []string{"# Install ROCm 7.14: " + host.ROCmDocs},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := rocmStepOf(t, tt.os, tt.version, tt.archs)
			if s.Done {
				t.Error("rocm.runtime must not be done on a host without ROCm")
			}
			cmds := strings.Join(s.Commands, "\n")
			for _, want := range tt.want {
				if !strings.Contains(cmds, want) {
					t.Errorf("commands lack %q:\n%s", want, cmds)
				}
			}
			if len(s.Commands) == 0 {
				t.Error("rocm.runtime must print commands")
			}
		})
	}
}

// TestROCmRuntimeDone checks the postcondition of the step against host
// trees with the three ROCm layouts.
func TestROCmRuntimeDone(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{
			name: "a TheRock tree is done",
			files: map[string]string{
				"opt/rocm/core-7.14/.info/version":   "7.14.1\n",
				"opt/rocm/core-7.14/bin/hipcc":       "",
				"opt/rocm/core-7.14/bin/rocminfo":    "",
				"etc/ld.so.conf.d/10-rocm-7.14.conf": "/opt/rocm/core-7.14/lib\n",
			},
			want: true,
		},
		{
			name: "a versioned tree is done",
			files: map[string]string{
				"opt/rocm-7.2.0/.info/version": "7.2.0\n",
				"opt/rocm-7.2.0/bin/hipcc":     "",
			},
			want: true,
		},
		{
			name: "a plain tree is done",
			files: map[string]string{
				"opt/rocm/.info/version": "6.4.0\n",
				"opt/rocm/bin/hipcc":     "",
			},
			want: true,
		},
		{
			name: "a tree without hipcc is not done",
			files: map[string]string{
				"opt/rocm/core-7.14/.info/version": "7.14.1\n",
				"opt/rocm/core-7.14/bin/rocminfo":  "",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			fixture.Write(t, root, tt.files)
			rep := rocm.Probe(context.Background(), root)
			h := host.Info{OS: host.OSRelease{ID: "fedora", VersionID: "43"}, PkgManager: "dnf"}
			steps := Plan(context.Background(), h, rep, Options{Mode: "host", Venv: filepath.Join(t.TempDir(), "venv"), Root: root})
			got := stepMap(steps)["rocm.runtime"].Done
			if got != tt.want {
				t.Errorf("done = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestROCmRuntimeLdSoConf checks that the loader configuration is written
// only when the packages did not ship one.
func TestROCmRuntimeLdSoConf(t *testing.T) {
	const want = "sudo tee /etc/ld.so.conf.d/10-rocm-7.14.conf"
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{
		"opt/rocm/core-7.14/.info/version":      "7.14.1\n",
		"opt/rocm/core-7.14/bin/hipcc":          "",
		"opt/rocm/core-7.14/lib/libamdhip64.so": "",
	})
	rep := rocm.Probe(context.Background(), root)
	h := host.Info{OS: host.OSRelease{ID: "fedora", VersionID: "43"}, PkgManager: "dnf"}
	steps := Plan(context.Background(), h, rep, Options{Mode: "host", Venv: filepath.Join(t.TempDir(), "venv"), Root: root})
	cmds := strings.Join(stepMap(steps)["rocm.runtime"].Commands, "\n")
	if !strings.Contains(cmds, want) {
		t.Errorf("commands lack the loader configuration:\n%s", cmds)
	}
	if !strings.Contains(cmds, "/opt/rocm/core-7.14/lib") {
		t.Errorf("the loader configuration must name the ROCm libraries:\n%s", cmds)
	}

	root2 := t.TempDir()
	fixture.Write(t, root2, map[string]string{
		"opt/rocm/core-7.14/.info/version":   "7.14.1\n",
		"opt/rocm/core-7.14/bin/hipcc":       "",
		"etc/ld.so.conf.d/10-rocm-7.14.conf": "/opt/rocm/core-7.14/lib\n",
	})
	rep2 := rocm.Probe(context.Background(), root2)
	steps2 := Plan(context.Background(), h, rep2, Options{Mode: "host", Venv: filepath.Join(t.TempDir(), "venv"), Root: root2})
	cmds2 := strings.Join(stepMap(steps2)["rocm.runtime"].Commands, "\n")
	if strings.Contains(cmds2, want) {
		t.Errorf("the packages ship the loader configuration:\n%s", cmds2)
	}
}
