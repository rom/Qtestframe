package store_test

import (
	"errors"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestTraceRulesCRUD(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "TR", Name: "Rules"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	r := &model.TraceRule{ProjectID: p.ID, Name: " Must be tested ", Kind: model.TraceVerifiedBy, Min: 1, Enabled: true,
		TraceRuleOptions: model.TraceRuleOptions{NotSuspect: true, RelationType: "refines", Direction: "incoming", TargetType: "url"}}
	must(t, st.CreateTraceRule(ctx, ten.ID, r))
	if r.Name != "Must be tested" || r.Scope != model.KindRequirement || r.Severity != "error" || r.Position != 1 ||
		r.RelationType != "" || r.Direction != "" || r.TargetType != "" || !r.NotSuspect {
		t.Fatalf("%+v", r)
	}
	if err := st.CreateTraceRule(ctx, ten.ID, &model.TraceRule{ProjectID: p.ID, Name: "Must be tested", Kind: model.TraceVerifiedBy, Min: 1}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	r2 := &model.TraceRule{ProjectID: p.ID, Name: "Tests verify", Kind: model.TraceVerifies, Min: 1, Severity: "warning",
		TraceRuleOptions: model.TraceRuleOptions{Passed: true, NotSuspect: true}}
	must(t, st.CreateTraceRule(ctx, ten.ID, r2))
	if r2.Scope != model.KindTest || r2.Passed || r2.Position != 2 || r2.Enabled {
		t.Fatalf("%+v", r2)
	}
	got, err := st.GetTraceRule(ctx, ten.ID, r.ID)
	if err != nil || got.Name != r.Name || !got.Enabled || !got.NotSuspect || got.Min != 1 {
		t.Fatal(got, err)
	}
	got.Enabled, got.Kind, got.RelationType, got.Direction, got.Min = false, model.TraceRelation, "refines", "incoming", 2
	must(t, st.UpdateTraceRule(ctx, ten.ID, got))
	list, err := st.ListTraceRules(ctx, ten.ID, p.ID)
	if err != nil || len(list) != 2 || list[0].ID != r.ID || list[0].Enabled || list[0].NotSuspect || list[0].RelationType != "refines" || list[0].Min != 2 {
		t.Fatal(list, err)
	}
	got.Name = "Tests verify"
	if err := st.UpdateTraceRule(ctx, ten.ID, got); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if err := st.UpdateTraceRule(ctx, ten.ID, &model.TraceRule{ID: "nope", Name: "x", Kind: model.TraceVerifies, Min: 1}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateTraceRule(ctx, ten.ID, &model.TraceRule{ID: r.ID}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.GetTraceRule(ctx, "other", r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.CreateTraceRule(ctx, ten.ID, &model.TraceRule{ProjectID: "nope", Name: "x", Kind: model.TraceVerifies, Min: 1}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.DeleteTraceRule(ctx, ten.ID, r2.ID))
	if err := st.DeleteTraceRule(ctx, ten.ID, r2.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// rules go with their project
	must(t, st.DeleteProject(ctx, ten.ID, p.ID))
	if list, _ := st.ListTraceRules(ctx, ten.ID, p.ID); len(list) != 0 {
		t.Fatal(list)
	}
	if _, err := st.GetTraceRule(ctx, ten.ID, r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestTraceRuleLimit(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "TL", Name: "Limit"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	for i := 0; i < store.MaxTraceRules; i++ {
		must(t, st.CreateTraceRule(ctx, ten.ID, &model.TraceRule{ProjectID: p.ID, Name: "r" + string(rune('A'+i/26)) + string(rune('a'+i%26)), Kind: model.TraceVerifies, Min: 1}))
	}
	if err := st.CreateTraceRule(ctx, ten.ID, &model.TraceRule{ProjectID: p.ID, Name: "one more", Kind: model.TraceVerifies, Min: 1}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestValidateTraceRule(t *testing.T) {
	long := make([]byte, 4097)
	for i := range long {
		long[i] = 'a'
	}
	bad := []model.TraceRule{
		{Kind: model.TraceVerifiedBy, Min: 1},
		{Name: "x", Description: string(long), Kind: model.TraceVerifiedBy, Min: 1},
		{Name: "x", Severity: "fatal", Kind: model.TraceVerifiedBy, Min: 1},
		{Name: "x", Kind: "nope", Min: 1},
		{Name: "x", Kind: model.TraceVerifiedBy, Scope: model.KindTest, Min: 1},
		{Name: "x", Kind: model.TraceRelation, Scope: model.KindTest, Min: 1},
		{Name: "x", Kind: model.TraceVerifies, Scope: model.KindRequirement, Min: 1},
		{Name: "x", Kind: model.TraceItemLink, Scope: "run", Min: 1},
		{Name: "x", Kind: model.TraceVerifiedBy, Min: 0},
		{Name: "x", Kind: model.TraceVerifiedBy, Min: 1001},
		{Name: "x", Kind: model.TraceVerifiedBy, Min: 1, Selector: string(long)},
		{Name: "x", Kind: model.TraceRelation, Min: 1, TraceRuleOptions: model.TraceRuleOptions{Direction: "up"}},
		{Name: "x", Kind: model.TraceRelation, Min: 1, TraceRuleOptions: model.TraceRuleOptions{RelationType: "loves"}},
		{Name: "x", Kind: model.TraceItemLink, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetType: "planet"}},
		{Name: "x", Kind: model.TraceItemLink, Min: 1, TraceRuleOptions: model.TraceRuleOptions{TargetType: "url", TargetQuery: "a"}},
	}
	for i, r := range bad {
		if err := store.ValidateTraceRule(&r); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%d: %v", i, err)
		}
	}
	ok := model.TraceRule{Name: "x", Kind: model.TraceItemLink, Scope: model.KindTest, Min: 0,
		TraceRuleOptions: model.TraceRuleOptions{Every: true, TargetType: "requirement", TargetQuery: " type:security ", Passed: true}}
	if err := store.ValidateTraceRule(&ok); err != nil || ok.TargetQuery != "type:security" || ok.Passed {
		t.Fatal(ok, err)
	}
	ok = model.TraceRule{Name: "x", Kind: model.TraceItemLink, Min: 1}
	if err := store.ValidateTraceRule(&ok); err != nil || ok.Scope != model.KindRequirement {
		t.Fatal(ok, err)
	}
}

func TestExitCriterionTraceRules(t *testing.T) {
	crit := []model.ExitCriterion{{Type: "trace_rules", Level: "error"}}
	res := store.EvaluateExitCriteria(crit, store.ReleaseData{})
	if !res[0].Met || res[0].Available {
		t.Fatal(res)
	}
	var asked string
	d := store.ReleaseData{RuleViolations: func(level string) []store.ReleaseItem {
		asked = level
		return []store.ReleaseItem{{Kind: model.KindRequirement, ID: "r", Key: "R-1", Note: "Rule"}}
	}}
	res = store.EvaluateExitCriteria([]model.ExitCriterion{{Type: "trace_rules", Level: "warning"}}, d)
	if res[0].Met || !res[0].Available || res[0].Actual != 1 || len(res[0].Items) != 1 || asked != "warning" {
		t.Fatal(res)
	}
	rel := &model.Release{Name: "x", ExitCriteria: []model.ExitCriterion{{Type: "trace_rules"}}}
	if err := store.ValidateRelease(rel); err != nil || rel.ExitCriteria[0].Level != "error" {
		t.Fatal(rel, err)
	}
	rel.ExitCriteria[0].Level = "info"
	if err := store.ValidateRelease(rel); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}
