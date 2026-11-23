package api

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestSolutionPreamble(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SOL")
	base := "/api/v1/projects/" + pid + "/solution-preamble"

	// empty at first; only software projects can save one
	if got := a.m("GET", base, nil, 200); got["filled"].(float64) != 0 || got["software"] != false || len(got["warnings"].([]any)) != 0 {
		t.Fatal(got)
	}
	a.m("PUT", base, map[string]any{"languages": "Go"}, 400)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"type": "software"}, 200)

	tpls := items(a.m("GET", base+"/templates", nil, 200))
	if len(tpls) < 6 || tpls[0]["id"] != "go-postgres-vue" {
		t.Fatal(tpls)
	}
	tpl := tpls[0]["preamble"].(map[string]any)
	tpl["template"] = "go-postgres-vue"
	for _, k := range []string{"version", "updated_at", "updated_by"} {
		delete(tpl, k)
	}
	got := a.m("PUT", base, tpl, 200)
	pre := got["preamble"].(map[string]any)
	if pre["version"].(float64) != 1 || pre["updated_by"] != "admin@example.com" || got["filled"].(float64) < 15 || got["software"] != true {
		t.Fatal(got)
	}

	// a credential is a warning (saved anyway), shown masked
	tpl["databases"] = "postgres://app:" + "Sup3r" + "-s3cret@db:5432/app"
	tpl["version"] = 1
	got = a.m("PUT", base, tpl, 200)
	w := got["warnings"].([]any)
	if len(w) != 1 || w[0].(map[string]any)["kind"] != "password_in_url" || w[0].(map[string]any)["field"] != "databases" ||
		strings.Contains(w[0].(map[string]any)["excerpt"].(string), "s3cret") {
		t.Fatal(w)
	}
	// check without saving
	if c := a.m("POST", base+"/check", map[string]any{"security": "password=" + "x9" + "-y8z7"}, 200); len(c["warnings"].([]any)) != 1 {
		t.Fatal(c)
	}
	// stale version → conflict; secret fields, unknown fields, bad values → 400
	a.m("PUT", base, map[string]any{"languages": "Go", "version": 1}, 409)
	for _, bad := range []map[string]any{
		{"db_password": "x"}, {"api_token": "x"}, {"connection_string": "x"}, {"colour": "blue"},
		{"architecture_style": "spaghetti"}, {"template": "cobol"}, {"languages": strings.Repeat("x", 4001)},
	} {
		a.m("PUT", base, bad, 400)
	}
	a.m("PUT", base, []any{"x"}, 400)
	a.m("PUT", base, map[string]any{"additional": strings.Repeat("x", 10000)}, 200)

	// history: one revision per save
	revs := items(a.m("GET", base+"/history", nil, 200))
	if len(revs) != 3 || revs[0]["entity_type"] != "solution_preamble" || revs[len(revs)-1]["action"] != "create" {
		t.Fatal(revs)
	}

	// permissions: reading needs project.read, saving project.update
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", base, nil, 200)
	v.m("PUT", base, map[string]any{"languages": "Go"}, 403)

	// the preamble is part of the project archive (a duplicate keeps it)
	a.m("PUT", base, map[string]any{"languages": "Go 1.26", "architecture_style": "microservices"}, 200)
	dup := a.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{"key": "SOLDUP"}, 201)
	did := dup["project"].(map[string]any)["id"].(string)
	dp := a.m("GET", "/api/v1/projects/"+did+"/solution-preamble", nil, 200)["preamble"].(map[string]any)
	if dp["languages"] != "Go 1.26" || dp["architecture_style"] != "microservices" {
		t.Fatal(dp)
	}
	if h := items(a.m("GET", "/api/v1/projects/"+did+"/solution-preamble/history", nil, 200)); len(h) != 4 {
		t.Fatalf("history of the duplicate: %d", len(h))
	}
}

func TestSolutionPrompts(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SP")
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"type": "software", "tech_context": "Monorepo"}, 200)
	a.m("PUT", "/api/v1/projects/"+pid+"/solution-preamble", map[string]any{"languages": "Go 1.26", "databases": "postgres://app:" + "Hun" + "ter-22@db/app",
		"architecture_style": "modular_monolith"}, 200)
	f1 := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"name": "Accounts", "kind": "requirement"}, 201)
	f2 := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"name": "Checkout", "kind": "requirement"}, 201)
	sub := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"name": "Cards", "kind": "requirement", "parent_id": f2["id"]}, 201)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in.", "obligation": "must",
		"folder_id": f1["id"], "create_test": true, "criteria": []map[string]any{{"description": "works", "threshold": "2 s"}}}, 201)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Pay by card", "description": "Cards SHOULD work.",
		"obligation": "should", "folder_id": sub["id"], "create_test": false}, 201)
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Guest checkout", "obligation": "may", "folder_id": f2["id"], "create_test": false}, 201)
	a.m("POST", "/api/v1/requirements/"+r2["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "depends_on"}, 201)
	risk := a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "Card fraud", "likelihood": 4, "impact": 5, "mitigation": "3-D Secure"}, 201)
	a.m("POST", "/api/v1/risks/"+risk["id"].(string)+"/requirements", map[string]any{"add": []string{r2["id"].(string)}}, 200)
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0"}, 201)
	a.m("POST", "/api/v1/releases/"+rel["id"].(string)+"/requirements", map[string]any{"add": []string{r1["id"].(string), r3["id"].(string)}}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/variants", map[string]any{"name": "EU"}, 201)
	url := "/api/v1/projects/" + pid + "/prompts"

	// the whole project
	res := a.m("POST", url, map[string]any{"kind": "full_solution"}, 200)
	txt := res["prompt"].(string)
	for _, want := range []string{"**Implementation languages and versions:** Go 1.26", "postgres://app:[REDACTED]@db/app", "modular monolith", "Monorepo",
		"Scope: the whole project — 3 requirements (of 3 in the project)", "Folders: Accounts (1), Checkout (2)", "Releases: 1.0 (planned)", "Variants: EU",
		"SP-RISK-1 Card fraud (level ", "mitigation: 3-D Secure", "Folder: Checkout / Cards", "SP-REQ-1 → SP-REQ-2", "Verified by:** SP-TC-1",
		"OpenAPI 3.1", "| Requirement |"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(txt, "ter-22") || res["items"].(float64) != 3 || res["tokens"].(float64) < 100 || len(res["parts"].([]any)) != 1 {
		t.Fatal(res["items"], res["tokens"])
	}

	// scopes: folder (with subfolders), release, variant, query, selected; in Swedish
	for _, c := range []struct {
		body  map[string]any
		items float64
		scope string
	}{
		{map[string]any{"kind": "architecture", "folder_id": f2["id"]}, 2, "folder Checkout"},
		{map[string]any{"kind": "design", "release_id": rel["id"]}, 2, "release 1.0"},
		{map[string]any{"kind": "design", "variant": "EU", "folder_id": f1["id"]}, 1, "folder Accounts · variant EU"},
		{map[string]any{"kind": "implementation_plan", "q": "obligation:must"}, 1, "query obligation:must"},
		{map[string]any{"kind": "implementation_plan", "requirement_ids": []any{r3["id"]}}, 1, "1 selected requirements"},
		{map[string]any{"kind": "architecture", "lang": "sv", "folder_id": "root"}, 0, "mapp —"},
	} {
		res := a.m("POST", url, c.body, 200)
		if res["items"].(float64) != c.items || !strings.Contains(res["prompt"].(string), c.scope) {
			t.Errorf("%v: %v items\n%s", c.body, res["items"], res["prompt"])
		}
	}
	// the folder scope also limits the existing kinds
	if res := a.m("POST", url, map[string]any{"kind": "implement", "folder_id": f1["id"]}, 200); res["items"].(float64) != 1 {
		t.Fatal(res)
	}

	// split per top-level folder, compact
	res = a.m("POST", url, map[string]any{"kind": "architecture", "split": "folder", "compact": true, "formats": []string{"mermaid"}}, 200)
	parts := res["parts"].([]any)
	if len(parts) != 2 || parts[0].(map[string]any)["label"] != "Accounts" || parts[1].(map[string]any)["items"].(float64) != 2 {
		t.Fatal(parts)
	}
	if p2 := parts[1].(map[string]any)["prompt"].(string); !strings.Contains(p2, "This is the last part (2 of 2)") || strings.Contains(p2, "Cards SHOULD work") ||
		!strings.Contains(p2, "Go 1.26") || strings.Contains(p2, "C4Context") {
		t.Fatal(p2)
	}
	// all parts as a ZIP archive
	resp := a.raw("POST", url, map[string]any{"kind": "architecture", "split": "folder", "format": "zip"})
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" || !strings.Contains(resp.Header.Get("Content-Disposition"), "SP-architecture-") {
		t.Fatal(resp.Status, resp.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) != 2 || !strings.HasSuffix(zr.File[1].Name, "-part-02.md") {
		t.Fatal(err, zr)
	}
	resp = a.raw("POST", url, map[string]any{"kind": "implement", "format": "zip"})
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil || len(zr.File) != 1 || strings.Contains(zr.File[0].Name, "part") {
		t.Fatal(err)
	}

	// validation
	for _, bad := range []map[string]any{
		{"kind": "design", "formats": []string{"pdf"}}, {"kind": "design", "split": "chapter"}, {"kind": "design", "max_tokens": 5},
		{"kind": "design", "format": "docx"}, {"kind": "design", "release_id": "nope"}, {"kind": "design", "variant": "Mars"},
		{"kind": "design", "folder_id": "nope"}, {"kind": "automate_tests", "release_id": rel["id"]},
	} {
		a.m("POST", url, bad, 400)
	}
	// an empty format list asks for Markdown only
	if txt := a.m("POST", url, map[string]any{"kind": "full_solution", "formats": []string{}}, 200)["prompt"].(string); strings.Contains(txt, "Mermaid") {
		t.Fatal("formats")
	}
	// without test.read the verifying tests and the test count stay out
	e.member("req@example.com", rbac.RoleViewer)
	a.m("PUT", "/api/v1/roles/viewer", map[string]any{"permissions": []string{"project.read", "requirement.read"}}, 200)
	v := e.login("req@example.com")
	if txt := v.m("POST", url, map[string]any{"kind": "design"}, 200)["prompt"].(string); strings.Contains(txt, "SP-TC-1") || !strings.Contains(txt, "(of 0 tests") {
		t.Fatal(txt)
	}
}
