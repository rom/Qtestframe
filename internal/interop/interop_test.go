package interop

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

func reqItem(id, key, title string, version int) *Item {
	return RequirementItem(&model.Requirement{ID: id, Key: key, Title: title, Version: version, Status: "draft", Priority: "low"}, "")
}

func TestFieldParsing(t *testing.T) {
	fs := BuiltinFields(KindRequirement)
	get := func(k string) *Field { return FieldByKey(fs, k) }
	if v, err := parseValue(get("obligation"), "SKALL"); err != nil || v != "shall" {
		t.Errorf("obligation: %q %v", v, err)
	}
	if _, err := parseValue(get("priority"), "whenever"); err == nil {
		t.Error("bad priority accepted")
	}
	if v, _ := parseValue(get("tags"), "a; b |c"); v != "a, b, c" {
		t.Errorf("tags: %q", v)
	}
	if v, _ := parseValue(get("folder"), `A\B > C`); v != "A/B/C" {
		t.Errorf("folder: %q", v)
	}
	if v, _ := parseValue(get("criteria"), "- inspection: looks right (threshold: 2)\n- works"); v != "inspection: looks right (threshold: 2)\nworks" {
		t.Errorf("criteria: %q", v)
	}
	tf := BuiltinFields(KindTest)
	if v, err := parseValue(FieldByKey(tf, "estimated_minutes"), "15.0"); err != nil || v != "15" {
		t.Errorf("minutes: %q %v", v, err)
	}
	if _, err := parseValue(FieldByKey(tf, "estimated_minutes"), "soon"); err == nil {
		t.Error("bad minutes accepted")
	}
	if v, err := parseValue(FieldByKey(tf, "execution_type"), "Automated"); err != nil || v != "automated" {
		t.Errorf("exec: %q %v", v, err)
	}
	if !equalValues(get("tags"), "b, a", "a,b") || equalValues(get("tags"), "a", "a, b") || !equalValues(get("folder"), "a/B", "A / b") {
		t.Error("equalValues")
	}
	if FieldByHeader(fs, "Prioritet").Key != "priority" || FieldByHeader(fs, "rel:refines").Key != "rel:refines" || FieldByHeader(fs, "nope") != nil {
		t.Error("FieldByHeader")
	}
	b := map[string]any{}
	it := reqItem("1", "K-1", "T", 1)
	it.Req.Criteria = []model.EvaluationCriterion{{ID: "c1", Description: "old"}}
	if err := putCriteria(b, "test: new\nsecond", it); err != nil {
		t.Fatal(err)
	}
	if cs := b["criteria"].([]model.EvaluationCriterion); len(cs) != 2 || cs[0].ID != "c1" || cs[0].Method != "test" || cs[1].ID != "" {
		t.Errorf("criteria body: %+v", cs)
	}
}

func TestExtensions(t *testing.T) {
	Register(Extension{Name: "test", Fields: func(_ context.Context, _, _, kind string) ([]Field, error) {
		if kind != KindRequirement {
			return nil, nil
		}
		return []Field{{Key: "cf:risk", Label: "Risk class", Type: Text, Get: func(it *Item) string { return it.Extra["risk"] },
			Put: func(b map[string]any, v string, _ *Item) error {
				b["custom_fields"] = map[string]any{"risk": v}
				return nil
			}}}, nil
	}, Load: func(_ context.Context, _, _, _ string, items []*Item) error {
		for _, it := range items {
			it.Extra["risk"] = "B"
		}
		return nil
	}})
	defer func() { extensions = nil }()
	fs, err := FieldsFor(context.Background(), "t", "p", KindRequirement)
	if err != nil || FieldByKey(fs, "cf:risk") == nil {
		t.Fatalf("extension fields: %v", err)
	}
	it := reqItem("1", "K-1", "T", 1)
	if err := LoadExtras(context.Background(), "t", "p", KindRequirement, []*Item{it}); err != nil || it.Extra["risk"] != "B" {
		t.Fatal("extras")
	}
	d := Compare(KindRequirement, fs, []Incoming{{ID: "1", Cells: map[string]Cell{"cf:risk": {Raw: "A"}}}}, ItemsByID([]*Item{it}), nil)
	if d.Rows[0].Status != StatusChanged {
		t.Fatalf("diff: %+v", d.Rows[0])
	}
	body, _ := Body(fs, &d.Rows[0], it)
	if body["custom_fields"].(map[string]any)["risk"] != "A" {
		t.Errorf("body: %v", body)
	}
	Register(Extension{Name: "broken", Fields: func(context.Context, string, string, string) ([]Field, error) { return nil, errors.New("x") },
		Load: func(context.Context, string, string, string, []*Item) error { return errors.New("y") }})
	if _, err := FieldsFor(context.Background(), "t", "p", KindTest); err == nil {
		t.Error("extension error lost")
	}
	if err := LoadExtras(context.Background(), "t", "p", KindTest, nil); err == nil {
		t.Error("load error lost")
	}
}

func TestCompare(t *testing.T) {
	fs := BuiltinFields(KindRequirement)
	cur := ItemsByID([]*Item{reqItem("a", "P-REQ-1", "One", 3), reqItem("b", "P-REQ-2", "Two", 1), reqItem("c", "P-REQ-3", "Three", 5)})
	lookup := func(kind, key string) (string, bool) {
		switch strings.ToUpper(key) {
		case "P-REQ-1":
			return "a", true
		case "P-REQ-2":
			return "b", true
		}
		return "", false
	}
	rows := []Incoming{
		{Row: 2, ID: "a", BaseVersion: 3, Cells: map[string]Cell{"title": {Raw: "One"}, "priority": {Raw: "Low"}}},             // unchanged
		{Row: 3, ID: "a", BaseVersion: 3, Cells: map[string]Cell{"title": {Raw: "One!"}}},                                      // duplicate id
		{Row: 4, ID: "b", BaseVersion: 1, Cells: map[string]Cell{"rel:refines": {Raw: "P-REQ-1, P-REQ-9"}}},                    // unknown key
		{Row: 5, ID: "b", BaseVersion: 1, Cells: map[string]Cell{"rel:refines": {Raw: "P-REQ-2"}}},                             // self + dup
		{Row: 6, Cells: map[string]Cell{"description": {Raw: "x"}}},                                                            // new without title column
		{Row: 7, Cells: map[string]Cell{"title": {Raw: " "}}},                                                                  // empty title
		{Row: 8, ID: "zzz", Cells: map[string]Cell{}},                                                                          // unknown id
		{Row: 9, Cells: map[string]Cell{"title": {Raw: "New"}, "verified_by": {Raw: "X"}, "rel:depends_on": {Raw: "p-req-2"}}}, // new
		{Row: 10, ID: "c", BaseVersion: 2, Cells: map[string]Cell{"status": {Raw: "approved"}}},                                // conflict
	}
	d := Compare(KindRequirement, fs, rows, cur, lookup)
	want := []string{StatusUnchanged, StatusError, StatusError, StatusError, StatusError, StatusError, StatusError, StatusNew, StatusConflict}
	for i, w := range want {
		if d.Rows[i].Status != w {
			t.Errorf("row %d: %s, want %s: %+v", rows[i].Row, d.Rows[i].Status, w, d.Rows[i])
		}
	}
	if d.Summary.New != 1 || d.Summary.Errors != 6 || d.Summary.Conflicts != 1 || d.Summary.Unchanged != 1 {
		t.Errorf("summary: %+v", d.Summary)
	}
	if d.Rows[7].Values["rel:depends_on"] != "p-req-2" || d.Rows[7].Title != "New" {
		t.Errorf("new row: %+v", d.Rows[7])
	}
	if c := d.Rows[8].Changes; len(c) != 1 || c[0].Old != "draft" || c[0].New != "approved" {
		t.Errorf("conflict changes: %+v", c)
	}
}

func TestWorkbookRoundTrip(t *testing.T) {
	fs := BuiltinFields(KindTest)
	call := "t2"
	t1 := TestItem(&model.TestCase{ID: "t1", Key: "P-TC-1", Title: "=formula?", Version: 2, ExecutionType: "manual", Status: "draft",
		Steps: []model.TestStep{{ID: "s1", Action: "Open", Expected: "Shown"}, {ID: "s2", CallTestID: &call, CallTestKey: "P-TC-2"}}}, "A/B")
	t1.Links = []string{"P-REQ-2", "P-REQ-1"}
	t2 := TestItem(&model.TestCase{ID: "t2", Key: "P-TC-2", Title: "Helper", Version: 1, ExecutionType: "manual", Status: "ready"}, "")
	var buf bytes.Buffer
	if err := WriteWorkbook(&buf, Workbook{Kind: KindTest, ProjectID: "p", ProjectKey: "P", ProjectName: "Proj", Fields: fs, Items: []*Item{t1, t2}}); err != nil {
		t.Fatal(err)
	}
	sh, err := ReadWorkbook(buf.Bytes(), "", func(string) []Field { return fs })
	if err != nil {
		t.Fatal(err)
	}
	if sh.Kind != KindTest || sh.ProjectID != "p" || len(sh.Rows) != 2 || !sh.HasSteps || len(sh.Steps["t1"]) != 2 {
		t.Fatalf("sheet: %+v", sh)
	}
	lookup := func(kind, key string) (string, bool) {
		m := map[string]string{"P-TC-2": "t2", "P-REQ-1": "r1", "P-REQ-2": "r2"}
		id, ok := m[strings.ToUpper(key)]
		return id, ok
	}
	in := sh.Incoming(func(key string) string { id, _ := lookup(KindTest, key); return id }, func(id string) bool { return id == "t1" || id == "t2" })
	d := Compare(KindTest, fs, in, ItemsByID([]*Item{t1, t2}), lookup)
	if d.Summary.Unchanged != 2 {
		t.Fatalf("round trip not unchanged: %+v %+v", d.Summary, d.Rows)
	}
	if _, err := ReadWorkbook(buf.Bytes(), KindRequirement, func(string) []Field { return fs }); err == nil {
		t.Error("kind mismatch accepted")
	}
	if _, err := ReadWorkbook([]byte("nope"), "", nil); err == nil {
		t.Error("not a zip")
	}
	// a hand-made sheet without the meta sheet: matched by header, by key
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Requirements")
	_ = f.SetSheetRow("Requirements", "A1", &[]any{"Key", "Titel", "Version", "Prioritet", "Comment", "Title"})
	_ = f.SetSheetRow("Requirements", "A2", &[]any{"P-REQ-1", "Renamed", "x"})
	_ = f.SetSheetRow("Requirements", "A4", &[]any{"", "Brand new", "", "high"})
	var hb bytes.Buffer
	_ = f.Write(&hb)
	rfs := BuiltinFields(KindRequirement)
	sh, err = ReadWorkbook(hb.Bytes(), "", func(string) []Field { return rfs })
	if err != nil {
		t.Fatal(err)
	}
	if len(sh.Rows) != 2 || len(sh.Ignored) != 1 || len(sh.Warnings) != 1 {
		t.Fatalf("hand-made: %+v", sh)
	}
	in = sh.Incoming(func(key string) string { return map[string]string{"P-REQ-1": "a"}[key] }, nil)
	if in[0].ID != "a" || len(in[0].Errors) != 1 || in[1].ID != "" {
		t.Errorf("incoming: %+v", in)
	}
	// no ID or Key column
	f2 := excelize.NewFile()
	_ = f2.SetSheetName("Sheet1", "Tests")
	_ = f2.SetSheetRow("Tests", "A1", &[]any{"Title"})
	var nb bytes.Buffer
	_ = f2.Write(&nb)
	if _, err := ReadWorkbook(nb.Bytes(), "", func(string) []Field { return fs }); err == nil {
		t.Error("sheet without ID accepted")
	}
}

func TestReqIFPlanAndRelationTypes(t *testing.T) {
	for name, want := range map[string]string{"Satisfies": "refines", "derived-from": "derived_from", "Depends On": "depends_on", "conflicts": "conflicts_with",
		"is duplicate of": "duplicates", "implements": "provides", "trace": "derived_from", "whatever": "relates_to", "relates_to": "relates_to"} {
		if got := RelationType(name); got != want {
			t.Errorf("RelationType(%q) = %s, want %s", name, got, want)
		}
	}
	fs := BuiltinFields(KindRequirement)
	if err := CheckMapping(fs, map[string]string{"a": "heading", "b": "", "c": "title"}); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"rel:refines", "verified_by", "nope"} {
		if err := CheckMapping(fs, map[string]string{"a": bad}); err == nil {
			t.Errorf("mapping %s accepted", bad)
		}
	}
	data, err := os.ReadFile("testdata/doors.reqif")
	if err != nil {
		t.Fatal(err)
	}
	m, err := reqimport.ParseReqIFModel(data)
	if err != nil {
		t.Fatal(err)
	}
	// without the hierarchy every object with text is a requirement
	p := PlanReqIF(m, fs, ReqIFOptions{}, nil)
	if len(p.Rows) != 6 || p.Folders != 0 || len(p.Relations) != 2 {
		t.Fatalf("flat: %d rows, %d folders, %d relations", len(p.Rows), p.Folders, len(p.Relations))
	}
	// the chapter name ignored, the verification method as rationale
	p = PlanReqIF(m, fs, ReqIFOptions{Hierarchy: true, Mapping: map[string]string{"ReqIF.ChapterName": "ignore", "verification method": "rationale"}}, nil)
	if p.Folders != 0 || len(p.Rows) != 3 || len(p.Skipped) != 3 || p.Rows[0].Cells["rationale"].Raw != "Test" {
		t.Errorf("mapped: %d rows %v %+v", len(p.Rows), p.Skipped, p.Rows[0].Cells)
	}
	// a multi-document archive: each specification is a top folder
	zipped := zipOf(t, map[string][]byte{"a.reqif": data, "b/polarion.reqif": mustRead(t, "testdata/polarion.reqif")})
	m, err = reqimport.ParseReqIFModel(zipped)
	if err != nil {
		t.Fatal(err)
	}
	p = PlanReqIF(m, fs, ReqIFOptions{Hierarchy: true, Root: "Imports"}, func(id string) (string, int) {
		if id == "_o-2" {
			return "x", 4
		}
		return "", 0
	})
	paths := map[string]bool{}
	for _, r := range p.Rows {
		paths[r.Cells["folder"].Raw] = true
		if r.Ref == "_o-2" && (r.ID != "x" || r.BaseVersion != 4 || r.Cells["title"].Raw != "") {
			t.Errorf("matched row: %+v", r)
		}
	}
	if !paths["Imports/System Requirements/Braking/Emergency braking"] || !paths["Imports/Product Requirements Specification/Loans"] {
		t.Errorf("paths: %v", paths)
	}
	if _, err := reqimport.ParseReqIFModel([]byte("")); err == nil {
		t.Error("empty accepted")
	}
	if _, err := reqimport.ParseReqIFModel(zipOf(t, map[string][]byte{"x.txt": []byte("x")})); err == nil {
		t.Error("zip without reqif accepted")
	}
	if got := titleFrom("# " + strings.Repeat("word ", 12)); !strings.HasSuffix(got, "…") || strings.HasPrefix(got, "#") {
		t.Errorf("titleFrom: %q", got)
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
