package gherkin

import "strings"

// Part is a test made from a feature file: the whole feature or one of
// its scenarios.
type Part struct {
	Title       string
	Description string
	// Source is the Gherkin of the test: the file as it is (a whole
	// feature) or a feature with the Background and one scenario.
	Source string
	// Tags are the tags of the feature and of the scenario (a whole
	// feature: of all its scenarios and examples too), without @, in order
	// and without repeats.
	Tags []string
}

// Split makes tests of a feature file: one per scenario (perScenario) or
// one for the whole feature. src is the text doc was parsed from.
func Split(doc *Document, src string, perScenario bool) []Part {
	if doc == nil || doc.Feature == nil {
		return nil
	}
	f := doc.Feature
	if !perScenario || len(f.AllScenarios()) == 0 {
		tags := append([]string{}, f.Tags...)
		for _, r := range f.Rules {
			tags = append(tags, r.Tags...)
		}
		for _, sc := range f.AllScenarios() {
			tags = append(tags, sc.Tags...)
			for _, ex := range sc.Examples {
				tags = append(tags, ex.Tags...)
			}
		}
		title := f.Name
		if title == "" {
			title = "Feature"
		}
		return []Part{{Title: title, Description: f.Description, Source: strings.TrimRight(src, "\n") + "\n", Tags: plainTags(tags)}}
	}
	var out []Part
	one := func(rule *Rule, sc *Scenario) {
		nf := &Feature{Keyword: f.Keyword, Name: f.Name, Description: f.Description, Tags: f.Tags, Background: f.Background, Scenarios: []*Scenario{}}
		tags := append([]string{}, f.Tags...)
		if rule != nil {
			nf.Rules = []*Rule{{Keyword: rule.Keyword, Name: rule.Name, Description: rule.Description, Tags: rule.Tags, Background: rule.Background, Scenarios: []*Scenario{sc}}}
			tags = append(tags, rule.Tags...)
		} else {
			nf.Scenarios = []*Scenario{sc}
		}
		tags = append(tags, sc.Tags...)
		for _, ex := range sc.Examples {
			tags = append(tags, ex.Tags...)
		}
		title := sc.Name
		if title == "" {
			title = f.Name
		}
		out = append(out, Part{Title: title, Description: sc.Description, Source: Format(&Document{Language: doc.Language, Feature: nf}), Tags: plainTags(tags)})
	}
	for _, sc := range f.Scenarios {
		one(nil, sc)
	}
	for _, r := range f.Rules {
		for _, sc := range r.Scenarios {
			one(r, sc)
		}
	}
	return out
}

func plainTags(tags []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range tags {
		t = strings.TrimPrefix(t, "@")
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}
