# Undantag, restrisk och uttalande om tillämplighet

Alla krav uppfylls inte i tid, och alla risker kan inte tas bort. Den här sidan beskriver tre sätt att dokumentera sådana beslut formellt, så att de syns, är godkända och tidsbegränsade:

- ett **undantag** accepterar att krav inte uppfylls tills vidare; en **avvikelse** accepterar att de uppfylls med en annan lösning;
- en risks **restrisk** är det som återstår efter åtgärden, jämförs med en **gräns för riskacceptans** och accepteras med en signering;
- **uttalandet om tillämplighet** (SoA, Statement of Applicability) anger för varje åtgärd i en standard som ISO/IEC 27001 om den är tillämplig, varför, vem som äger den och hur långt den är införd.

| Åtgärd | Roll som krävs |
|---|---|
| Visa undantag, restrisker och uttalandet om tillämplighet | Läsare |
| Skapa, redigera, begära och dra tillbaka undantag; ange restrisk; redigera uttalandet | Ansvarig (`requirement.write`) |
| Godkänna, avslå och återkalla undantag; acceptera en restrisk | Ansvarig (`waiver.approve`) |
| Ändra projektets gräns för riskacceptans | Ansvarig (`project.update`) |

## Undantag och avvikelser

**Undantag** i projektets sidofält listar alla undantag och avvikelser med typ, status, vad de gäller för (en release eller hela projektet), kraven och slutdatumet. Filtrera på text, status, typ och release, eller kryssa i **Endast gällande**. Siffrorna ovanför listan visar hur många undantag som gäller, hur många som väntar på godkännande och hur många som går ut inom 30 dagar.

Välj **Nytt undantag** (eller **Begär ett undantag** vid ett krav eller på fliken Status för en release) och fyll i:

- **Typ**: *Undantag* (kraven är underkända, otestade eller inte uppfyllda, och det accepteras tills vidare) eller *Avvikelse* (de uppfylls med en annan lösning).
- **Gäller för**: en release, eller hela projektet.
- **Krav**: de krav som undantaget accepterar.
- **Motivering**, **Kompenserande åtgärder** (för en avvikelse: lösningen som används i stället) och **Villkor**.
- **Kopplad risk**: en befintlig risk, eller **Skapa en ny risk** med sannolikhet och konsekvens; den nya risken kopplas till samma krav.
- **Går ut**: den sista dag som undantaget gäller. Lämna tomt för inget slutdatum.
- **Godkännare**: e-postadresserna till de personer som alla måste godkänna. Lämna tomt så räcker det att en person med behörigheten `waiver.approve` beslutar.

Ett nytt undantag är ett **utkast**. **Begär godkännande** gör det **begärt** och meddelar godkännarna. Tills beslutet är fattat kan du fortfarande redigera det eller **Dra tillbaka** det till ett utkast.

### Godkänna eller avslå

Öppna ett begärt undantag och välj **Godkänn eller avslå**. Välj beslut, skriv en kommentar (krävs vid avslag) och bekräfta med ditt lösenord: det är en **elektronisk signatur** över undantagets exakta innehåll, som [signeringen av en baslinje](baselines.md). Om undantaget ändras efteråt räknas tidigare signaturer inte längre och visas som *ogiltiga*. Du kan inte besluta om ett undantag som du själv har begärt, och API-token kan inte signera.

Undantaget **godkänns** när alla angivna godkännare har godkänt det (eller, utan godkännare, efter ett godkännande) och **avslås** vid första avslaget. Den som begärde det och godkännarna får ett meddelande.

### Vad ett godkänt undantag gör

Så länge ett undantag är godkänt och slutdatumet inte har passerats **gäller** det:

- **Releaseberedskap**: [utgångskriterierna](releases-and-risks.md#utgångskriterier) behandlar dess krav som accepterade. *Täckning* och *Godkända* räknar dem som uppfyllda, *MUST- och SHALL-krav godkända* listar dem inte, och ett underkänt testfall som bara verifierar undantagna krav räknas inte för *Inga underkända testfall*. De listas separat under varje kriterium som *undantagna*, och fliken **Status** för releasen visar de gällande undantagen.
- **Analys och efterlevnad**: krav som omfattas av ett undantag för hela projektet visas separat (iakttagelsen *Krav som accepteras genom ett undantag*, och *Undantaget* i efterlevnadsanalysen) och räknas varken som luckor eller som uppfyllda.
- **Rapporter och dokument**: efterlevnadsrapporten och rapporten om releaseberedskap har ett avsnitt om undantag, och [kravspecifikationen](documents.md) markerar undantagna krav (**Undantaget genom**) och listar undantagen i en bilaga.

Ett krav visar sina undantag under sina risker i kravredigeraren.

### Utgång och återkallelse

Dagen efter slutdatumet blir ett godkänt undantag **utgånget**: dess krav accepteras inte längre, och den som begärde det och godkännarna får ett meddelande. En godkännare kan också när som helst **Återkalla** ett godkänt undantag, med en motivering. Ett godkänt undantag kan inte redigeras eller tas bort; återkalla det och begär ett nytt i stället. Varje steg loggas i granskningsloggen.

## Restrisk

[Riskregistret](releases-and-risks.md#riskregistret) ger varje risk ett värde som sannolikhet × konsekvens. Under **Restrisk efter åtgärder** i riskdialogen anger du den sannolikhet och konsekvens du räknar med när åtgärden fungerar:

- Restrisken börjar gälla när åtgärden är **verifierad**: varje krav som är kopplat till risken har klarat alla sina testfall. Till dess gäller det ursprungliga värdet.
- Kryssa i **Ange restrisken för hand** och skriv en **motivering** för att den ska gälla direkt, till exempel när åtgärden är organisatorisk.
- **Granskningsdatum** och **Ansvarig för granskning** planerar nästa granskning; en risk vars granskningsdatum har passerats markeras.

Kolumnen **Restrisk** i registret visar värdet och nivån som gäller, med en ikon för en verifierad eller handinmatad restrisk. Växla värmekartan mellan **Före åtgärder** och **Efter åtgärder**; en vald ruta filtrerar registret på den vyn.

### Gräns för riskacceptans och att acceptera en risk

**Gräns för riskacceptans** under värmekartan är den högsta restnivå som projektet accepterar utan signering (som standard *Medel*). Risker vars restnivå ligger över gränsen markeras med en varningsikon.

För att acceptera en sådan risk öppnar du den och väljer **Acceptera risken** *(waiver.approve)*: bekräfta med ditt lösenord för att signera. Risken blir *Accepterad*. Signaturen gäller restrisken så som den är bedömd: när bedömningen ändras visas acceptansen som *inaktuell* och räknas inte längre. **Dra tillbaka acceptansen** tar bort den.

En release markerar de risker i omfattningen vars restnivå ligger över gränsen på fliken **Status**. Lägg till utgångskriteriet **Restrisk** för att de ska stoppa releasen om de inte är accepterade; kriteriet kan använda projektets gräns eller en egen nivå. [Riskrapporten](reports-and-export.md) visar värmekartan före och efter åtgärder och listar riskerna över gränsen.

## Uttalande om tillämplighet

**SoA** i projektets sidofält listar varje åtgärd i en referenskälla. Välj källa högst upp: **ISO/IEC 27001:2022 bilaga A** har sina 93 åtgärder (A.5.1–A.5.37, A.6.1–A.6.8, A.7.1–A.7.14 och A.8.1–A.8.34, med korta namn). För andra lagar och standarder bland [katalogens källor](catalogue.md) är åtgärderna de avsnitt som katalogen och era krav hänvisar till.

För varje åtgärd visar tabellen:

- **Tillämplig**: *Tillämplig*, *Undantagen* eller *Ej beslutad*, med **Motivering** till varför den ingår eller inte (krävs för en undantagen åtgärd) och **Ägare**;
- **Införande**, som härleds från de krav som hänvisar till åtgärden: *Införd* (alla har klarat sina testfall), *Delvis införd* (några har klarat dem eller accepteras genom ett undantag), *Planerad* (det finns krav, inget har klarat sina testfall än) eller *Inte införd* (inget krav hänvisar till den);
- **Krav** och **Belägg** (testfallen som verifierar kraven).

Ett krav hänvisar till en åtgärd när det har lagts till från ett katalogkrav vars referenser anger åtgärden (till exempel `ISO/IEC 27001:2022 A.8.5`), eller när kravets eget fält **Källa** anger den.

Tabellen fungerar som [kalkylbladsvyn](requirements.md): skriv i cellerna **Tillämplig**, **Motivering** och **Ägare**, klistra in ett block från ett kalkylblad eller fyll nedåt med <kbd>Ctrl</kbd>+<kbd>D</kbd>. Varje ändring sparas direkt. Tryck <kbd>Ctrl</kbd>+<kbd>Enter</kbd> på en åtgärd (eller dubbelklicka på den i första kolumnen) för att se dess krav och belägg. Filtrera på text, tillämplighet, införande och grupp; siffrorna ovanför tabellen räknar åtgärderna.

**Ladda ned** skapar uttalandet som PDF, Word, Excel eller Markdown; **Spara i arkivet** lägger det i [dokumentarkivet](documents.md). Det finns också som standardrapporten **Uttalande om tillämplighet** på sidan **Rapporter**.
