package suggest

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func values(s []Suggestion) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = x.Value
	}
	return out
}

func h(v string, uses int, ago time.Duration) Candidate {
	return Candidate{Value: v, Uses: uses, LastUsed: now.Add(-ago), Source: SourceHistory}
}

func TestRankClasses(t *testing.T) {
	cands := []Candidate{
		h("The login page opens", 1, 0),       // word start ("login")
		h("Login form is shown", 1, 0),        // prefix
		h("Relogin is offered", 9, 0),         // contains only
		h("Logout works", 5, 0),               // no match
		h("log in with SSO", 1, 0),            // no match ("log in" ≠ "login")
		h("Login", 1, 0),                      // the typed text itself: left out
		h("Användaren loggar in", 1, 0),       // no match
		h("user-login succeeds", 1, 0),        // word start after a hyphen
		h("  login button is enabled ", 3, 0), // prefix, more uses
	}
	got := values(Rank("login", cands, 0, now))
	want := []string{"  login button is enabled ", "Login form is shown", "The login page opens", "user-login succeeds", "Relogin is offered"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestRankTiesFrequencyRecency(t *testing.T) {
	cands := []Candidate{
		h("Step b", 2, 90*24*time.Hour),                   // old: 2/(1+3) = 0.5
		h("Step a", 1, 0),                                 // today: 1
		h("Step c", 1, 0),                                 // same as a: alphabetical
		h("Step d", 4, time.Hour),                         // frequent and recent
		{Value: "Step e", Uses: 3, Source: SourceProject}, // 1.5
	}
	got := strings.Join(values(Rank("step", cands, 0, now)), "|")
	if got != "Step d|Step e|Step a|Step c|Step b" {
		t.Fatalf("got %s", got)
	}
	// same score, the more recent one first
	got = strings.Join(values(Rank("x", []Candidate{h("x2", 1, 0), {Value: "x1", Uses: 2}}, 0, now)), "|")
	if got != "x2|x1" {
		t.Fatalf("recency: got %s", got)
	}
}

func TestRankLimitsAndEmptyQuery(t *testing.T) {
	var cands []Candidate
	for i := range 20 {
		cands = append(cands, h(fmt.Sprintf("value %02d", i), 1, time.Duration(i)*time.Hour))
	}
	if n := len(Rank("val", cands, 0, now)); n != MaxResults {
		t.Fatalf("default limit: %d", n)
	}
	if n := len(Rank("val", cands, 50, now)); n != MaxResults {
		t.Fatalf("limit is capped: %d", n)
	}
	if n := len(Rank("val", cands, 3, now)); n != 3 {
		t.Fatalf("limit 3: %d", n)
	}
	// nothing typed: the most recent values
	got := Rank("", cands, 2, now)
	if strings.Join(values(got), "|") != "value 00|value 01" {
		t.Fatalf("empty query: %v", values(got))
	}
	if got[0].LastUsed == nil || got[0].Source != SourceHistory {
		t.Fatalf("history fields: %+v", got[0])
	}
	if r := Rank(strings.Repeat("a", MaxQueryLen+1), []Candidate{h(strings.Repeat("a", 300), 1, 0)}, 0, now); len(r) != 0 {
		t.Fatalf("over-long query: %v", r)
	}
	if r := Rank("zz", nil, 0, now); r == nil || len(r) != 0 {
		t.Fatalf("no candidates must give an empty list: %#v", r)
	}
}

func TestMerge(t *testing.T) {
	got := Merge([]Candidate{
		{Value: "ISO 26262", Uses: 4, Source: SourceProject},
		h("iso 26262", 1, time.Hour),
		h("ISO 26262 ", 2, 0),
		{Value: "", Uses: 1, Source: SourceProject},
		{Value: "IEC 61508", Uses: 1, Source: SourceProject},
	})
	if len(got) != 2 {
		t.Fatalf("merge: %+v", got)
	}
	m := got[0]
	if m.Uses != 7 || m.Source != SourceHistory || m.Value != "ISO 26262 " || !m.LastUsed.Equal(now) {
		t.Fatalf("merged: %+v", m)
	}
	if got[1].Source != SourceProject {
		t.Fatalf("project value: %+v", got[1])
	}
	r := Rank("iso", got, 0, now)
	if len(r) != 1 || r[0].Source != SourceHistory {
		t.Fatalf("rank of merged: %+v", r)
	}
	r = Rank("iec", got, 0, now)
	if len(r) != 1 || r[0].LastUsed != nil {
		t.Fatalf("project value has no time: %+v", r)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, out string
		ok      bool
	}{
		{"  hello\r\nworld ", "hello\nworld", true},
		{"   ", "", false},
		{strings.Repeat("å", MaxValueLen), strings.Repeat("å", MaxValueLen), true},
		{strings.Repeat("å", MaxValueLen+1), "", false},
		{"bad \xff", "", false},
	}
	for _, c := range cases {
		out, ok := Normalize(c.in)
		if out != c.out || ok != c.ok {
			t.Errorf("Normalize(%q) = %q, %v", c.in, out, ok)
		}
	}
}

func TestMatchClassUnicode(t *testing.T) {
	if c := matchClass("ö", "kör ölet"); c != classWord {
		t.Fatalf("word start after a multi-byte letter: %d", c)
	}
	if c := matchClass("ör", "kör"); c != classContains {
		t.Fatalf("contains: %d", c)
	}
	if c := matchClass("x", "kör"); c != classNone {
		t.Fatalf("none: %d", c)
	}
	// future use (clock skew) counts as now
	if f := frecency(Candidate{Uses: 2, LastUsed: now.Add(time.Hour)}, now); f != 2 {
		t.Fatalf("frecency: %v", f)
	}
}
