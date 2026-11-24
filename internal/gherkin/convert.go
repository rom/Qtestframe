package gherkin

import (
	"regexp"
	"strings"
)

// PlainTest is a test in the step-by-step format: preconditions, steps
// (action, expected result, data) and data rows for {{parameter}}
// placeholders.
type PlainTest struct {
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	Preconditions string      `json:"preconditions"`
	Steps         []PlainStep `json:"steps"`
	Parameters    []string    `json:"parameters"`
	Rows          []PlainRow  `json:"data_rows"`
	Tags          []string    `json:"tags,omitempty"`
}

// PlainStep is one step of a PlainTest. Call names a called test
// ("KEY title"; "" for an ordinary step).
type PlainStep struct {
	Action   string `json:"action"`
	Expected string `json:"expected"`
	Data     string `json:"data"`
	Call     string `json:"call,omitempty"`
}

// PlainRow is a data row.
type PlainRow struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

var paramRe = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// tagName makes a tag of a text: @ and no spaces.
func tagName(s string) string {
	s = strings.Join(strings.Fields(s), "-")
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "@") {
		s = "@" + s
	}
	return s
}

// FromPlain writes a step-by-step test as a feature with one scenario
// (a Scenario Outline with Examples when it has data rows), in a language
// ("en" or "sv"): preconditions become Given steps, actions When steps,
// expected results Then steps and step data doc strings.
func FromPlain(t PlainTest, lang string) string {
	d := DialectOf(lang)
	outline := len(t.Rows) > 0 && len(t.Parameters) > 0
	toOutline := func(s string) string {
		if !outline {
			return s
		}
		return paramRe.ReplaceAllString(s, "<$1>")
	}
	sc := &Scenario{Keyword: d.Scenario[0], Name: oneLine(t.Title), Outline: outline, Tags: []string{}}
	if outline {
		sc.Keyword = d.Outline[len(d.Outline)-1]
	}
	add := func(kw string, text string, data string) {
		lines := splitLines(toOutline(text))
		if len(lines) == 0 {
			return
		}
		st := &Step{Keyword: kw, Type: StepKeywordType(kw), Text: lines[0]}
		rest := lines[1:]
		if data = strings.TrimSpace(toOutline(data)); data != "" {
			rest = append(rest, splitRaw(data)...)
		}
		if len(rest) > 0 {
			st.DocString = &DocString{Delimiter: `"""`, Content: strings.Join(rest, "\n")}
		}
		sc.Steps = append(sc.Steps, st)
	}
	for i, l := range splitLines(t.Preconditions) {
		kw := d.Given[0]
		if i > 0 {
			kw = d.And[0]
		}
		add(kw, l, "")
	}
	for _, st := range t.Steps {
		action := st.Action
		if st.Call != "" {
			action = "the steps of " + st.Call + " are done"
			if d.Code == "sv" {
				action = "stegen i " + st.Call + " är gjorda"
			}
		}
		if strings.TrimSpace(action) != "" {
			add(d.When[0], action, st.Data)
		}
		if strings.TrimSpace(st.Expected) != "" {
			add(d.Then[0], st.Expected, "")
		}
	}
	if outline {
		ex := &Examples{Keyword: d.Examples[0], Tags: []string{}}
		ex.Header = &Row{Cells: append([]string{}, t.Parameters...)}
		for _, r := range t.Rows {
			row := &Row{}
			for _, p := range t.Parameters {
				row.Cells = append(row.Cells, r.Values[p])
			}
			ex.Rows = append(ex.Rows, row)
		}
		sc.Examples = []*Examples{ex}
	}
	var tags []string
	for _, tg := range t.Tags {
		if tg = tagName(tg); tg != "" {
			tags = append(tags, tg)
		}
	}
	f := &Feature{Keyword: d.Feature[0], Name: oneLine(t.Title), Description: strings.TrimSpace(t.Description), Tags: tags, Scenarios: []*Scenario{sc}}
	return Format(&Document{Language: d.Code, Feature: f})
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// splitLines returns the non-empty lines, trimmed.
func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitRaw(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// ToPlain converts a feature to the step-by-step format (best effort):
// Given steps before the first When become preconditions, each When step a
// step, Then steps its expected result, doc strings and tables its data.
// Several scenarios are written one after the other, each first step
// prefixed with "[scenario name]". The first Examples table gives the
// parameters and data rows; placeholders become {{name}}. Warnings say
// what could not be kept.
func ToPlain(doc *Document) (PlainTest, []string) {
	var out PlainTest
	out.Steps = []PlainStep{}
	out.Parameters = []string{}
	out.Rows = []PlainRow{}
	var warn []string
	if doc == nil || doc.Feature == nil {
		return out, warn
	}
	f := doc.Feature
	out.Title = f.Name
	out.Description = f.Description
	for _, t := range f.Tags {
		out.Tags = append(out.Tags, strings.TrimPrefix(t, "@"))
	}
	var pre []string
	fromOutline := func(s string) string { return placeholderRe.ReplaceAllString(s, "{{$1}}") }
	stepText := func(st *Step) string { return fromOutline(st.Keyword + " " + st.Text) }
	argument := func(st *Step) string { return fromOutline(st.Argument()) }
	if f.Background != nil {
		for _, st := range f.Background.Steps {
			pre = append(pre, stepText(st))
		}
	}
	scenarios := f.AllScenarios()
	if len(f.Rules) > 0 {
		warn = append(warn, "rules are not kept; their scenarios are written in order")
		for _, r := range f.Rules {
			if r.Background != nil && len(r.Background.Steps) > 0 {
				warn = append(warn, "the background of rule \""+r.Name+"\" is not kept")
			}
		}
	}
	examplesSeen := false
	for si, sc := range scenarios {
		if len(scenarios) == 1 && out.Title == "" {
			out.Title = sc.Name
		}
		prefix := ""
		if len(scenarios) > 1 {
			prefix = "[" + sc.Name + "] "
		}
		var cur *PlainStep
		last := Context
		started := false
		for _, st := range sc.Steps {
			t := st.Type
			if t == Conjunction {
				t = last
			}
			last = t
			switch {
			case t == Context && !started && si == 0:
				pre = append(pre, stepText(st))
				if a := argument(st); a != "" {
					pre = append(pre, a)
				}
			case t == Outcome && cur != nil:
				if cur.Expected != "" {
					cur.Expected += "\n"
				}
				cur.Expected += fromOutline(st.Text)
				if a := argument(st); a != "" {
					cur.Expected += "\n" + a
				}
			default:
				text := fromOutline(st.Text)
				if t != Action {
					text = stepText(st)
				}
				if !started {
					text = prefix + text
				}
				out.Steps = append(out.Steps, PlainStep{Action: text, Data: argument(st)})
				cur = &out.Steps[len(out.Steps)-1]
				if t == Outcome {
					cur.Action, cur.Expected = prefix, fromOutline(st.Text)
					cur.Action = strings.TrimSpace(cur.Action)
				}
			}
			if t != Context {
				started = true
			}
		}
		for ei, ex := range sc.Examples {
			if ex.Header == nil {
				continue
			}
			if examplesSeen || ei > 0 {
				warn = append(warn, "only the first Examples table becomes test data")
				continue
			}
			examplesSeen = true
			out.Parameters = append(out.Parameters, ex.Header.Cells...)
			for _, r := range ex.Rows {
				row := PlainRow{Values: map[string]string{}}
				for i, h := range ex.Header.Cells {
					if i < len(r.Cells) {
						row.Values[h] = r.Cells[i]
					}
				}
				out.Rows = append(out.Rows, row)
			}
		}
	}
	out.Preconditions = strings.Join(pre, "\n")
	return out, dedupe(warn)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
