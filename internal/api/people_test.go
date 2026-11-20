package api

import (
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestPeopleAndMentions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PPL")
	e.member("viewer.one@example.com", rbac.RoleViewer)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "A", "owner": " @viewer.one ", "description": "Ask @admin about it.", "create_test": false}, 201)
	if r1["owner"] != "@viewer.one" {
		t.Errorf("owner not trimmed: %q", r1["owner"])
	}
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "B", "owner": "Carl Carlsson", "create_test": false}, 201)

	v := e.login("viewer.one@example.com")
	people := items(v.m("GET", "/api/v1/projects/"+pid+"/people", nil, 200))
	byHandle := map[string]map[string]any{}
	for _, p := range people {
		byHandle[p["handle"].(string)] = p
	}
	if p := byHandle["viewer.one"]; p == nil || p["kind"] != "user" || p["uses"].(float64) != 1 {
		t.Fatalf("viewer: %v", people)
	}
	if p := byHandle["admin"]; p == nil || p["kind"] != "user" || p["email"] != "admin@example.com" {
		t.Fatalf("admin: %v", people)
	}
	if p := byHandle["carl.carlsson"]; p == nil || p["kind"] != "person" || p["name"] != "Carl Carlsson" {
		t.Fatalf("person: %v", people)
	}
	v.m("GET", "/api/v1/projects/missing/people", nil, 404)

	// mention: finds owner and text mentions, with or without "@"
	for q, want := range map[string]int{"mention:viewer.one": 1, "mention:@admin": 1, "mentions:carl": 0, "-has:mention": 1} {
		got := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q="+url.QueryEscape(q), nil, 200))
		if len(got) != want {
			t.Errorf("%s: %d items, want %d", q, len(got), want)
		}
	}
	if got := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q="+url.QueryEscape("mention:viewer.one"), nil, 200)); got[0]["id"] != r1["id"] {
		t.Error(got)
	}
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T", "steps": []map[string]any{{"action": "ping @viewer.one", "expected": "ok"}}}, 201)
	if got := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=mention:viewer.one", nil, 200)); len(got) != 1 || got[0]["id"] != tc["id"] {
		t.Error(got)
	}
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{tc["id"].(string)}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"assignee": "@dora", "notes": "see @admin"}, 200)
	if got := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions?q=mention:dora", nil, 200)); len(got) != 1 {
		t.Error(got)
	}
	people = items(a.m("GET", "/api/v1/projects/"+pid+"/people", nil, 200))
	found := false
	for _, p := range people {
		found = found || (p["handle"] == "dora" && p["kind"] == "person")
	}
	if !found {
		t.Errorf("assignee not listed: %v", people)
	}
}

func TestSuggestTags(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TAG")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "tags": []string{"login", "audit"}, "create_test": false}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "y", "tags": []string{"login"}}, 201)
	got := items(a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", map[string]any{"title": "Login lockout", "description": "Lock the account after five failed logins; record it for GDPR.", "tags": []string{"audit"}}, 200))
	if len(got) == 0 || got[0]["tag"] != "login" || got[0]["reason"] != "existing" || got[0]["uses"].(float64) != 2 {
		t.Fatalf("%v", got)
	}
	var tags []string
	for _, g := range got {
		tags = append(tags, g["tag"].(string))
	}
	if s := strings.Join(tags, ","); !strings.Contains(s, "gdpr") || strings.Contains(s, "audit") {
		t.Errorf("tags: %s", s)
	}
	if got := items(a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", map[string]any{"title": " "}, 200)); len(got) != 0 {
		t.Error(got)
	}
	if got := items(a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", map[string]any{"title": "Login GDPR SSO API", "limit": 1}, 200)); len(got) != 1 {
		t.Error(got)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", map[string]any{"title": "x", "limit": 51}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", map[string]any{"title": strings.Repeat("x", 100_001)}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/tags/suggest", "", 400)
	a.m("POST", "/api/v1/projects/missing/tags/suggest", map[string]any{"title": "x"}, 404)
}

func TestListRelationColumns(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("REL")
	mk := func(title string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "create_test": false, "criteria": []map[string]any{{"description": "c"}}}, 201)["id"].(string)
	}
	api, ui, db := mk("API"), mk("UI"), mk("DB")
	a.m("POST", "/api/v1/requirements/"+ui+"/relations", map[string]any{"target_id": api, "type": "depends_on"}, 201)
	a.m("POST", "/api/v1/requirements/"+db+"/relations", map[string]any{"target_id": api, "type": "provides"}, 201)
	a.m("POST", "/api/v1/requirements/"+db+"/relations", map[string]any{"target_id": api, "type": "bogus"}, 400)
	shared := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Login steps", "steps": []map[string]any{{"action": "log in"}}}, 201)["id"].(string)
	caller := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Order", "steps": []map[string]any{{"call_test_id": shared}, {"call_test_id": shared}, {"action": "order"}}}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": api, "test_case_id": caller}, 201)

	reqs := map[string]map[string]any{}
	for _, r := range items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200)) {
		reqs[r["id"].(string)] = r
	}
	refs := func(m map[string]any) string {
		var out []string
		for _, x := range m["relations"].([]any) {
			r := x.(map[string]any)
			out = append(out, r["type"].(string)+"/"+r["direction"].(string)+"/"+r["key"].(string))
		}
		return strings.Join(out, ",")
	}
	if s := refs(reqs[api]); !strings.Contains(s, "depends_on/incoming/REL-REQ-2") || !strings.Contains(s, "provides/incoming/REL-REQ-3") || !strings.Contains(s, "verifies/incoming/REL-TC-2") {
		t.Errorf("api: %s", s)
	}
	if s := refs(reqs[ui]); s != "depends_on/outgoing/REL-REQ-1" {
		t.Errorf("ui: %s", s)
	}
	if reqs[api]["criteria_count"].(float64) != 1 || reqs[api]["relation_count"].(float64) != 2 {
		t.Errorf("counts: %v", reqs[api])
	}
	tests := map[string]map[string]any{}
	for _, x := range items(a.m("GET", "/api/v1/projects/"+pid+"/tests", nil, 200)) {
		tests[x["id"].(string)] = x
	}
	if s := refs(tests[caller]); s != "verifies/outgoing/REL-REQ-1,calls/outgoing/REL-TC-1" {
		t.Errorf("caller: %s", s)
	}
	if s := refs(tests[shared]); s != "calls/incoming/REL-TC-2" {
		t.Errorf("shared: %s", s)
	}
	if tests[caller]["step_count"].(float64) != 3 {
		t.Error(tests[caller])
	}
	// single GET includes the same references (called_by from the store)
	if s := refs(a.m("GET", "/api/v1/tests/"+shared, nil, 200)); s != "calls/incoming/REL-TC-2" {
		t.Errorf("get shared: %s", s)
	}
	if s := refs(a.m("GET", "/api/v1/requirements/"+ui, nil, 200)); s != "depends_on/outgoing/REL-REQ-1" {
		t.Errorf("get ui: %s", s)
	}
	// relations do not leak into the revision history
	a.m("PATCH", "/api/v1/requirements/"+api, map[string]any{"title": "API v2"}, 200)
	hist := items(a.m("GET", "/api/v1/requirements/"+api+"/history", nil, 200))
	if got := strings.Join(strs(hist[0]["changed_fields"]), ","); got != "title" {
		t.Errorf("changed fields: %s", got)
	}
}
