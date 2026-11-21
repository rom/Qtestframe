# References

Standards, guidelines and other documents behind Qtestframe's way of writing requirements and tests. Use them when you set up a specification process, train authors or need to justify a rule to an auditor. Links go to the publisher. ISO and IEEE standards are not free to read, but their catalogue pages describe the scope.

## Requirement key words (RFCs)

These define the obligation levels and the [language analysis](requirements.md#the-language-analysis) in Qtestframe.

| Document | What it says | Used in Qtestframe for |
|---|---|---|
| [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) – *Key words for use in RFCs to Indicate Requirement Levels* (S. Bradner, 1997) | Defines **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY** and **OPTIONAL**, and warns against using them where they are not needed. | The obligation levels (*must*, *shall*, *should*, *may*, *optional* and their negations) and keyword detection |
| [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) – *Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words* (B. Leiba, 2017) | Clarifies that only the **UPPERCASE** forms carry the RFC 2119 meaning; lowercase words have their normal English meaning. | The *Lower-case keyword* rule |
| [BCP 14](https://www.rfc-editor.org/info/bcp14) | The "best current practice" made up of RFC 2119 and RFC 8174. Specifications cite it with a standard sentence: *The key words "MUST", "MUST NOT", … are to be interpreted as described in BCP 14 [RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown here.* | Put this sentence in the introduction of your specification |
| [RFC 7322](https://www.rfc-editor.org/rfc/rfc7322) – *RFC Style Guide* (2014) | Editorial rules for RFCs, including how to cite BCP 14. | A model for a house style |
| [RFC 3552](https://www.rfc-editor.org/rfc/rfc3552) – *Guidelines for Writing RFC Text on Security Considerations* (BCP 72, 2003) | How to analyse and describe threats in a specification. | Writing *security* requirements |
| [RFC 6919](https://www.rfc-editor.org/rfc/rfc6919) – *Further Key Words for Use in RFCs to Indicate Requirement Levels* (2013) | An April Fools' RFC with key words such as *MUST (BUT WE KNOW YOU WON'T)*. | A humorous reminder of why ambiguous wording fails |

### Other RFCs that Qtestframe implements

| Document | Where |
|---|---|
| [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) – *HTTP Semantics* | The REST API |
| [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259) – *JSON* | API requests and responses, backups |
| [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339) – *Date and Time on the Internet: Timestamps* | Dates in the API, in queries (`updated_at>=2026-01-01`) and the audit log filter |
| [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180) – *CSV Files* | CSV export |
| [RFC 5424](https://www.rfc-editor.org/rfc/rfc5424) – *The Syslog Protocol* | Server logging to syslog |
| [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280) – *X.509 Certificate and CRL Profile* | TLS certificates and the certificate scripts |
| [RFC 6265](https://www.rfc-editor.org/rfc/rfc6265) – *HTTP State Management Mechanism* (cookies) | Session cookies |
| [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) – *OAuth 2.0* and [RFC 7519](https://www.rfc-editor.org/rfc/rfc7519) – *JSON Web Token* | Single sign-on with [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html) |
| [SAML 2.0 Core](https://docs.oasis-open.org/security/saml/v2.0/saml-core-2.0-os.pdf) (OASIS) | Single sign-on with SAML |

## NIST publications

The US National Institute of Standards and Technology publishes free guidance that many organisations use as requirement and test catalogues, especially for security.

| Document | Relevance |
|---|---|
| [NIST SP 800-53 Rev. 5](https://doi.org/10.6028/NIST.SP.800-53r5) – *Security and Privacy Controls for Information Systems and Organizations* (2020) | A catalogue of controls, often imported as security requirements. Use categories (for example *Control family / AC*) to keep track of them. |
| [NIST SP 800-53A Rev. 5](https://doi.org/10.6028/NIST.SP.800-53Ar5) – *Assessing Security and Privacy Controls in Information Systems and Organizations* (2022) | Assessment procedures per control, using the methods *examine*, *interview* and *test*; they map onto the [evaluation criteria](requirements.md#evaluation-criteria) methods. |
| [NIST SP 800-115](https://doi.org/10.6028/NIST.SP.800-115) – *Technical Guide to Information Security Testing and Assessment* (2008) | Planning and running security tests: review, target identification, vulnerability validation. |
| [NIST SP 800-160 Vol. 1 Rev. 1](https://doi.org/10.6028/NIST.SP.800-160v1r1) – *Engineering Trustworthy Secure Systems* (2022) | Systems security engineering, including stakeholder needs, requirements definition and verification, aligned with ISO/IEC/IEEE 15288. |
| [NIST SP 800-218](https://doi.org/10.6028/NIST.SP.800-218) – *Secure Software Development Framework (SSDF) Version 1.1* (2022) | Practices for secure development, including defining security requirements and testing them. |
| [NIST IR 8397](https://doi.org/10.6028/NIST.IR.8397) – *Guidelines on Minimum Standards for Developer Verification of Software* (2021) | Minimum verification techniques: threat modelling, automated testing, static and dynamic analysis, fuzzing and more. |
| [NIST SP 800-142](https://doi.org/10.6028/NIST.SP.800-142) – *Practical Combinatorial Testing* (2010) | Designing a small number of tests that cover parameter interactions. |
| [NIST SP 800-63B-4](https://doi.org/10.6028/NIST.SP.800-63B-4) – *Digital Identity Guidelines: Authentication and Authenticator Management* (2025) | Requirements for passwords, multi-factor authentication and sessions. |
| [NIST SP 800-92](https://doi.org/10.6028/NIST.SP.800-92) – *Guide to Computer Security Log Management* (2006) | Requirements for audit and security logging. |
| [NIST CSWP 29](https://doi.org/10.6028/NIST.CSWP.29) – *The NIST Cybersecurity Framework (CSF) 2.0* (2024) | High-level outcomes that can be broken down into requirements. |
| [NIST Planning Report 02-3](https://www.nist.gov/system/files/documents/director/planning/report02-3.pdf) – *The Economic Impacts of Inadequate Infrastructure for Software Testing* (2002) | A classic study of the cost of finding defects late. |

## Requirement engineering standards and guides

| Document | Relevance |
|---|---|
| [ISO/IEC/IEEE 29148:2018](https://www.iso.org/standard/72089.html) – *Systems and software engineering — Life cycle processes — Requirements engineering* | The main international standard for requirements: characteristics of good requirements (necessary, unambiguous, complete, singular, feasible, verifiable, correct), requirement attributes and specification contents. Replaces IEEE 830. |
| [ISO/IEC Directives, Part 2](https://www.iso.org/sites/directives/current/part2/index.xhtml) – *Principles and rules for the structure and drafting of ISO and IEC documents* | Defines the verbal forms of international standards: **shall** (requirement), **should** (recommendation), **may** (permission), **can** (possibility). Swedish standards from [SIS](https://www.sis.se/) use *ska*, *bör*, *får* and *kan*, which Qtestframe's Swedish analysis follows. |
| [INCOSE Guide to Writing Requirements](https://www.incose.org/docs/default-source/working-groups/requirements-wg/guidetowritingrequirements/incose_rwg_gtwr_v4_summary_sheet.pdf) (version 4, 2023; link to the free summary sheet) | Rules for well-formed requirements: active voice, no vague terms, no escape clauses, avoid "not", one thought per requirement. The language analysis checks several of these rules. |
| [EARS – Easy Approach to Requirements Syntax](https://doi.org/10.1109/RE.2009.9) (A. Mavin et al., IEEE RE 2009) | Simple sentence patterns: *While …, when …, the system shall …*. |
| [IREB CPRE Foundation Level](https://www.ireb.org/en/cpre/foundation/) | Certification syllabus and glossary for requirements engineering. |
| [ISO/IEC 25010:2023](https://www.iso.org/standard/78176.html) – *Product quality model* | Quality characteristics (performance efficiency, usability, security, reliability …) that make a good checklist for non-functional requirement types. |

## Testing standards and guides

| Document | Relevance |
|---|---|
| [ISO/IEC/IEEE 29119](https://committee.iso.org/sites/jtc1sc7/home/projects/flagship-standards/isoiecieee-29119-series.html) – *Software testing* series, especially [Part 1: General concepts](https://www.iso.org/standard/81291.html) (2022) and [Part 3: Test documentation](https://www.iso.org/standard/79429.html) (2021) | Test processes, documentation templates (test plan, test case, test log, incident report) and techniques. Part 3 replaces IEEE 829. |
| [IEEE 1012-2016](https://standards.ieee.org/ieee/1012/5609/) – *System, Software, and Hardware Verification and Validation* | V&V activities and integrity levels. |
| [ISTQB Glossary](https://glossary.istqb.org/) and [Certified Tester Foundation Level v4.0](https://www.istqb.org/certifications/certified-tester-foundation-level-ctfl-v4-0/) | The common vocabulary of testing (test case, test run, defect, coverage …) used in this guide. |
| [OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/) | A ready-made list of testable security requirements for web applications, in three levels. |
| [OWASP Web Security Testing Guide](https://owasp.org/www-project-web-security-testing-guide/) | How to test the security of web applications; complements ASVS. |
| [WCAG 2.2](https://www.w3.org/TR/WCAG22/) (and [WCAG 2.1](https://www.w3.org/TR/WCAG21/)) – *Web Content Accessibility Guidelines* | Testable accessibility success criteria. Qtestframe itself aims at WCAG 2.1 AA, see [Accessibility and languages](accessibility-and-languages.md). |

## Domain-specific standards

Regulated domains add their own requirement and verification rules. Qtestframe's obligation levels, evaluation criteria, baselines, history and audit log are useful there, but check the standard of your domain, for example *DO-178C* (airborne software), *IEC 62304* (medical device software), *ISO 26262* (road vehicles), *IEC 61508* (functional safety) or *EN 50128* (railway software).

<!-- BEGIN generated sources: make refs-docs -->

## Sources cited in the catalogue

The [requirement catalogue](catalogue.md) cites 339 laws, standards, best practices and guidelines. Each source has the number of catalogue requirements that cite it and of catalogue tests that cite it or verify such a requirement, the same numbers as when you choose sources to comply with in the catalogue. Where a source groups several documents, they are listed below it. This section is generated from the catalogue.

### Laws and regulations

#### Laws and regulations: European Union

- [Artificial Intelligence Act, Regulation (EU) 2024/1689](https://eur-lex.europa.eu/eli/reg/2024/1689/oj) (AI Act): 34 requirements, 23 tests
- [Batteries Regulation (EU) 2023/1542](https://eur-lex.europa.eu/eli/reg/2023/1542/oj): 3 requirements, 3 tests
- [CLP Regulation (EC) No 1272/2008 on classification, labelling and packaging](https://eur-lex.europa.eu/eli/reg/2008/1272/oj): 1 requirement, 1 test
- [Commission Regulation (EU) 2019/1782, ecodesign requirements for external power supplies](https://eur-lex.europa.eu/eli/reg/2019/1782/oj): 1 requirement, 1 test
- [Commission Regulation (EU) 2023/826, ecodesign requirements for off mode, standby mode and networked standby](https://eur-lex.europa.eu/eli/reg/2023/826/oj): 1 requirement, 1 test
- [Cyber Resilience Act, Regulation (EU) 2024/2847](https://eur-lex.europa.eu/eli/reg/2024/2847/oj) (CRA): 89 requirements, 84 tests
- [Data Act, Regulation (EU) 2023/2854](https://eur-lex.europa.eu/eli/reg/2023/2854/oj): 9 requirements, 6 tests
- [Decision No 768/2008/EC on a common framework for the marketing of products](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32008D0768) (Decision 768/2008/EC): 4 requirements, 3 tests
- [Digital Omnibus on AI, Regulation (EU) 2026/1744 amending the AI Act](https://eur-lex.europa.eu/eli/reg/2026/1744/oj): 5 requirements, 4 tests
- [Digital Operational Resilience Act, Regulation (EU) 2022/2554](https://eur-lex.europa.eu/eli/reg/2022/2554/oj) (DORA): 49 requirements, 41 tests
- [E-PRTR Regulation (EC) No 166/2006](https://eur-lex.europa.eu/eli/reg/2006/166/oj): 1 requirement, 1 test
- [Ecodesign for Sustainable Products Regulation, Regulation (EU) 2024/1781](https://eur-lex.europa.eu/eli/reg/2024/1781/oj) (ESPR): 5 requirements, 4 tests
- [eIDAS Regulation (EU) No 910/2014](https://eur-lex.europa.eu/eli/reg/2014/910/oj): 1 requirement, 1 test
- [EU electricity network codes and guidelines (SO GL, EB GL, NC ER)](https://eur-lex.europa.eu/eli/reg/2017/1485/oj): 4 requirements, 4 tests
  - [Commission Regulation (EU) 2017/1485](https://eur-lex.europa.eu/eli/reg/2017/1485/oj) · [Commission Regulation (EU) 2017/2195](https://eur-lex.europa.eu/eli/reg/2017/2195/oj) · [Commission Regulation (EU) 2017/2196](https://eur-lex.europa.eu/eli/reg/2017/2196/oj)
- [EU ETS Monitoring and Reporting Regulation (EU) 2018/2066](https://eur-lex.europa.eu/eli/reg_impl/2018/2066/oj): 1 requirement, 1 test
- [General Data Protection Regulation (EU) 2016/679](https://eur-lex.europa.eu/eli/reg/2016/679/oj) (GDPR): 61 requirements, 59 tests
- [Machinery Regulation (EU) 2023/1230](https://eur-lex.europa.eu/eli/reg/2023/1230/oj): 25 requirements, 21 tests
- [Methane Emissions Regulation (EU) 2024/1787](https://eur-lex.europa.eu/eli/reg/2024/1787/oj): 2 requirements, 2 tests
- [Network Code on Cybersecurity, Delegated Regulation (EU) 2024/1366](https://eur-lex.europa.eu/eli/reg_del/2024/1366/oj) (NCCS): 2 requirements, 2 tests
- [NIS2 Implementing Regulation (EU) 2024/2690](https://eur-lex.europa.eu/eli/reg_impl/2024/2690/oj) (NIS2 IR): 9 requirements, 11 tests
- [REACH, Regulation (EC) No 1907/2006](https://eur-lex.europa.eu/eli/reg/2006/1907/oj): 3 requirements, 3 tests
- [Regulation (EC) No 765/2008 on accreditation and CE marking](https://eur-lex.europa.eu/eli/reg/2008/765/oj): 1 requirement, 1 test
- [REMIT, Regulation (EU) No 1227/2011 on wholesale energy market integrity](https://eur-lex.europa.eu/eli/reg/2011/1227/oj): 1 requirement, 1 test

#### Laws and regulations: Sweden

- [Act (2006:985) on energy performance certificates for buildings](https://lagen.nu/2006:985): 1 requirement, 1 test
- [Act on flammable and explosive goods (Lag 2010:1011)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/lag-20101011-om-brandfarliga-och-explosiva_sfs-2010-1011/): 1 requirement, 1 test
- [Archives Act (Arkivlag 1990:782)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/arkivlag-1990782_sfs-1990-782/): 2 requirements, 1 test
- [Boverket's building regulations (BFS 2024:4–2024:13, BFS 2026:4 and 2026:9), which replaced BBR (BFS 2011:6) and EKS](https://www.boverket.se/sv/byggande/regler-for-byggande/om-boverkets-nya-byggregler/): 9 requirements, 6 tests
- [Camera Surveillance Act (2018:1200)](https://lagen.nu/2018:1200): 1 requirement, 1 test
- [Civil Protection Act (2003:778)](https://lagen.nu/2003:778) (LSO): 5 requirements, 5 tests
- [Electrical Safety Act (Elsäkerhetslag 2016:732)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/elsakerhetslag-2016732_sfs-2016-732/): 2 requirements, 1 test
- [Inspection of lifts and other motor-driven devices: Plan- och byggförordning (2011:338) 5 kap. 8–11 §§ and Boverket BFS 2025:11 and 2025:12](https://www.boverket.se/sv/PBL-kunskapsbanken/regler-om-byggande/motordrivna-anordningar/hissar/): 1 requirement, 1 test
- [Language Act (2009:600)](https://lagen.nu/2009:600): 3 requirements, 3 tests
- [Public Access to Information and Secrecy Act (Offentlighets- och sekretesslag 2009:400)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/offentlighets-och-sekretesslag-2009400_sfs-2009-400/) (OSL): 1 requirement, 1 test
- [Public Procurement Act (2016:1145)](https://lagen.nu/2016:1145) (LOU): 4 requirements, 3 tests
- [Swedish Act on accessibility of digital public services (2018:1937) and DIGG regulations](https://lagen.nu/2018:1937) (DOS-lagen): 4 requirements, 4 tests
  - [Lag (2018:1937)](https://lagen.nu/2018:1937) · [DIGG MDFFS 2019:2](https://www.digg.se/om-oss/forfattningssamling/foreskrifter-om-tillganglighet-till-digital-offentlig-service-mdffs-20192/grundforfattning-mdffs-20192)
- [Swedish Bookkeeping Act (1999:1078)](https://lagen.nu/1999:1078): 2 requirements, 2 tests
- [Swedish Cybersecurity Act (2025:1506), implementing NIS2](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/cybersakerhetslag-20251506_sfs-2025-1506/) (Cybersäkerhetslag): 7 requirements, 5 tests
- [Swedish Electricity Preparedness Act (1997:288)](https://lagen.nu/1997:288): 1 requirement, 1 test
- [Swedish Electronic Communications Act (2022:482)](https://lagen.nu/2022:482) (LEK): 1 requirement, 1 test
- [Swedish Marketing Act (2008:486)](https://lagen.nu/2008:486): 1 requirement, 1 test
- [Swedish Patient Data Act (2008:355)](https://lagen.nu/2008:355): 1 requirement, 1 test
- [Swedish Protective Security Act (2018:585), Ordinance (2021:955) and regulations](https://lagen.nu/2018:585) (Säkerhetsskydd): 16 requirements, 13 tests
  - [Säkerhetsskyddslag (2018:585)](https://lagen.nu/2018:585) · [Säkerhetsskyddsförordning (2021:955)](https://lagen.nu/2021:955)
- [Swedish Whistleblower Act (Lag 2021:890)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/lag-2021890-om-skydd-for-personer-som_sfs-2021-890/): 1 requirement, 1 test
- [Swedish Work Environment Authority regulations (AFS 2023:1–2023:15)](https://www.av.se/arbetsmiljoarbete-och-inspektioner/publikationer/foreskrifter/): 4 requirements, 3 tests
- [Ventilation inspection rules: Plan- och byggförordning (2011:338) 5 kap. 1–7 §§ and Boverket BFS 2011:16 (OVK)](https://www.boverket.se/sv/PBL-kunskapsbanken/regler-om-byggande/ovk/): 2 requirements, 2 tests

#### Laws and regulations: United States

- [HIPAA Security Rule (45 CFR Part 164)](https://www.ecfr.gov/current/title-45/subtitle-A/subchapter-C/part-164/subpart-C): 1 requirement, 1 test
- [OSHA Process Safety Management, 29 CFR 1910.119](https://www.osha.gov/laws-regs/regulations/standardnumber/1910/1910.119): 1 requirement, 1 test
- [US Code of Federal Regulations Title 47 (FCC), Parts 2 and 15](https://www.ecfr.gov/current/title-47) (FCC 47 CFR): 1 requirement, 1 test
- [US FDA 21 CFR Part 11 (electronic records and signatures)](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11): 13 requirements, 12 tests

### Directives

#### Directives: European Union

- [ATEX Workplace Directive 1999/92/EC](https://eur-lex.europa.eu/eli/dir/1999/92/oj) (ATEX 153): 1 requirement, 1 test
- [Common charger, Directive (EU) 2022/2380 amending the Radio Equipment Directive](https://eur-lex.europa.eu/eli/dir/2022/2380/oj): 1 requirement, 1 test
- [Consumer Rights Directive 2011/83/EU (as amended by Directive (EU) 2023/2673)](https://eur-lex.europa.eu/eli/dir/2011/83/oj): 8 requirements, 6 tests
  - [Directive 2011/83/EU](https://eur-lex.europa.eu/eli/dir/2011/83/oj) · [Directive (EU) 2023/2673](https://eur-lex.europa.eu/eli/dir/2023/2673/oj)
- [Critical Entities Resilience Directive (EU) 2022/2557](https://eur-lex.europa.eu/eli/dir/2022/2557/oj) (CER): 15 requirements, 13 tests
- [Electricity Market Directive (EU) 2019/944](https://eur-lex.europa.eu/eli/dir/2019/944/oj): 3 requirements, 3 tests
- [Electromagnetic Compatibility Directive 2014/30/EU](https://eur-lex.europa.eu/eli/dir/2014/30/oj) (EMC): 12 requirements, 9 tests
- [Energy Efficiency Directive (EU) 2023/1791](https://eur-lex.europa.eu/eli/dir/2023/1791/oj): 2 requirements, 2 tests
- [Energy Performance of Buildings Directive (EU) 2024/1275](https://eur-lex.europa.eu/eli/dir/2024/1275/oj) (EPBD): 3 requirements, 3 tests
- [ePrivacy Directive 2002/58/EC](https://eur-lex.europa.eu/eli/dir/2002/58/oj): 6 requirements, 6 tests
- [European Electronic Communications Code, Directive (EU) 2018/1972](https://eur-lex.europa.eu/eli/dir/2018/1972/oj) (EECC): 8 requirements, 6 tests
  - [Directive (EU) 2018/1972](https://eur-lex.europa.eu/eli/dir/2018/1972/oj) · [Commission Delegated Regulation (EU) 2023/444](https://eur-lex.europa.eu/eli/reg_del/2023/444/oj)
- [Industrial Emissions Directive 2010/75/EU](https://eur-lex.europa.eu/eli/dir/2010/75/oj) (IED): 2 requirements, 1 test
- [Low Voltage Directive 2014/35/EU](https://eur-lex.europa.eu/eli/dir/2014/35/oj) (LVD): 11 requirements, 8 tests
- [Measuring Instruments Directive 2014/32/EU](https://eur-lex.europa.eu/eli/dir/2014/32/oj) (MID): 2 requirements, 2 tests
- [NIS2 Directive (EU) 2022/2555](https://eur-lex.europa.eu/eli/dir/2022/2555/oj): 25 requirements, 24 tests
- [Noise at work, Directive 2003/10/EC on exposure of workers to the risks arising from noise](https://eur-lex.europa.eu/eli/dir/2003/10/oj): 1 requirement, 1 test
- [Payment Services Directive (EU) 2015/2366 (PSD2) with the RTS on strong customer authentication](https://eur-lex.europa.eu/eli/dir/2015/2366/oj): 2 requirements, 2 tests
  - [Directive (EU) 2015/2366](https://eur-lex.europa.eu/eli/dir/2015/2366/oj) · [Commission Delegated Regulation (EU) 2018/389](https://eur-lex.europa.eu/eli/reg_del/2018/389/oj)
- [Pressure Equipment Directive 2014/68/EU](https://eur-lex.europa.eu/eli/dir/2014/68/oj) (PED): 14 requirements, 9 tests
- [Public Procurement Directive 2014/24/EU](https://eur-lex.europa.eu/eli/dir/2014/24/oj): 5 requirements, 4 tests
- [Radio Equipment Directive 2014/53/EU with Delegated Regulation (EU) 2022/30 and EN 18031](https://eur-lex.europa.eu/eli/dir/2014/53/oj) (RED): 23 requirements, 23 tests
  - [Directive 2014/53/EU](https://eur-lex.europa.eu/eli/dir/2014/53/oj) · [Commission Delegated Regulation (EU) 2022/30](https://eur-lex.europa.eu/eli/reg_del/2022/30/oj) · [EN 18031](https://single-market-economy.ec.europa.eu/single-market/goods/european-standards/harmonised-standards/radio-equipment_en)
- [RoHS Directive 2011/65/EU on hazardous substances in electrical and electronic equipment](https://eur-lex.europa.eu/eli/dir/2011/65/oj): 3 requirements, 2 tests
- [Seveso III Directive 2012/18/EU](https://eur-lex.europa.eu/eli/dir/2012/18/oj): 3 requirements, 2 tests
- [Unfair Commercial Practices Directive 2005/29/EC](https://eur-lex.europa.eu/eli/dir/2005/29/oj) (UCPD): 3 requirements, 3 tests
- [Unfair Contract Terms Directive 93/13/EEC](https://eur-lex.europa.eu/eli/dir/1993/13/oj): 1 requirement, 1 test
- [VAT Directive 2006/112/EC](https://eur-lex.europa.eu/eli/dir/2006/112/oj): 5 requirements, 4 tests
- [Waste Electrical and Electronic Equipment Directive 2012/19/EU](https://eur-lex.europa.eu/eli/dir/2012/19/oj) (WEEE): 6 requirements, 5 tests
- [Waste Framework Directive 2008/98/EC](https://eur-lex.europa.eu/eli/dir/2008/98/oj): 1 requirement, 1 test
- [Web Accessibility Directive (EU) 2016/2102](https://eur-lex.europa.eu/eli/dir/2016/2102/oj): 4 requirements, 4 tests
  - [Directive (EU) 2016/2102](https://eur-lex.europa.eu/eli/dir/2016/2102/oj) · [Commission Implementing Decision (EU) 2018/1523](https://eur-lex.europa.eu/eli/dec_impl/2018/1523/oj)
- [Whistleblower Protection Directive (EU) 2019/1937](https://eur-lex.europa.eu/eli/dir/2019/1937/oj) (Whistleblower Directive): 1 requirement, 1 test
- [Work Equipment Directive 2009/104/EC](https://eur-lex.europa.eu/eli/dir/2009/104/oj): 5 requirements, 4 tests

### Standards and specifications

#### Standards and specifications: International

- [3GPP technical specifications](https://www.3gpp.org/specifications-technologies): 19 requirements, 16 tests
  - [3GPP TS 22.179](https://www.3gpp.org/DynaReport/22179.htm) · [3GPP TS 22.280](https://www.3gpp.org/DynaReport/22280.htm) · [3GPP TS 23.167](https://www.3gpp.org/DynaReport/23167.htm) · [3GPP TS 23.283](https://www.3gpp.org/DynaReport/23283.htm) · [3GPP TS 23.501](https://www.3gpp.org/DynaReport/23501.htm) · [3GPP TS 23.682](https://www.3gpp.org/DynaReport/23682.htm) · [3GPP TS 28.552](https://www.3gpp.org/DynaReport/28552.htm) · [3GPP TS 32.240](https://www.3gpp.org/DynaReport/32240.htm) · [3GPP TS 32.298](https://www.3gpp.org/DynaReport/32298.htm) · [3GPP TS 32.401](https://www.3gpp.org/DynaReport/32401.htm) · [3GPP TS 33.126](https://www.3gpp.org/DynaReport/33126.htm) · [3GPP TS 33.127](https://www.3gpp.org/DynaReport/33127.htm) · [3GPP TS 33.180](https://www.3gpp.org/DynaReport/33180.htm) · [3GPP TS 33.501](https://www.3gpp.org/DynaReport/33501.htm)
- [ANSI/ISA-101.01 Human machine interfaces for process automation](https://www.isa.org/standards-and-publications/isa-standards/isa-101-standards): 3 requirements, 3 tests
- [C2PA content credentials specification](https://c2pa.org/specifications/specifications/2.1/specs/C2PA_Specification.html): 2 requirements, 2 tests
- [CycloneDX bill of materials standard](https://cyclonedx.org/specification/overview/): 3 requirements, 4 tests
- [ECMA-376 Office Open XML file formats](https://ecma-international.org/publications-and-standards/standards/ecma-376/): 2 requirements, 2 tests
- [ECSS space product assurance standards](https://ecss.nl/standards/): 2 requirements, 2 tests
  - [ECSS-Q-ST-30-11C](https://ecss.nl/standard/ecss-q-st-30-11c-rev-2-derating-eee-components-23-june-2021/)
- [EMC standards (EN 55032, EN 55035, IEC 61000-4)](https://webstore.iec.ch/en/publication/22046): 7 requirements, 7 tests
  - [CISPR 32 (EN 55032)](https://webstore.iec.ch/en/publication/22046) · [CISPR 35 (EN 55035)](https://webstore.iec.ch/en/publication/25667) · [IEC 61000-4-2](https://webstore.iec.ch/en/publication/68954) · [IEC 61000-4-11](https://webstore.iec.ch/en/publication/63503)
- [Environmental testing and protection classes (IEC 60068, 60721-3, 60529, 62262)](https://webstore.iec.ch/en/publication/501): 11 requirements, 9 tests
  - [IEC 60068-1](https://webstore.iec.ch/en/publication/501) · [IEC 60529](https://webstore.iec.ch/en/publication/2452)
- [ETSI NFV specifications (network functions virtualisation)](https://www.etsi.org/technologies/nfv): 1 requirement, 1 test
  - [ETSI GS NFV-SEC 001](https://www.etsi.org/deliver/etsi_gs/NFV-SEC/001_099/001/) · [ETSI GS NFV-SEC 021](https://www.etsi.org/deliver/etsi_gs/NFV-SEC/001_099/021/) · [ETSI GS NFV-SOL 004](https://www.etsi.org/deliver/etsi_gs/NFV-SOL/001_099/004/)
- [ETSI technical specifications (lawful interception, retained data, emergency location)](https://www.etsi.org/standards) (ETSI TS): 6 requirements, 5 tests
  - [ETSI TS 102 232-1](https://www.etsi.org/deliver/etsi_ts/102200_102299/10223201/) · [ETSI TS 102 657](https://www.etsi.org/deliver/etsi_ts/102600_102699/102657/) · [ETSI TS 103 221-1](https://www.etsi.org/deliver/etsi_ts/103200_103299/10322101/) · [ETSI TS 103 221-2](https://www.etsi.org/deliver/etsi_ts/103200_103299/10322102/) · [ETSI TS 103 625](https://www.etsi.org/deliver/etsi_ts/103600_103699/103625/)
- [FIRST standards and guidelines (CVSS, EPSS, multi-party disclosure)](https://www.first.org/standards/): 4 requirements, 5 tests
  - [FIRST CVSS v4.0](https://www.first.org/cvss/v4.0/specification-document) · [FIRST EPSS](https://www.first.org/epss/) · [FIRST Guidelines and Practices for Multi-Party Vulnerability Coordination](https://www.first.org/global/sigs/vulnerability-coordination/multiparty/guidelines-v1.1)
- [Greenhouse Gas Protocol](https://ghgprotocol.org/) (GHG Protocol): 1 requirement, 2 tests
  - [GHG Protocol Scope 3 Standard](https://ghgprotocol.org/corporate-value-chain-scope-3-standard)
- [GS1 General Specifications](https://www.gs1.org/standards/barcodes-epcrfid-id-keys/gs1-general-specifications): 1 requirement, 1 test
- [IANA Time Zone Database](https://www.iana.org/time-zones) (tz): 6 requirements, 4 tests
- [IEC 60079 Explosive atmospheres](https://webstore.iec.ch/en/publication/63327): 1 requirement, 1 test
- [IEC 60118-4 Audio-frequency induction loop systems](https://webstore.iec.ch/en/publication/798): 1 requirement, 1 test
- [IEC 60870 Telecontrol equipment and systems](https://webstore.iec.ch/en/publication/3755): 1 requirement, 1 test
  - [IEC 60870-5-104](https://webstore.iec.ch/en/publication/3746)
- [IEC 61082 Preparation of documents used in electrotechnology](https://webstore.iec.ch/en/publication/4469): 2 requirements, 2 tests
- [IEC 61131-3 Programmable controller languages](https://webstore.iec.ch/en/publication/4552): 1 requirement, 1 test
- [IEC 61340 Electrostatics, protection of electronic devices from electrostatic phenomena](https://www.iec.ch/): 1 requirement, 1 test
- [IEC 61508 Functional safety of E/E/PE safety-related systems](https://webstore.iec.ch/en/publication/5515): 16 requirements, 13 tests
  - [IEC 61508-1](https://webstore.iec.ch/en/publication/5515) · [IEC 61508-2](https://webstore.iec.ch/en/publication/5516) · [IEC 61508-3](https://webstore.iec.ch/en/publication/5517) · [IEC 61508-5](https://webstore.iec.ch/en/publication/5519) · [IEC 61508-6](https://webstore.iec.ch/en/publication/5520)
- [IEC 61511 Functional safety for the process industry sector](https://webstore.iec.ch/en/publication/24241): 14 requirements, 12 tests
- [IEC 61850 Communication networks and systems for power utility automation](https://webstore.iec.ch/en/publication/6028): 4 requirements, 4 tests
  - [IEC 61850-5](https://webstore.iec.ch/en/publication/6012) · [IEC 61850-7-1](https://webstore.iec.ch/en/publication/6014) · [IEC 61850-10](https://webstore.iec.ch/en/publication/6008)
- [IEC 62056 Electricity metering data exchange (DLMS/COSEM)](https://webstore.iec.ch/en/publication/67915): 1 requirement, 1 test
- [IEC 62061 Functional safety of machinery control systems](https://webstore.iec.ch/en/publication/59927): 3 requirements, 3 tests
- [IEC 62351 Power systems data and communications security](https://webstore.iec.ch/en/publication/6912): 4 requirements, 3 tests
  - [IEC 62351-5](https://webstore.iec.ch/en/publication/65511) · [IEC 62351-7](https://webstore.iec.ch/en/publication/30593)
- [IEC 62402 Obsolescence management](https://webstore.iec.ch/en/publication/59531): 8 requirements, 6 tests
- [IEC 62443-2-1 Security programme requirements for asset owners](https://webstore.iec.ch/en/publication/62883): 4 requirements, 4 tests
- [IEC 62443-2-4 Security programme requirements for IACS service providers](https://webstore.iec.ch/en/publication/67631): 13 requirements, 12 tests
- [IEC 62443-3-2 Security risk assessment for system design](https://webstore.iec.ch/en/publication/30727): 6 requirements, 6 tests
- [IEC 62443-3-3 System security requirements and security levels](https://webstore.iec.ch/en/publication/7033): 153 requirements, 142 tests
- [IEC 62443-4-1 Secure product development lifecycle requirements](https://webstore.iec.ch/en/publication/33615): 40 requirements, 34 tests
- [IEC 62443-4-2 Technical security requirements for IACS components](https://webstore.iec.ch/en/publication/34421): 52 requirements, 52 tests
- [IEC 62541 OPC Unified Architecture](https://reference.opcfoundation.org/) (OPC UA): 3 requirements, 3 tests
- [IEC 62740 Root cause analysis (RCA)](https://webstore.iec.ch/): 1 requirement, 1 test
- [IEC 81346 Structuring principles and reference designations](https://www.iso.org/standard/82229.html): 2 requirements, 2 tests
- [IEC acceptance and commissioning of process equipment (IEC 62381, IEC 62337)](https://webstore.iec.ch/en/publication/6871): 6 requirements, 4 tests
  - [IEC 62337](https://webstore.iec.ch/en/publication/6871)
- [IEC Common Information Model (IEC 61968, 61970, 62325)](https://webstore.iec.ch/en/publication/62698) (IEC CIM): 3 requirements, 3 tests
  - [IEC 61970-301](https://webstore.iec.ch/en/publication/62698)
- [IEC dependability standards (IEC 60300, 60812, 61025, 61078, 61124, 61163, 61703, 61709, 62506, TR 63069)](https://webstore.iec.ch/en/publication/1293): 22 requirements, 18 tests
  - [IEC 60300-1](https://webstore.iec.ch/en/publication/1293) · [IEC 60812](https://webstore.iec.ch/en/publication/26359) · [IEC 61025](https://webstore.iec.ch/en/publication/4311) · [IEC 61703](https://webstore.iec.ch/en/publication/25646) · [IEC 61709](https://webstore.iec.ch/en/publication/28554)
- [IEC electrical product safety standards (IEC 62368-1, 60664-1, 60204-1, 60364-6)](https://webstore.iec.ch/en/publication/69308): 17 requirements, 15 tests
  - [IEC 62368-1](https://webstore.iec.ch/en/publication/69308) · [IEC 60664-1](https://webstore.iec.ch/en/publication/59671) · [IEC 60204-1](https://webstore.iec.ch/en/publication/26037) · [IEC 60364-6](https://webstore.iec.ch/en/publication/24656)
- [IEC standards for secondary lithium cells and batteries (IEC 62133, 61960, 62619)](https://webstore.iec.ch/en/publication/32662): 5 requirements, 5 tests
- [IEC TR 62443-2-3 Patch management in the IACS environment](https://webstore.iec.ch/en/publication/22811) (IEC 62443-2-3): 1 requirement, 1 test
- [IEC/IEEE 60255-118-1 Synchrophasor measurements](https://webstore.iec.ch/en/publication/28722): 1 requirement, 1 test
- [IEC/IEEE 82079-1 Preparation of information for use (instructions for use)](https://www.iso.org/standard/71620.html): 9 requirements, 7 tests
- [IEEE 1149.1 Boundary scan (JTAG)](https://standards.ieee.org/ieee/1149.1/4484/): 1 requirement, 1 test
- [IEEE 1588 Precision Time Protocol](https://standards.ieee.org/ieee/1588/6825/): 2 requirements, 2 tests
- [IEEE 1815 Distributed Network Protocol (DNP3)](https://standards.ieee.org/ieee/1815/5414/): 1 requirement, 1 test
- [IEEE 802.11 Wireless LAN medium access control and physical layer specifications](https://standards.ieee.org/ieee/802.11/10548/): 3 requirements, 1 test
- [IEEE 802.1X Port-based network access control](https://standards.ieee.org/ieee/802.1X/7345/): 2 requirements, 2 tests
- [IEEE 9274.1.1 Experience API (xAPI)](https://standards.ieee.org/ieee/9274.1.1/7321/): 1 requirement, 1 test
- [IETF RFCs and Internet-Drafts](https://www.rfc-editor.org/): 79 requirements, 69 tests
  - [RFC 2104](https://www.rfc-editor.org/rfc/rfc2104) · [RFC 2369](https://www.rfc-editor.org/rfc/rfc2369) · [RFC 2474](https://www.rfc-editor.org/rfc/rfc2474) · [RFC 3207](https://www.rfc-editor.org/rfc/rfc3207) · [RFC 3246](https://www.rfc-editor.org/rfc/rfc3246) · [RFC 3261](https://www.rfc-editor.org/rfc/rfc3261) · [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339) · [RFC 3414](https://www.rfc-editor.org/rfc/rfc3414) · [RFC 3464](https://www.rfc-editor.org/rfc/rfc3464) · [RFC 3629](https://www.rfc-editor.org/rfc/rfc3629) · [RFC 3711](https://www.rfc-editor.org/rfc/rfc3711) · [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986) · [RFC 4035](https://www.rfc-editor.org/rfc/rfc4035) · [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180) · [RFC 4594](https://www.rfc-editor.org/rfc/rfc4594) · [RFC 4647](https://www.rfc-editor.org/rfc/rfc4647) · [RFC 5321](https://www.rfc-editor.org/rfc/rfc5321) · [RFC 5322](https://www.rfc-editor.org/rfc/rfc5322) · [RFC 5424](https://www.rfc-editor.org/rfc/rfc5424) · [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545) · [RFC 5546](https://www.rfc-editor.org/rfc/rfc5546) · [RFC 5853](https://www.rfc-editor.org/rfc/rfc5853) · [RFC 5880](https://www.rfc-editor.org/rfc/rfc5880) · [RFC 5905](https://www.rfc-editor.org/rfc/rfc5905) · [RFC 6047](https://www.rfc-editor.org/rfc/rfc6047) · [RFC 6241](https://www.rfc-editor.org/rfc/rfc6241) · [RFC 6266](https://www.rfc-editor.org/rfc/rfc6266) · [RFC 6376](https://www.rfc-editor.org/rfc/rfc6376) · [RFC 6455](https://www.rfc-editor.org/rfc/rfc6455) · [RFC 6585](https://www.rfc-editor.org/rfc/rfc6585) · [RFC 6716](https://www.rfc-editor.org/rfc/rfc6716) · [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) · [RFC 6797](https://www.rfc-editor.org/rfc/rfc6797) · [RFC 7208](https://www.rfc-editor.org/rfc/rfc7208) · [RFC 7489](https://www.rfc-editor.org/rfc/rfc7489) · [RFC 7519](https://www.rfc-editor.org/rfc/rfc7519) · [RFC 7932](https://www.rfc-editor.org/rfc/rfc7932) · [RFC 7986](https://www.rfc-editor.org/rfc/rfc7986) · [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058) · [RFC 8246](https://www.rfc-editor.org/rfc/rfc8246) · [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259) · [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288) · [RFC 8301](https://www.rfc-editor.org/rfc/rfc8301) · [RFC 8460](https://www.rfc-editor.org/rfc/rfc8460) · [RFC 8461](https://www.rfc-editor.org/rfc/rfc8461) · [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594) · [RFC 8641](https://www.rfc-editor.org/rfc/rfc8641) · [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106) · [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) · [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111) · [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116) · [RFC 9413](https://www.rfc-editor.org/rfc/rfc9413) · [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) · [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700) · [RFC 9745](https://www.rfc-editor.org/rfc/rfc9745) · [draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/) · [draft-ietf-httpapi-ratelimit-headers](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/)
- [IPC electronics assembly and board standards (IPC-2221, IPC-6012, IPC-A-610, J-STD-001)](https://www.electronics.org/ipc-standards): 5 requirements, 4 tests
- [ISA-18.2 / IEC 62682 Alarm management](https://webstore.iec.ch/en/publication/65543): 10 requirements, 9 tests
  - [IEC 62682](https://webstore.iec.ch/en/publication/65543)
- [ISA-88 / IEC 61512 Batch control](https://www.isa.org/standards-and-publications/isa-standards/isa-88-standards): 4 requirements, 4 tests
  - [IEC 61512-1](https://webstore.iec.ch/en/publication/75287)
- [ISA-95 / IEC 62264 Enterprise-control system integration](https://www.isa.org/standards-and-publications/isa-standards/isa-95-standard): 5 requirements, 4 tests
  - [IEC 62264-1](https://www.iso.org/standard/57308.html) · [IEC 62264-2](https://webstore.iec.ch/en/publication/75127) · [IEC 62264-3](https://webstore.iec.ch/en/publication/33511)
- [ISAE 3402 Assurance reports on controls at a service organisation](https://www.iaasb.org/consultations-projects/assurance-reports-service-organizations-controls): 1 requirement, 1 test
- [ISO 10002 Customer satisfaction: complaints handling](https://www.iso.org/standard/71580.html): 3 requirements, 2 tests
- [ISO 10004 Customer satisfaction: monitoring and measuring](https://www.iso.org/standard/71582.html): 2 requirements, 2 tests
- [ISO 10006 Quality management in projects](https://www.iso.org/standard/70376.html): 2 requirements, 2 tests
- [ISO 10007 Guidelines for configuration management](https://www.iso.org/standard/70400.html): 11 requirements, 8 tests
- [ISO 10015 Guidelines for competence management and people development](https://www.iso.org/standard/69459.html): 11 requirements, 9 tests
- [ISO 10075 Ergonomic principles related to mental workload](https://www.iso.org/standard/76686.html): 1 requirement, 1 test
- [ISO 11064 Ergonomic design of control centres](https://www.iso.org/standard/19042.html): 5 requirements, 3 tests
- [ISO 13702 Control and mitigation of fires and explosions on offshore installations](https://www.iso.org/standard/57416.html): 2 requirements, 2 tests
- [ISO 13849 Safety-related parts of machine control systems](https://www.iso.org/standard/73481.html): 3 requirements, 3 tests
  - [ISO 13849-1](https://www.iso.org/standard/73481.html) · [ISO 13849-2](https://www.iso.org/standard/53640.html)
- [ISO 14001 Environmental management systems](https://www.iso.org/standard/92300.html): 1 requirement, 1 test
- [ISO 14064 Greenhouse gas quantification and reporting](https://www.iso.org/standard/66453.html): 1 requirement, 1 test
- [ISO 14224 Reliability and maintenance data for petroleum and natural gas equipment](https://www.iso.org/standard/64076.html): 3 requirements, 3 tests
- [ISO 14289 PDF/UA accessible PDF](https://www.iso.org/standard/64599.html): 2 requirements, 2 tests
- [ISO 14721 Open archival information system (OAIS) reference model](https://www.iso.org/search.html?q=ISO%2014721): 1 requirement, 1 test
- [ISO 15489 Records management](https://www.iso.org/standard/62542.html): 5 requirements, 4 tests
- [ISO 17100 Translation services](https://www.iso.org/standard/59149.html): 1 requirement, 1 test
- [ISO 17359 and ISO 13374 Condition monitoring of machines](https://www.iso.org/standard/71194.html): 3 requirements, 3 tests
  - [ISO 17359](https://www.iso.org/standard/71194.html) · [ISO 13374-1](https://www.iso.org/standard/21832.html)
- [ISO 19011 Guidelines for auditing management systems](https://www.iso.org/standard/88984.html): 4 requirements, 3 tests
- [ISO 20816 Mechanical vibration measurement](https://www.iso.org/standard/63180.html): 1 requirement, 1 test
- [ISO 21502, 21508 and 21511 Project, programme and portfolio management](https://www.iso.org/standard/74947.html): 30 requirements, 23 tests
  - [ISO 21502](https://www.iso.org/standard/74947.html) · [ISO 21508](https://www.iso.org/standard/63582.html) · [ISO 21511](https://www.iso.org/standard/69702.html)
- [ISO 21504 Project, programme and portfolio management: guidance on portfolio management](https://www.iso.org/search.html?q=ISO%2021504): 9 requirements, 6 tests
- [ISO 21542 Accessibility and usability of the built environment](https://www.iso.org/standard/71860.html): 2 requirements, 1 test
- [ISO 22301 Business continuity management systems](https://www.iso.org/standard/75106.html): 23 requirements, 18 tests
- [ISO 22400 Key performance indicators for manufacturing operations](https://www.iso.org/standard/54497.html): 2 requirements, 2 tests
- [ISO 22514 Statistical methods in process management (capability)](https://www.iso.org/standard/88883.html): 1 requirement, 1 test
- [ISO 24495 Plain language](https://www.iso.org/standard/78907.html): 1 requirement, 1 test
- [ISO 29993 Learning services outside formal education](https://www.iso.org/standard/70357.html): 6 requirements, 4 tests
- [ISO 31000 Risk management guidelines](https://www.iso.org/standard/65694.html): 14 requirements, 10 tests
- [ISO 32000 Portable Document Format (PDF)](https://www.iso.org/standard/75839.html): 1 requirement, 1 test
- [ISO 3382-3 Acoustics: measurement of room acoustic parameters in open plan offices](https://www.iso.org/search.html?q=ISO%203382-3): 1 requirement, 1 test
- [ISO 37301 Compliance management systems: requirements with guidance for use](https://www.iso.org/standard/75080.html): 4 requirements, 2 tests
- [ISO 3864 Safety colours and safety signs](https://www.iso.org/standard/66836.html): 1 requirement, 1 test
- [ISO 41001 Facility management systems](https://www.iso.org/standard/68021.html): 2 requirements, 2 tests
- [ISO 4126 Safety devices for protection against excessive pressure](https://www.iso.org/standard/50826.html): 1 requirement, 1 test
- [ISO 4217 Currency codes](https://www.iso.org/standard/64758.html): 4 requirements, 4 tests
- [ISO 45001 Occupational health and safety management systems](https://www.iso.org/standard/63787.html): 2 requirements, 2 tests
- [ISO 50001 Energy management systems](https://www.iso.org/standard/69426.html): 2 requirements, 2 tests
- [ISO 55001 Asset management systems](https://www.iso.org/standard/83054.html): 11 requirements, 10 tests
- [ISO 639 Language codes](https://www.iso.org/standard/74575.html): 1 requirement, 1 test
- [ISO 7870 Control charts](https://www.iso.org/standard/78859.html): 1 requirement, 1 test
- [ISO 8000 Data quality](https://www.iso.org/standard/60805.html): 1 requirement, 1 test
- [ISO 8601 Date and time representation](https://www.iso.org/standard/70907.html): 5 requirements, 5 tests
- [ISO 9001 Quality management systems](https://www.iso.org/standard/88464.html): 91 requirements, 72 tests
- [ISO 9241 Ergonomics of human-system interaction](https://www.iso.org/standard/63500.html): 82 requirements, 68 tests
  - [ISO 9241-11](https://www.iso.org/standard/63500.html) · [ISO 9241-110](https://www.iso.org/standard/75258.html) · [ISO 9241-143](https://www.iso.org/standard/53590.html) · [ISO 9241-303](https://www.iso.org/standard/57992.html)
- [ISO/IEC 15415 and 15416 Bar code print quality](https://www.iso.org/standard/76876.html): 1 requirement, 1 test
  - [ISO/IEC 15415](https://www.iso.org/standard/76876.html) · [ISO/IEC 15416](https://www.iso.org/standard/83390.html)
- [ISO/IEC 17025 Competence of testing and calibration laboratories](https://www.iso.org/standard/66912.html): 6 requirements, 6 tests
- [ISO/IEC 19941 Cloud computing interoperability and portability](https://www.iso.org/standard/66639.html): 2 requirements, 2 tests
- [ISO/IEC 20000-1 Service management systems](https://www.iso.org/standard/70636.html): 31 requirements, 29 tests
- [ISO/IEC 20889 Privacy enhancing data de-identification techniques](https://www.iso.org/standard/69373.html): 1 requirement, 1 test
- [ISO/IEC 21031 Software Carbon Intensity (SCI) specification](https://www.iso.org/standard/86612.html): 3 requirements, 3 tests
- [ISO/IEC 25000 series quality measurement and reporting (25022, 25023, 25062)](https://www.iso.org/standard/35746.html): 5 requirements, 5 tests
  - [ISO/IEC 25022](https://www.iso.org/standard/35746.html) · [ISO/IEC 25023](https://www.iso.org/standard/35747.html) · [ISO/IEC 25062](https://www.iso.org/standard/43046.html)
- [ISO/IEC 25010 Product quality model](https://www.iso.org/standard/78176.html): 103 requirements, 97 tests
- [ISO/IEC 25012 Data quality model](https://www.iso.org/standard/35736.html): 9 requirements, 8 tests
- [ISO/IEC 27001 Information security management systems](https://www.iso.org/standard/27001): 423 requirements, 373 tests
- [ISO/IEC 27002 Information security controls](https://www.iso.org/standard/75652.html): 3 requirements, 3 tests
- [ISO/IEC 27005 Information security risk management](https://www.iso.org/standard/80585.html): 3 requirements, 2 tests
- [ISO/IEC 27017 Information security controls for cloud services](https://www.iso.org/standard/82878.html): 7 requirements, 7 tests
- [ISO/IEC 27018 Protection of PII in public clouds](https://www.iso.org/standard/88150.html): 4 requirements, 5 tests
- [ISO/IEC 27019 Information security for the energy utility industry](https://www.iso.org/standard/85056.html): 7 requirements, 7 tests
- [ISO/IEC 27031 ICT readiness for business continuity](https://www.iso.org/standard/80975.html): 6 requirements, 5 tests
- [ISO/IEC 27033 Network security](https://www.iso.org/standard/63461.html): 15 requirements, 14 tests
  - [ISO/IEC 27033-1](https://www.iso.org/standard/63461.html) · [ISO/IEC 27033-2](https://www.iso.org/standard/51581.html) · [ISO/IEC 27033-4](https://www.iso.org/standard/51583.html) · [ISO/IEC 27033-5](https://www.iso.org/standard/51584.html) · [ISO/IEC 27033-6](https://www.iso.org/standard/51585.html)
- [ISO/IEC 27034 Application security](https://www.iso.org/standard/44378.html): 1 requirement, 1 test
- [ISO/IEC 27035 Information security incident management](https://www.iso.org/standard/78973.html): 8 requirements, 8 tests
  - [ISO/IEC 27035-1](https://www.iso.org/standard/78973.html) · [ISO/IEC 27035-2](https://www.iso.org/standard/78974.html)
- [ISO/IEC 27036 Information security for supplier relationships](https://www.iso.org/standard/82060.html): 10 requirements, 9 tests
  - [ISO/IEC 27036-2](https://www.iso.org/standard/82060.html) · [ISO/IEC 27036-3](https://www.iso.org/standard/82890.html)
- [ISO/IEC 27037 Handling of digital evidence](https://www.iso.org/standard/44381.html): 1 requirement, 1 test
- [ISO/IEC 27040 Storage security](https://www.iso.org/standard/80194.html): 3 requirements, 3 tests
- [ISO/IEC 27701 Privacy information management](https://www.iso.org/standard/85819.html): 14 requirements, 14 tests
- [ISO/IEC 29147 Vulnerability disclosure](https://www.iso.org/standard/72311.html): 15 requirements, 13 tests
- [ISO/IEC 30111 Vulnerability handling processes](https://www.iso.org/standard/69725.html): 13 requirements, 12 tests
- [ISO/IEC 42001 AI management systems](https://www.iso.org/standard/81230.html): 33 requirements, 26 tests
- [ISO/IEC AI standards (22989, 23894, 25059, 5259, 8183, TR 24027, TS 4213)](https://www.iso.org/standard/74296.html): 14 requirements, 11 tests
  - [ISO/IEC 22989](https://www.iso.org/standard/74296.html) · [ISO/IEC 23894](https://www.iso.org/standard/77304.html) · [ISO/IEC 25059](https://www.iso.org/standard/80655.html) · [ISO/IEC 5259-2](https://www.iso.org/standard/81860.html) · [ISO/IEC 8183](https://www.iso.org/standard/83002.html) · [ISO/IEC TR 24027](https://www.iso.org/standard/77607.html) · [ISO/IEC TS 4213](https://www.iso.org/standard/79799.html)
- [ISO/IEC/IEEE 12207 Software life cycle processes](https://www.iso.org/standard/63712.html): 1 requirement, 1 test
- [ISO/IEC/IEEE 15288 System life cycle processes](https://www.iso.org/standard/81702.html): 39 requirements, 30 tests
- [ISO/IEC/IEEE 15289 Content of life-cycle information items](https://www.iso.org/standard/74909.html): 11 requirements, 8 tests
- [ISO/IEC/IEEE 26514 Design and development of information for users](https://www.iso.org/standard/77451.html): 10 requirements, 7 tests
- [ISO/IEC/IEEE 29119 Software testing](https://www.iso.org/standard/79429.html): 9 requirements, 7 tests
  - [ISO/IEC/IEEE 29119-3](https://www.iso.org/standard/79429.html)
- [ISO/IEC/IEEE 29148 Requirements engineering](https://www.iso.org/standard/72089.html): 10 requirements, 7 tests
- [ISO/IEC/IEEE 42010 Architecture description](https://www.iso.org/standard/74393.html): 4 requirements, 4 tests
- [ITU-R recommendations (radio propagation)](https://www.itu.int/pub/R-REC): 1 requirement, 1 test
  - [ITU-R P.530](https://www.itu.int/rec/R-REC-P.530) · [ITU-R P.837](https://www.itu.int/rec/R-REC-P.837)
- [ITU-T Recommendations](https://www.itu.int/en/ITU-T/publications/Pages/recs.aspx): 11 requirements, 10 tests
  - [ITU-T E.800](https://www.itu.int/rec/T-REC-E.800) · [ITU-T G.107](https://www.itu.int/rec/T-REC-G.107) · [ITU-T G.114](https://www.itu.int/rec/T-REC-G.114) · [ITU-T G.722](https://www.itu.int/rec/T-REC-G.722) · [ITU-T G.8271](https://www.itu.int/rec/T-REC-G.8271) · [ITU-T G.8275.1](https://www.itu.int/rec/T-REC-G.8275.1) · [ITU-T M.3400](https://www.itu.int/rec/T-REC-M.3400) · [ITU-T X.733](https://www.itu.int/rec/T-REC-X.733) · [ITU-T Y.1541](https://www.itu.int/rec/T-REC-Y.1541)
- [LoRa Alliance LoRaWAN specifications (link layer, regional parameters, application layer)](https://resources.lora-alliance.org/technical-specifications): 6 requirements, 4 tests
- [Machine safety standards (ISO 12100, 13850, 13855, 13857, 14118, 14119, 14120)](https://www.iso.org/standard/51528.html): 8 requirements, 7 tests
  - [ISO 12100](https://www.iso.org/standard/51528.html) · [ISO 13850](https://www.iso.org/standard/59970.html) · [ISO 13855](https://www.iso.org/standard/80590.html) · [ISO 13857](https://www.iso.org/standard/69569.html) · [ISO 14118](https://www.iso.org/standard/66460.html) · [ISO 14119](https://www.iso.org/standard/75942.html) · [ISO 14120](https://www.iso.org/standard/59545.html)
- [Material declaration and marking standards (EN IEC 63000, IEC 62474, EN 50419)](https://webstore.iec.ch/en/publication/29857): 2 requirements, 2 tests
  - [IEC 62474](https://webstore.iec.ch/en/publication/29857)
- [Metrology standards and guides (ISO 10012, JCGM 100 GUM, ILAC-G24)](https://www.iso.org/standard/26033.html): 4 requirements, 4 tests
  - [ISO 10012](https://www.iso.org/standard/26033.html) · [JCGM 100](https://www.bipm.org/en/doi/10.59161/JCGM100-2008E) · [ILAC-G24](https://ilac.org/publications-and-resources/ilac-guidance-series/)
- [MITRE Common Weakness Enumeration (CWE)](https://cwe.mitre.org/): 13 requirements, 11 tests
  - [CWE-22](https://cwe.mitre.org/data/definitions/22.html) · [CWE-79](https://cwe.mitre.org/data/definitions/79.html) · [CWE-93](https://cwe.mitre.org/data/definitions/93.html) · [CWE-214](https://cwe.mitre.org/data/definitions/214.html) · [CWE-409](https://cwe.mitre.org/data/definitions/409.html) · [CWE-434](https://cwe.mitre.org/data/definitions/434.html) · [CWE-524](https://cwe.mitre.org/data/definitions/524.html) · [CWE-598](https://cwe.mitre.org/data/definitions/598.html) · [CWE-1236](https://cwe.mitre.org/data/definitions/1236.html)
- [OASIS Common Security Advisory Framework (CSAF)](https://docs.oasis-open.org/csaf/csaf/v2.0/csaf-v2.0.html): 1 requirement, 1 test
- [OASIS MQTT](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html): 2 requirements, 2 tests
- [Open Container Initiative specifications](https://opencontainers.org/) (OCI): 1 requirement, 1 test
  - [Open Container Initiative Image Format Specification](https://github.com/opencontainers/image-spec)
- [OpenAPI Specification](https://spec.openapis.org/oas/v3.1.0): 4 requirements, 3 tests
- [OpenChain ISO/IEC 5230 Open source licence compliance](https://www.iso.org/standard/81039.html): 4 requirements, 3 tests
- [OpenFeature specification](https://openfeature.dev/specification/): 2 requirements, 2 tests
- [OpenID Connect](https://openid.net/specs/openid-connect-core-1_0.html) (OIDC): 1 requirement, 1 test
- [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/): 2 requirements, 2 tests
- [Operator interface standards (IEC 60073, 60447, 61310-1, EN 894-3, ISO 7010)](https://webstore.iec.ch/en/publication/5263): 4 requirements, 3 tests
  - [IEC 61310-1](https://webstore.iec.ch/en/publication/5263) · [ISO 7010](https://www.iso.org/standard/72424.html)
- [OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/) (OWASP ASVS): 122 requirements, 116 tests
- [OWASP Mobile Application Security Verification Standard (MASVS)](https://mas.owasp.org/MASVS/) (OWASP MASVS): 2 requirements, 2 tests
- [PCI Data Security Standard and PCI SSC guidance](https://www.pcisecuritystandards.org/) (PCI DSS): 23 requirements, 22 tests
- [Peppol BIS Billing](https://docs.peppol.eu/poacc/billing/3.0/): 1 requirement, 1 test
- [SAE EIA-649 Configuration management standard](https://www.sae.org/standards/content/eia649c/): 4 requirements, 3 tests
- [SCORM e-learning content packaging](https://www.adlnet.gov/research/scorm): 1 requirement, 1 test
- [Semantic Versioning](https://semver.org/spec/v2.0.0.html) (SemVer): 5 requirements, 5 tests
- [SPDX specification and licence list](https://spdx.dev/): 4 requirements, 5 tests
  - [SPDX Specification 2.3](https://spdx.github.io/spdx-spec/v2.3/) · [SPDX License List](https://spdx.org/licenses/)
- [Standard Webhooks specification](https://www.standardwebhooks.com/): 2 requirements, 2 tests
- [Systems and software life-cycle standards (ISO/IEC/IEEE 15939, 24748-1, IEEE 1012)](https://www.iso.org/standard/71197.html): 10 requirements, 10 tests
  - [ISO/IEC/IEEE 15939](https://www.iso.org/standard/71197.html) · [ISO/IEC/IEEE 24748-1](https://www.iso.org/standard/84709.html) · [IEEE 1012](https://standards.ieee.org/ieee/1012/5609/)
- [Technical product documentation: title blocks and drawing principles (ISO 7200, ISO 128)](https://www.iso.org/search.html?q=ISO%207200): 2 requirements, 1 test
- [The Unicode Standard, CLDR, ICU and ISO/IEC 10646](https://www.unicode.org/standard/standard.html): 12 requirements, 11 tests
  - [Unicode CLDR](https://cldr.unicode.org/) · [Unicode UAX #9](https://www.unicode.org/reports/tr9/) · [Unicode UAX #15](https://www.unicode.org/reports/tr15/) · [Unicode UTS #10](https://www.unicode.org/reports/tr10/) · [Unicode UTS #35](https://www.unicode.org/reports/tr35/) · [ICU MessageFormat](https://unicode-org.github.io/icu/userguide/format_parse/messages/) · [ISO/IEC 10646](https://www.iso.org/standard/76835.html)
- [TM Forum Open APIs](https://www.tmforum.org/oda/open-apis): 4 requirements, 4 tests
- [Universal Postal Union standards (S42 addressing)](https://www.upu.int/en/postal-solutions/programmes-services/addressing-solutions) (UPU): 1 requirement, 1 test
- [W3C web standards (internationalisation, Trace Context, WebAuthn, CSP, Media Queries)](https://www.w3.org/TR/): 17 requirements, 14 tests
  - [W3C Character Model for the World Wide Web](https://www.w3.org/TR/charmod/) · [W3C Character Model: String Matching](https://www.w3.org/TR/charmod-norm/) · [W3C Content Security Policy Level 3](https://www.w3.org/TR/CSP3/) · [W3C Internationalization Best Practices](https://www.w3.org/International/techniques/authoring-html) · [W3C Internationalization: language negotiation](https://www.w3.org/International/questions/qa-accept-lang-locales) · [W3C Internationalization: text size in translation](https://www.w3.org/International/articles/article-text-size) · [W3C Personal names around the world](https://www.w3.org/International/questions/qa-personal-names) · [W3C Structural markup and right-to-left text in HTML](https://www.w3.org/International/questions/qa-html-dir) · [W3C Trace Context](https://www.w3.org/TR/trace-context/) · [W3C Web Authentication Level 2](https://www.w3.org/TR/webauthn-2/) · [Media Queries Level 5](https://www.w3.org/TR/mediaqueries-5/)
- [Web Content Accessibility Guidelines (WCAG) 2.2](https://www.w3.org/TR/WCAG22/) (WCAG 2.2): 60 requirements, 46 tests
- [WHATWG Living Standards (HTML, Fetch)](https://spec.whatwg.org/): 3 requirements, 3 tests
  - [HTML Living Standard](https://html.spec.whatwg.org/multipage/) · [Fetch Living Standard](https://fetch.spec.whatwg.org/)

#### Standards and specifications: European Union

- [EN 12464-1 Light and lighting: lighting of indoor work places](https://standards.cencenelec.eu/): 1 requirement, 1 test
- [EN 12845 Fixed firefighting systems: automatic sprinkler systems](https://standards.cencenelec.eu/): 1 requirement, 1 test
- [EN 13306 Maintenance terminology](https://www.sis.se/en/produkter/standardization/vocabularies/services/ss-en-133062017/): 11 requirements, 8 tests
- EN 14181 and EN 17255 Automated emission monitoring: 1 requirement, 1 test
- [EN 16798-1 Energy performance of buildings: indoor environmental input parameters](https://standards.cencenelec.eu/): 3 requirements, 2 tests
- [EN 16931 Electronic invoicing](https://ec.europa.eu/digital-building-blocks/sites/display/DIGITAL/Obtaining+a+copy+of+the+European+standard+on+eInvoicing): 1 requirement, 1 test
- [EN 1717 Protection against pollution of potable water in water installations (backflow)](https://standards.cencenelec.eu/): 1 requirement, 1 test
- [EN 3 Portable fire extinguishers](https://standards.cencenelec.eu/): 1 requirement, 1 test
- [EN 301 549 Accessibility requirements for ICT products and services](https://www.etsi.org/deliver/etsi_en/301500_301599/301549/): 18 requirements, 14 tests
- EN 50110-1 Operation of electrical installations: 3 requirements, 3 tests
- [EN 50126 and EN 50129 Railway RAMS and safety-related electronic systems](https://www.sis.se/en/produkter/electrical-engineering/electric-traction-equipment/ss-en-50126-1/): 12 requirements, 8 tests
- [EN 50518 Monitoring and alarm receiving centre](https://standards.cencenelec.eu/): 2 requirements, 2 tests
- EN 50600 Data centre facilities and infrastructures: 16 requirements, 13 tests
- [ETSI EN 300 220 Short Range Devices operating in the frequency range 25 MHz to 1 000 MHz](https://www.etsi.org/deliver/etsi_en/300200_300299/30022002/): 1 requirement, 1 test
- [ETSI EN 303 645 Cyber security for consumer IoT](https://www.etsi.org/deliver/etsi_en/303600_303699/303645/): 7 requirements, 7 tests
- [ETSI harmonised radio standards (EN 300 328, EN 300 396, EN 301 893)](https://www.etsi.org/deliver/etsi_en/300300_300399/300328/): 3 requirements, 2 tests
  - [ETSI EN 300 328](https://www.etsi.org/deliver/etsi_en/300300_300399/300328/) · [ETSI EN 301 893](https://www.etsi.org/deliver/etsi_en/301800_301899/301893/)
- [European accessibility standards for the built environment (EN 17210, EN 81-70)](https://www.sis.se/en/produkter/construction-materials-and-building/elements-of-buildings/elements-of-buildings-in-general/ss-en-172102021/): 3 requirements, 2 tests
  - [EN 17210](https://www.sis.se/en/produkter/construction-materials-and-building/elements-of-buildings/elements-of-buildings-in-general/ss-en-172102021/)
- European fire and emergency standards (EN 54, EN 1155, EN 1838, EN 15004-1, EN 50172): 8 requirements, 7 tests
- European maintenance standards (EN 13269, EN 13460, EN 15341, EN 17007): 15 requirements, 13 tests
- European security and safety standards for buildings (EN 1627, EN 50131, EN 60839-11-1, EN 62676-4): 6 requirements, 6 tests
- [Safety rules for lifts and escalators (EN 81-20, EN 81-28, EN 115-1)](https://standards.cencenelec.eu/): 2 requirements, 1 test

#### Standards and specifications: Norway

- [NORSOK standards (Norwegian petroleum industry)](https://standard.no/en/sectors/petroleum/): 2 requirements, 2 tests

#### Standards and specifications: Sweden

- [SS 436 40 00 Electrical low-voltage installations: rules for design and erection](https://www.elstandard.se/): 4 requirements, 3 tests

#### Standards and specifications: United States

- [American Petroleum Institute standards and recommended practices](https://www.api.org/products-and-services/standards) (API): 5 requirements, 5 tests
- [NERC Critical Infrastructure Protection standards](https://www.nerc.com/standards/reliability-standards) (NERC CIP): 2 requirements, 2 tests
- [NIST cryptographic standards and guidelines (FIPS 180-4, SP 800-52, 800-57, 800-90A, 800-107, 800-131A)](https://csrc.nist.gov/projects/cryptographic-standards-and-guidelines): 11 requirements, 12 tests
  - [NIST FIPS 180-4](https://doi.org/10.6028/NIST.FIPS.180-4) · [NIST SP 800-52 Rev. 2](https://doi.org/10.6028/NIST.SP.800-52r2) · [NIST SP 800-57 Part 1 Rev. 5](https://doi.org/10.6028/NIST.SP.800-57pt1r5) · [NIST SP 800-90A Rev. 1](https://doi.org/10.6028/NIST.SP.800-90Ar1) · [NIST SP 800-107 Rev. 1](https://doi.org/10.6028/NIST.SP.800-107r1) · [NIST SP 800-131A Rev. 2](https://doi.org/10.6028/NIST.SP.800-131Ar2)
- [NIST SP 800-53 Security and privacy controls](https://doi.org/10.6028/NIST.SP.800-53r5): 33 requirements, 31 tests

### Best practices and frameworks

#### Best practices and frameworks: International

- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/): 3 requirements, 3 tests
  - [AWS Well-Architected Framework, Sustainability Pillar](https://docs.aws.amazon.com/wellarchitected/latest/sustainability-pillar/sustainability-pillar.html) · [AWS Well-Architected SaaS Lens](https://docs.aws.amazon.com/wellarchitected/latest/saas-lens/saas-lens.html)
- [Continuous Delivery (Humble and Farley)](https://continuousdelivery.com/): 1 requirement, 1 test
- [CSA Cloud Controls Matrix](https://cloudsecurityalliance.org/research/cloud-controls-matrix) (CSA CCM): 5 requirements, 4 tests
- [CVE Program](https://www.cve.org/): 1 requirement, 1 test
- [DORA State of DevOps research](https://dora.dev/research/): 3 requirements, 2 tests
- [Feature Toggles (Martin Fowler's site)](https://martinfowler.com/articles/feature-toggles.html): 4 requirements, 3 tests
- [Goal Structuring Notation (GSN) Community Standard, Safety-Critical Systems Club](https://scsc.uk/gsn): 1 requirement, 1 test
- [Google Site Reliability Engineering books](https://sre.google/books/) (Google SRE): 28 requirements, 26 tests
- [Green Software Foundation guidance](https://greensoftware.foundation/) (GSF): 4 requirements, 4 tests
  - [Green Software Foundation: Carbon Aware SDK](https://github.com/Green-Software-Foundation/carbon-aware-sdk) · [Green Software Foundation: SCI guidance](https://sci-guide.greensoftware.foundation/)
- [ISTQB syllabi](https://www.istqb.org/): 17 requirements, 18 tests
  - [ISTQB CTFL 4.0](https://www.istqb.org/certifications/certified-tester-foundation-level-ctfl-v4-0/)
- [ITIL 4 service management practices](https://www.axelos.com/certifications/itil-service-management): 81 requirements, 68 tests
- [Keep a Changelog](https://keepachangelog.com/en/1.1.0/): 3 requirements, 3 tests
- [M3AAWG best practices for email](https://www.m3aawg.org/published-documents): 4 requirements, 4 tests
- [MITRE ATLAS adversarial threat landscape for AI systems](https://atlas.mitre.org/): 1 requirement, 1 test
- [MITRE ATT&CK](https://attack.mitre.org/): 2 requirements, 2 tests
  - [MITRE ATT&CK T1078](https://attack.mitre.org/techniques/T1078/)
- [Nielsen's usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/): 23 requirements, 20 tests
- [Oil and Gas Methane Partnership 2.0 reporting framework](https://ogmpartnership.org/) (OGMP 2.0): 1 requirement, 1 test
- [Open source and content licence conditions](https://opensource.org/licenses): 5 requirements, 6 tests
  - [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) · [BSD 3-Clause License](https://opensource.org/license/bsd-3-clause) · [MIT License](https://opensource.org/license/mit) · [GNU AGPL v3](https://www.gnu.org/licenses/agpl-3.0.html) · [GNU GPL v3](https://www.gnu.org/licenses/gpl-3.0.html) · [GNU LGPL v2.1](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html) · [Mozilla Public License 2.0](https://www.mozilla.org/en-US/MPL/2.0/) · [Server Side Public License v1](https://www.mongodb.com/legal/licensing/server-side-public-license) · [Creative Commons Attribution-ShareAlike 4.0](https://creativecommons.org/licenses/by-sa/4.0/) · [Creative Commons licences 4.0](https://creativecommons.org/licenses/) · [SIL Open Font License 1.1](https://openfontlicense.org/)
- [OpenSSF Scorecard](https://scorecard.dev/): 1 requirement, 1 test
- [OWASP API Security Top 10](https://owasp.org/API-Security/editions/2023/en/0x11-t10/) (OWASP API Top 10): 9 requirements, 9 tests
- [OWASP Software Assurance Maturity Model (SAMM)](https://owaspsamm.org/) (OWASP SAMM): 2 requirements, 2 tests
- [OWASP Top 10 for LLM applications](https://genai.owasp.org/llm-top-10/) (OWASP LLM Top 10): 12 requirements, 10 tests
- [OWASP Top 10 web application security risks](https://owasp.org/Top10/): 34 requirements, 35 tests
- [PMBOK Guide (Project Management Institute)](https://www.pmi.org/standards/pmbok): 6 requirements, 6 tests
- [PRINCE2 project management method](https://www.axelos.com/certifications/propath/prince2-project-management): 14 requirements, 12 tests
- RACI responsibility assignment matrix: 1 requirement, 1 test
- [REUSE Specification (licence and copyright information)](https://reuse.software/spec-3.2/): 1 requirement, 1 test
- [Scaled Agile Framework (SAFe)](https://framework.scaledagile.com/): 2 requirements, 1 test
- [SLSA supply-chain levels for software artifacts](https://slsa.dev/spec/v1.0/): 4 requirements, 4 tests
- System Usability Scale (SUS): 1 requirement, 1 test
- [The Scrum Guide (Schwaber and Sutherland)](https://scrumguides.org/scrum-guide.html): 6 requirements, 4 tests
- [The Twelve-Factor App](https://12factor.net/): 13 requirements, 13 tests
- Training design and evaluation models (ADDIE, Kirkpatrick): 9 requirements, 5 tests
  - [Kirkpatrick four-level training evaluation model](https://www.kirkpatrickpartners.com/the-kirkpatrick-model/)

#### Best practices and frameworks: United States

- [FEMA Incident Command System](https://training.fema.gov/emiweb/is/icsresource/) (ICS): 1 requirement, 1 test
- [NIST AI Risk Management Framework and Generative AI Profile](https://www.nist.gov/itl/ai-risk-management-framework) (NIST AI RMF): 24 requirements, 20 tests
  - [NIST AI RMF 1.0](https://doi.org/10.6028/NIST.AI.100-1) · [NIST AI 600-1](https://doi.org/10.6028/NIST.AI.600-1) · [NIST AI 100-2](https://doi.org/10.6028/NIST.AI.100-2e2025)
- [NIST SP 800-218 Secure Software Development Framework (SSDF)](https://doi.org/10.6028/NIST.SP.800-218) (NIST SSDF): 20 requirements, 17 tests

### Guidelines and guidance

#### Guidelines and guidance: International

- [ASHRAE TC 9.9 Thermal guidelines for data processing environments](https://tc0909.ashraetcs.org/): 2 requirements, 2 tests
- [CERT/CC Guide to Coordinated Vulnerability Disclosure](https://certcc.github.io/CERT-Guide-to-CVD/): 2 requirements, 2 tests
- DNV-RP-G108 Cyber security in the oil and gas industry: 1 requirement, 1 test
- [EEMUA 191 Alarm systems guide](https://www.eemua.org/products/publications/digital/eemua-publication-191): 2 requirements, 2 tests
- GSMA guidelines (FS and NG series): 5 requirements, 4 tests
- [IOGP reports](https://idms.iogp.org/): 1 requirement, 1 test
- [OWASP Cheat Sheet Series and guides](https://cheatsheetseries.owasp.org/) (OWASP Cheat Sheets): 49 requirements, 45 tests
- [OWASP Web Security Testing Guide](https://owasp.org/www-project-web-security-testing-guide/) (OWASP WSTG): 7 requirements, 25 tests
- [PIC/S guidance on data management and integrity](https://picscheme.org/en/publications): 1 requirement, 1 test
- Platform design guidelines (Material Design, Apple Human Interface Guidelines): 5 requirements, 5 tests
  - [Material Design](https://m3.material.io/) · [Apple Human Interface Guidelines](https://developer.apple.com/design/human-interface-guidelines/)
- [UN Recommendations on the Transport of Dangerous Goods, Manual of Tests and Criteria](https://unece.org/transport/dangerous-goods/un-manual-tests-and-criteria): 1 requirement, 1 test
- [W3C Web Sustainability Guidelines (WSG)](https://www.w3.org/TR/web-sustainability-guidelines/): 3 requirements, 2 tests
- [Web performance guidance (web.dev, HTTP Archive, Google Web Fundamentals)](https://web.dev/): 7 requirements, 4 tests
  - [web.dev Core Web Vitals](https://web.dev/articles/vitals) · [web.dev Performance budgets](https://web.dev/articles/performance-budgets-101) · [HTTP Archive Web Almanac 2024, Page Weight chapter](https://almanac.httparchive.org/en/2024/page-weight)

#### Guidelines and guidance: European Union

- [CEN/TR 16355 Recommendations for prevention of Legionella growth in installations inside buildings conveying water for human consumption](https://standards.cencenelec.eu/): 1 requirement, 1 test
- [EDPB and Article 29 Working Party guidelines](https://www.edpb.europa.eu/documents_en): 4 requirements, 4 tests
  - [EDPB Guidelines 9/2022](https://www.edpb.europa.eu/documents/guideline/guidelines-92022-on-personal-data-breach-notification-under-gdpr_en)
- [ENISA guidance](https://www.enisa.europa.eu/publications): 2 requirements, 2 tests
- [EU Good Manufacturing Practice guidelines (EudraLex Volume 4)](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en) (EU GMP): 14 requirements, 14 tests
- [The Blue Guide on the implementation of EU product rules (2022/C 247/01)](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52022XC0629%2804%29): 2 requirements, 2 tests

#### Guidelines and guidance: United Kingdom

- [UK HSE HSG250, Guidance on permit-to-work systems](https://www.hse.gov.uk/pubns/books/hsg250.htm): 2 requirements, 2 tests

#### Guidelines and guidance: United States

- [CISA guidance and catalogues (KEV, VEX)](https://www.cisa.gov/known-exploited-vulnerabilities-catalog): 3 requirements, 3 tests
  - [CISA Known Exploited Vulnerabilities Catalog](https://www.cisa.gov/known-exploited-vulnerabilities-catalog) · [CISA Minimum Requirements for VEX](https://www.cisa.gov/resources-tools/resources/minimum-requirements-vulnerability-exploitability-exchange-vex)
- [NIST SP 800-190 Application container security guide](https://doi.org/10.6028/NIST.SP.800-190): 1 requirement, 1 test
- [NIST SP 800-207 Zero trust architecture](https://doi.org/10.6028/NIST.SP.800-207): 2 requirements, 2 tests
- [NIST SP 800-209 Security guidelines for storage infrastructure](https://doi.org/10.6028/NIST.SP.800-209): 1 requirement, 1 test
- [NIST SP 800-34 Contingency planning guide](https://doi.org/10.6028/NIST.SP.800-34r1): 1 requirement, 1 test
- [NIST SP 800-61 Computer security incident handling guide](https://doi.org/10.6028/NIST.SP.800-61r2): 6 requirements, 6 tests
- [NIST SP 800-63 Digital Identity Guidelines (63A, 63B)](https://pages.nist.gov/800-63-3/): 20 requirements, 20 tests
- [NIST SP 800-82 Guide to operational technology security](https://doi.org/10.6028/NIST.SP.800-82r3): 5 requirements, 4 tests
- [NIST SP 800-84 Test, training and exercise programmes](https://doi.org/10.6028/NIST.SP.800-84): 1 requirement, 2 tests
- [NIST SP 800-88 Guidelines for media sanitization](https://doi.org/10.6028/NIST.SP.800-88r2): 3 requirements, 3 tests
- [NIST system and platform security guidance (SP 800-160, 800-193, 800-90B)](https://doi.org/10.6028/NIST.SP.800-160v1r1): 7 requirements, 6 tests
  - [NIST SP 800-160 Vol. 1 Rev. 1](https://doi.org/10.6028/NIST.SP.800-160v1r1) · [NIST SP 800-193](https://doi.org/10.6028/NIST.SP.800-193) · [NIST SP 800-90B](https://doi.org/10.6028/NIST.SP.800-90B)
- [NTIA Minimum Elements for a Software Bill of Materials](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom) (NTIA SBOM): 1 requirement, 1 test

<!-- END generated sources -->
