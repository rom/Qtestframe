# Duplicates and conflicts

Large requirement sets collect the same requirement twice (written by two people, imported from a customer document and from the [catalogue](catalogue.md)) and requirements that contradict each other (*passwords at least 12 characters* and *at most 8*). Qtestframe looks for both: while you write, in the [built-in consistency checks](traceability-and-coverage.md#built-in-consistency-checks) and on the **Duplicates & conflicts** page, where you decide what to do about each pair.

Everything is computed on the Qtestframe server. No text leaves it unless an administrator has turned on an [embeddings service](#embeddings-service).

## Similar requirements while writing

When you create or edit a requirement, **Similar requirements** below the description shows the five most similar requirements of the project, each with its similarity in percent, a moment after you stop typing. A similarity at or above the project's duplicate threshold (68 % by default) is highlighted: the requirement probably exists already. Open it in a new tab, or keep writing if yours says something else.

The test editor has the same panel, **Similar tests**, which compares the title, description, preconditions and steps.

Rejected and obsolete requirements and deprecated tests are not compared. The panel only appears for people who may change the item.

## How similarity is measured

Two requirements are similar when they use the same important words, not when they look alike letter by letter:

- **Normalisation and word forms.** Texts are compared in lower case without punctuation; words are reduced to their stem in English and Swedish (*users*, *user* → *user*; *användaren*, *användarna* → *använd*), so inflections do not matter.
- **Stop words.** Words without meaning of their own (*the*, *shall*, *och*, *ska* …) are ignored. The obligation keywords are compared by the conflict detection instead.
- **Synonyms and abbreviations** of the domain count as the same word, also across the two languages: *log in*, *sign in*, *logga in* and *inloggning*; *user*, *end user*, *användare* and *brukare*; *e-mail*, *e-post* and *mejl*; *2FA*, *two-factor* and *tvåfaktorsautentisering*; *delete*, *remove*, *radera* and *ta bort*, and more.
- **Compounds.** Swedish compounds are split when a part is a known word (*användarkonto* = *användare* + *konto*), and pieces of four letters make *lösenordsbyte* and *byte av lösenord* similar.
- **Rare words count more.** A word that appears in a few requirements of the project says more than one that appears in most of them (TF-IDF over the project).
- **Field weights.** The title counts most, then the description, then the acceptance criteria (for tests: preconditions, steps and expected results).

The result is a similarity from 0 to 1 (shown as a percentage). Texts that are the same apart from case and punctuation are 100 %.

## Potential conflicts

Two requirements **potentially conflict** when they talk about the same thing (they share enough of their important words) and:

| Kind | Example |
|---|---|
| **Opposite obligation** | *The user MUST be able to export reports as PDF.* and *The user MUST NOT be able to export reports as PDF.* (also *SKA* and *FÅR INTE*, *must* and *must never*) |
| **Thresholds** that cannot both hold | *respond within 2 seconds* and *respond in no less than 5 s*; *at least 12 characters* and *at most 8 characters*; *minst 12 tecken* and *högst 8 tecken*. Units are converted (2000 ms = 2 s, 15 minuters = 900 s), and a negative obligation turns the comparison round (*must not exceed 2 s* = at most 2 s) |
| **Exclusive values** | *enabled* and *disabled* by default, *mandatory* and *optional*, *encrypted* and *unencrypted*, *public* and *private*, *automatic* and *manual*, *aktiverad* and *avstängd* … |

Each conflict has a **confidence** from 0 to 1, higher when the two statements share more of their content. The obligation keywords are recognised as in the [language analysis](requirements.md) (English and Swedish). A pair that conflicts is not also reported as a duplicate.

## The Duplicates & conflicts page

Open **Duplicates & conflicts** in the project sidebar. Choose **Requirements** or **Tests** at the top.

- **Potential conflicts** (requirements) shows each pair with its kind, confidence and the decisive terms, and the sentence of each requirement with the **evidence highlighted**: the keywords, the thresholds or the exclusive values.
- **Suspected duplicate requirements** (or tests) shows each pair with its similarity and the number of trace links of each item.

**Similarity from** and **Conflict confidence from** change what is listed; they start at the project's thresholds (the built-in checks). **Show dismissed** lists the pairs you dismissed too, so you can show them again.

For each pair you can:

| Action | What happens |
|---|---|
| **A duplicates B** | adds the relation *A duplicates B* (for tests: an item link labelled *duplicates*). The pair is no longer suspected. |
| **Merge…** | choose the item to **keep**; the other one is merged into it: its trace links, relations (tests: prerequisites) and item links move to the item you keep, it gets the status *Obsolete* (tests: *Deprecated*) and is marked as a duplicate of the kept item. Both get a comment that records the merge, and the status change appears in the history. Links the kept item already has are not doubled. |
| **Mark as conflicting** | adds the relation *conflicts with*. From then on the check [Conflicting requirements in a release](traceability-and-coverage.md#built-in-consistency-checks) watches the pair. |
| **Not a duplicate** / **Not a conflict** | dismisses the finding: the pair is not shown again by the page or the built-in checks (it is remembered per pair and check, also in backups and project archives). **Show again** undoes it. |

Marking and merging need permission to change requirements (`requirement.write`) or tests (`test.write`). Everyone who can read the items can open the page.

## Built-in checks

The page uses the same detection as three [built-in consistency checks](traceability-and-coverage.md#built-in-consistency-checks): **Suspected duplicate requirements**, **Potentially conflicting requirements** and **Suspected duplicate tests**. A violation is reported on the newer item of the pair; its fixes are **Mark as duplicate**, **Mark as conflicting** and **Not a problem**.

## Thresholds

| Setting | Default | Where |
|---|---|---|
| Similarity of a suspected duplicate | 0.68 | the rule of the check (**Similarity threshold**, 0.5 to 1) |
| Confidence of a potential conflict | 0.6 | the rule of the conflict check (**Least confidence**, 0.5 to 1) |
| Least similarity shown while writing | 0.25 | fixed |

The defaults were chosen on a labelled set of English and Swedish requirements (paraphrased duplicates, contradictions of each kind, and related requirements that are neither, such as *lock the account* and *unlock an account*): with them every labelled duplicate and conflict is found and no unrelated pair is reported. Raise the duplicate threshold to about 0.8 when the check reports requirements that only share a topic; lower it towards 0.6 to find paraphrases with fewer common words. Similarity depends on the project: a word that is rare in a small project counts more than in a large one.

## Importing from the catalogue

Before you add requirements from the [catalogue](catalogue.md), the dialog warns about the chosen requirements that are similar to requirements the project already has and can leave them out. After adding, the result says how many new requirements are similar to existing ones.

## Embeddings service

An administrator can let an external **embeddings service** (an OpenAI-compatible `/v1/embeddings` endpoint) compute the similarity instead (**Administration → System settings → Similarity**; see the administrator guide). The page and the panel then say *Embeddings service*. The title, description and criteria or steps of requirements and tests are then **sent to that service**. Its vectors are cached per item version, so unchanged items are not sent again. The conflict detection always runs on the server. When the service fails, the local similarity is used.
