package api

import (
	"testing"
)

// The Verification view and the analytics (requirement analytics and
// compliance) for one variant (?variant=): its requirements and tests,
// verified by the results that count for it.
func TestVariantFilterVerificationAndAnalytics(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid
	run := func(name, variant string, tests ...string) string {
		body := map[string]any{"name": name, "test_ids": tests}
		if variant != "" {
			body["variant_id"] = variant
		}
		return a.m("POST", pp+"/runs", body, 201)["id"].(string)
	}
	result := func(run, id, status string) {
		a.m("POST", "/api/v1/runs/"+run+"/results", map[string]any{"results": []map[string]any{{"test_key": f.k[id], "status": status}}}, 200)
	}
	// T1 passes everywhere, T2 fails in the US, T3 passes in the EU
	result(run("Smoke", "", f.t1), f.t1, "passed")
	result(run("US", "US", f.t2), f.t2, "failed")
	result(run("EU", "EU", f.t3), f.t3, "passed")

	states := func(q string) (map[string]string, map[string]any) {
		v := a.m("GET", pp+"/verification"+q, nil, 200)
		out := map[string]string{}
		for _, x := range v["requirements"].([]any) {
			m := x.(map[string]any)
			out[f.k[m["requirement_id"].(string)]] = m["state"].(string)
		}
		return out, v
	}
	all, _ := states("")
	if all[f.k[f.r1]] != "failed" || len(all) != 3 {
		t.Fatal(all)
	}
	// in the EU only T1 verifies R1 (T2 is a US test): verified; R3 is not an EU requirement
	eu, v := states("?variant=EU")
	if eu[f.k[f.r1]] != "verified" || eu[f.k[f.r2]] != "verified" || len(eu) != 2 || v["variant"] != f.eu {
		t.Fatal(eu, v["variant"])
	}
	if v["counts"].(map[string]any)["verified"] != float64(2) {
		t.Fatal(v["counts"])
	}
	us, _ := states("?variant=US")
	if us[f.k[f.r1]] != "failed" || len(us) != 2 {
		t.Fatal(us)
	}
	a.m("GET", pp+"/verification?variant=Mars", nil, 400)

	// analytics: the requirements of the variant and its coverage
	an := a.m("GET", pp+"/analytics", nil, 200)["summary"].(map[string]any)
	if an["requirements"] != float64(3) || an["tests"] != float64(3) {
		t.Fatal(an)
	}
	an = a.m("GET", pp+"/analytics?variant=EU", nil, 200)["summary"].(map[string]any)
	if an["requirements"] != float64(2) || an["tests"] != float64(2) || an["covered"] != float64(2) || an["coverage_percent"] != float64(100) {
		t.Fatal(an)
	}
	an = a.m("GET", pp+"/analytics?variant=US", nil, 200)["summary"].(map[string]any)
	if an["requirements"] != float64(2) || an["covered"] != float64(1) || an["coverage_percent"] != float64(50) {
		t.Fatal(an)
	}
	a.m("GET", pp+"/analytics?variant=Mars", nil, 400)
	// compliance takes the same filter
	a.m("GET", pp+"/analytics/compliance?variant=EU", nil, 200)
	a.m("GET", pp+"/analytics/compliance?variant=Mars", nil, 400)
}
