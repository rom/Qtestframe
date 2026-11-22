package report

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

var t0 = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func sampleData() *Data {
	cats := map[string]model.Category{
		"c1": {ID: "c1", Dimension: "Polarity", Name: "Positive"},
		"c2": {ID: "c2", Dimension: "Polarity", Name: "Negative"},
		"c3": {ID: "c3", Dimension: "Level", Name: "System"},
	}
	reqs := []model.Requirement{
		{ID: "r1", Key: "P-REQ-1", Title: "Login", Status: "approved", Obligation: "must", Priority: "high", LinkedTests: 2, Coverage: "failed"},
		{ID: "r2", Key: "P-REQ-2", Title: "Logout", Status: "draft", Obligation: "", Priority: "low", LinkedTests: 1, Coverage: "passed"},
		{ID: "r3", Key: "P-REQ-10", Title: "Audit", Status: "custom_status", Obligation: "should", LinkedTests: 0, Coverage: "none"},
	}
	tests := []model.TestCase{
		{ID: "t1", Key: "P-TC-1", Title: "Valid login", ExecutionType: "manual", Status: "ready", Priority: "high", FolderPath: "Auth", CategoryIDs: []string{"c1", "c3"}, LinkedReqs: 1, LastResult: "passed", EstimatedMin: 5},
		{ID: "t2", Key: "P-TC-2", Title: "Wrong password", ExecutionType: "automated", Status: "ready", Priority: "medium", FolderPath: "Auth / Negative", CategoryIDs: []string{"c2", "gone"}, LinkedReqs: 1, LastResult: "failed", Script: "exit 1", EstimatedMin: 3},
		{ID: "t3", Key: "P-TC-3", Title: "Orphan", ExecutionType: "manual", Status: "draft", Priority: "low", LinkedReqs: 0, LastResult: ""},
	}
	links := []model.TraceLink{
		{RequirementID: "r1", TestCaseID: "t1", RequirementKey: "P-REQ-1", TestCaseKey: "P-TC-1"},
		{RequirementID: "r1", TestCaseID: "t2", RequirementKey: "P-REQ-1", TestCaseKey: "P-TC-2"},
		{RequirementID: "r2", TestCaseID: "t1", RequirementKey: "P-REQ-2", TestCaseKey: "P-TC-1"},
		{RequirementID: "r2", TestCaseID: "tx", RequirementKey: "P-REQ-2", TestCaseKey: "P-TC-X"}, // filtered out test
	}
	fin := t0.Add(time.Hour)
	runs := []model.TestRun{
		{ID: "run2", Name: "Run 2", Status: "in_progress", CreatedAt: t0.Add(24 * time.Hour), Build: "1.1", Summary: map[string]int{"total": 2, "passed": 1, "not_run": 1}},
		{ID: "run1", Name: "Run 1", Status: "completed", CreatedAt: t0, StartedAt: &t0, CompletedAt: &fin, Build: "1.0", Environment: "staging",
			Summary: map[string]int{"total": 3, "passed": 1, "failed": 1, "blocked": 1}},
	}
	execs := map[string][]model.Execution{
		"run1": {
			{TestCaseID: "t1", TestKey: "P-TC-1", TestTitle: "Valid login", Status: "passed", DurationMS: 1500, StartedAt: &t0, FinishedAt: &fin},
			{TestCaseID: "t2", TestKey: "P-TC-2", TestTitle: "Wrong password", Status: "failed", DurationMS: 500, Defects: []string{"BUG-10", "BUG-2"}, Notes: "leaks"},
			{TestCaseID: "t3", TestKey: "P-TC-3", TestTitle: "Orphan", Status: "blocked", Defects: []string{"https://bugs.example/7", " "}},
		},
		"run2": {
			{TestCaseID: "t2", TestKey: "P-TC-2", TestTitle: "Wrong password", Status: "passed", Defects: []string{"BUG-2"}, FinishedAt: ptr(t0.Add(25 * time.Hour))},
			{TestCaseID: "t1", TestKey: "P-TC-1", Status: "not_run"},
		},
	}
	return &Data{
		Project:      model.Project{Key: "P", Name: "Proj", DefectURLTemplate: "https://jira.example/browse/{id}"},
		Requirements: reqs, Tests: tests, Links: links, Runs: runs, Executions: execs, Categories: cats, Now: t0, Risks: sampleRisks(),
	}
}

func section(t *testing.T, ds *export.Dataset, name string) export.Section {
	t.Helper()
	for _, s := range ds.Sections {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no section %q in %v", name, names(ds))
	return export.Section{}
}

func names(ds *export.Dataset) []string {
	var out []string
	for _, s := range ds.Sections {
		out = append(out, s.Name)
	}
	return out
}

// metric returns the value of a KPI row.
func metric(t *testing.T, s export.Section, k string) string {
	t.Helper()
	for _, r := range s.Rows {
		if r[0] == k {
			return r[1]
		}
	}
	t.Fatalf("no metric %q in %v", k, s.Rows)
	return ""
}

func renderAll(t *testing.T, ds *export.Dataset) {
	t.Helper()
	for _, f := range []export.Format{export.PDF, export.DOCX, export.XLSX, export.CSV} {
		var buf bytes.Buffer
		if err := export.Write(&buf, f, *ds); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("%s: empty", f)
		}
		if f == export.CSV && len(ds.Sections) > 1 {
			if _, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
				t.Fatalf("csv zip: %v", err)
			}
		}
	}
}

func TestTemplatesList(t *testing.T) {
	ts := Templates()
	if len(ts) != 10 {
		t.Fatal(len(ts))
	}
	b, _ := json.Marshal(ts)
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	for _, x := range out {
		for _, k := range []string{"id", "name", "description", "params", "formats"} {
			if _, ok := x[k]; !ok {
				t.Fatalf("missing %s in %v", k, x)
			}
		}
		if len(x["formats"].([]any)) != 5 {
			t.Fatal(x)
		}
	}
	rs, ok := Lookup(RunSummary)
	if !ok || !rs.Params[0].Required || rs.Params[0].Name != "run_id" {
		t.Fatal(rs)
	}
	// returned slices are copies
	ts[0].Params = append(ts[0].Params, Param{Name: "x"})
	if p, _ := Lookup(ts[0].ID); len(p.Params) != 0 {
		t.Fatal("template params mutated")
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("lookup")
	}
}

func TestBuildErrors(t *testing.T) {
	d := sampleData()
	if _, err := Build("nope", d, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := Build(RunSummary, d, map[string]string{}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "run_id") {
		t.Fatal(err)
	}
	if _, err := Build(RunSummary, d, map[string]string{"run_id": "other"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestProjectStatus(t *testing.T) {
	d := sampleData()
	ds, err := Build(ProjectStatus, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Title != "Project status" || ds.Subtitle != "P – Proj" || !ds.GeneratedAt.Equal(t0) {
		t.Fatal(ds.Title, ds.Subtitle, ds.GeneratedAt)
	}
	k := section(t, ds, "kpis")
	if len(k.Columns) != 2 {
		t.Fatal(k.Columns)
	}
	checks := map[string]string{"Requirements": "3", "Requirements with linked tests": "2", "Requirements without tests": "1",
		"Test coverage (requirements with tests)": "66.7%", "Requirements verified (all linked tests passed)": "1",
		"Test cases": "3", "Automated test cases": "1", "Manual test cases": "2", "Pass rate (last results)": "50.0%",
		"Test runs": "2", "Runs in progress": "1"}
	for m, v := range checks {
		if got := metric(t, k, m); got != v {
			t.Errorf("%s = %s, want %s", m, got, v)
		}
	}
	st := section(t, ds, "requirements_by_status")
	if st.Rows[0][0] != "Draft" || st.Rows[0][1] != "1" {
		t.Fatal(st.Rows)
	}
	last := st.Rows[len(st.Rows)-1]
	if last[0] != "Total" || last[1] != "3" || last[2] != "100.0%" {
		t.Fatal(last)
	}
	// unknown status listed after the known ones
	if st.Rows[len(st.Rows)-2][0] != "custom_status" {
		t.Fatal(st.Rows)
	}
	ob := section(t, ds, "requirements_by_obligation")
	if ob.Rows[0][0] != "MUST" || metric(t, ob, "Unspecified") != "1" {
		t.Fatal(ob.Rows)
	}
	cov := section(t, ds, "coverage")
	if metric(t, cov, "No linked tests") != "1" || metric(t, cov, "Failed") != "1" {
		t.Fatal(cov.Rows)
	}
	res := section(t, ds, "test_results")
	if metric(t, res, "Not run") != "1" {
		t.Fatal(res.Rows)
	}
	rr := section(t, ds, "recent_runs")
	if len(rr.Rows) != 2 || rr.Rows[0][0] != "Run 2" || rr.Rows[1][5] != "3" || rr.Rows[1][7] != "1" {
		t.Fatal(rr.Rows)
	}
	if s := newLoc("").recentRuns(append(d.Runs, d.Runs...), 3); len(s.Rows) != 3 {
		t.Fatal(len(s.Rows))
	}
	renderAll(t, ds)
}

func TestRequirementsCoverage(t *testing.T) {
	d := sampleData()
	d.Query = "status:approved"
	ds, err := Build(RequirementsCoverage, d, map[string]string{"query": "status:approved"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ds.Subtitle, "Filter: status:approved") {
		t.Fatal(ds.Subtitle)
	}
	sum := section(t, ds, "summary")
	if metric(t, sum, "Requirements") != "3" || metric(t, sum, "With linked tests") != "2" || metric(t, sum, "Verified share") != "33.3%" {
		t.Fatal(sum.Rows)
	}
	det := section(t, ds, "requirements")
	r1 := det.Rows[0]
	if r1[0] != "P-REQ-1" || r1[3] != "MUST" || r1[5] != "P-TC-1, P-TC-2" || r1[6] != "P-TC-1: Passed\nP-TC-2: Failed" || r1[7] != "Failed" {
		t.Fatal(r1)
	}
	// links to tests outside the data are skipped
	if det.Rows[1][5] != "P-TC-1" {
		t.Fatal(det.Rows[1])
	}
	if det.Rows[2][7] != "No linked tests" {
		t.Fatal(det.Rows[2])
	}
	if coverageLabel("weird_one") != "Weird one" {
		t.Fatal(coverageLabel("weird_one"))
	}
	renderAll(t, ds)
}

func TestTraceability(t *testing.T) {
	ds, err := Build(Traceability, sampleData(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := section(t, ds, "summary")
	if metric(t, sum, "Links") != "3" || metric(t, sum, "Requirements without tests") != "1" || metric(t, sum, "Tests without requirements") != "1" {
		t.Fatal(sum.Rows)
	}
	m := section(t, ds, "matrix")
	if len(m.Rows) != 4 || m.Rows[3][0] != "P-REQ-10" || m.Rows[3][3] != "" {
		t.Fatal(m.Rows)
	}
	o := section(t, ds, "tests_without_requirements")
	if len(o.Rows) != 1 || o.Rows[0][0] != "P-TC-3" || o.Rows[0][3] != "Not run" {
		t.Fatal(o.Rows)
	}
	renderAll(t, ds)
}

func TestRunSummary(t *testing.T) {
	ds, err := Build(RunSummary, sampleData(), map[string]string{"run_id": " run1 "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(ds.Subtitle, "Run: Run 1") {
		t.Fatal(ds.Subtitle)
	}
	sum := section(t, ds, "summary")
	checks := map[string]string{"Run": "Run 1", "Environment": "staging", "Executions": "3", "Passed": "1", "Failed": "1",
		"Blocked": "1", "Not run": "0", "Progress (executions with a result)": "100.0%", "Pass rate (of executions with a result)": "33.3%",
		"Total duration": "2s", "Average duration": "1s", "Defects": "3", "Started": "2026-03-01T10:00:00Z", "Completed": "2026-03-01T11:00:00Z"}
	for k, v := range checks {
		if got := metric(t, sum, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	ex := section(t, ds, "executions")
	if len(ex.Rows) != 3 || ex.Rows[1][9] != "BUG-10, BUG-2" || ex.Rows[0][8] != "1500" {
		t.Fatal(ex.Rows)
	}
	df := section(t, ds, "defects")
	// natural order, blank ids dropped: BUG-2 < BUG-10 < https://...
	if len(df.Rows) != 3 || df.Rows[0][0] != "BUG-2" || df.Rows[1][0] != "BUG-10" || df.Rows[0][1] != "https://jira.example/browse/BUG-2" || df.Rows[2][1] != "https://bugs.example/7" {
		t.Fatal(df.Rows)
	}
	// a run without executions
	ds, err = Build(RunSummary, sampleData(), map[string]string{"run_id": "run2"})
	if err != nil {
		t.Fatal(err)
	}
	if v := metric(t, section(t, ds, "summary"), "Average duration"); v != "–" {
		t.Fatal(v)
	}
	renderAll(t, ds)
}

func TestDefects(t *testing.T) {
	d := sampleData()
	d.Project.DefectURLTemplate = ""
	ds, err := Build(Defects, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := section(t, ds, "summary")
	if metric(t, sum, "Distinct defects") != "3" || metric(t, sum, "Occurrences") != "4" || metric(t, sum, "Affected tests") != "2" || metric(t, sum, "Affected runs") != "2" {
		t.Fatal(sum.Rows)
	}
	l := section(t, ds, "defects")
	bug2 := l.Rows[0]
	if bug2[0] != "BUG-2" || bug2[1] != "" || bug2[2] != "2" || bug2[3] != "P-TC-2" || bug2[4] != "Run 1, Run 2" || bug2[5] != "Passed" || bug2[6] != "2026-03-02T11:00:00Z" {
		t.Fatal(bug2)
	}
	if l.Rows[2][1] != "https://bugs.example/7" {
		t.Fatal(l.Rows[2])
	}
	occ := section(t, ds, "occurrences")
	if len(occ.Rows) != 4 || occ.Rows[0][1] != "Run 1" {
		t.Fatal(occ.Rows)
	}
	renderAll(t, ds)
	// no defects at all
	ds, err = Build(Defects, &Data{}, nil)
	if err != nil || len(section(t, ds, "defects").Rows) != 0 {
		t.Fatal(err)
	}
	renderAll(t, ds)
}

func TestTestInventory(t *testing.T) {
	ds, err := Build(TestInventory, sampleData(), map[string]string{"query": ""})
	if err != nil {
		t.Fatal(err)
	}
	sum := section(t, ds, "summary")
	checks := map[string]string{"Test cases": "3", "Manual": "2", "Automated": "1", "With script": "1", "Linked to requirements": "2",
		"Not linked to requirements": "1", "Total estimate (minutes)": "8"}
	for k, v := range checks {
		if got := metric(t, sum, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	bf := section(t, ds, "by_folder")
	if bf.Rows[0][0] != "(no folder)" || bf.Rows[1][0] != "Auth" || bf.Rows[2][0] != "Auth / Negative" {
		t.Fatal(bf.Rows)
	}
	bc := section(t, ds, "by_category")
	if len(bc.Rows) != 3 || bc.Rows[0][0] != "Level" || bc.Rows[1][1] != "Negative" {
		t.Fatal(bc.Rows)
	}
	if metric(t, section(t, ds, "by_execution_type"), "Automated") != "1" || metric(t, section(t, ds, "by_status"), "Ready") != "2" ||
		metric(t, section(t, ds, "by_priority"), "High") != "1" {
		t.Fatal("counts")
	}
	list := section(t, ds, "tests")
	if list.Rows[0][6] != "Level: System, Polarity: Positive" || list.Rows[1][6] != "Polarity: Negative" || list.Rows[2][8] != "Not run" {
		t.Fatal(list.Rows)
	}
	renderAll(t, ds)
}

func TestHelpers(t *testing.T) {
	if pct(1, 0) != "–" || pct(1, 4) != "25.0%" {
		t.Fatal(pct(1, 4))
	}
	if humanize("") != "" || humanize("non_functional") != "Non functional" {
		t.Fatal(humanize("non_functional"))
	}
	if ts(time.Time{}) != "" || tsPtr(nil) != "" {
		t.Fatal("ts")
	}
	if defectLink("", "X-1") != "" || defectLink("https://x/{id}", "X-1") != "https://x/X-1" || defectLink("nope", "X") != "" || defectLink("", "http://a") != "http://a" {
		t.Fatal("defectLink")
	}
	if durationText(61500) != "1m1.5s" {
		t.Fatal(durationText(61500))
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"REQ-2", "REQ-10", -1}, {"REQ-10", "REQ-2", 1}, {"req-1", "REQ-1", 0}, {"a", "b", -1}, {"b", "a", 1},
		{"a", "ab", -1}, {"ab", "a", 1}, {"x007", "x7", 0}, {"x8", "x07", 1}, {"x12", "x13", -1}, {"", "", 0},
	}
	for _, c := range cases {
		if got := naturalCompare(c.a, c.b); got != c.want {
			t.Errorf("naturalCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if got := uniqSorted([]string{"b", "", "a10", "a2", "b"}); strings.Join(got, ",") != "a2,a10,b" {
		t.Fatal(got)
	}
	d := &Data{}
	if d.now().IsZero() {
		t.Fatal("now")
	}
}

// ---------------------------------------------------------------- custom

func TestFields(t *testing.T) {
	for _, e := range Entities {
		fs := Fields(e)
		if len(fs) == 0 {
			t.Fatal(e)
		}
		seen := map[string]bool{}
		for _, f := range fs {
			if f.ID == "" || f.Label == "" || seen[f.ID] {
				t.Fatalf("%s: %+v", e, f)
			}
			seen[f.ID] = true
		}
	}
	if Fields("nope") != nil {
		t.Fatal("unknown entity")
	}
	b, _ := json.Marshal(Fields(EntityRun)[0])
	if string(b) != `{"id":"name","label":"Name"}` {
		t.Fatal(string(b))
	}
}

func TestValidate(t *testing.T) {
	cols := func(n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, Fields(EntityRequirement)[i%len(Fields(EntityRequirement))].ID)
		}
		return out
	}
	bad := []CustomConfig{
		{Entity: "nope", Columns: []string{"key"}},
		{Entity: "requirement"},
		{Entity: "requirement", Columns: cols(41)},
		{Entity: "requirement", Columns: []string{"nope"}},
		{Entity: "requirement", Columns: []string{"key", "key"}},
		{Entity: "requirement", Columns: []string{"key"}, GroupBy: "nope"},
		{Entity: "requirement", Columns: []string{"key"}, SortBy: "nope"},
		{Entity: "requirement", Columns: []string{"key"}, RunID: "r"},
		{Entity: "requirement", Columns: []string{"key"}, Title: strings.Repeat("x", 201)},
		{Entity: "run", Columns: []string{"key"}},
	}
	for i, c := range bad {
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%d: %v", i, err)
		}
	}
	c := CustomConfig{Entity: " Execution ", Columns: []string{" key ", "duration_ms"}, GroupBy: " run ", SortBy: "key", RunID: " r1 ", Title: " T "}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Entity != "execution" || c.Columns[0] != "key" || c.GroupBy != "run" || c.RunID != "r1" || c.Title != "T" {
		t.Fatal(c)
	}
	if _, err := ParseCustomConfig(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := ParseCustomConfig(json.RawMessage(`null`)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := ParseCustomConfig(json.RawMessage(`{"entity":1}`)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := ParseCustomConfig(json.RawMessage(`{"entity":"test","columns":[]}`)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	pc, err := ParseCustomConfig(json.RawMessage(`{"entity":"test","columns":["key","title"],"include_summary":true}`))
	if err != nil || !pc.IncludeSummary {
		t.Fatal(pc, err)
	}
}

func recs() []query.MapRecord {
	return []query.MapRecord{
		{"key": {"P-REQ-10"}, "title": {"Ten"}, "status": {"approved"}, "tag": {"b", "a"}, "criteria": {"1. x", "2. y"}},
		{"key": {"P-REQ-2"}, "title": {"Two"}, "status": {"draft"}, "tag": {"a"}},
		{"key": {"P-REQ-1"}, "title": {"One"}, "status": {"approved"}},
	}
}

func TestBuildCustomPlain(t *testing.T) {
	c := CustomConfig{Entity: "requirement", Columns: []string{"key", "tag", "criteria"}, SortBy: "key", Query: "status:x"}
	ds := BuildCustom(c, recs(), "P – Proj", t0, "")
	if ds.Title != "Custom report – Requirements" || ds.Subtitle != "P – Proj · Filter: status:x" || !ds.GeneratedAt.Equal(t0) {
		t.Fatal(ds.Title, ds.Subtitle)
	}
	if len(ds.Sections) != 1 {
		t.Fatal(names(ds))
	}
	s := ds.Sections[0]
	if strings.Join(s.Columns, "|") != "Key|Tags|Evaluation criteria" {
		t.Fatal(s.Columns)
	}
	if s.Rows[0][0] != "P-REQ-1" || s.Rows[1][0] != "P-REQ-2" || s.Rows[2][0] != "P-REQ-10" {
		t.Fatal(s.Rows)
	}
	if s.Rows[2][1] != "b, a" || s.Rows[2][2] != "1. x\n2. y" {
		t.Fatal(s.Rows[2])
	}
	c.SortDesc, c.IncludeSummary, c.Title = true, true, "Mine"
	ds = BuildCustom(c, recs(), "P", time.Time{}, "en")
	if ds.Title != "Mine" || len(ds.Sections) != 2 || ds.Sections[0].Name != "summary" || ds.GeneratedAt.IsZero() {
		t.Fatal(ds.Title, names(ds))
	}
	if metric(t, ds.Sections[0], "Requirements") != "3" || ds.Sections[1].Rows[0][0] != "P-REQ-10" {
		t.Fatal(ds.Sections)
	}
	renderAll(t, ds)
}

func TestBuildCustomGrouped(t *testing.T) {
	c := CustomConfig{Entity: "requirement", Columns: []string{"key", "title"}, GroupBy: "tag", SortBy: "key", IncludeSummary: true}
	ds := BuildCustom(c, recs(), "P", t0, "")
	// summary, a, b, (none)
	if got := strings.Join(names(ds), ","); got != "summary,a,b,(none)" {
		t.Fatal(got)
	}
	sum := ds.Sections[0]
	if sum.Columns[0] != "Tags" || sum.Rows[0][0] != "a" || sum.Rows[0][1] != "2" || sum.Rows[2][0] != "(none)" ||
		sum.Rows[3][0] != "Total requirements" || sum.Rows[3][1] != "3" {
		t.Fatal(sum.Rows)
	}
	a := ds.Sections[1]
	if a.Title != "Tags: a" || len(a.Rows) != 2 || a.Rows[0][0] != "P-REQ-2" {
		t.Fatal(a)
	}
	renderAll(t, ds)
	// grouping without records gives an empty table
	ds = BuildCustom(CustomConfig{Entity: "run", Columns: []string{"name"}, GroupBy: "status"}, nil, "P", t0, "")
	if len(ds.Sections) != 1 || ds.Sections[0].Title != "Test runs" {
		t.Fatal(names(ds))
	}
	renderAll(t, ds)
}
