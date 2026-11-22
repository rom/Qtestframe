# Spårbarhet och täckning

Spårbarheten besvarar två frågor: *vilka testfall verifierar det här kravet?* och *har det verifierats med godkänt resultat?* Qtestframe besvarar dem med **länkar** mellan krav och testfall och med **täckningen** som härleds från testutfallen.

## Spårbarhetsmatrisen

Välj **Spårbarhet** i projektets sidopanel.

![Spårbarhetsmatrisen: krav i rader, testfall i kolumner, täckning per krav och senaste utfall per testfall](../images/sv-traceability.png)

- Kraven är **rader** och testfallen **kolumner**. En markerad (nedtryckt) cell betyder *länkad*.
- Varje markering färgas efter det länkade testfallets senaste utfall och har en egen symbol: ✓ Godkänd (grön), ✕ Underkänd (röd), ⊘ Blockerad (gul), » Överhoppad (lila), klocka Pågår (blå), – Ej körd (grå). En misstänkt länk har en gul ram och en varningstriangel. Förklaringen **Markeringar** beskriver dem; skärmläsare läser upp utfallet för varje cell. Vyn **Lista** färgar de länkade testfallen på samma sätt.
- Det färgade fältet först på varje rad visar kravets täckning; **Förklaring av täckning** förklarar färgerna. Peka på ett testfalls kolumnrubrik för att se nyckel, titel och senaste utfall.
- Sammanfattningen anger hur många krav som är länkade, till exempel *14/20 krav länkade (70 %)*.
- **Filtrera krav** och **Filtrera testfall** tar emot [frågor](search-and-filters.md), till exempel `folder:Kassa` och `type=automated`.
- **Bara länkade testfall** döljer testfall som inte är länkade till något av de visade kraven.
- **Vy** växlar mellan **Matris** och **Lista**. Listan visar varje krav med dess länkade testfall och är lättare att läsa på en mobil. **Regler** visar [spårbarhetsreglerna](#spårbarhetsregler).

**Länka och ta bort länkar:** klicka på (eller flytta fokus till och tryck <kbd>Enter</kbd> eller <kbd>Blanksteg</kbd> på) en tom cell för att länka kravet och testfallet; klicka på en markerad cell för att ta bort länken. Det kräver rollen **Testare**. Länkar kan också hanteras i kravredigeraren (**Länka testfall**) och testfallsredigeraren (**Länka krav**).

Matrisen rullar i båda riktningarna; kravkolumnen förblir synlig.

### Länka flera på en gång

Med rollen **Testare** finns ett verktygsfält ovanför matrisen (och listan) som arbetar med många länkar på en gång:

1. Kryssa i krav med kryssrutan först på raden, eller **Markera alla visade krav** (använd filtren först för att avgränsa).
2. Välj eventuellt testfall i **Testfall för de markerade kraven**; utan valda testfall gäller åtgärderna alla testfall.
3. Välj en åtgärd; talet på varje knapp anger hur många länkar den påverkar:
   - **Länka** länkar varje markerat krav till varje valt testfall; befintliga länkar behålls.
   - **Ta bort länkar** tar bort länkarna mellan de markerade kraven och de valda testfallen (efter en bekräftelse).
   - **Bekräfta misstänkta** bekräftar de misstänkta länkarna bland dem (celler med en varningssymbol: kravet eller testfallet har ändrats sedan länken bekräftades).

Väljer du bara testfall (inga krav) gäller **Ta bort länkar** och **Bekräfta misstänkta** alla länkar för de testfallen. Att länka och ta bort länkar kan ångras med **Ångra**; statusen som följer bevisen uppdateras en gång för alla berörda krav.

## Spårbarhetsregler

Regler anger hur ett projekts objekt ska spåras, så att luckor syns långt före en revision eller en release. En regel säger: *de objekt som matchar en fråga måste ha minst N spår av ett visst slag*, till exempel *varje MUST-krav verifieras av minst ett testfall*.

Välj **Spårbarhet** i projektets sidofält och **Vy → Regler**. Varje regel visar:

- en grön bock när inget objekt bryter mot den, ett rött kryss för ett brutet **fel** och ett gult för en bruten **varning**;
- vilka objekt den gäller och vad de behöver, till exempel *Krav som matchar obligation:must behöver minst 1 länkat testfall (inte misstänkta)*;
- hur många objekt som matchar och hur många av dem som bryter mot den. **Visa överträdelser** listar objekten som bryter mot regeln och vad som saknas (*har 0, behöver 1*, eller *1 av 2 räknas, behöver 1* när några spår inte uppfyller villkoren); välj en nyckel för att öppna objektet.

**Bara överträdelser** döljer regler som uppfylls och regler som är avstängda; **Filtrera regler och objekt** hittar en regel efter namn eller ett objekt efter nyckel eller titel. Kravens och testfallens redigerare visar reglerna som objektet bryter mot ovanför fälten, med en länk till reglerna.

### Skapa och redigera regler

Regler hanteras av användare som får ändra projektinställningarna (behörigheten `project.update`, som standard **Ansvarig** och **Administratör**); alla andra ser resultaten. Välj **Ny regel**, välj eventuellt en förinställning under **Utgå från** och fyll i:

| Fält | Betydelse |
|---|---|
| **Namn**, **Beskrivning** | vad regeln är till för |
| **Allvarlighet** | **Fel** eller **Varning**. Fel gör att reglerna inte uppfylls; utgångskriterier för releaser kan räkna bara fel eller även varningar |
| **Regeln är på** | avstängda regler listas men räknas inte |
| **Gäller** | krav eller testfall |
| **Objekt som regeln gäller** | en [fråga](search-and-filters.md), till exempel `obligation:must type:security` eller `folder:Payments`; tomt låter regeln gälla alla objekt |
| **Måste ha** | slaget av spår: **Testfall som verifierar det** (ett kravs spårlänkar), **Krav som det verifierar** (ett testfalls spårlänkar), **Relationer till krav** eller **Objektlänkar** |
| **Minst** | hur många spår ett objekt behöver (standard 1) |
| **Relationstyp**, **Riktning** | för relationer: till exempel *Förfinar* och *inkommande* betyder *förfinas av underordnade krav*; *valfri relation* och *båda riktningarna* räknar alla relationer |
| **Länk till**, **Riktning** | för objektlänkar: vad länken ska peka på (ett krav, testfall, en körning, fil, rapport, webbadress eller vad som helst) |
| **Bara … som matchar** | en fråga på objektet i andra änden, till exempel `type:functional` för att bara räkna funktionella underordnade krav |
| **Bara länkar som inte är misstänkta** | misstänkta länkar räknas inte |
| **Bara testfall vars senaste utfall är godkänt** | testfall som inte är godkända räknas inte |
| **Varje spår måste uppfylla villkoren** | ett objekt bryter mot regeln så fort ett av dess spår inte uppfyller villkoren (till exempel har en misstänkt länk). Då får **Minst** vara 0 |

**Prova** utvärderar regeln utan att spara den och visar hur många objekt som matchar och bryter mot den. Använd ögat för att stänga av och slå på en regel, pennan för att redigera den och papperskorgen för att ta bort den; allt detta kan ångras.

Förinställningarna:

| Förinställning | Regel |
|---|---|
| Varje MUST-krav verifieras av ett testfall | krav som matchar `obligation:must OR obligation:shall` behöver minst 1 länkat testfall (fel) |
| Varje MUST-krav har ett godkänt testfall | som ovan, men testfallet får inte vara misstänkt och dess senaste utfall måste vara godkänt (fel) |
| Inga misstänkta länkar på MUST-krav | ingen länk för ett MUST- eller SHALL-krav får vara misstänkt, minst 0 (varning) |
| Varje testfall verifierar ett krav | varje testfall är länkat till minst 1 krav (varning) |
| Verksamhetskrav förfinas av minst ett krav | krav som matchar `type:business` har minst 1 inkommande *förfinar*-relation (varning) |
| Säkerhetskrav verifieras av två testfall | krav som matchar `type:security` behöver minst 2 länkade testfall (varning) |

Reglerna utvärderas också på en releases omfattning av utgångskriteriet [Spårbarhetsregler](releases-and-risks.md#utgångskriterier), och rapporten **Spårbarhet** listar varje påslagen regel med resultat och de objekt som bryter mot den.

### Inbyggda konsistenskontroller

Utöver de regler du skriver har Qtestframe **inbyggda kontroller** med fasta villkor. De hittar objekt vars tillstånd motsäger deras relationer, länkar eller release:

| Kontroll | Ett objekt bryter mot den när |
|---|---|
| Motstridiga krav i en release | två krav med relationen *står i konflikt med* är båda godkända (eller implementerade eller verifierade) och i samma release |
| Godkänt krav med ouppfyllt beroende | ett godkänt krav *beror på* ett krav som är avvisat, föråldrat eller borttaget (finns kvar i papperskorgen) |
| Aktivt krav med föråldrad förälder | ett krav som *förfinar* eller är *härlett från* ett föråldrat krav är inte själv avvisat eller föråldrat |
| Testfall verifierar ett inaktivt krav | ett testfall verifierar ett avvisat eller föråldrat krav |
| Trasig testförutsättning | en förutsättning för ett testfall är borttagen, underkändes senast, eller förutsättningarna bildar en cykel |
| SKA-krav i en release utan godkänt belägg | ett MUST- eller SHALL-krav i en planerad eller aktiv release har inget länkat testfall som inte är misstänkt och som godkändes senast |
| Hög risk utan verifierad åtgärd | en hög eller kritisk risk (inte stängd eller accepterad) saknar länkat krav, eller har krav som inte är verifierade (en risk vars krav alla är verifierade räknas som åtgärdad) |
| Regelverkskrav avvisat utan motivering | ett krav som lagts till från ett regelverk i [katalogen](catalogue.md) (en lag eller ett direktiv) är avvisat och har ingen kommentar och ingen egen motivering |
| Misstänkt dubblerade krav | två aktiva krav säger samma sak: titel, beskrivning och kriterier liknar varandra (minst 0,68 som standard, synonymer och böjningsformer på svenska och engelska inräknade) och de har ingen relation *dubblerar*. Se [Dubbletter och konflikter](duplicates-and-conflicts.md) |
| Möjligen motstridiga krav | två aktiva krav motsäger troligen varandra: samma påstående med motsatta förpliktelser (SKA och FÅR INTE), gränsvärden för samma storhet som inte kan gälla samtidigt (≤ 2 s och ≥ 5 s) eller olika värden ur en uppsättning som utesluter varandra (aktiverad och avstängd), med en säkerhet på minst 0,6 som standard, och de har ingen relation *står i konflikt med* |
| Misstänkt dubblerade testfall | två aktiva testfall säger samma sak (titel, beskrivning, förutsättningar och steg) och är inte länkade som dubbletter |

Ett projekt får dem med **Lägg till inbyggda kontroller** (bland reglerna eller under **Analys**): de läggs till som regler med allvarligheten *varning*, så de syns i utgångskriteriet [Spårbarhetsregler](releases-and-risks.md#utgångskriterier) först när det räknar varningar, eller när du höjer en kontroll till *fel*. En inbyggd kontroll är märkt **Inbyggd**; du kan byta namn, ändra allvarlighet, stänga av den, begränsa den med en fråga och, för dubblettkontrollerna, ange **Likhetsgräns** (0,5 till 1), för konfliktkontrollen **Lägsta säkerhet** (0,5 till 1) eller, för riskkontrollen, **Lägsta risknivå**.

Varje överträdelse visar de andra berörda objekten (det motstridiga kravet och releasen, den föråldrade föräldern, förutsättningen som underkändes …), var och en som länk, och föreslår en åtgärd där det finns en självklar: **Ta bort relationen**, **Ta bort länken**, **Ta bort förutsättningen**, **Ta bort från** releasen, **Markera som föråldrat**, **Registrera en motivering**, **Länka en åtgärd**, för en misstänkt dubblett **Markera som dubblett** (lägger till relationen *dubblerar*; för testfall en objektlänk med etiketten *duplicates*), för en möjlig konflikt **Markera som motstridiga** (lägger till relationen *står i konflikt med*) och **Inget problem** (paret rapporteras inte igen). [Dubbletter och konflikter](duplicates-and-conflicts.md) visar beläggen och kan också slå ihop dubbletter. **Inget problem** kvitterar också ett beroende på ett borttaget objekt.

## Ändringspåverkan

En ändring av ett krav stannar sällan hos det kravet. När **innehållet** i ett krav ändras (titel, beskrivning, kravnivå eller kriterier) markerar Qtestframe det som kanske inte längre stämmer:

- testfallen som verifierar det: deras länkar blir [misstänkta](#spårbarhetsmatrisen);
- kraven som är relaterade till det: som standard de som **förfinar**, är **härledda från** eller **beror på** det. Deras relation blir misstänkt, liksom länkarna till deras testfall;
- dess ärende i ärendehanteraren: det markeras som **inaktuellt** tills du uppdaterar ärendet från kravet (**Uppdatera ärendet**) eller väljer **Ärendet stämmer fortfarande**;
- de genererade dokumenten i arkivet som innehåller det: de visas som **Inaktuellt** i **Dokument**, med objekten som ändrats sedan;
- de senaste utfallen för dess testfall: de blir [inaktuella](#inaktuella-utfall).

En ändring av ett testfalls titel, beskrivning, förutsättningar, steg, förväntade resultat, skript eller körningstyp gör dess länkar misstänkta och dess senaste utfall inaktuellt.

### En ändring sprids vidare

Det som en ändring gör misstänkt kan i sin tur ändras, så påverkan sprids vidare: när A ändras blir B (som förfinar A) misstänkt, sedan C (som förfinar B) och så vidare, upp till projektets **påverkansdjup** (3 som standard, högst 10; se [Hur långt en ändring sprids](#hur-långt-en-ändring-sprids)). Varje post nås en gång, längs den kortaste vägen, även när relationerna bildar en cykel, och dialogerna visar **vägen** dit: *A → förfinas av → B → förfinas av → C → verifieras av → T-12*.

Utöver kravrelationerna och testfallen når en ändring, som **Granskning rekommenderas**:

- **riskerna** som ett ändrat krav hanterar (och, från en risk, de andra krav som hanterar den);
- de **öppna defekterna** för ett påverkat krav eller testfall;
- krav och testfall som är kopplade med **länkar**, och länkade körningar;
- testfall som **anropar** ett ändrat testfall som delade steg eller **beror på** det;
- **varianterna** som en påverkad post gäller (när de är valda uttryckligen) och **kodändringarna** (commits, merge requests) som hänvisar till den.

De markeras inte; deras ägare kan aviseras. Ett testfall som ändras påverkar kraven som det verifierar, inte det som ligger bortom dem. En ändring av bara **statusen** (till exempel i en statusändring för flera) rekommenderar bara granskningar. Ärenden, dokument och utfall hör till den ändrade posten själv.

### Innan du sparar

Varje ändring går igenom samma förhandsgranskning: när du sparar ett krav eller testfall, gör en **massredigering** eller **statusändring för flera** i listan, en **Excel**- eller **ReqIF**-import, **återställer en version** från historiken eller **återställer poster från en baslinje**. När något skulle påverkas visar en dialog det som ett träd grupperat per typ och djup, med antalen per typ och per djup, och för varje post om den **Blir misstänkt** eller bara **Granskning rekommenderas**, och vägen som ändringen når den längs. Välj vad som ska göras:

| Val | Vad som händer |
|---|---|
| **Markera misstänkt** (standard) | Markerar det som projektets regler markerar misstänkt, markerar ärendena som inaktuella och aviserar ägarna till posterna som rekommenderas för granskning |
| **Avisera bara ägarna** | Markerar ingenting; ägarna (kravens och riskernas ägare, defekternas tilldelade, testfallens författare) får en avisering var med sina påverkade poster |
| **Hoppa över** | Markerar ingenting och aviserar ingen. Utfall som registrerats före ändringen blir ändå inaktuella och dokument blir ändå inaktuella: det följer av själva ändringen |

Välj **Spara** för att spara med ditt val, eller **Avbryt** för att inte ändra något. Poster som redan är markerade visas som sådana och räknas inte igen.

Via API:t tar varje ändrande anrop `?dry_run=impact` (ändringen görs i en transaktion, dess påverkan returneras och allt rullas tillbaka) och `?impact=suspect|notify|skip`.

### Fliken Påverkan

Fliken **Påverkan** för ett krav eller testfall visar samma lista när som helst, under **Om kravet ändras** (eller **Om testfallet ändras**). För ett krav visar den också:

- **Blockerande defekter**: de öppna defekter som blockerar det eller varnar för det (se [Blockerande defekter](#blockerande-defekter));
- **Misstänkt nu**: dess misstänkta relationer, med **Bekräfta** för var och en (och **Bekräfta alla**), och de länkade testfall vars utfall är inaktuellt.

Märkena bredvid kravets nyckel (**Blockeras av 1 defekt**, **1 inaktuellt utfall**, **1 misstänkt relation**) öppnar fliken. För ett testfall visar fliken också dess **Regressionshistorik** (se [Defekter](defects.md#regressioner)). **Påverkansanalys** öppnar posten i [påverkansanalysen](#påverkansanalys).

### Påverkan sedan en baslinje

Under **Baslinjer**, jämför en baslinje med nuläget och välj **Påverkan**. Dialogen listar varje krav och testfall vars innehåll ändrats sedan baslinjen, med störst påverkan först, och summan av de objekt som påverkas. Öppna ett objekt för att se vad dess ändring gör misstänkt eller inaktuellt.

### Påverkansanalys

**Påverkansanalys** (bland projektets avsnitt) visar påverkan av en eller flera poster, eller av allt som ändrats sedan en baslinje, utan att ändra något. Välj **Krav** eller **Testfall** för att välja poster, eller välj en baslinje under **Ändringar sedan en baslinje**, ange **Påverkansdjup** och välj **Analysera**. Resultatet visar antalen per typ och djup och antingen en **Graf** (de ändrade posterna till vänster, varje påverkad post till höger om posten den nås från; streckad: granskning rekommenderas) eller en **Tabell** med typ, djup, effekt, ägare och väg för varje post. **CSV** och **PDF** exporterar analysen. Sidans adress behåller valet, så du kan skicka en analys som en länk.

### Genom vilka relationer

**Kategorier & inställningar → Ändringspåverkan** avgör, per relationstyp, vilket krav som blir misstänkt när kravet i andra änden ändras, och om det blir **misstänkt** eller bara får en **rekommenderad granskning**:

| Inställning | Betydelse |
|---|---|
| **Inkommande (kravet som pekar på det)** | A *förfinar* B: när B ändras blir A misstänkt (standard för förfinar, härlett från och beror på) |
| **Utgående (kravet det pekar på)** | A *tillhandahåller* B: när A ändras blir B misstänkt |
| **Båda riktningarna** | en ändring i endera änden gör den andra misstänkt |
| **Av** | ändringar sprids inte genom typen (standard för övriga typer) |

Läget bredvid varje typ (**Blir misstänkt** eller **Granskning rekommenderas**) gäller allt som nås genom den: en post är misstänkt bara när varje steg på vägen markerar misstänkt.

**Markera ärendena för ett ändrat krav som inaktuella** kan stängas av.

### Hur långt en ändring sprids

**Påverkansdjup** (1 till 10, standard 3) är antalet steg mellan krav, testfall och risker som en ändring sprids. Djup 1 är bara den direkta påverkan: kraven som är relaterade till det ändrade och testfallen för båda. De verifierande testfallen för ett påverkat krav och defekter, varianter och kodändringar för en påverkad post räknas inte som ett steg.

**Andra kopplingar** slår på eller av de andra slagen av koppling: spårlänkar (de kan markera misstänkt eller bara rekommendera granskning), delade steg, testberoenden, länkar, risker, defekter, varianter och kodändringar (de rekommenderar bara granskning).

## Inaktuella utfall

Ett testutfall blir **inaktuellt** när

- det är äldre än projektet tillåter (som standard 90 dagar; **Utfall blir inaktuella efter** kan ändras eller stängas av), eller
- testfallet, eller ett krav som det verifierar, ändrades i innehållet efter att utfallet registrerades (**Utfall blir inaktuella när testfallet eller ett krav det verifierar ändras**).

Ett inaktuellt utfall räknas inte som godkänt underlag. Ett krav blir inte **Verifierat** på grund av det (och ett verifierat krav går tillbaka till **Implementerat**), täckningen räknar det som ej kört, och releasens utgångskriterier **Klarade testfall** och **Obligatoriska krav klarade** listar kravet med *inaktuellt utfall*. Inaktuella utfall har märket **Inaktuellt** i testfallslistan, i täckningskolumnen i kravlistan, i spårbarhetsmatrisen och i rapporter. Kör testfallet igen för att få ett färskt utfall.

I sökningen hittar `stale:true` testfall med ett inaktuellt senaste utfall och `stale>0` krav med ett sådant.

## Blockerande defekter

En öppen defekt som är länkad till ett krav, till ett testfall som verifierar det eller till en körning av ett sådant testfall **blockerar** kravet, eller **varnar** bara för det, beroende på allvarlighetsgraden. Som standard blockerar kritiska och större defekter, mindre varnar och triviala gör ingetdera; **Kategorier & inställningar → Ändringspåverkan → Defekter som blockerar krav** ändrar detta per projekt.

Blockerade krav visar **Blockerat** i kravlistan och i spårbarhetsmatrisen, kravet visar **Blockeras av 1 defekt** bredvid nyckeln, och avsnittet **Kvalitet** i översikten räknar blockerade krav, krav med varningar, inaktuella utfall och misstänkta relationer. I sökningen hittar `blocked>0` blockerade krav. Releasens utgångskriterium **Inga blockerande defekter** är inte uppfyllt så länge ett krav i omfattningen är blockerat.

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

**Konfigurationer:** när testfall exekveras i [konfigurationer](runs.md#konfigurationer-i-en-körning) räknas varje konfiguration som ett testfall är planerat i **som ett eget utfall**: testfallets senaste utfall *i den konfigurationen* (en exekvering utan konfiguration räknas som ytterligare en konfiguration). Reglerna ovan tillämpas sedan på alla dessa utfall tillsammans. Ett krav blir alltså **Godkänd** bara när vart och ett av dess testfall har godkänts i **varje konfiguration** det är planerat i; en underkänd eller blockerad konfiguration gör det **Underkänd**; ett testfall som har godkänts i Chrome men inte körts i Firefox än gör det **Delvis**. En konfiguration räknas som planerad så snart testfallet har en exekvering i den i någon körning i projektet, även en som inte har körts än.

- REQ-6 är länkat till TC-5, planerat i *Chrome* (godkänd) och *Firefox* (godkänd) → **Godkänd**.
- REQ-7 är länkat till TC-6, planerat i *Chrome* (godkänd) och *Firefox* (ej körd) → **Delvis**.
- REQ-8 är länkat till TC-7, planerat i *Chrome* (godkänd) och *Safari* (underkänd) → **Underkänd**.

Utan konfigurationer ändras ingenting: varje testfall har ett utfall, sitt senaste utfall. En release räknar ett testfall som godkänt bara när det har godkänts i varje konfiguration i releasens körningar.

**Täckning per konfiguration:** kravredigeraren visar under de länkade testfallen en tabell med det senaste utfallet för varje länkat testfall i varje konfiguration och täckningen **i varje konfiguration** (där bara testfall som är planerade i den räknas). I spårbarhetsmatrisen visar **Täckning i konfiguration** täckningen och testfallens utfall i en enda konfiguration, eller för exekveringarna utan konfiguration.

Tänk på följande:

- Täckningen beräknas per krav, inte per utvärderingskriterium; alla länkar räknas lika. Vilka kriterier testfallen och andra belägg verifierar visas under [Verifiering](#verifiering).
- Bara körningar i samma projekt räknas. Ett testfall som har flyttats från ett annat projekt behåller sina gamla exekveringar, men de räknas inte längre.
- Ett nyare utfall *Godkänd* ersätter ett äldre *Underkänd*; du behöver inte ta bort gamla körningar.

Täckningen visas som kolumn och snabbfilter i kravlistan, som frågefältet `coverage` (till exempel `coverage:failed` eller `coverage:none category:Security`), i matrisen, i exporter, i rapporter och på översikten.

## Verifiering

Täckningen visar om de länkade testfallen är godkända. **Verifiering** går ett steg längre: ett krav räknas som **verifierat** först när **varje utvärderingskriterium har godkända belägg**, och – i en hierarki – när varje krav som förfinar det också är verifierat. Belägg är ett godkänt testfall, ett godkänt teststeg eller en **verifieringspost** för en inspektion, analys, demonstration eller granskning. Därmed kan även krav på hårdvara, lokaler och styrning som inget testfall kan verifiera bli verifierade.

Öppna ett krav och välj fliken **Verifiering**. Den visar läget med skälen när kravet inte är verifierat (till exempel *Kriterium 2 saknar belägg.*), beläggen för varje kriterium, kraven som förfinar det, verifieringsposterna, kriterierna för de länkade testfallen och systemelementen kravet är allokerat till. Märket bredvid täckningen i redigerarens sidhuvud öppnar fliken.

| Läge | När |
|---|---|
| **Verifierat** | varje kriterium (och varje underkrav) har godkända belägg |
| **Delvis verifierat** | en del är verifierat, inte allt |
| **Underkänt** | något belägg underkändes: ett testfall underkändes eller blockerades, en post har utfallet *Underkänd* |
| **Inte godkänt än** | det finns belägg, men inget är godkänt än (ej kört, en misstänkt länk, ett steg som inte körts) |
| **Inga belägg** | inget testfall och ingen verifieringspost |

### Testfall för kriterier och teststeg

Ett testfall som är länkat till ett krav kan kopplas till de kriterier det verifierar. Välj **Koppla kriterier** bredvid testfallet på fliken **Verifiering**, kryssa i kriterierna och, om du vill, det **steg** i testfallet som visar det. Ett kriterium som är kopplat till ett steg är godkänt när det steget godkändes i testfallets senaste exekvering. När du länkar ett testfall kan du också välja ett kriterium direkt (väljaren bredvid **Länka testfall**).

En länk **utan** kriterier räknas för hela kravet, som tidigare: för varje kriterium som saknar egna belägg. Ett projekt som vill att varje kriterium har egna belägg slår på **strikt kriterietäckning** under **Inställningar → Projekt → Verifiering**. Ett underkänt testfall underkänner alltid kravet, kopplat eller inte. Där kan ett projekt också låta utvärderade [utforskande sessioner](exploratory-testing.md#sessioner-som-verifieringsbevis) räknas som belägg för kraven i deras omfattning (avstängt som standard).

### Verifieringsposter

För ett kriterium som verifieras genom inspektion, analys, demonstration eller granskning väljer du **Registrera belägg för kriterium n** (eller **Registrera belägg** för hela kravet) och fyller i:

- **Metod** och **Utfall**: *Godkänd*, *Underkänd* eller *Godkänd med villkor* (räknas som godkänd och visas med villkor);
- **Utförd av** och **Datum**;
- en **Anteckning** och **Dokument från arkivet** (till exempel inspektionsprotokollet som laddats upp under [Dokument](documents.md));
- **Giltig till**: posten räknas inte efter den dagen; **Giltig tills kravet ändras**: när titel, beskrivning, kravnivå eller kriterier ändras ogiltigförklaras posten (som en misstänkt länk) och behöver registreras igen.

För varje kriterium räknas bara den nyaste giltiga posten; äldre visas som ersatta. En andra person kan **kontrasignera** en post (den som har behörighet att granska eller exekvera, utom postens författare och den som utförde den). En ändring av metod, utfall, kriterium, datum eller utförare tar bort kontrasigneringen.

### Kravhierarki och sammanräkning

Hierarkin byggs av relationen **förfinar**: ett krav som förfinar ett annat är dess underkrav (se [Relationer](requirements.md)). Ett överordnat krav är verifierat först när dess egna kriterier är verifierade och alla underkrav (utom avvisade och inaktuella) är det. Ett överordnat krav utan egna kriterier och belägg är verifierat när alla dess underkrav är det.

**Verifiering** i projektets navigering visar hierarkin som ett träd med märken: läget, verifierade kriterier (*2/3 kriterier*) och verifierade underkrav (*1/2 underkrav*). **Bara ej verifierade** behåller grenarna som fortfarande behöver arbete.

Med standardinställningen följer statusen beläggen (**Administration → Testexekvering**, se [Inställningar för testexekvering](administration.md#inställningar-för-testexekvering)): ett implementerat krav blir verifierat när det är verifierat, och ett verifierat krav går tillbaka till implementerat när ett underkrav eller kriterium inte längre är det – så ett verifierat överordnat krav med ett underkrav som inte är verifierat kan inte bli kvar. När organisationen verifierar manuellt listas ett sådant krav under **Verifiering → Inkonsekvenser** och i [kravanalysen](#kravanalys) (*Verifierade krav med underkrav som inte är verifierade*).

### Systemelement

**Verifiering → Systemelement** innehåller projektets produktuppdelning: system, delsystem, komponenter, produkter och leverantörer, som ett träd. Krav och testfall allokeras till element på fliken **Verifiering** i ett krav (och via API:t). Varje element visar hur många av kraven som är allokerade till det **eller till ett element under det** som är verifierade (*42/50 verifierade*), hur många som har belägg eller är underkända, och hur många av dess testfall som är godkända. Siffrorna länkar till listorna filtrerade på elementet.

I frågespråket hittar `element:"Kraftenhet"` kraven och testfallen som är allokerade till *Kraftenhet* eller till ett element under det, och `verification:verified`, `verification:none` … filtrerar på verifieringsläget.

Rapporten **Verifiering** och kravspecifikationen visar samma sak: rapporten har beläggen per kriterium, kraven som inte är verifierade och varför, posterna, sammanräkningen, inkonsekvenserna och täckningen per element; specifikationen visar verifieringen av varje krav och kriterium.

## Översikten

**Översikt** är den första sidan i varje projekt. Den börjar med tre översiktspaneler som visas som flikar: **Översikt** med läget i dag, **Trender** med hur det har förändrats och **Kvalitet** med det som behöver åtgärdas. Varje översiktspanel är ett rutnät av widgetar som du kan ordna om och bygga ut, och ett projekt kan ha fler översiktspaneler (se [Anpassa översiktspaneler](#anpassa-översiktspaneler)). Fliken ingår i adressen, så en länk kan öppna översikten på en viss flik. På en mobil visas widgetarna under varandra.

![Översikten med nyckeltal, kravtäckning, utfall av exekveringar och de senaste körningarna](../images/sv-dashboard.png)

**Översikt**

- **Nyckeltal**: antal krav (och hur många som saknar täckning), täckning (hur många som täcks av testfall), antal testfall (manuella och automatiserade), testkörningar (som pågår) och andelen godkända av alla exekveringar.
- **Kravtäckning**: hur många krav som är *Godkänd*, *Underkänd*, *Delvis*, *Ej körd* och *Ingen täckning*.
- **Utfall av exekveringar**: alla utfall i alla körningar.
- **Framsteg i aktiva körningar**: en stapel per planerad eller pågående körning (de åtta nyaste), uppdelad i godkända, underkända, blockerade, överhoppade och ej körda.
- **Senaste körningar**: de fem nyaste körningarna med förlopp; **Alla körningar →** öppnar listan över körningar.
- **Krav per status** och **Krav per kravnivå** (krav utan kravord räknas som *ospecificerade*).
- **Testfall per status**.
- **Fler nyckeltal** (under de första): **Verifierade krav** (andelen krav vars länkade testfall alla är godkända), **Öppna risker** (med hur många som är kritiska och höga), **Öppna defekter** (med deras medelålder), **Misstänkta länkar** (länkar vars krav eller testfall har ändrats sedan dess, och hur många krav som har en, se [spårbarhetsmatrisen](#spårbarhetsmatrisen)), **Överträdelser av spårbarhetsregler** (objekt som bryter mot de aktiverade [spårbarhetsreglerna](#spårbarhetsregler), och hur många regler som bryts) och **Releasens framsteg** (andelen exekveringar med utfall i den aktuella releasen).
- **Manuella och automatiserade testfall per senaste utfall**: en stapel för manuella och en för automatiserade testfall, uppdelade i godkända, underkända, blockerade, överhoppade och ej körda.
- **Kravens granskningsstatus**: kraven efter status på sin senaste granskningsbegäran: *Inte granskat*, *Granskning pågår*, *Ändringar begärda* eller *Godkänt* (avbrutna begäranden räknas inte). Se [Granskningar](reviews-and-notifications.md).
- **Releasens exekveringsframsteg**: för den aktuella releasen (den aktiva, annars den planerade med tidigast måldatum) utfallen av alla exekveringar i dess körningar, och hur många som har utfall och är godkända. Releasens namn öppnar den.

På översikten räknas ett krav som *täckt* så snart minst ett testfall är länkat, oavsett utfall. *Verifierade* krav är de vars länkade testfall alla är godkända.

**Trender**

- **Täckning och andel godkända över tid**: täckningen (andelen krav med länkade testfall) och andelen godkända (andelen exekverade testfall vars senaste utfall är godkänt) per dag. Välj period ovanför diagrammet: 30, 90 eller 365 dagar.
- **Öppnade och stängda defekter per vecka**: nya defekter och defekter som har lösts, stängts eller avvisats, per vecka (måndag till söndag).
- **Aktivitet de senaste 30 dagarna**: per dag ändringarna av krav och testfall (varje sparad version), de avslutade exekveringarna och de rapporterade defekterna. Diagrammet visar alltid de senaste 30 dagarna, oavsett vald period.
- **Burn-up för release**: för den release som är vald i listan visas kraven i omfattningen, hur många av dem som har länkade testfall och hur många vars testfall alla är godkända i releasens körningar, dag för dag från det att releasen skapades. Releasens måldatum står ovanför diagrammet. Se [Releaser](releases-and-risks.md).

Trenderna bygger på en ögonblicksbild av siffrorna som Qtestframe sparar en gång om dagen för varje projekt (och när översikten öppnas och dagens bild saknas). Ett nytt projekt, eller ett projekt från tiden före funktionen, börjar inte tomt: vid den första ögonblicksbilden återskapas de tidigare dagarna utifrån det som har datum – när krav, testfall och länkar skapades, utfallen i körningarna och när defekter rapporterades och löstes (upp till ett år bakåt). Dagar utan ögonblicksbild överbryggas av linjen.

**Kvalitet**

- **Instabila testfall**: de tio testfall vars utfall oftast växlar mellan godkänt och underkänt, med de senaste utfallen (äldst först; en ring markerar varje växling) och när utfallet senast växlade. Klicka på ett testfall för att öppna det. Se [Instabila testfall](tests.md#instabila-testfall).
- **Öppna defekter per allvarlighetsgrad**: öppna och påbörjade defekter per allvarlighetsgrad.
- **Kravtäckning per grupp**: täckningen per mapp på översta nivån (krav i undermappar räknas till den översta mappen) eller, med **Kategori**, per kravkategori; de tolv största grupperna.
- **Öppna risker per nivå**: öppna risker (*Identifierad* eller *Åtgärdas*) per nivå; **Riskregister →** öppnar [riskregistret](releases-and-risks.md#riskregistret).
- **Värmekarta för risker**: en liten tabell 5 × 5 med de risker som inte är stängda, per sannolikhet (rader, 5 överst) och konsekvens (kolumner). Utöver färgen har varje nivå en egen kantlinje (prickad för medel, streckad för hög, heldragen med ränder för kritisk), förklarad under tabellen.
- **Åtgärder för öppna risker**: de öppna riskerna efter hur långt kraven som hanterar dem är testade: *Inga krav*, *Otestad*, *Delvis testad*, *Testad, inte alla godkända* och *Verifierad* (som i [riskrapporten](reports-and-export.md#standardrapporter)).
- **Ålder på öppna defekter**: öppna och påbörjade defekter efter ålder (under 7 dagar, 7–30, 30–90, över 90 dagar), uppdelade per allvarlighetsgrad, med medelåldern och den äldsta.
- **Spårbarhetsregler**: om alla aktiverade regler med allvarlighet *fel* är uppfyllda, hur många regler som är aktiverade, hur många som bryts (fel och varningar) och antalet överträdelser. **Spårbarhet →** öppnar matrisen med reglerna.
- **Kvalitetsmått** och **Konsistenskontroller** (de som hittar problem): samma som under [Analys](#kvalitetsmått).

De extra nyckeltalen och diagrammen räknas fram på servern i en enda begäran, som läses in samtidigt med resten av översikten.

Varje diagram har ett textalternativ med siffrorna för skärmläsare, och förklaringen namnger varje färg. Värmekartan är en tabell med rubrik, och varje ruta anger sin sannolikhet, konsekvens, nivå och antal risker.

### Anpassa översiktspaneler

Varje widget i en översiktspanel kan flyttas, ändras i storlek, ställas in och tas bort, och du kan lägga till widgetar och översiktspaneler.

- **Delade översiktspaneler** syns för alla som får läsa projektet. De inbyggda **Översikt**, **Trender** och **Kvalitet** är delade; projektets administratörer (behörigheten *project.update*) kan ändra dem, lägga till fler delade översiktspaneler och välja den som projektet öppnas med (**⋮ → Gör till projektets standard**, markerad med en stjärna).
- **Personliga översiktspaneler** är bara dina. Välj **⋮ → Ny översiktspanel** (om du vill med en kopia av den som visas som start), eller anpassa en delad översiktspanel som du inte får ändra: **Spara som min översiktspanel** sparar dina ändringar som en personlig kopia.
- **Hem** i sidomenyn är din personliga **hempanel** över alla projekt: från början **Mitt arbete**, dina diskussioner och en anteckning. En widget som gäller ett projekt anger projektet i sina inställningar. Du kan göra den till din startsida under **Profil → Startsida**.

Välj **Anpassa** för att redigera översiktspanelen som visas:

- **Lägg till widget** listar de widgettyper du får se, med en kort beskrivning. Bland dem: nyckeltal, kravtäckning, verifieringsstatus, antal per status, allvarlighet eller nivå (som stapeldiagram, liggande staplar, ringdiagram eller tabell), trend för testresultat, instabila tester, defekter, riskvärmekartan, releaseberedskap (go / no-go per slutkriterium och per variant), burn-up för release, burn-down för cykel, kvalitetsmått, konsistenskontroller, Mitt arbete, diskussioner, senaste aktivitet, luckor i spårbarheten (krav utan tester och tester utan krav), utforskande testning, täckning av MITRE ATT&CK, D3FEND och EMB3D, en **sparad fråga** – valfri fråga i [frågespråket](search-and-filters.md), som en tabell med de första objekten eller som ett antal – och en **anteckning** i Markdown.
- Dra en widget i dess **flytthandtag** (pilarna bredvid rubriken) och ändra storleken med handtaget i nedre högra hörnet. Widgetar som är i vägen flyttas nedåt; **Städa upp** tar bort luckorna.
- Med tangentbordet: ge flytt- eller storlekshandtaget fokus och använd **piltangenterna** (en cell per tryck; med **Skift** på flytthandtaget ändras storleken). **Home** och **End** går till första eller sista kolumnen (på storlekshandtaget: smalast och full bredd). **Enter** eller **blanksteg** tar upp widgeten och släpper den igen, **Escape** lägger tillbaka den där den var. Varje ändring läses upp för skärmläsare, till exempel ”Kravtäckning: kolumn 7, rad 1”.
- **Inställningar** (kugghjulet) anger rubriken, bredden (1–12 kolumner) och höjden (rader på ungefär 80 pixlar) och widgetens inställningar: release, cykel eller variant, frågan, diagramtypen, perioden och så vidare. **Ta bort** (×) tar bort widgeten från översiktspanelen.
- **Spara** sparar layouten; **Avbryt** kastar ändringarna.

En del widgetar har egna snabbval, till exempel perioden för en trend eller releasen för en burn-up; de ändrar bara visningen tills du lämnar sidan.

Menyn **⋮** har också **Byt namn**, **Exportera som JSON** och **Importera från JSON** (en översiktspanel som exporterats här eller från en annan server; widgetar av okända typer utelämnas), **Återställ till standard** (en inbyggd översiktspanel får tillbaka sin ursprungliga layout, en kopia layouten den kopierades från) och **Ta bort översiktspanelen**.

Widgetar visar bara det du får se: en widget vars data kräver en behörighet du saknar, eller som gäller ett projekt du inte kan öppna, säger det i stället. Widgetarna på skärmen läser in sina data tillsammans i en begäran, och widgetar längre ned läses in när du rullar dit; **Uppdatera** läser in alla igen.

## Beroendegrafer

**Beroendegraf** i projektets sidopanel ritar hur objekten bygger på varandra. Välj **Krav** eller **Testfall** överst:

- **Krav**: relationerna mellan krav, som *beror på*, *tillhandahåller*, *förfinar*, *härlett från*, *står i konflikt med*, *duplicerar* och *relaterar till* (se [Relationer](requirements.md#relationer));
- **Testfall**: [beroendena](tests.md#beroenden) mellan testfall (*beror på*) och de delade stegen (*anropar*).

Grafen läses från vänster till höger: förutsättningar, överordnade krav och källor står till vänster, det som bygger på dem till höger. Varje ruta visar nyckel, titel och täckning (krav) eller senaste utfall (testfall), med en färgad list i samma färger som i resten av appen. Varje relationstyp har en egen färg och linjestil, förklarad vid dess kryssruta.

- Kryssa ur en relationstyp för att dölja de linjerna; objekt som då saknar relationer försvinner om inte **Visa objekt utan relationer** är på.
- Skriv i **Markera objekt…** för att markera matchande objekt.
- Högerklicka på ett objekt (eller ge det fokus och tryck <kbd>Blanksteg</kbd>) för att markera dess grannar; **Visa alla** avslutar det.
- Välj ett objekt (eller tryck <kbd>Enter</kbd>) för att öppna det. Zooma med förstoringsknapparna; rulla för att flytta dig.
- **Rotera 90°** (knappen med den vridna pilen bredvid zoomen) ritar grafen uppifrån och ned i stället för från vänster till höger: förutsättningar och överordnade krav hamnar då överst. Valet sparas i adressen (`dir=vertical`) och i din webbläsare.
- **Skriv ut eller spara** skriver ut grafen på en egen sida (liggande när den är bred) eller sparar den som **SVG**-bild (skarp i alla storlekar, behåller länkarna till posterna), **PNG**-bild eller **Markdown** med ett Mermaid-diagram (ritas av GitHub, GitLab och många redigerare) och en tabell med posterna och deras relationer. De sparade filerna visar det som syns på skärmen: valda relationstyper, riktning och poster.
- **Lista** visar samma relationer som en tabell, ett objekt per rad, vilket är lättare att läsa med skärmläsare eller att skriva ut.

Cykler (till exempel två krav som beror på varandra) ritas, men en av linjerna visas utan pil.

## Kravanalys

**Analys** i projektets navigering analyserar alla krav i projektet på en gång (öppna den igen eller välj **Analysera igen** efter ändringar). Överst: **hälsan** (andelen krav utan problem eller varningar), antalet krav och testfall, andelen **täckta av testfall** (avvisade och inaktuella krav räknas inte) och den genomsnittliga **språkpoängen**.

- **Kravnivåer** visar hur många krav som har varje nivå, inklusive de som **saknar kravnivå**.
- **Nyckelord per betydelse** grupperar nyckelord som betyder samma sak (MUST, SHALL och REQUIRED; MAY och OPTIONAL …) och visar hur de används, så att du direkt ser projektets konvention.

**Anmärkningar** listar vad analysen har lagt märke till, problem först, var och en med en förklaring, **vad du kan göra** och de krav (eller testfall) det gäller; välj en nyckel för att öppna objektet. Filtrera på allvarlighet eller kategori:

| Kategori | Anmärkningar |
|---|---|
| Kravnivåer | nyckelord som inte skrivs med versaler som RFC 2119 och RFC 8174 kräver ("must", "Should"), och krav vars nyckelord *bara* står med gemener och därför saknar normativt nyckelord; WILL använt som kravnivå (det är inget nyckelord i RFC 2119); krav utan kravnivå; nivå i fältet men inget nyckelord i texten; fältet och texten säger olika; flera nivåer i ett krav; fördelningen: nästan allt obligatoriskt, inga obligatoriska krav, många valfria (MAY) eller många BÖR- eller beskrivande krav |
| Konsekvens | krav på mer än ett språk (till exempel mest engelska med några svenska), ett krav vars titel och meningar är på olika språk, och engelska nyckelord i ett krav skrivet på ett annat språk (MUST i en svensk mening); ett nyckelord som används i några få krav medan ett annat med samma betydelse dominerar, till exempel *SHALL används i 1 krav, medan MUST används i 95 %*; nyckelord med samma betydelse som används ungefär lika mycket |
| Täckning | obligatoriska och andra krav utan testfall; underkända eller ännu inte körda testfall; krav utan acceptanskriterier |
| Testfall | testfall som inte verifierar något krav; godkända krav utan testfall som är Klart |
| Kvalitet | tomma beskrivningar, dubblerade titlar, krav i konflikt, låg språkpoäng, många utkast, krav utan ägare |
| Språk | [språkanalysens](requirements.md#språkanalysen) anmärkningar summerade per regel: vaga ord, TBD, undanflykter … |

**Kravens språk** visar hur många krav som är skrivna på varje språk. Bara texter som är tillräckligt långa för att avgöra språket räknas; en titel på ett eller två ord räcker inte.

**Språk** väljer analysens språk (identifieras per krav som standard). Anmärkningar om testfall visas när du får läsa testfall.

### Konsistenskontroller

**Konsistenskontroller** under **Analys** utvärderar varje [inbyggd konsistenskontroll](#inbyggda-konsistenskontroller) på projektet, även de som inte lagts till som regler än (märkta **Standard**; de räknas inte för releaser förrän de lagts till). **Visa överträdelser** listar objekten med de berörda objekten och åtgärderna.

### Kvalitetsmått

**Kvalitetsmått** under **Analys** (och på översiktens flik **Kvalitet**) mäter projektet ur dess historik. Välj period (30, 90, 180 eller 365 dagar) och efter hur många dagar ett senaste resultat räknas som inaktuellt. Trenderna är per vecka, eller per månad för perioder över 120 dagar.

| Mått | Vad det mäter |
|---|---|
| **Kravens föränderlighet** | ändringarna av krav efter att de skapades (varje sparad version; flyttar räknas inte), per period, som ändringar per krav och period, och de mest ändrade kraven |
| **Defekttäthet** | defekter per aktivt krav, per mapp och per kategori; en defekt räknas för ett krav när den är länkad till kravet, till ett testfall som verifierar det eller till en körning av ett sådant testfall |
| **Läckta defekter** | defekter som hittats i produktion (defektens fält **Upptäckt i**, se [Defekter](defects.md)) eller rapporterats efter att en release de berör släppts; andelen av alla defekter, per upptäcktsfas och per release |
| **Testeffektivitet** | defekter hittade av test (länkade till ett testfall eller en av dess körningar) per kört testfall och per körtimme; körningar utan tidsåtgång räknas inte i timmarna |
| **Resultatens ålder** | hur gammalt varje testfalls senaste resultat är (0–7, 8–30, 31–90, 91–180 och fler dagar, aldrig körda) och hur många som är äldre än gränsen |
| **Granskningstid** | tiden från en granskningsbegäran till beslutet (median, medel och 90:e percentilen), och de väntande begärandena |
| **Tid till rättning av defekter** | tiden från att en defekt rapporteras tills den är löst, per allvarlighet (avvisade defekter räknas inte) |
| **Kravkvalitet** | andelen aktiva krav vars text har varningar från språkanalysen; trenden spelar upp varje kravs text ur dess historik |

Varje diagram har ett textalternativ och **Visa som tabell** visar siffrorna i en tabell. **Exportera som rapport** öppnar rapporterna: standardrapporten **Kvalitetsmått** innehåller samma siffror.

### Efterlevnad av lagar och standarder

Längst ned i **Analys** visar **Efterlevnad av lagar, standarder och bästa praxis** hur väl projektet uppfyller regelverken och standarderna i [katalogen](catalogue.md#uppfyll-lagar-standarder-och-bästa-praxis). Utan val visas alla källor som projektet använder; välj andra i listan (grupperad efter slag, med sökfält) för att se vad som saknas även för dem.

För varje källa:

- **Katalogkrav** är de krav i katalogen som hänvisar till den. Ett katalogkrav är **infört** när projektet har ett krav som lagts till från det och som inte är avvisat eller inaktuellt; annars **saknas** det. **Egna** krav är projektets krav som inte lagts till från de posterna men som nämner källan i fältet **Källa**, till exempel `GDPR Art. 32; ISO/IEC 27001:2022 A.8.5` (skrivet som under *Baserat på* i katalogen; stora och små bokstäver spelar ingen roll).
- **Uppfyllelse**: ett krav är *uppfyllt* när det har testfall och det senaste utfallet för vart och ett är godkänt, *underkänt* när något är underkänt, *blockerat* när något är blockerat, och *ej testat* annars. När projektet har flera kopior av samma katalogpost räknas det sämsta läget.
- **Efterlevnad** är andelen uppfyllda krav av alla katalogkrav och egna krav för källan; **Infört** är andelen katalogkrav som projektet har; **MUST-krav (SKA)** är efterlevnaden för de obligatoriska kraven. Stapeln visar uppfyllda, underkända, blockerade, ej testade och saknade krav.
- **Per kravnivå** delar upp siffrorna på MUST (även MUST NOT), SHOULD och MAY. **Per avsnitt** visar efterlevnaden per artikel, bilaga eller grupp av åtgärder, hämtat ur hänvisningarna (`Art. 32`, `A.8`, `Annex I Part I`, `SR 1` …; *(allmänt)* för hänvisningar till källan som helhet).
- **Saknade krav** listar luckorna, MUST först: välj ett för att öppna det i katalogen, eller **Lägg till det som saknas från katalogen** för att öppna katalogen med källan vald. **Infört men inte uppfyllt** listar projektets krav som fortfarande är underkända eller saknar godkända testfall; välj en nyckel för att öppna kravet.

Standardrapporten **Efterlevnad** (se [Rapporter och export](reports-and-export.md)) innehåller samma siffror, per källa och avsnitt, med luckorna. Analysen hjälper dig att planera och följa upp; den är ingen juridisk bedömning.
