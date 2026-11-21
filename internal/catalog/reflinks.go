package catalog

import (
	"sort"
	"strings"
)

// Links from refs to the texts they cite: the source's url, one of its
// documents (links) or a document derived from the ref (ref_links).

// RefTarget is a ref with its source and the link to its text.
type RefTarget struct {
	Ref string `json:"ref"`
	// Source is the id of the source ("" when the ref matches none).
	Source string `json:"source,omitempty"`
	// Name is the source's name in the requested language.
	Name string `json:"name,omitempty"`
	// URL is the most specific link: the cited document, else the source.
	URL string `json:"url,omitempty"`
}

// RefTargets resolves refs to their sources and links, in lang.
func (c *Catalog) RefTargets(refs []string, lang string) []RefTarget {
	lang = Lang(lang)
	out := make([]RefTarget, 0, len(refs))
	for _, ref := range refs {
		t := RefTarget{Ref: ref}
		if s := c.Source(c.RefSource(ref)); s != nil {
			t.Source, t.Name = s.ID, s.Name.Get(lang)
			if d, ok := s.DocLink(ref); ok {
				t.URL = d.URL
			}
		}
		out = append(out, t)
	}
	return out
}

// SourceRefs returns the distinct refs of the catalogue items that cite
// the source, sorted.
func (c *Catalog) SourceRefs(id string) []string {
	var out []string
	for ref, sid := range c.refSource {
		if sid == id {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}

// CitedDocuments returns the documents the ref_links patterns of the
// source derive from the refs that cite it (each url once), in natural
// order of their labels ("RFC 791" before "RFC 2119").
func (c *Catalog) CitedDocuments(id string) []DocLink {
	s := c.Source(id)
	if s == nil || len(s.RefLinks) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []DocLink
	for _, ref := range c.SourceRefs(id) {
		if d, ok := s.refLink(ref); ok && !seen[d.URL] {
			seen[d.URL] = true
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return naturalLess(out[i].Label, out[j].Label) })
	return out
}

// naturalLess compares strings with runs of digits compared as numbers.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digitRun(a), digitRun(b)
		if da > 0 && db > 0 {
			na, nb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitRun(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}
