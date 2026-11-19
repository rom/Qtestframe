// Package query implements the filter / search expression language used by
// the `q` query parameters and the search endpoint.
//
// Grammar (informal):
//
//	expr    = or
//	or      = and { ("OR" | "||") and }
//	and     = unary { ["AND" | "&&"] unary }        // adjacency = implicit AND
//	unary   = ("NOT" | "!" | "-") unary | primary
//	primary = "(" expr ")" | term
//	term    = field op value | value
//	op      = ":" | "=" | "!=" | "~" | "!~" | ">" | ">=" | "<" | "<="
//	value   = bare word | "quoted \"string\"" | /regex/flags
//
// Precedence is NOT > AND > OR. Keywords are case-insensitive.
package query

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Limits protecting against abusive expressions.
const (
	MaxLength  = 4096 // maximum expression length in bytes
	MaxRegexps = 64   // maximum number of regular expressions / globs
	maxDepth   = 128  // maximum nesting depth of groups / negations
)

// Record is something a Query can be matched against.
type Record interface {
	// Values returns the values of a field (field name already lowercased &
	// alias-resolved). ok=false => unknown field.
	Values(field string) (vals []string, ok bool)
	// AllValues returns every searchable value, used for unqualified terms.
	AllValues() []string
}

// FieldLister is an optional interface a Record may implement so that
// MatchedFields can report which concrete field matched an unqualified term
// (instead of "*").
type FieldLister interface {
	Fields() []string
}

// MapRecord is a simple Record backed by a map of field -> values.
type MapRecord map[string][]string

// Values implements Record. Lookup is exact first, then case-insensitive.
func (m MapRecord) Values(field string) ([]string, bool) {
	if v, ok := m[field]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, field) {
			return v, true
		}
	}
	return nil, false
}

// Fields implements FieldLister (sorted key order).
func (m MapRecord) Fields() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AllValues implements Record: all values of all keys in sorted key order.
func (m MapRecord) AllValues() []string {
	var out []string
	for _, k := range m.Fields() {
		out = append(out, m[k]...)
	}
	return out
}

// Schema optionally restricts the accepted field names. A nil schema accepts
// any field.
type Schema struct {
	Fields  []string
	Aliases map[string]string
}

// resolve lowercases and alias-resolves name; ok=false if unknown.
func (s *Schema) resolve(name string) (string, bool) {
	name = strings.ToLower(name)
	if s == nil {
		return name, true
	}
	for k, v := range s.Aliases {
		if strings.ToLower(k) == name {
			name = strings.ToLower(v)
			break
		}
	}
	for _, f := range s.Fields {
		if strings.ToLower(f) == name {
			return name, true
		}
	}
	return name, false
}

// Error is a parse error with a byte position in the expression.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string {
	return fmt.Sprintf("query: %s at position %d", e.Msg, e.Pos)
}

// ---------------------------------------------------------------------------
// AST

type nodeKind int

const (
	nAnd nodeKind = iota
	nOr
	nNot
	nCond
)

type condKind int

const (
	cContains condKind = iota // ":" plain contains
	cGlob                     // ":" with wildcards (or unqualified glob)
	cHas                      // field:* / has:field
	cEq                       // "="
	cNe                       // "!="
	cRe                       // "~"
	cNotRe                    // "!~"
	cCmp                      // > >= < <=
)

type node struct {
	kind     nodeKind
	children []*node // and/or
	child    *node   // not

	// cond
	field   string // "" = unqualified
	op      string
	value   string
	ck      condKind
	re      *regexp.Regexp
	reSrc   string
	reFlags string
	isRe    bool // value written as /re/flags
	lower   string
	num     float64
	isNum   bool
}

// Query is a parsed expression.
type Query struct {
	root *node
	src  string
}

// ---------------------------------------------------------------------------
// Lexer

type tokKind int

const (
	tEOF tokKind = iota
	tLParen
	tRParen
	tAnd
	tOr
	tNot
	tTerm
)

type token struct {
	kind tokKind
	pos  int
	text string // original text for keywords
	cond *node  // for tTerm
	fpos int    // field position (for schema errors)
	raw  string // raw field name
}

type lexer struct {
	s      string
	i      int
	schema *Schema
	nre    int
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '.'
}

func (l *lexer) skipSpace() {
	for l.i < len(l.s) && isSpace(l.s[l.i]) {
		l.i++
	}
}

func (l *lexer) atBoundary(i int) bool {
	return i >= len(l.s) || isSpace(l.s[i]) || l.s[i] == '(' || l.s[i] == ')'
}

var ops = []string{"!=", "!~", ">=", "<=", ":", "=", "~", ">", "<"}

func (l *lexer) next() (token, error) {
	l.skipSpace()
	if l.i >= len(l.s) {
		return token{kind: tEOF, pos: len(l.s)}, nil
	}
	start := l.i
	c := l.s[l.i]
	switch {
	case c == '(':
		l.i++
		return token{kind: tLParen, pos: start}, nil
	case c == ')':
		l.i++
		return token{kind: tRParen, pos: start}, nil
	case strings.HasPrefix(l.s[l.i:], "&&"):
		l.i += 2
		return token{kind: tAnd, pos: start, text: "&&"}, nil
	case strings.HasPrefix(l.s[l.i:], "||"):
		l.i += 2
		return token{kind: tOr, pos: start, text: "||"}, nil
	case c == '!':
		l.i++
		return token{kind: tNot, pos: start, text: "!"}, nil
	case c == '-' && l.i+1 < len(l.s) && !isSpace(l.s[l.i+1]):
		l.i++
		return token{kind: tNot, pos: start, text: "-"}, nil
	case c == '"':
		v, err := l.quoted()
		if err != nil {
			return token{}, err
		}
		n, err := l.makeCond("", ":", v, false, "", start)
		if err != nil {
			return token{}, err
		}
		return token{kind: tTerm, pos: start, cond: n}, nil
	case c == '/':
		re, flags, err := l.regex()
		if err != nil {
			return token{}, err
		}
		n, err := l.makeCond("", "~", re, true, flags, start)
		if err != nil {
			return token{}, err
		}
		return token{kind: tTerm, pos: start, cond: n}, nil
	}

	// field<op>value ?
	if isIdentStart(c) {
		j := l.i
		for j < len(l.s) && isIdentChar(l.s[j]) {
			j++
		}
		for _, op := range ops {
			if strings.HasPrefix(l.s[j:], op) {
				field := l.s[l.i:j]
				l.i = j + len(op)
				return l.fieldTerm(field, op, start)
			}
		}
	}

	// bare word
	j := l.i
	for j < len(l.s) && !isSpace(l.s[j]) && l.s[j] != '(' && l.s[j] != ')' {
		j++
	}
	word := l.s[l.i:j]
	l.i = j
	switch strings.ToUpper(word) {
	case "AND":
		return token{kind: tAnd, pos: start, text: word}, nil
	case "OR":
		return token{kind: tOr, pos: start, text: word}, nil
	case "NOT":
		return token{kind: tNot, pos: start, text: word}, nil
	}
	n, err := l.makeCond("", ":", word, false, "", start)
	if err != nil {
		return token{}, err
	}
	return token{kind: tTerm, pos: start, cond: n}, nil
}

func (l *lexer) fieldTerm(rawField, op string, start int) (token, error) {
	vpos := l.i
	var (
		val   string
		isRe  bool
		flags string
		err   error
	)
	switch {
	case l.i < len(l.s) && l.s[l.i] == '"':
		val, err = l.quoted()
	case l.i < len(l.s) && l.s[l.i] == '/' && (op == "~" || op == "!~"):
		val, flags, err = l.regex()
		isRe = true
	default:
		j := l.i
		for j < len(l.s) && !l.atBoundary(j) {
			j++
		}
		if j == l.i {
			return token{}, &Error{Pos: vpos, Msg: fmt.Sprintf("missing value for field %q", rawField)}
		}
		val = l.s[l.i:j]
		l.i = j
	}
	if err != nil {
		return token{}, err
	}

	field, known := l.schema.resolve(rawField)
	if strings.EqualFold(rawField, "has") && op == ":" && (l.schema == nil || !known) {
		f, ok := l.schema.resolve(val)
		if !ok {
			return token{}, &Error{Pos: vpos, Msg: fmt.Sprintf("unknown field %q", val)}
		}
		return token{kind: tTerm, pos: start, cond: &node{kind: nCond, field: f, op: ":", value: "*", ck: cHas}}, nil
	}
	if !known {
		return token{}, &Error{Pos: start, Msg: fmt.Sprintf("unknown field %q", rawField)}
	}
	n, err := l.makeCond(field, op, val, isRe, flags, vpos)
	if err != nil {
		return token{}, err
	}
	return token{kind: tTerm, pos: start, cond: n}, nil
}

// quoted reads a double-quoted string starting at l.i.
func (l *lexer) quoted() (string, error) {
	start := l.i
	l.i++ // opening quote
	var b strings.Builder
	for l.i < len(l.s) {
		c := l.s[l.i]
		switch c {
		case '\\':
			if l.i+1 < len(l.s) {
				b.WriteByte(l.s[l.i+1])
				l.i += 2
				continue
			}
			l.i++
		case '"':
			l.i++
			return b.String(), nil
		default:
			b.WriteByte(c)
			l.i++
		}
	}
	return "", &Error{Pos: start, Msg: "unterminated quoted string"}
}

// regex reads /re/flags starting at l.i.
func (l *lexer) regex() (string, string, error) {
	start := l.i
	l.i++
	var b strings.Builder
	for l.i < len(l.s) {
		c := l.s[l.i]
		if c == '\\' && l.i+1 < len(l.s) {
			if l.s[l.i+1] == '/' {
				b.WriteByte('/')
			} else {
				b.WriteByte('\\')
				b.WriteByte(l.s[l.i+1])
			}
			l.i += 2
			continue
		}
		if c == '/' {
			l.i++
			fs := l.i
			for l.i < len(l.s) && !l.atBoundary(l.i) {
				if l.s[l.i] != 'i' {
					return "", "", &Error{Pos: l.i, Msg: fmt.Sprintf("invalid regex flag %q", l.s[l.i])}
				}
				l.i++
			}
			flags := ""
			if l.i > fs {
				flags = "i"
			}
			return b.String(), flags, nil
		}
		b.WriteByte(c)
		l.i++
	}
	return "", "", &Error{Pos: start, Msg: "unterminated regular expression"}
}

func hasWildcard(s string) bool { return strings.ContainsAny(s, "*?") }

func globToRegexp(g string) string {
	var b strings.Builder
	b.WriteString("(?is)^")
	for _, r := range g {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

func (l *lexer) compile(src string, pos int) (*regexp.Regexp, error) {
	l.nre++
	if l.nre > MaxRegexps {
		return nil, &Error{Pos: pos, Msg: fmt.Sprintf("too many regular expressions (max %d)", MaxRegexps)}
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, &Error{Pos: pos, Msg: "invalid regular expression: " + err.Error()}
	}
	return re, nil
}

func (l *lexer) makeCond(field, op, val string, isRe bool, flags string, pos int) (*node, error) {
	n := &node{kind: nCond, field: field, op: op, value: val, isRe: isRe, reFlags: flags}
	var err error
	switch op {
	case ":":
		switch {
		case field != "" && val == "*":
			n.ck = cHas
		case hasWildcard(val):
			n.ck = cGlob
			n.re, err = l.compile(globToRegexp(val), pos)
		default:
			n.ck = cContains
			n.lower = strings.ToLower(val)
		}
	case "=", "!=":
		n.ck = cEq
		if op == "!=" {
			n.ck = cNe
		}
	case "~", "!~":
		n.ck = cRe
		if op == "!~" {
			n.ck = cNotRe
		}
		n.reSrc = val
		src := val
		if flags == "i" {
			src = "(?i)" + src
		}
		n.re, err = l.compile(src, pos)
	default: // comparisons
		n.ck = cCmp
		if f, e := strconv.ParseFloat(strings.TrimSpace(val), 64); e == nil {
			n.num, n.isNum = f, true
		}
	}
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Parser

type parser struct {
	lex   *lexer
	tok   token
	depth int
}

func (p *parser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

func startsUnary(k tokKind) bool {
	return k == tTerm || k == tNot || k == tLParen
}

func (p *parser) parseOr() (*node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	kids := []*node{left}
	for p.tok.kind == tOr {
		opTok := p.tok
		if err := p.advance(); err != nil {
			return nil, err
		}
		if !startsUnary(p.tok.kind) {
			return nil, &Error{Pos: p.tok.pos, Msg: fmt.Sprintf("expected term after %s", opTok.text)}
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		kids = append(kids, right)
	}
	if len(kids) == 1 {
		return left, nil
	}
	return &node{kind: nOr, children: kids}, nil
}

func (p *parser) parseAnd() (*node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	kids := []*node{left}
	for {
		if p.tok.kind == tAnd {
			opTok := p.tok
			if err := p.advance(); err != nil {
				return nil, err
			}
			if !startsUnary(p.tok.kind) {
				return nil, &Error{Pos: p.tok.pos, Msg: fmt.Sprintf("expected term after %s", opTok.text)}
			}
		} else if !startsUnary(p.tok.kind) {
			break
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		kids = append(kids, right)
	}
	if len(kids) == 1 {
		return left, nil
	}
	return &node{kind: nAnd, children: kids}, nil
}

func (p *parser) enter(pos int) error {
	p.depth++
	if p.depth > maxDepth {
		return &Error{Pos: pos, Msg: "expression nested too deeply"}
	}
	return nil
}

func (p *parser) parseUnary() (*node, error) {
	switch p.tok.kind {
	case tNot:
		opTok := p.tok
		if err := p.enter(opTok.pos); err != nil {
			return nil, err
		}
		defer func() { p.depth-- }()
		if err := p.advance(); err != nil {
			return nil, err
		}
		if !startsUnary(p.tok.kind) {
			return nil, &Error{Pos: p.tok.pos, Msg: fmt.Sprintf("expected term after %s", opTok.text)}
		}
		c, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if c.kind == nNot { // double negation
			return c.child, nil
		}
		return &node{kind: nNot, child: c}, nil
	case tLParen:
		open := p.tok
		if err := p.enter(open.pos); err != nil {
			return nil, err
		}
		defer func() { p.depth-- }()
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind == tRParen {
			return nil, &Error{Pos: p.tok.pos, Msg: "empty group"}
		}
		if !startsUnary(p.tok.kind) {
			return nil, &Error{Pos: p.tok.pos, Msg: "unexpected " + p.tokName()}
		}
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.tok.kind != tRParen {
			return nil, &Error{Pos: open.pos, Msg: "unclosed '('"}
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return n, nil
	case tTerm:
		n := p.tok.cond
		if err := p.advance(); err != nil {
			return nil, err
		}
		return n, nil
	}
	return nil, &Error{Pos: p.tok.pos, Msg: "unexpected " + p.tokName()}
}

func (p *parser) tokName() string {
	switch p.tok.kind {
	case tEOF:
		return "end of expression"
	case tRParen:
		return "')'"
	default:
		return strings.ToUpper(p.tok.text)
	}
}

// Parse parses expr. An empty or whitespace-only expression yields a Query
// matching everything. With a non-nil schema, unknown fields are an error.
func Parse(expr string, schema *Schema) (*Query, error) {
	if len(expr) > MaxLength {
		return nil, &Error{Pos: MaxLength, Msg: fmt.Sprintf("expression too long (max %d characters)", MaxLength)}
	}
	q := &Query{src: expr}
	if strings.TrimSpace(expr) == "" {
		return q, nil
	}
	p := &parser{lex: &lexer{s: expr, schema: schema}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if !startsUnary(p.tok.kind) {
		return nil, &Error{Pos: p.tok.pos, Msg: "unexpected " + p.tokName()}
	}
	root, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.tok.kind != tEOF {
		return nil, &Error{Pos: p.tok.pos, Msg: "unexpected " + p.tokName()}
	}
	q.root = root
	return q, nil
}

// ---------------------------------------------------------------------------
// Evaluation

// IsEmpty reports whether the query has no conditions (matches everything).
func (q *Query) IsEmpty() bool { return q == nil || q.root == nil }

// Match reports whether r satisfies the query.
func (q *Query) Match(r Record) bool {
	if q.IsEmpty() {
		return true
	}
	return eval(q.root, r)
}

func eval(n *node, r Record) bool {
	switch n.kind {
	case nAnd:
		for _, c := range n.children {
			if !eval(c, r) {
				return false
			}
		}
		return true
	case nOr:
		for _, c := range n.children {
			if eval(c, r) {
				return true
			}
		}
		return false
	case nNot:
		return !eval(n.child, r)
	}
	var vals []string
	if n.field == "" {
		vals = r.AllValues()
	} else {
		vals, _ = r.Values(n.field)
	}
	return n.matchValues(vals)
}

func (n *node) matchValues(vals []string) bool {
	switch n.ck {
	case cHas:
		for _, v := range vals {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
		return false
	case cEq, cNe:
		var hit bool
		if n.value == "" {
			hit = true
			for _, v := range vals {
				if v != "" {
					hit = false
					break
				}
			}
		} else {
			for _, v := range vals {
				if strings.EqualFold(v, n.value) {
					hit = true
					break
				}
			}
		}
		if n.ck == cNe {
			return !hit
		}
		return hit
	case cRe, cNotRe, cGlob:
		hit := false
		for _, v := range vals {
			if n.re.MatchString(v) {
				hit = true
				break
			}
		}
		if n.ck == cNotRe {
			return !hit
		}
		return hit
	case cCmp:
		for _, v := range vals {
			if n.compare(v) {
				return true
			}
		}
		return false
	}
	// cContains
	for _, v := range vals {
		if strings.Contains(strings.ToLower(v), n.lower) {
			return true
		}
	}
	return false
}

func (n *node) compare(v string) bool {
	var c int
	if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && n.isNum {
		switch {
		case f < n.num:
			c = -1
		case f > n.num:
			c = 1
		}
	} else {
		c = strings.Compare(v, n.value)
	}
	switch n.op {
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	case "<":
		return c < 0
	}
	return c <= 0 // "<="
}

// positive reports whether a condition is a "positive" match condition, i.e.
// one whose success implies the presence of matching content.
func (n *node) positive() bool { return n.ck != cNe && n.ck != cNotRe }

// MatchedFields returns which fields produced positive matches of the
// (non-negated) terms of the query, sorted and de-duplicated. Unqualified
// terms report the concrete field when the record implements FieldLister,
// otherwise "*".
func (q *Query) MatchedFields(r Record) []string {
	if q.IsEmpty() {
		return nil
	}
	set := map[string]bool{}
	var walk func(n *node)
	walk = func(n *node) {
		switch n.kind {
		case nAnd, nOr:
			for _, c := range n.children {
				walk(c)
			}
			return
		case nNot:
			return
		}
		if !n.positive() {
			return
		}
		if n.field != "" {
			vals, _ := r.Values(n.field)
			if n.matchValues(vals) {
				set[n.field] = true
			}
			return
		}
		if fl, ok := r.(FieldLister); ok {
			for _, f := range fl.Fields() {
				vals, _ := r.Values(f)
				if n.matchValues(vals) {
					set[f] = true
				}
			}
			return
		}
		if n.matchValues(r.AllValues()) {
			set["*"] = true
		}
	}
	walk(q.root)
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Terms returns the plain free-text (unqualified, non-regex, non-negated)
// term values, de-duplicated in order of appearance. Useful for highlighting.
func (q *Query) Terms() []string {
	if q.IsEmpty() {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var walk func(n *node)
	walk = func(n *node) {
		switch n.kind {
		case nAnd, nOr:
			for _, c := range n.children {
				walk(c)
			}
		case nCond:
			if n.field == "" && !n.isRe && n.value != "" && !seen[n.value] {
				seen[n.value] = true
				out = append(out, n.value)
			}
		}
	}
	walk(q.root)
	return out
}

// ---------------------------------------------------------------------------
// Formatting

// String returns the normalized form of the query; parsing it again yields an
// equivalent query.
func (q *Query) String() string {
	if q.IsEmpty() {
		return ""
	}
	return format(q.root, false)
}

func format(n *node, nested bool) string {
	switch n.kind {
	case nAnd, nOr:
		sep := " AND "
		if n.kind == nOr {
			sep = " OR "
		}
		parts := make([]string, len(n.children))
		for i, c := range n.children {
			parts[i] = format(c, true)
		}
		s := strings.Join(parts, sep)
		if nested {
			return "(" + s + ")"
		}
		return s
	case nNot:
		return "NOT " + format(n.child, true)
	}
	switch n.ck {
	case cHas:
		return n.field + ":*"
	case cRe, cNotRe:
		re := "/" + strings.ReplaceAll(n.reSrc, "/", `\/`) + "/" + n.reFlags
		if n.field == "" {
			return re
		}
		return n.field + n.op + re
	}
	v := Quote(n.value)
	if n.field == "" {
		return v
	}
	return n.field + n.op + v
}

func bareSafe(s string) bool {
	if s == "" || s[0] == '-' || s[0] == '!' || s[0] == '/' {
		return false
	}
	switch strings.ToUpper(s) {
	case "AND", "OR", "NOT":
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_.-*?@#%+,'$^", r):
		case r > 127 && r != 0x85 && r != 0xA0 && r != 0x2028 && r != 0x2029:
		default:
			return false
		}
	}
	return true
}

// Quote returns s as a value token that parses back to s: bare if safe,
// otherwise double-quoted with `"` and `\` escaped.
func Quote(s string) string {
	if bareSafe(s) {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String()
}
