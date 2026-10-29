#!/usr/bin/env bash
# Live check of `bernie dash start|status|stop`: the detached service, the
# persistent token, the guard of the dashboard and the idempotent start.
#
# The script builds the binary, points every bernie directory at a fresh
# tree under $HOME (never /tmp), starts the dashboard on a free loopback
# port and checks:
#
#   1. status before the start: "not running"
#   2. start: the service comes up and prints the URL with the token
#   3. status after the start: "running" with the pid
#   4. the token file is mode 0600
#   5. a request without the token answers 401
#   6. the URL with ?token= answers 200 (the browser flow: the token sets
#      a cookie and redirects)
#   7. a request with the Authorization header answers 200
#   8. a second start fails: the service is already running
#   9. --rotate-token replaces the token
#  10. stop: the service is gone
#  11. status after the stop: "not running"
#  12. a second stop is not an error
#
# Requirements: bash, curl and go. The script skips itself when curl or go
# is missing.
set -uo pipefail

cd "$(dirname "$0")/.."

if ! command -v curl >/dev/null 2>&1; then
    printf 'test-dash.sh: skipping (curl is missing)\n'
    exit 0
fi
if ! command -v go >/dev/null 2>&1; then
    printf 'test-dash.sh: skipping (go is missing)\n'
    exit 0
fi

# Every bernie directory lives under $HOME, never under /tmp.
BASE="${HOME}/bernie-build/test-dash"
BIN="$BASE/bernie"
export BERNIE_STATE_DIR="$BASE/state"
export BERNIE_CONFIG_DIR="$BASE/config"
export BERNIE_DATA_DIR="$BASE/data"
export BERNIE_CACHE_DIR="$BASE/cache"
export BERNIE_ROOT="$BASE/host"

fail() { printf '  FAIL  %s\n' "$1"; exit 1; }
pass() { printf '  PASS  %s\n' "$1"; }

rm -rf "$BASE"
mkdir -p "$BERNIE_STATE_DIR" "$BERNIE_CONFIG_DIR" "$BERNIE_DATA_DIR" "$BERNIE_CACHE_DIR" "$BERNIE_ROOT"

printf 'test-dash.sh: building the binary\n'
go build -o "$BIN" ./cmd/bernie || fail "the binary does not build"

# A free loopback port.
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()' 2>/dev/null)
if [ -z "$PORT" ]; then
    PORT=$((20000 + $$ % 20000))
fi
ADDR="127.0.0.1:$PORT"
TOKEN_FILE="$BERNIE_STATE_DIR/dashboard/token"

printf 'test 1: status before the start\n'
out=$("$BIN" dash status 2>&1) || fail "status exited non-zero"
case "$out" in
    *"not running"*) pass "status says not running" ;;
    *) printf '%s\n' "$out"; fail "status does not say not running" ;;
esac

printf 'test 2: start brings the service up\n'
out=$("$BIN" dash start --listen "$ADDR" 2>&1) || fail "dash start exited non-zero"
case "$out" in
    *"?token="*) pass "the URL carries the token" ;;
    *) printf '%s\n' "$out"; fail "the URL carries no token" ;;
esac
case "$out" in
    *"ssh -L"*) pass "the ssh -L hint is printed" ;;
    *) fail "no ssh -L hint" ;;
esac

printf 'test 3: status after the start\n'
out=$("$BIN" dash status 2>&1) || fail "status exited non-zero"
case "$out" in
    *"running"*) pass "status says running" ;;
    *) printf '%s\n' "$out"; fail "status does not say running" ;;
esac

printf 'test 4: the token file is mode 0600\n'
mode=$(ls -l "$TOKEN_FILE" | cut -c1-10)
case "$mode" in
    -rw-------*) pass "token file mode $mode" ;;
    *) fail "token file mode is $mode, want -rw-------" ;;
esac

TOK=$(cat "$TOKEN_FILE")

printf 'test 5: a request without the token answers 401\n'
code=$(curl -s -o /dev/null -w '%{http_code}' "http://$ADDR/")
[ "$code" = "401" ] || fail "no token: got $code, want 401"
pass "no token: 401"

printf 'test 6: the URL with ?token= answers 200 after the redirect\n'
code=$(curl -s -L -c "$BASE/cookies" -o /dev/null -w '%{http_code}' "http://$ADDR/?token=$TOK")
[ "$code" = "200" ] || fail "with token: got $code, want 200"
pass "with token: 200"

printf 'test 7: the Authorization header answers 200\n'
code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOK" "http://$ADDR/api/hw")
[ "$code" = "200" ] || fail "bearer: got $code, want 200"
pass "bearer: 200"

printf 'test 8: a second start is refused\n'
out=$("$BIN" dash start --listen "$ADDR" 2>&1) && fail "the second start succeeded"
case "$out" in
    *"already running"*) pass "the second start is refused" ;;
    *) printf '%s\n' "$out"; fail "the second start is not refused clearly" ;;
esac

printf 'test 9: --rotate-token replaces the token\n'
"$BIN" dash stop >/dev/null 2>&1
"$BIN" dash start --listen "$ADDR" --rotate-token >/dev/null 2>&1 || fail "the start after the stop failed"
TOK2=$(cat "$TOKEN_FILE")
[ "$TOK" != "$TOK2" ] || fail "the token did not change"
code=$(curl -s -L -c "$BASE/cookies2" -o /dev/null -w '%{http_code}' "http://$ADDR/?token=$TOK2")
[ "$code" = "200" ] || fail "the new token: got $code, want 200"
pass "the token rotated"

printf 'test 10: stop ends the service\n'
out=$("$BIN" dash stop 2>&1) || fail "dash stop exited non-zero"
case "$out" in
    *"stopped"*) pass "stop says stopped" ;;
    *) printf '%s\n' "$out"; fail "stop does not say stopped" ;;
esac

printf 'test 11: status after the stop\n'
out=$("$BIN" dash status 2>&1) || fail "status exited non-zero"
case "$out" in
    *"not running"*) pass "status says not running" ;;
    *) printf '%s\n' "$out"; fail "status does not say not running" ;;
esac

printf 'test 12: a second stop is not an error\n'
"$BIN" dash stop >/dev/null 2>&1 || fail "the second stop failed"
pass "the second stop is not an error"

printf 'all dash tests passed\n'
