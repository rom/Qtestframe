package store_test

import (
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestStalePolicyCheck(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	set := model.DefaultCorrelationSettings()
	p := store.PolicyOf(&set, now)
	at := now.Add(-24 * time.Hour)
	if st := p.Check(at); st.Stale || st.RecordedAt == nil {
		t.Fatal(st)
	}
	if st := p.Check(now.Add(-91 * 24 * time.Hour)); !st.Stale || st.Reason != model.StaleAge {
		t.Fatal(st)
	}
	changes := []store.ItemChange{{Kind: model.KindRequirement, Key: "R-1", At: at.Add(time.Hour)}, {Kind: model.KindTest, Key: "T-1", At: at.Add(2 * time.Hour)}, {Kind: model.KindTest, Key: "T-0", At: at.Add(-time.Hour)}}
	if st := p.Check(at, changes...); !st.Stale || st.Reason != model.StaleTestChanged || st.Key != "T-1" {
		t.Fatal(st)
	}
	if st := p.Check(at, changes[0]); st.Reason != model.StaleRequirementChanged || st.Key != "R-1" {
		t.Fatal(st)
	}
	set.StaleAfterDays, set.StaleOnChange = 0, false
	p = store.PolicyOf(&set, now)
	if st := p.Check(now.Add(-1000*24*time.Hour), changes...); st.Stale {
		t.Fatal(st)
	}
	// a nil index finds nothing stale and passes results through
	var x *store.StaleIndex
	if x.Test("t").Stale || x.Unit("t", "").Stale || x.Check(at, "t").Stale || len(x.StaleTests()) != 0 || x.StaleCount([]string{"t"}) != 0 {
		t.Fatal("nil index")
	}
	if got := x.FilterLast(map[string]string{"t": "passed"}); got["t"] != "passed" {
		t.Fatal(got)
	}
	if got := x.FilterConfig(store.ConfigResults{"t": {"": "passed"}}); got["t"][""] != "passed" {
		t.Fatal(got)
	}
	if x.Settings().StaleAfterDays != model.DefaultStaleAfterDays {
		t.Fatal(x.Settings())
	}
}

func TestImpactedRelationsAndSettings(t *testing.T) {
	set := model.DefaultCorrelationSettings()
	rels := []model.Relation{
		{ID: "1", SourceID: "child", TargetID: "x", Type: "refines"},
		{ID: "2", SourceID: "x", TargetID: "parent", Type: "refines"},
		{ID: "3", SourceID: "y", TargetID: "x", Type: "relates_to"},
		{ID: "4", SourceID: "x", TargetID: "z", Type: "provides"},
	}
	got := store.ImpactedRelations(rels, "x", &set)
	if len(got) != 1 || got[0].OtherID != "child" || got[0].End != "source" || got[0].Direction != "incoming" {
		t.Fatal(got)
	}
	set.RelationImpact["refines"] = model.ImpactBoth
	set.RelationImpact["provides"] = model.ImpactOutgoing
	got = store.ImpactedRelations(rels, "x", &set)
	if len(got) != 3 || got[1].OtherID != "parent" || got[1].End != "target" || got[2].OtherID != "z" {
		t.Fatal(got)
	}
	if set.ImpactDirection("duplicates") != model.ImpactOff {
		t.Fatal("default off")
	}
	if set.DefectBlockLevel("critical") != model.DefectBlocks || set.DefectBlockLevel("minor") != model.DefectWarns || set.DefectBlockLevel("trivial") != "" {
		t.Fatal("levels")
	}
	n := model.CorrelationSettings{RelationImpact: map[string]string{"refines": ""}, BlockingSeverities: []string{"major", "major"}}
	if err := store.NormalizeCorrelationSettings(&n); err != nil {
		t.Fatal(err)
	}
	if len(n.RelationImpact) != len(model.RelationTypes) || n.RelationImpact["refines"] != model.ImpactOff || len(n.BlockingSeverities) != 1 || n.RegressionAction != model.RegressionReopen {
		t.Fatal(n)
	}
	if b, w := store.BlockCounts([]model.DefectBlock{{Level: model.DefectBlocks}, {Level: model.DefectWarns}, {Level: model.DefectWarns}}); b != 1 || w != 2 {
		t.Fatal(b, w)
	}
}
