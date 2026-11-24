package gherkin

import (
	"strings"
	"unicode/utf8"
)

// EscapeCell escapes a table cell (|, \ and newlines).
func EscapeCell(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

// formatTable renders rows with aligned columns, each line prefixed with
// indent.
func formatTable(indent string, rows []*Row) []string {
	width := map[int]int{}
	for _, r := range rows {
		for i, c := range r.Cells {
			if n := utf8.RuneCountInString(EscapeCell(c)); n > width[i] {
				width[i] = n
			}
		}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		var b strings.Builder
		b.WriteString(indent + "|")
		for i, c := range r.Cells {
			e := EscapeCell(c)
			b.WriteString(" " + e + strings.Repeat(" ", width[i]-utf8.RuneCountInString(e)) + " |")
		}
		out = append(out, b.String())
	}
	return out
}

// Format renders a document in a canonical layout: two spaces per level,
// aligned tables, tags on their own line. Comments are not kept.
func Format(doc *Document) string {
	if doc == nil || doc.Feature == nil {
		return ""
	}
	var w writer
	if doc.Language != "" && doc.Language != "en" {
		w.line("", "# language: "+doc.Language)
	}
	f := doc.Feature
	w.tags("", f.Tags)
	w.header("", f.Keyword, f.Name)
	w.desc("  ", f.Description)
	if f.Background != nil {
		w.blank()
		w.background("  ", f.Background)
	}
	for _, sc := range f.Scenarios {
		w.blank()
		w.scenario("  ", sc)
	}
	for _, r := range f.Rules {
		w.blank()
		w.tags("  ", r.Tags)
		w.header("  ", r.Keyword, r.Name)
		w.desc("    ", r.Description)
		if r.Background != nil {
			w.blank()
			w.background("    ", r.Background)
		}
		for _, sc := range r.Scenarios {
			w.blank()
			w.scenario("    ", sc)
		}
	}
	return w.String()
}

type writer struct{ lines []string }

func (w *writer) line(indent, s string) { w.lines = append(w.lines, strings.TrimRight(indent+s, " ")) }
func (w *writer) blank() {
	if len(w.lines) > 0 && w.lines[len(w.lines)-1] != "" {
		w.lines = append(w.lines, "")
	}
}
func (w *writer) String() string { return strings.Join(w.lines, "\n") + "\n" }

func (w *writer) tags(indent string, tags []string) {
	if len(tags) > 0 {
		w.line(indent, strings.Join(tags, " "))
	}
}

func (w *writer) header(indent, kw, name string) {
	w.line(indent, kw+": "+name)
}

func (w *writer) desc(indent, d string) {
	if d = strings.TrimSpace(d); d == "" {
		return
	}
	for _, l := range strings.Split(d, "\n") {
		if strings.TrimSpace(l) == "" {
			w.lines = append(w.lines, "")
			continue
		}
		w.line(indent, strings.TrimSpace(l))
	}
}

func (w *writer) background(indent string, b *Background) {
	w.header(indent, b.Keyword, b.Name)
	w.desc(indent+"  ", b.Description)
	w.steps(indent+"  ", b.Steps)
}

func (w *writer) scenario(indent string, sc *Scenario) {
	w.tags(indent, sc.Tags)
	w.header(indent, sc.Keyword, sc.Name)
	w.desc(indent+"  ", sc.Description)
	w.steps(indent+"  ", sc.Steps)
	for _, ex := range sc.Examples {
		w.blank()
		w.tags(indent+"  ", ex.Tags)
		w.header(indent+"  ", ex.Keyword, ex.Name)
		w.desc(indent+"    ", ex.Description)
		rows := ex.Rows
		if ex.Header != nil {
			rows = append([]*Row{ex.Header}, rows...)
		}
		w.lines = append(w.lines, formatTable(indent+"    ", rows)...)
	}
}

func (w *writer) steps(indent string, steps []*Step) {
	for _, st := range steps {
		w.line(indent, st.Keyword+" "+st.Text)
		if st.DocString != nil {
			ds := st.DocString
			d := ds.Delimiter
			if d == "" {
				d = `"""`
			}
			w.line(indent+"  ", d+ds.MediaType)
			if ds.Content != "" {
				for _, l := range strings.Split(ds.Content, "\n") {
					l = strings.ReplaceAll(l, d, `\`+d[:1]+`\`+d[:1]+`\`+d[:1])
					if l == "" {
						w.lines = append(w.lines, "")
					} else {
						w.lines = append(w.lines, indent+"  "+l)
					}
				}
			}
			w.line(indent+"  ", d)
		}
		if len(st.Table) > 0 {
			w.lines = append(w.lines, formatTable(indent+"  ", st.Table)...)
		}
	}
}

// AddTags returns the source with tags added to the Feature (on a line of
// their own above it) unless the feature already has them; the rest of the
// text, comments included, is kept as it is.
func AddTags(src string, tags []string) (string, error) {
	doc, err := Parse(src)
	if err != nil {
		return src, err
	}
	have := map[string]bool{}
	for _, t := range doc.Feature.Tags {
		have[strings.ToLower(t)] = true
	}
	var add []string
	for _, t := range tags {
		t = strings.Join(strings.Fields(t), "-")
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "@") {
			t = "@" + t
		}
		if !have[strings.ToLower(t)] {
			have[strings.ToLower(t)] = true
			add = append(add, t)
		}
	}
	if len(add) == 0 {
		return src, nil
	}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	at := doc.Feature.Line - 1
	// above the feature's own tag lines (comments between them stay)
	for at > 0 {
		prev := strings.TrimSpace(lines[at-1])
		if !strings.HasPrefix(prev, "@") {
			break
		}
		at--
	}
	indent := lines[doc.Feature.Line-1][:len(lines[doc.Feature.Line-1])-len(strings.TrimLeft(lines[doc.Feature.Line-1], " \t"))]
	out := append([]string{}, lines[:at]...)
	out = append(out, indent+strings.Join(add, " "))
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n"), nil
}
