package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type reportFixture struct {
	pid, otherPID, runID, otherRunID string
	reqKey, testKey                  string
}

// newReportFixture creates a project with requirements, tests (one with a
// call step), links and a run with results and defects.
func newReportFixture(t *testing.T, a *client) reportFixture {
	t.Helper()
	pid := a.project("RPT")
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"defect_url_template": "https://jira.example.com/browse/{id}"}, 200)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The system MUST log in.", "status": "approved",
		"criteria": []map[string]any{{"description": "valid login", "method": "test", "threshold": "1 s"}}}, 201)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout", "status": "draft", "tags": []string{"ui"}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Audit", "status": "approved"}, 201)
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "Suite"}, 201)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "folder_id": f["id"],
		"steps": []map[string]any{{"action": "open", "expected": "form"}}}, 201)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Wrong password", "execution_type": "automated", "script": "exit 1",
		"steps": []map[string]any{{"call_test_id": t1["id"]}, {"action": "type wrong", "expected": "error"}}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Orphan"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": t1["id"]}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": t2["id"]}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r2["id"], "test_case_id": t1["id"]}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Sprint 1", "environment": "staging", "build": "1.0",
		"test_ids": []any{t1["id"], t2["id"]}}, 201)
	runID := run["id"].(string)
	for _, ex := range items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200)) {
		body := map[string]any{"status": "passed", "duration_ms": 1200}
		if ex["test_case_id"] == t2["id"] {
			body = map[string]any{"status": "failed", "duration_ms": 800, "defects": []string{"BUG-7"}, "notes": "leaks account"}
		}
		a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), body, 200)
	}
	other := a.project("OTR")
	ot := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "x"}, 201)
	orun := a.m("POST", "/api/v1/projects/"+other+"/runs", map[string]any{"name": "Other", "test_ids": []any{ot["id"]}}, 201)
	return reportFixture{pid: pid, otherPID: other, runID: runID, otherRunID: orun["id"].(string),
		reqKey: r1["key"].(string), testKey: t2["key"].(string)}
}

// download posts a report request and returns the response body.
func (c *client) download(path string, body any, want int) (*http.Response, []byte) {
	c.e.t.Helper()
	res := c.raw("POST", path, body)
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("POST %s: status %d, want %d: %s", path, res.StatusCode, want, data)
	}
	return res, data
}

func zipFiles(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestReportTemplatesAndFields(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("FLD")
	var tmpls []map[string]any
	a.call("GET", "/api/v1/reports/templates", nil, 200, &tmpls)
	if len(tmpls) != 13 {
		t.Fatal(tmpls)
	}
	ids := map[string]bool{}
	for _, x := range tmpls {
		ids[x["id"].(string)] = true
		if len(x["formats"].([]any)) != 5 || x["params"] == nil {
			t.Fatal(x)
		}
	}
	for _, id := range []string{"project_status", "requirements_coverage", "traceability", "run_summary", "defects", "test_inventory", "risks", "compliance"} {
		if !ids[id] {
			t.Fatal("missing", id)
		}
	}
	for _, ent := range []string{"requirement", "test", "run", "execution"} {
		var fs []map[string]string
		a.call("GET", "/api/v1/projects/"+pid+"/reports/fields?entity="+ent, nil, 200, &fs)
		if len(fs) < 5 || fs[0]["id"] == "" || fs[0]["label"] == "" {
			t.Fatal(ent, fs)
		}
	}
	a.m("GET", "/api/v1/projects/"+pid+"/reports/fields?entity=nope", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/reports/fields", nil, 400)
	a.m("GET", "/api/v1/projects/missing/reports/fields?entity=test", nil, 404)
}

func TestGenerateTemplateReports(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	gen := "/api/v1/projects/" + fx.pid + "/reports/generate"
	magic := map[string]string{"pdf": "%PDF", "docx": "PK", "xlsx": "PK", "csv": "PK"}
	for _, tmpl := range []string{"project_status", "requirements_coverage", "traceability", "run_summary", "defects", "test_inventory", "risks"} {
		for _, f := range []string{"pdf", "docx", "xlsx", "csv"} {
			res, data := a.download(gen, map[string]any{"template": tmpl, "format": f, "params": map[string]string{"run_id": fx.runID}}, 200)
			if !bytes.HasPrefix(data, []byte(magic[f])) {
				t.Fatalf("%s/%s: %q", tmpl, f, data[:min(len(data), 20)])
			}
			cd := res.Header.Get("Content-Disposition")
			ext := f
			if f == "csv" {
				ext = "zip"
			}
			if !strings.HasPrefix(cd, `attachment; filename="`+tmpl+"-") || !strings.HasSuffix(cd, "."+ext+`"`) {
				t.Fatal(cd)
			}
		}
	}
	// content checks via CSV
	_, data := a.download(gen, map[string]any{"template": "requirements_coverage", "format": "csv", "params": map[string]string{"query": "status:approved"}}, 200)
	files := zipFiles(t, data)
	reqs := files["requirements.csv"]
	if !strings.Contains(reqs, fx.reqKey) || strings.Contains(reqs, "Logout") || !strings.Contains(reqs, "Failed") {
		t.Fatal(reqs)
	}
	if !strings.Contains(files["summary.csv"], "Requirements,2") {
		t.Fatal(files["summary.csv"])
	}
	_, data = a.download(gen, map[string]any{"template": "run_summary", "format": "csv", "params": map[string]string{"run_id": fx.runID}}, 200)
	files = zipFiles(t, data)
	if !strings.Contains(files["defects.csv"], "BUG-7,https://jira.example.com/browse/BUG-7,"+fx.testKey) || !strings.Contains(files["summary.csv"], "Total duration,2s") {
		t.Fatal(files)
	}
	_, data = a.download(gen, map[string]any{"template": "defects", "format": "csv"}, 200)
	if !strings.Contains(zipFiles(t, data)["defects.csv"], "BUG-7") {
		t.Fatal("defects")
	}
	_, data = a.download(gen, map[string]any{"template": "traceability", "format": "csv", "params": map[string]string{"query": "execution_type:automated"}}, 200)
	if m := zipFiles(t, data)["matrix.csv"]; strings.Count(m, "Valid login") != 0 || !strings.Contains(m, "Wrong password") {
		t.Fatal(m)
	}
	_, data = a.download(gen, map[string]any{"template": "test_inventory", "format": "csv", "params": map[string]string{"query": "folder:Suite"}}, 200)
	if l := zipFiles(t, data)["tests.csv"]; !strings.Contains(l, "Valid login") || strings.Contains(l, "Orphan") {
		t.Fatal(l)
	}

	// errors
	a.m("POST", gen, map[string]any{"template": "run_summary", "format": "pdf"}, 400)
	a.m("POST", gen, map[string]any{"template": "run_summary", "format": "pdf", "params": map[string]string{"run_id": fx.otherRunID}}, 400)
	a.m("POST", gen, map[string]any{"template": "run_summary", "format": "pdf", "params": map[string]string{"run_id": "missing"}}, 400)
	a.m("POST", gen, map[string]any{"template": "nope", "format": "pdf"}, 400)
	a.m("POST", gen, map[string]any{"template": "defects", "format": "odt"}, 400)
	a.m("POST", gen, map[string]any{"format": "pdf"}, 400)
	a.m("POST", gen, map[string]any{"template": "defects", "custom": map[string]any{"entity": "test", "columns": []string{"key"}}, "format": "pdf"}, 400)
	if m := a.m("POST", gen, map[string]any{"template": "test_inventory", "format": "pdf", "params": map[string]string{"query": "nope:x"}}, 400); m["code"] != "invalid_query" {
		t.Fatal(m)
	}
	a.m("POST", gen, "{", 400)
	a.m("POST", "/api/v1/projects/missing/reports/generate", map[string]any{"template": "defects", "format": "pdf"}, 404)

	// archive
	var fi map[string]any
	a.call("POST", gen, map[string]any{"template": "project_status", "format": "xlsx", "archive": true}, 201, &fi)
	name := fi["name"].(string)
	if fi["kind"] != "report" || fi["project_id"] != fx.pid || !strings.HasPrefix(name, "project_status-") || !strings.HasSuffix(name, ".xlsx") ||
		fi["url"] != "/api/v1/files/"+fi["id"].(string)+"/content" || fi["content_type"] != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatal(fi)
	}
	blob, err := e.st.FileData(context.Background(), fi["id"].(string))
	if err != nil || !bytes.HasPrefix(blob, []byte("PK")) || int64(len(blob)) != int64(fi["size"].(float64)) {
		t.Fatal(err, len(blob))
	}
	a.call("POST", gen, map[string]any{"template": "defects", "format": "pdf", "archive": true, "name": "Weekly défects"}, 201, &fi)
	if fi["name"] != "Weekly défects.pdf" {
		t.Fatal(fi["name"])
	}
	a.call("POST", gen, map[string]any{"template": "defects", "format": "csv", "archive": true, "name": "d.zip"}, 201, &fi)
	if fi["name"] != "d.zip" || fi["content_type"] != "application/zip" {
		t.Fatal(fi)
	}
	// a download with a non-ASCII name
	res, _ := a.download(gen, map[string]any{"template": "defects", "format": "pdf", "name": `Rapport "ä"`}, 200)
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="Rapport _.pdf"; filename*=UTF-8''Rapport%20%C3%A4.pdf` {
		t.Fatal(cd)
	}
	// audit both outcomes
	entries, err := e.st.ListAudit(context.Background(), e.tenant.ID, 500)
	if err != nil {
		t.Fatal(err)
	}
	acts := map[string]int{}
	for _, en := range entries {
		acts[en.Action]++
	}
	if acts["report.generate"] == 0 || acts["report.archive"] != 3 {
		t.Fatal(acts)
	}
	// viewers hold the export permission
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.download(gen, map[string]any{"template": "project_status", "format": "pdf"}, 200)
}

func TestGenerateCustomReports(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	gen := "/api/v1/projects/" + fx.pid + "/reports/generate"
	custom := func(c map[string]any) map[string]any { return map[string]any{"custom": c, "format": "csv"} }

	_, data := a.download(gen, custom(map[string]any{"entity": "requirement", "columns": []string{"key", "title", "criteria", "status"},
		"group_by": "status", "sort_by": "key", "include_summary": true, "title": "By status"}), 200)
	files := zipFiles(t, data)
	if len(files) != 3 || !strings.Contains(files["summary.csv"], "approved,2") || !strings.Contains(files["approved.csv"], "1. valid login (test; 1 s)") ||
		!strings.Contains(files["draft.csv"], "Logout") {
		t.Fatal(files)
	}
	// a single section is plain CSV
	res, data := a.download(gen, custom(map[string]any{"entity": "test", "columns": []string{"key", "step", "last_result"}, "query": "type:automated"}), 200)
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") || !strings.HasPrefix(res.Header.Get("Content-Disposition"), `attachment; filename="custom_report-`) {
		t.Fatal(ct)
	}
	if s := string(data); !strings.Contains(s, "[call RPT-TC-1]") || !strings.Contains(s, "2. type wrong => error") || !strings.Contains(s, "failed") || strings.Contains(s, "Valid login") {
		t.Fatal(s)
	}
	_, data = a.download(gen, custom(map[string]any{"entity": "run", "columns": []string{"name", "total", "passed", "failed", "pass_rate", "started_at"}}), 200)
	if s := string(data); !strings.Contains(s, "Sprint 1,2,1,1,50.0%,") {
		t.Fatal(s)
	}
	_, data = a.download(gen, custom(map[string]any{"entity": "execution", "columns": []string{"key", "status", "duration_ms", "defect", "run"},
		"run_id": fx.runID, "sort_by": "duration_ms", "sort_desc": true, "query": "status:passed OR status:failed"}), 200)
	lines := strings.Split(strings.TrimSpace(string(data)), "\r\n")
	if len(lines) != 3 || !strings.Contains(lines[1], ",passed,1200,,Sprint 1") || !strings.Contains(lines[2], "failed,800,BUG-7,Sprint 1") {
		t.Fatal(lines)
	}
	// executions of all runs of the project
	_, data = a.download(gen, custom(map[string]any{"entity": "execution", "columns": []string{"key", "run"}}), 200)
	if strings.Count(string(data), "Sprint 1") != 2 || strings.Contains(string(data), "Other") {
		t.Fatal(string(data))
	}
	for _, f := range []string{"pdf", "docx", "xlsx"} {
		a.download(gen, map[string]any{"custom": map[string]any{"entity": "requirement", "columns": []string{"key"}, "group_by": "tag", "include_summary": true}, "format": f}, 200)
	}
	var fi map[string]any
	a.call("POST", gen, map[string]any{"custom": map[string]any{"entity": "test", "columns": []string{"key"}}, "format": "pdf", "archive": true}, 201, &fi)
	if !strings.HasPrefix(fi["name"].(string), "custom_report-") || fi["description"] != "Custom report – Test cases" {
		t.Fatal(fi)
	}

	// validation
	for _, c := range []map[string]any{
		{"entity": "nope", "columns": []string{"key"}},
		{"entity": "test", "columns": []string{}},
		{"entity": "test", "columns": []string{"nope"}},
		{"entity": "test", "columns": []string{"key"}, "group_by": "nope"},
		{"entity": "test", "columns": []string{"key"}, "sort_by": "nope"},
		{"entity": "test", "columns": []string{"key"}, "run_id": fx.runID},
		{"entity": "execution", "columns": []string{"key"}, "run_id": fx.otherRunID},
		{"entity": "execution", "columns": []string{"key"}, "run_id": "missing"},
		{"entity": "run", "columns": []string{"name"}, "query": "nope:x"},
	} {
		a.m("POST", gen, custom(c), 400)
	}
	a.m("POST", gen, map[string]any{"custom": "x", "format": "csv"}, 400)
}

func TestReportDefinitions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	base := "/api/v1/projects/" + fx.pid + "/reports/definitions"
	cfg := map[string]any{"entity": "requirement", "query": "status:approved", "columns": []string{"key", "title"}, "unknown": 1}
	d := a.m("POST", base, map[string]any{"name": " Approved ", "description": " d ", "config": cfg}, 201)
	id := d["id"].(string)
	if d["name"] != "Approved" || d["description"] != "d" || d["created_by"] != "admin@example.com" || d["project_id"] != fx.pid {
		t.Fatal(d)
	}
	var stored map[string]any
	b, _ := json.Marshal(d["config"])
	json.Unmarshal(b, &stored)
	if stored["entity"] != "requirement" || stored["unknown"] != nil || stored["include_summary"] != false {
		t.Fatal(stored)
	}
	a.m("POST", base, map[string]any{"name": "Approved", "config": cfg}, 409)
	a.m("POST", base, map[string]any{"name": "", "config": cfg}, 400)
	a.m("POST", base, map[string]any{"name": "x"}, 400)
	a.m("POST", base, map[string]any{"name": "x", "config": map[string]any{"entity": "test", "columns": []string{"nope"}}}, 400)
	a.m("POST", base, map[string]any{"name": "x", "config": map[string]any{"entity": "test", "columns": []string{"key"}, "query": "bad:x"}}, 400)
	a.m("POST", base, "{", 400)
	a.m("POST", "/api/v1/projects/missing/reports/definitions", map[string]any{"name": "x", "config": cfg}, 404)

	if l := items(a.m("GET", base, nil, 200)); len(l) != 1 || l[0]["id"] != id {
		t.Fatal(l)
	}
	if g := a.m("GET", "/api/v1/reports/definitions/"+id, nil, 200); g["name"] != "Approved" {
		t.Fatal(g)
	}
	a.m("GET", "/api/v1/reports/definitions/missing", nil, 404)
	up := a.m("PATCH", "/api/v1/reports/definitions/"+id, map[string]any{"name": "Approved reqs", "config": map[string]any{"entity": "execution", "columns": []string{"key", "status"}, "run_id": fx.runID}}, 200)
	if up["name"] != "Approved reqs" {
		t.Fatal(up)
	}
	a.m("PATCH", "/api/v1/reports/definitions/"+id, map[string]any{"description": "only description"}, 200)
	a.m("PATCH", "/api/v1/reports/definitions/"+id, map[string]any{"config": map[string]any{"entity": "execution", "columns": []string{"key"}, "run_id": fx.otherRunID}}, 400)
	a.m("PATCH", "/api/v1/reports/definitions/"+id, map[string]any{"name": " "}, 400)
	a.m("PATCH", "/api/v1/reports/definitions/"+id, "{", 400)
	a.m("PATCH", "/api/v1/reports/definitions/missing", map[string]any{"name": "x"}, 404)

	// generate a saved definition
	gen := "/api/v1/reports/definitions/" + id + "/generate"
	res, data := a.download(gen, map[string]any{"format": "csv"}, 200)
	if !strings.Contains(string(data), "Test key,Result") || !strings.Contains(string(data), ",failed") ||
		!strings.HasPrefix(res.Header.Get("Content-Disposition"), `attachment; filename="Approved reqs-`) {
		t.Fatal(string(data), res.Header)
	}
	var fi map[string]any
	a.call("POST", gen, map[string]any{"format": "pdf", "archive": true, "name": "saved"}, 201, &fi)
	if fi["name"] != "saved.pdf" || fi["description"] != "Approved reqs" {
		t.Fatal(fi)
	}
	a.m("POST", gen, map[string]any{"format": "nope"}, 400)
	a.m("POST", gen, "{", 400)
	a.m("POST", "/api/v1/reports/definitions/missing/generate", map[string]any{"format": "pdf"}, 404)

	// permissions: viewers read and generate, testers (test.write) manage
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", base, nil, 200)
	v.download(gen, map[string]any{"format": "csv"}, 200)
	v.m("POST", base, map[string]any{"name": "v", "config": cfg}, 403)
	v.m("PATCH", "/api/v1/reports/definitions/"+id, map[string]any{"name": "v"}, 403)
	v.m("DELETE", "/api/v1/reports/definitions/"+id, nil, 403)
	e.member("tester@example.com", "tester")
	tc := e.login("tester@example.com")
	td := tc.m("POST", base, map[string]any{"name": "Tester's", "config": map[string]any{"entity": "test", "columns": []string{"key"}}}, 201)
	tc.m("DELETE", "/api/v1/reports/definitions/"+td["id"].(string), nil, 204)

	// another tenant cannot see it
	ctx := context.Background()
	ten2, _ := e.st.CreateTenant(ctx, "other", "Other")
	u2 := mkUser(t, e.st, "other@example.com", false)
	e.st.SetMembership(ctx, ten2.ID, u2.ID, "admin")
	o := e.login("other@example.com")
	o.m("GET", "/api/v1/reports/definitions/"+id, nil, 404)
	o.m("POST", "/api/v1/reports/definitions/"+id+"/generate", map[string]any{"format": "pdf"}, 404)

	a.m("DELETE", "/api/v1/reports/definitions/"+id, nil, 204)
	a.m("DELETE", "/api/v1/reports/definitions/"+id, nil, 404)
	if l := items(a.m("GET", base, nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/missing/reports/definitions", nil, 404)
}

func TestReportDefinitionStoredConfigBroken(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BRK")
	d := a.m("POST", "/api/v1/projects/"+pid+"/reports/definitions", map[string]any{"name": "x", "config": map[string]any{"entity": "test", "columns": []string{"key"}}}, 201)
	// a config that became invalid (e.g. written by another version)
	if _, err := e.st.DB.Exec(e.st.DB.Rebind(`UPDATE report_definitions SET config = ? WHERE id = ?`), `{"entity":"gone"}`, d["id"]); err != nil {
		t.Fatal(err)
	}
	a.m("POST", "/api/v1/reports/definitions/"+d["id"].(string)+"/generate", map[string]any{"format": "pdf"}, 400)
	// the definition's project was deleted
	pid2 := a.project("BRK2")
	d2 := a.m("POST", "/api/v1/projects/"+pid2+"/reports/definitions", map[string]any{"name": "y", "config": map[string]any{"entity": "test", "columns": []string{"key"}}}, 201)
	a.m("DELETE", "/api/v1/projects/"+pid2, nil, 204)
	a.m("GET", "/api/v1/reports/definitions/"+d2["id"].(string), nil, 404)
	if reportError(errBadRequest) != errBadRequest {
		t.Fatal("reportError passthrough")
	}
}
