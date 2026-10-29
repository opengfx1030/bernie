#!/usr/bin/env bash
# Check the dashboard web tree against the front-end rules in
# dashboard/AGENTS.md. The script reads its exceptions and its oversized-file
# entries from scripts/web-allowlist.txt.
set -euo pipefail

cd "$(dirname "$0")/.."

export LC_ALL=C

readonly ALLOWLIST="scripts/web-allowlist.txt"
readonly README="dashboard/README.md"
readonly WEB="dashboard/web"
readonly DATA_FILE="dashboard/web/js/data.js"
readonly LINE_LIMIT=400
readonly ALLOWLIST_LINE_CAP=1024
readonly DATA_PATTERN='fetch\(|EventSource|setInterval|setTimeout'

mode="check"
case "${1:-}" in
	"")
		;;
	--print-globals)
		mode="print-globals"
		;;
	-h|--help)
		echo "usage: scripts/check-web.sh [--print-globals]"
		exit 0
		;;
	*)
		echo "usage: scripts/check-web.sh [--print-globals]" >&2
		exit 2
		;;
esac
if [ "$#" -gt 1 ]; then
	echo "usage: scripts/check-web.sh [--print-globals]" >&2
	exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

failures=0
fail() {
	printf 'web check: %s\n' "$*" >&2
	failures=$((failures + 1))
}

find "$WEB" -type f -name '*.js' -print | sort > "$tmp/js-files"

# Each line is: path, declaration kind, name. Only declarations at column zero
# are published globals. The kind keeps the duplicate-function check separate
# from the manifest check.
: > "$tmp/declarations"
while IFS= read -r file; do
	awk '
		/^async[[:space:]]+function[[:space:]]+[A-Za-z_$][A-Za-z0-9_$]*/ ||
		/^function[[:space:]]+[A-Za-z_$][A-Za-z0-9_$]*/ ||
		/^const[[:space:]]+[A-Za-z_$][A-Za-z0-9_$]*/ ||
		/^let[[:space:]]+[A-Za-z_$][A-Za-z0-9_$]*/ ||
		/^var[[:space:]]+[A-Za-z_$][A-Za-z0-9_$]*/ {
			line = $0
			if (sub(/^async[[:space:]]+function[[:space:]]+/, "", line)) {
				kind = "function"
			} else if (sub(/^function[[:space:]]+/, "", line)) {
				kind = "function"
			} else if (sub(/^const[[:space:]]+/, "", line)) {
				kind = "const"
			} else if (sub(/^let[[:space:]]+/, "", line)) {
				kind = "let"
			} else {
				kind = "var"
			}
			name = line
			sub(/[^A-Za-z0-9_$].*$/, "", name)
			if (name != "" && !seen[name]++) {
				print kind "\t" name
			}
		}
	' "$file" | while IFS=$'\t' read -r kind name; do
		printf '%s\t%s\t%s\n' "$file" "$kind" "$name"
	done >> "$tmp/declarations"
done < "$tmp/js-files"

if [ "$mode" = "print-globals" ]; then
	while IFS= read -r file; do
		cell=""
		while IFS= read -r name; do
			if [ -n "$cell" ]; then
				cell="$cell, "
			fi
			cell="${cell}\`${name}\`"
		done < <(awk -F'\t' -v file="$file" '$1 == file { print $3 }' "$tmp/declarations")
		if [ -z "$cell" ]; then
			cell="–"
		fi
		printf '| `%s` | %s |\n' "$file" "$cell"
	done < "$tmp/js-files"
	exit 0
fi

if [ ! -f "$ALLOWLIST" ]; then
	fail "the allowlist is missing: $ALLOWLIST"
	: > "$tmp/exceptions"
	: > "$tmp/sizes"
else
	awk -F'\t' '
		/^# Data-path exceptions\.$/ { section = "exceptions"; next }
		/^# Oversized-file allowlist\.$/ { section = "sizes"; next }
		/^#/ { next }
		/^[[:space:]]*$/ { next }
		section == "exceptions" { print }
	' "$ALLOWLIST" > "$tmp/exceptions"
	awk -F'\t' '
		/^# Data-path exceptions\.$/ { section = "exceptions"; next }
		/^# Oversized-file allowlist\.$/ { section = "sizes"; next }
		/^#/ { next }
		/^[[:space:]]*$/ { next }
		section == "sizes" { print }
	' "$ALLOWLIST" > "$tmp/sizes"
fi

# Data path. dashboard/web/js/data.js is the one data path. Every other file
# that opens the network or owns a timer must have a documented exception.
: > "$tmp/exception-paths"
while IFS=$'\t' read -r path reason; do
	[ -n "$path" ] || continue
	case "$path" in
		dashboard/web/*) ;;
		*) fail "the data-path exception is outside dashboard/web: $path" ;;
	esac
	if [ -z "$reason" ]; then
		fail "the data-path exception has no reason: $path"
	fi
	if [ ! -f "$path" ]; then
		fail "the data-path exception names a missing file: $path"
	elif ! grep -Eq "$DATA_PATTERN" "$path"; then
		fail "the data-path exception is unused: $path"
	fi
	printf '%s\n' "$path" >> "$tmp/exception-paths"
done < "$tmp/exceptions"
while IFS= read -r path; do
	[ -n "$path" ] || continue
	fail "the data-path exception is listed twice: $path"
done < <(sort "$tmp/exception-paths" | uniq -d)

while IFS= read -r file; do
	[ "$file" = "$DATA_FILE" ] && continue
	if grep -Eq "$DATA_PATTERN" "$file" && ! grep -Fxq "$file" "$tmp/exception-paths"; then
		fail "$file uses fetch, EventSource or a timer outside $DATA_FILE"
	fi
done < "$tmp/js-files"

# File size. Every file over the limit needs an owner note in the allowlist.
: > "$tmp/allowed-size-paths"
while IFS=$'\t' read -r path expected note; do
	[ -n "$path" ] || continue
	case "$expected" in
		''|*[!0-9]*) fail "the oversized-file entry has no line count: $path" ;;
	esac
	if [ -z "$note" ]; then
		fail "the oversized-file entry has no owner note: $path"
	fi
	if [ ! -f "$path" ]; then
		fail "the oversized-file entry names a missing file: $path"
		continue
	fi
	actual=$(awk 'END { print NR }' "$path")
	if [ "$actual" -le "$LINE_LIMIT" ]; then
		fail "the oversized-file entry is no longer needed: $path ($actual lines)"
		continue
	fi
	if [ "$actual" -gt "$ALLOWLIST_LINE_CAP" ]; then
		fail "$path has $actual lines; even a listed file stays under $ALLOWLIST_LINE_CAP"
		continue
	fi
	if [ "$actual" -ne "$expected" ]; then
		fail "the oversized-file entry is stale: $path (allowlist $expected, actual $actual)"
		continue
	fi
	printf '%s\n' "$path" >> "$tmp/allowed-size-paths"
	printf 'web check: warning: %s has %s lines; %s\n' "$path" "$actual" "$note"
done < "$tmp/sizes"

# The size rule covers code files. Data files grow with their content.
find "$WEB" internal -type f \( -name '*.go' -o -name '*.js' -o -name '*.css' -o -name '*.html' \) -print | sort > "$tmp/size-files"
while IFS= read -r path; do
	[ -f "$path" ] || continue
	actual=$(awk 'END { print NR }' "$path")
	if [ "$actual" -gt "$LINE_LIMIT" ] && ! grep -Fxq "$path" "$tmp/allowed-size-paths"; then
		fail "$path has $actual lines; add it to $ALLOWLIST or split it"
	fi
done < "$tmp/size-files"

# Duplicate top-level functions. A name in two files is a global collision.
awk -F'\t' '$2 == "function" { print $3 "\t" $1 }' "$tmp/declarations" | sort > "$tmp/functions"
awk -F'\t' '
	function flush() {
		if (count > 1) {
			print name "\t" files
		}
	}
	$1 != current {
		flush()
		current = $1
		count = 0
		files = ""
		last = ""
	}
	$2 != last {
		count++
		if (count == 1) {
			files = $2
		} else {
			files = files ", " $2
		}
		last = $2
	}
	END { flush() }
' "$tmp/functions" > "$tmp/duplicate-functions"
while IFS=$'\t' read -r name files; do
	[ -n "$name" ] || continue
	fail "the top-level function '$name' is declared in $files"
done < "$tmp/duplicate-functions"

# Published globals manifest. The README table and the tree must name the
# same globals. A name published by two files is also a collision.
awk -F'\t' '{ print $1 "\t" $3 }' "$tmp/declarations" | sort > "$tmp/real-globals"
if [ ! -f "$README" ]; then
	fail "the published-globals table is missing: $README"
	: > "$tmp/table-rows"
else
	awk '
		/^## Published globals[[:space:]]*$/ { section = 1; next }
		/^## / { section = 0 }
		section && /^\|[[:space:]]*`dashboard\/web\/[^`]+\.js`[[:space:]]*\|/ { print }
	' "$README" > "$tmp/table-rows"
fi
if [ ! -s "$tmp/table-rows" ]; then
	fail "the published-globals table has no rows in $README"
fi

awk -F'`' '{ print $2 }' "$tmp/table-rows" | sort > "$tmp/table-files-all"
awk -F'`' '{ print $2 }' "$tmp/table-rows" | sort -u > "$tmp/table-files"
awk -F'`' '
	{
		path = $2
		for (i = 4; i <= NF; i += 2) {
			print path "\t" $i
		}
	}
' "$tmp/table-rows" | sort > "$tmp/table-globals"

while IFS= read -r path; do
	[ -n "$path" ] || continue
	fail "the published-globals table lists $path more than once"
done < <(sort "$tmp/table-files-all" | uniq -d)
while IFS= read -r row; do
	[ -n "$row" ] || continue
	fail "the published-globals table repeats $row"
done < <(sort "$tmp/table-globals" | uniq -d)

while IFS= read -r path; do
	[ -n "$path" ] || continue
	fail "the published-globals table has no row for $path"
done < <(comm -23 "$tmp/js-files" "$tmp/table-files")
while IFS= read -r path; do
	[ -n "$path" ] || continue
	fail "the published-globals table names a missing file: $path"
done < <(comm -13 "$tmp/js-files" "$tmp/table-files")
while IFS=$'\t' read -r path name; do
	[ -n "$path" ] || continue
	fail "$path publishes '$name', but the published-globals table does not list it"
done < <(comm -23 "$tmp/real-globals" "$tmp/table-globals")
while IFS=$'\t' read -r path name; do
	[ -n "$path" ] || continue
	fail "the published-globals table lists '$name' for $path, but the file does not publish it"
done < <(comm -13 "$tmp/real-globals" "$tmp/table-globals")

cut -f2 "$tmp/real-globals" | sort | uniq -d > "$tmp/duplicate-global-names"
awk -F'\t' 'NR == FNR { duplicate[$1] = 1; next } !($2 in duplicate)' \
	"$tmp/duplicate-functions" "$tmp/duplicate-global-names" > "$tmp/duplicate-other-globals"
while IFS= read -r name; do
	[ -n "$name" ] || continue
	paths=$(awk -F'\t' -v name="$name" '$2 == name { print $1 }' "$tmp/real-globals" | sort -u | paste -sd, -)
	fail "the global '$name' is published by $paths"
done < "$tmp/duplicate-other-globals"

if [ "$failures" -ne 0 ]; then
	printf 'web check: failed with %s problem(s)\n' "$failures" >&2
	exit 1
fi
printf 'web check: ok\n'
