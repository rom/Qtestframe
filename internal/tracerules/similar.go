package tracerules

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/similarity"
)

// The checks built on internal/similarity: suspected duplicate
// requirements (duplicate_suspect) and tests (test_duplicate_suspect),
// and potentially conflicting requirements (potential_conflict).

// Reason codes of potential_conflict (Violation.Check, Related.Check):
// the conflict kinds of internal/similarity.
const (
	ReasonOppositeObligation = "opposite_obligation"
	ReasonThreshold          = "threshold"
	ReasonExclusive          = "exclusive"
)

// FixMarkConflict adds the relation "violating item conflicts_with the
// related item".
const FixMarkConflict = "mark_conflict"

// conflictReason maps a similarity conflict kind to a reason code.
func conflictReason(kind string) string {
	switch kind {
	case similarity.ConflictObligation:
		return ReasonOppositeObligation
	case similarity.ConflictThreshold:
		return ReasonThreshold
	}
	return ReasonExclusive
}

// ConflictReason is the reason code of a similarity conflict kind.
func ConflictReason(kind string) string { return conflictReason(kind) }

// SimModel is the similarity model of the active items of one kind.
type SimModel struct {
	Kind string
	// Items in the order of the graph; Docs their texts.
	Items []Item
	Docs  []similarity.ConflictDoc
	// Index is the TF-IDF model (also used by the conflict detection);
	// Scorer the model the duplicate checks use (Index, or embeddings).
	Index  *similarity.Index
	Scorer similarity.Scorer

	conflicts []similarity.Conflict
	done      bool
}

// MinConflictConfidence is the lowest confidence computed (the lowest
// threshold a rule may have).
const MinConflictConfidence = 0.5

// Conflicts returns the conflicts among the items (confidence at least
// MinConflictConfidence), computed once.
func (m *SimModel) Conflicts() []similarity.Conflict {
	if !m.done {
		m.done = true
		if m.Kind == model.KindRequirement {
			m.conflicts = similarity.Conflicts(m.Index, m.Docs, MinConflictConfidence, nil)
		}
	}
	return m.conflicts
}

// active reports whether an item takes part in the similarity checks.
func (g *Graph) active(kind, id string) bool {
	if kind == model.KindTest {
		return g.TestStatus[id] != "deprecated"
	}
	return !inactive(g.Reqs[id].Status)
}

// Doc returns the text of an item for the similarity checks.
func (g *Graph) Doc(kind, id string) similarity.ConflictDoc {
	d, ok := g.Docs[id]
	if !ok {
		d = similarity.Doc{Description: g.Reqs[id].Text}
	}
	d.ID = id
	out := similarity.ConflictDoc{Doc: d}
	if kind == model.KindRequirement {
		out.Obligation = g.Reqs[id].Obligation
	}
	return out
}

// SimilarityModel returns the model of the active items of kind
// (requirement or test), built once per graph. Hooks.Scorer may replace
// the scorer (embeddings).
func (g *Graph) SimilarityModel(kind string) *SimModel {
	if m := g.sims[kind]; m != nil {
		return m
	}
	m := &SimModel{Kind: kind}
	items := g.Requirements
	if kind == model.KindTest {
		items = g.Tests
	}
	var docs []similarity.Doc
	for _, it := range items {
		if !g.active(kind, it.ID) {
			continue
		}
		d := g.Doc(kind, it.ID)
		if strings.TrimSpace(d.Text()) == "" {
			continue
		}
		m.Items = append(m.Items, it)
		m.Docs = append(m.Docs, d)
		docs = append(docs, d.Doc)
	}
	m.Index = similarity.NewIndex(docs)
	m.Scorer = m.Index
	if g.Hooks.Scorer != nil {
		if s := g.Hooks.Scorer(g, kind, m); s != nil {
			m.Scorer = s
		}
	}
	if g.sims == nil {
		g.sims = map[string]*SimModel{}
	}
	g.sims[kind] = m
	return m
}

func (g *Graph) created(kind, id string) time.Time {
	if kind == model.KindTest {
		return g.TestCreated[id]
	}
	return g.Reqs[id].Created
}

// newerFirst orders a pair so that the item created later comes first
// (it is the one reported: it duplicates or contradicts the older one).
func (g *Graph) newerFirst(kind string, a, b Item) (Item, Item) {
	ca, cb := g.created(kind, a.ID), g.created(kind, b.ID)
	if cb.After(ca) || (cb.Equal(ca) && b.Key > a.Key) {
		return b, a
	}
	return a, b
}

// MarkedDuplicates are the pairs (DismissKey("", a, b)) already marked as
// duplicates: requirements by a duplicates relation, tests by an item link
// labelled model.DuplicateLinkLabel.
func (g *Graph) MarkedDuplicates(kind string) map[string]bool {
	out := map[string]bool{}
	if kind == model.KindTest {
		for _, l := range g.ItemLinks {
			if l.SourceType == model.KindTest && l.TargetType == model.KindTest && l.TargetID != nil && strings.EqualFold(strings.TrimSpace(l.Label), model.DuplicateLinkLabel) {
				out[DismissKey("", l.SourceID, *l.TargetID)] = true
			}
		}
		return out
	}
	for _, rel := range g.Relations {
		if rel.Type == "duplicates" {
			out[DismissKey("", rel.SourceID, rel.TargetID)] = true
		}
	}
	return out
}

// MarkedConflicts are the pairs of requirements related by conflicts_with.
func (g *Graph) MarkedConflicts() map[string]bool {
	out := map[string]bool{}
	for _, rel := range g.Relations {
		if rel.Type == "conflicts_with" {
			out[DismissKey("", rel.SourceID, rel.TargetID)] = true
		}
	}
	return out
}

func pct(x float64) string { return fmt.Sprintf("%d%%", int(math.Floor(x*100+1e-9))) }

// similarDuplicates: duplicate_suspect (without a custom Similarity hook)
// and test_duplicate_suspect.
func (b *builtinEval) similarDuplicates(r model.TraceRule, checked []Item, kind string) {
	g := b.x.g
	t := r.Threshold
	if t <= 0 {
		t = similarity.DefaultDuplicateThreshold
	}
	m := g.SimilarityModel(kind)
	items := map[string]Item{}
	for _, it := range m.Items {
		items[it.ID] = it
	}
	inCheck := map[string]bool{}
	for _, it := range checked {
		inCheck[it.ID] = true
	}
	marked := g.MarkedDuplicates(kind)
	// a pair that contradicts itself is a conflict, not a duplicate
	conflicting := map[string]bool{}
	if kind == model.KindRequirement {
		for _, c := range m.Conflicts() {
			if c.Confidence >= similarity.DefaultConflictConfidence {
				conflicting[DismissKey("", c.A, c.B)] = true
			}
		}
	}
	type hit struct {
		rel Related
		sim float64
	}
	found := map[string][]hit{}
	for _, p := range m.Scorer.Pairs(t) {
		a, okA := items[p.A]
		c, okC := items[p.B]
		if !okA || !okC {
			continue
		}
		key := DismissKey("", a.ID, c.ID)
		newer, older := g.newerFirst(kind, a, c)
		if !inCheck[newer.ID] || marked[key] || conflicting[key] || g.Dismissed[DismissKey(r.Kind, a.ID, c.ID)] {
			continue
		}
		older.Status = b.status(kind, older.ID)
		found[newer.ID] = append(found[newer.ID], hit{Related{Item: older, Role: "duplicate", Check: ReasonSimilar, Note: pct(p.Score)}, p.Score})
	}
	for _, it := range checked {
		hits := found[it.ID]
		if len(hits) == 0 {
			continue
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].sim > hits[j].sim })
		rel := make([]Related, len(hits))
		for i, h := range hits {
			rel[i] = h.rel
		}
		it.Status = b.status(kind, it.ID)
		b.add(Violation{Item: it, Check: ReasonSimilar, Note: rel[0].Note, Related: rel, Fixes: []string{FixMarkDuplicate, FixDismiss}})
	}
}

func (b *builtinEval) status(kind, id string) string {
	if kind == model.KindTest {
		return b.x.g.TestStatus[id]
	}
	return b.x.g.Reqs[id].Status
}

// potentialConflicts: potential_conflict.
func (b *builtinEval) potentialConflicts(r model.TraceRule, checked []Item) {
	g := b.x.g
	t := r.Threshold
	if t <= 0 {
		t = similarity.DefaultConflictConfidence
	}
	m := g.SimilarityModel(model.KindRequirement)
	items := map[string]Item{}
	for _, it := range m.Items {
		items[it.ID] = it
	}
	inCheck := map[string]bool{}
	for _, it := range checked {
		inCheck[it.ID] = true
	}
	marked := g.MarkedConflicts()
	type hit struct {
		rel  Related
		conf float64
	}
	found := map[string][]hit{}
	for _, c := range m.Conflicts() {
		if c.Confidence < t {
			continue
		}
		a, okA := items[c.A]
		o, okO := items[c.B]
		if !okA || !okO || marked[DismissKey("", a.ID, o.ID)] || g.Dismissed[DismissKey(r.Kind, a.ID, o.ID)] {
			continue
		}
		newer, older := g.newerFirst(model.KindRequirement, a, o)
		if !inCheck[newer.ID] {
			continue
		}
		older.Status = g.Reqs[older.ID].Status
		found[newer.ID] = append(found[newer.ID], hit{Related{Item: older, Role: "conflict", Check: conflictReason(c.Kind), Note: c.Note}, c.Confidence})
	}
	for _, it := range checked {
		hits := found[it.ID]
		if len(hits) == 0 {
			continue
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].conf > hits[j].conf })
		rel := make([]Related, len(hits))
		for i, h := range hits {
			rel[i] = h.rel
		}
		it.Status = g.Reqs[it.ID].Status
		b.add(Violation{Item: it, Check: rel[0].Check, Note: pct(hits[0].conf), Related: rel, Fixes: []string{FixMarkConflict, FixDismiss}})
	}
}
