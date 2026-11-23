package prompt

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

// The secrets in these tests are assembled from parts and obviously fake,
// so that no literal in the repository looks like a real credential.
var (
	fakePass = "Wh4t" + "ever" + "-x9"
	fakeKey  = "AK" + "IA" + strings.Repeat("Q", 16)
	fakeGH   = "gh" + "p_" + strings.Repeat("a1B2", 9)
	fakeJWT  = "ey" + "J" + strings.Repeat("hbGciOi", 3) + "." + strings.Repeat("eyJzdWIi", 3) + "." + strings.Repeat("c2lnbmF0", 3)
	fakeRand = strings.Repeat("aZ3qR7", 8)
)

func TestCheckPreamble(t *testing.T) {
	for _, c := range []struct {
		text, kind string
	}{
		{"postgres://app:" + fakePass + "@db.internal:5432/shop", WarnPasswordInURL},
		{"mongodb+srv://svc:" + fakePass + "@cluster0.example.net/app", WarnPasswordInURL},
		{"Server=sql1;Database=Shop;User Id=app;Password=" + fakePass + ";", WarnPasswordAssignment},
		{"jdbc:oracle:thin:@host:1521/ORCL?user=app&password=" + fakePass, WarnPasswordAssignment},
		{"host=db user=app pwd=" + fakePass, WarnPasswordAssignment},
		{`{"api_key": "` + fakePass + `"}`, WarnPasswordAssignment},
		{"password: " + fakePass, WarnPasswordAssignment},
		{"-----BEGIN RSA PRIVATE KEY-----\nMIIE...", WarnPrivateKey},
		{"key id " + fakeKey, WarnToken},
		{"token " + fakeGH, WarnToken},
		{"bearer " + fakeJWT, WarnToken},
		{"seed " + fakeRand, WarnToken},
	} {
		p := &model.SolutionPreamble{Databases: c.text}
		w := CheckPreamble(p)
		if len(w) != 1 || w[0].Kind != c.kind || w[0].Field != "databases" || w[0].Message == "" {
			t.Errorf("%q: %+v", c.text, w)
			continue
		}
		if strings.Contains(w[0].Excerpt, fakePass) || !strings.Contains(w[0].Excerpt, masked) {
			t.Errorf("excerpt shows the secret: %q", w[0].Excerpt)
		}
		if r := Redact(c.text); strings.Contains(r, fakePass) || !strings.Contains(r, masked) {
			t.Errorf("redact %q → %q", c.text, r)
		}
	}
	// descriptions and placeholders are fine
	for _, ok := range []string{
		"PostgreSQL 16; the application connects with the credentials from the vault",
		"postgres://app@db.internal:5432/shop (no password: IAM authentication)",
		"postgres://app:${DB_PASSWORD}@db:5432/shop",
		"Server=sql1;Password=<from Key Vault>;",
		"password=*** (injected at runtime)",
		"Passwords: hashed with argon2id",
		"password: argon2id",
		"Authentication: token-based (OAuth 2.0 bearer tokens)",
		"Go 1.26, Vue 3 — github.com/rom/qtestframe/internal/prompt/preamble_test",
		"550e8400-e29b-41d4-a716-446655440000",
	} {
		if w := CheckPreamble(&model.SolutionPreamble{Security: ok}); len(w) != 0 {
			t.Errorf("%q: %+v", ok, w)
		}
		if Redact(ok) != ok {
			t.Errorf("redacted %q", ok)
		}
	}
	// several findings in several fields; nothing is double-counted
	p := &model.SolutionPreamble{Databases: "postgres://a:" + fakePass + "@h/db and password=" + fakePass, Integrations: "key " + fakeKey}
	if w := CheckPreamble(p); len(w) != 3 || w[2].Field != "integrations" {
		t.Errorf("%+v", w)
	}
	if r := Redact("postgres://a:" + fakePass + "@h/db password=" + fakePass); r != "postgres://a:"+masked+"@h/db password="+masked {
		t.Error(r)
	}
	long := strings.Repeat("y", 200) + " password=" + fakePass + " " + strings.Repeat("z", 200)
	if w := CheckPreamble(&model.SolutionPreamble{Additional: long}); len(w) != 1 || len([]rune(w[0].Excerpt)) > 120 {
		t.Errorf("%+v", w)
	}
}

func TestRedactInPrompt(t *testing.T) {
	in := solutionSample(Architecture, "en")
	in.Preamble.Databases = "postgres://app:" + fakePass + "@db/shop"
	in.TechContext = "deploy key " + fakeGH
	txt := Build(in).Text
	if strings.Contains(txt, fakePass) || strings.Contains(txt, fakeGH) || !strings.Contains(txt, "postgres://app:[REDACTED]@db/shop") {
		t.Error(txt)
	}
}

func TestForbiddenPreambleKey(t *testing.T) {
	for _, k := range []string{"password", "db_password", "Secret", "api_token", "credentials", "apiKey", "connection_string"} {
		if !ForbiddenPreambleKey(k) {
			t.Errorf("%s allowed", k)
		}
	}
	for _, k := range []string{"databases", "security", "additional", "languages"} {
		if ForbiddenPreambleKey(k) {
			t.Errorf("%s refused", k)
		}
	}
}

func TestTemplates(t *testing.T) {
	ids := map[string]bool{}
	for _, tp := range Templates {
		if ids[tp.ID] || tp.Name["en"] == "" || tp.Name["sv"] == "" {
			t.Errorf("template %q", tp.ID)
		}
		ids[tp.ID] = true
		p := tp.Preamble
		if p.Languages == "" || p.Databases == "" || p.Security == "" || p.Testing == "" || p.ArchitectureStyle == "" {
			t.Errorf("%s incomplete", tp.ID)
		}
		found := false
		for _, s := range model.ArchitectureStyles {
			found = found || s == p.ArchitectureStyle
		}
		if !found {
			t.Errorf("%s: style %q", tp.ID, p.ArchitectureStyle)
		}
		// templates never trip the credential check
		if w := CheckPreamble(&p); len(w) != 0 {
			t.Errorf("%s: %+v", tp.ID, w)
		}
		for _, f := range model.PreambleFields {
			if n := len([]rune(*p.Field(f))); n > model.MaxPreambleField {
				t.Errorf("%s.%s too long (%d)", tp.ID, f, n)
			}
		}
	}
	for _, want := range []string{"go-postgres-vue", "java-spring-oracle", "dotnet-sqlserver", "python-fastapi-postgres", "node-ts-mongodb", "embedded-c-rtos"} {
		if FindTemplate(want) == nil {
			t.Errorf("no template %s", want)
		}
	}
	if FindTemplate("cobol") != nil {
		t.Error("unknown template found")
	}
}
