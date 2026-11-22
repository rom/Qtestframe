package admindoc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rom/qtestframe/internal/app"
)

// command is one documented command line of qtestframe.
type command struct {
	// args are the words of the command ("secrets", "rotate").
	args []string
	doc  text
}

// commands lists every command of `qtestframe help` (TestCommandsCovered
// fails when one is missing).
var commands = []command{
	{[]string{"serve"}, text{
		"Runs the HTTP(S) server. Migrates the database, opens the key provider, creates the bootstrap administrator when there are no users, applies the system settings stored in the database, locks the process down and listens. Stops on `SIGINT`/`SIGTERM` (15 s for requests in flight); `SIGHUP` re-reads the TLS certificate.",
		"Kör HTTP(S)-servern. Migrerar databasen, öppnar nyckelleverantören, skapar bootstrap-administratören när det inte finns några användare, tillämpar systeminställningarna som sparats i databasen, låser processen och lyssnar. Stoppas med `SIGINT`/`SIGTERM` (15 s för pågående begäranden); `SIGHUP` läser om TLS-certifikatet.",
	}},
	{[]string{"migrate"}, text{
		"Applies the pending database migrations and prints how many. `serve` does the same at every start.",
		"Tillämpar de migreringar som återstår och skriver hur många. `serve` gör detsamma vid varje start.",
	}},
	{[]string{"user", "create"}, text{
		"Creates a user. With `--password-stdin` the password is read as one line from standard input (at least 10 characters with a letter and a non-letter); without it the user can only sign in with single sign-on.",
		"Skapar en användare. Med `--password-stdin` läses lösenordet som en rad från standard input (minst 10 tecken med en bokstav och ett tecken som inte är en bokstav); utan den kan användaren bara logga in med single sign-on.",
	}},
	{[]string{"tenant", "create"}, text{
		"Creates an organisation (tenant).",
		"Skapar en organisation (tenant).",
	}},
	{[]string{"member", "add"}, text{
		"Gives a user a role in an organisation, or changes it. The role is a built-in role or a custom role of the organisation.",
		"Ger en användare en roll i en organisation, eller ändrar den. Rollen är en inbyggd roll eller en egen roll i organisationen.",
	}},
	{[]string{"seed-demo"}, text{
		"Adds the demo projects to an organisation (default `bootstrap.tenant_slug`, created when missing), with demo users. Not for production.",
		"Lägger till demoprojekten i en organisation (standard `bootstrap.tenant_slug`, skapas om den saknas), med demoanvändare. Inte för produktion.",
	}},
	{[]string{"backup"}, text{
		"Writes a logical backup of all data (every organisation) to a ZIP file, through a temporary file that is renamed when complete. Audited as `admin.backup`.",
		"Skriver en logisk säkerhetskopia av alla data (alla organisationer) till en ZIP-fil, via en tillfällig fil som byter namn när den är klar. Granskas som `admin.backup`.",
	}},
	{[]string{"restore"}, text{
		"Replaces **all** data with a backup, in one transaction, and signs everyone out. Needs `--yes`. Audited as `admin.restore`.",
		"Ersätter **alla** data med en säkerhetskopia, i en transaktion, och loggar ut alla. Kräver `--yes`. Granskas som `admin.restore`.",
	}},
	{[]string{"secrets", "status"}, text{
		"Shows the key provider, its key fingerprint, the data-encryption keys and how many stored secrets are encrypted. Changes nothing (creates no key file).",
		"Visar nyckelleverantören, nyckelns fingeravtryck, datakrypteringsnycklarna och hur många lagrade hemligheter som är krypterade. Ändrar ingenting (skapar ingen nyckelfil).",
	}},
	{[]string{"secrets", "migrate"}, text{
		"Encrypts the secrets still stored as plaintext (also done at every start).",
		"Krypterar de hemligheter som fortfarande lagras i klartext (görs också vid varje start).",
	}},
	{[]string{"secrets", "rotate"}, text{
		"Creates a new data-encryption key and re-encrypts every secret with it; `--prune` then deletes the keys no value uses.",
		"Skapar en ny datakrypteringsnyckel och krypterar om varje hemlighet med den; `--prune` tar sedan bort de nycklar som inget värde använder.",
	}},
	{[]string{"secrets", "rewrap"}, text{
		"Wraps the data-encryption keys with the configured key provider; `--from-config OLD.yaml` names the configuration of the provider that wrapped them.",
		"Omsluter datakrypteringsnycklarna med den konfigurerade nyckelleverantören; `--from-config GAMMAL.yaml` anger konfigurationen för leverantören som omslöt dem.",
	}},
	{[]string{"secrets", "decrypt"}, text{
		"Stores every secret as plaintext again (before a downgrade). Needs `--yes`.",
		"Lagrar alla hemligheter i klartext igen (före en nedgradering). Kräver `--yes`.",
	}},
	{[]string{"secrets", "generate-key"}, text{
		"Prints a new random 256-bit key (base64) for `QTF_SECRETS_KEY` or a key file.",
		"Skriver ut en ny slumpmässig 256-bitarsnyckel (base64) för `QTF_SECRETS_KEY` eller en nyckelfil.",
	}},
	{[]string{"check-config"}, text{
		"Loads and validates the configuration (secret files included) without touching the database, and prints `configuration OK`.",
		"Läser in och kontrollerar konfigurationen (även hemlighetsfiler) utan att röra databasen, och skriver `configuration OK`.",
	}},
	{[]string{"healthcheck"}, text{
		"Requests `/healthz` of the local server (URL from `server.listen`, `https` with TLS, certificate not verified) and exits with 0 on HTTP 200. For container health checks without curl.",
		"Begär `/healthz` från den lokala servern (adress från `server.listen`, `https` med TLS, certifikatet verifieras inte) och avslutar med 0 vid HTTP 200. För hälsokontroller i containrar utan curl.",
	}},
	{[]string{"report"}, text{
		"Uploads JUnit XML or JSON results of a CI job to a run on a server, through the REST API with an API token. Further arguments are report files (`*.json` as JSON, others as JUnit XML). In GitLab CI, GitHub Actions and Jenkins the pipeline is recorded as the build of the results (`--ci none` turns that off); `--post-status` and `--post-comment` post the commit status `qtestframe/tests` and a merge request note.",
		"Laddar upp JUnit XML- eller JSON-resultat från ett CI-jobb till en körning på en server, via REST-API:t med en API-nyckel. Fler argument är rapportfiler (`*.json` som JSON, övriga som JUnit XML). I GitLab CI, GitHub Actions och Jenkins registreras pipelinen som resultatens bygge (`--ci none` stänger av det); `--post-status` och `--post-comment` skickar commit-statusen `qtestframe/tests` och en anteckning i merge requesten.",
	}},
	{[]string{"gate"}, text{
		"Checks the quality gate of a CI pipeline on a server: the exit criteria of a release (`--release`, optionally `--variant`) or the conditions of `--require` (`must`, `no_failed`, `no_stale`, `no_blocking_defects`, `coverage>=80`, `no_open_defects:major`, `cycle_executed:NAME>=90`, …) on the results of a run (`--run`) or build (`--build`). Prints a summary with the failing items and links; `--format junit|markdown|json --output FILE` also writes a report for the pipeline. Exit code 0 when the gate passes, 1 when it fails, 2 when it cannot be evaluated. `--post-status` and `--post-comment` post a commit status and a merge request note (GitLab or GitHub, see the user guide page CI/CD).",
		"Kontrollerar kvalitetsgrinden för en CI-pipeline på en server: slutkriterierna för en release (`--release`, valfritt `--variant`) eller villkoren i `--require` (`must`, `no_failed`, `no_stale`, `no_blocking_defects`, `coverage>=80`, `no_open_defects:major`, `cycle_executed:NAMN>=90`, …) på resultaten i en körning (`--run`) eller ett bygge (`--build`). Skriver en sammanfattning med det som inte uppfylls och länkar; `--format junit|markdown|json --output FIL` skriver också en rapport för pipelinen. Slutkod 0 när grinden passeras, 1 när den inte gör det, 2 när den inte kan utvärderas. `--post-status` och `--post-comment` skickar en commit-status och en anteckning i merge requesten (GitLab eller GitHub, se sidan CI/CD i användarhandboken).",
	}},
	{[]string{"select"}, text{
		"Prints the tests a CI pipeline runs for a change: the requirements and tests that the change's commits (from git, `--since`, or `--commits`), merge request, changed files (with the path map `qtestframe-map.yml`) and commit messages refer to, the regression selection and the tests with the `--tag` tags. `--format` gives test keys, a test runner filter (`pytest`, `go`, `playwright`, `maven`), JSON or a GitLab child pipeline (`gitlab-child-pipeline`); `--create-run` creates a run of the selection.",
		"Skriver de testfall en CI-pipeline kör för en ändring: de krav och testfall som ändringens commits (från git, `--since` eller `--commits`), merge request, ändrade filer (med sökvägskartan `qtestframe-map.yml`) och commit-meddelanden hänvisar till, regressionsurvalet och testfallen med taggarna i `--tag`. `--format` ger testnycklar, ett filter för testköraren (`pytest`, `go`, `playwright`, `maven`), JSON eller en underpipeline för GitLab (`gitlab-child-pipeline`); `--create-run` skapar en körning av urvalet.",
	}},
	{[]string{"version"}, text{
		"Prints the version (also `--version`, `-v`).",
		"Skriver ut versionen (även `--version`, `-v`).",
	}},
}

// cliFlag is a flag as `-h` prints it.
type cliFlag struct{ name, typ, usage string }

var flagLine = regexp.MustCompile(`^  -(\S+)(?: (\S+))?$`)

// flagsOf runs `qtestframe <args> -h` and parses the flag list.
func flagsOf(args []string) ([]cliFlag, error) {
	var out, errb bytes.Buffer
	code := app.Run(append(append([]string{}, args...), "-h"), app.Env{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Ctx: context.Background()})
	if code != 0 {
		return nil, fmt.Errorf("qtestframe %s -h: exit %d: %s", strings.Join(args, " "), code, errb.String())
	}
	var flags []cliFlag
	for _, line := range strings.Split(errb.String(), "\n") {
		if m := flagLine.FindStringSubmatch(line); m != nil {
			flags = append(flags, cliFlag{name: m[1], typ: m[2]})
			continue
		}
		if strings.HasPrefix(line, "    \t") && len(flags) > 0 {
			f := &flags[len(flags)-1]
			f.usage = strings.TrimSpace(f.usage + " " + strings.TrimSpace(line))
		}
	}
	return flags, nil
}

var cliHead = text{"| Flag | Value | Meaning |\n|---|---|---|", "| Flagga | Värde | Betydelse |\n|---|---|---|"}

var cliNoFlags = text{"No flags.", "Inga flaggor."}

var cliSecretsNote = text{
	"The `secrets` commands share one set of flags; each uses the ones that concern it.",
	"Kommandona under `secrets` delar en uppsättning flaggor; vart och ett använder de som gäller det.",
}

// cliReference documents every command with the flags the binary defines.
func cliReference(lang string) (string, error) {
	// defaults read from the environment must not leak into the guide
	for _, v := range []string{"QTF_URL", "QTF_CONFIGURATION"} {
		if old, ok := os.LookupEnv(v); ok {
			os.Unsetenv(v)
			defer os.Setenv(v, old)
		}
	}
	var b strings.Builder
	table := func(args []string) error {
		flags, err := flagsOf(args)
		if err != nil {
			return err
		}
		if len(flags) == 0 {
			b.WriteString(cliNoFlags.in(lang) + "\n\n")
			return nil
		}
		b.WriteString(cliHead.in(lang) + "\n")
		for _, f := range flags {
			typ := "—"
			if f.typ != "" {
				typ = code(f.typ)
			}
			dash := "--"
			if len(f.name) == 1 {
				dash = "-"
			}
			fmt.Fprintf(&b, "| `%s%s` | %s | %s |\n", dash, f.name, typ, cell(f.usage))
		}
		b.WriteString("\n")
		return nil
	}
	secrets := false
	for _, c := range commands {
		name := strings.Join(c.args, " ")
		if c.args[0] == "secrets" {
			if !secrets {
				secrets = true
				fmt.Fprintf(&b, "### `qtestframe secrets`\n\n%s\n\n", cliSecretsNote.in(lang))
				if err := table(c.args); err != nil {
					return "", err
				}
			}
			fmt.Fprintf(&b, "#### `qtestframe %s`\n\n%s\n\n", name, c.doc.in(lang))
			continue
		}
		fmt.Fprintf(&b, "### `qtestframe %s`\n\n%s\n\n", name, c.doc.in(lang))
		if c.args[0] == "version" {
			continue
		}
		if err := table(c.args); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}
