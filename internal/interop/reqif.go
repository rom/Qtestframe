package interop

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

// ReqIF import: the objects of a ReqIF document become requirements or
// test cases (by their SPEC-OBJECT-TYPE: the "Test case" type of
// Qtestframe's export and types named like it are tests, a mapping by type
// name adjusts it), their attributes fields (a mapping by attribute name per
// kind, guessed from the name and adjustable; enumerations map to built-in
// and custom fields alike), the heading objects of the SPECIFICATION
// hierarchy folders, objects nested below a requirement optionally refine
// it, and the SPEC-RELATIONs become requirement relations or, between a
// test and a requirement, trace links (a mapping by relation type name
// adjusts the types). Re-importing a document updates the items imported
// from it before, matched by the objects' IDENTIFIERs.

// Roles an attribute can have besides a field.
const (
	RoleHeading    = "heading"     // chapter name: the object is a folder
	RoleExternalID = "external_id" // the other tool's id: recorded in Source of new requirements
	RoleVersion    = "version"     // the version of a Qtestframe export
	RoleSteps      = "steps"       // the steps of a test (a table or list)
)

// Object kinds of the type mapping.
const (
	ObjRequirement = "requirement"
	ObjTest        = "test"
	ObjHeading     = "heading"
	ObjIgnore      = "ignore"
)

// RelVerifies is the relation mapping that makes a trace link (a test
// verifies a requirement); RelIgnore leaves relations of a type out.
const (
	RelVerifies = "verifies"
	RelIgnore   = "ignore"
)

// ReqIFAttr is an attribute of the document with its mapping.
type ReqIFAttr struct {
	reqimport.ReqIFAttribute
	// Field is the mapping in use ("" = ignored); Guess the automatic one.
	Field string `json:"field"`
	Guess string `json:"guess"`
}

// ReqIFType is a SPEC-OBJECT-TYPE of the document with its kind.
type ReqIFType struct {
	Name    string `json:"name"`
	Objects int    `json:"objects"`
	// Kind is the mapping in use (ObjRequirement …); Guess the automatic one.
	Kind  string `json:"kind"`
	Guess string `json:"guess"`
}

// ReqIFRelType is a SPEC-RELATION-TYPE with its mapping.
type ReqIFRelType struct {
	Name      string `json:"name"`
	Relations int    `json:"relations"`
	// Type is the mapping in use: a relation type, RelVerifies or
	// RelIgnore; Guess the automatic one.
	Type  string `json:"type"`
	Guess string `json:"guess"`
}

// PlannedRelation is a relation of the document between two imported
// objects: a requirement relation, or a trace link (Type RelVerifies,
// Source the test, Target the requirement).
type PlannedRelation struct {
	Ref      string `json:"ref"`
	Type     string `json:"type"`      // model relation type or RelVerifies
	FileType string `json:"file_type"` // the type's name in the file ("" for the hierarchy)
	Source   string `json:"source"`    // object IDENTIFIERs
	Target   string `json:"target"`
}

// ReqIFPlan is a document mapped to incoming requirements and tests.
type ReqIFPlan struct {
	Attributes     []ReqIFAttr       `json:"attributes"`
	TestAttributes []ReqIFAttr       `json:"test_attributes"`
	Types          []ReqIFType       `json:"object_types"`
	RelationTypes  []ReqIFRelType    `json:"relation_type_mapping"`
	Rows           []Incoming        `json:"-"`
	TestRows       []Incoming        `json:"-"`
	Folders        int               `json:"folders"`
	TestFolders    int               `json:"test_folders"`
	Relations      []PlannedRelation `json:"-"`
	// RelTypes: file type → model type (as mapped).
	RelTypes map[string]string     `json:"relation_types"`
	Skipped  []string              `json:"skipped"`
	Model    *reqimport.ReqIFModel `json:"-"`
	// Kind of each imported object (IDENTIFIER → requirement | test).
	Kinds map[string]string `json:"-"`
	// TestKeys: the ReqIF.ForeignID (key in the other tool) of test
	// objects → IDENTIFIER, for the calls of test steps.
	TestKeys map[string]string `json:"-"`
}

// ReqIFOptions control the mapping.
type ReqIFOptions struct {
	// Mapping overrides the guessed role of requirement attributes (by long
	// name, case-insensitive): a field key, a Role…, or "" / "ignore";
	// TestMapping those of test attributes.
	Mapping     map[string]string
	TestMapping map[string]string
	// TestFields are the fields of tests (nil: no test is imported).
	TestFields []Field
	// ObjectTypes overrides the guessed kind of object types (by long name):
	// ObjRequirement, ObjTest, ObjHeading or ObjIgnore.
	ObjectTypes map[string]string
	// RelationTypes overrides the guessed mapping of relation types (by
	// long name): a model relation type, RelVerifies or RelIgnore.
	RelationTypes map[string]string
	// Hierarchy makes heading objects folders (else every object with
	// text is a requirement and folders are not changed).
	Hierarchy bool
	// Refines: a requirement nested below another requirement in the
	// SPECIFICATION hierarchy refines it.
	Refines bool
	// Root and TestRoot are the folder paths the hierarchies go below
	// ("" = top level).
	Root, TestRoot string
	// MatchTest finds the stored test of a test object.
	MatchTest Match
}

// Match finds the stored item an object was imported as (by its
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
	if strings.EqualFold(strings.TrimSpace(name), "severity") && FieldByKey(fields, "obligation") != nil {
		return "obligation" // MoSCoW in Polarion
	}
	return ""
}

// GuessTestRole guesses the role of an attribute of test objects.
func GuessTestRole(fields []Field, name string, qtf bool) string {
	switch strings.ToLower(strings.Join(strings.Fields(strings.TrimPrefix(strings.ToLower(name), "reqif.")), " ")) {
	case "steps", "test steps", "teststeps", "step", "teststeg", "steg", "test script steps":
		return RoleSteps
	case "expected", "expected results", "förväntat resultat":
		if FieldByKey(fields, "expected_result") != nil {
			return "expected_result"
		}
	case "preconditions", "pre-conditions", "precondition", "prerequisites", "setup":
		if FieldByKey(fields, "preconditions") != nil {
			return "preconditions"
		}
	}
	return GuessRole(fields, name, qtf)
}

// CheckMapping validates a mapping against the fields.
func CheckMapping(fields []Field, mapping map[string]string) error {
	for attr, v := range mapping {
		v = strings.TrimSpace(v)
		switch v {
		case "", "ignore", RoleHeading, RoleExternalID, RoleVersion:
			continue
		case RoleSteps:
			if FieldByKey(fields, "preconditions") != nil {
				continue
			}
		}
		f := FieldByKey(fields, v)
		if f == nil || f.ReadOnly || f.Type == Keys {
			return fmt.Errorf("mapping for attribute %q: unknown field %q", attr, v)
		}
	}
	return nil
}

// CheckTypeMappings validates the object and relation type mappings.
func CheckTypeMappings(objectTypes, relationTypes map[string]string) error {
	for name, v := range objectTypes {
		switch strings.TrimSpace(v) {
		case ObjRequirement, ObjTest, ObjHeading, ObjIgnore, "":
		default:
			return fmt.Errorf("object_types: %q for %q is not requirement, test, heading or ignore", v, name)
		}
	}
	for name, v := range relationTypes {
		v = strings.TrimSpace(v)
		if v != "" && v != RelVerifies && v != RelIgnore && !model.Valid(model.RelationTypes, v) {
			return fmt.Errorf("relation_types: %q for %q is not a relation type, verifies or ignore", v, name)
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

func rawMappingFor(mapping map[string]string, name string) (string, bool) {
	for k, v := range mapping {
		if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(name)) && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

func relWords(name string) string {
	return strings.ToLower(strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == ' ' || r == '.' }), " "))
}

// RelationType maps a relation type name of another tool to a Qtestframe
// relation type (relates_to when nothing fits).
func RelationType(name string) string {
	n := relWords(name)
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

// GuessRelationMapping guesses the mapping of a relation type: trace links
// for "verifies", "validated by", "tested by" and the like, else
// RelationType.
func GuessRelationMapping(name string) string {
	n := relWords(name)
	switch {
	case strings.Contains(n, "record of"):
		return RelIgnore
	case strings.Contains(n, "verif"), strings.Contains(n, "validat"), strings.Contains(n, "tested by"), strings.Contains(n, "tests"),
		strings.Contains(n, "testad"), strings.Contains(n, "covered by"), strings.Contains(n, "täcks"):
		return RelVerifies
	}
	return RelationType(name)
}

// GuessObjectKind guesses what objects of a SPEC-OBJECT-TYPE are.
func GuessObjectKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "verification record"), strings.Contains(n, "verifieringspost"):
		return ObjIgnore
	case strings.Contains(n, "requirement"), strings.Contains(n, "krav"):
		return ObjRequirement
	case strings.Contains(n, "test"):
		return ObjTest
	case isHeadingType(name):
		return ObjHeading
	}
	return ObjRequirement
}

func isHeadingType(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "heading") || strings.Contains(n, "chapter") || strings.Contains(n, "rubrik") || strings.Contains(n, "kapitel")
}

// attrsOf maps the attributes defined by object types of kind.
func attrsOf(m *reqimport.ReqIFModel, fields []Field, mapping map[string]string, types map[string]bool, guess func([]Field, string, bool) string) ([]ReqIFAttr, map[string]string) {
	out := []ReqIFAttr{}
	role := map[string]string{} // attribute (lower) → role
	for _, a := range m.Attributes {
		used := len(a.ObjectTypes) == 0
		for _, t := range a.ObjectTypes {
			if types[t] {
				used = true
			}
		}
		if !used {
			continue
		}
		ra := ReqIFAttr{ReqIFAttribute: a, Guess: guess(fields, a.Name, m.Qtestframe)}
		ra.Field = ra.Guess
		if v, ok := mappingFor(mapping, a.Name); ok {
			ra.Field = v
		}
		role[strings.ToLower(a.Name)] = ra.Field
		out = append(out, ra)
	}
	return out, role
}

// PlanReqIF maps the objects of m to incoming requirement and test rows.
func PlanReqIF(m *reqimport.ReqIFModel, fields []Field, opt ReqIFOptions, match Match) *ReqIFPlan {
	p := &ReqIFPlan{Model: m, RelTypes: map[string]string{}, Skipped: []string{}, Types: []ReqIFType{}, RelationTypes: []ReqIFRelType{},
		Kinds: map[string]string{}, TestKeys: map[string]string{}, Relations: []PlannedRelation{}}
	kindOf := map[string]string{} // type name → kind
	reqTypes, testTypes := map[string]bool{}, map[string]bool{}
	for _, t := range m.ObjectTypes {
		rt := ReqIFType{Name: t.Name, Objects: t.Objects, Guess: GuessObjectKind(t.Name)}
		if opt.TestFields == nil && rt.Guess == ObjTest {
			rt.Guess = ObjIgnore
		}
		rt.Kind = rt.Guess
		if v, ok := rawMappingFor(opt.ObjectTypes, t.Name); ok {
			rt.Kind = v
		}
		if rt.Kind == ObjTest && opt.TestFields == nil {
			rt.Kind = ObjIgnore
		}
		kindOf[t.Name] = rt.Kind
		switch rt.Kind {
		case ObjTest:
			testTypes[t.Name] = true
		case ObjRequirement, ObjHeading:
			reqTypes[t.Name] = true
		}
		p.Types = append(p.Types, rt)
	}
	var reqRole, testRole map[string]string
	p.Attributes, reqRole = attrsOf(m, fields, opt.Mapping, reqTypes, GuessRole)
	p.TestAttributes, testRole = attrsOf(m, opt.TestFields, opt.TestMapping, testTypes, GuessTestRole)
	relCount := map[string]int{}
	for _, r := range m.Relations {
		relCount[r.Type]++
	}
	relMap := map[string]string{}
	for _, t := range m.RelationTypes {
		rt := ReqIFRelType{Name: t, Relations: relCount[t], Guess: GuessRelationMapping(t)}
		rt.Type = rt.Guess
		if v, ok := rawMappingFor(opt.RelationTypes, t); ok {
			rt.Type = v
		}
		relMap[t] = rt.Type
		p.RelTypes[t] = rt.Type
		p.RelationTypes = append(p.RelationTypes, rt)
	}
	valueOf := func(o *reqimport.ReqIFObject, role map[string]string, want string) (string, string) {
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
	roleOf := func(o *reqimport.ReqIFObject) map[string]string {
		if kindOf[o.TypeName] == ObjTest {
			return testRole
		}
		return reqRole
	}
	// headings
	heading := map[string]string{} // object id → folder name
	byID := map[string]*reqimport.ReqIFObject{}
	for i := range m.Objects {
		o := &m.Objects[i]
		byID[o.Identifier] = o
		kind := kindOf[o.TypeName]
		if !opt.Hierarchy || kind == ObjIgnore {
			continue
		}
		role := roleOf(o)
		chapter, _ := valueOf(o, role, RoleHeading)
		text, _ := valueOf(o, role, "description")
		// a heading type the user chose is a folder whatever it holds; one
		// guessed from the type name only without text
		forced := false
		if v, ok := rawMappingFor(opt.ObjectTypes, o.TypeName); ok && v == ObjHeading {
			forced = true
		}
		if forced || (chapter == "" && kind == ObjHeading && text == "") || (chapter == "" && isHeadingType(o.TypeName) && text == "") {
			if chapter == "" {
				chapter, _ = valueOf(o, role, "title")
			}
			if chapter == "" {
				chapter = firstNonEmpty(o.LongName, text)
			}
		}
		if chapter != "" && (text == "" || forced) {
			heading[o.Identifier] = strings.ReplaceAll(strings.Join(strings.Fields(chapter), " "), "/", "-")
		}
	}
	// the kind of the items below each object (for folder counts and spec
	// prefixes)
	itemKind := func(o *reqimport.ReqIFObject) string {
		if _, h := heading[o.Identifier]; h {
			return ""
		}
		switch kindOf[o.TypeName] {
		case ObjTest:
			return ObjTest
		case ObjRequirement, ObjHeading:
			return ObjRequirement
		}
		return ""
	}
	specsOf := map[string]map[string]bool{} // kind → specs holding items
	folderKinds := map[string]map[string]bool{}
	for i := range m.Objects {
		o := &m.Objects[i]
		k := itemKind(o)
		if k == "" {
			continue
		}
		if specsOf[k] == nil {
			specsOf[k] = map[string]bool{}
		}
		specsOf[k][o.Spec] = true
		seen := map[string]bool{}
		for id := o.Parent; id != "" && !seen[id]; {
			seen[id] = true
			if _, ok := heading[id]; ok {
				if folderKinds[id] == nil {
					folderKinds[id] = map[string]bool{}
				}
				folderKinds[id][k] = true
			}
			po := byID[id]
			if po == nil {
				break
			}
			id = po.Parent
		}
	}
	for _, ks := range folderKinds {
		if ks[ObjRequirement] {
			p.Folders++
		}
		if ks[ObjTest] {
			p.TestFolders++
		}
	}
	folderOf := func(o *reqimport.ReqIFObject, kind string) string {
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
		if len(specsOf[kind]) > 1 && o.Spec != "" {
			parts = append([]string{strings.ReplaceAll(o.Spec, "/", "-")}, parts...)
		}
		root := opt.Root
		if kind == ObjTest {
			root = opt.TestRoot
		}
		if root != "" {
			parts = append(reqimport.FolderParts(root), parts...)
		}
		return strings.Join(parts, "/")
	}
	imported := map[string]bool{}
	for i := range m.Objects {
		o := &m.Objects[i]
		kind := kindOf[o.TypeName]
		if _, isHeading := heading[o.Identifier]; isHeading || kind == ObjIgnore {
			continue
		}
		if kind == ObjHeading {
			kind = ObjRequirement // a heading type with text (without the hierarchy)
		}
		flds, role, mt := fields, reqRole, match
		if kind == ObjTest {
			flds, role, mt = opt.TestFields, testRole, opt.MatchTest
		}
		in := Incoming{Row: o.Order + 1, Ref: o.Identifier, Cells: map[string]Cell{}}
		extID, _ := valueOf(o, role, RoleExternalID)
		extID = strings.Split(extID, "\n")[0]
		in.Location = "SPEC-OBJECT " + firstNonEmpty(extID, o.LongName, o.Identifier)
		for _, f := range flds {
			if f.ReadOnly || f.Type == Keys || f.Key == "folder" {
				continue
			}
			if v, refs := valueOf(o, role, f.Key); v != "" {
				in.Cells[f.Key] = Cell{Raw: v, Ref: refs}
			}
		}
		if opt.Hierarchy {
			in.Cells["folder"] = Cell{Raw: folderOf(o, kind), Ref: "SPECIFICATION"}
		} else if fv, refs := valueOf(o, role, "folder"); fv != "" {
			in.Cells["folder"] = Cell{Raw: fv, Ref: refs}
		}
		if kind == ObjTest {
			for name, r := range role {
				if r != RoleSteps {
					continue
				}
				for an, raw := range o.XHTML {
					if strings.ToLower(an) == name {
						for _, st := range reqimport.ParseStepsXHTML(raw) {
							in.Steps = append(in.Steps, StepRow{Action: st.Action, Data: st.Data, Expected: st.Expected, Call: st.Call, Ref: an})
						}
						in.HasSteps = true
					}
				}
				for an, v := range o.Values {
					if strings.ToLower(an) == name && o.XHTML[an] == "" {
						for _, l := range strings.Split(v, "\n") {
							if l = strings.TrimSpace(l); l != "" {
								in.Steps = append(in.Steps, StepRow{Action: l, Ref: an})
							}
						}
						in.HasSteps = true
					}
				}
			}
			if extID != "" {
				p.TestKeys[strings.ToLower(extID)] = o.Identifier
			}
		}
		if mt != nil {
			in.ID, in.BaseVersion = mt(o.Identifier)
		}
		// without a title attribute a new item gets one from its text; an
		// existing one keeps its title
		_, hasTitle := in.Cells["title"]
		_, hasText := in.Cells["description"]
		if !hasTitle && !hasText {
			if ch, _ := valueOf(o, role, RoleHeading); ch != "" {
				in.Cells["title"] = Cell{Raw: ch, Ref: "chapter name"}
				hasTitle = true
			} else if kind == ObjTest && o.LongName != "" {
				in.Cells["title"] = Cell{Raw: o.LongName, Ref: "LONG-NAME"}
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
		if v, _ := valueOf(o, role, RoleVersion); v != "" && in.ID != "" && m.Qtestframe && in.BaseVersion == 0 {
			in.BaseVersion, _ = strconv.Atoi(strings.TrimSpace(v))
		}
		if in.ID == "" && extID != "" && kind == ObjRequirement {
			// a new requirement records the other tool's id in its source
			src := in.Cells["source"].Raw
			if !strings.Contains(strings.ToLower(src), "id: "+strings.ToLower(extID)) {
				src = strings.TrimPrefix(strings.TrimSpace(src+", ID: "+extID), ", ")
			}
			in.Cells["source"] = Cell{Raw: src, Ref: "ID"}
		}
		if in.ID == "" && extID != "" {
			in.Key = extID
		}
		imported[o.Identifier] = true
		p.Kinds[o.Identifier] = kind
		if kind == ObjTest {
			p.TestRows = append(p.TestRows, in)
		} else {
			p.Rows = append(p.Rows, in)
		}
	}
	for _, r := range m.Relations {
		if !imported[r.Source] || !imported[r.Target] || r.Source == r.Target {
			continue
		}
		typ := relMap[r.Type]
		if typ == "" {
			typ = GuessRelationMapping(r.Type)
		}
		if typ == RelIgnore {
			continue
		}
		sk, tk := p.Kinds[r.Source], p.Kinds[r.Target]
		pr := PlannedRelation{Ref: r.Identifier, Type: typ, FileType: r.Type, Source: r.Source, Target: r.Target}
		switch {
		case sk == ObjTest && tk == ObjRequirement:
			pr.Type = RelVerifies
		case sk == ObjRequirement && tk == ObjTest:
			pr.Type, pr.Source, pr.Target = RelVerifies, r.Target, r.Source
		case sk == ObjTest || tk == ObjTest:
			p.Skipped = append(p.Skipped, fmt.Sprintf("SPEC-RELATION %s (%s): relations between tests are not imported", r.Identifier, r.Type))
			continue
		case typ == RelVerifies:
			p.Skipped = append(p.Skipped, fmt.Sprintf("SPEC-RELATION %s (%s): verifies needs a test and a requirement", r.Identifier, r.Type))
			continue
		}
		p.Relations = append(p.Relations, pr)
	}
	if opt.Refines {
		have := map[string]bool{} // the document's own refines relations
		for _, r := range p.Relations {
			if r.Type == "refines" {
				have[r.Source+"\x00"+r.Target] = true
			}
		}
		for i := range m.Objects {
			o := &m.Objects[i]
			if p.Kinds[o.Identifier] != ObjRequirement || o.Parent == "" || have[o.Identifier+"\x00"+o.Parent] {
				continue
			}
			if p.Kinds[o.Parent] == ObjRequirement {
				p.Relations = append(p.Relations, PlannedRelation{Ref: "hierarchy:" + o.Identifier + ">" + o.Parent, Type: "refines",
					Source: o.Identifier, Target: o.Parent})
			}
		}
	}
	return p
}

// titleFrom builds a title from the first words of a text.
func titleFrom(text string) string {
	first := strings.TrimSpace(text)
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.NewReplacer("**", "", "__", "", "`", "").Replace(first)
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
