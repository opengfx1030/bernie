package doctor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
)

func pythonChecks(in inputs) []Check {
	const cat = "python"
	if in.venv == "" {
		return []Check{check(cat, "venv", Skip, "Python venv", "no venv: pass --venv DIR or set venv in the config").with("", docHostVenv)}
	}
	v := in.venvRep
	setupFix := "bernie setup apply --mode host"
	if v.Python == "" && v.Torch == "" {
		return []Check{check(cat, "venv", Fail, "Python venv", v.Err).with(setupFix, docHostVenv)}
	}
	var out []Check
	if strings.HasPrefix(v.Python, "3.12") {
		out = append(out, check(cat, "version", OK, "Python version", v.Python+" in "+in.venv))
	} else {
		out = append(out, check(cat, "version", Warn, "Python version", v.Python+": the wheels and images target 3.12").with("uv venv --python 3.12 "+in.venv, docHostVenv))
	}
	if v.Torch == "" {
		out = append(out, check(cat, "torch", Fail, "torch imports", v.Err).with("bernie setup apply --step venv.torch", docHostVenv))
		return out
	}
	out = append(out, check(cat, "torch", OK, "torch imports", v.Torch))
	if v.HIP == "" {
		out = append(out, check(cat, "torch_hip", Fail, "torch.version.hip is set", "torch "+v.Torch+" is not a ROCm build").with("bernie setup apply --step venv.torch", docGeneral))
	} else {
		out = append(out, check(cat, "torch_hip", OK, "torch.version.hip is set", "HIP "+v.HIP))
	}
	switch {
	case v.DeviceCount == 0:
		out = append(out, check(cat, "gpus", Fail, "torch sees the GPUs", "torch.cuda.device_count() = 0").with("", docGeneral))
	case v.DeviceCount != len(in.gpus):
		out = append(out, check(cat, "gpus", Warn, "torch sees the GPUs", fmt.Sprintf("torch sees %d, KFD has %d (HIP_VISIBLE_DEVICES set?)", v.DeviceCount, len(in.gpus))).with("", docGeneral))
	default:
		out = append(out, check(cat, "gpus", OK, "torch sees the GPUs", fmt.Sprintf("%d GPUs", v.DeviceCount)))
	}
	if in.site != "" && dirExists(filepath.Join(in.site, "_rocm_sdk_core")) {
		var missing []string
		targets := gpu.CodeTargets(archOrder(in.gpus))
		for _, gfx := range targets {
			if m, _ := filepath.Glob(filepath.Join(in.site, "rocm_sdk_device_"+gfx+"-*.dist-info")); len(m) == 0 {
				missing = append(missing, gfx)
			}
		}
		if len(missing) > 0 {
			out = append(out, check(cat, "device_wheels", Warn, "ROCm device wheels match the GPUs",
				"missing rocm-sdk-device wheels for "+strings.Join(missing, ", ")+" (hipErrorInvalidImage at kernel launch)").
				with("bernie setup apply --step venv.torch", docHostVenv))
		} else {
			out = append(out, check(cat, "device_wheels", OK, "ROCm device wheels match the GPUs", strings.Join(targets, ", ")))
		}
	}
	if tv := tritonVersion(in.site); tv != "" {
		out = append(out, tritonCheck(in, tv))
	}
	if v.VLLM == "" {
		out = append(out, check(cat, "vllm", Fail, "vllm imports", v.Err).with("bernie setup apply --step vllm.build", docHostVenv))
		return out
	}
	out = append(out, check(cat, "vllm", OK, "vllm imports", v.VLLM))
	if v.RDNAOps > 10 {
		out = append(out, check(cat, "rdna_ops", OK, "vllm-rdna ops are registered", fmt.Sprintf("%d _rocm_C rdna ops", v.RDNAOps)))
	} else {
		out = append(out, check(cat, "rdna_ops", Warn, "vllm-rdna ops are registered",
			fmt.Sprintf("%d _rocm_C rdna ops (need > 10 for vllm-rdna recipes)", v.RDNAOps)).
			with("bernie setup apply --step vllm.build", docHostVenv))
	}
	return out
}

func tritonVersion(site string) string {
	if site == "" {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(site, "triton-*.dist-info"))
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m[len(m)-1]), "triton-"), ".dist-info")
	return v
}

// tritonCheck fails when the installed Triton dropped a generation the host has.
func tritonCheck(in inputs, tv string) Check {
	const title = "Triton supports the GPU generations"
	for _, gfx := range archOrder(in.gpus) {
		if a, ok := gpu.LookupArch(gfx); ok && a.TritonBelow != "" && rocm.CompareVersions(tv, a.TritonBelow) >= 0 {
			return check("python", "triton", Fail, title, fmt.Sprintf("triton %s removed %s support (%s)", tv, a.Label(), gfx)).
				with(in.venv+"/bin/pip install triton==3.7.1", docHostVenv)
		}
	}
	return check("python", "triton", OK, title, "triton "+tv)
}
