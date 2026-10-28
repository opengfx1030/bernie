// SPDX-License-Identifier: Apache-2.0
// Copied from opengfx1030/vllm-rdna (rdna_extras) tuned-moe/.

// Package tunedmoe embeds the fork's tuned fused-MoE kernel configs
// (VLLM_TUNED_CONFIG_FOLDER). Host mode materializes them into
// <data>/tuned-moe; the vllm-rdna image has the same files at
// /src/vllm/tuned-moe.
package tunedmoe

import "embed"

// FS holds the *.json configs.
//
//go:embed *.json
var FS embed.FS
