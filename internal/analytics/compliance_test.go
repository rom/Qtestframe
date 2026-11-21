package analytics

import (
	"slices"
	"testing"

	"github.com/rom/qtestframe/internal/catalog"
)

func TestStateOf(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, StateUntested},
		{[]string{"passed", "passed"}, StatePassed},
		{[]string{"passed", ""}, StateUntested},
		{[]string{"passed", "skipped"}, StateUntested},
		{[]string{"blocked", "passed"}, StateBlocked},
		{[]string{"blocked", "failed"}, StateFailing},
	} {
		if got := StateOf(c.in); got != c.want {
			t.Errorf("%v: %s, want %s", c.in, got, c.want)
		}
	}
}

func TestCompliance(t *testing.T) {
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cra := c.FindRequirements(catalog.Filter{Source: "cra"}, "en")
	if len(cra) < 4 {
		t.Fatalf("only %d CRA requirements", len(cra))
	}
	// four adopted: passed, failing, blocked (two copies, one passed), untested; one rejected copy (a gap)
	reqs := []ComplianceRequirement{
		{ID: "r1", Key: "P-1", Title: "a", Status: "approved", CatalogRef: cra[0].ID, Results: []string{"passed"}},
		{ID: "r2", Key: "P-2", Title: "b", Status: "draft", CatalogRef: cra[1].ID, Results: []string{"failed", "passed"}},
		{ID: "r3", Key: "P-3", Title: "c", Status: "draft", CatalogRef: cra[2].ID, Results: []string{"passed"}},
		{ID: "r3b", Key: "P-3b", Title: "c2", Status: "draft", CatalogRef: cra[2].ID, Results: []string{"blocked"}},
		{ID: "r4", Key: "P-4", Title: "d", Status: "draft", CatalogRef: cra[3].ID},
		{ID: "r5", Key: "P-5", Title: "e", Status: "rejected", CatalogRef: cra[4].ID, Results: []string{"passed"}},
		// own requirements naming the CRA and GDPR in their source field
		{ID: "o1", Key: "P-6", Title: "own", Status: "approved", Obligation: "shall", Source: "Regulation (EU) 2024/2847 (CRA) Art. 14; gdpr art. 32", Results: []string{"passed"}},
		{ID: "o2", Key: "P-7", Title: "other", Status: "approved", Source: "Internal policy"},
	}
	rep := Compliance(ComplianceInput{Catalog: c, Sources: []string{"cra", "nope", "cra"}, Requirements: reqs, Lang: "sv"})
	if !slices.Equal(rep.Unknown, []string{"nope"}) || len(rep.Sources) != 1 {
		t.Fatalf("unknown %v, %d sources", rep.Unknown, len(rep.Sources))
	}
	s := rep.Sources[0]
	if s.ID != "cra" || s.Kind != "law" || s.Catalogue != len(cra) || s.Adopted != 4 || s.Missing != len(cra)-4 || s.Own != 1 {
		t.Fatalf("counts %+v", s)
	}
	if s.Fulfilment != (Fulfilment{Passed: 2, Failing: 1, Blocked: 1, Untested: 1}) {
		t.Errorf("fulfilment %+v", s.Fulfilment)
	}
	if want := percent(2, len(cra)+1); s.Percent != want || s.AdoptionPercent != percent(4, len(cra)) {
		t.Errorf("percent %d (want %d), adoption %d", s.Percent, want, s.AdoptionPercent)
	}
	if s.MandatoryPercent < 0 || s.MandatoryPercent > 100 {
		t.Errorf("mandatory %d", s.MandatoryPercent)
	}
	var gap *ComplianceGap
	for i := range s.Gaps {
		if s.Gaps[i].CatalogID == cra[4].ID {
			gap = &s.Gaps[i]
		}
	}
	if gap == nil || len(gap.Inactive) != 1 || gap.Inactive[0].Key != "P-5" || len(gap.Clauses) == 0 || gap.Title == "" {
		t.Fatalf("gap for the rejected copy: %+v", gap)
	}
	if s.GapsTotal != len(s.Gaps) || s.ItemsTotal != 6 || s.Items[0].State != StateFailing {
		t.Errorf("items %d/%d, first %+v", len(s.Items), s.ItemsTotal, s.Items[0])
	}
	total := 0
	clauseNames := []string{}
	for _, cl := range s.Clauses {
		total += cl.Catalogue
		clauseNames = append(clauseNames, cl.Clause)
		if cl.Percent < 0 || cl.Percent > 100 {
			t.Errorf("clause %+v", cl)
		}
	}
	if total < s.Catalogue || !slices.Contains(clauseNames, "Art. 14") || !slices.Contains(clauseNames, "Annex I Part I") {
		t.Errorf("clauses %v (catalogue %d in clauses, %d)", clauseNames, total, s.Catalogue)
	}
	if len(s.Obligations) == 0 || s.Obligations[0].Obligation != "must" {
		t.Errorf("obligations %+v", s.Obligations)
	}
	if rep.Summary.Sources != 1 || rep.Summary.Adopted != 4 || rep.Summary.Own != 1 {
		t.Errorf("summary %+v", rep.Summary)
	}

	// without a selection: every source the project uses (CRA, GDPR via o1, and the other sources of the adopted items)
	auto := Compliance(ComplianceInput{Catalog: c, Requirements: reqs})
	var ids []string
	for _, x := range auto.Sources {
		ids = append(ids, x.ID)
	}
	if !slices.Contains(ids, "cra") || !slices.Contains(ids, "gdpr") {
		t.Errorf("auto sources %v", ids)
	}
	for _, x := range auto.Sources {
		if x.ID == "gdpr" && (x.Own != 1 || x.Fulfilment.Passed < 1) {
			t.Errorf("gdpr %+v", x)
		}
	}
	if len(Compliance(ComplianceInput{}).Sources) != 0 {
		t.Error("nil catalogue")
	}
}

func TestClauseLess(t *testing.T) {
	in := []string{"", "Art. 32", "Art. 5", "A.10", "A.8"}
	slices.SortFunc(in, func(a, b string) int {
		if clauseLess(a, b) {
			return -1
		}
		if clauseLess(b, a) {
			return 1
		}
		return 0
	})
	if !slices.Equal(in, []string{"A.8", "A.10", "Art. 5", "Art. 32", ""}) {
		t.Errorf("%v", in)
	}
	if obligationGroup("describe") != "other" || obligationGroup("optional") != "may" || priorityRank("x") != 4 {
		t.Error("groups")
	}
}
