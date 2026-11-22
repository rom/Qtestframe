package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestExploratorySessionFlow(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	e.st.SetClock(func() time.Time { return now })
	a := e.login("admin@example.com")
	e.member("tester@example.com", rbac.RoleTester)
	// a role that runs sessions but may not plan or change others' (no run.write)
	a.m("POST", "/api/v1/roles", map[string]any{"name": "explorer", "permissions": []string{"project.read", "test.read", "requirement.read", "run.read", "run.execute"}}, 201)
	e.member("other@example.com", "explorer")
	e.member("lead@example.com", rbac.RoleTestLead)
	e.member("viewer@example.com", rbac.RoleViewer)
	tester, other, viewer, lead := e.login("tester@example.com"), e.login("other@example.com"), e.login("viewer@example.com"), e.login("lead@example.com")
	pid := a.project("EXP")
	base := "/api/v1/projects/" + pid
	area := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Checkout"}, 201)["id"].(string)
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Pay by card", "folder_id": area, "status": "implemented"}, 201)["id"].(string)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "Card payment"}, 201)["id"].(string)
	other2 := e.login("admin@example.com").project("OTH")
	foreign := a.m("POST", "/api/v1/projects/"+other2+"/requirements", map[string]any{"title": "elsewhere"}, 201)["id"].(string)

	// validation and permissions
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": ""}, 400)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x", "timebox_minutes": 1}, 400)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x", "tester": "no-at"}, 400)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x", "tester": "stranger@example.com"}, 400)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x", "scope": []any{map[string]any{"type": "requirement", "id": foreign}}}, 400)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x", "scope": []any{map[string]any{"type": "nope", "id": r1}}}, 400)
	viewer.m("POST", base+"/exploratory-sessions", map[string]any{"title": "x"}, 403)

	x := a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "Explore checkout with expired cards", "mission": "Explore payment with expired and blocked cards to find unclear errors",
		"tester": "Tester@example.com", "timebox_minutes": 90, "scope": []any{
			map[string]any{"type": "folder", "id": area}, map[string]any{"type": "requirement", "id": r1}, map[string]any{"type": "test", "id": t1},
			map[string]any{"type": "requirement", "id": r1}}}, 201)
	id := x["id"].(string)
	sp := "/api/v1/exploratory-sessions/" + id
	if x["key"] != "EXP-EXP-1" || x["status"] != "planned" || x["tester"] != "tester@example.com" || len(x["scope"].([]any)) != 3 || x["timebox_minutes"].(float64) != 90 {
		t.Fatal(x)
	}
	viewer.m("GET", sp, nil, 200)

	// notes need a started session; only the tester (or run.write) runs it
	tester.m("POST", sp+"/notes", map[string]any{"type": "observation", "body": "too early"}, 409)
	viewer.m("POST", sp+"/timer", map[string]any{"action": "start"}, 403)
	other.m("POST", sp+"/timer", map[string]any{"action": "start"}, 403) // not the tester
	tester.m("POST", sp+"/timer", map[string]any{"action": "pause"}, 409)
	tester.m("POST", sp+"/timer", map[string]any{"action": "explode"}, 400)
	x = tester.m("POST", sp+"/timer", map[string]any{"action": "start"}, 200)
	if x["status"] != "in_progress" || x["running_since"] == nil {
		t.Fatal(x)
	}
	now = now.Add(10 * time.Minute)
	// the timer survives reloads: the server computes the elapsed time
	if g := tester.m("GET", sp, nil, 200); g["elapsed_seconds"].(float64) != 600 {
		t.Fatal(g)
	}
	n1 := tester.m("POST", sp+"/notes", map[string]any{"type": "question", "body": "Should an expired card show the date?"}, 201)
	if n1["elapsed_seconds"].(float64) != 600 || n1["type"] != "question" {
		t.Fatal(n1)
	}
	tester.m("POST", sp+"/notes", map[string]any{"type": "rant", "body": "x"}, 400)
	tester.m("POST", sp+"/notes", map[string]any{"type": "idea", "body": "  "}, 400)
	tester.m("POST", sp+"/timer", map[string]any{"action": "pause"}, 200)
	now = now.Add(30 * time.Minute) // paused time does not count
	tester.m("POST", sp+"/timer", map[string]any{"action": "start"}, 200)
	now = now.Add(5 * time.Minute)

	// an attachment (screenshot) referenced by a note
	f := tester.upload("POST", sp+"/attachments", "file", "error.png", "image/png", []byte("\x89PNG fake"), nil, 201)
	if links := f["links"].([]any); len(links) != 1 || links[0].(map[string]any)["key"] != "EXP-EXP-1" {
		t.Fatal(f)
	}
	viewer.upload("POST", sp+"/attachments", "file", "x.png", "image/png", []byte("x"), nil, 403)
	if l := items(viewer.m("GET", sp+"/attachments", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	bug := tester.m("POST", sp+"/notes", map[string]any{"type": "issue", "body": "Expired card error is a raw code\nShows E_CARD_042", "file_ids": []string{f["id"].(string)}}, 201)
	if bug["elapsed_seconds"].(float64) != 900 || len(bug["files"].([]any)) != 1 {
		t.Fatal(bug)
	}
	tester.m("POST", sp+"/notes", map[string]any{"type": "idea", "body": "x", "file_ids": []string{"nope"}}, 400)
	other.m("PATCH", sp+"/notes/"+n1["id"].(string), map[string]any{"body": "mine now"}, 403)
	if u := tester.m("PATCH", sp+"/notes/"+n1["id"].(string), map[string]any{"body": "Should an expired card show its date?"}, 200); u["body"] != "Should an expired card show its date?" {
		t.Fatal(u)
	}
	tmp := tester.m("POST", sp+"/notes", map[string]any{"type": "idea", "body": "delete me"}, 201)
	tester.m("DELETE", sp+"/notes/"+tmp["id"].(string), nil, 204)
	tester.m("DELETE", sp+"/notes/"+tmp["id"].(string), nil, 404)

	// a defect raised from a note: prefilled, linked to the session and the scoped requirements
	d := tester.m("POST", sp+"/notes/"+bug["id"].(string)+"/defect", map[string]any{"severity": "minor"}, 201)
	if d["title"] != "Expired card error is a raw code" || d["severity"] != "minor" || d["found_in"] != "test" || !strings.Contains(d["description"].(string), "EXP-EXP-1") {
		t.Fatal(d)
	}
	linked := map[string]bool{}
	for _, l := range d["links"].([]any) {
		m := l.(map[string]any)
		linked[m["entity_type"].(string)+":"+m["entity_id"].(string)] = true
	}
	if !linked[model.KindExploratory+":"+id] || !linked["requirement:"+r1] {
		t.Fatal(d["links"])
	}
	tester.m("POST", sp+"/notes/"+bug["id"].(string)+"/defect", nil, 409)
	notes := items(tester.m("GET", sp+"/notes", nil, 200))
	if len(notes) != 2 || notes[1]["defect_key"] != d["key"] || notes[1]["type"] != "bug" {
		t.Fatal(notes)
	}
	if ds := items(a.m("GET", "/api/v1/requirements/"+r1+"/defects", nil, 200)); len(ds) != 1 {
		t.Fatal(ds)
	}

	// debrief needs a stopped session; the time split adds up to 100
	deb := map[string]any{"summary": "Card errors need work", "coverage_pct": 70, "setup_pct": 10, "testing_pct": 60, "bug_pct": 30, "outcome": "failed"}
	tester.m("PUT", sp+"/debrief", deb, 409)
	x = tester.m("POST", sp+"/timer", map[string]any{"action": "stop"}, 200)
	if x["status"] != "completed" || x["elapsed_seconds"].(float64) != 900 || x["running_since"] != nil || x["ended_at"] == nil {
		t.Fatal(x)
	}
	now = now.Add(time.Hour)
	if g := tester.m("GET", sp, nil, 200); g["elapsed_seconds"].(float64) != 900 || g["bug_count"].(float64) != 1 || g["note_count"].(float64) != 2 {
		t.Fatal(g)
	}
	tester.m("PUT", sp+"/debrief", map[string]any{"setup_pct": 10, "testing_pct": 10, "bug_pct": 10, "outcome": "failed"}, 400)
	tester.m("PUT", sp+"/debrief", map[string]any{"setup_pct": 10, "testing_pct": 60, "bug_pct": 30, "outcome": "maybe"}, 400)
	other.m("PUT", sp+"/debrief", deb, 403)
	lead.m("PUT", sp+"/debrief", deb, 200) // a reviewer debrief (run.write)
	x = tester.m("PUT", sp+"/debrief", deb, 200)
	db := x["debrief"].(map[string]any)
	if x["status"] != "debriefed" || db["outcome"] != "failed" || db["by"] != "tester@example.com" || db["reviewed"] != false {
		t.Fatal(x)
	}
	// frozen after the debrief
	tester.m("POST", sp+"/notes", map[string]any{"body": "late"}, 409)
	a.m("PATCH", sp, map[string]any{"title": "renamed"}, 409)
	x = a.m("PUT", sp+"/debrief", map[string]any{"summary": "Reviewed", "coverage_pct": 60, "setup_pct": 10, "testing_pct": 60, "bug_pct": 30, "outcome": "failed", "comment": "Agree"}, 200)
	if db := x["debrief"].(map[string]any); db["reviewed"] != true || db["comment"] != "Agree" {
		t.Fatal(x)
	}

	// history and audit
	hist := items(a.m("GET", sp+"/history", nil, 200))
	if len(hist) < 4 || hist[0]["action"] != "debrief" {
		t.Fatal(hist)
	}
	audit := items(a.m("GET", "/api/v1/audit?q=action:exploratory.*", nil, 200))
	if len(audit) < 5 {
		t.Fatal(audit)
	}

	// list, filters, query language
	tester.m("POST", base+"/exploratory-sessions", map[string]any{"title": "Explore refunds", "timebox_minutes": 30}, 201)
	a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "Unassigned", "tester": ""}, 201)
	if l := items(a.m("GET", base+"/exploratory-sessions", nil, 200)); len(l) != 3 {
		t.Fatal(l)
	}
	if l := items(tester.m("GET", base+"/exploratory-sessions?mine=true", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/exploratory-sessions?q=area:Checkout%20outcome:failed", nil, 200)); len(l) != 1 || l[0]["id"] != id {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/exploratory-sessions?q=title:refunds", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/exploratory-sessions?q=charter:blocked", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/exploratory-sessions?requirement_id="+r1, nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", base+"/exploratory-sessions?status=bogus", nil, 400)
	a.m("GET", base+"/exploratory-sessions?q=nofield:1", nil, 400)
	// the work list of a tester
	mine, err := e.st.ListUserExploratorySessions(context.Background(), e.tenant.ID, "TESTER@example.com")
	if err != nil || len(mine) != 1 || mine[0].Title != "Explore refunds" {
		t.Fatal(mine, err)
	}

	// a session without tester becomes the starter's
	un := items(a.m("GET", base+"/exploratory-sessions?q=title:Unassigned", nil, 200))[0]
	if s := other.m("POST", "/api/v1/exploratory-sessions/"+un["id"].(string)+"/timer", map[string]any{"action": "start"}, 200); s["tester"] != "other@example.com" {
		t.Fatal(s)
	}

	// metrics: sessions per area and bugs per session hour
	m := a.m("GET", base+"/exploratory-metrics", nil, 200)
	if m["sessions"].(float64) != 3 || m["bugs"].(float64) != 1 || m["bugs_per_hour"].(float64) != 4 || m["avg_testing_pct"].(float64) != 60 {
		t.Fatal(m)
	}
	areas := m["by_area"].([]any)
	if len(areas) != 2 || areas[0].(map[string]any)["id"] != "" || areas[0].(map[string]any)["sessions"].(float64) != 2 ||
		areas[1].(map[string]any)["name"] != "Checkout" || areas[1].(map[string]any)["bugs_per_hour"].(float64) != 4 {
		t.Fatal(areas)
	}

	// report export through the report generation
	res := a.raw("GET", sp+"/report?format=md", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "Expired card error is a raw code") || !strings.Contains(string(body), "Bug investigation") {
		t.Fatal(res.StatusCode, string(body))
	}
	res = a.raw("GET", sp+"/report?format=pdf&lang=sv", nil)
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(string(body), "%PDF") {
		t.Fatal(res.StatusCode)
	}
	viewer.raw("GET", sp+"/report", nil).Body.Close()
	for _, tpl := range []string{"exploratory_sessions", "exploratory_session"} {
		res = a.raw("POST", base+"/reports/generate", map[string]any{"template": tpl, "format": "md"})
		body, _ = io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "EXP-EXP-1") {
			t.Fatal(tpl, res.StatusCode, string(body))
		}
	}
	a.m("POST", base+"/reports/generate", map[string]any{"template": "exploratory_session", "format": "md", "params": map[string]any{"session_id": "nope"}}, 400)

	// reopen and delete
	tester.m("DELETE", sp+"/debrief", nil, 200) // run.write
	a.m("DELETE", sp+"/debrief", nil, 409)
	viewer.m("DELETE", sp, nil, 403)
	a.m("DELETE", sp, nil, 204)
	a.m("GET", sp, nil, 404)
	if dd := a.m("GET", "/api/v1/defects/"+d["id"].(string), nil, 200); len(dd["links"].([]any)) != 1 {
		t.Fatal(dd["links"])
	}
	a.m("GET", "/api/v1/files/"+f["id"].(string), nil, 404) // the attachment went with the session
}

func TestExploratoryVerificationEvidence(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	e.st.SetClock(func() time.Time { return now })
	a := e.login("admin@example.com")
	pid := a.project("EXV")
	base := "/api/v1/projects/" + pid
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Plain"}, 201)["id"].(string)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "With criteria", "criteria": []any{map[string]any{"description": "c1", "method": "test"}}}, 201)["id"].(string)
	run := func(outcome string) string {
		x := a.m("POST", base+"/exploratory-sessions", map[string]any{"title": "S " + outcome, "scope": []any{
			map[string]any{"type": "requirement", "id": r1}, map[string]any{"type": "requirement", "id": r2}}}, 201)
		sp := "/api/v1/exploratory-sessions/" + x["id"].(string)
		a.m("POST", sp+"/timer", map[string]any{"action": "start"}, 200)
		now = now.Add(20 * time.Minute)
		a.m("POST", sp+"/timer", map[string]any{"action": "stop"}, 200)
		a.m("PUT", sp+"/debrief", map[string]any{"setup_pct": 20, "testing_pct": 70, "bug_pct": 10, "outcome": outcome, "coverage_pct": 80}, 200)
		return x["id"].(string)
	}
	run("passed")
	state := func(id string) map[string]any {
		return a.m("GET", "/api/v1/requirements/"+id+"/verification", nil, 200)
	}
	// off by default: sessions are no evidence
	if v := state(r1); v["own"] != model.VerNone {
		t.Fatal(v)
	}
	st := a.m("PUT", base+"/verification-settings", map[string]any{"exploratory_evidence": true}, 200)
	if st["exploratory_evidence"] != true || st["strict_criteria"] != false {
		t.Fatal(st)
	}
	// a partial PUT keeps the other setting
	a.m("PUT", base+"/verification-settings", map[string]any{"strict_criteria": true}, 200)
	if g := a.m("GET", base+"/verification-settings", nil, 200); g["exploratory_evidence"] != true || g["strict_criteria"] != true {
		t.Fatal(g)
	}
	a.m("PUT", base+"/verification-settings", map[string]any{"strict_criteria": false}, 200)
	v := state(r1)
	if v["own"] != model.VerVerified || v["evidence"].([]any)[0].(map[string]any)["kind"] != "exploratory" {
		t.Fatal(v)
	}
	// conservative: never spread to criteria
	if v := state(r2); v["verified"] != false || v["criteria"].([]any)[0].(map[string]any)["state"] != model.VerNone {
		t.Fatal(v)
	}
	// the newest counting session wins; inconclusive does not count
	now = now.Add(time.Hour)
	run("inconclusive")
	if v := state(r1); v["own"] != model.VerVerified {
		t.Fatal(v)
	}
	now = now.Add(time.Hour)
	run("failed")
	if v := state(r1); v["own"] != model.VerFailed {
		t.Fatal(v)
	}
	ev := state(r1)["evidence"].([]any)
	if len(ev) != 3 || ev[0].(map[string]any)["state"] != model.EvidenceFailed || ev[2].(map[string]any)["note"] != "superseded" {
		t.Fatal(ev)
	}
}
