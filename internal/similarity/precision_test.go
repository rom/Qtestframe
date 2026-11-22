package similarity

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// The precision tests run the scorer on the labelled corpora of
// testdata/fixtures.json (English and Swedish) with the default
// thresholds. They fail when a change of the word lists, the stemmers or
// the weights makes the defaults report unrelated pairs, or miss most of
// the labelled ones. The numbers are documented in the user guide
// (Duplicates and conflicts → Thresholds).

type fixtureDoc struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Details     []string `json:"details"`
}

type corpus struct {
	Name       string       `json:"name"`
	Docs       []fixtureDoc `json:"docs"`
	Duplicates [][2]string  `json:"duplicates"`
	Conflicts  [][3]string  `json:"conflicts"`
}

func loadFixtures(t *testing.T) []corpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Corpora []corpus `json:"corpora"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Corpora
}

func pairKey(a, b string) string {
	if b < a {
		a, b = b, a
	}
	return a + "|" + b
}

func (c corpus) index() (*Index, []ConflictDoc) {
	docs := make([]Doc, len(c.Docs))
	cds := make([]ConflictDoc, len(c.Docs))
	for i, d := range c.Docs {
		docs[i] = Doc{ID: d.ID, Title: d.Title, Description: d.Description, Details: d.Details, Lang: c.Name}
		cds[i] = ConflictDoc{Doc: docs[i]}
	}
	return NewIndex(docs), cds
}

type score struct{ tp, fp, fn int }

func (s score) precision() float64 {
	if s.tp+s.fp == 0 {
		return 1
	}
	return float64(s.tp) / float64(s.tp+s.fp)
}

func (s score) recall() float64 {
	if s.tp+s.fn == 0 {
		return 1
	}
	return float64(s.tp) / float64(s.tp+s.fn)
}

func TestDuplicatePrecision(t *testing.T) {
	for _, c := range loadFixtures(t) {
		t.Run(c.Name, func(t *testing.T) {
			x, cds := c.index()
			want := map[string]bool{}
			for _, p := range c.Duplicates {
				want[pairKey(p[0], p[1])] = true
			}
			// a pair that conflicts is not reported as a duplicate
			conflicting := map[string]bool{}
			for _, cf := range Conflicts(x, cds, DefaultConflictConfidence, nil) {
				conflicting[pairKey(cf.A, cf.B)] = true
			}
			var s score
			got := map[string]bool{}
			for _, p := range x.Pairs(DefaultDuplicateThreshold) {
				k := pairKey(p.A, p.B)
				if conflicting[k] {
					continue
				}
				got[k] = true
				if want[k] {
					s.tp++
				} else {
					s.fp++
					t.Logf("unexpected duplicate %s %.2f", k, p.Score)
				}
			}
			for k := range want {
				if !got[k] {
					s.fn++
					a, b := splitKey(k)
					t.Logf("missed duplicate %s %.2f", k, x.Score(a, b))
				}
			}
			t.Logf("%s duplicates: precision %.2f recall %.2f (tp %d fp %d fn %d)", c.Name, s.precision(), s.recall(), s.tp, s.fp, s.fn)
			if s.precision() < 0.9 || s.recall() < 0.75 {
				t.Errorf("precision %.2f (want ≥ 0.90), recall %.2f (want ≥ 0.75)", s.precision(), s.recall())
			}
		})
	}
}

func splitKey(k string) (string, string) {
	for i := range k {
		if k[i] == '|' {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

func TestConflictPrecision(t *testing.T) {
	for _, c := range loadFixtures(t) {
		t.Run(c.Name, func(t *testing.T) {
			x, cds := c.index()
			want := map[string]string{}
			for _, p := range c.Conflicts {
				want[pairKey(p[0], p[1])] = p[2]
			}
			var s score
			got := map[string]bool{}
			found := Conflicts(x, cds, DefaultConflictConfidence, nil)
			for _, cf := range found {
				k := pairKey(cf.A, cf.B)
				got[k] = true
				switch kind, ok := want[k]; {
				case !ok:
					s.fp++
					t.Logf("unexpected conflict %s %s %.2f %s", k, cf.Kind, cf.Confidence, cf.Note)
				case kind != cf.Kind:
					s.fp++
					t.Errorf("conflict %s: kind %s, want %s", k, cf.Kind, kind)
				default:
					s.tp++
				}
			}
			var missed []string
			for k := range want {
				if !got[k] {
					s.fn++
					missed = append(missed, k)
				}
			}
			sort.Strings(missed)
			for _, k := range missed {
				t.Logf("missed conflict %s", k)
			}
			t.Logf("%s conflicts: precision %.2f recall %.2f (tp %d fp %d fn %d)", c.Name, s.precision(), s.recall(), s.tp, s.fp, s.fn)
			if s.precision() < 0.9 || s.recall() < 0.8 {
				t.Errorf("precision %.2f (want ≥ 0.90), recall %.2f (want ≥ 0.80)", s.precision(), s.recall())
			}
			// every conflict shows its evidence
			for _, cf := range found {
				if !hasMark(cf.EvidenceA) || !hasMark(cf.EvidenceB) || cf.Note == "" {
					t.Errorf("conflict %s-%s without evidence: %+v", cf.A, cf.B, cf)
				}
			}
		})
	}
}

func hasMark(e Evidence) bool {
	for _, p := range e.Parts {
		if p.Mark {
			return true
		}
	}
	return false
}
