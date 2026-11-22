# Kommandoradsreferens

`qtestframe` är både servern och administrationsverktyget. Alla kommandon utom `report`, `version`, `secrets generate-key` och `healthcheck --url` läser konfigurationen (`--config FIL`, standard `$QTF_CONFIG`, sedan de inbyggda standardvärdena och `QTF_*`-variabler). De kommandon som öppnar databasen migrerar den först.

```text
qtestframe <kommando> [flaggor]
qtestframe help                 # översikten
qtestframe <kommando> -h        # ett kommandos flaggor
```

- Kör administrationskommandon **som tjänsteanvändaren** (`sudo -u qtestframe …`), så att filer som en ny nyckelfil får rätt ägare.
- Avslutningskod: `0` vid framgång (även för `-h`), `1` vid fel (skrivs som `error: …`), `2` för ett okänt kommando.
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
| `--role` | `string` | role (default "viewer") |
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

Laddar upp JUnit XML- eller JSON-resultat från ett CI-jobb till en körning på en server, via REST-API:t med en API-token. Fler argument är rapportfiler (`*.json` som JSON, övriga som JUnit XML).

| Flagga | Värde | Betydelse |
|---|---|---|
| `--build` | `string` | build of a run created with --create-run |
| `--configuration` | `string` | configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION) |
| `--create-missing` | — | create draft automated tests for JUnit test cases without a test key (needs test.write) |
| `--create-run` | — | create the run named by --run when the project has none |
| `--environment` | `string` | environment of a run created with --create-run |
| `--fail-on-unknown` | — | exit with an error when results match no test (unknown keys or test cases without a key) |
| `--json` | `value` | results in the JSON import format {"results":[...]} (file or glob; repeatable) |
| `--junit` | `value` | JUnit XML report (file or glob; repeatable) |
| `--project` | `string` | project key, e.g. SHOP (needed with --run) |
| `--run` | `string` | name of the test run to report into |
| `--run-id` | `string` | id of the test run to report into |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

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
```
