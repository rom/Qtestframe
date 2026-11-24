package gherkin

import (
	"sort"
	"strings"
)

// Dialect holds the keywords of one language. Header keywords are
// followed by a colon; step keywords by a space (except "*").
type Dialect struct {
	Code       string
	Feature    []string
	Background []string
	Rule       []string
	Scenario   []string
	Outline    []string
	Examples   []string
	Given      []string
	When       []string
	Then       []string
	And        []string
	But        []string
}

// Dialects are the supported languages. Both are always understood; the
// "# language:" header only decides Document.Language (and so which
// keywords Format and conversions write).
var Dialects = map[string]*Dialect{
	"en": {
		Code:       "en",
		Feature:    []string{"Feature", "Business Need", "Ability"},
		Background: []string{"Background"},
		Rule:       []string{"Rule"},
		Scenario:   []string{"Scenario", "Example"},
		Outline:    []string{"Scenario Outline", "Scenario Template"},
		Examples:   []string{"Examples", "Scenarios"},
		Given:      []string{"Given"},
		When:       []string{"When"},
		Then:       []string{"Then"},
		And:        []string{"And"},
		But:        []string{"But"},
	},
	"sv": {
		Code:       "sv",
		Feature:    []string{"Egenskap"},
		Background: []string{"Bakgrund"},
		Rule:       []string{"Regel"},
		Scenario:   []string{"Scenario"},
		Outline:    []string{"Abstrakt Scenario", "Abstrakt scenario", "Scenariomall"},
		Examples:   []string{"Exempel"},
		Given:      []string{"Givet"},
		When:       []string{"När"},
		Then:       []string{"Så"},
		And:        []string{"Och"},
		But:        []string{"Men"},
	},
}

// DialectOf returns the dialect of a language code (English for unknown ones).
func DialectOf(code string) *Dialect {
	if d, ok := Dialects[strings.ToLower(code)]; ok {
		return d
	}
	return Dialects["en"]
}

type headerKind int

const (
	hFeature headerKind = iota + 1
	hBackground
	hRule
	hScenario
	hOutline
	hExamples
)

type headerKW struct {
	word string
	kind headerKind
	lang string
}

type stepKW struct {
	word string
	typ  StepType
	lang string
}

var (
	headerKeywords []headerKW
	stepKeywords   []stepKW
)

func init() {
	for code, d := range Dialects {
		for _, g := range []struct {
			words []string
			kind  headerKind
		}{{d.Feature, hFeature}, {d.Background, hBackground}, {d.Rule, hRule}, {d.Scenario, hScenario}, {d.Outline, hOutline}, {d.Examples, hExamples}} {
			for _, w := range g.words {
				headerKeywords = append(headerKeywords, headerKW{w, g.kind, code})
			}
		}
		for _, g := range []struct {
			words []string
			typ   StepType
		}{{d.Given, Context}, {d.When, Action}, {d.Then, Outcome}, {d.And, Conjunction}, {d.But, Conjunction}} {
			for _, w := range g.words {
				stepKeywords = append(stepKeywords, stepKW{w, g.typ, code})
			}
		}
	}
	// longest first, so "Scenario Outline:" wins over "Scenario:"
	sort.SliceStable(headerKeywords, func(i, j int) bool {
		if len(headerKeywords[i].word) != len(headerKeywords[j].word) {
			return len(headerKeywords[i].word) > len(headerKeywords[j].word)
		}
		return headerKeywords[i].word < headerKeywords[j].word
	})
	sort.SliceStable(stepKeywords, func(i, j int) bool {
		if len(stepKeywords[i].word) != len(stepKeywords[j].word) {
			return len(stepKeywords[i].word) > len(stepKeywords[j].word)
		}
		return stepKeywords[i].word < stepKeywords[j].word
	})
}

// matchHeader recognises "Keyword: name" (the line already trimmed).
func matchHeader(line string) (headerKW, string, bool) {
	for _, k := range headerKeywords {
		if strings.HasPrefix(line, k.word+":") {
			return k, strings.TrimSpace(line[len(k.word)+1:]), true
		}
	}
	return headerKW{}, "", false
}

// matchStep recognises "Keyword text" (the line already trimmed).
func matchStep(line string) (stepKW, string, bool) {
	if strings.HasPrefix(line, "* ") || line == "*" {
		return stepKW{"*", Conjunction, ""}, strings.TrimSpace(line[1:]), true
	}
	for _, k := range stepKeywords {
		if strings.HasPrefix(line, k.word+" ") || line == k.word {
			return k, strings.TrimSpace(line[len(k.word):]), true
		}
	}
	return stepKW{}, "", false
}

// StepKeywordType returns the type of a step keyword ("" when it is not one).
func StepKeywordType(word string) StepType {
	if word == "*" {
		return Conjunction
	}
	for _, k := range stepKeywords {
		if k.word == word {
			return k.typ
		}
	}
	return ""
}
