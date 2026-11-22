package api

import (
	"net/url"
	"slices"
	"testing"
)

// MITRE ATT&CK: the technique list, the coverage of tactics, the risk
// filters, and the techniques of project risks (copied on import, edited,
// validated and queried).
func TestCatalogAttack(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ATK")
	base := "/api/v1/projects/" + pid

	tl := a.m("GET", "/api/v1/catalog/attack/techniques", nil, 200)
	if len(anyList(tl["items"])) < 900 || len(anyList(tl["tactics"])) < 40 || len(anyList(tl["matrices"])) != 4 {
		t.Fatalf("techniques: %d items, %d tactics", len(anyList(tl["items"])), len(anyList(tl["tactics"])))
	}
	q := items(a.m("GET", "/api/v1/catalog/attack/techniques?q=credential+stuffing", nil, 200))
	if len(q) != 1 || q[0]["id"] != "T1110.004" || q[0]["name"] != "Brute Force: Credential Stuffing" || q[0]["url"] == "" {
		t.Fatalf("search: %v", q)
	}
	if ics := items(a.m("GET", "/api/v1/catalog/attack/techniques?matrix=ics&tactic=ta0106", nil, 200)); len(ics) == 0 || ics[0]["matrix"] != "ics" {
		t.Fatalf("ics: %v", ics)
	}
	cov := anyList(a.m("GET", "/api/v1/catalog/attack", nil, 200)["matrices"])
	if len(cov) != 4 {
		t.Fatalf("coverage: %v", cov)
	}
	ent := cov[0].(map[string]any)
	if ent["id"] != "enterprise" || ent["risks"].(float64) < 100 || len(anyList(ent["tactics"])) != 15 {
		t.Fatalf("enterprise: %v", ent["risks"])
	}

	// the catalogue risks by technique (parents include sub-techniques) and tactic
	if l := items(a.m("GET", "/api/v1/catalog/risks?attack=T1110", nil, 200)); !slices.ContainsFunc(l, func(x map[string]any) bool { return x["id"] == "AUTH-R01" }) {
		t.Fatalf("attack filter: %d", len(l))
	}
	if l := items(a.m("GET", "/api/v1/catalog/risks?tactic=TA0006", nil, 200)); len(l) < 10 {
		t.Fatalf("tactic filter: %d", len(l))
	}
	d := a.m("GET", "/api/v1/catalog/risks/AUTH-R01", nil, 200)
	tech := anyList(d["techniques"])
	if !slices.Equal(strList(d["attack"]), []string{"T1110.003", "T1110.004"}) || len(tech) != 2 ||
		tech[0].(map[string]any)["name"] != "Brute Force: Password Spraying" || len(strList(d["tactics"])) != 1 {
		t.Fatalf("detail: %v %v", d["attack"], tech)
	}

	// an import copies the techniques to the project risk
	res := a.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R01"}, "include_requirements": false}, 201)
	rid := anyList(res["risks"])[0].(map[string]any)["id"].(string)
	k := a.m("GET", "/api/v1/risks/"+rid, nil, 200)
	if !slices.Equal(strList(k["attack"]), []string{"T1110.003", "T1110.004"}) {
		t.Fatalf("imported: %v", k["attack"])
	}
	// edited and validated
	k = a.m("PATCH", "/api/v1/risks/"+rid, map[string]any{"attack": []string{" t1078 ", "T1078", "aml.t0051"}}, 200)
	if !slices.Equal(strList(k["attack"]), []string{"T1078", "AML.T0051"}) {
		t.Fatalf("patched: %v", k["attack"])
	}
	a.m("PATCH", "/api/v1/risks/"+rid, map[string]any{"attack": []string{"T9999"}}, 400)
	a.m("POST", base+"/risks", map[string]any{"title": "Bad", "attack": []string{"T1562"}}, 400)
	n := a.m("POST", base+"/risks", map[string]any{"title": "Phishing", "attack": []string{"T1566.002"}}, 201)
	plain := a.m("POST", base+"/risks", map[string]any{"title": "Flood"}, 201)
	if len(anyList(plain["attack"])) != 0 || plain["attack"] == nil {
		t.Fatalf("no techniques: %v", plain["attack"])
	}
	// queried by technique, sub-technique, name and tactic
	for qs, want := range map[string]int{"attack:T1566": 1, "attack:T1078": 1, `attack:"spearphishing link"`: 1, "technique:AML.T0051": 1,
		"tactic:TA0001": 2, `tactic:"initial access"`: 2, "attack:T1110": 0} {
		if l := items(a.m("GET", base+"/risks?q="+url.QueryEscape(qs), nil, 200)); len(l) != want {
			t.Errorf("%s: %d, want %d", qs, len(l), want)
		}
	}
	_ = n
}
