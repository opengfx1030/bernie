// Package tunableop embeds the TunableOp profiles, one folder per GPU
// architecture and rocBLAS build: tunableop/<arch>/<profile>/. A profile
// carries its profile.json manifest, the solver rows of every rank and a
// README.md and provenance.json. The rows come from the
// opengfx1030/vllm-rdna fork (tunableop/). internal/serving/tuning discovers the
// profiles by walking this tree, so a new profile needs no change here.
package tunableop

import "embed"

// FS holds README.md and <arch>/<profile>/{profile.json,tunableop_results<rank>.csv,README.md,provenance.json}.
//
//go:embed README.md */*/*
var FS embed.FS
