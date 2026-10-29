#!/bin/sh
# Install the latest bernie release binary and verify its checksum.
#
#   curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
#
# Flags:
#   --version TAG    release tag, e.g. v0.1.0 (default: latest)
#   --dir DIR        install directory (default: ~/.local/bin;
#                    if DIR is not writable, prints the exact command
#                    for you to run with sudo and exits non-zero)
#   --dry-run        print what would be done, do nothing
#   -h | --help      this help
#
# Environment (flags take precedence):
#   BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
#   BERNIE_VERSION   release tag              (default: latest)
#   BERNIE_BIN_DIR   install directory        (default: ~/.local/bin)
#   BERNIE_BASE_URL  base URL (testing only; default https://github.com/$repo)
#
# The script never calls `sudo` itself. If the chosen directory is not
# writable, it prints the exact command and exits non-zero.
#
# After install, run:  bernie up

set -eu

usage() {
    cat <<'EOF'
Usage: install.sh [--version TAG] [--dir DIR] [--dry-run] [-h|--help]

Install the latest bernie release binary.

  --version TAG    release tag, e.g. v0.1.0 (default: latest)
  --dir DIR        install directory (default: ~/.local/bin)
  --dry-run        print what would be done; do nothing

Environment:
  BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
  BERNIE_VERSION   release tag              (overrides --version)
  BERNIE_BIN_DIR   install directory        (overrides --dir)
  BERNIE_BASE_URL  base URL                 (testing only)

After install, run:  bernie up
EOF
}

# Defaults. The flags override the environment.
repo=${BERNIE_REPO:-opengfx1030/bernie}
version=${BERNIE_VERSION:-latest}
bin_dir=${BERNIE_BIN_DIR:-$HOME/.local/bin}
base_url=${BERNIE_BASE_URL:-https://github.com/$repo}
dry_run=0

while [ $# -gt 0 ]; do
    case "$1" in
        --version) version=$2; shift 2 ;;
        --dir)     bin_dir=$2; shift 2 ;;
        --dry-run) dry_run=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) printf 'bernie: unknown flag: %s\n\n' "$1" >&2
           usage >&2
           exit 2 ;;
    esac
done

# bernie ships as a Linux static binary; macOS and Windows are out of scope.
case $(uname -s) in
    Linux) ;;
    *) printf 'bernie: only Linux is supported (got %s)\n' "$(uname -s)" >&2
       exit 1 ;;
esac
case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) printf 'bernie: unsupported CPU %s\n' "$(uname -m)" >&2
       exit 1 ;;
esac

if [ "$version" = latest ]; then
    download_base="$base_url/releases/latest/download"
else
    download_base="$base_url/releases/download/$version"
fi
asset="bernie-linux-$arch"

# curl or wget; either works for the install line.
dl() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$2" "$1"
    else
        printf 'bernie: need curl or wget\n' >&2
        exit 1
    fi
}

# sudo hint, printed only when the chosen dir is not writable.
sudo_hint() {
    printf 'bernie: rerun with sudo:\n' >&2
    printf '    curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sudo sh -s -- --dir %s\n' "$bin_dir" >&2
}

# Announce the plan. Always printed so the operator sees what will happen.
printf 'bernie: target    %s @ %s (%s)\n' "$repo" "$version" "$arch"
printf 'bernie: dest      %s/bernie\n' "$bin_dir"
printf 'bernie: download  %s/%s\n' "$download_base" "$asset"

# Dry-run: stop here. The operator has the plan; nothing was touched.
if [ "$dry_run" = 1 ]; then
    printf 'bernie: dry-run: stopping before download\n'
    exit 0
fi

# Writability check, BEFORE the download. The script never calls sudo; it
# just reports. Three cases:
#   a) the path exists but is not a directory -> fail
#   b) the path does not exist and we cannot create it -> fail with hint
#   c) the path exists but is not writable -> fail with hint
if [ -e "$bin_dir" ] && [ ! -d "$bin_dir" ]; then
    printf 'bernie: %s exists and is not a directory\n' "$bin_dir" >&2
    exit 1
fi
if [ ! -d "$bin_dir" ]; then
    if mkdir -p "$bin_dir" 2>/dev/null; then :; else
        printf 'bernie: cannot create %s (not writable as %s)\n' "$bin_dir" "$(id -un)" >&2
        sudo_hint
        exit 1
    fi
fi
if [ ! -w "$bin_dir" ]; then
    printf 'bernie: %s is not writable as %s\n' "$bin_dir" "$(id -un)" >&2
    sudo_hint
    exit 1
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/bernie-install.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

dl "$download_base/$asset" "$work/$asset"
dl "$download_base/SHA256SUMS" "$work/SHA256SUMS"

# Verify the checksum. The expected line is "<sha>  <filename>".
if ! grep -q " $asset\$" "$work/SHA256SUMS"; then
    printf 'bernie: %s not listed in SHA256SUMS\n' "$asset" >&2
    exit 1
fi
(cd "$work" && grep " $asset\$" SHA256SUMS | sha256sum -c -) >/dev/null || {
    printf 'bernie: checksum mismatch, aborting\n' >&2
    exit 1
}

# Install. Use cp+chmod (POSIX) instead of GNU install(1) for portability.
cp "$work/$asset" "$bin_dir/bernie"
chmod 0755 "$bin_dir/bernie"

# Confirm the binary runs. `bernie version` prints one line.
if ! "$bin_dir/bernie" version >/dev/null 2>&1; then
    printf 'bernie: installed binary does not run on this host\n' >&2
    exit 1
fi

printf 'bernie: installed %s\n' "$("$bin_dir/bernie" version)"

case :$PATH: in
    *:"$bin_dir":*) ;;
    *) printf 'bernie: add %s to your PATH\n' "$bin_dir" ;;
esac
printf 'bernie: next step: bernie up\n'
