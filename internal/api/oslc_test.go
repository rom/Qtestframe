package api

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func (c *client) accept(path, accept string, want int) (string, string) {
	c.e.t.Helper()
	old := c.headers
	c.headers = map[string]string{"Accept": accept}
	for k, v := range old {
		c.headers[k] = v
	}
	defer func() { c.headers = old }()
	res := c.raw("GET", path, nil)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("GET %s: status %d, want %d: %s", path, res.StatusCode, want, b)
	}
	return string(b), res.Header.Get("Content-Type")
}

func TestOSLCProvider(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("OS")
	base := "/api/v1/projects/" + pid
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Login & <access>", "description": "Users SHALL log in.", "tags": []string{"auth"}}, 201)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Logout"}, 201)
	a.m("POST", "/api/v1/requirements/"+r2["id"].(string)+"/relations", map[string]any{"target_id": r1["id"], "type": "refines"}, 201)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "Login test"}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": tc["id"]}, 201)
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "oslc"}, 201)["token"].(string)
	c := e.anon()
	c.bearer = tok

	// discovery is public
	root, ct := e.anon().accept("/api/v1/oslc/rootservices", "", 200)
	if !strings.HasPrefix(ct, "application/rdf+xml") || !strings.Contains(root, "rmServiceProviders") {
		t.Fatalf("rootservices: %s %s", ct, root)
	}
	e.anon().accept("/api/v1/oslc/catalog", "", 401)

	cat, _ := c.accept("/api/v1/oslc/catalog", "application/rdf+xml", 200)
	var parsed struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal([]byte(cat), &parsed); err != nil || parsed.XMLName.Local != "RDF" {
		t.Fatalf("catalog is not RDF/XML: %v %s", err, cat)
	}
	if !strings.Contains(cat, "ServiceProviderCatalog") || !strings.Contains(cat, "/oslc/projects/"+pid) {
		t.Errorf("catalog: %s", cat)
	}
	sp, ct := c.accept("/api/v1/oslc/projects/"+pid, "text/turtle", 200)
	if !strings.HasPrefix(ct, "text/turtle") || !strings.Contains(sp, "oslc:queryCapability") || !strings.Contains(sp, "oslc_rm:Requirement") || !strings.Contains(sp, "oslc_qm:TestCase") {
		t.Errorf("provider: %s", sp)
	}
	rq, _ := c.accept("/api/v1/oslc/projects/"+pid+"/requirements/"+r1["id"].(string), "", 200)
	if !strings.Contains(rq, "Login &amp; &lt;access&gt;") || !strings.Contains(rq, "oslc_rm:validatedBy") || !strings.Contains(rq, "/tests/"+tc["id"].(string)) {
		t.Errorf("requirement: %s", rq)
	}
	if err := xml.Unmarshal([]byte(rq), &parsed); err != nil {
		t.Errorf("requirement XML: %v", err)
	}
	// by key, as JSON-LD
	js, ct := c.accept("/api/v1/oslc/projects/"+pid+"/requirements/"+r2["key"].(string), "application/ld+json", 200)
	var ld map[string]any
	if err := json.Unmarshal([]byte(js), &ld); err != nil || !strings.HasPrefix(ct, "application/ld+json") {
		t.Fatalf("json-ld: %v %s", err, js)
	}
	if !strings.Contains(js, "oslc_rm:elaborates") || !strings.Contains(js, `"dcterms:identifier": "`+r2["key"].(string)+`"`) {
		t.Errorf("json-ld: %s", js)
	}
	tt, _ := c.accept("/api/v1/oslc/projects/"+pid+"/tests/"+tc["id"].(string), "text/turtle", 200)
	if !strings.Contains(tt, "oslc_qm:validatesRequirement") || !strings.Contains(tt, "a oslc_qm:TestCase") {
		t.Errorf("test: %s", tt)
	}
	c.accept("/api/v1/oslc/projects/"+pid+"/tests/nope", "", 404)

	// queries
	q := func(where string, extra string) string {
		body, _ := c.accept("/api/v1/oslc/projects/"+pid+"/requirements?oslc.where="+url.QueryEscape(where)+extra, "text/turtle", 200)
		return body
	}
	if body := q(`dcterms:identifier="`+r2["key"].(string)+`"`, ""); !strings.Contains(body, "Logout") || strings.Contains(body, "Users SHALL") ||
		!strings.Contains(body, `oslc:totalCount "1"`) {
		t.Errorf("identifier query: %s", body)
	}
	if body := q(`dcterms:title in ["Logout","Nothing"] and dcterms:modified>"2000-01-01T00:00:00Z"^^xsd:dateTime`, ""); !strings.Contains(body, `oslc:totalCount "1"`) {
		t.Errorf("in query: %s", body)
	}
	if body := q(`dc:modified<"2000-01-01"`, "&oslc.prefix="+url.QueryEscape("dc=<http://purl.org/dc/terms/>")); !strings.Contains(body, `oslc:totalCount "0"`) {
		t.Errorf("prefix query: %s", body)
	}
	// paging
	body, _ := c.accept("/api/v1/oslc/projects/"+pid+"/requirements?oslc.paging=true&oslc.pageSize=1", "text/turtle", 200)
	if !strings.Contains(body, "oslc:nextPage") || strings.Count(body, "rdfs:member") != 1 {
		t.Errorf("paging: %s", body)
	}
	for _, bad := range []string{`title="x"`, `dcterms:title "x"`, `dcterms:title=x`, `foo:title="x"`} {
		c.accept("/api/v1/oslc/projects/"+pid+"/requirements?oslc.where="+url.QueryEscape(bad), "", 400)
	}
	c.accept("/api/v1/oslc/projects/"+pid+"/tests?oslc.pageSize=0", "", 400)
	if body, _ := c.accept("/api/v1/oslc/projects/"+pid+"/tests", "application/json", 200); !strings.Contains(body, "Login test") {
		t.Errorf("tests query: %s", body)
	}

	// Basic authentication with the token as password
	basic := e.anon()
	basic.headers = map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("any:"+tok))}
	basic.accept("/api/v1/oslc/projects/"+pid, "", 200)
	basic.headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte("admin@example.com:"+testPassword))
	basic.accept("/api/v1/oslc/projects/"+pid, "", 401)
	// Basic with a token outside the OSLC paths is not accepted
	basic.headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte("any:"+tok))
	basic.m("GET", "/api/v1/projects", nil, 401)
}

func TestOSLCPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	open := a.project("OP")
	secret := a.project("SC")
	a.m("POST", "/api/v1/projects/"+secret+"/requirements", map[string]any{"title": "Hidden"}, 201)
	a.m("PUT", "/api/v1/projects/"+secret+"/access", map[string]any{"access": "restricted"}, 200)
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	cat, _ := v.accept("/api/v1/oslc/catalog", "text/turtle", 200)
	if !strings.Contains(cat, open) || strings.Contains(cat, secret) {
		t.Errorf("catalog shows the restricted project: %s", cat)
	}
	v.accept("/api/v1/oslc/projects/"+secret, "", 404)
	v.accept("/api/v1/oslc/projects/"+secret+"/requirements", "", 404)
	v.accept("/api/v1/oslc/projects/"+open+"/requirements", "", 200)
}
