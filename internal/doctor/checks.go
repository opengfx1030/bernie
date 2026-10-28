package doctor

import (
	"github.com/opengfx1030/rdnactl/internal/machine/host"
)

// WikiURL is the gfx1030 community wiki used for doc links.
const WikiURL = "https://blivioniag.github.io/gfx1030-wiki/"

var (
	docOS       = WikiURL + "setup/installing-rocm.html#supported-operating-systems"
	docInstall  = WikiURL + "setup/installing-rocm.html"
	docGotchas  = WikiURL + "setup/installing-rocm.html#notes--gotchas"
	docROCmPin  = WikiURL + "setup/installing-rocm.html#multi-gpu-pin-rocm-720-or-7140"
	docGeneral  = WikiURL + "troubleshooting/general.html"
	docGroups   = WikiURL + "troubleshooting/vllm.html#failed-to-infer-device-type--amdsmi_status_not_init"
	docHardware = WikiURL + "setup/hardware.html"
	docHSA      = WikiURL + "setup/hsa-override.html"
	docP2P      = WikiURL + "tuning/p2p.html"
	docTopology = WikiURL + "tuning/p2p.html#host-topology"
	docHostVenv = WikiURL + "vllm/host-venv.html"
	docTunable  = WikiURL + "vllm/serve-scripts.html#tunableop"
	docMismatch = WikiURL + "troubleshooting/vllm.html#tunableop-rocblas-mismatch"
	docRecipes  = WikiURL + "vllm/recipes.html"
	docCompose  = WikiURL + "vllm/configuration.html#docker-compose-example-gptq--mtp--cuda-graphs"
	docRunning  = WikiURL + "vllm/running.html"
	docPower    = WikiURL + "tuning/power.html"
)

const gib = 1 << 30

// evaluate runs every check on the gathered inputs.
func evaluate(in inputs) []Check {
	var out []Check
	for _, f := range []func(inputs) []Check{osChecks, gpuChecks, kernelChecks, rocmChecks, pythonChecks, containerChecks, resourceChecks, tuningChecks} {
		out = append(out, f(in)...)
	}
	return out
}

func check(cat, id string, st Status, title, detail string) Check {
	return Check{ID: cat + "." + id, Category: cat, Status: st, Title: title, Detail: detail}
}

func (c Check) with(fix, doc string) Check {
	c.Fix, c.Doc = fix, doc
	return c
}

// install returns the package-manager command, else a sentence.
func install(pm, feature, sentence string) string {
	if cmd := host.InstallCommand(pm, feature); cmd != "" {
		return cmd
	}
	return sentence
}
