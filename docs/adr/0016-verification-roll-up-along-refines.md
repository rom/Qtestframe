# 16. Criterion-level verification and roll-up along "refines"

Date: 2026-11-22

## Status

Accepted.

## Context

Coverage was computed per requirement from the last results of its linked
tests; evaluation criteria were descriptive (a link could name one
criterion, but every link counted the same). Hardware, facility and
governance requirements are verified by inspection, analysis,
demonstration or review, which no test result expresses, so they could
never reach "verified". Systems engineering also expects a decomposed
requirement to be verified only when its parts are, and coverage per
system element (subsystem, supplier, product).

## Decision

* **Evidence per criterion.** A trace link can be mapped to several
  criteria (`link_criteria`), each optionally through one test step.
  **Verification records** (`verification_records`) are evidence other
  than test results, for a criterion or the whole requirement, with a
  result (pass, fail, conditional), performer, date, note, archive
  documents, optional second-person sign-off, and validity until a date or
  until the requirement's content changes.
* **Verified** means every criterion has passing evidence (newest valid
  record per criterion; tests by last result or step result; a suspect
  link is pending) and all plain evidence passed.
* **Compatibility.** A link without criterion mapping is *plain* evidence
  and counts for every criterion without evidence of its own, so existing
  projects keep their state. Projects opt into **strict criterion
  coverage** (`verification_settings.strict_criteria`), where each
  criterion needs its own evidence.
* **Hierarchy = `refines`.** We reuse the existing relation instead of a
  new parent/child column: it already means decomposition (source refines,
  is a child of, target), is acyclic by construction, shows in the
  dependency graph and the query fields `parent`/`child`, is versioned in
  baselines' relations and imported/exported (ReqIF). Multiple parents are
  allowed. A parent is verified only when its own state is verified (or it
  has no criteria and no evidence) and every active child is.
* **Status follows the roll-up.** "Status follows evidence" now uses this
  verification for the requirement and every ancestor, so an unverified
  child under a verified parent cannot persist; with manual verification it
  is reported (`inconsistencies`, analytics finding
  `verified_parent_unverified_child`).
* **System elements** are a separate per-project tree
  (`system_elements`) with allocation tables for requirements and tests;
  counts include the elements below.
* The computation is a pure package (`internal/verification`) over data
  the store loads, so other features (release criteria, staleness, SoA,
  waivers) can call `Store.ProjectVerification` / `IsVerified`.

## Consequences

* Requirement lists compute the verification once per request (lazily,
  only when requirements are decorated): one extra pass over the project's
  requirements, tests, step results and records.
* Verification uses the latest result of a test across configurations
  (like "status follows evidence"), not the per-configuration coverage.
* A link mapped to criteria no longer counts for the other criteria; such
  links existed only with the single `criterion_id` before and are
  migrated to `link_criteria`.
