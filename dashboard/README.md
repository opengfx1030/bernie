# Dashboard

The dashboard is a web page that shows one host: its GPUs, its servers and
their speed. It is a view of the CLI. It does not hold logic of its own.

Spec: [../docs/spec.md §11](../docs/spec.md#11-dashboard) is the contract for
the API. This page is the contract for the UI. Rules for agents are in
[AGENTS.md](AGENTS.md). Rules for contributors are in
[CONTRIBUTING.md](CONTRIBUTING.md).

Status: draft 1. We revisit this page when the UI changes.

## Run it

```bash
bernie dash                       # http://127.0.0.1:7860, this machine only
bernie dash --listen 0.0.0.0:7860 # other machines: a token is required
bernie up                         # starts it at the end of the setup path
```

On a non-loopback address the server prints a token. Open the page with
`/?token=<token>`. The details of the auth rules are in spec §11.

## How it works

```mermaid
flowchart LR
  subgraph Browser
    UI["web/app.js + views/*.js<br/>6 tabs"]
  end
  subgraph "package dashboard"
    API["api.go<br/>JSON + SSE"]
    HUB["hub.go<br/>one sampler, many viewers"]
    AUTH["auth.go<br/>token, Host, CSRF"]
  end
  subgraph "domain packages"
    D["host, gpu, topo, doctor,<br/>recipe, runner, bench"]
  end
  UI -->|fetch / EventSource| AUTH --> API
  API --> D
  HUB --> D
  API --> HUB
```

- The UI calls the JSON API. It never reads the host directly.
- Each endpoint calls the same Go function as the matching CLI command. It
  returns the same JSON as `--json`.
- One sampler reads GPU and server data once per second. The hub sends it to
  every open page (server-sent events). Ten open tabs cost the same as one.
- The page is plain HTML, CSS and JavaScript. It is embedded in the binary.
  There is no build step, no npm and no framework.

## Files

| Path | Job |
|---|---|
| `dashboard.go` | `Serve`: listener, routes, graceful stop. `/mcp` is MCP over HTTP (`internal/mcp`, spec §13.10) with its own guard |
| `api.go` | One handler per endpoint in spec §11 |
| `auth.go` | Token, loopback Host check, `X-Bernie` header on writes |
| `hub.go`, `telemetry.go`, `sse.go` | 1 Hz samples and event streams |
| `internal/support/redact` | Removes secrets from anything the API returns |
| `web/index.html` | Page shell: header, tab bar, one section per tab |
| `web/app.js` | Core: helpers (`h()`, `api()`), shared state, telemetry stream |
| `web/views/<tab>.js` | One file per tab: `viewOverview`, `viewGPUs`, ... |
| `web/boot.js` | Router and start-up. Loaded last. |
| `web/app.css` | Theme (light and dark), layout, components |
| `dashboard_test.go` | API, auth and stream tests |

## The six tabs

Each tab answers one question. Keep it that way.

| Tab | Question it answers | Data | Actions |
|---|---|---|---|
| Overview | Is this host ready, and what do I do next? | `/api/hw`, `/api/doctor`, `/api/runs` | none |
| GPUs | How busy, how hot and at which clock is each GPU? | `/api/telemetry`, `/api/hw` | none |
| Topology | Which GPUs talk fast to which, and will P2P work? | `/api/topo`, `/api/topo/check` | none |
| Recipes | What can I serve, and what will it run? | `/api/recipes`, `.../plan` | start a run |
| Runs | What is running, and why did it fail? | `/api/runs` (state, phase sentence), `.../logs` | stop, read logs |
| Bench | How fast is it, and does it read the whole prompt? | `/api/bench` | start a bench |

`up` starts the dashboard before the server is healthy. The Runs tab then
shows the run as `starting`, says in words that the server is loading the
model, and follows the log of that run. The Overview tab shows the same
state in words.

## UI and UX rules

The goal: a person who has never used vLLM understands the page in one
minute. An agent can read the same page as text.

1. **One question per tab.** If a tab needs two answers, split it.
2. **Say the state in words first.** A badge (`ok`, `warn`, `fail`, `skip`)
   has a word. Color never carries the meaning alone.
3. **Every problem has a fix next to it.** Show the cause and the command to
   run. A copy button sits on every command. Never show a bare error code.
4. **Next step is always visible.** The Overview tab shows one next command
   for the current state (no GPU, no recipe fits, ready to serve, starting,
   running).
5. **Numbers have units and a range.** `18.2 / 29.4 GiB`, not `18.2`. A
   missing value shows `–`, never `0` or `NaN`.
6. **Show what will happen before it happens.** A start button shows the plan
   (command, GPUs, port) first. It does not start on the first click.
7. **Slow work shows progress.** Server start and downloads can take minutes.
   Show one plain sentence ("Loading weights, 4 of 8 GPUs ready") and the log.
   Do not show a spinner alone.
8. **Empty states teach.** No runs? Say what a run is and show the command to
   start one.
9. **Same data as the CLI.** A value on the page also exists in `--json`
   output with the same name. If it does not, add it to the CLI first.
10. **Fast and small.** First paint under 1 s on a local host. No web fonts, no
    images except icons inlined as text, no dependencies.
11. **Works with a keyboard and a screen reader.** Real `<a>`, `<button>`,
    `role=tab`, visible focus, `aria-live` for updates.
12. **Light and dark.** Both follow the system setting. Contrast meets WCAG AA.

## Safe by default

- Loopback only unless you pass `--listen`. Non-loopback needs a token.
- Writes need the `X-Bernie: 1` header. A form on another site cannot send it.
- The page builds DOM with `h()` and never uses `innerHTML`. Server data is
  text, never markup.
- `internal/support/redact` removes tokens and keys from every response.

## Known gaps (revisit)

| Gap | Idea |
|---|---|
| No settings view | Show `config.yaml` values and where they come from |
| No log-error help in the Runs tab | Show the matched rule (id, cause, fix) from the log doctor |
| No demo mode | Serve a fixture host so people can try the UI without GPUs |
| Mobile layout is basic | Check each tab at 390 px wide |
| No screenshot tests | Add a headless-browser smoke test for the six tabs |
