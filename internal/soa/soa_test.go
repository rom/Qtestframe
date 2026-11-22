package soa

import (
	"testing"
	"testing/fstest"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
)

func cat(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAnnexAControls(t *testing.T) {
	c := cat(t)
	cs, err := Controls(c, "iso-27001", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 93 {
		t.Fatalf("%d controls, want 93", len(cs))
	}
	groups := map[string]int{}
	for _, x := range cs {
		groups[x.Group]++
		if x.Title == "" || x.GroupTitle == "" {
			t.Errorf("%s: no title", x.ID)
		}
	}
	if groups["A.5"] != 37 || groups["A.6"] != 8 || groups["A.7"] != 14 || groups["A.8"] != 34 {
		t.Fatal(groups)
	}
	if cs[0].ID != "A.5.1" || cs[92].ID != "A.8.34" || cs[92].Title != "Protection of information systems during audit testing" {
		t.Fatal(cs[0], cs[92])
	}
	sv, _ := Controls(c, "iso-27001", "sv")
	if sv[0].Title != "Informationssäkerhetspolicyer" || sv[0].GroupTitle != "Organisatoriska säkerhetsåtgärder" {
		t.Fatal(sv[0])
	}
	if !HasControl(cs, "A.8.13") || HasControl(cs, "A.9.1") {
		t.Fatal("HasControl")
	}
	ls, _ := shipped()
	for ref, want := range map[string]string{"ISO/IEC 27001:2022 A.8.13 (information backup)": "A.8.13", "ISO/IEC 27001:2022 A.8.1": "A.8.1", "ISO/IEC 27001:2022 A.9.1": "", "ISO/IEC 27001:2022 Clause 6.1": ""} {
		if got := ls["iso-27001"].controlOf(ref); got != want {
			t.Errorf("%s: %q, want %q", ref, got, want)
		}
	}
	if _, err := Controls(c, "nope", "en"); err == nil {
		t.Fatal("unknown source")
	}
}

func TestDerivedControlsAndSources(t *testing.T) {
	c := cat(t)
	cs, err := Controls(c, "gdpr", "en", "GDPR Art. 99 (made up)")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 || !HasControl(cs, "Art. 99") || !HasControl(cs, "Art. 32") {
		t.Fatal(cs)
	}
	for i := 1; i < len(cs); i++ {
		if !naturalLess(cs[i-1].ID, cs[i].ID) {
			t.Fatalf("order %s %s", cs[i-1].ID, cs[i].ID)
		}
	}
	srcs, err := Sources(c, "sv")
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) < 2 || srcs[0].ID != "iso-27001" || !srcs[0].Shipped || srcs[0].Controls != 93 || srcs[0].Title != "ISO/IEC 27001:2022 bilaga A" {
		t.Fatal(srcs[0])
	}
}

func TestBuild(t *testing.T) {
	c := cat(t)
	// a catalogue requirement citing A.8.5
	var ref string
	for id, rq := range c.Requirements {
		for _, r := range rq.Refs {
			if r == "ISO/IEC 27001:2022 A.8.5" {
				ref = id
			}
		}
	}
	if ref == "" {
		t.Fatal("no catalogue requirement cites A.8.5")
	}
	yes, no := true, false
	in := Input{Catalog: c, SourceID: "iso-27001", Lang: "en",
		Requirements: []Requirement{
			{ID: "r1", Key: "P-REQ-1", Title: "MFA", Status: "approved", CatalogRef: ref, Tests: []Evidence{{ID: "t1", Key: "P-TC-1", Result: "passed"}}},
			{ID: "r2", Key: "P-REQ-2", Title: "Backups", Status: "approved", Source: "ISO/IEC 27001:2022 A.8.13", Tests: []Evidence{{ID: "t2", Key: "P-TC-2", Result: "failed"}}},
			{ID: "r3", Key: "P-REQ-3", Title: "Restore", Status: "approved", Source: "ISO/IEC 27001:2022 A.8.13", Waiver: "P-WVR-1"},
			{ID: "r4", Key: "P-REQ-4", Title: "Logs", Status: "draft", Source: "ISO/IEC 27001:2022 A.8.15"},
			{ID: "r5", Key: "P-REQ-5", Title: "Old", Status: "obsolete", Source: "ISO/IEC 27001:2022 A.8.16"},
		},
		Decisions: map[string]model.SoADecision{
			"A.8.5":  {ControlID: "A.8.5", Applicable: &yes, Justification: "Users sign in", Owner: "ciso"},
			"A.8.13": {ControlID: "A.8.13", Applicable: &yes},
			"A.8.15": {ControlID: "A.8.15", Applicable: &yes},
			"A.7.1":  {ControlID: "A.7.1", Applicable: &no, Justification: "No premises"},
			"A.6.1":  {ControlID: "A.6.1", Applicable: &yes},
		},
	}
	st, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]Row{}
	for _, r := range st.Rows {
		rows[r.ID] = r
	}
	if len(st.Rows) != 93 || st.Source.Title != "ISO/IEC 27001:2022 Annex A" || !st.Source.Shipped {
		t.Fatal(len(st.Rows), st.Source)
	}
	if r := rows["A.8.5"]; r.Status != Implemented || len(r.Requirements) != 1 || r.Owner != "ciso" || len(r.Evidence) != 1 || r.Catalogue == 0 {
		t.Fatalf("%+v", r)
	}
	if r := rows["A.8.13"]; r.Status != Partial || len(r.Requirements) != 2 || r.Requirements[1].Waiver != "P-WVR-1" {
		t.Fatalf("%+v", r)
	}
	if r := rows["A.8.15"]; r.Status != Planned {
		t.Fatalf("%+v", r)
	}
	if r := rows["A.7.1"]; r.Status != "" || *r.Applicable {
		t.Fatalf("%+v", r)
	}
	if r := rows["A.6.1"]; r.Status != NotImplemented {
		t.Fatalf("%+v", r)
	}
	if r := rows["A.8.16"]; len(r.Requirements) != 0 || r.Applicable != nil || r.Suggested {
		t.Fatalf("inactive requirement counted: %+v", r)
	}
	s := st.Summary
	if s.Controls != 93 || s.Applicable != 4 || s.Excluded != 1 || s.Undecided != 88 || s.Implemented != 1 || s.Partial != 1 || s.Planned != 1 || s.NotImplemented != 1 {
		t.Fatalf("%+v", s)
	}
	if _, err := Build(Input{Catalog: c, SourceID: "nope"}); err == nil {
		t.Fatal("unknown source")
	}
	if _, err := Build(Input{}); err == nil {
		t.Fatal("no catalogue")
	}
}

func TestStatusOf(t *testing.T) {
	for _, tc := range []struct {
		reqs []RowRequirement
		want string
	}{
		{nil, NotImplemented},
		{[]RowRequirement{{State: StatePassed}}, Implemented},
		{[]RowRequirement{{State: StatePassed}, {State: StateFailing}}, Partial},
		{[]RowRequirement{{State: StateUntested, Waiver: "W"}}, Partial},
		{[]RowRequirement{{State: StateBlocked}}, Planned},
	} {
		if got := StatusOf(tc.reqs); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.reqs, got, tc.want)
		}
	}
	if stateOf([]Evidence{{Result: "passed"}, {Result: "blocked"}}) != StateBlocked || stateOf([]Evidence{{Result: ""}}) != StateUntested {
		t.Fatal("stateOf")
	}
}

func TestLoadListsErrors(t *testing.T) {
	for name, body := range map[string]string{
		"bad yaml":  "source: [",
		"no source": "control: 'x'",
		"bad re":    "source: s\ncontrol: '('",
		"dup":       "source: s\ncontrol: 'A'\ngroups: [{id: g, controls: [{id: A, en: a}, {id: A, en: b}]}]",
	} {
		fs := fstest.MapFS{"d/x.yaml": {Data: []byte(body)}}
		if _, err := loadLists(fs, "d"); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	fs := fstest.MapFS{"d/a.yaml": {Data: []byte("source: s\ncontrol: 'A'")}, "d/b.yaml": {Data: []byte("source: s\ncontrol: 'A'")}}
	if _, err := loadLists(fs, "d"); err == nil {
		t.Error("duplicate source")
	}
}
