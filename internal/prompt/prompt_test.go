package prompt

import (
	"strings"
	"testing"
)

func sample(kind, lang string) Input {
	return Input{
		Kind: kind, Lang: lang, ProjectKey: "SHOP", ProjectName: "Web shop", ProjectDescription: "An online\nshop",
		TechContext: "Go 1.26 backend, Vue 3 client, PostgreSQL.", TestType: "integration", Framework: "Go testing + testcontainers",
		Notes: "Use table-driven tests.",
		Requirements: []Requirement{{
			Key: "SHOP-REQ-1", Title: "Login", Description: "The user MUST be able to log in.\nWith e-mail.", Obligation: "must", Type: "functional",
			Priority: "high", Status: "approved", Source: "Customer", Rationale: "Security", Tags: []string{"auth"},
			Criteria:  []Criterion{{Description: "Valid login succeeds", Method: "test", Threshold: "< 2 s"}},
			Relations: []string{RelationText(lang, "depends_on", true) + " SHOP-REQ-3 Accounts"},
			Tests:     []string{"SHOP-TC-1 Valid login"},
		}},
		Tests: []Test{{
			Key: "SHOP-TC-1", Title: "Valid login", Description: "Happy path", Preconditions: "A user exists", ExecutionType: "manual",
			Priority: "high", Status: "ready", LastResult: "passed", Tags: []string{"smoke"}, Script: "exit 0", ScriptLanguage: "sh",
			Steps:        []Step{{Action: "Open the login page", Expected: "Form shown"}, {Action: "Sign in", Data: "alice", Expected: "Dashboard"}, {Calls: "SHOP-TC-9 Log out"}},
			Requirements: []string{"SHOP-REQ-1 Login"}, DependsOn: []string{"SHOP-TC-0 Setup"}, ExpectedResult: "Signed in",
		}},
	}
}

func TestValidate(t *testing.T) {
	for _, c := range []struct {
		in   Input
		want string
	}{
		{Input{Kind: "bogus"}, "kind must be"},
		{Input{Kind: Implement, Lang: "de"}, "lang must be"},
		{Input{Kind: DesignTests}, "test_type is required"},
		{Input{Kind: TestStrategy, TestType: "nope"}, "test_type must be"},
	} {
		if err := Validate(&c.in); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c.in, err)
		}
	}
	in := Input{Kind: Implement}
	if err := Validate(&in); err != nil || in.Lang != "en" {
		t.Fatal(err, in.Lang)
	}
}

func TestBuildEveryKind(t *testing.T) {
	for _, lang := range Languages {
		for _, kind := range Kinds {
			if IsSolution(kind) {
				continue // solution_test.go
			}
			in := sample(kind, lang)
			if err := Validate(&in); err != nil {
				t.Fatal(err)
			}
			res := Build(in)
			txt := res.Text
			if res.Items != 2 || res.Kind != kind || !strings.HasPrefix(txt, "# "+res.Title+"\n") {
				t.Fatalf("%s/%s: %+v", lang, kind, res)
			}
			for _, want := range []string{"Web shop (SHOP)", "Go 1.26 backend", "SHOP-REQ-1 Login", "> The user MUST be able to log in.\n> With e-mail.",
				"Valid login succeeds", "< 2 s", "SHOP-REQ-3 Accounts", "SHOP-TC-1 Valid login", "Open the login page", "alice", "SHOP-TC-9 Log out",
				"SHOP-TC-0 Setup", "Use table-driven tests.", "An online shop"} {
				if !strings.Contains(txt, want) {
					t.Errorf("%s/%s: missing %q in\n%s", lang, kind, want, txt)
				}
			}
			if strings.Contains(txt, "%!") || strings.Contains(txt, "{framework}") {
				t.Errorf("%s/%s: formatting leftovers\n%s", lang, kind, txt)
			}
			// the script is only included when automating
			if got := strings.Contains(txt, "exit 0"); got != (kind == AutomateTests) {
				t.Errorf("%s/%s: script included = %v", lang, kind, got)
			}
			if len(english.instructions[kind]) == 0 || len(swedish.instructions[kind]) != len(english.instructions[kind]) || len(swedish.answers[kind]) == 0 {
				t.Errorf("%s: instruction texts missing", kind)
			}
		}
	}
}

func TestTextsComplete(t *testing.T) {
	for key := range english.s {
		if swedish.s[key] == "" {
			t.Errorf("sv lacks %q", key)
		}
	}
	for key := range swedish.s {
		if english.s[key] == "" {
			t.Errorf("en lacks %q", key)
		}
	}
	for key := range english.values {
		if swedish.values[key] == "" {
			t.Errorf("sv lacks value %q", key)
		}
	}
	for _, tt := range TestTypes {
		for _, l := range []*lang{english, swedish} {
			if l.testTypes[tt].name == "" || l.testTypes[tt].focus == "" {
				t.Errorf("test type %s incomplete", tt)
			}
		}
	}
}

func TestDetails(t *testing.T) {
	in := sample(DesignTests, "sv")
	in.TestType = "security"
	res := Build(in)
	if !strings.Contains(res.Title, "säkerhetstester") || !strings.Contains(res.Text, "OWASP") || !strings.Contains(res.Text, "beror på SHOP-REQ-3") {
		t.Fatal(res.Text)
	}
	empty := Build(Input{Kind: AutomateTests, Lang: "en", ProjectName: "X"})
	if !strings.Contains(empty.Text, "No items were selected") || !strings.Contains(empty.Text, "No technical context") ||
		!strings.Contains(empty.Text, "the test framework the project already uses") || empty.Items != 0 {
		t.Fatal(empty.Text)
	}
	if Build(Input{Kind: Implement, Lang: "xx"}).Lang != "en" {
		t.Error("unknown language falls back to English")
	}
	if RelationText("sv", "depends_on", false) != "krävs av" || RelationText("xx", "refines", true) != "refines" || RelationText("en", "new_type", true) != "new type" {
		t.Error("relation texts")
	}
	if TestTypeName("sv", "unit") != "enhetstester" || TestTypeName("xx", "unit") != "unit" {
		t.Error("test type names")
	}
	if ForRequirements(AutomateTests) || !ForRequirements(DesignTests) {
		t.Error("ForRequirements")
	}
	b := &builder{tx: english}
	if b.f("missing.key") != "missing.key" || b.label("status", "brand_new") != "brand new" {
		t.Error("fallbacks")
	}
}
