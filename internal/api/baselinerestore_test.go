package api

import (
	"testing"
)

func TestRestoreFromBaseline(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BRS")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in."}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout"}, 201)["id"].(string)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "steps": []map[string]any{{"action": "Open", "expected": "Form"}}}, 201)["id"].(string)
	b := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "B1"}, 201)["id"].(string)

	// changes after the baseline
	a.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"title": "Login with 2FA", "description": "The user MUST log in with 2FA."}, 200)
	a.m("PATCH", "/api/v1/tests/"+t1, map[string]any{"title": "Valid login (2FA)", "steps": []map[string]any{{"action": "Open", "expected": "Form"}, {"action": "Code", "expected": "In"}}}, 200)
	added := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "New"}, 201)["id"].(string)
	a.m("DELETE", "/api/v1/requirements/"+r2, nil, 204)

	path := "/api/v1/baselines/" + b + "/restore"
	a.m("POST", path, map[string]any{"items": []any{}}, 400)
	a.m("POST", "/api/v1/baselines/missing/restore", map[string]any{"items": []map[string]any{{"entity_type": "requirement", "entity_id": r1}}}, 404)
	res := a.m("POST", path, map[string]any{"items": []map[string]any{
		{"entity_type": "requirement", "entity_id": r1},
		{"entity_type": "test", "entity_id": t1},
		{"entity_type": "requirement", "entity_id": r2},
		{"entity_type": "requirement", "entity_id": added},
	}}, 200)
	if len(res["restored"].([]any)) != 2 {
		t.Fatal(res)
	}
	reasons := map[string]bool{}
	for _, s := range res["skipped"].([]any) {
		reasons[s.(map[string]any)["reason"].(string)] = true
	}
	if !reasons["deleted"] || !reasons["not_in_baseline"] {
		t.Fatal(res)
	}
	if got := a.m("GET", "/api/v1/requirements/"+r1, nil, 200); got["title"] != "Login" || got["description"] != "The user MUST log in." {
		t.Fatal(got)
	}
	if got := a.m("GET", "/api/v1/tests/"+t1, nil, 200); got["title"] != "Valid login" || len(got["steps"].([]any)) != 1 {
		t.Fatal(got)
	}
	if hist := items(a.m("GET", "/api/v1/requirements/"+r1+"/history", nil, 200)); hist[0]["action"] != "restore" {
		t.Fatal(hist[0])
	}
	// the comparison now shows them as changed again only by version, and a second restore skips nothing new
	cmp := a.m("GET", "/api/v1/baselines/"+b+"/compare?to=current", nil, 200)
	for _, c := range cmp["changed"].([]any) {
		if fields := c.(map[string]any)["changed_fields"]; fields != nil && len(fields.([]any)) > 0 {
			t.Fatal("content still differs", c)
		}
	}
	// a tester cannot restore requirements, only tests
	e.member("tester@example.com", "tester")
	tr := e.login("tester@example.com").m("POST", path, map[string]any{"items": []map[string]any{{"entity_type": "requirement", "entity_id": r1}}}, 200)
	if tr["skipped"].([]any)[0].(map[string]any)["reason"] != "not_allowed" {
		t.Fatal(tr)
	}
}

func TestBaselineCompareIgnoresDerivedState(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BDS")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in."}, 201)["id"].(string)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rq, "test_case_id": tc}, 201)
	b := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "B"}, 201)["id"].(string)
	// the requirement's text changes: its link becomes suspect, a defect and a risk point at it
	a.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"description": "The user MUST log in with 2FA."}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "D", "links": []map[string]any{{"entity_type": "requirement", "entity_id": rq}}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "R", "likelihood": 5, "impact": 5, "requirement_ids": []string{rq}}, 201)
	cmp := a.m("GET", "/api/v1/baselines/"+b+"/compare?to=current", nil, 200)
	changed := cmp["changed"].([]any)
	// only the requirement changed, and only its description
	if len(changed) != 1 {
		t.Fatal(changed)
	}
	if f := strs(changed[0].(map[string]any)["changed_fields"]); len(f) != 1 || f[0] != "description" {
		t.Fatal(f)
	}
}
