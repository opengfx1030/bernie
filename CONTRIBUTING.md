# Contributing to rdnactl

Thank you for helping. This page tells you how to build, test and send a
change. AI agents also follow [AGENTS.md](AGENTS.md).

## Ways to help

| Contribution | Where |
|---|---|
| Report a host that does not work | Open an issue. Attach `rdnactl doctor --json` and `rdnactl hw --json`. |
| Add or fix a recipe | `recipes/<id>.yaml`. See [docs/recipes.md](docs/recipes.md). |
| Add a doctor check | `internal/doctor/`. Add a fixture and a test. |
| Add TunableOp rows | `tunableop/<profile>/` + `profiles.json`. Include `provenance.json`. |
| Support a distro | Package names and paths in `internal/machine/host`, `internal/setup`, `internal/doctor`. |
| Improve the dashboard | `dashboard/web/` (plain HTML/CSS/JS, no build step). |

## Build and test

```bash
git clone https://github.com/opengfx1030/rdnactl
cd rdnactl
make check          # gofmt + go vet + go test
make build          # bin/rdnactl
make linux          # bin/rdnactl-linux-amd64 for a GPU host
```

You need Go 1.23 or later. You do not need a GPU to run the tests.

## Recipe contributions

1. Start from the nearest built-in recipe. Use `extends` for shared parts.
2. Set `requires` (arch, GPU count, VRAM, RAM) so `recipe list` shows the
   right fit.
3. Run `rdnactl recipe validate recipes/<id>.yaml`.
4. Run `rdnactl recipe plan <id>` on a real host and check the command.
5. In the pull request, give the hardware, ROCm version, image or venv, and
   a `rdnactl bench --json` result. Set `status: validated` only with this
   evidence. Else use `status: experimental`.

## Pull requests

- Keep a pull request to one topic.
- Update `docs/spec.md` when behavior changes.
- Write what you tested and on which hardware.
- CI must pass: `gofmt`, `go vet`, `go test`, linux build.

## Fixtures from your host

Tests read sysfs snapshots. To add your hardware:

```bash
scripts/capture-fixture.sh my-host-name      # writes my-host-name.tar.gz
```

The script copies only the sysfs and procfs files rdnactl reads. It does not
copy serial numbers or user data. Check the archive before you share it.

## Code of conduct

Be kind and specific. Critique code, not people.
