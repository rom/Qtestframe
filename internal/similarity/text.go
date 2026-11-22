package similarity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/reqlang"
)

// Text analysis: words with their byte offsets, concepts (synonyms and
// abbreviations), stop words and stems, and the features a field of an
// item contributes to its vector.

// word is a word of a text: its lower-case form and byte range.
type word struct {
	lower      string
	start, end int
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// words splits text into words (letters and digits; an apostrophe inside
// a word belongs to it).
func words(text string) []word {
	var out []word
	start := -1
	for i, r := range text {
		in := isWordRune(r) || (r == '\'' && start >= 0)
		if in && start < 0 {
			start = i
		}
		if !in && start >= 0 {
			out = append(out, word{strings.ToLower(strings.TrimRight(text[start:i], "'")), start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, word{strings.ToLower(strings.TrimRight(text[start:], "'")), start, len(text)})
	}
	return out
}

// term is an analysed token: a concept, a stem or a number.
type term struct {
	key        string // feature key: c:<concept>, w:<stem>, n:<number>
	surface    string // lower-case surface form (n-grams)
	start, end int
	content    bool // counts for the content overlap of statements
}

func isNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

// Language returns the language used to analyse text ("en" or "sv").
func Language(text string) string {
	if l := reqlang.Detect(text); l == "sv" {
		return "sv"
	}
	return "en"
}

// terms analyses text in lang: concepts replace their phrases, stop words
// are dropped, other words are stemmed.
func terms(text, lang string) []term {
	ws := words(text)
	var out []term
	for i := 0; i < len(ws); i++ {
		w := ws[i]
		matched := false
		for _, p := range conceptIndex[w.lower] {
			if i+len(p.words) > len(ws) {
				continue
			}
			ok := true
			for k, pw := range p.words {
				if ws[i+k].lower != pw {
					ok = false
					break
				}
			}
			if ok {
				last := ws[i+len(p.words)-1]
				surface := w.lower
				if len(p.words) > 1 {
					surface = ""
				}
				out = append(out, term{key: "c:" + p.concept, surface: surface, start: w.start, end: last.end, content: true})
				i += len(p.words) - 1
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		switch {
		case isNumber(w.lower):
			out = append(out, term{key: "n:" + strings.TrimLeft(w.lower, "0"), start: w.start, end: w.end})
		case numberWords[w.lower] != "":
			out = append(out, term{key: "n:" + numberWords[w.lower], start: w.start, end: w.end})
		case stopWords[w.lower] || negations[w.lower]:
		case utf8.RuneCountInString(w.lower) < 2:
		default:
			if parts := decompose(w.lower, lang); parts != nil {
				for k, p := range parts {
					t := term{key: p, start: w.start, end: w.end, content: true}
					if k == 0 {
						t.surface = w.lower // the n-grams of the whole word once
					}
					out = append(out, t)
				}
				continue
			}
			out = append(out, term{key: "w:" + Stem(lang, w.lower), surface: w.lower, start: w.start, end: w.end, content: true})
		}
	}
	return out
}

// singleConcept is the concept of a one-word phrase ("" if none).
func singleConcept(w string) string {
	for _, p := range conceptIndex[w] {
		if len(p.words) == 1 {
			return p.concept
		}
	}
	return ""
}

// decompose splits a compound whose first or last part is a concept
// ("användarinloggning" = user + login, "lösenordslängd" = password +
// längd). It returns the feature keys of the parts, or nil.
func decompose(w, lang string) []string {
	r := []rune(w)
	if len(r) < 7 {
		return nil
	}
	part := func(s string) string {
		if c := singleConcept(s); c != "" {
			return "c:" + c
		}
		return "w:" + Stem(lang, s)
	}
	// the first part that is a concept (with a linking s, or without the
	// final e/a of its base form: användar|e); a split whose last part is
	// a concept too wins, then the longer first part
	best, bestScore := []string(nil), 0
	for i := len(r) - 3; i >= 4; i-- {
		type cand struct{ head, rest string }
		cands := []cand{{string(r[:i]), string(r[i:])}, {string(r[:i]) + "e", string(r[i:])}, {string(r[:i]) + "a", string(r[i:])}}
		if r[i-1] == 's' {
			cands = append(cands, cand{string(r[:i-1]), string(r[i:])})
		}
		for _, c := range cands {
			hc := singleConcept(c.head)
			if hc == "" {
				continue
			}
			score := 1
			if singleConcept(c.rest) != "" {
				score = 2
			}
			if score > bestScore {
				best, bestScore = []string{"c:" + hc, part(c.rest)}, score
			}
		}
	}
	if best != nil {
		return best
	}
	// the longest last part that is a concept
	for i := 3; i <= len(r)-4; i++ {
		head, rest := string(r[:i]), string(r[i:])
		if c := singleConcept(rest); c != "" {
			h := head
			if strings.HasSuffix(h, "s") && len([]rune(h)) > 3 {
				h = strings.TrimSuffix(h, "s")
			}
			return []string{part(h), "c:" + c}
		}
	}
	return nil
}

// Weights of the features of a word.
const (
	wordWeight   = 1.0
	numberWeight = 0.5
	// gramWeight is the weight of the character n-grams of a word in all;
	// they let compounds ("lösenordsbyte") match their parts ("byte av
	// lösenord").
	gramWeight = 0.6
	gramSize   = 4
)

// features adds the features of a text to f (weights summed).
func features(text, lang string, f map[string]float64) {
	for _, t := range terms(text, lang) {
		w := wordWeight
		if strings.HasPrefix(t.key, "n:") {
			w = numberWeight
		}
		f[t.key] += w
		grams := ngrams(t.surface)
		for _, g := range grams {
			f["g:"+g] += gramWeight / float64(len(grams))
		}
	}
}

// ngrams are the character n-grams of a word of at least gramSize letters
// (none for shorter words).
func ngrams(s string) []string {
	r := []rune(s)
	if len(r) < gramSize {
		return nil
	}
	out := make([]string, 0, len(r)-gramSize+1)
	for i := 0; i+gramSize <= len(r); i++ {
		out = append(out, string(r[i:i+gramSize]))
	}
	return out
}

// sentence is a byte range of a text.
type sentence struct{ start, end int }

// sentences splits text on . ! ? ; followed by a space or the end, and on
// line breaks (decimals such as 2.5 stay in one sentence).
func sentences(text string) []sentence {
	var out []sentence
	start := 0
	add := func(s, e int) {
		for s < e && (text[s] == ' ' || text[s] == '\t' || text[s] == '\r' || text[s] == '\n') {
			s++
		}
		for e > s && (text[e-1] == ' ' || text[e-1] == '\t' || text[e-1] == '\r' || text[e-1] == '\n') {
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
		case (c == '.' || c == '!' || c == '?' || c == ';') && (i+1 == len(text) || text[i+1] == ' ' || text[i+1] == '\n' || text[i+1] == '\t' || text[i+1] == '\r'):
			add(start, i+1)
			start = i + 1
		}
	}
	add(start, len(text))
	return out
}
