# Design av Web shop (SHOP)

Du är en erfaren mjukvarudesigner och seniorutvecklare som omsätter krav och en arkitektur i en detaljerad design som kan implementeras direkt.

Ta fram lösningens design för kraven nedan (3), i linje med lösningsbeskrivningen.

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

- Utforma moduler och komponenter: ansvar, publika gränssnitt och beroenden, i linje med arkitekturstilen och kodförrådets struktur i lösningsbeskrivningen.
- Härled datamodellen från kraven: entiteter, attribut, nycklar, samband och villkor (en ER-modell).
- Utforma API:et: resurser eller operationer, format för anrop och svar, status- och felkoder, sidindelning, versionshantering och behörighet för varje operation.
- Beskriv sekvensflödena för de viktigaste användningsfallen (MUST-kraven först), steg för steg genom komponenterna.
- Definiera felhanteringen: validering, förväntade fel, omförsök och idempotens, tidsgränser och vad användaren eller anroparen ser.
- Utforma säkerheten: autentisering, behörighet, validering av indata, skydd av data i vila och under överföring, loggning för spårbarhet, samt åtgärderna för de kopplade riskerna och de mappade CWE-, ATT&CK-, D3FEND- och EMB3D-posterna.
- Spåra varje designelement (komponent, beslut, entitet, endpoint, flöde, uppgift) till de kravnycklar det tjänar, och lista de krav som inget element täcker.
- Hitta inte på krav eller beteenden: lyft fram oklarheter, luckor och konflikter mellan krav (även relationerna "står i konflikt med") som öppna frågor, med ett förslag till lösning.
- Lista varje antagande du gör, numrerat (A1, A2 …), och hänvisa till det där det används.
- Använd teknikerna, versionerna och konventionerna i lösningsbeskrivningen; föreslå ett alternativ bara med motivering.
- MUST-krav och deras acceptanskriterier är inte förhandlingsbara; SHOULD-krav får skjutas upp med motivering; MAY-krav är valfria.
- Skriv aldrig lösenord, nycklar eller andra hemligheter i svaret; hänvisa till var de förvaras.
- Skriv svaret i Markdown, med de numrerade avsnitten i svarsformatet som rubriker.
- Rita diagram som Mermaid-kodblock (```mermaid): flödes- eller komponentdiagram, sekvensdiagram för flödena, erDiagram för datamodellen.
- Ge API:et som ett OpenAPI 3.1-skelett i YAML (sökvägar, operationer, scheman, felsvar, säkerhetsscheman) med kravnycklarna i x-requirements.
- Ge datamodellen som SQL DDL (CREATE TABLE med nycklar, villkor och index) för databasmotorn i lösningsbeskrivningen, med kravnycklarna i kommentarer.

### Ytterligare instruktioner

Prefer boring technology.

## Svarets form

1. Moduler och komponenter
2. Datamodell
3. API-design
4. Sekvensflöden för de viktigaste användningsfallen
5. Felhantering
6. Säkerhetsdesign
7. Antaganden
8. Öppna frågor, oklarheter och konflikter
9. Spårbarhet: en tabell med en rad per kravnyckel, enligt nedan

| Krav | Täcks av (element, beslut, uppgifter) | Verifierande tester | Status |
|---|---|---|---|
| KEY-1 Titel | t.ex. komponent, ADR-1, endpoint, uppgift T3 | testnycklar eller "att skriva" | täckt / delvis / inte täckt / fråga |
