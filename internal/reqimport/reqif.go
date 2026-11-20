package reqimport

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReqIF 1.x documents. encoding/xml matches local names, so the namespace
// prefixes used by different tools do not matter.

type rqDoc struct {
	Header  rqHeader  `xml:"THE-HEADER>REQ-IF-HEADER"`
	Content rqContent `xml:"CORE-CONTENT>REQ-IF-CONTENT"`
}

type rqHeader struct {
	ToolID       string `xml:"REQ-IF-TOOL-ID"`
	SourceToolID string `xml:"SOURCE-TOOL-ID"`
}

// fromQtestframe reports whether the document was exported by Qtestframe:
// its XHTML is then read back as the Markdown it was made from.
func (h rqHeader) fromQtestframe() bool {
	return strings.TrimSpace(h.ToolID) == "Qtestframe" || strings.HasPrefix(strings.TrimSpace(h.SourceToolID), "Qtestframe")
}

type rqContent struct {
	Enums     []rqEnumType `xml:"DATATYPES>DATATYPE-DEFINITION-ENUMERATION"`
	Objects   []rqObject   `xml:"SPEC-OBJECTS>SPEC-OBJECT"`
	Specs     []rqSpec     `xml:"SPECIFICATIONS>SPECIFICATION"`
	SpecTypes struct {
		All []rqAnyType `xml:",any"`
	} `xml:"SPEC-TYPES"`
}

type rqEnumType struct {
	Values []rqIdent `xml:"SPECIFIED-VALUES>ENUM-VALUE"`
}

type rqIdent struct {
	ID   string `xml:"IDENTIFIER,attr"`
	Name string `xml:"LONG-NAME,attr"`
}

// rqAnyType is any spec type (object, specification, relation …); their
// attribute definitions are collected for name lookup.
type rqAnyType struct {
	Attrs struct {
		Defs []rqIdent `xml:",any"`
	} `xml:"SPEC-ATTRIBUTES"`
}

type rqObject struct {
	rqIdent
	Type   string `xml:"TYPE>SPEC-OBJECT-TYPE-REF"`
	Values struct {
		All []rqValue `xml:",any"`
	} `xml:"VALUES"`
}

type rqValue struct {
	XMLName  xml.Name
	Attr     string   `xml:"THE-VALUE,attr"`
	Def      rqDefRef `xml:"DEFINITION"`
	XHTML    rqInner  `xml:"THE-VALUE"`
	EnumRefs []string `xml:"VALUES>ENUM-VALUE-REF"`
}

type rqDefRef struct {
	Refs []string `xml:",any"`
}

type rqInner struct {
	Inner string `xml:",innerxml"`
}

type rqSpec struct {
	rqIdent
	Children []rqHier `xml:"CHILDREN>SPEC-HIERARCHY"`
}

type rqHier struct {
	Object   string   `xml:"OBJECT>SPEC-OBJECT-REF"`
	Children []rqHier `xml:"CHILDREN>SPEC-HIERARCHY"`
}

// reqifRole classifies attribute long names beyond the column table.
func reqifRole(name string) string {
	n := strings.TrimPrefix(normHeader(name), "reqif.")
	switch n {
	case "chaptername", "chapter name", "object heading", "heading":
		return "heading"
	case "name", "title", "shorttext", "short text":
		return FieldTitle
	case "text", "description", "object text", "longtext", "long text":
		return FieldDescription
	case "foreignid", "foreign id", "id", "identifier", "absolute number", "absolutenumber":
		return FieldExternalID
	case "prefix", "foreigncreatedby", "foreignmodifiedby", "foreigncreatedon", "foreignmodifiedon",
		"foreigndeleted", "category":
		return ""
	}
	f, _ := GuessField(name)
	return f
}

func parseReqIF(data []byte, opt Options, res *Result) error {
	var docs [][]byte
	if isZip(data) {
		zr, err := openZip(data, ".reqifz")
		if err != nil {
			return err
		}
		var names []string
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".reqif") {
				names = append(names, f.Name)
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return invalidf("the .reqifz archive contains no .reqif file")
		}
		for _, n := range names {
			b, err := readZipEntry(zipFile(zr, n))
			if err != nil {
				return err
			}
			docs = append(docs, b)
		}
	} else {
		docs = [][]byte{data}
	}
	for _, d := range docs {
		if err := parseReqIFDoc(d, opt, res); err != nil {
			return err
		}
	}
	return nil
}

func parseReqIFDoc(data []byte, opt Options, res *Result) error {
	var doc rqDoc
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		if l := strings.ToLower(label); l == "utf-8" || l == "us-ascii" || l == "ascii" {
			return in, nil
		}
		return nil, fmt.Errorf("unsupported encoding %q", label)
	}
	if err := dec.Decode(&doc); err != nil {
		return invalidf("not a valid ReqIF file (%v)", err)
	}
	c := doc.Content
	markdown := doc.Header.fromQtestframe()
	if len(c.Objects) == 0 && len(c.Specs) == 0 {
		return invalidf("not a ReqIF file: no SPEC-OBJECTS found")
	}
	enum := map[string]string{}
	for _, e := range c.Enums {
		for _, v := range e.Values {
			enum[v.ID] = v.Name
		}
	}
	// Attribute definitions: id → long name; the columns of the preview are
	// the distinct long names.
	attrName := map[string]string{}
	for _, t := range c.SpecTypes.All {
		for _, a := range t.Attrs.Defs {
			if a.ID != "" {
				attrName[a.ID] = firstNonEmpty(a.Name, a.ID)
			}
		}
	}
	colField := map[string]string{}
	for _, name := range attrName {
		if _, ok := colField[name]; ok {
			continue
		}
		f, explicit := mappedField(opt.Mapping, name)
		if !explicit {
			f = reqifRole(name)
		}
		colField[name] = f
	}
	for _, name := range sortedKeys(colField) {
		if containsColumn(res.Columns, name) {
			continue
		}
		f := colField[name]
		if f == "heading" {
			f = FieldTitle
		}
		res.Columns = append(res.Columns, Column{Index: len(res.Columns), Header: name, Field: f})
	}
	objects := map[string]*rqObject{}
	for i := range c.Objects {
		objects[c.Objects[i].ID] = &c.Objects[i]
	}
	used := map[string]bool{}
	ordinal := len(res.Candidates) + len(res.Errors)
	// candidate builds the candidate of an object; heading is the chapter
	// name when the object is a heading without text.
	candidate := func(o *rqObject, folder string) (cand *Candidate, heading string) {
		ordinal++
		cd := Candidate{Row: ordinal, Location: "SPEC-OBJECT " + firstNonEmpty(o.Name, o.ID), Folder: folder}
		var rw raw
		var chapter string
		for _, v := range o.Values.All {
			name := ""
			if len(v.Def.Refs) > 0 {
				name = attrName[strings.TrimSpace(v.Def.Refs[0])]
			}
			if name == "" {
				continue
			}
			text := reqifValue(v, enum, markdown)
			if text == "" {
				continue
			}
			switch colField[name] {
			case "heading":
				chapter = text
			case FieldTitle:
				cd.Title = strings.TrimSpace(cd.Title + " " + text)
			case FieldDescription:
				cd.Description = joinText(cd.Description, text)
			case FieldExternalID:
				cd.ExternalID = firstNonEmpty(cd.ExternalID, text)
			case FieldObligation:
				rw.obligation = text
			case FieldType:
				rw.typ = text
			case FieldPriority:
				rw.priority = text
			case FieldStatus:
				rw.status = text
			case FieldSource:
				cd.Source = firstNonEmpty(cd.Source, text)
			case FieldRationale:
				cd.Rationale = joinText(cd.Rationale, text)
			case FieldOwner:
				cd.Owner = firstNonEmpty(cd.Owner, text)
			case FieldTags:
				cd.Tags = append(cd.Tags, splitTags(text)...)
			case FieldFolder:
				cd.Folder = joinFolder(folder, text)
			case FieldCriteria:
				rw.criteria = joinText(rw.criteria, text)
			}
		}
		if chapter != "" && cd.Description == "" {
			return nil, chapter
		}
		if cd.Title == "" && chapter != "" {
			cd.Title = chapter
		}
		if !finish(&cd, rw) {
			res.errorf("%s: no title or text; skipped", cd.Location)
			return nil, ""
		}
		return &cd, ""
	}
	var walk func(nodes []rqHier, folder string, depth int)
	walk = func(nodes []rqHier, folder string, depth int) {
		if depth > 64 {
			res.warn("the specification hierarchy is deeper than 64 levels; deeper objects were left out")
			return
		}
		for _, n := range nodes {
			id := strings.TrimSpace(n.Object)
			o := objects[id]
			childFolder := folder
			if o == nil {
				if id != "" {
					res.errorf("SPEC-HIERARCHY refers to the unknown SPEC-OBJECT %q; skipped", id)
				}
			} else if !used[id] {
				used[id] = true
				cd, heading := candidate(o, folder)
				switch {
				case cd != nil:
					res.Candidates = append(res.Candidates, *cd)
				case heading != "":
					childFolder = joinFolder(folder, strings.ReplaceAll(heading, "/", "-"))
				}
			}
			walk(n.Children, childFolder, depth+1)
		}
	}
	for _, sp := range c.Specs {
		folder := ""
		if len(c.Specs) > 1 && sp.Name != "" {
			folder = strings.ReplaceAll(sp.Name, "/", "-")
		}
		walk(sp.Children, folder, 0)
	}
	// Objects outside every specification.
	for i := range c.Objects {
		o := &c.Objects[i]
		if used[o.ID] {
			continue
		}
		used[o.ID] = true
		if cd, _ := candidate(o, ""); cd != nil {
			res.Candidates = append(res.Candidates, *cd)
		}
	}
	return nil
}

func containsColumn(cols []Column, header string) bool {
	for _, c := range cols {
		if strings.EqualFold(c.Header, header) {
			return true
		}
	}
	return false
}

// reqifValue returns the text of an attribute value of any type.
func reqifValue(v rqValue, enum map[string]string, markdown bool) string {
	switch v.XMLName.Local {
	case "ATTRIBUTE-VALUE-XHTML":
		if markdown {
			return xhtmlMarkdown(v.XHTML.Inner)
		}
		return xhtmlText(v.XHTML.Inner)
	case "ATTRIBUTE-VALUE-ENUMERATION":
		var names []string
		for _, r := range v.EnumRefs {
			if n := enum[strings.TrimSpace(r)]; n != "" {
				names = append(names, n)
			}
		}
		return strings.Join(names, ", ")
	}
	return strings.TrimSpace(v.Attr)
}

// blockElements end a line in XHTML text.
var blockElements = map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "ul": true, "ol": true, "table": true, "pre": true, "blockquote": true,
	"dd": true, "dt": true}

// xhtmlText strips XHTML markup and keeps line breaks; list items start
// with "- ".
func xhtmlText(inner string) string {
	dec := xml.NewDecoder(strings.NewReader("<x>" + inner + "</x>"))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if blockElements[name] {
				b.WriteString("\n")
			}
			if name == "li" {
				b.WriteString("- ")
			}
			if name == "td" || name == "th" {
				b.WriteString(" ")
			}
		case xml.EndElement:
			if blockElements[strings.ToLower(t.Name.Local)] {
				b.WriteString("\n")
			}
		case xml.CharData:
			b.Write(t)
		}
	}
	return tidyLines(b.String())
}

// tidyLines collapses spaces within lines and drops empty lines.
func tidyLines(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
