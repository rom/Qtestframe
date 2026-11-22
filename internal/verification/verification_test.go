package verification

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

func req(id, status string, crits ...string) model.Requirement {
	r := model.Requirement{ID: id, Key: strings.ToUpper(id), Title: id, Status: status}
	for i, c := range crits {
		r.Criteria = append(r.Criteria, model.EvaluationCriterion{ID: id + "-" + c, RequirementID: id, Description: c, Method: "test", Position: i})
	}
	return r
}

func link(id, r, t string, crits ...model.LinkCriterion) model.TraceLink {
	return model.TraceLink{ID: id, RequirementID: r, TestCaseID: t, TestCaseKey: strings.ToUpper(t), Criteria: crits}
}

func ptr(s string) *string { return &s }

func TestAggregate(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, model.VerNone},
		{[]string{"none", "none"}, model.VerNone},
		{[]string{"verified", "verified"}, model.VerVerified},
		{[]string{"verified", "none"}, model.VerPartial},
		{[]string{"partial", "pending"}, model.VerPartial},
		{[]string{"pending", "none"}, model.VerPending},
		{[]string{"verified", "failed"}, model.VerFailed},
	} {
		if got := Aggregate(c.in); got != c.want {
			t.Errorf("%v: %s, want %s", c.in, got, c.want)
		}
	}
}

func TestPlainLinksCountAsBefore(t *testing.T) {
	in := Input{
		Requirements: []model.Requirement{req("a", "implemented"), req("b", "implemented", "c1", "c2"), req("c", "implemented")},
		Links:        []model.TraceLink{link("l1", "a", "t1"), link("l2", "b", "t1"), link("l3", "b", "t2"), link("l4", "c", "t3")},
		Last:         map[string]string{"t1": "passed", "t2": "passed", "t3": "failed"},
	}
	res := Compute(in)
	if !res["a"].Verified || res["a"].Own != model.VerVerified {
		t.Fatalf("a: %+v", res["a"])
	}
	// non-strict: plain links cover both criteria
	if !res["b"].Verified || res["b"].CriteriaVerified != 2 || !res["b"].Criteria[0].Evidence[0].Plain {
		t.Fatalf("b: %+v", res["b"])
	}
	if res["c"].State != model.VerFailed || res["c"].Reasons[0].Code != "evidence_failed" {
		t.Fatalf("c: %+v", res["c"])
	}
	// strict: criteria need their own evidence
	in.Strict = true
	res = Compute(in)
	b := res["b"]
	if b.Verified || b.State != model.VerPending || b.CriteriaVerified != 0 || len(b.Reasons) != 2 || b.Reasons[0].Code != "criterion_missing" {
		t.Fatalf("strict b: %+v", b)
	}
	if !res["a"].Verified {
		t.Fatal("strict keeps requirements without criteria")
	}
	if Why(b) != "criterion 1 has no evidence; criterion 2 has no evidence" {
		t.Fatal(Why(b))
	}
}

func TestCriterionMappingStepsAndSuspect(t *testing.T) {
	now := time.Now()
	in := Input{
		Strict:       true,
		Requirements: []model.Requirement{req("a", "implemented", "c1", "c2", "c3")},
		Links: []model.TraceLink{
			link("l1", "a", "t1", model.LinkCriterion{CriterionID: "a-c1"}, model.LinkCriterion{CriterionID: "a-c2", StepID: "s2"}),
			link("l2", "a", "t2", model.LinkCriterion{CriterionID: "a-c3"}),
			link("l3", "a", "t3", model.LinkCriterion{CriterionID: "unknown"}),
		},
		Tests: map[string]TestInfo{"t1": {Key: "T1", StepPos: map[string]int{"s1": 1, "s2": 2}}},
		Last:  map[string]string{"t1": "failed", "t2": "passed", "t3": "passed"},
		Steps: map[string]map[string]string{"t1": {"s1": "failed", "s2": "passed"}},
	}
	in.Links[1].SuspectSince = &now
	v := Compute(in)["a"]
	c := v.Criteria
	if c[0].State != model.VerFailed || c[1].State != model.VerVerified || c[1].Evidence[0].StepPosition != 2 {
		t.Fatalf("criteria: %+v", c)
	}
	if c[2].State != model.VerPending || c[2].Evidence[0].Note != "suspect" {
		t.Fatalf("suspect: %+v", c[2])
	}
	// a mapping to a criterion of another requirement is a plain link
	if len(v.Evidence) != 1 || v.Evidence[0].ID != "t3" || v.State != model.VerFailed {
		t.Fatalf("plain: %+v", v)
	}
	// step not executed
	in.Steps = map[string]map[string]string{"t1": {"s1": "passed"}}
	in.Last["t1"] = "passed"
	in.Links[1].SuspectSince = nil
	v = Compute(in)["a"]
	if v.Criteria[1].State != model.VerPending || v.Criteria[1].Evidence[0].Note != "step_not_run" {
		t.Fatalf("step: %+v", v.Criteria[1])
	}
	in.Steps["t1"]["s2"] = "passed"
	if v = Compute(in)["a"]; !v.Verified {
		t.Fatalf("all passed: %+v", v)
	}
	// never run
	delete(in.Last, "t2")
	if v = Compute(in)["a"]; v.Criteria[2].Evidence[0].Note != "not_run" || v.State != model.VerPartial {
		t.Fatalf("not run: %+v", v)
	}
}

func TestRecords(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := Input{
		Strict:       true,
		Today:        "2026-06-01",
		Requirements: []model.Requirement{req("a", "implemented", "c1", "c2"), req("b", "implemented")},
		Records: []model.VerificationRecord{
			{ID: "r1", RequirementID: "a", CriterionID: ptr("a-c1"), Method: "inspection", Result: "fail", PerformedOn: "2026-02-01", CreatedAt: t0},
			{ID: "r2", RequirementID: "a", CriterionID: ptr("a-c1"), Method: "inspection", Result: "pass", PerformedOn: "2026-03-01", CreatedAt: t0},
			{ID: "r3", RequirementID: "a", CriterionID: ptr("a-c2"), Method: "analysis", Result: "conditional", PerformedOn: "2026-03-01", ValidUntil: "2026-12-31", CreatedAt: t0},
			{ID: "r4", RequirementID: "b", Method: "review", Result: "pass", PerformedOn: "2026-01-01", ValidUntil: "2026-05-01", CreatedAt: t0},
		},
	}
	res := Compute(in)
	a := res["a"]
	if !a.Verified || a.Criteria[0].Evidence[0].ID != "r2" || a.Criteria[0].Evidence[1].Note != "superseded" || a.Criteria[1].Evidence[0].Note != "conditional" {
		t.Fatalf("a: %+v", a)
	}
	if b := res["b"]; b.State != model.VerNone || b.Evidence[0].Note != "expired" || b.Reasons[0].Code != "criterion_missing" && b.Reasons[0].Code != "no_evidence" {
		t.Fatalf("b: %+v", b)
	}
	// invalidated by a change
	inv := t0
	in.Records[1].InvalidatedAt = &inv
	if a = Compute(in)["a"]; a.Criteria[0].State != model.VerFailed || a.Criteria[0].Evidence[0].Note != "invalidated" {
		t.Fatalf("invalidated: %+v", a.Criteria[0])
	}
	if Validity(&in.Records[3], "2026-05-01") != "valid" || Validity(&in.Records[3], "2026-05-02") != "expired" {
		t.Fatal("validity")
	}
}

func TestRollUp(t *testing.T) {
	in := Input{
		Requirements: []model.Requirement{req("p", "verified"), req("c1", "verified"), req("c2", "implemented"), req("c3", "obsolete"), req("g", "implemented")},
		Links:        []model.TraceLink{link("l1", "c1", "t1"), link("l2", "c2", "t2"), link("l3", "g", "t1")},
		Last:         map[string]string{"t1": "passed", "t2": "not_run"},
		Relations: []model.Relation{
			{SourceID: "c1", TargetID: "p", Type: "refines"}, {SourceID: "c2", TargetID: "p", Type: "refines"},
			{SourceID: "c3", TargetID: "p", Type: "refines"}, {SourceID: "g", TargetID: "c2", Type: "refines"},
			{SourceID: "p", TargetID: "g", Type: "relates_to"},
		},
	}
	res := Compute(in)
	p := res["p"]
	if p.Verified || p.State != model.VerPartial || p.ChildrenVerified != 1 || len(p.Children) != 3 {
		t.Fatalf("p: %+v", p)
	}
	codes := []string{}
	for _, r := range p.Inconsistencies {
		codes = append(codes, r.Code)
	}
	if strings.Join(codes, ",") != "status_inconsistent,child_status_unverified" {
		t.Fatalf("inconsistencies %v", codes)
	}
	if got := Ancestors(res, []string{"g"}); strings.Join(got, ",") != "c2,p" {
		t.Fatalf("ancestors %v", got)
	}
	in.Last["t2"] = "passed"
	if res = Compute(in); !res["p"].Verified || !res["c2"].Verified {
		t.Fatalf("all verified: %+v", res["p"])
	}
	// a parent with own failing evidence fails
	in.Links = append(in.Links, link("l4", "p", "t3"))
	in.Last["t3"] = "failed"
	if res = Compute(in); res["p"].State != model.VerFailed {
		t.Fatal("own failure")
	}
	// cycles do not loop
	in.Relations = append(in.Relations, model.Relation{SourceID: "p", TargetID: "g", Type: "refines"})
	res = Compute(in)
	if res["p"].State != model.VerFailed {
		t.Fatalf("cycle: %+v", res["p"])
	}
	if c := Counts(res, []string{"p", "c1"}); c[model.VerFailed] != 1 || c[model.VerVerified] != 1 {
		t.Fatalf("counts %v", c)
	}
}

func TestElementCoverage(t *testing.T) {
	root, sub := "e1", "e2"
	elements := []model.SystemElement{
		{ID: root, Name: "Vehicle", Path: "Vehicle", RequirementIDs: []string{"a"}, TestIDs: []string{"t1"}},
		{ID: sub, ParentID: &root, Name: "Power", Path: "Vehicle / Power", RequirementIDs: []string{"b", "c", "x"}, TestIDs: []string{"t2"}},
	}
	anc := map[string][]string{root: {root}, sub: {sub, root}}
	res := Result{"a": {State: model.VerVerified}, "b": {State: model.VerFailed}, "c": {State: model.VerNone}}
	st := map[string]string{"a": "verified", "b": "implemented", "c": "draft", "x": "obsolete"}
	out := ElementCoverage(elements, anc, st, res, map[string]string{"t1": "passed"})
	if out[0].Requirements != 3 || out[0].Verified != 1 || out[0].Failed != 1 || out[0].Covered != 2 || out[0].Tests != 2 || out[0].TestsPassed != 1 || out[0].Percent != 33.3 {
		t.Fatalf("root %+v", out[0])
	}
	if out[1].Requirements != 2 || out[1].Depth != 1 || out[1].Verified != 0 {
		t.Fatalf("sub %+v", out[1])
	}
}
