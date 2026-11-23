#!/usr/bin/env bash
# Runs all Go tests with cross-package coverage and enforces the floor.
# Usage: scripts/coverage.sh [floor]   (default 85)
# Set QTF_TEST_POSTGRES_DSN to additionally exercise PostgreSQL.
set -euo pipefail
cd "$(dirname "$0")/.."
FLOOR="${1:-${COVERAGE_FLOOR:-85}}"
OUT="${COVERAGE_OUT:-coverage.out}"
export CGO_ENABLED=0
# internal/testutil is test scaffolding; cmd/ is a three-line main.
PKGS=$(go list ./internal/... | grep -v /internal/testutil | paste -sd, -)
# Only packages with tests are run (avoids the covdata tool requirement for
# test-less packages); their code is still measured through -coverpkg.
TESTED=$(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... | grep .)
# The API package runs for well over Go's default 10 minutes on PostgreSQL
# and with coverage; GO_TEST_TIMEOUT overrides the limit.
# shellcheck disable=SC2086
go test -count=1 -timeout "${GO_TEST_TIMEOUT:-45m}" -coverpkg="$PKGS" -coverprofile="$OUT" $TESTED
TOTAL=$(go tool cover -func="$OUT" | awk '/^total:/ {sub("%","",$3); print $3}')
echo "total coverage: ${TOTAL}% (floor ${FLOOR}%)"
if awk -v t="$TOTAL" -v f="$FLOOR" 'BEGIN { exit !(t+0 < f+0) }'; then
  echo "FAIL: coverage ${TOTAL}% is below the ${FLOOR}% floor" >&2
  exit 1
fi
