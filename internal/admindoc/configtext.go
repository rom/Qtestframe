package admindoc

// The descriptions of the configuration reference, in English and Swedish.
// TestEveryKeyDescribed fails when a key of internal/config has no
// description here (or a description names a key that no longer exists),
// so a new setting cannot be added without documenting it.

// text is one text in the languages of the guide.
type text struct{ en, sv string }

func (t text) in(lang string) string {
	if lang == "sv" {
		return t.sv
	}
	return t.en
}

// keyDocs describes every key of the configuration file.
var keyDocs = map[string]text{
	"server.listen": {
		"Address and port of the HTTP(S) listener, `host:port`; `:8080` listens on every address. Replaced by `--listen`, and the port alone by `--port`/`-p` or `QTF_SERVER_PORT`.",
		"Adress och port som servern lyssnar på, `värd:port`; `:8080` lyssnar på alla adresser. Ersätts av `--listen`, och bara porten av `--port`/`-p` eller `QTF_SERVER_PORT`.",
	},
	"server.tls_cert": {
		"PEM file with the server certificate and its chain (leaf first). With `tls_key` it turns on HTTPS (TLS 1.2 or later); re-read on `SIGHUP`.",
		"PEM-fil med servercertifikatet och dess kedja (lövcertifikatet först). Tillsammans med `tls_key` slås HTTPS på (TLS 1.2 eller senare); läses om vid `SIGHUP`.",
	},
	"server.tls_key": {
		"PEM file with the private key of `tls_cert`. Both must be set, or neither.",
		"PEM-fil med den privata nyckeln till `tls_cert`. Båda måste anges, eller ingen.",
	},
	"server.base_url": {
		"The URL users type in the browser, without a trailing slash (`https://qtf.example.com`). Needed for single sign-on (callback and SAML entity URLs), for e-mail (links; required when e-mail is on) and for the origin check of the CSRF protection.",
		"Adressen som användarna skriver i webbläsaren, utan avslutande snedstreck (`https://qtf.example.com`). Behövs för single sign-on (callback- och SAML-adresser), för e-post (länkar; krävs när e-post är på) och för ursprungskontrollen i CSRF-skyddet.",
	},
	"server.read_timeout": {
		"Longest time to read a whole request, body included (uploads, restores).",
		"Längsta tid för att läsa en hel begäran, inklusive innehållet (uppladdningar, återställningar).",
	},
	"server.write_timeout": {
		"Longest time to write a response (downloads, exports, backups in the web client). Keep it above `runner.timeout`.",
		"Längsta tid för att skriva ett svar (nedladdningar, exporter, säkerhetskopior i webbklienten). Håll den över `runner.timeout`.",
	},
	"server.trusted_proxies": {
		"Addresses or CIDR ranges of reverse proxies. Only for requests from them is `X-Forwarded-For` used as the client address (its last entry), for logs, the audit trail and the sign-in limits.",
		"Adresser eller CIDR-intervall för omvända proxyer. Bara för begäranden från dem används `X-Forwarded-For` som klientadress (sista posten), i loggar, granskningsloggen och inloggningsgränserna.",
	},
	"server.hsts": {
		"Send `Strict-Transport-Security` (two years, subdomains included) on HTTPS connections the server terminates itself. A TLS-terminating proxy must send it instead.",
		"Skicka `Strict-Transport-Security` (två år, med underdomäner) på HTTPS-anslutningar som servern själv avslutar. En proxy som avslutar TLS måste skicka den i stället.",
	},
	"server.frame_ancestors": {
		"Origins (`https://doors.example.com`) of other tools that may show the OSLC selection and creation dialogs in a frame (`Content-Security-Policy: frame-ancestors`). Without any the dialogs open in a window of their own. The browser must also send the session cookie to the frame: both tools on the same site (subdomains of one domain).",
		"Ursprung (`https://doors.example.com`) för andra verktyg som får visa OSLC-dialogerna för att välja och skapa i en ram (`Content-Security-Policy: frame-ancestors`). Utan några öppnas dialogerna i ett eget fönster. Webbläsaren måste också skicka sessionskakan till ramen: båda verktygen på samma webbplats (underdomäner till samma domän).",
	},
	"database.driver": {
		"`sqlite` (a file, nothing to install) or `postgres`.",
		"`sqlite` (en fil, inget att installera) eller `postgres`.",
	},
	"database.dsn": {
		"SQLite: the database file (its directory is created with mode 0700). PostgreSQL: a connection URL such as `postgres://user:pass@host:5432/db?sslmode=verify-full`.",
		"SQLite: databasfilen (dess katalog skapas med läget 0700). PostgreSQL: en anslutnings-URL som `postgres://användare:lösen@värd:5432/db?sslmode=verify-full`.",
	},
	"database.dsn_file": {
		"File that holds the DSN, so that the password is not in the configuration. A relative path is looked up in `$CREDENTIALS_DIRECTORY`. When set, it replaces `dsn` and `QTF_DATABASE_DSN`.",
		"Fil som innehåller DSN, så att lösenordet inte står i konfigurationen. En relativ sökväg slås upp i `$CREDENTIALS_DIRECTORY`. När den anges ersätter den `dsn` och `QTF_DATABASE_DSN`.",
	},
	"database.max_open_conns": {
		"Size of the PostgreSQL connection pool. SQLite always uses one connection.",
		"Storleken på anslutningspoolen mot PostgreSQL. SQLite använder alltid en anslutning.",
	},
	"logging.level": {
		"`debug`, `info`, `warn` or `error`.",
		"`debug`, `info`, `warn` eller `error`.",
	},
	"logging.format": {
		"`text` (key=value) or `json`, written to standard error.",
		"`text` (nyckel=värde) eller `json`, skrivs till standard error.",
	},
	"logging.syslog.enabled": {
		"Also send every log record, the audit records included, to a syslog server.",
		"Skicka också varje loggpost, även granskningsposterna, till en syslog-server.",
	},
	"logging.syslog.network": {
		"`udp`, `tcp`, `tls` (RFC 5425), `unix` or `unixgram`.",
		"`udp`, `tcp`, `tls` (RFC 5425), `unix` eller `unixgram`.",
	},
	"logging.syslog.address": {
		"`host:port` of the syslog server, or the path of a Unix socket such as `/dev/log`.",
		"`värd:port` för syslog-servern, eller sökvägen till en Unix-socket som `/dev/log`.",
	},
	"logging.syslog.tag": {
		"APP-NAME (RFC 5424) or tag (RFC 3164) of the messages.",
		"APP-NAME (RFC 5424) eller tagg (RFC 3164) i meddelandena.",
	},
	"logging.syslog.facility": {
		"Syslog facility, e.g. `local0` or `daemon`.",
		"Syslog-facility, t.ex. `local0` eller `daemon`.",
	},
	"logging.syslog.format": {
		"`rfc5424` or `rfc3164`. The message is the log record as JSON.",
		"`rfc5424` eller `rfc3164`. Meddelandet är loggposten som JSON.",
	},
	"logging.syslog.ca_file": {
		"`tls`: CA certificates that verify the syslog server (default: the system roots).",
		"`tls`: CA-certifikat som verifierar syslog-servern (standard: systemets rotcertifikat).",
	},
	"logging.syslog.cert_file": {
		"`tls`: client certificate for mutual TLS.",
		"`tls`: klientcertifikat för ömsesidig TLS.",
	},
	"logging.syslog.key_file": {
		"`tls`: private key of `cert_file`.",
		"`tls`: privat nyckel till `cert_file`.",
	},
	"auth.local": {
		"Allow sign-in with e-mail and password. Can only be turned off in the web client while a single sign-on provider is enabled.",
		"Tillåt inloggning med e-post och lösenord. Kan bara stängas av i webbklienten när en single sign-on-leverantör är påslagen.",
	},
	"auth.session_ttl": {
		"Absolute lifetime of a session (no sliding renewal).",
		"Absolut livslängd för en session (förlängs inte vid aktivitet).",
	},
	"auth.cookie_secure": {
		"Mark the session cookies `Secure`. Must be `false` when the server is reached over plain HTTP, else browsers do not keep the session cookie.",
		"Markera sessionskakorna `Secure`. Måste vara `false` när servern nås över okrypterad HTTP, annars behåller webbläsarna inte sessionskakan.",
	},
	"auth.password_min_length": {
		"Minimum length of local passwords (8–72) set in the web client or the API. `user create` and the bootstrap administrator always use 10.",
		"Minsta längd på lokala lösenord (8–72) som anges i webbklienten eller API:t. `user create` och bootstrap-administratören använder alltid 10.",
	},
	"auth.login_max_attempts": {
		"Sign-in attempts per client address and e-mail address within `login_lockout` (1–1000); further attempts get HTTP 429.",
		"Inloggningsförsök per klientadress och e-postadress inom `login_lockout` (1–1000); fler försök får HTTP 429.",
	},
	"auth.login_lockout": {
		"Window of `login_max_attempts` (1m–24h).",
		"Tidsfönstret för `login_max_attempts` (1m–24h).",
	},
	"auth.oidc": {
		"OpenID Connect providers, each shown as a button on the sign-in page. Can be replaced in **Administration → System settings**.",
		"OpenID Connect-leverantörer, var och en som en knapp på inloggningssidan. Kan ersättas under **Administration → Systeminställningar**.",
	},
	"auth.oidc[].name": {
		"Identifier of the provider in its URLs (`/api/v1/auth/oidc/<name>/callback`) and in `QTF_OIDC_<NAME>_CLIENT_SECRET`. Use lower-case letters, digits and `_`.",
		"Leverantörens id i dess adresser (`/api/v1/auth/oidc/<namn>/callback`) och i `QTF_OIDC_<NAMN>_CLIENT_SECRET`. Använd gemena bokstäver, siffror och `_`.",
	},
	"auth.oidc[].enabled": {
		"`false` keeps the provider configured but hides it from the sign-in page. Default `true`.",
		"`false` behåller leverantören men döljer den på inloggningssidan. Standard `true`.",
	},
	"auth.oidc[].display_name": {
		"Label of the button on the sign-in page.",
		"Texten på knappen på inloggningssidan.",
	},
	"auth.oidc[].issuer": {
		"Issuer URL; the server reads `<issuer>/.well-known/openid-configuration` at the first sign-in. Required.",
		"Utfärdarens URL; servern läser `<issuer>/.well-known/openid-configuration` vid första inloggningen. Krävs.",
	},
	"auth.oidc[].client_id": {
		"Client ID registered at the provider. Required.",
		"Klient-id som registrerats hos leverantören. Krävs.",
	},
	"auth.oidc[].client_secret": {
		"Client secret written in the file. Prefer `client_secret_file` or the environment variable.",
		"Klienthemlighet skriven i filen. Använd hellre `client_secret_file` eller miljövariabeln.",
	},
	"auth.oidc[].client_secret_file": {
		"File with the client secret (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_OIDC_<NAME>_CLIENT_SECRET` wins over it.",
		"Fil med klienthemligheten (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_OIDC_<NAMN>_CLIENT_SECRET` vinner över den.",
	},
	"auth.oidc[].scopes": {
		"Requested scopes. Default `[openid, email, profile]`; add `groups` where the provider needs it.",
		"Begärda scopes. Standard `[openid, email, profile]`; lägg till `groups` där leverantören kräver det.",
	},
	"auth.oidc[].email_claim": {
		"Claim with the e-mail address (default `email`). A token with `email_verified: false` is refused.",
		"Claim med e-postadressen (standard `email`). En token med `email_verified: false` avvisas.",
	},
	"auth.oidc[].name_claim": {
		"Claim with the display name (default `name`).",
		"Claim med visningsnamnet (standard `name`).",
	},
	"auth.oidc[].groups_claim": {
		"Claim with the groups used by `role_mapping` (default `groups`).",
		"Claim med grupperna som `role_mapping` använder (standard `groups`).",
	},
	"auth.oidc[].local_mfa": {
		"Qtestframe's own second factor after this provider: `off` (default; the provider is responsible), `unless_idp_mfa` (unless `amr`/`acr` show that the provider used MFA) or `always`.",
		"Qtestframes egen andra faktor efter den här leverantören: `off` (standard; leverantören ansvarar), `unless_idp_mfa` (om inte `amr`/`acr` visar att leverantören använde MFA) eller `always`.",
	},
	"auth.oidc[].mfa_amr_values": {
		"`amr` values that count as MFA by the provider (default `[mfa]`).",
		"`amr`-värden som räknas som MFA hos leverantören (standard `[mfa]`).",
	},
	"auth.oidc[].mfa_acr_values": {
		"`acr` values that count as MFA by the provider (default none).",
		"`acr`-värden som räknas som MFA hos leverantören (standard inga).",
	},
	"auth.oidc[].auto_provision": {
		"Create unknown users at their first sign-in (and their membership in `default_tenant`).",
		"Skapa okända användare vid deras första inloggning (och deras medlemskap i `default_tenant`).",
	},
	"auth.oidc[].default_tenant": {
		"Slug of the organisation whose membership the sign-in creates or updates.",
		"Slug för organisationen vars medlemskap inloggningen skapar eller uppdaterar.",
	},
	"auth.oidc[].default_role": {
		"Role when no group is mapped (an unknown or empty role means `viewer`).",
		"Roll när ingen grupp mappas (en okänd eller tom roll betyder `viewer`).",
	},
	"auth.oidc[].role_mapping": {
		"Group → role. The most privileged mapped role wins and is set again at every sign-in.",
		"Grupp → roll. Den mest privilegierade mappade rollen vinner och sätts på nytt vid varje inloggning.",
	},
	"auth.saml": {
		"SAML 2.0 identity providers (only in the file).",
		"SAML 2.0-identitetsleverantörer (bara i filen).",
	},
	"auth.saml[].name": {
		"Identifier in the URLs `/api/v1/auth/saml/<name>/{metadata,login,acs}`.",
		"Id i adresserna `/api/v1/auth/saml/<namn>/{metadata,login,acs}`.",
	},
	"auth.saml[].display_name": {
		"Label of the button on the sign-in page.",
		"Texten på knappen på inloggningssidan.",
	},
	"auth.saml[].idp_metadata_url": {
		"URL of the IdP metadata, fetched at the first sign-in. This or `idp_metadata_file` is required.",
		"URL till IdP:ns metadata, hämtas vid första inloggningen. Den här eller `idp_metadata_file` krävs.",
	},
	"auth.saml[].idp_metadata_file": {
		"File with the IdP metadata (no outgoing request needed).",
		"Fil med IdP:ns metadata (ingen utgående begäran behövs).",
	},
	"auth.saml[].entity_id": {
		"Entity ID of Qtestframe (default: the SP metadata URL `<base_url>/api/v1/auth/saml/<name>/metadata`).",
		"Qtestframes entitets-id (standard: SP-metadataadressen `<base_url>/api/v1/auth/saml/<namn>/metadata`).",
	},
	"auth.saml[].sp_cert_file": {
		"Certificate of the SP key pair (`scripts/certs/gen-saml-sp-cert.sh`), published in the SP metadata.",
		"Certifikatet för SP-nyckelparet (`scripts/certs/gen-saml-sp-cert.sh`), publiceras i SP-metadata.",
	},
	"auth.saml[].sp_key_file": {
		"Private key of the SP; decrypts encrypted assertions.",
		"SP:ns privata nyckel; dekrypterar krypterade assertions.",
	},
	"auth.saml[].email_attr": {
		"Attribute with the e-mail address (default `email`; else a NameID that contains `@`).",
		"Attribut med e-postadressen (standard `email`; annars ett NameID som innehåller `@`).",
	},
	"auth.saml[].name_attr": {
		"Attribute with the display name (default `displayName`).",
		"Attribut med visningsnamnet (standard `displayName`).",
	},
	"auth.saml[].groups_attr": {
		"Attribute with the groups (default `groups`).",
		"Attribut med grupperna (standard `groups`).",
	},
	"auth.saml[].auto_provision": {
		"As for OIDC.",
		"Som för OIDC.",
	},
	"auth.saml[].default_tenant": {
		"As for OIDC.",
		"Som för OIDC.",
	},
	"auth.saml[].default_role": {
		"As for OIDC.",
		"Som för OIDC.",
	},
	"auth.saml[].role_mapping": {
		"As for OIDC.",
		"Som för OIDC.",
	},
	"runner.enabled": {
		"Allow running automated test scripts on the server. Read the security chapter first.",
		"Tillåt att automatiska testskript körs på servern. Läs säkerhetskapitlet först.",
	},
	"runner.timeout": {
		"Wall-clock limit of one script; the process group is then killed.",
		"Tidsgräns för ett skript; processgruppen avbryts därefter.",
	},
	"runner.work_dir": {
		"Directory of the job directories (created with mode 0700). Keep the path short: Chromium's socket path must stay below 108 bytes.",
		"Katalog för jobbkatalogerna (skapas med läget 0700). Håll sökvägen kort: Chromiums socket-sökväg måste vara kortare än 108 byte.",
	},
	"runner.max_output_bytes": {
		"Maximum stored output (stdout and stderr together) of one script.",
		"Största sparade utdata (stdout och stderr tillsammans) från ett skript.",
	},
	"runner.interpreters": {
		"Script language → interpreter. Remove the languages you do not need.",
		"Skriptspråk → tolk. Ta bort de språk ni inte behöver.",
	},
	"runner.sandbox": {
		"Start scripts through `qtestframe sandbox-exec` (rlimits, no_new_privs, Landlock, seccomp). Only in the file.",
		"Starta skript via `qtestframe sandbox-exec` (rlimits, no_new_privs, Landlock, seccomp). Bara i filen.",
	},
	"runner.deny_network": {
		"Block socket creation for sandboxed scripts (not for web tests).",
		"Blockera att sandlådade skript skapar sockets (gäller inte webbtester).",
	},
	"runner.web.enabled": {
		"Web tests with Playwright in a real browser. Needs `runner.enabled`.",
		"Webbtester med Playwright i en riktig webbläsare. Kräver `runner.enabled`.",
	},
	"runner.web.node": {
		"Node.js binary.",
		"Node.js-programmet.",
	},
	"runner.web.playwright_dir": {
		"Directory that contains `node_modules/@playwright/test`. Required (unless `command` is set).",
		"Katalog som innehåller `node_modules/@playwright/test`. Krävs (om inte `command` anges).",
	},
	"runner.web.command": {
		"Command that replaces `<node> <playwright_dir>/node_modules/@playwright/test/cli.js`; `test --config <file>` is appended. Split at spaces.",
		"Kommando som ersätter `<node> <playwright_dir>/node_modules/@playwright/test/cli.js`; `test --config <fil>` läggs till. Delas vid mellanslag.",
	},
	"runner.web.browsers_path": {
		"`PLAYWRIGHT_BROWSERS_PATH`, where `npx playwright install` put the browsers (empty: Playwright's default).",
		"`PLAYWRIGHT_BROWSERS_PATH`, där `npx playwright install` lade webbläsarna (tom: Playwrights standard).",
	},
	"runner.web.browser": {
		"Default browser: `chromium`, `firefox` or `webkit`. A test configuration with a Browser dimension picks another.",
		"Standardwebbläsare: `chromium`, `firefox` eller `webkit`. En testkonfiguration med dimensionen Browser väljer en annan.",
	},
	"runner.web.channel": {
		"Chromium channel, e.g. `chrome` or `msedge` (empty: the bundled Chromium).",
		"Chromium-kanal, t.ex. `chrome` eller `msedge` (tom: den medföljande Chromium).",
	},
	"runner.web.browser_channels": {
		"Configurations named Chrome or Edge use the installed Google Chrome or Microsoft Edge.",
		"Konfigurationer som heter Chrome eller Edge använder installerade Google Chrome eller Microsoft Edge.",
	},
	"runner.web.executable_path": {
		"Chromium executable to launch instead of Playwright's.",
		"Chromium-program som startas i stället för Playwrights.",
	},
	"runner.web.headless": {
		"Run the browser without a window.",
		"Kör webbläsaren utan fönster.",
	},
	"runner.web.timeout": {
		"Playwright's per-test timeout.",
		"Playwrights tidsgräns per test.",
	},
	"runner.web.job_timeout": {
		"Limit of a whole Playwright process.",
		"Tidsgräns för en hel Playwright-process.",
	},
	"runner.web.screenshots": {
		"`on_failure`, `always` or `off`.",
		"`on_failure`, `always` eller `off`.",
	},
	"runner.web.trace": {
		"`on_failure`, `always` or `off`.",
		"`on_failure`, `always` eller `off`.",
	},
	"runner.web.video": {
		"`off`, `on_failure` or `always`.",
		"`off`, `on_failure` eller `always`.",
	},
	"runner.web.max_parallel": {
		"Background jobs (web tests and *Run automated tests*) running at the same time per server process (1–32).",
		"Bakgrundsjobb (webbtester och *Kör automatiska tester*) som körs samtidigt per serverprocess (1–32).",
	},
	"runner.web.max_evidence_mb": {
		"Largest screenshot, trace or video kept; larger files are left out (0: `files.max_upload_mb`).",
		"Största skärmbild, trace eller video som sparas; större filer utelämnas (0: `files.max_upload_mb`).",
	},
	"files.max_upload_mb": {
		"Largest uploaded file in MB (1–1024). Files are stored in the database.",
		"Största uppladdade fil i MB (1–1024). Filer lagras i databasen.",
	},
	"requirements.auto_create_test": {
		"Create a linked draft test with every new requirement (the request can opt out).",
		"Skapa ett länkat testutkast med varje nytt krav (begäran kan välja bort det).",
	},
	"hardening.no_new_privs": {
		"Set `PR_SET_NO_NEW_PRIVS` after start-up. The only hardening whose failure stops the server.",
		"Sätt `PR_SET_NO_NEW_PRIVS` efter uppstart. Den enda härdning vars fel stoppar servern.",
	},
	"hardening.landlock": {
		"Restrict file access with Landlock (Linux 5.13+); skipped with a log entry where unsupported.",
		"Begränsa filåtkomst med Landlock (Linux 5.13+); hoppas över med en loggpost där det inte stöds.",
	},
	"hardening.seccomp": {
		"Install a seccomp deny-list of dangerous system calls.",
		"Installera en seccomp-spärrlista med farliga systemanrop.",
	},
	"hardening.drop_capabilities": {
		"Drop every capability except `CAP_NET_BIND_SERVICE`.",
		"Släpp alla capabilities utom `CAP_NET_BIND_SERVICE`.",
	},
	"hardening.read_only_paths": {
		"Further paths the server may read after the lock-down.",
		"Fler sökvägar som servern får läsa efter låsningen.",
	},
	"hardening.read_write_paths": {
		"Further paths the server may write after the lock-down (e.g. `/run/postgresql`, a SoftHSM token directory).",
		"Fler sökvägar som servern får skriva efter låsningen (t.ex. `/run/postgresql`, en SoftHSM-tokenkatalog).",
	},
	"bootstrap.tenant_slug": {
		"Organisation created for the first administrator (and used by `seed-demo`).",
		"Organisation som skapas för den första administratören (och används av `seed-demo`).",
	},
	"bootstrap.tenant_name": {
		"Name of that organisation.",
		"Namnet på den organisationen.",
	},
	"bootstrap.admin_email": {
		"E-mail of the first administrator, created as super admin and admin of `tenant_slug` only while the database has no users.",
		"E-post för den första administratören, som skapas som superadministratör och admin i `tenant_slug` bara medan databasen saknar användare.",
	},
	"bootstrap.admin_password": {
		"Its password (at least 10 characters with a letter and a non-letter). Prefer `admin_password_file`.",
		"Dess lösenord (minst 10 tecken med en bokstav och ett tecken som inte är en bokstav). Använd hellre `admin_password_file`.",
	},
	"bootstrap.admin_password_file": {
		"File with that password (relative: in `$CREDENTIALS_DIRECTORY`). When set, it replaces `admin_password` and `QTF_BOOTSTRAP_ADMIN_PASSWORD`.",
		"Fil med lösenordet (relativ: i `$CREDENTIALS_DIRECTORY`). När den anges ersätter den `admin_password` och `QTF_BOOTSTRAP_ADMIN_PASSWORD`.",
	},
	"email.enabled": {
		"`false` keeps the settings but sends nothing.",
		"`false` behåller inställningarna men skickar ingenting.",
	},
	"email.host": {
		"SMTP server. E-mail is off while it is empty.",
		"SMTP-server. E-post är av medan den är tom.",
	},
	"email.port": {
		"SMTP port: 587 for STARTTLS, 465 for implicit TLS.",
		"SMTP-port: 587 för STARTTLS, 465 för implicit TLS.",
	},
	"email.username": {
		"User name for SMTP authentication (leave out for a relay without authentication).",
		"Användarnamn för SMTP-autentisering (utelämna för ett relä utan autentisering).",
	},
	"email.password_file": {
		"File with the SMTP password (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_EMAIL_PASSWORD` wins over it.",
		"Fil med SMTP-lösenordet (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_EMAIL_PASSWORD` vinner över den.",
	},
	"email.from": {
		"Sender address, a plain address without a name.",
		"Avsändaradress, en ren adress utan namn.",
	},
	"email.from_name": {
		"Sender name.",
		"Avsändarnamn.",
	},
	"email.tls": {
		"`starttls`, `tls` (implicit) or `none` (a local relay only).",
		"`starttls`, `tls` (implicit) eller `none` (bara ett lokalt relä).",
	},
	"email.timeout": {
		"Limit for sending one message.",
		"Tidsgräns för att skicka ett meddelande.",
	},
	"email.digest_hour": {
		"Hour of the day (UTC, 0–23) when daily digests are sent.",
		"Timme på dygnet (UTC, 0–23) då dagliga sammanfattningar skickas.",
	},
	"system_settings.locked": {
		"Settings that cannot be changed in **Administration → System settings**: keys (`logging.level`), prefixes ending in `.*` (`email.*`) or `*`. Their stored overrides are ignored.",
		"Inställningar som inte kan ändras under **Administration → Systeminställningar**: nycklar (`logging.level`), prefix som slutar med `.*` (`email.*`) eller `*`. Deras sparade värden ignoreras.",
	},
	"secrets.provider": {
		"Key provider that protects the stored secrets: `local`, `tpm` or `pkcs11`.",
		"Nyckelleverantör som skyddar de lagrade hemligheterna: `local`, `tpm` eller `pkcs11`.",
	},
	"secrets.local.key_file": {
		"Key file of the local provider (256-bit key, created with mode 0600 when missing). `QTF_SECRETS_KEY` replaces the file.",
		"Nyckelfil för den lokala leverantören (256-bitarsnyckel, skapas med läget 0600 när den saknas). `QTF_SECRETS_KEY` ersätter filen.",
	},
	"secrets.tpm.device": {
		"TPM resource manager, or the Unix socket of a TPM emulator (`swtpm`).",
		"TPM:ens resurshanterare, eller Unix-socketen för en TPM-emulator (`swtpm`).",
	},
	"secrets.tpm.pcrs": {
		"PCRs the sealed key is bound to, e.g. `[7]` (Secure Boot state); unsealing fails after they change.",
		"PCR:er som den förseglade nyckeln binds till, t.ex. `[7]` (Secure Boot-läget); avförseglingen misslyckas när de ändras.",
	},
	"secrets.tpm.pcr_bank": {
		"PCR bank of the policy: `sha256` or `sha1`.",
		"PCR-bank för policyn: `sha256` eller `sha1`.",
	},
	"secrets.pkcs11.module": {
		"PKCS#11 library of the HSM, e.g. `/usr/lib/softhsm/libsofthsm2.so`. Needs a binary built with `make go-pkcs11`.",
		"HSM:ens PKCS#11-bibliotek, t.ex. `/usr/lib/softhsm/libsofthsm2.so`. Kräver ett program byggt med `make go-pkcs11`.",
	},
	"secrets.pkcs11.token_label": {
		"Label of the token; this or `slot` is required.",
		"Tokenens etikett; den här eller `slot` krävs.",
	},
	"secrets.pkcs11.slot": {
		"Slot number, used instead of `token_label`.",
		"Platsnummer, används i stället för `token_label`.",
	},
	"secrets.pkcs11.pin_file": {
		"File with the user PIN (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_SECRETS_PKCS11_PIN` wins over it.",
		"Fil med användar-PIN (relativ: i `$CREDENTIALS_DIRECTORY`). `QTF_SECRETS_PKCS11_PIN` vinner över den.",
	},
	"secrets.pkcs11.key_label": {
		"`CKA_LABEL` of the AES key that wraps the data keys; must be unique in the token.",
		"`CKA_LABEL` för AES-nyckeln som omsluter datanycklarna; måste vara unik i tokenen.",
	},
	"secrets.pkcs11.generate_key": {
		"Create a non-extractable AES-256 key with `key_label` when the token has none.",
		"Skapa en icke-exporterbar AES-256-nyckel med `key_label` när tokenen saknar en.",
	},
}

// defaultDocs replace the default shown for keys whose default is not a
// plain value of config.Default().
var defaultDocs = map[string]text{
	"runner.work_dir":            {"`$TMPDIR/qtestframe-runner`", "`$TMPDIR/qtestframe-runner`"},
	"email.enabled":              {"on while `host` is set", "på när `host` är satt"},
	"secrets.local.key_file":     {"`qtestframe-secrets.key` next to a SQLite database, else in `$STATE_DIRECTORY`, `/var/lib/qtestframe` or the working directory", "`qtestframe-secrets.key` bredvid en SQLite-databas, annars i `$STATE_DIRECTORY`, `/var/lib/qtestframe` eller arbetskatalogen"},
	"secrets.pkcs11.slot":        {"none", "ingen"},
	"runner.web.max_evidence_mb": {"`0` (= `files.max_upload_mb`)", "`0` (= `files.max_upload_mb`)"},
	"auth.oidc[].enabled":        {"`true`", "`true`"},
	"auth.oidc[].scopes":         {"`[openid, email, profile]`", "`[openid, email, profile]`"},
	"auth.oidc[].email_claim":    {"`email`", "`email`"},
	"auth.oidc[].name_claim":     {"`name`", "`name`"},
	"auth.oidc[].groups_claim":   {"`groups`", "`groups`"},
	"auth.oidc[].local_mfa":      {"`off`", "`off`"},
	"auth.oidc[].mfa_amr_values": {"`[mfa]`", "`[mfa]`"},
	"auth.oidc[].default_role":   {"`viewer`", "`viewer`"},
	"auth.saml[].email_attr":     {"`email`", "`email`"},
	"auth.saml[].name_attr":      {"`displayName`", "`displayName`"},
	"auth.saml[].groups_attr":    {"`groups`", "`groups`"},
	"auth.saml[].default_role":   {"`viewer`", "`viewer`"},
	"auth.saml[].entity_id":      {"the SP metadata URL", "SP-metadataadressen"},
	"auth.oidc[].auto_provision": {"`false`", "`false`"},
	"auth.saml[].auto_provision": {"`false`", "`false`"},
}

// envDocs describes the environment variables of config.EnvVars and the
// others the server reads.
var envDocs = map[string]text{
	"QTF_DATABASE_DRIVER":           {"`database.driver`", "`database.driver`"},
	"QTF_DATABASE_DSN":              {"`database.dsn` (replaced by `database.dsn_file` when that is set)", "`database.dsn` (ersätts av `database.dsn_file` när den är satt)"},
	"QTF_SERVER_LISTEN":             {"`server.listen`", "`server.listen`"},
	"QTF_SERVER_PORT":               {"the port of `server.listen` (the host stays)", "porten i `server.listen` (värden behålls)"},
	"QTF_SERVER_BASE_URL":           {"`server.base_url`", "`server.base_url`"},
	"QTF_LOG_LEVEL":                 {"`logging.level`", "`logging.level`"},
	"QTF_LOG_FORMAT":                {"`logging.format`", "`logging.format`"},
	"QTF_BOOTSTRAP_ADMIN_EMAIL":     {"`bootstrap.admin_email`", "`bootstrap.admin_email`"},
	"QTF_BOOTSTRAP_ADMIN_PASSWORD":  {"`bootstrap.admin_password` (replaced by `bootstrap.admin_password_file` when that is set)", "`bootstrap.admin_password` (ersätts av `bootstrap.admin_password_file` när den är satt)"},
	"QTF_EMAIL_HOST":                {"`email.host`", "`email.host`"},
	"QTF_EMAIL_USERNAME":            {"`email.username`", "`email.username`"},
	"QTF_EMAIL_FROM":                {"`email.from`", "`email.from`"},
	"QTF_EMAIL_PORT":                {"`email.port` (ignored when not a number)", "`email.port` (ignoreras om det inte är ett tal)"},
	"QTF_EMAIL_PASSWORD":            {"the SMTP password; wins over `email.password_file` and a password stored in the web client", "SMTP-lösenordet; vinner över `email.password_file` och ett lösenord sparat i webbklienten"},
	"QTF_RUNNER_ENABLED":            {"`runner.enabled` (`1` or `true` = on)", "`runner.enabled` (`1` eller `true` = på)"},
	"QTF_RUNNER_WEB_ENABLED":        {"`runner.web.enabled` (`1` or `true` = on)", "`runner.web.enabled` (`1` eller `true` = på)"},
	"QTF_RUNNER_WORK_DIR":           {"`runner.work_dir`", "`runner.work_dir`"},
	"QTF_RUNNER_WEB_NODE":           {"`runner.web.node`", "`runner.web.node`"},
	"QTF_RUNNER_WEB_PLAYWRIGHT_DIR": {"`runner.web.playwright_dir`", "`runner.web.playwright_dir`"},
	"QTF_RUNNER_WEB_BROWSERS_PATH":  {"`runner.web.browsers_path`", "`runner.web.browsers_path`"},
	"QTF_COOKIE_SECURE":             {"`auth.cookie_secure` (`1` or `true` = on, anything else off)", "`auth.cookie_secure` (`1` eller `true` = på, allt annat av)"},
	"QTF_SECRETS_PROVIDER":          {"`secrets.provider`", "`secrets.provider`"},
	"QTF_SECRETS_KEY_FILE":          {"`secrets.local.key_file`", "`secrets.local.key_file`"},
	"QTF_SECRETS_KEY":               {"the key of the local provider itself (32 bytes, base64 or hex; `qtestframe secrets generate-key`); no key file is used then", "själva nyckeln för den lokala leverantören (32 byte, base64 eller hex; `qtestframe secrets generate-key`); då används ingen nyckelfil"},
	"QTF_SECRETS_TPM_DEVICE":        {"`secrets.tpm.device`", "`secrets.tpm.device`"},
	"QTF_SECRETS_PKCS11_MODULE":     {"`secrets.pkcs11.module`", "`secrets.pkcs11.module`"},
	"QTF_SECRETS_PKCS11_PIN":        {"the HSM user PIN; wins over `secrets.pkcs11.pin_file`", "HSM:ens användar-PIN; vinner över `secrets.pkcs11.pin_file`"},
}

// extraEnv are environment variables outside config.EnvVars, in the order
// of the reference.
var extraEnv = []struct {
	name string
	doc  text
}{
	{"QTF_CONFIG", text{"the configuration file when `--config` is not given", "konfigurationsfilen när `--config` inte anges"}},
	{"QTF_OIDC_<NAME>_CLIENT_SECRET", text{"client secret of the OIDC provider `<name>` (upper case); wins over `client_secret_file` and a secret stored in the web client", "klienthemligheten för OIDC-leverantören `<namn>` (versaler); vinner över `client_secret_file` och en hemlighet sparad i webbklienten"}},
	{"CREDENTIALS_DIRECTORY", text{"set by systemd (`LoadCredential=`): relative `*_file` paths are resolved in it, and it is readable after the lock-down", "sätts av systemd (`LoadCredential=`): relativa `*_file`-sökvägar slås upp där, och den är läsbar efter låsningen"}},
	{"STATE_DIRECTORY", text{"set by systemd (`StateDirectory=`): default place of the key file when the database is not SQLite", "sätts av systemd (`StateDirectory=`): standardplats för nyckelfilen när databasen inte är SQLite"}},
	{"QTF_URL, QTF_TOKEN, QTF_CONFIGURATION", text{"defaults of `--url`, `--token` and `--configuration` of `qtestframe report`", "standardvärden för `--url`, `--token` och `--configuration` i `qtestframe report`"}},
}
