// Package prompt builds prompts for AI assistants from the requirements and
// tests of a software project: implement requirements, review them, design
// test cases of a test type, automate test cases, review tests, and plan the
// tests of one test type; and the solution prompts (solution.go):
// architecture, design, implementation plan and full solution from the
// project's solution preamble and the requirements of a scope. The prompts are plain Markdown, in English or
// Swedish, and contain everything the assistant needs: the project's
// technical context, the items with their criteria, steps and links, the
// instructions and the expected form of the answer.
package prompt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Prompt kinds.
const (
	Implement          = "implement"
	ReviewRequirements = "review_requirements"
	DesignTests        = "design_tests"
	AutomateTests      = "automate_tests"
	ReviewTests        = "review_tests"
	TestStrategy       = "test_strategy"
)

// Kinds lists the prompt kinds; the first three work on requirements, the
// next three on tests, the solution kinds (SolutionKinds) on requirements.
var Kinds = []string{Implement, ReviewRequirements, DesignTests, AutomateTests, ReviewTests, TestStrategy,
	Architecture, Design, ImplementationPlan, FullSolution}

// ForRequirements reports whether a kind takes requirements (else tests).
// TestStrategy takes both: the tests, and the requirements they verify.
func ForRequirements(kind string) bool {
	return kind == Implement || kind == ReviewRequirements || kind == DesignTests || IsSolution(kind)
}

// TestTypes are the test types (levels and quality characteristics) a
// prompt can be specialised for.
var TestTypes = []string{"unit", "integration", "system", "acceptance", "end_to_end", "api", "regression", "smoke", "performance", "security", "usability", "accessibility"}

// Languages of the prompts.
var Languages = []string{"en", "sv"}

// Criterion is an evaluation criterion of a requirement.
type Criterion struct {
	Description, Method, Threshold string
}

// Requirement is the prompt view of a requirement.
type Requirement struct {
	Key, Title, Description, Obligation, Type, Priority, Status string
	Source, Rationale                                           string
	Tags                                                        []string
	Criteria                                                    []Criterion
	// Relations are pre-formatted, e.g. "depends on SHOP-REQ-3 Payment".
	Relations []string
	// Tests that verify it, e.g. "SHOP-TC-1 Valid login".
	Tests []string
	// Solution prompts: the folder path, the keys of the requirements it
	// depends on, the linked risks, the standards it cites (catalogue refs
	// such as CWE ids and standard clauses) and its D3FEND / EMB3D ids.
	// Group is the top-level folder (for SplitFolder; "" derives it from
	// Folder).
	Folder, Group string
	DependsOn     []string
	Risks         []Risk
	Standards     []string
	D3fend, Emb3d []string
}

// Step is a manual step of a test.
type Step struct {
	Action, Expected, Data string
	// Calls names the test whose steps this step runs ("SHOP-TC-9 Log in").
	Calls string
}

// Test is the prompt view of a test case.
type Test struct {
	Key, Title, Description, Preconditions, ExecutionType string
	ScriptLanguage, Script, ExpectedResult, Priority      string
	Status, LastResult                                    string
	Tags                                                  []string
	Steps                                                 []Step
	// Requirements it verifies ("SHOP-REQ-1 Login").
	Requirements []string
	// DependsOn: tests that must pass first.
	DependsOn []string
}

// Input is everything a prompt is built from.
type Input struct {
	Kind, Lang                                  string
	ProjectKey, ProjectName, ProjectDescription string
	TechContext                                 string
	TestType                                    string
	Framework                                   string
	Notes                                       string
	Requirements                                []Requirement
	Tests                                       []Test
	// Solution prompts: the preamble (nil: none), the project summary,
	// compact mode (titles and criteria only), the split (SplitNone,
	// SplitFolder, SplitSize), the token budget of a part (0: none) and
	// the output formats (Formats).
	Preamble  *model.SolutionPreamble
	Summary   Summary
	Compact   bool
	Split     string
	MaxTokens int
	Formats   []string
}

// Result is a built prompt.
type Result struct {
	Kind  string `json:"kind"`
	Lang  string `json:"lang"`
	Title string `json:"title"`
	Text  string `json:"prompt"`
	// Items is the number of requirements and tests in the prompt.
	Items int `json:"items"`
	// Tokens estimates the size of the prompt (characters / 4).
	Tokens int `json:"tokens"`
	// Parts of a solution prompt, in order (one when it is not split);
	// Text is then all parts, separated by "---" lines.
	Parts []Part `json:"parts,omitempty"`
}

// Validate checks kind, language and test type.
func Validate(in *Input) error {
	if !slices.Contains(Kinds, in.Kind) {
		return fmt.Errorf("kind must be one of %s", strings.Join(Kinds, ", "))
	}
	if in.Lang == "" {
		in.Lang = "en"
	}
	if !slices.Contains(Languages, in.Lang) {
		return fmt.Errorf("lang must be one of %s", strings.Join(Languages, ", "))
	}
	if in.TestType != "" && !slices.Contains(TestTypes, in.TestType) {
		return fmt.Errorf("test_type must be one of %s", strings.Join(TestTypes, ", "))
	}
	if (in.Kind == DesignTests || in.Kind == TestStrategy) && in.TestType == "" {
		return fmt.Errorf("test_type is required for %s", in.Kind)
	}
	if IsSolution(in.Kind) {
		return validateSolution(in)
	}
	return nil
}

// Build creates the prompt. Call Validate first.
func Build(in Input) Result {
	if IsSolution(in.Kind) {
		return buildSolution(in)
	}
	tx := texts[in.Lang]
	if tx == nil {
		tx = texts["en"]
		in.Lang = "en"
	}
	b := &builder{tx: tx, lang: in.Lang}
	project := strings.TrimSpace(in.ProjectName)
	if in.ProjectKey != "" {
		project += " (" + in.ProjectKey + ")"
	}
	typeName := ""
	if in.TestType != "" {
		typeName = tx.testTypes[in.TestType].name
	}
	title := b.f("title."+in.Kind, project, typeName)
	b.line("# " + title)
	b.blank()
	b.para(tx.s["role."+in.Kind])
	b.para(b.f("task."+in.Kind, len(in.Requirements), len(in.Tests), typeName))

	// context
	b.line("## " + tx.s["h.context"])
	b.blank()
	b.line("- " + tx.s["project"] + ": " + project)
	if d := strings.TrimSpace(in.ProjectDescription); d != "" {
		b.line("- " + tx.s["description"] + ": " + oneLine(d))
	}
	if in.Framework != "" {
		b.line("- " + tx.s["framework"] + ": " + in.Framework)
	}
	b.blank()
	if tc := strings.TrimSpace(in.TechContext); tc != "" {
		b.line("### " + tx.s["h.tech"])
		b.blank()
		b.para(tc)
	} else {
		b.para("_" + tx.s["noTech"] + "_")
	}
	if in.TestType != "" {
		tt := tx.testTypes[in.TestType]
		b.line("### " + b.f("h.testType", tt.name))
		b.blank()
		b.para(tt.focus)
	}

	// items
	if len(in.Requirements) > 0 {
		b.line("## " + tx.s["h.requirements"])
		b.blank()
		b.para(tx.s["rfc2119"])
		for _, r := range in.Requirements {
			b.requirement(r)
		}
	}
	if len(in.Tests) > 0 {
		b.line("## " + tx.s["h.tests"])
		b.blank()
		for _, t := range in.Tests {
			b.test(t, in.Kind == AutomateTests)
		}
	}
	if len(in.Requirements) == 0 && len(in.Tests) == 0 {
		b.para("_" + tx.s["noItems"] + "_")
	}

	// instructions and answer
	b.line("## " + tx.s["h.instructions"])
	b.blank()
	for _, i := range tx.instructions[in.Kind] {
		b.line("- " + strings.ReplaceAll(i, "{framework}", or(in.Framework, tx.s["frameworkFallback"])))
	}
	b.blank()
	if n := strings.TrimSpace(in.Notes); n != "" {
		b.line("### " + tx.s["h.notes"])
		b.blank()
		b.para(n)
	}
	b.line("## " + tx.s["h.answer"])
	b.blank()
	for _, i := range tx.answers[in.Kind] {
		b.line(i)
	}
	text := strings.TrimRight(b.sb.String(), "\n") + "\n"
	return Result{Kind: in.Kind, Lang: in.Lang, Title: title, Text: text, Items: len(in.Requirements) + len(in.Tests), Tokens: EstimateTokens(text)}
}

func or(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

type builder struct {
	sb   strings.Builder
	tx   *lang
	lang string
}

func (b *builder) line(s string) { b.sb.WriteString(s + "\n") }
func (b *builder) blank()        { b.sb.WriteString("\n") }
func (b *builder) para(s string) {
	b.line(strings.TrimSpace(s))
	b.blank()
}

// f formats a text with positional arguments (%[1]v …), dropping unused ones.
func (b *builder) f(key string, args ...any) string {
	format := b.tx.s[key]
	if format == "" {
		return key
	}
	s := fmt.Sprintf(format, args...)
	if i := strings.Index(s, "%!(EXTRA"); i >= 0 {
		s = s[:i]
	}
	return s
}

func (b *builder) field(label, value string) {
	if v := strings.TrimSpace(value); v != "" {
		b.line("- **" + label + ":** " + v)
	}
}

func (b *builder) label(group, value string) string {
	if v, ok := b.tx.values[group+"."+value]; ok {
		return v
	}
	return strings.ReplaceAll(value, "_", " ")
}

func (b *builder) requirement(r Requirement) {
	tx := b.tx.s
	b.line(fmt.Sprintf("### %s %s", r.Key, r.Title))
	b.blank()
	var meta []string
	if r.Obligation != "" {
		meta = append(meta, strings.ToUpper(strings.ReplaceAll(r.Obligation, "_", " ")))
	}
	for _, v := range []struct{ group, value string }{{"type", r.Type}, {"priority", r.Priority}, {"status", r.Status}} {
		if v.value != "" {
			meta = append(meta, tx["f."+v.group]+": "+b.label(v.group, v.value))
		}
	}
	if len(meta) > 0 {
		b.line("_" + strings.Join(meta, " · ") + "_")
		b.blank()
	}
	if d := strings.TrimSpace(r.Description); d != "" {
		b.para(quote(d))
	}
	b.field(tx["f.rationale"], r.Rationale)
	b.field(tx["f.source"], r.Source)
	if len(r.Tags) > 0 {
		b.field(tx["f.tags"], strings.Join(r.Tags, ", "))
	}
	if len(r.Criteria) > 0 {
		b.line("- **" + tx["f.criteria"] + ":**")
		for _, c := range r.Criteria {
			s := "  - " + strings.TrimSpace(c.Description)
			if c.Method != "" {
				s += " (" + b.label("method", c.Method) + ")"
			}
			if t := strings.TrimSpace(c.Threshold); t != "" {
				s += " — " + tx["f.threshold"] + ": " + t
			}
			b.line(s)
		}
	}
	if len(r.Relations) > 0 {
		b.field(tx["f.relations"], strings.Join(r.Relations, "; "))
	}
	if len(r.Tests) > 0 {
		b.field(tx["f.verifiedBy"], strings.Join(r.Tests, "; "))
	}
	b.blank()
}

func (b *builder) test(t Test, withScript bool) {
	tx := b.tx.s
	b.line(fmt.Sprintf("### %s %s", t.Key, t.Title))
	b.blank()
	var meta []string
	if t.ExecutionType != "" {
		meta = append(meta, b.label("execution", t.ExecutionType))
	}
	for _, v := range []struct{ group, value string }{{"priority", t.Priority}, {"status", t.Status}, {"result", t.LastResult}} {
		if v.value != "" {
			meta = append(meta, tx["f."+v.group]+": "+b.label(v.group, v.value))
		}
	}
	if len(meta) > 0 {
		b.line("_" + strings.Join(meta, " · ") + "_")
		b.blank()
	}
	if d := strings.TrimSpace(t.Description); d != "" {
		b.para(quote(d))
	}
	if len(t.Requirements) > 0 {
		b.field(tx["f.verifies"], strings.Join(t.Requirements, "; "))
	}
	if len(t.DependsOn) > 0 {
		b.field(tx["f.dependsOn"], strings.Join(t.DependsOn, "; "))
	}
	if len(t.Tags) > 0 {
		b.field(tx["f.tags"], strings.Join(t.Tags, ", "))
	}
	b.field(tx["f.preconditions"], oneLine(t.Preconditions))
	if len(t.Steps) > 0 {
		b.line("- **" + tx["f.steps"] + ":**")
		for i, s := range t.Steps {
			if s.Calls != "" {
				b.line(fmt.Sprintf("  %d. %s %s", i+1, tx["f.calls"], s.Calls))
				continue
			}
			line := fmt.Sprintf("  %d. %s", i+1, oneLine(s.Action))
			if d := strings.TrimSpace(s.Data); d != "" {
				line += " — " + tx["f.data"] + ": " + oneLine(d)
			}
			if e := strings.TrimSpace(s.Expected); e != "" {
				line += " → " + tx["f.expected"] + ": " + oneLine(e)
			}
			b.line(line)
		}
	}
	b.field(tx["f.expectedResult"], oneLine(t.ExpectedResult))
	if withScript && strings.TrimSpace(t.Script) != "" {
		b.line("- **" + tx["f.script"] + " (" + or(t.ScriptLanguage, "sh") + "):**")
		b.blank()
		b.line("```" + t.ScriptLanguage)
		b.line(strings.TrimRight(t.Script, "\n"))
		b.line("```")
	}
	b.blank()
}

// quote prefixes every line with "> ".
func quote(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight("> "+l, " ")
	}
	return strings.Join(lines, "\n")
}

// RelationText phrases a requirement relation in a language, e.g.
// ("en", "depends_on", true) → "depends on"; outgoing=false gives the
// inverse ("is required by").
func RelationText(lang, typ string, outgoing bool) string {
	tx := texts[lang]
	if tx == nil {
		tx = texts["en"]
	}
	dir := "in"
	if outgoing {
		dir = "out"
	}
	if v, ok := tx.values["rel."+dir+"."+typ]; ok {
		return v
	}
	return strings.ReplaceAll(typ, "_", " ")
}

// TestTypeName is the display name of a test type.
func TestTypeName(lang, typ string) string {
	if tx := texts[lang]; tx != nil {
		if t, ok := tx.testTypes[typ]; ok {
			return t.name
		}
	}
	return typ
}
