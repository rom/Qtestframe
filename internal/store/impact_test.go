package store_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// impactProject: requirements R0 … R(n-1), relations and tests as built by
// the test.
type impactProject struct {
	st   *store.Store
	ctx  context.Context
	ten  string
	pid  string
	reqs []string
	keys map[string]string // id → key
}

func newImpactProject(t *testing.T, n int) *impactProject {
	t.Helper()
	st, ten, ctx := setup(t)
	pr := &model.Project{Key: "IMP", Name: "Impact"}
	must(t, st.CreateProject(ctx, ten.ID, pr, false))
	x := &impactProject{st: st, ctx: ctx, ten: ten.ID, pid: pr.ID, keys: map[string]string{}}
	must(t, st.InTx(ctx, func(ctx context.Context) error {
		for i := range n {
			r := &model.Requirement{ProjectID: pr.ID, Title: fmt.Sprintf("R%d", i), Status: "draft", Owner: "owner@example.com"}
			if err := st.CreateRequirement(ctx, ten.ID, r); err != nil {
				return err
			}
			x.reqs = append(x.reqs, r.ID)
			x.keys[r.ID] = r.Key
		}
		return nil
	}))
	return x
}

func (x *impactProject) rel(t *testing.T, src int, typ string, dst int) {
	t.Helper()
	must(t, x.st.CreateRelation(x.ctx, x.ten, &model.Relation{ProjectID: x.pid, SourceID: x.reqs[src], TargetID: x.reqs[dst], Type: typ}))
}

func (x *impactProject) test(t *testing.T, title string, reqs ...int) string {
	t.Helper()
	tc := &model.TestCase{ProjectID: x.pid, Title: title, Status: "ready"}
	must(t, x.st.CreateTest(x.ctx, x.ten, tc))
	for _, r := range reqs {
		must(t, x.st.CreateLink(x.ctx, x.ten, &model.TraceLink{RequirementID: x.reqs[r], TestCaseID: tc.ID}))
	}
	return tc.ID
}

func (x *impactProject) analyze(t *testing.T, depth int, roots ...int) (*model.ChangeImpact, []store.ImpactMark) {
	t.Helper()
	idx, err := x.st.LoadImpactIndex(x.ctx, x.ten, x.pid)
	must(t, err)
	var rs []store.ImpactRoot
	for _, r := range roots {
		rs = append(rs, store.ImpactRoot{Kind: model.KindRequirement, ID: x.reqs[r]})
	}
	return idx.Analyze(rs, depth)
}

func ids(list []model.ImpactItem) map[string]model.ImpactItem {
	out := map[string]model.ImpactItem{}
	for _, it := range list {
		out[it.ID] = it
	}
	return out
}

func TestTransitiveImpact(t *testing.T) {
	x := newImpactProject(t, 6)
	// R1 refines R0, R2 refines R1, R3 refines R2 (a chain); R4 depends on
	// R2, R2 depends on R4 (a cycle); R5 relates to R0 (does not travel)
	x.rel(t, 1, "refines", 0)
	x.rel(t, 2, "refines", 1)
	x.rel(t, 3, "refines", 2)
	x.rel(t, 4, "depends_on", 2)
	x.rel(t, 2, "depends_on", 4)
	x.rel(t, 5, "relates_to", 0)
	t3 := x.test(t, "T3", 3)
	t0 := x.test(t, "T0", 0, 1)

	// depth 1: the one-level analysis
	imp, marks := x.analyze(t, 1, 0)
	if got := ids(imp.Requirements); len(got) != 1 || got[x.reqs[1]].Depth != 1 || got[x.reqs[1]].Relation != "refines" || got[x.reqs[1]].Direction != "incoming" {
		t.Fatal(imp.Requirements)
	}
	if got := ids(imp.Tests); len(got) != 1 || got[t0].Depth != 1 {
		t.Fatal(imp.Tests)
	}
	// both links of T0 (to R0 and to R1) and the relation R1 → R0 are marked
	var links, rels int
	for _, m := range marks {
		switch m.Edge {
		case model.EdgeVerifies:
			links++
		case "refines":
			rels++
			if m.End != "source" || m.ItemID != x.reqs[1] {
				t.Fatal(m)
			}
		}
	}
	if links != 2 || rels != 1 {
		t.Fatal(marks)
	}

	// the default depth 3: down the chain, through the cycle, once each
	imp, _ = x.analyze(t, 0, 0)
	got := ids(imp.Requirements)
	if imp.Depth != model.DefaultImpactDepth || len(got) != 4 || got[x.reqs[2]].Depth != 2 || got[x.reqs[3]].Depth != 3 || got[x.reqs[4]].Depth != 3 {
		t.Fatal(imp.Requirements)
	}
	if p := store.PathText(got[x.reqs[3]].Path); p != x.keys[x.reqs[0]]+" → refines → "+x.keys[x.reqs[1]]+" → refines → "+x.keys[x.reqs[2]]+" → refines → "+x.keys[x.reqs[3]] {
		t.Fatal(p)
	}
	// T3 verifies R3: evidence at depth 4, not counted as a step
	if tt := ids(imp.Tests)[t3]; tt.Depth != 4 || tt.Via != x.keys[x.reqs[3]] || tt.Mode != model.ModeSuspect {
		t.Fatal(imp.Tests)
	}
	if imp.ByDepth["3"] != 2 || imp.ByKind["requirements"] != 4 || imp.Total != imp.Suspect || imp.Review != 0 {
		t.Fatal(imp.ByDepth, imp.ByKind, imp.Total, imp.Suspect)
	}

	// review mode: depends_on only recommends a review, and so does what lies beyond
	set, err := x.st.CorrelationSettings(x.ctx, x.ten, x.pid)
	must(t, err)
	set.RelationImpact["depends_on"] = model.ImpactIncoming
	set.RelationModes["depends_on"] = model.ModeReview
	set.ImpactDepth = 5
	must(t, x.st.SaveCorrelationSettings(x.ctx, x.ten, x.pid, set))
	imp, marks = x.analyze(t, 0, 2)
	got = ids(imp.Requirements)
	if got[x.reqs[4]].Mode != model.ModeReview || got[x.reqs[3]].Mode != model.ModeSuspect || imp.Review != 1 {
		t.Fatal(imp.Requirements)
	}
	for _, m := range marks {
		if m.ItemID == x.reqs[4] {
			t.Fatal("a review is not marked", m)
		}
	}
	// a status change only recommends reviews
	idx, err := x.st.LoadImpactIndex(x.ctx, x.ten, x.pid)
	must(t, err)
	imp, marks = idx.Analyze([]store.ImpactRoot{{Kind: model.KindRequirement, ID: x.reqs[0], ReviewOnly: true}}, 0)
	if len(marks) != 0 || imp.Suspect != 0 || imp.Review == 0 || !imp.Roots[0].ReviewOnly {
		t.Fatal(imp, marks)
	}
	// several roots: a root is not affected by another
	imp, _ = x.analyze(t, 0, 0, 1)
	if _, ok := ids(imp.Requirements)[x.reqs[1]]; ok || len(imp.Roots) != 2 || imp.Key != "" {
		t.Fatal(imp.Requirements, imp.Roots)
	}
	// a test changes: the requirements it verifies, not what lies beyond
	ti, _ := idx.Analyze([]store.ImpactRoot{{Kind: model.KindTest, ID: t0}}, 0)
	if len(ti.Requirements) != 2 || ti.Requirements[0].Relation != model.EdgeVerifies {
		t.Fatal(ti.Requirements)
	}
	if idx.Of(model.KindRequirement, "missing") != nil || idx.Of(model.KindRequirement, x.reqs[0]) == nil {
		t.Fatal("Of")
	}
	// filtering leaves out what is reached through a left out item
	imp, _ = x.analyze(t, 0, 0)
	store.FilterImpact(imp, func(it model.ImpactItem) bool { return it.ID != x.reqs[2] })
	if _, ok := ids(imp.Requirements)[x.reqs[3]]; ok || len(imp.Requirements) != 1 || imp.ByKind["requirements"] != 1 {
		t.Fatal(imp.Requirements)
	}

	// settings validation
	for _, bad := range []model.CorrelationSettings{
		{ImpactDepth: 11},
		{ImpactDepth: -1},
		{RelationModes: map[string]string{"nope": "review"}},
		{RelationModes: map[string]string{"refines": "maybe"}},
		{ImpactRules: map[string]model.ImpactRule{"nope": {}}},
		{ImpactRules: map[string]model.ImpactRule{"risk": {Direction: "sideways"}}},
		{ImpactRules: map[string]model.ImpactRule{"risk": {Direction: "both", Mode: "maybe"}}},
		{ImpactRules: map[string]model.ImpactRule{"risk": {Direction: "both", Mode: "suspect"}}},
	} {
		if err := store.NormalizeCorrelationSettings(&bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	n := model.CorrelationSettings{ImpactRules: map[string]model.ImpactRule{"risk": {}}}
	must(t, store.NormalizeCorrelationSettings(&n))
	if n.ImpactDepth != model.DefaultImpactDepth || n.ImpactRules["risk"].Direction != model.ImpactOff || n.ImpactRules["defect"].Mode != model.ModeReview ||
		n.RelationModes["refines"] != model.ModeSuspect || n.Rule("risk").Direction != model.ImpactOff || n.Rule("verifies").Mode != model.ModeSuspect {
		t.Fatal(n)
	}
	if d := (&model.CorrelationSettings{ImpactDepth: 99}).Depth(); d != model.MaxImpactDepth {
		t.Fatal(d)
	}
}

// TestImpactPerformance: a generated project of 5000 requirements (a
// four-way refinement tree with dependencies across it) and 2500 tests;
// the analysis of depth 5 from the root stays well under 300 ms on SQLite.
func TestImpactPerformance(t *testing.T) {
	n := 5000
	pg := os.Getenv("QTF_TEST_POSTGRES_DSN") != ""
	if pg || testing.Short() {
		n = 400
	}
	var x *impactProject
	if pg || n < 5000 {
		x = newImpactProject(t, n)
		must(t, x.st.InTx(x.ctx, func(ctx context.Context) error {
			for i := 1; i < n; i++ {
				if err := x.st.CreateRelation(ctx, x.ten, &model.Relation{ProjectID: x.pid, SourceID: x.reqs[i], TargetID: x.reqs[(i-1)/4], Type: "refines"}); err != nil {
					return err
				}
			}
			for i := 0; i < n/2; i++ {
				tc := &model.TestCase{ProjectID: x.pid, Title: fmt.Sprintf("T%d", i), Status: "ready"}
				if err := x.st.CreateTest(ctx, x.ten, tc); err != nil {
					return err
				}
				for _, r := range []int{2 * i, 2*i + 1} {
					if err := x.st.CreateLink(ctx, x.ten, &model.TraceLink{RequirementID: x.reqs[r], TestCaseID: tc.ID}); err != nil {
						return err
					}
				}
			}
			return nil
		}))
	} else {
		x = generateImpactProject(t, n)
	}
	start := time.Now()
	idx, err := x.st.LoadImpactIndex(x.ctx, x.ten, x.pid)
	must(t, err)
	loaded := time.Since(start)
	imp, marks := idx.Analyze([]store.ImpactRoot{{Kind: model.KindRequirement, ID: x.reqs[0]}}, 5)
	total := time.Since(start)
	t.Logf("%d requirements: load %v, load and analyse %v; %d requirements, %d tests affected, %d marks", n, loaded, total, len(imp.Requirements), len(imp.Tests), len(marks))
	if n == 5000 && len(imp.Requirements) < 1364 {
		t.Fatal(len(imp.Requirements))
	}
	if !pg && total > 300*time.Millisecond {
		t.Fatalf("impact analysis of depth 5 took %v", total)
	}
}

// generateImpactProject inserts a large project directly (SQLite): n
// requirements in a four-way refinement tree with a dependency on every
// seventh, n/2 tests verifying two requirements each.
func generateImpactProject(t *testing.T, n int) *impactProject {
	t.Helper()
	st, ten, ctx := setup(t)
	pr := &model.Project{Key: "IMP", Name: "Impact"}
	must(t, st.CreateProject(ctx, ten.ID, pr, false))
	x := &impactProject{st: st, ctx: ctx, ten: ten.ID, pid: pr.ID, keys: map[string]string{}}
	tx, err := st.DB.BeginTx(ctx, nil)
	must(t, err)
	now := time.Now().UTC()
	insert := func(table, cols string, rows [][]any) {
		per := strings.Count(cols, ",") + 1
		for len(rows) > 0 {
			chunk := rows[:min(len(rows), 400)]
			rows = rows[len(chunk):]
			ph := "(" + strings.TrimSuffix(strings.Repeat("?,", per), ",") + ")"
			var args []any
			for _, r := range chunk {
				args = append(args, r...)
			}
			q := "INSERT INTO " + table + " (" + cols + ") VALUES " + strings.TrimSuffix(strings.Repeat(ph+",", len(chunk)), ",")
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	var reqs, tests, rels, links [][]any
	rid := func(i int) string { return fmt.Sprintf("r%05d", i) }
	for i := range n {
		id := rid(i)
		x.reqs = append(x.reqs, id)
		x.keys[id] = fmt.Sprintf("IMP-%d", i+1)
		reqs = append(reqs, []any{id, ten.ID, pr.ID, i + 1, x.keys[id], fmt.Sprintf("R%d", i), "draft", now, now})
		if i > 0 {
			rels = append(rels, []any{fmt.Sprintf("rel%05d", i), ten.ID, pr.ID, id, rid((i - 1) / 4), "refines", now})
		}
		if i%7 == 0 && i > 0 && (i*13)%n != i {
			rels = append(rels, []any{fmt.Sprintf("dep%05d", i), ten.ID, pr.ID, rid((i * 13) % n), id, "depends_on", now})
		}
	}
	for i := 0; i < n/2; i++ {
		id := fmt.Sprintf("t%05d", i)
		tests = append(tests, []any{id, ten.ID, pr.ID, i + 1, fmt.Sprintf("IMP-TC-%d", i+1), fmt.Sprintf("T%d", i), "ready", now, now})
		for _, r := range []int{2 * i, 2*i + 1} {
			links = append(links, []any{fmt.Sprintf("l%05d-%d", i, r), ten.ID, pr.ID, x.reqs[r], id, now})
		}
	}
	insert("requirements", "id, tenant_id, project_id, seq, key, title, status, created_at, updated_at", reqs)
	insert("test_cases", "id, tenant_id, project_id, seq, key, title, status, created_at, updated_at", tests)
	insert("requirement_relations", "id, tenant_id, project_id, source_id, target_id, type, created_at", rels)
	insert("trace_links", "id, tenant_id, project_id, requirement_id, test_case_id, created_at", links)
	must(t, tx.Commit())
	return x
}
