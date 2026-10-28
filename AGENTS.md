# AGENTS.md

Rules for AI coding agents (and humans) who change this repository. Read
[docs/spec.md](docs/spec.md) and [docs/architecture.md](docs/architecture.md)
before you write code.

Working on the dashboard (`dashboard/`)? Also read
[dashboard/README.md](dashboard/README.md) (UI and UX rules) and
[dashboard/AGENTS.md](dashboard/AGENTS.md). Planned work and rejected ideas
are in [docs/roadmap.md](docs/roadmap.md).

## 1. What this repo is

bernie is a Go CLI with an embedded dashboard. It checks, sets up, tunes,
serves and benchmarks vLLM on AMD ROCm GPUs. The spec is the
contract. The code follows the spec.

The product surface is one binary. `bernie` with no arguments shows the
host status and the next command. `bernie up` runs the whole path. Every
other command is a step of that path. Keep it that way: a new feature is a
step `up` can call, not a new tool, script or required file.

## 2. Commands you need

```bash
make build        # bin/bernie for this machine
make linux        # bin/bernie-linux-amd64 (static, CGO_ENABLED=0)
make test         # go test ./...
make lint         # gofmt check + go vet
make check        # lint + test (run before every commit)
```

Requirements: Go 1.26 or later. No other toolchain. No CGO.

## 3. Hard rules

1. **Spec first.** A change to a command, a flag, a JSON field, the recipe
   schema or a file path changes `docs/spec.md` in the same commit.
2. **Layers.** Follow the import graph in `docs/architecture.md` §1. Domain
   packages return structs and never print. `internal/cli` and `dashboard`
   only parse input and format output.
3. **One file per command.** Add a command as `internal/cli/cmds/<family>/cmd_<name>.go`
   with `cli.Register` in `init()`. Do not edit `cli.go` for a new command (a new family package is blank-imported in `cmd/bernie/main.go`).
4. **`--json` is an API.** Every command supports it. Never put progress or
   logs on stdout. Keep JSON field names stable (snake_case).
5. **Read-only by default.** Inspection commands never change the host.
   Never call `sudo`. Print root steps as commands.
6. **No `/tmp`.** Use `internal/support/paths`. Hosts reboot; `/tmp` is wiped.
7. **Data over code.** Model, flag and env knowledge goes in `recipes/`.
   Do not add model-specific branches to Go code.
8. **Never break other platforms.** A recipe declares `requires.arch`. Keep
   CDNA (AITER) and NVIDIA concerns out of RDNA defaults.
9. **Dependencies.** The only third-party module is `gopkg.in/yaml.v3`. Ask a
   maintainer before you add one.
10. **No external calls in probes without a timeout.** Use `internal/support/execx`.
11. **Structure.** Follow spec §2.1: files ≤ 400 lines, directories ≤ 12
    entries, data keyed by GPU architecture (`<kind>/<arch>/...`), embedded
    trees discovered by walking, no code that names a device. Split by a real
    axis (device generation, model family, concern) before a folder gets flat
    and long.
12. **No competitor names.** Code, docs, comments, recipes, test data and
    commit messages do not name or link other local-LLM engines or tools.
    Describe the behavior or the idea instead. Research notes stay outside
    the repo.

## 4. Tests

- Read host state through a root path. Test against fixture trees in
  `internal/<pkg>/testdata/`. Capture new fixtures with
  `scripts/capture-fixture.sh` on the real host.
- Test behavior: merge rules, parsing, classification, rendering, selection,
  error paths. Do not write tests that only check wiring or echo a mock.
- Every built-in recipe must load, resolve, validate and plan
  (`go test ./recipes/...`).

## 5. Real hardware

The reference host is 8× V620 + 2× W7800 behind two PCIe switches. If you
have access to a GPU host:

- Build with `make linux` and copy the binary to the host.
- You may run `hw`, `topo`, `doctor`, `recipe plan`, `compose` freely.
- Do not start vLLM, change power caps or write sysfs on a shared host
  without the owner's approval.

## 6. Commits and pull requests

- One logical change per commit. Message: `area: imperative summary`
  (for example `recipe: support null to delete inherited args`).
- Run `make check` before you push.
- Describe what you verified and how (command + observed result).
- Commit as the human you work for. Do not add agent identities or
  `Co-authored-by` trailers for tools.

## 7. Writing style for docs

Use short sentences. One fact or one instruction per sentence. Active voice.
Use the terms defined in `docs/spec.md`. Put diagrams in Mermaid. Keep docs
next to the behavior they describe.
