# 1. Record architecture decisions

Date: 2026-11-19

## Status

Accepted

## Context

Qtestframe combines a Go backend, an embedded web client, two database
engines, several authentication protocols and Linux hardening. Many of the
choices are trade-offs (security against operability, portability against
features) that are not obvious from the code alone. Reviewers and future
maintainers need to know why the system looks the way it does, and which
alternatives were rejected.

## Decision

We record significant architecture decisions as Architecture Decision
Records in the format described by Michael Nygard ("Documenting
Architecture Decisions", 2011): one Markdown file per decision in
`docs/adr/`, numbered sequentially (`NNNN-short-title.md`), with the
sections **Status**, **Context**, **Decision** and **Consequences**.

An ADR is immutable once accepted. A changed decision gets a new ADR that
supersedes the old one; the old ADR's status is updated to
"Superseded by ADR NNNN".

## Consequences

* Decisions and their reasons are reviewed together with the code.
* The index in [docs/README.md](../README.md) lists all ADRs.
* Writing an ADR costs a little time for every significant change.
