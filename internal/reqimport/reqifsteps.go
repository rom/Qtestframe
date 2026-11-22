package reqimport

import (
	"encoding/xml"
	"strings"
)

// Test steps and embedded objects in the XHTML of ReqIF documents.

// ReqIFStep is a test step read from XHTML.
type ReqIFStep struct {
	Action, Data, Expected string
	// Call is the key of a called test (the "Calls" column of a
	// Qtestframe export).
	Call string
}

type xnode struct {
	name     string
	attrs    map[string]string
	children []*xnode
	text     string // CharData (for name == "")
}

func parseXHTML(inner string) *xnode {
	dec := xml.NewDecoder(strings.NewReader("<x>" + inner + "</x>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	root := &xnode{name: "root"}
	stack := []*xnode{root}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: strings.ToLower(t.Name.Local), attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			top.children = append(top.children, &xnode{text: string(t)})
		}
	}
	return root
}

func (n *xnode) walk(fn func(*xnode) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.children {
		c.walk(fn)
	}
}

// textOf returns the text of a node, <br> and block ends as line breaks.
func (n *xnode) textOf() string {
	var b strings.Builder
	var rec func(x *xnode)
	rec = func(x *xnode) {
		if x.name == "" {
			b.WriteString(x.text)
			return
		}
		if x.name == "br" {
			b.WriteString("\n")
		}
		for _, c := range x.children {
			rec(c)
		}
		if blockElements[x.name] && x.name != "br" {
			b.WriteString("\n")
		}
	}
	rec(n)
	return tidyLines(b.String())
}

// ParseStepsXHTML reads test steps from XHTML: the rows of a table (with
// an optional header row naming the columns Action, Data, Expected
// result, Calls; without one 1 column is the action, 2 action and expected
// result, 3 or more action, data and expected result) or else the items
// of a list (one step each; "action → expected" or "action | expected").
func ParseStepsXHTML(inner string) []ReqIFStep {
	root := parseXHTML(inner)
	var rows [][]*xnode
	var items []*xnode
	root.walk(func(n *xnode) bool {
		switch n.name {
		case "tr":
			var cells []*xnode
			for _, c := range n.children {
				if c.name == "td" || c.name == "th" {
					cells = append(cells, c)
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return false
		case "li":
			items = append(items, n)
			return false
		}
		return true
	})
	var out []ReqIFStep
	if len(rows) > 0 {
		col := map[string]int{"action": 0, "data": -1, "expected": -1, "call": -1}
		start := 0
		header := true
		for _, c := range rows[0] {
			if c.name != "th" {
				header = false
			}
		}
		named := false
		if header || len(rows) > 1 {
			for i, c := range rows[0] {
				switch h := strings.ToLower(c.textOf()); {
				case strings.Contains(h, "expect") || strings.Contains(h, "förvänt") || strings.Contains(h, "result"):
					col["expected"], named = i, true
				case strings.Contains(h, "data") || strings.Contains(h, "input") || strings.Contains(h, "indata"):
					col["data"], named = i, true
				case strings.Contains(h, "call") || strings.Contains(h, "anrop"):
					col["call"], named = i, true
				case strings.Contains(h, "action") || strings.Contains(h, "step") || strings.Contains(h, "åtgärd") || strings.Contains(h, "steg"):
					col["action"], named = i, true
				case h == "#" || h == "no" || h == "nr" || h == "no.":
					if col["action"] == i {
						col["action"] = i + 1
					}
				}
			}
		}
		if named || header {
			start = 1
		} else {
			switch n := len(rows[0]); {
			case n == 2:
				col["expected"] = 1
			case n >= 3:
				col["data"], col["expected"] = 1, 2
			}
		}
		get := func(cells []*xnode, i int) string {
			if i < 0 || i >= len(cells) {
				return ""
			}
			return cells[i].textOf()
		}
		for _, cells := range rows[start:] {
			st := ReqIFStep{Action: get(cells, col["action"]), Data: get(cells, col["data"]), Expected: get(cells, col["expected"]), Call: get(cells, col["call"])}
			if st.Action != "" || st.Expected != "" || st.Call != "" {
				out = append(out, st)
			}
		}
		return out
	}
	for _, li := range items {
		text := li.textOf()
		st := ReqIFStep{Action: text}
		for _, sep := range []string{" → ", " -> ", " | "} {
			if a, e, ok := strings.Cut(text, sep); ok {
				st.Action, st.Expected = strings.TrimSpace(a), strings.TrimSpace(e)
				break
			}
		}
		if st.Action != "" {
			out = append(out, st)
		}
	}
	return out
}

// EmbeddedRefs returns the files XHTML embeds or links to by a relative
// path: the data of <object>s (the innermost of nested ones), the src of
// <img>s and the href of links to files of the archive.
func EmbeddedRefs(inner string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] || strings.Contains(ref, "://") || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "/") ||
			strings.HasPrefix(ref, "mailto:") || strings.HasPrefix(ref, "data:") {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	parseXHTML(inner).walk(func(n *xnode) bool {
		switch n.name {
		case "object":
			add(n.attrs["data"])
		case "img":
			add(n.attrs["src"])
		case "a":
			add(n.attrs["href"])
		}
		return true
	})
	return out
}
