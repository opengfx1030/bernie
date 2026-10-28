// Package tunableop embeds the TunableOp profile registry (profiles.json) and
// the solver rows of every registered profile (one directory per rocBLAS
// build). The rows come from the opengfx1030/vllm-rdna fork (tunableop/),
// together with each profile's README.md and provenance.json.
package tunableop

import "embed"

// FS holds profiles.json, README.md and <profile-dir>/{tunableop_results<rank>.csv,README.md,provenance.json}.
//
//go:embed profiles.json README.md */*
var FS embed.FS
