package topo

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// Doc links for the readiness items.
const (
	P2PDoc      = "https://blivioniag.github.io/gfx1030-wiki/tuning/p2p.html"
	TopologyDoc = P2PDoc + "#host-topology"
)

// P2PStatus is the status of one readiness item.
type P2PStatus string

// Checklist statuses. A fail blocks peer-to-peer copies, a warn costs
// performance or stability, a skip means the fact is unknown here.
const (
	P2POK   P2PStatus = "ok"
	P2PWarn P2PStatus = "warn"
	P2PFail P2PStatus = "fail"
	P2PSkip P2PStatus = "skip"
)

// CheckItem is one item of the P2P readiness checklist.
type CheckItem struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Status P2PStatus `json:"status"`
	Detail string    `json:"detail"`
	Fix    string    `json:"fix,omitempty"`
	Doc    string    `json:"doc,omitempty"`
}

// RecipeEnv is the environment of the recipe bernie would run. The
// checklist shows its communication variables.
type RecipeEnv struct {
	ID  string
	Env map[string]string
}

// P2POptions are the inputs of P2PChecklist.
type P2POptions struct {
	// Cmdline is the kernel command line. It is read from root when empty.
	Cmdline string
	// PkgManager picks the package manager of the fix commands (dnf, apt...).
	PkgManager string
	// Recipe is the chosen recipe. It is informational.
	Recipe RecipeEnv
}

// P2PChecklist answers "will multi-GPU P2P and the custom all-reduce work on
// this host, and if not, why". The items are ordered: the peer links first,
// then the kernel settings, then the PCIe layout, then the optional tools and
// the recipe environment.
func P2PChecklist(root string, gpus []gpu.GPU, o P2POptions) ([]CheckItem, error) {
	root = canonRoot(root)
	if o.Cmdline == "" {
		o.Cmdline = readString(filepath.Join(root, "proc/cmdline"))
	}
	if len(gpus) == 0 {
		return []CheckItem{p2pItem("kfd_links", "KFD peer links", P2PSkip,
			"no GPUs: there is no multi-GPU peer copy to check")}, nil
	}
	t, err := Discover(root, gpus)
	if err != nil {
		return nil, err
	}
	return []CheckItem{
		kfdLinksItem(root, gpus, t),
		largeBARItem(root, gpus, o.Cmdline),
		iommuItem(o),
		acsItem(o, t),
		linkSpeedItem(root, gpus),
		linkClassItem(t),
		bandwidthToolItem(root),
		recipeEnvItem(o.Recipe),
	}, nil
}

func p2pItem(id, title string, status P2PStatus, detail string) CheckItem {
	return CheckItem{ID: id, Title: title, Status: status, Detail: detail}
}

// with sets the fix command and the documentation link of an item.
func (c CheckItem) with(fix, doc string) CheckItem {
	c.Fix, c.Doc = fix, doc
	return c
}

// commEnv are the recipe variables that steer peer copies and the custom
// all-reduce.
var commEnv = map[string]bool{
	"HSA_FORCE_FINE_GRAIN_PCIE":    true,
	"VLLM_FORCE_CUSTOM_ALL_REDUCE": true,
	"VLLM_RDNA_AR":                 true,
	"VLLM_RDNA_AR_MAX_KB":          true,
}

// cmdlineHas reports whether the kernel command line has the argument.
func cmdlineHas(cmdline, arg string) bool {
	for _, f := range strings.Fields(cmdline) {
		if f == arg {
			return true
		}
	}
	return false
}

// cmdlinePrefix reports whether a kernel command line argument starts with
// the prefix.
func cmdlinePrefix(cmdline, prefix string) bool {
	for _, f := range strings.Fields(cmdline) {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// hasPCIOpt reports whether a pci= kernel parameter carries the option.
func hasPCIOpt(cmdline, opt string) bool {
	for _, f := range strings.Fields(cmdline) {
		if !strings.HasPrefix(f, "pci=") {
			continue
		}
		for _, o := range strings.Split(strings.TrimPrefix(f, "pci="), ",") {
			if o == opt {
				return true
			}
		}
	}
	return false
}

// readIntFile reads an integer file, or 0.
func readIntFile(path string) int {
	v, err := strconv.Atoi(readString(path))
	if err != nil {
		return 0
	}
	return v
}

// sizeText renders a byte count with a unit.
func sizeText(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MiB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KiB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// linkText renders a PCIe link as "gen4 x16".
func linkText(l gpu.Link) string {
	return fmt.Sprintf("gen%d x%d", l.Gen, l.Width)
}
