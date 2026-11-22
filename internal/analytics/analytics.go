// Package analytics analyses the requirements (and tests) of a project as
// a whole: the spread of obligation levels, inconsistent use of keywords
// that mean the same (MUST vs SHALL), requirements without an obligation,
// without tests or acceptance criteria, failing coverage, tests that verify
// nothing, duplicates, conflicts and the language findings of package
// reqlang summed up per rule.
//
// Findings carry a code and parameters, not text: clients phrase them in
// their language.
package analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/rom/qtestframe/internal/reqlang"
)

// Severities of findings.
const (
	Problem = "problem"
	Warning = "warning"
	Info    = "info"
)

// Categories of findings.
const (
	CatObligations = "obligations"
	CatConsistency = "consistency"
	CatCoverage    = "coverage"
	CatQuality     = "quality"
	CatLanguage    = "language"
	CatTests       = "tests"
)

// Finding codes.
const (
	CodeObligationMissing   = "obligation_missing"
	CodeKeywordMissing      = "keyword_missing"
	CodeObligationMismatch  = "obligation_mismatch"
	CodeAllMandatory        = "spread_all_mandatory"
	CodeManyOptional        = "spread_many_optional"
	CodeNoMandatory         = "spread_no_mandatory"
	CodeManyRecommendations = "spread_many_should"
	CodeManyDescriptive     = "spread_many_descriptive"
	CodeInconsistentKeyword = "inconsistent_keyword"
	CodeMixedKeywords       = "mixed_keywords"
	CodeMultipleLevels      = "multiple_levels"
	CodeNoTestsMandatory    = "no_tests_mandatory"
	CodeNoTests             = "no_tests"
	CodeTestsFailing        = "tests_failing"
	CodeTestsNotRun         = "tests_not_run"
	CodeNoCriteria          = "no_criteria"
	CodeEmptyDescription    = "empty_description"
	CodeDuplicateTitles     = "duplicate_titles"
	CodeConflicts           = "conflicts"
	CodeLowQuality          = "low_quality"
	CodeLanguageRule        = "language_rule"
	CodeOrphanTests         = "orphan_tests"
	CodeTestsNotReady       = "tests_not_ready"
	CodeManyDrafts          = "many_drafts"
	CodeNoOwner             = "no_owner"
	CodeSuspectLinks        = "suspect_links"
	CodeMixedLanguages      = "mixed_languages"
	CodeMixedLanguageText   = "mixed_language_text"
	CodeEnglishKeywords     = "english_keywords"
	CodeRFCCasing           = "rfc_casing"
	CodeRFCNotNormative     = "rfc_not_normative"
	CodeNonRFCKeyword       = "non_rfc_keyword"
	// verification correlations (package verification)
	CodeCriteriaNoEvidence       = "criteria_no_evidence"
	CodeVerifiedParentUnverified = "verified_parent_unverified_child"
	CodeStatusAheadOfEvidence    = "status_ahead_of_evidence"
	// CodeWaived: requirements without tests or with failing or unrun
	// tests that an approved waiver accepts (counted here, not as gaps).
	CodeWaived = "waived"
)

// Thresholds (variables so tests and callers can tune them).
var (
	// DominantShare: a keyword used at least this share of its family is
	// the convention; the others in the family are inconsistent.
	DominantShare = 0.75
	// MinForSpread: spread comments need at least this many requirements.
	MinForSpread = 5
	// MaxItems bounds the items listed per finding.
	MaxItems = 200
	// LowScore: the language analysis score below which a requirement is flagged.
	LowScore = 60
	// LanguageEvidence: marker words a text needs before its language counts.
	LanguageEvidence = 2
)

// Requirement is the analytics view of a requirement.
type Requirement struct {
	ID, Key, Title, Description string
	// Obligation as stored (must, shall, …, describe, or "").
	Obligation string
	Status     string
	Owner      string
	Criteria   int
	// Tests: number of linked tests; Coverage: none | not_run | passed | failed | partial.
	Tests    int
	Coverage string
	// TestsReady: number of linked tests whose status is ready (-1 unknown).
	TestsReady int
	// Conflicts: keys of requirements it conflicts with.
	Conflicts []string
	// Suspect: number of its links to tests that are suspect.
	Suspect int
	// Verification: the rolled-up verification state ("" unknown);
	// CriteriaNoEvidence: criteria without any evidence; UnverifiedChildren:
	// keys of its children (refines) that are not verified while its
	// status is verified; StatusAhead: the status is verified but the
	// evidence is not.
	Verification       string
	CriteriaNoEvidence int
	UnverifiedChildren []string
	StatusAhead        bool
	// Waived: an approved, unexpired project-wide waiver accepts it.
	Waived bool
}

// Test is the analytics view of a test.
type Test struct {
	ID, Key, Title, Status, LastResult string
	Requirements                       int
}

// Input is a project's data.
type Input struct {
	Requirements []Requirement
	// Tests is nil when the caller may not read tests (test findings are skipped).
	Tests []Test
	// Lang is the analysis language ("" or "auto" detects per requirement).
	Lang string
}

// Item is a requirement or test a finding refers to.
type Item struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	// Note gives item-specific detail (the keyword used, the score …).
	Note string `json:"note,omitempty"`
}

// Finding is one observation.
type Finding struct {
	Code     string            `json:"code"`
	Severity string            `json:"severity"`
	Category string            `json:"category"`
	Params   map[string]string `json:"params"`
	Items    []Item            `json:"items"`
	// Total is the number of items before MaxItems.
	Total int `json:"total"`
}

// Count is how many requirements have a value.
type Count struct {
	Value   string  `json:"value"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// Family groups the keywords that mean the same, with their use.
type Family struct {
	Family   string  `json:"family"`
	Keywords []Count `json:"keywords"`
	Total    int     `json:"total"`
	// Dominant is the keyword used most ("" when the family is not used).
	Dominant string `json:"dominant"`
}

// Summary holds the key figures.
type Summary struct {
	Requirements int `json:"requirements"`
	Tests        int `json:"tests"`
	// WithFindings: requirements with at least one problem or warning.
	WithFindings int `json:"with_findings"`
	// Health is the share (0–100) of requirements without problems or warnings.
	Health   int `json:"health"`
	Problems int `json:"problems"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
	// Covered: requirements with at least one linked test.
	Covered int `json:"covered"`
	// Coverage: percent of requirements (excluding rejected/obsolete) with tests.
	CoveragePercent int `json:"coverage_percent"`
	// AverageScore of the language analysis (0–100).
	AverageScore int `json:"average_score"`
	// Waived: requirements whose missing or failing verification a waiver
	// accepts (shown separately, not as coverage gaps).
	Waived int `json:"waived"`
}

// Report is the result of Analyze.
type Report struct {
	Summary     Summary  `json:"summary"`
	Obligations []Count  `json:"obligations"`
	Families    []Family `json:"families"`
	Statuses    []Count  `json:"statuses"`
	Coverage    []Count  `json:"coverage"`
	// Verification: rolled-up verification states (when known).
	Verification []Count `json:"verification"`
	// Languages of the requirement texts (only texts whose language is clear).
	Languages []Count   `json:"languages"`
	Findings  []Finding `json:"findings"`
}

// family of an obligation level: keywords in one family mean the same.
func family(level string) string {
	switch level {
	case reqlang.LevelMust, reqlang.LevelShall:
		return "mandatory"
	case reqlang.LevelMustNot, reqlang.LevelShallNot:
		return "prohibition"
	case reqlang.LevelShould:
		return "recommendation"
	case reqlang.LevelShouldNot:
		return "discouragement"
	case reqlang.LevelMay, reqlang.LevelOptional:
		return "permission"
	case reqlang.LevelWill:
		return "declaration"
	case reqlang.LevelDescribe:
		return "descriptive"
	}
	return ""
}

// Families in display order.
var Families = []string{"mandatory", "prohibition", "recommendation", "discouragement", "permission", "declaration"}

// defaultKeyword is the keyword of a stored level when the text has none.
func defaultKeyword(level string) string {
	if level == "" || level == reqlang.LevelDescribe {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(level, "_", " "))
}

// inactive requirements are left out of coverage checks.
func inactive(status string) bool { return status == "rejected" || status == "obsolete" }

type builder struct {
	findings []Finding
	// flagged: requirement id → worst severity seen
	flagged map[string]string
}

func (b *builder) add(code, sev, cat string, params map[string]string, items []Item) {
	if len(items) == 0 && params["force"] == "" {
		return
	}
	delete(params, "force")
	if params == nil {
		params = map[string]string{}
	}
	f := Finding{Code: code, Severity: sev, Category: cat, Params: params, Total: len(items)}
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	f.Items = append([]Item{}, items...)
	b.findings = append(b.findings, f)
	if sev == Problem || sev == Warning {
		for _, it := range items {
			if it.Kind == "requirement" {
				if b.flagged[it.ID] != Problem {
					b.flagged[it.ID] = sev
				}
			}
		}
	}
}

func reqItem(r *Requirement, note string) Item {
	return Item{Kind: "requirement", ID: r.ID, Key: r.Key, Title: r.Title, Note: note}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)*1000/float64(total)) / 10
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func fmtPct(p float64) string {
	if p == math.Trunc(p) {
		return fmt.Sprintf("%.0f", p)
	}
	return fmt.Sprintf("%.1f", p)
}

// Analyze builds the report.
func Analyze(in Input) Report {
	b := &builder{flagged: map[string]string{}}
	reqs := in.Requirements
	n := len(reqs)
	rep := Report{Summary: Summary{Requirements: n, Tests: len(in.Tests)}}

	// per requirement: language analysis and the keyword used
	type info struct {
		a       reqlang.Analysis
		text    string // the analysed text
		keyword string // canonical keyword of the stored level's family (text first)
		level   string
		lang    string // language of title and description ("" when unclear)
	}
	infos := make([]info, n)
	scoreSum := 0
	for i := range reqs {
		r := &reqs[i]
		text := strings.TrimSpace(r.Description)
		if text == "" {
			text = r.Title
		}
		a := reqlang.AnalyzeLang(text, in.Lang)
		level := r.Obligation
		kw := ""
		fam := family(level)
		for _, k := range a.Keywords {
			if family(k.Level) == fam && fam != "" {
				kw = strings.ToUpper(k.Keyword)
				break
			}
		}
		if kw == "" {
			kw = defaultKeyword(level)
		}
		lang, _ := reqlang.DetectConfident(r.Title+". "+r.Description, LanguageEvidence)
		infos[i] = info{a: a, text: text, keyword: kw, level: level, lang: lang}
		scoreSum += a.Score
	}
	if n > 0 {
		rep.Summary.AverageScore = int(math.Round(float64(scoreSum) / float64(n)))
	}

	// ---- obligation distribution
	levelCount := map[string]int{}
	for _, inf := range infos {
		l := inf.level
		if l == "" {
			l = "none"
		}
		levelCount[l]++
	}
	order := append(append([]string{}, reqlang.Levels...), "none")
	for _, l := range order {
		if c := levelCount[l]; c > 0 {
			rep.Obligations = append(rep.Obligations, Count{Value: l, Count: c, Percent: pct(c, n)})
		}
	}

	// ---- keyword families and consistency
	famKw := map[string]map[string][]int{} // family → keyword → requirement indexes
	for i, inf := range infos {
		f := family(inf.level)
		if f == "" || f == "descriptive" || inf.keyword == "" {
			continue
		}
		if famKw[f] == nil {
			famKw[f] = map[string][]int{}
		}
		famKw[f][inf.keyword] = append(famKw[f][inf.keyword], i)
	}
	for _, f := range Families {
		kws := famKw[f]
		if len(kws) == 0 {
			continue
		}
		fam := Family{Family: f}
		for k, idx := range kws {
			fam.Total += len(idx)
			fam.Keywords = append(fam.Keywords, Count{Value: k, Count: len(idx)})
		}
		sort.Slice(fam.Keywords, func(i, j int) bool {
			if fam.Keywords[i].Count != fam.Keywords[j].Count {
				return fam.Keywords[i].Count > fam.Keywords[j].Count
			}
			return fam.Keywords[i].Value < fam.Keywords[j].Value
		})
		for i := range fam.Keywords {
			fam.Keywords[i].Percent = pct(fam.Keywords[i].Count, fam.Total)
		}
		fam.Dominant = fam.Keywords[0].Value
		rep.Families = append(rep.Families, fam)
		if len(fam.Keywords) < 2 {
			continue
		}
		top := fam.Keywords[0]
		share := float64(top.Count) / float64(fam.Total)
		if share >= DominantShare {
			for _, minor := range fam.Keywords[1:] {
				var items []Item
				for _, i := range kws[minor.Value] {
					items = append(items, reqItem(&reqs[i], minor.Value))
				}
				b.add(CodeInconsistentKeyword, Warning, CatConsistency, map[string]string{
					"family": f, "dominant": top.Value, "dominantPercent": fmtPct(top.Percent), "dominantCount": itoa(top.Count),
					"keyword": minor.Value, "count": itoa(minor.Count), "percent": fmtPct(minor.Percent),
				}, items)
			}
		} else {
			var names []string
			for _, k := range fam.Keywords {
				names = append(names, fmt.Sprintf("%s %s%%", k.Value, fmtPct(k.Percent)))
			}
			b.add(CodeMixedKeywords, Info, CatConsistency, map[string]string{"family": f, "keywords": strings.Join(names, ", "), "force": "1"}, nil)
		}
	}

	// ---- per requirement checks
	var missing, kwMissing, mismatch, multi, noTestsM, noTests, failing, notRun, noCrit, empty, low, noOwner, conflicts, notReady, waived []Item
	var mixedText, englishKw, casing, notNormative, nonRFC, suspect []Item
	var critNoEvidence, parentUnverified, statusAhead []Item
	byRule := map[string][]Item{}
	covered, active, drafts := 0, 0, 0
	for i := range reqs {
		r := &reqs[i]
		inf := infos[i]
		a := inf.a
		if r.Status == "draft" {
			drafts++
		}
		if r.Obligation == "" {
			missing = append(missing, reqItem(r, ""))
		} else if r.Obligation != reqlang.LevelDescribe && len(a.Keywords) == 0 {
			kwMissing = append(kwMissing, reqItem(r, defaultKeyword(r.Obligation)))
		}
		if r.Obligation != "" && a.Obligation != "" && a.Obligation != reqlang.LevelDescribe && family(a.Obligation) != family(r.Obligation) {
			mismatch = append(mismatch, reqItem(r, defaultKeyword(r.Obligation)+" ≠ "+defaultKeyword(a.Obligation)))
		}
		levels := map[string]bool{}
		var used []string
		for _, k := range a.Keywords {
			f := family(k.Level)
			if !levels[f] {
				levels[f] = true
				used = append(used, strings.ToUpper(k.Keyword))
			}
		}
		if len(levels) > 1 {
			multi = append(multi, reqItem(r, strings.Join(used, ", ")))
		}
		if strings.TrimSpace(r.Description) == "" {
			empty = append(empty, reqItem(r, ""))
		}
		// languages within one requirement: the title and every sentence
		if langs := textLanguages(r.Title, r.Description); len(langs) > 1 {
			mixedText = append(mixedText, reqItem(r, strings.Join(langs, " + ")))
		}
		// RFC 2119 / 8174: keywords in capitals, English keywords in other languages
		var lower, english []string
		rfc, upper := 0, 0
		for _, k := range a.Keywords {
			if !reqlang.IsRFCKeyword(k.Keyword) {
				if k.Level == reqlang.LevelWill && a.Language == "en" && inf.text[k.Offset:k.Offset+k.Length] == "WILL" {
					nonRFC = append(nonRFC, reqItem(r, "WILL"))
				}
				continue
			}
			rfc++
			word := inf.text[k.Offset : k.Offset+k.Length]
			if word == strings.ToUpper(word) {
				upper++
				if inf.lang != "" && inf.lang != "en" {
					english = append(english, word)
				}
			} else {
				lower = append(lower, word)
			}
		}
		if len(lower) > 0 {
			if upper == 0 {
				notNormative = append(notNormative, reqItem(r, strings.Join(uniq(lower), ", ")))
			} else {
				casing = append(casing, reqItem(r, strings.Join(uniq(lower), ", ")))
			}
		}
		if len(english) > 0 {
			englishKw = append(englishKw, reqItem(r, strings.Join(uniq(english), ", ")))
		}
		if a.Score < LowScore {
			low = append(low, reqItem(r, itoa(a.Score)))
		}
		seenRule := map[string]bool{}
		for _, is := range a.Issues {
			if is.Rule == reqlang.RuleMissingKeyword || is.Rule == reqlang.RuleMultipleObligations || is.Rule == reqlang.RuleLowercaseKeyword || seenRule[is.Rule] {
				continue // reported above
			}
			seenRule[is.Rule] = true
			byRule[is.Rule] = append(byRule[is.Rule], reqItem(r, is.Params["term"]))
		}
		if strings.TrimSpace(r.Owner) == "" {
			noOwner = append(noOwner, reqItem(r, ""))
		}
		if r.Suspect > 0 {
			suspect = append(suspect, reqItem(r, itoa(r.Suspect)))
		}
		if len(r.Conflicts) > 0 {
			conflicts = append(conflicts, reqItem(r, strings.Join(r.Conflicts, ", ")))
		}
		if inactive(r.Status) {
			continue
		}
		active++
		if len(r.UnverifiedChildren) > 0 {
			parentUnverified = append(parentUnverified, reqItem(r, strings.Join(r.UnverifiedChildren, ", ")))
		}
		if r.StatusAhead {
			statusAhead = append(statusAhead, reqItem(r, r.Verification))
		}
		if r.CriteriaNoEvidence > 0 && (r.Tests > 0 || r.Verification != "none") {
			critNoEvidence = append(critNoEvidence, reqItem(r, itoa(r.CriteriaNoEvidence)))
		}
		if r.Waived && (r.Tests == 0 || r.Coverage == "failed" || r.Coverage == "partial" || r.Coverage == "not_run") {
			waived = append(waived, reqItem(r, ""))
			if r.Tests > 0 {
				covered++
			}
			if r.Criteria == 0 && r.Obligation != reqlang.LevelDescribe {
				noCrit = append(noCrit, reqItem(r, ""))
			}
			continue
		}
		if r.Tests > 0 || (r.Verification != "" && r.Verification != "none") {
			covered++
		} else if family(r.Obligation) == "mandatory" || family(r.Obligation) == "prohibition" {
			noTestsM = append(noTestsM, reqItem(r, defaultKeyword(r.Obligation)))
		} else if r.Obligation != reqlang.LevelDescribe {
			noTests = append(noTests, reqItem(r, defaultKeyword(r.Obligation)))
		}
		switch r.Coverage {
		case "failed", "partial":
			failing = append(failing, reqItem(r, r.Coverage))
		case "not_run":
			notRun = append(notRun, reqItem(r, ""))
		}
		if r.Criteria == 0 && r.Obligation != reqlang.LevelDescribe {
			noCrit = append(noCrit, reqItem(r, ""))
		}
		if r.Tests > 0 && r.TestsReady == 0 && (r.Status == "approved" || r.Status == "implemented" || r.Status == "verified") {
			notReady = append(notReady, reqItem(r, r.Status))
		}
	}
	rep.Summary.Covered = covered
	rep.Summary.Waived = len(waived)
	if active > 0 {
		rep.Summary.CoveragePercent = int(math.Round(float64(covered) * 100 / float64(active)))
	}

	b.add(CodeObligationMissing, Problem, CatObligations, map[string]string{"count": itoa(len(missing))}, missing)
	b.add(CodeKeywordMissing, Warning, CatObligations, map[string]string{"count": itoa(len(kwMissing))}, kwMissing)
	b.add(CodeObligationMismatch, Warning, CatObligations, map[string]string{"count": itoa(len(mismatch))}, mismatch)
	b.add(CodeMultipleLevels, Info, CatObligations, map[string]string{"count": itoa(len(multi))}, multi)
	b.add(CodeRFCNotNormative, Warning, CatObligations, map[string]string{"count": itoa(len(notNormative))}, notNormative)
	b.add(CodeRFCCasing, Warning, CatObligations, map[string]string{"count": itoa(len(casing))}, casing)
	b.add(CodeNonRFCKeyword, Info, CatObligations, map[string]string{"count": itoa(len(nonRFC)), "keyword": "WILL"}, nonRFC)

	// ---- languages
	langCount := map[string][]int{}
	known := 0
	for i, inf := range infos {
		if inf.lang != "" {
			langCount[inf.lang] = append(langCount[inf.lang], i)
			known++
		}
	}
	for l, idx := range langCount {
		rep.Languages = append(rep.Languages, Count{Value: l, Count: len(idx), Percent: pct(len(idx), known)})
	}
	sort.Slice(rep.Languages, func(i, j int) bool {
		if rep.Languages[i].Count != rep.Languages[j].Count {
			return rep.Languages[i].Count > rep.Languages[j].Count
		}
		return rep.Languages[i].Value < rep.Languages[j].Value
	})
	if len(rep.Languages) > 1 {
		top := rep.Languages[0]
		var names []string
		for _, l := range rep.Languages {
			names = append(names, fmt.Sprintf("%s %s%%", l.Value, fmtPct(l.Percent)))
		}
		var items []Item
		for _, l := range rep.Languages[1:] {
			for _, i := range langCount[l.Value] {
				items = append(items, reqItem(&reqs[i], l.Value))
			}
		}
		b.add(CodeMixedLanguages, Warning, CatConsistency, map[string]string{"dominant": top.Value, "dominantPercent": fmtPct(top.Percent),
			"languages": strings.Join(names, ", "), "count": itoa(len(items))}, items)
	}
	b.add(CodeMixedLanguageText, Warning, CatConsistency, map[string]string{"count": itoa(len(mixedText))}, mixedText)
	b.add(CodeEnglishKeywords, Info, CatConsistency, map[string]string{"count": itoa(len(englishKw))}, englishKw)

	// ---- spread of obligation levels
	if n >= MinForSpread {
		famCount := map[string]int{}
		for _, inf := range infos {
			famCount[family(inf.level)]++
		}
		mandatory := famCount["mandatory"] + famCount["prohibition"]
		optional := famCount["permission"]
		should := famCount["recommendation"] + famCount["discouragement"]
		descriptive := famCount["descriptive"]
		p := func(c int) string { return fmtPct(pct(c, n)) }
		params := func(c int) map[string]string {
			return map[string]string{"count": itoa(c), "total": itoa(n), "percent": p(c), "mandatory": p(mandatory), "should": p(should), "optional": p(optional), "force": "1"}
		}
		switch {
		case mandatory == 0:
			b.add(CodeNoMandatory, Warning, CatObligations, params(0), nil)
		case float64(mandatory)/float64(n) >= 0.9:
			b.add(CodeAllMandatory, Info, CatObligations, params(mandatory), nil)
		}
		if float64(optional)/float64(n) > 0.3 {
			b.add(CodeManyOptional, Warning, CatObligations, params(optional), levelItems(reqs, "permission"))
		}
		if float64(should)/float64(n) > 0.5 {
			b.add(CodeManyRecommendations, Info, CatObligations, params(should), nil)
		}
		if float64(descriptive)/float64(n) > 0.3 {
			b.add(CodeManyDescriptive, Info, CatObligations, params(descriptive), levelItems(reqs, "descriptive"))
		}
		if float64(drafts)/float64(n) > 0.5 {
			b.add(CodeManyDrafts, Info, CatQuality, map[string]string{"count": itoa(drafts), "total": itoa(n), "percent": p(drafts), "force": "1"}, nil)
		}
	}

	// ---- coverage
	b.add(CodeNoTestsMandatory, Problem, CatCoverage, map[string]string{"count": itoa(len(noTestsM))}, noTestsM)
	b.add(CodeNoTests, Warning, CatCoverage, map[string]string{"count": itoa(len(noTests))}, noTests)
	b.add(CodeTestsFailing, Problem, CatCoverage, map[string]string{"count": itoa(len(failing))}, failing)
	b.add(CodeSuspectLinks, Warning, CatCoverage, map[string]string{"count": itoa(len(suspect))}, suspect)
	b.add(CodeTestsNotRun, Info, CatCoverage, map[string]string{"count": itoa(len(notRun))}, notRun)
	b.add(CodeNoCriteria, Warning, CatCoverage, map[string]string{"count": itoa(len(noCrit))}, noCrit)
	b.add(CodeVerifiedParentUnverified, Problem, CatCoverage, map[string]string{"count": itoa(len(parentUnverified))}, parentUnverified)
	b.add(CodeStatusAheadOfEvidence, Warning, CatCoverage, map[string]string{"count": itoa(len(statusAhead))}, statusAhead)
	b.add(CodeCriteriaNoEvidence, Warning, CatCoverage, map[string]string{"count": itoa(len(critNoEvidence))}, critNoEvidence)
	b.add(CodeWaived, Info, CatCoverage, map[string]string{"count": itoa(len(waived))}, waived)
	if in.Tests != nil {
		b.add(CodeTestsNotReady, Info, CatTests, map[string]string{"count": itoa(len(notReady))}, notReady)
		var orphans []Item
		for _, t := range in.Tests {
			if t.Requirements == 0 && t.Status != "deprecated" {
				orphans = append(orphans, Item{Kind: "test", ID: t.ID, Key: t.Key, Title: t.Title})
			}
		}
		b.add(CodeOrphanTests, Warning, CatTests, map[string]string{"count": itoa(len(orphans))}, orphans)
	}

	// ---- quality
	b.add(CodeEmptyDescription, Problem, CatQuality, map[string]string{"count": itoa(len(empty))}, empty)
	b.add(CodeConflicts, Warning, CatQuality, map[string]string{"count": itoa(len(conflicts))}, conflicts)
	b.add(CodeLowQuality, Warning, CatQuality, map[string]string{"count": itoa(len(low)), "threshold": itoa(LowScore)}, low)
	b.add(CodeNoOwner, Info, CatQuality, map[string]string{"count": itoa(len(noOwner))}, noOwner)
	dups := map[string][]int{}
	for i := range reqs {
		if k := normalizeTitle(reqs[i].Title); k != "" {
			dups[k] = append(dups[k], i)
		}
	}
	var dupKeys []string
	for k, idx := range dups {
		if len(idx) > 1 {
			dupKeys = append(dupKeys, k)
		}
	}
	sort.Strings(dupKeys)
	for _, k := range dupKeys {
		var items []Item
		for _, i := range dups[k] {
			items = append(items, reqItem(&reqs[i], ""))
		}
		b.add(CodeDuplicateTitles, Warning, CatQuality, map[string]string{"title": reqs[dups[k][0]].Title, "count": itoa(len(items))}, items)
	}

	// ---- language findings per rule
	rules := make([]string, 0, len(byRule))
	for r := range byRule {
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool {
		if len(byRule[rules[i]]) != len(byRule[rules[j]]) {
			return len(byRule[rules[i]]) > len(byRule[rules[j]])
		}
		return rules[i] < rules[j]
	})
	for _, rule := range rules {
		sev := Info
		switch rule {
		case reqlang.RuleTBD, reqlang.RuleAmbiguousTerm, reqlang.RuleEscapeClause, reqlang.RuleAmbiguousNegation, reqlang.RuleLowercaseKeyword:
			sev = Warning
		}
		b.add(CodeLanguageRule, sev, CatLanguage, map[string]string{"rule": rule, "count": itoa(len(byRule[rule]))}, byRule[rule])
	}

	// ---- order: problems, warnings, infos (stable within)
	rank := map[string]int{Problem: 0, Warning: 1, Info: 2}
	sort.SliceStable(b.findings, func(i, j int) bool { return rank[b.findings[i].Severity] < rank[b.findings[j].Severity] })
	rep.Findings = b.findings
	if rep.Findings == nil {
		rep.Findings = []Finding{}
	}
	for _, f := range rep.Findings {
		switch f.Severity {
		case Problem:
			rep.Summary.Problems++
		case Warning:
			rep.Summary.Warnings++
		default:
			rep.Summary.Infos++
		}
	}
	rep.Summary.WithFindings = len(b.flagged)
	if n > 0 {
		rep.Summary.Health = int(math.Round(float64(n-len(b.flagged)) * 100 / float64(n)))
	} else {
		rep.Summary.Health = 100
	}

	// ---- statuses and coverage counts
	rep.Statuses = counts(reqs, func(r *Requirement) string { return r.Status })
	rep.Coverage = counts(reqs, func(r *Requirement) string {
		if r.Coverage == "" {
			return "none"
		}
		return r.Coverage
	})
	rep.Verification = []Count{}
	if n > 0 && reqs[0].Verification != "" {
		rep.Verification = counts(reqs, func(r *Requirement) string { return r.Verification })
	}
	if rep.Obligations == nil {
		rep.Obligations = []Count{}
	}
	if rep.Families == nil {
		rep.Families = []Family{}
	}
	if rep.Languages == nil {
		rep.Languages = []Count{}
	}
	return rep
}

func levelItems(reqs []Requirement, fam string) []Item {
	var out []Item
	for i := range reqs {
		if family(reqs[i].Obligation) == fam {
			out = append(out, reqItem(&reqs[i], defaultKeyword(reqs[i].Obligation)))
		}
	}
	return out
}

func counts(reqs []Requirement, key func(*Requirement) string) []Count {
	m := map[string]int{}
	for i := range reqs {
		m[key(&reqs[i])]++
	}
	out := make([]Count, 0, len(m))
	for k, c := range m {
		out = append(out, Count{Value: k, Count: c, Percent: pct(c, len(reqs))})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// normalizeTitle lower-cases and drops punctuation and extra spaces.
func normalizeTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// textLanguages returns the clear languages of a title and of each sentence
// of a description, in order of appearance.
func textLanguages(title, description string) []string {
	parts := []string{title}
	parts = append(parts, strings.FieldsFunc(description, func(r rune) bool { return r == '.' || r == '!' || r == '?' || r == '\n' })...)
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		if l, ok := reqlang.DetectConfident(p, LanguageEvidence); ok && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

func uniq(v []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range v {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
