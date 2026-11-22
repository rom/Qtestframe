package query

import (
	"sort"
	"strings"
	"testing"
)

func TestCustomFieldPrefix(t *testing.T) {
	s := &Schema{Fields: []string{"title"}, Prefixes: []string{"cf."}}
	r := MapRecord{"title": {"Bracket"}, "cf.supplier": {"Acme Ltd"}, "cf.mass": {"2.5"}, "cf.coating": {"zinc", "paint"}}
	for expr, want := range map[string]bool{
		`cf.supplier:"Acme"`:        true,
		`cf.supplier:Other`:         false,
		`cf.mass>2`:                 true,
		`cf.mass>=3`:                false,
		`cf.coating=paint`:          true,
		`has:cf.mass`:               true,
		`has:cf.unknown`:            false,
		`cf.unknown:x`:              false,
		`CF.Supplier:acme`:          true,
		`title:bracket cf.mass<2.6`: true,
	} {
		q, err := Parse(expr, s)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got := q.Match(r); got != want {
			t.Errorf("%s = %v, want %v", expr, got, want)
		}
	}
	for _, bad := range []string{`cf.:x`, `other.x:1`} {
		if _, err := Parse(bad, s); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}

func TestSort(t *testing.T) {
	s := &Schema{Fields: []string{"title", "key"}, Prefixes: []string{"cf."}}
	recs := []MapRecord{
		{"key": {"A"}, "title": {"b"}, "cf.mass": {"10"}},
		{"key": {"B"}, "title": {"a"}, "cf.mass": {"2"}},
		{"key": {"C"}, "title": {"c"}},
		{"key": {"D"}, "title": {"A"}, "cf.mass": {"2.5"}},
	}
	order := func(expr string) string {
		q, err := Parse(expr, s)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		cp := append([]MapRecord(nil), recs...)
		sort.SliceStable(cp, func(i, j int) bool { return q.Compare(cp[i], cp[j]) < 0 })
		var keys []string
		for _, r := range cp {
			if q.Match(r) {
				keys = append(keys, r["key"][0])
			}
		}
		return strings.Join(keys, "")
	}
	for expr, want := range map[string]string{
		"sort:cf.mass":              "BDAC", // numeric, empty last
		"sort:-cf.mass":             "ADBC", // empty last also descending
		"sort:cf.mass:desc":         "ADBC",
		"sort:title":                "BDAC", // case-insensitive, stable
		"sort:title sort:-key":      "DBAC",
		"title:a sort:cf.mass:asc":  "BD",
		"":                          "ABCD",
		"key:A OR key:B sort:title": "AB", // OR binds looser: sort is in an OR branch
	} {
		if expr == "key:A OR key:B sort:title" {
			if _, err := Parse(expr, s); err == nil {
				t.Errorf("%s: sort inside OR should be an error", expr)
			}
			continue
		}
		if got := order(expr); got != want {
			t.Errorf("%q = %s, want %s", expr, got, want)
		}
	}
	for _, bad := range []string{"sort:nope", "NOT sort:title", "sort:title:sideways", "sort:-"} {
		if _, err := Parse(bad, s); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
	q, _ := Parse("title:a sort:-cf.mass", s)
	if got := q.String(); got != "title:a AND sort:-cf.mass" {
		t.Errorf("String() = %q", got)
	}
	if q2, err := Parse(q.String(), s); err != nil || len(q2.Sort()) != 1 || !q2.Sort()[0].Desc {
		t.Errorf("round trip: %v %v", err, q2.Sort())
	}
	if (*Query)(nil).Sort() != nil {
		t.Error("nil query has no sort")
	}
}
