package tracker

import (
	"strings"
)

// Doc builds an issue description in a tracker's markup: GitLab/GitHub
// Markdown or Jira wiki markup.
type Doc struct {
	markup string
	parts  []string
}

// NewDoc starts a description in the markup of a client (Client.Markup).
func NewDoc(markup string) *Doc { return &Doc{markup: markup} }

func (d *Doc) jira() bool { return d.markup == MarkupJira }

// Heading adds a section heading.
func (d *Doc) Heading(text string) {
	if d.jira() {
		d.parts = append(d.parts, "h3. "+oneLine(text))
		return
	}
	d.parts = append(d.parts, "### "+oneLine(text))
}

// Text adds a paragraph of the user's own text, as it is.
func (d *Doc) Text(text string) {
	if t := strings.TrimSpace(text); t != "" {
		d.parts = append(d.parts, t)
	}
}

// Note adds a quoted note (already formatted inline text).
func (d *Doc) Note(text string) {
	if d.jira() {
		d.parts = append(d.parts, "{quote}"+text+"{quote}")
		return
	}
	d.parts = append(d.parts, "> "+strings.ReplaceAll(text, "\n", "\n> "))
}

// Fields adds a bullet list of name: value pairs; empty values are left out.
func (d *Doc) Fields(pairs [][2]string) {
	var lines []string
	for _, p := range pairs {
		if strings.TrimSpace(p[1]) == "" {
			continue
		}
		if d.jira() {
			lines = append(lines, "* *"+p[0]+":* "+oneLine(p[1]))
		} else {
			lines = append(lines, "- **"+p[0]+":** "+oneLine(p[1]))
		}
	}
	if len(lines) > 0 {
		d.parts = append(d.parts, strings.Join(lines, "\n"))
	}
}

// Table adds a table. Cells are inline markup: pass plain text through Cell.
func (d *Doc) Table(head []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	var b strings.Builder
	if d.jira() {
		b.WriteString("||" + strings.Join(head, "||") + "||")
		for _, r := range rows {
			b.WriteString("\n|" + strings.Join(r, "|") + "|")
		}
	} else {
		b.WriteString("| " + strings.Join(head, " | ") + " |\n|")
		for range head {
			b.WriteString(" --- |")
		}
		for _, r := range rows {
			b.WriteString("\n| " + strings.Join(r, " | ") + " |")
		}
	}
	d.parts = append(d.parts, b.String())
}

// Cell escapes plain text for a table cell.
func (d *Doc) Cell(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return " "
	}
	t = strings.ReplaceAll(t, "|", `\|`)
	br := "<br>"
	if d.jira() {
		br = `\\ `
	}
	return strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\n", br)
}

// Link is inline markup for a link (just the text without a URL).
func (d *Doc) Link(text, url string) string {
	if url == "" {
		return text
	}
	if d.jira() {
		return "[" + strings.NewReplacer("[", "(", "]", ")", "|", "/").Replace(text) + "|" + url + "]"
	}
	return "[" + strings.NewReplacer("[", `\[`, "]", `\]`).Replace(text) + "](" + url + ")"
}

// Bold is inline markup for bold text.
func (d *Doc) Bold(text string) string {
	if d.jira() {
		return "*" + text + "*"
	}
	return "**" + text + "**"
}

// String is the description.
func (d *Doc) String() string { return strings.Join(d.parts, "\n\n") }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Markers around the part of a Markdown description that Qtestframe
// manages: an update replaces what is between them and keeps the rest.
const (
	BeginMarker = "<!-- qtestframe:begin -->"
	EndMarker   = "<!-- qtestframe:end -->"
)

// Managed wraps a generated Markdown description in the markers (Jira
// descriptions are not wrapped: wiki markup has no comments).
func Managed(markup, body string) string {
	if markup == MarkupJira {
		return body
	}
	return BeginMarker + "\n" + body + "\n" + EndMarker
}

// MergeManaged puts a new managed part into the current description: it
// replaces the old part between the markers and keeps the text around it.
// A description without the markers (or a Jira one) is replaced, except
// that text someone wrote in GitLab or GitHub is kept below the new part.
func MergeManaged(markup, current, managed string) string {
	if markup == MarkupJira {
		return managed
	}
	i := strings.Index(current, BeginMarker)
	j := strings.Index(current, EndMarker)
	if i >= 0 && j > i {
		return current[:i] + managed + current[j+len(EndMarker):]
	}
	if c := strings.TrimSpace(current); c != "" {
		return managed + "\n\n" + c
	}
	return managed
}
