# Användarhandbok för Qtestframe

Qtestframe är ett verktyg för att hantera **kravspecifikationer och tester** på ett ställe. Du skriver krav, utformar manuella och automatiserade testfall som verifierar dem, genomför testkörningar, registrerar utfall och defekter och visar täckningen med en spårbarhetsmatris, en översikt och rapporter. Flera organisationer kan dela en installation utan att se varandras data.

Handboken riktar sig till alla som arbetar i Qtestframe: kravförfattare, testare, testledare och administratörer. Den beskriver webbklienten så som du ser den. Du kan läsa den här i appen (**Hjälp → Användarhandbok** eller **?**-länken på många sidor) eller som Markdown-filer i mappen `docs/user-guide` i källkoden.

![Projektlistan efter inloggningen, med sidopanelen till vänster och övre fältet med sökning, hjälp, tema och användarmeny](../images/sv-layout.png)

## Var börjar jag?

- **Ny i Qtestframe?** Läs [Kom igång](getting-started.md) och gå sedan igenom handledningen [Din första testkampanj](tutorial.md). Den tar ungefär 30 minuter och berör alla viktiga funktioner.
- **Letar du efter ett svar?** Titta i [Vanliga frågor](faq.md) eller använd sökfältet högst upp i handboken. Okända ord förklaras i [Ordlistan](glossary.md).
- **Vill du se helheten?** [Så fungerar Qtestframe](how-it-works.md) förklarar hur verktyget är uppbyggt och hur objekten hänger ihop.
- **Administratör?** Gå direkt till [Administration](administration.md).

## Innehåll

| Sida | Innehåll |
|---|---|
| [Kom igång](getting-started.md) | Inloggning, sidans uppbyggnad, organisationer, sökning, tema och språk, profil och inställningar |
| [Handledning: din första testkampanj](tutorial.md) | En komplett genomgång från ett tomt projekt till en rapport |
| [Krav](requirements.md) | Kravnivåer, språkanalysen, fält, utvärderingskriterier, relationer, kommentarer, versionshistorik, massändring, flytt och bilagor |
| [Testfall](tests.md) | Manuella steg, automatiserade skript, delade steg, kategorier, exekveringshistorik, kommentarer, versionshistorik och bilagor |
| [Katalog](catalogue.md) | Standardkrav och testfall att lägga till i ett projekt: bläddra, söka, förhandsvisa och lägga till med testfall och spårlänkar |
| [Testkörningar och exekvering](runs.md) | Körningar, exekveringsvyn och dess kortkommandon, defekter, CI-import med API-nycklar, bilagor |
| [Utforskande test](exploratory-testing.md) | Sessionsbaserad utforskande testning: uppdrag, sessionens tidtagning, typade anteckningar med kortkommandon, defekter från anteckningar, utvärderingar, mått och sessionsrapporten |
| [Undantag, restrisk och SoA](waivers-and-soa.md) | Undantag och avvikelser med signerade godkännanden, restrisk och dess acceptans, uttalandet om tillämplighet (ISO/IEC 27001 bilaga A) |
| [Ändringsbegäranden och ändringshantering](change-requests.md) | Att föreslå ändringar av godkända krav och tester, bedömningen av påverkan, ändringsnämndens signerade beslut, att genomföra, verifiera och stänga; tvingande ändringshantering och nödåtgärden |
| [Spårbarhet och täckning](traceability-and-coverage.md) | Spårbarhetsmatrisen, hur täckningen beräknas, översikten |
| [Testcykler](test-cycles.md) | Planera testcykler: testfall, testare och uppskattningar, körningarna en cykel skapar, framsteg, nedbränning, arbetsbelastning, återkommande cykler |
| [Dubbletter och konflikter](duplicates-and-conflicts.md) | Liknande krav medan du skriver, misstänkt dubblerade krav och testfall, möjligen motstridiga krav, sammanslagning, gränsvärden |
| [Varianter och kod](variants-and-code.md) | Varianter och tillämplighet, täckning per variant, incheckningar, merge requests, byggen och regressionsurvalet |
| [CI/CD](ci-cd.md) | Pipelinemallar för GitLab och GitHub, uppladdning av resultat, kvalitetsgrinden, commit-statusar och anteckningar i merge requests, testurval och underpipelines |
| [Baslinjer](baselines.md) | Frysa en release och jämföra versioner |
| [Diskussioner](discussions.md) | Kommentarstrådar på objekt med Markdown, @omnämnanden, reaktioner och lösta trådar, och projektets sida Diskussioner |
| [Sökning och filter](search-and-filters.md) | Frågespråket med många exempel |
| [Rapporter och export](reports-and-export.md) | Standardrapporter, den egna rapportbyggaren, exportformat och dokumentarkivet |
| [Samverkan](interoperability.md) | ReqIF-export och -import med testfall, relationer och bilagor; OSLC-länkar, urval och skapande från DOORS Next, Polarion, Codebeamer |
| [Dokument](documents.md) | Projektets dokumentarkiv: uppladdning, förhandsvisning och länkar |
| [Administration](administration.md) | Organisationer, användare, medlemmar och roller, delning av projekt, återställning av lösenord, API-nycklar, händelseloggen, säkerhetskopiering och återställning |
| [Tillgänglighet och språk](accessibility-and-languages.md) | Tangentbord, skärmläsare, teman, engelska och svenska, mobiler och surfplattor |
| [Så fungerar Qtestframe](how-it-works.md) | Programmet, var data finns, organisationer, projekt och roller, objekten och hur de hänger ihop (med ett diagram), frågespråket, arbete i bakgrunden, notiser, integrationer, säkerhet, säkerhetskopior och hänvisningar för administratörer |
| [Vanliga frågor](faq.md) | Svar på vanliga frågor om hur verktyget används, begrepp inom test och kravhantering, standarder och format, och hur Qtestframe fungerar |
| [Ordlista](glossary.md) | Begrepp inom test, kravhantering, standarder och Qtestframe i bokstavsordning, vart och ett med vad det betyder i Qtestframe |
| [Referenser](references.md) | RFC 2119, RFC 8174 och andra RFC:er, NIST-publikationer, standarder för krav och test samt de format, protokoll och ramverk som Qtestframe använder |

## Viktiga begrepp

| Begrepp | Betydelse |
|---|---|
| **Organisation** | En isolerad arbetsyta. Projekt, medlemmar och API-nycklar hör till en organisation. Du kan vara medlem i flera och byta mellan dem. |
| **Projekt** | En behållare för krav, testfall, körningar, baslinjer och dokument. Projektets **nyckel** (till exempel `SHOP`) inleder varje objekts id: `SHOP-REQ-12`, `SHOP-TC-7`. |
| **Krav** | Ett påstående om vad systemet ska, bör eller får göra, med kravnivå, status, prioritet, utvärderingskriterier med mera. |
| **Utvärderingskriterium** | Hur man bedömer att ett krav är uppfyllt: en metod (test, inspektion, analys, demonstration, granskning) och ett tröskelvärde. |
| **Testfall** | Ett manuellt testfall med steg eller ett automatiserat testfall med ett skript. |
| **Länk** (spårlänk) | Kopplar ett krav till ett testfall som verifierar det. Länkarna är grunden för täckningen. |
| **Körning** (testkörning) | En testkampanj, till exempel *Regressionstest sprint 12*, med en uppsättning testfall som ska exekveras i en viss miljö och ett visst bygge. |
| **Exekvering** | Ett testfall i en körning, med utfall, utfall per steg, anteckningar, defekter och utdata. |
| **Täckning** | Ett kravs verifieringsläge, härlett från de senaste utfallen för dess länkade testfall: *Ingen täckning*, *Ej körd*, *Godkänd*, *Underkänd* eller *Delvis*. |
| **Baslinje** | En fryst ögonblicksbild av versionerna av ett projekts krav och testfall, till exempel inför en release. |
| **Roll** | Vad du får göra i en organisation: *Läsare*, *Testare*, *Ansvarig* eller *Administratör*. *Superadministratörer* hanterar hela installationen. |

## Konventioner i handboken

- Namn på knappar, menyer och fält skrivs i **fetstil**, precis som de står i det svenska gränssnittet. Den engelska handboken använder de engelska etiketterna.
- Menyvägar skrivs med pilar: **Användarmeny → Profil och lösenord**.
- Tangenter skrivs som <kbd>Enter</kbd> och frågeexempel som `status:approved`.
- En anmärkning som *(Ansvarig)* efter en åtgärd anger den lägsta roll som får utföra den. Se [rollmatrisen](administration.md#roller-och-behörigheter).
