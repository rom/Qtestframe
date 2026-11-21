package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/suggest"
)

func suggestValues(c *client, field, q, pid string, extra ...string) []string {
	c.e.t.Helper()
	v := url.Values{"field": {field}, "q": {q}}
	if pid != "" {
		v.Set("project_id", pid)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	out := []string{}
	for _, it := range items(c.m("GET", "/api/v1/suggest?"+v.Encode(), nil, 200)) {
		out = append(out, it["value"].(string)+"/"+it["source"].(string))
	}
	return out
}

func record(c *client, pid string, want int, entries ...string) map[string]any {
	c.e.t.Helper()
	var es []map[string]string
	for i := 0; i+1 < len(entries); i += 2 {
		es = append(es, map[string]string{"field": entries[i], "value": entries[i+1]})
	}
	return c.m("POST", "/api/v1/suggest/history", map[string]any{"project_id": pid, "entries": es}, want)
}

func TestSuggestHistoryAndPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	mia := e.member("mia@example.com", rbac.RoleManager)
	m := e.login("mia@example.com")
	p1, p2 := a.project("ONE"), a.project("TWO")

	record(m, p1, 200, "requirement.title", "  Login works ", "requirement.title", "Logout works", "requirement.title", "")
	record(m, p1, 200, "requirement.title", "Login works") // used twice
	record(m, p2, 200, "requirement.title", "The login page opens", "test.step.expected", "The dashboard is shown")

	got := suggestValues(m, "requirement.title", "log", p1)
	if strings.Join(got, "|") != "Login works/history|Logout works/history|The login page opens/history" {
		t.Fatalf("prefix matches (the more used first), then word starts: %v", got)
	}
	// word starts too, and the history of another readable project
	got = suggestValues(m, "requirement.title", "login", p1)
	if strings.Join(got, "|") != "Login works/history|The login page opens/history" {
		t.Fatalf("across projects: %v", got)
	}
	// another field, another user: nothing
	if got := suggestValues(m, "test.title", "log", p1); len(got) != 0 {
		t.Fatalf("other field: %v", got)
	}
	if got := suggestValues(a, "requirement.title", "log", p1); len(got) != 0 {
		t.Fatalf("other user: %v", got)
	}

	// TWO becomes restricted, mia is no member: its values are no longer suggested
	a.m("PUT", "/api/v1/projects/"+p2+"/access", map[string]any{"access": "restricted"}, 200)
	got = suggestValues(m, "requirement.title", "login", p1)
	if strings.Join(got, "|") != "Login works/history" {
		t.Fatalf("restricted project leaks: %v", got)
	}
	if got := suggestValues(m, "test.step.expected", "the", p1); len(got) != 0 {
		t.Fatalf("restricted project leaks a step: %v", got)
	}
	m.m("GET", "/api/v1/suggest?field=requirement.title&q=l&project_id="+p2, nil, 404)
	record(m, p2, 404, "requirement.title", "x")
	// a project member again: the values come back
	a.m("POST", "/api/v1/projects/"+p2+"/members", map[string]any{"user_id": mia.ID, "role": "viewer"}, 201)
	if got := suggestValues(m, "test.step.expected", "dash", p1); strings.Join(got, "|") != "The dashboard is shown/history" {
		t.Fatalf("member again: %v", got)
	}

	// validation
	m.m("GET", "/api/v1/suggest?field=nope&q=x&project_id="+p1, nil, 400)
	m.m("GET", "/api/v1/suggest?field=requirement.title&q=x", nil, 400)
	m.m("GET", "/api/v1/suggest?field=search&q=x&project_id="+p1, nil, 400)
	m.m("GET", "/api/v1/suggest?field=requirement.title&q=x&project_id=missing", nil, 404)
	record(m, p1, 400, "nope", "x")
	many := make([]map[string]string, maxSuggestEntries+1)
	for i := range many {
		many[i] = map[string]string{"field": "comment", "value": "x"}
	}
	m.m("POST", "/api/v1/suggest/history", map[string]any{"project_id": p1, "entries": many}, 400)

	// the search box belongs to no project
	m.m("POST", "/api/v1/suggest/history", map[string]any{"entries": []map[string]string{{"field": "search", "value": "status:draft"}}}, 200)
	if got := suggestValues(m, "search", "stat", ""); strings.Join(got, "|") != "status:draft/history" {
		t.Fatalf("search: %v", got)
	}

	// limits
	var es []string
	for i := range 12 {
		es = append(es, "comment", fmt.Sprintf("Looks good %02d", i))
	}
	record(m, p1, 200, es...)
	if got := suggestValues(m, "comment", "looks", p1); len(got) != suggest.MaxResults {
		t.Fatalf("at most 8: %d", len(got))
	}
	if got := suggestValues(m, "comment", "looks", p1, "limit", "3"); len(got) != 3 {
		t.Fatalf("limit 3: %d", len(got))
	}

	// clearing
	cnt := m.m("GET", "/api/v1/suggest/history", nil, 200)["count"].(float64)
	if cnt != 17 { // 4 titles and steps, 1 search, 12 comments
		t.Fatalf("count: %v", cnt)
	}
	if d := m.m("DELETE", "/api/v1/suggest/history", nil, 200)["deleted"].(float64); d != cnt {
		t.Fatalf("deleted %v of %v", d, cnt)
	}
	if got := suggestValues(m, "requirement.title", "log", p1); len(got) != 0 {
		t.Fatalf("after clearing: %v", got)
	}

	// the history goes with the membership
	record(m, p1, 200, "requirement.title", "Kept?")
	a.m("DELETE", "/api/v1/members/"+mia.ID, nil, 204)
	if n, _ := e.st.CountSuggestHistory(context.Background(), mia.ID); n != 0 {
		t.Fatalf("history after leaving the organisation: %d", n)
	}
}

func TestSuggestProjectValuesAndPreference(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	p1 := a.project("ONE")
	for _, src := range []string{"ISO 26262", "ISO 26262", "IEC 61508", ""} {
		a.m("POST", "/api/v1/projects/"+p1+"/requirements", map[string]any{"title": "R", "source": src, "owner": "@ann"}, 201)
	}
	got := suggestValues(a, "requirement.source", "", p1)
	if strings.Join(got, "|") != "ISO 26262/project|IEC 61508/project" {
		t.Fatalf("project values by frequency: %v", got)
	}
	if got := suggestValues(a, "requirement.owner", "an", p1); strings.Join(got, "|") != "@ann/project" {
		t.Fatalf("owner: %v", got)
	}
	// history and project values merge
	record(a, p1, 200, "requirement.source", "iso 26262", "requirement.source", "ISO 9001")
	got = suggestValues(a, "requirement.source", "iso", p1)
	if strings.Join(got, "|") != "iso 26262/history|ISO 9001/history" {
		t.Fatalf("merged: %v", got)
	}
	if got := suggestValues(a, "requirement.source", "iso", p1, "sources", "project"); strings.Join(got, "|") != "ISO 26262/project" {
		t.Fatalf("sources=project: %v", got)
	}

	pref := func(v any) {
		a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"autocomplete": v}}, 200)
	}
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"autocomplete": "sometimes"}}, 400)
	pref("history")
	if got := suggestValues(a, "requirement.source", "i", p1); strings.Join(got, "|") != "iso 26262/history|ISO 9001/history" {
		t.Fatalf("history only: %v", got)
	}
	pref("project")
	if got := suggestValues(a, "requirement.source", "i", p1); strings.Join(got, "|") != "ISO 26262/project|IEC 61508/project" {
		t.Fatalf("project only: %v", got)
	}
	if r := record(a, p1, 200, "requirement.source", "New one"); r["recorded"].(float64) != 0 {
		t.Fatalf("project only records nothing: %v", r)
	}
	pref("off")
	if got := suggestValues(a, "requirement.source", "i", p1); len(got) != 0 {
		t.Fatalf("off: %v", got)
	}
	if r := record(a, p1, 200, "requirement.source", "Another"); r["recorded"].(float64) != 0 {
		t.Fatalf("off records nothing: %v", r)
	}
	pref(nil)
	if r := record(a, p1, 200, "requirement.source", "Third"); r["recorded"].(float64) != 1 {
		t.Fatalf("on again: %v", r)
	}
	if n, _ := e.st.CountSuggestHistory(context.Background(), e.admin.ID); n != 3 {
		t.Fatalf("stored: %d", n)
	}

	// rate limit
	e.srv.suggestLimiter = newRateLimiter(1, time.Minute)
	suggestValues(a, "requirement.source", "i", p1)
	a.m("GET", "/api/v1/suggest?field=requirement.source&q=i&project_id="+p1, nil, 429)
}

func TestSuggestHistoryCapAndStore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	st := e.st
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	step := 0
	st.SetClock(func() time.Time { step++; return base.Add(time.Duration(step) * time.Second) })
	a := e.login("admin@example.com")
	p1 := a.project("ONE")
	var es []string
	for i := range maxSuggestEntries {
		es = append(es, "test.step.action", fmt.Sprintf("Step %03d", i))
	}
	record(a, p1, 200, es...)
	record(a, p1, 200, "test.step.action", "Newest")
	record(a, p1, 200, "test.step.action", "Step 000")
	rows, err := st.SuggestHistory(ctx, e.tenant.ID, e.admin.ID, "test.step.action")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != suggest.HistoryCap {
		t.Fatalf("cap: %d", len(rows))
	}
	kept := map[string]bool{}
	for _, r := range rows {
		kept[r.Value] = true
		if r.Value == "Step 000" && r.Uses != 2 {
			t.Fatalf("uses: %+v", r)
		}
	}
	// one of the least recently used went; the new and the re-used one stayed
	if !kept["Step 000"] || !kept["Newest"] {
		t.Fatalf("trimmed the wrong values")
	}
	// other users and organisations are untouched by clearing one organisation
	if n, err := st.ClearSuggestHistory(ctx, "other-tenant", e.admin.ID); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if n, err := st.ClearSuggestHistory(ctx, e.tenant.ID, e.admin.ID); err != nil || n != int64(suggest.HistoryCap) {
		t.Fatal(n, err)
	}
	if err := st.RecordSuggestHistory(ctx, e.tenant.ID, e.admin.ID, nil); err != nil {
		t.Fatal(err)
	}
	if vals, err := st.SuggestProjectValues(ctx, e.tenant.ID, p1, "comment"); err != nil || len(vals) != 0 {
		t.Fatal(vals, err)
	}
	vals, err := st.SuggestProjectValues(ctx, e.tenant.ID, p1, "risk.owner")
	if err != nil || len(vals) != 0 {
		t.Fatal(vals, err)
	}
}
