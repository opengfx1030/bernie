package host

import (
	"context"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/execx"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

func TestProbePar1(t *testing.T) {
	root := fixture.Root(t, "par1-cs25")
	info, err := Probe(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if info.OS.ID != "fedora" || info.OS.VersionID != "43" || info.OS.Pretty != "Fedora Linux 43 (Server Edition)" {
		t.Errorf("os = %+v", info.OS)
	}
	if info.PkgManager != "dnf" || info.Kernel != "7.1.7-100.p2p_post7.fc43.x86_64" || info.Hostname != "par1-cs25" {
		t.Errorf("pkg=%q kernel=%q host=%q", info.PkgManager, info.Kernel, info.Hostname)
	}
	want := CPU{Model: "AMD EPYC 7K62 48-Core Processor", Vendor: "AuthenticAMD", Sockets: 1, Cores: 48, Threads: 96}
	if info.CPU != want {
		t.Errorf("cpu = %+v", info.CPU)
	}
	if info.RAMBytes < 500e9 || info.SELinux != "enforcing" || info.Virtualization != "none" {
		t.Errorf("ram=%d selinux=%q virt=%q", info.RAMBytes, info.SELinux, info.Virtualization)
	}
	if !strings.Contains(info.Cmdline, "iommu=pt") {
		t.Errorf("cmdline = %q", info.Cmdline)
	}
	if len(info.Engines) != 0 {
		t.Errorf("engines must not be probed for a fixture root: %+v", info.Engines)
	}
}

func TestPkgManager(t *testing.T) {
	cases := []struct {
		osr  OSRelease
		want string
	}{
		{OSRelease{ID: "ubuntu", IDLike: "debian"}, "apt"},
		{OSRelease{ID: "pop", IDLike: "ubuntu debian"}, "apt"},
		{OSRelease{ID: "someos", IDLike: "rhel centos fedora"}, "dnf"},
		{OSRelease{ID: "cachyos", IDLike: "arch"}, "pacman"},
		{OSRelease{ID: "opensuse-tumbleweed", IDLike: "opensuse suse"}, "zypper"},
		{OSRelease{ID: "unknown"}, ""},
	}
	for _, c := range cases {
		if got := PkgManager(t.TempDir(), c.osr); got != c.want {
			t.Errorf("%+v: got %q want %q", c.osr, got, c.want)
		}
	}
	root := t.TempDir()
	fixture.Write(t, root, map[string]string{"usr/bin/zypper": ""})
	if got := PkgManager(root, OSRelease{ID: "unknown"}); got != "zypper" {
		t.Errorf("binary fallback = %q", got)
	}
}

func TestParseCPUInfoTwoSockets(t *testing.T) {
	var b strings.Builder
	for p := range 8 {
		sock := p / 4
		core := (p % 4) / 2 // 2 cores x 2 threads per socket
		b.WriteString("processor\t: " + itoa(p) + "\nvendor_id\t: GenuineIntel\nmodel name\t: Xeon\nphysical id\t: " + itoa(sock) + "\ncore id\t\t: " + itoa(core) + "\ncpu cores\t: 2\n\n")
	}
	got := ParseCPUInfo(b.String())
	want := CPU{Model: "Xeon", Vendor: "GenuineIntel", Sockets: 2, Cores: 4, Threads: 8}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
	// ARM-style cpuinfo without topology fields.
	got = ParseCPUInfo("processor\t: 0\nprocessor\t: 1\n")
	if got.Sockets != 1 || got.Cores != 2 || got.Threads != 2 {
		t.Errorf("no topology: %+v", got)
	}
}

func TestVirtualization(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{"sys/class/dmi/id/sys_vendor": "QEMU", "sys/class/dmi/id/product_name": "Standard PC (Q35 + ICH9, 2009)"}, "kvm"},
		{map[string]string{"sys/class/dmi/id/sys_vendor": "Microsoft Corporation", "sys/class/dmi/id/product_name": "Virtual Machine"}, "microsoft"},
		{map[string]string{"run/.containerenv": ""}, "podman"},
		{map[string]string{"proc/cpuinfo": "processor : 0\nflags : fpu hypervisor sse\n"}, "vm"},
		{map[string]string{"sys/class/dmi/id/sys_vendor": "Supermicro"}, "none"},
	}
	for _, c := range cases {
		root := t.TempDir()
		fixture.Write(t, root, c.files)
		if got := Virtualization(root, CPU{}); got != c.want {
			t.Errorf("%v: got %q want %q", c.files, got, c.want)
		}
	}
}

// fakeRunner answers commands from a table keyed by "name arg...".
type fakeRunner struct {
	bins map[string]bool
	out  map[string]execx.Result
}

func (f fakeRunner) Which(name string) string {
	if f.bins[name] {
		return "/usr/bin/" + name
	}
	return ""
}

func (f fakeRunner) Run(_ context.Context, name string, args ...string) execx.Result {
	if r, ok := f.out[strings.Join(append([]string{name}, args...), " ")]; ok {
		return r
	}
	return execx.Result{ExitCode: 1}
}

func TestProbeEngines(t *testing.T) {
	ok := func(s string) execx.Result { return execx.Result{Stdout: s} }
	cases := []struct {
		name string
		f    fakeRunner
		want []Engine
	}{
		{"docker with plugin (par1-cs25)", fakeRunner{
			bins: map[string]bool{"docker": true},
			out: map[string]execx.Result{
				"docker version --format {{.Client.Version}}":    ok("29.8.1\n"),
				"docker info --format {{json .SecurityOptions}}": ok(`["name=seccomp,profile=builtin","name=selinux","name=cgroupns"]`),
				"docker compose version":                         ok("Docker Compose version v5.5.1\n"),
			}},
			[]Engine{{Name: "docker", Path: "/usr/bin/docker", Version: "29.8.1", Usable: true, Compose: "docker compose"}}},
		{"docker daemon denied, standalone compose", fakeRunner{
			bins: map[string]bool{"docker": true, "docker-compose": true},
			out:  map[string]execx.Result{"docker version --format {{.Client.Version}}": ok("27.0.1\n")}},
			[]Engine{{Name: "docker", Path: "/usr/bin/docker", Version: "27.0.1", Compose: "docker-compose"}}},
		{"rootless docker", fakeRunner{
			bins: map[string]bool{"docker": true},
			out:  map[string]execx.Result{"docker info --format {{json .SecurityOptions}}": ok(`["name=seccomp,profile=builtin","name=rootless"]`)}},
			[]Engine{{Name: "docker", Path: "/usr/bin/docker", Usable: true, Rootless: true}}},
		{"rootless podman with podman-compose", fakeRunner{
			bins: map[string]bool{"podman": true, "podman-compose": true},
			out: map[string]execx.Result{
				"podman version --format {{.Client.Version}}":      ok("5.6.2\n"),
				"podman info --format {{.Host.Security.Rootless}}": ok("true\n"),
			}},
			[]Engine{{Name: "podman", Path: "/usr/bin/podman", Version: "5.6.2", Usable: true, Rootless: true, Compose: "podman-compose"}}},
		{"none", fakeRunner{}, []Engine{}},
	}
	for _, c := range cases {
		got := ProbeEngines(context.Background(), c.f)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %+v", c.name, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %+v want %+v", c.name, got[i], c.want[i])
			}
		}
	}
}

func TestPreferred(t *testing.T) {
	es := []Engine{{Name: "docker"}, {Name: "podman", Usable: true}}
	if e, _ := Preferred(es, ""); e.Name != "podman" {
		t.Errorf("usable first: %s", e.Name)
	}
	if e, _ := Preferred(es, "docker"); e.Name != "docker" {
		t.Errorf("explicit: %s", e.Name)
	}
	if _, ok := Preferred(es[:1], "podman"); ok {
		t.Error("explicit missing engine must fail")
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestProbeEmptyRoot(t *testing.T) {
	// macOS or a minimal container: no /etc/os-release, /proc or /sys.
	info, err := Probe(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Probe must be best-effort, got %v", err)
	}
	if info.OS != (OSRelease{}) || info.PkgManager != "" || info.Kernel != "" || info.RAMBytes != 0 || info.CPU != (CPU{}) {
		t.Errorf("empty root must give empty fields: %+v", info)
	}
	if info.Virtualization != "none" || info.SELinux != "" || info.Engines == nil {
		t.Errorf("virt=%q selinux=%q engines=%v", info.Virtualization, info.SELinux, info.Engines)
	}
}
