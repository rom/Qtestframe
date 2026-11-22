// Package config loads the server configuration from YAML and environment.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
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
	// SystemSettings controls the settings super administrators can change
	// at runtime (Administration → System settings, stored in the database).
	SystemSettings SystemSettings `yaml:"system_settings"`
	// Secrets configures the encryption of the secrets stored in the
	// database (see internal/secrets and docs/security.md).
	Secrets Secrets `yaml:"secrets"`
	// Similarity configures the detection of similar requirements and
	// tests (suspected duplicates, see internal/similarity).
	Similarity Similarity `yaml:"similarity"`
	// AI configures the optional AI assistance (off by default, see
	// internal/ai): drafts and suggestions from a language model service.
	AI AI `yaml:"ai"`

	// Meta records where values came from (file keys, environment
	// variables); filled by Load, not part of the YAML.
	Meta Meta `yaml:"-"`
}

// SystemSettings controls runtime-editable system settings.
type SystemSettings struct {
	// Locked lists setting keys (e.g. "logging.level"), key prefixes ending
	// in ".*" (e.g. "logging.*") or "*" that cannot be changed in the
	// administration view; their database overrides are ignored. Settings
	// set by a QTF_* environment variable are always locked.
	Locked []string `yaml:"locked"`
}

// Secret key providers (secrets.provider).
const (
	SecretsLocal  = "local"
	SecretsTPM    = "tpm"
	SecretsPKCS11 = "pkcs11"
)

// DefaultKeyFileName is the name of the local key file when
// secrets.local.key_file is not set.
const DefaultKeyFileName = "qtestframe-secrets.key"

// Secrets configures how the data-encryption key of stored secrets (TOTP
// secrets, the SMTP password, OIDC client secrets, webhook secrets, tracker
// tokens, secret web test variables) is protected.
type Secrets struct {
	// Provider is local (default), tpm or pkcs11.
	Provider string    `yaml:"provider"`
	Local    LocalKey  `yaml:"local"`
	TPM      TPMKey    `yaml:"tpm"`
	PKCS11   PKCS11Key `yaml:"pkcs11"`
}

// LocalKey is the key-encryption key of the local provider: a 256-bit key
// in a file (created with mode 0600 when missing) or, when set, in the
// environment variable QTF_SECRETS_KEY (base64 or hex; never in the file).
type LocalKey struct {
	// KeyFile defaults to qtestframe-secrets.key next to a SQLite
	// database, else in $STATE_DIRECTORY, /var/lib/qtestframe or the
	// working directory (see KeyFilePath).
	KeyFile string `yaml:"key_file"`
	// Key is the key from QTF_SECRETS_KEY.
	Key string `yaml:"-"`
}

// TPMKey seals the data-encryption key to a TPM 2.0.
type TPMKey struct {
	// Device is the TPM resource manager (default /dev/tpmrm0) or the Unix
	// socket of a TPM emulator (swtpm --server type=unixio).
	Device string `yaml:"device"`
	// PCRs, when set, bind the sealed key to the current values of these
	// PCRs (e.g. [7] for the Secure Boot state): unsealing fails after
	// they change.
	PCRs []int `yaml:"pcrs"`
	// PCRBank is the PCR bank of the policy: sha256 (default) or sha1.
	PCRBank string `yaml:"pcr_bank"`
}

// PKCS11Key wraps the data-encryption key with an AES key held in an HSM
// (PKCS#11). Needs a binary built with -tags pkcs11 (make go-pkcs11).
type PKCS11Key struct {
	// Module is the PKCS#11 library, e.g. /usr/lib/softhsm/libsofthsm2.so.
	Module string `yaml:"module"`
	// TokenLabel selects the token; Slot (when set) selects the slot
	// instead.
	TokenLabel string `yaml:"token_label"`
	Slot       *uint  `yaml:"slot"`
	// PINFile holds the user PIN (or QTF_SECRETS_PKCS11_PIN); the PIN is
	// never written in the configuration file.
	PINFile string `yaml:"pin_file"`
	PIN     string `yaml:"-"`
	// KeyLabel is the CKA_LABEL of the AES key (default qtestframe-kek).
	KeyLabel string `yaml:"key_label"`
	// GenerateKey creates a non-extractable AES-256 key with KeyLabel in
	// the token when there is none.
	GenerateKey bool `yaml:"generate_key"`
}

// KeyFilePath returns the key file of the local provider: key_file, else
// qtestframe-secrets.key next to a SQLite database file, else in
// $STATE_DIRECTORY (systemd), /var/lib/qtestframe (when it exists) or the
// working directory. It returns "" for an in-memory SQLite database
// without key_file (an ephemeral key is used, as the data is ephemeral).
func (c *Config) KeyFilePath() string {
	if f := c.Secrets.Local.KeyFile; f != "" {
		return f
	}
	if c.Database.Driver == "sqlite" {
		path := strings.TrimPrefix(strings.SplitN(c.Database.DSN, "?", 2)[0], "file:")
		if path == "" || strings.HasPrefix(path, ":memory:") || strings.Contains(c.Database.DSN, "mode=memory") {
			return ""
		}
		return filepath.Join(filepath.Dir(path), DefaultKeyFileName)
	}
	if dir := strings.Split(os.Getenv("STATE_DIRECTORY"), ":")[0]; dir != "" {
		return filepath.Join(dir, DefaultKeyFileName)
	}
	if fi, err := os.Stat("/var/lib/qtestframe"); err == nil && fi.IsDir() {
		return filepath.Join("/var/lib/qtestframe", DefaultKeyFileName)
	}
	return DefaultKeyFileName
}

func (s Secrets) validate() []error {
	var errs []error
	switch s.Provider {
	case "", SecretsLocal, SecretsTPM:
	case SecretsPKCS11:
		if s.PKCS11.Module == "" {
			errs = append(errs, errors.New("secrets.pkcs11.module is required"))
		}
		if s.PKCS11.TokenLabel == "" && s.PKCS11.Slot == nil {
			errs = append(errs, errors.New("secrets.pkcs11: token_label or slot is required"))
		}
		if s.PKCS11.KeyLabel == "" {
			errs = append(errs, errors.New("secrets.pkcs11.key_label is required"))
		}
	default:
		errs = append(errs, fmt.Errorf("secrets.provider must be local, tpm or pkcs11, got %q", s.Provider))
	}
	switch s.TPM.PCRBank {
	case "", "sha1", "sha256":
	default:
		errs = append(errs, fmt.Errorf("secrets.tpm.pcr_bank must be sha1 or sha256, got %q", s.TPM.PCRBank))
	}
	for _, p := range s.TPM.PCRs {
		if p < 0 || p > 23 {
			errs = append(errs, fmt.Errorf("secrets.tpm.pcrs: %d is not a PCR (0-23)", p))
		}
	}
	return errs
}

// Meta records the origin of loaded values.
type Meta struct {
	// File is the configuration file read ("" = none).
	File string
	// FileKeys holds the dotted keys present in the file (e.g.
	// "logging.syslog.address"; lists are recorded by their key, e.g.
	// "auth.oidc"), plus secrets read from a *_file option.
	FileKeys map[string]bool
	// EnvKeys maps dotted keys to the environment variable that set them.
	EnvKeys map[string]string
}

// IsLocked reports whether key matches one of the locked patterns.
func (s SystemSettings) IsLocked(key string) bool {
	for _, p := range s.Locked {
		p = strings.TrimSpace(p)
		switch {
		case p == "*" || p == key:
			return true
		case strings.HasSuffix(p, ".*") && strings.HasPrefix(key, strings.TrimSuffix(p, "*")):
			return true
		}
	}
	return false
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

// Similarity configures the similarity checks. Without embeddings the
// similarity is computed locally (TF-IDF); nothing leaves the server.
type Similarity struct {
	Embeddings Embeddings `yaml:"embeddings"`
}

// Embeddings is an optional OpenAI-compatible embeddings endpoint
// (POST {model, input} → {data: [{embedding}]}). When enabled, the text of
// requirements and tests is sent to it to compute their similarity. The
// key is never written in the file: it comes from api_key_file or the
// system settings.
type Embeddings struct {
	Enabled    bool          `yaml:"enabled"`
	URL        string        `yaml:"url"`
	Model      string        `yaml:"model"`
	APIKeyFile string        `yaml:"api_key_file"`
	APIKey     string        `yaml:"-"`
	Timeout    time.Duration `yaml:"timeout"`
}

// On reports whether the embeddings endpoint is to be used.
func (e Embeddings) On() bool { return e.Enabled && e.URL != "" }

func (e Embeddings) validate() []error {
	var errs []error
	if e.URL != "" {
		u, err := url.Parse(e.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, errors.New("similarity.embeddings.url must be an absolute http(s) URL"))
		}
	}
	if e.Enabled && e.URL == "" {
		errs = append(errs, errors.New("similarity.embeddings.url is required when embeddings are enabled"))
	}
	if e.Timeout < 0 {
		errs = append(errs, errors.New("similarity.embeddings.timeout must not be negative"))
	}
	return errs
}

// AI providers (ai.provider).
const (
	AIAnthropic = "anthropic"
	AIOpenAI    = "openai"
)

// DefaultAIModel is the model asked for when ai.model is not set (a
// configurable default; any model id of the provider can be set).
const DefaultAIModel = "claude-sonnet-5-5"

// AI is the optional AI assistance: a language model service the
// administrator configures (Anthropic Messages API, or an OpenAI-compatible
// chat completions endpoint such as Azure OpenAI, Ollama or vLLM). When
// enabled, the text of the items a user asks about is sent to it. The key
// is never written in the file: it comes from api_key_file or the system
// settings.
type AI struct {
	Enabled bool `yaml:"enabled"`
	// Provider is "anthropic" (default) or "openai".
	Provider string `yaml:"provider"`
	// URL is the base URL of the service ("" with anthropic: the public
	// API), e.g. https://llm.example.com/v1 for an OpenAI-compatible one.
	URL        string        `yaml:"url"`
	Model      string        `yaml:"model"`
	APIKeyFile string        `yaml:"api_key_file"`
	APIKey     string        `yaml:"-"`
	Timeout    time.Duration `yaml:"timeout"`
	MaxTokens  int           `yaml:"max_tokens"`
	// Temperature is "" (the service's default) or a number 0–2.
	Temperature string `yaml:"temperature"`
	// RateLimit is the most AI requests of one user per hour.
	RateLimit int `yaml:"rate_limit"`
	// Features turns the single features on or off.
	Features AIFeatures `yaml:"features"`
	// PriceInput and PriceOutput are optional prices per million input
	// and output tokens ("" none), used to show an estimated cost.
	PriceInput  string `yaml:"price_input"`
	PriceOutput string `yaml:"price_output"`
}

// AIFeatures are the features of the AI assistance.
type AIFeatures struct {
	DraftTests         bool `yaml:"draft_tests"`
	ImproveRequirement bool `yaml:"improve_requirement"`
	SummarizeRun       bool `yaml:"summarize_run"`
	SuggestMappings    bool `yaml:"suggest_mappings"`
	SuggestCriteria    bool `yaml:"suggest_criteria"`
}

// On reports whether the AI service is to be used.
func (a AI) On() bool {
	return a.Enabled && (a.URL != "" || a.ProviderName() == AIAnthropic) && a.ModelName() != ""
}

// ProviderName returns the provider with its default.
func (a AI) ProviderName() string {
	if a.Provider == "" {
		return AIAnthropic
	}
	return a.Provider
}

// ModelName returns the model with its default (the default only for the
// Anthropic provider; OpenAI-compatible services name their own).
func (a AI) ModelName() string {
	if a.Model == "" && a.ProviderName() == AIAnthropic {
		return DefaultAIModel
	}
	return a.Model
}

// ParseTemperature returns the temperature (nil: not set).
func ParseTemperature(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 2 {
		return nil, errors.New("must be empty or a number between 0 and 2")
	}
	return &f, nil
}

// ParsePrice returns a price per million tokens (0: not set).
func ParsePrice(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 1e6 {
		return 0, errors.New("must be empty or a non-negative number")
	}
	return f, nil
}

func (a AI) validate() []error {
	var errs []error
	switch a.Provider {
	case "", AIAnthropic, AIOpenAI:
	default:
		errs = append(errs, errors.New("ai.provider must be anthropic or openai"))
	}
	if a.URL != "" {
		u, err := url.Parse(a.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, errors.New("ai.url must be an absolute http(s) URL"))
		}
	}
	if a.Enabled && a.URL == "" && a.ProviderName() == AIOpenAI {
		errs = append(errs, errors.New("ai.url is required for an OpenAI-compatible service"))
	}
	if a.Timeout < 0 {
		errs = append(errs, errors.New("ai.timeout must not be negative"))
	}
	if a.MaxTokens < 0 || a.MaxTokens > 128000 {
		errs = append(errs, errors.New("ai.max_tokens must be between 0 and 128000"))
	}
	if a.RateLimit < 0 {
		errs = append(errs, errors.New("ai.rate_limit must not be negative"))
	}
	if _, err := ParseTemperature(a.Temperature); err != nil {
		errs = append(errs, fmt.Errorf("ai.temperature %w", err))
	}
	if _, err := ParsePrice(a.PriceInput); err != nil {
		errs = append(errs, fmt.Errorf("ai.price_input %w", err))
	}
	if _, err := ParsePrice(a.PriceOutput); err != nil {
		errs = append(errs, fmt.Errorf("ai.price_output %w", err))
	}
	return errs
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
	// FrameAncestors are the origins (https://doors.example.com) that may
	// show the OSLC delegated dialogs in a frame; without any they open
	// in a window of their own.
	FrameAncestors []string `yaml:"frame_ancestors"`
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
	// PasswordMinLength is the minimum length of local passwords (8-72).
	PasswordMinLength int `yaml:"password_min_length"`
	// LoginMaxAttempts sign-in attempts per client address and e-mail
	// address within LoginLockout; further attempts are refused until the
	// window has passed.
	LoginMaxAttempts int           `yaml:"login_max_attempts"`
	LoginLockout     time.Duration `yaml:"login_lockout"`
	OIDC             []OIDC        `yaml:"oidc"`
	SAML             []SAML        `yaml:"saml"`
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
	Name string `yaml:"name"`
	// Enabled is false to keep a provider configured but hide it from the
	// sign-in page (nil = enabled).
	Enabled          *bool    `yaml:"enabled"`
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

// On reports whether the provider is enabled.
func (o OIDC) On() bool { return o.Enabled == nil || *o.Enabled }

// OIDCSecretEnv is the environment variable holding the client secret of
// the OIDC provider name.
func OIDCSecretEnv(name string) string {
	return "QTF_OIDC_" + strings.ToUpper(name) + "_CLIENT_SECRET"
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
		Auth: Auth{Local: true, SessionTTL: 12 * time.Hour, CookieSecure: true,
			PasswordMinLength: 10, LoginMaxAttempts: 10, LoginLockout: 5 * time.Minute},
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
		Similarity:   Similarity{Embeddings: Embeddings{Timeout: 30 * time.Second}},
		AI: AI{Provider: AIAnthropic, Model: DefaultAIModel, Timeout: 2 * time.Minute, MaxTokens: 8192, RateLimit: 30,
			Features: AIFeatures{DraftTests: true, ImproveRequirement: true, SummarizeRun: true, SuggestMappings: true, SuggestCriteria: true}},
		Secrets: Secrets{Provider: SecretsLocal, TPM: TPMKey{Device: "/dev/tpmrm0", PCRBank: "sha256"},
			PKCS11: PKCS11Key{KeyLabel: "qtestframe-kek"}},
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
		cfg.Meta.File = path
		cfg.Meta.FileKeys = fileKeys(b)
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

// fileKeys returns the dotted keys of the mappings in a YAML document
// (sequences are recorded by their own key, not descended into).
func fileKeys(b []byte) map[string]bool {
	out := map[string]bool{}
	var root yaml.Node
	if yaml.Unmarshal(b, &root) != nil || len(root.Content) == 0 {
		return out
	}
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		if n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			if prefix != "" {
				k = prefix + "." + k
			}
			out[k] = true
			walk(n.Content[i+1], k)
		}
	}
	walk(root.Content[0], "")
	return out
}

// EnvVars maps the environment variables read by Load to the dotted key
// they set (QTF_OIDC_<NAME>_CLIENT_SECRET is per provider, see
// OIDCSecretEnv).
var EnvVars = map[string]string{
	"QTF_DATABASE_DRIVER":           "database.driver",
	"QTF_DATABASE_DSN":              "database.dsn",
	"QTF_SERVER_LISTEN":             "server.listen",
	"QTF_SERVER_PORT":               "server.listen",
	"QTF_SERVER_BASE_URL":           "server.base_url",
	"QTF_LOG_LEVEL":                 "logging.level",
	"QTF_LOG_FORMAT":                "logging.format",
	"QTF_BOOTSTRAP_ADMIN_EMAIL":     "bootstrap.admin_email",
	"QTF_BOOTSTRAP_ADMIN_PASSWORD":  "bootstrap.admin_password",
	"QTF_EMAIL_HOST":                "email.host",
	"QTF_EMAIL_USERNAME":            "email.username",
	"QTF_EMAIL_FROM":                "email.from",
	"QTF_EMAIL_PORT":                "email.port",
	"QTF_EMAIL_PASSWORD":            "email.password",
	"QTF_RUNNER_ENABLED":            "runner.enabled",
	"QTF_RUNNER_WEB_ENABLED":        "runner.web.enabled",
	"QTF_RUNNER_WORK_DIR":           "runner.work_dir",
	"QTF_RUNNER_WEB_NODE":           "runner.web.node",
	"QTF_RUNNER_WEB_PLAYWRIGHT_DIR": "runner.web.playwright_dir",
	"QTF_RUNNER_WEB_BROWSERS_PATH":  "runner.web.browsers_path",
	"QTF_COOKIE_SECURE":             "auth.cookie_secure",
	"QTF_SECRETS_PROVIDER":          "secrets.provider",
	"QTF_SECRETS_KEY_FILE":          "secrets.local.key_file",
	"QTF_SECRETS_KEY":               "secrets.local.key",
	"QTF_SECRETS_TPM_DEVICE":        "secrets.tpm.device",
	"QTF_SECRETS_PKCS11_MODULE":     "secrets.pkcs11.module",
	"QTF_SECRETS_PKCS11_PIN":        "secrets.pkcs11.pin",
}

func (c *Config) initMeta() {
	if c.Meta.EnvKeys == nil {
		c.Meta.EnvKeys = map[string]string{}
	}
	if c.Meta.FileKeys == nil {
		c.Meta.FileKeys = map[string]bool{}
	}
}

func applyEnv(c *Config) {
	c.initMeta()
	lookup := func(env string) (string, bool) {
		v, ok := os.LookupEnv(env)
		if ok {
			c.Meta.EnvKeys[EnvVars[env]] = env
		}
		return v, ok
	}
	set := func(env string, dst *string) {
		if v, ok := lookup(env); ok {
			*dst = v
		}
	}
	set("QTF_DATABASE_DRIVER", &c.Database.Driver)
	set("QTF_DATABASE_DSN", &c.Database.DSN)
	set("QTF_SERVER_LISTEN", &c.Server.Listen)
	if v, ok := lookup("QTF_SERVER_PORT"); ok {
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
	if v, ok := lookup("QTF_EMAIL_PORT"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Email.Port = n
		}
	}
	setBool := func(env string, dst *bool) {
		if v, ok := lookup(env); ok {
			*dst = v == "1" || strings.EqualFold(v, "true")
		}
	}
	setBool("QTF_RUNNER_ENABLED", &c.Runner.Enabled)
	setBool("QTF_RUNNER_WEB_ENABLED", &c.Runner.Web.Enabled)
	set("QTF_RUNNER_WORK_DIR", &c.Runner.WorkDir)
	set("QTF_RUNNER_WEB_NODE", &c.Runner.Web.Node)
	set("QTF_RUNNER_WEB_PLAYWRIGHT_DIR", &c.Runner.Web.PlaywrightDir)
	set("QTF_RUNNER_WEB_BROWSERS_PATH", &c.Runner.Web.BrowsersPath)
	setBool("QTF_COOKIE_SECURE", &c.Auth.CookieSecure)
	set("QTF_SECRETS_PROVIDER", &c.Secrets.Provider)
	set("QTF_SECRETS_KEY_FILE", &c.Secrets.Local.KeyFile)
	set("QTF_SECRETS_TPM_DEVICE", &c.Secrets.TPM.Device)
	set("QTF_SECRETS_PKCS11_MODULE", &c.Secrets.PKCS11.Module)
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
	c.initMeta()
	if c.Email.PasswordFile != "" {
		if c.Email.Password, err = readSecret(c.Email.PasswordFile); err != nil {
			return err
		}
		c.Meta.FileKeys["email.password"] = true
	}
	if c.Similarity.Embeddings.APIKeyFile != "" {
		if c.Similarity.Embeddings.APIKey, err = readSecret(c.Similarity.Embeddings.APIKeyFile); err != nil {
			return err
		}
		c.Meta.FileKeys["similarity.embeddings.api_key"] = true
	}
	if c.AI.APIKeyFile != "" {
		if c.AI.APIKey, err = readSecret(c.AI.APIKeyFile); err != nil {
			return err
		}
		c.Meta.FileKeys["ai.api_key"] = true
	}
	if env := os.Getenv("QTF_EMAIL_PASSWORD"); env != "" {
		c.Email.Password = env
		c.Meta.EnvKeys["email.password"] = "QTF_EMAIL_PASSWORD"
	}
	if c.Secrets.PKCS11.PINFile != "" {
		if c.Secrets.PKCS11.PIN, err = readSecret(c.Secrets.PKCS11.PINFile); err != nil {
			return err
		}
	}
	if env := os.Getenv("QTF_SECRETS_PKCS11_PIN"); env != "" {
		c.Secrets.PKCS11.PIN = env
		c.Meta.EnvKeys["secrets.pkcs11.pin"] = "QTF_SECRETS_PKCS11_PIN"
	}
	if env := os.Getenv("QTF_SECRETS_KEY"); env != "" {
		c.Secrets.Local.Key = env
		c.Meta.EnvKeys["secrets.local.key"] = "QTF_SECRETS_KEY"
	}
	for i := range c.Auth.OIDC {
		o := &c.Auth.OIDC[i]
		if o.ClientSecretFile != "" {
			if o.ClientSecret, err = readSecret(o.ClientSecretFile); err != nil {
				return err
			}
		}
		if env := os.Getenv(OIDCSecretEnv(o.Name)); env != "" {
			o.ClientSecret = env
			c.Meta.EnvKeys["auth.oidc."+o.Name+".client_secret"] = OIDCSecretEnv(o.Name)
		}
	}
	return nil
}

// ParseLevel parses a log level name (debug, info, warn, error).
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	err := l.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(s))))
	return l, err
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
	for _, o := range c.Server.FrameAncestors {
		if !ValidOrigin(o) {
			errs = append(errs, fmt.Errorf("server.frame_ancestors: %q is not an origin such as https://doors.example.com", o))
		}
	}
	if c.Auth.SessionTTL <= 0 {
		errs = append(errs, errors.New("auth.session_ttl must be positive"))
	}
	if c.Auth.PasswordMinLength < 8 || c.Auth.PasswordMinLength > 72 {
		errs = append(errs, errors.New("auth.password_min_length must be between 8 and 72"))
	}
	if c.Auth.LoginMaxAttempts < 1 || c.Auth.LoginMaxAttempts > 1000 {
		errs = append(errs, errors.New("auth.login_max_attempts must be between 1 and 1000"))
	}
	if c.Auth.LoginLockout < time.Minute || c.Auth.LoginLockout > 24*time.Hour {
		errs = append(errs, errors.New("auth.login_lockout must be between 1m and 24h"))
	}
	if _, err := ParseLevel(c.Logging.Level); err != nil {
		errs = append(errs, fmt.Errorf("logging.level %q must be debug, info, warn or error", c.Logging.Level))
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
	errs = append(errs, c.Secrets.validate()...)
	errs = append(errs, c.Similarity.Embeddings.validate()...)
	errs = append(errs, c.AI.validate()...)
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

// ValidOrigin reports whether s is an origin: http(s)://host[:port]
// without a path, query or user.
func ValidOrigin(s string) bool {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return false
	}
	return (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == "" && !strings.ContainsAny(s, " ;,'\"")
}
