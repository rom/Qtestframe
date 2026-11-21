// Package suggest ranks autocomplete suggestions for free-text fields: the
// user's own recent values of a field (their history) and the values
// already used in the project (for short fields such as a requirement's
// source or owner).
//
// Ranking: a value that starts with the typed text comes first, then one
// where a word starts with it, then one that merely contains it; values
// that do not contain it are left out, and so is the typed text itself.
// Within a class the more often and the more recently used value wins
// (frecency: uses weighted down by age), then the alphabetical order. At
// most MaxResults values are returned. Matching ignores letter case and
// surrounding white space.
package suggest

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Limits.
const (
	// MaxResults is the most suggestions returned for one query.
	MaxResults = 8
	// HistoryCap is the most distinct values kept per user, organisation
	// and field (the least recently used are dropped).
	HistoryCap = 200
	// MaxValueLen is the longest value (in characters) recorded; longer
	// texts are not useful as suggestions.
	MaxValueLen = 1000
	// MaxQueryLen bounds the typed text that is matched.
	MaxQueryLen = 200
)

// Sources of a suggestion.
const (
	SourceHistory = "history"
	SourceProject = "project"
)

// Candidate is a value that may be suggested.
type Candidate struct {
	Value    string
	Uses     int
	LastUsed time.Time // zero for project values
	Source   string
}

// Suggestion is one ranked suggestion.
type Suggestion struct {
	Value  string `json:"value"`
	Source string `json:"source"` // history | project
	Uses   int    `json:"uses"`
	// LastUsed is when the user last saved the value (history only).
	LastUsed *time.Time `json:"last_used,omitempty"`
}

// Normalize returns the value as it is recorded: trimmed, with Windows
// line breaks made plain. ok is false for an empty value, one longer than
// MaxValueLen characters or one that is not valid UTF-8.
func Normalize(v string) (string, bool) {
	v = strings.TrimSpace(strings.ReplaceAll(v, "\r\n", "\n"))
	if v == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > MaxValueLen {
		return "", false
	}
	return v, true
}

// Fold is the matching form of a text: trimmed and lower case.
func Fold(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// match classes
const (
	classPrefix = iota
	classWord
	classContains
	classNone
)

// matchClass classifies how folded value v matches folded query q.
func matchClass(q, v string) int {
	if q == "" || strings.HasPrefix(v, q) {
		return classPrefix
	}
	found := false
	for i := 0; i < len(v); {
		j := strings.Index(v[i:], q)
		if j < 0 {
			break
		}
		found = true
		at := i + j
		prev, _ := utf8.DecodeLastRuneInString(v[:at])
		if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) {
			return classWord
		}
		_, size := utf8.DecodeRuneInString(v[at:])
		i = at + size
	}
	if found {
		return classContains
	}
	return classNone
}

// frecency weighs the uses of a value by its age: a value used today
// counts fully, one used a month ago half. Project values (no time) count
// half.
func frecency(c Candidate, now time.Time) float64 {
	if c.LastUsed.IsZero() {
		return float64(c.Uses) / 2
	}
	days := now.Sub(c.LastUsed).Hours() / 24
	if days < 0 {
		days = 0
	}
	return float64(c.Uses) / (1 + days/30)
}

// Merge combines candidates with the same value (ignoring case): the uses
// add up, the latest use wins, and a value from the history keeps its
// source and its spelling.
func Merge(cands []Candidate) []Candidate {
	idx := map[string]int{}
	out := []Candidate{}
	for _, c := range cands {
		k := Fold(c.Value)
		if k == "" {
			continue
		}
		i, ok := idx[k]
		if !ok {
			idx[k] = len(out)
			out = append(out, c)
			continue
		}
		m := &out[i]
		m.Uses += c.Uses
		if c.Source == SourceHistory && (m.Source != SourceHistory || c.LastUsed.After(m.LastUsed)) {
			m.Value = c.Value
		}
		if c.LastUsed.After(m.LastUsed) {
			m.LastUsed = c.LastUsed
		}
		if c.Source == SourceHistory {
			m.Source = SourceHistory
		}
	}
	return out
}

// Rank returns at most limit (1..MaxResults; otherwise MaxResults)
// suggestions for the typed text q among the candidates.
func Rank(q string, cands []Candidate, limit int, now time.Time) []Suggestion {
	if limit <= 0 || limit > MaxResults {
		limit = MaxResults
	}
	fq := Fold(q)
	if utf8.RuneCountInString(fq) > MaxQueryLen {
		return []Suggestion{}
	}
	type scored struct {
		c     Candidate
		class int
		score float64
		fold  string
	}
	var list []scored
	for _, c := range Merge(cands) {
		fv := Fold(c.Value)
		if fv == fq {
			continue // what is typed already
		}
		cl := matchClass(fq, fv)
		if cl == classNone {
			continue
		}
		list = append(list, scored{c: c, class: cl, score: frecency(c, now), fold: fv})
	}
	slices.SortFunc(list, func(a, b scored) int {
		if a.class != b.class {
			return cmp.Compare(a.class, b.class)
		}
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		if !a.c.LastUsed.Equal(b.c.LastUsed) {
			return b.c.LastUsed.Compare(a.c.LastUsed)
		}
		return cmp.Compare(a.fold, b.fold)
	})
	out := []Suggestion{}
	for _, s := range list {
		if len(out) == limit {
			break
		}
		sg := Suggestion{Value: s.c.Value, Source: s.c.Source, Uses: s.c.Uses}
		if !s.c.LastUsed.IsZero() {
			t := s.c.LastUsed.UTC()
			sg.LastUsed = &t
		}
		out = append(out, sg)
	}
	return out
}
