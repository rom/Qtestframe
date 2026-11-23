package prompt

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

func solutionSample(kind, lang string) Input {
	pre := model.SolutionPreamble{
		Languages:         "Go 1.26, TypeScript 5",
		Frameworks:        "net/http, pgx, Vue 3",
		ArchitectureStyle: "modular_monolith",
		Databases:         "PostgreSQL 16, schema owned by the app,\nmigrations embedded in the binary",
		Security:          "OIDC sign-in, RBAC",
		OutOfScope:        "Native mobile apps",
	}
	return Input{
		Kind: kind, Lang: lang, ProjectKey: "SHOP", ProjectName: "Web shop", ProjectDescription: "An online shop",
		TechContext: "Monorepo.", Notes: "Prefer boring technology.", Preamble: &pre, Formats: DefaultFormats(kind),
		Summary: Summary{Scope: "", Total: 5, InScope: 3, Tests: 4, Folders: []Count{{"Checkout", 2}, {"Accounts", 1}},
			Releases: []string{"1.0 (planned)"}, Variants: []string{"EU"}},
		Requirements: []Requirement{
			{Key: "SHOP-REQ-1", Title: "Login", Description: "The user MUST be able to log in.", Obligation: "must", Type: "security",
				Priority: "high", Status: "approved", Folder: "Accounts", Rationale: "Protect accounts",
				Criteria:  []Criterion{{Description: "Valid login succeeds", Method: "test", Threshold: "< 2 s"}},
				Relations: []string{RelationText(lang, "conflicts_with", true) + " SHOP-REQ-3 Guest checkout"},
				Risks:     []Risk{{Key: "SHOP-RISK-1", Title: "Credential stuffing", Level: "high", Mitigation: "Rate limiting and MFA", Attack: []string{"T1110.004"}}},
				Tests:     []string{"SHOP-TC-1 Valid login"}, Standards: []string{"CWE-307", "OWASP ASVS V2.2"}, D3fend: []string{"D3-MFA"}},
			{Key: "SHOP-REQ-2", Title: "Pay by card", Description: "The shop SHOULD accept cards.", Obligation: "should", Type: "functional",
				Priority: "medium", Status: "draft", Folder: "Checkout/Payment", DependsOn: []string{"SHOP-REQ-1"},
				Relations: []string{RelationText(lang, "depends_on", true) + " SHOP-REQ-1 Login"}},
			{Key: "SHOP-REQ-3", Title: "Guest checkout", Obligation: "may", Folder: "Checkout", Emb3d: []string{"MID-001"},
				Criteria: []Criterion{{Description: "No account needed"}}},
		},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/prompt -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file (run go test ./internal/prompt -update and review the diff):\n%s", name, got)
	}
}

// Golden tests: every solution kind in both languages.
func TestSolutionGolden(t *testing.T) {
	for _, lang := range Languages {
		for _, kind := range SolutionKinds {
			in := solutionSample(kind, lang)
			if err := Validate(&in); err != nil {
				t.Fatal(err)
			}
			res := Build(in)
			if res.Items != 3 || len(res.Parts) != 1 || res.Parts[0].Text != res.Text || res.Tokens != EstimateTokens(res.Text) || res.Tokens == 0 {
				t.Fatalf("%s/%s: %+v", lang, kind, res)
			}
			if strings.Contains(res.Text, "%!") || strings.Contains(res.Text, "EXTRA") {
				t.Errorf("%s/%s: formatting leftovers", lang, kind)
			}
			golden(t, "solution_"+kind+"_"+lang+".md", res.Text)
		}
	}
}

func TestSolutionContent(t *testing.T) {
	res := Build(solutionSample(FullSolution, "en"))
	for _, want := range []string{
		"# Solution for Web shop (SHOP)", "## Solution preamble", "**Architecture style:** modular monolith",
		"**Databases (described; no credentials):**\n  PostgreSQL 16", "### Technical context", "Scope: the whole project — 3 requirements (of 5 in the project)",
		"Requirements by obligation: MUST 1, SHOULD 1, MAY 1", "Folders: Checkout (2), Accounts (1)", "Releases: 1.0 (planned)", "Variants: EU",
		"Linked risks: 1 · verifying tests: 1 (of 4 tests in the project)", "SHOP-RISK-1 Credential stuffing (level high; mitigation: Rate limiting and MFA; ATT&CK T1110.004)",
		"CWE-307; OWASP ASVS V2.2; D3FEND D3-MFA", "EMB3D MID-001", "## Dependency order", "SHOP-REQ-1 → SHOP-REQ-2", "### Architecture", "### Design",
		"### Implementation plan", "C4Context", "OpenAPI 3.1", "SQL DDL", "Prefer boring technology.", "| Requirement | Covered by", "conflicts with SHOP-REQ-3",
	} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("missing %q in\n%s", want, res.Text)
		}
	}
	// architecture: no dependency order, its own formats only
	arch := Build(solutionSample(Architecture, "en")).Text
	if strings.Contains(arch, "## Dependency order") || strings.Contains(arch, "OpenAPI") || !strings.Contains(arch, "Mermaid") {
		t.Error(arch)
	}
	// no preamble
	in := solutionSample(Design, "sv")
	in.Preamble, in.TechContext = nil, ""
	if txt := Build(in).Text; !strings.Contains(txt, "Ingen lösningsbeskrivning angavs") || strings.Contains(txt, "### Teknisk") {
		t.Error(txt)
	}
}

func TestSolutionCompact(t *testing.T) {
	in := solutionSample(Design, "en")
	full := Build(in)
	in.Compact = true
	c := Build(in)
	if c.Tokens >= full.Tokens {
		t.Fatalf("compact %d ≥ full %d", c.Tokens, full.Tokens)
	}
	for _, gone := range []string{"> The user MUST", "Protect accounts", "SHOP-RISK-1", "Verified by", "Folder: Accounts", "(test)"} {
		if strings.Contains(c.Text, gone) {
			t.Errorf("compact still has %q", gone)
		}
	}
	for _, kept := range []string{"### SHOP-REQ-1 Login", "_MUST · Priority: high_", "Valid login succeeds — threshold: < 2 s", "Compact mode"} {
		if !strings.Contains(c.Text, kept) {
			t.Errorf("compact lacks %q\n%s", kept, c.Text)
		}
	}
}

func TestSolutionSplitFolder(t *testing.T) {
	in := solutionSample(ImplementationPlan, "en")
	in.Split = SplitFolder
	if err := Validate(&in); err != nil {
		t.Fatal(err)
	}
	res := Build(in)
	if len(res.Parts) != 2 || res.Parts[0].Label != "Accounts" || res.Parts[1].Label != "Checkout" || res.Parts[0].Items != 1 || res.Parts[1].Items != 2 {
		t.Fatalf("%+v", res.Parts)
	}
	p1, p2 := res.Parts[0].Text, res.Parts[1].Text
	if !strings.HasPrefix(p1, "# Implementation plan for Web shop (SHOP) — part 1 of 2\n") || res.Parts[1].Title != "Implementation plan for Web shop (SHOP) — part 2 of 2: Checkout" {
		t.Fatal(p1[:80], res.Parts[1].Title)
	}
	// both parts carry the preamble; the continuation instructions differ
	for _, p := range []string{p1, p2} {
		if !strings.Contains(p, "**Implementation languages and versions:** Go 1.26") || !strings.Contains(p, "## Continuation") {
			t.Error(p)
		}
	}
	if !strings.Contains(p1, "part 1 of 2 of one prompt; the requirements are split by top-level folder") || strings.Contains(p1, "SHOP-REQ-2 Pay") {
		t.Error(p1)
	}
	if !strings.Contains(p2, "This is the last part (2 of 2); the earlier parts covered Accounts") || !strings.Contains(p2, "This part: 2 of 2, with 2 requirements") {
		t.Error(p2)
	}
	// the dependency on a requirement of another part is not ordered here
	if strings.Contains(p2, "## Dependency order") {
		t.Error("cross-part dependency ordered")
	}
	if res.Text != p1+"\n---\n\n"+p2 || res.Tokens != EstimateTokens(res.Text) {
		t.Error("joined text")
	}
	golden(t, "solution_split_folder_sv.md", func() string { in.Lang = "sv"; return Build(in).Text }())
}

func TestSolutionSplitSize(t *testing.T) {
	in := solutionSample(Architecture, "en")
	for i := 4; i <= 30; i++ {
		in.Requirements = append(in.Requirements, Requirement{Key: "SHOP-REQ-" + itoa(i), Title: "Requirement " + itoa(i), Obligation: "must",
			Description: strings.Repeat("The system MUST do a thing that takes some words to describe. ", 8), Folder: "Checkout"})
	}
	in.Split = SplitSize
	if err := Validate(&in); err != nil || in.MaxTokens != DefaultPartTokens {
		t.Fatal(err, in.MaxTokens)
	}
	in.MaxTokens = 2500
	res := Build(in)
	if len(res.Parts) < 3 {
		t.Fatalf("%d parts", len(res.Parts))
	}
	total := 0
	for i, p := range res.Parts {
		total += p.Items
		if p.Tokens > in.MaxTokens+200 {
			t.Errorf("part %d has %d tokens", i+1, p.Tokens)
		}
		if !strings.HasPrefix(p.Label, "SHOP-REQ-") || !strings.Contains(p.Text, "## Continuation") {
			t.Errorf("part %d: %q", i+1, p.Label)
		}
	}
	if total != len(in.Requirements) {
		t.Errorf("%d of %d requirements in the parts", total, len(in.Requirements))
	}
	if mid := res.Parts[1].Text; !strings.Contains(mid, "This is part 2 of") || !strings.Contains(mid, "Continue the same answer as the earlier parts (SHOP-REQ-1 – ") {
		t.Error(mid)
	}
	// split by folder with a budget: big folders are split further and numbered
	in.Split = SplitFolder
	res = Build(in)
	labels := []string{}
	for _, p := range res.Parts {
		labels = append(labels, p.Label)
	}
	if labels[0] != "Accounts" || labels[1] != "Checkout (1)" || labels[2] != "Checkout (2)" {
		t.Errorf("%v", labels)
	}
	// one requirement larger than the budget still gets a part of its own
	in.MaxTokens = 1000
	in.Requirements[0].Description = strings.Repeat("x ", 4000)
	if res = Build(in); res.Parts[0].Items != 1 {
		t.Errorf("%+v", res.Parts[0])
	}
}

func itoa(i int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+i/10)) + string(rune('0'+i%10)))
}

func TestSolutionValidate(t *testing.T) {
	for _, c := range []struct {
		in   Input
		want string
	}{
		{Input{Kind: Design, Formats: []string{"pdf"}}, "formats must be"},
		{Input{Kind: Design, Split: "chapter"}, "split must be"},
		{Input{Kind: Design, MaxTokens: 10}, "max_tokens must be"},
	} {
		if err := Validate(&c.in); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c.in, err)
		}
	}
	in := Input{Kind: FullSolution}
	if err := Validate(&in); err != nil || in.Split != SplitNone || !ForRequirements(FullSolution) {
		t.Fatal(err)
	}
	if len(DefaultFormats(ImplementationPlan)) != 0 || len(DefaultFormats(FullSolution)) != 4 {
		t.Error("default formats")
	}
	if EstimateTokens("") != 0 || EstimateTokens("abcd") != 1 || EstimateTokens("abcde") != 2 || EstimateTokens("åäöå") != 1 {
		t.Error("EstimateTokens")
	}
	// an empty prompt still renders
	res := Build(Input{Kind: Architecture, Lang: "xx", ProjectName: "X"})
	if res.Lang != "en" || !strings.Contains(res.Text, "No items were selected") || len(res.Parts) != 1 {
		t.Error(res.Text)
	}
	if topFolder("A/B/C") != "A" || topFolder(" A ") != "A" {
		t.Error("topFolder")
	}
}

func TestSolutionTextsComplete(t *testing.T) {
	en, sv := solTexts["en"], solTexts["sv"]
	for k := range en.s {
		if sv.s[k] == "" {
			t.Errorf("sv lacks %q", k)
		}
	}
	for k := range sv.s {
		if en.s[k] == "" {
			t.Errorf("en lacks %q", k)
		}
	}
	for _, f := range append([]string{"architecture_style"}, model.PreambleFields...) {
		if en.labels[f] == "" || sv.labels[f] == "" || PreambleLabel("sv", f) != sv.labels[f] || PreambleLabel("xx", f) != en.labels[f] {
			t.Errorf("label %s", f)
		}
	}
	for _, st := range model.ArchitectureStyles {
		if en.styles[st] == "" || sv.styles[st] == "" {
			t.Errorf("style %s", st)
		}
	}
	for _, k := range SolutionKinds {
		if len(en.answers[k]) == 0 || len(sv.answers[k]) != len(en.answers[k]) || len(sv.instructions[k]) != len(en.instructions[k]) {
			t.Errorf("kind %s", k)
		}
	}
	for _, f := range append([]string{"markdown"}, Formats...) {
		if en.formats[f] == "" || sv.formats[f] == "" {
			t.Errorf("format %s", f)
		}
	}
}

func TestDependencyOrder(t *testing.T) {
	reqs := []Requirement{{Key: "A", DependsOn: []string{"C"}}, {Key: "B"}, {Key: "C", DependsOn: []string{"X"}}, {Key: "D", DependsOn: []string{"E"}}, {Key: "E", DependsOn: []string{"D"}}}
	order, cycles := dependencyOrder(reqs)
	if strings.Join(order, ",") != "C,A,D,E" || len(cycles) != 1 || strings.Join(cycles[0], ",") != "D,E" {
		t.Fatal(order, cycles)
	}
	if o, c := dependencyOrder(reqs[1:2]); o != nil || c != nil {
		t.Error(o, c)
	}
	in := solutionSample(ImplementationPlan, "en")
	in.Requirements = []Requirement{{Key: "D", Title: "d", DependsOn: []string{"E"}}, {Key: "E", Title: "e", DependsOn: []string{"D"}}}
	if txt := Build(in).Text; !strings.Contains(txt, "- Dependency cycle, decide the order: D, E") {
		t.Error(txt)
	}
}
