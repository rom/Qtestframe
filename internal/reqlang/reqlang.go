// Package reqlang analyses requirement text: it detects obligation keywords
// (RFC 2119 / RFC 8174 in English, the corresponding terms in other
// languages) and lints the text for common quality problems (ambiguity,
// TBDs, escape clauses, ...). Languages are defined as packs in
// languages.go; the language of a text can be given or detected.
//
// All offsets and lengths are byte offsets into the analysed text.
package reqlang

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Obligation levels.
const (
	LevelMust      = "must"
	LevelMustNot   = "must_not"
	LevelShall     = "shall"
	LevelShallNot  = "shall_not"
	LevelShould    = "should"
	LevelShouldNot = "should_not"
	LevelMay       = "may"
	LevelOptional  = "optional"
	LevelWill      = "will"
	LevelDescribe  = "describe"
)

// Levels lists all obligation levels from strongest to weakest.
var Levels = []string{
	LevelMustNot, LevelMust, LevelShallNot, LevelShall, LevelShouldNot,
	LevelShould, LevelMay, LevelOptional, LevelWill, LevelDescribe,
}

// Severities.
const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// Rule identifiers.
const (
	RuleMissingKeyword      = "missing-keyword"
	RuleLowercaseKeyword    = "lowercase-keyword"
	RuleMultipleObligations = "multiple-obligations"
	RuleAmbiguousTerm       = "ambiguous-term"
	RuleTBD                 = "tbd"
	RuleUnboundedTerm       = "unbounded-term"
	RuleAndOr               = "and-or"
	RulePassiveCapability   = "passive-capability"
	RuleLongSentence        = "long-sentence"
	RuleNegativeRequirement = "negative-requirement"
	RuleEscapeClause        = "escape-clause"
	// RuleAmbiguousNegation flags negations whose meaning is commonly
	// misread, e.g. Swedish "måste inte" ("need not", not "must not").
	RuleAmbiguousNegation = "ambiguous-negation"
)

// LongSentenceWords is the word count above which a sentence is "long".
const LongSentenceWords = 40

// Keyword is a detected obligation keyword.
type Keyword struct {
	Keyword string `json:"keyword"`
	Level   string `json:"level"`
	Offset  int    `json:"offset"`
	Length  int    `json:"length"`
}

// Issue is a lint finding.
type Issue struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"` // English; clients translate by Rule + Params
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	// Params are the values inserted into the message: "term" (the text
	// found), "keyword" (canonical keyword), "keywords" (comma separated),
	// "words" and "max" (long sentences), "examples" (missing keyword).
	Params map[string]string `json:"params,omitempty"`
}

// Analysis is the result of Analyze.
type Analysis struct {
	Language   string    `json:"language"` // language pack used, e.g. "en", "sv"
	Obligation string    `json:"obligation"`
	Keywords   []Keyword `json:"keywords"`
	Issues     []Issue   `json:"issues"`
	Score      int       `json:"score"`
}

type phrase struct {
	text          string // canonical form
	level         string // keywords only
	caseSensitive bool   // match exactly as written (e.g. RFC keywords inside another language)
	re            *regexp.Regexp
}

func compileWords(words []string) []phrase {
	pairs := make([]kw, len(words))
	for i, w := range words {
		pairs[i] = kw{text: w}
	}
	return compile(pairs)
}

func compile(pairs []kw) []phrase {
	out := make([]phrase, 0, len(pairs))
	for _, p := range pairs {
		parts := strings.Fields(p.text)
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		flags := `(?i)`
		if p.caseSensitive {
			flags = ``
		}
		out = append(out, phrase{
			text:          p.text,
			level:         p.level,
			caseSensitive: p.caseSensitive,
			re:            regexp.MustCompile(flags + strings.Join(parts, `\s+`)),
		})
	}
	// longest first so that multi-word phrases claim their range first
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].text) > len(out[j].text) })
	return out
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// bounded reports whether text[s:e] is delimited by non-word runes where the
// match itself starts / ends with a word rune.
func bounded(text string, s, e int) bool {
	first, _ := utf8.DecodeRuneInString(text[s:])
	if isWordRune(first) && s > 0 {
		prev, _ := utf8.DecodeLastRuneInString(text[:s])
		if isWordRune(prev) {
			return false
		}
	}
	last, _ := utf8.DecodeLastRuneInString(text[:e])
	if isWordRune(last) && e < len(text) {
		next, _ := utf8.DecodeRuneInString(text[e:])
		if isWordRune(next) {
			return false
		}
	}
	return true
}

type match struct {
	p    *phrase
	s, e int
}

// findAll returns the non-overlapping, word-bounded matches of the table's
// phrases (longest phrase wins), sorted by offset.
func findAll(text string, table []phrase) []match {
	var out []match
	overlaps := func(s, e int) bool {
		for _, m := range out {
			if s < m.e && m.s < e {
				return true
			}
		}
		return false
	}
	for i := range table {
		p := &table[i]
		for _, loc := range p.re.FindAllStringIndex(text, -1) {
			if bounded(text, loc[0], loc[1]) && !overlaps(loc[0], loc[1]) {
				out = append(out, match{p: p, s: loc[0], e: loc[1]})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].s < out[j].s })
	return out
}

func isUpper(s string) bool {
	return s == strings.ToUpper(s) && s != strings.ToLower(s)
}

type sentence struct{ s, e int }

// sentences splits text into sentences on . ! ? followed by whitespace/end
// and on line breaks. Leading/trailing whitespace is trimmed from ranges.
func sentences(text string) []sentence {
	var out []sentence
	start := 0
	add := func(s, e int) {
		for s < e && isSpaceByte(text[s]) {
			s++
		}
		for e > s && isSpaceByte(text[e-1]) {
			e--
		}
		if e > s {
			out = append(out, sentence{s, e})
		}
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\n':
			add(start, i)
			start = i + 1
		case (c == '.' || c == '!' || c == '?') && (i+1 == len(text) || isSpaceByte(text[i+1])):
			add(start, i+1)
			start = i + 1
		}
	}
	add(start, len(text))
	return out
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func rank(level string) int {
	for i, l := range Levels {
		if l == level {
			return i
		}
	}
	return len(Levels)
}

// DetectObligation returns the strongest obligation level in text ("" if
// none), detecting the language. It equals Analyze(text).Obligation.
func DetectObligation(text string) string {
	return Analyze(text).Obligation
}

// Analyze detects the language, keywords and lints the text.
func Analyze(text string) Analysis {
	return AnalyzeLang(text, "")
}

// AnalyzeLang analyses text with the given language pack; "" or "auto"
// detects the language. Unknown languages fall back to detection.
func AnalyzeLang(text, lang string) Analysis {
	a := Analysis{Keywords: []Keyword{}, Issues: []Issue{}}
	if strings.TrimSpace(text) == "" {
		a.Language = lang
		if packs[lang] == nil {
			a.Language = ""
		}
		return a
	}
	pk := packs[lang]
	if pk == nil {
		pk = packs[Detect(text)]
	}
	a.Language = pk.code
	issue := func(rule, sev, msg string, s, e int, params map[string]string) {
		a.Issues = append(a.Issues, Issue{Rule: rule, Severity: sev, Message: msg, Offset: s, Length: e - s, Params: params})
	}

	// Keywords.
	for _, m := range findAll(text, pk.keywords) {
		kw := Keyword{Keyword: m.p.text, Level: m.p.level, Offset: m.s, Length: m.e - m.s}
		a.Keywords = append(a.Keywords, kw)
		if a.Obligation == "" || rank(kw.Level) < rank(a.Obligation) {
			a.Obligation = kw.Level
		}
		word := text[m.s:m.e]
		if pk.uppercaseRule && !m.p.caseSensitive && !isUpper(word) {
			issue(RuleLowercaseKeyword, SeverityInfo,
				fmt.Sprintf("%q is not in uppercase; per RFC 8174 only uppercase %s has normative meaning", word, m.p.text), m.s, m.e,
				map[string]string{"term": word, "keyword": m.p.text})
		}
		if kw.Level == LevelMustNot || kw.Level == LevelShallNot {
			issue(RuleNegativeRequirement, SeverityInfo,
				fmt.Sprintf("negative requirement (%s) is harder to verify; consider stating what the system does instead", m.p.text), m.s, m.e,
				map[string]string{"term": word, "keyword": m.p.text})
		}
	}
	if len(a.Keywords) == 0 {
		issue(RuleMissingKeyword, SeverityWarning,
			fmt.Sprintf("no obligation keyword (%s, ...) found", pk.examples), 0, len(text),
			map[string]string{"examples": pk.examples})
	}

	// Sentence-level rules.
	for _, s := range sentences(text) {
		distinct := map[string]bool{}
		var names []string
		for _, k := range a.Keywords {
			if k.Offset >= s.s && k.Offset < s.e && k.Level != LevelDescribe && !distinct[k.Keyword] {
				distinct[k.Keyword] = true
				names = append(names, k.Keyword)
			}
		}
		if len(names) > 1 {
			list := strings.Join(names, ", ")
			issue(RuleMultipleObligations, SeverityWarning,
				fmt.Sprintf("sentence contains several obligation keywords (%s); consider splitting it into separate requirements", list), s.s, s.e,
				map[string]string{"keywords": list})
		}
		if n := len(strings.Fields(text[s.s:s.e])); n > LongSentenceWords {
			issue(RuleLongSentence, SeverityInfo,
				fmt.Sprintf("sentence has %d words (more than %d); consider splitting it", n, LongSentenceWords), s.s, s.e,
				map[string]string{"words": fmt.Sprint(n), "max": fmt.Sprint(LongSentenceWords)})
		}
	}

	// Term rules.
	termRules := []struct {
		table []phrase
		rule  string
		sev   string
		msg   string
	}{
		{pk.ambiguous, RuleAmbiguousTerm, SeverityWarning, "ambiguous term %q is not verifiable; quantify it"},
		{pk.tbd, RuleTBD, SeverityError, "placeholder %q: requirement is incomplete"},
		{pk.unbounded, RuleUnboundedTerm, SeverityInfo, "unbounded term %q; check that it is really meant universally and can be verified"},
		{pk.andOr, RuleAndOr, SeverityWarning, "%q is ambiguous; state whether both or either are required"},
		{pk.passive, RulePassiveCapability, SeverityInfo, "%q describes a capability rather than a behaviour; state what the system does"},
		{pk.escape, RuleEscapeClause, SeverityWarning, "escape clause %q weakens the requirement; make the condition explicit"},
		{pk.ambiguousNegation, RuleAmbiguousNegation, SeverityWarning, "%q is often misread; say explicitly whether it is a prohibition or not required"},
	}
	for _, tr := range termRules {
		for _, m := range findAll(text, tr.table) {
			term := text[m.s:m.e]
			issue(tr.rule, tr.sev, fmt.Sprintf(tr.msg, term), m.s, m.e, map[string]string{"term": term})
		}
	}
	sort.SliceStable(a.Issues, func(i, j int) bool {
		x, y := a.Issues[i], a.Issues[j]
		if x.Offset != y.Offset {
			return x.Offset < y.Offset
		}
		if x.Length != y.Length {
			return x.Length > y.Length
		}
		return x.Rule < y.Rule
	})

	a.Score = 100
	for _, is := range a.Issues {
		switch is.Severity {
		case SeverityError:
			a.Score -= 25
		case SeverityWarning:
			a.Score -= 10
		default:
			a.Score -= 2
		}
	}
	if a.Score < 0 {
		a.Score = 0
	}
	return a
}
