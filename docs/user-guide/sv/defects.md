# Defekter

En **defekt** (bugg) är ett problem som hittats vid testning. Varje defekt har en nyckel som `SHOP-BUG-3`, en titel, en beskrivning, en status, en allvarlighetsgrad, en prioritet och en tilldelad person, och den kopplas till de krav, testfall och exekveringar den gäller. Defekterna finns under **Defekter** i projektets sidomeny.

Defekter kan speglas i **Jira** eller **GitHub Issues**: Qtestframe skapar ärendet åt dig och läser tillbaka dess status, så att utvecklarna arbetar i sin ärendehanterare medan testteamet ser varje defekts läge bredvid testfallen.

| Åtgärd | Roll som krävs |
|---|---|
| Visa defekter | Läsare |
| Rapportera och redigera defekter, koppla dem, skicka, synkronisera och importera ärenden | Testare |
| Ta bort defekter, ansluta ärendehanteraren | Ansvarig |

## Listan över defekter

**Defekter** listar projektets defekter, öppna först och de allvarligaste överst. Kolumnerna visar nyckel, titel, status, allvarlighetsgrad, tilldelad person, de kopplade objekten och ärendet i ärendehanteraren med dess egen status.

Filtrera med snabbfiltren **Status** och **Allvarlighetsgrad**, eller med [frågespråket](search-and-filters.md), till exempel `open:true severity:critical`, `assignee:anna*` eller `linked:SHOP-REQ-12`. Fälten är `key`, `title`, `description`, `status`, `severity`, `priority`, `assignee`, `external` (ärendets nyckel i ärendehanteraren), `created_by`, `open` (`true` för *Öppen* och *Pågår*) och `linked` (nycklar för kopplade krav och testfall).

Välj en defekt för att öppna och redigera den. Välj **Ny defekt** för att rapportera en.

## Ändra många defekter på en gång

Kryssa i defekter i listan (rubrikens kryssruta markerar hela sidan). Verktygsfältet ändrar alla på en gång:

- **Ange status** och **Ange allvarlighetsgrad**, samt **Tilldela…** för att ge dem till någon (tomt tar bort tilldelningen). Den nya ansvariga får en notis. Kräver behörighet att spara utfall (`run.execute`).
- **Skicka till Jira/GitHub** skapar ett ärende för varje markerad defekt som saknar ett; defekter som redan har ett ärende lämnas orörda. **Synkronisera** läser status för de markerade defekternas ärenden. Båda visas när en ärendehanterare är ansluten.
- **Ta bort** (behörighet att ändra projektet), efter bekräftelse.

Alla defekter kontrolleras innan någon ändras, så ett felaktigt värde ändrar ingenting. Ändringar av status, allvarlighetsgrad och tilldelning kan ångras med ångra-knappen i övre fältet.

## Statusar och allvarlighetsgrader

| Status | Betydelse |
|---|---|
| **Öppen** | Rapporterad, ingen arbetar med den än |
| **Pågår** | Någon åtgärdar den |
| **Åtgärdad** | Rättad, väntar på att verifieras av ett nytt test |
| **Stängd** | Verifierad eller klar |
| **Avvisad** | Ingen defekt, en dubblett eller kommer inte att åtgärdas |

*Öppen* och *Pågår* räknas som **öppna**. Öppna defekter visas på krav och testfall och stoppar en release (se [Öppna defekter och releaser](#öppna-defekter-och-releaser)).

| Allvarlighetsgrad | Typisk användning |
|---|---|
| **Kritisk** | Krasch, dataförlust, säkerhetsproblem, ingen tillfällig lösning |
| **Allvarlig** | En viktig funktion fungerar inte (standard) |
| **Mindre** | Fungerar med en tillfällig lösning, eller en liten funktion påverkas |
| **Obetydlig** | Kosmetiskt, t.ex. ett stavfel |

Allvarlighetsgraden säger hur illa problemet är; **prioriteten** (kritisk, hög, medel, låg) säger hur snart det ska åtgärdas.

## Koppla defekter till objekt

En defekt kan kopplas till valfritt antal **krav**, **testfall** och **exekveringar** i projektet. I defektdialogen, under **Kopplade objekt**, väljer du **Koppla krav** eller **Koppla testfall** för att välja objekt, eller **Koppla exekvering** för att välja en körning och en av dess exekveringar. Ta bort en koppling med ×.

Fliken **Defekter** för ett krav eller testfall visar dess defekter:

- ett **testfall** visar defekterna som är kopplade till det och till dess exekveringar i alla körningar (*via en exekvering*);
- ett **krav** visar defekterna som är kopplade till det, till dess testfall och till deras exekveringar (*via ett testfall*).

Där kan du välja **Rapportera defekt** (en ny defekt som redan är kopplad till objektet) eller **Koppla befintlig** (välj en defekt i projektet). × tar bort en direkt koppling. Ett krav med öppna defekter visar en röd markering som *2 öppna defekter* bredvid nyckeln; välj den för att öppna fliken.

Defektnycklar fungerar som andra nycklar: `SHOP-BUG-3` i en beskrivning eller kommentar blir en länk, och `/go/SHOP-BUG-3` öppnar defekten.

Defekt-id:n som skrivs i fältet **Defekter** för en exekvering (den äldre fritextlistan, se [Defekter i exekveringsvyn](runs.md#defekter)) fungerar som förut. När ett sådant id är en defektnyckel i verktyget, till exempel `SHOP-BUG-3`, länkar det till själva defekten.

## Rapportera en defekt från en misslyckad exekvering

När ett testfall misslyckas i [exekveringsvyn](runs.md#exekveringsvyn) visas en notis med **Rapportera defekt** vid utfallet. Den öppnar defektdialogen ifylld med:

- testfallets nyckel och titel som titel;
- körningen, de misslyckade stegen med förväntat och faktiskt resultat samt exekveringens anteckningar som beskrivning;
- en koppling till exekveringen (och därmed till testfallet och dess krav).

Justera texten, ange allvarlighetsgrad och välj **Rapportera defekt**. Den nya defekten visas på fliken **Defekter** i exekveringsvyn, och testfallet och dess krav visar den också. Fliken har även **Koppla befintlig** för en defekt som redan är känd.

## Anslut Jira eller GitHub

En användare med rollen **Ansvarig** ansluter projektet till en ärendehanterare under **Kategorier & inställningar**, fliken **Projekt**, kortet **Ärendehanterare för defekter**. Välj **Ärendehanterare** och fyll i fälten; **Testa anslutningen** sparar inställningarna och kontrollerar adressen, projektet eller repot och token.

### Jira

- **Jira-webbplatsens adress**: webbplatsens adress, t.ex. `https://example.atlassian.net`, eller adressen till er Jira Server eller Data Center.
- **Jira-projektnyckel**, t.ex. `SHOP`, och **Ärendetyp** för nya ärenden (standard *Bug*).
- **Jira-kontots e-postadress** och **Token**: för Jira Cloud kontots e-postadress och en **API-token** som skapas på id.atlassian.com under *Säkerhet, API-tokens*. För Jira Server eller Data Center lämnar du e-postadressen tom och använder en **personlig åtkomsttoken**.

Kontot behöver behörighet att bläddra i projektet och skapa ärenden i det.

### GitHub Issues

- **Repo** som `ägare/repo`.
- **API-adress**: tom för github.com; för GitHub Enterprise Server t.ex. `https://github.example.com/api/v3`.
- **Etiketter** som läggs till på nya ärenden, separerade med komma, t.ex. `bug, qtestframe`.
- **Token**: en **finkornig personlig åtkomsttoken** (fine-grained) med åtkomst till repot och repobehörigheten **Issues: Read and write** (*Metadata: Read* ingår automatiskt).

### Token

Token kan **bara skrivas, aldrig läsas**: när den väl är sparad visas den aldrig och skickas aldrig tillbaka till webbläsaren, kortet säger bara **Token sparad**. Lämna fältet tomt när du ändrar andra inställningar för att behålla den. Skriv en ny och spara för att byta. **Ta bort token** raderar den. Om du sätter ärendehanteraren till **Ingen** tas hela anslutningen bort.

## Skicka, synkronisera och importera

När en ärendehanterare är ansluten har listan över defekter tre åtgärder till:

- **Skicka till ärendehanteraren** (uppladdningsknappen för en defekt) skapar ett ärende från defekten: titeln med defektens nyckel, beskrivningen, allvarlighetsgraden och prioriteten samt nycklarna för de kopplade objekten. Defekten visar sedan ärendets nyckel, en länk till det och dess status i ärendehanteraren. En defekt som redan har ett ärende kan inte skickas igen.
- **Synkronisera** (synkknappen för en defekt) läser ärendets status och uppdaterar defektens status; **Synkronisera alla** gör det för varje defekt med ett ärende i den anslutna ärendehanteraren och visar hur många som ändrades. Statusarna översätts så här:

  | Jira | GitHub | Defektens status |
  |---|---|---|
  | statuskategori *To Do* | open | Öppen |
  | statuskategori *In Progress* | | Pågår |
  | *Done* | closed (completed) | Stängd |
  | *Done* med lösningen *Won't Do*, *Duplicate* eller liknande | closed (not planned) | Avvisad |

- **Importera ärende** skapar en defekt från ett ärende som redan finns i ärendehanteraren: ange dess nyckel (`SHOP-123` i Jira, `#123` eller `ägare/repo#123` i GitHub) och allvarlighetsgraden. Titel och status hämtas från ärendet.

Synkronisering läser bara; den ändrar aldrig ärendet. Ändringar du gör i en defekt i Qtestframe skrivs inte till ärendehanteraren.

Om ärendehanteraren inte kan nås eller avvisar en begäran, till exempel för att token har gått ut eller saknar en behörighet, visar en röd notis överst i listan ärendehanterarens eget svar. Kontrollera inställningarna med **Testa anslutningen**.

## Öppna defekter och releaser

Öppna defekter (status *Öppen* eller *Pågår*) som är kopplade till ett krav, till dess testfall eller till deras exekveringar räknas för kravet. De visas:

- som markeringen på kravet och antalet på fliken **Defekter**;
- i kravlistan som kolumnen *Öppna defekter* (`open_defects`);
- i [releasens](releases-and-risks.md) slutkriterium **Inga öppna defekter** (`no_open_defects`): en release är inte klar så länge ett krav i dess omfattning har en öppen defekt. Kriteriet kan begränsas till en lägsta allvarlighetsgrad, t.ex. att bara *kritiska* och *allvarliga* defekter stoppar releasen. Releasens sida listar defekterna som stoppar den.

Åtgärda eller stäng defekterna (i Qtestframe eller i ärendehanteraren följt av **Synkronisera**) för att uppfylla kriteriet.
