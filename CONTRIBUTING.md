# Contributing to bernie

Thank you for helping. This page tells you how to build, test and send a
change. AI agents also follow [AGENTS.md](AGENTS.md).

## Ways to help

[docs/extending.md](docs/extending.md) lists the exact files and the test to
run for each change below.

| Contribution | Where |
|---|---|
| Report a host that does not work | Open an issue. Attach `bernie doctor --json` and `bernie hw --json`. |
| Add or fix a recipe | `recipes/<family>/<id>.yaml`. See [docs/recipes.md](docs/recipes.md). |
| Add a doctor check | `internal/doctor/checks_<category>.go`. Add a case to `doctor_test.go`. |
| Add TunableOp rows | `tunableop/<arch>/<profile>/` with `profile.json`. Include `provenance.json`. |
| Add tuned MoE configs | `tuned-moe/<arch>/`. See `tuned-moe/README.md`. |
| Support a GPU or a GPU generation | `internal/machine/gpu/devices/<generation>.yaml`. See [docs/extending.md](docs/extending.md#2-how-to-add-a-new-gpu-generation). |
| Support a distro | Package names and paths in `internal/machine/host`, `internal/setup`, `internal/doctor`. |
| Improve the dashboard | `dashboard/web/` (plain HTML/CSS/JS, no build step). |

## Build and test

```bash
git clone https://github.com/opengfx1030/bernie
cd bernie
make check          # gofmt + go vet + go test
make build          # bin/bernie
make linux          # bin/bernie-linux-amd64 for a GPU host
```

You need Go 1.26 or later. You do not need a GPU to run the tests.

## Recipe contributions

1. Start from the nearest built-in recipe. Use `extends` for shared parts.
2. Set `requires` (arch, GPU count, VRAM, RAM) so `recipe list` shows the
   right fit.
3. Run `bernie recipe validate recipes/<family>/<id>.yaml`.
4. Run `bernie recipe plan <id>` on a real host and check the command.
5. In the pull request, give the hardware, ROCm version, image or venv, and
   a `bernie bench --json` result. Set `status: validated` only with this
   evidence. Else use `status: experimental`.

## Pull requests

- Keep a pull request to one topic.
- Update `docs/spec.md` when behavior changes.
- Write what you tested and on which hardware.
- CI must pass: `gofmt`, `go vet`, `go test`, linux build.

## Validate a change with the bench suite

A change that touches the serving path (a recipe, a vLLM build, a tuning
row, a kernel) is only measured on real hardware. bernie has one command
for it: the `pr` suite.

```bash
bernie bench --suite pr --url http://127.0.0.1:8000
```

The suite runs the fixed cells of a pull request: 1024 prompt tokens and
512 output tokens at the levels 1, 4 and 8, and 16384 prompt tokens and
1024 output tokens at the same levels. Every cell runs several times, and
the cell keeps the medians, the repetitions and the spread of its
metrics. Every prompt hides a short random fact: a cell that does not
repeat it has broken attention, and its speed numbers mean nothing.

The server must run with prefix caching off. Otherwise a long prompt comes
from the cache and the cell measures nothing.

To compare a baseline with a candidate, run the suite twice and compare
the two results:

```bash
bernie bench --suite pr --url http://127.0.0.1:8000 --tag baseline
bernie bench --suite pr --url http://127.0.0.1:8000 --tag candidate
bernie bench compare <baseline-id> <candidate-id> --markdown
```

The `--markdown` output is the receipt of the comparison: the scope of the
two runs, their correctness, the numbers of every cell with the verdict of
every metric, and the command that reproduces it. Paste it into the
pull request.

When the change is a new vLLM build or a new recipe, let bernie run both
arms back to back:

```bash
bernie bench ab --base <baseline-recipe> --cand <candidate-recipe> --suite pr
```

The visits run in the order base, cand, cand, base, so a drift of the
machine hits both arms the same way. One server runs at a time, and every
visit saves its own result. The verdict uses the same rules as
`bench compare`.

A pull request without a receipt is fine when the change does not touch
the serving path. Say so in the description.

## Fixtures from your host

Tests read sysfs snapshots. To add your hardware:

```bash
scripts/capture-fixture.sh my-host-name      # writes my-host-name.tar.gz
```

The script copies only the sysfs and procfs files bernie reads. It does not
copy serial numbers or user data. Check the archive before you share it.
Put the archive in `internal/support/fixture/testdata/` and add a row to the
README there.

A fixture that you did not capture is synthetic. Name it
`<name>-synthetic.tar.gz` and add a `SYNTHETIC` file at its root. The README
row says which values are real and which you wrote by hand.

## Code of conduct

Be kind and specific. Critique code, not people.
