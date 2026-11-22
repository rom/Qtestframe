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

### `auth.oidc`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `auth.oidc` | lista | — | — | OpenID Connect-leverantörer, var och en som en knapp på inloggningssidan. Kan ersättas under **Administration → Systeminställningar**. |
| `auth.oidc[].name` | text | — | — | Leverantörens id i dess adresser (`/api/v1/auth/oidc/<namn>/callback`) och i `QTF_OIDC_<NAMN>_CLIENT_SECRET`. Använd gemena bokstäver, siffror och `_`. |
| `auth.oidc[].enabled` | bool | `true` | — | `false` behåller leverantören men döljer den på inloggningssidan. Standard `true`. |
| `auth.oidc[].display_name` | text | — | — | Texten på knappen på inloggningssidan. |
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
| `QTF_URL`, `QTF_TOKEN`, `QTF_CONFIGURATION` | standardvärden för `--url`, `--token` och `--configuration` i `qtestframe report` |

<!-- END GENERATED: environment -->

Booleska variabler är på för `1` eller `true` (oavsett skiftläge) och av för alla andra värden. `QTF_SERVER_PORT` ändrar bara porten när `server.listen` är en giltig `värd:port`.
