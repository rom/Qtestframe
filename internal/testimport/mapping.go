package testimport

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/model"
)

// norm lower-cases s and keeps only letters and digits, single spaces
// between words.
func norm(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

// guessPriority maps the priority names of the tools to model priorities
// ("" when unknown: the default medium).
func guessPriority(raw string) string {
	n := norm(raw)
	switch {
	case n == "":
		return ""
	case containsAny(n, "critical", "blocker", "highest", "urgent", "kritisk", "showstopper"):
		return "critical"
	case containsAny(n, "don t test", "dont test", "lowest", "trivial", "minor", "low", "låg"):
		return "low"
	case containsAny(n, "must test", "high", "major", "hög", "important"):
		return "high"
	case containsAny(n, "test if time", "medium", "normal", "moderate", "average", "mellan", "default"):
		return "medium"
	}
	// P1 … P4, 1 … 4 (Azure DevOps: 1 is the highest)
	n = strings.TrimPrefix(n, "p")
	switch n {
	case "0", "1":
		return "critical"
	case "2":
		return "high"
	case "3":
		return "medium"
	case "4", "5":
		return "low"
	}
	return ""
}

// guessStatus maps test case statuses and workflow states to model test
// statuses ("" when unknown: draft).
func guessStatus(raw string) string {
	n := norm(raw)
	switch {
	case n == "":
		return ""
	case containsAny(n, "review", "granskning", "awaiting", "pending approval"):
		return "review"
	case containsAny(n, "deprecated", "obsolete", "retired", "archived", "inactive", "closed", "removed", "föråldrad"):
		return "deprecated"
	case containsAny(n, "ready", "approved", "active", "done", "final", "published", "baselined", "released", "klar", "godkänd"):
		return "ready"
	case containsAny(n, "draft", "new", "design", "progress", "to do", "todo", "open", "backlog", "utkast", "planned", "not started", "work"):
		return "draft"
	}
	return ""
}

// GuessResult maps an execution result of the tools to a final model
// result ("" for results that are not final: untested, retest, in
// progress, unknown).
func GuessResult(raw string) string {
	n := norm(raw)
	switch {
	case n == "":
		return ""
	case containsAny(n, "untested", "not run", "not executed", "unexecuted", "retest", "progress", "executing", "todo", "to do", "no run", "pending"):
		return ""
	case containsAny(n, "pass", "ok", "success", "godkänd"):
		return model.ResultPassed
	case containsAny(n, "fail", "error", "broken", "underkänd"):
		return model.ResultFailed
	case containsAny(n, "block"):
		return model.ResultBlocked
	case containsAny(n, "skip", "n a", "not applicable", "na", "ignored", "cancel", "abort"):
		return model.ResultSkipped
	}
	return ""
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if s == w || strings.HasPrefix(s, w+" ") || strings.HasSuffix(s, " "+w) || strings.Contains(s, " "+w+" ") ||
			(len(w) > 3 && strings.Contains(s, w)) {
			return true
		}
	}
	return false
}

// mappedField returns the custom field key a source field maps to: the
// user's choice (a key of defs, or "" to leave it out), else the field
// whose label, export header or key matches the name.
func mappedField(choice map[string]string, name string, defs []model.CustomField) (string, bool) {
	known := func(k string) bool {
		for _, d := range defs {
			if d.Key == k {
				return true
			}
		}
		return false
	}
	for k, v := range choice {
		if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(name)) {
			v = strings.TrimPrefix(strings.TrimSpace(v), "cf.")
			if v == "" || v == "ignore" {
				return "", false
			}
			if known(v) {
				return v, false
			}
		}
	}
	n := norm(name)
	key := customfield.KeyFromLabel(name)
	for _, d := range defs {
		if norm(d.Label) == n || norm(customfield.Column(&d)) == n || d.Key == key || d.Key == strings.ToLower(strings.TrimSpace(name)) {
			return d.Key, true
		}
	}
	return "", true
}

var (
	tagRe       = regexp.MustCompile(`(?s)<[^>]*>`)
	blockTagRe  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/tr|/h[1-6])\s*/?>`)
	liTagRe     = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
	blankRunsRe = regexp.MustCompile(`\n{3,}`)
)

// htmlText turns the HTML of rich text fields (Azure DevOps, TestRail,
// qTest) into plain text; text without tags is returned as it is.
func htmlText(s string) string {
	if !strings.Contains(s, "<") {
		return strings.TrimSpace(html.UnescapeString(s))
	}
	s = blockTagRe.ReplaceAllString(s, "\n")
	s = liTagRe.ReplaceAllString(s, "- ")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(strings.ReplaceAll(s, " ", " "))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	s = blankRunsRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
}

var numberedRe = regexp.MustCompile(`^\s*(?:step\s*)?(\d{1,3})\s*[.):\-]\s+(.*)$`)

// splitNumbered splits a numbered list ("1. Open … \n2. Click …") into its
// items; text that is not a numbered list of at least two items is one item.
func splitNumbered(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var items []string
	numbered := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if m := numberedRe.FindStringSubmatch(line); m != nil {
			items = append(items, m[2])
			numbered++
			continue
		}
		if len(items) == 0 {
			items = append(items, line)
			continue
		}
		items[len(items)-1] += "\n" + line
	}
	if numbered < 2 {
		return []string{text}
	}
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	return items
}

// textSteps makes steps of the step and expected texts of a text template:
// numbered lists of the same length are paired, else the expected text is
// the expected result of the last step.
func textSteps(steps, expected string) ([]Step, string) {
	as, es := splitNumbered(steps), splitNumbered(expected)
	if len(as) == 0 {
		return nil, expected
	}
	out := make([]Step, len(as))
	for i, a := range as {
		out[i].Action = a
	}
	if len(es) == len(as) && len(as) > 1 {
		for i, e := range es {
			out[i].Expected = e
		}
		return out, ""
	}
	if len(as) == 1 {
		out[0].Expected = strings.TrimSpace(expected)
		return out, ""
	}
	return out, expected
}

var gherkinStepRe = regexp.MustCompile(`(?i)^\s*(given|when|then|and|but|\*)\s+(.*)$`)

// GherkinSteps turns the steps of a Gherkin scenario into test steps: a
// Given/When line (and the And/But lines after it) is an action, a Then
// line (and its And/But lines) the expected result of the last action.
func GherkinSteps(text string) []Step {
	var out []Step
	then := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		m := gherkinStepRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kw := strings.ToLower(m[1])
		body := strings.TrimSpace(m[1] + " " + m[2])
		switch {
		case kw == "then":
			then = true
		case kw == "given" || kw == "when":
			then = false
		}
		if then {
			if len(out) == 0 {
				out = append(out, Step{})
			}
			last := &out[len(out)-1]
			if last.Expected != "" {
				last.Expected += "\n"
			}
			last.Expected += body
			continue
		}
		out = append(out, Step{Action: body})
	}
	return out
}

// splitRefs splits a list of issue keys or work item ids.
func splitRefs(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
		p = strings.TrimSpace(p)
		// "CALC-12 Summary text" or "#4711": the key only
		if f := strings.Fields(p); len(f) > 0 {
			p = f[0]
		}
		p = strings.TrimPrefix(p, "#")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitTags splits labels (comma, semicolon or space separated).
func splitTags(s string) []string {
	sep := func(r rune) bool { return r == ',' || r == ';' || r == '\n' }
	if !strings.ContainsAny(s, ",;\n") {
		sep = unicode.IsSpace
	}
	var out []string
	for _, p := range strings.FieldsFunc(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var estimateRe = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(w|d|h|m|s|min|mins|minutes|hours?|days?)?`)

// estimateMinutes reads an estimate ("1h 30m", "45", "2d", "90 min",
// "01:30") as minutes; seconds when unit is "s" (Zephyr exports
// milliseconds as numbers, see callers).
func estimateMinutes(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if h, m, ok := strings.Cut(s, ":"); ok {
		hh, err1 := strconv.Atoi(strings.TrimSpace(h))
		mm, err2 := strconv.Atoi(strings.TrimSpace(m))
		if err1 == nil && err2 == nil {
			return hh*60 + mm
		}
	}
	total := 0.0
	for _, m := range estimateRe.FindAllStringSubmatch(s, -1) {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(m[2]) {
		case "w":
			total += v * 5 * 8 * 60
		case "d", "day", "days":
			total += v * 8 * 60
		case "h", "hour", "hours":
			total += v * 60
		case "s":
			total += v / 60
		default:
			total += v
		}
	}
	return int(total + 0.5)
}

var dateLayouts = []string{
	time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02", "1/2/2006 3:04:05 PM", "1/2/2006 3:04 PM", "1/2/2006 15:04", "1/2/2006", "02.01.2006 15:04", "02.01.2006", "02/Jan/06 3:04 PM", "2/Jan/06 3:04 PM", "02 Jan 2006 15:04",
}

// parseTime reads the dates of the exports (UTC unless they name a zone).
func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms > 1e11 {
		t := time.UnixMilli(ms).UTC()
		return &t
	}
	return nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(oneLine(s), 200)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
