# Install bernie

bernie is one static Linux binary (amd64 and arm64). Pick one method.
No method needs root, except the package manager and the container engine.
bernie never runs `sudo` by itself.

| Method | Use it when |
|---|---|
| [Installer script](#1-installer-script) | You want the newest binary in `~/.local/bin`. |
| [rpm or deb package](#2-rpm-or-deb-package) | You want the package manager to own the file. |
| [Container image](#3-container-image) | You want only the dashboard, `hw`, `topo`, `doctor`, `bench` in a container. |
| [Compose](#4-compose) | Same, with a file you can keep. |
| [Build from source](#5-build-from-source) | You develop bernie. |

After you install, run `bernie doctor`.

## 1. Installer script

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
```

The script:

1. downloads `bernie-linux-<arch>` and `SHA256SUMS` from the latest release;
2. checks the SHA-256 sum and stops if it does not match;
3. installs the file as `~/.local/bin/bernie`.

Settings (environment variables):

| Variable | Default | Meaning |
|---|---|---|
| `BERNIE_VERSION` | `latest` | Release tag, for example `v0.1.0`. |
| `BERNIE_BIN_DIR` | `~/.local/bin` | Install directory. |
| `BERNIE_REPO` | `opengfx1030/bernie` | GitHub `owner/repo`. |

## 2. rpm or deb package

Each release has these files (`<ver>` has no leading `v`):

| File | System |
|---|---|
| `bernie-<ver>-1.x86_64.rpm`, `bernie-<ver>-1.aarch64.rpm` | Fedora, RHEL, openSUSE |
| `bernie_<ver>_amd64.deb`, `bernie_<ver>_arm64.deb` | Debian, Ubuntu |
| `bernie_<ver>_linux_<arch>.tar.gz` | Any Linux: binary, `LICENSE`, `README.md` |
| `SHA256SUMS`, `SHA256SUMS.bundle` | Checksums and their signature |

The package installs `/usr/bin/bernie` and the documents in
`/usr/share/doc/bernie/`. It has no install scripts. It does not change
groups, users or GPU settings.

### Check the download

Check the checksums first:

```bash
sha256sum --ignore-missing -c SHA256SUMS
```

Then check the signature of `SHA256SUMS`. The release workflow signs it with
[cosign](https://docs.sigstore.dev/cosign/) (keyless). A good result proves
that GitHub Actions built the release from this repository:

```bash
cosign verify-blob \
  --bundle SHA256SUMS.bundle \
  --certificate-identity-regexp '^https://github.com/opengfx1030/bernie/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

### Install

```bash
sudo dnf install ./bernie-<ver>-1.x86_64.rpm      # Fedora, RHEL
sudo apt install ./bernie_<ver>_amd64.deb         # Debian, Ubuntu
```

## 3. Container image

```bash
docker pull ghcr.io/opengfx1030/bernie:v0.1.0      # or podman pull
```

The image has multi-arch tags: `vX.Y.Z` and `latest`. It holds only the
`bernie` binary on `scratch` (no shell, no libc, no CA bundle) and runs as user 65532 (not root).
The default command is `dash --listen 0.0.0.0:7860`.

### What works in the image

| Works | Does not work |
|---|---|
| `dash`, `hw`, `topo`, `doctor`, `recipe`, `bench` | `serve` in host mode: the image has no Python venv. |
| Reading `/sys` and `/proc` | `serve` in container mode: the image has no `docker` or `podman` program. |

Container-mode `serve` needs an engine program and the engine socket in the
container. To get it, make your own image from this one and add the program.
For all vLLM runs, use the `bernie` binary on the host.

### Run

The dashboard listens on a non-loopback address inside the container.
Therefore it needs a token. Set `BERNIE_DASH_TOKEN`. If you do not, bernie
makes a random token and prints the URL in the log (`docker logs`).

```bash
mkdir -p data && sudo chown 65532:65532 data
docker run -d --name bernie-dash \
  -p 127.0.0.1:7860:7860 \
  -e BERNIE_DASH_TOKEN="$(openssl rand -hex 24)" \
  --device /dev/kfd --device /dev/dri \
  --group-add "$(stat -c %g /dev/kfd)" --group-add "$(stat -c %g /dev/dri/renderD128)" \
  --security-opt label=disable --security-opt seccomp=unconfined \
  -v "$PWD/data:/data" -v /sys:/sys:ro \
  ghcr.io/opengfx1030/bernie:v0.1.0
```

Notes:

- **GPU access.** `/dev/kfd` and `/dev/dri` belong to the groups `render` and
  `video`. The image has no such group names, so use the numeric group ids
  of your host.
- **Rootless Podman.** Use `--group-add keep-groups --user 0:0` instead. The
  container root is your own user on the host.
- **Data.** `/data` holds config, state, data and cache
  (`BERNIE_CONFIG_DIR`, `BERNIE_STATE_DIR`, `BERNIE_DATA_DIR`,
  `BERNIE_CACHE_DIR`).
- **Port.** Publish on `127.0.0.1`. Publish on `0.0.0.0` only on a network you
  trust. The token is then the only protection.
- **Engine socket.** Do not mount it unless you must. Access to the socket
  is the same as root on the host. See [SECURITY.md](../SECURITY.md).
- **Health check.** The image has no shell, so it has no `HEALTHCHECK`. Test
  from outside: `curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7860/api/hw`.

## 4. Compose

`deploy/compose/compose.yml` is a reference file for the same container.
It works with `docker compose`, `podman compose` and `podman-compose`.

```bash
cd deploy/compose
cp .env.example .env          # set BERNIE_DASH_TOKEN, VIDEO_GID, RENDER_GID
mkdir -p data && sudo chown 65532:65532 data
docker compose up -d          # open http://127.0.0.1:7860
```

For rootless Podman, add the override file:

```bash
podman compose -f compose.yml -f compose.podman.yml up -d
```

The override needs `!override` support (docker compose 2.24 or later,
podman-compose 1.3 or later).

## 5. Build from source

You need Go 1.26 or later.

```bash
git clone https://github.com/opengfx1030/bernie && cd bernie
make build            # bin/bernie
make dist             # dist/: static binaries, tar.gz archives, SHA256SUMS
make packages         # dist/: rpm and deb (runs nfpm through `go run`)
make image            # container image (docker or podman)
```

`make dist` builds both `linux/amd64` and `linux/arm64`. Set `VERSION=v1.2.3`
to override the version taken from `git describe`.
