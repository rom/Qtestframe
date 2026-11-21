package report

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/tracerules"
)

// failOnMissing makes every text without a translation fail the test.
func failOnMissing(t *testing.T) {
	t.Helper()
	missingTranslation = func(lang, key string) { t.Errorf("no %s translation of %q", lang, key) }
	t.Cleanup(func() { missingTranslation = nil })
}

// swedishData is the sample data with trace rules and a run param.
func swedishData() *Data {
	d := sampleData()
	d.Lang = "sv-SE"
	d.Query = "status:approved"
	d.TraceRules = []tracerules.Result{
		{Rule: model.TraceRule{Name: "MUST verifieras", Severity: "error", Scope: "requirement"}, Matched: 2, ViolationCount: 1,
			Violations: []tracerules.Violation{{Item: tracerules.Item{Key: "P-REQ-1", Title: "Login"}, Found: 0, Required: 1}}},
		{Rule: model.TraceRule{Name: "Testfall har krav", Severity: "warning", Scope: "test"}, Matched: 3, Passed: true},
		{Rule: model.TraceRule{Name: "Trasig", Severity: "warning", Scope: "test"}, Error: "bad query"},
	}
	return d
}

// english lists English texts that must not appear in a Swedish report's
// own texts (titles, headers, labels).
var english = []string{"Requirements", "Summary", "Metric", "Value", "Count", "Share", "Total", "Passed", "Failed",
	"Not run", "Coverage", "Test cases", "Key figures", "Created", "Title", "Key", "Defects", "Run", "Folder", "Priority"}

func checkSwedish(t *testing.T, ds *export.Dataset) {
	t.Helper()
	if ds.Lang != Swedish {
		t.Fatalf("lang %q", ds.Lang)
	}
	texts := []string{ds.Title}
	for _, s := range ds.Sections {
		texts = append(texts, s.Title)
		texts = append(texts, s.Columns...)
		for _, r := range s.Rows {
			if len(r) > 0 && s.Columns[0] == "Mått" {
				texts = append(texts, r[0]) // metric names
			}
		}
	}
	for _, x := range texts {
		for _, e := range english {
			if x == e {
				t.Errorf("English text %q in %s", x, ds.Title)
			}
		}
	}
}

func TestSwedishTemplates(t *testing.T) {
	failOnMissing(t)
	for _, tpl := range Templates() {
		ds, err := Build(tpl.ID, swedishData(), map[string]string{"run_id": "run1"})
		if err != nil {
			t.Fatal(tpl.ID, err)
		}
		checkSwedish(t, ds)
		renderAll(t, ds)
	}
}

func TestSwedishProjectStatus(t *testing.T) {
	failOnMissing(t)
	ds, _ := Build(ProjectStatus, swedishData(), nil)
	if ds.Title != "Projektstatus" || ds.Subtitle != "P – Proj · Filter: status:approved" {
		t.Fatal(ds.Title, ds.Subtitle)
	}
	k := section(t, ds, "kpis")
	if k.Title != "Nyckeltal" || metric(t, k, "Krav") != "3" || metric(t, k, "Testtäckning (krav med testfall)") != "66,7 %" {
		t.Fatal(k)
	}
	st := section(t, ds, "requirements_by_status")
	if st.Title != "Krav per status" || strings.Join(st.Columns, "|") != "Status|Antal|Andel" || st.Rows[0][0] != "Utkast" ||
		st.Rows[2][0] != "Godkänt" || st.Rows[len(st.Rows)-1][0] != "Totalt" {
		t.Fatal(st)
	}
	ob := section(t, ds, "requirements_by_obligation")
	if ob.Rows[0][0] != "MUST (SKA)" || ob.Rows[4][0] != "SHOULD (BÖR)" || ob.Rows[6][0] != "MAY (FÅR)" || ob.Rows[10][0] != "Ej angiven" {
		t.Fatal(ob.Rows)
	}
	cov := section(t, ds, "coverage")
	if cov.Title != "Kravtäckning" || cov.Rows[0][0] != "Godkänd" || cov.Rows[1][0] != "Delvis" || cov.Rows[4][0] != "Ingen täckning" {
		t.Fatal(cov.Rows)
	}
	res := section(t, ds, "test_results")
	if res.Columns[0] != "Utfall" || res.Rows[0][0] != "Ej körd" || res.Rows[3][0] != "Underkänd" {
		t.Fatal(res.Rows)
	}
	runs := section(t, ds, "recent_runs")
	if runs.Columns[0] != "Testkörning" || runs.Rows[0][1] != "Pågår" || runs.Rows[1][1] != "Slutförd" {
		t.Fatal(runs.Rows)
	}
}

func TestSwedishCoverageAndTraceability(t *testing.T) {
	failOnMissing(t)
	ds, _ := Build(RequirementsCoverage, swedishData(), nil)
	r := section(t, ds, "requirements")
	if ds.Title != "Kravtäckning" || r.Columns[4] != "Prioritet" || r.Rows[0][2] != "Godkänt" || r.Rows[0][3] != "MUST (SKA)" ||
		r.Rows[0][4] != "Hög" || r.Rows[0][6] != "P-TC-1: Godkänd\nP-TC-2: Underkänd" || r.Rows[0][7] != "Underkänd" || r.Rows[2][2] != "Custom status" {
		t.Fatal(r.Rows)
	}
	if metric(t, section(t, ds, "summary"), "Andel verifierade") != "33,3 %" {
		t.Fatal(ds.Sections[0])
	}
	ds, _ = Build(Traceability, swedishData(), nil)
	if ds.Title != "Spårbarhet" || section(t, ds, "matrix").Title != "Spårbarhetsmatris" {
		t.Fatal(ds.Title)
	}
	rules := section(t, ds, "trace_rules")
	if rules.Rows[0][1] != "Fel" || rules.Rows[0][2] != "Krav" || rules.Rows[0][6] != "Bryts" || rules.Rows[1][1] != "Varning" ||
		rules.Rows[1][2] != "Testfall" || rules.Rows[1][6] != "Uppfylld" || rules.Rows[2][6] != "Ogiltig: bad query" || rules.Rows[0][0] != "MUST verifieras" {
		t.Fatal(rules.Rows)
	}
	v := section(t, ds, "trace_rule_violations")
	if v.Title != "Överträdelser av spårbarhetsregler" || v.Columns[4] != "Hittade" || v.Rows[0][1] != "Fel" {
		t.Fatal(v)
	}
}

func TestSwedishRunsDefectsInventory(t *testing.T) {
	failOnMissing(t)
	ds, _ := Build(RunSummary, swedishData(), map[string]string{"run_id": "run1"})
	sum := section(t, ds, "summary")
	if ds.Title != "Sammanfattning av körning" || ds.Subtitle != "P – Proj · Testkörning: Run 1" || metric(t, sum, "Status") != "Slutförd" ||
		metric(t, sum, "Godkända") != "1" || metric(t, sum, "Pågående") != "0" || metric(t, sum, "Andel godkända (av exekveringar med utfall)") != "33,3 %" ||
		metric(t, sum, "Genomsnittlig tid") != "1s" {
		t.Fatal(sum.Rows)
	}
	ex := section(t, ds, "executions")
	if ex.Title != "Exekveringar" || ex.Columns[3] != "Utfall" || ex.Rows[0][3] != "Godkänd" || ex.Rows[1][10] != "leaks" {
		t.Fatal(ex.Rows)
	}
	ds, _ = Build(Defects, swedishData(), nil)
	if ds.Title != "Defekter" || section(t, ds, "defects").Columns[6] != "Senast sedd" || section(t, ds, "occurrences").Rows[0][5] != "Underkänd" {
		t.Fatal(ds.Sections)
	}
	ds, _ = Build(TestInventory, swedishData(), nil)
	list := section(t, ds, "tests")
	if ds.Title != "Testförteckning" || list.Rows[0][3] != "Manuellt" || list.Rows[0][4] != "Klart" || list.Rows[1][3] != "Automatiserat" {
		t.Fatal(list.Rows)
	}
	folders := section(t, ds, "by_folder")
	if folders.Rows[0][0] != "(ingen mapp)" && folders.Rows[2][0] != "(ingen mapp)" {
		t.Fatal(folders.Rows)
	}
	if section(t, ds, "by_priority").Rows[1][0] != "Hög" || section(t, ds, "by_execution_type").Columns[0] != "Exekveringstyp" {
		t.Fatal(ds.Sections)
	}
}

func TestSwedishCustom(t *testing.T) {
	failOnMissing(t)
	for entity, fs := range fields {
		cols := []string{}
		for _, f := range fs {
			cols = append(cols, f.ID)
		}
		ds := BuildCustom(CustomConfig{Entity: entity, Columns: cols, IncludeSummary: true, Query: "x"}, nil, "P", t0, "sv")
		checkSwedish(t, ds)
		ds = BuildCustom(CustomConfig{Entity: entity, Columns: cols, IncludeSummary: true, GroupBy: "status"}, nil, "P", t0, "sv")
		checkSwedish(t, ds)
	}
	c := CustomConfig{Entity: "requirement", Columns: []string{"key", "status", "obligation", "tag"}, GroupBy: "status", SortBy: "key", IncludeSummary: true}
	recs := append(recs(), query.MapRecord{"key": {"P-REQ-3"}, "obligation": {"should"}})
	ds := BuildCustom(c, recs, "P", t0, "sv")
	if ds.Title != "Anpassad rapport – Krav" || ds.Lang != "sv" {
		t.Fatal(ds.Title)
	}
	if got := strings.Join(names(ds), ","); got != "summary,Godkänt,Utkast,(ingen)" {
		t.Fatal(got)
	}
	sum := ds.Sections[0]
	if sum.Title != "Sammanfattning per status" || strings.Join(sum.Columns, "|") != "Status|Antal|Andel" || sum.Rows[0][2] != "50,0 %" ||
		sum.Rows[3][0] != "Totalt krav" {
		t.Fatal(sum)
	}
	g := ds.Sections[1]
	if g.Title != "Status: Godkänt" || strings.Join(g.Columns, "|") != "Nyckel|Status|Kravnivå|Taggar" || g.Rows[0][1] != "Godkänt" {
		t.Fatal(g)
	}
	if ds.Sections[3].Rows[0][2] != "SHOULD (BÖR)" {
		t.Fatal(ds.Sections[3].Rows)
	}
	// English keeps the stored values; the records are not changed
	ds = BuildCustom(c, recs, "P", t0, "de")
	if ds.Lang != English || ds.Sections[1].Title != "Status: approved" || recs[0]["status"][0] != "approved" {
		t.Fatal(ds.Sections[1].Title)
	}
	ds = BuildCustom(CustomConfig{Entity: "execution", Columns: []string{"key", "status"}}, []query.MapRecord{{"key": {"T-1"}, "status": {"blocked"}}}, "P", t0, "sv")
	if ds.Sections[0].Rows[0][1] != "Blockerad" || ds.Sections[0].Columns[1] != "Utfall" || ds.Sections[0].Title != "Exekveringar" {
		t.Fatal(ds.Sections[0])
	}
}

func TestLanguageHelpers(t *testing.T) {
	for in, want := range map[string]string{"sv": "sv", "SV": "sv", " sv-SE ": "sv", "sv_FI": "sv", "en-GB": "en", "de": "", "": "", "svenska": ""} {
		if got := NormalizeLang(in); got != want {
			t.Errorf("NormalizeLang(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{
		"sv-SE,sv;q=0.9,en;q=0.8": "sv", "de-DE,en;q=0.5,sv;q=0.9": "sv", "fr, en-US;q=0.7": "en", "de": "", "": "",
		"sv;q=0,en": "en", "en;q=bad,sv;q=0.5": "en",
	} {
		if got := LangFromAcceptLanguage(in); got != want {
			t.Errorf("LangFromAcceptLanguage(%q) = %q", in, got)
		}
	}
	if ResolveLang("", "xx", "sv", "en") != "sv" || ResolveLang() != "en" || ResolveLang("fr") != "en" {
		t.Fatal("ResolveLang")
	}
	if T("sv", "Summary") != "Sammanfattning" || T("en", "Summary") != "Summary" || T("sv", "unknown text") != "unknown text" {
		t.Fatal("T")
	}
	if EnumLabel("sv", EnumEvalMethod, "inspection") != "Inspektion" || EnumLabel("en", EnumStatus, "in_progress") != "In progress" ||
		EnumLabel("sv", EnumStatus, "my_status") != "My status" || EnumLabel("sv", EnumObligation, "odd_one") != "ODD ONE" || EnumLabel("sv", EnumStatus, "") != "" {
		t.Fatal("EnumLabel")
	}
	if (loc{lang: Swedish}).pct(1, 0) != "–" || (loc{lang: English}).pct(1, 4) != "25.0%" || (loc{lang: Swedish}).pct(1, 4) != "25,0 %" {
		t.Fatal("pct")
	}
}
