// Package app implements the qtestframe command line (serve, migrate and
// administrative sub-commands).
package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rom/qtestframe/internal/api"
	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/hardening"
	"github.com/rom/qtestframe/internal/logging"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/webui"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Env bundles process I/O so commands are testable.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Ctx is cancelled to stop `serve` (signals are also handled).
	Ctx context.Context
	// Ready, if set, receives the listen address once serving.
	Ready func(addr string)
}

const usage = `qtestframe – requirements and test management server

Usage:
  qtestframe serve       [--config FILE] [--port N | -p N] [--listen HOST:PORT]
                                                  run the HTTP(S) server
  qtestframe migrate     [--config FILE]          apply database migrations
  qtestframe user create [--config FILE] --email E [--name N] [--password-stdin] [--super-admin]
  qtestframe tenant create [--config FILE] --slug S --name N
  qtestframe member add  [--config FILE] --tenant SLUG --email E --role ROLE[,ROLE...]
                         (admin|manager|test_lead|tester|sme|reviewer|viewer or a custom role)
  qtestframe seed-demo   [--config FILE] [--tenant SLUG]
  qtestframe backup      [--config FILE] --out FILE.zip
                                                  write a logical backup of all data
  qtestframe restore     [--config FILE] --in FILE.zip --yes
                                                  replace ALL data with a backup
  qtestframe secrets status|migrate|rotate [--prune]|rewrap [--from-config OLD]|decrypt --yes|generate-key [--config FILE]
                                                  encryption of stored secrets (docs/security.md)
  qtestframe audit verify [--config FILE] [--tenant SLUG] [--file EXPORT.json] [--json]
                                                  check the hash chain of the audit trail
  qtestframe check-config [--config FILE] [--port N] [--listen HOST:PORT]
  qtestframe healthcheck [--config FILE] [--port N] [--url URL]
  qtestframe report      --url URL --token T (--project KEY --run NAME [--create-run] | --run-id ID)
                         (--junit FILE | --json FILE)... [FILE...] [--build B] [--environment E]
                         [--create-missing] [--fail-on-unknown] [--ci auto|none]
                         [--post-status] [--post-comment]
                                                  upload automated test results to a server; in CI
                                                  the pipeline is recorded as the build and a commit
                                                  status / merge request note can be posted
  qtestframe gate        --project KEY [--release R [--variant V]] [--run NAME|--run-id ID] [--build B]
                         [--require COND]... [--scope all|affected|QUERY]
                         [--format text|json|junit|markdown] [--output FILE]
                         [--post-status] [--post-comment]
                                                  check the quality gate: exit 0 pass, 1 fail, 2 error
                                                  (COND: must, no_failed, no_stale, no_blocking_defects,
                                                  coverage>=80, no_open_defects:major, cycle_executed:NAME …)
  qtestframe select      --project KEY [--since REV] [--commits SHA,…] [--files F,…] [--map FILE]
                         [--tag T]... [--requirement KEY]... [--create-run --run NAME]
                         [--format keys|pytest|go|playwright|maven|json|gitlab-child-pipeline]
                         [--output FILE] [--job RUNNER=CMD] [--image RUNNER=IMAGE]
                                                  the tests to run for a change (merge request)
  qtestframe stubs       --project KEY --framework pytest|playwright|go|junit5|jest|vitest|robot|cucumber
                         [--query Q] [--ids KEY,…] [--out DIR] [--package P] [--dry-run]
                         [--mark-script [--script-dir DIR]]
                                                  write test code stubs named after the test keys (an
                                                  unimplemented stub reports as skipped); generating
                                                  again keeps implemented bodies
  qtestframe prompt      --project KEY [--kind architecture|design|implementation_plan|full_solution|implement|…]
                         [--scope QUERY] [--folder PATH] [--release R] [--variant V] [--lang en|sv]
                         [--compact] [--split none|folder|size] [--max-tokens N] [--formats mermaid,c4,openapi,sql]
                         [--out DIR]
                                                  build a prompt for an AI assistant (with the project's
                                                  solution preamble) and print it, or write one .md file
                                                  per part; nothing is sent to an AI service
  qtestframe import-tests --from testrail|xray|zephyr|qtest|azure FILE --project KEY [--dry-run]
                         [--format csv|xml|json|xlsx] [--folder PATH] [--overwrite] [--no-history]
                         [--no-links] [--no-folders]
                                                  import the test cases (and past results) of another
                                                  test management tool; importing again updates them
                         report, gate, select, stubs, prompt and import-tests read QTF_URL, QTF_TOKEN and QTF_PROJECT, and the
                         GitLab CI / GitHub Actions variables; tokens for posting: QTF_GITLAB_TOKEN,
                         QTF_GITHUB_TOKEN
  qtestframe version

The configuration file is YAML (see deploy/config.example.yaml); unknown keys
are rejected. Precedence: command line flags > QTF_* environment variables >
YAML file > built-in defaults.
`

// Run executes the command line and returns the exit code.
func Run(args []string, env Env) int {
	if env.Ctx == nil {
		env.Ctx = context.Background()
	}
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "serve":
		err = cmdServe(args[1:], env)
	case "migrate":
		err = cmdMigrate(args[1:], env)
	case "user":
		err = sub(args[1:], env, "create", cmdUserCreate)
	case "tenant":
		err = sub(args[1:], env, "create", cmdTenantCreate)
	case "member":
		err = sub(args[1:], env, "add", cmdMemberAdd)
	case "seed-demo":
		err = cmdSeedDemo(args[1:], env)
	case "backup":
		err = cmdBackup(args[1:], env)
	case "restore":
		err = cmdRestore(args[1:], env)
	case "secrets":
		err = cmdSecrets(args[1:], env)
	case "audit":
		err = cmdAudit(args[1:], env)
	case "healthcheck":
		err = cmdHealthcheck(args[1:], env)
	case "check-config":
		err = cmdCheckConfig(args[1:], env)
	case "report":
		err = cmdReport(args[1:], env)
	case "gate":
		err = cmdGate(args[1:], env)
	case "select":
		err = cmdSelect(args[1:], env)
	case "stubs":
		err = cmdStubs(args[1:], env)
	case "prompt":
		err = cmdPrompt(args[1:], env)
	case "import-tests":
		err = cmdImportTests(args[1:], env)
	case "sandbox-exec":
		var c *runner.SandboxConfig
		if c, err = runner.ParseSandboxArgs(args[1:], env.Stderr); err == nil {
			err = runner.SandboxExec(c)
		}
	case "version", "--version", "-v":
		fmt.Fprintf(env.Stdout, "qtestframe %s\n", Version)
	case "help", "--help", "-h":
		fmt.Fprint(env.Stdout, usage)
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		var ee *exitError
		if errors.As(err, &ee) {
			if ee.err != nil {
				fmt.Fprintf(env.Stderr, "error: %v\n", ee.err)
			}
			return ee.code
		}
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func sub(args []string, env Env, name string, fn func([]string, Env) error) error {
	if len(args) == 0 || args[0] != name {
		return fmt.Errorf("expected sub-command %q", name)
	}
	return fn(args[1:], env)
}

func newFlags(name string, env Env) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	cfg := fs.String("config", "", "configuration file (default $QTF_CONFIG)")
	return fs, cfg
}

// listenFlags registers --listen and --port (-p) on fs.
func listenFlags(fs *flag.FlagSet) *config.Overrides {
	o := &config.Overrides{}
	fs.StringVar(&o.Listen, "listen", "", "listen address host:port (overrides server.listen)")
	fs.IntVar(&o.Port, "port", 0, "listen port (overrides the port of server.listen)")
	fs.IntVar(&o.Port, "p", 0, "shorthand for --port")
	return o
}

// openStore loads configuration, opens and migrates the database and sets
// up the encryption of stored secrets (notes, e.g. a created key file, are
// written to notes).
func openStore(ctx context.Context, cfgPath string, notes io.Writer) (config.Config, *store.Store, error) {
	cfg, st, err := openDB(ctx, cfgPath)
	if err != nil {
		return cfg, nil, err
	}
	if _, err := setupSecrets(ctx, &cfg, st, func(msg string) { fmt.Fprintf(notes, "secrets: %s\n", msg) }); err != nil {
		st.DB.Close()
		return cfg, nil, err
	}
	return cfg, st, nil
}

// openDB loads configuration, opens and migrates the database (stored
// secrets stay as they are: backup, secrets status and rewrap).
func openDB(ctx context.Context, cfgPath string) (config.Config, *store.Store, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return cfg, nil, err
	}
	d, err := db.Open(ctx, cfg.Database.Driver, cfg.Database.DSN, cfg.Database.MaxOpenConns)
	if err != nil {
		return cfg, nil, err
	}
	if _, err := d.Migrate(ctx); err != nil {
		d.Close()
		return cfg, nil, err
	}
	return cfg, store.New(d), nil
}

func cmdCheckConfig(args []string, env Env) error {
	fs, cfgPath := newFlags("check-config", env)
	ov := listenFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadWith(*cfgPath, *ov)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "configuration OK (listen %s, database %s)\n", cfg.Server.Listen, cfg.Database.Driver)
	return nil
}

func cmdMigrate(args []string, env Env) error {
	fs, cfgPath := newFlags("migrate", env)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	d, err := db.Open(env.Ctx, cfg.Database.Driver, cfg.Database.DSN, cfg.Database.MaxOpenConns)
	if err != nil {
		return err
	}
	defer d.Close()
	n, err := d.Migrate(env.Ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "applied %d migration(s)\n", n)
	return nil
}

func readPassword(env Env) (string, error) {
	line, err := bufio.NewReader(env.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func cmdUserCreate(args []string, env Env) error {
	fs, cfgPath := newFlags("user create", env)
	email := fs.String("email", "", "e-mail address")
	name := fs.String("name", "", "display name")
	pwStdin := fs.Bool("password-stdin", false, "read the password from stdin")
	super := fs.Bool("super-admin", false, "grant global super administrator rights")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required")
	}
	_, st, err := openStore(env.Ctx, *cfgPath, env.Stderr)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	u := &model.User{Email: *email, DisplayName: *name, SuperAdmin: *super}
	if *pwStdin {
		pw, err := readPassword(env)
		if err != nil {
			return err
		}
		if err := auth.ValidatePassword(pw); err != nil {
			return err
		}
		if u.PasswordHash, err = auth.HashPassword(pw); err != nil {
			return err
		}
	}
	if err := st.CreateUser(env.Ctx, u); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "created user %s (%s)\n", u.Email, u.ID)
	return nil
}

func cmdTenantCreate(args []string, env Env) error {
	fs, cfgPath := newFlags("tenant create", env)
	slug := fs.String("slug", "", "tenant slug")
	name := fs.String("name", "", "tenant name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, st, err := openStore(env.Ctx, *cfgPath, env.Stderr)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	t, err := st.CreateTenant(env.Ctx, *slug, *name)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "created tenant %s (%s)\n", t.Slug, t.ID)
	return nil
}

func cmdMemberAdd(args []string, env Env) error {
	fs, cfgPath := newFlags("member add", env)
	tenant := fs.String("tenant", "", "tenant slug")
	email := fs.String("email", "", "user e-mail")
	role := fs.String("role", rbac.RoleViewer, "role, or several roles separated by commas (e.g. sme,tester)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, st, err := openStore(env.Ctx, *cfgPath, env.Stderr)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	t, err := st.GetTenantBySlug(env.Ctx, *tenant)
	if err != nil {
		return err
	}
	set, err := st.RoleSet(env.Ctx, t.ID)
	if err != nil {
		return err
	}
	roles := rbac.NormalizeRoles(strings.Split(*role, ","))
	if ok, bad := set.ValidAll(roles); !ok {
		return fmt.Errorf("invalid role %q (roles: %s)", bad, strings.Join(set.Names(), ", "))
	}
	u, err := st.GetUserByEmail(env.Ctx, *email)
	if err != nil {
		return err
	}
	if err := st.SetMembershipRoles(env.Ctx, t.ID, u.ID, roles); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "%s is now %s in %s\n", u.Email, strings.Join(roles, ", "), t.Slug)
	return nil
}

func cmdSeedDemo(args []string, env Env) error {
	fs, cfgPath := newFlags("seed-demo", env)
	tenant := fs.String("tenant", "", "tenant slug (default: bootstrap tenant)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, st, err := openStore(env.Ctx, *cfgPath, env.Stderr)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	if *tenant == "" {
		*tenant = cfg.Bootstrap.TenantSlug
	}
	t, err := st.GetTenantBySlug(env.Ctx, *tenant)
	if err != nil {
		if t, err = st.CreateTenant(env.Ctx, *tenant, cfg.Bootstrap.TenantName); err != nil {
			return err
		}
	}
	p, err := SeedDemo(env.Ctx, st, t.ID)
	if err != nil {
		return err
	}
	if err := SeedDemoWeb(env.Ctx, st, t.ID, p, cfg.Server.BaseURL); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "seeded demo project %s in tenant %s\n", p.Key, t.Slug)
	// the demo users must not take the place of the bootstrap administrator,
	// who is only created while the database has no users
	if err := Bootstrap(env.Ctx, st, cfg.Bootstrap, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		return err
	}
	sp, err := SeedDemoSharing(env.Ctx, st, t.ID)
	if err != nil {
		return err
	}
	// the administrator started the demo projects (they existed before the
	// account did)
	if admin, err := st.GetUserByEmail(env.Ctx, cfg.Bootstrap.AdminEmail); err == nil {
		for _, x := range []*model.Project{p, sp} {
			if x == nil {
				continue
			}
			if err := st.SetProjectCreatorIfUnknown(env.Ctx, t.ID, x.ID, admin.ID); err != nil {
				return err
			}
		}
	}
	if sp != nil {
		fmt.Fprintf(env.Stdout, "seeded restricted project %s: shared with %s (viewer) and %s (sme, reviewer; sme and tester in the organisation); %s is not a member (password of all: %s)\n",
			sp.Key, DemoMemberEmail, DemoSMEEmail, DemoOutsiderEmail, DemoUserPassword)
	}
	// owners and assignments for "My work" (the users exist now)
	if err := SeedDemoWorkList(env.Ctx, st, t.ID, p, cfg.Bootstrap.AdminEmail); err != nil {
		return err
	}
	// change control and change requests (seedchangerequests.go)
	if err := SeedDemoChangeRequests(env.Ctx, st, t.ID, p, cfg.Bootstrap.AdminEmail); err != nil {
		return err
	}
	// live updates and presence: nothing to store but a profile setting
	if err := SeedDemoLive(env.Ctx, st); err != nil {
		return err
	}
	// the two-way sync with Jira and Azure DevOps, run as the administrator (seedimport.go)
	if err := SeedDemoReqSync(env.Ctx, st, t.ID, p, cfg.Bootstrap.AdminEmail); err != nil {
		return err
	}
	// discussions by the demo users (seeddiscussions.go)
	if err := SeedDemoDiscussions(env.Ctx, st, t.ID, p); err != nil {
		return err
	}
	// test cycles with the demo users as testers (after the bootstrap too)
	if err := SeedDemoCycles(env.Ctx, st, t.ID, p); err != nil {
		return err
	}
	// test environments, bookings and test data (after the runs and cycles)
	return SeedDemoEnvironments(env.Ctx, st, t.ID, p)
}

// cmdBackup writes a logical, database-agnostic backup (see
// store.Backup). The file is written to a temporary name and renamed
// once complete.
func cmdBackup(args []string, env Env) error {
	fs, cfgPath := newFlags("backup", env)
	out := fs.String("out", "", "backup file to write (ZIP)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	// the backup keeps the stored secrets encrypted (and the wrapped keys)
	_, st, err := openDB(env.Ctx, *cfgPath)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	tmp, err := os.CreateTemp(filepath.Dir(*out), ".qtf-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	m, err := st.Backup(env.Ctx, tmp, Version)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), *out); err != nil {
		return err
	}
	details := fmt.Sprintf("%d rows in %d tables, schema %d (command line)", m.Rows(), len(m.Tables), m.SchemaVersion)
	_ = st.Audit(env.Ctx, model.AuditEntry{UserEmail: "cli", Action: "admin.backup", EntityType: "system", Details: details})
	fmt.Fprintf(env.Stdout, "backup written to %s: %d rows in %d tables (schema version %d)\n", *out, m.Rows(), len(m.Tables), m.SchemaVersion)
	return nil
}

// cmdRestore replaces all data with a backup (see store.Restore).
func cmdRestore(args []string, env Env) error {
	fs, cfgPath := newFlags("restore", env)
	in := fs.String("in", "", "backup file to restore (ZIP)")
	yes := fs.Bool("yes", false, "confirm that ALL existing data is replaced")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return errors.New("--in is required")
	}
	if !*yes {
		return errors.New("restore replaces ALL data and signs everyone out; pass --yes to confirm")
	}
	f, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	_, st, err := openStore(env.Ctx, *cfgPath, env.Stderr)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	counts, err := st.Restore(env.Ctx, f, fi.Size())
	if err != nil {
		return err
	}
	total := 0
	for _, t := range store.SortedTables(counts) {
		total += counts[t]
		fmt.Fprintf(env.Stdout, "  %-24s %d\n", t, counts[t])
	}
	details := fmt.Sprintf("%d rows in %d tables from %s (command line)", total, len(counts), filepath.Base(*in))
	_ = st.Audit(env.Ctx, model.AuditEntry{UserEmail: "cli", Action: "admin.restore", EntityType: "system", Details: details})
	fmt.Fprintf(env.Stdout, "restored %d rows in %d tables from %s\n", total, len(counts), *in)
	return nil
}

// Bootstrap creates the initial tenant and administrator when the
// database has no users and bootstrap credentials are configured.
func Bootstrap(ctx context.Context, st *store.Store, b config.Bootstrap, log *slog.Logger) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if b.AdminEmail == "" || b.AdminPassword == "" {
		log.Warn("no users exist; configure bootstrap.admin_email/admin_password or run `qtestframe user create`")
		return nil
	}
	if err := auth.ValidatePassword(b.AdminPassword); err != nil {
		return fmt.Errorf("bootstrap admin password: %w", err)
	}
	t, err := st.GetTenantBySlug(ctx, b.TenantSlug)
	if errors.Is(err, store.ErrNotFound) {
		t, err = st.CreateTenant(ctx, b.TenantSlug, b.TenantName)
	}
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(b.AdminPassword)
	if err != nil {
		return err
	}
	u := &model.User{Email: b.AdminEmail, DisplayName: "Administrator", PasswordHash: hash, SuperAdmin: true}
	if err := st.CreateUser(ctx, u); err != nil {
		return err
	}
	if err := st.SetMembership(ctx, t.ID, u.ID, rbac.RoleAdmin); err != nil {
		return err
	}
	log.Info("bootstrapped administrator", "email", u.Email, "tenant", t.Slug)
	return nil
}

// hardeningOptions derives in-process restrictions from configuration.
func hardeningOptions(cfg config.Config) hardening.Options {
	h := cfg.Hardening
	o := hardening.Options{
		NoNewPrivs:       h.NoNewPrivs,
		DropCapabilities: h.DropCapabilities,
		KeepCapabilities: []int{10}, // CAP_NET_BIND_SERVICE
		Landlock:         h.Landlock,
		Seccomp:          h.Seccomp,
		ReadOnlyPaths:    append([]string{}, h.ReadOnlyPaths...),
		ReadWritePaths:   append([]string{os.TempDir()}, h.ReadWritePaths...),
	}
	addRO := func(paths ...string) {
		for _, p := range paths {
			if p != "" {
				o.ReadOnlyPaths = append(o.ReadOnlyPaths, p)
			}
		}
	}
	addRO(cfg.Server.TLSCert, cfg.Server.TLSKey, cfg.Logging.Syslog.CAFile, cfg.Logging.Syslog.CertFile, cfg.Logging.Syslog.KeyFile)
	if d := os.Getenv("CREDENTIALS_DIRECTORY"); d != "" {
		addRO(d)
	}
	for _, s := range cfg.Auth.SAML {
		addRO(s.IDPMetadataFile, s.SPCertFile, s.SPKeyFile)
	}
	if cfg.Database.Driver == "sqlite" && cfg.Database.DSN != ":memory:" {
		dsn := strings.TrimPrefix(strings.SplitN(cfg.Database.DSN, "?", 2)[0], "file:")
		if abs, err := filepath.Abs(dsn); err == nil {
			// directory is needed for the -wal/-shm files
			o.ReadWritePaths = append(o.ReadWritePaths, filepath.Dir(abs))
		}
	}
	if cfg.Logging.Syslog.Enabled && strings.HasPrefix(cfg.Logging.Syslog.Network, "unix") {
		o.ReadWritePaths = append(o.ReadWritePaths, cfg.Logging.Syslog.Address)
	}
	// the TPM unwraps data-encryption keys created later (secrets rotate
	// by another process, a restore)
	if cfg.Secrets.Provider == config.SecretsTPM && cfg.Secrets.TPM.Device != "" {
		o.ReadWritePaths = append(o.ReadWritePaths, cfg.Secrets.TPM.Device)
	}
	if cfg.Runner.Enabled {
		o.ReadWritePaths = append(o.ReadWritePaths, cfg.Runner.WorkDir)
		o.ExecPaths = []string{"/usr", "/bin", "/lib", "/lib64", "/opt"}
		if self, err := os.Executable(); err == nil {
			o.ExecPaths = append(o.ExecPaths, filepath.Dir(self))
		}
		for _, i := range cfg.Runner.Interpreters {
			o.ExecPaths = append(o.ExecPaths, filepath.Dir(i))
		}
		// web tests: the child processes (Node, Playwright, the browser)
		// inherit the server's Landlock domain
		if w := cfg.Runner.Web; w.Enabled {
			for _, p := range []string{w.PlaywrightDir, w.BrowsersPath} {
				if abs, err := filepath.Abs(p); p != "" && err == nil {
					o.ExecPaths = append(o.ExecPaths, abs)
				}
			}
			for _, p := range []string{w.Node, w.ExecutablePath} {
				if p != "" {
					o.ExecPaths = append(o.ExecPaths, filepath.Dir(p))
					if real, err := filepath.EvalSymlinks(p); err == nil {
						o.ExecPaths = append(o.ExecPaths, filepath.Dir(real))
					}
				}
			}
			if f := strings.Fields(w.Command); len(f) > 0 && filepath.IsAbs(f[0]) {
				o.ExecPaths = append(o.ExecPaths, filepath.Dir(f[0]))
			}
			o.ExecPaths = append(o.ExecPaths, "/sbin")
			o.ReadOnlyPaths = append(o.ReadOnlyPaths, "/etc", "/proc", "/sys")
			o.ReadWritePaths = append(o.ReadWritePaths, "/dev/shm")
		}
	}
	return o
}

func cmdServe(args []string, env Env) error {
	fs, cfgPath := newFlags("serve", env)
	ov := listenFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadWith(*cfgPath, *ov)
	if err != nil {
		return err
	}
	logs, err := logging.NewManager(cfg.Logging, env.Stderr)
	if err != nil {
		return err
	}
	defer logs.Close()
	log := logs.Logger()
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(env.Ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg.Database.Driver, cfg.Database.DSN, cfg.Database.MaxOpenConns)
	if err != nil {
		return err
	}
	defer d.Close()
	n, err := d.Migrate(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Info("applied database migrations", "count", n)
	}
	st := store.New(d)
	// stored secrets are encrypted; the keys are unwrapped now, before the
	// process is locked down (key file, TPM, HSM)
	kp, err := setupSecrets(ctx, &cfg, st, func(msg string) { log.Warn("secrets: " + msg) })
	if err != nil {
		return err
	}
	defer kp.Close()
	log.Info("secrets encrypted", "provider", kp.Name(), "key", kp.KeyID(), "data_key", st.Keyring().Active())
	if err := Bootstrap(ctx, st, cfg.Bootstrap, log); err != nil {
		return err
	}
	// system settings stored in the database (Administration → System
	// settings) apply over the file and the environment; cfg stays the
	// file/environment configuration, eff is what this process runs with
	base := cfg
	eff, err := api.EffectiveConfig(ctx, st, base)
	if err != nil {
		log.Error("stored system settings are invalid and were ignored; using the configuration file", "err", err)
	}
	if !reflect.DeepEqual(eff.Logging, base.Logging) {
		if err := logs.Reconfigure(eff.Logging); err != nil {
			log.Error("stored logging settings not applied", "err", err)
		}
	}
	cfg = eff
	if cfg.Runner.Enabled {
		if err := os.MkdirAll(cfg.Runner.WorkDir, 0o700); err != nil {
			return fmt.Errorf("runner work dir: %w", err)
		}
	}
	api.Version = Version
	srv, err := api.New(st, base, log, runner.New(cfg.Runner), webui.FS())
	if err != nil {
		return err
	}
	srv.Logs = logs
	ln, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		return err
	}
	tlsOn := cfg.Server.TLSCert != ""
	if tlsOn {
		certs, err := newCertReloader(cfg.Server.TLSCert, cfg.Server.TLSKey)
		if err != nil {
			ln.Close()
			return err
		}
		go certs.watch(ctx, log)
		ln = tls.NewListener(ln, &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certs.get,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
			NextProtos: []string{"h2", "http/1.1"},
		})
	}
	if !tlsOn {
		signal.Ignore(syscall.SIGHUP) // reload is a no-op without TLS
	}
	// All files needed at start-up are open: lock the process down.
	rep, err := hardening.Apply(hardeningOptions(cfg))
	if err != nil {
		ln.Close()
		return fmt.Errorf("hardening: %w", err)
	}
	log.Info("hardening", "applied", rep.Applied, "skipped", rep.Skipped)

	hs := &http.Server{
		Handler:           srv.Handler(),
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	go purgeSessions(ctx, st, log)
	go recordMetrics(ctx, st, log, time.Hour)
	// e-mail digests and scheduled reports (only when e-mail is configured)
	if cfg.Email.On() {
		log.Info("e-mail enabled", "host", cfg.Email.Host, "port", cfg.Email.Port, "tls", cfg.Email.TLS, "from", cfg.Email.From)
	}
	srv.StartJobs(ctx)
	// automated tests (web tests, "Run automated tests") run in the background
	srv.StartScriptJobs(ctx)
	// live updates: the cross-instance bus (PostgreSQL) and presence expiry;
	// open event streams never become idle, so shutdown ends them
	srv.StartLive(ctx)
	hs.RegisterOnShutdown(srv.StopLive)
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	log.Info("qtestframe listening", "addr", ln.Addr().String(), "tls", tlsOn, "version", Version, "db", cfg.Database.Driver)
	if env.Ready != nil {
		env.Ready(ln.Addr().String())
	}
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = hs.Shutdown(sctx)
	srv.WaitJobsStopped(10 * time.Second) // jobs end when ctx does
	srv.WaitMail()                        // e-mails in flight (each is bounded by email.timeout)
	return err
}

// certReloader serves the current key pair and reloads it on SIGHUP
// (e.g. after a Let's Encrypt renewal).
type certReloader struct {
	certFile, keyFile string
	cur               atomic.Pointer[tls.Certificate]
}

func newCertReloader(certFile, keyFile string) (*certReloader, error) {
	c := &certReloader{certFile: certFile, keyFile: keyFile}
	return c, c.reload()
}

func (c *certReloader) reload() error {
	kp, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}
	c.cur.Store(&kp)
	return nil
}

func (c *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) { return c.cur.Load(), nil }

func (c *certReloader) watch(ctx context.Context, log *slog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			if err := c.reload(); err != nil {
				log.Error("tls reload failed; keeping previous certificate", "err", err)
			} else {
				log.Info("tls certificate reloaded")
			}
		}
	}
}

// cmdHealthcheck probes /healthz of the local server (for container
// HEALTHCHECK directives where no curl is available).
func cmdHealthcheck(args []string, env Env) error {
	fs, cfgPath := newFlags("healthcheck", env)
	rawURL := fs.String("url", "", "URL to probe (default derived from server.listen)")
	ov := listenFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	u := *rawURL
	if u == "" {
		cfg, err := config.LoadWith(*cfgPath, *ov)
		if err != nil {
			return err
		}
		host, port, err := net.SplitHostPort(cfg.Server.Listen)
		if err != nil {
			return err
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		scheme := "http"
		if cfg.Server.TLSCert != "" {
			scheme = "https"
		}
		u = scheme + "://" + net.JoinHostPort(host, port) + "/healthz"
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		// local probe of our own listener; certificate names rarely match 127.0.0.1
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec
	}}
	res, err := client.Get(u)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", res.StatusCode)
	}
	fmt.Fprintln(env.Stdout, "ok")
	return nil
}

// recordMetrics records the daily metric snapshot of every project: once at
// start and then every interval for the projects that have no snapshot for
// the current day (of the store's clock) yet, so each project gets one
// snapshot per day.
func recordMetrics(ctx context.Context, st *store.Store, log *slog.Logger, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := st.RecordAllSnapshots(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("metric snapshots failed", "err", err)
		} else if n > 0 {
			log.Info("recorded metric snapshots", "projects", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func purgeSessions(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := st.PurgeExpiredSessions(ctx); err != nil {
				log.Warn("session purge failed", "err", err)
			} else if n > 0 {
				log.Info("purged expired sessions", "count", n)
			}
			if n, err := st.PurgeTrash(ctx); err != nil {
				log.Warn("trash purge failed", "err", err)
			} else if n > 0 {
				log.Info("purged expired trash entries", "count", n)
			}
		}
	}
}
