package app

import (
	"os"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// RecipeList is the `recipe list --json` document: every loaded recipe with
// its fit to this host. all includes the recipes that do not fit. Only the
// GPUs the environment leaves visible count for the fit (spec §7.7).
func RecipeList(all bool) (recipe.Listing, error) {
	vis, err := gpu.Visible(root)
	if gpu.Restricted(err) {
		return recipe.Listing{}, err // the restriction names a GPU this host does not have
	}
	return recipe.List(recipe.Load(), vis.GPUs, hostRAM(), all), nil
}

// Restriction returns the one line that says the environment limits the
// GPUs bernie uses (spec §7.7). It is empty when nothing is restricted.
func Restriction() string {
	r, _ := gpu.EnvRestriction()
	return r.Note()
}

func hostRAM() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return host.ParseMemTotal(string(b))
}
