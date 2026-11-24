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

**I en säkerhetskopia** (117 tabeller, i den ordning de återställs): `tenants`, `users`, `memberships`, `api_tokens`, `projects`, `configurations`, `environments`, `releases`, `folders`, `categories`, `requirements`, `requirement_categories`, `evaluation_criteria`, `test_cases`, `test_steps`, `test_case_categories`, `trace_links`, `variants`, `builds`, `test_cycles`, `test_cycle_tests`, `test_runs`, `run_configurations`, `test_executions`, `execution_iterations`, `step_results`, `audit_log`, `revisions`, `requirement_relations`, `baselines`, `baseline_items`, `comments`, `files`, `file_blobs`, `attachments`, `report_definitions`, `login_events`, `roles`, `test_dependencies`, `trash`, `item_links`, `release_requirements`, `defects`, `defect_links`, `risks`, `risk_requirements`, `notifications`, `webhooks`, `review_requests`, `baseline_signoffs`, `scim_tokens`, `scim_settings`, `scim_users`, `scim_groups`, `scim_group_members`, `metric_snapshots`, `trace_rules`, `document_items`, `defect_regressions`, `consistency_dismissals`, `invitations`, `report_subscriptions`, `user_mfa`, `mfa_recovery_codes`, `mfa_policies`, `webauthn_credentials`, `project_members`, `waivers`, `waiver_requirements`, `waiver_signoffs`, `soa_decisions`, `system_settings`, `external_refs`, `link_criteria`, `system_elements`, `requirement_elements`, `test_elements`, `verification_settings`, `verification_records`, `verification_record_files`, `item_applicability`, `code_changes`, `code_change_items`, `code_settings`, `custom_fields`, `field_set_fields`, `custom_field_archive`, `item_templates`, `org_item_templates`, `project_template_defaults`, `exploratory_sessions`, `exploratory_scope`, `exploratory_notes`, `test_cycle_schedules`, `secret_keys`, `chat_channels`, `change_control_settings`, `change_requests`, `change_request_items`, `change_request_signoffs`, `change_request_events`, `req_sync_configs`, `req_sync_conflicts`, `dashboards`, `dashboard_defaults`, `audit_heads`, `audit_checkpoints`, `audit_keys`, `audit_backfill`, `erecords_settings`, `esignatures`, `environment_bookings`, `test_data_sets`, `test_data_links`, `test_data_usage`, `test_data_refresh_requests`, `step_definitions`.

**Inte i en säkerhetskopia:** `backup_locks`, `backup_runs`, `chat_deliveries`, `ci_deliveries`, `esign_tickets`, `job_runs`, `mfa_challenges`, `mfa_trusted_devices`, `req_sync_runs`, `schema_migrations`, `script_jobs`, `sessions`, `similarity_embeddings`, `suggest_history`, `webauthn_challenges`.

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

## Schemalagda säkerhetskopior

I stället för en timer kan servern ta säkerhetskopiorna själv. Slå på dem under **Administration → Systeminställningar → Schemalagda säkerhetskopior** (eller i avsnittet `backup` i konfigurationsfilen):

| Inställning | Betydelse |
|---|---|
| `backup.enabled` | ta schemalagda säkerhetskopior |
| `backup.schedule` | när, i UTC: `daily 02:00`, `weekly sun 03:30` eller `every 6h` (15 minuter till 30 dagar) |
| `backup.directory` | absolut sökväg till katalogen som filerna skrivs till, t.ex. `/var/backups/qtestframe`; den gäller **efter omstart**, eftersom servern öppnar den för skrivning innan processen låses (Landlock) |
| `backup.keep_last`, `backup.keep_days` | gallring: behåll de N nyaste filerna, ta bort filer som är äldre än N dagar |
| `backup.passphrase` | kryptera filerna (se nedan); kan bara skrivas |
| `backup.s3.*` | ladda också upp dem till S3-kompatibel lagring (se nedan) |

```yaml
backup:
  enabled: true
  schedule: daily 02:00
  directory: /var/backups/qtestframe
  keep_last: 14
  keep_days: 30
  passphrase_file: backup-passphrase   # i $CREDENTIALS_DIRECTORY
```

Vad som händer:

- En goroutine i servern kontrollerar schemat varje minut. En säkerhetskopiering skriver samma arkiv som `qtestframe backup` till en tillfällig fil i katalogen, byter namn på den till `qtestframe-backup-ÅÅÅÅMMDD-TTMMSS.zip` (`.zip.enc` när den är krypterad) och registrerar dess storlek och **SHA-256**.
- **Flera servrar** mot samma PostgreSQL-databas: varje säkerhetskopiering tar en låsrad i databasen (`backup_locks`) med ett lån på fem minuter som servern förnyar medan den skriver, så att exakt en server tar varje säkerhetskopia; låset för en server som kraschat löper ut. Ge alla servrar samma katalog bara om den är delad lagring; annars sprids filerna över servrarna.
- En tidpunkt som missades medan ingen server körde (en uppgradering kl. 02.00) tas en gång, vid nästa kontroll. En misslyckad säkerhetskopiering görs inte om varje minut utan vid nästa tidpunkt; använd **Säkerhetskopiera nu** för att försöka igen direkt.
- Efter en lyckad säkerhetskopiering tar **gallringsreglerna** bort gamla filer: `keep_last` behåller de N nyaste, `keep_days` tar bort filer som är äldre än N dagar, och med båda tas en fil bort när någon av reglerna säger det. Den nyaste filen behålls alltid, och bara filer som heter som schemalagda säkerhetskopior tas någonsin bort. `0` stänger av en regel; med båda på `0` tas ingenting bort.
- Varje körning registreras (tabellen `backup_runs`: start, slut, fil, storlek, SHA-256, kryptering, uppladdning, status, fel) och granskas: `admin.backup.completed` eller `admin.backup.failed` (systemposter), `admin.backup.run` när en superadministratör startar en. En misslyckad säkerhetskopiering **skickas också med e-post till alla superadministratörer** när e-post är konfigurerad. En säkerhetskopiering som fortfarande pågår när servern stoppas registreras som misslyckad (`interrupted`) av nästa.

**Administration → Säkerhetskopiering** visar schemat, nästa säkerhetskopia och historiken, med **Säkerhetskopiera nu** (`POST /api/v1/admin/backups/run`, även utan schema när katalogen är angiven), **nedladdning** av en fil (`GET /api/v1/admin/backups/{id}/download`, granskas som `admin.backup.download`) och **verifiering** (nedan). Historiken och låset ingår inte i en säkerhetskopia; filerna ligger utanför databasen, så kopiera katalogen till en annan värd eller använd uppladdningen.

## Krypterade säkerhetskopior

Med en lösenfras krypteras arkivet: AES-256-GCM i block om 1 MiB med en nyckel som härleds från lösenfrasen med scrypt (N = 2^15, r = 8, p = 1, slumpmässigt salt). Varje block autentiseras, så en ändrad, avkortad eller omkastad fil upptäcks; fel lösenfras och en skadad fil ger samma fel. Den krypterade filen börjar med `QTFENC01`.

- Schemalagda säkerhetskopior: ange `backup.passphrase` i systeminställningarna (minst 12 tecken; lagras krypterad med nyckeln för de lagrade hemligheterna, visas aldrig) eller `backup.passphrase_file` i konfigurationsfilen.
- Kommandoraden: `qtestframe backup --out FIL.zip.enc --encrypt` läser lösenfrasen från `--passphrase-file FIL`, annars `QTF_BACKUP_PASSPHRASE`, annars en rad på standard input. Inledande och avslutande mellanslag ignoreras.
- Återställning: `qtestframe restore --in FIL.zip.enc --yes` känner igen en krypterad fil och frågar efter lösenfrasen på samma sätt; den dekrypteras till en tillfällig fil bredvid säkerhetskopian (eller i `$TMPDIR`), som tas bort efteråt. Under **Administration → Säkerhetskopiering** skriver du lösenfrasen i återställningsformuläret, eller lämnar den tom för att använda lösenfrasen för de schemalagda säkerhetskopiorna.

Förvara lösenfrasen någon annanstans än säkerhetskopiorna – i en lösenordshanterare eller ett förseglat kuvert. Utan den kan ingen återställa en krypterad säkerhetskopia. Lösenfrasen skyddar hela arkivet; de lagrade hemligheterna i det är dessutom krypterade med serverns nyckel som tidigare.

## Verifiera en säkerhetskopia

```sh
qtestframe backup verify /var/backups/qtestframe/qtestframe-backup-20261124-020000.zip.enc
```

Kontrollen behöver ingen databas: den dekrypterar en krypterad fil (lösenfras som ovan), öppnar ZIP-arkivet och läser varje post till slutet (vilket kontrollerar dess CRC-32), kontrollerar manifestet (formatversion, en fil för varje tabell som det listar och inga andra) och att varje rad är ett JSON-objekt, och jämför antalet rader med manifestet. Den skriver ut SHA-256, när säkerhetskopian skapades, version, drivrutin och schemaversion och avslutar med `1` när något är fel. Flaggorna står före filen. Den kontrollerar inte att schemaversionen passar en databas – det gör en återställning.

**Verifiera** i historiken gör samma sak på servern och jämför dessutom filens SHA-256 med den som registrerades när den skrevs (`POST /api/v1/admin/backups/{id}/verify`, granskas som `admin.backup.verify`; resultatet visas i historiken). Verifiera en säkerhetskopia då och då, och återställ en i en testinstallation ibland: bara en återställning bevisar en säkerhetskopia.

## Uppladdning till S3-kompatibel lagring

Schemalagda säkerhetskopior kan också laddas upp till Amazon S3 eller annan S3-kompatibel objektlagring (MinIO, Ceph, Garage, …). Qtestframe skickar en `PUT` per fil med vanlig HTTPS, signerad med AWS Signature Version 4 – ingen SDK och ingen uppladdning i flera delar, så en fil kan vara högst 5 GiB. Begäran bär filens SHA-256 som innehållshash, så lagringen avvisar en skadad uppladdning.

| Inställning | Exempel |
|---|---|
| `backup.s3.enabled` | `true` |
| `backup.s3.endpoint` | `https://s3.eu-north-1.amazonaws.com`, `https://minio.example.com:9000` |
| `backup.s3.region` | `eu-north-1` (`us-east-1` för de flesta S3-kompatibla servrar) |
| `backup.s3.bucket` | `qtf-backups` |
| `backup.s3.prefix` | `qtestframe/` |
| `backup.s3.access_key_id` | åtkomstnyckelns ID |
| `backup.s3.secret_access_key` | kan bara skrivas; eller `backup.s3.secret_access_key_file` i konfigurationsfilen |

Begärandena använder sökvägsadressering (`ENDPOINT/BUCKET/NYCKEL`) och går via inställningarna för utgående anslutningar (proxy, tillåtna och nekade värdar). Inloggningsuppgifterna behöver bara `s3:PutObject` på prefixet. En misslyckad uppladdning gör att körningen misslyckas (och skickas med e-post); den lokala filen behålls. Gallringsreglerna gäller bara katalogen: ta bort gamla objekt med en **livscykelregel** för bucketen, och överväg objektlås (WORM) så att en komprometterad server inte kan ta bort sina säkerhetskopior.

## Återställning

En återställning **ersätter alla data** i installationen med säkerhetskopian, i **en transaktion**: varje tabell töms och fylls på nytt, så antingen återställs allt eller så ändras ingenting.

```sh
sudo systemctl stop qtestframe
sudo -u qtestframe qtestframe restore --config /etc/qtestframe/config.yaml --in /var/backups/qtestframe/qtf-2026-11-19.zip --yes
sudo systemctl start qtestframe
```

Kommandot skriver ut antalet rader per tabell och granskas (`admin.restore`). En krypterad säkerhetskopia (`.zip.enc`) dekrypteras först, se [Krypterade säkerhetskopior](#krypterade-säkerhetskopior). Superadministratörer kan också återställa under **Administration → Säkerhetskopiering** (`POST /api/v1/admin/restore`, bekräftas genom att skriva `RESTORE`); uppladdningen begränsas av `server.read_timeout`, så använd kommandoraden för stora säkerhetskopior. Stoppa servern för en återställning på kommandoraden: en server som kör behåller sina nycklar och cachade data från före återställningen.

En återställning avvisas, utan att något ändras, när:

- säkerhetskopians **schemaversion** skiljer sig från databasens. Återställ med den Qtestframe-version som gjorde säkerhetskopian (manifestet anger den) och uppgradera sedan; eller uppgradera den gamla installationen först och säkerhetskopiera igen;
- serverns **nyckel inte kan låsa upp** säkerhetskopians datanycklar (`the backup's encrypted secrets cannot be decrypted with this server's key`). Konfigurera nyckeln som säkerhetskopian gjordes med – samma nyckelfil eller `QTF_SECRETS_KEY`, samma TPM, samma HSM-nyckel – eller se [katastrofåterställning](security.md#katastrofåterställning-för-tpm-förseglade-nycklar);
- arkivet är skadat eller ofullständigt, eller större än 4 GiB okomprimerat.

Säkerhetskopians systeminställningar gäller direkt. Efter en återställning: kontrollera `qtestframe secrets status` och logga in.

Använd historiken i webbklienten för att återställa ett enskilt objekt; för att flytta ett projekt, se nedan.

## Händelseloggen i säkerhetskopior

Händelseloggens hashkedja (se [Manipuleringssäker händelselogg](security.md#manipuleringssäker-händelselogg)) klarar säkerhetskopiering och återställning, också mellan SQLite och PostgreSQL:

- En **säkerhetskopia** är en konsekvent ögonblicksbild av posterna, kedjornas registrerade slut (`audit_heads`), de signerade kontrollpunkterna och deras nyckel (`audit_keys`, krypterad med de säkerhetskopierade datanycklarna), så den återställda kedjan verifieras som originalet. Tiderna sparas på mikrosekunden, vilket båda databaserna lagrar.
- En **återställning** ersätter kedjan med säkerhetskopians: poster som skrevs efter säkerhetskopian försvinner, liksom kontrollpunkterna som skrevs efter den, så den återställda kedjan är intakt; själva återställningen (`admin.restore`) blir nästa post. Spara händelseloggens exporter från tiden mellan säkerhetskopian och återställningen om ni måste kunna visa vad som hände då. Verifiera efter en återställning: `qtestframe audit verify`.
- Återställning till en installation med en annan nyckel misslyckas som för alla krypterade hemligheter; nyckeln till kontrollpunkterna kan inte läsas utan säkerhetskopians nyckel.
- **Projektarkiv** (nedan) innehåller aldrig händelseloggen eller de elektroniska signaturerna: de hör till organisationen. Att spara, läsa in, kopiera eller ta bort ett projekt lägger bara till poster i kedjan; inget tas bort ur den. Elektroniska signaturer behålls när deras projekt tas bort.

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
| 54 | `0054_change_requests.sql` |
| 55 | `0055_audit_chain_erecords.sql` |
| 57 | `0057_environments.sql` |
| 59 | `0059_dashboards.sql` |
| 60 | `0060_req_sync.sql` |
| 61 | `0061_ai_assistance.sql` |
| 62 | `0062_project_logo.sql` |
| 63 | `0063_tenant_admin.sql` |
| 64 | `0064_solution_preamble.sql` |
| 65 | `0065_backup_schedule.sql` |
| 67 | `0067_passkeys.sql` |
| 70 | `0070_gherkin.sql` |

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
