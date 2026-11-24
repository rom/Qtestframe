package gherkin

import (
	"fmt"
	"regexp"
	"strings"
)

// Expression kinds of step definitions.
const (
	KindCucumber = "cucumber"
	KindRegex    = "regex"
)

// ParameterTypes are the built-in parameter types of Cucumber expressions
// and the regular expressions they match.
var ParameterTypes = map[string]string{
	"int":        `-?\d+`,
	"float":      `-?\d*[.,]?\d+(?:[eE][-+]?\d+)?`,
	"word":       `[^\s]+`,
	"string":     `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`,
	"":           `.*`,
	"bigdecimal": `-?\d*[.,]?\d+`,
	"double":     `-?\d*[.,]?\d+(?:[eE][-+]?\d+)?`,
	"biginteger": `-?\d+`,
	"byte":       `-?\d+`,
	"short":      `-?\d+`,
	"long":       `-?\d+`,
}

// placeholderAlt lets a parameter also match an outline <placeholder>, so
// that the steps of a Scenario Outline match their definitions.
const placeholderAlt = `|<[^<>]+>`

// Expression is a compiled step definition pattern.
type Expression struct {
	Pattern string
	Kind    string
	re      *regexp.Regexp
	// params: the parameter type names in order (cucumber); nil for regex
	params []string
	// segments of a cucumber expression for Render
	segs []segment
}

type segment struct {
	text  string // literal text (optional text and alternatives resolved)
	param bool
}

// Arg is a matched parameter: its text as written (quotes included for a
// {string}) and its value (quotes removed).
type Arg struct {
	Raw   string `json:"raw"`
	Value string `json:"value"`
}

// Compile compiles a pattern of a kind ("cucumber", the default, or "regex").
func Compile(pattern, kind string) (*Expression, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("the pattern is empty")
	}
	if kind == "" {
		kind = KindCucumber
	}
	e := &Expression{Pattern: pattern, Kind: kind}
	switch kind {
	case KindRegex:
		p := strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
		re, err := regexp.Compile(`^(?:` + p + `)$`)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression: %v", err)
		}
		e.re = re
	case KindCucumber:
		src, err := e.cucumber(pattern)
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(`^` + src + `$`)
		if err != nil {
			return nil, fmt.Errorf("invalid expression: %v", err)
		}
		e.re = re
	default:
		return nil, fmt.Errorf("unknown kind %q (cucumber or regex)", kind)
	}
	return e, nil
}

// cucumber translates a Cucumber expression to a regular expression.
func (e *Expression) cucumber(p string) (string, error) {
	var b strings.Builder
	rs := []rune(p)
	var lit strings.Builder // literal text of the current segment
	flush := func() {
		if lit.Len() > 0 {
			e.segs = append(e.segs, segment{text: lit.String()})
			lit.Reset()
		}
	}
	// alternatives: a/b/c within a word (no spaces)
	word := func(start int) (int, bool) {
		// returns the end index of a word containing '/', when it has one
		j := start
		has := false
		for j < len(rs) && rs[j] != ' ' && rs[j] != '{' && rs[j] != '(' {
			if rs[j] == '\\' {
				j += 2
				continue
			}
			if rs[j] == '/' {
				has = true
			}
			j++
		}
		return j, has
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			b.WriteString(regexp.QuoteMeta(string(rs[i])))
			lit.WriteRune(rs[i])
		case r == '{':
			j := i + 1
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j >= len(rs) {
				return "", fmt.Errorf("the parameter at character %d is not closed with }", i+1)
			}
			name := string(rs[i+1 : j])
			re, ok := ParameterTypes[name]
			if !ok {
				return "", fmt.Errorf("unknown parameter type {%s} (use {int}, {float}, {word}, {string} or {})", name)
			}
			b.WriteString("(" + re + placeholderAlt + ")")
			flush()
			e.segs = append(e.segs, segment{param: true})
			e.params = append(e.params, name)
			i = j
		case r == '(':
			j := i + 1
			for j < len(rs) && rs[j] != ')' {
				j++
			}
			if j >= len(rs) {
				return "", fmt.Errorf("the optional text at character %d is not closed with )", i+1)
			}
			opt := string(rs[i+1 : j])
			if strings.ContainsAny(opt, "{(") {
				return "", fmt.Errorf("optional text cannot contain parameters or other optional text")
			}
			b.WriteString("(?:" + regexp.QuoteMeta(opt) + ")?")
			lit.WriteString(opt)
			i = j
		case r == '}' || r == ')':
			return "", fmt.Errorf("unexpected %c at character %d", r, i+1)
		default:
			end, alt := word(i)
			if alt {
				parts := strings.Split(string(rs[i:end]), "/")
				q := make([]string, len(parts))
				for k, s := range parts {
					if s == "" {
						return "", fmt.Errorf("an alternative is empty")
					}
					q[k] = regexp.QuoteMeta(strings.ReplaceAll(s, `\`, ""))
				}
				b.WriteString("(?:" + strings.Join(q, "|") + ")")
				lit.WriteString(strings.ReplaceAll(parts[0], `\`, ""))
				i = end - 1
				continue
			}
			b.WriteString(regexp.QuoteMeta(string(r)))
			lit.WriteRune(r)
		}
	}
	flush()
	return b.String(), nil
}

// Params returns the number of parameters.
func (e *Expression) Params() int {
	if e.Kind == KindCucumber {
		return len(e.params)
	}
	return e.re.NumSubexp()
}

// Match matches a step text and returns its parameters.
func (e *Expression) Match(text string) ([]Arg, bool) {
	m := e.re.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return nil, false
	}
	args := []Arg{}
	for i, g := range m[1:] {
		v := g
		if e.Kind == KindCucumber && i < len(e.params) && e.params[i] == "string" && len(g) >= 2 && (g[0] == '"' || g[0] == '\'') {
			v = g[1 : len(g)-1]
		}
		args = append(args, Arg{Raw: g, Value: v})
	}
	return args, true
}

// Render writes a step text from a Cucumber expression and parameter
// texts (as written, e.g. with quotes): optional text is included and
// the first of alternatives is used.
func (e *Expression) Render(args []Arg) (string, error) {
	if e.Kind != KindCucumber {
		return "", fmt.Errorf("only a Cucumber expression can write step texts")
	}
	if len(args) != len(e.params) {
		return "", fmt.Errorf("the expression has %d parameters, the step %d", len(e.params), len(args))
	}
	var b strings.Builder
	n := 0
	for _, s := range e.segs {
		if s.param {
			b.WriteString(args[n].Raw)
			n++
		} else {
			b.WriteString(s.text)
		}
	}
	return b.String(), nil
}

var (
	numberRe = regexp.MustCompile(`^-?\d+$`)
	floatRe  = regexp.MustCompile(`^-?\d*[.,]\d+$`)
	quotedRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
)

// Suggest proposes a Cucumber expression for an undefined step text:
// quoted strings become {string}, numbers {int} or {float}, outline
// placeholders {}. Braces and parentheses are escaped.
func Suggest(text string) string {
	text = strings.TrimSpace(text)
	var out []string
	// split on quoted strings first
	idx := quotedRe.FindAllStringIndex(text, -1)
	pos := 0
	var parts []string
	var quoted []bool
	for _, ix := range idx {
		parts = append(parts, text[pos:ix[0]])
		quoted = append(quoted, false)
		parts = append(parts, text[ix[0]:ix[1]])
		quoted = append(quoted, true)
		pos = ix[1]
	}
	parts = append(parts, text[pos:])
	quoted = append(quoted, false)
	escape := strings.NewReplacer(`\`, `\\`, "{", `\{`, "}", `\}`, "(", `\(`, ")", `\)`, "/", `\/`)
	for i, p := range parts {
		if quoted[i] {
			out = append(out, "{string}")
			continue
		}
		fields := strings.SplitAfter(p, " ")
		var b strings.Builder
		for _, f := range fields {
			w := strings.TrimRight(f, " ")
			sp := f[len(w):]
			switch {
			case numberRe.MatchString(w):
				b.WriteString("{int}")
			case floatRe.MatchString(w):
				b.WriteString("{float}")
			case placeholderRe.MatchString(w) && placeholderRe.FindString(w) == w:
				b.WriteString("{}")
			default:
				b.WriteString(escape.Replace(w))
			}
			b.WriteString(sp)
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "")
}
