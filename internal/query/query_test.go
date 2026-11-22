package query

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

var rec = MapRecord{
	"key":        {"REQ-12"},
	"title":      {"Login fails on timeout"},
	"status":     {"approved"},
	"priority":   {"high"},
	"version":    {"3"},
	"created_at": {"2025-06-01T10:00:00Z"},
	"category":   {"Negative", "Network based"},
	"criteria":   {""},
	"tag":        {"a/b", "x"},
}

func mustParse(t *testing.T, s string, sc *Schema) *Query {
	t.Helper()
	q, err := Parse(s, sc)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return q
}

func TestMatch(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		{"", true},
		{"   ", true},
		{"status:approved priority=high", true},
		{"status:appr", true},
		{"status=appr", false},
		{"STATUS=APPROVED", true},
		{"status:approved AND priority=low", false},
		{"status:approved && priority=low", false},
		{"priority=low OR status:approved", true},
		{"priority=low || status:x", false},
		{"NOT status:obsolete", true},
		{"-status:approved", false},
		{"!status:approved", false},
		{"not not status:approved", true},
		{`title~/^login.*(fail|error)/i`, true},
		{`title~/^login/`, false},
		{`title~"^Login"`, true},
		{`title~^Login`, true},
		{`title!~/TBD/`, true},
		{`title!~/Login/`, false},
		{`"fails on"`, true},
		{`"fails  on"`, false},
		{`/time[- ]?out/`, true},
		{`/TIMEOUT/i`, true},
		{`/TIMEOUT/`, false},
		{"key:REQ-1*", true},
		{"key:req-1?", true},
		{"key:REQ-1?3", false},
		{"key:REQ-?", false},
		{"REQ-1*", true},
		{"version>=2 created_at<2026-01-01", true},
		{"version>3", false},
		{"version>=3", true},
		{"version<=3", true},
		{"version<3", false},
		{"version>10", false},
		{"created_at>2025-01-01", true},
		{"created_at<=2025-01-01", false},
		{"title>abc", false}, // "L" < "a" case-sensitive
		{"category:Negative", true},
		{`category:"Network based"`, true},
		{`category="network BASED"`, true},
		{"category!=Negative", false},
		{"category!=Positive", true},
		{"has:criteria", false},
		{"criteria:*", false},
		{"has:status", true},
		{"status:*", true},
		{`criteria=""`, true},
		{`missing=""`, true},
		{`status=""`, false},
		{`status!=""`, true},
		{"missing:foo", false},
		{"missing!=foo", true},
		{"(priority=low OR status:approved) AND key:REQ", true},
		{"(priority=low OR status:x) AND key:REQ", false},
		{"priority=low OR status:approved AND key:nope", false},
		{"-(priority=low OR status:x)", true},
		{"timeout", true},
		{"nothere", false},
		{"tag:a/b", true},
		{"tag=X", true},
		{"login timeout", true},
		{"login -timeout", false},
		{`"a\"b"`, false},
		{"-", true}, // lone dash is a plain term
	}
	for _, c := range cases {
		q := mustParse(t, c.q, nil)
		if got := q.Match(rec); got != c.want {
			t.Errorf("%q: got %v want %v", c.q, got, c.want)
		}
	}
}

func TestSchema(t *testing.T) {
	sc := &Schema{Fields: []string{"Key", "status", "execution_type", "has"}, Aliases: map[string]string{"Type": "execution_type"}}
	r := MapRecord{"execution_type": {"manual"}, "status": {"x"}}
	q := mustParse(t, "TYPE=manual", sc)
	if !q.Match(r) || q.String() != "execution_type=manual" {
		t.Fatalf("alias: %v %q", q.Match(r), q.String())
	}
	// "has" is a schema field here, so it is a normal condition.
	q = mustParse(t, "has:status", sc)
	if q.Match(r) {
		t.Fatal("has should be a normal field when in schema")
	}
	sc2 := &Schema{Fields: []string{"status"}}
	q = mustParse(t, "has:status", sc2)
	if !q.Match(r) {
		t.Fatal("has:status")
	}
	_, err := Parse("has:bogus", sc2)
	assertErr(t, err, 4, "unknown field")
	_, err = Parse("status:a  bogus:x", sc2)
	assertErr(t, err, 10, "unknown field")
}

func assertErr(t *testing.T, err error, pos int, msg string) {
	t.Helper()
	var qe *Error
	if !errors.As(err, &qe) {
		t.Fatalf("want *Error, got %v", err)
	}
	if qe.Pos != pos || !strings.Contains(qe.Msg, msg) {
		t.Fatalf("got pos=%d msg=%q, want pos=%d msg~%q", qe.Pos, qe.Msg, pos, msg)
	}
	if !strings.HasPrefix(qe.Error(), "query: ") || !strings.HasSuffix(qe.Error(), " at position "+strconv.Itoa(pos)) {
		t.Fatalf("bad Error(): %q", qe.Error())
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		q   string
		pos int
		msg string
	}{
		{"(a OR b", 0, "unclosed"},
		{"a)", 1, "unexpected ')'"},
		{")", 0, "unexpected ')'"},
		{"()", 1, "empty group"},
		{"( OR a)", 2, "unexpected OR"},
		{"a AND", 5, "expected term after AND"},
		{"a OR", 4, "expected term after OR"},
		{"a && )", 5, "expected term after &&"},
		{"NOT", 3, "expected term after NOT"},
		{"OR a", 0, "unexpected OR"},
		{"status:", 7, "missing value"},
		{"status: x", 7, "missing value"},
		{`"abc`, 0, "unterminated quoted"},
		{`title:"abc`, 6, "unterminated quoted"},
		{"/abc", 0, "unterminated regular"},
		{"/abc/x", 5, "invalid regex flag"},
		{"title~/(/", 6, "invalid regular expression"},
		{"title~(", 6, "missing value"},
		{"/(/", 0, "invalid regular expression"},
		{"a!~[", 3, "invalid regular expression"},
	}
	for _, c := range cases {
		_, err := Parse(c.q, nil)
		if err == nil {
			t.Errorf("%q: expected error", c.q)
			continue
		}
		var qe *Error
		if !errors.As(err, &qe) || qe.Pos != c.pos || !strings.Contains(qe.Msg, c.msg) {
			t.Errorf("%q: got %v, want pos %d msg %q", c.q, err, c.pos, c.msg)
		}
	}
}

func TestLimits(t *testing.T) {
	_, err := Parse(strings.Repeat("a", MaxLength+1), nil)
	assertErr(t, err, MaxLength, "too long")
	if _, err := Parse(strings.Repeat("a", MaxLength), nil); err != nil {
		t.Fatal(err)
	}
	var parts []string
	for i := 0; i < MaxRegexps+1; i++ {
		parts = append(parts, "/x/")
	}
	_, err = Parse(strings.Join(parts, " "), nil)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("regex limit: %v", err)
	}
	_, err = Parse(strings.Repeat("(", 200)+"a"+strings.Repeat(")", 200), nil)
	if err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("depth: %v", err)
	}
	_, err = Parse(strings.Repeat("NOT ", 200)+"a", nil)
	if err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("depth not: %v", err)
	}
	// errors propagated from within groups / after operators
	for _, s := range []string{"(a /(/)", "a OR /(/", "a AND /(/", "NOT /(/", "a /(/", "(/(/", "(a OR b) /(/", "a OR b /(/", "NOT a /(/", "a)/(/"} {
		if _, err := Parse(s, nil); err == nil {
			t.Errorf("%q: expected error", s)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	cases := map[string]string{
		"status:approved priority=high":    "status:approved AND priority=high",
		"a OR b c":                         "a OR (b AND c)",
		"(a OR b) c":                       "(a OR b) AND c",
		"-status:x":                        "NOT status:x",
		"NOT (a b)":                        "NOT (a AND b)",
		`title~/a\/b/i`:                    `title~/a\/b/i`,
		`title~"x y"`:                      `title~/x y/`,
		`/re/`:                             `/re/`,
		`has:Key`:                          `key:*`,
		`"exact phrase"`:                   `"exact phrase"`,
		`category:"Network based"`:         `category:"Network based"`,
		`k="a\"b\\c"`:                      `k="a\"b\\c"`,
		`"and"`:                            `"and"`,
		`x=""`:                             `x=""`,
		"version>=2 created_at<2026-01-01": "version>=2 AND created_at<2026-01-01",
		"key:REQ-1*":                       "key:REQ-1*",
		"a:b:c":                            `a:"b:c"`,
		"Ünïcode":                          "Ünïcode",
		"(a)":                              "a",
	}
	for in, want := range cases {
		q := mustParse(t, in, nil)
		got := q.String()
		if got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
		q2 := mustParse(t, got, nil)
		if q2.String() != got {
			t.Errorf("not idempotent: %q -> %q", got, q2.String())
		}
	}
	if mustParse(t, "", nil).String() != "" {
		t.Fatal("empty string")
	}
}

func TestQuote(t *testing.T) {
	for _, s := range []string{"", "plain", "two words", `q"uote`, `back\slash`, "-neg", "!x", "/x/", "OR", "not", "a(b", "a:b", "x=y", "ü ö", "tab\there", "REQ-1", "a&&b"} {
		q := Quote(s)
		p, err := Parse("f="+q, nil)
		if err != nil {
			t.Errorf("Quote(%q)=%q: %v", s, q, err)
			continue
		}
		n := p.root
		if n.kind != nCond || n.value != s {
			t.Errorf("Quote(%q)=%q round-trips to %q", s, q, n.value)
		}
	}
	if Quote("abc") != "abc" || Quote("a b") != `"a b"` {
		t.Fatal("quote basics")
	}
}

func TestTermsAndEmpty(t *testing.T) {
	q := mustParse(t, `login "time out" status:x /re/ -neg login (foo OR bar)`, nil)
	want := []string{"login", "time out", "foo", "bar"}
	if got := q.Terms(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Terms = %v", got)
	}
	e := mustParse(t, " ", nil)
	if !e.IsEmpty() || e.Terms() != nil || e.MatchedFields(rec) != nil || !e.Match(rec) {
		t.Fatal("empty query")
	}
	var nilQ *Query
	if !nilQ.IsEmpty() {
		t.Fatal("nil query")
	}
	if mustParse(t, "a", nil).IsEmpty() {
		t.Fatal("non-empty")
	}
}

// plainRecord does not implement FieldLister.
type plainRecord struct{ m MapRecord }

func (p plainRecord) Values(f string) ([]string, bool) { return p.m.Values(f) }
func (p plainRecord) AllValues() []string              { return p.m.AllValues() }

func TestMatchedFields(t *testing.T) {
	q := mustParse(t, `timeout status:approved priority=low -key:REQ category:neg* title!=x title!~/zzz/ OR /network/i`, nil)
	got := q.MatchedFields(rec)
	want := []string{"category", "status", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchedFields = %v", got)
	}
	got = q.MatchedFields(plainRecord{rec})
	want = []string{"*", "category", "status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchedFields plain = %v", got)
	}
	q = mustParse(t, "zzz", nil)
	if got := q.MatchedFields(plainRecord{rec}); len(got) != 0 {
		t.Fatalf("no match: %v", got)
	}
}

func TestMapRecord(t *testing.T) {
	m := MapRecord{"B": {"2"}, "a": {"1", "1b"}}
	if v, ok := m.Values("b"); !ok || v[0] != "2" {
		t.Fatal("case-insensitive lookup")
	}
	if _, ok := m.Values("zz"); ok {
		t.Fatal("unknown")
	}
	if got := m.AllValues(); !reflect.DeepEqual(got, []string{"2", "1", "1b"}) {
		t.Fatalf("AllValues %v", got)
	}
}

func TestQuotedEscapes(t *testing.T) {
	q := mustParse(t, `title:"on \"time\"" `, nil)
	if q.root.value != `on "time"` {
		t.Fatal(q.root.value)
	}
	// trailing backslash inside an unterminated string
	if _, err := Parse(`"abc\`, nil); err == nil {
		t.Fatal("expected error")
	}
	q = mustParse(t, `/a\\/`, nil)
	if q.root.reSrc != `a\\` {
		t.Fatal(q.root.reSrc)
	}
	if _, err := Parse(`/a\`, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestBind(t *testing.T) {
	r := MapRecord{"owner": {"Anna@Example.com"}, "title": {"me and you"}, "assignee": {"james"}}
	for expr, want := range map[string]bool{
		"owner:me":                   true,
		"owner=me":                   true,
		"owner!=me":                  false,
		"-owner:me":                  false,
		"assignee:me":                false, // "james" contains "me" but is not the caller
		"title:me":                   true,  // not bound: plain contains
		"owner:me AND assignee:jam":  true,
		"(owner:me OR assignee:me)":  true,
		"owner:/me/":                 false,
		"owner:anna":                 true,
		"NOT (owner:me) OR title:me": true,
	} {
		q, err := Parse(expr, nil)
		if err != nil {
			t.Fatal(expr, err)
		}
		q.Bind([]string{"owner", "assignee"}, "me", []string{"anna@example.com", " ", "anna"})
		if got := q.Match(r); got != want {
			t.Errorf("%s: got %v want %v", expr, got, want)
		}
	}
	var empty *Query
	empty.Bind([]string{"owner"}, "me", nil)
	q, _ := Parse("owner:me", nil)
	q.Bind([]string{"owner"}, "me", nil)
	if q.Match(r) {
		t.Error("no names: matches nothing")
	}
}
