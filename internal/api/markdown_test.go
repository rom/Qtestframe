package api

import (
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/report"
)

// mdTablesOK reports whether every GFM table row of md has as many cells
// as its header row.
func mdTablesOK(md string) bool {
	want := -1
	for _, l := range strings.Split(md, "\n") {
		if !strings.HasPrefix(l, "|") {
			want = -1
			continue
		}
		n := strings.Count(l, "|") - strings.Count(l, `\|`)
		if want < 0 {
			want = n
		} else if n != want {
			return false
		}
	}
	return true
}

func TestMarkdownDownloads(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	get := func(path string) (string, string, string) {
		t.Helper()
		res := a.raw("GET", path, nil)
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("GET %s: %d %s", path, res.StatusCode, data)
		}
		return res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"), string(data)
	}

	// project export: one document with all aspects, keys linked (base URL configured)
	for _, format := range []string{"md", "markdown"} {
		ct, cd, md := get("/api/v1/projects/" + fx.pid + "/export?format=" + format + "&q=" + url.QueryEscape("status:approved"))
		if ct != "text/markdown; charset=utf-8" || !strings.HasSuffix(cd, `.md"`) || !strings.HasPrefix(md, "# RPT – ") ||
			!strings.Contains(md, "- **Filter:** status:approved\n") || !strings.Contains(md, "## Contents") ||
			!strings.Contains(md, "## Requirements\n") || !strings.Contains(md, "## Traceability matrix\n") ||
			!strings.Contains(md, "[RPT-REQ-1](http://example.test/go/RPT-REQ-1)") || !mdTablesOK(md) {
			t.Fatalf("%s %s\n%s", ct, cd, md)
		}
	}

	// reports: every template and a custom report, in English and Swedish
	gen := "/api/v1/projects/" + fx.pid + "/reports/generate"
	for _, lang := range []string{"en", "sv"} {
		for _, tpl := range report.Templates() {
			if !strings.Contains(strings.Join(tpl.Formats, ","), "md") {
				t.Fatalf("%s formats %v", tpl.ID, tpl.Formats)
			}
			body := map[string]any{"template": tpl.ID, "format": "md", "lang": lang}
			if tpl.ID == report.RunSummary {
				body["params"] = map[string]string{"run_id": fx.runID}
			}
			res, data := a.download(gen, body, 200)
			md := string(data)
			if res.Header.Get("Content-Type") != "text/markdown; charset=utf-8" || !strings.Contains(res.Header.Get("Content-Disposition"), ".md") ||
				!strings.HasPrefix(md, "# ") || !strings.Contains(md, "\n## ") || !mdTablesOK(md) {
				t.Fatalf("%s/%s: %v\n%s", tpl.ID, lang, res.Header, md)
			}
			generated := map[string]string{"en": "- **Generated:** ", "sv": "- **Skapad:** "}[lang]
			if !strings.Contains(md, generated) {
				t.Fatalf("%s/%s: %s", tpl.ID, lang, md)
			}
		}
	}
	_, data := a.download(gen, map[string]any{"template": "requirements_coverage", "format": "md", "lang": "sv"}, 200)
	if md := string(data); !strings.HasPrefix(md, "# Kravtäckning\n") || !strings.Contains(md, "| Mått | Värde |") || !strings.Contains(md, "## Innehåll") {
		t.Fatal(md)
	}
	custom := map[string]any{"entity": "requirement", "columns": []string{"key", "title", "status", "criteria"}, "sort_by": "key", "title": "Mine | yours"}
	_, data = a.download(gen, map[string]any{"custom": custom, "format": "md", "lang": "sv"}, 200)
	if md := string(data); !strings.HasPrefix(md, "# Mine \\| yours\n") || !strings.Contains(md, "| Nyckel | Titel | Status | Utvärderingskriterier |") ||
		!strings.Contains(md, "Godkänt") || !strings.Contains(md, "[RPT-REQ-1](http://example.test/go/RPT-REQ-1)") || !mdTablesOK(md) {
		t.Fatal(md)
	}
	// saved in the document archive
	fi := a.m("POST", gen, map[string]any{"template": "defects", "format": "md", "archive": true, "name": "defects"}, 201)
	if fi["name"] != "defects.md" || fi["content_type"] != "text/markdown; charset=utf-8" {
		t.Fatal(fi)
	}
	// several at once: a ZIP of Markdown files
	_, data = a.download("/api/v1/projects/"+fx.pid+"/reports/batch", map[string]any{
		"templates": []map[string]any{{"template": "project_status"}, {"template": "test_inventory"}}, "format": "md", "lang": "sv"}, 200)
	files := zipFiles(t, data)
	if len(files) != 2 {
		t.Fatal(files)
	}
	for name, md := range files {
		if !strings.HasSuffix(name, ".md") || !strings.Contains(md, "**Skapad:**") || !mdTablesOK(md) {
			t.Fatal(name, md)
		}
	}
	// e-mailed as a Markdown attachment
	tess := e.member("tess@example.com", "tester")
	a.m("POST", "/api/v1/projects/"+fx.pid+"/reports/email", map[string]any{"template": "project_status", "format": "md", "recipients": []string{tess.ID}}, 202)
	if ms := sent(e, f); len(ms) != 1 || len(ms[0].Attachments) != 1 || !strings.HasSuffix(ms[0].Attachments[0].Name, ".md") ||
		ms[0].Attachments[0].ContentType != "text/markdown; charset=utf-8" || !strings.Contains(ms[0].Text, "MD") {
		t.Fatal(ms)
	}
	// subscriptions accept the format (and its alias)
	def := a.m("POST", "/api/v1/projects/"+fx.pid+"/reports/definitions", map[string]any{"name": "Tests", "config": map[string]any{"entity": "test", "columns": []string{"key", "title"}}}, 201)
	sub := a.m("POST", "/api/v1/projects/"+fx.pid+"/reports/subscriptions", map[string]any{"definition_id": def["id"], "format": "markdown", "recipients": []string{tess.ID}}, 201)
	if sub["format"] != "md" {
		t.Fatal(sub)
	}

	// audit trail
	ct, cd, md := get("/api/v1/audit?format=md&q=" + url.QueryEscape("action:project.*"))
	if ct != "text/markdown; charset=utf-8" || !strings.HasSuffix(cd, `.md"`) || !strings.HasPrefix(md, "# Audit trail\n") ||
		!strings.Contains(md, "- **Filter:** action:project.\\*\n") || !strings.Contains(md, "| Time (UTC) | User | Action | Entity | Entity ID | Details |") ||
		!strings.Contains(md, "project.create") || !mdTablesOK(md) {
		t.Fatalf("%s %s\n%s", ct, cd, md)
	}
	if _, _, md := get("/api/v1/audit?format=markdown&from=2000-01-01&to=2000-01-02"); !strings.Contains(md, "- **From:** 2000-01-01 00:00:00\n- **To:** 2000-01-02 23:59:59\n- **Entries:** 0\n") ||
		!strings.Contains(md, "*(no rows)*") {
		t.Fatal(md)
	}
	if l := items(a.m("GET", "/api/v1/audit?format=json&limit=1", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/audit?format=pdf", nil, 400)
	a.m("GET", "/api/v1/audit?format=html", nil, 400)
}

func TestMarkdownWithoutBaseURL(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Server.BaseURL = "" })
	a := e.login("admin@example.com")
	pid := a.project("NOB")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "A | B", "description": "line 1\nline 2"}, 201)
	res := a.raw("GET", "/api/v1/projects/"+pid+"/export?format=md&aspects=requirements", nil)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	md := string(data)
	if !strings.Contains(md, "| NOB-REQ-1 | A \\| B |") || strings.Contains(md, "/go/") || !strings.Contains(md, "line 1<br>line 2") || !mdTablesOK(md) {
		t.Fatal(md)
	}
	if (&Server{}).itemLink(nil) != nil {
		t.Fatal("no base URL, no links")
	}
}
