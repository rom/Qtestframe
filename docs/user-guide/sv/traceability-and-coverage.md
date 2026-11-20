# Spårbarhet och täckning

Spårbarheten besvarar två frågor: *vilka testfall verifierar det här kravet?* och *har det verifierats med godkänt resultat?* Qtestframe besvarar dem med **länkar** mellan krav och testfall och med **täckningen** som härleds från testutfallen.

## Spårbarhetsmatrisen

Välj **Spårbarhet** i projektets sidopanel.

![Spårbarhetsmatrisen: krav i rader, testfall i kolumner, täckning per krav och senaste utfall per testfall](../images/sv-traceability.png)

- Kraven är **rader** och testfallen **kolumner**. En markerad (nedtryckt) cell betyder *länkad*.
- Det färgade fältet först på varje rad visar kravets täckning; **Förklaring av täckning** förklarar färgerna. Peka på ett testfalls kolumnrubrik för att se nyckel, titel och senaste utfall.
- Sammanfattningen anger hur många krav som är länkade, till exempel *14/20 krav länkade (70 %)*.
- **Filtrera krav** och **Filtrera testfall** tar emot [frågor](search-and-filters.md), till exempel `folder:Kassa` och `type=automated`.
- **Bara länkade testfall** döljer testfall som inte är länkade till något av de visade kraven.
- **Vy** växlar mellan **Matris** och **Lista**. Listan visar varje krav med dess länkade testfall och är lättare att läsa på en mobil.

**Länka och ta bort länkar:** klicka på (eller flytta fokus till och tryck <kbd>Enter</kbd> eller <kbd>Blanksteg</kbd> på) en tom cell för att länka kravet och testfallet; klicka på en markerad cell för att ta bort länken. Det kräver rollen **Testare**. Länkar kan också hanteras i kravredigeraren (**Länka testfall**) och testfallsredigeraren (**Länka krav**).

Matrisen rullar i båda riktningarna; kravkolumnen förblir synlig.

## Så beräknas täckningen

Täckningen härleds från det **senaste utfallet** för varje länkat testfall. Den beräknas varje gång du tittar på den och sparas aldrig, så den är alltid aktuell.

**Senaste utfall för ett testfall:** bland testfallets exekveringar i körningar i det här projektet som inte är *Ej körd*, den med den senaste tidpunkten (avslutad, annars startad, annars körningens skapandetid). Ett testfall utan någon sådan exekvering har senaste utfall **Ej körd**.

**Täckning för ett krav**, utvärderat uppifrån och ned:

| Täckning | När |
|---|---|
| **Ingen täckning** | inget testfall är länkat |
| **Underkänd** | minst ett länkat testfall är *Underkänd* eller *Blockerad* |
| **Godkänd** | alla länkade testfall är *Godkänd* |
| **Delvis** | minst ett länkat testfall är *Godkänd*, de övriga är *Ej körd*, *Överhoppad* eller *Pågår* |
| **Ej körd** | annars: testfall är länkade, men inget har godkänts eller underkänts än |

Exempel:

- REQ-1 är länkat till TC-1 (godkänd) och TC-2 (godkänd) → **Godkänd**.
- REQ-2 är länkat till TC-1 (godkänd) och TC-3 (ej körd) → **Delvis**.
- REQ-3 är länkat till TC-1 (godkänd) och TC-4 (blockerad) → **Underkänd**.
- REQ-4 har ett länkat testfall som aldrig har exekverats → **Ej körd**.
- REQ-5 har inga länkar → **Ingen täckning**.

Tänk på följande:

- Täckningen beräknas per krav, inte per utvärderingskriterium. En länk kan ange vilket kriterium ett testfall verifierar, men alla länkar räknas lika.
- Bara körningar i samma projekt räknas. Ett testfall som har flyttats från ett annat projekt behåller sina gamla exekveringar, men de räknas inte längre.
- Ett nyare utfall *Godkänd* ersätter ett äldre *Underkänd*; du behöver inte ta bort gamla körningar.

Täckningen visas som kolumn och snabbfilter i kravlistan, som frågefältet `coverage` (till exempel `coverage:failed` eller `coverage:none category:Security`), i matrisen, i exporter, i rapporter och på översikten.

## Översikten

**Översikt** är den första sidan i varje projekt.

![Översikten med nyckeltal, kravtäckning, utfall av exekveringar och de senaste körningarna](../images/sv-dashboard.png)

- **Nyckeltal**: antal krav (och hur många som saknar täckning), täckning (hur många som täcks av testfall), antal testfall (manuella och automatiserade), testkörningar (som pågår) och andelen godkända av alla exekveringar.
- **Kravtäckning**: hur många krav som är *Godkänd*, *Underkänd*, *Delvis*, *Ej körd* och *Ingen täckning*.
- **Utfall av exekveringar**: alla utfall i alla körningar.
- **Krav per status** och **Krav per kravnivå** (krav utan kravord räknas som *ospecificerade*).
- **Testfall per status**.
- **Senaste körningar**: de fem nyaste körningarna med förlopp; **Alla körningar →** öppnar listan över körningar.

På översikten räknas ett krav som *täckt* så snart minst ett testfall är länkat, oavsett utfall.

## Beroendegrafer

**Beroendegraf** i projektets sidopanel ritar hur objekten bygger på varandra. Välj **Krav** eller **Testfall** överst:

- **Krav**: relationerna mellan krav, som *beror på*, *tillhandahåller*, *förfinar*, *härlett från*, *står i konflikt med*, *duplicerar* och *relaterar till* (se [Relationer](requirements.md#relationer));
- **Testfall**: [beroendena](tests.md#beroenden) mellan testfall (*beror på*) och de delade stegen (*anropar*).

Grafen läses från vänster till höger: förutsättningar, överordnade krav och källor står till vänster, det som bygger på dem till höger. Varje ruta visar nyckel, titel och täckning (krav) eller senaste utfall (testfall), med en färgad list i samma färger som i resten av appen. Varje relationstyp har en egen färg och linjestil, förklarad vid dess kryssruta.

- Kryssa ur en relationstyp för att dölja de linjerna; objekt som då saknar relationer försvinner om inte **Visa objekt utan relationer** är på.
- Skriv i **Markera objekt…** för att markera matchande objekt.
- Högerklicka på ett objekt (eller ge det fokus och tryck <kbd>Blanksteg</kbd>) för att markera dess grannar; **Visa alla** avslutar det.
- Välj ett objekt (eller tryck <kbd>Enter</kbd>) för att öppna det. Zooma med förstoringsknapparna; rulla för att flytta dig.
- **Lista** visar samma relationer som en tabell, ett objekt per rad, vilket är lättare att läsa med skärmläsare eller att skriva ut.

Cykler (till exempel två krav som beror på varandra) ritas, men en av linjerna visas utan pil.
