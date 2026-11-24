# Integritet och gallring

Det här kapitlet är för superadministratörer och dataskyddsombud. Det förklarar hur länge Qtestframe sparar uppgifter, hur gallringen raderar äldre poster, vad ett **juridiskt bevarande** skyddar, hur du besvarar en **begäran från en registrerad** (export och radering enligt GDPR) och var regelverkens krav på arkivering går före radering.

## Gallringspolicy

Superadministratörer anger gallringen under **Administration → Systeminställningar → Gallring och säkerhetskopior**. Varje tid anges i **dagar; 0 sparar uppgifterna för alltid**, vilket är standard. Ingenting raderas förrän du slår på den nattliga gallringen.

| Inställning | Vad som raderas när det är äldre än tiden |
|---|---|
| `retention.runs_days` | Testkörningar med exekveringar, stegresultat, iterationer, kommentarer, defekt- och objektlänkar och bilagor. Pågående körningar behålls. |
| `retention.runs_keep_latest` | Ingen tid: körningar som innehåller något av de *N* senaste resultaten för ett test behålls alltid, hur gamla de än är (0 = inget undantag). |
| `retention.logs_days` | Loggar och uppladdade artefakter: inloggningsloggen, avslutade jobb i testkörningen, CI-statusposter som skickats eller gett upp, avslutade synkloggar mot ärendesystem och bevis från webbtester (skärmbilder, spår, videor) för gamla exekveringar – själva resultaten finns kvar. |
| `retention.notifications_days` | Aviseringar i appen, lästa eller inte. |
| `retention.sessions_days` | Inloggningssessioner som inte använts så länge, och API-tokens som gick ut för så länge sedan. |
| `retention.chat_days` | Historiken för chattmeddelanden (skickade eller uppgivna meddelanden med sin text) och den personliga historiken för inmatningsförslag. |
| `retention.exploratory_days` | Anteckningarna från avslutade eller genomgångna utforskande sessioner, räknat från sessionens senaste ändring. Sessionen och dess genomgång finns kvar. |
| `retention.enabled`, `retention.hour` | Den nattliga gallringen och den timme (UTC) den körs. |

**Egen gallring per organisation.** Under **Administration → Organisationer → (organisation) → Regler → Gallring** kan en superadministratör ge en organisation egna tider (till exempel ett kundavtal som kräver 10 år för körningar). Ett tomt fält använder systeminställningen. Inloggningsloggen och inloggningssessionerna hör inte till någon organisation och använder alltid systeminställningen.

**Granskningsloggen gallras aldrig.** Den är en hashkedja ([Manipuleringssäker händelselogg](security.md#manipuleringssäker-händelselogg)): verifieringen går igenom varje post från den första, och signerade kontrollpunkter pekar ut poster med löpnummer, så att radera gamla poster skulle bryta kedjan. Kedjan har ingen förseglad startpunkt som skulle göra det säkert att kapa den, så Qtestframe behåller hela loggen. För att begränsa vad den levande databasen innehåller arkiverar du loggen med **Exportera med hashar** till lagring som bara kan skrivas en gång; arkivet kan verifieras fristående (`qtestframe audit verify --file`). Elektroniska signaturer och godkännanden gallras inte heller.

### Förhandsvisa innan du slår på den

Under inställningarna visar **Förhandsvisning och gallring** vad gallringen skulle radera *nu*: per organisation och kategori antalet poster som är äldre än tiden, och hur många äldre poster som är **bevarade** – behålls av juridiskt bevarande eller en elektronisk signatur. Förhandsvisningen ändrar ingenting. Kontrollera den, justera tiderna och slå först därefter på `retention.enabled`.

**Gallra nu** kör gallringen direkt (även när den nattliga gallringen är av) när du har skrivit `PURGE`. API: `GET /api/v1/admin/retention` (förhandsvisning), `POST /api/v1/admin/retention/run` med `{"confirm": "PURGE"}`.

Varje gallring registreras i granskningsloggen som `retention.purge`, i varje organisation där något raderades och i systemets logg, med antalen per kategori och de bevarade posterna. Raderade poster kan bara återställas från en [säkerhetskopia](backup-and-migration.md); ta en före den första gallringen.

## Juridiskt bevarande

Ett juridiskt bevarande skyddar ett helt **projekt** eller en enskild **release** när uppgifter måste sparas – en tvist, en begäran från en myndighet, en revision. Så länge bevarandet gäller:

- raderar gallringen ingenting av det (körningar i projektet eller releasen, deras resultat och bevis, loggar, utforskande anteckningar och chatthistorik);
- **kan projektet**, organisationen som har det och körningarna i en bevarad release **inte tas bort** (409).

Organisationsadministratörer (`tenant.manage`) och superadministratörer inför och häver bevarande i projektets **Inställningar → Juridiskt bevarande**, med ett skäl som visas med bevarandet. Att införa och häva registreras i granskningsloggen (`legal_hold.set`, `legal_hold.lift`). Gallringssidan listar alla projekt och releaser med bevarande i alla organisationer. API: `GET`/`PUT /api/v1/projects/{pid}/legal-hold` och `PUT /api/v1/projects/{pid}/releases/{rid}/legal-hold` med `{"on": true, "reason": "…"}`.

## Begäran från registrerade

### Tillgång och dataportabilitet (export)

Under **Administration → Användare** laddar **Exportera personuppgifter** ner en ZIP-fil med JSON-filer med allt servern har om personen: profil och inställningar, medlemskap och roller, projektroller, kommentarer personen skrivit, kommentarer som nämner personen, sessioner, API-tokens (namn och datum, aldrig hemligheten), inloggningsloggen, betrodda enheter, aviseringar, elektroniska signaturer, inmatningshistorik och granskningsposter av eller om personen. Lösenord, hemligheter för andra faktorn och tokenhashvärden exporteras aldrig. API: `GET /api/v1/users/{id}/export`.

Varje användare kan ladda ner samma export om sig själv på sin **Profil** (**Ladda ner mina uppgifter**, `GET /api/v1/auth/me/export`). Båda registreras som `user.data_export`.

Innehåll som personen skapat i projekt (krav, tester, körningar, dokument) tillhör organisationen; exportera det med projektexporterna om begäran omfattar det.

### Radering (anonymisering)

**Radera användare** under **Administration → Användare** anonymiserar ett konto när du har skrivit dess e-postadress:

- namnet och adressen blir en pseudonym, **Former user #*n*** (`former-user-n@erased.invalid`), numrerad i ordning;
- raderas: personliga inställningar, profilbild, sessioner, API-tokens, andra faktorn och reservkoder, betrodda enheter, väntande signeringsbiljetter, aviseringar, inloggningsloggen, inmatningshistorik, inbjudningar, SCIM-kopplingar, medlemskap och projektroller;
- kontot inaktiveras, saknar lösenord och **kan aldrig logga in igen**; enkel inloggning kan inte längre matcha det;
- innehåll som personen skrivit finns kvar och **tillskrivs pseudonymen**: fält som anger upphovspersonen med e-postadress (skapad av, tilldelad, ägare, testare, uppladdad av och liknande) skrivs om, och `@`-omnämnanden av personen i kommentarer blir `@former-user-n`.

Du kan inte radera ditt eget konto eller den sista superadministratören. Raderingen registreras i granskningsloggen som `user.erase` med pseudonymen och antal – inte med den raderade adressen. API: `POST /api/v1/users/{id}/erase` med `{"confirm": "<e-postadress>"}`.

### Vad raderingen behåller, och varför

- **Elektroniska signaturer och godkännanden** (godkännanden av baslinjer, avsteg och ändringsbegäranden, `esignatures`) behåller signerarens namn och e-postadress precis som de signerades. 21 CFR Part 11 (§11.50, §11.70) och EU GMP Annex 11 (§14) kräver att signaturens manifestation förblir kopplad till den signerade posten; att ändra den skulle göra posten ogiltig. GDPR art. 17.3 b tillåter att uppgifter som behövs för att uppfylla en sådan rättslig förpliktelse sparas.
- **Granskningsloggen** behåller e-postadressen i de poster personen gjort, och poster som nämner personen i sina detaljer. Att skriva om en post bryter hashkedjan, och loggen är själv en post som regelverken kräver (Part 11 §11.10(e), Annex 11 §9). Användarlistan och nya vyer av granskningsloggen visar pseudonymen för kontot.
- **Säkerhetskopior** som tagits före raderingen innehåller fortfarande de ursprungliga uppgifterna. Spara säkerhetskopior bara så länge er gallringsplan anger, och upprepa raderingen efter en återställning.
- **Fritext** – ett namn som skrivits som vanlig text i en beskrivning, ett steg eller ett ägarfält – söks inte igenom och ersätts inte; granska den om begäran kräver det.

Dokumentera undantagen i registret över behandlingar och i svaret till personen.

## Integritetsinställningar

| Inställning | Verkan |
|---|---|
| `privacy.truncate_ip` | Klientadresser lagras och loggas förkortade – IPv4 till /24 (`192.0.2.0`), IPv6 till /48 – i inloggningsloggen, sessioner, betrodda enheter, begäransloggen och meddelandena om misslyckade inloggningar. Begränsningar av antal försök använder fortfarande hela adressen, bara i minnet. Granskningsposter innehåller aldrig klientadresser. |
| `privacy.notice_url` | En länk till er integritetspolicy, visas på inloggningssidan (tom = ingen). |

Båda finns under **Administration → Systeminställningar → Integritet** och gäller direkt.
