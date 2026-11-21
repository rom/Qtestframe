package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadProjectAPI(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ARC")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Keep me"}, 201)["id"].(string)
	tid := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Check it", "steps": []map[string]any{{"action": "a", "expected": "b"}}}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": tid}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Run 1", "test_ids": []string{tid}}, 201)
	exid := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+exid, map[string]any{"status": "passed"}, 200)
	a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "spec.txt", "text/plain", []byte("the spec"), nil, 201)
	const secret, token = "hook-secret-123456", "tracker-token-987654"
	a.m("POST", "/api/v1/projects/"+pid+"/webhooks", map[string]any{"name": "h", "url": "https://example.com/h", "secret": secret, "events": []string{"requirement.created"}, "enabled": true}, 201)
	a.m("PUT", "/api/v1/projects/"+pid+"/defect-tracker", map[string]any{"type": "github", "repo": "acme/app", "token": token}, 200)

	res, archive := a.getBytes("/api/v1/projects/"+pid+"/archive", 200)
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatal(ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="arc-project-`) {
		t.Fatal(cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if bytes.Contains(data, []byte(secret)) || bytes.Contains(data, []byte(token)) {
			t.Fatalf("%s contains a secret", f.Name)
		}
	}
	if !names["manifest.json"] || !names["tables/requirements.jsonl"] || !names["tables/test_executions.jsonl"] {
		t.Fatal(names)
	}
	if acts := auditActions(t, e); !acts["project.save"] {
		t.Fatal(acts)
	}

	// dry run: the archive's key is taken, another key is fine
	_ = a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"dry_run": "true"}, 409)
	dry := a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"dry_run": "true", "key": "NEW"}, 200)
	if dry["dry_run"] != true || dry["manifest"].(map[string]any)["project"].(map[string]any)["key"] != "ARC" {
		t.Fatal(dry)
	}
	if ps := items(a.m("GET", "/api/v1/projects", nil, 200)); len(ps) != 1 {
		t.Fatal("dry run created a project")
	}
	msg := a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, nil, 409)
	if msg["code"] != "conflict" || !strings.Contains(msg["error"].(string), "ARC already exists") {
		t.Fatal(msg)
	}
	out := a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"key": "new", "name": "Loaded"}, 201)
	np := out["project"].(map[string]any)
	npid := np["id"].(string)
	if np["key"] != "NEW" || np["name"] != "Loaded" || npid == pid {
		t.Fatal(np)
	}
	reqs := items(a.m("GET", "/api/v1/projects/"+npid+"/requirements", nil, 200))
	if len(reqs) != 1 || reqs[0]["key"] != "NEW-REQ-1" || reqs[0]["id"] == rid {
		t.Fatal(reqs)
	}
	nrid := reqs[0]["id"].(string)
	links := items(a.m("GET", "/api/v1/projects/"+npid+"/links", nil, 200))
	if len(links) != 1 || links[0]["requirement_id"] != nrid || links[0]["test_case_id"] == tid {
		t.Fatal(links)
	}
	runs := items(a.m("GET", "/api/v1/projects/"+npid+"/runs", nil, 200))
	if len(runs) != 1 || runs[0]["id"] == run["id"] {
		t.Fatal(runs)
	}
	ex := items(a.m("GET", "/api/v1/runs/"+runs[0]["id"].(string)+"/executions", nil, 200))
	if len(ex) != 1 || ex[0]["status"] != "passed" || ex[0]["id"] == exid {
		t.Fatal(ex)
	}
	att := items(a.m("GET", "/api/v1/requirements/"+nrid+"/attachments", nil, 200))
	if len(att) != 1 {
		t.Fatal(att)
	}
	_, content := a.getBytes(att[0]["url"].(string), 200)
	if string(content) != "the spec" {
		t.Fatal(string(content))
	}
	hooks := items(a.m("GET", "/api/v1/projects/"+npid+"/webhooks", nil, 200))
	if len(hooks) != 1 || hooks[0]["enabled"] != false || hooks[0]["has_secret"] != false {
		t.Fatal(hooks)
	}
	if tr := a.m("GET", "/api/v1/projects/"+npid+"/defect-tracker", nil, 200); tr["has_token"] != false || tr["repo"] != "acme/app" {
		t.Fatal(tr)
	}
	// the original keeps its rows
	if r := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200)); len(r) != 1 || r[0]["id"] != rid {
		t.Fatal(r)
	}
	// duplicate
	dup := a.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{"key": "dup"}, 201)
	if p := dup["project"].(map[string]any); p["key"] != "DUP" || p["name"] != "ARC project" {
		t.Fatal(p)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{"key": "DUP"}, 409)
	a.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{}, 400)
	if acts := auditActions(t, e); !acts["project.load"] || !acts["project.duplicate"] {
		t.Fatal(acts)
	}

	// newer schema, wrong files, bad requests
	newer := editManifest(t, archive, func(m map[string]any) { m["schema_version"] = 99999 })
	msg = a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", newer, map[string]string{"key": "NEWER"}, 409)
	if !strings.Contains(msg["error"].(string), "newer version") {
		t.Fatal(msg)
	}
	_, backup := a.getBytes("/api/v1/admin/backup", 200)
	a.upload("POST", "/api/v1/projects/import", "archive", "b.zip", "application/zip", backup, map[string]string{"key": "BK"}, 400)
	a.upload("POST", "/api/v1/projects/import", "archive", "x.zip", "application/zip", []byte("not a zip"), map[string]string{"key": "BK"}, 400)
	a.upload("POST", "/api/v1/projects/import", "", "", "", nil, map[string]string{"key": "BK"}, 400)
	a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"key": "BK", "dry_run": "maybe"}, 400)
	a.m("POST", "/api/v1/projects/import", map[string]any{"key": "X"}, 400) // not multipart
	a.getBytes("/api/v1/projects/00000000-0000-0000-0000-000000000000/archive", 404)
}

func TestProjectArchivePermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PRM")
	_, archive := a.getBytes("/api/v1/projects/"+pid+"/archive", 200)
	e.member("viewer@example.com", "viewer")
	e.member("tester@example.com", "tester")
	e.member("manager@example.com", "manager")
	for _, email := range []string{"viewer@example.com", "tester@example.com"} {
		c := e.login(email)
		c.getBytes("/api/v1/projects/"+pid+"/archive", 403)
		c.upload("POST", "/api/v1/projects/import", "archive", "a.zip", "application/zip", archive, map[string]string{"key": "NOPE"}, 403)
		c.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{"key": "NOPE"}, 403)
	}
	m := e.login("manager@example.com")
	m.getBytes("/api/v1/projects/"+pid+"/archive", 200)
	m.upload("POST", "/api/v1/projects/import", "archive", "a.zip", "application/zip", archive, map[string]string{"key": "MGR"}, 201)
	// an API token with the tester role cannot save either
	tok := m.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "role": "tester"}, 201)
	bt := &client{e: e, hc: http.DefaultClient, bearer: tok["token"].(string)}
	bt.getBytes("/api/v1/projects/"+pid+"/archive", 403)
	if got := archiveFileName("ABC", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); got != "abc-project-20260102-030405.zip" {
		t.Fatal(got)
	}
}
