# 7. A custom query language evaluated in process instead of translated to SQL

Date: 2026-11-19

## Status

Accepted

## Context

Users of test management tools expect a compact filter syntax
(`status:approved priority=high -tag:legacy`) with free text, regular
expressions, globs and boolean logic, usable in every list, in run
creation, in exports and in a cross-project search. Many fields are
derived rather than stored: folder path, category names, coverage, last
result, linked keys, criteria and step text. Translating such expressions to
SQL would require a SQL generator for two dialects, regex support differing
between SQLite and PostgreSQL, and careful injection-proofing.

## Decision

* `internal/query` implements a lexer, a recursive-descent parser
  (`NOT` > `AND` > `OR`, implicit AND, grouping) and an evaluator over a
  `Record` interface (field → list of string values).
* For each entity a `query.Schema` lists the accepted fields and aliases;
  unknown fields are parse errors with a byte position. The API builds
  `query.MapRecord`s from fully decorated entities (`internal/api/records.go`)
  and evaluates the parsed query against them in Go.
* Regular expressions use Go's RE2 engine. Expressions are limited to 4096
  bytes, 64 regexes/globs and nesting depth 128.
* `Query.MatchedFields` and `Query.Terms` support search highlighting;
  `Query.String` produces a normalised form that the client also composes.

## Consequences

* Derived and multi-valued fields are first-class and behave the same in
  every view, the export and the search.
* No user input reaches SQL text; RE2 rules out catastrophic backtracking.
* The same semantics on SQLite and PostgreSQL.
* Filtering loads all items of a project (search: of all projects of the
  tenant) and decorates them before matching. Cost is linear in the data
  size per request; this is acceptable for projects with thousands of
  items but would need indexing or SQL pre-filtering for much larger data.
* No pagination: list endpoints return all matches (search is capped by
  `limit`).
