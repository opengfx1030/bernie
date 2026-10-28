package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/host"
	"github.com/opengfx1030/rdnactl/internal/rocm"
)

func rocmChecks(in inputs) []Check {
	const cat = "rocm"
	pm := in.host.PkgManager
	r := in.rocm
	var out []Check
	alt := containerUsable(in.host) || in.site != "" && dirExists(filepath.Join(in.site, "_rocm_sdk_core"))
	switch len(r.Installs) {
	case 0:
		st, detail := Fail, "no ROCm under /opt/rocm*, /opt/rocm/core-* or /usr"
		if alt {
			st, detail = Warn, detail+"; container mode or the venv ROCm wheels still work"
		}
		out = append(out, check(cat, "installs", st, "ROCm is installed", detail).with(install(pm, host.PkgROCm, "Install ROCm: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/"), docInstall))
	case 1:
		out = append(out, check(cat, "installs", OK, "ROCm is installed", installList(r.Installs)).with("", docInstall))
	default:
		out = append(out, check(cat, "installs", Warn, "ROCm is installed",
			fmt.Sprintf("%d installs: %s; tools and libraries may mix versions", len(r.Installs), installList(r.Installs))).
			with("Keep one ROCm userspace on PATH/LD_LIBRARY_PATH (host mode uses the venv wheels first).", docInstall))
	}
	if len(in.gpus) > 1 {
		var bad []string
		for _, i := range r.Installs {
			if deadlockRange(i.Version) {
				bad = append(bad, i.Root+" "+i.Version)
			}
		}
		if v := wheelROCm(in.venvRep.Torch); deadlockRange(v) {
			bad = append(bad, "venv torch ROCm "+v)
		}
		if len(bad) > 0 {
			out = append(out, check(cat, "version", Warn, "ROCm version is safe for multi-GPU",
				strings.Join(bad, "; ")+": ROCm 7.2.1-7.13 has the RCCL tensor-parallel deadlock").
				with("Use ROCm 7.2.0 or 7.14 (venv wheels or image).", docROCmPin))
		} else if len(r.Installs) > 0 {
			out = append(out, check(cat, "version", OK, "ROCm version is safe for multi-GPU", "outside 7.2.1-7.13").with("", docROCmPin))
		}
	}
	switch {
	case r.Tools["rocminfo"] == "":
		out = append(out, check(cat, "rocminfo", Skip, "rocminfo lists the GPUs", "rocminfo not found").with(install(pm, host.PkgROCmInfo, "Install rocminfo."), docInstall))
	case in.root != "/":
		out = append(out, check(cat, "rocminfo", Skip, "rocminfo lists the GPUs", "not run for a fixture root"))
	case len(r.Agents) == 0 && len(in.gpus) > 0:
		out = append(out, check(cat, "rocminfo", Fail, "rocminfo lists the GPUs", "rocminfo lists no GPU agents").with("sudo usermod -aG video,render $USER  # then log out and in", docGeneral))
	case len(r.Agents) < len(in.gpus):
		out = append(out, check(cat, "rocminfo", Warn, "rocminfo lists the GPUs", fmt.Sprintf("%d of %d GPUs (check ROCR_VISIBLE_DEVICES/HIP_VISIBLE_DEVICES)", len(r.Agents), len(in.gpus))).with("", docGeneral))
	default:
		out = append(out, check(cat, "rocminfo", OK, "rocminfo lists the GPUs", fmt.Sprintf("%d GPU agents", len(r.Agents))))
	}
	if p := r.Tools["hipcc"]; p != "" {
		d := p
		if r.HIPVersion != "" {
			d += " (HIP " + r.HIPVersion + ")"
		}
		out = append(out, check(cat, "hipcc", OK, "hipcc is installed", d))
	} else {
		out = append(out, check(cat, "hipcc", Warn, "hipcc is installed", "needed to build vLLM in host mode").with(install(pm, host.PkgHIPCC, "Install the HIP SDK (hipcc)."), docHostVenv))
	}
	smi := r.Tools["amd-smi"]
	if smi == "" {
		smi = r.Tools["rocm-smi"]
	}
	if smi != "" {
		out = append(out, check(cat, "smi", OK, "amd-smi or rocm-smi is installed", smi))
	} else {
		out = append(out, check(cat, "smi", Warn, "amd-smi or rocm-smi is installed", "GPU monitoring tools missing").with(install(pm, host.PkgSMI, "Install rocm-smi."), docInstall))
	}
	switch {
	case in.rocblas != "":
		out = append(out, check(cat, "rocblas", OK, "librocblas.so.5 is found", in.rocblas))
	case alt:
		out = append(out, check(cat, "rocblas", Warn, "librocblas.so.5 is found", "no system or venv rocBLAS; container mode still works").
			with(install(pm, host.PkgROCBLAS, "Install rocBLAS."), docTunable))
	default:
		out = append(out, check(cat, "rocblas", Fail, "librocblas.so.5 is found", "no system or venv rocBLAS").
			with(install(pm, host.PkgROCBLAS, "Install rocBLAS."), docTunable))
	}
	return out
}

func installList(in []rocm.Install) string {
	var s []string
	for _, i := range in {
		s = append(s, fmt.Sprintf("%s (%s, %s)", i.Root, i.Version, i.Kind))
	}
	return strings.Join(s, ", ")
}

// deadlockRange reports ROCm 7.2.1 <= v <= 7.13.x.
func deadlockRange(v string) bool {
	if v == "" {
		return false
	}
	return rocm.CompareVersions(v, "7.2.1") >= 0 && rocm.CompareVersions(v, "7.14") < 0
}

// wheelROCm extracts "7.14.0" from a torch version "2.12.0+rocm7.14.0".
func wheelROCm(torch string) string {
	_, v, ok := strings.Cut(torch, "+rocm")
	if !ok {
		return ""
	}
	return v
}

func containerUsable(h host.Info) bool {
	for _, e := range h.Engines {
		if e.Usable {
			return true
		}
	}
	return false
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
