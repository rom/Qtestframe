# Catalogue data format

The standard requirement and test library lives in `data/*.yaml`, one file
per **area** (a topic such as authentication or performance). Every file is
embedded into the binary and checked when the catalogue loads (run
`go test ./internal/catalog` after each change): the format above and the
keyword, analysis, traceability and language rules below are enforced.

```yaml
area: authentication            # unique area id: lower case, a-z 0-9 and "-"
prefix: AUTH                    # id prefix of the area's items: 2-6 capitals
group: security                 # one of the groups in groups.yaml
catalogue: software             # optional: a catalogue type of catalogues.yaml (default software)
name: {en: Authentication, sv: Autentisering}
description: {en: "…", sv: "…"} # one or two sentences

requirements:
  - id: AUTH-01                 # <prefix>-NN, unique in the catalogue
    type: security              # functional | non_functional | security | performance | usability | interface | constraint | business | regulatory
    obligation: must            # must | must_not | should | should_not | may (the keyword used in the text)
    priority: high              # critical | high | medium | low
    tags: [password, login]     # lower case, a-z 0-9 and "-"
    refs: ["NIST SP 800-63B-4 §3.1.1.2 (password verifiers)"]   # standards it is based on: cited, never copied
    tests: [AUTH-T01, AUTH-T02] # at least one catalogue test that verifies it (same or other area)
    d3fend: [D3-SPP]            # optional: MITRE D3FEND defensive techniques it implements (data/d3fend.yaml); see "MITRE D3FEND and EMB3D"
    emb3d: []                   # optional: MITRE EMB3D mitigations (MID-nnn) and threats (TID-nnn) it addresses (data/emb3d.yaml)
    en:
      title: Minimum password length          # short, no final full stop
      description: The system MUST reject a new password shorter than 12 characters.
      rationale: Short passwords are guessed quickly by offline attacks.
      criteria:                 # evaluation criteria (at least one)
        - method: test          # test | inspection | analysis | demonstration | review
          text: A password of 11 characters is rejected and one of 12 is accepted.
          threshold: ""         # optional measurable limit, e.g. "≤ 2 s for 95 % of requests"
    sv:
      title: Minsta lösenordslängd
      description: Systemet SKA avvisa ett nytt lösenord som är kortare än 12 tecken.
      rationale: Korta lösenord gissas snabbt vid offlineattacker.
      criteria:
        - method: test
          text: Ett lösenord med 11 tecken avvisas och ett med 12 tecken godtas.

tests:
  - id: AUTH-T01                # <prefix>-TNN, unique in the catalogue
    priority: high
    execution: manual           # manual | automated (automated: the steps describe what a script checks)
    tags: [password]
    refs: []
    d3fend: [D3-SPP]            # optional: the D3FEND techniques whose implementation the steps verify
    emb3d: []                   # optional: the EMB3D mitigations and threats the steps check
    en:
      title: Password shorter than the minimum is rejected
      description: Checks the minimum password length when a password is set.
      preconditions: A signed-in user on the change-password page.
      expected: Only passwords of at least the minimum length are accepted.   # overall expected result
      steps:                    # at least one; data is optional
        - action: Enter a new password of 11 characters and save.
          expected: The password is rejected with a message stating the minimum length.
          data: "Abcdefgh1!x"
        - action: Enter a new password of 12 characters and save.
          expected: The password is accepted.
    sv: { … the same fields in Swedish … }

risks:                          # optional: typical risks of the area
  - id: AUTH-R01                # <prefix>-RNN, unique in the catalogue
    category: security          # security | privacy | safety | availability | data | compliance | supply-chain | ai | operational | quality | project | environmental
    likelihood: 5               # typical likelihood 1–5 (the project risk scale)
    impact: 4                   # typical impact 1–5; likelihood × impact gives the level as for project risks
    tags: [credential-stuffing]
    refs: ["ISO/IEC 27005:2022 §7 (risk assessment process)", "CWE-307 (improper restriction of excessive authentication attempts)"]
    mitigated_by: [AUTH-02, AUTH-07, SLOG-06]   # at least one catalogue requirement (any area) that mitigates it
    attack: [T1110.003, T1110.004]  # MITRE ATT&CK techniques (Enterprise, ICS, Mobile) of data/attack.yaml; see "MITRE ATT&CK"
    atlas: []                   # MITRE ATLAS techniques (AML.Tnnnn) of data/atlas.yaml, for attacks on AI systems
    attack_exempt: false        # true only for a security risk that no adversary technique describes
    emb3d: []                   # MITRE EMB3D threats (TID-nnn) that realise the risk on an embedded device
    tests: []                   # optional: catalogue tests that check the risk directly
    en:
      title: Accounts taken over with leaked passwords   # short, no final full stop
      description: Attackers try passwords leaked from other services … # the threat or hazard and its consequence
      mitigation: Refuse breached passwords, offer multi-factor authentication …  # the suggested treatment
    sv: { … the same fields in Swedish … }
```

## Risks

A risk describes something that typically goes wrong in the area: the
threat or hazard, what it leads to, a typical likelihood and impact on the
project scale (1–5 each; the product gives the level low, medium, high or
critical as in the risk register) and a suggested mitigation. Every risk
names the catalogue requirements that mitigate it in `mitigated_by`
(checked: at least one, each must exist, no duplicates); a requirement can
mitigate risks of any area. When a team adds a risk to a project, the
mitigating requirements and their tests can come along, linked to the
project risk.

- **Write the risk, not the control.** The description says what happens
  and to whom ("an attacker who is signed in changes the record identifier
  … confidential data leaks"); the mitigation summarises what the
  requirements in `mitigated_by` do. Keep the typical values honest: they
  are a starting point for the team's own assessment.
- **Cite the risk sources** in `refs` like for requirements: the risk
  management standard of the domain (ISO 31000, ISO/IEC 27005, ISO/IEC
  23894, ISO 14971, ISO 26262-3, ISO 12100, IEC 62443-3-2, EU AI Act
  Art. 9, GDPR Art. 35), threat catalogues (OWASP Top 10, ENISA Threat
  Landscape) and the CWE weaknesses of the mitigating requirements. Every
  ref must match a source of the registry. ATT&CK and ATLAS techniques go
  in `attack` and `atlas`, not in `refs` (checked).
- **3 to 8 risks per area** (checked): the typical ones, not every
  conceivable failure.
- **Both languages complete** (checked): title, description and mitigation
  in English and Swedish; no full stop at the end of the title.

## MITRE ATT&CK

`data/attack.yaml` is the list of the MITRE ATT&CK tactics, techniques,
sub-techniques and mitigations of the Enterprise, ICS and Mobile matrices
(id, name, matrix, tactics, url), without revoked or deprecated objects.
It is generated from the ATT&CK STIX bundles; never edit it by hand:

```sh
go run ./internal/catalog/cmd/attackgen -out internal/catalog/data/attack.yaml \
    enterprise-attack.json ics-attack.json mobile-attack.json
```

`data/atlas.yaml` has the same format for the MITRE ATLAS techniques that
risks use (a curated subset, kept by hand: add a technique with its id,
name and tactics exactly as on atlas.mitre.org before a risk lists it).
After a new ATT&CK release, regenerate the list and run the tests: every
id that was revoked or deprecated fails where a risk or a ref uses it.

A catalogue risk lists the adversary techniques that realise it:

- **`attack`**: ATT&CK techniques and sub-techniques (`T1110.004`,
  `T0886` for ICS, `T1409` for Mobile). Prefer the most specific
  sub-technique; list a parent only when no sub-technique fits, never a
  parent together with one of its sub-techniques (checked). Typically 1–3.
- **`atlas`**: ATLAS techniques (`AML.T0051.001`) for attacks on AI
  systems, models and their data (prompt injection, poisoning, model
  theft). An AI risk can list both.
- **Who needs a mapping** (checked): a risk of category `security` lists at
  least one technique in `attack` or `atlas`, unless `attack_exempt: true`
  says that no adversary technique describes it (a lost access card, a
  repeated incident). Risks of the categories `safety`, `environmental`,
  `quality`, `project` and `compliance` are hazards, failures or
  obligations, not attacks: they list none. Risks of the other categories
  (`privacy`, `ai`, `availability`, `data`, `supply-chain`, `operational`)
  list techniques when an adversary causes them (a data breach, a
  denial-of-service attack, a poisoned dependency) and none when they come
  from failure or error (an outage, a lost backup, a model that drifts).
- **Ids are checked** against the lists: unknown, revoked and deprecated ids
  fail, an ATT&CK id in `atlas` or the reverse fails, and so does an
  ATT&CK technique in `refs`. The ATT&CK ids in the `refs` of requirements
  and tests (`MITRE ATT&CK T1110.001 (password guessing)`,
  `MITRE ATT&CK M1032 (…)`) must also be in the list.

The catalogue shows the techniques of a risk with names, tactics and links,
filters risks by technique and tactic, and the ATT&CK coverage page counts
per tactic the risks that list one of its techniques and the requirements
that cite one or mitigate such a risk. A risk added to a project keeps its
techniques (the project risk's `attack` list).

## MITRE D3FEND and EMB3D

`data/d3fend.yaml` is the list of the [MITRE D3FEND](https://d3fend.mitre.org/)
defensive tactics and techniques (id `D3-MFA`, name, tactic Model, Harden,
Detect, Isolate, Deceive, Evict or Restore, parent technique, link) with,
for each technique, the ATT&CK techniques it counters as D3FEND infers them
through the digital artifacts both act on (only ids of `attack.yaml`).
`data/emb3d.yaml` is the list of the [MITRE EMB3D](https://emb3d.mitre.org/)
device properties (`PID-nn`), threats (`TID-nnn`, with category, the
properties that expose a device and the mitigations) and mitigations
(`MID-nnn`, with maturity). Both are generated; never edit them by hand:

```sh
go run ./internal/catalog/cmd/d3fendgen -ontology d3fend.json -api api \
    -attack internal/catalog/data/attack.yaml -out internal/catalog/data/d3fend.yaml
go run ./internal/catalog/cmd/emb3dgen -out internal/catalog/data/emb3d.yaml emb3d-stix-2.0.1.json
```

(`d3fend.json` is `https://d3fend.mitre.org/ontologies/d3fend.json`, `api/`
holds the technique pages of the D3FEND API, see the comment of
`d3fendgen`; the EMB3D bundle is `assets/emb3d-stix-<version>.json` of the
EMB3D repository.) Regenerate `d3fend.yaml` after `attack.yaml`.

- **`d3fend`** (requirements and tests): the defensive techniques the
  requirement implements or whose implementation the test verifies.
  Precision over recall: list a technique only when the item clearly *is*
  that technique (*Minimum password length* → `D3-SPP` Strong Password
  Policy; *Secure boot verifies each stage* → `D3-BA` Bootloader
  Authentication; *Debug ports locked* → `D3-IOPR` IO Port Restriction), not
  one it merely supports. Prefer the most specific technique; never a
  parent together with one of its sub-techniques (checked). Typically 1,
  at most 8 (checked). Organisational items (training, contracts, reviews)
  list none.
- **`emb3d`** (requirements, tests and risks): for items about embedded
  devices (the hardware catalogue, industrial control, IoT and OT areas).
  Requirements and tests list the EMB3D **mitigations** (`MID-026` Secure
  Firmware Update) they implement or check and, where no mitigation fits,
  the **threats** (`TID-210` Device Vulnerabilities Unpatchable) they
  address; risks list the **threats** that realise them (a mitigation in
  a risk fails). Risks of the categories that list no ATT&CK technique
  (safety, environmental, quality, project, compliance) list no threat
  either (checked). The area `embedded-security` is mapped throughout: every
  risk lists threats and most requirements an EMB3D id (checked by the
  tests).
- **Ids are checked** against the lists: unknown and duplicate ids fail.
  D3FEND and EMB3D ids go in these fields, never in `refs` (a ref that
  starts with `MITRE D3FEND` or `MITRE EMB3D` fails).

The catalogue shows the techniques, mitigations and threats with names and
links, filters by them (`d3fend=`, `emb3d=`; a threat also finds the items
with one of its mitigations), and shows for a risk its **defenses**: per
ATT&CK technique the D3FEND techniques that counter it and the
requirements that list one, a technique no mitigating requirement counters
being a gap. The framework coverage page has a D3FEND tab (techniques per
tactic, risks fully countered) and an EMB3D tab (threats per category with
the risks, requirements and tests that address them). Imported items keep
their `d3fend` and `emb3d` lists.

## Writing rules

- **Original text.** Base items on standards and good practice (OWASP ASVS,
  OWASP Top 10, WCAG 2.2, ISO/IEC 25010, NIST SP 800-63B, GDPR, ISO/IEC 27001
  Annex A and the rest of the ISO/IEC 27000 series, IEC 62443, the EU
  regulations listed below, ISTQB, …) but write every sentence yourself and
  cite the standard in `refs` by name and section only. Never copy
  normative text. Cite only what the item really implements, keep a `refs`
  list to at most 10 entries (checked; no duplicates), and spell each
  source the same way everywhere (see *Citing standards* below). Aim for
  about 8 standards; the last places are meant for the CWE weaknesses and
  MITRE ATT&CK techniques of a security item.
- **Generic and reusable.** Write "the system", "a user", "an administrator";
  no product names. Put the adjustable value (12 characters, 2 seconds,
  99.9 %) in the text so a team can edit it after adding the item.
- **One verifiable statement** per requirement, with exactly one obligation
  keyword in upper case: English MUST / MUST NOT / SHOULD / SHOULD NOT / MAY
  (RFC 2119), Swedish SKA / SKA INTE / BÖR / BÖR INTE / FÅR (SIS convention).
  `obligation` must match the keyword (SKA counts as must). The requirement
  analysis (internal/reqlang) must find no errors or warnings: avoid vague
  words such as "fast", "user-friendly", "etc.", "and/or", "as appropriate",
  "snabbt", "användarvänlig", "osv", "vid behov". In the description,
  ordinary words that are also keywords count as a second obligation:
  "required", "optional", "recommended", "will" in English and "får",
  "måste", "kommer att", "valfri" in Swedish (write "ges", "tillåts",
  "mandatory" instead). Escape clauses ("unless", "except", "om inte",
  "utom") and vague sizes ("significant", "large", "betydande", "stora",
  "flera", "några") are warnings too.
- **Tests** are concrete and executable: preconditions, numbered actions with
  an expected result each, test data where it helps. Aim for roughly one
  test per requirement; a test may verify several requirements.
- **Traceability** (checked): every requirement lists at least one existing
  test in `tests`, and every test is listed by at least one requirement.
- **Both languages complete** (checked): every area has `name` and
  `description` in English and Swedish. Every requirement has title,
  description, rationale and criteria in both; the Swedish criteria have
  the same count and the same `method` in the same order as the English
  ones, and a `threshold` wherever English has one. Every test has title,
  description, preconditions, expected and steps in both; Swedish has the
  same number of steps, each with action and expected, and `data` wherever
  English has it. The browser and the import fall back to English for a
  missing text, but the data must never rely on it.
- **No secrets.** Never put anything that looks like a real key, token,
  password or card number in the data, not even a published example
  value: describe such test data in words ("a test card from the payment
  provider", "a known test secret").
- **Swedish** is natural, idiomatic Swedish (not word-for-word), using the
  terms of the Swedish user interface: krav, testfall, körning, organisation,
  administratör, inloggning.

## Citing standards

Use one spelling per source so that the catalogue search (which looks in
`refs`) finds every item that cites it, and every ref resolves to its
entry in the source registry (see *Reference sources*). An optional topic in brackets may
follow the clause, e.g. `"ISO/IEC 27001:2022 A.8.13 (information backup)"`.
A test cites the standards of the requirements it verifies that its steps
actually check.

| Source | Spelling in `refs` |
|---|---|
| ISO/IEC 27001 Annex A controls (2022 numbering) | `ISO/IEC 27001:2022 A.8.5` (not "Annex A 8.5") |
| ISO/IEC 27002 implementation guidance | `ISO/IEC 27002:2022 §8.5` |
| ISO/IEC 27005 information security risk management | `ISO/IEC 27005:2022 §7 (risk assessment process)` |
| ISO/IEC 27017 cloud services | `ISO/IEC 27017:2026 (segregation in virtual computing environments)` |
| ISO/IEC 27018 PII in public clouds | `ISO/IEC 27018:2025 (topic)` |
| ISO/IEC 27019 energy utility industry | `ISO/IEC 27019:2024 (topic)` |
| ISO/IEC 27031 ICT readiness for business continuity | `ISO/IEC 27031:2025` |
| ISO/IEC 27033 network security | `ISO/IEC 27033-1:2015` |
| ISO/IEC 27034 application security | `ISO/IEC 27034-1:2011` |
| ISO/IEC 27035 incident management | `ISO/IEC 27035-1:2023`, `ISO/IEC 27035-2:2023` |
| ISO/IEC 27036 supplier relationships | `ISO/IEC 27036-3:2023` |
| ISO/IEC 27040 storage security | `ISO/IEC 27040:2024` |
| ISO/IEC 27701 privacy information management | `ISO/IEC 27701:2025 (access, correction or erasure)` |
| IEC 62443-3-3 system security requirements | `IEC 62443-3-3 SR 1.7` |
| IEC 62443-4-2 component security requirements | `IEC 62443-4-2 CR 1.7` |
| IEC 62443-4-1 secure product development lifecycle | `IEC 62443-4-1 SM-9`, `SR-2`, `SI-1`, `SVV-3`, `DM-4`, `SUM-5` … |
| IEC 62443-2-1 security programme, IEC 62443-2-4 service providers | `IEC 62443-2-1:2024 (topic)`, `IEC 62443-2-4:2023 SP.07 (remote access)` |
| GDPR (Regulation (EU) 2016/679) | `GDPR Art. 32(1)(a)` |
| Cyber Resilience Act | `Regulation (EU) 2024/2847 (CRA) Annex I Part I §2(d)`, `… Annex I Part II §2`, `… Art. 14` |
| AI Act (consolidated text; mark amended provisions) | `EU AI Act (Regulation (EU) 2024/1689) Art. 4(1) (AI literacy, as amended by Regulation (EU) 2026/1744)` |
| Digital Omnibus on AI (amends the AI Act) | `Regulation (EU) 2026/1744 (Digital Omnibus on AI) Art. 1(5) (new AI Act Art. 4)` |
| DORA (digital operational resilience, financial sector) | `Regulation (EU) 2022/2554 (DORA) Art. 9(4)(c)` |
| CER (resilience of critical entities) | `Directive (EU) 2022/2557 (CER) Art. 13(1)(d)` |
| NIS2 | `Directive (EU) 2022/2555 (NIS2) Art. 23` |
| Swedish Cybersecurity Act (implements NIS2) | `Cybersäkerhetslag (2025:1506) 2 kap. 3 §` |
| OWASP ASVS (current edition 5.0; no 4.0 numbering) | `OWASP ASVS 5.0 V6.2.1 (password length)`, or the section `OWASP ASVS 5.0 V13.3 (secret management)` |
| NIST SP 800-63-4 digital identity guidelines (not the Rev. 3 numbering) | `NIST SP 800-63B-4 §3.1.1.2 (password verifiers)`, `NIST SP 800-63A-4 §3.8 (confirmation codes)` |
| PCI DSS (current version v4.0.1) | `PCI DSS v4.0.1 Req. 8.2.6`, `PCI DSS v4.0.1 SAQ A (eligibility criteria)` |
| MITRE CWE weakness (current CWE list; prefer base and variant entries, never deprecated ones) | `CWE-521 (weak password requirements)`, `CWE Top 25 Most Dangerous Software Weaknesses` |
| MITRE ATT&CK technique, sub-technique or mitigation (Enterprise, ICS or Mobile; never revoked or deprecated ids) | `MITRE ATT&CK T1110.001 (password guessing)`, `MITRE ATT&CK T0886 (remote services)`, `MITRE ATT&CK M1032 (multi-factor authentication)` |
| Radio Equipment Directive, Art. 3(3)(d)–(f) as applied by Delegated Regulation (EU) 2022/30, with the harmonised standard EN 18031-1/-2/-3 and its mechanism (AUM, ACM, SCM, SSM, SUM, RLM, CCK, CRY, GEC, DLM …) | `Directive 2014/53/EU (RED) Art. 3(3)(d) (EN 18031-1 AUM)` |

Give security items (the areas of the security group, and the security
requirements of the privacy group and the other catalogues) the CWE
weaknesses they prevent, typically 1–3 of the most specific entries, and
the ATT&CK techniques they mitigate or detect, typically 1–2; an ATT&CK
mitigation (`M1032`, `M0930` for ICS) where it is the clearer link. Use
ATT&CK for ICS for industrial control items and the Mobile matrix for
mobile apps. Check every id on cwe.mitre.org and attack.mitre.org, cite
the short name in lower case as in the examples, and add the same refs to
the tests whose steps check that weakness or technique. The catalogue
browser links each id to its page (`ref_links` of the `cwe` and
`mitre-attack` sources), and the search finds `CWE-79` or `T1110`.

Cite the RED only for items that make sense for internet-connected radio
equipment, and IEC 62443 where the item fits an industrial system or
component.

## Reference sources

`data/sources.yaml` is the registry of the laws, regulations, directives,
standards, best practices and guidelines the catalogue cites. **Every entry
of `refs` must match exactly one source**; `go test ./internal/catalog`
(`TestSourcesCoverEveryRef`) fails with the item id and the ref otherwise.
The registry lets a team pick "comply with the CRA" in the catalogue
browser and get every requirement that cites it, and lets the requirement
analytics measure compliance per source and clause.

```yaml
kinds:                          # display order of the source kinds
  - id: law                     # law | directive | standard | practice | guideline
    name: {en: Laws and regulations, sv: Lagar och förordningar}

sources:
  - id: cra                     # unique: a-z 0-9 and "-"
    kind: law                   # one of the kinds above
    jurisdiction: EU            # optional: two capitals (EU, SE, US, UK, NO …) or international
    short: CRA                  # optional short name, the same in every language
    name: {en: "Cyber Resilience Act, Regulation (EU) 2024/2847", sv: "Cyberresiliensförordningen (EU) 2024/2847"}
    match: ["Regulation (EU) 2024/2847"]   # prefixes a ref starts with (case sensitive)
    regex: []                   # optional Go regular expressions, e.g. '^OWASP .*Cheat Sheet'
    clause: ''                  # optional, see below
    clause_format: ''           # optional, see below
    url: https://eur-lex.europa.eu/eli/reg/2024/2847/oj   # the official page, https only
    links:                      # optional: the documents of a family, each with its page
      - {label: "IEC 60300-1", url: "https://webstore.iec.ch/en/publication/1293"}
      - {label: "CISPR 32 (EN 55032)", url: "https://webstore.iec.ch/en/publication/22046", match: ["EN 55032"]}
    ref_links:                  # optional: a link per cited document, derived from the ref
      - {regex: '^RFC (\d{1,5})\b', label: 'RFC $1', url: 'https://www.rfc-editor.org/rfc/rfc$1'}
```

- **Adding a source** is one entry: when a new item cites a law or
  standard that no source matches, add an entry with an id, kind, names in
  English and Swedish and a prefix that the ref starts with. Prefer a
  prefix that is the spelling of *Citing standards* (`ISO/IEC 27001`,
  `Directive (EU) 2022/2555`); list several prefixes when a source is
  cited in more than one way (`"Directive (EU) 2022/2555", "NIS2 Directive"`).
- **Exactly one.** Two sources whose patterns both match a ref are an
  error too: make the prefixes longer (`"OWASP Top 10 2021"` and
  `"OWASP Top 10 for LLM"`, not `"OWASP Top 10"`).
- **Grain.** A source is what a team would claim compliance with: a law
  with its implementing acts where they belong together (RED with
  Delegated Regulation (EU) 2022/30 and EN 18031), one entry per part of a
  standard series when the parts are used on their own (ISO/IEC 27001,
  27002, 27701 …; IEC 62443-3-3, 4-1, 4-2 …), one entry for a family of
  small specifications (IETF RFCs, W3C web standards, open source licences).
- **Clauses.** The analytics group a source's requirements by clause or
  article, derived from the ref without the matched prefix. The default
  patterns, tried in order: `Annex I Part II`, `Art. 32`, `A.8`
  (ISO/IEC 27001 Annex A), `SR 1`/`CR 2`/`SC 1`/`Req. 10` (the first
  number), `A01`/`API3`/`LLM02`, `AC`/`SM`/`WSTG-ATHZ` (an upper-case
  prefix before `-N`), `PO`/`CLD` (before `.N`), `V2`, `ch. 6`, `2 kap.`,
  `§5`; none gives the source as a whole. A source can set `clause`, a
  regular expression on the whole ref: the clause is the expansion of
  `clause_format` (`'Req. $1'`) when set, else its first capture group,
  else the whole match. The first letter is upper-cased.
- **Project text.** The compliance analysis also counts project
  requirements whose `source` field names a source: each part separated by
  `;` or a line break is matched like a ref, and failing that without
  regard to case (`gdpr art. 32`).
- **Links.** Give every source a `url`: the official or publisher page
  (EUR-Lex ELI links for EU acts, lagen.nu or riksdagen.se for Swedish
  SFS, the agency for its regulations, `iso.org/standard/<number>.html`,
  `webstore.iec.ch/en/publication/<number>`, `standards.ieee.org`, the
  `etsi.org/deliver/` folder of a deliverable, `doi.org/10.6028/…` for
  NIST and so on). Check that the page is the right document; leave the
  `url` out rather than guess. A source that groups several documents
  lists them in `links` (`label` is the designation, the same in every
  language; a ref links to the document whose `match` prefix, by default
  the label, it starts with), and a large family derives a link per cited
  document from the ref with `ref_links` (`$1` … are the capture groups of
  `regex`). Every url must start with `https://`; the loader checks it.
  The catalogue browser links each ref to the most specific of these: the
  `ref_links` document, else the `links` document, else the `url`.
- **The guide.** The section *Sources cited in the catalogue* of
  `docs/user-guide/<lang>/references.md` is generated from the registry
  and the catalogue's refs, between the markers
  `<!-- BEGIN generated sources: make refs-docs -->` and
  `<!-- END generated sources -->`. After changing the registry or the
  refs, run `make refs-docs` (or `go generate ./internal/catalog`) and
  then `make guide-docs`; `go test ./internal/catalog`
  (`TestReferencesDocIsCurrent`) fails until the pages are current.
