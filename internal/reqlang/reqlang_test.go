package reqlang

import (
	"encoding/json"
	"strings"
	"testing"
)

func rules(a Analysis) map[string]int {
	m := map[string]int{}
	for _, is := range a.Issues {
		m[is.Rule]++
	}
	return m
}

func TestEmpty(t *testing.T) {
	for _, s := range []string{"", "  \n\t"} {
		a := Analyze(s)
		if a.Obligation != "" || len(a.Issues) != 0 || len(a.Keywords) != 0 || a.Score != 0 {
			t.Fatalf("%q: %+v", s, a)
		}
		if a.Keywords == nil || a.Issues == nil {
			t.Fatal("slices must be non-nil for JSON")
		}
	}
}

func TestCleanRequirement(t *testing.T) {
	a := Analyze("The system MUST log each failed login attempt within 2 seconds.")
	if a.Obligation != LevelMust || a.Score != 100 || len(a.Issues) != 0 {
		t.Fatalf("%+v", a)
	}
	if len(a.Keywords) != 1 || a.Keywords[0] != (Keyword{"MUST", LevelMust, 11, 4}) {
		t.Fatalf("%+v", a.Keywords)
	}
	b, _ := json.Marshal(a)
	if !strings.Contains(string(b), `"keywords":[{"keyword":"MUST","level":"must","offset":11,"length":4}]`) {
		t.Fatal(string(b))
	}
}

func TestKeywordLevels(t *testing.T) {
	cases := map[string]string{
		"It MUST NOT crash.":             LevelMustNot,
		"It MUST  NOT\tcrash.":           LevelMustNot,
		"It SHALL NOT crash.":            LevelShallNot,
		"It SHOULD NOT crash.":           LevelShouldNot,
		"This is NOT RECOMMENDED.":       LevelShouldNot,
		"This is REQUIRED.":              LevelMust,
		"It SHALL work.":                 LevelShall,
		"It SHOULD work.":                LevelShould,
		"This is RECOMMENDED.":           LevelShould,
		"It MAY work.":                   LevelMay,
		"This is OPTIONAL.":              LevelOptional,
		"It WILL work.":                  LevelWill,
		"This section DESCRIBES it.":     LevelDescribe,
		"DESCRIBE the flow.":             LevelDescribe,
		"INFORMATIVE text.":              LevelDescribe,
		"NOTE: this is informational.":   LevelDescribe,
		"It MAY work. It SHALL work.":    LevelShall,
		"It MUST work. It MUST NOT die.": LevelMustNot,
		"It SHOULD NOT x. It SHALL y.":   LevelShall,
		"It WILL x. NOTE: y":             LevelWill,
		"musty mustard mayonnaise":       "",
	}
	for in, want := range cases {
		if got := DetectObligation(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	a := Analyze("It MUST NOT crash.")
	if len(a.Keywords) != 1 || a.Keywords[0].Keyword != "MUST NOT" || a.Keywords[0].Length != 8 {
		t.Fatalf("%+v", a.Keywords)
	}
	if rules(a)[RuleNegativeRequirement] != 1 {
		t.Fatal("negative requirement")
	}
}

func TestLowercase(t *testing.T) {
	a := Analyze("The user must log in. Access Should be logged.")
	if a.Obligation != LevelMust || len(a.Keywords) != 2 {
		t.Fatalf("%+v", a)
	}
	if rules(a)[RuleLowercaseKeyword] != 2 {
		t.Fatalf("%+v", a.Issues)
	}
	if a.Score != 96 {
		t.Fatalf("score %d", a.Score)
	}
}

func TestMissingKeyword(t *testing.T) {
	a := Analyze("The system logs failed logins.")
	if a.Obligation != "" || rules(a)[RuleMissingKeyword] != 1 || a.Score != 90 {
		t.Fatalf("%+v", a)
	}
	if a.Issues[0].Offset != 0 || a.Issues[0].Length != len("The system logs failed logins.") {
		t.Fatal(a.Issues[0])
	}
}

func TestRules(t *testing.T) {
	text := "The UI MUST be fast and user-friendly, etc. Response SHALL be TBD and/or TODO ???. " +
		"It SHOULD always be able to recover unless offline, where applicable.\n" +
		"It MUST be capable of every thing if possible. It MAY except none."
	a := Analyze(text)
	r := rules(a)
	want := map[string]int{
		RuleAmbiguousTerm:       3, // fast, user-friendly, etc.
		RuleTBD:                 3,
		RuleAndOr:               1,
		RuleUnboundedTerm:       3, // always, every, none
		RulePassiveCapability:   2,
		RuleEscapeClause:        4, // unless, where applicable, if possible, except
		RuleMultipleObligations: 0,
	}
	for k, v := range want {
		if r[k] != v {
			t.Errorf("rule %s: got %d want %d (%+v)", k, r[k], v, a.Issues)
		}
	}
	for _, is := range a.Issues {
		if got := text[is.Offset : is.Offset+is.Length]; is.Rule == RuleTBD && got != "TBD" && got != "TODO" && got != "???" {
			t.Errorf("tbd span %q", got)
		}
		if is.Message == "" || is.Severity == "" {
			t.Errorf("empty message/severity: %+v", is)
		}
	}
	for i := 1; i < len(a.Issues); i++ {
		if a.Issues[i].Offset < a.Issues[i-1].Offset {
			t.Fatal("issues not sorted")
		}
	}
	if a.Score != 0 {
		t.Fatalf("score should clamp to 0, got %d", a.Score)
	}
}

func TestMultipleObligationsAndLong(t *testing.T) {
	a := Analyze("The system MUST log and SHOULD alert. It MUST x and MUST y. NOTE: it MAY z!")
	if rules(a)[RuleMultipleObligations] != 1 { // repeated MUST and NOTE: do not count
		t.Fatalf("%+v", a.Issues)
	}
	if a.Issues[0].Offset != 0 || a.Issues[0].Length != len("The system MUST log and SHOULD alert.") {
		t.Fatalf("%+v", a.Issues[0])
	}
	long := "The system MUST " + strings.Repeat("word ", 45) + "end."
	a = Analyze(long)
	if rules(a)[RuleLongSentence] != 1 || a.Score != 98 {
		t.Fatalf("%+v", a)
	}
}

func TestUnicodeOffsets(t *testing.T) {
	text := "Ünïcödé façade — das System MUST prüfen; größe MAY vary? schnëll"
	a := Analyze(text)
	for _, k := range a.Keywords {
		if text[k.Offset:k.Offset+k.Length] != k.Keyword {
			t.Fatalf("offset mismatch: %+v", k)
		}
	}
	if len(a.Keywords) != 2 || a.Obligation != LevelMust {
		t.Fatalf("%+v", a.Keywords)
	}
	// Non-ASCII letters adjacent to a keyword prevent the match.
	if got := DetectObligation("éMUST MUSTé"); got != "" {
		t.Fatalf("got %q", got)
	}
	// keyword at very start and end
	a = Analyze("MUST")
	if a.Obligation != LevelMust || len(a.Issues) != 0 {
		t.Fatalf("%+v", a)
	}
}

func TestNoDuplicates(t *testing.T) {
	a := Analyze("as appropriate, appropriate. and so on")
	if rules(a)[RuleAmbiguousTerm] != 3 {
		t.Fatalf("%+v", a.Issues)
	}
	seen := map[[3]any]bool{}
	for _, is := range a.Issues {
		k := [3]any{is.Rule, is.Offset, is.Length}
		if seen[k] {
			t.Fatalf("dup %+v", is)
		}
		seen[k] = true
	}
}

func TestSentences(t *testing.T) {
	got := sentences("  A. B!  C?\nD e.g.x  ")
	want := []string{"A.", "B!", "C?", "D e.g.x"}
	text := "  A. B!  C?\nD e.g.x  "
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i, s := range got {
		if text[s.s:s.e] != want[i] {
			t.Fatalf("%d: %q", i, text[s.s:s.e])
		}
	}
	if rank("bogus") != len(Levels) {
		t.Fatal("rank")
	}
}
