package tagsuggest

import (
	"strings"
	"testing"
)

func tags(s []Suggestion) string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = x.Tag + "/" + x.Reason
	}
	return strings.Join(out, ",")
}

func TestExistingTagsComeFirst(t *testing.T) {
	existing := map[string]int{"password": 4, "login": 7, "gdpr": 2, "payment": 3, "two-factor": 1}
	got := Suggest("Password login", "The system MUST lock the account after 5 failed logins. Passwords are hashed. Two factor is optional.", existing, nil, 0)
	s := tags(got)
	for _, want := range []string{"login/existing", "password/existing", "two-factor/existing"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	if strings.Contains(s, "payment") || strings.Contains(s, "gdpr") {
		t.Errorf("unrelated tag suggested: %s", s)
	}
	if got[0].Reason != ReasonExisting || got[0].Uses == 0 {
		t.Errorf("first suggestion should be an existing tag: %+v", got[0])
	}
	// the title boosts: login (title, 7 uses) before two-factor (description, 1 use)
	if strings.Index(s, "login") > strings.Index(s, "two-factor") {
		t.Errorf("order: %s", s)
	}
}

func TestKeywordsAndAcronyms(t *testing.T) {
	got := tags(Suggest("Session timeout", "The session MUST expire after 30 minutes of inactivity. Expired session tokens are rejected by the API (GDPR).", nil, nil, 0))
	for _, want := range []string{"session/keyword", "api/acronym", "gdpr/acronym", "timeout/keyword"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	for _, bad := range []string{"must", "the", "minutes", "30", "after"} {
		if strings.Contains(got, bad+"/") {
			t.Errorf("stop word or single mention %q suggested: %s", bad, got)
		}
	}
}

func TestSwedish(t *testing.T) {
	got := tags(Suggest("Lösenord för inloggning", "Systemet SKA låsa kontot efter fem misslyckade inloggningar. Lösenordet SKA lagras som hash.", map[string]int{"lösenord": 2}, nil, 0))
	if !strings.Contains(got, "lösenord/existing") || !strings.Contains(got, "inloggning/keyword") {
		t.Errorf("got %s", got)
	}
	if strings.Contains(got, "ska/") || strings.Contains(got, "systemet/") {
		t.Errorf("keyword or stop word suggested: %s", got)
	}
}

func TestCurrentTagsAndLimit(t *testing.T) {
	got := Suggest("Export export export", "CSV PDF XLSX DOCX ZIP", map[string]int{"export": 1}, []string{"Export", "csv"}, 3)
	if len(got) != 3 {
		t.Fatalf("limit: %v", got)
	}
	for _, s := range got {
		if s.Tag == "export" || s.Tag == "csv" {
			t.Errorf("current tag suggested again: %v", got)
		}
	}
	if len(Suggest("", "", map[string]int{"x": 1, "": 3}, nil, 0)) != 0 {
		t.Error("empty text gives suggestions")
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"passwords": "password", "stories": "story", "logging": "logg", "lösenordet": "lösenord", "api": "api", "inloggningar": "inloggning"} {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
	if !isAcronym("GDPR") || !isAcronym("SHA256") || isAcronym("X509") || isAcronym("Gdpr") || isAcronym("A") || isAcronym("ABCDEFGHIJ") {
		t.Error("isAcronym")
	}
	if normalize("  Two  Factor ") != "two-factor" || len([]rune(normalize(strings.Repeat("å", 100)))) != MaxTagLength {
		t.Error("normalize")
	}
	if !isNumber("1.4.2") || isNumber("v1") {
		t.Error("isNumber")
	}
	toks := tokenize("re-use, foo_bar. -x- e.g.", false, nil)
	if len(toks) != 4 || toks[0].lower != "re-use" || toks[1].lower != "foo_bar" || toks[2].lower != "x" || toks[3].lower != "e.g" {
		t.Errorf("tokenize: %+v", toks)
	}
}

func TestExistingSpellingKept(t *testing.T) {
	got := Suggest("GDPR export", "Export personal data per GDPR.", map[string]int{"GDPR": 3, "Two_Factor": 1}, []string{"export"}, 0)
	if len(got) != 1 || got[0].Tag != "GDPR" || got[0].Reason != ReasonExisting {
		t.Errorf("%+v", got)
	}
}

func TestDuplicateWeights(t *testing.T) {
	// an existing tag that is also a keyword keeps the higher score and its use count
	got := Suggest("Audit audit", "audit log", map[string]int{"audit": 5}, nil, 0)
	if got[0].Tag != "audit" || got[0].Reason != ReasonExisting || got[0].Uses != 5 {
		t.Errorf("%+v", got)
	}
}
