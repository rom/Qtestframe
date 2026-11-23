package api

// System settings (Administration → System settings): super administrators
// change a whitelisted set of configuration keys at runtime. Overrides are
// stored in the database (table system_settings); the effective value is
// the override, else the environment / configuration file value, else the
// default (see internal/settings for the rules and the list of settings).
// Runtime settings apply at once: the e-mail sender, the OIDC providers,
// the logger and the login limits are rebuilt here. Restart settings (the
// script and web test runner) are stored and apply at the next start.

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/logging"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/settings"
	"github.com/rom/qtestframe/internal/store"
)

// sysState is the runtime state of the system settings.
type sysState struct {
	// mu serialises changes of the settings.
	mu sync.Mutex
	// live is the effective configuration (nil = Server.Cfg unchanged).
	live atomic.Pointer[config.Config]
	// boot is the effective configuration at start-up (restart settings
	// keep these values until the next start).
	boot *config.Config
	// loadErr explains why stored settings were not applied at start-up.
	loadErr string

	// mailMu guards Server.Mail; ownMail is true when the sender was built
	// from the settings (and is rebuilt when they change).
	mailMu  sync.RWMutex
	ownMail bool
	// oidcMu guards Server.OIDC; oidcSrc is the configuration each
	// provider was built from (unchanged providers are kept).
	oidcMu  sync.RWMutex
	oidcSrc map[string]settings.OIDCProvider

	// newSMTP builds the sender of "Send test e-mail" (tests replace it).
	newSMTP func(mail.SMTPConfig) mail.Sender
}

// conf returns the effective configuration. Callers must not modify it.
func (s *Server) conf() *config.Config {
	if c := s.sys.live.Load(); c != nil {
		return c
	}
	return &s.Cfg
}

// minPassword is the minimum password length of c.
func minPassword(c *config.Config) int {
	if c.Auth.PasswordMinLength > 0 {
		return c.Auth.PasswordMinLength
	}
	return auth.MinPasswordLength
}

// pwPolicy is the password policy of c.
func pwPolicy(c *config.Config) auth.PasswordPolicy {
	return auth.PasswordPolicy{MinLength: minPassword(c), MinClasses: c.Auth.PasswordMinClasses}
}

// mailer returns the current e-mail sender (nil = e-mail off).
func (s *Server) mailer() mail.Sender {
	s.sys.mailMu.RLock()
	defer s.sys.mailMu.RUnlock()
	return s.Mail
}

// oidcProvider returns the enabled OIDC provider name.
func (s *Server) oidcProvider(name string) (*auth.OIDCProvider, bool) {
	s.sys.oidcMu.RLock()
	defer s.sys.oidcMu.RUnlock()
	p, ok := s.OIDC[name]
	return p, ok
}

func rawSettings(stored map[string]store.SystemSetting) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(stored))
	for k, v := range stored {
		out[k] = v.Value
	}
	return out
}

// initSettings applies the stored settings at start-up. Invalid stored
// settings are ignored (with an error in the log and the settings view).
func (s *Server) initSettings(ctx context.Context) {
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		s.Log.Warn("system settings not loaded", "err", err)
		s.sys.loadErr = err.Error()
	}
	eff, err := s.effective(rawSettings(stored))
	if err != nil {
		s.Log.Error("stored system settings are invalid and were ignored; using the configuration file", "err", err)
		s.sys.loadErr = err.Error()
		eff = s.Cfg
	}
	boot := eff
	s.sys.boot = &boot
	s.install(&eff, true)
}

// EffectiveConfig returns base (file and environment) with the system
// settings stored in st applied, restart settings included, as the server
// uses it at start-up. On error (invalid stored settings) it returns base
// and the error.
func EffectiveConfig(ctx context.Context, st *store.Store, base config.Config) (config.Config, error) {
	stored, err := st.SystemSettings(ctx)
	if err != nil {
		return base, err
	}
	eff, err := (&Server{Cfg: base}).effective(rawSettings(stored))
	if err != nil {
		return base, err
	}
	return eff, nil
}

// ReloadSettings re-reads the stored settings (after a restore).
func (s *Server) ReloadSettings(ctx context.Context) error {
	s.sys.mu.Lock()
	defer s.sys.mu.Unlock()
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		return err
	}
	live, err := s.liveConfig(rawSettings(stored))
	if err != nil {
		return err
	}
	s.install(&live, false)
	return nil
}

// effective returns the configuration with all stored overrides (restart
// settings included) and validates it.
func (s *Server) effective(raw map[string]json.RawMessage) (config.Config, error) {
	eff, _ := settings.Effective(s.Cfg, raw, nil)
	if err := eff.Validate(); err != nil {
		return eff, err
	}
	if !eff.Auth.Local && !hasSSO(&eff) {
		return eff, errors.New("auth.local: local sign-in cannot be turned off while no single sign-on provider is enabled")
	}
	if eff.Logging.Syslog.Enabled {
		if _, err := logging.NewWriter(eff.Logging.Syslog); err != nil {
			return eff, err
		}
	}
	return eff, nil
}

func hasSSO(c *config.Config) bool {
	if len(c.Auth.SAML) > 0 {
		return true
	}
	for _, o := range c.Auth.OIDC {
		if o.On() {
			return true
		}
	}
	return false
}

// liveConfig is effective with the restart settings at their start-up values.
func (s *Server) liveConfig(raw map[string]json.RawMessage) (config.Config, error) {
	eff, err := s.effective(raw)
	if err != nil {
		return eff, err
	}
	if s.sys.boot != nil {
		settings.CopyRestart(&eff, s.sys.boot)
	}
	return eff, nil
}

func smtpConfig(e config.Email) mail.SMTPConfig {
	return mail.SMTPConfig{Host: e.Host, Port: e.Port, Username: e.Username, Password: e.Password, TLS: e.TLS, Timeout: e.Timeout}
}

// install makes next the live configuration and rebuilds what depends on
// it: the e-mail sender, the OIDC providers, the login limits and (when
// Logs is set) the logger.
func (s *Server) install(next *config.Config, initial bool) {
	prev := s.conf()
	if s.Logs != nil && !reflect.DeepEqual(prev.Logging, next.Logging) {
		if err := s.Logs.Reconfigure(next.Logging); err != nil {
			s.Log.Error("logging settings not applied", "err", err)
		}
	}
	if initial || !reflect.DeepEqual(prev.Email, next.Email) {
		s.sys.mailMu.Lock()
		if initial || s.sys.ownMail || s.Mail == nil {
			s.Mail, s.sys.ownMail = nil, false
			if next.Email.On() {
				s.Mail, s.sys.ownMail = mail.NewSMTP(smtpConfig(next.Email)), true
			}
		}
		s.sys.mailMu.Unlock()
	}
	s.installOIDC(next)
	s.limiter.setLimits(next.Auth.LoginMaxAttempts, next.Auth.LoginLockout)
	if reflect.DeepEqual(*next, s.Cfg) {
		s.sys.live.Store(nil)
	} else {
		c := *next
		s.sys.live.Store(&c)
	}
}

// installOIDC rebuilds the provider set. Providers whose configuration is
// unchanged are kept (with their discovery state), so sign-ins in progress
// continue; a sign-in that started at a removed provider fails at the
// callback ("unknown provider").
func (s *Server) installOIDC(next *config.Config) {
	base := strings.TrimSuffix(next.Server.BaseURL, "/")
	s.sys.oidcMu.Lock()
	defer s.sys.oidcMu.Unlock()
	m := map[string]*auth.OIDCProvider{}
	src := map[string]settings.OIDCProvider{}
	for _, o := range next.Auth.OIDC {
		if !o.On() {
			continue
		}
		redirect := base + "/api/v1/auth/oidc/" + url.PathEscape(o.Name) + "/callback"
		if old, ok := s.OIDC[o.Name]; ok && reflect.DeepEqual(s.sys.oidcSrc[o.Name], settings.FromConfig([]config.OIDC{o})[0]) && old.RedirectURL == redirect {
			m[o.Name] = old
		} else {
			p := auth.NewOIDCProvider(o, redirect)
			if s.HTTPClient != nil {
				p.HTTPClient = s.HTTPClient
			}
			m[o.Name] = p
		}
		src[o.Name] = settings.FromConfig([]config.OIDC{o})[0]
	}
	s.OIDC, s.sys.oidcSrc = m, src
}

func (s *Server) routesSystemSettings(h routeFunc) {
	h("GET /api/v1/admin/settings", s.handleGetSystemSettings)
	h("PUT /api/v1/admin/settings", s.handlePutSystemSettings)
	h("GET /api/v1/admin/settings/export", s.handleExportSystemSettings)
	h("DELETE /api/v1/admin/settings/{key}", s.handleResetSystemSetting)
	h("POST /api/v1/admin/settings/oidc", s.handleCreateOIDCProvider)
	h("PUT /api/v1/admin/settings/oidc/{name}", s.handleUpdateOIDCProvider)
	h("DELETE /api/v1/admin/settings/oidc/{name}", s.handleDeleteOIDCProvider)
	h("POST /api/v1/admin/settings/test/oidc", s.handleTestOIDC)
	h("POST /api/v1/admin/settings/test/syslog", s.handleTestSyslog)
	h("POST /api/v1/admin/settings/test/email", s.handleTestSettingsEmail)
	h("POST /api/v1/admin/settings/test/outbound", s.handleTestOutbound)
	h("POST /api/v1/admin/settings/test/saml", s.handleTestSAML)
}

// ---------------------------------------------------------------- views

type settingView struct {
	Key     string   `json:"key"`
	Section string   `json:"section"`
	Type    string   `json:"type"`
	Apply   string   `json:"apply"`
	Options []string `json:"options,omitempty"`
	Min     *int64   `json:"min,omitempty"`
	Max     *int64   `json:"max,omitempty"`
	// Value is the effective value (for restart settings: the value that
	// applies after the next start); never set for secrets.
	Value any `json:"value"`
	// RunningValue is the value in use of a restart setting.
	RunningValue   any  `json:"running_value,omitempty"`
	PendingRestart bool `json:"pending_restart"`
	// FileValue is the value without the database override and
	// FileSource where it comes from (default, file or environment).
	FileValue  any    `json:"file_value"`
	FileSource string `json:"file_source"`
	// Source is where the effective value comes from.
	Source     string     `json:"source"`
	Stored     bool       `json:"stored"`
	Ignored    bool       `json:"ignored"`
	Locked     bool       `json:"locked"`
	LockReason string     `json:"lock_reason,omitempty"`
	Env        string     `json:"env,omitempty"`
	HasSecret  *bool      `json:"has_secret,omitempty"`
	FileSecret *bool      `json:"file_has_secret,omitempty"`
	UpdatedBy  string     `json:"updated_by,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type oidcView struct {
	settings.OIDCProvider
	HasSecret    bool   `json:"has_secret"`
	SecretSource string `json:"secret_source"`
	SecretEnv    string `json:"secret_env"`
	InFile       bool   `json:"in_file"`
	CallbackURL  string `json:"callback_url"`
}

type oidcSection struct {
	Providers  []oidcView `json:"providers"`
	Source     string     `json:"source"`
	FileSource string     `json:"file_source"`
	Stored     bool       `json:"stored"`
	Locked     bool       `json:"locked"`
	LockReason string     `json:"lock_reason,omitempty"`
	UpdatedBy  string     `json:"updated_by,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

// samlView is a SAML provider of the configuration file (read-only).
type samlView struct {
	Name           string            `json:"name"`
	DisplayName    string            `json:"display_name"`
	EntityID       string            `json:"entity_id"`
	MetadataURL    string            `json:"metadata_url"`
	MetadataFile   string            `json:"metadata_file"`
	SPCertFile     string            `json:"sp_cert_file"`
	EmailAttr      string            `json:"email_attr"`
	NameAttr       string            `json:"name_attr"`
	GroupsAttr     string            `json:"groups_attr"`
	AutoProvision  bool              `json:"auto_provision"`
	DefaultTenant  string            `json:"default_tenant"`
	DefaultRole    string            `json:"default_role"`
	RoleMapping    map[string]string `json:"role_mapping"`
	AllowedDomains []string          `json:"allowed_domains"`
	// ACSURL and SPMetadataURL are the addresses to register at the
	// identity provider.
	ACSURL        string `json:"acs_url"`
	SPMetadataURL string `json:"sp_metadata_url"`
}

type systemSettingsView struct {
	Sections []string      `json:"sections"`
	Settings []settingView `json:"settings"`
	OIDC     oidcSection   `json:"oidc"`
	// SAML lists the SAML providers of the configuration file (they need
	// key and certificate files and cannot be changed here).
	SAML []samlView `json:"saml"`
	// OIDCIcons are the icons a sign-in button can have.
	OIDCIcons []string `json:"oidc_icons"`
	// Locked are the patterns of system_settings.locked.
	Locked         []string `json:"locked_patterns"`
	ConfigFile     string   `json:"config_file"`
	RestartPending bool     `json:"restart_pending"`
	LoadError      string   `json:"load_error,omitempty"`
	// TrashRetentionDays is fixed (not a setting).
	TrashRetentionDays int `json:"trash_retention_days"`
	// LoggingApplies is false when logging changes apply after a restart
	// only (the server has no runtime logger).
	LoggingRuntime bool `json:"logging_runtime"`
	// Secrets describes the encryption of stored secrets (read-only: it is
	// configured in the file, section secrets).
	Secrets *secretsView `json:"secrets,omitempty"`
}

// secretsView is the key provider protecting the stored secrets.
type secretsView struct {
	// Provider is local, tpm or pkcs11; KeyID identifies its key (a
	// fingerprint, never the key).
	Provider string            `json:"provider"`
	KeyID    string            `json:"key_id"`
	Info     map[string]string `json:"info"`
	// ActiveKey is the data-encryption key new values are encrypted with;
	// Keys counts the stored data-encryption keys.
	ActiveKey string `json:"active_key"`
	Keys      int    `json:"keys"`
	// Encrypted and Plaintext count the stored secret values.
	Encrypted int `json:"encrypted"`
	Plaintext int `json:"plaintext"`
}

func (s *Server) secretsView(ctx context.Context) (*secretsView, error) {
	kr := s.Store.Keyring()
	if kr == nil {
		return nil, nil
	}
	p := kr.Provider()
	v := &secretsView{Provider: p.Name(), KeyID: p.KeyID(), Info: p.Info(), ActiveKey: kr.Active()}
	keys, err := s.Store.SecretKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.Keys = len(keys)
	st, err := s.Store.SecretStatus(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range st.Encrypted {
		v.Encrypted += n
	}
	v.Plaintext = st.Plaintext
	return v, nil
}

func ptr[T any](v T) *T { return &v }

func (s *Server) settingsView(ctx context.Context) (*systemSettingsView, error) {
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		return nil, err
	}
	raw := rawSettings(stored)
	base := &s.Cfg
	eff, ignored := settings.Effective(s.Cfg, raw, nil)
	boot := s.sys.boot
	if boot == nil {
		boot = base
	}
	v := &systemSettingsView{Sections: settings.Sections, Settings: []settingView{}, Locked: append([]string{}, s.Cfg.SystemSettings.Locked...),
		ConfigFile: s.Cfg.Meta.File, LoadError: s.sys.loadErr, TrashRetentionDays: int(store.TrashRetention.Hours() / 24),
		LoggingRuntime: s.Logs != nil}
	if v.Locked == nil {
		v.Locked = []string{}
	}
	v.OIDCIcons = config.OIDCIcons
	v.SAML = []samlView{}
	baseURL := strings.TrimSuffix(eff.Server.BaseURL, "/")
	for _, sc := range eff.Auth.SAML {
		sp := baseURL + "/api/v1/auth/saml/" + url.PathEscape(sc.Name)
		x := samlView{Name: sc.Name, DisplayName: sc.DisplayName, EntityID: sc.EntityID, MetadataURL: sc.IDPMetadataURL,
			MetadataFile: sc.IDPMetadataFile, SPCertFile: sc.SPCertFile, EmailAttr: sc.EmailAttr, NameAttr: sc.NameAttr,
			GroupsAttr: sc.GroupsAttr, AutoProvision: sc.AutoProvision, DefaultTenant: sc.DefaultTenant, DefaultRole: sc.DefaultRole,
			RoleMapping: sc.RoleMapping, AllowedDomains: append([]string{}, sc.AllowedDomains...), ACSURL: sp + "/acs", SPMetadataURL: sp + "/metadata"}
		if x.EntityID == "" {
			x.EntityID = sp + "/metadata"
		}
		v.SAML = append(v.SAML, x)
	}
	for _, d := range settings.Registry {
		x := settingView{Key: d.Key, Section: d.Section, Type: string(d.Kind), Apply: string(d.Apply), Options: d.Options}
		if d.Kind == settings.Int {
			x.Min, x.Max = ptr(d.Min), ptr(d.Max)
		}
		x.Locked, x.LockReason = d.Locked(base)
		x.Env = base.Meta.EnvKeys[d.Key]
		x.FileSource = d.Source(base)
		x.FileValue = d.Display(base)
		x.Value = d.Display(&eff)
		x.Source = x.FileSource
		if st, ok := stored[d.Key]; ok {
			x.Stored = true
			x.Ignored = slices.Contains(ignored, d.Key)
			if !x.Ignored {
				x.Source = settings.SourceDatabase
			}
			x.UpdatedBy, x.UpdatedAt = st.UpdatedBy, ptr(st.UpdatedAt)
		}
		if d.Kind == settings.Secret {
			x.HasSecret, x.FileSecret = ptr(d.Get(&eff).(string) != ""), ptr(d.Get(base).(string) != "")
		}
		if d.Kind == settings.List && x.Value == nil {
			x.Value, x.FileValue = []string{}, []string{}
		}
		if d.Apply == settings.Restart {
			x.RunningValue = d.Display(boot)
			x.PendingRestart = !reflect.DeepEqual(x.RunningValue, x.Value)
			v.RestartPending = v.RestartPending || x.PendingRestart
		}
		if d.Kind == settings.OIDCList {
			v.OIDC = s.oidcSectionView(base, &eff, stored, ignored, x)
			continue
		}
		v.Settings = append(v.Settings, x)
	}
	if v.Secrets, err = s.secretsView(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Server) oidcSectionView(base, eff *config.Config, stored map[string]store.SystemSetting, ignored []string, x settingView) oidcSection {
	sec := oidcSection{Providers: []oidcView{}, Source: x.Source, FileSource: x.FileSource, Stored: x.Stored,
		Locked: x.Locked, LockReason: x.LockReason, UpdatedBy: x.UpdatedBy, UpdatedAt: x.UpdatedAt}
	var list []settings.OIDCProvider
	if st, ok := stored["auth.oidc"]; ok && !slices.Contains(ignored, "auth.oidc") {
		_ = json.Unmarshal(st.Value, &list)
	}
	baseURL := strings.TrimSuffix(eff.Server.BaseURL, "/")
	for _, p := range settings.FromConfig(eff.Auth.OIDC) {
		view := oidcView{OIDCProvider: p, HasSecret: p.ClientSecret != "", SecretSource: settings.SecretSource(base, list, p.Name),
			SecretEnv: config.OIDCSecretEnv(p.Name), CallbackURL: baseURL + "/api/v1/auth/oidc/" + url.PathEscape(p.Name) + "/callback"}
		view.ClientSecret = "" // write-only
		if view.Scopes == nil {
			view.Scopes = []string{}
		}
		for _, b := range base.Auth.OIDC {
			if b.Name == p.Name {
				view.InFile = true
			}
		}
		sec.Providers = append(sec.Providers, view)
	}
	return sec
}

func (s *Server) handleGetSystemSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	v, err := s.settingsView(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------------------------------------------------------------- changes

var errSettingLocked = errors.New("locked")

func lockedError(d *settings.Def, base *config.Config) error {
	_, why := d.Locked(base)
	switch why {
	case settings.LockEnvironment:
		return fmt.Errorf("%w: %s is set by the environment variable %s and cannot be changed here", errSettingLocked, d.Key, base.Meta.EnvKeys[d.Key])
	case settings.LockConfig:
		return fmt.Errorf("%w: %s is locked by system_settings.locked in the configuration file", errSettingLocked, d.Key)
	}
	return fmt.Errorf("%w: %s can only be set in the configuration file", errSettingLocked, d.Key)
}

// change is one requested change: a JSON value or nil (back to the file
// value).
type change struct {
	key   string
	value json.RawMessage
}

// applyChanges validates, stores, applies and audits changes. Restart
// settings are stored only.
func (s *Server) applyChanges(ctx context.Context, p *Principal, changes []change, describe func(old, next *config.Config) []string) error {
	s.sys.mu.Lock()
	defer s.sys.mu.Unlock()
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		return err
	}
	raw := rawSettings(stored)
	prevEff, _ := settings.Effective(s.Cfg, raw, nil)
	next := map[string]json.RawMessage{}
	for k, v := range raw {
		next[k] = v
	}
	write := map[string]json.RawMessage{}
	for _, c := range changes {
		d, ok := settings.Lookup(c.key)
		if !ok {
			return badRequest("unknown setting %q", c.key)
		}
		if locked, _ := d.Locked(&s.Cfg); locked {
			return lockedError(d, &s.Cfg)
		}
		if c.value == nil || string(c.value) == "null" || (d.Kind == settings.Secret && string(c.value) == `""`) {
			delete(next, c.key)
			write[c.key] = nil
			continue
		}
		v, err := d.Decode(c.value)
		if err != nil {
			return badRequest("%v", err)
		}
		next[c.key] = d.Encode(v)
		write[c.key] = next[c.key]
	}
	if len(write) == 0 {
		return nil
	}
	live, err := s.liveConfig(next)
	if err != nil {
		return badRequest("%v", err)
	}
	eff, _ := settings.Effective(s.Cfg, next, nil)
	if err := s.Store.SaveSystemSettings(ctx, write, p.User.Email); err != nil {
		return err
	}
	s.install(&live, false)
	var lines []string
	if describe != nil {
		lines = describe(&prevEff, &eff)
	} else {
		for _, k := range sortedKeys(write) {
			d, _ := settings.Lookup(k)
			lines = append(lines, describeChange(d, &prevEff, &eff, write[k] == nil))
		}
	}
	s.audit(ctx, p, "settings.update", "system_settings", strings.Join(sortedKeys(write), ","), strings.Join(lines, "; "))
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// describeChange is the audit text of one change (secrets redacted).
func describeChange(d *settings.Def, old, next *config.Config, reset bool) string {
	suffix := ""
	if reset {
		suffix = " (reset to the file value)"
	}
	if d.Kind == settings.Secret {
		was, is := d.Get(old).(string) != "", d.Get(next).(string) != ""
		state := map[bool]string{true: "set", false: "not set"}
		return fmt.Sprintf("%s: %s → %s (changed)%s", d.Key, state[was], state[is], suffix)
	}
	return fmt.Sprintf("%s: %s → %s%s", d.Key, auditValue(d.Display(old)), auditValue(d.Display(next)), suffix)
}

func auditValue(v any) string {
	if s, ok := v.(string); ok {
		if s == "" {
			return `""`
		}
		return s
	}
	return fmt.Sprint(v)
}

func (s *Server) handlePutSystemSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	changes := make([]change, 0, len(req.Values))
	for _, k := range sortedKeys(req.Values) {
		if k == "auth.oidc" {
			s.fail(w, r, badRequest("OIDC providers are changed with /api/v1/admin/settings/oidc"))
			return
		}
		changes = append(changes, change{k, req.Values[k]})
	}
	s.respondChanges(w, r, p, changes, nil)
}

func (s *Server) respondChanges(w http.ResponseWriter, r *http.Request, p *Principal, changes []change, describe func(old, next *config.Config) []string) {
	if err := s.applyChanges(r.Context(), p, changes, describe); err != nil {
		if errors.Is(err, errSettingLocked) {
			writeError(w, http.StatusConflict, "locked", err.Error())
			return
		}
		s.fail(w, r, err)
		return
	}
	s.handleGetSystemSettings(w, r, p)
}

func (s *Server) handleResetSystemSetting(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	key := r.PathValue("key")
	if _, ok := settings.Lookup(key); !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown setting "+key)
		return
	}
	s.respondChanges(w, r, p, []change{{key: key}}, nil)
}

// ---------------------------------------------------------------- OIDC providers

// currentProviders returns the provider list to edit: the stored one or
// the providers of the file (without their secrets, which stay there).
func (s *Server) currentProviders(ctx context.Context) ([]settings.OIDCProvider, error) {
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		return nil, err
	}
	if st, ok := stored["auth.oidc"]; ok {
		var list []settings.OIDCProvider
		if json.Unmarshal(st.Value, &list) == nil {
			return list, nil
		}
	}
	list := settings.FromConfig(s.Cfg.Auth.OIDC)
	for i := range list {
		list[i].ClientSecret = ""
	}
	return list, nil
}

type oidcRequest struct {
	settings.OIDCProvider
	// ClearSecret removes the secret stored in the database (the secret
	// of the configuration file or the environment, if any, then applies).
	ClearSecret bool `json:"clear_secret"`
}

func normalizeProvider(p *settings.OIDCProvider) {
	p.Name = strings.TrimSpace(p.Name)
	p.Issuer = strings.TrimSpace(p.Issuer)
	p.ClientID = strings.TrimSpace(p.ClientID)
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.ClientSecret = strings.TrimSpace(p.ClientSecret)
	scopes := []string{}
	for _, sc := range p.Scopes {
		if sc = strings.TrimSpace(sc); sc != "" && !slices.Contains(scopes, sc) {
			scopes = append(scopes, sc)
		}
	}
	p.Scopes = scopes
	if len(p.RoleMapping) == 0 {
		p.RoleMapping = nil
	}
}

func (s *Server) changeProviders(w http.ResponseWriter, r *http.Request, p *Principal, edit func([]settings.OIDCProvider) ([]settings.OIDCProvider, string, error)) {
	if !requireSuper(w, p) {
		return
	}
	d, _ := settings.Lookup("auth.oidc")
	if locked, _ := d.Locked(&s.Cfg); locked {
		writeError(w, http.StatusConflict, "locked", lockedError(d, &s.Cfg).Error())
		return
	}
	list, err := s.currentProviders(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, what, err := edit(list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := json.Marshal(list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.respondChanges(w, r, p, []change{{key: "auth.oidc", value: b}}, func(old, next *config.Config) []string {
		return []string{"auth.oidc: " + what}
	})
}

func (s *Server) handleCreateOIDCProvider(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req oidcRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	normalizeProvider(&req.OIDCProvider)
	s.changeProviders(w, r, p, func(list []settings.OIDCProvider) ([]settings.OIDCProvider, string, error) {
		for _, x := range list {
			if x.Name == req.Name {
				return nil, "", fmt.Errorf("%w: an OIDC provider named %q exists", store.ErrConflict, req.Name)
			}
		}
		if err := settings.ValidateProvider(req.OIDCProvider); err != nil {
			return nil, "", badRequest("%v", err)
		}
		return append(list, req.OIDCProvider), fmt.Sprintf("provider %s added (issuer %s, secret %s)", req.Name, req.Issuer, secretState(req.ClientSecret != "")), nil
	})
}

func secretState(set bool) string {
	if set {
		return "set"
	}
	return "not set"
}

func (s *Server) handleUpdateOIDCProvider(w http.ResponseWriter, r *http.Request, p *Principal) {
	name := r.PathValue("name")
	var req oidcRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	req.Name = name
	normalizeProvider(&req.OIDCProvider)
	s.changeProviders(w, r, p, func(list []settings.OIDCProvider) ([]settings.OIDCProvider, string, error) {
		i := slices.IndexFunc(list, func(x settings.OIDCProvider) bool { return x.Name == name })
		if i < 0 {
			return nil, "", fmt.Errorf("%w: no OIDC provider %q", store.ErrNotFound, name)
		}
		old := list[i]
		secret := "unchanged"
		switch {
		case req.ClearSecret:
			req.ClientSecret, secret = "", "removed"
		case req.ClientSecret == "":
			req.ClientSecret = old.ClientSecret
		default:
			secret = "changed"
		}
		if err := settings.ValidateProvider(req.OIDCProvider); err != nil {
			return nil, "", badRequest("%v", err)
		}
		list[i] = req.OIDCProvider
		return list, fmt.Sprintf("provider %s changed (%s; secret %s)", name, providerDiff(old, req.OIDCProvider), secret), nil
	})
}

// providerDiff names the changed fields of a provider (old → new, without
// the secret).
func providerDiff(a, b settings.OIDCProvider) string {
	a.ClientSecret, b.ClientSecret = "", ""
	var ja, jb map[string]any
	ba, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	_ = json.Unmarshal(ba, &ja)
	_ = json.Unmarshal(bb, &jb)
	var out []string
	for _, k := range sortedKeys(jb) {
		if !reflect.DeepEqual(ja[k], jb[k]) {
			out = append(out, fmt.Sprintf("%s: %v → %v", k, ja[k], jb[k]))
		}
	}
	if len(out) == 0 {
		return "no field changes"
	}
	return strings.Join(out, ", ")
}

func (s *Server) handleDeleteOIDCProvider(w http.ResponseWriter, r *http.Request, p *Principal) {
	name := r.PathValue("name")
	s.changeProviders(w, r, p, func(list []settings.OIDCProvider) ([]settings.OIDCProvider, string, error) {
		i := slices.IndexFunc(list, func(x settings.OIDCProvider) bool { return x.Name == name })
		if i < 0 {
			return nil, "", fmt.Errorf("%w: no OIDC provider %q", store.ErrNotFound, name)
		}
		return slices.Delete(list, i, i+1), "provider " + name + " deleted", nil
	})
}

// ---------------------------------------------------------------- tests

// candidate returns the effective configuration with unsaved form values
// (keys of one section) applied, for the test buttons.
func (s *Server) candidate(ctx context.Context, values map[string]json.RawMessage, prefix string) (config.Config, error) {
	stored, err := s.Store.SystemSettings(ctx)
	if err != nil {
		return config.Config{}, err
	}
	raw := rawSettings(stored)
	for k, v := range values {
		d, ok := settings.Lookup(k)
		if !ok || !strings.HasPrefix(k, prefix) {
			return config.Config{}, badRequest("unknown setting %q", k)
		}
		if locked, _ := d.Locked(&s.Cfg); locked {
			continue // the locked value is used
		}
		if v == nil || string(v) == "null" || (d.Kind == settings.Secret && string(v) == `""`) {
			continue // keep the effective value (unsaved secrets are typed, not cleared)
		}
		x, err := d.Decode(v)
		if err != nil {
			return config.Config{}, badRequest("%v", err)
		}
		raw[k] = d.Encode(x)
	}
	eff, _ := settings.Effective(s.Cfg, raw, nil)
	return eff, nil
}

type testResult struct {
	OK      bool           `json:"ok"`
	Message string         `json:"message,omitempty"`
	Error   string         `json:"error,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

func (s *Server) handleTestSyslog(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decodeOptional(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := s.candidate(r.Context(), req.Values, "logging.")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sc := c.Logging.Syslog
	res := testResult{}
	target := sc.Network + "://" + sc.Address
	switch {
	case strings.TrimSpace(sc.Address) == "":
		res.Error = "logging.syslog.address is required"
	default:
		msg := fmt.Sprintf(`{"time":%q,"level":"INFO","msg":"Qtestframe syslog test message","user":%q}`, time.Now().UTC().Format(time.RFC3339), p.User.Email)
		if err := logging.SendTestMessage(sc, []byte(msg)); err != nil {
			res.Error = err.Error()
		} else {
			res.OK, res.Message = true, "test message sent to "+target
		}
	}
	details := target
	if res.Error != "" {
		details += ": " + res.Error
	}
	s.audit(r.Context(), p, "settings.test_syslog", "system_settings", "logging.syslog", details)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTestSettingsEmail(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decodeOptional(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := s.candidate(r.Context(), req.Values, "email.")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := testResult{Details: map[string]any{"to": p.User.Email}}
	e := c.Email
	if e.Host == "" {
		res.Error = "email.host is required"
	} else if errs := c.Validate(); errs != nil && strings.Contains(errs.Error(), "email.") {
		res.Error = errs.Error()
	} else {
		newSMTP := s.sys.newSMTP
		if newSMTP == nil {
			newSMTP = func(c mail.SMTPConfig) mail.Sender { return mail.NewSMTP(c) }
		}
		lang := p.User.Preferences.Language
		who := p.User.DisplayName
		if who == "" {
			who = p.User.Email
		}
		mc := mailContent{Lang: mailLang(lang), Title: tr(lang, "test.subject"), Intro: []string{tr(lang, "test.intro", who)},
			Action: &mailLink{Text: tr(lang, "added.action"), URL: strings.TrimSuffix(c.Server.BaseURL, "/") + "/"}}
		m := &mail.Message{From: mail.Address{Name: e.FromName, Email: e.From}, To: []mail.Address{userAddress(p.User)},
			Subject: tr(lang, "test.subject"), Text: mc.text(), HTML: mc.html()}
		timeout := e.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := newSMTP(smtpConfig(e)).Send(ctx, m); err != nil {
			res.Error = err.Error()
		} else {
			res.OK, res.Message = true, "test e-mail sent to "+p.User.Email
		}
	}
	details := p.User.Email + " via " + e.Host
	if res.Error != "" {
		details += ": " + res.Error
	}
	s.audit(r.Context(), p, "settings.test_email", "system_settings", "email", details)
	writeJSON(w, http.StatusOK, res)
}

// handleTestOIDC fetches the discovery document of an issuer and reports
// what it found.
func (s *Server) handleTestOIDC(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Issuer string `json:"issuer"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	issuer := strings.TrimSpace(req.Issuer)
	u, err := url.Parse(issuer)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		s.fail(w, r, badRequest("the issuer must be an http(s) URL"))
		return
	}
	res := s.discover(r.Context(), issuer)
	details := issuer
	if res.Error != "" {
		details += ": " + res.Error
	}
	s.audit(r.Context(), p, "settings.test_oidc", "system_settings", "auth.oidc", details)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) discover(ctx context.Context, issuer string) testResult {
	res := testResult{Details: map[string]any{}}
	wellKnown := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	res.Details["url"] = wellKnown
	client := &http.Client{Timeout: 10 * time.Second}
	if s.HTTPClient != nil {
		c := *s.HTTPClient
		client = &c
	}
	// a discovery document is never a redirect target we follow blindly
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		res.Error = "the discovery document could not be fetched: " + err.Error()
		return res
	}
	defer resp.Body.Close()
	res.Details["status"] = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		res.Error = fmt.Sprintf("the discovery document answered with HTTP %d", resp.StatusCode)
		return res
	}
	var doc struct {
		Issuer        string   `json:"issuer"`
		Authorization string   `json:"authorization_endpoint"`
		Token         string   `json:"token_endpoint"`
		UserInfo      string   `json:"userinfo_endpoint"`
		JWKS          string   `json:"jwks_uri"`
		Scopes        []string `json:"scopes_supported"`
		Claims        []string `json:"claims_supported"`
		PKCE          []string `json:"code_challenge_methods_supported"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&doc); err != nil {
		res.Error = "the discovery document is not valid JSON: " + err.Error()
		return res
	}
	res.Details["issuer"] = doc.Issuer
	res.Details["authorization_endpoint"] = doc.Authorization
	res.Details["token_endpoint"] = doc.Token
	res.Details["userinfo_endpoint"] = doc.UserInfo
	res.Details["jwks_uri"] = doc.JWKS
	res.Details["scopes_supported"] = doc.Scopes
	res.Details["claims_supported"] = doc.Claims
	res.Details["code_challenge_methods_supported"] = doc.PKCE
	var missing []string
	for k, v := range map[string]string{"authorization_endpoint": doc.Authorization, "token_endpoint": doc.Token, "jwks_uri": doc.JWKS} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	switch {
	case doc.Issuer != issuer:
		res.Error = fmt.Sprintf("the document names the issuer %q, not %q (they must match exactly)", doc.Issuer, issuer)
	case len(missing) > 0:
		res.Error = "the document has no " + strings.Join(missing, ", ")
	default:
		res.OK, res.Message = true, "discovery succeeded"
	}
	return res
}

// handleTestOutbound checks whether the outbound settings (with unsaved
// form values) allow a call to a URL, and fetches it.
func (s *Server) handleTestOutbound(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		URL    string                     `json:"url"`
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		s.fail(w, r, badRequest("the URL must be an absolute http(s) URL"))
		return
	}
	c, err := s.candidate(r.Context(), req.Values, "outbound.")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := testResult{Details: map[string]any{"url": u.String()}}
	if c.Outbound.Proxy != "" {
		res.Details["proxy"] = redactURL(c.Outbound.Proxy)
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: newOutboundTransport(func() config.Outbound { return c.Outbound }),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	hr, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := client.Do(hr)
	switch {
	case errors.Is(err, errOutboundBlocked):
		res.Error = err.Error()
		res.Details["blocked"] = true
	case err != nil:
		res.Error = "allowed by the settings, but the request failed: " + err.Error()
	default:
		resp.Body.Close()
		res.Details["status"] = resp.StatusCode
		res.OK, res.Message = true, fmt.Sprintf("allowed; %s answered with HTTP %d", u.Host, resp.StatusCode)
	}
	details := u.Host
	if res.Error != "" {
		details += ": " + res.Error
	}
	s.audit(r.Context(), p, "settings.test_outbound", "system_settings", "outbound", details)
	writeJSON(w, http.StatusOK, res)
}

// redactURL removes the password of a URL (proxy credentials).
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "xxxxx")
	}
	return u.String()
}

// handleTestSAML fetches the metadata of a SAML provider of the
// configuration file and reports what it found.
func (s *Server) handleTestSAML(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	var sc *config.SAML
	for i := range s.conf().Auth.SAML {
		if x := &s.conf().Auth.SAML[i]; x.Name == req.Name {
			sc = x
		}
	}
	if sc == nil {
		writeError(w, http.StatusNotFound, "not_found", "unknown SAML provider")
		return
	}
	res := testResult{Details: map[string]any{}}
	var body []byte
	var err error
	if sc.IDPMetadataURL != "" {
		res.Details["url"] = sc.IDPMetadataURL
		body, err = s.fetchMetadata(r.Context(), sc.IDPMetadataURL)
	} else {
		res.Details["file"] = sc.IDPMetadataFile
		body, err = os.ReadFile(sc.IDPMetadataFile)
	}
	if err != nil {
		res.Error = "the metadata could not be read: " + err.Error()
	} else {
		var md struct {
			XMLName  xml.Name
			EntityID string `xml:"entityID,attr"`
			IDP      []struct {
				Keys []struct {
					Use string `xml:"use,attr"`
				} `xml:"KeyDescriptor"`
				SSO []struct {
					Binding  string `xml:"Binding,attr"`
					Location string `xml:"Location,attr"`
				} `xml:"SingleSignOnService"`
			} `xml:"IDPSSODescriptor"`
		}
		switch err := xml.Unmarshal(body, &md); {
		case err != nil:
			res.Error = "the metadata is not valid XML: " + err.Error()
		case md.XMLName.Local != "EntityDescriptor" || len(md.IDP) == 0:
			res.Error = "the metadata has no EntityDescriptor with an IDPSSODescriptor"
		default:
			res.Details["entity_id"] = md.EntityID
			res.Details["certificates"] = len(md.IDP[0].Keys)
			var sso []string
			for _, x := range md.IDP[0].SSO {
				sso = append(sso, x.Location)
			}
			res.Details["sso_urls"] = sso
			if len(md.IDP[0].Keys) == 0 {
				res.Error = "the metadata has no signing certificate"
			} else {
				res.OK, res.Message = true, "metadata read: "+md.EntityID
			}
		}
	}
	details := sc.Name
	if res.Error != "" {
		details += ": " + res.Error
	}
	s.audit(r.Context(), p, "settings.test_saml", "system_settings", "auth.saml", details)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) fetchMetadata(ctx context.Context, u string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// decodeOptional is decode that accepts an empty body.
func decodeOptional(r *http.Request, v any) error {
	body, err := readBody(r)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return badRequest("invalid JSON: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------- export

const redacted = "REDACTED"

// handleExportSystemSettings returns the effective configuration as YAML
// (restart settings with their stored values), secrets redacted.
func (s *Server) handleExportSystemSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	stored, err := s.Store.SystemSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, _ := settings.Effective(s.Cfg, rawSettings(stored), nil)
	out, err := ExportYAML(c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "settings.export", "system_settings", "", fmt.Sprintf("%d bytes", len(out)))
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="qtestframe-config.yaml"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// ExportYAML renders c as a configuration file with secrets redacted.
func ExportYAML(c config.Config) ([]byte, error) {
	if c.Database.DSN != "" && c.Database.Driver != "sqlite" {
		c.Database.DSN = redacted
	}
	if c.Bootstrap.AdminPassword != "" {
		c.Bootstrap.AdminPassword = redacted
	}
	oidc := make([]config.OIDC, len(c.Auth.OIDC))
	for i, o := range c.Auth.OIDC {
		if o.ClientSecret != "" {
			o.ClientSecret = redacted
		}
		oidc[i] = o
	}
	c.Auth.OIDC = oidc
	body, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	head := "# Qtestframe effective configuration, exported " + time.Now().UTC().Format(time.RFC3339) + ".\n" +
		"# Values changed in Administration → System settings are included; secrets are\n" +
		"# redacted (" + redacted + ") and the e-mail password is never exported.\n"
	return append([]byte(head), body...), nil
}
