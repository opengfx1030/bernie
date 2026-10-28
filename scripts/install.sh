#!/bin/sh
# Install the latest rdnactl release binary and verify its checksum.
#
#   curl -fsSL https://raw.githubusercontent.com/opengfx1030/rdnactl/main/scripts/install.sh | sh
#
# Environment:
#   RDNACTL_REPO     GitHub owner/repo        (default opengfx1030/rdnactl)
#   RDNACTL_VERSION  release tag, e.g. v0.1.0 (default: latest)
#   RDNACTL_BIN_DIR  install directory        (default ~/.local/bin)
# Never needs root. Never writes outside RDNACTL_BIN_DIR.
set -eu

repo=${RDNACTL_REPO:-opengfx1030/rdnactl}
version=${RDNACTL_VERSION:-latest}
bin_dir=${RDNACTL_BIN_DIR:-$HOME/.local/bin}

case $(uname -s) in
    Linux) ;;
    *) echo "rdnactl: only Linux is supported (got $(uname -s))" >&2; exit 1 ;;
esac
case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) echo "rdnactl: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

if [ "$version" = latest ]; then
    base="https://github.com/$repo/releases/latest/download"
else
    base="https://github.com/$repo/releases/download/$version"
fi
asset="rdnactl-linux-$arch"

dl() {
    if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
    else echo "rdnactl: need curl or wget" >&2; exit 1
    fi
}

work=$(mktemp -d "${TMPDIR:-/tmp}/rdnactl-install.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

echo "rdnactl: downloading $asset ($version) from $repo"
dl "$base/$asset" "$work/$asset"
dl "$base/SHA256SUMS" "$work/SHA256SUMS"

(cd "$work" && grep " $asset\$" SHA256SUMS | sha256sum -c -) >/dev/null || {
    echo "rdnactl: checksum mismatch, aborting" >&2
    exit 1
}

mkdir -p "$bin_dir"
install -m 0755 "$work/$asset" "$bin_dir/rdnactl"
echo "rdnactl: installed $("$bin_dir/rdnactl" version)"
case :$PATH: in
    *:"$bin_dir":*) ;;
    *) echo "rdnactl: add $bin_dir to PATH (export PATH=\"$bin_dir:\$PATH\")" ;;
esac
echo "rdnactl: next step: rdnactl doctor"
