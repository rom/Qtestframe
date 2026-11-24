package admindoc

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/settings"
)

// Fields returns every key of the configuration file with its default.
func Fields() []Field {
	d := config.Default()
	return walk(reflect.TypeOf(d), reflect.ValueOf(d), "")
}

// group is the heading a key is listed under: the first part of the key,
// lists of providers and runner.web get their own.
func group(key string) string {
	switch {
	case strings.HasPrefix(key, "auth.oidc"):
		return "auth.oidc"
	case strings.HasPrefix(key, "auth.saml"):
		return "auth.saml"
	case strings.HasPrefix(key, "runner.web."):
		return "runner.web"
	}
	g, _, _ := strings.Cut(key, ".")
	return g
}

// envByKey maps dotted keys to the environment variables that set them.
func envByKey() map[string][]string {
	out := map[string][]string{}
	for env, key := range config.EnvVars {
		out[key] = append(out[key], env)
	}
	for _, v := range out {
		sort.Strings(v)
	}
	return out
}

var none = text{"—", "—"}

// formatDefault renders a default value.
func formatDefault(f Field, lang string) string {
	if t, ok := defaultDocs[f.Key]; ok {
		return t.in(lang)
	}
	v := f.Default
	if !v.IsValid() {
		return none.in(lang)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return none.in(lang)
		}
		v = v.Elem()
	}
	if v.Type() == durationType {
		return code(settings.ShortDuration(time.Duration(v.Int())))
	}
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if s == "" {
			return none.in(lang)
		}
		if tmp := os.TempDir(); strings.HasPrefix(s, tmp) {
			s = "$TMPDIR" + strings.TrimPrefix(s, tmp)
		}
		return code(s)
	case reflect.Bool:
		return code(fmt.Sprint(v.Bool()))
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		return code(fmt.Sprint(v.Interface()))
	case reflect.Slice:
		if v.Len() == 0 {
			return none.in(lang)
		}
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = fmt.Sprint(v.Index(i).Interface())
		}
		return code("[" + strings.Join(parts, ", ") + "]")
	case reflect.Map:
		if v.Len() == 0 {
			return none.in(lang)
		}
		keys := make([]string, 0, v.Len())
		for _, k := range v.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = code(k + ": " + fmt.Sprint(v.MapIndex(reflect.ValueOf(k)).Interface()))
		}
		return strings.Join(parts, ", ")
	}
	return code(fmt.Sprint(v.Interface()))
}

var typeText = map[string]text{
	"string": {"string", "text"}, "bool": {"bool", "bool"}, "integer": {"integer", "heltal"},
	"duration": {"duration", "tidslängd"}, "list": {"list", "lista"}, "map": {"map", "mappning"},
}

var configHeader = map[string]text{
	"cols": {"| Key | Type | Default | Environment | Description |", "| Nyckel | Typ | Standard | Miljövariabel | Beskrivning |"},
}

// configReference is the table of every configuration key, grouped by
// section.
func configReference(lang string) (string, error) {
	env := envByKey()
	var b strings.Builder
	cur := ""
	for _, f := range Fields() {
		doc, ok := keyDocs[f.Key]
		if !ok {
			return "", fmt.Errorf("no description of %s in internal/admindoc/configtext.go", f.Key)
		}
		if g := group(f.Key); g != cur {
			if cur != "" {
				b.WriteString("\n")
			}
			cur = g
			fmt.Fprintf(&b, "### `%s`\n\n%s\n|---|---|---|---|---|\n", g, configHeader["cols"].in(lang))
		}
		e := strings.Join(env[f.Key], ", ")
		if e == "" {
			e = none.in(lang)
		} else {
			e = code(strings.ReplaceAll(e, ", ", "`, `"))
		}
		if f.Key == "auth.oidc[].client_secret" {
			e = code("QTF_OIDC_<NAME>_CLIENT_SECRET")
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", f.Key, typeText[f.Type].in(lang), cell(formatDefault(f, lang)), e, cell(doc.in(lang)))
	}
	return b.String(), nil
}

var envHeader = text{"| Variable | Sets |\n|---|---|", "| Variabel | Sätter |\n|---|---|"}

// envReference lists the environment variables.
func envReference(lang string) (string, error) {
	names := make([]string, 0, len(config.EnvVars))
	for n := range config.EnvVars {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(envHeader.in(lang) + "\n")
	for _, e := range extraEnv[:1] {
		fmt.Fprintf(&b, "| `%s` | %s |\n", e.name, cell(e.doc.in(lang)))
	}
	for _, n := range names {
		d, ok := envDocs[n]
		if !ok {
			return "", fmt.Errorf("no description of %s in internal/admindoc/configtext.go", n)
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", n, cell(d.in(lang)))
	}
	for _, e := range extraEnv[1:] {
		fmt.Fprintf(&b, "| `%s` | %s |\n", strings.ReplaceAll(e.name, ", ", "`, `"), cell(e.doc.in(lang)))
	}
	return b.String(), nil
}

// ---------------------------------------------------------------- system settings

var sectionText = map[string]text{
	"general": {"General", "Allmänt"}, "server": {"Server and database", "Server och databas"},
	"security": {"Sign-in and passwords", "Inloggning och lösenord"}, "tokens": {"API tokens", "API-tokens"},
	"oidc":    {"Single sign-on", "Enkel inloggning"},
	"logging": {"Logging and syslog", "Loggning och syslog"}, "email": {"E-mail (SMTP)", "E-post (SMTP)"},
	"outbound": {"Outgoing connections", "Utgående anslutningar"}, "live": {"Live updates", "Liveuppdateringar"},
	"runner": {"Script and web test runner", "Körning av skript och webbtester"}, "similarity": {"Similarity (duplicates)", "Likhet (dubbletter)"}, "ai": {"AI assistance", "AI-assistans"}, "retention": {"Retention and backups", "Gallring och säkerhetskopior"},
	"backup": {"Scheduled backups", "Schemalagda säkerhetskopior"},
}

var applyText = map[settings.Apply]text{
	settings.Runtime:  {"at once", "direkt"},
	settings.Restart:  {"after a restart", "efter omstart"},
	settings.ReadOnly: {"shown only (file)", "visas bara (filen)"},
}

var settingsHeader = text{"| Section | Key | Values | Applies |\n|---|---|---|---|", "| Avsnitt | Nyckel | Värden | Gäller |\n|---|---|---|---|"}

var kindText = map[settings.Kind]text{
	settings.String: {"text", "text"}, settings.Bool: {"on/off", "på/av"}, settings.Secret: {"secret (write-only)", "hemlighet (skrivskyddad läsning)"},
	settings.List: {"list", "lista"}, settings.OIDCList: {"list of providers", "lista med leverantörer"},
	settings.Int: {"number", "tal"}, settings.Duration: {"duration", "tidslängd"}, settings.Enum: {"choice", "val"},
}

// settingsReference lists the settings of Administration → System
// settings (settings.Registry).
func settingsReference(lang string) (string, error) {
	var b strings.Builder
	b.WriteString(settingsHeader.in(lang) + "\n")
	for _, d := range settings.Registry {
		sec, ok := sectionText[d.Section]
		if !ok {
			return "", fmt.Errorf("no name for the section %q", d.Section)
		}
		var values string
		switch {
		case d.Apply == settings.ReadOnly:
			values = kindText[d.Kind].in(lang)
		case d.Kind == settings.Int:
			values = fmt.Sprintf("%d–%d", d.Min, d.Max)
		case d.Kind == settings.Duration:
			values = settings.ShortDuration(d.MinDur) + "–" + settings.ShortDuration(d.MaxDur)
		case d.Kind == settings.Enum:
			values = "`" + strings.Join(d.Options, "`, `") + "`"
		default:
			values = kindText[d.Kind].in(lang)
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n", sec.in(lang), d.Key, cell(values), applyText[d.Apply].in(lang))
	}
	return b.String(), nil
}
