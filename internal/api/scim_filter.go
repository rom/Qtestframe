package api

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SCIM filters (RFC 7644 §3.4.2.2) and PATCH paths (§3.5.2), evaluated in
// memory against a resource in its JSON form (map[string]any). Supported:
// eq ne co sw ew gt ge lt le pr, and/or/not, parentheses, value paths
// such as emails[type eq "work"] and emails[type eq "work"].value eq "x",
// sub-attributes (name.givenName) and attribute names with a schema URN.

var errFilter = errors.New("invalid filter")

func filterErrorf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errFilter, fmt.Sprintf(format, a...))
}

// scimFilter is a parsed filter expression.
type scimFilter interface {
	match(res map[string]any) bool
}

type tokKind int

const (
	tWord tokKind = iota
	tString
	tLParen
	tRParen
	tLBracket
	tRBracket
	tEOF
)

type ftoken struct {
	kind tokKind
	val  string
}

func lexFilter(s string) ([]ftoken, error) {
	var out []ftoken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, ftoken{tLParen, "("})
			i++
		case c == ')':
			out = append(out, ftoken{tRParen, ")"})
			i++
		case c == '[':
			out = append(out, ftoken{tLBracket, "["})
			i++
		case c == ']':
			out = append(out, ftoken{tRBracket, "]"})
			i++
		case c == '"':
			// a JSON string
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return nil, filterErrorf("unterminated string")
			}
			v, err := strconv.Unquote(s[i : j+1])
			if err != nil {
				return nil, filterErrorf("bad string %s", s[i:j+1])
			}
			out = append(out, ftoken{tString, v})
			i = j + 1
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\n\r()[]\"", rune(s[j])) {
				j++
			}
			out = append(out, ftoken{tWord, s[i:j]})
			i = j
		}
	}
	return append(out, ftoken{tEOF, ""}), nil
}

type filterParser struct {
	toks []ftoken
	pos  int
}

func (p *filterParser) peek() ftoken { return p.toks[p.pos] }
func (p *filterParser) next() ftoken {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *filterParser) keyword(k string) bool {
	t := p.peek()
	if t.kind == tWord && strings.EqualFold(t.val, k) {
		p.pos++
		return true
	}
	return false
}

// parseSCIMFilter parses a filter expression.
func parseSCIMFilter(s string) (scimFilter, error) {
	toks, err := lexFilter(s)
	if err != nil {
		return nil, err
	}
	p := &filterParser{toks: toks}
	f, err := p.or()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, filterErrorf("unexpected %q", t.val)
	}
	return f, nil
}

type orFilter struct{ a, b scimFilter }
type andFilter struct{ a, b scimFilter }
type notFilter struct{ f scimFilter }

func (f orFilter) match(r map[string]any) bool  { return f.a.match(r) || f.b.match(r) }
func (f andFilter) match(r map[string]any) bool { return f.a.match(r) && f.b.match(r) }
func (f notFilter) match(r map[string]any) bool { return !f.f.match(r) }

func (p *filterParser) or() (scimFilter, error) {
	f, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.keyword("or") {
		g, err := p.and()
		if err != nil {
			return nil, err
		}
		f = orFilter{f, g}
	}
	return f, nil
}

func (p *filterParser) and() (scimFilter, error) {
	f, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.keyword("and") {
		g, err := p.unary()
		if err != nil {
			return nil, err
		}
		f = andFilter{f, g}
	}
	return f, nil
}

func (p *filterParser) unary() (scimFilter, error) {
	if p.keyword("not") {
		if p.next().kind != tLParen {
			return nil, filterErrorf("not must be followed by (")
		}
		f, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRParen {
			return nil, filterErrorf("missing )")
		}
		return notFilter{f}, nil
	}
	if p.peek().kind == tLParen {
		p.next()
		f, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRParen {
			return nil, filterErrorf("missing )")
		}
		return f, nil
	}
	return p.comparison()
}

// attrFilter compares the values at path (all of them, any may match).
type attrFilter struct {
	path  []string
	op    string
	value any
}

// valuePathFilter matches when an element of the multi-valued attribute
// matches the inner filter (and, with sub, the comparison on its sub
// attribute).
type valuePathFilter struct {
	path  []string
	inner scimFilter
	sub   *attrFilter
}

var filterOps = map[string]bool{"eq": true, "ne": true, "co": true, "sw": true, "ew": true, "gt": true, "ge": true, "lt": true, "le": true}

func (p *filterParser) comparison() (scimFilter, error) {
	t := p.next()
	if t.kind != tWord {
		return nil, filterErrorf("attribute expected")
	}
	path := splitAttrPath(t.val)
	if p.peek().kind == tLBracket {
		p.next()
		inner, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRBracket {
			return nil, filterErrorf("missing ]")
		}
		vp := valuePathFilter{path: path, inner: inner}
		if n := p.peek(); n.kind == tWord && strings.HasPrefix(n.val, ".") {
			p.next()
			sub, err := p.operator(splitAttrPath(strings.TrimPrefix(n.val, ".")))
			if err != nil {
				return nil, err
			}
			vp.sub = sub
		}
		return vp, nil
	}
	return p.operator(path)
}

func (p *filterParser) operator(path []string) (*attrFilter, error) {
	op := p.next()
	if op.kind != tWord {
		return nil, filterErrorf("operator expected")
	}
	o := strings.ToLower(op.val)
	if o == "pr" {
		return &attrFilter{path: path, op: o}, nil
	}
	if !filterOps[o] {
		return nil, filterErrorf("unknown operator %q", op.val)
	}
	v := p.next()
	var val any
	switch {
	case v.kind == tString:
		val = v.val
	case v.kind == tWord && strings.EqualFold(v.val, "true"):
		val = true
	case v.kind == tWord && strings.EqualFold(v.val, "false"):
		val = false
	case v.kind == tWord && strings.EqualFold(v.val, "null"):
		val = nil
	case v.kind == tWord:
		f, err := strconv.ParseFloat(v.val, 64)
		if err != nil {
			return nil, filterErrorf("bad value %q", v.val)
		}
		val = f
	default:
		return nil, filterErrorf("value expected")
	}
	return &attrFilter{path: path, op: o, value: val}, nil
}

// splitAttrPath splits an attribute path into its names, dropping a core
// schema URN; another schema URN (an extension) is one name.
func splitAttrPath(s string) []string {
	low := strings.ToLower(s)
	for _, urn := range []string{scimUserSchema, scimGroupSchema} {
		if strings.HasPrefix(low, strings.ToLower(urn)+":") {
			s = s[len(urn)+1:]
			return strings.Split(s, ".")
		}
	}
	if strings.HasPrefix(low, "urn:") {
		i := strings.LastIndex(s, ":")
		return append([]string{s[:i]}, strings.Split(s[i+1:], ".")...)
	}
	return strings.Split(s, ".")
}

// lookup returns the value of key in m, matching the key case-insensitively.
func lookup(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// valuesAt returns every value at path, flattening multi-valued
// attributes on the way.
func valuesAt(v any, path []string) []any {
	if len(path) == 0 {
		if l, ok := v.([]any); ok {
			return l
		}
		if v == nil {
			return nil
		}
		return []any{v}
	}
	switch x := v.(type) {
	case map[string]any:
		c, ok := lookup(x, path[0])
		if !ok {
			return nil
		}
		return valuesAt(c, path[1:])
	case []any:
		var out []any
		for _, e := range x {
			out = append(out, valuesAt(e, path)...)
		}
		return out
	}
	return nil
}

// caseExact attributes compare strings exactly; all others ignore case.
var caseExact = map[string]bool{"id": true, "externalid": true}

func (f *attrFilter) match(r map[string]any) bool {
	vals := valuesAt(r, f.path)
	if f.op == "pr" {
		for _, v := range vals {
			if s, ok := v.(string); !ok || s != "" {
				return true
			}
		}
		return false
	}
	if f.value == nil && (f.op == "eq" || f.op == "ne") {
		// eq null: no value; ne null: a value
		return (len(vals) == 0) == (f.op == "eq")
	}
	exact := caseExact[strings.ToLower(f.path[len(f.path)-1])]
	if f.op == "ne" {
		for _, v := range vals {
			if compare(v, "eq", f.value, exact) {
				return false
			}
		}
		return true
	}
	for _, v := range vals {
		if compare(v, f.op, f.value, exact) {
			return true
		}
	}
	return false
}

func (f valuePathFilter) match(r map[string]any) bool {
	for _, e := range valuesAt(r, f.path) {
		m, ok := e.(map[string]any)
		if !ok || !f.inner.match(m) {
			continue
		}
		if f.sub == nil || f.sub.match(m) {
			return true
		}
	}
	return false
}

func compare(have any, op string, want any, exact bool) bool {
	switch w := want.(type) {
	case nil:
		return op == "eq" && have == nil
	case bool:
		h, ok := have.(bool)
		return ok && op == "eq" && h == w
	case float64:
		h, ok := have.(float64)
		if !ok {
			return false
		}
		switch op {
		case "eq":
			return h == w
		case "gt":
			return h > w
		case "ge":
			return h >= w
		case "lt":
			return h < w
		case "le":
			return h <= w
		}
		return false
	case string:
		h, ok := have.(string)
		if !ok {
			return false
		}
		if !exact {
			h, w = strings.ToLower(h), strings.ToLower(w)
		}
		switch op {
		case "eq":
			return h == w
		case "co":
			return strings.Contains(h, w)
		case "sw":
			return strings.HasPrefix(h, w)
		case "ew":
			return strings.HasSuffix(h, w)
		case "gt":
			return h > w
		case "ge":
			return h >= w
		case "lt":
			return h < w
		case "le":
			return h <= w
		}
	}
	return false
}

// scimPath is a parsed PATCH path: attr[filter].sub.
type scimPath struct {
	attr   string // lower-case top-level attribute (URN dropped)
	sub    string // lower-case sub-attribute or ""
	filter scimFilter
}

// parseSCIMPath parses a PATCH path such as "active", "name.givenName",
// `emails[type eq "work"].value` or `members[value eq "id"]`.
func parseSCIMPath(s string) (*scimPath, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, filterErrorf("empty path")
	}
	toks, err := lexFilter(s)
	if err != nil {
		return nil, err
	}
	p := &filterParser{toks: toks}
	t := p.next()
	if t.kind != tWord {
		return nil, filterErrorf("attribute expected")
	}
	names := splitAttrPath(t.val)
	out := &scimPath{attr: strings.ToLower(names[0])}
	if len(names) > 1 {
		out.sub = strings.ToLower(strings.Join(names[1:], "."))
	}
	if p.peek().kind == tLBracket {
		if out.sub != "" {
			return nil, filterErrorf("bad path %q", s)
		}
		p.next()
		f, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRBracket {
			return nil, filterErrorf("missing ]")
		}
		out.filter = f
		if n := p.peek(); n.kind == tWord && strings.HasPrefix(n.val, ".") {
			p.next()
			out.sub = strings.ToLower(strings.TrimPrefix(n.val, "."))
		}
	}
	if p.peek().kind != tEOF {
		return nil, filterErrorf("bad path %q", s)
	}
	return out, nil
}
