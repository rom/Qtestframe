package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The "Sources cited in the catalogue" section of the user guide page
// docs/user-guide/<lang>/references.md, generated from the source registry
// between two markers (go run ./internal/catalog/cmd/refsdoc, make
// refs-docs). TestReferencesDocIsCurrent fails when it is out of date.

//go:generate go run ./cmd/refsdoc -dir ../../docs/user-guide

// Markers around the generated section.
const (
	RefsDocBegin = "<!-- BEGIN generated sources: make refs-docs -->"
	RefsDocEnd   = "<!-- END generated sources -->"
)

// RefsDocPage is the guide page, relative to docs/user-guide/<lang>/.
const RefsDocPage = "references.md"

type refsDocText struct {
	heading, intro, other string
	jurisdictions         map[string]string
	reqs, tests, risks    func(n int) string
}

var refsDocTexts = map[string]refsDocText{
	"en": {
		heading: "Sources cited in the catalogue",
		intro: "The [requirement catalogue](catalogue.md) cites %d laws, standards, best practices and guidelines. " +
			"Each source has the number of catalogue requirements that cite it and of catalogue tests that cite it or verify such a requirement and, where there are any, of catalogue risks that cite it, " +
			"the same numbers as when you choose sources to comply with in the catalogue. " +
			"Where a source groups several documents, they are listed below it. " +
			"This section is generated from the catalogue.",
		other:         "Other",
		jurisdictions: map[string]string{"international": "International", "EU": "European Union", "SE": "Sweden", "US": "United States", "UK": "United Kingdom", "NO": "Norway"},
		reqs:          func(n int) string { return plural(n, "requirement", "requirements") },
		tests:         func(n int) string { return plural(n, "test", "tests") },
		risks:         func(n int) string { return plural(n, "risk", "risks") },
	},
	"sv": {
		heading: "Källor som katalogen hänvisar till",
		intro: "[Kravkatalogen](catalogue.md) hänvisar till %d lagar, standarder, bästa praxis och vägledningar. " +
			"Vid varje källa står hur många krav i katalogen som hänvisar till den och hur många testfall i katalogen som hänvisar till den eller verifierar ett sådant krav och, när det finns några, hur många risker i katalogen som hänvisar till den, " +
			"samma antal som när du väljer källor att uppfylla i katalogen. " +
			"När en källa samlar flera dokument listas de under den. " +
			"Avsnittet genereras från katalogen.",
		other:         "Övriga",
		jurisdictions: map[string]string{"international": "Internationella", "EU": "Europeiska unionen", "SE": "Sverige", "US": "USA", "UK": "Storbritannien", "NO": "Norge"},
		reqs:          func(n int) string { return plural(n, "krav", "krav") },
		tests:         func(n int) string { return plural(n, "testfall", "testfall") },
		risks:         func(n int) string { return plural(n, "risk", "risker") },
	},
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// jurisdictionRank orders international first, then EU, then the
// countries by code and sources without a jurisdiction last.
func jurisdictionRank(j string) string {
	switch j {
	case "international":
		return "0"
	case "EU":
		return "1"
	case "":
		return "3"
	}
	return "2" + j
}

// mdEscape escapes the characters that would start Markdown markup.
func mdEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_[]<>|", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func mdLink(text, url string) string {
	if url == "" {
		return mdEscape(text)
	}
	return "[" + mdEscape(text) + "](" + strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(url) + ")"
}

// sortKey folds case and puts å, ä, ö after z, as in Swedish.
func sortKey(s string) string {
	return strings.NewReplacer("å", "z{", "ä", "z|", "ö", "z}").Replace(strings.ToLower(s))
}

// SourcesMarkdown returns the generated section in lang (en or sv),
// markers included: the sources grouped by kind, then jurisdiction, each
// with its link, its counts and the links of its documents. Sources
// without a url are listed without a link.
func (c *Catalog) SourcesMarkdown(lang string) string {
	lang = Lang(lang)
	tx, ok := refsDocTexts[lang]
	if !ok {
		tx = refsDocTexts["en"]
	}
	all := c.SourcesOverview(lang, "", true)
	var b strings.Builder
	b.WriteString(RefsDocBegin + "\n\n")
	fmt.Fprintf(&b, "## %s\n\n", tx.heading)
	fmt.Fprintf(&b, tx.intro+"\n", len(all))
	for _, k := range c.SourceKinds {
		var ss []SourceSummary
		for _, s := range all {
			if s.Kind == k.ID {
				ss = append(ss, s)
			}
		}
		if len(ss) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n", k.Name.Get(lang))
		sort.SliceStable(ss, func(i, j int) bool {
			ri, rj := jurisdictionRank(ss[i].Jurisdiction), jurisdictionRank(ss[j].Jurisdiction)
			if ri != rj {
				return ri < rj
			}
			return sortKey(ss[i].Name) < sortKey(ss[j].Name)
		})
		cur := "-"
		for _, s := range ss {
			if s.Jurisdiction != cur {
				cur = s.Jurisdiction
				name := tx.jurisdictions[cur]
				switch {
				case cur == "":
					name = tx.other
				case name == "":
					name = cur
				}
				// the kind in the heading keeps the anchors unique
				fmt.Fprintf(&b, "\n#### %s: %s\n\n", k.Name.Get(lang), name)
			}
			c.writeSourceItem(&b, s, tx)
		}
	}
	b.WriteString("\n" + RefsDocEnd)
	return b.String()
}

func (c *Catalog) writeSourceItem(b *strings.Builder, s SourceSummary, tx refsDocText) {
	src := c.Source(s.ID)
	b.WriteString("- " + mdLink(s.Name, s.URL))
	if s.Short != "" && !strings.Contains(s.Name, s.Short) {
		b.WriteString(" (" + mdEscape(s.Short) + ")")
	}
	fmt.Fprintf(b, ": %s, %s", tx.reqs(s.Requirements), tx.tests(s.Tests))
	if s.Risks > 0 {
		b.WriteString(", " + tx.risks(s.Risks))
	}
	b.WriteString("\n")
	var docs []DocLink
	for _, l := range src.Links {
		docs = append(docs, DocLink{Label: l.Label, URL: l.URL})
	}
	docs = append(docs, c.CitedDocuments(s.ID)...)
	if len(docs) == 0 {
		return
	}
	parts := make([]string, len(docs))
	for i, d := range docs {
		parts[i] = mdLink(d.Label, d.URL)
	}
	// one line of documents keeps long families (RFCs) readable
	b.WriteString("  - " + strings.Join(parts, " · ") + "\n")
}

// ErrNoRefsDocMarkers: the page has only one of the two markers.
var ErrNoRefsDocMarkers = errors.New("references page: unbalanced generated-section markers")

// ReplaceRefsDoc returns page with the generated section replaced by
// section (markers included); a page without the markers gets the
// section appended.
func ReplaceRefsDoc(page, section string) (string, error) {
	i, j := strings.Index(page, RefsDocBegin), strings.Index(page, RefsDocEnd)
	switch {
	case i < 0 && j < 0:
		return strings.TrimRight(page, "\n") + "\n\n" + section + "\n", nil
	case i < 0 || j < i:
		return "", ErrNoRefsDocMarkers
	}
	return page[:i] + section + page[j+len(RefsDocEnd):], nil
}
