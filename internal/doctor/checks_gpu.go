package doctor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/gpu"
)

// supportChecks reports, per distinct gfx target of the host, whether the
// device tables know it and whether recipes are validated on it
// (gpu.supported.<gfx>), and the HSA override the table prescribes
// (gpu.hsa_override.<gfx>). The device facts come from internal/gpu/devices.
func supportChecks(in inputs) []Check {
	const cat = "gpu"
	var out []Check
	for _, gfx := range archOrder(in.gpus) {
		id, title := "supported."+gfx, gfx+" is supported"
		a, known := gpu.LookupArch(gfx)
		switch {
		case !known:
			out = append(out, check(cat, id, Warn, title, gfx+" is not in the device tables: its ROCm and vLLM support is unchecked").with("", docHardware))
		case slices.Contains(in.validated, gfx):
			out = append(out, check(cat, id, OK, title, a.Label()+" target; recipes are validated on it"))
		default:
			out = append(out, check(cat, id, Warn, title,
				fmt.Sprintf("%s target; recipes are validated on %s", a.Label(), validatedSentence(in.validated))))
		}
		if known && a.HSAOverride != "" {
			out = append(out, check(cat, "hsa_override."+gfx, Warn, gfx+" needs an HSA override",
				fmt.Sprintf("%s target without its own validated code objects; it runs %s code objects", a.Label(), a.CodeTarget())).
				with(fmt.Sprintf("Set HSA_OVERRIDE_GFX_VERSION=%[1]s in the serve environment (recipe env or --set env.HSA_OVERRIDE_GFX_VERSION=%[1]s).", a.HSAOverride), docHSA))
		}
	}
	return out
}

func validatedSentence(archs []string) string {
	switch len(archs) {
	case 0:
		return "no target yet"
	case 1:
		return archs[0] + " only"
	}
	return strings.Join(archs, ", ") + " only"
}
