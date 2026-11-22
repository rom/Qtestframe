// Package report builds standard and custom reports as format-neutral
// export.Datasets (rendered to PDF, DOCX, XLSX, CSV or Markdown by package
// export).
//
// The package is independent of HTTP and storage: callers load and
// decorate the project data (derived fields such as coverage and last
// results, filtering with the query language) and pass it in.
package report

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/analytics"
	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/tracerules"
)

// ErrInvalid marks invalid report requests (unknown template, missing
// parameter, invalid custom configuration).
var ErrInvalid = errors.New("invalid report")

func invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Formats lists the output formats of every report.
var Formats = []string{string(export.PDF), string(export.DOCX), string(export.XLSX), string(export.CSV), string(export.MD)}

// Param describes a template parameter.
type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // query | run
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// Template describes a standard report.
type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []Param  `json:"params"`
	Formats     []string `json:"formats"`

	build func(d *Data, params map[string]string) (*export.Dataset, error)
}

// Template ids.
const (
	ProjectStatus        = "project_status"
	RequirementsCoverage = "requirements_coverage"
	Traceability         = "traceability"
	RunSummary           = "run_summary"
	Defects              = "defects"
	TestInventory        = "test_inventory"
	Risks                = "risks"
)

var (
	queryParam = func(of string) Param {
		return Param{Name: "query", Type: "query", Description: "filter expression applied to " + of}
	}
	templates = []Template{
		{ID: ProjectStatus, Name: "Project status", Description: "Key figures, requirements by status and obligation, coverage, test results and recent runs.",
			Params: []Param{}, build: buildProjectStatus},
		{ID: RequirementsCoverage, Name: "Requirements coverage", Description: "Every requirement with its linked tests, their last results and the resulting coverage.",
			Params: []Param{queryParam("requirements")}, build: buildRequirementsCoverage},
		{ID: Traceability, Name: "Traceability matrix", Description: "Requirement × test links, requirements without tests and tests without requirements.",
			Params: []Param{queryParam("requirements and/or tests")}, build: buildTraceability},
		{ID: RunSummary, Name: "Test run summary", Description: "One test run: totals, executions, defects and durations.",
			Params: []Param{{Name: "run_id", Type: "run", Required: true, Description: "the test run"}}, build: buildRunSummary},
		{ID: Defects, Name: "Defects", Description: "Defects recorded on executions across all runs, grouped by defect.",
			Params: []Param{}, build: buildDefects},
		{ID: TestInventory, Name: "Test inventory", Description: "Test cases by folder, execution type, status and category, with the full list.",
			Params: []Param{queryParam("tests")}, build: buildTestInventory},
		{ID: Risks, Name: "Risk report", Description: "Risks by level and status, a likelihood × impact heat map, the risk register, traceability to requirements, tests and their latest results, untested mitigations and the top risks.",
			Params: []Param{}, build: buildRisks},
	}
)

// Templates returns the standard report templates.
func Templates() []Template {
	out := make([]Template, len(templates))
	for i, t := range templates {
		t.Formats = append([]string{}, Formats...)
		t.Params = append([]Param{}, t.Params...)
		out[i] = t
	}
	return out
}

// Lookup returns the template with id.
func Lookup(id string) (Template, bool) {
	for _, t := range Templates() {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Data is the (already filtered and decorated) project content a template
// works on. Requirements and tests carry their derived fields
// (FolderPath, LinkedTests, Coverage, LinkedReqs, LastResult).
type Data struct {
	Project      model.Project
	Requirements []model.Requirement
	Tests        []model.TestCase
	// Links are the trace links of the project (with keys).
	Links []model.TraceLink
	// Runs of the project, newest first, with Summary.
	Runs []model.TestRun
	// Executions per run id (only loaded where a template needs them).
	Executions map[string][]model.Execution
	Categories map[string]model.Category
	// TraceRules are the evaluated traceability rules (traceability report;
	// evaluated on the requirements and tests above).
	TraceRules []tracerules.Result
	// Risks of the project with their requirement IDs (risk report).
	Risks []model.Risk
	// Quality: the computed quality metrics (quality metrics report).
	Quality *analytics.QualityReport
	// Query is the filter applied by the caller (shown in the subtitle).
	Query string
	Now   time.Time
	// Lang is the language of the report's own texts (en, sv; "" and
	// unknown languages give English).
	Lang string
}

func (d *Data) loc() loc { return newLoc(d.Lang) }

// Build renders template id with params over d.
func Build(id string, d *Data, params map[string]string) (*export.Dataset, error) {
	t, ok := Lookup(id)
	if !ok {
		return nil, invalidf("unknown report template %q", id)
	}
	for _, p := range t.Params {
		if p.Required && strings.TrimSpace(params[p.Name]) == "" {
			return nil, invalidf("parameter %s is required for report %s", p.Name, id)
		}
	}
	ds, err := t.build(d, params)
	if err != nil {
		return nil, err
	}
	l := d.loc()
	ds.Title = l.templateName(t)
	ds.Lang = l.lang
	if ds.Subtitle == "" {
		ds.Subtitle = d.subtitle()
	}
	ds.GeneratedAt = d.now()
	return ds, nil
}

func (d *Data) now() time.Time {
	if d.Now.IsZero() {
		return time.Now().UTC()
	}
	return d.Now.UTC()
}

func (d *Data) subtitle() string {
	s := d.Project.Key + " – " + d.Project.Name
	if q := strings.TrimSpace(d.Query); q != "" {
		s += d.loc().t(" · Filter: ") + q
	}
	return s
}

// ---------------------------------------------------------------- helpers

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func tsPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

func itoa(n int) string { return strconv.Itoa(n) }

// pct formats a/b as a percentage ("–" when b is 0).
func pct(a, b int) string {
	if b == 0 {
		return "–"
	}
	return strconv.FormatFloat(float64(a)*100/float64(b), 'f', 1, 64) + "%"
}

// kv is one metric of a KPI section.
type kv struct{ k, v string }

// kpiSection is a two-column metric/value table.
// Titles and metric names are English and translated here.
func (l loc) kpiSection(name, title string, rows ...kv) export.Section {
	s := export.Section{Name: name, Title: l.t(title), Columns: []string{l.t("Metric"), l.t("Value")}}
	for _, r := range rows {
		s.Rows = append(s.Rows, []string{l.t(r.k), r.v})
	}
	return s
}

// countSection tabulates counts in the given order (unknown keys follow in
// alphabetical order; zero counts of known keys are shown). title and label
// are English and translated here; labels are already translated.
func (l loc) countSection(name, title, label string, counts map[string]int, order []string, labels map[string]string) export.Section {
	s := export.Section{Name: name, Title: l.t(title), Columns: []string{l.t(label), l.t("Count"), l.t("Share")}}
	total := 0
	for _, n := range counts {
		total += n
	}
	seen := map[string]bool{}
	keys := append([]string{}, order...)
	for _, k := range order {
		seen[k] = true
	}
	var extra []string
	for k := range counts {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	for _, k := range keys {
		lbl := k
		if v, ok := labels[k]; ok {
			lbl = v
		}
		s.Rows = append(s.Rows, []string{lbl, itoa(counts[k]), l.pct(counts[k], total)})
	}
	s.Rows = append(s.Rows, []string{l.t("Total"), itoa(total), l.pct(total, total)})
	return s
}

// humanize turns an enumeration value into a label ("non_functional" →
// "Non functional").
func humanize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

func (d *Data) categoryNames(ids []string) string {
	var out []string
	for _, id := range ids {
		if c, ok := d.Categories[id]; ok {
			out = append(out, c.Dimension+": "+c.Name)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// durationText formats milliseconds as a Go duration ("1m2.5s").
func durationText(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).String()
}

// executed reports whether an execution has a final result.
func executed(status string) bool {
	return status != model.ResultNotRun && status != model.ResultInProgress && status != ""
}

// defectLink renders a defect id as a URL using the project's template
// (ids that already are URLs are returned unchanged).
func defectLink(tmpl, id string) string {
	if strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://") {
		return id
	}
	if tmpl == "" || !strings.Contains(tmpl, "{id}") {
		return ""
	}
	return strings.ReplaceAll(tmpl, "{id}", id)
}

func uniqSorted(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i], out[j]) })
	return out
}

// naturalLess compares strings case-insensitively with digit runs compared
// numerically ("REQ-2" < "REQ-10").
func naturalLess(a, b string) bool {
	return naturalCompare(a, b) < 0
}

func naturalCompare(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			i, j := 0, 0
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := strings.TrimLeft(a[:i], "0"), strings.TrimLeft(b[:j], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}
