package oslc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// OSLC Core 2.0 query syntax, the part the provider supports:
//
//	oslc.where  = term { " and " term }
//	term        = property op value | property " in [" value { "," value } "]"
//	op          = "=" | "!=" | "<" | ">" | "<=" | ">="
//	value       = "string" ["^^" type] | number | true | false | <iri>
//	oslc.prefix = prefix "=<" namespace ">" { "," prefix "=<" namespace ">" }
//
// Properties are prefixed names (dcterms:title); the caller decides which
// properties it can filter on.

// Condition is one term of oslc.where.
type Condition struct {
	Property string   // full IRI
	Op       string   // = != < > <= >= in
	Values   []string // literal values (IRIs without <>)
	IRIs     []bool   // whether each value was an <iri>
}

// Query is a parsed oslc.where.
type Query struct{ Conditions []Condition }

// ParsePrefixes parses oslc.prefix into prefix → namespace, on top of
// base.
func ParsePrefixes(s string, base map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	s = strings.TrimSpace(s)
	for s != "" {
		eq := strings.Index(s, "=")
		lt := strings.Index(s, "<")
		gt := strings.Index(s, ">")
		if eq <= 0 || lt != eq+1 || gt < lt {
			return nil, fmt.Errorf("oslc.prefix: expected prefix=<namespace> at %q", s)
		}
		out[strings.TrimSpace(s[:eq])] = s[lt+1 : gt]
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s[gt+1:]), ","))
	}
	return out, nil
}

type lexer struct {
	s   string
	pos int
}

func (l *lexer) space() {
	for l.pos < len(l.s) && unicode.IsSpace(rune(l.s[l.pos])) {
		l.pos++
	}
}

func (l *lexer) errf(format string, a ...any) error {
	return fmt.Errorf("oslc.where at %d: %s", l.pos+1, fmt.Sprintf(format, a...))
}

func (l *lexer) word() string {
	start := l.pos
	for l.pos < len(l.s) {
		c := l.s[l.pos]
		if unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)) || c == ':' || c == '_' || c == '-' || c == '.' {
			l.pos++
			continue
		}
		break
	}
	return l.s[start:l.pos]
}

// ParseWhere parses oslc.where with the prefixes.
func ParseWhere(s string, prefixes map[string]string) (*Query, error) {
	q := &Query{}
	l := &lexer{s: s}
	for {
		l.space()
		if l.pos >= len(l.s) {
			if len(q.Conditions) == 0 {
				return nil, l.errf("empty")
			}
			return q, nil
		}
		name := l.word()
		if name == "" {
			return nil, l.errf("expected a property")
		}
		if strings.Contains(name, "{") {
			return nil, l.errf("nested properties are not supported")
		}
		prop, err := expand(name, prefixes)
		if err != nil {
			return nil, l.errf("%v", err)
		}
		l.space()
		c := Condition{Property: prop}
		switch {
		case strings.HasPrefix(l.s[l.pos:], "!="), strings.HasPrefix(l.s[l.pos:], "<="), strings.HasPrefix(l.s[l.pos:], ">="):
			c.Op = l.s[l.pos : l.pos+2]
			l.pos += 2
		case strings.HasPrefix(l.s[l.pos:], "="), strings.HasPrefix(l.s[l.pos:], "<"), strings.HasPrefix(l.s[l.pos:], ">"):
			c.Op = l.s[l.pos : l.pos+1]
			l.pos++
		case strings.HasPrefix(l.s[l.pos:], "in"):
			c.Op = "in"
			l.pos += 2
		default:
			return nil, l.errf("expected an operator (=, !=, <, >, <=, >=, in)")
		}
		l.space()
		if c.Op == "in" {
			if l.pos >= len(l.s) || l.s[l.pos] != '[' {
				return nil, l.errf("expected [ after in")
			}
			l.pos++
			for {
				l.space()
				v, iri, err := l.value()
				if err != nil {
					return nil, err
				}
				c.Values, c.IRIs = append(c.Values, v), append(c.IRIs, iri)
				l.space()
				if l.pos < len(l.s) && l.s[l.pos] == ',' {
					l.pos++
					continue
				}
				if l.pos < len(l.s) && l.s[l.pos] == ']' {
					l.pos++
					break
				}
				return nil, l.errf("expected , or ]")
			}
		} else {
			v, iri, err := l.value()
			if err != nil {
				return nil, err
			}
			c.Values, c.IRIs = []string{v}, []bool{iri}
		}
		q.Conditions = append(q.Conditions, c)
		l.space()
		if l.pos >= len(l.s) {
			return q, nil
		}
		if w := l.word(); w != "and" {
			return nil, l.errf("expected and, found %q", w)
		}
	}
}

func expand(name string, prefixes map[string]string) (string, error) {
	p, local, ok := strings.Cut(name, ":")
	if !ok {
		return "", fmt.Errorf("property %q needs a prefix (dcterms:title)", name)
	}
	ns, ok := prefixes[p]
	if !ok {
		return "", fmt.Errorf("unknown prefix %q (declare it with oslc.prefix)", p)
	}
	return ns + local, nil
}

func (l *lexer) value() (string, bool, error) {
	if l.pos >= len(l.s) {
		return "", false, l.errf("expected a value")
	}
	switch c := l.s[l.pos]; {
	case c == '"':
		l.pos++
		var b strings.Builder
		for l.pos < len(l.s) && l.s[l.pos] != '"' {
			if l.s[l.pos] == '\\' && l.pos+1 < len(l.s) {
				l.pos++
			}
			b.WriteByte(l.s[l.pos])
			l.pos++
		}
		if l.pos >= len(l.s) {
			return "", false, l.errf("unterminated string")
		}
		l.pos++
		// a datatype or language tag is accepted and ignored
		if strings.HasPrefix(l.s[l.pos:], "^^") {
			l.pos += 2
			l.word()
		} else if l.pos < len(l.s) && l.s[l.pos] == '@' {
			l.pos++
			l.word()
		}
		return b.String(), false, nil
	case c == '<':
		end := strings.IndexByte(l.s[l.pos:], '>')
		if end < 0 {
			return "", false, l.errf("unterminated <iri>")
		}
		v := l.s[l.pos+1 : l.pos+end]
		l.pos += end + 1
		return v, true, nil
	default:
		w := l.word()
		if w == "" {
			return "", false, l.errf("expected a value")
		}
		if w != "true" && w != "false" {
			if _, err := strconv.ParseFloat(w, 64); err != nil {
				return "", false, l.errf("%q is not a value (quote strings)", w)
			}
		}
		return w, false, nil
	}
}

// Match reports whether the value of a property satisfies c: strings
// compare as text (case-sensitive), times as times (RFC 3339).
func (c Condition) Match(value string, isTime bool) bool {
	cmp := func(v string) int {
		if isTime {
			a, err1 := parseTime(value)
			b, err2 := parseTime(v)
			if err1 == nil && err2 == nil {
				return a.Compare(b)
			}
		}
		return strings.Compare(value, v)
	}
	switch c.Op {
	case "in":
		for _, v := range c.Values {
			if cmp(v) == 0 {
				return true
			}
		}
		return false
	}
	n := cmp(c.Values[0])
	switch c.Op {
	case "=":
		return n == 0
	case "!=":
		return n != 0
	case "<":
		return n < 0
	case ">":
		return n > 0
	case "<=":
		return n <= 0
	case ">=":
		return n >= 0
	}
	return false
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("not a time: %q", s)
}
