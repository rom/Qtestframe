package reqimport

import (
	"encoding/xml"
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
			case "p", "div", "table", "blockquote":
				if len(lists) == 0 {
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
			case "br", "tr":
				cur.WriteString("\n")
			case "td", "th":
				cur.WriteString(" ")
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
			case "a", "object":
				attr := "href"
				if name == "object" {
					attr = "data"
					cur.WriteString("!")
				}
				href := ""
				for _, a := range t.Attr {
					if strings.EqualFold(a.Name.Local, attr) {
						href = a.Value
					}
				}
				hrefs = append(hrefs, href)
				cur.WriteString("[")
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "p", "div", "table", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
				if len(lists) == 0 {
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
			case "a", "object":
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
