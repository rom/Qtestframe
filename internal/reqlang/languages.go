package reqlang

import (
	"sort"
	"strings"
	"unicode"
)

// kw is a keyword or term definition of a language pack.
type kw struct {
	text          string
	level         string
	caseSensitive bool
}

// Pack defines the vocabulary of one language. Term lists are matched
// case-insensitively on word boundaries; multi-word phrases may span any
// whitespace.
type Pack struct {
	Code string // ISO 639-1, e.g. "en"
	Name string // native name
	// Keywords map obligation phrases to levels (longest match wins).
	Keywords []kw
	// UppercaseRule enables the RFC 8174 "only uppercase is normative" rule.
	UppercaseRule bool
	// Examples are shown in the missing-keyword message.
	Examples                                                  string
	Ambiguous, TBD, Unbounded, AndOr, Passive, Escape, AmbNeg []string
	// Markers are frequent function words used for language detection.
	Markers []string
	// Letters are characters typical for the language; text containing any
	// of them scores extra in language detection.
	Letters string
}

// compiled is a pack with compiled phrase tables.
type compiled struct {
	code, examples                                              string
	uppercaseRule                                               bool
	keywords, ambiguous, tbd, unbounded, andOr, passive, escape []phrase
	ambiguousNegation                                           []phrase
	markers                                                     map[string]bool
	markerList                                                  []string
	letters                                                     string
}

// rfcKeywords are the English RFC 2119 keywords. Other packs accept them in
// uppercase too, since many specifications use them regardless of language.
var rfcKeywords = []kw{
	{text: "MUST NOT", level: LevelMustNot},
	{text: "SHALL NOT", level: LevelShallNot},
	{text: "SHOULD NOT", level: LevelShouldNot},
	{text: "NOT RECOMMENDED", level: LevelShouldNot},
	{text: "MUST", level: LevelMust},
	{text: "REQUIRED", level: LevelMust},
	{text: "SHALL", level: LevelShall},
	{text: "SHOULD", level: LevelShould},
	{text: "RECOMMENDED", level: LevelShould},
	{text: "MAY", level: LevelMay},
	{text: "OPTIONAL", level: LevelOptional},
}

// English is the RFC 2119 / RFC 8174 pack.
var English = Pack{
	Code: "en", Name: "English",
	Keywords: append(append([]kw{}, rfcKeywords...),
		kw{text: "WILL", level: LevelWill},
		kw{text: "DESCRIBES", level: LevelDescribe},
		kw{text: "DESCRIBE", level: LevelDescribe},
		kw{text: "INFORMATIVE", level: LevelDescribe},
		kw{text: "NOTE:", level: LevelDescribe},
	),
	UppercaseRule: true,
	Examples:      "MUST, SHALL, SHOULD, MAY",
	Ambiguous: []string{
		"fast", "quick", "quickly", "easy", "easily", "user-friendly", "user friendly",
		"simple", "efficient", "efficiently", "adequate", "appropriate",
		"as appropriate", "approximately", "about", "flexible", "robust",
		"reliable", "intuitive", "minimal", "maximize", "minimize", "sufficient",
		"etc.", "etc", "and so on", "as needed", "as required", "normally",
		"typically", "usually", "state-of-the-art", "seamless", "seamlessly",
		"scalable", "optimal", "best", "several", "some", "many", "few", "large",
		"small", "significant", "reasonable", "timely", "acceptable", "easy to use",
		"as soon as possible", "and the like", "various", "similar", "suitable",
	},
	TBD:       []string{"TBD", "TBC", "TODO", "XXX", "???"},
	Unbounded: []string{"all", "always", "never", "every", "any", "none"},
	AndOr:     []string{"and/or"},
	Passive:   []string{"be able to", "be capable of"},
	Escape:    []string{"if possible", "where applicable", "unless", "except"},
	Markers: []string{"the", "a", "an", "and", "or", "of", "to", "in", "is", "be", "for", "with", "that", "when",
		"shall", "must", "should", "may", "not", "system", "user", "it", "on", "by"},
}

// Swedish follows the conventions of Swedish standards (SIS): "ska"/"skall"
// for requirements, "bör" for recommendations, "får" for permissions and
// "får inte" for prohibitions; "måste" is accepted as a requirement.
var Swedish = Pack{
	Code: "sv", Name: "Svenska",
	Keywords: append([]kw{
		{text: "SKA INTE", level: LevelShallNot},
		{text: "SKALL INTE", level: LevelShallNot},
		{text: "FÅR INTE", level: LevelMustNot},
		{text: "FÅR EJ", level: LevelMustNot},
		{text: "BÖR INTE", level: LevelShouldNot},
		{text: "BÖR EJ", level: LevelShouldNot},
		{text: "REKOMMENDERAS INTE", level: LevelShouldNot},
		{text: "MÅSTE INTE", level: LevelOptional}, // "need not"; flagged as ambiguous negation
		{text: "BEHÖVER INTE", level: LevelOptional},
		{text: "MÅSTE", level: LevelMust},
		{text: "OBLIGATORISK", level: LevelMust},
		{text: "OBLIGATORISKT", level: LevelMust},
		{text: "OBLIGATORISKA", level: LevelMust},
		{text: "SKA", level: LevelShall},
		{text: "SKALL", level: LevelShall},
		{text: "BÖR", level: LevelShould},
		{text: "REKOMMENDERAS", level: LevelShould},
		{text: "FÅR", level: LevelMay},
		{text: "VALFRI", level: LevelOptional},
		{text: "VALFRITT", level: LevelOptional},
		{text: "VALFRIA", level: LevelOptional},
		{text: "KOMMER ATT", level: LevelWill},
		{text: "BESKRIVER", level: LevelDescribe},
		{text: "INFORMATIV", level: LevelDescribe},
		{text: "INFORMATIVT", level: LevelDescribe},
		{text: "NOT:", level: LevelDescribe},
		{text: "ANMÄRKNING:", level: LevelDescribe},
	}, upperOnly(rfcKeywords)...),
	Examples: "SKA, MÅSTE, BÖR, FÅR",
	Ambiguous: []string{
		"snabb", "snabbt", "snabba", "enkel", "enkelt", "enkla", "användarvänlig", "användarvänligt",
		"användarvänliga", "lättanvänd", "lättanvänt", "lätt att använda", "effektiv", "effektivt",
		"effektiva", "tillräcklig", "tillräckligt", "tillräckliga", "lämplig", "lämpligt", "lämpliga",
		"ungefär", "cirka", "ca", "flexibel", "flexibelt", "flexibla", "robust", "robusta", "pålitlig",
		"pålitligt", "tillförlitlig", "tillförlitligt", "intuitiv", "intuitivt", "minimal", "minimalt",
		"maximera", "minimera", "osv", "osv.", "etc", "etc.", "och så vidare", "vid behov",
		"efter behov", "normalt", "vanligtvis", "oftast", "typiskt", "modern", "moderna", "sömlös",
		"sömlöst", "skalbar", "skalbart", "optimal", "optimalt", "bäst", "bästa", "flera", "några",
		"många", "stor", "stort", "stora", "liten", "litet", "små", "betydande", "rimlig", "rimligt",
		"i rimlig tid", "acceptabel", "acceptabelt", "så snart som möjligt", "och liknande",
		"diverse", "liknande", "passande",
	},
	TBD:       []string{"TBD", "TBC", "TODO", "XXX", "???", "bestäms senare", "att bestämma", "återkommer"},
	Unbounded: []string{"alla", "allt", "alltid", "aldrig", "varje", "varenda", "samtliga", "ingen", "inget", "inga"},
	AndOr:     []string{"och/eller"},
	Passive:   []string{"kunna", "ha möjlighet att", "ha förmåga att", "vara kapabel att", "vara i stånd att"},
	Escape:    []string{"om möjligt", "där tillämpligt", "i mån av", "i den mån", "om inte", "utom", "förutom", "med undantag för"},
	AmbNeg:    []string{"måste inte"},
	Letters:   "åäö",
	Markers: []string{"och", "att", "är", "ska", "skall", "måste", "bör", "får", "inte", "för", "med", "som",
		"systemet", "användaren", "en", "ett", "det", "av", "på", "när", "till", "om", "eller", "kunna"},
}

func upperOnly(in []kw) []kw {
	out := make([]kw, len(in))
	for i, k := range in {
		k.caseSensitive = true
		out[i] = k
	}
	return out
}

var packs = map[string]*compiled{}

// Register adds or replaces a language pack.
func Register(p Pack) {
	c := &compiled{
		code: p.Code, examples: p.Examples, uppercaseRule: p.UppercaseRule,
		keywords:          compile(p.Keywords),
		ambiguous:         compileWords(p.Ambiguous),
		tbd:               compileWords(p.TBD),
		unbounded:         compileWords(p.Unbounded),
		andOr:             compileWords(p.AndOr),
		passive:           compileWords(p.Passive),
		escape:            compileWords(p.Escape),
		ambiguousNegation: compileWords(p.AmbNeg),
		markers:           map[string]bool{},
		letters:           strings.ToLower(p.Letters),
	}
	for _, m := range p.Markers {
		m = strings.ToLower(m)
		if !c.markers[m] {
			c.markerList = append(c.markerList, m)
		}
		c.markers[m] = true
	}
	packs[p.Code] = c
}

func init() {
	Register(English)
	Register(Swedish)
}

// Languages returns the codes of the registered packs, sorted.
func Languages() []string {
	out := make([]string, 0, len(packs))
	for c := range packs {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Supported reports whether lang is a registered pack.
func Supported(lang string) bool { return packs[lang] != nil }

// Detect guesses the language of text from frequent function words (and
// letters typical for a language); English wins ties and empty input.
func Detect(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || r == '\'')
	})
	best, bestScore := "en", 0
	for _, code := range Languages() {
		pk := packs[code]
		score := 0
		for _, w := range words {
			if pk.markers[w] {
				score++
			}
		}
		if pk.letters != "" && strings.ContainsAny(strings.ToLower(text), pk.letters) {
			score += 2
		}
		if score > bestScore || (score == bestScore && code == "en") {
			best, bestScore = code, score
		}
	}
	return best
}

// DetectConfident guesses the language of text like Detect, but only
// answers when the text has enough evidence: at least minScore marker
// words (or Swedish letters) and more than any other language. ok is false
// for short or ambiguous text.
func DetectConfident(text string, minScore int) (lang string, ok bool) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || r == '\'')
	})
	best, bestScore, second := "", 0, 0
	for _, code := range Languages() {
		pk := packs[code]
		score := 0
		for _, w := range words {
			if pk.markers[w] {
				score++
			}
		}
		if pk.letters != "" && strings.ContainsAny(strings.ToLower(text), pk.letters) {
			score += 2
		}
		switch {
		case score > bestScore:
			best, second, bestScore = code, bestScore, score
		case score > second:
			second = score
		}
	}
	if bestScore < minScore || bestScore == second {
		return "", false
	}
	return best, true
}

// IsRFCKeyword reports whether a canonical keyword is one of the English
// RFC 2119 keywords (MUST, SHALL NOT, RECOMMENDED …).
func IsRFCKeyword(keyword string) bool {
	k := strings.ToUpper(keyword)
	for _, x := range rfcKeywords {
		if x.text == k {
			return true
		}
	}
	return false
}
