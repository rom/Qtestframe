package export

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MarkdownWriter builds a GitHub-flavoured Markdown document. All text
// passed to it is plain text: it is escaped so that it shows literally
// (Markdown and HTML characters, table pipes, line breaks). Blocks are
// separated by blank lines.
type MarkdownWriter struct {
	b strings.Builder
	// flushed: blocks were already written out by Flush
	flushed bool
}

// String returns the document written so far (ending in one newline).
func (m *MarkdownWriter) String() string {
	return strings.TrimRight(m.b.String(), "\n") + "\n"
}

// WriteTo writes the document to w.
func (m *MarkdownWriter) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, m.String())
	return int64(n), err
}

// Flush writes the blocks written so far to w and empties the buffer, so
// that long documents can be streamed; the separation of the following
// blocks is kept.
func (m *MarkdownWriter) Flush(w io.Writer) error {
	if m.b.Len() == 0 {
		return nil
	}
	_, err := io.WriteString(w, m.b.String())
	m.b.Reset()
	m.flushed = true
	return err
}

func (m *MarkdownWriter) block(s string) {
	if m.b.Len() > 0 || m.flushed {
		m.b.WriteString("\n")
	}
	m.b.WriteString(s)
	m.b.WriteString("\n")
}

// Heading writes an ATX heading of level 1–6 (out-of-range levels are
// clamped). Line breaks in text become spaces.
func (m *MarkdownWriter) Heading(level int, text string) {
	level = min(max(level, 1), 6)
	// inside an ATX heading nothing starts a block: "## 1. Scope" is a heading
	t := escapeInline(oneLine(text))
	if t == "" {
		t = "\u00a0" // an empty heading would not be a heading
	}
	if strings.HasSuffix(t, "#") && !strings.HasSuffix(t, `\#`) { // not a closing sequence
		t = t[:len(t)-1] + `\#`
	}
	m.block(strings.Repeat("#", level) + " " + t)
}

// Paragraph writes text as paragraphs: blank lines separate paragraphs,
// single line breaks are kept as hard breaks. Empty text writes nothing.
func (m *MarkdownWriter) Paragraph(text string) {
	var para []string
	flush := func() {
		if len(para) > 0 {
			m.block(strings.Join(para, "\\\n"))
			para = nil
		}
	}
	for _, l := range splitLines(text) {
		if l = MarkdownText(strings.TrimSpace(l)); l == "" {
			flush()
			continue
		}
		para = append(para, l)
	}
	flush()
}

// Emphasis writes text as an italic paragraph (e.g. "(no rows)").
func (m *MarkdownWriter) Emphasis(text string) {
	if t := MarkdownText(oneLine(text)); t != "" {
		m.block("*" + t + "*")
	}
}

// Item is a list entry: an optional bold label and a value.
type Item struct {
	Label, Value string
}

// List writes a bullet list. An item with a label is written as
// "**label:** value"; items without a value are left out.
func (m *MarkdownWriter) List(items ...Item) {
	var b strings.Builder
	for _, it := range items {
		v := MarkdownText(oneLine(it.Value))
		if v == "" {
			continue
		}
		if l := MarkdownText(oneLine(it.Label)); l != "" {
			v = "**" + l + ":** " + v
		}
		b.WriteString("- " + v + "\n")
	}
	if b.Len() > 0 {
		m.block(strings.TrimSuffix(b.String(), "\n"))
	}
}

// Table writes a GFM table. Every row has as many cells as the widest of
// columns and rows (missing cells are empty). link, when not nil, maps a
// cell value to a URL; such cells become links. Cells that are plain
// http(s) URLs become links too.
func (m *MarkdownWriter) Table(columns []string, rows [][]string, link func(string) string) {
	n := len(columns)
	for _, r := range rows {
		n = max(n, len(r))
	}
	if n == 0 {
		return
	}
	var b strings.Builder
	line := func(cells []string, conv func(string) string) {
		b.WriteString("|")
		for i := 0; i < n; i++ {
			c := conv(cell(cells, i))
			if c == "" {
				b.WriteString(" |")
				continue
			}
			b.WriteString(" " + c + " |")
		}
		b.WriteString("\n")
	}
	line(columns, MarkdownCell)
	b.WriteString("|" + strings.Repeat(" --- |", n) + "\n")
	conv := func(s string) string { return markdownLinkCell(s, link) }
	for _, r := range rows {
		line(r, conv)
	}
	m.block(strings.TrimSuffix(b.String(), "\n"))
}

// Link returns an inline link to url with text, or the escaped text alone
// when url is not a safe link target (http, https, mailto or relative).
func Link(text, url string) string {
	t := MarkdownText(oneLine(text))
	url = strings.TrimSpace(url)
	if url == "" || !safeURL(url) || strings.ContainsAny(url, "<>\n\r") {
		return t
	}
	if t == "" {
		t = MarkdownText(url)
	}
	url = strings.ReplaceAll(url, "|", "%7C") // would end a table cell
	if strings.ContainsAny(url, " ()") {
		url = "<" + url + ">"
	}
	return "[" + t + "](" + url + ")"
}

func markdownLinkCell(s string, link func(string) string) string {
	v := strings.TrimSpace(s)
	if v != "" && !strings.ContainsAny(v, " \t\r\n") {
		if link != nil {
			if u := link(v); u != "" {
				return Link(v, u)
			}
		}
		if l := strings.ToLower(v); strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			return Link(v, v)
		}
	}
	return MarkdownCell(s)
}

// MarkdownCell escapes s for a GFM table cell: inline text with pipes
// escaped and line breaks written as <br>.
func MarkdownCell(s string) string {
	lines := splitLines(s)
	for i, l := range lines {
		lines[i] = escapeInline(strings.TrimSpace(l))
	}
	return strings.Join(trimEmptyEnds(lines), "<br>")
}

// MarkdownText escapes s so that it shows literally as inline Markdown at
// the start of a block (no heading, list, quote or rule is started).
func MarkdownText(s string) string {
	return blockStart(escapeInline(s))
}

// blockStart escapes the character of an (already inline-escaped) line
// that would start a heading, list, quote or rule.
func blockStart(s string) string {
	// block starters: # > - + = and "1." / "1)" at the start of a line
	if s != "" && strings.ContainsRune("#>-+=", rune(s[0])) {
		s = `\` + s
	} else if m := mdOrdered.FindStringIndex(s); m != nil {
		s = s[:m[1]-1] + `\` + s[m[1]-1:]
	}
	return s
}

var mdOrdered = regexp.MustCompile(`^\d{1,9}[.)]`)

// escapeInline backslash-escapes the characters that have an inline
// meaning in GFM (emphasis, code, links, HTML, tables, strikethrough,
// entities) and removes control characters. An underscore between two
// letters or digits ("not_run") cannot start emphasis and is kept as is.
func escapeInline(s string) string {
	rs := []rune(strings.ToValidUTF8(s, "�"))
	word := func(i int) bool {
		return i >= 0 && i < len(rs) && (unicode.IsLetter(rs[i]) || unicode.IsDigit(rs[i]))
	}
	var b strings.Builder
	for i, r := range rs {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
		case r == '_' && word(i-1) && word(i+1):
			b.WriteRune(r)
		case strings.ContainsRune("\\`*_[]<>|~&!", r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.Split(s, "\n")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.Join(splitLines(s), " ")), " ")
}

func trimEmptyEnds(lines []string) []string {
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// WriteMarkdown writes ds as one GitHub-flavoured Markdown document: the
// title, the subtitle and metadata, a list of the sections when there are
// several, and one table per section.
func WriteMarkdown(w io.Writer, ds Dataset) error {
	var m MarkdownWriter
	if s := markdownLogo(ds.Logo); s != "" {
		m.block(s)
	}
	m.Heading(1, ds.Title)
	m.Paragraph(ds.Subtitle)
	meta := []Item{{Label: ds.label("generated"), Value: generatedAt(ds).Format("2006-01-02 15:04:05 MST")}}
	meta = append(meta, ds.Meta...)
	m.List(meta...)
	if len(ds.Sections) > 1 {
		m.Heading(2, ds.label("contents"))
		items := make([]Item, len(ds.Sections))
		for i, s := range ds.Sections {
			items[i] = Item{Value: ds.sectionTitle(s, i) + " (" + ds.rowCount(len(s.Rows)) + ")"}
		}
		m.List(items...)
	}
	for i, s := range ds.Sections {
		m.Heading(2, ds.sectionTitle(s, i))
		if width(s) == 0 {
			m.Emphasis(ds.label("noData"))
			continue
		}
		m.Table(s.Columns, s.Rows, ds.Link)
		if len(s.Rows) == 0 {
			m.Emphasis(ds.label("noRows"))
		}
	}
	_, err := m.WriteTo(w)
	return err
}

// rowCount is "1 row" / "n rows" in the dataset's language.
func (ds Dataset) rowCount(n int) string {
	if n == 1 {
		return ds.label("oneRow")
	}
	return strings.Replace(ds.label("nRows"), "%d", strconv.Itoa(n), 1)
}
