package settings

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

// Every setting has a label and a hint in both languages of the web client
// (sysSettings.fields.<key with _ for .>).
func TestRegistryTexts(t *testing.T) {
	for _, lang := range []string{"en", "sv"} {
		b, err := os.ReadFile("../../web/src/i18n/locales/" + lang + ".ts")
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		i := strings.Index(src, "  sysSettings: {")
		j := strings.Index(src[i:], "\n    fields: {")
		if i < 0 || j < 0 {
			t.Fatalf("%s: no sysSettings.fields", lang)
		}
		fields := src[i+j:]
		fields = fields[:strings.Index(fields, "\n    },")]
		for _, d := range Registry {
			if d.Kind == OIDCList {
				continue
			}
			id := strings.ReplaceAll(d.Key, ".", "_")
			re := regexp.MustCompile(`\n      ` + id + `: \{ label: '[^']+', hint: '[^']+' \}`)
			if !re.MatchString(fields) {
				t.Errorf("%s: no label and hint for %s", lang, d.Key)
			}
		}
	}
}

func TestNewSettingsDecode(t *testing.T) {
	ok := map[string]string{
		"outbound.allowed_hosts":    `[" jira.example.com ", "*.example.org", "jira.example.com", "", "10.0.0.1"]`,
		"outbound.proxy":            `"http://user@proxy.example.com:3128"`,
		"server.frame_ancestors":    `["https://doors.example.com"]`,
		"auth.session_idle_timeout": `"0s"`,
		"email.reply_to":            `"help@example.com"`,
		"auth.password_min_classes": `3`,
		"live.max_connections":      `100`,
		"runner.web.channel":        `""`,
	}
	for k, raw := range ok {
		v, err := def(t, k).Decode(json.RawMessage(raw))
		if err != nil {
			t.Errorf("%s %s: %v", k, raw, err)
		}
		if k == "outbound.allowed_hosts" && strings.Join(v.([]string), ",") != "jira.example.com,*.example.org,10.0.0.1" {
			t.Errorf("list %v", v)
		}
	}
	bad := map[string][]string{
		"outbound.allowed_hosts":    {`"a"`, `["a b"]`, `["https://x"]`, `["a\nb"]`, `[1]`},
		"outbound.proxy":            {`"socks5://p:1"`, `"proxy:3128"`, `"http://u:pw@proxy:3128"`},
		"server.frame_ancestors":    {`["https://a/path"]`, `["a.example.com"]`},
		"auth.session_idle_timeout": {`"30s"`, `"800h"`},
		"email.reply_to":            {`"Help <h@example.com>"`, `"nope"`},
		"auth.password_min_classes": {`5`, `-1`},
		"auth.token_max_days":       {`4000`},
		"runner.web.channel":        {`"a b"`},
		"server.listen":             {`":9"`},
		"database.dsn":              {`"x"`},
	}
	for k, raws := range bad {
		for _, raw := range raws {
			if _, err := def(t, k).Decode(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s %s accepted: %v", k, raw, err)
			}
		}
	}
	many := make([]string, 201)
	for i := range many {
		many[i] = "h" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + ".example.com" + strings.Repeat("y", i/26)
	}
	b, _ := json.Marshal(many)
	if _, err := def(t, "outbound.denied_hosts").Decode(b); err == nil {
		t.Error("too many entries")
	}
}

func TestNewSettingsApply(t *testing.T) {
	base := config.Default()
	stored := map[string]json.RawMessage{
		"outbound.denied_hosts":     json.RawMessage(`["169.254.169.254"]`),
		"auth.session_idle_timeout": json.RawMessage(`"15m"`),
		"live.replay_buffer":        json.RawMessage(`10`),
		"email.reply_to":            json.RawMessage(`"a@example.com"`),
	}
	eff, ignored := Effective(base, stored, nil)
	if len(ignored) != 0 || eff.Outbound.DeniedHosts[0] != "169.254.169.254" || eff.Auth.SessionIdleTimeout != 15*time.Minute || eff.Live.ReplayBuffer != 10 || eff.Email.ReplyTo != "a@example.com" {
		t.Fatalf("%v %+v", ignored, eff.Outbound)
	}
	if base.Outbound.DeniedHosts != nil {
		t.Error("base modified")
	}
	// restart settings keep their running value
	running := config.Default()
	CopyRestart(&eff, &running)
	if eff.Live.ReplayBuffer != running.Live.ReplayBuffer || eff.Auth.SessionIdleTimeout != 15*time.Minute {
		t.Error("CopyRestart")
	}
	// read-only values: lists never nil, secrets never shown, maps as entries
	c := config.Default()
	if v := def(t, "server.trusted_proxies").Display(&c).([]string); v == nil || len(v) != 0 {
		t.Error(v)
	}
	if def(t, "database.dsn").Display(&c) != nil || def(t, "database.dsn").Get(&c) != "qtestframe.db" {
		t.Error("dsn shown")
	}
	if v := def(t, "runner.interpreters").Display(&c).([]string); len(v) != 4 || v[0] != "bash=/bin/bash" {
		t.Error(v)
	}
	c.Outbound.Proxy = "http://u:secret@proxy:3128"
	if v := def(t, "outbound.proxy").Display(&c); v != "http://u:REDACTED@proxy:3128" {
		t.Error(v)
	}
	for _, d := range Registry {
		if _, ok := FileOnly[d.Key]; ok {
			t.Errorf("%s in the registry and in FileOnly", d.Key)
		}
	}
}

func TestOIDCProviderNewFields(t *testing.T) {
	p := OIDCProvider{Name: "corp", Issuer: "https://idp.example.com", ClientID: "c", Icon: "microsoft", AllowedDomains: []string{"example.com", "*.example.org"},
		RoleMapping: map[string]string{"qa": "tester, reviewer"}}
	if err := ValidateProvider(p); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []OIDCProvider{
		{Name: "a", Issuer: "https://i", ClientID: "c", Icon: "rocket"},
		{Name: "a", Issuer: "https://i", ClientID: "c", AllowedDomains: []string{"a@example.com"}},
		{Name: "a", Issuer: "https://i", ClientID: "c", RoleMapping: map[string]string{"qa": " "}},
	} {
		if err := ValidateProvider(bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	base := config.Default()
	got := ResolveProviders(&base, []OIDCProvider{p})
	if got[0].Icon != "microsoft" || len(got[0].AllowedDomains) != 2 {
		t.Fatalf("%+v", got[0])
	}
	back := FromConfig(got)
	if back[0].Icon != "microsoft" || len(back[0].AllowedDomains) != 2 {
		t.Fatalf("%+v", back[0])
	}
}
