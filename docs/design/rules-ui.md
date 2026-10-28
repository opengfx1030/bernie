# Dashboard rules — the long reference

The rules live in [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md). This
file holds the reasoning behind them: the sources, the detail of every rule
and the ranked changes that carry them out. Nothing here is a rule of its
own; when the two disagree, `dashboard/AGENTS.md` wins.

Three research passes fed these rules:

- the Setup and onboarding study (Setup tab, wizard and progress guidance),
- the data-display study (Dashboard and Benchmark tabs, charts and tables),
- the lean front-end study (payload, CSP, accessibility, budgets).

## 1. Setup and onboarding rules

The Setup tab answers one question: what do I do to make this box serve?
The path is the 8 steps of spec §3.3 plus the 3 optional ones; a step is
`done | todo | running | blocked | skipped`.

Sources: NN/g *Wizards* (the path, the order, the labels, resumability),
NN/g *Progress Indicators* and *Response Times* (what a slow step needs),
NN/g *Error-Message Guidelines* and *User Control and Freedom* (cause, fix,
undo), Conrad et al. 2010 (progress and abandonment), clig.dev (the CLI
contract), GOV.UK Design System (question pages, check answers, details,
tabs), Primer (feature onboarding, empty states).

The measured facts behind the rules: the tab is one ordered list with a
state word per row; every runnable step carries a filled `Run`; a root row
has the command but no re-check; the progress bar always renders
`aria-valuenow` and therefore reads `0 %` when the fraction is unknown; the
onboarding document carries no job id, so a reload during a model download
cannot re-attach to the running job; jobs *are* persisted on disk, so the
data survives and only the page cannot find it.

Two rules need a spec change before they can be built: the `job_id` on the
step (or a `GET /api/jobs?state=running` endpoint) and a stop endpoint for a
running step. The other ten are UI work.

Rejected, with the reason: a modal wizard (the path is observed, not typed,
and a modal hides the doctor findings); a percentage for every step (a fake
percentage is the most damaging thing on the tab); teaching bubbles (the
reader has exactly one task); a real undo (the steps are idempotent, and a
real undo would delete a venv or a model).

## 2. Data display rules

Sources: WCAG 2.2 SC 1.4.1, 1.4.11, 2.1.1, 2.4.7, 2.5.8, 4.1.3; Grafana
dashboard best practices (answer a question, symptom before cause, compare
like to like, soft min/max); Google SRE (the mean of a quantity is
dangerous, dashboards answer basic questions); the EU data visualisation
guide (zero-based axes, no double y-axis, small multiples, direct labels);
the Royal Statistical Society guide (tables for exact values, right-aligned
numbers, colour is a weak channel); Carbon (palettes, simple charts,
notification severity); Tufte (sparklines are data only); Cleveland & McGill
1984 (position and length beat angle and area); Datawrapper and Paul Tol
(the colour-blind safe pairs: blue + orange).

The defects the rules answer, measured in the v3 screenshots:

- every sparkline auto-scales to its own data maximum, so a flat series
  draws as a solid block at the top of the box (fixed scale, no fill);
- the chart says "now" on its right edge even when the last sample is a
  minute old (the age of the newest sample);
- the length curve puts prefill and TTFT on two y-axes with their own
  maxima, so two unrelated lines look parallel (two stacked charts);
- the Benchmark headline number cannot be found in the table without
  knowing which cell it came from (name the cell);
- the bar comparison rescales every metric group to its own pair, so a 2 %
  change and a 40 % change look the same (one scale, or indexed to 100);
- the comparison takes the lowest-concurrency cell per metric, so the five
  bars can come from five different cells (compare cell by cell);
- the table mixes `TTFT S` and `ITL MS` (one unit per measure).

Owner decisions: the GPU load becomes a **0–100 % bar** in the Dashboard tab,
because an arc is the weakest encoding in the product and the number already
printed inside it carries the information. The **Classic tab keeps the
ring**: it is the reference-parity port, and the two are compared side by
side. `--series-5` and `--series-6` stay in the token file, but they may not
be the only distinction between two categories; a second channel (label,
dash, shape) always separates them.

Not recommended: a gauge library, a donut for the fleet mix, 3-D charts, a
second y-axis anywhere, alert banners on cause-level metrics. Material is
not a source for chart rules: Material 2 is unmaintained and Material 3 has
no chart guidance.

## 3. Front-end rules

Sources: W3C WCAG 2.2 (2.1.4 character key shortcuts, 2.5.8 target size,
1.4.3 contrast, 4.1.3 status messages); MDN (Page Visibility, requestAnimationFrame);
OWASP (CSP, CSRF); web.dev (font best practices, INP, long tasks, Core Web
Vitals); Go documentation (`http.ServeContent` and ETags); Playwright and
axe-core for the browser harness.

The defect that matters most: the served CSP is `style-src 'self'`, and the
browser drops every `style` attribute. The code still writes them in the
charts and in several views, so the SVG paths compute to `stroke:none;
fill:black` and the charts draw nothing. A static style belongs in a class
in `web/css/`; a runtime value is written through the CSSOM.

The rest of the measured state: the page is 2.6× over its JavaScript budget
and 1.5× over its CSS budget, and nothing measures it; there are 40 requests
on the first paint and every response is `no-store`, so a reload
re-downloads everything; the poller never checks `document.hidden`; a second
1 Hz timer lives in a view next to the SSE stream; a `g`-then-letter
shortcut works from anywhere (SC 2.1.4 covers sequences); the info button
is 18×18 and the JSON link is about 11 px (SC 2.5.8 wants 24×24); the
horizontal tab list consumes Up/Down (the APG forbids it).

Ranked changes, by reader harm avoided over effort:

1. Move the chart and series styles into CSS classes; set dynamic values
   with the CSSOM. Fixes the blank charts.
2. Fail the build on inline styles: a Go test walks `web/**/*.js` and
   refuses `style:`, `setAttribute("style"` and `innerHTML`, and checks
   that every file named in `index.html` exists.
3. Pause the poller and the repaints while the tab is hidden; refresh once
   on return.
4. One update path, one frame: delete the extra 1 Hz timer, let the SSE
   sample drive the repaint, batch the repaint in one
   `requestAnimationFrame`.
5. Cheap charts: cap the drawn points at 240, build the axes once, write
   only the path `d` on an update, keep the full series for the table.
6. Cache and compress the assets: gzip with an `ETag` and
   `Cache-Control: no-cache` for the static assets, `no-store` for `/api`,
   the HTML and the streams. Needs the spec §11 change first.
7. 24 px targets, 44 px for the main action of a card.
8. Fix the keyboard defects: the letter shortcut must satisfy SC 2.1.4,
   the horizontal tab list must not consume Up/Down, a panel without a
   focusable child takes `tabindex="0"`.
9. Harden the CSP (`base-uri 'none'; object-src 'none'; form-action 'self'`)
   and the write guard (`Sec-Fetch-Site`, `Origin`).
10. Live-region hygiene: the run pill announces state changes only.
11. Table semantics (`scope`, `<caption>`) and a data table under every
    chart.
12. A dev-only browser harness: Playwright opens the tabs, fails on any
    console error, screenshots each tab (light and dark, 1280 and 390) and
    runs axe-core. Not shipped in the binary, not part of `make check`.
13. Reconcile the docs: the "no web fonts" rule (four self-hosted woff2
    ship), the stale budget, and these rules.

## 4. What is measured and what is not

Measured on 2026-10-29 from the files under `dashboard/web/**`: the byte
counts, the request count and the payload. Not measured: the gzip ratio,
every Core Web Vital, the 1 Hz repaint cost on a ten-GPU host and the
44×44 figure of SC 2.5.5. The budget table in `dashboard/AGENTS.md` marks
what is a number and what is still a target.
