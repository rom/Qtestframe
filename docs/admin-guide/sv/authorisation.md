# Behörigheter

## Modellen

- Varje medlem har **en eller flera roller per organisation**; utan medlemskap ser en användare ingenting av en organisation. Den som både skriver krav och testar har till exempel *Ämnesexpert* och *Testare*.
- En roll är en namngiven mängd **behörigheter**; en medlem med flera roller har **unionen** av deras behörigheter (roller lägger bara till rättigheter). Sju roller är inbyggda: Administratör, Ansvarig, Testledare, Testare, Ämnesexpert, Granskare och Läsare.
- Den som har Administratör bland sina roller är organisationsadministratör. Äldre API-klienter ser en `role`, den *primära* rollen: medlemmens starkaste inbyggda roll (alltid Administratör när medlemmen har den).
- **Superadministratör** är en flagga på användarkontot, som sätts med `qtestframe user create --super-admin` eller under **Administration → Användare**. En superadministratör har alla behörigheter i alla organisationer (och agerar som administratör i en organisation utan att vara medlem), och är ensam om att hantera organisationer och användare, säkerhetskopiera och återställa, och ändra systeminställningarna. Det gäller bara inloggade sessioner: API-nycklar har det aldrig.
- Servern kontrollerar behörigheterna vid varje begäran; webbklienten döljer bara det man inte får göra. Ett objekt i en annan organisation, eller i ett projekt man inte får se, svarar `404`, så att dess existens inte avslöjas.

## Standardmatrisen för behörigheter

Det här är de inbyggda rollernas behörigheter tills en organisation ändrar dem. Administratör har alltid alla behörigheter och kan inte ändras.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Behörighet | Läsare | Granskare | Ämnesexpert | Testare | Testledare | Ansvarig | Administratör |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  | ✔ |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ | ✔ |  |  | ✔ | ✔ |
| `waiver.approve` |  |  |  |  |  | ✔ | ✔ |
| `change.create` |  |  | ✔ |  | ✔ | ✔ | ✔ |
| `change.approve` |  |  |  |  | ✔ | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `cycle.plan` |  |  |  |  | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  |  | ✔ | ✔ | ✔ |
| `comment.create` |  | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

### Ämnesexperten

**Ämnesexpert** (`sme`, subject matter expert) är en domänexpert som deltar i att skriva kraven inom sitt område – en kliniker, en säkerhetsingenjör, en jurist. Rollen:

- skapar och redigerar krav (`requirement.write`): text, acceptanskriterier, motivering, källor, relationer, spårnings- och objektlänkar och bilagor;
- deltar i kravgranskningar (`requirement.review`) och diskussioner (`comment.create`);
- läser tester, körningar, defekter och risker som underlag, och exporterar och tar fram rapporter (`export`).

Den skriver och granskar medvetet **inte** tester, skapar eller kör inte testkörningar, planerar inte cykler, godkänner inte undantag eller restrisker, ändrar inte projektinställningar, delning eller medlemmar och skapar inga API-nycklar. Belägg för testkörningar registreras av testare; en ämnesexpert bifogar underlag till själva kraven. Ge en andra roll till den som gör mer, t.ex. *Ämnesexpert* + *Testare*.

Eftersom rollen både skriver och granskar krav kan en ämnesexpert godkänna sina egna krav. Där åtskild behörighet är viktig tar du bort `requirement.review` från rollen i matrisen (eller ger den bara till separata granskare).

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
- Administratör behåller alltid alla behörigheter, och en organisation har alltid minst en medlem med rollen Administratör: den sista kan inte förlora rollen eller tas bort – inte i medlemslistan, inte i grupp, inte via SCIM. Administratör kan kombineras med andra roller.
- En egen roll har ett tekniskt namn på 2–32 tecken (`a-z`, `0-9`, `_`, börjar med en bokstav), som är vad `member add --role` (flera roller kommaseparerade: `--role sme,tester`), `role_mapping` och SCIM-gruppmappningar använder. Den kan tas bort när ingen medlem, projektmedlem eller nyckel har den, inte heller bland andra roller.
- En inbyggd roll vars behörigheter skiljer sig från standarden markeras *ändrad* och kan återställas. Ändringar loggas, och en ändring av en medlems roller loggas med de roller som lagts till och tagits bort (`+sme −viewer`).
- API-nycklar har också roller (som standard alla ägarens roller) och får aldrig mer än ägarens nuvarande roller tillåter.
- Med single sign-on (`role_mapping`) och SCIM får en användare i flera mappade grupper rollerna för alla.

En organisations gällande matris finns på `GET /api/v1/roles`.

## Begränsade och delade projekt

Ett projekt är antingen **organisationsgemensamt** (standard: alla medlemmar ser det med sin organisationsroll) eller **begränsat** (bara dess projektmedlemmar ser det). Projektmedlemmar är användare eller SCIM-grupper med en eller flera **projektroller**, som ersätter organisationsrollerna i projektet, uppåt eller nedåt. Användare med `project.update` i projektet hanterar detta under **Dela** ([användarhandboken](../../user-guide/sv/administration.md#delning-av-projekt)).

| Vem | Behörigheter i ett projekt |
|---|---|
| organisationsadministratörer och superadministratörer | sina egna, alltid – de ser alla projekt |
| en projektmedlem (direkt eller via en grupp; det direkta medlemskapet vinner, grupper läggs ihop) | projektrollernas projektbehörigheter (sammanlagda) |
| övriga medlemmar, organisationsgemensamt projekt | organisationsrollerna |
| övriga medlemmar, begränsat projekt | inga: projektet svarar `404` |

Bara **projektbehörigheter** kommer från projektrollerna (`project.read`, `project.update`, `project.delete`, `requirement.*`, `waiver.approve`, `test.*`, `run.*`, `cycle.plan`, `script.execute`, `export`, `comment.create`). Behörigheter på organisationsnivå – medlemmar, roller, API-nycklar, händelseloggen, att skapa projekt – kommer alltid från organisationsrollerna, och en API-nyckel begränsas av sina egna roller även i ett projekt.

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
