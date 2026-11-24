package testimport

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

func parse(t *testing.T, source, file string, opt Options) *Result {
	t.Helper()
	r, err := Parse(source, fixture(t, file), file, opt)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return r
}

func byID(r *Result, id string) *Case {
	for i := range r.Cases {
		if r.Cases[i].ExternalID == id {
			return &r.Cases[i]
		}
	}
	return nil
}

func TestTestRailCSV(t *testing.T) {
	defs := []model.CustomField{{Key: "component", Label: "Component", Type: model.FieldText}, {Key: "tool", Label: "Tool", Type: model.FieldText}}
	r := parse(t, "testrail", "testrail-cases.csv", Options{CustomFields: defs, Fields: map[string]string{"automation type": "tool", "Type": "ignore"},
		Priorities: map[string]string{"low": "medium", "high": "bogus"}})
	if r.Format != FormatCSV || len(r.Cases) != 4 || len(r.Errors) != 0 {
		t.Fatalf("result: %+v", r)
	}
	c := byID(r, "C101")
	if c.Title != "Sign in with a valid password" || c.Folder != "Accounts/Sign in" || len(c.Steps) != 3 || c.Steps[2].Expected != "The account overview is shown" ||
		c.EstimatedMin != 5 || !reflect.DeepEqual(c.Refs, []string{"SHOP-101", "SHOP-102"}) || c.Priority != "high" {
		t.Errorf("C101: %+v", c)
	}
	if c := byID(r, "C103"); len(c.Steps) != 2 || c.Steps[1].Expected != "The cart shows one item" || c.EstimatedMin != 90 {
		t.Errorf("C103 (text template): %+v", c)
	}
	if c := byID(r, "C104"); !c.Automated || c.Priority != "medium" {
		t.Errorf("C104: %+v", c)
	}
	targets := map[string]string{}
	for _, f := range r.Fields {
		targets[f.Name] = f.Target
	}
	if targets["Component"] != "component" || targets["Automation Type"] != "tool" || targets["Type"] != "" {
		t.Errorf("fields: %+v", r.Fields)
	}
	for _, v := range r.Priorities {
		if v.Value == "Low" && (v.Target != "medium" || v.Guessed) || v.Value == "High" && (v.Target != "high" || !v.Guessed) {
			t.Errorf("priority %+v", v)
		}
	}
}

func TestTestRailRunAndXML(t *testing.T) {
	r := parse(t, "testrail", "testrail-run.csv", Options{})
	if len(r.Cases) != 3 || r.Executions != 3 {
		t.Fatalf("run: %+v", r)
	}
	c := byID(r, "C101")
	if len(c.Executions) != 2 || c.Executions[0].RunID != "R12" || c.Executions[1].Status != model.ResultBlocked || c.Executions[0].ExecutedAt == nil {
		t.Errorf("C101 executions: %+v", c.Executions)
	}
	x := parse(t, "testrail", "testrail-suite.xml", Options{})
	if x.Format != FormatXML || len(x.Cases) != 2 {
		t.Fatalf("xml: %+v", x)
	}
	c = byID(x, "C101")
	if c.Folder != "Accounts/Sign in" || len(c.Steps) != 3 || c.Steps[1].Data != "ada@example.com / <password>" || c.Custom["component"] != "Accounts" || c.Custom["automation_type"] != "None" {
		t.Errorf("xml C101: %+v", c)
	}
	if c := byID(x, "C103"); c.Custom["browsers"] != "Firefox, Chrome" || len(c.Steps) != 2 {
		t.Errorf("xml C103: %+v", c)
	}
	// several suites: the suite becomes the first folder level
	multi := "ID,Title,Section,Suite\nC1,A,S1,Web\nC2,B,S2,App\n"
	m, err := Parse("testrail", []byte(multi), "m.csv", Options{})
	if err != nil || m.Cases[0].Folder != "Web/S1" || m.Cases[1].Folder != "App/S2" {
		t.Errorf("suites: %v %+v", err, m)
	}
	if _, err := Parse("testrail", []byte("<other/>"), "x.xml", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("wrong root: %v", err)
	}
	if _, err := Parse("testrail", []byte("ID,Name\n1,2\n"), "x.csv", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no title column: %v", err)
	}
}

func TestXray(t *testing.T) {
	for _, file := range []string{"xray-tests.csv", "xray-tests.json"} {
		r := parse(t, "xray", file, Options{})
		if len(r.Cases) != 3 {
			t.Fatalf("%s: %+v", file, r)
		}
		m := byID(r, "SHOP-T1")
		if len(m.Steps) != 2 || m.Steps[0].Data != "lamp" || m.Preconditions != "The catalogue holds the demo products." || m.Folder != "Catalogue/Search" ||
			m.Status != "ready" || !contains(m.Tags, "SHOP-50") || !contains(m.Tags, "smoke") {
			t.Errorf("%s manual: %+v", file, m)
		}
		cu := byID(r, "SHOP-T2")
		if cu.Gherkin == "" || len(cu.Steps) < 2 || !strings.HasPrefix(cu.Steps[0].Action, "Given") || cu.Status != "review" || !contains(cu.Tags, "bdd") ||
			!reflect.DeepEqual(cu.Refs, []string{"SHOP-120", "SHOP-121"}) {
			t.Errorf("%s cucumber: %+v", file, cu)
		}
		g := byID(r, "SHOP-T3")
		if !g.Automated || g.Definition != "tests/api/test_prices.py::test_price_list" {
			t.Errorf("%s generic: %+v", file, g)
		}
	}
	r := parse(t, "xray", "xray-tests.json", Options{})
	if m := byID(r, "SHOP-T1"); len(m.Executions) != 2 || m.Executions[1].Comment != "No suggestions for umlauts" || m.Custom["Channel"] != "Web" {
		t.Errorf("json runs: %+v", m)
	}
	// an object with a list, Cloud steps, definitions and string refs
	obj := `{"tests":[{"testKey":"X-1","summary":"S","testType":{"name":"Cucumber"},"definition":"Given a\nThen b","requirements":"X-9, X-10",
		"preconditions":["Logged in"],"steps":[{"fields":{"Action":"a","Expected Result":"e"}}],"labels":"one two"},
		{"key":"X-2","summary":"G","testType":"Generic","definition":"run.sh","executions":[{"key":"E-1","status":{"name":"PASS"},"finishedOn":"2026-10-01"}]}, 7]}`
	r, err := Parse("xray", []byte(obj), "x.json", Options{})
	if err != nil || len(r.Cases) != 2 || len(r.Errors) != 1 {
		t.Fatalf("object: %v %+v", err, r)
	}
	if c := byID(r, "X-1"); c.Gherkin == "" || !reflect.DeepEqual(c.Refs, []string{"X-9", "X-10"}) || c.Preconditions != "Logged in" || !contains(c.Tags, "two") {
		t.Errorf("X-1: %+v", c)
	}
	if c := byID(r, "X-2"); c.Definition != "run.sh" || len(c.Executions) != 1 || c.Executions[0].Run != "E-1" {
		t.Errorf("X-2: %+v", c)
	}
	if _, err := Parse("xray", []byte("{oops"), "x.json", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad json: %v", err)
	}
	if _, err := Parse("xray", []byte(`"text"`), "x.json", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("not a list: %v", err)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestZephyr(t *testing.T) {
	r := parse(t, "zephyr", "zephyr-scale.csv", Options{})
	if len(r.Cases) != 3 {
		t.Fatalf("csv: %+v", r)
	}
	if c := byID(r, "SHOP-T10"); len(c.Steps) != 2 || c.Steps[1].Data != "Street: Storgatan 1" || c.EstimatedMin != 10 || c.Status != "ready" || c.Custom["Browser"] != "Firefox" {
		t.Errorf("T10: %+v", c)
	}
	if c := byID(r, "SHOP-T11"); len(c.Steps) != 2 || c.Priority != "medium" || len(c.Refs) != 2 {
		t.Errorf("T11 (plain text): %+v", c)
	}
	if c := byID(r, "SHOP-T12"); c.Gherkin == "" || c.Status != "deprecated" || c.Steps[len(c.Steps)-1].Expected != "Then a refund is issued" {
		t.Errorf("T12 (BDD): %+v", c)
	}
	x := parse(t, "zephyr", "zephyr-scale.xml", Options{})
	if len(x.Cases) != 2 {
		t.Fatalf("xml: %+v", x)
	}
	if c := byID(x, "SHOP-T10"); c.Steps[0].Action != "Open My addresses" || c.Description != "The customer can change where the order goes." || c.Custom["Reviewed"] != "true" ||
		!reflect.DeepEqual(c.Refs, []string{"SHOP-140"}) || c.Folder != "Accounts/Addresses" {
		t.Errorf("xml T10: %+v", c)
	}
	if _, err := Parse("zephyr", []byte("<project/>"), "z.xml", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no test cases: %v", err)
	}
	if _, err := Parse("zephyr", []byte("<project><broken"), "z.xml", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad xml: %v", err)
	}
}

func TestQTestAndAzure(t *testing.T) {
	for _, file := range []string{"qtest-cases.xlsx", "qtest-cases.csv"} {
		r := parse(t, "qtest", file, Options{})
		c := byID(r, "TC-21")
		if len(r.Cases) != 2 || c.Folder != "Catalogue/Filters" || len(c.Steps) != 2 || c.Priority != "critical" || c.Description != "The price filter narrows the list." ||
			c.Custom["Test Environment"] != "Staging" {
			t.Errorf("%s: %+v", file, c)
		}
		if c := byID(r, "TC-22"); !c.Automated || c.Status != "draft" {
			t.Errorf("%s TC-22: %+v", file, c)
		}
	}
	if _, err := Parse("qtest", []byte("{}"), "x.json", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("qtest json: %v", err)
	}
	r := parse(t, "azure", "azure-testplans.csv", Options{})
	c := byID(r, "4711")
	if len(r.Cases) != 2 || c.Folder != "Orders/Cancel" || len(c.Steps) != 2 || c.Owner != "ada@example.com" || c.Priority != "critical" || !reflect.DeepEqual(c.Tags, []string{"orders", "smoke"}) {
		t.Errorf("azure: %+v", c)
	}
	q := parse(t, "azure", "azure-query.csv", Options{})
	if len(q.Cases) != 1 || len(q.Warnings) != 1 || len(q.Cases[0].Steps) != 3 || q.Cases[0].Steps[1].Action != "Press Invoice" || len(q.Cases[0].Warnings) != 1 {
		t.Errorf("azure query: %+v", q)
	}
	steps, _ := AzureSteps("just text")
	if len(steps) != 1 || steps[0].Action != "just text" {
		t.Errorf("text steps: %+v", steps)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse("nope", []byte("x"), "x.csv", Options{}); !errors.Is(err, ErrInvalid) {
		t.Error(err)
	}
	if _, err := Parse("xray", []byte("  "), "x.csv", Options{}); !errors.Is(err, ErrInvalid) {
		t.Error(err)
	}
	if _, err := Parse("azure", []byte("Title\nA\nB\n"), "x.csv", Options{MaxCases: 1}); !errors.Is(err, ErrInvalid) {
		t.Error("max cases", err)
	}
	r, err := Parse("azure", []byte("Title,Description\n,\n"), "x.csv", Options{})
	if err != nil || len(r.Warnings) != 1 {
		t.Errorf("no cases: %v %+v", err, r)
	}
	r, err = Parse("azure", []byte("Title,Description\n,Only a description\n"), "x.csv", Options{})
	if err != nil || r.Cases[0].Title != "Only a description" || len(r.Cases[0].Warnings) != 1 {
		t.Errorf("title from description: %v %+v", err, r)
	}
	for _, x := range []struct{ src, explicit, name, data, want string }{
		{"testrail", "", "a.xml", "<x/>", FormatXML},
		{"testrail", "", "a", "<x/>", FormatXML},
		{"xray", "", "a", "[1]", FormatJSON},
		{"qtest", "", "a", "PK\x03\x04", FormatXLSX},
		{"xray", "", "a.txt", "a", FormatCSV},
		{"xray", "json", "a.csv", "a", FormatJSON},
		{"qtest", "excel", "a.csv", "a", FormatXLSX},
		{"azure", "tsv", "a", "a", FormatCSV},
		{"zephyr", "xml", "a", "a", FormatXML},
	} {
		if f, err := DetectFormat(x.src, x.explicit, x.name, []byte(x.data)); err != nil || f != x.want {
			t.Errorf("%+v: %s %v", x, f, err)
		}
	}
	for _, x := range [][3]string{{"azure", "", "a.xml"}, {"testrail", "pdf", "a"}, {"qtest", "", "a.xls"}, {"testrail", "", "a.json"}} {
		if _, err := DetectFormat(x[0], x[1], x[2], []byte("a")); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: %v", x, err)
		}
	}
	for _, s := range []string{"Test-Rail", "jira", "squad", "q-test", "ado"} {
		if _, ok := ValidSource(s); !ok {
			t.Error(s)
		}
	}
}

func TestMappings(t *testing.T) {
	for in, want := range map[string]string{"Highest": "critical", "Blocker": "critical", "P1": "critical", "1": "critical", "Major": "high", "3 - Must Test": "high",
		"2 - Test If Time": "medium", "Normal": "medium", "1 - Don't Test": "low", "Trivial": "low", "4": "low", "P2": "high", "3": "medium", "": "", "whatever": ""} {
		if got := guessPriority(in); got != want {
			t.Errorf("priority %q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Approved": "ready", "Ready for Baseline": "ready", "In Review": "review", "Closed": "deprecated", "Design": "draft",
		"To Do": "draft", "Obsolete": "deprecated", "": "", "zzz": ""} {
		if got := guessStatus(in); got != want {
			t.Errorf("status %q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Passed": "passed", "PASS": "passed", "Failed": "failed", "Blocked": "blocked", "N/A": "skipped", "Skipped": "skipped",
		"Untested": "", "Retest": "", "EXECUTING": "", "": "", "Weird": ""} {
		if got := GuessResult(in); got != want {
			t.Errorf("result %q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]int{"1h 30m": 90, "45": 45, "2d": 960, "1w": 2400, "01:30": 90, "90 min": 90, "30s": 1, "": 0, "x": 0} {
		if got := estimateMinutes(in); got != want {
			t.Errorf("estimate %q: %d, want %d", in, got, want)
		}
	}
	for _, s := range []string{"2026-10-01T09:12:00Z", "2026-10-01 09:12", "10/1/2026 9:12 AM", "01.10.2026", "1790000000000"} {
		if parseTime(s) == nil {
			t.Errorf("time %q", s)
		}
	}
	if parseTime("") != nil || parseTime("soon") != nil {
		t.Error("bad times")
	}
	if got := htmlText("<div><p>One&nbsp;&amp; two</p><ul><li>a</li><li>b</li></ul><br/></div>"); got != "One & two\n- a\n- b" {
		t.Errorf("html: %q", got)
	}
	if got := splitNumbered("Just one thing"); len(got) != 1 {
		t.Errorf("numbered: %q", got)
	}
	if got := splitNumbered("Intro\n1. a\n2) b\ncontinued"); len(got) != 3 || got[2] != "b\ncontinued" {
		t.Errorf("numbered: %q", got)
	}
	if steps, exp := textSteps("1. a\n2. b\n3. c", "All done"); len(steps) != 3 || exp != "All done" {
		t.Errorf("text steps: %+v %q", steps, exp)
	}
	if steps, exp := textSteps("", "e"); steps != nil || exp != "e" {
		t.Errorf("no steps: %+v %q", steps, exp)
	}
	if got := GherkinSteps("Then only an outcome\nBut nothing else"); len(got) != 1 || got[0].Action != "" {
		t.Errorf("gherkin: %+v", got)
	}
	if got := splitRefs("#4711; CALC-1 Summary text | X-2"); !reflect.DeepEqual(got, []string{"4711", "CALC-1", "X-2"}) {
		t.Errorf("refs: %q", got)
	}
	if got := FolderParts(" a / /b "); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("folders: %q", got)
	}
	if clip("abcdef", 4) != "abc…" || firstLine("  x\ny") != "x" {
		t.Error("clip")
	}
	if got := zephyrEstimate("120000"); got != 2 {
		t.Errorf("zephyr estimate %d", got)
	}
	defs := []model.CustomField{{Key: "mass_kg", Label: "Mass", Unit: "kg", Type: model.FieldNumber}}
	if k, g := mappedField(nil, "Mass (kg)", defs); k != "mass_kg" || !g {
		t.Errorf("by column: %q", k)
	}
	if k, _ := mappedField(map[string]string{"mass (kg)": "cf.mass_kg"}, "Mass (kg)", defs); k != "mass_kg" {
		t.Errorf("by choice: %q", k)
	}
	if k, g := mappedField(map[string]string{"Weight": "unknown"}, "Weight", defs); k != "" || !g {
		t.Errorf("unknown choice: %q", k)
	}
}
