# Interoperability: ReqIF and OSLC

Qtestframe works together with requirements and ALM tools such as IBM DOORS and DOORS Next (IBM Engineering Lifecycle Management), Siemens Polarion, Codebeamer, Jama and ProR in two ways:

- **ReqIF files** move whole specifications between tools: requirements, test cases, folders, relations, trace links and attachments. You export a file here and import it there, or the other way round, as often as you like.
- **OSLC** connects the tools live: another tool finds Qtestframe's requirements and test cases, links its own items to them, and can create and change them without a file.

This page explains both. For the Excel round-trip and the other export formats see [Reports and export](reports-and-export.md#exchanging-data-with-other-tools); your administrator connects the tools as described in the administrator guide (*Connecting other tools*).

## ReqIF export

**Export** (in the project) offers **ReqIF** and **ReqIF (with attachments)**. A ReqIF file always contains the requirements with their folders (as chapter headings), custom fields, acceptance criteria, D3FEND and EMB3D ids and relations; importing the file reads the D3FEND and EMB3D attributes back. Under **ReqIF content** you add more:

| Option | What it adds |
|---|---|
| **Test cases with their steps and trace links** | Every test case as an object of the type *Test case* (title, description, preconditions, the steps as a table, expected result, execution type, priority, status, estimate, tags, D3FEND and EMB3D, custom fields), the test folders as a second chapter tree, and a *Verifies* relation from each test to each requirement it verifies. |
| **Verification records** | Inspections, analyses, demonstrations and reviews recorded for the requirements, each related to its requirement (*Verification record of*). They are for reading in the other tool; importing the file ignores them. |
| **Version** | *Current state*, or one of the project's [baselines](baselines.md): the requirements and tests exactly as the baseline froze them. The filter does not apply to a baseline; relations, links and records are those of today between its items. |

The filter limits the requirements as for the other formats; with test cases, only the tests that verify an exported requirement are included. **ReqIF (with attachments)** is a `.reqifz` archive with the attachments and the images of descriptions as files.

The file uses the ReqIF standard names (*ReqIF.Name*, *ReqIF.Text*, *ReqIF.ChapterName*, *ReqIF.ForeignID*) that DOORS, DOORS Next, Polarion and Codebeamer recognise, and value lists (enumerations) for obligation, type, priority, status and select custom fields. Every object keeps the same identifier in every export, so a tool that imported it before updates its copy.

## ReqIF import

**Import → ReqIF from DOORS or Polarion, with updates** (in the requirements or tests list) reads a `.reqif` or `.reqifz` file and shows what would change before anything is saved *(Manager)*.

1. **Choose the file.** The preview shows the document's object types, attributes and relation types with what Qtestframe makes of them, and the changes.
2. **Object types.** Each type of object in the document becomes **Requirement**, **Test case**, **Heading (folder)** or is ignored. Qtestframe's own *Test case* type and types with *test* in their name are guessed as test cases, everything else as requirements; choose differently for the types of the other tool (for example DOORS Next *Test Case* artifacts, Polarion *Test Case* work items or Codebeamer *Test Case* trackers). Importing test cases needs the permission to write tests.
3. **Attributes.** For requirements and for test cases separately, each attribute is read into the field shown: built-in fields and your project's [custom fields](requirements.md#custom-fields) (matched by their label; value lists match by option name). **Test steps** reads a table (action, data, expected result) or a list. **Heading (folder)** marks the chapter names, **ID in the other tool** is kept in the source of new requirements. Choose another field or *ignore*; the preview updates at once.
4. **Relation types.** Each relation type becomes a relation type of Qtestframe (*refines*, *depends on* …), a **trace link** (a test verifies a requirement) or is ignored. A relation between a test case and a requirement is always a trace link, whichever direction it has.
5. **Where the items go.** Choose the target folders for requirements and for test cases, and whether **headings become folders**. With **Requirements nested below a requirement refine it**, a requirement that sits below another requirement in the document's hierarchy (as in DOORS) gets a *refines* relation to it.
6. **Check the changes** for requirements and for test cases (new, changed, unchanged, conflicts, errors, cell by cell), the relations and trace links to add or remove, and the embedded files, and choose **Save**. Everything is saved in one go, or nothing.

What happens:

- **Formatted text** (XHTML) becomes Markdown with its paragraphs, lists, tables, bold and italic text and links.
- **Embedded files** of a `.reqifz` (images and other objects in the text, the *Attachments* of a Qtestframe export) become attachments of the item, and the text shows the image from the attachment. A file the item has already is not attached again.
- **Importing the same document again** updates the requirements, test cases, relations and trace links it created and adds new ones. An item changed in Qtestframe since the last import shows as a conflict when the document has other values. Relations and trace links removed in the other tool are removed here as well; those you made in Qtestframe stay. An import never deletes items.
- A test step that calls a test case which is new in the same import is imported as a plain step; import the document once more to make it a call.

### Round trip

A ReqIF file exported from Qtestframe with test cases can be imported into another project (or another Qtestframe) and holds the same items afterwards: requirements and test cases with their fields, custom fields (define the same custom fields in the target project first), criteria, steps, folders, relations, trace links and attachments. The original keys are kept as the ID in the other tool; the items get keys of the target project. Importing the file into the **same** project matches every item, so you can send a specification to a supplier and import their edited version back with the same preview.

## OSLC

OSLC (Open Services for Lifecycle Collaboration) is the standard ALM tools use to link items across tools. Qtestframe is an OSLC provider for **Requirements Management 2.1** (requirements) and **Quality Management 2.1** (test cases). Once your administrator has connected a tool, its users can:

- **Pick** Qtestframe requirements or test cases in a small selection window inside their tool, search by text, and link to them: for example link a DOORS Next requirement to the Qtestframe test case that validates it, or a Polarion work item to the requirement it implements.
- **Create** a requirement or test case in Qtestframe from their tool, in a short creation window, and link it in the same step.
- **Read** the items with their properties and links (which tests verify a requirement, which requirements it refines, which work items implement it), search them, and follow the links to the Qtestframe page.
- **Change and delete** items through OSLC, with the same rules as in Qtestframe: validation, review permissions, history and the audit log. When someone changed the item in the meantime, the tool is told to read it again instead of overwriting the other change.

The selection and creation windows use your Qtestframe session in the browser. If you are not signed in, they offer a link to sign in in another tab; then choose **Try again**.

**Links.** OSLC links map to Qtestframe's own links:

| OSLC link | In Qtestframe |
|---|---|
| *validatedBy* (requirement), *validatesRequirement* (test case) | trace link: the test verifies the requirement |
| *elaborates* / *elaboratedBy* | relation *refines* |
| *constrainedBy* / *constrains* | relation *depends on* |
| *satisfies* / *satisfiedBy* | relation *derived from* |
| *relation* | relation *relates to* |
| *implementedBy*, *trackedBy*, *affectedBy*, *testsChangeRequest* and links to items of the other tool | an [item link](requirements.md#links-to-other-items) to the item's address, labelled with the link name |

Each tool connects with an [API token](administration.md#api-tokens) of a person or a service account; the tool can do what the token's role allows and sees only the projects that person may see.

## Which one to use?

| You want to … | Use |
|---|---|
| Hand a whole specification to a supplier or customer, or take theirs | ReqIF export and import |
| Keep a specification maintained in DOORS, DOORS Next, Polarion or Codebeamer up to date in Qtestframe | ReqIF import, again after every change there |
| Copy a project with its tests to another Qtestframe | ReqIF with test cases (or [Save and load projects](reports-and-export.md#save-and-load-projects)) |
| Freeze what was handed over | ReqIF export of a baseline |
| Link items of an ALM tool to requirements and tests while you work | OSLC |
