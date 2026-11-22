# Ändringsbegäranden och ändringshantering

När ett krav är godkänt, eller fryst i en signerad baslinje, bör en ändring vara ett beslut och inte en redigering. **Ändringshantering** gör den till det: en **ändringsbegäran** föreslår nya värden för ett eller flera krav och tester, säger varför, visar vad mer ändringen påverkar, beslutas av projektets **ändringsnämnd** med elektroniska signaturer och genomförs sedan i ett steg, med vanliga versioner och historik.

| Åtgärd | Roll som krävs |
|---|---|
| Se ändringsbegäranden | Läsare |
| Föreslå ändringar; redigera, skicka in, bedöma, skicka till nämnden, genomföra, verifiera och stänga ändringsbegäranden | Ämnesexpert, Testledare, Förvaltare (`change.create`) |
| Besluta om ändringsbegäranden som nämndmedlem; nödåtgärd | Testledare, Förvaltare (`change.approve`) |
| Ändra projektets inställningar för ändringshantering | Förvaltare (`project.update`) |

För att genomföra en ändringsbegäran krävs också behörighet att redigera posterna den ändrar (`requirement.write`, `test.write`, och granskningsbehörigheten för en granskningsstatus).

## Ändringshantering i ett projekt

Under **Inställningar → Projekt → Ändringshantering** väljer du hur strikt projektet hanterar ändringar:

- **Av** (standard): poster ändras direkt, som utan ändringshantering. Ändringsbegäranden kan ändå användas.
- **Varna**: skyddade poster kan fortfarande ändras direkt, men redigeraren säger att de omfattas av ändringshantering och ändringen flaggas.
- **Tvinga**: skyddade poster kan inte ändras direkt. Att spara dem, en massändring, kalkylbladsvyn, en Excel- eller ReqIF-import, att återställa en äldre version eller en baslinje och att ta bort avvisas med *Inte sparat: posten omfattas av ändringshantering*, och redigeraren erbjuder att **föreslå ändringen** i stället.

En post är **skyddad** när dess status är en av de **skyddade statusarna** (standard *Godkänt*, *Implementerat*, *Verifierat* och, för tester, *Klar*) eller, med **Signerade baslinjer låser sina poster**, när den finns i en baslinje som har en [signatur](baselines.md). Granskningsbeslut och statusen som följer bevisen påverkas inte.

**Ändringsnämnden** är en lista med medlemmar (e-postadresser) och roller. Bara de beslutar om ändringsbegäranden, och bara om de också har `change.approve`; om ingen anges beslutar alla med `change.approve`. **Beslutsförheten** anger hur många godkännanden som behövs: *all* (varje angiven medlem och en innehavare av varje angiven roll), *any* (en medlem) eller ett antal. **Signaturens betydelse** är meningen en nämndmedlem signerar, till exempel *Godkänd för release av ändringsnämnden*. **Att stänga en ändringsbegäran skapar en baslinje som standard** förväljer det valet när en ändringsbegäran stängs.

### Nödåtgärd

När en rättning inte kan vänta på nämnden kan den som har `change.approve` spara en skyddad post direkt med en **nödåtgärd** och en motivering på minst tio tecken (i API:t `?change_override=<motivering>`). Ändringen sparas som vanligt, och granskningsloggen registrerar `change_control.override` med posten, varför den var skyddad och motiveringen.

## Föreslå en ändring

Det finns tre sätt att lägga en ändring i en ändringsbegäran:

- **I redigeraren**: öppna ett krav eller test, ändra fälten som vanligt och välj **Föreslå ändring** under formuläret i stället för **Spara**. Fälten du ändrade (titel, beskrivning, förpliktelse, typ, prioritet, status, källa, motivering, ägare, taggar och egna fält; för tester också förutsättningar, förväntat resultat, utförande och uppskattning) blir förslaget. Ingenting sparas på posten.
- **För flera**: markera poster i listan och välj **Föreslå ändring** i markeringsraden, sedan ny status, prioritet, förpliktelse eller utförande och ägare för alla.
- **Efter ett avvisat sparande**: med *tvinga* öppnar ett sparande av en skyddad post samma dialog med dina ändringar.

I dialogen väljer du **En ny ändringsbegäran** (titel, motivering, kategori och prioritet) eller **Ett av mina utkast** för att lägga ändringen i ett utkast du fortfarande skriver. Dialogen visar varje ändring som en skillnad: borttagen text överstruken, tillagd text markerad.

Poster med öppna ändringsbegäranden har en **ÄB**-märkning i listorna, och redigeraren visar deras öppna ändringsbegäranden. I [frågespråket](search-and-filters.md) hittar `cr:open` (eller `has:cr`) poster med en öppen ändringsbegäran och `cr:DEMO-CR-3` posterna i en.

## Arbetsflödet

**Ändringsbegäranden** i projektets sidomeny listar dem med nyckel (*PROJ-CR-n*), status, kategori, prioritet, poster och den som begärde. Öppna en för att se de föreslagna ändringarna post för post och fält för fält, påverkan, signaturerna och tidslinjen.

1. **Utkast**: den som begär skriver ändringsbegäran. **Kategori** är en av *Rättelse*, *Förbättring*, *Regelkrav*, *Defektdriven*, *Riskdriven* och *Övrigt*. Koppla defekterna, riskerna och ärendena ändringen kommer från, välj vad genomförandet gör med [ändringspåverkan](traceability-and-coverage.md) (*Markera misstänkta*, *Meddela ägare* eller *Ingetdera*) och om stängning skapar en baslinje.
2. **Skicka in**: kräver minst en post och en motivering. Den transitiva ändringspåverkan av de föreslagna ändringarna beräknas och **sparas på ändringsbegäran** som en ögonblicksbild, med vägen till varje påverkad post, så att nämnden beslutar om det som bedömdes.
3. **Bedöm påverkan**: en bedömare lägger till anteckningar och en uppskattad insats; påverkan beräknas igen. *Skicka sedan till ändringsnämnden* för den vidare direkt.
4. **Under beslut**: nämndens medlemmar meddelas, och den visas i deras [Mitt arbete](reviews-and-notifications.md) som *Ändringsbeslut*.
5. **Besluta**: en nämndmedlem väljer **Besluta…**, väljer *Godkänn*, *Avslå* eller *Skjut upp*, skriver en kommentar (krävs för avslag och uppskjutande) och bekräftar med sitt lösenord. Det är en **elektronisk signatur** över ändringsbegärans exakta innehåll med projektets signaturbetydelse. Ett avslag eller uppskjutande beslutar direkt; godkännanden beslutar när beslutsförheten nås. Den som begärde ändringen kan inte besluta om den och meddelas om beslutet.
6. **Genomför**: en godkänd ändringsbegäran genomförs i ett steg. Varje post får de föreslagna värdena genom det vanliga sparandet, så validering, behörigheter, versioner, historik och granskningsloggen är desamma som vid en redigering; sedan hanteras påverkan en gång, med det valda alternativet. Om en post inte kan sparas sparas ingenting. Om en post har ändrats sedan ändringen föreslogs är ändringsbegäran *inaktuell* och kan varken beslutas eller genomföras: återta den och föreslå ändringen igen.
7. **Verifiera** och **Stäng**: någon bekräftar att ändringen fungerar som avsett och stänger den sedan, om man vill med en ny baslinje av projektet.

En avslagen eller uppskjuten ändringsbegäran kan **återtas** till utkast och skickas in igen, eller stängas. Utkast och avslagna ändringsbegäranden kan tas bort. Varje steg finns på **tidslinjen** med vem, när och varför.

## Rapporter, aviseringar och integrationer

- Rapportmallen **Ändringsbegäranden** (under **Rapporter**) ger en ändringsbegäran (de föreslagna ändringarna fält för fält, påverkan, signaturerna och tidslinjen) eller registret över alla; **PDF-rapport** i ändringsbegäran gör detsamma för den.
- Webhooks och chattkanaler kan prenumerera på `change_request.submitted`, `change_request.review`, `change_request.approved`, `change_request.rejected`, `change_request.deferred`, `change_request.implemented` och `change_request.closed`.
- Ändringsbegäranden ingår i projektarkiv och säkerhetskopior.

## I demot

Demoprojektet har ändringshanteringen *varna*, med administratören och förvaltarna som nämnd. Saras ändringsbegäran *Lock accounts for 30 minutes instead of 15* väntar på administratörens beslut i Mitt arbete; det finns också en stängd ändringsbegäran vars ändring har genomförts, en uppskjuten och ett utkast.
