# Arkitektur för Web shop (SHOP)

Du är en erfaren mjukvaruarkitekt. Du utformar lösningar som uppfyller kraven inom ramarna i lösningsbeskrivningen, och du gör avvägningarna tydliga.

Föreslå lösningens arkitektur för kraven nedan (3). Skriv inte implementationen.

## Lösningsbeskrivning

- **Arkitekturstil:** modulär monolit
- **Implementationsspråk och versioner:** Go 1.26, TypeScript 5
- **Ramverk och bibliotek:** net/http, pgx, Vue 3
- **Databaser (beskrivna; inga inloggningsuppgifter):**
  PostgreSQL 16, schema owned by the app,
  migrations embedded in the binary
- **Säkerhets- och regelefterlevnadskrav:** OIDC sign-in, RBAC
- **Utanför omfattningen:** Native mobile apps

### Teknisk beskrivning

Monorepo.

## Sammanfattning av projektet

- Projekt: Web shop (SHOP)
- Beskrivning: An online shop
- Omfattning: hela projektet — 3 krav (av 5 i projektet)
- Krav per kravnivå: MUST 1, SHOULD 1, MAY 1
- Mappar: Checkout (2), Accounts (1)
- Releaser: 1.0 (planned)
- Varianter: EU
- Kopplade risker: 1 · verifierande tester: 1 (av 4 tester i projektet)

## Krav

Nyckelorden MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, RECOMMENDED, MAY och OPTIONAL tolkas enligt RFC 2119 och RFC 8174 när, och bara när, de skrivs med versaler (på svenska: SKA, SKA INTE, BÖR, BÖR INTE, KAN). Varje kravs kravnivå anges på dess första rad.

### SHOP-REQ-1 Login

_MUST · Typ: säkerhet · Prioritet: hög · Status: godkänt · Mapp: Accounts_

> The user MUST be able to log in.

- **Motivering:** Protect accounts
- **Acceptanskriterier:**
  - Valid login succeeds (test) — gränsvärde: < 2 s
- **Relationer:** står i konflikt med SHOP-REQ-3 Guest checkout
- **Risker:** SHOP-RISK-1 Credential stuffing (nivå hög; åtgärd: Rate limiting and MFA; ATT&CK T1110.004)
- **Verifieras av:** SHOP-TC-1 Valid login
- **Standarder och ramverk:** CWE-307; OWASP ASVS V2.2; D3FEND D3-MFA

### SHOP-REQ-2 Pay by card

_SHOULD · Typ: funktionellt · Prioritet: medel · Status: utkast · Mapp: Checkout/Payment_

> The shop SHOULD accept cards.

- **Relationer:** beror på SHOP-REQ-1 Login

### SHOP-REQ-3 Guest checkout

_MAY · Mapp: Checkout_

- **Acceptanskriterier:**
  - No account needed
- **Standarder och ramverk:** EMB3D MID-001

## Instruktioner

- Beskriv systemets omgivning: användare och roller, externa system (integrationerna i lösningsbeskrivningen och de som kraven nämner) och vad som passerar systemgränsen.
- Beskriv containrarna (driftsättningsbara eller körbara enheter, datalager, meddelandekanaler) och de viktigaste komponenterna i var och en, med ansvar och teknikerna i lösningsbeskrivningen.
- Dokumentera de viktigaste arkitekturbesluten som ADR-kandidater: sammanhang, beslut, övervägda alternativ, konsekvenser och de kravnycklar som driver varje beslut.
- Skriv kvalitetsattributscenarier (källa, stimulus, miljö, respons, responsmått) för de icke-funktionella kraven och målen i lösningsbeskrivningen, vart och ett kopplat till kravnycklar.
- Lista arkitekturrisker och teknisk skuld, med de kopplade projektriskerna där de gäller, och hur arkitekturen hanterar dem.
- Håll dig till arkitekturstilen, målmiljöerna och det som ligger utanför omfattningen i lösningsbeskrivningen; om ett krav inte passar, säg det i stället för att ändra ramarna.
- Spåra varje designelement (komponent, beslut, entitet, endpoint, flöde, uppgift) till de kravnycklar det tjänar, och lista de krav som inget element täcker.
- Hitta inte på krav eller beteenden: lyft fram oklarheter, luckor och konflikter mellan krav (även relationerna "står i konflikt med") som öppna frågor, med ett förslag till lösning.
- Lista varje antagande du gör, numrerat (A1, A2 …), och hänvisa till det där det används.
- Använd teknikerna, versionerna och konventionerna i lösningsbeskrivningen; föreslå ett alternativ bara med motivering.
- MUST-krav och deras acceptanskriterier är inte förhandlingsbara; SHOULD-krav får skjutas upp med motivering; MAY-krav är valfria.
- Skriv aldrig lösenord, nycklar eller andra hemligheter i svaret; hänvisa till var de förvaras.
- Skriv svaret i Markdown, med de numrerade avsnitten i svarsformatet som rubriker.
- Rita diagram som Mermaid-kodblock (```mermaid): flödes- eller komponentdiagram, sekvensdiagram för flödena, erDiagram för datamodellen.
- Visa systemets omgivning och containrarna enligt C4-modellen (Mermaid-diagram C4Context och C4Container).

### Ytterligare instruktioner

Prefer boring technology.

## Svarets form

1. Systemets omgivning
2. Containrar och komponenter
3. Arkitekturbeslut (ADR-kandidater)
4. Kvalitetsattributscenarier
5. Risker och teknisk skuld
6. Antaganden
7. Öppna frågor, oklarheter och konflikter
8. Spårbarhet: en tabell med en rad per kravnyckel, enligt nedan

| Krav | Täcks av (element, beslut, uppgifter) | Verifierande tester | Status |
|---|---|---|---|
| KEY-1 Titel | t.ex. komponent, ADR-1, endpoint, uppgift T3 | testnycklar eller "att skriva" | täckt / delvis / inte täckt / fråga |
