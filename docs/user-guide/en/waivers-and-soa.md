# Waivers, residual risk and the Statement of Applicability

Not every requirement is met in time, and not every risk can be removed. This page explains three ways to record such decisions formally, so that they are visible, approved and time-limited:

- a **waiver** accepts that requirements are not met for now; a **deviation** accepts that they are met by a different solution;
- the **residual risk** of a risk is what is left after its mitigation, compared with a **risk acceptance threshold** and accepted with a sign-off;
- the **Statement of Applicability** (SoA) records, for every control of a standard such as ISO/IEC 27001, whether it applies, why, who owns it and how far it is implemented.

| Action | Role needed |
|---|---|
| View waivers, residual risks and the Statement of Applicability | Viewer |
| Create, edit, request and withdraw waivers; enter residual risk; edit the SoA | Manager (`requirement.write`) |
| Approve, reject and revoke waivers; accept a residual risk | Manager (`waiver.approve`) |
| Change the project's risk acceptance threshold | Manager (`project.update`) |

## Waivers and deviations

**Waivers** in the project sidebar lists every waiver and deviation with its type, status, what it applies to (a release or the whole project), its requirements and its expiry date. Filter by text, status, type and release, or tick **In force only**. The figures above the list show how many waivers are in force, how many await approval and how many expire within 30 days.

To request one, select **New waiver** (or **Request a waiver** in a requirement or on the status tab of a release) and fill in:

- **Type**: *Waiver* (the requirements are failed, untested or not fulfilled, and that is accepted for now) or *Deviation* (they are met by a different solution).
- **Applies to**: one release, or the whole project.
- **Requirements**: the requirements it accepts.
- **Justification**, **Compensating measures** (for a deviation: the solution chosen instead) and **Conditions**.
- **Linked risk**: an existing risk, or **Create a new risk** with a likelihood and impact; the new risk is linked to the same requirements.
- **Expires**: the last day the waiver counts. Leave it empty for no expiry date.
- **Approvers**: the e-mail addresses of the people who must all approve it. Leave it empty to let one person with the permission `waiver.approve` decide.

A new waiver is a **draft**. **Request approval** makes it **requested** and notifies the approvers. Until it is decided you can still edit it or **Withdraw** it back to a draft.

### Approve or reject

Open a requested waiver and select **Approve or reject**. Choose the decision, add a comment (required for a rejection) and confirm with your password: this is an **electronic signature** over the exact content of the waiver, like the [sign-off of a baseline](baselines.md). If the waiver changes afterwards, earlier signatures no longer count and are shown as *not valid*. You cannot decide a waiver you requested yourself, and API tokens cannot sign.

The waiver is **approved** when every designated approver has approved it (or, without approvers, after one approval) and **rejected** at the first rejection. The requester and the approvers are notified.

### What an approved waiver does

While a waiver is approved and its expiry date has not passed, it is **in force**:

- **Release readiness**: the [exit criteria](releases-and-risks.md#exit-criteria) treat its requirements as accepted. *Coverage* and *Passed* count them as met, *MUST and SHALL requirements passed* does not list them, and a failed test of only waived requirements does not count for *No failed tests*. They are listed separately under each criterion as *waived*, and the **Status** tab of the release lists the waivers in force.
- **Analytics and compliance**: requirements under a project-wide waiver are shown separately (the finding *Requirements accepted by a waiver*, and *Waived* in the compliance analysis) and are not counted as gaps or as fulfilled.
- **Reports and documents**: the compliance report and the release readiness report have a section on waivers, and the [requirements specification](documents.md) marks waived requirements (**Waived by**) and lists the waivers in an appendix.

A requirement shows its waivers below its risks in the requirement editor.

In a project with [variants](variants-and-code.md), a waiver can be limited to variants with **Only for variants**. In a release that targets variants it then accepts its requirements only in those variants and the variants below them; a release evaluated without variants counts only waivers for every variant (see [release gates per variant](variants-and-code.md#release-gates-per-variant)).

### Expiry and revocation

The day after its expiry date, an approved waiver becomes **expired**: its requirements are no longer accepted, and the requester and approvers are notified. An approver can also **Revoke** an approved waiver at any time, with a reason. An approved waiver cannot be edited or deleted; revoke it and request a new one instead. Every step is recorded in the audit log.

## Residual risk

The [risk register](releases-and-risks.md#the-risk-register) scores each risk by likelihood × impact. Under **Residual risk after mitigation** in the risk dialog you enter the likelihood and impact you expect once the mitigation works:

- The residual score takes effect when the mitigation is **verified**: every requirement linked to the risk has passed all its tests. Until then the inherent score still applies.
- Tick **Enter the residual score by hand** and give a **justification** to make it take effect at once, for example when the mitigation is organisational.
- **Review date** and **Review owner** plan the next review; a risk whose review date has passed is marked *Review due*.

The **Residual** column of the register shows the score and level in effect, with an icon for a verified or hand-entered score. Switch the heat map between **Before mitigation** and **After mitigation**; selecting a cell filters the register on that view.

### Risk acceptance threshold and accepting a risk

The **Risk acceptance threshold** below the heat map is the highest residual level the project accepts without a sign-off (by default *Medium*). Risks whose residual level is above it are flagged with a warning icon.

To accept such a risk, open it and select **Accept risk** *(waiver.approve)*: confirm with your password to sign. The risk becomes *Accepted*. The signature covers the residual assessment as it is: when the assessment changes, the acceptance is shown as *out of date* and no longer counts. **Withdraw acceptance** removes it.

A release flags the risks of its scope whose residual level is above the threshold on its **Status** tab. Add the exit criterion **Residual risk** to make them block the release unless they are accepted; the criterion can use the project's threshold or a level of its own. The [risk report](reports-and-export.md) shows the heat map before and after mitigation and lists the risks above the threshold.

## Statement of Applicability

**SoA** in the project sidebar lists every control of a reference source. Choose the source at the top: **ISO/IEC 27001:2022 Annex A** comes with its 93 controls (A.5.1–A.5.37, A.6.1–A.6.8, A.7.1–A.7.14 and A.8.1–A.8.34, with their short names). For any other law or standard of the [catalogue's sources](catalogue.md), the controls are the clauses that the catalogue and your requirements cite.

For each control the table shows:

- **Applicable**: *Applicable*, *Excluded* or *Not decided*, with the **Justification** for including or excluding it (required for an exclusion) and the **Owner**;
- **Implementation**, derived from the requirements that cite the control: *Implemented* (all of them passed their tests), *Partially implemented* (some passed, or are accepted by a waiver), *Planned* (requirements exist, none passed yet) or *Not implemented* (no requirement cites it);
- the **Requirements** and the **Evidence** (the tests that verify them).

A requirement cites a control when it was added from a catalogue requirement whose references name the control (for example `ISO/IEC 27001:2022 A.8.5`), or when its own **Source** field names it.

The table works like the [spreadsheet view](requirements.md): type in the **Applicable**, **Justification** and **Owner** cells, paste a block from a spreadsheet or fill down with <kbd>Ctrl</kbd>+<kbd>D</kbd>. Each change is saved at once. Press <kbd>Ctrl</kbd>+<kbd>Enter</kbd> on a control (or double-click it in the first column) to see its requirements and evidence. Filter by text, applicability, implementation and group; the figures above the table count the controls.

**Download** creates the statement as PDF, Word, Excel or Markdown; **Save to archive** stores it in the [document archive](documents.md). It is also the standard report **Statement of Applicability** on the **Reports** page.
