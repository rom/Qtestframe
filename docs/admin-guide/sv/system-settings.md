# Systeminställningar under drift

Superadministratörer kan ändra en bestämd uppsättning inställningar i webbklienten, under **Administration → Systeminställningar** (API `GET`/`PUT /api/v1/admin/settings`), utan att redigera filen eller starta om servern. Ändringarna sparas i databasen (tabellen `system_settings`) och gäller hela installationen; ingen inställning gäller per organisation. Hur sidan ser ut beskrivs i [användarhandboken](../../user-guide/sv/administration.md#systeminställningar); det här kapitlet handlar om reglerna.

Bara superadministratörer **inloggade med en session** kan läsa eller ändra inställningarna; API-nycklar (även en superadministratörs) och organisationsadministratörer får `403`.

## Företräde och låsning

**Företräde**, från starkast till svagast:

1. en **miljövariabel** `QTF_*` – den vinner alltid och låser inställningen på sidan;
2. ett värde **ändrat i webbklienten** (sparat i databasen) – om inte `system_settings.locked` låser nyckeln, då vinner filen;
3. **konfigurationsfilen**;
4. det inbyggda **standardvärdet**.

Sidan visar varifrån varje värde kommer (*Standard*, *Konfigurationsfil*, *Miljö*, *Ändrat här*). Ett sparat värde som skuggas – nyckeln sattes senare av en miljövariabel eller låstes i filen – behålls men markeras *ignoreras*, och värdet som används visas. **Återställ till filens värde** tar bort det sparade värdet. Ordningen är densamma som innan sidan fanns; driftansvariga som vill att filen ska vinna låser nycklarna.

**Driftansvarig vinner alltid**, genom tre sorters lås:

| Lås | Hur | Verkan |
|---|---|---|
| Miljö | inställningen sätts av en `QTF_*`-variabel | skrivskyddad på sidan, med variabelns namn; ett sparat värde ignoreras |
| Konfiguration | `system_settings.locked` i filen listar den | skrivskyddad; ett sparat värde ignoreras |
| Bara filen | inställningen hör till processens uppsättning | visas som information, kan aldrig ändras under drift |

`system_settings.locked` tar nycklar, prefix som slutar med `.*`, eller `*` för allt:

```yaml
system_settings:
  locked: ["auth.oidc", "email.*", "runner.*"]
```

Använd miljövariabler eller lås för det som inte får ändras från webbklienten – i containrar är compose-filen en naturlig plats. Den som inte vill ha hemligheter i databasen låser `email.*` och `auth.oidc` och fortsätter använda `*_file`-inställningar.

## När en ändring gäller

| Avsnitt | Gäller |
|---|---|
| Allmänt, Inloggning och lösenord, API-tokens, Enkel inloggning, Loggning och syslog, E-post (SMTP), Utgående anslutningar, Likhet, AI-assistans | **direkt** – e-postavsändaren, OIDC-leverantörerna (oförändrade leverantörer behålls, så pågående inloggningar fortsätter), loggningen, inloggningsgränserna och den utgående HTTP-klienten följer de nya värdena |
| Liveuppdateringar | brytaren och pulsen **direkt** (nya anslutningar); anslutningsgränserna, repriskbufferten och tidsgränsen för närvaro **efter en omstart** |
| Server och databas | HTTP-tidsgränserna för läsning och skrivning **efter en omstart**; allt annat anges i filen och visas med sin källa (lyssningsadress, TLS-filer, databas, första start, härdning, nyckelleverantör) |
| Körning av skript och webbtester | **efter en omstart** – körningen och dess sandlåda sätts upp innan processen låses (Landlocks körbara sökvägar, arbetskatalogen); värdet sparas direkt och sidan visar värdet som används tills dess |
| Schemalagda säkerhetskopior | **direkt** (på/av, schema, gallring, lösenfras, uppladdning); katalogen **efter omstart**, eftersom servern öppnar den för skrivning innan processen låses – se [Schemalagda säkerhetskopior](backup-and-migration.md#schemalagda-säkerhetskopior) |
| Gallring och säkerhetskopior, Behörigheter, Flerfaktorsautentisering | inget att ändra här: de förklarar och länkar till sidorna som hanterar dem (papperskorgen behåller borttagna objekt i 30 dagar, vilket är fast) |

Inställningar märkta **Gäller efter omstart** behåller sitt värde tills nästa start. En banderoll överst på sidan listar varje sparad ändring som väntar på en omstart, med en länk till inställningen.

`auth.session_ttl` gäller sessioner som skapas efter ändringen. Nya värden kontrolleras tillsammans med resten av konfigurationen – samma kontroller som `check-config` – innan de sparas. Om de sparade inställningarna är ogiltiga vid start (till exempel efter en ändring av filen) loggar servern `stored system settings are invalid and were ignored`, använder filen, och sidan visar en varning; rätta eller återställ värdena där.

## Alla inställningar

Inställningarna som sidan erbjuder, genererade från `internal/settings`:

<!-- BEGIN GENERATED: system-settings (make admin-guide-ref) -->

| Avsnitt | Nyckel | Värden | Gäller |
|---|---|---|---|
| Allmänt | `server.base_url` | text | direkt |
| Allmänt | `files.max_upload_mb` | 1–1024 | direkt |
| Allmänt | `requirements.auto_create_test` | på/av | direkt |
| Server och databas | `server.listen` | text | visas bara (filen) |
| Server och databas | `server.tls_cert` | text | visas bara (filen) |
| Server och databas | `server.tls_key` | text | visas bara (filen) |
| Server och databas | `server.read_timeout` | 1s–2h | efter omstart |
| Server och databas | `server.write_timeout` | 1s–2h | efter omstart |
| Server och databas | `server.frame_ancestors` | lista | direkt |
| Server och databas | `server.trusted_proxies` | lista | visas bara (filen) |
| Server och databas | `server.hsts` | på/av | visas bara (filen) |
| Server och databas | `database.driver` | text | visas bara (filen) |
| Server och databas | `database.dsn` | hemlighet (skrivskyddad läsning) | visas bara (filen) |
| Server och databas | `database.max_open_conns` | tal | visas bara (filen) |
| Server och databas | `bootstrap.tenant_slug` | text | visas bara (filen) |
| Server och databas | `bootstrap.tenant_name` | text | visas bara (filen) |
| Server och databas | `bootstrap.admin_email` | text | visas bara (filen) |
| Server och databas | `hardening.no_new_privs` | på/av | visas bara (filen) |
| Server och databas | `hardening.landlock` | på/av | visas bara (filen) |
| Server och databas | `hardening.seccomp` | på/av | visas bara (filen) |
| Server och databas | `hardening.drop_capabilities` | på/av | visas bara (filen) |
| Server och databas | `hardening.read_only_paths` | lista | visas bara (filen) |
| Server och databas | `hardening.read_write_paths` | lista | visas bara (filen) |
| Server och databas | `secrets.provider` | text | visas bara (filen) |
| Inloggning och lösenord | `auth.local` | på/av | direkt |
| Inloggning och lösenord | `auth.session_ttl` | 5m–2160h | direkt |
| Inloggning och lösenord | `auth.session_idle_timeout` | 0s–720h | direkt |
| Inloggning och lösenord | `auth.password_min_length` | 8–72 | direkt |
| Inloggning och lösenord | `auth.password_min_classes` | 0–4 | direkt |
| Inloggning och lösenord | `auth.password_history` | 0–24 | direkt |
| Inloggning och lösenord | `auth.password_max_age_days` | 0–3650 | direkt |
| Inloggning och lösenord | `auth.login_max_attempts` | 1–1000 | direkt |
| Inloggning och lösenord | `auth.login_lockout` | 1m–24h | direkt |
| Inloggning och lösenord | `auth.cookie_secure` | på/av | visas bara (filen) |
| API-tokens | `auth.token_max_days` | 0–3650 | direkt |
| API-tokens | `auth.token_require_expiry` | på/av | direkt |
| Enkel inloggning | `auth.oidc` | lista med leverantörer | direkt |
| Loggning och syslog | `logging.level` | `debug`, `info`, `warn`, `error` | direkt |
| Loggning och syslog | `logging.format` | `text`, `json` | direkt |
| Loggning och syslog | `logging.syslog.enabled` | på/av | direkt |
| Loggning och syslog | `logging.syslog.network` | `udp`, `tcp`, `tls`, `unix`, `unixgram` | direkt |
| Loggning och syslog | `logging.syslog.address` | text | direkt |
| Loggning och syslog | `logging.syslog.format` | `rfc5424`, `rfc3164` | direkt |
| Loggning och syslog | `logging.syslog.facility` | `kern`, `user`, `mail`, `daemon`, `auth`, `syslog`, `lpr`, `news`, `uucp`, `cron`, `authpriv`, `ftp`, `local0`, `local1`, `local2`, `local3`, `local4`, `local5`, `local6`, `local7` | direkt |
| Loggning och syslog | `logging.syslog.tag` | text | direkt |
| Loggning och syslog | `logging.syslog.ca_file` | text | visas bara (filen) |
| Loggning och syslog | `logging.syslog.cert_file` | text | visas bara (filen) |
| Loggning och syslog | `logging.syslog.key_file` | text | visas bara (filen) |
| E-post (SMTP) | `email.enabled` | på/av | direkt |
| E-post (SMTP) | `email.host` | text | direkt |
| E-post (SMTP) | `email.port` | 1–65535 | direkt |
| E-post (SMTP) | `email.tls` | `starttls`, `tls`, `none` | direkt |
| E-post (SMTP) | `email.username` | text | direkt |
| E-post (SMTP) | `email.password` | hemlighet (skrivskyddad läsning) | direkt |
| E-post (SMTP) | `email.from` | text | direkt |
| E-post (SMTP) | `email.from_name` | text | direkt |
| E-post (SMTP) | `email.reply_to` | text | direkt |
| E-post (SMTP) | `email.timeout` | 1s–10m | direkt |
| E-post (SMTP) | `email.digest_hour` | 0–23 | direkt |
| Utgående anslutningar | `outbound.proxy` | text | direkt |
| Utgående anslutningar | `outbound.no_proxy` | lista | direkt |
| Utgående anslutningar | `outbound.allowed_hosts` | lista | direkt |
| Utgående anslutningar | `outbound.denied_hosts` | lista | direkt |
| Utgående anslutningar | `outbound.block_private` | på/av | direkt |
| Körning av skript och webbtester | `runner.enabled` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.max_output_bytes` | 1024–67108864 | efter omstart |
| Körning av skript och webbtester | `runner.deny_network` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.sandbox` | på/av | visas bara (filen) |
| Körning av skript och webbtester | `runner.work_dir` | text | visas bara (filen) |
| Körning av skript och webbtester | `runner.interpreters` | lista | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.enabled` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.web.browser` | `chromium`, `firefox`, `webkit` | efter omstart |
| Körning av skript och webbtester | `runner.web.channel` | text | efter omstart |
| Körning av skript och webbtester | `runner.web.browser_channels` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.web.headless` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.web.timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.web.job_timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.web.max_parallel` | 1–32 | efter omstart |
| Körning av skript och webbtester | `runner.web.max_evidence_mb` | 0–1024 | efter omstart |
| Körning av skript och webbtester | `runner.web.screenshots` | `on_failure`, `always`, `off` | efter omstart |
| Körning av skript och webbtester | `runner.web.trace` | `on_failure`, `always`, `off` | efter omstart |
| Körning av skript och webbtester | `runner.web.video` | `off`, `on_failure`, `always` | efter omstart |
| Körning av skript och webbtester | `runner.web.node` | text | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.playwright_dir` | text | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.command` | text | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.browsers_path` | text | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.executable_path` | text | visas bara (filen) |
| Liveuppdateringar | `live.enabled` | på/av | direkt |
| Liveuppdateringar | `live.heartbeat` | 1s–5m | direkt |
| Liveuppdateringar | `live.presence_ttl` | 5s–1h | efter omstart |
| Liveuppdateringar | `live.max_connections` | 1–1000000 | efter omstart |
| Liveuppdateringar | `live.max_connections_per_user` | 1–1000 | efter omstart |
| Liveuppdateringar | `live.replay_buffer` | 0–100000 | efter omstart |
| Likhet (dubbletter) | `similarity.embeddings.enabled` | på/av | direkt |
| Likhet (dubbletter) | `similarity.embeddings.url` | text | direkt |
| Likhet (dubbletter) | `similarity.embeddings.model` | text | direkt |
| Likhet (dubbletter) | `similarity.embeddings.api_key` | hemlighet (skrivskyddad läsning) | direkt |
| Likhet (dubbletter) | `similarity.embeddings.timeout` | 1s–5m | direkt |
| AI-assistans | `ai.enabled` | på/av | direkt |
| AI-assistans | `ai.provider` | `anthropic`, `openai` | direkt |
| AI-assistans | `ai.url` | text | direkt |
| AI-assistans | `ai.model` | text | direkt |
| AI-assistans | `ai.api_key` | hemlighet (skrivskyddad läsning) | direkt |
| AI-assistans | `ai.timeout` | 5s–10m | direkt |
| AI-assistans | `ai.max_tokens` | 256–128000 | direkt |
| AI-assistans | `ai.temperature` | text | direkt |
| AI-assistans | `ai.rate_limit` | 1–10000 | direkt |
| AI-assistans | `ai.features.draft_tests` | på/av | direkt |
| AI-assistans | `ai.features.improve_requirement` | på/av | direkt |
| AI-assistans | `ai.features.summarize_run` | på/av | direkt |
| AI-assistans | `ai.features.suggest_mappings` | på/av | direkt |
| AI-assistans | `ai.features.suggest_criteria` | på/av | direkt |
| AI-assistans | `ai.price_input` | text | direkt |
| AI-assistans | `ai.price_output` | text | direkt |
| Schemalagda säkerhetskopior | `backup.enabled` | på/av | direkt |
| Schemalagda säkerhetskopior | `backup.schedule` | text | direkt |
| Schemalagda säkerhetskopior | `backup.directory` | text | efter omstart |
| Schemalagda säkerhetskopior | `backup.keep_last` | 0–1000 | direkt |
| Schemalagda säkerhetskopior | `backup.keep_days` | 0–3650 | direkt |
| Schemalagda säkerhetskopior | `backup.passphrase` | hemlighet (skrivskyddad läsning) | direkt |
| Schemalagda säkerhetskopior | `backup.s3.enabled` | på/av | direkt |
| Schemalagda säkerhetskopior | `backup.s3.endpoint` | text | direkt |
| Schemalagda säkerhetskopior | `backup.s3.region` | text | direkt |
| Schemalagda säkerhetskopior | `backup.s3.bucket` | text | direkt |
| Schemalagda säkerhetskopior | `backup.s3.prefix` | text | direkt |
| Schemalagda säkerhetskopior | `backup.s3.access_key_id` | text | direkt |
| Schemalagda säkerhetskopior | `backup.s3.secret_access_key` | hemlighet (skrivskyddad läsning) | direkt |

<!-- END GENERATED: system-settings -->

Vad varje nyckel betyder står i [konfigurationsreferensen](configuration.md#referens). Varje nyckel i konfigurationsfilen finns antingen i tabellen – ändringsbar, eller *visas bara (filen)* med värde och källa – eller är avsiktligt bara i filen: hemligheternas `*_file`-alternativ, bootstrap-lösenordet, SAML-leverantörerna (de behöver nyckel- och certifikatfiler; sidan listar dem skrivskyddade med en knapp *Testa metadata*), `system_settings.locked` självt och detaljerna om nyckelleverantören för lagrade hemligheter (visas i krypteringspanelen under *Server och databas*). Ett test (`TestSettingsInventory`) ser till att det förblir så när en nyckel läggs till.

## Att använda sidan

- Sidonavigeringen grupperar avsnitten i *Server*, *Åtkomst* och *Tjänster*. **Sök** hittar en inställning i alla avsnitt via namn, hjälptext, nyckel, värde eller miljövariabel och hoppar dit.
- Varje avsnitt sparas för sig. **Spara avsnittet** visar först ändringarna – gammalt och nytt värde, hemligheter bara som *ett nytt värde sparas* – och om de gäller direkt eller efter en omstart.
- **Exportera inställningar (JSON)** laddar ned värdena som ändrats här (inte filens, aldrig hemligheter, utan OIDC-leverantörslistan) som `qtestframe-settings.json`; **Importera inställningar** läser en sådan fil på en annan server, hoppar över okända, låsta och hemliga nycklar och visar ändringarna innan de sparas. **Exportera konfigurationen** laddar fortfarande ned hela den gällande konfigurationen som YAML.

## Om single sign-on, syslog och e-post

- **OIDC-leverantörer.** När leverantörerna ändras i webbklienten sparas hela listan, som då **ersätter** `auth.oidc` i filen. Klienthemligheter för leverantörer som kommer från filen kopieras inte till databasen: de kommer fortsatt från `client_secret_file` eller `QTF_OIDC_<NAMN>_CLIENT_SECRET`, som alltid vinner. **Återställ till filens värde** går tillbaka till filens lista.
- **SAML**-leverantörer kan bara konfigureras i filen; sidan listar dem (entitets-ID, metadata, ACS-adress, attribut, rollkoppling) och *Testa metadata* läser identitetsleverantörens metadata.
- **Utgående anslutningar.** `outbound.*` gäller varje utgående HTTP-anrop: webhooks, chattkanaler, ärendehanterare och kravkällor, AI-assistans, tjänsten för inbäddningar, OIDC-discovery och hälsokontroller av miljöer (inte SMTP eller syslog). `allowed_hosts` och `denied_hosts` tar exakta namn, adresser eller `*.example.com`; `block_private` nekar loopback-, privata, länklokala och andra icke-publika adresser, kontrollerat på adressen som faktiskt ansluts (så ett DNS-namn som senare pekar på en intern adress nekas ändå), om inte värden uttryckligen är tillåten. *Kontrollera adress* testar en adress mot reglerna, även osparade ändringar. Med en proxy kontrolleras adresserna på servern före anropet. Ett proxylösenord kan bara komma från filen eller `HTTPS_PROXY`; sidan och exporten visar det som `REDACTED`. Det finns ingen separat hastighetsgräns för webhooks eller chatt utöver deras egna regler för omförsök.
- **Syslog över TLS** använder `ca_file`, `cert_file` och `key_file` från filen; de är läsbara efter låsningen eftersom de läggs till bland Landlocks skrivskyddade sökvägar vid start. En Unix-socket som inte var konfigurerad vid start kan nekas av Landlock; använd `udp`, `tcp` eller `tls`, eller starta om.
- **Hemligheter** som anges på sidan (SMTP-lösenordet, OIDC-klienthemligheter) kan bara skrivas, lagras [krypterade](security.md#kryptering-av-lagrade-hemligheter), loggas aldrig och visas aldrig igen.
- **Testknapparna** (*Testa anslutningen* för OIDC-discovery, *Testa metadata* för SAML, *Skicka testmeddelande* för syslog, *Skicka testmejl*, *Testa tjänsten* för inbäddningar och AI, *Kontrollera adress* för utgående anslutningar) får servern att ansluta till en adress som superadministratören anger. De följer reglerna för utgående anslutningar när de använder HTTP; filtrera serverns utgående trafik om den når interna tjänster.
- **Inte på sidan:** historiken för de schemalagda säkerhetskopiorna (den finns under Administration → Säkerhetskopiering, se [Schemalagda säkerhetskopior](backup-and-migration.md#schemalagda-säkerhetskopior)), metrikslutpunkter (det finns ingen; `/healthz` och `/readyz` är publika och innehåller inga data), standardvärden för CI och ärendehanterare (de är per projekt) och inställningar per organisation, som finns på organisationens egna sidor (AI-assistans, Elektroniska poster, Flerfaktorsautentisering) eller i dess regler (Administration → Organisationer).

## Tjänst för inbäddningar för liknande objekt

Misstänkt dubblerade krav och testfall, panelen *Liknande krav* i redigerarna och varningen vid import från katalogen jämför objekt på servern som standard: TF-IDF över projektet med ordstammar, stoppord och synonymer för svenska och engelska (se användarguiden, *Dubbletter och konflikter*). Ingenting skickas någonstans.

Om man vill kan en **OpenAI-kompatibel embeddings-slutpunkt** beräkna likheten i stället: **Administration → Systeminställningar → Likhet (dubbletter)**, eller `similarity.embeddings` i [konfigurationsfilen](configuration.md#referens).

| Inställning | Betydelse |
|---|---|
| `similarity.embeddings.enabled` | av som standard |
| `similarity.embeddings.url` | slutpunkten, t.ex. `https://llm.example.com/v1/embeddings`; den får `POST {"model": …, "input": [texter]}` och svarar `{"data": [{"index": i, "embedding": [tal]}]}` |
| `similarity.embeddings.model` | skickas som `model` (tom: tjänstens standard) |
| `similarity.embeddings.api_key` | skickas som `Authorization: Bearer …`; kan bara skrivas, lagras [krypterad](security.md#kryptering-av-lagrade-hemligheter); i filen bara som `api_key_file` |
| `similarity.embeddings.timeout` | längsta väntan (standard 30 s) |

**Integritet.** När den är på **skickas titel, beskrivning och acceptanskriterier för krav och titel, beskrivning, förutsättningar och steg för testfall till tjänsten**, för alla projekt i alla organisationer på servern, när likhetskontrollerna körs eller någon skriver i en redigerare (texten som skrivs skickas också). Använd en tjänst som drivs av er organisation eller som ni har avtal med; låt den annars vara av.

Vektorerna lagras i tabellen `similarity_embeddings` per objekt, modell och text (ett ändrat objekt skickas igen, ett oförändrat inte). Den är en cache: den ingår inte i säkerhetskopior eller projektarkiv och fylls på nytt efter en återställning. Byts modellen beräknas alla vektorer om. **Testa tjänsten** på sidan skickar en kort exempelmening och visar vektorns storlek. När tjänsten misslyckas eller inte svarar loggar servern en varning och använder den lokala likheten, så kontrollerna slutar aldrig fungera. Konfliktsökningen (motsatta förpliktelser, gränsvärden, uteslutande värden) körs alltid på servern. Kontrollernas gränsvärden gäller även tjänstens cosinuslikhet; tjänster skiljer sig, så granska de misstänkta dubbletterna när den har slagits på och justera gränsen för dubblettkontrollerna vid behov.

## AI-assistans

Qtestframe kan be en språkmodelltjänst om **förslag** som användarna granskar innan något sparas: utkast till tester för ett krav, en förbättrad formulering av ett krav, en sammanfattning av en underkänd körning med utkast till defekter, ramverksmappningar (ATT&CK, D3FEND, EMB3D, CWE) och acceptanskriterier (se användarhandboken, *AI-assistans*). Den är **av som standard** och kräver tre brytare:

1. **Administration → Systeminställningar → AI-assistans** (eller `ai` i [konfigurationsfilen](configuration.md#referens)): tjänsten och `ai.enabled`.
2. **Administration → AI-assistans**: organisationens brytare (`settings.manage`).
3. Varje projekt kan välja bort den: **Inställningar → Allmänt → AI-assistans** (`project.update`).

| Inställning | Betydelse |
|---|---|
| `ai.provider` | `anthropic`: Anthropics Messages API (`POST {url}/v1/messages`, nyckeln som `x-api-key`). `openai`: en OpenAI-kompatibel chat completions-slutpunkt (`POST {url}/chat/completions`, nyckeln som `Authorization: Bearer`; en Azure OpenAI-deployment-URL med `?api-version=…` skickar den som `api-key`): OpenAI, Azure OpenAI eller en lokal server som Ollama eller vLLM |
| `ai.url` | bas-URL; tom med `anthropic` betyder `https://api.anthropic.com`; krävs med `openai`, t.ex. `https://llm.example.com/v1` |
| `ai.model` | modell-id (krävs; det finns ingen inbyggd standard): modellen i ditt Anthropic-konto, eller din modell eller deployment för `openai` |
| `ai.api_key` | kan bara skrivas, lagras [krypterad](security.md#kryptering-av-lagrade-hemligheter); i filen bara som `api_key_file`; kan vara tom för en lokal server |
| `ai.timeout`, `ai.max_tokens`, `ai.temperature` | längsta väntan på ett svar (standard 2 min), högsta antal token i ett svar (standard 8192), temperaturen (tom: skickas inte) |
| `ai.rate_limit` | AI-förfrågningar per användare och timme (standard 30) |
| `ai.features.*` | stäng av enskilda funktioner |
| `ai.price_input`, `ai.price_output` | valfria priser per miljon token: varje svar visar då en uppskattad kostnad |

**Testa tjänsten** på sidan skickar en fast kort mening (inga objektdata) och visar modellen och antalet token.

**Dataflöde och integritet.** Ingenting skickas förrän en användare klickar på en *Assistera*-åtgärd. Servern skickar då **bara objekten för den åtgärden** till tjänsten, från serverns nätverk:

| Åtgärd | Skickas till tjänsten |
|---|---|
| Utkast till tester, Föreslå acceptanskriterier | kravets nyckel, titel, beskrivning, typ, förpliktelse, motivering och acceptanskriterier (redigerarens aktuella text) |
| Förbättra krav | detsamma, och fynden från kravkontrollerna |
| Sammanfatta fel | körningens namn, status, miljö och bygge; nyckel, titel, anteckningar, felutskrift (de sista 3000 tecknen) och underkända steg för upp till 40 underkända eller blockerade utföranden; nyckel, titel, status och allvarlighetsgrad för defekterna som är länkade till körningen |
| Föreslå mappningar | nyckel, titel, beskrivning (risker: kategori och åtgärd) och de id som redan är mappade |

Användarna ser en upplysning om att texten skickas innan de börjar. Använd en tjänst som drivs av er organisation eller som ni har ett avtal med som täcker dessa data (lagring, träning, plats). En lokal server (Ollama, vLLM) håller allt i ert nätverk.

**Säkerhet.** Tjänsten får inga verktyg och ingenting som den svarar körs. Objekttexten skickas som JSON mellan `<item_data>`-taggar och instruktionerna säger åt modellen att behandla den som data (JSON skyddar `<` och `>`, så ett objekt kan inte stänga taggen). Svaren måste vara ett JSON-objekt med ett fast schema; servern kontrollerar det strikt, frågar en gång till när det inte stämmer och tar bort det som inte kan verifieras (ramverks-id som inte finns i de inbyggda listorna, kriterier som redan finns, tester utanför körningen). Omskrivningen av ett krav kontrolleras igen med kravkontrollerna på servern, som visar vilka fynd den verkligen åtgärdar. Inga anrop till tjänsten görs inuti en databastransaktion, och användaren kan avbryta en begäran när som helst.

**Granskning.** Varje begäran skrivs i händelseloggen som `ai.request` med funktion, användare, objekt-id, leverantör och modell, promptversion, antal token, tid och utfall (`ok`, `cancelled`, `ai_refused`, `ai_invalid_answer`, `ai_service`, `ai_timeout`) — aldrig innehållet. Promptarna är inbyggda i programmet och versionerade (`prompt_version` i varje svar och händelse). Ändringar av organisationens brytare granskas som `settings.update`, projektets val som `project.ai_settings`, testknappen som `settings.test_ai`.

## Granskning och export

- Varje ändring skrivs i händelseloggen som `settings.update` med gammalt → nytt värde (hemligheter bara som *satt* / *ändrad* / *borttagen*), och i serverloggen. Testknapparna granskas som `settings.test_*`.
- **Exportera konfiguration** (`GET /api/v1/admin/settings/export`) laddar ned den gällande konfigurationen som YAML med hemligheter ersatta av `REDACTED` – till exempel för att flytta värdena till filen och sedan återställa dem på sidan.
- [Säkerhetskopior](backup-and-migration.md) innehåller tabellen `system_settings` med dess hemligheter, fortfarande krypterade; en återställning tillämpar den direkt. Projektarkiv innehåller den inte.
