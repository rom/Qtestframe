package stubgen

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// fixtures: a plain test and a data-driven one with awkward texts.
func fixtures() []Test {
	return []Test{
		{
			Key: "SHOP-TC-12", Title: "Checkout with a saved card", Priority: "high",
			URL:          "https://qtestframe.example.com/p/p1/tests?item=t12",
			Requirements: []string{"SHOP-REQ-3", "SHOP-REQ-4"}, Tags: []string{"checkout", "runner:pytest"},
			Preconditions: "A customer with a saved card is logged in.\nThe cart holds two items.",
			Expected:      "The order is placed and paid.",
			Steps: []Step{
				{Action: "Open the cart", Expected: "The cart shows 2 items"},
				{Action: "Choose the saved card\nand confirm", Data: "card ending 4242", Expected: "The receipt page opens"},
				{Call: "SHOP-TC-3 Log out"},
			},
		},
		{
			Key: "SHOP-TC-7", Title: `Login "quoted" \ back*/ #hash ${var} \u000a Åäö`,
			Description: "Data-driven login.",
			Steps: []Step{
				{Action: "Log in as {{user name}} with {{password}}", Expected: "Welcome, {{user name}} | done"},
			},
			Parameters: []string{"user name", "password", "class"},
			Rows: []Row{
				{Name: "admin", Values: map[string]string{"user name": "admin", "password": `p'a"s\s|w`, "class": "x"}},
				{Name: "", Values: map[string]string{"user name": "ö  #  ${x}", "password": "", "class": "y"}},
				{Name: "admin", Values: map[string]string{"user name": "dup", "password": "z"}},
			},
		},
		{Key: "SHOP-TC-1", Title: "", Expected: "Works"},
	}
}

func TestGolden(t *testing.T) {
	for _, fw := range Frameworks {
		for _, tc := range fixtures() {
			f, err := Generate(fw.ID, tc, Options{})
			if err != nil {
				t.Fatalf("%s %s: %v", fw.ID, tc.Key, err)
			}
			if f.Path != FileName(fw.ID, tc.Key) || f.Key != tc.Key || f.Framework != fw.ID {
				t.Fatal(f.Path, f.Key, f.Framework)
			}
			golden := filepath.Join("testdata", "golden", fw.ID, f.Path)
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(f.Content), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/stubgen -update)", err)
			}
			if string(want) != f.Content {
				t.Errorf("%s differs from the golden file (go test ./internal/stubgen -update):\n%s", golden, f.Content)
			}
			// every stub carries the key in a form the JUnit mapping finds
			// and marks itself as not implemented
			if !strings.Contains(f.Content, Ident(tc.Key)) && !strings.Contains(f.Content, tc.Key) {
				t.Errorf("%s: no key", golden)
			}
			if !strings.Contains(f.Content, "qtestframe stub") && fw.ID != "cucumber" {
				t.Errorf("%s: no stub marker", golden)
			}
		}
	}
}

func TestLookupAndNames(t *testing.T) {
	for alias, want := range map[string]string{"py": "pytest", "Go": "go", "java": "junit5", "gherkin": "cucumber", "robotframework": "robot", "vitest": "vitest", "pw": "playwright"} {
		if f, ok := Lookup(alias); !ok || f.ID != want {
			t.Error(alias, f.ID)
		}
	}
	if _, ok := Lookup("cobol"); ok {
		t.Fatal("cobol")
	}
	if _, err := Generate("cobol", fixtures()[0], Options{}); err == nil {
		t.Fatal("unknown framework")
	}
	if _, err := Generate("go", Test{}, Options{}); err == nil {
		t.Fatal("no key")
	}
	if len(FrameworkIDs()) != len(Frameworks) {
		t.Fatal(FrameworkIDs())
	}
	if FileName("other", "A-TC-1") != "A_TC_1" {
		t.Fatal(FileName("other", "A-TC-1"))
	}
	// packages
	f, _ := Generate("go", fixtures()[0], Options{Package: "e2e"})
	if !strings.Contains(f.Content, "\npackage e2e\n") {
		t.Fatal(f.Content)
	}
	f, _ = Generate("go", fixtures()[0], Options{Package: "not valid"})
	if !strings.Contains(f.Content, "\npackage tests\n") {
		t.Fatal(f.Content)
	}
	f, _ = Generate("junit5", fixtures()[0], Options{Package: "com.example.shop"})
	if !strings.Contains(f.Content, "\npackage com.example.shop;\n") {
		t.Fatal(f.Content)
	}
	f, _ = Generate("junit5", fixtures()[0], Options{Package: "com.class"})
	if strings.Contains(f.Content, "package") {
		t.Fatal(f.Content)
	}
}

func TestFromModel(t *testing.T) {
	call := "t3"
	m := &model.TestCase{Key: "A-TC-2", Title: "T", ExpectedResult: "E", Tags: []string{"x"}, Parameters: []string{"p"},
		DataRows: []model.DataRow{{Name: "r", Values: map[string]string{"p": "1"}}},
		Steps:    []model.TestStep{{Action: "a", Expected: "b", Data: "d"}, {CallTestID: &call, CallTestKey: "A-TC-3", CallTestTitle: "Other"}, {CallTestID: &call}}}
	got := FromModel(m, []string{"A-REQ-2", "A-REQ-1"}, "u")
	if got.Requirements[0] != "A-REQ-1" || got.Steps[1].Call != "A-TC-3 Other" || got.Steps[2].Call != "another test" || got.Rows[0].Values["p"] != "1" || got.URL != "u" || got.Expected != "E" {
		t.Fatalf("%+v", got)
	}
}

func TestMerge(t *testing.T) {
	tc := fixtures()[0]
	gen, _ := Generate("pytest", tc, Options{})
	if out, a := Merge(nil, gen); a != Created || out != gen.Content {
		t.Fatal(a)
	}
	if _, a := Merge([]byte(gen.Content), gen); a != Unchanged {
		t.Fatal(a)
	}
	// the test changed, the stub not: regenerate the whole file
	tc2 := tc
	tc2.Title = "Checkout with a stored card"
	tc2.Steps = append(tc2.Steps, Step{Action: "Log out"})
	gen2, _ := Generate("pytest", tc2, Options{})
	crlf := strings.ReplaceAll(gen.Content, "\n", "\r\n") // an editor's line ends do not count as a change
	if out, a := Merge([]byte(crlf), gen2); a != Regenerated || out != gen2.Content {
		t.Fatal(a, out)
	}
	// implemented: only the header changes
	impl := strings.Replace(gen.Content, `@pytest.mark.skip(reason="qtestframe stub: not implemented")`+"\n", "", 1) + "    assert True\n"
	out, a := Merge([]byte(impl), gen2)
	if a != HeaderUpdated || !strings.Contains(out, "assert True") || !strings.Contains(out, "# SHOP-TC-12: Checkout with a stored card") ||
		strings.Contains(out, "pytest.mark.skip") || strings.Contains(out, "4. TODO") {
		t.Fatal(a, out)
	}
	if out2, a := Merge([]byte(out), gen2); a != Unchanged || out2 != out {
		t.Fatal(a)
	}
	// text before the header block is kept
	pre := "# -*- coding: utf-8 -*-\n" + impl
	if out, a := Merge([]byte(pre), gen2); a != HeaderUpdated || !strings.HasPrefix(out, "# -*- coding: utf-8 -*-\n# qtestframe:begin SHOP-TC-12") {
		t.Fatal(a, out)
	}
	// no markers, another test's markers, a broken block: left alone
	for _, s := range []string{"def test_x():\n    pass\n", strings.Replace(gen.Content, "SHOP-TC-12 pytest", "SHOP-TC-99 pytest", 1),
		"# qtestframe:begin SHOP-TC-12 pytest body=1\n", "# qtestframe:begin"} {
		if out, a := Merge([]byte(s), gen); a != Kept || out != s {
			t.Fatal(a, s)
		}
	}
	if Body(gen.Content) == gen.Content || Body("x") != "x" {
		t.Fatal("Body")
	}
	// a block at the very end of a file
	if b := findBlock("# qtestframe:begin K go body=1\n# qtestframe:end"); b.end != len("# qtestframe:begin K go body=1\n# qtestframe:end") {
		t.Fatal(b)
	}
}

func TestScriptRef(t *testing.T) {
	if p, ok := ScriptRefPath(ScriptRef("tests/test_A_TC_1.py")); !ok || p != "tests/test_A_TC_1.py" {
		t.Fatal(p, ok)
	}
	for _, s := range []string{"echo hi", "qtestframe:stub a\nrm -rf /", ""} {
		if _, ok := ScriptRefPath(s); ok {
			t.Fatal(s)
		}
	}
}

func TestEscaping(t *testing.T) {
	if got := robotCell(""); got != "${EMPTY}" {
		t.Fatal(got)
	}
	if got := robotCell("=x ...[a] ${b} #c \\"); got != `\=x ...[a] \${b} \#c \\` {
		t.Fatal(got)
	}
	if got := robotCell("...more"); got != `\...more` {
		t.Fatal(got)
	}
	if got := pyString("a\"\\\n\t\r\x01\u2028"); got != `"a\"\\\n\t\r\u0001\u2028"` {
		t.Fatal(got)
	}
	if got := jsString("a'\\\n\t\r\x01"); got != `'a\'\\\n\t\r\u0001'` {
		t.Fatal(got)
	}
	if got := javaString("a\"\\\n\t\r\x01"); got != `"a\"\\\n\t\r\\u0001"` {
		t.Fatal(got)
	}
	if got := gherkinCell("a|b\\c\nd"); got != `a\|b\\c\nd` {
		t.Fatal(got)
	}
	if paramIdent(Framework{Language: "go"}, "1 type") != "p1Type" || paramIdent(Framework{Language: "python"}, "class") != "class_" || paramIdent(Framework{Language: "java"}, "€") != "value" {
		t.Fatal("paramIdent")
	}
	if validIdent("") || validIdent("func") || validIdent("1a") || !validIdent("a1") {
		t.Fatal("validIdent")
	}
}
