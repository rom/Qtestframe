# Utforskande test

Utforskande test är testdesign och testutförande på samma gång: testaren lär sig produkten medan hen testar den och bestämmer vad som ska prövas härnäst utifrån det hen just såg. Qtestframe stöder **sessionsbaserad testledning** (SBTM): arbetet görs i **sessioner** med en bestämd längd, var och en med ett **uppdrag**, och varje session avslutas med en **utvärdering**.

| Åtgärd | Roll som krävs |
|---|---|
| Se sessioner, anteckningar, mått och historik | Läsare (`run.read`) |
| Planera, ändra och ta bort sessioner; öppna en utvärdering igen | Testare (`run.write`) |
| Genomföra en session: tidtagningen, anteckningar, filer, defekter från anteckningar, utvärderingen | Testare (`run.execute`) |
| Exportera sessionsrapporten | Läsare (`export`) |
| Låta sessioner räknas som verifieringsbevis | Förvaltare (`project.update`) |

Bara sessionens **testare** genomför den, antecknar och utvärderar den. Någon annan behöver dessutom `run.write`: en testledare som stoppar en bortglömd session, eller en granskare som utvärderar den. En session utan testare tillhör den som startar den.

## Planera en session

Öppna **Utforskande** i projektets navigering och välj **Ny session**.

- **Uppdrag**: ett kort namn på uppdraget, till exempel *Utforska kassan med utgångna kort*.
- **Mål**: vad som ska utforskas, med vilka resurser, för att upptäcka vad (*Utforska … med … för att upptäcka …*).
- **Testare**: vem som gör sessionen (som standard du).
- **Tidsram**: den planerade längden, oftast 30, 60 eller 90 minuter; alla längder från 5 till 480 minuter godtas.
- **Områden**: de krav- och testmappar som sessionen utforskar. Måtten räknar sessioner och buggar per område.
- **Krav** och **Testfall** i omfattningen: det sessionen handlar om. En defekt som skapas i sessionen länkas till dessa krav, och en utvärderad session kan vara bevis för dem (se nedan).

En ny session är **planerad**. Listan visar alla sessioner med uppdrag, testare, status, utfall, sessionstid mot tidsramen, antal anteckningar och buggar. Filtrera på text, status och område, eller välj **Mina sessioner**.

## Genomföra en session

Öppna en session för att se **sessionsvyn**: tidtagningen överst, anteckningsfältet, anteckningarna hittills och vid sidan uppdraget med dess omfattning, skärmbilder och filer samt historiken.

### Tidtagningen

**Starta** startar tidtagningen, **Pausa** pausar den (en paus räknas inte) och **Fortsätt** fortsätter. **Stoppa** avslutar sessionen för gott. Tiden hålls av servern, inte av webbläsaren: du kan ladda om sidan, stänga webbläsaren eller fortsätta på en annan dator, och sessionstiden stämmer ändå. Stapeln under klockan visar hur mycket av tidsramen som har använts; efter tidsramen blir den orange, men tidtagningen fortsätter.

| Status | Betydelse |
|---|---|
| Planerad | Inte startad än |
| Pågår | Tidtagningen går |
| Pausad | Startad, tidtagningen är pausad |
| Avslutad | Stoppad, väntar på utvärdering |
| Utvärderad | Utvärderad; sessionen är stängd |

### Anteckningar

Anteckningar görs från sessionens start till dess utvärdering. Varje anteckning har en **typ** och stämplas med den sessionstid då den gjordes:

- **Observation**: vad du såg, även förberedelser och det som fungerar.
- **Fråga**: något att fråga produktägaren eller utvecklarna.
- **Problem**: något som är fel eller oklart men kanske inte en bugg.
- **Idé**: en testidé för senare, eller en förbättring.
- **Bugg**: ett fel i produkten.

Skriv anteckningen och välj **Lägg till anteckning**, eller tryck **Ctrl+Enter**. Välj **Sessionens filer** för att hänvisa till skärmbilder som du laddat upp under **Skärmbilder och filer** (filerna lagras som alla bilagor och syns i projektets [dokument](documents.md)).

### Kortkommandon

Sessionsvyn är gjord för att användas utan mus. Utanför anteckningsfältet:

| Tangent | Åtgärd |
|---|---|
| O, Q, I, D, B | Välj typ av anteckning (Observation, fråga/Question, problem/Issue, iDé, Bugg) och gå till anteckningsfältet |
| N | Gå till anteckningsfältet och behåll typen |
| T | Starta eller pausa tidtagningen |
| ? | Visa eller dölj kortkommandona |

Tangenterna är desamma på alla språk. I anteckningsfältet lägger **Ctrl+Enter** (⌘+Enter på Mac) till anteckningen och **Esc** lämnar fältet så att bokstäverna fungerar som kortkommandon igen. Skärmläsare läser upp vald typ och varje tillagd anteckning.

### Skapa en defekt från en anteckning

Välj **Skapa defekt** vid en anteckning. Defekten är förifylld: titeln är anteckningens första rad, beskrivningen anteckningen med sessionens nyckel, sessionstiden och uppdraget, och *hittad i* är *test*. Ändra det som behövs och bekräfta. Defekten länkas till sessionen och till kraven i dess omfattning, anteckningen visar defektens nyckel och blir en bugganteckning. Se [Defekter](defects.md).

## Utvärdering

När sessionen stoppas öppnas utvärderingen. I SBTM är utvärderingen ett kort samtal mellan testaren och en testledare; registrera resultatet här:

- **Sammanfattning**: vad som täcktes, vad som hittades, vad som återstår.
- **Utfall**: *Godkänd* (omfattningen fungerar som krävs), *Underkänd* (problem hittades som underkänner den) eller *Ofullständig* (den kunde inte bedömas, till exempel för att den var blockerad).
- **Täckning av uppdraget**: din uppskattning i procent av hur mycket av uppdraget sessionen täckte.
- **Tidsfördelning**: andelen av sessionen som gick åt till **förberedelse** (miljö och testdata), **testning** (testdesign och utförande) och **felutredning** (utreda och rapportera fel). De tre blir tillsammans 100 %. Formuläret föreslår en fördelning utifrån anteckningarna.
- **Granskarens kommentar**: det en granskare lägger till.

Testaren utvärderar sin egen session. Någon annan med `run.write` kan utvärdera den som **granskare**: sessionen visar då *Granskad av*. En utvärdering kan ändras senare; **Öppna igen** (med `run.write`) tar bort den och gör sessionen avslutad igen. En utvärderad session och dess anteckningar kan inte ändras.

## Sessioner som verifieringsbevis

Som standard är utforskande sessioner inte verifieringsbevis. Under **Inställningar → Projekt → Verifiering** kan en förvaltare slå på **Utforskande sessioner är verifieringsbevis**. Då räknas en utvärderad session som bevis av typen *utforskande* för kraven i dess omfattning, för kravet som helhet:

- *Godkänd* godkänner, *Underkänd* underkänner, *Ofullständig* räknas inte;
- för varje krav räknas bara den senast utvärderade sessionen; äldre visas som *ersatta*;
- en session verifierar aldrig ett [utvärderingskriterium](requirements.md) ensam, så ensam verifierar den bara ett krav utan kriterier. Med kriterier bidrar den till bevisen: en underkänd session underkänner kravet.

Kravens status följer verifieringen när projektet låter statusen följa bevisen. Se [Spårbarhet och täckning](traceability-and-coverage.md).

## Mått

Ovanför listan visar **Mått** antal sessioner, sessionstimmar, buggar (bugganteckningar och anteckningar som en defekt skapats från) och **buggar per sessionstimme**, utvärderingarnas genomsnittliga tidsfördelning och täckning, samt detsamma per **område** och per **testare**.

## Rapport, historik och händelselogg

**Markdown** och **PDF** överst i sessionsvyn exporterar **sessionsrapporten**: uppdraget och omfattningen, sessionstiden, utvärderingen, alla anteckningar i den ordning de gjordes och de defekter som skapats. Samma rapport, och sammanställningen av alla sessioner (*Utforskande test*), är standardrapporter på sidan **Rapporter**, där de också kan sparas i dokumentarkivet eller skickas med e-post ([Rapporter och export](reports-and-export.md)).

**Historik** listar sessionens versioner: när den planerades, ändrades, stoppades, utvärderades och öppnades igen, med de ändrade fälten. Varje åtgärd (planering, tidtagning, anteckningar, defekter, utvärdering) registreras i organisationens händelselogg. Sessioner, deras omfattning och anteckningar ingår i projektarkivet (**Spara projekt**) och i säkerhetskopian.
