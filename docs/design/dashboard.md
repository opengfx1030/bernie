# Dashboard v3 — the Dashboard tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **is it serving, and how fast?**

This tab carries our own surface: the fleet tiles, the per-GPU cards with
their ring and their meters, the power caps, the run card, the topology and
the known problems. It is drawn in the same visual language as the other
tabs.

Wireframes: `design/dashboard-1280.svg` (a box that serves),
`design/dashboard-390.svg` (the same at 390 px),
`design/dashboard-empty-1280.svg` (nothing serves).

## The order of the page

1. The state headline and the actions.
2. The hero: the output speed of the live run on the left, the fleet tiles
   on the right.
3. The section `THE CARDS`: one card per GPU.
4. The section `THE LAST MINUTES`: the four area charts.
5. The section `THE RUN`: the run card.
6. The section `THE POWER CAPS`: the caps of the cards.
7. The section `THE TOPOLOGY`: the tree, the link classes and the
   readiness checklist.
8. The section `KNOWN PROBLEMS`: the doctor findings.
9. The section `ADVANCED`.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline | `GET /api/telemetry` → `runs[]`; `GET /api/runs` | every sample (1 Hz) | `NOTHING IS SERVING.` and `Start a model server on this host, then this tab fills with its numbers.` | `THE STATE OF THIS BOX IS UNKNOWN.` plus `Try again` |
| 2 | The hero number: the output speed of the live run | `GET /api/telemetry` → `runs[].gen_tok_s`; the sparkline from `GET /api/gpus/history?window=5m` (G4) | every sample; the sparkline is appended from the same stream | `– tok/s` and `No run is serving.` | `– tok/s` and `The telemetry stream is down.` |
| 3 | The tile: the load of the fleet | `GET /api/telemetry` → `gpus[].busy_pct`, the mean over the cards | every sample | `–` | `–` |
| 4 | The tile: the power of the fleet | `gpus[].power_w`, the sum | every sample | `–` | `–` |
| 5 | The tile: the memory of the fleet | `gpus[].vram_used` and `vram_total` | every sample | `–` | `–` |
| 6 | The tile: the hottest card | `gpus[].temp_c`, the maximum, with the name of the card | every sample | `–` | `–` |
| 7 | The tile: the highest clock | `gpus[].sclk_mhz`, the maximum | every sample | `–` | `–` |
| 8 | The tile: the tokens per second of the live run | `runs[].gen_tok_s` | every sample | `– tok/s` | `– tok/s` |
| 9 | The tile: the running requests | `runs[].stats.requests_running` and `requests_waiting` | every sample | `0` and `No run is serving.` | `–` |
| 10 | The tile: the KV cache | `runs[].stats.kv_cache_usage` (a fraction from 0 to 1) | every sample | `–` and `The server does not publish its cache.` | `–` |
| 11 | The card of one GPU | `GET /api/telemetry` → `gpus[]`; `GET /api/hw` → `gpus[]` for the name, the PCI address and the limits | every sample | the card is hidden | the card shows `–` for every value |
| 12 | The ring of a card | `gpus[].busy_pct` | every sample | `–` | `–` |
| 13 | The memory meter of a card | `gpus[].vram_used` and `vram_total` | every sample | `–` and an empty track | `–` |
| 14 | The power meter of a card | `gpus[].power_w` and `power_cap_w` | every sample | `–` | `–` |
| 15 | The temperatures of a card | `gpus[].temp_c`, `temp_junction_c`, `temp_mem_c`; the limits from `GET /api/hw` → `gpus[].temp_limits_c` | every sample | `–` | `–` |
| 16 | The clocks of a card | `gpus[].sclk_mhz` and `mclk_mhz` | every sample | `–` | `–` |
| 17 | The ECC state of a card | `GET /api/doctor` → the check whose id is `gpu.ecc.<hip index>` | on open and on `Re-run` | `unknown` and `The doctor has not checked the ECC counters yet.` | `unknown` and the reason of the error |
| 18 | The four area charts | `GET /api/gpus/history?window=5m\|15m\|1h` (G4) | once per window change; the points are appended from the telemetry stream | `No samples yet.` and `The chart fills after the first minute.` | `The history could not be read.` plus `Try again` |
| 19 | The run card | `GET /api/runs` → the run records; `GET /api/telemetry` → `runs[]` for the live numbers | every sample | `No run is serving.` and `A run is a model server on this host: it loads the model and then answers requests.` plus the command `bernie serve <recipe>` | `The runs could not be read.` plus `Try again` |
| 20 | The phase of a run | `runs[].phase` (the phase sentence of spec §5.4) | every sample | `–` | `–` |
| 21 | The endpoint of a run | `GET /api/runs` → `url` and `port` | every sample | `–` | `–` |
| 22 | The age of a run | `GET /api/runs` → `started_at` | every sample | `–` | `–` |
| 23 | The buttons of the run card | `DELETE /api/runs/{name}`, `POST /api/runs` with the same recipe | on demand | the buttons are hidden | the buttons are disabled |
| 24 | The `Try it` box of the run card | `POST /api/smoke` (G3) | on demand | the box is disabled with `Start the server first.` | the box shows `The server did not answer.` and the reason |
| 25 | The findings of an exited run | `GET /api/runs` → `findings[]` | every sample | `No known failure in this log.` | `The findings could not be read.` |
| 26 | The power caps | `GET /api/power` → the service, the desired value, the effective value, the cap and the drift of every card | on open and after a write | `The power service is not installed.` and the command `bernie tune power service install-plan` | `The power caps could not be read.` plus `Try again` |
| 27 | The write of a power cap | `PUT /api/power` with `{"gpu": "all", "watts": 180}` | on demand | the field is disabled | the toast says `The cap was not saved: <reason>` |
| 28 | The PCIe tree | `GET /api/topo` → `nodes[]` | on open | `There is no PCIe tree to show.` and `The tree is the way the cards are wired to each other. It needs at least one GPU.` | `The topology could not be read.` plus `Try again` |
| 29 | The matrix of the link classes | `GET /api/topo` → the links and their class | on open | the matrix is hidden | the matrix is hidden |
| 30 | The readiness checklist | `GET /api/topo/check` → `ready`, `summary`, `checklist[]` | on open and on `Re-check` | `The checklist is not available.` | `The checklist could not be read.` plus `Try again` |
| 31 | The known problems | `GET /api/doctor` → the checks whose status is `fail` or `warn` | on open and on `Re-run` | `Nothing is wrong on this host.` and `Every check of the host passed.` | `The doctor could not run.` plus the command `bernie doctor` |
| 32 | The Advanced panel | `/api/hw`, `/api/topo`, `/api/topo/check`, `/api/doctor`, `/api/runs`, `/api/power` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The state headline

| State | The headline | The lede |
|---|---|---|
| A run answers | `THIS BOX IS SERVING.` | `The model answers on port 18096 at 56.1 tok/s.` |
| A run loads | `THE SERVER IS LOADING THE MODEL.` | the phase sentence of the run, for example `Loading the model weights, 2 of 5 shards` |
| A run does not answer | `THE SERVER DOES NOT ANSWER.` | `The process is alive but /health stays silent. The log says why.` |
| No run | `NOTHING IS SERVING.` | `Start a model server on this host, then this tab fills with its numbers.` |
| No GPU | `NO AMD GPU ON THIS MACHINE.` | `bernie reads the cards from /sys/class/kfd. The amdgpu driver has to be loaded and /dev/kfd has to exist.` plus the command `sudo modprobe amdgpu` |

## The fleet tiles

Eight tiles, in this order: the load, the power, the memory, the hottest
card, the highest clock, the tokens per second, the running requests and the
KV cache. Every tile has a label, a number with its unit, a sub-line and a
sparkline of the last minute.

| Tile | The number | The unit | The sub-line |
|---|---|---|---|
| The load | the mean of `busy_pct` over the cards | `%` | `the mean of 8 cards` |
| The power | the sum of `power_w` | `W` | `of 1400 W allowed` |
| The memory | the sum of `vram_used` | `GiB` | `of 240 GiB` |
| The hottest card | the maximum of `temp_c` | `°C` | the index and the name of the card (`GPU 7 · AMD RADEON PRO V620 Azure`) |
| The highest clock | the maximum of `sclk_mhz` | `MHz` | the index and the name of the card |
| The tokens per second | `gen_tok_s` of the live run | `tok/s` | `1-min avg 44.0 tok/s` |
| The running requests | `requests_running` | | `2 waiting` |
| The KV cache | `kv_cache_usage` | `%` | `of the cache of the engine` |

A tile that the server does not publish shows `–` and says so in its
sub-line. The tile is never `0` for a value that was not measured.

## The card of one GPU

Each card holds, in this order: the name of the card and its PCI address,
the ring of the load with the caption `GPU 0`, the memory meter, the power
meter with the cap tick, the temperatures (the edge, the junction and the
memory), the clocks (the shader and the memory), the ECC state as a pill,
and the sparkline of the last minute.

The card of a card that is hot says so in words: the temperature is drawn in
`--serious` and the caption reads `hot: the limit of this card is 100 °C`.
A card that does not publish the junction or the memory temperature shows
`–` for it.

## The charts of the last minutes

Four area charts side by side: the load, the power, the memory and the
tokens per second. One segmented control above them picks the window: `5
min`, `15 min`, `1 h`. The control is a segmented control, not a select.

The charts are drawn from the history of G4. The window changes the read
once; after that the points are appended from the telemetry stream.

## The run card

| Part | What it holds |
|---|---|
| The name | the name of the run and its state pill |
| The phase | the phase sentence of the log |
| The endpoint | the URL of the server, in a command block with a copy button |
| The age | how long it has been running |
| The buttons | `Stop` and `Restart` as ghost buttons |
| The `Try it` box | one prompt, one answer, the time to first token and the tokens per second |
| The findings | the finding cards of the log doctor, with their fix |

The primary action of the tab is the `Try it` box of the run card when a run
answers. Without a run, the tab has no primary action and says so.

## The topology section

Three blocks side by side: the PCIe tree, the matrix of the link classes and
the readiness checklist of `bernie topo --check`.

The matrix is a heat map: 2 px gaps, 6 px radius cells, one colour per
class (`PIX`, `PXB`, `PHB`, `SYS`), and a dot in a cell where a KFD peer
link exists. Every row and every column carries the name of the card, so
the matrix is readable as text.

The checklist is a list of rows: the status pill, the title, the detail and
the fix command with a copy button. The section opens with the verdict in
words: `Multi-GPU copies will work on this host.` or `Multi-GPU copies will
not work: 2 checks failed.`
