// Package bench embeds the built-in benchmark suites. A file is one suite;
// its stem is the suite id, which is unique across the whole tree. The loader
// walks the tree, so a new suite needs no change here. See docs/spec.md §10.
package bench

import "embed"

// FS holds the built-in suite files.
//
//go:embed suites/*.yaml
var FS embed.FS
