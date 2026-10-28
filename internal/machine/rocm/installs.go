package rocm

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/support/factcache"
)

// installsKey is the cache key of the ROCm trees of a root.
func installsKey(root string) string { return "rocm/installs/" + root }

// installsFingerprint names the ROCm trees of a root: the times of the
// directories that hold them.
func installsFingerprint(root string) string {
	var out []string
	for _, pat := range []string{"opt/rocm*", "opt/rocm*/core-*"} {
		m, _ := filepath.Glob(filepath.Join(root, pat))
		for _, d := range m {
			out = append(out, stamp(d))
		}
	}
	return strings.Join(out, ",")
}

// FindInstalls lists ROCm trees: /opt/rocm*, /opt/rocm*/core-* (TheRock
// layout) and a distro install in /usr. Trees reached through several
// symlinks are reported once, by their resolved path. A tree inside another
// ROCm tree is nested under it, not a second install.
//
// On the live host the list is cached under the state directory: the trees
// change when ROCm is installed or removed, and the walk of a big tree
// costs time.
func FindInstalls(root string) []Install {
	if root != "/" {
		return findInstalls(root)
	}
	installs, _, _, err := factcache.Get(context.Background(), installsKey(root), installsFingerprint(root), InstallsTTL,
		func(context.Context) ([]Install, error) { return findInstalls(root), nil })
	if err != nil {
		return findInstalls(root)
	}
	return installs
}

// findInstalls lists the ROCm trees under root.
func findInstalls(root string) []Install {
	var cands []string
	for _, pat := range []string{"opt/rocm*", "opt/rocm*/core-*"} {
		m, _ := filepath.Glob(filepath.Join(root, pat))
		cands = append(cands, m...)
	}
	trees := map[string]*Install{}
	var order []string
	for _, c := range cands {
		rel := "/" + strings.TrimPrefix(filepath.ToSlash(strings.TrimPrefix(c, filepath.Clean(root))), "/")
		real := resolveIn(root, rel)
		dir := filepath.Join(root, real)
		if _, seen := trees[real]; seen || !isDir(dir) || !isROCmTree(dir) {
			continue
		}
		kind := "opt"
		if strings.HasPrefix(filepath.Base(real), "core-") {
			kind = "therock"
		}
		trees[real] = &Install{Root: real, Version: treeVersion(dir), Kind: kind}
		order = append(order, real)
	}
	children := map[string][]*Install{}
	var roots []*Install
	for _, real := range order {
		t := trees[real]
		parent := ""
		for _, other := range order {
			if other != real && pathUnder(real, other) && (parent == "" || len(other) > len(parent)) {
				parent = other
			}
		}
		if parent == "" {
			roots = append(roots, t)
		} else {
			children[parent] = append(children[parent], t)
		}
	}
	var attach func(*Install)
	attach = func(t *Install) {
		sort.Slice(children[t.Root], func(i, j int) bool { return children[t.Root][i].Root < children[t.Root][j].Root })
		for _, ch := range children[t.Root] {
			attach(ch)
			t.Nested = append(t.Nested, *ch)
		}
	}
	for _, t := range roots {
		attach(t)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Root < roots[j].Root })
	out := make([]Install, 0, len(roots))
	for _, t := range roots {
		out = append(out, *t)
	}
	if v, ok := distroInstall(root); ok {
		out = append(out, Install{Root: "/usr", Version: v, Kind: "distro"})
	}
	return out
}

func isROCmTree(dir string) bool {
	for _, marker := range []string{".info/version", "bin/rocminfo", "bin/hipcc", "lib/libamdhip64.so"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// installTool finds a tool in the install bin dirs (newest version first).
func installTool(root string, installs []Install, name string) string {
	sorted := installTrees(installs)
	sort.SliceStable(sorted, func(i, j int) bool { return CompareVersions(sorted[i].Version, sorted[j].Version) > 0 })
	for _, in := range sorted {
		p := filepath.Join(root, in.Root, "bin", name)
		if isFile(p) {
			return p
		}
	}
	return ""
}

// installTrees flattens nested install layouts for tool lookup.
func installTrees(installs []Install) []Install {
	var out []Install
	for _, in := range installs {
		out = append(out, in)
		out = append(out, installTrees(in.Nested)...)
	}
	return out
}

// pathUnder reports whether p is strictly inside prefix.
func pathUnder(p, prefix string) bool {
	return p != prefix && strings.HasPrefix(p, strings.TrimSuffix(prefix, "/")+"/")
}

// ResolvePath resolves symlinks in an installed tool path.
func ResolvePath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// InstallForPath returns the most specific install that contains p.
func InstallForPath(installs []Install, p string) (Install, bool) {
	p = ResolvePath(p)
	best := ""
	var found Install
	for _, in := range installTrees(installs) {
		if pathUnder(p, in.Root) && len(in.Root) > len(best) {
			best, found = in.Root, in
		}
	}
	return found, best != ""
}
