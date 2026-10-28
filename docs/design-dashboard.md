# Dashboard v3 — the exact design

Status: design spec, 2026-10-28. Nothing in this file is implemented yet.

This file is the contract for the front-end workers. The behaviour of the
product is [spec.md](spec.md); this file is the contract for the page.

Read this file, then the sheet of the tab you build:
[design/components.md](design/components.md) (the component catalogue),
[design/setup.md](design/setup.md), [design/dashboard.md](design/dashboard.md),
[design/benchmark.md](design/benchmark.md) and
[design/logs.md](design/logs.md) (the tab sheets). The wireframes are in
`design/*.svg`: one per tab and state, plus one per component. Every file of
this set stays under 400 lines.

## 1. Principles

1. **Clean room, ours.** Every word, colour, name, number and layout in the
   dashboard is written by us. Nothing is copied from another product: no
   markup, no CSS, no text, no assets, no brand, no font by CDN. Facts that
   belong to the machine (a threshold, a sysfs path, a metric name) are
   re-expressed in our own words. The information design we follow — a state
   headline, one primary action, a step timeline, facts with reasons, KPI
   tiles, copy buttons — is a pattern, not an expression. §5 records, per
   component, what the reference does, what we do and why.
2. **Words first.** Every state is a sentence before it is a colour or a
   number. A badge is an icon and a word. A missing number is `–`, never `0`.
   Numbers carry their unit and their range.
3. **One primary action per tab.** Exactly one filled button per tab: the
   action the current state asks for. Every other control is a ghost button.
4. **Plain and legible.** System type, no web fonts, no CDN, no images except
   text glyphs. Short sentences, active voice, second person.
5. **Accessibility is the floor.** Text at 4.5:1, large text and control
   edges at 3:1, a visible focus ring, the whole page with the keyboard,
   `role="tab"` with arrow keys, `aria-live` on the state headline.
6. **No framework.** Plain HTML, CSS and JavaScript embedded in the binary.
   No npm, no build step. DOM is built with `h()`; `innerHTML` never appears.
7. **One question per tab.** Setup: what do I do to make this box serve?
   Dashboard: is it serving, and how fast? Benchmark: how fast is it, and
   does it read the whole prompt? Logs: what did the server say?
8. **390 px is a first-class width.** Nothing is hidden, nothing overflows
   the page, tables scroll inside their card.
9. **The page is a view of the CLI.** A value on the page exists in the
   `--json` output of a command with the same name. If it does not, the
   command goes first (§7 lists the gaps). Empty states teach: no runs? Say
   what a run is and show the command that starts one.

## 2. The shell

The shell is the same on every tab. It is a rail on the left and a content
column on the right.

```
┌────────────┬──────────────────────────────────────────────────┐
│ bernie     │  Setup                                           │
│ host       │  ────────────────────────────────                │
│            │  headline, lede, facts                           │
│ ▸ Setup    │  hero: copy on the left, facts on the right      │
│   Dashboard│  steps, recipes, doctor                          │
│   Benchmark│                                                  │
│   Logs     │                                                  │
│            │                                                  │
│ ● running  │                                                  │
│ [Dark][Ad] │                                                  │
└────────────┴──────────────────────────────────────────────────┘
```

### 2.1 The rail

| Part | Rule |
|---|---|
| Frame | `display: grid; grid-template-columns: 220px 1fr;` the rail is `position: sticky; top: 0; height: 100vh` |
| Padding | `22px 14px` |
| Brand | the word `bernie` in 15 px 800, `margin-bottom: 26px`; the host name under it in 12 px `--muted` |
| Links | one per tab, `border-radius: 8px`, `padding: 8px 10px`, `gap: 2px` between them, 13 px 600; the open one has a `--surface-3` background and a 2 px `--accent` bar on the left |
| Footer | the run status pill, then the theme button, then the Advanced button; `margin-top: auto` |
| Width | fixed at 220 px, not collapsible; at 390 px it becomes a horizontal bar (§8.1) |

### 2.2 The tab bar

Four tabs, in this order: **Setup**, **Dashboard**, **Benchmark**, **Logs**.

The rail links are a real tablist: `role="tablist"` on the container,
`role="tab"` on each link, `aria-controls` pointing at the panel,
`aria-selected` on the open one, a roving `tabindex`, and Left/Right/Home/End
moving the focus. The panel is `role="tabpanel"` with `aria-labelledby` the
tab.

Routes are hash routes: `#setup`, `#dashboard`, `#benchmark`, `#logs`.

Old links keep working. The router rewrites them and replaces the history
entry, so the back button does not walk through them:

| Old route | Opens | Lands on |
|---|---|---|
| `#home`, `#overview` | Setup | the headline |
| `#gpus` | Dashboard | the per-GPU cards |
| `#topology` | Dashboard | the topology section |
| `#runs` | Dashboard | the run card |
| `#recipes` | Setup | the recipe chooser |
| `#bench` | Benchmark | the suite cards |

A section anchor inside a tab is `#dashboard/topology`; the router scrolls to
the section and sets the focus to its heading.

### 2.3 The run status pill

The pill in the rail footer answers "is something serving?".

| State | Icon | Word | Colour |
|---|---|---|---|
| a run answers `/health` | ● | `serving` | `--ok` |
| a run is loading the model | ◐ | `starting` | `--info` |
| a run exists but does not answer | ! | `not answering` | `--warn` |
| no run | – | `idle` | `--skip` |
| the run died | ✕ | `stopped` | `--fail` |

The pill carries the phase sentence as its `title` and its `aria-label`, for
example `starting — Loading the model weights, 2 of 5 shards`. The full
sentence is always written out in the state headline of the tab.

### 2.4 Theme and Advanced

- **Theme.** The button reads `Dark` in the light palette and `Light` in the
  dark one. The palette follows the system by default; the button sets
  `data-theme` on `<html>` and stores the choice in `localStorage`
  (`bernie.theme`). The button has `aria-pressed`.
- **Advanced.** The button opens or closes every Advanced panel of every tab
  at once and stores the state (`bernie.advanced`). It has `aria-pressed`.
  Each tab also has its own Advanced panel with its own toggle, so a tab can
  be opened without the header.

### 2.5 Toast, tooltip, copy

- **Toast.** Fixed at the bottom centre, 2.6 s, `role="status"`,
  `aria-live="polite"`, one at a time. A click dismisses it. It is used for
  a copy, a saved change and a failed write. The text says what happened:
  `Copied the command.`, `Power cap saved. The service applies it.`,
  `Stop failed: the run is already gone.`
- **Tooltip.** The small `i` mark opens a panel with `role="tooltip"`. It
  opens on hover and on focus, closes on Escape and on blur. The tooltip
  never holds the only copy of a fact: the same sentence is in the label of
  the value or in the Advanced panel.
- **Copy.** Every command, plan, URL and JSON document has a copy button
  next to it. The button copies the text exactly as it is shown. The button
  reads `Copy`, then `Copied` for 1.2 s, and the toast confirms. A failure
  toasts `Copy failed — select the text and press Ctrl-C.`

### 2.6 The Advanced panel

Every tab ends with one Advanced panel. It holds two things:

1. **The raw JSON** of the endpoints the tab used, one `<pre>` per endpoint,
   each with a `json` link (the endpoint in a new tab) and a copy button.
2. **The CLI equivalent**: the commands that produce the same documents,
   each in a command block with a copy button.

| Tab | The documents | The commands |
|---|---|---|
| Setup | `/api/onboarding`, `/api/recipes`, `/api/recipes/{id}/plan`, `/api/doctor` | `bernie status --steps`, `bernie status --json`, `bernie recipe list`, `bernie recipe plan ID`, `bernie doctor` |
| Dashboard | `/api/hw`, `/api/topo`, `/api/topo/check`, `/api/doctor`, `/api/runs`, `/api/power`, `/api/telemetry` | `bernie hw`, `bernie topo`, `bernie topo --check`, `bernie doctor`, `bernie ps`, `bernie tune power status` |
| Benchmark | `/api/bench`, `/api/bench/suites`, `/api/bench/compare` | `bernie bench list`, `bernie bench suites`, `bernie bench compare A B` |
| Logs | `/api/runs`, `/api/runs/{name}/logs` | `bernie ps`, `bernie logs NAME` |

### 2.7 Loading, empty and error

| State | What it looks like |
|---|---|
| Loading | three grey skeleton lines and the sentence `Reading the host…`. Never a spinner alone. |
| Empty | two sentences: what the thing is, then the command that makes one. The command is in a command block with a copy button. |
| Error | the message in words, the endpoint that failed, a `Try again` ghost button and the raw answer in a disclosure. |
| Stale | the onboarding document is older than its time to live: a banner `bernie is checking the host again. The values below are the last known state.` with a 3 px `--warn` bar on the left. |

## 3. Tokens

The tokens live in `dashboard/web/css/tokens.css`. The names are ours. The
full palettes, the measured contrast of every pair, the type scale, the
spacing, the radii, the elevation, the motion and the chart colours are in
[design/tokens.md](design/tokens.md). The summary:

| Group | Rule |
|---|---|
| Surfaces | `--bg` `#f4f5f7`, `--surface` `#ffffff`, `--surface-2` `#eef1f5`, `--surface-3` `#e4e8ee` in the light palette; `#0f1216`, `#171b21`, `#10141a`, `#21262e` in the dark one. The dark palette is the system preference; the light one is the override. |
| Lines | `--border` is a decorative hairline; `--border-strong` is the edge of a control and holds 3:1 (`#828a93` light, `#5a6778` dark). |
| Text | `--text`, `--text-2`, `--muted`. `--muted` holds 4.5:1 on every surface. |
| Identity | `--accent` (`#b3122c` light, `#ff5c6c` dark) and `--on-accent`; `--key` (`#8a4f00` light, `#f5c451` dark) is the one number that matters on a screen. |
| Status | `--ok` `--warn` `--serious` `--fail` `--skip` `--info`, a four-step scale and two quiet ones. |
| Charts | `--series-1..6` and `--seq-1..6`. |
| Type | the system stack, no web font, no CDN. Display 900 uppercase with a hard accent shadow; lede 18 px; eyebrow 12 px uppercase; mono 12.5 px. |
| Space and shape | 4 8 12 16 20 24 32 34 48 56 80 px; radii 6 8 9 10 16 18 999 px; two soft shadows, used by the toast and the tooltip only. |
| Motion | 120 ms hover, 200 ms meter, 400 ms progress bar, a 220 ms card entrance. `prefers-reduced-motion` removes all of it. |

Every pair of §3 of [design/tokens.md](design/tokens.md) was measured with
the WCAG 2.1 relative luminance formula. The numbers are the ratios.

## 4. The component catalogue

[design/components.md](design/components.md) holds every component with its
anatomy, its states, the data it takes and the SVG that draws it. The files
are `design/state-headline.svg`, `design/button-primary.svg`,
`design/pill.svg`, `design/cmd-block.svg`, `design/fact-row.svg`,
`design/step-row.svg`, `design/kpi-tile.svg`, `design/area-chart.svg`,
`design/busy-ring.svg`, `design/meter.svg`, `design/table.svg`,
`design/finding-card.svg`, `design/suite-card.svg`, `design/verdict-chip.svg`,
`design/length-curve.svg`, `design/bar-compare.svg`, `design/toast.svg` and
`design/tooltip.svg`.

The shared JavaScript keeps its parts in `dashboard/web/components/`:
`command.js` (the command block and the copy button), `cards.js` (the card
head, the state headline, the empty and the failed state), `charts.js` (the
sparkline, the area chart, the ring, the meter), `metrics.js` (the metric
table, the verdict chip, the bar comparison, the length curve) and
`timeline.js` (the step row). A worker who needs a new part puts it in the
file of its role and tells the other workers.

## 5. Reference parity table

The reference is a dashboard of another project. Its repository carries no
licence, so we write our own code, our own CSS and our own copy, and we
reproduce its layout, its chart types and its interaction patterns. This
table is the swap list: if the author grants a licence, a worker can take
the reference's markup and CSS for one component at a time, without
redesigning anything.

| Component | Reference behaviour and measurement | Our implementation | Status |
|---|---|---|---|
| Frame | 220 px sticky rail, content `padding: 34px clamp(16px, 4vw, 56px) 80px`, view at most 1240 px | the same, in §2.1 | same |
| Rail | brand with 26 px of space under it, links with an 8 px radius and a 2 px gap, a footer with a status pill and a theme button | the same; our links carry our own text labels | same |
| Tab bar | plain links, no `aria-controls` | a real tablist with arrow keys, `aria-selected` and a roving tabindex | adapted (accessibility) |
| Display headline | 38–74 px, italic 900, uppercase, a hard two-layer offset shadow | 900 uppercase with one hard shadow in the accent; the system stack, because bernie bans web fonts and CDNs | adapted (no web font) |
| Lede | 18 px, the second text colour, 70 characters | the same | same |
| Eyebrow | 12 px 800, uppercase, `.14em` | the same | same |
| Section | a 1 px top rule and 24 px of padding, a 13 px uppercase label | the same | same |
| Facts strip | an uppercase 12 px label over a 17 px 700 value, hairline separated | the same, plus the reason under the value | adapted (bernie feature, done) |
| Step timeline | 36 px counter circles, a 2 px connector, one colour per state | the same; the six states are the ones of spec §3.3 | same (done) |
| Command block | mono on the second surface, an 8 px radius, the copy button pinned to the top right | the same | same |
| Primary button | italic 900 uppercase, the key colour, a 4 px hard shadow that moves on `:active` | 900 uppercase, the accent background, a 3 px hard shadow; the accent is bernie's identity | adapted (identity) |
| Ghost button | transparent with a 1 px border and an 8 px radius | the same | same |
| Segmented control | a 9 px radius track, 3 px of padding, a raised active segment | the same; it holds the host/container mode and the history window | same |
| Status pill | an icon, a word and a colour | the same; the word always carries the meaning | same |
| KPI tile | a 30 px 900 number with its unit, a sub-line, a 34 px sparkline | the same; the eight tiles of the fleet, in `web/views/dashboard/gpus.js` | same (done) |
| Hero number | a 64 px 900 number in the key colour with a 44 px sparkline | the same; it carries the output speed of the live run | same (done) |
| Meter | a 6–8 px track, a 4 px radius, a coloured fill, 400 ms | the same; the memory and the power of every card, with the tick of the cap | same (done) |
| Donut ring | an 84 px SVG ring with the number in the middle and a caption under it | the same; the load of every card | same (done) |
| Area chart | an inline SVG, grid lines, axis labels, a five-minute window | the same, with a window the reader picks (5 min, 15 min, 1 h) | adapted (bernie feature, done) |
| Matrix | 2 px gaps, 6 px radius cells, one colour per class | the same; the classes of the PCIe links use `--series-1..4` | same (done) |
| Bar chart | SVG bars for the comparison of two runs | the same; the five metric groups of spec §10.1 are the groups | same (done) |
| Log viewer | 12.5 px mono on near-black, 340 px tall, `pre-wrap` | the same | same |
| Progress bar | a 10 px track, a 6 px radius, the key colour, and one plain sentence under it | the same | same (done) |
| Radio card | a big radio, a title, one line of detail | the same; it picks the benchmark suite | same (done) |
| Toast | fixed at the bottom centre, 2.6 s, `role="status"`, `aria-live="polite"` | the same | same |
| Tooltip | a small mark that opens a fixed panel with `role="tooltip"` | the same, and it opens on focus as well | adapted (accessibility) |
| Banner | the third surface with a 3 px bar on the left, dismissible | the same; it carries the stale and the offline notice | same |
| Copy on every command | one copy button per command, per endpoint and per snippet | the same | same |
| One primary action per view | one filled button, everything else a ghost | the same; the Try it box of the run card is the filled button | same (done) |
| Empty states that teach | what the thing is, then what to do | the same; the run card and the power caps teach | same (done) |
| Chat view | a transcript with a meta line per message | deferred, see §10 | deferred |
| Fleet view | a list of other boxes | out of scope: bernie is one host | out of scope |

## 6. The four tabs

| Tab | Question | Sheet | Wireframes |
|---|---|---|---|
| Setup | What do I do to make this box serve? | [design/setup.md](design/setup.md) | `design/setup-1280.svg`, `design/setup-390.svg`, `design/setup-blocked-1280.svg` |
| Dashboard | Is it serving, and how fast? | [design/dashboard.md](design/dashboard.md) | `design/dashboard-1280.svg`, `design/dashboard-390.svg`, `design/dashboard-empty-1280.svg` |
| Benchmark | How fast is it, and does it read the whole prompt? | [design/benchmark.md](design/benchmark.md) | `design/benchmark-1280.svg`, `design/benchmark-390.svg`, `design/benchmark-empty-1280.svg` |
| Logs | What did the server say? | [design/logs.md](design/logs.md) | `design/logs-1280.svg`, `design/logs-390.svg`, `design/logs-empty-1280.svg` |

Every tab opens with a state headline: one sentence that says what the
situation is, and one lede under it that says what to do next. The headline
is `aria-live="polite"`.

## 7. Data and API gaps

The widgets below need a field or an endpoint that does not exist yet. The
proposal is written so that a backend worker can implement it without
asking. Everything else in this design reads an endpoint of spec §11 as it
is.

| # | Widget | What is missing | Proposal |
|---|---|---|---|
| G1 | The facts with their reasons on Setup | the onboarding document has no facts | add `facts` to the document of §3.3: `{"facts": [{"key": "gpus", "label": "GPUs", "value": "8 × Radeon PRO V620", "why": "Every card is gfx1030, so one recipe family fits them all."}]}`; the CLI prints the same array in `bernie status --steps` |
| G2 | The progress of the model download | the job document has no numbers | add `progress` to the job document: `{"progress": {"phase": "model.download", "bytes_done": 1800000000, "bytes_total": 2300000000, "rate_bps": 45200000, "files_done": 7, "files_total": 14, "eta_s": 11}}`; the existing `done`/`total` stay for the steps that count items |
| G3 | The "Try it" box on Setup and on the Dashboard | there is no endpoint for one prompt | new `POST /api/smoke`, body `{"prompt": "Hello", "run": "flashnext"}`: it calls the same `onboarding.Smoke` as `bernie smoke` with the prompt of the body, and answers `{"url": "http://127.0.0.1:8000", "model": "flash-next", "health": true, "reply": "…", "ttft_ms": 214, "output_tokens": 12, "tok_s": 56.1}`; `ttft_ms`, `output_tokens` and `tok_s` are new fields of `onboarding.SmokeResult`; the target is the running run or the one named by `run`, and a name that is not running is a 404 |
| G4 | The history charts of the Dashboard | the telemetry stream holds one sample, no history | new `GET /api/gpus/history?window=15m`: `{"window": "15m", "interval_s": 2, "points": 450, "series": [{"hip_index": 0, "busy_pct": [0, 12, 30], "power_w": [11.2, 18.4, 22.0], "vram_used": [24117248, 24117248, 24117248], "tokens_per_s": [0, 0, 0]}]}`; the ring buffer holds one sample per GPU every 2 s for 30 minutes (900 points) in the hub; `window` is `5m`, `15m` or `1h`; `1h` is downsampled to one point per 10 s; the answer is at most 120 KB and is cached for 2 s |
| G5 | The log viewer with levels and a search | the log stream sends plain lines | new `GET /api/runs/{name}/log?tail=2000&level=error&q=oom`: `{"name": "flashnext", "state": "exited", "total_lines": 8123, "truncated": false, "lines": [{"no": 4211, "level": "error", "text": "…"}]}`; `level` is `all`, `error`, `warn` or `info`; `q` is a case-insensitive substring; the lines are the tail of the log, newest last |
| G6 | The findings of the log doctor for one run | the findings only ride on `ps --json` for an exited run | new `GET /api/runs/{name}/findings`: `{"name": "flashnext", "matches": [{"id": "memory.kv-too-small", "severity": "error", "cause": "…", "fix": "bernie up flashnext --set args.max-model-len=16384", "doc": "…", "line": "…", "line_no": 82}]}`; it is the scan of `doctor --run`, so the same rules and the same exit code apply |
| G7 | The live progress of a benchmark | `POST /api/bench` runs to the end and answers the result | new `POST /api/bench/run` with the same body as `POST /api/bench`: it answers `202` and `{"id": "b-7f3a"}`; new `GET /api/bench/{id}` answers `{"id": "b-7f3a", "status": "running", "phase": "cell 2 of 6: 1k at concurrency 4", "done": 2, "total": 6, "result_id": ""}`; new `GET /api/bench/{id}/events` is a server-sent stream with `progress`, `log` and `done` or `failed` events; the old `POST /api/bench` stays as it is |
| G8 | The KV cache of the live run | it exists | `GET /api/telemetry`, `runs[].stats.kv_cache_usage` (a fraction from 0 to 1) | none |
| G9 | The ECC state of a card | it exists | `GET /api/doctor`, the check whose id is `gpu.ecc.<hip index>` | none |
| G10 | The endpoint and the age of a run | they exist | `GET /api/runs`, `url`, `started_at` | none |
| G11 | The receipt of a comparison | the compare endpoint answers the table only | `GET /api/bench/compare?a=ID&b=ID&format=markdown` answers `text/markdown`: the same text as `bernie bench compare A B --markdown` |
| G12 | The A/B run | the CLI has it, the API does not | `POST /api/bench/ab` with `{"base": "flashnext", "cand": "http://127.0.0.1:8001"}` answers `202` and `{"id": "b-7f3a"}`; the job endpoints of G7 carry it |

Rules for the new endpoints: they live in `dashboard/`, they call the same
`internal/app` function as the matching command, they answer the same JSON
as `--json`, writes keep the `X-Bernie` header check and the token rules of
spec §11, and every answer goes through `internal/support/redact`. A new
field in a domain struct changes `docs/spec.md` in the same commit.

## 8. Responsive, accessibility and performance

### 8.1 Responsive

| Width | The rules |
|---|---|
| ≥ 1280 px | the rail is 220 px, the content is at most 1240 px, the hero is two columns, the KPI tiles are six across |
| 960–1279 px | the tiles are four across, the GPU cards three |
| 640–959 px | the rail is 200 px, the hero stacks, the columns are two |
| ≤ 639 px | the rail becomes a horizontal scrolling bar under the brand, the tiles are two across, the charts and the GPU cards are full width, a table scrolls inside its card, the hero number is 40 px |
| 390 px | the page never scrolls sideways; a table scrolls inside its card; nothing is hidden |

### 8.2 Accessibility checklist

- [ ] The tab bar is a real tablist: `role="tablist"`, `role="tab"`, `aria-controls`, `aria-selected`, a roving `tabindex`, Left/Right/Home/End move the focus.
- [ ] Every control is a real `<button>` or `<a>`; a copy button is `type="button"`, so it never submits a form next to it. The focus ring is visible on every control: a 2 px `--accent` ring with a 2 px offset.
- [ ] Every status is an icon and a word. No meaning is carried by colour alone.
- [ ] Every chart has a text form: the same numbers are in a tile, a table row or a sentence.
- [ ] The state headline is `aria-live="polite"`; the toast is `role="status"`; the tooltip is `role="tooltip"`.
- [ ] The log viewer is `role="log"` with `aria-live="off"`; it is read on demand, not announced line by line.
- [ ] Every touch target is at least 24 × 24 px, and every control is a real `<button>` or `<a>`.
- [ ] `prefers-reduced-motion` removes every animation (§3.3).
- [ ] The page works with the keyboard alone: no trap, Escape closes a disclosure and a tooltip.
- [ ] Both palettes meet the ratios of §3.
- [ ] The empty, the loading and the error states say what happened in words.

### 8.3 Performance

| Budget | Value |
|---|---|
| The whole page | at most 140 KB of HTML, CSS and JavaScript, uncompressed, no images and no fonts; first paint under 1 s on a loopback host |
| The files | HTML at most 12 KB, CSS at most 40 KB in four files, JavaScript at most 90 KB in the core, the components and the four views |
| The live numbers | one stream: `GET /api/telemetry` at 1 Hz drives every tile, every ring, every meter and every sparkline |
| The poller | one poller for the page, in `js/data.js`; no view opens its own timer. The onboarding document is re-read every 15 s only while a job runs |
| The charts | the history is read once per window change and then appended from the telemetry stream; the view never re-reads it |
| The payload of the history | at most 120 KB for a 15-minute window; the ring buffer answers at most 900 points per GPU |

## 9. The old tabs, the commit plan and the owners

### 9.1 Where the old tabs go

| Old tab | New home | What moves |
|---|---|---|
| Home (`#home`, `#overview`) | Setup | the headline, the facts, the step timeline, the doctor card, the run card |
| GPUs (`#gpus`) | Dashboard | the tiles, the per-GPU cards, the power caps |
| Topology (`#topology`) | Dashboard | the PCIe tree, the link classes, the readiness checklist, in a section of their own |
| Recipes (`#recipes`) | Setup | the recipe list, the fit, the plan, the settings with their reasons, the mode switch |
| Runs (`#runs`) | Dashboard | the run card: state, phase, endpoint, uptime, stop, restart, the findings of the log doctor |
| Runs, the log | Logs | the live tail, the filter, the findings, the download |
| Bench (`#bench`) | Benchmark | the suites, the run, the results, the comparison, the length curve, the history |

### 9.2 The commit plan

Each commit is shippable on its own: the binary builds, the tests pass and
the page works. Each ends with a browser check.

| # | Commit | Files | The browser check |
|---|---|---|---|
| 1 | `dashboard: rebuild the shell as a rail with four tabs` | `web/index.html`, `web/css/tokens.css`, `web/css/layout.css`, `web/css/components.css`, `web/js/shell.js`, `web/js/boot.js` | Four tabs in the rail; `#home` opens Setup; the theme button flips the palette; the Advanced button opens the panels; nothing scrolls sideways at 390 px |
| 2 | `dashboard: draw the Setup tab as a timeline` | `web/views/setup/setup.js`, `web/views/setup/steps.js`, `web/views/setup/recipes.js`, `web/views/setup/advanced.js`, `web/components/timeline.js`, `web/components/cards.js` | The eight steps of `/api/onboarding` appear in order; Run starts a job and the output lands under the step; the root step shows a command with a copy button and no Run button |
| 3 | `dashboard: draw the Dashboard tab with the fleet tiles` | `web/views/dashboard/dashboard.js`, `web/views/dashboard/gpus.js`, `web/views/dashboard/power.js`, `web/views/dashboard/topology.js`, `web/views/dashboard/overview.js`, `web/components/charts.js` | The tiles follow `/api/telemetry`; the ring shows the load of the card; the power meter shows the cap; the topology section shows the matrix and the checklist |
| 4 | `dashboard: move the bench tab to Benchmark` | `web/views/benchmark/benchmark.js`, `web/views/benchmark/suites.js`, `web/css/bench.css`, `web/components/metrics.js` | The `pr` suite runs; the five metric groups are in the table; the comparison draws the bars and the verdict chips; the receipt copies as markdown |
| 5 | `dashboard: draw the Logs tab` | `web/views/logs/logs.js` | The tail follows the run; the level filter narrows it; a finding copies its fix; the download writes a file |
| 6 | `dashboard: answer the new endpoints of the design` | `docs/spec.md`, `internal/onboarding/`, `internal/app/`, `dashboard/internal/api/history.go`, `dashboard/internal/api/smoke.go`, `dashboard/internal/api/logview.go`, `dashboard/internal/api/benchrun.go`, `dashboard/internal/api/*_test.go` | `GET /api/gpus/history?window=15m`, `POST /api/smoke`, `GET /api/runs/{name}/log`, `GET /api/runs/{name}/findings`, `POST /api/bench/run` answer the JSON of §7 |

### 9.3 The owners

Five workers work in parallel. Each owns the files of its row and touches
nothing else. `js/core.js` (`h()`, `put()`, `api()`, `num()`, `bytes()`) and
`js/data.js` (the state, the fetches, the streams) stay with worker 1 and
change only by agreement.

| Worker | Files | The seam |
|---|---|---|
| 1 shell and tokens | `web/index.html`, `web/css/*`, `web/js/shell.js`, `web/js/boot.js`, `web/js/core.js`, `web/js/data.js`, `web/components/command.js` | owns the tokens and the shell; publishes `cmdBlock()`, `copyBtn()`, `cardHead()`, `stateHead()`, `setHead()`, `emptyState()`, `loading()`, `failed()`, `api()`, `openTelemetry()`, `openRunLogs()` |
| 2 Setup | `web/views/setup/*`, `web/components/timeline.js` | publishes `stepTimeline(steps, options)` |
| 3 Dashboard and charts | `web/views/dashboard/*`, `web/components/charts.js` | publishes `spark()`, `areaChart()`, `ring()`, `meter()` |
| 4 Benchmark | `web/views/benchmark/*`, `web/css/bench.css`, `web/components/metrics.js` | publishes `metricTable()`, `verdictChip()`, `barCompare()`, `lengthCurve()` |
| 5 Logs | `web/views/logs/*` | publishes nothing; it uses the components of the other workers |

## 10. Deferred: the chat tab

Chat is not in this version. The reference has one; a client that wants it
asks for a transcript, a system prompt, a parameter panel and a streaming
reply, and that is a fifth tab of its own. The sketch is kept so it can be
revived without a redesign:

```
POST /api/chat  {"run": "flashnext", "model": "flash-next", "stream": true,
                 "messages": [{"role": "user", "content": "hi"}],
                 "temperature": 0.7, "max_tokens": 256, "reasoning": false}
200 text/event-stream, one chunk per event; 400 when the run is not ready;
401 without the token; 403 without X-Bernie
```

It would proxy the OpenAI API of the running run, to a loopback target only.
The cheap version of the same idea ships now: the **Try it** box on the
Dashboard run card and on the verify step of Setup (G3) sends one prompt and
answers with the text, the time to first token and the tokens per second.
