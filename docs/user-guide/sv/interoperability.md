# Samverkan: ReqIF och OSLC

Qtestframe fungerar tillsammans med krav- och ALM-verktyg som IBM DOORS och DOORS Next (IBM Engineering Lifecycle Management), Siemens Polarion, Codebeamer, Jama och ProR på två sätt:

- **ReqIF-filer** flyttar hela specifikationer mellan verktyg: krav, testfall, mappar, relationer, spårlänkar och bilagor. Du exporterar en fil här och importerar den där, eller tvärtom, så ofta du vill.
- **OSLC** kopplar ihop verktygen direkt: ett annat verktyg hittar Qtestframes krav och testfall, länkar sina egna poster till dem och kan skapa och ändra dem utan någon fil.

Den här sidan förklarar båda. Excel fram och tillbaka och de andra exportformaten beskrivs i [Rapporter och export](reports-and-export.md#utbyta-data-med-andra-verktyg); din administratör kopplar ihop verktygen enligt administratörshandboken (*Koppla ihop med andra verktyg*).

## ReqIF-export

**Export** (i projektet) har **ReqIF** och **ReqIF (med bilagor)**. En ReqIF-fil innehåller alltid kraven med sina mappar (som kapitelrubriker), egna fält, acceptanskriterier, D3FEND- och EMB3D-id och relationer; vid import läses D3FEND- och EMB3D-attributen tillbaka. Under **Innehåll i ReqIF** lägger du till mer:

| Val | Det här tillkommer |
|---|---|
| **Testfall med steg och spårlänkar** | Varje testfall som ett objekt av typen *Test case* (titel, beskrivning, förutsättningar, stegen som en tabell, förväntat resultat, exekveringstyp, prioritet, status, uppskattning, taggar, D3FEND och EMB3D, egna fält), testmapparna som ett andra kapitelträd och en relation *Verifies* från varje testfall till varje krav som det verifierar. |
| **Verifieringsposter** | Granskningar, analyser, demonstrationer och genomgångar som har registrerats för kraven, var och en relaterad till sitt krav (*Verification record of*). De är till för att läsas i det andra verktyget; en import av filen hoppar över dem. |
| **Version** | *Nuvarande läge*, eller en av projektets [baslinjer](baselines.md): kraven och testfallen exakt som baslinjen frös dem. Filtret gäller inte för en baslinje; relationer, länkar och poster är dagens mellan dess poster. |

Filtret begränsar kraven som för de andra formaten; med testfall tas bara de testfall med som verifierar ett exporterat krav. **ReqIF (med bilagor)** är ett `.reqifz`-arkiv med bilagorna och bilderna i beskrivningarna som filer.

Filen använder ReqIF-standardnamnen (*ReqIF.Name*, *ReqIF.Text*, *ReqIF.ChapterName*, *ReqIF.ForeignID*) som DOORS, DOORS Next, Polarion och Codebeamer känner igen, och värdelistor (uppräkningar) för kravnivå, typ, prioritet, status och egna fält med val. Varje objekt behåller samma identifierare i varje export, så att ett verktyg som har importerat den tidigare uppdaterar sin kopia.

## ReqIF-import

**Importera → ReqIF från DOORS eller Polarion, med uppdateringar** (i krav- eller testfallslistan) läser en `.reqif`- eller `.reqifz`-fil och visar vad som skulle ändras innan något sparas *(Manager)*.

1. **Välj filen.** Förhandsgranskningen visar dokumentets objekttyper, attribut och relationstyper med vad Qtestframe gör av dem, och ändringarna.
2. **Objekttyper.** Varje typ av objekt i dokumentet blir **Krav**, **Testfall**, **Rubrik (mapp)** eller hoppas över. Qtestframes egen typ *Test case* och typer med *test* i namnet gissas som testfall, allt annat som krav; välj annorlunda för det andra verktygets typer (till exempel artefakter av typen *Test Case* i DOORS Next, arbetsposter av typen *Test Case* i Polarion eller trackers av typen *Test Case* i Codebeamer). Att importera testfall kräver behörighet att skriva testfall.
3. **Attribut.** För krav och testfall var för sig läses varje attribut in i fältet som visas: inbyggda fält och projektets [egna fält](requirements.md#egna-fält) (som matchas på sin etikett; värdelistor matchas på valens namn). **Teststeg** läser en tabell (åtgärd, data, förväntat resultat) eller en lista. **Rubrik (mapp)** markerar kapitelnamnen, **ID i det andra verktyget** sparas i källan för nya krav. Välj ett annat fält eller *ignorera*; förhandsgranskningen uppdateras direkt.
4. **Relationstyper.** Varje relationstyp blir en relationstyp i Qtestframe (*förfinar*, *beror på* …), en **spårlänk** (ett testfall verifierar ett krav) eller hoppas över. En relation mellan ett testfall och ett krav blir alltid en spårlänk, oavsett riktning.
5. **Var posterna hamnar.** Välj målmapparna för krav och för testfall, och om **rubriker blir mappar**. Med **Krav som ligger under ett krav förfinar det** får ett krav som ligger under ett annat krav i dokumentets hierarki (som i DOORS) en relation *förfinar* till det.
6. **Kontrollera ändringarna** för krav och för testfall (nya, ändrade, oförändrade, konflikter, fel, cell för cell), relationerna och spårlänkarna som läggs till eller tas bort och de inbäddade filerna, och välj **Spara**. Allt sparas på en gång, eller ingenting.

Det här händer:

- **Formaterad text** (XHTML) blir Markdown med sina stycken, listor, tabeller, fet och kursiv text och länkar.
- **Inbäddade filer** i en `.reqifz` (bilder och andra objekt i texten, *Attachments* i en export från Qtestframe) blir bilagor till posten, och texten visar bilden från bilagan. En fil som posten redan har bifogas inte igen.
- **En ny import av samma dokument** uppdaterar kraven, testfallen, relationerna och spårlänkarna som det skapade och lägger till nya. En post som har ändrats i Qtestframe sedan förra importen visas som en konflikt när dokumentet har andra värden. Relationer och spårlänkar som har tagits bort i det andra verktyget tas bort här också; de som du har skapat i Qtestframe finns kvar. En import tar aldrig bort poster.
- Ett teststeg som anropar ett testfall som är nytt i samma import importeras som ett vanligt steg; importera dokumentet en gång till för att göra det till ett anrop.

### Fram och tillbaka

En ReqIF-fil som har exporterats från Qtestframe med testfall kan importeras till ett annat projekt (eller ett annat Qtestframe) och innehåller sedan samma poster: krav och testfall med fält, egna fält (skapa samma egna fält i målprojektet först), kriterier, steg, mappar, relationer, spårlänkar och bilagor. De ursprungliga nycklarna sparas som ID i det andra verktyget; posterna får nycklar i målprojektet. Om du importerar filen till **samma** projekt matchas varje post, så att du kan skicka en specifikation till en leverantör och importera deras redigerade version tillbaka med samma förhandsgranskning.

## OSLC

OSLC (Open Services for Lifecycle Collaboration) är standarden som ALM-verktyg använder för att länka poster mellan verktyg. Qtestframe är en OSLC-leverantör för **Requirements Management 2.1** (krav) och **Quality Management 2.1** (testfall). När din administratör har kopplat in ett verktyg kan dess användare:

- **Välja** krav eller testfall i Qtestframe i ett litet urvalsfönster i sitt eget verktyg, söka på text och länka till dem: till exempel länka ett krav i DOORS Next till testfallet i Qtestframe som validerar det, eller en arbetspost i Polarion till kravet som den implementerar.
- **Skapa** ett krav eller testfall i Qtestframe från sitt verktyg, i ett kort formulär, och länka det i samma steg.
- **Läsa** posterna med egenskaper och länkar (vilka testfall som verifierar ett krav, vilka krav det förfinar, vilka arbetsposter som implementerar det), söka i dem och följa länkarna till sidan i Qtestframe.
- **Ändra och ta bort** poster via OSLC, med samma regler som i Qtestframe: kontroller, granskningsbehörigheter, historik och granskningsloggen. Om någon har ändrat posten under tiden får verktyget veta att det ska läsa den igen i stället för att skriva över den andra ändringen.

Urvals- och skapafönstren använder din session i Qtestframe i webbläsaren. Om du inte är inloggad visas en länk för att logga in i en annan flik; välj sedan **Försök igen**.

**Länkar.** OSLC-länkar motsvarar Qtestframes egna länkar:

| OSLC-länk | I Qtestframe |
|---|---|
| *validatedBy* (krav), *validatesRequirement* (testfall) | spårlänk: testfallet verifierar kravet |
| *elaborates* / *elaboratedBy* | relationen *förfinar* |
| *constrainedBy* / *constrains* | relationen *beror på* |
| *satisfies* / *satisfiedBy* | relationen *härledd från* |
| *relation* | relationen *relaterar till* |
| *implementedBy*, *trackedBy*, *affectedBy*, *testsChangeRequest* och länkar till poster i det andra verktyget | en [länk](requirements.md#länkar) till postens adress, med länkens namn som etikett |

Varje verktyg ansluter med en [API-nyckel](administration.md#api-nycklar) för en person eller ett tjänstekonto; verktyget kan göra det som nyckelns roll tillåter och ser bara projekt som den personen får se.

## Vilket ska jag välja?

| Du vill … | Använd |
|---|---|
| Lämna en hel specifikation till en leverantör eller kund, eller ta emot deras | ReqIF-export och -import |
| Hålla en specifikation som underhålls i DOORS, DOORS Next, Polarion eller Codebeamer aktuell i Qtestframe | ReqIF-import, igen efter varje ändring där |
| Kopiera ett projekt med testfall till ett annat Qtestframe | ReqIF med testfall (eller [Spara och läs in projekt](reports-and-export.md#spara-och-läs-in-projekt)) |
| Frysa det som lämnades över | ReqIF-export av en baslinje |
| Länka poster i ett ALM-verktyg till krav och testfall medan du arbetar | OSLC |
