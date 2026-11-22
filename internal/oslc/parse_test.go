package oslc

import (
	"bytes"
	"strings"
	"testing"
)

func lits(g *Graph, s Term, p string) []string {
	var out []string
	for _, o := range g.Objects(s, p) {
		if o.IsLiteral() {
			out = append(out, o.Literal)
		} else {
			out = append(out, "<"+o.IRI+o.BNode+">")
		}
	}
	return out
}

func TestParseTurtle(t *testing.T) {
	doc := `@prefix dcterms: <http://purl.org/dc/terms/> .
PREFIX oslc_rm: <http://open-services.net/ns/rm#>
@base <http://h/x/> .
# a comment
<> a oslc_rm:Requirement ;
   dcterms:title "Brake \"fast\"" , 'Other' ;
   dcterms:description """Line 1
Line 2"""@en ;
   oslc_rm:validatedBy <tests/1>, <http://ext/tc/9> ;
   <http://q/n> 42, -1.5, true ;
   dcterms:creator [ <http://xmlns.com/foaf/0.1/name> "Anna" ] ;
   dcterms:subject "a"^^<http://www.w3.org/2001/XMLSchema#string> ; .
_:b1 dcterms:title "blank" .`
	g, err := Parse([]byte(doc), MediaTurtle, "http://h/x/new")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := g.SubjectOf(NSRM+"Requirement", "")
	if !ok || s.IRI != "http://h/x/" {
		t.Fatalf("subject %+v", s)
	}
	if got := lits(g, s, NSDCTerms+"title"); strings.Join(got, "|") != `Brake "fast"|Other` {
		t.Errorf("titles %v", got)
	}
	if got := lits(g, s, NSDCTerms+"description"); len(got) != 1 || got[0] != "Line 1\nLine 2" {
		t.Errorf("description %q", got)
	}
	if got := lits(g, s, NSRM+"validatedBy"); strings.Join(got, " ") != "<http://h/x/tests/1> <http://ext/tc/9>" {
		t.Errorf("links %v", got)
	}
	if got := lits(g, s, "http://q/n"); strings.Join(got, " ") != "42 -1.5 true" {
		t.Errorf("numbers %v", got)
	}
	if c := g.Objects(s, NSDCTerms+"creator"); len(c) != 1 || lits(g, c[0], NSFOAF+"name")[0] != "Anna" {
		t.Errorf("creator %v", c)
	}
	if o := g.Objects(s, NSDCTerms+"subject"); len(o) != 1 || o[0].Datatype != "" {
		t.Errorf("xsd:string %v", o)
	}
	for _, bad := range []string{`<a> <b> "x`, `<a> <b> ( 1 ) .`, `x:y <b> "c" .`, `<a> <b> "c"`} {
		if _, err := Parse([]byte(bad), MediaTurtle, "http://h/"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseRDFXML(t *testing.T) {
	doc := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dcterms="http://purl.org/dc/terms/"
  xmlns:oslc_rm="http://open-services.net/ns/rm#" xmlns:q="http://q/">
  <oslc_rm:Requirement rdf:about="" q:short="S1">
    <dcterms:title>Brake &amp; stop</dcterms:title>
    <dcterms:description rdf:parseType="Literal"><div xmlns="http://www.w3.org/1999/xhtml"><p>It <b>shall</b></p></div></dcterms:description>
    <oslc_rm:validatedBy rdf:resource="http://h/tests/1"/>
    <q:n rdf:datatype="http://www.w3.org/2001/XMLSchema#integer">3</q:n>
    <q:node><rdf:Description rdf:nodeID="x"><dcterms:title>inner</dcterms:title></rdf:Description></q:node>
    <q:res rdf:parseType="Resource"><dcterms:title>res</dcterms:title></q:res>
  </oslc_rm:Requirement>
</rdf:RDF>`
	g, err := Parse([]byte(doc), MediaRDFXML, "http://h/r/1")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := g.SubjectOf(NSRM+"Requirement", "")
	if s.IRI != "http://h/r/1" || lits(g, s, NSDCTerms+"title")[0] != "Brake & stop" || lits(g, s, "http://q/short")[0] != "S1" {
		t.Fatalf("%+v %v", s, g.Triples)
	}
	d := g.Objects(s, NSDCTerms+"description")
	if len(d) != 1 || d[0].Datatype != XMLLiteral || !strings.Contains(d[0].Literal, "<b>shall</b>") {
		t.Errorf("literal %+v", d)
	}
	if got := lits(g, s, NSRM+"validatedBy"); got[0] != "<http://h/tests/1>" {
		t.Errorf("resource %v", got)
	}
	if n := g.Objects(s, "http://q/n"); n[0].Datatype != NSXSD+"integer" {
		t.Errorf("datatype %+v", n)
	}
	for _, p := range []string{"http://q/node", "http://q/res"} {
		o := g.Objects(s, p)
		if len(o) != 1 || len(lits(g, o[0], NSDCTerms+"title")) != 1 {
			t.Errorf("%s: %+v", p, o)
		}
	}
	if _, err := Parse([]byte("<rdf:RDF"), MediaRDFXML, ""); err == nil {
		t.Error("broken XML accepted")
	}
}

func TestParseJSONLDRoundTrip(t *testing.T) {
	g := NewGraph(map[string]string{"qtf": "http://h/vocab#"})
	s := IRI("http://h/r/1")
	g.Type(s, NSRM+"Requirement")
	g.Add(s, NSDCTerms+"title", Lit("T"))
	g.Add(s, NSRM+"validatedBy", IRI("http://h/t/1"))
	g.Add(s, NSRM+"validatedBy", IRI("http://h/t/2"))
	g.Add(s, "http://h/vocab#version", Typed("3", NSXSD+"integer"))
	for _, mt := range []string{MediaJSONLD, MediaTurtle, MediaRDFXML} {
		var b bytes.Buffer
		if err := g.Write(&b, mt); err != nil {
			t.Fatal(err)
		}
		back, err := Parse(b.Bytes(), mt, "http://h/")
		if err != nil {
			t.Fatalf("%s: %v\n%s", mt, err, b.String())
		}
		subj, ok := back.SubjectOf(NSRM+"Requirement", "")
		if !ok || subj.IRI != "http://h/r/1" || len(back.Objects(subj, NSRM+"validatedBy")) != 2 || lits(back, subj, NSDCTerms+"title")[0] != "T" ||
			back.Objects(subj, "http://h/vocab#version")[0].Literal != "3" {
			t.Errorf("%s round trip: %+v", mt, back.Triples)
		}
	}
	// OSLC 2.0 JSON and plain JSON-LD with terms
	for _, doc := range []string{
		`{"prefixes":{"dcterms":"http://purl.org/dc/terms/","oslc_rm":"http://open-services.net/ns/rm#"},"rdf:about":"http://h/r/2",
		  "rdf:type":[{"rdf:resource":"http://open-services.net/ns/rm#Requirement"}],"dcterms:title":"J","oslc_rm:validatedBy":[{"rdf:resource":"http://h/t/1"}]}`,
		`{"@context":{"title":"http://purl.org/dc/terms/title","vb":{"@id":"http://open-services.net/ns/rm#validatedBy","@type":"@id"}},
		  "@id":"http://h/r/2","@type":"http://open-services.net/ns/rm#Requirement","title":"J","vb":"http://h/t/1","unknown":"dropped"}`,
	} {
		back, err := Parse([]byte(doc), MediaJSONLD, "http://h/")
		if err != nil {
			t.Fatal(err)
		}
		subj, _ := back.SubjectOf(NSRM+"Requirement", "")
		if subj.IRI != "http://h/r/2" || lits(back, subj, NSDCTerms+"title")[0] != "J" || lits(back, subj, NSRM+"validatedBy")[0] != "<http://h/t/1>" {
			t.Errorf("json: %+v", back.Triples)
		}
	}
	if _, err := Parse([]byte("{"), MediaJSONLD, ""); err == nil {
		t.Error("broken JSON accepted")
	}
	if _, err := Parse([]byte("x"), "text/plain", ""); err == nil {
		t.Error("unknown media type accepted")
	}
	if MediaOf("text/turtle; charset=utf-8") != MediaTurtle || MediaOf("application/json") != MediaJSONLD || MediaOf("text/html") != "" {
		t.Error("MediaOf")
	}
}
