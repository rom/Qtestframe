# 13. A test coverage floor of 85 %

Date: 2026-11-19

## Status

Accepted

## Context

Much of the code is security-relevant (tenant filtering, RBAC, CSRF,
token handling, SSO, sandboxing) or has many edge cases (query language,
requirement lint, move semantics, exports). Regressions there are expensive
and often silent. Coverage is an imperfect measure, but a floor stops
untested code from accumulating and makes reviewers ask for tests.

## Decision

* Go: `make coverage-go` runs `scripts/coverage.sh`, which executes
  `go test -count=1 -coverpkg=<all internal packages except internal/testutil> -coverprofile=coverage.out ./...`
  and fails if the total statement coverage is below 85 %
  (`COVERAGE_FLOOR`, or the first script argument). Cross-package coverage
  counts code exercised by the API integration tests in every package.
* Web: `npm run coverage` runs Vitest with V8 coverage over `web/src/lib/**`
  and thresholds of 85 % for lines, functions, branches and statements
  (`web/vitest.config.ts`).
* `make coverage` runs both; CI runs both, the Go part also against
  PostgreSQL.

## Consequences

* New code needs tests to keep the build green; deleting tests that lower
  coverage is visible.
* Code that only runs in other processes (hardening, sandbox) counts
  because the child processes write coverage data to the same
  `-test.gocoverdir`.
* Vue components and views are outside the web threshold; logic is
  therefore kept in `web/src/lib`.
* The floor is a minimum, not a goal; low-value tests written only for the
  number should be rejected in review.
