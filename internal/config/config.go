// Package config loads the server configuration from YAML and environment.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the complete server configuration.
type Config struct {
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	Logging  Logging  `yaml:"logging"`
	Auth     Auth     `yaml:"auth"`
	Runner   Runner   `yaml:"runner"`
	// Files configures uploads (attachments, documents, avatars).
	Files Files `yaml:"files"`
	// Requirements holds behaviour defaults for requirement management.
	Requirements Requirements `yaml:"requirements"`
	Hardening    Hardening    `yaml:"hardening"`
	Bootstrap    Bootstrap    `yaml:"bootstrap"`
	// Email configures outgoing mail (notifications, invitations, reports).
	Email Email `yaml:"email"`
}

// Email configures the SMTP server used for outgoing mail. Mail is on when
// host is set, unless enabled is false. The password is never written in
// the file: it comes from password_file or QTF_EMAIL_PASSWORD.
type Email struct {
	Enabled      *bool         `yaml:"enabled"`
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	Username     string        `yaml:"username"`
	PasswordFile string        `yaml:"password_file"`
	Password     string        `yaml:"-"`
	From         string        `yaml:"from"`
	FromName     string        `yaml:"from_name"`
	TLS          string        `yaml:"tls"` // starttls | tls | none
	Timeout      time.Duration `yaml:"timeout"`
	// DigestHour is the hour of the day (UTC) when daily digests are sent.
	DigestHour int `yaml:"digest_hour"`
}

// On reports whether outgoing mail is configured and enabled.
func (e Email) On() bool {
	return e.Host != "" && (e.Enabled == nil || *e.Enabled)
}

// Server holds HTTP listener settings.
type Server struct {
	Listen         string        `yaml:"listen"`
	TLSCert        string        `yaml:"tls_cert"`
	TLSKey         string        `yaml:"tls_key"`
	BaseURL        string        `yaml:"base_url"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	TrustedProxies []string      `yaml:"trusted_proxies"`
	HSTS           bool          `yaml:"hsts"`
}

// Database selects the SQL backend.
type Database struct {
	Driver       string `yaml:"driver"` // sqlite | postgres
	DSN          string `yaml:"dsn"`
	DSNFile      string `yaml:"dsn_file"`
	MaxOpenConns int    `yaml:"max_open_conns"`
}

// Logging configures slog and optional remote syslog.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // json | text
	Syslog Syslog `yaml:"syslog"`
}

// Syslog configures shipping logs to a central syslog server.
type Syslog struct {
	Enabled  bool   `yaml:"enabled"`
	Network  string `yaml:"network"` // udp | tcp | tls | unix
	Address  string `yaml:"address"`
	Tag      string `yaml:"tag"`
	Facility string `yaml:"facility"`
	Format   string `yaml:"format"` // rfc5424 | rfc3164
	CAFile   string `yaml:"ca_file"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Auth configures authentication providers.
type Auth struct {
	Local        bool          `yaml:"local"`
	SessionTTL   time.Duration `yaml:"session_ttl"`
	CookieSecure bool          `yaml:"cookie_secure"`
	OIDC         []OIDC        `yaml:"oidc"`
	SAML         []SAML        `yaml:"saml"`
}

// Provisioning holds settings shared by external identity providers.
type Provisioning struct {
	AutoProvision bool              `yaml:"auto_provision"`
	DefaultTenant string            `yaml:"default_tenant"`
	DefaultRole   string            `yaml:"default_role"`
	RoleMapping   map[string]string `yaml:"role_mapping"`
}

// OIDC is an OpenID Connect provider.
type OIDC struct {
	Name             string   `yaml:"name"`
	DisplayName      string   `yaml:"display_name"`
	Issuer           string   `yaml:"issuer"`
	ClientID         string   `yaml:"client_id"`
	ClientSecret     string   `yaml:"client_secret"`
	ClientSecretFile string   `yaml:"client_secret_file"`
	Scopes           []string `yaml:"scopes"`
	EmailClaim       string   `yaml:"email_claim"`
	NameClaim        string   `yaml:"name_claim"`
	GroupsClaim      string   `yaml:"groups_claim"`
	// LocalMFA decides whether a sign-in through this provider also needs
	// Qtestframe's own second factor (see docs/security.md): "off" (the
	// default: the identity provider is responsible for MFA),
	// "unless_idp_mfa" (needed unless the ID token's amr/acr claims show
	// that the provider used MFA) or "always".
	LocalMFA string `yaml:"local_mfa"`
	// MFAAMRValues are amr values that count as MFA by the provider
	// (default "mfa"); MFAACRValues are acr values that do.
	MFAAMRValues []string `yaml:"mfa_amr_values"`
	MFAACRValues []string `yaml:"mfa_acr_values"`
	Provisioning `yaml:",inline"`
}

// OIDC local_mfa values.
const (
	LocalMFAOff          = "off"
	LocalMFAUnlessIdPMFA = "unless_idp_mfa"
	LocalMFAAlways       = "always"
)

// SAML is a SAML 2.0 identity provider.
type SAML struct {
	Name            string `yaml:"name"`
	DisplayName     string `yaml:"display_name"`
	IDPMetadataURL  string `yaml:"idp_metadata_url"`
	IDPMetadataFile string `yaml:"idp_metadata_file"`
	EntityID        string `yaml:"entity_id"`
	SPCertFile      string `yaml:"sp_cert_file"`
	SPKeyFile       string `yaml:"sp_key_file"`
	EmailAttr       string `yaml:"email_attr"`
	NameAttr        string `yaml:"name_attr"`
	GroupsAttr      string `yaml:"groups_attr"`
	Provisioning    `yaml:",inline"`
}

// Files configures uploaded files, which are stored in the database.
type Files struct {
	// MaxUploadMB is the maximum size of one uploaded file.
	MaxUploadMB int `yaml:"max_upload_mb"`
}

// Requirements configures requirement behaviour.
type Requirements struct {
	// AutoCreateTest creates a linked test case (one step per evaluation
	// criterion) whenever a requirement is created, unless the request
	// says "create_test": false.
	AutoCreateTest bool `yaml:"auto_create_test"`
}

// Runner configures server side execution of automated test scripts.
type Runner struct {
	Enabled        bool              `yaml:"enabled"`
	Timeout        time.Duration     `yaml:"timeout"`
	WorkDir        string            `yaml:"work_dir"`
	MaxOutputBytes int               `yaml:"max_output_bytes"`
	Interpreters   map[string]string `yaml:"interpreters"`
	Sandbox        bool              `yaml:"sandbox"`
	// DenyNetwork blocks socket creation for sandboxed scripts (seccomp).
	DenyNetwork bool `yaml:"deny_network"`
	// Web configures web tests (script language "playwright"), which run
	// in a real browser through Playwright as background jobs.
	Web WebRunner `yaml:"web"`
}

// WebRunner configures web tests run with Playwright (@playwright/test).
// They need runner.enabled as well. Web tests always have network access
// (deny_network does not apply) and run without the seccomp filter of the
// script sandbox; see docs/security.md.
type WebRunner struct {
	Enabled bool `yaml:"enabled"`
	// Node is the Node.js binary (default /usr/bin/node).
	Node string `yaml:"node"`
	// PlaywrightDir is the directory that contains
	// node_modules/@playwright/test (e.g. where `npm i @playwright/test`
	// was run). Specs resolve @playwright/test from there.
	PlaywrightDir string `yaml:"playwright_dir"`
	// Command replaces the default command
	// "<node> <playwright_dir>/node_modules/@playwright/test/cli.js", e.g.
	// "/opt/pw/node_modules/.bin/playwright"; "test --config <file>" is
	// appended. Split at spaces.
	Command string `yaml:"command"`
	// BrowsersPath is PLAYWRIGHT_BROWSERS_PATH (where `npx playwright
	// install` put the browsers); empty = Playwright's default.
	BrowsersPath string `yaml:"browsers_path"`
	// Browser is the default browser: chromium, firefox or webkit. A test
	// configuration with a Browser dimension picks another one.
	Browser string `yaml:"browser"`
	// Channel is the default Chromium channel (e.g. chrome, msedge); empty
	// = the bundled Chromium.
	Channel string `yaml:"channel"`
	// BrowserChannels: a configuration "Chrome" or "Edge" uses the
	// installed Google Chrome (channel chrome) or Microsoft Edge (msedge)
	// instead of the bundled Chromium.
	BrowserChannels bool `yaml:"browser_channels"`
	// ExecutablePath overrides the Chromium executable (launch option).
	ExecutablePath string `yaml:"executable_path"`
	Headless       bool   `yaml:"headless"`
	// Timeout is the timeout of one test (Playwright's per-test timeout).
	Timeout time.Duration `yaml:"timeout"`
	// JobTimeout bounds a whole Playwright process (all tests of a spec).
	JobTimeout time.Duration `yaml:"job_timeout"`
	// Screenshots: on_failure (default), always or off.
	Screenshots string `yaml:"screenshots"`
	// Trace: on_failure (default), always or off.
	Trace string `yaml:"trace"`
	// Video: off (default), on_failure or always.
	Video string `yaml:"video"`
	// MaxParallel is the number of jobs (web tests, or scripts started
	// with "Run automated tests") that run at the same time.
	MaxParallel int `yaml:"max_parallel"`
	// MaxEvidenceMB limits one evidence file (screenshot, trace, video);
	// larger files are left out (default files.max_upload_mb).
	MaxEvidenceMB int `yaml:"max_evidence_mb"`
}

// Hardening toggles in-process Linux hardening.
type Hardening struct {
	NoNewPrivs       bool     `yaml:"no_new_privs"`
	Landlock         bool     `yaml:"landlock"`
	Seccomp          bool     `yaml:"seccomp"`
	DropCapabilities bool     `yaml:"drop_capabilities"`
	ReadOnlyPaths    []string `yaml:"read_only_paths"`
	ReadWritePaths   []string `yaml:"read_write_paths"`
}

// Bootstrap creates an initial tenant and administrator on first start.
type Bootstrap struct {
	TenantSlug        string `yaml:"tenant_slug"`
	TenantName        string `yaml:"tenant_name"`
	AdminEmail        string `yaml:"admin_email"`
	AdminPassword     string `yaml:"admin_password"`
	AdminPasswordFile string `yaml:"admin_password_file"`
}

// Default returns a configuration suitable for local development.
func Default() Config {
	return Config{
		Server: Server{
			Listen:       ":8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			HSTS:         true,
		},
		Database: Database{Driver: "sqlite", DSN: "qtestframe.db", MaxOpenConns: 20},
		Logging: Logging{Level: "info", Format: "text", Syslog: Syslog{
			Network: "udp", Tag: "qtestframe", Facility: "local0", Format: "rfc5424",
		}},
		Auth: Auth{Local: true, SessionTTL: 12 * time.Hour, CookieSecure: true},
		Runner: Runner{
			Timeout:        60 * time.Second,
			WorkDir:        filepath.Join(os.TempDir(), "qtestframe-runner"),
			MaxOutputBytes: 1 << 20,
			Sandbox:        true,
			Interpreters: map[string]string{
				"sh": "/bin/sh", "bash": "/bin/bash",
				"python": "/usr/bin/python3", "javascript": "/usr/bin/node",
			},
			Web: WebRunner{Node: "/usr/bin/node", Browser: "chromium", Headless: true, Timeout: 60 * time.Second,
				JobTimeout: 10 * time.Minute, Screenshots: "on_failure", Trace: "on_failure", Video: "off", MaxParallel: 2},
		},
		Requirements: Requirements{AutoCreateTest: true},
		Files:        Files{MaxUploadMB: 25},
		Hardening:    Hardening{NoNewPrivs: true, Landlock: true, Seccomp: true, DropCapabilities: true},
		Bootstrap:    Bootstrap{TenantSlug: "default", TenantName: "Default"},
		Email:        Email{Port: 587, TLS: "starttls", Timeout: 30 * time.Second, DigestHour: 7, FromName: "Qtestframe"},
	}
}

// Overrides are command line settings; they take precedence over the
// configuration file and the environment.
type Overrides struct {
	// Listen replaces server.listen (host:port, e.g. "0.0.0.0:8443").
	Listen string
	// Port replaces only the port of server.listen (0 = unchanged).
	Port int
}

// Apply applies the overrides to c.
func (o Overrides) Apply(c *Config) error {
	if o.Listen != "" {
		c.Server.Listen = o.Listen
	}
	if o.Port != 0 {
		if o.Port < 1 || o.Port > 65535 {
			return fmt.Errorf("config: port %d out of range 1-65535", o.Port)
		}
		host, _, err := net.SplitHostPort(c.Server.Listen)
		if err != nil {
			host = ""
		}
		c.Server.Listen = net.JoinHostPort(host, strconv.Itoa(o.Port))
	}
	return nil
}

// Load reads the YAML file at path (if non-empty) over the defaults and
// applies environment overrides.
func Load(path string) (Config, error) {
	return LoadWith(path, Overrides{})
}

// LoadWith is Load followed by command line overrides.
// Precedence: command line > environment > YAML file > defaults.
func LoadWith(path string, o Overrides) (Config, error) {
	cfg := Default()
	if path == "" {
		path = os.Getenv("QTF_CONFIG")
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("config: %w", err)
		}
		if err := decodeYAML(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := o.Apply(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.resolveSecrets(); err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

// decodeYAML strictly decodes a single YAML document: unknown keys (e.g.
// typos such as "listn") are errors instead of being silently ignored.
func decodeYAML(b []byte, cfg *Config) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("only one YAML document is allowed")
	}
	return nil
}

func applyEnv(c *Config) {
	set := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	set("QTF_DATABASE_DRIVER", &c.Database.Driver)
	set("QTF_DATABASE_DSN", &c.Database.DSN)
	set("QTF_SERVER_LISTEN", &c.Server.Listen)
	if v, ok := os.LookupEnv("QTF_SERVER_PORT"); ok {
		if host, _, err := net.SplitHostPort(c.Server.Listen); err == nil {
			c.Server.Listen = net.JoinHostPort(host, v)
		}
	}
	set("QTF_SERVER_BASE_URL", &c.Server.BaseURL)
	set("QTF_LOG_LEVEL", &c.Logging.Level)
	set("QTF_LOG_FORMAT", &c.Logging.Format)
	set("QTF_BOOTSTRAP_ADMIN_EMAIL", &c.Bootstrap.AdminEmail)
	set("QTF_BOOTSTRAP_ADMIN_PASSWORD", &c.Bootstrap.AdminPassword)
	set("QTF_EMAIL_HOST", &c.Email.Host)
	set("QTF_EMAIL_USERNAME", &c.Email.Username)
	set("QTF_EMAIL_FROM", &c.Email.From)
	if v, ok := os.LookupEnv("QTF_EMAIL_PORT"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Email.Port = n
		}
	}
	setBool := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v == "1" || strings.EqualFold(v, "true")
		}
	}
	setBool("QTF_RUNNER_ENABLED", &c.Runner.Enabled)
	setBool("QTF_RUNNER_WEB_ENABLED", &c.Runner.Web.Enabled)
	set("QTF_RUNNER_WORK_DIR", &c.Runner.WorkDir)
	set("QTF_RUNNER_WEB_NODE", &c.Runner.Web.Node)
	set("QTF_RUNNER_WEB_PLAYWRIGHT_DIR", &c.Runner.Web.PlaywrightDir)
	set("QTF_RUNNER_WEB_BROWSERS_PATH", &c.Runner.Web.BrowsersPath)
	if v, ok := os.LookupEnv("QTF_COOKIE_SECURE"); ok {
		c.Auth.CookieSecure = v == "1" || strings.EqualFold(v, "true")
	}
}

// readSecret returns the trimmed contents of file. A relative path is
// resolved against $CREDENTIALS_DIRECTORY (systemd LoadCredential=).
func readSecret(file string) (string, error) {
	if !filepath.IsAbs(file) {
		if dir := os.Getenv("CREDENTIALS_DIRECTORY"); dir != "" {
			file = filepath.Join(dir, file)
		}
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("config: secret: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *Config) resolveSecrets() error {
	var err error
	if c.Database.DSNFile != "" {
		if c.Database.DSN, err = readSecret(c.Database.DSNFile); err != nil {
			return err
		}
	}
	if c.Bootstrap.AdminPasswordFile != "" {
		if c.Bootstrap.AdminPassword, err = readSecret(c.Bootstrap.AdminPasswordFile); err != nil {
			return err
		}
	}
	if c.Email.PasswordFile != "" {
		if c.Email.Password, err = readSecret(c.Email.PasswordFile); err != nil {
			return err
		}
	}
	if env := os.Getenv("QTF_EMAIL_PASSWORD"); env != "" {
		c.Email.Password = env
	}
	for i := range c.Auth.OIDC {
		o := &c.Auth.OIDC[i]
		if o.ClientSecretFile != "" {
			if o.ClientSecret, err = readSecret(o.ClientSecretFile); err != nil {
				return err
			}
		}
		if env := os.Getenv("QTF_OIDC_" + strings.ToUpper(o.Name) + "_CLIENT_SECRET"); env != "" {
			o.ClientSecret = env
		}
	}
	return nil
}

// Validate checks semantic constraints.
func (c *Config) Validate() error {
	var errs []error
	if _, port, err := net.SplitHostPort(c.Server.Listen); err != nil {
		errs = append(errs, fmt.Errorf("server.listen %q must be host:port (e.g. \":8443\")", c.Server.Listen))
	} else if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
		errs = append(errs, fmt.Errorf("server.listen %q has an invalid port", c.Server.Listen))
	}
	switch c.Database.Driver {
	case "sqlite", "postgres":
	default:
		errs = append(errs, fmt.Errorf("database.driver must be sqlite or postgres, got %q", c.Database.Driver))
	}
	if c.Database.DSN == "" {
		errs = append(errs, errors.New("database.dsn is required"))
	}
	if (c.Server.TLSCert == "") != (c.Server.TLSKey == "") {
		errs = append(errs, errors.New("server.tls_cert and server.tls_key must be set together"))
	}
	if c.Auth.SessionTTL <= 0 {
		errs = append(errs, errors.New("auth.session_ttl must be positive"))
	}
	names := map[string]bool{}
	for _, o := range c.Auth.OIDC {
		if o.Name == "" || o.Issuer == "" || o.ClientID == "" {
			errs = append(errs, fmt.Errorf("oidc provider %q: name, issuer and client_id are required", o.Name))
		}
		if names["oidc/"+o.Name] {
			errs = append(errs, fmt.Errorf("duplicate oidc provider %q", o.Name))
		}
		names["oidc/"+o.Name] = true
		switch o.LocalMFA {
		case "", LocalMFAOff, LocalMFAUnlessIdPMFA, LocalMFAAlways:
		default:
			errs = append(errs, fmt.Errorf("oidc provider %q: local_mfa must be off, unless_idp_mfa or always, got %q", o.Name, o.LocalMFA))
		}
	}
	for _, s := range c.Auth.SAML {
		if s.Name == "" || (s.IDPMetadataURL == "" && s.IDPMetadataFile == "") {
			errs = append(errs, fmt.Errorf("saml provider %q: name and idp metadata are required", s.Name))
		}
		if names["saml/"+s.Name] {
			errs = append(errs, fmt.Errorf("duplicate saml provider %q", s.Name))
		}
		names["saml/"+s.Name] = true
	}
	if c.Logging.Syslog.Enabled {
		switch c.Logging.Syslog.Network {
		case "udp", "tcp", "tls", "unix", "unixgram":
		default:
			errs = append(errs, fmt.Errorf("logging.syslog.network %q invalid", c.Logging.Syslog.Network))
		}
		if c.Logging.Syslog.Address == "" {
			errs = append(errs, errors.New("logging.syslog.address is required"))
		}
	}
	if c.Files.MaxUploadMB < 1 || c.Files.MaxUploadMB > 1024 {
		errs = append(errs, errors.New("files.max_upload_mb must be between 1 and 1024"))
	}
	if c.Runner.Enabled && c.Runner.Timeout <= 0 {
		errs = append(errs, errors.New("runner.timeout must be positive"))
	}
	errs = append(errs, c.Runner.Web.validate()...)
	errs = append(errs, c.Email.validate()...)
	if c.Email.On() && c.Server.BaseURL == "" {
		errs = append(errs, errors.New("server.base_url is required for e-mail (links in the messages)"))
	}
	return errors.Join(errs...)
}

func (w WebRunner) validate() []error {
	var errs []error
	if w.MaxParallel < 1 || w.MaxParallel > 32 {
		errs = append(errs, errors.New("runner.web.max_parallel must be between 1 and 32"))
	}
	if !w.Enabled {
		return errs
	}
	switch w.Browser {
	case "chromium", "firefox", "webkit":
	default:
		errs = append(errs, fmt.Errorf("runner.web.browser must be chromium, firefox or webkit, got %q", w.Browser))
	}
	for _, o := range []struct{ key, v string }{{"screenshots", w.Screenshots}, {"trace", w.Trace}} {
		if o.v != "on_failure" && o.v != "always" && o.v != "off" {
			errs = append(errs, fmt.Errorf("runner.web.%s must be on_failure, always or off, got %q", o.key, o.v))
		}
	}
	if w.Video != "off" && w.Video != "on_failure" && w.Video != "always" {
		errs = append(errs, fmt.Errorf("runner.web.video must be off, on_failure or always, got %q", w.Video))
	}
	if w.Timeout <= 0 || w.JobTimeout <= 0 {
		errs = append(errs, errors.New("runner.web.timeout and runner.web.job_timeout must be positive"))
	}
	if w.Command == "" && (w.PlaywrightDir == "" || w.Node == "") {
		errs = append(errs, errors.New("runner.web needs playwright_dir (the directory with node_modules/@playwright/test) and node, or a command"))
	}
	if w.MaxEvidenceMB < 0 {
		errs = append(errs, errors.New("runner.web.max_evidence_mb must not be negative"))
	}
	return errs
}

func (e Email) validate() []error {
	if !e.On() {
		return nil
	}
	var errs []error
	if e.Host == "" {
		return nil
	}
	if e.Port < 1 || e.Port > 65535 {
		errs = append(errs, fmt.Errorf("email.port %d out of range 1-65535", e.Port))
	}
	switch e.TLS {
	case "starttls", "tls", "none":
	default:
		errs = append(errs, fmt.Errorf("email.tls must be starttls, tls or none, got %q", e.TLS))
	}
	if a, err := mail.ParseAddress(e.From); err != nil || a.Name != "" || strings.ContainsAny(e.From, "\r\n") {
		errs = append(errs, fmt.Errorf("email.from %q must be a plain e-mail address (put the name in email.from_name)", e.From))
	}
	if strings.ContainsAny(e.FromName, "\r\n") {
		errs = append(errs, errors.New("email.from_name must be one line"))
	}
	if e.Password != "" && e.Username == "" {
		errs = append(errs, errors.New("email.username is required with a password"))
	}
	if e.Timeout <= 0 {
		errs = append(errs, errors.New("email.timeout must be positive"))
	}
	if e.DigestHour < 0 || e.DigestHour > 23 {
		errs = append(errs, errors.New("email.digest_hour must be between 0 and 23"))
	}
	return errs
}
