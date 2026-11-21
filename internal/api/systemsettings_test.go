package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/logging"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/settings"
	"github.com/rom/qtestframe/internal/store"
)

type settingsResp struct {
	Settings []settingView `json:"settings"`
	OIDC     struct {
		Providers []map[string]any `json:"providers"`
		Source    string           `json:"source"`
		Locked    bool             `json:"locked"`
	} `json:"oidc"`
	RestartPending bool     `json:"restart_pending"`
	LoadError      string   `json:"load_error"`
	Locked         []string `json:"locked_patterns"`
	Sections       []string `json:"sections"`
}

func (r settingsResp) get(t *testing.T, key string) settingView {
	t.Helper()
	for _, s := range r.Settings {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("setting %s not listed", key)
	return settingView{}
}

func getSettings(t *testing.T, c *client) settingsResp {
	t.Helper()
	var r settingsResp
	c.call("GET", "/api/v1/admin/settings", nil, 200, &r)
	return r
}

func putSettings(c *client, values map[string]any, want int) settingsResp {
	var r settingsResp
	c.call("PUT", "/api/v1/admin/settings", map[string]any{"values": values}, want, &r)
	return r
}

func auditDetails(t *testing.T, e *env, action string) []string {
	t.Helper()
	es, err := e.st.ListAudit(context.Background(), e.tenant.ID, 200)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range es {
		if a.Action == action {
			out = append(out, a.Details)
		}
	}
	return out
}

func TestSystemSettingsAccess(t *testing.T) {
	e := newEnv(t)
	e.member("ta@example.com", rbac.RoleAdmin)
	ta := e.login("ta@example.com")
	for _, rq := range []struct{ m, p string }{
		{"GET", "/api/v1/admin/settings"}, {"PUT", "/api/v1/admin/settings"}, {"GET", "/api/v1/admin/settings/export"},
		{"DELETE", "/api/v1/admin/settings/logging.level"}, {"POST", "/api/v1/admin/settings/oidc"},
		{"PUT", "/api/v1/admin/settings/oidc/x"}, {"DELETE", "/api/v1/admin/settings/oidc/x"},
		{"POST", "/api/v1/admin/settings/test/oidc"}, {"POST", "/api/v1/admin/settings/test/syslog"}, {"POST", "/api/v1/admin/settings/test/email"},
	} {
		ta.call(rq.m, rq.p, map[string]any{}, 403, nil)
	}
	// API tokens of super admins cannot change system settings either
	a := e.login("admin@example.com")
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "t", "role": "admin"}, 201)
	b := e.anon()
	b.bearer = tok["token"].(string)
	b.call("GET", "/api/v1/admin/settings", nil, 403, nil)
	r := getSettings(t, a)
	if len(r.Sections) < 5 || r.get(t, "logging.level").Source != "default" || r.get(t, "server.base_url").Value != "http://example.test" {
		t.Errorf("view: %+v", r.get(t, "logging.level"))
	}
	if l := r.get(t, "server.listen"); !l.Locked || l.LockReason != "readonly" || l.Apply != "readonly" {
		t.Errorf("listen: %+v", l)
	}
	if mx := r.get(t, "files.max_upload_mb"); mx.Min == nil || *mx.Max != 1024 {
		t.Errorf("bounds: %+v", mx)
	}
}

func TestSystemSettingsRuntimeChange(t *testing.T) {
	e := newEnv(t)
	var buf syncBuffer
	logs, err := logging.NewManager(config.Logging{Level: "info", Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	e.srv.Logs, e.srv.Log = logs, logs.Logger()
	a := e.login("admin@example.com")

	r := putSettings(a, map[string]any{"logging.level": "debug", "logging.format": "json", "files.max_upload_mb": 7, "auth.session_ttl": "2h"}, 200)
	lv := r.get(t, "logging.level")
	if lv.Value != "debug" || lv.Source != "database" || lv.FileValue != "info" || lv.FileSource != "default" || !lv.Stored || lv.UpdatedBy != "admin@example.com" {
		t.Errorf("level: %+v", lv)
	}
	if e.srv.conf().Logging.Level != "debug" || e.srv.maxUpload() != 7<<20 || logs.Config().Format != "json" {
		t.Errorf("not applied: %+v", e.srv.conf().Logging)
	}
	if ttl := r.get(t, "auth.session_ttl"); ttl.Value != "2h" {
		t.Errorf("ttl %v", ttl.Value)
	}
	e.srv.Log.Debug("after the change")
	if !strings.Contains(buf.String(), `"msg":"after the change"`) {
		t.Errorf("logger not reconfigured: %s", buf.String())
	}
	// a new session lasts the new lifetime
	b := e.login("admin@example.com")
	var sess struct{ Items []model.SessionInfo }
	b.call("GET", "/api/v1/auth/sessions", nil, 200, &sess)
	var newest time.Time
	for _, s := range sess.Items {
		if s.Current {
			newest = s.ExpiresAt
		}
	}
	if d := time.Until(newest); d > 2*time.Hour+time.Minute || d < 2*time.Hour-time.Minute {
		t.Errorf("session lifetime %v", d)
	}
	if got := auditDetails(t, e, "settings.update"); len(got) != 1 || !strings.Contains(got[0], "logging.level: info → debug") || !strings.Contains(got[0], "files.max_upload_mb: 25 → 7") {
		t.Errorf("audit: %v", got)
	}
	// meta follows the setting
	if m := a.m("GET", "/api/v1/meta", nil, 200); m["max_upload_mb"] != float64(7) {
		t.Errorf("meta %v", m["max_upload_mb"])
	}

	// reset to the file value
	var rr settingsResp
	a.call("DELETE", "/api/v1/admin/settings/logging.level", nil, 200, &rr)
	if lv := rr.get(t, "logging.level"); lv.Value != "info" || lv.Source != "default" || lv.Stored {
		t.Errorf("reset: %+v", lv)
	}
	a.call("DELETE", "/api/v1/admin/settings/nope", nil, 404, nil)
	// null also resets; everything reset = back to the plain configuration
	putSettings(a, map[string]any{"logging.format": nil, "files.max_upload_mb": nil, "auth.session_ttl": nil}, 200)
	if e.srv.sys.live.Load() != nil {
		t.Error("live config kept without overrides")
	}
	if got := auditDetails(t, e, "settings.update"); !strings.Contains(strings.Join(got, "|"), "(reset to the file value)") {
		t.Errorf("reset audit: %v", got)
	}
	// nothing to change
	putSettings(a, map[string]any{}, 200)
}

type syncBuffer struct {
	mu  chan struct{}
	buf bytes.Buffer
}

func (b *syncBuffer) lock() {
	if b.mu == nil {
		b.mu = make(chan struct{}, 1)
	}
	b.mu <- struct{}{}
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.lock()
	defer func() { <-b.mu }()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.lock()
	defer func() { <-b.mu }()
	return b.buf.String()
}

func TestSystemSettingsValidation(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	for name, v := range map[string]map[string]any{
		"enum":       {"logging.level": "loud"},
		"range":      {"files.max_upload_mb": 5000},
		"type":       {"files.max_upload_mb": "ten"},
		"bool":       {"auth.local": "yes"},
		"duration":   {"auth.session_ttl": "forever"},
		"dur range":  {"auth.session_ttl": "1s"},
		"unknown":    {"no.such": 1},
		"url":        {"server.base_url": "ftp://x"},
		"tag":        {"logging.syslog.tag": "with space"},
		"config":     {"logging.syslog.enabled": true, "logging.syslog.address": ""},
		"no sso":     {"auth.local": false},
		"oidc via":   {"auth.oidc": []any{}},
		"required":   {"logging.syslog.tag": ""},
		"email from": {"email.host": "smtp.example.test", "email.from": "Name <x@example.test>"},
		"one line":   {"email.from_name": "a\nb"},
	} {
		res := a.raw("PUT", "/api/v1/admin/settings", map[string]any{"values": v})
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Errorf("%s: %d %s", name, res.StatusCode, body)
		}
	}
	a.call("PUT", "/api/v1/admin/settings", "{", 400, nil)
	// read-only settings
	if res := a.raw("PUT", "/api/v1/admin/settings", map[string]any{"values": map[string]any{"server.listen": ":1"}}); res.StatusCode != 409 {
		t.Errorf("readonly: %d", res.StatusCode)
	}
	// nothing was stored
	if st, _ := e.st.SystemSettings(context.Background()); len(st) != 0 {
		t.Errorf("stored: %v", st)
	}
}

func TestSystemSettingsLocked(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Meta.EnvKeys = map[string]string{"logging.level": "QTF_LOG_LEVEL"}
		c.Meta.FileKeys = map[string]bool{"logging.format": true}
		c.SystemSettings.Locked = []string{"email.*", "auth.oidc"}
	})
	a := e.login("admin@example.com")
	for _, k := range []string{"logging.level", "email.host"} {
		res := a.raw("PUT", "/api/v1/admin/settings", map[string]any{"values": map[string]any{k: "x"}})
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 409 || !strings.Contains(string(body), "locked") {
			t.Errorf("%s: %d %s", k, res.StatusCode, body)
		}
	}
	a.call("POST", "/api/v1/admin/settings/oidc", map[string]any{"name": "x", "issuer": "https://i", "client_id": "c"}, 409, nil)
	a.call("DELETE", "/api/v1/admin/settings/email.host", nil, 409, nil)
	r := getSettings(t, a)
	if lv := r.get(t, "logging.level"); !lv.Locked || lv.LockReason != "environment" || lv.Env != "QTF_LOG_LEVEL" || lv.Source != "environment" {
		t.Errorf("env lock: %+v", lv)
	}
	if lf := r.get(t, "logging.format"); lf.Locked || lf.Source != "file" {
		t.Errorf("file: %+v", lf)
	}
	if lh := r.get(t, "email.host"); !lh.Locked || lh.LockReason != "config" {
		t.Errorf("config lock: %+v", lh)
	}
	if !r.OIDC.Locked || len(r.Locked) != 2 {
		t.Errorf("oidc lock: %+v", r)
	}
	// a stored override of a locked setting is ignored
	if err := e.st.SaveSystemSettings(context.Background(), map[string]json.RawMessage{"logging.level": json.RawMessage(`"error"`)}, "x"); err != nil {
		t.Fatal(err)
	}
	r = getSettings(t, a)
	if lv := r.get(t, "logging.level"); !lv.Ignored || lv.Value != "info" || lv.Source != "environment" {
		t.Errorf("ignored: %+v", lv)
	}
}

func TestSystemSettingsEmail(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	if e.srv.emailOn() {
		t.Fatal("mail on")
	}
	r := putSettings(a, map[string]any{"email.host": "smtp.example.test", "email.from": "qtf@example.test", "email.username": "u", "email.password": "s3cret-Pw"}, 200)
	if !e.srv.emailOn() {
		t.Fatal("mail not turned on")
	}
	pw := r.get(t, "email.password")
	if pw.Value != nil || pw.HasSecret == nil || !*pw.HasSecret || *pw.FileSecret || pw.Source != "database" {
		t.Errorf("password view: %+v", pw)
	}
	res := a.raw("GET", "/api/v1/admin/settings", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	exp := a.raw("GET", "/api/v1/admin/settings/export", nil)
	yml, _ := io.ReadAll(exp.Body)
	exp.Body.Close()
	audit := strings.Join(auditDetails(t, e, "settings.update"), "|")
	for name, s := range map[string]string{"GET": string(body), "export": string(yml), "audit": audit} {
		if strings.Contains(s, "s3cret-Pw") {
			t.Errorf("%s leaks the password", name)
		}
	}
	if !strings.Contains(audit, "email.password: not set → set (changed)") {
		t.Errorf("audit: %s", audit)
	}
	if exp.Header.Get("Content-Type") != "application/yaml; charset=utf-8" || !strings.Contains(string(yml), "host: smtp.example.test") {
		t.Errorf("export: %s", yml)
	}
	smtp, ok := e.srv.mailer().(*mail.SMTP)
	if !ok || smtp.Cfg.Password != "s3cret-Pw" || smtp.Cfg.Host != "smtp.example.test" {
		t.Fatalf("sender: %#v", e.srv.mailer())
	}

	// "Send test e-mail" with unsaved values and the stored password
	var got mail.SMTPConfig
	f := &mail.Fake{}
	e.srv.sys.newSMTP = func(c mail.SMTPConfig) mail.Sender { got = c; return f }
	var tr testResult
	a.call("POST", "/api/v1/admin/settings/test/email", map[string]any{"values": map[string]any{"email.port": 2525, "email.password": ""}}, 200, &tr)
	if !tr.OK || got.Port != 2525 || got.Password != "s3cret-Pw" || len(f.Messages()) != 1 || f.Messages()[0].To[0].Email != "admin@example.com" {
		t.Errorf("test mail: %+v %+v", tr, got)
	}
	f.Err = errors.New("535 authentication failed")
	a.call("POST", "/api/v1/admin/settings/test/email", nil, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "535") {
		t.Errorf("failing test mail: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/email", map[string]any{"values": map[string]any{"logging.level": "x"}}, 400, nil)
	a.call("POST", "/api/v1/admin/settings/test/email", map[string]any{"values": map[string]any{"email.port": 0}}, 400, nil)
	a.call("POST", "/api/v1/admin/settings/test/email", map[string]any{"values": map[string]any{"email.host": ""}}, 200, &tr)
	if tr.OK || tr.Error == "" {
		t.Errorf("no host: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/email", map[string]any{"values": map[string]any{"email.from": "bad"}}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "email.from") {
		t.Errorf("bad from: %+v", tr)
	}

	// an empty secret removes it; turning e-mail off removes the sender
	putSettings(a, map[string]any{"email.password": "", "email.username": nil}, 200)
	if smtp := e.srv.mailer().(*mail.SMTP); smtp.Cfg.Password != "" {
		t.Error("password kept")
	}
	putSettings(a, map[string]any{"email.enabled": false}, 200)
	if e.srv.emailOn() {
		t.Error("mail still on")
	}
}

func TestSystemSettingsSyslog(t *testing.T) {
	e := newEnv(t)
	var buf syncBuffer
	logs, err := logging.NewManager(config.Logging{Level: "info", Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	e.srv.Logs, e.srv.Log = logs, logs.Logger()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	read := func() string {
		_ = pc.SetReadDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 4096)
		n, _, err := pc.ReadFrom(b)
		if err != nil {
			t.Fatalf("no syslog message: %v", err)
		}
		return string(b[:n])
	}
	a := e.login("admin@example.com")
	addr := pc.LocalAddr().String()
	var tr testResult
	a.call("POST", "/api/v1/admin/settings/test/syslog", map[string]any{"values": map[string]any{"logging.syslog.network": "udp", "logging.syslog.address": addr, "logging.syslog.tag": "qtftest"}}, 200, &tr)
	if !tr.OK {
		t.Fatalf("test: %+v", tr)
	}
	if m := read(); !strings.Contains(m, "qtftest") || !strings.Contains(m, "syslog test message") || !strings.HasPrefix(m, "<134>1 ") {
		t.Errorf("message: %s", m)
	}
	a.call("POST", "/api/v1/admin/settings/test/syslog", map[string]any{}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "address") {
		t.Errorf("no address: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/syslog", map[string]any{"values": map[string]any{"logging.syslog.address": "127.0.0.1:1", "logging.syslog.network": "tcp"}}, 200, &tr)
	if tr.OK || tr.Error == "" {
		t.Errorf("refused: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/syslog", map[string]any{"values": map[string]any{"email.host": "x"}}, 400, nil)

	// enable forwarding: the server's own log lines arrive
	putSettings(a, map[string]any{"logging.syslog.enabled": true, "logging.syslog.network": "udp", "logging.syslog.address": addr, "logging.syslog.facility": "local3"}, 200)
	e.srv.Log.Info("forwarded line")
	for i := 0; i < 5; i++ {
		if m := read(); strings.Contains(m, "forwarded line") {
			if !strings.HasPrefix(m, "<158>1 ") { // local3.info
				t.Errorf("priority: %s", m)
			}
			break
		} else if i == 4 {
			t.Errorf("forwarded line not received, last %s", m)
		}
	}
	if !logs.Config().Syslog.Enabled {
		t.Error("manager not reconfigured")
	}
	if got := auditDetails(t, e, "settings.test_syslog"); len(got) != 3 {
		t.Errorf("test audit: %v", got)
	}
}

func discoveryServer(t *testing.T, issuer *string, status int) *httptest.Server {
	t.Helper()
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" || status != 200 {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": *issuer, "authorization_endpoint": *issuer + "/auth", "token_endpoint": *issuer + "/token",
			"jwks_uri": *issuer + "/jwks", "scopes_supported": []string{"openid", "email"},
		})
	}))
	t.Cleanup(hs.Close)
	return hs
}

func TestSystemSettingsOIDC(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Auth.OIDC = []config.OIDC{{Name: "file", Issuer: "https://idp.example.test", ClientID: "fc", ClientSecret: "file-secret"}}
		c.Meta.FileKeys = map[string]bool{"auth.oidc": true}
	})
	a := e.login("admin@example.com")
	r := getSettings(t, a)
	if len(r.OIDC.Providers) != 1 || r.OIDC.Providers[0]["has_secret"] != true || r.OIDC.Providers[0]["secret_source"] != "file" || r.OIDC.Source != "file" || r.OIDC.Providers[0]["in_file"] != true {
		t.Fatalf("file providers: %+v", r.OIDC)
	}
	if _, ok := r.OIDC.Providers[0]["client_secret"]; ok {
		t.Error("secret returned")
	}
	if cb := r.OIDC.Providers[0]["callback_url"]; cb != "http://example.test/api/v1/auth/oidc/file/callback" {
		t.Errorf("callback %v", cb)
	}
	before, _ := e.srv.oidcProvider("file")

	// add a provider: the file provider keeps its secret (not copied)
	var rr settingsResp
	a.call("POST", "/api/v1/admin/settings/oidc", map[string]any{"name": "corp", "display_name": "Corporate SSO", "enabled": true,
		"issuer": "https://login.example.test", "client_id": "cid", "client_secret": "corp-secret", "scopes": []string{"openid", " email ", "email"},
		"auto_provision": true, "default_role": "tester", "role_mapping": map[string]string{"qa": "test_lead"}}, 200, &rr)
	if len(rr.OIDC.Providers) != 2 || rr.OIDC.Source != "database" {
		t.Fatalf("after add: %+v", rr.OIDC)
	}
	stored, _ := e.st.SystemSettings(context.Background())
	if v := string(stored["auth.oidc"].Value); strings.Contains(v, "file-secret") || !strings.Contains(v, "corp-secret") {
		t.Errorf("stored: %s", v)
	}
	p, ok := e.srv.oidcProvider("corp")
	if !ok || p.Cfg.ClientSecret != "corp-secret" || p.Cfg.DefaultRole != "tester" || len(p.Cfg.Scopes) != 2 || p.RedirectURL != "http://example.test/api/v1/auth/oidc/corp/callback" {
		t.Fatalf("provider: %+v", p)
	}
	if after, _ := e.srv.oidcProvider("file"); after != before || after.Cfg.ClientSecret != "file-secret" {
		t.Error("unchanged provider rebuilt or lost its secret")
	}
	var provs struct{ OIDC []providerInfo }
	e.anon().call("GET", "/api/v1/auth/providers", nil, 200, &provs)
	if len(provs.OIDC) != 2 || provs.OIDC[1].DisplayName != "Corporate SSO" {
		t.Errorf("sign-in page: %+v", provs)
	}
	a.call("POST", "/api/v1/admin/settings/oidc", map[string]any{"name": "corp", "issuer": "https://x", "client_id": "c"}, 409, nil)
	a.call("POST", "/api/v1/admin/settings/oidc", map[string]any{"name": "Bad Name", "issuer": "https://x", "client_id": "c"}, 400, nil)
	a.call("POST", "/api/v1/admin/settings/oidc", map[string]any{"name": "nocid", "issuer": "x"}, 400, nil)
	a.call("POST", "/api/v1/admin/settings/oidc", "{", 400, nil)

	// edit without a secret keeps it; disable hides it from the sign-in page
	a.call("PUT", "/api/v1/admin/settings/oidc/corp", map[string]any{"display_name": "Corp", "enabled": false, "issuer": "https://login.example.test", "client_id": "cid2"}, 200, &rr)
	if _, ok := e.srv.oidcProvider("corp"); ok {
		t.Error("disabled provider still active")
	}
	e.anon().call("GET", "/api/v1/auth/providers", nil, 200, &provs)
	if len(provs.OIDC) != 1 {
		t.Errorf("disabled provider listed: %+v", provs)
	}
	e.anon().call("GET", "/api/v1/auth/oidc/corp/login", nil, 404, nil)
	a.call("PUT", "/api/v1/admin/settings/oidc/corp", map[string]any{"enabled": true, "issuer": "https://login.example.test", "client_id": "cid2"}, 200, &rr)
	if p, _ := e.srv.oidcProvider("corp"); p == nil || p.Cfg.ClientSecret != "corp-secret" || p.Cfg.ClientID != "cid2" {
		t.Errorf("secret not kept: %+v", p)
	}
	// clearing the stored secret of the file provider falls back to the file
	a.call("PUT", "/api/v1/admin/settings/oidc/file", map[string]any{"enabled": true, "issuer": "https://idp.example.test", "client_id": "fc", "client_secret": "db-secret"}, 200, &rr)
	if p, _ := e.srv.oidcProvider("file"); p.Cfg.ClientSecret != "db-secret" {
		t.Errorf("db secret: %v", p.Cfg.ClientSecret)
	}
	a.call("PUT", "/api/v1/admin/settings/oidc/file", map[string]any{"enabled": true, "issuer": "https://idp.example.test", "client_id": "fc", "clear_secret": true}, 200, &rr)
	if p, _ := e.srv.oidcProvider("file"); p.Cfg.ClientSecret != "file-secret" {
		t.Errorf("fallback: %v", p.Cfg.ClientSecret)
	}
	a.call("PUT", "/api/v1/admin/settings/oidc/none", map[string]any{"issuer": "https://x", "client_id": "c"}, 404, nil)
	a.call("PUT", "/api/v1/admin/settings/oidc/corp", map[string]any{"issuer": "nope", "client_id": "c"}, 400, nil)
	a.call("PUT", "/api/v1/admin/settings/oidc/corp", "[", 400, nil)
	audit := strings.Join(auditDetails(t, e, "settings.update"), "|")
	if strings.Contains(audit, "secret\":") || strings.Contains(audit, "corp-secret") || !strings.Contains(audit, "provider corp added") || !strings.Contains(audit, "client_id: cid → cid2") {
		t.Errorf("audit: %s", audit)
	}

	// delete, then reset to the file's providers
	a.call("DELETE", "/api/v1/admin/settings/oidc/corp", nil, 200, &rr)
	a.call("DELETE", "/api/v1/admin/settings/oidc/corp", nil, 404, nil)
	if _, ok := e.srv.oidcProvider("corp"); ok || len(rr.OIDC.Providers) != 1 {
		t.Error("not deleted")
	}
	a.call("DELETE", "/api/v1/admin/settings/auth.oidc", nil, 200, &rr)
	if rr.OIDC.Source != "file" {
		t.Errorf("reset: %+v", rr.OIDC)
	}
	// only enabled SSO lets local sign-in go
	putSettings(a, map[string]any{"auth.local": false}, 200)
	a.call("POST", "/api/v1/auth/login", map[string]any{"email": "admin@example.com", "password": testPassword}, 403, nil)
	putSettings(a, map[string]any{"auth.local": nil}, 200)
}

func TestSystemSettingsDiscovery(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	var issuer string
	hs := discoveryServer(t, &issuer, 200)
	issuer = hs.URL
	var tr testResult
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": hs.URL}, 200, &tr)
	if !tr.OK || tr.Details["token_endpoint"] != hs.URL+"/token" {
		t.Errorf("discovery: %+v", tr)
	}
	issuer = "https://other.example.test"
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": hs.URL}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "must match") {
		t.Errorf("mismatch: %+v", tr)
	}
	missing := discoveryServer(t, &issuer, 404)
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": missing.URL}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "HTTP 404") {
		t.Errorf("404: %+v", tr)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer bad.Close()
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": bad.URL}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "not valid JSON") {
		t.Errorf("html: %+v", tr)
	}
	partial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host})
	}))
	defer partial.Close()
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": partial.URL}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "authorization_endpoint") {
		t.Errorf("partial: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": "http://127.0.0.1:1"}, 200, &tr)
	if tr.OK || !strings.Contains(tr.Error, "could not be fetched") {
		t.Errorf("unreachable: %+v", tr)
	}
	a.call("POST", "/api/v1/admin/settings/test/oidc", map[string]any{"issuer": "not a url"}, 400, nil)
	a.call("POST", "/api/v1/admin/settings/test/oidc", nil, 400, nil)
}

func TestSystemSettingsSecurity(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"auth.password_min_length": 14, "auth.login_max_attempts": 2, "auth.login_lockout": "10m"}, 200)
	var provs map[string]any
	e.anon().call("GET", "/api/v1/auth/providers", nil, 200, &provs)
	if provs["password_min_length"] != float64(14) {
		t.Errorf("providers: %v", provs)
	}
	res := a.raw("POST", "/api/v1/auth/password", map[string]any{"current_password": testPassword, "new_password": "abcdefgh-123"})
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 400 || !strings.Contains(string(body), "at least 14 characters") {
		t.Errorf("policy: %d %s", res.StatusCode, body)
	}
	// login lockout after two failures
	c := e.anon()
	for i := 0; i < 2; i++ {
		c.call("POST", "/api/v1/auth/login", map[string]any{"email": "nobody@example.com", "password": "wrong"}, 401, nil)
	}
	c.call("POST", "/api/v1/auth/login", map[string]any{"email": "nobody@example.com", "password": "wrong"}, 429, nil)
}

func TestSystemSettingsRestart(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	r := putSettings(a, map[string]any{"runner.web.max_parallel": 5, "runner.timeout": "90s"}, 200)
	mp := r.get(t, "runner.web.max_parallel")
	if mp.Value != float64(5) || mp.RunningValue != float64(2) || !mp.PendingRestart || !r.RestartPending || mp.Apply != "restart" {
		t.Errorf("restart: %+v", mp)
	}
	if e.srv.maxParallel() != 2 {
		t.Error("restart setting applied at runtime")
	}
	// the next start uses the stored values
	eff, err := EffectiveConfig(context.Background(), e.st, e.cfg)
	if err != nil || eff.Runner.Web.MaxParallel != 5 || eff.Runner.Timeout != 90*time.Second {
		t.Errorf("effective: %v %+v", err, eff.Runner)
	}
	srv, err := New(e.st, e.cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	if err != nil || srv.maxParallel() != 5 {
		t.Errorf("new server: %v %d", err, srv.maxParallel())
	}
	exp := a.raw("GET", "/api/v1/admin/settings/export", nil)
	yml, _ := io.ReadAll(exp.Body)
	exp.Body.Close()
	if !strings.Contains(string(yml), "max_parallel: 5") || !strings.Contains(string(yml), "timeout: 1m30s") {
		t.Errorf("export: %s", yml)
	}
}

func TestSystemSettingsInvalidStored(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// valid on their own, invalid together (syslog without an address)
	if err := e.st.SaveSystemSettings(ctx, map[string]json.RawMessage{"logging.syslog.enabled": json.RawMessage(`true`), "logging.level": json.RawMessage(`"debug"`)}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := EffectiveConfig(ctx, e.st, e.cfg); err == nil {
		t.Error("invalid stored settings accepted")
	}
	srv, err := New(e.st, e.cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if srv.conf().Logging.Level != "info" || srv.sys.loadErr == "" {
		t.Errorf("invalid settings applied: %s %q", srv.conf().Logging.Level, srv.sys.loadErr)
	}
	if err := srv.ReloadSettings(ctx); err == nil {
		t.Error("reload accepted invalid settings")
	}
	// after a fix, reload applies them
	if err := e.st.SaveSystemSettings(ctx, map[string]json.RawMessage{"logging.syslog.enabled": nil}, "x"); err != nil {
		t.Fatal(err)
	}
	if err := srv.ReloadSettings(ctx); err != nil || srv.conf().Logging.Level != "debug" {
		t.Errorf("reload: %v %s", err, srv.conf().Logging.Level)
	}
	// unknown keys are ignored
	if err := e.st.SaveSystemSettings(ctx, map[string]json.RawMessage{"gone.key": json.RawMessage(`1`)}, "x"); err != nil {
		t.Fatal(err)
	}
	a := e.login("admin@example.com")
	getSettings(t, a)
}

func TestExportYAMLRedacts(t *testing.T) {
	c := config.Default()
	c.Database.Driver, c.Database.DSN = "postgres", "postgres://u:pw@db/x"
	c.Bootstrap.AdminPassword = "boot-pw"
	c.Auth.OIDC = []config.OIDC{{Name: "a", Issuer: "https://i", ClientID: "c", ClientSecret: "oidc-pw"}}
	c.Email.Password = "mail-pw"
	b, err := ExportYAML(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"pw@db", "boot-pw", "oidc-pw", "mail-pw"} {
		if strings.Contains(string(b), s) {
			t.Errorf("leaks %s", s)
		}
	}
	var back config.Config
	if err := yamlUnmarshalStrict(b, &back); err != nil {
		t.Fatalf("export does not load: %v\n%s", err, b)
	}
	if c.Auth.OIDC[0].ClientSecret != "oidc-pw" {
		t.Error("input modified")
	}
}

func TestSystemSettingsRestoreReload(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"logging.level": "warn"}, 200)
	if err := e.st.SaveSystemSettings(context.Background(), map[string]json.RawMessage{"logging.level": json.RawMessage(`"error"`)}, "restore"); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.ReloadSettings(context.Background()); err != nil || e.srv.conf().Logging.Level != "error" {
		t.Errorf("reload: %v", err)
	}
	if _, ok := settings.Lookup("logging.level"); !ok {
		t.Error("lookup")
	}
	_ = store.TrashRetention
}

func yamlUnmarshalStrict(b []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(v)
}
