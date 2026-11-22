package store_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)


func TestVariantSetApplies(t *testing.T) {
	vs := store.NewVariantSet([]model.Variant{
		{ID: "us", Name: "US", Kind: "market", Attributes: map[string]string{"region": "US", "voltage": "110V"}, Position: 2},
		{ID: "eu", Name: "EU", Kind: "market", Attributes: map[string]string{"region": "EU", "voltage": "230V"}, Position: 1},
		{ID: "de", Name: "EU model DE", ParentID: ptr("eu"), Attributes: map[string]string{"Voltage": "240V", "lang": "de"}},
		{ID: "x", Name: "Orphan", ParentID: ptr("missing")},
	})
	var paths []string
	for _, v := range vs.List() {
		paths = append(paths, v.Path)
	}
	if strings.Join(paths, "|") != "Orphan|EU|EU / EU model DE|US" {
		t.Fatal(paths)
	}
	if vs.Find("eu model de").ID != "de" || vs.Find("EU / EU model DE").ID != "de" || vs.Find("de").ID != "de" || vs.Find("nope") != nil || vs.Find("") != nil {
		t.Fatal("find")
	}
	if a := vs.Attributes("de"); a["region"] != "EU" || a["Voltage"] != "240V" || len(a) != 3 {
		t.Fatal(a)
	}
	if !slices.Equal(vs.Lineage("de"), []string{"de", "eu"}) {
		t.Fatal(vs.Lineage("de"))
	}
	for _, c := range []struct {
		a    *model.Applicability
		want string
	}{
		{nil, "x,eu,de,us"},
		{&model.Applicability{}, "x,eu,de,us"},
		{&model.Applicability{VariantIDs: []string{"eu"}}, "eu,de"},
		{&model.Applicability{VariantIDs: []string{"de"}}, "de"},
		{&model.Applicability{Expression: "region:EU AND NOT voltage:240V"}, "eu"},
		{&model.Applicability{Expression: `variant:"EU"`}, "eu,de"},
		{&model.Applicability{Expression: "kind:market"}, "eu,de,us"},
		{&model.Applicability{Expression: "Orphan OR lang:de"}, "x,de"},
		{&model.Applicability{VariantIDs: []string{"us"}, Expression: "lang:de"}, "de,us"},
	} {
		var got []string
		for _, v := range vs.AppliesTo(c.a) {
			got = append(got, v.ID)
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%+v: %v, want %s", c.a, got, c.want)
		}
	}
	if _, err := store.ParseApplicability("(a"); err == nil {
		t.Fatal("bad expression accepted")
	}
	if _, err := store.ParseApplicability(strings.Repeat("a", 1001)); err == nil {
		t.Fatal("long expression accepted")
	}
}

func TestExtractItemKeys(t *testing.T) {
	text := "Fix DEMO-REQ-12 and req-3; see TC-007, OTHER-REQ-1, fix-REQ-4 and DEMO-TC-2 (DEMO-REQ-12 again) REQ-0"
	got := store.ExtractItemKeys(text, "DEMO")
	if strings.Join(got, ",") != "DEMO-REQ-12,DEMO-REQ-3,DEMO-TC-7,DEMO-REQ-4,DEMO-TC-2" {
		t.Fatal(got)
	}
	if len(store.ExtractItemKeys(strings.Repeat("REQ-1 REQ-2 ", 10)+strings.Repeat("x ", 3), "P")) != 2 {
		t.Fatal("duplicates")
	}
}

func TestValidateCodeAndBuild(t *testing.T) {
	for _, c := range []model.CodeChange{
		{Kind: "push", Ref: "abc"},
		{Kind: "commit", Provider: "svn", Ref: "abcd"},
		{Kind: "commit", Provider: "github", Ref: "not-a-sha"},
		{Kind: "commit"},
		{Kind: "merge_request", Ref: "1", Status: "draft"},
		{Kind: "commit", Ref: "abcd", URL: "javascript:alert(1)"},
	} {
		if err := store.ValidateCodeChange(&c); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
	c := model.CodeChange{Kind: "commit", Provider: "github", Ref: "ABCDEF1", Title: "Subject\n\nbody", Branch: "refs/heads/main", Status: "open"}
	if err := store.ValidateCodeChange(&c); err != nil || c.Ref != "abcdef1" || c.Title != "Subject" || c.Branch != "main" || c.Status != "" {
		t.Fatal(err, c)
	}
	m := model.CodeChange{Kind: "merge_request", URL: "https://example.org/mr/5"}
	if err := store.ValidateCodeChange(&m); err != nil || m.Provider != "manual" || m.Ref != m.URL || m.Status != "open" {
		t.Fatal(err, m)
	}
	b := model.Build{Number: " 7 ", Branch: "refs/heads/dev"}
	if err := store.ValidateBuild(&b); err != nil || b.ExternalID != "7" || b.Branch != "dev" || b.Label() != "7" {
		t.Fatal(err, b)
	}
	for _, b := range []model.Build{{}, {ExternalID: "1", CommitSHA: "xyz"}, {ExternalID: "1", PipelineURL: "file:///x"}} {
		if err := store.ValidateBuild(&b); err == nil {
			t.Errorf("%+v accepted", b)
		}
	}
}

// TestVariantDataInArchive: variants, applicability, builds and code links
// travel with a project archive, with their references rewritten.
func TestVariantDataInArchive(t *testing.T) {
	st, ten, ctx := setup(t)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })
	p := &model.Project{Key: "ARC", Name: "Archive"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	eu := &model.Variant{ProjectID: p.ID, Name: "EU", Attributes: map[string]string{"region": "EU"}}
	must(t, st.CreateVariant(ctx, ten.ID, eu))
	de := &model.Variant{ProjectID: p.ID, Name: "DE", ParentID: &eu.ID}
	must(t, st.CreateVariant(ctx, ten.ID, de))
	rq := &model.Requirement{ProjectID: p.ID, Title: "R"}
	must(t, st.CreateRequirement(ctx, ten.ID, rq))
	tc := &model.TestCase{ProjectID: p.ID, Title: "T"}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	if _, err := st.SetApplicability(ctx, ten.ID, p.ID, model.KindRequirement, []string{rq.ID}, &model.Applicability{VariantIDs: []string{"DE"}}, "me"); err != nil {
		t.Fatal(err)
	}
	b := &model.Build{ProjectID: p.ID, ExternalID: "9", CommitSHA: "abcdef1"}
	if _, err := st.UpsertBuild(ctx, ten.ID, b); err != nil {
		t.Fatal(err)
	}
	run := &model.TestRun{ProjectID: p.ID, Name: "Run", VariantID: &de.ID, BuildID: &b.ID}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{tc.ID}))
	ch := &model.CodeChange{ProjectID: p.ID, Kind: model.CodeCommit, Provider: model.ProviderGitHub, Ref: "abcdef1", Repository: "a/b"}
	if _, err := st.UpsertCodeChange(ctx, ten.ID, ch); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LinkCodeChange(ctx, ten.ID, ch.ID, []model.CodeChangeItem{{ItemType: model.KindTest, ItemID: tc.ID}}, model.CodeLinkAuto, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveCodeSettings(ctx, ten.ID, p.ID, "a-secret-that-stays-home", false, "me"); err != nil {
		t.Fatal(err)
	}

	data, m := saveProject(t, st, ten.ID, p.ID)
	for _, tbl := range []string{"variants", "item_applicability", "builds", "code_changes", "code_change_items"} {
		if m.Tables[tbl] == 0 {
			t.Errorf("archive has no %s", tbl)
		}
	}
	if strings.Contains(string(data), "a-secret-that-stays-home") {
		t.Fatal("the webhook secret is archived")
	}
	res, err := loadProject(st, ten.ID, data, store.ProjectImport{Key: "COPY"})
	must(t, err)
	np := res.Project.ID
	vs, err := st.ListVariants(ctx, ten.ID, np)
	must(t, err)
	if len(vs) != 2 || vs[1].ParentID == nil || *vs[1].ParentID != vs[0].ID || vs[0].ID == eu.ID {
		t.Fatal(vs)
	}
	applic, err := st.ListApplicability(ctx, ten.ID, np)
	must(t, err)
	reqs, _ := st.ListRequirements(ctx, ten.ID, np)
	if a := applic[reqs[0].ID]; a == nil || !slices.Equal(a.VariantIDs, []string{vs[1].ID}) {
		t.Fatal(applic)
	}
	runs, _ := st.ListRuns(ctx, ten.ID, np)
	if len(runs) != 1 || runs[0].VariantID == nil || *runs[0].VariantID != vs[1].ID || runs[0].VariantName != "DE" || runs[0].BuildLabel != "9" {
		t.Fatalf("%+v", runs)
	}
	tests, _ := st.ListTests(ctx, ten.ID, np)
	changes, err := st.ListCodeChanges(ctx, ten.ID, np, store.CodeChangeFilter{ItemType: model.KindTest, ItemID: tests[0].ID})
	must(t, err)
	if len(changes) != 1 || len(changes[0].Builds) != 1 || changes[0].Items[0].Key != "COPY-TC-1" {
		t.Fatalf("%+v", changes)
	}
	if cs, _ := st.CodeSettings(ctx, ten.ID, np); cs.HasSecret {
		t.Fatal("the copy has a secret")
	}

	// coverage per variant: the run for DE counts for DE, not for EU
	vc, err := st.LoadVariantCoverage(ctx, ten.ID, p.ID, map[string][]string{rq.ID: {tc.ID}})
	must(t, err)
	if !vc.AppliesTo(rq.ID, de.ID) || vc.AppliesTo(rq.ID, eu.ID) || vc.Requirement(rq.ID, eu.ID) != store.CoverageNotApplicable {
		t.Fatal("applicability")
	}
	if vc.Requirement(rq.ID, de.ID) != model.ResultNotRun || vc.TestResult(tc.ID, eu.ID) != model.ResultNotRun {
		t.Fatal(vc.Requirement(rq.ID, de.ID))
	}
	if vc.RunCounts(&de.ID, eu.ID) || !vc.RunCounts(&eu.ID, de.ID) || !vc.RunCounts(nil, eu.ID) {
		t.Fatal("run counts")
	}
	// deleting the variant DE: the run becomes neutral, the requirement applies to all
	must(t, st.DeleteVariant(ctx, ten.ID, de.ID))
	r, _ := st.GetRun(ctx, ten.ID, run.ID)
	if r.VariantID != nil {
		t.Fatal(r.VariantID)
	}
	if a, _ := st.GetApplicability(ctx, ten.ID, model.KindRequirement, rq.ID); !a.All() {
		t.Fatal(a)
	}
	// secrets are re-encrypted with the others
	if _, err := st.SecretStatus(ctx); err != nil {
		t.Fatal(err)
	}
}
