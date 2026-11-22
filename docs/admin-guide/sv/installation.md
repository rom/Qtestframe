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

Välj **SQLite** för en enskild server, utvärderingar och team som vill ha så lite som möjligt att drifta. Välj **PostgreSQL** när ni behöver mer än en serverprocess, när databasen ska skötas av ett databasteam, eller när ni vill ha replikering och återställning till valfri tidpunkt. Ni kan flytta senare åt båda hållen med en [logisk säkerhetskopia](backup-and-migration.md#flytt-mellan-sqlite-och-postgresql).

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

Om det konfigurerade certifikatet inte finns ännu startas inte tjänsten. Redigera `/etc/qtestframe/config.yaml` (minst `server.base_url`, TLS och bootstrap-administratören, se [Första start](first-start.md)) och sedan:

```sh
sudo systemctl enable --now qtestframe
journalctl -u qtestframe -f
```

Enheten kör `qtestframe migrate` före `serve` (`ExecStartPre`), startar om tjänsten vid fel, och `systemctl reload qtestframe` skickar `SIGHUP`, som läser om TLS-certifikatet. Ändra enheten bara med en drop-in (`sudo systemctl edit qtestframe`), till exempel för `LoadCredential=`, `MemoryMax=` eller de lättnader som körningen kräver (se [Integrationer](integrations.md#körning-av-skript-och-webbtester)).

Ta bort med `sudo scripts/uninstall.sh` (behåller konfiguration och data) eller `--purge` (tar bort allt, även databasen).

### Brandvägg, MAC och port 443

- `deploy/nftables/qtestframe.nft` är en brandväggsregeluppsättning med en valfri policy för utgående trafik; gå igenom dess mängder (`admin_v4`, `out_tcp_ports`, …) innan den läses in. Let's Encrypt HTTP-01 kräver inkommande port 80.
- SELinux- (`deploy/selinux`) och AppArmor-profiler (`deploy/apparmor`) begränsar tjänsten; valfria funktioner styrs av SELinux-booleans som `qtestframe_connect_postgresql`, `qtestframe_connect_http` (single sign-on), `qtestframe_remote_syslog` och `qtestframe_run_scripts`. Se [Säkerhet](security.md#lager-i-operativsystemet).
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

- **Miljövariabler låser systeminställningar.** En inställning som sätts av en `QTF_*`-variabel kan inte ändras under **Administration → Systeminställningar** (se [Systeminställningar under drift](system-settings.md#företräde-och-låsning)); i avbilden gäller det loggformatet och lyssningsadressen.
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

Resultatet länkas dynamiskt mot libc och läser in leverantörens modul vid körning. Testa leverantören mot SoftHSM2 med `make test-pkcs11` (`apt-get install softhsm2`), och TPM-leverantören mot en mjukvaru-TPM med `make test-tpm` (`apt-get install swtpm`). Hur båda konfigureras står under [Säkerhet](security.md#nyckelleverantörer).
