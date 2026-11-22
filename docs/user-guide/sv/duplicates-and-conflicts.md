# Dubbletter och konflikter

I stora kravmängder hamnar samma krav ofta två gånger (skrivet av två personer, importerat både från ett kunddokument och från [katalogen](catalogue.md)), och en del krav motsäger varandra (*lösenord minst 12 tecken* och *högst 8*). Qtestframe letar efter båda: medan du skriver, i de [inbyggda konsistenskontrollerna](traceability-and-coverage.md#inbyggda-konsistenskontroller) och på sidan **Dubbletter och konflikter**, där du bestämmer vad som ska göras med varje par.

Allt beräknas på Qtestframe-servern. Ingen text lämnar den om inte en administratör har slagit på en [tjänst för inbäddningar](#tjänst-för-inbäddningar).

## Liknande krav medan du skriver

När du skapar eller redigerar ett krav visar **Liknande krav** under beskrivningen de fem krav i projektet som liknar det mest, vart och ett med likheten i procent, en stund efter att du slutat skriva. En likhet på eller över projektets dubblettgräns (68 % som standard) markeras: kravet finns troligen redan. Öppna det i en ny flik, eller fortsätt skriva om ditt säger något annat.

Testfallsredigeraren har samma panel, **Liknande testfall**, som jämför titel, beskrivning, förutsättningar och steg.

Avvisade och föråldrade krav och utfasade testfall jämförs inte. Panelen visas bara för den som får ändra objektet.

## Hur likheten mäts

Två krav liknar varandra när de använder samma viktiga ord, inte när de ser lika ut bokstav för bokstav:

- **Normalisering och böjningsformer.** Texterna jämförs med små bokstäver och utan skiljetecken; orden förkortas till sin stam på svenska och engelska (*användaren*, *användarna* → *använd*; *users*, *user* → *user*), så böjningar spelar ingen roll.
- **Stoppord.** Ord utan egen betydelse (*och*, *ska*, *the*, *shall* …) räknas inte. Förpliktelseorden jämförs i stället av konfliktsökningen.
- **Synonymer och förkortningar** inom området räknas som samma ord, även mellan språken: *logga in*, *inloggning*, *log in* och *sign in*; *användare*, *brukare*, *user* och *end user*; *e-post*, *mejl* och *e-mail*; *2FA*, *tvåfaktorsautentisering* och *two-factor*; *radera*, *ta bort*, *delete* och *remove*, med flera.
- **Sammansättningar.** Svenska sammansatta ord delas när en del är ett känt ord (*användarkonto* = *användare* + *konto*), och bitar om fyra bokstäver gör *lösenordsbyte* och *byte av lösenord* lika.
- **Ovanliga ord väger tyngre.** Ett ord som finns i några få krav i projektet säger mer än ett som finns i de flesta (TF-IDF över projektet).
- **Fältvikter.** Titeln väger mest, sedan beskrivningen och sist acceptanskriterierna (för testfall: förutsättningar, steg och förväntat resultat).

Resultatet är en likhet från 0 till 1 (visas i procent). Texter som bara skiljer sig i versaler och skiljetecken är 100 %.

## Möjliga konflikter

Två krav är **möjligen motstridiga** när de handlar om samma sak (de har tillräckligt många viktiga ord gemensamt) och:

| Slag | Exempel |
|---|---|
| **Motsatt förpliktelse** | *Användaren ska kunna exportera rapporter som PDF.* och *Användaren får inte kunna exportera rapporter som PDF.* (även *MUST* och *MUST NOT*, *ska* och *ska aldrig*) |
| **Gränsvärden** som inte kan gälla samtidigt | *svara inom 2 sekunder* och *en svarstid på minst 5 sekunder*; *minst 12 tecken* och *högst 8 tecken*; *at least 12 characters* och *at most 8 characters*. Enheter räknas om (2000 ms = 2 s, 15 minuters = 900 s), och en negativ förpliktelse vänder jämförelsen (*får inte överstiga 2 s* = högst 2 s) |
| **Uteslutande värden** | *aktiverad* och *avstängd* som standard, *obligatorisk* och *frivillig*, *krypterad* och *okrypterad*, *offentlig* och *privat*, *automatisk* och *manuell*, *enabled* och *disabled* … |

Varje konflikt har en **säkerhet** från 0 till 1, högre när de två påståendena har mer innehåll gemensamt. Förpliktelseorden känns igen som i [språkanalysen](requirements.md) (svenska och engelska). Ett par som är motstridigt rapporteras inte också som dubblett.

## Sidan Dubbletter och konflikter

Öppna **Dubbletter och konflikter** i projektets sidomeny. Välj **Krav** eller **Testfall** överst.

- **Möjliga konflikter** (krav) visar varje par med slag, säkerhet och de avgörande orden, och meningen i vart och ett av kraven med **beläggen markerade**: förpliktelseorden, gränsvärdena eller de uteslutande värdena.
- **Misstänkt dubblerade krav** (eller testfall) visar varje par med likheten och antalet spårlänkar för varje objekt.

**Likhet från** och **Säkerhet för konflikt från** ändrar vad som visas; de börjar på projektets gränser (de inbyggda kontrollerna). **Visa avfärdade** visar också paren du har avfärdat, så att du kan visa dem igen.

För varje par kan du:

| Åtgärd | Vad som händer |
|---|---|
| **A dubblerar B** | lägger till relationen *A dubblerar B* (för testfall: en objektlänk med etiketten *duplicates*). Paret är inte längre misstänkt. |
| **Slå ihop…** | välj vilket objekt som ska **behållas**; det andra slås ihop med det: dess spårlänkar, relationer (testfall: förutsättningar) och objektlänkar flyttas till objektet du behåller, det får statusen *Inaktuellt* (testfall: *Utfasat*) och markeras som dubblett av det behållna objektet. Båda får en kommentar om sammanslagningen, och statusändringen syns i historiken. Länkar som det behållna objektet redan har dubbleras inte. |
| **Markera som motstridiga** | lägger till relationen *står i konflikt med*. Därefter bevakas paret av kontrollen [Motstridiga krav i en release](traceability-and-coverage.md#inbyggda-konsistenskontroller). |
| **Ingen dubblett** / **Ingen konflikt** | avfärdar fyndet: paret visas inte igen på sidan eller i de inbyggda kontrollerna (det sparas per par och kontroll, även i säkerhetskopior och projektarkiv). **Visa igen** ångrar det. |

Att markera och slå ihop kräver behörighet att ändra krav (`requirement.write`) eller testfall (`test.write`). Alla som kan läsa objekten kan öppna sidan.

## Inbyggda kontroller

Sidan använder samma sökning som tre [inbyggda konsistenskontroller](traceability-and-coverage.md#inbyggda-konsistenskontroller): **Misstänkt dubblerade krav**, **Möjligen motstridiga krav** och **Misstänkt dubblerade testfall**. En överträdelse rapporteras på det nyare objektet i paret; åtgärderna är **Markera som dubblett**, **Markera som motstridiga** och **Inget problem**.

## Gränsvärden

| Inställning | Standard | Var |
|---|---|---|
| Likhet för en misstänkt dubblett | 0,68 | kontrollens regel (**Likhetsgräns**, 0,5 till 1) |
| Säkerhet för en möjlig konflikt | 0,6 | konfliktkontrollens regel (**Lägsta säkerhet**, 0,5 till 1) |
| Lägsta likhet som visas medan du skriver | 0,25 | fast |

Standardvärdena valdes på en märkt uppsättning svenska och engelska krav (omformulerade dubbletter, motsägelser av varje slag och närliggande krav som är varken eller, som *spärra kontot* och *låsa upp ett konto*): med dem hittas varje märkt dubblett och konflikt, och inget orelaterat par rapporteras. Höj dubblettgränsen till omkring 0,8 när kontrollen rapporterar krav som bara har samma ämne; sänk den mot 0,6 för att hitta omformuleringar med färre gemensamma ord. Likheten beror på projektet: ett ord som är ovanligt i ett litet projekt väger tyngre än i ett stort.

## Import från katalogen

Innan du lägger till krav från [katalogen](catalogue.md) varnar dialogen för de valda krav som liknar krav som projektet redan har, och kan hoppa över dem. Efteråt visar resultatet hur många nya krav som liknar befintliga.

## Tjänst för inbäddningar

En administratör kan låta en extern **tjänst för inbäddningar** (en OpenAI-kompatibel slutpunkt `/v1/embeddings`) beräkna likheten i stället (**Administration → Systeminställningar → Likhet**; se administratörsguiden). Sidan och panelen visar då *Tjänst för inbäddningar*. Titel, beskrivning och kriterier eller steg för krav och testfall **skickas då till tjänsten**. Vektorerna cachas per objektversion, så oförändrade objekt skickas inte igen. Konfliktsökningen körs alltid på servern. När tjänsten inte svarar används den lokala likheten.
