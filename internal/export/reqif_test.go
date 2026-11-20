package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/reqimport"
)

func TestMarkdownToXHTML(t *testing.T) {
	cases := []struct{ md, want string }{
		{"", "<xhtml:div></xhtml:div>"},
		{"Plain & <simple>", "<xhtml:div><xhtml:p>Plain &amp; &lt;simple&gt;</xhtml:p></xhtml:div>"},
		{"a\nb\n\nc", "<xhtml:div><xhtml:p>a<xhtml:br/>b</xhtml:p><xhtml:p>c</xhtml:p></xhtml:div>"},
		{"# Title #\n- one\n* two\n\n1. x\n2) y", "<xhtml:div><xhtml:h1>Title</xhtml:h1><xhtml:ul><xhtml:li>one</xhtml:li><xhtml:li>two</xhtml:li></xhtml:ul>" +
			"<xhtml:ol><xhtml:li>x</xhtml:li><xhtml:li>y</xhtml:li></xhtml:ol></xhtml:div>"},
		{"```\n  code <x>\n```\nafter", "<xhtml:div><xhtml:pre>  code &lt;x&gt;</xhtml:pre><xhtml:p>after</xhtml:p></xhtml:div>"},
		{"```\nunclosed", "<xhtml:div><xhtml:pre>unclosed</xhtml:pre></xhtml:div>"},
		{"The **system** *shall* use `a*b` and [docs](https://x.org/?a=1&b=2).",
			`<xhtml:div><xhtml:p>The <xhtml:strong>system</xhtml:strong> <xhtml:em>shall</xhtml:em> use <xhtml:code>a*b</xhtml:code> and <xhtml:a href="https://x.org/?a=1&amp;b=2">docs</xhtml:a>.</xhtml:p></xhtml:div>`},
		{"5 * 3 * 2", "<xhtml:div><xhtml:p>5 * 3 * 2</xhtml:p></xhtml:div>"},
		{"[x](javascript:alert(1)) ![p](data:x)", "<xhtml:div><xhtml:p>[x](javascript:alert(1)) ![p](data:x)</xhtml:p></xhtml:div>"},
		{"![Plan](img/plan.JPG)", `<xhtml:div><xhtml:p><xhtml:object data="img/plan.JPG" type="image/jpeg">Plan</xhtml:object></xhtml:p></xhtml:div>`},
	}
	for _, c := range cases {
		if got := MarkdownToXHTML(c.md, nil); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.md, got, c.want)
		}
	}
	got := MarkdownToXHTML("![a](/api/v1/files/f1/content)", func(u string) (string, string) { return "files/R/a.png", "image/png" })
	if !strings.Contains(got, `data="files/R/a.png" type="image/png"`) {
		t.Error(got)
	}
	for name, want := range map[string]string{"a.gif": "image/gif", "a.svg?x": "image/svg+xml", "a.webp": "image/webp", "a.jpeg": "image/jpeg", "a": "image/png"} {
		if imageType(name) != want {
			t.Errorf("%s: %s", name, imageType(name))
		}
	}
}

func reqifSample() ReqIFDocument {
	at := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return ReqIFDocument{
		ProjectID: "p1", ProjectKey: "SHOP", Title: "SHOP – Webshop", Comment: "test", ToolVersion: "1.2.3", GeneratedAt: at,
		Folders: []ReqIFFolder{{ID: "f1", Name: "Security", CreatedAt: at}, {ID: "f2", ParentID: "f1", Name: "Login", CreatedAt: at},
			{ID: "f3", Name: "Empty"}},
		Requirements: []ReqIFRequirement{
			{ID: "r1", Key: "SHOP-REQ-1", Title: "Lockout", Description: "The system **SHALL** lock accounts.\n\n- after 5 attempts\n- for 10 min",
				Obligation: "shall", Type: "security", Priority: "high", Status: "approved", Source: "Customer", Rationale: "Brute force\nprotection",
				Owner: "anna", Tags: []string{"auth", "web"}, Categories: []string{"Area/Web"}, Version: 3, FolderID: "f2", CreatedBy: "a@x",
				Criteria:  []ReqIFCriterion{{Description: "Locked after 5 failures", Method: "test", Threshold: "5"}, {Description: "Reviewed"}},
				CreatedAt: at, UpdatedAt: at},
			{ID: "r2", Key: "SHOP-REQ-2", Title: "Users MAY export", Description: "The user MAY export data.", Obligation: "may",
				Type: "non_functional", Priority: "low", Status: "draft", FolderID: "f1"},
			{ID: "r3", Key: "SHOP-REQ-3", Title: "Top level", Description: "Line one\nLine two", Obligation: "must_not", Status: "weird", FolderID: "missing"},
		},
		Relations: []ReqIFRelation{{ID: "x1", Type: "refines", SourceID: "r2", TargetID: "r1"}, {ID: "x2", Type: "refines", SourceID: "r2", TargetID: "gone"},
			{ID: "x3", Type: "depends_on", SourceID: "r1", TargetID: "r3"}},
	}
}

// wellFormed decodes the whole document.
func wellFormed(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
	}
}

func TestWriteReqIF(t *testing.T) {
	var b bytes.Buffer
	if err := WriteReqIF(&b, reqifSample()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	wellFormed(t, b.Bytes())
	for _, s := range []string{
		`<REQ-IF xmlns="http://www.omg.org/spec/ReqIF/20110401/reqif.xsd" xmlns:xhtml="http://www.w3.org/1999/xhtml">`,
		"<REQ-IF-TOOL-ID>Qtestframe</REQ-IF-TOOL-ID>", "<SOURCE-TOOL-ID>Qtestframe 1.2.3</SOURCE-TOOL-ID>",
		"<REQ-IF-VERSION>1.0</REQ-IF-VERSION>", "<CREATION-TIME>2026-11-20T10:00:00.000Z</CREATION-TIME>",
		`<ENUM-VALUE IDENTIFIER="qtf-ev-obligation-must-not" LAST-CHANGE="2026-11-20T10:00:00.000Z" LONG-NAME="MUST NOT">`,
		`LONG-NAME="Non-functional"`, `OTHER-CONTENT="non_functional"`,
		`<SPEC-OBJECT IDENTIFIER="qtf-req-r1" LAST-CHANGE="2026-11-20T10:00:00.000Z" LONG-NAME="SHOP-REQ-1">`,
		`THE-VALUE="Brute force&#xA;protection"`, `<xhtml:strong>SHALL</xhtml:strong>`,
		`<xhtml:li>test: Locked after 5 failures (threshold: 5)</xhtml:li><xhtml:li>Reviewed</xhtml:li>`,
		`<SPEC-RELATION IDENTIFIER="qtf-rel-x1"`, "<SPEC-RELATION-TYPE-REF>qtf-reltype-depends-on</SPEC-RELATION-TYPE-REF>",
		`<SPECIFICATION IDENTIFIER="qtf-spec-p1"`, "<SPEC-OBJECT-REF>qtf-folder-f2</SPEC-OBJECT-REF>",
		`<ATTRIBUTE-DEFINITION-ENUMERATION IDENTIFIER="qtf-ad-status" LAST-CHANGE="2026-11-20T10:00:00.000Z" LONG-NAME="Status" MULTI-VALUED="false">`,
		`<ATTRIBUTE-VALUE-DATE THE-VALUE="2026-11-20T10:00:00.000Z">`, `<ATTRIBUTE-VALUE-INTEGER THE-VALUE="3">`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %s", s)
		}
	}
	if strings.Contains(out, "qtf-rel-x2") || strings.Contains(out, "qtf-ev-status-weird") || strings.Contains(out, "qtf-ad-attachments</ATTRIBUTE-DEFINITION-XHTML-REF>") {
		t.Error("dangling relation, invalid enum value or attachments in a plain .reqif")
	}
	// Element order of the standard: header before content, the content
	// sections in schema order, VALUES before TYPE in objects.
	order := []string{"<THE-HEADER>", "<CORE-CONTENT>", "<DATATYPES>", "<SPEC-TYPES>", "<SPEC-OBJECTS>", "<SPEC-RELATIONS>", "<SPECIFICATIONS>"}
	last := -1
	for _, o := range order {
		i := strings.Index(out, o)
		if i <= last {
			t.Errorf("%s out of order", o)
		}
		last = i
	}
	obj := out[strings.Index(out, `<SPEC-OBJECT IDENTIFIER="qtf-req-r1"`):]
	obj = obj[:strings.Index(obj, "</SPEC-OBJECT>")]
	if strings.Index(obj, "<VALUES>") > strings.Index(obj, "<TYPE>") {
		t.Error("TYPE before VALUES")
	}
	// The hierarchy: Security > (REQ-2, Login > REQ-1), REQ-3 at the top
	// (its folder is unknown); the empty folder is a heading without children.
	spec := out[strings.Index(out, "<SPECIFICATIONS>"):]
	if !(strings.Index(spec, "qtf-req-r3") < strings.Index(spec, "qtf-folder-f1") &&
		strings.Index(spec, "qtf-folder-f1") < strings.Index(spec, "qtf-req-r2") &&
		strings.Index(spec, "qtf-req-r2") < strings.Index(spec, "qtf-folder-f2") &&
		strings.Index(spec, "qtf-folder-f2") < strings.Index(spec, "qtf-req-r1")) {
		t.Errorf("hierarchy:\n%s", spec)
	}
	if err := WriteReqIF(&failWriter{}, reqifSample()); err == nil {
		t.Error("write error not returned")
	}
	if n := ReqIFFileName("SHOP", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), true); n != "SHOP-20260102-030405.reqifz" {
		t.Error(n)
	}
	if n := ReqIFFileName("SHOP", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), false); n != "SHOP-20260102-030405.reqif" {
		t.Error(n)
	}
}

// TestReqIFRoundTrip imports an export with the requirement importer.
func TestReqIFRoundTrip(t *testing.T) {
	doc := reqifSample()
	doc.Requirements[0].Description = "The system **SHALL** lock accounts after `5` failed *attempts*.\nSee [policy](https://example.org/p).\n\n" +
		"- after 5 attempts\n- for 10 min\n\n1. first\n2. second\n\n## Notes\n\n```\nif x {\n    lock()\n}\n```\n\nLast & final <line>"
	var b bytes.Buffer
	if err := WriteReqIF(&b, doc); err != nil {
		t.Fatal(err)
	}
	res, err := reqimport.Parse(b.Bytes(), "SHOP.reqif", reqimport.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 3 || len(res.Errors) != 0 {
		t.Fatalf("candidates %d, errors %v", len(res.Candidates), res.Errors)
	}
	byID := map[string]reqimport.Candidate{}
	for _, c := range res.Candidates {
		byID[c.ExternalID] = c
	}
	for _, rq := range doc.Requirements {
		c, ok := byID[rq.Key]
		if !ok {
			t.Fatalf("%s missing: %+v", rq.Key, res.Candidates)
		}
		status := rq.Status
		if status == "weird" {
			status = ""
		}
		if c.Title != rq.Title || c.Description != rq.Description || c.Obligation != rq.Obligation || c.Type != rq.Type ||
			c.Priority != rq.Priority || c.Status != status || c.Source != rq.Source || c.Rationale != rq.Rationale || c.Owner != rq.Owner {
			t.Errorf("%s:\n got %+v\nwant %+v", rq.Key, c, rq)
		}
		if strings.Join(c.Tags, ",") != strings.Join(rq.Tags, ",") {
			t.Errorf("%s tags %v", rq.Key, c.Tags)
		}
		if len(c.Criteria) != len(rq.Criteria) {
			t.Errorf("%s criteria %+v", rq.Key, c.Criteria)
		}
		for i, cr := range rq.Criteria {
			if i < len(c.Criteria) && (c.Criteria[i].Description != cr.Description || c.Criteria[i].Method != cr.Method || c.Criteria[i].Threshold != cr.Threshold) {
				t.Errorf("%s criterion %d: %+v", rq.Key, i, c.Criteria[i])
			}
		}
	}
	if byID["SHOP-REQ-1"].Folder != "Security/Login" || byID["SHOP-REQ-2"].Folder != "Security" || byID["SHOP-REQ-3"].Folder != "" {
		t.Errorf("folders: %q %q %q", byID["SHOP-REQ-1"].Folder, byID["SHOP-REQ-2"].Folder, byID["SHOP-REQ-3"].Folder)
	}
}

func TestWriteReqIFZ(t *testing.T) {
	doc := reqifSample()
	doc.Requirements[0].Description = "See ![diagram](/api/v1/files/a1/content) and ![other](/api/v1/files/zz/content)"
	doc.Requirements[0].Attachments = []ReqIFFile{
		{ID: "a1", Name: "diagram.png", ContentType: "image/png", Data: []byte("PNG")},
		{ID: "a2", Name: "diagram.png", Load: func() ([]byte, error) { return []byte("second"), nil }},
		{ID: "a3", Name: "../", Data: []byte("x")},
	}
	doc.ProjectKey = ""
	doc.Requirements[0].Key = ""
	var b bytes.Buffer
	if err := WriteReqIFZ(&b, doc); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(data)
	}
	main := files["requirements.reqif"]
	if files["files/requirement/diagram.png"] != "PNG" || files["files/requirement/diagram-2.png"] != "second" || files["files/requirement/file"] != "x" || main == "" {
		t.Fatalf("archive: %v", len(files))
	}
	wellFormed(t, []byte(main))
	for _, s := range []string{`<xhtml:object data="files/requirement/diagram.png" type="image/png">diagram</xhtml:object>`,
		`<xhtml:object data="/api/v1/files/zz/content" type="image/png">other</xhtml:object>`,
		`<xhtml:li><xhtml:object data="files/requirement/diagram-2.png" type="application/octet-stream">diagram.png</xhtml:object></xhtml:li>`} {
		if !strings.Contains(main, s) {
			t.Errorf("missing %s", s)
		}
	}
	// The archive imports like the plain document.
	res, err := reqimport.Parse(b.Bytes(), "x.reqifz", reqimport.Options{})
	if err != nil || len(res.Candidates) != 3 {
		t.Fatalf("%v %+v", err, res)
	}
	// Load errors are returned.
	doc.Requirements[0].Attachments = []ReqIFFile{{ID: "e", Name: "e", Load: func() ([]byte, error) { return nil, errors.New("boom") }}}
	if err := WriteReqIFZ(io.Discard, doc); err == nil {
		t.Error("load error not returned")
	}
	if err := WriteReqIFZ(&failWriter{}, reqifSample()); err == nil {
		t.Error("write error not returned")
	}
}

func TestReqIFLabel(t *testing.T) {
	for _, c := range [][3]string{{"obligation", "should_not", "SHOULD NOT"}, {"type", "non_functional", "Non-functional"}, {"", "depends_on", "Depends on"}, {"status", "", ""}} {
		if got := reqifLabel(c[0], c[1]); got != c[2] {
			t.Errorf("%v: %q", c, got)
		}
	}
	if reqifTime(time.Time{}) != "1970-01-01T00:00:00.000Z" {
		t.Error(reqifTime(time.Time{}))
	}
}
