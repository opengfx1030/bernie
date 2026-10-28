package logdoc

import (
	"regexp"
	"strings"
	"testing"
)

// The tails below are excerpts of real vLLM 0.31 serve logs from the build
// box (par1-cs25, 8x Radeon PRO V620, ROCm 7.14). Lines longer than 200
// characters are cut.
const weightsTail = `(Worker_TP0 pid=32316) WARNING 09-29 02:12:21 [compilation.py:1369] Op 'sparse_attn_indexer' doesn't exist (or wasn't imported/registered), enabling with '+sparse_attn_indexer' has no effect
(Worker_TP0 pid=32316) INFO 09-29 02:12:21 [weight_utils.py:858] Filesystem type for checkpoints: XFS. Checkpoint size: 19.57 GiB. Available RAM: 483.18 GiB.
(Worker_TP0 pid=32316) INFO 09-29 02:12:21 [weight_utils.py:881] Auto-prefetch is disabled because the filesystem (XFS) is not a recognized network FS (NFS/Lustre). If you want to force prefetching, s
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:   0% Completed | 0/5 [00:00<?, ?it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  20% Completed | 1/5 [00:00<00:01,  2.80it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  40% Completed | 2/5 [00:01<00:01,  1.82it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  60% Completed | 3/5 [00:01<00:01,  1.48it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  80% Completed | 4/5 [00:02<00:00,  1.32it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:03<00:00,  1.57it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:03<00:00,  1.58it/s]
(Worker_TP0 pid=32316)
(Worker_TP0 pid=32316) INFO 09-29 02:12:24 [default_loader.py:430] Loading weights took 3.23 seconds
(Worker_TP0 pid=32316) INFO 09-29 02:12:25 [weight_utils.py:858] Filesystem type for checkpoints: XFS. Checkpoint size: 19.57 GiB. Available RAM: 480.28 GiB.
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:   0% Completed | 0/5 [00:00<?, ?it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  20% Completed | 1/5 [00:00<00:01,  3.53it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards:  40% Completed | 2/5 [00:00<00:00,  3.65it/s]
(Worker_TP2 pid=32318) INFO 09-29 02:12:25 [speculator.py:321] Using local argmax reduction for draft token generation (communication: O(2*tp_size) vs O(vocab_size)).
(Worker_TP3 pid=32319) INFO 09-29 02:12:25 [speculator.py:321] Using local argmax reduction for draft token generation (communication: O(2*tp_size) vs O(vocab_size)).
(Worker_TP1 pid=32317) INFO 09-29 02:12:25 [speculator.py:321] Using local argmax reduction for draft token generation (communication: O(2*tp_size) vs O(vocab_size)).
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:00<00:00,  7.13it/s]
(Worker_TP0 pid=32316) Loading safetensors checkpoint shards: 100% Completed | 5/5 [00:00<00:00,  6.16it/s]
`

const compileTail = `(Worker_TP0 pid=32316)
(Worker_TP0 pid=32316) INFO 09-29 02:12:25 [default_loader.py:430] Loading weights took 0.83 seconds
(Worker_TP0 pid=32316) INFO 09-29 02:12:25 [speculator.py:321] Using local argmax reduction for draft token generation (communication: O(2*tp_size) vs O(vocab_size)).
(Worker_TP3 pid=32319) INFO 09-29 02:12:26 [model_runner.py:460] Model loading took 5.8 GiB memory and 5.947072 seconds
(Worker_TP2 pid=32318) INFO 09-29 02:12:26 [model_runner.py:460] Model loading took 5.8 GiB memory and 5.950274 seconds
(Worker_TP1 pid=32317) INFO 09-29 02:12:26 [model_runner.py:460] Model loading took 5.8 GiB memory and 5.948866 seconds
(Worker_TP3 pid=32319) INFO 09-29 02:12:26 [interface.py:935] Padding mamba page size by 29.13% to ensure that mamba page size and attention page size are exactly equal.
(Worker_TP1 pid=32317) INFO 09-29 02:12:26 [interface.py:935] Padding mamba page size by 29.13% to ensure that mamba page size and attention page size are exactly equal.
(Worker_TP2 pid=32318) INFO 09-29 02:12:26 [interface.py:935] Padding mamba page size by 29.13% to ensure that mamba page size and attention page size are exactly equal.
(Worker_TP0 pid=32316) INFO 09-29 02:12:26 [model_runner.py:460] Model loading took 5.8 GiB memory and 6.014470 seconds
(Worker_TP0 pid=32316) INFO 09-29 02:12:26 [interface.py:935] Padding mamba page size by 29.13% to ensure that mamba page size and attention page size are exactly equal.
(EngineCore pid=32278) WARNING 09-29 02:12:26 [torch_utils.py:265] OMP_NUM_THREADS=4 is set; leaving Torch threads at 4 for serving. Multi-threaded torch CPU ops during serving can degrade performance
(EngineCore pid=32278) INFO 09-29 02:12:26 [utils.py:304] Using LBHNC KV cache layout.
(Worker_TP1 pid=32317) INFO 09-29 02:12:27 [caching.py:343] reconstructed serializable fn from standalone compile artifacts. num_artifacts=8 num_submods=146
(Worker_TP2 pid=32318) INFO 09-29 02:12:27 [caching.py:343] reconstructed serializable fn from standalone compile artifacts. num_artifacts=8 num_submods=146
(Worker_TP3 pid=32319) INFO 09-29 02:12:27 [caching.py:343] reconstructed serializable fn from standalone compile artifacts. num_artifacts=8 num_submods=146
(Worker_TP1 pid=32317) WARNING 09-29 02:12:27 [decorators.py:323] Compiling model again due to a load failure from /home/chenco_adm/vllm-rdna-0.28.0/cache/vllm/torch_compile_cache/torch_aot_compile/cf
(Worker_TP2 pid=32318) WARNING 09-29 02:12:27 [decorators.py:323] Compiling model again due to a load failure from /home/chenco_adm/vllm-rdna-0.28.0/cache/vllm/torch_compile_cache/torch_aot_compile/cf
(Worker_TP3 pid=32319) WARNING 09-29 02:12:27 [decorators.py:323] Compiling model again due to a load failure from /home/chenco_adm/vllm-rdna-0.28.0/cache/vllm/torch_compile_cache/torch_aot_compile/cf
(Worker_TP0 pid=32316) INFO 09-29 02:12:27 [caching.py:343] reconstructed serializable fn from standalone compile artifacts. num_artifacts=8 num_submods=146
(Worker_TP0 pid=32316) WARNING 09-29 02:12:27 [decorators.py:323] Compiling model again due to a load failure from /home/chenco_adm/vllm-rdna-0.28.0/cache/vllm/torch_compile_cache/torch_aot_compile/cf
(Worker_TP0 pid=32316) INFO 09-29 02:12:41 [backends.py:1180] Using cache directory: /home/chenco_adm/vllm-rdna-0.28.0/cache/vllm/torch_compile_cache/19f51a9da7/rank_0_0/backbone for vLLM's torch.comp
(Worker_TP0 pid=32316) INFO 09-29 02:12:41 [backends.py:1241] Dynamo bytecode transform time: 13.46 s
(Worker_TP0 pid=32316) INFO 09-29 02:12:46 [backends.py:441] Compiling a graph for compile range (1, 2048) takes 3.77 s
(Worker_TP2 pid=32318) INFO 09-29 02:12:59 [backends.py:1006] collected artifacts: 146 entries, 7 artifacts, 2297129 bytes total
[rank2]:[W929 02:12:59.938956626 w4a8_sdot4_rdna2.hip:356] Warning: RDNA2 W4A8 sdot4 path active (config a8_lds_k32_ag) (function operator())
(Worker_TP1 pid=32317) INFO 09-29 02:12:59 [backends.py:1006] collected artifacts: 146 entries, 7 artifacts, 2371915 bytes total
[rank1]:[W929 02:12:59.034571997 w4a8_sdot4_rdna2.hip:356] Warning: RDNA2 W4A8 sdot4 path active (config a8_lds_k32_ag) (function operator())
(Worker_TP3 pid=32319) INFO 09-29 02:12:59 [backends.py:1006] collected artifacts: 146 entries, 7 artifacts, 2373521 bytes total
[rank3]:[W929 02:12:59.256792209 w4a8_sdot4_rdna2.hip:356] Warning: RDNA2 W4A8 sdot4 path active (config a8_lds_k32_ag) (function operator())
(Worker_TP0 pid=32316) INFO 09-29 02:12:59 [backends.py:1006] collected artifacts: 146 entries, 7 artifacts, 2637471 bytes total
`

const captureTail = `(Worker_TP2 pid=32318) INFO 09-29 02:13:04 [gpu_worker.py:638] Initial free memory 29.5 GiB, reserved 7.45 GiB memory for KV Cache as specified by kv_cache_memory_bytes config and skipped memory profi
(EngineCore pid=32278) WARNING 09-29 02:13:04 [kv_cache_utils.py:1249] Add 3 padding layers, may waste at most 6.25% KV cache memory
(EngineCore pid=32278) INFO 09-29 02:13:04 [kv_cache_utils.py:2310] GPU KV cache size: 438,210 tokens, Maximum concurrency for 262,144 tokens per request: 1.67x
(Worker_TP0 pid=32316) INFO 09-29 02:13:04 [kernel_warmup.py:124] JIT kernel warmup starting.
(Worker_TP0 pid=32316) INFO 09-29 02:13:04 [kernel_warmup.py:134] JIT kernel warmup finished in 0.00s.
(Worker_TP0 pid=32316) INFO 09-29 02:13:04 [qwen_triton_warmup.py:270] Warming up Qwen Triton kernels for model_type=qwen3_5_text.
(Worker_TP1 pid=32317) INFO 09-29 02:13:04 [kernel_warmup.py:124] JIT kernel warmup starting.
(Worker_TP1 pid=32317) INFO 09-29 02:13:04 [kernel_warmup.py:134] JIT kernel warmup finished in 0.00s.
(Worker_TP1 pid=32317) INFO 09-29 02:13:04 [qwen_triton_warmup.py:270] Warming up Qwen Triton kernels for model_type=qwen3_5_text.
(Worker_TP3 pid=32319) INFO 09-29 02:13:04 [kernel_warmup.py:124] JIT kernel warmup starting.
(Worker_TP2 pid=32318) INFO 09-29 02:13:04 [kernel_warmup.py:124] JIT kernel warmup starting.
(Worker_TP3 pid=32319) INFO 09-29 02:13:04 [kernel_warmup.py:134] JIT kernel warmup finished in 0.00s.
(Worker_TP2 pid=32318) INFO 09-29 02:13:04 [kernel_warmup.py:134] JIT kernel warmup finished in 0.00s.
(Worker_TP2 pid=32318) INFO 09-29 02:13:04 [qwen_triton_warmup.py:270] Warming up Qwen Triton kernels for model_type=qwen3_5_text.
(Worker_TP1 pid=32317) INFO 09-29 02:13:04 [ll_bf16.py:30] cuteDSL (CUTLASS Python) not available, ll_bf16_gemm disabled
(Worker_TP0 pid=32316) INFO 09-29 02:13:04 [ll_bf16.py:30] cuteDSL (CUTLASS Python) not available, ll_bf16_gemm disabled
(Worker_TP3 pid=32319) INFO 09-29 02:13:04 [ll_bf16.py:30] cuteDSL (CUTLASS Python) not available, ll_bf16_gemm disabled
(Worker_TP2 pid=32318) INFO 09-29 02:13:04 [ll_bf16.py:30] cuteDSL (CUTLASS Python) not available, ll_bf16_gemm disabled
(Worker_TP0 pid=32316) Capturing CUDA graphs (PIECEWISE):   0%|          | 0/4 [00:00<?, ?it/s]Capturing CUDA graphs (PIECEWISE):  25%|██▌       | 1/4 [00:00<00:02,  1.07it/s]Capturing CUDA graphs
(Worker_TP0 pid=32316) Capturing CUDA graphs (FULL):   0%|          | 0/4 [00:00<?, ?it/s](Worker_TP2 pid=32318) INFO 09-29 02:13:08 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDes
(Worker_TP1 pid=32317) INFO 09-29 02:13:08 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
(Worker_TP3 pid=32319) INFO 09-29 02:13:08 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
(Worker_TP0 pid=32316) INFO 09-29 02:13:08 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
Capturing CUDA graphs (FULL):  25%|██▌       | 1/4 [00:01<00:04,  1.35s/it](Worker_TP2 pid=32318) INFO 09-29 02:13:09 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode
(Worker_TP1 pid=32317) INFO 09-29 02:13:09 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=12, num_reqs=4, uniform_token_count=3,
(Worker_TP3 pid=32319) INFO 09-29 02:13:09 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=12, num_reqs=4, uniform_token_count=3,
(Worker_TP0 pid=32316) INFO 09-29 02:13:09 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=12, num_reqs=4, uniform_token_count=3,
Capturing CUDA graphs (FULL):  50%|█████     | 2/4 [00:02<00:02,  1.33s/it](Worker_TP3 pid=32319) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode
(Worker_TP1 pid=32317) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=6, num_reqs=2, uniform_token_count=3, m
(Worker_TP2 pid=32318) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=6, num_reqs=2, uniform_token_count=3, m
(Worker_TP0 pid=32316) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=6, num_reqs=2, uniform_token_count=3, m
Capturing CUDA graphs (FULL):  75%|███████▌ | 3/4 [00:02<00:00,  1.26it/s](Worker_TP2 pid=32318) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode
(Worker_TP2 pid=32318) INFO 09-29 02:13:10 [speculator.py:151] Capturing model for speculator...
(Worker_TP1 pid=32317) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=3, num_reqs=1, uniform_token_count=3, m
(Worker_TP1 pid=32317) INFO 09-29 02:13:10 [speculator.py:151] Capturing model for speculator...
(Worker_TP0 pid=32316) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=3, num_reqs=1, uniform_token_count=3, m
Capturing CUDA graphs (FULL): 100%|██████████| 4/4 [00:03<00:00,  1.82it/s]Capturing CUDA graphs (FULL): 100%|██████████| 4/4 [00:03<00:00,  1.33it/s]
(Worker_TP0 pid=32316) INFO 09-29 02:13:10 [speculator.py:151] Capturing model for speculator...
(Worker_TP3 pid=32319) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=3, num_reqs=1, uniform_token_count=3, m
(Worker_TP3 pid=32319) INFO 09-29 02:13:10 [speculator.py:151] Capturing model for speculator...
(Worker_TP0 pid=32316) Capturing prefill CUDA graphs (PIECEWISE):   0%|          | 0/4 [00:00<?, ?it/s]Capturing prefill CUDA graphs (PIECEWISE): 100%|██████████| 4/4 [00:00<00:00, 55.79it/s]
(Worker_TP0 pid=32316) Capturing prefill CUDA graphs (FULL):   0%|          | 0/4 [00:00<?, ?it/s](Worker_TP1 pid=32317) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExec
(Worker_TP2 pid=32318) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
(Worker_TP3 pid=32319) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
(Worker_TP0 pid=32316) INFO 09-29 02:13:10 [cudagraph_utils.py:417] Captured FULL cudagraph BatchExecutionDescriptor(cg_mode=<CUDAGraphMode.FULL: 2>, num_tokens=24, num_reqs=8, uniform_token_count=3,
`

const readyTail = `(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/responses, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/responses/{response_id}, Methods: GET
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/responses/{response_id}/cancel, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/completions, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/messages, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/messages/count_tokens, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /generative_scoring, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /scale_elastic_ep, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /is_scaling_elastic_ep, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/chat/completions/render, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/completions/render, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/chat/completions/derender, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /v1/completions/derender, Methods: POST
(APIServer pid=32147) INFO 09-29 02:13:15 [launcher.py:70] Route: /inference/v1/generate, Methods: POST
(APIServer pid=32147) INFO:     Started server process [32147]
(APIServer pid=32147) INFO:     Waiting for application startup.
(APIServer pid=32147) INFO:     Application startup complete.
(APIServer pid=32147) INFO:     127.0.0.1:47074 - "GET /v1/models HTTP/1.1" 200 OK
`

const failedTail = `(APIServer pid=170347) INFO 09-29 19:02:34 [launcher.py:70] Route: /v1/completions/derender, Methods: POST
(APIServer pid=170347) INFO 09-29 19:02:34 [launcher.py:70] Route: /inference/v1/generate, Methods: POST
(APIServer pid=170347) INFO:     Started server process [170347]
(APIServer pid=170347) INFO:     Waiting for application startup.
(APIServer pid=170347) INFO:     Application startup complete.
(APIServer pid=170347) INFO:     127.0.0.1:42242 - "GET /v1/models HTTP/1.1" 200 OK
(APIServer pid=170347) INFO:     127.0.0.1:57790 - "POST /tokenize HTTP/1.1" 404 Not Found
(APIServer pid=170347) INFO:     127.0.0.1:57794 - "GET /metrics HTTP/1.1" 200 OK
(APIServer pid=170347) INFO:     127.0.0.1:57794 - "GET /metrics HTTP/1.1" 200 OK
(APIServer pid=170347) INFO:     127.0.0.1:57794 - "POST /v1/completions HTTP/1.1" 200 OK
!!!!!!! Segfault encountered !!!!!!!

!!!!!!! Segfault encountered !!!!!!!
!!!!!!! Segfault encountered !!!!!!!

!!!!!!! Segfault encountered !!!!!!!


(PleOffloadWorker pid=172365) INFO 09-29 19:03:51 [worker.py:269] Parent exited, shutting down.
(EngineCore pid=171333) ERROR 09-29 19:04:04 [multiproc_executor.py:314] Worker proc VllmWorker-2 died unexpectedly (exit code: -11), shutting down executor.
(EngineCore pid=171333) INFO 09-29 19:04:04 [multiproc_executor.py:472] [shutdown] Executor: waiting for worker exit count=0
(EngineCore pid=171333) INFO 09-29 19:04:04 [multiproc_executor.py:479] [shutdown] Executor: all workers exited gracefully
(EngineCore pid=171333) ERROR 09-29 19:04:04 [dump_input.py:72] Dumping input data for V1 LLM engine (v0.20.1.dev99) with config: model='/home/chenco_adm/hfcache/hub/models--wtdcode--Qwen3.8-Flash-Nex
(EngineCore pid=171333) ERROR 09-29 19:04:04 [dump_input.py:79] Dumping scheduler output for model execution: SchedulerOutput(scheduled_new_reqs=[NewRequestData(req_id=cmpl-bench-ab0cf9db-0-0-9c2e58f4
(EngineCore pid=171333) ERROR 09-29 19:04:04 [dump_input.py:81] Dumping scheduler stats: SchedulerStats(num_running_reqs=1, num_waiting_reqs=0, num_skipped_waiting_reqs=0, step_counter=0, current_wave
(EngineCore pid=171333) ERROR 09-29 19:04:04 [core.py:1385] EngineCore encountered a fatal error.
(EngineCore pid=171333) ERROR 09-29 19:04:04 [core.py:1385] Traceback (most recent call last):
(EngineCore pid=171333) ERROR 09-29 19:04:04 [core.py:1385]   File "/home/chenco_adm/vllm-rdna-0.28.0/vllm/v1/engine/core.py", line 1369, in run_engine_core
(EngineCore pid=171333) ERROR 09-29 19:04:04 [core.py:1385]     engine_core.run_busy_loop()
`

// A tail of a running server: only request lines, nothing the catalogue knows.
const servingTail = `(APIServer pid=32147) INFO:     127.0.0.1:34460 - "POST /v1/completions HTTP/1.1" 200 OK
(APIServer pid=32147) INFO:     127.0.0.1:34466 - "POST /v1/completions HTTP/1.1" 200 OK
(APIServer pid=32147) INFO:     127.0.0.1:34470 - "POST /v1/completions HTTP/1.1" 200 OK
(APIServer pid=32147) INFO 09-29 02:19:46 [loggers.py:310] Engine 000: Avg prompt throughput: 409.6 tokens/s, Avg generation throughput: 0.8 tokens/s, Running: 7 reqs
(APIServer pid=32147) INFO 09-29 02:19:56 [loggers.py:310] Engine 000: Avg prompt throughput: 409.5 tokens/s, Avg generation throughput: 95.8 tokens/s, Running: 8 reqs
(APIServer pid=32147) INFO:     127.0.0.1:34430 - "GET /metrics HTTP/1.1" 200 OK
`

func TestPhaseCatalogueIsValid(t *testing.T) {
	rules, err := LoadPhases()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 5 {
		t.Fatalf("catalogue has %d phases, want at least 5", len(rules))
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate phase id %s", r.ID)
		}
		seen[r.ID] = true
		if strings.TrimSpace(r.Sentence) == "" {
			t.Errorf("%s: empty sentence", r.ID)
		}
		if _, err := regexp.Compile(r.Match); err != nil {
			t.Errorf("%s: match does not compile: %v", r.ID, err)
		}
		if r.Progress != "" {
			re, err := regexp.Compile(r.Progress)
			if err != nil {
				t.Errorf("%s: progress does not compile: %v", r.ID, err)
				continue
			}
			if re.NumSubexp() != 2 {
				t.Errorf("%s: progress has %d groups, want 2", r.ID, re.NumSubexp())
			}
		}
	}
}

func TestPhaseOfRealTails(t *testing.T) {
	cases := []struct {
		name  string
		tail  string
		id    string
		sent  string
		done  int
		total int
	}{
		{name: "weights", tail: weightsTail, id: "loading-weights",
			sent: "Loading the model weights, 5 of 5 shards", done: 5, total: 5},
		{name: "compiling", tail: compileTail, id: "compiling",
			sent: "Compiling the model (the first start is slow, later starts reuse the cache)"},
		{name: "capturing", tail: captureTail, id: "capturing-graphs",
			sent: "Capturing the graphs, 0 of 4", done: 0, total: 4},
		{name: "ready", tail: readyTail, id: "ready",
			sent: "The server is ready and answers requests"},
		{name: "failed", tail: failedTail, id: "failed",
			sent: "The server failed during startup"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := PhaseOf([]byte(c.tail))
			if !ok {
				t.Fatal("no phase")
			}
			if got.ID != c.id {
				t.Errorf("phase = %q, want %q", got.ID, c.id)
			}
			if got.Sentence != c.sent {
				t.Errorf("sentence = %q, want %q", got.Sentence, c.sent)
			}
			if got.Done != c.done || got.Total != c.total {
				t.Errorf("progress = %d/%d, want %d/%d", got.Done, got.Total, c.done, c.total)
			}
		})
	}
}

func TestPhaseOfUnknownTailHasNoSentence(t *testing.T) {
	for _, tail := range []string{"", servingTail, "hello\nworld\n"} {
		if p, ok := PhaseOf([]byte(tail)); ok {
			t.Errorf("tail %q gave phase %q", tail, p.ID)
		}
	}
}

// The phase is the last known step: a later line wins over an earlier one.
func TestPhaseOfTakesTheLastKnownLine(t *testing.T) {
	tail := weightsTail + compileTail + readyTail
	p, ok := PhaseOf([]byte(tail))
	if !ok || p.ID != "ready" {
		t.Fatalf("phase = %+v (ok=%v), want ready", p, ok)
	}
}

// A healthy startup ends in the ready phase.
func TestPhaseOfHealthyStartup(t *testing.T) {
	p, ok := PhaseOf([]byte(healthyLog))
	if !ok || p.ID != "ready" {
		t.Fatalf("phase = %+v (ok=%v), want ready", p, ok)
	}
}

// The progress of a progress bar is the last update on the line.
func TestPhaseProgressTakesTheLastUpdate(t *testing.T) {
	line := "(Worker_TP0 pid=32316) Capturing CUDA graphs (FULL): 100%|██████████| 4/4 [00:03<00:00,  1.82it/s]Capturing CUDA graphs (FULL): 100%|██████████| 4/4 [00:03<00:00,  1.33it/s]"
	p, ok := PhaseOf([]byte(line))
	if !ok {
		t.Fatal("no phase")
	}
	if p.Done != 4 || p.Total != 4 {
		t.Errorf("progress = %d/%d, want 4/4", p.Done, p.Total)
	}
}
