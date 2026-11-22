package oslc

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Reading RDF: the representations OSLC clients send to the creation
// factories and resources (POST, PUT) as Turtle, RDF/XML or JSON-LD (and
// the OSLC 2.0 JSON format). It covers what resource descriptions use:
//
//   - Turtle: @prefix / PREFIX, @base / BASE, IRIs, prefixed names, "a",
//     blank nodes (_:x and [ … ]), literals ("…", '…', """…""", numbers,
//     booleans, @lang, ^^datatype), the ; and , lists (not collections);
//   - RDF/XML: rdf:Description and typed node elements, rdf:about,
//     rdf:nodeID, rdf:resource, rdf:datatype, rdf:parseType="Literal"
//     (an rdf:XMLLiteral holding the inner XML) and "Resource", nested node
//     elements and property attributes;
//   - JSON-LD: an inline @context (prefixes and terms, "@type": "@id"),
//     @graph, @id, @type, @value, nested node objects and arrays; remote
//     contexts are not fetched.
//
// Relative IRIs (<> included) resolve against base, the request URL.

// XMLLiteral is the datatype of rdf:parseType="Literal" values.
const XMLLiteral = NSRDF + "XMLLiteral"

// ErrSyntax wraps the errors of malformed documents.
var ErrSyntax = errors.New("invalid RDF")

func syntaxf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrSyntax, fmt.Sprintf(format, a...))
}

// MediaOf returns the RDF media type of a Content-Type header ("" when it
// is not one).
func MediaOf(contentType string) string {
	mt := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch mt {
	case MediaRDFXML, "application/xml", "text/xml":
		return MediaRDFXML
	case MediaTurtle, "application/x-turtle":
		return MediaTurtle
	case MediaJSONLD, "application/json":
		return MediaJSONLD
	}
	return ""
}

// Parse reads an RDF document of media type mt.
func Parse(data []byte, mt, base string) (*Graph, error) {
	g := NewGraph(nil)
	var err error
	switch mt {
	case MediaTurtle:
		err = parseTurtle(g, string(data), base)
	case MediaJSONLD:
		err = parseJSONLD(g, data, base)
	case MediaRDFXML:
		err = parseRDFXML(g, data, base)
	default:
		return nil, syntaxf("unsupported media type %q (use text/turtle, application/rdf+xml or application/ld+json)", mt)
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

func resolve(base, ref string) string {
	if ref == "" {
		return base
	}
	u, err := url.Parse(ref)
	if err != nil || u.IsAbs() {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return b.ResolveReference(u).String()
}

// Subjects returns the subjects of the graph in order of first use.
func (g *Graph) Subjects() []Term { return g.subjects() }

// Objects returns the objects of the triples s p ?.
func (g *Graph) Objects(s Term, p string) []Term {
	var out []Term
	for _, t := range g.Triples {
		if t.P == p && t.S.IRI == s.IRI && t.S.BNode == s.BNode {
			out = append(out, t.O)
		}
	}
	return out
}

// Has reports whether s has any of the predicates.
func (g *Graph) Has(s Term, preds ...string) bool {
	for _, p := range preds {
		if len(g.Objects(s, p)) > 0 {
			return true
		}
	}
	return false
}

// SubjectOf returns the resource a request body describes: the subject of
// type class, else the one with the IRI want, else the first subject.
func (g *Graph) SubjectOf(class, want string) (Term, bool) {
	for _, t := range g.Triples {
		if t.P == NSRDF+"type" && t.O.IRI == class {
			return t.S, true
		}
	}
	for _, s := range g.subjects() {
		if want != "" && s.IRI == want {
			return s, true
		}
	}
	ss := g.subjects()
	if len(ss) == 0 {
		return Term{}, false
	}
	return ss[0], true
}

// ---------------------------------------------------------------- Turtle

type turtle struct {
	s        string
	pos      int
	g        *Graph
	prefixes map[string]string
	base     string
	bnodes   int
}

func parseTurtle(g *Graph, s, base string) error {
	p := &turtle{s: s, g: g, prefixes: map[string]string{}, base: base}
	for {
		p.skip()
		if p.pos >= len(p.s) {
			return nil
		}
		if err := p.statement(); err != nil {
			return err
		}
	}
}

func (p *turtle) errf(format string, a ...any) error {
	line := strings.Count(p.s[:min(p.pos, len(p.s))], "\n") + 1
	return syntaxf("Turtle line %d: %s", line, fmt.Sprintf(format, a...))
}

// skip skips white space and comments.
func (p *turtle) skip() {
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == '#':
			for p.pos < len(p.s) && p.s[p.pos] != '\n' {
				p.pos++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *turtle) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *turtle) keyword(k string) bool {
	if len(p.s)-p.pos >= len(k) && strings.EqualFold(p.s[p.pos:p.pos+len(k)], k) {
		end := p.pos + len(k)
		if end == len(p.s) || unicode.IsSpace(rune(p.s[end])) || p.s[end] == '<' {
			p.pos = end
			return true
		}
	}
	return false
}

func (p *turtle) statement() error {
	at := p.peek() == '@'
	switch {
	case p.keyword("@prefix") || p.keyword("prefix"):
		sparql := !at
		p.skip()
		name := p.pname()
		if !strings.HasSuffix(name, ":") {
			return p.errf("expected a prefix name ending in ':'")
		}
		p.skip()
		iri, err := p.iriref()
		if err != nil {
			return err
		}
		p.prefixes[strings.TrimSuffix(name, ":")] = iri
		p.g.Prefixes[strings.TrimSuffix(name, ":")] = iri
		if !sparql {
			return p.expect('.')
		}
		return nil
	case p.keyword("@base") || p.keyword("base"):
		sparql := !at
		p.skip()
		iri, err := p.iriref()
		if err != nil {
			return err
		}
		p.base = iri
		if !sparql {
			return p.expect('.')
		}
		return nil
	}
	subj, err := p.subject()
	if err != nil {
		return err
	}
	p.skip()
	if p.peek() == '.' { // "[ … ] ." alone
		p.pos++
		return nil
	}
	if err := p.predicateObjects(subj); err != nil {
		return err
	}
	return p.expect('.')
}

func (p *turtle) expect(c byte) error {
	p.skip()
	if p.peek() != c {
		return p.errf("expected '%c'", c)
	}
	p.pos++
	return nil
}

func (p *turtle) iriref() (string, error) {
	if p.peek() != '<' {
		return "", p.errf("expected <iri>")
	}
	end := strings.IndexByte(p.s[p.pos:], '>')
	if end < 0 {
		return "", p.errf("unterminated <iri>")
	}
	v := p.s[p.pos+1 : p.pos+end]
	p.pos += end + 1
	return resolve(p.base, v), nil
}

func isPNChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == ':' || c == '%' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func (p *turtle) pname() string {
	start := p.pos
	for p.pos < len(p.s) && isPNChar(p.s[p.pos]) {
		p.pos++
	}
	// a name does not end with '.'
	for p.pos > start && p.s[p.pos-1] == '.' {
		p.pos--
	}
	return p.s[start:p.pos]
}

func (p *turtle) prefixed() (string, error) {
	name := p.pname()
	pre, local, ok := strings.Cut(name, ":")
	if !ok {
		return "", p.errf("expected a prefixed name, got %q", name)
	}
	ns, ok := p.prefixes[pre]
	if !ok {
		return "", p.errf("unknown prefix %q", pre)
	}
	return ns + local, nil
}

func (p *turtle) iri() (string, error) {
	if p.peek() == '<' {
		return p.iriref()
	}
	return p.prefixed()
}

func (p *turtle) blank() Term {
	p.bnodes++
	return Blank("b" + strconv.Itoa(p.bnodes))
}

func (p *turtle) subject() (Term, error) {
	p.skip()
	switch {
	case strings.HasPrefix(p.s[p.pos:], "_:"):
		p.pos += 2
		return Blank("n-" + p.pname()), nil
	case p.peek() == '[':
		return p.anon()
	case p.peek() == '(':
		return Term{}, p.errf("collections are not supported")
	}
	iri, err := p.iri()
	return IRI(iri), err
}

func (p *turtle) anon() (Term, error) {
	p.pos++ // [
	b := p.blank()
	p.skip()
	if p.peek() == ']' {
		p.pos++
		return b, nil
	}
	if err := p.predicateObjects(b); err != nil {
		return Term{}, err
	}
	return b, p.expect(']')
}

func (p *turtle) predicateObjects(subj Term) error {
	for {
		p.skip()
		var pred string
		if p.peek() == 'a' && p.pos+1 < len(p.s) && !isPNChar(p.s[p.pos+1]) {
			p.pos++
			pred = NSRDF + "type"
		} else {
			var err error
			if pred, err = p.iri(); err != nil {
				return err
			}
		}
		for {
			o, err := p.object()
			if err != nil {
				return err
			}
			p.g.Triples = append(p.g.Triples, Triple{subj, pred, o})
			p.skip()
			if p.peek() != ',' {
				break
			}
			p.pos++
		}
		p.skip()
		if p.peek() != ';' {
			return nil
		}
		for p.peek() == ';' {
			p.pos++
			p.skip()
		}
		if c := p.peek(); c == '.' || c == ']' || c == 0 {
			return nil
		}
	}
}

func (p *turtle) object() (Term, error) {
	p.skip()
	c := p.peek()
	switch {
	case c == '"' || c == '\'':
		return p.literal()
	case c == '[' || c == '_' && strings.HasPrefix(p.s[p.pos:], "_:"):
		return p.subject()
	case c == '(':
		return Term{}, p.errf("collections are not supported")
	case c == '<':
		iri, err := p.iriref()
		return IRI(iri), err
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		start := p.pos
		for p.pos < len(p.s) && strings.IndexByte("+-.0123456789eE", p.s[p.pos]) >= 0 {
			p.pos++
		}
		for p.pos > start && p.s[p.pos-1] == '.' {
			p.pos--
		}
		n := p.s[start:p.pos]
		dt := NSXSD + "integer"
		if strings.ContainsAny(n, "eE") {
			dt = NSXSD + "double"
		} else if strings.Contains(n, ".") {
			dt = NSXSD + "decimal"
		}
		return Typed(n, dt), nil
	}
	if p.keyword("true") {
		return Typed("true", NSXSD+"boolean"), nil
	}
	if p.keyword("false") {
		return Typed("false", NSXSD+"boolean"), nil
	}
	iri, err := p.prefixed()
	return IRI(iri), err
}

func (p *turtle) literal() (Term, error) {
	q := p.s[p.pos]
	long := strings.HasPrefix(p.s[p.pos:], strings.Repeat(string(q), 3))
	delim := string(q)
	if long {
		delim = strings.Repeat(string(q), 3)
	}
	p.pos += len(delim)
	var b strings.Builder
	for {
		if p.pos >= len(p.s) {
			return Term{}, p.errf("unterminated string")
		}
		if strings.HasPrefix(p.s[p.pos:], delim) {
			p.pos += len(delim)
			break
		}
		c := p.s[p.pos]
		if !long && (c == '\n' || c == '\r') {
			return Term{}, p.errf("line break in a short string")
		}
		if c == '\\' && p.pos+1 < len(p.s) {
			p.pos++
			switch e := p.s[p.pos]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'u', 'U':
				n := 4
				if e == 'U' {
					n = 8
				}
				if p.pos+n >= len(p.s) {
					return Term{}, p.errf("bad escape")
				}
				r, err := strconv.ParseUint(p.s[p.pos+1:p.pos+1+n], 16, 32)
				if err != nil {
					return Term{}, p.errf("bad escape")
				}
				b.WriteRune(rune(r))
				p.pos += n
			default:
				b.WriteByte(e)
			}
			p.pos++
			continue
		}
		b.WriteByte(c)
		p.pos++
	}
	t := Lit(b.String())
	if p.peek() == '@' {
		p.pos++
		for p.pos < len(p.s) && (isPNChar(p.s[p.pos]) && p.s[p.pos] != ':' && p.s[p.pos] != '.') {
			p.pos++
		}
	} else if strings.HasPrefix(p.s[p.pos:], "^^") {
		p.pos += 2
		dt, err := p.iri()
		if err != nil {
			return Term{}, err
		}
		t.Datatype = dt
		if dt == NSXSD+"string" {
			t.Datatype = ""
		}
	}
	return t, nil
}

// ---------------------------------------------------------------- RDF/XML

type xelem struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Inner    string     `xml:",innerxml"`
	Children []xelem    `xml:",any"`
	Text     string     `xml:",chardata"`
}

func (e *xelem) attr(space, local string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

type rdfxml struct {
	g      *Graph
	base   string
	bnodes int
}

func parseRDFXML(g *Graph, data []byte, base string) error {
	var root xelem
	dec := xml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&root); err != nil {
		return syntaxf("RDF/XML: %v", err)
	}
	p := &rdfxml{g: g, base: base}
	if root.XMLName.Space == NSRDF && root.XMLName.Local == "RDF" {
		if b, ok := root.attr("http://www.w3.org/XML/1998/namespace", "base"); ok {
			p.base = resolve(base, b)
		}
		for i := range root.Children {
			if _, err := p.node(&root.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	_, err := p.node(&root)
	return err
}

func (p *rdfxml) blank() Term {
	p.bnodes++
	return Blank("x" + strconv.Itoa(p.bnodes))
}

// node reads a node element and returns its subject.
func (p *rdfxml) node(e *xelem) (Term, error) {
	var subj Term
	if v, ok := e.attr(NSRDF, "about"); ok {
		subj = IRI(resolve(p.base, v))
	} else if v, ok := e.attr(NSRDF, "nodeID"); ok {
		subj = Blank("n-" + v)
	} else if v, ok := e.attr(NSRDF, "ID"); ok {
		subj = IRI(resolve(p.base, "#"+v))
	} else {
		subj = p.blank()
	}
	if !(e.XMLName.Space == NSRDF && e.XMLName.Local == "Description") {
		p.g.Triples = append(p.g.Triples, Triple{subj, NSRDF + "type", IRI(e.XMLName.Space + e.XMLName.Local)})
	}
	for _, a := range e.Attrs {
		if a.Name.Space == NSRDF || a.Name.Space == "xmlns" || a.Name.Space == "" || strings.HasPrefix(a.Name.Space, "http://www.w3.org/XML/1998/namespace") {
			continue
		}
		p.g.Triples = append(p.g.Triples, Triple{subj, a.Name.Space + a.Name.Local, Lit(a.Value)})
	}
	for i := range e.Children {
		if err := p.property(subj, &e.Children[i]); err != nil {
			return Term{}, err
		}
	}
	return subj, nil
}

func (p *rdfxml) property(subj Term, e *xelem) error {
	pred := e.XMLName.Space + e.XMLName.Local
	if e.XMLName.Space == "" {
		return syntaxf("RDF/XML: the property element <%s> has no namespace", e.XMLName.Local)
	}
	add := func(o Term) { p.g.Triples = append(p.g.Triples, Triple{subj, pred, o}) }
	if v, ok := e.attr(NSRDF, "resource"); ok {
		add(IRI(resolve(p.base, v)))
		return nil
	}
	if v, ok := e.attr(NSRDF, "nodeID"); ok {
		add(Blank("n-" + v))
		return nil
	}
	switch pt, _ := e.attr(NSRDF, "parseType"); pt {
	case "Literal":
		add(Typed(strings.TrimSpace(e.Inner), XMLLiteral))
		return nil
	case "Resource":
		b := p.blank()
		add(b)
		for i := range e.Children {
			if err := p.property(b, &e.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if len(e.Children) > 0 {
		for i := range e.Children {
			o, err := p.node(&e.Children[i])
			if err != nil {
				return err
			}
			add(o)
		}
		return nil
	}
	t := Lit(e.Text)
	if dt, ok := e.attr(NSRDF, "datatype"); ok && dt != NSXSD+"string" {
		t.Datatype = dt
	}
	add(t)
	return nil
}

// ---------------------------------------------------------------- JSON-LD

type jsonld struct {
	g      *Graph
	base   string
	bnodes int
}

type ldContext struct {
	prefixes map[string]string
	terms    map[string]string // term → IRI
	ids      map[string]bool   // terms whose string values are IRIs
}

func parseJSONLD(g *Graph, data []byte, base string) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return syntaxf("JSON-LD: %v", err)
	}
	p := &jsonld{g: g, base: base}
	ctx := &ldContext{prefixes: map[string]string{}, terms: map[string]string{}, ids: map[string]bool{}}
	for k, v := range g.Prefixes {
		ctx.prefixes[k] = v
	}
	switch d := doc.(type) {
	case []any:
		for _, n := range d {
			if m, ok := n.(map[string]any); ok {
				if _, err := p.node(m, ctx); err != nil {
					return err
				}
			}
		}
		return nil
	case map[string]any:
		ctx = p.context(d, ctx)
		if gr, ok := d["@graph"].([]any); ok {
			for _, n := range gr {
				if m, ok := n.(map[string]any); ok {
					if _, err := p.node(m, ctx); err != nil {
						return err
					}
				}
			}
			return nil
		}
		_, err := p.node(d, ctx)
		return err
	}
	return syntaxf("JSON-LD: expected an object or an array")
}

func (p *jsonld) context(n map[string]any, parent *ldContext) *ldContext {
	c := &ldContext{prefixes: map[string]string{}, terms: map[string]string{}, ids: map[string]bool{}}
	for k, v := range parent.prefixes {
		c.prefixes[k] = v
	}
	for k, v := range parent.terms {
		c.terms[k] = v
	}
	for k, v := range parent.ids {
		c.ids[k] = v
	}
	read := func(m map[string]any) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch v := m[k].(type) {
			case string:
				if strings.HasSuffix(v, "#") || strings.HasSuffix(v, "/") {
					c.prefixes[k] = v
				}
				c.terms[k] = v
			case map[string]any:
				if id, ok := v["@id"].(string); ok {
					c.terms[k] = id
				}
				if t, _ := v["@type"].(string); t == "@id" {
					c.ids[k] = true
				}
			}
		}
	}
	switch x := n["@context"].(type) {
	case map[string]any:
		read(x)
	case []any:
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				read(m)
			}
		}
	}
	// the OSLC 2.0 JSON format
	if m, ok := n["prefixes"].(map[string]any); ok {
		read(m)
	}
	// terms may use prefixes
	for k, v := range c.terms {
		c.terms[k] = c.expand(v)
	}
	return c
}

func (c *ldContext) expand(s string) string {
	if strings.HasPrefix(s, "@") {
		return s
	}
	if iri, ok := c.terms[s]; ok && iri != s {
		return iri
	}
	if pre, local, ok := strings.Cut(s, ":"); ok && !strings.HasPrefix(local, "//") {
		if ns, ok := c.prefixes[pre]; ok {
			return ns + local
		}
	}
	return s
}

func (p *jsonld) blank() Term {
	p.bnodes++
	return Blank("j" + strconv.Itoa(p.bnodes))
}

func (p *jsonld) ref(s string, c *ldContext) Term {
	if strings.HasPrefix(s, "_:") {
		return Blank("n-" + strings.TrimPrefix(s, "_:"))
	}
	return IRI(resolve(p.base, c.expand(s)))
}

func (p *jsonld) node(n map[string]any, parent *ldContext) (Term, error) {
	c := parent
	if _, ok := n["@context"]; ok {
		c = p.context(n, parent)
	} else if _, ok := n["prefixes"]; ok {
		c = p.context(n, parent)
	}
	var subj Term
	if id, ok := n["@id"].(string); ok {
		subj = p.ref(id, c)
	} else if id, ok := n["rdf:about"].(string); ok {
		subj = p.ref(id, c)
	} else {
		subj = p.blank()
	}
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := n[k]
		switch k {
		case "@id", "@context", "rdf:about", "prefixes", "@graph":
			continue
		case "@type":
			for _, t := range asList(v) {
				if s, ok := t.(string); ok {
					p.g.Triples = append(p.g.Triples, Triple{subj, NSRDF + "type", IRI(c.expand(s))})
				}
			}
			continue
		}
		pred := c.expand(k)
		if pred == "rdf:type" || pred == NSRDF+"type" {
			pred = NSRDF + "type"
		}
		if !strings.Contains(pred, ":") {
			continue // a term without a mapping is dropped (as JSON-LD does)
		}
		for _, x := range asList(v) {
			o, err := p.value(x, c, c.ids[k] || pred == NSRDF+"type")
			if err != nil {
				return Term{}, err
			}
			if o != nil {
				p.g.Triples = append(p.g.Triples, Triple{subj, pred, *o})
			}
		}
	}
	return subj, nil
}

func asList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

func (p *jsonld) value(v any, c *ldContext, isID bool) (*Term, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if isID {
			t := p.ref(x, c)
			return &t, nil
		}
		t := Lit(x)
		return &t, nil
	case bool:
		t := Typed(strconv.FormatBool(x), NSXSD+"boolean")
		return &t, nil
	case json.Number:
		dt := NSXSD + "integer"
		if strings.ContainsAny(x.String(), ".eE") {
			dt = NSXSD + "double"
		}
		t := Typed(x.String(), dt)
		return &t, nil
	case map[string]any:
		if val, ok := x["@value"]; ok {
			s := fmt.Sprint(val)
			t := Lit(s)
			if dt, ok := x["@type"].(string); ok {
				t.Datatype = c.expand(dt)
			}
			return &t, nil
		}
		if r, ok := x["rdf:resource"].(string); ok {
			t := p.ref(r, c)
			return &t, nil
		}
		if id, ok := x["@id"].(string); ok && len(x) == 1 {
			t := p.ref(id, c)
			return &t, nil
		}
		t, err := p.node(x, c)
		if err != nil {
			return nil, err
		}
		return &t, nil
	}
	return nil, syntaxf("JSON-LD: unsupported value %v", v)
}
