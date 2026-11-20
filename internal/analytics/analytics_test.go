package analytics

import (
	"fmt"
	"strings"
	"testing"
)

func req(i int, obligation, text string) Requirement {
	return Requirement{ID: fmt.Sprint("r", i), Key: fmt.Sprint("P-REQ-", i), Title: fmt.Sprint("Requirement ", i), Description: text,
		Obligation: obligation, Status: "approved", Owner: "@anna", Criteria: 1, Tests: 1, Coverage: "passed", TestsReady: 1}
}

func find(rep Report, code string) *Finding {
	for i := range rep.Findings {
		if rep.Findings[i].Code == code {
			return &rep.Findings[i]
		}
	}
	return nil
}

func TestInconsistentKeyword(t *testing.T) {
	var reqs []Requirement
	for i := 1; i <= 19; i++ {
		reqs = append(reqs, req(i, "must", "The system MUST log event "+fmt.Sprint(i)+" within 2 seconds."))
	}
	reqs = append(reqs, req(20, "shall", "The system SHALL store the audit trail for 5 years."))
	rep := Analyze(Input{Requirements: reqs, Lang: "en"})
	f := find(rep, CodeInconsistentKeyword)
	if f == nil || f.Params["dominant"] != "MUST" || f.Params["dominantPercent"] != "95" || f.Params["keyword"] != "SHALL" || len(f.Items) != 1 || f.Items[0].Key != "P-REQ-20" {
		t.Fatalf("%+v", f)
	}
	if find(rep, CodeAllMandatory) == nil {
		t.Error("all mandatory not reported")
	}
	if rep.Families[0].Family != "mandatory" || rep.Families[0].Dominant != "MUST" || rep.Families[0].Total != 20 {
		t.Fatalf("%+v", rep.Families)
	}
	if rep.Obligations[0].Value != "must" || rep.Obligations[0].Percent != 95 {
		t.Fatalf("%+v", rep.Obligations)
	}
	if rep.Summary.Health != 95 || rep.Summary.WithFindings != 1 || rep.Summary.CoveragePercent != 100 {
		t.Fatalf("%+v", rep.Summary)
	}
	// about equal use: mixed, not inconsistent
	reqs[0].Obligation, reqs[0].Description = "shall", "The system SHALL do it."
	for i := 1; i < 10; i++ {
		reqs[i].Obligation, reqs[i].Description = "shall", "The system SHALL do "+fmt.Sprint(i)+"."
	}
	rep = Analyze(Input{Requirements: reqs})
	if find(rep, CodeInconsistentKeyword) != nil || find(rep, CodeMixedKeywords) == nil || !strings.Contains(find(rep, CodeMixedKeywords).Params["keywords"], "SHALL 55%") {
		t.Fatalf("%+v", rep.Findings)
	}
}

func TestObligationChecks(t *testing.T) {
	reqs := []Requirement{
		req(1, "", "The user can log in."),                                   // no obligation
		req(2, "must", "The user can log out."),                              // no keyword in the text
		req(3, "should", "The system MUST encrypt data at rest."),            // stored ≠ text
		req(4, "must", "The system MUST log in users and MAY cache tokens."), // two levels
		req(5, "describe", "This chapter describes the login."),
		req(6, "must", "The system MUST be fast."),
	}
	rep := Analyze(Input{Requirements: reqs})
	if f := find(rep, CodeObligationMissing); f == nil || f.Severity != Problem || f.Items[0].Key != "P-REQ-1" {
		t.Fatal(f)
	}
	if f := find(rep, CodeKeywordMissing); f == nil || len(f.Items) != 1 || f.Items[0].Key != "P-REQ-2" {
		t.Fatal(f)
	}
	if f := find(rep, CodeObligationMismatch); f == nil || f.Items[0].Note != "SHOULD ≠ MUST" {
		t.Fatal(f)
	}
	if f := find(rep, CodeMultipleLevels); f == nil || f.Items[0].Note != "MUST, MAY" {
		t.Fatal(f)
	}
	if f := find(rep, CodeLanguageRule); f == nil {
		t.Fatal("language rules")
	}
	if rep.Findings[0].Severity != Problem {
		t.Fatal("problems first")
	}
	var none bool
	for _, o := range rep.Obligations {
		if o.Value == "none" && o.Count == 1 {
			none = true
		}
	}
	if !none {
		t.Fatalf("%+v", rep.Obligations)
	}
}

func TestSpread(t *testing.T) {
	var reqs []Requirement
	for i := 1; i <= 6; i++ {
		reqs = append(reqs, req(i, "may", "The user MAY export report "+fmt.Sprint(i)+"."))
	}
	reqs = append(reqs, req(7, "should", "The system SHOULD respond within 1 s."))
	rep := Analyze(Input{Requirements: reqs})
	if find(rep, CodeNoMandatory) == nil || find(rep, CodeManyOptional) == nil || len(find(rep, CodeManyOptional).Items) != 6 {
		t.Fatalf("%+v", rep.Findings)
	}
	var shoulds []Requirement
	for i := 1; i <= 6; i++ {
		shoulds = append(shoulds, req(i, "should", "It SHOULD do "+fmt.Sprint(i)+"."))
	}
	shoulds = append(shoulds, req(7, "describe", "Background."), req(8, "describe", "More."), req(9, "describe", "Even more."), req(10, "describe", "Last."))
	for i := range shoulds {
		shoulds[i].Status = "draft"
	}
	rep = Analyze(Input{Requirements: shoulds})
	for _, c := range []string{CodeManyRecommendations, CodeManyDescriptive, CodeManyDrafts} {
		if find(rep, c) == nil {
			t.Errorf("%s missing", c)
		}
	}
	// too few requirements: no spread comments
	if rep := Analyze(Input{Requirements: reqs[:3]}); find(rep, CodeManyOptional) != nil {
		t.Error("spread with too few requirements")
	}
}

func TestCoverageAndQuality(t *testing.T) {
	reqs := []Requirement{
		req(1, "must", "The system MUST lock accounts after 5 failed logins."),
		req(2, "may", "The user MAY choose a theme."),
		req(3, "must", "The system MUST log out idle users after 15 minutes."),
		req(4, "must", "The system MUST log out idle users after 15 minutes."),
		req(5, "must", ""),
		req(6, "must", "The system MUST be user friendly and fast TBD."),
		req(7, "must", "Obsolete one MUST not count."),
	}
	reqs[0].Tests, reqs[0].Coverage = 0, "none"
	reqs[1].Tests, reqs[1].Coverage = 0, "none"
	reqs[2].Coverage = "failed"
	reqs[3].Coverage, reqs[3].Title, reqs[3].Criteria, reqs[3].Owner = "not_run", "requirement 3!", 0, ""
	reqs[2].Title = "Requirement  3"
	reqs[4].Conflicts = []string{"P-REQ-6"}
	reqs[5].TestsReady = 0
	reqs[6].Status, reqs[6].Tests, reqs[6].Coverage = "obsolete", 0, "none"
	tests := []Test{{ID: "t1", Key: "P-TC-1", Title: "Linked", Requirements: 1}, {ID: "t2", Key: "P-TC-2", Title: "Orphan"}, {ID: "t3", Key: "P-TC-3", Status: "deprecated"}}
	rep := Analyze(Input{Requirements: reqs, Tests: tests})
	check := func(code, key string) {
		t.Helper()
		f := find(rep, code)
		if f == nil {
			t.Fatalf("%s missing", code)
		}
		for _, it := range f.Items {
			if it.Key == key {
				return
			}
		}
		t.Fatalf("%s: %s not in %+v", code, key, f.Items)
	}
	check(CodeNoTestsMandatory, "P-REQ-1")
	check(CodeNoTests, "P-REQ-2")
	check(CodeTestsFailing, "P-REQ-3")
	check(CodeTestsNotRun, "P-REQ-4")
	check(CodeNoCriteria, "P-REQ-4")
	check(CodeDuplicateTitles, "P-REQ-3")
	check(CodeEmptyDescription, "P-REQ-5")
	check(CodeConflicts, "P-REQ-5")
	check(CodeLowQuality, "P-REQ-6")
	check(CodeTestsNotReady, "P-REQ-6")
	check(CodeOrphanTests, "P-TC-2")
	check(CodeNoOwner, "P-REQ-4")
	if f := find(rep, CodeNoTestsMandatory); len(f.Items) != 1 {
		t.Fatal("obsolete requirements are not checked for tests", f.Items)
	}
	if f := find(rep, CodeOrphanTests); len(f.Items) != 1 {
		t.Fatal(f.Items)
	}
	if rep.Summary.Covered != 4 || rep.Summary.CoveragePercent != 67 || rep.Summary.Tests != 3 {
		t.Fatalf("%+v", rep.Summary)
	}
	if rep.Coverage[0].Count < 1 || len(rep.Statuses) != 2 {
		t.Fatalf("%+v %+v", rep.Coverage, rep.Statuses)
	}
	// without permission to read tests, test findings are skipped
	if rep := Analyze(Input{Requirements: reqs}); find(rep, CodeOrphanTests) != nil || find(rep, CodeTestsNotReady) != nil {
		t.Fatal("test findings without tests")
	}
}

func TestEmptyAndLimits(t *testing.T) {
	rep := Analyze(Input{})
	if rep.Summary.Health != 100 || len(rep.Findings) != 0 || rep.Obligations == nil || rep.Families == nil {
		t.Fatalf("%+v", rep)
	}
	old := MaxItems
	MaxItems = 2
	defer func() { MaxItems = old }()
	var reqs []Requirement
	for i := 1; i <= 5; i++ {
		reqs = append(reqs, req(i, "", "Text."))
	}
	f := find(Analyze(Input{Requirements: reqs}), CodeObligationMissing)
	if f.Total != 5 || len(f.Items) != 2 {
		t.Fatal(f)
	}
	if normalizeTitle("  Log-in, now! ") != "log in now" || fmtPct(12.5) != "12.5" || pct(1, 0) != 0 {
		t.Fatal("helpers")
	}
	if family("weird") != "" || defaultKeyword("describe") != "" || defaultKeyword("must_not") != "MUST NOT" {
		t.Fatal("families")
	}
}

func TestLanguagesAndRFCCasing(t *testing.T) {
	reqs := []Requirement{
		req(1, "must", "The system MUST log every sign-in of the user."),
		req(2, "must", "The system MUST lock the account after five failed attempts."),
		req(3, "must", "The user must be able to reset the password by e-mail."),                        // lowercase only
		req(4, "should", "The system SHOULD respond within 2 seconds, and it should cache the result."), // mixed casing
		req(5, "shall", "Systemet SKA spara loggar i fem år och de ska kunna exporteras."),              // Swedish
		req(6, "must", "Systemet MUST kryptera all data som lagras på disk."),                           // English keyword in Swedish
		req(7, "must", "The system MUST show a receipt. Kvittot ska skickas till kunden med e-post."),   // mixed in one text
		req(8, "will", "The vendor WILL deliver the test data for the system."),
	}
	reqs[4].Title, reqs[5].Title = "Loggar för revision", "Kryptering av data"
	rep := Analyze(Input{Requirements: reqs})
	check := func(code, key, note string) *Finding {
		t.Helper()
		f := find(rep, code)
		if f == nil {
			t.Fatalf("%s missing: %+v", code, rep.Findings)
		}
		for _, it := range f.Items {
			if it.Key == key && (note == "" || it.Note == note) {
				return f
			}
		}
		t.Fatalf("%s: %s/%s not in %+v", code, key, note, f.Items)
		return nil
	}
	check(CodeRFCNotNormative, "P-REQ-3", "must")
	check(CodeRFCCasing, "P-REQ-4", "should")
	if f := find(rep, CodeRFCCasing); len(f.Items) != 1 {
		t.Fatal("only requirements with both forms", f.Items)
	}
	check(CodeEnglishKeywords, "P-REQ-6", "MUST")
	check(CodeMixedLanguageText, "P-REQ-7", "en + sv")
	check(CodeNonRFCKeyword, "P-REQ-8", "WILL")
	f := check(CodeMixedLanguages, "P-REQ-5", "sv")
	if f.Params["dominant"] != "en" || !strings.Contains(f.Params["languages"], "sv") {
		t.Fatal(f.Params)
	}
	if len(rep.Languages) != 2 || rep.Languages[0].Value != "en" {
		t.Fatal(rep.Languages)
	}
	// casing is reported on its own, not again as a language rule
	for _, x := range rep.Findings {
		if x.Code == CodeLanguageRule && x.Params["rule"] == "lowercase-keyword" {
			t.Fatal("lowercase keywords counted twice")
		}
	}
	// one language: no language findings
	rep = Analyze(Input{Requirements: reqs[:2]})
	if find(rep, CodeMixedLanguages) != nil || find(rep, CodeMixedLanguageText) != nil || len(rep.Languages) != 1 {
		t.Fatalf("%+v", rep.Findings)
	}
	if got := textLanguages("Login", ""); len(got) != 0 {
		t.Fatal(got)
	}
	if got := uniq([]string{"a", "b", "a"}); len(got) != 2 {
		t.Fatal(got)
	}
}

func TestSuspectLinksFinding(t *testing.T) {
	r := req(1, "must", "The system MUST log every sign-in.")
	r.Suspect = 2
	f := find(Analyze(Input{Requirements: []Requirement{r}}), CodeSuspectLinks)
	if f == nil || f.Items[0].Note != "2" || f.Severity != Warning {
		t.Fatal(f)
	}
}
