# Så fungerar Qtestframe

Den här sidan förklarar hur Qtestframe är uppbyggt och hur delarna fungerar tillsammans, utan att gå in i källkoden. Läs den när du vill förstå varför verktyget beter sig som det gör, eller när du ska förklara det för kollegor. Begreppen i fetstil förklaras i [Ordlistan](glossary.md); administratörer hittar detaljerna i [administratörshandboken](../../admin-guide/sv/index.md).

## Ett program i webbläsaren

Qtestframe är **ett enda program** på en server. Samma program innehåller webbapplikationen som du använder i webbläsaren, REST-API:et som webbapplikationen, CI-pipelines och andra verktyg anropar, och administratörens kommandoradsverktyg. Du behöver inte installera något på din dator: en aktuell webbläsare räcker, på dator, surfplatta eller mobil.

- Webbapplikationen är en så kallad single-page application: efter första inläsningen utbyter den bara data med servern, så sidbyten går snabbt och det du har skrivit men inte sparat ligger kvar medan du rör dig i appen.
- Allt du kan göra i webbläsaren kan också göras via API:et, som beskrivs med **[OpenAPI](glossary.md#openapi)** på `/api/v1/openapi.json`. Det är så [`qtestframe report`](runs.md#kommandot-qtestframe-report), JUnit-uppladdningen, Playwright-rapportören och andra verktyg fungerar.
- Användargränssnittet och handboken finns på svenska och engelska; [språkanalysen](requirements.md#språkanalysen) förstår kravtexter på båda språken.

## Var dina data finns

Allt sparas i **en databas**: organisationer, användare, projekt, krav, testfall, körningar och deras utfall, versionshistorik, kommentarer, händelseloggen och även uppladdade filer (bilagor, dokument, framtagna rapporter). Det finns ingen separat filserver.

- Databasen är **SQLite** (en enda fil på servern, ingen databasserver behövs) eller **PostgreSQL** (för större installationer eller flera servrar bakom en lastbalanserare). Du märker ingen skillnad; administratören väljer.
- Hemligheter som servern måste kunna läsa, till exempel token för ärendesystem eller webhookhemligheter, krypteras med en nyckel som förvaras utanför databasen (se [Säkerhet](#säkerhet)).
- Borttagna objekt hamnar i papperskorgen i 30 dagar innan de tas bort för gott.

## Organisationer, projekt och roller

En installation kan rymma flera **organisationer** som aldrig ser varandras data. Du kan vara medlem i flera och byta mellan dem i det övre fältet.

| Nivå | Vad som hör dit | Vem som hanterar det |
|---|---|---|
| Installation | användare, organisationer, systeminställningar, säkerhetskopior | superadministratörer |
| Organisation | medlemmar och deras roller, projekt, API-nycklar, testkonfigurationer, inställningar för testexekvering, händelseloggen | organisationens administratörer |
| Projekt | krav, testfall, körningar, testcykler, utforskande sessioner, defekter, risker, releaser, undantag, baslinjer, dokument, projektinställningar | ansvariga (och projektroller) |

Vad du får göra avgörs av din **roll** i organisationen: *Läsare*, *Granskare*, *Testare*, *Testledare*, *Ansvarig* eller *Administratör*, eller en roll som organisationen har skapat. En roll är en uppsättning behörigheter som `requirement.write` eller `run.execute`. Ett projekt kan begränsas till vissa medlemmar och ge någon en annan roll just i det projektet. Servern kontrollerar varje begäran mot behörigheterna; knappar som du inte får använda döljs eller är avstängda, och orsaken visas. Se [Roller och behörigheter](administration.md#roller-och-behörigheter).

## Objekten och hur de hänger ihop

![Diagram över objekten i ett projekt: krav med utvärderingskriterier och verifieringsposter, risker som kraven hanterar, testfall som verifierar dem, releaser, baslinjer, testcykler som skapar körningar, körningar som innehåller exekveringar av testfall, defekter som hittats i exekveringar och utforskande sessioner vars anteckningar blir defekter](../images/sv-item-model.png)

Varje objekt har en **nyckel** som består av projektnyckeln och ett nummer, till exempel `SHOP-REQ-12` eller `SHOP-TC-7`, en status, en ägare och en **versionshistorik**. De viktigaste sambanden:

| Objekt | Vad det är | Hänger ihop med |
|---|---|---|
| [Krav](requirements.md) | vad produkten ska, bör eller får göra | sina **utvärderingskriterier**; andra krav (*förfinar*, *härlett från*, *beror på* …); de **risker** det hanterar; de testfall som verifierar det |
| [Testfall](tests.md) | ett manuellt testfall med steg eller ett automatiserat med ett skript | de krav (och kriterier) det **verifierar** via spårlänkar; testfall som det anropar som delade steg eller beror på |
| [Körning](runs.md) | en testomgång i en miljö och ett bygge | sina **exekveringar**: en per testfall (och konfiguration), var och en med utfall, utfall per steg, bevis och defekter |
| [Testcykel](test-cycles.md) | planen för en testperiod | de testfall, testare och uppskattningar den planerar; körningen den skapar |
| [Utforskande session](exploratory-testing.md) | tidsbegränsad utforskande testning med ett uppdrag | kraven i dess omfattning; defekterna som skapats från dess anteckningar |
| [Defekt](defects.md) | ett funnet problem | de exekveringar, testfall och krav den är kopplad till; ett ärende i Jira, GitHub eller GitLab |
| [Risk](releases-and-risks.md#riskregistret) | något som kan gå fel, efter sannolikhet och konsekvens | de krav som hanterar den; MITRE-tekniker |
| [Release](releases-and-risks.md) | en version som ska levereras | dess **omfattning** (krav), dess körningar, dess **utgångskriterier**, undantag |
| [Undantag](waivers-and-soa.md) | ett signerat godkännande av krav som inte är uppfyllda | kraven och releasen det gäller, en risk |
| [Baslinje](baselines.md) | en fryst mängd versioner | versionen av varje krav och testfall vid ett visst tillfälle |
| [Dokument](documents.md) | en fil i projektets arkiv | objekten det är länkat till |

Täckning, verifiering, inaktualitet och om en release är redo **sparas aldrig**: de räknas fram ur objekten varje gång du tittar, så de är alltid aktuella.

## Från krav till release

Ett typiskt flöde genom ett projekt, och vad Qtestframe gör i varje steg:

1. **Skriv kraven.** Språkanalysen granskar texten medan du skriver och känner igen [kravnivån](requirements.md#kravnivåer) från SKA, BÖR, FÅR (eller MUST, SHOULD, MAY). Lägg till [utvärderingskriterier](requirements.md#utvärderingskriterier) som säger hur uppfyllandet bedöms. En [granskning](reviews-and-notifications.md#begär-granskning) godkänner dem.
2. **Ta fram testfall** och länka dem till kraven. Ett nytt krav kan få ett länkat testfall automatiskt. [Spårbarhetsmatrisen](traceability-and-coverage.md#spårbarhetsmatrisen) och [spårbarhetsreglerna](traceability-and-coverage.md#spårbarhetsregler) visar vad som saknas.
3. **Planera och kör.** En [testcykel](test-cycles.md) eller en [körning](runs.md) väljer ut testfallen; innan ett testfall exekveras kontrollerar [exekveringsreglerna](runs.md#när-kan-ett-testfall-exekveras) att det är klart. Testarna registrerar utfallen i exekveringsvyn; CI laddar upp de automatiserade utfallen.
4. **Följ upp.** Fel blir [defekter](defects.md); [täckning](traceability-and-coverage.md#så-beräknas-täckningen) och [verifiering](traceability-and-coverage.md#verifiering) härleds från de senaste utfallen; [översikten](traceability-and-coverage.md#översikten) visar läget.
5. **Ändra.** När ett krav eller testfall ändras i sak blir dess länkar **misstänkta**, dess utfall **inaktuella**, och [ändringspåverkan](traceability-and-coverage.md#ändringspåverkan) sprids vidare till relaterade objekt, dokument och ärenden.
6. **Släpp.** En [release](releases-and-risks.md) kontrollerar sina utgångskriterier och visar go eller no-go; [undantag](waivers-and-soa.md) godtar det som medvetet inte är uppfyllt; en [baslinje](baselines.md) fryser resultatet, och [rapporter](reports-and-export.md) dokumenterar det.

## Versioner, historik och baslinjer

Varje ändring av ett krav, ett testfall och de flesta andra objekt skapar en ny **version**: fliken **Versionshistorik** visar vem som ändrade vad och när, jämför versioner och återställer en gammal. **Ångra** i det övre fältet tar tillbaka dina senaste åtgärder. Två personer som redigerar samma objekt skriver inte över varandra: den andra sparningen avvisas tillsammans med den version den bygger på (en [ETag](glossary.md#etag-och-if-match)), och du väljer hur ändringarna ska slås ihop (se [När någon annan har sparat först](requirements.md#när-någon-annan-har-sparat-först)).

En **baslinje** registrerar versionen av varje objekt vid ett visst tillfälle, inför en release eller en revision. **Händelseloggen** registrerar åtgärder i hela organisationen: inloggningar, granskningsbeslut, signaturer, importer, exporter och inställningar.

## Frågespråket

Alla listor, spårbarhetsmatrisen, releasers omfattning, urvalet av testfall till körningar, rapporter, spårbarhetsregler och API:et använder samma [frågespråk](search-and-filters.md): `status:approved priority=high`, `-has:tests`, `linked:SHOP-TC-7`, `updated_at>=2026-01-01`, `title~/login/i`. Det du lär dig i en lista fungerar överallt. Den globala sökningen omfattar alla projekt i den aktuella organisationen som du kan läsa.

## Arbete i bakgrunden

En del arbete sker på servern utan att du behöver vänta på det:

- **Automatiserade testfall** som du startar med **Kör automatiserade tester** körs som jobb, några i taget; utfallen dyker upp i körningen allteftersom de blir klara.
- **Notiser** och **webhooks** skickas när ändringen som orsakade dem har sparats.
- **Sammanfattningar med e-post** och **schemalagda rapporter** skickas vid de tider du har valt.
- En daglig **ögonblicksbild** av varje projekts mått ger trenderna i översikten.
- Utgångna sessioner och poster som legat mer än 30 dagar i papperskorgen rensas varje timme.

## Notiser, webhooks och chatt

Du får veta det som berör dig: granskningsförfrågningar, omnämnanden, objekt som tilldelats dig, svar i dina diskussioner, statusändringar på dina defekter. [Notiserna](reviews-and-notifications.md#notiser) visas i appen och, om du vill, med e-post direkt eller som en daglig sammanfattning. [Mitt arbete](reviews-and-notifications.md#mitt-arbete) listar allt som väntar på dig i alla projekt.

Projekt kan också skicka sina händelser till andra system: **[webhooks](reviews-and-notifications.md#webhooks)** levererar signerade JSON-begäranden till valfri mottagare, och **[chattkanaler](reviews-and-notifications.md#chattaviseringar)** postar läsbara meddelanden i Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix eller Discord, filtrerade på händelse, prioritet och release.

## Integrationer

| Med | Hur | Läs mer |
|---|---|---|
| Defekt- och ärendesystem (Jira, GitHub Issues, GitLab, Request Tracker) | skicka defekter och krav som ärenden, synkronisera status, importera ärenden | [Defekter](defects.md#anslut-jira-github-eller-gitlab) |
| Jira och Azure DevOps | importera krav och uppdatera dem senare | [Rapporter och export](reports-and-export.md#jira-och-azure-devops) |
| Kodvärdar (GitHub, GitLab) | webhooks länkar incheckningar och merge requests som nämner en objektnyckel; byggen registrerar vilka incheckningar som testades | [Varianter och kod](variants-and-code.md#kodlänkar) |
| CI-pipelines | `qtestframe report`, uppladdning av JUnit XML, Playwright-rapportören, med en API-nyckel | [Importera utfall från CI](runs.md#importera-utfall-från-ci) |
| Krav- och ALM-verktyg (DOORS, DOORS Next, Polarion, Codebeamer, Jama, ProR) | ReqIF-filer åt båda hållen; levande OSLC-länkar, urval och skapande | [Samverkan](interoperability.md) |
| Kontorsprogram | export till PDF, Word, Excel, CSV och Markdown; redigera i Excel och importera tillbaka | [Rapporter och export](reports-and-export.md) |
| Identitetsleverantörer | enkel inloggning med OpenID Connect eller SAML 2.0, provisionering av användare med SCIM | [Administration](administration.md#provisionering-med-scim) |

Standarderna och formaten bakom integrationerna finns i [Ordlistan](glossary.md#uppslagsord-efter-ämne) och på sidan [Referenser](references.md#format-protokoll-och-ramverk).

## Säkerhet

### Sessioner och CSRF-skydd

När du loggar in skapar servern en **session**, och webbläsaren får en slumpmässig kaka som skript på sidan inte kan läsa. Sessionen upphör efter en högsta ålder (12 timmar som standard), när du loggar ut eller när en administratör återställer ditt lösenord eller stänger av ditt konto; **Profil → Aktiva sessioner** visar dina. Varje ändring du gör skickar en **CSRF-token** för sessionen och kontrolleras mot sitt ursprung, så att en annan webbplats inte kan agera i ditt namn. Skript och CI använder i stället **API-nycklar**, som visas en gång och bara sparas som hashvärde.

### Enkel inloggning och tvåstegsverifiering

Din organisation kan låta dig logga in med sin identitetsleverantör via **OpenID Connect** eller **SAML 2.0**; användare och grupper kan provisioneras automatiskt med **SCIM**. Lokala konton kan, eller måste, lägga till en andra faktor: en **TOTP**-kod från en autentiseringsapp, med engångskoder för återställning. Se [Tvåstegsverifiering](getting-started.md#tvåstegsverifiering).

### Hemligheter

Lösenord och nycklar som bara behöver kontrolleras sparas som hashvärden. Hemligheter som servern själv måste använda, till exempel hemligheter för autentiseringsappar, webhookhemligheter, token för ärendesystem och hemliga testvariabler, krypteras i databasen med en nyckel som förvaras utanför den (en nyckelfil, en miljövariabel, ett TPM eller en HSM). Hemligheter går **bara att skriva**: när de väl är sparade visar ingen sida eller något API dem igen, och projektarkiv innehåller dem aldrig.

### Sandlådan för skript

När administratören har aktiverat det körs skript i automatiserade testfall och webbtester i Playwright på servern. Varje skript körs i en egen, begränsad process: begränsad processortid, filstorlek och antal öppna filer, inga nya privilegier, filåtkomst begränsad till den egna jobbkatalogen med **Landlock**, farliga systemanrop spärrade med **seccomp** och om så önskas inget nätverk. Servern begränsar sig själv på samma sätt efter starten. Bara personer med behörigheten `script.execute` kan köra skript.

## Säkerhetskopior och projektarkiv

- En **säkerhetskopia** innehåller hela installationen: alla organisationer med deras data och filer. Superadministratörer tar den i webbklienten eller på servern; en återställning ersätter allt och loggar ut alla. Se [Säkerhetskopiering och återställning](administration.md#säkerhetskopiering-och-återställning).
- Ett **projektarkiv** innehåller ett projekt, med historik, utfall och bilagor men utan hemligheter, som en ZIP-fil. Läs in det som ett nytt projekt på samma eller en annan server. Se [Spara och läs in projekt](reports-and-export.md#spara-och-läs-in-projekt).
- **Exporter** (PDF, Word, Excel, CSV, Markdown, ReqIF) tar ut data till andra personer och verktyg.

## För administratörer

[Administratörshandboken](../../admin-guide/sv/index.md) beskriver hur man driver Qtestframe:

- [översikt och arkitektur](../../admin-guide/sv/overview.md): komponenter, datalager, portar, utgående anslutningar och bakgrundsuppgifter;
- [installation](../../admin-guide/sv/installation.md) och [konfiguration](../../admin-guide/sv/configuration.md), även valet mellan SQLite och PostgreSQL;
- [autentisering](../../admin-guide/sv/authentication.md) (lösenord, policyer för tvåstegsverifiering, OIDC, SAML, SCIM, API-nycklar, sessioner) och [behörigheter](../../admin-guide/sv/authorisation.md);
- [säkerhet](../../admin-guide/sv/security.md): TLS, härdning med Landlock och seccomp, sandlådan för skript, kryptering av lagrade hemligheter och nyckelbyte;
- [integrationer](../../admin-guide/sv/integrations.md), [att koppla ihop med andra verktyg](../../admin-guide/sv/connecting-tools.md) via OSLC och ReqIF, [loggning och övervakning](../../admin-guide/sv/logging-and-monitoring.md) med syslog;
- [säkerhetskopiering, återställning och migrering](../../admin-guide/sv/backup-and-migration.md), [driftrutinerna](../../admin-guide/sv/runbook.md) och [kommandoradsreferensen](../../admin-guide/sv/cli-reference.md).
