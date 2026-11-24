package gherkin

import (
	"errors"
	"strings"
	"testing"
)

const shop = `# a comment
@shop @REQ-12
Feature: Checkout
  Customers pay for what is in the cart.

  Background:
    Given a customer with a saved card

  @smoke
  Scenario: Pay with the saved card
    When the customer pays 25 EUR
    And confirms with "1234"
    Then the order is "paid"
    But no e-mail is sent
    * the cart is empty

  Scenario Outline: Pay <amount>
    When the customer pays <amount> EUR
    Then the receipt says:
      """text
      Paid <amount>
      """
    And the totals are
      | item | price |
      | book | <amount> |

    @fast
    Examples: small
      | amount |
      | 5      |
      | 10     |
`

func mustParse(t *testing.T, src string) *Document {
	t.Helper()
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func TestParseEnglish(t *testing.T) {
	doc := mustParse(t, shop)
	f := doc.Feature
	if doc.Language != "en" || f.Name != "Checkout" || f.Description != "Customers pay for what is in the cart." {
		t.Fatalf("feature: %+v", f)
	}
	if strings.Join(f.Tags, " ") != "@shop @REQ-12" || f.Line != 3 {
		t.Fatalf("tags/line: %v %d", f.Tags, f.Line)
	}
	if f.Background == nil || len(f.Background.Steps) != 1 || f.Background.Steps[0].Type != Context {
		t.Fatalf("background: %+v", f.Background)
	}
	if len(f.Scenarios) != 2 {
		t.Fatalf("scenarios: %d", len(f.Scenarios))
	}
	sc := f.Scenarios[0]
	if sc.Tags[0] != "@smoke" || len(sc.Steps) != 5 || sc.Steps[1].Type != Conjunction || sc.Steps[4].Keyword != "*" {
		t.Fatalf("scenario: %+v", sc)
	}
	o := f.Scenarios[1]
	if !o.Outline || len(o.Examples) != 1 || o.Examples[0].Name != "small" || o.Examples[0].Tags[0] != "@fast" || len(o.Examples[0].Rows) != 2 {
		t.Fatalf("outline: %+v", o)
	}
	ds := o.Steps[1].DocString
	if ds == nil || ds.MediaType != "text" || ds.Content != "Paid <amount>" {
		t.Fatalf("doc string: %+v", ds)
	}
	if len(o.Steps[2].Table) != 2 || o.Steps[2].Table[1].Cells[1] != "<amount>" {
		t.Fatalf("table: %+v", o.Steps[2].Table)
	}
	if got := len(f.AllSteps()); got != 1+5+3 {
		t.Fatalf("all steps: %d", got)
	}
}

const swedish = `# language: sv
@kassa
Egenskap: Kassan
  Bakgrund:
    Givet en kund med ett sparat kort

  Scenario: Betala
    När kunden betalar 25 kr
    Och bekräftar
    Så är ordern betald
    Men inget mejl skickas

  Abstrakt scenario: Betala <belopp>
    När kunden betalar <belopp> kr
    Så är ordern betald

    Exempel:
      | belopp |
      | 5      |

  Scenariomall: Igen
    När kunden betalar <x> kr

    Exempel:
      | x |
      | 1 |

  Regel: Återbetalning
    Bakgrund:
      Givet en betald order

    Scenario: Återbetala
      När kunden ångrar köpet
      Så återbetalas pengarna
`

func TestParseSwedish(t *testing.T) {
	doc := mustParse(t, swedish)
	f := doc.Feature
	if doc.Language != "sv" || f.Keyword != "Egenskap" || f.Background.Keyword != "Bakgrund" {
		t.Fatalf("feature: %+v", f)
	}
	if len(f.Scenarios) != 3 || !f.Scenarios[1].Outline || !f.Scenarios[2].Outline || f.Scenarios[1].Examples[0].Keyword != "Exempel" {
		t.Fatalf("scenarios: %+v", f.Scenarios)
	}
	types := []StepType{}
	for _, st := range f.Scenarios[0].Steps {
		types = append(types, st.Type)
	}
	if types[0] != Action || types[1] != Conjunction || types[2] != Outcome {
		t.Fatalf("types: %v", types)
	}
	if len(f.Rules) != 1 || f.Rules[0].Keyword != "Regel" || f.Rules[0].Background == nil || len(f.Rules[0].Scenarios) != 1 {
		t.Fatalf("rules: %+v", f.Rules)
	}
	if len(f.AllScenarios()) != 4 {
		t.Fatalf("all scenarios")
	}
	// detected without a header
	doc = mustParse(t, "Egenskap: X\n  Scenario: Y\n    Givet z\n")
	if doc.Language != "sv" {
		t.Fatalf("language: %s", doc.Language)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		src  string
		line int
		want string
	}{
		{"Scenario: x\n", 1, "inside a Feature"},
		{"hello\n", 1, "expected \"Feature:\""},
		{"", 1, "no Feature"},
		{"# language: fr\nFeature: x\n", 1, "not supported"},
		{"Feature: a\nFeature: b\n", 2, "one Feature"},
		{"Feature: a\n  Given x\n", 2, "inside a Scenario"},
		{"Feature: a\n  Scenario: s\n    Given x\n    something else\n", 4, "expected a step"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n", 2, "no Examples"},
		{"Feature: a\n  Scenario Outline: s\n    Given <y>\n  Examples:\n    | x |\n    | 1 |\n", 3, "<y> is not a column"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n  Examples:\n", 4, "no table"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n  Examples:\n    | x |\n", 4, "no rows"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n  Examples:\n    | x |\n    | 1 | 2 |\n", 6, "2 cells"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n  Examples:\n    | x | x |\n    | 1 | 2 |\n", 5, "twice"},
		{"Feature: a\n  Scenario Outline: s\n    Given <x>\n  Examples:\n    | x | |\n    | 1 | 2 |\n", 5, "no name"},
		{"Feature: a\n  Scenario: s\n    Given x\n      | a |\n      | b | c |\n", 5, "first row"},
		{"Feature: a\n  Scenario: s\n    Given x\n      | a \n", 4, "end with |"},
		{"Feature: a\n  | a |\n", 2, "must follow a step or Examples"},
		{"Feature: a\n  Scenario: s\n    Given x\n      \"\"\"\n      open\n", 4, "not closed"},
		{"Feature: a\n  \"\"\"\n  \"\"\"\n", 2, "must follow a step"},
		{"Feature: a\n  @t\n", 2, "must be followed"},
		{"Feature: a\n  @t x\n  Scenario: s\n", 2, "a tag starts with @"},
		{"Feature: a\n  @t\n  Background:\n", 2, "cannot have tags"},
		{"Feature: a\n  Background:\n  Background:\n", 3, "already has a Background"},
		{"Feature: a\n  Scenario: s\n  Background:\n", 3, "before the scenarios"},
		{"Feature: a\n  Examples:\n", 2, "must follow a Scenario Outline"},
		{"Feature: a\n  Scenario: s\n    Given\n", 3, "has no text"},
		{"Feature: a\n  Scenario: s\n  Examples:\n    | x |\n    | 1 |\n    Given y\n", 6, "must be inside"},
		{"Feature: a\n  Scenario: s\n  Examples:\n    | x |\n    | 1 |\n    text\n", 6, "expected a table row"},
		{"Feature: a\n  Scenario: s\n    Given y\n    @t\n    When z\n", 4, "cannot have tags"},
		{"Feature: a\n  Rule: r\n    Background:\n    Background:\n", 4, "Rule already has"},
		{"Feature: a\n  Rule: r\n    Scenario: s\n    Background:\n", 4, "before the scenarios of the Rule"},
		{"\xff", 1, "UTF-8"},
		{strings.Repeat("a", MaxSource+1), 1, "larger"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		var es Errors
		if !errors.As(err, &es) || len(es) == 0 {
			t.Errorf("%q: want error, got %v", c.src, err)
			continue
		}
		found := false
		for _, e := range es {
			if e.Line == c.line && strings.Contains(e.Message, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: want line %d %q, got %v", c.src, c.line, c.want, es)
		}
	}
	// Error strings
	e := &Error{Line: 3, Column: 5, Message: "x"}
	if e.Error() != "line 3:5: x" || (&Error{Line: 2, Message: "y"}).Error() != "line 2: y" {
		t.Fatal(e.Error())
	}
	if (Errors{e, e}).Error() != "line 3:5: x; line 3:5: x" {
		t.Fatal("errors")
	}
}

func TestCells(t *testing.T) {
	cells, ok := SplitCells(`| a \| b | c\\d | e\nf | \x |`)
	if !ok || len(cells) != 4 || cells[0] != "a | b" || cells[1] != `c\d` || cells[2] != "e\nf" || cells[3] != `\x` {
		t.Fatalf("%q %v", cells, ok)
	}
	if _, ok := SplitCells("x"); ok {
		t.Fatal("not a row")
	}
	if EscapeCell("a|b\\\n") != `a\|b\\\n` {
		t.Fatal(EscapeCell("a|b\\\n"))
	}
}

func TestFormatRoundTrip(t *testing.T) {
	for _, src := range []string{shop, swedish} {
		doc := mustParse(t, src)
		out := Format(doc)
		doc2 := mustParse(t, out)
		if Format(doc2) != out {
			t.Fatalf("not stable:\n%s\n---\n%s", out, Format(doc2))
		}
		if len(doc2.Feature.AllSteps()) != len(doc.Feature.AllSteps()) {
			t.Fatal("steps lost")
		}
	}
	out := Format(mustParse(t, shop))
	if !strings.Contains(out, "      | item | price    |\n      | book | <amount> |") {
		t.Fatalf("table not aligned:\n%s", out)
	}
	if !strings.Contains(Format(mustParse(t, swedish)), "# language: sv") {
		t.Fatal("language header")
	}
	if Format(nil) != "" || Format(&Document{}) != "" {
		t.Fatal("empty")
	}
	// doc strings with delimiters inside and blank lines
	src := "Feature: f\n  Scenario: s\n    Given x\n      ```\n      a \\`\\`\\` b\n\n      c\n      ```\n"
	doc := mustParse(t, src)
	if doc.Feature.Scenarios[0].Steps[0].DocString.Content != "a ``` b\n\nc" {
		t.Fatalf("%q", doc.Feature.Scenarios[0].Steps[0].DocString.Content)
	}
	if f := Format(doc); !strings.Contains(f, "a \\`\\`\\` b\n\n      c") {
		t.Fatalf("%s", f)
	}
}

func TestAddTags(t *testing.T) {
	out, err := AddTags(shop, []string{"SHOP-TC-1", "@shop", "two words", ""})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# a comment\n@SHOP-TC-1 @two-words\n@shop @REQ-12\nFeature: Checkout") {
		t.Fatalf("%s", out)
	}
	same, _ := AddTags(shop, []string{"@shop"})
	if same != shop {
		t.Fatal("unchanged")
	}
	if _, err := AddTags("bad", []string{"x"}); err == nil {
		t.Fatal("want error")
	}
	out, _ = AddTags("Feature: a\n", []string{"k"})
	if out != "@k\nFeature: a\n" {
		t.Fatalf("%q", out)
	}
}

func TestExpand(t *testing.T) {
	inst := Expand(mustParse(t, shop))
	if len(inst) != 3 {
		t.Fatalf("instances: %d", len(inst))
	}
	if inst[0].Name != "Pay with the saved card" || len(inst[0].Steps) != 6 || inst[0].Steps[0].Text != "a customer with a saved card" {
		t.Fatalf("first: %+v", inst[0])
	}
	if inst[0].Steps[2].Type != Action || inst[0].Steps[4].Type != Outcome || inst[0].Steps[5].Type != Outcome {
		t.Fatalf("types: %+v", inst[0].Steps)
	}
	if strings.Join(inst[0].Tags, " ") != "@shop @REQ-12 @smoke" {
		t.Fatalf("tags: %v", inst[0].Tags)
	}
	o := inst[1]
	if o.Name != "Pay 5" || o.Values["amount"] != "5" || o.RowLine == 0 || o.Steps[1].Text != "the customer pays 5 EUR" || o.Steps[2].Argument != "Paid 5" {
		t.Fatalf("outline: %+v", o)
	}
	if !strings.Contains(o.Steps[3].Argument, "| book | 5     |") || o.Tags[len(o.Tags)-1] != "@fast" {
		t.Fatalf("table: %q %v", o.Steps[3].Argument, o.Tags)
	}
	sv := Expand(mustParse(t, swedish))
	if len(sv) != 4 || sv[2].Name != "Igen #1 (x=1)" || sv[3].Rule != "Återbetalning" || len(sv[3].Steps) != 4 {
		t.Fatalf("sv: %+v", sv)
	}
	if Expand(nil) != nil {
		t.Fatal("nil")
	}
	if st := (&Step{}); st.Argument() != "" {
		t.Fatal("argument")
	}
}

func TestExpressions(t *testing.T) {
	cases := []struct {
		pattern, kind, text string
		ok                  bool
		values              []string
	}{
		{"I have {int} cucumber(s)", "", "I have 3 cucumbers", true, []string{"3"}},
		{"I have {int} cucumber(s)", "", "I have 1 cucumber", true, []string{"1"}},
		{"I have {int} cucumber(s)", "", "I have x cucumbers", false, nil},
		{"I log in as {string}", "cucumber", `I log in as "ann"`, true, []string{"ann"}},
		{"I log in as {string}", "cucumber", `I log in as 'bo b'`, true, []string{"bo b"}},
		{"I log in as {string}", "cucumber", `I log in as "<user>"`, true, []string{"<user>"}},
		{"I log in as {string}", "cucumber", `I log in as <user>`, true, []string{"<user>"}},
		{"it costs {float} EUR", "", "it costs 2.50 EUR", true, []string{"2.50"}},
		{"the {word} button", "", "the OK button", true, []string{"OK"}},
		{"I wait/pause {}", "", "I pause for a while", true, []string{"for a while"}},
		{"a \\{literal\\} \\(x\\)", "", "a {literal} (x)", true, nil},
		{`^I pay (\d+) EUR$`, "regex", "I pay 25 EUR", true, []string{"25"}},
		{`I pay (\d+) EUR`, "regex", "I pay 25 EUR!", false, nil},
	}
	for _, c := range cases {
		e, err := Compile(c.pattern, c.kind)
		if err != nil {
			t.Fatalf("%s: %v", c.pattern, err)
		}
		args, ok := e.Match(c.text)
		if ok != c.ok {
			t.Errorf("%s ~ %s: %v", c.pattern, c.text, ok)
			continue
		}
		for i, v := range c.values {
			if args[i].Value != v {
				t.Errorf("%s ~ %s: arg %d = %q", c.pattern, c.text, i, args[i].Value)
			}
		}
	}
	for _, bad := range []struct{ p, k string }{{"", ""}, {"x {nope}", ""}, {"x {int", ""}, {"x (y", ""}, {"x )", ""}, {"(a{int})", ""}, {"a//b", ""}, {"(", "regex"}, {"x", "glob"}} {
		if _, err := Compile(bad.p, bad.k); err == nil {
			t.Errorf("%q %q: want error", bad.p, bad.k)
		}
	}
	e, _ := Compile("I log in as {string} with {int} tries", "")
	if e.Params() != 2 {
		t.Fatal("params")
	}
	to, _ := Compile("I sign in/up as {string} after {int} attempt(s)", "")
	args, _ := e.Match(`I log in as "ann" with 3 tries`)
	out, err := to.Render(args)
	if err != nil || out != `I sign in as "ann" after 3 attempts` {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := to.Render(args[:1]); err == nil {
		t.Fatal("arity")
	}
	re, _ := Compile(`I pay (\d+)`, "regex")
	if re.Params() != 1 {
		t.Fatal("regex params")
	}
	if _, err := re.Render(nil); err == nil {
		t.Fatal("regex render")
	}
}

func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{
		`I log in as "ann" with 3 tries`: "I log in as {string} with {int} tries",
		"it costs 2.50 EUR":              "it costs {float} EUR",
		"I pay <amount> now":             "I pay {} now",
		"a {b} (c) d/e":                  `a \{b\} \(c\) d\/e`,
	} {
		if got := Suggest(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
		e, err := Compile(Suggest(in), "")
		if err != nil {
			t.Fatalf("%q: %v", Suggest(in), err)
		}
		if _, ok := e.Match(in); !ok {
			t.Errorf("suggestion %q does not match %q", Suggest(in), in)
		}
	}
}

func TestAnalyseAndRename(t *testing.T) {
	defs := []Definition{
		{ID: "d1", Pattern: "the customer pays {int} EUR"},
		{ID: "d2", Pattern: "a customer with a saved card"},
		{ID: "d3", Pattern: "bad {x}"},
	}
	srcs := []Source{{TestID: "t1", TestKey: "TC-1", Text: shop}, {TestID: "t2", TestKey: "TC-2", Text: "Feature: broken\n  Scenario: s\n    Given the customer pays 3 EUR\n    oops\n"}}
	a := Analyse(defs, srcs)
	if len(a.Uses["d1"]) != 3 || len(a.Uses["d2"]) != 1 || a.Invalid["d3"] == "" || a.Unparsed["t2"] == "" {
		t.Fatalf("analysis: %+v", a)
	}
	if len(a.Uses["d3"]) != 0 {
		t.Fatal("invalid uses")
	}
	found := false
	for _, u := range a.Undefined {
		if u.Text == `confirms with "1234"` && u.Suggestion == "confirms with {string}" && u.Uses[0].Line == 12 {
			found = true
		}
	}
	if !found {
		t.Fatalf("undefined: %+v", a.Undefined)
	}
	from, _ := Compile("the customer pays {int} EUR", "")
	to, _ := Compile("the customer pays {int} euro(s)", "")
	out, n, err := Rename(shop, from, to)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if !strings.Contains(out, "    When the customer pays 25 euros\n") || !strings.Contains(out, "When the customer pays <amount> euros") || !strings.Contains(out, "# a comment") {
		t.Fatalf("%s", out)
	}
	two, _ := Compile("x {int} {int}", "")
	if _, _, err := Rename(shop, from, two); err == nil {
		t.Fatal("arity")
	}
	re, _ := Compile("x (y)", "regex")
	if _, _, err := Rename(shop, from, re); err == nil {
		t.Fatal("regex")
	}
	if _, _, err := Rename("nope", from, to); err == nil {
		t.Fatal("unparsable")
	}
}

func TestConvert(t *testing.T) {
	pt := PlainTest{Title: "Pay", Description: "Pays.", Preconditions: "logged in\nhas a card",
		Steps:      []PlainStep{{Action: "pay {{amount}}", Expected: "paid", Data: "card=1\nexp=12"}, {Call: "TC-9 Login"}, {Expected: "receipt"}},
		Parameters: []string{"amount"}, Rows: []PlainRow{{Values: map[string]string{"amount": "5"}}}, Tags: []string{"smoke", "two words"}}
	src := FromPlain(pt, "en")
	doc := mustParse(t, src)
	sc := doc.Feature.Scenarios[0]
	if !sc.Outline || sc.Keyword != "Scenario Template" || sc.Steps[0].Keyword != "Given" || sc.Steps[1].Keyword != "And" || sc.Steps[2].Text != "pay <amount>" {
		t.Fatalf("%s", src)
	}
	if sc.Steps[2].DocString == nil || sc.Steps[2].DocString.Content != "card=1\nexp=12" || !strings.Contains(src, "the steps of TC-9 Login are done") {
		t.Fatalf("%s", src)
	}
	if strings.Join(doc.Feature.Tags, " ") != "@smoke @two-words" {
		t.Fatal(doc.Feature.Tags)
	}
	sv := FromPlain(PlainTest{Title: "Betala", Steps: []PlainStep{{Action: "betala\nmed kort", Expected: "betald"}, {Call: "TC-1"}}}, "sv")
	if !strings.Contains(sv, "# language: sv\nEgenskap: Betala\n\n  Scenario: Betala\n    När betala\n      \"\"\"\n      med kort\n      \"\"\"\n    Så betald\n    När stegen i TC-1 är gjorda") {
		t.Fatalf("%s", sv)
	}
	back, warn := ToPlain(doc)
	if back.Title != "Pay" || back.Preconditions != "Given logged in\nAnd has a card" || len(back.Steps) != 2 || back.Steps[1].Expected != "receipt" || back.Steps[0].Action != "pay {{amount}}" || back.Steps[0].Expected != "paid" {
		t.Fatalf("%+v", back)
	}
	if len(back.Parameters) != 1 || back.Rows[0].Values["amount"] != "5" || len(warn) != 0 {
		t.Fatalf("%+v %v", back, warn)
	}
	// several scenarios, rules, a Then first, a Given in the middle
	multi := mustParse(t, swedish+"\n")
	m, warn := ToPlain(multi)
	if !strings.HasPrefix(m.Steps[0].Action, "[Betala] ") || len(warn) == 0 || m.Preconditions != "Givet en kund med ett sparat kort" {
		t.Fatalf("%+v %v", m, warn)
	}
	odd := mustParse(t, "Feature: f\n  Scenario: s\n    Then first\n    When act\n    Given later\n      | a |\n  Scenario Outline: o\n    When <x>\n    Examples:\n      | x |\n      | 1 |\n    Examples:\n      | x |\n      | 2 |\n")
	op, warn := ToPlain(odd)
	if op.Steps[0].Action != "[s]" || op.Steps[0].Expected != "first" || op.Steps[2].Action != "Given later" || op.Steps[2].Data != "| a |" || len(warn) != 1 {
		t.Fatalf("%+v %v", op, warn)
	}
	if p, _ := ToPlain(nil); len(p.Steps) != 0 {
		t.Fatal("nil")
	}
}

func TestSplit(t *testing.T) {
	doc := mustParse(t, swedish)
	parts := Split(doc, swedish, true)
	if len(parts) != 4 {
		t.Fatalf("%d", len(parts))
	}
	for _, p := range parts {
		d := mustParse(t, p.Source)
		if len(d.Feature.AllScenarios()) != 1 || d.Feature.Background == nil {
			t.Fatalf("%s", p.Source)
		}
	}
	if parts[3].Title != "Återbetala" || !strings.Contains(parts[3].Source, "Regel: Återbetalning") || parts[0].Tags[0] != "kassa" {
		t.Fatalf("%+v", parts[3])
	}
	whole := Split(mustParse(t, shop), shop, false)
	if len(whole) != 1 || whole[0].Title != "Checkout" || strings.Join(whole[0].Tags, ",") != "shop,REQ-12,smoke,fast" || whole[0].Source != shop {
		t.Fatalf("%+v", whole)
	}
	if Split(nil, "", true) != nil {
		t.Fatal("nil")
	}
	empty := Split(mustParse(t, "Feature:\n"), "Feature:\n", true)
	if len(empty) != 1 || empty[0].Title != "Feature" {
		t.Fatalf("%+v", empty)
	}
	untitled := Split(mustParse(t, "Feature: F\n  Scenario:\n    Given x\n"), "", true)
	if untitled[0].Title != "F" {
		t.Fatal(untitled[0].Title)
	}
}

func TestKeywords(t *testing.T) {
	if StepKeywordType("Givet") != Context || StepKeywordType("*") != Conjunction || StepKeywordType("Nope") != "" {
		t.Fatal("types")
	}
	if DialectOf("SV").Code != "sv" || DialectOf("xx").Code != "en" {
		t.Fatal("dialect")
	}
}
