# Rapporter och export

Qtestframe tar ut data på två sätt (ett helt projekt kan också [sparas som fil och läsas in igen](#spara-och-läs-in-projekt)):

- **Export** laddar ned projektets rådata (krav, kriterier, testfall, spårbarhet, körningar) som kalkylblad, dokument eller Markdown, för granskningar, revisioner, arbete offline eller andra verktyg.
- **Rapporter** tar fram formaterade dokument för människor: en statusrapport för projektet, en täckningsrapport, en sammanfattning av en körning, en defektlista eller en rapport som du utformar själv. Rapporter kan laddas ned eller sparas i projektets [dokumentarkiv](documents.md).

Alla med rollen **Läsare** eller högre får exportera och ta fram rapporter. Att spara en egen rapportdefinition kräver **Testare** (testfall) eller **Ansvarig** (krav); att spara en framtagen rapport i arkivet får alla som får ta fram den.

## Export

Välj **Export** i projektets sidopanel.

![Exportsidan med format, innehåll och ett valfritt filter](../images/sv-export.png)

1. Välj **Format**:

   | Format | Resultat |
   |---|---|
   | **PDF** | En utskriftsvänlig rapport i liggande format med en tabell per del och sidnummer. |
   | **DOCX** | Ett redigerbart Word-dokument med en rubrik och en tabell per del, till exempel som grund för ett specifikationsdokument. |
   | **XLSX** | En Excel-arbetsbok med bladet *Summary* och ett blad per del; fet rubrikrad, låst första rad, autofilter och lämpliga kolumnbredder. |
   | **CSV** | En `.csv`-fil för en del; ett `.zip`-arkiv med en CSV-fil per del när du väljer flera. UTF-8, kommaavgränsad. |
   | **Markdown** | En `.md`-fil med alla delar: en rubrik, projektet, filtret och tidpunkten och en tabell per del. Ren text som läses bra i en wiki, ett Git-förråd eller vilken textredigerare som helst; nycklar länkar till Qtestframe när serverns adress är inställd. |
   | **ReqIF** | Kraven som en ReqIF 1.2-fil (`.reqif`) för kravverktyg som IBM DOORS, Polarion, Jama eller ProR. Se [ReqIF för kravverktyg](#reqif-för-kravverktyg). |
   | **ReqIF (med bilagor)** | Ett `.reqifz`-arkiv: ReqIF-filen och kravens bilagor och bilder. |

2. Välj **Innehåll**:

   | Del | Kolumner |
   |---|---|
   | **Krav** | nyckel, titel, kravnivå, typ, prioritet, status, mapp, kategorier, taggar, beskrivning, motivering, källa, ägare, version, länkade testfall, täckning |
   | **Utvärderingskriterier** | krav, nummer, kriterium, metod, tröskelvärde |
   | **Testfall (med steg)** | nyckel, titel, exekvering, status, prioritet, mapp, kategorier, taggar, förutsättningar, steg, förväntat resultat, skriptspråk, länkade krav, senaste utfall |
   | **Spårbarhetsmatris** | en rad per länk mellan krav och testfall med täckning och senaste utfall; krav utan länkar får en rad |
   | **Körningar och exekveringar** | en rad per exekvering med körning, status, miljö, bygge, utfall, exekverad av, tidsåtgång och anteckningar |

   Kolumnrubrikerna i de exporterade filerna är på engelska.

   ReqIF innehåller bara krav, så valet av innehåll används inte för det formatet.

3. Ange eventuellt ett **Filter**. Det gäller krav om det är en giltig fråga för krav och testfall om det är en giltig fråga för testfall: `obligation=must` filtrerar bara krav, `type=automated` bara testfall, `folder:Kassa` båda.
4. Välj **Ladda ned**. Filen namnges efter projektnyckeln och tidpunkten, till exempel `SHOP-20261119-143000.xlsx`.

**Säkert för kalkylblad:** värden som börjar med `=`, `+`, `-` eller `@` får ett prefix i CSV-filer, och celler i XLSX är alltid text, så inget i dina data körs som en formel.

### ReqIF för kravverktyg

ReqIF (Requirements Interchange Format) är standardformatet för att utbyta krav mellan verktyg. Använd det för att lämna dina krav till en kund eller leverantör i deras eget verktyg, eller för att hämta tillbaka krav som har bearbetats där.

Det här innehåller ReqIF-filen:

- **Ett objekt per krav** med nyckel (*ReqIF.ForeignID*), rubrik (*ReqIF.Name*), beskrivning som formaterad text (*ReqIF.Text*), kravnivå, typ, prioritet och status som värdelistor, källa, motivering, ägare, taggar, kategorier, acceptanskriterier (en rad per kriterium: metod, text och tröskelvärde), version, upphovsperson och när kravet skapades.
- **Mapparna som rubriker.** Varje mapp blir ett rubrikobjekt (*ReqIF.ChapterName*); mappens krav och sedan dess undermappar ligger under rubriken, så att kravverktyget visar samma kapitel som kravträdet. Med ett filter tas bara mappar med exporterade krav med.
- **Relationer** mellan de exporterade kraven (*förfinar*, *beror på* och de andra) som ReqIF-relationer.
- **Formatering.** Stycken, radbrytningar, listor, rubriker, kodblock, fetstil, kursiv, länkar och bilder i beskrivningar blir formaterad text. I en `.reqifz`-fil ligger bilder och andra bilagor som filer i arkivet och listas i attributet *Attachments*.
- **Stabila identifierare.** Varje objekt får samma identifierare i varje export, så att ett verktyg känner igen krav som det redan har och kan uppdatera dem.

**Importera igen.** En ReqIF-fil från Qtestframe kan [importeras](requirements.md#importera-krav) i vilket projekt som helst. Rubriker, beskrivningar med sin formatering, kravnivå, typ, prioritet, status, källa, motivering, ägare, taggar, kriterier och mappar kommer tillbaka; den ursprungliga nyckeln sparas som externt ID (*ID: SHOP-REQ-12* i källan), och de nya kraven får nycklar i målprojektet. Kategorier och bilagor importeras inte. För att hämta tillbaka en redigerad version till **samma** projekt som uppdateringar, med relationer, använder du [ReqIF-importen med uppdateringar](#reqif-från-doors-och-polarion).

## Standardrapporter

Välj **Rapporter** i projektets sidopanel. Standardrapporterna är färdiga upplägg:

| Rapport | Innehåll | Parametrar |
|---|---|---|
| **Projektstatus** | nyckeltal, krav per status och kravnivå, täckning, senaste körningar | – |
| **Kravtäckning** | varje krav med länkade testfall, deras senaste utfall och täckningen | valfritt filter |
| **Spårbarhet** | länkarna mellan krav och testfall, och [spårbarhetsreglerna](traceability-and-coverage.md#spårbarhetsregler) med resultat och överträdelser | valfritt filter |
| **Sammanfattning av körning** | en körning: summor, exekveringar, defekter och tidsåtgång | körningen (obligatorisk) |
| **Defekter** | alla defekter som registrerats på exekveringar, grupperade per defekt | – |
| **Testförteckning** | testfall per mapp, typ, status och kategori | valfritt filter |
| **Riskrapport** | [riskerna](releases-and-risks.md) med nyckeltal, värmekartan före och efter åtgärder, riskerna över acceptansgränsen, riskregistret med [restrisken](waivers-and-soa.md#restrisk), spårbarhet till krav och testfall, otestade åtgärder och de största riskerna | – |
| **Efterlevnad** | efterlevnad av lagar, standarder och bästa praxis: per källa, per avsnitt, de katalogkrav som saknas och de krav som inte är uppfyllda (se [Efterlevnad av lagar och standarder](traceability-and-coverage.md#efterlevnad-av-lagar-och-standarder)) | valfria lagar, standarder och bästa praxis (standard: de som projektet använder) |
| **Uttalande om tillämplighet** | varje åtgärd i en standard med tillämplighet, motivering, införande, ägare, krav och belägg (se [Uttalande om tillämplighet](waivers-and-soa.md#uttalande-om-tillämplighet)) | standarden (standard: ISO/IEC 27001:2022 bilaga A) |
| **Releaseberedskap** | en release: go / no-go, varje utgångskriterium med vad som stoppar det och vad ett [undantag](waivers-and-soa.md#undantag-och-avvikelser) eller en signerad risk accepterar, gällande undantag och riskerna över acceptansgränsen | releasen (standard: den aktuella) |

**Riskrapporten** innehåller:

- **Nyckeltal**: alla risker; öppna risker (*Identifierad* eller *Åtgärdas*); åtgärdade, accepterade och stängda risker; öppna risker per nivå; öppna risker utan krav, med otestad åtgärd och med verifierad åtgärd; och öppna risker som inte har ändrats på 90 dagar eller mer.
- **Risker per nivå och status**: en tabell med en rad per nivå och en kolumn per status.
- **Öppna risker per åtgärdsstatus** (se nedan).
- **Värmekarta**: sannolikhet (rader, 5 överst) × konsekvens (kolumner). Varje ruta anger antalet risker som inte är stängda och rutans nivå, till exempel *2 · Hög*, så att tabellen kan läsas utan färger.
- **Största öppna risker**: de tio öppna riskerna med högst poäng.
- **Riskregister**: varje risk med nyckel, titel, kategori, ägare, sannolikhet, konsekvens, poäng, nivå, status, åtgärd, kraven som hanterar den, åtgärdens status och när den senast ändrades.
- **Spårbarhet**: en rad per risk, krav och länkat testfall, med kravets täckning och testfallets senaste utfall.
- **Öppna risker med otestad åtgärd**: öppna risker utan krav, eller med krav som saknar testfall, och vilka av deras krav som saknar testfall eller inte är godkända.
- **Öppna risker som inte uppdaterats på 90 dagar eller mer**: en påminnelse om att se över dem.

**Åtgärdens status** för en risk kommer från kraven som hanterar den: *Inga krav*, *Otestad* (inget av dem har länkade testfall), *Delvis testad* (några har), *Testad, inte alla godkända* (alla har testfall, men alla testfall är inte godkända) eller *Verifierad* (alla länkade testfall för alla dess krav är godkända). En risk har en poäng för sannolikhet × konsekvens; rapporten visar poängen och nivån, och hur långt åtgärden är verifierad av tester.

![Rapportsidan med standardrapporterna och rapportbyggaren](../images/sv-reports.png)

1. Välj en rapport.
2. Fyll i parametrarna, till exempel körningen för **Sammanfattning av körning**.
3. Välj **Format**: **PDF**, **Word** (DOCX), **Excel** (XLSX), **CSV** eller **Markdown** (`.md`, text med tabeller för wikis, Git-förråd och chattverktyg; flera avsnitt blir ett dokument).
4. Välj **Ladda ner**, eller **Spara i arkivet**. En sparad rapport blir ett dokument av typen *rapport* under **Dokument**, där hela teamet kan hitta, förhandsvisa och ladda ned den senare.

Rapporter tas fram på det språk du just nu använder i Qtestframe (**Användarmeny → Språk**, se [Språk](accessibility-and-languages.md#språk)). Med det svenska gränssnittet är rapportens rubrik, avsnitt, kolumnrubriker, nyckeltal och värden som status, utfall, täckning och kravnivå (till exempel *Godkänt*, *Underkänd*, *MUST (SKA)*) på svenska, med samma ord som i appen. Ditt eget innehåll, som titlar, namn, beskrivningar, taggar och anteckningar, översätts aldrig. Det gäller nedladdningar, rapporter som sparas i arkivet, egna rapporter, flera rapporter på en gång och rapporter som skickas med e-post.

### Dokument

Under standardrapporterna finns **Dokument** med två kompletta dokument: en **kravspecifikation** och en **testplan** med testspecifikation. **Skapa …** öppnar verktyget i [dokumentarkivet](documents.md#skapa-en-kravspecifikation-eller-en-testplan), där dokumentet sparas.

## Egna rapporter

Under **Anpassad rapport** utformar du en rapport själv:

1. **Rapport över**: krav, testfall, testkörningar eller exekveringarna i en körning (välj då körningen).
2. **Filter**: valfri [fråga](search-and-filters.md), till exempel `status:approved obligation=must`.
3. **Kolumner**: välj kolumnerna och ordna dem under kolumnordningen.
4. **Gruppera efter**: gruppera eventuellt raderna, till exempel per status eller mapp, med ett antal per grupp.
5. **Sortera efter**, eventuellt **Fallande**.
6. **Ta med sammanfattning**: summor överst.
7. Rapportens **Rubrik**.

![Rapportbyggaren med objekttyp, filter, kolumner och gruppering](../images/sv-report-builder.png)

Ta fram rapporten i valfritt format, som ovan. För att återanvända den väljer du **Spara rapportdefinition** och ger den ett namn: med **Läs in sparad rapport** hämtar du den när som helst för att ta fram den igen med aktuella data, ändra den eller ta bort den.

**Exempel:**

- *Öppna MUST-krav*: krav, filter `obligation=must -status:verified`, kolumnerna nyckel, titel, status, ägare, täckning, grupperade per ägare.
- *Underkända testfall i sprinten*: exekveringar i körningen *Sprint 12*, filter `status:failed`, kolumnerna nyckel, titel, exekverad av, defekter, anteckningar.
- *Automatiseringsgrad*: testfall, grupperade per exekveringstyp, kolumnerna nyckel, titel, status, senaste utfall.

## Flera rapporter på en gång

**Flera rapporter på en gång**, längst ner på sidan Rapporter, listar standardrapporterna och dina sparade egna rapporter. Kryssa i dem du behöver, till exempel ett veckopaket med *Projektstatus*, *Sammanfattning av körning* och två sparade rapporter, och sedan:

- välj **format** (bara format som alla markerade rapporter stöder visas),
- för *Sammanfattning av körning*, välj **körning**,
- **Ladda ner som ZIP** för att få alla i en fil, eller **Spara i arkivet** för att lägga var och en i [dokumentarkivet](documents.md) (taggad *report*). När du sparar listas en rapport som inte kan skapas med orsaken, och de andra sparas ändå.

**Ta bort sparade rapporter** tar bort de markerade sparade egna rapporterna (standardrapporter kan inte tas bort). Att spara i arkivet och att ta bort kräver behörighet att redigera krav eller testfall; att ladda ner kräver exportbehörighet.

## Skicka en rapport med e-post

När servern kan skicka e-post har varje standardrapport en kuvertknapp (**Skicka med e-post**), och verktyget för egna rapporter har också **Skicka med e-post**. Välj:

- **Medlemmar**: personer i din organisation som får läsa projektet. En medlem som inte får läsa det rapporten visar (till exempel testkörningar) hoppas över, och meddelandet säger vem.
- **Andra adresser**, åtskilda med kommatecken: personer utanför organisationen. Det kräver behörighet att ändra projektet, så att Qtestframe inte kan användas för att skicka e-post till vem som helst.
- ett **Meddelande** om du vill och **Format**.

Rapporten tas fram med dina behörigheter och bifogas e-postmeddelandet; rapporten och e-posttexten är på ditt nuvarande gränssnittsspråk. Om det inte kan levereras får du en [notis](reviews-and-notifications.md#notiser).

## Schemalagda rapporter

**Schemalagda rapporter**, längst ned på sidan Rapporter, skickar [sparade egna rapporter](#egna-rapporter) med e-post enligt ett schema. *(Kräver behörighet att redigera krav eller tester.)* Välj **Schemalägg en rapport** och välj sedan den sparade rapporten, **Hur ofta** (varje dag, varje vecka på en veckodag eller varje månad på en dag från 1 till 28), **Tid** och **Tidszon**, **Format**, mottagarna och ett meddelande om du vill.

Listan visar varje schema med dess mottagare, när det **Senast skickades** (eller felet vid senaste försöket) och **Nästa utskick**. Stäng av **Aktiv** för att pausa ett schema, välj **Skicka nu** för att skicka direkt (schemat ändras inte), eller ändra och ta bort det. Bara den som schemalade rapporten, eller någon som får ändra projektet, kan ändra schemat.

Bra att veta:

- Rapporten tas fram med behörigheterna hos den som schemalade den, vid den tidpunkt den skickas. Om personen inte längre är medlem eller inte längre får exportera visar schemat ett fel i stället för att skicka.
- Rapporten och e-postmeddelandet är på gränssnittsspråket hos den som schemalade rapporten eller senast ändrade schemat. Byt språk och spara schemat igen för att ändra det.
- Medlemmar som inte längre får läsa rapporten hoppas över.
- Om servern var nere när en rapport skulle skickas skickas den en gång när servern är igång igen, inte en gång för varje missat tillfälle.
- När den sparade rapporten tas bort försvinner även dess scheman.

## Spara och läs in projekt

Ett helt projekt kan sparas som en fil och läsas in igen som ett **nytt** projekt, på samma server eller en annan: för att lämna över ett projekt, behålla en kopia utanför servern eller starta ett nytt projekt från en mall.

- **Spara projekt**: i projektlistan (nedladdningsknappen för ett projekt) eller under **Kategorier & inställningar → Projekt → Spara och läs in**. Filen (`<nyckel>-project-<datum>.zip`) innehåller allt i projektet: krav med kriterier och mappar, testfall med steg och testdata, spårbarhetslänkar och relationer, länkar, körningar med alla resultat, iterationer och bevis, defekter, risker, releaser, baslinjer och signeringar, granskningar, kommentarer, versionshistorik, dokument och bilagor med innehåll, kategorier, konfigurationer, spårbarhetsregler, sparade rapporter, webhooks och projektets inställningar. Det kräver behörighet att ändra projektet och att exportera (som standard **Ansvarig** och **Administratör**).
- **Läs in projekt**: i projektlistan eller i samma avsnitt av inställningarna. Välj filen; Qtestframe kontrollerar den och visar vad den innehåller. Välj sedan det nya projektets **Nyckel** och **Namn** (en ledig nyckel föreslås om den ursprungliga är upptagen) och välj **Läs in projekt**. Du behöver behörighet att skapa projekt. Det nya projektet öppnas när det är klart.
- **Duplicera projekt** gör båda i ett steg: det skapar en kopia med en ny nyckel i samma organisation.

Bra att veta om inlästa projekt:

- Det nya projektet får nya id:n överallt, och objektnycklarna följer den nya projektnyckeln (`DEMO-REQ-1` blir `COPY-REQ-1`, även där en beskrivning nämner den). Nya objekt fortsätter originalets numrering.
- Personer förblir desamma: *skapad av*, ägare och tilldelade behålls som text, och kommentarer och granskningar hänvisar till samma konton. En granskningsbegäran vars granskare inte har något konto på servern utelämnas; resultatet visar hur många poster som utelämnades.
- **Hemligheter sparas aldrig**: webhooks läses in avstängda och utan hemlighet, och felhanteringssystemet utan sin token. Ange dem igen under **Kategorier & inställningar → Projekt** innan du slår på dem.
- Medlemmar, roller, API-nycklar, aviseringar, schemalagda e-postrapporter, händelseloggen och papperskorgen ingår inte i filen. Åtkomsten till det nya projektet följer organisationens roller, som för alla projekt.
- En fil som sparats av en nyare version av Qtestframe kan inte läsas in; uppdatera servern först. Filer från äldre versioner går att läsa in.

Filen innehåller projektets data i klartext. Förvara den lika omsorgsfullt som själva projektet.

## Utbyta data med andra verktyg

Krav finns ofta på mer än ett ställe: en kund håller dem i IBM DOORS eller Siemens Polarion, en kollega vill granska dem i Excel och ett ALM-verktyg vill länka till dem. Qtestframe utbyter data på fyra sätt, alla under **Importera** på krav- och testsidorna och via API:et:

| Sätt | Riktning | Passar för |
|---|---|---|
| [Excel fram och tillbaka](#redigera-i-excel-och-importera-tillbaka) | ut och in igen | redigera många poster på en gång i Excel, även offline |
| [ReqIF-import med uppdateringar](#reqif-från-doors-och-polarion) | in, gång på gång | en specifikation som förvaltas i DOORS, DOORS Next eller Polarion |
| [CSV- och Excel-exporter från DOORS och Polarion](#csv-och-excel-från-doors-och-polarion) | in, en gång | en första import när ReqIF inte finns |
| [OSLC](#oslc-länkar-från-alm-verktyg) | läses av andra verktyg | länkar från IBM ELM, Polarion, Codebeamer och andra OSLC-klienter |

Varje import visar först en **förhandsgranskning** och sparar ingenting förrän du väljer **Spara**. Förhandsgranskningen jämför filen med projektet, rad för rad:

- **Ny**: en post som inte finns och som skapas.
- **Ändrad**: en post som filen ändrar; varje ändrat fält visas med gammalt och nytt värde.
- **Oförändrad**: inget att göra.
- **Konflikt**: någon har sparat posten i Qtestframe efter att filen gjordes, och filen har andra värden. En konflikt sparas inte om du inte väljer **Skriv över konflikterna med filen**. Öppna posten för att se vad som ändrades, eller exportera igen.
- **Fel**: ett värde som inte kan importeras, med cellen (till exempel `F12`) och orsaken: en okänd prioritet, en okänd kravnyckel, en tom titel, en status du inte får sätta.

Använd statusknapparna ovanför tabellen för att bara visa vissa rader, och avmarkera rader du inte vill ha. **Spara** lagrar alla valda rader på en gång: om en av dem inte kan sparas sparas ingenting. Varje sparad post får en ny version i historiken, ändrade länkar blir misstänkta som vid en vanlig ändring, och importen registreras i händelseloggen.

### Redigera i Excel och importera tillbaka

1. Välj **Importera → Exportera för redigering i Excel** på krav- eller testsidan. Arbetsboken innehåller posterna i den aktuella listan (med dess filter och mapp) *(Läsare med exportbehörighet)*.
2. Redigera arbetsboken i Excel eller ett annat kalkylprogram. Behåll kolumnerna **ID** och **Version**: de talar om för Qtestframe vilken post en rad är och vilken version du utgick från.
3. Välj **Importera → Importera en redigerad Excel-arbetsbok**, välj filen, kontrollera förhandsgranskningen och välj **Spara** *(Manager)*.

Det här innehåller arbetsboken:

- **En rad per post** med *ID*, *Key*, *Version* och en kolumn per fält: mapp, titel, beskrivning, kravnivå, typ, prioritet, status, källa, motivering, ägare, taggar, acceptanskriterier (ett per rad, till exempel `test: Inloggning fungerar (threshold: 2 s)`) och en kolumn per relationstyp (*Refines*, *Depends on* …) med nycklarna till de relaterade kraven. Tester har i stället förutsättningar, förväntat resultat, exekveringstyp, uppskattade minuter och *Verifies* (nycklarna till de krav som testet verifierar).
- **Teststeg** på bladet *Steps*, en rad per steg: testet, stegnumret, åtgärd, data, förväntat resultat och, för ett delat steg, nyckeln till det anropade testet. Stegen i ett test som finns på bladet *Tests* ersätts av dess rader på bladet *Steps*.
- **Listrutor** för fälten med fasta värden, grå kolumner som bara är information (*Verified by*) och ett blad *Instructions*.

Kolumnrubrikerna är på engelska; svenska rubriker som *Titel*, *Prioritet* och *Mapp* känns också igen.

Tips:

- **Nya poster:** lägg till en rad och lämna *ID* tomt. För att ge ett nytt test steg skriver du en tillfällig nyckel som `NY-1` i *Key* och använder den på bladet *Steps*.
- **Mappar:** skriv en sökväg som `Säkerhet/Inloggning`; mappar som saknas skapas.
- **Kolumner** får tas bort eller flyttas: en kolumn som saknas ändrar ingenting. Kolumner som Qtestframe inte känner till listas och ignoreras.
- **Ett annat projekt:** rader med ID för en post i ett annat projekt är fel; töm ID-kolumnen för att skapa posterna i det här projektet.

### ReqIF från DOORS och Polarion

ReqIF är formatet som IBM DOORS, DOORS Next, Siemens Polarion och de flesta kravverktyg exporterar. **Importera → ReqIF från DOORS eller Polarion, med uppdateringar** importerar en sådan fil (`.reqif` eller `.reqifz`) och kommer ihåg var varje krav kom ifrån, så att en import av en nyare version av samma dokument **uppdaterar** kraven i stället för att skapa dem igen *(Manager)*.

1. Välj filen. Förhandsgranskningen listar dokumentets **attribut** med typ, ett exempelvärde och fältet som vart och ett läses in i.
2. Kontrollera fälten. Qtestframe känner igen ReqIF-standardnamnen (*ReqIF.Name*, *ReqIF.Text*, *ReqIF.ChapterName*, *ReqIF.ForeignID*) och vanliga namn i DOORS och Polarion (*Title*, *Description*, *Status*, *Priority*, *Severity* som kravnivå, *Assignee* som ägare, *ID*). Välj ett annat fält eller *ignorera* för valfritt attribut; förhandsgranskningen uppdateras direkt. **Rubrik (mapp)** markerar attributet med kapitelnamn; **ID i det andra verktyget** sparas i källan för nya krav (*ID: BRK-12*).
3. Välj **Målmapp** och om **Rubriker blir mappar**: kapitlen i dokumentets hierarki blir mappar under målmappen (i DOORS objekt med kapitelnamn och utan text, i Polarion arbetsposter av typen *Heading*).
4. Kontrollera raderna och **relationerna** och välj **Spara**.

Det här händer:

- **Relationer** mellan de importerade kraven (länkmoduler i DOORS, länkroller i Polarion) blir kravrelationer. Typen tas från namnet: *refines*, *satisfies* och *has parent* blir *förfinar*, *derived from* blir *härledd från*, *depends on* och *requires* blir *beror på* och så vidare; övriga blir *relaterar till*.
- **Formaterad text** (XHTML) blir vanlig text med sina radbrytningar och listor.
- **En ny import** uppdaterar kraven som skapades av tidigare importer av samma dokument och lägger till nya. Ett krav som har ändrats i Qtestframe sedan förra importen visas som en konflikt när dokumentet har andra värden. Relationer som har tagits bort i det andra verktyget tas bort här också; relationer som du har skapat i Qtestframe finns kvar. En import tar aldrig bort krav.
- Ett krav utan titel i dokumentet får de första orden i sin text när det skapas; senare importer behåller titeln du har gett det.
- En ReqIF-fil som har **exporterats från samma projekt** (se [ovan](#reqif-för-kravverktyg)) matchas mot sina krav, så att du kan skicka en specifikation till en leverantör och importera deras redigerade version tillbaka med samma förhandsgranskning.

### CSV och Excel från DOORS och Polarion

När bara en CSV- eller Excel-export finns använder du **Importera → Från en fil** och väljer **Exporterad från: IBM DOORS** eller **Siemens Polarion**. Förvalet läser de vanliga kolumnerna i sådana exporter: från DOORS *Absolute Number*, *Object Heading*, *Object Text*, *Object Level* och *Object Number*; från Polarion *ID*, *Title*, *Description*, *Status*, *Type*, *Priority*, *Severity*, *Assignee* och *Outline Number*. Rubrikrader blir mappar, nästlade efter sin nivå. Du kan fortfarande byta fält för varje kolumn. Den här importen skapar kraven en gång; använd ReqIF för upprepade uppdateringar.

### OSLC: länkar från ALM-verktyg

OSLC (Open Services for Lifecycle Collaboration) låter andra verktyg hitta kraven och testfallen i Qtestframe och länka till dem. Qtestframe är en OSLC-leverantör med läsbehörighet för Requirements Management 2.0 och Quality Management 2.0. Ge verktyget adressen till katalogen, `<serverns adress>/api/v1/oslc/catalog` (eller upptäcktsdokumentet `/api/v1/oslc/rootservices`), och en [API-nyckel](administration.md#api-nycklar). Verktyget ser då de projekt du får läsa, kan söka krav och testfall på nyckel, titel eller ändringsdatum och visar dem med sina länkar (vilka tester som verifierar ett krav, vilka krav ett test verifierar, relationer). Urvalsdialoger i det andra verktyget och att skapa poster via OSLC stöds inte. Din administratör hittar detaljerna under [Administration](administration.md#oslc).

## Vilket ska jag välja?

| Behov | Använd |
|---|---|
| Arbeta med data i Excel eller ett annat verktyg | **Export**, XLSX eller CSV |
| Lägga in data eller en rapport i en wiki, en pull request eller ett Git-förråd | **Export** eller **Rapporter**, Markdown |
| Lämna över krav till ett kravverktyg (DOORS, Polarion …) eller en annan Qtestframe | **Export**, ReqIF |
| Redigera många krav eller tester i Excel och hämta tillbaka ändringarna | **Importera → Exportera för redigering i Excel** och **Importera en redigerad Excel-arbetsbok** |
| Hålla krav som förvaltas i DOORS eller Polarion aktuella | **Importera → ReqIF från DOORS eller Polarion, med uppdateringar** |
| Låta ett ALM-verktyg länka till krav och tester | [OSLC](#oslc-länkar-från-alm-verktyg) |
| Ett specifikationsdokument för granskning eller underskrift | **Dokument → Skapa dokument → Kravspecifikation** (PDF eller Word, från en baslinje om den ska signeras); för en enkel tabell **Export**, DOCX |
| En testplan med testspecifikation | **Dokument → Skapa dokument → Testplan** |
| En statusrapport till ett styrgruppsmöte | **Rapporter → Projektstatus**, PDF |
| Underlag för en revision eller en release | **Rapporter → Kravtäckning** eller **Sammanfattning av körning**, sparad i arkivet, tillsammans med en [baslinje](baselines.md) |
| En återkommande lista med egna kolumner | En sparad **anpassad rapport** |
| Samma rapport i mottagarnas inkorg varje vecka | En sparad anpassad rapport under **Schemalagda rapporter** |
| Lämna över ett helt projekt, behålla en kopia eller utgå från en mall | **Spara projekt** och **Läs in projekt** (se [ovan](#spara-och-läs-in-projekt)) |
