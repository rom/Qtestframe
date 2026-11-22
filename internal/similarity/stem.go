package similarity

import "strings"

// Stemmers for English and Swedish, after the Snowball algorithms
// (Porter2 for English, the Snowball Swedish stemmer). Words are expected
// in lower case; other languages are returned unchanged.

// Stem reduces a lower-case word to its stem in lang ("en", "sv"; any
// other language returns w).
func Stem(lang, w string) string {
	switch lang {
	case "en":
		return StemEnglish(w)
	case "sv":
		return StemSwedish(w)
	}
	return w
}

// ---------------------------------------------------------------- English

func enVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

var enExceptions = map[string]string{
	"skis": "ski", "skies": "sky", "dying": "die", "lying": "lie", "tying": "tie",
	"idly": "idl", "gently": "gentl", "ugly": "ugli", "early": "earli", "only": "onli", "singly": "singl",
	"sky": "sky", "news": "news", "howe": "howe", "atlas": "atlas", "cosmos": "cosmos", "bias": "bias", "andes": "andes",
}

var enAfter1a = map[string]bool{"inning": true, "outing": true, "canning": true, "herring": true, "earring": true,
	"proceed": true, "exceed": true, "succeed": true}

// enRegions returns the start of R1 and R2 in w.
func enRegions(w []rune) (int, int) {
	r1 := len(w)
	s := string(w)
	switch {
	case strings.HasPrefix(s, "gener"), strings.HasPrefix(s, "arsen"):
		r1 = 5
	case strings.HasPrefix(s, "commun"):
		r1 = 6
	default:
		r1 = region(w, 0, enVowel)
	}
	return r1, region(w, r1, enVowel)
}

// region is the position after the first non-vowel following a vowel at
// or after from (len(w) if none).
func region(w []rune, from int, vowel func(rune) bool) int {
	for i := from + 1; i < len(w); i++ {
		if !vowel(w[i]) && vowel(w[i-1]) {
			return i + 1
		}
	}
	return len(w)
}

func hasSuffix(w []rune, suf string) bool {
	s := []rune(suf)
	if len(s) > len(w) {
		return false
	}
	for i := range s {
		if w[len(w)-len(s)+i] != s[i] {
			return false
		}
	}
	return true
}

// shortSyllableAt: w ends (at end) with a short syllable.
func enShortSyllable(w []rune) bool {
	n := len(w)
	if n == 2 {
		return enVowel(w[0]) && !enVowel(w[1])
	}
	if n < 3 {
		return false
	}
	c := w[n-1]
	return !enVowel(w[n-3]) && enVowel(w[n-2]) && !enVowel(c) && c != 'w' && c != 'x' && c != 'Y'
}

func containsVowel(w []rune) bool {
	for _, r := range w {
		if enVowel(r) {
			return true
		}
	}
	return false
}

// StemEnglish is the Porter2 (Snowball English) stemmer.
func StemEnglish(word string) string {
	if x, ok := enExceptions[word]; ok {
		return x
	}
	w := []rune(word)
	if len(w) <= 2 {
		return word
	}
	if w[0] == '\'' {
		w = w[1:]
	}
	// y at the start or after a vowel is a consonant (Y)
	for i := range w {
		if w[i] == 'y' && (i == 0 || enVowel(w[i-1])) {
			w[i] = 'Y'
		}
	}
	r1, r2 := enRegions(w)

	// step 0
	for _, suf := range []string{"'s'", "'s", "'"} {
		if hasSuffix(w, suf) {
			w = w[:len(w)-len([]rune(suf))]
			break
		}
	}
	// step 1a
	switch {
	case hasSuffix(w, "sses"):
		w = w[:len(w)-2]
	case hasSuffix(w, "ied"), hasSuffix(w, "ies"):
		if len(w) > 4 {
			w = w[:len(w)-2]
		} else {
			w = w[:len(w)-1]
		}
	case hasSuffix(w, "us"), hasSuffix(w, "ss"):
	case hasSuffix(w, "s"):
		if len(w) >= 2 && containsVowel(w[:len(w)-2]) {
			w = w[:len(w)-1]
		}
	}
	if enAfter1a[string(w)] {
		return string(w)
	}
	// step 1b
	switch {
	case hasSuffix(w, "eedly"):
		if len(w)-5 >= r1 {
			w = w[:len(w)-3]
		}
	case hasSuffix(w, "eed"):
		if len(w)-3 >= r1 {
			w = w[:len(w)-1]
		}
	default:
		for _, suf := range []string{"ingly", "edly", "ing", "ed"} {
			if !hasSuffix(w, suf) {
				continue
			}
			stem := w[:len(w)-len(suf)]
			if containsVowel(stem) {
				w = stem
				switch {
				case hasSuffix(w, "at"), hasSuffix(w, "bl"), hasSuffix(w, "iz"):
					w = append(w, 'e')
				case len(w) >= 2 && w[len(w)-1] == w[len(w)-2] && strings.ContainsRune("bdfgmnprt", w[len(w)-1]):
					w = w[:len(w)-1]
				case r1 >= len(w) && enShortSyllable(w):
					w = append(w, 'e')
				}
			}
			break
		}
	}
	// step 1c
	if n := len(w); n > 2 && (w[n-1] == 'y' || w[n-1] == 'Y') && !enVowel(w[n-2]) {
		w[n-1] = 'i'
	}
	// step 2
	step2 := []struct{ suf, rep string }{
		{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"}, {"anci", "ance"}, {"abli", "able"}, {"entli", "ent"},
		{"ization", "ize"}, {"izer", "ize"}, {"ation", "ate"}, {"ator", "ate"}, {"alism", "al"}, {"aliti", "al"}, {"alli", "al"},
		{"fulness", "ful"}, {"ousli", "ous"}, {"ousness", "ous"}, {"iveness", "ive"}, {"iviti", "ive"}, {"biliti", "ble"},
		{"bli", "ble"}, {"fulli", "ful"}, {"lessli", "less"}, {"ogi", "og"}, {"li", ""},
	}
	w = longestRule(w, step2, func(w []rune, suf string) bool {
		pos := len(w) - len([]rune(suf))
		if pos < r1 {
			return false
		}
		switch suf {
		case "ogi":
			return pos > 0 && w[pos-1] == 'l'
		case "li":
			return pos > 0 && strings.ContainsRune("cdeghkmnrt", w[pos-1])
		}
		return true
	})
	// step 3
	step3 := []struct{ suf, rep string }{
		{"ational", "ate"}, {"tional", "tion"}, {"alize", "al"}, {"icate", "ic"}, {"iciti", "ic"}, {"ical", "ic"},
		{"ful", ""}, {"ness", ""}, {"ative", ""},
	}
	w = longestRule(w, step3, func(w []rune, suf string) bool {
		pos := len(w) - len([]rune(suf))
		if suf == "ative" {
			return pos >= r2
		}
		return pos >= r1
	})
	// step 4
	step4 := []struct{ suf, rep string }{
		{"ement", ""}, {"ance", ""}, {"ence", ""}, {"able", ""}, {"ible", ""}, {"ment", ""}, {"ant", ""}, {"ent", ""},
		{"ism", ""}, {"ate", ""}, {"iti", ""}, {"ous", ""}, {"ive", ""}, {"ize", ""}, {"ion", ""}, {"al", ""}, {"er", ""}, {"ic", ""},
	}
	w = longestRule(w, step4, func(w []rune, suf string) bool {
		pos := len(w) - len([]rune(suf))
		if pos < r2 {
			return false
		}
		if suf == "ion" {
			return pos > 0 && (w[pos-1] == 's' || w[pos-1] == 't')
		}
		return true
	})
	// step 5
	if n := len(w); n > 0 {
		switch w[n-1] {
		case 'e':
			if n-1 >= r2 || (n-1 >= r1 && !enShortSyllable(w[:n-1])) {
				w = w[:n-1]
			}
		case 'l':
			if n-1 >= r2 && n >= 2 && w[n-2] == 'l' {
				w = w[:n-1]
			}
		}
	}
	return strings.ReplaceAll(string(w), "Y", "y")
}

// longestRule applies the rule with the longest matching suffix when ok
// allows it (a matching suffix that is not allowed stops the step).
func longestRule(w []rune, rules []struct{ suf, rep string }, ok func([]rune, string) bool) []rune {
	best := -1
	for i, r := range rules {
		if hasSuffix(w, r.suf) && (best < 0 || len(r.suf) > len(rules[best].suf)) {
			best = i
		}
	}
	if best < 0 || !ok(w, rules[best].suf) {
		return w
	}
	r := rules[best]
	out := append([]rune{}, w[:len(w)-len([]rune(r.suf))]...)
	return append(out, []rune(r.rep)...)
}

// ---------------------------------------------------------------- Swedish

func svVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y', 'ä', 'å', 'ö':
		return true
	}
	return false
}

var svStep1 = []string{
	"heterna", "hetens", "anden", "andes", "andet", "arens", "arnas", "ernas", "heten", "heter", "ornas",
	"ande", "ades", "arna", "arne", "aren", "aste", "erna", "erns", "orna", "ade", "are", "ast", "ens", "ern", "het",
	"ad", "ar", "as", "at", "en", "er", "es", "et", "or", "a", "e",
}

// StemSwedish is the Snowball Swedish stemmer, with the definite neuter
// ending -et added to step 1 ("lösenordet", "systemet").
func StemSwedish(word string) string {
	w := []rune(word)
	if len(w) < 3 {
		return word
	}
	r1 := region(w, 0, svVowel)
	if r1 < 3 {
		r1 = 3
	}
	if r1 > len(w) {
		r1 = len(w)
	}
	// step 1: the longest suffix in R1
	done := false
	for _, suf := range svStep1 { // longest first
		if hasSuffix(w, suf) && len(w)-len([]rune(suf)) >= r1 {
			w = w[:len(w)-len([]rune(suf))]
			done = true
			break
		}
	}
	if !done && hasSuffix(w, "s") && len(w)-1 >= r1 && len(w) >= 2 && strings.ContainsRune("bcdfghjklmnoprtvy", w[len(w)-2]) {
		w = w[:len(w)-1]
	}
	// step 2: undouble
	for _, suf := range []string{"dd", "gd", "nn", "dt", "gt", "kt", "tt"} {
		if hasSuffix(w, suf) && len(w)-2 >= r1 {
			w = w[:len(w)-1]
			break
		}
	}
	// step 3
	switch {
	case hasSuffix(w, "fullt") && len(w)-5 >= r1:
		w = w[:len(w)-1]
	case hasSuffix(w, "löst") && len(w)-4 >= r1:
		w = w[:len(w)-1]
	case hasSuffix(w, "lig") && len(w)-3 >= r1, hasSuffix(w, "els") && len(w)-3 >= r1:
		w = w[:len(w)-3]
	case hasSuffix(w, "ig") && len(w)-2 >= r1:
		w = w[:len(w)-2]
	}
	return string(w)
}
