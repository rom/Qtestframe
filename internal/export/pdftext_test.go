package export

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// pdfText is one text drawn on a PDF page.
type pdfText struct {
	Y    float64 // from the bottom of the page (points)
	Text string
}

var (
	pdfObjRe   = regexp.MustCompile(`(?s)(\d+) 0 obj\s*(.*?)endobj`)
	pdfKidsRe  = regexp.MustCompile(`/Kids \[([^\]]*)\]`)
	pdfContRe  = regexp.MustCompile(`/Contents (\d+) 0 R`)
	pdfStreamR = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfTextRe  = regexp.MustCompile(`(?s)BT ([\d.]+) ([\d.]+) Td \(((?:\\.|[^\\)])*)\) ?Tj ET`)
)

// pdfPages extracts the texts of every page of a PDF written by fpdf (in
// page order and drawing order): a pure-Go check of what each page holds.
func pdfPages(t *testing.T, b []byte) [][]pdfText {
	t.Helper()
	objs := map[string]string{}
	for _, m := range pdfObjRe.FindAllSubmatch(b, -1) {
		objs[string(m[1])] = string(m[2])
	}
	var kids []string
	for _, body := range objs {
		if strings.Contains(body, "/Type /Pages") {
			if m := pdfKidsRe.FindStringSubmatch(body); m != nil {
				for _, f := range strings.Fields(m[1]) {
					if f != "0" && f != "R" {
						kids = append(kids, f)
					}
				}
			}
		}
	}
	if len(kids) == 0 {
		t.Fatal("no pages")
	}
	var pages [][]pdfText
	for _, k := range kids {
		m := pdfContRe.FindStringSubmatch(objs[k])
		if m == nil {
			t.Fatalf("page %s has no contents", k)
		}
		s := pdfStreamR.FindStringSubmatch(objs[m[1]])
		if s == nil {
			t.Fatalf("contents %s has no stream", m[1])
		}
		data := []byte(s[1])
		if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			if data, err = io.ReadAll(r); err != nil {
				t.Fatal(err)
			}
		}
		var texts []pdfText
		for _, tm := range pdfTextRe.FindAllSubmatch(data, -1) {
			y, _ := strconv.ParseFloat(string(tm[2]), 64)
			txt := strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(string(tm[3]))
			texts = append(texts, pdfText{Y: y, Text: txt})
		}
		pages = append(pages, texts)
	}
	return pages
}

// pageOf returns the index of the first page with a text starting with
// prefix (-1 when there is none).
func pageOf(pages [][]pdfText, prefix string) int {
	for i, p := range pages {
		for _, x := range p {
			if strings.HasPrefix(x.Text, prefix) {
				return i
			}
		}
	}
	return -1
}

func TestPDFPagesHelper(t *testing.T) {
	var b bytes.Buffer
	if err := WriteDocument(&b, PDF, sampleDocument(3)); err != nil {
		t.Fatal(err)
	}
	pages := pdfPages(t, b.Bytes())
	if len(pages) < 3 || pageOf(pages, "Spec") != 0 || pageOf(pages, "1 Introduction") < 1 {
		t.Fatal(len(pages), pages)
	}
}
