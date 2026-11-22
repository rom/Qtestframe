package oslc

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

func sample() *Graph {
	g := NewGraph(map[string]string{"qtf": "http://example.test/vocab#"})
	r := IRI("http://example.test/r/1")
	g.Type(r, NSRM+"Requirement")
	g.Add(r, NSDCTerms+"title", Lit(`Login "quoted" & <b>`+"\nline"))
	g.Add(r, NSDCTerms+"modified", Typed("2026-01-02T03:04:05Z", NSXSD+"dateTime"))
	g.Add(r, NSDCTerms+"description", Lit("")) // left out
	g.Add(r, NSDCTerms+"subject", Lit("a"))
	g.Add(r, NSDCTerms+"subject", Lit("b"))
	g.Add(r, "http://other.example/ns/x", IRI("http://example.test/t/1"))
	g.Add(r, "http://example.test/vocab#9bad", Lit("odd local"))
	b := Blank("p1")
	g.Add(r, NSDCTerms+"creator", b)
	g.Type(b, NSFOAF+"Person")
	g.Add(b, NSFOAF+"name", Lit("ann"))
	g.Add(r, NSRDFS+"seeAlso", Term{}) // empty: left out
	return g
}

func TestWriters(t *testing.T) {
	g := sample()
	var x bytes.Buffer
	if err := g.Write(&x, MediaRDFXML); err != nil {
		t.Fatal(err)
	}
	var doc struct{ XMLName xml.Name }
	if err := xml.Unmarshal(x.Bytes(), &doc); err != nil {
		t.Fatalf("RDF/XML: %v\n%s", err, x.String())
	}
	for _, want := range []string{`rdf:about="http://example.test/r/1"`, `&quot;quoted&quot; &amp; &lt;b&gt;`, `rdf:datatype="http://www.w3.org/2001/XMLSchema#dateTime"`,
		`rdf:nodeID="p1"`, `xmlns:ns1="http://other.example/ns/"`, `<ns1:x rdf:resource=`} {
		if !strings.Contains(x.String(), want) {
			t.Errorf("RDF/XML lacks %s:\n%s", want, x.String())
		}
	}
	if strings.Contains(x.String(), "description") || strings.Contains(x.String(), "seeAlso") {
		t.Errorf("empty values written:\n%s", x.String())
	}
	var tt bytes.Buffer
	_ = g.Write(&tt, MediaTurtle)
	for _, want := range []string{"@prefix dcterms: <http://purl.org/dc/terms/> .", "a oslc_rm:Requirement ;", `"Login \"quoted\" & <b>\nline"`,
		`"2026-01-02T03:04:05Z"^^xsd:dateTime`, "dcterms:creator _:p1", "_:p1\n    a foaf:Person ;", "<http://other.example/ns/x>", "<http://example.test/vocab#9bad>"} {
		if !strings.Contains(tt.String(), want) {
			t.Errorf("Turtle lacks %s:\n%s", want, tt.String())
		}
	}
	var j bytes.Buffer
	_ = g.Write(&j, MediaJSONLD)
	var ld map[string]any
	if err := json.Unmarshal(j.Bytes(), &ld); err != nil {
		t.Fatal(err)
	}
	node := ld["@graph"].([]any)[0].(map[string]any)
	if node["@type"] != "oslc_rm:Requirement" || len(node["dcterms:subject"].([]any)) != 2 ||
		node["dcterms:creator"].(map[string]any)["@id"] != "_:p1" || node["dcterms:modified"].(map[string]any)["@type"] != "xsd:dateTime" {
		t.Errorf("JSON-LD: %s", j.String())
	}
	var empty bytes.Buffer
	_ = NewGraph(nil).WriteJSONLD(&empty)
	if !strings.Contains(empty.String(), `"@graph": []`) {
		t.Errorf("empty graph: %s", empty.String())
	}
}

func TestNegotiate(t *testing.T) {
	for accept, want := range map[string]string{
		"":                                       MediaRDFXML,
		"*/*":                                    MediaRDFXML,
		"text/turtle":                            MediaTurtle,
		"application/json":                       MediaJSONLD,
		"text/html, application/ld+json;q=0.9":   MediaJSONLD,
		"application/rdf+xml;q=0.5, text/turtle": MediaTurtle,
		"text/turtle;q=0, application/xml":       MediaRDFXML,
	} {
		if got := Negotiate(accept); got != want {
			t.Errorf("Negotiate(%q) = %s, want %s", accept, got, want)
		}
	}
}

func TestWhere(t *testing.T) {
	px, err := ParsePrefixes("dc=<http://purl.org/dc/terms/>, x=<urn:x#>", NewGraph(nil).Prefixes)
	if err != nil || px["dc"] != NSDCTerms || px["x"] != "urn:x#" || px["oslc"] != NSOSLC {
		t.Fatalf("prefixes: %v %v", px, err)
	}
	if _, err := ParsePrefixes("dc=http://x", nil); err == nil {
		t.Error("bad prefix accepted")
	}
	q, err := ParseWhere(`dc:identifier in ["A-1", "A-2"] and dcterms:modified>="2026-01-01T00:00:00Z"^^xsd:dateTime and dcterms:title!="x\"y"@en and x:n<3 and x:r=<urn:r>`, px)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Conditions) != 5 || q.Conditions[0].Op != "in" || len(q.Conditions[0].Values) != 2 || q.Conditions[2].Values[0] != `x"y` || !q.Conditions[4].IRIs[0] {
		t.Fatalf("conditions: %+v", q.Conditions)
	}
	c := q.Conditions
	if !c[0].Match("A-2", false) || c[0].Match("A-3", false) {
		t.Error("in")
	}
	if !c[1].Match("2026-03-01T10:00:00.5Z", true) || c[1].Match("2025-12-31T23:59:59Z", true) {
		t.Error(">= time")
	}
	if !c[2].Match("z", false) || c[2].Match(`x"y`, false) {
		t.Error("!=")
	}
	for op, want := range map[string][]bool{"<": {true, false, false}, ">": {false, false, true}, "<=": {true, true, false}, ">=": {false, true, true}, "=": {false, true, false}} {
		cond := Condition{Op: op, Values: []string{"b"}}
		for i, v := range []string{"a", "b", "c"} {
			if cond.Match(v, false) != want[i] {
				t.Errorf("%s %s", v, op)
			}
		}
	}
	if (Condition{Op: "?", Values: []string{"a"}}).Match("a", false) {
		t.Error("unknown op")
	}
	if !(Condition{Op: "=", Values: []string{"2026-01-01"}}).Match("2026-01-01T00:00:00Z", true) {
		t.Error("date only")
	}
	for _, bad := range []string{"", "title=\"x\"", "dcterms:title", "dcterms:title ~ 1", `dcterms:title="x`, "dcterms:title=x",
		`dcterms:title in "x"`, `dcterms:title in ["x" "y"]`, `dcterms:title="x" or dcterms:title="y"`, `nope:title="x"`, "dcterms:title=<urn", "dcterms:title=",
		`dcterms:title{x}="a"`} {
		if _, err := ParseWhere(bad, px); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if q, err := ParseWhere("dcterms:title=true and dcterms:title=1.5", px); err != nil || len(q.Conditions) != 2 {
		t.Errorf("literals: %v %v", q, err)
	}
}
