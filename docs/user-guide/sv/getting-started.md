# Kom igång

Den här sidan förklarar hur du loggar in, hittar rätt på skärmen och ställer in Qtestframe som du vill ha det.

## Logga in

Öppna adressen du har fått av din administratör (för en lokal testinstallation `http://localhost:8080`).

![Inloggningssidan med e-post, lösenord, språkväljaren och länken för att logga in i en viss organisation](../images/sv-login.png)

1. Ange **E-post** och **Lösenord** och välj **Logga in**.
2. Om du tillhör flera organisationer och vill börja i en viss, välj **Logga in i en viss organisation …** och ange dess kortnamn, till exempel `acme`. Utan kortnamn hamnar du i din standardorganisation (se [Inställningar](#inställningar)) eller den du använde senast.
3. Om din organisation använder enkel inloggning (SSO) visar sidan en knapp per leverantör under **eller fortsätt med**. Välj den och logga in hos identitetsleverantören; du kommer automatiskt tillbaka till Qtestframe.

Språkknapparna högst upp på inloggningssidan växlar mellan engelska och svenska innan du loggar in. Efter inloggningen gäller språket i dina inställningar.

> **Tips:** Efter tio misslyckade försök inom fem minuter spärras inloggningen en stund för den e-postadressen och nätverksadressen. Vänta några minuter och försök igen, eller be en administratör att [återställa ditt lösenord](administration.md#återställa-ett-lösenord).

### Det här ser du efter inloggningen

- **Introduktionsturen.** Första gången du loggar in visar en kort tur de viktigaste delarna av skärmen. Använd **Nästa** och **Tillbaka**, eller **Hoppa över turen** (eller <kbd>Esc</kbd>) för att stänga den. Du kan starta den igen när som helst med **Hjälp → Starta introduktionsturen igen**.

  ![Introduktionsturen: ett stegkort bredvid den markerade posten Krav i sidopanelen, med Tillbaka, Nästa och Hoppa över turen](../images/sv-intro-tour.png)
- **Information om inloggningen.** Vid senare inloggningar kan Qtestframe visa när och varifrån du loggade in förra gången och hur många misslyckade försök det har varit sedan dess. Om du inte känner igen en inloggning, byt lösenord och logga ut dina andra sessioner (se [Säkerhet](#säkerhet)).
- **Tvingat lösenordsbyte.** Om en administratör har återställt ditt lösenord måste du välja ett nytt innan du kan fortsätta.

## Sidans uppbyggnad

![Huvudlayouten: sidopanel till vänster, övre fältet med organisationsväljare, sökning, hjälp, temaväljare, mörkt läge och användarmenyn](../images/sv-layout.png)

### Sidopanelen

Det mörka fältet till vänster är huvudnavigeringen:

- **Arbetsyta:** **Projekt** (alla projekt i den aktuella organisationen) och **Sök** (sökning i alla projekt).
- **Det öppna projektet:** när du öppnar ett projekt visas dess nyckel och namn med projektets delar: **Översikt**, **Krav**, **Testfall**, **Testkörningar**, **Spårbarhet**, **Analys**, **Beroendegraf**, **Testflöde**, **AI-prompter** (mjukvaruprojekt), **Taggar**, **Baslinjer**, **Kategorier & inställningar**, **Export** och, där de finns, **Dokument** och **Rapporter**.
- **Administration**, om din roll tillåter det.

I ett projekt visas samma delar också som flikar högst upp på sidan, under sökvägen *Projekt › projektets namn*.

**Fäll ihop** längst ned krymper sidopanelen till ikoner; håll pekaren över en ikon för att se namnet. På skärmar smalare än 1024 bildpunkter är sidopanelen dold och öppnas med menyknappen (☰) i övre fältet; <kbd>Esc</kbd> eller stängningsknappen stänger den igen.

### Övre fältet

Från vänster till höger:

| Del | Användning |
|---|---|
| **Organisation** | Visar den aktuella organisationen och låter dig byta till en annan som du är medlem i. På mobiler är det en byggnadsikon. |
| **Sökfältet** | Söker bland krav, testfall och körningar i alla projekt i organisationen. Skriv text eller en [fråga](search-and-filters.md) och tryck <kbd>Enter</kbd>. På mobiler är det en förstoringsglasknapp. |
| **Hjälp** (?) | Öppnar **Användarhandbok**, **Hjälp för den här sidan**, **Frågespråket**, **Referenser och standarder** och **Starta introduktionsturen igen**. |
| **Ångra / Gör om / Ångrahistorik** | Ångrar och gör om dina ändringar, se [Ångra och gör om](#ångra-och-gör-om). |
| **Färgtema** | Väljer ett av de tjugotvå temana, se [Teman](accessibility-and-languages.md#teman). |
| **Ljust läge / Mörkt läge** | Växlar snabbt mellan ljust och mörkt. |
| **Användarmeny** | Ditt namn och din roll, **Profil och lösenord**, ljust/mörkt läge, **Språk** och **Logga ut**. |

### Ångra och gör om

De flesta ändringar kan ångras: att redigera, skapa, kopiera och ta bort krav och testfall, flytta dem, massändringar, länkar, relationer, beroenden, granskningsbeslut, att återställa en version, utfall i exekveringsvyn samt att skapa, byta namn på och flytta mappar.

- **Ångra** (↶, <kbd>Ctrl</kbd>+<kbd>Z</kbd>) återställer den senaste ändringen; välj den igen för att gå längre bakåt. Knappens tips namnger ändringen, till exempel *Ångra: Redigera SHOP-TC-4*.
- **Gör om** (↷, <kbd>Ctrl</kbd>+<kbd>Skift</kbd>+<kbd>Z</kbd> eller <kbd>Ctrl</kbd>+<kbd>Y</kbd>) gör en ångrad ändring igen. En ny ändring rensar det som kunde göras om.
- **Ångrahistorik** (🕘) listar dina senaste 50 ändringar, nyast först, med när de gjordes. Väljer du en ångras den och allt som gjordes efter den.

Kortkommandona fungerar utanför textfält; i ett textfält ångrar de som vanligt det du skrivit. Historiken hör till dig och din webbläsarflik: den rensas när du byter organisation, loggar ut eller laddar om sidan. Ett borttaget krav eller testfall kommer tillbaka med sin nyckel, sina länkar, relationer, kommentarer, versionshistorik, bilagor och utfall; borttagningar kan ångras i 30 dagar via API:t (se `GET /projects/{pid}/trash` i API-referensen). Ett ångra misslyckas om läget har ändrats under tiden, till exempel om någon annan har redigerat objektet eller om du inte längre har behörigheten; objektet lämnas då som det är och ett meddelande förklarar varför.

### Hjälp där du behöver den

- Många fält och avsnitt har ett litet **i** bredvid namnet. Peka på det med musen, eller flytta tangentbordsfokus dit, så visas vad fältet betyder. Välj det (eller tryck på det på en mobil) för att låta förklaringen vara kvar, och välj **Läs mer i användarhandboken** för att öppna avsnittet som beskriver det. <kbd>Esc</kbd> stänger förklaringen.
- Tryck <kbd>?</kbd> var som helst utanför ett textfält för att öppna handbokens sida för den vy du är i (i exekveringsvyn visar <kbd>?</kbd> i stället vyns kortkommandon).
- **?** bredvid varje filterfält listar operatorerna och fälten i [frågespråket](search-and-filters.md) för just den listan.

### Organisationer

En organisation är en separat arbetsyta med egna projekt, medlemmar och API-nycklar. När du byter organisation i övre fältet tar Qtestframe dig till organisationens projektlista. Din roll kan skilja sig mellan organisationer, så vad du får göra kan också ändras.

Om du ser meddelandet *Du har ingen aktuell organisation*, välj en i organisationsväljaren. Om du inte är medlem i någon organisation, be en administratör att lägga till dig.

### Sökning

Sökfältet i övre fältet söker överallt i den aktuella organisationen. Träffarna grupperas per typ med den matchande texten markerad; välj en träff för att öppna den. Alla detaljer finns i [Sökning och filter](search-and-filters.md).

### Tema och språk

- Välj **färgtema** i övre fältet eller i dina inställningar. Valet sparas med ditt konto och följer med till alla enheter.
- Byt **språk** med **Användarmeny → Språk**: *English* eller *Svenska*. Ändringen gäller direkt och sparas som din inställning.

## Din profil

Öppna **Användarmeny → Profil och lösenord**.

![Profilsidan med kontouppgifter, medlemskap, inställningar och säkerhet](../images/sv-profile.png)

### Konto

- **Visningsnamn**: namnet som andra ser i kommentarer, versionshistorik och händelseloggen.
- **Profilbild**: välj en bild för ditt konto: ladda upp en bild (PNG, JPEG, GIF eller WebP, högst 1 MB), välj en av de inbyggda bilderna eller använd dina initialer.
- **Inloggningsleverantör**, **Medlem sedan** och dina **Medlemskap**: organisationerna du tillhör och din roll i var och en.

### Inställningar

Inställningarna sparas med ditt konto och gäller på alla enheter. Ändra dem och välj **Spara inställningar**.

| Inställning | Effekt |
|---|---|
| **Språk** | Engelska eller svenska; *Webbläsarens språk* följer webbläsaren. |
| **Tema** | Färgtema. *Följ operativsystemet* växlar mellan ljust och mörkt enligt systemets inställning. |
| **Startsida** | Det du ser efter inloggningen: projektlistan, det senast öppnade projektet, ett standardprojekt eller sökningen. |
| **Standardprojekt** | Projektet som öppnas när startsidan är *Standardprojekt*. |
| **Standardorganisation** | Organisationen som väljs vid inloggning när du inte anger någon. |
| **Rader per tabellsida** | 10, 25, 50 eller 100 rader i tabeller. |
| **Datumformat** | *Språkets format*, *ISO* (2026-03-14 09:30) eller *Relativt* (för 3 timmar sedan). |
| **Tidszon** | Tidszonen för alla tider; *Webbläsarens* använder datorns. |
| **Skapa även länkat testfall (nya krav)** | Ditt personliga standardval för kryssrutan i kravredigeraren. |
| **Kompakta tabeller** | Tätare tabellrader, så att du ser mer på en gång. |
| **Dölj den löpande språkanalysen av kraven** | Stänger av analysrutan i kravredigeraren. |

### Säkerhet

- **Lösenord**: för lokala konton anger du **Nuvarande lösenord** och **Nytt lösenord** två gånger och väljer **Byt lösenord**. Om du loggar in med enkel inloggning hanteras lösenordet av din identitetsleverantör.
- **API-nycklar**: nycklar för CI-pipelines och skript skapas under **Administration → API-nycklar** (se [API-nycklar](administration.md#api-nycklar)).
- **Aktiva sessioner**: alla webbläsare och enheter som är inloggade med ditt konto, med webbläsare, IP-adress, inloggningstid och senaste aktivitet. **Logga ut** avslutar en session; **Logga ut alla andra sessioner** avslutar alla utom den du använder.
- **Inloggningshistorik**: dina senaste inloggningar och misslyckade försök, om de visas.

## Nästa steg

- Följ [handledningen](tutorial.md) och bygg ett litet projekt från början.
- Lär dig [frågespråket](search-and-filters.md); det fungerar i alla filterfält.
