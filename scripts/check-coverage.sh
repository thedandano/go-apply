#!/usr/bin/env bash
# Enforce a minimum test coverage threshold.
# Usage: check-coverage.sh [threshold [profile]]
#   threshold  minimum coverage percentage (default: 65)
#   profile    existing coverage profile to reuse; if absent, tests are run
set -euo pipefail

THRESHOLD="${1:-65}"
PROFILE="${2:-}"

if [ -z "$PROFILE" ]; then
  PROFILE="${TMPDIR:-/tmp}/go-apply-coverage.out"
  go test -coverprofile="$PROFILE" ./internal/... >/dev/null
fi

COVERAGE=$(go tool cover -func="$PROFILE" | grep '^total:' | awk '{print $3}' | tr -d '%')
echo "Total coverage: ${COVERAGE}%"

awk -v c="$COVERAGE" -v t="$THRESHOLD" \
  'BEGIN { if (c+0 < t+0) { print "Coverage " c "% is below the " t "% minimum"; exit 1 } }'
