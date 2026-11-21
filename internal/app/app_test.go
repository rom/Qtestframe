package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestMain(m *testing.M) {
	auth.BcryptCost = bcrypt.MinCost
	// Ambient configuration must not leak into the tests.
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "QTF_") || k == "CREDENTIALS_DIRECTORY" {
			os.Unsetenv(k)
		}
	}
	os.Exit(m.Run())
}

const strongPassword = "Sup3r-secret-pw"

// writeConfig writes a configuration using a fresh SQLite file and
// returns its path. extra is YAML deep-merged over the base configuration.
func writeConfig(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	base := map[string]any{
		"server":   map[string]any{"listen": "127.0.0.1:0"},
		"database": map[string]any{"driver": "sqlite", "dsn": filepath.Join(dir, "qtf.db")},
		"logging":  map[string]any{"level": "warn"},
		"hardening": map[string]any{
			"no_new_privs": false, "landlock": false, "seccomp": false, "drop_capabilities": false,
		},
		"runner": map[string]any{"enabled": false, "sandbox": false},
	}
	var over map[string]any
	if err := yaml.Unmarshal([]byte(extra), &over); err != nil {
		t.Fatalf("extra yaml: %v", err)
	}
	merge(base, over)
	b, err := yaml.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func merge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				merge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, Env{Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb})
	return result{code, out.String(), errb.String()}
}

func openTestStore(t *testing.T, cfgPath string) *store.Store {
	t.Helper()
	cfg, st, err := openStore(context.Background(), cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg
	t.Cleanup(func() { st.DB.Close() })
	return st
}

func genCert(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	certFile, keyFile = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o600)
	return certFile, keyFile
}

// ---------------------------------------------------------------- basics

func TestRunBasics(t *testing.T) {
	if r := run(t, ""); r.code != 2 || !strings.Contains(r.stderr, "Usage:") {
		t.Errorf("no args: %+v", r)
	}
	for _, a := range []string{"help", "--help", "-h"} {
		if r := run(t, "", a); r.code != 0 || !strings.Contains(r.stdout, "Usage:") {
			t.Errorf("%s: %+v", a, r)
		}
	}
	for _, a := range []string{"version", "--version", "-v"} {
		if r := run(t, "", a); r.code != 0 || r.stdout != "qtestframe "+Version+"\n" {
			t.Errorf("%s: %+v", a, r)
		}
	}
	if r := run(t, "", "frobnicate"); r.code != 2 || !strings.Contains(r.stderr, `unknown command "frobnicate"`) {
		t.Errorf("unknown: %+v", r)
	}
	for _, args := range [][]string{{"user"}, {"user", "delete"}, {"tenant", "remove"}, {"member", "create"}} {
		if r := run(t, "", args...); r.code != 1 || !strings.Contains(r.stderr, "expected sub-command") {
			t.Errorf("%v: %+v", args, r)
		}
	}
	// sandbox-exec without arguments fails during argument parsing.
	if r := run(t, "", "sandbox-exec"); r.code != 1 || !strings.Contains(r.stderr, "--workdir") {
		t.Errorf("sandbox-exec: %+v", r)
	}
	// Flag errors and -h for every command.
	for _, cmd := range [][]string{{"serve"}, {"migrate"}, {"user", "create"}, {"tenant", "create"}, {"member", "add"}, {"seed-demo"}, {"check-config"}, {"healthcheck"}} {
		if r := run(t, "", append(cmd, "--bogus")...); r.code != 1 {
			t.Errorf("%v --bogus: %+v", cmd, r)
		}
		if r := run(t, "", append(cmd, "-h")...); r.code != 0 {
			t.Errorf("%v -h: %+v", cmd, r)
		}
	}
}

func TestNilCtx(t *testing.T) {
	// Env.Ctx defaults to Background.
	if code := Run([]string{"version"}, Env{Stdout: io.Discard, Stderr: io.Discard}); code != 0 {
		t.Error(code)
	}
}

func TestCheckConfigAndMigrate(t *testing.T) {
	cfg := writeConfig(t, "")
	if r := run(t, "", "check-config", "--config", cfg); r.code != 0 || !strings.Contains(r.stdout, "configuration OK") {
		t.Errorf("check-config: %+v", r)
	}
	bad := writeConfig(t, "auth: {session_ttl: -1s}")
	if r := run(t, "", "check-config", "--config", bad); r.code != 1 || !strings.Contains(r.stderr, "session_ttl") {
		t.Errorf("check-config invalid: %+v", r)
	}
	r := run(t, "", "migrate", "--config", cfg)
	if r.code != 0 || !strings.HasPrefix(r.stdout, "applied ") || strings.HasPrefix(r.stdout, "applied 0 ") {
		t.Errorf("migrate: %+v", r)
	}
	if r := run(t, "", "migrate", "--config", cfg); r.code != 0 || r.stdout != "applied 0 migration(s)\n" {
		t.Errorf("migrate again: %+v", r)
	}
	if r := run(t, "", "migrate", "--config", bad); r.code != 1 {
		t.Errorf("migrate bad config: %+v", r)
	}
	// DSN whose parent "directory" is a regular file: it cannot be created.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	missingDir := writeConfig(t, fmt.Sprintf("database: {dsn: %q}", filepath.Join(blocker, "dir", "qtf.db")))
	if r := run(t, "", "migrate", "--config", missingDir); r.code != 1 || !strings.Contains(r.stderr, "db:") {
		t.Errorf("migrate bad db: %+v", r)
	}
	// Store-backed commands fail the same way.
	for _, args := range [][]string{
		{"user", "create", "--config", missingDir, "--email", "a@b.c"},
		{"tenant", "create", "--config", missingDir, "--slug", "abc", "--name", "A"},
		{"member", "add", "--config", missingDir, "--tenant", "abc", "--email", "a@b.c"},
		{"seed-demo", "--config", missingDir},
		{"user", "create", "--config", bad, "--email", "a@b.c"},
	} {
		if r := run(t, "", args...); r.code != 1 {
			t.Errorf("%v: %+v", args, r)
		}
	}
}

func TestUserTenantMember(t *testing.T) {
	cfg := writeConfig(t, "")
	if r := run(t, "", "user", "create", "--config", cfg); r.code != 1 || !strings.Contains(r.stderr, "--email is required") {
		t.Errorf("missing email: %+v", r)
	}
	if r := run(t, "short\n", "user", "create", "--config", cfg, "--email", "weak@example.com", "--password-stdin"); r.code != 1 || !strings.Contains(r.stderr, "password must be") {
		t.Errorf("weak password: %+v", r)
	}
	var out, errb bytes.Buffer
	code := Run([]string{"user", "create", "--config", cfg, "--email", "x@example.com", "--password-stdin"},
		Env{Stdin: iotest.ErrReader(io.ErrUnexpectedEOF), Stdout: &out, Stderr: &errb})
	if code != 1 {
		t.Errorf("stdin error: %d %s", code, errb.String())
	}
	r := run(t, strongPassword+"\r\n", "user", "create", "--config", cfg, "--email", "Alice@Example.com", "--name", "Alice", "--password-stdin", "--super-admin")
	if r.code != 0 || !strings.Contains(r.stdout, "created user alice@example.com") {
		t.Fatalf("user create: %+v", r)
	}
	// Password without trailing newline (EOF) also works.
	if r := run(t, strongPassword, "user", "create", "--config", cfg, "--email", "bob@example.com", "--password-stdin"); r.code != 0 {
		t.Errorf("user create EOF: %+v", r)
	}
	if r := run(t, "", "user", "create", "--config", cfg, "--email", "alice@example.com"); r.code != 1 {
		t.Errorf("duplicate user: %+v", r)
	}
	if r := run(t, "", "user", "create", "--config", cfg, "--email", "not-an-email"); r.code != 1 {
		t.Errorf("invalid email: %+v", r)
	}

	if r := run(t, "", "tenant", "create", "--config", cfg, "--slug", "acme", "--name", "Acme"); r.code != 0 || !strings.Contains(r.stdout, "created tenant acme") {
		t.Fatalf("tenant create: %+v", r)
	}
	if r := run(t, "", "tenant", "create", "--config", cfg, "--slug", "!", "--name", "Bad"); r.code != 1 {
		t.Errorf("bad slug: %+v", r)
	}

	if r := run(t, "", "member", "add", "--config", cfg, "--tenant", "acme", "--email", "alice@example.com", "--role", "god"); r.code != 1 || !strings.Contains(r.stderr, `invalid role "god"`) {
		t.Errorf("invalid role: %+v", r)
	}
	if r := run(t, "", "member", "add", "--config", cfg, "--tenant", "nope", "--email", "alice@example.com", "--role", "admin"); r.code != 1 || !strings.Contains(r.stderr, "not found") {
		t.Errorf("unknown tenant: %+v", r)
	}
	if r := run(t, "", "member", "add", "--config", cfg, "--tenant", "acme", "--email", "ghost@example.com"); r.code != 1 {
		t.Errorf("unknown user: %+v", r)
	}
	if r := run(t, "", "member", "add", "--config", cfg, "--tenant", "acme", "--email", "alice@example.com", "--role", "manager"); r.code != 0 || r.stdout != "alice@example.com is now manager in acme\n" {
		t.Errorf("member add: %+v", r)
	}

	st := openTestStore(t, cfg)
	u, err := st.GetUserByEmail(context.Background(), "alice@example.com")
	if err != nil || !u.SuperAdmin || u.DisplayName != "Alice" || !auth.CheckPassword(u.PasswordHash, strongPassword) {
		t.Errorf("stored user %+v %v", u, err)
	}
}

func TestSeedDemo(t *testing.T) {
	cfg := writeConfig(t, "")
	r := run(t, "", "seed-demo", "--config", cfg)
	shared := "seeded restricted project VAULT: shared with dana@example.com (viewer); erik@example.com is not a member (password of both: Demo-user-1234)\n"
	if r.code != 0 || r.stdout != "seeded demo project DEMO in tenant default\n"+shared {
		t.Fatalf("seed 1: %+v", r)
	}
	r = run(t, "", "seed-demo", "--config", cfg)
	if r.code != 0 || r.stdout != "seeded demo project DEMO2 in tenant default\n" {
		t.Fatalf("seed 2: %+v", r)
	}
	r = run(t, "", "seed-demo", "--config", cfg, "--tenant", "other")
	if r.code != 0 || r.stdout != "seeded demo project DEMO in tenant other\n"+shared {
		t.Fatalf("seed other: %+v", r)
	}
	if r := run(t, "", "seed-demo", "--config", cfg, "--tenant", "X!"); r.code != 1 {
		t.Errorf("bad tenant slug: %+v", r)
	}
	st := openTestStore(t, cfg)
	tn, err := st.GetTenantBySlug(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := st.ListProjects(context.Background(), tn.ID)
	if err != nil || len(ps) != 3 {
		t.Errorf("projects %v %v", ps, err)
	}
	// the restricted project, shared with Dana as a viewer
	var vault *model.Project
	for i := range ps {
		if ps[i].Key == "VAULT" {
			vault = &ps[i]
		}
	}
	if vault == nil || vault.Access != model.ProjectAccessRestricted {
		t.Fatalf("vault %+v", vault)
	}
	dana, err := st.GetUserByEmail(context.Background(), DemoMemberEmail)
	if err != nil {
		t.Fatal(err)
	}
	if g, err := st.ProjectGrantOf(context.Background(), tn.ID, vault.ID, dana.ID); err != nil || g.UserRole != "viewer" {
		t.Errorf("grant %+v %v", g, err)
	}
	ps = slices.DeleteFunc(ps, func(p model.Project) bool { return p.Key == "VAULT" })
	// the web test against the server itself
	set, err := st.WebTestSettings(context.Background(), tn.ID, ps[0].ID)
	if err != nil || len(set.Variables) != 1 {
		t.Errorf("web settings %+v %v", set, err)
	}
	tests, _ := st.ListTests(context.Background(), tn.ID, ps[0].ID)
	web := 0
	for _, tc := range tests {
		if tc.ScriptLanguage == "playwright" && strings.Contains(tc.Script, "page.goto('/login')") {
			web++
		}
	}
	if web != 1 {
		t.Errorf("%d web tests", web)
	}
	// with bootstrap credentials the administrator is created before the
	// demo users (who would otherwise stop the bootstrap of the first start)
	bcfg := writeConfig(t, "bootstrap: {admin_email: boss@example.com, admin_password: "+strongPassword+"}")
	if r := run(t, "", "seed-demo", "--config", bcfg); r.code != 0 {
		t.Fatalf("seed with bootstrap: %+v", r)
	}
	if u, err := openTestStore(t, bcfg).GetUserByEmail(context.Background(), "boss@example.com"); err != nil || !u.SuperAdmin {
		t.Errorf("bootstrap admin %+v %v", u, err)
	}
	if itoa(0) != "0" || itoa(123) != "123" {
		t.Errorf("itoa: %q %q", itoa(0), itoa(123))
	}
}

func TestSeedDemoClosedStore(t *testing.T) {
	d, err := db.Open(context.Background(), "sqlite", ":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	if _, err := SeedDemo(context.Background(), store.New(d), "t"); err == nil {
		t.Error("expected error on closed database")
	}
}

// ---------------------------------------------------------------- bootstrap

func TestBootstrap(t *testing.T) {
	ctx := context.Background()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	st := openTestStore(t, writeConfig(t, ""))
	// Missing credentials: warn only.
	if err := Bootstrap(ctx, st, config.Bootstrap{TenantSlug: "default", TenantName: "Default"}, log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "no users exist") {
		t.Errorf("logs: %s", logs.String())
	}
	// Weak password.
	err := Bootstrap(ctx, st, config.Bootstrap{TenantSlug: "default", AdminEmail: "a@example.com", AdminPassword: "weak"}, log)
	if err == nil || !strings.Contains(err.Error(), "bootstrap admin password") {
		t.Errorf("weak: %v", err)
	}
	// Invalid tenant slug.
	if err := Bootstrap(ctx, st, config.Bootstrap{TenantSlug: "!", AdminEmail: "a@example.com", AdminPassword: strongPassword}, log); err == nil {
		t.Error("bad slug accepted")
	}
	// Invalid e-mail.
	if err := Bootstrap(ctx, st, config.Bootstrap{TenantSlug: "default", TenantName: "Default", AdminEmail: "nope", AdminPassword: strongPassword}, log); err == nil {
		t.Error("bad email accepted")
	}
	// Success (tenant "default" may already exist from the previous attempt).
	b := config.Bootstrap{TenantSlug: "default", TenantName: "Default", AdminEmail: "root@example.com", AdminPassword: strongPassword}
	if err := Bootstrap(ctx, st, b, log); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUserByEmail(ctx, "root@example.com")
	if err != nil || !u.SuperAdmin || !auth.CheckPassword(u.PasswordHash, strongPassword) {
		t.Fatalf("admin %+v %v", u, err)
	}
	// Users exist: no-op.
	logs.Reset()
	if err := Bootstrap(ctx, st, config.Bootstrap{TenantSlug: "x", AdminEmail: "other@example.com", AdminPassword: strongPassword}, log); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetUserByEmail(ctx, "other@example.com"); err == nil {
		t.Error("bootstrap ran although users exist")
	}
	if n, _ := st.CountUsers(ctx); n != 1 {
		t.Errorf("users = %d", n)
	}

	// Fresh store, tenant created by bootstrap.
	st2 := openTestStore(t, writeConfig(t, ""))
	if err := Bootstrap(ctx, st2, config.Bootstrap{TenantSlug: "corp", TenantName: "Corp", AdminEmail: "x@example.com", AdminPassword: strongPassword}, log); err != nil {
		t.Fatal(err)
	}
	if _, err := st2.GetTenantBySlug(ctx, "corp"); err != nil {
		t.Error(err)
	}

	// Closed database.
	d, _ := db.Open(ctx, "sqlite", ":memory:", 0)
	d.Close()
	if err := Bootstrap(ctx, store.New(d), b, log); err == nil {
		t.Error("closed db: no error")
	}
}

// ---------------------------------------------------------------- hardening options

func TestHardeningOptions(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "/run/credentials/qtf")
	cfg := config.Default()
	cfg.Hardening = config.Hardening{NoNewPrivs: true, Landlock: true, Seccomp: true, DropCapabilities: true,
		ReadOnlyPaths: []string{"/srv/ro"}, ReadWritePaths: []string{"/srv/rw"}}
	cfg.Server.TLSCert, cfg.Server.TLSKey = "/etc/tls/c.pem", "/etc/tls/k.pem"
	cfg.Logging.Syslog = config.Syslog{Enabled: true, Network: "unixgram", Address: "/dev/log", CAFile: "/etc/ca.pem"}
	cfg.Auth.SAML = []config.SAML{{Name: "s", IDPMetadataFile: "/etc/idp.xml", SPCertFile: "/etc/sp.crt", SPKeyFile: "/etc/sp.key"}, {Name: "u", IDPMetadataURL: "https://x"}}
	cfg.Database.DSN = "file:/var/lib/qtf/db.sqlite?mode=rwc"
	cfg.Runner.Enabled = true
	cfg.Runner.WorkDir = "/var/lib/qtf/runner"
	cfg.Runner.Interpreters = map[string]string{"python": "/opt/py/bin/python3"}

	o := hardeningOptions(cfg)
	if !o.NoNewPrivs || !o.Landlock || !o.Seccomp || !o.DropCapabilities || len(o.KeepCapabilities) != 1 || o.KeepCapabilities[0] != 10 {
		t.Errorf("flags %+v", o)
	}
	has := func(list []string, p string) bool {
		for _, x := range list {
			if x == p {
				return true
			}
		}
		return false
	}
	for _, p := range []string{"/srv/ro", "/etc/tls/c.pem", "/etc/tls/k.pem", "/etc/ca.pem", "/run/credentials/qtf", "/etc/idp.xml", "/etc/sp.crt", "/etc/sp.key"} {
		if !has(o.ReadOnlyPaths, p) {
			t.Errorf("RO missing %s: %v", p, o.ReadOnlyPaths)
		}
	}
	if has(o.ReadOnlyPaths, "") {
		t.Error("empty RO path")
	}
	for _, p := range []string{os.TempDir(), "/srv/rw", "/var/lib/qtf", "/dev/log", "/var/lib/qtf/runner"} {
		if !has(o.ReadWritePaths, p) {
			t.Errorf("RW missing %s: %v", p, o.ReadWritePaths)
		}
	}
	self, _ := os.Executable()
	for _, p := range []string{"/usr", "/bin", "/opt/py/bin", filepath.Dir(self)} {
		if !has(o.ExecPaths, p) {
			t.Errorf("exec missing %s: %v", p, o.ExecPaths)
		}
	}
	// web tests: Node, Playwright and the browsers
	cfg.Runner.Web = config.WebRunner{Enabled: true, Node: "/opt/node/bin/node", PlaywrightDir: "/srv/pw", BrowsersPath: "/srv/browsers",
		ExecutablePath: "/srv/chrome/chrome", Command: "/srv/bin/playwright test"}
	ow := hardeningOptions(cfg)
	for _, p := range []string{"/opt/node/bin", "/srv/pw", "/srv/browsers", "/srv/chrome", "/srv/bin"} {
		if !has(ow.ExecPaths, p) {
			t.Errorf("web exec missing %s: %v", p, ow.ExecPaths)
		}
	}
	if !has(ow.ReadWritePaths, "/dev/shm") || !has(ow.ReadOnlyPaths, "/proc") {
		t.Errorf("web paths %v %v", ow.ReadWritePaths, ow.ReadOnlyPaths)
	}
	cfg.Runner.Web = config.Default().Runner.Web

	// Input slices are not aliased.
	o.ReadOnlyPaths[0] = "changed"
	if cfg.Hardening.ReadOnlyPaths[0] != "/srv/ro" {
		t.Error("ReadOnlyPaths aliased")
	}

	// Relative sqlite DSN, no runner, no syslog, no credentials dir.
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	cfg = config.Default()
	cfg.Database.DSN = "data/q.db"
	o = hardeningOptions(cfg)
	abs, _ := filepath.Abs("data")
	if !has(o.ReadWritePaths, abs) || len(o.ExecPaths) != 0 || len(o.ReadOnlyPaths) != 0 {
		t.Errorf("relative: %+v", o)
	}
	// In-memory and postgres databases add nothing.
	cfg.Database.DSN = ":memory:"
	if o := hardeningOptions(cfg); len(o.ReadWritePaths) != 1 {
		t.Errorf("memory: %v", o.ReadWritePaths)
	}
	cfg.Database = config.Database{Driver: "postgres", DSN: "postgres://x"}
	if o := hardeningOptions(cfg); len(o.ReadWritePaths) != 1 {
		t.Errorf("postgres: %v", o.ReadWritePaths)
	}
}

// ---------------------------------------------------------------- serve

type server struct {
	addr   string
	cancel context.CancelFunc
	done   chan int
	stderr *syncBuffer
}

type syncBuffer struct {
	mu chan struct{}
	b  bytes.Buffer
}

func newSyncBuffer() *syncBuffer { s := &syncBuffer{mu: make(chan struct{}, 1)}; return s }
func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	return s.b.String()
}

func startServe(t *testing.T, cfgPath string, extra ...string) *server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &server{cancel: cancel, done: make(chan int, 1), stderr: newSyncBuffer()}
	ready := make(chan string, 1)
	go func() {
		s.done <- Run(append([]string{"serve", "--config", cfgPath}, extra...), Env{
			Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: s.stderr, Ctx: ctx,
			Ready: func(addr string) { ready <- addr },
		})
	}()
	select {
	case s.addr = <-ready:
	case code := <-s.done:
		cancel()
		t.Fatalf("serve exited early with %d: %s", code, s.stderr.String())
	case <-time.After(20 * time.Second):
		cancel()
		t.Fatal("serve did not become ready")
	}
	t.Cleanup(func() { s.stop(t) })
	return s
}

func (s *server) stop(t *testing.T) {
	t.Helper()
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
	select {
	case code := <-s.done:
		if code != 0 {
			t.Errorf("serve exited with %d: %s", code, s.stderr.String())
		}
	case <-time.After(20 * time.Second):
		t.Error("serve did not shut down")
	}
}

func get(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestServeHTTP(t *testing.T) {
	cfg := writeConfig(t, fmt.Sprintf("bootstrap: {admin_email: boot@example.com, admin_password: %q}\nrunner: {enabled: true, timeout: 5s, work_dir: %q}", strongPassword, filepath.Join(t.TempDir(), "rw")))
	s := startServe(t, cfg)
	c := &http.Client{Timeout: 5 * time.Second}
	for _, p := range []string{"/healthz", "/readyz"} {
		if code := get(t, c, "http://"+s.addr+p); code != http.StatusOK {
			t.Errorf("%s: %d", p, code)
		}
	}
	// healthcheck sub-command against the live server.
	if r := run(t, "", "healthcheck", "--url", "http://"+s.addr+"/healthz"); r.code != 0 || r.stdout != "ok\n" {
		t.Errorf("healthcheck: %+v", r)
	}
	s.stop(t)
	// The bootstrap admin was created.
	st := openTestStore(t, cfg)
	if _, err := st.GetUserByEmail(context.Background(), "boot@example.com"); err != nil {
		t.Errorf("bootstrap admin: %v", err)
	}
}

func TestServeTLSAndReload(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := genCert(t, dir)
	cfg := writeConfig(t, fmt.Sprintf("server: {tls_cert: %q, tls_key: %q}", certFile, keyFile))
	s := startServe(t, cfg)
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if code := get(t, c, "https://"+s.addr+"/healthz"); code != http.StatusOK {
		t.Errorf("https healthz: %d", code)
	}
	if r := run(t, "", "healthcheck", "--url", "https://"+s.addr+"/healthz"); r.code != 0 {
		t.Errorf("healthcheck https: %+v", r)
	}
	s.stop(t)
}

func TestServeErrors(t *testing.T) {
	dir := t.TempDir()
	certFile, _ := genCert(t, dir)
	badKey := filepath.Join(dir, "bad.key")
	os.WriteFile(badKey, []byte("not a key"), 0o600)
	file := filepath.Join(dir, "file")
	os.WriteFile(file, nil, 0o600)
	idp := filepath.Join(dir, "idp.xml")
	os.WriteFile(idp, []byte("<x/>"), 0o600)

	cases := map[string]struct {
		cfg  string
		want string
	}{
		"bad config":    {writeConfig(t, "auth: {session_ttl: 0s}"), "session_ttl"},
		"bad log level": {writeConfig(t, "logging: {level: shouting}"), "logging:"},
		"bad tls key":   {writeConfig(t, fmt.Sprintf("server: {tls_cert: %q, tls_key: %q}", certFile, badKey)), "tls:"},
		"bad listen":    {writeConfig(t, "server: {listen: not-an-address}"), "listen"},
		"runner dir":    {writeConfig(t, fmt.Sprintf("runner: {enabled: true, timeout: 1s, work_dir: %q}", filepath.Join(file, "sub"))), "runner work dir"},
		"bootstrap":     {writeConfig(t, "bootstrap: {admin_email: a@example.com, admin_password: weak}"), "bootstrap admin password"},
		"api":           {writeConfig(t, fmt.Sprintf("server: {base_url: 'http://[::1'}\nauth: {saml: [{name: s, idp_metadata_file: %q}]}", idp)), "base url"},
		"syslog":        {writeConfig(t, "logging: {syslog: {enabled: true, network: udp, address: '127.0.0.1:1', facility: bogus}}"), "facility"},
		"db":            {writeConfig(t, "database: {driver: postgres, dsn: 'postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=2'}"), "db:"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := run(t, "", "serve", "--config", c.cfg)
			if r.code != 1 || !strings.Contains(r.stderr, c.want) {
				t.Errorf("code=%d stderr=%s", r.code, r.stderr)
			}
		})
	}
}

// ---------------------------------------------------------------- healthcheck

func TestHealthcheck(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	}))
	defer ok.Close()
	okTLS := httptest.NewTLSServer(ok.Config.Handler)
	defer okTLS.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failing.Close()

	_, okPort, _ := net.SplitHostPort(strings.TrimPrefix(ok.URL, "http://"))
	_, tlsPort, _ := net.SplitHostPort(strings.TrimPrefix(okTLS.URL, "https://"))

	cfgListen := func(listen string, tlsOn bool) string {
		extra := fmt.Sprintf("server: {listen: %q}", listen)
		if tlsOn {
			extra = fmt.Sprintf("server: {listen: %q, tls_cert: /nonexistent/c.pem, tls_key: /nonexistent/k.pem}", listen)
		}
		return writeConfig(t, extra)
	}

	for name, args := range map[string][]string{
		"explicit url": {"--url", ok.URL + "/healthz"},
		"empty host":   {"--config", cfgListen(":"+okPort, false)},
		"0.0.0.0":      {"--config", cfgListen("0.0.0.0:"+okPort, false)},
		"ipv6 any":     {"--config", cfgListen("[::]:"+okPort, false)},
		"loopback":     {"--config", cfgListen("127.0.0.1:"+okPort, false)},
		"https":        {"--config", cfgListen("0.0.0.0:"+tlsPort, true)},
	} {
		if r := run(t, "", append([]string{"healthcheck"}, args...)...); r.code != 0 || r.stdout != "ok\n" {
			t.Errorf("%s: %+v", name, r)
		}
	}
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"non-200":     {[]string{"--url", failing.URL + "/healthz"}, "status 503"},
		"404":         {[]string{"--url", ok.URL + "/other"}, "status 404"},
		"refused":     {[]string{"--url", "http://127.0.0.1:1/healthz"}, "error:"},
		"bad listen":  {[]string{"--config", cfgListen("nonsense", false)}, "must be host:port"},
		"bad config":  {[]string{"--config", writeConfig(t, "auth: {session_ttl: 0s}")}, "session_ttl"},
		"http on tls": {[]string{"--config", cfgListen("127.0.0.1:"+okPort, true)}, "error:"},
	} {
		if r := run(t, "", append([]string{"healthcheck"}, c.args...)...); r.code != 1 || !strings.Contains(r.stderr, c.want) {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestServePortFlag(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	s := startServe(t, writeConfig(t, ""), "-p", strconv.Itoa(port))
	if want := fmt.Sprintf("127.0.0.1:%d", port); s.addr != want {
		t.Fatalf("listening on %s, want %s", s.addr, want)
	}
	c := &http.Client{Timeout: 5 * time.Second}
	if code := get(t, c, "http://"+s.addr+"/healthz"); code != http.StatusOK {
		t.Fatal(code)
	}
	// healthcheck derives the URL from the same flag
	if r := run(t, "", "healthcheck", "--config", writeConfig(t, ""), "--port", strconv.Itoa(port)); r.code != 0 {
		t.Errorf("healthcheck --port: %+v", r)
	}
	s.stop(t)
	// --listen and invalid ports
	if r := run(t, "", "check-config", "--config", writeConfig(t, ""), "--listen", "0.0.0.0:9443"); r.code != 0 || !strings.Contains(r.stdout, "listen 0.0.0.0:9443") {
		t.Errorf("check-config --listen: %+v", r)
	}
	if r := run(t, "", "serve", "--config", writeConfig(t, ""), "--port", "70000"); r.code != 1 || !strings.Contains(r.stderr, "out of range") {
		t.Errorf("serve --port 70000: %+v", r)
	}
}
