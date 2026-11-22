package tracerules

import (
	"slices"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/similarity"
)

// simGraph: requirements P1 "at least 12 characters", P2 "at most 8"
// (conflict), L1 and L2 the same login requirement in other words, U
// unrelated, O an obsolete copy of L1 (ignored); tests T1 and T2 the same
// login test, T3 another, T4 a deprecated copy (ignored).
func simGraph() *Graph {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := &Graph{Schemas: map[string]*query.Schema{}}
	g.Reqs, g.TestStatus, g.Docs, g.TestCreated = map[string]ReqFacts{}, map[string]string{}, map[string]similarity.Doc{}, map[string]time.Time{}
	for i, x := range [][4]string{
		{"p1", "approved", "Password length", "Passwords MUST be at least 12 characters long."},
		{"p2", "draft", "Short passwords", "Passwords MUST be at most 8 characters."},
		{"l1", "approved", "User login", "The user MUST be able to sign in with their e-mail address and password."},
		{"l2", "draft", "Log in with email", "Users SHALL log in using their email address and a password."},
		{"u", "draft", "Dark mode", "The user interface SHOULD offer a dark colour scheme."},
		{"o", "obsolete", "User login", "The user MUST be able to sign in with their e-mail address and password."},
		{"n", "draft", "Nightly backup", "The database SHALL be backed up every night."},
	} {
		g.Requirements = append(g.Requirements, Item{Kind: model.KindRequirement, ID: x[0], Key: "R-" + x[0], Title: x[2]})
		g.Reqs[x[0]] = ReqFacts{Status: x[1], Created: t0.Add(time.Duration(i) * time.Hour)}
		g.Docs[x[0]] = similarity.Doc{Title: x[2], Description: x[3]}
	}
	for i, x := range [][3]string{
		{"t1", "ready", "Sign in with a valid password"}, {"t2", "ready", "Log in with a valid password"},
		{"t3", "ready", "Export the monthly report"}, {"t4", "deprecated", "Sign in with a valid password"},
		{"t5", "ready", "Switch to the dark theme"},
	} {
		g.Tests = append(g.Tests, Item{Kind: model.KindTest, ID: x[0], Key: "T-" + x[0], Title: x[2]})
		g.TestStatus[x[0]] = x[1]
		g.TestCreated[x[0]] = t0.Add(time.Duration(i) * time.Hour)
		g.Docs[x[0]] = similarity.Doc{Title: x[2], Details: []string{"Open the page", "Enter the e-mail address and the password"}}
	}
	return g
}

func TestSimilarityChecks(t *testing.T) {
	g := simGraph()
	if got := ids(one(t, g, rule(model.CheckDuplicateSuspect))); !slices.Equal(got, []string{"l2"}) {
		t.Fatal("duplicates", got)
	}
	res := one(t, g, rule(model.CheckPotentialConflict))
	if got := ids(res); !slices.Equal(got, []string{"p2"}) {
		t.Fatal("conflicts", got)
	}
	v := res.Violations[0]
	if v.Check != ReasonThreshold || v.Related[0].ID != "p1" || v.Related[0].Role != "conflict" || v.Related[0].Note == "" || !slices.Equal(v.Fixes, []string{FixMarkConflict, FixDismiss}) {
		t.Fatalf("%+v", v)
	}
	dt := one(t, g, rule(model.CheckTestDuplicateSuspect))
	if got := ids(dt); !slices.Equal(got, []string{"t2"}) || dt.Violations[0].Related[0].ID != "t1" {
		t.Fatal("tests", got)
	}
	m := g.SimilarityModel(model.KindTest)
	if len(m.Items) != 4 || m.Conflicts() != nil {
		t.Fatal(m.Items)
	}

	// marked, dismissed or above the threshold: gone
	g = simGraph()
	g.Relations = []model.Relation{{ID: "c", SourceID: "p2", TargetID: "p1", Type: "conflicts_with"}, {ID: "d", SourceID: "l2", TargetID: "l1", Type: "duplicates"}}
	target := "t1"
	g.ItemLinks = []model.ItemLink{{SourceType: model.KindTest, SourceID: "t2", TargetType: model.KindTest, TargetID: &target, Label: "Duplicates"}}
	for _, k := range []string{model.CheckDuplicateSuspect, model.CheckPotentialConflict, model.CheckTestDuplicateSuspect} {
		if got := ids(one(t, g, rule(k))); len(got) != 0 {
			t.Error(k, got)
		}
	}
	g = simGraph()
	g.Dismissed = map[string]bool{DismissKey(model.CheckPotentialConflict, "p1", "p2"): true, DismissKey(model.CheckTestDuplicateSuspect, "t2", "t1"): true}
	if got := ids(one(t, g, rule(model.CheckPotentialConflict))); len(got) != 0 {
		t.Error("dismissed conflict", got)
	}
	if got := ids(one(t, g, rule(model.CheckTestDuplicateSuspect))); len(got) != 0 {
		t.Error("dismissed test", got)
	}
	r := rule(model.CheckPotentialConflict)
	r.Threshold = 1
	if got := ids(one(t, g, r)); len(got) != 0 {
		t.Error("threshold", got)
	}

	// a replaced scorer (embeddings): every pair the same
	g = simGraph()
	g.Hooks.Scorer = func(_ *Graph, kind string, m *SimModel) similarity.Scorer {
		var ids []string
		var vecs [][]float32
		for _, it := range m.Items {
			ids = append(ids, it.ID)
			vecs = append(vecs, []float32{1, 0})
		}
		return similarity.NewDenseIndex(ids, vecs)
	}
	if got := ids(one(t, g, rule(model.CheckTestDuplicateSuspect))); len(got) != 3 {
		t.Error("scorer", got)
	}
	if ConflictReason(similarity.ConflictObligation) != ReasonOppositeObligation || ConflictReason(similarity.ConflictExclusive) != ReasonExclusive {
		t.Error("reasons")
	}
}
