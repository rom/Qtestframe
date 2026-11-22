# Granskningar och notiser

Qtestframe säger till när något behöver din uppmärksamhet, låter dig be kollegor granska ett krav eller ett test och sparar elektroniska signeringar av baslinjer. Webhooks och chattkanaler skickar samma händelser vidare till andra system och till Slack, Teams och andra chattverktyg.

## Mitt arbete

**Mitt arbete** i huvudnavigeringen (under **Projekt** och **Sök**) är din personliga arbetslista: allt som väntar på dig, i alla projekt som du ser, även dina granskningar. Siffran bredvid visar hur många poster som behöver dig; den blir röd när några av dem är försenade. Varje rad länkar till posten och visar varför den finns med. En post visas en gång, med alla sina orsaker: ett krav som du äger och som någon har bett dig granska är en rad med **Ägare** och **Granskning begärd**.

| Orsak | Posten finns med eftersom |
|---|---|
| **Ägare** | du äger ett krav med status utkast, granskning, godkänt eller implementerat, ett testfall med status utkast eller granskning, eller en öppen risk (identifierad eller under åtgärd) |
| **Tilldelad** | en öppen defekt eller en testexekvering som inte har körts än (i en planerad eller pågående körning) är tilldelad dig |
| **Granskning begärd** | någon har bett dig granska ett krav eller ett testfall. Välj **Granska** på raden för att öppna granskningen direkt (se [Granska](#granska)) |
| **Ändringar begärda** | en granskare som du frågade har begärt ändringar, och du har inte ändrat posten sedan dess. När du ändrar posten, eller frågar granskaren igen, försvinner orsaken |
| **Väntar på granskning** | en granskning som du har begärt väntar fortfarande (på någon annan) |
| **Under granskning** | ett krav eller testfall med status **Granskning** som du äger, skrev eller har ombetts granska (väntar på någon annan om du inte själv är granskare) |
| **Godkännande** | ett begärt undantag eller en avvikelse väntar på din signatur: du är en av godkännarna, eller inga godkännare anges och du får godkänna undantag. Dina egna begäranden visas inte. |
| **Riskaccept** | en åtgärdad eller accepterad risk ligger över projektets acceptansnivå och saknar en giltig signerad accept, och du får signera den |
| **Riskgenomgång** | du äger nästa genomgång av en risk och den infaller inom en vecka |
| **Försenad** | förfallodatumet har passerat: en granskningsförfrågans förfallodatum eller en risks genomgångsdatum |
| **Misstänkt**, **Inaktuella resultat**, **Blockerad** | ett krav eller testfall som du äger har blivit misstänkt (något länkat eller relaterat har ändrats), har inaktuella testresultat eller blockeras av en öppen defekt — även när det annars är klart |

Rader som bara väntar på någon annan (**Väntar på granskning**, **Under granskning**) visas men räknas inte i siffran; raden ovanför listan anger hur många som väntar på andra. Försenade poster kommer först, sedan efter förfallodatum, sedan de senast ändrade. Välj ett projekt för att bara se dess poster, eller en orsak för att bara se de raderna. Projektets översikt visar samma lista för projektet i widgeten **Mitt arbete**, med antalet poster per orsak; välj en orsak för att öppna den filtrerade listan.

Arbetslistan beräknas när du öppnar den; när arbetet är gjort (granskningen godkänd, defekten stängd, testet kört, statusen ändrad) försvinner raden.

### Granskningar i Mitt arbete

Växla **Mitt arbete** från **Allt arbete** till **Granskningar** för att se allt om granskningar på ett ställe (siffran är hur många granskningar som väntar på dig):

- **Att granska**: granskningsförfrågningarna till dig. Välj **Granska** för att öppna en.
- **Begärda av mig**: förfrågningarna som du har skickat, med vem som granskar och vad de beslutade.
- **Status Granskning**: de krav och testfall med status Granskning som berör dig (se [Poster med status Granskning](#poster-med-status-granskning)).

**Väntande** visar de öppna förfrågningarna, **Alla** även de beslutade och avbrutna. Länkar till en granskningsförfrågan (i en notis, en diskussion eller ett äldre bokmärke till **Mina granskningar**) öppnar den här.

## Notiser

Klockan i övre fältet visar hur många notiser du inte har läst. Öppna den för att se de senaste; välj en för att gå till posten den gäller. **Alla notiser** visar allt, med ett filter för olästa.

Du får en notis när någon annan:

- gör dig till ägare av ett krav, ett testfall eller en risk (även vid en massändring), eller tilldelar dig en exekvering eller en defekt,
- nämner dig med `@handle` i en beskrivning, en kommentar eller anteckningar vid exekvering,
- svarar i eller löser en [diskussionstråd](discussions.md) som du har skrivit i, eller startar en tråd om ett objekt du är ansvarig för eller en granskning du deltar i,
- ber dig granska något, eller har granskat det du bad om,
- ändrar status på en defekt som är tilldelad dig.

Du får aldrig notiser om dina egna ändringar. Lästa notiser rensas bort efter en tid.

På **Alla notiser** kryssar du i notiser (eller **Markera alla som visas**) för att **Markera som läst**, **Markera som oläst** eller **Ta bort** dem tillsammans. **Ta bort lästa** tar bort alla notiser som du redan har läst.

### Notiser med e-post

När servern kan skicka e-post kan du också få dina notiser med e-post. Välj under **Profil → Notiser med e-post** (eller **Notiser med e-post** på notissidan):

- **Av** (standard): notiserna visas bara i appen.
- **Direkt**: varje notis skickas till dig med e-post, med en länk till posten.
- **Daglig sammanfattning**: en gång om dagen får du ett e-postmeddelande med de notiser som du inte har läst än och som inte har skickats tidigare. Inget skickas de dagar då inga nya notiser har kommit.

Du får aldrig e-post om det du själv har gjort. Varje e-postmeddelande med notiser har en länk tillbaka till den här inställningen, så att du när som helst kan ändra eller stänga av den.

## Begär granskning

Öppna ett krav eller ett test. Under granskningsfältet visar **Granskningsförfrågningar** vilka som har tillfrågats och vad de beslutade. Välj **Begär granskning** (du behöver skrivbehörighet för posten):

1. Välj en eller flera granskare.
2. Skriv ett meddelande och, om du vill, ett datum då det ska vara klart.
3. Justera checklistan: en punkt per rad. Varje sorts post har en standardlista (till exempel "Verifierbart (kan testas)" för krav).

Varje granskare får en egen förfrågan och en notis. Den som redan har en öppen förfrågan för posten tillfrågas inte igen. Du kan avbryta en öppen förfrågan med krysset bredvid den.

## Granska

Förfrågningarna till dig finns i **Mitt arbete**: som rader med orsaken **Granskning begärd**, och under **Granskningar → Att granska**, där försenade är markerade. Välj **Granska** för att öppna en. Dialogen länkar till posten (**Öppna** och dess nyckel) om du vill läsa den i sin helhet först:

- Bocka för varje punkt i checklistan. En punkt som inte gäller behöver i stället en kort anteckning.
- Skriv en kommentar.
- **Godkänn** kräver att alla punkter är förbockade eller förklarade. **Begär ändringar** kräver en kommentar om vad som ska ändras.
- **Spara** sparar ditt arbete utan beslut.

Om posten ändras efter att förfrågan skickades markeras granskningen "ändrad sedan dess", så att alla ser att beslutet gällde en äldre version.

### Poster med status Granskning

**Mitt arbete → Granskningar → Status Granskning** visar de krav och testfall som har status **Granskning** och som berör dig, i alla projekt, även när ingen har skickat en förfrågan till dig. **Varför du ser det** anger hur de berör dig:

- **du äger det**: kravets **Ägare** anger dig (ditt namn, din e-postadress eller ditt @namn);
- **du skrev det**: du skapade kravet eller testfallet;
- **granskning begärd**: någon har bett dig granska det;
- **du får granska**: din roll får granska krav eller testfall.

De som har väntat längst kommer först. Slå på **Bara sådant jag äger, skrev eller ombetts granska** för att dölja dem du bara ser på grund av din roll. Öppna en post för att granska den: godkänn den, avvisa den eller sätt tillbaka den till **Utkast**. När statusen inte längre är **Granskning** försvinner den från listan.

Många granskningar på en gång: när du godkänner eller avslår krav, eller markerar tester som klara, i en [massgranskning](requirements.md#godkänn-många-krav-på-en-gång) kan du slutföra dina öppna förfrågningar för de posterna i samma steg. De beslutas då utan checklistan och markeras som beslutade i en massgranskning.

En granskningsförfrågan sparar ett beslut; den ändrar inte postens status. Den som har granskningsbehörighet godkänner fortfarande ett krav eller markerar ett test som klart med granskningsfältet.

## Många granskningsförfrågningar på en gång

I **Mitt arbete → Granskningar** visar **Att granska** förfrågningarna till dig och **Begärda av mig** förfrågningarna du har skickat. Kryssa i förfrågningar i någon av listorna för att få ett verktygsfält; bara väntande förfrågningar ändras.

- **Att granska**: **Godkänn** flera på en gång (med en kommentar, och efter att du bekräftat att checklistorna inte bockas för; varje granskning sparar att den godkändes i en massgranskning), eller **Begär ändringar** med en kommentar för alla. De som frågade får en notis.
- **Begärda av mig**: **Påminn** granskarna (de får en ny notis, med ditt meddelande), ange **Klart senast** (tomt tar bort datumet), eller **Avbryt** förfrågningarna.

## Signera en baslinje

Öppna en baslinje under **Baslinjer**. **Signeringar** visar de elektroniska signaturerna: vem som signerade, i vilken roll (författare, granskare, godkännare eller release), när, och ett fingeravtryck av innehållet.

För att signera behöver du granskningsbehörighet för krav eller tester. Välj **Signera**, välj vad din signatur betyder och bekräfta med ditt lösenord. Om du loggar in med enkel inloggning måste inloggningen vara mindre än 15 minuter gammal. API-token kan inte signera.

Varje signatur sparar ett SHA-256-fingeravtryck av baslinjens poster. **Giltig** betyder att innehållet är exakt det som signerades. En signerad baslinje kan inte tas bort, och varje signatur skrivs i granskningsloggen.

## Webhooks

Under **Kategorier & inställningar** kan projektets administratörer (behörighet att ändra projektet) lägga till webhooks. En webhook skickar händelser som HTTP POST-anrop med JSON-innehåll:

```json
{ "id": "…", "event": "defect.created", "project_id": "…", "actor": "anna@example.com", "time": "…", "data": { … } }
```

- Välj händelser, eller **Alla händelser**. Exempel är `requirement.updated`, `run.completed`, `defect.status`, `release.released`, `review.completed` och `baseline.signed`.
- Signeringshemligheten visas bara en gång, när du skapar webhooken eller väljer **Ny hemlighet**.
- Varje anrop har huvudet `X-Qtestframe-Signature: sha256=<hex>`, HMAC-SHA256 av innehållet med hemligheten. Kontrollera det innan du litar på ett anrop.
- **Skicka test** skickar en `ping`-händelse direkt. Tabellen visar resultatet av den senaste leveransen.

Leveranser görs inte om och omdirigeringar följs inte.

Kryssa i webhooks i listan för att **Slå på** eller **Stäng av** flera på en gång, **Lägg till händelser** eller **Ta bort händelser** (varje webhook måste ha minst en händelse kvar, annars ändras ingenting), **Skicka test** till alla (resultatet för varje visas) eller **Ta bort** dem. Andra ändringar än borttagning kan ångras med ångra-knappen.

## Chattaviseringar

Under **Kategorier & inställningar** kan projektets administratörer (behörighet att ändra projektet) skicka projektets händelser till en chattkanal i **Slack**, **Microsoft Teams**, **Mattermost**, **Rocket.Chat**, **Google Chat**, **Matrix** eller **Discord**. Välj **Lägg till chattkanal**, välj chattverktyg och klistra in den webhook-URL som verktyget ger dig (Matrix: hemservern, rummet och åtkomsttoken för en botanvändare). Administratörshandboken beskriver hur du får fram URL:en i varje verktyg.

Varje meddelande anger händelsen, länkar objektets nyckel och titel till objektet i Qtestframe och visar dess status (i ord och färg), prioritet, vem som gjorde det och en kort sammanfattning, till exempel ”9 passed, 2 failed” för en avslutad körning eller början av en ny defekts beskrivning. Meddelandena skrivs på det språk du väljer för kanalen (engelska eller svenska) och hålls inom varje chattverktygs längdgränser.

För varje kanal väljer du:

- **Händelser**: **Alla händelser** (även händelser som senare versioner av Qtestframe lägger till) eller ett urval, grupperat i testkörningar, defekter, krav och tester, granskningar, releaser, undantag och risker.
- **Filter**: **bara underkända** testkörningar och resultat; en **lägsta prioritet** (till exempel *Hög*: bara objekt med hög eller kritisk prioritet; defekter använder sin prioritet eller allvarlighetsgrad, risker sin nivå; händelser utan prioritet, som en avslutad körning, släpps ändå igenom); en **release** (bara releasen själv, dess körningar och resultat och dess undantag).
- **Sammandrag**: 0 skickar varje händelse direkt. Med till exempel 60 minuter samlas händelserna och skickas som ett meddelande varje timme (”12 händelser i BANK …”, med de första händelserna och ”… och 4 till”).
- **Aktiv**: stäng av kanalen utan att förlora dess inställningar.

Webhook-URL:en och token är hemligheter: de lagras krypterade och visas aldrig igen; listan visar bara vart kanalen skickar (till exempel `hooks.slack.com`). Ange en ny för att byta; lämna fältet tomt för att behålla den sparade.

**Skicka testmeddelande** skickar ett testmeddelande direkt, även till en avstängd kanal, och visar om det kom fram. **Leveranslogg** listar de senaste 50 meddelandena med HTTP-status och fel. Ett meddelande som misslyckas försöks igen efter 1, 2, 4, 8 och 16 minuter (en URL eller token som chattverktyget vägrar försöks inte igen). Efter 5 misslyckade meddelanden i rad stänger kanalen av sig själv, listan visar varför och projektets administratörer får en avisering. Rätta URL:en, skicka ett testmeddelande och slå på kanalen igen.

I ett **begränsat** projekt visar meddelandena bara händelse, objektets nyckel, status och länk, så att titlar och beskrivningar inte hamnar i en chatt som personer utanför projektet kan läsa. En organisationsadministratör kan tillåta att en kanal visar hela innehållet.
