# AI-assistans

Qtestframe kan be en språkmodelltjänst om **förslag**: utkast till tester för ett krav, en bättre formulering av ett krav, en sammanfattning av en underkänd körning med utkast till defekter, ramverksmappningar och acceptanskriterier. Du granskar och redigerar varje förslag; **ingenting sparas förrän du godkänner det**.

AI-assistansen är valfri och **av som standard**. Den kräver en tjänst som en superadministratör ställer in, organisationen måste slå på den, och ett projekt kan stänga av den (se [Slå på och av](#slå-på-och-av)).

## Knappen Assistera

Sidor som kan använda AI-assistans har knappen **Assistera** med en gnistrande ikon:

| Var | Åtgärder |
|---|---|
| Kravredigeraren | **Förbättra formuleringen**, **Föreslå acceptanskriterier**, **Föreslå ramverksmappningar**, **Utkast till tester** |
| Testredigeraren | **Gör ett utkast till testet från** det första länkade kravet |
| Körningssidan | **Sammanfatta fel** |
| Riskdialogen | **Föreslå ramverksmappningar** |

**Assistera** öppnar en liten meny med åtgärderna. När en åtgärd inte kan användas syns den ändå, avstängd, med orsaken under: AI-assistansen är inte inställd eller avstängd, funktionen är avstängd, objektet är inte sparat ännu, körningen har inga fel, eller du har använt alla förfrågningar för den här timmen. Menyn visar också vilken tjänst texten skickas till och hur många förfrågningar du har kvar.

En åtgärd öppnar en dialog som frågar tjänsten direkt. Medan den skriver visar dialogen hur långt den har kommit; **Avbryt begäran** stoppar den. Sedan ser du förslagen, modellen, antalet token, tiden och, när administratören har angett priser, en uppskattad kostnad. **Fråga igen** skickar begäran en gång till.

Förslagen och promptarna är på användargränssnittets språk (engelska eller svenska).

## Utkast till tester från ett krav

**Utkast till tester** i ett sparat krav skickar dess titel, beskrivning och acceptanskriterier och föreslår upp till åtta manuella tester: titel, beskrivning, förutsättningar, testdata, förväntat resultat och steg med åtgärder, data och förväntade resultat, och vilka acceptanskriterier varje test täcker.

Varje test kan redigeras i dialogen: öppna detaljerna för att ändra texterna, redigera, lägga till eller ta bort steg. Bocka ur de tester du inte vill ha. **Skapa testutkast** skapar de markerade testerna med status *Utkast* och länkar vart och ett till kravet och till kriterierna det täcker. Testdatan läggs till förutsättningarna. De nya testerna syns i kravets [spårbarhet](traceability-and-coverage.md); granska dem som andra utkast innan du sätter dem till *Klar*.

I **testredigeraren** ber **Gör ett utkast till testet från** *NYCKEL* om utkast från testets första länkade krav. Välj ett utkast och **Använd det här utkastet**: dess titel (om testet saknar en), beskrivning, förutsättningar, förväntade resultat och steg läggs in i formuläret. Spara testet för att behålla dem.

## Förbättra ett krav

**Förbättra formuleringen** skickar kravets aktuella text (även när det inte är sparat ännu) tillsammans med fynden från [kravkontrollerna](requirements.md) och ber om en omskrivning som följer reglerna som kontrollerna letar efter: en förpliktelse med ett nyckelord i versaler (SKA, BÖR, KAN … eller MUST, SHOULD, MAY …), ett EARS-mönster där det passar (*När* … *SKA systemet* …), inga vaga ord, inga TBD, inga öppna listor eller undanflykter, aktiv form och mätbara värden.

Dialogen visar:

- **ändringarna** som en ordvis skillnad (tillagda ord understrukna, borttagna överstrukna) och förklaringen;
- de **åtgärdade fynden**: servern kontrollerar omskrivningen igen med samma kontroller, så listan visar vad omskrivningen verkligen åtgärdar, inte vad tjänsten påstår;
- de **fynd som finns kvar** och eventuella **nya fynd**, och kvalitetspoängen före och efter.

Redigera titeln och texten vid behov och välj sedan **Använd den här texten**: den ersätter texten i redigeraren. Den sparas när du sparar kravet, så du kan fortfarande jämföra och ångra. En platshållare som *[värde]* betyder att tjänsten behövde ett värde som den inte kunde veta.

## Acceptanskriterier

**Föreslå acceptanskriterier** föreslår verifierbara kriterier med en metod (test, inspektion, analys, demonstration, granskning) och, där kravet antyder en kvalitet, ett gränsvärde med enhet. Kriterier som kravet redan har utelämnas. Redigera eller bocka ur dem och välj **Lägg till kriterier**: de läggs till formulärets kriterier.

## Ramverksmappningar

**Föreslå ramverksmappningar** föreslår id i säkerhetsramverk med ett skäl för varje:

- för ett **krav**: MITRE D3FEND-tekniker och EMB3D-mitigeringar eller -hot, och CWE-svagheter;
- för en **risk**: MITRE ATT&CK- och ATLAS-tekniker och EMB3D-hot.

Varje id kontrolleras mot listorna som är inbyggda i Qtestframe; id som inte finns där tas bort, och dialogen säger hur många. Id som redan är mappade utelämnas. **Lägg till mappningar** lägger till de markerade id:na i formulärets fält för D3FEND, EMB3D eller ATT&CK; CWE-id läggs till kravets taggar (*CWE-79*), eftersom krav saknar ett CWE-fält. CWE-id är kända när [katalogen](catalogue.md) hänvisar till dem.

## Sammanfatta en underkänd körning

**Sammanfatta fel** på en körningssida skickar de underkända och blockerade utförandena (test, anteckningar, slutet av felutskriften, underkända steg; högst 40) och defekterna som är länkade till körningen. Du får en kort sammanfattning, troliga gemensamma orsaker (hypoteser utifrån datan) och utkast till defekter för misslyckanden som inget länkat fel täcker ännu.

Redigera utkastens titel, beskrivning och allvarlighetsgrad, bocka ur det du inte vill ha och välj **Skapa defekter**: varje defekt skapas och länkas till körningen och till utförandena av dess tester. Hantera dem sedan enligt [Defekter](defects.md).

## Integritet och vad som skickas

När du klickar på en åtgärd skickas texten i **det objektet** till AI-tjänsten som administratören har konfigurerat — ingenting annat, och ingenting i bakgrunden:

| Åtgärd | Vad som skickas |
|---|---|
| Utkast till tester, acceptanskriterier, förbättra formuleringen | kravets nyckel, titel, beskrivning, typ, förpliktelse, motivering och acceptanskriterier (vid förbättring: fynden från kontrollerna) |
| Sammanfatta fel | körningens namn, miljö och bygge, de underkända och blockerade utförandena med anteckningar, felutskrift och underkända steg, och de länkade defekterna |
| Ramverksmappningar | kravets eller riskens titel och beskrivning (risker: även kategori och åtgärd) och de id som redan är mappade |

Varje begäran registreras i organisationens händelselogg (funktion, användare, objekt, token) utan innehållet. Tjänsten svarar bara med text: den kan inte ändra något i Qtestframe, och objekttexten skickas till den som data, inte som instruktioner. Förslag kan ändå vara fel eller ofullständiga: granska dem som arbetet av en ny kollega.

## Slå på och av

- **Server:** en superadministratör konfigurerar tjänsten (Anthropic, eller en OpenAI-kompatibel tjänst som Azure OpenAI eller en lokal server) under **Administration → Systeminställningar → AI-assistans** och kan stänga av enskilda funktioner (se administratörshandboken).
- **Organisation:** en administratör (*settings.manage*) slår på den under **Administration → AI-assistans**.
- **Projekt:** en projektansvarig (*project.update*) kan stänga av den för ett projekt under **Inställningar → Projekt → AI-assistans**, till exempel när projektets data inte får lämna servern.

En gräns per användare och timme skyddar mot skenande kostnader; menyn Assistera visar hur många förfrågningar som finns kvar.
