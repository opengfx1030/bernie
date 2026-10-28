# rdnactl specification

Status: draft 1 (2026-10-28). This document is the contract. Code, tests and
the dashboard follow it. Change this file in the same pull request as the
behavior.

Writing rules for this file: short sentences, one fact or one instruction per
sentence, active voice, defined terms only (see [Terms](#terms)).

## 1. Purpose

rdnactl helps an operator run vLLM on AMD ROCm GPUs, RDNA first
(gfx1030, then gfx11/gfx12). It replaces the serve scripts and `.env`
recipes of the `opengfx1030/vllm-rdna` fork with one tool.

rdnactl does these tasks:

1. Inspect the host: hardware, PCIe topology, ROCm, libraries, containers.
2. Report what is missing, and give the fix for the host's package manager.
3. Install the software stack (host venv or container image).
4. Load recipes and turn one recipe into one vLLM launch.
5. Start, stop, list and log vLLM servers (host process or container).
6. Apply tuning: TunableOp rows, GPU power cap.
7. Benchmark an OpenAI-compatible server.
8. Show all of the above in a web dashboard.

## 2. Principles

- **CLI first.** Every feature is a CLI command. The dashboard calls the same
  Go packages. The dashboard has no feature that the CLI does not have.
- **Machine-readable.** Every command accepts `--json` and then writes exactly
  one JSON document to stdout. Agents and scripts use this mode.
- **One static binary.** The binary embeds the built-in recipes, the TunableOp
  rows and the dashboard assets. The target host needs no Python, Node or Go
  to run rdnactl.
- **Read-only by default.** Inspection commands never change the host.
  Commands that change the host print the plan first. rdnactl never runs
  `sudo`. Steps that need root are printed as commands for the operator.
- **Data over code.** Model and hardware knowledge lives in recipes (YAML).
  Go code holds only generic mechanics.
- **No `/tmp`.** All files go under the XDG paths in [§12](#12-files-and-paths).
- **Docker optional.** Every feature works with Docker or Podman and
  without any container engine. Every recipe runs in host mode. Container
  mode is an option a recipe adds with `container.image`. A host without an
  engine is a normal host: container checks are `skip`, never `fail`, and
  `up` selects host mode.
- **Never break other platforms.** A recipe declares its GPU architectures.
  rdnactl refuses to launch a recipe on GPUs that do not match.

## Terms

| Term | Meaning |
|---|---|
| host | The Linux machine with the GPUs. |
| mode | How vLLM runs: `host` (a Python venv on the host) or `container` (Docker or Podman). |
| engine | The container program: `docker` or `podman`. |
| recipe | A YAML file that describes one vLLM configuration. |
| fragment | A recipe whose id starts with `_`. It is only used through `extends`. It is never listed or launched. |
| input | A named value that a recipe needs from the operator, for example a local path. |
| plan | The resolved launch: environment, arguments, GPUs, image. A plan is the output of a recipe plus overrides plus the host. |
| run | One started plan. A run has a name, a record on disk and a log. |
| check | One doctor test with a status: `ok`, `warn`, `fail` or `skip`. |
| HIP index | The GPU index that `HIP_VISIBLE_DEVICES` uses. It is the order of GPU nodes in the KFD topology. |

## 3. Command surface

### 3.1 The default: run the binary

The whole product is one file. The operator downloads it and runs it. No
other file, script or compose file is required.

- `rdnactl` with no arguments runs `rdnactl status`. It changes nothing.
  It prints the GPUs, the doctor summary, the best recipe for this host,
  the running runs, and the one next command to type.
- `rdnactl up [RECIPE]` does the full path in one command. It runs the
  doctor, applies the setup steps that do not need root, selects the best
  recipe that fits (or RECIPE), starts it, waits until `/health` answers,
  and starts the dashboard. It stops before a step that needs root or an
  input, and prints what the operator must do. It asks for confirmation
  once, unless `--yes` is given.
- With `--json`, `status` and `up` write one JSON document. `up --json`
  never prompts; it requires `--yes` to change the host.

### 3.2 Rules

- `--json` and `--help` can be anywhere on the command line.
- Diagnostics and progress go to stderr.
- Flags and positional arguments can be in any order.
- Exit codes: `0` success, `1` runtime failure, `2` usage error,
  `3` checks failed (the report is still printed).
- With `--json`, a failure writes `{"error": "...", "exit_code": N}`.

| Command | Changes host | Purpose |
|---|---|---|
| `rdnactl` / `rdnactl status` | no | Overview and the next command to type. |
| `rdnactl up [RECIPE] [--set K=V]... [--mode M] [--yes]` | yes | Doctor → setup → best recipe → serve → dashboard. |
| `rdnactl hw` | no | Hardware and OS inventory. |
| `rdnactl topo [--bandwidth] [--venv DIR]` | no | PCIe tree, P2P matrix, link classes. Optional bandwidth test. |
| `rdnactl doctor [--category C]... [--venv DIR]` | no | All checks. Exit 3 if one check fails. |
| `rdnactl setup plan [--mode M]` | no | Ordered install steps for this host. |
| `rdnactl setup apply [--mode M] [--step ID]... [--yes]` | yes | Run the steps that do not need root. Print the root steps. |
| `rdnactl recipe list [--all]` | no | Recipes, with fit to this host. `--all` includes recipes that do not fit. |
| `rdnactl recipe show <id>` | no | The merged recipe (after `extends`). |
| `rdnactl recipe validate [FILE]...` | no | Validate files, or all loaded recipes. |
| `rdnactl recipe plan <id> [--set K=V]... [--mode M]` | no | The plan, as an exact command and environment. |
| `rdnactl serve <id> [--set K=V]... [--mode M] [--name N] [--foreground]` | yes | Start a run. |
| `rdnactl ps` | no | Runs, with state and health. |
| `rdnactl stop [NAME]` | yes | Stop a run (the only run if NAME is absent). |
| `rdnactl logs [NAME] [-f] [--tail N]` | no | Show the log of a run. |
| `rdnactl compose <id> [--set K=V]... [-o FILE]` | no* | Write a `compose.yml` for the plan (*writes FILE only). |
| `rdnactl tune tunableop list` | no | Registered TunableOp profiles. |
| `rdnactl tune tunableop status [--venv DIR]` | no | Loaded rocBLAS build, matching profile. |
| `rdnactl tune tunableop env [--venv DIR] [--profile P] [--tuning]` | no* | `export` lines for TunableOp (*materializes rows). |
| `rdnactl tune power status` | no | Power cap per GPU. |
| `rdnactl tune power set <WATTS> [--gpu IDX]...` | yes | Set the power cap. Without root, print the command. |
| `rdnactl bench [--url U] [flags]` | no | Benchmark a server. Save the result. |
| `rdnactl bench list` / `bench show <ID>` | no | Saved results. |
| `rdnactl config show` / `config set <KEY> <VALUE>` | yes (config file) | User defaults. |
| `rdnactl dash [--listen ADDR] [--token T]` | no | Start the dashboard. |
| `rdnactl version` | no | Version. |

## 4. Host inspection (`hw`, `topo`)

### 4.1 Sources

rdnactl reads sysfs and procfs first. It runs external tools only to add
information. A missing tool is a `skip`, not an error.

| Data | Source |
|---|---|
| GPUs, gfx target, HIP index | `/sys/class/kfd/kfd/topology/nodes/*/properties` (`gfx_target_version`, `location_id`, `drm_render_minor`). Nodes with `simd_count > 0` are GPUs. |
| PCI address, VRAM, link | `/sys/class/drm/renderD<minor>/device` → `mem_info_vram_total`, `current_link_speed`, `current_link_width`, `numa_node`. |
| Telemetry | `gpu_busy_percent`, `mem_info_vram_used`, `hwmon/*/temp*_input`, `power1_average`, `power1_cap`. |
| PCIe tree | The real path of the PCI device in `/sys/devices`. |
| P2P | `nodes/<n>/io_links` and `nodes/<n>/p2p_links`. |
| OS | `/etc/os-release`, `uname`, `/proc/cmdline`, `/proc/meminfo`, `/proc/cpuinfo`. |

`gfx_target_version` `100300` is `gfx1030`. The rule: major = v/10000,
minor = (v/100)%100, step = v%100; name = `gfx` + major + minor + step, with
minor and step in hex.

### 4.2 Link classes

`topo` classifies every GPU pair with the RCCL names:

| Class | Meaning |
|---|---|
| `PIX` | Both GPUs are below the same PCIe switch. |
| `PXB` | The path crosses more than one switch, but not the root complex. |
| `PHB` | The path crosses one root complex (host bridge). |
| `SYS` | The path crosses root complexes (CPU interconnect). |

`topo` also reports `p2p: true|false` from the KFD links.
`topo --bandwidth` runs `rocm-bandwidth-test` when it is installed. Without
it, and with `--venv`, it runs a PyTorch peer-copy probe. It reports GB/s per
pair.

### 4.3 JSON shape (`hw`)

```json
{
  "host": {"hostname": "", "os": {"id": "fedora", "version_id": "43", "pretty": ""},
           "kernel": "", "pkg_manager": "dnf", "cpu": {"model": "", "vendor": "",
           "sockets": 1, "cores": 48, "threads": 96}, "ram_bytes": 0,
           "virtualization": "none"},
  "gpus": [{"hip_index": 0, "kfd_node": 1, "name": "", "gfx": "gfx1030",
            "pci": "0000:07:00.0", "device_id": "0x73a1", "vram_bytes": 0,
            "render_minor": 128, "numa_node": 0,
            "link": {"gen": 4, "width": 16}, "power_cap_w": 0}]
}
```

## 5. Doctor

### 5.1 Check record

```json
{"id": "rocm.hipcc", "category": "rocm", "status": "ok|warn|fail|skip",
 "title": "hipcc is installed", "detail": "/opt/rocm/core-7.14/bin/hipcc (HIP 7.14.60850)",
 "fix": "sudo dnf install hipcc", "doc": "https://..."}
```

The report is `{"summary": {"ok": 0, "warn": 0, "fail": 0, "skip": 0}, "checks": [...]}`.
`fix` is a command for the host's package manager when one exists, else one
sentence. Rule: `fail` blocks a launch; `warn` costs performance or
stability; `skip` means the check cannot run here.

### 5.2 Required checks

| Category | Checks |
|---|---|
| `os` | Distro and version known. Package manager known. Kernel version. |
| `gpu` | `/dev/kfd` exists and is readable/writable. `/dev/dri/renderD*` exist. User is in `video` and `render`. At least one GPU. Each GPU arch is known; gfx1031/1032 and other RDNA2 parts get the `HSA_OVERRIDE_GFX_VERSION=10.3.0` advice. |
| `kernel` | `amdgpu` is loaded. IOMMU mode from `/proc/cmdline` (`iommu=pt` advice for P2P). |
| `rocm` | ROCm installs found (`/opt/rocm*`, `/opt/rocm/core-*`, `/usr`) with versions. More than one install: `warn`. `rocminfo` lists the GPUs. `hipcc`, `amd-smi` or `rocm-smi` present. `librocblas.so.5` found. ROCm 7.2.1 to 7.13 with more than one GPU: `warn` (RCCL TP>1 deadlock range). |
| `python` | With `--venv` (or the configured venv): Python version, `torch` imports, `torch.version.hip` set, GPU count matches, `vllm` imports, fork ops registered (`_rocm_C::*rdna*` schema count > 10). |
| `container` | `docker` or `podman` present, daemon or socket usable, compose present, rootless state, SELinux mode. |
| `resources` | RAM. Free disk in the Hugging Face cache. `/dev/shm` size. |
| `tuning` | TunableOp profile matches the loaded rocBLAS. Power cap per GPU (V620: 180 W advice). |

## 6. Setup

`setup plan` returns ordered steps. Each step has this record:

```json
{"id": "venv.create", "title": "", "mode": "host|container|both",
 "root": false, "done": false, "commands": ["..."], "why": ""}
```

`done` is true when the step's postcondition already holds (rdnactl checks it).
`setup apply` runs, in order, the steps that are not done and do not need
root. It stops at the first failure. It prints root steps and does not run
them. `--yes` skips the confirmation prompt. `--step` limits the run to named
steps.

Steps (filtered by mode and host):

| ID | Root | Mode | Content |
|---|---|---|---|
| `groups` | yes | both | `usermod -aG video,render $USER`. |
| `rocm.runtime` | yes | host | Install the ROCm runtime with the package manager. |
| `container.engine` | yes | container | Install Docker or Podman and compose. |
| `venv.create` | no | host | Create the venv (`uv` if present, else `python3 -m venv`). |
| `venv.torch` | no | host | Install `rocm-sdk` and `torch` from the AMD ROCm wheel index. |
| `vllm.build` | no | host | Clone the fork and build it in the venv. |
| `image.pull` | no | container | Pull the recipe image. |
| `model.download` | no | both | `hf download <repo>` into the Hugging Face cache. |

## 7. Recipes

### 7.1 Files and lookup

A recipe is one YAML file. The file name stem is the recipe id. rdnactl loads
recipes in this order. A later file with the same id replaces an earlier one.

1. Built-in recipes (embedded from `recipes/`).
2. `<config>/recipes/*.yaml`.
3. Each directory in `$RDNACTL_RECIPES` (`:`-separated).

A file that does not parse or validate is reported by `recipe list` and
`recipe validate`. It is never dropped silently.

### 7.2 Schema

```yaml
schema: 1                       # required, integer
extends: [_base, _rdna-ar]      # optional; fragments or recipes, merged left to right
name: Qwen3.8 Flash-Next AWQ + MTP=2
description: One paragraph. What, why, validated where.
status: validated               # validated | experimental | reference
priority: 30                    # higher wins among recipes that fit (used by `up`); default 0
tags: [qwen, moe, mtp]
entrypoint: cli                 # cli = `vllm serve` (default); api = openai.api_server (as validated by the fork)

model:
  source: wtdcode/Qwen3.8-Flash-Next-AWQ-W4A16   # HF repo id or absolute path
  served_name: flash-next
  revision: main                # optional

requires:
  arch: [gfx1030]               # gfx targets or prefixes ("gfx11" matches gfx1100)
  gpus: 4                       # GPUs of a matching arch
  vram_gib: 28                  # per GPU
  ram_gib: 64                   # host RAM
  engine: vllm-rdna             # vllm | vllm-rdna (needs the fork's kernels)

inputs:                         # values the operator gives with --set NAME=VALUE
  PLE_QUANT_DIR:
    description: ples_int4 directory from primitive-ai/Qwen3.8-Flash-Next-PLE-quant
    required: true
    path: true                  # mounted into the container at the same path
    default: ""

container:                      # omit (or null) = host mode only: the image lacks this feature
  image: docker.io/blivioniag/vllm-rdna:v0.31.0-extras
  tunableop_profile: rocm7.14-rocblas5.5   # rows for the image's rocBLAS build
  shm_size: 16g
  pull: true                    # pull before start (tags are refreshed in place); --no-pull skips

env:                            # environment for vLLM; null removes a key
  VLLM_RDNA_AR: "1"

args:                           # vllm serve flags without the leading --
  tensor-parallel-size: 4
  max-model-len: 262144
  enable-prefix-caching: true
  compilation-config: {mode: 3, cudagraph_mode: FULL_AND_PIECEWISE}
```

### 7.3 Merge rules

- `extends` is resolved depth first, then left to right, then the file itself.
- Maps merge key by key. Lists replace. Scalars replace.
- A `null` value deletes the key (for example, to remove an inherited flag).
- A cycle in `extends` is an error.

### 7.4 Interpolation

`${NAME}` in any string of `model`, `env` or `args` is replaced by the input
`NAME`. A required input with no value is an error at plan time. An unknown
`${NAME}` is a validation error.

### 7.5 Overrides (`--set`)

- `--set KEY=VALUE` where KEY has no dot sets the input KEY.
- `--set section.key=VALUE` sets a field, for example
  `--set args.max-model-len=32768` or `--set env.VLLM_RDNA_AR=0`.
- VALUE is parsed as YAML: `true`, `32768`, `null`, `{a: 1}`, `[1, 2]`.
- Precedence: recipe < config defaults < `--set`.

### 7.6 Argument rendering

- The command is `vllm serve <model.source>` followed by `args` sorted by key.
- `true` gives `--key`. `false` and `null` give nothing.
- A map or list gives `--key '<compact JSON>'`.
- Other values give `--key VALUE`.
- `served-model-name` comes from `model.served_name`. `revision` comes from
  `model.revision`.

### 7.7 GPU selection

When the plan has no `HIP_VISIBLE_DEVICES`, rdnactl selects GPUs:

1. Keep GPUs whose arch matches `requires.arch`.
2. Need `requires.gpus` of them, else the recipe does not fit.
3. Prefer a set that shares one PCIe switch (`PIX`), then the lowest HIP
   indices.

`recipe plan` shows the selection and the reason.

### 7.8 Fit

`recipe list` gives each recipe a fit: `fits`, `no` (with reasons), or
`unknown` (no GPU data). Reasons: arch, GPU count, VRAM, RAM.

## 8. Runs

### 8.1 Host mode

- Python: `<venv>/bin/python -m vllm.entrypoints.cli.main serve <model> ...`
  for `entrypoint: cli`, or
  `<venv>/bin/python -m vllm.entrypoints.openai.api_server --model <model> ...`
  for `entrypoint: api`. Container mode uses the same choice
  (`--entrypoint python3` with the module for `api`).
- `ROCM_HOME`, `ROCM_PATH`, `HIP_PATH` come from the detected ROCm install
  (never a hard-coded path) unless the recipe or `--set` gives them.
- `HF_HUB_OFFLINE` and `TRANSFORMERS_OFFLINE` are `1` when `model.source`
  is a local path, and `0` when it is a Hugging Face repo id.
- Host mode needs the fork built in the venv for `requires.engine: vllm-rdna`.
  It is not "the container without Docker".
- Venv: `--venv`, then `$RDNACTL_VENV`, then config `venv`, then `$VIRTUAL_ENV`.
- `LD_LIBRARY_PATH` starts with the venv's ROCm wheel libraries when present:
  `_rocm_sdk_libraries/lib`, `_rocm_sdk_core/lib/{host-math,rocm_sysdeps,core}/lib`,
  `torch/lib`. Reason: the system rocBLAS must not load before the wheel one.
- Caches: `VLLM_CACHE_ROOT`, `TRITON_CACHE_DIR`, `TORCHINDUCTOR_CACHE_DIR`
  under `<cache>/runs/<name>/`.
- The process starts in a new session. The working directory is the run
  directory (never the vLLM source tree: `vllm/entrypoints/cli/openai.py`
  shadows the `openai` package).
- `stop` sends SIGTERM to the process group, waits 20 s, then SIGKILL.

### 8.2 Container mode

The run command is:

```
<engine> run -d --name rdnactl-<name> --label rdnactl.run=<name>
  --network host --ipc host --shm-size <shm>
  --device /dev/kfd --device /dev/dri --group-add video --group-add render
  --security-opt seccomp=unconfined --security-opt label=disable
  -v <hf-cache>:/root/.cache/huggingface
  -v <tunableop rows>:/rdnactl/tunableop:ro
  -v <tuned-moe dir>:/rdnactl/tuned-moe:ro   (when the recipe sets VLLM_TUNED_CONFIG_FOLDER)
  -v <path input>:<path input>         (one per path input)
  -e KEY=VALUE ...
  <image> serve <model> <args>
```

Rootless Podman uses `--group-add keep-groups` instead of the two group flags.
`compose` writes the same plan as a Compose file that works with
`docker compose`, `podman compose` and `podman-compose`.

### 8.3 Run record

`<state>/runs/<name>/run.json`:

```json
{"name": "flashnext-mtp2", "recipe": "flashnext-mtp2", "mode": "host",
 "pid": 1234, "container": "", "engine": "", "host": "127.0.0.1", "port": 18096,
 "started_at": "RFC3339", "log": "<state>/runs/<name>/serve.log",
 "image": "", "image_digest": "", "plan": {...}}
```

`image_digest` is set in container mode from `<engine> image inspect` after
the pull. Tags such as `v0.31.0-extras` are refreshed in place, so the tag
alone does not identify the image.

`ps` state: `running`, `starting` (process alive, `/health` not ready),
`exited`. One name has one run. `serve` refuses a name that is running.

## 9. Tuning

### 9.1 TunableOp

- Rows are build-specific. A profile is valid only for the rocBLAS build
  (sha256 of `librocblas.so.5`) that produced it.
- The registry is `tunableop/profiles.json`, embedded in the binary.
- `env` and `serve` materialize the rows into `<data>/tunableop/<profile>/`.
- Host mode: rdnactl hashes the loaded `librocblas.so.5` (venv wheel first,
  then `/opt/rocm/lib`) and selects the profile with the same sha256 prefix
  (12 hex). An explicit profile that does not match is an error.
- No match: per-user rows at `$HOME/.cache/tunableop/tunableop_results.csv`.
- Container mode: the recipe names the profile for its pinned image.
- Lookup only by default (`PYTORCH_TUNABLEOP_TUNING=0`). `--tuning` enables
  tuning into the per-user file.
- Environment set: `PYTORCH_TUNABLEOP_ENABLED=1`, `PYTORCH_TUNABLEOP_TUNING`,
  `PYTORCH_TUNABLEOP_HIPBLASLT_ENABLED=0`, `PYTORCH_TUNABLEOP_FILENAME`.

### 9.2 Power

`tune power status` reads `power1_cap`, `power1_cap_min`, `power1_cap_max`.
`tune power set` writes `power1_cap` (microwatts) when it runs as root. Else
it prints the exact command. The recipe or the doctor gives the advice
(V620: 180 W).

## 10. Benchmark

`rdnactl bench` sends streaming requests to `/v1/completions`.

| Flag | Default | Meaning |
|---|---|---|
| `--url` | the only running run, else `http://127.0.0.1:8000` | Server base URL. |
| `--model` | first id of `/v1/models` | Model name. |
| `--input-len` | `1024` | Prompt tokens (calibrated with `/tokenize` when present). |
| `--output-len` | `512` | Output tokens (`ignore_eos`, temperature 0). |
| `--concurrency` | `1` | Comma list, for example `1,8`. One cell per value. |
| `--num-prompts` | `max(4, 2 × concurrency)` | Requests per cell. |
| `--warmup` | `1` | Requests sent and discarded before each cell. |
| `--seed` | `0` | Prompt seed. |
| `--tag` | none | Free label stored with the result. |

Every prompt gets a unique salt so the prefix cache does not change the
result. Prompts are sized with `/tokenize` when the server has it. Without
it, the word count is an estimate and the real mean is in the result.
With MTP, a chunk can carry several tokens. rdnactl divides the gap of a
chunk by its token count, so ITL is per token.

Results go to `<state>/bench/<id>.json` (id = UTC timestamp + short hash).
The fingerprint has: host, GPU summary, recipe name, image digest (container
runs) and run plan hash. Cell fields:

| Field | Meaning |
|---|---|
| `concurrency`, `num_prompts`, `completed`, `failed` | Cell size and outcome. |
| `ttft_p50_ms`, `ttft_p95_ms` | Time to first token. |
| `itl_p50_ms`, `itl_p95_ms` | Inter-token latency. |
| `decode_tok_s` | Mean per request: tokens after the first ÷ (last − first chunk time). |
| `prefill_tok_s` | Mean per request: prompt tokens ÷ TTFT. |
| `output_tok_s` | All completion tokens ÷ cell wall time. |
| `mean_prompt_tokens`, `mean_output_tokens`, `tokens_per_chunk` | Real sizes seen. |
| `duration_s`, `errors` | Wall time; sample of error strings. |

A cell where every request fails makes `bench` exit 1 and saves nothing.

## 11. Dashboard

`rdnactl dash` serves an embedded web UI and a JSON API. It listens on
`127.0.0.1:7860` by default. On a non-loopback address, every request needs
the token (`Authorization: Bearer <t>` or the cookie set by `/?token=<t>`).
The token is `--token`, else `$RDNACTL_DASH_TOKEN`, else a random token
printed at start. POST requests also need the header `X-Rdnactl: 1`.

| Method | Path | Body / output |
|---|---|---|
| GET | `/api/hw` | `hw` JSON |
| GET | `/api/topo` | `topo` JSON |
| GET | `/api/doctor` | `doctor` JSON |
| GET | `/api/recipes` | `recipe list --all` JSON |
| GET | `/api/recipes/{id}/plan` | `recipe plan` JSON (query `set=K=V`, repeatable; `mode`) |
| GET | `/api/runs` | `ps` JSON |
| POST | `/api/runs` | `{"recipe": "", "set": ["K=V"], "mode": "", "name": ""}` → run record |
| DELETE | `/api/runs/{name}` | stop |
| GET | `/api/runs/{name}/logs` | Server-sent events, one line per event |
| GET | `/api/telemetry` | Server-sent events, 1 Hz: GPU telemetry + vLLM metrics of running runs |
| GET | `/api/bench` | `bench list` JSON |
| POST | `/api/bench` | bench flags as JSON → result |

## 12. Files and paths

| Name | Default | Override |
|---|---|---|
| config | `~/.config/rdnactl` | `$RDNACTL_CONFIG_DIR`, `$XDG_CONFIG_HOME` |
| state | `~/.local/state/rdnactl` | `$RDNACTL_STATE_DIR`, `$XDG_STATE_HOME` |
| data | `~/.local/share/rdnactl` | `$RDNACTL_DATA_DIR`, `$XDG_DATA_HOME` |
| cache | `~/.cache/rdnactl` | `$RDNACTL_CACHE_DIR`, `$XDG_CACHE_HOME` |

Config file `<config>/config.yaml`:

```yaml
venv: /home/me/Apps/vllm/venv-7.14.0   # host-mode default
mode: host                              # host | container
engine: podman                          # docker | podman (auto when empty)
hf_home: /data/hf                       # Hugging Face cache root
set: ["env.GPU_MAX_HW_QUEUES=2"]        # default overrides, applied before --set
```

## 13. Out of scope (draft 1)

- Model quantization and checkpoint conversion.
- Engine changes (kernels, MoE offload). They belong to the fork.
- Multi-host fleet control. The dashboard shows one host.
- A one-click onboarding wizard. `setup apply` is the base for it.
