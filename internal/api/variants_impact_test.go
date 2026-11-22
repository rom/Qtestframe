package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

// A changed applicability recommends a review of what the impact rules
// reach (default), marks trace links and relations suspect when the
// project says so, or does nothing.
func TestApplicabilityChangeImpact(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid
	e.member("lead@example.com", rbac.RoleTestLead)
	lead := e.login("lead@example.com")
	a.m("PATCH", "/api/v1/requirements/"+f.r2, map[string]any{"owner": "lead@example.com"}, 200)
	// R2 refines R1: a change of R1 makes R2 suspect along the relation
	a.m("POST", "/api/v1/requirements/"+f.r2+"/relations", map[string]any{"target_id": f.r1, "type": "refines"}, 201)
	set := a.m("GET", pp+"/correlation-settings", nil, 200)
	if set["applicability_impact"] != "review" {
		t.Fatal(set["applicability_impact"])
	}
	suspectLinks := func(id string) float64 {
		return a.m("GET", "/api/v1/requirements/"+id, nil, 200)["suspect_links"].(float64)
	}

	// the preview
	dry := a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability?dry_run=impact", map[string]any{"variant_ids": []string{f.eu}}, 200)
	imp := dry["impact"].(map[string]any)
	root := imp["roots"].([]any)[0].(map[string]any)
	if root["review_only"] != true || jsonText(root["fields"]) != `["applicability"]` || !strings.Contains(jsonText(imp["tests"]), f.k[f.t1]) {
		t.Fatal(jsonText(imp))
	}
	if ap := a.m("GET", "/api/v1/requirements/"+f.r1+"/applicability", nil, 200); ap["all"] != true {
		t.Fatal("the dry run changed the applicability", ap)
	}

	// default: review recommended; nothing is marked, the owner of T1 hears of it
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"variant_ids": []string{f.eu}}, 200)
	if n := suspectLinks(f.r1); n != 0 {
		t.Fatal("suspect links", n)
	}
	found := false
	for _, n := range items(lead.m("GET", "/api/v1/notifications", nil, 200)) {
		if n["event"] == "impact.review" && strings.Contains(n["title"].(string), f.k[f.r1]) {
			found = true
		}
	}
	if !found {
		t.Fatal("no review notification", jsonText(lead.m("GET", "/api/v1/notifications", nil, 200)))
	}

	// as a change of substance: the rules mark links and relations suspect
	a.m("PUT", pp+"/correlation-settings", map[string]any{"applicability_impact": "suspect"}, 200)
	a.m("PUT", pp+"/correlation-settings", map[string]any{"applicability_impact": "sometimes"}, 400)
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"variant_ids": []string{f.eu, f.us}}, 200)
	if n := suspectLinks(f.r1); n != 2 {
		t.Fatal("suspect links", n)
	}
	r2 := a.m("GET", "/api/v1/requirements/"+f.r2, nil, 200)
	if r2["suspect_relations"].(float64) != 1 {
		t.Fatal(r2["suspect_relations"])
	}
	audit := jsonText(a.m("GET", "/api/v1/audit?q=action:link.suspect", nil, 200))
	if !strings.Contains(audit, "applicability changed (EU → EU, US)") {
		t.Fatal(audit)
	}
	// the same applicability again changes nothing
	a.m("POST", "/api/v1/requirements/"+f.r1+"/links/confirm", nil, 200)
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"variant_ids": []string{f.us, f.eu}}, 200)
	if n := suspectLinks(f.r1); n != 0 {
		t.Fatal("unchanged applicability marked links", n)
	}
	// a test's applicability (bulk) marks its links
	a.m("PUT", pp+"/applicability", map[string]any{"item_type": "test", "item_ids": []string{f.t3}, "variant_ids": []string{f.de}}, 200)
	if n := suspectLinks(f.r2); n != 1 {
		t.Fatal("bulk: suspect links of R2", n)
	}
	// ?impact=skip
	a.m("POST", "/api/v1/requirements/"+f.r2+"/links/confirm", nil, 200)
	a.m("PUT", "/api/v1/tests/"+f.t3+"/applicability?impact=skip", map[string]any{"variant_ids": []string{f.eu}}, 200)
	if n := suspectLinks(f.r2); n != 0 {
		t.Fatal("skip marked links", n)
	}

	// off: nothing at all
	a.m("PUT", pp+"/correlation-settings", map[string]any{"applicability_impact": "off"}, 200)
	a.m("PUT", "/api/v1/tests/"+f.t3+"/applicability", map[string]any{"variant_ids": []string{f.us}}, 200)
	if n := suspectLinks(f.r2); n != 0 {
		t.Fatal("off marked links", n)
	}
}
