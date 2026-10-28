package app

import (
	"os"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// RecipeList is the `recipe list --json` document: every loaded recipe with
// its fit to this host. all includes the recipes that do not fit.
func RecipeList(all bool) recipe.Listing {
	gpus, _ := gpu.List(root) // no GPU data gives fit "unknown"
	return recipe.List(recipe.Load(), gpus, hostRAM(), all)
}

func hostRAM() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return host.ParseMemTotal(string(b))
}
