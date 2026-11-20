package export

import (
	"regexp"
	"strconv"
	"strings"
)

// MarkdownToXHTML converts the lightweight Markdown of requirement texts to
// an XHTML fragment whose elements carry the prefix "xhtml:" (the prefix
// must be bound to http://www.w3.org/1999/xhtml by the enclosing document,
// as ReqIF tools expect). Supported: paragraphs (blank-line separated, single
// newlines become <br/>), ATX headings (# …), bullet lists (- * +),
// numbered lists (1. or 1), renumbered from 1), fenced code blocks (```), and inline code,
// **strong**, *emphasis*, [links](url) and ![images](url). Everything else is
// kept as escaped text. image maps an image URL to the URL used in the
// output (nil keeps it); an image becomes an <xhtml:object>.
func MarkdownToXHTML(md string, image func(url string) (data, contentType string)) string {
	md = strings.ReplaceAll(strings.ReplaceAll(md, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(md, "\n")
	var b strings.Builder
	inline := func(s string) string { return mdInline(s, image) }
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, "```"):
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // closing fence (or end of text)
			b.WriteString("<xhtml:pre>" + esc(strings.Join(code, "\n")) + "</xhtml:pre>")
		case mdHeading.MatchString(trimmed):
			m := mdHeading.FindStringSubmatch(trimmed)
			n := strconv.Itoa(len(m[1]))
			b.WriteString("<xhtml:h" + n + ">" + inline(m[2]) + "</xhtml:h" + n + ">")
			i++
		case mdBullet.MatchString(line), mdNumber.MatchString(line):
			ordered := !mdBullet.MatchString(line)
			re, tag := mdBullet, "ul"
			if ordered {
				re, tag = mdNumber, "ol"
			}
			// no start attribute: the ReqIF XHTML subset has none
			b.WriteString("<xhtml:" + tag + ">")
			for i < len(lines) && re.MatchString(lines[i]) {
				m := re.FindStringSubmatch(lines[i])
				b.WriteString("<xhtml:li>" + inline(m[len(m)-1]) + "</xhtml:li>")
				i++
			}
			b.WriteString("</xhtml:" + tag + ">")
		default:
			var para []string
			for i < len(lines) {
				l := lines[i]
				t := strings.TrimSpace(l)
				if t == "" || strings.HasPrefix(t, "```") || mdHeading.MatchString(t) || mdBullet.MatchString(l) || mdNumber.MatchString(l) {
					break
				}
				para = append(para, inline(t))
				i++
			}
			b.WriteString("<xhtml:p>" + strings.Join(para, "<xhtml:br/>") + "</xhtml:p>")
		}
	}
	return "<xhtml:div>" + b.String() + "</xhtml:div>"
}

var (
	mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*$`)
	mdBullet  = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	mdNumber  = regexp.MustCompile(`^\s*(\d{1,9})[.)]\s+(.*)$`)
	// mdSpan finds inline code, images, links, strong and emphasis.
	mdSpan = regexp.MustCompile("`([^`]+)`" + `|!\[([^\]]*)\]\(([^)\s]+)\)|\[([^\]]+)\]\(([^)\s]+)\)|\*\*([^*\s](?:[^*]*[^*\s])?)\*\*|\*([^*\s](?:[^*]*[^*\s])?)\*`)
)

// safeURL reports whether a link target may be written to the document.
func safeURL(u string) bool {
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:") {
		return true
	}
	return !strings.Contains(l, ":") // relative
}

func mdInline(s string, image func(string) (string, string)) string {
	var b strings.Builder
	last := 0
	for _, m := range mdSpan.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(esc(s[last:m[0]]))
		last = m[1]
		g := func(i int) string {
			if m[2*i] < 0 {
				return ""
			}
			return s[m[2*i]:m[2*i+1]]
		}
		switch {
		case m[2] >= 0:
			b.WriteString("<xhtml:code>" + esc(g(1)) + "</xhtml:code>")
		case m[6] >= 0: // image
			alt, url := g(2), g(3)
			data, ct := url, ""
			if image != nil {
				data, ct = image(url)
			}
			if !safeURL(data) {
				b.WriteString(esc(s[m[0]:m[1]]))
				continue
			}
			if ct == "" {
				ct = imageType(data)
			}
			b.WriteString(`<xhtml:object data="` + esc(data) + `" type="` + esc(ct) + `">` + esc(alt) + "</xhtml:object>")
		case m[8] >= 0: // link
			text, url := g(4), g(5)
			if !safeURL(url) {
				b.WriteString(esc(s[m[0]:m[1]]))
				continue
			}
			b.WriteString(`<xhtml:a href="` + esc(url) + `">` + mdInline(text, nil) + "</xhtml:a>")
		case m[12] >= 0:
			b.WriteString("<xhtml:strong>" + mdInline(g(6), image) + "</xhtml:strong>")
		default:
			b.WriteString("<xhtml:em>" + mdInline(g(7), image) + "</xhtml:em>")
		}
	}
	b.WriteString(esc(s[last:]))
	return b.String()
}

// imageType guesses the MIME type of an image from its file extension.
func imageType(name string) string {
	n := strings.ToLower(name)
	if i := strings.IndexAny(n, "?#"); i >= 0 {
		n = n[:i]
	}
	switch {
	case strings.HasSuffix(n, ".jpg"), strings.HasSuffix(n, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(n, ".gif"):
		return "image/gif"
	case strings.HasSuffix(n, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(n, ".webp"):
		return "image/webp"
	}
	return "image/png"
}
