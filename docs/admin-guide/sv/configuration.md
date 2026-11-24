# Konfiguration

## Konfigurationsfilen

Konfigurationen är en **YAML**-fil som anges med `--config FIL` eller miljövariabeln `QTF_CONFIG`. Alla kommandon läser samma fil. Utan fil gäller de inbyggda standardvärdena (HTTP på `:8080`, SQLite-filen `qtestframe.db` i arbetskatalogen).

Tre filer att utgå från:

| Fil | Användning |
|---|---|
| `examples/qtestframe.yaml` | lokal provning: HTTP på `:8080`, SQLite i `./data`, skriptkörning på, bootstrap-administratör |
| `examples/qtestframe-tls.yaml` | samma med HTTPS på `:8443` och certifikatet från `make certs-dev` |
| `deploy/config.example.yaml` | den kommenterade produktionsmallen som `install.sh` kopierar till `/etc/qtestframe/config.yaml` |

Filen tolkas **strikt**: en okänd eller felstavad nyckel (`listn:`) och ett andra YAML-dokument stoppar programmet med ett fel som anger raden. Kontrollera en fil utan att starta servern, hemlighetsfiler inräknade:

```sh
qtestframe check-config --config /etc/qtestframe/config.yaml
```

Tidslängder skrivs som i Go: `30s`, `15m`, `12h`, `1h30m`.

## Var ett värde kommer ifrån

Senare källor vinner:

1. de inbyggda standardvärdena (kolumnen *Standard* i [referensen](#referens));
2. YAML-filen;
3. `QTF_*`-miljövariabler ([lista](#miljövariabler));
4. flaggor på kommandoraden för `serve`, `check-config` och `healthcheck`: `--listen VÄRD:PORT` ersätter `server.listen`, `--port N` / `-p N` bara dess port;
5. för `serve`: de [systeminställningar](system-settings.md) som superadministratörer har ändrat i webbklienten och som sparats i databasen – utom inställningar som en miljövariabel sätter eller som `system_settings.locked` listar.

Hemligheter har egna regler, som tillämpas efter miljön:

| Hemlighet | Ordning (den sista som är satt vinner) |
|---|---|
| databasens DSN | `database.dsn`, `QTF_DATABASE_DSN`, **`database.dsn_file`** |
| bootstrap-lösenord | `bootstrap.admin_password`, `QTF_BOOTSTRAP_ADMIN_PASSWORD`, **`bootstrap.admin_password_file`** |
| SMTP-lösenord | `email.password_file`, ett lösenord sparat i webbklienten, **`QTF_EMAIL_PASSWORD`** |
| OIDC-klienthemlighet | `client_secret`, `client_secret_file`, en hemlighet sparad i webbklienten, **`QTF_OIDC_<NAMN>_CLIENT_SECRET`** |
| HSM-PIN | `secrets.pkcs11.pin_file`, **`QTF_SECRETS_PKCS11_PIN`** |
| lokal nyckel | `secrets.local.key_file`, **`QTF_SECRETS_KEY`** (då används ingen fil) |

En `*_file`-inställning i filen vinner alltså över miljön för DSN och bootstrap-lösenordet, medan miljön vinner för de andra hemligheterna.

## Hemligheter i filer

Skriv inga lösenord i `config.yaml`. Använd `*_file`-inställningarna: `database.dsn_file`, `auth.oidc[].client_secret_file`, `bootstrap.admin_password_file`, `email.password_file` och `secrets.pkcs11.pin_file`. Innehållet läses en gång vid start, utan inledande och avslutande blanktecken.

En **relativ** sökväg i någon av dessa inställningar slås upp i `$CREDENTIALS_DIRECTORY`, som systemd sätter för `LoadCredential=`. Hemligheterna hamnar i en privat katalog som inte växlas ut och som bara tjänsten kan läsa:

```ini
# sudo systemctl edit qtestframe
[Service]
LoadCredential=db-dsn:/etc/qtestframe/secrets/db-dsn
LoadCredential=oidc-corp:/etc/qtestframe/secrets/oidc-corp
```

```yaml
database:
  driver: postgres
  dsn_file: db-dsn             # $CREDENTIALS_DIRECTORY/db-dsn
auth:
  oidc:
    - name: corp
      client_secret_file: oidc-corp
```

Sökvägar till nycklar och certifikat (`server.tls_key`, `auth.saml[].sp_key_file`, `logging.syslog.key_file`, …) används som de står. Miljövariabler fungerar också (och är vad containrar använder), men syns i `/proc/<pid>/environ` för samma användare; använd hellre filer.

## Efter en ändring

| Ändrat | Gäller |
|---|---|
| YAML-filen eller en miljövariabel | efter `systemctl restart qtestframe` |
| TLS-certifikatet eller nyckelfilen | efter `systemctl reload qtestframe` (`SIGHUP`); kan det nya paret inte läsas används det gamla vidare och ett fel loggas |
| en inställning under **Administration → Systeminställningar** | direkt, eller efter en omstart för körningen av skript och webbtester (se [Systeminställningar under drift](system-settings.md)) |

## Referens

Alla nycklar i filen, deras typ, standardvärde och miljövariabeln som sätter dem. *Typ* `tidslängd` tar värden som `30s` eller `12h`; `lista` är en YAML-lista; `mappning` en YAML-mappning. Nycklarna för leverantörerna i `auth.oidc` och `auth.saml` skrivs med `[]`: de hör till varje post i listan.

<!-- BEGIN GENERATED: config-reference (make admin-guide-ref) -->

### `server`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `server.listen` | text | `:8080` | `QTF_SERVER_LISTEN`, `QTF_SERVER_PORT` | Adress och port som servern lyssnar på, `värd:port`; `:8080` lyssnar på alla adresser. Ersätts av `--listen`, och bara porten av `--port`/`-p` eller `QTF_SERVER_PORT`. |
| `server.tls_cert` | text | — | — | PEM-fil med servercertifikatet och dess kedja (lövcertifikatet först). Tillsammans med `tls_key` slås HTTPS på (TLS 1.2 eller senare); läses om vid `SIGHUP`. |
| `server.tls_key` | text | — | — | PEM-fil med den privata nyckeln till `tls_cert`. Båda måste anges, eller ingen. |
| `server.base_url` | text | — | `QTF_SERVER_BASE_URL` | Adressen som användarna skriver i webbläsaren, utan avslutande snedstreck (`https://qtf.example.com`). Behövs för single sign-on (callback- och SAML-adresser), för e-post (länkar; krävs när e-post är på) och för ursprungskontrollen i CSRF-skyddet. |
| `server.read_timeout` | tidslängd | `30s` | — | Längsta tid för att läsa en hel begäran, inklusive innehållet (uppladdningar, återställningar). |
| `server.write_timeout` | tidslängd | `2m` | — | Längsta tid för att skriva ett svar (nedladdningar, exporter, säkerhetskopior i webbklienten). Håll den över `runner.timeout`. |
| `server.trusted_proxies` | lista | — | — | Adresser eller CIDR-intervall för omvända proxyer. Bara för begäranden från dem används `X-Forwarded-For` som klientadress (sista posten), i loggar, granskningsloggen och inloggningsgränserna. |
| `server.hsts` | bool | `true` | — | Skicka `Strict-Transport-Security` (två år, med underdomäner) på HTTPS-anslutningar som servern själv avslutar. En proxy som avslutar TLS måste skicka den i stället. |
| `server.frame_ancestors` | lista | — | — | Ursprung (`https://doors.example.com`) för andra verktyg som får visa OSLC-dialogerna för att välja och skapa i en ram (`Content-Security-Policy: frame-ancestors`). Utan några öppnas dialogerna i ett eget fönster. Webbläsaren måste också skicka sessionskakan till ramen: båda verktygen på samma webbplats (underdomäner till samma domän). |

### `database`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `database.driver` | text | `sqlite` | `QTF_DATABASE_DRIVER` | `sqlite` (en fil, inget att installera) eller `postgres`. |
| `database.dsn` | text | `qtestframe.db` | `QTF_DATABASE_DSN` | SQLite: databasfilen (dess katalog skapas med läget 0700). PostgreSQL: en anslutnings-URL som `postgres://användare:lösen@värd:5432/db?sslmode=verify-full`. |
| `database.dsn_file` | text | — | — | Fil som innehåller DSN, så att lösenordet inte står i konfigurationen. En relativ sökväg slås upp i `$CREDENTIALS_DIRECTORY`. När den anges ersätter den `dsn` och `QTF_DATABASE_DSN`. |
| `database.max_open_conns` | heltal | `20` | — | Storleken på anslutningspoolen mot PostgreSQL. SQLite använder alltid en anslutning. |

### `logging`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `logging.level` | text | `info` | `QTF_LOG_LEVEL` | `debug`, `info`, `warn` eller `error`. |
| `logging.format` | text | `text` | `QTF_LOG_FORMAT` | `text` (nyckel=värde) eller `json`, skrivs till standard error. |
| `logging.syslog.enabled` | bool | `false` | — | Skicka också varje loggpost, även granskningsposterna, till en syslog-server. |
| `logging.syslog.network` | text | `udp` | — | `udp`, `tcp`, `tls` (RFC 5425), `unix` eller `unixgram`. |
| `logging.syslog.address` | text | — | — | `värd:port` för syslog-servern, eller sökvägen till en Unix-socket som `/dev/log`. |
| `logging.syslog.tag` | text | `qtestframe` | — | APP-NAME (RFC 5424) eller tagg (RFC 3164) i meddelandena. |
| `logging.syslog.facility` | text | `local0` | — | Syslog-facility, t.ex. `local0` eller `daemon`. |
| `logging.syslog.format` | text | `rfc5424` | — | `rfc5424` eller `rfc3164`. Meddelandet är loggposten som JSON. |
| `logging.syslog.ca_file` | text | — | — | `tls`: CA-certifikat som verifierar syslog-servern (standard: systemets rotcertifikat). |
| `logging.syslog.cert_file` | text | — | — | `tls`: klientcertifikat för ömsesidig TLS. |
| `logging.syslog.key_file` | text | — | — | `tls`: privat nyckel till `cert_file`. |

### `auth`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `auth.local` | bool | `true` | — | Tillåt inloggning med e-post och lösenord. Kan bara stängas av i webbklienten när en single sign-on-leverantör är påslagen. |
| `auth.session_ttl` | tidslängd | `12h` | — | Absolut livslängd för en session (förlängs inte vid aktivitet). |
| `auth.cookie_secure` | bool | `true` | `QTF_COOKIE_SECURE` | Markera sessionskakorna `Secure`. Måste vara `false` när servern nås över okrypterad HTTP, annars behåller webbläsarna inte sessionskakan. |
| `auth.password_min_length` | heltal | `10` | — | Minsta längd på lokala lösenord (8–72) som anges i webbklienten eller API:t. `user create` och bootstrap-administratören använder alltid 10. |
| `auth.login_max_attempts` | heltal | `10` | — | Inloggningsförsök per klientadress och e-postadress inom `login_lockout` (1–1000); fler försök får HTTP 429. |
| `auth.login_lockout` | tidslängd | `5m` | — | Tidsfönstret för `login_max_attempts` (1m–24h). |
| `auth.session_idle_timeout` | tidslängd | `0s` | — | Avslutar en webbläsarsession efter så här lång tid utan aktivitet, t.ex. `30m` (`0` = av: bara `session_ttl` gäller). En organisations läge för elektroniska poster kan ange en kortare; den kortare gäller. |
| `auth.password_min_classes` | heltal | `0` | — | Hur många av teckenklasserna gemener, versaler, siffror och symboler ett lokalt lösenord måste innehålla (2 till 4; `0` = bokstäver och minst en siffra eller symbol). |
| `auth.password_history` | heltal | `0` | — | Hur många tidigare lösenord en användare inte kan välja igen när lösenordet byts (0 till 24; `0` = av). |
| `auth.password_max_age_days` | heltal | `0` | — | Ett lokalt lösenord går ut så här många dagar efter att det sattes: nästa inloggning måste byta det (`0` = aldrig). För konton som är äldre än inställningen räknas dagarna från nästa inloggning. |
| `auth.token_max_days` | heltal | `0` | — | Längsta livslängd för en ny API-token i dagar (`0` = ingen gräns). Med en gräns behöver varje ny token ett utgångsdatum inom den; befintliga tokens behåller sina. |
| `auth.token_require_expiry` | bool | `false` | — | Neka nya API-tokens utan utgångsdatum. |

### `auth.oidc`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `auth.oidc` | lista | — | — | OpenID Connect-leverantörer, var och en som en knapp på inloggningssidan. Kan ersättas under **Administration → Systeminställningar**. |
| `auth.oidc[].name` | text | — | — | Leverantörens id i dess adresser (`/api/v1/auth/oidc/<namn>/callback`) och i `QTF_OIDC_<NAMN>_CLIENT_SECRET`. Använd gemena bokstäver, siffror och `_`. |
| `auth.oidc[].enabled` | bool | `true` | — | `false` behåller leverantören men döljer den på inloggningssidan. Standard `true`. |
| `auth.oidc[].display_name` | text | — | — | Texten på knappen på inloggningssidan. |
| `auth.oidc[].icon` | text | — | — | Ikon på inloggningsknappen: `sign-in`, `key`, `shield`, `building`, `microsoft`, `google`, `github`, `amazon` eller `apple` (tom = en allmän ikon). |
| `auth.oidc[].issuer` | text | — | — | Utfärdarens URL; servern läser `<issuer>/.well-known/openid-configuration` vid första inloggningen. Krävs. |
| `auth.oidc[].client_id` | text | — | — | Klient-id som registrerats hos leverantören. Krävs. |
| `auth.oidc[].client_secret` | text | — | `QTF_OIDC_<NAME>_CLIENT_SECRET` | Klienthemlighet skriven i filen. Använd hellre `client_secret_file` eller miljövariabeln. |
| `auth.oidc[].client_secret_file` | text | — | — | Fil med klienthemligheten (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_OIDC_<NAMN>_CLIENT_SECRET` vinner över den. |
| `auth.oidc[].scopes` | lista | `[openid, email, profile]` | — | Begärda scopes. Standard `[openid, email, profile]`; lägg till `groups` där leverantören kräver det. |
| `auth.oidc[].email_claim` | text | `email` | — | Claim med e-postadressen (standard `email`). En token med `email_verified: false` avvisas. |
| `auth.oidc[].name_claim` | text | `name` | — | Claim med visningsnamnet (standard `name`). |
| `auth.oidc[].groups_claim` | text | `groups` | — | Claim med grupperna som `role_mapping` använder (standard `groups`). |
| `auth.oidc[].local_mfa` | text | `off` | — | Qtestframes egen andra faktor efter den här leverantören: `off` (standard; leverantören ansvarar), `unless_idp_mfa` (om inte `amr`/`acr` visar att leverantören använde MFA) eller `always`. |
| `auth.oidc[].mfa_amr_values` | lista | `[mfa]` | — | `amr`-värden som räknas som MFA hos leverantören (standard `[mfa]`). |
| `auth.oidc[].mfa_acr_values` | lista | — | — | `acr`-värden som räknas som MFA hos leverantören (standard inga). |
| `auth.oidc[].auto_provision` | bool | `false` | — | Skapa okända användare vid deras första inloggning (och deras medlemskap i `default_tenant`). |
| `auth.oidc[].default_tenant` | text | — | — | Slug för organisationen vars medlemskap inloggningen skapar eller uppdaterar. |
| `auth.oidc[].default_role` | text | `viewer` | — | Roll när ingen grupp mappas (en okänd eller tom roll betyder `viewer`). |
| `auth.oidc[].role_mapping` | mappning | — | — | Grupp → roll. Den mest privilegierade mappade rollen vinner och sätts på nytt vid varje inloggning. |
| `auth.oidc[].allowed_domains` | lista | — | — | E-postdomäner som får logga in via leverantören (`example.com`; `*.example.com` för dess underdomäner). Tom = alla. Andra adresser nekas innan ett konto slås upp eller skapas. |

### `auth.saml`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `auth.saml` | lista | — | — | SAML 2.0-identitetsleverantörer (bara i filen). |
| `auth.saml[].name` | text | — | — | Id i adresserna `/api/v1/auth/saml/<namn>/{metadata,login,acs}`. |
| `auth.saml[].display_name` | text | — | — | Texten på knappen på inloggningssidan. |
| `auth.saml[].idp_metadata_url` | text | — | — | URL till IdP:ns metadata, hämtas vid första inloggningen. Den här eller `idp_metadata_file` krävs. |
| `auth.saml[].idp_metadata_file` | text | — | — | Fil med IdP:ns metadata (ingen utgående begäran behövs). |
| `auth.saml[].entity_id` | text | SP-metadataadressen | — | Qtestframes entitets-id (standard: SP-metadataadressen `<base_url>/api/v1/auth/saml/<namn>/metadata`). |
| `auth.saml[].sp_cert_file` | text | — | — | Certifikatet för SP-nyckelparet (`scripts/certs/gen-saml-sp-cert.sh`), publiceras i SP-metadata. |
| `auth.saml[].sp_key_file` | text | — | — | SP:ns privata nyckel; dekrypterar krypterade assertions. |
| `auth.saml[].email_attr` | text | `email` | — | Attribut med e-postadressen (standard `email`; annars ett NameID som innehåller `@`). |
| `auth.saml[].name_attr` | text | `displayName` | — | Attribut med visningsnamnet (standard `displayName`). |
| `auth.saml[].groups_attr` | text | `groups` | — | Attribut med grupperna (standard `groups`). |
| `auth.saml[].auto_provision` | bool | `false` | — | Som för OIDC. |
| `auth.saml[].default_tenant` | text | — | — | Som för OIDC. |
| `auth.saml[].default_role` | text | `viewer` | — | Som för OIDC. |
| `auth.saml[].role_mapping` | mappning | — | — | Som för OIDC. |
| `auth.saml[].allowed_domains` | lista | — | — | E-postdomäner som får logga in via leverantören, som för OIDC. Tom = alla. |

### `runner`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `runner.enabled` | bool | `false` | `QTF_RUNNER_ENABLED` | Tillåt att automatiska testskript körs på servern. Läs säkerhetskapitlet först. |
| `runner.timeout` | tidslängd | `1m` | — | Tidsgräns för ett skript; processgruppen avbryts därefter. |
| `runner.work_dir` | text | `$TMPDIR/qtestframe-runner` | `QTF_RUNNER_WORK_DIR` | Katalog för jobbkatalogerna (skapas med läget 0700). Håll sökvägen kort: Chromiums socket-sökväg måste vara kortare än 108 byte. |
| `runner.max_output_bytes` | heltal | `1048576` | — | Största sparade utdata (stdout och stderr tillsammans) från ett skript. |
| `runner.interpreters` | mappning | `bash: /bin/bash`, `javascript: /usr/bin/node`, `python: /usr/bin/python3`, `sh: /bin/sh` | — | Skriptspråk → tolk. Ta bort de språk ni inte behöver. |
| `runner.sandbox` | bool | `true` | — | Starta skript via `qtestframe sandbox-exec` (rlimits, no_new_privs, Landlock, seccomp). Bara i filen. |
| `runner.deny_network` | bool | `false` | — | Blockera att sandlådade skript skapar sockets (gäller inte webbtester). |

### `runner.web`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `runner.web.enabled` | bool | `false` | `QTF_RUNNER_WEB_ENABLED` | Webbtester med Playwright i en riktig webbläsare. Kräver `runner.enabled`. |
| `runner.web.node` | text | `/usr/bin/node` | `QTF_RUNNER_WEB_NODE` | Node.js-programmet. |
| `runner.web.playwright_dir` | text | — | `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | Katalog som innehåller `node_modules/@playwright/test`. Krävs (om inte `command` anges). |
| `runner.web.command` | text | — | — | Kommando som ersätter `<node> <playwright_dir>/node_modules/@playwright/test/cli.js`; `test --config <fil>` läggs till. Delas vid mellanslag. |
| `runner.web.browsers_path` | text | — | `QTF_RUNNER_WEB_BROWSERS_PATH` | `PLAYWRIGHT_BROWSERS_PATH`, där `npx playwright install` lade webbläsarna (tom: Playwrights standard). |
| `runner.web.browser` | text | `chromium` | — | Standardwebbläsare: `chromium`, `firefox` eller `webkit`. En testkonfiguration med dimensionen Browser väljer en annan. |
| `runner.web.channel` | text | — | — | Chromium-kanal, t.ex. `chrome` eller `msedge` (tom: den medföljande Chromium). |
| `runner.web.browser_channels` | bool | `false` | — | Konfigurationer som heter Chrome eller Edge använder installerade Google Chrome eller Microsoft Edge. |
| `runner.web.executable_path` | text | — | — | Chromium-program som startas i stället för Playwrights. |
| `runner.web.headless` | bool | `true` | — | Kör webbläsaren utan fönster. |
| `runner.web.timeout` | tidslängd | `1m` | — | Playwrights tidsgräns per test. |
| `runner.web.job_timeout` | tidslängd | `10m` | — | Tidsgräns för en hel Playwright-process. |
| `runner.web.screenshots` | text | `on_failure` | — | `on_failure`, `always` eller `off`. |
| `runner.web.trace` | text | `on_failure` | — | `on_failure`, `always` eller `off`. |
| `runner.web.video` | text | `off` | — | `off`, `on_failure` eller `always`. |
| `runner.web.max_parallel` | heltal | `2` | — | Bakgrundsjobb (webbtester och *Kör automatiska tester*) som körs samtidigt per serverprocess (1–32). |
| `runner.web.max_evidence_mb` | heltal | `0` (= `files.max_upload_mb`) | — | Största skärmbild, trace eller video som sparas; större filer utelämnas (0: `files.max_upload_mb`). |

### `files`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `files.max_upload_mb` | heltal | `25` | — | Största uppladdade fil i MB (1–1024). Filer lagras i databasen. |

### `requirements`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `requirements.auto_create_test` | bool | `true` | — | Skapa ett länkat testutkast med varje nytt krav (begäran kan välja bort det). |

### `hardening`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `hardening.no_new_privs` | bool | `true` | — | Sätt `PR_SET_NO_NEW_PRIVS` efter uppstart. Den enda härdning vars fel stoppar servern. |
| `hardening.landlock` | bool | `true` | — | Begränsa filåtkomst med Landlock (Linux 5.13+); hoppas över med en loggpost där det inte stöds. |
| `hardening.seccomp` | bool | `true` | — | Installera en seccomp-spärrlista med farliga systemanrop. |
| `hardening.drop_capabilities` | bool | `true` | — | Släpp alla capabilities utom `CAP_NET_BIND_SERVICE`. |
| `hardening.read_only_paths` | lista | — | — | Fler sökvägar som servern får läsa efter låsningen. |
| `hardening.read_write_paths` | lista | — | — | Fler sökvägar som servern får skriva efter låsningen (t.ex. `/run/postgresql`, en SoftHSM-tokenkatalog). |

### `bootstrap`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `bootstrap.tenant_slug` | text | `default` | — | Organisation som skapas för den första administratören (och används av `seed-demo`). |
| `bootstrap.tenant_name` | text | `Default` | — | Namnet på den organisationen. |
| `bootstrap.admin_email` | text | — | `QTF_BOOTSTRAP_ADMIN_EMAIL` | E-post för den första administratören, som skapas som superadministratör och admin i `tenant_slug` bara medan databasen saknar användare. |
| `bootstrap.admin_password` | text | — | `QTF_BOOTSTRAP_ADMIN_PASSWORD` | Dess lösenord (minst 10 tecken med en bokstav och ett tecken som inte är en bokstav). Använd hellre `admin_password_file`. |
| `bootstrap.admin_password_file` | text | — | — | Fil med lösenordet (relativ: i `$CREDENTIALS_DIRECTORY`). När den anges ersätter den `admin_password` och `QTF_BOOTSTRAP_ADMIN_PASSWORD`. |

### `email`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `email.enabled` | bool | på när `host` är satt | — | `false` behåller inställningarna men skickar ingenting. |
| `email.host` | text | — | `QTF_EMAIL_HOST` | SMTP-server. E-post är av medan den är tom. |
| `email.port` | heltal | `587` | `QTF_EMAIL_PORT` | SMTP-port: 587 för STARTTLS, 465 för implicit TLS. |
| `email.username` | text | — | `QTF_EMAIL_USERNAME` | Användarnamn för SMTP-autentisering (utelämna för ett relä utan autentisering). |
| `email.password_file` | text | — | — | Fil med SMTP-lösenordet (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_EMAIL_PASSWORD` vinner över den. |
| `email.from` | text | — | `QTF_EMAIL_FROM` | Avsändaradress, en ren adress utan namn. |
| `email.from_name` | text | `Qtestframe` | — | Avsändarnamn. |
| `email.tls` | text | `starttls` | — | `starttls`, `tls` (implicit) eller `none` (bara ett lokalt relä). |
| `email.timeout` | tidslängd | `30s` | — | Tidsgräns för att skicka ett meddelande. |
| `email.digest_hour` | heltal | `7` | — | Timme på dygnet (UTC, 0–23) då dagliga sammanfattningar skickas. |
| `email.reply_to` | text | — | — | Svarsadress (Reply-To) för e-post som saknar en egen, t.ex. en supportbrevlåda (tom = ingen). Inbjudningar och delade rapporter svarar till personen som skickade dem. |

### `system_settings`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `system_settings.locked` | lista | — | — | Inställningar som inte kan ändras under **Administration → Systeminställningar**: nycklar (`logging.level`), prefix som slutar med `.*` (`email.*`) eller `*`. Deras sparade värden ignoreras. |

### `secrets`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `secrets.provider` | text | `local` | `QTF_SECRETS_PROVIDER` | Nyckelleverantör som skyddar de lagrade hemligheterna: `local`, `tpm` eller `pkcs11`. |
| `secrets.local.key_file` | text | `qtestframe-secrets.key` bredvid en SQLite-databas, annars i `$STATE_DIRECTORY`, `/var/lib/qtestframe` eller arbetskatalogen | `QTF_SECRETS_KEY_FILE` | Nyckelfil för den lokala leverantören (256-bitarsnyckel, skapas med läget 0600 när den saknas). `QTF_SECRETS_KEY` ersätter filen. |
| `secrets.tpm.device` | text | `/dev/tpmrm0` | `QTF_SECRETS_TPM_DEVICE` | TPM:ens resurshanterare, eller Unix-socketen för en TPM-emulator (`swtpm`). |
| `secrets.tpm.pcrs` | lista | — | — | PCR:er som den förseglade nyckeln binds till, t.ex. `[7]` (Secure Boot-läget); avförseglingen misslyckas när de ändras. |
| `secrets.tpm.pcr_bank` | text | `sha256` | — | PCR-bank för policyn: `sha256` eller `sha1`. |
| `secrets.pkcs11.module` | text | — | `QTF_SECRETS_PKCS11_MODULE` | HSM:ens PKCS#11-bibliotek, t.ex. `/usr/lib/softhsm/libsofthsm2.so`. Kräver ett program byggt med `make go-pkcs11`. |
| `secrets.pkcs11.token_label` | text | — | — | Tokenens etikett; den här eller `slot` krävs. |
| `secrets.pkcs11.slot` | heltal | ingen | — | Platsnummer, används i stället för `token_label`. |
| `secrets.pkcs11.pin_file` | text | — | — | Fil med användar-PIN (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_SECRETS_PKCS11_PIN` vinner över den. |
| `secrets.pkcs11.key_label` | text | `qtestframe-kek` | — | `CKA_LABEL` för AES-nyckeln som omsluter datanycklarna; måste vara unik i tokenen. |
| `secrets.pkcs11.generate_key` | bool | `false` | — | Skapa en icke-exporterbar AES-256-nyckel med `key_label` när tokenen saknar en. |

### `similarity`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `similarity.embeddings.enabled` | bool | `false` | — | Använd en tjänst för inbäddningar (embeddings) för likheten mellan krav och tester (misstänkta dubbletter, panelen *Liknande krav*). Av som standard: likheten beräknas då på servern. **När den är på skickas rubrik, beskrivning och kriterier eller steg för objekten till tjänsten.** Finns även under **Administration → Systeminställningar**. |
| `similarity.embeddings.url` | text | — | — | Adress till en OpenAI-kompatibel embeddings-slutpunkt, t.ex. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, svaret `{data: [{embedding}]}`). |
| `similarity.embeddings.model` | text | — | — | Modell som skickas som `model` (tom: tjänstens standard). Vektorerna cachas per objektversion och modell; byts modellen beräknas de om. |
| `similarity.embeddings.api_key_file` | text | — | — | Fil med API-nyckeln, som skickas som bearer-token (relativ: i `$CREDENTIALS_DIRECTORY`). Nyckeln kan också sparas i systeminställningarna; den visas aldrig igen. |
| `similarity.embeddings.timeout` | tidslängd | `30s` | — | Längsta tid att vänta på tjänsten. När den misslyckas eller inte svarar används den lokala likheten. |

### `live`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `live.enabled` | bool | `true` | `QTF_LIVE_ENABLED` | Liveuppdateringar och närvaro: listor, översikter, notisklockan och öppna objekt uppdateras när någon annan ändrar något, och redigerare visar vem mer som tittar på eller redigerar ett objekt (en Server-Sent Events-ström, `GET /api/v1/events/stream`). Av: webbklienten frågar i stället med jämna mellanrum och konfliktkontrollen vid sparande gäller som förut. Varje användare kan också stänga av det i profilen. Bakom en omvänd proxy ska buffring av svar stängas av för strömmen (se *Liveuppdateringar* i kapitlet *Funktioner som lagts till senare*). |
| `live.heartbeat` | tidslängd | `25s` | — | Intervall för håll-vid-liv-kommentarer på en ström utan händelser. Håll det under tidsgränsen för inaktiva anslutningar i proxyer och lastbalanserare (nginx `proxy_read_timeout`, 60 s som standard). |
| `live.max_connections` | heltal | `2000` | — | Högsta antal öppna händelseströmmar i den här serverinstansen; fler anslutningar avvisas med 503 och klienterna frågar i stället med jämna mellanrum. |
| `live.max_connections_per_user` | heltal | `8` | — | Högsta antal öppna händelseströmmar för en användare (en per flik i webbläsaren; en flik i bakgrunden stänger sin ström efter en minut). |
| `live.replay_buffer` | heltal | `512` | — | Antal senaste händelser som hålls i minnet så att en klient som ansluter igen (`Last-Event-ID`) får det den missade; finns dess senaste händelse inte kvar läser den i stället om det den visar. |
| `live.presence_ttl` | tidslängd | `45s` | — | Hur länge en användare visas som att den tittar på eller redigerar ett objekt efter den senaste närvarosignalen från webbläsaren (skickas var 20:e sekund). |

### `ai`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `ai.enabled` | bool | `false` | — | Slå på den valfria [AI-assistansen](system-settings.md#ai-assistans) (utkast till tester, förbättrade krav, sammanfattningar av körningar, ramverksmappningar, acceptanskriterier). Av som standard. **När den är på skickas texten i de objekt som en användare frågar om till den konfigurerade tjänsten.** Varje organisation måste också slå på den, och projekt kan välja bort den. Finns även under **Administration → Systeminställningar**. |
| `ai.provider` | text | `anthropic` | — | Typ av tjänst: `anthropic` (Anthropics Messages API) eller `openai` (en OpenAI-kompatibel chat completions-slutpunkt: OpenAI, Azure OpenAI eller en lokal server som Ollama eller vLLM). |
| `ai.url` | text | — | — | Tjänstens bas-URL. Tom med `anthropic`: `https://api.anthropic.com`. Krävs med `openai`, t.ex. `https://llm.example.com/v1` (servern lägger till `/chat/completions`); en Azure OpenAI-deployment-URL med `?api-version=…` skickar nyckeln som `api-key`. |
| `ai.model` | text | — | — | Modell-id som skickas till tjänsten (krävs): modellen i ditt Anthropic-konto, eller modellen eller deploymenten i din OpenAI-kompatibla tjänst. Det finns ingen inbyggd standard. |
| `ai.api_key_file` | text | — | — | Fil med API-nyckeln (relativ: i `$CREDENTIALS_DIRECTORY`); skickas som `x-api-key` (Anthropic), `Authorization: Bearer` eller `api-key` (Azure). Nyckeln kan också sparas, krypterad, i systeminställningarna; den visas aldrig igen. Lokala servrar behöver kanske ingen nyckel. |
| `ai.timeout` | tidslängd | `2m` | — | Längsta tid att vänta på ett svar (5 s–10 min). Användaren kan också avbryta en begäran. |
| `ai.max_tokens` | heltal | `8192` | — | Högsta antal token i ett svar (256–128000). |
| `ai.temperature` | text | — | — | Temperatur som skickas till tjänsten (0–2). Tom: skickas inte (tjänstens standard; vissa modeller accepterar inget annat värde). |
| `ai.rate_limit` | heltal | `30` | — | Högsta antal AI-förfrågningar per användare och timme. |
| `ai.features.draft_tests` | bool | `true` | — | Erbjud *Utkast till tester* i kravredigeraren (och utkast till ett test från ett länkat krav i testredigeraren). |
| `ai.features.improve_requirement` | bool | `true` | — | Erbjud *Förbättra krav* (en omskrivning för kravkontrollerna, visad som skillnad). |
| `ai.features.summarize_run` | bool | `true` | — | Erbjud *Sammanfatta fel* på körningssidan (en sammanfattning och utkast till defekter). |
| `ai.features.suggest_mappings` | bool | `true` | — | Erbjud *Föreslå mappningar* (ATT&CK, D3FEND, EMB3D, CWE) för krav och risker, kontrollerade mot de inbyggda listorna. |
| `ai.features.suggest_criteria` | bool | `true` | — | Erbjud *Föreslå acceptanskriterier* i kravredigeraren. |
| `ai.price_input` | text | — | — | Valfritt pris per miljon indata-token (valfri valuta), för att visa en uppskattad kostnad med varje svar. |
| `ai.price_output` | text | — | — | Valfritt pris per miljon utdata-token. |

### `outbound`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `outbound.proxy` | text | — | — | HTTP(S)-proxy för serverns utgående anrop (webhooks, chatt, ärendehanterare och kravkällor, AI, embeddings, OIDC-discovery, hälsokontroller av miljöer), t.ex. `http://proxy.example.com:3128`. Tom = miljövariablerna `HTTPS_PROXY`, `HTTP_PROXY` och `NO_PROXY`. |
| `outbound.no_proxy` | lista | — | — | Värdar som nås utan `proxy` (exakta namn eller `*.example.com`). |
| `outbound.allowed_hosts` | lista | — | — | När den är satt de enda värdar som utgående anrop får nå (exakta namn, adresser eller `*.example.com`); andra nekas. Skyddar mot server-side request forgery via webhook- och ärendehanteraradresser. |
| `outbound.denied_hosts` | lista | — | — | Värdar som utgående anrop aldrig når, t.ex. `metadata.google.internal` eller `169.254.169.254`. |
| `outbound.block_private` | bool | `false` | — | Neka utgående anrop till loopback-, privata, länklokala och andra icke-publika adresser (kontrolleras på adressen som faktiskt ansluts), om inte värden finns i `allowed_hosts`. Slå på den när servern nås från internet och användarna inte ska nå ert interna nät genom den. |

### `backup`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `backup.enabled` | bool | `false` | — | Ta schemalagda säkerhetskopior: servern skriver arkivet från `qtestframe backup` till `directory` enligt `schedule`. Med flera servrar mot samma databas tar en av dem varje säkerhetskopia (en låsrad i databasen). |
| `backup.schedule` | text | `daily 02:00` | — | När säkerhetskopiorna tas, i UTC: `daily HH:MM`, `weekly DAG HH:MM` (`mon`–`sun`) eller `every TIDSLÄNGD` (`15m` till `720h`, t.ex. `every 6h`). En tidpunkt som missades medan ingen server körde tas en gång vid nästa start. |
| `backup.directory` | text | — | — | Absolut sökväg till katalogen som säkerhetskopiorna skrivs till (skapas med läget 0700, skrivbar under Landlock). Krävs för schemalagda säkerhetskopior och *Säkerhetskopiera nu*. Filerna heter `qtestframe-backup-ÅÅÅÅMMDD-TTMMSS.zip` (`.zip.enc` när de är krypterade). |
| `backup.keep_last` | heltal | `7` | — | Behåll de N nyaste säkerhetskopiorna i `directory` och ta bort äldre (`0` = ingen gräns). Bara filer som heter som schemalagda säkerhetskopior tas bort. |
| `backup.keep_days` | heltal | `0` | — | Ta bort säkerhetskopior som är äldre än N dagar (`0` = aldrig). Tillsammans med `keep_last` tas en fil bort när någon av reglerna säger det; den nyaste filen behålls alltid. |
| `backup.passphrase_file` | text | — | — | Fil med lösenfrasen som krypterar schemalagda säkerhetskopior (AES-256-GCM med en nyckel som härleds med scrypt). En relativ sökväg slås upp i `$CREDENTIALS_DIRECTORY`. Lösenfrasen kan också anges i systeminställningarna; utan den krypteras säkerhetskopiorna inte. |
| `backup.s3.enabled` | bool | `false` | — | Ladda upp varje schemalagd säkerhetskopia till S3-kompatibel objektlagring (Amazon S3, MinIO, Ceph, Garage …) när den har skrivits. En misslyckad uppladdning gör att körningen misslyckas; den lokala filen behålls. |
| `backup.s3.endpoint` | text | — | — | Bas-URL till lagringen, t.ex. `https://s3.eu-north-1.amazonaws.com` eller `https://minio.example.com:9000` (sökvägsadressering: `ENDPOINT/BUCKET/NYCKEL`). Uppladdningen följer inställningarna under `outbound`. |
| `backup.s3.region` | text | `us-east-1` | — | Region för signaturen (AWS Signature Version 4); `us-east-1` för de flesta S3-kompatibla servrar. |
| `backup.s3.bucket` | text | — | — | Bucket som säkerhetskopiorna laddas upp till. Ta bort gamla objekt med en livscykelregel för bucketen: gallringsreglerna gäller bara den lokala katalogen. |
| `backup.s3.prefix` | text | — | — | Prefix för objektnycklarna, t.ex. `qtestframe/` (tomt = högst upp i bucketen). |
| `backup.s3.access_key_id` | text | — | — | Åtkomstnyckel-ID för de inloggningsuppgifter som får göra `PutObject` i bucketen. |
| `backup.s3.secret_access_key_file` | text | — | — | Fil med den hemliga åtkomstnyckeln. En relativ sökväg slås upp i `$CREDENTIALS_DIRECTORY`. Nyckeln kan också anges i systeminställningarna (kan bara skrivas). |

<!-- END GENERATED: config-reference -->

## Miljövariabler

<!-- BEGIN GENERATED: environment (make admin-guide-ref) -->

| Variabel | Sätter |
|---|---|
| `QTF_CONFIG` | konfigurationsfilen när `--config` inte anges |
| `QTF_BOOTSTRAP_ADMIN_EMAIL` | `bootstrap.admin_email` |
| `QTF_BOOTSTRAP_ADMIN_PASSWORD` | `bootstrap.admin_password` (ersätts av `bootstrap.admin_password_file` när den är satt) |
| `QTF_COOKIE_SECURE` | `auth.cookie_secure` (`1` eller `true` = på, allt annat av) |
| `QTF_DATABASE_DRIVER` | `database.driver` |
| `QTF_DATABASE_DSN` | `database.dsn` (ersätts av `database.dsn_file` när den är satt) |
| `QTF_EMAIL_FROM` | `email.from` |
| `QTF_EMAIL_HOST` | `email.host` |
| `QTF_EMAIL_PASSWORD` | SMTP-lösenordet; vinner över `email.password_file` och ett lösenord sparat i webbklienten |
| `QTF_EMAIL_PORT` | `email.port` (ignoreras om det inte är ett tal) |
| `QTF_EMAIL_USERNAME` | `email.username` |
| `QTF_LIVE_ENABLED` | `live.enabled` (`1` eller `true` = på, allt annat = av) |
| `QTF_LOG_FORMAT` | `logging.format` |
| `QTF_LOG_LEVEL` | `logging.level` |
| `QTF_RUNNER_ENABLED` | `runner.enabled` (`1` eller `true` = på) |
| `QTF_RUNNER_WEB_BROWSERS_PATH` | `runner.web.browsers_path` |
| `QTF_RUNNER_WEB_ENABLED` | `runner.web.enabled` (`1` eller `true` = på) |
| `QTF_RUNNER_WEB_NODE` | `runner.web.node` |
| `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | `runner.web.playwright_dir` |
| `QTF_RUNNER_WORK_DIR` | `runner.work_dir` |
| `QTF_SECRETS_KEY` | själva nyckeln för den lokala leverantören (32 byte, base64 eller hex; `qtestframe secrets generate-key`); då används ingen nyckelfil |
| `QTF_SECRETS_KEY_FILE` | `secrets.local.key_file` |
| `QTF_SECRETS_PKCS11_MODULE` | `secrets.pkcs11.module` |
| `QTF_SECRETS_PKCS11_PIN` | HSM:ens användar-PIN; vinner över `secrets.pkcs11.pin_file` |
| `QTF_SECRETS_PROVIDER` | `secrets.provider` |
| `QTF_SECRETS_TPM_DEVICE` | `secrets.tpm.device` |
| `QTF_SERVER_BASE_URL` | `server.base_url` |
| `QTF_SERVER_LISTEN` | `server.listen` |
| `QTF_SERVER_PORT` | porten i `server.listen` (värden behålls) |
| `QTF_OIDC_<NAME>_CLIENT_SECRET` | klienthemligheten för OIDC-leverantören `<namn>` (versaler); vinner över `client_secret_file` och en hemlighet sparad i webbklienten |
| `CREDENTIALS_DIRECTORY` | sätts av systemd (`LoadCredential=`): relativa `*_file`-sökvägar slås upp där, och den är läsbar efter låsningen |
| `STATE_DIRECTORY` | sätts av systemd (`StateDirectory=`): standardplats för nyckelfilen när databasen inte är SQLite |
| `QTF_BACKUP_PASSPHRASE` | lösenfrasen för `qtestframe backup --encrypt`, `backup verify` och `restore` när `--passphrase-file` inte anges (servern läser `backup.passphrase` i stället) |
| `QTF_URL`, `QTF_TOKEN`, `QTF_CONFIGURATION` | standardvärden för `--url`, `--token` och `--configuration` i `qtestframe report` |

<!-- END GENERATED: environment -->

Booleska variabler är på för `1` eller `true` (oavsett skiftläge) och av för alla andra värden. `QTF_SERVER_PORT` ändrar bara porten när `server.listen` är en giltig `värd:port`.
