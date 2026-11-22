# Testcykler

En **testcykel** planerar testningen för ett mål, till exempel *Regression sprint 14* eller *Acceptanstest release 2.1*. Den anger vilka testfall som ska köras (dess **plan**), i vilka konfigurationer, av vem och hur lång tid de bör ta. När cykeln startar skapar den sin [testkörning](runs.md) och följer sedan resultaten: planerade mot utförda, godkända, underkända och blockerade, uppskattad mot faktisk tid, en nedbränning och arbetsbelastningen för varje testare.

Välj **Testcykler** i projektets sidofält.

| Uppgift | Roll (behörighet) |
|---|---|
| Se cykler, deras plan och framsteg | alla roller som ser körningar (`run.read`) |
| Skapa och ändra cykler, planera testfall, tilldela testare, starta och avsluta cykler, återkommande cykler | **Testledare**, **Ansvarig**, **Administratör** (`cycle.plan`) |
| Skapa cykelns körningar | även `run.write` |
| Utföra de planerade testfallen | **Testare** och högre (`run.execute`), i cykelns körning som vanligt |

## Skapa en cykel

Välj **Ny cykel** och ange:

- ett **namn**, unikt i projektet, och gärna ett **mål**;
- **ägaren**, oftast testledaren (du själv som standard);
- **startdatum** och **slutdatum**; de avgränsar också nedbränningen;
- valfritt den **release**, **variant** och det **bygge** som cykeln testar, och dess **miljö**. Cykelns körningar får dem, så att resultaten räknas för den releasen och varianten;
- de **konfigurationer** som ska testas. Varje planerat testfall utförs då en gång per konfiguration, och varje konfiguration kan ha en egen testare;
- valfritt en tid då cykeln **startar automatiskt**: då startar den av sig själv och dess körning skapas;
- eller välj **Kopiera en tidigare cykel**: den nya cykeln får den cykelns plan, testare, uppskattningar, release, variant och konfigurationer. Det är det snabbaste sättet att få nästa sprints regression.

En ny cykel är **Planerad**.

## Planera testfall

Välj **Planera testfall** på fliken **Plan** och välj hur testfallen ska läggas till:

| Genom | Vad som läggs till |
|---|---|
| **Urval** | testfallen du väljer i testfallsväljaren |
| **Fråga** | alla testfall som matchar en [fråga](search-and-filters.md), till exempel `tag:regression priority:high` |
| **Mapp** | testfallen i en mapp, och om du vill i mapparna under den |
| **Regressionsurval** | testfallen som verifierar ändrade krav (och om du vill kraven som beror på, förfinar eller härleds från dem) med deras förutsättningar, som [regressionsurvalet](variants-and-code.md) på sidan Kod; för cykelns variant |
| **Tidigare cykel** | en annan cykels plan med testare, uppskattningar och konfigurationer |

Du kan välja en **testare** och en **uppskattning** för testfallen du lägger till, och konfigurationerna (utan val planeras ett testfall i alla cykelns konfigurationer). Testfall som redan är planerade i en konfiguration hoppas över.

**Uppskattningar.** Utan en egen uppskattning får ett planerat testfall testfallets **Uppskattade minuter** (ett fält i testfallet), annars medeltiden för dess tidigare exekveringar med ett utfall. Ändra uppskattningar och testare senare genom att markera rader i planen och välja **Tilldela** eller **Ange uppskattning**; **Ta bort** tar rader ur planen (utfall som redan är registrerade finns kvar i körningen).

Planen visar varje rad med konfiguration, testare, uppskattning, det senaste utfallet i cykelns körningar (välj det för att öppna exekveringen) och den faktiska tiden.

## Köra cykeln

Välj **Starta cykeln**. Cykeln blir **Pågår** och skapar sin körning: en exekvering per planerat testfall och konfiguration, tilldelad testaren, med cykelns release, variant, bygge och miljö. Testarna får en avisering och utför som vanligt i [exekveringsvyn](runs.md). Byter du testare i planen ändras också de exekveringar som ännu saknar utfall.

Testfall som planeras efter starten finns inte i någon körning ännu; knappen **Skapa körning för n nya testfall** skapar ytterligare en körning av cykeln för dem. **Avsluta cykeln** avslutar den; **Öppna igen** gör den pågående igen.

Ett planerat testfall räknas som **utfört** när det har ett slutligt utfall (godkänt, underkänt, blockerat eller överhoppat) i någon av cykelns körningar. Har ett testfall körts flera gånger räknas det senaste slutliga utfallet.

## Tavla per testare

Fliken **Tavla per testare** har en kolumn per testare (och en för testfall som ingen är tilldelad) med de öppna testfallen, uppskattad tid kvar och de utförda testfallen hopfällda. Välj ett testfall för att öppna dess exekvering.

## Framsteg

Nyckeltalen överst visar de planerade testfallen (och hur många som saknar testare), andelen utförda, utfallen, den uppskattade tiden med tiden kvar, och den faktiska tiden för de utförda testfallen mot deras uppskattningar.

Fliken **Framsteg** har:

- **nedbränningen**: per dag från startdatum till slutdatum de planerade testfall (eller den uppskattade tid) som inte var utförda vid dagens slut, mot den **ideala** linjen från hela planen den första dagen till ingenting den sista. Texten ovanför anger hur långt efter eller före den ideala linjen cykeln ligger i dag. Testfall som planerats senare syns från den dag de lades till;
- **arbetsbelastningen per testare**: planerade, utförda (per utfall) och öppna testfall per testare, med uppskattad tid, tid kvar och faktisk tid.

## Återkommande cykler

På fliken **Återkommande** i cykellistan skapar en **återkommande cykel** en ny cykel enligt ett schema – dagligen, varje vecka på en veckodag eller varje månad på en dag, vid en tid i en tidszon, som [schemalagda rapporter via e-post](reports-and-export.md). Varje ny cykel får:

- sitt **namn** från ett mönster: `{date}` blir startdatumet och `{n}` cykelns nummer, till exempel *Nattlig regression {date}*;
- fälten, planen, testarna och uppskattningarna från en **mallcykel**, och/eller testfallen som matchar en **fråga**;
- dagens datum som startdatum och den **längd** du anger;
- med **Starta varje ny cykel direkt** sin körning med en gång.

Cyklerna skapas som den återkommande cykelns **ägare**, som fortfarande måste få planera cykler i projektet. **Skapa nästa cykel nu** (spelaknappen) skapar en direkt. En återkommande cykel vars senaste körning misslyckades visar felet.

## Tidslinje

Fliken **Tidslinje** visar cyklerna med datum som staplar på en gemensam tidsaxel, färgade efter status.

## Filtrera cykler

Filterfältet i cykellistan tar [frågespråket](search-and-filters.md) med dessa fält:

| Fält | Matchar |
|---|---|
| `name`, `goal`, `description`, `environment` | cykelns text |
| `status` | `planned`, `active`, `closed` |
| `owner`, `assignee` | ägaren, någon testare (e-post eller namn) |
| `release`, `variant`, `build`, `configuration` | det cykeln testar |
| `start`, `end` | datumen, t.ex. `start>=2027-03-01` |
| `planned`, `executed`, `passed`, `failed`, `blocked`, `unassigned` | antal, t.ex. `failed>0` |
| `progress` | procent utförda, t.ex. `progress<50` |
| `test` | nyckeln till ett planerat testfall |

I testfallslistan hittar `cycle:"Sprint 14"` testfallen som är planerade i en cykel; i körningslistan hittar `cycle:` en cykels körningar.

## Releaser och rapporter

En release kan vänta på en cykel: utgångskriteriet **Testcykel genomförd** (se [utgångskriterier](releases-and-risks.md#utgångskriterier)) är uppfyllt när minst tröskeln – 100 % som standard – av cykelns planerade testfall har ett utfall.

**Testcykelrapporten** på sidan [Rapporter](reports-and-export.md) visar en cykels nyckeltal, arbetsbelastningen per testare, nedbränningen och hela planen med utfall, som PDF, Word, Excel, CSV eller Markdown.

## Historik och granskningslogg

Fliken **Historik** listar cykelns versioner – fälten och planen – med vem som ändrade vad. Att skapa, ändra, planera och ta bort cykler och återkommande cykler registreras i granskningsloggen. Cykler och återkommande cykler ingår i projektarkiv och säkerhetskopior; i ett inläst projekt är de återkommande cyklerna avstängda tills någon slår på dem.
