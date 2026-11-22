package reqimport

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReqIF documents read as a model (for the ReqIF import with re-import and
// relations, internal/interop): every SPEC-OBJECT with its IDENTIFIER, its
// attribute values by attribute long name, its place in the SPECIFICATION
// hierarchy, and the SPEC-RELATIONs. Unlike Parse, nothing is mapped to
// requirement fields here; the caller maps the attributes.

// ReqIFAttribute is an attribute definition (merged by long name over all
// object types) and what the document holds for it.
type ReqIFAttribute struct {
	Name string `json:"name"`
	// Type is the ReqIF data type: string, xhtml, enumeration, integer,
	// real, date or boolean.
	Type string `json:"type"`
	// Values are the enumeration's value names.
	Values []string `json:"values,omitempty"`
	// Objects counts the objects with a value; Sample is the first value.
	Objects int    `json:"objects"`
	Sample  string `json:"sample"`
	// ObjectTypes names the SPEC-OBJECT-TYPEs that define it.
	ObjectTypes []string `json:"object_types"`
}

// ReqIFObject is a SPEC-OBJECT.
type ReqIFObject struct {
	Identifier string `json:"identifier"`
	LongName   string `json:"long_name"`
	TypeName   string `json:"type"`
	LastChange string `json:"last_change"`
	// Values by attribute long name (XHTML as text, or as Markdown in a
	// Qtestframe export; enumerations as their value names, ", "-joined).
	Values map[string]string `json:"values"`
	// Parent is the IDENTIFIER of the object above it in the hierarchy
	// ("" at the top), Spec the long name of its SPECIFICATION ("" for
	// objects outside every specification) and Depth its level (0 = top).
	Parent string `json:"parent"`
	Spec   string `json:"spec"`
	Depth  int    `json:"depth"`
	// Order is the position in document order (hierarchy first).
	Order int `json:"order"`
}

// ReqIFRelation is a SPEC-RELATION between two objects.
type ReqIFRelation struct {
	Identifier string `json:"identifier"`
	Type       string `json:"type"` // the SPEC-RELATION-TYPE long name
	Source     string `json:"source"`
	Target     string `json:"target"`
}

// ReqIFModel is a parsed ReqIF document (or the documents of a .reqifz).
type ReqIFModel struct {
	Title      string `json:"title"`
	ToolID     string `json:"tool_id"`
	SourceTool string `json:"source_tool"`
	// Qtestframe is set for exports of Qtestframe (identifiers qtf-req-<id>).
	Qtestframe    bool             `json:"qtestframe"`
	Specs         []string         `json:"specifications"`
	Attributes    []ReqIFAttribute `json:"attributes"`
	Objects       []ReqIFObject    `json:"-"`
	Relations     []ReqIFRelation  `json:"-"`
	RelationTypes []string         `json:"relation_types"`
	Warnings      []string         `json:"warnings"`
	Errors        []string         `json:"errors"`
}

type rmDoc struct {
	Header struct {
		rqHeader
		Title string `xml:"TITLE"`
	} `xml:"THE-HEADER>REQ-IF-HEADER"`
	Content struct {
		Datatypes struct {
			All []rmDatatype `xml:",any"`
		} `xml:"DATATYPES"`
		SpecTypes struct {
			All []rmSpecType `xml:",any"`
		} `xml:"SPEC-TYPES"`
		Objects   []rqObject   `xml:"SPEC-OBJECTS>SPEC-OBJECT"`
		Relations []rmRelation `xml:"SPEC-RELATIONS>SPEC-RELATION"`
		Specs     []rqSpec     `xml:"SPECIFICATIONS>SPECIFICATION"`
	} `xml:"CORE-CONTENT>REQ-IF-CONTENT"`
}

type rmDatatype struct {
	XMLName xml.Name
	rqIdent
	Values []rqIdent `xml:"SPECIFIED-VALUES>ENUM-VALUE"`
}

type rmSpecType struct {
	XMLName xml.Name
	rqIdent
	Attrs struct {
		Defs []rmAttrDef `xml:",any"`
	} `xml:"SPEC-ATTRIBUTES"`
}

type rmAttrDef struct {
	XMLName xml.Name
	rqIdent
	Type rqDefRef `xml:"TYPE"`
}

type rmRelation struct {
	rqIdent
	Type   string `xml:"TYPE>SPEC-RELATION-TYPE-REF"`
	Source string `xml:"SOURCE>SPEC-OBJECT-REF"`
	Target string `xml:"TARGET>SPEC-OBJECT-REF"`
}

// attrKind turns ATTRIBUTE-DEFINITION-XHTML into "xhtml".
func attrKind(local string) string {
	k := strings.ToLower(strings.TrimPrefix(local, "ATTRIBUTE-DEFINITION-"))
	if k == "" || k == strings.ToLower(local) {
		return "string"
	}
	return k
}

// ParseReqIFModel reads a .reqif or .reqifz file. Errors wrapping
// ErrInvalid describe unusable files.
func ParseReqIFModel(data []byte) (m *ReqIFModel, err error) {
	defer func() {
		if p := recover(); p != nil {
			m, err = nil, invalidf("the file could not be read (%v)", p)
		}
	}()
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, invalidf("the file is empty")
	}
	var docs [][]byte
	if isZip(data) {
		zr, err := openZip(data, ".reqifz")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".reqif") {
				names = append(names, f.Name)
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return nil, invalidf("the .reqifz archive contains no .reqif file")
		}
		for _, n := range names {
			b, err := readZipEntry(zipFile(zr, n))
			if err != nil {
				return nil, err
			}
			docs = append(docs, b)
		}
	} else {
		docs = [][]byte{data}
	}
	m = &ReqIFModel{Attributes: []ReqIFAttribute{}, Specs: []string{}, RelationTypes: []string{}, Warnings: []string{}, Errors: []string{}}
	for _, d := range docs {
		if err := m.add(d); err != nil {
			return nil, err
		}
	}
	if len(m.Objects) > DefaultMaxCandidates {
		return nil, invalidf("the file contains %d objects; at most %d can be imported at once", len(m.Objects), DefaultMaxCandidates)
	}
	return m, nil
}

func (m *ReqIFModel) add(data []byte) error {
	var doc rmDoc
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
	c := &doc.Content
	if len(c.Objects) == 0 && len(c.Specs) == 0 {
		return invalidf("not a ReqIF file: no SPEC-OBJECTS found")
	}
	markdown := doc.Header.fromQtestframe()
	if markdown {
		m.Qtestframe = true
	}
	m.Title = firstNonEmpty(m.Title, strings.TrimSpace(doc.Header.Title))
	m.ToolID = firstNonEmpty(m.ToolID, strings.TrimSpace(doc.Header.ToolID))
	m.SourceTool = firstNonEmpty(m.SourceTool, strings.TrimSpace(doc.Header.SourceToolID))

	enumNames := map[string]string{}    // enum value id → long name
	enumValues := map[string][]string{} // datatype id → value names
	for _, dt := range c.Datatypes.All {
		if dt.XMLName.Local != "DATATYPE-DEFINITION-ENUMERATION" {
			continue
		}
		for _, v := range dt.Values {
			name := firstNonEmpty(v.Name, v.ID)
			enumNames[v.ID] = name
			enumValues[dt.ID] = append(enumValues[dt.ID], name)
		}
	}
	attrName := map[string]string{} // definition id → long name
	typeName := map[string]string{} // spec type id → long name
	relType := map[string]string{}
	attrIdx := map[string]int{}
	for i, a := range m.Attributes {
		attrIdx[strings.ToLower(a.Name)] = i
	}
	for _, st := range c.SpecTypes.All {
		name := firstNonEmpty(st.Name, st.ID)
		typeName[st.ID] = name
		if st.XMLName.Local == "SPEC-RELATION-TYPE" {
			relType[st.ID] = name
		}
		for _, a := range st.Attrs.Defs {
			if a.ID == "" {
				continue
			}
			an := firstNonEmpty(a.Name, a.ID)
			attrName[a.ID] = an
			if st.XMLName.Local != "SPEC-OBJECT-TYPE" {
				continue
			}
			key := strings.ToLower(an)
			i, ok := attrIdx[key]
			if !ok {
				i = len(m.Attributes)
				attrIdx[key] = i
				ra := ReqIFAttribute{Name: an, Type: attrKind(a.XMLName.Local), ObjectTypes: []string{}}
				if ra.Type == "enumeration" && len(a.Type.Refs) > 0 {
					ra.Values = enumValues[strings.TrimSpace(a.Type.Refs[0])]
				}
				m.Attributes = append(m.Attributes, ra)
			}
			if !containsFold(m.Attributes[i].ObjectTypes, name) {
				m.Attributes[i].ObjectTypes = append(m.Attributes[i].ObjectTypes, name)
			}
		}
	}
	byID := map[string]*rqObject{}
	for i := range c.Objects {
		byID[c.Objects[i].ID] = &c.Objects[i]
	}
	have := map[string]bool{}
	for _, o := range m.Objects {
		have[o.Identifier] = true
	}
	add := func(o *rqObject, parent, spec string, depth int) {
		have[o.ID] = true
		ob := ReqIFObject{Identifier: o.ID, LongName: o.Name, TypeName: typeName[strings.TrimSpace(o.Type)], Values: map[string]string{},
			Parent: parent, Spec: spec, Depth: depth, Order: len(m.Objects)}
		for _, v := range o.Values.All {
			if len(v.Def.Refs) == 0 {
				continue
			}
			name := attrName[strings.TrimSpace(v.Def.Refs[0])]
			if name == "" {
				continue
			}
			text := reqifValue(v, enumNames, markdown)
			if text == "" {
				continue
			}
			ob.Values[name] = text
			if i, ok := attrIdx[strings.ToLower(name)]; ok {
				a := &m.Attributes[i]
				a.Objects++
				if a.Sample == "" {
					a.Sample = truncRunes(strings.Join(strings.Fields(text), " "), 80)
				}
			}
		}
		m.Objects = append(m.Objects, ob)
	}
	var walk func(nodes []rqHier, parent, spec string, depth int)
	walk = func(nodes []rqHier, parent, spec string, depth int) {
		if depth > 64 {
			m.Warnings = append(m.Warnings, "the specification hierarchy is deeper than 64 levels; deeper objects were left out")
			return
		}
		for _, n := range nodes {
			id := strings.TrimSpace(n.Object)
			o := byID[id]
			switch {
			case o == nil:
				if id != "" {
					m.Errors = append(m.Errors, fmt.Sprintf("SPEC-HIERARCHY refers to the unknown SPEC-OBJECT %q; skipped", id))
				}
				walk(n.Children, parent, spec, depth+1)
				continue
			case !have[id]:
				add(o, parent, spec, depth)
			}
			walk(n.Children, id, spec, depth+1)
		}
	}
	for _, sp := range c.Specs {
		name := strings.TrimSpace(firstNonEmpty(sp.Name, sp.ID))
		m.Specs = append(m.Specs, name)
		walk(sp.Children, "", name, 0)
	}
	for i := range c.Objects {
		if o := &c.Objects[i]; !have[o.ID] {
			add(o, "", "", 0)
		}
	}
	for _, r := range c.Relations {
		t := firstNonEmpty(relType[strings.TrimSpace(r.Type)], strings.TrimSpace(r.Type))
		rel := ReqIFRelation{Identifier: r.ID, Type: firstNonEmpty(t, r.Name), Source: strings.TrimSpace(r.Source), Target: strings.TrimSpace(r.Target)}
		if rel.Source == "" || rel.Target == "" {
			m.Errors = append(m.Errors, fmt.Sprintf("SPEC-RELATION %q has no source or target; skipped", r.ID))
			continue
		}
		m.Relations = append(m.Relations, rel)
		if rel.Type != "" && !containsFold(m.RelationTypes, rel.Type) {
			m.RelationTypes = append(m.RelationTypes, rel.Type)
		}
	}
	return nil
}

// ReqIFRole guesses what an attribute (by long name) holds: a field of
// Fields, "heading" for a chapter name, "version" for the version of a
// Qtestframe export, or "" (ignored).
func ReqIFRole(name string) string {
	switch strings.TrimPrefix(normHeader(name), "reqif.") {
	case "version":
		return "version"
	case "foreigncreatedby", "foreigncreatedon", "foreignmodifiedby", "foreignmodifiedon", "foreigndeleted",
		"prefix", "attachments", "categories", "created", "updated", "author", "created by", "created on",
		"last modified by", "last modified on", "outline number", "object number", "object level":
		return ""
	}
	return reqifRole(name)
}

// ParseCriteria reads evaluation criteria, one per line ("method: text
// (threshold: x)").
func ParseCriteria(s string) []Criterion { return parseCriteria(s) }

// CleanCell trims a spreadsheet cell (see cleanCell).
func CleanCell(s string) string { return cleanCell(s) }

// SplitTags splits a tag list on , ; | and line breaks.
func SplitTags(s string) []string { return splitTags(s) }
