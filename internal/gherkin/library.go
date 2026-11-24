package gherkin

import (
	"fmt"
	"sort"
	"strings"
)

// Definition is a step definition of a step library.
type Definition struct {
	ID      string
	Pattern string
	Kind    string
}

// Source is the Gherkin of a test.
type Source struct {
	TestID, TestKey, TestTitle string
	Text                       string
}

// Use is a step of a test that matches a definition (or that no
// definition matches).
type Use struct {
	TestID    string `json:"test_id"`
	TestKey   string `json:"test_key"`
	TestTitle string `json:"test_title"`
	Line      int    `json:"line"`
	Keyword   string `json:"keyword"`
	Text      string `json:"text"`
}

// Undefined is a step text no definition matches, with where it is used
// and a suggested Cucumber expression.
type Undefined struct {
	Text       string `json:"text"`
	Keyword    string `json:"keyword"`
	Suggestion string `json:"suggestion"`
	Uses       []Use  `json:"uses"`
}

// Analysis is the result of Analyse.
type Analysis struct {
	// Uses by definition id (every definition has an entry).
	Uses map[string][]Use
	// Undefined steps, the most used first.
	Undefined []Undefined
	// Invalid: definitions whose pattern does not compile (id → error).
	Invalid map[string]string
	// Unparsed: tests whose Gherkin has errors (test id → first error).
	Unparsed map[string]string
}

// CompileAll compiles definitions; invalid ones are reported by id.
func CompileAll(defs []Definition) (map[string]*Expression, map[string]string) {
	exprs := map[string]*Expression{}
	invalid := map[string]string{}
	for _, d := range defs {
		e, err := Compile(d.Pattern, d.Kind)
		if err != nil {
			invalid[d.ID] = err.Error()
			continue
		}
		exprs[d.ID] = e
	}
	return exprs, invalid
}

// Analyse matches every step of the sources against the definitions.
func Analyse(defs []Definition, sources []Source) Analysis {
	exprs, invalid := CompileAll(defs)
	a := Analysis{Uses: map[string][]Use{}, Invalid: invalid, Unparsed: map[string]string{}}
	for _, d := range defs {
		a.Uses[d.ID] = []Use{}
	}
	undef := map[string]*Undefined{}
	var order []string
	for _, src := range sources {
		doc, err := Parse(src.Text)
		if err != nil {
			if es, ok := err.(Errors); ok && len(es) > 0 {
				a.Unparsed[src.TestID] = es[0].Error()
			} else {
				a.Unparsed[src.TestID] = err.Error()
			}
		}
		for _, st := range doc.Feature.AllSteps() {
			u := Use{TestID: src.TestID, TestKey: src.TestKey, TestTitle: src.TestTitle, Line: st.Line, Keyword: st.Keyword, Text: st.Text}
			matched := false
			for _, d := range defs {
				e := exprs[d.ID]
				if e == nil {
					continue
				}
				if _, ok := e.Match(st.Text); ok {
					a.Uses[d.ID] = append(a.Uses[d.ID], u)
					matched = true
				}
			}
			if !matched && st.Text != "" {
				k := strings.ToLower(st.Text)
				if undef[k] == nil {
					undef[k] = &Undefined{Text: st.Text, Keyword: st.Keyword, Suggestion: Suggest(st.Text)}
					order = append(order, k)
				}
				undef[k].Uses = append(undef[k].Uses, u)
			}
		}
	}
	for _, k := range order {
		a.Undefined = append(a.Undefined, *undef[k])
	}
	sort.SliceStable(a.Undefined, func(i, j int) bool { return len(a.Undefined[i].Uses) > len(a.Undefined[j].Uses) })
	return a
}

// Rename rewrites the steps of src that match from so that they match
// to (a Cucumber expression with as many parameters): their parameters
// are kept. It returns the new text and the number of steps changed. The
// rest of the text is kept as it is.
func Rename(src string, from, to *Expression) (string, int, error) {
	if to.Kind != KindCucumber {
		return src, 0, fmt.Errorf("a step can only be renamed to a Cucumber expression")
	}
	if from.Params() != to.Params() {
		return src, 0, fmt.Errorf("the new expression has %d parameters, the old one %d", to.Params(), from.Params())
	}
	doc, err := Parse(src)
	if doc.Feature == nil {
		return src, 0, err
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	n := 0
	for _, st := range doc.Feature.AllSteps() {
		args, ok := from.Match(st.Text)
		if !ok {
			continue
		}
		text, err := to.Render(args)
		if err != nil {
			return src, 0, err
		}
		if text == st.Text {
			continue
		}
		l := lines[st.Line-1]
		i := strings.Index(l, st.Keyword)
		if i < 0 {
			continue
		}
		rest := l[i+len(st.Keyword):]
		sp := rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
		lines[st.Line-1] = l[:i+len(st.Keyword)] + sp + text
		n++
	}
	return strings.Join(lines, "\n"), n, nil
}
