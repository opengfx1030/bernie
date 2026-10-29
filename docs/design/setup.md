# Dashboard v3 — the Setup tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **what do I do to make this box serve?**

Wireframes: `design/setup-1280.svg` (the machine that is ready), 
`design/setup-390.svg` (the same at 390 px),
`design/setup-blocked-1280.svg` (a step that needs root and a step that
runs).

## The order of the page

1. The state headline and the actions of the tab.
2. The hero: the plan in words on the left, the facts on the right.
3. The section `THE PATH TO A SERVING MODEL`: the step timeline.
4. The section `THE RECIPE`: the chooser and the plan.
5. The section `DOCTOR`: the findings.
6. The section `ADVANCED`: the raw documents and the commands.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline and the lede | `GET /api/onboarding` → `next`, `steps[].state` | on open, after every job event, and every 15 s while a job runs | `BERNIE IS CHECKING THE HOST.` with the lede `Reading the host. The checklist arrives in a moment.` | `The state of this machine is unknown.` and the lede `The dashboard could not read /api/onboarding.` plus `Try again` |
| 2 | The primary action | the same document, `next` | with the headline | the button is replaced by the sentence `Nothing is left to do.` | the button is disabled and reads `Try again` |
| 3 | The facts strip | `GET /api/onboarding` → `facts[]` (G1); `GET /api/hw` and `GET /api/doctor` fill it until G1 lands | on open and every 15 s | the strip is hidden and the hero shows only the plan | one fact shows `–` and the reason `The dashboard could not read the host.` |
| 4 | The step timeline | `GET /api/onboarding` → `steps[]` | on open and after every job event | `There is no checklist on this host.` and `The checklist is the ordered list of steps that lead from a bare machine to a serving model. It comes from the doctor report, the setup plan and the runs.` plus the command `bernie status --steps` | `The dashboard could not read the checklist.` plus `Try again` |
| 5 | The model download progress | the job of the `model` step: `GET /api/jobs/{id}` → `progress` (G2) and `GET /api/jobs/{id}/events` | the job stream, every event | the step says `The model is not on this host yet.` | the step says `The download stopped.` and shows the last line of the job |
| 6 | The recipe chooser | `GET /api/recipes` → `recipes[]`, `errors[]` | on open | `There is no recipe to serve.` and `A recipe is a model, the flags it needs and the way it is served. The recipes live in the recipes/ directory of the repository.` | the load errors are listed under the heading `LOAD ERRORS`, one line per file |
| 7 | The fit of a recipe | the same document, `recipes[].fit` | with the chooser | the recipe says `fit: unknown` and `No GPU data on this host.` | the recipe says `fit: unknown` and the reason of the error |
| 8 | The plan of the picked recipe | `GET /api/recipes/{id}/plan?set=K=V&mode=host` → `settings[]` (the setting, the value and the reason), `adjustments[]`, the command, the GPU selection | on open, and again when the mode or an override changes | `The plan is not ready.` and the reason of the failure | `The plan could not be built.` plus `Try again` |
| 9 | The mode switch | `GET /api/recipes/{id}/plan?mode=host|container` | on change | the segmented control shows one segment only when the recipe has no other mode | the switch keeps the old mode and the card shows the error |
| 10 | The doctor findings | `GET /api/doctor` → `checks[]`, `summary` | on open and on `Re-run` | `The doctor found nothing to report.` and `Every check of the host passed.` | `The doctor could not run.` plus the command `bernie doctor` and `Try again` |
| 11 | The verify step and the `Try it` box | `POST /api/smoke` (G3) | on demand | the box is disabled with the sentence `Start the server first.` | the box shows `The server did not answer.` and the reason |
| 12 | The stale banner | `GET /api/onboarding` → `stale`, `facts_age_s` | with the document | the banner is hidden | the banner is hidden |
| 13 | The Advanced panel | `GET /api/onboarding`, `GET /api/recipes`, `GET /api/doctor` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The steps of the timeline

The eight steps of the checklist of spec §3.3, in this order, and the three
extras after them:

| Order | Id | Title | What the row shows |
|---|---|---|---|
| 1 | `hardware` | GPUs found and supported | the doctor checks of the `gpu` category |
| 2 | `access` | Device access and groups | the `gpu.groups` check and its fix |
| 3 | `rocm` | ROCm runtime and kernel | the `rocm` category and its fix |
| 4 | `python-env` | Python environment (torch and vLLM) | the `python` category and the venv it used |
| 5 | `model` | Model files | the download progress and its sentence |
| 6 | `recipe` | Recipe | the fit of the best recipe and the reason |
| 7 | `serve` | Serve the model | the state of the run and its phase |
| 8 | `verify` | The server answers a request | the `Try it` box |
| 9 | `bench` | Measure the speed (optional) | a link to the Benchmark tab |
| 10 | `tuning` | TunableOp profile (optional) | a link to the Dashboard tab |
| 11 | `power` | Power cap (optional) | a link to the Dashboard tab |

The titles are the titles of the code (`internal/onboarding/steps.go`), the
same words that `bernie status --steps` prints.

The step the reader does next is the **current** step: it carries the word
`next`, its row carries `aria-current="step"`, and its button is the filled
one. Every later step of the path carries the word `queued` and a ghost
button that names the step it waits for; clicking it says what comes first.

A button names the action of its step, never only `Run`: `Build the Python
environment`, `Download the model`, `Start the server`, `Ask the server`.
The button of a step that is done says the action again, with `again` at
the end: `Download the model again`.

A step that needs root has the exact command, a copy button and a
`Re-check` button that reads the host again. bernie never runs a command
as root.

The recipe step carries the recommended recipe and why it is recommended:
`Recommended: Qwen3.8 Flash-Next AWQ - it fits this host: gfx1030, 4 GPUs.`

## The progress of a step

A step that runs shows a sentence, a bar and the output of the job.

The bar is drawn only when the work is measured: the job carries the bytes
or the files it has done and the total it has to do. A step that measures
nothing shows no bar, no `aria-valuenow`, and the line under the sentence
says `working… 42 s`. A bar of `0 %` for work that is only unmeasured is a
lie.

The line under the sentence holds the numbers that were measured:
`43% - 1.8 GB of 2.3 GB at 45.2 MB/s`. The time that is left is shown only
when the server measured it (`about 11 s left`); when the work takes longer
than the estimate, the tab says so in words (`longer than the 11 s
estimate`).

The output of the job lands under the step and stays open.

## A step survives a reload

The checklist names the job of a step that runs: `job_id` and `job_state`
on the step. On load the tab opens the event stream of that job, which
replays its output from the first line, and shows its progress. A step
whose job runs has no `Run` button: the work is already going.

A job that finished less than five minutes ago is still named by the
checklist: the tab shows its last output and its result.

## The headline and its action

| State of the host | The headline | The lede | The primary action |
|---|---|---|---|
| The checklist is on its way | `BERNIE IS CHECKING THE HOST.` | `Reading the host. The checklist arrives in a moment.` | none |
| There is no GPU bernie can use | `THIS BOX HAS NO GPU FOR BERNIE.` | `bernie needs an AMD card with /dev/kfd. The doctor report says what is missing.` | `Read the doctor report` (a ghost link to the doctor card) |
| Steps are left | `N STEPS LEFT BEFORE THIS BOX CAN SERVE.` | the next action and who does it: `Next: build the Python environment. bernie does it here.`; when a step needs root: `Copy the command, run it as root in a terminal, then press Re-check.` | the action of the next step, for example `Build the Python environment`; `Copy the root command` when the next step needs root |
| A step runs | `SETTING UP THIS BOX.` | the phase sentence of the step, for example `Downloading the model weights, 2 of 5 shards` | `Run the rest of the steps` |
| The path is complete and nothing serves | `THIS BOX IS READY TO SERVE.` | `Everything bernie needs is in place. Start the server and it answers on port 18096.` | `Start the server` |
| A run answers | `THIS BOX IS SERVING.` | `The model answers on port 18096. Try it, or read the logs.` | `Try it` (the smoke box) |

`N` is the number of steps of the path that are neither done nor skipped.
The lede never names a mechanism: it names the state and the consequence.

## The recipe chooser

The chooser is a list of radio cards, one per recipe, each with the name,
the status (`validated`, `experimental`, `reference`), the number of GPUs
it wants, its arch and its fit. A recipe that does not fit says why in one
line: `no: 4 GPUs of gfx1030 are needed, this host has 2`.

The recipe that fits this host comes first: it is the recommended choice.
Its card carries the word `recommended` and the reason: `recommended: it
fits this host: gfx1030, 4 GPUs.` The recipes that this host cannot tell
come next, the ones that do not fit come last.

The plan card under the chooser holds:

| Part | Data |
|---|---|
| The launch command | `plan.command`, in a command block with a copy button |
| The GPU selection | `plan.gpus` and the reason of the selection |
| The settings | `plan.settings[]`: the setting path, the value and the reason, one row per setting |
| The adjustments | `plan.adjustments[]`: what the plan changed to fit, and why |
| The mode | a segmented control: `host` and `container` |

The `Start the server` button is the primary action of the plan card when
the tab has no other primary action. It is the only other filled button of
the tab, and it is disabled while the plan is not ready: a recipe that does
not fit this host cannot be served from here, and the button says so
(`The plan is not ready: fit: unknown - No GPU data on this host.`).

## The copy of the tab

| Place | The text |
|---|---|
| The plan sentence of the hero | `bernie picked the setup that fits this host. Anything that needs a password is shown as a command to paste.` |
| The step that needs root | `This step needs root. Run it in a terminal, then re-check.` |
| The lede when a step needs root | `Copy the command, run it as root in a terminal, then press Re-check.` |
| The button of the current step | the action of the step, for example `Download the model` |
| The button of a step that is done | the action of the step with `again` at the end, for example `Download the model again` |
| The word on the current step | `next` |
| The word on a later step | `queued` |
| The progress of work that is not measured | `working… 42 s` |
| The estimate of a step | `about 11 s left`, or `longer than the 11 s estimate` when the work takes more time than that |
| The button of the doctor card | `Re-run` |
| The try-it box | `Ask the server something.` and the placeholder `What is the capital of Portugal?` |
| The answer of the try-it box | the text of the reply, then `214 ms to the first token · 56.1 tok/s` |
| The section `DOCTOR` when all is well | `Every check of the host passed.` |
