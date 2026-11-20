package reqimport

import (
	"errors"
	"strings"
	"testing"
)

const testReqIF = `<?xml version="1.0" encoding="UTF-8"?>
<REQ-IF xmlns="http://www.omg.org/spec/ReqIF/20110401/reqif.xsd" xmlns:xhtml="http://www.w3.org/1999/xhtml">
  <THE-HEADER><REQ-IF-HEADER IDENTIFIER="h"><TITLE>Spec</TITLE></REQ-IF-HEADER></THE-HEADER>
  <CORE-CONTENT><REQ-IF-CONTENT>
    <DATATYPES>
      <DATATYPE-DEFINITION-STRING IDENTIFIER="dt-str" LONG-NAME="String"/>
      <DATATYPE-DEFINITION-XHTML IDENTIFIER="dt-x" LONG-NAME="XHTML"/>
      <DATATYPE-DEFINITION-ENUMERATION IDENTIFIER="dt-prio" LONG-NAME="Prio">
        <SPECIFIED-VALUES>
          <ENUM-VALUE IDENTIFIER="ev-high" LONG-NAME="High"/>
          <ENUM-VALUE IDENTIFIER="ev-low" LONG-NAME="Low"/>
        </SPECIFIED-VALUES>
      </DATATYPE-DEFINITION-ENUMERATION>
    </DATATYPES>
    <SPEC-TYPES>
      <SPEC-OBJECT-TYPE IDENTIFIER="t-req" LONG-NAME="Requirement">
        <SPEC-ATTRIBUTES>
          <ATTRIBUTE-DEFINITION-STRING IDENTIFIER="a-id" LONG-NAME="ReqIF.ForeignID"/>
          <ATTRIBUTE-DEFINITION-STRING IDENTIFIER="a-name" LONG-NAME="ReqIF.Name"/>
          <ATTRIBUTE-DEFINITION-XHTML IDENTIFIER="a-text" LONG-NAME="ReqIF.Text"/>
          <ATTRIBUTE-DEFINITION-XHTML IDENTIFIER="a-chap" LONG-NAME="ReqIF.ChapterName"/>
          <ATTRIBUTE-DEFINITION-ENUMERATION IDENTIFIER="a-prio" LONG-NAME="Priority"/>
          <ATTRIBUTE-DEFINITION-STRING IDENTIFIER="a-misc" LONG-NAME="Verification Notes"/>
          <ATTRIBUTE-DEFINITION-STRING IDENTIFIER="a-pre" LONG-NAME="ReqIF.Prefix"/>
        </SPEC-ATTRIBUTES>
      </SPEC-OBJECT-TYPE>
      <SPECIFICATION-TYPE IDENTIFIER="t-spec" LONG-NAME="Spec type"/>
    </SPEC-TYPES>
    <SPEC-OBJECTS>
      <SPEC-OBJECT IDENTIFIER="o-h1" LONG-NAME="Heading">
        <TYPE><SPEC-OBJECT-TYPE-REF>t-req</SPEC-OBJECT-TYPE-REF></TYPE>
        <VALUES>
          <ATTRIBUTE-VALUE-XHTML><DEFINITION><ATTRIBUTE-DEFINITION-XHTML-REF>a-chap</ATTRIBUTE-DEFINITION-XHTML-REF></DEFINITION>
            <THE-VALUE><xhtml:div>Security / Access</xhtml:div></THE-VALUE></ATTRIBUTE-VALUE-XHTML>
        </VALUES>
      </SPEC-OBJECT>
      <SPEC-OBJECT IDENTIFIER="o-1">
        <TYPE><SPEC-OBJECT-TYPE-REF>t-req</SPEC-OBJECT-TYPE-REF></TYPE>
        <VALUES>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="REQ-1"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>a-id</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="Lockout"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>a-name</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
          <ATTRIBUTE-VALUE-XHTML><DEFINITION><ATTRIBUTE-DEFINITION-XHTML-REF>a-text</ATTRIBUTE-DEFINITION-XHTML-REF></DEFINITION>
            <THE-VALUE><xhtml:div><xhtml:p>The system <xhtml:b>SHALL</xhtml:b> lock&#160;accounts.</xhtml:p><xhtml:ul><xhtml:li>after 5 attempts</xhtml:li><xhtml:li>for 10 min</xhtml:li></xhtml:ul>Line<xhtml:br/>break &amp; more</xhtml:div></THE-VALUE></ATTRIBUTE-VALUE-XHTML>
          <ATTRIBUTE-VALUE-ENUMERATION><DEFINITION><ATTRIBUTE-DEFINITION-ENUMERATION-REF>a-prio</ATTRIBUTE-DEFINITION-ENUMERATION-REF></DEFINITION>
            <VALUES><ENUM-VALUE-REF>ev-high</ENUM-VALUE-REF></VALUES></ATTRIBUTE-VALUE-ENUMERATION>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="x"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>a-pre</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
        </VALUES>
      </SPEC-OBJECT>
      <SPEC-OBJECT IDENTIFIER="o-2">
        <TYPE><SPEC-OBJECT-TYPE-REF>t-req</SPEC-OBJECT-TYPE-REF></TYPE>
        <VALUES>
          <ATTRIBUTE-VALUE-XHTML><DEFINITION><ATTRIBUTE-DEFINITION-XHTML-REF>a-text</ATTRIBUTE-DEFINITION-XHTML-REF></DEFINITION>
            <THE-VALUE><xhtml:div>Användaren BÖR få ett meddelande.</xhtml:div></THE-VALUE></ATTRIBUTE-VALUE-XHTML>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="checked by QA"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>a-misc</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
        </VALUES>
      </SPEC-OBJECT>
      <SPEC-OBJECT IDENTIFIER="o-empty"><TYPE><SPEC-OBJECT-TYPE-REF>t-req</SPEC-OBJECT-TYPE-REF></TYPE><VALUES/></SPEC-OBJECT>
      <SPEC-OBJECT IDENTIFIER="o-loose">
        <VALUES>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="Loose one MAY exist"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>a-name</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
          <ATTRIBUTE-VALUE-STRING THE-VALUE="ignored"><DEFINITION><ATTRIBUTE-DEFINITION-STRING-REF>unknown-def</ATTRIBUTE-DEFINITION-STRING-REF></DEFINITION></ATTRIBUTE-VALUE-STRING>
        </VALUES>
      </SPEC-OBJECT>
    </SPEC-OBJECTS>
    <SPECIFICATIONS>
      <SPECIFICATION IDENTIFIER="s-1" LONG-NAME="System spec">
        <TYPE><SPECIFICATION-TYPE-REF>t-spec</SPECIFICATION-TYPE-REF></TYPE>
        <CHILDREN>
          <SPEC-HIERARCHY IDENTIFIER="h-1"><OBJECT><SPEC-OBJECT-REF>o-h1</SPEC-OBJECT-REF></OBJECT>
            <CHILDREN>
              <SPEC-HIERARCHY IDENTIFIER="h-2"><OBJECT><SPEC-OBJECT-REF>o-1</SPEC-OBJECT-REF></OBJECT>
                <CHILDREN>
                  <SPEC-HIERARCHY IDENTIFIER="h-3"><OBJECT><SPEC-OBJECT-REF>o-2</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>
                </CHILDREN>
              </SPEC-HIERARCHY>
              <SPEC-HIERARCHY IDENTIFIER="h-4"><OBJECT><SPEC-OBJECT-REF>o-missing</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>
              <SPEC-HIERARCHY IDENTIFIER="h-5"><OBJECT><SPEC-OBJECT-REF>o-empty</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY>
            </CHILDREN>
          </SPEC-HIERARCHY>
        </CHILDREN>
      </SPECIFICATION>
    </SPECIFICATIONS>
  </REQ-IF-CONTENT></CORE-CONTENT>
</REQ-IF>`

func TestReqIF(t *testing.T) {
	res := mustParse(t, []byte(testReqIF), "spec.reqif", Options{})
	if res.Format != FormatReqIF || len(res.Candidates) != 3 {
		for _, c := range res.Candidates {
			t.Logf("%+v", c)
		}
		t.Fatalf("result: %d candidates, errors %v", len(res.Candidates), res.Errors)
	}
	a := res.Candidates[0]
	if a.ExternalID != "REQ-1" || a.Title != "Lockout" || a.Folder != "Security - Access" || a.Priority != "high" || a.Obligation != "shall" {
		t.Errorf("first: %+v", a)
	}
	if a.Description != "The system SHALL lock accounts.\n- after 5 attempts\n- for 10 min\nLine\nbreak & more" &&
		a.Description != "The system SHALL lock accounts.\n- after 5 attempts\n- for 10 min\nLine\nbreak & more" {
		t.Errorf("xhtml text: %q", a.Description)
	}
	b := res.Candidates[1]
	if b.Folder != "Security - Access" || b.Obligation != "should" || b.Title != "Användaren BÖR få ett meddelande" {
		t.Errorf("second: %+v", b)
	}
	if c := res.Candidates[2]; c.Title != "Loose one MAY exist" || c.Folder != "" {
		t.Errorf("loose: %+v", c)
	}
	if len(res.Errors) != 2 {
		t.Errorf("errors: %v", res.Errors)
	}
	if fieldOf(res, "ReqIF.Text") != FieldDescription || fieldOf(res, "Verification Notes") != "" || fieldOf(res, "ReqIF.Prefix") != "" ||
		fieldOf(res, "ReqIF.ChapterName") != FieldTitle {
		t.Errorf("columns: %+v", res.Columns)
	}
	// Mapping override: notes become the rationale.
	res = mustParse(t, []byte(testReqIF), "spec.reqif", Options{Mapping: map[string]string{"Verification Notes": "rationale"}})
	if res.Candidates[1].Rationale != "checked by QA" {
		t.Errorf("mapping: %+v", res.Candidates[1])
	}
}

func TestReqIFZipAndErrors(t *testing.T) {
	two := strings.Replace(testReqIF, `</SPECIFICATIONS>`, `<SPECIFICATION IDENTIFIER="s-2" LONG-NAME="Other"><CHILDREN>
	<SPEC-HIERARCHY><OBJECT><SPEC-OBJECT-REF>o-loose</SPEC-OBJECT-REF></OBJECT></SPEC-HIERARCHY></CHILDREN></SPECIFICATION></SPECIFICATIONS>`, 1)
	z := zipOf(t, map[string]string{"a/spec.reqif": two, "readme.txt": "x"})
	res := mustParse(t, z, "spec.reqifz", Options{})
	if len(res.Candidates) != 3 {
		t.Fatalf("zip: %+v", res.Candidates)
	}
	if res.Candidates[0].Folder != "System spec/Security - Access" || res.Candidates[2].Folder != "Other" {
		t.Errorf("spec folders: %q %q", res.Candidates[0].Folder, res.Candidates[2].Folder)
	}
	if _, err := Parse(zipOf(t, map[string]string{"x.txt": "y"}), "a.reqifz", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no reqif in zip: %v", err)
	}
	if _, err := Parse([]byte("<REQ-IF><broken"), "a.reqif", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("broken: %v", err)
	}
	if _, err := Parse([]byte(`<?xml version="1.0"?><REQ-IF/>`), "a.reqif", Options{}); err == nil || !strings.Contains(err.Error(), "SPEC-OBJECTS") {
		t.Errorf("empty: %v", err)
	}
	latin := `<?xml version="1.0" encoding="ISO-8859-1"?><REQ-IF/>`
	if _, err := Parse([]byte(latin), "a.reqif", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("encoding: %v", err)
	}
	if got := xhtmlText(`<p>a</p><table><tr><td>x</td><td>y</td></tr></table><h1>T</h1>`); got != "a\nx y\nT" {
		t.Errorf("xhtmlText: %q", got)
	}
}
