// Package tracerules evaluates the traceability rules of a project (see
// model.TraceRule) on its requirements, tests, trace links, relations and
// item links. The caller loads the project once into a Graph; Evaluate
// then indexes the links and relations once and checks every rule.
//
// Besides the configurable trace kinds there are built-in consistency
// checks (model.TraceBuiltinKinds, builtin.go) with a fixed condition, such
// as conflicting requirements in one release or a test that verifies an
// obsolete requirement. Their predicates go through Hooks so that other
// features (verification records, staleness, waivers) can refine them.
package tracerules

import (
	"fmt"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

// Item is a requirement or test of the project (or, in the built-in
// checks, a risk or release).
type Item struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	// Status of the item where a check needs it ("" unknown; "deleted"
	// for an item in the trash).
	Status string `json:"status,omitempty"`
}

// Graph is the project content the rules are evaluated on.
type Graph struct {
	Requirements []Item
	Tests        []Item
	Links        []model.TraceLink
	Relations    []model.Relation
	ItemLinks    []model.ItemLink
	// LastResults: test id → last result.
	LastResults map[string]string
	// Record returns the query record of an item (requirement or test).
	Record func(kind, id string) query.Record
	// Schemas: item kind → query schema (nil accepts any field).
	Schemas map[string]*query.Schema

	// The facts below are only used by the built-in consistency checks
	// (nil: the check finds nothing in them).
	Facts
	// Hooks refine the predicates of the built-in checks (nil fields:
	// DefaultHooks).
	Hooks Hooks

	linksByReq map[string][]model.TraceLink
	sims       map[string]*SimModel
}

// Violation is an item that breaks a rule.
type Violation struct {
	Item
	// Found is the number of qualifying traces, Required the rule's minimum.
	Found    int `json:"found"`
	Required int `json:"required"`
	// Total is the number of traces of the kind (of the relation type,
	// direction and target type) before the conditions (not suspect,
	// passed, target query) were applied.
	Total int `json:"total"`
	// Built-in checks: Check is the reason code (e.g. obsolete, deleted,
	// failing, cycle; see builtin.go), Note a detail (the similarity, the
	// catalogue id), Related the other items involved and Fixes the
	// actions that resolve the violation (FixActions).
	Check   string    `json:"check,omitempty"`
	Note    string    `json:"note,omitempty"`
	Related []Related `json:"related,omitempty"`
	Fixes   []string  `json:"fixes,omitempty"`
}

// Related is another item involved in a violation of a built-in check.
type Related struct {
	Item
	// Role: how it is involved (conflicts_with, depends_on, parent,
	// verifies, prerequisite, cycle, release, mitigation, duplicate).
	Role string `json:"role"`
	// Link is the id of the relation, trace link or test prerequisite
	// that connects it to the violating item ("" none or deleted).
	Link string `json:"link,omitempty"`
	// Check is the reason code for this item (as Violation.Check).
	Check string `json:"check,omitempty"`
	Note  string `json:"note,omitempty"`
}

// Result is the evaluation of one rule.
type Result struct {
	Rule model.TraceRule `json:"rule"`
	// Matched is the number of items the selector matched (in scope).
	Matched        int         `json:"matched"`
	Violations     []Violation `json:"violations"`
	ViolationCount int         `json:"violation_count"`
	Passed         bool        `json:"passed"`
	// Error is set when a query of the rule is invalid (the rule fails).
	Error string `json:"error,omitempty"`
	// Waived is the number of violations a waiver accepts (Hooks.Waived);
	// they are not listed and do not fail the rule.
	Waived int `json:"waived,omitempty"`
}

// Scope restricts the items checked (nil: every item).
type Scope func(kind, id string) bool

type index struct {
	g        *Graph
	reqLinks map[string][]model.TraceLink
	tstLinks map[string][]model.TraceLink
	relOut   map[string][]model.Relation
	relIn    map[string][]model.Relation
	linkOut  map[string][]model.ItemLink
	linkIn   map[string][]model.ItemLink
	records  map[string]query.Record
	matches  map[string]bool
}

func ref(kind, id string) string { return kind + ":" + id }

func newIndex(g *Graph) *index {
	x := &index{g: g, reqLinks: map[string][]model.TraceLink{}, tstLinks: map[string][]model.TraceLink{},
		relOut: map[string][]model.Relation{}, relIn: map[string][]model.Relation{},
		linkOut: map[string][]model.ItemLink{}, linkIn: map[string][]model.ItemLink{},
		records: map[string]query.Record{}, matches: map[string]bool{}}
	for _, l := range g.Links {
		x.reqLinks[l.RequirementID] = append(x.reqLinks[l.RequirementID], l)
		x.tstLinks[l.TestCaseID] = append(x.tstLinks[l.TestCaseID], l)
	}
	for _, r := range g.Relations {
		x.relOut[r.SourceID] = append(x.relOut[r.SourceID], r)
		x.relIn[r.TargetID] = append(x.relIn[r.TargetID], r)
	}
	for _, l := range g.ItemLinks {
		x.linkOut[ref(l.SourceType, l.SourceID)] = append(x.linkOut[ref(l.SourceType, l.SourceID)], l)
		if l.TargetID != nil {
			x.linkIn[ref(l.TargetType, *l.TargetID)] = append(x.linkIn[ref(l.TargetType, *l.TargetID)], l)
		}
	}
	return x
}

func (x *index) record(kind, id string) query.Record {
	k := ref(kind, id)
	if r, ok := x.records[k]; ok {
		return r
	}
	var r query.Record = query.MapRecord{}
	if x.g.Record != nil {
		if rec := x.g.Record(kind, id); rec != nil {
			r = rec
		}
	}
	x.records[k] = r
	return r
}

// match reports whether item kind/id matches q (nil: always). Results are
// cached per query and item.
func (x *index) match(q *query.Query, qid, kind, id string) bool {
	if q == nil {
		return true
	}
	k := qid + "|" + ref(kind, id)
	if m, ok := x.matches[k]; ok {
		return m
	}
	m := q.Match(x.record(kind, id))
	x.matches[k] = m
	return m
}

func (x *index) parse(expr, kind string) (*query.Query, error) {
	if expr == "" {
		return nil, nil
	}
	return query.Parse(expr, x.g.Schemas[kind])
}

// Evaluate checks rules (disabled ones too: filter before calling) on the
// items in scope. Violations keep the order of g.Requirements / g.Tests.
func Evaluate(rules []model.TraceRule, g *Graph, scope Scope) []Result {
	x := newIndex(g)
	out := make([]Result, 0, len(rules))
	for i, r := range rules {
		var res Result
		if model.IsBuiltinCheck(r.Kind) {
			res = x.evaluateBuiltin(i, r, scope)
		} else {
			res = x.evaluate(i, r, scope)
		}
		x.waive(&res)
		out = append(out, res)
	}
	return out
}

// waive drops the violations a waiver accepts.
func (x *index) waive(res *Result) {
	waived := x.hooks().Waived
	if waived == nil || len(res.Violations) == 0 {
		return
	}
	kept := res.Violations[:0]
	for _, v := range res.Violations {
		if waived(x.g, res.Rule, v) {
			res.Waived++
			continue
		}
		kept = append(kept, v)
	}
	res.Violations = kept
	res.ViolationCount = len(kept)
	res.Passed = res.Error == "" && res.ViolationCount == 0
}

func (x *index) evaluate(n int, r model.TraceRule, scope Scope) Result {
	res := Result{Rule: r, Violations: []Violation{}}
	sel, err := x.parse(r.Selector, r.Scope)
	if err != nil {
		res.Error = fmt.Sprintf("selector: %v", err)
		return res
	}
	otherKind := model.KindRequirement
	switch {
	case r.Kind == model.TraceVerifiedBy:
		otherKind = model.KindTest
	case r.Kind == model.TraceItemLink:
		otherKind = r.TargetType
	}
	var target *query.Query
	if r.TargetQuery != "" {
		if otherKind != model.KindRequirement && otherKind != model.KindTest {
			res.Error = "target_query needs target_type requirement or test"
			return res
		}
		if target, err = x.parse(r.TargetQuery, otherKind); err != nil {
			res.Error = fmt.Sprintf("target query: %v", err)
			return res
		}
	}
	need := max(r.Min, 0)
	items := x.g.Requirements
	if r.Scope == model.KindTest {
		items = x.g.Tests
	}
	selID, targetID := fmt.Sprintf("s%d", n), fmt.Sprintf("t%d", n)
	for _, it := range items {
		if scope != nil && !scope(it.Kind, it.ID) {
			continue
		}
		if !x.match(sel, selID, it.Kind, it.ID) {
			continue
		}
		res.Matched++
		found, total := x.count(r, it, target, targetID)
		if found < need || (r.Every && found < total) {
			res.Violations = append(res.Violations, Violation{Item: it, Found: found, Required: need, Total: total})
		}
	}
	res.ViolationCount = len(res.Violations)
	res.Passed = res.ViolationCount == 0
	return res
}

// count returns the qualifying traces of it and all traces of the rule's kind.
func (x *index) count(r model.TraceRule, it Item, target *query.Query, tid string) (found, total int) {
	other := func(kind, id string) bool { return x.match(target, tid, kind, id) }
	switch r.Kind {
	case model.TraceVerifiedBy:
		for _, l := range x.reqLinks[it.ID] {
			total++
			if r.NotSuspect && l.SuspectSince != nil {
				continue
			}
			if r.Passed && x.g.LastResults[l.TestCaseID] != model.ResultPassed {
				continue
			}
			if other(model.KindTest, l.TestCaseID) {
				found++
			}
		}
	case model.TraceVerifies:
		for _, l := range x.tstLinks[it.ID] {
			total++
			if r.NotSuspect && l.SuspectSince != nil {
				continue
			}
			if other(model.KindRequirement, l.RequirementID) {
				found++
			}
		}
	case model.TraceRelation:
		check := func(rels []model.Relation, outgoing bool) {
			for _, rel := range rels {
				if r.RelationType != "" && rel.Type != r.RelationType {
					continue
				}
				total++
				end := rel.TargetID
				if !outgoing {
					end = rel.SourceID
				}
				if other(model.KindRequirement, end) {
					found++
				}
			}
		}
		if r.Direction != "incoming" {
			check(x.relOut[it.ID], true)
		}
		if r.Direction != "outgoing" {
			check(x.relIn[it.ID], false)
		}
	case model.TraceItemLink:
		check := func(links []model.ItemLink, outgoing bool) {
			for _, l := range links {
				typ, id := l.TargetType, ""
				if l.TargetID != nil {
					id = *l.TargetID
				}
				if !outgoing {
					typ, id = l.SourceType, l.SourceID
				}
				if r.TargetType != "" && typ != r.TargetType {
					continue
				}
				total++
				if target != nil && !other(typ, id) {
					continue
				}
				found++
			}
		}
		k := ref(it.Kind, it.ID)
		if r.Direction != "incoming" {
			check(x.linkOut[k], true)
		}
		if r.Direction != "outgoing" {
			check(x.linkIn[k], false)
		}
	}
	return found, total
}

// ItemViolation is a rule an item breaks.
type ItemViolation struct {
	Rule model.TraceRule `json:"rule"`
	Violation
}

// ForItem returns the rules results break for one item.
func ForItem(results []Result, kind, id string) []ItemViolation {
	out := []ItemViolation{}
	for _, res := range results {
		for _, v := range res.Violations {
			if v.Kind == kind && v.ID == id {
				out = append(out, ItemViolation{Rule: res.Rule, Violation: v})
			}
		}
	}
	return out
}

// Failing reports whether a result counts against a severity level:
// "warning" counts errors and warnings, anything else errors only.
func Failing(res Result, level string) bool {
	if res.Passed {
		return false
	}
	return res.Rule.Severity == "error" || level == "warning"
}
