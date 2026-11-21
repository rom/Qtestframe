# Catalogue

The **catalogue** is a built-in library of standard requirements and tests for software development: security, privacy and compliance, performance and reliability, operations and maintainability, usability and accessibility (also forms, preferences and mobile apps), common functionality (also workflow and approvals, collaboration, files, scheduling) and domain-specific functionality such as e-commerce checkout, billing, industrial automation and control systems, electrical energy, and oil and gas. Instead of writing *"Passwords must be at least 12 characters"* and its test for every project, you choose the items that fit and add them to the project in one go, with their tests and trace links.

Browsing the catalogue needs permission to read requirements or tests (every role has it). Adding requirements needs **Manager** (`requirement.write`), adding tests **Tester** (`test.write`).

## Where the texts come from

The catalogue texts are written for Qtestframe, in English and Swedish. Each item is based on recognised standards and good practice, such as OWASP ASVS and the OWASP Top 10, WCAG 2.2, ISO/IEC 25010, NIST SP 800-63B, the GDPR, ISO/IEC 27001, PCI DSS and the EU AI Act, and it **cites** them under **Based on** by name and section. The catalogue does not copy the text of any standard: read the cited standard when you need its exact wording.

Every requirement is one verifiable statement with exactly one obligation keyword (MUST, SHOULD, MAY … in English; SKA, BÖR, FÅR … in Swedish, see [Requirements](requirements.md)), evaluation criteria and a rationale. Every test has preconditions, numbered steps with an expected result each and an overall expected result. Values that a team usually adjusts (12 characters, 2 seconds, 99.9 %) are part of the text, so you can change them after adding the item.

Every requirement lists at least one catalogue test that verifies it, and every test verifies at least one requirement, so adding a requirement with its tests always gives you a way to check it. Every item is complete in both English and Swedish, with the same criteria and steps in both languages.

## Browse and search

Open **Catalogue** in the project sidebar, or select **Add from catalogue** at the top of the requirement or test list.

- Switch between **Requirements** and **Tests** at the top.
- Choose **English** or **Svenska** for the language of the texts. The items are added in this language.
- On the left, **Groups and areas** lists the catalogue's groups (for example *Security*) and their areas (for example *Authentication*) with the number of items that match the current search and filters. Select **Everything** to see all areas. Use the arrow keys to move in the tree and Enter to select. On a narrow screen the tree opens with the **Areas** button.
- The **search** field finds items whose title, ID, tags, cited standards or description contain all the words you type. The best matches come first: an exact ID such as `AUTH-01`, then the words in the title, then the other fields.
- Narrow the list with **Type**, **Obligation** and **Priority** (requirements), **Execution** (tests) and **Tags**. **Hide items already in the project** leaves out what you have added before.

Each row shows the ID, the title, the obligation and priority, and how many catalogue tests verify the requirement (or how many requirements the test verifies). **In project** marks items that the project already has; hover over it to see their keys.

## Preview an item

Select a row to see the full text on the right (on a phone it opens as a panel): the description with the obligation keyword highlighted, the rationale, the evaluation criteria with their thresholds, the cited standards and the catalogue tests that verify the requirement. For a test you see the preconditions, the steps, the expected result and the requirements it verifies. Select an ID under *Verified by* or *Verifies* to jump to that item.

## Add items to the project

1. Tick the items you want. **Select all shown** ticks everything the list shows, so to take a whole area, select the area in the tree first. You can tick requirements and tests on both tabs; the bar at the bottom counts them.
2. Select **Add N requirements (+ M tests)**. M includes the tests that verify the chosen requirements.
3. In the dialog, choose:
   - **Language** of the texts.
   - **Folder for the requirements** and **Folder for the tests** (top level by default).
   - **Subfolders**: none, *one folder per area* (for example *Authentication*) or *group and area folders* (*Security / Authentication*). Existing folders with the same name are reused.
   - **Status** of the requirements and the tests (*Draft* by default, so they are reviewed like other new items).
   - **Source of the requirements**: the cited standards (default) or one of the project's sources.
   - **Also add the tests that verify the chosen requirements** (on by default).
   - **Add copies of items already in the project**: off by default, so items the project already has are skipped.
4. The summary shows how many requirements and tests will be added and how many are skipped. Select **Add**.

At most 500 items can be added at once. The import is all or nothing: if something fails, nothing is added.

Qtestframe creates the requirements and tests as if you had written them, with a first history version, and creates **trace links** between each requirement and the tests that verify it, also to matching items that were added to the project earlier. The automatic linked draft test that a new requirement usually gets is not created: the catalogue tests take its place.

## After adding

A box at the top of the page sums up what was added. **Open the added requirements** and **Open the added tests** open the lists filtered to the new items. **Undo** (or Ctrl+Z) moves them to the trash; redo brings them back with their links. Folders that the import created stay.

The added items are ordinary project items: edit the texts and values to fit your system. The editor shows **From the catalogue: AUTH-01 · …** with the cited standards, and **Show in the catalogue** opens the original entry. This origin is kept when you edit, copy, move or restore the item. In filters, `catalog:AUTH-01` finds the items copied from one entry, `catalog:AUTH-*` those of an area and `has:catalog` every item from the catalogue (see [Search and filters](search-and-filters.md)).

## Tips

- Start a new project from the catalogue: add the groups that apply, then delete or adapt what does not fit.
- Use the **Draft** status and a [bulk review](requirements.md) to agree on the adapted items as a team.
- The catalogue is part of Qtestframe and grows with new versions. Items you have added do not change when the catalogue changes.
