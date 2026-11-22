package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/oslc"
	"github.com/rom/qtestframe/internal/rbac"
)

// oslcClient is an OSLC consumer: an API token, RDF in and out.
type oslcClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *oslcClient) do(method, path, ctype, accept, ifMatch, body string) (int, http.Header, string) {
	c.t.Helper()
	u := path
	if i := strings.Index(u, "/api/"); i >= 0 {
		u = c.base + u[i:] // the configured base URL is not served by the test server
	}
	req, err := http.NewRequest(method, u, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	req.Header.Set("OSLC-Core-Version", "2.0")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(b)
}

func (c *oslcClient) graph(body, mt, base string) *oslc.Graph {
	c.t.Helper()
	g, err := oslc.Parse([]byte(body), mt, base)
	if err != nil {
		c.t.Fatalf("%v\n%s", err, body)
	}
	return g
}

func objects(g *oslc.Graph, s, p string) []string {
	var out []string
	for _, o := range g.Objects(oslc.IRI(s), p) {
		out = append(out, termValue(o))
	}
	return out
}

// TestOSLCWriteCycle is a remote tool working with the provider: discover
// the creation factory, create a requirement (Turtle) and a test case
// (RDF/XML) linked to it, query them, update with If-Match (and a stale
// ETag), link to a work item of another tool, a selective update, delete.
func TestOSLCWriteCycle(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("OW")
	parent := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Braking"}, 201)
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "oslc"}, 201)["token"].(string)
	c := &oslcClient{t: t, base: e.hs.URL, token: tok}

	// discovery: rootservices → catalog → provider → creation factory
	st, _, root := c.do("GET", "/api/v1/oslc/rootservices", "", "text/turtle", "", "")
	if st != 200 || !strings.Contains(root, "oslcCatalogs") {
		t.Fatalf("rootservices: %d %s", st, root)
	}
	st, _, cat := c.do("GET", "/api/v1/oslc/projects/"+pid+"/catalog", "", "text/turtle", "", "")
	if st != 200 || !strings.Contains(cat, "oslc:ServiceProviderCatalog") || !strings.Contains(cat, "/oslc/projects/"+pid+">") {
		t.Fatalf("project catalog: %d %s", st, cat)
	}
	_, _, sp := c.do("GET", "/api/v1/oslc/projects/"+pid, "", "application/rdf+xml", "", "")
	pg := c.graph(sp, oslc.MediaRDFXML, e.hs.URL)
	var factory, dialog string
	for _, tr := range pg.Triples {
		if tr.P == oslc.NSOSLC+"creation" && strings.HasSuffix(tr.O.IRI, "/requirements") {
			factory = tr.O.IRI
		}
		if tr.P == oslc.NSOSLC+"dialog" && strings.HasSuffix(tr.O.IRI, "/requirement/select") {
			dialog = tr.O.IRI
		}
	}
	if factory == "" || dialog == "" {
		t.Fatalf("no creation factory or selection dialog:\n%s", sp)
	}
	pub := factory[:strings.Index(factory, "/api/")]
	vocab := pub + "/api/v1/oslc/vocab#"
	st, _, shape := c.do("GET", "/api/v1/oslc/projects/"+pid+"/shapes/requirement", "", "text/turtle", "", "")
	if st != 200 || !strings.Contains(shape, "oslc:ResourceShape") || !strings.Contains(shape, "oslc_rm:validatedBy") {
		t.Errorf("shape: %d %s", st, shape)
	}

	// create a requirement (Turtle, XHTML description)
	parentIRI := pub + "/api/v1/oslc/projects/" + pid + "/requirements/" + parent["id"].(string)
	body := `@prefix dcterms: <http://purl.org/dc/terms/> .
@prefix oslc_rm: <http://open-services.net/ns/rm#> .
@prefix qtf: <` + vocab + `> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
<> a oslc_rm:Requirement ;
  dcterms:title "Emergency stop" ;
  dcterms:description "<div xmlns=\"http://www.w3.org/1999/xhtml\"><p>The car <b>shall</b> stop.</p></div>"^^rdf:XMLLiteral ;
  dcterms:subject "safety", "brakes" ;
  qtf:priority "high" ; qtf:obligation "shall" ; qtf:folder "Safety/Brakes" ;
  oslc_rm:elaborates <` + parentIRI + `> ;
  oslc_rm:implementedBy <https://tracker.example/issues/7> .`
	st, hd, out := c.do("POST", factory, "text/turtle", "text/turtle", "", body)
	if st != 201 || hd.Get("Location") == "" || hd.Get("ETag") != `"1"` {
		t.Fatalf("create: %d %v %s", st, hd, out)
	}
	req := hd.Get("Location")
	g := c.graph(out, oslc.MediaTurtle, e.hs.URL)
	if got := objects(g, req, oslc.NSDCTerms+"description"); len(got) != 1 || got[0] != "The car **shall** stop." {
		t.Errorf("description: %v", got)
	}
	if got := objects(g, req, vocab+"folder"); len(got) != 1 || got[0] != "Safety/Brakes" {
		t.Errorf("folder: %v", got)
	}
	if got := objects(g, req, oslc.NSRM+"elaborates"); len(got) != 1 || got[0] != parentIRI {
		t.Errorf("elaborates: %v", got)
	}
	if got := objects(g, req, oslc.NSRM+"implementedBy"); len(got) != 1 || got[0] != "https://tracker.example/issues/7" {
		t.Errorf("implementedBy: %v", got)
	}
	// the relation and the item link exist in Qtestframe
	reqID := req[strings.LastIndex(req, "/")+1:]
	rels := items(a.m("GET", "/api/v1/requirements/"+reqID+"/relations", nil, 200))
	if len(rels) != 1 || rels[0]["type"] != "refines" {
		t.Errorf("relations: %v", rels)
	}
	links := items(a.m("GET", "/api/v1/requirements/"+reqID+"/item-links", nil, 200))
	if len(links) != 1 || links[0]["url"] != "https://tracker.example/issues/7" || links[0]["label"] != "oslc_rm:implementedBy" {
		t.Errorf("item links: %v", links)
	}
	// the parent shows the inverse property
	_, _, pbody := c.do("GET", parentIRI, "", "text/turtle", "", "")
	if !strings.Contains(pbody, "oslc_rm:elaboratedBy <"+req+">") {
		t.Errorf("elaboratedBy: %s", pbody)
	}

	// create a test case (RDF/XML) that validates the requirement
	tbody := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dcterms="http://purl.org/dc/terms/"
  xmlns:oslc_qm="http://open-services.net/ns/qm#" xmlns:qtf="` + vocab + `">
  <oslc_qm:TestCase rdf:about="">
    <dcterms:title>Stop on obstacle</dcterms:title>
    <qtf:preconditions>Track is free</qtf:preconditions>
    <qtf:estimatedMinutes>15</qtf:estimatedMinutes>
    <oslc_qm:validatesRequirement rdf:resource="` + req + `"/>
    <oslc_qm:testsChangeRequest rdf:resource="https://tracker.example/issues/8"/>
  </oslc_qm:TestCase>
</rdf:RDF>`
	st, hd, out = c.do("POST", "/api/v1/oslc/projects/"+pid+"/tests", "application/rdf+xml", "application/ld+json", "", tbody)
	if st != 201 || !strings.Contains(out, "validatesRequirement") {
		t.Fatalf("create test: %d %s", st, out)
	}
	test := hd.Get("Location")
	_, _, rbody := c.do("GET", req, "", "application/ld+json", "", "")
	if !strings.Contains(rbody, test) {
		t.Errorf("validatedBy missing: %s", rbody)
	}

	// query: where on a vocabulary property and a link, select, order, search
	q := func(path string) *oslc.Graph {
		st, _, body := c.do("GET", path, "", "text/turtle", "", "")
		if st != 200 {
			t.Fatalf("%s: %d %s", path, st, body)
		}
		return c.graph(body, oslc.MediaTurtle, e.hs.URL)
	}
	qb := "/api/v1/oslc/projects/" + pid + "/requirements"
	g = q(qb + "?oslc.where=" + url.QueryEscape(`qtf:priority="high" and oslc_rm:validatedBy=<`+test+`>`) + "&oslc.select=" + url.QueryEscape("dcterms:title,oslc_rm:validatedBy"))
	if m := objects(g, pub+qb, oslc.NSRDFS+"member"); len(m) != 1 || m[0] != req {
		t.Fatalf("where: %v", m)
	}
	if objects(g, req, oslc.NSDCTerms+"description") != nil || objects(g, req, oslc.NSDCTerms+"title")[0] != "Emergency stop" {
		t.Errorf("select: %v", g.Triples)
	}
	g = q(qb + "?oslc.orderBy=" + url.QueryEscape("-dcterms:title") + "&oslc.select=dcterms:title")
	if m := objects(g, pub+qb, oslc.NSRDFS+"member"); len(m) != 2 {
		t.Errorf("members: %v", m)
	}
	g = q(qb + "?oslc.searchTerms=" + url.QueryEscape(`"emergency"`))
	if m := objects(g, pub+qb, oslc.NSRDFS+"member"); len(m) != 1 || m[0] != req {
		t.Errorf("search: %v", m)
	}
	g = q(qb + "?oslc.where=" + url.QueryEscape(`dcterms:subject!="safety"`))
	if m := objects(g, pub+qb, oslc.NSRDFS+"member"); len(m) != 1 || m[0] != parentIRI {
		t.Errorf("!=: %v", m)
	}
	for _, bad := range []string{"oslc.select=nope:x", "oslc.orderBy=nope:x"} {
		if st, _, _ := c.do("GET", qb+"?"+bad, "", "", "", ""); st != 400 {
			t.Errorf("%s: %d", bad, st)
		}
	}

	// update: stale ETag → 412, current ETag → 200 with a new ETag
	upd := `@prefix dcterms: <http://purl.org/dc/terms/> .
@prefix oslc_rm: <http://open-services.net/ns/rm#> .
<` + req + `> a oslc_rm:Requirement ; dcterms:title "Emergency stop v2" ;
  oslc_rm:implementedBy <https://tracker.example/issues/9> .`
	if st, _, body := c.do("PUT", req, "text/turtle", "text/turtle", `"7"`, upd); st != 412 || !strings.Contains(body, "oslc:Error") {
		t.Errorf("stale: %d %s", st, body)
	}
	st, hd, out = c.do("PUT", req, "text/turtle", "text/turtle", `"1"`, upd)
	if st != 200 || hd.Get("ETag") != `"2"` || !strings.Contains(out, "Emergency stop v2") || strings.Contains(out, "issues/7") || !strings.Contains(out, "issues/9") {
		t.Fatalf("update: %d %v %s", st, hd, out)
	}
	// properties left out stay; the relation and the trace link are kept
	if !strings.Contains(out, "oslc_rm:elaborates") || !strings.Contains(out, "oslc_rm:validatedBy") || !strings.Contains(out, "safety") {
		t.Errorf("kept: %s", out)
	}
	// a selective update clears the listed property the body leaves out
	sel := `@prefix dcterms: <http://purl.org/dc/terms/> . <` + req + `> dcterms:title "Emergency stop v3" .`
	st, _, out = c.do("PUT", req+"?oslc.properties="+url.QueryEscape("dcterms:title,oslc_rm:elaborates,dcterms:subject"), "text/turtle", "text/turtle", `"2"`, sel)
	if st != 200 || strings.Contains(out, "oslc_rm:elaborates") || strings.Contains(out, "safety") || !strings.Contains(out, "oslc_rm:validatedBy") {
		t.Errorf("selective: %d %s", st, out)
	}
	// errors
	if st, _, _ := c.do("PUT", req, "text/plain", "", "", "x"); st != 415 {
		t.Errorf("media type: %d", st)
	}
	if st, _, _ := c.do("POST", factory, "text/turtle", "", "", `<> <http://purl.org/dc/terms/description> "no title" .`); st != 400 {
		t.Errorf("no title: %d", st)
	}
	if st, _, _ := c.do("POST", factory, "text/turtle", "", "", `<> <http://purl.org/dc/terms/title> "x" ; <http://example.com/v#status> "y" ; <`+vocab+`status> "bogus" .`); st != 400 {
		t.Errorf("bad status: %d", st)
	}
	if st, _, _ := c.do("POST", factory, "text/turtle", "", "", `<> <http://purl.org/dc/terms/title> "x" ; <`+vocab+`refines> "lit" .`); st != 400 {
		t.Errorf("literal link: %d", st)
	}
	if st, _, _ := c.do("POST", factory, "text/turtle", "", "", `<> <http://purl.org/dc/terms/title> "x`); st != 400 {
		t.Errorf("syntax: %d", st)
	}

	// a read-only token cannot write; delete with the ETag
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	testPath := test[strings.Index(test, "/api/"):]
	if res := v.raw("DELETE", testPath, nil); res.StatusCode != 403 {
		t.Errorf("viewer delete: %d", res.StatusCode)
	}
	sp, _ = v.accept("/api/v1/oslc/projects/"+pid, "text/turtle", 200)
	if strings.Contains(sp, "creationFactory") || !strings.Contains(sp, "selectionDialog") {
		t.Errorf("viewer provider: %s", sp)
	}
	if st, _, _ := c.do("DELETE", test, "", "", `"9"`, ""); st != 412 {
		t.Errorf("stale delete: %d", st)
	}
	if st, _, _ := c.do("DELETE", test, "", "", `"1"`, ""); st != 204 {
		t.Errorf("delete: %d", st)
	}
	if st, _, _ := c.do("GET", test, "", "", "", ""); st != 404 {
		t.Errorf("deleted test: %d", st)
	}
}

// TestOSLCDialogs serves the delegated dialogs: HTML that may be framed by
// the configured origins, and the script.
func TestOSLCDialogs(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DL")
	res := e.anon().raw("GET", "/api/v1/oslc/projects/"+pid+"/dialogs/requirement/select", nil)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(b), `<html lang="en">`) ||
		!strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'self'") || res.Header.Get("X-Frame-Options") != "SAMEORIGIN" ||
		res.Header.Get("Cross-Origin-Opener-Policy") != "unsafe-none" {
		t.Fatalf("dialog: %d %v %s", res.StatusCode, res.Header, b)
	}
	c := e.anon()
	c.headers = map[string]string{"Accept-Language": "sv-SE,sv;q=0.9"}
	res = c.raw("GET", "/api/v1/oslc/projects/"+pid+"/dialogs/test/create", nil)
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "Nytt testfall") || !strings.Contains(string(b), `id="preconditions"`) {
		t.Errorf("swedish create dialog: %s", b)
	}
	if res := e.anon().raw("GET", "/api/v1/oslc/projects/"+pid+"/dialogs/run/select", nil); res.StatusCode != 404 {
		t.Errorf("unknown dialog: %d", res.StatusCode)
	}
	res = e.anon().raw("GET", "/api/v1/oslc/dialog.js", nil)
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "oslc-response:") || !strings.Contains(res.Header.Get("Content-Type"), "javascript") {
		t.Errorf("script: %s", res.Header.Get("Content-Type"))
	}
}
