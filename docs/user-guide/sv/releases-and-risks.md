# Releaser och risker

En **release** är en version av produkten som du planerar, testar och levererar, till exempel *Vår 2027* eller *2.1.0*. Den samlar de krav som ska levereras (releasens **omfattning**), de testkörningar som testar den och de **utgångskriterier** som avgör om den får släppas. **Riskregistret** listar vad som kan gå fel, hur sannolikt det är, hur allvarlig konsekvensen blir och vilka krav som hanterar varje risk. Öppna risker påverkar direkt en releases utgångskriterier.

Releaser och risker finns i projektets sidopanel.

| Åtgärd | Roll som krävs |
|---|---|
| Visa releaser, deras status och riskregistret | Läsare |
| Skapa, redigera och ta bort releaser, ändra omfattning och utgångskriterier, markera som släppt | Ansvarig |
| Skapa, redigera och ta bort risker och länka krav till dem | Ansvarig |
| Lägga en körning i en release | Testare |

## Listan över releaser

**Releaser** listar projektets releaser med version, status (*Planerad*, *Aktiv*, *Släppt* eller *Inställd*), måldatum, antal krav och körningar och en etikett **Go** eller **No-go**. Välj en release för att öppna detaljerna under listan.

Skapa en release med **Ny release**. Ange ett namn och gärna version, status, måldatum och beskrivning. En ny release får standardkriterierna:

- täckning minst 100 %,
- varje MUST- och SHALL-krav har klarat sina testfall,
- minst 95 % av kraven har klarat alla sina testfall,
- inga underkända testfall,
- inga misstänkta länkar.

## Omfattning

Fliken **Omfattning** listar de krav som releasen ska leverera, med status, täckning och risknivå.

- Välj **Lägg till krav** för att välja krav i en lista.
- Eller skriv ett filter i frågespråket, till exempel `tag:kassa` eller `risk:high`, och välj **Lägg till matchande**: alla krav som matchar läggs till. Syntaxen beskrivs i [Sökning och filter](search-and-filters.md).
- Välj **×** på en rad för att ta bort kravet ur omfattningen.

Ett krav kan ingå i flera releaser.

## Körningar i en release

En körning hör till en release när du väljer releasen under **Release** i dialogen **Ny testkörning** eller i **Redigera körning** på körningens sida. Välj **Ingen release** för att ta bort den igen. Listan **Testkörningar** visar varje körnings release i kolumnen **Release**, och körningens sida visar den bredvid miljö och bygge.

Bara utfallen i releasens egna körningar räknas för utgångskriterierna: ett testfall som blev godkänt i en körning för en annan release gör inte den här releasen godkänd.

Fliken **Körningar** listar releasens körningar med status och förlopp; välj en körning för att öppna den.

## Utgångskriterier

Fliken **Utgångskriterier** listar de villkor som releasen måste uppfylla. Välj **Lägg till kriterium**, välj en typ och, om typen har en sådan, en tröskel eller nivå; välj papperskorgen för att ta bort ett kriterium. Välj **Spara kriterier** för att behålla ändringarna.

| Kriterium | Uppfyllt när | Parameter |
|---|---|---|
| Täckning | minst tröskeln av kraven i omfattningen har ett testfall | tröskel i % |
| Klarade testfall | minst tröskeln av kraven har klarat alla sina testfall i releasens körningar | tröskel i % |
| Obligatoriska krav klarade | varje MUST- och SHALL-krav i omfattningen har klarat sina testfall | — |
| Exekverat | minst tröskeln av exekveringarna i releasens körningar har ett utfall | tröskel i % |
| Inga underkända testfall | inget testfall är underkänt eller blockerat i releasens körningar | — |
| Godkända krav | minst tröskeln av kraven är godkända, implementerade eller verifierade | tröskel i % |
| Inga misstänkta länkar | inget krav i omfattningen har en [misstänkt länk](traceability-and-coverage.md) | — |
| Inga öppna defekter | ingen öppen defekt med minst den valda allvarlighetsgraden är länkad till ett krav i omfattningen eller dess testfall | lägsta allvarlighetsgrad |
| Risker hanterade | varje risk på minst den valda nivån som ett krav i omfattningen hanterar är åtgärdad, accepterad eller stängd | lägsta risknivå |

En release utan utgångskriterier är alltid go.

## Go eller no-go

Fliken **Status** utvärderar utgångskriterierna just nu. Den visar **Go** när alla kriterier är uppfyllda och annars **No-go**, hur många kriterier som är uppfyllda och en sammanfattning av omfattningen (krav, hur många som har testfall, godkända, underkända och ej körda) och körningarna.

Varje kriterium visar om det är uppfyllt, det faktiska värdet bredvid målet (till exempel *80 % (mål ≥ 95 %)*) och det som blockerar kriteriet: krav utan testfall, underkända testfall, öppna defekter eller risker som fortfarande kräver åtgärd. Varje objekt är en länk som öppnar det.

Etiketten i listan över releaser bygger på samma utvärdering, så att du direkt ser vilka releaser som är klara.

## Markera en release som släppt

Välj **Markera som släppt** på en öppen release.

- Är releasen **go** bekräftar du, och releasen markeras som släppt.
- Är den **no-go** listar dialogen de kriterier som inte är uppfyllda. Du kan ändå släppa releasen genom att **åsidosätta** kriterierna: skriv varför i **Varför släppa ändå?** (minst 3 tecken) och välj **Släpp ändå**. Motiveringen sparas med releasen och i händelseloggen.

När en release markeras som släppt skapar Qtestframe [baslinjen](baselines.md) *Release* följt av releasens namn: den fryser kraven i omfattningen och deras testfall i de versioner de hade. Releasen visar sedan när och av vem den släpptes, eventuell motivering och en länk till baslinjen.

## Riskregistret

**Risker** listar projektets risker. Varje risk har en nyckel som *SHOP-RISK-3*, en titel, en kategori, en ägare, en **sannolikhet** och en **konsekvens** från 1 till 5, en status och en **åtgärd**: det ni gör för att göra risken mindre sannolik eller mindre skadlig.

| Sannolikhet | | Konsekvens | |
|---|---|---|---|
| 1 | Sällsynt | 1 | Försumbar |
| 2 | Osannolik | 2 | Liten |
| 3 | Möjlig | 3 | Måttlig |
| 4 | Trolig | 4 | Stor |
| 5 | Nästan säker | 5 | Allvarlig |

Registret visar varje risks poäng och nivå, status, ägare, de krav som hanterar risken och hur många av dem som är **täckta** (har testfall) och **verifierade** (alla testfall godkända). Filtrera på text, nivå eller status. Stängda risker döljs om du inte väljer **Ta med stängda risker** eller filtrerar på statusen *Stängd*.

Lägg till en risk med **Ny risk**. Välj sannolikhet och konsekvens (dialogen visar poäng och nivå medan du väljer), beskriv åtgärden och välj **Länka krav** för att välja de krav som hanterar risken. Välj en risk i registret för att ändra den.

Kravredigeraren visar under **Risker** de risker som kravet hanterar, och kravlistan har de valfria kolumnerna **Risk** och **Öppna defekter** (se **Kolumner**).

### Ändra många risker på en gång

Kryssa i risker i registret. Verktygsfältet anger **status**, **sannolikhet** eller **konsekvens** för alla, **Ägare / kategori** anger det ena eller båda (tomt rensar), **Länka krav** länkar samma krav till varje markerad risk, och **Ta bort** tar bort dem efter bekräftelse. Bara rader som visas förblir markerade, så när du filtrerar registret eller väljer en ruta i värmekartan blir inga dolda risker kvar i urvalet. Alla risker kontrolleras innan någon ändras. Andra ändringar än länkning och borttagning kan ångras med ångra-knappen.

## Värmekartan

Värmekartan räknar riskerna i ett rutnät med 5 × 5 rutor: sannolikhet från 5 överst till 1 nederst, konsekvens från 1 till vänster till 5 till höger. Rutans färg är nivån för dess poäng. Välj en ruta för att bara visa riskerna med den sannolikheten och konsekvensen i registret; välj den igen, eller **×** på filteretiketten, för att visa alla.

Bredvid värmekartan räknar **Öppna risker per nivå** de risker som inte är stängda, per nivå; välj en nivå för att filtrera registret.

## Risknivåer

En risks **poäng** är sannolikhet × konsekvens, från 1 till 25. Poängen ger **nivån**:

| Nivå | Poäng |
|---|---|
| Låg | 1–4 |
| Medel | 5–9 |
| Hög | 10–16 |
| Kritisk | 17–25 |

Statusarna *Identifierad* och *Åtgärdas* betyder att risken fortfarande kräver åtgärd; *Åtgärdad*, *Accepterad* och *Stängd* betyder att den inte gör det.

## Hur risker påverkar en release

Ett krav får nivån för den högsta **öppna** risk som det hanterar. Du kan filtrera krav på den med `risk:critical` eller `risk:high`, och på öppna defekter med `defects>0`.

Utgångskriteriet **Risker hanterade** med nivån *Hög* blockerar en release så länge ett krav i omfattningen hanterar en risk på nivå hög eller kritisk som fortfarande är *Identifierad* eller *Åtgärdas*. Statusfliken listar de riskerna; åtgärda (eller acceptera) dem och ändra deras status, så blir kriteriet uppfyllt. Ett typiskt arbetssätt:

1. Registrera riskerna och länka de krav som hanterar dem.
2. Lägg kraven i releasens omfattning.
3. Lägg till **Risker hanterade** med den nivå ni kan acceptera i utgångskriterierna.
4. Testa kraven i releasens körningar; när åtgärderna fungerar sätter du riskerna till *Åtgärdad*.
