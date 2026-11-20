# Catalogue data format

The standard requirement and test library lives in `data/*.yaml`, one file
per **area** (a topic such as authentication or performance). Every file is
embedded into the binary and checked by `go test ./internal/catalog`.

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
    tests: [AUTH-T01, AUTH-T02] # catalogue tests that verify it (same or other area)
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
  Annex A, ISTQB, …) but write every sentence yourself and cite the standard
  in `refs` by name and section only. Never copy normative text.
- **Generic and reusable.** Write "the system", "a user", "an administrator";
  no product names. Put the adjustable value (12 characters, 2 seconds,
  99.9 %) in the text so a team can edit it after adding the item.
- **One verifiable statement** per requirement, with exactly one obligation
  keyword in upper case: English MUST / MUST NOT / SHOULD / SHOULD NOT / MAY
  (RFC 2119), Swedish SKA / SKA INTE / BÖR / BÖR INTE / FÅR (SIS convention).
  `obligation` must match the keyword (SKA counts as must). The requirement
  analysis (internal/reqlang) must find no errors or warnings: avoid vague
  words such as "fast", "user-friendly", "etc.", "and/or", "as appropriate",
  "snabbt", "användarvänlig", "osv", "vid behov".
- **Tests** are concrete and executable: preconditions, numbered actions with
  an expected result each, test data where it helps. A requirement's `tests`
  lists the catalogue tests that verify it; every test should verify at
  least one requirement.
- **Swedish** is natural, idiomatic Swedish (not word-for-word), using the
  terms of the Swedish user interface: krav, testfall, körning, organisation,
  administratör, inloggning.
