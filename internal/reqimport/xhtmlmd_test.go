package reqimport

import (
	"strings"
	"testing"
)

func TestXHTMLMarkdown(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<xhtml:div><xhtml:p>a<xhtml:br/>b</xhtml:p><xhtml:p>c</xhtml:p></xhtml:div>", "a\nb\n\nc"},
		{"<div>Text <b>bold</b> <i>it</i> <tt>x</tt>\n  more</div>", "Text **bold** *it* `x` more"},
		{"<ol><li>a</li><li>b<ul><li>c</li></ul></li></ol><p>after</p>", "1. a\n2. b\n- c\n\nafter"},
		{"<ul><li><p>para in item</p></li></ul>", "- para in item"},
		{"<h3>Head</h3><pre>  x &lt; y\n</pre>", "### Head\n\n```\n  x < y\n```"},
		{"<table><tr><td>a</td><td>b</td></tr><tr><td>c</td></tr></table>", "a b\nc"},
		{`<p><a href="https://x">link</a> <object data="f/i.png" type="image/png">img</object></p>`, "[link](https://x) ![img](f/i.png)"},
		{"<p>  </p><p>x<span> </span>y</p></ul>", "x y"},
	}
	for _, c := range cases {
		if got := xhtmlMarkdown(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestReqIFFromQtestframe(t *testing.T) {
	doc := strings.Replace(testReqIF, `<TITLE>Spec</TITLE>`, `<REQ-IF-TOOL-ID>Qtestframe</REQ-IF-TOOL-ID><TITLE>Spec</TITLE>`, 1)
	res := mustParse(t, []byte(doc), "spec.reqif", Options{})
	if d := res.Candidates[0].Description; d != "The system **SHALL** lock accounts.\n\n- after 5 attempts\n- for 10 min\n\nLine\nbreak & more" {
		t.Errorf("markdown: %q", d)
	}
	if !(rqHeader{SourceToolID: " Qtestframe 1.0"}).fromQtestframe() || (rqHeader{ToolID: "DOORS"}).fromQtestframe() {
		t.Error("tool detection")
	}
}

func TestCriteriaThreshold(t *testing.T) {
	cs := parseCriteria("test: Responds in time (threshold: 2 s)\n(threshold: only)\nanalys: Load (tröskelvärde: 100 users)")
	if len(cs) != 3 || cs[0].Method != "test" || cs[0].Description != "Responds in time" || cs[0].Threshold != "2 s" ||
		cs[1].Threshold != "" || cs[2].Method != "analysis" || cs[2].Threshold != "100 users" {
		t.Errorf("%+v", cs)
	}
}
