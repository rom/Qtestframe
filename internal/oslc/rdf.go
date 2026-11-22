// Package oslc renders RDF graphs for the OSLC provider (internal/api
// oslc.go) as RDF/XML, Turtle and JSON-LD, and parses OSLC query
// parameters (oslc.where, oslc.prefix). It is a small writer for the
// resources Qtestframe serves, not a general RDF library.
package oslc

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Namespaces.
const (
	NSRDF     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRDFS    = "http://www.w3.org/2000/01/rdf-schema#"
	NSDCTerms = "http://purl.org/dc/terms/"
	NSOSLC    = "http://open-services.net/ns/core#"
	NSRM      = "http://open-services.net/ns/rm#"
	NSQM      = "http://open-services.net/ns/qm#"
	NSFOAF    = "http://xmlns.com/foaf/0.1/"
	NSXSD     = "http://www.w3.org/2001/XMLSchema#"
)

// Term is an RDF node: an IRI, a blank node or a literal.
type Term struct {
	IRI      string
	BNode    string
	Literal  string
	Datatype string // IRI of the literal's datatype ("" = xsd:string)
	isLit    bool
}

// IRI, Blank, Lit and Typed build terms.
func IRI(s string) Term { return Term{IRI: s} }

// Blank returns a blank node with the label.
func Blank(label string) Term { return Term{BNode: label} }

// Lit returns a plain string literal.
func Lit(s string) Term { return Term{Literal: s, isLit: true} }

// Typed returns a typed literal.
func Typed(s, datatype string) Term { return Term{Literal: s, Datatype: datatype, isLit: true} }

// IsLiteral reports whether t is a literal.
func (t Term) IsLiteral() bool { return t.isLit }

// Triple is a statement.
type Triple struct {
	S Term
	P string
	O Term
}

// Graph is a set of triples with namespace prefixes.
type Graph struct {
	Prefixes map[string]string // prefix → namespace
	Triples  []Triple
}

// NewGraph returns a graph with the OSLC prefixes and extra (prefix →
// namespace).
func NewGraph(extra map[string]string) *Graph {
	g := &Graph{Prefixes: map[string]string{
		"rdf": NSRDF, "rdfs": NSRDFS, "dcterms": NSDCTerms, "oslc": NSOSLC, "oslc_rm": NSRM, "oslc_qm": NSQM,
		"foaf": NSFOAF, "xsd": NSXSD,
	}}
	for k, v := range extra {
		g.Prefixes[k] = v
	}
	return g
}

// Add adds a triple; an empty literal is left out.
func (g *Graph) Add(s Term, p string, o Term) {
	if o.isLit && o.Literal == "" {
		return
	}
	if !o.isLit && o.IRI == "" && o.BNode == "" {
		return
	}
	g.Triples = append(g.Triples, Triple{s, p, o})
}

// Type adds rdf:type.
func (g *Graph) Type(s Term, class string) { g.Add(s, NSRDF+"type", IRI(class)) }

// subjects returns the subjects in order of first use.
func (g *Graph) subjects() []Term {
	var out []Term
	seen := map[string]bool{}
	for _, t := range g.Triples {
		k := t.S.IRI + "\x00" + t.S.BNode
		if !seen[k] {
			seen[k] = true
			out = append(out, t.S)
		}
	}
	return out
}

func (g *Graph) about(s Term) []Triple {
	var out []Triple
	for _, t := range g.Triples {
		if t.S.IRI == s.IRI && t.S.BNode == s.BNode {
			out = append(out, t)
		}
	}
	return out
}

// split cuts an IRI into a known namespace prefix and a local name.
func (g *Graph) split(iri string) (prefix, local string, ok bool) {
	best := ""
	for p, ns := range g.Prefixes {
		if strings.HasPrefix(iri, ns) && len(ns) > len(g.Prefixes[best]) {
			best = p
		}
	}
	if best == "" {
		return "", "", false
	}
	local = strings.TrimPrefix(iri, g.Prefixes[best])
	if local == "" || !validLocal(local) {
		return "", "", false
	}
	return best, local, true
}

func validLocal(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case (r >= '0' && r <= '9') || r == '-' || r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return !strings.HasSuffix(s, ".")
}

func (g *Graph) sortedPrefixes() []string {
	ps := make([]string, 0, len(g.Prefixes))
	for p := range g.Prefixes {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

// ---------------------------------------------------------------- Turtle

func turtleString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func (g *Graph) turtleTerm(t Term) string {
	switch {
	case t.isLit:
		s := turtleString(t.Literal)
		if t.Datatype != "" {
			s += "^^" + g.turtleIRI(t.Datatype)
		}
		return s
	case t.BNode != "":
		return "_:" + t.BNode
	}
	return g.turtleIRI(t.IRI)
}

func (g *Graph) turtleIRI(iri string) string {
	if p, l, ok := g.split(iri); ok {
		return p + ":" + l
	}
	return "<" + strings.NewReplacer(">", "%3E", " ", "%20", `"`, "%22").Replace(iri) + ">"
}

// WriteTurtle writes the graph as Turtle.
func (g *Graph) WriteTurtle(w io.Writer) error {
	var b strings.Builder
	for _, p := range g.sortedPrefixes() {
		fmt.Fprintf(&b, "@prefix %s: <%s> .\n", p, g.Prefixes[p])
	}
	for _, s := range g.subjects() {
		b.WriteString("\n" + g.turtleTerm(s))
		ts := g.about(s)
		for i, t := range ts {
			sep := " ;"
			if i == len(ts)-1 {
				sep = " ."
			}
			pred := g.turtleIRI(t.P)
			if t.P == NSRDF+"type" {
				pred = "a"
			}
			fmt.Fprintf(&b, "\n    %s %s%s", pred, g.turtleTerm(t.O), sep)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// ---------------------------------------------------------------- RDF/XML

func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\r':
			b.WriteString("&#13;")
		default:
			if r == 0x9 || r == 0xA || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// WriteRDFXML writes the graph as RDF/XML: one rdf:Description per
// subject. Predicates outside the known prefixes get their own namespace
// declaration.
func (g *Graph) WriteRDFXML(w io.Writer) error {
	var body strings.Builder
	extra := map[string]string{} // namespace → generated prefix
	qname := func(iri string) string {
		if p, l, ok := g.split(iri); ok {
			return p + ":" + l
		}
		// cut after the last # or /
		i := strings.LastIndexAny(iri, "#/")
		ns, local := iri[:i+1], iri[i+1:]
		p, ok := extra[ns]
		if !ok {
			p = fmt.Sprintf("ns%d", len(extra)+1)
			extra[ns] = p
		}
		return p + ":" + local
	}
	for _, s := range g.subjects() {
		if s.BNode != "" {
			fmt.Fprintf(&body, "  <rdf:Description rdf:nodeID=\"%s\">\n", xmlEsc(s.BNode))
		} else {
			fmt.Fprintf(&body, "  <rdf:Description rdf:about=\"%s\">\n", xmlEsc(s.IRI))
		}
		for _, t := range g.about(s) {
			q := qname(t.P)
			switch {
			case t.O.isLit && t.O.Datatype != "":
				fmt.Fprintf(&body, "    <%s rdf:datatype=\"%s\">%s</%s>\n", q, xmlEsc(t.O.Datatype), xmlEsc(t.O.Literal), q)
			case t.O.isLit:
				fmt.Fprintf(&body, "    <%s>%s</%s>\n", q, xmlEsc(t.O.Literal), q)
			case t.O.BNode != "":
				fmt.Fprintf(&body, "    <%s rdf:nodeID=\"%s\"/>\n", q, xmlEsc(t.O.BNode))
			default:
				fmt.Fprintf(&body, "    <%s rdf:resource=\"%s\"/>\n", q, xmlEsc(t.O.IRI))
			}
		}
		body.WriteString("  </rdf:Description>\n")
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<rdf:RDF")
	for _, p := range g.sortedPrefixes() {
		fmt.Fprintf(&b, "\n    xmlns:%s=\"%s\"", p, xmlEsc(g.Prefixes[p]))
	}
	nss := make([]string, 0, len(extra))
	for ns := range extra {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	for _, ns := range nss {
		fmt.Fprintf(&b, "\n    xmlns:%s=\"%s\"", extra[ns], xmlEsc(ns))
	}
	b.WriteString(">\n" + body.String() + "</rdf:RDF>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// ---------------------------------------------------------------- JSON-LD

// WriteJSONLD writes the graph as compacted-by-prefix JSON-LD with a
// @graph of node objects.
func (g *Graph) WriteJSONLD(w io.Writer) error {
	ctx := map[string]any{}
	for p, ns := range g.Prefixes {
		ctx[p] = ns
	}
	compact := func(iri string) string {
		if p, l, ok := g.split(iri); ok {
			return p + ":" + l
		}
		return iri
	}
	var nodes []map[string]any
	for _, s := range g.subjects() {
		n := map[string]any{}
		if s.BNode != "" {
			n["@id"] = "_:" + s.BNode
		} else {
			n["@id"] = s.IRI
		}
		for _, t := range g.about(s) {
			var key string
			var val any
			switch {
			case t.P == NSRDF+"type":
				key, val = "@type", compact(t.O.IRI)
			case t.O.isLit && t.O.Datatype != "":
				key, val = compact(t.P), map[string]any{"@value": t.O.Literal, "@type": compact(t.O.Datatype)}
			case t.O.isLit:
				key, val = compact(t.P), t.O.Literal
			case t.O.BNode != "":
				key, val = compact(t.P), map[string]any{"@id": "_:" + t.O.BNode}
			default:
				key, val = compact(t.P), map[string]any{"@id": t.O.IRI}
			}
			switch cur := n[key].(type) {
			case nil:
				n[key] = val
			case []any:
				n[key] = append(cur, val)
			default:
				n[key] = []any{cur, val}
			}
		}
		nodes = append(nodes, n)
	}
	if nodes == nil {
		nodes = []map[string]any{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"@context": ctx, "@graph": nodes})
}

// ---------------------------------------------------------------- formats

// Media types.
const (
	MediaRDFXML = "application/rdf+xml"
	MediaTurtle = "text/turtle"
	MediaJSONLD = "application/ld+json"
)

// Negotiate picks the media type for an Accept header (RDF/XML by
// default, as OSLC 2.0 requires).
func Negotiate(accept string) string {
	type cand struct {
		media string
		q     float64
		pos   int
	}
	var best *cand
	for i, part := range strings.Split(accept, ",") {
		fields := strings.Split(part, ";")
		mt := strings.ToLower(strings.TrimSpace(fields[0]))
		q := 1.0
		for _, f := range fields[1:] {
			if k, v, ok := strings.Cut(strings.TrimSpace(f), "="); ok && strings.TrimSpace(k) == "q" {
				fmt.Sscanf(strings.TrimSpace(v), "%g", &q)
			}
		}
		var media string
		switch mt {
		case MediaRDFXML, "application/xml", "text/xml":
			media = MediaRDFXML
		case MediaTurtle, "application/x-turtle":
			media = MediaTurtle
		case MediaJSONLD, "application/json":
			media = MediaJSONLD
		default:
			continue
		}
		if q <= 0 {
			continue
		}
		if best == nil || q > best.q {
			best = &cand{media, q, i}
		}
	}
	if best == nil {
		return MediaRDFXML
	}
	return best.media
}

// Write renders g in media type mt.
func (g *Graph) Write(w io.Writer, mt string) error {
	switch mt {
	case MediaTurtle:
		return g.WriteTurtle(w)
	case MediaJSONLD:
		return g.WriteJSONLD(w)
	}
	return g.WriteRDFXML(w)
}
