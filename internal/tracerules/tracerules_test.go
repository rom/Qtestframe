package tracerules

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

func ptr(s string) *string { return &s }

// graph: R1 must (2 tests: T1 passed, T2 suspect+failed), R2 must (none),
// R3 business refined by R4 (functional) and R5 (security), T3 linked to nothing.
func graph() *Graph {
	now := time.Now()
	reqs := map[string]query.MapRecord{
		"r1": {"key": {"R-1"}, "obligation": {"must"}, "type": {"functional"}},
		"r2": {"key": {"R-2"}, "obligation": {"must"}, "type": {"security"}},
		"r3": {"key": {"R-3"}, "obligation": {"should"}, "type": {"business"}},
		"r4": {"key": {"R-4"}, "obligation": {"may"}, "type": {"functional"}},
		"r5": {"key": {"R-5"}, "obligation": {"may"}, "type": {"security"}},
	}
	tests := map[string]query.MapRecord{
		"t1": {"key": {"T-1"}, "status": {"ready"}},
		"t2": {"key": {"T-2"}, "status": {"draft"}},
		"t3": {"key": {"T-3"}, "status": {"ready"}},
	}
	g := &Graph{
		LastResults: map[string]string{"t1": model.ResultPassed, "t2": model.ResultFailed},
		Links: []model.TraceLink{
			{RequirementID: "r1", TestCaseID: "t1"},
			{RequirementID: "r1", TestCaseID: "t2", SuspectSince: &now},
			{RequirementID: "r4", TestCaseID: "t1"},
		},
		Relations: []model.Relation{
			{SourceID: "r4", TargetID: "r3", Type: "refines"},
			{SourceID: "r5", TargetID: "r3", Type: "refines"},
			{SourceID: "r2", TargetID: "r1", Type: "depends_on"},
		},
		ItemLinks: []model.ItemLink{
			{SourceType: "requirement", SourceID: "r1", TargetType: "url", URL: "https://example.com"},
			{SourceType: "test", SourceID: "t3", TargetType: "requirement", TargetID: ptr("r2")},
		},
		Schemas: map[string]*query.Schema{},
		Record: func(kind, id string) query.Record {
			if kind == model.KindRequirement {
				if r, ok := reqs[id]; ok {
					return r
				}
			}
			if kind == model.KindTest {
				if t, ok := tests[id]; ok {
					return t
				}
			}
			return nil
		},
	}
	for _, id := range []string{"r1", "r2", "r3", "r4", "r5"} {
		g.Requirements = append(g.Requirements, Item{Kind: model.KindRequirement, ID: id, Key: reqs[id]["key"][0]})
	}
	for _, id := range []string{"t1", "t2", "t3"} {
		g.Tests = append(g.Tests, Item{Kind: model.KindTest, ID: id, Key: tests[id]["key"][0]})
	}
	return g
}

func keys(res Result) string {
	var out []string
	for _, v := range res.Violations {
		out = append(out, v.Key)
	}
	return strings.Join(out, ",")
}

func TestEvaluate(t *testing.T) {
	g := graph()
	req := model.KindRequirement
	cases := []struct {
		name    string
		rule    model.TraceRule
		matched int
		want    string
	}{
		{"must verified", model.TraceRule{Scope: req, Selector: "obligation:must", Kind: model.TraceVerifiedBy, Min: 1}, 2, "R-2"},
		{"all verified twice", model.TraceRule{Scope: req, Kind: model.TraceVerifiedBy, Min: 2}, 5, "R-2,R-3,R-4,R-5"},
		{"not suspect", model.TraceRule{Scope: req, Selector: "obligation:must", Kind: model.TraceVerifiedBy, Min: 2,
			TraceRuleOptions: model.TraceRuleOptions{NotSuspect: true}}, 2, "R-1,R-2"},
		{"passed", model.TraceRule{Scope: req, Selector: "key:R-1", Kind: model.TraceVerifiedBy, Min: 2,
			TraceRuleOptions: model.TraceRuleOptions{Passed: true}}, 1, "R-1"},
		{"target query tests", model.TraceRule{Scope: req, Selector: "key:R-1", Kind: model.TraceVerifiedBy, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{TargetQuery: "status:draft"}}, 1, ""},
		{"no suspect at all", model.TraceRule{Scope: req, Selector: "obligation:must", Kind: model.TraceVerifiedBy, Min: 0,
			TraceRuleOptions: model.TraceRuleOptions{NotSuspect: true, Every: true}}, 2, "R-1"},
		{"refined by children", model.TraceRule{Scope: req, Selector: "type:business", Kind: model.TraceRelation, Min: 2,
			TraceRuleOptions: model.TraceRuleOptions{RelationType: "refines", Direction: "incoming"}}, 1, ""},
		{"refined by functional children", model.TraceRule{Scope: req, Selector: "type:business", Kind: model.TraceRelation, Min: 2,
			TraceRuleOptions: model.TraceRuleOptions{RelationType: "refines", Direction: "incoming", TargetQuery: "type:functional"}}, 1, "R-3"},
		{"every child functional", model.TraceRule{Scope: req, Selector: "type:business", Kind: model.TraceRelation, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{RelationType: "refines", Direction: "incoming", TargetQuery: "type:functional", Every: true}}, 1, "R-3"},
		{"refines a parent", model.TraceRule{Scope: req, Selector: "obligation:may", Kind: model.TraceRelation, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{RelationType: "refines", Direction: "outgoing"}}, 2, ""},
		{"any relation", model.TraceRule{Scope: req, Kind: model.TraceRelation, Min: 1}, 5, ""},
		{"outgoing only", model.TraceRule{Scope: req, Kind: model.TraceRelation, Min: 1, TraceRuleOptions: model.TraceRuleOptions{Direction: "outgoing"}}, 5, "R-1,R-3"},
		{"item link any", model.TraceRule{Scope: req, Kind: model.TraceItemLink, Min: 1}, 5, "R-3,R-4,R-5"},
		{"item link url outgoing", model.TraceRule{Scope: req, Kind: model.TraceItemLink, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetType: "url", Direction: "outgoing"}}, 5, "R-2,R-3,R-4,R-5"},
		{"item link incoming test", model.TraceRule{Scope: req, Kind: model.TraceItemLink, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{TargetType: "test", Direction: "incoming", TargetQuery: "status:ready"}}, 5, "R-1,R-3,R-4,R-5"},
		{"test links", model.TraceRule{Scope: model.KindTest, Kind: model.TraceItemLink, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetType: "requirement", TargetQuery: "type:security"}}, 3, "T-1,T-2"},
		{"tests verify", model.TraceRule{Scope: model.KindTest, Kind: model.TraceVerifies, Min: 1}, 3, "T-3"},
		{"tests verify not suspect", model.TraceRule{Scope: model.KindTest, Kind: model.TraceVerifies, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{NotSuspect: true, TargetQuery: "obligation:must"}}, 3, "T-2,T-3"},
	}
	for _, c := range cases {
		res := Evaluate([]model.TraceRule{c.rule}, g, nil)[0]
		if res.Error != "" || res.Matched != c.matched || keys(res) != c.want || res.Passed != (c.want == "") || res.ViolationCount != len(res.Violations) {
			t.Errorf("%s: %+v", c.name, res)
		}
	}
	// found / total / required of a violation
	res := Evaluate([]model.TraceRule{{Scope: req, Selector: "key:R-1", Kind: model.TraceVerifiedBy, Min: 2, TraceRuleOptions: model.TraceRuleOptions{Passed: true}}}, g, nil)[0]
	if v := res.Violations[0]; v.Found != 1 || v.Total != 2 || v.Required != 2 || v.Kind != req || v.ID != "r1" {
		t.Fatal(v)
	}
}

func TestEvaluateScopeErrorsAndHelpers(t *testing.T) {
	g := graph()
	rules := []model.TraceRule{
		{ID: "a", Name: "A", Severity: "error", Scope: model.KindRequirement, Selector: "obligation:must", Kind: model.TraceVerifiedBy, Min: 1},
		{ID: "b", Name: "B", Severity: "warning", Scope: model.KindTest, Kind: model.TraceVerifies, Min: 1},
		{ID: "c", Name: "C", Scope: model.KindRequirement, Selector: "(", Kind: model.TraceVerifiedBy, Min: 1},
		{ID: "d", Name: "D", Scope: model.KindRequirement, Kind: model.TraceVerifiedBy, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetQuery: "/["}},
		{ID: "e", Name: "E", Scope: model.KindRequirement, Kind: model.TraceItemLink, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetType: "url", TargetQuery: "x"}},
	}
	all := Evaluate(rules, g, nil)
	if len(all) != 5 || all[2].Error == "" || all[2].Passed || all[3].Error == "" || all[4].Error == "" {
		t.Fatal(all)
	}
	// scope: only R-2 and T-3
	in := func(kind, id string) bool { return id == "r2" || id == "t3" }
	sc := Evaluate(rules[:2], g, in)
	if sc[0].Matched != 1 || keys(sc[0]) != "R-2" || sc[1].Matched != 1 || keys(sc[1]) != "T-3" {
		t.Fatal(sc)
	}
	if iv := ForItem(all, model.KindRequirement, "r2"); len(iv) != 1 || iv[0].Rule.ID != "a" || iv[0].Key != "R-2" {
		t.Fatal(iv)
	}
	if iv := ForItem(all, model.KindTest, "t1"); len(iv) != 0 {
		t.Fatal(iv)
	}
	if !Failing(all[0], "error") || Failing(all[1], "error") || !Failing(all[1], "warning") || Failing(Result{Passed: true}, "warning") {
		t.Fatal("failing")
	}
	// a nil record function and unknown items match nothing but empty queries
	g.Record = nil
	res := Evaluate([]model.TraceRule{{Scope: model.KindRequirement, Selector: "obligation:must", Kind: model.TraceVerifiedBy, Min: 1}}, g, nil)[0]
	if res.Matched != 0 || !res.Passed {
		t.Fatal(res)
	}
	if out := Evaluate(nil, g, nil); out == nil || len(out) != 0 {
		t.Fatal(out)
	}
}
