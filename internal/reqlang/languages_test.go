package reqlang

import (
	"strings"
	"testing"
)

func issuesByRule(a Analysis) map[string][]Issue {
	out := map[string][]Issue{}
	for _, is := range a.Issues {
		out[is.Rule] = append(out[is.Rule], is)
	}
	return out
}

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"": "en",
		"The system SHALL log every login attempt.":         "en",
		"Systemet ska logga alla inloggningsförsök.":        "sv",
		"Användaren får inte se andra användares data.":     "sv",
		"Det är viktigt att systemet är snabbt och enkelt.": "sv",
		"System ska":   "en", // one marker each: ties go to English
		"Systemet ska": "sv",
		"12345 !!!":    "en",
		"Systemet SHALL logga alla försök (engelska nyckelord).": "sv",
	}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%q) = %s, want %s", text, got, want)
		}
	}
	if got := Languages(); strings.Join(got, ",") != "en,sv" || !Supported("sv") || Supported("de") {
		t.Fatal(got)
	}
}

func TestSwedishKeywords(t *testing.T) {
	cases := map[string]string{
		"Systemet ska logga inloggningar.":                  LevelShall,
		"Systemet skall logga inloggningar.":                LevelShall,
		"Systemet måste kryptera lösenord.":                 LevelMust,
		"Kryptering är obligatorisk för alla anslutningar.": LevelMust,
		"Gränssnittet bör vara tillgängligt på svenska.":    LevelShould,
		"Användaren får exportera rapporter.":               LevelMay,
		"Systemet ska inte spara lösenord i klartext.":      LevelShallNot,
		"Användaren får inte se andras data.":               LevelMustNot,
		"Loggar bör inte innehålla personnummer.":           LevelShouldNot,
		"Exporten är valfri.":                               LevelOptional,
		"Systemet kommer att ersätta det gamla systemet.":   LevelWill,
		"Det här avsnittet beskriver arkitekturen.":         LevelDescribe,
		"Systemet SHALL logga allt på svenska.":             LevelShall, // English RFC keyword in Swedish text
	}
	for text, want := range cases {
		a := AnalyzeLang(text, "sv")
		if a.Obligation != want || a.Language != "sv" {
			t.Errorf("%q: obligation %q (lang %s), want %q", text, a.Obligation, a.Language, want)
		}
	}
	// "får inte" wins over "får" (longest match) and the strongest level is chosen
	a := AnalyzeLang("Systemet SKA logga och får inte radera loggar.", "sv")
	if a.Obligation != LevelMustNot || len(a.Keywords) != 2 || a.Keywords[1].Keyword != "FÅR INTE" {
		t.Fatal(a.Keywords)
	}
	// lowercase English RFC words are not keywords in Swedish text, and the
	// RFC 8174 uppercase rule does not apply to Swedish keywords
	a = AnalyzeLang("Systemet ska logga, the user may read.", "sv")
	if len(a.Keywords) != 1 || len(issuesByRule(a)[RuleLowercaseKeyword]) != 0 {
		t.Fatal(a)
	}
}

func TestSwedishRules(t *testing.T) {
	a := AnalyzeLang("Systemet ska vara snabbt och användarvänligt och/eller flexibelt om möjligt. Svarstid: TBD. "+
		"Alla användare ska kunna logga in.", "")
	if a.Language != "sv" {
		t.Fatal(a.Language)
	}
	r := issuesByRule(a)
	terms := func(rule string) []string {
		var out []string
		for _, is := range r[rule] {
			out = append(out, is.Params["term"])
		}
		return out
	}
	if got := strings.Join(terms(RuleAmbiguousTerm), ","); got != "snabbt,användarvänligt,flexibelt" {
		t.Errorf("ambiguous: %s", got)
	}
	for rule, want := range map[string]string{RuleAndOr: "och/eller", RuleEscapeClause: "om möjligt", RuleTBD: "TBD",
		RuleUnboundedTerm: "Alla", RulePassiveCapability: "kunna"} {
		if got := strings.Join(terms(rule), ","); got != want {
			t.Errorf("%s: %q, want %q", rule, got, want)
		}
	}
	if len(r[RuleMultipleObligations]) != 0 {
		t.Error("one keyword per sentence")
	}
	// "måste inte" = "need not": optional, flagged as an ambiguous negation
	a = AnalyzeLang("Användaren måste inte ange telefonnummer.", "sv")
	r = issuesByRule(a)
	if a.Obligation != LevelOptional || len(r[RuleAmbiguousNegation]) != 1 || r[RuleAmbiguousNegation][0].Params["term"] != "måste inte" {
		t.Fatal(a)
	}
	// missing keyword names Swedish examples; multiple obligations list keywords
	a = AnalyzeLang("Systemet loggar inloggningar.", "sv")
	if is := issuesByRule(a)[RuleMissingKeyword]; len(is) != 1 || is[0].Params["examples"] != "SKA, MÅSTE, BÖR, FÅR" {
		t.Fatal(a.Issues)
	}
	a = AnalyzeLang("Systemet ska logga och bör arkivera.", "sv")
	if is := issuesByRule(a)[RuleMultipleObligations]; len(is) != 1 || is[0].Params["keywords"] != "SKA, BÖR" {
		t.Fatal(a.Issues)
	}
	if is := issuesByRule(AnalyzeLang("Systemet "+strings.Repeat("ska logga mycket ", 15)+".", "sv"))[RuleLongSentence]; len(is) != 1 || is[0].Params["max"] != "40" {
		t.Fatal(is)
	}
}

func TestLanguageSelection(t *testing.T) {
	// explicit language overrides detection; unknown falls back to detection
	if a := AnalyzeLang("The system ska log.", "sv"); a.Language != "sv" || a.Obligation != LevelShall {
		t.Fatal(a)
	}
	if a := AnalyzeLang("Systemet ska logga.", "de"); a.Language != "sv" {
		t.Fatal(a.Language)
	}
	if a := AnalyzeLang("Systemet ska logga.", "auto"); a.Language != "sv" {
		t.Fatal(a.Language)
	}
	if a := AnalyzeLang("  ", "sv"); a.Language != "sv" || len(a.Issues) != 0 || a.Score != 0 {
		t.Fatal(a)
	}
	if a := AnalyzeLang("", "de"); a.Language != "" {
		t.Fatal(a.Language)
	}
	if DetectObligation("Systemet måste logga.") != LevelMust {
		t.Fatal("auto detection for obligations")
	}
	// English messages carry params for translation
	a := Analyze("The system should be fast.")
	if a.Language != "en" {
		t.Fatal(a.Language)
	}
	for _, is := range a.Issues {
		if is.Rule == RuleAmbiguousTerm && is.Params["term"] != "fast" {
			t.Fatal(is)
		}
	}
}

func TestDetectConfident(t *testing.T) {
	for _, c := range []struct {
		text string
		lang string
		ok   bool
	}{
		{"The system MUST log every sign-in of the user.", "en", true},
		{"Systemet SKA logga varje inloggning för användaren.", "sv", true},
		{"Login", "", false},
		{"", "", false},
		{"API 2.0", "", false},
	} {
		lang, ok := DetectConfident(c.text, 2)
		if lang != c.lang || ok != c.ok {
			t.Errorf("%q: %q %v", c.text, lang, ok)
		}
	}
	if !IsRFCKeyword("must not") || !IsRFCKeyword("RECOMMENDED") || IsRFCKeyword("SKA") || IsRFCKeyword("WILL") {
		t.Error("IsRFCKeyword")
	}
}
