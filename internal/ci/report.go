// Package ci holds what Qtestframe's CI/CD integration shares between the
// server and the command line: the quality gate report and its renderings
// (text for the job log, Markdown for merge request notes, JUnit XML for
// the pipeline's test report, JSON), the detection of the CI system a
// command runs in, the formats of a test selection (test runner filters
// and a generated GitLab child pipeline) and the ready-made pipeline
// templates. See docs/user-guide/en/ci-cd.md.
package ci

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Ref names something a report refers to, with its page.
type Ref struct {
	ID   string `json:"id"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Item is a requirement, test, defect or risk of a condition or a failure.
type Item struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
	URL   string `json:"url,omitempty"`
	// Variants in which the item keeps the condition from being met.
	Variants []string `json:"variants,omitempty"`
}

// Condition is one evaluated gate condition (an exit criterion).
type Condition struct {
	Type      string  `json:"type"`
	Threshold float64 `json:"threshold,omitempty"`
	Level     string  `json:"level,omitempty"`
	CycleID   string  `json:"cycle_id,omitempty"`
	// Label: the condition in words, e.g. "coverage >= 80%".
	Label string `json:"label"`
	// State: pass | waived (met thanks to waivers) | fail.
	State     string  `json:"state"`
	Met       bool    `json:"met"`
	Actual    float64 `json:"actual"`
	Total     int     `json:"total"`
	Available bool    `json:"available"`
	Items     []Item  `json:"items"`
	Waived    []Item  `json:"waived"`
}

// Results counts the results of the run or build a report is about.
type Results struct {
	Total      int `json:"total"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	Blocked    int `json:"blocked"`
	Skipped    int `json:"skipped"`
	NotRun     int `json:"not_run"`
	InProgress int `json:"in_progress"`
}

// Affected is the coverage of the requirements a change refers to.
type Affected struct {
	Requirements []AffectedRequirement `json:"requirements"`
	// Covered: with tests; Passed: all their tests passed.
	Covered int `json:"covered"`
	Passed  int `json:"passed"`
	Total   int `json:"total"`
}

// AffectedRequirement is a requirement a merge request or commit refers to.
type AffectedRequirement struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
	// Result: passed | failed | not_run | none (no tests).
	Result string `json:"result"`
	Tests  int    `json:"tests"`
}

// VariantVerdict is the gate of one targeted variant.
type VariantVerdict struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
}

// Report is the answer of the quality gate (POST /projects/{pid}/gate).
type Report struct {
	// Pass: the gate passed (with summary_only: no failed or blocked result).
	Pass bool `json:"pass"`
	// Verdict: pass | fail.
	Verdict     string `json:"verdict"`
	SummaryOnly bool   `json:"summary_only,omitempty"`
	Project     Ref    `json:"project"`
	Release     *Ref   `json:"release,omitempty"`
	Variant     *Ref   `json:"variant,omitempty"`
	Run         *Ref   `json:"run,omitempty"`
	Build       *Ref   `json:"build,omitempty"`
	// Scope: all | affected | a query; ScopeSize: its requirements.
	Scope        string `json:"scope"`
	ScopeSize    int    `json:"scope_size"`
	Commit       string `json:"commit,omitempty"`
	MergeRequest string `json:"merge_request,omitempty"`
	// URL: the page to open (the run, else the build's, else the release).
	URL        string           `json:"url"`
	Conditions []Condition      `json:"conditions"`
	Variants   []VariantVerdict `json:"variants,omitempty"`
	Results    *Results         `json:"results,omitempty"`
	// Failures: failed and blocked tests of the run or build (at most 50).
	Failures []Item    `json:"failures"`
	Affected *Affected `json:"affected,omitempty"`
	// Waivers in force (key and title).
	Waivers []Item         `json:"waivers"`
	Summary map[string]int `json:"summary,omitempty"`
}

// Verdicts and condition states.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
	StatePass   = "pass"
	StateWaived = "waived"
	StateFail   = "fail"
)

// percentTypes are the conditions measured in percent.
var percentTypes = []string{"coverage", "passed", "executed", "approved", "cycle_executed"}

// ConditionLabel describes a condition in words.
func ConditionLabel(typ string, threshold float64, level, cycle string) string {
	switch {
	case slices.Contains(percentTypes, typ) && typ == "cycle_executed":
		return fmt.Sprintf("cycle_executed %s >= %s%%", cycle, pct(threshold))
	case slices.Contains(percentTypes, typ):
		return fmt.Sprintf("%s >= %s%%", typ, pct(threshold))
	case level != "":
		return typ + ":" + level
	}
	return typ
}

func pct(v float64) string {
	if v == 0 {
		v = 100
	}
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", v), "0"), ".")
}

// measure is the value of a condition, e.g. "92.5% of 40" or "2 found".
func (c *Condition) measure() string {
	switch {
	case !c.Available && slices.Contains(percentTypes, c.Type):
		return "nothing to measure"
	case slices.Contains(percentTypes, c.Type):
		return fmt.Sprintf("%s%% of %d", strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", c.Actual), "0"), "."), c.Total)
	case c.Type == "mandatory_passed":
		return fmt.Sprintf("%d of %d not passed", int(c.Actual), c.Total)
	}
	return fmt.Sprintf("%d found", int(c.Actual))
}

// Failing returns the conditions that are not met.
func (r *Report) Failing() []Condition {
	var out []Condition
	for _, c := range r.Conditions {
		if !c.Met {
			out = append(out, c)
		}
	}
	return out
}

// Subject describes what the report is about, e.g. `release "1.0", run "Nightly"`.
func (r *Report) Subject() string {
	var parts []string
	if r.Release != nil {
		parts = append(parts, fmt.Sprintf("release %q", r.Release.Name))
	}
	if r.Variant != nil {
		parts = append(parts, fmt.Sprintf("variant %q", r.Variant.Name))
	}
	if r.Run != nil {
		parts = append(parts, fmt.Sprintf("run %q", r.Run.Name))
	}
	if r.Build != nil {
		parts = append(parts, fmt.Sprintf("build %s", r.Build.Name))
	}
	if len(parts) == 0 {
		parts = append(parts, "project "+r.Project.Key)
	}
	return strings.Join(parts, ", ")
}

// ResultLine is e.g. "40 passed, 2 failed, 1 blocked, 3 not run (46)".
func (x *Results) ResultLine() string {
	var parts []string
	for _, p := range []struct {
		n    int
		name string
	}{{x.Passed, "passed"}, {x.Failed, "failed"}, {x.Blocked, "blocked"}, {x.Skipped, "skipped"}, {x.InProgress, "in progress"}, {x.NotRun, "not run"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.name))
		}
	}
	if len(parts) == 0 {
		return "no results"
	}
	return fmt.Sprintf("%s (%d)", strings.Join(parts, ", "), x.Total)
}

// StatusDescription is a short summary for a commit status.
func (r *Report) StatusDescription() string {
	var parts []string
	if !r.SummaryOnly {
		if r.Pass {
			parts = append(parts, "gate passed")
		} else {
			parts = append(parts, fmt.Sprintf("gate failed (%d of %d conditions)", len(r.Failing()), len(r.Conditions)))
		}
	}
	if r.Results != nil {
		parts = append(parts, r.Results.ResultLine())
	}
	return strings.Join(parts, "; ")
}

// maxListed bounds the items listed per condition in the text and Markdown.
const maxListed = 10

func itemLine(it Item) string {
	s := it.Key
	if it.Title != "" {
		s += "  " + it.Title
	}
	if it.Note != "" {
		s += "  (" + it.Note + ")"
	}
	if len(it.Variants) > 0 {
		s += "  [" + strings.Join(it.Variants, ", ") + "]"
	}
	if it.URL != "" {
		s += "  " + it.URL
	}
	return s
}

// WriteText writes the human-readable summary for the job log.
func (r *Report) WriteText(w io.Writer) {
	head := "Qtestframe quality gate"
	switch {
	case r.SummaryOnly:
		head = "Qtestframe results"
	case r.Pass:
		head += ": PASSED"
	default:
		head += ": FAILED"
	}
	fmt.Fprintf(w, "%s  (%s)\n", head, r.Subject())
	if !r.SummaryOnly {
		fmt.Fprintf(w, "scope: %s (%d requirements)\n", r.Scope, r.ScopeSize)
	}
	for _, c := range r.Conditions {
		mark := "[PASS]  "
		switch c.State {
		case StateWaived:
			mark = "[WAIVED]"
		case StateFail:
			mark = "[FAIL]  "
		}
		fmt.Fprintf(w, "  %s %-34s %s\n", mark, c.Label, c.measure())
		if !c.Met {
			for i, it := range c.Items {
				if i == maxListed {
					fmt.Fprintf(w, "           … and %d more\n", len(c.Items)-maxListed)
					break
				}
				fmt.Fprintf(w, "           %s\n", itemLine(it))
			}
		}
		for i, it := range c.Waived {
			if i == maxListed {
				break
			}
			fmt.Fprintf(w, "           waived: %s\n", itemLine(it))
		}
	}
	for _, v := range r.Variants {
		state := "pass"
		if !v.Pass {
			state = "FAIL"
		}
		fmt.Fprintf(w, "  variant %s: %s\n", v.Name, state)
	}
	if r.Results != nil {
		fmt.Fprintf(w, "results: %s\n", r.Results.ResultLine())
	}
	if len(r.Failures) > 0 {
		fmt.Fprintf(w, "failed tests (%d):\n", len(r.Failures))
		for _, it := range r.Failures {
			fmt.Fprintf(w, "  %s\n", itemLine(it))
		}
	}
	if a := r.Affected; a != nil && a.Total > 0 {
		fmt.Fprintf(w, "affected requirements: %d, %d covered, %d passed\n", a.Total, a.Covered, a.Passed)
		for _, q := range a.Requirements {
			fmt.Fprintf(w, "  %-8s %s  %s\n", q.Result, q.Key, q.Title)
		}
	}
	if r.URL != "" {
		fmt.Fprintf(w, "details: %s\n", r.URL)
	}
}

// NoteMarker is the hidden marker of the merge request note of a project:
// the note that contains it is updated instead of adding another.
func NoteMarker(projectKey string) string {
	return "<!-- qtestframe:ci-summary:" + strings.ToUpper(projectKey) + " -->"
}

func mdEscape(s string) string {
	r := strings.NewReplacer("|", "\\|", "\n", " ", "<", "&lt;", ">", "&gt;", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`")
	return r.Replace(s)
}

func mdLink(text, url string) string {
	if url == "" {
		return mdEscape(text)
	}
	return "[" + mdEscape(text) + "](" + url + ")"
}

// Markdown renders the report for a merge request note (with the hidden
// marker of the project as its first line).
func (r *Report) Markdown() string {
	var b strings.Builder
	b.WriteString(NoteMarker(r.Project.Key) + "\n")
	icon := map[bool]string{true: "✅", false: "❌"}
	switch {
	case r.SummaryOnly:
		fmt.Fprintf(&b, "### %s Qtestframe test results\n\n", icon[r.Pass])
	case r.Pass:
		fmt.Fprintf(&b, "### %s Qtestframe quality gate passed\n\n", icon[true])
	default:
		fmt.Fprintf(&b, "### %s Qtestframe quality gate failed\n\n", icon[false])
	}
	var about []string
	if r.Release != nil {
		about = append(about, "Release "+mdLink(r.Release.Name, r.Release.URL))
	}
	if r.Variant != nil {
		about = append(about, "variant "+mdEscape(r.Variant.Name))
	}
	if r.Run != nil {
		about = append(about, "run "+mdLink(r.Run.Name, r.Run.URL))
	}
	if r.Build != nil {
		about = append(about, "build "+mdLink(r.Build.Name, r.Build.URL))
	}
	if len(about) == 0 {
		about = append(about, "Project "+mdEscape(r.Project.Key))
	}
	b.WriteString(strings.Join(about, " · ") + "\n\n")
	if r.Results != nil {
		fmt.Fprintf(&b, "**Results:** %s\n\n", r.Results.ResultLine())
	}
	if len(r.Conditions) > 0 {
		b.WriteString("| | Condition | Value |\n|---|---|---|\n")
		for _, c := range r.Conditions {
			mark := "✅"
			switch c.State {
			case StateWaived:
				mark = "☑️ waived"
			case StateFail:
				mark = "❌"
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", mark, c.Label, mdEscape(c.measure()))
		}
		b.WriteString("\n")
	}
	for _, v := range r.Variants {
		fmt.Fprintf(&b, "- Variant %s: %s\n", mdEscape(v.Name), map[bool]string{true: "passed", false: "**failed**"}[v.Pass])
	}
	if len(r.Variants) > 0 {
		b.WriteString("\n")
	}
	if f := r.Failing(); len(f) > 0 {
		b.WriteString("<details><summary>What keeps the gate from passing</summary>\n\n")
		for _, c := range f {
			fmt.Fprintf(&b, "**`%s`**\n\n", c.Label)
			for i, it := range c.Items {
				if i == maxListed {
					fmt.Fprintf(&b, "- … and %d more\n", len(c.Items)-maxListed)
					break
				}
				fmt.Fprintf(&b, "- %s %s%s\n", mdLink(it.Key, it.URL), mdEscape(it.Title), noteMD(it))
			}
			if len(c.Items) == 0 {
				b.WriteString("- nothing to measure\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("</details>\n\n")
	}
	if len(r.Failures) > 0 {
		fmt.Fprintf(&b, "**Failed tests (%d)**\n\n", len(r.Failures))
		for _, it := range r.Failures {
			fmt.Fprintf(&b, "- %s %s%s\n", mdLink(it.Key, it.URL), mdEscape(it.Title), noteMD(it))
		}
		b.WriteString("\n")
	}
	if a := r.Affected; a != nil && a.Total > 0 {
		fmt.Fprintf(&b, "**Requirements of this change:** %d, %d with tests, %d passed\n\n", a.Total, a.Covered, a.Passed)
		b.WriteString("| Requirement | Tests | Result |\n|---|---|---|\n")
		for _, q := range a.Requirements {
			fmt.Fprintf(&b, "| %s %s | %d | %s |\n", mdLink(q.Key, q.URL), mdEscape(q.Title), q.Tests, resultMD(q.Result))
		}
		b.WriteString("\n")
	}
	if len(r.Waivers) > 0 {
		var ws []string
		for _, w := range r.Waivers {
			ws = append(ws, mdLink(w.Key, w.URL))
		}
		fmt.Fprintf(&b, "Waivers in force: %s\n\n", strings.Join(ws, ", "))
	}
	if r.URL != "" {
		fmt.Fprintf(&b, "[Open in Qtestframe](%s)\n", r.URL)
	}
	return b.String()
}

func noteMD(it Item) string {
	s := ""
	if it.Note != "" {
		s += " — " + mdEscape(it.Note)
	}
	if len(it.Variants) > 0 {
		s += " (" + mdEscape(strings.Join(it.Variants, ", ")) + ")"
	}
	return s
}

func resultMD(r string) string {
	switch r {
	case "passed":
		return "✅ passed"
	case "failed":
		return "❌ failed"
	case "none":
		return "⚠️ no tests"
	}
	return "⏳ not run"
}

// JUnit renders the gate as a JUnit XML report: one test case per
// condition (a failure when it is not met), so the pipeline's test report
// shows the gate next to the tests.
func (r *Report) JUnit() ([]byte, error) {
	type failure struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Text    string `xml:",chardata"`
	}
	type tc struct {
		Name      string   `xml:"name,attr"`
		ClassName string   `xml:"classname,attr"`
		Time      string   `xml:"time,attr"`
		Failure   *failure `xml:"failure,omitempty"`
		SystemOut string   `xml:"system-out,omitempty"`
	}
	type suite struct {
		XMLName  xml.Name `xml:"testsuite"`
		Name     string   `xml:"name,attr"`
		Tests    int      `xml:"tests,attr"`
		Failures int      `xml:"failures,attr"`
		Errors   int      `xml:"errors,attr"`
		Cases    []tc     `xml:"testcase"`
	}
	s := suite{Name: "qtestframe.gate", Cases: []tc{}}
	class := "qtestframe.gate." + strings.ToLower(r.Project.Key)
	for _, c := range r.Conditions {
		x := tc{Name: c.Label, ClassName: class, Time: "0"}
		var lines []string
		for _, it := range c.Items {
			lines = append(lines, itemLine(it))
		}
		if !c.Met {
			x.Failure = &failure{Message: c.Label + ": " + c.measure(), Type: "gate", Text: strings.Join(lines, "\n")}
			s.Failures++
		} else {
			var w []string
			for _, it := range c.Waived {
				w = append(w, "waived: "+itemLine(it))
			}
			x.SystemOut = strings.Join(append([]string{c.measure()}, w...), "\n")
		}
		s.Cases = append(s.Cases, x)
	}
	s.Tests = len(s.Cases)
	out, err := xml.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(out, '\n')...), nil
}

// Write renders the report in a format: text, json, junit or markdown.
func (r *Report) Write(w io.Writer, format string) error {
	switch format {
	case "", "text":
		r.WriteText(w)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "junit":
		b, err := r.JUnit()
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	case "markdown", "md":
		_, err := io.WriteString(w, r.Markdown())
		return err
	default:
		return fmt.Errorf("unknown format %q (text, json, junit or markdown)", format)
	}
	return nil
}
