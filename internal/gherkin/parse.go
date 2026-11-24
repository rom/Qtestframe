package gherkin

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Error is a problem in a feature file, at a line (1-based) and column
// (1-based, in characters; 0 when it is about the whole line).
type Error struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Column > 0 {
		return fmt.Sprintf("line %d:%d: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// Errors are all problems of a document, in line order.
type Errors []*Error

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

// MaxSource bounds the size of a feature file.
const MaxSource = 1 << 20

var (
	languageRe    = regexp.MustCompile(`^#\s*language\s*:\s*([A-Za-z_-]+)\s*$`)
	placeholderRe = regexp.MustCompile(`<([^<>]+)>`)
)

type parser struct {
	lines []string
	errs  Errors
	doc   *Document

	tags     []string
	tagsLine int

	// what descriptions and steps attach to
	rule       *Rule
	background *Background
	scenario   *Scenario
	examples   *Examples
	step       *Step
	// set while free text after a header is its description
	desc *string
	// lastType resolves And/But to a type
	lastType StepType
}

func (p *parser) errorf(line, col int, format string, args ...any) {
	p.errs = append(p.errs, &Error{Line: line, Column: col, Message: fmt.Sprintf(format, args...)})
}

// Parse reads a feature file. On errors it returns the document as far as
// it could be read, with Errors.
func Parse(src string) (*Document, error) {
	p := &parser{doc: &Document{Language: ""}}
	if len(src) > MaxSource {
		return p.doc, Errors{{Line: 1, Message: fmt.Sprintf("the feature file is larger than %d bytes", MaxSource)}}
	}
	if !utf8.ValidString(src) {
		return p.doc, Errors{{Line: 1, Message: "the feature file is not valid UTF-8"}}
	}
	src = strings.TrimPrefix(src, "\ufeff")
	p.lines = strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	p.run()
	if p.doc.Language == "" {
		p.doc.Language = "en"
	}
	if len(p.errs) > 0 {
		return p.doc, p.errs
	}
	return p.doc, nil
}

// indentOf returns the number of leading whitespace characters.
func indentOf(s string) int {
	return utf8.RuneCountInString(s) - utf8.RuneCountInString(strings.TrimLeft(s, " \t"))
}

func (p *parser) run() {
	headerDone := false
	for i := 0; i < len(p.lines); i++ {
		raw := p.lines[i]
		ln := i + 1
		line := strings.TrimSpace(raw)
		col := indentOf(raw) + 1
		if line == "" {
			if p.desc != nil && *p.desc != "" {
				*p.desc += "\n"
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			if m := languageRe.FindStringSubmatch(line); m != nil && !headerDone && p.doc.Feature == nil {
				code := strings.ToLower(m[1])
				if _, ok := Dialects[code]; !ok {
					p.errorf(ln, col, "language %q is not supported (use en or sv)", m[1])
				} else {
					p.doc.Language = code
				}
			}
			continue
		}
		headerDone = true
		if strings.HasPrefix(line, `"""`) || strings.HasPrefix(line, "```") {
			i = p.docString(i, raw, line)
			continue
		}
		if strings.HasPrefix(line, "|") {
			p.tableRow(ln, col, line)
			continue
		}
		if strings.HasPrefix(line, "@") {
			p.tagLine(ln, col, line)
			continue
		}
		if k, name, ok := matchHeader(line); ok {
			p.header(ln, col, k, name)
			continue
		}
		if k, text, ok := matchStep(line); ok && p.doc.Feature != nil {
			if p.inSteps() {
				p.addStep(ln, col, k, text)
			} else {
				p.errorf(ln, col, "the %s step must be inside a Scenario or Background", k.word)
				p.desc = nil
			}
			continue
		}
		// free text: the description of the last header
		if p.desc != nil {
			if *p.desc != "" && !strings.HasSuffix(*p.desc, "\n") {
				*p.desc += "\n"
			}
			*p.desc += line
			continue
		}
		switch {
		case p.doc.Feature == nil:
			p.errorf(ln, col, "expected \"Feature:\" (or \"Egenskap:\"), found %q", clip(line))
		case p.examples != nil:
			p.errorf(ln, col, "expected a table row (| … |) in Examples, found %q", clip(line))
		case p.scenario != nil || p.background != nil:
			p.errorf(ln, col, "expected a step (Given, When, Then, And, But or Givet, När, Så, Och, Men), found %q", clip(line))
		default:
			p.errorf(ln, col, "expected Scenario, Scenario Outline, Background or Rule, found %q", clip(line))
		}
	}
	if len(p.tags) > 0 {
		p.errorf(p.tagsLine, 0, "tags must be followed by Feature, Rule, Scenario or Examples")
	}
	p.closeScenario()
	if p.doc.Feature == nil && len(p.errs) == 0 {
		p.errorf(1, 0, "the document has no Feature")
	}
	p.trimDescriptions()
}

func clip(s string) string {
	if utf8.RuneCountInString(s) > 60 {
		r := []rune(s)
		return string(r[:57]) + "…"
	}
	return s
}

// inSteps says whether a step can be added now.
func (p *parser) inSteps() bool {
	return (p.scenario != nil && p.examples == nil) || p.background != nil
}

func (p *parser) tagLine(ln, col int, line string) {
	// a comment may end the line
	if i := strings.Index(line, " #"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(p.tags) == 0 {
		p.tagsLine = ln
	}
	off := 0
	for _, f := range strings.Fields(line) {
		idx := strings.Index(line[off:], f) + off
		off = idx + len(f)
		if !strings.HasPrefix(f, "@") || len(f) == 1 {
			p.errorf(ln, col+utf8.RuneCountInString(line[:idx]), "a tag starts with @ and has a name: %q", f)
			continue
		}
		p.tags = append(p.tags, f)
	}
	p.desc = nil
}

func (p *parser) takeTags() []string {
	t := p.tags
	p.tags = nil
	if t == nil {
		t = []string{}
	}
	return t
}

// closeScenario checks the scenario that ends here.
func (p *parser) closeScenario() {
	sc := p.scenario
	if sc == nil {
		return
	}
	if sc.Outline && len(sc.Examples) == 0 {
		p.errorf(sc.Line, 0, "%s %q has no Examples", sc.Keyword, sc.Name)
	}
	if len(sc.Examples) > 0 {
		sc.Outline = true
		known := map[string]bool{}
		for _, ex := range sc.Examples {
			if ex.Header != nil {
				for _, c := range ex.Header.Cells {
					known[c] = true
				}
			}
		}
		check := func(line int, s string) {
			for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
				if !known[m[1]] {
					p.errorf(line, 0, "<%s> is not a column of the Examples", m[1])
				}
			}
		}
		check(sc.Line, sc.Name)
		for _, st := range sc.Steps {
			check(st.Line, st.Text)
			if st.DocString != nil {
				check(st.DocString.Line, st.DocString.Content)
			}
			for _, r := range st.Table {
				for _, c := range r.Cells {
					check(r.Line, c)
				}
			}
		}
	}
	for _, ex := range sc.Examples {
		if ex.Header == nil {
			p.errorf(ex.Line, 0, "%s has no table (a header row and a row per example)", ex.Keyword)
		} else if len(ex.Rows) == 0 {
			p.errorf(ex.Line, 0, "%s has a header but no rows", ex.Keyword)
		}
	}
	p.scenario, p.examples, p.step = nil, nil, nil
}

func (p *parser) header(ln, col int, k headerKW, name string) {
	if k.kind != hFeature && p.doc.Feature == nil {
		p.errorf(ln, col, "%s: must be inside a Feature", k.word)
		// recover: pretend there was a feature
		p.doc.Feature = &Feature{Line: ln, Keyword: "Feature", Tags: []string{}, Scenarios: []*Scenario{}}
	}
	if k.kind != hExamples && k.kind != hFeature {
		p.closeScenario()
	}
	p.step = nil
	p.lastType = ""
	f := p.doc.Feature
	switch k.kind {
	case hFeature:
		if f != nil {
			p.errorf(ln, col, "a document has one Feature (a second one starts here)")
			p.takeTags()
			p.desc = nil
			return
		}
		if p.doc.Language == "" {
			p.doc.Language = k.lang
		}
		p.doc.Feature = &Feature{Line: ln, Tags: p.takeTags(), Keyword: k.word, Name: name, Scenarios: []*Scenario{}}
		p.desc = &p.doc.Feature.Description
	case hBackground:
		if len(p.tags) > 0 {
			p.errorf(p.tagsLine, 0, "a Background cannot have tags")
			p.takeTags()
		}
		bg := &Background{Line: ln, Keyword: k.word, Name: name, Steps: []*Step{}}
		if p.rule != nil {
			if p.rule.Background != nil {
				p.errorf(ln, col, "the Rule already has a Background")
			} else if len(p.rule.Scenarios) > 0 {
				p.errorf(ln, col, "the Background must come before the scenarios of the Rule")
			}
			p.rule.Background = bg
		} else {
			if f.Background != nil {
				p.errorf(ln, col, "the Feature already has a Background")
			} else if len(f.Scenarios) > 0 {
				p.errorf(ln, col, "the Background must come before the scenarios")
			}
			f.Background = bg
		}
		p.background = bg
		p.desc = &bg.Description
	case hRule:
		p.background = nil
		r := &Rule{Line: ln, Tags: p.takeTags(), Keyword: k.word, Name: name, Scenarios: []*Scenario{}}
		f.Rules = append(f.Rules, r)
		p.rule = r
		p.desc = &r.Description
	case hScenario, hOutline:
		p.background = nil
		sc := &Scenario{Line: ln, Tags: p.takeTags(), Keyword: k.word, Name: name, Outline: k.kind == hOutline, Steps: []*Step{}}
		if p.rule != nil {
			p.rule.Scenarios = append(p.rule.Scenarios, sc)
		} else {
			f.Scenarios = append(f.Scenarios, sc)
		}
		p.scenario = sc
		p.desc = &sc.Description
	case hExamples:
		if p.scenario == nil {
			p.errorf(ln, col, "%s: must follow a Scenario Outline", k.word)
			p.takeTags()
			p.desc = nil
			return
		}
		ex := &Examples{Line: ln, Tags: p.takeTags(), Keyword: k.word, Name: name, Rows: []*Row{}}
		p.scenario.Examples = append(p.scenario.Examples, ex)
		p.examples = ex
		p.desc = &ex.Description
	}
}

func (p *parser) addStep(ln, col int, k stepKW, text string) {
	if len(p.tags) > 0 {
		p.errorf(p.tagsLine, 0, "a step cannot have tags")
		p.takeTags()
	}
	if text == "" {
		p.errorf(ln, col, "the %s step has no text", k.word)
	}
	t := k.typ
	st := &Step{Line: ln, Keyword: k.word, Type: t, Text: text}
	if t != Conjunction {
		p.lastType = t
	} else if p.lastType == "" {
		p.lastType = Context
	}
	if p.background != nil {
		p.background.Steps = append(p.background.Steps, st)
	} else {
		p.scenario.Steps = append(p.scenario.Steps, st)
	}
	p.step = st
	p.desc = nil
}

// SplitCells splits a table row "| a | b \| c |" into its cells.
func SplitCells(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil, false
	}
	var cells []string
	var b strings.Builder
	closed := false
	rs := []rune(line[1:])
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			switch rs[i] {
			case 'n':
				b.WriteByte('\n')
			case '|', '\\':
				b.WriteRune(rs[i])
			default:
				b.WriteRune('\\')
				b.WriteRune(rs[i])
			}
			closed = false
		case r == '|':
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
			closed = true
		default:
			b.WriteRune(r)
			if r != ' ' && r != '\t' {
				closed = false
			}
		}
	}
	return cells, closed
}

func (p *parser) tableRow(ln, col int, line string) {
	cells, closed := SplitCells(line)
	if !closed {
		p.errorf(ln, col+utf8.RuneCountInString(line)-1, "the table row must end with |")
	}
	row := &Row{Line: ln, Cells: cells}
	p.desc = nil
	switch {
	case p.examples != nil:
		ex := p.examples
		if ex.Header == nil {
			ex.Header = row
			seen := map[string]bool{}
			for _, c := range cells {
				if c == "" {
					p.errorf(ln, col, "a column of the Examples header has no name")
				} else if seen[c] {
					p.errorf(ln, col, "the Examples header has the column %q twice", c)
				}
				seen[c] = true
			}
			return
		}
		if len(cells) != len(ex.Header.Cells) {
			p.errorf(ln, col, "the row has %d cells, the header %d", len(cells), len(ex.Header.Cells))
		}
		ex.Rows = append(ex.Rows, row)
	case p.step != nil && p.step.DocString == nil:
		if len(p.step.Table) > 0 && len(cells) != len(p.step.Table[0].Cells) {
			p.errorf(ln, col, "the row has %d cells, the first row of the table %d", len(cells), len(p.step.Table[0].Cells))
		}
		p.step.Table = append(p.step.Table, row)
	default:
		p.errorf(ln, col, "a table must follow a step or Examples")
	}
}

// docString reads a doc string starting at line index i; returns the index
// of its closing line.
func (p *parser) docString(i int, raw, line string) int {
	ln := i + 1
	delim := line[:3]
	media := strings.TrimSpace(line[3:])
	indent := indentOf(raw)
	if p.step == nil || p.step.DocString != nil || len(p.step.Table) > 0 {
		p.errorf(ln, indent+1, "a doc string must follow a step")
	}
	var content []string
	for j := i + 1; j < len(p.lines); j++ {
		l := p.lines[j]
		if strings.TrimSpace(l) == delim {
			ds := &DocString{Line: ln, Delimiter: delim, MediaType: media, Content: strings.Join(content, "\n")}
			if p.step != nil && p.step.DocString == nil && len(p.step.Table) == 0 {
				p.step.DocString = ds
			}
			p.desc = nil
			return j
		}
		// remove the indentation of the opening delimiter
		r := []rune(l)
		k := 0
		for k < indent && k < len(r) && (r[k] == ' ' || r[k] == '\t') {
			k++
		}
		c := string(r[k:])
		c = strings.ReplaceAll(c, `\`+delim[:1]+`\`+delim[:1]+`\`+delim[:1], delim)
		content = append(content, c)
	}
	p.errorf(ln, indent+1, "the doc string %s is not closed", delim)
	return len(p.lines)
}

func (p *parser) trimDescriptions() {
	f := p.doc.Feature
	if f == nil {
		return
	}
	trim := func(s *string) { *s = strings.TrimSpace(*s) }
	trim(&f.Description)
	if f.Background != nil {
		trim(&f.Background.Description)
	}
	for _, sc := range f.AllScenarios() {
		trim(&sc.Description)
		for _, ex := range sc.Examples {
			trim(&ex.Description)
		}
	}
	for _, r := range f.Rules {
		trim(&r.Description)
		if r.Background != nil {
			trim(&r.Background.Description)
		}
	}
}
