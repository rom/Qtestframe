package report

import (
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// Discussions in generated documents (DocOptions.Discussions): an appendix
// with the threads of the document's requirements (specification) or tests
// (plan), in document order. Removed comments are left out; resolved
// threads are marked.

func init() {
	for k, v := range discussionTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
}

// discussionTexts are the Swedish texts of the discussion appendix.
var discussionTexts = map[string]string{
	"Discussions": "Diskussioner",
	"The discussions of the items of this document: each thread with its replies, oldest first.": "Diskussionerna om objekten i dokumentet: varje tråd med sina svar, äldst först.",
	"Resolved":         "Löst",
	"Open":             "Öppen",
	"Resolved by {by}": "Löst av {by}",
	"Reply":            "Svar",
}

// discussionItems are the ids, keys and titles of the document's items in
// order that have discussions.
func (g *docGen) discussionItems() []struct{ id, key, title string } {
	var out []struct{ id, key, title string }
	if len(g.d.Discussions) == 0 {
		return nil
	}
	if g.kind == DocTestPlan {
		for _, t := range g.d.Tests {
			if len(g.d.Discussions[t.ID]) > 0 {
				out = append(out, struct{ id, key, title string }{t.ID, t.Key, t.Title})
			}
		}
		return out
	}
	for _, q := range g.d.Requirements {
		if len(g.d.Discussions[q.ID]) > 0 {
			out = append(out, struct{ id, key, title string }{q.ID, q.Key, q.Title})
		}
	}
	return out
}

func (g *docGen) hasDiscussions() bool {
	return g.o.Discussions && len(g.discussionItems()) > 0
}

// discussionAppendix adds the appendix of the discussions.
func (g *docGen) discussionAppendix() {
	items := g.discussionItems()
	if !g.o.Discussions || len(items) == 0 {
		return
	}
	l := g.l
	g.add(export.PageBreak())
	g.h(1, l.t("Discussions"))
	g.add(export.Paragraph(l.t("The discussions of the items of this document: each thread with its replies, oldest first.")))
	for _, it := range items {
		g.itemHeading(2, it.key, it.title)
		for _, th := range threadsOf(g.d.Discussions[it.id]) {
			root := th[0]
			state := l.t("Open")
			if root.ResolvedAt != nil {
				state = l.t("Resolved")
				if root.ResolvedByName != "" {
					state = g.f("Resolved by {by}", "by", root.ResolvedByName)
				}
			}
			var b strings.Builder
			b.WriteString(commentLine(root) + " [" + state + "]\n" + root.Body)
			for _, c := range th[1:] {
				b.WriteString("\n\n" + l.t("Reply") + " – " + commentLine(c) + "\n" + c.Body)
			}
			g.add(export.Paragraph(b.String()))
		}
	}
}

func commentLine(c model.Comment) string {
	who := c.UserName
	if who == "" {
		who = c.UserEmail
	}
	return who + ", " + c.CreatedAt.UTC().Format("2006-01-02 15:04")
}

// threadsOf groups comments (oldest first) into threads, each its first
// comment followed by its replies; threads whose first comment is missing
// (removed) start with their first reply.
func threadsOf(list []model.Comment) [][]model.Comment {
	var order []string
	byRoot := map[string][]model.Comment{}
	for _, c := range list {
		root := c.ID
		if c.ParentID != "" {
			root = c.ParentID
		}
		if _, ok := byRoot[root]; !ok {
			order = append(order, root)
		}
		if c.ParentID == "" {
			byRoot[root] = append([]model.Comment{c}, byRoot[root]...)
		} else {
			byRoot[root] = append(byRoot[root], c)
		}
	}
	out := make([][]model.Comment, 0, len(order))
	for _, id := range order {
		out = append(out, byRoot[id])
	}
	return out
}
