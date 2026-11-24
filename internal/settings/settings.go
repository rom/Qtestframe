// Package settings defines the system settings super administrators can
// change at runtime (Administration → System settings) and computes the
// effective configuration from the configuration file, the environment and
// the overrides stored in the database (table system_settings).
//
// Precedence: database override > environment / configuration file >
// built-in default. A setting is locked — its override is ignored and the
// administration view shows it read-only — when a QTF_* environment
// variable sets it ("the environment wins"), when the operator lists it in
// system_settings.locked, or when it can only be set in the file.
//
// Settings apply either at once ("runtime") or at the next start
// ("restart": the runner and its sandbox are set up before the process is
// locked down, so their stored values are read at start-up).
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

// Kind is the value type of a setting.
type Kind string

// Value kinds.
const (
	String   Kind = "string"
	Int      Kind = "int"
	Bool     Kind = "bool"
	Duration Kind = "duration"
	Enum     Kind = "enum"
	Secret   Kind = "secret"
	List     Kind = "list"
	OIDCList Kind = "oidc"
)

// Apply says when a changed value takes effect.
type Apply string

// Apply modes.
const (
	Runtime  Apply = "runtime"
	Restart  Apply = "restart"
	ReadOnly Apply = "readonly"
)

// Sources of an effective value.
const (
	SourceDefault     = "default"
	SourceFile        = "file"
	SourceEnvironment = "environment"
	SourceDatabase    = "database"
)

// Reasons a setting is locked.
const (
	LockEnvironment = "environment"
	LockConfig      = "config"
	LockReadOnly    = "readonly"
)

// Sections of the administration view, in order.
var Sections = []string{"general", "server", "security", "tokens", "oidc", "logging", "email", "outbound", "runner", "live", "similarity", "ai", "retention"}

// Def describes one setting.
type Def struct {
	Key     string   `json:"key"`
	Section string   `json:"section"`
	Kind    Kind     `json:"type"`
	Apply   Apply    `json:"apply"`
	Options []string `json:"options,omitempty"`
	Min     int64    `json:"min,omitempty"`
	Max     int64    `json:"max,omitempty"`
	// Durations: bounds.
	MinDur time.Duration `json:"-"`
	MaxDur time.Duration `json:"-"`
	// Optional strings may be empty.
	Optional bool `json:"-"`
	// Check validates a decoded value further (nil = none).
	Check func(any) error `json:"-"`
	// ItemCheck validates each entry of a list (nil = none).
	ItemCheck func(string) error `json:"-"`

	get func(*config.Config) any
	set func(*config.Config, any)
}

func str(key, section string, a Apply, p func(*config.Config) *string) *Def {
	return &Def{Key: key, Section: section, Kind: String, Apply: a,
		get: func(c *config.Config) any { return *p(c) }, set: func(c *config.Config, v any) { *p(c) = v.(string) }}
}

func num(key, section string, a Apply, min, max int64, p func(*config.Config) *int) *Def {
	return &Def{Key: key, Section: section, Kind: Int, Apply: a, Min: min, Max: max,
		get: func(c *config.Config) any { return int64(*p(c)) }, set: func(c *config.Config, v any) { *p(c) = int(v.(int64)) }}
}

func flag(key, section string, a Apply, p func(*config.Config) *bool) *Def {
	return &Def{Key: key, Section: section, Kind: Bool, Apply: a,
		get: func(c *config.Config) any { return *p(c) }, set: func(c *config.Config, v any) { *p(c) = v.(bool) }}
}

func dur(key, section string, a Apply, min, max time.Duration, p func(*config.Config) *time.Duration) *Def {
	return &Def{Key: key, Section: section, Kind: Duration, Apply: a, MinDur: min, MaxDur: max,
		get: func(c *config.Config) any { return *p(c) }, set: func(c *config.Config, v any) { *p(c) = v.(time.Duration) }}
}

func enum(key, section string, a Apply, opts []string, p func(*config.Config) *string) *Def {
	d := str(key, section, a, p)
	d.Kind, d.Options = Enum, opts
	return d
}

func optional(d *Def) *Def {
	d.Optional = true
	return d
}

func checked(d *Def, f func(any) error) *Def {
	d.Check = f
	return d
}

// Facilities are the syslog facilities (internal/logging).
var Facilities = []string{"kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news", "uucp", "cron", "authpriv", "ftp",
	"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7"}

func checkBaseURL(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.HasSuffix(s, "/") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be an absolute http(s) URL without a trailing slash, e.g. https://qtf.example.com")
	}
	return nil
}

func oneLine(v any) error {
	if strings.ContainsAny(v.(string), "\r\n") {
		return errors.New("must be one line")
	}
	return nil
}

// Registry lists all settings shown in the administration view.
var Registry = []*Def{
	// general
	checked(optional(str("server.base_url", "general", Runtime, func(c *config.Config) *string { return &c.Server.BaseURL })), checkBaseURL),
	num("files.max_upload_mb", "general", Runtime, 1, 1024, func(c *config.Config) *int { return &c.Files.MaxUploadMB }),
	flag("requirements.auto_create_test", "general", Runtime, func(c *config.Config) *bool { return &c.Requirements.AutoCreateTest }),

	// server: the listener, the database and the start-up options (set in
	// the file; the HTTP timeouts are read at start-up)
	str("server.listen", "server", ReadOnly, func(c *config.Config) *string { return &c.Server.Listen }),
	str("server.tls_cert", "server", ReadOnly, func(c *config.Config) *string { return &c.Server.TLSCert }),
	str("server.tls_key", "server", ReadOnly, func(c *config.Config) *string { return &c.Server.TLSKey }),
	dur("server.read_timeout", "server", Restart, time.Second, 2*time.Hour, func(c *config.Config) *time.Duration { return &c.Server.ReadTimeout }),
	dur("server.write_timeout", "server", Restart, time.Second, 2*time.Hour, func(c *config.Config) *time.Duration { return &c.Server.WriteTimeout }),
	itemChecked(list("server.frame_ancestors", "server", Runtime, func(c *config.Config) *[]string { return &c.Server.FrameAncestors }), checkOrigin),
	roList("server.trusted_proxies", "server", func(c *config.Config) []string { return c.Server.TrustedProxies }),
	flag("server.hsts", "server", ReadOnly, func(c *config.Config) *bool { return &c.Server.HSTS }),
	str("database.driver", "server", ReadOnly, func(c *config.Config) *string { return &c.Database.Driver }),
	{Key: "database.dsn", Section: "server", Kind: Secret, Apply: ReadOnly,
		get: func(c *config.Config) any { return c.Database.DSN }},
	num("database.max_open_conns", "server", ReadOnly, 0, 0, func(c *config.Config) *int { return &c.Database.MaxOpenConns }),
	str("bootstrap.tenant_slug", "server", ReadOnly, func(c *config.Config) *string { return &c.Bootstrap.TenantSlug }),
	str("bootstrap.tenant_name", "server", ReadOnly, func(c *config.Config) *string { return &c.Bootstrap.TenantName }),
	str("bootstrap.admin_email", "server", ReadOnly, func(c *config.Config) *string { return &c.Bootstrap.AdminEmail }),
	flag("hardening.no_new_privs", "server", ReadOnly, func(c *config.Config) *bool { return &c.Hardening.NoNewPrivs }),
	flag("hardening.landlock", "server", ReadOnly, func(c *config.Config) *bool { return &c.Hardening.Landlock }),
	flag("hardening.seccomp", "server", ReadOnly, func(c *config.Config) *bool { return &c.Hardening.Seccomp }),
	flag("hardening.drop_capabilities", "server", ReadOnly, func(c *config.Config) *bool { return &c.Hardening.DropCapabilities }),
	roList("hardening.read_only_paths", "server", func(c *config.Config) []string { return c.Hardening.ReadOnlyPaths }),
	roList("hardening.read_write_paths", "server", func(c *config.Config) []string { return c.Hardening.ReadWritePaths }),
	str("secrets.provider", "server", ReadOnly, func(c *config.Config) *string { return &c.Secrets.Provider }),

	// security: sign-in, sessions and the password policy
	flag("auth.local", "security", Runtime, func(c *config.Config) *bool { return &c.Auth.Local }),
	dur("auth.session_ttl", "security", Runtime, 5*time.Minute, 90*24*time.Hour, func(c *config.Config) *time.Duration { return &c.Auth.SessionTTL }),
	checked(dur("auth.session_idle_timeout", "security", Runtime, 0, 30*24*time.Hour, func(c *config.Config) *time.Duration { return &c.Auth.SessionIdleTimeout }), checkIdle),
	num("auth.password_min_length", "security", Runtime, 8, 72, func(c *config.Config) *int { return &c.Auth.PasswordMinLength }),
	num("auth.password_min_classes", "security", Runtime, 0, 4, func(c *config.Config) *int { return &c.Auth.PasswordMinClasses }),
	num("auth.password_history", "security", Runtime, 0, 24, func(c *config.Config) *int { return &c.Auth.PasswordHistory }),
	num("auth.password_max_age_days", "security", Runtime, 0, 3650, func(c *config.Config) *int { return &c.Auth.PasswordMaxAgeDays }),
	num("auth.login_max_attempts", "security", Runtime, 1, 1000, func(c *config.Config) *int { return &c.Auth.LoginMaxAttempts }),
	dur("auth.login_lockout", "security", Runtime, time.Minute, 24*time.Hour, func(c *config.Config) *time.Duration { return &c.Auth.LoginLockout }),
	flag("auth.cookie_secure", "security", ReadOnly, func(c *config.Config) *bool { return &c.Auth.CookieSecure }),
	// passkeys (WebAuthn)
	flag("auth.passkeys.enabled", "security", Runtime, func(c *config.Config) *bool { return &c.Auth.Passkeys.Enabled }),
	flag("auth.passkeys.passwordless", "security", Runtime, func(c *config.Config) *bool { return &c.Auth.Passkeys.Passwordless }),
	flag("auth.passkeys.require_for_admins", "security", Runtime, func(c *config.Config) *bool { return &c.Auth.Passkeys.RequireForAdmins }),
	checked(optional(str("auth.passkeys.rp_id", "security", Runtime, func(c *config.Config) *string { return &c.Auth.Passkeys.RPID })), checkRPID),
	itemChecked(list("auth.passkeys.origins", "security", Runtime, func(c *config.Config) *[]string { return &c.Auth.Passkeys.Origins }), checkOrigin),

	// API tokens
	num("auth.token_max_days", "tokens", Runtime, 0, 3650, func(c *config.Config) *int { return &c.Auth.TokenMaxDays }),
	flag("auth.token_require_expiry", "tokens", Runtime, func(c *config.Config) *bool { return &c.Auth.TokenRequireExpiry }),

	// single sign-on
	{Key: "auth.oidc", Section: "oidc", Kind: OIDCList, Apply: Runtime,
		get: func(c *config.Config) any { return c.Auth.OIDC }, set: func(c *config.Config, v any) { c.Auth.OIDC = v.([]config.OIDC) }},

	// logging
	enum("logging.level", "logging", Runtime, []string{"debug", "info", "warn", "error"}, func(c *config.Config) *string { return &c.Logging.Level }),
	enum("logging.format", "logging", Runtime, []string{"text", "json"}, func(c *config.Config) *string { return &c.Logging.Format }),
	flag("logging.syslog.enabled", "logging", Runtime, func(c *config.Config) *bool { return &c.Logging.Syslog.Enabled }),
	enum("logging.syslog.network", "logging", Runtime, []string{"udp", "tcp", "tls", "unix", "unixgram"}, func(c *config.Config) *string { return &c.Logging.Syslog.Network }),
	checked(optional(str("logging.syslog.address", "logging", Runtime, func(c *config.Config) *string { return &c.Logging.Syslog.Address })), oneLine),
	enum("logging.syslog.format", "logging", Runtime, []string{"rfc5424", "rfc3164"}, func(c *config.Config) *string { return &c.Logging.Syslog.Format }),
	enum("logging.syslog.facility", "logging", Runtime, Facilities, func(c *config.Config) *string { return &c.Logging.Syslog.Facility }),
	checked(str("logging.syslog.tag", "logging", Runtime, func(c *config.Config) *string { return &c.Logging.Syslog.Tag }), checkTag),
	str("logging.syslog.ca_file", "logging", ReadOnly, func(c *config.Config) *string { return &c.Logging.Syslog.CAFile }),
	str("logging.syslog.cert_file", "logging", ReadOnly, func(c *config.Config) *string { return &c.Logging.Syslog.CertFile }),
	str("logging.syslog.key_file", "logging", ReadOnly, func(c *config.Config) *string { return &c.Logging.Syslog.KeyFile }),

	// e-mail
	{Key: "email.enabled", Section: "email", Kind: Bool, Apply: Runtime,
		get: func(c *config.Config) any { return c.Email.Enabled == nil || *c.Email.Enabled },
		set: func(c *config.Config, v any) { b := v.(bool); c.Email.Enabled = &b }},
	checked(optional(str("email.host", "email", Runtime, func(c *config.Config) *string { return &c.Email.Host })), oneLine),
	num("email.port", "email", Runtime, 1, 65535, func(c *config.Config) *int { return &c.Email.Port }),
	enum("email.tls", "email", Runtime, []string{"starttls", "tls", "none"}, func(c *config.Config) *string { return &c.Email.TLS }),
	checked(optional(str("email.username", "email", Runtime, func(c *config.Config) *string { return &c.Email.Username })), oneLine),
	{Key: "email.password", Section: "email", Kind: Secret, Apply: Runtime, Optional: true,
		get: func(c *config.Config) any { return c.Email.Password }, set: func(c *config.Config, v any) { c.Email.Password = v.(string) }},
	checked(optional(str("email.from", "email", Runtime, func(c *config.Config) *string { return &c.Email.From })), oneLine),
	checked(optional(str("email.from_name", "email", Runtime, func(c *config.Config) *string { return &c.Email.FromName })), oneLine),
	checked(optional(str("email.reply_to", "email", Runtime, func(c *config.Config) *string { return &c.Email.ReplyTo })), checkAddress),
	dur("email.timeout", "email", Runtime, time.Second, 10*time.Minute, func(c *config.Config) *time.Duration { return &c.Email.Timeout }),
	num("email.digest_hour", "email", Runtime, 0, 23, func(c *config.Config) *int { return &c.Email.DigestHour }),

	// outgoing HTTP calls: proxy and the hosts they may reach
	checked(optional(str("outbound.proxy", "outbound", Runtime, func(c *config.Config) *string { return &c.Outbound.Proxy })), checkProxy),
	itemChecked(list("outbound.no_proxy", "outbound", Runtime, func(c *config.Config) *[]string { return &c.Outbound.NoProxy }), checkHostPattern),
	itemChecked(list("outbound.allowed_hosts", "outbound", Runtime, func(c *config.Config) *[]string { return &c.Outbound.AllowedHosts }), checkHostPattern),
	itemChecked(list("outbound.denied_hosts", "outbound", Runtime, func(c *config.Config) *[]string { return &c.Outbound.DeniedHosts }), checkHostPattern),
	flag("outbound.block_private", "outbound", Runtime, func(c *config.Config) *bool { return &c.Outbound.BlockPrivate }),

	// script and web test runner (set up at start-up: restart)
	flag("runner.enabled", "runner", Restart, func(c *config.Config) *bool { return &c.Runner.Enabled }),
	dur("runner.timeout", "runner", Restart, time.Second, 24*time.Hour, func(c *config.Config) *time.Duration { return &c.Runner.Timeout }),
	num("runner.max_output_bytes", "runner", Restart, 1024, 64<<20, func(c *config.Config) *int { return &c.Runner.MaxOutputBytes }),
	flag("runner.deny_network", "runner", Restart, func(c *config.Config) *bool { return &c.Runner.DenyNetwork }),
	flag("runner.sandbox", "runner", ReadOnly, func(c *config.Config) *bool { return &c.Runner.Sandbox }),
	str("runner.work_dir", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.WorkDir }),
	roList("runner.interpreters", "runner", func(c *config.Config) []string { return mapList(c.Runner.Interpreters) }),
	flag("runner.web.enabled", "runner", Restart, func(c *config.Config) *bool { return &c.Runner.Web.Enabled }),
	enum("runner.web.browser", "runner", Restart, []string{"chromium", "firefox", "webkit"}, func(c *config.Config) *string { return &c.Runner.Web.Browser }),
	checked(optional(str("runner.web.channel", "runner", Restart, func(c *config.Config) *string { return &c.Runner.Web.Channel })), checkTagOrEmpty),
	flag("runner.web.browser_channels", "runner", Restart, func(c *config.Config) *bool { return &c.Runner.Web.BrowserChannels }),
	flag("runner.web.headless", "runner", Restart, func(c *config.Config) *bool { return &c.Runner.Web.Headless }),
	dur("runner.web.timeout", "runner", Restart, time.Second, 24*time.Hour, func(c *config.Config) *time.Duration { return &c.Runner.Web.Timeout }),
	dur("runner.web.job_timeout", "runner", Restart, time.Second, 24*time.Hour, func(c *config.Config) *time.Duration { return &c.Runner.Web.JobTimeout }),
	num("runner.web.max_parallel", "runner", Restart, 1, 32, func(c *config.Config) *int { return &c.Runner.Web.MaxParallel }),
	num("runner.web.max_evidence_mb", "runner", Restart, 0, 1024, func(c *config.Config) *int { return &c.Runner.Web.MaxEvidenceMB }),
	enum("runner.web.screenshots", "runner", Restart, []string{"on_failure", "always", "off"}, func(c *config.Config) *string { return &c.Runner.Web.Screenshots }),
	enum("runner.web.trace", "runner", Restart, []string{"on_failure", "always", "off"}, func(c *config.Config) *string { return &c.Runner.Web.Trace }),
	enum("runner.web.video", "runner", Restart, []string{"off", "on_failure", "always"}, func(c *config.Config) *string { return &c.Runner.Web.Video }),
	str("runner.web.node", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.Web.Node }),
	str("runner.web.playwright_dir", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.Web.PlaywrightDir }),
	str("runner.web.command", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.Web.Command }),
	str("runner.web.browsers_path", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.Web.BrowsersPath }),
	str("runner.web.executable_path", "runner", ReadOnly, func(c *config.Config) *string { return &c.Runner.Web.ExecutablePath }),

	// live updates: the switch and the heartbeat apply to new streams at
	// once; the limits of the hub are set up at start-up
	flag("live.enabled", "live", Runtime, func(c *config.Config) *bool { return &c.Live.Enabled }),
	dur("live.heartbeat", "live", Runtime, time.Second, 5*time.Minute, func(c *config.Config) *time.Duration { return &c.Live.Heartbeat }),
	dur("live.presence_ttl", "live", Restart, 5*time.Second, time.Hour, func(c *config.Config) *time.Duration { return &c.Live.PresenceTTL }),
	num("live.max_connections", "live", Restart, 1, 1000000, func(c *config.Config) *int { return &c.Live.MaxConnections }),
	num("live.max_connections_per_user", "live", Restart, 1, 1000, func(c *config.Config) *int { return &c.Live.MaxConnectionsPerUser }),
	num("live.replay_buffer", "live", Restart, 0, 100000, func(c *config.Config) *int { return &c.Live.ReplayBuffer }),

	// similarity: the optional embeddings service (off by default; sends
	// the text of requirements and tests to it)
	flag("similarity.embeddings.enabled", "similarity", Runtime, func(c *config.Config) *bool { return &c.Similarity.Embeddings.Enabled }),
	checked(optional(str("similarity.embeddings.url", "similarity", Runtime, func(c *config.Config) *string { return &c.Similarity.Embeddings.URL })), checkEndpoint),
	checked(optional(str("similarity.embeddings.model", "similarity", Runtime, func(c *config.Config) *string { return &c.Similarity.Embeddings.Model })), oneLine),
	{Key: "similarity.embeddings.api_key", Section: "similarity", Kind: Secret, Apply: Runtime, Optional: true,
		get: func(c *config.Config) any { return c.Similarity.Embeddings.APIKey }, set: func(c *config.Config, v any) { c.Similarity.Embeddings.APIKey = v.(string) }},
	dur("similarity.embeddings.timeout", "similarity", Runtime, time.Second, 5*time.Minute, func(c *config.Config) *time.Duration { return &c.Similarity.Embeddings.Timeout }),

	// ai: the optional AI assistance (off by default; sends the text of the
	// items a user asks about to the service)
	flag("ai.enabled", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Enabled }),
	enum("ai.provider", "ai", Runtime, []string{config.AIAnthropic, config.AIOpenAI}, func(c *config.Config) *string { return &c.AI.Provider }),
	checked(optional(str("ai.url", "ai", Runtime, func(c *config.Config) *string { return &c.AI.URL })), checkBase),
	checked(optional(str("ai.model", "ai", Runtime, func(c *config.Config) *string { return &c.AI.Model })), oneLine),
	{Key: "ai.api_key", Section: "ai", Kind: Secret, Apply: Runtime, Optional: true,
		get: func(c *config.Config) any { return c.AI.APIKey }, set: func(c *config.Config, v any) { c.AI.APIKey = v.(string) }},
	dur("ai.timeout", "ai", Runtime, 5*time.Second, 10*time.Minute, func(c *config.Config) *time.Duration { return &c.AI.Timeout }),
	num("ai.max_tokens", "ai", Runtime, 256, 128000, func(c *config.Config) *int { return &c.AI.MaxTokens }),
	checked(optional(str("ai.temperature", "ai", Runtime, func(c *config.Config) *string { return &c.AI.Temperature })), checkTemperature),
	num("ai.rate_limit", "ai", Runtime, 1, 10000, func(c *config.Config) *int { return &c.AI.RateLimit }),
	flag("ai.features.draft_tests", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Features.DraftTests }),
	flag("ai.features.improve_requirement", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Features.ImproveRequirement }),
	flag("ai.features.summarize_run", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Features.SummarizeRun }),
	flag("ai.features.suggest_mappings", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Features.SuggestMappings }),
	flag("ai.features.suggest_criteria", "ai", Runtime, func(c *config.Config) *bool { return &c.AI.Features.SuggestCriteria }),
	checked(optional(str("ai.price_input", "ai", Runtime, func(c *config.Config) *string { return &c.AI.PriceInput })), checkPrice),
	checked(optional(str("ai.price_output", "ai", Runtime, func(c *config.Config) *string { return &c.AI.PriceOutput })), checkPrice),
}

// FileOnly are the keys of the configuration file that are not in the
// Registry, with the reason (the inventory of internal/admindoc checks that
// every key is in one of the two).
var FileOnly = map[string]string{
	"database.dsn_file":                  "the file the database DSN is read from at start-up",
	"email.password_file":                "a secret file read at start-up (the password itself can be set here)",
	"similarity.embeddings.api_key_file": "a secret file read at start-up (the key itself can be set here)",
	"ai.api_key_file":                    "a secret file read at start-up (the key itself can be set here)",
	"bootstrap.admin_password":           "a secret used once, at the first start",
	"bootstrap.admin_password_file":      "a secret file used once, at the first start",
	"auth.saml":                          "SAML providers need key and certificate files: listed read-only",
	"system_settings.locked":             "the operator's lock on these settings: shown read-only",
	"secrets.local.key_file":             "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.tpm.device":                 "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.tpm.pcrs":                   "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.tpm.pcr_bank":               "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.module":              "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.token_label":         "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.slot":                "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.pin_file":            "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.key_label":           "the key that protects the stored secrets: shown in the encryption panel",
	"secrets.pkcs11.generate_key":        "the key that protects the stored secrets: shown in the encryption panel",
}

// list is an editable list of strings.
func list(key, section string, a Apply, p func(*config.Config) *[]string) *Def {
	return &Def{Key: key, Section: section, Kind: List, Apply: a,
		get: func(c *config.Config) any { return append([]string{}, *p(c)...) },
		set: func(c *config.Config, v any) { *p(c) = append([]string(nil), v.([]string)...) }}
}

// roList is a read-only list.
func roList(key, section string, get func(*config.Config) []string) *Def {
	return &Def{Key: key, Section: section, Kind: List, Apply: ReadOnly,
		get: func(c *config.Config) any { return append([]string{}, get(c)...) }}
}

func itemChecked(d *Def, f func(string) error) *Def {
	d.ItemCheck = f
	return d
}

// mapList renders a map as sorted "key=value" entries.
func mapList(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// checkRPID: a domain name (or empty: the host of server.base_url).
func checkRPID(v any) error {
	if s := v.(string); s != "" && !config.ValidRPID(s) {
		return fmt.Errorf("%q is not a domain name such as example.com", s)
	}
	return nil
}

func checkOrigin(s string) error {
	if !config.ValidOrigin(s) {
		return fmt.Errorf("%q is not an origin such as https://doors.example.com", s)
	}
	return nil
}

func checkHostPattern(s string) error {
	if !config.ValidHostPattern(s) {
		return fmt.Errorf("%q is not a host name, address or *.domain pattern", s)
	}
	return nil
}

// checkProxy: an http(s) proxy URL (or empty).
func checkProxy(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(s, " \r\n") {
		return errors.New("must be an http(s) URL such as http://proxy.example.com:3128")
	}
	if _, ok := u.User.Password(); ok {
		return errors.New("a proxy password cannot be stored here: set it in the configuration file or with HTTPS_PROXY")
	}
	return nil
}

func checkAddress(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	if a, err := mail.ParseAddress(s); err != nil || a.Name != "" || strings.ContainsAny(s, "\r\n<>") {
		return errors.New("must be a plain e-mail address such as qtf@example.com")
	}
	return nil
}

func checkIdle(v any) error {
	if t := v.(time.Duration); t != 0 && t < time.Minute {
		return errors.New("must be 0 (off) or at least 1m")
	}
	return nil
}

func checkTagOrEmpty(v any) error {
	if v.(string) == "" {
		return nil
	}
	return checkTag(v)
}

// checkBase: an absolute http(s) base URL without fragment (or empty).
func checkBase(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Fragment != "" || strings.ContainsAny(s, " \r\n") {
		return errors.New("must be an absolute http(s) URL, e.g. https://llm.example.com/v1")
	}
	return nil
}

func checkTemperature(v any) error {
	_, err := config.ParseTemperature(v.(string))
	return err
}

func checkPrice(v any) error {
	_, err := config.ParsePrice(v.(string))
	return err
}

// checkEndpoint: an absolute http(s) URL (or empty).
func checkEndpoint(v any) error {
	s := v.(string)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Fragment != "" || strings.ContainsAny(s, " \r\n") {
		return errors.New("must be an absolute http(s) URL, e.g. https://llm.example.com/v1/embeddings")
	}
	return nil
}

var tagRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,48}$`)

func checkTag(v any) error {
	if !tagRE.MatchString(v.(string)) {
		return errors.New("must be 1-48 letters, digits, dots, dashes or underscores")
	}
	return nil
}

var byKey = func() map[string]*Def {
	m := map[string]*Def{}
	for _, d := range Registry {
		m[d.Key] = d
	}
	return m
}()

// Lookup returns the definition of key.
func Lookup(key string) (*Def, bool) {
	d, ok := byKey[key]
	return d, ok
}

// Get returns the value of the setting in c.
func (d *Def) Get(c *config.Config) any { return d.get(c) }

// Locked reports whether d cannot be changed on the server whose
// configuration (file and environment) is base, and why.
func (d *Def) Locked(base *config.Config) (bool, string) {
	switch {
	case d.Apply == ReadOnly:
		return true, LockReadOnly
	case base.Meta.EnvKeys[d.Key] != "":
		return true, LockEnvironment
	case base.SystemSettings.IsLocked(d.Key):
		return true, LockConfig
	}
	return false, ""
}

// Source returns where the value of d in base comes from (without
// database overrides).
func (d *Def) Source(base *config.Config) string {
	switch {
	case base.Meta.EnvKeys[d.Key] != "":
		return SourceEnvironment
	case base.Meta.FileKeys[d.Key]:
		return SourceFile
	}
	return SourceDefault
}

// Decode parses and validates a JSON value for d. Values of kind
// OIDCList are []OIDCProvider.
func (d *Def) Decode(raw json.RawMessage) (any, error) {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%s: %s", d.Key, fmt.Sprintf(format, a...))
	}
	if d.Apply == ReadOnly {
		return nil, bad("can only be set in the configuration file")
	}
	var v any
	switch d.Kind {
	case String, Enum, Secret:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, bad("expected a string")
		}
		s = strings.TrimSpace(s)
		if d.Kind == Enum && !slices.Contains(d.Options, s) {
			return nil, bad("must be one of %s", strings.Join(d.Options, ", "))
		}
		if s == "" && !d.Optional && d.Kind != Enum {
			return nil, bad("is required")
		}
		if len(s) > 2000 {
			return nil, bad("is too long")
		}
		v = s
	case Int:
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, bad("expected a whole number")
		}
		if n < d.Min || n > d.Max {
			return nil, bad("must be between %d and %d", d.Min, d.Max)
		}
		v = n
	case Bool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, bad("expected true or false")
		}
		v = b
	case Duration:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, bad("expected a duration such as \"30s\", \"15m\" or \"12h\"")
		}
		t, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return nil, bad("expected a duration such as \"30s\", \"15m\" or \"12h\"")
		}
		if t < d.MinDur || t > d.MaxDur {
			return nil, bad("must be between %s and %s", ShortDuration(d.MinDur), ShortDuration(d.MaxDur))
		}
		v = t
	case List:
		var in []string
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, bad("expected a list of strings")
		}
		out := []string{}
		for _, s := range in {
			s = strings.TrimSpace(s)
			if s == "" || slices.Contains(out, s) {
				continue
			}
			if len(s) > 500 || strings.ContainsAny(s, "\r\n") {
				return nil, bad("entries must be one line")
			}
			if d.ItemCheck != nil {
				if err := d.ItemCheck(s); err != nil {
					return nil, bad("%v", err)
				}
			}
			out = append(out, s)
		}
		if len(out) > 200 {
			return nil, bad("at most 200 entries")
		}
		v = out
	case OIDCList:
		var ps []OIDCProvider
		if err := json.Unmarshal(raw, &ps); err != nil {
			return nil, bad("expected a list of providers")
		}
		if err := ValidateProviders(ps); err != nil {
			return nil, bad("%v", err)
		}
		v = ps
	default:
		return nil, bad("cannot be changed")
	}
	if d.Check != nil {
		if err := d.Check(v); err != nil {
			return nil, bad("%v", err)
		}
	}
	return v, nil
}

// Encode returns the JSON stored for a decoded value.
func (d *Def) Encode(v any) json.RawMessage {
	if t, ok := v.(time.Duration); ok {
		v = t.String()
	}
	b, _ := json.Marshal(v)
	return b
}

// ShortDuration formats t without zero units ("12h", "1h30m", "45s").
func ShortDuration(t time.Duration) string {
	s := t.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// Display returns the value of d in c for the API: durations as short
// strings, secrets never (use HasSecret), providers as OIDCProvider.
func (d *Def) Display(c *config.Config) any {
	switch d.Kind {
	case Secret:
		return nil
	case Duration:
		return ShortDuration(d.get(c).(time.Duration))
	case OIDCList:
		return nil
	}
	if d.Key == "outbound.proxy" {
		return RedactURL(d.get(c).(string))
	}
	return d.get(c)
}

// RedactURL replaces the password of a URL (proxy credentials) by
// "REDACTED"; other values are returned unchanged.
func RedactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "REDACTED")
	}
	return u.String()
}

// Effective returns base with the stored overrides applied. Overrides of
// locked, read-only, unknown or invalid settings are skipped and reported
// in ignored. only, when set, limits the applied settings (e.g. to the
// runtime ones).
func Effective(base config.Config, stored map[string]json.RawMessage, only func(*Def) bool) (cfg config.Config, ignored []string) {
	cfg = base
	keys := make([]string, 0, len(stored))
	for k := range stored {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		d, ok := byKey[k]
		if !ok || d.set == nil {
			ignored = append(ignored, k)
			continue
		}
		if locked, _ := d.Locked(&base); locked {
			ignored = append(ignored, k)
			continue
		}
		if only != nil && !only(d) {
			continue
		}
		v, err := d.Decode(stored[k])
		if err != nil {
			ignored = append(ignored, k)
			continue
		}
		if d.Kind == OIDCList {
			v = ResolveProviders(&base, v.([]OIDCProvider))
		}
		d.set(&cfg, v)
	}
	return cfg, ignored
}

// CopyRestart copies the values of the restart settings from running to
// cfg (they keep their start-up value until the next start).
func CopyRestart(cfg *config.Config, running *config.Config) {
	for _, d := range Registry {
		if d.Apply == Restart && d.set != nil {
			d.set(cfg, d.get(running))
		}
	}
}

// ---------------------------------------------------------------- OIDC providers

// OIDCProvider is an OpenID Connect provider as stored in the database and
// exchanged with the administration view. ClientSecret is write-only: the
// API never returns it.
type OIDCProvider struct {
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name"`
	Icon          string            `json:"icon"`
	Enabled       bool              `json:"enabled"`
	Issuer        string            `json:"issuer"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret,omitempty"`
	Scopes        []string          `json:"scopes"`
	EmailClaim    string            `json:"email_claim"`
	NameClaim     string            `json:"name_claim"`
	GroupsClaim   string            `json:"groups_claim"`
	AutoProvision bool              `json:"auto_provision"`
	DefaultTenant string            `json:"default_tenant"`
	DefaultRole   string            `json:"default_role"`
	RoleMapping   map[string]string `json:"role_mapping"`
	// AllowedDomains: only e-mail addresses of these domains may sign in
	// (empty = any).
	AllowedDomains []string `json:"allowed_domains"`
	// LocalMFA: whether Qtestframe asks for its own second factor after a
	// sign-in through this provider ("off", "unless_idp_mfa", "always");
	// the amr/acr values count as MFA done by the provider.
	LocalMFA     string   `json:"local_mfa"`
	MFAAMRValues []string `json:"mfa_amr_values"`
	MFAACRValues []string `json:"mfa_acr_values"`
}

var providerNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// ValidateProvider checks one provider.
func ValidateProvider(p OIDCProvider) error {
	var errs []error
	if !providerNameRE.MatchString(p.Name) {
		errs = append(errs, fmt.Errorf("oidc provider %q: the name must be 1-40 lower-case letters, digits, - or _", p.Name))
	}
	if u, err := url.Parse(p.Issuer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		errs = append(errs, fmt.Errorf("oidc provider %q: the issuer must be an http(s) URL", p.Name))
	}
	if strings.TrimSpace(p.ClientID) == "" {
		errs = append(errs, fmt.Errorf("oidc provider %q: client_id is required", p.Name))
	}
	for _, s := range append(append([]string{p.DisplayName, p.ClientID, p.EmailClaim, p.NameClaim, p.GroupsClaim, p.DefaultTenant, p.DefaultRole}, p.Scopes...), p.ClientSecret) {
		if strings.ContainsAny(s, "\r\n") || len(s) > 2000 {
			errs = append(errs, fmt.Errorf("oidc provider %q: values must be one line", p.Name))
			break
		}
	}
	switch p.LocalMFA {
	case "", config.LocalMFAOff, config.LocalMFAUnlessIdPMFA, config.LocalMFAAlways:
	default:
		errs = append(errs, fmt.Errorf("oidc provider %q: local_mfa must be off, unless_idp_mfa or always", p.Name))
	}
	if !slices.Contains(config.OIDCIcons, p.Icon) {
		errs = append(errs, fmt.Errorf("oidc provider %q: icon must be one of %s", p.Name, strings.Join(config.OIDCIcons[1:], ", ")))
	}
	for _, d := range p.AllowedDomains {
		if strings.Contains(d, "@") || !config.ValidHostPattern(d) {
			errs = append(errs, fmt.Errorf("oidc provider %q: %q is not a domain such as example.com", p.Name, d))
			break
		}
	}
	for g, r := range p.RoleMapping {
		if strings.TrimSpace(g) == "" || strings.TrimSpace(r) == "" || strings.ContainsAny(g+r, "\r\n") {
			errs = append(errs, fmt.Errorf("oidc provider %q: role mappings need a group and at least one role", p.Name))
			break
		}
	}
	for _, s := range append(append([]string{}, p.MFAAMRValues...), p.MFAACRValues...) {
		if strings.ContainsAny(s, " \t\r\n") || s == "" || len(s) > 200 {
			errs = append(errs, fmt.Errorf("oidc provider %q: amr/acr values must not be empty or contain spaces", p.Name))
			break
		}
	}
	for _, s := range p.Scopes {
		if strings.ContainsAny(s, " \t") || s == "" {
			errs = append(errs, fmt.Errorf("oidc provider %q: scopes must not be empty or contain spaces", p.Name))
			break
		}
	}
	return errors.Join(errs...)
}

// ValidateProviders checks a provider list (also for duplicate names).
func ValidateProviders(ps []OIDCProvider) error {
	var errs []error
	seen := map[string]bool{}
	for _, p := range ps {
		if err := ValidateProvider(p); err != nil {
			errs = append(errs, err)
		}
		if seen[p.Name] {
			errs = append(errs, fmt.Errorf("duplicate oidc provider %q", p.Name))
		}
		seen[p.Name] = true
	}
	return errors.Join(errs...)
}

// FromConfig converts configured providers (secrets included).
func FromConfig(list []config.OIDC) []OIDCProvider {
	out := make([]OIDCProvider, 0, len(list))
	for _, o := range list {
		out = append(out, OIDCProvider{
			Name: o.Name, DisplayName: o.DisplayName, Icon: o.Icon, Enabled: o.On(), Issuer: o.Issuer, ClientID: o.ClientID,
			ClientSecret: o.ClientSecret, Scopes: append([]string{}, o.Scopes...),
			EmailClaim: o.EmailClaim, NameClaim: o.NameClaim, GroupsClaim: o.GroupsClaim,
			AutoProvision: o.AutoProvision, DefaultTenant: o.DefaultTenant, DefaultRole: o.DefaultRole,
			RoleMapping: o.RoleMapping, AllowedDomains: append([]string{}, o.AllowedDomains...), LocalMFA: o.LocalMFA,
			MFAAMRValues: append([]string{}, o.MFAAMRValues...), MFAACRValues: append([]string{}, o.MFAACRValues...),
		})
	}
	return out
}

// ResolveProviders converts stored providers to the configuration. The
// client secret is, in this order: QTF_OIDC_<NAME>_CLIENT_SECRET, the
// secret stored with the provider, the secret of the provider with the
// same name in the configuration file (which is thus never copied into
// the database).
func ResolveProviders(base *config.Config, ps []OIDCProvider) []config.OIDC {
	out := make([]config.OIDC, 0, len(ps))
	for _, p := range ps {
		secret := p.ClientSecret
		if env := os.Getenv(config.OIDCSecretEnv(p.Name)); env != "" {
			secret = env
		} else if secret == "" {
			for _, b := range base.Auth.OIDC {
				if b.Name == p.Name {
					secret = b.ClientSecret
				}
			}
		}
		on := p.Enabled
		out = append(out, config.OIDC{
			Name: p.Name, DisplayName: p.DisplayName, Icon: p.Icon, Enabled: &on, Issuer: p.Issuer, ClientID: p.ClientID,
			ClientSecret: secret, Scopes: append([]string{}, p.Scopes...),
			EmailClaim: p.EmailClaim, NameClaim: p.NameClaim, GroupsClaim: p.GroupsClaim,
			Provisioning: config.Provisioning{AutoProvision: p.AutoProvision, DefaultTenant: p.DefaultTenant,
				DefaultRole: p.DefaultRole, RoleMapping: p.RoleMapping, AllowedDomains: append([]string(nil), p.AllowedDomains...)},
			LocalMFA: p.LocalMFA, MFAAMRValues: append([]string{}, p.MFAAMRValues...), MFAACRValues: append([]string{}, p.MFAACRValues...),
		})
	}
	return out
}

// SecretSource says where the client secret of provider name comes from
// with the stored list (nil = providers of the file): "environment",
// "database", "file" or "" (none).
func SecretSource(base *config.Config, stored []OIDCProvider, name string) string {
	if os.Getenv(config.OIDCSecretEnv(name)) != "" {
		return SourceEnvironment
	}
	if stored != nil {
		for _, p := range stored {
			if p.Name == name && p.ClientSecret != "" {
				return SourceDatabase
			}
		}
	}
	for _, b := range base.Auth.OIDC {
		if b.Name == name && b.ClientSecret != "" {
			return SourceFile
		}
	}
	return ""
}
