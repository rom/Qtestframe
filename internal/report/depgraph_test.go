package report

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// graphDocData is a project with folders and relations of every type
// (with a cycle and a relation to a requirement outside the document).
func graphDocData() *DocData {
	d := &DocData{Project: model.Project{Key: "G", Name: "Graph"}, Now: t0,
		Folders: []model.Folder{{ID: "fa", Kind: "requirement", Name: "Accounts"}, {ID: "fb", Kind: "requirement", Name: "Billing"},
			{ID: "fa1", Kind: "requirement", Name: "Login", ParentID: ptrS("fa")}}}
	statuses := model.RequirementStatuses
	for i := 1; i <= 9; i++ {
		f := "fa1"
		if i > 5 {
			f = "fb"
		}
		d.Requirements = append(d.Requirements, model.Requirement{ID: fmt.Sprintf("g%d", i), Key: fmt.Sprintf("G-REQ-%d", i),
			Title: fmt.Sprintf("Requirement number %d with a rather long title that does not fit in its box", i), FolderID: ptrS(f), Status: statuses[i%5]})
	}
	rel := func(from, to int, typ string) {
		rq := &d.Requirements[from-1]
		key, title, id := fmt.Sprintf("G-REQ-%d", to), "", fmt.Sprintf("g%d", to)
		if to <= len(d.Requirements) {
			title = d.Requirements[to-1].Title
		}
		rq.RelationRefs = append(rq.RelationRefs, model.ItemRef{Type: typ, Direction: "outgoing", Kind: "requirement", ID: id, Key: key, Title: title})
		if to <= len(d.Requirements) {
			o := &d.Requirements[to-1]
			o.RelationRefs = append(o.RelationRefs, model.ItemRef{Type: typ, Direction: "incoming", Kind: "requirement", ID: rq.ID, Key: rq.Key})
		}
	}
	rel(2, 1, "refines")
	rel(3, 1, "refines")
	rel(4, 2, "depends_on")
	rel(5, 4, "derived_from")
	rel(1, 6, "provides")
	rel(6, 7, "conflicts_with")
	rel(7, 8, "duplicates")
	rel(8, 6, "relates_to")
	rel(5, 3, "depends_on")
	rel(3, 5, "depends_on") // a cycle
	rel(9, 42, "refines")   // outside the document
	return d
}

func specMarkdown(t *testing.T, d *DocData, o DocOptions) string {
	t.Helper()
	doc, err := BuildDocument(DocRequirementsSpec, d, o)
	if err != nil {
		t.Fatal(err)
	}
	return docText(t, doc)
}

var svgURIRe = regexp.MustCompile(`\(data:image/svg\+xml;base64,([A-Za-z0-9+/=]+)\)`)

// figuresOf returns the SVG figures of a Markdown document.
func figuresOf(t *testing.T, md string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, m := range svgURIRe.FindAllStringSubmatch(md, -1) {
		b, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// parseSVG checks that b is well-formed SVG and returns its texts and the
// number of elements per name.
func parseSVG(t *testing.T, b []byte) (texts []string, count map[string]int) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	count = map[string]int{}
	var stack []string
	root := ""
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v\n%s", err, b)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if root == "" {
				root = v.Name.Local
				if v.Name.Space != "http://www.w3.org/2000/svg" {
					t.Errorf("root namespace %q", v.Name.Space)
				}
			}
			stack = append(stack, v.Name.Local)
			count[v.Name.Local]++
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "text" && strings.TrimSpace(string(v)) != "" {
				texts = append(texts, string(v))
			}
		}
	}
	if root != "svg" {
		t.Errorf("root element %q", root)
	}
	return texts, count
}

func TestDependencyGraphAppendix(t *testing.T) {
	md := specMarkdown(t, graphDocData(), DocOptions{})
	for _, want := range []string{
		"## Appendix B. Dependency graph", "**Figure B.1. Requirement relations**", "Table B.1 lists every relation as text.",
		"**Table B.1. Relations between the requirements**",
		"| G-REQ-2 Requirement number 2 with a rather long title that does not fit in its box | refines | G-REQ-1 Requirement number 1",
		"| G-REQ-9 Requirement number 9 with a rather long title that does not fit in its box | refines | G-REQ-42 (not in this specification) | |",
		"| depends on |", "| derived from |", "| provides |", "| conflicts with |", "| duplicates |", "| relates to |",
		"[Appendix B. Dependency graph](#appendix-b-dependency-graph)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	figs := figuresOf(t, md)
	if len(figs) != 1 {
		t.Fatalf("%d figures, want 1", len(figs))
	}
	texts, count := parseSVG(t, figs[0])
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"G-REQ-1", "G-REQ-8", "Relations:", "refines", "depends on", "Status:", "Approved", "Requirement number 1 with a"} {
		if !strings.Contains(joined, want) {
			t.Errorf("figure lacks %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "G-REQ-9") || strings.Contains(joined, "G-REQ-42") {
		t.Error("requirements without relations inside the document are not drawn")
	}
	if !strings.Contains(joined, "…") {
		t.Error("long titles are shortened")
	}
	// 8 boxes with a status stripe; an edge per relation, arrows for the directed ones
	if count["path"] < 10 || count["polygon"] < 7 {
		t.Errorf("elements %v", count)
	}
	if !bytes.Contains(figs[0], []byte(`role="img"`)) || !bytes.Contains(figs[0], []byte("<desc id=\"fig-desc\">Diagram of the relations between 8 requirements: G-REQ-1, G-REQ-2")) {
		t.Errorf("figure has no text alternative:\n%s", figs[0])
	}

	// the option, and test plans never
	f := false
	if md := specMarkdown(t, graphDocData(), DocOptions{DependencyGraph: &f}); strings.Contains(md, "Dependency graph") {
		t.Error("dependency_graph false leaves the appendix out")
	}
	doc, err := BuildDocument(DocTestPlan, graphDocData(), DocOptions{DependencyGraph: ptrB(true)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(docText(t, doc), "Dependency graph") {
		t.Error("a test plan has no dependency graph")
	}
	// without relations: no appendix
	d := graphDocData()
	for i := range d.Requirements {
		d.Requirements[i].RelationRefs = nil
	}
	if md := specMarkdown(t, d, DocOptions{}); strings.Contains(md, "Dependency graph") {
		t.Error("no appendix without relations")
	}
	// only relations to requirements outside: the table, no figure
	d.Requirements[0].RelationRefs = []model.ItemRef{{Type: "refines", Direction: "outgoing", Kind: "requirement", ID: "x", Key: "G-REQ-99"}}
	md = specMarkdown(t, d, DocOptions{})
	if !strings.Contains(md, "No two requirements of this specification are related") || len(figuresOf(t, md)) != 0 || !strings.Contains(md, "G-REQ-99 (not in this specification)") {
		t.Errorf("outside relations only:\n%s", md)
	}
}

func ptrB(b bool) *bool { return &b }

// TestDependencyGraphDeterministic: the same figure whatever the order of
// the requirements and relations, and from one run to the next.
func TestDependencyGraphDeterministic(t *testing.T) {
	first := figuresOf(t, specMarkdown(t, graphDocData(), DocOptions{}))
	for run := 0; run < 5; run++ {
		d := graphDocData()
		// reverse the requirements and their relations
		for i, j := 0, len(d.Requirements)-1; i < j; i, j = i+1, j-1 {
			d.Requirements[i], d.Requirements[j] = d.Requirements[j], d.Requirements[i]
		}
		for i := range d.Requirements {
			rs := d.Requirements[i].RelationRefs
			for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
				rs[a], rs[b] = rs[b], rs[a]
			}
		}
		got := figuresOf(t, specMarkdown(t, d, DocOptions{}))
		if len(got) != len(first) || !bytes.Equal(got[0], first[0]) {
			t.Fatalf("run %d: the figure differs", run)
		}
	}
}

func TestGraphLayout(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	edges := []graphEdge{{"b", "a", "refines"}, {"c", "b", "depends_on"}, {"a", "d", "provides"}, {"d", "e", "relates_to"}, {"a", "c", "depends_on"}}
	o := layoutOpts{nodeW: 100, nodeH: 30, gapX: 50, gapY: 10}
	l := layered(ids, edges, o)
	layer := map[string]int{}
	for _, n := range l.Nodes {
		layer[n.ID] = n.Layer
	}
	// a is upstream of b (b refines a) and of d (a provides d); c → b closes a cycle with a → c
	if layer["a"] != 0 || layer["b"] != 1 || layer["d"] != 1 {
		t.Errorf("layers %v", layer)
	}
	if layer["e"] != 0 {
		t.Errorf("a symmetric relation does not order: %v", layer)
	}
	cycle := 0
	for _, e := range edges {
		if _, _, ok := flowOf(e); ok && !l.Flow[e] {
			cycle++
		}
	}
	if cycle != 1 {
		t.Errorf("%d edges ignored for the layers, want 1 (the cycle)", cycle)
	}
	again := layered(ids, edges, o)
	if fmt.Sprint(again.Nodes) != fmt.Sprint(l.Nodes) || again.Width != l.Width || again.Height != l.Height {
		t.Error("the layout is not deterministic")
	}
	v := layered(ids, edges, layoutOpts{nodeW: 100, nodeH: 30, gapX: 10, gapY: 40, vertical: true})
	for _, n := range v.Nodes {
		if n.Y != float64(n.Layer)*70 {
			t.Errorf("vertical: %+v", n)
		}
	}
	if e := layered(nil, nil, o); e.Width != 0 || len(e.Nodes) != 0 {
		t.Errorf("empty layout %+v", e)
	}
}

// TestDependencyGraphFormats: the appendix is in every format, as a vector
// figure (SVG in Markdown and DOCX with a PNG fallback, drawing operators
// in PDF).
func TestDependencyGraphFormats(t *testing.T) {
	doc, err := BuildDocument(DocRequirementsSpec, graphDocData(), DocOptions{Lang: "sv"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[export.Format][]byte{}
	for _, f := range export.DocumentFormats {
		var b bytes.Buffer
		if err := export.WriteDocument(&b, f, doc); err != nil {
			t.Fatal(err)
		}
		out[f] = b.Bytes()
	}
	if md := string(out[export.MD]); !strings.Contains(md, "Bilaga B. Beroendegraf") || !strings.Contains(md, "Figur B.1. Kravrelationer") || !strings.Contains(md, "| beror på |") {
		t.Errorf("Markdown:\n%s", md)
	}
	zr, err := zip.NewReader(bytes.NewReader(out[export.DOCX]), int64(len(out[export.DOCX])))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		parts[f.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	body := string(parts["word/document.xml"])
	if !strings.Contains(body, "Figur B.1. Kravrelationer") || !strings.Contains(body, `<asvg:svgBlip r:embed="rIdS1"/>`) || !strings.Contains(body, `descr="Diagram över relationerna`) {
		t.Error("DOCX: the figure is not in the document")
	}
	if err := xml.Unmarshal(parts["word/document.xml"], new(any)); err != nil {
		t.Errorf("DOCX document.xml: %v", err)
	}
	if !strings.Contains(string(parts["word/_rels/document.xml.rels"]), `Target="media/figure1.svg"`) || !strings.Contains(string(parts["[Content_Types].xml"]), `Extension="svg"`) {
		t.Error("DOCX: the figure's relationships")
	}
	if !bytes.HasPrefix(parts["word/media/figure1.png"], []byte("\x89PNG")) {
		t.Error("DOCX: no PNG fallback")
	}
	if texts, _ := parseSVG(t, parts["word/media/figure1.svg"]); !strings.Contains(strings.Join(texts, " "), "beror på") {
		t.Errorf("DOCX SVG texts %v", texts)
	}
	pdf := pdfContent(t, out[export.PDF])
	if !strings.Contains(pdf, "(Figur B.1. Kravrelationer)") || !strings.Contains(pdf, "(G-REQ-1)") || !regexp.MustCompile(`[\d.]+ [\d.]+ [\d.]+ [\d.]+ [\d.]+ [\d.]+ c`).MatchString(pdf) {
		t.Error("PDF: no vector figure")
	}

	if dir := os.Getenv("QTF_DEPGRAPH_OUT"); dir != "" { // for a look at the result
		for f, b := range out {
			_ = os.WriteFile(filepath.Join(dir, "depgraph."+string(f)), b, 0o644)
		}
		_ = os.WriteFile(filepath.Join(dir, "depgraph.png"), parts["word/media/figure1.png"], 0o644)
	}
}

// pdfContent is the uncompressed content of a PDF's streams.
func pdfContent(t *testing.T, b []byte) string {
	t.Helper()
	var out strings.Builder
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`).FindAllSubmatch(b, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			out.Write(m[1])
			continue
		}
		data, _ := io.ReadAll(r)
		out.Write(data)
	}
	return out.String()
}

// TestDependencyGraphLarge: a large specification gets several figures of
// at most graphFigureCap requirements each, split by top-level folder,
// with references between them, and every relation in the table.
func TestDependencyGraphLarge(t *testing.T) {
	d := largeDocData(300)
	rels := 0
	for i := range d.Requirements {
		// a chain through the whole project (one connected group), and some
		// relations within each folder
		if i > 0 {
			rq := &d.Requirements[i]
			o := d.Requirements[i-1]
			rq.RelationRefs = append(rq.RelationRefs, model.ItemRef{Type: "depends_on", Direction: "outgoing", Kind: "requirement", ID: o.ID, Key: o.Key})
			rels++
		}
		if i >= 20 {
			o := d.Requirements[i-20]
			d.Requirements[i].RelationRefs = append(d.Requirements[i].RelationRefs, model.ItemRef{Type: "refines", Direction: "outgoing", Kind: "requirement", ID: o.ID, Key: o.Key})
			rels++
		}
	}
	md := specMarkdown(t, d, DocOptions{})
	figs := figuresOf(t, md)
	if len(figs) < 300/graphFigureCap {
		t.Fatalf("%d figures for 300 requirements", len(figs))
	}
	seen := map[string]int{}
	for i, f := range figs {
		texts, _ := parseSVG(t, f)
		own := 0
		for _, tx := range texts {
			if strings.HasPrefix(tx, "BIG-REQ-") {
				seen[tx]++
				own++
			}
		}
		refs := strings.Count(strings.Join(texts, "\n"), "see Figure B.")
		if own-refs > graphFigureCap {
			t.Errorf("figure %d has %d requirements", i+1, own-refs)
		}
	}
	for i := 1; i <= 300; i++ {
		if seen[fmt.Sprintf("BIG-REQ-%d", i)] == 0 {
			t.Errorf("BIG-REQ-%d is in no figure", i)
		}
	}
	if !strings.Contains(md, "**Figure B.1. Requirement relations in Area 0 (part 1 of 1)**") && !strings.Contains(md, "Requirement relations in Area 0") {
		t.Errorf("split by top-level folder:\n%s", md[:2000])
	}
	if !strings.Contains(md, "see Figure B.") && !strings.Contains(string(figs[0]), "see Figure B.") {
		t.Error("references to other figures")
	}
	if n := strings.Count(md, "| depends on |") + strings.Count(md, "| refines |"); n != rels {
		t.Errorf("%d relations in the table, want %d", n, rels)
	}
	if !strings.Contains(md, "The graph is divided into") {
		t.Error("the division is explained")
	}
	// every format, in reasonable time
	doc, err := BuildDocument(DocRequirementsSpec, d, DocOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range export.DocumentFormats {
		start := time.Now()
		var b bytes.Buffer
		if err := export.WriteDocument(&b, f, doc); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %v, %d KB", f, time.Since(start), b.Len()>>10)
		if dir := os.Getenv("QTF_DEPGRAPH_OUT"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "depgraph-large."+string(f)), b.Bytes(), 0o644)
		}
	}
}
