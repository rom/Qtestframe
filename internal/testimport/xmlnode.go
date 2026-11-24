package testimport

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// node is a generic XML element (the exports have no schema worth
// binding to structs: fields vary by tool version and configuration).
type node struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []node     `xml:",any"`
}

func parseXML(data []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var root node
	if err := d.Decode(&root); err != nil {
		return nil, invalidf("not a valid XML file (%v)", err)
	}
	return &root, nil
}

func (n *node) name() string { return strings.ToLower(n.XMLName.Local) }

// child returns the first child element named name (case-insensitive).
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for i := range n.Nodes {
		if strings.EqualFold(n.Nodes[i].XMLName.Local, name) {
			return &n.Nodes[i]
		}
	}
	return nil
}

// children returns the child elements named name.
func (n *node) children(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for i := range n.Nodes {
		if strings.EqualFold(n.Nodes[i].XMLName.Local, name) {
			out = append(out, &n.Nodes[i])
		}
	}
	return out
}

// path follows child names.
func (n *node) path(names ...string) *node {
	for _, s := range names {
		n = n.child(s)
	}
	return n
}

// text returns the trimmed text of the child element named name.
func (n *node) text(name string) string {
	return n.child(name).value()
}

// value returns the element's trimmed text; an element with a <value>
// child (TestRail dropdowns) returns that.
func (n *node) value() string {
	if n == nil {
		return ""
	}
	if v := n.child("value"); v != nil && strings.TrimSpace(n.Text) == "" {
		return strings.TrimSpace(v.Text)
	}
	return strings.TrimSpace(n.Text)
}

func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

// find returns the first element named name at any depth (n included).
func (n *node) find(name string) *node {
	if n == nil {
		return nil
	}
	if strings.EqualFold(n.XMLName.Local, name) {
		return n
	}
	for i := range n.Nodes {
		if f := n.Nodes[i].find(name); f != nil {
			return f
		}
	}
	return nil
}
