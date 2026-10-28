# Roadmap

Status: draft 2 (2026-10-28).

Rule: bernie stays lean and simple, and keeps what really helps users. A
feature earns a place if it gets a new user from a bare machine to a working
server and a connected client, or keeps results honest. One command does the
job. No wizard, no new product surface.

Each item has a spec section and one test that proves it.

## Next: make `up` work the first time

| # | Item | Spec | Test |
|---|---|---|---|
| 1 | **Fit in recipe data.** A recipe lists `download_gb`, `vram_gib`, `ram_gib`, `kv_bytes_per_token`. `recipe list` shows `fits`, `tight: <reason>` or `no: <reason>`. | §7.2, §7.8 | `recipe list --json` on a fixture host: each recipe has `fit.verdict`; `fit.reasons` is non-empty unless `fits`. |
| 2 | **Plan shrinks to fit.** `recipe plan` lowers `max-model-len` (32K, 16K, 8K) until the KV budget fits free VRAM. Each change is in `plan.adjustments[]` with a reason. A `--set` value is never changed. | §7 | `recipe plan --json` on a 16 GB fixture shows the adjustment; `--set args.max-model-len=8192` stays 8192. |
| 4 | **Ready means answers.** A run is `ready` only after `/health` and a one-token completion succeed. | §3.1, §8.3 | A run with the model unloaded stays `starting` and says why. |
| 5 | **Log doctor on failure.** When a run exits during startup, `serve` and `up` print the matched rule: id, cause, fix. | §5.3 | A fixture log with a known error prints that rule. |
| 6 | **Next command in every state.** `status --json` has `next` for: no GPU, no recipe fits, recipe fits, run starting, run ready. | §3.1 | Fixture set covers the five states. |

## Then: honest results and connected clients

| # | Item | Spec | Test |
|---|---|---|---|
| 7 | **Model file hashes.** A result stores the hashes of the model files it measured, so a number can be tied to the exact weights. | §10 | A result of a run holds the hashes of the model files. |
| 8 | **Redacted report.** `doctor --report` writes JSON with no hostname, user name, home paths or keys, and lists what it removed. | §5 | `grep` for user, host and `/home/` finds nothing. |
| 9 | **Smoke chat.** `bernie chat --run NAME "prompt"` sends one prompt, prints the reply, time to first token and tok/s. It proves the server works. | §3 | Prints reply, TTFT and tok/s for a fixture server; exits non-zero when the run is not ready. |
| 10 | **`connect <app>`.** Prints the ready-to-paste config for opencode, Open WebUI or a generic OpenAI client, using the run's port and model name. It prints only; it writes nothing. | §3 | `connect opencode` output contains the run's base URL and served model name. |

## Shipped

- **ECC check.** The doctor check `gpu.ecc.<hip index>` reads the RAS
  counters of every card: `ok` when ECC is on and the card counted no
  memory error, `warn` when ECC is off on a card that has it or when the
  card corrected errors, `fail` when the card counted uncorrectable ones.
  The counters come from sysfs, `amd-smi` is the fallback. Spec
  [§5.2](spec.md#52-required-checks).
- **Junction and memory temperature, clocks.** The telemetry sample carries
  the temperature of the memory junction, the temperature of the memory and
  the shader and memory clocks of every card. The GPUs tab shows them with
  their units and the word `hot` when a card reaches the limit of its
  device table. Spec [§11](spec.md#11-dashboard).
- **Progress words.** One function turns the tail of a setup or serve log into one
  sentence ("Loading the model weights, 2 of 5 shards"). `serve` and `up`
  print it when it changes, `ps --json` and `status --json` carry the phase
  of a live run, and the Runs tab shows the sentence under the state badge.
  Spec [§5.4](spec.md#54-log-phase).
- **P2P readiness checklist.** `bernie topo --check` and the doctor category
  `topo.p2p` answer whether multi-GPU peer-to-peer copies and the custom
  all-reduce work on this host, and say why when they do not. The Topology
  tab shows the same checklist. Spec
  [§4.5](spec.md#45-p2p-readiness-checklist).
- **MCP server.** `bernie mcp` on stdio and `POST /mcp` on the dashboard,
  read-only by default, write tools behind `--allow-write` and a confirm
  token. It reuses the `--json` code paths. Spec [§13](spec.md#13-model-context-protocol),
  how-to [mcp.md](mcp.md).
- **Pass key recall.** `bernie bench --recall` hides a short random fact in
  every prompt and checks the reply for it. A cell that misses the fact has
  broken attention, and its speed numbers mean nothing. Spec
  [§10](spec.md#10-benchmark).
- **Bench workload.** A result stores its `workload` (`protocol_id`,
  `corpus_sha256`, input, output, concurrency, recall) and `bernie bench
  compare` refuses two results of different workloads. Spec
  [§10](spec.md#10-benchmark).
- **Bench metrics.** Every cell reports five groups of numbers: the prefill
  and the decode speed (the rate of the whole cell and the distribution of
  the rates of its requests), the time to first token in seconds, the
  inter-token latency in milliseconds and the output rate. The server's own
  numbers from `/metrics` come next to them in `server_side`. The text, the
  dashboard and the MCP tools show the same numbers with their units.
  Spec [§10](spec.md#101-what-the-numbers-mean).
- **Numeric GPU group ids.** A container run adds the numeric ids of the
  GPU groups of the host, never their names: the image has no /etc/group to
  resolve a name in. The plan shows the ids and where they came from. Spec
  [§8.2](spec.md#82-container-mode).
- **Resumable model download.** The model step fetches the files of a
  Hugging Face repo into the cache itself: a partial file continues with a
  `Range` request, the checksum of every file is checked, and the progress
  is one line with a speed. Spec [§6.1](spec.md#61-the-model-download).
- **Power cap service.** `tune power set` writes the desired caps to
  `<config>/power.yaml`, `tune power apply` writes them to hwmon as root, and
  `tune power service` prints the systemd unit pair that applies the file at
  boot and on every change. The GPUs tab shows the cap, the allowed range,
  the desired value and the drift, and writes a new desired value. Spec
  [§9.2](spec.md#92-power).

## Not planned

These add surface without helping first-run success. Reopen only with a
concrete user request.

- Dashboard chat page. The `chat` command covers the check.
- `connect --write` and self-check. Print-only first.
- Demo mode for the dashboard.
- "Did you mean" suggestions beyond what the flag parser prints.
- A separate privacy page. The spec's "files and paths" section covers it.
- Download byte counts in plans. Progress words cover the wait.
- Per-recipe `sources` field.

## What bernie keeps as its own

- It runs vLLM, with tensor-parallel across cards, topology and P2P tools.
- Recipes are plain YAML that anyone can add.
- CLI first. The dashboard is a view of the same data.
- It never becomes an inference engine. Engine work goes to the fork.
