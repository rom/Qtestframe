package tracerules

import (
	"slices"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

// builtinGraph: requirements
//
//	A approved must, conflicts_with B (approved); both in release R1 (active)
//	C approved, depends_on D (rejected) and on a deleted requirement X
//	E draft, refines F (obsolete); G obsolete refines F (inactive: ignored)
//	H rejected, adopted from a regulation, not justified; I rejected, justified
//	J "The system MUST log every failed sign-in attempt." K nearly the same
//
// tests T1 verifies D (rejected); T2 → T3 → T2 (cycle); T4 → T5 (failed);
// T6 → deleted test Y. Risk K1 high without requirements, K2 critical
// mitigated by A (not verified), K3 low (below the level).
func builtinGraph() *Graph {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	req := func(id, status, obl, text string, n int) (Item, ReqFacts) {
		return Item{Kind: model.KindRequirement, ID: id, Key: "R-" + id, Title: id},
			ReqFacts{Status: status, Obligation: obl, Text: text, Created: t0.Add(time.Duration(n) * time.Hour)}
	}
	g := &Graph{Schemas: map[string]*query.Schema{}, LastResults: map[string]string{"t5": model.ResultFailed, "t1": model.ResultPassed}}
	g.Reqs = map[string]ReqFacts{}
	for i, x := range [][4]string{
		{"a", "approved", "must", "alpha"}, {"b", "verified", "", "beta"}, {"c", "approved", "", "gamma"}, {"d", "rejected", "", "delta"},
		{"e", "draft", "", "epsilon"}, {"f", "obsolete", "", "phi"}, {"g", "obsolete", "", "gee"},
		{"h", "rejected", "must", "eta"}, {"i", "rejected", "must", "iota"},
		{"j", "approved", "must", "The system MUST log every failed sign-in attempt."},
		{"k", "draft", "must", "The system MUST log every failed sign-in attempts"},
	} {
		it, f := req(x[0], x[1], x[2], x[3], i)
		g.Requirements = append(g.Requirements, it)
		g.Reqs[x[0]] = f
	}
	g.TestStatus = map[string]string{}
	for _, id := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
		g.Tests = append(g.Tests, Item{Kind: model.KindTest, ID: id, Key: "T-" + id, Title: id})
		g.TestStatus[id] = "ready"
	}
	g.Relations = []model.Relation{
		{ID: "rel1", SourceID: "a", TargetID: "b", Type: "conflicts_with"},
		{ID: "rel2", SourceID: "c", TargetID: "d", Type: "depends_on"},
		{ID: "rel3", SourceID: "e", TargetID: "f", Type: "refines"},
		{ID: "rel4", SourceID: "g", TargetID: "f", Type: "derived_from"},
	}
	g.Links = []model.TraceLink{{ID: "l1", RequirementID: "d", TestCaseID: "t1"}}
	g.Dependencies = []model.TestDependency{
		{ID: "d1", TestID: "t2", DependsOnID: "t3"}, {ID: "d2", TestID: "t3", DependsOnID: "t2"}, {ID: "d3", TestID: "t4", DependsOnID: "t5"},
	}
	g.Releases = []Release{{ID: "r1", Name: "1.0", Status: "active", RequirementIDs: []string{"a", "b", "j"}}, {ID: "r2", Name: "0.9", Status: "cancelled", RequirementIDs: []string{"a", "b"}}}
	g.Risks = []Risk{
		{ID: "k1", Key: "K-1", Title: "No mitigation", Status: "identified", Likelihood: 4, Impact: 3},
		{ID: "k2", Key: "K-2", Title: "Untested", Status: "mitigating", Likelihood: 5, Impact: 4, RequirementIDs: []string{"a"}},
		{ID: "k3", Key: "K-3", Title: "Low", Status: "identified", Likelihood: 1, Impact: 1},
		{ID: "k4", Key: "K-4", Title: "Accepted", Status: "accepted", Likelihood: 5, Impact: 5},
	}
	g.Deleted = map[string]Item{"x": {Kind: model.KindRequirement, ID: "x", Key: "R-x", Title: "gone"}, "y": {Kind: model.KindTest, ID: "y", Key: "T-y", Title: "gone"}}
	g.DeletedRelations = []model.Relation{{ID: "relx", SourceID: "c", TargetID: "x", Type: "depends_on"}}
	g.DeletedDependencies = []model.TestDependency{{ID: "dy", TestID: "t6", DependsOnID: "y"}}
	g.FromRegulation = map[string]bool{"h": true, "i": true}
	g.Justified = map[string]bool{"i": true}
	return g
}

func rule(kind string) model.TraceRule {
	return model.TraceRule{ID: kind, Name: kind, Kind: kind, Scope: model.BuiltinScope(kind), Enabled: true, Severity: "warning", Min: 1}
}

func one(t *testing.T, g *Graph, r model.TraceRule) Result {
	t.Helper()
	res := Evaluate([]model.TraceRule{r}, g, nil)[0]
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	return res
}

func ids(res Result) []string {
	var out []string
	for _, v := range res.Violations {
		out = append(out, v.ID)
	}
	return out
}

func TestBuiltinChecks(t *testing.T) {
	g := builtinGraph()
	cases := map[string][]string{
		model.CheckConflictInRelease:    {"a", "b"},
		model.CheckUnmetDependency:      {"c"},
		model.CheckObsoleteParent:       {"e"},
		model.CheckVerifiesInactive:     {"t1"},
		model.CheckBrokenPrerequisite:   {"t2", "t3", "t4", "t6"},
		model.CheckUnevidencedMust:      {"a", "j"},
		model.CheckUnmitigatedRisk:      {"k1", "k2"},
		model.CheckUnjustifiedRejection: {"h"},
		model.CheckDuplicateSuspect:     {"k"},
	}
	for kind, want := range cases {
		res := one(t, g, rule(kind))
		if got := ids(res); !slices.Equal(got, want) {
			t.Errorf("%s: violations %v, want %v", kind, got, want)
		}
		if res.Passed || res.ViolationCount != len(want) {
			t.Errorf("%s: %+v", kind, res)
		}
	}

	// details: related items, reasons and fixes
	res := one(t, g, rule(model.CheckConflictInRelease))
	v := res.Violations[0]
	if v.Check != ReasonConflict || len(v.Related) != 2 || v.Related[0].ID != "b" || v.Related[0].Link != "rel1" || v.Related[1].Kind != "release" || v.Related[1].ID != "r1" ||
		!slices.Contains(v.Fixes, FixRemoveFromRelease) {
		t.Fatalf("conflict %+v", v)
	}
	res = one(t, g, rule(model.CheckUnmetDependency))
	v = res.Violations[0]
	if len(v.Related) != 2 || v.Related[0].Check != ReasonRejected || v.Related[1].Check != ReasonDeleted || v.Related[1].Key != "R-x" || v.Related[1].Status != ReasonDeleted ||
		!slices.Equal(v.Fixes, []string{FixRemoveRelation, FixDismiss}) {
		t.Fatalf("dependency %+v", v)
	}
	res = one(t, g, rule(model.CheckBrokenPrerequisite))
	reasons := map[string]string{}
	for _, v := range res.Violations {
		reasons[v.ID] = v.Check
	}
	if reasons["t2"] != ReasonCycle || reasons["t3"] != ReasonCycle || reasons["t4"] != ReasonFailing || reasons["t6"] != ReasonDeleted {
		t.Fatal(reasons)
	}
	res = one(t, g, rule(model.CheckUnmitigatedRisk))
	if res.Violations[0].Check != ReasonNoMitigation || res.Violations[1].Check != ReasonNotVerified || res.Violations[1].Related[0].ID != "a" || res.Matched != 2 {
		t.Fatalf("risks %+v", res)
	}
	res = one(t, g, rule(model.CheckDuplicateSuspect))
	if v := res.Violations[0]; v.Related[0].ID != "j" || v.Note == "" || !slices.Equal(v.Fixes, []string{FixMarkDuplicate, FixDismiss}) {
		t.Fatalf("duplicate %+v", v)
	}

	// a risk level option: critical only
	r := rule(model.CheckUnmitigatedRisk)
	r.Level = "critical"
	if got := ids(one(t, g, r)); !slices.Equal(got, []string{"k2"}) {
		t.Fatal(got)
	}
	// threshold 1: only identical texts
	r = rule(model.CheckDuplicateSuspect)
	r.Threshold = 1
	if got := ids(one(t, g, r)); len(got) != 0 {
		t.Fatal(got)
	}

	// fixing the data clears the violations
	g.Relations = append(g.Relations, model.Relation{ID: "dup", SourceID: "k", TargetID: "j", Type: "duplicates"})
	g.Dismissed = map[string]bool{DismissKey(model.CheckUnmetDependency, "c", "x"): true, DismissKey(model.CheckBrokenPrerequisite, "t6", "y"): true}
	g.LastResults["t1"] = model.ResultPassed
	g.Links = append(g.Links, model.TraceLink{ID: "l2", RequirementID: "a", TestCaseID: "t1"})
	g.linksByReq = nil
	if got := ids(one(t, g, rule(model.CheckDuplicateSuspect))); len(got) != 0 {
		t.Fatal("duplicate", got)
	}
	if v := one(t, g, rule(model.CheckUnmetDependency)).Violations[0]; len(v.Related) != 1 {
		t.Fatal("dismissed deleted dependency", v)
	}
	if got := ids(one(t, g, rule(model.CheckBrokenPrerequisite))); slices.Contains(got, "t6") {
		t.Fatal("dismissed prerequisite", got)
	}
	if got := ids(one(t, g, rule(model.CheckUnevidencedMust))); !slices.Equal(got, []string{"j"}) {
		t.Fatal("evidence", got)
	}
	if got := ids(one(t, g, rule(model.CheckUnmitigatedRisk))); !slices.Equal(got, []string{"k1"}) {
		t.Fatal("verified mitigation", got)
	}
}

func TestBuiltinHooksAndScope(t *testing.T) {
	g := builtinGraph()
	// waivers apply to every rule kind
	g.Hooks.Waived = func(_ *Graph, r model.TraceRule, v Violation) bool { return v.ID == "a" }
	res := one(t, g, rule(model.CheckConflictInRelease))
	if res.Waived != 1 || ids(res)[0] != "b" || res.ViolationCount != 1 {
		t.Fatalf("waived %+v", res)
	}
	// verification from another source
	g.Hooks.Verified = func(_ *Graph, id string) bool { return id == "j" || id == "a" }
	if got := ids(one(t, g, rule(model.CheckUnevidencedMust))); len(got) != 0 {
		t.Fatal(got)
	}
	g.Hooks.TestFailing = func(*Graph, string) bool { return false }
	if got := ids(one(t, g, rule(model.CheckBrokenPrerequisite))); slices.Contains(got, "t4") {
		t.Fatal(got)
	}
	g.Hooks.Justified = func(*Graph, string) bool { return true }
	if got := ids(one(t, g, rule(model.CheckUnjustifiedRejection))); len(got) != 0 {
		t.Fatal(got)
	}
	g.Hooks.Similarity = func(a, b string) float64 { return 0 }
	if got := ids(one(t, g, rule(model.CheckDuplicateSuspect))); len(got) != 0 {
		t.Fatal(got)
	}
	// scope: a release scope also reaches the risks of its requirements
	g = builtinGraph()
	in := map[string]bool{"requirement:a": true}
	scope := func(k, id string) bool { return in[k+":"+id] }
	res = Evaluate([]model.TraceRule{rule(model.CheckUnmitigatedRisk), rule(model.CheckConflictInRelease)}, g, scope)[0]
	if got := ids(res); !slices.Equal(got, []string{"k2"}) {
		t.Fatal(got)
	}
	// a selector narrows the checked items; an invalid one fails the rule
	g = builtinGraph()
	g.Record = func(kind, id string) query.Record { return query.MapRecord{"key": {"R-" + id}} }
	r := rule(model.CheckConflictInRelease)
	r.Selector = "key:R-b"
	if got := ids(one(t, g, r)); !slices.Equal(got, []string{"b"}) {
		t.Fatal(got)
	}
	r.Selector = "("
	if res := Evaluate([]model.TraceRule{r}, g, nil)[0]; res.Error == "" || res.Passed {
		t.Fatal(res)
	}
	// without facts nothing is found
	empty := &Graph{Requirements: []Item{{Kind: "requirement", ID: "a"}}, Tests: []Item{{Kind: "test", ID: "t"}}}
	for _, k := range model.TraceBuiltinKinds {
		if res := Evaluate([]model.TraceRule{rule(k)}, empty, nil)[0]; !res.Passed {
			t.Fatal(k, res)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if Similarity("Hello, World!", "hello world") != 1 || Similarity("", "x") != 0 {
		t.Fatal("normalised")
	}
	if s := Similarity("The system MUST log every failed sign-in", "The system MUST log every failed sign-up"); s < 0.8 || s >= 1 {
		t.Fatal(s)
	}
	if s := Similarity("Passwords are hashed", "Reports export to PDF"); s > 0.3 {
		t.Fatal(s)
	}
	if Normalize("  Ab-C  d ") != "ab c d" || Similarity("ab", "ab") != 1 {
		t.Fatal(Normalize("  Ab-C  d "))
	}
	if cycleMembers([]model.TestDependency{{TestID: "a", DependsOnID: "a"}})["a"] == 0 {
		t.Fatal("self loop")
	}
	if len(Builtins) != len(model.TraceBuiltinKinds) || BuiltinRule(Builtins[6], "p").Scope != model.KindRisk {
		t.Fatal("builtins")
	}
}
