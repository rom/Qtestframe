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
