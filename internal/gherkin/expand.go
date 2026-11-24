package gherkin

import (
	"strconv"
	"strings"
)

// Instance is one executable scenario: a Scenario, or one Examples row of
// a Scenario Outline, with the Background steps first and <placeholders>
// replaced by the row's values.
type Instance struct {
	// Name of the scenario (an outline's name with the row's values; the
	// row number and values are appended when the name has no placeholders).
	Name string `json:"name"`
	// ScenarioLine is the line of the scenario, RowLine the line of the
	// Examples row (0 for a plain scenario).
	ScenarioLine int `json:"scenario_line"`
	RowLine      int `json:"row_line,omitempty"`
	// Rule is the name of the Rule the scenario belongs to ("" none).
	Rule   string            `json:"rule,omitempty"`
	Tags   []string          `json:"tags"`
	Values map[string]string `json:"values,omitempty"`
	Steps  []InstanceStep    `json:"steps"`
}

// InstanceStep is a step of an Instance.
type InstanceStep struct {
	// Line of the step in the source (for mapping results back).
	Line    int      `json:"line"`
	Keyword string   `json:"keyword"`
	Type    StepType `json:"type"`
	Text    string   `json:"text"`
	// Argument is the doc string or the table (aligned) of the step.
	Argument string `json:"argument,omitempty"`
}

// Expand returns the executable scenarios of a document.
func Expand(doc *Document) []Instance {
	if doc == nil || doc.Feature == nil {
		return nil
	}
	f := doc.Feature
	var out []Instance
	add := func(rule *Rule, sc *Scenario) {
		var bg []*Step
		if f.Background != nil {
			bg = append(bg, f.Background.Steps...)
		}
		tags := append([]string{}, f.Tags...)
		ruleName := ""
		if rule != nil {
			ruleName = rule.Name
			tags = append(tags, rule.Tags...)
			if rule.Background != nil {
				bg = append(bg, rule.Background.Steps...)
			}
		}
		tags = append(tags, sc.Tags...)
		if !sc.Outline || len(sc.Examples) == 0 {
			out = append(out, Instance{Name: sc.Name, ScenarioLine: sc.Line, Rule: ruleName, Tags: tags,
				Steps: instanceSteps(append(append([]*Step{}, bg...), sc.Steps...), nil, nil)})
			return
		}
		n := 0
		for _, ex := range sc.Examples {
			if ex.Header == nil {
				continue
			}
			for _, row := range ex.Rows {
				n++
				values := map[string]string{}
				for i, h := range ex.Header.Cells {
					if i < len(row.Cells) {
						values[h] = row.Cells[i]
					}
				}
				name := substitute(sc.Name, values)
				if name == sc.Name {
					var vs []string
					for i, h := range ex.Header.Cells {
						if i < len(row.Cells) {
							vs = append(vs, h+"="+row.Cells[i])
						}
					}
					name = strings.TrimSpace(sc.Name + " #" + strconv.Itoa(n) + " (" + strings.Join(vs, ", ") + ")")
				}
				t := append(append([]string{}, tags...), ex.Tags...)
				out = append(out, Instance{Name: name, ScenarioLine: sc.Line, RowLine: row.Line, Rule: ruleName, Tags: t, Values: values,
					Steps: instanceSteps(bg, sc.Steps, values)})
			}
		}
	}
	for _, sc := range f.Scenarios {
		add(nil, sc)
	}
	for _, r := range f.Rules {
		for _, sc := range r.Scenarios {
			add(r, sc)
		}
	}
	return out
}

// instanceSteps resolves And/But to the previous type; values replace the
// placeholders of the scenario steps (not of the background).
func instanceSteps(bg, steps []*Step, values map[string]string) []InstanceStep {
	out := make([]InstanceStep, 0, len(bg)+len(steps))
	last := Context
	conv := func(st *Step, vals map[string]string) {
		t := st.Type
		if t == Conjunction {
			t = last
		}
		last = t
		is := InstanceStep{Line: st.Line, Keyword: st.Keyword, Type: t, Text: substitute(st.Text, vals)}
		switch {
		case st.DocString != nil:
			is.Argument = substitute(st.DocString.Content, vals)
		case len(st.Table) > 0:
			rows := make([]*Row, len(st.Table))
			for i, r := range st.Table {
				cells := make([]string, len(r.Cells))
				for j, c := range r.Cells {
					cells[j] = substitute(c, vals)
				}
				rows[i] = &Row{Line: r.Line, Cells: cells}
			}
			is.Argument = strings.Join(formatTable("", rows), "\n")
		}
		out = append(out, is)
	}
	for _, st := range bg {
		conv(st, nil)
	}
	for _, st := range steps {
		conv(st, values)
	}
	return out
}

func substitute(s string, values map[string]string) string {
	if len(values) == 0 || !strings.Contains(s, "<") {
		return s
	}
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := values[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}
