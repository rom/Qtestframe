package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestInReview(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("INR")
	o := e.member("olle@example.com", rbac.RoleTester) // may not review anything
	e.member("vera@example.com", rbac.RoleViewer)
	req := func(title, status, owner string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "status": status, "owner": owner}, 201)["id"].(string)
	}
	byHandle := req("Owned by handle", "review", "@olle")
	byEmail := req("Owned by e-mail", "review", "Olle@Example.com")
	other := req("Someone else's", "review", "Anna")
	req("Owned but draft", "draft", "@olle")

	ol := e.login("olle@example.com")
	mine := ol.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Olle's test", "status": "review"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Admin's test", "status": "review"}, 201)
	// an explicit review request to Olle for someone else's requirement
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": other, "reviewers": []string{o.ID}}, 201)

	reasons := func(c *client) map[string][]string {
		out := map[string][]string{}
		for _, it := range items(c.m("GET", "/api/v1/reviews/in-review", nil, 200)) {
			out[it["entity_id"].(string)] = strs(it["reasons"])
			if it["project_key"] != "INR" || it["key"] == "" {
				t.Fatal(it)
			}
		}
		return out
	}
	got := reasons(ol)
	if len(got) != 4 {
		t.Fatal(got)
	}
	want := map[string][]string{byHandle: {"owner"}, byEmail: {"owner"}, other: {"requested"}, mine: {"author"}}
	for id, w := range want {
		if g := got[id]; len(g) != len(w) || g[0] != w[0] {
			t.Fatal(id, g)
		}
	}
	// the admin may review everything: all five items in review
	all := reasons(a)
	if len(all) != 5 {
		t.Fatal(all)
	}
	for _, r := range all {
		if r[len(r)-1] != "reviewer" {
			t.Fatal(r)
		}
	}
	// a viewer is concerned by nothing
	if n := len(reasons(e.login("vera@example.com"))); n != 0 {
		t.Fatal(n)
	}
	// once approved, an item leaves the list
	a.m("PATCH", "/api/v1/requirements/"+byHandle, map[string]any{"status": "approved"}, 200)
	if _, ok := reasons(ol)[byHandle]; ok {
		t.Fatal("approved item still listed")
	}
}
