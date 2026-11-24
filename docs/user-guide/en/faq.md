# FAQ

Answers to the questions users ask most often: first about using the tool, then, from [Testing concepts](#testing-concepts) on, about the concepts of testing and requirements engineering, the standards behind Qtestframe and how it works. Use the search field at the top of the guide to find a word on all pages; the [Glossary](glossary.md) explains the terms one by one.

## Getting access

### I forgot my password. What do I do?

There is no self-service reset. Ask an **admin** of your organisation to [reset your password](administration.md#reset-a-password). You get a temporary password that you must change at the next sign-in. If you sign in with single sign-on (a button below *or continue with*), your password is managed by your company's identity provider; reset it there.

### Why was my session ended?

A session ends when:

- it reaches its maximum age (12 hours by default, set by your administrator) and you have to sign in again;
- you or someone with access to your account selected **Sign out** for it under **Profile → Active sessions**, or **Sign out all other sessions** on another device;
- an administrator reset your password, set a new one, or disabled your account;
- a super admin restored a backup, which signs everyone out;
- your browser deleted its cookies.

Sign in again. If you did not expect it, change your password and check your active sessions.

### I see "You have no current organisation". What now?

You are signed in but not working in an organisation. Choose one in the organisation switch in the top bar. If the list is empty, you are not a member of any organisation yet; ask an admin to add you.

### How do I switch to another organisation?

Use the organisation switch at the left of the top bar (on phones, the building icon). Qtestframe opens that organisation's project list. To start in a particular organisation every time, set **Default organisation** in your preferences.

### A colleague cannot see a project. Why?

The project is probably **restricted** (it shows a lock 🔒): only its project members and the organisation's admins see it. Someone who may change the project adds your colleague with **Share** in the project header; see [Project sharing](administration.md#project-sharing). The project role can also be lower than the organisation role, so a colleague may see a project but not be allowed to edit it.

## Editing

### How do I refer to a colleague in a requirement or comment?

Type `@` followed by the start of their name, then pick them from the list, see [Mention people](requirements.md#mention-people). The list holds the users of the organisation and names already used as owner or assignee in the project. `mention:` in a filter finds what mentions someone.

### Where do the suggested tags come from?

From the title and description of the item. Tags the project already uses come first when the text mentions them, then abbreviations and frequent words. Common words and requirement keywords are never suggested. Select a suggestion to add it; ignore the others.

### Why can't I edit a requirement?

Writing requirements needs the **Manager** role in the current organisation. With **Viewer** or **Tester** the editor opens read-only, and buttons such as **New requirement** and **Save** are hidden. Check your role in the user menu; ask an admin if you need more rights. Testers can still link tests to requirements and comment on them.

### Why can't I see the Run script button?

Running scripts on the server needs the **Test lead** or **Manager** role (the permission *script.execute*), and the administrator must enable the script runner (`runner.enabled`). Without the role, the runner says that the *script.execute* permission is needed. If the runner is disabled on the server, use the [CI import](runs.md#import-results-from-ci) instead.

### How do I undo a change?

Select **Undo** (↶) in the top bar or press <kbd>Ctrl</kbd>+<kbd>Z</kbd> outside a text field; the **Undo history** (🕘) lists your recent changes. This works for edits, deletions, moves, bulk edits, links and more, see [Undo and redo](getting-started.md#undo-and-redo). A deleted requirement or test comes back with its key, links, comments and history.

For older changes, open the item, go to the **History** tab, select the version you want and choose **Restore this version**. This creates a new version with the old content; nothing is lost and you can restore again.

### Can Qtestframe write code or tests with AI?

In a **software** project, **AI prompts** builds a prompt from your requirements or tests (implement them, review them, design or automate test cases, plan a test type) that you copy into the AI assistant your organisation uses. Qtestframe itself does not call any AI service. See [AI prompts](requirements.md#ai-prompts-software-projects).

### Why can't I approve a requirement or mark a test ready?

Approving or rejecting a requirement needs the permission *requirement.review*, marking a test **Ready** needs *test.review*. By default the **Reviewer** and **Manager** roles have both and the **Test lead** has *test.review*; a **Tester** can write tests but not mark them ready. Without the permission these statuses are greyed out in the editor. See [Roles and permissions](administration.md#roles-and-permissions).

### Can two people edit the same requirement at the same time?

Yes, but the last one to save wins: each save replaces the fields with what that person sees. Both versions are in the **History**, so nothing is lost; compare them there and restore if needed. Use **Comments** to coordinate.

### Why was the obligation not updated when I changed the text?

The obligation is detected automatically only when the requirement is **created** with **Auto-detect from text**. After that it is never changed behind your back. Use **Apply detected obligation** in the analysis box, or pick the level yourself.

### What happens when I move a requirement to another project?

It gets a **new key** in the target project (for example `SHOP-REQ-7` becomes `CORE-REQ-31`); categories are matched by dimension and name and dropped if the target has no such value; **links to tests are removed**, because links never cross projects; criteria and history move along. Within a project, moving only changes the folder. See [Move requirements and folders](requirements.md#move-requirements-and-folders).

### Who can see my comments?

Everyone who can read the item, which means every member of the organisation with access to the project, whatever their role. Comments are not visible to other organisations. You can edit and remove your own comments; moderators (who may change the project's settings) can remove any comment, which then shows as removed. Comments are deleted together with their item. See [Discussions](discussions.md).

### What does "auto-generated" mean on a test?

The test was created automatically together with a requirement (**Also create linked test**). It is an ordinary test: refine its steps, change its status and remove the tag if you like.

## Coverage and results

### How is coverage calculated?

From the **last result** of every test linked to the requirement: no linked test → *None*; any linked test failed or blocked → *Failed*; all passed → *Passed*; some passed and the rest not run, skipped or in progress → *Partial*; otherwise *Not run*. Only runs of the same project count. Details and examples are in [How coverage is calculated](traceability-and-coverage.md#how-coverage-is-calculated).

### What does "Not run" mean?

For a **test** or **execution**: it has no result yet. For a **requirement's coverage**: tests are linked, but none of them has passed or failed in any run of this project yet. That is different from *None*, which means no test is linked at all.

### A test passed again; why is my requirement still "Failed"?

Coverage uses the latest result of **each** linked test. If another linked test is still failed or blocked, the requirement stays *Failed*. Open the requirement's **Linked tests** to see each test's last result. Also check that the new result was saved (in the runner, **Save**).

### Why does the dashboard say "covered" when the tests have not been run?

On the dashboard, *covered* means that at least one test is linked. Whether those tests passed is shown in **Requirement coverage** next to it.

### How do I import CI results?

Create an API token (**Administration → API tokens**, role Tester or higher), open the run and select **CI import** for a ready-made command, and post the results by test key from your pipeline. See [Import results from CI](runs.md#import-results-from-ci).

### A CI import reports "unknown" test keys. Why?

Those keys do not exist in the run's project. Check the spelling (case does not matter) and that the tests have not been moved to another project, which gives them new keys.

### Why are Pass and Fail disabled in the runner?

The test may not be executed yet: it is not **Ready**, a requirement it verifies is not approved, or a test it depends on has not passed. The red box above the steps lists each reason with what to do and the setting that decides: whether the **test** is not Ready (still Draft or in Review), a **requirement** it verifies does not have one of the statuses the execution settings require (for example it is Approved but the settings require Implemented), or a **prerequisite** has not passed. Fix them (or ask an admin or test lead to change the [test execution settings](administration.md#test-execution-settings)); meanwhile you can mark the test **Blocked** or **Skipped**.

### How do I create a role of my own?

*(Admin)* **Administration → Role matrix → New role**: give it a name, choose the role whose permissions it starts with, then tick and untick permissions in the matrix and save. See [Change roles and create your own](administration.md#change-roles-and-create-your-own).

### Can I change a result after saving it?

Yes. Open the execution in the runner, change the step results or the overall status, and save. The time and the person are updated. The test's execution history shows the latest state of each execution.

## Baselines and history

### What is the difference between a baseline and a revision?

A **revision** is one version of one item, created automatically on every change (the **History** tab). A **baseline** is a named snapshot of the versions of all (or filtered) items of a project at one moment, created by you, for example for a release. See [Baseline or revision?](baselines.md#baseline-or-revision).

### Does deleting a baseline delete requirements?

No. A baseline only records versions. Deleting it leaves all items unchanged.

## Search and filters

### Why is my query invalid?

Usually a field that does not exist in that list (for example `coverage` in tests), a space after the operator (`status: approved`), an unclosed quote, parenthesis or regex, or a regex feature RE2 does not support. The field is marked and the message names the position. Open **Query help** (**?** next to the filter) for the fields. See [Why is my query invalid?](search-and-filters.md#why-is-my-query-invalid).

### How do I find requirements without tests?

Filter the requirements with `coverage:none`, or `-linked:*`. Combine it, for example `coverage:none obligation=must`.

### Does search cover all projects?

The search in the top bar covers all projects of the **current organisation** that you can read. Filters in a list only cover that list. Other organisations are never searched.

## Settings and devices

### What does a field mean?

Point at the small **i** next to it, or move the keyboard focus there: a short explanation appears, with a link to the section of this guide that describes the field. Press <kbd>?</kbd> to open the guide page of the current view. For the standards behind requirement levels and testing, see [References](references.md).

### How do I change the language or theme?

Language: **User menu → Language** (English or Svenska). Theme: the **Colour theme** picker in the top bar, or the sun/moon button for light and dark. Both are also in **Profile → Preferences** and are stored with your account, so they follow you to every device.

### Can I use Qtestframe on a phone?

Yes. The interface adapts to phones and tablets: the sidebar becomes a drawer behind the menu button, tables become cards and editors open full-screen. Executing manual tests on a tablet works well. See [Phones and tablets](accessibility-and-languages.md#phones-and-tablets).

### How do I restart the intro tour?

Select **Help** (the question mark in the top bar) → **Restart intro tour**.

### Can I change the key of a project?

No. The key is part of every item ID and cannot be changed after the project is created. You can change the project's name, description and colour in **Categories & settings** (or with ✎ on the project list). If you really need another key, use **Duplicate project** on the project list: the copy gets the new key, and all item keys follow it (see [Save and load projects](reports-and-export.md#save-and-load-projects)). Then delete the original.

## Files and data

### Where are files stored?

In the Qtestframe database, together with the rest of the data. Files are therefore included in backups, protected by the project's access rules, and deleted with the project. See [Documents](documents.md#where-are-files-stored).

### How large may an uploaded file be?

25 MB per file by default; your administrator can change the limit. Avatars and project icons may be at most 1 MB.

### Why does a file download instead of opening?

Only images (PNG, JPEG, GIF, WebP), PDF files and plain text are shown in the browser. All other types are downloaded, so that no file can run code in your browser.

### How do I restore a backup?

A **super admin** opens **Administration → Backup**, chooses the backup file under **Restore**, types `RESTORE` and selects **Restore backup**. All current data is replaced and everyone is signed out. The backup must come from the same schema version. Server administrators can use `qtestframe restore --in FILE.zip --yes`. To recover a single item, use its history instead.

### How do I move a project to another server?

Select **Save project** (the download button on the project list) and, on the other server, **Load project** with the file. The copy has everything in the project except secrets such as webhook secrets and the tracker token; see [Save and load projects](reports-and-export.md#save-and-load-projects). To move all organisations at once, a super admin uses a backup instead.

### Can I get my data out of Qtestframe?

Yes: **Export** downloads requirements, criteria, tests, traceability and runs as XLSX, CSV, PDF or DOCX, and every function is also available through the REST API with an API token.

## Testing concepts

### What is the difference between a test case, a test run and an execution?

A **test case** (in Qtestframe: a *test*) describes how to check something: preconditions, steps and expected results, or a script. A **test run** is one round of testing, such as *Sprint 12 regression*, in an environment and build. An **execution** is one test inside one run, with its result: the same test has a new execution in every run it is part of, and its history shows them all. A **test step** is one action of a manual test with its expected result. See [Test case](glossary.md#test-case), [Test run](glossary.md#test-run) and [Test execution](glossary.md#test-execution).

### What is the difference between a test cycle, a test plan and a test suite?

A **test plan** is the document that says what will be tested, how, by whom and with which entry and exit criteria; Qtestframe can [generate one](documents.md#create-a-requirements-specification-or-a-test-plan) from the project. A **test cycle** is the operational plan of one testing period: which tests, which testers, which estimates; when it starts, it creates its run. A **test suite** is a group of tests for a purpose; Qtestframe has no separate suite object, so group tests with folders, categories or tags and create runs from a query. See [Test cycle](glossary.md#test-cycle), [Test plan](glossary.md#test-plan) and [Test suite](glossary.md#test-suite).

### Which test levels and test types does Qtestframe support?

All of them: Qtestframe does not prescribe levels (unit, integration, system, acceptance, end-to-end) or types (functional, performance, security, usability, accessibility, regression …). Record them where you need them: as a category dimension such as *Level / System* or *Type / Security*, as tags, as folders, or as separate runs and cycles. Unit and integration results usually come from CI, system and acceptance results from manual runs. See [Test level](glossary.md#test-level) and [Test type](glossary.md#test-type).

### How do I apply test design techniques such as boundary values in Qtestframe?

Design the cases with the technique, then make them **test data**: one row per equivalence partition, boundary value, decision table rule or pairwise combination, with the parameters used as `{{name}}` in the steps. Each row becomes an iteration with its own result. Record the technique as a category if you want to report on it. See [Data-driven tests](tests.md#data-driven-tests) and [Test design technique](glossary.md#test-design-technique).

### What is the difference between severity and priority?

**Severity** says how bad a defect's effect is (from *Critical* to *Trivial*); **priority** says how soon it should be fixed (from *Critical* to *Low*). A typo in the company name on the start page is trivial but urgent; a crash in a rarely used export can be critical but wait. Release exit criteria and blocking rules use severity. See [Statuses and severities](defects.md#statuses-and-severities).

### What is the difference between verification and validation?

**Verification** checks that the product meets its specified requirements ("built right"); **validation** checks that it meets its intended use ("the right product"). In Qtestframe, a requirement is *verified* when every evaluation criterion has passing evidence; validation is usually shown by acceptance tests, demonstrations and exploratory sessions with users against stakeholder requirements. See [Verification](glossary.md#verification) and [Validation](glossary.md#validation).

### What is a flaky test, and how does Qtestframe find them?

A test that passes and fails by turns without any change. Qtestframe counts the flips between passed and failed in the last 10 results of every test, manual and automated; two or more flips make it **Flaky**. Find them with `flaky:true`. See [Flaky tests](tests.md#flaky-tests).

### Does Qtestframe measure code coverage?

No. Code coverage is measured by your build and test tools. Qtestframe measures **requirement coverage**: which requirements have tests and what their latest results are. It also links commits and builds to requirements and tests, so you can see which changes were tested. See [Code coverage](glossary.md#code-coverage) and [How coverage is calculated](traceability-and-coverage.md#how-coverage-is-calculated).

### What are entry and exit criteria in Qtestframe?

**Entry criteria** decide when testing may start: per test, the [execution rules](runs.md#when-can-a-test-be-executed) require it to be *Ready*, its requirements approved and its prerequisites passed. **Exit criteria** decide when testing is done and a release may go: coverage, pass rate, mandatory requirements passed, no open defects, risks mitigated and more, on the release's [Exit criteria](releases-and-risks.md#exit-criteria) tab.

### Can I do exploratory testing in Qtestframe?

Yes, with **session-based test management**: plan a session with a charter and scope, test with a timer and typed notes, raise defects from notes, and close it with a debrief. Debriefed sessions can count as verification evidence. See [Exploratory testing](exploratory-testing.md).

### How does Qtestframe fit into CI/CD, TDD or BDD?

Your pipeline runs the automated tests and uploads the results with `qtestframe report`, as JUnit XML or with the Playwright reporter; they land in a run, count for coverage and release criteria, and record the build. Tests written in TDD or BDD style are linked to Qtestframe tests by their test key. See [Import results from CI](runs.md#import-results-from-ci), [Test-driven development](glossary.md#test-driven-development) and [Behaviour-driven development](glossary.md#behaviour-driven-development).

## Requirements concepts

### What is the difference between a requirement and a specification?

A **requirement** is one verifiable statement of what is needed. A **specification** is the organised whole: introduction, scope, definitions and the requirements, often for one system or delivery. In Qtestframe the project is the living specification, and **Create document → Requirements specification** turns it into a document. See [Specification](glossary.md#specification).

### Should I write MUST or SHALL? And what about SKA and BÖR?

Both MUST and SHALL mark an absolute requirement under RFC 2119; choose one and use it consistently (many organisations use SHALL for contracts, MUST for technical specifications). Write the keywords in capitals: only upper-case keywords carry the normative meaning (RFC 8174). In Swedish, use **SKA** (shall), **BÖR** (should) and **FÅR** (may), and **FÅR INTE** for a prohibition; avoid *måste inte*, which means "need not". See [Obligation levels](requirements.md#obligation-levels) and [SKA, BÖR and FÅR](glossary.md#ska-bör-and-får).

### What is the difference between functional and non-functional requirements?

A **functional** requirement says what the system does ("sends an order confirmation"); a **non-functional** requirement says how well or under which conditions ("within 5 seconds for 99 % of orders"). Non-functional requirements need measurable thresholds; put them in an evaluation criterion. ISO/IEC 25010 is a good checklist of qualities. See [Non-functional requirement](glossary.md#non-functional-requirement).

### What is the difference between acceptance criteria and evaluation criteria?

Acceptance criteria are the conditions for accepting a requirement or a story. Qtestframe calls them **evaluation criteria** and adds to each a method (test, inspection, analysis, demonstration, review) and a threshold, so that it is clear how each criterion will be shown. See [Evaluation criteria](requirements.md#evaluation-criteria).

### Which verification method should I choose?

**Test** when the behaviour can be executed with defined inputs and expected results; **inspection** for documents, code, labels or configuration; **analysis** for calculations, simulations or measurements evaluated afterwards; **demonstration** to show a function to a reviewer without detailed measurement; **review** for a formal assessment by qualified people. Methods other than test are recorded as [verification records](traceability-and-coverage.md#verification-records). See [Verification method](glossary.md#verification-method).

### What does bidirectional traceability mean in Qtestframe?

Every trace link can be followed both ways: from a requirement to its tests (forward) and from a test to its requirements (backward), in the editors and in the traceability matrix. Relations do the same between requirements (*refines* / *refined by*). Traceability rules check both directions, for example "every approved requirement has a test" and "every test verifies a requirement". See [Traceability](glossary.md#traceability).

### What is a suspect link, and how do I clear it?

A link becomes **suspect** when the requirement or test at one end changes in substance after the link was confirmed, because the other end may no longer fit. Review the other item, update it if needed, then confirm the link: with **Confirm suspect** in the traceability matrix, or **Confirm** on the requirement's **Impact** tab. See [Change impact](traceability-and-coverage.md#change-impact).

### What is the difference between refines and derived from?

**Refines** is decomposition: the child requirement is a more detailed part of its parent, and the parent is verified only when its children are. **Derived from** says a requirement follows from another one or from a design decision without being part of it, such as a time budget for a component derived from a system response time. Both make the dependent requirement suspect when the other changes. See [Relations](requirements.md#relations).

### When do I need a waiver and when a deviation?

A **waiver** accepts that a requirement is not met (failed, untested or not fulfilled) for a release or a period; a **deviation** accepts that it is met by a different solution than specified. Both need a signed approval and can expire. See [Waivers and deviations](waivers-and-soa.md#waivers-and-deviations).

### How do I write good requirements?

One requirement per statement, with a clear subject, an obligation keyword, measurable terms and no escape clauses; say what, not how; give the rationale and the source. The [language analysis](requirements.md#the-language-analysis) checks many of these rules as you type, following the INCOSE guide; ISO/IEC/IEEE 29148 lists the characteristics of good requirements, and EARS gives sentence patterns. See [Requirement quality](glossary.md#requirement-quality).

### How do I work risk-based?

Record the risks in the risk register, link the requirements that address them, and let the risk level drive what you specify and test first: build release scopes with `risk:high`, and add the exit criteria *Risks mitigated* and *Residual risk*. See [Risk-based testing](glossary.md#risk-based-testing) and [How risks affect a release](releases-and-risks.md#how-risks-affect-a-release).

## Standards and formats

### Which standards does Qtestframe follow?

For requirement wording RFC 2119 and RFC 8174 (and SIS usage in Swedish); for documents ISO/IEC/IEEE 29148 (requirements specification) and 29119-3 (test plan); for exchange ReqIF and OSLC; for results JUnit XML; for sign-in OpenID Connect, OAuth 2.0 with PKCE, SAML 2.0, SCIM 2.0 and TOTP; for the API HTTP semantics with ETag/If-Match and OpenAPI; for logs syslog; for the web client WCAG 2.1 AA. The [References](references.md) page lists them with links; the [Glossary](glossary.md#entries-by-topic) explains each.

### ReqIF or OSLC: which one should I use?

**ReqIF** moves whole specifications as files between tools, for hand-overs, supplier exchange and round trips. **OSLC** connects tools live: another tool links its items to Qtestframe's requirements and tests and can create and change them without a file. Many teams use both. See [Which one to use?](interoperability.md#which-one-to-use)

### Which test result formats can Qtestframe import?

**JUnit XML**, which almost every test framework writes, through the API or `qtestframe report`, and the results of the **Playwright reporter**. Manual results are recorded in the runner or many at once on the run page. See [Import results from CI](runs.md#import-results-from-ci).

### Which single sign-on standards are supported?

**OpenID Connect** (on OAuth 2.0, with PKCE) and **SAML 2.0**, with user provisioning through **SCIM 2.0**. Your administrator sets them up; see [Single sign-on (OIDC)](administration.md#single-sign-on-oidc) and [Provisioning with SCIM](administration.md#provisioning-with-scim).

### Does Qtestframe support passkeys or security keys?

Yes. Add a passkey in your profile (see [Passkeys](getting-started.md#passkeys)): it signs you in without a password, and it is a second factor after your password, next to an authenticator app. Accounts of single sign-on sign in at the identity provider, which can also require passkeys or security keys. The catalogue also has requirements and tests that cite WebAuthn for your own products. See [WebAuthn and FIDO2](glossary.md#webauthn-and-fido2).

### Where does the MITRE ATT&CK data come from?

From MITRE's published data: the ATT&CK (Enterprise, ICS, Mobile), ATLAS and EMB3D lists in the catalogue are generated from their **STIX 2.1** bundles, and D3FEND from its published data, without revoked or deprecated entries. Names are shown in English, as at MITRE. See [MITRE ATT&CK](catalogue.md#mitre-attck).

### Can Qtestframe help with ISO/IEC 27001, the CRA or the AI Act?

Yes. The catalogue lets you choose the laws and standards a project must comply with and adds the requirements that cite them, with tests; the compliance analytics show how far each source is verified, and the **SoA** page covers the ISO/IEC 27001 Annex A controls. Qtestframe does not certify anything: it organises your evidence. See [Comply with laws, standards and best practices](catalogue.md#comply-with-laws-standards-and-best-practices).

### Is Qtestframe itself accessible?

The web client aims at WCAG 2.1 level AA: full keyboard use, screen reader support, high-contrast and other colour themes, browser zoom with a layout that reflows, and phone layouts, checked with automated accessibility tests. See [Accessibility and languages](accessibility-and-languages.md).

## How Qtestframe works

### Where is my data stored?

In one database on your organisation's server, SQLite or PostgreSQL, including uploaded files. Secrets are encrypted with a key kept outside the database. See [Where your data lives](how-it-works.md#where-your-data-lives).

### What happens in the background?

Automated tests run as jobs, notifications, webhooks and chat messages are delivered after a change is saved, e-mail digests and scheduled reports go out on time, dashboard trends are recorded daily, and old sessions and trash are purged. See [Background work](how-it-works.md#background-work).

### How is my data protected?

Organisations are isolated from each other, every request is checked against your role's permissions, sessions are protected against cross-site request forgery, multi-factor authentication and single sign-on are available, secrets are encrypted and write-only, and the audit log records important actions. See [Security](how-it-works.md#security).

### Can scripts on the server harm it?

Only people with the `script.execute` permission can run scripts, and only when the administrator has enabled the runner. Each script runs in a sandbox with resource limits, no new privileges, file access limited by Landlock and system calls limited by seccomp. Web tests need a browser and the network, so their sandbox is weaker; administrators should limit the server's outgoing connections. See [The script sandbox](how-it-works.md#the-script-sandbox).

### What is the difference between a backup, a project archive and an export?

A **backup** holds the whole installation and is restored by a super admin. A **project archive** holds one project, without secrets, and is loaded as a new project. An **export** takes data out in a format for people or other tools (PDF, Word, Excel, CSV, Markdown, ReqIF). See [Backups and project archives](how-it-works.md#backups-and-project-archives).

### How do I find a word I don't understand?

Look it up in the [Glossary](glossary.md), or type it in the search field at the top of the guide: the search covers every page, the glossary included.
