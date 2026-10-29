package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// cdiRoot is a root tree with a CDI spec and the device nodes it names.
func cdiRoot(t *testing.T, spec string, nodes ...string) string {
	t.Helper()
	root := t.TempDir()
	if spec != "" {
		b, err := os.ReadFile(filepath.Join("testdata/cdi", spec))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "etc/cdi"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "etc/cdi/amd.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range nodes {
		p := filepath.Join(root, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The spec of the AMD toolkit is found in the CDI directory of the engine.
func TestProbeCDIFindsSpec(t *testing.T) {
	root := cdiRoot(t, "amd.json", "/dev/kfd", "/dev/dri/renderD128", "/dev/dri/renderD129", "/dev/dri/renderD130", "/dev/dri/renderD131")
	c := probeCDI(context.Background(), root, "docker", fakeRunner{})
	if c.Spec == nil {
		t.Fatal("no spec found")
	}
	if c.Spec.Kind != CDIKind {
		t.Errorf("kind = %q, want %q", c.Spec.Kind, CDIKind)
	}
	if c.Spec.Path != "/etc/cdi/amd.json" {
		t.Errorf("path = %q", c.Spec.Path)
	}
	if c.Spec.Version != "0.6.0" {
		t.Errorf("version = %q", c.Spec.Version)
	}
	if got := c.Spec.GPUCount(); got != 4 {
		t.Errorf("gpu count = %d, want 4 (the all device does not count)", got)
	}
}

// A host without the toolkit and without a spec has no CDI facts.
func TestProbeCDIWithoutSpec(t *testing.T) {
	c := probeCDI(context.Background(), cdiRoot(t, ""), "docker", fakeRunner{})
	if c.Spec != nil {
		t.Errorf("spec = %+v, want none", c.Spec)
	}
	if c.Toolkit {
		t.Error("the toolkit must not be reported without amd-ctk")
	}
}

// A spec of another vendor is not the AMD spec.
func TestProbeCDIOtherKind(t *testing.T) {
	c := probeCDI(context.Background(), cdiRoot(t, "other-kind.json"), "docker", fakeRunner{})
	if c.Spec != nil {
		t.Errorf("spec = %+v, want none", c.Spec)
	}
}

// amd-ctk is found and its version is read.
func TestProbeCDIToolkit(t *testing.T) {
	r := fakeRunner{
		bins: map[string]bool{"amd-ctk": true},
		out: map[string]execx.Result{
			"amd-ctk version": {Stdout: "Version: v1.3.0\nBuild Date: 2026-04-28T17:13:10Z\nGitCommit: 59f21d9\n"},
		},
	}
	c := probeCDI(context.Background(), cdiRoot(t, ""), "docker", r)
	if !c.Toolkit {
		t.Fatal("the toolkit must be reported")
	}
	if c.ToolkitVersion != "v1.3.0" {
		t.Errorf("version = %q", c.ToolkitVersion)
	}
}

// The daemon may name its own CDI directories.
func TestProbeCDISpecDirsFromDaemon(t *testing.T) {
	r := fakeRunner{out: map[string]execx.Result{
		"docker info --format {{json .CDISpecDirs}}": {Stdout: `["/etc/cdi","/var/run/cdi","/srv/cdi"]`},
	}}
	c := probeCDI(context.Background(), cdiRoot(t, ""), "docker", r)
	if len(c.SpecDirs) != 3 || c.SpecDirs[2] != "/srv/cdi" {
		t.Errorf("spec dirs = %v", c.SpecDirs)
	}
}

// A spec that names another number of GPUs, or a node the host does not
// have, is stale.
func TestSpecStale(t *testing.T) {
	root := cdiRoot(t, "amd.json", "/dev/kfd", "/dev/dri/renderD128", "/dev/dri/renderD129", "/dev/dri/renderD130", "/dev/dri/renderD131")
	c := probeCDI(context.Background(), root, "docker", fakeRunner{})
	if got := SpecStale(root, c.Spec, 4); got != "" {
		t.Errorf("a fresh spec must not be stale: %q", got)
	}
	if got := SpecStale(root, c.Spec, 2); got == "" {
		t.Error("a spec with another number of GPUs must be stale")
	}
	missing := cdiRoot(t, "amd.json", "/dev/kfd", "/dev/dri/renderD128")
	c2 := probeCDI(context.Background(), missing, "docker", fakeRunner{})
	if got := SpecStale(missing, c2.Spec, 4); got == "" {
		t.Error("a spec that names a node the host does not have must be stale")
	}
}

// The engine must know CDI, and the daemon must not switch it off.
func TestEngineCDISupport(t *testing.T) {
	cases := []struct {
		name    string
		engine  Engine
		daemon  string
		want    bool
		wantMsg string
	}{
		{"docker 29.8.1", Engine{Name: "docker", Version: "29.8.1"}, "", true, ""},
		{"docker 27.0 with the feature flag", Engine{Name: "docker", Version: "27.0.1"}, `{"features":{"cdi":true}}`, true, ""},
		{"docker 27.0 without the feature flag", Engine{Name: "docker", Version: "27.0.1"}, "", false, "features.cdi"},
		{"docker 24.0", Engine{Name: "docker", Version: "24.0.9"}, "", false, "25.0.0"},
		{"docker with cdi switched off", Engine{Name: "docker", Version: "29.8.1"}, `{"features":{"cdi":false}}`, false, "features.cdi"},
		{"podman 5.6", Engine{Name: "podman", Version: "5.6.2"}, "", true, ""},
		{"podman 3.9", Engine{Name: "podman", Version: "3.9.4"}, "", false, "4.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.daemon != "" {
				if err := os.MkdirAll(filepath.Join(root, "etc/docker"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "etc/docker/daemon.json"), []byte(tc.daemon), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ok, msg := EngineCDISupport(tc.engine, root)
			if ok != tc.want {
				t.Errorf("supported = %v, want %v (%s)", ok, tc.want, msg)
			}
			if !ok && !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("message = %q, want it to name %q", msg, tc.wantMsg)
			}
		})
	}
}

// auto uses CDI when the host can and the classic form otherwise.
func TestResolveDeviceAccess(t *testing.T) {
	spec := &CDISpec{Path: "/etc/cdi/amd.json", Kind: CDIKind}
	docker := Engine{Name: "docker", Version: "29.8.1"}
	cases := []struct {
		name     string
		req      string
		cdi      CDI
		stale    string
		why      string
		wantMode string
	}{
		{"auto without a spec", DeviceAccessAuto, CDI{}, "", "", DeviceAccessClassic},
		{"auto with a valid spec", DeviceAccessAuto, CDI{Toolkit: true, Spec: spec}, "", "", DeviceAccessCDI},
		{"auto with a stale spec", DeviceAccessAuto, CDI{Toolkit: true, Spec: spec}, "the host has 2 GPUs", "", DeviceAccessClassic},
		{"auto with an old engine", DeviceAccessAuto, CDI{Toolkit: true, Spec: spec}, "", "docker 24.0.9 does not support CDI", DeviceAccessClassic},
		{"classic always", DeviceAccessClassic, CDI{Toolkit: true, Spec: spec}, "", "", DeviceAccessClassic},
		{"cdi asked for", DeviceAccessCDI, CDI{Toolkit: true, Spec: spec}, "", "", DeviceAccessCDI},
		{"cdi asked for without a spec", DeviceAccessCDI, CDI{Toolkit: true}, "", "", DeviceAccessClassic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveDeviceAccess(tc.req, docker, tc.cdi, tc.stale, tc.why)
			if got.Mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if got.Reason == "" {
				t.Error("the mode needs a reason")
			}
		})
	}
}
