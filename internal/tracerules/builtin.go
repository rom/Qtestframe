package tracerules

import (
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/similarity"
)

// Built-in consistency checks (model.TraceBuiltinKinds). Each one finds
// items whose state contradicts their relations, links or scope; the
// violations name the other items involved (Related) and the actions that
// resolve them (Fixes).

// Reason codes of the built-in checks (Violation.Check, Related.Check).
const (
	ReasonConflict     = "conflict"      // conflict_in_release
	ReasonRejected     = "rejected"      // the other requirement is rejected
	ReasonObsolete     = "obsolete"      // the other requirement is obsolete
	ReasonDeleted      = "deleted"       // the other item is in the trash
	ReasonFailing      = "failing"       // the prerequisite's latest result is failing
	ReasonCycle        = "cycle"         // prerequisites form a cycle
	ReasonNoEvidence   = "no_evidence"   // unevidenced_must
	ReasonNoMitigation = "no_mitigation" // the risk has no mitigation requirement
	ReasonNotVerified  = "not_verified"  // none of the risk's mitigations is verified
	ReasonUnjustified  = "unjustified"   // unjustified_rejection
	ReasonSimilar      = "similar"       // duplicate_suspect
)

// Fix actions (Violation.Fixes). The client performs them with the
// ordinary endpoints, using the ids in Related.
const (
	// FixRemoveRelation deletes the relation Related.Link.
	FixRemoveRelation = "remove_relation"
	// FixRemoveLink deletes the trace link Related.Link.
	FixRemoveLink = "remove_link"
	// FixRemoveDependency deletes the test prerequisite Related.Link.
	FixRemoveDependency = "remove_dependency"
	// FixRemoveFromRelease takes the violating requirement out of the
	// release (a Related item of kind release).
	FixRemoveFromRelease = "remove_from_release"
	// FixSetObsolete sets the violating requirement's status to obsolete.
	FixSetObsolete = "set_obsolete"
	// FixAddJustification opens the requirement to record why it is
	// rejected (a comment).
	FixAddJustification = "add_justification"
	// FixLinkMitigation opens the risk to link mitigation requirements.
	FixLinkMitigation = "link_mitigation"
	// FixMarkDuplicate adds the relation "violating item duplicates the
	// related item".
	FixMarkDuplicate = "mark_duplicate"
	// FixDismiss records that the finding about the related item is not
	// a problem ("not a duplicate", "acknowledge a deleted item").
	FixDismiss = "dismiss"
)

// FixActions lists every fix action.
var FixActions = []string{FixRemoveRelation, FixRemoveLink, FixRemoveDependency, FixRemoveFromRelease, FixSetObsolete,
	FixAddJustification, FixLinkMitigation, FixMarkDuplicate, FixMarkConflict, FixDismiss}

// Builtin is the project default of a built-in check: what installing the
// built-in checks into a project creates.
type Builtin struct {
	Kind        string
	Name        string
	Description string
	Severity    string
	Enabled     bool
}

// Builtins are the defaults of the built-in checks, in display order.
// They start as warnings so that a project's release readiness only
// changes when a team raises a check to an error.
var Builtins = []Builtin{
	{model.CheckConflictInRelease, "Conflicting requirements in a release", "Two requirements related by conflicts_with are both approved (or later) in the same release.", "warning", true},
	{model.CheckUnmetDependency, "Approved requirement with an unmet dependency", "An approved requirement depends on a requirement that is rejected, obsolete or deleted.", "warning", true},
	{model.CheckObsoleteParent, "Active requirement with an obsolete parent", "A requirement that refines or is derived from an obsolete requirement is still active.", "warning", true},
	{model.CheckVerifiesInactive, "Test verifies an inactive requirement", "A test verifies a requirement that is rejected or obsolete.", "warning", true},
	{model.CheckBrokenPrerequisite, "Broken test prerequisite", "A test prerequisite is deleted or its latest result is failing, or prerequisites form a cycle.", "warning", true},
	{model.CheckUnevidencedMust, "MUST requirement in a release without passing evidence", "A MUST or SHALL requirement in a planned or active release has no passing evidence.", "warning", true},
	{model.CheckUnmitigatedRisk, "High risk without verified mitigation", "A high or critical risk has no linked mitigation requirement, or none of its mitigations is verified.", "warning", true},
	{model.CheckUnjustifiedRejection, "Regulatory requirement rejected without justification", "A requirement adopted from a regulation of the catalogue is rejected without a recorded justification.", "warning", true},
	{model.CheckDuplicateSuspect, "Suspected duplicate requirements", "Requirements that say the same thing (similar text, synonyms included) and have no duplicates relation.", "warning", true},
	{model.CheckPotentialConflict, "Potentially conflicting requirements", "Requirements that likely contradict each other (opposite obligations, thresholds that cannot both hold, mutually exclusive values) and have no conflicts_with relation.", "warning", true},
	{model.CheckTestDuplicateSuspect, "Suspected duplicate tests", "Tests that say the same thing and are not linked as duplicates.", "warning", true},
}

// BuiltinRule returns the project default of a built-in check as a rule.
func BuiltinRule(b Builtin, projectID string) model.TraceRule {
	return model.TraceRule{ProjectID: projectID, Name: b.Name, Description: b.Description, Severity: b.Severity, Enabled: b.Enabled,
		Kind: b.Kind, Scope: model.BuiltinScope(b.Kind), Min: 1}
}

// Release is a release of the project with its scope.
type Release struct {
	ID, Name, Status string
	RequirementIDs   []string
}

// Risk is a risk of the project with its mitigation requirements.
type Risk struct {
	ID, Key, Title, Status string
	Likelihood, Impact     int
	RequirementIDs         []string
}

// ReqFacts are the fields of a requirement the built-in checks look at.
type ReqFacts struct {
	Status, Obligation, CatalogRef string
	// Text is the title and description (duplicate check).
	Text    string
	Created time.Time
}

// Facts is the part of a Graph only the built-in checks use.
type Facts struct {
	// Reqs: requirement id → facts.
	Reqs map[string]ReqFacts
	// TestStatus: test id → status.
	TestStatus   map[string]string
	Dependencies []model.TestDependency
	Releases     []Release
	Risks        []Risk
	// Deleted: items in the trash by id; DeletedRelations and
	// DeletedDependencies the references to them that the deletion removed
	// from items that still exist.
	Deleted             map[string]Item
	DeletedRelations    []model.Relation
	DeletedDependencies []model.TestDependency
	// Dismissed findings: keys made by DismissKey.
	Dismissed map[string]bool
	// FromRegulation: requirement ids adopted from a regulation (a
	// catalogue source of kind law or directive).
	FromRegulation map[string]bool
	// Justified: requirement ids whose rejection is justified.
	Justified map[string]bool
	// Docs: requirement and test id → its text for the similarity checks
	// (a requirement without one: its Text as the description).
	Docs map[string]similarity.Doc
	// TestCreated: test id → creation time (test_duplicate_suspect reports
	// the newer test).
	TestCreated map[string]time.Time
}

// DismissKey is the key of a dismissed finding of check about items a, b.
func DismissKey(check, a, b string) string {
	if b < a {
		a, b = b, a
	}
	return check + "|" + a + "|" + b
}

// Hooks are the predicates of the built-in checks. A nil field uses the
// one of DefaultHooks; callers (and other features) replace them to refine
// what counts as verified, failing, waived or justified without changing
// the checks.
type Hooks struct {
	// Verified reports whether a requirement has passing evidence
	// (unevidenced_must, and the mitigations of unmitigated_risk).
	Verified func(g *Graph, reqID string) bool
	// TestFailing reports whether a test's latest result counts as
	// failing (broken_prerequisite).
	TestFailing func(g *Graph, testID string) bool
	// Waived reports whether a violation of a rule is accepted by a
	// waiver; waived violations are not listed and do not fail the rule.
	// It applies to every rule kind.
	Waived func(g *Graph, rule model.TraceRule, v Violation) bool
	// Justified reports whether a rejected requirement has a recorded
	// justification (unjustified_rejection).
	Justified func(g *Graph, reqID string) bool
	// FromRegulation reports whether a requirement was adopted from a
	// regulation of the catalogue (unjustified_rejection).
	FromRegulation func(g *Graph, reqID string) bool
	// Similarity of two texts, 0–1: when set, duplicate_suspect compares
	// the Text of every pair of requirements with it instead of the
	// similarity model.
	Similarity func(a, b string) float64
	// Scorer replaces the similarity model of the active items of a kind
	// (m.Index, TF-IDF) for the duplicate checks, e.g. with embeddings
	// (nil result: keep m.Index).
	Scorer func(g *Graph, kind string, m *SimModel) similarity.Scorer
}

// DefaultHooks are today's predicates.
var DefaultHooks = Hooks{
	Verified:       DefaultVerified,
	TestFailing:    DefaultTestFailing,
	Waived:         nil, // nothing is waived
	Justified:      func(g *Graph, id string) bool { return g.Justified[id] },
	FromRegulation: func(g *Graph, id string) bool { return g.FromRegulation[id] },
	Similarity:     Similarity,
}

// DefaultVerified: a requirement is verified when a linked test that is
// not suspect passed its latest execution.
func DefaultVerified(g *Graph, reqID string) bool {
	for _, l := range g.LinksOf(reqID) {
		if l.SuspectSince == nil && g.LastResults[l.TestCaseID] == model.ResultPassed {
			return true
		}
	}
	return false
}

// DefaultTestFailing: the latest result of the test is failed.
func DefaultTestFailing(g *Graph, testID string) bool {
	return g.LastResults[testID] == model.ResultFailed
}

// LinksOf returns the trace links of a requirement.
func (g *Graph) LinksOf(reqID string) []model.TraceLink {
	if g.linksByReq == nil {
		g.linksByReq = map[string][]model.TraceLink{}
		for _, l := range g.Links {
			g.linksByReq[l.RequirementID] = append(g.linksByReq[l.RequirementID], l)
		}
	}
	return g.linksByReq[reqID]
}

func (x *index) hooks() Hooks {
	h := x.g.Hooks
	d := DefaultHooks
	if h.Verified == nil {
		h.Verified = d.Verified
	}
	if h.TestFailing == nil {
		h.TestFailing = d.TestFailing
	}
	if h.Waived == nil {
		h.Waived = d.Waived
	}
	if h.Justified == nil {
		h.Justified = d.Justified
	}
	if h.FromRegulation == nil {
		h.FromRegulation = d.FromRegulation
	}
	if h.Similarity == nil {
		h.Similarity = d.Similarity
	}
	return h
}

// approvedStatuses: approved or later.
func approved(status string) bool {
	return status == "approved" || status == "implemented" || status == "verified"
}

func inactive(status string) bool { return status == "rejected" || status == "obsolete" }

func mandatory(obligation string) bool {
	switch obligation {
	case model.ObligationMust, model.ObligationMustNot, model.ObligationShall, model.ObligationShallNot:
		return true
	}
	return false
}

func riskLevelAtLeast(level, min string) bool {
	return slices.Index(model.RiskLevels, level) >= slices.Index(model.RiskLevels, min)
}

// builtinEval holds the state of one built-in rule evaluation.
type builtinEval struct {
	x     *index
	h     Hooks
	reqs  map[string]Item
	tests map[string]Item
	res   *Result
}

func (b *builtinEval) req(id string) (Item, bool) {
	if it, ok := b.reqs[id]; ok {
		it.Status = b.x.g.Reqs[id].Status
		return it, true
	}
	if it, ok := b.x.g.Deleted[id]; ok {
		it.Status = ReasonDeleted
		return it, false
	}
	return Item{Kind: model.KindRequirement, ID: id, Status: ReasonDeleted}, false
}

func (b *builtinEval) test(id string) (Item, bool) {
	if it, ok := b.tests[id]; ok {
		it.Status = b.x.g.TestStatus[id]
		return it, true
	}
	if it, ok := b.x.g.Deleted[id]; ok {
		it.Status = ReasonDeleted
		return it, false
	}
	return Item{Kind: model.KindTest, ID: id, Status: ReasonDeleted}, false
}

func (b *builtinEval) add(v Violation) {
	if v.Related == nil {
		v.Related = []Related{}
	}
	if v.Fixes == nil {
		v.Fixes = []string{}
	}
	v.Required = 0
	v.Total = len(v.Related)
	v.Found = len(v.Related)
	b.res.Violations = append(b.res.Violations, v)
}

func addFix(fixes []string, f string) []string {
	if !slices.Contains(fixes, f) {
		fixes = append(fixes, f)
	}
	return fixes
}

func (x *index) evaluateBuiltin(n int, r model.TraceRule, scope Scope) Result {
	res := Result{Rule: r, Violations: []Violation{}}
	b := &builtinEval{x: x, h: x.hooks(), reqs: map[string]Item{}, tests: map[string]Item{}, res: &res}
	for _, it := range x.g.Requirements {
		b.reqs[it.ID] = it
	}
	for _, it := range x.g.Tests {
		b.tests[it.ID] = it
	}
	kind := model.BuiltinScope(r.Kind)
	if kind == model.KindRisk {
		b.risks(r, scope)
		res.ViolationCount = len(res.Violations)
		res.Passed = res.ViolationCount == 0
		return res
	}
	sel, err := x.parse(r.Selector, kind)
	if err != nil {
		res.Error = fmt.Sprintf("selector: %v", err)
		return res
	}
	selID := fmt.Sprintf("s%d", n)
	items := x.g.Requirements
	if kind == model.KindTest {
		items = x.g.Tests
	}
	var checked []Item
	for _, it := range items {
		if scope != nil && !scope(it.Kind, it.ID) {
			continue
		}
		if !x.match(sel, selID, it.Kind, it.ID) {
			continue
		}
		checked = append(checked, it)
	}
	res.Matched = len(checked)
	switch r.Kind {
	case model.CheckConflictInRelease:
		b.conflicts(checked)
	case model.CheckUnmetDependency:
		b.dependencies(checked)
	case model.CheckObsoleteParent:
		b.parents(checked)
	case model.CheckVerifiesInactive:
		b.verifiesInactive(checked)
	case model.CheckBrokenPrerequisite:
		b.prerequisites(checked)
	case model.CheckUnevidencedMust:
		b.unevidenced(checked)
	case model.CheckUnjustifiedRejection:
		b.rejections(checked)
	case model.CheckDuplicateSuspect:
		if x.g.Hooks.Similarity != nil {
			b.duplicates(r, checked)
		} else {
			b.similarDuplicates(r, checked, model.KindRequirement)
		}
	case model.CheckTestDuplicateSuspect:
		b.similarDuplicates(r, checked, model.KindTest)
	case model.CheckPotentialConflict:
		b.potentialConflicts(r, checked)
	}
	res.ViolationCount = len(res.Violations)
	res.Passed = res.ViolationCount == 0
	return res
}

// liveReleases are the releases that are not cancelled (released or not).
func (b *builtinEval) releasesOf() map[string][]Release {
	out := map[string][]Release{}
	for _, rel := range b.x.g.Releases {
		if rel.Status == "cancelled" {
			continue
		}
		for _, id := range rel.RequirementIDs {
			out[id] = append(out[id], rel)
		}
	}
	return out
}

func releaseItem(rel Release) Item {
	return Item{Kind: "release", ID: rel.ID, Key: rel.Name, Title: rel.Name, Status: rel.Status}
}

func (b *builtinEval) conflicts(checked []Item) {
	g := b.x.g
	rels := b.releasesOf()
	for _, it := range checked {
		if !approved(g.Reqs[it.ID].Status) {
			continue
		}
		mine := map[string]Release{}
		for _, rel := range rels[it.ID] {
			mine[rel.ID] = rel
		}
		if len(mine) == 0 {
			continue
		}
		v := Violation{Item: it, Check: ReasonConflict}
		v.Status = g.Reqs[it.ID].Status
		seenRel := map[string]bool{}
		check := func(rel model.Relation, other string) {
			if rel.Type != "conflicts_with" {
				return
			}
			o, ok := b.req(other)
			if !ok || !approved(o.Status) {
				return
			}
			var shared []Release
			for _, orel := range rels[other] {
				if r, ok := mine[orel.ID]; ok {
					shared = append(shared, r)
				}
			}
			if len(shared) == 0 {
				return
			}
			names := make([]string, len(shared))
			for i, r := range shared {
				names[i] = r.Name
			}
			v.Related = append(v.Related, Related{Item: o, Role: "conflicts_with", Link: rel.ID, Check: ReasonConflict, Note: strings.Join(names, ", ")})
			for _, r := range shared {
				if !seenRel[r.ID] {
					seenRel[r.ID] = true
					v.Related = append(v.Related, Related{Item: releaseItem(r), Role: "release"})
				}
			}
		}
		for _, rel := range b.x.relOut[it.ID] {
			check(rel, rel.TargetID)
		}
		for _, rel := range b.x.relIn[it.ID] {
			check(rel, rel.SourceID)
		}
		if len(v.Related) > 0 {
			v.Fixes = []string{FixRemoveFromRelease, FixRemoveRelation}
			b.add(v)
		}
	}
}

// deletedRelationsFrom: relations removed with a trashed target, by source.
func (b *builtinEval) deletedRelationsFrom() map[string][]model.Relation {
	out := map[string][]model.Relation{}
	for _, r := range b.x.g.DeletedRelations {
		out[r.SourceID] = append(out[r.SourceID], r)
	}
	return out
}

func (b *builtinEval) dependencies(checked []Item) {
	g := b.x.g
	deleted := b.deletedRelationsFrom()
	for _, it := range checked {
		st := g.Reqs[it.ID].Status
		if !approved(st) {
			continue
		}
		v := Violation{Item: it}
		v.Status = st
		for _, rel := range b.x.relOut[it.ID] {
			if rel.Type != "depends_on" {
				continue
			}
			o, ok := b.req(rel.TargetID)
			switch {
			case !ok:
				v.Related = append(v.Related, Related{Item: o, Role: "depends_on", Link: rel.ID, Check: ReasonDeleted})
				v.Fixes = addFix(v.Fixes, FixRemoveRelation)
			case o.Status == "rejected" || o.Status == "obsolete":
				v.Related = append(v.Related, Related{Item: o, Role: "depends_on", Link: rel.ID, Check: o.Status})
				v.Fixes = addFix(v.Fixes, FixRemoveRelation)
			}
		}
		for _, rel := range deleted[it.ID] {
			if rel.Type != "depends_on" || g.Dismissed[DismissKey(model.CheckUnmetDependency, it.ID, rel.TargetID)] {
				continue
			}
			o, _ := b.req(rel.TargetID)
			v.Related = append(v.Related, Related{Item: o, Role: "depends_on", Check: ReasonDeleted})
			v.Fixes = addFix(v.Fixes, FixDismiss)
		}
		if len(v.Related) > 0 {
			v.Check = v.Related[0].Check
			b.add(v)
		}
	}
}

func (b *builtinEval) parents(checked []Item) {
	g := b.x.g
	for _, it := range checked {
		st := g.Reqs[it.ID].Status
		if inactive(st) {
			continue
		}
		v := Violation{Item: it, Check: ReasonObsolete}
		v.Status = st
		for _, rel := range b.x.relOut[it.ID] {
			if rel.Type != "refines" && rel.Type != "derived_from" {
				continue
			}
			if o, ok := b.req(rel.TargetID); ok && o.Status == "obsolete" {
				v.Related = append(v.Related, Related{Item: o, Role: rel.Type, Link: rel.ID, Check: ReasonObsolete})
			}
		}
		if len(v.Related) > 0 {
			v.Fixes = []string{FixSetObsolete, FixRemoveRelation}
			b.add(v)
		}
	}
}

func (b *builtinEval) verifiesInactive(checked []Item) {
	for _, it := range checked {
		v := Violation{Item: it}
		v.Status = b.x.g.TestStatus[it.ID]
		for _, l := range b.x.tstLinks[it.ID] {
			if o, ok := b.req(l.RequirementID); ok && inactive(o.Status) {
				v.Related = append(v.Related, Related{Item: o, Role: "verifies", Link: l.ID, Check: o.Status})
			}
		}
		if len(v.Related) > 0 {
			v.Check = v.Related[0].Check
			v.Fixes = []string{FixRemoveLink}
			b.add(v)
		}
	}
}

func (b *builtinEval) prerequisites(checked []Item) {
	g := b.x.g
	out := map[string][]model.TestDependency{}
	for _, d := range g.Dependencies {
		out[d.TestID] = append(out[d.TestID], d)
	}
	cycles := cycleMembers(g.Dependencies)
	deleted := map[string][]model.TestDependency{}
	for _, d := range g.DeletedDependencies {
		deleted[d.TestID] = append(deleted[d.TestID], d)
	}
	for _, it := range checked {
		v := Violation{Item: it}
		v.Status = g.TestStatus[it.ID]
		comp := cycles[it.ID]
		for _, d := range out[it.ID] {
			o, ok := b.test(d.DependsOnID)
			switch {
			case !ok:
				v.Related = append(v.Related, Related{Item: o, Role: "prerequisite", Link: d.ID, Check: ReasonDeleted})
				v.Fixes = addFix(v.Fixes, FixRemoveDependency)
			case comp != 0 && cycles[d.DependsOnID] == comp:
				v.Related = append(v.Related, Related{Item: o, Role: "prerequisite", Link: d.ID, Check: ReasonCycle})
				v.Fixes = addFix(v.Fixes, FixRemoveDependency)
			case b.h.TestFailing(g, d.DependsOnID):
				o.Status = g.LastResults[d.DependsOnID]
				v.Related = append(v.Related, Related{Item: o, Role: "prerequisite", Link: d.ID, Check: ReasonFailing, Note: g.LastResults[d.DependsOnID]})
			}
		}
		for _, d := range deleted[it.ID] {
			if g.Dismissed[DismissKey(model.CheckBrokenPrerequisite, it.ID, d.DependsOnID)] {
				continue
			}
			o, _ := b.test(d.DependsOnID)
			v.Related = append(v.Related, Related{Item: o, Role: "prerequisite", Check: ReasonDeleted})
			v.Fixes = addFix(v.Fixes, FixDismiss)
		}
		if len(v.Related) > 0 {
			// the worst reason first: a cycle, then deleted, then failing
			v.Check = ReasonFailing
			for _, rl := range v.Related {
				if rl.Check == ReasonCycle {
					v.Check = ReasonCycle
					break
				}
				if rl.Check == ReasonDeleted {
					v.Check = ReasonDeleted
				}
			}
			b.add(v)
		}
	}
}

// cycleMembers returns, for every test on a prerequisite cycle, the
// number (1, 2 …) of its strongly connected component (Tarjan).
func cycleMembers(deps []model.TestDependency) map[string]int {
	adj := map[string][]string{}
	var nodes []string
	seen := map[string]bool{}
	self := map[string]bool{}
	for _, d := range deps {
		adj[d.TestID] = append(adj[d.TestID], d.DependsOnID)
		if d.TestID == d.DependsOnID {
			self[d.TestID] = true
		}
		for _, n := range []string{d.TestID, d.DependsOnID} {
			if !seen[n] {
				seen[n] = true
				nodes = append(nodes, n)
			}
		}
	}
	sort.Strings(nodes)
	idx, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	var stack []string
	counter, comp := 0, 0
	out := map[string]int{}
	var visit func(v string)
	visit = func(v string) {
		idx[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		on[v] = true
		for _, w := range adj[v] {
			if _, ok := idx[w]; !ok {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], idx[w])
			}
		}
		if low[v] == idx[v] {
			var members []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				members = append(members, w)
				if w == v {
					break
				}
			}
			if len(members) > 1 || self[v] {
				comp++
				for _, m := range members {
					out[m] = comp
				}
			}
		}
	}
	for _, n := range nodes {
		if _, ok := idx[n]; !ok {
			visit(n)
		}
	}
	return out
}

func (b *builtinEval) unevidenced(checked []Item) {
	g := b.x.g
	rels := map[string][]Release{}
	for _, rel := range g.Releases {
		if rel.Status != "planned" && rel.Status != "active" {
			continue
		}
		for _, id := range rel.RequirementIDs {
			rels[id] = append(rels[id], rel)
		}
	}
	for _, it := range checked {
		f := g.Reqs[it.ID]
		if !mandatory(f.Obligation) || inactive(f.Status) || len(rels[it.ID]) == 0 || b.h.Verified(g, it.ID) {
			continue
		}
		v := Violation{Item: it, Check: ReasonNoEvidence, Note: f.Obligation}
		v.Status = f.Status
		for _, rel := range rels[it.ID] {
			v.Related = append(v.Related, Related{Item: releaseItem(rel), Role: "release"})
		}
		b.add(v)
	}
}

func (b *builtinEval) rejections(checked []Item) {
	g := b.x.g
	for _, it := range checked {
		f := g.Reqs[it.ID]
		if f.Status != "rejected" || !b.h.FromRegulation(g, it.ID) || b.h.Justified(g, it.ID) {
			continue
		}
		v := Violation{Item: it, Check: ReasonUnjustified, Note: f.CatalogRef, Fixes: []string{FixAddJustification}}
		v.Status = f.Status
		b.add(v)
	}
}

func (b *builtinEval) risks(r model.TraceRule, scope Scope) {
	g := b.x.g
	level := r.Level
	if level == "" {
		level = "high"
	}
	for _, k := range g.Risks {
		if scope != nil && !scope(model.KindRisk, k.ID) {
			inScope := false
			for _, id := range k.RequirementIDs {
				if scope(model.KindRequirement, id) {
					inScope = true
					break
				}
			}
			if !inScope {
				continue
			}
		}
		lv := model.RiskLevel(k.Likelihood, k.Impact)
		if k.Status == "closed" || k.Status == "accepted" || !riskLevelAtLeast(lv, level) {
			continue
		}
		b.res.Matched++
		it := Item{Kind: model.KindRisk, ID: k.ID, Key: k.Key, Title: k.Title, Status: k.Status}
		v := Violation{Item: it, Note: lv, Fixes: []string{FixLinkMitigation}}
		if len(k.RequirementIDs) == 0 {
			v.Check = ReasonNoMitigation
			b.add(v)
			continue
		}
		verified := false
		for _, id := range k.RequirementIDs {
			if b.h.Verified(g, id) {
				verified = true
				break
			}
		}
		if verified {
			continue
		}
		v.Check = ReasonNotVerified
		for _, id := range k.RequirementIDs {
			if o, ok := b.req(id); ok {
				v.Related = append(v.Related, Related{Item: o, Role: "mitigation", Check: ReasonNotVerified})
			}
		}
		b.add(v)
	}
}

// ---------------------------------------------------------------- duplicates

// Normalize lower-cases a text and keeps letters and digits, words
// separated by single spaces.
func Normalize(s string) string {
	var sb strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			space = false
			sb.WriteRune(r)
		} else {
			space = true
		}
	}
	return sb.String()
}

// shingles returns the sorted distinct hashed character trigrams of a
// normalised text (texts shorter than three characters are one shingle).
func shingles(s string) []uint32 {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	set := map[uint32]bool{}
	h := fnv.New32a()
	add := func(part []rune) {
		h.Reset()
		_, _ = h.Write([]byte(string(part)))
		set[h.Sum32()] = true
	}
	if len(r) < 3 {
		add(r)
	} else {
		for i := 0; i+3 <= len(r); i++ {
			add(r[i : i+3])
		}
	}
	out := make([]uint32, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func dice(a, b []uint32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, common := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			common++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return 2 * float64(common) / float64(len(a)+len(b))
}

// Similarity of two texts (0–1): the Dice coefficient of the character
// trigrams of their normalised forms; identical normalised texts are 1.
func Similarity(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	return dice(shingles(na), shingles(nb))
}

func (b *builtinEval) duplicates(r model.TraceRule, checked []Item) {
	g := b.x.g
	t := r.Threshold
	if t <= 0 {
		t = 0.9
	}
	related := map[string]bool{}
	for _, rel := range g.Relations {
		if rel.Type == "duplicates" {
			related[DismissKey("", rel.SourceID, rel.TargetID)] = true
		}
	}
	inCheck := map[string]bool{}
	for _, it := range checked {
		inCheck[it.ID] = true
	}
	type cand struct {
		it   Item
		norm string
		sh   []uint32
		f    ReqFacts
	}
	var cs []cand
	for _, it := range g.Requirements {
		f := g.Reqs[it.ID]
		if inactive(f.Status) {
			continue
		}
		n := Normalize(f.Text)
		if n == "" {
			continue
		}
		it.Status = f.Status
		cs = append(cs, cand{it: it, norm: n, sh: shingles(n), f: f})
	}
	sort.SliceStable(cs, func(i, j int) bool { return len(cs[i].sh) < len(cs[j].sh) })
	custom := b.x.g.Hooks.Similarity != nil
	// Dice ≥ t needs the smaller set to be at least t/(2-t) of the larger
	ratio := t / (2 - t)
	type hit struct {
		rel Related
		sim float64
	}
	found := map[string][]hit{}
	var order []string
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			if !custom && float64(len(cs[i].sh)) < ratio*float64(len(cs[j].sh)) {
				break
			}
			a, c := cs[i], cs[j]
			// report on the newer one: it duplicates the older
			newer, older := a, c
			if c.f.Created.After(a.f.Created) || (c.f.Created.Equal(a.f.Created) && c.it.Key > a.it.Key) {
				newer, older = c, a
			}
			if !inCheck[newer.it.ID] || related[DismissKey("", a.it.ID, c.it.ID)] || g.Dismissed[DismissKey(model.CheckDuplicateSuspect, a.it.ID, c.it.ID)] {
				continue
			}
			var sim float64
			switch {
			case a.norm == c.norm:
				sim = 1
			case custom:
				sim = b.h.Similarity(a.f.Text, c.f.Text)
			default:
				sim = dice(a.sh, c.sh)
			}
			if sim < t {
				continue
			}
			if _, ok := found[newer.it.ID]; !ok {
				order = append(order, newer.it.ID)
			}
			found[newer.it.ID] = append(found[newer.it.ID], hit{Related{Item: older.it, Role: "duplicate", Check: ReasonSimilar,
				Note: fmt.Sprintf("%d%%", int(math.Floor(sim*100+1e-9)))}, sim})
		}
	}
	// violations in the order of the requirements
	pos := map[string]int{}
	for i, it := range g.Requirements {
		pos[it.ID] = i
	}
	sort.Slice(order, func(i, j int) bool { return pos[order[i]] < pos[order[j]] })
	for _, id := range order {
		it := b.reqs[id]
		it.Status = g.Reqs[id].Status
		hits := found[id]
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].sim > hits[j].sim })
		rel := make([]Related, len(hits))
		for i, h := range hits {
			rel[i] = h.rel
		}
		b.add(Violation{Item: it, Check: ReasonSimilar, Note: rel[0].Note, Related: rel, Fixes: []string{FixMarkDuplicate, FixDismiss}})
	}
}
