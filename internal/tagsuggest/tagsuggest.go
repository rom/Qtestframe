// Package tagsuggest proposes tags for a requirement or test from its title
// and description.
//
// Tags the project already uses come first when the text mentions them
// (also in a simple inflected form, e.g. "passwords" for the tag
// "password", or "lösenordet" for "lösenord"); then come significant words
// of the text: acronyms (GDPR, SSO), words of the title and words that occur
// more than once. Stop words of English and Swedish, requirement keywords
// (MUST, SKA …) and placeholders (TBD) are never suggested.
package tagsuggest

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Reasons for a suggestion.
const (
	ReasonExisting = "existing" // a tag already used in the project
	ReasonAcronym  = "acronym"  // an upper-case abbreviation in the text
	ReasonKeyword  = "keyword"  // a significant word of the text
)

// Suggestion is one proposed tag.
type Suggestion struct {
	Tag    string  `json:"tag"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score"`
	// Uses is how many items of the project carry the tag (existing tags only).
	Uses int `json:"uses,omitempty"`
}

// DefaultLimit is the number of suggestions returned when limit <= 0.
const DefaultLimit = 8

// MaxTagLength mirrors the longest tag the editors accept.
const MaxTagLength = 64

type token struct {
	raw, lower string
	title      bool
}

func tokenize(s string, title bool, out []token) []token {
	start := -1
	runes := []rune(s)
	flush := func(end int) {
		if start < 0 {
			return
		}
		w := strings.Trim(string(runes[start:end]), "-_.")
		start = -1
		if w != "" {
			out = append(out, token{raw: w, lower: strings.ToLower(w), title: title})
		}
	}
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || ((r == '-' || r == '_' || r == '.') && start >= 0 && i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1]))) {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(runes))
	return out
}

// stem reduces a word to a crude common form so that inflected words match
// a tag ("passwords" → "password", "lösenordet" → "lösenord").
func stem(w string) string {
	w = strings.ToLower(w)
	if len([]rune(w)) <= 4 {
		return w
	}
	for _, suf := range []string{"ernas", "arna", "orna", "erna", "ies", "ing", "es", "ed", "en", "et", "er", "ar", "or", "na", "s"} {
		if strings.HasSuffix(w, suf) && len([]rune(w))-len([]rune(suf)) >= 4 {
			base := strings.TrimSuffix(w, suf)
			if suf == "ies" {
				base += "y"
			}
			return base
		}
	}
	return w
}

func isAcronym(w string) bool {
	n := 0
	for _, r := range w {
		switch {
		case unicode.IsUpper(r):
			n++
		case unicode.IsDigit(r):
		default:
			return false
		}
	}
	return n >= 2 && len([]rune(w)) <= 8
}

func isNumber(w string) bool {
	for _, r := range w {
		if !unicode.IsDigit(r) && r != '.' && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// normalize turns a word or phrase into tag form: lower case, spaces → "-".
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), "-")
	if r := []rune(s); len(r) > MaxTagLength {
		s = string(r[:MaxTagLength])
	}
	return s
}

// Suggest proposes up to limit tags for an item with the given title and
// description. existing maps the project's tags to the number of items
// that use them; current lists the item's own tags, which are never
// suggested again.
func Suggest(title, description string, existing map[string]int, current []string, limit int) []Suggestion {
	if limit <= 0 {
		limit = DefaultLimit
	}
	have := map[string]bool{}
	for _, t := range current {
		have[normalize(t)] = true
	}
	toks := tokenize(title, true, nil)
	toks = tokenize(description, false, toks)
	text := " " + strings.Join(lowerWords(toks), " ") + " "

	byStem := map[string][]token{}
	for _, t := range toks {
		byStem[stem(t.lower)] = append(byStem[stem(t.lower)], t)
	}

	out := map[string]*Suggestion{}
	// add records a suggestion; existing tags keep their spelling, other
	// words are put into tag form. Duplicates are found case-insensitively.
	add := func(tag, reason string, score float64, uses int) {
		key := normalize(tag)
		if reason != ReasonExisting {
			tag = key
		}
		if key == "" || have[key] {
			return
		}
		if s, ok := out[key]; ok {
			if score > s.Score {
				s.Score, s.Reason = score, reason
			}
			if reason == ReasonExisting {
				s.Tag = tag
			}
			if uses > s.Uses {
				s.Uses = uses
			}
			return
		}
		out[key] = &Suggestion{Tag: tag, Reason: reason, Score: score, Uses: uses}
	}

	// 1. existing project tags mentioned in the text
	for tag, uses := range existing {
		norm := normalize(tag)
		if norm == "" {
			continue
		}
		words := strings.FieldsFunc(norm, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
		hit, inTitle := false, false
		if len(words) > 1 {
			if strings.Contains(text, " "+strings.Join(words, " ")+" ") || strings.Contains(text, " "+norm+" ") {
				hit = true
				inTitle = strings.Contains(" "+strings.Join(lowerWords(tokenize(title, true, nil)), " ")+" ", " "+strings.Join(words, " ")+" ")
			}
		} else if ts, ok := byStem[stem(norm)]; ok {
			hit = true
			for _, t := range ts {
				inTitle = inTitle || t.title
			}
		}
		if !hit {
			continue
		}
		score := 10 + math.Log1p(float64(uses))
		if inTitle {
			score += 3
		}
		add(tag, ReasonExisting, score, uses)
	}

	// 2. acronyms and significant words
	freq := map[string]float64{}
	first := map[string]string{}
	acr := map[string]bool{}
	for _, t := range toks {
		if stopword(t.lower) || isNumber(t.lower) {
			continue
		}
		if isAcronym(t.raw) {
			acr[t.lower] = true
		} else if len([]rune(t.lower)) < 4 {
			continue
		}
		key := stem(t.lower)
		w := 1.0
		if t.title {
			w = 2
		}
		freq[key] += w
		if _, ok := first[key]; !ok || len(t.lower) < len(first[key]) {
			first[key] = t.lower
		}
	}
	for key, f := range freq {
		word := first[key]
		switch {
		case acr[word]:
			add(word, ReasonAcronym, 5+f, 0)
		case f >= 2:
			add(word, ReasonKeyword, f+math.Min(float64(len([]rune(word)))/10, 1), 0)
		}
	}

	list := make([]Suggestion, 0, len(out))
	for _, s := range out {
		s.Score = math.Round(s.Score*100) / 100
		list = append(list, *s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return list[i].Tag < list[j].Tag
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func lowerWords(ts []token) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.lower
	}
	return out
}

func stopword(w string) bool { return stopwords[w] }

var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, list := range []string{
		// English function words
		`a about above after again against all also am an and any are as at be because been before being below between both but by
		can cannot could did do does doing done down during each either else even every few for from further had has have having he
		her here hers him his how however i if in into is it its itself just least less like made make makes many may me might more
		most much must my neither no nor not now of off on once one only or other otherwise our out over own per same shall she
		should since so some such than that the their them then there these they this those through thus to too under until up upon
		us use used uses using very via was we were what when where whether which while who whom whose why will with within without
		would yet you your etc e.g i.e`,
		// requirement vocabulary and placeholders
		`required recommended optional describe describes informative note shall must should may will tbd tbc todo xxx
		system systems requirement requirements test tests testcase case cases able capable possible ensure ensures provide
		provides support supports allow allows given then when user users data information`,
		// Swedish function words
		`och att det som en ett den de är var för på med av till om inte har ska skall kan får måste bör kommer
		vid eller så sig sin sitt sina han hon vi ni man när där här efter under över utan mellan också samt
		alla allt alltid aldrig varje varje inga ingen inget denna detta dessa vara vilket vilken vilka bara
		mot från hos nu då än även redan ytterligare enligt exempelvis exempel osv etc ca`,
		// Swedish requirement vocabulary
		`krav kraven kravet systemet användaren användare användarna test testfall kunna möjlighet möjligt behöver
		valfri valfritt valfria obligatorisk obligatoriskt beskriver informativ anmärkning`,
	} {
		for _, w := range strings.Fields(list) {
			m[w] = true
		}
	}
	return m
}()
