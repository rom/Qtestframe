# Behörigheter

## Modellen

- Varje användare har **en roll per organisation** där hen är medlem; utan medlemskap ser en användare ingenting av en organisation.
- En roll är en namngiven mängd **behörigheter**. Sex roller är inbyggda: Administratör, Ansvarig, Testledare, Testare, Granskare och Läsare.
- **Superadministratör** är en flagga på användarkontot, som sätts med `qtestframe user create --super-admin` eller under **Administration → Användare**. En superadministratör har alla behörigheter i alla organisationer (och agerar som administratör i en organisation utan att vara medlem), och är ensam om att hantera organisationer och användare, säkerhetskopiera och återställa, och ändra systeminställningarna. Det gäller bara inloggade sessioner: API-nycklar har det aldrig.
- Servern kontrollerar behörigheterna vid varje begäran; webbklienten döljer bara det man inte får göra. Ett objekt i en annan organisation, eller i ett projekt man inte får se, svarar `404`, så att dess existens inte avslöjas.

## Standardmatrisen för behörigheter

Det här är de inbyggda rollernas behörigheter tills en organisation ändrar dem. Administratör har alltid alla behörigheter och kan inte ändras.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Behörighet | Läsare | Granskare | Testare | Testledare | Ansvarig | Administratör |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ |  |  | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  | ✔ | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

Vad behörigheterna tillåter:

| Behörighet | Tillåter |
|---|---|
| `project.read`, `requirement.read`, `test.read`, `run.read` | att läsa projekt och deras krav, tester och körningar |
| `project.create`, `project.update`, `project.delete` | att skapa projekt, ändra deras inställningar (även ärendesystem, webhooks, webbtestinställningar och delning), ta bort dem |
| `requirement.write`, `test.write` | att redigera krav och tester |
| `requirement.review`, `test.review` | granskningsbeslut: godkänna eller avslå krav, markera tester som klara |
| `run.write`, `run.execute` | att skapa och redigera körningar; registrera resultat, CI-importer, skicka defekter till ärendesystem |
| `script.execute` | att köra skript och webbtester **på servern** |
| `export` | exporter och rapporter |
| `audit.read` | organisationens händelselogg |
| `token.manage` | egna API-nycklar |
| `member.read`, `member.manage` | medlemslistan; lägga till, ändra och ta bort medlemmar, återställa deras lösenord och MFA |
| `role.manage` | rollmatrisen och egna roller |
| `settings.manage` | organisationens inställningar för testexekvering |
| `tenant.manage` | att byta namn på organisationen, dess MFA-policy och SCIM-provisionering |

Ge `script.execute` bara till personer som ni skulle låta köra kod på servern: skript och webbtester körs med serveranvändarens rättigheter, i sandlådan som beskrivs under [Säkerhet](security.md#sandlådan-för-skript-och-webbtester).

## Egna och ändrade roller

Den som har `role.manage` (Administratör som standard) ändrar behörigheterna för alla inbyggda roller utom Administratör och skapar **egna roller** under **Administration → Rollmatris** ([användarhandboken](../../user-guide/sv/administration.md#ändra-roller-och-skapa-egna)). Reglerna som gör det säkert:

- Ingen kan ge en behörighet som hen inte själv har – inte till en roll, inte till en API-nyckel, inte via projektdelning.
- Administratör behåller alltid alla behörigheter, och en organisation har alltid minst en administratör (den sista kan inte nedgraderas eller tas bort, inte heller via SCIM).
- En egen roll har ett tekniskt namn på 2–32 tecken (`a-z`, `0-9`, `_`, börjar med en bokstav), som är vad `member add --role`, `role_mapping` och SCIM-gruppmappningar använder. Den kan tas bort när ingen medlem eller nyckel använder den.
- En inbyggd roll vars behörigheter skiljer sig från standarden markeras *ändrad* och kan återställas. Ändringar granskas.

En organisations gällande matris finns på `GET /api/v1/roles`.

## Begränsade och delade projekt

Ett projekt är antingen **organisationsgemensamt** (standard: alla medlemmar ser det med sin organisationsroll) eller **begränsat** (bara dess projektmedlemmar ser det). Projektmedlemmar är användare eller SCIM-grupper med en **projektroll**, som ersätter organisationsrollen i projektet, uppåt eller nedåt. Användare med `project.update` i projektet hanterar detta under **Dela** ([användarhandboken](../../user-guide/sv/administration.md#delning-av-projekt)).

| Vem | Behörigheter i ett projekt |
|---|---|
| organisationsadministratörer och superadministratörer | sina egna, alltid – de ser alla projekt |
| en projektmedlem (direkt eller via en grupp; det direkta medlemskapet vinner, grupper läggs ihop) | projektrollens projektbehörigheter |
| övriga medlemmar, organisationsgemensamt projekt | organisationsrollen |
| övriga medlemmar, begränsat projekt | inga: projektet svarar `404` |

Bara **projektbehörigheter** kommer från projektrollen (`project.read`, `project.update`, `project.delete`, `requirement.*`, `test.*`, `run.*`, `script.execute`, `export`). Behörigheter på organisationsnivå – medlemmar, roller, API-nycklar, händelseloggen, att skapa projekt – kommer alltid från organisationsrollen, och en API-nyckel begränsas av sin egen roll även i ett projekt.

Vad en begränsning **inte** döljer, så planera för det:

- organisationens **händelselogg** (`audit.read`) registrerar åtgärder i alla projekt;
- e-postsammanfattningar kan innehålla aviseringar som skapades medan mottagaren fortfarande hade åtkomst;
- **säkerhetskopior** innehåller alla projekt; projektarkiv (*Spara projekt*) innehåller hela projektet.

## Var administrationen sker

| Sida i webbklienten | Vem |
|---|---|
| Medlemmar, Rollmatris | `member.read` (alla inbyggda roller) för att se dem; ändringar kräver `member.manage` / `role.manage` |
| API-nycklar | `token.manage` |
| Inställningar för testexekvering | `settings.manage` |
| Händelselogg | `audit.read` (Testledare och uppåt som standard) |
| Provisionering (SCIM), Tvåstegsverifiering (organisationens policy) | `tenant.manage` |
| Organisationer, Användare, Säkerhetskopiering, Systeminställningar, serverns MFA-policy | superadministratörer |
