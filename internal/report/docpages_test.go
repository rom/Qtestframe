package report

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

var (
	pdfObjRe   = regexp.MustCompile(`(?s)(\d+) 0 obj\s*(.*?)endobj`)
	pdfKidsRe  = regexp.MustCompile(`/Kids \[([^\]]*)\]`)
	pdfContRe  = regexp.MustCompile(`/Contents (\d+) 0 R`)
	pdfStreamR = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfTextRe  = regexp.MustCompile(`(?s)BT [\d.]+ [\d.]+ Td \(((?:\\.|[^\\)])*)\) ?Tj ET`)
)

// pdfPageTexts extracts the texts of every page of a generated PDF, in
// page and drawing order (pure Go: the PDF writer's page objects and
// content streams).
func pdfPageTexts(t *testing.T, b []byte) [][]string {
	t.Helper()
	objs := map[string]string{}
	for _, m := range pdfObjRe.FindAllSubmatch(b, -1) {
		objs[string(m[1])] = string(m[2])
	}
	var pages [][]string
	for _, body := range objs {
		m := pdfKidsRe.FindStringSubmatch(body)
		if m == nil || !strings.Contains(body, "/Type /Pages") {
			continue
		}
		for _, k := range strings.Fields(m[1]) {
			if k == "0" || k == "R" {
				continue
			}
			c := pdfContRe.FindStringSubmatch(objs[k])
			if c == nil {
				t.Fatalf("page %s has no contents", k)
			}
			s := pdfStreamR.FindStringSubmatch(objs[c[1]])
			if s == nil {
				t.Fatalf("no stream in %s", c[1])
			}
			data := []byte(s[1])
			if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
				data, _ = io.ReadAll(r)
			}
			var texts []string
			for _, tm := range pdfTextRe.FindAllSubmatch(data, -1) {
				texts = append(texts, strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(string(tm[1])))
			}
			pages = append(pages, texts)
		}
	}
	return pages
}

func render(t *testing.T, kind string, d *DocData, o DocOptions, f export.Format) []byte {
	t.Helper()
	doc, err := BuildDocument(kind, d, o)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := export.WriteDocument(&b, f, doc); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var numberedHeading = regexp.MustCompile(`^\d+(\.\d+)* \S`)

// reqHeadings returns, per page, the requirement headings on it.
func reqHeadings(pages [][]string, d *DocData) map[int][]string {
	titles := map[string]bool{}
	for _, r := range d.Requirements {
		titles[r.Key+" "+r.Title] = true
	}
	out := map[int][]string{}
	for i, p := range pages {
		for _, x := range p {
			if titles[x] {
				out[i] = append(out[i], x)
			}
		}
	}
	return out
}

// TestSpecEveryRequirementOnNewPage: in the PDF every requirement starts a
// page; only its folder headings (chapter, folder) come before it there,
// and no page is blank. Turned off, requirements share pages.
func TestSpecEveryRequirementOnNewPage(t *testing.T) {
	d := largeDocData(45)
	pages := pdfPageTexts(t, render(t, DocRequirementsSpec, d, DocOptions{}, export.PDF))
	header := pages[1][0] // the running header of the pages after the title page
	byPage := reqHeadings(pages, d)
	seen := 0
	for p, hs := range byPage {
		if len(hs) != 1 {
			t.Errorf("page %d has %d requirements: %v", p+1, len(hs), hs)
		}
		for _, x := range pages[p][1:] {
			if x == hs[0] {
				break
			}
			if x == header || !numberedHeading.MatchString(x) {
				t.Errorf("page %d: %q comes before %s", p+1, x, hs[0])
			}
		}
		seen += len(hs)
	}
	if seen != len(d.Requirements) {
		t.Fatalf("%d of %d requirement headings found", seen, len(d.Requirements))
	}
	// folder headings stay with their first requirement: "2.1 Area 0" is on
	// the page of the requirement after it
	for p, texts := range pages {
		if p > 0 && len(texts) < 3 {
			t.Errorf("page %d is blank: %v", p+1, texts)
		}
		for i, x := range texts {
			if strings.HasPrefix(x, "2.") && numberedHeading.MatchString(x) && i == len(texts)-2 { // the last text is the footer
				t.Errorf("page %d ends with the heading %q", p+1, x)
			}
		}
	}

	off := false
	compact := pdfPageTexts(t, render(t, DocRequirementsSpec, d, DocOptions{NewPagePerRequirement: &off}, export.PDF))
	shared := 0
	for _, hs := range reqHeadings(compact, d) {
		if len(hs) > 1 {
			shared++
		}
	}
	if shared == 0 || len(compact) >= len(pages) {
		t.Fatalf("compact: %d pages (%d with several requirements), one per page: %d", len(compact), shared, len(pages))
	}
}

// docxParagraphs splits word/document.xml into its top-level paragraphs
// and tables.
func docxParagraphs(t *testing.T, b []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			x, _ := io.ReadAll(rc)
			rc.Close()
			return regexp.MustCompile(`(?s)<w:tbl>.*?</w:tbl>|<w:p>.*?</w:p>|<w:p/>`).FindAllString(string(x), -1)
		}
	}
	t.Fatal("no document.xml")
	return nil
}

// TestSpecEveryRequirementOnNewPageDOCX: every requirement heading starts a
// page (pageBreakBefore), or the folder headings right before it do.
func TestSpecEveryRequirementOnNewPageDOCX(t *testing.T) {
	d := largeDocData(45)
	ps := docxParagraphs(t, render(t, DocRequirementsSpec, d, DocOptions{}, export.DOCX))
	isHeading := func(p string) bool { return strings.Contains(p, `<w:pStyle w:val="Heading`) }
	n := 0
	for i, p := range ps {
		if !strings.Contains(p, `<w:pStyle w:val="ItemHeading"/>`) || !strings.Contains(p, "BIG-REQ-") {
			continue
		}
		n++
		ok := false
		for j := i; j >= 0; j-- {
			q := ps[j]
			if strings.Contains(q, "<w:pageBreakBefore/>") || strings.Contains(q, `<w:br w:type="page"/>`) {
				ok = true
				break
			}
			if j < i && !isHeading(q) && !(j > 0 && isHeading(ps[j-1])) { // a heading, or the one paragraph after it
				break
			}
		}
		if !ok {
			t.Errorf("requirement heading %d does not start a page: %s", n, p)
		}
	}
	if n != len(d.Requirements) {
		t.Fatalf("%d requirement headings", n)
	}
	off := false
	compact := render(t, DocRequirementsSpec, d, DocOptions{NewPagePerRequirement: &off}, export.DOCX)
	if bytes.Contains(compact, []byte("pageBreakBefore")) {
		t.Fatal("compact printing has page breaks before requirements")
	}
	if md := string(render(t, DocRequirementsSpec, d, DocOptions{}, export.MD)); strings.Contains(md, "page-break") {
		t.Fatal("Markdown has no page breaks")
	}
}

// TestTestPlanKeepsTestsTogether: a test that fits on a page is not split
// across two.
func TestTestPlanKeepsTestsTogether(t *testing.T) {
	d := largeDocData(40)
	for i := range d.Tests {
		var steps []model.TestStep
		for k := range 2 + i%7 {
			steps = append(steps, model.TestStep{Action: fmt.Sprintf("T%d action %d", i, k), Expected: "ok"})
		}
		d.Tests[i].Steps = steps
	}
	pages := pdfPageTexts(t, render(t, DocTestPlan, d, DocOptions{}, export.PDF))
	pageOf := func(s string) int {
		for i, p := range pages {
			for _, x := range p {
				if x == s {
					return i
				}
			}
		}
		return -1
	}
	for i, tc := range d.Tests {
		h, last := pageOf(tc.Key+" "+tc.Title), pageOf(fmt.Sprintf("T%d action %d", i, len(tc.Steps)-1))
		if h < 0 || h != last {
			t.Errorf("%s: heading on page %d, last step on page %d", tc.Key, h+1, last+1)
		}
	}
}

func TestNewPagePerRequirementOption(t *testing.T) {
	o := DocOptions{}
	if err := o.Normalize(DocRequirementsSpec); err != nil || o.NewPagePerRequirement == nil || !*o.NewPagePerRequirement {
		t.Fatal(o.NewPagePerRequirement, err)
	}
	on := true
	p := DocOptions{NewPagePerRequirement: &on}
	if err := p.Normalize(DocTestPlan); err != nil || p.NewPagePerRequirement != nil {
		t.Fatal(p.NewPagePerRequirement, err)
	}
	// the headings: the chapter's first folder stays on the chapter's page,
	// the others start one, and so do requirements after another one
	doc, _ := BuildDocument(DocRequirementsSpec, sampleDocData(), DocOptions{})
	var starts []string
	for _, b := range doc.Blocks {
		if b.NewPage {
			starts = append(starts, b.Text)
		}
		if b.Kind == export.BlockHeading && b.NoTOC != b.KeepTogether {
			t.Errorf("%q: item headings stay together", b.Text)
		}
	}
	if got := strings.Join(starts, " | "); got != "2.2 Checkout | 2.2.1 Payment" {
		t.Fatal(got)
	}
}
