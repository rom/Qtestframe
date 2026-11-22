package api

import (
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

// Applicability in the item history, baselines, undo data and the trash.
func TestApplicabilityHistoryAndBaselines(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid
	hist := func(id string) []map[string]any { return items(a.m("GET", "/api/v1/requirements/"+id+"/history", nil, 200)) }

	// the change of R2 to EU is in its history, with the variants' names
	h := hist(f.r2)
	if len(h) != 2 || h[0]["action"] != "applicability" || !slices.Contains(toStrings(h[0]["changed_fields"]), "applicability") {
		t.Fatal(toJSON(h))
	}
	ap := h[0]["snapshot"].(map[string]any)["applicability"].(map[string]any)
	if ap["variant_ids"].([]any)[0] != f.eu || ap["variants"].([]any)[0].(map[string]any)["name"] != "EU" || ap["updated_at"] != nil {
		t.Fatal(ap)
	}
	if h[1]["snapshot"].(map[string]any)["applicability"] != nil {
		t.Fatal(h[1])
	}
	// saving the same applicability again adds no revision; the audit says before → after
	a.m("PUT", "/api/v1/requirements/"+f.r2+"/applicability", map[string]any{"variant_ids": []string{"EU"}}, 200)
	if len(hist(f.r2)) != 2 {
		t.Fatal(hist(f.r2))
	}
	var details string
	if err := e.st.DB.QueryRow(e.st.DB.Rebind(`SELECT details FROM audit_log WHERE action = 'requirement.applicability' AND entity_id = ? ORDER BY created_at LIMIT 1`), f.r2).Scan(&details); err != nil ||
		details != f.k[f.r2]+": all variants → EU" {
		t.Fatal(details, err)
	}
	// a content change keeps the applicability in the snapshot without reporting it
	a.m("PATCH", "/api/v1/requirements/"+f.r2, map[string]any{"title": "CE marking"}, 200)
	if h := hist(f.r2); h[0]["action"] != "update" || slices.Contains(toStrings(h[0]["changed_fields"]), "applicability") || h[0]["snapshot"].(map[string]any)["applicability"] == nil {
		t.Fatal(toJSON(h[0]))
	}

	// a baseline keeps the applicability; compare shows the difference
	b := a.m("POST", pp+"/baselines", map[string]any{"name": "B1"}, 201)["id"].(string)
	a.m("PUT", "/api/v1/requirements/"+f.r2+"/applicability", map[string]any{"variant_ids": []string{"US"}}, 200)
	cmp := a.m("GET", "/api/v1/baselines/"+b+"/compare", nil, 200)
	changed := cmp["changed"].([]any)
	if len(changed) != 1 || changed[0].(map[string]any)["entity_id"] != f.r2 || toStrings(changed[0].(map[string]any)["changed_fields"])[0] != "applicability" {
		t.Fatal(toJSON(cmp))
	}
	// restoring from the baseline restores the applicability (the content is unchanged)
	res := a.m("POST", "/api/v1/baselines/"+b+"/restore", map[string]any{"items": []map[string]any{{"entity_type": "requirement", "entity_id": f.r2}, {"entity_type": "requirement", "entity_id": f.r1}}}, 200)
	if len(res["restored"].([]any)) != 1 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res)
	}
	if ap := a.m("GET", "/api/v1/requirements/"+f.r2+"/applicability", nil, 200); ap["variant_ids"].([]any)[0] != f.eu {
		t.Fatal(ap)
	}
	if cmp := a.m("GET", "/api/v1/baselines/"+b+"/compare", nil, 200); len(cmp["changed"].([]any)) != 0 {
		t.Fatal(toJSON(cmp))
	}

	// restoring the first version of R2 makes it apply to all variants again
	h = hist(f.r2)
	first := h[len(h)-1]["id"].(string)
	a.m("POST", "/api/v1/requirements/"+f.r2+"/restore", map[string]any{"revision_id": first}, 200)
	if ap := a.m("GET", "/api/v1/requirements/"+f.r2+"/applicability", nil, 200); ap["all"] != true {
		t.Fatal(ap)
	}
	if h := hist(f.r2); h[0]["action"] != "restore" || !slices.Contains(toStrings(h[0]["changed_fields"]), "applicability") {
		t.Fatal(toJSON(h[0]))
	}

	// the bulk change returns what the items had (to undo it)
	bulk := a.m("PUT", pp+"/applicability", map[string]any{"item_type": "test", "item_ids": []string{f.t1, f.t3}, "variant_ids": []string{"US"}}, 200)
	prev := bulk["previous"].([]any)
	if len(prev) != 2 || len(prev[0].(map[string]any)["variant_ids"].([]any)) != 0 || prev[1].(map[string]any)["variant_ids"].([]any)[0] != f.eu {
		t.Fatal(toJSON(bulk))
	}
	if th := items(a.m("GET", "/api/v1/tests/"+f.t3+"/history", nil, 200)); th[0]["action"] != "applicability" {
		t.Fatal(toJSON(th))
	}

	// the trash keeps the applicability of a deleted item
	del := a.m("DELETE", pp+"/tests/bulk", map[string]any{"ids": []string{f.t3}}, 200)
	a.m("POST", "/api/v1/trash/"+del["trash_id"].(string)+"/restore", nil, 200)
	if ap := a.m("GET", "/api/v1/tests/"+f.t3+"/applicability", nil, 200); ap["variant_ids"].([]any)[0] != f.us {
		t.Fatal(ap)
	}

	// runs: the query field variant
	run := a.m("POST", pp+"/runs", map[string]any{"name": "US run", "variant_id": f.us, "test_ids": []string{f.t1}}, 201)["id"].(string)
	a.m("POST", pp+"/runs", map[string]any{"name": "Neutral", "test_ids": []string{f.t1}}, 201)
	if rl := items(a.m("GET", pp+"/runs?q=variant:US", nil, 200)); len(rl) != 1 || rl[0]["id"] != run {
		t.Fatal(rl)
	}
}

// Release gates per variant and waivers limited to variants.
func TestReleaseGatesPerVariant(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("mgr@example.com", rbac.RoleManager)
	mgr := e.login("mgr@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid

	// results: a neutral run passes T1, a US run fails T2, an EU run passes T3
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "3.0", "variant_ids": []string{"EU", f.us}, "exit_criteria": []map[string]any{
		{"type": "coverage", "threshold": 100}, {"type": "passed", "threshold": 100}, {"type": "no_failed"}}}, 201)
	id := rel["id"].(string)
	if vs := rel["variants"].([]any); len(vs) != 2 || vs[0].(map[string]any)["name"] != "EU" {
		t.Fatal(rel)
	}
	a.m("POST", pp+"/releases", map[string]any{"name": "x", "variant_ids": []string{"Mars"}}, 400)
	a.m("POST", "/api/v1/releases/"+id+"/requirements", map[string]any{"add": []string{f.r1, f.r2, f.r3}}, 200)
	for _, r := range []struct{ variant, test, status string }{{"", f.t1, "passed"}, {f.us, f.t2, "failed"}, {f.eu, f.t3, "passed"}} {
		body := map[string]any{"name": "run", "test_ids": []string{r.test}, "release_id": id}
		if r.variant != "" {
			body["variant_id"] = r.variant
		}
		run := a.m("POST", pp+"/runs", body, 201)["id"].(string)
		a.m("POST", "/api/v1/runs/"+run+"/results", map[string]any{"results": []map[string]any{{"test_key": f.k[r.test], "status": r.status}}}, 200)
	}
	gate := func(st map[string]any, name string) map[string]any {
		for _, g := range st["variants"].([]any) {
			if g.(map[string]any)["variant"].(map[string]any)["name"] == name {
				return g.(map[string]any)
			}
		}
		t.Fatal("no gate for", name, toJSON(st))
		return nil
	}
	states := func(g map[string]any) string {
		var out []string
		for _, c := range g["criteria"].([]any) {
			out = append(out, c.(map[string]any)["state"].(string))
		}
		return strings.Join(out, ",")
	}
	st := a.m("GET", "/api/v1/releases/"+id+"/status", nil, 200)
	eu, us := gate(st, "EU"), gate(st, "US")
	// EU: R1 (T1) and R2 (T3) pass; US: R1 fails through T2, R3 has no test
	if eu["go"] != true || states(eu) != "pass,pass,pass" || us["go"] != false || states(us) != "fail,fail,fail" || st["go"] != false {
		t.Fatal(toJSON(st))
	}
	if st["summary"].(map[string]any)["variants"] != float64(2) || st["summary"].(map[string]any)["variants_go"] != float64(1) {
		t.Fatal(st["summary"])
	}
	// the combined criteria: unmet, with the variants of each item
	cov := st["criteria"].([]any)[0].(map[string]any)
	if cov["met"] != false || cov["state"] != "fail" || cov["items"].([]any)[0].(map[string]any)["variants"].([]any)[0] != "US" {
		t.Fatal(toJSON(cov))
	}
	// one variant only, as before
	if one := a.m("GET", "/api/v1/releases/"+id+"/status?variant=EU", nil, 200); one["go"] != true || one["variants"] != nil {
		t.Fatal(toJSON(one))
	}

	// a waiver for R1 and R3 limited to the EU does not help the US
	waive := func(variants []string) string {
		w := a.m("POST", pp+"/waivers", map[string]any{"title": "US later", "requirement_ids": []string{f.r1, f.r3}, "variant_ids": variants,
			"justification": "UL listing next quarter", "submit": true}, 201)
		mgr.m("POST", "/api/v1/waivers/"+w["id"].(string)+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 200)
		return w["id"].(string)
	}
	w1 := waive([]string{"EU"})
	if w := a.m("GET", "/api/v1/waivers/"+w1, nil, 200); w["variants"].([]any)[0].(map[string]any)["name"] != "EU" {
		t.Fatal(w)
	}
	if st := a.m("GET", "/api/v1/releases/"+id+"/status", nil, 200); gate(st, "US")["go"] != false || states(gate(st, "EU")) != "pass,pass,pass" {
		t.Fatal(toJSON(st))
	}
	// one limited to the US makes the US a go (waived) and the release a go
	waive([]string{"US"})
	st = a.m("GET", "/api/v1/releases/"+id+"/status", nil, 200)
	if us := gate(st, "US"); us["go"] != true || states(us) != "waived,waived,waived" || st["go"] != true {
		t.Fatal(toJSON(st))
	}
	if len(st["waivers"].([]any)) != 2 {
		t.Fatal(st["waivers"])
	}
	// a variant-limited waiver does not count for a release evaluated without variants
	a.m("PATCH", "/api/v1/releases/"+id, map[string]any{"variant_ids": []string{}}, 200)
	if st := a.m("GET", "/api/v1/releases/"+id+"/status", nil, 200); st["variants"] != nil || st["go"] != false {
		t.Fatal(toJSON(st))
	}
	a.m("PATCH", "/api/v1/releases/"+id, map[string]any{"variant_ids": []string{"EU", "US"}}, 200)

	// the release readiness report has the matrix criteria × variants
	_, data := a.download(pp+"/reports/generate", map[string]any{"template": "release", "format": "md", "params": map[string]string{"release_id": id}}, 200)
	md := string(data)
	if !strings.Contains(md, "Exit criteria per variant") || !strings.Contains(md, "| Waived") || !strings.Contains(md, "2 of 2") {
		t.Fatal(md)
	}
	_, data = a.download(pp+"/reports/generate", map[string]any{"template": "release", "format": "md", "lang": "sv", "params": map[string]string{"release_id": id}}, 200)
	if !strings.Contains(string(data), "Slutkriterier per variant") {
		t.Fatal(string(data))
	}

	// deleting a targeted variant: the release no longer targets it
	a.m("DELETE", "/api/v1/variants/"+f.us, nil, 204)
	if r := a.m("GET", "/api/v1/releases/"+id, nil, 200); len(r["variant_ids"].([]any)) != 1 {
		t.Fatal(r)
	}
}
