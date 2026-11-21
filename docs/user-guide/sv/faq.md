# Vanliga frågor

Svar på de frågor som användare oftast ställer. Använd sökfältet högst upp i handboken för att hitta ett ord på alla sidor.

## Åtkomst

### Jag har glömt mitt lösenord. Vad gör jag?

Det finns ingen självbetjäning för lösenord. Be en **administratör** i din organisation att [återställa ditt lösenord](administration.md#återställa-ett-lösenord). Du får ett tillfälligt lösenord som du måste byta vid nästa inloggning. Om du loggar in med enkel inloggning (en knapp under *eller fortsätt med*) hanteras lösenordet av företagets identitetsleverantör; återställ det där.

### Varför avslutades min session?

En session avslutas när:

- den når sin längsta livslängd (12 timmar som standard, inställt av administratören) och du måste logga in igen;
- du, eller någon med tillgång till ditt konto, valde **Logga ut** för den under **Profil → Aktiva sessioner**, eller **Logga ut alla andra sessioner** på en annan enhet;
- en administratör återställde ditt lösenord, angav ett nytt eller inaktiverade ditt konto;
- en superadministratör återställde en säkerhetskopia, vilket loggar ut alla;
- webbläsaren tog bort sina kakor.

Logga in igen. Om du inte väntade dig det, byt lösenord och kontrollera dina aktiva sessioner.

### Jag ser ”Du har ingen aktuell organisation”. Vad nu?

Du är inloggad men arbetar inte i någon organisation. Välj en i organisationsväljaren i övre fältet. Om listan är tom är du ännu inte medlem i någon organisation; be en administratör att lägga till dig.

### Hur byter jag till en annan organisation?

Använd organisationsväljaren till vänster i övre fältet (på mobiler byggnadsikonen). Qtestframe öppnar organisationens projektlista. För att alltid börja i en viss organisation anger du **Standardorganisation** i dina inställningar.

### En kollega ser inte ett projekt. Varför?

Projektet är troligen **begränsat** (det visas med ett lås 🔒): bara dess projektmedlemmar och organisationens administratörer ser det. Någon som får ändra projektet lägger till din kollega med **Dela** i projektets sidhuvud; se [Delning av projekt](administration.md#delning-av-projekt). Projektrollen kan också vara lägre än organisationsrollen, så en kollega kan se ett projekt men inte få ändra i det.

## Redigering

### Hur hänvisar jag till en kollega i ett krav eller en kommentar?

Skriv `@` följt av början på namnet och välj personen i listan, se [Nämn personer](requirements.md#nämn-personer). Listan innehåller organisationens användare och namn som redan används som ägare eller tilldelad i projektet. `mention:` i ett filter hittar det som nämner någon.

### Var kommer de föreslagna taggarna ifrån?

Från objektets titel och beskrivning. Taggar som projektet redan använder kommer först när texten nämner dem, sedan förkortningar och vanliga ord i texten. Vardagliga ord och kravord föreslås aldrig. Välj ett förslag för att lägga till det; strunta i de andra.

### Varför kan jag inte redigera ett krav?

Att skriva krav kräver rollen **Ansvarig** i den aktuella organisationen. Med **Läsare** eller **Testare** öppnas redigeraren skrivskyddad, och knappar som **Nytt krav** och **Spara** döljs. Kontrollera din roll i användarmenyn och be en administratör om fler rättigheter om du behöver det. Testare kan ändå länka testfall till krav och kommentera dem.

### Varför ser jag inte knappen Kör skript?

Att köra skript på servern kräver rollen **Testledare** eller **Ansvarig** (behörigheten *script.execute*), och administratören måste ha aktiverat skriptexekveringen (`runner.enabled`). Utan rollen visar exekveringsvyn att behörigheten *script.execute* krävs. Om exekveraren är avstängd på servern använder du [CI-importen](runs.md#importera-utfall-från-ci) i stället.

### Hur ångrar jag en ändring?

Välj **Ångra** (↶) i övre fältet eller tryck <kbd>Ctrl</kbd>+<kbd>Z</kbd> utanför ett textfält; **Ångrahistorik** (🕘) listar dina senaste ändringar. Det fungerar för redigeringar, borttagningar, flyttar, massändringar, länkar med mera, se [Ångra och gör om](getting-started.md#ångra-och-gör-om). Ett borttaget krav eller testfall kommer tillbaka med sin nyckel, sina länkar, kommentarer och sin historik.

För äldre ändringar öppnar du objektet, går till fliken **Versionshistorik**, väljer den version du vill ha och väljer **Återställ den här versionen**. Det skapar en ny version med det gamla innehållet; inget går förlorat och du kan återställa igen.

### Kan Qtestframe skriva kod eller testfall med AI?

I ett **mjukvaruprojekt** bygger **AI-prompter** en prompt från dina krav eller testfall (implementera dem, granska dem, ta fram eller automatisera testfall, planera en testtyp) som du kopierar till den AI-assistent din organisation använder. Qtestframe anropar själv ingen AI-tjänst. Se [AI-prompter](requirements.md#ai-prompter-mjukvaruprojekt).

### Varför kan jag inte godkänna ett krav eller markera ett testfall som klart?

Att godkänna eller avvisa ett krav kräver behörigheten *requirement.review*, att markera ett testfall som **Klart** kräver *test.review*. Som standard har rollerna **Granskare** och **Ansvarig** båda och **Testledare** har *test.review*; en **Testare** kan skriva testfall men inte markera dem som klara. Utan behörigheten är statusarna gråade i redigeraren. Se [Roller och behörigheter](administration.md#roller-och-behörigheter).

### Kan två personer redigera samma krav samtidigt?

Ja, men den som sparar sist vinner: varje sparning ersätter fälten med det personen ser. Båda versionerna finns i **Versionshistorik**, så inget går förlorat; jämför dem där och återställ vid behov. Använd **Kommentarer** för att samordna er.

### Varför uppdaterades inte kravnivån när jag ändrade texten?

Kravnivån identifieras automatiskt bara när kravet **skapas** med **Identifiera automatiskt från texten**. Efter det ändras den aldrig utan att du vet om det. Använd **Använd identifierad kravnivå** i analysrutan eller välj nivån själv.

### Vad händer när jag flyttar ett krav till ett annat projekt?

Det får en **ny nyckel** i målprojektet (till exempel blir `SHOP-REQ-7` `CORE-REQ-31`); kategorier matchas på dimension och namn och tas bort om målprojektet saknar värdet; **länkar till testfall tas bort**, eftersom länkar aldrig går mellan projekt; kriterier och historik följer med. Inom ett projekt ändrar en flytt bara mappen. Se [Flytta krav och mappar](requirements.md#flytta-krav-och-mappar).

### Vem kan se mina kommentarer?

Alla som kan läsa objektet, det vill säga alla medlemmar i organisationen med åtkomst till projektet, oavsett roll. Kommentarer syns inte för andra organisationer. Du kan redigera och ta bort dina egna kommentarer; administratörer kan ta bort alla kommentarer. Kommentarerna tas bort tillsammans med sitt objekt.

### Vad betyder ”auto-generated” på ett testfall?

Testfallet skapades automatiskt tillsammans med ett krav (**Skapa även länkat testfall**). Det är ett vanligt testfall: förfina stegen, ändra status och ta bort taggen om du vill.

## Täckning och utfall

### Hur beräknas täckningen?

Utifrån det **senaste utfallet** för varje testfall som är länkat till kravet: inget länkat testfall → *Ingen täckning*; något länkat testfall underkänt eller blockerat → *Underkänd*; alla godkända → *Godkänd*; några godkända och resten ej körda, överhoppade eller pågående → *Delvis*; annars *Ej körd*. Bara körningar i samma projekt räknas. Detaljer och exempel finns under [Så beräknas täckningen](traceability-and-coverage.md#så-beräknas-täckningen).

### Vad betyder ”Ej körd”?

För ett **testfall** eller en **exekvering**: det finns inget utfall än. För ett **kravs täckning**: testfall är länkade, men inget av dem har godkänts eller underkänts i någon körning i projektet än. Det skiljer sig från *Ingen täckning*, som betyder att inget testfall alls är länkat.

### Ett testfall godkändes igen; varför är mitt krav fortfarande ”Underkänd”?

Täckningen använder det senaste utfallet för **varje** länkat testfall. Om ett annat länkat testfall fortfarande är underkänt eller blockerat förblir kravet *Underkänd*. Öppna kravets **Länkade testfall** för att se varje testfalls senaste utfall. Kontrollera också att det nya utfallet sparades (**Spara** i exekveringsvyn).

### Varför säger översikten ”täckta” när testfallen inte har körts?

På översikten betyder *täckt* att minst ett testfall är länkat. Om testfallen har godkänts visas i **Kravtäckning** bredvid.

### Hur importerar jag utfall från CI?

Skapa en API-nyckel (**Administration → API-nycklar**, roll Testare eller högre), öppna körningen och välj **CI-import** för ett färdigt kommando, och skicka utfallen per testnyckel från din pipeline. Se [Importera utfall från CI](runs.md#importera-utfall-från-ci).

### En CI-import rapporterar ”unknown” testnycklar. Varför?

De nycklarna finns inte i körningens projekt. Kontrollera stavningen (versaler och gemener spelar ingen roll) och att testfallen inte har flyttats till ett annat projekt, vilket ger dem nya nycklar.

### Varför är Godkänd och Underkänd avstängda i exekveringsvyn?

Testfallet får inte exekveras än: det är inte **Klart**, ett krav det verifierar är inte godkänt, eller ett testfall det beror på har inte godkänts. Den röda rutan ovanför stegen listar varje orsak med vad du kan göra och vilken inställning som avgör: om **testfallet** inte är Klart (fortfarande Utkast eller under Granskning), om ett **krav** det verifierar inte har någon av de statusar som exekveringsinställningarna kräver (till exempel Godkänt när inställningarna kräver Implementerat), eller om ett **förutsättande test** inte har godkänts. Åtgärda dem (eller be en administratör eller testledare ändra [inställningarna för testexekvering](administration.md#inställningar-för-testexekvering)); under tiden kan du markera testfallet som **Blockerat** eller **Överhoppat**.

### Hur skapar jag en egen roll?

*(Administratör)* **Administration → Rollmatris → Ny roll**: ge den ett namn, välj vilken roll den ska börja med behörigheterna från, kryssa sedan i och ur behörigheter i matrisen och spara. Se [Ändra roller och skapa egna](administration.md#ändra-roller-och-skapa-egna).

### Kan jag ändra ett utfall efter att jag har sparat det?

Ja. Öppna exekveringen i exekveringsvyn, ändra stegens utfall eller det samlade utfallet och spara. Tidpunkten och personen uppdateras. Testfallets exekveringshistorik visar det senaste läget för varje exekvering.

## Baslinjer och versionshistorik

### Vad är skillnaden mellan en baslinje och en version?

En **version** är ett läge för ett enskilt objekt och skapas automatiskt vid varje ändring (fliken **Versionshistorik**). En **baslinje** är en namngiven ögonblicksbild av versionerna av alla (eller filtrerade) objekt i ett projekt vid ett visst tillfälle, som du skapar själv, till exempel inför en release. Se [Baslinje eller version?](baselines.md#baslinje-eller-version).

### Tas kraven bort om jag tar bort en baslinje?

Nej. En baslinje registrerar bara versioner. Om du tar bort den förblir alla objekt oförändrade.

## Sökning och filter

### Varför är min fråga ogiltig?

Oftast ett fält som inte finns i listan (till exempel `coverage` för testfall), ett blanksteg efter operatorn (`status: approved`), ett citattecken, en parentes eller ett reguljärt uttryck som inte avslutas, eller en funktion i reguljära uttryck som RE2 inte stöder. Fältet markeras och meddelandet anger positionen. Öppna **Hjälp för frågespråket** (**?** bredvid filtret) för att se fälten. Kom ihåg att fält och värden skrivs på engelska. Se [Varför är min fråga ogiltig?](search-and-filters.md#varför-är-min-fråga-ogiltig).

### Hur hittar jag krav utan testfall?

Filtrera kraven med `coverage:none` eller `-linked:*`. Kombinera gärna, till exempel `coverage:none obligation=must`.

### Söker sökningen i alla projekt?

Sökningen i övre fältet omfattar alla projekt i den **aktuella organisationen** som du kan läsa. Filter i en lista gäller bara den listan. Andra organisationer genomsöks aldrig.

## Inställningar och enheter

### Vad betyder ett fält?

Peka på det lilla **i** bredvid fältet, eller flytta tangentbordsfokus dit: en kort förklaring visas, med en länk till det avsnitt i handboken som beskriver fältet. Tryck <kbd>?</kbd> för att öppna handbokens sida för den aktuella vyn. Standarderna bakom kravnivåer och test finns under [Referenser](references.md).

### Hur byter jag språk eller tema?

Språk: **Användarmeny → Språk** (English eller Svenska). Tema: **Färgtema** i övre fältet, eller sol-/måneknappen för ljust och mörkt. Båda finns också under **Profil → Inställningar** och sparas med ditt konto, så de följer med till alla enheter.

### Kan jag använda Qtestframe på en mobil?

Ja. Gränssnittet anpassar sig efter mobiler och surfplattor: sidopanelen blir en utfällbar panel bakom menyknappen, tabeller blir kort och redigerare öppnas i helskärm. Att exekvera manuella testfall på en surfplatta fungerar bra. Se [Mobiler och surfplattor](accessibility-and-languages.md#mobiler-och-surfplattor).

### Hur startar jag introduktionsturen igen?

Välj **Hjälp** (frågetecknet i övre fältet) → **Starta introduktionsturen igen**.

### Kan jag ändra nyckeln för ett projekt?

Nej. Nyckeln ingår i varje objekts id och kan inte ändras när projektet har skapats. Du kan ändra projektets namn, beskrivning och färg under **Kategorier & inställningar** (eller med ✎ i projektlistan). Om du verkligen behöver en annan nyckel, använd **Duplicera projekt** i projektlistan: kopian får den nya nyckeln och alla objektnycklar följer med (se [Spara och läs in projekt](reports-and-export.md#spara-och-läs-in-projekt)). Ta sedan bort originalet.

## Filer och data

### Var lagras filerna?

I Qtestframes databas, tillsammans med övriga data. Filerna ingår därför i säkerhetskopior, skyddas av projektets behörighetsregler och tas bort med projektet. Se [Dokument](documents.md#var-lagras-filerna).

### Hur stor får en uppladdad fil vara?

25 MB per fil som standard; administratören kan ändra gränsen. Profilbilder och projektikoner får vara högst 1 MB.

### Varför laddas en fil ned i stället för att öppnas?

Bara bilder (PNG, JPEG, GIF, WebP), PDF-filer och vanlig text visas i webbläsaren. Alla andra typer laddas ned, så att ingen fil kan köra kod i din webbläsare.

### Hur återställer jag en säkerhetskopia?

En **superadministratör** öppnar **Administration → Säkerhetskopiering**, väljer filen med säkerhetskopian under **Återställning**, skriver `RESTORE` och väljer **Återställ säkerhetskopia**. Alla aktuella data ersätts och alla loggas ut. Säkerhetskopian måste komma från samma schemaversion. Serveradministratörer kan använda `qtestframe restore --in FIL.zip --yes`. För att få tillbaka ett enskilt objekt använder du i stället dess versionshistorik.

### Hur flyttar jag ett projekt till en annan server?

Välj **Spara projekt** (nedladdningsknappen i projektlistan) och, på den andra servern, **Läs in projekt** med filen. Kopian har allt i projektet utom hemligheter som webhook-hemligheter och felhanteringssystemets token; se [Spara och läs in projekt](reports-and-export.md#spara-och-läs-in-projekt). För att flytta alla organisationer på en gång använder en superadministratör i stället en säkerhetskopia.

### Kan jag få ut mina data ur Qtestframe?

Ja: **Export** laddar ned krav, kriterier, testfall, spårbarhet och körningar som XLSX, CSV, PDF eller DOCX, och alla funktioner finns också i REST-API:et med en API-nyckel.
