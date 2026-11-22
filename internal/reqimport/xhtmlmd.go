package reqimport

import (
	"encoding/xml"
	"path"
	"strconv"
	"strings"
)

// xhtmlMarkdown turns the XHTML of a Qtestframe ReqIF export back into the
// Markdown it was made from (the inverse of export.MarkdownToXHTML):
// paragraphs are separated by a blank line, <br/> is a line break, lists
// become "- " / "1. " lines, headings "# ", <pre> a fenced code block and
// strong, em, code, links and objects their inline Markdown.
func xhtmlMarkdown(inner string) string {
	dec := xml.NewDecoder(strings.NewReader("<x>" + inner + "</x>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	var (
		blocks []string
		cur    strings.Builder
		pre    bool
		lists  []int // item counter per open list; -1 = bullet list
		hrefs  []string
		// objects: the open <object>s; an inner object (the preview image
		// of an OLE object) replaces the outer one
		objects []*mdObject
		// tables: rows of cells of the open table
		table *mdTable
	)
	flush := func() {
		if pre {
			return
		}
		var ls []string
		for _, l := range strings.Split(cur.String(), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				ls = append(ls, l)
			}
		}
		if len(ls) > 0 {
			blocks = append(blocks, strings.Join(ls, "\n"))
		}
		cur.Reset()
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "table":
				if len(lists) == 0 {
					flush()
				}
				if table == nil {
					table = &mdTable{}
				}
			case "p", "div", "blockquote":
				if len(lists) == 0 && table == nil {
					flush()
				}
			case "h1", "h2", "h3", "h4", "h5", "h6":
				flush()
				n, _ := strconv.Atoi(name[1:])
				cur.WriteString(strings.Repeat("#", n) + " ")
			case "ul", "ol":
				if len(lists) == 0 {
					flush()
				}
				if name == "ul" {
					lists = append(lists, -1)
				} else {
					lists = append(lists, 0)
				}
			case "li":
				cur.WriteString("\n")
				if n := len(lists); n > 0 && lists[n-1] >= 0 {
					lists[n-1]++
					cur.WriteString(strconv.Itoa(lists[n-1]) + ". ")
				} else {
					cur.WriteString("- ")
				}
			case "br":
				if table != nil {
					cur.WriteString("<br>")
				} else {
					cur.WriteString("\n")
				}
			case "tr":
				if table != nil {
					table.rows = append(table.rows, nil)
				} else {
					cur.WriteString("\n")
				}
			case "td", "th":
				if table != nil {
					table.cellStart = cur.Len()
				} else {
					cur.WriteString(" ")
				}
			case "img":
				src, alt := "", ""
				for _, a := range t.Attr {
					switch strings.ToLower(a.Name.Local) {
					case "src":
						src = a.Value
					case "alt":
						alt = a.Value
					}
				}
				if src != "" {
					cur.WriteString("![" + alt + "](" + src + ")")
				}
			case "pre":
				flush()
				pre = true
				cur.WriteString("```\n")
			case "strong", "b":
				cur.WriteString("**")
			case "em", "i":
				cur.WriteString("*")
			case "code", "tt":
				if !pre {
					cur.WriteString("`")
				}
			case "object":
				o := &mdObject{start: cur.Len()}
				for _, a := range t.Attr {
					switch strings.ToLower(a.Name.Local) {
					case "data":
						o.data = a.Value
					case "type":
						o.typ = strings.ToLower(a.Value)
					case "name":
						o.name = a.Value
					}
				}
				if n := len(objects); n > 0 {
					// an inner object: an image stands for the outer one
					if outer := objects[n-1]; strings.HasPrefix(o.typ, "image/") && !strings.HasPrefix(outer.typ, "image/") {
						outer.data, outer.typ = o.data, o.typ
					}
					o.inner = true
				}
				objects = append(objects, o)
			case "a":
				href := ""
				for _, a := range t.Attr {
					if strings.EqualFold(a.Name.Local, "href") {
						href = a.Value
					}
				}
				hrefs = append(hrefs, href)
				cur.WriteString("[")
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "table":
				if table != nil {
					md := table.markdown()
					table = nil
					cur.Reset()
					if md != "" {
						blocks = append(blocks, md)
					}
				}
			case "td", "th":
				if table != nil && len(table.rows) > 0 {
					s := cur.String()
					if table.cellStart <= len(s) {
						s = s[table.cellStart:]
					}
					cur.Reset()
					cell := strings.Join(strings.Fields(strings.ReplaceAll(s, "|", `\|`)), " ")
					table.rows[len(table.rows)-1] = append(table.rows[len(table.rows)-1], cell)
				}
			case "p", "div", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
				if table != nil {
					cur.WriteString(" ")
				} else if len(lists) == 0 {
					flush()
				}
			case "ul", "ol":
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				if len(lists) == 0 {
					flush()
				}
			case "pre":
				code := strings.TrimRight(cur.String(), "\n")
				cur.Reset()
				blocks = append(blocks, code+"\n```")
				pre = false
			case "strong", "b":
				cur.WriteString("**")
			case "em", "i":
				cur.WriteString("*")
			case "code", "tt":
				if !pre {
					cur.WriteString("`")
				}
			case "object":
				n := len(objects)
				if n == 0 {
					continue
				}
				o := objects[n-1]
				objects = objects[:n-1]
				s := cur.String()
				label := ""
				if o.start <= len(s) {
					label = strings.TrimSpace(s[o.start:])
					cur.Reset()
					cur.WriteString(s[:o.start])
				}
				if o.inner {
					continue
				}
				label = firstNonEmpty(firstNonEmpty(label, o.name), path.Base(o.data))
				if o.data != "" {
					cur.WriteString("![" + label + "](" + o.data + ")")
				} else {
					cur.WriteString(label)
				}
			case "a":
				href := ""
				if n := len(hrefs); n > 0 {
					href, hrefs = hrefs[n-1], hrefs[:n-1]
				}
				cur.WriteString("](" + href + ")")
			}
		case xml.CharData:
			if pre {
				cur.Write(t)
				continue
			}
			text := strings.Join(strings.Fields(string(t)), " ")
			if text == "" {
				if len(t) > 0 && !strings.HasSuffix(cur.String(), " ") && !strings.HasSuffix(cur.String(), "\n") && cur.Len() > 0 {
					cur.WriteString(" ")
				}
				continue
			}
			if isSpace(t[0]) && cur.Len() > 0 && !strings.HasSuffix(cur.String(), " ") {
				cur.WriteString(" ")
			}
			cur.WriteString(text)
			if isSpace(t[len(t)-1]) {
				cur.WriteString(" ")
			}
		}
	}
	flush()
	return strings.Join(blocks, "\n\n")
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' }

// HTMLMarkdown turns the rich text of another tool (the HTML of Azure
// DevOps work item fields) into Markdown as xhtmlMarkdown does; elements it
// does not know keep only their text.
func HTMLMarkdown(html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	return xhtmlMarkdown(strings.NewReplacer("&nbsp;", " ", "<br>", "<br/>", "<BR>", "<br/>").Replace(html))
}

type mdObject struct {
	data, typ, name string
	start           int
	inner           bool
}

type mdTable struct {
	rows      [][]string
	cellStart int
}

// markdown renders the table as a GFM table, the first row as the header.
func (t *mdTable) markdown() string {
	var rows [][]string
	width := 0
	for _, r := range t.rows {
		if len(r) == 0 {
			continue
		}
		rows = append(rows, r)
		width = max(width, len(r))
	}
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	line := func(cells []string) {
		b.WriteString("|")
		for i := 0; i < width; i++ {
			c := ""
			if i < len(cells) {
				c = cells[i]
			}
			b.WriteString(" " + c + " |")
		}
		b.WriteString("\n")
	}
	line(rows[0])
	sep := make([]string, width)
	for i := range sep {
		sep[i] = "---"
	}
	line(sep)
	for _, r := range rows[1:] {
		line(r)
	}
	return strings.TrimRight(b.String(), "\n")
}

// XHTMLToMarkdown turns XHTML (the inner XML of a ReqIF or OSLC value)
// into Markdown: paragraphs, line breaks, lists, headings, code, emphasis,
// links, tables (GFM) and embedded objects and images (![name](path)).
func XHTMLToMarkdown(inner string) string { return xhtmlMarkdown(inner) }

// XHTMLText strips XHTML markup and keeps line breaks.
func XHTMLText(inner string) string { return xhtmlText(inner) }
