// Package gherkin parses, formats and expands Gherkin feature files (the
// BDD format of Cucumber) in English and Swedish, matches step texts
// against step definitions (Cucumber expressions or regular expressions)
// and converts between Gherkin and the step-by-step format of Qtestframe
// tests.
//
// The parser is deliberately small: it reads one Feature per document with
// tags, a description, an optional Background, Scenarios, Scenario
// Outlines with Examples tables, Rules, doc strings and data tables. Every
// error carries the line (and column) it was found on, so an editor can
// show it inline.
package gherkin

import "strings"

// StepType is what a step does: it sets up a context (Given), acts (When)
// or checks an outcome (Then). And, But and * continue the previous type.
type StepType string

const (
	Context     StepType = "context"
	Action      StepType = "action"
	Outcome     StepType = "outcome"
	Conjunction StepType = "conjunction"
)

// Document is a parsed feature file.
type Document struct {
	// Language is "en" or "sv": the "# language:" header, or the language
	// of the Feature keyword.
	Language string   `json:"language"`
	Feature  *Feature `json:"feature"`
}

// Feature is the top-level element of a feature file.
type Feature struct {
	Line        int         `json:"line"`
	Tags        []string    `json:"tags"`
	Keyword     string      `json:"keyword"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Background  *Background `json:"background,omitempty"`
	Scenarios   []*Scenario `json:"scenarios"`
	Rules       []*Rule     `json:"rules,omitempty"`
}

// Rule groups scenarios under a business rule (with its own Background).
type Rule struct {
	Line        int         `json:"line"`
	Tags        []string    `json:"tags"`
	Keyword     string      `json:"keyword"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Background  *Background `json:"background,omitempty"`
	Scenarios   []*Scenario `json:"scenarios"`
}

// Background holds steps that run before every scenario.
type Background struct {
	Line        int     `json:"line"`
	Keyword     string  `json:"keyword"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Steps       []*Step `json:"steps"`
}

// Scenario is a Scenario (Example) or a Scenario Outline (Template).
type Scenario struct {
	Line        int         `json:"line"`
	Tags        []string    `json:"tags"`
	Keyword     string      `json:"keyword"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Outline     bool        `json:"outline"`
	Steps       []*Step     `json:"steps"`
	Examples    []*Examples `json:"examples,omitempty"`
}

// Examples is a table of values for a Scenario Outline: one scenario per
// row, <header> placeholders replaced by the row's cells.
type Examples struct {
	Line        int      `json:"line"`
	Tags        []string `json:"tags"`
	Keyword     string   `json:"keyword"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Header      *Row     `json:"header"`
	Rows        []*Row   `json:"rows"`
}

// Row is a row of a table.
type Row struct {
	Line  int      `json:"line"`
	Cells []string `json:"cells"`
}

// Step is one Given/When/Then/And/But/* step.
type Step struct {
	Line int `json:"line"`
	// Keyword as written, without the trailing space ("Given", "Och").
	Keyword   string     `json:"keyword"`
	Type      StepType   `json:"type"`
	Text      string     `json:"text"`
	DocString *DocString `json:"doc_string,omitempty"`
	Table     []*Row     `json:"table,omitempty"`
}

// DocString is a multi-line argument of a step.
type DocString struct {
	Line      int    `json:"line"`
	Delimiter string `json:"delimiter"`
	MediaType string `json:"media_type,omitempty"`
	Content   string `json:"content"`
}

// AllScenarios returns the scenarios of the feature and of its rules in
// document order.
func (f *Feature) AllScenarios() []*Scenario {
	if f == nil {
		return nil
	}
	out := append([]*Scenario(nil), f.Scenarios...)
	for _, r := range f.Rules {
		out = append(out, r.Scenarios...)
	}
	return out
}

// AllSteps returns every step of the feature (backgrounds, scenarios and
// rules) in document order.
func (f *Feature) AllSteps() []*Step {
	if f == nil {
		return nil
	}
	var out []*Step
	if f.Background != nil {
		out = append(out, f.Background.Steps...)
	}
	for _, sc := range f.Scenarios {
		out = append(out, sc.Steps...)
	}
	for _, r := range f.Rules {
		if r.Background != nil {
			out = append(out, r.Background.Steps...)
		}
		for _, sc := range r.Scenarios {
			out = append(out, sc.Steps...)
		}
	}
	return out
}

// Argument renders the doc string or table of a step as text ("" none).
func (s *Step) Argument() string {
	if s.DocString != nil {
		return s.DocString.Content
	}
	if len(s.Table) > 0 {
		return strings.Join(formatTable("", s.Table), "\n")
	}
	return ""
}
