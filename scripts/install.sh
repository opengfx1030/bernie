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
#   --start          start the dashboard service after install (one-line
#                    then browser)
#   --listen ADDR    listen address for the dashboard (default:
#                    127.0.0.1:7860; a non-loopback address is opt-in
#                    and requires a token; the URL carries ?token=...)
#   --no-lan         equivalent to --listen 127.0.0.1:7860 (the default)
#   --dry-run        print what would be done, do nothing
#   -h | --help      this help
#
# Environment (flags take precedence):
#   BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
#   BERNIE_VERSION   release tag              (default: latest)
#   BERNIE_BIN_DIR   install directory        (default: ~/.local/bin)
#   BERNIE_BASE_URL  base URL (testing only; default https://github.com/$repo)
#   BERNIE_LISTEN    listen address for --start (same as --listen)
#
# The script never calls `sudo` itself. If the chosen directory is not
# writable, it prints the exact command and exits non-zero.
#
# Re-running the line updates the binary. With --start, the dashboard is
# restarted only when the installed version changed.

set -eu

usage() {
    cat <<'EOF'
Usage: install.sh [--version TAG] [--dir DIR] [--start] [--listen ADDR] [--no-lan] [--dry-run] [-h|--help]

Install the latest bernie release binary.

  --version TAG    release tag, e.g. v0.1.0 (default: latest)
  --dir DIR        install directory (default: ~/.local/bin)
  --start          start the dashboard service after install
  --listen ADDR    listen address for the dashboard (default 127.0.0.1:7860)
  --no-lan         force loopback only (same as the default)
  --dry-run        print what would be done; do nothing

Environment:
  BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
  BERNIE_VERSION   release tag              (overrides --version)
  BERNIE_BIN_DIR   install directory        (overrides --dir)
  BERNIE_BASE_URL  base URL                 (testing only)
  BERNIE_LISTEN    listen address           (overrides --listen)

After install, run:  bernie up

With --start the dashboard is left running as a background service; the
script then prints the loopback URL, the LAN URLs and an ssh -L hint.
EOF
}

# Defaults. The flags override the environment.
repo=${BERNIE_REPO:-opengfx1030/bernie}
version=${BERNIE_VERSION:-latest}
bin_dir=${BERNIE_BIN_DIR:-$HOME/.local/bin}
base_url=${BERNIE_BASE_URL:-https://github.com/$repo}
listen=${BERNIE_LISTEN:-127.0.0.1:7860}
dry_run=0
start_after=0

while [ $# -gt 0 ]; do
    case "$1" in
        --version) version=$2; shift 2 ;;
        --dir)     bin_dir=$2; shift 2 ;;
        --start)   start_after=1; shift ;;
        --listen)  listen=$2; shift 2 ;;
        --no-lan)  listen=127.0.0.1:7860; shift ;;
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
if [ "$start_after" = 1 ]; then
    printf 'bernie: dash      start (listen %s)\n' "$listen"
fi

# Dry-run: stop here. The operator has the plan; nothing was touched.
if [ "$dry_run" = 1 ]; then
    printf 'bernie: dry-run: stopping before download\n'
    if [ "$start_after" = 1 ]; then
        printf 'bernie: dry-run: would start the dashboard on %s\n' "$listen"
    fi
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
    printf 'bernie: %s is not writable as %s)\n' "$bin_dir" "$(id -un)" >&2
    sudo_hint
    exit 1
fi

# Capture the running version before the upgrade; the dashboard restart is
# skipped when nothing changed.
prev_version=""
if [ -x "$bin_dir/bernie" ]; then
    prev_version=$("$bin_dir/bernie" version 2>/dev/null || printf '')
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

new_version=$("$bin_dir/bernie" version)
printf 'bernie: installed %s\n' "$new_version"

case :$PATH: in
    *:"$bin_dir":*) ;;
    *) printf 'bernie: add %s to your PATH\n' "$bin_dir" ;;
esac

# Start the dashboard when asked. Re-running the line on the same version
# does not restart the dashboard; a version bump does.
if [ "$start_after" = 1 ]; then
    changed=0
    if [ "$prev_version" != "$new_version" ]; then
        changed=1
    fi
    if [ "$changed" = 1 ] || [ "$prev_version" = "" ]; then
        # A loopback listen is the safe default; warn for any other.
        case "$listen" in
            127.0.0.1:*|[::1]*|localhost:*)
                ;;
            *)
                printf 'bernie: WARNING: %s is a non-loopback address; anyone with the URL can drive this host\n' "$listen"
                printf 'bernie: the dashboard will generate a token and require it on every request\n'
                ;;
        esac
        printf 'bernie: starting dashboard on %s ...\n' "$listen"
        if ! "$bin_dir/bernie" dash start --listen "$listen" >/dev/null 2>&1; then
            rc=$?
            printf 'bernie: dash start exited %d; the binary is installed, run `bernie dash start` to retry\n' "$rc"
        fi
    else
        printf 'bernie: version unchanged; dashboard left as-is (run `bernie dash status` to verify)\n'
    fi
    printf 'bernie: dashboard status:\n'
    "$bin_dir/bernie" dash status || true
    printf 'bernie: next step: open the URL above in a browser (Ctrl-C to keep the dashboard running)\n'
else
    printf 'bernie: next step: bernie up\n'
fi
