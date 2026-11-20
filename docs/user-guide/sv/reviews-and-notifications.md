# Granskningar och notiser

Qtestframe säger till när något behöver din uppmärksamhet, låter dig be kollegor granska ett krav eller ett test och sparar elektroniska signeringar av baslinjer. Webhooks skickar samma händelser vidare till andra system.

## Notiser

Klockan i övre fältet visar hur många notiser du inte har läst. Öppna den för att se de senaste; välj en för att gå till posten den gäller. **Alla notiser** visar allt, med ett filter för olästa.

Du får en notis när någon annan:

- tilldelar dig ett krav (ägare), en exekvering eller en defekt,
- nämner dig med `@handle` i en beskrivning, en kommentar eller anteckningar vid exekvering,
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

**Mina granskningar** i sidofältet visar förfrågningarna till dig; försenade är markerade. Välj **Granska** för att öppna en:

- Bocka för varje punkt i checklistan. En punkt som inte gäller behöver i stället en kort anteckning.
- Skriv en kommentar.
- **Godkänn** kräver att alla punkter är förbockade eller förklarade. **Begär ändringar** kräver en kommentar om vad som ska ändras.
- **Spara** sparar ditt arbete utan beslut.

Om posten ändras efter att förfrågan skickades markeras granskningen "ändrad sedan dess", så att alla ser att beslutet gällde en äldre version.

### Poster med status Granskning

Under förfrågningarna visar **Status Granskning** de krav och testfall som har status **Granskning** och som berör dig, i alla projekt, även när ingen har skickat en förfrågan till dig. **Varför du ser det** anger hur de berör dig:

- **du äger det**: kravets **Ägare** anger dig (ditt namn, din e-postadress eller ditt @namn);
- **du skrev det**: du skapade kravet eller testfallet;
- **granskning begärd**: någon har bett dig granska det;
- **du får granska**: din roll får granska krav eller testfall.

De som har väntat längst kommer först. Slå på **Bara sådant jag äger, skrev eller ombetts granska** för att dölja dem du bara ser på grund av din roll. Öppna en post för att granska den: godkänn den, avvisa den eller sätt tillbaka den till **Utkast**. När statusen inte längre är **Granskning** försvinner den från listan.

Många granskningar på en gång: när du godkänner eller avslår krav, eller markerar tester som klara, i en [massgranskning](requirements.md#godkänn-många-krav-på-en-gång) kan du slutföra dina öppna förfrågningar för de posterna i samma steg. De beslutas då utan checklistan och markeras som beslutade i en massgranskning.

En granskningsförfrågan sparar ett beslut; den ändrar inte postens status. Den som har granskningsbehörighet godkänner fortfarande ett krav eller markerar ett test som klart med granskningsfältet.

## Många granskningsförfrågningar på en gång

**Mina granskningar** har två flikar: **Att granska** (förfrågningar till dig) och **Begärda av mig** (förfrågningar du har skickat). Kryssa i förfrågningar i någon av listorna för att få ett verktygsfält; bara väntande förfrågningar ändras.

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
