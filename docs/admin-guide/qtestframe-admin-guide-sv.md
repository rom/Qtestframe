# Administratörshandbok för Qtestframe

Den här handboken är för dem som **installerar, konfigurerar och driftar** en Qtestframe-server: systemadministratörer, driftpersonal och säkerhetsansvariga. Den handlar om servern, konfigurationsfilen, single sign-on, krypteringsnycklar, säkerhetskopior och den dagliga driften.

De som *använder* Qtestframe – även organisationsadministratörer som hanterar medlemmar och roller i webbklienten – har [användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/index.md). Där båda handböckerna tar upp samma sak (medlemmar, roller, systeminställningar) förklarar den här handboken serversidan och länkar till användarhandboken för skärmbilderna.

## Kapitel

1. [Översikt och arkitektur](#översikt-och-arkitektur) – komponenter, datalager, portar, vad som körs var.
2. [Installation](#installation) – systemkrav, programfil, container, systemd, bygga från källkod, SQLite eller PostgreSQL.
3. [Konfiguration](#konfiguration) – hur inställningar läses in, hemlighetsfiler, och referensen över alla nycklar och miljövariabler.
4. [Första start](#första-start) – den första administratören, organisationer, demodata.
5. [Autentisering](#autentisering) – lokala lösenord, flerfaktorsautentisering, OIDC, SAML, SCIM, API-token, sessioner.
6. [Behörigheter](#behörigheter) – roller, egna roller, behörighetsmatrisen, begränsade och delade projekt.
7. [Systeminställningar under drift](#systeminställningar-under-drift) – vad superadministratörer kan ändra i webbklienten, och vad som kräver omstart.
8. [Säkerhet](#säkerhet) – TLS, härdning med Landlock och seccomp, kryptering av lagrade hemligheter och nyckelleverantörerna, nyckelbyte, katastrofåterställning, granskningsloggen, säkerhetshuvuden.
9. [E-post, webhooks och integrationer](#e-post-webhooks-och-integrationer) – SMTP, aviseringar, webhooks, Jira, GitHub och GitLab, körning av skript och webbtester.
10. [Koppla ihop med andra verktyg](#koppla-ihop-med-andra-verktyg-oslc-och-reqif) – OSLC-kopplingar till IBM DOORS Next och ETM, Polarion och Codebeamer, dialoger i ramar, ReqIF.
11. [Loggning och övervakning](#loggning-och-övervakning) – loggformat, syslog, hälsokontroller.
12. [Säkerhetskopiering, återställning och migrering](#säkerhetskopiering-återställning-och-migrering) – vad en säkerhetskopia innehåller, återställning, uppgradering och nedgradering, flytt mellan SQLite och PostgreSQL.
13. [Driftrutiner](#driftrutiner) – återkommande uppgifter, kapacitet, felsökning, loggmeddelanden.
14. [Kommandoradsreferens](#kommandoradsreferens) – alla `qtestframe`-kommandon och flaggor.
15. [Funktioner som lagts till senare](#funktioner-som-lagts-till-senare) – administrationsanteckningar för nyare funktioner.

## Konventioner

- `qtestframe …` är serverprogrammet; alla kommandon tar `--config FIL` (standard: `$QTF_CONFIG`). På en systemd-värd körs det som tjänsteanvändaren, till exempel `sudo -u qtestframe qtestframe secrets status --config /etc/qtestframe/config.yaml`.
- Konfigurationsnycklar skrivs med punkter: `server.base_url` är `base_url` i avsnittet `server:` i YAML-filen. `auth.oidc[].name` är `name` i en post i listan `auth.oidc`.
- **Superadministratör** är en flagga på ett användarkonto som ger alla behörigheter i alla organisationer och de installationsövergripande sidorna (organisationer, användare, säkerhetskopiering, systeminställningar). **Administratör** (admin) är en roll i en organisation.
- Sökvägar som `/etc/qtestframe` och `/var/lib/qtestframe` är systemd-installationens (`scripts/install.sh`); containeravbilden använder samma sökvägar.

Delar av [konfigurationsreferensen](#referens), [systeminställningarna](#alla-inställningar), [behörighetsmatrisen](#standardmatrisen-för-behörigheter), [webhook-händelserna](#webhooks), [tabellerna i en säkerhetskopia](#vad-en-säkerhetskopia-innehåller), [migreringarna](#migreringar) och [kommandoradsreferensen](#kommandoradsreferens) genereras från källkoden (`make admin-guide-ref`), så de stämmer med den version de följer med.

## Finns också som

Handboken publiceras också som PDF och som en enda Markdown-fil per språk på [user-guide-utgåvan](https://github.com/rom/Qtestframe/releases/tag/user-guide) (`qtestframe-admin-guide-en.pdf`, `qtestframe-admin-guide-sv.pdf`), och visas i webbklienten under **Hjälp → Administratörshandbok** för superadministratörer och organisationsadministratörer.

# Översikt och arkitektur

Qtestframe är **ett program**, `qtestframe`. Samma programfil är webbservern, REST-API:t, den inbyggda webbklienten och kommandoradsverktyget för administration. Under drift behövs inget annat än en databas, och med SQLite inte ens det.

## Komponenter

| Komponent | Vad den är |
|---|---|
| `qtestframe serve` | Serverprocessen: HTTP(S)-lyssnare, REST-API, webbklienten, bakgrundsjobb. |
| Webbklient | En Vue 3-applikation som är inkompilerad i programfilen (`internal/webui`). Den serveras från `/` och talar bara med serverns eget API. |
| REST-API | `/api/v1/…`, beskrivet som OpenAPI 3.1 på `/api/v1/openapi.json` (öppet, ingen inloggning). Används av webbklienten, CI-flöden (API-token) och `qtestframe report`. |
| SCIM 2.0 | `/scim/v2/…`, provisionering av användare och grupper från en identitetsleverantör, med egna token. |
| Hälsokontroller | `/healthz` (processen lever, returnerar versionen) och `/readyz` (databasen svarar). Loggas inte, ingen inloggning. |
| Kommandorad | `qtestframe migrate`, `user create`, `backup`, `secrets …` med flera öppnar databasen direkt; se [kommandoradsreferensen](#kommandoradsreferens). |
| Skriptkörning | Valfri (`runner.enabled`). Kör skripten i automatiska tester på servern, vart och ett i en sandlådad barnprocess (`qtestframe sandbox-exec`). |
| Webbtestkörning | Valfri (`runner.web.enabled`). Kör Playwright-tester med Node.js och en riktig webbläsare som bakgrundsjobb. |

## Datalager

| Lager | Innehåll | Anmärkning |
|---|---|---|
| **Databasen** (SQLite eller PostgreSQL) | Allt: organisationer, användare, projekt, krav, tester, körningar, historik, kommentarer, granskningsloggen, sessioner, **uppladdade filer** (bilagor, dokument, rapporter, avatarer) och systeminställningarna som ändrats i webbklienten. | Skapas och uppgraderas automatiskt av [migreringarna](#migreringar). |
| **Nyckeln till de lagrade hemligheterna** | Nyckelkrypteringsnyckeln som skyddar de hemligheter servern måste kunna läsa tillbaka (TOTP-hemligheter, webhook-hemligheter, ärendesystemtoken, hemliga testvariabler, SMTP-lösenord och OIDC-klienthemligheter sparade i webbklienten). | En nyckelfil (`qtestframe-secrets.key`), variabeln `QTF_SECRETS_KEY`, en TPM 2.0 eller en HSM; se [Kryptering av lagrade hemligheter](#kryptering-av-lagrade-hemligheter). **Aldrig i databasen.** |
| **Konfiguration** | YAML-filen (`/etc/qtestframe/config.yaml`), hemlighetsfiler, TLS- och SAML-nycklar. | Läses en gång vid start; bara TLS-nyckelparet läses om vid `SIGHUP`. |
| **Arbetskatalog för körningar** | En jobbkatalog per skript eller webbtest (`runner.work_dir`). | Tillfälligt utrymme, säkerhetskopieras inte. |

Det finns inget separat fillager, ingen cache och ingen meddelandekö: en säkerhetskopia av databasen (plus nyckeln) är en säkerhetskopia av installationen.

## Portar och anslutningar

Qtestframe lyssnar på **en TCP-port** (`server.listen`, standard `:8080`; produktionsmallarna använder `:8443` med TLS). Allt nedan går genom den: webbklienten, API:t, SCIM, callback-adresserna för single sign-on och hälsokontrollerna.

Utgående anslutningar, bara när funktionen är konfigurerad:

| Till | För | Konfigureras av |
|---|---|---|
| PostgreSQL | databasen | `database.dsn` |
| OIDC-leverantör | discovery, token-endpoint, nycklar (JWKS) | `auth.oidc` |
| SAML-IdP | hämtning av metadata (inte med `idp_metadata_file`) | `auth.saml` |
| SMTP-server | aviseringsmejl, inbjudningar, rapporter | `email` |
| Syslog-server | loggskickning | `logging.syslog` |
| Jira, GitHub, GitLab | defekter och ärenden | projektinställningar (behörigheten `project.update`) |
| Mottagare av webhooks | signerade händelser | projektinställningar (behörigheten `project.update`) |
| System som testas | webbtester och skript | testernas innehåll (behörigheten `script.execute`) |

Eftersom projektadministratörer väljer adresserna till ärendesystem och webhooks, och skriptförfattare målen för webbtester, bör serverns utgående trafik filtreras när den når interna tjänster (se [Säkerhet](#utgående-anslutningar)).

## Vad som körs i serverprocessen

Förutom att besvara begäranden kör `serve` några bakgrundsuppgifter:

| Uppgift | När |
|---|---|
| Rensa utgångna sessioner och papperskorgsposter äldre än 30 dagar | varje timme |
| Spara en daglig mätpunkt per projekt (trender på översiktspanelen) | vid start och sedan varje timme, en gång per projekt och dag |
| Dagliga e-postsammanfattningar och schemalagda rapporter | kontrolleras varje minut, bara när e-post är på |
| Arbetspool för webbtester och *Kör automatiska tester* | `runner.web.max_parallel` jobb åt gången |
| Leverans av webhooks | när ändringen som utlöser dem har sparats |

## En instans eller flera

**SQLite** betyder exakt en serverprocess per databasfil: databasen körs genom en enda anslutning i WAL-läge, vilket gör att skrivningar sker i tur och ordning. Det är den enklaste uppsättningen och räcker för små och medelstora team.

**PostgreSQL** tillåter flera serverprocesser mot samma databas (till exempel bakom en lastbalanserare). Sessioner, token och alla data delas. Varje process har en del eget tillstånd, så:

- Konfigurera **session affinity** i lastbalanseraren åtminstone för `/api/v1/auth/`: inloggningstillståndet för OIDC/SAML signeras med en nyckel som varje process skapar vid start, så omdirigeringen till identitetsleverantören och callback-anropet måste nå samma process.
- **Gränserna för inloggningsförsök** räknas per process.
- Varje process kör sin egen **jobbpool**; jobb från en process som stoppas återställs av de andra efter ungefär en minut.
- **Uppgradera alla processer samtidigt**: en äldre version kan inte läsa det en nyare har skrivit (migreringar, krypterade hemligheter).

Hur man väljer databas står under [Installation](#sqlite-eller-postgresql).

# Installation

## Systemkrav

| | |
|---|---|
| **Operativsystem** | Linux på amd64 eller arm64 för produktion. macOS (arm64) fungerar för att prova och för utveckling; härdningen i processen (Landlock, seccomp, capabilities) finns bara på Linux och rapporteras som överhoppad på andra system. |
| **Kärna** | Landlock kräver Linux 5.13 eller senare med `landlock` bland de aktiva säkerhetsmodulerna; äldre kärnor kör utan (loggas vid start). |
| **Minne och processer** | Den medföljande systemd-enheten tillåter 2 GB (`MemoryMax=2G`) och 512 trådar/processer; compose-filen 1 GB och 256 processer. Webbtester startar en webbläsare per jobb och behöver betydligt mer (se `runner.web.max_parallel`). |
| **Disk** | Databasen innehåller alla uppladdade filer. Räkna med projektens bilagor och dokument plus säkerhetskopiorna. |
| **Databas** | Inget för SQLite (inbyggt, ren Go). PostgreSQL 13 eller senare (16 rekommenderas) för större installationer. |
| **Valfritt** | En SMTP-server för e-post; en OIDC- eller SAML-identitetsleverantör; en syslog-server; Node.js 18+ och Playwright för webbtester; Python, Bash och Node.js för skript; en TPM 2.0 eller en HSM för krypteringsnyckeln. |
| **För att bygga** | Go 1.26 (`go.mod` låser verktygskedjan 1.26.11, som `go` laddar ned), Node.js 20 eller senare (CI använder 22), `make`. En C-kompilator bara för HSM-bygget. |

## SQLite eller PostgreSQL

| | SQLite (standard) | PostgreSQL |
|---|---|---|
| Uppsättning | ingen: filen skapas vid första starten | en separat databasserver |
| Serverprocesser | exakt **en** per databasfil | en eller flera (lastbalanserare, rullande omstarter) |
| Samtidighet | en anslutning, skrivningar i tur och ordning; räcker för tiotals samtidiga användare och databaser på några GB | en anslutningspool (`database.max_open_conns`, standard 20) |
| Säkerhetskopiering | `qtestframe backup`, eller en filkopia med SQLites onlinebackup | `qtestframe backup`, `pg_dump`, återställning till valfri tidpunkt |
| Hög tillgänglighet | nej | replikering av databasservern |
| Filsystem | bara lokal disk (inte NFS); WAL-filerna `-wal` och `-shm` bredvid | — |

Välj **SQLite** för en enskild server, utvärderingar och team som vill ha så lite som möjligt att drifta. Välj **PostgreSQL** när ni behöver mer än en serverprocess, när databasen ska skötas av ett databasteam, eller när ni vill ha replikering och återställning till valfri tidpunkt. Ni kan flytta senare åt båda hållen med en [logisk säkerhetskopia](#flytt-mellan-sqlite-och-postgresql).

## Från en programfil

Programfilen är statisk (`CGO_ENABLED=0`, SQLite-drivrutinen är ren Go), så en kopia av `qtestframe` är hela installationen. Statiska programfiler för Linux amd64/arm64 och macOS arm64 byggs med:

```sh
make release          # dist/qtestframe-linux-amd64, -linux-arm64, -darwin-arm64
```

Prova med den färdiga exempelkonfigurationen (HTTP på port 8080, SQLite i `./data`, en bootstrap-administratör):

```sh
./qtestframe serve --config examples/qtestframe.yaml
```

Logga sedan in på `http://localhost:8080` som `admin@example.com` med `Change-me-1234` och byt lösenordet. **Exemplet är inte för produktion**: det stänger av säkra kakor och HSTS, slår på skriptkörning och har bootstrap-lösenordet i filen.

## På en systemd-värd

`scripts/install.sh` installerar programfilen, en härdad enhet, tjänsteanvändaren och katalogstrukturen. Skriptet kan köras flera gånger: en ny körning uppgraderar programfil och enhet och behåller konfiguration, nycklar och data.

```sh
make build                                      # webbklient + bin/qtestframe
sudo scripts/install.sh --binary bin/qtestframe --self-signed-cert qtf.example.com
```

| Flagga | Verkan |
|---|---|
| `--binary SÖKVÄG` | programfil att installera (standard `./bin/qtestframe` eller `./dist/qtestframe`) |
| `--selinux auto\|yes\|no` | bygg och läs in SELinux-modulen (standard `auto`: när SELinux är påslaget) |
| `--apparmor` | installera och tillämpa AppArmor-profilen |
| `--sysctl` | installera `deploy/sysctl/99-qtestframe.conf` |
| `--self-signed-cert [CN]` | skapa ett självsignerat certifikat när inget finns |
| `--no-enable`, `--no-start` | aktivera eller (om)starta inte enheten |
| `--dry-run` | skriv bara ut vad som skulle göras |

Det skapar:

| Sökväg | Ägare och läge | Innehåll |
|---|---|---|
| `/usr/bin/qtestframe` | root, 0755 | programfilen |
| `/etc/qtestframe/config.yaml` | root:qtestframe, 0640 | konfigurationen, från `deploy/config.example.yaml`; skrivs aldrig över |
| `/etc/qtestframe/tls/` | root:qtestframe, 0750 | `server.crt`, `server.key` (0640), SAML- och klientnycklar |
| `/etc/qtestframe/secrets/` | root:qtestframe, 0750 | filer som `*_file`-inställningar pekar på |
| `/var/lib/qtestframe/` | qtestframe, 0750 | SQLite-databasen, nyckelfilen, `runner/` |
| `/etc/systemd/system/qtestframe.service` | root, 0644 | enheten (`deploy/systemd/qtestframe.service`) |
| `/etc/sysusers.d/`, `/etc/tmpfiles.d/` | root | användaren `qtestframe` och katalogerna |

Om det konfigurerade certifikatet inte finns ännu startas inte tjänsten. Redigera `/etc/qtestframe/config.yaml` (minst `server.base_url`, TLS och bootstrap-administratören, se [Första start](#första-start)) och sedan:

```sh
sudo systemctl enable --now qtestframe
journalctl -u qtestframe -f
```

Enheten kör `qtestframe migrate` före `serve` (`ExecStartPre`), startar om tjänsten vid fel, och `systemctl reload qtestframe` skickar `SIGHUP`, som läser om TLS-certifikatet. Ändra enheten bara med en drop-in (`sudo systemctl edit qtestframe`), till exempel för `LoadCredential=`, `MemoryMax=` eller de lättnader som körningen kräver (se [Integrationer](#körning-av-skript-och-webbtester)).

Ta bort med `sudo scripts/uninstall.sh` (behåller konfiguration och data) eller `--purge` (tar bort allt, även databasen).

### Brandvägg, MAC och port 443

- `deploy/nftables/qtestframe.nft` är en brandväggsregeluppsättning med en valfri policy för utgående trafik; gå igenom dess mängder (`admin_v4`, `out_tcp_ports`, …) innan den läses in. Let's Encrypt HTTP-01 kräver inkommande port 80.
- SELinux- (`deploy/selinux`) och AppArmor-profiler (`deploy/apparmor`) begränsar tjänsten; valfria funktioner styrs av SELinux-booleans som `qtestframe_connect_postgresql`, `qtestframe_connect_http` (single sign-on), `qtestframe_remote_syslog` och `qtestframe_run_scripts`. Se [Säkerhet](#lager-i-operativsystemet).
- För port 443: sätt antingen `server.listen: ":443"` (enheten ger `CAP_NET_BIND_SERVICE`; med SELinux också `setsebool -P qtestframe_bind_http_port on`), eller behåll `:8443` och styr om 443 dit med nftables, vilket tillåter en enhet helt utan capabilities.

## I en container (Docker eller Podman)

`deploy/container/Containerfile` bygger en distroless, statisk avbild som körs som UID 65532 med skrivskyddat rotfilsystem; tillståndet ligger i volymen `/var/lib/qtestframe`. Avbilden sätter `QTF_DATABASE_DRIVER=sqlite`, `QTF_DATABASE_DSN=/var/lib/qtestframe/qtestframe.db`, `QTF_SERVER_LISTEN=:8443` och `QTF_LOG_FORMAT=json`, så den kör utan konfigurationsfil. Dess `HEALTHCHECK` kör `qtestframe healthcheck` (avbilden har varken skal eller curl).

```sh
docker build -f deploy/container/Containerfile --build-arg VERSION=$(git describe --tags --always) -t qtestframe .
```

`deploy/container/compose.yaml` kör Qtestframe med PostgreSQL 16 på ett internt nät. Förbered hemligheterna och certifikatet enligt beskrivningen överst i filen, och sedan, från `deploy/container` (sökvägen till seccomp-profilen är relativ):

```sh
docker compose up -d --build        # eller: podman compose up -d --build
docker compose logs -f qtestframe
```

En engångstjänst `migrate` körs före servern. Compose-filen tillämpar `cap_drop: ALL`, `no-new-privileges`, seccomp-listan `deploy/seccomp/qtestframe-seccomp.json`, en minnes- och processgräns, och sätter `QTF_DATABASE_DRIVER=postgres` (avbildens standard är SQLite).

Att tänka på med containrar:

- **Miljövariabler låser systeminställningar.** En inställning som sätts av en `QTF_*`-variabel kan inte ändras under **Administration → Systeminställningar** (se [Systeminställningar under drift](#företräde-och-låsning)); i avbilden gäller det loggformatet och lyssningsadressen.
- **Nyckelfilen** för de lagrade hemligheterna ligger som standard på datavolymen (`/var/lib/qtestframe/qtestframe-secrets.key`). Säkerhetskopiera den separat, eller ge nyckeln som `QTF_SECRETS_KEY` från ert hemlighetslager.
- **Webbtester** kräver en större avbild: `--build-arg WITH_PLAYWRIGHT=true` bygger en Debian-baserad avbild med Node.js, `@playwright/test` och Chromium (`PLAYWRIGHT_BROWSERS` lägger till webbläsare). Slå på dem med `QTF_RUNNER_ENABLED=true` och `QTF_RUNNER_WEB_ENABLED=true`. Standardavbilden har inga tolkar, så skript kan inte köras i den.
- **Kubernetes**: använd samma avbild med `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, seccomp-profilen som `Localhost`-profil och HTTP-kontroller mot `/healthz` (liveness) och `/readyz` (readiness).

## Bygga från källkod

```sh
make build            # npm ci + bygge av webbklienten, sedan den statiska Go-programfilen i bin/qtestframe
make go               # bara Go-programfilen (kräver att webbklienten redan är byggd)
make test             # enhetstester för Go och webben
```

Bygget bäddar in versionen från `git describe` (`qtestframe version`). `make web` skriver webbklienten till `internal/webui/dist`, som Go-bygget bäddar in.

### Med HSM-stöd (PKCS#11)

PKCS#11-moduler är C-bibliotek, så HSM-nyckelleverantören kräver cgo. Standardprogrammet vägrar `secrets.provider: pkcs11` och talar om hur man bygger ett som stöder det:

```sh
make go-pkcs11        # CGO_ENABLED=1 go build -tags pkcs11 → bin/qtestframe
```

Resultatet länkas dynamiskt mot libc och läser in leverantörens modul vid körning. Testa leverantören mot SoftHSM2 med `make test-pkcs11` (`apt-get install softhsm2`), och TPM-leverantören mot en mjukvaru-TPM med `make test-tpm` (`apt-get install swtpm`). Hur båda konfigureras står under [Säkerhet](#nyckelleverantörer).

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
5. för `serve`: de [systeminställningar](#systeminställningar-under-drift) som superadministratörer har ändrat i webbklienten och som sparats i databasen – utom inställningar som en miljövariabel sätter eller som `system_settings.locked` listar.

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
| en inställning under **Administration → Systeminställningar** | direkt, eller efter en omstart för körningen av skript och webbtester (se [Systeminställningar under drift](#systeminställningar-under-drift)) |

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

### `similarity`

| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |
|---|---|---|---|---|
| `similarity.embeddings.enabled` | bool | `false` | — | Använd en tjänst för inbäddningar (embeddings) för likheten mellan krav och tester (misstänkta dubbletter, panelen *Liknande krav*). Av som standard: likheten beräknas då på servern. **När den är på skickas rubrik, beskrivning och kriterier eller steg för objekten till tjänsten.** Finns även under **Administration → Systeminställningar**. |
| `similarity.embeddings.url` | text | — | — | Adress till en OpenAI-kompatibel embeddings-slutpunkt, t.ex. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, svaret `{data: [{embedding}]}`). |
| `similarity.embeddings.model` | text | — | — | Modell som skickas som `model` (tom: tjänstens standard). Vektorerna cachas per objektversion och modell; byts modellen beräknas de om. |
| `similarity.embeddings.api_key_file` | text | — | — | Fil med API-nyckeln, som skickas som bearer-token (relativ: i `$CREDENTIALS_DIRECTORY`). Nyckeln kan också sparas i systeminställningarna; den visas aldrig igen. |
| `similarity.embeddings.timeout` | tidslängd | `30s` | — | Längsta tid att vänta på tjänsten. När den misslyckas eller inte svarar används den lokala likheten. |

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

# Första start

## Vad som händer vid start

`qtestframe serve` gör i den här ordningen:

1. läser in och kontrollerar konfigurationen (en ogiltig konfiguration stoppar starten, med alla fel listade);
2. öppnar databasen och tillämpar de [migreringar](#migreringar) som återstår;
3. öppnar nyckelleverantören för de lagrade hemligheterna, och krypterar hemligheter som en tidigare version lagrat i klartext (en saknad eller fel nyckel stoppar starten här);
4. skapar **bootstrap-administratören** om databasen saknar användare;
5. tillämpar systeminställningarna som sparats i databasen;
6. öppnar lyssnaren och TLS-certifikatet, och **låser sedan processen** (no_new_privs, capabilities, Landlock, seccomp) och loggar en rad `hardening`;
7. startar bakgrundsjobben och loggar `qtestframe listening` med adress, TLS, version och databasdrivrutin.

## Den första administratören

Det finns två sätt att få det första kontot.

**Bootstrap i konfigurationen.** Så länge databasen **inte har några användare alls** skapar servern en användare från avsnittet `bootstrap`, gör den till **superadministratör** och **administratör** i organisationen `bootstrap.tenant_slug` (skapas om den saknas, standard `default` / *Default*):

```yaml
bootstrap:
  tenant_slug: acme
  tenant_name: "Acme AB"
  admin_email: admin@acme.example
  admin_password_file: /etc/qtestframe/secrets/admin-password
```

Lösenordet måste ha minst 10 tecken med minst en bokstav och ett tecken som inte är en bokstav; ett svagare stoppar starten. När någon användare finns ignoreras avsnittet, så det kan stå kvar i filen, men ta bort lösenordsfilen när administratören har loggat in och bytt lösenord. Utan bootstrap-inställningar loggar servern `no users exist; configure bootstrap.admin_email/admin_password or run qtestframe user create`.

**På kommandoraden**, som tjänsteanvändaren:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
read -rs PW && printf '%s\n' "$PW" | qtf user create --email admin@acme.example --name "Admin" --password-stdin --super-admin
qtf tenant create --slug acme --name "Acme AB"
qtf member add --tenant acme --email admin@acme.example --role admin
```

Lösenordet läses från standard input, aldrig från kommandoraden (där `ps` skulle visa det). En användare som skapas utan `--password-stdin` har inget lösenord och kan bara logga in med single sign-on.

## Organisationer

En **organisation** (tenant) är en isolerad arbetsyta: projekt, medlemmar, roller, API-token, SCIM-token, MFA-policy och granskningslogg hör till en organisation och syns inte för de andra. Ett användarkonto hör till installationen och kan vara medlem i flera organisationer, med en roll i varje; användaren växlar mellan dem i webbklienten.

- Skapa organisationer under **Administration → Organisationer** (superadministratörer) eller med `qtestframe tenant create --slug S --name N`. Slug är 2–63 tecken av `a-z`, `0-9` och `-`; användare kan skriva den vid inloggningen.
- Ge personer roller under **Administration → Medlemmar** (organisationsadministratörer) eller med `qtestframe member add`.
- Att ta bort en organisation tar bort alla dess projekt och data och kan inte ångras (bara en [återställning](#återställning) tar tillbaka den).

Single sign-on kan lägga nya användare direkt i en organisation (`default_tenant`, `role_mapping`, se [Autentisering](#provisionering-av-användare-med-single-sign-on)), och SCIM provisionerar medlemmarna i en organisation per token.

## Demodata

```sh
qtf seed-demo                 # i bootstrap.tenant_slug
qtf seed-demo --tenant acme
```

`seed-demo` lägger till exempelprojektet *Demo – Online Banking* (nyckeln `DEMO`, sedan `DEMO2`, … när den redan finns) med krav, tester, länkar, körningar med historik, en release, ett demowebbtest, och ett begränsat projekt som delas med en av två demoanvändare, `dana@example.com` och `erik@example.com`, vars lösenord är `Demo-user-1234`. Kommandot kör bootstrap först, så demoanvändarna tar aldrig den första administratörens plats. Demowebbtestet öppnar serverns egen inloggningssida på `server.base_url`.

**Kör aldrig `seed-demo` mot en produktionsdatabas**: det skapar konton med ett publicerat lösenord. Använd en separat databas för provning och utbildning.

## Checklista efter första starten

- Logga in som bootstrap-administratören och **byt lösenord** (användarmenyn → **Profil och lösenord**); lägg till en autentiseringsapp.
- Sätt `server.base_url` till adressen som användarna skriver; den behövs för single sign-on och e-post.
- Säkerhetskopiera **nyckelfilen** för de lagrade hemligheterna (`/var/lib/qtestframe/qtestframe-secrets.key` på en SQLite-installation) till en plats skild från databasens säkerhetskopior. Se [Säkerhet](#kryptering-av-lagrade-hemligheter).
- Bestäm om single sign-on, en [MFA-policy](#flerfaktorsautentisering), e-post och syslog.
- Sätt upp regelbundna [säkerhetskopior](#säkerhetskopiering-återställning-och-migrering) och prova en återställning.

# Autentisering

Qtestframe känner till de här sätten att visa vem någon är:

| Metod | För | Konfigureras i |
|---|---|---|
| Lokalt lösenord | personer | `auth.local`, lösenordsregler |
| Andra faktor (TOTP-autentiseringsapp, återställningskoder) | personer med lokalt lösenord, valfritt efter OIDC | MFA-policyn i webbklienten; `local_mfa` per OIDC-leverantör |
| OpenID Connect | personer | `auth.oidc` (filen eller systeminställningarna) |
| SAML 2.0 | personer | `auth.saml` (bara filen) |
| Inbjudan via e-post | en persons första inloggning | kräver e-post |
| API-nyckel (`qtf_…`) | skript och CI | **Administration → API-nycklar** |
| SCIM-token (`qtfscim_…`) | en identitetsleverantörs provisionering | **Administration → Provisionering (SCIM)** |

## Lokala lösenord

- Lösenord hashas med **bcrypt** (kostnad 12). Reglerna är: minst `auth.password_min_length` tecken (standard 10, högst 72 byte, eftersom bcrypt ignorerar resten), med minst en bokstav och ett tecken som inte är en bokstav. `qtestframe user create` och bootstrap-administratören kontrolleras alltid mot 10.
- Okända användare, fel lösenord, avstängda konton och konton utan lösenord får alla samma svar (`401 invalid credentials`) och tar lika lång tid.
- **Gräns för inloggningsförsök:** högst `auth.login_max_attempts` försök (standard 10) per *klientadress + e-postadress* inom `auth.login_lockout` (standard 5 minuter); fler får `429 rate_limited`. En lyckad inloggning nollställer räknaren. Räknaren finns i minnet, per serverprocess. Bakom en omvänd proxy: sätt `server.trusted_proxies`, annars delar alla användare proxyns adress.
- Misslyckade inloggningar loggas på nivån WARN (`login failed`, med e-post och klientadress) och sparas i användarens inloggningshistorik, som webbklienten visar som "misslyckade försök sedan din senaste inloggning".
- **Stäng av lokal inloggning** med `auth.local: false` när single sign-on fungerar. I webbklienten går det bara när en single sign-on-leverantör är påslagen. Superadministratörer kan fortfarande hanteras på kommandoraden.
- Organisationsadministratörer återställer lösenord för medlemmar som bara tillhör deras organisation (**Administration → Medlemmar → Återställ lösenord**: ett tillfälligt lösenord, som visas en gång och måste bytas vid nästa inloggning); superadministratörer återställer allas under **Användare**. Båda loggar ut användaren överallt.

## Flerfaktorsautentisering

Den andra faktorn är en **TOTP**-kod från en autentiseringsapp (RFC 6238: HMAC-SHA-1, 6 siffror, 30-sekundersintervall, ±1 intervall för klockavvikelse), med **10 återställningskoder** för engångsbruk. Användarna lägger till den i sin profil. Varje kod fungerar bara en gång (det senast använda intervallet sparas), så högst en inloggning per 30 sekunder och användare.

**Policyer** bestämmer vem som måste använda den (**Administration → Tvåstegsverifiering**, se [användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md#tv%C3%A5stegsverifiering)):

| Policy | Sätts av | Gäller |
|---|---|---|
| Serverpolicy | superadministratörer | alla användare; roller räknas i alla organisationer, plus pseudorollen *superadministratör* |
| Organisationspolicy | organisationsadministratörer (`tenant.manage`) | organisationens medlemmar efter deras roll där |

Var och en är *valfri* (standard), *krävs för alla* eller *krävs för vissa roller*. En organisation kan kräva mer än servern, aldrig mindre. En **övergångsperiod** (dagar) börjar när en policy börjar kräva MFA för fler, eller när användaren skapades om det är senare; därefter måste en användare utan andra faktor lägga till en innan något annat fungerar (`403 mfa_enrolment_required`). Policyn utvärderas vid inloggning, så en striktare policy når befintliga sessioner vid deras nästa inloggning.

**Kom ihåg den här enheten** (serverpolicy, *dagar*, standard 0 = av) låter en webbläsare hoppa över koden – aldrig lösenordet – under så många dagar. Token kan återkallas på servern; användare kan glömma sina enheter, och en administratörsåterställning eller att stänga av MFA glömmer dem också.

Gissning begränsas på tre sätt: varje kod räknas mot inloggningsgränsen ovan (som det rätta lösenordet ensamt inte nollställer), ett konto tillåter 10 kodförsök per 15 minuter oavsett adress, och en påbörjad inloggning tas bort efter 5 felaktiga koder.

**Borttappad telefon och återställningskoder:** en organisationsadministratör använder **Återställ tvåstegsverifiering** under **Medlemmar** (för medlemmar som bara tillhör den organisationen), en superadministratör under **Användare**. Det tar bort autentiseringsappen, återställningskoderna och ihågkomna enheter, loggar ut användaren och meddelar hen med avisering och e-post. Kontrollera begäran via en annan kanal först.

TOTP-hemligheterna lagras krypterade (se [Säkerhet](#kryptering-av-lagrade-hemligheter)). API-nycklar och SCIM-token påverkas inte av MFA; skydda dem som lösenord. WebAuthn och passkeys finns inte.

## OpenID Connect

Registrera en **konfidentiell webbklient** hos leverantören med:

- omdirigerings-URI `<server.base_url>/api/v1/auth/oidc/<namn>/callback`,
- beviljandetypen *authorization code* (Qtestframe använder alltid PKCE med S256),
- scopes `openid email profile`, plus `groups` om leverantören erbjuder det.

Lägg sedan till leverantören:

```yaml
auth:
  oidc:
    - name: corp
      display_name: "Företagsinloggning"
      issuer: "https://sso.example.com/realms/acme"
      client_id: qtestframe
      client_secret_file: oidc-corp            # eller QTF_OIDC_CORP_CLIENT_SECRET
      scopes: [openid, email, profile]
      groups_claim: groups
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"qtf-admins": admin, "qtf-testers": tester}
```

eller lägg till den under **Administration → Systeminställningar → Enkel inloggning (OIDC)**, som kan testa discovery-dokumentet. Alla nycklar står i [referensen](#authoidc). Steg för Keycloak, Microsoft Entra ID och Okta finns i [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#6-single-sign-on-oidc) (på engelska).

Vad Qtestframe kontrollerar: state, nonce och PKCE-verifieraren sparas i en signerad kaka för engångsbruk som gäller i 10 minuter; ID-tokens signatur verifieras mot leverantörens nycklar, och utfärdare, mottagare och giltighetstid kontrolleras. Token måste innehålla en e-postadress (`email_claim`); `email_verified: false` avvisas. Efter inloggningen går webbläsaren alltid till `/`.

Servern behöver utgående HTTPS till utfärdaren. Med SELinux: `setsebool -P qtestframe_connect_http on`. Namnge leverantörer med gemena bokstäver, siffror och `_`: namnet ingår i adressen och, med versaler, i `QTF_OIDC_<NAMN>_CLIENT_SECRET` (ett `-` gör variabeln svår att sätta).

### Lokal MFA och amr/acr

Som standard ansvarar identitetsleverantören för MFA, och Qtestframes egen andra faktor efterfrågas inte efter en OIDC-inloggning (`local_mfa: off`). Två striktare inställningar:

| `local_mfa` | Qtestframe frågar efter sin egen kod |
|---|---|
| `off` (standard) | aldrig |
| `unless_idp_mfa` | om inte ID-token visar MFA: dess claim `amr` innehåller `mfa` eller ett värde i `mfa_amr_values` (t.ex. `[mfa, otp, hwk]`), eller dess `acr` är ett av `mfa_acr_values` |
| `always` | vid varje inloggning via den här leverantören |

När koden efterfrågas behöver användarna en autentiseringsapp i Qtestframe också (eller måste lägga till en om MFA-policyn kräver det). De rapporterade värdena för `amr`/`acr` skrivs i granskningsposten för inloggningen. SAML-inloggningar använder aldrig den lokala andra faktorn; konfigurera MFA hos identitetsleverantören.

## SAML 2.0

Skapa ett nyckelpar för Qtestframe som tjänsteleverantör (SP) och peka IdP:n mot SP-metadata:

```sh
sudo scripts/certs/gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs --out /etc/qtestframe/tls --group qtestframe
```

```yaml
auth:
  saml:
    - name: adfs
      display_name: "Företagsinloggning (ADFS)"
      idp_metadata_url: "https://adfs.example.com/FederationMetadata/2007-06/FederationMetadata.xml"
      sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
      sp_key_file: /etc/qtestframe/tls/saml-adfs.key
      email_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"
      groups_attr: "http://schemas.xmlsoap.org/claims/Group"
      auto_provision: true
      default_tenant: acme
      role_mapping: {"QTF-Admins": admin}
```

| Adress | |
|---|---|
| `<base_url>/api/v1/auth/saml/<namn>/metadata` | SP-metadata (också standard för entitets-id) |
| `<base_url>/api/v1/auth/saml/<namn>/acs` | assertion consumer service (HTTP-POST) |

Bara **SP-initierad** inloggning stöds; IdP-initierade (oombedda) svar avvisas. Svaret eller varje assertion måste vara signerat av IdP-certifikatet i metadata; destination, mottagare och giltighetstid kontrolleras. AuthnRequests signeras inte, så stäng av *client signature required* hos IdP:n. Krypterade assertions dekrypteras med `sp_key_file`. SAML-leverantörer konfigureras bara i filen och läser `server.base_url` vid start. Metadataadressen hämtas vid första inloggningen; `idp_metadata_file` kräver ingen utgående anslutning.

## Provisionering av användare med single sign-on

Vid varje inloggning med OIDC eller SAML:

1. Användaren söks efter leverantör och subjekt. Annars efter e-post: ett lokalt konto (eller ett som SCIM skapat) med den adressen **kopplas** till leverantören och dess lösenord tas bort; ett konto som är kopplat till en *annan* leverantör avvisas. Annars skapas en ny användare bara med `auto_provision: true`.
2. Avstängda användare avvisas.
3. Med `default_tenant` blir rollen den mest privilegierade roll som `role_mapping` ger någon av användarens grupper, annars `default_role` (`viewer` när den är tom eller okänd). Ett saknat medlemskap skapas med `auto_provision`; med en `role_mapping` sätts ett befintligt medlemskap till den mappade rollen vid varje inloggning – identitetsleverantören bestämmer.

Eftersom konton kopplas via e-postadress: anslut bara identitetsleverantörer vars adresser är verifierade och tillhör er organisation. Medlemmar som SCIM hanterar lämnas åt SCIM.

## Provisionering med SCIM

Med SCIM 2.0 skapar, uppdaterar och inaktiverar en identitetsleverantör (Microsoft Entra ID, Okta med flera) medlemmarna i **en organisation**. Organisationsadministratören skapar en SCIM-token under **Administration → Provisionering (SCIM)** och ger leverantören basadressen `<base_url>/scim/v2` och token; [användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md#provisionering-med-scim) beskriver leverantörernas skärmar.

Det här bör serveradministratören veta:

- En SCIM-token (`qtfscim_` + 256 slumpbitar) lagras som hash, visas en gång, kan ha ett utgångsdatum och autentiserar bara `/scim/v2` för sin organisation. Den styr organisationens medlemskap upp till administratörsrollen (via en grupp som mappas till admin), så behandla den som ett administratörslösenord.
- Den kan inte ändra superadministratörsstatus, lösenord, projektdata eller andra organisationer; en organisation behåller alltid en administratör.
- Provisionerade konton saknar lösenord: de loggar in via OIDC eller SAML, som kopplar dem via e-post.
- När en användare inaktiveras tas medlemskapet bort och användarens sessioner och API-nycklar i den organisationen upphör.
- Gränser: 1200 begäranden per minut och token; 20 misslyckade autentiseringar per 5 minuter blockerar klientadressen (`429`). Varje ändring granskas med aktören `SCIM: <tokennamn>`.

## API-nycklar

API-nycklar är för skript och CI-flöden (`Authorization: Bearer qtf_…`). Medlemmar med `token.manage` (Testare och uppåt som standard) skapar dem under **Administration → API-nycklar**.

- En nyckel är `qtf_` + 32 slumpbyte; bara dess SHA-256-hash och ett kort prefix lagras, och den visas en gång.
- Den hör till **en organisation** och har en roll som inte kan överstiga skaparens. Vid varje begäran **begränsas** dess behörigheter **av skaparens nuvarande roll**: sänks användarens roll sänks nyckelns, tas medlemmen bort upphör den.
- Nycklar kan inte skapa nycklar, har aldrig superadministratörsrättigheter (inte ens en superadministratörs), kan inte använda systeminställningarna och omfattas inte av MFA.
- Valfritt utgångsdatum i dagar; senaste användning sparas; återkallelse gäller direkt.
- Begäranden med en nyckel hoppar över CSRF-kontrollen (det finns ingen kaka), så nyckeln är den enda inloggningsuppgiften: förvara den i CI-systemets hemlighetslager.

## Sessioner

- En session skapas vid varje inloggning. Webbläsaren har bara den slumpmässiga kakan `qtf_session` (`HttpOnly`, `SameSite=Lax`, `Secure` utom med `auth.cookie_secure: false`); databasen lagrar en SHA-256-hash av den.
- Sessioner upphör efter `auth.session_ttl` (standard 12 h, absolut, förlängs inte), vid utloggning, och när en administratör stänger av användaren, återställer lösenordet eller återställer MFA. Att byta sitt eget lösenord avslutar användarens andra sessioner. Utgångna sessioner rensas varje timme.
- Användare ser sina sessioner (webbläsare, adress, senaste aktivitet) i sin profil och kan avsluta var och en.
- Begäranden från webbläsaren som ändrar något måste skicka sessionens CSRF-token och klara en ursprungskontroll mot begärans värd eller `server.base_url`.
- Sessioner ingår inte i en säkerhetskopia; en återställning loggar ut alla.

# Behörigheter

## Modellen

- Varje medlem har **en eller flera roller per organisation**; utan medlemskap ser en användare ingenting av en organisation. Den som både skriver krav och testar har till exempel *Ämnesexpert* och *Testare*.
- En roll är en namngiven mängd **behörigheter**; en medlem med flera roller har **unionen** av deras behörigheter (roller lägger bara till rättigheter). Sju roller är inbyggda: Administratör, Ansvarig, Testledare, Testare, Ämnesexpert, Granskare och Läsare.
- Den som har Administratör bland sina roller är organisationsadministratör. Äldre API-klienter ser en `role`, den *primära* rollen: medlemmens starkaste inbyggda roll (alltid Administratör när medlemmen har den).
- **Superadministratör** är en flagga på användarkontot, som sätts med `qtestframe user create --super-admin` eller under **Administration → Användare**. En superadministratör har alla behörigheter i alla organisationer (och agerar som administratör i en organisation utan att vara medlem), och är ensam om att hantera organisationer och användare, säkerhetskopiera och återställa, och ändra systeminställningarna. Det gäller bara inloggade sessioner: API-nycklar har det aldrig.
- Servern kontrollerar behörigheterna vid varje begäran; webbklienten döljer bara det man inte får göra. Ett objekt i en annan organisation, eller i ett projekt man inte får se, svarar `404`, så att dess existens inte avslöjas.

## Standardmatrisen för behörigheter

Det här är de inbyggda rollernas behörigheter tills en organisation ändrar dem. Administratör har alltid alla behörigheter och kan inte ändras.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Behörighet | Läsare | Granskare | Ämnesexpert | Testare | Testledare | Ansvarig | Administratör |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  | ✔ |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ | ✔ |  |  | ✔ | ✔ |
| `waiver.approve` |  |  |  |  |  | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `cycle.plan` |  |  |  |  | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  |  | ✔ | ✔ | ✔ |
| `comment.create` |  | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

### Ämnesexperten

**Ämnesexpert** (`sme`, subject matter expert) är en domänexpert som deltar i att skriva kraven inom sitt område – en kliniker, en säkerhetsingenjör, en jurist. Rollen:

- skapar och redigerar krav (`requirement.write`): text, acceptanskriterier, motivering, källor, relationer, spårnings- och objektlänkar och bilagor;
- deltar i kravgranskningar (`requirement.review`) och diskussioner (`comment.create`);
- läser tester, körningar, defekter och risker som underlag, och exporterar och tar fram rapporter (`export`).

Den skriver och granskar medvetet **inte** tester, skapar eller kör inte testkörningar, planerar inte cykler, godkänner inte undantag eller restrisker, ändrar inte projektinställningar, delning eller medlemmar och skapar inga API-nycklar. Belägg för testkörningar registreras av testare; en ämnesexpert bifogar underlag till själva kraven. Ge en andra roll till den som gör mer, t.ex. *Ämnesexpert* + *Testare*.

Eftersom rollen både skriver och granskar krav kan en ämnesexpert godkänna sina egna krav. Där åtskild behörighet är viktig tar du bort `requirement.review` från rollen i matrisen (eller ger den bara till separata granskare).

Vad behörigheterna tillåter:

| Behörighet | Tillåter |
|---|---|
| `project.read`, `requirement.read`, `test.read`, `run.read` | att läsa projekt och deras krav, tester och körningar |
| `project.create`, `project.update`, `project.delete` | att skapa projekt, ändra deras inställningar (även ärendesystem, webhooks, webbtestinställningar och delning), ta bort dem |
| `requirement.write`, `test.write` | att redigera krav och tester |
| `requirement.review`, `test.review` | granskningsbeslut: godkänna eller avslå krav, markera tester som klara |
| `run.write`, `run.execute` | att skapa och redigera körningar; registrera resultat, CI-importer, skicka defekter till ärendesystem |
| `script.execute` | att köra skript och webbtester **på servern** |
| `export` | exporter och rapporter |
| `audit.read` | organisationens händelselogg |
| `token.manage` | egna API-nycklar |
| `member.read`, `member.manage` | medlemslistan; lägga till, ändra och ta bort medlemmar, återställa deras lösenord och MFA |
| `role.manage` | rollmatrisen och egna roller |
| `settings.manage` | organisationens inställningar för testexekvering |
| `tenant.manage` | att byta namn på organisationen, dess MFA-policy och SCIM-provisionering |

Ge `script.execute` bara till personer som ni skulle låta köra kod på servern: skript och webbtester körs med serveranvändarens rättigheter, i sandlådan som beskrivs under [Säkerhet](#sandlådan-för-skript-och-webbtester).

## Egna och ändrade roller

Den som har `role.manage` (Administratör som standard) ändrar behörigheterna för alla inbyggda roller utom Administratör och skapar **egna roller** under **Administration → Rollmatris** ([användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md#%C3%A4ndra-roller-och-skapa-egna)). Reglerna som gör det säkert:

- Ingen kan ge en behörighet som hen inte själv har – inte till en roll, inte till en API-nyckel, inte via projektdelning.
- Administratör behåller alltid alla behörigheter, och en organisation har alltid minst en medlem med rollen Administratör: den sista kan inte förlora rollen eller tas bort – inte i medlemslistan, inte i grupp, inte via SCIM. Administratör kan kombineras med andra roller.
- En egen roll har ett tekniskt namn på 2–32 tecken (`a-z`, `0-9`, `_`, börjar med en bokstav), som är vad `member add --role` (flera roller kommaseparerade: `--role sme,tester`), `role_mapping` och SCIM-gruppmappningar använder. Den kan tas bort när ingen medlem, projektmedlem eller nyckel har den, inte heller bland andra roller.
- En inbyggd roll vars behörigheter skiljer sig från standarden markeras *ändrad* och kan återställas. Ändringar loggas, och en ändring av en medlems roller loggas med de roller som lagts till och tagits bort (`+sme −viewer`).
- API-nycklar har också roller (som standard alla ägarens roller) och får aldrig mer än ägarens nuvarande roller tillåter.
- Med single sign-on (`role_mapping`) och SCIM får en användare i flera mappade grupper rollerna för alla.

En organisations gällande matris finns på `GET /api/v1/roles`.

## Begränsade och delade projekt

Ett projekt är antingen **organisationsgemensamt** (standard: alla medlemmar ser det med sin organisationsroll) eller **begränsat** (bara dess projektmedlemmar ser det). Projektmedlemmar är användare eller SCIM-grupper med en eller flera **projektroller**, som ersätter organisationsrollerna i projektet, uppåt eller nedåt. Användare med `project.update` i projektet hanterar detta under **Dela** ([användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md#delning-av-projekt)).

| Vem | Behörigheter i ett projekt |
|---|---|
| organisationsadministratörer och superadministratörer | sina egna, alltid – de ser alla projekt |
| en projektmedlem (direkt eller via en grupp; det direkta medlemskapet vinner, grupper läggs ihop) | projektrollernas projektbehörigheter (sammanlagda) |
| övriga medlemmar, organisationsgemensamt projekt | organisationsrollerna |
| övriga medlemmar, begränsat projekt | inga: projektet svarar `404` |

Bara **projektbehörigheter** kommer från projektrollerna (`project.read`, `project.update`, `project.delete`, `requirement.*`, `waiver.approve`, `test.*`, `run.*`, `cycle.plan`, `script.execute`, `export`, `comment.create`). Behörigheter på organisationsnivå – medlemmar, roller, API-nycklar, händelseloggen, att skapa projekt – kommer alltid från organisationsrollerna, och en API-nyckel begränsas av sina egna roller även i ett projekt.

Vad en begränsning **inte** döljer, så planera för det:

- organisationens **händelselogg** (`audit.read`) registrerar åtgärder i alla projekt;
- e-postsammanfattningar kan innehålla aviseringar som skapades medan mottagaren fortfarande hade åtkomst;
- **säkerhetskopior** innehåller alla projekt; projektarkiv (*Spara projekt*) innehåller hela projektet.

## Var administrationen sker

| Sida i webbklienten | Vem |
|---|---|
| Medlemmar, Rollmatris | `member.read` (alla inbyggda roller) för att se dem; ändringar kräver `member.manage` / `role.manage` |
| API-nycklar | `token.manage` |
| Inställningar för testexekvering | `settings.manage` |
| Händelselogg | `audit.read` (Testledare och uppåt som standard) |
| Provisionering (SCIM), Tvåstegsverifiering (organisationens policy) | `tenant.manage` |
| Organisationer, Användare, Säkerhetskopiering, Systeminställningar, serverns MFA-policy | superadministratörer |

# Systeminställningar under drift

Superadministratörer kan ändra en bestämd uppsättning inställningar i webbklienten, under **Administration → Systeminställningar** (API `GET`/`PUT /api/v1/admin/settings`), utan att redigera filen eller starta om servern. Ändringarna sparas i databasen (tabellen `system_settings`) och gäller hela installationen; ingen inställning gäller per organisation. Hur sidan ser ut beskrivs i [användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md#systeminst%C3%A4llningar); det här kapitlet handlar om reglerna.

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

<!-- END GENERATED: system-settings -->

Vad varje nyckel betyder står i [konfigurationsreferensen](#referens). Allt annat – TLS, databasen, härdningen, nyckelleverantören, SAML, `trusted_proxies`, `cookie_secure`, `hsts`, körningens sökvägar – sätts bara i filen.

## Om single sign-on, syslog och e-post

- **OIDC-leverantörer.** När leverantörerna ändras i webbklienten sparas hela listan, som då **ersätter** `auth.oidc` i filen. Klienthemligheter för leverantörer som kommer från filen kopieras inte till databasen: de kommer fortsatt från `client_secret_file` eller `QTF_OIDC_<NAMN>_CLIENT_SECRET`, som alltid vinner. **Återställ till filens värde** går tillbaka till filens lista.
- **SAML**-leverantörer kan bara konfigureras i filen.
- **Syslog över TLS** använder `ca_file`, `cert_file` och `key_file` från filen; de är läsbara efter låsningen eftersom de läggs till bland Landlocks skrivskyddade sökvägar vid start. En Unix-socket som inte var konfigurerad vid start kan nekas av Landlock; använd `udp`, `tcp` eller `tls`, eller starta om.
- **Hemligheter** som anges på sidan (SMTP-lösenordet, OIDC-klienthemligheter) kan bara skrivas, lagras [krypterade](#kryptering-av-lagrade-hemligheter), loggas aldrig och visas aldrig igen.
- **Testknapparna** (*Testa anslutningen* för OIDC-discovery, *Skicka testmeddelande* för syslog, *Skicka testmejl*) får servern att ansluta till en adress som superadministratören anger. Filtrera serverns utgående trafik om den når interna tjänster.

## Tjänst för inbäddningar för liknande objekt

Misstänkt dubblerade krav och testfall, panelen *Liknande krav* i redigerarna och varningen vid import från katalogen jämför objekt på servern som standard: TF-IDF över projektet med ordstammar, stoppord och synonymer för svenska och engelska (se användarguiden, *Dubbletter och konflikter*). Ingenting skickas någonstans.

Om man vill kan en **OpenAI-kompatibel embeddings-slutpunkt** beräkna likheten i stället: **Administration → Systeminställningar → Likhet (dubbletter)**, eller `similarity.embeddings` i [konfigurationsfilen](#referens).

| Inställning | Betydelse |
|---|---|
| `similarity.embeddings.enabled` | av som standard |
| `similarity.embeddings.url` | slutpunkten, t.ex. `https://llm.example.com/v1/embeddings`; den får `POST {"model": …, "input": [texter]}` och svarar `{"data": [{"index": i, "embedding": [tal]}]}` |
| `similarity.embeddings.model` | skickas som `model` (tom: tjänstens standard) |
| `similarity.embeddings.api_key` | skickas som `Authorization: Bearer …`; kan bara skrivas, lagras [krypterad](#kryptering-av-lagrade-hemligheter); i filen bara som `api_key_file` |
| `similarity.embeddings.timeout` | längsta väntan (standard 30 s) |

**Integritet.** När den är på **skickas titel, beskrivning och acceptanskriterier för krav och titel, beskrivning, förutsättningar och steg för testfall till tjänsten**, för alla projekt i alla organisationer på servern, när likhetskontrollerna körs eller någon skriver i en redigerare (texten som skrivs skickas också). Använd en tjänst som drivs av er organisation eller som ni har avtal med; låt den annars vara av.

Vektorerna lagras i tabellen `similarity_embeddings` per objekt, modell och text (ett ändrat objekt skickas igen, ett oförändrat inte). Den är en cache: den ingår inte i säkerhetskopior eller projektarkiv och fylls på nytt efter en återställning. Byts modellen beräknas alla vektorer om. **Testa tjänsten** på sidan skickar en kort exempelmening och visar vektorns storlek. När tjänsten misslyckas eller inte svarar loggar servern en varning och använder den lokala likheten, så kontrollerna slutar aldrig fungera. Konfliktsökningen (motsatta förpliktelser, gränsvärden, uteslutande värden) körs alltid på servern. Kontrollernas gränsvärden gäller även tjänstens cosinuslikhet; tjänster skiljer sig, så granska de misstänkta dubbletterna när den har slagits på och justera gränsen för dubblettkontrollerna vid behov.

## Granskning och export

- Varje ändring skrivs i händelseloggen som `settings.update` med gammalt → nytt värde (hemligheter bara som *satt* / *ändrad* / *borttagen*), och i serverloggen. Testknapparna granskas som `settings.test_*`.
- **Exportera konfiguration** (`GET /api/v1/admin/settings/export`) laddar ned den gällande konfigurationen som YAML med hemligheter ersatta av `REDACTED` – till exempel för att flytta värdena till filen och sedan återställa dem på sidan.
- [Säkerhetskopior](#säkerhetskopiering-återställning-och-migrering) innehåller tabellen `system_settings` med dess hemligheter, fortfarande krypterade; en återställning tillämpar den direkt. Projektarkiv innehåller den inte.

# Säkerhet

Det här kapitlet är driftens perspektiv. Hotmodellen och designen bakom finns i [docs/security.md](https://github.com/rom/Qtestframe/blob/rom/docs/security.md) (på engelska).

## TLS och certifikat

Sätt `server.tls_cert` och `server.tls_key` för att servera HTTPS direkt. Servern godtar bara TLS 1.2 och 1.3; TLS 1.2 begränsas till ECDHE-nyckelutbyte med AES-GCM eller ChaCha20-Poly1305; ALPN erbjuder HTTP/2. Certifikatfilen innehåller hela kedjan, lövcertifikatet först.

Skript i `scripts/certs/`:

```sh
# Let's Encrypt (publikt namn, port 80 nåbar, eller en DNS-01-metod)
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com --email ops@example.com

# en intern CA och ett servercertifikat som den signerar
scripts/certs/gen-ca.sh --dir /root/qtf-ca --cn "ACME Internal CA"
sudo scripts/certs/gen-server-cert.sh --ca-dir /root/qtf-ca --cn qtf.example.com \
    --san DNS:qtf.example.com,IP:10.0.0.5 --out /etc/qtestframe/tls --group qtestframe

# kontrollera ett certifikat, eller vad en server visar upp
scripts/certs/verify-cert.sh --cert /etc/qtestframe/tls/server.crt --key /etc/qtestframe/tls/server.key --host qtf.example.com
scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21    # avslutningskod 2 = går snart ut
```

**Förnyelse utan avbrott:** `systemctl reload qtestframe` skickar `SIGHUP`, och servern läser in det nya paret för nya anslutningar; kan det inte läsas används det gamla vidare och `tls reload failed; keeping previous certificate` loggas. Deploy-kroken för Let's Encrypt gör detta åt er. Nyckelfiler: `0640 root:qtestframe`; med SELinux kör `restorecon -RF /etc/qtestframe/tls`.

**Bakom en omvänd proxy som avslutar TLS:** lyssna på `127.0.0.1`, sätt `server.trusted_proxies` till proxyns adress, behåll `auth.cookie_secure: true` och `server.base_url` på `https://`, och låt proxyn skicka `Strict-Transport-Security` – Qtestframe skickar HSTS bara på TLS-anslutningar som den själv avslutar.

## Härdning

### I processen

Efter starten – när databasen, nyckelleverantören, certifikatet och lyssnaren är öppna – begränsar `serve` sig själv (`internal/hardening`, bara Linux):

| Begränsning | Nyckel | Vad den gör |
|---|---|---|
| `no_new_privs` | `hardening.no_new_privs` | inga höjda rättigheter via setuid-program eller fil-capabilities, för servern och allt den startar. Ett fel **stoppar starten**. |
| capabilities | `hardening.drop_capabilities` | släpper alla capabilities utom `CAP_NET_BIND_SERVICE` (bounding, effective, permitted, inheritable, ambient) |
| Landlock | `hardening.landlock` | filåtkomst begränsad till de sökvägar konfigurationen behöver (så långt kärnan klarar, upp till Landlock ABI 5) |
| seccomp | `hardening.seccomp` | en spärrlista med systemanrop som en server aldrig behöver; de misslyckas med `EPERM` |

Utom för `no_new_privs` hoppas en begränsning som kärnan inte stöder över, utan att starten stoppas. Startloggen visar resultatet:

```text
time=… level=INFO msg=hardening applied="[no_new_privs capabilities landlock seccomp]" skipped=[]
```

**Landlocks sökvägar.** Servern får läsa den inbyggda listan nedan plus filerna den konfigurerats med (TLS-, syslog- och SAML-filer, `$CREDENTIALS_DIRECTORY`), och skriva i den tillfälliga katalogen, SQLite-databasens katalog, en Unix-socket för syslog, TPM-enheten, och med skriptkörning dess arbetskatalog (och köra program från `/usr`, `/bin`, `/lib`, `/lib64`, `/opt`, programfilens katalog och tolkarnas kataloger). Webbtester lägger till katalogerna för Playwright, webbläsarna och Node.js, `/etc`, `/proc`, `/sys` och `/dev/shm`. **Bara sökvägar som finns vid start läggs till.** Allt annat kräver `hardening.read_only_paths` eller `hardening.read_write_paths`, till exempel `/run/postgresql` för en lokal PostgreSQL-socket eller SoftHSM:s tokenkatalog.

<!-- BEGIN GENERATED: hardening (make admin-guide-ref) -->

| Lista | Poster |
|---|---|
| Sökvägar som får läsas efter låsningen (utöver de konfigurerade filerna) | `/etc/ssl`, `/etc/pki`, `/etc/ca-certificates`, `/usr/share/ca-certificates`, `/etc/resolv.conf`, `/etc/hosts`, `/etc/nsswitch.conf`, `/etc/localtime`, `/etc/gai.conf`, `/etc/services`, `/etc/protocols`, `/usr/share/zoneinfo`, `/etc/qtestframe`, `/proc/self` |
| Systemanrop som seccomp nekar (`EPERM`) | `acct`, `add_key`, `bpf`, `chroot`, `clock_adjtime`, `clock_settime`, `delete_module`, `finit_module`, `fsconfig`, `fsmount`, `fsopen`, `fspick`, `init_module`, `iopl`, `ioperm`, `kexec_file_load`, `kexec_load`, `keyctl`, `lookup_dcookie`, `mount`, `mount_setattr`, `move_mount`, `name_to_handle_at`, `open_by_handle_at`, `open_tree`, `perf_event_open`, `personality`, `pivot_root`, `process_vm_readv`, `process_vm_writev`, `ptrace`, `quotactl`, `reboot`, `request_key`, `setdomainname`, `sethostname`, `setns`, `settimeofday`, `swapoff`, `swapon`, `syslog`, `umount2`, `unshare`, `userfaultfd`, `vhangup` |
| Fler systemanrop som nekas skript med `runner.deny_network` | `socket`, `socketpair`, `connect`, `bind`, `listen`, `accept`, `accept4` |

<!-- END GENERATED: hardening -->

Ett `permission denied` efter start för en fil som servern behöver betyder att en sökväg saknas i reglerna. En post `landlock: …` under `skipped` betyder att kärnan saknar Landlock (äldre än 5.13, eller `landlock` saknas i startparametern `lsm=`).

### Lager i operativsystemet

Härdningen i processen fungerar också i containrar och utan systemd. På en värd tillkommer fler lager:

| Lager | Fil | Anmärkning |
|---|---|---|
| systemd-sandlåda | `deploy/systemd/qtestframe.service` | egen användare, `ProtectSystem=strict`, privata `/tmp` och enheter, `NoNewPrivileges`, bara `CAP_NET_BIND_SERVICE`, filter för systemanrop, `MemoryDenyWriteExecute`, `MemoryMax`/`TasksMax`. `systemd-analyze security qtestframe` bör visa ungefär 1.x. |
| SELinux | `deploy/selinux/` | domänen `qtestframe_t`, booleans för valfria funktioner (av som standard): `qtestframe_connect_postgresql`, `qtestframe_remote_syslog`, `qtestframe_connect_http`, `qtestframe_bind_http_port`, `qtestframe_run_scripts`, `qtestframe_runner_execmem` |
| AppArmor | `deploy/apparmor/usr.bin.qtestframe` | skrivskyddad konfiguration, skript i barnprofilen `qtestframe//runner` utan nätverk |
| seccomp för containrar | `deploy/seccomp/qtestframe-seccomp.json` | en lista över tillåtna anrop för containermiljöer |
| brandvägg, sysctl | `deploy/nftables/`, `deploy/sysctl/` | inkommande och valfri utgående policy, kärninställningar |

Kontrollera en server som körs:

```sh
PID=$(systemctl show -p MainPID --value qtestframe)
grep -E 'NoNewPrivs|Seccomp|CapEff' /proc/$PID/status    # NoNewPrivs 1, Seccomp 2, CapEff 0000000000000400
journalctl -u qtestframe | grep -i hardening
systemd-analyze security qtestframe
```

### Sandlådan för skript och webbtester

När `runner.enabled` är på kör medlemmar med `script.execute` kod på servern. Med `runner.sandbox: true` (standard) körs varje skript via `qtestframe sandbox-exec`, som innan tolken startas sätter resursgränser (CPU-tid = tidsgränsen + 1 s, 64 MiB filstorlek, 256 öppna filer, inga minnesdumpar), no_new_privs, släpper capabilities, tillämpar Landlock-regler (skrivskyddade systemkataloger, skrivning bara i jobbkatalogen) och seccomp-spärrlistan, med `runner.deny_network: true` även socketanropen. Miljön innehåller bara `PATH`, `HOME`/`TMPDIR` (jobbkatalogen), `LANG`, körningens `QTF_*`-variabler och projektets testvariabler. Hela processgruppen avbryts vid `runner.timeout`.

**Webbtester har en svagare sandlåda:** inget seccomp-filter och alltid nätverksåtkomst, eftersom en webbläsare behöver båda. Ett webbtest kan nå alla värdar som servern når, även interna tjänster och molnens metadatatjänster. Begränsa serverns utgående anslutningar (brandvägg, proxy) eller kör webbtester på en egen instans.

Även med alla lager: se körningen som en gräns som en målmedveten person med `script.execute` till slut kan ta sig förbi. Observera att `/etc` (med `/etc/qtestframe`) är läsbar i sandlådan: skydda konfiguration och nycklar med filrättigheter och SELinux/AppArmor.

## Kryptering av lagrade hemligheter

Servern måste kunna läsa vissa hemligheter tillbaka, så de kan inte hashas. De lagras **krypterade** med AES-256-GCM:

| Hemlighet | Lagras i |
|---|---|
| TOTP-hemligheter för autentiseringsappar | `user_mfa.totp_secret` |
| signeringshemligheter för webhooks | `webhooks.secret` |
| token för ärendesystem (Jira, GitHub, GitLab) | `projects.defect_tracker` |
| värden för hemliga webbtestvariabler | `projects.web_tests` |
| SMTP-lösenord, nyckeln till tjänsten för inbäddningar och OIDC-klienthemligheter som angetts i webbklienten | `system_settings` |

Lösenord (bcrypt), API- och SCIM-nycklar, sessioner, inbjudningar, ihågkomna enheter och återställningskoder lagras som hashar och behöver ingen nyckel. Hemligheter i konfigurationsfilen, `*_file`-inställningar och `QTF_*`-variabler lagras aldrig i databasen.

**Kuvertkryptering.** Varje värde krypteras med en slumpmässig 256-bitars *datakrypteringsnyckel* (DEK), lagras som `enc:v1:<dek-id>:<data>`, och binds till sin tabell, kolumn, rad och sitt fält, så att ett värde som kopieras någon annanstans inte går att dekryptera. DEK:arna ligger i tabellen `secret_keys`, **bara omslutna** av *nyckelkrypteringsnyckeln* (KEK) från den konfigurerade **nyckelleverantören**. KEK:en kommer aldrig in i databasen; oomslutna DEK:ar finns bara i minnet.

Vid varje start, och med `qtestframe secrets migrate`, krypteras värden som en äldre version lagrat i klartext. `qtestframe secrets status` visar leverantören, nyckelns fingeravtryck, DEK:arna och hur många värden som är krypterade; **Administration → Systeminställningar → Säkerhet** visar detsamma skrivskyddat.

### Nyckelleverantörer

| Leverantör | KEK | Skyddar mot | Bygge |
|---|---|---|---|
| `local` (standard) | en 256-bitarsnyckel i en nyckelfil, eller `QTF_SECRETS_KEY` | kopior av databasen, dumpar, säkerhetskopior och repliker som lämnar servern utan nyckeln | statiskt |
| `tpm` | förseglad till maskinens TPM 2.0, valfritt bunden till PCR-värden | dessutom: en kopierad disk, och med PCR:er en ändrad startkedja | statiskt (ren Go) |
| `pkcs11` | en AES-256-nyckel i en HSM, som används via PKCS#11 | dessutom: nyckeln kan aldrig kopieras ut ur HSM:en | `make go-pkcs11` (cgo) |

Ingen av dem skyddar mot den som kontrollerar den körande servern.

**local.** Inget att konfigurera. Nyckelfilen `qtestframe-secrets.key` skapas med läget 0600 när den saknas, bredvid en SQLite-databas (annars i `$STATE_DIRECTORY`, `/var/lib/qtestframe` eller arbetskatalogen); `secrets.local.key_file` lägger den någon annanstans, till exempel på en annan volym. Eller skapa en nyckel och ge den i miljön från ert hemlighetslager – då används ingen fil:

```sh
qtestframe secrets generate-key        # skriver ut en base64-nyckel för QTF_SECRETS_KEY
```

Servern varnar vid start när nyckelfilen kan läsas av gruppen eller andra. Flera serverprocesser mot samma PostgreSQL-databas behöver **samma** nyckel.

**tpm.**

```yaml
secrets:
  provider: tpm
  tpm:
    device: /dev/tpmrm0
    pcrs: [7]              # valfritt: bind till Secure Boot-läget
    pcr_bank: sha256
```

Ge tjänsten enheten i en drop-in, eftersom enheten har `PrivateDevices=yes`:

```ini
[Service]
PrivateDevices=no
DeviceAllow=/dev/tpmrm0 rw
SupplementaryGroups=tss
```

DEK:en förseglas under TPM:ens lagringsrotnyckel genom en saltad session, så den passerar aldrig bussen i klartext. Den kan bara avförseglas av samma TPM (tills dess ägarhierarki rensas), och med `pcrs` bara så länge de PCR:erna har kvar sina värden. Testa mot en mjukvaru-TPM med `swtpm` (`make test-tpm`).

**pkcs11.**

```yaml
secrets:
  provider: pkcs11
  pkcs11:
    module: /usr/lib/softhsm/libsofthsm2.so
    token_label: qtestframe       # eller slot: 0
    pin_file: hsm-pin             # eller QTF_SECRETS_PKCS11_PIN
    key_label: qtestframe-kek
    generate_key: false           # true: skapa en icke-exporterbar AES-256-nyckel om den saknas
```

DEK:en omsluts med `CKM_AES_GCM` av HSM-nyckeln. Programmet måste byggas med `make go-pkcs11`. Med Landlock behöver en filbaserad token som SoftHSM sin tokenkatalog i `hardening.read_write_paths`. För en testtoken: `softhsm2-util --init-token --free --label qtestframe --pin … --so-pin …`.

**Om nyckeln saknas eller är fel** vägrar servern starta med ett fel som anger vilken nyckel den väntar sig (till exempel `the stored data-encryption keys are wrapped by another key: … the configured key is …`). Den faller aldrig tillbaka till klartext.

### Nyckelbyte och omslutning

| Uppgift | Kommando | Verkan |
|---|---|---|
| ny datanyckel | `qtestframe secrets rotate` | skapar en ny DEK, gör den aktiv och krypterar om varje värde; gamla DEK:ar behålls |
| ta bort gamla datanycklar | `qtestframe secrets rotate --prune` | som ovan, och tar sedan bort de DEK:ar som inget värde använder; stoppa eller starta om de andra serverprocesserna först |
| ny KEK, eller annan leverantör | `qtestframe secrets rewrap --config NY.yaml --from-config GAMMAL.yaml` | låser upp DEK:arna med leverantören i `GAMMAL.yaml` och omsluter dem med den i `NY.yaml`; de lagrade värdena ändras inte |

Andra serverprocesser tar en roterad DEK i bruk när de stöter på ett värde som krypterats med den. **Så byter ni KEK eller leverantör** (till exempel en ny lokal nyckelfil, local → TPM, local → HSM):

1. Ta en säkerhetskopia (`qtestframe backup`) och behåll den nuvarande nyckeln.
2. Kopiera konfigurationen: `cp config.yaml config.new.yaml`, och ändra avsnittet `secrets` i kopian (ny `key_file`, eller `provider: tpm` / `pkcs11`).
3. Stoppa servern och kör `qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml`. En ny lokal nyckelfil skapas i det här steget.
4. Gör `config.new.yaml` till konfigurationen och starta servern; kontrollera `qtestframe secrets status`.
5. Behåll den gamla nyckeln tills den nya uppsättningen fungerar – **säkerhetskopior som tagits tidigare kräver den gamla nyckeln**.

### Katastrofåterställning för TPM-förseglade nycklar

En TPM kan inte säkerhetskopieras, och med `pcrs` kan till och med en uppdatering av den inbyggda programvaran göra nyckeln omöjlig att avförsegla. Planera för det:

- **Planerade ändringar** (firmware, Secure Boot-nycklar, startladdare, ett nytt moderkort): omslut först till en lokal nyckel (`secrets rewrap` med en `local`-konfiguration som `--config` och TPM-konfigurationen som `--from-config`), kör med den lokala nyckeln under ändringen, och omslut sedan tillbaka till TPM:en.
- **Oplanerad förlust** (trasigt moderkort, rensad TPM): bara en **återställningskopia** vars DEK:ar är omslutna av en nyckel som ni har någon annanstans hjälper. Gör en regelbundet, och efter varje `secrets rotate`, på TPM-maskinen och som tjänsteanvändaren (som får använda TPM:en):

  ```sh
  # recovery-tpm.yaml:    sqlite /var/lib/qtestframe/recovery.db, secrets som i produktion (tpm)
  # recovery-local.yaml:  samma databas, provider local, key_file på flyttbar eller frånkopplad lagring
  qtestframe backup  --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf.zip
  qtestframe restore --config recovery-tpm.yaml --in /var/backups/qtestframe/qtf.zip --yes
  qtestframe secrets rewrap --config recovery-local.yaml --from-config recovery-tpm.yaml
  qtestframe backup  --config recovery-local.yaml --out /var/backups/qtestframe/qtf-recovery.zip
  rm /var/lib/qtestframe/recovery.db*
  ```

  Förvara `qtf-recovery.zip` och reservnyckeln åtskilda (nyckeln aldrig bredvid säkerhetskopiorna). Återställningen till den tillfälliga databasen fungerar eftersom samma TPM låser upp nycklarna; omslutningen skapar reservnyckelns fil. För att återställa: läs in återställningskopian på en server som är konfigurerad med reservnyckeln (leverantören local), och omslut sedan till den nya TPM:en.
- Om KEK:en är förlorad och det inte finns någon återställningskopia går de lagrade hemligheterna inte att få tillbaka; servern vägrar starta med den databasen.

Samma sak gäller, i svagare form, HSM-nycklar (använd HSM:ens egen nyckelsäkerhetskopiering) och lokala nyckelfiler (säkerhetskopiera dem – skilt från databasen).

## Händelseloggen

Varje organisation har en händelselogg (**Administration → Händelselogg**, behörigheten `audit.read`, API `GET /api/v1/audit`, nyaste först, upp till 5000 poster per begäran): vem (användar-id och e-post, eller `SCIM: <token>`, eller `cli`), vad (`action`, objekt, korta detaljer) och när. Här registreras inloggningar (med metod, och `mfa=…`, `amr`/`acr` för single sign-on), ändringar av projekt, objekt, körningar, medlemmar, roller, nycklar, delning, organisationer och användare, exporter, säkerhetskopiering och återställning (`admin.backup`, `admin.restore`), MFA-återställningar och policyändringar, samt systeminställningar (`settings.update`). Gamla och nya värden för objekt finns i objektens historik, inte i händelseloggen; klientadressen finns i begärandeloggen, inte i posten.

Varje post skrivs också som en loggpost `audit` på nivån INFO. Skicka loggen till en central syslog-server (se [Loggning](#central-syslog)), så att spåret överlever att värden komprometteras. Misslyckade inloggningar syns i serverloggen (`login failed`) och i användarens inloggningshistorik, inte i händelseloggen. Händelseloggen ingår i en säkerhetskopia.

## Säkerhetshuvuden och CSP

Varje svar har:

| Huvud | Värde |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:; object-src 'none'` |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`, med `server.hsts: true` på TLS-anslutningar som servern avslutar |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` |
| `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` | `same-origin` |
| `Cache-Control` | `no-store` för `/api/…` |

CSP:n tillåter inga inbäddade skript och ingen `eval`; inbäddade stilar behövs av komponentbiblioteket, och `form-action https:` av omdirigeringarna för single sign-on. En omvänd proxy bör släppa igenom huvudena oförändrade; Qtestframe kan inte bäddas in i ramar på andra webbplatser.

Andra gränser: JSON-innehåll högst 8 MiB, huvuden 1 MiB, `ReadHeaderTimeout` 10 s, `IdleTimeout` 120 s; frågor högst 4096 byte med RE2-reguljära uttryck (ingen katastrofal backtracking).

## Utgående anslutningar

Projektadministratörer (`project.update`) väljer adresserna till ärendesystem och webhooks, och skriptförfattare (`script.execute`) målen för webbtester; superadministratörer kan få servern att ansluta till adresser med testknapparna i systeminställningarna. När den valfria [tjänsten för inbäddningar](#tjänst-för-inbäddningar-för-liknande-objekt) är på skickar servern texten i krav och testfall till den. Alla sådana begäranden går ut från serverns nät. Om servern når interna tjänster: filtrera dess utgående trafik med brandvägg eller proxy (`deploy/nftables` har en valfri policy för utgående trafik). Leveranser av webhooks följer aldrig omdirigeringar.

## Checklista

- [ ] TLS-certifikat från en betrodd CA; `server.hsts: true`; `auth.cookie_secure: true`; `server.base_url` på `https://`
- [ ] single sign-on konfigurerad; `auth.local: false`, eller få lokala konton med starka lösenord
- [ ] MFA krävs åtminstone för administratörer (MFA-policyn, eller hos identitetsleverantören med `local_mfa` satt därefter)
- [ ] hemligheter i `*_file`-inställningar eller `LoadCredential=`, filer `0640 root:qtestframe`
- [ ] nyckeln till de lagrade hemligheterna säkerhetskopierad skilt från databasen (eller TPM/HSM med en återställningsplan); `secrets status` visar inga klartexthemligheter
- [ ] PostgreSQL med `sslmode=verify-full` och en egen roll
- [ ] loggar skickas till en central syslog-server över `tls`
- [ ] `runner.enabled: false` om det inte behövs; annars bara betrodda personer med `script.execute`, MAC tillämpad, utgående trafik filtrerad
- [ ] SELinux-modul eller AppArmor-profil inläst; `systemd-analyze security qtestframe` ≤ 2.0
- [ ] `server.trusted_proxies` satt bakom en omvänd proxy
- [ ] säkerhetskopior krypterade, lagrade utanför värden och en återställning provad

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
| Testkörningar | `run.created` (Testkörning skapad), `run.updated` (Testkörning ändrad), `run.completed` (Testkörning avslutad), `execution.updated` (Testresultat registrerat) |
| Defekter | `defect.created` (Defekt rapporterad), `defect.updated` (Defekt ändrad), `defect.status` (Defektens status ändrad), `defect.assigned` (Defekt tilldelad), `defect.regression` (Regression upptäckt) |
| Krav och tester | `requirement.created` (Krav skapat), `requirement.updated` (Krav ändrat), `test.created` (Test skapat), `test.updated` (Test ändrat), `comment.created` (Ny kommentar), `comment.mentioned` (Comment mentioned), `comment.resolved` (Comment resolved), `baseline.signed` (Baslinje signerad) |
| Granskningar | `review.requested` (Granskning begärd), `review.completed` (Granskning klar) |
| Releaser | `release.created` (Release skapad), `release.updated` (Release ändrad), `release.released` (Release släppt) |
| Undantag | `waiver.requested` (Undantag begärt), `waiver.approved` (Undantag godkänt), `waiver.rejected` (Undantag avslaget), `waiver.revoked` (Undantag återkallat) |
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

Projekt kan skicka defekter och krav till **Jira**, **GitHub**, **GitLab** eller **Request Tracker (RT)**, synkronisera deras status tillbaka och importera ärenden. Användare med `project.update` ansluter ett projekt i dess inställningar ([användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/defects.md#anslut-jira-github-eller-gitlab)); att skicka och synkronisera kräver `run.execute`, ärenden från krav `requirement.write`.

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

Användare med `requirement.write` importerar krav från en JQL-fråga i Jira eller en WIQL-fråga i Azure DevOps ([användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/reports-and-export.md#jira-och-azure-devops)); anslutningarna ställs in av användare med `project.update` under **Projekt → Importkällor** och lagras i `projects.import_sources` (migrering 0037).

| Källa | Adress | Inloggning | Minsta användbara omfattning |
|---|---|---|---|
| Jira Cloud | `https://<webbplats>.atlassian.net` (eller ingen: projektets Jira-ärendehanterare används) | användarens e-post + API-token (HTTP Basic) | *Browse projects* i de projekt som frågas |
| Jira Server / Data Center | er Jira-adress | personlig åtkomsttoken (Bearer) | som ovan |
| Azure DevOps Services | `https://dev.azure.com/<organisation>` och projektets namn | personlig åtkomsttoken (HTTP Basic med tomt användarnamn) | omfattningen *Work Items (Read)*, begränsad till den enda organisationen |
| Azure DevOps Server | samlingens adress, t.ex. `https://tfs.example.com/tfs/DefaultCollection` | personlig åtkomsttoken | som ovan |

Jira söks med `POST /rest/api/2/search/jql` (Jira Cloud) och, när den inte finns, `POST /rest/api/2/search` (Server och Data Center); Azure DevOps med slutpunkterna för WIQL, fält och arbetsobjekt i REST API 7.1. Högst 1 000 objekt läses per import. Importen läser bara; den granskas som `requirement.import`, ändringar av anslutningen som `import_sources.update`. Token krypteras med serverns hemliga nycklar (de krypteras om vid nyckelrotation), returneras aldrig och utelämnas ur projektarkiv. Importen når källorna via serverns utgående proxy (`HTTPS_PROXY`, `NO_PROXY`) som ärendehanterarna; tillåt adresserna ovan i en brandvägg.

## CI/CD-pipelines (GitLab, GitHub Actions)

Pipelines pratar med servern med kommandot `qtestframe` (`report`, `gate`, `select`; [kommandoradsreferensen](#kommandoradsreferens)) och en API-nyckel. Inget behöver ställas in på servern; det här behöver varje del:

- **API-nyckel.** Skapa en egen CI-användare, gör den till medlem med rollen **tester** i projekten den rapporterar till (tester har `run.execute` för uppladdningar, `run.write` för `--create-run` och `test.write` för `--create-missing`), och skapa dess API-nyckel med rollen *tester* under **Administration → API-nycklar**. En nyckel får aldrig mer än användarens aktuella roll. Spara den som en maskerad CI/CD-variabel `QTF_TOKEN`. Det finns inga finare behörigheter för API-nycklar än roller; en egen roll med bara `project.read`, `test.read`, `run.read`, `run.execute` och `run.write` är den snävaste som kör alla kommandon.
- **Programmet.** Jobben behöver en avbildning med det statiska programmet `qtestframe` (`QTF_CLI_IMAGE`); serverns avbildning är distroless, så kopiera programmet till din CI-avbildning (`COPY --from=qtestframe:<version> /usr/bin/qtestframe /usr/local/bin/`).
- **Mallar.** `examples/ci/` innehåller mallar för `.gitlab-ci.yml` (med och utan testurval), en CI/CD-komponent och ett arbetsflöde för GitHub Actions; samma mallar, med serverns adress och projektnyckeln ifyllda, finns under **Inställningar → Projekt → CI-uppsättning** och på den publika `GET /api/v1/ci/templates/{name}` (för `include: remote:`; de innehåller inga hemligheter).
- **Commit-statusar och anteckningar i merge requests från pipelinen** behöver en GitLab-token i `QTF_GITLAB_TOKEN`: en projektåtkomsttoken med rollen **Developer** (commit-statusar kräver Developer, anteckningar Reporter) och behörigheten `api`. GitLab godtar när detta skrivs inte jobbtoken `CI_JOB_TOKEN` för `POST /projects/:id/statuses/:sha` eller anteckningar i merge requests (dess finkorniga behörigheter för jobbtoken täcker dem inte); kommandot faller tillbaka på den och skriver en varning när GitLab avvisar den. I GitHub Actions räcker arbetsflödets `GITHUB_TOKEN` med `statuses: write` och `pull-requests: write` (`QTF_GITHUB_TOKEN`).
- **Rapporter från servern.** Ett projekt kan i stället låta servern skicka: **Inställningar → Projekt → Resultat tillbaka till GitLab** sparar GitLabs API-adress, projektet och en projektåtkomsttoken (krypterad som de andra hemligheterna, bara skrivbar, omkrypteras av `qtestframe secrets`, utelämnas ur projektarkiv). När resultat med `?commit=` kommer in läggs en rad i utkorgen `ci_deliveries` och skickas i bakgrunden: `qtestframe/tests` (och `qtestframe/gate` med en release eller villkor) och en anteckning i merge requesten som uppdateras i stället för att upprepas. Misslyckade försök görs om av minutjobbet med exponentiell väntan (1, 2, 4, … minuter, högst 6 försök; vid HTTP 429 väntar den så länge GitLab ber om); en avvisad token eller ett avvisat projekt (övriga 4xx) försöks inte igen. De 50 senaste rapporterna per projekt visas i inställningarna; `ci_deliveries` säkerhetskopieras inte. Servern behöver utgående HTTPS till GitLab-instansen.

## Körning av skript och webbtester

Körningen utför skripten i automatiska tester (`sh`, `bash`, `python`, `javascript`) och webbtester (Playwright) på servern. Den är **av som standard**; läs [avsnittet om sandlådan](#sandlådan-för-skript-och-webbtester) innan den slås på. Inställningarna gäller efter en omstart.

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

Containeravbilden kräver `--build-arg WITH_PLAYWRIGHT=true` och `QTF_RUNNER_ENABLED=true`, `QTF_RUNNER_WEB_ENABLED=true` (se [Installation](#i-en-container-docker-eller-podman)).

# Koppla ihop med andra verktyg (OSLC och ReqIF)

Qtestframe utbyter krav och testfall med ALM- och kravverktyg på två sätt (användarhandbokens sida *Samverkan* beskriver vad användarna ser):

- **OSLC**, direkt: Qtestframe är en OSLC-leverantör för Requirements Management 2.1 (krav) och Quality Management 2.1 (testfall). Andra verktyg hittar den, ställer frågor, skapar, ändrar och tar bort poster, länkar sina egna poster till dem och visar Qtestframes urvals- och skapadialoger för sina användare.
- **ReqIF-filer**, för hela specifikationer: export från ett verktyg, import i det andra, gång på gång. Inget behöver ställas in på servern.

Det här kapitlet handlar om att sätta upp OSLC-kopplingar till IBM Engineering Lifecycle Management (DOORS Next, Engineering Test Management), Siemens Polarion och PTC Codebeamer, och om vad du kontrollerar när en koppling inte fungerar.

## Det här erbjuder servern

| Adress | Användning |
|---|---|
| `<base_url>/api/v1/oslc/rootservices` | Upptäcktsdokument i Jazz-stil (öppet, innehåller inga data). IBM ELM och verktyg som använder "friends" börjar här. |
| `<base_url>/api/v1/oslc/catalog` | Katalog över tjänsteleverantörer: en per projekt som kontot får läsa. |
| `<base_url>/api/v1/oslc/projects/<projekt-id>/catalog` | En katalog med bara ett projekt, för verktyg som kopplar ett av sina projekt till ett projekt här. |
| `<base_url>/api/v1/oslc/projects/<projekt-id>` | Projektets tjänsteleverantör: frågefunktioner, creation factories, urvals- och skapadialoger, resource shapes. |

Projekt-id:t är det i projektets webbadress (`/p/<id>/…`). Resurser skickas som RDF/XML (OSLC:s standard), Turtle eller JSON-LD; innehåll i begäranden tas emot i samma format. API-referensen (`docs/api.md`, avsnittet *OSLC*) listar alla egenskaper, länkegenskaperna och frågesyntaxen.

## Innan du kopplar in ett verktyg

1. **Bas-URL.** Ställ in `server.base_url` (eller **Bas-URL** i *Systeminställningar*) på adressen som det andra verktyget når Qtestframe på, till exempel `https://qtf.example.com`. Varje OSLC-resursadress byggs av den; bakom en omvänd proxy måste det vara den publika adressen, inte den interna.
2. **TLS.** De flesta ALM-servrar vägrar vanlig HTTP och certifikat som de inte litar på. Använd ett certifikat från en CA som det andra verktygets Java-förtroendelager känner till (eller lägg till din CA där).
3. **Ett tjänstekonto och en API-nyckel.** OSLC-klienter loggar in med en API-nyckel, antingen som `Authorization: Bearer <nyckel>` eller som lösenord i Basic-autentisering (valfritt användarnamn), bara på OSLC-sökvägarna. OAuth 1.0a, som IBM ELM använder mellan Jazz-program, stöds inte; använd anslutningstypen "custom" eller "basic" i det andra verktyget med nyckeln. Skapa en användare (till exempel `alm-connector@example.com`) och ge den rollen som kopplingen behöver:
   - **Läsare** för att låta verktyget läsa och länka (länken sparas i det andra verktyget; Qtestframe visar den där).
   - En roll med `requirement.write` och/eller `test.write` för att låta verktyget skapa och ändra poster, och `test.write` för att ändra spårlänkar (*validatedBy* / *validatesRequirement*).
   Logga in som den användaren, skapa nyckeln under **Administration → API-nycklar** med rollen som kopplingen behöver (en nyckel kan ha lägre roll än sitt konto, aldrig högre) och ge den till administratören för det andra verktyget. Begränsade projekt syns bara när kontot är medlem.
4. **Dialoger i en ram** (valfritt). Urvals- och skapadialogerna öppnas i ett eget fönster, vilket fungerar överallt. Verktyg som visar dem inne på sin egen sida (en iframe) behöver två saker:
   - verktygets ursprung i `server.frame_ancestors`, till exempel
     ```yaml
     server:
       base_url: https://qtf.example.com
       frame_ancestors:
         - https://elm.example.com
         - https://polarion.example.com
     ```
     Dialogsidorna skickar då `Content-Security-Policy: frame-ancestors 'self' https://elm.example.com …`; utan inställningen får bara Qtestframe självt visa dem i en ram.
   - webbläsaren måste skicka Qtestframes sessionskaka inne i ramen. Kakan är `SameSite=Lax`, så ramen fungerar när båda verktygen ligger på **samma webbplats** (underdomäner till samma registrerbara domän, som `qtf.example.com` och `elm.example.com`). För verktyg på en annan webbplats låter du dialogerna öppnas i ett fönster.
   Dialogerna använder användarens egen session: varje användare loggar in i Qtestframe i webbläsaren (dialogen visar en länk om de inte har gjort det).

## IBM DOORS Next och Engineering Test Management

DOORS Next länkar sina krav till testfall hos en QM-leverantör, och ETM länkar sina testfall till krav hos en RM-leverantör. Qtestframe kan ha RM-rollen (sina krav) och QM-rollen (sina testfall).

1. Lägg till en association i ELM:s **Lifecycle Project Administration** (eller projektområdets **Associations**) till en **OSLC-leverantör som inte är ett Jazz-program**: välj *Add → Service provider catalog* (ordalydelsen varierar mellan versioner) och ange `<base_url>/api/v1/oslc/catalog`, eller `<base_url>/api/v1/oslc/projects/<id>/catalog` för ett projekt. När versionen bara tar emot friends registrerar du en friend med rotdokumentet `<base_url>/api/v1/oslc/rootservices` och väljer Basic-autentisering med tjänstekontots e-postadress och API-nyckeln som lösenord.
2. Välj associationstyp: **Provides Requirements** (eller *Uses Requirements* från ETM-sidan) för krav i Qtestframe, **Provides Test Cases** (*Validated By*) för testfall i Qtestframe.
3. I en artefakt i DOORS Next erbjuder **Links → Add link → Validated By** nu *Qtestframe* som mål: urvalsdialogen öppnas, användaren söker och markerar testfall, och DOORS Next sparar länkarna. **Create** öppnar Qtestframes skapadialog i stället.

Länkar som DOORS Next sparar finns i DOORS Next. När ett verktyg också skriver länken i Qtestframe-resursen (en PUT med `oslc_qm:validatesRequirement` eller `oslc_rm:validatedBy`) behåller Qtestframe den som en spårlänk när den andra änden är en av dess egna poster, annars som en länk till det andra verktygets adress.

## Siemens Polarion

Polarion länkar arbetsposter till OSLC-resurser på andra servrar genom *OSLC-anslutningar*:

1. Lägg till en leverantör i **Administration → Linked data / OSLC → Remote OSLC providers** (Polarions konfiguration av *Connections*) med katalogen `<base_url>/api/v1/oslc/catalog`, autentisering **Basic**, tjänstekontots e-postadress som användare och API-nyckeln som lösenord.
2. Mappa länkrollerna: ett krav i Polarion *is validated by* → testfall i Qtestframe (`oslc_qm:TestCase`), ett testfall i Polarion *verifies* → krav i Qtestframe (`oslc_rm:Requirement`).
3. Polarions användare väljer sedan poster i Qtestframe i dialogen **Link to OSLC resource**, som öppnar Qtestframes urvalsdialog.

För en enstaka eller regelbunden överföring av ett Polarion-dokument använder du ReqIF i stället: exportera dokumentet som ReqIF i Polarion och importera det i Qtestframe med *Importera → ReqIF … med uppdateringar*; mappa arbetsposttyperna (till exempel *Test Case* till testfall) i förhandsgranskningen av importen.

## PTC Codebeamer

Codebeamer ansluter till OSLC-leverantörer som *Remote trackers / OSLC associations*:

1. Lägg till en server i projektets **Admin → Integrations → OSLC** med rotdokumentets eller katalogens adress ovan, autentisering **Basic** med tjänstekontot och API-nyckeln.
2. Välj fjärrtjänst: *Requirements* (krav i Qtestframe) eller *Test cases* (testfall i Qtestframe), och vilken associationstyp i Codebeamer den ska skapa (till exempel *verifies* eller *related*).
3. Codebeamers associationsdialog listar sedan poster i Qtestframe; de delegerade dialogerna används där Codebeamer stöder dem.

Codebeamer exporterar och importerar också ReqIF; dess trackers av typen *Test Case* mappas till testfall i förhandsgranskningen av importen.

## Andra OSLC-klienter och skript

Alla OSLC 2.0-klienter fungerar med katalogen och en nyckel. En snabb kontroll från kommandoraden:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Accept: text/turtle" \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements?oslc.where=qtf:status=%22approved%22&oslc.select=dcterms:title"
```

Skapa ett krav:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: text/turtle" -H "Accept: text/turtle" \
  --data-binary '<> a <http://open-services.net/ns/rm#Requirement> ; <http://purl.org/dc/terms/title> "Nödstopp" .' \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements"
```

Svaret är `201 Created` med den nya adressen i `Location` och versionen i `ETag`. En uppdatering (PUT) med `If-Match: "<ETag>"` nekas med `412 Precondition Failed` om någon har sparat under tiden, som i webbklienten.

## Det här sparas, loggas och säkerhetskopieras

- Poster som skapas, ändras eller tas bort via OSLC är vanliga poster: de har historik, ändringarna står i granskningsloggen under nyckelns konto, och borttagning flyttar dem till projektets papperskorg.
- Spårlänkar, relationer och länkar som skrivs via OSLC är vanliga länkar; länkar till resurser i andra verktyg är länkar till deras URL, med OSLC-egenskapen som etikett (`oslc_rm:implementedBy` …).
- ReqIF-importen sparar var varje post, relation och spårlänk kom ifrån (tabellen `external_refs`), så att en ny import av samma dokument uppdaterar dem. Tabellen ingår i säkerhetskopior och projektarkiv.
- Inget om en koppling sparas i Qtestframe utöver API-nyckeln: det andra verktyget har kopplingens inställningar.

## Felsökning

| Symptom | Kontrollera |
|---|---|
| Verktyget säger att katalogen är tom | Kontot är inte medlem i projektet (begränsade projekt), eller nyckelns roll saknar läsbehörighet. Öppna katalogadressen med nyckeln i en webbläsare eller med `curl`. |
| Länkar pekar på `http://localhost:…` eller en intern värd | `server.base_url` är inte den publika adressen. |
| 401 Unauthorized | Nyckeln är fel, har gått ut eller har återkallats; Basic-autentisering måste ha nyckeln som lösenord. Vanliga lösenord tas inte emot på OSLC-sökvägarna. |
| 403 Forbidden vid skapa eller ändra | Nyckelns roll saknar `requirement.write` / `test.write`, eller `test.write` för spårlänkar. |
| 412 Precondition Failed | Posten har ändrats sedan verktyget läste den; verktyget måste läsa den igen (dess ETag är postens version). |
| 415 Unsupported Media Type | Verktyget skickar innehåll som inte är Turtle, RDF/XML eller JSON-LD. |
| Dialogen förblir tom i en ram | Verktygets ursprung finns inte i `server.frame_ancestors`, eller verktygen ligger på olika webbplatser så att webbläsaren inte skickar sessionskakan: använd dialogen i ett fönster. |
| Dialogen säger *Du är inte inloggad* | Användaren har ingen session i Qtestframe i den här webbläsaren: logga in i en annan flik och välj **Försök igen**. |

# Loggning och övervakning

## Serverloggen

Servern skriver strukturerade loggposter (Go `log/slog`) till **standard error**; under systemd hamnar de i journalen (`journalctl -u qtestframe -f`, `-o json` för fälten). Två format:

| `logging.format` | Exempel |
|---|---|
| `text` (standard) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` är `debug`, `info` (standard), `warn` eller `error`. Nivå och format kan ändras under drift under **Administration → Systeminställningar → Loggning och syslog**, om inte `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` sätter dem (containeravbilden sätter formatet till `json`).

Det här loggas:

- **Varje begäran** utom `/healthz` och `/readyz`: metod, sökväg (utan frågesträng), status, byte, tid och klientadress (från `X-Forwarded-For` bara för `trusted_proxies`). Begäranden som besvaras med 5xx loggas på nivån ERROR, övriga på INFO.
- **Granskningsposter** (`msg=audit`, INFO) med organisation, användare, åtgärd, objekt och id, en för varje post i händelseloggen.
- **Säkerhetshändelser**: `login failed` och `second factor failed` (WARN, med e-post och klientadress), problem med konfiguration och nycklar vid start, raden `hardening`.
- **Bakgrundsarbete**: migreringar, rensade sessioner och papperskorgsposter, mätpunkter, fel vid e-post och webhooks, återställda jobb.

Lösenord, token, kakor och `Authorization`-huvuden loggas aldrig. Oväntade fel besvaras bara med "internal server error"; detaljerna finns i loggraden `request failed` (sökväg och fel).

## Central syslog

Varje post – även granskningsposterna – kan också skickas till en syslog-server, som ett JSON-meddelande per post:

```yaml
logging:
  level: info
  format: json
  syslog:
    enabled: true
    network: tls                 # udp | tcp | tls | unix | unixgram
    address: "logs.example.com:6514"
    tag: qtestframe
    facility: local0
    format: rfc5424              # eller rfc3164
    ca_file: /etc/qtestframe/tls/ca.crt
    cert_file: /etc/qtestframe/tls/syslog-client.crt   # valfritt, ömsesidig TLS
    key_file: /etc/qtestframe/tls/syslog-client.key
```

- Strömmande transporter (`tcp`, `tls`, `unix`) använder oktetträkning enligt RFC 6587; `tls` följer RFC 5425 och verifierar servern mot `ca_file` (eller systemets rotcertifikat) med TLS 1.2 eller senare. Ett klientcertifikat skapas med `scripts/certs/gen-client-cert.sh`.
- Allvarlighetsgrader: ERROR → 3, WARN → 4, INFO → 6, DEBUG → 7. Meddelandets APP-NAME är `tag`.
- Leveransen sker efter bästa förmåga: ett nytt anslutningsförsök per meddelande, en skrivgräns på 5 sekunder, ingen kö på disk. Journalen har den lokala kopian.
- Med SELinux: `setsebool -P qtestframe_remote_syslog on`.
- **Skicka testmeddelande** i systeminställningarna skickar ett meddelande med värdena i formuläret.

Exempel på mottagare för rsyslog (`imtcp` med `gtls`) och syslog-ng (`network(transport("tls") flags(syslog-protocol))`) finns i [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#8-logging-and-central-syslog) (på engelska). JSON-innehållet kan delas upp i fält med rsyslog `mmjsonparse` eller syslog-ng `json-parser()`.

## Hälsokontroller

| Adress | Svarar | Användning |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok","version":"…"}` så länge processen besvarar begäranden | liveness-kontroll |
| `GET /readyz` | `200 {"status":"ready"}`, eller `503` när databasen inte svarar inom 2 sekunder | readiness-kontroll, lastbalanserare |

Båda kräver ingen inloggning och skrivs inte i begärandeloggen. `qtestframe healthcheck` kontrollerar `/healthz` på den lokala servern för hälsokontroller i containrar.

Det finns **ingen adress för mätvärden** (ingen Prometheus-exporter). *Trenderna* på översiktspanelen är projektdata (dagliga mätpunkter), inte servermätvärden.

## Vad som bör övervakas

| Signal | Hur |
|---|---|
| liveness och readiness | `/healthz`, `/readyz` (t.ex. Prometheus `blackbox_exporter`, modulen `http_2xx`) |
| tjänstens tillstånd | `systemctl is-active qtestframe`; enheten startar om servern vid fel |
| fel | loggposter med `level=ERROR`; `stored system settings are invalid`; `tls reload failed` |
| angrepp | många `login failed`, `second factor failed` eller misslyckade SCIM-autentiseringar från en adress |
| certifikatets giltighet | `scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21` (avslutningskod 2 = går ut snart) |
| resurser | `systemctl status qtestframe` (minne och processer mot `MemoryMax`/`TasksMax`), ledigt diskutrymme för databasen och säkerhetskopiorna |
| säkerhetskopior | åldern på den senaste säkerhetskopian och på det senaste återställningstestet |
| nyckeln till de lagrade hemligheterna | `qtestframe secrets status` visar inga klartexthemligheter och den förväntade nyckeln |

Ett enkelt alternativ med cron till ett övervakningssystem:

```sh
curl -fsS --max-time 5 https://qtf.example.com/readyz >/dev/null || echo "qtestframe svarar inte" | mail -s LARM ops@example.com
```

# Säkerhetskopiering, återställning och migrering

## Vad som ska säkerhetskopieras

| Vad | Var | Hur |
|---|---|---|
| **data** | databasen | `qtestframe backup` (logisk, rekommenderas), eller en säkerhetskopia av själva databasen |
| **nyckeln till de lagrade hemligheterna** | nyckelfilen (`qtestframe-secrets.key`), `QTF_SECRETS_KEY`, HSM-nyckeln; en TPM kan inte säkerhetskopieras | separat – **aldrig på samma ställe som databasens säkerhetskopior** |
| konfigurationen | `/etc/qtestframe` (konfiguration, hemlighetsfiler, TLS- och SAML-nycklar) | er konfigurationshantering eller en krypterad säkerhetskopia |

Uppladdade filer (bilagor, dokument, rapporter, avatarer) lagras i databasen, så en säkerhetskopia av databasen innehåller dem. Körningens arbetskatalog är tillfälligt utrymme.

En säkerhetskopia utan nyckeln är oanvändbar för de krypterade hemligheterna; en säkerhetskopia tillsammans med nyckeln ger allt till den som har båda. Kryptera säkerhetskopior och förvara dem utanför värden.

## Vad en säkerhetskopia innehåller

`qtestframe backup` skriver en **logisk säkerhetskopia**: en ZIP-fil med `manifest.json` (formatversion, schemaversion, tidpunkt, Qtestframe-version, databasdrivrutin, antal rader per tabell) och en fil `<tabell>.jsonl` per tabell, ett JSON-objekt per rad. Tider, booleska värden och binärdata skrivs oberoende av databas, så en säkerhetskopia från SQLite kan återställas i PostgreSQL och tvärtom. Den innehåller **alla organisationer**: användare och lösenordshashar, medlemskap, projekt med alla objekt, historik och filer, händelseloggen, API- och SCIM-nycklar (som hashar), MFA-inställningarna, systeminställningarna och de omslutna datakrypteringsnycklarna. Säkerhetskopian är en konsistent ögonblicksbild (en läsande transaktion).

<!-- BEGIN GENERATED: backup-tables (make admin-guide-ref) -->

**I en säkerhetskopia** (94 tabeller, i den ordning de återställs): `tenants`, `users`, `memberships`, `api_tokens`, `projects`, `configurations`, `releases`, `folders`, `categories`, `requirements`, `requirement_categories`, `evaluation_criteria`, `test_cases`, `test_steps`, `test_case_categories`, `trace_links`, `variants`, `builds`, `test_cycles`, `test_cycle_tests`, `test_runs`, `run_configurations`, `test_executions`, `execution_iterations`, `step_results`, `audit_log`, `revisions`, `requirement_relations`, `baselines`, `baseline_items`, `comments`, `files`, `file_blobs`, `attachments`, `report_definitions`, `login_events`, `roles`, `test_dependencies`, `trash`, `item_links`, `release_requirements`, `defects`, `defect_links`, `risks`, `risk_requirements`, `notifications`, `webhooks`, `review_requests`, `baseline_signoffs`, `scim_tokens`, `scim_settings`, `scim_users`, `scim_groups`, `scim_group_members`, `metric_snapshots`, `trace_rules`, `document_items`, `defect_regressions`, `consistency_dismissals`, `invitations`, `report_subscriptions`, `user_mfa`, `mfa_recovery_codes`, `mfa_policies`, `project_members`, `waivers`, `waiver_requirements`, `waiver_signoffs`, `soa_decisions`, `system_settings`, `external_refs`, `link_criteria`, `system_elements`, `requirement_elements`, `test_elements`, `verification_settings`, `verification_records`, `verification_record_files`, `item_applicability`, `code_changes`, `code_change_items`, `code_settings`, `custom_fields`, `field_set_fields`, `custom_field_archive`, `item_templates`, `org_item_templates`, `project_template_defaults`, `exploratory_sessions`, `exploratory_scope`, `exploratory_notes`, `test_cycle_schedules`, `secret_keys`, `chat_channels`.

**Inte i en säkerhetskopia:** `chat_deliveries`, `ci_deliveries`, `job_runs`, `mfa_challenges`, `mfa_trusted_devices`, `schema_migrations`, `script_jobs`, `sessions`, `similarity_embeddings`, `suggest_history`.

<!-- END GENERATED: backup-tables -->

En återställning loggar alltså ut alla (sessioner), tar bort påbörjade MFA-inloggningar och ihågkomna enheter, tömmer varje användares **historik för autokomplettering** (en personlig bekvämlighet som medvetet inte säkerhetskopieras), och glömmer köade bakgrundsjobb. **Nyckeln** till de lagrade hemligheterna finns aldrig i en säkerhetskopia: de lagrade hemligheterna (TOTP-hemligheter, webhook-hemligheter, ärendesystemtoken, hemliga testvariabler, systeminställningarnas SMTP-lösenord och OIDC-klienthemligheter) förblir **krypterade** i den, tillsammans med sina omslutna datanycklar (`secret_keys`).

En säkerhetskopiering vägrar köra medan databasen har en tabell som den inte känner till (ett skydd under utvecklingen).

## Ta en säkerhetskopia

```sh
sudo -u qtestframe qtestframe backup --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf-$(date +%F).zip
```

Den kan köras medan servern kör. Filen skrivs under ett tillfälligt namn och byter namn när den är klar, och säkerhetskopieringen granskas (`admin.backup`). Superadministratörer kan också ladda ned en under **Administration → Säkerhetskopiering** (`GET /api/v1/admin/backup`); en nedladdning via webbläsaren begränsas av `server.write_timeout`, så använd kommandoraden för stora installationer. Med SQLite är servern upptagen medan en säkerhetskopia strömmas.

En daglig timer:

```ini
# /etc/systemd/system/qtestframe-backup.service
[Service]
Type=oneshot
User=qtestframe
ExecStart=/bin/sh -c '/usr/bin/qtestframe backup --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf-$(date +%%F).zip'

# /etc/systemd/system/qtestframe-backup.timer
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
```

Katalogen måste finnas och vara skrivbar för `qtestframe`. Kopiera filerna till en annan värd (restic, borg) och ta bort gamla.

## Återställning

En återställning **ersätter alla data** i installationen med säkerhetskopian, i **en transaktion**: varje tabell töms och fylls på nytt, så antingen återställs allt eller så ändras ingenting.

```sh
sudo systemctl stop qtestframe
sudo -u qtestframe qtestframe restore --config /etc/qtestframe/config.yaml --in /var/backups/qtestframe/qtf-2026-11-19.zip --yes
sudo systemctl start qtestframe
```

Kommandot skriver ut antalet rader per tabell och granskas (`admin.restore`). Superadministratörer kan också återställa under **Administration → Säkerhetskopiering** (`POST /api/v1/admin/restore`, bekräftas genom att skriva `RESTORE`); uppladdningen begränsas av `server.read_timeout`, så använd kommandoraden för stora säkerhetskopior. Stoppa servern för en återställning på kommandoraden: en server som kör behåller sina nycklar och cachade data från före återställningen.

En återställning avvisas, utan att något ändras, när:

- säkerhetskopians **schemaversion** skiljer sig från databasens. Återställ med den Qtestframe-version som gjorde säkerhetskopian (manifestet anger den) och uppgradera sedan; eller uppgradera den gamla installationen först och säkerhetskopiera igen;
- serverns **nyckel inte kan låsa upp** säkerhetskopians datanycklar (`the backup's encrypted secrets cannot be decrypted with this server's key`). Konfigurera nyckeln som säkerhetskopian gjordes med – samma nyckelfil eller `QTF_SECRETS_KEY`, samma TPM, samma HSM-nyckel – eller se [katastrofåterställning](#katastrofåterställning-för-tpm-förseglade-nycklar);
- arkivet är skadat eller ofullständigt, eller större än 4 GiB okomprimerat.

Säkerhetskopians systeminställningar gäller direkt. Efter en återställning: kontrollera `qtestframe secrets status` och logga in.

Använd historiken i webbklienten för att återställa ett enskilt objekt; för att flytta ett projekt, se nedan.

## Uppgradering

1. Läs versionsnyteterna.
2. Ta en säkerhetskopia och kontrollera att ni har nyckeln.
3. Installera den nya programfilen (`sudo scripts/install.sh --binary bin/qtestframe` behåller konfigurationen och installerar om enhet och policyer), eller hämta eller bygg den nya avbilden.
4. Starta om: `sudo systemctl restart qtestframe`. Enhetens `ExecStartPre` kör `qtestframe migrate`, och `serve` migrerar också, innan den nya versionen besvarar begäranden. I compose körs tjänsten `migrate` först.
5. Kontrollera `journalctl -u qtestframe -b` (`applied database migrations count=…`) och `curl -fsk https://localhost:8443/readyz`.
6. Jämför `deploy/config.example.yaml` med er konfiguration för nya inställningar; `check-config` avvisar nycklar som inte längre finns.

Med PostgreSQL och flera serverprocesser: **uppgradera alla samtidigt** – kör migreringen en gång och starta sedan den nya versionen överallt. En äldre version kan inte läsa det en nyare har skrivit.

## Migreringar

Databasschemat skapas och uppgraderas av numrerade migreringar som är inbäddade i programfilen (`internal/db/migrations`). Var och en tillämpas en gång, i en egen transaktion, och registreras i `schema_migrations`; det högsta numret är den *schemaversion* som säkerhetskopior anger.

<!-- BEGIN GENERATED: migrations (make admin-guide-ref) -->

| Version | Migrering |
|---|---|
| 1 | `0001_init.sql` |
| 2 | `0002_history_relations_baselines.sql` |
| 3 | `0003_files_reports_logins.sql` |
| 4 | `0004_roles_settings_dependencies.sql` |
| 5 | `0005_project_color_sources.sql` |
| 6 | `0006_trash.sql` |
| 7 | `0007_project_type.sql` |
| 8 | `0008_item_links.sql` |
| 9 | `0009_suspect_links.sql` |
| 10 | `0010_releases.sql` |
| 11 | `0011_defects.sql` |
| 12 | `0012_risks.sql` |
| 13 | `0013_notifications.sql` |
| 14 | `0014_reviews.sql` |
| 15 | `0015_email.sql` |
| 16 | `0016_configurations.sql` |
| 17 | `0017_trace_rules.sql` |
| 18 | `0018_metrics.sql` |
| 19 | `0019_scim.sql` |
| 20 | `0020_catalog.sql` |
| 21 | `0021_report_language.sql` |
| 22 | `0022_folder_color.sql` |
| 23 | `0023_web_tests.sql` |
| 24 | `0024_project_access.sql` |
| 25 | `0025_mfa.sql` |
| 26 | `0026_system_settings.sql` |
| 27 | `0027_secret_keys.sql` |
| 28 | `0028_item_link_issues.sql` |
| 29 | `0029_suggest_history.sql` |
| 30 | `0030_verification.sql` |
| 31 | `0031_change_defect_correlations.sql` |
| 32 | `0032_waivers_residual_soa.sql` |
| 33 | `0033_consistency_quality.sql` |
| 34 | `0034_custom_fields.sql` |
| 35 | `0035_external_refs.sql` |
| 36 | `0036_variants_code.sql` |
| 37 | `0037_import_sources.sql` |
| 38 | `0038_owners_worklist.sql` |
| 39 | `0039_discussions.sql` |
| 40 | `0040_exploratory.sql` |
| 41 | `0041_test_cycles.sql` |
| 43 | `0043_similarity.sql` |
| 45 | `0045_chat_channels.sql` |
| 46 | `0046_variant_gates.sql` |
| 47 | `0047_risk_catalog.sql` |
| 49 | `0049_risk_attack.sql` |
| 50 | `0050_project_creator.sql` |
| 51 | `0051_frameworks.sql` |
| 52 | `0052_multiple_roles.sql` |
| 53 | `0053_ci_integration.sql` |

<!-- END GENERATED: migrations -->

## Nedgradering

Migreringar går bara framåt. För att gå tillbaka till en tidigare version: återställ säkerhetskopian som togs före uppgraderingen **med den tidigare programfilen**; data som skrivits sedan dess går förlorade. Starta inte en äldre programfil mot en databas som en nyare har migrerat.

Versioner från före krypteringen av lagrade hemligheter (schemaversion under 27) läser krypterade värden som felaktiga hemligheter. Innan ni går tillbaka till en sådan utan säkerhetskopia, kör

```sh
qtestframe secrets decrypt --yes --config /etc/qtestframe/config.yaml
```

som lagrar alla hemligheter i klartext igen (en senare start av en aktuell version krypterar dem på nytt).

## Flytt mellan SQLite och PostgreSQL

Den logiska säkerhetskopian är sättet att flytta, åt båda hållen:

1. Säkerhetskopiera den gamla installationen: `qtestframe backup --config gammal.yaml --out flytt.zip`.
2. Förbered den nya konfigurationen med det andra avsnittet `database` och **samma nyckel** för de lagrade hemligheterna: kopiera nyckelfilen och peka `secrets.local.key_file` på den (standardplatsen skiljer sig: bredvid en SQLite-databas, annars `$STATE_DIRECTORY` eller `/var/lib/qtestframe`), eller sätt samma `QTF_SECRETS_KEY`.
3. Skapa det tomma schemat: `qtestframe migrate --config ny.yaml` (samma version av Qtestframe).
4. Återställ: `qtestframe restore --config ny.yaml --in flytt.zip --yes`.
5. Starta servern med `ny.yaml` och kontrollera den.

För PostgreSQL: skapa först en egen roll och databas, till exempel:

```sql
CREATE ROLE qtestframe LOGIN PASSWORD 'byt-mig';
CREATE DATABASE qtestframe OWNER qtestframe ENCODING 'UTF8' TEMPLATE template0;
```

och anslut med TLS (`sslmode=verify-full`). Detaljer, `pg_hba.conf` och klientcertifikat finns i [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#4-database-sqlite-or-postgresql) (på engelska).

## Säkerhetskopior av själva databasen

De fungerar också, men är knutna till ett databassystem:

- **SQLite:** en kopia under drift med `sqlite3 /var/lib/qtestframe/qtestframe.db ".backup '/var/backups/qtestframe/qtf.db'"` (eller `VACUUM INTO`). Kopiera aldrig bara `.db`-filen medan servern kör; utan `sqlite3`, stoppa tjänsten och kopiera `qtestframe.db*`. För att återställa: stoppa tjänsten, lägg kopian på plats, ta bort filerna `-wal` och `-shm` och starta (en äldre kopia migreras framåt).
- **PostgreSQL:** `pg_dump -Fc` och `pg_restore`, eller WAL-arkivering (pgBackRest, Barman) för återställning till valfri tidpunkt.

Båda innehåller samma krypterade hemligheter och kräver samma nyckel.

## Flytta ett enskilt projekt

För att flytta eller kopiera **ett** projekt i stället för installationen: använd **Spara projekt** (kräver `project.update` och `export`) och **Läs in projekt** på målservern (kräver `project.create`). Projektet får nya id:n och en nyckel som användaren väljer. Arkiv med ett äldre schema kan läsas in i en nyare server, inte tvärtom. Hemligheter (webhook-hemligheter, ärendesystemtoken) finns inte i arkivet och måste anges igen; inlästa webhooks är avstängda.

# Driftrutiner

På en systemd-värd använder exemplen det här aliaset, som kör kommandot som tjänsteanvändaren med produktionskonfigurationen:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
```

## Återkommande uppgifter

| När | Uppgift |
|---|---|
| dagligen | Säkerhetskopia (`qtf backup --out …`) kopierad till en annan värd; titta på poster med ERROR och WARN (`journalctl -u qtestframe -p warning --since yesterday`). |
| varje vecka | Certifikatens giltighet (`scripts/certs/verify-cert.sh -q --connect …`); diskutrymme för databasen och säkerhetskopiorna; misslyckade inloggningar och SCIM-fel. |
| varje månad | Ett **återställningstest** till en tillfällig installation (med en kopia av nyckeln); `qtf secrets status`; gå igenom superadministratörerna (**Administration → Användare**), API-nycklar som inte använts på länge, SCIM-token. |
| per version | Uppgradering (se [Uppgradering](#uppgradering)); jämför `deploy/config.example.yaml` med er konfiguration. |
| årligen, eller enligt policy | `qtf secrets rotate` (ny datanyckel), sedan `qtf secrets rotate --prune` när alla serverprocesser har startats om; med en TPM, förnya [återställningskopian](#katastrofåterställning-för-tpm-förseglade-nycklar). |

Andra vanliga uppgifter:

| Uppgift | Hur |
|---|---|
| läs om TLS-certifikatet | `sudo systemctl reload qtestframe` |
| ändra loggnivån en tid | **Administration → Systeminställningar → Loggning och syslog**, eller `QTF_LOG_LEVEL=debug` i en drop-in och en omstart |
| en användare har glömt lösenordet | organisationsadministratör: **Medlemmar → Återställ lösenord**; superadministratör: **Användare**. Det finns ingen lösenordsåterställning på kommandoraden; om ingen superadministratör kan logga in, skapa en ny (nedan) |
| en användare har tappat bort autentiseringsappen och återställningskoderna | **Medlemmar → Återställ tvåstegsverifiering** (organisationsadministratör) eller **Användare** (superadministratör), efter att ha kontrollerat personen |
| stänga ute någon snabbt | stäng av kontot under **Användare** (avslutar dess sessioner); återkalla dess API-nycklar; för SSO-konton, stäng av dem hos identitetsleverantören också |
| den sista superadministratören är borta | på värden: `qtf user create --email … --password-stdin --super-admin` skapar en ny |
| kontrollera en konfigurationsändring | `qtf check-config` före omstarten |

## Kapacitet

- **SQLite** skriver genom en anslutning, i tur och ordning. Den klarar gott tiotals samtidiga användare och databaser på några GB på en lokal SSD. När begäranden börjar köa (stigande `duration_ms` i begärandeloggen under last), flytta till PostgreSQL.
- **PostgreSQL**: `database.max_open_conns` (standard 20) per serverprocess; databasservern måste tillåta summan över alla processer.
- **Filer** lagras i databasen. `files.max_upload_mb` begränsar en fil; databasen och varje säkerhetskopia växer med bilagorna.
- **Minne**: enheten begränsar servern till 2 GB (`MemoryMax`, `MemoryHigh=1536M`). Stora importer, exporter och rapporter behöver minne medan de byggs; webbtester startar en webbläsare per jobb, så höj `MemoryMax` tillsammans med `runner.web.max_parallel`.
- **Tidsgränser**: `server.write_timeout` (standard 2 minuter) begränsar nedladdningar, exporter och synkrona skriptkörningar; höj den för mycket stora exporter, eller använd kommandoraden för säkerhetskopior.

## Felsökning

| Symptom | Orsak och åtgärd |
|---|---|
| tjänsten startar inte | `journalctl -u qtestframe -b -e`; `qtf check-config` listar konfigurationsfel. |
| `config: parse …: field listn not found` | en felstavad eller borttagen nyckel; filen tolkas strikt. |
| `server.base_url is required for e-mail` | sätt `server.base_url`, eller stäng av e-post. |
| `secrets: the stored data-encryption keys are wrapped by another key: … the configured key is …` | servern har en annan nyckel än den som data krypterades med: fel `key_file`, `QTF_SECRETS_KEY`, en annan TPM eller HSM-nyckel. Konfigurera rätt nyckel, eller omslut med `secrets rewrap --from-config GAMMAL.yaml`. |
| `secrets: create the key file …` | standardplatsen för nyckeln är inte skrivbar; sätt `secrets.local.key_file` eller `QTF_SECRETS_KEY`. |
| `secrets: TPM: unseal failed; the PCRs … have changed` | firmware eller startkedja har ändrats med PCR-bindning: starta det tidigare läget och omslut, eller använd återställningskopian. |
| `secrets: TPM: load the sealed key (was it sealed by another TPM, or was the TPM cleared?)` | data förseglades på en annan maskin eller TPM:en rensades; se [katastrofåterställning](#katastrofåterställning-för-tpm-förseglade-nycklar). |
| `secrets: open TPM /dev/tpmrm0: …` | tjänsten kan inte öppna enheten: drop-in med `PrivateDevices=no`, `DeviceAllow=/dev/tpmrm0 rw`, gruppen `tss`. |
| `secrets: the pkcs11 (HSM) key provider is not available in this binary` | bygg med `make go-pkcs11`. |
| `secrets: pkcs11: log in to token … (check the PIN)` / `no token with …` / `has no AES key labelled …` | PIN, tokenetikett eller plats, eller nyckeletikett; `generate_key: true` skapar nyckeln. |
| `hardening: …` och starten stoppas | `no_new_privs` misslyckades; kontrollera containermiljön eller init-systemet. |
| `permission denied` för en fil efter start | sökvägen finns inte i Landlocks regler: lägg till den i `hardening.read_only_paths` / `read_write_paths` (den måste finnas vid start). |
| `landlock: …` under `skipped` | kärnan saknar Landlock; lägg till `landlock` i startparametern `lsm=`, eller godta det. |
| `Operation not permitted` vid ett systemanrop | ett `SystemCallFilter` i systemd eller seccomp-listan; bekräfta med `ausearch -m SECCOMP` och rapportera det. |
| SELinux- eller AppArmor-nekanden | `ausearch -m AVC -ts recent -c qtestframe` och `getsebool -a \| grep qtestframe`; `journalctl -k \| grep apparmor="DENIED"`. |
| bindning till `:443` misslyckas | `CAP_NET_BIND_SERVICE` borttagen av en drop-in, eller SELinux `qtestframe_bind_http_port` av; eller styr om 443 till 8443 med nftables. |
| ingen kan logga in över okrypterad HTTP | `auth.cookie_secure` måste vara `false` utan TLS. |
| `429 rate_limited` vid inloggning | `auth.login_max_attempts` inom `auth.login_lockout` per adress och e-post; bakom en proxy, sätt `server.trusted_proxies`, annars delar alla en adress. |
| single sign-on går i loop, `oidc: state mismatch` | `server.base_url` måste stämma med webbläsarens adress; session affinity med flera instanser; inloggningen tog över 10 minuter; klockavvikelse. |
| `external login failed … oidc …: discovery` | servern når inte utfärdaren: brandvägg för utgående trafik, proxy, SELinux `qtestframe_connect_http`. |
| `external login failed … e-mail address not verified` / `no e-mail claim` | IdP:n skickar `email_verified: false` eller ingen `email_claim`. |
| `external login failed … user not provisioned` | användaren finns inte och `auto_provision` är av. |
| `external login failed … e-mail already bound to another identity provider` | ett konto med den e-postadressen är kopplat till en annan leverantör. |
| `403 mfa_enrolment_required` för API-anrop från en webbläsarsession | MFA-policyn kräver en andra faktor; användaren måste lägga till en. |
| `stored system settings are invalid and were ignored` | en filändring gjorde en sparad inställning ogiltig; rätta eller återställ den under **Systeminställningar**. |
| `backup schema version N does not match the database schema version M` | återställ med versionen som gjorde säkerhetskopian och uppgradera sedan. |
| TLS-fel mot PostgreSQL | `sslrootcert` läsbar för tjänsten; `sslmode=verify-full` kräver servernamnet i certifikatet. |
| webbtester misslyckas direkt | `MemoryDenyWriteExecute=no` och `ExecPaths=` för Playwright-katalogen; `/dev/shm` skrivbar; `runner.work_dir` kort (socket-sökväg under 108 byte). |
| skript: `409` med `not_ready` | inställningarna för testexekvering förbjuder resultatet; se körningens kolumn för beredskap. |

## Loggmeddelanden att hålla utkik efter

| Meddelande | Nivå | Betydelse |
|---|---|---|
| `qtestframe listening` | INFO | servern är igång (adress, TLS, version, databas) |
| `hardening` | INFO | vilka begränsningar som tillämpades och vilka som hoppades över |
| `secrets encrypted` | INFO | nyckelleverantören, nyckelns fingeravtryck och den aktiva datanyckeln |
| `secrets: created the key file …` | WARN | en ny nyckelfil skapades: säkerhetskopiera den nu |
| `secrets: the key file … is readable by group or others` | WARN | rätta läget med `chmod 600` |
| `secrets: encrypted N secret(s) that were stored as plaintext` | WARN | värden från en äldre version krypterades vid den här starten |
| `applied database migrations` | INFO | en uppgradering migrerade schemat |
| `bootstrapped administrator` | INFO | den första administratören skapades |
| `no users exist; …` | WARN | det finns inget konto ännu och ingen bootstrap-konfiguration |
| `stored system settings are invalid and were ignored; …` | ERROR | se ovan |
| `login failed`, `second factor failed` | WARN | en misslyckad inloggning, med e-post och adress |
| `external login failed` | WARN | en misslyckad inloggning med OIDC eller SAML, med orsaken |
| `scim authentication failed` | WARN | en begäran med fel SCIM-token |
| `request failed` | ERROR | ett oväntat fel bakom ett `500`-svar, med sökväg och fel |
| `tls certificate reloaded` / `tls reload failed; keeping previous certificate` | INFO / ERROR | resultatet av en `SIGHUP` |
| `webhook delivery` | WARN | en webhook kunde inte levereras |
| `digest job`, `report subscriptions job` | WARN | schemalagd e-post misslyckades |
| `reset interrupted job` | INFO | ett webbtestjobb från en stoppad process återställdes |
| `audit write failed` | WARN | en granskningspost kunde inte sparas (loggposten skrivs ändå) |
| `shutting down` | INFO | servern fick `SIGTERM` eller `SIGINT` |

# Kommandoradsreferens

`qtestframe` är både servern och administrationsverktyget. Alla kommandon utom `report`, `gate`, `select`, `version`, `secrets generate-key` och `healthcheck --url` läser konfigurationen (`--config FIL`, standard `$QTF_CONFIG`, sedan de inbyggda standardvärdena och `QTF_*`-variabler). De kommandon som öppnar databasen migrerar den först.

```text
qtestframe <kommando> [flaggor]
qtestframe help                 # översikten
qtestframe <kommando> -h        # ett kommandos flaggor
```

- Kör administrationskommandon **som tjänsteanvändaren** (`sudo -u qtestframe …`), så att filer som en ny nyckelfil får rätt ägare.
- Avslutningskod: `0` vid framgång (även för `-h`), `1` vid fel (skrivs som `error: …`), `2` för ett okänt kommando. `gate`: `0` grinden passerades, `1` den passerades inte, `2` den kunde inte utvärderas.
- Flaggor kan skrivas med ett eller två bindestreck (`-config` och `--config`). Lösenord läses bara från standard input.
- `qtestframe sandbox-exec` är internt: servern startar det för att sandlåda skript. Anropa det inte själv.

Beskrivningarna nedan är skrivna för handboken; flaggtabellerna genereras från programmets egna flaggdefinitioner, och flaggornas förklaringar är därför på engelska, som programmet skriver ut dem.

<!-- BEGIN GENERATED: cli (make admin-guide-ref) -->

### `qtestframe serve`

Kör HTTP(S)-servern. Migrerar databasen, öppnar nyckelleverantören, skapar bootstrap-administratören när det inte finns några användare, tillämpar systeminställningarna som sparats i databasen, låser processen och lyssnar. Stoppas med `SIGINT`/`SIGTERM` (15 s för pågående begäranden); `SIGHUP` läser om TLS-certifikatet.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe migrate`

Tillämpar de migreringar som återstår och skriver hur många. `serve` gör detsamma vid varje start.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |

### `qtestframe user create`

Skapar en användare. Med `--password-stdin` läses lösenordet som en rad från standard input (minst 10 tecken med en bokstav och ett tecken som inte är en bokstav); utan den kan användaren bara logga in med single sign-on.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | e-mail address |
| `--name` | `string` | display name |
| `--password-stdin` | — | read the password from stdin |
| `--super-admin` | — | grant global super administrator rights |

### `qtestframe tenant create`

Skapar en organisation (tenant).

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--name` | `string` | tenant name |
| `--slug` | `string` | tenant slug |

### `qtestframe member add`

Ger en användare en roll i en organisation, eller ändrar den. Rollen är en inbyggd roll eller en egen roll i organisationen.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | user e-mail |
| `--role` | `string` | role, or several roles separated by commas (e.g. sme,tester) (default "viewer") |
| `--tenant` | `string` | tenant slug |

### `qtestframe seed-demo`

Lägger till demoprojekten i en organisation (standard `bootstrap.tenant_slug`, skapas om den saknas), med demoanvändare. Inte för produktion.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--tenant` | `string` | tenant slug (default: bootstrap tenant) |

### `qtestframe backup`

Skriver en logisk säkerhetskopia av alla data (alla organisationer) till en ZIP-fil, via en tillfällig fil som byter namn när den är klar. Granskas som `admin.backup`.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--out` | `string` | backup file to write (ZIP) |

### `qtestframe restore`

Ersätter **alla** data med en säkerhetskopia, i en transaktion, och loggar ut alla. Kräver `--yes`. Granskas som `admin.restore`.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--in` | `string` | backup file to restore (ZIP) |
| `--yes` | — | confirm that ALL existing data is replaced |

### `qtestframe secrets`

Kommandona under `secrets` delar en uppsättning flaggor; vart och ett använder de som gäller det.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--from-config` | `string` | rewrap: configuration file of the key provider that wrapped the keys |
| `--prune` | — | rotate: delete the old data-encryption keys no value uses any more |
| `--yes` | — | decrypt: confirm that the secrets are stored as plaintext |

#### `qtestframe secrets status`

Visar nyckelleverantören, nyckelns fingeravtryck, datakrypteringsnycklarna och hur många lagrade hemligheter som är krypterade. Ändrar ingenting (skapar ingen nyckelfil).

#### `qtestframe secrets migrate`

Krypterar de hemligheter som fortfarande lagras i klartext (görs också vid varje start).

#### `qtestframe secrets rotate`

Skapar en ny datakrypteringsnyckel och krypterar om varje hemlighet med den; `--prune` tar sedan bort de nycklar som inget värde använder.

#### `qtestframe secrets rewrap`

Omsluter datakrypteringsnycklarna med den konfigurerade nyckelleverantören; `--from-config GAMMAL.yaml` anger konfigurationen för leverantören som omslöt dem.

#### `qtestframe secrets decrypt`

Lagrar alla hemligheter i klartext igen (före en nedgradering). Kräver `--yes`.

#### `qtestframe secrets generate-key`

Skriver ut en ny slumpmässig 256-bitarsnyckel (base64) för `QTF_SECRETS_KEY` eller en nyckelfil.

### `qtestframe check-config`

Läser in och kontrollerar konfigurationen (även hemlighetsfiler) utan att röra databasen, och skriver `configuration OK`.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe healthcheck`

Begär `/healthz` från den lokala servern (adress från `server.listen`, `https` med TLS, certifikatet verifieras inte) och avslutar med 0 vid HTTP 200. För hälsokontroller i containrar utan curl.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |
| `--url` | `string` | URL to probe (default derived from server.listen) |

### `qtestframe report`

Laddar upp JUnit XML- eller JSON-resultat från ett CI-jobb till en körning på en server, via REST-API:t med en API-nyckel. Fler argument är rapportfiler (`*.json` som JSON, övriga som JUnit XML). I GitLab CI, GitHub Actions och Jenkins registreras pipelinen som resultatens bygge (`--ci none` stänger av det); `--post-status` och `--post-comment` skickar commit-statusen `qtestframe/tests` och en anteckning i merge requesten.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--branch` | `string` | branch of the build (default from the CI variables) |
| `--build` | `string` | build of a run created with --create-run (default the CI pipeline number) |
| `--build-id` | `string` | CI build id, unique in the project (default from the CI variables) |
| `--build-number` | `string` | CI build number (default from the CI variables) |
| `--ci` | `string` | record the CI build (pipeline id and number, commit, branch, merge request) of the results: auto (from the CI variables) or none (default "auto") |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit SHA of the build (default from the CI variables) |
| `--configuration` | `string` | configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION) |
| `--create-missing` | — | create draft automated tests for JUnit test cases without a test key (needs test.write) |
| `--create-run` | — | create the run named by --run when the project has none |
| `--environment` | `string` | environment of a run created with --create-run |
| `--fail-on-unknown` | — | exit with an error when results match no test (unknown keys or test cases without a key) |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--json` | `value` | results in the JSON import format {"results":[...]} (file or glob; repeatable) |
| `--junit` | `value` | JUnit XML report (file or glob; repeatable) |
| `--merge-request` | `string` | merge / pull request of the build (default from the CI variables) |
| `--pipeline-url` | `string` | pipeline URL of the build (default from the CI variables) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--run` | `string` | name of the test run to report into |
| `--run-id` | `string` | id of the test run to report into |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

### `qtestframe gate`

Kontrollerar kvalitetsgrinden för en CI-pipeline på en server: slutkriterierna för en release (`--release`, valfritt `--variant`) eller villkoren i `--require` (`must`, `no_failed`, `no_stale`, `no_blocking_defects`, `coverage>=80`, `no_open_defects:major`, `cycle_executed:NAMN>=90`, …) på resultaten i en körning (`--run`) eller ett bygge (`--build`). Skriver en sammanfattning med det som inte uppfylls och länkar; `--format junit|markdown|json --output FIL` skriver också en rapport för pipelinen. Slutkod 0 när grinden passeras, 1 när den inte gör det, 2 när den inte kan utvärderas. `--post-status` och `--post-comment` skickar en commit-status och en anteckning i merge requesten (GitLab eller GitHub, se sidan CI/CD i användarhandboken).

| Flagga | Värde | Betydelse |
|---|---|---|
| `--build` | `string` | build (CI id or number) whose runs' results are checked |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit of the change (default from the CI variables) |
| `--format` | `string` | output: text, json, junit or markdown (default "text") |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--merge-request` | `string` | merge / pull request of the change (default from the CI variables) |
| `--output` | `string` | write the output to this file (the text summary still goes to the log) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) whose exit criteria are checked |
| `--require` | `value` | condition, e.g. coverage>=80, must, no_failed, no_stale, no_blocking_defects, no_open_defects:major, cycle_executed:NAME>=90 (repeatable, or comma-separated); replaces the release's exit criteria |
| `--run` | `string` | run (name or id) whose results are checked |
| `--run-id` | `string` | id of the run whose results are checked |
| `--scope` | `string` | requirements of a gate without --release: all (default), affected (those the merge request / commit refers to) or a query |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | evaluate for this variant (name or id) only |

### `qtestframe select`

Skriver de testfall en CI-pipeline kör för en ändring: de krav och testfall som ändringens commits (från git, `--since` eller `--commits`), merge request, ändrade filer (med sökvägskartan `qtestframe-map.yml`) och commit-meddelanden hänvisar till, regressionsurvalet och testfallen med taggarna i `--tag`. `--format` ger testnycklar, ett filter för testköraren (`pytest`, `go`, `playwright`, `maven`), JSON eller en underpipeline för GitLab (`gitlab-child-pipeline`); `--create-run` skapar en körning av urvalet.

| Flagga | Värde | Betydelse |
|---|---|---|
| `--all` | — | select every test |
| `--cli-image` | `string` | image with the qtestframe binary for the child pipeline's report job (default $QTF_CLI_IMAGE) |
| `--commits` | `value` | commit SHAs of the change (repeatable or comma-separated; instead of git) |
| `--create-run` | — | create a run of the selected tests (results of report --run-id land in it) |
| `--environment` | `string` | environment of the created run |
| `--fallback` | `string` | when the change is unknown (no commits, files or keys): none (select nothing) or all (default "none") |
| `--files` | `value` | changed files (repeatable or comma-separated; instead of git) |
| `--format` | `string` | output: keys, pytest, go, playwright, maven, json, gitlab-child-pipeline (default "keys") |
| `--gate-release` | `string` | release whose exit criteria the child pipeline's gate checks |
| `--gate-require` | `value` | condition of the child pipeline's gate (repeatable) |
| `--git` | — | read the commits (messages) and changed files with git (default true) |
| `--head` | `string` | the revision of the change (default $CI_COMMIT_SHA, else HEAD) |
| `--image` | `value` | image of a runner's job in the child pipeline, RUNNER=IMAGE (repeatable) |
| `--include-manual` | — | select manual tests too |
| `--include-related` | — | add requirements that depend on, refine or derive from a changed one |
| `--job` | `value` | test command of a runner's job in the child pipeline, RUNNER=COMMAND (repeatable) |
| `--map` | `string` | path map: changed files → tests, tags, requirements (default qtestframe-map.yml when it exists) |
| `--merge-request` | `string` | merge / pull request number (default from the CI variables) |
| `--no-gate` | — | no gate in the child pipeline |
| `--output` | `string` | write the output to this file |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) of the created run |
| `--requirement` | `value` | changed requirement (key or id; repeatable) |
| `--run` | `string` | name of the run to create (default $QTF_RUN) |
| `--since` | `string` | compare with this git revision (default: the merge request's diff base, else the commit before the push) |
| `--since-build` | `string` | also the commits after this build (CI id or number) |
| `--tag` | `value` | always run the tests with this tag (repeatable) |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | only tests that apply to this variant |

### `qtestframe version`

Skriver ut versionen (även `--version`, `-v`).

<!-- END GENERATED: cli -->

## Exempel

```sh
# första superadministratören och en organisation
read -rs PW && printf '%s\n' "$PW" | qtestframe user create --email admin@example.com --password-stdin --super-admin
qtestframe tenant create --slug acme --name "Acme AB"
qtestframe member add --tenant acme --email admin@example.com --role admin

# säkerhetskopiering och återställning
qtestframe backup --out /var/backups/qtestframe/qtf.zip
qtestframe restore --in /var/backups/qtestframe/qtf.zip --yes

# nycklarna till de lagrade hemligheterna
qtestframe secrets status
qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml

# CI: rapportera JUnit-resultat till en körning, som skapas om den saknas
QTF_URL=https://qtf.example.com QTF_TOKEN=qtf_… \
  qtestframe report --project SHOP --run "Nightly $BUILD" --create-run --build "$BUILD" --junit 'reports/*.xml'

# CI: kvalitetsgrinden för pipelinens körning, som JUnit för pipelinen
qtestframe gate --project SHOP --run "Nightly $BUILD" --release 1.0 --format junit --output qtf-gate.xml

# CI: en merge requests testfall som pytest-filter, eller som underpipeline
pytest -k "$(qtestframe select --project SHOP --format pytest)"
qtestframe select --project SHOP --format gitlab-child-pipeline --create-run --run "MR $CI_MERGE_REQUEST_IID" --output child.yml
```

Färdiga pipelinedefinitioner finns i `examples/ci/` och i appen (**Inställningar → Projekt → CI-uppsättning**); se sidan *CI/CD* i användarhandboken.

# Funktioner som lagts till senare

Det här kapitlet samlar administrationsanteckningarna på serversidan för funktioner som lagts till efter att resten av handboken skrevs: konfigurationsnycklar, behörigheter, bakgrundsjobb, data som en säkerhetskopia innehåller eller inte innehåller, och utgående anslutningar. Varje funktion som behöver en sådan anteckning får ett avsnitt nedan; till dess beskriver [användarhandboken](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/sv/administration.md) funktionens skärmar.

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

Kvalitetsgrinden, testurvalet och pipelinemallarna (migrering 0053) behöver ingen konfiguration; serverns rapporter av commit-statusar och anteckningar i merge requests till GitLab är en projektinställning med en krypterad token och en utkorg i bakgrunden (`ci_deliveries`, som minutjobbet försöker igen, och som inte säkerhetskopieras). Tokens, roller, jobbtokens begränsningar och utgående anslutningar beskrivs i [E-post, webhooks och integrationer](#e-post-webhooks-och-integrationer); ändringar av inställningarna loggas som `project.ci_settings`.
