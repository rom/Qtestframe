package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func testImportFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testimport", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func num(m map[string]any, k string) int {
	f, _ := m[k].(float64)
	return int(f)
}

func TestTestImportTestRail(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TR")
	base := "/api/v1/projects/" + pid
	// requirements imported from Jira earlier: matched by "ID: <key>" in the source
	r101 := a.m("POST", base+"/requirements", map[string]any{"title": "Sign in", "source": "Jira, ID: SHOP-101"}, 201)
	a.m("POST", base+"/requirements", map[string]any{"title": "Cart", "source": "ID: SHOP-200"}, 201)
	a.m("POST", base+"/custom-fields", map[string]any{"entity_type": "test", "label": "Component", "type": "single_select", "options": []string{"Accounts", "Checkout"}}, 201)
	a.m("POST", base+"/custom-fields", map[string]any{"entity_type": "test", "label": "Automation tool", "type": "text"}, 201)

	file := testImportFixture(t, "testrail-cases.csv")
	up := func(path string, data []byte, name string, extra map[string]string, want int) map[string]any {
		f := map[string]string{"source": "testrail"}
		for k, v := range extra {
			f[k] = v
		}
		return a.upload("POST", base+"/tests/import/"+path, "file", name, "", data, f, want)
	}
	pv := up("preview", file, "cases.csv", nil, 200)
	sum := pv["summary"].(map[string]any)
	if num(pv, "total") != 4 || num(sum, "new") != 4 || pv["source_name"] != "TestRail" {
		t.Fatalf("preview: %v", pv)
	}
	unresolved := pv["unresolved_links"].([]any)
	if len(unresolved) != 3 { // SHOP-102, SHOP-201, SHOP-999
		t.Errorf("unresolved: %v", unresolved)
	}
	var comp map[string]any
	for _, f := range pv["fields"].([]any) {
		if f.(map[string]any)["name"] == "Component" {
			comp = f.(map[string]any)
		}
	}
	if comp == nil || comp["target"] != "component" {
		t.Errorf("component mapping: %v", pv["fields"])
	}
	if len(items(a.m("GET", base+"/tests", nil, 200))) != 0 {
		t.Fatal("preview saved tests")
	}

	// map Automation Type to the text field, the priority Low to medium
	extra := map[string]string{"fields": `{"Automation Type":"automation_tool"}`, "priorities": `{"Low":"medium"}`}
	res := up("apply", file, "cases.csv", extra, 200)
	if num(res, "created") != 4 || num(res, "folders_created") != 5 || num(res, "links_added") != 3 {
		t.Fatalf("apply: %v", res)
	}
	tests := items(a.m("GET", base+"/tests", nil, 200))
	var signIn, pay map[string]any
	for _, x := range tests {
		switch x["title"] {
		case "Sign in with a valid password":
			signIn = x
		case "Pay with a saved card":
			pay = x
		}
	}
	if signIn == nil || pay == nil {
		t.Fatalf("tests: %v", tests)
	}
	full := a.m("GET", "/api/v1/tests/"+signIn["id"].(string), nil, 200)
	steps := full["steps"].([]any)
	cf, _ := full["custom_fields"].(map[string]any)
	if len(steps) != 3 || full["priority"] != "high" || full["preconditions"] != "A registered customer account exists." || full["folder_path"] != "Accounts / Sign in" ||
		cf["component"] != "Accounts" || cf["automation_tool"] != "None" || full["estimated_minutes"].(float64) != 5 {
		t.Errorf("sign-in test: %v", full)
	}
	if pay["priority"] != "medium" || pay["execution_type"] != "automated" {
		t.Errorf("pay test: %v", pay)
	}
	var links []map[string]any
	for _, l := range items(a.m("GET", base+"/links", nil, 200)) {
		if l["requirement_id"] == r101["id"] {
			links = append(links, l)
		}
	}
	if len(links) != 2 {
		t.Errorf("links of SHOP-101: %v", links)
	}

	// the same file again: nothing to do
	pv = up("preview", file, "cases.csv", extra, 200)
	if sum := pv["summary"].(map[string]any); num(sum, "unchanged") != 4 || num(pv, "previous") != 4 {
		t.Fatalf("second preview: %v", sum)
	}
	// a test edited here, the file edited there: a conflict; another
	// edited only there: an update
	a.m("PATCH", "/api/v1/tests/"+signIn["id"].(string), map[string]any{"title": "Sign in (edited)"}, 200)
	edited := []byte(strings.NewReplacer("Open the sign-in page", "Open the start page", "Choose the saved card and pay", "Choose the saved card").Replace(string(file)))
	pv = up("preview", edited, "cases.csv", extra, 200)
	sum = pv["summary"].(map[string]any)
	if num(sum, "conflict") != 1 || num(sum, "update") != 1 || num(sum, "unchanged") != 2 {
		t.Fatalf("edited preview: %v", sum)
	}
	res = up("apply", edited, "cases.csv", extra, 200)
	if num(res, "updated") != 1 || num(res, "skipped") != 1 || num(res, "created") != 0 {
		t.Fatalf("edited apply: %v", res)
	}
	res = up("apply", edited, "cases.csv", map[string]string{"fields": extra["fields"], "priorities": extra["priorities"], "overwrite_conflicts": "true"}, 200)
	if num(res, "updated") != 1 {
		t.Fatalf("overwrite: %v", res)
	}
	full = a.m("GET", "/api/v1/tests/"+signIn["id"].(string), nil, 200)
	if full["title"] != "Sign in with a valid password" || full["steps"].([]any)[0].(map[string]any)["action"] != "Open the start page" {
		t.Errorf("overwritten: %v", full)
	}
	if n := len(items(a.m("GET", base+"/tests", nil, 200))); n != 4 {
		t.Errorf("tests after re-imports: %d", n)
	}

	// the results of two runs
	run := testImportFixture(t, "testrail-run.csv")
	pv = up("preview", run, "run.csv", nil, 200)
	if h := pv["history"].(map[string]any); num(h, "runs") != 2 || num(h, "executions") != 3 {
		t.Fatalf("history preview: %v", h)
	}
	res = up("apply", run, "run.csv", map[string]string{"overwrite_conflicts": "true"}, 200)
	if num(res, "runs_created") != 2 || num(res, "results_imported") != 3 {
		t.Fatalf("history apply: %v", res)
	}
	runs := items(a.m("GET", base+"/runs", nil, 200))
	if len(runs) != 2 {
		t.Fatalf("runs: %v", runs)
	}
	for _, r := range runs {
		if r["status"] != "completed" || !strings.HasPrefix(r["name"].(string), "TestRail: Release 2.") {
			t.Errorf("run: %v", r)
		}
	}
	execs := items(a.m("GET", "/api/v1/tests/"+signIn["id"].(string)+"/executions", nil, 200))
	if len(execs) != 2 {
		t.Errorf("history of sign-in: %v", execs)
	}
	// importing the results again adds nothing
	res = up("apply", run, "run.csv", nil, 200)
	if num(res, "runs_created") != 0 || num(res, "results_imported") != 0 {
		t.Errorf("history again: %v", res)
	}
	audit := a.m("GET", "/api/v1/audit?action=test.import", nil, 200)
	if !strings.Contains(string(mustJSON(audit)), "TestRail csv") {
		t.Errorf("audit: %v", audit)
	}
}

func TestTestImportSources(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TS")
	base := "/api/v1/projects/" + pid
	folder := a.m("POST", base+"/folders", map[string]any{"kind": "test", "name": "Imported"}, 201)
	a.m("POST", base+"/requirements", map[string]any{"title": "Search", "source": "ID: SHOP-110"}, 201)
	cases := []struct {
		source, file string
		total        int
		extra        map[string]string
		check        func(pv, res map[string]any)
	}{
		{"xray", "xray-tests.json", 3, nil, func(pv, res map[string]any) {
			if h := pv["history"].(map[string]any); num(h, "executions") != 2 || num(res, "results_imported") != 2 || num(res, "links_added") != 1 {
				t.Errorf("xray: %v %v", h, res)
			}
		}},
		{"xray", "xray-tests.csv", 3, map[string]string{"format": "csv"}, nil},
		{"zephyr", "zephyr-scale.xml", 2, nil, nil},
		{"zephyr", "zephyr-scale.csv", 3, nil, nil},
		{"qtest", "qtest-cases.xlsx", 2, nil, nil},
		{"qtest", "qtest-cases.csv", 2, nil, nil},
		{"azure", "azure-testplans.csv", 2, nil, nil},
		{"azure", "azure-query.csv", 1, map[string]string{"create_folders": "false", "link_requirements": "false", "history": "false"}, nil},
		{"testrail", "testrail-suite.xml", 2, map[string]string{"exclude": "[1]"}, func(_, res map[string]any) {
			if num(res, "created") != 1 || num(res, "skipped") != 1 {
				t.Errorf("testrail xml: %v", res)
			}
		}},
	}
	for _, c := range cases {
		f := map[string]string{"source": c.source, "folder_id": folder["id"].(string)}
		for k, v := range c.extra {
			f[k] = v
		}
		data := testImportFixture(t, c.file)
		pv := a.upload("POST", base+"/tests/import/preview", "file", c.file, "", data, f, 200)
		if num(pv, "total") != c.total || pv["target_folder"] != "Imported" {
			t.Errorf("%s: preview %v", c.file, pv)
			continue
		}
		res := a.upload("POST", base+"/tests/import/apply", "file", c.file, "", data, f, 200)
		if c.check != nil {
			c.check(pv, res)
		}
	}
	var gherkin map[string]any
	for _, x := range items(a.m("GET", base+"/tests", nil, 200)) {
		if x["title"] == "Guest checkout" {
			gherkin = a.m("GET", "/api/v1/tests/"+x["id"].(string), nil, 200)
		}
	}
	if gherkin == nil || !strings.Contains(gherkin["description"].(string), "```gherkin") || len(gherkin["steps"].([]any)) == 0 || gherkin["status"] != "review" {
		t.Errorf("cucumber test: %v", gherkin)
	}

	// errors
	a.upload("POST", base+"/tests/import/preview", "file", "x.csv", "", []byte("a,b\n1,2\n"), map[string]string{"source": "nope"}, 400)
	a.upload("POST", base+"/tests/import/preview", "file", "x.json", "", []byte("[]"), map[string]string{"source": "testrail"}, 400)
	a.upload("POST", base+"/tests/import/preview", "file", "x.csv", "", []byte("a,b\n1,2\n"), map[string]string{"source": "azure"}, 400)
	a.upload("POST", base+"/tests/import/preview", "file", "x.csv", "", []byte("Title\nx\n"), map[string]string{"source": "azure", "priorities": "[1]"}, 400)
	a.upload("POST", base+"/tests/import/preview", "file", "x.csv", "", []byte("Title\nx\n"), map[string]string{"source": "azure", "folder_id": "nope"}, 400)
}

func TestTestImportPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TP")
	base := "/api/v1/projects/" + pid
	e.member("tester@example.com", rbac.RoleTester)
	e.member("viewer@example.com", rbac.RoleViewer)
	data := testImportFixture(t, "xray-tests.json")
	v := e.login("viewer@example.com")
	v.upload("POST", base+"/tests/import/preview", "file", "x.json", "", data, map[string]string{"source": "xray"}, 403)
	// a tester cannot set tests ready: they are imported as drafts
	ts := e.login("tester@example.com")
	pv := ts.upload("POST", base+"/tests/import/preview", "file", "x.json", "", data, map[string]string{"source": "xray"}, 200)
	if !strings.Contains(string(mustJSON(pv)), "imported as draft") {
		t.Errorf("tester preview: %v", pv["rows"])
	}
	res := ts.upload("POST", base+"/tests/import/apply", "file", "x.json", "", data, map[string]string{"source": "xray"}, 200)
	if num(res, "created") != 3 || num(res, "results_imported") != 2 {
		t.Errorf("tester apply: %v", res)
	}
}
