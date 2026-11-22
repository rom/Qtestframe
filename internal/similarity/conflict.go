package similarity

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/reqlang"
)

// Conflict kinds.
const (
	// ConflictObligation: the same statement with opposite obligations
	// (MUST vs MUST NOT, SKA vs FÅR INTE).
	ConflictObligation = "obligation"
	// ConflictThreshold: numeric thresholds on the same quantity that
	// cannot both hold (≤ 2 s vs ≥ 5 s).
	ConflictThreshold = "threshold"
	// ConflictExclusive: different values of a mutually exclusive set
	// (enabled vs disabled, mandatory vs optional).
	ConflictExclusive = "exclusive"
)

// ConflictKinds lists the kinds in display order.
var ConflictKinds = []string{ConflictObligation, ConflictThreshold, ConflictExclusive}

// Thresholds of the conflict detection.
const (
	// DefaultConflictConfidence is the least confidence reported by
	// default (the potential_conflict check and the review page).
	DefaultConflictConfidence = 0.6
	// conflictCandidate: pairs of items less similar than this are not
	// compared statement by statement.
	conflictCandidate = 0.08
	// statementOverlap: the least share of content two statements must
	// have in common to be about the same thing.
	obligationOverlap = 0.5
	thresholdOverlap  = 0.15
	// thresholdSimilarity: items this similar compare their thresholds
	// whenever their statements share content.
	thresholdSimilarity = 0.3
	exclusiveOverlap    = 0.3
)

// Part is a piece of a statement; Mark highlights the evidence.
type Part struct {
	Text string `json:"text"`
	Mark bool   `json:"mark,omitempty"`
}

// Evidence is the statement of an item that conflicts, in parts.
type Evidence struct {
	// Field: title, description or details.
	Field string `json:"field"`
	Parts []Part `json:"parts"`
}

// Conflict is a pair of documents that likely contradict each other.
type Conflict struct {
	A, B string
	Kind string
	// Confidence 0–1 of the conflict.
	Confidence float64
	// Similarity of the two documents.
	Similarity float64
	// Note: the decisive terms, e.g. "MUST ↔ MUST NOT", "≤ 2 s ↔ ≥ 5 s".
	Note      string
	EvidenceA Evidence
	EvidenceB Evidence
}

// ConflictDoc is a document with its declared obligation (the obligation
// field of a requirement, used for statements without a keyword).
type ConflictDoc struct {
	Doc
	Obligation string
}

// statement is a sentence of a document, analysed.
type statement struct {
	field      string
	text       string
	polarity   int // +1 positive, -1 negative obligation, 0 none
	keyword    bool
	marks      [][2]int // obligation keywords / negations (byte ranges)
	content    map[string]bool
	exclusive  map[int]exclusiveHit // group → value
	thresholds []constraint
}

type exclusiveHit struct {
	value      int
	start, end int
	mixed      bool // several values of the group
}

func positive(level string) bool {
	switch level {
	case reqlang.LevelMust, reqlang.LevelShall, reqlang.LevelShould, reqlang.LevelWill, reqlang.LevelMay, reqlang.LevelOptional:
		return true
	}
	return false
}

func negativeLevel(level string) bool {
	return level == reqlang.LevelMustNot || level == reqlang.LevelShallNot || level == reqlang.LevelShouldNot
}

func analyseStatements(d ConflictDoc) []statement {
	lang := d.lang()
	var out []statement
	add := func(field, text string) {
		for _, s := range sentences(text) {
			out = append(out, analyseStatement(field, text[s.start:s.end], lang, d.Obligation))
		}
	}
	add("title", d.Title)
	add("description", d.Description)
	for _, x := range d.Details {
		add("details", x)
	}
	return out
}

func analyseStatement(field, text, lang, declared string) statement {
	st := statement{field: field, text: text, content: map[string]bool{}, exclusive: map[int]exclusiveHit{}}
	a := reqlang.AnalyzeLang(text, lang)
	for _, k := range a.Keywords {
		switch {
		case negativeLevel(k.Level):
			st.polarity, st.keyword = -1, true
			st.marks = append(st.marks, [2]int{k.Offset, k.Offset + k.Length})
		case positive(k.Level):
			if st.polarity == 0 {
				st.polarity = 1
			}
			st.keyword = true
			st.marks = append(st.marks, [2]int{k.Offset, k.Offset + k.Length})
		}
	}
	ws := words(text)
	// a negation right after a positive keyword ("must never", "ska
	// aldrig"), or in a statement without a keyword ("is not stored")
	for i, w := range ws {
		if !negations[w.lower] {
			continue
		}
		after := false
		for _, m := range st.marks {
			if w.start >= m[1] && w.start-m[1] <= 12 {
				after = true
			}
			if w.start >= m[0] && w.end <= m[1] {
				after = false // part of the keyword (MUST NOT)
				break
			}
		}
		if (st.keyword && after && st.polarity > 0) || (!st.keyword && i > 0) {
			st.polarity = -1
			st.marks = append(st.marks, [2]int{w.start, w.end})
		}
	}
	if !st.keyword && st.polarity == 0 && positive(declared) {
		st.polarity = 1
	}
	if !st.keyword && st.polarity == 0 && negativeLevel(declared) {
		st.polarity = -1
	}
	// content terms and exclusive values
	lower := make([]string, len(ws))
	for i, w := range ws {
		lower[i] = w.lower
	}
	skip := map[int]bool{}
	for i := range ws {
		key := lower[i]
		end := ws[i].end
		if i+1 < len(ws) {
			if v, ok := exclusiveIndex[key+" "+lower[i+1]]; ok {
				st.addExclusive(v, ws[i].start, ws[i+1].end)
				skip[i], skip[i+1] = true, true
				continue
			}
		}
		if v, ok := exclusiveIndex[key]; ok {
			st.addExclusive(v, ws[i].start, end)
			skip[i] = true
		}
	}
	for _, t := range terms(text, lang) {
		if !t.content || strings.HasPrefix(t.key, "n:") {
			continue
		}
		excl := false
		for i := range ws {
			if skip[i] && ws[i].start >= t.start && ws[i].end <= t.end {
				excl = true
			}
		}
		if !excl && !units[strings.TrimPrefix(t.key, "w:")].isUnit() {
			st.content[t.key] = true
		}
	}
	st.thresholds = parseConstraints(text, st.polarity < 0)
	// a threshold's own words are not content ("at least", "minutes")
	for _, c := range st.thresholds {
		for _, t := range terms(text[c.start:c.end], lang) {
			if t.key != "c:attempt" && !strings.HasPrefix(t.key, "c:") {
				delete(st.content, t.key)
			}
		}
	}
	return st
}

func (u unit) isUnit() bool { return u.dim != "" }

func (st *statement) addExclusive(v exclusiveValue, start, end int) {
	if h, ok := st.exclusive[v.group]; ok {
		if h.value != v.value {
			h.mixed = true
			st.exclusive[v.group] = h
		}
		return
	}
	st.exclusive[v.group] = exclusiveHit{value: v.value, start: start, end: end}
}

// evidence splits a statement into parts with the ranges marked.
func evidence(st statement, ranges [][2]int) Evidence {
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	var parts []Part
	pos := 0
	for _, r := range ranges {
		if r[0] < pos || r[1] > len(st.text) || r[0] >= r[1] {
			continue
		}
		if r[0] > pos {
			parts = append(parts, Part{Text: st.text[pos:r[0]]})
		}
		parts = append(parts, Part{Text: st.text[r[0]:r[1]], Mark: true})
		pos = r[1]
	}
	if pos < len(st.text) {
		parts = append(parts, Part{Text: st.text[pos:]})
	}
	return Evidence{Field: st.field, Parts: parts}
}

// Conflicts finds the pairs of docs that likely contradict each other with
// a confidence of at least min, most confident first. The index must hold
// the docs (it supplies their similarity and the rarity of terms); skip
// excludes pairs (nil: none).
func Conflicts(x *Index, docs []ConflictDoc, min float64, skip func(a, b string) bool) []Conflict {
	stmts := make(map[string][]statement, len(docs))
	order := map[string]int{}
	for i, d := range docs {
		stmts[d.ID] = analyseStatements(d)
		order[d.ID] = i
	}
	var out []Conflict
	for _, p := range x.Pairs(conflictCandidate) {
		sa, okA := stmts[p.A]
		sb, okB := stmts[p.B]
		if !okA || !okB || (skip != nil && skip(p.A, p.B)) {
			continue
		}
		a, b := p.A, p.B
		if order[b] < order[a] {
			a, b, sa, sb = b, a, sb, sa
		}
		if c, ok := x.compare(a, b, sa, sb, p.Score); ok && c.Confidence >= min {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out
}

// compare finds the strongest conflict between the statements of two docs.
func (x *Index) compare(a, b string, sa, sb []statement, sim float64) (Conflict, bool) {
	best := Conflict{}
	found := false
	consider := func(c Conflict) {
		if !found || c.Confidence > best.Confidence {
			best, found = c, true
		}
	}
	for _, s := range sa {
		for _, t := range sb {
			ov := x.overlap(s.content, t.content)
			if ov == 0 {
				continue
			}
			base := Conflict{A: a, B: b, Similarity: sim}
			// different values of an exclusive set, same polarity
			exclusive := false
			for g, hs := range s.exclusive {
				ht, ok := t.exclusive[g]
				if !ok || hs.mixed || ht.mixed || hs.value == ht.value {
					continue
				}
				exclusive = true
				// (a statement without an obligation, such as a title, is
				// positive)
				if positiveOrNone(s.polarity) == positiveOrNone(t.polarity) && ov >= exclusiveOverlap {
					c := base
					c.Kind = ConflictExclusive
					c.Confidence = confidence(0.45, ov, sim, 0.9)
					c.Note = fmt.Sprintf("%s ↔ %s", s.text[hs.start:hs.end], t.text[ht.start:ht.end])
					c.EvidenceA = evidence(s, append([][2]int{{hs.start, hs.end}}, s.marks...))
					c.EvidenceB = evidence(t, append([][2]int{{ht.start, ht.end}}, t.marks...))
					consider(c)
				}
			}
			// opposite obligations on the same statement
			if !exclusive && s.polarity*t.polarity < 0 && ov >= obligationOverlap && sameThresholds(s, t) {
				c := base
				c.Kind = ConflictObligation
				strength := 1.0
				if !s.keyword || !t.keyword {
					strength = 0.8
				}
				c.Confidence = confidence(0.45, ov, sim, strength)
				c.Note = fmt.Sprintf("%s ↔ %s", marked(s), marked(t))
				c.EvidenceA = evidence(s, s.marks)
				c.EvidenceB = evidence(t, t.marks)
				consider(c)
			}
			// thresholds on the same quantity that cannot both hold
			// (statements on one subject: they share content, and the
			// statements or the items are similar enough)
			if ov >= thresholdOverlap || sim >= thresholdSimilarity {
				for _, ca := range s.thresholds {
					for _, cb := range t.thresholds {
						if !disjoint(ca, cb) {
							continue
						}
						c := base
						c.Kind = ConflictThreshold
						c.Confidence = confidence(0.5, math.Max(ov, sim), sim, 1)
						c.Note = fmt.Sprintf("%s ↔ %s", strings.TrimSpace(s.text[ca.start:ca.end]), strings.TrimSpace(t.text[cb.start:cb.end]))
						c.EvidenceA = evidence(s, [][2]int{{ca.start, ca.end}})
						c.EvidenceB = evidence(t, [][2]int{{cb.start, cb.end}})
						consider(c)
					}
				}
			}
		}
	}
	return best, found
}

// sameThresholds: two statements with opposite obligations only say the
// opposite when they do not differ in their thresholds ("must respond
// within 2 s" and "must not take longer than 5 s" agree).
func sameThresholds(s, t statement) bool {
	return len(s.thresholds) == 0 && len(t.thresholds) == 0
}

func positiveOrNone(p int) int {
	if p == 0 {
		return 1
	}
	return p
}

func marked(s statement) string {
	var parts []string
	for _, m := range s.marks {
		parts = append(parts, s.text[m[0]:m[1]])
	}
	return strings.Join(parts, " ")
}

// confidence of a conflict from the overlap of the statements, the
// similarity of the items and the strength of the evidence.
func confidence(base, overlap, sim, strength float64) float64 {
	c := (base + 0.4*overlap + 0.15*math.Min(1, sim*2)) * strength
	return math.Min(math.Round(c*100)/100, 0.99)
}
