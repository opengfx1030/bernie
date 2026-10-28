#!/usr/bin/env bash
# Check the import layers of bernie (docs/architecture.md §2) and the
# leftovers of the old fork tooling. It reads the package list only, so it
# runs in a second.
#
# Layers, lowest first:
#   0 support   internal/support/*
#   1 machine   internal/machine/*
#   2 serving   internal/serving/*, internal/doctor, internal/setup,
#               internal/logdoc
#   3 chains    internal/app, internal/flow, internal/onboarding
#   4 front     internal/cli/*, internal/mcp, dashboard, cmd/bernie
# A package may import a package of its own layer or of a lower one. Never
# a higher one. The data packages (recipes, tunableop, tuned-moe, bench) have
# no layer: they hold data and import nothing of bernie.
set -euo pipefail

cd "$(dirname "$0")/.."

module=$(go list -m)
fail=0

layer() {
	case "$1" in
	"$module/internal/support"/*) echo 0 ;;
	"$module/internal/machine"/*) echo 1 ;;
	"$module/internal/serving"/*|"$module/internal/doctor"|"$module/internal/setup"|"$module/internal/logdoc") echo 2 ;;
	"$module/internal/app"|"$module/internal/flow"|"$module/internal/onboarding") echo 3 ;;
	"$module/internal/cli"|"$module/internal/cli"/*|"$module/internal/mcp"|"$module/dashboard"|"$module/cmd/bernie") echo 4 ;;
	*) echo -1 ;;
	esac
}

while read -r pkg imports; do
	[ -n "$pkg" ] || continue
	from=$(layer "$pkg")
	[ "$from" -ge 0 ] || continue
	for imp in $imports; do
		case "$imp" in
		"$module"/*) ;;
		*) continue ;;
		esac
		to=$(layer "$imp")
		[ "$to" -ge 0 ] || continue
		if [ "$to" -gt "$from" ]; then
			echo "$pkg imports $imp: a package never imports a higher layer" >&2
			fail=1
		fi
	done
done <<EOF
$(go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...)
EOF

if [ "$fail" -ne 0 ]; then
	echo "layers: see docs/architecture.md §2" >&2
	exit 1
fi

# The tooling of the old fork must not come back.
needle="rdnact""l"
if git grep -in -- "$needle" -- . >/dev/null; then
	echo "the old fork tooling ($needle) must not come back" >&2
	git grep -in -- "$needle" -- . >&2
	exit 1
fi
