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

<!-- END GENERATED: system-settings -->

Vad varje nyckel betyder står i [konfigurationsreferensen](configuration.md#referens). Allt annat – TLS, databasen, härdningen, nyckelleverantören, SAML, `trusted_proxies`, `cookie_secure`, `hsts`, körningens sökvägar – sätts bara i filen.

## Om single sign-on, syslog och e-post

- **OIDC-leverantörer.** När leverantörerna ändras i webbklienten sparas hela listan, som då **ersätter** `auth.oidc` i filen. Klienthemligheter för leverantörer som kommer från filen kopieras inte till databasen: de kommer fortsatt från `client_secret_file` eller `QTF_OIDC_<NAMN>_CLIENT_SECRET`, som alltid vinner. **Återställ till filens värde** går tillbaka till filens lista.
- **SAML**-leverantörer kan bara konfigureras i filen.
- **Syslog över TLS** använder `ca_file`, `cert_file` och `key_file` från filen; de är läsbara efter låsningen eftersom de läggs till bland Landlocks skrivskyddade sökvägar vid start. En Unix-socket som inte var konfigurerad vid start kan nekas av Landlock; använd `udp`, `tcp` eller `tls`, eller starta om.
- **Hemligheter** som anges på sidan (SMTP-lösenordet, OIDC-klienthemligheter) kan bara skrivas, lagras [krypterade](security.md#kryptering-av-lagrade-hemligheter), loggas aldrig och visas aldrig igen.
- **Testknapparna** (*Testa anslutningen* för OIDC-discovery, *Skicka testmeddelande* för syslog, *Skicka testmejl*) får servern att ansluta till en adress som superadministratören anger. Filtrera serverns utgående trafik om den når interna tjänster.

## Granskning och export

- Varje ändring skrivs i händelseloggen som `settings.update` med gammalt → nytt värde (hemligheter bara som *satt* / *ändrad* / *borttagen*), och i serverloggen. Testknapparna granskas som `settings.test_*`.
- **Exportera konfiguration** (`GET /api/v1/admin/settings/export`) laddar ned den gällande konfigurationen som YAML med hemligheter ersatta av `REDACTED` – till exempel för att flytta värdena till filen och sedan återställa dem på sidan.
- [Säkerhetskopior](backup-and-migration.md) innehåller tabellen `system_settings` med dess hemligheter, fortfarande krypterade; en återställning tillämpar den direkt. Projektarkiv innehåller den inte.
