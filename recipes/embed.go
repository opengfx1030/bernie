// Package recipes embeds the built-in rdnactl recipes. A file is one recipe;
// its stem is the recipe id. Files whose id starts with "_" are fragments:
// they are only used through `extends`. See docs/recipes.md.
package recipes

import "embed"

// FS holds the built-in recipe files (*.yaml).
//
//go:embed *.yaml
var FS embed.FS
