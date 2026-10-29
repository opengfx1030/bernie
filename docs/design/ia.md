# Information architecture

Status: design proposal, 2026-10-29. Nothing in this file is implemented yet.

This file is the contract for the *shape* of bernie: which capability lives
where, what a tab is for, what "more information" means, and the words the
three front ends use. The behaviour of the product is [spec.md](../spec.md).
The visual design is [../design-dashboard.md](../design-dashboard.md) and the
component sheets in this folder.

Read it with:

- [../spec.md](../spec.md) — the contract for the commands, the API and MCP.
- [../../dashboard/README.md](../../dashboard/README.md) — the tab table of the
  page as it is today.
- [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md) — the rules of the
  page.
- [../design/rules-ui.md](../rules-ui.md) — the reasoning behind the rules.

## Open decisions

Five decisions the owner has to make. Each one has a recommendation. The
rest of this file assumes the recommendation.

| # | Decision | Recommendation | What it costs to change it later |
|---|---|---|---|
| D1 | **What is the Advanced tab?** Today it is the first bernie dashboard (the old layout) kept for comparison. The page also has a per-tab `Advanced` disclosure and a rail-wide `Show details` switch. | Keep the **Advanced tab** as the expert/operations view: everything the simple tabs hide, in one place, plus the raw documents of every endpoint. Keep the per-tab **Show details** toggle as *more detail about the same thing* on that tab. The two jobs are different, so both stay — but the **Advanced tab stops being a copy of the Dashboard tab**: it becomes the operator's console (power caps, the full per-GPU table, the raw documents, the run record detail). The old first-dashboard layout is retired from the tab and kept only as the raw documents it produced. | Renaming or removing the tab later is cheap (one `VIEWS` entry). Rebuilding it is not. |
| D2 | **Chat and Fleet stay as placeholders?** The owner ordered them present-but-not-usable. | Keep the **Fleet** tab as a placeholder for one more release and mark it *planned*; ship a **lean Chat** now: one prompt box that posts to the running run and streams the reply (`POST /api/smoke` already exists; a streaming variant is ~1 day). A placeholder tab that says "planned" forever is a dead tab; a working one-prompt Chat is the smallest thing that is not a promise. See [§6](#6-chat-and-fleet-the-two-placeholder-tabs). | Low. A placeholder tab can become real at any time. |
| D3 | **Does the Dashboard tab keep both a simple view and an advanced view?** Today the Dashboard tab has a hand-written *simple* view (`views/dashboard/simple.js`) and the old layout lives in the Advanced tab. | **Yes, keep both, with one rule**: the Dashboard tab's simple view is the default and shows the one question (is it serving, how fast); its `Show details` toggle reveals the per-GPU table, the history window control and the power read-outs for the *same* run. The Advanced tab keeps the *cross-cutting* things: the power **writes**, the raw JSON of every endpoint, the job list and the doctor report in full. | Medium. The Dashboard and Advanced views share `components/charts.js`. |
| D4 | **"Recipe" or "profile"?** The word *profile* is used today for a TunableOp profile (`tune tunableop`), and *recipe* for a model+flags+mode. The reference uses *profile* for the host profile. | **Recipe** stays the word for a model. **TunableOp profile** keeps its name and is always written with its qualifier. The word *profile* alone is retired from the UI, because the reference uses it for the host. | Low, but it touches copy in four places. |
| D5 | **Device access for containers: the classic group setup or the AMD Container Toolkit (amd-ctk) CDI?** Today the container plan always passes `--device /dev/kfd --device /dev/dri` and `--group-add <gid>` (`internal/serving/runner/command.go:156`, `:324`). | Support **both**, chosen by what the host has: if `amd-ctk` and a CDI spec are present, use the CDI device list and no group flags; else the classic devices and numeric group ids. Decide it on the host, print which one was chosen in the plan, and let `--device-access` override it. The alternative (classic only) means the setup path fails on a CDI-only host; the alternative (CDI only) breaks every host that has no amd-ctk. | Medium-high. It touches the container plan, the compose file, the doctor `container` category and the `access` onboarding step. |

---

## 1. The shape of the product

bernie is one binary with three front ends:

| Front end | What it is | Who reads it |
|---|---|---|
| CLI | `bernie` and its 19 commands | a person in a terminal, a script |
| MCP | 25 tools over stdio and `POST /mcp` | an agent |
| Dashboard | 8 tabs, a JSON API and 4 event streams | a person in a browser |

The rule that keeps them one product is in spec §13.1: *MCP adds no feature of
its own: every tool calls the code of a CLI command and returns the same JSON
document as that command with `--json`.* The same holds for the dashboard: it
is a view of the CLI, never a second implementation.

So the information architecture below is really one question asked three
times: **what is the smallest set of questions a user has, and where does each
question live?**

The answer today, and the answer this file proposes, is the same shape:

```mermaid
flowchart TD
  A[Bernie] --> B[CLI]
  A --> C[MCP]
  A --> D[Dashboard]
  B --> S[Setup: make this box serve]
  B --> T[Dashboard: is it serving and how fast]
  B --> M[Benchmark: how fast and does it read the prompt]
  B --> G[Topology: can the cards copy to each other]
  B --> L[Logs: what did the server say]
  B --> X[Advanced: the operator's console]
  D --> S
  D --> T
  D --> M
  D --> G
  D --> L
  D --> X
```

## 2. The vocabulary

One word, one meaning, in all three front ends. The words below are the ones
that exist today; the ones marked **today: X** are the collisions to fix.

| Word | Means | Never means | Where it lives today |
|---|---|---|---|
| **host** | the machine bernie runs on | a container | CLI `--root`, spec §12 |
| **container** | the run inside a Docker or Podman container | the host | `recipe.ModeContainer` (`internal/serving/recipe/plan.go:18`) |
| **run** | one served model: a process or a container, with a name | a job | `<state>/runs/<name>/run.json`, spec §8.3 |
| **job** | one background action bernie runs now (a setup step) | a run | `<state>/jobs/<id>`, spec §3.3 |
| **recipe** | a model plus the flags, the mode and the way it is served | a profile, a preset | `recipes/*.yaml`, spec §7 |
| **profile** (TunableOp profile) | a table of tuned settings for one rocBLAS build | the host profile | `tunableop/<arch>/<profile>/`, spec §9.1 |
| **step** | one entry of the onboarding checklist | a job, a setup step of the plan | spec §3.3 (`hardware`, `access`, `rocm`, `python-env`, `model`, `recipe`, `serve`, `verify`, plus `bench`, `tuning`, `power`) |
| **setup step** | one install action of `setup plan` (`venv.create`, `model.download`, …) | a checklist step | `internal/setup/steps_*.go` |
| **check** | one doctor check | a P2P checklist item | `internal/doctor`, spec §5.2 |
| **finding** | one known failure matched in a log | a check | `internal/logdoc`, spec §5.3 |
| **suite** | one benchmark suite (`smoke`, `pr`, `long-context`) | a bench run | `bench/suites/*.yaml` |
| **result** | one saved benchmark result | a suite | `bench list` |
| **verdict** | the outcome of comparing two results | a score | spec §10.3 |
| **state** | the word that says what a thing is doing: `done`, `todo`, `running`, `blocked`, `skipped` (checklist); `starting`, `running`, `exited` (run); `running`, `done`, `failed` (job) | a colour | spec §3.3, §8.3 |
| **status** | the word that says what a check says: `ok`, `warn`, `fail`, `skip` | a state | spec §5.1 |
| **fix** | the exact command that repairs a check or a finding | advice | spec §5.1, §5.3 |
| **plan** | the resolved environment and command of a recipe | a preview | spec §7.2 |
| **mode** | `host` or `container` | an engine | spec §8.2 |

### 2.1 The three state vocabularies, and why they are three

Today three different sets of words describe three different kinds of thing.
That is correct — a checklist step and a GPU check are not the same kind of
thing — but the *shape* of the sets must match, or the page cannot say "the
same state" once.

| Kind | Words | Source |
|---|---|---|
| checklist step | `done` `todo` `running` `blocked` `skipped` | spec §3.3 |
| job | `running` `done` `failed` | spec §3.3 |
| run | `starting` `running` `exited` | spec §8.3 |
| check | `ok` `warn` `fail` `skip` | spec §5.1 |
| finding | `error` `warn` | spec §5.3 |
| benchmark job | `running` `done` `failed` | spec §11 |
| topology link class | `PIX` `PXB` `PHB` `SYS` | spec §4.2 |

The page must show these words unchanged. `dashboard/web/components/timeline.js:9`
already holds `TIMELINE_STATE_WORD` with `done/todo/running/blocked/skipped` and
one extra (`failed`) that the spec does not have; `dashboard/web/js/shell.js:259`
holds the run pill (`idle`, `starting`, `serving`, `not answering`, `stopped`).
The run pill is the one place where a new word was invented: it does not exist
in the spec. **Decision for the commit plan**: the rail pill keeps its five
words, and spec §8.3 gains them as the *view* of the run state (`idle` = no
run, `not answering` = a run that exists but does not answer). One run, one
list of words, in the spec.

### 2.2 The error wording pattern

Every failure a user can see answers three questions in this order:

1. **What happened.** One sentence, present tense, no blame.
2. **Why.** The cause, in the words of the machine (a path, a number, a
   threshold) or in the words of the user (a missing package).
3. **What to do next.** The exact command, copyable, or the exact control to
   press.

The doctor already does this: `{"id", "category", "status", "title", "detail",
"fix", "doc"}` (spec §5.1). The log doctor does the same with
`{"id", "severity", "cause", "fix", "doc", "line", "line_no"}` (spec §5.3).
The dashboard's finding card uses the same shape
(`dashboard/web/views/logs/findings.js`).

Two rules the page breaks today, both in the Setup tab:

- The rule "no `invalid`, `illegal`, `wrong`, `incorrect` in the user-visible
  copy of this tab" (dashboard/AGENTS.md, Setup rule 9) holds for the setup
  copy. The **CLI** does not follow the same rule: `bernie status` prints
  `[FAIL] /dev/kfd is readable and writable → sudo modprobe amdgpu`, which is
  fine, but a usage error prints `bernie: unknown command "tune power"`,
  which names the mistake without the fix. The CLI's usage error should end
  with the command that would have worked.
- The CLI's finding block prints `cause:`, `fix:`, `doc:`, `line:` as raw
  labels (`internal/cli/findings.go:16-22`). The page renders the same four
  facts as a card. The labels should be the same words on both front ends:
  **cause**, **fix**, **doc**, **line**.

### 2.3 The step names, one list

The eight checklist steps and the three extras are the spine of the product.
They are named in spec §3.3 and must never be renamed per front end.

| # | Id | Title (the words of the row) | One-line job |
|---|---|---|---|
| 1 | `hardware` | The cards are there | the `gpu` category passes |
| 2 | `access` | The groups are right | the user can open the device nodes |
| 3 | `rocm` | The ROCm tools work | the `rocm` and `kernel` categories pass |
| 4 | `python-env` | The Python environment is ready | the `python` category passes in the venv |
| 5 | `model` | The model is on the disk | the weights are downloaded |
| 6 | `recipe` | A recipe fits | a validated recipe matches the GPUs |
| 7 | `serve` | The server answers | a run serves |
| 8 | `verify` | The server answers a request | one prompt comes back |
| 9 | `bench` | The speed is measured | optional: a suite has run |
| 10 | `tuning` | The kernels are tuned | optional: a TunableOp profile is loaded |
| 11 | `power` | The power caps are set | optional: the caps are written |

The dashboard's Setup tab already renders these as `THE PATH TO A SERVING
MODEL` (`dashboard/web/views/setup/setup.js:72`) and the tab sheet
[setup.md](setup.md) gives each row its title. The CLI's `status --steps`
must print the same titles.

---

## 3. The inventory of bernie today

Every user-visible capability, where it lives today, and where it should
live. **89 rows.** Sources: `bernie --help` and `bernie <cmd> --help`
(19 top-level commands, 25 leaf commands), `internal/mcp/tools_read.go` and
`tools_write.go` (19 read tools, 6 write tools), `dashboard/internal/api/api.go`
(31 routes), `dashboard/README.md` (the tab table), spec §3.2, §4, §5, §7,
§8, §9, §10, §11, §13.

Legend for the last two columns: **S** = the simple view of the tab (the
default), **D** = the *Show details* toggle of that tab, **A** = the Advanced
tab.

### 3.1 Host inspection and diagnosis

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 1 | Hardware and OS inventory | `bernie hw` | `hw` | Advanced tab (raw doc); Setup reads it for the facts | **A** (raw) + Setup **D** (facts) | one document, three readers |
| 2 | PCIe tree | `bernie topo` | `topo` | Topology tab, `THE PCIE TREE` | Topology **S** | already right |
| 3 | P2P link classes (PIX/PXB/PHB/SYS) | `bernie topo` | `topo` | Topology tab, `THE LINK CLASSES` | Topology **S** | already right |
| 4 | P2P readiness checklist (8 items) | `bernie topo --check` | `topo` | Topology tab, `THE READINESS CHECKLIST` | Topology **S** | the doctor's `topo.p2p` category is the same list |
| 5 | P2P readiness with a named recipe's env | `bernie topo --check --recipe ID` | `topo` | Topology tab (`recipe_env` item) | Topology **D** | one item of the checklist |
| 6 | GPU-to-GPU bandwidth measurement | `bernie topo --bandwidth` | `topo` (`bandwidth`) | Topology tab, `THE BANDWIDTH` — says the dashboard never measures it | Topology **D** | a long measurement; the tab names the command |
| 7 | All host checks (9 categories) | `bernie doctor` | `doctor` | Setup tab, `DOCTOR`; Advanced tab | Setup **D** (full list) + **A** (raw) | the count of failures is the tab badge |
| 8 | One category of checks | `bernie doctor --category C` | `doctor` (`categories`) | not on the page | Setup **D** | the page filters client-side |
| 9 | Log doctor on a file or stdin | `bernie doctor --log FILE\|-` | `doctor_log` (by run name only) | Logs tab, `THE KNOWN PROBLEMS` | Logs **S** | the MCP tool takes a run name, never a path (spec §13.7) |
| 10 | Log doctor on a run | `bernie doctor --run NAME` | `doctor_log` | Logs tab | Logs **S** | same rule set |
| 11 | The phase sentence of a run | (in `ps --json`) | `ps` | rail pill, Logs tab, Dashboard hero | Dashboard **S** | spec §5.4 |

### 3.2 The setup path

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 12 | The host status and the next command | `bernie status` | `status` | not on the page (Setup reads the same facts) | Setup **S** (headline) | the CLI is the printed form of the Setup headline |
| 13 | The onboarding checklist | `bernie status --steps` | `onboarding_status` | Setup tab, `THE PATH TO A SERVING MODEL` | Setup **S** | the spine of the product |
| 14 | The facts of the host (label, value, why) | `bernie status --json` (`onboarding.facts`) | `onboarding_status` | Setup tab, facts strip | Setup **S** | spec §3.3 |
| 15 | The next step in words | `bernie status --json` (`onboarding.next_action`) | `onboarding_status` | Setup tab, headline lede | Setup **S** | one record, three front ends |
| 16 | Run one non-root setup step | `bernie setup apply --step ID` | `setup_apply` (all steps) | Setup tab, `Run` on a step | Setup **S** | one job at a time |
| 17 | Run all non-root setup steps | `bernie setup apply` | `setup_apply` | not on the page (one step at a time) | Setup **S** (one primary button) | the reference's "set everything up" |
| 18 | The install plan of this host | `bernie setup plan` | — | not on the page | Setup **D** | the plan behind the steps |
| 19 | Root steps as commands | `bernie setup apply` (prints them) | `setup_apply` (returns them) | Setup tab, command block per root step | Setup **S** | bernie never runs sudo |
| 20 | The venv and where it came from | `bernie status --json` (`venv`) | `status`, `onboarding_status` | Setup tab, facts | Setup **D** | spec §6.2 |
| 21 | The glossary of the words | — (no CLI command) | — | `/api/glossary`, Setup tab `The words on this page` | Setup **D** + Dashboard **D** | the API exists, the CLI does not: **gap** |

### 3.3 Recipes and the plan

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 22 | The list of recipes with their fit | `bernie recipe list [--all]` | `recipe_list` (`all`) | Setup tab, `THE RECIPE` | Setup **S** | |
| 23 | One recipe, merged | `bernie recipe show ID` | `recipe_show` | Setup tab (the picked recipe) | Setup **D** | |
| 24 | The plan: env, args, GPU selection | `bernie recipe plan ID` | `recipe_plan` | Setup tab, the plan card | Setup **D** | spec §7.2 |
| 25 | Override a setting | `bernie recipe plan ID --set K=V` | `recipe_plan` (`set`) | Setup tab, overrides | Setup **D** | |
| 26 | The reason behind every setting | (in `recipe plan --json`) | `recipe_plan` | Setup tab, `Settings and reasons` | Setup **S** | already shipped (spec §7.9) |
| 27 | The mode switch (host / container) | `--mode host\|container` | `recipe_plan` (`mode`) | Setup tab, the mode switch | Setup **S** | |
| 28 | Validate the recipe files | `bernie recipe validate` | — | not on the page | **A** | a maintainer command |
| 29 | The recipe catalogue (13 recipes, 4 families) | `bernie recipe list --all` | `recipe_list` | Setup tab | Setup **S** | `recipes/`: flashnext ×6, qwen-dense ×3, deepseek ×1, + fragments |

### 3.4 Runs

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 30 | Start a run | `bernie serve ID` | `serve` | Setup tab (the `serve` step) | Setup **S** | |
| 31 | Start a run and wait for health | `bernie serve ID --wait D` | — (poll `ps`) | not on the page | Setup **D** | |
| 32 | The whole path in one command | `bernie up [RECIPE]` | not a tool (spec §13.4) | not on the page | Setup **S** (the one primary button) | the reference's "set everything up" |
| 33 | The list of runs | `bernie ps` | `ps` | Logs tab, `THE RUN`; rail pill | Dashboard **S** + Logs **S** | one document, two readers |
| 34 | Stop a run | `bernie stop [NAME]` | `stop` | Dashboard hero `Stop`; Logs tab | Dashboard **S** | |
| 35 | The log of a run | `bernie logs [NAME]` | `logs` | Logs tab, `THE LOG` | Logs **S** | |
| 36 | Follow the log | `bernie logs NAME -f` | — | Logs tab (SSE) | Logs **S** | |
| 37 | The tail of the log with levels and a search | `bernie logs --tail N` | `logs` (`tail`) | Logs tab (level filter, search) | Logs **S** | |
| 38 | The findings of an exited run | (in `ps --json`) | `ps` | Logs tab, `THE KNOWN PROBLEMS` | Logs **S** | spec §8.3 |
| 39 | Check that the server answers | `bernie smoke [--url] [--model]` | `smoke` | Setup tab (the `verify` step) | Setup **S** + Dashboard **S** | spec §3.3 step 8 |
| 40 | The run record (pid, container, port, plan) | (in `ps --json`) | `ps` | Advanced tab | **A** | spec §8.3 |

### 3.5 Container mode

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 41 | Run in a container | `bernie serve ID --mode container` | `serve` (`mode`) | Setup tab (the mode switch) | Setup **S** | |
| 42 | The container plan (`docker run` argv) | `bernie recipe plan ID --mode container` | `recipe_plan` (`mode`) | Setup tab, the plan card | Setup **D** | `internal/serving/runner/command.go:148` |
| 43 | A compose file for the plan | `bernie compose ID [-o FILE]` | — | not on the page | **A** | `internal/serving/runner/command.go:295` |
| 44 | The GPU devices of the container | (in the plan) | (in the plan) | not on the page | **A** | today `--device /dev/kfd --device /dev/dri`; see D5 |
| 45 | The GPU group ids of the container | (in the plan, `group_add`) | (in the plan) | not on the page | **A** | numeric ids, never names (spec §8.2) |
| 46 | The container engine (Docker or Podman) | `bernie doctor --category container` | `doctor` | Setup tab, `DOCTOR` | Setup **D** | |
| 47 | Rootless Podman (`--group-add keep-groups`) | (automatic) | — | not on the page | **A** | spec §8.2 |

### 3.6 Tuning and power

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 48 | The TunableOp profiles | `bernie tune tunableop list` | — | not on the page | **A** | |
| 49 | The loaded rocBLAS and its profile | `bernie tune tunableop status` | `tunableop_status` | Setup tab (the `tuning` extra step) | Setup **D** | |
| 50 | The TunableOp env lines | `bernie tune tunableop env` | — | not on the page | **A** | materialises the rows |
| 51 | Force a profile / no TunableOp | `--profile P`, `--no-tunableop`, `--tuning` | `recipe_plan` (`profile`, `no_tunableop`, `tuning`) | Setup tab, the plan card | Setup **D** | |
| 52 | The power cap of every GPU | `bernie tune power status` | `power_status` | Advanced tab, `THE POWER CAPS` | Advanced **D** → **A** | the read moves to **A** |
| 53 | Write the desired power cap | `bernie tune power set W [--gpu IDX]` | `tune_power_set` | Advanced tab (a write) | **A** | the only power write |
| 54 | Apply the caps (needs root) | `bernie tune power apply` | — | not on the page (a root command) | **A** | prints the root command |
| 55 | The power service (systemd pair) | `bernie tune power service print\|install-plan\|status\|remove-plan` | `power_status` (state only) | not on the page | **A** | spec §9.2 |
| 56 | The power advice per card | (in the doctor `tuning` category) | `doctor` | Setup tab, `DOCTOR` | Setup **D** | |

### 3.7 Benchmark

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 57 | Run a suite | `bernie bench [--suite]` | `bench_run` | Benchmark tab, `The suites` → `Run the suite` | Benchmark **S** | |
| 58 | The built-in suites (3: `smoke`, `pr`, `long-context`) | `bernie bench suites` | `bench_suites` | Benchmark tab, the suite cards | Benchmark **S** | `bench/suites/*.yaml` |
| 59 | The saved results | `bernie bench list` | `bench_list` | Benchmark tab, `The history` | Benchmark **S** | |
| 60 | One saved result | `bernie bench show ID` | `bench_show` | Benchmark tab, `The result` | Benchmark **S** | |
| 61 | Compare two results with a verdict | `bernie bench compare A B` | `bench_compare` | Benchmark tab, `The comparison` | Benchmark **S** | spec §10.3 |
| 62 | The receipt of the comparison | `bernie bench compare A B --markdown` | — | Benchmark tab, `Copy the receipt` | Benchmark **S** | |
| 63 | The A/B run | `bernie bench ab --base --cand` | `bench_ab` | Benchmark tab, `An A/B run` | Benchmark **S** | spec §10.4 |
| 64 | The pass-key recall check | `bernie bench --recall` | `bench_run` (`recall`) | Benchmark tab, the suite options | Benchmark **S** | |
| 65 | The length curve of a suite | (in `bench show --json`) | `bench_show` | Benchmark tab, `The result` | Benchmark **S** | |

### 3.8 The dashboard, the telemetry and the jobs

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 66 | The live sample of the host | — (no CLI) | — | Dashboard tab, the tiles and the charts (1 Hz SSE) | Dashboard **S** | `/api/telemetry` |
| 67 | The history of the last minutes | — (no CLI) | — | Dashboard tab, the charts | Dashboard **S** (window) | `/api/gpus/history` |
| 68 | The power service state | `bernie tune power service status` | `power_status` | `/api/power` | Advanced **A** | |
| 69 | Write the power cap | `bernie tune power set` | `tune_power_set` | `PUT /api/power` | Advanced **A** | the only write on the Dashboard API besides runs |
| 70 | The jobs of the host | — (no CLI) | `jobs` | Setup tab (the job of a step) | Setup **D** + **A** | |
| 71 | One job and its output | — (no CLI) | — | Setup tab, under the step | Setup **S** (while it runs) | |
| 72 | The event stream of a job | — | — | Setup tab, the job stream | Setup **S** | |

### 3.9 The product surface itself

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 73 | Serve the dashboard | `bernie dash [--listen] [--token]` | — | the page | — | spec §11 |
| 74 | The dashboard over MCP | — | `POST /mcp` on the dashboard | — | — | spec §13.10 |
| 75 | The MCP server on stdio | `bernie mcp [--allow-write]` | — | — | — | spec §13.2 |
| 76 | The user defaults | `bernie config show` / `config set K V` | — | not on the page | **A** | `venv`, `mode`, `engine`, `hf_home`, `set` |
| 77 | The version | `bernie version` | — | not on the page | **A** | |

### 3.10 The counts, checked

| What | Count | Where it was checked |
|---|---|---|
| Top-level CLI commands | 19 | `bernie --help` |
| Leaf CLI commands | 25 | `bernie <family> --help` for the 6 families |
| MCP read tools | 19 | `internal/mcp/server_test.go:106` |
| MCP write tools | 6 | `internal/mcp/tools_write.go:105-122` |
| HTTP routes under `/api` | 31 | `dashboard/internal/api/api.go:60-90` |
| Event streams | 4 | `/api/telemetry`, `/api/runs/{n}/logs`, `/api/jobs/{id}/events`, `/api/bench/{id}/events` |
| Dashboard tabs | 8 | `dashboard/web/js/boot.js` `VIEWS` |
| Recipes | 13 (+ fragments) | `recipes/` |
| Doctor categories | 9 | `internal/doctor/doctor.go:32` |
| P2P checklist items | 8 | spec §4.5 |
| Benchmark suites | 3 | `bench/suites/` |
| Onboarding steps | 8 + 3 | spec §3.3 |
| Glossary words | 26 | `internal/glossary/glossary.go` |

**Two gaps found while counting.** The API table in spec §11 is missing
`GET /api/bench/suites`, and the spec's claim that the Advanced tab reads
`/api/gpus/history` is wrong: the Dashboard tab reads it, the Advanced tab
does not (`dashboard/web/views/dashboard/` vs `dashboard/web/views/advanced/`).
Both go into the commit plan as spec fixes.

---

## 4. The proposed organisation

### 4.1 The tabs, and the job of each

Eight tabs, in this order. The order is the order of the life of the box:
set it up, watch it, use it, measure it, wire it, read it, go deeper.

| # | Tab | Its one question | Its one action | Simple (the default) | Details (the toggle) | Advanced tab |
|---|---|---|---|---|---|---|
| 1 | **Setup** | What do I do to make this box serve? | the next step of the path | the headline, the facts, the path, the recipe, the doctor summary | the install plan, the venv, the glossary, the raw documents | the job list, the doctor report in full |
| 2 | **Dashboard** | Is it serving, and how fast? | stop or start | the hero, the six tiles, the two charts, the GPU cards, the machine, how to connect | the per-GPU table, the history window, the power read-out | the run record, the telemetry frame, the raw documents |
| 3 | **Chat** | What will I ask the model? | send one prompt | one prompt box and the reply | the request and the answer as text | — (until it has something) |
| 4 | **Benchmark** | How fast is it, and does it read the whole prompt? | run the suite | the suites, the last result, the comparison, the history | the raw JSON of every endpoint | — |
| 5 | **Topology** | Can the cards copy to each other? | check the readiness | the link classes, the tree, the checklist, the last bandwidth | the pair table, the raw documents | — |
| 6 | **Fleet** | What do the other hosts do? | — | *planned* | — | — |
| 7 | **Logs** | What is running, and why did it fail? | stop the run | the run picker, the findings, the tail | the raw documents, the log as text | — |
| 8 | **Advanced** | What does the operator need to see and change? | write the power cap | the operator's console: the full host table, the power write, the doctor report, the jobs | — (the tab is already the deep one) | — |

### 4.2 The one mechanism for "more information"

**Both, with different jobs. This is the answer to the overlap.**

| Mechanism | What it holds | When it is used | Where it lives |
|---|---|---|---|
| **Show details** (per tab) | *More about the same thing on this tab.* The extra rows of the same table, the raw JSON of the endpoints that tab read, the CLI commands that produce them. It never shows a different subject. | the reader wants the numbers behind the sentence they just read | a `details` disclosure at the end of the tab, opened by the rail's `Show details` button or by its own summary |
| **Advanced tab** | *The operator's console.* The things that are not part of any one tab's question: the full per-GPU table, the power write, the doctor report in full, the job list, the run record, and the raw JSON of **every** endpoint. | the reader is operating or diagnosing the host, not reading it | its own tab, the eighth |

The rule that keeps them apart: **a thing belongs in a tab's details when
its question is the tab's question; it belongs in the Advanced tab when its
question is about the host as a machine.** The power cap read is about the
host (Advanced). The output speed is about the run (Dashboard).

What happens to today's overlap:

| Today | Where it goes |
|---|---|
| The Advanced tab is the old first dashboard: the fleet tiles, the per-GPU cards, the exact per-GPU table, the last-minutes charts, the run card, the power caps, the topology, the known problems, the raw documents | **Split.** The exact per-GPU table, the run record, the job list and the power write stay in the Advanced tab and become its permanent job. The fleet tiles, the last-minutes charts and the per-GPU *cards* leave: the cards are on the Dashboard tab today (`views/dashboard/gpus.js`), the charts are on the Dashboard tab today (`views/dashboard/simple.js`), and the topology is its own tab. What remains of the old layout that is unique is the raw document list, which becomes the Advanced tab's **Details** section. |
| The Dashboard tab has a hand-written *simple* view and no details disclosure | **Keep the simple view** (it is the better answer) and **add a details disclosure** to the Dashboard tab: the per-GPU table, the history window, the power read-out, the raw documents of `/api/telemetry`, `/api/hw`, `/api/runs`. |
| The Setup tab has an `Advanced` disclosure (raw documents + CLI commands) | **Rename it to `Show details`** and put it under the same rule: the raw documents of the endpoints the Setup tab read and the CLI commands that produce them. It exists today (`views/setup/advanced.js`) and is already correct in content. |
| The Logs tab has an `Advanced` disclosure | Same: rename to `Show details`, keep the content. |
| The Topology tab has an `Advanced` disclosure | Same: rename to `Show details`, keep the content. |
| The Benchmark tab has an `Advanced` button **and** an `Advanced` disclosure | Same: one disclosure, named `Show details`, no second button in the head. Two controls for one thing is the inconsistency to remove. |
| The Advanced tab has no disclosure | The Advanced tab's own details are the whole tab. It ends with the raw document list. |

**Naming.** The page-wide switch in the rail is `Show details`
(`dashboard/web/index.html:44`). The per-tab disclosures are `Advanced`
(`views/setup/advanced.js:9`, `views/logs/advanced.js:18`,
`views/topology/advanced.js:35`, `views/benchmark/suites.js:225`). **One
word: `Show details`** for the toggle, everywhere. The word `Advanced` is
kept for the **tab** only. That removes the collision between the button and
the tab.

### 4.3 What each tab shows

The SIMPLE view of a tab is *the one question, the one action, and the
numbers that answer the question*. The DETAILS view of a tab is *the same
numbers with their raw form, their reason and their document*.

**Setup** — SIMPLE

1. the headline (`THIS BOX IS SERVING.` / `SETTING UP THIS BOX.` / `THIS BOX IS READY TO SERVE.`) and the lede naming one next action and who does it;
2. the facts strip: GPUs, memory, ROCm, the system — each with its reason;
3. the path: the 8 steps plus the 3 extras, one row each, with the state word, one sentence, the command, and one filled button on the step to do now;
4. the recipe: the best fit, why it fits, the mode switch, the plan with the settings and their reasons;
5. the doctor summary: the count of checks that pass, warn and fail, and the failing ones with their fix.

**Setup** — DETAILS: the install plan (`setup plan`), the venv and its source, the glossary of the words on the page, the raw JSON of `/api/onboarding`, `/api/recipes`, `/api/recipes/{id}/plan`, `/api/doctor`, and the four commands that produce them.

**Dashboard** — SIMPLE: the hero (state word, sentence, endpoint + `Copy URL`, `Stop`, the output speed with its unit), the six serving tiles (Prompt reading, Active requests, First token, KV cache, Prefix cache hits, Draft accepted), two rate charts, one card per GPU, `This machine`, `Connect an app` (curl and Python).

**Dashboard** — DETAILS: the exact per-GPU table, the history window control, the power read-out per card, the raw JSON of `/api/telemetry`, `/api/hw`, `/api/runs`, `/api/gpus/history`.

**Benchmark** — SIMPLE: the suites as radio cards with their digest, the last result in one sentence, the table of the cells (median and p95, with the count behind them), the length curve, the comparison of two results with a verdict on every group, the receipt, the history of the saved results, and the A/B section.

**Benchmark** — DETAILS: the raw JSON of `/api/bench`, `/api/bench/suites`, `/api/bench/compare`, and the commands `bernie bench list`, `bernie bench suites`, `bernie bench compare A B`, `bernie bench compare A B --markdown`, `bernie bench ab BASE CAND`.

**Topology** — SIMPLE: the link classes, the PCIe tree, the readiness checklist (8 items with a verdict and a line each), the last bandwidth measurement.

**Topology** — DETAILS: the pair table, the raw JSON of `/api/topo` and `/api/topo/check`, and the commands `bernie topo`, `bernie topo --check`.

**Logs** — SIMPLE: the run picker, the findings (cause, fix, doc, line), the tail with the level filter and the search.

**Logs** — DETAILS: the raw JSON of `/api/runs` and the log of the picked run, and the commands `bernie ps` and `bernie logs NAME`.

**Advanced** — the operator's console: the full per-GPU table, the power caps (read and write), the doctor report in full (all 9 categories, all checks), the job list, the run record, and the raw JSON of every endpoint of the page.

### 4.4 Navigation

**Order and labels.** `Setup`, `Dashboard`, `Chat`, `Benchmark`, `Topology`,
`Fleet`, `Logs`, `Advanced`. The order is unchanged from the page today
(`dashboard/web/js/boot.js:11-20`). The labels are unchanged.

**The two placeholders.** `Chat` and `Fleet` carry the word `Soon` in the
rail and one honest sentence in the panel (`views/chat/chat.js`,
`views/fleet/fleet.js`). They are *not* in the tab bar's keyboard order in
the sense that they are disabled — they are still tabs, and they still hold
their route (`#chat`, `#fleet`), so a link to them works and lands on an
honest page. That is the right behaviour for a planned tab.

**Entry points between tabs.** Today the page has two (`#setup` from the
Dashboard hero, `#logs` from the Setup headline). The proposal adds the
missing ones, all of them one click:

| From | To | The control | Why |
|---|---|---|---|
| Setup, the `bench` extra step | Benchmark | a link on the step row | the step says "the speed is measured" and the reader must be able to go and measure it |
| Setup, the `tuning` extra step | Advanced | a link on the step row | the TunableOp profile is read in Advanced |
| Setup, the `power` extra step | Advanced | a link on the step row | the power caps are in Advanced |
| Setup, the `verify` step | Logs | `Read the logs` (exists) | |
| Setup, the doctor findings | Advanced | a link on the section head | the full report is in Advanced |
| Dashboard, a problem | Setup | a `Fix this` link on the finding | **the missing one.** The Dashboard hero has a problem card? Today it does not; the doctor findings are on Setup and Advanced. When the Dashboard shows a problem, it must link to the Setup step that fixes it. |
| Logs, a finding's fix | the terminal | the copy button on the fix command (exists) | |
| Topology, a failed checklist item | Setup | a link on the item | a failed `access` item is a Setup step |
| Any tab, the raw document | the terminal | the command in the details disclosure (exists) | |

**Empty states.** Every empty state has three parts: what the thing is, the
command that makes one, and a copy button. Today 24 empty states exist and
only 3 carry a command. The rule to enforce: **every empty state carries a
command**. The wording pattern is fixed by
`dashboard/web/components/cards.js:78` (`emptyState(what, next, command)`).

The empty states today that do not carry a command and must be fixed:

| Tab | Empty state | The command it must carry |
|---|---|---|
| Setup | "There is no recipe to serve." | `bernie recipe list --all` |
| Setup | "No recipe fits this host yet." | `bernie recipe list --all` |
| Benchmark | "No result yet." / "Run a suite above to fill this table." | `bernie bench --suite smoke` |
| Benchmark | "Pick two results to compare." | `bernie bench list` |
| Benchmark | "There is no suite to run." | `bernie bench suites` |
| Benchmark | "No saved result yet." | `bernie bench --suite smoke` |
| Logs | "No known failure in this log." | `bernie doctor --run NAME` |
| Logs | "The findings could not be read." | `bernie doctor --run NAME` |
| Logs | "No run has ever served on this host." | `bernie serve <recipe>` |
| Topology | "There is no PCIe tree to show." | `bernie hw` |
| Topology | "There is no matrix to show." | `bernie topo` |
| Topology | "There are no pairs to show." | `bernie topo --bandwidth` |
| Dashboard | "No run is serving." | `bernie serve <recipe>` (exists) |

**The landing tab.** First run: **Setup**. The path is not finished, so
Setup holds the one action. Returning user: **Dashboard**. The box is
serving; the question is "is it OK?". The rule: the landing tab is the tab
that holds the current primary action. When the path is complete, the landing
tab is Dashboard; while it is not, it is Setup. This is computable from
`/api/onboarding` (`next` is empty ⇒ the path is complete), so it needs no
new state.

The route stays explicit: the router keeps the hash. There is no "home"
route; `#setup` is the first tab and the default.

### 4.5 The onboarding path, in the same frame

The reference (`vllm-rdna-dash`) is the product in one page: a headline that
states the situation, one paragraph, four facts, **one big button**, and the
step timeline below it (`~/ui-study/design-dashboard-v2.md` §1.2 and §3.1;
`~/ui-study/collab/01-setup.png`, `27-setup-autopilot-running.png`,
`28-setup-autopilot-done.png`).

bernie has every piece of this but in a different shape: the headline and the
facts are on the Setup tab, the timeline is there, the steps have their
buttons, the jobs report their progress — but there is **no single primary
action that runs the whole path**, and the Setup tab's primary button names
only the *next* step.

The proposal, in the same frame:

| Element | bernie today | The proposal |
|---|---|---|
| The headline | `THIS BOX IS READY TO SERVE.` / `SETTING UP THIS BOX.` / `THIS BOX IS SERVING.` | unchanged; the wording is already right |
| The lede | one sentence naming one action and who does it | unchanged |
| The facts | the facts strip with the reasons | unchanged |
| The one primary action | the *next step* only ("Build the Python environment", "Download the model", "Start the server") | **two states of the same button**: before the path is done it says **"Set up this box"** and runs the whole non-root path (the CLI's `bernie up`); a small line under it says "or go step by step below". Once the path is done it says **"Start the server"** |
| The step timeline | 11 steps, one filled `Run` on the next one | unchanged; the per-step `Run` stays for the step-by-step path |
| Root work | a command block with a copy button and a `Re-check` | unchanged; after the root command, `Re-check` re-reads the host in place |
| The progress sentence | the phase sentence of the job | unchanged, plus the bytes/rate/ETA sentence of the model download (spec §3.3 `progress`) |
| The end state | the headline says `THIS BOX IS SERVING.` | unchanged; the reader lands on the Dashboard tab by following the `Try it` link |

**What the primary button needs.** A new endpoint: `POST /api/onboarding/run`
that walks the non-root steps in order, one job at a time, and stops at the
first step that needs root or that fails. That is the CLI's `up` path
(`internal/flow/up.go`), exposed the way `POST /api/onboarding/{step}/run`
already is. Effort: **M** (the flow exists; the endpoint, the job chaining
and the UI are new).

The reference's `Autopilot` (`POST /api/autopilot`) is the same thing. It
must be in the design as `Set up this box`, with one honest difference: the
reference *runs the root steps itself* through `sudo bash`; bernie never does
(spec: never call `sudo`). bernie's version stops at the root step and shows
the command — which is what the reference's own page promises ("Anything that
needs your password shows up as a command to paste") but does not always do.

---

## 5. The reference: every feature, and where it lands

The reference is `vllm-rdna-dash` by CrispyGinger, HEAD `74fb1d8`
(`~/.cache/minimax-researcher/vllm-rdna-dash`), live demo at
`http://127.0.0.1:18095/`. Its repository carries no licence, so bernie
re-implements its behaviour in its own words; the layout, the palette and the
interaction patterns are used with the author's permission
([../design-dashboard.md](../design-dashboard.md) §5).

**Rule for this section: no feature of the reference is dropped.** Every
feature below is either already in bernie, is a change bernie should make, or
is a change bernie should make *differently* — and the reason is written
down.

Legend: **present** = in bernie today (with the place); **partial** = the
core is there, the reference's version has more; **absent** = not in bernie.
Effort: S = under a day, M = a few days, L = a week or more.

### 5.1 The reference's own structure

| # | Reference feature | Reference endpoint / file | Status in bernie | Proposed home | Effort | Commit |
|---|---|---|---|---|---|---|
| R1 | Seven views: setup, dashboard, chat, bench, topology, fleet, logs | `rdna_dash/web/app.js` (hash router) | **present** — eight tabs (Setup, Dashboard, Chat, Benchmark, Topology, Fleet, Logs, Advanced), one extra (Advanced) | the same eight | — | — |
| R2 | A 220 px sticky left rail with glyph + word links, a status pill and a theme button in the footer | `app.css`, `app.js` | **present** — `dashboard/web/index.html:19-47` | the same | — | — |
| R3 | A status pill with a word and an icon | `app.js` (`.pill`) | **present** — `dashboard/web/js/shell.js:248` | the same, plus the spec change of §2.1 | S | C4 |
| R4 | A theme button (dark default, light override) | `app.css` `data-theme` | **present** — `dashboard/web/js/shell.js:191` | the same | — | — |
| R5 | The display headline: italic 900 uppercase with a hard offset shadow | `app.css` `.display` | **present** — `tokens.css` `--font-display` | the same | — | — |
| R6 | The facts strip (label / value / reason) | `app.js` (`.fact`) | **present** — `components/cards.js:47` (`factRow`) | Setup **S** | — | — |
| R7 | A copy button on every command, endpoint and snippet | `app.js` (`.cmd .copy`) | **present** — `components/command.js` | the same | — | — |
| R8 | One filled primary button per view, everything else ghost | `app.css` `.btn.primary` | **present** — the rule is in `dashboard/AGENTS.md` UI rule 4 | every tab | — | — |
| R9 | The step timeline with a per-step state and a per-step action | `app.js` (`.step`) | **present** — `components/timeline.js` | Setup **S** | — | — |
| R10 | The toast | `app.css` `.toast` | **present** — `js/shell.js` `toast` | the same | — | — |
| R11 | The info tooltip (`i` mark) | `app.js` (`.q`) | **present** — `components/cards.js` + `js/shell.js` `openTip` | the same | — | — |
| R12 | The banner (a stale notice with a 3 px left bar) | `app.css` `.banner` | **present** — the stale banner of the Setup tab | the same | — | — |
| R13 | The segmented control | `app.css` `.seg` | **present** — the mode switch of the Setup tab, the history window of the Dashboard tab | the same | — | — |
| R14 | The KPI tile, the hero number, the meter, the area chart, the donut ring, the heatmap matrix, the bar chart | `app.js`, `components/charts.js` in bernie | **present** — `dashboard/web/components/charts.js` | the same | — | — |
| R15 | The radio card (the suite picker) | `app.js` | **present** — the suite cards of the Benchmark tab | Benchmark **S** | — | — |
| R16 | The key/value/why list ("YOUR TUNED SETUP") | `app.js` | **present** — the plan's `Settings and reasons` (spec §7.9) | Setup **S** | — | — |

### 5.2 The features bernie does not have yet

| # | Reference feature | Reference endpoint / file | Status in bernie | Proposed home | Effort | Commit |
|---|---|---|---|---|---|---|
| R17 | **"Set everything up"** — one button that runs the whole path | `POST /api/autopilot`, `rdna_dash/jobs.py` | **absent** — the Setup tab runs one step at a time | Setup **S**: the primary button, with "or go step by step below" | M | C6 |
| R18 | **Re-check without a reload** — "Re-check this box" | `POST /api/profile/refresh` | **partial** — the Setup tab has a `Re-check` button that reloads the onboarding document; there is no *profile* refresh | Setup **S**: `Re-check` re-reads the host in place and updates the step states | S | C5 |
| R19 | **The root-command pause** — the autopilot stops and says "Some things need you first" | `rdna_dash/jobs.py` | **partial** — the step shows the root command, but the path does not stop there in one place | Setup **S**: the path stops at the root step and the headline names it | S | C6 |
| R20 | **The progress sentence with bytes, rate and ETA** ("88% · 53.7 of 61.4 GB · 112 MB/s · about 4s left · file 7 of 14") | `rdna_dash/jobs.py` | **partial** — the job carries `progress` (spec §3.3) but the page shows the phase sentence only | Setup **S**: the sentence under the progress bar of the `model` step | S | C5 |
| R21 | **The download speed limit** ("Changes take effect within a second") | `POST /api/download/limit`, `rdna_dash/download.py` | **absent** — bernie's downloader has no limit | Setup **D**: a field on the `model` step | M | C7 |
| R22 | **The Chat tab** | `POST /api/chat`, `rdna_dash/serve.py` | **absent** — the tab is a placeholder by the owner's order | Chat **S**: one prompt box, one streaming reply | M | C8 |
| R23 | **The Fleet tab** — the other boxes, a share button, a remove button | `GET /api/fleet`, `POST /api/fleet/remove`, `rdna_dash/profile.py` | **absent** — the tab is a placeholder by the owner's order; spec §14 says multi-host fleet control is out of scope | Fleet **S** (planned): the list of the other bernie hosts and their state | L | C9 |
| R24 | **The redacted share report** ("Copies a report with no hostname, username or tokens") | `POST /api/profile/share` | **absent** — the doctor has no `--report`; roadmap item 8 wants one | Advanced **A**: a `Copy a redacted report` button | M | C7 |
| R25 | **The server start / stop / restart from the page** | `POST /api/server/start`, `/stop`, `/restart` | **present** — `POST /api/runs`, `DELETE /api/runs/{name}`; the Dashboard hero has `Stop` and `Start a run` | Dashboard **S** | — | — |
| R26 | **The server log pane** | `GET /api/server/log` | **present** — the Logs tab | Logs **S** | — | — |
| R27 | **The profile refresh / host probe** | `POST /api/profile/refresh`, `rdna_dash/profile.py` | **partial** — `bernie hw` and `bernie doctor` read the host; there is no single "re-read the host" call behind one button | Setup **S**: the `Re-check` button | S | C5 |
| R28 | **The preferences panel** | `GET /api/prefs`, `rdna_dash/server.py` | **partial** — `bernie config show` / `config set` exist; the page has no preferences panel | Advanced **A**: the user defaults (`venv`, `mode`, `engine`, `hf_home`) | M | C7 |
| R29 | **The attach action** (attach to the running server) | `POST /api/attach` | **absent** — bernie starts its own runs and does not attach to a foreign server | Advanced **A**, or **not at all**: bernie's model is "bernie owns the run" | S | — (decision) |
| R30 | **The benchmark start / cancel / delete** | `POST /api/bench/start`, `/cancel`, `/delete`, `rdna_dash/bench.py` | **partial** — bernie has `POST /api/bench`, `/api/bench/run`, `/api/bench/ab` with a 409 while one runs; there is no cancel and no delete | Benchmark **S**: a `Stop` on the running suite; the history keeps its results (no delete — a saved result is an artefact) | M | C7 |
| R31 | **The models list** | `GET /api/models/` | **partial** — `bernie smoke --model` defaults to the first id of `/v1/models`; there is no list command | Dashboard **D**: the model the run serves | S | C7 |
| R32 | **The topology bandwidth heatmap** | `GET /api/topology/bandwidth`, `rdna_dash/topology.py` | **partial** — `bernie topo --bandwidth` measures it; the page says the dashboard never measures it and shows the last result as a table | Topology **D**: the matrix, drawn as a heatmap | M | C7 |
| R33 | **The install scripts** (`install.sh`, `install-host.sh`) | `~/.cache/minimax-researcher/vllm-rdna-dash/install.sh` | **absent** — bernie is one binary; `scripts/install.sh` in this repo installs bernie, not the host | not a feature of the dashboard; the *host* install is the Setup path (the `access`, `rocm`, `container` steps) | — | — |
| R34 | **The Dockerfile and compose.yml of the dashboard itself** | `Dockerfile`, `compose.yml` of the reference | **absent** — bernie's dashboard ships inside the binary; `bernie compose` writes a compose file for a *run*, not for the dashboard | not applicable; the product surface is one binary | — | — |
| R35 | **The example recipes and the `recipes/` JSON files** | `examples/`, `recipes/*.json` of the reference | **present** — bernie's `recipes/` is YAML with 13 recipes and the same families (flashnext, qwen 27B, deepseek) | Setup **S** | — | — |
| R36 | **The ECC and power tools** (`tools/ecc.sh`, `tools/power.sh`) | `tools/` of the reference | **partial** — bernie has the doctor checks `gpu.ecc.*` and `tuning.power` and the power service; the root commands are printed, not scripted | Setup **S** (the `access` and `power` steps) | — | — |
| R37 | **The log doctor** | `rdna_dash/logdoctor.py` | **present and stronger** — bernie's catalog has 58 rules with an id, a cause, a fix and a doc link (spec §5.3) | Logs **S** | — | — |
| R38 | **The demo mode** | `rdna_dash/demo.py` | **absent** — the reference's demo replaces every step script with a canned output, so the whole UI can be tried with no hardware | **not planned.** bernie's fixtures (`internal/support/fixture/testdata/*.tar.gz`) cover the tests. A demo mode for the *page* is a product decision the owner has not made | L | — (decision) |

### 5.3 The reference's features bernie deliberately does not copy

These are listed so that "no feature dropped" is not read as "no judgement
exercised". Each one has a reason that is already written down in this
repository.

| Reference feature | Why bernie does not copy it | Where the reason is written |
|---|---|---|
| The plan embeds shell scripts and runs `curl … \| sudo bash -s` | bernie never calls `sudo` and never runs a root command; root steps are printed as commands | spec §2 principle 5, `dashboard/AGENTS.md` hard rule 5 |
| The chat page and the fleet view were "not planned" in the earlier bernie study | the owner has since ordered the tabs to be present; see D2 | `~/ui-study/design-dashboard-v2.md` §2.2 N8 |
| The font CDN | bernie forbids a CDN; the three families are self-hosted woff2 in the binary | `dashboard/AGENTS.md` hard rule 3, [tokens.md](tokens.md) |
| Polling every second | bernie uses one SSE at 1 Hz for every viewer | spec §11, `dashboard/web/js/data.js` |
| The optional login, the dashboard container that mounts the Docker socket | bernie requires a token off loopback and refuses a foreign `Origin` | spec §11, §13.10 |
| The attach action | bernie owns the runs it starts; attaching to a foreign server is a different product | spec §14 |
| Deleting a saved benchmark result | a saved result is the evidence of a measurement; bernie keeps it | spec §10 |

---

## 6. Chat and Fleet: the two placeholder tabs

The owner's order: the tabs are there, and they are not usable yet. This
section says what they become, and what it costs.

### 6.1 Chat

The lean version: **one prompt box, one reply, streamed.** The reference's
chat is a transcript with a meta line per message, a system prompt and a
parameter panel (`~/ui-study/design-dashboard-v2.md` §1.4, `collab/11-chat-live.png`).
bernie needs none of that to answer the question of the tab.

| Piece | What | Where |
|---|---|---|
| The prompt box | one `<textarea>` and a `Send` button | Chat **S** |
| The reply | the streamed text, with the time to first token and the tokens per second under it | Chat **S** |
| The model | the model of the running run | Chat **D** |
| The request as text | the exact JSON body that was sent | Chat **D** |

**What it needs.** `POST /api/chat` (spec §10 of `docs/design-dashboard.md`
has the shape: `{"run", "model", "stream", "messages", "temperature",
"max_tokens", "reasoning"}`, `text/event-stream`, 400 when the run is not
ready, 401 without the token, 403 without `X-Bernie`). The backend proxies the
OpenAI API of the running run, to a loopback target only — the same rule the
`/api/smoke` endpoint already follows (spec §11).

**Effort: M.** The endpoint, the streaming writer (the SSE writer of
`dashboard/internal/sse/` already exists) and the page. The design has the
shape in `docs/design-dashboard.md` §10.

**Why not keep it a placeholder.** A tab that says "planned" for a release
and a half teaches the reader that bernie promises and does not deliver. A
one-prompt Chat that works is the smallest honest version of the tab, and it
reuses `/api/smoke`, which already exists and already answers.

### 6.2 Fleet

The lean version: **the list of the other bernie hosts and what they are
doing.** The reference's fleet view is a list of the other boxes with their
telemetry, a share button and a remove button
(`GET /api/fleet`, `POST /api/fleet/remove`).

bernie's problem: spec §14 says *"Multi-host fleet control. The dashboard
shows one host"* is out of scope for draft 1. A Fleet tab that lists other
hosts needs a way to know about them: either a file of host addresses that
bernie reads, or a federation between the dashboards themselves.

| Piece | What | Where |
|---|---|---|
| The list of hosts | the hosts bernie knows, with their state and their last answer | Fleet **S** |
| The share button | the redacted report of this host, copied (R24) | Fleet **S** |
| The remove button | forget a host | Fleet **S** |

**What it needs.** A place to declare the other hosts: `bernie config set
fleet` (a list of `name=url` pairs), and a read of each host's `/api/telemetry`
over the network — which means the other host's dashboard token. That is a
security surface bernie does not have today (the token mode is per-process,
and the token is printed once).

**Effort: L**, and it opens a security question (how does one host hold
another's token?) that the owner should answer before any code is written.

**Recommendation.** Ship the **redacted report** (R24) first, in the Advanced
tab, and keep the Fleet tab as a placeholder until the token question has an
answer. That gets the reference's most useful fleet feature — the one-click
report that is safe to paste into an issue — without inventing a federation
model in a design document.

---

## 7. The design coherence rules, and what breaks today

The rules already exist, in three places:

| Where | What it holds |
|---|---|
| [../design-dashboard.md](../design-dashboard.md) | the visual contract: the shell, the rail, the tab bar, the pill, the Advanced panel, the loading/empty/error states |
| [tokens.md](tokens.md) | the palette, the type, the space, the radii, the motion; the measured contrast of every pair |
| [components.md](components.md) | the anatomy of every component, its states and the data it takes |
| [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md) | the rules of the page and the check that proves each one |

### 7.1 The concrete inconsistencies in the built tree

Measured by reading the files of the built binary (`bin/bernie`) and the
fixture (`internal/support/fixture/testdata/par1-cs25.tar.gz`), 2026-10-29.

| # | What | Where | The rule it breaks | The fix |
|---|---|---|---|---|
| I1 | The section heading is `h2.section-label` in Setup and Topology, `h2.lg-section-title` in Logs, and `h2.section-label` with a **sentence-case** label in Benchmark (`"The suites"`, `"The result"`, `"The comparison"`, `"The history"`, `"An A/B run"`) | `views/setup/setup.js:72`, `views/topology/topology.js:33`, `views/logs/logs.js:17`, `views/benchmark/benchmark.js:17` | one shape for a section label (tokens.css `--section-label`) | one `sectionLabel(text)` helper in `components/cards.js`; every tab uses it; the label is the uppercase form in every tab |
| I2 | The label of a section is uppercase in Setup, Topology and Logs (`THE PATH TO A SERVING MODEL`) and sentence case in Benchmark (`The suites`) | as above | the same | the same helper decides the case |
| I3 | The toggle is called `Show details` in the rail and `Advanced` in four tab disclosures | `web/index.html:44` vs `views/setup/advanced.js:9`, `views/logs/advanced.js:18`, `views/topology/advanced.js:35`, `views/benchmark/suites.js:225` | one word for one thing | rename every disclosure summary to `Show details`; keep `Advanced` for the tab |
| I4 | The Benchmark tab has **two** controls for the same disclosure: an `Advanced` button in the head (`aria-expanded`) and an `Advanced` summary in the panel | `views/benchmark/benchmark.js:135` and `views/benchmark/suites.js:224` | one control per thing | the button in the head goes; the summary stays |
| I5 | The Setup tab has an `Advanced` button in the head that toggles the *page-wide* switch, so it opens the disclosures of the other tabs too | `views/setup/headline.js:75`, `:85` | a tab control acts on its own tab | the head button opens the Setup disclosure only |
| I6 | The pill vocabulary differs per tab: the Dashboard uses `Serving` / `Starting` / `No run` (title case), the rail uses `serving` / `starting` / `not answering` / `idle` / `stopped` (lower case), the topology uses `ok` / `warn` / `fail` / `skip` (status words), the checklist uses `done` / `todo` / `running` / `blocked` / `skipped` (state words) | `views/dashboard/simple.js:63`, `js/shell.js:259`, `views/topology/topology.js:22`, `components/timeline.js:9` | a state is a word, never a colour — and the same state has the same word | one `statePill(kind, word)` in `components/cards.js`; the four call sites pass the word of their own list |
| I7 | Two icon maps for the same status: `DASH_PILL_ICON` has 7 kinds (`ok warn serious fail skip info running`), `TOPO_PILL_ICON` has 4 (`ok warn fail skip`) | `views/dashboard/dashboard.js:47`, `views/topology/topology.js:22` | one component per concern | one map, in `components/cards.js` |
| I8 | The empty state vocabulary is 24 different sentences, and only 3 carry a command | `views/**`, `components/**` (24 calls to `emptyState`) | an empty state teaches: what, then what to do | one `emptyState(what, next, command)`; every call carries a command (see the table in §4.4) |
| I9 | The buttons that re-read the host are called four different things: `Re-check` (Setup), `Check readiness` (Topology), `Re-check the links` (Advanced), `Try again` (the error state) | `views/setup/headline.js:80`, `views/topology/topology.js:44`, `views/advanced/topology.js:9` | the same action has the same name | `Re-check` everywhere; `Try again` stays for a failed request |
| I10 | The primary action of the Dashboard is a link (`<a class="btn.primary" href="#setup">`) while every other tab uses a `<button class="primary">` | `views/dashboard/simple.js:27` vs `views/setup/headline.js:74` | a control is a button or a link, and a primary action is a button | the Dashboard's primary is a button that starts a run, or the link stays a link but is not styled `primary` |
| I11 | The Dashboard tab has no details disclosure, while Setup, Logs, Topology and Benchmark all have one | `views/dashboard/**` | one disclosure per tab | add the Dashboard disclosure (per §4.3) |
| I12 | The Advanced tab reads no `/api/gpus/history`, but spec §11 says it does | `views/advanced/**` vs spec §11 | the spec is the contract | fix the spec (the Dashboard tab reads it) |
| I13 | The API table of spec §11 has no `GET /api/bench/suites`, but the route exists and the Benchmark tab calls it | spec §11 vs `dashboard/internal/api/api.go:79` | the spec is the contract | add the row |
| I14 | The tab bar's counts exist for Setup, Dashboard and Topology only; Benchmark and Logs have no count even though they have failures (a failed bench, a finding) | `web/index.html:26-34` | a tab shows the number of things that wait | add the count for Logs (the findings) and for Benchmark (the failed runs) |
| I15 | The Setup tab's `Re-check` button reloads the onboarding document but does not re-read the host; the reference's `Re-check this box` does | `views/setup/headline.js:80`, `views/setup/setup.js` (`loadSetupDocument`) | the button says what it does | the button re-reads the host (the doctor and the setup plan), then the document |
| I16 | The tab order of the rail and the keyboard shortcuts disagree: the shortcuts are `g` then `s d b t l c`, which is Setup, Dashboard, Benchmark, Topology, Logs, Advanced — Chat and Fleet have no shortcut | `js/shell.js:140-152` | every tab is reachable | add `g` then `h` for Chat and `g` then `f` for Fleet, or leave the two planned tabs without one on purpose and say so |
| I17 | The rail's `Show details` button opens the disclosures of every tab, including the tabs that are closed, so the state of the button does not match what the reader sees | `js/shell.js:214-227` | one control, one effect | the button opens the disclosure of the tab the reader is on; a second control (in the rail footer, or `g` then `d` for details) opens them all |

### 7.2 The rules that already hold, and must keep holding

- One filled button per tab.
- Every state is an icon and a word.
- Every number has a unit and a range; a missing number is `–`.
- Every command, URL and document has a copy button, and the button copies
  exactly what is shown.
- Every tab opens with a headline and a lede; the headline is `aria-live`.
- Every empty state teaches.
- One question per tab.
- Every field on the page exists in the `--json` output with the same name.
- The page is small: ≤ 430 KB of HTML+CSS+JS, ≤ 24 requests on the first
  paint, ≤ 240 points per chart.

---

## 8. The commit plan

Nine commits. Each is shippable on its own: the binary builds, `make check`
passes, the page works, and each ends with a browser check. The commits are
ordered so that every one of them is safe to stop after.

| # | Commit | Files | What moves | Acceptance check |
|---|---|---|---|---|
| **C1** | `docs: record the information architecture and the inventory` | `docs/design/ia.md` | this file | the file reads; every row of the inventory names a command, a tool or a route that exists |
| **C2** | `spec: fix the two API gaps the inventory found` | `docs/spec.md` | the missing `GET /api/bench/suites` row; the claim about the Advanced tab and `/api/gpus/history`; the run states of the rail pill | `make check`; the API table of spec §11 has 32 rows and matches `dashboard/internal/api/api.go` line for line |
| **C3** | `web: one section label, one pill, one empty state` | `dashboard/web/components/cards.js`, `dashboard/web/views/**` | the `sectionLabel()` and `statePill()` helpers; every tab uses them; every `emptyState` carries a command | no `h2.lg-section-title` and no `h2.section-label` outside `cards.js`; every `emptyState(` call has three arguments; `make check` |
| **C4** | `web: call the disclosures "Show details"` | `dashboard/web/index.html`, `dashboard/web/views/setup/advanced.js`, `views/logs/advanced.js`, `views/topology/advanced.js`, `views/benchmark/suites.js`, `dashboard/web/js/shell.js` | the summary of every disclosure reads `Show details`; the head `Advanced` button of the Benchmark tab goes; the Setup head button opens its own disclosure | the rail button and the four disclosures agree; no string `Advanced` outside the tab name; `make check` |
| **C5** | `web: give the Dashboard tab its details` | `dashboard/web/views/dashboard/*`, `dashboard/web/components/cards.js` | the Dashboard details disclosure: the per-GPU table, the history window, the power read-out, the raw documents | the Dashboard tab has a `Show details` disclosure that opens the four documents; `make check` |
| **C6** | `dashboard: one button that sets the box up` | `docs/spec.md`, `dashboard/internal/api/onboarding.go`, `dashboard/internal/api/api.go`, `dashboard/internal/app/`, `dashboard/web/views/setup/*` | `POST /api/onboarding/run`: the non-root path in one job; the Setup primary button reads `Set up this box`; the root step pauses the path | the button runs the path; the root step stops it with the command; `bernie up` and the endpoint agree; `make check` |
| **C7** | `dashboard: the operator's console is the Advanced tab` | `dashboard/web/views/advanced/*`, `dashboard/web/views/dashboard/*`, `dashboard/web/css/advanced.css` | the old layout leaves the Advanced tab; the per-GPU table, the power write, the doctor report in full and the job list stay; the fleet tiles and the last-minutes charts leave (they are on the Dashboard tab) | the Advanced tab holds the four operator things and no duplicate of the Dashboard; `make check`; a browser check of both tabs |
| **C8** | `dashboard: the Chat tab sends one prompt` | `docs/spec.md`, `dashboard/internal/api/chat.go`, `dashboard/web/views/chat/chat.js` | `POST /api/chat` (streamed, loopback only); the prompt box and the reply | a prompt comes back with the time to first token; an unready run answers 400; `make check` |
| **C9** | `dashboard: the redacted report, and the preferences` | `docs/spec.md`, `internal/app/`, `dashboard/internal/api/`, `dashboard/web/views/advanced/*` | the redacted report (the reference's share button); the preferences panel (the config defaults) | the copied report has no hostname, no username and no token; `make check` |

### 8.1 The risks

| Risk | Where | How it is held |
|---|---|---|
| The Advanced tab loses the old layout that some readers know | C7 | the per-GPU table, the power write, the doctor report and the jobs stay; the old route `#classic` keeps working; the raw documents at the end of the tab are what the old layout's raw list was |
| The Dashboard tab grows too heavy | C5, C7 | the budget in `dashboard/AGENTS.md`; the details are a disclosure, and the raw documents are read when it opens, not on load |
| `POST /api/onboarding/run` duplicates `bernie up` | C6 | the endpoint calls the same `internal/flow` code the CLI calls; the CLI keeps the confirmation, the page has its own button |
| The spec drifts again | C2 and every commit | the rule of `AGENTS.md` §3.1: a change to a command, a flag, a JSON field or a path changes the spec in the same commit |
| The Chat endpoint becomes an open proxy | C8 | loopback targets only, the same rule as `/api/smoke`; the token and `X-Bernie` rules of spec §11 |
| The Fleet tab needs a cross-host token | — | the recommendation of §6.2: the Fleet tab stays a placeholder until the owner answers the token question |

### 8.2 The decisions that belong to the owner, again

D1 (what the Advanced tab is), D2 (Chat and Fleet), D3 (the Dashboard's two
views), D4 (recipe vs profile) and D5 (the device access) are the five.
They are at the top of this file, with their recommendations.

---

## 9. The facts this document is built on

Read, not assumed:

- `bernie --help` and `bernie <command> --help` for the 19 commands and the
  25 leaves.
- `internal/cli/cli.go` (the router, the exit codes, the global flags),
  `internal/cli/cmds/**` (the families), `internal/cli/findings.go` (the
  finding block).
- `internal/mcp/tools_read.go`, `internal/mcp/tools_write.go`,
  `internal/mcp/tool.go`, `internal/mcp/server_test.go:106` (the 19 read
  tools), `internal/mcp/server.go` (the transports and the guard).
- `dashboard/internal/api/api.go:60-90` (the 31 routes),
  `dashboard/internal/api/*.go` (the handlers), `dashboard/internal/sse/`
  (the four streams), `dashboard/internal/telemetry/` (the sampler and the
  ring buffer).
- `dashboard/web/index.html` (the shell and the eight tabs),
  `dashboard/web/js/boot.js` (`VIEWS`, `OLD_ROUTES`, the manifest),
  `dashboard/web/js/shell.js` (the theme, the advanced switch, the run
  pill, the shortcuts), `dashboard/web/js/data.js` (the one data path),
  `dashboard/web/components/*.js`, `dashboard/web/views/**`.
- `docs/spec.md` §3 (the command surface), §3.3 (the checklist), §4.5 (the
  P2P checklist), §5 (the doctor and the log doctor), §7 (the recipes),
  §8 (the runs), §9 (the tuning), §10 (the benchmark), §11 (the dashboard
  API), §13 (MCP).
- `docs/design-dashboard.md`, `docs/design/setup.md`, `docs/design/dashboard.md`,
  `docs/design/benchmark.md`, `docs/design/logs.md`, `docs/design/tokens.md`,
  `docs/design/components.md`, `docs/design/rules-ui.md`.
- `dashboard/AGENTS.md`, `dashboard/README.md`, `AGENTS.md`.
- `~/ui-study/notes.md`, `~/ui-study/design-dashboard-v2.md` and the 29
  screenshots in `~/ui-study/collab/`.
- The reference `vllm-rdna-dash` at HEAD `74fb1d8` and its live demo at
  `http://127.0.0.1:18095/`.
