package doctor

import (
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

// p2pChecks turns the P2P readiness checklist into doctor checks. The
// checklist is the same one `bernie topo --check` prints.
func p2pChecks(in inputs) []Check {
	const cat = "topo.p2p"
	items, err := topo.P2PChecklist(in.root, in.gpus, topo.P2POptions{
		Cmdline: in.host.Cmdline, PkgManager: in.host.PkgManager, Recipe: in.recipe,
	})
	if err != nil {
		return []Check{check(cat, "error", Skip, "P2P readiness",
			"the PCIe topology could not be read: "+err.Error()).with("", topo.P2PDoc)}
	}
	out := make([]Check, 0, len(items))
	for _, it := range items {
		out = append(out, check(cat, it.ID, Status(it.Status), it.Title, it.Detail).with(it.Fix, it.Doc))
	}
	return out
}
