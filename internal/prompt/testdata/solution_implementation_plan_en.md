# Implementation plan for Web shop (SHOP)

You are an experienced technical lead who plans implementation work so that every task is traceable to the requirements and "done" means verified.

Break the implementation of the requirements below (3) into milestones, epics and tasks, in a workable order.

## Solution preamble

- **Architecture style:** modular monolith
- **Implementation languages and versions:** Go 1.26, TypeScript 5
- **Frameworks and libraries:** net/http, pgx, Vue 3
- **Databases (described; no credentials):**
  PostgreSQL 16, schema owned by the app,
  migrations embedded in the binary
- **Security and compliance constraints:** OIDC sign-in, RBAC
- **Out of scope:** Native mobile apps

### Technical context

Monorepo.

## Project summary

- Project: Web shop (SHOP)
- Description: An online shop
- Scope: the whole project — 3 requirements (of 5 in the project)
- Requirements by obligation: MUST 1, SHOULD 1, MAY 1
- Folders: Checkout (2), Accounts (1)
- Releases: 1.0 (planned)
- Variants: EU
- Linked risks: 1 · verifying tests: 1 (of 4 tests in the project)

## Requirements

The keywords MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, RECOMMENDED, MAY and OPTIONAL are to be interpreted as described in RFC 2119 and RFC 8174 when, and only when, they appear in all capitals. The obligation of each requirement is given in its first line.

### SHOP-REQ-1 Login

_MUST · Type: security · Priority: high · Status: approved · Folder: Accounts_

> The user MUST be able to log in.

- **Rationale:** Protect accounts
- **Acceptance criteria:**
  - Valid login succeeds (test) — threshold: < 2 s
- **Relations:** conflicts with SHOP-REQ-3 Guest checkout
- **Risks:** SHOP-RISK-1 Credential stuffing (level high; mitigation: Rate limiting and MFA; ATT&CK T1110.004)
- **Verified by:** SHOP-TC-1 Valid login
- **Standards and frameworks:** CWE-307; OWASP ASVS V2.2; D3FEND D3-MFA

### SHOP-REQ-2 Pay by card

_SHOULD · Type: functional · Priority: medium · Status: draft · Folder: Checkout/Payment_

> The shop SHOULD accept cards.

- **Relations:** depends on SHOP-REQ-1 Login

### SHOP-REQ-3 Guest checkout

_MAY · Folder: Checkout_

- **Acceptance criteria:**
  - No account needed
- **Standards and frameworks:** EMB3D MID-001

## Dependency order

The requirements with dependencies between them, prerequisites first (from the "depends on" relations):

SHOP-REQ-1 → SHOP-REQ-2

## Instructions

- Break the work into milestones, epics and tasks; every task names the requirement keys it implements or supports and is small enough for one person in a few days.
- Order the work by the dependencies between the requirements ("depends on", "refines"; see the dependency order), then by obligation and priority: MUST before SHOULD before MAY.
- Give each epic a definition of done: its acceptance criteria met, its verifying tests (by key and title) passing, and the reviews, documentation and deployment steps the preamble requires.
- Name the tests to write where a requirement has no verifying test yet.
- Show what can run in parallel and what blocks other work; flag the risks to the plan.
- Trace every design element (component, decision, entity, endpoint, flow, task) to the requirement keys it serves, and list the requirements that no element covers.
- Do not invent requirements or behaviour: flag ambiguities, gaps and conflicts between requirements (including "conflicts with" relations) as open questions, with a proposed resolution.
- List every assumption you make, numbered (A1, A2 …), and refer to it where it is used.
- Use the technologies, versions and conventions of the solution preamble; propose an alternative only with a reason.
- MUST requirements and their acceptance criteria are not negotiable; SHOULD requirements may be deferred with a reason; MAY requirements are optional.
- Never put passwords, keys or other secrets in the answer; refer to where they are kept.
- Write the answer in Markdown, with the numbered sections of the answer format as headings.

### Additional instructions

Prefer boring technology.

## Answer format

1. Milestones
2. Epics and tasks (table: id, task, requirement keys, depends on, estimate)
3. Order and dependencies
4. Definition of done per epic, with the verifying tests
5. Risks to the plan
6. Assumptions
7. Open questions, ambiguities and conflicts
8. Traceability: a table with one row per requirement key, as below

| Requirement | Covered by (elements, decisions, tasks) | Verifying tests | Status |
|---|---|---|---|
| KEY-1 Title | e.g. component, ADR-1, endpoint, task T3 | test keys or "to write" | covered / partly / not covered / question |
