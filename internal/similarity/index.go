// Package similarity finds requirements and tests that say the same thing
// (suspected duplicates) or that contradict each other (potential
// conflicts), offline and in pure Go, for English and Swedish text.
//
// Similarity: the text of an item is analysed per field (title,
// description, details such as acceptance criteria or test steps):
// normalised, split into words, synonyms and abbreviations of the domain
// replaced by concepts ("sign in" = "log in" = "logga in"), stop words
// dropped and the other words stemmed (Snowball English and Swedish
// stemmers, stem.go). Character 4-grams of the words let Swedish compounds
// match their parts. Each field becomes a TF-IDF vector over the corpus of
// the project; the fields are weighted (title > description > details)
// into one vector, and two items are as similar as the cosine of their
// vectors (0–1). Optionally an embeddings service supplies the vectors
// (embeddings.go).
//
// Conflicts (conflict.go): pairs of similar statements with opposite
// obligations (MUST vs MUST NOT), numeric thresholds on the same quantity
// that cannot both hold (≤ 2 s vs ≥ 5 s, "minst 12 tecken" vs "högst 8"),
// or different values of a mutually exclusive set (enabled vs disabled).
package similarity

import (
	"math"
	"sort"
	"strings"
)

// Field weights: how much each field counts in the vector of an item.
const (
	TitleWeight       = 1.3
	DescriptionWeight = 1.0
	DetailsWeight     = 0.6
)

// Default thresholds (see the precision tests and the user guide).
const (
	// DefaultDuplicateThreshold: items at least this similar are suspected
	// duplicates (duplicate_suspect, test_duplicate_suspect).
	DefaultDuplicateThreshold = 0.68
	// DefaultSuggestThreshold: the least similarity shown while editing.
	DefaultSuggestThreshold = 0.25
)

// Doc is a requirement or test to compare.
type Doc struct {
	ID          string
	Title       string
	Description string
	// Details: acceptance criteria of a requirement; preconditions, steps
	// and expected results of a test.
	Details []string
	// Lang: "en" or "sv" ("" detects it from the text).
	Lang string
}

// Text is the whole text of a document (what an embeddings service sees).
func (d Doc) Text() string {
	parts := []string{strings.TrimSpace(d.Title), strings.TrimSpace(d.Description)}
	for _, x := range d.Details {
		if x = strings.TrimSpace(x); x != "" {
			parts = append(parts, x)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (d Doc) lang() string {
	if d.Lang == "en" || d.Lang == "sv" {
		return d.Lang
	}
	return Language(d.Text())
}

// entry is one feature of a sparse vector.
type entry struct {
	f int
	w float64
}

// Vector is a normalised sparse vector (feature ids ascending).
type Vector []entry

func dot(a, b Vector) float64 {
	i, j, s := 0, 0, 0.0
	for i < len(a) && j < len(b) {
		switch {
		case a[i].f == b[j].f:
			s += a[i].w * b[j].w
			i++
			j++
		case a[i].f < b[j].f:
			i++
		default:
			j++
		}
	}
	return s
}

// Index is the TF-IDF model of a corpus.
type Index struct {
	ids   []string
	pos   map[string]int
	feat  map[string]int // feature → id
	df    []int
	vecs  []Vector
	post  [][]posting // feature id → documents
	n     int
	exact []string // normalised text (identical texts score 1)
}

type posting struct {
	doc int
	w   float64
}

// raw is the unweighted features of each field of a document.
type raw struct {
	fields [3]map[string]float64
	norm   string
}

func analyse(d Doc) raw {
	lang := d.lang()
	var r raw
	for i := range r.fields {
		r.fields[i] = map[string]float64{}
	}
	features(d.Title, lang, r.fields[0])
	features(d.Description, lang, r.fields[1])
	for _, x := range d.Details {
		features(x, lang, r.fields[2])
	}
	r.norm = normalize(d.Title + " " + d.Description)
	return r
}

// normalize lower-cases a text and keeps its words separated by spaces.
func normalize(s string) string {
	ws := words(s)
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.lower
	}
	return strings.Join(parts, " ")
}

// NewIndex builds the model of docs (ids must be unique).
func NewIndex(docs []Doc) *Index {
	x := &Index{pos: map[string]int{}, feat: map[string]int{}, n: len(docs)}
	raws := make([]raw, len(docs))
	for i, d := range docs {
		raws[i] = analyse(d)
		x.ids = append(x.ids, d.ID)
		x.pos[d.ID] = i
		x.exact = append(x.exact, raws[i].norm)
		seen := map[string]bool{}
		for _, f := range raws[i].fields {
			for k := range f {
				if !seen[k] {
					seen[k] = true
					id, ok := x.feat[k]
					if !ok {
						id = len(x.df)
						x.feat[k] = id
						x.df = append(x.df, 0)
					}
					x.df[id]++
				}
			}
		}
	}
	x.post = make([][]posting, len(x.df))
	x.vecs = make([]Vector, len(docs))
	for i := range raws {
		v := x.vector(raws[i])
		x.vecs[i] = v
		for _, e := range v {
			x.post[e.f] = append(x.post[e.f], posting{i, e.w})
		}
	}
	return x
}

// idf of a feature id (unknown features: as rare as possible).
func (x *Index) idf(f int) float64 {
	df := 1
	if f >= 0 {
		df = max(x.df[f], 1)
	}
	return math.Log(1 + float64(max(x.n, 1))/float64(df))
}

// common reports features too frequent to tell documents apart (in more
// than half of a corpus of at least 20 documents).
func (x *Index) common(f int) bool {
	return x.n >= 20 && x.df[f]*2 > x.n
}

func (x *Index) vector(r raw) Vector {
	acc := map[int]float64{}
	for i, weight := range []float64{TitleWeight, DescriptionWeight, DetailsWeight} {
		field := map[int]float64{}
		norm := 0.0
		for k, tf := range r.fields[i] {
			f, ok := x.feat[k]
			if !ok {
				f = -1
			} else if x.common(f) {
				continue
			}
			w := tf * x.idf(f)
			norm += w * w
			if f >= 0 {
				field[f] = w
			}
		}
		if norm == 0 {
			continue
		}
		norm = math.Sqrt(norm)
		for f, w := range field {
			acc[f] += weight * w / norm
		}
	}
	v := make(Vector, 0, len(acc))
	total := 0.0
	for f, w := range acc {
		v = append(v, entry{f, w})
		total += w * w
	}
	if total == 0 {
		return nil
	}
	total = math.Sqrt(total)
	for i := range v {
		v[i].w /= total
	}
	sort.Slice(v, func(i, j int) bool { return v[i].f < v[j].f })
	return v
}

// Len is the number of documents.
func (x *Index) Len() int { return x.n }

// Has reports whether id is a document of the index.
func (x *Index) Has(id string) bool { _, ok := x.pos[id]; return ok }

// Score is the similarity of two documents of the index (0 if unknown).
func (x *Index) Score(a, b string) float64 {
	i, ok1 := x.pos[a]
	j, ok2 := x.pos[b]
	if !ok1 || !ok2 {
		return 0
	}
	if x.exact[i] != "" && x.exact[i] == x.exact[j] {
		return 1
	}
	return clamp(dot(x.vecs[i], x.vecs[j]))
}

func clamp(s float64) float64 {
	if s > 1 {
		return 1
	}
	if s < 0 {
		return 0
	}
	return s
}

// Match is a document similar to a query.
type Match struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// Similar returns the documents most similar to d (at most limit, score
// at least min), best first; skip excludes documents (nil: none).
func (x *Index) Similar(d Doc, limit int, min float64, skip func(id string) bool) []Match {
	r := analyse(d)
	q := x.vector(r)
	acc := map[int]float64{}
	for _, e := range q {
		for _, p := range x.post[e.f] {
			acc[p.doc] += e.w * p.w
		}
	}
	if r.norm != "" {
		for i, n := range x.exact {
			if n == r.norm {
				acc[i] = 1
			}
		}
	}
	out := []Match{}
	for i, s := range acc {
		s = clamp(s)
		if s < min || s <= 0 || (skip != nil && skip(x.ids[i])) {
			continue
		}
		out = append(out, Match{ID: x.ids[i], Score: s})
	}
	sortMatches(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortMatches(m []Match) {
	sort.Slice(m, func(i, j int) bool {
		if m[i].Score != m[j].Score {
			return m[i].Score > m[j].Score
		}
		return m[i].ID < m[j].ID
	})
}

// Pair is two documents (A before B in the corpus) and their similarity.
type Pair struct {
	A, B  string
	Score float64
}

// Pairs returns every pair of documents with a similarity of at least min,
// most similar first.
func (x *Index) Pairs(min float64) []Pair {
	var out []Pair
	acc := make([]float64, x.n)
	for i := range x.vecs {
		for j := range acc {
			acc[j] = 0
		}
		for _, e := range x.vecs[i] {
			for _, p := range x.post[e.f] {
				if p.doc > i {
					acc[p.doc] += e.w * p.w
				}
			}
		}
		for j := i + 1; j < x.n; j++ {
			s := clamp(acc[j])
			if x.exact[i] != "" && x.exact[i] == x.exact[j] {
				s = 1
			}
			if s >= min && s > 0 {
				out = append(out, Pair{A: x.ids[i], B: x.ids[j], Score: s})
			}
		}
	}
	sortPairs(out)
	return out
}

func sortPairs(p []Pair) {
	sort.SliceStable(p, func(i, j int) bool { return p[i].Score > p[j].Score })
}

// overlap is the share of the content of two statements they have in
// common, weighted by how rare each term is in the corpus (0–1).
func (x *Index) overlap(a, b map[string]bool) float64 {
	shared, union := 0.0, 0.0
	for k := range a {
		w := x.termIDF(k)
		union += w
		if b[k] {
			shared += w
		}
	}
	for k := range b {
		if !a[k] {
			union += x.termIDF(k)
		}
	}
	if union == 0 {
		return 0
	}
	return shared / union
}

func (x *Index) termIDF(k string) float64 {
	if f, ok := x.feat[k]; ok {
		return x.idf(f)
	}
	return x.idf(-1)
}

// Scorer is a similarity model of a corpus: TF-IDF (Index) or embeddings
// (DenseIndex).
type Scorer interface {
	Pairs(min float64) []Pair
	Score(a, b string) float64
}

var (
	_ Scorer = (*Index)(nil)
	_ Scorer = (*DenseIndex)(nil)
)
