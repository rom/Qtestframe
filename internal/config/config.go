// Package config loads the server configuration from YAML and environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the complete server configuration.
type Config struct {
	Server    Server    `yaml:"server"`
	Database  Database  `yaml:"database"`
	Logging   Logging   `yaml:"logging"`
	Auth      Auth      `yaml:"auth"`
	Runner    Runner    `yaml:"runner"`
	Hardening Hardening `yaml:"hardening"`
	Bootstrap Bootstrap `yaml:"bootstrap"`
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
	Provisioning     `yaml:",inline"`
}

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

// Runner configures server side execution of automated test scripts.
type Runner struct {
	Enabled        bool              `yaml:"enabled"`
	Timeout        time.Duration     `yaml:"timeout"`
	WorkDir        string            `yaml:"work_dir"`
	MaxOutputBytes int               `yaml:"max_output_bytes"`
	Interpreters   map[string]string `yaml:"interpreters"`
	Sandbox        bool              `yaml:"sandbox"`
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
		},
		Hardening: Hardening{NoNewPrivs: true, Landlock: true, Seccomp: true, DropCapabilities: true},
		Bootstrap: Bootstrap{TenantSlug: "default", TenantName: "Default"},
	}
}

// Load reads path (if non-empty) over the defaults and applies environment
// overrides.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = os.Getenv("QTF_CONFIG")
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("config: %w", err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.resolveSecrets(); err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
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
	set("QTF_SERVER_BASE_URL", &c.Server.BaseURL)
	set("QTF_LOG_LEVEL", &c.Logging.Level)
	set("QTF_LOG_FORMAT", &c.Logging.Format)
	set("QTF_BOOTSTRAP_ADMIN_EMAIL", &c.Bootstrap.AdminEmail)
	set("QTF_BOOTSTRAP_ADMIN_PASSWORD", &c.Bootstrap.AdminPassword)
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
	if c.Runner.Enabled && c.Runner.Timeout <= 0 {
		errs = append(errs, errors.New("runner.timeout must be positive"))
	}
	return errors.Join(errs...)
}
