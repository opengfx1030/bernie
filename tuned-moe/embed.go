// SPDX-License-Identifier: Apache-2.0
// Copied from opengfx1030/vllm-rdna (rdna_extras) tuned-moe/.

// Package tunedmoe embeds the tuned fused-MoE kernel configs
// (VLLM_TUNED_CONFIG_FOLDER), one folder per GPU architecture:
// tuned-moe/<arch>/E=...json. vLLM reads a flat folder, so the per-arch
// folder is the unit. Host mode materializes the folder of the selected
// GPUs' architecture into <data>/tuned-moe/<arch>; the vllm-rdna image has
// the gfx1030 files at /src/vllm/tuned-moe. See README.md to add an
// architecture.
package tunedmoe

import "embed"

// FS holds <arch>/*.json.
//
//go:embed */*.json
var FS embed.FS
