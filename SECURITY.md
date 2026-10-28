# Security policy

## Report a vulnerability

Do not open a public issue. Use GitHub private vulnerability reporting
("Security" tab → "Report a vulnerability"). Give the version
(`bernie version`), the command, and the impact.

## Threat model (short)

- `bernie dash` can start processes and containers. It listens on
  `127.0.0.1` by default. On any other address it requires a token, and
  state-changing requests require the `X-Bernie: 1` header.
- Treat the dashboard token like a password. Anyone with it can run vLLM
  with any recipe on the host.
- Container mode talks to the Docker or Podman socket. Access to that socket
  is equal to root on most hosts.
- bernie never runs `sudo`. Commands that need root are printed for the
  operator.
- Plans and logs redact environment keys that contain `TOKEN`, `KEY`,
  `SECRET` or `PASSWORD` before the dashboard shows them.

## Supply chain

Nothing in this repository floats.

- **GitHub Actions** are pinned to a full commit SHA with the tag in a comment.
- **Go modules** are pinned in `go.mod` and `go.sum`. CI runs `go mod verify` and
  `go mod tidy -diff`. Adding a module needs a maintainer's approval
  ([AGENTS.md](AGENTS.md) §3).
- **Tools** that CI runs (`govulncheck`) carry an explicit version, never `@latest`.
- **Container images** are pinned by digest in `deploy/docker/Dockerfile`. The
  final image is `scratch`: only the static binary, run as uid 65532.
- **Compose** requires `BERNIE_TAG`, a release tag. It has no default.
- **Releases** are built in CI. Checksums are signed with cosign.
- **Updates** come as Dependabot pull requests (actions, modules, base images).
  A maintainer reviews each one. Bump the pinned tool versions by hand.
