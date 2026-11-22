package export

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Document is a format-neutral text document such as a requirements
// specification or a test plan: a title page, then a sequence of blocks
// (headings, paragraphs, tables, key/value tables, lists, figures, page
// breaks and the place of the table of contents). WriteDocument renders it as PDF,
// DOCX or Markdown.
//
// Headings carry their final text (numbering is up to the caller). The
// writers stream: DOCX and Markdown are written block by block; the PDF
// writer keeps the pages in memory (as the PDF library does) and fills in
// the table of contents once the page numbers are known.
type Document struct {
	Title    string
	Subtitle string
	// TitleFields are shown as a table on the title page (project,
	// version, date, author, …).
	TitleFields []Item
	// Header is the running header of the PDF pages after the title page
	// (e.g. "Title · Version 2.0").
	Header      string
	Author      string
	Lang        string
	GeneratedAt time.Time
	// TOCLevels is the deepest heading level listed in the table of
	// contents (default 3).
	TOCLevels int
	Blocks    []Block
	// Link maps a table cell value (an item key) to a URL; the Markdown
	// writer turns such cells into links.
	Link func(value string) string `json:"-"`
}

// BlockKind is the kind of a document block.
type BlockKind int

// Block kinds.
const (
	// BlockHeading: Text at Level 1–6. NoTOC keeps it out of the table of
	// contents (e.g. one heading per requirement). NewPage starts it on a
	// new page (PDF, DOCX; not when the page is still empty). KeepTogether
	// marks the heading of an item: it and the blocks up to the next
	// heading stay on one page when they fit on one, together with the
	// headings (and their one paragraph each) right before it.
	BlockHeading BlockKind = iota
	// BlockParagraph: Text, or Spans for mixed emphasis. Blank lines
	// separate paragraphs, single line breaks are kept.
	BlockParagraph
	// BlockTable: Columns and Rows, optionally relative column Widths.
	BlockTable
	// BlockFields: a two-column key/value table of Fields (items without
	// a value are left out).
	BlockFields
	// BlockList: a bullet list of Items.
	BlockList
	// BlockPageBreak starts a new page (PDF, DOCX).
	BlockPageBreak
	// BlockTOC is where the table of contents goes.
	BlockTOC
	// BlockFigure: a vector drawing (Figure), as wide as the page at most.
	BlockFigure
)

// Span is a run of paragraph text with optional emphasis.
type Span struct {
	Text   string
	Bold   bool
	Italic bool
}

// Block is one element of a document.
type Block struct {
	Kind         BlockKind
	Level        int
	Text         string
	Spans        []Span
	NoTOC        bool
	NewPage      bool // heading: starts a new page
	KeepTogether bool // heading: its item stays on one page (see BlockHeading)
	Italic       bool // the whole paragraph is emphasised (notes, placeholders)
	Columns      []string
	Rows         [][]string
	Widths       []float64
	Fields       []Item
	Items        []string
	Figure       *Figure
}

// Heading returns a heading block.
func Heading(level int, text string) Block {
	return Block{Kind: BlockHeading, Level: level, Text: text}
}

// Paragraph returns a plain paragraph block.
func Paragraph(text string) Block { return Block{Kind: BlockParagraph, Text: text} }

// Note returns an emphasised paragraph block.
func Note(text string) Block { return Block{Kind: BlockParagraph, Text: text, Italic: true} }

// RichParagraph returns a paragraph of spans.
func RichParagraph(spans ...Span) Block { return Block{Kind: BlockParagraph, Spans: spans} }

// Table returns a table block.
func Table(columns []string, rows [][]string, widths ...float64) Block {
	return Block{Kind: BlockTable, Columns: columns, Rows: rows, Widths: widths}
}

// Fields returns a key/value block.
func Fields(items ...Item) Block { return Block{Kind: BlockFields, Fields: items} }

// List returns a bullet list block.
func List(items ...string) Block { return Block{Kind: BlockList, Items: items} }

// PageBreak returns a page break.
func PageBreak() Block { return Block{Kind: BlockPageBreak} }

// TOC returns the table of contents placeholder.
func TOC() Block { return Block{Kind: BlockTOC} }

// DocumentFormats are the formats WriteDocument supports.
var DocumentFormats = []Format{PDF, DOCX, MD}

// WriteDocument renders doc in format f (PDF, DOCX or Markdown).
func WriteDocument(w io.Writer, f Format, doc *Document) error {
	switch f {
	case PDF:
		return writeDocPDF(w, doc)
	case DOCX:
		return writeDocDOCX(w, doc)
	case MD:
		return writeDocMarkdown(w, doc)
	}
	return fmt.Errorf("export: documents cannot be written as %q (want pdf, docx or md)", string(f))
}

// ---------------------------------------------------------------- helpers

func (doc *Document) dataset() Dataset {
	return Dataset{Title: doc.Title, Subtitle: doc.Subtitle, Lang: doc.Lang, GeneratedAt: doc.GeneratedAt}
}

func (doc *Document) tocLevels() int {
	if doc.TOCLevels <= 0 {
		return 3
	}
	return min(doc.TOCLevels, 6)
}

// tocEntry is a heading listed in the table of contents.
type tocEntry struct {
	block int // index in Blocks
	level int
	text  string
}

// toc lists the headings after the TOC block that belong in the table of
// contents (none when the document has no TOC block).
func (doc *Document) toc() (entries []tocEntry, has bool) {
	max := doc.tocLevels()
	for i, b := range doc.Blocks {
		switch {
		case b.Kind == BlockTOC:
			has = true
		case has && b.Kind == BlockHeading && !b.NoTOC && clampLevel(b.Level) <= max:
			entries = append(entries, tocEntry{block: i, level: clampLevel(b.Level), text: oneLine(b.Text)})
		}
	}
	return entries, has
}

func clampLevel(l int) int { return min(max(l, 1), 6) }

// keepUnit returns the end (exclusive) of the blocks that start at block i
// and stay on one page: a run of headings (each with at most one paragraph
// after it, such as a folder description) that ends with a KeepTogether
// heading, plus the item's blocks up to the next heading. It returns i when
// block i starts no such unit. A NewPage heading inside the run ends it (the
// unit starts there).
func (doc *Document) keepUnit(i int) int {
	bs := doc.Blocks
	for j := i; j < len(bs); j++ {
		b := bs[j]
		switch {
		case b.Kind == BlockHeading && j > i && b.NewPage:
			return i
		case b.Kind == BlockHeading && b.KeepTogether:
			end := j + 1
			for end < len(bs) && bs[end].Kind != BlockHeading && bs[end].Kind != BlockPageBreak && bs[end].Kind != BlockTOC {
				end++
			}
			return end
		case b.Kind == BlockHeading:
		case b.Kind == BlockParagraph && j > i && bs[j-1].Kind == BlockHeading && !b.caption():
		default:
			return i
		}
	}
	return i
}

// spans returns the spans of a paragraph block.
func (b Block) spans() []Span {
	if len(b.Spans) > 0 {
		return b.Spans
	}
	return []Span{{Text: b.Text, Italic: b.Italic}}
}

// caption reports whether a paragraph is a single bold run (a caption of
// what follows).
func (b Block) caption() bool {
	return b.Kind == BlockParagraph && len(b.Spans) == 1 && b.Spans[0].Bold && !strings.Contains(b.Spans[0].Text, "\n")
}

// plain returns the text of a paragraph block without emphasis.
func (b Block) plain() string {
	var s strings.Builder
	for _, sp := range b.spans() {
		s.WriteString(sp.Text)
	}
	return s.String()
}

// fieldItems are the fields with a value.
func (b Block) fieldItems() []Item {
	out := make([]Item, 0, len(b.Fields))
	for _, f := range b.Fields {
		if strings.TrimSpace(f.Value) != "" {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------- Markdown

// gfmSlug is the anchor GitHub gives a heading with this text; seen counts
// the slugs used so far (duplicates get -1, -2, …).
func gfmSlug(text string, seen map[string]int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	s := b.String()
	n := seen[s]
	seen[s] = n + 1
	if n > 0 {
		s += "-" + strconv.Itoa(n)
	}
	return s
}

// markdownSpans renders paragraph spans as Markdown paragraphs.
func markdownSpans(spans []Span) string {
	// render line by line: a span may contain line breaks
	lines := []string{""}
	for _, sp := range spans {
		for i, part := range splitLines(sp.Text) {
			if i > 0 {
				lines = append(lines, "")
			}
			t := escapeInline(part)
			if strings.TrimSpace(t) != "" {
				lead := t[:len(t)-len(strings.TrimLeft(t, " "))]
				trail := t[len(strings.TrimRight(t, " ")):]
				core := strings.TrimSpace(t)
				switch {
				case sp.Bold && sp.Italic:
					core = "***" + core + "***"
				case sp.Bold:
					core = "**" + core + "**"
				case sp.Italic:
					core = "*" + core + "*"
				}
				t = lead + core + trail
			}
			lines[len(lines)-1] += t
		}
	}
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, strings.Join(cur, "\\\n"))
			cur = nil
		}
	}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			flush()
			continue
		}
		cur = append(cur, blockStart(l))
	}
	flush()
	return strings.Join(paras, "\n\n")
}

func writeDocMarkdown(w io.Writer, doc *Document) error {
	ds := doc.dataset()
	var m MarkdownWriter
	m.Heading(1, doc.Title)
	m.Paragraph(doc.Subtitle)
	fields := append([]Item{}, doc.TitleFields...)
	fields = append(fields, Item{Label: ds.label("generated"), Value: generatedAt(ds).Format("2006-01-02 15:04:05 MST")})
	m.List(fields...)
	entries, _ := doc.toc()
	// anchors of every heading in document order (GitHub numbers duplicates)
	seen := map[string]int{}
	gfmSlug(oneLine(doc.Title), seen)
	anchors := map[int]string{}
	for i, b := range doc.Blocks {
		if b.Kind == BlockHeading {
			anchors[i] = gfmSlug(oneLine(b.Text), seen)
		}
	}
	for i, b := range doc.Blocks {
		switch b.Kind {
		case BlockHeading:
			m.Heading(min(clampLevel(b.Level)+1, 6), b.Text)
		case BlockParagraph:
			if b.Italic && len(b.Spans) == 0 {
				m.Emphasis(b.Text)
			} else if s := markdownSpans(b.spans()); s != "" {
				m.block(s)
			}
		case BlockTable:
			if len(b.Rows) == 0 {
				m.Emphasis(ds.label("noRows"))
				continue
			}
			m.Table(b.Columns, b.Rows, doc.Link)
		case BlockFields:
			m.List(b.fieldItems()...)
		case BlockList:
			items := make([]Item, len(b.Items))
			for j, it := range b.Items {
				items[j] = Item{Value: it}
			}
			m.List(items...)
		case BlockFigure:
			if b.Figure != nil {
				m.block(markdownFigure(b.Figure))
			}
		case BlockTOC:
			if len(entries) == 0 {
				continue
			}
			m.Heading(2, ds.label("contents"))
			var lb strings.Builder
			for _, e := range entries {
				lb.WriteString(strings.Repeat("  ", e.level-1) + "- " + Link(e.text, "#"+anchors[e.block]) + "\n")
			}
			m.block(strings.TrimSuffix(lb.String(), "\n"))
		}
		if i%64 == 63 {
			if err := m.Flush(w); err != nil {
				return err
			}
		}
	}
	if m.b.Len() == 0 && m.flushed {
		return nil
	}
	_, err := m.WriteTo(w)
	return err
}
