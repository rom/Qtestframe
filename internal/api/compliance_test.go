package api

import (
	"slices"
	"strings"
	"testing"
)

// The reference sources: listing with counts, the ?source= filter and
// the compliance analysis of a project that imported CRA requirements.
func TestCatalogSourcesAndCompliance(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CMP")
	base := "/api/v1/projects/" + pid

	src := a.m("GET", "/api/v1/catalog/sources?lang=sv", nil, 200)
	if len(anyList(src["kinds"])) == 0 {
		t.Fatalf("kinds: %v", src)
	}
	var cra map[string]any
	for _, x := range anyList(src["sources"]) {
		if m := x.(map[string]any); m["id"] == "cra" {
			cra = m
		}
	}
	if cra == nil || cra["kind"] != "law" || cra["jurisdiction"] != "EU" || cra["requirements"].(float64) < 4 || cra["tests"].(float64) < 1 {
		t.Fatalf("cra: %v", cra)
	}
	all := a.m("GET", "/api/v1/catalog/sources?all=true&catalogue=software", nil, 200)
	if len(anyList(all["sources"])) < len(anyList(src["sources"])) {
		t.Fatal("all=true lists fewer sources")
	}
	a.m("GET", "/api/v1/catalog/sources?lang=xx", nil, 400)

	reqs := items(a.m("GET", "/api/v1/catalog/requirements?source=cra", nil, 200))
	if len(reqs) != int(cra["requirements"].(float64)) {
		t.Fatalf("%d CRA requirements, overview says %v", len(reqs), cra["requirements"])
	}
	for _, r := range reqs {
		if !slices.Contains(anyList(r["sources"]), any("cra")) {
			t.Fatalf("%v lacks cra", r)
		}
	}
	if l := items(a.m("GET", "/api/v1/catalog/tests?source=cra", nil, 200)); len(l) != int(cra["tests"].(float64)) {
		t.Fatalf("%d CRA tests, overview says %v", len(l), cra["tests"])
	}

	// import two CRA requirements with their tests
	ids := []string{reqs[0]["id"].(string), reqs[1]["id"].(string)}
	res := a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": ids}, 201)
	if len(anyList(res["requirements"])) != 2 || len(anyList(res["tests"])) == 0 {
		t.Fatalf("import: %v", res)
	}
	// pass every imported test
	var testIDs []string
	for _, x := range anyList(res["tests"]) {
		testIDs = append(testIDs, x.(map[string]any)["id"].(string))
	}
	run := a.m("POST", base+"/runs", map[string]any{"name": "R", "test_ids": testIDs}, 201)
	var results []map[string]any
	for _, x := range anyList(res["tests"]) {
		results = append(results, map[string]any{"test_key": x.(map[string]any)["key"], "status": "passed"})
	}
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/results", map[string]any{"results": results}, 200)
	// an own requirement that cites the CRA in its source field
	a.m("POST", base+"/requirements", map[string]any{"title": "Own", "description": "The product MUST log updates.", "source": "Regulation (EU) 2024/2847 (CRA) Art. 14"}, 201)

	rep := a.m("GET", base+"/analytics/compliance?source=cra,nope&lang=sv", nil, 200)
	srcs := anyList(rep["sources"])
	if len(srcs) != 1 || !slices.Equal(anyList(rep["unknown"]), []any{"nope"}) {
		t.Fatalf("report: %v", rep)
	}
	s := srcs[0].(map[string]any)
	f := s["fulfilment"].(map[string]any)
	if s["adopted"].(float64) != 2 || s["own"].(float64) != 1 || f["passed"].(float64) != 2 || f["untested"].(float64) != 1 ||
		s["missing"].(float64) != cra["requirements"].(float64)-2 || len(anyList(s["clauses"])) == 0 || len(anyList(s["gaps"])) == 0 {
		t.Fatalf("cra compliance: %v", s)
	}
	// without ?source= the sources the project uses
	auto := a.m("GET", base+"/analytics/compliance", nil, 200)
	found := false
	for _, x := range anyList(auto["sources"]) {
		found = found || x.(map[string]any)["id"] == "cra"
	}
	if !found {
		t.Fatalf("auto: %v", auto)
	}
	// the compliance report
	_, data := a.download(base+"/reports/generate", map[string]any{"template": "compliance", "format": "md", "params": map[string]string{"sources": "cra"}}, 200)
	if !strings.Contains(string(data), "Compliance by source") || !strings.Contains(string(data), "Cyber Resilience Act") {
		t.Fatalf("report: %s", data)
	}
	a.m("POST", base+"/reports/generate", map[string]any{"template": "compliance", "format": "md", "params": map[string]string{"sources": "nope"}}, 400)
	a.m("GET", base+"/analytics/compliance?lang=xx", nil, 400)
	a.m("GET", "/api/v1/projects/nope/analytics/compliance", nil, 404)
}
