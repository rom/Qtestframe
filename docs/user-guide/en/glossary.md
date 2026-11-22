# Glossary

The words of testing, requirements engineering and the standards and formats behind Qtestframe, explained in a few sentences each, with what they mean in Qtestframe and where to read more. Definitions are written for this guide; where a term comes from a standard or a common vocabulary such as the ISTQB Glossary or ISO/IEC/IEEE 29119, the source is named and listed on the [References](references.md) page. For the big picture, read [How Qtestframe works](how-it-works.md).

**Jump to:** [A](#a) · [B](#b) · [C](#c) · [D](#d) · [E](#e) · [F](#f) · [G](#g) · [H](#h) · [I](#i) · [J](#j) · [K](#k) · [L](#l) · [M](#m) · [N](#n) · [O](#o) · [P](#p) · [Q](#q) · [R](#r) · [S](#s) · [T](#t) · [U](#u) · [V](#v) · [W](#w)

## Entries by topic

**Testing:** [Acceptance testing](#acceptance-testing) · [Accessibility testing](#accessibility-testing) · [Behaviour-driven development](#behaviour-driven-development) · [Black-box testing](#black-box-testing) · [Boundary value analysis](#boundary-value-analysis) · [Code coverage](#code-coverage) · [Continuous integration](#continuous-integration) · [Coverage](#coverage) · [Data-driven testing](#data-driven-testing) · [Decision table testing](#decision-table-testing) · [Defect](#defect) · [Defect lifecycle](#defect-lifecycle) · [End-to-end testing](#end-to-end-testing) · [Entry criteria](#entry-criteria) · [Equivalence partitioning](#equivalence-partitioning) · [Error guessing](#error-guessing) · [Exit criteria](#exit-criteria) · [Exploratory testing](#exploratory-testing) · [Flaky test](#flaky-test) · [Functional testing](#functional-testing) · [Grey-box testing](#grey-box-testing) · [Integration testing](#integration-testing) · [Load testing](#load-testing) · [Non-functional testing](#non-functional-testing) · [Pairwise testing](#pairwise-testing) · [Performance testing](#performance-testing) · [Priority](#priority) · [Regression testing](#regression-testing) · [Requirement coverage](#requirement-coverage) · [Security testing](#security-testing) · [Session-based test management](#session-based-test-management) · [Severity](#severity) · [Shift-left](#shift-left) · [Smoke and sanity testing](#smoke-and-sanity-testing) · [State transition testing](#state-transition-testing) · [Stress testing](#stress-testing) · [System testing](#system-testing) · [Test case](#test-case) · [Test configuration](#test-configuration) · [Test cycle](#test-cycle) · [Test data](#test-data) · [Test design technique](#test-design-technique) · [Test environment](#test-environment) · [Test execution](#test-execution) · [Test level](#test-level) · [Test oracle](#test-oracle) · [Test plan](#test-plan) · [Test result](#test-result) · [Test run](#test-run) · [Test step](#test-step) · [Test suite](#test-suite) · [Test type](#test-type) · [Test-driven development](#test-driven-development) · [Unit testing](#unit-testing) · [Usability testing](#usability-testing) · [V-model](#v-model) · [Validation](#validation) · [Verification](#verification) · [White-box testing](#white-box-testing)

**Requirements:** [Acceptance criteria](#acceptance-criteria) · [Backward traceability](#backward-traceability) · [Baseline](#baseline) · [Bidirectional traceability](#bidirectional-traceability) · [Business requirement](#business-requirement) · [Change impact](#change-impact) · [Constraint](#constraint) · [Derived requirement](#derived-requirement) · [Deviation](#deviation) · [EARS](#ears) · [Evaluation criterion](#evaluation-criterion) · [Forward traceability](#forward-traceability) · [Functional requirement](#functional-requirement) · [INCOSE rules](#incose-rules) · [Non-functional requirement](#non-functional-requirement) · [Obligation level](#obligation-level) · [Quality attribute](#quality-attribute) · [Refinement](#refinement) · [Requirement](#requirement) · [Requirement key words](#requirement-key-words) · [Requirement quality](#requirement-quality) · [Residual risk](#residual-risk) · [Risk](#risk) · [Risk-based testing](#risk-based-testing) · [SKA, BÖR and FÅR](#ska-bör-and-får) · [Software requirement](#software-requirement) · [Specification](#specification) · [Stakeholder](#stakeholder) · [Stakeholder requirement](#stakeholder-requirement) · [Statement of Applicability](#statement-of-applicability) · [Suspect link](#suspect-link) · [System requirement](#system-requirement) · [Trace link](#trace-link) · [Traceability](#traceability) · [Traceability matrix](#traceability-matrix) · [User requirement](#user-requirement) · [Verification method](#verification-method) · [Verification record](#verification-record) · [Waiver](#waiver)

**Standards, formats and frameworks:** [Bearer token](#bearer-token) · [CSV](#csv) · [CWE](#cwe) · [Cyber Resilience Act](#cyber-resilience-act) · [ETag and If-Match](#etag-and-if-match) · [EU AI Act](#eu-ai-act) · [HOTP](#hotp) · [HTTP semantics](#http-semantics) · [ISO/IEC 25010](#isoiec-25010) · [ISO/IEC 27001 Annex A](#isoiec-27001-annex-a) · [ISO/IEC/IEEE 29119](#isoiecieee-29119) · [ISO/IEC/IEEE 29148](#isoiecieee-29148) · [ISTQB](#istqb) · [JSON Web Token](#json-web-token) · [JUnit XML](#junit-xml) · [Landlock](#landlock) · [Markdown](#markdown) · [MITRE ATLAS](#mitre-atlas) · [MITRE ATT&CK](#mitre-attck) · [MITRE D3FEND](#mitre-d3fend) · [MITRE EMB3D](#mitre-emb3d) · [OAuth 2.0](#oauth-20) · [OpenAPI](#openapi) · [OpenID Connect](#openid-connect) · [OSLC](#oslc) · [PKCE](#pkce) · [Problem Details](#problem-details) · [ReqIF](#reqif) · [SAML 2.0](#saml-20) · [SCIM](#scim) · [seccomp](#seccomp) · [STIX](#stix) · [Syslog](#syslog) · [TOTP](#totp) · [WCAG](#wcag) · [WebAuthn and FIDO2](#webauthn-and-fido2)

**Qtestframe:** [API token](#api-token) · [Audit log](#audit-log) · [Background job](#background-job) · [Backup](#backup) · [Catalogue](#catalogue) · [Code link](#code-link) · [Consistency check](#consistency-check) · [CSRF protection](#csrf-protection) · [Database](#database) · [Discussion](#discussion) · [Issue tracker](#issue-tracker) · [Iteration](#iteration) · [Key](#key) · [Language analysis](#language-analysis) · [Multi-factor authentication](#multi-factor-authentication) · [Notification](#notification) · [Organisation](#organisation) · [Project](#project) · [Project archive](#project-archive) · [qtestframe report](#qtestframe-report) · [Query language](#query-language) · [Release](#release) · [Review](#review) · [Revision](#revision) · [Role and permission](#role-and-permission) · [Script runner](#script-runner) · [Secrets encryption](#secrets-encryption) · [Session](#session) · [Shared step](#shared-step) · [Stale result](#stale-result) · [System element](#system-element) · [Traceability rule](#traceability-rule) · [Trash](#trash) · [Variant](#variant) · [Web test](#web-test) · [Webhook](#webhook)

## A

### Acceptance criteria

The conditions a product or a requirement must meet to be accepted by the customer, a user or another authorised party. Good acceptance criteria are concrete and checkable, so that everyone agrees beforehand what "done" means. In agile work they are often written under a user story; in classic specifications they are the verification part of each requirement.

**In Qtestframe:** acceptance criteria are written as [evaluation criteria](#evaluation-criterion) of a requirement: a description, a method and an optional threshold. A requirement counts as verified only when every criterion has passing evidence.

**See also:** [Evaluation criterion](#evaluation-criterion), [Acceptance testing](#acceptance-testing), [Evaluation criteria](requirements.md#evaluation-criteria)

### Acceptance testing

The test level that decides whether a system is ready to be accepted: by its users (user acceptance testing), by operations (operational acceptance), or against a contract or regulation. It answers "is this what we need?" rather than "does the code work?", so it is mostly a form of [validation](#validation). Acceptance tests are usually derived from [acceptance criteria](#acceptance-criteria) and business processes (ISTQB Glossary).

**In Qtestframe:** plan acceptance testing as a [test cycle](#test-cycle) or [run](#test-run) of its own, for example *Release 2.1 acceptance*, and tag or categorise the tests (`tag:acceptance`). The template for *Business* requirements suggests an acceptance test.

**See also:** [Test level](#test-level), [System testing](#system-testing), [Release](#release)

### Accessibility testing

Testing whether people with disabilities can use the product: with a keyboard only, a screen reader, magnification, high contrast, or without relying on colour. Automated checkers find part of the problems; the rest needs manual checks against a standard such as [WCAG](#wcag).

**In Qtestframe:** the catalogue has accessibility requirements and tests that cite WCAG 2.2. The web client itself aims at WCAG 2.1 AA and is checked with automated accessibility tests in light, dark and high-contrast themes.

**See also:** [WCAG](#wcag), [Non-functional testing](#non-functional-testing), [Accessibility and languages](accessibility-and-languages.md)

### API token

A secret string that a script or a CI pipeline sends instead of a password to call the API. It belongs to a user and an organisation, has a role that can be lower than the user's, and can expire or be revoked.

**In Qtestframe:** tokens start with `qtf_`, are sent as a [bearer token](#bearer-token) and are shown only once, when you create them; the server stores only a hash. You need a token for [`qtestframe report`](#qtestframe-report), the JUnit upload and the Playwright reporter.

**See also:** [API tokens](administration.md#api-tokens), [Bearer token](#bearer-token), [Import results from CI](runs.md#import-results-from-ci)

### Audit log

A tamper-evident record of who did what and when, kept for security reviews and audits. Unlike the history of an item, it also records sign-ins, permission changes, exports and administrative actions.

**In Qtestframe:** the audit log of an organisation is under **Administration → Audit log** (needs `audit.read`); every review decision, waiver signature, import and setting change is recorded there. Administrators can also send it to a central [syslog](#syslog) server.

**See also:** [Audit log](administration.md#audit-log), [Revision](#revision), [Syslog](#syslog)

## B

### Background job

Work the server does on its own, outside a single click: purging expired sessions, recording daily metrics, sending e-mail digests and scheduled reports, delivering webhooks and running automated tests. Jobs run inside the server process; there is no separate job server.

**In Qtestframe:** you see the results, not the jobs: dashboard trends, digests in your mailbox, run results filling in while **Run automated tests** works through a run.

**See also:** [Background work](how-it-works.md#background-work), [Webhook](#webhook), [Script runner](#script-runner)

### Backup

A copy of all data from which an installation can be restored after a failure. For Qtestframe a backup of the database plus the key of the stored secrets is a backup of the whole installation.

**In Qtestframe:** super admins take and restore backups under **Administration → Backup**, or with `qtestframe backup` and `restore` on the server. A restore signs everyone out. A single project is saved and moved with a [project archive](#project-archive) instead.

**See also:** [Backup and restore](administration.md#backup-and-restore), [Project archive](#project-archive), [Backup and migration](../../admin-guide/en/backup-and-migration.md)

### Backward traceability

Tracing from a lower-level or later artefact back to its origin: from a test to the requirement it verifies, from a software requirement to the system or stakeholder requirement it was derived from. It shows why something exists and finds "orphans" without a reason.

**In Qtestframe:** the **Linked requirements** of a test, the *refines* and *derived from* relations of a requirement, and the [traceability rules](#traceability-rule) such as "every test verifies a requirement" check backward traceability.

**See also:** [Forward traceability](#forward-traceability), [Bidirectional traceability](#bidirectional-traceability), [Traceability rules](traceability-and-coverage.md#traceability-rules)

### Baseline

An agreed, frozen state of a set of items that serves as the reference for further work and for changes. Typical baselines are the approved specification of a release or the test basis handed to a supplier. Changes after the baseline are made under change control and compared against it.

**In Qtestframe:** a baseline records which version of every requirement and test belonged to it; it copies nothing you can edit, so deleting it never changes the items. You can compare two baselines, see the [change impact](#change-impact) since one, sign it and export it as ReqIF.

**See also:** [Revision](#revision), [Baselines](baselines.md), [Release](#release)

### Bearer token

An access token that is valid for whoever holds it, sent in the HTTP header `Authorization: Bearer <token>`. RFC 6750 defines this way of sending OAuth 2.0 access tokens; since anyone who has the token can use it, it must only travel over TLS and be kept secret.

**In Qtestframe:** [API tokens](#api-token) and SCIM tokens are sent in exactly this header. The web client does not use tokens but a [session](#session) cookie.

**See also:** [API token](#api-token), [OAuth 2.0](#oauth-20), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

### Behaviour-driven development

BDD: a way of working where the team describes the wanted behaviour as concrete examples in a structured language (*Given … When … Then …*) before it is built. The examples serve as requirements, as acceptance criteria and, when automated, as tests.

**In Qtestframe:** write a scenario as the steps of a test (Given as precondition, When as action, Then as expected result) and link it to the requirement. Automated scenarios report their results through JUnit XML like any other test.

**See also:** [Test-driven development](#test-driven-development), [Acceptance criteria](#acceptance-criteria), [JUnit XML](#junit-xml)

### Bidirectional traceability

Traceability in both directions at once: from every requirement to the tests and design that realise it, and from every test or design element back to its requirement. Many safety and quality standards ask for it, because it shows both that nothing is missing and that nothing is superfluous.

**In Qtestframe:** a [trace link](#trace-link) is stored once and shown from both ends: on the requirement as **Linked tests**, on the test as **Linked requirements**, and in the [traceability matrix](#traceability-matrix).

**See also:** [Forward traceability](#forward-traceability), [Backward traceability](#backward-traceability), [Traceability](#traceability)

### Black-box testing

Testing based on the specification only, without looking at the code or the inner structure: the tester knows what goes in and what should come out. [Equivalence partitioning](#equivalence-partitioning), [boundary values](#boundary-value-analysis), [decision tables](#decision-table-testing) and [state transitions](#state-transition-testing) are black-box techniques (ISO/IEC/IEEE 29119-4).

**In Qtestframe:** most manual tests are black-box tests derived from the requirement text and its evaluation criteria.

**See also:** [White-box testing](#white-box-testing), [Grey-box testing](#grey-box-testing), [Test design technique](#test-design-technique)

### Boundary value analysis

A test design technique that puts test values at the edges of [equivalence partitions](#equivalence-partitioning), because faults cluster there: "less than" written instead of "less than or equal", off-by-one loops. For a field that accepts 1 to 100, you test 0, 1, 100 and 101 (and with three-value boundaries also 2 and 99).

**In Qtestframe:** put the boundary values in the [test data](#test-data) of a data-driven test, one row per value, so that each becomes its own iteration with its own result.

**See also:** [Equivalence partitioning](#equivalence-partitioning), [Data-driven testing](#data-driven-testing), [Data-driven tests](tests.md#data-driven-tests)

### Business requirement

A requirement that states a goal or need of the organisation, such as "reduce manual order handling by half", rather than a function of a system. Business requirements explain why a project exists and are refined into stakeholder, user and system requirements.

**In Qtestframe:** choose the type **Business**. Relate the requirements that implement it with **refines**, so that the requirement hierarchy rolls their verification up to it.

**See also:** [Stakeholder requirement](#stakeholder-requirement), [Refinement](#refinement), [Requirement](#requirement)

## C

### Catalogue

A library of ready-made requirements, tests and risks for common needs, such as authentication, logging, privacy or accessibility, that cite the laws, standards and best practices they are based on.

**In Qtestframe:** the [catalogue](catalogue.md) lets you choose the laws and standards a project has to comply with and adds the matching requirements with their tests and trace links. It also maps risks to MITRE ATT&CK, ATLAS, D3FEND and EMB3D.

**See also:** [Catalogue](catalogue.md), [MITRE ATT&CK](#mitre-attck), [Statement of Applicability](#statement-of-applicability)

### Change impact

What a change may affect: the tests that verify a changed requirement, the requirements that refine or depend on it, their results, documents and open defects. Change impact analysis finds these before or after a change, so that nothing is left out of date unnoticed.

**In Qtestframe:** when the substance of a requirement or test changes, its links and relations become [suspect](#suspect-link), results become [stale](#stale-result), tracker issues and generated documents are marked out of date, and the impact travels on along the relations up to the project's impact depth. The **Impact** tab and the impact analysis show the paths.

**See also:** [Change impact](traceability-and-coverage.md#change-impact), [Suspect link](#suspect-link), [Baseline](#baseline)

### Code coverage

The share of the code that tests execute, measured as statements, branches, conditions or paths. It shows untested code, but high code coverage alone says nothing about whether the requirements are met: code that does the wrong thing can be fully covered.

**In Qtestframe:** code coverage is measured by your build tools, not by Qtestframe. Qtestframe measures [requirement coverage](#requirement-coverage), and links code changes and builds to requirements and tests through [code links](#code-link).

**See also:** [Requirement coverage](#requirement-coverage), [White-box testing](#white-box-testing), [Coverage](#coverage)

### Code link

A link between an item and a change in a version control system: a commit, or a merge or pull request.

**In Qtestframe:** commits and merge requests that name an item key (`SHOP-REQ-12`) are linked automatically through GitHub and GitLab webhooks, or by hand in the **Code** tab. Builds uploaded by CI show which commits were tested, and the regression selection uses the links.

**See also:** [Code links](variants-and-code.md#code-links), [Continuous integration](#continuous-integration), [Regression testing](#regression-testing)

### Consistency check

An automatic check for problems across many items: requirements without tests, approved requirements with unapproved parents, suspected duplicates, possibly conflicting requirements.

**In Qtestframe:** built-in checks and your own [traceability rules](#traceability-rule) run on demand and in the analytics; each violation names the item and offers fixes such as **Mark as duplicate** or **Not a problem**.

**See also:** [Built-in consistency checks](traceability-and-coverage.md#built-in-consistency-checks), [Duplicates and conflicts](duplicates-and-conflicts.md), [Traceability rule](#traceability-rule)

### Constraint

A requirement that limits how a solution may be built instead of saying what it does: a mandated platform, a programming language, a standard interface, a law, a budget. Constraints narrow the design space and are often non-negotiable.

**In Qtestframe:** choose the type **Constraint**. Verify it by **Inspection** or **Analysis** when there is nothing to execute.

**See also:** [Non-functional requirement](#non-functional-requirement), [Verification method](#verification-method), [Requirement](#requirement)

### Continuous integration

CI: every change is merged often into a shared branch and automatically built and tested, so that problems show up within minutes. Continuous delivery and deployment (CD) extend the pipeline up to a releasable or released product. Testing in CI/CD relies on fast, reliable automated tests at several [test levels](#test-level).

**In Qtestframe:** CI pipelines upload their results with [`qtestframe report`](#qtestframe-report), as [JUnit XML](#junit-xml) or with the Playwright reporter. The results land in a run, count for coverage and release exit criteria, and record the build.

**See also:** [Import results from CI](runs.md#import-results-from-ci), [Shift-left](#shift-left), [Builds](variants-and-code.md#builds)

### Coverage

How much of something the tests reach. The "something" decides the kind of coverage: requirements, risks, code, configurations or data combinations. Coverage is always relative to a model and never proves the absence of defects.

**In Qtestframe:** "coverage" means [requirement coverage](#requirement-coverage): the verification state of each requirement derived from the latest results of its linked tests (*None*, *Not run*, *Passed*, *Failed*, *Partial*). Risk coverage is shown in the risk register.

**See also:** [How coverage is calculated](traceability-and-coverage.md#how-coverage-is-calculated), [Code coverage](#code-coverage), [Requirement coverage](#requirement-coverage)

### CSRF protection

Cross-site request forgery is an attack in which another website makes your browser send a request to an application you are signed in to. The defence is a secret token per session that the page must send with every change, which a foreign site cannot know, and a check of the request's origin.

**In Qtestframe:** every change from the web client sends the session's CSRF token in a header and passes an origin check; the session cookie is also `SameSite=Lax`. API calls with a [bearer token](#bearer-token) need no CSRF token.

**See also:** [Session](#session), [Security](how-it-works.md#security), [Sessions](../../admin-guide/en/authentication.md#sessions)

### CSV

Comma-separated values: a plain text table with one record per line. RFC 4180 describes the common format, including quoting of fields that contain commas, quotes or line breaks.

**In Qtestframe:** the CSV export writes RFC 4180 files in UTF-8, one file per aspect or a ZIP with several. CSV files from DOORS or Polarion can be imported with a column mapping.

**See also:** [Export](reports-and-export.md#export), [Exchanging data with other tools](reports-and-export.md#exchanging-data-with-other-tools), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

### CWE

The Common Weakness Enumeration from MITRE: a numbered list of the kinds of weaknesses in software and hardware, such as *CWE-79 Cross-site scripting* or *CWE-521 Weak password requirements*. It gives security requirements and findings a shared vocabulary.

**In Qtestframe:** catalogue requirements, tests and risks cite CWE entries, and the search finds them by id (`CWE-79`).

**See also:** [MITRE ATT&CK](#mitre-attck), [Catalogue](#catalogue), [Sources cited in the catalogue](references.md#sources-cited-in-the-catalogue)

### Cyber Resilience Act

The EU regulation (EU) 2024/2847 on cybersecurity requirements for products with digital elements: secure by design, without known exploitable vulnerabilities, with security updates and vulnerability handling over the support period. Most obligations apply from December 2027.

**In Qtestframe:** choose *Cyber Resilience Act* under **Comply with laws, standards and best practices** in the catalogue to add its essential requirements with tests; the compliance analytics then show how far they are verified.

**See also:** [EU AI Act](#eu-ai-act), [Compliance with laws and standards](traceability-and-coverage.md#compliance-with-laws-and-standards), [Catalogue](#catalogue)

## D

### Data-driven testing

Writing a test once and running it with several sets of input and expected values taken from a table. It separates the test logic from the test data, so that adding a case means adding a row.

**In Qtestframe:** give a test parameters and rows under **Test data** and use them as `{{name}}` in steps. Each row becomes an **iteration** of the execution, with its own step results.

**See also:** [Data-driven tests](tests.md#data-driven-tests), [Iteration](#iteration), [Test data](#test-data)

### Database

Where an application keeps its data. Qtestframe keeps everything, uploaded files included, in one relational database.

**In Qtestframe:** SQLite (a single file, no database server, the default) or PostgreSQL (for larger installations and several server instances). Users see no difference; the administrator chooses at installation.

**See also:** [Where your data lives](how-it-works.md#where-your-data-lives), [Backup](#backup), [SQLite or PostgreSQL](../../admin-guide/en/installation.md#sqlite-or-postgresql)

### Decision table testing

A test design technique for rules that combine several conditions. Each column of the table is one combination of conditions with the expected actions, and each column becomes at least one test case. It makes missing or contradicting rules visible (ISO/IEC/IEEE 29119-4).

**In Qtestframe:** paste the decision table as [test data](#test-data): one row per rule, one parameter per condition and expected outcome.

**See also:** [Test design technique](#test-design-technique), [Pairwise testing](#pairwise-testing), [Data-driven testing](#data-driven-testing)

### Defect

A flaw in a work product that can make it fail to meet a requirement: in code, in a document, in a configuration. A failure is what you observe when a defect is executed; an error (mistake) is the human action that caused it (ISTQB Glossary). In daily speech defect, bug and fault are used for the same thing.

**In Qtestframe:** defects have a status, [severity](#severity), [priority](#priority) and the phase they were found in, and are linked to requirements, tests and executions. Open defects can block requirements and releases, and they can be pushed to Jira, GitHub or GitLab.

**See also:** [Defects](defects.md), [Defect lifecycle](#defect-lifecycle), [Issue tracker](#issue-tracker)

### Defect lifecycle

The states a defect passes through from report to closure, and who moves it between them. A typical cycle is reported → in progress → resolved (fixed) → verified and closed, with rejection for duplicates and non-defects and reopening when the fix does not hold.

**In Qtestframe:** the statuses are *Open*, *In progress*, *Resolved*, *Closed* and *Rejected*; *Open* and *In progress* count as open. A defect that reappears after it was resolved is reported as a regression.

**See also:** [Statuses and severities](defects.md#statuses-and-severities), [Regressions](defects.md#regressions), [Defect](#defect)

### Derived requirement

A requirement that is not stated by a stakeholder but follows from another requirement or from a design decision. A response time for the whole system, for example, leads to derived time budgets for each component.

**In Qtestframe:** use the relation **derived from** / **source of**. A change of the source makes the derived requirement suspect, so it is reviewed.

**See also:** [Refinement](#refinement), [Relations](requirements.md#relations), [Change impact](#change-impact)

### Deviation

An accepted departure from a requirement in which the requirement is met in a different way than specified, usually with compensating measures. Unlike a [waiver](#waiver), the intent of the requirement is still achieved.

**In Qtestframe:** a deviation is a waiver of type *Deviation*, with the solution chosen instead, signed approval and an optional expiry date.

**See also:** [Waiver](#waiver), [Waivers and deviations](waivers-and-soa.md#waivers-and-deviations), [Residual risk](#residual-risk)

### Discussion

A thread of comments attached to an item, where people ask questions, propose changes and decide.

**In Qtestframe:** requirements, tests, runs, defects, risks and review requests have discussions with Markdown, @mentions, reactions and resolving; the project's **Discussions** page collects the threads.

**See also:** [Discussions](discussions.md), [Markdown](#markdown), [Notification](#notification)

## E

### EARS

The Easy Approach to Requirements Syntax: a small set of sentence patterns that make requirements less ambiguous. The patterns are *ubiquitous* (The system shall …), *event-driven* (When …, the system shall …), *state-driven* (While …, the system shall …), *unwanted behaviour* (If …, then the system shall …) and *optional feature* (Where …, the system shall …), and combinations of them (Mavin et al., 2009).

**In Qtestframe:** write the requirement text in an EARS pattern with an obligation keyword; the [language analysis](#language-analysis) still checks it for vague terms and escape clauses.

**See also:** [INCOSE rules](#incose-rules), [Requirement quality](#requirement-quality), [Requirement engineering standards](references.md#requirement-engineering-standards-and-guides)

### End-to-end testing

Testing a complete flow through the whole system and its neighbours, as a user would go through it: from sign-in over ordering to the confirmation e-mail. End-to-end tests find integration problems that narrower tests miss but are slower and more fragile, so they are kept few.

**In Qtestframe:** end-to-end tests are typically [web tests](#web-test) (Playwright) or manual tests with many steps; use [dependencies](tests.md#dependencies) for flows that build on each other.

**See also:** [System testing](#system-testing), [Web test](#web-test), [Flaky test](#flaky-test)

### Entry criteria

The conditions that must hold before a test activity may start: the test basis is approved, the environment and data are ready, the build passed its smoke test. They avoid wasting effort on testing something that cannot pass yet (ISO/IEC/IEEE 29119-3).

**In Qtestframe:** the execution rules of your organisation are entry criteria per test: the test must be *Ready*, its requirements approved and its prerequisites passed. They appear as entry criteria in a generated test plan.

**See also:** [When can a test be executed?](runs.md#when-can-a-test-be-executed), [Exit criteria](#exit-criteria), [Test plan](#test-plan)

### Equivalence partitioning

A test design technique that divides the possible values of an input or output into groups (partitions) that the system should treat alike, then tests one value from each group. Partitions of invalid values are tested one at a time, so that one error does not hide another (ISO/IEC/IEEE 29119-4).

**In Qtestframe:** one row of [test data](#test-data) per partition keeps the cases together in one test; name each row after its partition.

**See also:** [Boundary value analysis](#boundary-value-analysis), [Test design technique](#test-design-technique), [Black-box testing](#black-box-testing)

### Error guessing

A test design technique in which the tester uses experience of typical mistakes to choose tests: empty input, very long input, special characters, time zones, double clicks, interrupted connections. It works best as a complement to systematic techniques, often guided by a list of past defects.

**In Qtestframe:** past defects (filter `found_in:production`) and [exploratory sessions](#exploratory-testing) are good sources; keep the resulting tests in a category of their own.

**See also:** [Exploratory testing](#exploratory-testing), [Test design technique](#test-design-technique), [Defect](#defect)

### ETag and If-Match

HTTP headers for optimistic concurrency (RFC 9110). The server sends a version tag (`ETag`) with a resource; a client that changes the resource sends the tag back in `If-Match`, and the server refuses the change with *412 Precondition Failed* if someone saved a newer version in the meantime. Nobody's work is overwritten silently, and nothing is locked.

**In Qtestframe:** the ETag of an item is its version. The web client sends it when you save, which is why you see **Someone else changed this** instead of losing a colleague's changes; OSLC and API clients use the same mechanism.

**See also:** [When someone else saved first](requirements.md#when-someone-else-saved-first), [HTTP semantics](#http-semantics), [Connecting other tools](../../admin-guide/en/connecting-tools.md)

### EU AI Act

Regulation (EU) 2024/1689 on artificial intelligence. It sorts AI systems by risk and sets requirements for high-risk systems, such as risk management, data governance, technical documentation, logging, human oversight, accuracy and robustness, and transparency obligations for some others.

**In Qtestframe:** the catalogue has AI Act requirements, tests and risks (the risks also cite MITRE ATLAS techniques); choose the AI Act under **Comply with laws, standards and best practices**.

**See also:** [MITRE ATLAS](#mitre-atlas), [Cyber Resilience Act](#cyber-resilience-act), [Catalogue](#catalogue)

### Evaluation criterion

A statement of how the fulfilment of a requirement will be judged: what is checked, by which [method](#verification-method), and the threshold that must be reached, such as "p95 response time below 2 s, measured under 200 parallel users".

**In Qtestframe:** each requirement can have several criteria, each with a method (test, inspection, analysis, demonstration, review) and a threshold. Tests are linked to a criterion, other methods record [verification records](#verification-record); the requirement is verified when every criterion is.

**See also:** [Evaluation criteria](requirements.md#evaluation-criteria), [Acceptance criteria](#acceptance-criteria), [Verification](traceability-and-coverage.md#verification)

### Exit criteria

The conditions that must hold to end a test activity or to release: a share of requirements covered and passed, no open critical defects, risks mitigated. They make "are we done?" a check instead of a feeling (ISO/IEC/IEEE 29119-3, ISTQB Glossary).

**In Qtestframe:** a [release](#release) has exit criteria such as *Coverage*, *Passed*, *Mandatory passed*, *No open defects*, *No suspect links*, *Risks mitigated* or *Residual risk*, and the **Status** tab shows go or no-go.

**See also:** [Exit criteria](releases-and-risks.md#exit-criteria), [Entry criteria](#entry-criteria), [Waiver](#waiver)

### Exploratory testing

Test design and test execution at the same time: the tester learns about the product while testing it and chooses the next test from what they just saw. It finds problems that nobody thought to write a test for, and is most effective when it is structured with a goal and a time box.

**In Qtestframe:** exploratory testing is done in sessions with a charter, a timer, typed notes and a debrief; see [session-based test management](#session-based-test-management).

**See also:** [Exploratory testing](exploratory-testing.md), [Session-based test management](#session-based-test-management), [Error guessing](#error-guessing)

## F

### Flaky test

A test whose result changes between passed and failed without any change in the test or the product: because of timing, shared test data, order dependencies or an unstable environment. Flaky tests hide real failures and erode trust in the whole suite.

**In Qtestframe:** a test with 2 or more flips among its last 10 passed or failed results gets a **Flaky** badge; `flaky:true` and `flakiness>=3` find them, and the dashboard lists the flakiest.

**See also:** [Flaky tests](tests.md#flaky-tests), [Continuous integration](#continuous-integration), [Test result](#test-result)

### Forward traceability

Tracing from a requirement to everything that realises and verifies it: lower-level requirements, design, code, tests and results. It answers "is every requirement implemented and tested?" and "what is affected if this changes?".

**In Qtestframe:** the **Linked tests** of a requirement, its children (*refined by*), its code links and the coverage column show forward traceability; `-has:tests` finds requirements that have none.

**See also:** [Backward traceability](#backward-traceability), [Change impact](#change-impact), [Traceability matrix](#traceability-matrix)

### Functional requirement

A requirement that states what the system does: a function, a behaviour, a reaction to an input, a calculation. "The system shall send an order confirmation by e-mail" is functional; how fast it does so is [non-functional](#non-functional-requirement).

**In Qtestframe:** choose the type **Functional** (the default). Functional requirements are usually verified by tests.

**See also:** [Non-functional requirement](#non-functional-requirement), [Functional testing](#functional-testing), [Fields](requirements.md#fields)

### Functional testing

Testing what the system does against its functional requirements, regardless of how it is built. It can be done at every [test level](#test-level) and is mostly [black-box](#black-box-testing).

**In Qtestframe:** functional tests are the manual and automated tests linked to functional requirements; filter them with `linked:` and the requirement type, or with a category.

**See also:** [Non-functional testing](#non-functional-testing), [Test type](#test-type), [Functional requirement](#functional-requirement)

## G

### Grey-box testing

Testing with partial knowledge of the inside: the tester designs black-box tests but uses knowledge of the architecture, the database or the interfaces to choose them better, for example to check that an order is stored correctly after it was placed in the user interface.

**In Qtestframe:** grey-box tests often combine manual steps with an inspection of logs or data; record what was checked as notes and evidence in the execution.

**See also:** [Black-box testing](#black-box-testing), [White-box testing](#white-box-testing), [Evidence](runs.md#evidence)

## H

### HOTP

The HMAC-based One-Time Password algorithm (RFC 4226): a code computed from a shared secret and a counter. [TOTP](#totp) is HOTP with the current time step as the counter.

**In Qtestframe:** used only inside TOTP; the length of the stored secret follows the recommendation of RFC 4226.

**See also:** [TOTP](#totp), [Multi-factor authentication](#multi-factor-authentication), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

### HTTP semantics

RFC 9110 defines what HTTP methods, status codes and headers mean, independently of the HTTP version: that GET only reads, that PUT replaces, what 404 or 412 mean, and conditional requests with [ETag and If-Match](#etag-and-if-match).

**In Qtestframe:** the REST API, OSLC and SCIM follow it; error answers carry a status code and a machine-readable error code.

**See also:** [ETag and If-Match](#etag-and-if-match), [OpenAPI](#openapi), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

## I

### INCOSE rules

The *Guide to Writing Requirements* of the International Council on Systems Engineering: rules for well-formed requirements, such as active voice with a clear subject, one thought per requirement, no vague terms, no escape clauses, no open-ended lists, defined units and tolerances.

**In Qtestframe:** the [language analysis](#language-analysis) checks several of these rules (ambiguous terms, escape clauses, several obligations in one sentence, passive capability, placeholders) and gives the text a score.

**See also:** [Requirement quality](#requirement-quality), [EARS](#ears), [The language analysis](requirements.md#the-language-analysis)

### Integration testing

The test level that checks the interaction between components or systems: interfaces, data passed between them, protocols and timing. Component integration testing looks inside one system, system integration testing at its connections to other systems.

**In Qtestframe:** link integration tests to *Interface* requirements and allocate both to [system elements](#system-element) to see coverage per component.

**See also:** [Test level](#test-level), [Unit testing](#unit-testing), [System testing](#system-testing)

### ISO/IEC 25010

The product quality model of the SQuaRE series. It divides software quality into characteristics such as functional suitability, performance efficiency, compatibility, interaction capability (usability), reliability, security, maintainability, flexibility (portability) and safety, each with sub-characteristics.

**In Qtestframe:** use the characteristics as a checklist for [non-functional requirements](#non-functional-requirement) and as a category dimension (for example *Quality / Security*); the requirement types cover the most common ones.

**See also:** [Quality attribute](#quality-attribute), [Non-functional testing](#non-functional-testing), [Requirement engineering standards](references.md#requirement-engineering-standards-and-guides)

### ISO/IEC 27001 Annex A

ISO/IEC 27001 specifies an information security management system. Its Annex A lists 93 controls in four themes (organisational, people, physical, technological; 2022 edition). An organisation decides which controls apply and records this in a [Statement of Applicability](#statement-of-applicability).

**In Qtestframe:** the **SoA** page lists the Annex A controls with applicability, justification, owner, the requirements that cite each control and their evidence; the catalogue has ISO/IEC 27001 requirements to add.

**See also:** [Statement of Applicability](waivers-and-soa.md#statement-of-applicability), [Catalogue](#catalogue), [Sources cited in the catalogue](references.md#sources-cited-in-the-catalogue)

### ISO/IEC/IEEE 29119

The international series of standards on software testing: concepts and vocabulary (part 1), test processes (part 2), test documentation such as test plans, test case specifications and test reports (part 3) and test design techniques (part 4).

**In Qtestframe:** the generated test plan follows the structure of part 3, and this glossary uses the vocabulary of parts 1 and 4.

**See also:** [ISTQB](#istqb), [Test plan](#test-plan), [Testing standards and guides](references.md#testing-standards-and-guides)

### ISO/IEC/IEEE 29148

The international standard for requirements engineering: processes, the information items (stakeholder, system and software requirements specifications), and the characteristics of good individual requirements and of requirement sets.

**In Qtestframe:** the generated requirements specification is modelled on it, and the [requirement quality](#requirement-quality) entry summarises its characteristics.

**See also:** [Requirement quality](#requirement-quality), [Specification](#specification), [Requirement engineering standards](references.md#requirement-engineering-standards-and-guides)

### Issue tracker

A tool where development teams manage bugs and tasks, such as Jira, GitHub Issues or GitLab.

**In Qtestframe:** a project can push defects and requirements to Jira, GitHub, GitLab or Request Tracker, synchronise their status and import issues; a changed requirement marks its issue as out of date.

**See also:** [Connect Jira, GitHub or GitLab](defects.md#connect-jira-github-or-gitlab), [Integrations](how-it-works.md#integrations), [Defect](#defect)

### ISTQB

The International Software Testing Qualifications Board publishes the certified tester syllabi and the ISTQB Glossary, the most widely used vocabulary of testing.

**In Qtestframe:** this guide uses the ISTQB terms (test case, test run, defect, coverage …) wherever the user interface does.

**See also:** [ISO/IEC/IEEE 29119](#isoiecieee-29119), [Testing standards and guides](references.md#testing-standards-and-guides)

### Iteration

One pass through a data-driven test with one row of test data.

**In Qtestframe:** each row of a test's test data becomes an iteration of its execution, with its own step results; the execution's result is derived from its iterations.

**See also:** [Data-driven tests: iterations](runs.md#data-driven-tests-iterations), [Data-driven testing](#data-driven-testing), [Test execution](#test-execution)

## J

### JSON Web Token

JWT (RFC 7519): a compact, signed set of claims, such as who a user is, who issued the token and when it expires, that a receiver can verify without asking the issuer.

**In Qtestframe:** the ID token of [OpenID Connect](#openid-connect) single sign-on is a JWT; Qtestframe checks its signature, issuer, audience, expiry and nonce. API tokens are not JWTs but random strings.

**See also:** [OpenID Connect](#openid-connect), [OAuth 2.0](#oauth-20), [API token](#api-token)

### JUnit XML

An XML format for test results, named after the Java framework JUnit, with test suites, test cases, durations, failures, errors and skipped tests. There is no formal standard, but almost every test tool can write it, which makes it the common language between CI and test management.

**In Qtestframe:** upload JUnit XML to a run (`POST /api/v1/runs/{id}/junit`) or with [`qtestframe report`](#qtestframe-report); test cases are matched to Qtestframe tests by their test key.

**See also:** [JUnit XML](runs.md#junit-xml), [Continuous integration](#continuous-integration), [Formats, protocols and frameworks](references.md#formats-protocols-and-frameworks)

## K

### Key

A short, readable, unique identifier.

**In Qtestframe:** every project has a key such as `SHOP`, and every item a key made from it: `SHOP-REQ-12`, `SHOP-TC-7`, `SHOP-RISK-3`. Keys are never reused within a project. Mention a key in a commit message, a description or a CI test name to link to the item.

**See also:** [Project](#project), [Code link](#code-link), [Key concepts](index.md#key-concepts)

## L

### Landlock

A Linux security module with which a program restricts its own access to files: after it has opened what it needs, it declares the directories it may still read and write, and the kernel refuses everything else, even if the program is later taken over.

**In Qtestframe:** on Linux the server restricts itself with Landlock after start-up, and every sandboxed script runs with a Landlock rule set that allows writing only in its job directory.

**See also:** [seccomp](#seccomp), [Script runner](#script-runner), [Hardening](../../admin-guide/en/security.md#hardening)

### Language analysis

An automatic review of requirement text against writing rules.

**In Qtestframe:** while you type, the analysis detects the [obligation level](#obligation-level) from the keywords, highlights them, lists findings such as vague terms, escape clauses, placeholders or several obligations in one sentence, and gives a score from 0 to 100, in English and Swedish.

**See also:** [The language analysis](requirements.md#the-language-analysis), [INCOSE rules](#incose-rules), [Requirement key words](#requirement-key-words)

### Load testing

A kind of [performance testing](#performance-testing) that checks behaviour under the expected load, such as the number of users or transactions at peak time, to confirm that response times and throughput meet their requirements.

**In Qtestframe:** state the load and the limits in the evaluation criterion (method *Analysis* or *Test*, threshold *p95 < 2 s at 500 users*) and attach the load test report as evidence.

**See also:** [Performance testing](#performance-testing), [Stress testing](#stress-testing), [Evaluation criterion](#evaluation-criterion)

## M

### Markdown

A light text format in which `**bold**`, lists, links and tables are written with plain characters. CommonMark is a precise specification of Markdown; GitHub Flavored Markdown adds tables and more.

**In Qtestframe:** comments in discussions are written in Markdown, this guide is Markdown, and the export can produce a Markdown document; the dependency graph can be saved as Markdown with a Mermaid diagram.

**See also:** [Discussion](#discussion), [Export](reports-and-export.md#export), [Formats, protocols and frameworks](references.md#formats-protocols-and-frameworks)

### MITRE ATLAS

The Adversarial Threat Landscape for Artificial-Intelligence Systems: a knowledge base of tactics and techniques of attacks on AI and machine learning systems, such as prompt injection or poisoning of training data, in the style of ATT&CK.

**In Qtestframe:** AI risks in the catalogue list the ATLAS techniques that realise them (`AML.T0051`), next to the ATT&CK techniques.

**See also:** [MITRE ATT&CK](#mitre-attck), [EU AI Act](#eu-ai-act), [MITRE ATT&CK in the catalogue](catalogue.md#mitre-attck)

### MITRE ATT&CK

A knowledge base of how real attackers work: tactics (their goals, such as initial access or credential access) and techniques (how they reach them, such as *T1110 Brute Force*), in matrices for enterprise IT, industrial control systems and mobile devices.

**In Qtestframe:** catalogue risks caused by an attacker list their ATT&CK techniques; filter by tactic or technique, see the framework coverage, and keep the techniques on risks you add to a project.

**See also:** [MITRE D3FEND](#mitre-d3fend), [STIX](#stix), [MITRE ATT&CK in the catalogue](catalogue.md#mitre-attck)

### MITRE D3FEND

A knowledge graph of defensive cybersecurity techniques, grouped by tactics such as *Harden*, *Detect* and *Isolate*, and related to the ATT&CK techniques they counter.

**In Qtestframe:** requirements and tests list the D3FEND techniques they implement or verify (field **MITRE D3FEND techniques**, query `d3fend:`); for a risk, the catalogue shows the defences that counter its techniques and marks gaps.

**See also:** [MITRE ATT&CK](#mitre-attck), [MITRE EMB3D](#mitre-emb3d), [MITRE D3FEND and EMB3D](catalogue.md#mitre-d3fend-and-emb3d)

### MITRE EMB3D

A threat model for embedded devices: threats (*TID-…*) such as inadequate bootloader protection, and mitigations (*MID-…*) such as secure firmware update.

**In Qtestframe:** device and IoT requirements and tests list the EMB3D mitigations and threats they address (query `emb3d:`), and the catalogue shows which threats no requirement addresses.

**See also:** [MITRE D3FEND](#mitre-d3fend), [STIX](#stix), [MITRE D3FEND and EMB3D](catalogue.md#mitre-d3fend-and-emb3d)

### Multi-factor authentication

Signing in with more than one kind of proof: something you know (a password) and something you have (a phone with an authenticator app, a security key). A stolen password alone is then not enough.

**In Qtestframe:** the second factor is a [TOTP](#totp) code with ten single-use recovery codes. Server and organisation policies can require it for everyone or for some roles; with single sign-on your identity provider usually handles it.

**See also:** [Multi-factor authentication](getting-started.md#multi-factor-authentication), [TOTP](#totp), [WebAuthn and FIDO2](#webauthn-and-fido2)

## N

### Non-functional requirement

A requirement on how well the system does what it does, or under which conditions: performance, security, usability, reliability, maintainability, portability. Non-functional requirements need measurable thresholds, otherwise they cannot be verified ("fast" is not a requirement; "p95 below 2 s" is).

**In Qtestframe:** choose the type **Non-functional** or the more specific **Security**, **Performance** or **Usability**, and put the measurable threshold in an [evaluation criterion](#evaluation-criterion).

**See also:** [Quality attribute](#quality-attribute), [Functional requirement](#functional-requirement), [Non-functional testing](#non-functional-testing)

### Non-functional testing

Testing quality characteristics rather than functions: performance, load, security, usability, accessibility, reliability, compatibility. It often needs special tools, environments and measurements.

**In Qtestframe:** non-functional tests are linked to non-functional requirements like any other test; analyses and measurements that are not test executions are recorded as [verification records](#verification-record).

**See also:** [Functional testing](#functional-testing), [ISO/IEC 25010](#isoiec-25010), [Test type](#test-type)

### Notification

A message that tells you something happened that concerns you: a review request, a mention, an assignment, a result.

**In Qtestframe:** notifications appear in the app and, if you want, by e-mail at once or as a daily digest. Projects can also post events to chat channels (Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix, Discord) and to [webhooks](#webhook).

**See also:** [Notifications](reviews-and-notifications.md#notifications), [Chat notifications](reviews-and-notifications.md#chat-notifications), [Webhook](#webhook)

## O

### OAuth 2.0

The authorisation framework of RFC 6749: an application obtains an access token from an authorisation server on behalf of a user, without seeing the user's password. It is the basis of [OpenID Connect](#openid-connect).

**In Qtestframe:** used for single sign-on through OpenID Connect, with the authorisation code flow and [PKCE](#pkce). Qtestframe itself is not an OAuth authorisation server; its API uses [API tokens](#api-token).

**See also:** [OpenID Connect](#openid-connect), [PKCE](#pkce), [Bearer token](#bearer-token)

### Obligation level

How binding a requirement is: mandatory, recommended, optional, or merely descriptive. It is expressed by keywords such as MUST, SHOULD and MAY and decides how a requirement is weighed in release decisions.

**In Qtestframe:** the levels are *Must*, *Shall*, *Must not*, *Shall not*, *Should*, *Should not*, *May*, *Optional*, *Will* and *Describe*. They are detected from the text when you create a requirement, highlighted in descriptions, and the exit criterion *Mandatory passed* checks every MUST and SHALL requirement.

**See also:** [Obligation levels](requirements.md#obligation-levels), [Requirement key words](#requirement-key-words), [SKA, BÖR and FÅR](#ska-bör-and-får)

### OpenAPI

A machine-readable description format for HTTP APIs: endpoints, parameters, request and response bodies, authentication. Tools generate documentation, clients and tests from it.

**In Qtestframe:** the server publishes its REST API as OpenAPI 3.1 at `/api/v1/openapi.json`, without sign-in, so you can generate a client for your scripts.

**See also:** [HTTP semantics](#http-semantics), [API token](#api-token), [Overview](../../admin-guide/en/overview.md)

### OpenID Connect

OIDC: an identity layer on top of [OAuth 2.0](#oauth-20). After the user signs in at the identity provider (such as Microsoft Entra ID, Okta or Keycloak), the application receives an ID token ([JWT](#json-web-token)) that says who the user is.

**In Qtestframe:** single sign-on with one or more OIDC providers, with PKCE and nonce; users can be created at first sign-in and get roles from groups.

**See also:** [Single sign-on (OIDC)](administration.md#single-sign-on-oidc), [SAML 2.0](#saml-20), [OpenID Connect](../../admin-guide/en/authentication.md#openid-connect)

### Organisation

An isolated workspace in a shared installation (a tenant). Organisations do not see each other's data.

**In Qtestframe:** projects, members, roles, API tokens, settings and the audit log belong to an organisation. You can be a member of several and switch between them; your role can differ in each.

**See also:** [Organisations, projects and roles](how-it-works.md#organisations-projects-and-roles), [Role and permission](#role-and-permission), [Project](#project)

### OSLC

Open Services for Lifecycle Collaboration: an OASIS open standard for linking items across lifecycle tools over HTTP, with domains for requirements management (RM) and quality management (QM). Tools discover each other's services, pick or create items in small dialogs, and store links instead of copies.

**In Qtestframe:** Qtestframe is an OSLC provider for RM 2.1 (requirements) and QM 2.1 (test cases): DOORS Next, Polarion, Codebeamer and other tools can pick, create, read and change items and link to them.

**See also:** [OSLC](interoperability.md#oslc), [ReqIF](#reqif), [Connecting other tools](../../admin-guide/en/connecting-tools.md)

## P

### Pairwise testing

A combinatorial test design technique: instead of testing every combination of parameter values, choose a much smaller set of tests in which every pair of values occurs at least once. Most interaction faults involve two parameters, so few tests find most of them (NIST SP 800-142).

**In Qtestframe:** generate the pairwise combinations with a tool and paste them as [test data](#test-data), or use them to choose the [test configurations](#test-configuration) of a run.

**See also:** [Test configuration](#test-configuration), [Decision table testing](#decision-table-testing), [NIST publications](references.md#nist-publications)

### Performance testing

Testing response times, throughput and resource use under a given load. [Load testing](#load-testing) checks the expected load, [stress testing](#stress-testing) goes beyond it, and endurance (soak) testing keeps a load for a long time to find leaks.

**In Qtestframe:** link performance tests to *Performance* requirements with thresholds; record measurement reports as evidence or as analysis records.

**See also:** [Load testing](#load-testing), [Stress testing](#stress-testing), [Non-functional testing](#non-functional-testing)

### PKCE

Proof Key for Code Exchange (RFC 7636): an extension of the OAuth authorisation code flow. The client sends a hashed random value with the sign-in request and the value itself when it exchanges the code, so an intercepted code is useless to anyone else.

**In Qtestframe:** OIDC sign-in always uses PKCE with the S256 method, and in addition a nonce in the ID token.

**See also:** [OAuth 2.0](#oauth-20), [OpenID Connect](#openid-connect), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

### Priority

How soon something should be dealt with, as a business decision. A cosmetic defect on the start page can have high priority; a crash in a rarely used admin function can have low priority. Priority is different from [severity](#severity).

**In Qtestframe:** requirements, tests and defects have a priority (*Critical*, *High*, *Medium*, *Low*). It orders work lists and filters, and chat channels can post only items of a minimum priority.

**See also:** [Severity](#severity), [Defect](#defect), [Statuses and severities](defects.md#statuses-and-severities)

### Problem Details

RFC 9457 defines a standard JSON format for error responses of HTTP APIs (`application/problem+json`) with the members type, title, status and detail.

**In Qtestframe:** the catalogue has API requirements and tests that cite RFC 9457 for systems you build. Qtestframe's own API answers errors with its own JSON error object and code.

**See also:** [HTTP semantics](#http-semantics), [OpenAPI](#openapi), [Catalogue](#catalogue)

### Project

A container for one product, system or delivery.

**In Qtestframe:** a project holds requirements, tests, runs, cycles, sessions, defects, risks, releases, baselines, documents and its settings. Its key prefixes all item keys. Projects can be restricted to the members who work on them, with a project role of their own, and saved as a [project archive](#project-archive) to be loaded elsewhere.

**See also:** [The items and how they relate](how-it-works.md#the-items-and-how-they-relate), [Organisation](#organisation), [Key](#key)

### Project archive

A single file that contains a whole project, for handing it over, keeping a copy or starting a new project from it.

**In Qtestframe:** **Save project** writes a ZIP with everything in the project, including history, results and attachments, but never secrets; **Load project** creates a new project from it, on the same or another server, with new IDs and keys.

**See also:** [Save and load projects](reports-and-export.md#save-and-load-projects), [Backup](#backup), [Project](#project)

## Q

### qtestframe report

The command of the `qtestframe` program that uploads test results from a CI pipeline.

**In Qtestframe:** `qtestframe report --project SHOP --run "Nightly 412" --create-run --junit 'results/*.xml'` finds or creates the run, uploads JUnit XML and records build and environment; it authenticates with an [API token](#api-token) in `QTF_TOKEN`.

**See also:** [The qtestframe report command](runs.md#the-qtestframe-report-command), [JUnit XML](#junit-xml), [Continuous integration](#continuous-integration)

### Quality attribute

A measurable property of a system other than its functions, such as response time, availability, learnability or resistance to attack. [ISO/IEC 25010](#isoiec-25010) organises them into a quality model.

**In Qtestframe:** quality attributes are stated as non-functional requirements with an evaluation criterion and threshold; a category dimension such as *Quality* groups them.

**See also:** [Non-functional requirement](#non-functional-requirement), [ISO/IEC 25010](#isoiec-25010), [Evaluation criterion](#evaluation-criterion)

### Query language

A small text language for filtering lists.

**In Qtestframe:** the same language filters every list, the traceability matrix, release scopes, reports and the API: `status:approved priority=high`, `-has:tests`, `updated_at>=2026-01-01`, `title~/login/i`. An invalid query shows where the problem is.

**See also:** [Search and filters](search-and-filters.md), [The query language](how-it-works.md#the-query-language)

## R

### Refinement

Breaking a requirement down into more detailed requirements that together fulfil it: a stakeholder need into system requirements, a system requirement into software requirements. The detailed requirements refine the general one.

**In Qtestframe:** the relation **refines (parent)** / **refined by (child)** builds the requirement hierarchy; refines relations may not form a cycle, and a parent is verified only when its children are.

**See also:** [Requirement hierarchy and roll-up](traceability-and-coverage.md#requirement-hierarchy-and-roll-up), [Derived requirement](#derived-requirement), [Relations](requirements.md#relations)

### Regression testing

Testing again what worked before, after a change, to find out whether the change broke it. Regression suites grow over time, so they are usually automated and selected by risk and by what changed.

**In Qtestframe:** the **Regression selection** proposes tests from changed requirements and from commits since a build; a defect that reappears after being resolved is flagged as a regression.

**See also:** [Regression selection](variants-and-code.md#regression-selection), [Smoke and sanity testing](#smoke-and-sanity-testing), [Continuous integration](#continuous-integration)

### Release

A version of the product that is planned, tested and delivered as a whole.

**In Qtestframe:** a release has a scope (the requirements it must deliver), the runs that test it, [exit criteria](#exit-criteria) and a go/no-go status; waivers can accept requirements for one release, and a baseline usually freezes it.

**See also:** [Releases and risks](releases-and-risks.md), [Exit criteria](#exit-criteria), [Baseline](#baseline)

### ReqIF

The Requirements Interchange Format of the Object Management Group (OMG): an XML format for exchanging requirement specifications between tools, with their attributes, hierarchy and relations. A `.reqifz` file is a ZIP with the ReqIF file and the attached files.

**In Qtestframe:** export requirements (optionally with tests, trace links, verification records or a baseline) as ReqIF 1.2 and import ReqIF from DOORS, Polarion, Codebeamer, Jama or ProR, also as a round trip.

**See also:** [ReqIF export](interoperability.md#reqif-export), [ReqIF import](interoperability.md#reqif-import), [OSLC](#oslc)

### Requirement

A statement of a need, capability or condition that a system must meet, written so that it can be agreed on and verified. A requirement says what is needed, not how to build it, and carries attributes such as priority, source and rationale.

**In Qtestframe:** a requirement has a key, title, description, [obligation level](#obligation-level), type, status, priority, owner, source, rationale, [evaluation criteria](#evaluation-criterion), relations and a version history; its coverage comes from its linked tests.

**See also:** [Requirements](requirements.md), [Specification](#specification), [Requirement quality](#requirement-quality)

### Requirement coverage

The share of requirements that tests reach, and how far their results show the requirements to be met. It is the main coverage measure of test management, because it ties testing to what the product must do.

**In Qtestframe:** each requirement has a coverage state (*None*, *Not run*, *Passed*, *Failed*, *Partial*) from the latest results of its tests, per configuration; the dashboard and the release criteria *Coverage* and *Passed* add it up.

**See also:** [How coverage is calculated](traceability-and-coverage.md#how-coverage-is-calculated), [Coverage](#coverage), [Code coverage](#code-coverage)

### Requirement key words

The words that mark how binding a sentence is. RFC 2119 defines MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, RECOMMENDED, NOT RECOMMENDED, MAY and OPTIONAL; RFC 8174 adds that only the upper-case forms carry this meaning. Together they form BCP 14.

**In Qtestframe:** the keywords set the [obligation level](#obligation-level) and are highlighted; the language analysis flags lower-case keywords and several keywords in one sentence.

**See also:** [Requirement key words (RFCs)](references.md#requirement-key-words-rfcs), [SKA, BÖR and FÅR](#ska-bör-and-får), [Obligation level](#obligation-level)

### Requirement quality

The properties that make a requirement good. ISO/IEC/IEEE 29148 asks of each requirement that it is necessary, appropriate (at the right level), unambiguous, complete, singular, feasible, verifiable, correct and conforming, and of a set that it is complete, consistent, feasible, comprehensible and able to be validated.

**In Qtestframe:** the [language analysis](#language-analysis) scores each text, the consistency checks find duplicates, conflicts and gaps, and the analytics show quality metrics for the whole project.

**See also:** [INCOSE rules](#incose-rules), [Requirement analytics](traceability-and-coverage.md#requirement-analytics), [ISO/IEC/IEEE 29148](#isoiecieee-29148)

### Residual risk

The risk that remains after the mitigations are in place. It must be at or below what the organisation is willing to accept, or someone with the authority must accept it explicitly.

**In Qtestframe:** a risk has a residual likelihood and impact; above the project's risk acceptance threshold, its acceptance must be signed (`waiver.approve`), and the exit criterion *Residual risk* checks it.

**See also:** [Residual risk](waivers-and-soa.md#residual-risk), [Risk](#risk), [Waiver](#waiver)

### Review

A check of a work product by people other than its author, from an informal look to a formal inspection with roles and a protocol. In Qtestframe "review" also means the approval decision on an item.

**In Qtestframe:** request a review of requirements or tests; reviewers approve or reject requirements (`requirement.review`) and mark tests ready (`test.review`), and every decision is in the audit log. *Review* is also one of the [verification methods](#verification-method).

**See also:** [Request a review](reviews-and-notifications.md#request-a-review), [Role and permission](#role-and-permission), [Verification method](#verification-method)

### Revision

One saved version of one item. Every change creates a new revision, so the history shows who changed what, when, and how.

**In Qtestframe:** the **History** tab compares and restores revisions; the version number in the editor header counts them. A [baseline](#baseline) records a revision of every item at one moment.

**See also:** [History and restore](requirements.md#history-and-restore), [Baseline](#baseline), [Baseline or revision?](baselines.md#baseline-or-revision)

### Risk

Something uncertain that, if it happens, harms the product, the project or its users, rated by likelihood and impact. Product risks concern quality and safety; project risks concern schedule, budget and staff.

**In Qtestframe:** the risk register rates each risk 1–5 for likelihood and impact (score 1–25, level *Low* to *Critical*), lists the requirements that address it and how far they are covered and verified; open risks affect release exit criteria.

**See also:** [The risk register](releases-and-risks.md#the-risk-register), [Risk-based testing](#risk-based-testing), [Residual risk](#residual-risk)

### Risk-based testing

Choosing, ordering and dosing requirements and tests by risk: the higher the likelihood and impact of a failure, the earlier and more thoroughly it is specified and tested. It makes the most of limited test time and explains to stakeholders what was left out and why.

**In Qtestframe:** link requirements to the risks they address, build release scopes with `risk:high`, and use the exit criteria *Risks mitigated* and *Residual risk*. The risk report and heat map show where coverage is missing.

**See also:** [Risk](#risk), [How risks affect a release](releases-and-risks.md#how-risks-affect-a-release), [Exit criteria](#exit-criteria)

### Role and permission

A named set of permissions given to a person.

**In Qtestframe:** every organisation has the roles *Viewer*, *Reviewer*, *Tester*, *Test lead*, *Manager* and *Admin*, and can change them or create its own; a project can give a member a different role. *Super admins* manage the whole installation.

**See also:** [Roles and permissions](administration.md#roles-and-permissions), [Organisation](#organisation), [Authorisation](../../admin-guide/en/authorisation.md)

## S

### SAML 2.0

The Security Assertion Markup Language, an OASIS standard for single sign-on: the identity provider sends the application a signed XML assertion about the user.

**In Qtestframe:** single sign-on with a SAML identity provider such as Active Directory Federation Services (AD FS), as an alternative to OpenID Connect.

**See also:** [OpenID Connect](#openid-connect), [SCIM](#scim), [SAML 2.0](../../admin-guide/en/authentication.md#saml-20)

### SCIM

The System for Cross-domain Identity Management (RFC 7643 for the data model, RFC 7644 for the protocol): a REST interface through which an identity provider creates, changes and removes user accounts and groups in an application automatically.

**In Qtestframe:** `/scim/v2` lets your identity provider provision users and group memberships of an organisation, with its own token; deactivating a user removes the membership and ends the user's sessions and API tokens in that organisation at once.

**See also:** [Provisioning with SCIM](administration.md#provisioning-with-scim), [SAML 2.0](#saml-20), [Other RFCs](references.md#other-rfcs-that-qtestframe-implements)

### Script runner

The part of the server that runs the scripts of automated tests.

**In Qtestframe:** when an administrator turns it on, members with `script.execute` can run a test's script on the server. Each script runs in a sandboxed child process with resource limits, Landlock, seccomp and optionally no network; web tests run Playwright with a real browser.

**See also:** [Automated tests: scripts](tests.md#automated-tests-scripts), [Landlock](#landlock), [The script and web test sandbox](../../admin-guide/en/security.md#the-script-and-web-test-sandbox)

### seccomp

Secure computing mode: a Linux kernel feature with which a process tells the kernel which system calls it will never make, so that they fail even if an attacker takes the process over.

**In Qtestframe:** the server and every sandboxed script install a deny-list of system calls that a server never needs; with `runner.deny_network` scripts also lose the socket calls.

**See also:** [Landlock](#landlock), [Script runner](#script-runner), [Hardening](../../admin-guide/en/security.md#hardening)

### Secrets encryption

Protecting stored passwords, tokens and keys that the server must be able to read back.

**In Qtestframe:** TOTP secrets, webhook secrets, tracker tokens, secret test variables and similar secrets are encrypted in the database with a key that is never stored there: a key file, an environment variable, a TPM 2.0 or an HSM. Secrets are write-only: the API never returns them.

**See also:** [Encryption of stored secrets](administration.md#encryption-of-stored-secrets), [Backup](#backup), [Security](how-it-works.md#security)

### Security testing

Testing whether the system protects data and functions against attacks and misuse: authentication, authorisation, input handling, session management, cryptography, logging. It ranges from checks of single requirements to penetration tests and fuzzing.

**In Qtestframe:** the catalogue has security requirements and tests that cite OWASP ASVS, NIST, CWE and ATT&CK; link findings as defects with severity *Critical* where needed.

**See also:** [CWE](#cwe), [MITRE ATT&CK](#mitre-attck), [Non-functional testing](#non-functional-testing)

### Session

1. A signed-in visit of a user, kept by a cookie until it expires or the user signs out. 2. In exploratory testing, a time-boxed period of testing with a charter (see [session-based test management](#session-based-test-management)).

**In Qtestframe:** sign-in sessions last 12 hours by default; you see and end yours under **Profile → Active sessions**. Exploratory sessions are under **Exploratory testing** in a project.

**See also:** [Why was my session ended?](faq.md#why-was-my-session-ended), [CSRF protection](#csrf-protection), [Session-based test management](#session-based-test-management)

### Session-based test management

SBTM: a way to plan, run and account for [exploratory testing](#exploratory-testing), described by Jonathan and James Bach in 2000. Testing happens in uninterrupted sessions of fixed length, each with a charter (the mission), notes taken while testing, and a debrief with a test lead afterwards; metrics show how session time was spent.

**In Qtestframe:** plan sessions with a charter and scope, run them with a timer and typed notes (bug, issue, question, idea …), raise defects from notes, and debrief with a result; debriefed sessions can count as verification evidence.

**See also:** [Exploratory testing](exploratory-testing.md), [Exploratory testing](#exploratory-testing), [Session](#session)

### Severity

How serious the effect of a defect is on the system or its users, from a crash or data loss to a typo. Severity is a technical judgement; [priority](#priority) is the decision how soon to fix it.

**In Qtestframe:** defects have the severities *Critical*, *Major*, *Minor* and *Trivial*; the exit criterion *No open defects* and the blocking rules use a minimum severity.

**See also:** [Priority](#priority), [Blocking defects](traceability-and-coverage.md#blocking-defects), [Defect](#defect)

### Shared step

A sequence of steps that many tests use, such as signing in, written once and called from other tests.

**In Qtestframe:** a test step can call another test of the project; when a run is created, the call is replaced by the called test's steps, resolved with the parameters of the calling test. A change of the called test reaches every caller in the next run.

**See also:** [Shared steps](tests.md#shared-steps), [Test step](#test-step), [Data-driven testing](#data-driven-testing)

### Shift-left

Moving testing and quality work earlier in the lifecycle: reviewing requirements before design, writing tests before or with the code, running automated tests on every change. Defects found early are much cheaper to fix than those found in production.

**In Qtestframe:** the language analysis and reviews check requirements while they are written, a linked test can be created with each requirement, and CI results arrive with every build.

**See also:** [Continuous integration](#continuous-integration), [Test-driven development](#test-driven-development), [V-model](#v-model)

### SKA, BÖR and FÅR

The Swedish obligation keywords. Swedish standards (SIS, following the ISO/IEC Directives) use *ska* for a requirement, *bör* for a recommendation, *får* for a permission and *kan* for a possibility; negations are *ska inte*, *bör inte* and *får inte*.

**In Qtestframe:** Swedish requirement text uses **SKA**/**SKALL** and **MÅSTE** (must), **BÖR** (should), **FÅR** (may), **FÅR INTE** and **SKA INTE** (must not), **BÖR INTE**, **BEHÖVER INTE** and **VALFRI** (optional), **KOMMER ATT** (will) and **BESKRIVER** (descriptive). The analysis warns about *måste inte*, which means "need not" but is often read as a prohibition.

**See also:** [The language analysis](requirements.md#the-language-analysis), [Requirement key words](#requirement-key-words), [Requirement engineering standards](references.md#requirement-engineering-standards-and-guides)

### Smoke and sanity testing

A smoke test is a small, quick set of tests that checks that a new build works at all (it starts, the main pages open, sign-in works) before more testing is spent on it. A sanity test is a quick, narrow check that a specific fix or change works as intended.

**In Qtestframe:** tag the tests `smoke` and create a run from `tag:smoke`, or let CI run them first; a failed smoke run tells everyone to wait for the next build.

**See also:** [Regression testing](#regression-testing), [Entry criteria](#entry-criteria), [Test run](#test-run)

### Software requirement

A requirement on a software item, derived from system requirements and the architecture: its functions, interfaces, data and quality. Software requirements are what developers implement and what unit and integration tests verify.

**In Qtestframe:** relate software requirements to their system requirements with **refines** or **derived from**, and allocate them to the software components under [system elements](#system-element).

**See also:** [System requirement](#system-requirement), [Refinement](#refinement), [Unit testing](#unit-testing)

### Specification

A document, or a structured set of items, that states requirements, design or behaviour completely and precisely. A requirement is a single statement; a specification is the organised whole, with an introduction, scope, definitions and the requirements themselves.

**In Qtestframe:** the project is the living specification; **Create document → Requirements specification** writes a complete document modelled on ISO/IEC/IEEE 29148 into the archive, and ReqIF exchanges the specification with other tools.

**See also:** [Create a requirements specification or a test plan](documents.md#create-a-requirements-specification-or-a-test-plan), [Requirement](#requirement), [ReqIF](#reqif)

### Stakeholder

A person or organisation with an interest in the system or affected by it: customers, users, operators, regulators, the development team.

**In Qtestframe:** record who asked for a requirement in **Source** (for example *Stakeholder* or *Customer spec §4.2*) and who is responsible in **Owner**.

**See also:** [Stakeholder requirement](#stakeholder-requirement), [Fields](requirements.md#fields), [Requirement](#requirement)

### Stakeholder requirement

A requirement as a stakeholder states it, in their language: what they need from the system and under which conditions. Stakeholder requirements are validated with the stakeholders and transformed into [system requirements](#system-requirement) (ISO/IEC/IEEE 29148).

**In Qtestframe:** keep stakeholder requirements in a folder or category of their own and refine them into system requirements; their verification rolls up from the children.

**See also:** [User requirement](#user-requirement), [Business requirement](#business-requirement), [Validation](#validation)

### Stale result

A test result that may no longer be valid evidence: it is too old, or the test or a requirement it verifies changed after it was recorded.

**In Qtestframe:** stale results have a **Stale** badge, count as not run for coverage and do not verify a requirement; run the test again. The age limit and the rule for changes are project settings.

**See also:** [Stale results](traceability-and-coverage.md#stale-results), [Change impact](#change-impact), [Suspect link](#suspect-link)

### State transition testing

A test design technique for systems whose behaviour depends on their state, such as an order (*created, paid, shipped, cancelled*) or a session. Tests cover the states, the valid transitions and the events that should be refused in a state (ISO/IEC/IEEE 29119-4).

**In Qtestframe:** one test per path or transition with the state in the preconditions; the test flow diagram helps to see the order of steps and dependencies.

**See also:** [Test design technique](#test-design-technique), [Test flow](tests.md#test-flow), [Decision table testing](#decision-table-testing)

### Statement of Applicability

SoA: the document in which an organisation states, for every control of a reference list such as ISO/IEC 27001 Annex A, whether it applies, why or why not, and whether it is implemented.

**In Qtestframe:** the **SoA** page lists the controls with applicability, justification and owner, derives the implementation from the requirements that cite each control and their tests, and the **Statement of Applicability** report prints it.

**See also:** [Statement of Applicability](waivers-and-soa.md#statement-of-applicability), [ISO/IEC 27001 Annex A](#isoiec-27001-annex-a), [Waiver](#waiver)

### STIX

Structured Threat Information Expression: an OASIS JSON format for exchanging cyber threat information, such as attack patterns, threat actors and mitigations, and the relations between them.

**In Qtestframe:** MITRE publishes ATT&CK, ATLAS and EMB3D as STIX 2.1 bundles; the technique, threat and mitigation lists of the catalogue are generated from them.

**See also:** [MITRE ATT&CK](#mitre-attck), [MITRE EMB3D](#mitre-emb3d), [Formats, protocols and frameworks](references.md#formats-protocols-and-frameworks)

### Stress testing

A kind of [performance testing](#performance-testing) beyond the expected load or with reduced resources, to find the breaking point and to see whether the system fails gracefully and recovers.

**In Qtestframe:** state the expected behaviour at overload (for example "refuses new sessions with a message, keeps existing ones") as a requirement with criteria, so that the stress test has something to verify.

**See also:** [Performance testing](#performance-testing), [Load testing](#load-testing), [Non-functional testing](#non-functional-testing)

### Suspect link

A trace link or relation whose one end changed in substance after the link was made or last confirmed, so that the other end may no longer fit. A suspect link must be reviewed and then cleared or the item updated.

**In Qtestframe:** links and relations become suspect automatically when the title, description, obligation or criteria of a requirement, or the content of a test, change; they have an amber frame in the matrix, show in work lists, and the exit criterion *No suspect links* checks them.

**See also:** [Change impact](#change-impact), [The traceability matrix](traceability-and-coverage.md#the-traceability-matrix), [Stale result](#stale-result)

### Syslog

The standard for sending log messages to a central log server. RFC 5424 defines the current message format, RFC 3164 the older BSD format.

**In Qtestframe:** the server can send its log and the audit log to syslog over UDP, TCP, TLS or a Unix socket, in RFC 5424 or RFC 3164 format.

**See also:** [Logging and syslog](administration.md#logging-and-syslog), [Audit log](#audit-log), [Central syslog](../../admin-guide/en/logging-and-monitoring.md#central-syslog)

### System element

A part of the product breakdown: a system, subsystem, component, product or supplier delivery.

**In Qtestframe:** under **Verification → System elements** you build the breakdown as a tree and allocate requirements and tests to elements; each element shows how many of its requirements are verified, and `element:"Power unit"` filters by it.

**See also:** [System elements](traceability-and-coverage.md#system-elements), [Integration testing](#integration-testing), [Software requirement](#software-requirement)

### System requirement

A requirement on the system as a whole, as seen from outside: what it must do and how well, in the language of engineers rather than stakeholders. System requirements are derived from stakeholder requirements and refined into requirements on subsystems, hardware and software.

**In Qtestframe:** relate them upward to stakeholder requirements and downward to software requirements with **refines**; system tests verify them.

**See also:** [Stakeholder requirement](#stakeholder-requirement), [Software requirement](#software-requirement), [System testing](#system-testing)

### System testing

The test level that checks the complete, integrated system against its system requirements, in an environment as close to production as possible. It includes functional and non-functional tests.

**In Qtestframe:** system test runs usually target several [test configurations](#test-configuration) and environments; their results count towards the release exit criteria.

**See also:** [Test level](#test-level), [Acceptance testing](#acceptance-testing), [System requirement](#system-requirement)

## T

### Test case

A set of preconditions, inputs, actions and expected results, designed to check one or more requirements or conditions. A good test case can be repeated by someone else with the same outcome.

**In Qtestframe:** a test (key `…-TC-n`) is manual, with [steps](#test-step), or automated, with a script or a Playwright web test. It has a status (*Draft*, *Ready*, *Review*, *Deprecated*), links to the requirements it verifies, test data and a version history.

**See also:** [Tests](tests.md), [Test step](#test-step), [Test execution](#test-execution)

### Test configuration

A combination of the conditions a test is run under, such as browser, operating system, device or language. Running the same tests in several configurations finds problems that show only in one of them.

**In Qtestframe:** an organisation defines configurations (for example *Chrome / Windows*); a run with three configurations gets one execution per test and configuration, and a requirement passes only when its tests passed in every configuration they are planned in.

**See also:** [Configurations in a run](runs.md#configurations-in-a-run), [Test configurations](administration.md#test-configurations), [Pairwise testing](#pairwise-testing)

### Test cycle

A planned period of testing for one goal, such as a sprint regression or a release acceptance, with its tests, testers, estimates and dates.

**In Qtestframe:** a test cycle holds the plan (which tests, in which configurations, by whom, how long); when it starts it creates its [run](#test-run) and tracks planned against executed, a burndown and each tester's workload. Cycles can recur.

**See also:** [Test cycles](test-cycles.md), [Test run](#test-run), [Test plan](#test-plan)

### Test data

The data a test needs: input values, expected values, accounts, files and the state of databases. Test data must be known, repeatable and safe (no real personal data without reason).

**In Qtestframe:** a test's **Test data** table holds parameters and rows for [data-driven testing](#data-driven-testing); larger data sets and files go into attachments or the document archive, and secret values into secret test variables.

**See also:** [Data-driven tests](tests.md#data-driven-tests), [Data-driven testing](#data-driven-testing), [Test environment](#test-environment)

### Test design technique

A systematic way to derive test cases and test data from a test basis, so that tests are chosen for a reason and their coverage can be stated. ISO/IEC/IEEE 29119-4 groups them into specification-based (black-box), structure-based (white-box) and experience-based techniques.

**In Qtestframe:** the technique is chosen by the author; record it as a category dimension (for example *Technique / Boundary values*) to report on it.

**See also:** [Equivalence partitioning](#equivalence-partitioning), [Boundary value analysis](#boundary-value-analysis), [Error guessing](#error-guessing)

### Test environment

The hardware, software, network, data and tools in which tests are executed, such as *staging* or a lab rack. Results only mean something together with the environment they were obtained in.

**In Qtestframe:** a run records its **Environment** and **Build**; combine with [test configurations](#test-configuration) for browsers or devices.

**See also:** [Create a run](runs.md#create-a-run), [Test configuration](#test-configuration), [Test run](#test-run)

### Test execution

Running a test case and recording what happened: the result, step results, observations, evidence and any defects.

**In Qtestframe:** an execution is one test inside a [run](#test-run) (per configuration), with its result, step results, iterations, notes, attachments, defects and duration. Steps are copied when the test is added, so later edits of the test do not change recorded results.

**See also:** [The execution runner](runs.md#the-execution-runner), [Test run](#test-run), [Test result](#test-result)

### Test level

A group of test activities that are organised together and aimed at one level of the system: [unit](#unit-testing) (component), [integration](#integration-testing), [system](#system-testing) and [acceptance](#acceptance-testing) testing, plus [end-to-end](#end-to-end-testing) testing across systems. Each level has its own test basis, objects, typical defects and responsibilities (ISTQB Glossary).

**In Qtestframe:** there is no fixed level field; use a category dimension such as *Level / System*, folders, or separate runs and cycles per level.

**See also:** [Test type](#test-type), [V-model](#v-model), [Categories](tests.md#categories)

### Test oracle

The source that tells whether a result is correct: the specification, a reference system, a previous version, a calculation by hand, or the judgement of an expert. Without an oracle a test can be run but not evaluated.

**In Qtestframe:** the expected result of each step and the overall expected result of a test are its oracle; for automated tests the assertions in the script are.

**See also:** [Test step](#test-step), [Test case](#test-case), [Manual tests: steps](tests.md#manual-tests-steps)

### Test plan

A document that describes the scope, approach, resources, schedule, entry and exit criteria and risks of testing (ISO/IEC/IEEE 29119-3). It is the agreement on what will be tested, how and by whom.

**In Qtestframe:** **Create document → Test plan** writes a test plan with its test specification from the project: features to be tested, approach with entry and exit criteria, environment, roles, schedule, every test with its steps and a coverage appendix. A [test cycle](#test-cycle) is the operational plan of one testing period.

**See also:** [Create a requirements specification or a test plan](documents.md#create-a-requirements-specification-or-a-test-plan), [Test cycle](#test-cycle), [Exit criteria](#exit-criteria)

### Test result

The outcome of executing a test, also called its verdict.

**In Qtestframe:** an execution is *Passed*, *Failed*, *Blocked* (could not be executed because of an obstacle), *Skipped* (deliberately left out), *In progress* or *Not run*. The latest result of each test decides the coverage of its requirements.

**See also:** [How coverage is calculated](traceability-and-coverage.md#how-coverage-is-calculated), [Test execution](#test-execution), [Stale result](#stale-result)

### Test run

One round of executing a selected set of tests, in an environment and build, often called a test campaign.

**In Qtestframe:** a run (for example *Sprint 12 regression*) holds one [execution](#test-execution) per test (and configuration), shows progress and pass rate, and can belong to a release. Runs are created by hand, by a test cycle, from a query or by CI.

**See also:** [Test runs and execution](runs.md), [Test cycle](#test-cycle), [Test suite](#test-suite)

### Test step

One action within a manual test, with the expected result of that action.

**In Qtestframe:** steps have an action, an expected result and optionally test data placeholders; in the runner each step gets its own result, and a step can call another test as a [shared step](#shared-step).

**See also:** [Manual tests: steps](tests.md#manual-tests-steps), [Test case](#test-case), [Shared step](#shared-step)

### Test suite

A set of test cases grouped for a purpose, such as all tests of one feature or the nightly regression. In many tools a suite is a folder; in JUnit XML it is a group of test cases in a report.

**In Qtestframe:** there is no separate suite object: group tests with folders, categories or tags, save the query, and create a run from it.

**See also:** [Test run](#test-run), [Query language](#query-language), [JUnit XML](#junit-xml)

### Test type

A group of test activities aimed at one kind of quality: [functional](#functional-testing), [non-functional](#non-functional-testing) (performance, security, usability, accessibility …), change-related ([regression](#regression-testing), confirmation testing) and structural testing. Every test type can be performed at every [test level](#test-level).

**In Qtestframe:** use a category dimension (*Type / Security*) or tags; the requirement types and the catalogue categories follow the same division.

**See also:** [Test level](#test-level), [Categories](tests.md#categories), [Non-functional testing](#non-functional-testing)

### Test-driven development

TDD: a development practice in short cycles: write a failing automated test for the next small piece of behaviour, write just enough code to make it pass, then improve the code while the tests stay green.

**In Qtestframe:** unit tests written in TDD are reported through JUnit XML; link them to the software requirements they cover through their test keys.

**See also:** [Behaviour-driven development](#behaviour-driven-development), [Unit testing](#unit-testing), [Shift-left](#shift-left)

### TOTP

The Time-based One-Time Password algorithm (RFC 6238): a six-digit code derived from a shared secret and the current 30-second time step, shown by an authenticator app.

**In Qtestframe:** the second factor of [multi-factor authentication](#multi-factor-authentication): HMAC-SHA-1, 6 digits, 30-second steps, with one step of clock drift allowed and each code accepted only once.

**See also:** [HOTP](#hotp), [Multi-factor authentication](#multi-factor-authentication), [Multi-factor authentication](../../admin-guide/en/authentication.md#multi-factor-authentication)

### Trace link

A recorded relationship between two items in the lifecycle, such as "test T verifies requirement R". Trace links are the raw material of [traceability](#traceability).

**In Qtestframe:** a trace link connects a test with a requirement it verifies, optionally with one evaluation criterion; it is the basis of coverage. Other links connect requirements with each other (relations), with code, runs and documents.

**See also:** [Linked tests](requirements.md#linked-tests), [Traceability](#traceability), [Suspect link](#suspect-link)

### Traceability

The ability to follow the life of a requirement forwards and backwards: from its origin, through refinement, design and implementation, to its tests and their results. It supports impact analysis, completeness checks and audits.

**In Qtestframe:** trace links, relations, code links, the traceability matrix, dependency graphs, traceability rules and the impact analysis all serve traceability.

**See also:** [Traceability and coverage](traceability-and-coverage.md), [Forward traceability](#forward-traceability), [Backward traceability](#backward-traceability)

### Traceability matrix

A table with requirements in one dimension and tests (or other items) in the other, marking which are linked. It shows at a glance which requirements have no tests and which tests verify nothing.

**In Qtestframe:** **Traceability** shows requirements as rows and tests as columns, each mark coloured by the test's latest result and framed when the link is suspect; you can link and unlink in the matrix.

**See also:** [The traceability matrix](traceability-and-coverage.md#the-traceability-matrix), [Trace link](#trace-link), [Bidirectional traceability](#bidirectional-traceability)

### Traceability rule

A rule that items must follow in their links, such as "every approved MUST requirement has at least one test" or "every test verifies a requirement".

**In Qtestframe:** managers define rules with a query for the items, a condition and a severity; violations appear in the analytics and the work lists, and a release exit criterion can require that none are broken.

**See also:** [Traceability rules](traceability-and-coverage.md#traceability-rules), [Consistency check](#consistency-check), [Exit criteria](#exit-criteria)

### Trash

Where deleted items wait before they are removed for good.

**In Qtestframe:** deleted items can be restored from the trash for 30 days; afterwards the server purges them.

**See also:** [How do I undo a change?](faq.md#how-do-i-undo-a-change), [Revision](#revision), [Background job](#background-job)

## U

### Unit testing

The test level for the smallest testable parts of the software, such as functions, classes or modules, in isolation from the rest. Unit tests are written by developers, run in seconds and form the base of the test pyramid (also called component testing).

**In Qtestframe:** unit test results arrive from CI as JUnit XML; a test with a test key links them to Qtestframe tests and requirements.

**See also:** [Test level](#test-level), [Test-driven development](#test-driven-development), [Link automated tests to test keys](runs.md#link-automated-tests-to-test-keys)

### Usability testing

Testing how easily, effectively and pleasantly real users reach their goals with the product, typically by watching representative users perform tasks and measuring success, time and errors.

**In Qtestframe:** state usability goals as *Usability* requirements with measurable criteria, and record observations from test sessions as exploratory sessions or as *Demonstration* records.

**See also:** [Accessibility testing](#accessibility-testing), [Non-functional testing](#non-functional-testing), [Exploratory testing](#exploratory-testing)

### User requirement

A requirement that describes what a user must be able to do with the system, often written as a use case or a user story ("As a customer I want to …"). It is a kind of stakeholder requirement, focused on the users' tasks.

**In Qtestframe:** write user stories as requirements and their acceptance criteria as evaluation criteria; refine them into system or software requirements where needed.

**See also:** [Stakeholder requirement](#stakeholder-requirement), [Acceptance criteria](#acceptance-criteria), [Behaviour-driven development](#behaviour-driven-development)

## V

### V-model

A lifecycle model drawn as a V: the left side goes down from requirements over architecture to detailed design and code, the right side goes up through unit, integration, system and acceptance testing, and each test level verifies the work product opposite it. It makes clear early which tests will verify which specification.

**In Qtestframe:** requirement levels connected by **refines** and the tests linked at each level reflect the V; the roll-up shows verification from the bottom of the V to the top.

**See also:** [Test level](#test-level), [Verification](#verification), [Refinement](#refinement)

### Validation

Confirming that the product fulfils its intended use in its intended environment: "are we building the right product?" Validation involves stakeholders and real use; a product can pass every verification and still fail validation (IEEE 1012, ISO/IEC/IEEE 29148).

**In Qtestframe:** acceptance tests, demonstrations to users and exploratory sessions with users provide validation evidence; record them against the stakeholder or user requirements.

**See also:** [Verification](#verification), [Acceptance testing](#acceptance-testing), [Stakeholder requirement](#stakeholder-requirement)

### Variant

One version of a product line that shares most requirements and tests with the others, such as a model for one market.

**In Qtestframe:** items can apply to all variants or only some; lists, coverage, releases, documents and release gates can be shown for one variant.

**See also:** [Variants](variants-and-code.md#variants), [Release](#release), [Test configuration](#test-configuration)

### Verification

Confirming, with objective evidence, that a work product meets its specified requirements: "are we building the product right?" Verification uses test, inspection, analysis, demonstration and review (IEEE 1012, ISTQB Glossary).

**In Qtestframe:** a requirement is verified when every evaluation criterion has passing evidence (test results, verification records, optionally exploratory sessions) and all its children are verified; **Verification** shows the tree and the gaps.

**See also:** [Verification](traceability-and-coverage.md#verification), [Validation](#validation), [Verification method](#verification-method)

### Verification method

The way in which fulfilment of a requirement is shown. The classic methods are **test** (executing with defined inputs and expected results), **inspection** (examining code, documents or the product), **analysis** (calculation, simulation, modelling), **demonstration** (showing the function, without detailed measurement) and **review** (a formal assessment by qualified people).

**In Qtestframe:** each evaluation criterion has one of these five methods; *Test* is satisfied by linked tests, the others by [verification records](#verification-record).

**See also:** [Evaluation criteria](requirements.md#evaluation-criteria), [Verification record](#verification-record), [Verification](#verification)

### Verification record

The evidence of a verification that is not a test execution: who inspected, analysed, demonstrated or reviewed what, when, with which result and documents.

**In Qtestframe:** a record has a method, a result (*Pass*, *Fail*, *Conditional pass*), performer, date, note, documents and validity; it can be signed off by a second person and becomes invalid when the requirement changes, if so chosen.

**See also:** [Verification records](traceability-and-coverage.md#verification-records), [Verification method](#verification-method), [Evaluation criterion](#evaluation-criterion)

## W

### Waiver

A formal, signed acceptance that a requirement is not met (failed, untested or not fulfilled) for a release or a period, usually with conditions, compensating measures and an expiry date. Unlike a [deviation](#deviation), the requirement's intent is not achieved; the risk is accepted.

**In Qtestframe:** waivers are requested on the **Waivers** page or from a requirement, approved with an electronic signature by someone with `waiver.approve` (never the requester), and make the release exit criteria accept the waived requirements.

**See also:** [Waivers and deviations](waivers-and-soa.md#waivers-and-deviations), [Deviation](#deviation), [Residual risk](#residual-risk)

### WCAG

The Web Content Accessibility Guidelines of the W3C: testable success criteria, organised under the principles perceivable, operable, understandable and robust, at levels A, AA and AAA. Version 2.2 (2023) extends 2.1.

**In Qtestframe:** the web client aims at WCAG 2.1 AA, and the catalogue has requirements and tests that cite WCAG 2.2 for your own products.

**See also:** [Accessibility testing](#accessibility-testing), [Accessibility and languages](accessibility-and-languages.md), [Testing standards and guides](references.md#testing-standards-and-guides)

### Web test

An automated test that drives a real browser through a web application.

**In Qtestframe:** web tests are Playwright scripts (script language **Web test (Playwright)**), run on the server as background jobs when the administrator has enabled them, and store the output, screenshots and a trace as evidence.

**See also:** [Web tests (Playwright)](tests.md#web-tests-playwright), [End-to-end testing](#end-to-end-testing), [Script runner](#script-runner)

### WebAuthn and FIDO2

The W3C Web Authentication API and the FIDO2 standards: sign-in with a security key or a passkey, based on a key pair bound to the website, which resists phishing.

**In Qtestframe:** not a sign-in method of Qtestframe itself (it uses passwords, single sign-on and TOTP), but the catalogue has requirements and tests for passkeys and security keys that cite WebAuthn.

**See also:** [Multi-factor authentication](#multi-factor-authentication), [TOTP](#totp), [Catalogue](#catalogue)

### Webhook

An HTTP request that one application sends to another when something happens, so that the other does not have to ask repeatedly.

**In Qtestframe:** project webhooks send events such as `run.completed` or `defect.created` as signed JSON POST requests; incoming webhooks from GitHub and GitLab create [code links](#code-link). Webhook secrets are shown only once.

**See also:** [Webhooks](reviews-and-notifications.md#webhooks), [Code links](variants-and-code.md#code-links), [Notification](#notification)

### White-box testing

Testing based on the internal structure of the code or system: tests are designed to execute statements, branches, conditions or paths, and [code coverage](#code-coverage) measures how far they do (ISO/IEC/IEEE 29119-4).

**In Qtestframe:** white-box tests are usually unit tests run in CI; their results are imported, their code coverage stays in your build tools.

**See also:** [Black-box testing](#black-box-testing), [Code coverage](#code-coverage), [Unit testing](#unit-testing)
