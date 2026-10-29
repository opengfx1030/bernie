package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// PrepareOptions are the inputs of Prepare: the same as `serve` and
// `recipe plan`.
type PrepareOptions struct {
	Recipe string
	Mode   string // host | container | "" (flag absent: config, then auto)
	Name   string
	Venv   string
	Engine string
	// DeviceAccess is how the container gets the GPU: classic, cdi or
	// auto (the default, spec §8.2).
	DeviceAccess string
	Sets         []string // --set, applied after the config defaults
	// Profile forces a TunableOp profile; NoTunableOp skips TunableOp;
	// Tuning enables online tuning into the per-user file.
	Profile     string
	NoTunableOp bool
	Tuning      bool
	// Extra recipe directories or files (recipe validate, tests).
	RecipePaths []string
}

// Prepare runs the launch flow up to the plan (architecture §4): config
// defaults, recipe load and resolve, GPU and topology discovery, mode
// choice, BuildPlan, host environment (ROCm, HF home), TunableOp profile
// selection and rows, container mounts, and the exact command (Plan.Exec).
// Both the CLI and the dashboard call it.
func Prepare(ctx context.Context, o PrepareOptions) (*recipe.Plan, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	set := recipe.Load(o.RecipePaths...)
	r, err := set.Resolve(o.Recipe)
	if err != nil {
		return nil, err
	}
	venv := cfg.ResolveVenv(o.Venv)

	var gpus []gpu.GPU
	var tp *topo.Topology
	// Only the GPUs the environment leaves visible are used (spec §7.7).
	vis, verr := gpu.Visible(hostroot.Root())
	if gpu.Restricted(verr) {
		return nil, verr // the restriction names a GPU this host does not have
	}
	if vis.GPUs != nil {
		gpus = vis.GPUs
		if t, err := topo.Discover(hostroot.Root(), gpus); err == nil {
			tp = &t
		}
	}
	restriction := vis.Restriction.Note()

	var warnings []string
	mode := o.Mode
	if mode == "" {
		mode = cfg.Mode
		if mode == recipe.ModeContainer && r.Container == nil {
			warnings = append(warnings, fmt.Sprintf("config mode is container, but recipe %s is host mode only; using host", r.ID))
			mode = recipe.ModeHost
		}
	}
	if mode == "" {
		mode = recipe.ModeHost
		if r.Container != nil {
			if _, err := resolveEngine(ctx, o.Engine, cfg); err == nil {
				mode = recipe.ModeContainer
			}
		}
	}

	sets := append(append([]string{}, cfg.Set...), o.Sets...)
	p, err := recipe.BuildPlan(r, recipe.PlanOptions{Mode: mode, Name: o.Name, Sets: sets, GPUs: gpus, Topo: tp, TunedMoE: tuning.MaterializeTunedMoE, Restriction: restriction})
	if err != nil {
		return nil, err
	}
	p.Warnings = append(warnings, p.Warnings...)
	// The device access mode: the flag, then the config, then the recipe.
	if req := o.DeviceAccess; req != "" {
		p.DeviceAccessRequest = req
	} else if cfg.DeviceAccess != "" {
		p.DeviceAccessRequest = cfg.DeviceAccess
	}
	if req := p.DeviceAccessRequest; req != "" && !slices.Contains(host.DeviceAccessModes, req) {
		return nil, fmt.Errorf("device access must be one of %s, not %q", strings.Join(host.DeviceAccessModes, ", "), req)
	}

	if mode == recipe.ModeHost {
		hostEnvDefaults(p, cfg)
		if venv == "" {
			p.Warnings = append(p.Warnings, "no venv: use --venv, $BERNIE_VENV, `bernie config set venv DIR` or activate one")
		}
		if v := p.Env["VLLM_TUNED_CONFIG_FOLDER"]; v != "" && !isDir(v) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("VLLM_TUNED_CONFIG_FOLDER %s does not exist on this host", v))
		}
	} else {
		containerMounts(p)
		p.GroupAdd = host.GPUGroups(ctx, hostroot.Root())
		if len(p.GroupAdd) == 0 {
			p.Warnings = append(p.Warnings, "no GPU group id found on this host: the container may not open /dev/kfd")
		}
	}
	if !o.NoTunableOp {
		if err := applyTunableOp(p, gpu.Archs(gpus, p.GPUs), venv, o.Profile, o.Tuning); err != nil {
			return nil, err
		}
	}

	if mode == recipe.ModeHost && venv == "" {
		p.Exec = &recipe.Exec{Argv: hostArgv(p, "$VENV/bin/python"), Env: hostEnv(p, ""), Unset: p.Unset, Dir: RunDir(p.Name)}
	} else if ex, err := Command(p, StartOptions{Venv: venv, Engine: o.Engine}); err == nil {
		p.Exec = ex
	} else {
		p.Warnings = append(p.Warnings, err.Error())
		if mode == recipe.ModeContainer {
			p.Exec = &recipe.Exec{Argv: ContainerCommand(p, host.Engine{Name: "docker"}, rowsMount(p), cfg.ResolveHFHome())}
		} else {
			p.Exec = &recipe.Exec{Argv: hostArgv(p, filepath.Join(venv, "bin", "python")), Env: hostEnv(p, venv), Unset: p.Unset, Dir: RunDir(p.Name)}
		}
	}
	p.RefreshSettingValues()
	return p, nil
}

// hostEnvDefaults adds ROCM_HOME/ROCM_PATH/HIP_PATH/HIP_ROOT_DIR from the
// ROCm install the host runs (the tree whose tools win on PATH, else the
// newest install, nested trees included) and HF_HOME from the config,
// unless the recipe or --set gave them.
func hostEnvDefaults(p *recipe.Plan, cfg config.Config) {
	best := rocm.RuntimeInstall(hostroot.Root())
	if best.Root != "" {
		for _, k := range []string{"ROCM_HOME", "ROCM_PATH", "HIP_PATH", "HIP_ROOT_DIR"} {
			if _, ok := p.Env[k]; !ok {
				p.Env[k] = best.Root
			}
		}
	}
	if cfg.HFHome != "" {
		if _, ok := p.Env["HF_HOME"]; !ok {
			p.Env["HF_HOME"] = cfg.ResolveHFHome()
		}
	}
}

// containerMounts mounts a tuned-MoE directory that exists on the host at
// /bernie/tuned-moe (a path inside the image, like the default
// /src/vllm/tuned-moe, needs no mount).
func containerMounts(p *recipe.Plan) {
	v := p.Env["VLLM_TUNED_CONFIG_FOLDER"]
	if v == "" || !filepath.IsAbs(v) || !isDir(v) {
		return
	}
	p.Mounts = append(p.Mounts, recipe.Mount{Host: v, Container: containerTunedMoE, ReadOnly: true})
	p.Env["VLLM_TUNED_CONFIG_FOLDER"] = containerTunedMoE
}

// applyTunableOp selects the profile (host: by the loaded rocBLAS hash and
// the architecture of the selected GPUs, archs; container: the recipe's
// profile), materializes its rows and merges the TunableOp environment.
// Variables the recipe or --set gave win.
func applyTunableOp(p *recipe.Plan, archs []string, venv, explicit string, tune bool) error {
	var prof *tuning.Profile
	var env map[string]string
	if p.Mode == recipe.ModeContainer {
		name := explicit
		if name == "" {
			name = p.TunableOpProfile
		}
		if name == "" {
			p.Warnings = append(p.Warnings, "recipe names no TunableOp profile for its image; TunableOp is off")
			return nil
		}
		pr, err := tuning.Select("", name, archs)
		if err != nil {
			return err
		}
		prof = pr
		env = tuning.Env(prof, containerRowsDir+"/"+prof.Name, tune)
		if tune {
			p.Warnings = append(p.Warnings, "--tuning in container mode writes inside the container (lost on exit)")
		}
	} else {
		lib := rocm.FindROCBLAS(hostroot.Root(), venv)
		if lib == "" {
			p.Warnings = append(p.Warnings, "librocblas.so.5 not found (venv wheel or system); TunableOp is off")
			return nil
		}
		sha, err := tuning.HashLib(lib)
		if err != nil {
			return fmt.Errorf("hash %s: %w", lib, err)
		}
		prof, err = tuning.Select(sha, explicit, archs)
		if err != nil {
			return err
		}
		if prof == nil {
			on := ""
			if len(archs) > 0 {
				on = " on " + strings.Join(archs, "/")
			}
			p.Warnings = append(p.Warnings, fmt.Sprintf("no TunableOp profile matches %s (sha256 %s)%s; using per-user rows %s",
				lib, sha[:tuning.SHAPrefix], on, tuning.PerUserFile()))
		}
	}
	if prof != nil {
		dir, err := tuning.Materialize(prof)
		if err != nil {
			return err
		}
		p.TunableOpProfile = prof.Name
		p.TunableOpRows = dir
		if env == nil {
			env = tuning.Env(prof, dir, tune)
		}
	} else if env == nil {
		env = tuning.Env(nil, "", tune)
	}
	for k, v := range env {
		if _, ok := p.Env[k]; !ok {
			p.Env[k] = v
		}
	}
	return nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
