package reqlang

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the web client's keyword table and parity cases")

// clientCases are texts the web client must analyse exactly like Analyze
// (web/src/lib/obligationKeywords.test.ts reads the expected results).
var clientCases = []string{
	"The system MUST log every login.",
	"The system MUST NOT store passwords in clear text.",
	"The service SHALL NOT  restart; it SHOULD NOT\nwarn and MAY retry.",
	"Logging is OPTIONAL. Encryption is REQUIRED and RECOMMENDED, but NOT RECOMMENDED for tests.",
	"MAYBE the system MUSTER the users; SHALLOW copies.",
	"The system may retry and must log.",
	"The client WILL show a NOTE: describing it.",
	"Systemet ska logga alla inloggningar.",
	"Systemet SKA INTE spara lösenord i klartext.",
	"Användaren får inte radera loggen och bör inte se den.",
	"Användaren FÅR exportera data. FÅRA och SKALA är inga nyckelord.",
	"Systemet skall stödja export och bör ej kräva inloggning; det måste inte vara snabbt.",
	"Systemet MUST support SSO och ska logga det.",
	"Systemet must support SSO och ska logga det.",
	"Underscored MUST_NOT and digits MUST2 are not keywords, but (MUST) is.",
	"Ändringen BÖR göras; det är valfritt och obligatoriskt att granska.",
	"",
}

type clientCaseKeyword struct {
	Text    string `json:"text"` // as written in the case
	Keyword string `json:"keyword"`
	Level   string `json:"level"`
}

type clientCase struct {
	Text       string              `json:"text"`
	Language   string              `json:"language"`
	Obligation string              `json:"obligation"`
	Keywords   []clientCaseKeyword `json:"keywords"`
}

func webLib(name string) string { return filepath.Join("..", "..", "web", "src", "lib", name) }

func checkGolden(t *testing.T, path string, v any) {
	t.Helper()
	want, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/reqlang -run TestClientTable -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date: run go test ./internal/reqlang -run TestClientTable -update", path)
	}
}

// TestClientTable pins the web client's keyword table to the language
// packs, so the client never has a diverging keyword list.
func TestClientTable(t *testing.T) {
	checkGolden(t, webLib("obligationKeywords.json"), KeywordTable())

	cases := make([]clientCase, 0, len(clientCases))
	for _, text := range clientCases {
		a := Analyze(text)
		c := clientCase{Text: text, Language: a.Language, Obligation: a.Obligation, Keywords: []clientCaseKeyword{}}
		for _, k := range a.Keywords {
			c.Keywords = append(c.Keywords, clientCaseKeyword{Text: text[k.Offset : k.Offset+k.Length], Keyword: k.Keyword, Level: k.Level})
		}
		cases = append(cases, c)
	}
	checkGolden(t, webLib("obligationKeywords.cases.json"), cases)
}

func TestKeywordTable(t *testing.T) {
	tb := KeywordTable()
	if len(tb.Packs) != len(Languages()) || len(tb.Levels) != len(Levels) {
		t.Fatalf("table: %+v", tb)
	}
	for _, p := range tb.Packs {
		if len(p.Keywords) == 0 || len(p.Markers) == 0 {
			t.Errorf("pack %s has no keywords or markers", p.Code)
		}
		// longest first, as matched
		for i := 1; i < len(p.Keywords); i++ {
			if len(p.Keywords[i].Text) > len(p.Keywords[i-1].Text) {
				t.Errorf("pack %s: %q after %q", p.Code, p.Keywords[i].Text, p.Keywords[i-1].Text)
			}
		}
	}
	sv := tb.Packs[1]
	if sv.Code != "sv" || sv.Letters != "åäö" || sv.UppercaseRule {
		t.Errorf("sv pack: %+v", sv)
	}
}
