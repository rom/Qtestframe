package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/rom/qtestframe/internal/rbac"
)

func (c *client) get(path string, want int) []byte {
	c.e.t.Helper()
	res := c.raw("GET", path, nil)
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("GET %s: status %d, want %d: %s", path, res.StatusCode, want, data)
	}
	return data
}

// sheetEdit opens a workbook, lets edit change it and returns the bytes.
func sheetEdit(t *testing.T, data []byte, edit func(f *excelize.File)) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	edit(f)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// colOf returns the column letter of header in row 1 of sheet.
func colOf(t *testing.T, f *excelize.File, sheet, header string) string {
	t.Helper()
	rows, _ := f.GetRows(sheet)
	for i, h := range rows[0] {
		if h == header {
			n, _ := excelize.ColumnNumberToName(i + 1)
			return n
		}
	}
	t.Fatalf("no column %q in %v", header, rows[0])
	return ""
}

// rowOf returns the sheet row of key.
func rowOf(t *testing.T, f *excelize.File, sheet, key string) string {
	t.Helper()
	rows, _ := f.GetRows(sheet)
	for i, r := range rows {
		if len(r) > 1 && r[1] == key {
			return itoa(i + 1)
		}
	}
	t.Fatalf("no row %q", key)
	return ""
}

func summaryOf(m map[string]any) map[string]any { return m["summary"].(map[string]any) }

func rowsOf(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, x := range m["rows"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestExchangeRequirementRoundTrip(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RT")
	base := "/api/v1/projects/" + pid
	sec := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Security"}, 201)
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Login", "description": "Users SHALL log in.", "folder_id": sec["id"], "tags": []string{"auth"},
		"criteria": []map[string]any{{"description": "Login works", "method": "test"}}}, 201)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Logout", "description": "Users SHOULD log out."}, 201)
	r3 := a.m("POST", base+"/requirements", map[string]any{"title": "Audit", "description": "The system MUST log events."}, 201)
	a.m("POST", "/api/v1/requirements/"+r1["id"].(string)+"/relations", map[string]any{"target_id": r2["id"], "type": "refines"}, 201)
	k1, k2, k3 := r1["key"].(string), r2["key"].(string), r3["key"].(string)

	wb := a.get(base+"/exchange/xlsx?kind=requirement", 200)
	edited := sheetEdit(t, wb, func(f *excelize.File) {
		sh := "Requirements"
		if v, _ := f.GetCellValue(sh, colOf(t, f, sh, "Folder")+rowOf(t, f, sh, k1)); v != "Security" {
			t.Errorf("folder cell: %q", v)
		}
		if v, _ := f.GetCellValue(sh, colOf(t, f, sh, "Refines")+rowOf(t, f, sh, k1)); v != k2 {
			t.Errorf("refines cell: %q", v)
		}
		// r1: new title, refines r3 instead of r2
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Title")+rowOf(t, f, sh, k1), "Sign in")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Refines")+rowOf(t, f, sh, k1), k3)
		// r2: edited here and (below) by someone else: a conflict
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Description")+rowOf(t, f, sh, k2), "Users MUST log out.")
		// r3: an invalid priority
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Priority")+rowOf(t, f, sh, k3), "urgentissimo")
		// a new requirement in a new folder
		_ = f.SetCellValue(sh, "B10", "NEW-1")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Title")+"10", "Password reset")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Description")+"10", "Users SHALL be able to reset their password.")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Folder")+"10", "Security/Passwords")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Priority")+"10", "Hög")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Depends on")+"10", k1)
	})
	// someone else saves r2 meanwhile
	a.m("PATCH", "/api/v1/requirements/"+r2["id"].(string), map[string]any{"rationale": "Shared terminals"}, 200)

	up := func(path string, extra map[string]string, want int) map[string]any {
		return a.upload("POST", base+"/exchange/xlsx/"+path, "file", "rt.xlsx", "", edited, extra, want)
	}
	pv := up("preview", nil, 200)
	sum := summaryOf(pv)
	if sum["new"].(float64) != 1 || sum["changed"].(float64) != 1 || sum["conflicts"].(float64) != 1 || sum["errors"].(float64) != 1 {
		t.Fatalf("summary: %v\n%v", sum, pv["rows"])
	}
	for _, r := range rowsOf(pv) {
		switch r["key"] {
		case k1:
			if r["status"] != "changed" || len(r["changes"].([]any)) != 2 {
				t.Errorf("r1: %v", r)
			}
		case k3:
			errs := r["errors"].([]any)
			if r["status"] != "error" || len(errs) != 1 || errs[0].(map[string]any)["field"] != "priority" || errs[0].(map[string]any)["cell"] == "" {
				t.Errorf("r3: %v", r)
			}
		}
	}
	// nothing saved by the preview
	if got := a.m("GET", "/api/v1/requirements/"+r1["id"].(string), nil, 200); got["title"] != "Login" {
		t.Fatalf("preview saved: %v", got["title"])
	}

	res := up("apply", nil, 200)
	if res["created"].(float64) != 1 || res["updated"].(float64) != 1 || res["skipped"].(float64) != 2 || res["folders_created"].(float64) != 1 ||
		res["relations_added"].(float64) != 2 || res["relations_removed"].(float64) != 1 {
		t.Fatalf("apply: %v", res)
	}
	got := a.m("GET", "/api/v1/requirements/"+r1["id"].(string), nil, 200)
	if got["title"] != "Sign in" || got["version"].(float64) != 2 {
		t.Errorf("r1 after apply: %v", got)
	}
	rels := items(a.m("GET", base+"/relations", nil, 200))
	if len(rels) != 2 {
		t.Errorf("relations: %v", rels)
	}
	hist := a.m("GET", "/api/v1/requirements/"+r1["id"].(string)+"/history", nil, 200)
	if len(items(hist)) < 2 {
		t.Errorf("history: %v", hist)
	}
	var newReq map[string]any
	for _, rq := range items(a.m("GET", base+"/requirements", nil, 200)) {
		if rq["title"] == "Password reset" {
			newReq = rq
		}
	}
	if newReq == nil || newReq["folder_path"] != "Security / Passwords" || newReq["priority"] != "high" || newReq["obligation"] != "shall" {
		t.Fatalf("new requirement: %v", newReq)
	}
	audit := a.m("GET", "/api/v1/audit?action=requirement.import", nil, 200)
	if !strings.Contains(string(mustJSON(audit)), "xlsx round-trip") {
		t.Errorf("audit: %v", audit)
	}

	// the same workbook again: r1 is now based on an old version
	pv = up("preview", nil, 200)
	if sum := summaryOf(pv); sum["conflicts"].(float64) != 1 || sum["new"].(float64) != 1 {
		t.Errorf("second preview: %v", sum)
	}
	// overwriting conflicts saves r2; the new row is left out
	res = up("apply", map[string]string{"overwrite_conflicts": "true", "create": "false", "exclude": "[2]"}, 200)
	if res["updated"].(float64) != 1 || res["created"].(float64) != 0 {
		t.Fatalf("overwrite: %v", res)
	}
	if got := a.m("GET", "/api/v1/requirements/"+r2["id"].(string), nil, 200); got["description"] != "Users MUST log out." {
		t.Errorf("r2 overwritten: %v", got["description"])
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestExchangeTestsWithSteps(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TS")
	base := "/api/v1/projects/" + pid
	rq := a.m("POST", base+"/requirements", map[string]any{"title": "Login"}, 201)
	rq2 := a.m("POST", base+"/requirements", map[string]any{"title": "Logout"}, 201)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "Login test", "steps": []map[string]any{{"action": "Open page", "expected": "Form shown"}, {"action": "Sign in", "expected": "Welcome"}}}, 201)
	helper := a.m("POST", base+"/tests", map[string]any{"title": "Helper", "steps": []map[string]any{{"action": "Prepare"}}}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": rq["id"], "test_case_id": tc["id"]}, 201)

	wb := a.get(base+"/exchange/xlsx?kind=test", 200)
	edited := sheetEdit(t, wb, func(f *excelize.File) {
		sh := "Tests"
		row := rowOf(t, f, sh, tc["key"].(string))
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Verifies")+row, rq["key"].(string)+", "+rq2["key"].(string))
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Estimated minutes")+row, 15)
		// steps: change the second, add a call step
		_ = f.SetCellValue("Steps", "F3", "Welcome page")
		_ = f.SetSheetRow("Steps", "A5", &[]any{tc["id"], tc["key"], 3, "", "", "", helper["key"]})
		// a new test with one step
		_ = f.SetCellValue(sh, "B5", "T-NEW")
		_ = f.SetCellValue(sh, colOf(t, f, sh, "Title")+"5", "Logout test")
		_ = f.SetSheetRow("Steps", "A6", &[]any{"", "T-NEW", 1, "Click logout", "", "Signed out"})
	})
	pv := a.upload("POST", base+"/exchange/xlsx/preview", "file", "t.xlsx", "", edited, nil, 200)
	if s := summaryOf(pv); s["changed"].(float64) != 1 || s["new"].(float64) != 1 || s["unchanged"].(float64) != 1 {
		t.Fatalf("preview: %v %v", s, pv["rows"])
	}
	res := a.upload("POST", base+"/exchange/xlsx/apply", "file", "t.xlsx", "", edited, nil, 200)
	if res["links_added"].(float64) != 1 || res["created"].(float64) != 1 {
		t.Fatalf("apply: %v", res)
	}
	got := a.m("GET", "/api/v1/tests/"+tc["id"].(string), nil, 200)
	steps := got["steps"].([]any)
	if len(steps) != 3 || steps[1].(map[string]any)["expected"] != "Welcome page" || steps[2].(map[string]any)["call_test_id"] != helper["id"] || got["estimated_minutes"].(float64) != 15 {
		t.Errorf("test after apply: %v", got)
	}
	var created map[string]any
	for _, x := range items(a.m("GET", base+"/tests", nil, 200)) {
		if x["title"] == "Logout test" {
			created = x
		}
	}
	if created == nil {
		t.Fatal("new test not created")
	}
	if st := a.m("GET", "/api/v1/tests/"+created["id"].(string), nil, 200)["steps"].([]any); len(st) != 1 {
		t.Errorf("new test steps: %v", st)
	}
	// an unknown call target is a cell error
	bad := sheetEdit(t, edited, func(f *excelize.File) { _ = f.SetCellValue("Steps", "G5", "TS-TC-999") })
	pv = a.upload("POST", base+"/exchange/xlsx/preview", "file", "t.xlsx", "", bad, nil, 200)
	if s := summaryOf(pv); s["errors"].(float64) != 1 {
		t.Errorf("bad call: %v", s)
	}
}

func TestExchangePermissionsAndErrors(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PE")
	base := "/api/v1/projects/" + pid
	a.m("POST", base+"/requirements", map[string]any{"title": "One"}, 201)
	wb := a.get(base+"/exchange/xlsx", 200)
	a.get(base+"/exchange/xlsx?kind=nope", 400)

	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.upload("POST", base+"/exchange/xlsx/preview", "file", "x.xlsx", "", wb, nil, 403)
	v.upload("POST", base+"/exchange/reqif/preview", "file", "x.reqif", "", []byte("<REQ-IF/>"), nil, 403)

	a.upload("POST", base+"/exchange/xlsx/preview", "file", "x.xlsx", "", []byte("not a workbook"), nil, 400)
	// a workbook without the round-trip sheets
	f := excelize.NewFile()
	var buf bytes.Buffer
	_ = f.Write(&buf)
	a.upload("POST", base+"/exchange/xlsx/preview", "file", "x.xlsx", "", buf.Bytes(), nil, 400)
	// the requirement workbook as tests
	a.upload("POST", base+"/exchange/xlsx/preview", "file", "x.xlsx", "", wb, map[string]string{"kind": "test"}, 400)
	// another project's workbook: its IDs are errors
	other := a.project("PX")
	pv := a.upload("POST", "/api/v1/projects/"+other+"/exchange/xlsx/preview", "file", "x.xlsx", "", wb, nil, 200)
	if summaryOf(pv)["errors"].(float64) != 1 || len(pv["warnings"].([]any)) == 0 {
		t.Errorf("other project: %v", pv)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../interop/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReqIFImportDOORS(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DR")
	base := "/api/v1/projects/" + pid
	doc := readFixture(t, "doors.reqif")
	pv := a.upload("POST", base+"/exchange/reqif/preview", "file", "sysreq.reqif", "", doc, nil, 200)
	if s := summaryOf(pv); s["new"].(float64) != 3 || s["errors"].(float64) != 0 {
		t.Fatalf("preview: %v %v", s, pv["rows"])
	}
	if pv["folders"].(float64) != 3 || pv["tool"] != "DOORS 9.7.2" {
		t.Errorf("folders / tool: %v %v", pv["folders"], pv["tool"])
	}
	attrs := map[string]string{}
	for _, x := range pv["attributes"].([]any) {
		m := x.(map[string]any)
		attrs[m["name"].(string)] = m["field"].(string) + "/" + m["type"].(string)
	}
	if attrs["ReqIF.Text"] != "description/xhtml" || attrs["ReqIF.ChapterName"] != "heading/xhtml" || attrs["ReqIF.ForeignID"] != "external_id/string" ||
		attrs["Priority"] != "priority/enumeration" || attrs["Req Status"] != "status/enumeration" || attrs["ReqIF.ForeignModifiedOn"] != "/date" {
		t.Errorf("attributes: %v", attrs)
	}
	if rels := pv["relations"].(map[string]any); rels["add"].(float64) != 2 {
		t.Errorf("relations: %v", rels)
	}
	res := a.upload("POST", base+"/exchange/reqif/apply", "file", "sysreq.reqif", "", doc, nil, 200)
	if res["created"].(float64) != 3 || res["relations_added"].(float64) != 2 || res["folders_created"].(float64) != 3 || res["references_saved"].(float64) != 3 {
		t.Fatalf("apply: %v", res)
	}
	reqs := items(a.m("GET", base+"/requirements", nil, 200))
	byTitle := map[string]map[string]any{}
	for _, r := range reqs {
		byTitle[r["title"].(string)] = r
	}
	emerg := byTitle["The system SHALL apply full braking force when…"]
	if emerg == nil || emerg["folder_path"] != "Braking / Emergency braking" || emerg["status"] != "approved" || emerg["priority"] != "high" ||
		!strings.Contains(emerg["source"].(string), "ID: BRK-5") || !strings.Contains(emerg["description"].(string), "- at speeds above 5 km/h") {
		t.Fatalf("emergency braking: %v\n%v", emerg, byTitle)
	}
	rels := items(a.m("GET", base+"/relations", nil, 200))
	types := map[string]bool{}
	for _, r := range rels {
		types[r["type"].(string)] = true
	}
	if !types["refines"] || !types["conflicts_with"] {
		t.Errorf("relation types: %v", rels)
	}

	// re-import unchanged: nothing to do
	pv = a.upload("POST", base+"/exchange/reqif/preview", "file", "sysreq.reqif", "", doc, nil, 200)
	if s := summaryOf(pv); s["unchanged"].(float64) != 3 || pv["previous"].(float64) != 3 || pv["relations"].(map[string]any)["keep"].(float64) != 2 {
		t.Fatalf("re-import preview: %v %v", s, pv["relations"])
	}
	// DOORS changed BRK-6, dropped the conflict link; someone edited BRK-2 here
	v2 := strings.Replace(string(doc), "The driver SHOULD be warned acoustically", "The driver SHALL be warned acoustically and visually", 1)
	start := strings.Index(v2, `<SPEC-RELATION IDENTIFIER="_l-2"`)
	end := strings.Index(v2[start:], "</SPEC-RELATION>") + start + len("</SPEC-RELATION>")
	v2 = v2[:start] + v2[end:]
	v2 = strings.Replace(v2, "start up within 200&#160;ms", "start up within 150&#160;ms", 1)
	startup := byTitle["The brake controller shall start up within 200…"]
	if startup == nil {
		t.Fatalf("startup missing: %v", byTitle)
	}
	a.m("PATCH", "/api/v1/requirements/"+startup["id"].(string), map[string]any{"description": "Edited in Qtestframe"}, 200)
	pv = a.upload("POST", base+"/exchange/reqif/preview", "file", "sysreq.reqif", "", []byte(v2), nil, 200)
	s := summaryOf(pv)
	if s["changed"].(float64) != 1 || s["conflicts"].(float64) != 1 || s["unchanged"].(float64) != 1 || pv["relations"].(map[string]any)["remove"].(float64) != 1 {
		t.Fatalf("changed preview: %v %v", s, pv["relations"])
	}
	res = a.upload("POST", base+"/exchange/reqif/apply", "file", "sysreq.reqif", "", []byte(v2), nil, 200)
	if res["updated"].(float64) != 1 || res["relations_removed"].(float64) != 1 || res["skipped"].(float64) != 1 {
		t.Fatalf("re-import: %v", res)
	}
	if n := len(items(a.m("GET", base+"/requirements", nil, 200))); n != 3 {
		t.Errorf("re-import created requirements: %d", n)
	}
	if got := a.m("GET", "/api/v1/requirements/"+startup["id"].(string), nil, 200); got["description"] != "Edited in Qtestframe" {
		t.Errorf("conflict was overwritten: %v", got["description"])
	}
}

func TestReqIFImportPolarionMapping(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PO")
	base := "/api/v1/projects/" + pid
	doc := readFixture(t, "polarion.reqif")
	root := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Polarion"}, 201)
	extra := map[string]string{"mapping": `{"Component":"tags","Assignee":""}`, "folder_id": root["id"].(string)}
	pv := a.upload("POST", base+"/exchange/reqif/preview", "file", "prs.reqif", "", doc, extra, 200)
	if s := summaryOf(pv); s["new"].(float64) != 3 {
		t.Fatalf("preview: %v %v", s, pv["rows"])
	}
	if pv["relation_types"].(map[string]any)["has parent"] != "refines" || pv["target_folder"] != "Polarion" {
		t.Errorf("preview: %v %v", pv["relation_types"], pv["target_folder"])
	}
	a.upload("POST", base+"/exchange/reqif/preview", "file", "prs.reqif", "", doc, map[string]string{"mapping": `{"Title":"nope"}`}, 400)
	a.upload("POST", base+"/exchange/reqif/preview", "file", "prs.reqif", "", []byte("<x/>"), nil, 400)
	res := a.upload("POST", base+"/exchange/reqif/apply", "file", "prs.reqif", "", doc, extra, 200)
	if res["created"].(float64) != 3 || res["relations_added"].(float64) != 2 {
		t.Fatalf("apply: %v", res)
	}
	for _, r := range items(a.m("GET", base+"/requirements", nil, 200)) {
		switch r["title"] {
		case "Self-service registration":
			if r["obligation"] != "must" || r["folder_path"] != "Polarion / User Accounts" || r["owner"] != "" || len(r["tags"].([]any)) != 1 {
				t.Errorf("registration: %v", r)
			}
		case "E-mail verification":
			if r["status"] != "review" || r["obligation"] != "should" || !strings.Contains(r["description"].(string), "| Link lifetime | 24 h |") {
				t.Errorf("verification: %v", r)
			}
		case "Loan renewal":
			if r["folder_path"] != "Polarion / Loans" || r["obligation"] != "may" {
				t.Errorf("renewal: %v", r)
			}
		default:
			t.Errorf("unexpected %v", r["title"])
		}
	}
}

func TestReqIFExportReimport(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("QX")
	base := "/api/v1/projects/" + pid
	f := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Core"}, 201)
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Start", "description": "The app **MUST** start.\n\n- fast\n- quietly", "folder_id": f["id"],
		"priority": "high", "type": "performance", "tags": []string{"boot", "ux"}, "criteria": []map[string]any{{"description": "Starts in 2 s", "method": "test", "threshold": "2 s"}}}, 201)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Stop", "description": "The app SHOULD stop.", "obligation": "should"}, 201)
	a.m("POST", "/api/v1/requirements/"+r2["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "depends_on"}, 201)
	doc := a.get(base+"/export?format=reqif", 200)

	// into the same project: everything unchanged
	pv := a.upload("POST", base+"/exchange/reqif/preview", "file", "qx.reqif", "", doc, nil, 200)
	if s := summaryOf(pv); s["unchanged"].(float64) != 2 || pv["qtestframe"] != true || pv["relations"].(map[string]any)["keep"].(float64) != 1 {
		t.Fatalf("round trip: %v %v %v", s, pv["rows"], pv["relations"])
	}
	// edited in another tool: the change is picked up
	edited := strings.Replace(string(doc), `THE-VALUE="Stop"`, `THE-VALUE="Stop cleanly"`, 1)
	res := a.upload("POST", base+"/exchange/reqif/apply", "file", "qx.reqif", "", []byte(edited), nil, 200)
	if res["updated"].(float64) != 1 || res["unchanged"].(float64) != 1 {
		t.Fatalf("apply: %v", res)
	}
	if got := a.m("GET", "/api/v1/requirements/"+r2["id"].(string), nil, 200); got["title"] != "Stop cleanly" {
		t.Errorf("title: %v", got["title"])
	}
	// into another project: new requirements, folders and relations
	other := a.project("QY")
	res = a.upload("POST", "/api/v1/projects/"+other+"/exchange/reqif/apply", "file", "qx.reqif", "", doc, nil, 200)
	if res["created"].(float64) != 2 || res["relations_added"].(float64) != 1 || res["folders_created"].(float64) != 1 {
		t.Fatalf("other project: %v", res)
	}
	for _, r := range items(a.m("GET", "/api/v1/projects/"+other+"/requirements", nil, 200)) {
		if r["title"] == "Start" {
			full := a.m("GET", "/api/v1/requirements/"+r["id"].(string), nil, 200)
			if full["description"] != r1["description"] || full["type"] != "performance" || len(full["criteria"].([]any)) != 1 || full["folder_path"] != "Core" {
				t.Errorf("copied requirement: %v", full)
			}
		}
	}
	_ = context.Background()
}
