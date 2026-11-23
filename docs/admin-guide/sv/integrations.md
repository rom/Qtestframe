# E-post, webhooks och integrationer

## E-post

Utgående e-post är valfri. Med den kan användare få sina aviseringar via e-post (var och en direkt eller i en daglig sammanfattning, som de väljer i sin profil), administratörer kan bjuda in nya medlemmar via e-post, och rapporter kan skickas via e-post eller enligt ett schema. Utan den döljs funktionerna och API:t svarar att e-post inte är konfigurerad.

```yaml
server:
  base_url: https://qtf.example.com      # krävs med e-post: länkar i meddelandena
email:
  host: smtp.example.com
  port: 587                  # 587 = STARTTLS, 465 = implicit TLS
  tls: starttls              # starttls | tls | none (bara ett lokalt relä)
  username: qtestframe       # utelämna för ett relä utan autentisering
  password_file: smtp-password
  from: qtestframe@example.com
  from_name: Qtestframe
  timeout: 30s
  digest_hour: 7             # UTC
```

E-post är på medan `email.host` är satt (`enabled: false` behåller inställningarna men skickar ingenting). Samma inställningar kan ändras under drift under **Administration → Systeminställningar → E-post (SMTP)**, som också har **Skicka testmejl**; organisationsadministratörer har **Skicka testmejl** under **Medlemmar**. Båda visar e-postserverns svar när utskicket misslyckas.

- E-post skickas i bakgrunden, aldrig för en begäran som misslyckades; fel loggas på nivån WARN. Vid avstängning väntar servern på meddelanden som håller på att skickas (vart och ett begränsat av `email.timeout`).
- Den dagliga sammanfattningen och schemalagda rapporter kontrolleras varje minut; sammanfattningen skickas klockan `digest_hour` (UTC).
- Inbjudningslänkar gäller i 7 dagar och kan användas en gång. Att skicka rapporter via e-post är begränsat till 30 per användare och 10 minuter.
- Aviseringar, @omnämnanden och rapportmejl går bara till personer som får se projektet de handlar om.

## Aviseringar

Aviseringar i appen (tilldelningar, @omnämnanden, granskningsförfrågningar, ändringar av defekter, projektdelning) behöver ingen konfiguration. Användarna väljer i sin profil om de också vill få dem via e-post.

## Webhooks

Projekt kan skicka signerade händelser till andra system. Användare med `project.update` lägger upp dem i projektinställningarna; varje webhook har en adress, de händelser den vill ha (eller alla) och en hemlighet som visas en gång när den skapas eller byts.

<!-- BEGIN GENERATED: webhook-events (make admin-guide-ref) -->

| Objekt | Händelser |
|---|---|
| requirement | `requirement.created`, `requirement.updated` |
| test | `test.created`, `test.updated` |
| run | `run.created`, `run.updated`, `run.completed` |
| execution | `execution.updated` |
| defect | `defect.created`, `defect.updated`, `defect.status`, `defect.assigned`, `defect.regression` |
| risk | `risk.created`, `risk.updated` |
| release | `release.created`, `release.updated`, `release.released` |
| comment | `comment.created`, `comment.mentioned`, `comment.resolved` |
| review | `review.requested`, `review.completed` |
| baseline | `baseline.signed` |
| waiver | `waiver.requested`, `waiver.approved`, `waiver.rejected`, `waiver.revoked` |
| change_request | `change_request.submitted`, `change_request.review`, `change_request.approved`, `change_request.rejected`, `change_request.deferred`, `change_request.implemented`, `change_request.closed` |
| environment | `environment.status_changed`, `environment.booking_requested`, `environment.booking_approved`, `environment.booking_rejected` |
| testdata | `testdata.refresh_requested`, `testdata.refresh_resolved` |

<!-- END GENERATED: webhook-events -->

Varje leverans är en `POST` med JSON-innehållet `{"id", "event", "project_id", "actor", "time", "data"}` och huvudena:

| Huvud | |
|---|---|
| `X-Qtestframe-Event` | händelsen, t.ex. `run.completed` |
| `X-Qtestframe-Webhook` | webhookens id |
| `X-Qtestframe-Signature` | `sha256=` + hex-kodad HMAC-SHA256 av det råa innehållet med webhookens hemlighet |
| `User-Agent` | `Qtestframe-Webhook/1` |

Mottagare bör verifiera signaturen (jämför i konstant tid) och ignorera upprepade `id`. Leveranser görs när ändringen har sparats, avbryts efter 10 sekunder, **följer aldrig omdirigeringar** och försöks en gång; status för den senaste leveransen visas vid webhooken, och fel loggas (`webhook delivery`). Webhook-hemligheterna lagras krypterade. Inlästa projektarkiv innehåller inga hemligheter och deras webhooks är avstängda.

## Chattaviseringar

Projekt kan skicka sina händelser till chattkanaler: Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix och Discord. Användare med `project.update` lägger upp kanalerna i projektinställningarna (se användarhandboken, *Granskningar och notiser*). Inget behöver konfigureras på servern, men:

- Ange `server.base_url` (eller systeminställningen): utan den kan meddelandena inte länka till objekten.
- Servern måste nå chattverktyget över HTTPS (utgående proxy och brandvägg). Anropen avbryts efter 10 sekunder och följer aldrig omdirigeringar.
- En kanals webhook-URL eller Matrix-token lagras krypterad som de andra hemligheterna (`qtestframe secrets` krypterar om den), returneras aldrig av API:t, skrivs inte till granskningsloggen eller i ett felmeddelande och tas inte med i projektarkiv (en inläst kanal är avstängd tills någon anger en ny URL).

**Händelser.** En kanal erbjuder händelserna i det centrala händelseregistret (samma som för webhooks ovan), grupperade för inställningarna. Listan härleds ur registret: en händelse som en senare version lägger till syns i kanalinställningarna och i **Alla händelser** utan någon ändring i chattkoden; gruppen tas från den del av namnet som står före punkten (okända delar hamnar under *Övrigt*), etiketten från händelsens namn om ingen översättning finns.

<!-- BEGIN GENERATED: chat-events (make admin-guide-ref) -->

| Grupp | Händelser |
|---|---|
| Testkörningar | `run.created` (Testkörning skapad), `run.updated` (Testkörning ändrad), `run.completed` (Testkörning avslutad), `execution.updated` (Testresultat registrerat), `environment.status_changed` (Miljöns status ändrad), `environment.booking_requested` (Miljöbokning begärd), `environment.booking_approved` (Miljöbokning godkänd), `environment.booking_rejected` (Miljöbokning avslagen), `testdata.refresh_requested` (Uppdatering av testdata begärd), `testdata.refresh_resolved` (Begäran om uppdatering av testdata stängd) |
| Defekter | `defect.created` (Defekt rapporterad), `defect.updated` (Defekt ändrad), `defect.status` (Defektens status ändrad), `defect.assigned` (Defekt tilldelad), `defect.regression` (Regression upptäckt) |
| Krav och tester | `requirement.created` (Krav skapat), `requirement.updated` (Krav ändrat), `test.created` (Test skapat), `test.updated` (Test ändrat), `comment.created` (Ny kommentar), `comment.mentioned` (Comment mentioned), `comment.resolved` (Comment resolved), `baseline.signed` (Baslinje signerad) |
| Granskningar | `review.requested` (Granskning begärd), `review.completed` (Granskning klar) |
| Releaser | `release.created` (Release skapad), `release.updated` (Release ändrad), `release.released` (Release släppt) |
| Undantag | `waiver.requested` (Undantag begärt), `waiver.approved` (Undantag godkänt), `waiver.rejected` (Undantag avslaget), `waiver.revoked` (Undantag återkallat) |
| Ändringsbegäranden | `change_request.submitted` (Ändringsbegäran inskickad), `change_request.review` (Ändringsbegäran att besluta), `change_request.approved` (Ändringsbegäran godkänd), `change_request.rejected` (Ändringsbegäran avslagen), `change_request.deferred` (Ändringsbegäran uppskjuten), `change_request.implemented` (Ändringsbegäran genomförd), `change_request.closed` (Ändringsbegäran stängd) |
| Risker | `risk.created` (Risk identifierad), `risk.updated` (Risk ändrad) |

<!-- END GENERATED: chat-events -->

**Leverans.** Varje meddelande sparas i en kö (tabellen `chat_deliveries`) när ändringen är sparad och skickas direkt i bakgrunden. Ett misslyckat meddelande försöks igen av serverns minutjobb med exponentiell väntetid (1, 2, 4, 8 och 16 minuter, eller `Retry-After` i ett HTTP 429-svar, högst en timme), upp till 6 försök; ett HTTP 4xx-svar utom 408 och 429 (chattverktyget vägrade URL, token eller meddelande) är slutgiltigt. Flera servrar som delar en PostgreSQL-databas skickar aldrig samma meddelande två gånger (ett meddelande reserveras innan det skickas). Efter 5 misslyckade meddelanden i rad stänger kanalen av sig själv; projektets administratörer får en avisering i appen (och ett e-postmeddelande om de valt omedelbar e-post), och granskningsloggen får en post `chat.disabled`. De senaste 50 meddelandena för varje kanal sparas som dess leveranslogg; kön och loggen ingår inte i säkerhetskopior (kanalerna gör det). Kanaler med sammandrag samlar sina händelser och skickar dem som ett meddelande var N:e minut (1–1440).

**Innehåll.** Meddelandena innehåller händelsen, objektets nyckel och titel, status, prioritet, personen och en sammanfattning på högst några hundra tecken. De innehåller aldrig hemligheter. I ett projekt med begränsad åtkomst innehåller meddelandena bara händelse, nyckel, status och länk, om inte en organisationsadministratör (`tenant.manage`) tillåter kanalen hela innehållet. Massomnämnanden (`@channel`, `@here`, `@all`, `@everyone`) i texterna oskadliggörs.

### Så lägger du upp varje chattverktyg

| Verktyg | Vad du skapar | Vad du anger i Qtestframe |
|---|---|---|
| Slack | En Slack-app med **Incoming Webhooks** påslaget; **Add New Webhook to Workspace** och välj kanalen. Meddelandena använder Block Kit. | Webhook-URL:en `https://hooks.slack.com/services/…` |
| Microsoft Teams | I kanalen: **Arbetsflöden** → mallen *Publicera i en kanal när en webhook-begäran tas emot* (Power Automate). Meddelandena är Adaptive Cards 1.4. | URL:en till arbetsflödets HTTP-utlösare |
| Microsoft Teams (Office 365-anslutning) | Den äldre anslutningen **Incoming Webhook** för en kanal (formatet MessageCard). Microsoft fasar ut dessa anslutningar: använd den bara där Arbetsflöden inte finns. | Anslutningens URL |
| Mattermost | **Integrations → Incoming Webhooks → Add Incoming Webhook**, välj kanalen. Tillåt att integrationer ändrar användarnamnet om meddelandena ska visas som *Qtestframe*. | URL:en `https://<server>/hooks/<id>` |
| Rocket.Chat | **Administration → Workspace → Integrations → New → Incoming**: slå på den, välj kanal (`#qa`) och användaren som skriver; inget skript behövs. | Webhook-URL:en som visas när du sparat |
| Google Chat | I utrymmet: **Appar och integrationer → Webhooks → Lägg till webhook**. Meddelandena är kort (cardsV2). | Webhook-URL:en (den innehåller parametrarna `key` och `token`) |
| Matrix | Ett botkonto på hemservern; bjud in det till rummet och låt det gå med. Hämta dess åtkomsttoken (till exempel genom att logga in med det en gång via klient-API:t). Meddelandena är `m.notice`-händelser med HTML. | Hemserverns URL, rummets ID (`!…:server`) eller alias (`#…:server`) och åtkomsttoken |
| Discord | **Kanalinställningar → Integrationer → Webhooks → Ny webhook**. Meddelandena är embeds; omnämnanden är avstängda. | Webhook-URL:en `https://discord.com/api/webhooks/…` |

Använd **Skicka testmeddelande** för att kontrollera en kanal; leveransloggen visar HTTP-status och fel för varje meddelande (till exempel `HTTP 404: no_service` från Slack när webhooken tagits bort, `HTTP 401: M_UNKNOWN_TOKEN` från Matrix). Personliga chattaviseringar (direktmeddelanden till en användare) erbjuds inte; användarna får sådana som aviseringar i appen och e-post.

## Defekt- och ärendesystem

Projekt kan skicka defekter och krav till **Jira**, **GitHub**, **GitLab** eller **Request Tracker (RT)**, synkronisera deras status tillbaka och importera ärenden. Användare med `project.update` ansluter ett projekt i dess inställningar ([användarhandboken](../../user-guide/sv/defects.md#anslut-jira-github-eller-gitlab)); att skicka och synkronisera kräver `run.execute`, ärenden från krav `requirement.write`.

| System | Adress | Inloggning | Minsta användbara omfattning |
|---|---|---|---|
| Jira Cloud | `https://<webbplats>.atlassian.net` | användarens e-post + API-token (HTTP Basic) | en användare som får bläddra bland och skapa ärenden i projektet |
| Jira Server / Data Center | er Jira-adress | personlig åtkomsttoken (Bearer) | som ovan |
| GitHub | standard `https://api.github.com` (GitHub Enterprise: dess API-adress) | token (Bearer) | finkornig token med *Issues: read and write* för det enda repot |
| GitLab | standard `https://gitlab.com`, eller en egen instans (HTTPS) | token i `PRIVATE-TOKEN` | projektåtkomsttoken med omfattningen `api` och rollen Reporter |
| Request Tracker 5 | RT:s basadress, t.ex. `https://rt.example.com` (REST2 under `/REST/2.0`) | autentiseringstoken i `Authorization: token …` | en RT-användare med *CreateTicket*, *ShowTicket*, *ModifyTicket* och *CommentOnTicket* i kön |

Token anges en gång, lagras krypterad och returneras aldrig av API:t (bara *har token*); den ingår inte i ett projektarkiv. Begäranden följer `HTTPS_PROXY`/`NO_PROXY` och systemets CA-certifikat, har en tidsgräns, och väntar på systemets `Retry-After` när den är kort. Utskick och synkroniseringar granskas (`defect.push`, `defect.sync`, `requirement.issue.*`).

### Request Tracker

RT behöver REST2-API:et, som ingår i RT 5 (för RT 4.4 tillägget *RT::Extension::REST2*). Skapa en egen RT-användare för Qtestframe, ge den rättigheterna ovan i kön (en grupprättighet på kön är enklast), logga in som den användaren och skapa en token under **Logged in as → Settings → Auth Tokens**. Projektinställningarna tar RT:s basadress, **kön** (namn eller id) och eventuellt namnet på ett anpassat fält som tar emot etiketter (RT har inga egna etiketter). Kontrollera att en omvänd proxy framför RT skickar vidare huvudet `Authorization` till `/REST/2.0`. Statussynkroniseringen översätter standardlivscykeln (*new*, *open*, *stalled*, *resolved*, *rejected*, *deleted*); statusar i egna livscykler visas men ändrar inte defekten. Synkronisering sker på begäran (**Synkronisera** / **Synkronisera alla**, eller `POST /api/v1/projects/{pid}/defects/sync` från en schemaläggare), som för de andra systemen: ingen inkommande webhook behövs.

## Kravimport från Jira och Azure DevOps

Användare med `requirement.write` importerar krav från en JQL-fråga i Jira eller en WIQL-fråga i Azure DevOps ([användarhandboken](../../user-guide/sv/reports-and-export.md#jira-och-azure-devops)); anslutningarna ställs in av användare med `project.update` under **Projekt → Importkällor** och lagras i `projects.import_sources` (migrering 0037).

| Källa | Adress | Inloggning | Minsta användbara omfattning |
|---|---|---|---|
| Jira Cloud | `https://<webbplats>.atlassian.net` (eller ingen: projektets Jira-ärendehanterare används) | användarens e-post + API-token (HTTP Basic) | *Browse projects* i de projekt som frågas |
| Jira Server / Data Center | er Jira-adress | personlig åtkomsttoken (Bearer) | som ovan |
| Azure DevOps Services | `https://dev.azure.com/<organisation>` och projektets namn | personlig åtkomsttoken (HTTP Basic med tomt användarnamn) | omfattningen *Work Items (Read)*, begränsad till den enda organisationen |
| Azure DevOps Server | samlingens adress, t.ex. `https://tfs.example.com/tfs/DefaultCollection` | personlig åtkomsttoken | som ovan |

Jira söks med `POST /rest/api/2/search/jql` (Jira Cloud) och, när den inte finns, `POST /rest/api/2/search` (Server och Data Center); Azure DevOps med slutpunkterna för WIQL, fält och arbetsobjekt i REST API 7.1. Högst 1 000 objekt läses per import. Importen läser bara; den granskas som `requirement.import`, ändringar av anslutningen som `import_sources.update`. Token krypteras med serverns hemliga nycklar (de krypteras om vid nyckelrotation), returneras aldrig och utelämnas ur projektarkiv. Importen når källorna via serverns utgående proxy (`HTTPS_PROXY`, `NO_PROXY`) som ärendehanterarna; tillåt adresserna ovan i en brandvägg.

### Tvåvägssynk

Tvåvägssynken (migrering 0060, [användarhandboken](../../user-guide/sv/reports-and-export.md#tvåvägssynk-med-jira-och-azure-devops)) använder samma anslutningar och skriver till ärendesystemet, så dess token behöver mer än läsrätt:

| Källa | Behövs för synken |
|---|---|
| Jira Cloud, Server, Data Center | *Browse projects*, *Edit issues*, *Transition issues*; *Create issues* för att skicka nya krav, *Add comments* för kommentarer, *Link issues* för länkar |
| Azure DevOps | personlig åtkomsttoken med omfånget *Work Items (Read & write)* |

Användare med `project.update` ändrar inställningarna (**Ärendesynk → Inställningar**), användare med `requirement.write` kör en synk, förhandsgranskar den och avgör konflikter; alla som kan läsa projektet ser översikten och loggen. Schemalagda synkar och webhook-synkar körs som det konto som senast sparade inställningarna (`req_sync_configs.run_as`), med dess aktuella roller och projektåtkomst: ett konto som förlorar `requirement.write` eller projektet stoppar synken (loggas som misslyckad). Ändringar som hämtas från ärendesystemet går genom den vanliga uppdateringen av krav (behörigheter, granskningsregler, historik, granskningslogg, optimistisk låsning) med konfigurationens val av påverkan.

**Bakgrundsjobb och gränser.** Minutjobbet kör de synkar som står på tur (`interval_minutes`, 0 = inget schema). En körning håller ett lås i databasen (`lease_until`, 15 minuter), så med flera servrar kör bara en av dem en konfiguration åt gången. En körning som misslyckas fördubblar väntan för varje misslyckande i rad (högst ett dygn); ett ärendesystem som svarar 429 försöks igen av klienten efter en kort `Retry-After`, annars stoppas körningen och nästa väntar minst så länge som begärts. Skrivningar sprids ut enligt *Skrivningar per minut* (standard 60) och är högst 500 per körning. Högst 1 000 objekt läses per fråga, plus de länkade objekt som frågan inte längre hittar (Jira: en begäran per objekt; Azure DevOps: i omgångar om 200).

**Webhooks.** Mottagarna är publika ändpunkter; den delade hemligheten (16–200 tecken, krypterad i `req_sync_configs.webhook_secret`, skrivskyddad, omkrypteras vid nyckelrotation) är behörighetsbeviset. Varje anrop besvaras med 202 och synken av de namngivna objekten körs i bakgrunden; anrop för samma konfiguration slås ihop. Högst 600 anrop per projekt och minut tas emot.

| Ärendesystem | Adress | Inställning |
|---|---|---|
| Jira Cloud | `<base_url>/api/v1/hooks/req-sync/jira/<projekt-id>` | **System → WebHooks**: händelserna *Issue created*, *Issue updated*, *Comment created*; hemligheten i webhookens fält *Secret* (innehållet signeras, huvudet `X-Hub-Signature: sha256=…`). |
| Jira Server / Data Center | samma adress med `?secret=<hemligheten>` | **System → WebHooks** (där finns inget fält för hemligheten, så den läggs i adressen: använd HTTPS). |
| Azure DevOps | `<base_url>/api/v1/hooks/req-sync/azure-devops/<projekt-id>` | **Project settings → Service hooks → Web Hooks**: *Work item created*, *Work item updated*, *Work item commented on*; *Basic authentication* med valfritt användarnamn och hemligheten som lösenord. |

Huvudet `X-Qtestframe-Secret: <hemligheten>` godtas från båda. Ärendesystemet måste nå `server.base_url`; öppna brandväggen för det, eller lita på schemat.

**Data.** Synkläget för varje länkat krav sparas med dess externa referens (`external_refs`: ärendesystemets version, kontrollsummor och värden på båda sidor efter senaste synken). `req_sync_configs` och `req_sync_conflicts` ingår i säkerhetskopian; `req_sync_runs` (loggen, de 100 senaste körningarna per projekt och källa) gör det inte. Projektarkiv utelämnar alla tre (hemligheten och kontot); ett inläst projekt måste ställas in på nytt. Granskningsloggens åtgärder: `req_sync.update` (inställningar), `req_sync.run` (varje körning med sina antal), `requirement.sync.push` och `requirement.sync.create` (ändringar till ärendesystemet), `req_sync.conflict.resolve` och `req_sync.conflict.closed`, utöver de vanliga `requirement.update`, `requirement.create` och `relation.create` för ändringarna här.

## CI/CD-pipelines (GitLab, GitHub Actions)

Pipelines pratar med servern med kommandot `qtestframe` (`report`, `gate`, `select`; [kommandoradsreferensen](cli-reference.md)) och en API-nyckel. Inget behöver ställas in på servern; det här behöver varje del:

- **API-nyckel.** Skapa en egen CI-användare, gör den till medlem med rollen **tester** i projekten den rapporterar till (tester har `run.execute` för uppladdningar, `run.write` för `--create-run` och `test.write` för `--create-missing`), och skapa dess API-nyckel med rollen *tester* under **Administration → API-nycklar**. En nyckel får aldrig mer än användarens aktuella roll. Spara den som en maskerad CI/CD-variabel `QTF_TOKEN`. Det finns inga finare behörigheter för API-nycklar än roller; en egen roll med bara `project.read`, `test.read`, `run.read`, `run.execute` och `run.write` är den snävaste som kör alla kommandon.
- **Programmet.** Jobben behöver en avbildning med det statiska programmet `qtestframe` (`QTF_CLI_IMAGE`); serverns avbildning är distroless, så kopiera programmet till din CI-avbildning (`COPY --from=qtestframe:<version> /usr/bin/qtestframe /usr/local/bin/`).
- **Mallar.** `examples/ci/` innehåller mallar för `.gitlab-ci.yml` (med och utan testurval), en CI/CD-komponent och ett arbetsflöde för GitHub Actions; samma mallar, med serverns adress och projektnyckeln ifyllda, finns under **Inställningar → Projekt → CI-uppsättning** och på den publika `GET /api/v1/ci/templates/{name}` (för `include: remote:`; de innehåller inga hemligheter).
- **Commit-statusar och anteckningar i merge requests från pipelinen** behöver en GitLab-token i `QTF_GITLAB_TOKEN`: en projektåtkomsttoken med rollen **Developer** (commit-statusar kräver Developer, anteckningar Reporter) och behörigheten `api`. GitLab godtar när detta skrivs inte jobbtoken `CI_JOB_TOKEN` för `POST /projects/:id/statuses/:sha` eller anteckningar i merge requests (dess finkorniga behörigheter för jobbtoken täcker dem inte); kommandot faller tillbaka på den och skriver en varning när GitLab avvisar den. I GitHub Actions räcker arbetsflödets `GITHUB_TOKEN` med `statuses: write` och `pull-requests: write` (`QTF_GITHUB_TOKEN`).
- **Rapporter från servern.** Ett projekt kan i stället låta servern skicka: **Inställningar → Projekt → Resultat tillbaka till GitLab** sparar GitLabs API-adress, projektet och en projektåtkomsttoken (krypterad som de andra hemligheterna, bara skrivbar, omkrypteras av `qtestframe secrets`, utelämnas ur projektarkiv). När resultat med `?commit=` kommer in läggs en rad i utkorgen `ci_deliveries` och skickas i bakgrunden: `qtestframe/tests` (och `qtestframe/gate` med en release eller villkor) och en anteckning i merge requesten som uppdateras i stället för att upprepas. Misslyckade försök görs om av minutjobbet med exponentiell väntan (1, 2, 4, … minuter, högst 6 försök; vid HTTP 429 väntar den så länge GitLab ber om); en avvisad token eller ett avvisat projekt (övriga 4xx) försöks inte igen. De 50 senaste rapporterna per projekt visas i inställningarna; `ci_deliveries` säkerhetskopieras inte. Servern behöver utgående HTTPS till GitLab-instansen.

## Körning av skript och webbtester

Körningen utför skripten i automatiska tester (`sh`, `bash`, `python`, `javascript`) och webbtester (Playwright) på servern. Den är **av som standard**; läs [avsnittet om sandlådan](security.md#sandlådan-för-skript-och-webbtester) innan den slås på. Inställningarna gäller efter en omstart.

### Skript

```yaml
runner:
  enabled: true
  timeout: 60s                       # håll under server.write_timeout
  work_dir: /var/lib/qtestframe/runner
  max_output_bytes: 1048576
  interpreters: {sh: /bin/sh, python: /usr/bin/python3}   # bara det som behövs
  sandbox: true
  deny_network: true                 # när skripten inte behöver nätverk
```

*Kör skript* för ett enskilt skript körs inom begäran, så `runner.timeout` måste vara kortare än `server.write_timeout`. *Kör automatiska tester* för en körning lägger alla automatiska tester i körningen i kö som bakgrundsjobb. Arbetskatalogen skapas med läget 0700 och läggs till bland Landlocks sökvägar; jobbkatalogerna är tillfälligt utrymme.

Under systemd behöver Node.js `MemoryDenyWriteExecute=no` i en drop-in (dess JIT), eller konfigurera tolken som ett omslag som kör `node --jitless`; med SELinux slå på `qtestframe_run_scripts` (och `qtestframe_runner_execmem` för tolkar med JIT). AppArmor kör tolkar i barnprofilen `qtestframe//runner` utan nätverk.

### Webbtester (Playwright)

Installera Node.js 18 eller senare och Playwright med minst en webbläsare:

```sh
sudo mkdir -p /opt/qtestframe-playwright && cd /opt/qtestframe-playwright
sudo npm init -y && sudo npm i @playwright/test
sudo PLAYWRIGHT_BROWSERS_PATH=/opt/qtestframe-playwright/browsers npx playwright install --with-deps chromium
```

```yaml
runner:
  enabled: true
  work_dir: /var/lib/qtestframe/runner       # håll den kort
  web:
    enabled: true
    node: /usr/bin/node
    playwright_dir: /opt/qtestframe-playwright
    browsers_path: /opt/qtestframe-playwright/browsers
    browser: chromium
    timeout: 60s              # per test
    job_timeout: 10m          # hela Playwright-processen
    max_parallel: 2           # per serverprocess
    screenshots: on_failure
    trace: on_failure
    video: "off"
```

Och en drop-in för systemd:

```ini
[Service]
MemoryDenyWriteExecute=no
ExecPaths=/opt/qtestframe-playwright
ReadWritePaths=/dev/shm
```

För varje webbtest skriver servern specen och en genererad `playwright.config.js` i en jobbkatalog och kör `<node> <playwright_dir>/node_modules/@playwright/test/cli.js test --config <fil>` (`npx` används inte, inget laddas ned); resultatet tas från Playwrights JSON-rapport. Skärmbilder, trace och video blir bilagor till exekveringen (var och en högst `max_evidence_mb`). En trace innehåller testets sidor, nätverksbegäranden (även kakor) och konsol.

Webbtester körs alltid som **bakgrundsjobb**; `max_parallel` av dem körs samtidigt per serverprocess. Jobb överlever inte en omstart: oavslutade markeras som avbrutna och deras exekveringar får tillbaka sitt tidigare resultat, vid start (SQLite) eller när instansen som köade dem inte hörts av på ungefär en minut (PostgreSQL).

Projekten anger adressen till systemet som testas och testvariabler under **Inställningar → Webbtester**. Testvariabler når varje automatiskt skript som `QTF_VAR_<NAMN>`; hemliga lagras krypterade och maskeras som `***` i sparad utdata (inte i skärmbilder eller trace).

Containeravbilden kräver `--build-arg WITH_PLAYWRIGHT=true` och `QTF_RUNNER_ENABLED=true`, `QTF_RUNNER_WEB_ENABLED=true` (se [Installation](installation.md#i-en-container-docker-eller-podman)).
