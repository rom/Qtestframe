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
name: {en: Authentication, sv: Autentisering}
description: {en: "…", sv: "…"} # one or two sentences

requirements:
  - id: AUTH-01                 # <prefix>-NN, unique in the catalogue
    type: security              # functional | non_functional | security | performance | usability | interface | constraint | business | regulatory
    obligation: must            # must | must_not | should | should_not | may (the keyword used in the text)
    priority: high              # critical | high | medium | low
    tags: [password, login]     # lower case, a-z 0-9 and "-"
    refs: ["NIST SP 800-63B §5.1.1.2"]   # standards it is based on: cited, never copied
    tests: [AUTH-T01, AUTH-T02] # at least one catalogue test that verifies it (same or other area)
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
```

## Writing rules

- **Original text.** Base items on standards and good practice (OWASP ASVS,
  OWASP Top 10, WCAG 2.2, ISO/IEC 25010, NIST SP 800-63B, GDPR, ISO/IEC 27001
  Annex A and the rest of the ISO/IEC 27000 series, IEC 62443, the EU
  regulations listed below, ISTQB, …) but write every sentence yourself and
  cite the standard in `refs` by name and section only. Never copy
  normative text. Cite only what the item really implements, keep a `refs`
  list to at most 8 entries, and spell each source the same way everywhere
  (see *Citing standards* below).
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
`refs`) finds every item that cites it. An optional topic in brackets may
follow the clause, e.g. `"ISO/IEC 27001:2022 A.8.13 (information backup)"`.
A test cites the standards of the requirements it verifies that its steps
actually check.

| Source | Spelling in `refs` |
|---|---|
| ISO/IEC 27001 Annex A controls (2022 numbering) | `ISO/IEC 27001:2022 A.8.5` (not "Annex A 8.5") |
| ISO/IEC 27002 implementation guidance | `ISO/IEC 27002:2022 §8.5` |
| ISO/IEC 27005 information security risk management | `ISO/IEC 27005:2022 §7 (risk assessment process)` |
| ISO/IEC 27017 cloud services | `ISO/IEC 27017:2015 CLD.9.5.1` |
| ISO/IEC 27018 PII in public clouds | `ISO/IEC 27018:2019 (topic)` |
| ISO/IEC 27019 energy utility industry | `ISO/IEC 27019:2024 (topic)` |
| ISO/IEC 27031 ICT readiness for business continuity | `ISO/IEC 27031:2011` |
| ISO/IEC 27033 network security | `ISO/IEC 27033-1:2015` |
| ISO/IEC 27034 application security | `ISO/IEC 27034-1:2011` |
| ISO/IEC 27035 incident management | `ISO/IEC 27035-1:2023`, `ISO/IEC 27035-2:2023` |
| ISO/IEC 27036 supplier relationships | `ISO/IEC 27036-3:2023` |
| ISO/IEC 27040 storage security | `ISO/IEC 27040:2024` |
| ISO/IEC 27701 privacy information management | `ISO/IEC 27701:2019 §7.3.6` |
| IEC 62443-3-3 system security requirements | `IEC 62443-3-3 SR 1.7` |
| IEC 62443-4-2 component security requirements | `IEC 62443-4-2 CR 1.7` |
| IEC 62443-4-1 secure product development lifecycle | `IEC 62443-4-1 SM-9`, `SR-2`, `SI-1`, `SVV-3`, `DM-4`, `SUM-5` … |
| IEC 62443-2-1 security programme, IEC 62443-2-4 service providers | `IEC 62443-2-1:2024 (topic)`, `IEC 62443-2-4 (topic)` |
| GDPR (Regulation (EU) 2016/679) | `GDPR Art. 32(1)(a)` |
| Cyber Resilience Act | `Regulation (EU) 2024/2847 (CRA) Annex I Part I §2(d)`, `… Annex I Part II §2`, `… Art. 14` |
| DORA (digital operational resilience, financial sector) | `Regulation (EU) 2022/2554 (DORA) Art. 9(4)(c)` |
| CER (resilience of critical entities) | `Directive (EU) 2022/2557 (CER) Art. 13(1)(d)` |
| NIS2 | `NIS2 Directive (EU) 2022/2555 Art. 23` |
| Radio Equipment Directive, Art. 3(3)(d)–(f) as applied by Delegated Regulation (EU) 2022/30, with the harmonised standard EN 18031-1/-2/-3 and its mechanism (AUM, ACM, SCM, SSM, SUM, RLM, CCK, CRY, GEC, DLM …) | `Directive 2014/53/EU (RED) Art. 3(3)(d) (EN 18031-1 AUM)` |

Cite the RED only for items that make sense for internet-connected radio
equipment, and IEC 62443 where the item fits an industrial system or
component.
