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
| SMTP-lösenord och OIDC-klienthemligheter som angetts i webbklienten | `system_settings` |

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

Varje post skrivs också som en loggpost `audit` på nivån INFO. Skicka loggen till en central syslog-server (se [Loggning](logging-and-monitoring.md#central-syslog)), så att spåret överlever att värden komprometteras. Misslyckade inloggningar syns i serverloggen (`login failed`) och i användarens inloggningshistorik, inte i händelseloggen. Händelseloggen ingår i en säkerhetskopia.

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

Projektadministratörer (`project.update`) väljer adresserna till ärendesystem och webhooks, och skriptförfattare (`script.execute`) målen för webbtester; superadministratörer kan få servern att ansluta till adresser med testknapparna i systeminställningarna. Alla sådana begäranden går ut från serverns nät. Om servern når interna tjänster: filtrera dess utgående trafik med brandvägg eller proxy (`deploy/nftables` har en valfri policy för utgående trafik). Leveranser av webhooks följer aldrig omdirigeringar.

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
