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
| Kommandorad | `qtestframe migrate`, `user create`, `backup`, `secrets …` med flera öppnar databasen direkt; se [kommandoradsreferensen](cli-reference.md). |
| Skriptkörning | Valfri (`runner.enabled`). Kör skripten i automatiska tester på servern, vart och ett i en sandlådad barnprocess (`qtestframe sandbox-exec`). |
| Webbtestkörning | Valfri (`runner.web.enabled`). Kör Playwright-tester med Node.js och en riktig webbläsare som bakgrundsjobb. |

## Datalager

| Lager | Innehåll | Anmärkning |
|---|---|---|
| **Databasen** (SQLite eller PostgreSQL) | Allt: organisationer, användare, projekt, krav, tester, körningar, historik, kommentarer, granskningsloggen, sessioner, **uppladdade filer** (bilagor, dokument, rapporter, avatarer) och systeminställningarna som ändrats i webbklienten. | Skapas och uppgraderas automatiskt av [migreringarna](backup-and-migration.md#migreringar). |
| **Nyckeln till de lagrade hemligheterna** | Nyckelkrypteringsnyckeln som skyddar de hemligheter servern måste kunna läsa tillbaka (TOTP-hemligheter, webhook-hemligheter, ärendesystemtoken, hemliga testvariabler, SMTP-lösenord och OIDC-klienthemligheter sparade i webbklienten). | En nyckelfil (`qtestframe-secrets.key`), variabeln `QTF_SECRETS_KEY`, en TPM 2.0 eller en HSM; se [Kryptering av lagrade hemligheter](security.md#kryptering-av-lagrade-hemligheter). **Aldrig i databasen.** |
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

Eftersom projektadministratörer väljer adresserna till ärendesystem och webhooks, och skriptförfattare målen för webbtester, bör serverns utgående trafik filtreras när den når interna tjänster (se [Säkerhet](security.md#utgående-anslutningar)).

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

Hur man väljer databas står under [Installation](installation.md#sqlite-eller-postgresql).
