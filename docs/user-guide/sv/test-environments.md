# Testmiljöer och testdata

En **testmiljö** är ett system som dina tester körs mot: ett utvecklings- eller stagingsystem, ett förproduktionssystem, ett labb, en hardware-in-the-loop-rigg (HIL) eller en enhet. Qtestframe håller reda på vad varje miljö består av (dess **komponenter och versioner**), vem som äger den, om den kan användas just nu och vem som har **bokat** den när. Varje testresultat registrerar miljön det togs fram i och versionerna av dess komponenter i det ögonblicket, så att en release kan visa vilka miljöer och versioner som testades.

**Testdatamängder** är namngivna, versionshanterade mängder testdata: tabeller med parametrar som datadrivna tester kan dela, eller datafiler. Varje mängd har en klassning (syntetisk, anonymiserad, kopia av produktionsdata), en ägare och ett utgångsdatum.

Välj **Miljöer** i projektets sidomeny. Sidan Testdata nås därifrån.

| Uppgift | Roll (behörighet) |
|---|---|
| Se miljöer, deras bokningar och testdata | alla roller som ser körningar och tester (`run.read`, `test.read`) |
| Boka en miljö | **Testare** och uppåt (`run.execute`) |
| Skapa och ändra miljöer, godkänna bokningar, visa en sparad hemlighet | **Testledare**, **Förvaltare**, **Administratör** (`environment.manage`); för en miljö som delas av organisationen, i organisationsrollerna |
| Skapa och ändra testdatamängder | **Testare** och uppåt (`test.write`) |
| Be om uppdatering av testdata | alla som skriver tester eller kör testkörningar |

## Miljöer

Välj **Ny miljö** och ange:

- ett **namn**, unikt i projektet, och **typen**: utveckling, test, staging, förproduktion, produktionslik, labb, HIL-rigg eller enhet;
- **statusen**: *Tillgänglig*, *Används*, *Underhåll*, *Trasig* eller *Avvecklad*, med en **statusanteckning** (till exempel *Uppdatering av firmware till fredag*);
- **ägaren**, den som ansvarar för miljön (du som standard);
- **kapaciteten**: hur många bokningar som får överlappa. 1 betyder en i taget; en miljö av CI-containrar kan tillåta många;
- **Bokningar kräver godkännande**: bokningar väntar tills ägaren eller en miljöförvaltare godkänner dem;
- **adresserna**, en beskrivning och taggar;
- **komponenterna och versionerna**, till exempel *Webbapp 1.4.0*, *PostgreSQL 16.4*, *Terminalfirmware 5.0.1*;
- de **konfigurationer** och **varianter** den erbjuder;
- var **inloggningsuppgifterna** finns, och eventuellt en **hemlighet** (se nedan);
- eventuellt en **adress för hälsokontroll**.

Välj **Delad av organisationen** för att göra miljön tillgänglig för alla projekt i organisationen (till exempel ett enhetslabb som flera team använder). Bara den som förvaltar miljöer i organisationen kan göra det.

**Statustavlan** visar varje miljö som ett kort: dess status, komponenter, hur många bokningar den har just nu och nästa bokning, och resultatet av den senaste hälsokontrollen. Fliken **Lista** har samma miljöer i en tabell som du kan filtrera med [frågespråket](search-and-filters.md), till exempel `status:available component:PostgreSQL`, `type:hil_rig` eller `booked:true`. Välj en miljö för att öppna den.

### Komponenter och versioner

Håll versionerna aktuella när miljön uppgraderas. Ett resultat som registrerats i miljön behåller versionerna det registrerades med: att ändra miljön senare ändrar inte äldre resultat. Exekveringsvyn visar miljön och dess versioner bredvid resultatet, och **Releaser → Körningar → Testade miljöer** listar miljöerna och versionerna som releasens resultat kommer från (även i rapporten om releaseberedskap).

### Inloggningsuppgifter och hemligheter

**Var inloggningsuppgifterna finns** är en hänvisning: en sökväg i ett valv, en post i lösenordshanteraren, ett nyckelskåp. Använd den hellre än att spara hemligheten i Qtestframe.

Om du ändå sparar en **hemlighet** är den endast skrivbar: den krypteras som de andra hemligheter Qtestframe sparar och visas aldrig igen i dialogen. Miljöförvaltare kan välja **Visa** på miljöns sida; varje visning registreras i granskningsloggen. Ett projektarkiv (*Spara projekt*) utelämnar hemligheten.

### Hälsokontroll

Med en **adress för hälsokontroll** hämtar Qtestframe adressen i bakgrunden med några minuters mellanrum (**Kontrollera var**). Ett svar med en HTTP-status under 400 betyder *uppe*. När kontrollen misslyckas blir en *Tillgänglig* miljö eller en som *Används* **Trasig** (markerad *satt av hälsokontrollen*) och ägaren får en notifiering; när kontrollen lyckas igen blir den *Tillgänglig* igen. En status du satt för hand ligger kvar: hälsokontrollen ändrar inte *Underhåll*, *Avvecklad* eller *Trasig* satt för hand. Välj **Kontrollera nu** för att kontrollera direkt.

## Boka en miljö

Välj **Boka** på sidan Miljöer eller på en miljö, eller välj en dag i **Bokningskalendern**. Ange **Från**, **Till** och **syftet**.

Du kan också boka när du skapar en [körning](runs.md), en [testcykel](test-cycles.md) eller en [utforskande session](exploratory-testing.md): välj **Miljö** och kryssa i **Boka den för detta**. Bokningen kopplas till körningen, cykeln eller sessionen.

- En miljö tar emot så många överlappande bokningar som dess **kapacitet**. En bokning som inte får plats avvisas och dialogen listar bokningarna som är i vägen. Bokningar som bara möts (en slutar när nästa börjar) överlappar inte.
- När miljön kräver godkännande är bokningen **Väntar** tills ägaren eller en miljöförvaltare godkänner eller avslår den. De ser den i **Mitt arbete** (*Bokning att godkänna*) och får en notifiering; du ser din egen väntande bokning i Mitt arbete som *Väntar på godkännande* och får besked om beslutet. Ägare och förvaltare bokar utan godkännande.
- Att flytta en godkänd bokning av en miljö som kräver godkännande gör den väntande igen.
- Du kan avboka din egen bokning; ägare och förvaltare kan avboka alla bokningar av sin miljö.
- En avvecklad miljö kan inte bokas. En bokning av en delad miljö som gjorts för ett projekt du inte ser visar bara tiden.

**Bokningskalendern** visar en vecka: en rad per miljö, en stapel per bokning (väntande bokningar streckade). Välj en stapel för att se, flytta, avboka, godkänna eller avslå bokningen. Miljöns egen sida har samma kalender och dess kommande och tidigare bokningar.

## Miljöer för körningar, cykler och sessioner

Körningar, testcykler och utforskande sessioner väljer en **miljö** bland projektets miljöer (och de delade). Körningen behåller miljöns namn som sin miljötext, så filter som `environment:staging` fungerar som förut. En körning som skapas med en miljö i fritext (till exempel från CI eller en resultatimport) kopplas till miljön med det namnet om den finns. När du uppgraderade blev fritextmiljöerna i dina befintliga körningar och cykler miljöer i projektet.

En testcykels körningar får cykelns miljö.

## Testdatamängder

Öppna **Testdata** från sidan Miljöer. Välj **Ny datamängd** och ange:

- ett **namn**, **klassningen** och **sorten**: en **tabell med parametrar** eller **filer**;
- **ägaren**, ett **utgångsdatum** och taggar;
- för en tabell: **parametrarna** och **raderna**. Välj **Klistra in tabell** för att klistra in data från ett kalkylark (CSV, semikolon- eller tabbseparerat; första raden namnger parametrarna, en kolumn som heter *name* ger radernas namn);
- för filer: spara mängden och lägg sedan till filerna på fliken **Filer**.

**Klassning.** *Syntetiska* data är påhittade; *anonymiserade* data kommer från produktion med personuppgifterna utbytta; en **produktionskopia** kan innehålla personuppgifter eller konfidentiell information och visas med en varning. Begränsa åtkomsten till den, följ era dataskyddsregler och ta bort den när den går ut.

**Versioner.** Varje ändring av en mängd är en ny version (se fliken **Historik**).

### Dela data mellan tester

Koppla mängden till tester, konfigurationer och körningar på fliken **Kopplingar**. Kryssa i **Som datakälla för testerna** när du väljer datadrivna tester: ett sådant test hämtar då sina parametrar och rader från mängden i stället för sina egna (se [datadrivna tester](tests.md)). När en körning skapas med testet blir varje rad i mängden en iteration, och fliken **Användning** registrerar vilken körning som använde vilken version. Testets redigerare visar vilken delad mängd det hämtar sina rader från. En arkiverad mängd används inte: testet använder då sina egna rader.

### Uppdateringsbegäran och utgång

Alla som skriver tester eller kör körningar kan **be om uppdatering** på fliken **Uppdateringsbegäran** (vad som ska uppdateras och när det behövs). Ägaren får en notifiering och ser begäran i **Mitt arbete** (*Uppdatering begärd*); ägaren markerar den **Klar** eller **Avslår** den, och den som bad får besked.

En mängd vars utgångsdatum har passerats markeras **Utgången**, och en som går ut inom 14 dagar **Går snart ut**. Båda syns i ägarens Mitt arbete, så att data uppdateras, förlängs eller arkiveras i tid.

## Notifieringar och händelser

| Händelse | Vem som notifieras |
|---|---|
| `environment.booking_requested` | miljöns ägare och förvaltare |
| `environment.booking_approved`, `environment.booking_rejected` | den som bokade |
| `environment.status_changed` | ägaren (även när hälsokontrollen ändrar statusen) |
| `testdata.refresh_requested` | datamängdens ägare |
| `testdata.refresh_resolved` | den som bad om uppdateringen |

Händelserna kan också skickas till [webhooks och chattkanaler](reviews-and-notifications.md).

## Säkerhetskopior och projektarkiv

Miljöer, bokningar, testdatamängder, deras kopplingar, användning och uppdateringsbegäranden ingår i säkerhetskopior. Ett projektarkiv innehåller projektets egna miljöer och deras bokningar samt projektets testdatamängder; delade miljöer tillhör organisationen och stannar kvar (körningarna behåller namnet), liksom sparade hemligheter.
