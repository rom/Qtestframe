# Systeminställningar under drift

Superadministratörer kan ändra en bestämd uppsättning inställningar i webbklienten, under **Administration → Systeminställningar** (API `GET`/`PUT /api/v1/admin/settings`), utan att redigera filen eller starta om servern. Ändringarna sparas i databasen (tabellen `system_settings`) och gäller hela installationen; ingen inställning gäller per organisation. Hur sidan ser ut beskrivs i [användarhandboken](../../user-guide/sv/administration.md#systeminställningar); det här kapitlet handlar om reglerna.

Bara superadministratörer **inloggade med en session** kan läsa eller ändra inställningarna; API-nycklar (även en superadministratörs) och organisationsadministratörer får `403`.

## Företräde och låsning

**Företräde:** ett värde som ändrats i webbklienten vinner över `QTF_*`-miljövariabeln och konfigurationsfilen, som vinner över det inbyggda standardvärdet. Sidan visar varifrån varje värde kommer (*Standard*, *Konfigurationsfil*, *Miljö*, *Ändrat här*), och **Återställ till filens värde** tar bort det sparade värdet.

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
| Allmänt, Säkerhet, Enkel inloggning (OIDC), Loggning och syslog, E-post (SMTP) | **direkt** – e-postavsändaren, OIDC-leverantörerna (oförändrade leverantörer behålls, så pågående inloggningar fortsätter), loggningen och inloggningsgränserna byggs om |
| Körning av skript och webbtester | **efter en omstart** – körningen och dess sandlåda sätts upp innan processen låses (Landlocks körbara sökvägar, arbetskatalogen); värdet sparas direkt och sidan visar värdet som används tills dess |
| Gallring och säkerhetskopior | inget att ändra: papperskorgen behåller borttagna objekt i 30 dagar, vilket är fast |

`auth.session_ttl` gäller sessioner som skapas efter ändringen. Nya värden kontrolleras tillsammans med resten av konfigurationen – samma kontroller som `check-config` – innan de sparas. Om de sparade inställningarna är ogiltiga vid start (till exempel efter en ändring av filen) loggar servern `stored system settings are invalid and were ignored`, använder filen, och sidan visar en varning; rätta eller återställ värdena där.

## Alla inställningar

Inställningarna som sidan erbjuder, genererade från `internal/settings`:

<!-- BEGIN GENERATED: system-settings (make admin-guide-ref) -->

| Avsnitt | Nyckel | Värden | Gäller |
|---|---|---|---|
| Allmänt | `server.base_url` | text | direkt |
| Allmänt | `files.max_upload_mb` | 1–1024 | direkt |
| Allmänt | `requirements.auto_create_test` | på/av | direkt |
| Allmänt | `server.listen` | text | visas bara (filen) |
| Allmänt | `database.driver` | text | visas bara (filen) |
| Säkerhet | `auth.local` | på/av | direkt |
| Säkerhet | `auth.session_ttl` | 5m–2160h | direkt |
| Säkerhet | `auth.password_min_length` | 8–72 | direkt |
| Säkerhet | `auth.login_max_attempts` | 1–1000 | direkt |
| Säkerhet | `auth.login_lockout` | 1m–24h | direkt |
| Säkerhet | `auth.cookie_secure` | på/av | visas bara (filen) |
| Säkerhet | `server.hsts` | på/av | visas bara (filen) |
| Säkerhet | `server.trusted_proxies` | lista | visas bara (filen) |
| Enkel inloggning (OIDC) | `auth.oidc` | lista med leverantörer | direkt |
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
| E-post (SMTP) | `email.enabled` | på/av | direkt |
| E-post (SMTP) | `email.host` | text | direkt |
| E-post (SMTP) | `email.port` | 1–65535 | direkt |
| E-post (SMTP) | `email.tls` | `starttls`, `tls`, `none` | direkt |
| E-post (SMTP) | `email.username` | text | direkt |
| E-post (SMTP) | `email.password` | hemlighet (skrivskyddad läsning) | direkt |
| E-post (SMTP) | `email.from` | text | direkt |
| E-post (SMTP) | `email.from_name` | text | direkt |
| E-post (SMTP) | `email.timeout` | 1s–10m | direkt |
| E-post (SMTP) | `email.digest_hour` | 0–23 | direkt |
| Körning av skript och webbtester | `runner.enabled` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.max_output_bytes` | 1024–67108864 | efter omstart |
| Körning av skript och webbtester | `runner.deny_network` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.sandbox` | på/av | visas bara (filen) |
| Körning av skript och webbtester | `runner.web.enabled` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.web.browser` | `chromium`, `firefox`, `webkit` | efter omstart |
| Körning av skript och webbtester | `runner.web.headless` | på/av | efter omstart |
| Körning av skript och webbtester | `runner.web.timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.web.job_timeout` | 1s–24h | efter omstart |
| Körning av skript och webbtester | `runner.web.max_parallel` | 1–32 | efter omstart |
| Körning av skript och webbtester | `runner.web.max_evidence_mb` | 0–1024 | efter omstart |
| Körning av skript och webbtester | `runner.web.screenshots` | `on_failure`, `always`, `off` | efter omstart |
| Körning av skript och webbtester | `runner.web.trace` | `on_failure`, `always`, `off` | efter omstart |
| Körning av skript och webbtester | `runner.web.video` | `off`, `on_failure`, `always` | efter omstart |
| Körning av skript och webbtester | `runner.web.playwright_dir` | text | visas bara (filen) |
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

<!-- END GENERATED: system-settings -->

Vad varje nyckel betyder står i [konfigurationsreferensen](configuration.md#referens). Allt annat – TLS, databasen, härdningen, nyckelleverantören, SAML, `trusted_proxies`, `cookie_secure`, `hsts`, körningens sökvägar – sätts bara i filen.

## Om single sign-on, syslog och e-post

- **OIDC-leverantörer.** När leverantörerna ändras i webbklienten sparas hela listan, som då **ersätter** `auth.oidc` i filen. Klienthemligheter för leverantörer som kommer från filen kopieras inte till databasen: de kommer fortsatt från `client_secret_file` eller `QTF_OIDC_<NAMN>_CLIENT_SECRET`, som alltid vinner. **Återställ till filens värde** går tillbaka till filens lista.
- **SAML**-leverantörer kan bara konfigureras i filen.
- **Syslog över TLS** använder `ca_file`, `cert_file` och `key_file` från filen; de är läsbara efter låsningen eftersom de läggs till bland Landlocks skrivskyddade sökvägar vid start. En Unix-socket som inte var konfigurerad vid start kan nekas av Landlock; använd `udp`, `tcp` eller `tls`, eller starta om.
- **Hemligheter** som anges på sidan (SMTP-lösenordet, OIDC-klienthemligheter) kan bara skrivas, lagras [krypterade](security.md#kryptering-av-lagrade-hemligheter), loggas aldrig och visas aldrig igen.
- **Testknapparna** (*Testa anslutningen* för OIDC-discovery, *Skicka testmeddelande* för syslog, *Skicka testmejl*) får servern att ansluta till en adress som superadministratören anger. Filtrera serverns utgående trafik om den når interna tjänster.

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
