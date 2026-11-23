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

**I en säkerhetskopia** (115 tabeller, i den ordning de återställs): `tenants`, `users`, `memberships`, `api_tokens`, `projects`, `configurations`, `environments`, `releases`, `folders`, `categories`, `requirements`, `requirement_categories`, `evaluation_criteria`, `test_cases`, `test_steps`, `test_case_categories`, `trace_links`, `variants`, `builds`, `test_cycles`, `test_cycle_tests`, `test_runs`, `run_configurations`, `test_executions`, `execution_iterations`, `step_results`, `audit_log`, `revisions`, `requirement_relations`, `baselines`, `baseline_items`, `comments`, `files`, `file_blobs`, `attachments`, `report_definitions`, `login_events`, `roles`, `test_dependencies`, `trash`, `item_links`, `release_requirements`, `defects`, `defect_links`, `risks`, `risk_requirements`, `notifications`, `webhooks`, `review_requests`, `baseline_signoffs`, `scim_tokens`, `scim_settings`, `scim_users`, `scim_groups`, `scim_group_members`, `metric_snapshots`, `trace_rules`, `document_items`, `defect_regressions`, `consistency_dismissals`, `invitations`, `report_subscriptions`, `user_mfa`, `mfa_recovery_codes`, `mfa_policies`, `project_members`, `waivers`, `waiver_requirements`, `waiver_signoffs`, `soa_decisions`, `system_settings`, `external_refs`, `link_criteria`, `system_elements`, `requirement_elements`, `test_elements`, `verification_settings`, `verification_records`, `verification_record_files`, `item_applicability`, `code_changes`, `code_change_items`, `code_settings`, `custom_fields`, `field_set_fields`, `custom_field_archive`, `item_templates`, `org_item_templates`, `project_template_defaults`, `exploratory_sessions`, `exploratory_scope`, `exploratory_notes`, `test_cycle_schedules`, `secret_keys`, `chat_channels`, `change_control_settings`, `change_requests`, `change_request_items`, `change_request_signoffs`, `change_request_events`, `req_sync_configs`, `req_sync_conflicts`, `dashboards`, `dashboard_defaults`, `audit_heads`, `audit_checkpoints`, `audit_keys`, `audit_backfill`, `erecords_settings`, `esignatures`, `environment_bookings`, `test_data_sets`, `test_data_links`, `test_data_usage`, `test_data_refresh_requests`.

**Inte i en säkerhetskopia:** `chat_deliveries`, `ci_deliveries`, `esign_tickets`, `job_runs`, `mfa_challenges`, `mfa_trusted_devices`, `req_sync_runs`, `schema_migrations`, `script_jobs`, `sessions`, `similarity_embeddings`, `suggest_history`.

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
