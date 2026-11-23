# Implementationsplan för Web shop (SHOP)

Du är en erfaren teknisk projektledare som planerar implementationsarbetet så att varje uppgift kan spåras till kraven och "klar" betyder verifierad.

Dela upp implementationen av kraven nedan (3) i milstolpar, epics och uppgifter, i en genomförbar ordning.

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

## Beroendeordning

Kraven med beroenden mellan sig, förutsättningar först (från relationerna "beror på"):

SHOP-REQ-1 → SHOP-REQ-2

## Instruktioner

- Dela upp arbetet i milstolpar, epics och uppgifter; varje uppgift anger de kravnycklar den implementerar eller stödjer och är så liten att en person klarar den på några dagar.
- Ordna arbetet efter beroendena mellan kraven ("beror på", "förfinar"; se beroendeordningen), därefter efter kravnivå och prioritet: MUST före SHOULD före MAY.
- Ge varje epic en definition av klar: dess acceptanskriterier uppfyllda, dess verifierande tester (med nyckel och titel) godkända, samt de granskningar, den dokumentation och de driftsättningssteg som lösningsbeskrivningen kräver.
- Ange vilka tester som ska skrivas där ett krav ännu saknar verifierande test.
- Visa vad som kan göras parallellt och vad som blockerar annat arbete; peka ut riskerna för planen.
- Spåra varje designelement (komponent, beslut, entitet, endpoint, flöde, uppgift) till de kravnycklar det tjänar, och lista de krav som inget element täcker.
- Hitta inte på krav eller beteenden: lyft fram oklarheter, luckor och konflikter mellan krav (även relationerna "står i konflikt med") som öppna frågor, med ett förslag till lösning.
- Lista varje antagande du gör, numrerat (A1, A2 …), och hänvisa till det där det används.
- Använd teknikerna, versionerna och konventionerna i lösningsbeskrivningen; föreslå ett alternativ bara med motivering.
- MUST-krav och deras acceptanskriterier är inte förhandlingsbara; SHOULD-krav får skjutas upp med motivering; MAY-krav är valfria.
- Skriv aldrig lösenord, nycklar eller andra hemligheter i svaret; hänvisa till var de förvaras.
- Skriv svaret i Markdown, med de numrerade avsnitten i svarsformatet som rubriker.

### Ytterligare instruktioner

Prefer boring technology.

## Svarets form

1. Milstolpar
2. Epics och uppgifter (tabell: id, uppgift, kravnycklar, beror på, uppskattning)
3. Ordning och beroenden
4. Definition av klar per epic, med de verifierande testerna
5. Risker för planen
6. Antaganden
7. Öppna frågor, oklarheter och konflikter
8. Spårbarhet: en tabell med en rad per kravnyckel, enligt nedan

| Krav | Täcks av (element, beslut, uppgifter) | Verifierande tester | Status |
|---|---|---|---|
| KEY-1 Titel | t.ex. komponent, ADR-1, endpoint, uppgift T3 | testnycklar eller "att skriva" | täckt / delvis / inte täckt / fråga |
