# Reports and export

Qtestframe gets data out in two ways (a whole project can also be [saved as a file and loaded again](#save-and-load-projects)):

- **Export** downloads the raw project data (requirements, criteria, tests, traceability, runs) as spreadsheets, documents or Markdown, for reviews, audits, offline work or other tools.
- **Reports** produce formatted documents for people: a project status report, a coverage report, a run summary, a defect list or a report you design yourself. Reports can be downloaded or stored in the project's [document archive](documents.md).

Everyone with the **Viewer** role or higher may export and generate reports. Saving a custom report definition needs **Tester** (tests) or **Manager** (requirements); storing a generated report in the archive is open to everyone who may generate it.

## Export

Select **Export** in the project sidebar.

![The export page with format, aspects and an optional filter](../images/en-export.png)

1. Choose the **Format**:

   | Format | Result |
   |---|---|
   | **PDF** | A printable landscape report with one table per aspect and page numbers. |
   | **DOCX** | An editable Word document with a heading and a table per aspect, for example as the base of a specification document. |
   | **XLSX** | An Excel workbook with a *Summary* sheet and one sheet per aspect; bold header row, frozen first row, auto-filter and sensible column widths. |
   | **CSV** | One `.csv` file for one aspect; a `.zip` with one CSV per aspect when you choose several. UTF-8, comma-separated. |
   | **Markdown** | One `.md` file with every aspect: a heading, the project, filter and time, and a table per aspect. Plain text that reads well in a wiki, a Git repository or any text editor; keys link to Qtestframe when the server address is configured. |
   | **ReqIF** | The requirements as a ReqIF 1.2 file (`.reqif`) for requirements tools such as IBM DOORS, Polarion, Jama or ProR. See [ReqIF for requirements tools](#reqif-for-requirements-tools). |
   | **ReqIF (with attachments)** | A `.reqifz` archive: the ReqIF file and the requirements' attachments and images. |

2. Choose the **Aspects**:

   | Aspect | Columns |
   |---|---|
   | **Requirements** | key, title, obligation, type, priority, status, folder, categories, tags, description, rationale, source, owner, version, linked tests, coverage |
   | **Evaluation criteria** | requirement, number, criterion, method, threshold |
   | **Tests (with steps)** | key, title, execution, status, priority, folder, categories, tags, preconditions, steps, expected result, script language, linked requirements, last result |
   | **Traceability matrix** | one row per requirement–test link with coverage and last result; requirements without links get one row |
   | **Runs & executions** | one row per execution with run, status, environment, build, result, executed by, duration and notes |

   ReqIF contains requirements only, so the aspects are not used for it.

3. Optionally enter a **Filter**. It applies to requirements if it is a valid requirement query and to tests if it is a valid test query: `obligation=must` filters only requirements, `type=automated` only tests, `folder:Checkout` both.
4. Select **Download**. The file is named after the project key and the time, for example `SHOP-20261119-143000.xlsx`.

**Safe for spreadsheets:** values that start with `=`, `+`, `-` or `@` are prefixed in CSV files, and XLSX cells are always text, so nothing in your data is executed as a formula.

### ReqIF for requirements tools

ReqIF (Requirements Interchange Format) is the standard file format for exchanging requirements between tools. Use it to give a customer or supplier your requirements in their own tool, or to bring requirements back after they were worked on there.

What the ReqIF file contains:

- **One object per requirement** with key (*ReqIF.ForeignID*), title (*ReqIF.Name*), description as formatted text (*ReqIF.Text*), obligation, type, priority and status as value lists, source, rationale, owner, tags, categories, acceptance criteria (one line per criterion: method, text and threshold), version, author and creation time.
- **The folders as headings.** Every folder is a heading object (*ReqIF.ChapterName*); the requirements of a folder and then its subfolders are placed below it, so the requirements tool shows the same chapters as the requirements tree. With a filter, only folders that contain exported requirements are included.
- **Relations** between the exported requirements (*refines*, *depends on* and the others) as ReqIF relations.
- **Formatting.** Paragraphs, line breaks, lists, headings, code blocks, bold, italics, links and images in descriptions are converted to formatted text. In a `.reqifz` file, images and other attachments are files in the archive and are listed in the *Attachments* attribute.
- **Stable identifiers.** Every object keeps the same identifier in every export, so a tool can recognise requirements it already has and update them.

**Importing it again.** A ReqIF file from Qtestframe can be [imported](requirements.md#import-requirements) into any project. Titles, descriptions with their formatting, obligation, type, priority, status, source, rationale, owner, tags, criteria and folders come back; the original key is kept as the external ID (*ID: SHOP-REQ-12* in the source), and the new requirements get keys of the target project. Relations, categories and attachments are not imported.

## Standard reports

Select **Reports** in the project sidebar. The standard reports are ready-made layouts:

| Report | Content | Parameters |
|---|---|---|
| **Project status** | key figures, requirements by status and obligation, coverage, recent runs | – |
| **Requirements coverage** | every requirement with its linked tests, their last results and the coverage | optional filter |
| **Traceability** | the requirement × test links, and the [traceability rules](traceability-and-coverage.md#traceability-rules) with their results and violations | optional filter |
| **Run summary** | one run: totals, executions, defects and durations | the run (required) |
| **Defects** | all defects recorded on executions, grouped by defect | – |
| **Test inventory** | tests by folder, type, status and category | optional filter |
| **Risk report** | the [risks](releases-and-risks.md) with key figures, the heat map before and after mitigation, the risks above the acceptance threshold, the risk register with the [residual risk](waivers-and-soa.md#residual-risk), traceability to requirements and tests, untested mitigations and the top risks | – |
| **Compliance** | compliance with laws, standards and best practices: per source, per clause, the missing catalogue requirements and the requirements not fulfilled (see [Compliance with laws and standards](traceability-and-coverage.md#compliance-with-laws-and-standards)) | optional laws, standards and best practices (default: those the project uses) |
| **Statement of Applicability** | every control of a standard with applicability, justification, implementation, owner, requirements and evidence (see [Statement of Applicability](waivers-and-soa.md#statement-of-applicability)) | the standard (default: ISO/IEC 27001:2022 Annex A) |
| **Release readiness** | one release: go / no-go, every exit criterion with what blocks it and what a [waiver](waivers-and-soa.md#waivers-and-deviations) or risk sign-off accepts, the waivers in force and the risks above the acceptance threshold | the release (default: the current one) |

**The risk report** contains:

- **Key figures**: all risks; open risks (*Identified* or *Mitigating*); mitigated, accepted and closed risks; open risks per level; open risks without requirements, with an untested mitigation and with a verified mitigation; and open risks that have not been changed for 90 days or more.
- **Risks by level and status**: a table with one row per level and one column per status.
- **Open risks by mitigation state** (see below).
- **Heat map**: likelihood (rows, 5 at the top) × impact (columns). Each cell gives the number of risks that are not closed and the cell's level, for example *2 · High*, so the table can be read without colours.
- **Top open risks**: the ten open risks with the highest score.
- **Risk register**: every risk with key, title, category, owner, likelihood, impact, score, level, status, mitigation, the requirements that address it, the mitigation state and when it was last changed.
- **Traceability**: one row per risk, requirement and linked test, with the requirement's coverage and the test's latest result.
- **Open risks with untested mitigation**: open risks without requirements, or with requirements that have no tests, and which of their requirements have no tests or have not passed.
- **Open risks not updated for 90 days or more**: a reminder to review them.

The **mitigation state** of a risk comes from the requirements that address it: *No requirements*, *Untested* (none of them has linked tests), *Partly tested* (some have), *Tested, not all passed* (all have tests, but not all tests passed) or *Verified* (all linked tests of all its requirements passed). A risk has one likelihood × impact score; the report shows that score and its level, and how far the mitigation is verified by tests.

![The reports page with the standard reports and the custom report builder](../images/en-reports.png)

1. Choose a report.
2. Fill in its parameters, for example the run for **Run summary**.
3. Choose the **Format**: **PDF**, **Word** (DOCX), **Excel** (XLSX), **CSV** or **Markdown** (`.md`, text with tables for wikis, Git repositories and chat tools; several sections are one document).
4. Select **Download**, or **Save to archive**. A saved report becomes a document of kind *report* in **Documents**, where the whole team can find, preview and download it later.

Reports are produced in the language you currently use in Qtestframe (**User menu → Language**, see [Languages](accessibility-and-languages.md#languages)). With the Swedish interface, the report's title, section names, column headers, key figures and values such as statuses, results, coverage and obligation (for example *Godkänt*, *Underkänd*, *MUST (SKA)*) are in Swedish, with the same words as in the app. Your own content, such as titles, names, descriptions, tags and notes, is never translated. This applies to downloads, reports saved to the archive, custom reports, several reports at once and reports sent by e-mail.

### Documents

Below the standard reports, **Documents** offers two complete documents: a **requirements specification** and a **test plan** with its test specification. **Create…** opens the generator in the [document archive](documents.md#create-a-requirements-specification-or-a-test-plan), where the document is saved.

## Custom reports

Under **Custom report** you design a report yourself:

1. **Report on**: requirements, tests, test runs or the executions of a run (then choose the run).
2. **Filter**: any [query](search-and-filters.md), for example `status:approved obligation=must`.
3. **Columns**: pick the columns; arrange them under **Column order**.
4. **Group by**: optionally group rows, for example by status or folder, with a count per group.
5. **Sort by**, optionally **Descending**.
6. **Include summary**: totals at the top.
7. **Title** of the report.

![The custom report builder with item type, filter, columns and grouping](../images/en-report-builder.png)

Generate the report in any format, as above. To reuse it, select **Save report definition** and give it a name: **Load saved report** brings it back at any time, to generate it again with the current data, change it (**Save as a new report** keeps the original) or delete it. **New** starts an empty report.

**Examples:**

- *Open MUST requirements*: requirements, filter `obligation=must -status:verified`, columns key, title, status, owner, coverage, grouped by owner.
- *Failed tests this sprint*: executions of the run *Sprint 12*, filter `status:failed`, columns key, title, executed by, defects, notes.
- *Automation status*: tests, grouped by execution type, columns key, title, status, last result.

## Several reports at once

**Several reports at once**, at the bottom of the Reports page, lists the standard reports and your saved custom reports. Tick the ones you need, for example a weekly pack of *Project status*, *Run summary* and two saved reports, then:

- choose the **format** (only formats that every ticked report supports are offered),
- for *Run summary*, choose the **run**,
- **Download as ZIP** to get all of them in one file, or **Save to archive** to store each of them in the [document archive](documents.md) (tagged *report*). When saving, a report that cannot be created is listed with the reason and the others are still saved.

**Delete saved reports** removes the ticked saved custom reports (standard reports cannot be deleted). Saving to the archive and deleting need permission to edit requirements or tests; downloading needs the export permission.

## Send a report by e-mail

When the server can send e-mail, every standard report has an envelope button (**Send by e-mail**), and the custom report builder has **Send by e-mail** too. Choose:

- **Members**: people of your organisation who may read the project. A member who may not read what the report shows (for example test runs) is left out, and the message says who.
- **Other addresses**, separated by commas: people outside the organisation. This needs the permission to update the project, so that Qtestframe cannot be used to send e-mail to anyone.
- an optional **Message** and the **Format**.

The report is created with your permissions and attached to the e-mail; the report and the e-mail text are in your current interface language. If it cannot be delivered, you get a [notification](reviews-and-notifications.md#notifications).

## Scheduled reports

**Scheduled reports**, at the bottom of the Reports page, e-mails [saved custom reports](#custom-reports) on a schedule. *(Requires permission to edit requirements or tests.)* Select **Schedule a report**, then choose the saved report, **How often** (daily, weekly on a weekday, or monthly on a day from 1 to 28), the **Time** and **Time zone**, the **Format**, the recipients and an optional message.

The list shows each schedule with its recipients, when it was **Last sent** (or the error of the last attempt), and the **Next run**. Turn **Active** off to pause a schedule, **Send now** to send it at once (the schedule stays as it is), or edit and delete it. Only the person who scheduled a report, or someone who may update the project, can change it.

Good to know:

- The report is created with the permissions of the person who scheduled it, at the time it is sent. If that person is no longer a member or may no longer export, the schedule shows an error instead of sending.
- The report and its e-mail are in the interface language of the person who scheduled the report or last edited the schedule. Switch the language and save the schedule again to change it.
- Members who may no longer read the report are left out.
- If the server was down when a report was due, it is sent once when the server is back, not once for every missed time.
- Deleting the saved report also deletes its schedules.

## Save and load projects

A whole project can be saved as one file and loaded again as a **new** project, on the same server or another one: to hand a project over, keep a copy outside the server, or start a new project from a template.

- **Save project**: on the project list (the download button of a project) or under **Categories & settings → Project → Save and load**. The file (`<key>-project-<date>.zip`) holds everything in the project: requirements with criteria and folders, tests with steps and test data, traceability links and relations, links, runs with all results, iterations and evidence, defects, risks, releases, baselines and sign-offs, reviews, comments, version history, documents and attachments with their content, categories, configurations, traceability rules, saved reports, webhooks and the project settings. It needs the permissions to change the project and to export (by default **Manager** and **Admin**).
- **Load project**: on the project list, or in the same section of the settings. Choose the file; Qtestframe checks it and shows what it contains. Then choose the **Key** and **Name** of the new project (a free key is suggested if the original one is taken) and select **Load project**. You need the permission to create projects. The new project opens when it is ready.
- **Duplicate project** does both in one step: it creates a copy with a new key inside the same organisation.

What to know about loaded projects:

- The new project gets new IDs everywhere, and item keys follow the new project key (`DEMO-REQ-1` becomes `COPY-REQ-1`, also where a description mentions it). New items continue the numbering of the original.
- People stay as they were: *created by*, owners and assignees are kept as text, and comments and reviews refer to the same accounts. A review request whose reviewer has no account on the server is left out; the result says how many entries were left out.
- **Secrets are never saved**: webhooks are loaded switched off and without their secret, and the defect tracker without its token. Set them again under **Categories & settings → Project** before you switch them on.
- Members, roles, API tokens, notifications, scheduled e-mail reports, the audit log and the trash are not part of the file. Access to the new project follows the roles of the organisation, as for every project.
- A file saved by a newer version of Qtestframe cannot be loaded; update the server first. Files from older versions load.

The file contains the project data in plain form. Store it as carefully as the project itself.

## Which one to use?

| Need | Use |
|---|---|
| Work with the data in Excel or another tool | **Export**, XLSX or CSV |
| Put the data or a report into a wiki, a pull request or a Git repository | **Export** or **Reports**, Markdown |
| Hand requirements over to a requirements tool (DOORS, Polarion …) or another Qtestframe | **Export**, ReqIF |
| A specification document to review or sign | **Documents → Create document → Requirements specification** (PDF or Word, from a baseline if it is to be signed); for a plain table **Export**, DOCX |
| A test plan with the test specification | **Documents → Create document → Test plan** |
| A status report for a steering meeting | **Reports → Project status**, PDF |
| Evidence for an audit or a release | **Reports → Requirements coverage** or **Run summary**, saved to the archive, together with a [baseline](baselines.md) |
| A recurring list with your own columns | A saved **custom report** |
| The same report in people's inbox every week | A saved custom report under **Scheduled reports** |
| Hand a whole project over, keep a copy or start from a template | **Save project** and **Load project** (see [above](#save-and-load-projects)) |
