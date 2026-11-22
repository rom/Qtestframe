package api

import (
	"net/url"
	"slices"
	"testing"
)

// MITRE D3FEND and EMB3D: the lists, the coverage, the catalogue filters
// and details, and the d3fend and emb3d lists of project requirements,
// tests and risks (copied on import, edited, validated, queried and
// exported).
func TestCatalogFrameworks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("FWK")
	base := "/api/v1/projects/" + pid

	d3 := a.m("GET", "/api/v1/catalog/d3fend", nil, 200)
	if len(anyList(d3["items"])) < 250 || len(anyList(d3["tactics"])) != 7 || d3["version"] == "" {
		t.Fatalf("d3fend: %d items", len(anyList(d3["items"])))
	}
	if l := items(a.m("GET", "/api/v1/catalog/d3fend?q=multi-factor", nil, 200)); len(l) != 1 || l[0]["id"] != "D3-MFA" || l[0]["tactic"] != "Harden" || l[0]["parent_name"] != "Agent Authentication" {
		t.Fatalf("d3fend search: %v", l)
	}
	if l := items(a.m("GET", "/api/v1/catalog/d3fend?tactic=restore", nil, 200)); len(l) < 5 || l[0]["tactic"] != "Restore" {
		t.Fatalf("d3fend tactic: %v", l)
	}
	emb := a.m("GET", "/api/v1/catalog/emb3d", nil, 200)
	if len(anyList(emb["items"])) < 160 || len(anyList(emb["properties"])) < 50 || len(anyList(emb["categories"])) != 4 {
		t.Fatalf("emb3d: %d items", len(anyList(emb["items"])))
	}
	if l := items(a.m("GET", "/api/v1/catalog/emb3d?kind=threat&category=hardware", nil, 200)); len(l) < 10 || l[0]["kind"] != "threat" || l[0]["category"] != "hardware" {
		t.Fatalf("emb3d threats: %v", l)
	}
	if l := items(a.m("GET", "/api/v1/catalog/emb3d?q=secure+firmware+update", nil, 200)); len(l) != 1 || l[0]["id"] != "MID-026" || l[0]["kind"] != "mitigation" {
		t.Fatalf("emb3d search: %v", l)
	}
	cov := a.m("GET", "/api/v1/catalog/d3fend/coverage", nil, 200)
	if len(anyList(cov["tactics"])) != 7 || cov["risks"].(float64) < 100 {
		t.Fatalf("d3fend coverage: %v", cov["risks"])
	}
	ec := a.m("GET", "/api/v1/catalog/emb3d/coverage?catalogue=hardware", nil, 200)
	if len(anyList(ec["categories"])) != 4 || ec["mitigations"].(float64) < 30 {
		t.Fatalf("emb3d coverage: %v", ec["mitigations"])
	}

	// catalogue filters and details
	if l := items(a.m("GET", "/api/v1/catalog/requirements?d3fend=D3-MFA", nil, 200)); !slices.ContainsFunc(l, func(x map[string]any) bool { return x["id"] == "AUTH-06" }) {
		t.Fatalf("d3fend filter: %d", len(l))
	}
	if l := items(a.m("GET", "/api/v1/catalog/requirements?emb3d=TID-201", nil, 200)); !slices.ContainsFunc(l, func(x map[string]any) bool { return x["id"] == "HWSEC-01" }) {
		t.Fatalf("emb3d filter by threat finds the mitigation: %d", len(l))
	}
	if l := items(a.m("GET", "/api/v1/catalog/risks?emb3d=TID-327", nil, 200)); !slices.ContainsFunc(l, func(x map[string]any) bool { return x["id"] == "EMBS-R03" }) {
		t.Fatalf("risk emb3d filter: %d", len(l))
	}
	if l := items(a.m("GET", "/api/v1/catalog/tests?emb3d=MID-026", nil, 200)); len(l) == 0 {
		t.Fatal("test emb3d filter")
	}
	rd := a.m("GET", "/api/v1/catalog/requirements/HWSEC-01", nil, 200)
	if dt := anyList(rd["d3fend_techniques"]); len(dt) != 1 || dt[0].(map[string]any)["name"] != "Bootloader Authentication" || len(anyList(rd["emb3d_items"])) != 1 {
		t.Fatalf("requirement detail: %v %v", rd["d3fend_techniques"], rd["emb3d_items"])
	}
	td := a.m("GET", "/api/v1/catalog/tests/EMBS-T16", nil, 200)
	if len(anyList(td["d3fend_techniques"])) != 2 || len(anyList(td["emb3d_items"])) != 4 {
		t.Fatalf("test detail: %v", td["emb3d_items"])
	}
	kd := a.m("GET", "/api/v1/catalog/risks/AUTH-R01", nil, 200)
	defs := anyList(kd["defenses"])
	if len(defs) != 2 {
		t.Fatalf("defenses: %v", kd["defenses"])
	}
	d0 := defs[0].(map[string]any)
	if d0["technique"].(map[string]any)["id"] != "T1110.003" || len(anyList(d0["d3fend"])) == 0 || len(anyList(d0["requirements"])) == 0 {
		t.Fatalf("defense: %v", d0)
	}

	// an import copies the mappings
	res := a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"HWSEC-01"}, "risk_ids": []string{"EMBS-R03"}, "include_requirements": false}, 201)
	rid := anyList(res["requirements"])[0].(map[string]any)["id"].(string)
	rq := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if !slices.Equal(strList(rq["d3fend"]), []string{"D3-BA"}) || !slices.Equal(strList(rq["emb3d"]), []string{"MID-002"}) {
		t.Fatalf("imported requirement: %v %v", rq["d3fend"], rq["emb3d"])
	}
	tid := anyList(res["tests"])[0].(map[string]any)["id"].(string)
	if tc := a.m("GET", "/api/v1/tests/"+tid, nil, 200); !slices.Equal(strList(tc["d3fend"]), []string{"D3-BA"}) || len(strList(tc["emb3d"])) != 3 {
		t.Fatalf("imported test: %v %v", tc["d3fend"], tc["emb3d"])
	}
	kid := anyList(res["risks"])[0].(map[string]any)["id"].(string)
	if k := a.m("GET", "/api/v1/risks/"+kid, nil, 200); !slices.Equal(strList(k["emb3d"]), []string{"TID-327", "TID-202"}) {
		t.Fatalf("imported risk: %v", k["emb3d"])
	}

	// edited and validated
	rq = a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"d3fend": []string{" d3-mfa ", "D3-MFA", "D3-BA"}, "emb3d": []string{"tid-201", "MID-026"}}, 200)
	if !slices.Equal(strList(rq["d3fend"]), []string{"D3-MFA", "D3-BA"}) || !slices.Equal(strList(rq["emb3d"]), []string{"TID-201", "MID-026"}) {
		t.Fatalf("patched: %v %v", rq["d3fend"], rq["emb3d"])
	}
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"d3fend": []string{"D3-NOPE"}}, 400)
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"emb3d": []string{"PID-11"}}, 400)
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"emb3d": []string{"MID-999"}}, 400)
	a.m("PATCH", "/api/v1/risks/"+kid, map[string]any{"emb3d": []string{"MID-026"}}, 400)
	a.m("POST", base+"/requirements", map[string]any{"title": "Bad", "d3fend": []string{"T1110"}}, 400)
	a.m("POST", base+"/tests", map[string]any{"title": "Bad", "d3fend": []string{"D3-X"}}, 400)
	a.m("POST", base+"/risks", map[string]any{"title": "Bad", "emb3d": []string{"TID-000"}}, 400)
	plain := a.m("POST", base+"/requirements", map[string]any{"title": "Plain"}, 201)
	if plain["d3fend"] == nil || len(anyList(plain["d3fend"])) != 0 || plain["emb3d"] == nil {
		t.Fatalf("no mappings: %v", plain)
	}
	a.m("POST", base+"/tests", map[string]any{"title": "Secure boot", "d3fend": []string{"D3-BA"}, "emb3d": []string{"MID-001"}}, 201)
	a.m("POST", base+"/risks", map[string]any{"title": "Side channel", "emb3d": []string{"TID-101"}}, 201)
	// a change of the mappings is a change of the requirement's history
	hist := anyList(a.m("GET", "/api/v1/requirements/"+rid+"/history", nil, 200)["items"])
	if len(hist) < 2 {
		t.Fatalf("history: %v", hist)
	}

	// queried by id, parent technique, name, tactic and threat
	for qs, want := range map[string]int{"d3fend:D3-MFA": 1, "d3fend:D3-AA": 1, `d3fend:"bootloader authentication"`: 1, "d3fend:harden": 1, "defend:D3-BA": 1,
		"emb3d:TID-201": 1, "emb3d:MID-026": 1, `emb3d:"secure firmware update"`: 1, "d3fend:D3-NTA": 0} {
		if l := items(a.m("GET", base+"/requirements?q="+url.QueryEscape(qs), nil, 200)); len(l) != want {
			t.Errorf("requirements %s: %d, want %d", qs, len(l), want)
		}
	}
	for qs, want := range map[string]int{"d3fend:D3-BA": 2, "emb3d:MID-001": 1, "emb3d:TID-201": 2} {
		if l := items(a.m("GET", base+"/tests?q="+url.QueryEscape(qs), nil, 200)); len(l) != want {
			t.Errorf("tests %s: %d, want %d", qs, len(l), want)
		}
	}
	for qs, want := range map[string]int{"emb3d:TID-101": 1, "emb3d:hardware": 1, "threat:TID-327": 1, `emb3d:"out of bounds"`: 1} {
		if l := items(a.m("GET", base+"/risks?q="+url.QueryEscape(qs), nil, 200)); len(l) != want {
			t.Errorf("risks %s: %d, want %d", qs, len(l), want)
		}
	}
}
