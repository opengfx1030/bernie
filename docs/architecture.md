# bernie architecture

This document shows how the code is organized and how data moves through it.
The behavior contract is [spec.md](spec.md). If the two disagree, spec.md is
correct and this file is a bug.

Related pages:

- [packages.md](packages.md): the API of each package.
- [extending.md](extending.md): where to change what, and how to add a GPU
  generation.
- [recipes.md](recipes.md): how to write a recipe.

## 1. Layers

Packages sit in layers. An arrow means "imports". Nothing imports upward.
A solid arrow is a main dependency. A dotted arrow means "also imports
several packages of that group directly".

```mermaid
flowchart TB
  subgraph front["Front ends"]
    CLI["internal/cli<br/>+ cli/cmds/family"]
    DASH["dashboard<br/>HTTP, SSE, web/"]
    MCP["internal/mcp<br/>MCP tools"]
  end
  APP["internal/app<br/>shared operations"]
  FLOW["internal/flow<br/>status, up"]
  subgraph top["Domain, top level"]
    DOC["doctor"]
    SET["setup"]
    LOG["logdoc"]
  end
  subgraph serving["internal/serving"]
    RUN["runner"]
    REC["recipe"]
    TUN["tuning"]
    BEN["bench"]
    VLLM["vllm client"]
  end
  subgraph machine["internal/machine"]
    TOPO["topo"]
    GPU["gpu"]
    HOST["host"]
    ROCM["rocm"]
  end
  SUP["internal/support<br/>paths, execx, config, redact"]
  D1[("gpu/devices/*.yaml")]
  D2[("recipes/")]
  D3[("tunableop/<br/>tuned-moe/")]
  D4[("logdoc/*.yaml")]
  D5[("dashboard/web/")]
  CLI --> APP
  DASH --> APP
  MCP --> APP
  MCP --> FLOW
  DASH --> MCP
  APP -.-> top
  APP -.-> serving
  APP -.-> machine
  CLI --> FLOW
  CLI --> DASH
  CLI --> DOC
  DASH --> FLOW
  CLI --> LOG
  DASH --> DOC
  FLOW --> DOC
  FLOW --> SET
  FLOW --> RUN
  FLOW --> LOG
  RUN --> REC
  RUN --> TUN
  REC --> TOPO
  REC --> HOST
  DOC --> TUN
  TOPO --> GPU
  TOPO --> ROCM
  BEN --> VLLM
  CLI -.-> serving
  DASH -.-> serving
  MCP -.-> serving
  CLI -.-> machine
  DASH -.-> machine
  DOC -.-> machine
  SET -.-> machine
  serving -.-> machine
  machine --> SUP
  serving --> SUP
  GPU --> D1
  REC --> D2
  TUN --> D3
  LOG --> D4
  DASH --> D5
```

Two facts matter:

- `gpu` imports no other bernie package. It is the root of the graph.
- `dashboard` never imports `cli`. `cli` imports `dashboard` in three places:
  `dash`, `up` and `serve --dash`.
- `mcp` never imports `cli` or `dashboard`. `dashboard` imports `mcp` to
  mount `/mcp`; `cli` imports `mcp` for `bernie mcp`.

Rules:

1. Front ends (CLI, dashboard, MCP) parse input and format output. They hold
   no domain logic.
2. Domain packages return plain structs with JSON tags. They never print.
   They take a `context.Context` for anything that can block.
3. Domain packages read the host through an injectable root (a root path or
   an `fs.FS`), so tests run on fixture trees.
4. Arrows are the only allowed imports. `gpu` never imports `recipe`.
   `dashboard` never imports `cli`. `doctor` never imports `recipe`: the
   caller passes the validated architectures in `doctor.Options`.
5. The JSON field names of domain structs are the public API (spec.md).
   Rename a field only together with spec.md.
6. `flow` holds the two product-level chains (`status` and `up`).
   `runner.Prepare` is the only place that turns a recipe id into a ready
   plan. The CLI and the dashboard both call it.
7. `app` holds the operations that more than one front end runs and that no
   domain package owns: `hw`, `topo`, `doctor`, the run-log scan, `recipe
   list`, `tune` status and power, `setup` plan and apply, `bench` (run and
   save). Each returns the struct of its `--json` document. `app` never
   prints and never imports a front end.

## 2. Repository layout

```text
bernie/
├── cmd/bernie/           main.go: blank-imports each cmds family, calls cli.Main
├── internal/
│   ├── cli/              command framework, --json and exit-code contract, version
│   │   └── cmds/         one family package per area, one cmd_<name>.go per command
│   │       ├── health/       doctor, setup, status, up
│   │       ├── inventory/    hw, topo
│   │       ├── serving/      recipe, serve, ps, stop, logs, compose, config
│   │       ├── tunecmd/      tune
│   │       ├── benchcmd/     bench
│   │       ├── dashcmd/      dash
│   │       └── mcpcmd/       mcp
│   ├── app/              operations shared by the front ends, one `--json` document each
│   ├── mcp/              MCP server: tools, confirm tokens, audit (official Go SDK)
│   ├── flow/             status snapshot and `up`
│   ├── doctor/           check registry (one checks_<category>.go per category)
│   ├── setup/            install plan and non-root apply
│   ├── logdoc/           known-failure catalog, phase catalogue (embedded YAML) and log scanner
│   ├── machine/          what the host is
│   │   ├── gpu/              KFD/DRM/PCI enumeration, telemetry, devices/*.yaml tables
│   │   ├── host/             OS, package manager, CPU, RAM, groups, container engines
│   │   ├── rocm/             ROCm installs, tools, libraries, venv probe
│   │   └── topo/             PCIe tree, link classes, P2P matrix, bandwidth
│   ├── serving/          what bernie runs
│   │   ├── recipe/           schema, load, extends/merge, validate, fit, plan
│   │   ├── runner/           host and container backends, run records, logs
│   │   ├── tuning/           TunableOp profiles and rows, tuned-MoE folders, power cap
│   │   ├── vllm/             client: /health, /v1/models, /tokenize, /metrics
│   │   └── bench/            streaming benchmark and result store
│   └── support/          small shared helpers
│       ├── paths/            XDG config, state, data, cache
│       ├── execx/            bounded exec of external tools
│       ├── config/           <config>/config.yaml
│       ├── redact/           secret redaction for documents and log text
│       └── fixture/          test host snapshots (testdata/*.tar.gz)
├── dashboard/            HTTP API, SSE, embedded web/ (no build step)
├── recipes/              built-in recipes by family, fragments by concern, embed.go
├── tunableop/            <arch>/<profile>/ rows and profile.json, embed.go
├── tuned-moe/            <arch>/ fused-MoE configs, embed.go
├── deploy/               packaging (nfpm), docker, compose
├── scripts/              install.sh, capture-fixture.sh
└── docs/                 spec, architecture, packages, extending, recipes, install, overview.html
```

## 3. Flows

### 3.1 What `up` does

`up` is a thin front end over `flow.Up`. The sequence below shows who calls
whom. A step that cannot go on returns `Blocked`. `cli` prints it and exits
with code 3.

```mermaid
sequenceDiagram
  actor U as User
  participant C as cli (health/cmd_up)
  participant F as flow
  participant D as doctor
  participant R as recipe
  participant S as setup
  participant X as runner
  participant V as vLLM
  participant W as dashboard
  U->>C: bernie up [RECIPE]
  C->>F: Up(options)
  F->>D: Run(categories gpu, kernel)
  D-->>F: report (fail = Blocked + fix commands)
  F->>R: Load, List (fit), Best or Resolve
  R-->>F: recipe id
  F->>S: Plan(mode, GPU archs)
  S-->>F: steps (root step pending = Blocked + commands)
  F->>C: Confirm (once, skipped by --yes)
  C->>U: Run setup steps and start RECIPE?
  U-->>C: yes
  F->>S: Apply(non-root steps)
  F->>X: Prepare(recipe, mode, sets)
  X-->>F: plan (see 3.2)
  F->>X: Start(plan)
  X->>V: spawn process, or engine run
  F->>W: Start(listen) (the dashboard serves while the server loads)
  loop every 3 s until --wait
    F->>X: List
    X->>V: GET /health
  end
  Note over F,X: a run that exits: F scans its log (logdoc) and returns StartupExit with the matches
  F-->>C: UpResult (url, run record)
  C->>W: Wait (Ctrl-C stops the dashboard, the run goes on)
  W->>X: List, Logs (on request)
```

### 3.2 From recipe to command

`runner.Prepare` runs this chain. `recipe plan`, `serve`, `compose` and the
dashboard use the same chain. First, the recipe becomes a plan:

```mermaid
flowchart LR
  L["Load<br/>layers"] --> R["Resolve<br/>merge extends"]
  L --> F["CheckFit<br/>used by list, up"]
  R --> O["Apply<br/>config set, --set<br/>then Validate"]
  O --> P["BuildPlan<br/>inputs, env, args,<br/>GPU selection:<br/>arch, count, PCIe switch,<br/>host: tuned-MoE folder"]
  W["gpu.List<br/>topo.Discover"] --> P
```

Then the plan gets the host and becomes a command:

```mermaid
flowchart LR
  S["Plan (from BuildPlan)"] --> H["Host env, or<br/>container mounts"]
  H --> T["TunableOp profile<br/>host: rocBLAS hash + arch<br/>container: recipe profile"]
  T --> M{"mode"}
  M -- host --> X1["Plan.Exec<br/>venv python -m vllm"]
  M -- container --> X2["Plan.Exec<br/>engine run image,<br/>or compose file"]
```

### 3.3 Life of a run

The state of a run is not stored. `runner.List` derives it from the process
(or container) and `/health`. Only `run.json` is stored, when `Start`
succeeds. A plan has no record until it starts.

```mermaid
stateDiagram-v2
  [*] --> planned: Prepare (recipe plan)
  planned --> starting: Start writes run.json
  starting --> running: /health answers 200
  running --> starting: process alive, /health silent
  starting --> exited: process or container gone
  running --> exited: bernie stop, or a crash
  exited --> starting: serve with the same name
  exited --> [*]
```

`exited` covers a normal `stop` and a failure. To tell them apart, run
`bernie doctor --run NAME`. It matches the log against the known-failure
catalog.

## 4. Data keyed by GPU architecture

Four kinds of data depend on the GPU. Each has one folder per key. A new
architecture adds files. It never edits the files of another architecture.

```mermaid
flowchart LR
  ARCH(["GPU arch<br/>gfx1030, gfx1100, ..."])
  ARCH --- DEV["internal/machine/gpu/devices/<br/>generation.yaml"]
  ARCH --- TOP["tunableop/arch/profile/<br/>profile.json + rows"]
  ARCH --- MOE["tuned-moe/arch/<br/>E=...json"]
  ARCH --- REC["recipes/<br/>requires.arch + fragments/arch/"]
  DEV --> R1["gpu.Lookup, LookupArch, CodeTargets<br/>used by gpu, doctor, setup"]
  TOP --> R2["tuning.Profiles, Select<br/>used by runner, doctor"]
  MOE --> R3["tuning.MaterializeTunedMoE<br/>used by recipe plan"]
  REC --> R4["recipe.Load, CheckFit<br/>used by up, list, plan"]
```

Rules:

- Every reader finds the data by walking the embedded tree. No Go file lists
  a device, a profile or a recipe.
- A profile says which architectures it fits in its own `profile.json`.
- `doctor` learns the validated architectures from the recipes
  (`recipe.ValidatedArchs`), not from a list.

[extending.md](extending.md) lists the steps to add a generation.

## 5. Testing

- Unit tests read a root path. Fixture trees live in
  `internal/support/fixture/testdata/<name>.tar.gz`. `fixture.Root(t, name)`
  unpacks one. Capture a real host with `scripts/capture-fixture.sh`.
- A fixture that was not captured is synthetic. Its name ends in
  `-synthetic` and it has a `SYNTHETIC` file at its root. The README next to
  the fixtures says which values are real and which are written by hand.
  Never present a synthetic fixture as proof of hardware support.
- `gpu` tests check that every device table loads, ids are unique and each
  override points to an arch of the same file.
- `tuning` tests check the layout of `tunableop/` and `tuned-moe/` (top
  folders are gfx names, at most 12 entries per folder) and that every
  `profile.json` name equals its folder name.
- `recipe` tests check merge rules, interpolation, rendering and GPU
  selection. Every built-in recipe must load, resolve, validate and plan
  (`go test ./recipes/...`).
- `runner` tests check the rendered argv and compose output. They do not run
  processes.
- `bench` tests use an in-process fake OpenAI server.
- Before a release, smoke on real hardware: `doctor`, `topo`, `recipe plan`
  for each built-in recipe, one `serve` and one `bench` cell.
