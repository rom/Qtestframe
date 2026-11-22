# Administratörshandbok för Qtestframe

Den här handboken är för dem som **installerar, konfigurerar och driftar** en Qtestframe-server: systemadministratörer, driftpersonal och säkerhetsansvariga. Den handlar om servern, konfigurationsfilen, single sign-on, krypteringsnycklar, säkerhetskopior och den dagliga driften.

De som *använder* Qtestframe – även organisationsadministratörer som hanterar medlemmar och roller i webbklienten – har [användarhandboken](../../user-guide/sv/index.md). Där båda handböckerna tar upp samma sak (medlemmar, roller, systeminställningar) förklarar den här handboken serversidan och länkar till användarhandboken för skärmbilderna.

## Kapitel

1. [Översikt och arkitektur](overview.md) – komponenter, datalager, portar, vad som körs var.
2. [Installation](installation.md) – systemkrav, programfil, container, systemd, bygga från källkod, SQLite eller PostgreSQL.
3. [Konfiguration](configuration.md) – hur inställningar läses in, hemlighetsfiler, och referensen över alla nycklar och miljövariabler.
4. [Första start](first-start.md) – den första administratören, organisationer, demodata.
5. [Autentisering](authentication.md) – lokala lösenord, flerfaktorsautentisering, OIDC, SAML, SCIM, API-token, sessioner.
6. [Behörigheter](authorisation.md) – roller, egna roller, behörighetsmatrisen, begränsade och delade projekt.
7. [Systeminställningar under drift](system-settings.md) – vad superadministratörer kan ändra i webbklienten, och vad som kräver omstart.
8. [Säkerhet](security.md) – TLS, härdning med Landlock och seccomp, kryptering av lagrade hemligheter och nyckelleverantörerna, nyckelbyte, katastrofåterställning, granskningsloggen, säkerhetshuvuden.
9. [E-post, webhooks och integrationer](integrations.md) – SMTP, aviseringar, webhooks, Jira, GitHub och GitLab, körning av skript och webbtester.
10. [Loggning och övervakning](logging-and-monitoring.md) – loggformat, syslog, hälsokontroller.
11. [Säkerhetskopiering, återställning och migrering](backup-and-migration.md) – vad en säkerhetskopia innehåller, återställning, uppgradering och nedgradering, flytt mellan SQLite och PostgreSQL.
12. [Driftrutiner](runbook.md) – återkommande uppgifter, kapacitet, felsökning, loggmeddelanden.
13. [Kommandoradsreferens](cli-reference.md) – alla `qtestframe`-kommandon och flaggor.
14. [Funktioner som lagts till senare](features-added-later.md) – administrationsanteckningar för nyare funktioner.

## Konventioner

- `qtestframe …` är serverprogrammet; alla kommandon tar `--config FIL` (standard: `$QTF_CONFIG`). På en systemd-värd körs det som tjänsteanvändaren, till exempel `sudo -u qtestframe qtestframe secrets status --config /etc/qtestframe/config.yaml`.
- Konfigurationsnycklar skrivs med punkter: `server.base_url` är `base_url` i avsnittet `server:` i YAML-filen. `auth.oidc[].name` är `name` i en post i listan `auth.oidc`.
- **Superadministratör** är en flagga på ett användarkonto som ger alla behörigheter i alla organisationer och de installationsövergripande sidorna (organisationer, användare, säkerhetskopiering, systeminställningar). **Administratör** (admin) är en roll i en organisation.
- Sökvägar som `/etc/qtestframe` och `/var/lib/qtestframe` är systemd-installationens (`scripts/install.sh`); containeravbilden använder samma sökvägar.

Delar av [konfigurationsreferensen](configuration.md#referens), [systeminställningarna](system-settings.md#alla-inställningar), [behörighetsmatrisen](authorisation.md#standardmatrisen-för-behörigheter), [webhook-händelserna](integrations.md#webhooks), [tabellerna i en säkerhetskopia](backup-and-migration.md#vad-en-säkerhetskopia-innehåller), [migreringarna](backup-and-migration.md#migreringar) och [kommandoradsreferensen](cli-reference.md) genereras från källkoden (`make admin-guide-ref`), så de stämmer med den version de följer med.

## Finns också som

Handboken publiceras också som PDF och som en enda Markdown-fil per språk på [user-guide-utgåvan](https://github.com/rom/Qtestframe/releases/tag/user-guide) (`qtestframe-admin-guide-en.pdf`, `qtestframe-admin-guide-sv.pdf`), och visas i webbklienten under **Hjälp → Administratörshandbok** för superadministratörer och organisationsadministratörer.
