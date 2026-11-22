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

## Defekt- och ärendesystem

Projekt kan skicka defekter och krav till **Jira**, **GitHub** eller **GitLab**, synkronisera deras status tillbaka och importera ärenden. Användare med `project.update` ansluter ett projekt i dess inställningar ([användarhandboken](../../user-guide/sv/defects.md#anslut-jira-github-eller-gitlab)); att skicka och synkronisera kräver `run.execute`, ärenden från krav `requirement.write`.

| System | Adress | Inloggning | Minsta användbara omfattning |
|---|---|---|---|
| Jira Cloud | `https://<webbplats>.atlassian.net` | användarens e-post + API-token (HTTP Basic) | en användare som får bläddra bland och skapa ärenden i projektet |
| Jira Server / Data Center | er Jira-adress | personlig åtkomsttoken (Bearer) | som ovan |
| GitHub | standard `https://api.github.com` (GitHub Enterprise: dess API-adress) | token (Bearer) | finkornig token med *Issues: read and write* för det enda repot |
| GitLab | standard `https://gitlab.com`, eller en egen instans (HTTPS) | token i `PRIVATE-TOKEN` | projektåtkomsttoken med omfattningen `api` och rollen Reporter |

Token anges en gång, lagras krypterad och returneras aldrig av API:t (bara *har token*); den ingår inte i ett projektarkiv. Begäranden följer `HTTPS_PROXY`/`NO_PROXY` och systemets CA-certifikat, har en tidsgräns, och väntar på systemets `Retry-After` när den är kort. Utskick och synkroniseringar granskas (`defect.push`, `defect.sync`, `requirement.issue.*`).

## Körning av skript och webbtester

Körningen utför skripten i automatiska tester (`sh`, `bash`, `python`, `javascript`) och webbtester (Playwright) på servern. Den är **av som standard**; läs [avsnittet om sandlådan](security.md#sandlådan-för-skript-och-webbtester) innan den slås på. Inställningarna gäller efter en omstart.

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

Containeravbilden kräver `--build-arg WITH_PLAYWRIGHT=true` och `QTF_RUNNER_ENABLED=true`, `QTF_RUNNER_WEB_ENABLED=true` (se [Installation](installation.md#i-en-container-docker-eller-podman)).
