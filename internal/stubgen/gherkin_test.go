package stubgen

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/gherkin"
	"github.com/rom/qtestframe/internal/model"
)

func TestCucumberStubsAreGherkin(t *testing.T) {
	// the stubs of step-format tests are valid Gherkin
	for _, tc := range fixtures() {
		f, err := Generate("cucumber", tc, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gherkin.Parse(f.Content); err != nil {
			t.Errorf("%s: %v\n%s", tc.Key, err, f.Content)
		}
	}
	// a Gherkin test: its own feature, tagged, comments kept
	src := "# language: sv\n# kommentar\n@kassa\nEgenskap: Kassan\n  Scenario: Betala\n    När kunden betalar\n    Så är ordern betald\n"
	m := &model.TestCase{Key: "SHOP-TC-5", Title: "Kassan", Format: model.FormatGherkin, Gherkin: src}
	tc := FromModel(m, []string{"SHOP-REQ-1"}, "")
	if len(tc.Steps) != 1 || tc.Steps[0].Action != "kunden betalar" || tc.Steps[0].Expected != "är ordern betald" {
		t.Fatalf("steps: %+v", tc.Steps)
	}
	f, err := Generate("cucumber", tc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.Content, "# language: sv\n# qtestframe:begin SHOP-TC-5 cucumber") || !strings.Contains(f.Content, "# kommentar\n@SHOP-TC-5 @qtestframe-stub\n@kassa\nEgenskap: Kassan") {
		t.Fatalf("%s", f.Content)
	}
	doc, err := gherkin.Parse(f.Content)
	if err != nil || doc.Language != "sv" {
		t.Fatalf("%v %+v", err, doc)
	}
	if out, act := Merge([]byte(f.Content), f); act != Unchanged || out != f.Content {
		t.Fatal(act)
	}
	// other frameworks document the scenario steps
	py, err := Generate("pytest", tc, Options{})
	if err != nil || !strings.Contains(py.Content, "kunden betalar") {
		t.Fatalf("%v %s", err, py.Content)
	}
	// a Gherkin test that does not parse falls back to the template
	tc.Gherkin = "broken"
	f, err = Generate("cucumber", tc, Options{})
	if err != nil || !strings.Contains(f.Content, "Feature: SHOP-TC-5 Kassan") {
		t.Fatalf("%v %s", err, f.Content)
	}
}
