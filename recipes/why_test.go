package recipes_test

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

func TestBuiltinsExplainImportantSettings(t *testing.T) {
	want := map[string][]string{
		"flashnext-mtp2": {
			"env.GPU_MAX_HW_QUEUES", "env.VLLM_TUNED_CONFIG_FOLDER",
			"args.attention-backend", "args.max-num-seqs", "args.compilation-config",
			"args.kv-cache-memory-bytes", "tuning.tunableop_profile",
		},
		"flashnext-mtp0": {
			"env.GPU_MAX_HW_QUEUES", "env.VLLM_TUNED_CONFIG_FOLDER",
			"args.attention-backend", "args.max-num-seqs", "args.compilation-config",
			"args.kv-cache-memory-bytes", "tuning.tunableop_profile",
		},
		"flashnext": {
			"env.GPU_MAX_HW_QUEUES", "env.VLLM_TUNED_CONFIG_FOLDER",
			"args.attention-backend", "args.max-num-seqs", "args.kv-cache-memory-bytes",
			"tuning.tunableop_profile",
		},
		"flashnext-pp3": {
			"env.GPU_MAX_HW_QUEUES", "env.VLLM_PP_LAYER_PARTITION",
			"args.pipeline-parallel-size", "args.max-num-seqs", "tuning.tunableop_profile",
		},
		"flashnext-pp3-mtp2": {
			"env.VLLM_PP_LAYER_PARTITION", "args.max-num-seqs",
			"args.compilation-config", "tuning.tunableop_profile",
		},
		"flashnext-dflash2": {
			"env.VLLM_RDNA_QSA_HIP", "env.GPU_MAX_HW_QUEUES",
			"args.speculative-config", "args.compilation-config", "tuning.tunableop_profile",
		},
		"flashnext-exl3": {
			"env.VLLM_USE_AOT_COMPILE", "env.GPU_MAX_HW_QUEUES",
			"args.max-num-seqs", "tuning.tunableop_profile",
		},
		"27b-awq": {
			"env.VLLM_RDNA2_W4A8_SDOT4", "args.attention-backend",
			"args.max-num-seqs", "args.compilation-config", "args.kv-cache-memory-bytes",
			"tuning.tunableop_profile",
		},
		"27b-exl3": {
			"env.NCCL_P2P_LEVEL", "env.VLLM_ROCM_USE_AITER", "args.attention-backend",
			"args.max-num-seqs", "args.compilation-config", "args.kv-cache-memory-bytes",
			"tuning.tunableop_profile",
		},
		"full": {
			"env.NCCL_P2P_LEVEL", "env.VLLM_USE_RDNA2_FA", "args.max-num-seqs",
			"args.compilation-config", "args.kv-cache-memory-bytes", "tuning.tunableop_profile",
		},
		"dsv4-flash": {
			"env.NCCL_P2P_LEVEL", "env.VLLM_USE_BREAKABLE_CUDAGRAPH",
			"env.VLLM_TUNED_CONFIG_FOLDER", "args.max-num-seqs",
			"args.compilation-config", "args.kv-cache-memory-bytes", "tuning.tunableop_profile",
		},
	}
	s := recipe.Load()
	for id, keys := range want {
		r, err := s.Resolve(id)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		for _, key := range keys {
			if r.Why[key] == "" {
				t.Errorf("%s: missing reason for %s", id, key)
			}
		}
	}
}
