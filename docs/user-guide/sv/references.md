# Referenser

Standarder, riktlinjer och andra dokument som ligger bakom Qtestframes sätt att skriva krav och testfall. Använd dem när du inför en process för kravspecifikationer, utbildar kravskrivare eller behöver motivera en regel för en granskare. Länkarna går till utgivaren. ISO- och IEEE-standarder är inte fritt tillgängliga, men deras katalogsidor beskriver omfattningen. Dokumenten är på engelska om inget annat anges.

## Kravord (RFC)

Dessa dokument definierar kravnivåerna och [språkanalysen](requirements.md#språkanalysen) i Qtestframe.

| Dokument | Innehåll | Används i Qtestframe för |
|---|---|---|
| [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) – *Key words for use in RFCs to Indicate Requirement Levels* (S. Bradner, 1997) | Definierar **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY** och **OPTIONAL**, och varnar för att använda dem där de inte behövs. | Kravnivåerna (*must*, *shall*, *should*, *may*, *optional* och deras negationer) och identifieringen av kravord |
| [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) – *Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words* (B. Leiba, 2017) | Förtydligar att bara formerna med **VERSALER** har betydelsen enligt RFC 2119; ord med gemener har sin vanliga engelska betydelse. | Regeln *Kravord med gemener* |
| [BCP 14](https://www.rfc-editor.org/info/bcp14) | "Best current practice" som består av RFC 2119 och RFC 8174. Specifikationer hänvisar till den med en standardmening: *The key words "MUST", "MUST NOT", … are to be interpreted as described in BCP 14 [RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown here.* | Skriv en sådan mening i inledningen till din specifikation |
| [RFC 7322](https://www.rfc-editor.org/rfc/rfc7322) – *RFC Style Guide* (2014) | Redaktionella regler för RFC:er, bland annat hur BCP 14 citeras. | Förebild för en egen skrivhandledning |
| [RFC 3552](https://www.rfc-editor.org/rfc/rfc3552) – *Guidelines for Writing RFC Text on Security Considerations* (BCP 72, 2003) | Hur hot analyseras och beskrivs i en specifikation. | Att skriva *säkerhetskrav* |
| [RFC 6919](https://www.rfc-editor.org/rfc/rfc6919) – *Further Key Words for Use in RFCs to Indicate Requirement Levels* (2013) | En aprilskämts-RFC med kravord som *MUST (BUT WE KNOW YOU WON'T)*. | En humoristisk påminnelse om varför tvetydiga formuleringar inte fungerar |

### Andra RFC:er som Qtestframe följer

| Dokument | Var |
|---|---|
| [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) – *HTTP Semantics* | REST-API:t |
| [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259) – *JSON* | Anrop och svar i API:t, säkerhetskopior |
| [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339) – *Date and Time on the Internet: Timestamps* | Datum i API:t, i frågor (`updated_at>=2026-01-01`) och i händelseloggens filter |
| [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180) – *CSV Files* | CSV-export |
| [RFC 5424](https://www.rfc-editor.org/rfc/rfc5424) – *The Syslog Protocol* | Serverns loggning till syslog |
| [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280) – *X.509 Certificate and CRL Profile* | TLS-certifikat och certifikatskripten |
| [RFC 6265](https://www.rfc-editor.org/rfc/rfc6265) – *HTTP State Management Mechanism* (kakor) | Sessionskakor |
| [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) – *OAuth 2.0* och [RFC 7519](https://www.rfc-editor.org/rfc/rfc7519) – *JSON Web Token* | Enkel inloggning med [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html) |
| [SAML 2.0 Core](https://docs.oasis-open.org/security/saml/v2.0/saml-core-2.0-os.pdf) (OASIS) | Enkel inloggning med SAML |

## NIST-publikationer

Amerikanska National Institute of Standards and Technology ger ut fritt tillgängliga vägledningar som många organisationer använder som kravkataloger och testunderlag, särskilt för säkerhet.

| Dokument | Relevans |
|---|---|
| [NIST SP 800-53 Rev. 5](https://doi.org/10.6028/NIST.SP.800-53r5) – *Security and Privacy Controls for Information Systems and Organizations* (2020) | En katalog med säkerhetsåtgärder som ofta importeras som säkerhetskrav. Håll ordning på dem med kategorier (till exempel *Åtgärdsfamilj / AC*). |
| [NIST SP 800-53A Rev. 5](https://doi.org/10.6028/NIST.SP.800-53Ar5) – *Assessing Security and Privacy Controls in Information Systems and Organizations* (2022) | Bedömningsmetoder per åtgärd med metoderna *granska*, *intervjua* och *testa*; de motsvarar metoderna i [utvärderingskriterierna](requirements.md#utvärderingskriterier). |
| [NIST SP 800-115](https://doi.org/10.6028/NIST.SP.800-115) – *Technical Guide to Information Security Testing and Assessment* (2008) | Att planera och genomföra säkerhetstester: granskning, identifiering av mål, verifiering av sårbarheter. |
| [NIST SP 800-160 Vol. 1 Rev. 1](https://doi.org/10.6028/NIST.SP.800-160v1r1) – *Engineering Trustworthy Secure Systems* (2022) | Säkerhetsinriktad systemutveckling, bland annat intressenters behov, kravdefinition och verifiering, i linje med ISO/IEC/IEEE 15288. |
| [NIST SP 800-218](https://doi.org/10.6028/NIST.SP.800-218) – *Secure Software Development Framework (SSDF) Version 1.1* (2022) | Arbetssätt för säker utveckling, bland annat att definiera säkerhetskrav och testa dem. |
| [NIST IR 8397](https://doi.org/10.6028/NIST.IR.8397) – *Guidelines on Minimum Standards for Developer Verification of Software* (2021) | Minimikrav på verifiering: hotmodellering, automatiserade tester, statisk och dynamisk analys, fuzzning med mera. |
| [NIST SP 800-142](https://doi.org/10.6028/NIST.SP.800-142) – *Practical Combinatorial Testing* (2010) | Att ta fram ett litet antal testfall som täcker samspelet mellan parametrar. |
| [NIST SP 800-63B-4](https://doi.org/10.6028/NIST.SP.800-63B-4) – *Digital Identity Guidelines: Authentication and Authenticator Management* (2025) | Krav på lösenord, flerfaktorsautentisering och sessioner. |
| [NIST SP 800-92](https://doi.org/10.6028/NIST.SP.800-92) – *Guide to Computer Security Log Management* (2006) | Krav på händelse- och säkerhetsloggning. |
| [NIST CSWP 29](https://doi.org/10.6028/NIST.CSWP.29) – *The NIST Cybersecurity Framework (CSF) 2.0* (2024) | Övergripande mål som kan brytas ned i krav. |
| [NIST Planning Report 02-3](https://www.nist.gov/system/files/documents/director/planning/report02-3.pdf) – *The Economic Impacts of Inadequate Infrastructure for Software Testing* (2002) | En klassisk studie av vad det kostar att hitta fel sent. |

## Standarder och vägledningar för kravhantering

| Dokument | Relevans |
|---|---|
| [ISO/IEC/IEEE 29148:2018](https://www.iso.org/standard/72089.html) – *Systems and software engineering — Life cycle processes — Requirements engineering* | Den viktigaste internationella standarden för krav: egenskaper hos bra krav (nödvändiga, entydiga, fullständiga, odelbara, genomförbara, verifierbara, korrekta), kravattribut och specifikationens innehåll. Ersätter IEEE 830. |
| [ISO/IEC Directives, Part 2](https://www.iso.org/sites/directives/current/part2/index.xhtml) – *Principles and rules for the structure and drafting of ISO and IEC documents* | Definierar verbformerna i internationella standarder: **shall** (krav), **should** (rekommendation), **may** (tillåtelse), **can** (möjlighet). Svenska standarder från [SIS](https://www.sis.se/) använder *ska*, *bör*, *får* och *kan*, vilket Qtestframes svenska språkanalys följer. |
| [INCOSE Guide to Writing Requirements](https://www.incose.org/docs/default-source/working-groups/requirements-wg/guidetowritingrequirements/incose_rwg_gtwr_v4_summary_sheet.pdf) (version 4, 2023; länken går till den fria sammanfattningen) | Regler för välformulerade krav: aktiv form, inga vaga ord, inga undantagsklausuler, undvik "inte", en tanke per krav. Språkanalysen kontrollerar flera av reglerna. |
| [EARS – Easy Approach to Requirements Syntax](https://doi.org/10.1109/RE.2009.9) (A. Mavin m.fl., IEEE RE 2009) | Enkla meningsmönster: *Medan …, när …, ska systemet …*. |
| [IREB CPRE Foundation Level](https://www.ireb.org/en/cpre/foundation/) | Kursplan och ordlista för certifiering i kravhantering. |
| [ISO/IEC 25010:2023](https://www.iso.org/standard/78176.html) – *Product quality model* | Kvalitetsegenskaper (prestanda, användbarhet, säkerhet, tillförlitlighet …) som fungerar bra som checklista för icke-funktionella kravtyper. |

## Standarder och vägledningar för test

| Dokument | Relevans |
|---|---|
| [ISO/IEC/IEEE 29119](https://committee.iso.org/sites/jtc1sc7/home/projects/flagship-standards/isoiecieee-29119-series.html) – serien *Software testing*, särskilt [Part 1: General concepts](https://www.iso.org/standard/81291.html) (2022) och [Part 3: Test documentation](https://www.iso.org/standard/79429.html) (2021) | Testprocesser, dokumentmallar (testplan, testfall, testlogg, incidentrapport) och testtekniker. Del 3 ersätter IEEE 829. |
| [IEEE 1012-2016](https://standards.ieee.org/ieee/1012/5609/) – *System, Software, and Hardware Verification and Validation* | Verifierings- och valideringsaktiviteter och integritetsnivåer. |
| [ISTQB Glossary](https://glossary.istqb.org/) och [Certified Tester Foundation Level v4.0](https://www.istqb.org/certifications/certified-tester-foundation-level-ctfl-v4-0/) | Den gemensamma testterminologin (testfall, testkörning, defekt, täckning …). |
| [OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/) | En färdig lista med testbara säkerhetskrav för webbapplikationer, i tre nivåer. |
| [OWASP Web Security Testing Guide](https://owasp.org/www-project-web-security-testing-guide/) | Hur säkerheten i webbapplikationer testas; kompletterar ASVS. |
| [WCAG 2.2](https://www.w3.org/TR/WCAG22/) (och [WCAG 2.1](https://www.w3.org/TR/WCAG21/)) – *Web Content Accessibility Guidelines* | Testbara framgångskriterier för tillgänglighet. Qtestframe siktar själv på WCAG 2.1 AA, se [Tillgänglighet och språk](accessibility-and-languages.md). |

## Branschspecifika standarder

Reglerade branscher har egna regler för krav och verifiering. Qtestframes kravnivåer, utvärderingskriterier, baslinjer, versionshistorik och händelselogg är användbara där, men kontrollera standarden för din bransch, till exempel *DO-178C* (programvara i luftfartyg), *IEC 62304* (programvara i medicintekniska produkter), *ISO 26262* (vägfordon), *IEC 61508* (funktionell säkerhet) eller *EN 50128* (programvara för järnväg).

<!-- BEGIN generated sources: make refs-docs -->

## Källor som katalogen hänvisar till

[Kravkatalogen](catalogue.md) hänvisar till 370 lagar, standarder, bästa praxis och vägledningar. Vid varje källa står hur många krav i katalogen som hänvisar till den och hur många testfall i katalogen som hänvisar till den eller verifierar ett sådant krav, samma antal som när du väljer källor att uppfylla i katalogen. När en källa samlar flera dokument listas de under den. Avsnittet genereras från katalogen.

### Lagar och förordningar

#### Lagar och förordningar: Internationella

- [FN-föreskrift nr 155 – Cybersäkerhet och ledningssystem för cybersäkerhet](https://unece.org/transport/documents/2021/03/standards/un-regulation-no-155-cyber-security-and-cyber-security) (UN R155): 2 krav, 2 testfall
- [FN-föreskrift nr 156 – Programuppdateringar och ledningssystem för programuppdateringar](https://unece.org/transport/documents/2021/03/standards/un-regulation-no-156-software-update-and-software-update) (UN R156): 4 krav, 4 testfall

#### Lagar och förordningar: Europeiska unionen

- [AI-förordningen (EU) 2024/1689](https://eur-lex.europa.eu/eli/reg/2024/1689/oj) (AI Act): 40 krav, 29 testfall
- [Batteriförordningen (EU) 2023/1542](https://eur-lex.europa.eu/eli/reg/2023/1542/oj): 3 krav, 3 testfall
- [Beslut nr 768/2008/EG om en gemensam ram för saluföring av produkter](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32008D0768) (Decision 768/2008/EC): 4 krav, 3 testfall
- [CLP-förordningen (EG) nr 1272/2008 om klassificering, märkning och förpackning](https://eur-lex.europa.eu/eli/reg/2008/1272/oj): 2 krav, 2 testfall
- [Cyberresiliensförordningen (EU) 2024/2847](https://eur-lex.europa.eu/eli/reg/2024/2847/oj) (CRA): 100 krav, 95 testfall
- [Dataförordningen (EU) 2023/2854](https://eur-lex.europa.eu/eli/reg/2023/2854/oj) (Data Act): 9 krav, 6 testfall
- [Dataskyddsförordningen (EU) 2016/679](https://eur-lex.europa.eu/eli/reg/2016/679/oj) (GDPR): 97 krav, 95 testfall
- [Digital omnibusförordning om AI, förordning (EU) 2026/1744 om ändring av AI-förordningen](https://eur-lex.europa.eu/eli/reg/2026/1744/oj) (Digital Omnibus on AI): 5 krav, 4 testfall
- [E-PRTR-förordningen (EG) nr 166/2006](https://eur-lex.europa.eu/eli/reg/2006/166/oj): 1 krav, 1 testfall
- [eIDAS-förordningen (EU) nr 910/2014](https://eur-lex.europa.eu/eli/reg/2014/910/oj): 7 krav, 7 testfall
- [EU:s nätföreskrifter och riktlinjer för el (SO GL, EB GL, NC ER)](https://eur-lex.europa.eu/eli/reg/2017/1485/oj): 4 krav, 4 testfall
  - [Commission Regulation (EU) 2017/1485](https://eur-lex.europa.eu/eli/reg/2017/1485/oj) · [Commission Regulation (EU) 2017/2195](https://eur-lex.europa.eu/eli/reg/2017/2195/oj) · [Commission Regulation (EU) 2017/2196](https://eur-lex.europa.eu/eli/reg/2017/2196/oj)
- [Förordning (EG) nr 765/2008 om ackreditering och CE-märkning](https://eur-lex.europa.eu/eli/reg/2008/765/oj): 1 krav, 1 testfall
- [Förordningen om digital operativ motståndskraft (DORA), (EU) 2022/2554](https://eur-lex.europa.eu/eli/reg/2022/2554/oj): 50 krav, 42 testfall
- [Förordningen om digitala tjänster (EU) 2022/2065](https://eur-lex.europa.eu/eli/reg/2022/2065/oj) (DSA): 1 krav, 1 testfall
- [Förordningen om ekodesign för hållbara produkter (EU) 2024/1781](https://eur-lex.europa.eu/eli/reg/2024/1781/oj) (ESPR): 5 krav, 4 testfall
- [Förordningen om en gemensam digital ingång (EU) 2018/1724](https://eur-lex.europa.eu/eli/reg/2018/1724/oj) (SDG): 5 krav, 5 testfall
- [Förordningen om ett kompatibelt Europa (EU) 2024/903](https://eur-lex.europa.eu/eli/reg/2024/903/oj): 1 krav, 1 testfall
- [Förordningen om medicintekniska produkter (EU) 2017/745](https://eur-lex.europa.eu/eli/reg/2017/745/oj) (MDR): 6 krav, 6 testfall
- [Förordningen om metanutsläpp i energisektorn (EU) 2024/1787](https://eur-lex.europa.eu/eli/reg/2024/1787/oj): 3 krav, 3 testfall
- [Förordningen om öppen internetanslutning (EU) 2015/2120](https://eur-lex.europa.eu/eli/reg/2015/2120/oj): 1 krav, 1 testfall
- [Förordningen om övervakning och rapportering inom EU ETS, (EU) 2018/2066](https://eur-lex.europa.eu/eli/reg_impl/2018/2066/oj): 1 krav, 1 testfall
- [Genomförandeförordningen till NIS2, (EU) 2024/2690](https://eur-lex.europa.eu/eli/reg_impl/2024/2690/oj) (NIS2 IR): 13 krav, 15 testfall
- [Kommissionens förordning (EU) 2019/1782 om ekodesignkrav för externa nätaggregat](https://eur-lex.europa.eu/eli/reg/2019/1782/oj): 1 krav, 1 testfall
- [Kommissionens förordning (EU) 2023/826 om ekodesignkrav för frånläge, standbyläge och nätverksanslutet standbyläge](https://eur-lex.europa.eu/eli/reg/2023/826/oj): 1 krav, 1 testfall
- [Maskinförordningen (EU) 2023/1230](https://eur-lex.europa.eu/eli/reg/2023/1230/oj): 25 krav, 21 testfall
- [Nätföreskrifter för cybersäkerhet, delegerad förordning (EU) 2024/1366](https://eur-lex.europa.eu/eli/reg_del/2024/1366/oj) (NCCS): 2 krav, 2 testfall
- [Reach-förordningen (EG) nr 1907/2006](https://eur-lex.europa.eu/eli/reg/2006/1907/oj) (REACH): 3 krav, 3 testfall
- [REMIT, förordning (EU) nr 1227/2011 om integritet och öppenhet på grossistmarknaderna för energi](https://eur-lex.europa.eu/eli/reg/2011/1227/oj): 1 krav, 1 testfall

#### Lagar och förordningar: Sverige

- [Arbetsmiljöverkets föreskrifter (AFS 2023:1–2023:15)](https://www.av.se/arbetsmiljoarbete-och-inspektioner/publikationer/foreskrifter/): 4 krav, 3 testfall
- [Arkivlag (1990:782)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/arkivlag-1990782_sfs-1990-782/): 2 krav, 1 testfall
- [Bokföringslagen (1999:1078)](https://lagen.nu/1999:1078): 3 krav, 3 testfall
- [Boverkets byggregler (BFS 2024:4–2024:13, BFS 2026:4 och 2026:9), som har ersatt BBR (BFS 2011:6) och EKS](https://www.boverket.se/sv/byggande/regler-for-byggande/om-boverkets-nya-byggregler/): 9 krav, 6 testfall
- [Cybersäkerhetslag (2025:1506), som genomför NIS2](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/cybersakerhetslag-20251506_sfs-2025-1506/): 9 krav, 7 testfall
- [Elberedskapslag (1997:288)](https://lagen.nu/1997:288): 1 krav, 1 testfall
- [Elsäkerhetslag (2016:732)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/elsakerhetslag-2016732_sfs-2016-732/): 2 krav, 1 testfall
- [Funktionskontroll av ventilationssystem: plan- och byggförordningen (2011:338) 5 kap. 1–7 §§ och Boverkets BFS 2011:16 (OVK)](https://www.boverket.se/sv/PBL-kunskapsbanken/regler-om-byggande/ovk/): 2 krav, 2 testfall
- [Förvaltningslag (2017:900)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/forvaltningslag-2017900_sfs-2017-900/): 1 krav, 1 testfall
- [Kamerabevakningslag (2018:1200)](https://lagen.nu/2018:1200): 1 krav, 1 testfall
- [Kontroll av hissar och andra motordrivna anordningar: plan- och byggförordningen (2011:338) 5 kap. 8–11 §§ och Boverkets BFS 2025:11 och 2025:12](https://www.boverket.se/sv/PBL-kunskapsbanken/regler-om-byggande/motordrivna-anordningar/hissar/): 1 krav, 1 testfall
- [Lag (2003:778) om skydd mot olyckor](https://lagen.nu/2003:778) (LSO): 5 krav, 5 testfall
- [Lag (2006:985) om energideklaration för byggnader](https://lagen.nu/2006:985): 1 krav, 1 testfall
- [Lag (2010:1011) om brandfarliga och explosiva varor](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/lag-20101011-om-brandfarliga-och-explosiva_sfs-2010-1011/): 1 krav, 1 testfall
- [Lag (2016:1145) om offentlig upphandling (LOU)](https://lagen.nu/2016:1145): 4 krav, 3 testfall
- [Lag (2018:218) med kompletterande bestämmelser till EU:s dataskyddsförordning](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/lag-2018218-med-kompletterande-bestammelser_sfs-2018-218/): 1 krav, 1 testfall
- [Lag (2021:890) om skydd för personer som rapporterar om missförhållanden](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/lag-2021890-om-skydd-for-personer-som_sfs-2021-890/): 1 krav, 1 testfall
- [Lagen (2018:1937) om tillgänglighet till digital offentlig service och DIGG:s föreskrifter](https://lagen.nu/2018:1937) (DOS-lagen): 4 krav, 4 testfall
  - [Lag (2018:1937)](https://lagen.nu/2018:1937) · [DIGG MDFFS 2019:2](https://www.digg.se/om-oss/forfattningssamling/foreskrifter-om-tillganglighet-till-digital-offentlig-service-mdffs-20192/grundforfattning-mdffs-20192)
- [Lagen (2022:482) om elektronisk kommunikation](https://lagen.nu/2022:482) (LEK): 2 krav, 2 testfall
- [Marknadsföringslagen (2008:486)](https://lagen.nu/2008:486): 2 krav, 2 testfall
- [Offentlighets- och sekretesslag (2009:400)](https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/offentlighets-och-sekretesslag-2009400_sfs-2009-400/) (OSL): 2 krav, 2 testfall
- [Patientdatalagen (2008:355)](https://lagen.nu/2008:355): 1 krav, 1 testfall
- [Språklag (2009:600)](https://lagen.nu/2009:600): 3 krav, 3 testfall
- [Säkerhetsskyddslagen (2018:585), säkerhetsskyddsförordningen (2021:955) och föreskrifter](https://lagen.nu/2018:585): 20 krav, 17 testfall
  - [Säkerhetsskyddslag (2018:585)](https://lagen.nu/2018:585) · [Säkerhetsskyddsförordning (2021:955)](https://lagen.nu/2021:955)

#### Lagar och förordningar: USA

- [Amerikanska FDA 21 CFR Part 11 (elektroniska register och underskrifter)](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11): 15 krav, 14 testfall
- [HIPAA Security Rule (45 CFR del 164)](https://www.ecfr.gov/current/title-45/subtitle-A/subchapter-C/part-164/subpart-C): 1 krav, 1 testfall
- [OSHA processäkerhet, 29 CFR 1910.119](https://www.osha.gov/laws-regs/regulations/standardnumber/1910/1910.119): 1 krav, 1 testfall
- [USA:s Code of Federal Regulations avdelning 47 (FCC), del 2 och 15](https://www.ecfr.gov/current/title-47) (FCC 47 CFR): 1 krav, 1 testfall

### Direktiv

#### Direktiv: Europeiska unionen

- [Arbetsutrustningsdirektivet 2009/104/EG](https://eur-lex.europa.eu/eli/dir/2009/104/oj): 5 krav, 4 testfall
- [ATEX-direktivet om arbetsplatser 1999/92/EG](https://eur-lex.europa.eu/eli/dir/1999/92/oj) (ATEX 153): 1 krav, 1 testfall
- [Avfallsdirektivet 2008/98/EG](https://eur-lex.europa.eu/eli/dir/2008/98/oj): 1 krav, 1 testfall
- [Betaltjänstdirektivet (EU) 2015/2366 (PSD2) med tekniska standarder för stark kundautentisering](https://eur-lex.europa.eu/eli/dir/2015/2366/oj): 2 krav, 2 testfall
  - [Directive (EU) 2015/2366](https://eur-lex.europa.eu/eli/dir/2015/2366/oj) · [Commission Delegated Regulation (EU) 2018/389](https://eur-lex.europa.eu/eli/reg_del/2018/389/oj)
- [Buller i arbetet, direktiv 2003/10/EG om arbetstagares exponering för risker som har samband med buller](https://eur-lex.europa.eu/eli/dir/2003/10/oj): 1 krav, 1 testfall
- [Direktivet om avfall som utgörs av eller innehåller elektriska eller elektroniska produkter (WEEE) 2012/19/EU](https://eur-lex.europa.eu/eli/dir/2012/19/oj): 6 krav, 5 testfall
- [Direktivet om byggnaders energiprestanda (EU) 2024/1275](https://eur-lex.europa.eu/eli/dir/2024/1275/oj) (EPBD): 3 krav, 3 testfall
- [Direktivet om integritet och elektronisk kommunikation 2002/58/EG](https://eur-lex.europa.eu/eli/dir/2002/58/oj) (ePrivacy): 15 krav, 15 testfall
- [Direktivet om kritiska entiteters motståndskraft (CER), (EU) 2022/2557](https://eur-lex.europa.eu/eli/dir/2022/2557/oj): 14 krav, 12 testfall
- [Direktivet om offentlig upphandling 2014/24/EU](https://eur-lex.europa.eu/eli/dir/2014/24/oj): 5 krav, 4 testfall
- [Direktivet om oskäliga avtalsvillkor 93/13/EEG](https://eur-lex.europa.eu/eli/dir/1993/13/oj): 1 krav, 1 testfall
- [Direktivet om otillbörliga affärsmetoder 2005/29/EG](https://eur-lex.europa.eu/eli/dir/2005/29/oj) (UCPD): 4 krav, 4 testfall
- [Direktivet om tryckbärande anordningar 2014/68/EU](https://eur-lex.europa.eu/eli/dir/2014/68/oj) (PED): 14 krav, 9 testfall
- [Elmarknadsdirektivet (EU) 2019/944](https://eur-lex.europa.eu/eli/dir/2019/944/oj): 4 krav, 4 testfall
- [EMC-direktivet 2014/30/EU](https://eur-lex.europa.eu/eli/dir/2014/30/oj): 12 krav, 9 testfall
- [Energieffektivitetsdirektivet (EU) 2023/1791](https://eur-lex.europa.eu/eli/dir/2023/1791/oj): 2 krav, 2 testfall
- [Europeiska kodexen för elektronisk kommunikation, direktiv (EU) 2018/1972](https://eur-lex.europa.eu/eli/dir/2018/1972/oj) (EECC): 11 krav, 9 testfall
  - [Directive (EU) 2018/1972](https://eur-lex.europa.eu/eli/dir/2018/1972/oj) · [Commission Delegated Regulation (EU) 2023/444](https://eur-lex.europa.eu/eli/reg_del/2023/444/oj)
- [Gemensam laddare, direktiv (EU) 2022/2380 om ändring av radioutrustningsdirektivet](https://eur-lex.europa.eu/eli/dir/2022/2380/oj): 1 krav, 1 testfall
- [Hissdirektivet 2014/33/EU](https://eur-lex.europa.eu/eli/dir/2014/33/oj): 1 krav, 1 testfall
- [Industriutsläppsdirektivet 2010/75/EU](https://eur-lex.europa.eu/eli/dir/2010/75/oj) (IED): 2 krav, 1 testfall
- [Konsumenträttighetsdirektivet 2011/83/EU (ändrat genom direktiv (EU) 2023/2673)](https://eur-lex.europa.eu/eli/dir/2011/83/oj): 11 krav, 9 testfall
  - [Directive 2011/83/EU](https://eur-lex.europa.eu/eli/dir/2011/83/oj) · [Directive (EU) 2023/2673](https://eur-lex.europa.eu/eli/dir/2023/2673/oj)
- [Lågspänningsdirektivet 2014/35/EU](https://eur-lex.europa.eu/eli/dir/2014/35/oj) (LVD): 11 krav, 8 testfall
- [Mervärdesskattedirektivet 2006/112/EG](https://eur-lex.europa.eu/eli/dir/2006/112/oj): 9 krav, 8 testfall
- [Mätinstrumentdirektivet 2014/32/EU](https://eur-lex.europa.eu/eli/dir/2014/32/oj) (MID): 2 krav, 2 testfall
- [NIS2-direktivet (EU) 2022/2555](https://eur-lex.europa.eu/eli/dir/2022/2555/oj): 30 krav, 29 testfall
- [Prisinformationsdirektivet 98/6/EG (ändrat genom direktiv (EU) 2019/2161)](https://eur-lex.europa.eu/eli/dir/1998/6/oj): 1 krav, 1 testfall
- [Radioutrustningsdirektivet 2014/53/EU med delegerad förordning (EU) 2022/30 och EN 18031](https://eur-lex.europa.eu/eli/dir/2014/53/oj) (RED): 23 krav, 23 testfall
  - [Directive 2014/53/EU](https://eur-lex.europa.eu/eli/dir/2014/53/oj) · [Commission Delegated Regulation (EU) 2022/30](https://eur-lex.europa.eu/eli/reg_del/2022/30/oj) · [EN 18031](https://single-market-economy.ec.europa.eu/single-market/goods/european-standards/harmonised-standards/radio-equipment_en)
- [RoHS-direktivet 2011/65/EU om farliga ämnen i elektrisk och elektronisk utrustning](https://eur-lex.europa.eu/eli/dir/2011/65/oj): 3 krav, 2 testfall
- [Seveso III-direktivet 2012/18/EU](https://eur-lex.europa.eu/eli/dir/2012/18/oj): 3 krav, 2 testfall
- [Tillgänglighetsdirektivet (EU) 2019/882](https://eur-lex.europa.eu/eli/dir/2019/882/oj) (EAA): 2 krav, 2 testfall
- [Visselblåsardirektivet (EU) 2019/1937](https://eur-lex.europa.eu/eli/dir/2019/1937/oj) (Whistleblower Directive): 1 krav, 1 testfall
- [Webbtillgänglighetsdirektivet (EU) 2016/2102](https://eur-lex.europa.eu/eli/dir/2016/2102/oj): 4 krav, 4 testfall
  - [Directive (EU) 2016/2102](https://eur-lex.europa.eu/eli/dir/2016/2102/oj) · [Commission Implementing Decision (EU) 2018/1523](https://eur-lex.europa.eu/eli/dec_impl/2018/1523/oj)

### Standarder och specifikationer

#### Standarder och specifikationer: Internationella

- [3GPP tekniska specifikationer](https://www.3gpp.org/specifications-technologies): 19 krav, 16 testfall
  - [3GPP TS 22.179](https://www.3gpp.org/DynaReport/22179.htm) · [3GPP TS 22.280](https://www.3gpp.org/DynaReport/22280.htm) · [3GPP TS 23.167](https://www.3gpp.org/DynaReport/23167.htm) · [3GPP TS 23.283](https://www.3gpp.org/DynaReport/23283.htm) · [3GPP TS 23.501](https://www.3gpp.org/DynaReport/23501.htm) · [3GPP TS 23.682](https://www.3gpp.org/DynaReport/23682.htm) · [3GPP TS 28.552](https://www.3gpp.org/DynaReport/28552.htm) · [3GPP TS 32.240](https://www.3gpp.org/DynaReport/32240.htm) · [3GPP TS 32.298](https://www.3gpp.org/DynaReport/32298.htm) · [3GPP TS 32.401](https://www.3gpp.org/DynaReport/32401.htm) · [3GPP TS 33.126](https://www.3gpp.org/DynaReport/33126.htm) · [3GPP TS 33.127](https://www.3gpp.org/DynaReport/33127.htm) · [3GPP TS 33.180](https://www.3gpp.org/DynaReport/33180.htm) · [3GPP TS 33.501](https://www.3gpp.org/DynaReport/33501.htm)
- [ANSI/ISA-101.01 Operatörsgränssnitt för processautomation](https://www.isa.org/standards-and-publications/isa-standards/isa-101-standards): 3 krav, 3 testfall
- [C2PA-specifikationen för innehållsursprung](https://c2pa.org/specifications/specifications/2.1/specs/C2PA_Specification.html): 2 krav, 2 testfall
- [CycloneDX-standarden för materialförteckningar](https://cyclonedx.org/specification/overview/): 4 krav, 5 testfall
- [ECMA-376 Office Open XML-filformat](https://ecma-international.org/publications-and-standards/standards/ecma-376/): 2 krav, 2 testfall
- [ECSS-standarder för produktsäkring inom rymd](https://ecss.nl/standards/): 2 krav, 2 testfall
  - [ECSS-Q-ST-30-11C](https://ecss.nl/standard/ecss-q-st-30-11c-rev-2-derating-eee-components-23-june-2021/)
- [EMC-standarder (EN 55032, EN 55035, IEC 61000-4)](https://webstore.iec.ch/en/publication/22046): 7 krav, 7 testfall
  - [CISPR 32 (EN 55032)](https://webstore.iec.ch/en/publication/22046) · [CISPR 35 (EN 55035)](https://webstore.iec.ch/en/publication/25667) · [IEC 61000-4-2](https://webstore.iec.ch/en/publication/68954) · [IEC 61000-4-11](https://webstore.iec.ch/en/publication/63503) · [IEC 61000-4-6](https://webstore.iec.ch/en/publication/65586)
- [ETSI NFV-specifikationer (virtualisering av nätfunktioner)](https://www.etsi.org/technologies/nfv): 1 krav, 1 testfall
  - [ETSI GS NFV-SEC 001](https://www.etsi.org/deliver/etsi_gs/NFV-SEC/001_099/001/) · [ETSI GS NFV-SEC 021](https://www.etsi.org/deliver/etsi_gs/NFV-SEC/001_099/021/) · [ETSI GS NFV-SOL 004](https://www.etsi.org/deliver/etsi_gs/NFV-SOL/001_099/004/)
- [ETSI tekniska specifikationer (laglig avlyssning, lagrade data, nödpositionering)](https://www.etsi.org/standards) (ETSI TS): 7 krav, 6 testfall
  - [ETSI TS 102 232-1](https://www.etsi.org/deliver/etsi_ts/102200_102299/10223201/) · [ETSI TS 102 657](https://www.etsi.org/deliver/etsi_ts/102600_102699/102657/) · [ETSI TS 103 221-1](https://www.etsi.org/deliver/etsi_ts/103200_103299/10322101/) · [ETSI TS 103 221-2](https://www.etsi.org/deliver/etsi_ts/103200_103299/10322102/) · [ETSI TS 103 625](https://www.etsi.org/deliver/etsi_ts/103600_103699/103625/)
- [FIRST:s standarder och riktlinjer (CVSS, EPSS, samordnat offentliggörande)](https://www.first.org/standards/): 4 krav, 5 testfall
  - [FIRST CVSS v4.0](https://www.first.org/cvss/v4.0/specification-document) · [FIRST EPSS](https://www.first.org/epss/) · [FIRST Guidelines and Practices for Multi-Party Vulnerability Coordination](https://www.first.org/global/sigs/vulnerability-coordination/multiparty/guidelines-v1.1)
- [Greenhouse Gas Protocol](https://ghgprotocol.org/) (GHG Protocol): 1 krav, 2 testfall
  - [GHG Protocol Scope 3 Standard](https://ghgprotocol.org/corporate-value-chain-scope-3-standard)
- [GS1 General Specifications](https://www.gs1.org/standards/barcodes-epcrfid-id-keys/gs1-general-specifications): 1 krav, 1 testfall
- [IANA:s tidszonsdatabas](https://www.iana.org/time-zones) (tz): 6 krav, 4 testfall
- [IEC 60079 Explosiv atmosfär](https://webstore.iec.ch/en/publication/63327): 1 krav, 1 testfall
- [IEC 60118-4 Hörselslingor](https://webstore.iec.ch/en/publication/798): 1 krav, 1 testfall
- [IEC 60870 Fjärrkontrollutrustning och system](https://webstore.iec.ch/en/publication/3755): 1 krav, 1 testfall
  - [IEC 60870-5-104](https://webstore.iec.ch/en/publication/3746)
- [IEC 61082 Dokument inom elektrotekniken](https://webstore.iec.ch/en/publication/4469): 2 krav, 2 testfall
- [IEC 61131-3 Programspråk för programmerbara styrsystem](https://webstore.iec.ch/en/publication/4552): 1 krav, 1 testfall
- [IEC 61340 Elektrostatik, skydd av elektroniska komponenter mot elektrostatiska fenomen](https://webstore.iec.ch/en/publication/74748): 1 krav, 1 testfall
  - [IEC 61340-5-1](https://webstore.iec.ch/en/publication/74748)
- [IEC 61508 Funktionssäkerhet för elektriska, elektroniska och programmerbara säkerhetssystem](https://webstore.iec.ch/en/publication/5515): 16 krav, 13 testfall
  - [IEC 61508-1](https://webstore.iec.ch/en/publication/5515) · [IEC 61508-2](https://webstore.iec.ch/en/publication/5516) · [IEC 61508-3](https://webstore.iec.ch/en/publication/5517) · [IEC 61508-5](https://webstore.iec.ch/en/publication/5519) · [IEC 61508-6](https://webstore.iec.ch/en/publication/5520)
- [IEC 61511 Funktionssäkerhet inom processindustrin](https://webstore.iec.ch/en/publication/24241): 15 krav, 13 testfall
- [IEC 61850 Kommunikation för automation i elnät](https://webstore.iec.ch/en/publication/6028): 5 krav, 5 testfall
  - [IEC 61850-5](https://webstore.iec.ch/en/publication/6012) · [IEC 61850-7-1](https://webstore.iec.ch/en/publication/6014) · [IEC 61850-10](https://webstore.iec.ch/en/publication/6008)
- [IEC 62056 Datautbyte för elmätning (DLMS/COSEM)](https://webstore.iec.ch/en/publication/67915): 1 krav, 1 testfall
- [IEC 62061 Funktionssäkerhet för maskiners styrsystem](https://webstore.iec.ch/en/publication/59927): 3 krav, 3 testfall
- [IEC 62304 Programvara för medicintekniska produkter – Livscykelprocesser](https://webstore.iec.ch/en/publication/22794): 6 krav, 6 testfall
- [IEC 62351 Säkerhet för data och kommunikation i elkraftsystem](https://webstore.iec.ch/en/publication/6912): 4 krav, 3 testfall
  - [IEC 62351-5](https://webstore.iec.ch/en/publication/65511) · [IEC 62351-7](https://webstore.iec.ch/en/publication/30593)
- [IEC 62366-1 Medicintekniska produkter – Användbarhetsteknik för medicintekniska produkter](https://webstore.iec.ch/en/publication/67220): 1 krav, 1 testfall
- [IEC 62402 Hantering av inkurans](https://webstore.iec.ch/en/publication/59531): 8 krav, 6 testfall
- [IEC 62443-2-1 Krav på säkerhetsprogram för anläggningsägare](https://webstore.iec.ch/en/publication/62883): 4 krav, 4 testfall
- [IEC 62443-2-4 Krav på säkerhetsprogram för tjänsteleverantörer](https://webstore.iec.ch/en/publication/67631): 13 krav, 12 testfall
- [IEC 62443-3-2 Säkerhetsriskbedömning för systemdesign](https://webstore.iec.ch/en/publication/30727): 6 krav, 6 testfall
- [IEC 62443-3-3 Säkerhetskrav och säkerhetsnivåer för system](https://webstore.iec.ch/en/publication/7033): 170 krav, 159 testfall
- [IEC 62443-4-1 Krav på säker produktutvecklingsprocess](https://webstore.iec.ch/en/publication/33615): 45 krav, 39 testfall
- [IEC 62443-4-2 Tekniska säkerhetskrav för komponenter i industriella styrsystem](https://webstore.iec.ch/en/publication/34421): 56 krav, 56 testfall
- [IEC 62541 OPC Unified Architecture](https://reference.opcfoundation.org/) (OPC UA): 3 krav, 3 testfall
- [IEC 62740 Grundorsaksanalys (RCA)](https://webstore.iec.ch/en/publication/21810): 1 krav, 1 testfall
- [IEC 81001-5-1 Hälsoprogramvara och hälso-IT-system – Säkerhetsaktiviteter i produktens livscykel](https://webstore.iec.ch/en/publication/33615): 1 krav, 1 testfall
- [IEC 81346 Strukturering och referensbeteckningar](https://www.iso.org/standard/82229.html): 2 krav, 2 testfall
- [IEC TR 62443-2-3 Patchhantering i industriella styrsystem](https://webstore.iec.ch/en/publication/22811) (IEC 62443-2-3): 1 krav, 1 testfall
- [IEC-standarder för acceptans och idrifttagning av processutrustning (IEC 62381, IEC 62337)](https://webstore.iec.ch/en/publication/6871): 6 krav, 4 testfall
  - [IEC 62337](https://webstore.iec.ch/en/publication/6871)
- [IEC-standarder för driftsäkerhet (IEC 60300, 60812, 61025, 61078, 61124, 61163, 61703, 61709, 62506, TR 63069)](https://webstore.iec.ch/en/publication/1293): 22 krav, 18 testfall
  - [IEC 60300-1](https://webstore.iec.ch/en/publication/1293) · [IEC 60812](https://webstore.iec.ch/en/publication/26359) · [IEC 61025](https://webstore.iec.ch/en/publication/4311) · [IEC 61703](https://webstore.iec.ch/en/publication/25646) · [IEC 61709](https://webstore.iec.ch/en/publication/28554)
- [IEC-standarder för elsäkerhet hos produkter (IEC 62368-1, 60664-1, 60204-1, 60364-6)](https://webstore.iec.ch/en/publication/69308): 17 krav, 15 testfall
  - [IEC 62368-1](https://webstore.iec.ch/en/publication/69308) · [IEC 60664-1](https://webstore.iec.ch/en/publication/59671) · [IEC 60204-1](https://webstore.iec.ch/en/publication/26037) · [IEC 60364-6](https://webstore.iec.ch/en/publication/24656) · [IEC 60364-4-41](https://webstore.iec.ch/en/publication/60169)
- [IEC-standarder för sekundära litiumceller och litiumbatterier (IEC 62133, 61960, 62619)](https://webstore.iec.ch/en/publication/32662): 5 krav, 5 testfall
- [IEC/IEEE 60255-118-1 Synkrofasmätning](https://webstore.iec.ch/en/publication/28722): 1 krav, 1 testfall
- [IEC/IEEE 82079-1 Framtagning av information för användning (bruksanvisningar)](https://www.iso.org/standard/71620.html): 10 krav, 8 testfall
- [IEC:s gemensamma informationsmodell (IEC 61968, 61970, 62325)](https://webstore.iec.ch/en/publication/62698) (IEC CIM): 3 krav, 3 testfall
  - [IEC 61970-301](https://webstore.iec.ch/en/publication/62698)
- [IEEE 1149.1 Boundary scan (JTAG)](https://standards.ieee.org/ieee/1149.1/4484/): 1 krav, 1 testfall
- [IEEE 1588 Precision Time Protocol](https://standards.ieee.org/ieee/1588/6825/): 2 krav, 2 testfall
- [IEEE 1815 Distributed Network Protocol (DNP3)](https://standards.ieee.org/ieee/1815/5414/): 1 krav, 1 testfall
- [IEEE 802.11 Specifikationer för åtkomstkontroll och fysiskt lager i trådlösa lokala nät](https://standards.ieee.org/ieee/802.11/10548/): 3 krav, 1 testfall
- [IEEE 802.1X Portbaserad nätverksåtkomstkontroll](https://standards.ieee.org/ieee/802.1X/7345/): 2 krav, 2 testfall
- [IEEE 9274.1.1 Experience API (xAPI)](https://standards.ieee.org/ieee/9274.1.1/7321/): 1 krav, 1 testfall
- [IETF:s RFC:er och Internet-Drafts](https://www.rfc-editor.org/) (IETF RFC): 103 krav, 93 testfall
  - [RFC 2104](https://www.rfc-editor.org/rfc/rfc2104) · [RFC 2369](https://www.rfc-editor.org/rfc/rfc2369) · [RFC 2474](https://www.rfc-editor.org/rfc/rfc2474) · [RFC 3161](https://www.rfc-editor.org/rfc/rfc3161) · [RFC 3207](https://www.rfc-editor.org/rfc/rfc3207) · [RFC 3246](https://www.rfc-editor.org/rfc/rfc3246) · [RFC 3261](https://www.rfc-editor.org/rfc/rfc3261) · [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339) · [RFC 3414](https://www.rfc-editor.org/rfc/rfc3414) · [RFC 3464](https://www.rfc-editor.org/rfc/rfc3464) · [RFC 3629](https://www.rfc-editor.org/rfc/rfc3629) · [RFC 3711](https://www.rfc-editor.org/rfc/rfc3711) · [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986) · [RFC 4035](https://www.rfc-editor.org/rfc/rfc4035) · [RFC 4180](https://www.rfc-editor.org/rfc/rfc4180) · [RFC 4594](https://www.rfc-editor.org/rfc/rfc4594) · [RFC 4647](https://www.rfc-editor.org/rfc/rfc4647) · [RFC 5321](https://www.rfc-editor.org/rfc/rfc5321) · [RFC 5322](https://www.rfc-editor.org/rfc/rfc5322) · [RFC 5424](https://www.rfc-editor.org/rfc/rfc5424) · [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545) · [RFC 5546](https://www.rfc-editor.org/rfc/rfc5546) · [RFC 5853](https://www.rfc-editor.org/rfc/rfc5853) · [RFC 5880](https://www.rfc-editor.org/rfc/rfc5880) · [RFC 5905](https://www.rfc-editor.org/rfc/rfc5905) · [RFC 6047](https://www.rfc-editor.org/rfc/rfc6047) · [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238) · [RFC 6241](https://www.rfc-editor.org/rfc/rfc6241) · [RFC 6266](https://www.rfc-editor.org/rfc/rfc6266) · [RFC 6376](https://www.rfc-editor.org/rfc/rfc6376) · [RFC 6455](https://www.rfc-editor.org/rfc/rfc6455) · [RFC 6585](https://www.rfc-editor.org/rfc/rfc6585) · [RFC 6716](https://www.rfc-editor.org/rfc/rfc6716) · [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) · [RFC 6797](https://www.rfc-editor.org/rfc/rfc6797) · [RFC 7208](https://www.rfc-editor.org/rfc/rfc7208) · [RFC 7489](https://www.rfc-editor.org/rfc/rfc7489) · [RFC 7519](https://www.rfc-editor.org/rfc/rfc7519) · [RFC 7643](https://www.rfc-editor.org/rfc/rfc7643) · [RFC 7644](https://www.rfc-editor.org/rfc/rfc7644) · [RFC 7932](https://www.rfc-editor.org/rfc/rfc7932) · [RFC 7986](https://www.rfc-editor.org/rfc/rfc7986) · [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058) · [RFC 8246](https://www.rfc-editor.org/rfc/rfc8246) · [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259) · [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288) · [RFC 8301](https://www.rfc-editor.org/rfc/rfc8301) · [RFC 8460](https://www.rfc-editor.org/rfc/rfc8460) · [RFC 8461](https://www.rfc-editor.org/rfc/rfc8461) · [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594) · [RFC 8641](https://www.rfc-editor.org/rfc/rfc8641) · [RFC 8705](https://www.rfc-editor.org/rfc/rfc8705) · [RFC 8725](https://www.rfc-editor.org/rfc/rfc8725) · [RFC 9068](https://www.rfc-editor.org/rfc/rfc9068) · [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106) · [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) · [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111) · [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116) · [RFC 9413](https://www.rfc-editor.org/rfc/rfc9413) · [RFC 9449](https://www.rfc-editor.org/rfc/rfc9449) · [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) · [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700) · [RFC 9745](https://www.rfc-editor.org/rfc/rfc9745) · [draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/) · [draft-ietf-httpapi-ratelimit-headers](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/) · [draft-ietf-tls-ecdhe-mlkem](https://datatracker.ietf.org/doc/draft-ietf-tls-ecdhe-mlkem/)
- [IPC-standarder för kretskort och elektronikmontering (IPC-2221, IPC-6012, IPC-A-610, J-STD-001)](https://www.electronics.org/ipc-standards): 5 krav, 4 testfall
- [ISA-18.2 / IEC 62682 Larmhantering](https://webstore.iec.ch/en/publication/65543): 10 krav, 9 testfall
  - [IEC 62682](https://webstore.iec.ch/en/publication/65543)
- [ISA-88 / IEC 61512 Satsvis processtyrning](https://www.isa.org/standards-and-publications/isa-standards/isa-88-standards): 5 krav, 5 testfall
  - [IEC 61512-1](https://webstore.iec.ch/en/publication/75287)
- [ISA-95 / IEC 62264 Integration mellan affärs- och styrsystem](https://www.isa.org/standards-and-publications/isa-standards/isa-95-standard): 6 krav, 5 testfall
  - [IEC 62264-1](https://www.iso.org/standard/57308.html) · [IEC 62264-2](https://webstore.iec.ch/en/publication/75127) · [IEC 62264-3](https://webstore.iec.ch/en/publication/33511)
- [ISAE 3402 Bestyrkanderapporter om kontroller hos en serviceorganisation](https://www.iaasb.org/consultations-projects/assurance-reports-service-organizations-controls): 1 krav, 1 testfall
- [ISO 10002 Kundtillfredsställelse: klagomålshantering](https://www.iso.org/standard/71580.html): 3 krav, 2 testfall
- [ISO 10004 Kundtillfredsställelse: övervakning och mätning](https://www.iso.org/standard/71582.html): 3 krav, 3 testfall
- [ISO 10006 Kvalitetsledning i projekt](https://www.iso.org/standard/70376.html): 2 krav, 2 testfall
- [ISO 10007 Riktlinjer för konfigurationsledning](https://www.iso.org/standard/70400.html): 11 krav, 8 testfall
- [ISO 10015 Riktlinjer för kompetensförsörjning och personalutveckling](https://www.iso.org/standard/69459.html): 11 krav, 9 testfall
- [ISO 10075 Ergonomiska principer för mental belastning](https://www.iso.org/standard/76686.html): 1 krav, 1 testfall
- [ISO 11064 Ergonomisk utformning av kontrollcentraler](https://www.iso.org/standard/19042.html): 5 krav, 3 testfall
- [ISO 13485 Medicintekniska produkter – Ledningssystem för kvalitet](https://www.iso.org/standard/59752.html): 2 krav, 2 testfall
- [ISO 13702 Kontroll och begränsning av brand och explosion på offshoreanläggningar](https://www.iso.org/standard/57416.html): 2 krav, 2 testfall
- [ISO 13849 Säkerhetsrelaterade delar av maskiners styrsystem](https://www.iso.org/standard/73481.html): 3 krav, 3 testfall
  - [ISO 13849-1](https://www.iso.org/standard/73481.html) · [ISO 13849-2](https://www.iso.org/standard/53640.html)
- [ISO 14001 Miljöledningssystem](https://www.iso.org/standard/92300.html): 1 krav, 1 testfall
- [ISO 14064 Kvantifiering och rapportering av växthusgaser](https://www.iso.org/standard/66453.html): 1 krav, 1 testfall
- [ISO 14224 Tillförlitlighets- och underhållsdata för utrustning inom olja och gas](https://www.iso.org/standard/64076.html): 3 krav, 3 testfall
- [ISO 14289 PDF/UA tillgänglig PDF](https://www.iso.org/standard/64599.html): 3 krav, 3 testfall
- [ISO 14721 Referensmodell för öppna arkivinformationssystem (OAIS)](https://www.iso.org/standard/87471.html): 1 krav, 1 testfall
- [ISO 14971 Medicintekniska produkter – Riskhantering för medicintekniska produkter](https://www.iso.org/standard/72704.html): 2 krav, 2 testfall
- [ISO 15489 Informationshantering: dokumenthantering](https://www.iso.org/standard/62542.html): 7 krav, 6 testfall
- [ISO 17100 Översättningstjänster](https://www.iso.org/standard/59149.html): 1 krav, 1 testfall
- [ISO 17359 och ISO 13374 Tillståndsövervakning av maskiner](https://www.iso.org/standard/71194.html): 3 krav, 3 testfall
  - [ISO 17359](https://www.iso.org/standard/71194.html) · [ISO 13374-1](https://www.iso.org/standard/21832.html)
- [ISO 19011 Vägledning för revision av ledningssystem](https://www.iso.org/standard/88984.html): 4 krav, 3 testfall
- [ISO 20816 Mätning av mekaniska vibrationer](https://www.iso.org/standard/63180.html): 1 krav, 1 testfall
- [ISO 21502, 21508 och 21511 Projekt-, program- och portföljstyrning](https://www.iso.org/standard/74947.html): 30 krav, 23 testfall
  - [ISO 21502](https://www.iso.org/standard/74947.html) · [ISO 21508](https://www.iso.org/standard/63582.html) · [ISO 21511](https://www.iso.org/standard/69702.html)
- [ISO 21504 Projekt-, program- och portföljstyrning: vägledning för portföljstyrning](https://www.iso.org/standard/82867.html): 9 krav, 6 testfall
- [ISO 21542 Tillgänglighet och användbarhet i den byggda miljön](https://www.iso.org/standard/71860.html): 2 krav, 1 testfall
- [ISO 22301 Ledningssystem för kontinuitet](https://www.iso.org/standard/75106.html): 24 krav, 19 testfall
- [ISO 22400 Nyckeltal för tillverkning](https://www.iso.org/standard/54497.html): 3 krav, 3 testfall
- [ISO 22514 Statistiska metoder för processtyrning (duglighet)](https://www.iso.org/standard/88883.html): 1 krav, 1 testfall
- [ISO 24089 Vägfordon – Teknik för programuppdateringar](https://www.iso.org/standard/77796.html): 4 krav, 4 testfall
- [ISO 24495 Klarspråk](https://www.iso.org/standard/78907.html): 3 krav, 3 testfall
- [ISO 26262 Vägfordon – Funktionssäkerhet](https://www.iso.org/standard/68383.html): 5 krav, 5 testfall
- [ISO 29993 Utbildningstjänster utanför det formella utbildningssystemet](https://www.iso.org/standard/70357.html): 6 krav, 4 testfall
- [ISO 31000 Riskhantering: vägledning](https://www.iso.org/standard/65694.html): 14 krav, 10 testfall
- [ISO 32000 Portable Document Format (PDF)](https://www.iso.org/standard/75839.html): 1 krav, 1 testfall
- [ISO 3382-3 Akustik: mätning av rumsakustiska parametrar i kontorslandskap](https://www.iso.org/standard/77437.html): 1 krav, 1 testfall
- [ISO 37301 Ledningssystem för regelefterlevnad: krav med vägledning](https://www.iso.org/standard/75080.html): 4 krav, 2 testfall
- [ISO 3864 Säkerhetsfärger och säkerhetsskyltar](https://www.iso.org/standard/66836.html): 1 krav, 1 testfall
- [ISO 41001 Ledningssystem för fastighetsförvaltning (facility management)](https://www.iso.org/standard/68021.html): 2 krav, 2 testfall
- [ISO 4126 Säkerhetsanordningar för skydd mot för högt tryck](https://www.iso.org/standard/50826.html): 1 krav, 1 testfall
- [ISO 4217 Valutakoder](https://www.iso.org/standard/64758.html): 4 krav, 4 testfall
- [ISO 45001 Ledningssystem för arbetsmiljö](https://www.iso.org/standard/63787.html): 2 krav, 2 testfall
- [ISO 50001 Ledningssystem för energi](https://www.iso.org/standard/69426.html): 2 krav, 2 testfall
- [ISO 55001 Ledningssystem för tillgångsförvaltning](https://www.iso.org/standard/83054.html): 11 krav, 10 testfall
- [ISO 639 Språkkoder](https://www.iso.org/standard/74575.html): 1 krav, 1 testfall
- [ISO 7870 Styrdiagram](https://www.iso.org/standard/78859.html): 1 krav, 1 testfall
- [ISO 8000 Datakvalitet](https://www.iso.org/standard/60805.html): 4 krav, 4 testfall
- [ISO 8601 Representation av datum och tid](https://www.iso.org/standard/70907.html): 7 krav, 7 testfall
- [ISO 9001 Ledningssystem för kvalitet](https://www.iso.org/standard/88464.html): 96 krav, 77 testfall
- [ISO 9241 Ergonomi för människa–system-interaktion](https://www.iso.org/standard/63500.html): 114 krav, 100 testfall
  - [ISO 9241-11](https://www.iso.org/standard/63500.html) · [ISO 9241-110](https://www.iso.org/standard/75258.html) · [ISO 9241-143](https://www.iso.org/standard/53590.html) · [ISO 9241-303](https://www.iso.org/standard/57992.html)
- [ISO/IEC 15415 och 15416 Utskriftskvalitet för streckkoder](https://www.iso.org/standard/76876.html): 1 krav, 1 testfall
  - [ISO/IEC 15415](https://www.iso.org/standard/76876.html) · [ISO/IEC 15416](https://www.iso.org/standard/83390.html)
- [ISO/IEC 17025 Kompetens hos provnings- och kalibreringslaboratorier](https://www.iso.org/standard/66912.html): 6 krav, 6 testfall
- [ISO/IEC 19941 Interoperabilitet och portabilitet för molntjänster](https://www.iso.org/standard/66639.html): 2 krav, 2 testfall
- [ISO/IEC 20000-1 Ledningssystem för tjänster](https://www.iso.org/standard/70636.html): 32 krav, 30 testfall
- [ISO/IEC 20889 Tekniker för avidentifiering av data](https://www.iso.org/standard/69373.html): 1 krav, 1 testfall
- [ISO/IEC 21031 Specifikation för programvarans koldioxidintensitet (SCI)](https://www.iso.org/standard/86612.html): 3 krav, 3 testfall
- [ISO/IEC 25000-serien, kvalitetsmätning och rapportering (25022, 25023, 25062)](https://www.iso.org/standard/35746.html): 8 krav, 8 testfall
  - [ISO/IEC 25022](https://www.iso.org/standard/35746.html) · [ISO/IEC 25023](https://www.iso.org/standard/35747.html) · [ISO/IEC 25062](https://www.iso.org/standard/43046.html)
- [ISO/IEC 25010 Kvalitetsmodell för produkter](https://www.iso.org/standard/78176.html): 179 krav, 173 testfall
- [ISO/IEC 25012 Kvalitetsmodell för data](https://www.iso.org/standard/35736.html): 16 krav, 15 testfall
- [ISO/IEC 27001 Ledningssystem för informationssäkerhet](https://www.iso.org/standard/27001): 550 krav, 499 testfall
- [ISO/IEC 27002 Säkerhetsåtgärder för informationssäkerhet](https://www.iso.org/standard/75652.html): 4 krav, 4 testfall
- [ISO/IEC 27005 Riskhantering för informationssäkerhet](https://www.iso.org/standard/80585.html): 3 krav, 2 testfall
- [ISO/IEC 27017 Säkerhetsåtgärder för molntjänster](https://www.iso.org/standard/82878.html): 8 krav, 8 testfall
- [ISO/IEC 27018 Skydd av personuppgifter i publika moln](https://www.iso.org/standard/88150.html): 5 krav, 6 testfall
- [ISO/IEC 27019 Informationssäkerhet för energibranschen](https://www.iso.org/standard/85056.html): 7 krav, 7 testfall
- [ISO/IEC 27031 IT-beredskap för kontinuitet](https://www.iso.org/standard/80975.html): 6 krav, 5 testfall
- [ISO/IEC 27033 Nätverkssäkerhet](https://www.iso.org/standard/63461.html): 15 krav, 14 testfall
  - [ISO/IEC 27033-1](https://www.iso.org/standard/63461.html) · [ISO/IEC 27033-2](https://www.iso.org/standard/51581.html) · [ISO/IEC 27033-4](https://www.iso.org/standard/51583.html) · [ISO/IEC 27033-5](https://www.iso.org/standard/51584.html) · [ISO/IEC 27033-6](https://www.iso.org/standard/51585.html)
- [ISO/IEC 27034 Applikationssäkerhet](https://www.iso.org/standard/44378.html): 1 krav, 1 testfall
- [ISO/IEC 27035 Hantering av informationssäkerhetsincidenter](https://www.iso.org/standard/78973.html): 10 krav, 10 testfall
  - [ISO/IEC 27035-1](https://www.iso.org/standard/78973.html) · [ISO/IEC 27035-2](https://www.iso.org/standard/78974.html)
- [ISO/IEC 27036 Informationssäkerhet i leverantörsrelationer](https://www.iso.org/standard/82060.html): 11 krav, 10 testfall
  - [ISO/IEC 27036-2](https://www.iso.org/standard/82060.html) · [ISO/IEC 27036-3](https://www.iso.org/standard/82890.html)
- [ISO/IEC 27037 Hantering av digitala bevis](https://www.iso.org/standard/44381.html): 1 krav, 1 testfall
- [ISO/IEC 27040 Lagringssäkerhet](https://www.iso.org/standard/80194.html): 4 krav, 4 testfall
- [ISO/IEC 27701 Ledningssystem för integritetsskydd](https://www.iso.org/standard/85819.html): 23 krav, 23 testfall
- [ISO/IEC 29147 Offentliggörande av sårbarheter](https://www.iso.org/standard/72311.html): 18 krav, 16 testfall
- [ISO/IEC 30111 Processer för sårbarhetshantering](https://www.iso.org/standard/69725.html): 15 krav, 14 testfall
- [ISO/IEC 42001 Ledningssystem för AI](https://www.iso.org/standard/81230.html): 42 krav, 35 testfall
- [ISO/IEC-standarder för AI (22989, 23894, 25059, 5259, 8183, TR 24027, TS 4213)](https://www.iso.org/standard/74296.html): 14 krav, 11 testfall
  - [ISO/IEC 22989](https://www.iso.org/standard/74296.html) · [ISO/IEC 23894](https://www.iso.org/standard/77304.html) · [ISO/IEC 25059](https://www.iso.org/standard/80655.html) · [ISO/IEC 5259-2](https://www.iso.org/standard/81860.html) · [ISO/IEC 8183](https://www.iso.org/standard/83002.html) · [ISO/IEC TR 24027](https://www.iso.org/standard/77607.html) · [ISO/IEC TS 4213](https://www.iso.org/standard/79799.html)
- [ISO/IEC/IEEE 12207 Livscykelprocesser för programvara](https://www.iso.org/standard/63712.html): 1 krav, 1 testfall
- [ISO/IEC/IEEE 15288 Livscykelprocesser för system](https://www.iso.org/standard/81702.html): 39 krav, 30 testfall
- [ISO/IEC/IEEE 15289 Innehåll i informationsobjekt för livscykeln](https://www.iso.org/standard/74909.html): 11 krav, 8 testfall
- [ISO/IEC/IEEE 26514 Utformning av användarinformation](https://www.iso.org/standard/77451.html): 13 krav, 10 testfall
- [ISO/IEC/IEEE 29119 Programvarutestning](https://www.iso.org/standard/79429.html): 11 krav, 9 testfall
  - [ISO/IEC/IEEE 29119-3](https://www.iso.org/standard/79429.html)
- [ISO/IEC/IEEE 29148 Kravhantering](https://www.iso.org/standard/72089.html): 10 krav, 7 testfall
- [ISO/IEC/IEEE 42010 Arkitekturbeskrivning](https://www.iso.org/standard/74393.html): 5 krav, 5 testfall
- [ISO/SAE 21434 Vägfordon – Cybersäkerhetsteknik](https://www.iso.org/standard/70918.html): 3 krav, 3 testfall
- [ITU-R-rekommendationer (radioutbredning)](https://www.itu.int/pub/R-REC): 1 krav, 1 testfall
  - [ITU-R P.530](https://www.itu.int/rec/R-REC-P.530) · [ITU-R P.837](https://www.itu.int/rec/R-REC-P.837)
- [ITU-T-rekommendationer](https://www.itu.int/en/ITU-T/publications/Pages/recs.aspx): 11 krav, 10 testfall
  - [ITU-T E.800](https://www.itu.int/rec/T-REC-E.800) · [ITU-T G.107](https://www.itu.int/rec/T-REC-G.107) · [ITU-T G.114](https://www.itu.int/rec/T-REC-G.114) · [ITU-T G.722](https://www.itu.int/rec/T-REC-G.722) · [ITU-T G.8271](https://www.itu.int/rec/T-REC-G.8271) · [ITU-T G.8275.1](https://www.itu.int/rec/T-REC-G.8275.1) · [ITU-T M.3400](https://www.itu.int/rec/T-REC-M.3400) · [ITU-T X.733](https://www.itu.int/rec/T-REC-X.733) · [ITU-T Y.1541](https://www.itu.int/rec/T-REC-Y.1541)
- [LoRa Alliance LoRaWAN-specifikationer (länklager, regionala parametrar, applikationslager)](https://resources.lora-alliance.org/technical-specifications): 6 krav, 4 testfall
- [Miljöprovning och skyddsklasser (IEC 60068, 60721-3, 60529, 62262)](https://webstore.iec.ch/en/publication/501): 11 krav, 9 testfall
  - [IEC 60068-1](https://webstore.iec.ch/en/publication/501) · [IEC 60529](https://webstore.iec.ch/en/publication/2452)
- [MITRE Common Weakness Enumeration (CWE)](https://cwe.mitre.org/): 27 krav, 25 testfall
  - [CWE-22](https://cwe.mitre.org/data/definitions/22.html) · [CWE-79](https://cwe.mitre.org/data/definitions/79.html) · [CWE-93](https://cwe.mitre.org/data/definitions/93.html) · [CWE-113](https://cwe.mitre.org/data/definitions/113.html) · [CWE-214](https://cwe.mitre.org/data/definitions/214.html) · [CWE-400](https://cwe.mitre.org/data/definitions/400.html) · [CWE-409](https://cwe.mitre.org/data/definitions/409.html) · [CWE-427](https://cwe.mitre.org/data/definitions/427.html) · [CWE-434](https://cwe.mitre.org/data/definitions/434.html) · [CWE-524](https://cwe.mitre.org/data/definitions/524.html) · [CWE-526](https://cwe.mitre.org/data/definitions/526.html) · [CWE-532](https://cwe.mitre.org/data/definitions/532.html) · [CWE-598](https://cwe.mitre.org/data/definitions/598.html) · [CWE-601](https://cwe.mitre.org/data/definitions/601.html) · [CWE-749](https://cwe.mitre.org/data/definitions/749.html) · [CWE-798](https://cwe.mitre.org/data/definitions/798.html) · [CWE-799](https://cwe.mitre.org/data/definitions/799.html) · [CWE-926](https://cwe.mitre.org/data/definitions/926.html) · [CWE-1236](https://cwe.mitre.org/data/definitions/1236.html) · [CWE-1333](https://cwe.mitre.org/data/definitions/1333.html) · [CWE-1336](https://cwe.mitre.org/data/definitions/1336.html)
- [OASIS Common Security Advisory Framework (CSAF)](https://docs.oasis-open.org/csaf/csaf/v2.0/csaf-v2.0.html): 2 krav, 2 testfall
- [OASIS MQTT](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html): 2 krav, 2 testfall
- [Open Container Initiatives specifikationer](https://opencontainers.org/) (OCI): 1 krav, 1 testfall
  - [Open Container Initiative Image Format Specification](https://github.com/opencontainers/image-spec)
- [OpenAPI Specification](https://spec.openapis.org/oas/v3.1.0): 5 krav, 4 testfall
- [OpenChain ISO/IEC 5230 Efterlevnad av licenser för öppen källkod](https://www.iso.org/standard/81039.html): 8 krav, 7 testfall
- [OpenFeature-specifikationen](https://openfeature.dev/specification/): 5 krav, 5 testfall
- [OpenID Connect](https://openid.net/specs/openid-connect-core-1_0.html) (OIDC): 2 krav, 2 testfall
- [OpenTelemetry-specifikationen](https://opentelemetry.io/docs/specs/otel/): 5 krav, 5 testfall
- [OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/) (OWASP ASVS): 129 krav, 123 testfall
- [OWASP Mobile Application Security Verification Standard (MASVS)](https://mas.owasp.org/MASVS/) (OWASP MASVS): 18 krav, 18 testfall
- [PCI Data Security Standard och vägledning från PCI SSC](https://www.pcisecuritystandards.org/) (PCI DSS): 27 krav, 26 testfall
- [Peppol BIS Billing](https://docs.peppol.eu/poacc/billing/3.0/): 1 krav, 1 testfall
- [Riktlinjer för tillgängligt webbinnehåll (WCAG) 2.2](https://www.w3.org/TR/WCAG22/) (WCAG 2.2): 71 krav, 57 testfall
- [SAE EIA-649 Standard för konfigurationsledning](https://www.sae.org/standards/content/eia649c/): 4 krav, 3 testfall
- [SCORM, paketering av e-lärandeinnehåll](https://www.adlnet.gov/research/scorm): 1 krav, 1 testfall
- [Semantisk versionshantering](https://semver.org/spec/v2.0.0.html) (SemVer): 7 krav, 7 testfall
- [SPDX-specifikationen och licenslistan](https://spdx.dev/): 6 krav, 7 testfall
  - [SPDX Specification 2.3](https://spdx.github.io/spdx-spec/v2.3/) · [SPDX License List](https://spdx.org/licenses/)
- [Specifikationer för webbapplikationer offline (W3C Service Workers, W3C Indexed Database API, WHATWG Storage)](https://www.w3.org/TR/service-workers/): 2 krav, 2 testfall
- [Standard Webhooks-specifikationen](https://www.standardwebhooks.com/): 3 krav, 3 testfall
- [Standarder för livscykel för system och programvara (ISO/IEC/IEEE 15939, 24748-1, IEEE 1012)](https://www.iso.org/standard/71197.html): 10 krav, 10 testfall
  - [ISO/IEC/IEEE 15939](https://www.iso.org/standard/71197.html) · [ISO/IEC/IEEE 24748-1](https://www.iso.org/standard/84709.html) · [IEEE 1012](https://standards.ieee.org/ieee/1012/5609/)
- [Standarder för maskinsäkerhet (ISO 12100, 13850, 13855, 13857, 14118, 14119, 14120)](https://www.iso.org/standard/51528.html): 8 krav, 7 testfall
  - [ISO 12100](https://www.iso.org/standard/51528.html) · [ISO 13850](https://www.iso.org/standard/59970.html) · [ISO 13855](https://www.iso.org/standard/80590.html) · [ISO 13857](https://www.iso.org/standard/69569.html) · [ISO 14118](https://www.iso.org/standard/66460.html) · [ISO 14119](https://www.iso.org/standard/75942.html) · [ISO 14120](https://www.iso.org/standard/59545.html)
- [Standarder för materialdeklaration och märkning (EN IEC 63000, IEC 62474, EN 50419)](https://webstore.iec.ch/en/publication/29857): 2 krav, 2 testfall
  - [IEC 62474](https://webstore.iec.ch/en/publication/29857)
- [Standarder för operatörsgränssnitt (IEC 60073, 60447, 61310-1, EN 894-3, ISO 7010)](https://webstore.iec.ch/en/publication/5263): 4 krav, 3 testfall
  - [IEC 61310-1](https://webstore.iec.ch/en/publication/5263) · [ISO 7010](https://www.iso.org/standard/72424.html)
- [Standarder och vägledningar för mätteknik (ISO 10012, JCGM 100 GUM, ILAC-G24)](https://www.iso.org/standard/26033.html): 5 krav, 5 testfall
  - [ISO 10012](https://www.iso.org/standard/26033.html) · [JCGM 100](https://www.bipm.org/en/doi/10.59161/JCGM100-2008E) · [ILAC-G24](https://ilac.org/publications-and-resources/ilac-guidance-series/)
- [Teknisk produktdokumentation: namnrutor och ritningsprinciper (ISO 7200, ISO 128)](https://www.iso.org/standard/35446.html): 2 krav, 1 testfall
  - [ISO 7200](https://www.iso.org/standard/35446.html) · [ISO 128-1](https://www.iso.org/standard/65296.html)
- [TM Forum Open API:er](https://www.tmforum.org/oda/open-apis): 4 krav, 4 testfall
- [Unicode-standarden, CLDR, ICU och ISO/IEC 10646](https://www.unicode.org/standard/standard.html): 20 krav, 19 testfall
  - [Unicode CLDR](https://cldr.unicode.org/) · [Unicode UAX #9](https://www.unicode.org/reports/tr9/) · [Unicode UAX #15](https://www.unicode.org/reports/tr15/) · [Unicode UTS #10](https://www.unicode.org/reports/tr10/) · [Unicode UTS #35](https://www.unicode.org/reports/tr35/) · [ICU MessageFormat](https://unicode-org.github.io/icu/userguide/format_parse/messages/) · [ISO/IEC 10646](https://www.iso.org/standard/76835.html)
- [Världspostföreningens standarder (S42 adressering)](https://www.upu.int/en/postal-solutions/programmes-services/addressing-solutions) (UPU): 1 krav, 1 testfall
- [W3C Clear Site Data (rensning av webbplatsdata)](https://www.w3.org/TR/clear-site-data/): 1 krav, 1 testfall
- [W3C:s webbstandarder (internationalisering, Trace Context, WebAuthn, CSP, Media Queries)](https://www.w3.org/TR/): 24 krav, 21 testfall
  - [W3C Character Model for the World Wide Web](https://www.w3.org/TR/charmod/) · [W3C Character Model: String Matching](https://www.w3.org/TR/charmod-norm/) · [W3C Content Security Policy Level 3](https://www.w3.org/TR/CSP3/) · [W3C Internationalization Best Practices](https://www.w3.org/International/techniques/authoring-html) · [W3C Internationalization: language negotiation](https://www.w3.org/International/questions/qa-accept-lang-locales) · [W3C Internationalization: text size in translation](https://www.w3.org/International/articles/article-text-size) · [W3C Personal names around the world](https://www.w3.org/International/questions/qa-personal-names) · [W3C Structural markup and right-to-left text in HTML](https://www.w3.org/International/questions/qa-html-dir) · [W3C Trace Context](https://www.w3.org/TR/trace-context/) · [W3C Web Authentication Level 2](https://www.w3.org/TR/webauthn-2/) · [Media Queries Level 5](https://www.w3.org/TR/mediaqueries-5/)
- [WHATWG Living Standards (HTML, Fetch)](https://spec.whatwg.org/): 5 krav, 5 testfall
  - [HTML Living Standard](https://html.spec.whatwg.org/multipage/) · [Fetch Living Standard](https://fetch.spec.whatwg.org/)

#### Standarder och specifikationer: Europeiska unionen

- [EN 12464-1 Ljus och belysning: belysning av arbetsplatser inomhus](https://www.sis.se/en/produkter/construction-materials-and-building/lighting/interior-lighting/ss-en-12464-12021-f17a3ebb/): 1 krav, 1 testfall
- [EN 12845 Fasta släcksystem: automatiska sprinklersystem](https://www.sis.se/en/produkter/environment-health-protection-safety/protection-against-fire/firefighting/ss-en-128452015a22026/): 1 krav, 1 testfall
- [EN 13306 Underhållsterminologi](https://www.sis.se/en/produkter/standardization/vocabularies/services/ss-en-133062017/): 11 krav, 8 testfall
- [EN 14181 och EN 17255 Automatisk utsläppsmätning](https://www.sis.se/en/produkter/environment-health-protection-safety/air-quality/ssen141812014/): 1 krav, 1 testfall
  - [EN 14181](https://www.sis.se/en/produkter/environment-health-protection-safety/air-quality/ssen141812014/)
- [EN 16798-1 Byggnaders energiprestanda: indata för inomhusmiljön](https://www.sis.se/en/produkter/construction-materials-and-building/installations-in-buildings/general/ss-en-16798-12019/): 3 krav, 2 testfall
- [EN 16931 Elektronisk fakturering](https://ec.europa.eu/digital-building-blocks/sites/display/DIGITAL/Obtaining+a+copy+of+the+European+standard+on+eInvoicing): 1 krav, 1 testfall
- [EN 1717 Skydd mot förorening av dricksvatten i dricksvatteninstallationer (återströmning)](https://www.sis.se/en/produkter/fluid-systems-and-components-for-general-use/valves/general/ss-en-17172025/): 1 krav, 1 testfall
- [EN 3 Handbrandsläckare](https://www.sis.se/en/produkter/environment-health-protection-safety/protection-against-fire/firefighting/ssen372004a12007/): 1 krav, 1 testfall
- [EN 301 549 Tillgänglighetskrav för IKT-produkter och tjänster](https://www.etsi.org/deliver/etsi_en/301500_301599/301549/): 23 krav, 19 testfall
- [EN 50110-1 Skötsel av elektriska anläggningar](https://elstandard.se/standard/3246401): 3 krav, 3 testfall
- [EN 50126 och EN 50129 RAMS för järnväg och säkerhetsrelaterade elektroniska system](https://www.sis.se/en/produkter/electrical-engineering/electric-traction-equipment/ss-en-50126-1/): 12 krav, 8 testfall
- [EN 50518 Larmcentraler för övervakning och larmmottagning](https://www.sis.se/en/produkter/environment-health-protection-safety/alarm-and-warning-systems/ss-en-50518/): 2 krav, 2 testfall
- [EN 50600 Datacenteranläggningar och infrastruktur](https://www.sis.se/en/produkter/information-technology-office-machines/general/ssen506001/): 16 krav, 13 testfall
  - [EN 50600-1](https://www.sis.se/en/produkter/information-technology-office-machines/general/ssen506001/) · [EN 50600-3-1](https://www.sis.se/en/produkter/information-technology-office-machines/general/ssen5060031/)
- [ETSI EN 300 220 Kortdistansutrustning i frekvensområdet 25 MHz till 1 000 MHz](https://www.etsi.org/deliver/etsi_en/300200_300299/30022002/): 1 krav, 1 testfall
- [ETSI EN 303 645 Cybersäkerhet för konsumentprodukter för sakernas internet](https://www.etsi.org/deliver/etsi_en/303600_303699/303645/): 7 krav, 7 testfall
- [ETSI harmoniserade radiostandarder (EN 300 328, EN 300 396, EN 301 893)](https://www.etsi.org/deliver/etsi_en/300300_300399/300328/): 3 krav, 2 testfall
  - [ETSI EN 300 328](https://www.etsi.org/deliver/etsi_en/300300_300399/300328/) · [ETSI EN 301 893](https://www.etsi.org/deliver/etsi_en/301800_301899/301893/)
- [Europeiska standarder för brand och nödbelysning (EN 54, EN 1155, EN 1838, EN 15004-1, EN 50172)](https://www.sis.se/en/produkter/environment-health-protection-safety/protection-against-fire/fire-protection/ss-en-54-120212/): 8 krav, 7 testfall
- [Europeiska standarder för skydd av byggnader (EN 1627, EN 50131, EN 60839-11-1, EN 62676-4)](https://www.sis.se/en/produkter/environment-health-protection-safety/protection-against-crime/ss-en-50131-1-6a7cf25e/): 6 krav, 6 testfall
  - [EN 50131-1](https://www.sis.se/en/produkter/environment-health-protection-safety/protection-against-crime/ss-en-50131-1-6a7cf25e/)
- [Europeiska tillgänglighetsstandarder för byggd miljö (EN 17210, EN 81-70)](https://www.sis.se/en/produkter/construction-materials-and-building/elements-of-buildings/elements-of-buildings-in-general/ss-en-172102021/): 3 krav, 2 testfall
  - [EN 17210](https://www.sis.se/en/produkter/construction-materials-and-building/elements-of-buildings/elements-of-buildings-in-general/ss-en-172102021/)
- [Europeiska underhållsstandarder (EN 13269, EN 13460, EN 15341, EN 17007)](https://www.sis.se/en/produkter/image-technology/graphic-technology/reproduction-equipment/ss-en-153412019a12022/): 15 krav, 13 testfall
  - [EN 15341](https://www.sis.se/en/produkter/image-technology/graphic-technology/reproduction-equipment/ss-en-153412019a12022/) · [EN 13460](https://www.sis.se/en/produkter/standardization/technical-product-documentation/ss-en-134602009/)
- [Säkerhetsregler för hissar och rulltrappor (EN 81-20, EN 81-28, EN 115-1)](https://www.sis.se/en/produkter/maskinsakerhet-357f77b1/passenger-goods-and-service-lifts/ss-en-81-202020/): 2 krav, 1 testfall
  - [EN 81-20](https://www.sis.se/en/produkter/maskinsakerhet-357f77b1/passenger-goods-and-service-lifts/ss-en-81-202020/) · [EN 115-1](https://www.sis.se/en/produkter/maskinsakerhet-357f77b1/passenger-goods-and-service-lifts/ss-en-115-12017/)

#### Standarder och specifikationer: Norge

- [NORSOK-standarder (norsk petroleumindustri)](https://standard.no/en/sectors/petroleum/): 2 krav, 2 testfall

#### Standarder och specifikationer: Sverige

- [SS 436 40 00 Elinstallationer för lågspänning: utförande av elinstallationer](https://www.sis.se/en/produkter/construction-materials-and-building/installations-in-buildings/electricity-supply-systems/ss-436-40-00-utg-42023r12024/): 4 krav, 3 testfall

#### Standarder och specifikationer: USA

- [American Petroleum Institutes standarder och rekommenderade praxis](https://www.api.org/products-and-services/standards) (API): 7 krav, 7 testfall
- [NERC:s standarder för skydd av kritisk infrastruktur (CIP)](https://www.nerc.com/standards/reliability-standards) (NERC CIP): 2 krav, 2 testfall
- [NIST SP 800-53 Säkerhets- och integritetsåtgärder](https://doi.org/10.6028/NIST.SP.800-53r5): 59 krav, 57 testfall
- [NIST:s kryptografiska standarder och riktlinjer (FIPS 180-4, SP 800-52, 800-57, 800-90A, 800-107, 800-131A)](https://csrc.nist.gov/projects/cryptographic-standards-and-guidelines): 15 krav, 16 testfall
  - [NIST FIPS 180-4](https://doi.org/10.6028/NIST.FIPS.180-4) · [NIST SP 800-52 Rev. 2](https://doi.org/10.6028/NIST.SP.800-52r2) · [NIST SP 800-57 Part 1 Rev. 5](https://doi.org/10.6028/NIST.SP.800-57pt1r5) · [NIST SP 800-90A Rev. 1](https://doi.org/10.6028/NIST.SP.800-90Ar1) · [NIST SP 800-107 Rev. 1](https://doi.org/10.6028/NIST.SP.800-107r1) · [NIST SP 800-131A Rev. 2](https://doi.org/10.6028/NIST.SP.800-131Ar2)

### Bästa praxis och ramverk

#### Bästa praxis och ramverk: Internationella

- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/): 4 krav, 4 testfall
  - [AWS Well-Architected Framework, Sustainability Pillar](https://docs.aws.amazon.com/wellarchitected/latest/sustainability-pillar/sustainability-pillar.html) · [AWS Well-Architected SaaS Lens](https://docs.aws.amazon.com/wellarchitected/latest/saas-lens/saas-lens.html)
- [CIS-riktmärken för containrar (CIS Kubernetes Benchmark, CIS Docker Benchmark)](https://www.cisecurity.org/benchmark/kubernetes): 11 krav, 11 testfall
- [Continuous Delivery (Humble och Farley)](https://continuousdelivery.com/): 3 krav, 3 testfall
- [CSA Cloud Controls Matrix](https://cloudsecurityalliance.org/research/cloud-controls-matrix) (CSA CCM): 9 krav, 8 testfall
- [CVE-programmet](https://www.cve.org/): 1 krav, 1 testfall
- [DORA:s forskning State of DevOps](https://dora.dev/research/): 5 krav, 4 testfall
- [Feature Toggles (Martin Fowlers webbplats)](https://martinfowler.com/articles/feature-toggles.html): 7 krav, 6 testfall
- [Goal Structuring Notation (GSN) Community Standard, Safety-Critical Systems Club](https://scsc.uk/gsn): 1 krav, 1 testfall
- [Googles böcker om Site Reliability Engineering](https://sre.google/books/) (Google SRE): 51 krav, 49 testfall
- [ISTQB:s kursplaner](https://www.istqb.org/): 20 krav, 21 testfall
  - [ISTQB CTFL 4.0](https://www.istqb.org/certifications/certified-tester-foundation-level-ctfl-v4-0/)
- [ITIL 4, praxis för tjänstehantering](https://www.axelos.com/certifications/itil-service-management): 96 krav, 83 testfall
- [Keep a Changelog](https://keepachangelog.com/en/1.1.0/): 4 krav, 4 testfall
- [Kubernetes säkerhetsdokumentation (Pod Security Standards, god praxis för RBAC, granskning, kryptering vid lagring)](https://kubernetes.io/docs/concepts/security/): 7 krav, 7 testfall
- [M3AAWG:s bästa praxis för e-post](https://www.m3aawg.org/published-documents): 8 krav, 8 testfall
- [MITRE ATLAS, hotlandskap för AI-system](https://atlas.mitre.org/): 1 krav, 1 testfall
- [MITRE ATT&CK](https://attack.mitre.org/): 4 krav, 4 testfall
  - [MITRE ATT&CK T1078](https://attack.mitre.org/techniques/T1078/)
- Modeller för utformning och utvärdering av utbildning (ADDIE, Kirkpatrick): 9 krav, 5 testfall
  - [Kirkpatrick four-level training evaluation model](https://www.kirkpatrickpartners.com/the-kirkpatrick-model/)
- [Nielsens användbarhetsheuristiker](https://www.nngroup.com/articles/ten-usability-heuristics/): 28 krav, 25 testfall
- [Oil and Gas Methane Partnership 2.0, rapporteringsramverk](https://ogmpartnership.org/) (OGMP 2.0): 1 krav, 1 testfall
- [OpenSSF Scorecard](https://scorecard.dev/): 3 krav, 3 testfall
- [OWASP API Security Top 10](https://owasp.org/API-Security/editions/2023/en/0x11-t10/) (OWASP API Top 10): 13 krav, 13 testfall
- [OWASP Mobile Application Security Testing Guide (MASTG, testguide för mobilappar)](https://mas.owasp.org/MASTG/) (OWASP MASTG): 0 krav, 1 testfall
- [OWASP Software Assurance Maturity Model (SAMM)](https://owaspsamm.org/) (OWASP SAMM): 3 krav, 3 testfall
- [OWASP Top 10 CI/CD Security Risks (säkerhetsrisker i CI/CD)](https://owasp.org/www-project-top-10-ci-cd-security-risks/): 4 krav, 4 testfall
- [OWASP Top 10 för LLM-applikationer](https://genai.owasp.org/llm-top-10/) (OWASP LLM Top 10): 14 krav, 12 testfall
- [OWASP Top 10, de största säkerhetsriskerna för webbapplikationer](https://owasp.org/Top10/): 36 krav, 37 testfall
- [PMBOK Guide (Project Management Institute)](https://www.pmi.org/standards/pmbok): 6 krav, 6 testfall
- [PRINCE2, projektstyrningsmetod](https://www.axelos.com/certifications/propath/prince2-project-management): 14 krav, 12 testfall
- RACI-matris för ansvarsfördelning: 1 krav, 1 testfall
- [REUSE-specifikationen (licens- och upphovsrättsinformation)](https://reuse.software/spec-3.2/): 1 krav, 1 testfall
- [Scaled Agile Framework (SAFe), ramverk för skalad agilitet](https://framework.scaledagile.com/): 2 krav, 1 testfall
- [Scrumguiden (Schwaber och Sutherland)](https://scrumguides.org/scrum-guide.html) (Scrum Guide): 6 krav, 4 testfall
- [SLSA, nivåer för säkerhet i programvarans leveranskedja](https://slsa.dev/spec/v1.0/): 7 krav, 7 testfall
- System Usability Scale (SUS): 1 krav, 1 testfall
- [The Twelve-Factor App](https://12factor.net/): 20 krav, 20 testfall
- [Villkor i licenser för öppen källkod och innehåll](https://opensource.org/licenses): 6 krav, 7 testfall
  - [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) · [BSD 3-Clause License](https://opensource.org/license/bsd-3-clause) · [MIT License](https://opensource.org/license/mit) · [GNU AGPL v3](https://www.gnu.org/licenses/agpl-3.0.html) · [GNU GPL v3](https://www.gnu.org/licenses/gpl-3.0.html) · [GNU LGPL v2.1](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html) · [Mozilla Public License 2.0](https://www.mozilla.org/en-US/MPL/2.0/) · [Server Side Public License v1](https://www.mongodb.com/legal/licensing/server-side-public-license) · [Creative Commons Attribution-ShareAlike 4.0](https://creativecommons.org/licenses/by-sa/4.0/) · [Creative Commons licences 4.0](https://creativecommons.org/licenses/) · [SIL Open Font License 1.1](https://openfontlicense.org/)
- [Vägledningar från Green Software Foundation](https://greensoftware.foundation/) (GSF): 6 krav, 6 testfall
  - [Green Software Foundation: Carbon Aware SDK](https://github.com/Green-Software-Foundation/carbon-aware-sdk) · [Green Software Foundation: SCI guidance](https://sci-guide.greensoftware.foundation/)

#### Bästa praxis och ramverk: USA

- [FEMA Incident Command System](https://training.fema.gov/emiweb/is/icsresource/) (ICS): 1 krav, 1 testfall
- [NIST SP 800-218 Ramverk för säker programvaruutveckling (SSDF)](https://doi.org/10.6028/NIST.SP.800-218) (NIST SSDF): 24 krav, 21 testfall
- [NIST:s ramverk för AI-riskhantering och profilen för generativ AI](https://www.nist.gov/itl/ai-risk-management-framework) (NIST AI RMF): 26 krav, 22 testfall
  - [NIST AI RMF 1.0](https://doi.org/10.6028/NIST.AI.100-1) · [NIST AI 600-1](https://doi.org/10.6028/NIST.AI.600-1) · [NIST AI 100-2](https://doi.org/10.6028/NIST.AI.100-2e2025)

### Riktlinjer och vägledningar

#### Riktlinjer och vägledningar: Internationella

- [ASHRAE TC 9.9 Termiska riktlinjer för datormiljöer](https://tc0909.ashraetcs.org/): 2 krav, 2 testfall
- [CERT/CC:s vägledning om samordnat offentliggörande av sårbarheter](https://certcc.github.io/CERT-Guide-to-CVD/): 3 krav, 3 testfall
- [DNV-RP-G108 Cybersäkerhet inom olje- och gasindustrin](https://www.dnv.com/cybersecurity/recommended-practices/dnv-rp-g108-cyber-security-in-the-oil-and-gas-industry-based-on-IEC-62443.html): 1 krav, 1 testfall
- [EEMUA 191 Vägledning för larmsystem](https://www.eemua.org/products/publications/digital/eemua-publication-191): 2 krav, 2 testfall
- [FN:s rekommendationer om transport av farligt gods, handbok för provning och kriterier](https://unece.org/transport/dangerous-goods/un-manual-tests-and-criteria): 1 krav, 1 testfall
- [GSMA:s specifikationer och riktlinjer (FS-, NG- och SGP-serierna, IoT Security Guidelines)](https://www.gsma.com/solutions-and-impact/technologies/security/cybersecurity-knowledge-base/interworking-security/): 5 krav, 4 testfall
  - [GSMA SGP.32](https://www.gsma.com/solutions-and-impact/technologies/esim/gsma_resources/sgp-32-v1-3) · [GSMA IoT Security Guidelines](https://www.gsma.com/solutions-and-impact/technologies/internet-of-things/introduction-gsma-iot-security-guidelines-assessment/)
- [IOGP-rapporter](https://idms.iogp.org/): 1 krav, 1 testfall
- [OWASP Cheat Sheet Series och vägledningar](https://cheatsheetseries.owasp.org/) (OWASP Cheat Sheets): 71 krav, 67 testfall
- [OWASP Web Security Testing Guide](https://owasp.org/www-project-web-security-testing-guide/) (OWASP WSTG): 13 krav, 31 testfall
- [PIC/S vägledning om datahantering och dataintegritet](https://picscheme.org/en/publications): 2 krav, 2 testfall
- Plattformarnas designriktlinjer (Material Design, Apples Human Interface Guidelines): 11 krav, 11 testfall
  - [Material Design](https://m3.material.io/) · [Apple Human Interface Guidelines](https://developer.apple.com/design/human-interface-guidelines/)
- [Vägledning om webbprestanda (web.dev, HTTP Archive, Google Web Fundamentals)](https://web.dev/): 7 krav, 4 testfall
  - [web.dev Core Web Vitals](https://web.dev/articles/vitals) · [web.dev Performance budgets](https://web.dev/articles/performance-budgets-101) · [HTTP Archive Web Almanac 2024, Page Weight chapter](https://almanac.httparchive.org/en/2024/page-weight)
- [W3C:s riktlinjer för hållbar webb (WSG)](https://www.w3.org/TR/web-sustainability-guidelines/): 6 krav, 5 testfall

#### Riktlinjer och vägledningar: Europeiska unionen

- [Blå guiden om genomförandet av EU:s produktregler (2022/C 247/01)](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52022XC0629%2804%29) (Blue Guide): 2 krav, 2 testfall
- [CEN/TR 16355 Rekommendationer för att förhindra tillväxt av legionella i dricksvatteninstallationer i byggnader](https://www.din.de/en/wdc-beuth:din21:154167737): 1 krav, 1 testfall
- [EU:s riktlinjer för god tillverkningssed (EudraLex volym 4)](https://health.ec.europa.eu/medicinal-products/eudralex/eudralex-volume-4_en) (EU GMP): 18 krav, 18 testfall
- [NIS-samarbetsgruppen: samordnad färdplan för övergången till postkvantkryptografi](https://digital-strategy.ec.europa.eu/en/library/coordinated-implementation-roadmap-transition-post-quantum-cryptography): 3 krav, 2 testfall
- [Riktlinjer från EDPB och artikel 29-gruppen](https://www.edpb.europa.eu/documents_en): 16 krav, 16 testfall
  - [EDPB Guidelines 9/2022](https://www.edpb.europa.eu/documents/guideline/guidelines-92022-on-personal-data-breach-notification-under-gdpr_en)
- [Vägledningar från ENISA](https://www.enisa.europa.eu/publications): 3 krav, 3 testfall
- [Vägledningar från samordningsgruppen för medicintekniska produkter (MDCG)](https://health.ec.europa.eu/medical-devices-sector/new-regulations/guidance-mdcg-endorsed-documents-and-other-guidance_en): 1 krav, 1 testfall

#### Riktlinjer och vägledningar: Storbritannien

- [Brittiska HSE HSG250, vägledning om arbetstillståndssystem](https://www.hse.gov.uk/pubns/books/hsg250.htm): 3 krav, 3 testfall

#### Riktlinjer och vägledningar: USA

- [CISA:s vägledningar och kataloger (KEV, VEX)](https://www.cisa.gov/known-exploited-vulnerabilities-catalog): 9 krav, 9 testfall
  - [CISA Known Exploited Vulnerabilities Catalog](https://www.cisa.gov/known-exploited-vulnerabilities-catalog) · [CISA Minimum Requirements for VEX](https://www.cisa.gov/resources-tools/resources/minimum-requirements-vulnerability-exploitability-exchange-vex)
- [NIST IR 8547 Övergång till standarder för postkvantkryptografi](https://csrc.nist.gov/pubs/ir/8547/ipd): 5 krav, 4 testfall
- [NIST SP 800-190 Säkerhet för applikationscontainrar](https://doi.org/10.6028/NIST.SP.800-190): 15 krav, 15 testfall
- [NIST SP 800-204D Säkerhet i leveranskedjan för programvara i CI/CD-pipelines för DevSecOps](https://csrc.nist.gov/pubs/sp/800/204/d/final): 3 krav, 3 testfall
- [NIST SP 800-207 Nollförtroendearkitektur](https://doi.org/10.6028/NIST.SP.800-207): 17 krav, 17 testfall
- [NIST SP 800-209 Säkerhetsriktlinjer för lagringsinfrastruktur](https://doi.org/10.6028/NIST.SP.800-209): 1 krav, 1 testfall
- [NIST SP 800-34 Vägledning för kontinuitetsplanering](https://doi.org/10.6028/NIST.SP.800-34r1): 2 krav, 2 testfall
- [NIST SP 800-61 Vägledning för hantering av säkerhetsincidenter](https://doi.org/10.6028/NIST.SP.800-61r2): 9 krav, 9 testfall
- [NIST SP 800-63 Riktlinjer för digital identitet (63A, 63B)](https://pages.nist.gov/800-63-3/): 27 krav, 27 testfall
- [NIST SP 800-63C Riktlinjer för digital identitet: federation och intyg](https://pages.nist.gov/800-63-4/sp800-63c.html): 1 krav, 1 testfall
- [NIST SP 800-82 Vägledning för säkerhet i driftteknik (OT)](https://doi.org/10.6028/NIST.SP.800-82r3): 7 krav, 6 testfall
- [NIST SP 800-84 Program för test, utbildning och övning](https://doi.org/10.6028/NIST.SP.800-84): 1 krav, 3 testfall
- [NIST SP 800-88 Riktlinjer för sanering av lagringsmedia](https://doi.org/10.6028/NIST.SP.800-88r2): 5 krav, 5 testfall
- [NIST-vägledning för system- och plattformssäkerhet (SP 800-160, 800-193, 800-90B)](https://doi.org/10.6028/NIST.SP.800-160v1r1): 7 krav, 6 testfall
  - [NIST SP 800-160 Vol. 1 Rev. 1](https://doi.org/10.6028/NIST.SP.800-160v1r1) · [NIST SP 800-193](https://doi.org/10.6028/NIST.SP.800-193) · [NIST SP 800-90B](https://doi.org/10.6028/NIST.SP.800-90B)
- [NTIA:s minimikrav för en programvaruförteckning (SBOM)](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom) (NTIA SBOM): 1 krav, 1 testfall

<!-- END generated sources -->
