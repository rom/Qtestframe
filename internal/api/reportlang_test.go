package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

func TestReportLangResolution(t *testing.T) {
	p := func(pref string) *Principal {
		return &Principal{User: &model.User{Preferences: model.Preferences{Language: pref}}}
	}
	cases := []struct {
		name, body, query, pref, accept, want string
		nilPrincipal                          bool
	}{
		{"body wins", "sv", "en", "en", "en", "sv", false},
		{"body with region", "sv-SE", "", "", "", "sv", false},
		{"invalid body: query", "xx", "sv", "en", "en", "sv", false},
		{"query wins over preference", "", "sv", "en", "en", "sv", false},
		{"invalid query: preference", "", "de", "sv", "en", "sv", false},
		{"preference wins over browser", "", "", "en", "sv", "en", false},
		{"browser", "", "", "", "de-DE,sv;q=0.8,en;q=0.5", "sv", false},
		{"unsupported everywhere", "fr", "de", "xx", "fr-FR", "en", false},
		{"nothing", "", "", "", "", "en", false},
		{"no principal", "", "", "", "sv", "sv", true},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/x?lang="+c.query, nil)
		if c.accept != "" {
			r.Header.Set("Accept-Language", c.accept)
		}
		pr := p(c.pref)
		if c.nilPrincipal {
			pr = nil
		}
		if got := reportLang(r, pr, c.body); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSwedishReports(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	base := "/api/v1/projects/" + fx.pid + "/reports"
	gen := base + "/generate"
	custom := map[string]any{"entity": "requirement", "columns": []string{"key", "title", "status", "criteria"}, "sort_by": "key"}

	// English by default
	_, data := a.download(gen, map[string]any{"custom": custom, "format": "csv"}, 200)
	if !strings.HasPrefix(string(data), "Key,Title,Status,Evaluation criteria\r\n") || !strings.Contains(string(data), "approved") {
		t.Fatal(string(data))
	}
	// explicit lang in the body: headers, enumeration values and texts in
	// Swedish, the project's content unchanged
	_, data = a.download(gen, map[string]any{"custom": custom, "format": "csv", "lang": "sv"}, 200)
	if !strings.HasPrefix(string(data), "Nyckel,Titel,Status,Utvärderingskriterier\r\n") || !strings.Contains(string(data), "Godkänt") ||
		!strings.Contains(string(data), "Login") || !strings.Contains(string(data), "1. valid login (Test; 1 s)") {
		t.Fatal(string(data))
	}
	// the "lang" query parameter
	_, data = a.download(gen+"?lang=sv", map[string]any{"custom": map[string]any{"entity": "test", "columns": []string{"key", "step", "execution_type"}}, "format": "csv"}, 200)
	if !strings.HasPrefix(string(data), "Nyckel,Steg,Exekveringstyp\r\n") || !strings.Contains(string(data), "[anropa ") || !strings.Contains(string(data), "Automatiserat") {
		t.Fatal(string(data))
	}
	// the browser's language
	a.headers = map[string]string{"Accept-Language": "sv-SE,sv;q=0.9,en;q=0.8"}
	_, data = a.download(gen, map[string]any{"template": "requirements_coverage", "format": "csv"}, 200)
	files := zipFiles(t, data)
	if !strings.HasPrefix(files["summary.csv"], "Mått,Värde\r\nKrav,3\r\n") || !strings.Contains(files["requirements.csv"], "Kravnivå") {
		t.Fatal(files)
	}
	// the user's preference beats the browser; an explicit lang beats both
	setPrefs(t, a, map[string]any{"language": "en"})
	_, data = a.download(gen, map[string]any{"template": "requirements_coverage", "format": "csv"}, 200)
	if !strings.HasPrefix(zipFiles(t, data)["summary.csv"], "Metric,Value") {
		t.Fatal("preference")
	}
	a.headers = nil
	setPrefs(t, a, map[string]any{"language": "sv"})
	_, data = a.download(gen, map[string]any{"template": "requirements_coverage", "format": "csv"}, 200)
	if !strings.HasPrefix(zipFiles(t, data)["summary.csv"], "Mått,Värde") {
		t.Fatal("preference sv")
	}
	_, data = a.download(gen, map[string]any{"template": "requirements_coverage", "format": "csv", "lang": "en"}, 200)
	if !strings.HasPrefix(zipFiles(t, data)["summary.csv"], "Metric,Value") {
		t.Fatal("explicit en")
	}
	setPrefs(t, a, map[string]any{"language": ""})
	// archived reports get the translated title
	fi := a.m("POST", gen, map[string]any{"template": "run_summary", "params": map[string]string{"run_id": fx.runID}, "format": "pdf", "archive": true, "lang": "sv"}, 201)
	if fi["description"] != "Sammanfattning av körning" {
		t.Fatal(fi)
	}
	d := a.m("POST", base+"/definitions", map[string]any{"name": "Godkända krav", "config": map[string]any{"entity": "requirement", "columns": []string{"key", "obligation"}}}, 201)
	_, data = a.download("/api/v1/reports/definitions/"+d["id"].(string)+"/generate", map[string]any{"format": "csv", "lang": "sv"}, 200)
	if !strings.HasPrefix(string(data), "Nyckel,Kravnivå\r\n") {
		t.Fatal(string(data))
	}
	// several reports at once
	_, data = a.download(base+"/batch", map[string]any{"templates": []map[string]any{{"template": "test_inventory"}}, "definitions": []any{d["id"]}, "format": "csv", "lang": "sv"}, 200)
	n := 0
	for name, body := range zipFiles(t, data) {
		if strings.HasSuffix(name, ".zip") {
			body = zipFiles(t, []byte(body))["by_folder.csv"]
		}
		if strings.HasPrefix(body, "Mapp,Antal,Andel") || strings.HasPrefix(body, "Nyckel,Kravnivå") {
			n++
		} else {
			t.Errorf("%s is not Swedish", name)
		}
	}
	if n != 2 {
		t.Fatal(n)
	}

	// e-mail now: the report and the e-mail in the sender's UI language
	tess := e.member("tess@example.com", "tester")
	a.m("POST", base+"/email", map[string]any{"template": "requirements_coverage", "format": "xlsx", "recipients": []string{tess.ID}, "lang": "sv"}, 202)
	ms := sent(e, f)
	if len(ms) != 1 || ms[0].Subject != "Rapport: Kravtäckning" || !strings.Contains(ms[0].Text, "Kravtäckning") {
		t.Fatal(ms[0].Subject)
	}

	// subscriptions store the language of the request
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	e.srv.Clock = func() time.Time { return now }
	sub := a.m("POST", base+"/subscriptions?lang=sv", map[string]any{"definition_id": d["id"], "format": "csv", "recipients": []string{tess.ID}}, 201)
	if sub["lang"] != "sv" {
		t.Fatal(sub)
	}
	id := sub["id"].(string)
	// an unsupported language keeps it; enabling without a language too
	if up := a.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"lang": "de", "enabled": true}, 200); up["lang"] != "sv" {
		t.Fatal(up)
	}
	f.Reset()
	now = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if n, err := e.srv.RunReportSubscriptions(context.Background()); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	ms = f.Messages()
	if len(ms) != 1 || !strings.HasPrefix(string(ms[0].Attachments[0].Data), "Nyckel,Kravnivå") || !strings.Contains(ms[0].Text, "schemalagda rapporten") {
		t.Fatal(ms)
	}
	// changed to English by the next edit in an English UI
	if up := a.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"lang": "en"}, 200); up["lang"] != "en" {
		t.Fatal(up)
	}
	f.Reset()
	if res := a.m("POST", "/api/v1/reports/subscriptions/"+id+"/send", nil, 200); res["last_error"] != "" {
		t.Fatal(res)
	}
	if ms = f.Messages(); len(ms) != 1 || !strings.HasPrefix(string(ms[0].Attachments[0].Data), "Key,Obligation") {
		t.Fatal(ms)
	}
	// without a stored language the owner's preference is used
	sub = a.m("POST", base+"/subscriptions", map[string]any{"definition_id": d["id"], "format": "csv", "recipients": []string{tess.ID}}, 201)
	if sub["lang"] != "" {
		t.Fatal(sub)
	}
	setPrefs(t, a, map[string]any{"language": "sv"})
	f.Reset()
	a.m("POST", "/api/v1/reports/subscriptions/"+sub["id"].(string)+"/send", nil, 200)
	if ms = f.Messages(); len(ms) != 1 || !strings.HasPrefix(string(ms[0].Attachments[0].Data), "Nyckel,Kravnivå") {
		t.Fatal(ms)
	}
}
