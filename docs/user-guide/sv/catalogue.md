# Katalog

**Katalogen** är ett inbyggt bibliotek med standardkrav och testfall för programvaruutveckling: säkerhet, integritet och regelefterlevnad, prestanda och tillförlitlighet, drift och underhåll, användbarhet och tillgänglighet (även formulär, användarinställningar och mobilappar), vanlig funktionalitet (även arbetsflöde och godkännanden, samarbete, filer, schemaläggning) samt domänspecifik funktionalitet som kassa i e-handel och fakturering. I stället för att skriva *"Lösenord ska vara minst 12 tecken"* och dess testfall i varje projekt väljer du de poster som passar och lägger till dem i projektet på en gång, med testfall och spårlänkar.

För att bläddra i katalogen behövs behörighet att läsa krav eller testfall (alla roller har den). Att lägga till krav kräver **Ansvarig** (`requirement.write`), att lägga till testfall **Testare** (`test.write`).

## Varifrån texterna kommer

Katalogens texter är skrivna för Qtestframe, på engelska och svenska. Varje post bygger på erkända standarder och god praxis, till exempel OWASP ASVS och OWASP Top 10, WCAG 2.2, ISO/IEC 25010, NIST SP 800-63B, dataskyddsförordningen (GDPR), ISO/IEC 27001, PCI DSS och EU:s AI-förordning, och **hänvisar** till dem under **Baserad på** med namn och avsnitt. Katalogen kopierar inte texten i någon standard: läs standarden när du behöver dess exakta lydelse.

För svenska organisationer omfattar gruppen *Integritet och regelefterlevnad* även säkerhetsskydd enligt säkerhetsskyddslagen (2018:585) och cybersäkerhetslagen, som genomför NIS2-direktivet, och grupperna *Säkerhet* samt *Drift och underhåll* omfattar samordnat offentliggörande av sårbarheter (ISO/IEC 29147, RFC 9116, CSAF) och sårbarhetshantering (ISO/IEC 30111, IEC 62443-4-1, EU:s cyberresiliensförordning). Posterna hänvisar till lagar och förordningar med namn och avsnitt; de hjälper er att planera och kontrollera men är inte juridisk rådgivning.

Varje krav är ett verifierbart påstående med exakt ett förpliktelseord (SKA, BÖR, FÅR … på svenska; MUST, SHOULD, MAY … på engelska, se [Krav](requirements.md)), utvärderingskriterier och en motivering. Varje testfall har förutsättningar, numrerade steg med ett förväntat resultat vardera och ett samlat förväntat resultat. Värden som ett team brukar justera (12 tecken, 2 sekunder, 99,9 %) står i texten, så att du kan ändra dem när posten har lagts till.

Varje krav anger minst ett testfall i katalogen som verifierar det, och varje testfall verifierar minst ett krav, så när du lägger till ett krav med dess testfall har du alltid ett sätt att kontrollera det. Varje post är komplett på både engelska och svenska, med samma kriterier och steg på båda språken.

## Bläddra och sök

Öppna **Katalog** i projektets sidomeny eller välj **Lägg till från katalogen** överst i listan med krav eller testfall.

- Växla mellan **Krav** och **Testfall** överst.
- Välj **English** eller **Svenska** som språk för texterna. Posterna läggs till på det språket.
- Till vänster visar **Grupper och områden** katalogens grupper (till exempel *Säkerhet*) och deras områden (till exempel *Autentisering*) med antalet poster som matchar sökningen och filtren. Välj **Allt** för att se alla områden. Flytta i trädet med piltangenterna och välj med Enter. På en smal skärm öppnas trädet med knappen **Områden**.
- **Sökfältet** hittar poster vars titel, ID, taggar, standarder eller beskrivning innehåller alla ord du skriver. De bästa träffarna kommer först: ett exakt ID som `AUTH-01`, sedan orden i titeln, sedan de andra fälten.
- Begränsa listan med **Typ**, **Förpliktelse** och **Prioritet** (krav), **Utförande** (testfall) och **Taggar**. **Dölj poster som redan finns i projektet** visar inte det du har lagt till tidigare.

Varje rad visar ID, titel, förpliktelse och prioritet samt hur många testfall i katalogen som verifierar kravet (eller hur många krav testfallet verifierar). **I projektet** markerar poster som projektet redan har; håll pekaren över märket för att se deras nycklar.

## Förhandsvisa en post

Välj en rad för att se hela texten till höger (på en mobil öppnas den som en panel): beskrivningen med förpliktelseordet markerat, motiveringen, utvärderingskriterierna med gränsvärden, standarderna och katalogens testfall som verifierar kravet. För ett testfall ser du förutsättningarna, stegen, det förväntade resultatet och kraven det verifierar. Välj ett ID under *Verifieras av* eller *Verifierar* för att gå till den posten.

## Lägg till poster i projektet

1. Markera de poster du vill ha. **Markera alla som visas** markerar allt i listan, så välj först området i trädet om du vill ta ett helt område. Du kan markera krav och testfall på båda flikarna; listen längst ned räknar dem.
2. Välj **Lägg till N krav (+ M testfall)**. M omfattar testfallen som verifierar de valda kraven.
3. Välj i dialogen:
   - **Språk** för texterna.
   - **Mapp för kraven** och **Mapp för testfallen** (översta nivån som standard).
   - **Undermappar**: inga, *en mapp per område* (till exempel *Autentisering*) eller *mappar för grupp och område* (*Säkerhet / Autentisering*). Befintliga mappar med samma namn återanvänds.
   - **Status** för kraven och testfallen (*Utkast* som standard, så att de granskas som andra nya poster).
   - **Källa för kraven**: de angivna standarderna (standard) eller en av projektets källor.
   - **Lägg även till testfallen som verifierar de valda kraven** (på som standard).
   - **Lägg till kopior av poster som redan finns i projektet**: av som standard, så att poster som projektet redan har hoppas över.
4. Sammanfattningen visar hur många krav och testfall som läggs till och hur många som hoppas över. Välj **Lägg till**.

Högst 500 poster kan läggas till åt gången. Importen sker helt eller inte alls: om något misslyckas läggs ingenting till.

Qtestframe skapar kraven och testfallen som om du hade skrivit dem, med en första version i historiken, och skapar **spårlänkar** mellan varje krav och testfallen som verifierar det, även till motsvarande poster som lagts till i projektet tidigare. Det automatiska länkade testfallsutkastet som ett nytt krav annars får skapas inte: katalogens testfall ersätter det.

## Efteråt

En ruta överst på sidan sammanfattar vad som lades till. **Öppna de tillagda kraven** och **Öppna de tillagda testfallen** öppnar listorna filtrerade på de nya posterna. **Ångra** (eller Ctrl+Z) flyttar dem till papperskorgen; gör om tar tillbaka dem med sina länkar. Mappar som importen skapade finns kvar.

De tillagda posterna är vanliga poster i projektet: anpassa texterna och värdena till ert system. Redigeraren visar **Från katalogen: AUTH-01 · …** med standarderna, och **Visa i katalogen** öppnar originalposten. Ursprunget behålls när du redigerar, kopierar, flyttar eller återställer posten. I filter hittar `catalog:AUTH-01` posterna som kopierats från en katalogpost, `catalog:AUTH-*` ett områdes poster och `has:catalog` alla poster från katalogen (se [Sökning och filter](search-and-filters.md)).

## Tips

- Starta ett nytt projekt från katalogen: lägg till de grupper som gäller och ta sedan bort eller anpassa det som inte passar.
- Använd statusen **Utkast** och en [massgranskning](requirements.md) för att enas om de anpassade posterna i teamet.
- Katalogen är en del av Qtestframe och växer med nya versioner. Poster som du har lagt till ändras inte när katalogen ändras.
