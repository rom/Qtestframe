package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

const apiFeature = `@checkout @REQ-1 @req:BD-REQ-2 @unknown-REQ-9
Feature: Checkout
  Background:
    Given a customer with a saved card

  @smoke
  Scenario: Pay
    When the customer pays 25 EUR
    Then the order is "paid"

  Scenario Outline: Pay <amount>
    When the customer pays <amount> EUR

    Examples:
      | amount |
      | 5      |
`

func TestGherkinAPI(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BD")
	pp := "/api/v1/projects/" + pid
	r1 := a.m("POST", pp+"/requirements", map[string]any{"title": "Pay"}, 201)
	a.m("POST", pp+"/requirements", map[string]any{"title": "Refund"}, 201)

	// step library
	pay := a.m("POST", pp+"/step-definitions", map[string]any{"pattern": "the customer pays {int} EUR", "keyword": "when", "description": "Pays"}, 201)
	a.m("POST", pp+"/step-definitions", map[string]any{"pattern": "x {nope}"}, 400)
	a.m("POST", pp+"/step-definitions", map[string]any{"pattern": "the customer pays {int} EUR"}, 409)
	card := a.m("POST", pp+"/step-definitions", map[string]any{"pattern": `^a customer with a (\w+) card$`, "kind": "regex"}, 201)
	upd := a.m("PATCH", "/api/v1/step-definitions/"+card["id"].(string), map[string]any{"description": "Card"}, 200)
	if upd["description"] != "Card" || upd["kind"] != "regex" {
		t.Fatal(upd)
	}
	a.m("PATCH", "/api/v1/step-definitions/"+card["id"].(string), map[string]any{"pattern": ""}, 400)

	// check: errors with lines, defined and undefined steps
	chk := a.m("POST", pp+"/gherkin/check", map[string]any{"source": "Feature: a\n  Scenario: s\n    Given x\n    oops\n"}, 200)
	errs := toMaps(chk["errors"])
	if chk["valid"] != false || len(errs) != 1 || errs[0]["line"] != float64(4) {
		t.Fatal(chk)
	}
	chk = a.m("POST", pp+"/gherkin/check", map[string]any{"source": apiFeature}, 200)
	steps := toMaps(chk["steps"])
	if chk["valid"] != true || chk["feature"] != "Checkout" || chk["undefined"] != float64(1) || len(steps) != 4 || len(toMaps(chk["scenarios"])) != 2 {
		t.Fatal(chk)
	}
	if steps[1]["definition"] != pay["id"] || steps[2]["defined"] != false || steps[2]["suggestion"] != "the order is {string}" {
		t.Fatal(steps)
	}
	if c := a.m("POST", pp+"/gherkin/check", map[string]any{"source": ""}, 200); c["valid"] != false {
		t.Fatal(c)
	}

	// convert
	conv := a.m("POST", pp+"/gherkin/convert", map[string]any{"to": "gherkin", "language": "sv", "title": "Betala", "preconditions": "inloggad",
		"steps":      []map[string]any{{"action": "betala {{belopp}}", "expected": "betald"}, {"call_test_id": "x", "call_test_key": "BD-TC-9"}},
		"parameters": []string{"belopp"}, "data_rows": []map[string]any{{"values": map[string]string{"belopp": "5"}}}}, 200)
	g := conv["gherkin"].(string)
	if !strings.Contains(g, "Scenariomall: Betala") || !strings.Contains(g, "När betala <belopp>") || !strings.Contains(g, "När stegen i BD-TC-9 är gjorda") || len(conv["warnings"].([]any)) != 1 {
		t.Fatal(conv)
	}
	back := a.m("POST", pp+"/gherkin/convert", map[string]any{"to": "steps", "gherkin": g}, 200)
	if back["preconditions"] != "Givet inloggad" || len(toMaps(back["steps"])) != 2 || back["parameters"].([]any)[0] != "belopp" {
		t.Fatal(back)
	}
	a.m("POST", pp+"/gherkin/convert", map[string]any{"to": "steps", "gherkin": "nope"}, 400)
	a.m("POST", pp+"/gherkin/convert", map[string]any{"to": "yaml"}, 400)

	// a Gherkin test
	bad := a.raw("POST", pp+"/tests", map[string]any{"title": "x", "format": "gherkin", "gherkin": "Feature: a\n  Scenario: s\n    Given x\n    oops\n"})
	body, _ := io.ReadAll(bad.Body)
	bad.Body.Close()
	if bad.StatusCode != 400 || !strings.Contains(string(body), "line 4") {
		t.Fatal(bad.StatusCode, string(body))
	}
	tc := a.m("POST", pp+"/tests", map[string]any{"title": "Checkout", "format": "gherkin", "gherkin": apiFeature}, 201)
	tid := tc["id"].(string)
	if tc["format"] != "gherkin" {
		t.Fatal(tc)
	}
	list := a.m("GET", pp+"/tests?q="+url.QueryEscape("format:gherkin"), nil, 200)
	if len(items(list)) != 1 {
		t.Fatal(list)
	}

	// usage and rename
	lib := a.m("GET", pp+"/step-definitions", nil, 200)
	defs := items(lib)
	if len(defs) != 2 || len(toMaps(lib["undefined"])) != 1 {
		t.Fatal(lib)
	}
	for _, d := range defs {
		if d["id"] == pay["id"] && d["usage_count"] != float64(2) {
			t.Fatal(d)
		}
	}
	ren := a.m("POST", "/api/v1/step-definitions/"+pay["id"].(string)+"/rename", map[string]any{"pattern": "the customer pays {int} euro(s)"}, 200)
	if ren["steps"] != float64(2) || len(toMaps(ren["tests"])) != 1 {
		t.Fatal(ren)
	}
	got := a.m("GET", "/api/v1/tests/"+tid, nil, 200)
	if !strings.Contains(got["gherkin"].(string), "When the customer pays 25 euros") || got["version"] != float64(2) {
		t.Fatal(got)
	}
	hist := a.m("GET", "/api/v1/tests/"+tid+"/history", nil, 200)
	if len(items(hist)) < 2 {
		t.Fatal(hist)
	}
	a.m("POST", "/api/v1/step-definitions/"+pay["id"].(string)+"/rename", map[string]any{"pattern": "x {int} {int}"}, 400)

	// a run: scenarios as iterations, results back per line
	run := a.m("POST", pp+"/runs", map[string]any{"name": "r", "test_ids": []string{tid}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	exec := a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	srs := toMaps(exec["step_results"])
	if len(toMaps(exec["iterations"])) != 2 || len(srs) != 5 {
		t.Fatal(exec)
	}
	for _, sr := range srs {
		sr["status"] = model.ResultPassed
	}
	srs[1]["status"] = model.ResultFailed
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": srs}, 200)
	res := a.m("GET", "/api/v1/tests/"+tid+"/gherkin-results", nil, 200)
	lines := toMaps(res["steps"])
	if res["execution_id"] != ex["id"] || len(lines) != 4 || lines[0]["line"] != float64(4) || lines[1]["status"] != model.ResultFailed {
		t.Fatal(res)
	}

	// import: per scenario and per feature, requirement tags become links
	imp := a.m("POST", pp+"/tests/features/import", map[string]any{"mode": "scenario", "dry_run": true,
		"files": []map[string]any{{"name": "dir/checkout.feature", "content": apiFeature}, {"name": "bad.feature", "content": "Feature: x\n  oops\n  Scenario: s\n    Given a\n    oops\n"}}}, 200)
	tests := toMaps(imp["tests"])
	if imp["dry_run"] != true || len(tests) != 2 || tests[0]["file"] != "checkout.feature" || imp["links"] != float64(4) || len(toMaps(imp["errors"])) != 1 {
		t.Fatal(imp)
	}
	if strings.Join(toStrings(tests[0]["tags"]), ",") != "checkout,unknown-REQ-9,smoke" || strings.Join(toStrings(tests[0]["requirements"]), ",") != "BD-REQ-1,BD-REQ-2" {
		t.Fatal(tests[0])
	}
	if n := len(items(a.m("GET", pp+"/tests", nil, 200))); n != 1 {
		t.Fatal("dry run created tests", n)
	}
	imp = a.m("POST", pp+"/tests/features/import", map[string]any{"mode": "feature", "files": []map[string]any{{"name": "checkout.feature", "content": apiFeature}}}, 201)
	tests = toMaps(imp["tests"])
	if len(tests) != 1 || tests[0]["title"] != "Checkout" || imp["links"] != float64(2) {
		t.Fatal(imp)
	}
	links := a.m("GET", "/api/v1/tests/"+tests[0]["id"].(string)+"/links", nil, 200)
	if len(items(links)) != 2 {
		t.Fatal(links)
	}
	imp = a.m("POST", pp+"/tests/features/import", map[string]any{"files": []map[string]any{{"name": "checkout.feature", "content": apiFeature}}}, 201)
	if len(toMaps(imp["tests"])) != 2 {
		t.Fatal(imp)
	}
	a.m("POST", pp+"/tests/features/import", map[string]any{"files": []map[string]any{}}, 400)
	a.m("POST", pp+"/tests/features/import", map[string]any{"mode": "x", "files": []map[string]any{{"name": "a", "content": apiFeature}}}, 400)
	_ = r1

	// export: a zip of .feature files tagged with keys and requirements
	plain := a.m("POST", pp+"/tests", map[string]any{"title": "Plain steps", "steps": []map[string]any{{"action": "Open", "expected": "Opens"}}}, 201)
	data, hdr := a.body("GET", pp+"/tests/features?ids="+tid+","+plain["id"].(string), nil, 200)
	if hdr["Content-Type"][0] != "application/zip" {
		t.Fatal(hdr)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) != 2 {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		contents[f.Name] = string(b)
	}
	if c := contents["BD-TC-1-checkout.feature"]; !strings.HasPrefix(c, "@BD-TC-1\n@checkout") {
		t.Fatal(contents)
	}
	if c := contents[plain["key"].(string)+"-plain-steps.feature"]; !strings.Contains(c, "When Open\n    Then Opens") {
		t.Fatal(contents)
	}
	a.body("POST", pp+"/tests/features/export", map[string]any{"query": "format:gherkin", "language": "sv"}, 200)
	a.body("POST", pp+"/tests/features/export", map[string]any{"ids": []string{"none"}}, 400)

	// stubs of a Gherkin test are its feature
	st := a.m("POST", pp+"/tests/stubs", map[string]any{"framework": "cucumber", "format": "json", "ids": []string{tid}}, 200)
	if c := toMaps(st["files"])[0]["content"].(string); !strings.Contains(c, "@BD-TC-1 @qtestframe-stub\n@checkout") {
		t.Fatal(c)
	}

	// delete
	a.m("DELETE", "/api/v1/step-definitions/"+card["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/step-definitions/"+card["id"].(string), nil, 404)
	a.m("PATCH", "/api/v1/step-definitions/"+card["id"].(string), map[string]any{"description": "x"}, 404)
	a.m("POST", "/api/v1/step-definitions/"+card["id"].(string)+"/rename", map[string]any{"pattern": "x"}, 404)
	a.m("GET", "/api/v1/tests/missing/gherkin-results", nil, 404)
}

func TestFeatureHelpers(t *testing.T) {
	if featureFileName("K-1", "") != "K-1.feature" || featureFileName("K-1", strings.Repeat("ab ", 30)) != "K-1-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab.feature" {
		t.Fatal(featureFileName("K-1", strings.Repeat("ab ", 30)))
	}
	byKey := map[string]model.Requirement{"P-REQ-1": {ID: "1", Key: "P-REQ-1"}}
	for tag, ok := range map[string]bool{"@P-REQ-1": true, "@req:p-req-1": true, "@REQ-1": true, "@requirement=REQ-1": true, "@REQ-2": false} {
		if _, got := requirementOfTag(tag, "P", byKey); got != ok {
			t.Error(tag)
		}
	}
	// a broken Gherkin test exports as its steps
	out := FeatureOf(&model.TestCase{Key: "K-1", Title: "T", Format: model.FormatSteps, Steps: []model.TestStep{{Action: "a", CallTestID: new(string), CallTestKey: "K-2"}}}, []string{"R-1"}, "en")
	if !strings.HasPrefix(out, "@K-1 @R-1\nFeature: T") || !strings.Contains(out, "the steps of K-2 are done") {
		t.Fatal(out)
	}
}
