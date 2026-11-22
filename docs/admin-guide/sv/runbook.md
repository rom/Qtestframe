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
| per version | Uppgradering (se [Uppgradering](backup-and-migration.md#uppgradering)); jämför `deploy/config.example.yaml` med er konfiguration. |
| årligen, eller enligt policy | `qtf secrets rotate` (ny datanyckel), sedan `qtf secrets rotate --prune` när alla serverprocesser har startats om; med en TPM, förnya [återställningskopian](security.md#katastrofåterställning-för-tpm-förseglade-nycklar). |

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
| `secrets: TPM: load the sealed key (was it sealed by another TPM, or was the TPM cleared?)` | data förseglades på en annan maskin eller TPM:en rensades; se [katastrofåterställning](security.md#katastrofåterställning-för-tpm-förseglade-nycklar). |
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
