package gpu

import (
	"sort"
	"strings"
)

// ArchMatch reports whether gfx (for example "gfx1030") is named by any of
// patterns. A pattern is a full gfx name or a prefix: "gfx103" matches
// gfx1030 and gfx1031, "gfx11" matches gfx1100. An empty gfx matches nothing.
func ArchMatch(patterns []string, gfx string) bool {
	if gfx == "" {
		return false
	}
	for _, p := range patterns {
		if p != "" && strings.HasPrefix(gfx, p) {
			return true
		}
	}
	return false
}

// Archs returns the distinct gfx names of the GPUs whose HIP index is in
// idx, sorted. An index with no GPU is skipped.
func Archs(gpus []GPU, idx []int) []string {
	want := make(map[int]bool, len(idx))
	for _, i := range idx {
		want[i] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range gpus {
		if want[g.HIPIndex] && g.GFX != "" && !seen[g.GFX] {
			seen[g.GFX] = true
			out = append(out, g.GFX)
		}
	}
	sort.Strings(out)
	return out
}
