package logdoc

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestCatalogIsValid(t *testing.T) {
	rules, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 40 {
		t.Fatalf("catalog has %d rules, want at least 40", len(rules))
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate id %s", r.ID)
		}
		seen[r.ID] = true
		for name, v := range map[string]string{"id": r.ID, "pattern": r.Pattern, "cause": r.Cause, "fix": r.Fix, "doc": r.Doc} {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: empty %s", r.ID, name)
			}
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			t.Errorf("%s: pattern does not compile: %v", r.ID, err)
		}
		if !strings.HasPrefix(r.Doc, "https://") {
			t.Errorf("%s: doc %q is not an https link", r.ID, r.Doc)
		}
		if r.Severity != SeverityError && r.Severity != SeverityWarn {
			t.Errorf("%s: severity %q", r.ID, r.Severity)
		}
		if !strings.Contains(r.ID, ".") {
			t.Errorf("%s: id is not <family>.<slug>", r.ID)
		}
	}
}

func TestValidateRejectsBadRules(t *testing.T) {
	good := Rule{ID: "a.b", Severity: SeverityError, Pattern: "x", Cause: "c", Fix: "f", Doc: "https://d"}
	cases := []struct {
		name  string
		rules []Rule
		want  string
	}{
		{"duplicate id", []Rule{good, good}, "duplicate id"},
		{"empty fix", []Rule{{ID: "a.b", Severity: SeverityWarn, Pattern: "x", Cause: "c", Doc: "d"}}, "empty fix"},
		{"bad regexp", []Rule{{ID: "a.b", Severity: SeverityWarn, Pattern: "(", Cause: "c", Fix: "f", Doc: "d"}}, "pattern:"},
		{"bad severity", []Rule{{ID: "a.b", Severity: "fatal", Pattern: "x", Cause: "c", Fix: "f", Doc: "d"}}, "severity"},
	}
	for _, c := range cases {
		err := Validate(c.rules)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.want)
		}
	}
	if err := Validate([]Rule{good}); err != nil {
		t.Errorf("good rule rejected: %v", err)
	}
}

// samples holds one log excerpt per rule. Most lines are copied from real
// vLLM 0.31 / ROCm 7.14 serve logs; the others follow the upstream message
// text. also lists other rules the same excerpt legitimately triggers.
var samples = []struct {
	id      string
	log     string
	also    []string
	comment string
}{
	{id: "gpu.no-gpu", log: "RuntimeError: ROCm platform is not available because no GPU is found."},
	{id: "gpu.infer-device-type", log: "RuntimeError: Failed to infer device type."},
	{id: "gpu.amdsmi-not-init", log: "amdsmi.amdsmi_exception.AmdSmiLibraryException: AMDSMI_STATUS_NOT_INIT - Device not initialized"},
	{id: "gpu.kfd-access", log: "Failed to open /dev/kfd: Permission denied"},
	{id: "gpu.torch-not-rocm", log: "AssertionError: Torch not compiled with CUDA enabled"},
	{id: "gpu.invalid-ordinal", log: "RuntimeError: HIP error: invalid device ordinal"},
	{id: "gpu.world-size", log: "ValueError: World size (4) is larger than the number of available GPUs (2) in this node."},
	{id: "gpu.old-card-mmio", log: "Failed to map remapped mmio page on gpu_mem 0"},
	{id: "kernels.no-kernel-image", log: "torch.AcceleratorError: HIP error: no kernel image is available for execution on the device"},
	{id: "kernels.no-binary-for-gpu", log: "RuntimeError: HIP error: hipErrorNoBinaryForGpu: Unable to find code object for all current devices!"},
	{id: "kernels.invalid-image", log: "RuntimeError: HIP error: hipErrorInvalidImage"},
	{id: "kernels.invalid-isa", log: "HSA_STATUS_ERROR_INVALID_ISA: The instruction set architecture is invalid."},
	{id: "kernels.tensile-library-missing", log: "rocBLAS error: Cannot read /opt/rocm/lib/rocblas/library/TensileLibrary.dat: No such file or directory for GPU arch : gfx1030"},
	{id: "kernels.fork-ops-missing", log: "ModuleNotFoundError: No module named 'vllm._rocm_C'"},
	{id: "kernels.abi-mismatch", log: "ImportError: /home/u/vllm-rdna/vllm/_rocm_C.abi3.so: undefined symbol: _Z13rdna_ar2_initllRKN2at6TensorElRKNSt7__cxx1112basic_stringIcSt11char_traitsIcESaIcEEE"},
	{id: "kernels.rocm-lib-not-found", log: "ImportError: libamdhip64.so.7: cannot open shared object file: No such file or directory"},
	{id: "kernels.slow-linear-kernel", log: "(Worker_TP3 pid=193512) INFO 09-29 00:37:24 [gptq.py:137] Using ExllamaLinearKernel for GPTQLinearMethod"},
	{id: "gpu.memory-access-fault", log: "Memory access fault by GPU node-8 (Agent handle: 0x55c055c0a000) on address 0x7fce68e00000. Reason: Page not present or supervisor privilege."},
	{id: "gpu.page-fault", log: "[ 4417.12] amdgpu 0000:07:00.0: amdgpu: [gfxhub] page fault (src_id:0 ring:24 vmid:8 pasid:32771)\n[ 4417.13] amdgpu 0000:07:00.0: amdgpu: UTCL2 client id: 0x1b"},
	{id: "gpu.hsa-exception", log: "Queue error: HSA_STATUS_ERROR_EXCEPTION: An HSAIL operation resulted in a hardware exception."},
	{id: "gpu.hsa-aperture", log: "HSA_STATUS_ERROR_MEMORY_APERTURE_VIOLATION: The agent attempted to access memory beyond the largest legal address."},
	{id: "gpu.ring-timeout", log: "[ 902.3] amdgpu 0000:07:00.0: amdgpu: ring comp_1.0.0 timeout, signaled seq=1234, emitted seq=1236"},
	{id: "memory.gpu-oom", log: "(EngineCore pid=816631) ERROR 09-30 13:04:08 [core.py:1383] torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 772.00 MiB. GPU 0 has a total capacity of 29.98 GiB of which 38.00 MiB is free."},
	{id: "memory.startup-free-low", log: "(Worker pid=20126) ERROR 09-29 01:54:10 [multiproc_executor.py:948] ValueError: Free memory on device cuda:0 (12.78/29.98 GiB) on startup is less than desired GPU memory utilization (0.9, 26.99 GiB). Decrease GPU memory utilization or reduce GPU memory used by other processes."},
	{id: "memory.kv-too-small", log: "(EngineCore pid=472430) ERROR 10-18 06:55:44 [core.py:1472] ValueError: To serve at least one request with the model's max seq len (262144), (3.79 GiB KV cache is needed, which is larger than the available KV cache memory (3.74 GiB). Based on the available memory, the estimated maximum model length is 258000."},
	{id: "memory.host-oom-killed", log: "[ 8812.4] Out of memory: Killed process 4242 (VLLM::EngineCore) total-vm:98765432kB"},
	{id: "memory.shm-too-small", log: "RuntimeError: unable to write to file </torch_4242_1234567890_0>: No space left on device (28)"},
	{id: "memory.disk-full", log: "OSError: [Errno 28] No space left on device"},
	{id: "rccl.allreduce-init-failed", log: "RuntimeError: internal AllReduce init failed (n_devices != 2)"},
	{id: "rccl.hip-failure", log: "rccl-allreduce: NCCL WARN HIP failure 'invalid argument'\ntorch.distributed.DistBackendError: NCCL error: unhandled cuda error (run with NCCL_DEBUG=INFO for details), NCCL version 2.27.7\nncclUnhandledCudaError: Call to CUDA function failed."},
	{id: "rccl.collective-timeout", log: "[rank1]:[E ProcessGroupNCCL.cpp:688] Watchdog caught collective operation timeout: WorkNCCL(SeqNum=366, OpType=ALLREDUCE) ran for 600000 milliseconds"},
	{id: "rdna-ar.wedged", log: "(Worker_TP1 pid=768268) ERROR 10-08 22:40:53 [rdna_all_reduce.py:369] rdna_ar: WEDGED -- rank 1 timed out after ~1953 ms of spinning at collective #366: peer rank 2's flag never arrived"},
	{id: "engine.rpc-timeout", log: "(EngineCore pid=168215) ERROR 10-18 00:55:48 [core.py:1474]     raise TimeoutError(f\"RPC call to {method} timed out.\") from e\nTimeoutError: RPC call to sample_tokens timed out.", comment: "second line is the real runtime text"},
	{id: "engine.shm-broadcast-wait", log: "(EngineCore pid=429170) INFO 09-30 01:15:30 [shm_broadcast.py:801] No available shared memory broadcast block found in 60 seconds. This typically happens when some processes are hanging or doing some time-consuming work (e.g. compilation, weight/kv cache quantization)."},
	{id: "engine.worker-died", log: "(EngineCore pid=171333) ERROR 09-29 19:04:04 [multiproc_executor.py:314] Worker proc VllmWorker-2 died unexpectedly (exit code: -11), shutting down executor."},
	{id: "engine.graph-keyerror", log: "(EngineCore pid=1) ERROR 10-02 11:00:01 [core.py:900]     req_index = model_runner_output.req_id_to_index[req_id]"},
	{id: "compile.backend-failed", log: "(Worker_TP3 pid=839924) ERROR 09-30 13:39:45 [multiproc_executor.py:1054]     raise BackendCompilerFailed("},
	{id: "compile.dynamo-unsupported", log: "(Worker_PP1_TP0_EP0 pid=1422044) ERROR 10-11 23:39:49 [multiproc_executor.py:1062] torch._dynamo.exc.Unsupported: Data-dependent assertion failed (cannot compile partial graph)"},
	{id: "compile.stale-inductor-cache", log: "(Worker_TP2_EP2 pid=3174332) ERROR 10-10 07:50:39 [multiproc_executor.py:1062] AssertionError: expected size 8==8, stride 1536==4096 at dim=0"},
	{id: "compile.triton-error", log: "triton.compiler.errors.CompilationError: at 146:34:"},
	{id: "compile.ctypes-wrapper", log: "TypeError: _SimpleCData.__new__() takes exactly 1 argument (0 given)"},
	{id: "compile.graph-capture-failed", log: "RuntimeError: HIP error: operation not permitted when stream is capturing"},
	{id: "tunableop.rocblas-mismatch", log: "UserWarning: Failed validator: ROCBLAS_VERSION (Triggered internally at /pytorch/aten/src/ATen/cuda/tunable/Tunable.cpp:445.)"},
	{id: "tunableop.unmatched-validator", log: "UserWarning: Unmatched validator: \"HIPBLASLT_VERSION\" is required, but the tuning results does not provide it."},
	{id: "model.hf-offline", log: "huggingface_hub.errors.LocalEntryNotFoundError: Cannot find an appropriate cached snapshot folder for the specified revision on the local disk and outgoing traffic has been disabled. To enable repo look-ups and downloads online, pass 'local_files_only=False'."},
	{id: "model.hf-network", log: "requests.exceptions.ConnectionError: (MaxRetryError(\"HTTPSConnectionPool(host='huggingface.co', port=443): Max retries exceeded with url: /api/models/Qwen/Qwen3-8B (Caused by NameResolutionError)\"))"},
	{id: "model.hf-gated", log: "huggingface_hub.errors.GatedRepoError: 401 Client Error. Cannot access gated repo for url https://huggingface.co/meta-llama/x."},
	{id: "model.not-found", log: "huggingface_hub.errors.RepositoryNotFoundError: 404 Client Error. Repository Not Found for url: https://huggingface.co/nobody/nothing/resolve/main/config.json."},
	{id: "model.files-missing", log: "OSError: /models/foo does not appear to have a file named config.json."},
	{id: "model.safetensors-corrupt", log: "safetensors_rust.SafetensorError: Error while deserializing header: incomplete metadata, file not fully covered"},
	{id: "model.arch-unsupported", log: "ValueError: Model architectures ['Qwen4ForCausalLM'] are not supported for now. Supported architectures: ['LlamaForCausalLM']"},
	{id: "model.quant-unsupported", log: "NotImplementedError: Qwen4Exp PLE embedding does not support quantization config INCConfig"},
	{id: "config.max-model-len", log: "ValueError: User-specified max_model_len (200000) is greater than the derived max_model_len (max_position_embeddings=131072)."},
	{id: "config.tp-heads", log: "ValueError: Total number of attention heads (40) must be divisible by tensor parallel size (3)."},
	{id: "config.batched-tokens", log: "ValueError: max_num_batched_tokens (2048) is smaller than max_model_len (8192). This effectively limits the maximum sequence length."},
	{id: "config.cache-permission", log: "PermissionError: [Errno 13] Permission denied: '/root/.cache/vllm/torch_compile_cache'"},
	{id: "net.port-in-use", log: "ERROR:    [Errno 98] error while attempting to bind on address ('0.0.0.0', 8000): address already in use"},
	{id: "kv.lmcache-hma", log: "ValueError: Failed to promote local KV cache specs to one unified type"},
}

func TestEveryRuleMatchesItsSample(t *testing.T) {
	rules, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, s := range samples {
		have[s.id] = true
		t.Run(s.id, func(t *testing.T) {
			got := ids(Scan(strings.NewReader("INFO starting\n"+s.log+"\nINFO done\n"), rules))
			want := append([]string{s.id}, s.also...)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("matched %v, want %v", got, want)
			}
		})
	}
	for _, r := range rules {
		if !have[r.ID] {
			t.Errorf("rule %s has no sample in this test", r.ID)
		}
	}
	for id := range have {
		if !slices.ContainsFunc(rules, func(r Rule) bool { return r.ID == id }) {
			t.Errorf("sample for unknown rule %s", id)
		}
	}
}

// A healthy vLLM startup (lines from real runs) must produce no match.
const healthyLog = `(APIServer pid=192998) INFO 09-29 00:30:01 [utils.py:253] vLLM server version 0.31.0
TunableOp lookup enabled for rocBLAS build f30bb442e9b5; tuning stays off (set PYTORCH_TUNABLEOP_TUNING=1 to re-tune missing shapes).
(EngineCore pid=834976) INFO 09-30 13:36:13 [gpu_worker.py:941] Free memory on device (29.92/29.98 GiB) on startup. Desired GPU memory utilization is (0.85, 25.49 GiB). Actual usage is 11.56 GiB for consumed memory (weights + non-torch), 1.5 GiB for peak activation, and 3.91 GiB for CUDAGraph memory.
(Worker_TP3 pid=193512) INFO 09-29 00:37:24 [compressed_tensors_wNa16.py:137] Using RDNA2W4A16LinearKernel for CompressedTensorsWNA16
(EngineCore pid=1) INFO 09-29 00:38:10 [kv_cache_utils.py:1291] GPU KV cache size: 412,160 tokens
(EngineCore pid=1) INFO 09-29 00:38:10 [kv_cache_utils.py:1296] Maximum concurrency for 32,768 tokens per request: 12.58x
(Worker_TP0 pid=2) INFO 09-29 00:40:00 [gpu_model_runner.py:5510] Graph capturing finished in 31 secs, took 3.91 GiB
(APIServer pid=192998) INFO:     Started server process [192998]
(APIServer pid=192998) INFO:     Waiting for application startup.
(APIServer pid=192998) INFO:     Application startup complete.
(APIServer pid=192998) INFO 09-29 00:41:11 [loggers.py:123] Engine 000: Avg prompt throughput: 0.0 tokens/s, Avg generation throughput: 61.3 tokens/s, Running: 1 reqs, GPU KV cache usage: 0.4%, Prefix cache hit rate: 0.0%
`

func TestHealthyLogHasNoMatch(t *testing.T) {
	rules, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := Scan(strings.NewReader(healthyLog), rules); len(got) != 0 {
		t.Fatalf("healthy log matched %v", ids(got))
	}
}

func TestScanReportsFirstMatchingLine(t *testing.T) {
	rules := []Rule{
		{ID: "t.oom", Pattern: `out of memory`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
		{ID: "t.port", Pattern: `Errno 98`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
		{ID: "t.none", Pattern: `never appears`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
	}
	log := "start\r\nCUDA out of memory (first)\n\nErrno 98\nCUDA out of memory (second)\n"
	got := Scan(strings.NewReader(log), rules)
	if len(got) != 2 {
		t.Fatalf("got %d matches: %+v", len(got), got)
	}
	if got[0].ID != "t.oom" || got[0].LineNo != 2 || got[0].Line != "CUDA out of memory (first)" {
		t.Errorf("first match = %+v", got[0])
	}
	if got[1].ID != "t.port" || got[1].LineNo != 4 {
		t.Errorf("second match = %+v", got[1])
	}
	if got[0].Fix != "f" {
		t.Error("match does not carry the rule")
	}
}

func TestScanHandlesLongLines(t *testing.T) {
	rules := []Rule{
		{ID: "t.head", Pattern: `HEAD_MARK`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
		{ID: "t.tail", Pattern: `TAIL_MARK`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
		{ID: "t.after", Pattern: `AFTER_MARK`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"},
	}
	long := "HEAD_MARK" + strings.Repeat("x", 2*MaxLineBytes) + "TAIL_MARK"
	got := Scan(strings.NewReader("a\n"+long+"\nAFTER_MARK\n"), rules)
	if g := ids(got); !slices.Equal(g, []string{"t.after", "t.head"}) {
		t.Fatalf("matched %v, want t.after and t.head (text past the cap is skipped)", g)
	}
	for _, m := range got {
		switch m.ID {
		case "t.head":
			if m.LineNo != 2 || len(m.Line) > maxShownLine+len("…") {
				t.Errorf("head match: line %d, %d bytes", m.LineNo, len(m.Line))
			}
		case "t.after":
			if m.LineNo != 3 {
				t.Errorf("after match on line %d, want 3", m.LineNo)
			}
		}
	}
}

func TestScanHonoursRegexpAnchorsAndCase(t *testing.T) {
	// A pattern with a case-insensitive literal must not be skipped by the literal prefilter.
	rules := []Rule{{ID: "t.ci", Pattern: `(?i)out of MEMORY`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"}}
	if got := Scan(strings.NewReader("Out Of Memory\n"), rules); len(got) != 1 {
		t.Fatalf("case-insensitive rule did not match: %v", got)
	}
	// An alternation has no single required literal.
	alt := []Rule{{ID: "t.alt", Pattern: `foo|bar`, Severity: SeverityError, Cause: "c", Fix: "f", Doc: "d"}}
	if got := Scan(strings.NewReader("a bar b\n"), alt); len(got) != 1 {
		t.Fatalf("alternation did not match: %v", got)
	}
}

func TestScanFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "serve.log")
	if err := os.WriteFile(bad, []byte(healthyLog+"RuntimeError: HIP error: hipErrorInvalidImage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ScanFile(bad)
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); !slices.Equal(g, []string{"kernels.invalid-image"}) {
		t.Fatalf("matched %v", g)
	}
	if got[0].LineNo != strings.Count(healthyLog, "\n")+1 {
		t.Errorf("line number %d", got[0].LineNo)
	}
	if _, err := ScanFile(filepath.Join(dir, "missing.log")); err == nil {
		t.Error("missing file: no error")
	}
}

func ids(ms []Match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	slices.Sort(out)
	return out
}

// BenchmarkScanHealthy scans 8 MiB of ordinary startup lines with the full
// catalog. The literal prefilter keeps this far below running every regexp on
// every line.
func BenchmarkScanHealthy(b *testing.B) {
	rules, err := Load()
	if err != nil {
		b.Fatal(err)
	}
	data := []byte(strings.Repeat(healthyLog, (8<<20)/len(healthyLog)))
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		Scan(bytes.NewReader(data), rules)
	}
}
