package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBulkWebhooks(t *testing.T) {
	e := newEnv(t)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	a := e.login("admin@example.com")
	pid := a.project("BWH")
	mk := func(name, url string, events []string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/webhooks", map[string]any{"name": name, "url": url, "events": events, "enabled": true}, 201)["id"].(string)
	}
	h1 := mk("Chat", ok.URL, []string{"defect.created"})
	h2 := mk("CI", bad.URL, []string{"run.completed", "defect.created"})
	path := "/api/v1/projects/" + pid + "/webhooks/bulk"

	a.m("POST", path, map[string]any{"ids": []string{h1}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{h1}, "add_events": []string{"nope"}}, 400)
	// removing the only event of Chat is refused for both
	a.m("POST", path, map[string]any{"ids": []string{h1, h2}, "remove_events": []string{"defect.created"}}, 400)
	for _, w := range items(a.m("GET", "/api/v1/projects/"+pid+"/webhooks", nil, 200)) {
		if want := map[string]int{"Chat": 1, "CI": 2}[w["name"].(string)]; len(w["events"].([]any)) != want {
			t.Fatal("refused change was saved", w)
		}
	}
	res := a.m("POST", path, map[string]any{"ids": []string{h1, h2}, "enabled": false, "add_events": []string{"release.released"}}, 200)
	if res["updated"].(float64) != 2 {
		t.Fatal(res)
	}
	for _, w := range items(a.m("GET", "/api/v1/projects/"+pid+"/webhooks", nil, 200)) {
		ev := strs(w["events"])
		if w["enabled"] != false || !contains(ev, "release.released") || w["has_secret"] != true {
			t.Fatal(w)
		}
	}
	// test: one delivers, one fails; outcomes are recorded
	tr := a.m("POST", "/api/v1/projects/"+pid+"/webhooks/test", map[string]any{"ids": []string{h1, h2}}, 200)
	results := tr["results"].([]any)
	if len(results) != 2 || results[0].(map[string]any)["ok"] != true || results[1].(map[string]any)["ok"] != false || results[1].(map[string]any)["status"].(float64) != 500 {
		t.Fatal(tr)
	}
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", path, map[string]any{"ids": []string{h1}, "enabled": true}, 403)
	if res := a.m("DELETE", path, map[string]any{"ids": []string{h1, h2}}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	a.m("DELETE", path, map[string]any{"ids": []string{h1}}, 404)
}
