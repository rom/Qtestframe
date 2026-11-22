# Varianter och kod

En produkt byggs ofta i flera **varianter**: modeller, marknader eller kundkonfigurationer. Alla krav gäller inte alla varianter, och en testkörning på en variant säger ingenting om en annan. Qtestframe håller reda på vilka krav och testfall som gäller vilka varianter och beräknar täckningen per variant.

Den andra halvan av sidan handlar om **kod**: incheckningarna, merge requests och pull requests som hänvisar till krav och testfall, de **byggen** i CI vars resultat laddats upp och **regressionsurvalet** som föreslår vilka testfall som ska köras efter en ändring.

## Varianter

Välj **Varianter** i projektets sidomeny och sedan fliken **Varianter**. Med rollen **Förvaltare** (behörigheten `project.update`) kan du lägga till, ändra och ta bort varianter.

Varje variant har:

- ett **namn**, unikt i projektet, till exempel *EU-modell*;
- eventuellt en plats **under** en annan variant: varianterna bildar en hierarki, till exempel *EU* → *EU-modell DE*;
- en **typ**: produktvariant, marknad, kundkonfiguration eller annan;
- **egenskaper**: namn och värden som `region = EU` eller `voltage = 230V`. En variant ärver egenskaperna från varianterna över den; dess egna värden gäller före dem;
- de **testkonfigurationer** den testas i. Väljer du några räknas bara resultat i de konfigurationerna (och resultat utan konfiguration) för varianten. Lämna fältet tomt för att räkna alla konfigurationer. Se [testkonfigurationer](runs.md).

När en variant tas bort flyttas varianterna under den upp en nivå. Dess körningar blir variantneutrala, och krav och testfall som bara gällde den gäller alla varianter igen.

## Tillämplighet

Ett krav eller testfall **gäller** alla varianter tills du anger något annat. Öppna det och välj **Ändra** i avsnittet **Varianter** bland detaljerna:

- **Alla varianter**: standardvalet; det omfattar även varianter som läggs till senare.
- **Valda varianter**: objektet gäller de valda varianterna och varianterna under dem. Väljer du *EU* ingår även *EU-modell DE*.
- **Uttryck**: en [fråga](search-and-filters.md) som prövas mot varje variant. Fälten är `name` och `variant` (varianten och de över den), `kind` och egenskaperna. Exempel: `region:EU`, `region:EU AND NOT voltage:110V`, `kind:customer OR variant:"EU-modell"`.

För att ange tillämpligheten för många objekt på en gång markerar du dem i listan och väljer **Gäller** i verktygsfältet.

### Visa för en variant

Välj en variant i **Visa för variant** i krav- och testfallslistorna. Listan visar då bara objekten som gäller den, med den täckning och de senaste resultat som räknas för varianten. Frågespråket känner också till varianter, och API:et tar `?variant=` för spårbarhetsmatrisen, exporter, ReqIF och rapporter:

| Fråga | Hittar |
|---|---|
| `variant:"EU-modell"` | objekt som gäller varianten *EU-modell* |
| `applicability:selected` | objekt som bara gäller vissa varianter |
| `applicability:all` | objekt som gäller alla varianter |

## Täckning per variant

Täckningen beräknas per **variant × testkonfiguration**:

1. Ett krav räknas i en variant när det gäller den. Annars är dess täckning där **Ej tillämplig**.
2. Bara de länkade testfall som gäller varianten räknas.
3. Bara resultat som räknas för varianten används: resultat av körningar för varianten eller en variant över den, och resultat av **variantneutrala** körningar. Resultat i konfigurationer som varianten inte testas i lämnas utanför.

När du skapar en körning väljer du vilken **Variant** den testar, eller låter den vara variantneutral. Uppladdningar från CI kan också ange varianten (se [byggen](#byggen)).

Fliken **Täckningsmatris** visar alla krav mot alla varianter, med hur många av de tillämpliga kraven som godkänts. Filtrera kraven med en fråga och välj **I konfiguration** för att se en enda testkonfiguration. I ett kravs detaljer visar avsnittet **Varianter** dess täckning i varje variant och, per variant, i varje konfiguration.

### Releaser och dokument för en variant

Välj en variant i **Status för variant** på sidan **Releaser** för att utvärdera slutkriterierna för den: bara releasens krav som gäller varianten, testfallen som gäller den och resultaten som räknas. När en release släpps prövas fortfarande alla varianter.

När du [skapar ett dokument](documents.md) väljer du **För variant**: kravspecifikationen eller testplanen innehåller då bara det som gäller varianten, med dess täckning och resultat, och anger varianten på titelsidan.

## Kodlänkar

Krav och testfall kan länkas till **incheckningar** och **merge- eller pull requests**. Öppna ett objekt och välj fliken **Kod**. Den visar de länkade ändringarna med status (öppen, sammanfogad eller stängd), kodförråd, gren, författare och byggena av varje incheckning.

Länkarna skapas på två sätt:

- **Automatiskt**: när ett incheckningsmeddelande, eller titeln, beskrivningen eller grennamnet för en merge- eller pull request, nämner en objektnyckel länkas ändringen till objektet. Både hela nycklar (`DEMO-REQ-12`) och korta nycklar (`REQ-12`, `TC-3`, som avser det här projektet) fungerar. En nyckel från ett annat projekt (`OTHER-REQ-1`) hoppas över.
- **För hand**: välj **Länka incheckning eller MR** på fliken **Kod** och ange SHA eller nummer, adressen och en titel.

### Webhookar

De automatiska länkarna kommer från webhookar. Med rollen **Förvaltare** öppnar du **Inställningar → Projekt → Kodintegration**:

1. Välj **Skapa** (eller skriv en hemlighet på 16 till 200 tecken) och **Spara**. Kopiera hemligheten nu: den visas aldrig igen.
2. I **GitLab** öppnar du kodförrådets *Settings → Webhooks*, anger **Webhookadress för GitLab**, lägger hemligheten i *Secret token* och kryssar i *Push events* och *Merge request events*.
3. I **GitHub** öppnar du *Settings → Webhooks → Add webhook*, anger **Webhookadress för GitHub**, väljer *application/json*, lägger hemligheten i *Secret* och väljer händelserna *Pushes* och *Pull requests*.

Qtestframe kontrollerar varje leverans: GitLab måste skicka hemligheten, GitHub måste signera innehållet med hemligheten (HMAC-SHA256). Leveranser utan giltig signatur avvisas. **Ta bort hemligheten** stänger av webhookarna.

## Byggen

Ett **bygge** är ett bygge i CI som testresultat laddats upp för. CI anger det när [JUnit-resultat](runs.md) eller JSON-resultat laddas upp, som frågeparametrar:

```
POST /api/v1/runs/{run}/junit?build_id=4711&build_number=1.4.41&commit=3f2c9e1&branch=main&pipeline_url=https://ci.example.org/p/4711&variant=EU-modell
```

`build_id` är CI:s eget id (unikt per projekt); `build_number`, `commit`, `branch` och `pipeline_url` är valfria, och `variant` gör körningen till en körning för den varianten. Körningen kopplas sedan till bygget.

- **Verifierat i bygge**: fliken **Kod** för ett krav visar det senaste bygget där alla länkade testfall godkändes, och de byggen där dess testfall kördes med täckningen i varje.
- **Byggen** för ett testfall: fliken **Kod** för ett testfall visar byggena det kördes i, nyast först, med resultatet i varje.
- **Kod → Byggen** visar alla byggen med körningar och andel godkända.

## Regressionsurval

Välj **Kod → Regressionsurval** för att hitta testfallen som ska köras efter ändringar:

- **Ändrade krav**: nycklar till krav som ändrats;
- **Incheckningar sedan bygge**: incheckningarna efter byggets egen incheckning (på byggets gren), och de krav och testfall de nämner;
- **Variant**: bara krav och testfall som gäller den;
- **Ta med krav som beror på, förfinar eller härleds från dem**: ett steg längs dessa [relationer](requirements.md);
- **Ta med testfall som är förutsättningar**: testfallen som de valda testfallen beror på.

**Föreslå testfall** visar testfallen och varför vart och ett är med. Utfasade testfall lämnas utanför. **Skapa körning** gör en körning av dem (för den valda varianten), med byggets namn när du utgår från ett bygge. CI kan göra samma sak med `POST /api/v1/projects/{pid}/regression/run`.
