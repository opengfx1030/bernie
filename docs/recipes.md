# Writing rdnactl recipes

This document tells you how to write a recipe. The contract is
[spec.md §7](spec.md#7-recipes). If the two disagree, spec.md is correct.

Writing rules for this file: short sentences, one fact or one instruction per
sentence, active voice.

## 1. What a recipe is

A recipe is one YAML file. It describes one vLLM launch: the model, the
hardware it needs, the environment and the `vllm serve` flags.

- The file name stem is the recipe id. `flashnext-mtp2.yaml` has the id
  `flashnext-mtp2`.
- An id uses lowercase letters, digits, `.`, `_` and `-`.
- A file whose id starts with `_` is a fragment. A fragment is a shared part.
  You cannot plan or serve a fragment. Other recipes use it through `extends`.
- rdnactl turns a recipe into a plan: `rdnactl recipe plan <id>`. The plan is
  the exact command and environment. `rdnactl serve <id>` starts the plan.

## 2. Where rdnactl finds recipes

rdnactl loads recipes in this order. A later file with the same id replaces
an earlier one.

1. The built-in recipes (embedded from `recipes/` in the source tree).
2. `<config>/recipes/*.yaml` (default `~/.config/rdnactl/recipes/`).
3. Each directory in `$RDNACTL_RECIPES` (`:`-separated).
4. The files given to `rdnactl recipe validate FILE...`.

A file that does not load is listed by `recipe list` and `recipe validate`.
rdnactl does not drop it silently.

To change a built-in recipe, do not copy it. Write a new file that extends
it (see [§5](#5-extends-and-merge-rules)).

## 3. Schema

```yaml
schema: 1                       # required. Always 1.
extends: [_base, _attn-fa]      # optional. Merged left to right, then this file.
name: Qwen3.8 Flash-Next AWQ + MTP=2
description: >-
  One paragraph. What it is, why, and where it was validated.
status: validated               # validated | experimental | reference
priority: 40                    # higher wins among recipes that fit. Default 0.
tags: [qwen, moe, mtp]
entrypoint: api                 # cli = `vllm serve` (default). api = openai.api_server.

model:
  source: wtdcode/Qwen3.8-Flash-Next-AWQ-W4A16   # HF repo id or absolute path
  served_name: flash-next       # optional. Becomes --served-model-name.
  revision: main                # optional. Becomes --revision.

requires:
  arch: [gfx1030]               # gfx targets or prefixes. "gfx11" matches gfx1100.
  gpus: 4                       # GPUs of a matching arch
  vram_gib: 28                  # per GPU
  ram_gib: 48                   # host RAM
  engine: vllm-rdna             # vllm | vllm-rdna (needs the fork's kernels)

inputs:                         # values the operator gives with --set NAME=VALUE
  PLE_QUANT_DIR:
    description: ples_int4 directory (has META.json)
    required: true
    path: true                  # a host path; mounted into the container
    default: ""

container:                      # omit = host mode only
  image: docker.io/blivioniag/vllm-rdna:v0.31.0-extras
  tunableop_profile: rocm7.14-rocblas5.5
  shm_size: 16g                 # default 16g
  pull: true                    # default true. `serve --no-pull` skips the pull.

env:                            # environment of the vLLM process
  VLLM_RDNA_AR: "1"
  FLASH_ATTENTION_TRITON_AMD_ENABLE: null   # null removes the variable

args:                           # vllm serve flags, without the leading --
  tensor-parallel-size: 4
  max-model-len: 262144
  enable-prefix-caching: true
  compilation-config: {mode: 3, cudagraph_mode: FULL_AND_PIECEWISE}
```

### 3.1 Required fields

A recipe that is not a fragment needs these fields after the merge:

- `schema`, `name`, `status`, `model.source`.
- `requires.arch` and `requires.gpus`.
- `container.image` when `container` is set.

`requires.gpus` must equal `tensor-parallel-size` x `pipeline-parallel-size`
x `data-parallel-size` when the recipe sets one of them.

### 3.2 Field rules

- Write environment values as strings: `"1"`, not `1`. Numbers and booleans
  are accepted and written as text.
- Write `args` keys in lowercase with `-`. Do not write `--`.
- Do not put `model`, `served-model-name` or `revision` in `args`. Use the
  `model` block.
- Unknown fields are errors. A typo such as `modle:` fails the load.

### 3.3 Container or host

- A recipe with `container.image` runs in host mode and in container mode.
- A recipe without `container` runs in host mode only. Use this when the
  published image lacks a feature that the recipe needs. `--mode container`
  then fails with a clear message.
- The default mode is the config `mode`. When it is empty, rdnactl uses
  container mode if the recipe has an image and Docker or Podman is usable.
  Else it uses host mode.
- In host mode, rdnactl selects the TunableOp profile from the loaded
  `librocblas.so.5`. In container mode, it uses `container.tunableop_profile`.

## 4. Arguments

rdnactl renders `args` in key order:

| Value | Flag |
|---|---|
| `true` | `--key` |
| `false` or `null` | nothing |
| a map or a list | `--key '<compact JSON>'` |
| another value | `--key VALUE` |

Example: `speculative-config: {method: mtp, num_speculative_tokens: 2}` gives
`--speculative-config '{"method":"mtp","num_speculative_tokens":2}'`.

## 5. Extends and merge rules

`extends` lists fragments or recipes. rdnactl merges the parents depth first,
left to right, then the file itself. A cycle or a missing parent is an error.

Merge rules:

- Maps merge key by key. A nested map such as `compilation-config` merges too.
- Lists replace. `tags: [a]` in the child replaces the parent's tags.
- Scalars replace.
- `null` deletes the key. Under `env`, `null` also removes the variable from
  the inherited environment (and from the image environment in container
  mode).

Example: run `flashnext` with 4 sequences and no tool parser.

```yaml
# ~/.config/rdnactl/recipes/flashnext-small.yaml
schema: 1
extends: [flashnext]
name: Flash-Next, 4 sequences, no tools
status: experimental
args:
  max-num-seqs: 4
  enable-auto-tool-choice: null
  tool-call-parser: null
```

## 6. Inputs and interpolation

- `${NAME}` in `model`, `env` and `args` strings is replaced by the value of
  input `NAME`.
- Every `${NAME}` must be a declared input. An unknown name fails validation.
- `$${` writes a literal `${`.
- The value comes from `--set NAME=VALUE`, then from `default`.
- A `required` input without a value stops the plan. The message names the
  `--set` to give.
- A `path` input is a host path. rdnactl warns when it does not exist. In
  container mode, rdnactl mounts it at the same path.

## 7. Overrides

`--set KEY=VALUE` changes one value at plan time. It can be repeated.

- A key without a dot sets an input: `--set PLE_QUANT_DIR=/data/ples_int4`.
  The value is a plain string.
- A dotted key sets a field: `--set args.max-model-len=32768`,
  `--set env.GPU_MAX_HW_QUEUES=2`, `--set args.enforce-eager=true`. The
  value is parsed as YAML. `null` deletes the key.
- `set:` in `config.yaml` holds default overrides. rdnactl applies them before
  the `--set` flags. A later override wins.
- Change the GPUs with `--set env.HIP_VISIBLE_DEVICES=4,5,6,7`. Otherwise
  rdnactl selects them (spec §7.7): matching arch, one PCIe switch, lowest
  HIP indices.

## 8. Check your recipe

```sh
rdnactl recipe validate ~/.config/rdnactl/recipes/flashnext-small.yaml
rdnactl recipe show flashnext-small        # the merged recipe
rdnactl recipe plan flashnext-small        # env + command, GPU selection, warnings
rdnactl recipe plan flashnext-small --json # the plan for scripts
```

## 9. Built-in fragments

| Fragment | Content |
|---|---|
| `_base` | Shared environment and fixed flags of the fork launcher (`rdna_init`): spawn workers, no AITER, no hipBLASLt, FP16, TP4, 8 sequences, 2048 batched tokens, 127.0.0.1:18094. Sets the `v0.31.0-extras` image and the `rocm7.14-rocblas5.5` TunableOp profile. |
| `_attn-fa` | FA-RDNA2 attention: `RDNA_ATTN` backend, GQA decode. Removes `FLASH_ATTENTION_TRITON_AMD_ENABLE`. |
| `_attn-triton` | Triton attention (`TRITON_ATTN`), FA-RDNA2 off. |
| `_rccl-pcie` | RCCL over PCIe: P2P level `pxb`, Simple protocol, no MSCCL, fine-grained PCIe memory. |
| `_rdna-ar` | The fork's one-shot all-reduce up to 64 KiB, RCCL above. |
| `_tuned-moe` | `VLLM_TUNED_CONFIG_FOLDER` for the fork's tuned fused-MoE configs. In container mode it is `/src/vllm/tuned-moe` in the image. In host mode rdnactl writes its embedded copy to `<data>/tuned-moe` and uses that directory. |
| `_ple-int4` | Flash-Next PLE n-gram table, INT4, in pinned host RAM. Needs input `PLE_QUANT_DIR`. |
| `_ple-int4-disk` | The INT4 table memory-mapped from disk through the CPU sidecar (no RAM floor). |
| `_ple-exl3` | The PLE table from an EXL3 checkpoint through the CPU sidecar. Removes `PLE_QUANT_DIR`. |
| `_mtp2` | MTP speculative decode with 2 tokens and capture sizes 3, 6, 12, 24. |
| `_flashnext-common` | The shared Flash-Next AWQ-W4A16 profile on 4x gfx1030 (TP4/EP4). |

## 10. Built-in recipes

The 11 recipes come from `tools/rdna/recipes/*.env` of the
`opengfx1030/vllm-rdna` fork. The fork comments are kept in the files.

| Recipe | GPUs | Modes | What it is |
|---|---|---|---|
| `flashnext-mtp2` | 4 | host, container | Flash-Next AWQ-W4A16, MTP=2, TP4/EP4. Wins at 8x16k/1k c=8. Validated. |
| `flashnext-mtp0` | 4 | host, container | Flash-Next AWQ-W4A16, plain decode. Wins at 8x1k/512 c=8. Validated. |
| `flashnext` | 4 | host, container | Flash-Next production agentic serve: tool calling, reasoning parser, 6 sequences. Validated. |
| `flashnext-pp3` | 3 | host | Flash-Next over 3 cards, TP1/PP3, layers 16,16,16, dense INT8 only, text only. Validated. |
| `flashnext-pp3-mtp2` | 3 | host | `flashnext-pp3` with a W4A16 MTP drafter (input `MODEL`), layers 17,17,14. Experimental. |
| `flashnext-dflash2` | 4 | host | Flash-Next with the incoai DFlash2 drafter, 3 speculative tokens. Experimental. |
| `flashnext-exl3` | 4 | host | Flash-Next EXL3 3.05 bpw with the PLE table from the checkpoint. Experimental. |
| `27b-awq` | 4 | host, container | Qwen3.8-27B AWQ-INT4 dense, W4A8 prefill, TP4. Validated. |
| `27b-exl3` | 4 | host, container | Qwen3.8-27B EXL3 3.00 bpw, TP4, 20K context. Validated. |
| `full` | 2 | host, container | Qwen3.8-27B AWQ-INT4, full HIP path, TP2. Validated. |
| `dsv4-flash` | 4 | host, container | DeepSeek-V4-Flash K128 (fp8 + MXFP4 experts), TP4. Needs input `MODEL`. Experimental. |

`rdnactl recipe list` shows the fit of each recipe on this host.

## 11. Differences from the fork launcher

`rdnactl recipe plan` gives the same vLLM flags and kernel environment as
`PRINT=1 bash tools/rdna/serve_rdna.sh RECIPE=<id>`. These differences are on
purpose:

- GPUs: rdnactl selects GPUs of the recipe's arch on one PCIe switch. The
  fork uses `0,1,2,3` by default.
- Caches: rdnactl uses `<cache>/runs/<name>/` per run. The fork uses
  `<tree>/cache/`.
- `PYTHONPATH`: rdnactl does not set it. The venv has vLLM installed. The run
  directory is the working directory.
- `HF_HUB_OFFLINE` and `TRANSFORMERS_OFFLINE`: `0` for a repo id (downloads
  allowed), `1` for a local path. The fork always sets `1`. Set
  `env.HF_HUB_OFFLINE` in a recipe or with `--set` to force a value.
- TunableOp: the plan shows the `PYTORCH_TUNABLEOP_*` variables. The fork
  sets the same variables when it starts, but its `PRINT=1` output omits them.
- `VLLM_PP_LAYER_PARTITION`: the plan shows it. The fork exports it but its
  `PRINT=1` output omits it.
- `VLLM_TUNED_CONFIG_FOLDER`: in host mode it points to rdnactl's copy of the
  same file, not to the fork tree.
- `ROCM_HOME`, `ROCM_PATH`, `HIP_PATH`, `HIP_ROOT_DIR`: rdnactl uses the
  newest ROCm install it detects, as a resolved path.
