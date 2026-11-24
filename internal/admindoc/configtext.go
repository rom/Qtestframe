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
	"auth.session_idle_timeout": {
		"Ends a browser session after this long without activity, e.g. `30m` (`0` = off: only `session_ttl` applies). The electronic-records mode of an organisation can set a shorter one; the shorter applies.",
		"Avslutar en webbläsarsession efter så här lång tid utan aktivitet, t.ex. `30m` (`0` = av: bara `session_ttl` gäller). En organisations läge för elektroniska poster kan ange en kortare; den kortare gäller.",
	},
	"auth.password_min_classes": {
		"How many of the character classes lower-case letters, upper-case letters, digits and symbols a local password must use (2 to 4; `0` = letters and at least one digit or symbol).",
		"Hur många av teckenklasserna gemener, versaler, siffror och symboler ett lokalt lösenord måste innehålla (2 till 4; `0` = bokstäver och minst en siffra eller symbol).",
	},
	"auth.password_history": {
		"How many earlier passwords a user cannot choose again when they change their password (0 to 24; `0` = off).",
		"Hur många tidigare lösenord en användare inte kan välja igen när lösenordet byts (0 till 24; `0` = av).",
	},
	"auth.password_max_age_days": {
		"A local password expires this many days after it was set: the next sign-in must change it (`0` = never). For accounts older than this setting the days count from their next sign-in.",
		"Ett lokalt lösenord går ut så här många dagar efter att det sattes: nästa inloggning måste byta det (`0` = aldrig). För konton som är äldre än inställningen räknas dagarna från nästa inloggning.",
	},
	"auth.token_max_days": {
		"Longest lifetime of a new API token in days (`0` = no limit). With a limit every new token needs an expiry date within it; existing tokens keep theirs.",
		"Längsta livslängd för en ny API-token i dagar (`0` = ingen gräns). Med en gräns behöver varje ny token ett utgångsdatum inom den; befintliga tokens behåller sina.",
	},
	"auth.token_require_expiry": {
		"Refuse new API tokens without an expiry date.",
		"Neka nya API-tokens utan utgångsdatum.",
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
	"auth.oidc[].icon": {
		"Icon of the sign-in button: `sign-in`, `key`, `shield`, `building`, `microsoft`, `google`, `github`, `amazon` or `apple` (empty = a generic icon).",
		"Ikon på inloggningsknappen: `sign-in`, `key`, `shield`, `building`, `microsoft`, `google`, `github`, `amazon` eller `apple` (tom = en allmän ikon).",
	},
	"auth.oidc[].allowed_domains": {
		"E-mail domains that may sign in through the provider (`example.com`; `*.example.com` for its subdomains). Empty = any. Other addresses are refused before an account is looked up or created.",
		"E-postdomäner som får logga in via leverantören (`example.com`; `*.example.com` för dess underdomäner). Tom = alla. Andra adresser nekas innan ett konto slås upp eller skapas.",
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
	"auth.saml[].allowed_domains": {
		"E-mail domains that may sign in through the provider, as for OIDC. Empty = any.",
		"E-postdomäner som får logga in via leverantören, som för OIDC. Tom = alla.",
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
	"email.reply_to": {
		"Reply-To address of e-mails that have none of their own, e.g. a support mailbox (empty = none). Invitations and shared reports reply to the person who sent them.",
		"Svarsadress (Reply-To) för e-post som saknar en egen, t.ex. en supportbrevlåda (tom = ingen). Inbjudningar och delade rapporter svarar till personen som skickade dem.",
	},
	"outbound.proxy": {
		"HTTP(S) proxy for the server's outgoing calls (webhooks, chat, defect trackers and requirement sources, AI, embeddings, OIDC discovery, environment health checks), e.g. `http://proxy.example.com:3128`. Empty = the environment variables `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY`.",
		"HTTP(S)-proxy för serverns utgående anrop (webhooks, chatt, ärendehanterare och kravkällor, AI, embeddings, OIDC-discovery, hälsokontroller av miljöer), t.ex. `http://proxy.example.com:3128`. Tom = miljövariablerna `HTTPS_PROXY`, `HTTP_PROXY` och `NO_PROXY`.",
	},
	"outbound.no_proxy": {
		"Hosts reached without `proxy` (exact names or `*.example.com`).",
		"Värdar som nås utan `proxy` (exakta namn eller `*.example.com`).",
	},
	"outbound.allowed_hosts": {
		"When set, the only hosts outgoing calls may reach (exact names, addresses or `*.example.com`); others are refused. Protects against server-side request forgery through webhook and tracker URLs.",
		"När den är satt de enda värdar som utgående anrop får nå (exakta namn, adresser eller `*.example.com`); andra nekas. Skyddar mot server-side request forgery via webhook- och ärendehanteraradresser.",
	},
	"outbound.denied_hosts": {
		"Hosts outgoing calls never reach, e.g. `metadata.google.internal` or `169.254.169.254`.",
		"Värdar som utgående anrop aldrig når, t.ex. `metadata.google.internal` eller `169.254.169.254`.",
	},
	"outbound.block_private": {
		"Refuse outgoing calls to loopback, private, link-local and other non-public addresses (checked on the address actually connected to), unless the host is in `allowed_hosts`. Turn it on when the server is reachable from the internet and its users should not reach your internal network through it.",
		"Neka utgående anrop till loopback-, privata, länklokala och andra icke-publika adresser (kontrolleras på adressen som faktiskt ansluts), om inte värden finns i `allowed_hosts`. Slå på den när servern nås från internet och användarna inte ska nå ert interna nät genom den.",
	},
	"email.digest_hour": {
		"Hour of the day (UTC, 0–23) when daily digests are sent.",
		"Timme på dygnet (UTC, 0–23) då dagliga sammanfattningar skickas.",
	},
	"live.enabled": {
		"Live updates and presence: lists, dashboards, the notification bell and open items update when someone else changes something, and editors show who else is viewing or editing an item (a Server-Sent Events stream, `GET /api/v1/events/stream`). Off: the web client falls back to polling and to the conflict check on save. Each user can also turn it off in the profile. Behind a reverse proxy, turn off response buffering for the stream (see *Live updates* in the chapter *Features added later*).",
		"Liveuppdateringar och närvaro: listor, översikter, notisklockan och öppna objekt uppdateras när någon annan ändrar något, och redigerare visar vem mer som tittar på eller redigerar ett objekt (en Server-Sent Events-ström, `GET /api/v1/events/stream`). Av: webbklienten frågar i stället med jämna mellanrum och konfliktkontrollen vid sparande gäller som förut. Varje användare kan också stänga av det i profilen. Bakom en omvänd proxy ska buffring av svar stängas av för strömmen (se *Liveuppdateringar* i kapitlet *Funktioner som lagts till senare*).",
	},
	"live.heartbeat": {
		"Interval of the keep-alive comments on an idle stream. Keep it below the idle timeout of proxies and load balancers (nginx `proxy_read_timeout`, 60 s by default).",
		"Intervall för håll-vid-liv-kommentarer på en ström utan händelser. Håll det under tidsgränsen för inaktiva anslutningar i proxyer och lastbalanserare (nginx `proxy_read_timeout`, 60 s som standard).",
	},
	"live.max_connections": {
		"Most open event streams of this server instance; further connections are refused with 503 and the clients poll instead.",
		"Högsta antal öppna händelseströmmar i den här serverinstansen; fler anslutningar avvisas med 503 och klienterna frågar i stället med jämna mellanrum.",
	},
	"live.max_connections_per_user": {
		"Most open event streams of one user (one per browser tab; a tab in the background closes its stream after a minute).",
		"Högsta antal öppna händelseströmmar för en användare (en per flik i webbläsaren; en flik i bakgrunden stänger sin ström efter en minut).",
	},
	"live.replay_buffer": {
		"Number of recent events kept in memory so that a client that reconnects (`Last-Event-ID`) receives what it missed; when its last event is no longer kept, it reloads what it shows instead.",
		"Antal senaste händelser som hålls i minnet så att en klient som ansluter igen (`Last-Event-ID`) får det den missade; finns dess senaste händelse inte kvar läser den i stället om det den visar.",
	},
	"live.presence_ttl": {
		"How long a user stays listed as viewing or editing an item after the last presence heartbeat of their browser (sent every 20 s).",
		"Hur länge en användare visas som att den tittar på eller redigerar ett objekt efter den senaste närvarosignalen från webbläsaren (skickas var 20:e sekund).",
	},
	"similarity.embeddings.enabled": {
		"Use an embeddings service for the similarity of requirements and tests (suspected duplicates, the *Similar requirements* panel). Off by default: the similarity is then computed on the server. **When on, the title, description and criteria or steps of the items are sent to that service.** Also in **Administration → System settings**.",
		"Använd en tjänst för inbäddningar (embeddings) för likheten mellan krav och tester (misstänkta dubbletter, panelen *Liknande krav*). Av som standard: likheten beräknas då på servern. **När den är på skickas rubrik, beskrivning och kriterier eller steg för objekten till tjänsten.** Finns även under **Administration → Systeminställningar**.",
	},
	"similarity.embeddings.url": {
		"URL of an OpenAI-compatible embeddings endpoint, e.g. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, answer `{data: [{embedding}]}`).",
		"Adress till en OpenAI-kompatibel embeddings-slutpunkt, t.ex. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, svaret `{data: [{embedding}]}`).",
	},
	"similarity.embeddings.model": {
		"Model sent as `model` (empty: the service's default). Vectors are cached per item version and model; changing the model computes them again.",
		"Modell som skickas som `model` (tom: tjänstens standard). Vektorerna cachas per objektversion och modell; byts modellen beräknas de om.",
	},
	"similarity.embeddings.api_key_file": {
		"File with the API key, sent as a bearer token (relative: in `$CREDENTIALS_DIRECTORY`). The key can also be stored in the system settings; it is never shown again.",
		"Fil med API-nyckeln, som skickas som bearer-token (relativ: i `$CREDENTIALS_DIRECTORY`). Nyckeln kan också sparas i systeminställningarna; den visas aldrig igen.",
	},
	"similarity.embeddings.timeout": {
		"Longest time to wait for the service. When it fails or times out, the local similarity is used.",
		"Längsta tid att vänta på tjänsten. När den misslyckas eller inte svarar används den lokala likheten.",
	},
	"ai.enabled": {
		"Turn on the optional [AI assistance](system-settings.md#ai-assistance) (drafts of tests, improved requirements, run summaries, framework mappings, acceptance criteria). Off by default. **When on, the text of the items a user asks about is sent to the configured service.** Each organisation must also turn it on, and projects can opt out. Also in **Administration → System settings**.",
		"Slå på den valfria [AI-assistansen](system-settings.md#ai-assistans) (utkast till tester, förbättrade krav, sammanfattningar av körningar, ramverksmappningar, acceptanskriterier). Av som standard. **När den är på skickas texten i de objekt som en användare frågar om till den konfigurerade tjänsten.** Varje organisation måste också slå på den, och projekt kan välja bort den. Finns även under **Administration → Systeminställningar**.",
	},
	"ai.provider": {
		"Type of the service: `anthropic` (the Anthropic Messages API) or `openai` (an OpenAI-compatible chat completions endpoint: OpenAI, Azure OpenAI, or a local server such as Ollama or vLLM).",
		"Typ av tjänst: `anthropic` (Anthropics Messages API) eller `openai` (en OpenAI-kompatibel chat completions-slutpunkt: OpenAI, Azure OpenAI eller en lokal server som Ollama eller vLLM).",
	},
	"ai.url": {
		"Base URL of the service. Empty with `anthropic`: `https://api.anthropic.com`. With `openai` required, e.g. `https://llm.example.com/v1` (the server appends `/chat/completions`); an Azure OpenAI deployment URL with `?api-version=…` sends the key as `api-key`.",
		"Tjänstens bas-URL. Tom med `anthropic`: `https://api.anthropic.com`. Krävs med `openai`, t.ex. `https://llm.example.com/v1` (servern lägger till `/chat/completions`); en Azure OpenAI-deployment-URL med `?api-version=…` skickar nyckeln som `api-key`.",
	},
	"ai.model": {
		"Model id sent to the service (required): the model of your Anthropic account, or the model or deployment of your OpenAI-compatible service. There is no built-in default.",
		"Modell-id som skickas till tjänsten (krävs): modellen i ditt Anthropic-konto, eller modellen eller deploymenten i din OpenAI-kompatibla tjänst. Det finns ingen inbyggd standard.",
	},
	"ai.api_key_file": {
		"File with the API key (relative: in `$CREDENTIALS_DIRECTORY`); sent as `x-api-key` (Anthropic), `Authorization: Bearer` or `api-key` (Azure). The key can also be stored, encrypted, in the system settings; it is never shown again. Local servers may need no key.",
		"Fil med API-nyckeln (relativ: i `$CREDENTIALS_DIRECTORY`); skickas som `x-api-key` (Anthropic), `Authorization: Bearer` eller `api-key` (Azure). Nyckeln kan också sparas, krypterad, i systeminställningarna; den visas aldrig igen. Lokala servrar behöver kanske ingen nyckel.",
	},
	"ai.timeout": {
		"Longest time to wait for one answer (5 s–10 min). The user can also cancel a request.",
		"Längsta tid att vänta på ett svar (5 s–10 min). Användaren kan också avbryta en begäran.",
	},
	"ai.max_tokens": {
		"Most tokens of one answer (256–128000).",
		"Högsta antal token i ett svar (256–128000).",
	},
	"ai.temperature": {
		"Sampling temperature sent to the service (0–2). Empty: not sent (the service's default; some models accept no other value).",
		"Temperatur som skickas till tjänsten (0–2). Tom: skickas inte (tjänstens standard; vissa modeller accepterar inget annat värde).",
	},
	"ai.rate_limit": {
		"Most AI requests of one user per hour.",
		"Högsta antal AI-förfrågningar per användare och timme.",
	},
	"ai.features.draft_tests": {
		"Offer *Draft tests* in the requirement editor (and drafting a test from a linked requirement in the test editor).",
		"Erbjud *Utkast till tester* i kravredigeraren (och utkast till ett test från ett länkat krav i testredigeraren).",
	},
	"ai.features.improve_requirement": {
		"Offer *Improve requirement* (a rewrite for the requirement checks, shown as a diff).",
		"Erbjud *Förbättra krav* (en omskrivning för kravkontrollerna, visad som skillnad).",
	},
	"ai.features.summarize_run": {
		"Offer *Summarise failures* on the run page (a summary and defect drafts).",
		"Erbjud *Sammanfatta fel* på körningssidan (en sammanfattning och utkast till defekter).",
	},
	"ai.features.suggest_mappings": {
		"Offer *Suggest mappings* (ATT&CK, D3FEND, EMB3D, CWE) for requirements and risks, checked against the embedded lists.",
		"Erbjud *Föreslå mappningar* (ATT&CK, D3FEND, EMB3D, CWE) för krav och risker, kontrollerade mot de inbyggda listorna.",
	},
	"ai.features.suggest_criteria": {
		"Offer *Suggest acceptance criteria* in the requirement editor.",
		"Erbjud *Föreslå acceptanskriterier* i kravredigeraren.",
	},
	"ai.price_input": {
		"Optional price per million input tokens (any currency), to show an estimated cost with each answer.",
		"Valfritt pris per miljon indata-token (valfri valuta), för att visa en uppskattad kostnad med varje svar.",
	},
	"ai.price_output": {
		"Optional price per million output tokens.",
		"Valfritt pris per miljon utdata-token.",
	},
	"retention.enabled": {
		"Run the nightly retention job, which deletes records older than their retention period (see *Privacy and data retention*). Check the dry-run preview under **Administration → System settings → Retention and backups** before turning it on.",
		"Kör den nattliga gallringen, som raderar poster som är äldre än sin lagringstid (se *Integritet och gallring*). Kontrollera förhandsvisningen under **Administration → Systeminställningar → Gallring och säkerhetskopior** innan du slår på den.",
	},
	"retention.hour": {
		"Hour of the day (UTC, 0–23) the retention job runs.",
		"Timme på dygnet (UTC, 0–23) då gallringen körs.",
	},
	"retention.runs_days": {
		"Days to keep test runs with their results, step results, comments and attachments (0 = forever). Runs in progress, of a project or release under legal hold or with an electronic signature are kept.",
		"Dagar att behålla testkörningar med resultat, stegresultat, kommentarer och bilagor (0 = för alltid). Pågående körningar, körningar i ett projekt eller en release med juridiskt bevarande och körningar med en elektronisk signatur behålls.",
	},
	"retention.runs_keep_latest": {
		"Always keep the runs holding one of the latest N results of each test, however old (0 = no exception).",
		"Behåll alltid de körningar som innehåller något av de N senaste resultaten för varje test, hur gamla de än är (0 = inget undantag).",
	},
	"retention.logs_days": {
		"Days to keep logs and uploaded artifacts: the sign-in log, finished test runner jobs, CI status posts, tracker sync logs and web test evidence (screenshots, traces, videos) (0 = forever).",
		"Dagar att behålla loggar och uppladdade artefakter: inloggningsloggen, avslutade jobb i testkörningen, CI-statusposter, synkloggar mot ärendesystem och bevis från webbtester (skärmbilder, spår, videor) (0 = för alltid).",
	},
	"retention.notifications_days": {
		"Days to keep in-app notifications, read or not (0 = forever).",
		"Dagar att behålla aviseringar i appen, lästa eller inte (0 = för alltid).",
	},
	"retention.sessions_days": {
		"Days to keep sign-in sessions not used for that long and API tokens that expired that long ago (0 = forever).",
		"Dagar att behålla inloggningssessioner som inte använts så länge och API-tokens som gick ut för så länge sedan (0 = för alltid).",
	},
	"retention.chat_days": {
		"Days to keep the chat message history (sent or failed messages with their text) and the personal input suggestion history (0 = forever).",
		"Dagar att behålla historiken för chattmeddelanden (skickade eller misslyckade meddelanden med sin text) och den personliga historiken för inmatningsförslag (0 = för alltid).",
	},
	"retention.exploratory_days": {
		"Days to keep the notes of completed or debriefed exploratory sessions, counted from the session's last change (0 = forever). The session and its debrief stay.",
		"Dagar att behålla anteckningarna från avslutade eller genomgångna utforskande sessioner, räknat från sessionens senaste ändring (0 = för alltid). Sessionen och dess genomgång finns kvar.",
	},
	"privacy.truncate_ip": {
		"Store and log client addresses shortened: IPv4 to /24, IPv6 to /48 (sign-in log, sessions, remembered devices, request log). Rate limits still use the full address in memory.",
		"Lagra och logga klientadresser förkortade: IPv4 till /24, IPv6 till /48 (inloggningslogg, sessioner, betrodda enheter, begäranslogg). Begränsningar av antal försök använder fortfarande hela adressen i minnet.",
	},
	"privacy.notice_url": {
		"Link to your privacy notice, shown on the sign-in page (empty = none).",
		"Länk till er integritetspolicy, visas på inloggningssidan (tom = ingen).",
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
	"QTF_LIVE_ENABLED":              {"`live.enabled` (`1` or `true` = on, anything else = off)", "`live.enabled` (`1` eller `true` = på, allt annat = av)"},
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
