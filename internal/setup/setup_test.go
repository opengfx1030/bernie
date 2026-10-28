package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

func withWhich(t *testing.T, bins ...string) {
	old := which
	set := map[string]bool{}
	for _, b := range bins {
		set[b] = true
	}
	which = func(n string) string {
		if set[n] {
			return "/usr/bin/" + n
		}
		return ""
	}
	t.Cleanup(func() { which = old })
}

// par1Venv mirrors venv-7.14.0 on par1-cs25: torch + device wheels for
// gfx1030 and gfx1100 + the fork, no causal-conv1d, no flash-attn.
func par1Venv(t *testing.T) string {
	venv := t.TempDir()
	sp := "lib/python3.12/site-packages/"
	fixture.Write(t, venv, map[string]string{
		"bin/python": "",
		sp + "torch-2.12.0+rocm7.14.0.dist-info/METADATA":                  "",
		sp + "rocm_sdk_device_gfx1030-7.14.0.dist-info/METADATA":           "",
		sp + "rocm_sdk_device_gfx1100-7.14.0.dist-info/METADATA":           "",
		sp + "vllm-0.31.0+rdna.rocm714.rocm714.rocm714.dist-info/METADATA": "",
	})
	return venv
}

func par1Host() host.Info {
	return host.Info{
		PkgManager: "dnf", CPU: host.CPU{Cores: 48},
		User:    host.User{Name: "chenco_adm", Groups: []string{"chenco_adm", "video", "render", "docker"}},
		Engines: []host.Engine{{Name: "docker", Usable: true, Compose: "docker compose"}},
	}
}

func par1ROCm() rocm.Report {
	return rocm.Report{
		Installs: []rocm.Install{{Root: "/opt/rocm-7.2.0", Version: "7.2.0", Kind: "opt"}, {Root: "/opt/rocm-7.2.0/core-7.14", Version: "7.14.1", Kind: "therock"}},
		Tools:    map[string]string{"hipcc": "/opt/rocm/core-7.14/bin/hipcc"},
	}
}

func stepMap(steps []Step) map[string]Step {
	m := map[string]Step{}
	for _, s := range steps {
		m[s.ID] = s
	}
	return m
}

func idsOf(steps []Step) string {
	var s []string
	for _, st := range steps {
		s = append(s, st.ID)
	}
	return strings.Join(s, " ")
}

func TestPlanHostPar1(t *testing.T) {
	withWhich(t, "uv")
	venv := par1Venv(t)
	steps := Plan(context.Background(), par1Host(), par1ROCm(), Options{Mode: "host", Venv: venv, SrcDir: "/src", GPUArchs: []string{"gfx1100", "gfx1100", "gfx1030"}})
	if got, want := idsOf(steps), "groups rocm.runtime venv.create venv.torch causal-conv1d.build flash-attn.build vllm.build"; got != want {
		t.Fatalf("steps = %s, want %s", got, want)
	}
	m := stepMap(steps)
	for id, done := range map[string]bool{"groups": true, "rocm.runtime": true, "venv.create": true, "venv.torch": true, "causal-conv1d.build": false, "flash-attn.build": false, "vllm.build": true} {
		if m[id].Done != done {
			t.Errorf("%s done = %v, want %v", id, m[id].Done, done)
		}
	}
	torch := strings.Join(m["venv.torch"].Commands, "\n")
	for _, want := range []string{"uv pip install --python " + filepath.Join(venv, "bin/python"), WheelIndex, "torch==2.12.0+rocm7.14.0", "triton==3.7.1", "rocm-sdk-device-gfx1100==7.14.0", "amd-torch-device-gfx1030==2.12.0+rocm7.14.0"} {
		if !strings.Contains(torch, want) {
			t.Errorf("venv.torch commands lack %q:\n%s", want, torch)
		}
	}
	build := strings.Join(m["vllm.build"].Commands, "\n")
	for _, want := range []string{"git clone -b rdna_extras https://github.com/opengfx1030/vllm-rdna.git /src/vllm-rdna", "PYTORCH_ROCM_ARCH='gfx1100;gfx1030'", "ROCM_HOME=/opt/rocm-7.2.0/core-7.14", "MAX_JOBS=16", "-e . triton==3.7.1"} {
		if !strings.Contains(build, want) {
			t.Errorf("vllm.build commands lack %q:\n%s", want, build)
		}
	}
	fa := strings.Join(m["flash-attn.build"].Commands, "\n")
	if !strings.Contains(fa, "GPU_ARCHS=gfx1100 ") || strings.Contains(fa, "gfx1030") {
		t.Errorf("flash-attn must target only allowlisted archs:\n%s", fa)
	}
}

func TestPlanFreshHost(t *testing.T) {
	withWhich(t, "python3.12")
	h := host.Info{PkgManager: "apt", User: host.User{Name: "op", Groups: []string{"op"}}}
	venv := filepath.Join(t.TempDir(), "venv")
	steps := Plan(context.Background(), h, rocm.Report{Tools: map[string]string{}}, Options{Venv: venv, GPUArchs: []string{"gfx1032"}, ModelRepo: "Qwen/Qwen3-8B", HFHome: t.TempDir()})
	m := stepMap(steps)
	// gfx1032 builds as gfx1030: no flash-attn step, gfx1030 device wheels.
	if _, ok := m["flash-attn.build"]; ok {
		t.Error("flash-attn.build must not apply to RDNA2-only hosts")
	}
	for _, s := range steps {
		if s.Done {
			t.Errorf("%s done on a fresh host", s.ID)
		}
	}
	if c := m["venv.create"].Commands[0]; c != "python3.12 -m venv "+venv {
		t.Errorf("venv.create = %q", c)
	}
	if !strings.Contains(m["venv.torch"].Commands[1], "rocm-sdk-device-gfx1030") || !strings.HasPrefix(m["venv.torch"].Commands[0], filepath.Join(venv, "bin/python")+" -m pip install") {
		t.Errorf("venv.torch = %v", m["venv.torch"].Commands)
	}
	if !strings.Contains(strings.Join(m["rocm.runtime"].Commands, "\n"), "repo.radeon.com/rocm/apt/7.2") {
		t.Errorf("rocm.runtime apt = %v", m["rocm.runtime"].Commands)
	}
	if c := m["container.engine"].Commands; c[0] != "sudo apt install -y docker.io docker-compose-v2" || len(c) != 3 {
		t.Errorf("container.engine = %v", c)
	}
	if d := m["model.download"].Download; d == nil || d.Repo != "Qwen/Qwen3-8B" || m["model.download"].Mode != "both" {
		t.Errorf("model.download = %+v", m["model.download"])
	} else if !strings.HasSuffix(d.HubCache, "/hub") || d.ResumeBytes != 0 {
		t.Errorf("model.download = %+v", d)
	}
	if got := idsOf(Plan(context.Background(), h, rocm.Report{}, Options{Mode: "container", Venv: venv})); got != "groups container.engine image.pull" {
		t.Errorf("container steps = %s", got)
	}
}

func TestModelCached(t *testing.T) {
	hub := t.TempDir()
	fixture.Write(t, hub, map[string]string{
		"models--org--name/refs/main":                 "abc",
		"models--org--name/snapshots/abc/config.json": "{}",
	})
	if !modelCached(hub, "org/name") || modelCached(hub, "org/other") {
		t.Error("modelCached wrong")
	}
	// A snapshot without the ref is an interrupted download.
	partial := t.TempDir()
	fixture.Write(t, partial, map[string]string{"models--org--name/snapshots/abc/config.json": "{}"})
	if modelCached(partial, "org/name") {
		t.Error("modelCached must be false without refs/main")
	}
}

func TestApply(t *testing.T) {
	dir := t.TempDir()
	steps := []Step{
		{ID: "groups", Root: true, Commands: []string{"sudo usermod -aG video,render op"}},
		{ID: "a", Done: true, Commands: []string{"echo SHOULD-NOT-RUN"}},
		{ID: "b", Commands: []string{"echo ran-b > " + dir + "/b"}},
		{ID: "c", Commands: []string{"exit 3", "echo SHOULD-NOT-RUN"}},
		{ID: "d", Commands: []string{"echo SHOULD-NOT-RUN"}},
	}
	var log bytes.Buffer
	err := Apply(context.Background(), steps, nil, &log)
	if err == nil || !strings.Contains(err.Error(), "step c") {
		t.Fatalf("err = %v", err)
	}
	out := log.String()
	if strings.Count(out, "SHOULD-NOT-RUN") != 0 {
		t.Errorf("done/after-failure commands appeared:\n%s", out)
	}
	if !fileHas(dir+"/b", "ran-b") {
		t.Error("step b did not run")
	}

	log.Reset()
	if err := Apply(context.Background(), steps, []string{"groups", "b"}, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "Run these root steps yourself") || !strings.Contains(log.String(), "sudo usermod -aG video,render op") {
		t.Errorf("root step not printed:\n%s", log.String())
	}
	if err := Apply(context.Background(), steps, []string{"nope"}, &log); err == nil {
		t.Error("unknown step accepted")
	}
}

func fileHas(p, s string) bool {
	b, _ := os.ReadFile(p)
	return strings.Contains(string(b), s)
}

func TestShq(t *testing.T) {
	for in, want := range map[string]string{
		"/opt/rocm":       "/opt/rocm",
		"gfx1100;gfx1030": "'gfx1100;gfx1030'",
		"it's":            `'it'\''s'`,
		"":                "''",
	} {
		if got := shq(in); got != want {
			t.Errorf("shq(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestPlanDeviceWheelsFollowDeviceTables(t *testing.T) {
	withWhich(t, "uv")
	cases := []struct {
		name   string
		archs  []string
		wheels []string // device wheels expected in venv.torch
		noWhl  []string
		flash  string // GPU_ARCHS of flash-attn.build, "" = no step
		build  string // PYTORCH_ROCM_ARCH, "" = unset
		why    string
	}{
		{"rdna3 desktop", []string{"gfx1100"}, []string{"rocm-sdk-device-gfx1100==7.14.0", "amd-torch-device-gfx1100==2.12.0+rocm7.14.0"}, []string{"gfx1030"}, "gfx1100", "gfx1100", "Triton is pinned to 3.7.1."},
		{"rdna4", []string{"gfx1201", "gfx1200"}, []string{"rocm-sdk-device-gfx1201==", "rocm-sdk-device-gfx1200=="}, nil, "gfx1201;gfx1200", "gfx1201;gfx1200", "Triton is pinned"},
		{"rdna2 cousin runs gfx1030", []string{"gfx1031", "gfx1030"}, []string{"rocm-sdk-device-gfx1030=="}, []string{"gfx1031"}, "", "gfx1030", "Triton 3.9 removed RDNA2."},
		{"rdna3 cousin keeps its own target", []string{"gfx1102"}, []string{"rocm-sdk-device-gfx1102=="}, nil, "gfx1102", "gfx1102", "Triton is pinned"},
		{"no GPU", nil, nil, []string{"rocm-sdk-device"}, "", "", "no device wheels are planned"},
	}
	for _, c := range cases {
		steps := Plan(context.Background(), par1Host(), par1ROCm(), Options{Mode: "host", Venv: t.TempDir(), SrcDir: "/src", GPUArchs: c.archs})
		m := stepMap(steps)
		torch := strings.Join(m["venv.torch"].Commands, "\n")
		for _, w := range c.wheels {
			if !strings.Contains(torch, w) {
				t.Errorf("%s: venv.torch lacks %q:\n%s", c.name, w, torch)
			}
		}
		for _, w := range c.noWhl {
			if strings.Contains(torch, w) {
				t.Errorf("%s: venv.torch has %q:\n%s", c.name, w, torch)
			}
		}
		if !strings.Contains(m["venv.torch"].Why, c.why) {
			t.Errorf("%s: why = %q", c.name, m["venv.torch"].Why)
		}
		fa, ok := m["flash-attn.build"]
		if c.flash == "" && ok || c.flash != "" && (!ok || !strings.Contains(strings.Join(fa.Commands, "\n"), "GPU_ARCHS="+shq(c.flash)+" ")) {
			t.Errorf("%s: flash-attn = %v (%v), want GPU_ARCHS=%q", c.name, fa.Commands, ok, c.flash)
		}
		build := strings.Join(m["vllm.build"].Commands, "\n")
		if c.build == "" && strings.Contains(build, "PYTORCH_ROCM_ARCH") || c.build != "" && !strings.Contains(build, "PYTORCH_ROCM_ARCH="+shq(c.build)+" ") {
			t.Errorf("%s: vllm.build = %s, want PYTORCH_ROCM_ARCH=%q", c.name, build, c.build)
		}
	}
}
