# AGENTS.md (dashboard)

Rules for AI agents that change `dashboard/`. The repo rules in
[../AGENTS.md](../AGENTS.md) also apply. Read [README.md](README.md) first:
it holds the UI and UX rules.

## What this folder is

A Go package (`package dashboard`) plus a static web UI in `web/`. It is a
view of the CLI. It adds no logic.

## Hard rules

1. **No new logic here.** A value comes from a domain package. If the data does
   not exist, add it to the CLI and its `--json` first. Then expose it here.
2. **Spec first.** A new or changed endpoint is written in spec §11 before the
   code. The UI follows the endpoint, not the other way round.
3. **No dependencies.** No npm, no bundler, no framework, no CDN, no web
   fonts. The UI is plain HTML, CSS and JavaScript embedded with `go:embed`.
4. **No `innerHTML`.** Build DOM with `h()` in `web/app.js`. Server data is
   text.
5. **Writes are guarded.** Every POST and DELETE keeps the `X-Bernie` header
   check and the token rules in `auth.go`. Never loosen them.
6. **Redact.** Anything sent to the browser goes through `internal/support/redact`.
7. **One question per tab.** Do not add a seventh tab for a feature that fits
   an existing one. Do not put two questions in one tab.
8. **Files stay small.** One file per tab in `web/views/`. A view file that
   reaches 400 lines is split by card (`views/<tab>_<card>.js`) and loaded from
   `index.html`. Go files follow the repo limit.
9. **Same names as the CLI.** A field on the page has the JSON name of the
   CLI output.

## Where to change what

| Task | Files |
|---|---|
| New endpoint | spec §11, `api.go`, `dashboard_test.go` |
| New view or card | `web/views/<tab>.js` (one `view*` function), `web/app.css` |
| New tab | `web/index.html` tab link, section and script tag, `views` map in `web/boot.js`, a `GET /views/...` route is already generic, this README table |
| New live value | `telemetry.go`, `hub.go`, the sample type in `gpu`/`runner`, spec §11 |
| Change colors or layout | `web/app.css`: use the existing `--` variables |
| Auth change | `auth.go`, spec §11, a test that shows the old attack still fails |

## Check your work

```bash
make check                          # gofmt, vet, all tests
go test ./dashboard/...             # API, auth, streams
bernie dash --listen 127.0.0.1:7860
```

For a UI change, open each tab in a browser. Check: light and dark, a
narrow window (390 px), empty state (no GPUs, no runs), error state (stop the
API), keyboard only. Say in your report which tabs you opened and what you
saw. Passing tests alone is not proof for UI work.
