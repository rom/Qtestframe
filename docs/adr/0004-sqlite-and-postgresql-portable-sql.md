# 4. SQLite and PostgreSQL through portable SQL, `?` rebinding and templated migrations

Date: 2026-11-19

## Status

Accepted

## Context

Small installations want zero external dependencies: one file database
next to the binary. Larger ones need PostgreSQL for concurrency, several
application nodes, replication and point-in-time recovery. Maintaining two
data access layers, or adopting an ORM, would double the testing effort or
hide the SQL that tenant isolation depends on.

## Decision

* `internal/db` opens either `modernc.org/sqlite` (pure Go) or
  `jackc/pgx/v5/stdlib`, both through `database/sql`.
* All SQL in `internal/store` is written once, in the common subset of both
  dialects, with `?` placeholders. `db.Rebind` converts them to `$1, $2, …`
  for PostgreSQL (ignoring `?` inside single-quoted literals); the store's
  `conn` wrapper applies it to every statement.
* Constructs used must exist in both engines (for example
  `UPDATE … RETURNING` for key allocation, `COALESCE`, `INSERT … SELECT`).
* Migrations are embedded SQL files (`internal/db/migrations/NNNN_*.sql`)
  with dialect tokens `{{TS}}`, `{{BOOL}}`, `{{TRUE}}`, `{{FALSE}}`
  rendered by `db.Render` (TIMESTAMP/INTEGER for SQLite,
  TIMESTAMPTZ/BOOLEAN for PostgreSQL). Each file runs in one transaction and
  is recorded in `schema_migrations`.
* SQLite is opened with **one connection** (`SetMaxOpenConns(1)`), WAL
  journal mode, `foreign_keys(1)` and `busy_timeout(5000)`. One connection
  serialises writers in the Go process and avoids `SQLITE_BUSY` errors and
  lock upgrade deadlocks.
* IDs are application-generated UUID strings, timestamps are written as UTC
  `time.Time`.

## Consequences

* One store implementation; the full test suite runs on both engines
  (`QTF_TEST_POSTGRES_DSN`, see [testing.md](../testing.md)).
* Engine-specific features (full-text search, JSONB, partial indexes) are
  not used; tags are a JSON array in a TEXT column and filtering happens in
  process ([ADR 0007](0007-in-process-query-language.md)).
* SQLite throughput is bounded by a single connection, which is adequate
  for one node with tens of concurrent users; readers wait for writers.
  `database.max_open_conns` only applies to PostgreSQL.
* Unique-violation detection (`db.IsUniqueViolation`) matches driver error
  messages of both engines.
* Migrations are forward-only; downgrades restore a backup.
