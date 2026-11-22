// Package interop exchanges requirements and tests with other tools and
// with people editing them outside Qtestframe: the Excel round-trip
// (export a workbook, edit it, import it back with a diff preview) and the
// ReqIF import with re-import (IBM DOORS, DOORS Next, Siemens Polarion and
// Qtestframe's own ReqIF export). Both turn the file into Incoming rows; Diff
// compares them with the items as they are stored and the API applies the
// confirmed changes in one transaction through the regular create and
// update handlers.
//
// The fields are a registry (Field): the built-in fields of requirements
// and tests plus any Extension, so that custom fields take part in export,
// import, the ReqIF attribute mapping and the diff without changes here.
package interop

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
)

// Kinds.
const (
	KindRequirement = model.KindRequirement
	KindTest        = model.KindTest
)

// Type is the type of a field's value.
type Type string

// Field types. Keys holds item keys (relations and links), handled by the
// importer itself rather than through a PATCH.
const (
	Text     Type = "text"
	LongText Type = "longtext"
	Enum     Type = "enum"
	List     Type = "list"
	Int      Type = "int"
	Folder   Type = "folder"
	Keys     Type = "keys"
	Criteria Type = "criteria"
)

// Field is an exchangeable field of requirements or tests.
type Field struct {
	// Key identifies the field: "title", "rel:refines", "verifies", or an
	// extension's own key such as "cf:risk_class". It is stored in the
	// workbook, so it must stay stable.
	Key string `json:"key"`
	// Label is the column header (English); Aliases are other accepted
	// headers (Swedish …), matched case-insensitively.
	Label   string   `json:"label"`
	Aliases []string `json:"aliases,omitempty"`
	Type    Type     `json:"type"`
	// Values are the allowed codes of an Enum.
	Values []string `json:"values,omitempty"`
	// ReadOnly fields are exported for information and ignored on import.
	ReadOnly bool `json:"read_only"`
	// Relation is the relation type of a Keys field of requirements
	// ("refines" …); "" with Key "verifies" for the links of tests.
	Relation string `json:"relation,omitempty"`
	// Get returns the field's canonical value of an item.
	Get func(it *Item) string `json:"-"`
	// Parse turns a cell into the canonical value (nil: the default for
	// the type). The error message is shown at the cell.
	Parse func(raw string) (string, error) `json:"-"`
	// Put writes a canonical value into a create or PATCH body (nil: the
	// field cannot be written that way: Folder and Keys fields).
	Put func(body map[string]any, value string, it *Item) error `json:"-"`
}

// Item is a stored requirement or test with what the exchange needs
// beyond its own fields.
type Item struct {
	Req  *model.Requirement
	Test *model.TestCase
	// Folder is the folder path ("A/B", "" at the top level).
	Folder string
	// Relations: outgoing relation type → target keys (requirements).
	Relations map[string][]string
	// Links: the keys of the items at the other end of "verifies" links
	// (the requirements a test verifies; the tests verifying a requirement).
	Links []string
	// Extra holds values an Extension loaded for its fields (Get reads it).
	Extra map[string]string
}

// ID, Key and Version of the item.
func (it *Item) ID() string {
	if it.Req != nil {
		return it.Req.ID
	}
	return it.Test.ID
}

// Key returns the item key.
func (it *Item) Key() string {
	if it.Req != nil {
		return it.Req.Key
	}
	return it.Test.Key
}

// Version returns the stored version.
func (it *Item) Version() int {
	if it.Req != nil {
		return it.Req.Version
	}
	return it.Test.Version
}

// Title returns the title.
func (it *Item) Title() string {
	if it.Req != nil {
		return it.Req.Title
	}
	return it.Test.Title
}

// Extension adds fields of a project (custom fields): it is called with
// the kind of the items and returns the fields to append. Get should read
// Item.Extra, which Load fills, and Put writes the create / PATCH body.
type Extension struct {
	Name   string
	Fields func(ctx context.Context, tenantID, projectID, kind string) ([]Field, error)
	// Load fills Item.Extra for the items of a project (may be nil).
	Load func(ctx context.Context, tenantID, projectID, kind string, items []*Item) error
}

var (
	extMu      sync.RWMutex
	extensions []Extension
)

// Register adds an extension (typically from an init function or when
// the server starts).
func Register(e Extension) {
	extMu.Lock()
	defer extMu.Unlock()
	extensions = append(extensions, e)
}

// Extensions returns the registered extensions.
func Extensions() []Extension {
	extMu.RLock()
	defer extMu.RUnlock()
	return append([]Extension(nil), extensions...)
}

// FieldsFor returns the fields of kind in a project: the built-in fields
// followed by those of the extensions.
func FieldsFor(ctx context.Context, tenantID, projectID, kind string) ([]Field, error) {
	fs := BuiltinFields(kind)
	for _, e := range Extensions() {
		if e.Fields == nil {
			continue
		}
		more, err := e.Fields(ctx, tenantID, projectID, kind)
		if err != nil {
			return nil, fmt.Errorf("fields of %s: %w", e.Name, err)
		}
		fs = append(fs, more...)
	}
	return fs, nil
}

// LoadExtras lets the extensions fill Item.Extra.
func LoadExtras(ctx context.Context, tenantID, projectID, kind string, items []*Item) error {
	for _, e := range Extensions() {
		if e.Load == nil {
			continue
		}
		if err := e.Load(ctx, tenantID, projectID, kind, items); err != nil {
			return fmt.Errorf("values of %s: %w", e.Name, err)
		}
	}
	return nil
}

// BuiltinFields returns the built-in fields of kind, in column order.
func BuiltinFields(kind string) []Field {
	if kind == KindTest {
		return testFields()
	}
	return requirementFields()
}

func req(f func(r *model.Requirement) string) func(*Item) string {
	return func(it *Item) string {
		if it.Req == nil {
			return ""
		}
		return f(it.Req)
	}
}

func tst(f func(t *model.TestCase) string) func(*Item) string {
	return func(it *Item) string {
		if it.Test == nil {
			return ""
		}
		return f(it.Test)
	}
}

func put(key string) func(map[string]any, string, *Item) error {
	return func(b map[string]any, v string, _ *Item) error {
		b[key] = v
		return nil
	}
}

func putList(key string) func(map[string]any, string, *Item) error {
	return func(b map[string]any, v string, _ *Item) error {
		b[key] = SplitList(v)
		return nil
	}
}

func enumParser(codes []string, parse func(string) (string, bool)) func(string) (string, error) {
	return func(raw string) (string, error) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return "", nil
		}
		if v, ok := parse(raw); ok && v != "" {
			return v, nil
		}
		if v := strings.ReplaceAll(strings.ToLower(raw), " ", "_"); model.Valid(codes, v) {
			return v, nil
		}
		return "", fmt.Errorf("unknown value %q (use one of %s)", raw, strings.Join(codes, ", "))
	}
}

func codeParser(codes []string) func(string) (string, error) {
	return enumParser(codes, func(s string) (string, bool) { return "", false })
}

// Relation field labels, as in the requirement editor.
var relationLabels = map[string]string{
	"refines": "Refines", "depends_on": "Depends on", "provides": "Provides", "derived_from": "Derived from",
	"conflicts_with": "Conflicts with", "duplicates": "Duplicates", "relates_to": "Relates to",
}

var relationAliases = map[string][]string{
	"refines": {"Förfinar"}, "depends_on": {"Beror på"}, "provides": {"Tillhandahåller"}, "derived_from": {"Härledd från"},
	"conflicts_with": {"Står i konflikt med"}, "duplicates": {"Dubblerar"}, "relates_to": {"Relaterar till"},
}

func requirementFields() []Field {
	fs := []Field{
		{Key: "folder", Label: "Folder", Aliases: []string{"Mapp", "Folder path"}, Type: Folder, Get: func(it *Item) string { return it.Folder }},
		{Key: "title", Label: "Title", Aliases: []string{"Titel"}, Type: Text, Get: req(func(r *model.Requirement) string { return r.Title }), Put: put("title")},
		{Key: "description", Label: "Description", Aliases: []string{"Beskrivning", "Text"}, Type: LongText,
			Get: req(func(r *model.Requirement) string { return r.Description }), Put: put("description")},
		{Key: "obligation", Label: "Obligation", Aliases: []string{"Kravnivå"}, Type: Enum, Values: model.Obligations,
			Get: req(func(r *model.Requirement) string { return r.Obligation }), Parse: enumParser(model.Obligations, reqimport.ParseObligation), Put: put("obligation")},
		{Key: "type", Label: "Type", Aliases: []string{"Typ"}, Type: Enum, Values: model.RequirementTypes,
			Get: req(func(r *model.Requirement) string { return r.Type }), Parse: enumParser(model.RequirementTypes, reqimport.ParseType), Put: put("type")},
		{Key: "priority", Label: "Priority", Aliases: []string{"Prioritet"}, Type: Enum, Values: model.Priorities,
			Get: req(func(r *model.Requirement) string { return r.Priority }), Parse: enumParser(model.Priorities, reqimport.ParsePriority), Put: put("priority")},
		{Key: "status", Label: "Status", Type: Enum, Values: model.RequirementStatuses,
			Get: req(func(r *model.Requirement) string { return r.Status }), Parse: enumParser(model.RequirementStatuses, reqimport.ParseStatus), Put: put("status")},
		{Key: "source", Label: "Source", Aliases: []string{"Källa"}, Type: Text, Get: req(func(r *model.Requirement) string { return r.Source }), Put: put("source")},
		{Key: "rationale", Label: "Rationale", Aliases: []string{"Motivering"}, Type: LongText,
			Get: req(func(r *model.Requirement) string { return r.Rationale }), Put: put("rationale")},
		{Key: "owner", Label: "Owner", Aliases: []string{"Ägare"}, Type: Text, Get: req(func(r *model.Requirement) string { return r.Owner }), Put: put("owner")},
		{Key: "tags", Label: "Tags", Aliases: []string{"Taggar"}, Type: List, Get: req(func(r *model.Requirement) string { return JoinList(r.Tags) }), Put: putList("tags")},
		{Key: "criteria", Label: "Acceptance criteria", Aliases: []string{"Acceptanskriterier", "Criteria"}, Type: Criteria,
			Get: req(func(r *model.Requirement) string { return CriteriaText(r.Criteria) }), Parse: parseCriteriaCell, Put: putCriteria},
	}
	for _, rt := range model.RelationTypes {
		rt := rt
		fs = append(fs, Field{Key: "rel:" + rt, Label: relationLabels[rt], Aliases: relationAliases[rt], Type: Keys, Relation: rt,
			Get: func(it *Item) string { return JoinList(sortedCopy(it.Relations[rt])) }})
	}
	fs = append(fs, Field{Key: "verified_by", Label: "Verified by", Aliases: []string{"Verifieras av", "Linked tests"}, Type: Keys, ReadOnly: true,
		Get: func(it *Item) string { return JoinList(sortedCopy(it.Links)) }})
	return fs
}

func testFields() []Field {
	execTypes := []string{model.ExecManual, model.ExecAutomated}
	return []Field{
		{Key: "folder", Label: "Folder", Aliases: []string{"Mapp", "Folder path"}, Type: Folder, Get: func(it *Item) string { return it.Folder }},
		{Key: "title", Label: "Title", Aliases: []string{"Titel"}, Type: Text, Get: tst(func(t *model.TestCase) string { return t.Title }), Put: put("title")},
		{Key: "description", Label: "Description", Aliases: []string{"Beskrivning"}, Type: LongText,
			Get: tst(func(t *model.TestCase) string { return t.Description }), Put: put("description")},
		{Key: "preconditions", Label: "Preconditions", Aliases: []string{"Förutsättningar"}, Type: LongText,
			Get: tst(func(t *model.TestCase) string { return t.Preconditions }), Put: put("preconditions")},
		{Key: "expected_result", Label: "Expected result", Aliases: []string{"Förväntat resultat"}, Type: LongText,
			Get: tst(func(t *model.TestCase) string { return t.ExpectedResult }), Put: put("expected_result")},
		{Key: "execution_type", Label: "Execution type", Aliases: []string{"Exekveringstyp"}, Type: Enum, Values: execTypes,
			Get: tst(func(t *model.TestCase) string { return t.ExecutionType }), Parse: codeParser(execTypes), Put: put("execution_type")},
		{Key: "priority", Label: "Priority", Aliases: []string{"Prioritet"}, Type: Enum, Values: model.Priorities,
			Get: tst(func(t *model.TestCase) string { return t.Priority }), Parse: enumParser(model.Priorities, reqimport.ParsePriority), Put: put("priority")},
		{Key: "status", Label: "Status", Type: Enum, Values: model.TestStatuses,
			Get: tst(func(t *model.TestCase) string { return t.Status }), Parse: codeParser(model.TestStatuses), Put: put("status")},
		{Key: "estimated_minutes", Label: "Estimated minutes", Aliases: []string{"Uppskattade minuter"}, Type: Int,
			Get: tst(func(t *model.TestCase) string { return intText(t.EstimatedMin) }), Put: func(b map[string]any, v string, _ *Item) error {
				n, _ := strconv.Atoi(v)
				b["estimated_minutes"] = n
				return nil
			}},
		{Key: "tags", Label: "Tags", Aliases: []string{"Taggar"}, Type: List, Get: tst(func(t *model.TestCase) string { return JoinList(t.Tags) }), Put: putList("tags")},
		{Key: "verifies", Label: "Verifies", Aliases: []string{"Verifierar", "Linked requirements"}, Type: Keys,
			Get: func(it *Item) string { return JoinList(sortedCopy(it.Links)) }},
	}
}

func intText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// ---------------------------------------------------------------- values

// SplitList splits a list cell on , ; | and line breaks.
func SplitList(s string) []string {
	out := []string{}
	for _, t := range reqimport.SplitTags(s) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// JoinList joins list values as written to a cell.
func JoinList(vs []string) string { return strings.Join(vs, ", ") }

func sortedCopy(vs []string) []string {
	out := append([]string(nil), vs...)
	sort.Strings(out)
	return out
}

// CriteriaText renders criteria one per line: "method: description
// (threshold: x)", the form ParseCriteria reads.
func CriteriaText(cs []model.EvaluationCriterion) string {
	var ls []string
	for _, c := range cs {
		l := strings.Join(strings.Fields(c.Description), " ")
		if c.Method != "" {
			l = c.Method + ": " + l
		}
		if t := strings.TrimSpace(c.Threshold); t != "" {
			l += " (threshold: " + t + ")"
		}
		ls = append(ls, l)
	}
	return strings.Join(ls, "\n")
}

func parseCriteriaCell(raw string) (string, error) {
	var cs []model.EvaluationCriterion
	for _, c := range reqimport.ParseCriteria(raw) {
		cs = append(cs, model.EvaluationCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold})
	}
	return CriteriaText(cs), nil
}

// putCriteria writes the criteria, keeping the ids of the criteria at the
// same position so that links to them survive an edit.
func putCriteria(b map[string]any, v string, it *Item) error {
	out := []model.EvaluationCriterion{}
	for i, c := range reqimport.ParseCriteria(v) {
		cr := model.EvaluationCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold}
		if it != nil && it.Req != nil && i < len(it.Req.Criteria) {
			cr.ID = it.Req.Criteria[i].ID
		}
		out = append(out, cr)
	}
	b["criteria"] = out
	return nil
}

// errEmpty marks an empty required value.
var errEmpty = errors.New("a value is required")

// parseValue turns a cell into the canonical value of f.
func parseValue(f *Field, raw string) (string, error) {
	raw = normText(raw)
	if f.Parse != nil {
		return f.Parse(raw)
	}
	switch f.Type {
	case List, Keys:
		return JoinList(SplitList(raw)), nil
	case Int:
		if raw == "" {
			return "", nil
		}
		s := strings.TrimSuffix(raw, ".0")
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return "", fmt.Errorf("%q is not a whole number", raw)
		}
		return intText(n), nil
	case Folder:
		return strings.Join(reqimport.FolderParts(raw), "/"), nil
	}
	return raw, nil
}

// normText makes cell text comparable: line breaks as \n, trimmed, and the
// formula guard of exports removed.
func normText(s string) string { return reqimport.CleanCell(s) }

// equalValues compares canonical values of f.
func equalValues(f *Field, a, b string) bool {
	switch f.Type {
	case List, Keys:
		x, y := sortedCopy(SplitList(a)), sortedCopy(SplitList(b))
		return strings.Join(x, "\x00") == strings.Join(y, "\x00")
	case Folder:
		return strings.EqualFold(strings.Join(reqimport.FolderParts(a), "/"), strings.Join(reqimport.FolderParts(b), "/"))
	}
	return normText(a) == normText(b)
}

// FieldByHeader finds the field named by a column header (key, label or
// alias, case-insensitive).
func FieldByHeader(fs []Field, header string) *Field {
	h := strings.ToLower(strings.Join(strings.Fields(header), " "))
	for i := range fs {
		if strings.ToLower(fs[i].Key) == h || strings.ToLower(fs[i].Label) == h {
			return &fs[i]
		}
	}
	for i := range fs {
		for _, a := range fs[i].Aliases {
			if strings.ToLower(a) == h {
				return &fs[i]
			}
		}
	}
	return nil
}

// FieldByKey finds a field by key.
func FieldByKey(fs []Field, key string) *Field {
	for i := range fs {
		if fs[i].Key == key {
			return &fs[i]
		}
	}
	return nil
}
