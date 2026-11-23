# Funktioner som lagts till senare

Det här kapitlet samlar administrationsanteckningarna på serversidan för funktioner som lagts till efter att resten av handboken skrevs: konfigurationsnycklar, behörigheter, bakgrundsjobb, data som en säkerhetskopia innehåller eller inte innehåller, och utgående anslutningar. Varje funktion som behöver en sådan anteckning får ett avsnitt nedan; till dess beskriver [användarhandboken](../../user-guide/sv/administration.md) funktionens skärmar.

När en funktion lägger till konfigurationsnycklar, systeminställningar, behörigheter, webhook-händelser, tabeller eller kommandoradsflaggor tar handbokens genererade delar (konfigurationsreferensen, systeminställningarna, behörighetsmatrisen, webhook-händelserna, tabellerna i en säkerhetskopia, migreringarna, kommandoraden) med dem via `make admin-guide-ref`; bara förklaringen hör hemma här.

<!--
Platshållare för anteckningar som läggs in vid sammanslagning (ett "##"-avsnitt
per funktion, hämtat från funktionens administrationsanteckningar i
docs/user-guide/<lang>/administration.md och kontrollerat mot koden):

## Verifieringsposter
## Undantag (waivers)
## Egna fält
## OSLC
## Kod-webhooks
## Varianter
-->

## Utforskande test

Utforskande testsessioner (migrering 0040) behöver ingen konfiguration. De använder behörigheterna för testkörningar: `run.read` för att se sessioner, `run.write` för att planera, ändra och ta bort dem och för att utvärdera eller stoppa någon annans session, `run.execute` för att genomföra sin egen session (tidtagning, anteckningar, defekter från anteckningar, utvärdering). Sessionens tidtagning hålls i databasen (`exploratory_sessions.running_since`, `elapsed_seconds`), så en omstart av servern tappar ingen sessionstid; en session som lämnas igång fortsätter att räkna tills någon pausar eller stoppar den.

Tabellerna `exploratory_sessions`, `exploratory_scope` och `exploratory_notes` ingår i säkerhetskopian och i projektarkiv; skärmbilder är vanliga bilagor (`files`, `attachments`) och räknas mot `files.max_upload_mb`. Om utvärderade sessioner är verifieringsbevis är en projektinställning (`verification_settings.exploratory_evidence`, avstängd som standard) som ändras av någon med `project.update`. Varje åtgärd skrivs till händelseloggen med åtgärderna `exploratory.*`.

## CI/CD-integration

Kvalitetsgrinden, testurvalet och pipelinemallarna (migrering 0053) behöver ingen konfiguration; serverns rapporter av commit-statusar och anteckningar i merge requests till GitLab är en projektinställning med en krypterad token och en utkorg i bakgrunden (`ci_deliveries`, som minutjobbet försöker igen, och som inte säkerhetskopieras). Tokens, roller, jobbtokens begränsningar och utgående anslutningar beskrivs i [E-post, webhooks och integrationer](integrations.md); ändringar av inställningarna loggas som `project.ci_settings`.

## Liveuppdateringar

Listor, översikter, notisklockan och öppna objekt uppdateras när någon annan ändrar något, och redigerare visar vem mer som tittar på eller redigerar ett objekt. Webbklienten håller en Server-Sent Events-ström öppen per flik i webbläsaren (`GET /api/v1/events/stream`, se API-referensen `docs/openapi.yaml`); det finns ingen tabell och ingen migrering, allt finns i minnet. Det är på som standard; `live.enabled: false` (eller `QTF_LIVE_ENABLED=false`) stänger av det för servern, och varje användare kan stänga av det i profilen. När det är av, inte går att nå eller avbryts gör klienten som förut: klockan och *Mitt arbete* frågar varje minut, listor läses in när de öppnas, och konfliktkontrollen vid sparande (version, `If-Match`, 412) skyddar fortfarande mot att skriva över.

**Vad som skickas.** En händelse säger att ett objekt har ändrats: entitetstyp, id, projekt, version, åtgärd och vem som gjorde det. Den innehåller aldrig titlar eller innehåll; klienten läser om det den visar genom det vanliga API:et, med dess behörighetskontroller. Varje händelse kontrolleras mot användarens åtkomst till projektet (projektdelning, projektroller) och läsbehörigheten för typen (`requirement.read`, `test.read`, `run.read`, `project.read` för defekter och releaser) innan den skrivs till en ström; händelser av andra typer eller utan projekt skickas inte alls. Notishändelser når bara mottagarna och innehåller inget annat än en signal. Ändringar av projektåtkomst, roller eller medlemmar gör att strömmarna läser om vad användarna får se; varje minut autentiserar en ström om sin begäran, så en återkallad session eller API-token, en avstängd användare eller ett borttaget medlemskap avslutar den. Händelserna kommer från granskningsloggen (varje loggad ändring), efter att begärans transaktion har sparats; en ändring som rullas tillbaka meddelas aldrig.

**Närvaro** (vem som tittar på eller redigerar ett objekt) skickas av webbläsaren var 20:e sekund medan ett objekt är öppet (`POST /api/v1/presence`) och glöms efter `live.presence_ttl` (45 s) utan signal. Den sparas inte och finns inte i granskningsloggen. En användare som har stängt av liveuppdateringar visas inte för andra.

**Flera serverinstanser.** På PostgreSQL utbyter instanserna som delar en databas händelser och närvaro med `LISTEN`/`NOTIFY` på en kanal som är uppkallad efter databasen och schemat (`qtf_live_…`); varje instans håller en extra databasanslutning för att lyssna. En lyssnare som tappar anslutningen ansluter igen med ökande väntetid och ber sina klienter att läsa om. På SQLite finns bara en instans per databasfil, och händelserna stannar i den processen (bara inom processen). Inga klibbiga sessioner behövs: en klient som ansluter till en annan instans fortsätter från sin senaste händelse.

**Återupptagning och gränser.** Varje instans håller de senaste `live.replay_buffer` (512) händelserna; en klient som ansluter igen med `Last-Event-ID` får det den missade, eller läser om när händelsen inte längre finns kvar. Högst `live.max_connections` (2000) strömmar per instans och `live.max_connections_per_user` (8) per användare; fler avvisas med `503` och klienten försöker igen med ökande väntetid (1 s upp till 1 min) och frågar med jämna mellanrum under tiden. En flik i bakgrunden stänger sin ström efter en minut och fortsätter när den visas igen. Kommentarsrader skickas var `live.heartbeat` (25 s) så att proxyer inte stänger strömmar utan händelser. Vid avstängning (`SIGTERM`) avslutas strömmarna direkt, så att de 15 sekunderna för att tömma anslutningar inte fördröjs; klienterna ansluter igen.

**Omvänd proxy.** Strömmen är ett långlivat svar som inte får buffras eller komprimeras. Qtestframe skickar `Content-Type: text/event-stream`, `Cache-Control: no-store, no-transform` och `X-Accel-Buffering: no` (nginx buffrar den då inte ens utan inställningarna nedan). Håll proxyns läs-tidsgräns över `live.heartbeat`. Med nginx:

```nginx
location /api/v1/events/stream {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_read_timeout 1h;
}
```

Med Traefik (dynamisk konfiguration) behöver strömmen ingen egen router; en komprimerings-middleware måste hoppa över den, och ingångspunktens `respondingTimeouts` måste ligga över `live.heartbeat` (standardvärdena räcker):

```yaml
http:
  routers:
    qtestframe:
      rule: Host(`qtf.example.com`)
      service: qtestframe
      middlewares: [compress-except-stream]
  middlewares:
    compress-except-stream:
      compress:
        excludedContentTypes: [text/event-stream]
  services:
    qtestframe:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:8080
```

Varje öppen flik är en HTTP-anslutning; över HTTP/1.1 tillåter webbläsare sex per webbplats, så om många flikar är öppna bör webbplatsen använda HTTP/2 (en anslutning för alla strömmar).

## Testmiljöer och testdata

Testmiljöer, deras bokningar och testdatamängder (migrering 0057) behöver ingen konfiguration. Den nya behörigheten `environment.manage` (testledare, förvaltare och administratörer som standard; migreringen lägger till den i sparade testledar- och förvaltarroller som har `run.write`) skapar och ändrar miljöer, godkänner bokningar och visar en miljös hemlighet; för en miljö som delas av organisationen hämtas den från organisationsrollerna, inte från projektroller. Att boka kräver `run.execute` i projektet bokningen gäller; testdatamängder använder `test.read` och `test.write`.

En miljös valfria hemlighet krypteras som de andra sparade hemligheterna (`environments.secret`, krypteras om av `qtestframe secrets rotate`), är endast skrivbar i API:t, utelämnas ur projektarkiv, och varje visning loggas som `environment.secret_reveal`. Använd hellre hänvisningen till inloggningsuppgifterna (en sökväg i ett valv).

Minutjobbet kontrollerar miljöerna som har en adress för hälsokontroll (högst 100 per minut, 8 åt gången, 10 s tidsgräns var) med serverns utgående HTTP-klient: adresserna anges av miljöförvaltare och servern måste kunna nå dem. En misslyckad kontroll markerar en tillgänglig miljö som trasig och en lyckad gör den tillgänglig igen; statusbyten loggas som `environment.health`. Tabellerna `environments`, `environment_bookings`, `test_data_sets`, `test_data_links`, `test_data_usage` och `test_data_refresh_requests` ingår i säkerhetskopian; ett projektarkiv innehåller projektets egna miljöer med deras bokningar och dess testdatamängder (delade miljöer stannar hos organisationen). Migreringen gör om de olika fritextmiljöerna i varje projekts körningar och cykler till miljöposter och kopplar körningarna, cyklerna och deras slutliga resultat till dem. Nya webhook- och chatthändelser: `environment.status_changed`, `environment.booking_requested`, `environment.booking_approved`, `environment.booking_rejected`, `testdata.refresh_requested`, `testdata.refresh_resolved`.
