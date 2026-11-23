# AI assistance

Qtestframe can ask a language model service for **suggestions**: drafts of tests for a requirement, a better wording of a requirement, a summary of a failed run with defect drafts, framework mappings and acceptance criteria. You review and edit every suggestion; **nothing is saved until you accept it**.

AI assistance is optional and **off by default**. It needs a service set up by a super administrator, the organisation must turn it on, and a project can turn it off (see [Turning it on and off](#turning-it-on-and-off)).

## The Assist button

Pages that can use AI assistance have an **Assist** button with a sparkle icon:

| Where | Actions |
|---|---|
| Requirement editor | **Improve wording**, **Suggest acceptance criteria**, **Suggest framework mappings**, **Draft tests** |
| Test editor | **Draft this test from** the first linked requirement |
| Run page | **Summarise failures** |
| Risk dialog | **Suggest framework mappings** |

**Assist** opens a small menu with the actions. When an action cannot be used, it stays visible but disabled, with the reason below it: AI assistance is not set up or turned off, the feature is turned off, the item is not saved yet, the run has no failures, or you have used all requests of this hour. The menu also says which service the text goes to and how many requests you have left.

An action opens a dialog that asks the service at once. While it writes, the dialog shows how far it is; **Cancel request** stops it. Then you see the suggestions, the model, the tokens used, the time and, when the administrator entered prices, an estimated cost. **Ask again** sends the request once more.

Suggestions and the prompts are in the language of the user interface (English or Swedish).

In software projects, **AI prompts** can also send a built prompt – for example an architecture or full solution prompt – with **Send to AI assistance** and save the answer as a project document, see [Solution prompts](requirements.md#solution-prompts).

## Draft tests from a requirement

**Draft tests** in a saved requirement sends its title, description and acceptance criteria and suggests up to eight manual tests: title, description, preconditions, test data, expected result and steps with actions, data and expected results, and which acceptance criteria each test covers.

Every test can be edited in the dialog: open its details to change the texts, edit, add or remove steps. Untick the tests you do not want. **Create draft tests** creates the ticked tests with status *Draft* and links each to the requirement and to the criteria it covers. The test data is added to the preconditions. The new tests appear in the [traceability](traceability-and-coverage.md) of the requirement; review them like other drafts before you set them to *Ready*.

In the **test editor**, **Draft this test from** *KEY* asks for drafts from the test's first linked requirement. Choose one draft and **Use this draft**: its title (if the test has none), description, preconditions, expected result and steps are added to the form. Save the test to keep them.

## Improve a requirement

**Improve wording** sends the requirement's current text (also when it is not saved yet) together with the findings of the [requirement checks](requirements.md) and asks for a rewrite that follows the rules the checks look for: one obligation with an uppercase keyword (MUST, SHOULD, MAY …), an EARS pattern where it fits (*When* … *the system MUST* …), no vague terms, no TBDs, no open lists or escape clauses, active voice and measurable values.

The dialog shows:

- the **changes** as a word diff (inserted words underlined, removed ones struck through) and the explanation;
- the **findings fixed**: the server checks the rewrite again with the same checks, so the list shows what the rewrite really fixes, not what the service claims;
- the **findings that remain** and any **new findings**, and the quality score before and after.

Edit the title and text if needed, then **Use this text**: it replaces the text in the editor. It is saved when you save the requirement, so you can still compare and undo. A placeholder such as *[value]* means the service needed a value it could not know.

## Acceptance criteria

**Suggest acceptance criteria** proposes verifiable criteria with a method (test, inspection, analysis, demonstration, review) and, where the requirement implies a quality, a threshold with unit. Criteria that the requirement already has are left out. Edit or untick them and **Add criteria**: they are added to the criteria of the form.

## Framework mappings

**Suggest framework mappings** proposes ids of security frameworks with a reason for each:

- for a **requirement**: MITRE D3FEND techniques and EMB3D mitigations or threats, and CWE weaknesses;
- for a **risk**: MITRE ATT&CK and ATLAS techniques and EMB3D threats.

Every id is checked against the lists built into Qtestframe; ids that are not in them are dropped, and the dialog says how many. Ids that are already mapped are left out. **Add mappings** adds the ticked ids to the D3FEND, EMB3D or ATT&CK fields of the form; CWE ids are added to the requirement's tags (*CWE-79*), because requirements have no CWE field. CWE ids are known when the [catalogue](catalogue.md) refers to them.

## Summarise a failed run

**Summarise failures** on a run page sends the failed and blocked executions (test, notes, the end of the error output, failed steps; at most 40) and the defects linked to the run. You get a short summary, likely common causes (hypotheses from the data) and drafts of defects for failures that no linked defect covers yet.

Edit the title, description and severity of the drafts, untick what you do not want, and **Create defects**: each defect is created and linked to the run and to the executions of its tests. Then manage them as described in [Defects](defects.md).

## Privacy and what is sent

When you click an action, the text of **that item** is sent to the AI service configured by the administrator — nothing else, and nothing in the background:

| Action | What is sent |
|---|---|
| Draft tests, acceptance criteria, improve wording | the requirement's key, title, description, type, obligation, rationale and acceptance criteria (with improve: the findings of the checks) |
| Summarise failures | the run's name, environment and build, the failed and blocked executions with notes, error output and failed steps, and the linked defects |
| Framework mappings | the requirement's or risk's title and description (risks: also category and mitigation) and the ids already mapped |

Every request is recorded in the organisation's audit log (feature, user, item, tokens) without the content. The service only answers with text: it cannot change anything in Qtestframe, and the item text is passed to it as data, not as instructions. Suggestions can still be wrong or incomplete: check them like the work of a new colleague.

## Turning it on and off

- **Server:** a super administrator configures the service (Anthropic, or an OpenAI-compatible service such as Azure OpenAI or a local server) under **Administration → System settings → AI assistance** and can turn single features off (see the administrator guide).
- **Organisation:** an administrator (*settings.manage*) turns it on under **Administration → AI assistance**.
- **Project:** a project manager (*project.update*) can turn it off for one project under **Settings → Project → AI assistance**, for example when its data must not leave the server.

A limit per user and hour protects against runaway costs; the Assist menu shows how many requests are left.
