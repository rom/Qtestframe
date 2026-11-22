package api

import (
	"net/url"
	"slices"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func strList(v any) []string {
	var out []string
	for _, x := range anyList(v) {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// Catalogue risks: browse, the links to the mitigating requirements, and an
// import that brings the requirements along and links them to the risk.
func TestCatalogRisks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RSK")
	base := "/api/v1/projects/" + pid

	ov := a.m("GET", "/api/v1/catalog?lang=sv", nil, 200)
	if ov["risks"].(float64) < 250 || len(anyList(ov["risk_categories"])) == 0 || len(anyList(ov["risk_tags"])) == 0 {
		t.Fatalf("overview: %v %v", ov["risks"], ov["risk_categories"])
	}
	all := items(a.m("GET", "/api/v1/catalog/risks?lang=sv", nil, 200))
	if len(all) < 250 || all[0]["mitigation"] == "" || all[0]["category_name"] == "" || all[0]["level"] == "" {
		t.Fatalf("risks: %d %v", len(all), all[0])
	}
	if l := items(a.m("GET", "/api/v1/catalog/risks?category=safety&level=critical,high", nil, 200)); len(l) == 0 {
		t.Fatal("no high safety risks")
	} else {
		for _, x := range l {
			if x["category"] != "safety" || (x["level"] != "high" && x["level"] != "critical") {
				t.Fatalf("filter: %v", x)
			}
		}
	}
	if l := items(a.m("GET", "/api/v1/catalog/risks?q=AUTH-R01", nil, 200)); l[0]["id"] != "AUTH-R01" {
		t.Fatalf("search: %v", l[0])
	}
	a.m("GET", "/api/v1/catalog/risks/NOPE-R01", nil, 404)
	a.m("GET", "/api/v1/catalog/risks?lang=xx", nil, 400)

	d := a.m("GET", "/api/v1/catalog/risks/AUTH-R01?lang=sv&project_id="+pid, nil, 200)
	mit := anyList(d["mitigating_requirements"])
	if len(mit) != 4 || d["area_name"] != "Autentisering" || len(anyList(d["ref_targets"])) != len(anyList(d["refs"])) {
		t.Fatalf("detail: %v", d)
	}
	rq := a.m("GET", "/api/v1/catalog/requirements/AUTH-02?project_id="+pid, nil, 200)
	var mitigates []string
	for _, x := range anyList(rq["mitigates_risks"]) {
		mitigates = append(mitigates, x.(map[string]any)["id"].(string))
	}
	if !slices.Contains(mitigates, "AUTH-R01") || !slices.Contains(strList(rq["risks"]), "AUTH-R01") {
		t.Fatalf("mitigates: %v", rq["mitigates_risks"])
	}

	// import a risk: its four mitigating requirements and their tests come along
	res := a.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R01"}, "lang": "sv"}, 201)
	if len(anyList(res["risks"])) != 1 || len(anyList(res["requirements"])) != 4 || len(anyList(res["risk_links"])) != 4 || len(anyList(res["tests"])) == 0 {
		t.Fatalf("import: %v", res)
	}
	riskID := anyList(res["risks"])[0].(map[string]any)["id"].(string)
	k := a.m("GET", "/api/v1/risks/"+riskID, nil, 200)
	if k["catalog_ref"] != "AUTH-R01" || k["catalog_area"] != "authentication" || len(anyList(k["requirement_ids"])) != 4 ||
		k["category"] != "Informationssäkerhet" || k["likelihood"].(float64) != 5 || k["mitigation"] == "" || k["status"] != "identified" {
		t.Fatalf("risk: %v", k)
	}
	// a second import does not duplicate the risk or the requirements
	again := a.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R01"}}, 201)
	if len(anyList(again["risks"])) != 0 || len(anyList(again["requirements"])) != 0 || len(anyList(again["skipped"])) < 5 {
		t.Fatalf("re-import: %v", again)
	}
	pc := a.m("GET", base+"/catalog", nil, 200)
	if len(anyList(pc["risks"].(map[string]any)["AUTH-R01"])) != 1 {
		t.Fatalf("project catalogue: %v", pc["risks"])
	}
	if l := items(a.m("GET", "/api/v1/catalog/risks?not_in_project=true&project_id="+pid, nil, 200)); len(l) != len(all)-1 {
		t.Fatalf("not in project: %d of %d", len(l), len(all))
	}

	// a risk without its requirements, then a requirement that mitigates it:
	// the new requirement is linked to the risk already in the project
	res = a.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R02"}, "include_requirements": false, "risk_status": "mitigating"}, 201)
	if len(anyList(res["requirements"])) != 0 || len(anyList(res["risk_links"])) != 0 {
		t.Fatalf("risk only: %v", res)
	}
	r2 := anyList(res["risks"])[0].(map[string]any)["id"].(string)
	res = a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"AUTH-06"}, "include_tests": false}, 201)
	links := anyList(res["risk_links"])
	if len(links) != 1 || links[0].(map[string]any)["risk_id"] != r2 {
		t.Fatalf("link to existing risk: %v", res)
	}
	// requirements with the risks they mitigate
	res = a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"AUTH-05"}, "include_risks": true, "include_tests": false}, 201)
	var got []string
	for _, x := range anyList(res["risks"]) {
		got = append(got, x.(map[string]any)["catalog_ref"].(string))
	}
	if !slices.Contains(got, "AUTH-R04") {
		t.Fatalf("include_risks: %v", res)
	}
	a.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R03"}, "risk_status": "closed"}, 400)

	// the risk heat map filters by catalogue area
	if l := items(a.m("GET", base+"/risks?q="+url.QueryEscape("area:authentication"), nil, 200)); len(l) != 3 {
		t.Fatalf("area filter: %d", len(l))
	}
	if l := items(a.m("GET", base+"/risks?q="+url.QueryEscape("catalog:AUTH-R01"), nil, 200)); len(l) != 1 {
		t.Fatalf("catalog filter: %d", len(l))
	}

	// the requirement side: add and remove the risks it mitigates
	req06 := links[0].(map[string]any)["requirement_id"].(string)
	l := items(a.m("POST", "/api/v1/requirements/"+req06+"/risks", map[string]any{"add": []string{riskID}}, 200))
	if len(l) != 2 {
		t.Fatalf("add: %v", l)
	}
	l = items(a.m("POST", "/api/v1/requirements/"+req06+"/risks", map[string]any{"remove": []string{r2}}, 200))
	if len(l) != 1 || l[0]["id"] != riskID || l[0]["catalog_area"] != "authentication" {
		t.Fatalf("remove: %v", l)
	}
	other := a.project("OTH")
	ok := a.m("POST", "/api/v1/projects/"+other+"/risks", map[string]any{"title": "Elsewhere"}, 201)
	a.m("POST", "/api/v1/requirements/"+req06+"/risks", map[string]any{"add": []string{ok["id"].(string)}}, 400)
	a.m("POST", "/api/v1/requirements/"+req06+"/risks", map[string]any{"add": []string{"nope"}}, 404)
	a.m("POST", "/api/v1/requirements/nope/risks", map[string]any{"add": []string{riskID}}, 404)

	// a viewer browses risks but neither imports nor links them
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/catalog/risks", nil, 200)
	v.m("POST", base+"/catalog/import", map[string]any{"risk_ids": []string{"AUTH-R05"}}, 403)
	v.m("POST", "/api/v1/requirements/"+req06+"/risks", map[string]any{"remove": []string{riskID}}, 403)
}
