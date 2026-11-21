package api

import "testing"

// The "Highlight obligation keywords" preference is on by default and
// stored as hide_obligation_highlight.
func TestObligationHighlightPreference(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	prefsOf := func(me map[string]any) map[string]any {
		return me["user"].(map[string]any)["preferences"].(map[string]any)
	}
	if _, ok := prefsOf(a.m("GET", "/api/v1/auth/me", nil, 200))["hide_obligation_highlight"]; ok {
		t.Fatal("highlighting must be on by default")
	}
	me := a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"hide_obligation_highlight": true}}, 200)
	if prefsOf(me)["hide_obligation_highlight"] != true {
		t.Fatal(me)
	}
	me = a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"hide_obligation_highlight": nil}}, 200)
	if _, ok := prefsOf(me)["hide_obligation_highlight"]; ok {
		t.Fatal(me)
	}
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"hide_obligation_highlight": "yes"}}, 400)
}
