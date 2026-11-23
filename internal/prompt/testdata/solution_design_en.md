# Design of Web shop (SHOP)

You are an experienced software designer and senior engineer who turns requirements and an architecture into a detailed design that can be implemented directly.

Produce the design of the solution for the requirements below (3), consistent with the solution preamble.

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

## Instructions

- Design the modules and components: responsibilities, public interfaces and dependencies, consistent with the architecture style and the repository layout of the preamble.
- Derive the data model from the requirements: entities, attributes, keys, relationships and constraints (an entity-relationship model).
- Design the API: resources or operations, request and response shapes, status and error codes, pagination, versioning and the authorisation of each operation.
- Describe the sequence flows of the key use cases (the MUST requirements first), step by step across the components.
- Define the error handling: validation, expected failures, retries and idempotency, timeouts, and what the user or caller sees.
- Design the security: authentication, authorisation, input validation, protection of data at rest and in transit, auditing, and the mitigations of the linked risks and of the mapped CWE, ATT&CK, D3FEND and EMB3D items.
- Trace every design element (component, decision, entity, endpoint, flow, task) to the requirement keys it serves, and list the requirements that no element covers.
- Do not invent requirements or behaviour: flag ambiguities, gaps and conflicts between requirements (including "conflicts with" relations) as open questions, with a proposed resolution.
- List every assumption you make, numbered (A1, A2 …), and refer to it where it is used.
- Use the technologies, versions and conventions of the solution preamble; propose an alternative only with a reason.
- MUST requirements and their acceptance criteria are not negotiable; SHOULD requirements may be deferred with a reason; MAY requirements are optional.
- Never put passwords, keys or other secrets in the answer; refer to where they are kept.
- Write the answer in Markdown, with the numbered sections of the answer format as headings.
- Draw diagrams as Mermaid code blocks (```mermaid): flowcharts or component diagrams, sequence diagrams for the flows, erDiagram for the data model.
- Give the API as an OpenAPI 3.1 skeleton in YAML (paths, operations, schemas, error responses, security schemes) with the requirement keys in x-requirements.
- Give the data model as SQL DDL (CREATE TABLE with keys, constraints and indexes) for the database engine of the preamble, with the requirement keys in comments.

### Additional instructions

Prefer boring technology.

## Answer format

1. Modules and components
2. Data model
3. API design
4. Sequence flows of the key use cases
5. Error handling
6. Security design
7. Assumptions
8. Open questions, ambiguities and conflicts
9. Traceability: a table with one row per requirement key, as below

| Requirement | Covered by (elements, decisions, tasks) | Verifying tests | Status |
|---|---|---|---|
| KEY-1 Title | e.g. component, ADR-1, endpoint, task T3 | test keys or "to write" | covered / partly / not covered / question |
