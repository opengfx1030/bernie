# AGENTS.md (dashboard)

Rules for AI agents that change `dashboard/`. The repo rules in
[../AGENTS.md](../AGENTS.md) also apply. Read [README.md](README.md) first:
it holds the UI and UX rules and the layout of the folder.

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
4. **No `innerHTML`.** Build DOM with `h()` in `web/js/core.js`. Server data is
   text.
5. **Writes are guarded.** Every POST and DELETE keeps the `X-Bernie` header
   check and the token rules in `internal/auth`. Never loosen them.
6. **Redact.** Anything sent to the browser goes through `internal/support/redact`.
7. **One question per tab.** Do not add a sixth tab for a feature that fits
   an existing one. Do not put two questions in one tab. The Classic tab is
   the one exception: it is the reference-parity port, and it is there to be
   compared with the Dashboard tab.
8. **The visual design follows [../docs/design-dashboard.md](../docs/design-dashboard.md).**
   That page is the design contract. The colours live in `web/css/tokens.css`
   and nowhere else: a component file never writes a colour by hand.
   Exception: the Classic tab (`web/css/classic.css`) is a reference-parity
   port. It reads the same tokens (they carry the palette of the reference)
   and writes no colour of its own, and every rule of it is scoped under
   `.classic`, so it touches no other tab.
9. **Same names as the CLI.** A field on the page has the JSON name of the
   CLI output.

## Structure rules

These rules replace the old "a directory has at most N entries" cap. A count
is not a reason; a role is.

1. **One responsibility per package.** A package says in one sentence what it
   owns, at the top of its `doc.go` or its main file.
2. **A small public surface.** The exported names of a package are listed in
   [../docs/packages.md](../docs/packages.md). Keep the list short.
3. **Group by role, not by count.** A directory holds the files of one role.
   When a directory grows past about 15 files, or holds two roles that can be
   told apart, split it into sub-packages or sub-directories by that role.
4. **Readable files.** A file stays under about 400 lines. Split it by
   concept (one domain, one card), never by cutting a function in half.
5. **Tests live next to the code.** A test file sits in the package it
   tests. Fixtures live in `testdata/`.
6. **No cycles, no upward imports.** The layers are in
   [../docs/architecture.md](../docs/architecture.md): the dashboard may use
   `internal/app`, `internal/flow` and the domain packages, never the other
   way round.
7. **The web tree is grouped by role.** `web/css/` holds the stylesheets,
   `web/js/` the core scripts (DOM, shell, data, router), `web/components/`
   the shared widgets and `web/views/<tab>/` the per-tab code. One file per
   concern.

## Where to change what

| Task | Files |
|---|---|
| New endpoint | spec §11, `internal/api/<domain>.go`, `internal/api/api.go` (the route), a test |
| New view or card | `web/views/<tab>/<file>.js` (one `view*` function), `web/components/` |
| New tab | `web/index.html` tab link and section, `web/views/<tab>/`, the `views` map in `web/js/boot.js`, this README table |
| New live value | `internal/telemetry/sampler.go`, the sample type in `gpu`/`runner`, spec §11 |
| Change colours or layout | `web/css/tokens.css` for colours, `web/css/layout.css` for the shell |
| Auth change | `internal/auth/auth.go`, spec §11, a test that shows the old attack still fails |

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
