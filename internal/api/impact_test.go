package api

import (
	"io"
	"strings"
	"testing"
)

// impactFixture extends the correlation fixture: D refines B (two steps
// from A), a risk of A owned by owner@example.com and an open defect of TB.
type impactFixture struct {
	*correlationFixture
	rd, risk, defect string
	owner            *client
}

func newImpactFixture(t *testing.T, e *env) *impactFixture {
	t.Helper()
	f := &impactFixture{correlationFixture: newCorrelationFixture(t, e)}
	a := f.a
	pp := "/api/v1/projects/" + f.pid
	e.member("owner@example.com", "tester")
	f.owner = e.login("owner@example.com")
	f.rd = a.m("POST", pp+"/requirements", map[string]any{"title": "remember the user", "description": "The system SHOULD remember the user."}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+f.rd+"/relations", map[string]any{"target_id": f.rb, "type": "refines"}, 201)
	f.risk = a.m("POST", pp+"/risks", map[string]any{"title": "Account takeover", "owner": "owner@example.com", "requirement_ids": []string{f.ra}}, 201)["id"].(string)
	f.defect = a.m("POST", pp+"/defects", map[string]any{"title": "Password login flaky", "severity": "minor", "links": []map[string]any{{"entity_type": "test", "entity_id": f.tb}}}, 201)["id"].(string)
	return f
}

func section(imp map[string]any, name string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, x := range imp[name].([]any) {
		it := x.(map[string]any)
		out[it["id"].(string)] = it
	}
	return out
}

func TestTransitiveImpactAPI(t *testing.T) {
	e := newEnv(t)
	f := newImpactFixture(t, e)
	a := f.a

	// the default depth 3: D through B, the risk and the defect (reviews)
	imp := a.m("GET", "/api/v1/requirements/"+f.ra+"/impact", nil, 200)
	reqs := section(imp, "requirements")
	if imp["depth"].(float64) != 3 || len(reqs) != 2 || reqs[f.rd]["depth"].(float64) != 2 || reqs[f.rd]["mode"] != "suspect" {
		t.Fatal(imp)
	}
	path := reqs[f.rd]["path"].([]any)
	if len(path) != 3 || path[1].(map[string]any)["edge"] != "refines" || path[2].(map[string]any)["id"] != f.rd {
		t.Fatal(path)
	}
	if r := section(imp, "risks")[f.risk]; r == nil || r["mode"] != "review" || r["owner"] != "owner@example.com" {
		t.Fatal(imp["risks"])
	}
	if d := section(imp, "defects")[f.defect]; d == nil || d["depth"].(float64) != 3 {
		t.Fatal(imp["defects"])
	}
	if imp["review"].(float64) != 2 || imp["by_kind"].(map[string]any)["requirements"].(float64) != 2 {
		t.Fatal(imp)
	}
	// depth 1: the one-level analysis
	one := a.m("GET", "/api/v1/requirements/"+f.ra+"/impact?depth=1", nil, 200)
	if len(one["requirements"].([]any)) != 1 || one["depth"].(float64) != 1 {
		t.Fatal(one)
	}
	a.m("GET", "/api/v1/requirements/"+f.ra+"/impact?depth=11", nil, 400)

	// a dry run answers the impact and saves nothing
	ver := f.req(f.ra)["version"]
	dry := a.m("PATCH", "/api/v1/requirements/"+f.ra+"?dry_run=impact", map[string]any{"description": "The system MUST log in with MFA."}, 200)
	di := dry["impact"].(map[string]any)
	if dry["dry_run"] != "impact" || dry["status"].(float64) != 200 || len(di["roots"].([]any)) != 1 || len(di["requirements"].([]any)) != 2 {
		t.Fatal(dry)
	}
	if got := f.req(f.ra); strings.Contains(got["description"].(string), "MFA") || got["version"] != ver {
		t.Fatal(got)
	}
	if b := f.req(f.rb); b["suspect_relations"] != nil && b["suspect_relations"].(float64) != 0 {
		t.Fatal(b)
	}
	a.m("PATCH", "/api/v1/requirements/"+f.ra+"?dry_run=yes", map[string]any{"description": "x"}, 400)
	a.m("PATCH", "/api/v1/requirements/"+f.ra+"?impact=maybe", map[string]any{"description": "x"}, 400)
	// a dry run of a change the server refuses answers the refusal
	a.m("PATCH", "/api/v1/requirements/"+f.ra+"?dry_run=impact", map[string]any{"title": ""}, 400)

	// skip: nothing marked
	a.m("PATCH", "/api/v1/requirements/"+f.ra+"?impact=skip", map[string]any{"description": "The system MUST log in with a passkey."}, 200)
	if b := f.req(f.rb); b["suspect_relations"] != nil || b["suspect_links"].(float64) != 0 {
		t.Fatal(b)
	}
	// notify: nothing marked, the owners hear of it
	a.m("PATCH", "/api/v1/requirements/"+f.ra+"?impact=notify", map[string]any{"description": "The system MUST log in with MFA."}, 200)
	if b := f.req(f.rb); b["suspect_relations"] != nil {
		t.Fatal(b)
	}
	var found bool
	for _, n := range items(f.owner.m("GET", "/api/v1/notifications", nil, 200)) {
		found = found || (n["event"] == "impact.review" && strings.Contains(n["body"].(string), "RISK"))
	}
	if !found {
		t.Fatal("owner not notified")
	}
	// suspect (default): two levels marked
	a.m("PATCH", "/api/v1/requirements/"+f.ra, map[string]any{"description": "The system MUST log in with a hardware key."}, 200)
	if d := f.req(f.rd); d["suspect_relations"].(float64) != 1 {
		t.Fatal(d)
	}
	var reason string
	for _, r := range items(a.m("GET", "/api/v1/requirements/"+f.rd+"/relations", nil, 200)) {
		reason, _ = r["suspect_reason"].(string)
	}
	if !strings.Contains(reason, "→ refines →") {
		t.Fatal(reason)
	}

	// a bulk status change only recommends reviews
	bulk := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/bulk?dry_run=impact", map[string]any{"ids": []string{f.rb}, "set": map[string]any{"status": "draft"}}, 200)
	bi := bulk["impact"].(map[string]any)
	if roots := bi["roots"].([]any); len(roots) != 1 || roots[0].(map[string]any)["review_only"] != true || bi["suspect"].(float64) != 0 {
		t.Fatal(bulk)
	}
	if b := f.req(f.rb); b["status"] == "draft" {
		t.Fatal("dry run saved")
	}
	// a bulk change of substance applies the impact once
	a.m("POST", "/api/v1/relations/"+f.relBA+"/confirm", nil, 200)
	a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/bulk", map[string]any{"ids": []string{f.ra}, "set": map[string]any{"obligation": "should"}}, 200)
	if b := f.req(f.rb); b["suspect_relations"].(float64) != 1 {
		t.Fatal(b)
	}

	// the impact analysis: items, a baseline diff, export
	ana := a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"items": []map[string]any{{"kind": "requirement", "id": f.rb}, {"kind": "test", "id": f.ta}}, "depth": 2}, 200)
	if len(ana["roots"].([]any)) != 2 || ana["depth"].(float64) != 2 {
		t.Fatal(ana)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"items": []map[string]any{{"kind": "risk", "id": f.risk}}}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"items": []map[string]any{{"kind": "requirement", "id": "missing"}}}, 404)
	a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"items": []map[string]any{{"kind": "requirement", "id": f.ra}}, "depth": 12}, 400)
	bl := a.m("POST", "/api/v1/projects/"+f.pid+"/baselines", map[string]any{"name": "B1"}, 201)["id"].(string)
	a.m("PATCH", "/api/v1/requirements/"+f.rb, map[string]any{"title": "log in with a passphrase"}, 200)
	a.m("PATCH", "/api/v1/requirements/"+f.rc, map[string]any{"status": "draft"}, 200)
	bd := a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"baseline_id": bl}, 200)
	if roots := bd["roots"].([]any); len(roots) != 2 {
		t.Fatal(bd)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/impact", map[string]any{"baseline_id": "missing"}, 404)
	for _, format := range []string{"csv", "pdf"} {
		res := a.raw("GET", "/api/v1/projects/"+f.pid+"/impact/export?format="+format+"&lang=sv&depth=2&items=requirement:"+f.ra, nil)
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || (format == "csv" && !strings.Contains(string(body), "Nyckel")) || (format == "pdf" && !strings.HasPrefix(string(body), "%PDF")) {
			t.Fatal(format, res.StatusCode, string(body[:min(len(body), 200)]))
		}
	}
	res := a.raw("GET", "/api/v1/projects/"+f.pid+"/impact/export?baseline="+bl, nil)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	a.m("GET", "/api/v1/projects/"+f.pid+"/impact/export?format=docx&items=requirement:"+f.ra, nil, 400)
	a.m("GET", "/api/v1/projects/"+f.pid+"/impact/export?depth=x&items=requirement:"+f.ra, nil, 400)

	// restoring the baseline: preview first
	rs := a.m("POST", "/api/v1/baselines/"+bl+"/restore?dry_run=impact", map[string]any{"items": []map[string]any{{"entity_type": "requirement", "entity_id": f.rb}}}, 200)
	if roots := rs["impact"].(map[string]any)["roots"].([]any); len(roots) != 1 {
		t.Fatal(rs)
	}
	if b := f.req(f.rb); b["title"] != "log in with a passphrase" {
		t.Fatal(b)
	}

	// settings
	sp := "/api/v1/projects/" + f.pid + "/correlation-settings"
	got := a.m("PUT", sp, map[string]any{"impact_depth": 1, "relation_modes": map[string]any{"refines": "review"}, "impact_rules": map[string]any{"risk": map[string]any{"direction": "off", "mode": "review"}}}, 200)
	if got["impact_depth"].(float64) != 1 || got["relation_modes"].(map[string]any)["refines"] != "review" || got["impact_rules"].(map[string]any)["defect"] == nil {
		t.Fatal(got)
	}
	imp = a.m("GET", "/api/v1/requirements/"+f.ra+"/impact", nil, 200)
	if len(imp["risks"].([]any)) != 0 || len(imp["requirements"].([]any)) != 1 || imp["requirements"].([]any)[0].(map[string]any)["mode"] != "review" {
		t.Fatal(imp)
	}
	a.m("PUT", sp, map[string]any{"impact_depth": 0.5}, 400)
	a.m("PUT", sp, map[string]any{"impact_rules": map[string]any{"risk": map[string]any{"mode": "suspect"}}}, 400)

	// without test.read the tests and what lies beyond them are left out
	e.member("reader@example.com", "viewer")
	v := e.login("reader@example.com")
	if vi := v.m("GET", "/api/v1/requirements/"+f.ra+"/impact", nil, 200); vi["tests"] == nil {
		t.Fatal(vi)
	}
}
