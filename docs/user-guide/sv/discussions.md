# Diskussioner

Varje krav, testfall, defekt, risk, testkörning och granskningsbegäran har en **diskussion**: trådar med kommentarer där ni ställer frågor, kommer överens om ändringar och nämner kollegor. En tråd är öppen tills någon markerar den som löst, så att öppna frågor inte glöms bort.

## Var du hittar en diskussion

- **Krav och testfall**: fliken **Diskussion** i redigeraren. Siffran på fliken är antalet öppna (olösta) trådar.
- **Testkörningar**: körningens flik **Diskussion**, bredvid exekveringarna, filerna och länkarna.
- **Defekter och risker**: under formuläret när du öppnar en defekt eller en risk.
- **Granskningsbegäranden**: i granskningsdialogen, under checklistan och granskningskommentaren. Granskaren, den som bad om granskningen och alla som kan läsa det granskade objektet deltar.
- **Alla trådar i ett projekt**: projektets sida **Diskussioner** (se [Sidan Diskussioner](#sidan-diskussioner)).

## Skriva en kommentar

Skriv i fältet längst ned och välj **Starta tråd** (eller tryck <kbd>Ctrl</kbd>+<kbd>Enter</kbd>). Varje ny kommentar som skrivs där startar en tråd. För att svara i en tråd väljer du **Svara** under den. Svar finns i en nivå: ett svar på ett svar hamnar i samma tråd.

Kommentarer skrivs i **Markdown**:

| Du skriver | Du får |
|---|---|
| `**fetstil**`, `*kursiv*`, `~~överstruken~~` | **fetstil**, *kursiv*, ~~överstruken~~ |
| `` `kod` `` och block mellan rader med ```` ``` ```` | kod |
| rader som börjar med `-` eller `1.` | listor |
| `> text` | ett citat |
| `[text](https://example.com)` eller en ensam adress | en länk (öppnas i en ny flik) |
| `SHOP-REQ-12`, `SHOP-TC-3`, `SHOP-BUG-4` | en länk till objektet |

HTML som skrivs i en kommentar visas som text, och bilder visas inte, så en kommentar kan inte hämta något från andra webbplatser.

## Nämna personer

Skriv `@` så visas en lista med projektets personer, som i de andra textfälten (se [Nämn personer](requirements.md#nämn-personer)). Använd piltangenterna och <kbd>Enter</kbd> eller <kbd>Tab</kbd> för att infoga någon. Medlemmar som nämns får en notis, och ett e-postmeddelande om de har valt notiser via e-post (se [Notiser](reviews-and-notifications.md#notiser)). Ingen får höra om ett projekt som de inte kan se.

Vilka som också får veta:

- de som har skrivit i en tråd, om nya svar och när tråden markeras som löst;
- objektets ansvarige (kravets ägare, defektens tilldelade, riskens ägare, granskaren och den som begärde en granskning), om en ny tråd.

Var och en får veta det en gång per kommentar, och aldrig om sina egna kommentarer.

## Redigera, ta bort, reagera

- **Redigera** (pennan) ändrar din egen kommentar; den markeras sedan *(redigerad)*. Personer som du nämner i den nya texten får en notis.
- **Ta bort** (papperskorgen) tar bort din egen kommentar. Den finns kvar i tråden som *Kommentaren har tagits bort*, så att svaren fortfarande går att förstå; texten är borta. En **moderator** (den som får ändra projektets inställningar, `project.update`, eller hantera medlemmar) kan ta bort allas kommentarer; händelseloggen visar att den modererades.
- **Lägg till reaktion** (smileyn) lägger till tummen upp, tummen ner, hjärta, hurra, *tittar på det* eller *klart*. Välj en reaktion under en kommentar för att lägga till eller ta tillbaka din.

## Markera som löst och öppna igen

När en fråga är besvarad väljer du **Markera som löst** under tråden. Lösta trådar fälls ihop: slå på **Visa lösta** för att se dem, markerade *Löst* (peka på markeringen för att se vem som löste den och när). **Öppna igen** öppnar en tråd på nytt. Alla som får skriva i diskussionen kan lösa och öppna trådar.

## Sidan Diskussioner

Projektets sida **Diskussioner** listar trådarna för alla dess objekt, den senaste aktiviteten först: objektet (med sin typ), vem som startade tråden och hur, antalet svar och vem som skrev senast. Välj ett objekt för att öppna det med dess diskussion.

Begränsa listan med:

- **Öppna**, **Lösta** eller **Alla** trådar;
- **Alla trådar**, **Jag deltar i** (trådar du har skrivit i) eller **Nämner mig**;
- typ av objekt (krav, testfall, defekter, risker, testkörningar, granskningsbegäranden);
- **Sök i kommentarerna** efter ett ord i någon av trådens kommentarer.

Filtren är en del av sidans adress, så du kan spara *Öppna trådar som nämner mig* som bokmärke.

## Hitta objekt med öppna diskussioner

I listorna över krav och testfall filtrerar du med:

- `has:open-discussion`: objekt med minst en olöst tråd (`open_discussions>2` för fler än två);
- `mentions:me`: objekt som nämner dig, i texterna eller i diskussionen; `mention:anna.berg` för någon annan.

Se [Sökning och filter](search-and-filters.md).

## Behörigheter

| För att | Behöver du |
|---|---|
| läsa en diskussion | läsbehörigheten för objektet (till exempel `requirement.read`, `run.read`) |
| kommentera, svara, redigera dina kommentarer, lösa, öppna igen och reagera | `comment.create` (alla roller utom **Läsare** som standard) |
| ta bort dina egna kommentarer | `comment.create` |
| ta bort allas kommentarer | `project.update` i projektet, eller `member.manage` |

`comment.create` är en projektbehörighet: en projektroll kan ge eller ta bort den i ett enskilt projekt. Varje kommentar, ändring, borttagning, lösning och återöppning registreras i händelseloggen (`comment.create`, `comment.update`, `comment.delete`, `comment.resolve`, `comment.reopen`).

## Diskussioner på andra ställen

- **Skapade dokument**: markera **Diskussioner om objekten** under *Bilagor* när du skapar en kravspecifikation eller en testplan för att lägga till en bilaga med trådarna för dess krav eller testfall (se [Skapa en kravspecifikation eller en testplan](documents.md#skapa-en-kravspecifikation-eller-en-testplan)).
- **Sparade projekt, säkerhetskopior och papperskorgen**: diskussioner ingår i ett sparat projekt och i en säkerhetskopia, och ett borttaget krav eller testfall kommer tillbaka från papperskorgen med sin diskussion. När en defekt, en risk eller en körning tas bort tas dess diskussion bort.
- **Webhooks**: händelserna `comment.created`, `comment.mentioned` och `comment.resolved` skickar nya kommentarer, omnämnanden och lösta trådar till andra system.
