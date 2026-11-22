package interop

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

// ReqIF import: the objects of a ReqIF document become requirements, their
// attributes fields (a mapping by attribute name, guessed from the name and
// adjustable), the heading objects of the SPECIFICATION hierarchy folders
// and the SPEC-RELATIONs requirement relations. Re-importing a document
// updates the requirements imported from it before, matched by the
// objects' IDENTIFIERs.

// Roles an attribute can have besides a field.
const (
	RoleHeading    = "heading"     // chapter name: the object is a folder
	RoleExternalID = "external_id" // the other tool's id: recorded in Source of new requirements
	RoleVersion    = "version"     // the version of a Qtestframe export
)

// ReqIFAttr is an attribute of the document with its mapping.
type ReqIFAttr struct {
	reqimport.ReqIFAttribute
	// Field is the mapping in use ("" = ignored); Guess the automatic one.
	Field string `json:"field"`
	Guess string `json:"guess"`
}

// PlannedRelation is a relation of the document between two imported
// objects.
type PlannedRelation struct {
	Ref      string `json:"ref"`
	Type     string `json:"type"`      // model relation type
	FileType string `json:"file_type"` // the type's name in the file
	Source   string `json:"source"`    // object IDENTIFIERs
	Target   string `json:"target"`
}

// ReqIFPlan is a document mapped to incoming requirements.
type ReqIFPlan struct {
	Attributes []ReqIFAttr           `json:"attributes"`
	Rows       []Incoming            `json:"-"`
	Folders    int                   `json:"folders"`
	Relations  []PlannedRelation     `json:"-"`
	RelTypes   map[string]string     `json:"relation_types"` // file type → model type
	Skipped    []string              `json:"skipped"`
	Model      *reqimport.ReqIFModel `json:"-"`
}

// ReqIFOptions control the mapping.
type ReqIFOptions struct {
	// Mapping overrides the guessed role of attributes (by long name,
	// case-insensitive): a field key, a Role…, or "" / "ignore".
	Mapping map[string]string
	// Hierarchy makes heading objects folders (else every object with
	// text is a requirement and folders are not changed).
	Hierarchy bool
	// Root is the folder path the hierarchy goes below ("" = top level).
	Root string
}

// Match finds the stored requirement an object was imported as (by its
// IDENTIFIER) and the version written by that import (0 = unknown).
type Match func(identifier string) (id string, version int)

// GuessRole guesses the role of an attribute among fields.
func GuessRole(fields []Field, name string, qtf bool) string {
	switch reqimport.ReqIFRole(name) {
	case "heading":
		return RoleHeading
	case "version":
		if qtf {
			return RoleVersion
		}
		return ""
	case reqimport.FieldExternalID:
		return RoleExternalID
	case "":
	default:
		if f := FieldByKey(fields, reqimport.ReqIFRole(name)); f != nil && !f.ReadOnly && f.Type != Keys {
			return f.Key
		}
	}
	if f := FieldByHeader(fields, name); f != nil && !f.ReadOnly && f.Type != Keys {
		return f.Key
	}
	if strings.EqualFold(strings.TrimSpace(name), "severity") {
		return "obligation" // MoSCoW in Polarion
	}
	return ""
}

// CheckMapping validates a mapping against the fields.
func CheckMapping(fields []Field, mapping map[string]string) error {
	for attr, v := range mapping {
		v = strings.TrimSpace(v)
		switch v {
		case "", "ignore", RoleHeading, RoleExternalID, RoleVersion:
			continue
		}
		f := FieldByKey(fields, v)
		if f == nil || f.ReadOnly || f.Type == Keys {
			return fmt.Errorf("mapping for attribute %q: unknown field %q", attr, v)
		}
	}
	return nil
}

func mappingFor(mapping map[string]string, name string) (string, bool) {
	for k, v := range mapping {
		if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(name)) {
			v = strings.TrimSpace(v)
			if v == "ignore" {
				v = ""
			}
			return v, true
		}
	}
	return "", false
}

// RelationType maps a relation type name of another tool to a Qtestframe
// relation type (relates_to when nothing fits).
func RelationType(name string) string {
	n := strings.ToLower(strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == ' ' || r == '.' }), " "))
	if code := strings.ReplaceAll(n, " ", "_"); model.Valid(model.RelationTypes, code) {
		return code
	}
	switch {
	case strings.Contains(n, "refine"), strings.Contains(n, "satisf"), strings.Contains(n, "elaborat"), strings.Contains(n, "decompos"),
		strings.Contains(n, "förfin"), strings.Contains(n, "parent"), strings.Contains(n, "child"):
		return "refines"
	case strings.Contains(n, "deriv"), strings.Contains(n, "härled"), strings.Contains(n, "trace"):
		return "derived_from"
	case strings.Contains(n, "depend"), strings.Contains(n, "requires"), strings.Contains(n, "beror"):
		return "depends_on"
	case strings.Contains(n, "conflict"), strings.Contains(n, "konflikt"), strings.Contains(n, "contradict"):
		return "conflicts_with"
	case strings.Contains(n, "duplic"), strings.Contains(n, "dubbl"):
		return "duplicates"
	case strings.Contains(n, "provide"), strings.Contains(n, "implement"), strings.Contains(n, "realiz"), strings.Contains(n, "tillhandah"):
		return "provides"
	}
	return "relates_to"
}

func isHeadingType(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "heading") || strings.Contains(n, "chapter") || strings.Contains(n, "rubrik") || strings.Contains(n, "kapitel")
}

// PlanReqIF maps the objects of m to incoming requirement rows.
func PlanReqIF(m *reqimport.ReqIFModel, fields []Field, opt ReqIFOptions, match Match) *ReqIFPlan {
	p := &ReqIFPlan{Model: m, RelTypes: map[string]string{}, Skipped: []string{}, Attributes: []ReqIFAttr{}}
	role := map[string]string{} // attribute (lower) → role
	for _, a := range m.Attributes {
		ra := ReqIFAttr{ReqIFAttribute: a, Guess: GuessRole(fields, a.Name, m.Qtestframe)}
		ra.Field = ra.Guess
		if v, ok := mappingFor(opt.Mapping, a.Name); ok {
			ra.Field = v
		}
		role[strings.ToLower(a.Name)] = ra.Field
		p.Attributes = append(p.Attributes, ra)
	}
	for _, t := range m.RelationTypes {
		p.RelTypes[t] = RelationType(t)
	}
	valueOf := func(o *reqimport.ReqIFObject, want string) (string, string) {
		var vals, refs []string
		names := make([]string, 0, len(o.Values))
		for n := range o.Values {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if role[strings.ToLower(n)] == want {
				vals = append(vals, o.Values[n])
				refs = append(refs, n)
			}
		}
		return strings.Join(vals, "\n"), strings.Join(refs, ", ")
	}
	// headings
	heading := map[string]string{} // object id → folder name
	byID := map[string]*reqimport.ReqIFObject{}
	for i := range m.Objects {
		o := &m.Objects[i]
		byID[o.Identifier] = o
		if !opt.Hierarchy {
			continue
		}
		chapter, _ := valueOf(o, RoleHeading)
		text, _ := valueOf(o, "description")
		if chapter == "" && isHeadingType(o.TypeName) && text == "" {
			chapter, _ = valueOf(o, "title")
			if chapter == "" {
				chapter = o.LongName
			}
		}
		if chapter != "" && text == "" {
			heading[o.Identifier] = strings.ReplaceAll(strings.Join(strings.Fields(chapter), " "), "/", "-")
		}
	}
	p.Folders = len(heading)
	folderOf := func(o *reqimport.ReqIFObject) string {
		var parts []string
		seen := map[string]bool{}
		for id := o.Parent; id != "" && !seen[id]; {
			seen[id] = true
			if h, ok := heading[id]; ok {
				parts = append([]string{h}, parts...)
			}
			po := byID[id]
			if po == nil {
				break
			}
			id = po.Parent
		}
		if len(m.Specs) > 1 && o.Spec != "" {
			parts = append([]string{strings.ReplaceAll(o.Spec, "/", "-")}, parts...)
		}
		if opt.Root != "" {
			parts = append(reqimport.FolderParts(opt.Root), parts...)
		}
		return strings.Join(parts, "/")
	}
	imported := map[string]bool{}
	for i := range m.Objects {
		o := &m.Objects[i]
		if _, isHeading := heading[o.Identifier]; isHeading {
			continue
		}
		in := Incoming{Row: o.Order + 1, Ref: o.Identifier, Cells: map[string]Cell{}}
		extID, _ := valueOf(o, RoleExternalID)
		in.Location = "SPEC-OBJECT " + firstNonEmpty(strings.Split(extID, "\n")[0], o.LongName, o.Identifier)
		for _, f := range fields {
			if f.ReadOnly || f.Type == Keys || f.Key == "folder" {
				continue
			}
			if v, refs := valueOf(o, f.Key); v != "" {
				in.Cells[f.Key] = Cell{Raw: v, Ref: refs}
			}
		}
		if opt.Hierarchy {
			in.Cells["folder"] = Cell{Raw: folderOf(o), Ref: "SPECIFICATION"}
		} else if fv, refs := valueOf(o, "folder"); fv != "" {
			in.Cells["folder"] = Cell{Raw: fv, Ref: refs}
		}
		if match != nil {
			in.ID, in.BaseVersion = match(o.Identifier)
		}
		// without a title attribute a new requirement gets one from its
		// text; an existing one keeps its title
		_, hasTitle := in.Cells["title"]
		_, hasText := in.Cells["description"]
		if !hasTitle && !hasText {
			if ch, _ := valueOf(o, RoleHeading); ch != "" {
				in.Cells["title"] = Cell{Raw: ch, Ref: "chapter name"}
				hasTitle = true
			}
		}
		if !hasTitle && !hasText {
			p.Skipped = append(p.Skipped, in.Location+": no title or text")
			continue
		}
		if !hasTitle && in.ID == "" {
			d := in.Cells["description"]
			in.Cells["title"] = Cell{Raw: titleFrom(d.Raw), Ref: d.Ref}
			in.Warnings = append(in.Warnings, "title derived from the text")
		}
		if v, _ := valueOf(o, RoleVersion); v != "" && in.ID != "" && m.Qtestframe && in.BaseVersion == 0 {
			in.BaseVersion, _ = strconv.Atoi(strings.TrimSpace(v))
		}
		if in.ID == "" && extID != "" {
			// a new requirement records the other tool's id in its source
			src := in.Cells["source"].Raw
			if !strings.Contains(strings.ToLower(src), "id: "+strings.ToLower(extID)) {
				src = strings.TrimPrefix(strings.TrimSpace(src+", ID: "+strings.Split(extID, "\n")[0]), ", ")
			}
			in.Cells["source"] = Cell{Raw: src, Ref: "ID"}
			in.Key = strings.Split(extID, "\n")[0]
		}
		imported[o.Identifier] = true
		p.Rows = append(p.Rows, in)
	}
	for _, r := range m.Relations {
		if !imported[r.Source] || !imported[r.Target] || r.Source == r.Target {
			continue
		}
		p.Relations = append(p.Relations, PlannedRelation{Ref: r.Identifier, Type: RelationType(r.Type), FileType: r.Type, Source: r.Source, Target: r.Target})
	}
	return p
}

// titleFrom builds a title from the first words of a text.
func titleFrom(text string) string {
	first := strings.TrimSpace(text)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimLeft(first, "#-* ")
	words := strings.Fields(first)
	t := strings.Join(words, " ")
	if len(words) > 8 {
		t = strings.Join(words[:8], " ") + "…"
	}
	t = strings.TrimRight(t, ".,;:")
	if r := []rune(t); len(r) > 500 {
		t = string(r[:499]) + "…"
	}
	return t
}
