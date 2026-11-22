# Ordlista

Begreppen inom test, kravhantering och de standarder och format som Qtestframe bygger på, förklarade med några meningar vardera, med vad de betyder i Qtestframe och var du kan läsa mer. Förklaringarna är skrivna för den här handboken; när ett begrepp kommer från en standard eller en etablerad terminologi som ISTQB:s ordlista eller ISO/IEC/IEEE 29119 anges källan, och den finns på sidan [Referenser](references.md). Helheten beskrivs i [Så fungerar Qtestframe](how-it-works.md).

**Hoppa till:** [A](#a) · [B](#b) · [C](#c) · [D](#d) · [E](#e) · [F](#f) · [G](#g) · [H](#h) · [I](#i) · [J](#j) · [K](#k) · [L](#l) · [M](#m) · [N](#n) · [O](#o) · [P](#p) · [Q](#q) · [R](#r) · [S](#s) · [T](#t) · [U](#u) · [V](#v) · [W](#w) · [Ä](#ä)

## Uppslagsord efter ämne

**Test:** [Acceptanstest](#acceptanstest) · [Allvarlighetsgrad](#allvarlighetsgrad) · [Användbarhetstest](#användbarhetstest) · [Beslutstabelltest](#beslutstabelltest) · [Beteendedriven utveckling](#beteendedriven-utveckling) · [Datadriven testning](#datadriven-testning) · [Defekt](#defekt) · [Defektens livscykel](#defektens-livscykel) · [Ekvivalensklassindelning](#ekvivalensklassindelning) · [End-to-end-test](#end-to-end-test) · [Enhetstest](#enhetstest) · [Exekvering](#exekvering) · [Felgissning](#felgissning) · [Funktionell testning](#funktionell-testning) · [Grålådetest](#grålådetest) · [Gränsvärdesanalys](#gränsvärdesanalys) · [Icke-funktionell testning](#icke-funktionell-testning) · [Ingångskriterier](#ingångskriterier) · [Instabilt testfall](#instabilt-testfall) · [Integrationstest](#integrationstest) · [Kodtäckning](#kodtäckning) · [Kontinuerlig integration](#kontinuerlig-integration) · [Kravtäckning](#kravtäckning) · [Lasttest](#lasttest) · [Parvis testning](#parvis-testning) · [Prestandatest](#prestandatest) · [Prioritet](#prioritet) · [Regressionstest](#regressionstest) · [Röktest och sanitetstest](#röktest-och-sanitetstest) · [Sessionsbaserad testledning](#sessionsbaserad-testledning) · [Shift-left](#shift-left) · [Stresstest](#stresstest) · [Svartlådetest](#svartlådetest) · [Systemtest](#systemtest) · [Säkerhetstest](#säkerhetstest) · [Testcykel](#testcykel) · [Testdata](#testdata) · [Testdesignteknik](#testdesignteknik) · [Testdriven utveckling](#testdriven-utveckling) · [Testfall](#testfall) · [Testkonfiguration](#testkonfiguration) · [Testkörning](#testkörning) · [Testmiljö](#testmiljö) · [Testnivå](#testnivå) · [Testorakel](#testorakel) · [Testplan](#testplan) · [Teststeg](#teststeg) · [Testsvit](#testsvit) · [Testtyp](#testtyp) · [Testutfall](#testutfall) · [Tillgänglighetstest](#tillgänglighetstest) · [Tillståndsövergångstest](#tillståndsövergångstest) · [Täckning](#täckning) · [Utforskande testning](#utforskande-testning) · [Utgångskriterier](#utgångskriterier) · [V-modellen](#v-modellen) · [Validering](#validering) · [Verifiering](#verifiering) · [Vitlådetest](#vitlådetest)

**Krav:** [Acceptanskriterier](#acceptanskriterier) · [Användarkrav](#användarkrav) · [Avvikelse](#avvikelse) · [Bakåtspårbarhet](#bakåtspårbarhet) · [Baslinje](#baslinje) · [Begränsning](#begränsning) · [Dubbelriktad spårbarhet](#dubbelriktad-spårbarhet) · [EARS](#ears) · [Framåtspårbarhet](#framåtspårbarhet) · [Funktionellt krav](#funktionellt-krav) · [Förfining](#förfining) · [Härlett krav](#härlett-krav) · [Icke-funktionellt krav](#icke-funktionellt-krav) · [INCOSE-regler](#incose-regler) · [Intressent](#intressent) · [Intressentkrav](#intressentkrav) · [Krav](#krav) · [Kravkvalitet](#kravkvalitet) · [Kravnivå](#kravnivå) · [Kravord](#kravord) · [Kvalitetsegenskap](#kvalitetsegenskap) · [Misstänkt länk](#misstänkt-länk) · [Programvarukrav](#programvarukrav) · [Restrisk](#restrisk) · [Risk](#risk) · [Riskbaserad testning](#riskbaserad-testning) · [SKA, BÖR och FÅR](#ska-bör-och-får) · [Specifikation](#specifikation) · [Spårbarhet](#spårbarhet) · [Spårbarhetsmatris](#spårbarhetsmatris) · [Spårlänk](#spårlänk) · [Systemkrav](#systemkrav) · [Undantag](#undantag) · [Uttalande om tillämplighet](#uttalande-om-tillämplighet) · [Utvärderingskriterium](#utvärderingskriterium) · [Verifieringsmetod](#verifieringsmetod) · [Verifieringspost](#verifieringspost) · [Verksamhetskrav](#verksamhetskrav) · [Ändringspåverkan](#ändringspåverkan)

**Standarder, format och ramverk:** [AI-förordningen](#ai-förordningen) · [Bearer-token](#bearer-token) · [CSV](#csv) · [CWE](#cwe) · [Cyberresiliensförordningen](#cyberresiliensförordningen) · [ETag och If-Match](#etag-och-if-match) · [HOTP](#hotp) · [HTTP-semantik](#http-semantik) · [ISO/IEC 25010](#isoiec-25010) · [ISO/IEC 27001 bilaga A](#isoiec-27001-bilaga-a) · [ISO/IEC/IEEE 29119](#isoiecieee-29119) · [ISO/IEC/IEEE 29148](#isoiecieee-29148) · [ISTQB](#istqb) · [JSON Web Token](#json-web-token) · [JUnit XML](#junit-xml) · [Landlock](#landlock) · [Markdown](#markdown) · [MITRE ATLAS](#mitre-atlas) · [MITRE ATT&CK](#mitre-attck) · [MITRE D3FEND](#mitre-d3fend) · [MITRE EMB3D](#mitre-emb3d) · [OAuth 2.0](#oauth-20) · [OpenAPI](#openapi) · [OpenID Connect](#openid-connect) · [OSLC](#oslc) · [PKCE](#pkce) · [Problem Details](#problem-details) · [ReqIF](#reqif) · [SAML 2.0](#saml-20) · [SCIM](#scim) · [seccomp](#seccomp) · [STIX](#stix) · [Syslog](#syslog) · [TOTP](#totp) · [WCAG](#wcag) · [WebAuthn och FIDO2](#webauthn-och-fido2)

**Qtestframe:** [API-nyckel](#api-nyckel) · [Bakgrundsjobb](#bakgrundsjobb) · [CSRF-skydd](#csrf-skydd) · [Databas](#databas) · [Delat steg](#delat-steg) · [Diskussion](#diskussion) · [Frågespråk](#frågespråk) · [Granskning](#granskning) · [Händelselogg](#händelselogg) · [Inaktuellt utfall](#inaktuellt-utfall) · [Iteration](#iteration) · [Katalog](#katalog) · [Kodlänk](#kodlänk) · [Konsistenskontroll](#konsistenskontroll) · [Kryptering av hemligheter](#kryptering-av-hemligheter) · [Notis](#notis) · [Nyckel](#nyckel) · [Organisation](#organisation) · [Papperskorg](#papperskorg) · [Projekt](#projekt) · [Projektarkiv](#projektarkiv) · [qtestframe report](#qtestframe-report) · [Release](#release) · [Roll](#roll) · [Session](#session) · [Skriptkörning](#skriptkörning) · [Spårbarhetsregel](#spårbarhetsregel) · [Språkanalys](#språkanalys) · [Systemelement](#systemelement) · [Säkerhetskopia](#säkerhetskopia) · [Tvåstegsverifiering](#tvåstegsverifiering) · [Variant](#variant) · [Version](#version) · [Webbtest](#webbtest) · [Webhook](#webhook) · [Ärendesystem](#ärendesystem)

## A

### Acceptanskriterier

De villkor som en produkt eller ett krav ska uppfylla för att godtas av kunden, en användare eller någon annan med befogenhet. Bra acceptanskriterier är konkreta och kontrollerbara, så att alla i förväg är överens om vad ”klart” betyder. I agilt arbete skrivs de ofta under en användarberättelse; i klassiska specifikationer är de verifieringsdelen av varje krav.

**I Qtestframe:** acceptanskriterier skrivs som [utvärderingskriterier](#utvärderingskriterium) för ett krav: en beskrivning, en metod och ett valfritt tröskelvärde. Ett krav räknas som verifierat först när varje kriterium har godkända bevis.

**Se även:** [Utvärderingskriterium](#utvärderingskriterium), [Acceptanstest](#acceptanstest), [Utvärderingskriterier](requirements.md#utvärderingskriterier)

### Acceptanstest

Den testnivå som avgör om ett system är redo att tas emot: av användarna (användaracceptanstest), av driften (driftacceptanstest) eller mot ett avtal eller en föreskrift. Den svarar på ”är det här vad vi behöver?” snarare än ”fungerar koden?” och är därför till stor del [validering](#validering). Acceptanstest härleds oftast från [acceptanskriterier](#acceptanskriterier) och verksamhetens processer (ISTQB:s ordlista).

**I Qtestframe:** planera acceptanstestningen som en egen [testcykel](#testcykel) eller [körning](#testkörning), till exempel *Acceptanstest release 2.1*, och tagga eller kategorisera testfallen (`tag:acceptance`). Mallen för krav av typen *Verksamhet* föreslår ett acceptanstest.

**Se även:** [Testnivå](#testnivå), [Systemtest](#systemtest), [Release](#release)

### AI-förordningen

Förordning (EU) 2024/1689 om artificiell intelligens. Den delar in AI-system efter risk och ställer krav på system med hög risk, till exempel riskhantering, datastyrning, teknisk dokumentation, loggning, mänsklig tillsyn, noggrannhet och robusthet, och krav på transparens för vissa andra.

**I Qtestframe:** katalogen har krav, testfall och risker för AI-förordningen (riskerna hänvisar också till tekniker i MITRE ATLAS); välj förordningen under **Uppfyll lagar, standarder och bästa praxis**.

**Se även:** [MITRE ATLAS](#mitre-atlas), [Cyberresiliensförordningen](#cyberresiliensförordningen), [Katalog](#katalog)

### Allvarlighetsgrad

Hur allvarlig effekten av en defekt är för systemet eller användarna, från krasch eller dataförlust till ett stavfel. Allvarlighetsgraden är en teknisk bedömning; [prioriteten](#prioritet) är beslutet om hur snart felet ska åtgärdas.

**I Qtestframe:** defekter har allvarlighetsgraderna *Kritisk*, *Allvarlig*, *Mindre* och *Obetydlig*; utgångskriteriet *Inga öppna defekter* och reglerna för blockerande defekter använder en lägsta allvarlighetsgrad.

**Se även:** [Prioritet](#prioritet), [Blockerande defekter](traceability-and-coverage.md#blockerande-defekter), [Defekt](#defekt)

### Användarkrav

Ett krav som beskriver vad en användare ska kunna göra med systemet, ofta skrivet som ett användningsfall eller en användarberättelse (”Som kund vill jag …”). Det är en sorts intressentkrav med fokus på användarnas uppgifter.

**I Qtestframe:** skriv användarberättelser som krav och deras acceptanskriterier som utvärderingskriterier; förfina dem till system- eller programvarukrav där det behövs.

**Se även:** [Intressentkrav](#intressentkrav), [Acceptanskriterier](#acceptanskriterier), [Beteendedriven utveckling](#beteendedriven-utveckling)

### Användbarhetstest

Test av hur lätt, effektivt och tillfredsställande verkliga användare når sina mål med produkten, oftast genom att representativa användare får utföra uppgifter medan man observerar och mäter hur många som lyckas, tidsåtgång och fel.

**I Qtestframe:** formulera användbarhetsmål som krav av typen *Användbarhet* med mätbara kriterier, och registrera iakttagelser från testtillfällena som utforskande sessioner eller som verifieringsposter med metoden *Demonstration*.

**Se även:** [Tillgänglighetstest](#tillgänglighetstest), [Icke-funktionell testning](#icke-funktionell-testning), [Utforskande testning](#utforskande-testning)

### API-nyckel

En hemlig teckensträng som ett skript eller en CI-pipeline skickar i stället för ett lösenord när den anropar API:et. Den hör till en användare och en organisation, har en roll som kan vara lägre än användarens och kan löpa ut eller återkallas.

**I Qtestframe:** nycklarna börjar med `qtf_`, skickas som [bearer-token](#bearer-token) och visas bara en gång, när du skapar dem; servern sparar bara ett hashvärde. Du behöver en nyckel för [`qtestframe report`](#qtestframe-report), JUnit-uppladdningen och Playwright-rapportören.

**Se även:** [API-nycklar](administration.md#api-nycklar), [Bearer-token](#bearer-token), [Importera utfall från CI](runs.md#importera-utfall-från-ci)

### Avvikelse

Ett godkänt avsteg från ett krav där kravet uppfylls på ett annat sätt än det specificerade, oftast med kompenserande åtgärder. Till skillnad från ett [undantag](#undantag) uppnås ändå kravets syfte.

**I Qtestframe:** en avvikelse är ett undantag av typen *Avvikelse*, med den valda lösningen, ett signerat godkännande och ett valfritt slutdatum.

**Se även:** [Undantag](#undantag), [Undantag och avvikelser](waivers-and-soa.md#undantag-och-avvikelser), [Restrisk](#restrisk)

## B

### Bakgrundsjobb

Arbete som servern gör på egen hand, utanför ett enskilt klick: rensar utgångna sessioner, sparar dagliga mätvärden, skickar sammanfattningar och schemalagda rapporter med e-post, levererar webhooks och kör automatiserade testfall. Jobben körs i serverprocessen; det finns ingen separat jobbserver.

**I Qtestframe:** du ser resultatet, inte jobben: trender i översikten, sammanfattningar i inkorgen, utfall som fylls i medan **Kör automatiserade tester** arbetar sig igenom en körning.

**Se även:** [Arbete i bakgrunden](how-it-works.md#arbete-i-bakgrunden), [Webhook](#webhook), [Skriptkörning](#skriptkörning)

### Bakåtspårbarhet

Spårning från en senare eller mer detaljerad artefakt tillbaka till dess ursprung: från ett testfall till kravet det verifierar, från ett programvarukrav till system- eller intressentkravet det härleddes från. Den visar varför något finns och hittar ”föräldralösa” objekt utan anledning.

**I Qtestframe:** ett testfalls **Länkade krav**, kravens relationer *förfinar* och *härlett från* samt [spårbarhetsregler](#spårbarhetsregel) som ”varje testfall verifierar ett krav” kontrollerar bakåtspårbarheten.

**Se även:** [Framåtspårbarhet](#framåtspårbarhet), [Dubbelriktad spårbarhet](#dubbelriktad-spårbarhet), [Spårbarhetsregler](traceability-and-coverage.md#spårbarhetsregler)

### Baslinje

Ett överenskommet, fryst läge för en mängd objekt som är utgångspunkt för det fortsatta arbetet och för ändringar. Typiska baslinjer är den godkända specifikationen för en release eller testunderlaget som lämnas till en leverantör. Ändringar efter baslinjen görs under ändringsstyrning och jämförs mot den.

**I Qtestframe:** en baslinje registrerar vilken version av varje krav och testfall som hörde till den; den kopierar inget som går att redigera, så att ta bort den ändrar aldrig objekten. Du kan jämföra två baslinjer, se [ändringspåverkan](#ändringspåverkan) sedan en baslinje, signera den och exportera den som ReqIF.

**Se även:** [Version](#version), [Baslinjer](baselines.md), [Release](#release)

### Bearer-token

En åtkomsttoken som gäller för den som har den, skickad i HTTP-huvudet `Authorization: Bearer <token>`. RFC 6750 beskriver detta sätt att skicka åtkomsttoken i OAuth 2.0; eftersom vem som helst som har token kan använda den måste den skickas över TLS och hållas hemlig.

**I Qtestframe:** [API-nycklar](#api-nyckel) och SCIM-nycklar skickas i just det huvudet. Webbklienten använder inga token utan en kaka för [sessionen](#session).

**Se även:** [API-nyckel](#api-nyckel), [OAuth 2.0](#oauth-20), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### Begränsning

Ett krav som begränsar hur en lösning får byggas i stället för att säga vad den gör: en föreskriven plattform, ett programspråk, ett standardgränssnitt, en lag, en budget. Begränsningar smalnar av lösningsutrymmet och går ofta inte att förhandla bort.

**I Qtestframe:** välj typen **Begränsning**. Verifiera med **Inspektion** eller **Analys** när det inte finns något att exekvera.

**Se även:** [Icke-funktionellt krav](#icke-funktionellt-krav), [Verifieringsmetod](#verifieringsmetod), [Krav](#krav)

### Beslutstabelltest

En testdesignteknik för regler som kombinerar flera villkor. Varje kolumn i tabellen är en kombination av villkor med de förväntade åtgärderna, och varje kolumn blir minst ett testfall. Tekniken synliggör regler som saknas eller motsäger varandra (ISO/IEC/IEEE 29119-4).

**I Qtestframe:** klistra in beslutstabellen som [testdata](#testdata): en rad per regel och en parameter per villkor och förväntat utfall.

**Se även:** [Testdesignteknik](#testdesignteknik), [Parvis testning](#parvis-testning), [Datadriven testning](#datadriven-testning)

### Beteendedriven utveckling

BDD: ett arbetssätt där teamet beskriver det önskade beteendet som konkreta exempel i ett strukturerat språk (*Givet … När … Så …*) innan det byggs. Exemplen fungerar som krav, som acceptanskriterier och, när de automatiseras, som testfall.

**I Qtestframe:** skriv ett scenario som steg i ett testfall (Givet som förutsättning, När som åtgärd, Så som förväntat resultat) och länka det till kravet. Automatiserade scenarier rapporterar sina utfall med JUnit XML som andra testfall.

**Se även:** [Testdriven utveckling](#testdriven-utveckling), [Acceptanskriterier](#acceptanskriterier), [JUnit XML](#junit-xml)

## C

### CSRF-skydd

Förfalskning av begäranden mellan webbplatser (cross-site request forgery) är en attack där en annan webbplats får din webbläsare att skicka en begäran till ett program som du är inloggad i. Skyddet är en hemlig token per session som sidan måste skicka med varje ändring och som en främmande webbplats inte kan känna till, och en kontroll av var begäran kommer ifrån.

**I Qtestframe:** varje ändring från webbklienten skickar sessionens CSRF-token i ett huvud och kontrolleras mot ursprunget; sessionskakan är dessutom `SameSite=Lax`. Anrop med [bearer-token](#bearer-token) behöver ingen CSRF-token.

**Se även:** [Session](#session), [Säkerhet](how-it-works.md#säkerhet), [Sessioner](../../admin-guide/sv/authentication.md#sessioner)

### CSV

Kommaseparerade värden: en tabell i ren text med en post per rad. RFC 4180 beskriver det vanliga formatet, bland annat hur fält som innehåller kommatecken, citattecken eller radbrytningar citeras.

**I Qtestframe:** CSV-exporten skriver filer enligt RFC 4180 i UTF-8, en fil per aspekt eller en ZIP-fil med flera. CSV-filer från DOORS eller Polarion kan importeras med en kolumnmappning.

**Se även:** [Export](reports-and-export.md#export), [Utbyta data med andra verktyg](reports-and-export.md#utbyta-data-med-andra-verktyg), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### CWE

Common Weakness Enumeration från MITRE: en numrerad förteckning över typer av svagheter i program- och maskinvara, till exempel *CWE-79 Cross-site scripting* eller *CWE-521 Weak password requirements*. Den ger säkerhetskrav och fynd ett gemensamt språk.

**I Qtestframe:** krav, testfall och risker i katalogen hänvisar till CWE-poster, och sökningen hittar dem på id (`CWE-79`).

**Se även:** [MITRE ATT&CK](#mitre-attck), [Katalog](#katalog), [Källor som katalogen hänvisar till](references.md#källor-som-katalogen-hänvisar-till)

### Cyberresiliensförordningen

Förordning (EU) 2024/2847 (Cyber Resilience Act, CRA) om cybersäkerhetskrav för produkter med digitala element: inbyggd säkerhet, inga kända utnyttjbara sårbarheter, säkerhetsuppdateringar och sårbarhetshantering under supportperioden. De flesta skyldigheterna gäller från december 2027.

**I Qtestframe:** välj *Cyberresiliensförordningen* under **Uppfyll lagar, standarder och bästa praxis** i katalogen för att lägga till dess väsentliga krav med testfall; efterlevnadsanalysen visar sedan hur långt de är verifierade.

**Se även:** [AI-förordningen](#ai-förordningen), [Efterlevnad av lagar och standarder](traceability-and-coverage.md#efterlevnad-av-lagar-och-standarder), [Katalog](#katalog)

## D

### Databas

Där ett program sparar sina data. Qtestframe sparar allt, även uppladdade filer, i en relationsdatabas.

**I Qtestframe:** SQLite (en enda fil, ingen databasserver, standardvalet) eller PostgreSQL (för större installationer och flera serverinstanser). Användarna märker ingen skillnad; administratören väljer vid installationen.

**Se även:** [Var dina data finns](how-it-works.md#var-dina-data-finns), [Säkerhetskopia](#säkerhetskopia), [SQLite eller PostgreSQL](../../admin-guide/sv/installation.md#sqlite-eller-postgresql)

### Datadriven testning

Att skriva ett testfall en gång och köra det med flera uppsättningar indata och förväntade värden ur en tabell. Testlogiken skiljs från testdatan, så att ett nytt fall bara är en ny rad.

**I Qtestframe:** ge testfallet parametrar och rader under **Testdata** och använd dem som `{{namn}}` i stegen. Varje rad blir en **iteration** av exekveringen med egna utfall per steg.

**Se även:** [Datadrivna testfall](tests.md#datadrivna-testfall), [Iteration](#iteration), [Testdata](#testdata)

### Defekt

En brist i en arbetsprodukt som kan göra att den inte uppfyller ett krav: i kod, i ett dokument, i en konfiguration. Ett felbeteende (failure) är det du ser när en defekt exekveras; ett misstag (error) är den mänskliga handling som orsakade den (ISTQB:s ordlista). I dagligt tal används defekt, bugg och fel om samma sak.

**I Qtestframe:** defekter har status, [allvarlighetsgrad](#allvarlighetsgrad), [prioritet](#prioritet) och den fas de hittades i, och kopplas till krav, testfall och exekveringar. Öppna defekter kan blockera krav och releaser, och de kan skickas till Jira, GitHub eller GitLab.

**Se även:** [Defekter](defects.md), [Defektens livscykel](#defektens-livscykel), [Ärendesystem](#ärendesystem)

### Defektens livscykel

De tillstånd en defekt går igenom från rapport till avslut, och vem som flyttar den mellan dem. En typisk cykel är rapporterad → pågår → åtgärdad → verifierad och stängd, med avvisning för dubbletter och sådant som inte är fel, och återöppning när rättningen inte håller.

**I Qtestframe:** statusarna är *Öppen*, *Pågår*, *Åtgärdad*, *Stängd* och *Avvisad*; *Öppen* och *Pågår* räknas som öppna. En defekt som dyker upp igen efter att ha åtgärdats rapporteras som en regression.

**Se även:** [Statusar och allvarlighetsgrader](defects.md#statusar-och-allvarlighetsgrader), [Regressioner](defects.md#regressioner), [Defekt](#defekt)

### Delat steg

En följd av steg som många testfall använder, till exempel inloggning, skriven en gång och anropad från andra testfall.

**I Qtestframe:** ett teststeg kan anropa ett annat testfall i projektet; när en körning skapas ersätts anropet av det anropade testfallets steg, ifyllda med det anropande testfallets parametrar. En ändring i det anropade testfallet når alla som anropar det i nästa körning.

**Se även:** [Delade steg](tests.md#delade-steg), [Teststeg](#teststeg), [Datadriven testning](#datadriven-testning)

### Diskussion

En tråd med kommentarer på ett objekt, där man ställer frågor, föreslår ändringar och fattar beslut.

**I Qtestframe:** krav, testfall, körningar, defekter, risker och granskningsförfrågningar har diskussioner med Markdown, @omnämnanden, reaktioner och lösta trådar; projektets sida **Diskussioner** samlar trådarna.

**Se även:** [Diskussioner](discussions.md), [Markdown](#markdown), [Notis](#notis)

### Dubbelriktad spårbarhet

Spårbarhet åt båda hållen samtidigt: från varje krav till de testfall och den design som förverkligar det, och från varje testfall eller designelement tillbaka till dess krav. Många säkerhets- och kvalitetsstandarder kräver det, eftersom det visar både att inget saknas och att inget är överflödigt.

**I Qtestframe:** en [spårlänk](#spårlänk) sparas en gång och visas från båda håll: på kravet som **Länkade testfall**, på testfallet som **Länkade krav** och i [spårbarhetsmatrisen](#spårbarhetsmatris).

**Se även:** [Framåtspårbarhet](#framåtspårbarhet), [Bakåtspårbarhet](#bakåtspårbarhet), [Spårbarhet](#spårbarhet)

## E

### EARS

Easy Approach to Requirements Syntax: ett litet antal meningsmönster som gör krav mindre tvetydiga. Mönstren är *allmängiltiga* (Systemet ska …), *händelsestyrda* (När …, ska systemet …), *tillståndsstyrda* (Medan …, ska systemet …), *oönskat beteende* (Om …, så ska systemet …) och *valfri funktion* (Där …, ska systemet …), och kombinationer av dem (Mavin m.fl., 2009).

**I Qtestframe:** skriv kravtexten enligt ett EARS-mönster med ett kravord; [språkanalysen](#språkanalys) kontrollerar ändå vaga ord och reservationer.

**Se även:** [INCOSE-regler](#incose-regler), [Kravkvalitet](#kravkvalitet), [Standarder för kravhantering](references.md#standarder-och-vägledningar-för-kravhantering)

### Ekvivalensklassindelning

En testdesignteknik som delar in de möjliga värdena för en indata eller utdata i grupper (klasser) som systemet ska behandla lika, och sedan testar ett värde ur varje grupp. Klasser med ogiltiga värden testas en i taget, så att ett fel inte döljer ett annat (ISO/IEC/IEEE 29119-4).

**I Qtestframe:** en rad [testdata](#testdata) per klass håller ihop fallen i ett testfall; döp varje rad efter sin klass.

**Se även:** [Gränsvärdesanalys](#gränsvärdesanalys), [Testdesignteknik](#testdesignteknik), [Svartlådetest](#svartlådetest)

### End-to-end-test

Test av ett helt flöde genom systemet och dess grannsystem, så som en användare går igenom det: från inloggning via beställning till bekräftelsemejlet. End-to-end-test hittar integrationsproblem som smalare test missar men är långsammare och ömtåligare, så de hålls få.

**I Qtestframe:** end-to-end-test är oftast [webbtester](#webbtest) (Playwright) eller manuella testfall med många steg; använd [beroenden](tests.md#beroenden) för flöden som bygger på varandra.

**Se även:** [Systemtest](#systemtest), [Webbtest](#webbtest), [Instabilt testfall](#instabilt-testfall)

### Enhetstest

Testnivån för de minsta testbara delarna av programvaran, till exempel funktioner, klasser eller moduler, isolerade från resten. Enhetstest skrivs av utvecklarna, körs på sekunder och utgör basen i testpyramiden (kallas också komponenttest).

**I Qtestframe:** utfall från enhetstest kommer från CI som JUnit XML; ett testfall med en testnyckel kopplar dem till testfall och krav i Qtestframe.

**Se även:** [Testnivå](#testnivå), [Testdriven utveckling](#testdriven-utveckling), [Koppla automatiserade tester till testnycklar](runs.md#koppla-automatiserade-tester-till-testnycklar)

### ETag och If-Match

HTTP-huvuden för optimistisk samtidighetskontroll (RFC 9110). Servern skickar en versionsmärkning (`ETag`) med en resurs; en klient som ändrar resursen skickar tillbaka märkningen i `If-Match`, och servern avvisar ändringen med *412 Precondition Failed* om någon har sparat en nyare version under tiden. Ingens arbete skrivs över i tysthet, och inget låses.

**I Qtestframe:** ett objekts ETag är dess version. Webbklienten skickar den när du sparar, och därför får du se **Någon annan har ändrat det här** i stället för att en kollegas ändringar försvinner; OSLC- och API-klienter använder samma mekanism.

**Se även:** [När någon annan har sparat först](requirements.md#när-någon-annan-har-sparat-först), [HTTP-semantik](#http-semantik), [Koppla ihop med andra verktyg](../../admin-guide/sv/connecting-tools.md)

### Exekvering

Att köra ett testfall och registrera vad som hände: utfallet, utfallen per steg, iakttagelser, bevis och eventuella defekter.

**I Qtestframe:** en exekvering är ett testfall i en [körning](#testkörning) (per konfiguration), med utfall, utfall per steg, iterationer, anteckningar, bilagor, defekter och tidsåtgång. Stegen kopieras när testfallet läggs till, så senare ändringar i testfallet påverkar inte registrerade utfall.

**Se även:** [Exekveringsvyn](runs.md#exekveringsvyn), [Testkörning](#testkörning), [Testutfall](#testutfall)

## F

### Felgissning

En testdesignteknik där testaren utgår från erfarenhet av vanliga misstag när testfallen väljs: tom indata, mycket lång indata, specialtecken, tidszoner, dubbelklick, avbrutna anslutningar. Den fungerar bäst som komplement till systematiska tekniker, gärna styrd av en lista över tidigare defekter.

**I Qtestframe:** tidigare defekter (filtret `found_in:production`) och [utforskande sessioner](#utforskande-testning) är bra källor; samla de testfall det leder till i en egen kategori.

**Se även:** [Utforskande testning](#utforskande-testning), [Testdesignteknik](#testdesignteknik), [Defekt](#defekt)

### Framåtspårbarhet

Spårning från ett krav till allt som förverkligar och verifierar det: mer detaljerade krav, design, kod, testfall och utfall. Den svarar på ”är varje krav genomfört och testat?” och ”vad påverkas om det här ändras?”.

**I Qtestframe:** kravets **Länkade testfall**, dess barn (*förfinas av*), kodlänkar och täckningskolumnen visar framåtspårbarheten; `-has:tests` hittar krav som saknar testfall.

**Se även:** [Bakåtspårbarhet](#bakåtspårbarhet), [Ändringspåverkan](#ändringspåverkan), [Spårbarhetsmatris](#spårbarhetsmatris)

### Frågespråk

Ett litet textspråk för att filtrera listor.

**I Qtestframe:** samma språk filtrerar alla listor, spårbarhetsmatrisen, releasers omfattning, rapporter och API:et: `status:approved priority=high`, `-has:tests`, `updated_at>=2026-01-01`, `title~/login/i`. En ogiltig fråga visar var felet finns.

**Se även:** [Sökning och filter](search-and-filters.md), [Frågespråket](how-it-works.md#frågespråket)

### Funktionell testning

Test av vad systemet gör mot de funktionella kraven, oavsett hur det är byggt. Den kan göras på alla [testnivåer](#testnivå) och är oftast [svartlådetest](#svartlådetest).

**I Qtestframe:** funktionella testfall är de manuella och automatiserade testfall som är länkade till funktionella krav; filtrera fram dem med `linked:` och kravtypen eller med en kategori.

**Se även:** [Icke-funktionell testning](#icke-funktionell-testning), [Testtyp](#testtyp), [Funktionellt krav](#funktionellt-krav)

### Funktionellt krav

Ett krav som säger vad systemet gör: en funktion, ett beteende, en reaktion på en indata, en beräkning. ”Systemet ska skicka en orderbekräftelse med e-post” är funktionellt; hur snabbt det sker är [icke-funktionellt](#icke-funktionellt-krav).

**I Qtestframe:** välj typen **Funktionellt** (standardvalet). Funktionella krav verifieras oftast med testfall.

**Se även:** [Icke-funktionellt krav](#icke-funktionellt-krav), [Funktionell testning](#funktionell-testning), [Fält](requirements.md#fält)

### Förfining

Att bryta ned ett krav i mer detaljerade krav som tillsammans uppfyller det: ett intressentbehov i systemkrav, ett systemkrav i programvarukrav. De detaljerade kraven förfinar det övergripande.

**I Qtestframe:** relationen **förfinar (förälder)** / **förfinas av (barn)** bygger kravhierarkin; förfiningar får inte bilda en cykel, och en förälder är verifierad först när dess barn är det.

**Se även:** [Kravhierarki och sammanräkning](traceability-and-coverage.md#kravhierarki-och-sammanräkning), [Härlett krav](#härlett-krav), [Relationer](requirements.md#relationer)

## G

### Granskning

En kontroll av en arbetsprodukt som görs av andra än författaren, från en informell genomläsning till en formell inspektion med roller och protokoll. I Qtestframe betyder granskning också beslutet att godkänna ett objekt.

**I Qtestframe:** begär granskning av krav eller testfall; granskare godkänner eller avvisar krav (`requirement.review`) och markerar testfall som klara (`test.review`), och varje beslut hamnar i händelseloggen. *Granskning* är också en av [verifieringsmetoderna](#verifieringsmetod).

**Se även:** [Begär granskning](reviews-and-notifications.md#begär-granskning), [Roll](#roll), [Verifieringsmetod](#verifieringsmetod)

### Grålådetest

Test med delvis kännedom om det inre: testaren tar fram svartlådetest men använder kunskap om arkitekturen, databasen eller gränssnitten för att välja dem bättre, till exempel för att kontrollera att en order sparas rätt efter att den lagts i användargränssnittet.

**I Qtestframe:** grålådetest kombinerar ofta manuella steg med en kontroll av loggar eller data; registrera vad som kontrollerades som anteckningar och bevis i exekveringen.

**Se även:** [Svartlådetest](#svartlådetest), [Vitlådetest](#vitlådetest), [Bevis](runs.md#bevis)

### Gränsvärdesanalys

En testdesignteknik som lägger testvärden vid kanterna av [ekvivalensklasserna](#ekvivalensklassindelning), eftersom fel samlas där: ”mindre än” i stället för ”mindre än eller lika med”, slingor som går ett varv för mycket. För ett fält som tar emot 1 till 100 testar du 0, 1, 100 och 101 (och med tre värden per gräns även 2 och 99).

**I Qtestframe:** lägg gränsvärdena i [testdatan](#testdata) för ett datadrivet testfall, en rad per värde, så att varje värde blir en egen iteration med eget utfall.

**Se även:** [Ekvivalensklassindelning](#ekvivalensklassindelning), [Datadriven testning](#datadriven-testning), [Datadrivna testfall](tests.md#datadrivna-testfall)

## H

### HOTP

HMAC-based One-Time Password (RFC 4226): en kod som beräknas från en delad hemlighet och en räknare. [TOTP](#totp) är HOTP med det aktuella tidssteget som räknare.

**I Qtestframe:** används bara inuti TOTP; längden på den sparade hemligheten följer rekommendationen i RFC 4226.

**Se även:** [TOTP](#totp), [Tvåstegsverifiering](#tvåstegsverifiering), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### HTTP-semantik

RFC 9110 definierar vad HTTP-metoder, statuskoder och huvuden betyder, oberoende av HTTP-version: att GET bara läser, att PUT ersätter, vad 404 eller 412 betyder, och villkorade begäranden med [ETag och If-Match](#etag-och-if-match).

**I Qtestframe:** REST-API:et, OSLC och SCIM följer den; felsvar har en statuskod och en maskinläsbar felkod.

**Se även:** [ETag och If-Match](#etag-och-if-match), [OpenAPI](#openapi), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### Händelselogg

En spårbar logg över vem som gjorde vad och när, som sparas för säkerhetsgenomgångar och revisioner. Till skillnad från ett objekts versionshistorik registrerar den också inloggningar, ändrade behörigheter, exporter och administrativa åtgärder.

**I Qtestframe:** organisationens händelselogg finns under **Administration → Händelselogg** (kräver `audit.read`); varje granskningsbeslut, signerat undantag, import och ändrad inställning registreras där. Administratörer kan också skicka den till en central [syslog](#syslog)server.

**Se även:** [Händelselogg](administration.md#händelselogg), [Version](#version), [Syslog](#syslog)

### Härlett krav

Ett krav som ingen intressent har ställt men som följer av ett annat krav eller av ett designbeslut. En svarstid för hela systemet leder till exempel till härledda tidsbudgetar för varje komponent.

**I Qtestframe:** använd relationen **härlett från** / **källa till**. En ändring av källan gör det härledda kravet misstänkt, så att det granskas.

**Se även:** [Förfining](#förfining), [Relationer](requirements.md#relationer), [Ändringspåverkan](#ändringspåverkan)

## I

### Icke-funktionell testning

Test av kvalitetsegenskaper snarare än funktioner: prestanda, last, säkerhet, användbarhet, tillgänglighet, tillförlitlighet, kompatibilitet. Den kräver ofta särskilda verktyg, miljöer och mätningar.

**I Qtestframe:** icke-funktionella testfall länkas till icke-funktionella krav som alla andra testfall; analyser och mätningar som inte är exekveringar registreras som [verifieringsposter](#verifieringspost).

**Se även:** [Funktionell testning](#funktionell-testning), [ISO/IEC 25010](#isoiec-25010), [Testtyp](#testtyp)

### Icke-funktionellt krav

Ett krav på hur väl systemet gör det det gör, eller under vilka förhållanden: prestanda, säkerhet, användbarhet, tillförlitlighet, underhållbarhet, flyttbarhet. Icke-funktionella krav behöver mätbara gränser, annars går de inte att verifiera (”snabbt” är inget krav; ”p95 under 2 s” är det).

**I Qtestframe:** välj typen **Icke-funktionellt** eller den mer specifika **Säkerhet**, **Prestanda** eller **Användbarhet**, och lägg den mätbara gränsen i ett [utvärderingskriterium](#utvärderingskriterium).

**Se även:** [Kvalitetsegenskap](#kvalitetsegenskap), [Funktionellt krav](#funktionellt-krav), [Icke-funktionell testning](#icke-funktionell-testning)

### Inaktuellt utfall

Ett testutfall som kanske inte längre duger som bevis: det är för gammalt, eller testfallet eller ett krav som det verifierar har ändrats efter att utfallet registrerades.

**I Qtestframe:** inaktuella utfall har märket **Inaktuellt**, räknas som ej körda i täckningen och verifierar inte ett krav; kör testfallet igen. Åldersgränsen och regeln för ändringar är projektinställningar.

**Se även:** [Inaktuella utfall](traceability-and-coverage.md#inaktuella-utfall), [Ändringspåverkan](#ändringspåverkan), [Misstänkt länk](#misstänkt-länk)

### INCOSE-regler

*Guide to Writing Requirements* från International Council on Systems Engineering: regler för välformade krav, som aktiv form med tydligt subjekt, en tanke per krav, inga vaga ord, inga reservationer, inga öppna uppräkningar och angivna enheter och toleranser.

**I Qtestframe:** [språkanalysen](#språkanalys) kontrollerar flera av reglerna (tvetydiga ord, reservationer, flera kravord i en mening, passiv förmåga, platshållare) och sätter ett poängtal på texten.

**Se även:** [Kravkvalitet](#kravkvalitet), [EARS](#ears), [Språkanalysen](requirements.md#språkanalysen)

### Ingångskriterier

De villkor som ska vara uppfyllda innan en testaktivitet får börja: testunderlaget är godkänt, miljön och datan är klara, bygget har klarat sitt röktest. De hindrar att man lägger tid på att testa något som ännu inte kan bli godkänt (ISO/IEC/IEEE 29119-3).

**I Qtestframe:** organisationens exekveringsregler är ingångskriterier per testfall: testfallet ska vara *Klart*, dess krav godkända och dess förutsättningar godkända. De står som ingångskriterier i en framtagen testplan.

**Se även:** [När kan ett testfall exekveras?](runs.md#när-kan-ett-testfall-exekveras), [Utgångskriterier](#utgångskriterier), [Testplan](#testplan)

### Instabilt testfall

Ett testfall vars utfall växlar mellan godkänt och underkänt utan att testfallet eller produkten har ändrats, på grund av tidsberoenden, delad testdata, ordningsberoenden eller en ostabil miljö (på engelska *flaky*). Instabila testfall döljer verkliga fel och undergräver förtroendet för hela testsviten.

**I Qtestframe:** ett testfall med två eller fler växlingar bland sina tio senaste godkända eller underkända utfall får märket **Instabilt**; `flaky:true` och `flakiness>=3` hittar dem, och översikten listar de mest instabila.

**Se även:** [Instabila testfall](tests.md#instabila-testfall), [Kontinuerlig integration](#kontinuerlig-integration), [Testutfall](#testutfall)

### Integrationstest

Testnivån som kontrollerar samspelet mellan komponenter eller system: gränssnitt, data som skickas mellan dem, protokoll och tidsförhållanden. Komponentintegrationstest tittar inom ett system, systemintegrationstest på kopplingarna till andra system.

**I Qtestframe:** länka integrationstest till krav av typen *Gränssnitt* och allokera båda till [systemelement](#systemelement) för att se täckningen per komponent.

**Se även:** [Testnivå](#testnivå), [Enhetstest](#enhetstest), [Systemtest](#systemtest)

### Intressent

En person eller organisation som har ett intresse i systemet eller påverkas av det: kunder, användare, driftpersonal, tillsynsmyndigheter, utvecklingsteamet.

**I Qtestframe:** ange vem som ställde ett krav i **Källa** (till exempel *Intressent* eller *Kundspec. §4.2*) och vem som ansvarar för det i **Ägare**.

**Se även:** [Intressentkrav](#intressentkrav), [Fält](requirements.md#fält), [Krav](#krav)

### Intressentkrav

Ett krav så som en intressent uttrycker det, på intressentens språk: vad hen behöver av systemet och under vilka förhållanden. Intressentkrav valideras med intressenterna och omvandlas till [systemkrav](#systemkrav) (ISO/IEC/IEEE 29148).

**I Qtestframe:** håll intressentkraven i en egen mapp eller kategori och förfina dem till systemkrav; deras verifiering räknas samman från barnen.

**Se även:** [Användarkrav](#användarkrav), [Verksamhetskrav](#verksamhetskrav), [Validering](#validering)

### ISO/IEC 25010

Produktkvalitetsmodellen i SQuaRE-serien. Den delar in programvarukvalitet i egenskaper som funktionell lämplighet, prestandaeffektivitet, kompatibilitet, interaktionsförmåga (användbarhet), tillförlitlighet, säkerhet, underhållbarhet, flexibilitet (flyttbarhet) och skydd mot skada (safety), var och en med underegenskaper.

**I Qtestframe:** använd egenskaperna som checklista för [icke-funktionella krav](#icke-funktionellt-krav) och som kategoridimension (till exempel *Kvalitet / Säkerhet*); kravtyperna täcker de vanligaste.

**Se även:** [Kvalitetsegenskap](#kvalitetsegenskap), [Icke-funktionell testning](#icke-funktionell-testning), [Standarder för kravhantering](references.md#standarder-och-vägledningar-för-kravhantering)

### ISO/IEC 27001 bilaga A

ISO/IEC 27001 specificerar ett ledningssystem för informationssäkerhet. Bilaga A förtecknar 93 säkerhetsåtgärder i fyra teman (organisatoriska, personrelaterade, fysiska och tekniska; utgåvan 2022). Organisationen avgör vilka åtgärder som är tillämpliga och dokumenterar det i ett [uttalande om tillämplighet](#uttalande-om-tillämplighet).

**I Qtestframe:** sidan **SoA** listar åtgärderna i bilaga A med tillämplighet, motivering, ägare, de krav som hänvisar till varje åtgärd och deras bevis; katalogen har krav för ISO/IEC 27001 att lägga till.

**Se även:** [Uttalande om tillämplighet](waivers-and-soa.md#uttalande-om-tillämplighet), [Katalog](#katalog), [Källor som katalogen hänvisar till](references.md#källor-som-katalogen-hänvisar-till)

### ISO/IEC/IEEE 29119

Den internationella standardserien för programvarutest: begrepp och terminologi (del 1), testprocesser (del 2), testdokumentation som testplaner, testfallsspecifikationer och testrapporter (del 3) och testdesigntekniker (del 4).

**I Qtestframe:** den framtagna testplanen följer strukturen i del 3, och den här ordlistan använder begreppen i del 1 och 4.

**Se även:** [ISTQB](#istqb), [Testplan](#testplan), [Standarder och vägledningar för test](references.md#standarder-och-vägledningar-för-test)

### ISO/IEC/IEEE 29148

Den internationella standarden för kravhantering: processer, informationsobjekt (specifikationer av intressent-, system- och programvarukrav) och egenskaperna hos bra enskilda krav och kravmängder.

**I Qtestframe:** den framtagna kravspecifikationen bygger på den, och uppslagsordet [Kravkvalitet](#kravkvalitet) sammanfattar dess egenskaper.

**Se även:** [Kravkvalitet](#kravkvalitet), [Specifikation](#specifikation), [Standarder för kravhantering](references.md#standarder-och-vägledningar-för-kravhantering)

### ISTQB

International Software Testing Qualifications Board ger ut kursplanerna för certifierade testare och ISTQB:s ordlista, den mest använda terminologin inom test.

**I Qtestframe:** handboken använder ISTQB:s begrepp (testfall, testkörning, defekt, täckning …) där användargränssnittet gör det.

**Se även:** [ISO/IEC/IEEE 29119](#isoiecieee-29119), [Standarder och vägledningar för test](references.md#standarder-och-vägledningar-för-test)

### Iteration

Ett genomlopp av ett datadrivet testfall med en rad testdata.

**I Qtestframe:** varje rad i testfallets testdata blir en iteration av exekveringen, med egna utfall per steg; exekveringens utfall härleds från iterationerna.

**Se även:** [Datadrivna testfall: iterationer](runs.md#datadrivna-testfall-iterationer), [Datadriven testning](#datadriven-testning), [Exekvering](#exekvering)

## J

### JSON Web Token

JWT (RFC 7519): en kompakt, signerad samling påståenden, till exempel vem en användare är, vem som utfärdade token och när den går ut, som mottagaren kan kontrollera utan att fråga utfärdaren.

**I Qtestframe:** id-token vid enkel inloggning med [OpenID Connect](#openid-connect) är en JWT; Qtestframe kontrollerar signatur, utfärdare, mottagare, giltighetstid och nonce. API-nycklar är inte JWT utan slumpmässiga strängar.

**Se även:** [OpenID Connect](#openid-connect), [OAuth 2.0](#oauth-20), [API-nyckel](#api-nyckel)

### JUnit XML

Ett XML-format för testutfall, uppkallat efter Java-ramverket JUnit, med testsviter, testfall, tidsåtgång, fel och överhoppade testfall. Det finns ingen formell standard, men nästan alla testverktyg kan skriva formatet, vilket gör det till det gemensamma språket mellan CI och testhantering.

**I Qtestframe:** ladda upp JUnit XML till en körning (`POST /api/v1/runs/{id}/junit`) eller med [`qtestframe report`](#qtestframe-report); testfallen matchas mot testfall i Qtestframe på testnyckeln.

**Se även:** [JUnit XML](runs.md#junit-xml), [Kontinuerlig integration](#kontinuerlig-integration), [Format, protokoll och ramverk](references.md#format-protokoll-och-ramverk)

## K

### Katalog

Ett bibliotek med färdiga krav, testfall och risker för vanliga behov, som autentisering, loggning, dataskydd eller tillgänglighet, med hänvisningar till de lagar, standarder och den bästa praxis de bygger på.

**I Qtestframe:** i [katalogen](catalogue.md) väljer du de lagar och standarder som ett projekt ska uppfylla och lägger till de krav som hör till, med testfall och spårlänkar. Den kopplar också risker till MITRE ATT&CK, ATLAS, D3FEND och EMB3D.

**Se även:** [Katalog](catalogue.md), [MITRE ATT&CK](#mitre-attck), [Uttalande om tillämplighet](#uttalande-om-tillämplighet)

### Kodlänk

En länk mellan ett objekt och en ändring i ett versionshanteringssystem: en incheckning (commit) eller en merge request eller pull request.

**I Qtestframe:** incheckningar och merge requests som nämner en objektnyckel (`SHOP-REQ-12`) länkas automatiskt via webhooks från GitHub och GitLab, eller för hand under fliken **Kod**. Byggen som CI laddar upp visar vilka incheckningar som testades, och regressionsurvalet använder länkarna.

**Se även:** [Kodlänkar](variants-and-code.md#kodlänkar), [Kontinuerlig integration](#kontinuerlig-integration), [Regressionstest](#regressionstest)

### Kodtäckning

Andelen av koden som testerna exekverar, mätt som satser, grenar, villkor eller vägar. Den visar otestad kod, men hög kodtäckning säger i sig inget om huruvida kraven är uppfyllda: kod som gör fel sak kan vara helt täckt.

**I Qtestframe:** kodtäckning mäts av dina byggverktyg, inte av Qtestframe. Qtestframe mäter [kravtäckning](#kravtäckning) och kopplar kodändringar och byggen till krav och testfall med [kodlänkar](#kodlänk).

**Se även:** [Kravtäckning](#kravtäckning), [Vitlådetest](#vitlådetest), [Täckning](#täckning)

### Konsistenskontroll

En automatisk kontroll av problem som berör många objekt: krav utan testfall, godkända krav med ej godkända föräldrar, misstänkta dubbletter, möjligen motstridiga krav.

**I Qtestframe:** inbyggda kontroller och dina egna [spårbarhetsregler](#spårbarhetsregel) körs vid behov och i kravanalysen; varje avvikelse pekar ut objektet och föreslår åtgärder som **Markera som dubblett** eller **Inget problem**.

**Se även:** [Inbyggda konsistenskontroller](traceability-and-coverage.md#inbyggda-konsistenskontroller), [Dubbletter och konflikter](duplicates-and-conflicts.md), [Spårbarhetsregel](#spårbarhetsregel)

### Kontinuerlig integration

CI: varje ändring slås ofta ihop med en gemensam gren och byggs och testas automatiskt, så att problem syns inom några minuter. Kontinuerlig leverans och driftsättning (CD) förlänger kedjan ända fram till en produkt som kan släppas eller är släppt. Testning i CI/CD bygger på snabba, pålitliga automatiserade tester på flera [testnivåer](#testnivå).

**I Qtestframe:** CI-pipelines laddar upp sina utfall med [`qtestframe report`](#qtestframe-report), som [JUnit XML](#junit-xml) eller med Playwright-rapportören. Utfallen hamnar i en körning, räknas in i täckningen och releasens utgångskriterier och registrerar bygget.

**Se även:** [Importera utfall från CI](runs.md#importera-utfall-från-ci), [Shift-left](#shift-left), [Byggen](variants-and-code.md#byggen)

### Krav

Ett påstående om ett behov, en förmåga eller ett villkor som ett system ska uppfylla, formulerat så att man kan enas om det och verifiera det. Ett krav säger vad som behövs, inte hur det ska byggas, och har egenskaper som prioritet, källa och motivering.

**I Qtestframe:** ett krav har nyckel, titel, beskrivning, [kravnivå](#kravnivå), typ, status, prioritet, ägare, källa, motivering, [utvärderingskriterier](#utvärderingskriterium), relationer och versionshistorik; täckningen kommer från de länkade testfallen.

**Se även:** [Krav](requirements.md), [Specifikation](#specifikation), [Kravkvalitet](#kravkvalitet)

### Kravkvalitet

De egenskaper som gör ett krav bra. ISO/IEC/IEEE 29148 kräver att varje krav är nödvändigt, lämpligt (på rätt nivå), entydigt, fullständigt, odelat, genomförbart, verifierbart, korrekt och följer gällande form, och att en kravmängd är fullständig, konsistent, genomförbar, begriplig och möjlig att validera.

**I Qtestframe:** [språkanalysen](#språkanalys) sätter poäng på varje text, konsistenskontrollerna hittar dubbletter, konflikter och luckor, och kravanalysen visar kvalitetsmått för hela projektet.

**Se även:** [INCOSE-regler](#incose-regler), [Kravanalys](traceability-and-coverage.md#kravanalys), [ISO/IEC/IEEE 29148](#isoiecieee-29148)

### Kravnivå

Hur bindande ett krav är: obligatoriskt, rekommenderat, valfritt eller enbart beskrivande. Nivån uttrycks med kravord som SKA, BÖR och FÅR (MUST, SHOULD, MAY) och avgör hur kravet vägs in i beslut om en release.

**I Qtestframe:** nivåerna är MUST och SHALL (SKA), MUST NOT och SHALL NOT (SKA INTE), SHOULD (BÖR), SHOULD NOT (BÖR INTE), MAY (FÅR), OPTIONAL (VALFRITT), WILL (KOMMER ATT) och *Beskrivande*. De känns igen i texten när du skapar ett krav, markeras i beskrivningar, och utgångskriteriet *Obligatoriska krav klarade* kontrollerar alla krav på nivån SKA.

**Se även:** [Kravnivåer](requirements.md#kravnivåer), [Kravord](#kravord), [SKA, BÖR och FÅR](#ska-bör-och-får)

### Kravord

Orden som anger hur bindande en mening är. RFC 2119 definierar de engelska MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, RECOMMENDED, NOT RECOMMENDED, MAY och OPTIONAL; RFC 8174 tillägger att bara versalformerna har den betydelsen. Tillsammans utgör de BCP 14. Svenska motsvarigheter beskrivs under [SKA, BÖR och FÅR](#ska-bör-och-får).

**I Qtestframe:** kravorden bestämmer [kravnivån](#kravnivå) och markeras i texten; språkanalysen varnar för engelska kravord med gemener och för flera kravord i samma mening.

**Se även:** [Kravord (RFC)](references.md#kravord-rfc), [SKA, BÖR och FÅR](#ska-bör-och-får), [Kravnivå](#kravnivå)

### Kravtäckning

Den andel av kraven som testerna når, och hur långt utfallen visar att kraven är uppfyllda. Det är testhanteringens viktigaste täckningsmått, eftersom det knyter testningen till vad produkten ska göra.

**I Qtestframe:** varje krav har ett täckningsläge (*Ingen täckning*, *Ej körd*, *Godkänd*, *Underkänd*, *Delvis*) från de senaste utfallen för dess testfall, per konfiguration; översikten och releasekriterierna *Täckning* och *Klarade testfall* räknar samman dem.

**Se även:** [Så beräknas täckningen](traceability-and-coverage.md#så-beräknas-täckningen), [Täckning](#täckning), [Kodtäckning](#kodtäckning)

### Kryptering av hemligheter

Skydd av sparade lösenord, token och nycklar som servern själv måste kunna läsa.

**I Qtestframe:** TOTP-hemligheter, webhookhemligheter, token för ärendesystem, hemliga testvariabler och liknande krypteras i databasen med en nyckel som aldrig sparas där: en nyckelfil, en miljövariabel, ett TPM 2.0 eller en HSM. Hemligheter går bara att skriva: API:et returnerar dem aldrig.

**Se även:** [Kryptering av lagrade hemligheter](administration.md#kryptering-av-lagrade-hemligheter), [Säkerhetskopia](#säkerhetskopia), [Säkerhet](how-it-works.md#säkerhet)

### Kvalitetsegenskap

En mätbar egenskap hos ett system utöver dess funktioner, till exempel svarstid, tillgänglighet (drifttid), lärbarhet eller motståndskraft mot angrepp. [ISO/IEC 25010](#isoiec-25010) ordnar dem i en kvalitetsmodell.

**I Qtestframe:** kvalitetsegenskaper anges som icke-funktionella krav med utvärderingskriterium och tröskelvärde; en kategoridimension som *Kvalitet* grupperar dem.

**Se även:** [Icke-funktionellt krav](#icke-funktionellt-krav), [ISO/IEC 25010](#isoiec-25010), [Utvärderingskriterium](#utvärderingskriterium)

## L

### Landlock

En säkerhetsmodul i Linux med vilken ett program begränsar sin egen åtkomst till filer: när det har öppnat det det behöver anger det vilka kataloger det fortfarande får läsa och skriva, och kärnan vägrar allt annat, även om någon senare tar över programmet.

**I Qtestframe:** på Linux begränsar servern sig själv med Landlock efter starten, och varje skript i sandlådan körs med en Landlock-regeluppsättning som bara tillåter skrivning i dess jobbkatalog.

**Se även:** [seccomp](#seccomp), [Skriptkörning](#skriptkörning), [Härdning](../../admin-guide/sv/security.md#härdning)

### Lasttest

En sorts [prestandatest](#prestandatest) som kontrollerar beteendet under den förväntade belastningen, till exempel antalet användare eller transaktioner vid högsta trafik, för att bekräfta att svarstider och genomströmning uppfyller kraven.

**I Qtestframe:** ange belastningen och gränserna i utvärderingskriteriet (metod *Analys* eller *Test*, tröskelvärde *p95 < 2 s vid 500 användare*) och bifoga lasttestrapporten som bevis.

**Se även:** [Prestandatest](#prestandatest), [Stresstest](#stresstest), [Utvärderingskriterium](#utvärderingskriterium)

## M

### Markdown

Ett enkelt textformat där `**fetstil**`, listor, länkar och tabeller skrivs med vanliga tecken. CommonMark är en exakt specifikation av Markdown; GitHub Flavored Markdown lägger till tabeller med mera.

**I Qtestframe:** kommentarer i diskussioner skrivs i Markdown, den här handboken är skriven i Markdown, och exporten kan ta fram ett Markdown-dokument; beroendegrafen kan sparas som Markdown med ett Mermaid-diagram.

**Se även:** [Diskussion](#diskussion), [Export](reports-and-export.md#export), [Format, protokoll och ramverk](references.md#format-protokoll-och-ramverk)

### Misstänkt länk

En spårlänk eller relation där den ena änden har ändrats i sak efter att länken skapades eller senast bekräftades, så att den andra änden kanske inte längre passar. En misstänkt länk ska granskas och sedan bekräftas, eller så uppdateras objektet.

**I Qtestframe:** länkar och relationer blir misstänkta automatiskt när ett kravs titel, beskrivning, kravnivå eller kriterier, eller ett testfalls innehåll, ändras; de har en gul ram i matrisen, syns i arbetslistorna, och utgångskriteriet *Inga misstänkta länkar* kontrollerar dem.

**Se även:** [Ändringspåverkan](#ändringspåverkan), [Spårbarhetsmatrisen](traceability-and-coverage.md#spårbarhetsmatrisen), [Inaktuellt utfall](#inaktuellt-utfall)

### MITRE ATLAS

Adversarial Threat Landscape for Artificial-Intelligence Systems: en kunskapsbas över taktiker och tekniker för angrepp mot AI- och maskininlärningssystem, till exempel promptinjektion eller förgiftning av träningsdata, uppbyggd som ATT&CK.

**I Qtestframe:** AI-risker i katalogen anger de ATLAS-tekniker som förverkligar dem (`AML.T0051`), bredvid ATT&CK-teknikerna.

**Se även:** [MITRE ATT&CK](#mitre-attck), [AI-förordningen](#ai-förordningen), [MITRE ATT&CK i katalogen](catalogue.md#mitre-attck)

### MITRE ATT&CK

En kunskapsbas över hur verkliga angripare arbetar: taktiker (deras mål, som initial åtkomst eller åtkomst till inloggningsuppgifter) och tekniker (hur de når dem, som *T1110 Brute Force*), i matriser för företags-IT, industriella styrsystem och mobila enheter.

**I Qtestframe:** risker i katalogen som orsakas av en angripare anger sina ATT&CK-tekniker; filtrera på taktik eller teknik, se ramverkstäckningen och behåll teknikerna på risker som du lägger till i ett projekt.

**Se även:** [MITRE D3FEND](#mitre-d3fend), [STIX](#stix), [MITRE ATT&CK i katalogen](catalogue.md#mitre-attck)

### MITRE D3FEND

En kunskapsgraf över defensiva tekniker inom cybersäkerhet, grupperade efter taktiker som *Harden*, *Detect* och *Isolate* och kopplade till de ATT&CK-tekniker de motverkar.

**I Qtestframe:** krav och testfall anger de D3FEND-tekniker de genomför eller verifierar (fältet **MITRE D3FEND-tekniker**, frågan `d3fend:`); för en risk visar katalogen de försvar som motverkar dess tekniker och markerar luckor.

**Se även:** [MITRE ATT&CK](#mitre-attck), [MITRE EMB3D](#mitre-emb3d), [MITRE D3FEND och EMB3D](catalogue.md#mitre-d3fend-och-emb3d)

### MITRE EMB3D

En hotmodell för inbyggda enheter: hot (*TID-…*) som otillräckligt skydd av startladdaren och motåtgärder (*MID-…*) som säker uppdatering av fast programvara.

**I Qtestframe:** krav och testfall för enheter och IoT anger de EMB3D-motåtgärder och hot de hanterar (frågan `emb3d:`), och katalogen visar vilka hot inget krav hanterar.

**Se även:** [MITRE D3FEND](#mitre-d3fend), [STIX](#stix), [MITRE D3FEND och EMB3D](catalogue.md#mitre-d3fend-och-emb3d)

## N

### Notis

Ett meddelande om att något har hänt som berör dig: en granskningsförfrågan, ett omnämnande, en tilldelning, ett utfall.

**I Qtestframe:** notiserna visas i appen och, om du vill, med e-post direkt eller som en daglig sammanfattning. Projekt kan också skicka händelser till chattkanaler (Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix, Discord) och till [webhooks](#webhook).

**Se även:** [Notiser](reviews-and-notifications.md#notiser), [Chattaviseringar](reviews-and-notifications.md#chattaviseringar), [Webhook](#webhook)

### Nyckel

En kort, läsbar och unik identifierare.

**I Qtestframe:** varje projekt har en nyckel som `SHOP`, och varje objekt en nyckel som bygger på den: `SHOP-REQ-12`, `SHOP-TC-7`, `SHOP-RISK-3`. Nycklar återanvänds aldrig inom ett projekt. Nämn en nyckel i ett incheckningsmeddelande, en beskrivning eller ett testnamn i CI för att länka till objektet.

**Se även:** [Projekt](#projekt), [Kodlänk](#kodlänk), [Viktiga begrepp](index.md#viktiga-begrepp)

## O

### OAuth 2.0

Ramverket för auktorisering i RFC 6749: ett program får en åtkomsttoken från en auktoriseringsserver för en användares räkning, utan att se användarens lösenord. Det är grunden för [OpenID Connect](#openid-connect).

**I Qtestframe:** används för enkel inloggning med OpenID Connect, med auktoriseringskodflödet och [PKCE](#pkce). Qtestframe är själv ingen OAuth-auktoriseringsserver; API:et använder [API-nycklar](#api-nyckel).

**Se även:** [OpenID Connect](#openid-connect), [PKCE](#pkce), [Bearer-token](#bearer-token)

### OpenAPI

Ett maskinläsbart beskrivningsformat för HTTP-API:er: slutpunkter, parametrar, kroppar i begäranden och svar, autentisering. Verktyg tar fram dokumentation, klienter och tester ur beskrivningen.

**I Qtestframe:** servern publicerar REST-API:et som OpenAPI 3.1 på `/api/v1/openapi.json`, utan inloggning, så att du kan generera en klient för dina skript.

**Se även:** [HTTP-semantik](#http-semantik), [API-nyckel](#api-nyckel), [Översikt](../../admin-guide/sv/overview.md)

### OpenID Connect

OIDC: ett identitetslager ovanpå [OAuth 2.0](#oauth-20). När användaren har loggat in hos identitetsleverantören (till exempel Microsoft Entra ID, Okta eller Keycloak) får programmet en id-token ([JWT](#json-web-token)) som talar om vem användaren är.

**I Qtestframe:** enkel inloggning med en eller flera OIDC-leverantörer, med PKCE och nonce; användare kan skapas vid första inloggningen och få roller utifrån grupper.

**Se även:** [Enkel inloggning (OIDC)](administration.md#enkel-inloggning-oidc), [SAML 2.0](#saml-20), [OpenID Connect](../../admin-guide/sv/authentication.md#openid-connect)

### Organisation

En isolerad arbetsyta i en delad installation (på engelska *tenant*). Organisationer ser inte varandras data.

**I Qtestframe:** projekt, medlemmar, roller, API-nycklar, inställningar och händelseloggen hör till en organisation. Du kan vara medlem i flera och byta mellan dem; din roll kan skilja sig mellan dem.

**Se även:** [Organisationer, projekt och roller](how-it-works.md#organisationer-projekt-och-roller), [Roll](#roll), [Projekt](#projekt)

### OSLC

Open Services for Lifecycle Collaboration: en öppen OASIS-standard för att länka objekt mellan livscykelverktyg över HTTP, med domäner för kravhantering (RM) och kvalitetshantering (QM). Verktygen upptäcker varandras tjänster, väljer eller skapar objekt i små dialogrutor och sparar länkar i stället för kopior.

**I Qtestframe:** Qtestframe är OSLC-leverantör för RM 2.1 (krav) och QM 2.1 (testfall): DOORS Next, Polarion, Codebeamer och andra verktyg kan välja, skapa, läsa och ändra objekt och länka till dem.

**Se även:** [OSLC](interoperability.md#oslc), [ReqIF](#reqif), [Koppla ihop med andra verktyg](../../admin-guide/sv/connecting-tools.md)

## P

### Papperskorg

Där borttagna objekt väntar innan de tas bort för gott.

**I Qtestframe:** borttagna objekt kan återställas från papperskorgen i 30 dagar; sedan rensar servern bort dem.

**Se även:** [Hur ångrar jag en ändring?](faq.md#hur-ångrar-jag-en-ändring), [Version](#version), [Bakgrundsjobb](#bakgrundsjobb)

### Parvis testning

En kombinatorisk testdesignteknik: i stället för att testa varje kombination av parametervärden väljer man en mycket mindre mängd testfall där varje par av värden förekommer minst en gång. De flesta samspelsfel beror på två parametrar, så få testfall hittar de flesta (NIST SP 800-142).

**I Qtestframe:** ta fram de parvisa kombinationerna med ett verktyg och klistra in dem som [testdata](#testdata), eller använd dem för att välja [testkonfigurationerna](#testkonfiguration) i en körning.

**Se även:** [Testkonfiguration](#testkonfiguration), [Beslutstabelltest](#beslutstabelltest), [NIST-publikationer](references.md#nist-publikationer)

### PKCE

Proof Key for Code Exchange (RFC 7636): ett tillägg till auktoriseringskodflödet i OAuth. Klienten skickar ett hashat slumpvärde med inloggningsbegäran och själva värdet när koden växlas in, så att en uppsnappad kod är värdelös för någon annan.

**I Qtestframe:** inloggning med OIDC använder alltid PKCE med metoden S256, och dessutom en nonce i id-token.

**Se även:** [OAuth 2.0](#oauth-20), [OpenID Connect](#openid-connect), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### Prestandatest

Test av svarstider, genomströmning och resursanvändning under en viss belastning. [Lasttest](#lasttest) kontrollerar den förväntade belastningen, [stresstest](#stresstest) går förbi den, och uthållighetstest håller belastningen länge för att hitta läckor.

**I Qtestframe:** länka prestandatest till krav av typen *Prestanda* med tröskelvärden; registrera mätrapporter som bevis eller som verifieringsposter med metoden analys.

**Se även:** [Lasttest](#lasttest), [Stresstest](#stresstest), [Icke-funktionell testning](#icke-funktionell-testning)

### Prioritet

Hur snart något ska tas om hand, som ett verksamhetsbeslut. Ett kosmetiskt fel på startsidan kan ha hög prioritet; en krasch i en sällan använd administratörsfunktion kan ha låg. Prioritet är något annat än [allvarlighetsgrad](#allvarlighetsgrad).

**I Qtestframe:** krav, testfall och defekter har en prioritet (*Kritisk*, *Hög*, *Medel*, *Låg*). Den ordnar arbetslistor och filter, och chattkanaler kan begränsas till objekt med en lägsta prioritet.

**Se även:** [Allvarlighetsgrad](#allvarlighetsgrad), [Defekt](#defekt), [Statusar och allvarlighetsgrader](defects.md#statusar-och-allvarlighetsgrader)

### Problem Details

RFC 9457 definierar ett standardformat i JSON för felsvar från HTTP-API:er (`application/problem+json`) med medlemmarna type, title, status och detail.

**I Qtestframe:** katalogen har API-krav och testfall som hänvisar till RFC 9457 för system som du bygger. Qtestframes eget API svarar med ett eget JSON-felobjekt med felkod.

**Se även:** [HTTP-semantik](#http-semantik), [OpenAPI](#openapi), [Katalog](#katalog)

### Programvarukrav

Ett krav på en programvaruenhet, härlett från systemkraven och arkitekturen: dess funktioner, gränssnitt, data och kvalitet. Programvarukraven är det som utvecklarna genomför och som enhets- och integrationstest verifierar.

**I Qtestframe:** relatera programvarukraven till sina systemkrav med **förfinar** eller **härlett från**, och allokera dem till programvarukomponenterna under [systemelement](#systemelement).

**Se även:** [Systemkrav](#systemkrav), [Förfining](#förfining), [Enhetstest](#enhetstest)

### Projekt

En behållare för en produkt, ett system eller en leverans.

**I Qtestframe:** ett projekt innehåller krav, testfall, körningar, cykler, sessioner, defekter, risker, releaser, baslinjer, dokument och sina inställningar. Projektnyckeln inleder alla objektnycklar. Projekt kan begränsas till de medlemmar som arbetar i dem, med en egen projektroll, och sparas som ett [projektarkiv](#projektarkiv) som kan läsas in någon annanstans.

**Se även:** [Objekten och hur de hänger ihop](how-it-works.md#objekten-och-hur-de-hänger-ihop), [Organisation](#organisation), [Nyckel](#nyckel)

### Projektarkiv

En enda fil som innehåller ett helt projekt, för att lämna över det, behålla en kopia eller starta ett nytt projekt från det.

**I Qtestframe:** **Spara projekt** skriver en ZIP-fil med allt i projektet, även historik, utfall och bilagor, men aldrig hemligheter; **Läs in projekt** skapar ett nytt projekt från den, på samma eller en annan server, med nya id:n och nycklar.

**Se även:** [Spara och läs in projekt](reports-and-export.md#spara-och-läs-in-projekt), [Säkerhetskopia](#säkerhetskopia), [Projekt](#projekt)

## Q

### qtestframe report

Kommandot i programmet `qtestframe` som laddar upp testutfall från en CI-pipeline.

**I Qtestframe:** `qtestframe report --project SHOP --run "Nattlig 412" --create-run --junit 'results/*.xml'` hittar eller skapar körningen, laddar upp JUnit XML och registrerar bygge och miljö; det autentiserar sig med en [API-nyckel](#api-nyckel) i `QTF_TOKEN`.

**Se även:** [Kommandot qtestframe report](runs.md#kommandot-qtestframe-report), [JUnit XML](#junit-xml), [Kontinuerlig integration](#kontinuerlig-integration)

## R

### Regressionstest

Att testa det som fungerade tidigare igen efter en ändring, för att se om ändringen förstörde något. Regressionstestsviter växer med tiden och automatiseras därför oftast och väljs ut efter risk och efter vad som ändrats.

**I Qtestframe:** **Regressionsurval** föreslår testfall utifrån ändrade krav och incheckningar sedan ett bygge; en defekt som dyker upp igen efter att ha åtgärdats flaggas som en regression.

**Se även:** [Regressionsurval](variants-and-code.md#regressionsurval), [Röktest och sanitetstest](#röktest-och-sanitetstest), [Kontinuerlig integration](#kontinuerlig-integration)

### Release

En version av produkten som planeras, testas och levereras som en helhet.

**I Qtestframe:** en release har en omfattning (de krav den ska leverera), de körningar som testar den, [utgångskriterier](#utgångskriterier) och ett go/no-go-läge; undantag kan godta krav för en release, och en baslinje fryser den oftast.

**Se även:** [Releaser och risker](releases-and-risks.md), [Utgångskriterier](#utgångskriterier), [Baslinje](#baslinje)

### ReqIF

Requirements Interchange Format från Object Management Group (OMG): ett XML-format för att utbyta kravspecifikationer mellan verktyg, med attribut, hierarki och relationer. En `.reqifz`-fil är en ZIP-fil med ReqIF-filen och de bifogade filerna.

**I Qtestframe:** exportera krav (om du vill med testfall, spårlänkar, verifieringsposter eller en baslinje) som ReqIF 1.2 och importera ReqIF från DOORS, Polarion, Codebeamer, Jama eller ProR, även fram och tillbaka.

**Se även:** [ReqIF-export](interoperability.md#reqif-export), [ReqIF-import](interoperability.md#reqif-import), [OSLC](#oslc)

### Restrisk

Den risk som återstår när motåtgärderna är på plats. Den ska ligga på eller under det organisationen är beredd att acceptera, annars måste någon med befogenhet uttryckligen acceptera den.

**I Qtestframe:** en risk har en kvarstående sannolikhet och konsekvens; över projektets gräns för riskacceptans måste acceptansen signeras (`waiver.approve`), och utgångskriteriet *Restrisk* kontrollerar det.

**Se även:** [Restrisk](waivers-and-soa.md#restrisk), [Risk](#risk), [Undantag](#undantag)

### Risk

Något osäkert som, om det inträffar, skadar produkten, projektet eller användarna, bedömt efter sannolikhet och konsekvens. Produktrisker gäller kvalitet och säkerhet; projektrisker gäller tidplan, budget och bemanning.

**I Qtestframe:** riskregistret bedömer varje risk 1–5 för sannolikhet och konsekvens (poäng 1–25, nivå *Låg* till *Kritisk*), visar de krav som hanterar risken och hur långt de är täckta och verifierade; öppna risker påverkar releasens utgångskriterier.

**Se även:** [Riskregistret](releases-and-risks.md#riskregistret), [Riskbaserad testning](#riskbaserad-testning), [Restrisk](#restrisk)

### Riskbaserad testning

Att välja ut, ordna och dosera krav och testfall efter risk: ju större sannolikhet och konsekvens ett fel har, desto tidigare och grundligare specificeras och testas det. Det gör mest av begränsad testtid och förklarar för intressenterna vad som lämnades utanför och varför.

**I Qtestframe:** koppla kraven till de risker de hanterar, bygg releasens omfattning med `risk:high` och använd utgångskriterierna *Risker hanterade* och *Restrisk*. Riskrapporten och värmekartan visar var täckningen saknas.

**Se även:** [Risk](#risk), [Hur risker påverkar en release](releases-and-risks.md#hur-risker-påverkar-en-release), [Utgångskriterier](#utgångskriterier)

### Roll

En namngiven uppsättning behörigheter som ges till en person.

**I Qtestframe:** varje organisation har rollerna *Läsare*, *Granskare*, *Testare*, *Testledare*, *Ansvarig* och *Administratör* och kan ändra dem eller skapa egna; ett projekt kan ge en medlem en annan roll. *Superadministratörer* hanterar hela installationen.

**Se även:** [Roller och behörigheter](administration.md#roller-och-behörigheter), [Organisation](#organisation), [Behörigheter](../../admin-guide/sv/authorisation.md)

### Röktest och sanitetstest

Ett röktest (smoke test) är en liten, snabb uppsättning testfall som kontrollerar att ett nytt bygge alls fungerar (det startar, huvudsidorna öppnas, inloggningen fungerar) innan mer testtid läggs på det. Ett sanitetstest (sanity test) är en snabb, smal kontroll av att en viss rättning eller ändring fungerar som avsett.

**I Qtestframe:** tagga testfallen `smoke` och skapa en körning från `tag:smoke`, eller låt CI köra dem först; en underkänd röktestkörning säger åt alla att vänta på nästa bygge.

**Se även:** [Regressionstest](#regressionstest), [Ingångskriterier](#ingångskriterier), [Testkörning](#testkörning)

## S

### SAML 2.0

Security Assertion Markup Language, en OASIS-standard för enkel inloggning: identitetsleverantören skickar programmet ett signerat XML-intyg om användaren.

**I Qtestframe:** enkel inloggning med en SAML-identitetsleverantör som Active Directory Federation Services (AD FS), som alternativ till OpenID Connect.

**Se även:** [OpenID Connect](#openid-connect), [SCIM](#scim), [SAML 2.0](../../admin-guide/sv/authentication.md#saml-20)

### SCIM

System for Cross-domain Identity Management (RFC 7643 för datamodellen, RFC 7644 för protokollet): ett REST-gränssnitt genom vilket en identitetsleverantör automatiskt skapar, ändrar och tar bort användarkonton och grupper i ett program.

**I Qtestframe:** `/scim/v2` låter er identitetsleverantör provisionera användare och gruppmedlemskap i en organisation, med en egen nyckel; när en användare inaktiveras tas medlemskapet bort och användarens sessioner och API-nycklar i organisationen upphör direkt.

**Se även:** [Provisionering med SCIM](administration.md#provisionering-med-scim), [SAML 2.0](#saml-20), [Andra RFC:er](references.md#andra-rfcer-som-qtestframe-följer)

### seccomp

Secure computing mode: en funktion i Linuxkärnan med vilken en process talar om för kärnan vilka systemanrop den aldrig kommer att göra, så att de misslyckas även om en angripare tar över processen.

**I Qtestframe:** servern och varje skript i sandlådan installerar en spärrlista med systemanrop som en server aldrig behöver; med `runner.deny_network` förlorar skripten också anropen för nätverksuttag.

**Se även:** [Landlock](#landlock), [Skriptkörning](#skriptkörning), [Härdning](../../admin-guide/sv/security.md#härdning)

### Session

1. Ett inloggat besök av en användare, som hålls igång av en kaka tills den går ut eller användaren loggar ut. 2. Inom utforskande testning en tidsbegränsad testperiod med ett uppdrag (se [sessionsbaserad testledning](#sessionsbaserad-testledning)).

**I Qtestframe:** inloggningssessioner varar som standard i 12 timmar; du ser och avslutar dina under **Profil → Aktiva sessioner**. Utforskande sessioner finns under **Utforskande test** i ett projekt.

**Se även:** [Varför avslutades min session?](faq.md#varför-avslutades-min-session), [CSRF-skydd](#csrf-skydd), [Sessionsbaserad testledning](#sessionsbaserad-testledning)

### Sessionsbaserad testledning

SBTM (session-based test management): ett sätt att planera, genomföra och redovisa [utforskande testning](#utforskande-testning), beskrivet av Jonathan och James Bach år 2000. Testningen sker i ostörda sessioner med fast längd, var och en med ett uppdrag (charter), anteckningar under testningen och en utvärdering (debriefing) med en testledare efteråt; mått visar hur sessionstiden användes.

**I Qtestframe:** planera sessioner med uppdrag och omfattning, genomför dem med tidtagning och typade anteckningar (bugg, problem, fråga, idé …), skapa defekter från anteckningar och avsluta med en utvärdering och ett resultat; utvärderade sessioner kan räknas som verifieringsbevis.

**Se även:** [Utforskande test](exploratory-testing.md), [Utforskande testning](#utforskande-testning), [Session](#session)

### Shift-left

Att flytta test- och kvalitetsarbetet tidigare i livscykeln: granska kraven före designen, skriva testfall före eller samtidigt med koden, köra automatiserade tester vid varje ändring. Defekter som hittas tidigt är mycket billigare att rätta än de som hittas i produktion.

**I Qtestframe:** språkanalysen och granskningarna kontrollerar kraven medan de skrivs, ett länkat testfall kan skapas med varje krav, och utfall från CI kommer med varje bygge.

**Se även:** [Kontinuerlig integration](#kontinuerlig-integration), [Testdriven utveckling](#testdriven-utveckling), [V-modellen](#v-modellen)

### SKA, BÖR och FÅR

De svenska kravorden. Svenska standarder (SIS, i enlighet med ISO/IEC Directives) använder *ska* för krav, *bör* för rekommendation, *får* för tillåtelse och *kan* för möjlighet; negationerna är *ska inte*, *bör inte* och *får inte*.

**I Qtestframe:** svensk kravtext använder **SKA**/**SKALL** och **MÅSTE** (måste), **BÖR** (bör), **FÅR** (får), **FÅR INTE** och **SKA INTE** (får inte), **BÖR INTE**, **BEHÖVER INTE** och **VALFRI** (valfritt), **KOMMER ATT** (avsikt) och **BESKRIVER** (beskrivande). Analysen varnar för *måste inte*, som betyder ”behöver inte” men ofta läses som ett förbud. Observera att *får* också betyder ”tar emot”: *användaren får ett mejl* läses som tillåtelse.

**Se även:** [Språkanalysen](requirements.md#språkanalysen), [Kravord](#kravord), [Standarder för kravhantering](references.md#standarder-och-vägledningar-för-kravhantering)

### Skriptkörning

Den del av servern som kör skripten i automatiserade testfall.

**I Qtestframe:** när en administratör har slagit på den kan medlemmar med `script.execute` köra ett testfalls skript på servern. Varje skript körs i en avgränsad barnprocess med resursgränser, Landlock, seccomp och om så önskas utan nätverk; webbtester kör Playwright med en riktig webbläsare.

**Se även:** [Automatiserade testfall: skript](tests.md#automatiserade-testfall-skript), [Landlock](#landlock), [Sandlådan för skript och webbtester](../../admin-guide/sv/security.md#sandlådan-för-skript-och-webbtester)

### Specifikation

Ett dokument, eller en strukturerad mängd objekt, som anger krav, design eller beteende fullständigt och exakt. Ett krav är ett enskilt påstående; en specifikation är den ordnade helheten med inledning, omfattning, definitioner och själva kraven.

**I Qtestframe:** projektet är den levande specifikationen; **Skapa dokument → Kravspecifikation** skriver ett komplett dokument enligt ISO/IEC/IEEE 29148 till arkivet, och ReqIF utbyter specifikationen med andra verktyg.

**Se även:** [Skapa en kravspecifikation eller en testplan](documents.md#skapa-en-kravspecifikation-eller-en-testplan), [Krav](#krav), [ReqIF](#reqif)

### Språkanalys

En automatisk granskning av kravtexten mot skrivregler.

**I Qtestframe:** medan du skriver känner analysen igen [kravnivån](#kravnivå) från kravorden, markerar dem, listar fynd som vaga ord, reservationer, platshållare eller flera kravord i samma mening och sätter ett poängtal från 0 till 100, på svenska och engelska.

**Se även:** [Språkanalysen](requirements.md#språkanalysen), [INCOSE-regler](#incose-regler), [Kravord](#kravord)

### Spårbarhet

Förmågan att följa ett kravs liv framåt och bakåt: från ursprunget, genom förfining, design och genomförande, till dess testfall och deras utfall. Den stöder påverkansanalys, kontroller av fullständighet och revisioner.

**I Qtestframe:** spårlänkar, relationer, kodlänkar, spårbarhetsmatrisen, beroendegrafer, spårbarhetsregler och påverkansanalysen tjänar alla spårbarheten.

**Se även:** [Spårbarhet och täckning](traceability-and-coverage.md), [Framåtspårbarhet](#framåtspårbarhet), [Bakåtspårbarhet](#bakåtspårbarhet)

### Spårbarhetsmatris

En tabell med krav i den ena dimensionen och testfall (eller andra objekt) i den andra, där det markeras vilka som är länkade. Den visar med en blick vilka krav som saknar testfall och vilka testfall som inte verifierar något.

**I Qtestframe:** **Spårbarhet** visar krav som rader och testfall som kolumner; varje markering färgas efter testfallets senaste utfall och får en ram när länken är misstänkt, och du kan länka och ta bort länkar direkt i matrisen.

**Se även:** [Spårbarhetsmatrisen](traceability-and-coverage.md#spårbarhetsmatrisen), [Spårlänk](#spårlänk), [Dubbelriktad spårbarhet](#dubbelriktad-spårbarhet)

### Spårbarhetsregel

En regel som objekten ska följa i sina länkar, till exempel ”varje godkänt krav på nivån måste har minst ett testfall” eller ”varje testfall verifierar ett krav”.

**I Qtestframe:** ansvariga definierar regler med en fråga för objekten, ett villkor och en allvarlighetsgrad; överträdelser syns i kravanalysen och arbetslistorna, och ett utgångskriterium för releasen kan kräva att inga regler bryts.

**Se även:** [Spårbarhetsregler](traceability-and-coverage.md#spårbarhetsregler), [Konsistenskontroll](#konsistenskontroll), [Utgångskriterier](#utgångskriterier)

### Spårlänk

Ett registrerat samband mellan två objekt i livscykeln, till exempel ”testfall T verifierar krav K”. Spårlänkar är råmaterialet för [spårbarhet](#spårbarhet).

**I Qtestframe:** en spårlänk kopplar ett testfall till ett krav som det verifierar, om du vill till ett visst utvärderingskriterium; den är grunden för täckningen. Andra länkar kopplar krav till varandra (relationer), till kod, körningar och dokument.

**Se även:** [Länkade testfall](requirements.md#länkade-testfall), [Spårbarhet](#spårbarhet), [Misstänkt länk](#misstänkt-länk)

### STIX

Structured Threat Information Expression: ett JSON-format från OASIS för utbyte av information om cyberhot, till exempel angreppsmönster, hotaktörer och motåtgärder, och sambanden mellan dem.

**I Qtestframe:** MITRE publicerar ATT&CK, ATLAS och EMB3D som STIX 2.1-paket; katalogens listor över tekniker, hot och motåtgärder genereras från dem.

**Se även:** [MITRE ATT&CK](#mitre-attck), [MITRE EMB3D](#mitre-emb3d), [Format, protokoll och ramverk](references.md#format-protokoll-och-ramverk)

### Stresstest

En sorts [prestandatest](#prestandatest) bortom den förväntade belastningen eller med minskade resurser, för att hitta brytpunkten och se om systemet fallerar på ett kontrollerat sätt och återhämtar sig.

**I Qtestframe:** formulera det förväntade beteendet vid överbelastning (till exempel ”avvisar nya sessioner med ett meddelande men behåller befintliga”) som ett krav med kriterier, så att stresstestet har något att verifiera.

**Se även:** [Prestandatest](#prestandatest), [Lasttest](#lasttest), [Icke-funktionell testning](#icke-funktionell-testning)

### Svartlådetest

Test som bara utgår från specifikationen, utan att titta på koden eller den inre strukturen: testaren vet vad som går in och vad som ska komma ut. [Ekvivalensklassindelning](#ekvivalensklassindelning), [gränsvärden](#gränsvärdesanalys), [beslutstabeller](#beslutstabelltest) och [tillståndsövergångar](#tillståndsövergångstest) är svartlådetekniker (ISO/IEC/IEEE 29119-4).

**I Qtestframe:** de flesta manuella testfall är svartlådetest som härletts ur kravtexten och dess utvärderingskriterier.

**Se även:** [Vitlådetest](#vitlådetest), [Grålådetest](#grålådetest), [Testdesignteknik](#testdesignteknik)

### Syslog

Standarden för att skicka loggmeddelanden till en central loggserver. RFC 5424 definierar det nuvarande meddelandeformatet, RFC 3164 det äldre BSD-formatet.

**I Qtestframe:** servern kan skicka sin logg och händelseloggen till syslog över UDP, TCP, TLS eller ett Unix-uttag, i formatet RFC 5424 eller RFC 3164.

**Se även:** [Loggning och syslog](administration.md#loggning-och-syslog), [Händelselogg](#händelselogg), [Central syslog](../../admin-guide/sv/logging-and-monitoring.md#central-syslog)

### Systemelement

En del i produktstrukturen: ett system, delsystem, en komponent, produkt eller leverantörsleverans.

**I Qtestframe:** under **Verifiering → Systemelement** bygger du strukturen som ett träd och allokerar krav och testfall till elementen; varje element visar hur många av dess krav som är verifierade, och `element:"Kraftenhet"` filtrerar på det.

**Se även:** [Systemelement](traceability-and-coverage.md#systemelement), [Integrationstest](#integrationstest), [Programvarukrav](#programvarukrav)

### Systemkrav

Ett krav på systemet som helhet, sett utifrån: vad det ska göra och hur väl, på ingenjörernas språk snarare än intressenternas. Systemkrav härleds ur intressentkraven och förfinas till krav på delsystem, maskinvara och programvara.

**I Qtestframe:** relatera dem uppåt till intressentkrav och nedåt till programvarukrav med **förfinar**; systemtest verifierar dem.

**Se även:** [Intressentkrav](#intressentkrav), [Programvarukrav](#programvarukrav), [Systemtest](#systemtest)

### Systemtest

Testnivån som kontrollerar det kompletta, integrerade systemet mot systemkraven, i en miljö så lik produktionen som möjligt. Den omfattar både funktionella och icke-funktionella test.

**I Qtestframe:** systemtestkörningar riktar sig oftast mot flera [testkonfigurationer](#testkonfiguration) och miljöer; deras utfall räknas in i releasens utgångskriterier.

**Se även:** [Testnivå](#testnivå), [Acceptanstest](#acceptanstest), [Systemkrav](#systemkrav)

### Säkerhetskopia

En kopia av alla data som en installation kan återställas från efter ett fel. För Qtestframe är en säkerhetskopia av databasen plus nyckeln till de lagrade hemligheterna en säkerhetskopia av hela installationen.

**I Qtestframe:** superadministratörer tar och återställer säkerhetskopior under **Administration → Säkerhetskopiering**, eller med `qtestframe backup` och `restore` på servern. En återställning loggar ut alla. Ett enskilt projekt sparas och flyttas i stället med ett [projektarkiv](#projektarkiv).

**Se även:** [Säkerhetskopiering och återställning](administration.md#säkerhetskopiering-och-återställning), [Projektarkiv](#projektarkiv), [Säkerhetskopiering och migrering](../../admin-guide/sv/backup-and-migration.md)

### Säkerhetstest

Test av om systemet skyddar data och funktioner mot angrepp och missbruk: autentisering, behörighetskontroll, hantering av indata, sessionshantering, kryptografi, loggning. Det sträcker sig från kontroller av enskilda krav till penetrationstester och fuzzning.

**I Qtestframe:** katalogen har säkerhetskrav och testfall som hänvisar till OWASP ASVS, NIST, CWE och ATT&CK; registrera fynd som defekter, vid behov med allvarlighetsgraden *Kritisk*.

**Se även:** [CWE](#cwe), [MITRE ATT&CK](#mitre-attck), [Icke-funktionell testning](#icke-funktionell-testning)

## T

### Testcykel

En planerad period av testning för ett mål, till exempel regressionstest i en sprint eller acceptanstest inför en release, med sina testfall, testare, uppskattningar och datum.

**I Qtestframe:** en testcykel innehåller planen (vilka testfall, i vilka konfigurationer, av vem, hur länge); när den startar skapar den sin [körning](#testkörning) och följer planerat mot exekverat, en nedbränning och varje testares arbetsbelastning. Cykler kan återkomma.

**Se även:** [Testcykler](test-cycles.md), [Testkörning](#testkörning), [Testplan](#testplan)

### Testdata

De data ett testfall behöver: indata, förväntade värden, konton, filer och tillståndet i databaser. Testdata ska vara känd, upprepbar och säker (inga verkliga personuppgifter utan skäl).

**I Qtestframe:** testfallets tabell **Testdata** innehåller parametrar och rader för [datadriven testning](#datadriven-testning); större datamängder och filer läggs som bilagor eller i dokumentarkivet, och hemliga värden i hemliga testvariabler.

**Se även:** [Datadrivna testfall](tests.md#datadrivna-testfall), [Datadriven testning](#datadriven-testning), [Testmiljö](#testmiljö)

### Testdesignteknik

Ett systematiskt sätt att ta fram testfall och testdata ur ett testunderlag, så att testfallen väljs av ett skäl och deras täckning kan anges. ISO/IEC/IEEE 29119-4 delar in dem i specifikationsbaserade (svartlåda), strukturbaserade (vitlåda) och erfarenhetsbaserade tekniker.

**I Qtestframe:** författaren väljer tekniken; ange den som en kategoridimension (till exempel *Teknik / Gränsvärden*) om du vill rapportera på den.

**Se även:** [Ekvivalensklassindelning](#ekvivalensklassindelning), [Gränsvärdesanalys](#gränsvärdesanalys), [Felgissning](#felgissning)

### Testdriven utveckling

TDD: en utvecklingsmetod i korta cykler: skriv ett automatiserat test som misslyckas för nästa lilla del av beteendet, skriv precis så mycket kod att det går igenom, och förbättra sedan koden medan testerna förblir gröna.

**I Qtestframe:** enhetstest som skrivs med TDD rapporteras med JUnit XML; koppla dem till de programvarukrav de täcker via testnycklarna.

**Se även:** [Beteendedriven utveckling](#beteendedriven-utveckling), [Enhetstest](#enhetstest), [Shift-left](#shift-left)

### Testfall

En uppsättning förutsättningar, indata, åtgärder och förväntade resultat, framtagen för att kontrollera ett eller flera krav eller villkor. Ett bra testfall kan upprepas av någon annan med samma resultat.

**I Qtestframe:** ett testfall (nyckel `…-TC-n`) är manuellt, med [steg](#teststeg), eller automatiserat, med ett skript eller ett webbtest i Playwright. Det har status (*Utkast*, *Klart*, *Granskning*, *Utfasat*), länkar till de krav det verifierar, testdata och versionshistorik.

**Se även:** [Testfall](tests.md), [Teststeg](#teststeg), [Exekvering](#exekvering)

### Testkonfiguration

En kombination av de förhållanden ett testfall körs under, till exempel webbläsare, operativsystem, enhet eller språk. Att köra samma testfall i flera konfigurationer hittar problem som bara visar sig i en av dem.

**I Qtestframe:** organisationen definierar konfigurationer (till exempel *Chrome / Windows*); en körning med tre konfigurationer får en exekvering per testfall och konfiguration, och ett krav är godkänt först när dess testfall är godkända i varje konfiguration de är planerade i.

**Se även:** [Konfigurationer i en körning](runs.md#konfigurationer-i-en-körning), [Testkonfigurationer](administration.md#testkonfigurationer), [Parvis testning](#parvis-testning)

### Testkörning

En omgång där en utvald mängd testfall exekveras, i en miljö och ett bygge, ofta kallad testkampanj.

**I Qtestframe:** en körning (till exempel *Regressionstest sprint 12*) har en [exekvering](#exekvering) per testfall (och konfiguration), visar framsteg och andel godkända och kan höra till en release. Körningar skapas för hand, av en testcykel, från en fråga eller av CI.

**Se även:** [Testkörningar och exekvering](runs.md), [Testcykel](#testcykel), [Testsvit](#testsvit)

### Testmiljö

Den maskinvara, programvara, det nätverk, de data och verktyg som testerna körs i, till exempel *staging* eller ett labbrack. Utfall betyder bara något tillsammans med den miljö de togs fram i.

**I Qtestframe:** en körning registrerar **Miljö** och **Bygge**; kombinera med [testkonfigurationer](#testkonfiguration) för webbläsare eller enheter.

**Se även:** [Skapa en körning](runs.md#skapa-en-körning), [Testkonfiguration](#testkonfiguration), [Testkörning](#testkörning)

### Testnivå

En grupp testaktiviteter som organiseras tillsammans och riktar sig mot en nivå av systemet: [enhetstest](#enhetstest) (komponenttest), [integrationstest](#integrationstest), [systemtest](#systemtest) och [acceptanstest](#acceptanstest), samt [end-to-end-test](#end-to-end-test) över flera system. Varje nivå har sitt eget testunderlag, sina testobjekt, typiska defekter och ansvariga (ISTQB:s ordlista).

**I Qtestframe:** det finns inget fast fält för nivå; använd en kategoridimension som *Nivå / System*, mappar eller separata körningar och cykler per nivå.

**Se även:** [Testtyp](#testtyp), [V-modellen](#v-modellen), [Kategorier](tests.md#kategorier)

### Testorakel

Källan som avgör om ett resultat är rätt: specifikationen, ett referenssystem, en tidigare version, en handräkning eller en expertbedömning. Utan orakel kan ett testfall köras men inte bedömas.

**I Qtestframe:** det förväntade resultatet för varje steg och testfallets övergripande förväntade resultat är dess orakel; för automatiserade testfall är det kontrollerna i skriptet.

**Se även:** [Teststeg](#teststeg), [Testfall](#testfall), [Manuella testfall: steg](tests.md#manuella-testfall-steg)

### Testplan

Ett dokument som beskriver testningens omfattning, angreppssätt, resurser, tidplan, ingångs- och utgångskriterier och risker (ISO/IEC/IEEE 29119-3). Det är överenskommelsen om vad som ska testas, hur och av vem.

**I Qtestframe:** **Skapa dokument → Testplan** skriver en testplan med testspecifikation från projektet: de funktioner som ska testas, angreppssätt med ingångs- och utgångskriterier, miljö, roller, tidplan, varje testfall med sina steg och en bilaga om täckningen. En [testcykel](#testcykel) är den operativa planen för en testperiod.

**Se även:** [Skapa en kravspecifikation eller en testplan](documents.md#skapa-en-kravspecifikation-eller-en-testplan), [Testcykel](#testcykel), [Utgångskriterier](#utgångskriterier)

### Teststeg

En åtgärd i ett manuellt testfall, med det förväntade resultatet av åtgärden.

**I Qtestframe:** steg har en åtgärd, ett förväntat resultat och eventuellt platshållare för testdata; i exekveringsvyn får varje steg ett eget utfall, och ett steg kan anropa ett annat testfall som ett [delat steg](#delat-steg).

**Se även:** [Manuella testfall: steg](tests.md#manuella-testfall-steg), [Testfall](#testfall), [Delat steg](#delat-steg)

### Testsvit

En mängd testfall som grupperats för ett syfte, till exempel alla testfall för en funktion eller den nattliga regressionen. I många verktyg är en svit en mapp; i JUnit XML är den en grupp testfall i en rapport.

**I Qtestframe:** det finns inget separat svitobjekt: gruppera testfallen med mappar, kategorier eller taggar, spara frågan och skapa en körning från den.

**Se även:** [Testkörning](#testkörning), [Frågespråk](#frågespråk), [JUnit XML](#junit-xml)

### Testtyp

En grupp testaktiviteter som riktar sig mot en sorts kvalitet: [funktionell](#funktionell-testning), [icke-funktionell](#icke-funktionell-testning) (prestanda, säkerhet, användbarhet, tillgänglighet …), ändringsrelaterad ([regressionstest](#regressionstest), bekräftelsetest) och strukturell testning. Varje testtyp kan utföras på varje [testnivå](#testnivå).

**I Qtestframe:** använd en kategoridimension (*Typ / Säkerhet*) eller taggar; kravtyperna och katalogens kategorier följer samma indelning.

**Se även:** [Testnivå](#testnivå), [Kategorier](tests.md#kategorier), [Icke-funktionell testning](#icke-funktionell-testning)

### Testutfall

Resultatet av att exekvera ett testfall, också kallat dess bedömning (verdict).

**I Qtestframe:** en exekvering är *Godkänd*, *Underkänd*, *Blockerad* (kunde inte exekveras på grund av ett hinder), *Överhoppad* (medvetet utelämnad), *Pågår* eller *Ej körd*. Det senaste utfallet för varje testfall avgör täckningen för dess krav.

**Se även:** [Så beräknas täckningen](traceability-and-coverage.md#så-beräknas-täckningen), [Exekvering](#exekvering), [Inaktuellt utfall](#inaktuellt-utfall)

### Tillgänglighetstest

Test av om personer med funktionsnedsättning kan använda produkten: med enbart tangentbord, skärmläsare, förstoring, hög kontrast eller utan att vara beroende av färg. Automatiska verktyg hittar en del av problemen; resten kräver manuella kontroller mot en standard som [WCAG](#wcag).

**I Qtestframe:** katalogen har tillgänglighetskrav och testfall som hänvisar till WCAG 2.2. Webbklienten siktar själv på WCAG 2.1 AA och kontrolleras med automatiska tillgänglighetstester i ljust, mörkt och högkontrasttema.

**Se även:** [WCAG](#wcag), [Icke-funktionell testning](#icke-funktionell-testning), [Tillgänglighet och språk](accessibility-and-languages.md)

### Tillståndsövergångstest

En testdesignteknik för system vars beteende beror på deras tillstånd, till exempel en order (*skapad, betald, skickad, avbruten*) eller en session. Testfallen täcker tillstånden, de giltiga övergångarna och de händelser som ska avvisas i ett visst tillstånd (ISO/IEC/IEEE 29119-4).

**I Qtestframe:** ett testfall per väg eller övergång, med tillståndet i förutsättningarna; testflödesdiagrammet visar stegens och beroendenas ordning.

**Se även:** [Testdesignteknik](#testdesignteknik), [Testflöde](tests.md#testflöde), [Beslutstabelltest](#beslutstabelltest)

### TOTP

Time-based One-Time Password (RFC 6238): en sexsiffrig kod som beräknas från en delad hemlighet och det aktuella tidssteget på 30 sekunder och visas i en autentiseringsapp.

**I Qtestframe:** den andra faktorn i [tvåstegsverifieringen](#tvåstegsverifiering): HMAC-SHA-1, 6 siffror, steg om 30 sekunder, ett stegs klockavvikelse tillåts och varje kod godtas bara en gång.

**Se även:** [HOTP](#hotp), [Tvåstegsverifiering](#tvåstegsverifiering), [Flerfaktorsautentisering](../../admin-guide/sv/authentication.md#flerfaktorsautentisering)

### Tvåstegsverifiering

Inloggning med mer än ett slags bevis: något du vet (ett lösenord) och något du har (en telefon med en autentiseringsapp, en säkerhetsnyckel). Ett stulet lösenord räcker då inte. Kallas också flerfaktorsautentisering (MFA).

**I Qtestframe:** den andra faktorn är en [TOTP](#totp)-kod med tio engångskoder för återställning. Server- och organisationspolicyer kan kräva den av alla eller av vissa roller; vid enkel inloggning sköter oftast identitetsleverantören den.

**Se även:** [Tvåstegsverifiering](getting-started.md#tvåstegsverifiering), [TOTP](#totp), [WebAuthn och FIDO2](#webauthn-och-fido2)

### Täckning

Hur mycket av något som testerna når. Vad ”något” är avgör vilken sorts täckning det gäller: krav, risker, kod, konfigurationer eller datakombinationer. Täckning gäller alltid i förhållande till en modell och bevisar aldrig att det saknas defekter.

**I Qtestframe:** ”täckning” betyder [kravtäckning](#kravtäckning): varje kravs verifieringsläge, härlett från de senaste utfallen för dess länkade testfall (*Ingen täckning*, *Ej körd*, *Godkänd*, *Underkänd*, *Delvis*). Risktäckningen visas i riskregistret.

**Se även:** [Så beräknas täckningen](traceability-and-coverage.md#så-beräknas-täckningen), [Kodtäckning](#kodtäckning), [Kravtäckning](#kravtäckning)

## U

### Undantag

Ett formellt, signerat godkännande av att ett krav inte är uppfyllt (underkänt, otestat eller ej genomfört) för en release eller en period, oftast med villkor, kompenserande åtgärder och ett slutdatum (på engelska *waiver*). Till skillnad från en [avvikelse](#avvikelse) uppnås inte kravets syfte; risken accepteras.

**I Qtestframe:** undantag begärs på sidan **Undantag** eller från ett krav, godkänns med en elektronisk signatur av någon med `waiver.approve` (aldrig den som begärde det) och gör att releasens utgångskriterier godtar de undantagna kraven.

**Se även:** [Undantag och avvikelser](waivers-and-soa.md#undantag-och-avvikelser), [Avvikelse](#avvikelse), [Restrisk](#restrisk)

### Utforskande testning

Testdesign och testexekvering på samma gång: testaren lär sig produkten medan hen testar den och väljer nästa test utifrån det hen just såg. Den hittar problem som ingen tänkte på att skriva ett testfall för, och är mest effektiv när den är strukturerad med ett mål och en tidsram.

**I Qtestframe:** utforskande testning sker i sessioner med uppdrag, tidtagning, typade anteckningar och en utvärdering; se [sessionsbaserad testledning](#sessionsbaserad-testledning).

**Se även:** [Utforskande test](exploratory-testing.md), [Sessionsbaserad testledning](#sessionsbaserad-testledning), [Felgissning](#felgissning)

### Utgångskriterier

De villkor som ska vara uppfyllda för att avsluta en testaktivitet eller släppa en release: en andel krav täckta och godkända, inga öppna kritiska defekter, risker hanterade. De gör ”är vi klara?” till en kontroll i stället för en känsla (ISO/IEC/IEEE 29119-3, ISTQB:s ordlista).

**I Qtestframe:** en [release](#release) har utgångskriterier som *Täckning*, *Klarade testfall*, *Obligatoriska krav klarade*, *Inga öppna defekter*, *Inga misstänkta länkar*, *Risker hanterade* eller *Restrisk*, och fliken **Status** visar go eller no-go.

**Se även:** [Utgångskriterier](releases-and-risks.md#utgångskriterier), [Ingångskriterier](#ingångskriterier), [Undantag](#undantag)

### Uttalande om tillämplighet

SoA (Statement of Applicability): dokumentet där en organisation för varje åtgärd i en referensförteckning som ISO/IEC 27001 bilaga A anger om den är tillämplig, varför eller varför inte, och om den är genomförd.

**I Qtestframe:** sidan **SoA** listar åtgärderna med tillämplighet, motivering och ägare, härleder genomförandet från de krav som hänvisar till varje åtgärd och deras testfall, och rapporten **Uttalande om tillämplighet** skriver ut det.

**Se även:** [Uttalande om tillämplighet](waivers-and-soa.md#uttalande-om-tillämplighet), [ISO/IEC 27001 bilaga A](#isoiec-27001-bilaga-a), [Undantag](#undantag)

### Utvärderingskriterium

En beskrivning av hur uppfyllandet av ett krav ska bedömas: vad som kontrolleras, med vilken [metod](#verifieringsmetod) och vilket tröskelvärde som ska nås, till exempel ”svarstid p95 under 2 s, mätt med 200 samtidiga användare”.

**I Qtestframe:** varje krav kan ha flera kriterier, vart och ett med en metod (test, inspektion, analys, demonstration, granskning) och ett tröskelvärde. Testfall länkas till ett kriterium, andra metoder registreras som [verifieringsposter](#verifieringspost); kravet är verifierat när alla kriterier är det.

**Se även:** [Utvärderingskriterier](requirements.md#utvärderingskriterier), [Acceptanskriterier](#acceptanskriterier), [Verifiering](traceability-and-coverage.md#verifiering)

## V

### V-modellen

En livscykelmodell som ritas som ett V: den vänstra sidan går nedåt från krav via arkitektur till detaljerad design och kod, den högra sidan uppåt genom enhets-, integrations-, system- och acceptanstest, och varje testnivå verifierar arbetsprodukten mittemot. Den gör tidigt tydligt vilka test som ska verifiera vilken specifikation.

**I Qtestframe:** kravnivåer som kopplas ihop med **förfinar** och de testfall som länkas på varje nivå speglar V:et; sammanräkningen visar verifieringen från V:ets botten till toppen.

**Se även:** [Testnivå](#testnivå), [Verifiering](#verifiering), [Förfining](#förfining)

### Validering

Bekräftelse på att produkten uppfyller sin avsedda användning i sin avsedda miljö: ”bygger vi rätt produkt?” Validering involverar intressenter och verklig användning; en produkt kan klara all verifiering och ändå inte klara valideringen (IEEE 1012, ISO/IEC/IEEE 29148).

**I Qtestframe:** acceptanstest, demonstrationer för användare och utforskande sessioner med användare ger valideringsbevis; registrera dem mot intressent- eller användarkraven.

**Se även:** [Verifiering](#verifiering), [Acceptanstest](#acceptanstest), [Intressentkrav](#intressentkrav)

### Variant

En version i en produktfamilj som delar de flesta krav och testfall med de andra, till exempel en modell för en viss marknad.

**I Qtestframe:** objekt kan gälla alla varianter eller bara vissa; listor, täckning, releaser, dokument och releasegrindar kan visas för en variant.

**Se även:** [Varianter](variants-and-code.md#varianter), [Release](#release), [Testkonfiguration](#testkonfiguration)

### Verifiering

Bekräftelse, med objektiva bevis, på att en arbetsprodukt uppfyller sina specificerade krav: ”bygger vi produkten rätt?” Verifiering sker genom test, inspektion, analys, demonstration och granskning (IEEE 1012, ISTQB:s ordlista).

**I Qtestframe:** ett krav är verifierat när varje utvärderingskriterium har godkända bevis (testutfall, verifieringsposter, eventuellt utforskande sessioner) och alla dess barn är verifierade; **Verifiering** visar trädet och luckorna.

**Se även:** [Verifiering](traceability-and-coverage.md#verifiering), [Validering](#validering), [Verifieringsmetod](#verifieringsmetod)

### Verifieringsmetod

Sättet att visa att ett krav är uppfyllt. De klassiska metoderna är **test** (exekvering med bestämda indata och förväntade resultat), **inspektion** (granskning av kod, dokument eller produkten), **analys** (beräkning, simulering, modellering), **demonstration** (att visa funktionen utan detaljerade mätningar) och **granskning** (en formell bedömning av kvalificerade personer).

**I Qtestframe:** varje utvärderingskriterium har en av de fem metoderna; *Test* uppfylls av länkade testfall, de andra av [verifieringsposter](#verifieringspost).

**Se även:** [Utvärderingskriterier](requirements.md#utvärderingskriterier), [Verifieringspost](#verifieringspost), [Verifiering](#verifiering)

### Verifieringspost

Beviset för en verifiering som inte är en testexekvering: vem som inspekterade, analyserade, demonstrerade eller granskade vad, när, med vilket resultat och vilka dokument.

**I Qtestframe:** en post har metod, utfall (*Godkänd*, *Underkänd*, *Godkänd med villkor*), utförare, datum, anteckning, dokument och giltighet; den kan signeras av en andra person och blir ogiltig när kravet ändras, om man har valt det.

**Se även:** [Verifieringsposter](traceability-and-coverage.md#verifieringsposter), [Verifieringsmetod](#verifieringsmetod), [Utvärderingskriterium](#utvärderingskriterium)

### Verksamhetskrav

Ett krav som anger ett mål eller behov hos organisationen, till exempel ”halvera den manuella orderhanteringen”, snarare än en funktion i ett system. Verksamhetskrav förklarar varför ett projekt finns och förfinas till intressent-, användar- och systemkrav.

**I Qtestframe:** välj typen **Verksamhet**. Relatera de krav som genomför det med **förfinar**, så att kravhierarkin räknar samman deras verifiering till det.

**Se även:** [Intressentkrav](#intressentkrav), [Förfining](#förfining), [Krav](#krav)

### Version

En sparad version av ett objekt. Varje ändring skapar en ny version, så historiken visar vem som ändrade vad, när och hur.

**I Qtestframe:** fliken **Versionshistorik** jämför och återställer versioner; versionsnumret i redigerarens rubrik räknar dem. En [baslinje](#baslinje) registrerar en version av varje objekt vid ett visst tillfälle.

**Se även:** [Versionshistorik och återställning](requirements.md#versionshistorik-och-återställning), [Baslinje](#baslinje), [Baslinje eller version?](baselines.md#baslinje-eller-version)

### Vitlådetest

Test som utgår från den inre strukturen i koden eller systemet: testfallen tas fram för att exekvera satser, grenar, villkor eller vägar, och [kodtäckningen](#kodtäckning) mäter hur långt de når (ISO/IEC/IEEE 29119-4).

**I Qtestframe:** vitlådetest är oftast enhetstest som körs i CI; utfallen importeras, kodtäckningen stannar i byggverktygen.

**Se även:** [Svartlådetest](#svartlådetest), [Kodtäckning](#kodtäckning), [Enhetstest](#enhetstest)

## W

### WCAG

Web Content Accessibility Guidelines från W3C: testbara framgångskriterier, ordnade under principerna möjlig att uppfatta, hanterbar, begriplig och robust, på nivåerna A, AA och AAA. Version 2.2 (2023) bygger ut 2.1. Lagen om tillgänglighet till digital offentlig service hänvisar till dem via EN 301 549.

**I Qtestframe:** webbklienten siktar på WCAG 2.1 AA, och katalogen har krav och testfall som hänvisar till WCAG 2.2 för dina egna produkter.

**Se även:** [Tillgänglighetstest](#tillgänglighetstest), [Tillgänglighet och språk](accessibility-and-languages.md), [Standarder och vägledningar för test](references.md#standarder-och-vägledningar-för-test)

### WebAuthn och FIDO2

W3C:s Web Authentication API och FIDO2-standarderna: inloggning med en säkerhetsnyckel eller en passkey, baserad på ett nyckelpar som är bundet till webbplatsen och därför står emot nätfiske.

**I Qtestframe:** inte en inloggningsmetod i Qtestframe självt (det använder lösenord, enkel inloggning och TOTP), men katalogen har krav och testfall för passkeys och säkerhetsnycklar som hänvisar till WebAuthn.

**Se även:** [Tvåstegsverifiering](#tvåstegsverifiering), [TOTP](#totp), [Katalog](#katalog)

### Webbtest

Ett automatiserat testfall som styr en riktig webbläsare genom en webbapplikation.

**I Qtestframe:** webbtester är Playwright-skript (skriptspråket **Webbtest (Playwright)**), körs på servern som bakgrundsjobb när administratören har aktiverat dem och sparar utdata, skärmbilder och en spårning (trace) som bevis.

**Se även:** [Webbtester (Playwright)](tests.md#webbtester-playwright), [End-to-end-test](#end-to-end-test), [Skriptkörning](#skriptkörning)

### Webhook

En HTTP-begäran som ett program skickar till ett annat när något händer, så att det andra inte behöver fråga om och om igen.

**I Qtestframe:** projektets webhooks skickar händelser som `run.completed` eller `defect.created` som signerade JSON-begäranden med POST; inkommande webhooks från GitHub och GitLab skapar [kodlänkar](#kodlänk). Webhookhemligheter visas bara en gång.

**Se även:** [Webhooks](reviews-and-notifications.md#webhooks), [Kodlänkar](variants-and-code.md#kodlänkar), [Notis](#notis)

## Ä

### Ändringspåverkan

Det som en ändring kan påverka: de testfall som verifierar ett ändrat krav, de krav som förfinar det eller beror på det, deras utfall, dokument och öppna defekter. En påverkansanalys hittar dem före eller efter ändringen, så att inget blir inaktuellt utan att någon märker det.

**I Qtestframe:** när ett krav eller testfall ändras i sak blir dess länkar och relationer [misstänkta](#misstänkt-länk), utfallen [inaktuella](#inaktuellt-utfall), ärenden och framtagna dokument markeras som inaktuella, och påverkan sprids vidare längs relationerna upp till projektets påverkansdjup. Fliken **Påverkan** och påverkansanalysen visar vägarna.

**Se även:** [Ändringspåverkan](traceability-and-coverage.md#ändringspåverkan), [Misstänkt länk](#misstänkt-länk), [Baslinje](#baslinje)

### Ärendesystem

Ett verktyg där utvecklingsteam hanterar buggar och uppgifter, till exempel Jira, GitHub Issues eller GitLab.

**I Qtestframe:** ett projekt kan skicka defekter och krav till Jira, GitHub, GitLab eller Request Tracker, synkronisera deras status och importera ärenden; ett ändrat krav markerar sitt ärende som inaktuellt.

**Se även:** [Anslut Jira, GitHub eller GitLab](defects.md#anslut-jira-github-eller-gitlab), [Integrationer](how-it-works.md#integrationer), [Defekt](#defekt)
