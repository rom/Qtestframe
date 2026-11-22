package similarity

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Numeric thresholds in requirement text: a comparison ("at least",
// "högst", "≤", "within" …), a number and a unit ("s", "sekunder",
// "characters", "tecken", "MB", "%" …). Values are converted to a base
// unit per dimension so that "2 s" and "2000 ms" compare.

// Comparison operators.
const (
	opLE = "<="
	opLT = "<"
	opGE = ">="
	opGT = ">"
	opEQ = "="
)

type comparator struct {
	phrase string
	op     string
	// negated: the phrase already contains its negation ("no more
	// than"); a negative obligation does not reverse it.
	negated bool
}

// comparators before the number, longest first when matched.
var comparators = func() []comparator {
	list := []comparator{
		{"≤", opLE, false}, {"<=", opLE, false}, {"=<", opLE, false}, {"≥", opGE, false}, {">=", opGE, false}, {"=>", opGE, false},
		{"<", opLT, false}, {">", opGT, false}, {"=", opEQ, false},
		// English
		{"at most", opLE, false}, {"no more than", opLE, true}, {"not more than", opLE, true}, {"no longer than", opLE, true},
		{"not longer than", opLE, true}, {"no later than", opLE, true}, {"maximum", opLE, false}, {"maximum of", opLE, false},
		{"a maximum of", opLE, false}, {"max", opLE, false}, {"max.", opLE, false}, {"up to", opLE, false}, {"within", opLE, false},
		{"less than or equal to", opLE, false}, {"at the most", opLE, false}, {"no slower than", opLE, true},
		{"less than", opLT, false}, {"fewer than", opLT, false}, {"below", opLT, false}, {"under", opLT, false},
		{"shorter than", opLT, false}, {"faster than", opLT, false}, {"lower than", opLT, false},
		{"at least", opGE, false}, {"no less than", opGE, true}, {"not less than", opGE, true}, {"no fewer than", opGE, true},
		{"not fewer than", opGE, true}, {"no shorter than", opGE, true}, {"minimum", opGE, false}, {"minimum of", opGE, false},
		{"a minimum of", opGE, false}, {"min", opGE, false}, {"min.", opGE, false}, {"greater than or equal to", opGE, false},
		{"more than", opGT, false}, {"greater than", opGT, false}, {"longer than", opGT, false}, {"over", opGT, false},
		{"above", opGT, false}, {"exceed", opGT, false}, {"exceeds", opGT, false}, {"higher than", opGT, false},
		{"exactly", opEQ, false}, {"equal to", opEQ, false},
		// Swedish
		{"högst", opLE, false}, {"maximalt", opLE, false}, {"som mest", opLE, false}, {"upp till", opLE, false},
		{"inom", opLE, false}, {"inte mer än", opLE, true}, {"inte fler än", opLE, true}, {"inte längre än", opLE, true},
		{"senast inom", opLE, false}, {"mindre än eller lika med", opLE, false}, {"som högst", opLE, false},
		{"mindre än", opLT, false}, {"färre än", opLT, false}, {"kortare än", opLT, false}, {"snabbare än", opLT, false},
		{"lägre än", opLT, false}, {"understiga", opLT, false}, {"understiger", opLT, false},
		{"minst", opGE, false}, {"minimalt", opGE, false}, {"som minst", opGE, false}, {"inte mindre än", opGE, true},
		{"inte färre än", opGE, true}, {"inte kortare än", opGE, true}, {"lägst", opGE, false},
		{"mer än", opGT, false}, {"fler än", opGT, false}, {"längre än", opGT, false}, {"över", opGT, false},
		{"överstiga", opGT, false}, {"överstiger", opGT, false}, {"överskrida", opGT, false}, {"överskrider", opGT, false},
		{"högre än", opGT, false}, {"större än", opGT, false},
		{"exakt", opEQ, false}, {"precis", opEQ, false}, {"lika med", opEQ, false},
	}
	// longest phrase first
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && len(list[j].phrase) > len(list[j-1].phrase); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	return list
}()

// postfix comparators after the unit ("8 characters or more").
var postfixes = []comparator{
	{"or more", opGE, false}, {"or longer", opGE, false}, {"or greater", opGE, false}, {"or less", opLE, false},
	{"or fewer", opLE, false}, {"or shorter", opLE, false},
	{"eller mer", opGE, false}, {"eller fler", opGE, false}, {"eller längre", opGE, false}, {"eller mindre", opLE, false},
	{"eller färre", opLE, false}, {"eller kortare", opLE, false},
}

type unit struct {
	dim    string
	factor float64
}

// units by their (lower-case) word; the dimension and the factor to the
// base unit (seconds, bytes, characters …).
var units = func() map[string]unit {
	m := map[string]unit{}
	add := func(dim string, factor float64, words ...string) {
		for _, w := range words {
			m[w] = unit{dim, factor}
		}
	}
	add("time", 0.001, "ms", "msec", "millisecond", "milliseconds", "millisekund", "millisekunder", "millisekunden", "millisekunders")
	add("time", 1, "s", "sec", "secs", "second", "seconds", "sekund", "sekunder", "sekunden", "sekunders", "sek")
	add("time", 60, "min", "mins", "minute", "minutes", "minut", "minuter", "minuten", "minuters")
	add("time", 3600, "h", "hr", "hrs", "hour", "hours", "timme", "timmar", "timmen", "timmars", "tim")
	add("time", 86400, "day", "days", "dag", "dagar", "dygn", "dagen", "dagars", "dygns")
	add("time", 604800, "week", "weeks", "vecka", "veckor", "veckan", "veckors")
	add("time", 2592000, "month", "months", "månad", "månader", "månaden", "månaders")
	add("time", 31536000, "year", "years", "år", "året", "års")
	add("chars", 1, "character", "characters", "char", "chars", "tecken", "tecknen", "letters", "bokstäver", "symbols")
	add("digits", 1, "digit", "digits", "siffra", "siffror")
	add("bytes", 1, "b", "byte", "bytes")
	add("bytes", 1e3, "kb", "kilobyte", "kilobytes")
	add("bytes", 1024, "kib")
	add("bytes", 1e6, "mb", "megabyte", "megabytes")
	add("bytes", 1<<20, "mib")
	add("bytes", 1e9, "gb", "gigabyte", "gigabytes")
	add("bytes", 1<<30, "gib")
	add("bytes", 1e12, "tb", "terabyte", "terabytes")
	add("percent", 1, "%", "percent", "procent")
	return m
}()

// constraint is a numeric threshold found in a statement.
type constraint struct {
	op         string
	value      float64 // in the base unit
	dim        string
	start, end int // byte range in the statement (comparison to unit)
	ok         bool
}

// numberRE: a number (decimal point or comma) with an optional unit.
var numberRE = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*(%|[\p{L}]+\.?)?`)

// parseConstraints finds the thresholds in a statement; negative reverses
// comparisons without their own negation ("must not exceed 2 s" = ≤ 2 s).
func parseConstraints(s string, negative bool) []constraint {
	var out []constraint
	lower := strings.ToLower(s)
	for _, m := range numberRE.FindAllStringSubmatchIndex(lower, -1) {
		if m[0] > 0 && isWordRuneBefore(lower, m[0]) {
			continue // part of a word or identifier (ISO27001)
		}
		num := strings.ReplaceAll(lower[m[2]:m[3]], ",", ".")
		v, err := strconv.ParseFloat(num, 64)
		if err != nil {
			continue
		}
		c := constraint{start: m[0], end: m[3]}
		if m[4] >= 0 {
			word := strings.TrimSuffix(lower[m[4]:m[5]], ".")
			if u, ok := units[word]; ok {
				c.dim, v = u.dim, v*u.factor
				c.end = m[4] + len(word)
			} else if !stopWords[word] && !isNumber(word) {
				// a count of things: "5 attempts", "3 försök"
				c.dim = "count:" + countKey(word)
				c.end = m[5]
			}
		}
		if c.dim == "" {
			continue
		}
		c.value = v
		// comparison before the number
		before := strings.TrimRight(lower[:m[0]], " \t")
		for _, cp := range comparators {
			if strings.HasSuffix(before, cp.phrase) && phraseStart(before, len(before)-len(cp.phrase)) {
				c.op, c.start = cp.op, len(before)-len(cp.phrase)
				if negative && !cp.negated {
					c.op = reverse(c.op)
				}
				break
			}
		}
		// or after the unit
		if c.op == "" {
			after := strings.TrimLeft(lower[c.end:], " \t")
			for _, cp := range postfixes {
				if strings.HasPrefix(after, cp.phrase) {
					c.op = cp.op
					c.end = c.end + (len(lower[c.end:]) - len(after)) + len(cp.phrase)
					if negative {
						c.op = reverse(c.op)
					}
					break
				}
			}
		}
		if c.op == "" {
			continue
		}
		c.ok = true
		out = append(out, c)
	}
	return out
}

// countKey names the things counted ("attempts" and "försök" are both
// attempts).
func countKey(w string) string {
	for _, p := range conceptIndex[w] {
		if len(p.words) == 1 {
			return p.concept
		}
	}
	return Stem(Language(w), w)
}

func isWordRuneBefore(s string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		c := s[j]
		if c < 0x80 {
			return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
		}
		if c >= 0xC0 { // start of a multi-byte letter
			return true
		}
	}
	return false
}

// phraseStart: the phrase at i in s starts a word (or is a symbol).
func phraseStart(s string, i int) bool {
	if i <= 0 {
		return true
	}
	first := s[i]
	if !(first >= 'a' && first <= 'z') && first < 0x80 {
		return true // a symbol such as ≤ or <
	}
	return !isWordRuneBefore(s, i)
}

func reverse(op string) string {
	switch op {
	case opLE:
		return opGT
	case opLT:
		return opGE
	case opGE:
		return opLT
	case opGT:
		return opLE
	}
	return "" // not equal: no interval
}

// interval of the values a constraint allows (open ends by excl).
type interval struct {
	lo, hi         float64
	loExcl, hiExcl bool
}

func (c constraint) interval() (interval, bool) {
	switch c.op {
	case opLE:
		return interval{lo: math.Inf(-1), hi: c.value}, true
	case opLT:
		return interval{lo: math.Inf(-1), hi: c.value, hiExcl: true}, true
	case opGE:
		return interval{lo: c.value, hi: math.Inf(1)}, true
	case opGT:
		return interval{lo: c.value, hi: math.Inf(1), loExcl: true}, true
	case opEQ:
		return interval{lo: c.value, hi: c.value}, true
	}
	return interval{}, false
}

// disjoint reports whether two thresholds on one quantity cannot both
// hold.
func disjoint(a, b constraint) bool {
	ia, ok1 := a.interval()
	ib, ok2 := b.interval()
	if !ok1 || !ok2 || a.dim != b.dim {
		return false
	}
	lo, loExcl := ia.lo, ia.loExcl
	if ib.lo > lo || (ib.lo == lo && ib.loExcl) {
		lo, loExcl = ib.lo, ib.loExcl
	}
	hi, hiExcl := ia.hi, ia.hiExcl
	if ib.hi < hi || (ib.hi == hi && ib.hiExcl) {
		hi, hiExcl = ib.hi, ib.hiExcl
	}
	if lo > hi {
		return true
	}
	return lo == hi && (loExcl || hiExcl)
}
