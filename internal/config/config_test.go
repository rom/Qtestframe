package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// clearEnv makes sure ambient QTF_* variables do not influence the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "QTF_") || k == "CREDENTIALS_DIRECTORY" {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
	}
}

func TestDefaultIsValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if c.Database.Driver != "sqlite" || c.Server.Listen != ":8080" || !c.Runner.Sandbox || !c.Hardening.Seccomp {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestLoadNoFile(t *testing.T) {
	clearEnv(t)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.DSN != "qtestframe.db" {
		t.Errorf("dsn = %q", c.Database.DSN)
	}
}

func TestLoadYAML(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "c.yaml", `
server:
  listen: 127.0.0.1:9999
  read_timeout: 5s
  trusted_proxies: [10.0.0.0/8]
database:
  driver: postgres
  dsn: postgres://x
auth:
  session_ttl: 1h
  oidc:
    - name: corp
      issuer: https://idp
      client_id: cid
      client_secret: inline
      auto_provision: true
      default_role: tester
      role_mapping: {admins: admin}
  saml:
    - name: adfs
      idp_metadata_url: https://idp/md
runner:
  enabled: true
  timeout: 10s
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Listen != "127.0.0.1:9999" || c.Server.ReadTimeout != 5*time.Second || c.Server.TrustedProxies[0] != "10.0.0.0/8" {
		t.Errorf("server: %+v", c.Server)
	}
	if c.Database.Driver != "postgres" || c.Auth.SessionTTL != time.Hour {
		t.Errorf("db/auth: %+v %+v", c.Database, c.Auth)
	}
	o := c.Auth.OIDC[0]
	if o.ClientSecret != "inline" || !o.AutoProvision || o.DefaultRole != "tester" || o.RoleMapping["admins"] != "admin" {
		t.Errorf("oidc: %+v", o)
	}
	// Defaults not in the file are kept.
	if c.Server.WriteTimeout != 120*time.Second || c.Logging.Syslog.Facility != "local0" {
		t.Errorf("defaults lost: %+v", c)
	}
}

func TestLoadFromQTFConfigEnv(t *testing.T) {
	clearEnv(t)
	p := writeFile(t, t.TempDir(), "c.yaml", "server:\n  listen: ':1234'\n")
	t.Setenv("QTF_CONFIG", p)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Listen != ":1234" {
		t.Errorf("listen = %q", c.Server.Listen)
	}
}

func TestLoadErrors(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil || !strings.Contains(err.Error(), "config:") {
		t.Errorf("missing file: %v", err)
	}
	bad := writeFile(t, dir, "bad.yaml", "server: [unclosed\n")
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("bad yaml: %v", err)
	}
	invalid := writeFile(t, dir, "invalid.yaml", "database:\n  driver: mysql\n")
	if _, err := Load(invalid); err == nil || !strings.Contains(err.Error(), "database.driver") {
		t.Errorf("invalid: %v", err)
	}
}

func TestEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("QTF_DATABASE_DRIVER", "postgres")
	t.Setenv("QTF_DATABASE_DSN", "postgres://env")
	t.Setenv("QTF_SERVER_LISTEN", ":7000")
	t.Setenv("QTF_SERVER_BASE_URL", "https://qtf.example")
	t.Setenv("QTF_LOG_LEVEL", "debug")
	t.Setenv("QTF_LOG_FORMAT", "json")
	t.Setenv("QTF_BOOTSTRAP_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("QTF_BOOTSTRAP_ADMIN_PASSWORD", "pw")
	t.Setenv("QTF_COOKIE_SECURE", "false")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.Driver != "postgres" || c.Database.DSN != "postgres://env" || c.Server.Listen != ":7000" ||
		c.Server.BaseURL != "https://qtf.example" || c.Logging.Level != "debug" || c.Logging.Format != "json" ||
		c.Bootstrap.AdminEmail != "admin@example.com" || c.Bootstrap.AdminPassword != "pw" || c.Auth.CookieSecure {
		t.Errorf("env overrides not applied: %+v", c)
	}
	t.Setenv("QTF_RUNNER_ENABLED", "1")
	t.Setenv("QTF_RUNNER_WEB_ENABLED", "true")
	t.Setenv("QTF_RUNNER_WORK_DIR", "/var/lib/q/runner")
	t.Setenv("QTF_RUNNER_WEB_NODE", "/usr/local/bin/node")
	t.Setenv("QTF_RUNNER_WEB_PLAYWRIGHT_DIR", "/opt/pw")
	t.Setenv("QTF_RUNNER_WEB_BROWSERS_PATH", "/opt/pw/browsers")
	if c, err := Load(""); err != nil || !c.Runner.Enabled || !c.Runner.Web.Enabled || c.Runner.WorkDir != "/var/lib/q/runner" ||
		c.Runner.Web.Node != "/usr/local/bin/node" || c.Runner.Web.PlaywrightDir != "/opt/pw" || c.Runner.Web.BrowsersPath != "/opt/pw/browsers" {
		t.Errorf("runner env: %+v %v", c.Runner, err)
	}
	for v, want := range map[string]bool{"1": true, "TRUE": true, "true": true, "0": false, "yes": false} {
		t.Setenv("QTF_COOKIE_SECURE", v)
		c, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if c.Auth.CookieSecure != want {
			t.Errorf("QTF_COOKIE_SECURE=%q -> %v", v, c.Auth.CookieSecure)
		}
	}
}

func TestSecretFiles(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	creds := t.TempDir()
	dsn := writeFile(t, dir, "dsn", "  file:secret.db \n")
	writeFile(t, creds, "admin-pw", "S3cret-password\n")
	writeFile(t, creds, "oidc-secret", "from-file\n")
	cfgPath := writeFile(t, dir, "c.yaml", `
database:
  dsn_file: `+dsn+`
bootstrap:
  admin_password_file: admin-pw
auth:
  oidc:
    - name: corp
      issuer: https://idp
      client_id: cid
      client_secret_file: oidc-secret
    - name: other
      issuer: https://idp2
      client_id: cid2
      client_secret: keep
`)
	t.Setenv("CREDENTIALS_DIRECTORY", creds)
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.DSN != "file:secret.db" {
		t.Errorf("dsn = %q", c.Database.DSN)
	}
	if c.Bootstrap.AdminPassword != "S3cret-password" {
		t.Errorf("admin password = %q", c.Bootstrap.AdminPassword)
	}
	if c.Auth.OIDC[0].ClientSecret != "from-file" || c.Auth.OIDC[1].ClientSecret != "keep" {
		t.Errorf("oidc secrets = %q %q", c.Auth.OIDC[0].ClientSecret, c.Auth.OIDC[1].ClientSecret)
	}

	// Environment variable wins over the file.
	t.Setenv("QTF_OIDC_CORP_CLIENT_SECRET", "from-env")
	c, err = Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.OIDC[0].ClientSecret != "from-env" || c.Auth.OIDC[1].ClientSecret != "keep" {
		t.Errorf("env secret: %q %q", c.Auth.OIDC[0].ClientSecret, c.Auth.OIDC[1].ClientSecret)
	}
}

func TestSecretFileErrors(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	for name, body := range map[string]string{
		"dsn":   "database:\n  dsn_file: " + missing + "\n",
		"admin": "bootstrap:\n  admin_password_file: " + missing + "\n",
		"oidc":  "auth:\n  oidc:\n    - {name: a, issuer: i, client_id: c, client_secret_file: " + missing + "}\n",
	} {
		p := writeFile(t, dir, name+".yaml", body)
		if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "config: secret") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// Relative path without CREDENTIALS_DIRECTORY is resolved against cwd.
	if _, err := readSecret("definitely-not-here-qtf"); err == nil {
		t.Error("expected error for relative missing secret")
	}
}

func TestValidate(t *testing.T) {
	ok := Default
	cases := []struct {
		name string
		mod  func(*Config)
		want string
	}{
		{"driver", func(c *Config) { c.Database.Driver = "mysql" }, "database.driver"},
		{"dsn", func(c *Config) { c.Database.DSN = "" }, "database.dsn is required"},
		{"tls cert only", func(c *Config) { c.Server.TLSCert = "c.pem" }, "tls_cert and server.tls_key"},
		{"tls key only", func(c *Config) { c.Server.TLSKey = "k.pem" }, "tls_cert and server.tls_key"},
		{"ttl", func(c *Config) { c.Auth.SessionTTL = 0 }, "session_ttl"},
		{"oidc missing", func(c *Config) { c.Auth.OIDC = []OIDC{{Name: "x"}} }, "name, issuer and client_id"},
		{"oidc dup", func(c *Config) {
			c.Auth.OIDC = []OIDC{{Name: "x", Issuer: "i", ClientID: "c"}, {Name: "x", Issuer: "i", ClientID: "c"}}
		}, `duplicate oidc provider "x"`},
		{"saml missing", func(c *Config) { c.Auth.SAML = []SAML{{Name: "s"}} }, "name and idp metadata"},
		{"saml dup", func(c *Config) {
			c.Auth.SAML = []SAML{{Name: "s", IDPMetadataFile: "f"}, {Name: "s", IDPMetadataURL: "u"}}
		}, `duplicate saml provider "s"`},
		{"syslog network", func(c *Config) {
			c.Logging.Syslog = Syslog{Enabled: true, Network: "carrier-pigeon", Address: "x"}
		}, "logging.syslog.network"},
		{"syslog address", func(c *Config) { c.Logging.Syslog = Syslog{Enabled: true, Network: "udp"} }, "logging.syslog.address"},
		{"runner timeout", func(c *Config) { c.Runner.Enabled = true; c.Runner.Timeout = 0 }, "runner.timeout"},
		{"web parallel", func(c *Config) { c.Runner.Web.MaxParallel = 0 }, "max_parallel"},
		{"web install", func(c *Config) { c.Runner.Web.Enabled = true }, "playwright_dir"},
		{"web browser", func(c *Config) { c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.Browser = true, "pw", "lynx" }, "runner.web.browser"},
		{"web screenshots", func(c *Config) {
			c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.Screenshots = true, "pw", "never"
		}, "runner.web.screenshots"},
		{"web trace", func(c *Config) { c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.Trace = true, "pw", "x" }, "runner.web.trace"},
		{"web video", func(c *Config) { c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.Video = true, "pw", "on" }, "runner.web.video"},
		{"web timeout", func(c *Config) { c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.Timeout = true, "pw", 0 }, "runner.web.timeout"},
		{"web evidence", func(c *Config) {
			c.Runner.Web.Enabled, c.Runner.Web.Command, c.Runner.Web.MaxEvidenceMB = true, "pw", -1
		}, "max_evidence_mb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ok()
			tc.mod(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}

	// Valid combinations.
	c := ok()
	c.Database.Driver = "postgres"
	c.Server.TLSCert, c.Server.TLSKey = "c", "k"
	c.Auth.OIDC = []OIDC{{Name: "a", Issuer: "i", ClientID: "c"}}
	c.Auth.SAML = []SAML{{Name: "a", IDPMetadataURL: "u"}} // same name, different kind
	c.Runner.Enabled = true
	for _, n := range []string{"udp", "tcp", "tls", "unix", "unixgram"} {
		c.Logging.Syslog = Syslog{Enabled: true, Network: n, Address: "a"}
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}

	// a complete web runner
	c = ok()
	c.Runner.Web.Enabled, c.Runner.Web.PlaywrightDir = true, "/opt/pw"
	if err := c.Validate(); err != nil {
		t.Errorf("web: %v", err)
	}

	// Multiple errors are joined.
	c = ok()
	c.Database.Driver, c.Database.DSN = "x", ""
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "database.driver") || !strings.Contains(err.Error(), "database.dsn") {
		t.Errorf("joined: %v", err)
	}
}

func TestCommandLineOverrides(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "c.yaml", "server:\n  listen: \"127.0.0.1:8443\"\n")
	cases := []struct {
		o    Overrides
		env  string
		want string
	}{
		{Overrides{}, "", "127.0.0.1:8443"},
		{Overrides{Port: 9000}, "", "127.0.0.1:9000"},
		{Overrides{Listen: ":7000"}, "", ":7000"},
		{Overrides{Listen: "[::1]:7000", Port: 7001}, "", "[::1]:7001"},
		{Overrides{}, "9100", "127.0.0.1:9100"},           // QTF_SERVER_PORT beats the file
		{Overrides{Port: 9200}, "9100", "127.0.0.1:9200"}, // flag beats the environment
	}
	for _, c := range cases {
		if c.env != "" {
			t.Setenv("QTF_SERVER_PORT", c.env)
		} else {
			os.Unsetenv("QTF_SERVER_PORT")
		}
		cfg, err := LoadWith(p, c.o)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if cfg.Server.Listen != c.want {
			t.Errorf("%+v: listen %q, want %q", c, cfg.Server.Listen, c.want)
		}
	}
	os.Unsetenv("QTF_SERVER_PORT")
	for _, port := range []int{-1, 65536} {
		if _, err := LoadWith(p, Overrides{Port: port}); err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Errorf("port %d: %v", port, err)
		}
	}
	// a listen value without a port is replaced entirely by host-less --port
	var c Config
	c.Server.Listen = "garbage"
	if err := (Overrides{Port: 80}).Apply(&c); err != nil || c.Server.Listen != ":80" {
		t.Fatal(c.Server.Listen, err)
	}
	for _, bad := range []string{"nonsense", "host:notaport", "host:99999"} {
		if _, err := LoadWith("", Overrides{Listen: bad}); err == nil {
			t.Errorf("listen %q accepted", bad)
		}
	}
}

func TestStrictYAML(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		"typo":      "server:\n  listn: \":1\"\n",
		"top-level": "srever: {}\n",
		"two docs":  "server: {listen: \":1\"}\n---\nserver: {listen: \":2\"}\n",
		"not yaml":  "server: [unclosed\n",
	} {
		if _, err := Load(writeFile(t, dir, "x.yaml", content)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// empty and comment-only files give the defaults
	cfg, err := Load(writeFile(t, dir, "e.yaml", "# nothing here\n"))
	if err != nil || cfg.Server.Listen != ":8080" || !cfg.Requirements.AutoCreateTest {
		t.Fatal(cfg.Server.Listen, err)
	}
	cfg, err = Load(writeFile(t, dir, "r.yaml", "requirements:\n  auto_create_test: false\n"))
	if err != nil || cfg.Requirements.AutoCreateTest {
		t.Fatal(err)
	}
}

// The YAML files shipped in the repository must stay valid (strict keys).
func TestShippedConfigFilesLoad(t *testing.T) {
	clearEnv(t)
	for _, p := range []string{"../../examples/qtestframe.yaml", "../../examples/qtestframe-tls.yaml", "../../deploy/config.example.yaml"} {
		cfg, err := Load(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if cfg.Bootstrap.TenantSlug == "" || cfg.Server.Listen == "" {
			t.Errorf("%s: unexpected %+v", p, cfg.Server)
		}
	}
}

func TestEmailConfig(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	if c := Default(); c.Email.On() || c.Email.Port != 587 || c.Email.TLS != "starttls" {
		t.Fatalf("defaults: %+v", c.Email)
	}
	pw := writeFile(t, dir, "smtp-pass", "s3cret\n")
	p := writeFile(t, dir, "c.yaml", `
server:
  base_url: https://qtf.example.com
email:
  host: smtp.example.com
  port: 465
  username: qtf
  password_file: `+pw+`
  from: noreply@example.com
  from_name: Qtestframe Test
  tls: tls
  digest_hour: 6
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Email.On() || c.Email.Password != "s3cret" || c.Email.Port != 465 || c.Email.DigestHour != 6 {
		t.Fatalf("%+v", c.Email)
	}
	t.Setenv("QTF_EMAIL_PASSWORD", "from-env")
	t.Setenv("QTF_EMAIL_PORT", "2525")
	t.Setenv("QTF_EMAIL_FROM", "other@example.com")
	if c, err = Load(p); err != nil || c.Email.Password != "from-env" || c.Email.Port != 2525 || c.Email.From != "other@example.com" {
		t.Fatalf("%+v %v", c.Email, err)
	}
	// a plain password in the file is not accepted (strict keys)
	bad := writeFile(t, dir, "bad.yaml", "email:\n  host: x\n  password: secret\n")
	if _, err := Load(bad); err == nil {
		t.Fatal("email.password accepted")
	}
	off := false
	for name, mut := range map[string]func(*Email){
		"port":      func(e *Email) { e.Port = 0 },
		"tls":       func(e *Email) { e.TLS = "ssl" },
		"from":      func(e *Email) { e.From = "Name <a@b.c>" },
		"from crlf": func(e *Email) { e.From = "a@b.c\r\nBcc: x@y.z" },
		"name":      func(e *Email) { e.FromName = "a\nb" },
		"user":      func(e *Email) { e.Username, e.Password = "", "x" },
		"timeout":   func(e *Email) { e.Timeout = 0 },
		"digest":    func(e *Email) { e.DigestHour = 24 },
	} {
		c := Default()
		c.Server.BaseURL = "https://x"
		c.Email.Host, c.Email.From = "smtp.example.com", "a@b.c"
		mut(&c.Email)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c = Default()
	c.Email.Host, c.Email.From = "smtp.example.com", "a@b.c"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatal("base_url not required", err)
	}
	c.Email.Enabled = &off
	if c.Email.On() || c.Validate() != nil {
		t.Fatal("disabled e-mail is validated")
	}
	bad2 := writeFile(t, dir, "missing.yaml", "email:\n  password_file: "+filepath.Join(dir, "nope")+"\n")
	if _, err := Load(bad2); err == nil {
		t.Fatal("missing password file")
	}
}
