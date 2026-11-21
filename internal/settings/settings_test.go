package settings

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

func def(t *testing.T, key string) *Def {
	t.Helper()
	d, ok := Lookup(key)
	if !ok {
		t.Fatalf("no setting %s", key)
	}
	return d
}

func TestRegistry(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Registry {
		if seen[d.Key] {
			t.Errorf("duplicate %s", d.Key)
		}
		seen[d.Key] = true
		found := false
		for _, s := range Sections {
			found = found || s == d.Section
		}
		if !found {
			t.Errorf("%s: unknown section %s", d.Key, d.Section)
		}
		if d.Apply != ReadOnly && d.set == nil {
			t.Errorf("%s: editable without setter", d.Key)
		}
		c := config.Default()
		_ = d.Get(&c)
		_ = d.Display(&c)
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("lookup of unknown key")
	}
}

func TestDecode(t *testing.T) {
	ok := map[string]string{
		"logging.level":          `"debug"`,
		"files.max_upload_mb":    `100`,
		"auth.local":             `false`,
		"auth.session_ttl":       `"8h"`,
		"server.base_url":        `"https://qtf.example.com"`,
		"email.password":         `"pw"`,
		"logging.syslog.tag":     `"qtf.app-1"`,
		"email.from_name":        `""`,
		"auth.oidc":              `[{"name":"a","issuer":"https://i","client_id":"c"}]`,
		"logging.syslog.ca_file": `"x"`,
	}
	for k, raw := range ok {
		v, err := def(t, k).Decode(json.RawMessage(raw))
		if k == "logging.syslog.ca_file" {
			if err == nil {
				t.Error("read-only setting decoded")
			}
			continue
		}
		if err != nil {
			t.Errorf("%s %s: %v", k, raw, err)
		}
		if k == "auth.session_ttl" && v != 8*time.Hour {
			t.Errorf("ttl %v", v)
		}
	}
	bad := map[string][]string{
		"logging.level":       {`"loud"`, `1`},
		"files.max_upload_mb": {`0`, `2000`, `"x"`, `1.5`},
		"auth.local":          {`"true"`},
		"auth.session_ttl":    {`"x"`, `12`, `"1s"`, `"10000h"`},
		"server.base_url":     {`"ftp://x"`, `"https://x/"`, `"https://x?a=1"`, `"x"`, `5`},
		"logging.syslog.tag":  {`""`, `"a b"`},
		"email.host":          {`"a\nb"`, `"` + strings.Repeat("x", 2001) + `"`},
		"auth.oidc":           {`{}`, `[{"name":"A"}]`, `[{"name":"a","issuer":"https://i","client_id":"c"},{"name":"a","issuer":"https://i","client_id":"c"}]`},
	}
	for k, raws := range bad {
		for _, raw := range raws {
			if _, err := def(t, k).Decode(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s %s accepted: %v", k, raw, err)
			}
		}
	}
}

func TestEncodeAndDurations(t *testing.T) {
	d := def(t, "auth.session_ttl")
	if string(d.Encode(90*time.Minute)) != `"1h30m0s"` || string(def(t, "auth.local").Encode(true)) != "true" {
		t.Error("encode")
	}
	for in, want := range map[time.Duration]string{12 * time.Hour: "12h", 90 * time.Minute: "1h30m", 45 * time.Second: "45s", 5 * time.Minute: "5m", 1500 * time.Millisecond: "1.5s"} {
		if got := ShortDuration(in); got != want {
			t.Errorf("%v: %s, want %s", in, got, want)
		}
	}
}

func TestLockedAndSource(t *testing.T) {
	c := config.Default()
	c.Meta.EnvKeys = map[string]string{"logging.level": "QTF_LOG_LEVEL"}
	c.Meta.FileKeys = map[string]bool{"logging.format": true}
	c.SystemSettings.Locked = []string{"email.*", "auth.local"}
	cases := map[string]struct {
		locked      bool
		why, source string
	}{
		"logging.level":    {true, LockEnvironment, SourceEnvironment},
		"logging.format":   {false, "", SourceFile},
		"email.host":       {true, LockConfig, SourceDefault},
		"auth.local":       {true, LockConfig, SourceDefault},
		"server.listen":    {true, LockReadOnly, SourceDefault},
		"auth.session_ttl": {false, "", SourceDefault},
	}
	for k, want := range cases {
		d := def(t, k)
		l, why := d.Locked(&c)
		if l != want.locked || why != want.why || d.Source(&c) != want.source {
			t.Errorf("%s: %v %s %s", k, l, why, d.Source(&c))
		}
	}
}

func TestEffective(t *testing.T) {
	base := config.Default()
	base.Meta.EnvKeys = map[string]string{"logging.format": "QTF_LOG_FORMAT"}
	stored := map[string]json.RawMessage{
		"logging.level":       json.RawMessage(`"warn"`),
		"logging.format":      json.RawMessage(`"json"`), // locked by the environment
		"unknown.key":         json.RawMessage(`1`),
		"server.listen":       json.RawMessage(`":1"`), // read-only
		"files.max_upload_mb": json.RawMessage(`"bad"`),
		"runner.enabled":      json.RawMessage(`true`),
		"email.enabled":       json.RawMessage(`false`),
	}
	eff, ignored := Effective(base, stored, nil)
	if eff.Logging.Level != "warn" || eff.Logging.Format != "text" || eff.Server.Listen != ":8080" || !eff.Runner.Enabled || eff.Email.Enabled == nil || *eff.Email.Enabled {
		t.Errorf("effective: %+v", eff.Logging)
	}
	if strings.Join(ignored, ",") != "files.max_upload_mb,logging.format,server.listen,unknown.key" {
		t.Errorf("ignored %v", ignored)
	}
	if base.Logging.Level != "info" {
		t.Error("base modified")
	}
	// only runtime settings
	eff, _ = Effective(base, stored, func(d *Def) bool { return d.Apply == Runtime })
	if eff.Runner.Enabled {
		t.Error("restart setting applied")
	}
	// restart settings keep their running value
	running := config.Default()
	running.Runner.Web.MaxParallel = 7
	next := config.Default()
	next.Runner.Web.MaxParallel = 3
	CopyRestart(&next, &running)
	if next.Runner.Web.MaxParallel != 7 {
		t.Error("CopyRestart")
	}
}

func TestProviders(t *testing.T) {
	t.Setenv("QTF_OIDC_ENVP_CLIENT_SECRET", "from-env")
	base := config.Default()
	base.Auth.OIDC = []config.OIDC{{Name: "filep", Issuer: "https://f", ClientID: "c", ClientSecret: "file-secret"}}
	stored := []OIDCProvider{
		{Name: "filep", Issuer: "https://f", ClientID: "c", Enabled: true},
		{Name: "envp", Issuer: "https://e", ClientID: "c", ClientSecret: "db", Enabled: false},
		{Name: "dbp", Issuer: "https://d", ClientID: "c", ClientSecret: "db-secret", Enabled: true, RoleMapping: map[string]string{"g": "admin"}},
		{Name: "none", Issuer: "https://n", ClientID: "c", Enabled: true},
	}
	out := ResolveProviders(&base, stored)
	want := map[string]string{"filep": "file-secret", "envp": "from-env", "dbp": "db-secret", "none": ""}
	for _, o := range out {
		if o.ClientSecret != want[o.Name] {
			t.Errorf("%s: secret %q", o.Name, o.ClientSecret)
		}
	}
	if out[1].On() || !out[2].On() || out[2].RoleMapping["g"] != "admin" {
		t.Error("fields")
	}
	src := map[string]string{"filep": SourceFile, "envp": SourceEnvironment, "dbp": SourceDatabase, "none": ""}
	for n, w := range src {
		if got := SecretSource(&base, stored, n); got != w {
			t.Errorf("%s: source %q, want %q", n, got, w)
		}
	}
	if SecretSource(&base, nil, "filep") != SourceFile {
		t.Error("file source without stored list")
	}
	back := FromConfig(out)
	if len(back) != 4 || back[2].ClientSecret != "db-secret" || back[1].Enabled {
		t.Errorf("FromConfig: %+v", back)
	}
	for _, p := range []OIDCProvider{
		{Name: "", Issuer: "https://i", ClientID: "c"},
		{Name: "ok", Issuer: "ftp://i", ClientID: "c"},
		{Name: "ok", Issuer: "https://i", ClientID: " "},
		{Name: "ok", Issuer: "https://i", ClientID: "c", DisplayName: "a\nb"},
		{Name: "ok", Issuer: "https://i", ClientID: "c", Scopes: []string{"open id"}},
	} {
		if ValidateProvider(p) == nil {
			t.Errorf("accepted %+v", p)
		}
	}
	if err := ValidateProviders([]OIDCProvider{{Name: "a", Issuer: "https://i", ClientID: "c", Scopes: []string{"openid"}}}); err != nil {
		t.Error(err)
	}
}

func TestProviderLocalMFARoundTrip(t *testing.T) {
	base := &config.Config{}
	in := []config.OIDC{{Name: "corp", Issuer: "https://idp.example", ClientID: "c", LocalMFA: config.LocalMFAUnlessIdPMFA, MFAAMRValues: []string{"mfa", "otp"}, MFAACRValues: []string{"gold"}}}
	ps := FromConfig(in)
	if ps[0].LocalMFA != "unless_idp_mfa" || len(ps[0].MFAAMRValues) != 2 || ps[0].MFAACRValues[0] != "gold" {
		t.Fatal(ps[0])
	}
	out := ResolveProviders(base, ps)
	if out[0].LocalMFA != "unless_idp_mfa" || out[0].MFAAMRValues[1] != "otp" || out[0].MFAACRValues[0] != "gold" {
		t.Fatal(out[0])
	}
	bad := ps[0]
	bad.LocalMFA = "sometimes"
	if ValidateProvider(bad) == nil {
		t.Fatal("bad local_mfa accepted")
	}
	bad = ps[0]
	bad.MFAAMRValues = []string{"two words"}
	if ValidateProvider(bad) == nil {
		t.Fatal("bad amr accepted")
	}
	if err := ValidateProvider(ps[0]); err != nil {
		t.Fatal(err)
	}
}
