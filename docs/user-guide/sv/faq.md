# Vanliga frågor

Svar på de frågor som användare oftast ställer: först om hur verktyget används, sedan, från [Testbegrepp](#testbegrepp) och framåt, om begreppen inom test och kravhantering, standarderna bakom Qtestframe och hur det fungerar. Använd sökfältet högst upp i handboken för att hitta ett ord på alla sidor; [Ordlistan](glossary.md) förklarar begreppen ett i taget.

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

Alla som kan läsa objektet, det vill säga alla medlemmar i organisationen med åtkomst till projektet, oavsett roll. Kommentarer syns inte för andra organisationer. Du kan redigera och ta bort dina egna kommentarer; moderatorer (som får ändra projektets inställningar) kan ta bort alla kommentarer, som då visas som borttagna. Kommentarerna tas bort tillsammans med sitt objekt. Se [Diskussioner](discussions.md).

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

## Testbegrepp

### Vad är skillnaden mellan ett testfall, en testkörning och en exekvering?

Ett **testfall** beskriver hur något ska kontrolleras: förutsättningar, steg och förväntade resultat, eller ett skript. En **testkörning** är en testomgång, till exempel *Regressionstest sprint 12*, i en viss miljö och ett visst bygge. En **exekvering** är ett testfall i en körning, med sitt utfall: samma testfall får en ny exekvering i varje körning det ingår i, och dess historik visar alla. Ett **teststeg** är en åtgärd i ett manuellt testfall med sitt förväntade resultat. Se [Testfall](glossary.md#testfall), [Testkörning](glossary.md#testkörning) och [Exekvering](glossary.md#exekvering).

### Vad är skillnaden mellan en testcykel, en testplan och en testsvit?

En **testplan** är dokumentet som anger vad som ska testas, hur, av vem och med vilka ingångs- och utgångskriterier; Qtestframe kan [ta fram en](documents.md#skapa-en-kravspecifikation-eller-en-testplan) ur projektet. En **testcykel** är den operativa planen för en testperiod: vilka testfall, vilka testare, vilka uppskattningar; när den startar skapar den sin körning. En **testsvit** är en grupp testfall för ett visst syfte; Qtestframe har inget separat svitobjekt, så gruppera testfallen med mappar, kategorier eller taggar och skapa körningar från en fråga. Se [Testcykel](glossary.md#testcykel), [Testplan](glossary.md#testplan) och [Testsvit](glossary.md#testsvit).

### Vilka testnivåer och testtyper stöder Qtestframe?

Alla: Qtestframe föreskriver inga nivåer (enhets-, integrations-, system-, acceptans- och end-to-end-test) eller typer (funktionell testning, prestanda, säkerhet, användbarhet, tillgänglighet, regression …). Ange dem där du behöver dem: som en kategoridimension som *Nivå / System* eller *Typ / Säkerhet*, som taggar, som mappar eller som separata körningar och cykler. Utfall från enhets- och integrationstest kommer oftast från CI, utfall från system- och acceptanstest från manuella körningar. Se [Testnivå](glossary.md#testnivå) och [Testtyp](glossary.md#testtyp).

### Hur använder jag testdesigntekniker som gränsvärdesanalys i Qtestframe?

Ta fram fallen med tekniken och gör dem sedan till **testdata**: en rad per ekvivalensklass, gränsvärde, regel i beslutstabellen eller parvis kombination, med parametrarna som `{{namn}}` i stegen. Varje rad blir en iteration med eget utfall. Ange tekniken som en kategori om du vill rapportera på den. Se [Datadrivna testfall](tests.md#datadrivna-testfall) och [Testdesignteknik](glossary.md#testdesignteknik).

### Vad är skillnaden mellan allvarlighetsgrad och prioritet?

**Allvarlighetsgraden** anger hur illa en defekts effekt är (från *Kritisk* till *Obetydlig*); **prioriteten** anger hur snart den ska åtgärdas (från *Kritisk* till *Låg*). Ett stavfel i företagsnamnet på startsidan är obetydligt men brådskande; en krasch i en sällan använd export kan vara kritisk men får vänta. Releasens utgångskriterier och reglerna för blockerande defekter använder allvarlighetsgraden. Se [Statusar och allvarlighetsgrader](defects.md#statusar-och-allvarlighetsgrader).

### Vad är skillnaden mellan verifiering och validering?

**Verifiering** kontrollerar att produkten uppfyller sina specificerade krav (”bygger vi produkten rätt?”); **validering** kontrollerar att den uppfyller sin avsedda användning (”bygger vi rätt produkt?”). I Qtestframe är ett krav *verifierat* när varje utvärderingskriterium har godkända bevis; validering visas oftast med acceptanstest, demonstrationer och utforskande sessioner med användare mot intressentkraven. Se [Verifiering](glossary.md#verifiering) och [Validering](glossary.md#validering).

### Vad är ett instabilt testfall, och hur hittar Qtestframe dem?

Ett testfall som växelvis godkänns och underkänns utan att något har ändrats. Qtestframe räknar växlingarna mellan godkänt och underkänt bland de tio senaste utfallen för varje testfall, manuella som automatiserade; två eller fler växlingar ger märket **Instabilt**. Hitta dem med `flaky:true`. Se [Instabila testfall](tests.md#instabila-testfall).

### Mäter Qtestframe kodtäckning?

Nej. Kodtäckning mäts av dina bygg- och testverktyg. Qtestframe mäter **kravtäckning**: vilka krav som har testfall och vilka deras senaste utfall är. Verktyget kopplar också incheckningar och byggen till krav och testfall, så att du ser vilka ändringar som har testats. Se [Kodtäckning](glossary.md#kodtäckning) och [Så beräknas täckningen](traceability-and-coverage.md#så-beräknas-täckningen).

### Vad är ingångs- och utgångskriterier i Qtestframe?

**Ingångskriterier** avgör när testningen får börja: per testfall kräver [exekveringsreglerna](runs.md#när-kan-ett-testfall-exekveras) att det är *Klart*, att dess krav är godkända och att dess förutsättningar är godkända. **Utgångskriterier** avgör när testningen är klar och en release får släppas: täckning, andel godkända, obligatoriska krav klarade, inga öppna defekter, risker hanterade med mera, på releasens flik [Utgångskriterier](releases-and-risks.md#utgångskriterier).

### Kan jag göra utforskande testning i Qtestframe?

Ja, med **sessionsbaserad testledning**: planera en session med uppdrag och omfattning, testa med tidtagning och typade anteckningar, skapa defekter från anteckningarna och avsluta med en utvärdering. Utvärderade sessioner kan räknas som verifieringsbevis. Se [Utforskande test](exploratory-testing.md).

### Hur passar Qtestframe ihop med CI/CD, TDD eller BDD?

Din pipeline kör de automatiserade testerna och laddar upp utfallen med `qtestframe report`, som JUnit XML eller med Playwright-rapportören; de hamnar i en körning, räknas in i täckningen och releasekriterierna och registrerar bygget. Tester som skrivs enligt TDD eller BDD kopplas till testfall i Qtestframe via testnyckeln. Se [Importera utfall från CI](runs.md#importera-utfall-från-ci), [Testdriven utveckling](glossary.md#testdriven-utveckling) och [Beteendedriven utveckling](glossary.md#beteendedriven-utveckling).

## Kravbegrepp

### Vad är skillnaden mellan ett krav och en specifikation?

Ett **krav** är ett enskilt verifierbart påstående om vad som behövs. En **specifikation** är den ordnade helheten: inledning, omfattning, definitioner och kraven, ofta för ett system eller en leverans. I Qtestframe är projektet den levande specifikationen, och **Skapa dokument → Kravspecifikation** gör ett dokument av det. Se [Specifikation](glossary.md#specifikation).

### Ska jag skriva SKA eller MÅSTE? Och hur förhåller de sig till MUST och SHALL?

På svenska skriver du **SKA** för krav, **BÖR** för rekommendationer och **FÅR** för det som är tillåtet, som i standarder från SIS; ett förbud skrivs **FÅR INTE**. Undvik *måste inte*, som betyder ”behöver inte” men ofta läses som ett förbud. Qtestframe förstår också **MÅSTE** och **SKALL** som absoluta krav. I engelska texter markerar både MUST och SHALL ett absolut krav enligt RFC 2119; välj ett och använd det konsekvent, med versaler, eftersom bara versalformerna har den normativa betydelsen (RFC 8174). Se [Kravnivåer](requirements.md#kravnivåer) och [SKA, BÖR och FÅR](glossary.md#ska-bör-och-får).

### Vad är skillnaden mellan funktionella och icke-funktionella krav?

Ett **funktionellt** krav säger vad systemet gör (”skickar en orderbekräftelse”); ett **icke-funktionellt** krav säger hur väl eller under vilka förhållanden (”inom 5 sekunder för 99 % av ordrarna”). Icke-funktionella krav behöver mätbara gränser; lägg dem i ett utvärderingskriterium. ISO/IEC 25010 är en bra checklista över kvalitetsegenskaper. Se [Icke-funktionellt krav](glossary.md#icke-funktionellt-krav).

### Vad är skillnaden mellan acceptanskriterier och utvärderingskriterier?

Acceptanskriterier är villkoren för att godta ett krav eller en användarberättelse. Qtestframe kallar dem **utvärderingskriterier** och ger varje kriterium en metod (test, inspektion, analys, demonstration, granskning) och ett tröskelvärde, så att det är tydligt hur varje kriterium ska visas. Se [Utvärderingskriterier](requirements.md#utvärderingskriterier).

### Vilken verifieringsmetod ska jag välja?

**Test** när beteendet kan exekveras med bestämda indata och förväntade resultat; **inspektion** för dokument, kod, märkning eller konfiguration; **analys** för beräkningar, simuleringar eller mätningar som utvärderas i efterhand; **demonstration** för att visa en funktion för en granskare utan detaljerade mätningar; **granskning** för en formell bedömning av kvalificerade personer. Andra metoder än test registreras som [verifieringsposter](traceability-and-coverage.md#verifieringsposter). Se [Verifieringsmetod](glossary.md#verifieringsmetod).

### Vad betyder dubbelriktad spårbarhet i Qtestframe?

Varje spårlänk kan följas åt båda hållen: från ett krav till dess testfall (framåt) och från ett testfall till dess krav (bakåt), i redigerarna och i spårbarhetsmatrisen. Relationer gör detsamma mellan krav (*förfinar* / *förfinas av*). Spårbarhetsregler kontrollerar båda riktningarna, till exempel ”varje godkänt krav har ett testfall” och ”varje testfall verifierar ett krav”. Se [Spårbarhet](glossary.md#spårbarhet).

### Vad är en misstänkt länk, och hur blir jag av med den?

En länk blir **misstänkt** när kravet eller testfallet i ena änden ändras i sak efter att länken bekräftades, eftersom den andra änden kanske inte längre passar. Granska det andra objektet, uppdatera det vid behov och bekräfta sedan länken: med **Bekräfta misstänkta** i spårbarhetsmatrisen eller **Bekräfta** på kravets flik **Påverkan**. Se [Ändringspåverkan](traceability-and-coverage.md#ändringspåverkan).

### Vad är skillnaden mellan förfinar och härlett från?

**Förfinar** är nedbrytning: barnkravet är en mer detaljerad del av sin förälder, och föräldern är verifierad först när barnen är det. **Härlett från** säger att ett krav följer av ett annat eller av ett designbeslut utan att vara en del av det, till exempel en tidsbudget för en komponent som härletts ur systemets svarstid. Båda gör det beroende kravet misstänkt när det andra ändras. Se [Relationer](requirements.md#relationer).

### När behöver jag ett undantag och när en avvikelse?

Ett **undantag** godtar att ett krav inte är uppfyllt (underkänt, otestat eller ej genomfört) för en release eller en period; en **avvikelse** godtar att det uppfylls med en annan lösning än den specificerade. Båda kräver ett signerat godkännande och kan ha ett slutdatum. Se [Undantag och avvikelser](waivers-and-soa.md#undantag-och-avvikelser).

### Hur skriver jag bra krav?

Ett krav per påstående, med tydligt subjekt, ett kravord, mätbara begrepp och inga reservationer; säg vad, inte hur; ange motivering och källa. [Språkanalysen](requirements.md#språkanalysen) kontrollerar många av reglerna medan du skriver, enligt INCOSE:s vägledning; ISO/IEC/IEEE 29148 räknar upp egenskaperna hos bra krav, och EARS ger meningsmönster. Se [Kravkvalitet](glossary.md#kravkvalitet).

### Hur arbetar jag riskbaserat?

Registrera riskerna i riskregistret, koppla de krav som hanterar dem och låt risknivån styra vad du specificerar och testar först: bygg releasens omfattning med `risk:high` och lägg till utgångskriterierna *Risker hanterade* och *Restrisk*. Se [Riskbaserad testning](glossary.md#riskbaserad-testning) och [Hur risker påverkar en release](releases-and-risks.md#hur-risker-påverkar-en-release).

## Standarder och format

### Vilka standarder följer Qtestframe?

För kravformuleringar RFC 2119 och RFC 8174 (och SIS praxis på svenska); för dokument ISO/IEC/IEEE 29148 (kravspecifikation) och 29119-3 (testplan); för utbyte ReqIF och OSLC; för utfall JUnit XML; för inloggning OpenID Connect, OAuth 2.0 med PKCE, SAML 2.0, SCIM 2.0 och TOTP; för API:et HTTP-semantik med ETag/If-Match och OpenAPI; för loggar syslog; för webbklienten WCAG 2.1 AA. Sidan [Referenser](references.md) listar dem med länkar; [Ordlistan](glossary.md#uppslagsord-efter-ämne) förklarar var och en.

### ReqIF eller OSLC: vilket ska jag använda?

**ReqIF** flyttar hela specifikationer som filer mellan verktyg, för överlämningar, utbyte med leverantörer och fram och tillbaka. **OSLC** kopplar ihop verktygen direkt: ett annat verktyg länkar sina objekt till Qtestframes krav och testfall och kan skapa och ändra dem utan fil. Många team använder båda. Se [Vilket ska jag välja?](interoperability.md#vilket-ska-jag-välja)

### Vilka format för testutfall kan Qtestframe importera?

**JUnit XML**, som nästan alla testramverk skriver, via API:et eller `qtestframe report`, och utfallen från **Playwright-rapportören**. Manuella utfall registreras i exekveringsvyn eller många på en gång på körningens sida. Se [Importera utfall från CI](runs.md#importera-utfall-från-ci).

### Vilka standarder för enkel inloggning stöds?

**OpenID Connect** (ovanpå OAuth 2.0, med PKCE) och **SAML 2.0**, med provisionering av användare via **SCIM 2.0**. Administratören sätter upp dem; se [Enkel inloggning (OIDC)](administration.md#enkel-inloggning-oidc) och [Provisionering med SCIM](administration.md#provisionering-med-scim).

### Stöder Qtestframe passkeys eller säkerhetsnycklar?

Inte för inloggning i Qtestframe: där används lösenord eller enkel inloggning, med TOTP som andra faktor. Med enkel inloggning kan din identitetsleverantör kräva passkeys eller säkerhetsnycklar. Katalogen har krav och testfall som hänvisar till WebAuthn för dina egna produkter. Se [WebAuthn och FIDO2](glossary.md#webauthn-och-fido2).

### Var kommer uppgifterna om MITRE ATT&CK ifrån?

Från MITRE:s publicerade data: katalogens listor för ATT&CK (Enterprise, ICS, Mobile), ATLAS och EMB3D genereras från deras **STIX 2.1**-paket, och D3FEND från dess publicerade data, utan återkallade eller utfasade poster. Namnen visas på engelska, som hos MITRE. Se [MITRE ATT&CK](catalogue.md#mitre-attck).

### Kan Qtestframe hjälpa till med ISO/IEC 27001, CRA eller AI-förordningen?

Ja. I katalogen väljer du de lagar och standarder som ett projekt ska uppfylla och lägger till de krav som hänvisar till dem, med testfall; efterlevnadsanalysen visar hur långt varje källa är verifierad, och sidan **SoA** täcker åtgärderna i ISO/IEC 27001 bilaga A. Qtestframe certifierar ingenting: det ordnar dina bevis. Se [Uppfyll lagar, standarder och bästa praxis](catalogue.md#uppfyll-lagar-standarder-och-bästa-praxis).

### Är Qtestframe självt tillgängligt?

Webbklienten siktar på WCAG 2.1 nivå AA: full användning med tangentbord, stöd för skärmläsare, högkontrast och andra färgteman, webbläsarzoom med en layout som anpassar sig, och mobillayouter, kontrollerat med automatiska tillgänglighetstester. Se [Tillgänglighet och språk](accessibility-and-languages.md).

## Så fungerar Qtestframe

### Var lagras mina data?

I en databas på organisationens server, SQLite eller PostgreSQL, även uppladdade filer. Hemligheter krypteras med en nyckel som förvaras utanför databasen. Se [Var dina data finns](how-it-works.md#var-dina-data-finns).

### Vad händer i bakgrunden?

Automatiserade testfall körs som jobb, notiser, webhooks och chattmeddelanden levereras när en ändring har sparats, sammanfattningar med e-post och schemalagda rapporter skickas i tid, trenderna i översikten registreras dagligen, och gamla sessioner och papperskorgen rensas. Se [Arbete i bakgrunden](how-it-works.md#arbete-i-bakgrunden).

### Hur skyddas mina data?

Organisationerna är isolerade från varandra, varje begäran kontrolleras mot din rolls behörigheter, sessionerna är skyddade mot förfalskade begäranden från andra webbplatser, tvåstegsverifiering och enkel inloggning finns, hemligheter är krypterade och går bara att skriva, och händelseloggen registrerar viktiga åtgärder. Se [Säkerhet](how-it-works.md#säkerhet).

### Kan skript på servern skada den?

Bara personer med behörigheten `script.execute` kan köra skript, och bara när administratören har aktiverat skriptkörningen. Varje skript körs i en sandlåda med resursgränser, utan nya privilegier, med filåtkomst begränsad av Landlock och systemanrop begränsade av seccomp. Webbtester behöver en webbläsare och nätverket, så deras sandlåda är svagare; administratören bör begränsa serverns utgående anslutningar. Se [Sandlådan för skript](how-it-works.md#sandlådan-för-skript).

### Vad är skillnaden mellan en säkerhetskopia, ett projektarkiv och en export?

En **säkerhetskopia** innehåller hela installationen och återställs av en superadministratör. Ett **projektarkiv** innehåller ett projekt, utan hemligheter, och läses in som ett nytt projekt. En **export** tar ut data i ett format för människor eller andra verktyg (PDF, Word, Excel, CSV, Markdown, ReqIF). Se [Säkerhetskopior och projektarkiv](how-it-works.md#säkerhetskopior-och-projektarkiv).

### Hur hittar jag ett ord som jag inte förstår?

Slå upp det i [Ordlistan](glossary.md), eller skriv det i sökfältet högst upp i handboken: sökningen omfattar alla sidor, också ordlistan.
