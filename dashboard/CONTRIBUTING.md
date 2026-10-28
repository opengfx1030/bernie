# Contributing to the dashboard

Thank you for helping. General rules are in
[../CONTRIBUTING.md](../CONTRIBUTING.md). This page adds what is specific to
the dashboard. Read [README.md](README.md) for what the UI must do.

## Set up

You need Go 1.23 or later. You do not need Node. You do not need a GPU: the
page shows empty states without one.

```bash
git clone https://github.com/opengfx1030/rdnactl && cd rdnactl
make build
./bin/rdnactl dash          # open http://127.0.0.1:7860
```

The files in `web/` are embedded at build time. After you edit them, run
`make build` again and reload the page.

## Make a change

1. Open an issue or comment on one. Say which tab and which question it
   serves.
2. If the change needs data that the CLI does not give, change the CLI and
   its spec section first (see [../AGENTS.md](../AGENTS.md), "Spec first").
3. Make the smallest change. Use the existing helpers (`h()`, `badge()`,
   `bytes()`, `num()`, `toast()`) and the CSS variables.
4. Run `make check`.
5. Open the page and test the change by hand (list below).
6. Open a pull request. Add a screenshot of each changed tab, light and dark.

## Add a tab

1. Add the question it answers to the table in [README.md](README.md).
2. Add a link in `web/index.html` and a `<section id="tab-<name>">`.
3. Add `view<Name>(root)` in `web/app.js` and register it in `route()`.
4. Add the endpoint it needs (spec §11, `api.go`, a test).

## Add a card or a column

Use `h()`. Show the value with its unit and its limit. Show `–` when it is
missing. If the value can be a problem, show the fix next to it.

## Hand test list

Check each item for every tab you changed. Write the result in the pull
request.

- [ ] Light and dark.
- [ ] 390 px wide and 1400 px wide.
- [ ] No GPU, no runs (empty state teaches).
- [ ] API down (error text says what to do).
- [ ] Keyboard only: you can reach and use every control.
- [ ] A long value (model name, path) does not break the layout.

## Review checklist

A reviewer asks:

- Does the tab still answer one question?
- Does each value exist in the CLI `--json` with the same name?
- Does a problem show its fix and a copy button?
- Is there any `innerHTML`, dependency or network call to another host?
- Is every write guarded, and is every response redacted?
- Is the spec §11 updated?

## UX changes

The UI and UX rules in [README.md](README.md) are open to change. If you want
to change a rule, open an issue with the problem you saw and a screenshot.
We update the rule and the UI in the same pull request.
