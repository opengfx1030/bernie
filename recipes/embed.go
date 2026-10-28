// Package recipes embeds the built-in bernie recipes. A file is one recipe;
// its stem is the recipe id, which is unique across the whole tree. The tree
// is grouped by model family (flashnext/, qwen-dense/, deepseek/) and shared
// fragments by concern (_fragments/<concern>/). Files whose id starts with
// "_" are fragments: they are only used through `extends`. The loader walks
// the tree, so a new family folder needs no change here. See docs/recipes.md.
package recipes

import "embed"

// FS holds the built-in recipe files (<dir>/*.yaml and <dir>/<dir>/*.yaml).
//
//go:embed */*.yaml */*/*.yaml
var FS embed.FS
