# bernie specification

Status: draft 1 (2026-10-28). This document is the contract. Code, tests and
the dashboard follow it. Change this file in the same pull request as the
behavior.

Writing rules for this file: short sentences, one fact or one instruction per
sentence, active voice, defined terms only (see [Terms](#terms)).

## 1. Purpose

bernie helps an operator run vLLM on AMD ROCm GPUs. It replaces the
serve scripts and `.env`
recipes of the `opengfx1030/vllm-rdna` fork with one tool.

bernie does these tasks:

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
  rows, the tuned-MoE configs, the device tables and the dashboard assets. The
  target host needs no Python, Node or Go to run bernie.
- **Read-only by default.** Inspection commands never change the host.
  Commands that change the host print the plan first. bernie never runs
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
  bernie refuses to launch a recipe on GPUs that do not match.
- **Many devices.** Support covers AMD RDNA GPUs from the Radeon PRO V620 and
  RX 6800 XT (RDNA2, gfx103x) to the RX 9070 XT (RDNA4, gfx12) and later.
  Device facts live in data tables, one file per GPU generation
  (`internal/machine/gpu/devices/rdna2.yaml`, `rdna3.yaml`, ...; see
  [§4.4](#44-device-tables)). Tuning data and recipes are keyed by
  architecture. No Go code names a device.
- **Lean, readable, segmented.** See [§2.1](#21-structure-rules).

### 2.1 Structure rules

These rules apply to code, data and docs. A pull request that breaks one
needs a reason in its description.

1. **Lean.** Add nothing that no command, test or recipe uses. Delete dead
   code in the same change that makes it dead.
2. **Readable.** A Go file has at most 400 lines. A function has at most
   about 60 lines. When a file grows past the limit, split it by
   responsibility, not by size alone. A spec or doc page has one topic.
3. **Segmented.** A directory has at most 12 entries (not counting
   `testdata/`, `embed.go` and `README.md`). When a directory would grow
   past 12, split it by a real axis:
   - `internal/` by role: `machine/` (host, gpu, rocm, topo), `serving/`
     (recipe, runner, tuning, vllm, bench) and `support/` (execx, paths,
     config, fixture). Packages that sit on top (`cli`, `flow`, `doctor`,
     `setup`, `logdoc`) stay at the top. Group by role when `internal/`
     passes 12 entries;
   - recipes by model family (`recipes/flashnext/`, `recipes/qwen-dense/`),
     shared fragments by concern (`recipes/_fragments/<concern>/`);
   - tuning data by architecture, then by build
     (`tunableop/gfx1030/<profile>/`, `tuned-moe/gfx1030/`);
   - device tables by GPU generation
     (`internal/machine/gpu/devices/<generation>.yaml`);
   - CLI commands stay one file per command, in a family package
     (`internal/cli/cmds/<family>/cmd_<name>.go`).
4. **Keyed by device.** Data that depends on the GPU is stored under the
   architecture it applies to. A new architecture adds a folder. It never
   edits another architecture's files.
5. **Discovered, not listed.** Embedded trees are found by walking the
   embedded file system. No hand-edited index lists more than a screen of
   entries. Each tuning profile carries its own `profile.json` manifest.
6. **One owner per package.** A package has one responsibility that fits
   in one sentence at the top of its `doc.go` or main file.
7. **Honest fixtures.** A test fixture that was not captured on a real host
   is named `<name>-synthetic.tar.gz` and has a `SYNTHETIC` file at its root.
   `internal/support/fixture/testdata/README.md` says which values are
   captured and which are written by hand.

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
| arch | A gfx target name such as `gfx1030`, or a prefix such as `gfx103`. Data and recipes are keyed by arch. |
| generation | A GPU family: `rdna2`, `rdna3`, `rdna3.5`, `rdna4`. One device-table file per generation. |
| code target | The gfx target whose code objects a GPU runs. It is the GPU's own arch, or the arch of its `hsa_override`. |
| device tables | The embedded YAML files in `internal/machine/gpu/devices/`. They hold the facts of each arch and each PCI device. |

## 3. Command surface

### 3.1 The default: run the binary

The whole product is one file. The operator downloads it and runs it. No
other file, script or compose file is required.

- `bernie` with no arguments runs `bernie status`. It changes nothing.
  It prints the GPUs, the doctor summary, the best recipe for this host,
  the running runs, and the one next command to type.
- `bernie up [RECIPE]` does the full path in one command:
  1. It runs the doctor checks of the categories `gpu` and `kernel`. A `fail`
     stops `up` with the fix commands.
  2. It selects the best recipe that fits (or RECIPE): fit first, then
     `priority`, then id. A recipe with a required input and no value is
     skipped. A named recipe with a missing input stops `up`.
  3. It selects the mode (flag, config, else `container` when the recipe has
     an image and an engine is usable, else `host`).
  4. It plans the setup steps. A pending root step stops `up` and prints the
     commands.
  5. It asks for confirmation once, unless `--yes` is given.
  6. It applies the non-root steps, starts the run and waits until `/health`
     answers (`--wait`, default 30 minutes).
  7. It starts the dashboard on `--listen` (default `127.0.0.1:7860`), unless
     `--no-dash` is given.

  A stop before step 6 ("blocked") prints what the operator must do and
  exits with code 3.
- With `--json`, `status` and `up` write one JSON document. `up --json`
  never prompts; it requires `--yes` to change the host. A blocked `up`
  writes `{"blocked": {"reason": "", "commands": []}}`.

### 3.2 Rules

- `--json` can be anywhere before a `--`. After `--` everything is
  positional and kept verbatim. A `--json` that directly follows a flag
  (`--set --json`) is that flag's value if the flag takes a value. Otherwise
  it is the global flag.
- Explicit help (`--help`, `-h`, `help`) is printed on stdout and exits 0.
- Diagnostics and progress go to stderr.
- Flags and positional arguments can be in any order.
- Exit codes: `0` success, `1` runtime failure, `2` usage error,
  `3` checks failed (the report is still printed, with no error envelope).
- A bad flag or a usage error prints one `bernie: <message>` line on stderr
  and exits 2. With `--json`, it writes the error envelope
  `{"error": "...", "exit_code": 2}` instead.
- With `--json`, a runtime failure writes `{"error": "...", "exit_code": 1}`.

| Command | Changes host | Purpose |
|---|---|---|
| `bernie` / `bernie status` | no | Overview and the next command to type. |
| `bernie up [RECIPE] [--set K=V]... [--mode M] [--venv DIR] [--engine E] [--name N] [--yes] [--no-dash] [--listen ADDR] [--wait D]` | yes | Doctor → setup → best recipe → serve → dashboard. |
| `bernie hw` | no | Hardware and OS inventory. |
| `bernie topo [--bandwidth] [--venv DIR]` | no | PCIe tree, P2P matrix, link classes. Optional bandwidth test. |
| `bernie doctor [--category C]... [--venv DIR]` | no | All checks. Exit 3 if one check fails. |
| `bernie doctor --log FILE\|-` / `doctor --run NAME` | no | Match a vLLM log against known failures ([§5.3](#53-log-doctor)). |
| `bernie setup plan [--mode M]` | no | Ordered install steps for this host. |
| `bernie setup apply [--mode M] [--step ID]... [--yes]` | yes | Run the steps that do not need root. Print the root steps. |
| `bernie recipe list [--all]` | no | Recipes, with fit to this host. `--all` includes recipes that do not fit. |
| `bernie recipe show <id>` | no | The merged recipe (after `extends`). |
| `bernie recipe validate [FILE]...` | no | Validate files, or all loaded recipes. |
| `bernie recipe plan <id> [--set K=V]... [--mode M] [--name N]` | no | The plan, as an exact command and environment. |
| `bernie serve <id> [--set K=V]... [--mode M] [--name N] [--foreground] [--no-pull] [--dry-run]` | yes | Start a run. |
| `bernie ps` | no | Runs, with state and health. |
| `bernie stop [NAME]` | yes | Stop a run (the only run if NAME is absent). |
| `bernie logs [NAME] [-f] [--tail N]` | no | Show the log of a run. |
| `bernie compose <id> [--set K=V]... [-o FILE]` | no* | Write a `compose.yml` for the plan (*writes FILE only). |
| `bernie tune tunableop list` | no | Registered TunableOp profiles. |
| `bernie tune tunableop status [--venv DIR]` | no | Loaded rocBLAS build, matching profile. |
| `bernie tune tunableop env [--venv DIR] [--profile P] [--tuning]` | no* | `export` lines for TunableOp (*materializes rows). |
| `bernie tune power status` | no | Power cap per GPU. |
| `bernie tune power set <WATTS> [--gpu IDX]...` | yes | Set the power cap. Without root, print the command. |
| `bernie bench [--url U] [flags]` | no | Benchmark a server. Save the result. |
| `bernie bench list` / `bench show <ID>` | no | Saved results. |
| `bernie config show` / `config set <KEY> <VALUE>` | yes (config file) | User defaults. |
| `bernie dash [--listen ADDR] [--token T]` | no | Start the dashboard. |
| `bernie version` | no | Version. |

`recipe plan`, `serve` and `compose` share the launch flags: `--set`,
`--mode`, `--name`, `--venv`, `--engine`, `--profile` (force a TunableOp
profile), `--no-tunableop` and `--tuning`. `serve` adds `--foreground`,
`--no-pull` and `--dry-run` (same output as `recipe plan`). `compose` writes
container plans only. `setup plan` and `setup apply` also take `--venv`,
`--image` and `--model`. `status` takes `--venv`.

## 4. Host inspection (`hw`, `topo`)

### 4.1 Sources

bernie reads sysfs and procfs first. It runs external tools only to add
information. A missing tool is a `skip`, not an error.

| Data | Source |
|---|---|
| GPUs, gfx target, HIP index | `/sys/class/kfd/kfd/topology/nodes/*/properties` (`gfx_target_version`, `location_id`, `drm_render_minor`). Nodes with `simd_count > 0` are GPUs. |
| GPU name | The DRM `product_name`. Consumer cards have none: the device tables by PCI device id ([§4.4](#44-device-tables)), else `AMD GPU <gfx>`. |
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
| `PHB` | The path crosses one root complex (host bridge), or root complexes on the same NUMA node. |
| `SYS` | The path crosses NUMA nodes (CPU interconnect). |

Two rules keep the classes meaningful. A switch inside a GPU card (the AMD
bridge pair on Navi cards) does not count as a switch. The two internal
levels of a Broadcom PEX88096 count as one switch.
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

### 4.4 Device tables

Device facts live in embedded YAML files, one per GPU generation:
`internal/machine/gpu/devices/<generation>.yaml`. The file stem equals the
`generation` value. The loader walks the folder. A malformed file is a build
defect: the tests catch it.

```yaml
generation: rdna2          # equals the file stem
wave_size: 32
triton_below: "3.9"        # optional: Triton from this version dropped the generation
arches:                    # gfx targets of this generation
  gfx1030: {family: navi21, bf16: none}
  gfx1031: {family: navi22, bf16: none, hsa_override: "10.3.0"}
  gfx1035: {family: yellow_carp, bf16: none, hsa_override: "10.3.0", integrated: true}
devices:                   # PCI device ids, lower-case 0x%04x
  - {ids: ["0x73a1"], name: Radeon PRO V620, gfx: gfx1030, power_cap_advice_w: 180}
```

| Field | Meaning |
|---|---|
| `bf16` | `native` (BF16 matrix instructions), `none`, or absent when unsourced. |
| `hsa_override` | The `HSA_OVERRIDE_GFX_VERSION` that makes the arch run the code objects of a sibling arch of the same file. |
| `integrated` | The arch is an APU. It shares memory with the CPU. |
| `flash_attn` | The arch is on the FlashAttention ROCm allowlist. |
| `power_cap_advice_w` | Recommended power cap of this device. Optional, only with a source. |

Rules:

- An arch name and a device id are unique across all files.
- A device names an arch of its own file.
- Every file lists its sources in a header comment.
- A value without a source is left out. Absent means "unknown", not "no".

Go code reads the tables only through `gpu.Lookup(deviceID)`,
`gpu.LookupArch(gfx)` and `gpu.CodeTargets(gfxs)`. These users exist:

| User | What it takes |
|---|---|
| `gpu.List` | The device name when the DRM node has no `product_name`. |
| `doctor` | `gpu.supported.<gfx>`, `gpu.hsa_override.<gfx>`, `gpu.igpu`, the Triton check, the power-cap advice. |
| `setup` | Code targets (wheels, build targets), the FlashAttention allowlist. |

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
| `gpu` | `/dev/kfd` exists and is readable/writable (`gpu.kfd`). `/dev/dri/renderD*` exist. User is in `video` and `render`. At least one GPU. One `gpu.supported.<gfx>` per gfx target on the host: `ok` when the target is in the device tables and a validated recipe lists it, `warn` when the target is in the tables but no validated recipe lists it, `warn` ("not in the device tables") when the tables do not know it (for example a CDNA target). One `gpu.hsa_override.<gfx>` (`warn`) per target whose table row has an `hsa_override`, with the `HSA_OVERRIDE_GFX_VERSION` advice. `gpu.igpu` warns when an integrated GPU mixes with discrete GPUs. |
| `kernel` | `amdgpu` is loaded. IOMMU mode from `/proc/cmdline` (`iommu=pt` advice for P2P). PCIe links are gen4 x8 or better. |
| `rocm` | ROCm installs found (`/opt/rocm*`, `/opt/rocm/core-*`, `/usr`) with versions. More than one install: `warn`. `rocminfo` lists the GPUs. `hipcc`, `amd-smi` or `rocm-smi` present. `librocblas.so.5` found. ROCm 7.2.1 to 7.13 with more than one GPU: `warn` (RCCL TP>1 deadlock range). |
| `python` | With `--venv` (or the configured venv): Python version, `torch` imports, `torch.version.hip` set, GPU count matches, `rocm-sdk-device` wheels match the GPU code targets, Triton is below the version that dropped a GPU generation (`triton_below` in the device tables; `fail`), `vllm` imports, fork ops registered (`_rocm_C::*rdna*` schema count > 10). |
| `container` | `docker` or `podman` present, daemon or socket usable, compose present, rootless state, SELinux mode. |
| `resources` | RAM. Free disk in the Hugging Face cache. `/dev/shm` size. |
| `tuning` | TunableOp profile matches the loaded rocBLAS. Power cap per GPU: `warn` when a cap is above the advice of the device table ([§9.2](#92-power)). |

### 5.3 Log doctor

`doctor --log FILE|-` and `doctor --run NAME` match a vLLM log against a
catalog of known failures. `-` reads standard input. `--run` reads the `log`
file of the run record (`<state>/runs/<name>/run.json`, `serve.log` in the run
directory when that file is gone). For a container run it reads the engine's
`logs` output. `--log` and `--run` exclude each other and `--category` and
`--venv`. With either flag, `doctor` prints only the log findings and runs no
host checks.

Each rule has an id (`<family>.<slug>`), a severity (`error` or `warn`), a
regular expression (Go RE2, matched per line), a cause, a fix and a `doc`
link. The catalog is data (embedded YAML, `internal/logdoc/catalog.yaml`), not
code. The report lists each matched rule once, with the first matching line
and its line number. Lines are matched up to 1 MiB; a log can be any size.

```json
{"matches": [{"id": "memory.kv-too-small", "severity": "error", "pattern": "...",
              "cause": "", "fix": "", "doc": "https://...", "line": "", "line_no": 82}]}
```

`matches` is `[]` when nothing matched. Exit code 3 when at least one rule
matches. `serve` and `up` do not scan the log of a run that exits during
startup. `up` reports the exit and `bernie logs NAME`. The operator then
runs `doctor --run NAME`.

## 6. Setup

`setup plan` returns ordered steps. The steps read the GPU targets and the
device tables ([§4.4](#44-device-tables)) to choose wheels and build targets.
Each step has this record:

```json
{"id": "venv.create", "title": "", "mode": "host|container|both",
 "root": false, "done": false, "commands": ["..."], "why": ""}
```

`done` is true when the step's postcondition already holds (bernie checks it).
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
| `venv.torch` | no | host | Install `rocm-sdk`, `torch` and one `rocm-sdk-device-<gfx>` + `amd-torch-device-<gfx>` wheel per code target from the AMD multi-arch index `https://repo.amd.com/rocm/whl-multi-arch/`. The code target of a GPU is its gfx target, or the target of its `hsa_override` ([§4.4](#44-device-tables)): gfx1031 to gfx1036 use the gfx1030 wheels. |
| `causal-conv1d.build` | no | host | Build causal-conv1d from source for the host GPU targets. |
| `flash-attn.build` | no | host | Only when a GPU has `flash_attn: true` in the device tables. gfx1030 does not. |
| `vllm.build` | no | host | Clone the fork and build it in the venv. |
| `image.pull` | no | container | Pull the recipe image. |
| `model.download` | no | both | `hf download <repo>` into the Hugging Face cache. |

## 7. Recipes

### 7.1 Files and lookup

A recipe is one YAML file. The file name stem is the recipe id. bernie loads
recipes in layers, in this order:

1. Built-in recipes (embedded from `recipes/`).
2. `<config>/recipes/`.
3. Each directory in `$BERNIE_RECIPES` (`:`-separated).
4. The files and directories given to `recipe validate`.

Rules:

- Each directory is read recursively. Every `*.yaml` (and `*.yml` outside the
  built-in tree) below it is a recipe. Folders only group files. The folder
  never changes the id.
- An id is unique inside one layer. Two files with the same id in one layer
  are a load error. The error names both paths. The first file stays.
- A later layer replaces an earlier recipe with the same id.


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

When the plan has no `HIP_VISIBLE_DEVICES`, bernie selects GPUs:

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
- Venv: `--venv`, then `$BERNIE_VENV`, then config `venv`, then `$VIRTUAL_ENV`.
- `LD_LIBRARY_PATH` starts with the venv's ROCm wheel libraries when present:
  `_rocm_sdk_libraries/lib`, `_rocm_sdk_core/lib`,
  `_rocm_sdk_core/lib/{host-math,rocm_sysdeps,core}/lib`,
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
<engine> run -d --name bernie-<name> --label bernie.run=<name>
  --network host --ipc host --shm-size <shm>
  --device /dev/kfd --device /dev/dri --group-add video --group-add render
  --security-opt seccomp=unconfined --security-opt label=disable
  -v <hf-cache>:/root/.cache/huggingface
  -v <tunableop rows>:/bernie/tunableop:ro
  -v <tuned-moe dir>:/bernie/tuned-moe:ro   (when the recipe sets VLLM_TUNED_CONFIG_FOLDER)
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

`ps` state is derived from the process (or container) and `/health`. The
state is not stored in `run.json`:

| State | Meaning |
|---|---|
| `starting` | The process or container is alive. `/health` does not answer 200. |
| `running` | The process or container is alive. `/health` answers 200. |
| `exited` | The process or container is gone, whether `stop` ended it or it failed. |

`doctor --run NAME` explains a failed start ([§5.3](#53-log-doctor)). One
name has one run. `serve` refuses a name that is not `exited`.

## 9. Tuning

### 9.1 TunableOp

- Rows are build-specific and architecture-specific. A profile is valid only
  for the rocBLAS build (sha256 of `librocblas.so.5`) that produced it and for
  the GPU architectures in its manifest.
- Profiles live in `tunableop/<arch>/<profile>/`, embedded in the binary. Each
  profile has a `profile.json` manifest (`name`, `arch`, `rocblas`, `rocm`,
  `torch`, `lib_sha256`, `rows_per_rank`, `status`, `covers`) and its
  `tunableop_results<rank>.csv` files. There is no central registry: bernie
  discovers profiles by walking the embedded tree. `name` equals the folder
  name and is unique across architectures. `arch` lists gfx names or prefixes
  (`gfx1030`, `gfx103`, `gfx11`).
- `env` and `serve` materialize the rows into `<data>/tunableop/<profile>/`.
- Host mode: bernie hashes the loaded `librocblas.so.5` (venv wheel first,
  then `/opt/rocm/lib`) and selects the profile with the same sha256 prefix
  (12 hex) whose `arch` names the architecture of every selected GPU. An
  explicit profile must match both the hash and the architecture, else it is
  an error.
- No match: per-user rows at `$HOME/.cache/tunableop/tunableop_results.csv`.
- Container mode: the recipe names the profile for its pinned image. The
  profile must still fit the selected GPUs.
- Lookup only by default (`PYTORCH_TUNABLEOP_TUNING=0`). `--tuning` enables
  tuning into the per-user file.
- Environment set: `PYTORCH_TUNABLEOP_ENABLED=1`, `PYTORCH_TUNABLEOP_TUNING`,
  `PYTORCH_TUNABLEOP_HIPBLASLT_ENABLED=0`, `PYTORCH_TUNABLEOP_FILENAME`.
- Tuned fused-MoE configs follow the same keying: `tuned-moe/<arch>/` holds a
  flat folder of vLLM's `E=...json` files. A folder name is a gfx name or a
  prefix; the longest name that prefixes the GPU's gfx wins. Host mode sets
  `VLLM_TUNED_CONFIG_FOLDER` to the materialized folder of the selected GPUs'
  architecture (`<data>/tuned-moe/<arch>`). An architecture without a folder,
  and a selection that spans several architectures, gets no variable.

### 9.2 Power

`tune power status` reads `power1_cap`, `power1_cap_min`, `power1_cap_max`.
`tune power set` writes `power1_cap` (microwatts) when it runs as root. Else
it prints the exact command. The advice comes from the device tables
([§4.4](#44-device-tables)): a row with `power_cap_advice_w` gives the
recommended cap of that device (V620: 180 W). `doctor` compares each GPU's
cap with it. A device without that field has no advice.

## 10. Benchmark

`bernie bench` sends streaming requests to `/v1/completions`.

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
With MTP, a chunk can carry several tokens. bernie divides the gap of a
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

`bernie dash` serves an embedded web UI and a JSON API. It listens on
`127.0.0.1:7860` by default. On a non-loopback address, every request needs
the token (`Authorization: Bearer <t>` or the cookie set by `/?token=<t>`).
The token is `--token`, else `$BERNIE_DASH_TOKEN`, else a random token
printed at start. POST and DELETE requests also need the header
`X-Bernie: 1`. On a loopback address the `Host` header must be a loopback
name (this blocks DNS rebinding). Errors are `{"error": "...", "exit_code": N}`.

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
| GET | `/api/runs/{name}/logs?tail=N` | Server-sent events, one line per event (`tail` default 200). Ends with an `end` or `error` event. |
| GET | `/api/telemetry` | Server-sent events, 1 Hz: `{time, gpus: [Sample], runs: [{name, url, state, stats, gen_tok_s, prompt_tok_s, error}]}` |
| GET | `/api/bench` | `bench list` JSON |
| POST | `/api/bench` | bench flags as JSON (`concurrency` is a list or `"1,8"`) → result. One bench at a time; a second gets 409. |

## 12. Files and paths

| Name | Default | Override |
|---|---|---|
| config | `~/.config/bernie` | `$BERNIE_CONFIG_DIR`, `$XDG_CONFIG_HOME` |
| state | `~/.local/state/bernie` | `$BERNIE_STATE_DIR`, `$XDG_STATE_HOME` |
| data | `~/.local/share/bernie` | `$BERNIE_DATA_DIR`, `$XDG_DATA_HOME` |
| cache | `~/.cache/bernie` | `$BERNIE_CACHE_DIR`, `$XDG_CACHE_HOME` |

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
