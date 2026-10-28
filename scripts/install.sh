#!/bin/sh
# Install the latest bernie release binary and verify its checksum.
#
#   curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
#
# Environment:
#   BERNIE_REPO     GitHub owner/repo        (default opengfx1030/bernie)
#   BERNIE_VERSION  release tag, e.g. v0.1.0 (default: latest)
#   BERNIE_BIN_DIR  install directory        (default ~/.local/bin)
# Never needs root. Never writes outside BERNIE_BIN_DIR.
set -eu

repo=${BERNIE_REPO:-opengfx1030/bernie}
version=${BERNIE_VERSION:-latest}
bin_dir=${BERNIE_BIN_DIR:-$HOME/.local/bin}

case $(uname -s) in
    Linux) ;;
    *) echo "bernie: only Linux is supported (got $(uname -s))" >&2; exit 1 ;;
esac
case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) echo "bernie: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

if [ "$version" = latest ]; then
    base="https://github.com/$repo/releases/latest/download"
else
    base="https://github.com/$repo/releases/download/$version"
fi
asset="bernie-linux-$arch"

dl() {
    if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
    else echo "bernie: need curl or wget" >&2; exit 1
    fi
}

work=$(mktemp -d "${TMPDIR:-/tmp}/bernie-install.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

echo "bernie: downloading $asset ($version) from $repo"
dl "$base/$asset" "$work/$asset"
dl "$base/SHA256SUMS" "$work/SHA256SUMS"

(cd "$work" && grep " $asset\$" SHA256SUMS | sha256sum -c -) >/dev/null || {
    echo "bernie: checksum mismatch, aborting" >&2
    exit 1
}

mkdir -p "$bin_dir"
install -m 0755 "$work/$asset" "$bin_dir/bernie"
echo "bernie: installed $("$bin_dir/bernie" version)"
case :$PATH: in
    *:"$bin_dir":*) ;;
    *) echo "bernie: add $bin_dir to PATH (export PATH=\"$bin_dir:\$PATH\")" ;;
esac
echo "bernie: next step: bernie doctor"
