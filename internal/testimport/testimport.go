// Package testimport reads the test case exports of other test management
// tools (TestRail, Xray, Zephyr Scale/Squad, qTest and Azure Test Plans)
// into a common list of cases with their steps, folders, requirement
// references, custom field values and, where the format has them, past
// results. Nothing is stored here: the API previews the cases, maps
// priorities, statuses and custom fields and creates or updates tests (see
// internal/api/testimport.go).
package testimport

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Sources (the tools a file comes from).
const (
	SourceTestRail = "testrail"
	SourceXray     = "xray"
	SourceZephyr   = "zephyr"
	SourceQTest    = "qtest"
	SourceAzure    = "azure"
)

// Sources lists the supported sources.
var Sources = []string{SourceTestRail, SourceXray, SourceZephyr, SourceQTest, SourceAzure}

// Formats.
const (
	FormatCSV  = "csv"
	FormatXML  = "xml"
	FormatJSON = "json"
	FormatXLSX = "xlsx"
)

// SourceFormats are the file formats each source supports.
var SourceFormats = map[string][]string{
	SourceTestRail: {FormatCSV, FormatXML},
	SourceXray:     {FormatCSV, FormatJSON},
	SourceZephyr:   {FormatCSV, FormatXML},
	SourceQTest:    {FormatXLSX, FormatCSV},
	SourceAzure:    {FormatCSV},
}

// SourceNames are the display names of the sources.
var SourceNames = map[string]string{
	SourceTestRail: "TestRail",
	SourceXray:     "Xray",
	SourceZephyr:   "Zephyr",
	SourceQTest:    "qTest",
	SourceAzure:    "Azure Test Plans",
}

// Limits.
const (
	// DefaultMaxCases is the largest number of cases a file may hold.
	DefaultMaxCases = 5000
	maxUnzipped     = 128 << 20
	maxSteps        = 500
)

// ErrInvalid marks problems with the file or the options; the message is
// meant for the user.
var ErrInvalid = errors.New("invalid import")

func invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Step is one step of a case.
type Step struct {
	Action   string `json:"action"`
	Data     string `json:"data"`
	Expected string `json:"expected"`
}

// Execution is a past result of a case.
type Execution struct {
	// Run names the run, cycle or test execution of the source; RunID is
	// its identifier there (Run when the file has none).
	Run   string `json:"run"`
	RunID string `json:"run_id"`
	// Status is the model result (passed, failed, blocked, skipped) and
	// RawStatus the source's value.
	Status     string     `json:"status"`
	RawStatus  string     `json:"raw_status"`
	ExecutedBy string     `json:"executed_by"`
	ExecutedAt *time.Time `json:"executed_at"`
	Comment    string     `json:"comment"`
}

// Case is a test case found in a file.
type Case struct {
	// ExternalID is the case's identifier in the source ("C12", "CALC-7",
	// "4711"); "" when the file has none (a re-import then duplicates it).
	ExternalID    string `json:"external_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Preconditions string `json:"preconditions"`
	// ExpectedResult is the overall expected result (text templates).
	ExpectedResult string `json:"expected_result"`
	Steps          []Step `json:"steps"`
	// Type is the source's test type (Manual, Cucumber, Generic,
	// Functional …); Automated is set for automated types.
	Type      string `json:"type"`
	Automated bool   `json:"automated"`
	// Gherkin is the scenario text of a BDD (Cucumber) case.
	Gherkin string `json:"gherkin,omitempty"`
	// Definition is the definition of a generic (unstructured) test.
	Definition string `json:"definition,omitempty"`
	// RawPriority and RawStatus are the source's values; Priority and
	// Status the mapped model codes ("" = the default).
	RawPriority  string   `json:"raw_priority"`
	Priority     string   `json:"priority"`
	RawStatus    string   `json:"raw_status"`
	Status       string   `json:"status"`
	Folder       string   `json:"folder"` // "A/B"
	Tags         []string `json:"tags"`
	Refs         []string `json:"refs"`
	Owner        string   `json:"owner"`
	EstimatedMin int      `json:"estimated_minutes"`
	// Custom are the values of the source's other fields by field name.
	Custom     map[string]string `json:"custom,omitempty"`
	Executions []Execution       `json:"executions,omitempty"`
	Row        int               `json:"row"`
	Location   string            `json:"location"`
	Warnings   []string          `json:"warnings"`
}

func (c *Case) warn(format string, a ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, a...))
}

// Value is a distinct source value of a mapped field with its target.
type Value struct {
	Value  string `json:"value"`
	Count  int    `json:"count"`
	Target string `json:"target"` // model code, "" = default
	// Guessed is false when the target was chosen by the user.
	Guessed bool `json:"guessed"`
}

// Field is a source field without a built-in target (a custom field
// candidate) and the custom field it maps to ("" = not imported).
type Field struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Sample  string `json:"sample"`
	Target  string `json:"target"`
	Guessed bool   `json:"guessed"`
}

// Options control parsing and mapping.
type Options struct {
	// Format is csv, xml, json or xlsx; "" = from the file name and content.
	Format string
	// Priorities and Statuses map source values (case-insensitive) to
	// model codes, overriding the guesses; "" maps to the default.
	Priorities map[string]string
	Statuses   map[string]string
	// Fields map source field names (case-insensitive) to custom field
	// keys of CustomFields; "" leaves the field out.
	Fields map[string]string
	// CustomFields are the project's test custom fields.
	CustomFields []model.CustomField
	// MaxCases limits the result (0 = DefaultMaxCases).
	MaxCases int
}

// Result is a parsed file.
type Result struct {
	Source     string  `json:"source"`
	Format     string  `json:"format"`
	Cases      []Case  `json:"cases"`
	Priorities []Value `json:"priorities"`
	Statuses   []Value `json:"statuses"`
	Fields     []Field `json:"fields"`
	// Executions counts the past results in the file.
	Executions int      `json:"executions"`
	Warnings   []string `json:"warnings"`
	Errors     []string `json:"errors"`
}

func (r *Result) warn(format string, a ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
}

func (r *Result) errorf(format string, a ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, a...))
}

// ValidSource reports whether s names a source; it accepts some aliases
// and returns the canonical id.
func ValidSource(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "testrail", "test-rail":
		return SourceTestRail, true
	case "xray", "jira", "xray-jira":
		return SourceXray, true
	case "zephyr", "zephyr-scale", "zephyr-squad", "squad":
		return SourceZephyr, true
	case "qtest", "q-test", "tricentis-qtest":
		return SourceQTest, true
	case "azure", "azure-test-plans", "azure-devops", "ado":
		return SourceAzure, true
	}
	return "", false
}

// Parse reads the export of source. Errors wrapping ErrInvalid describe
// unusable files or options.
func Parse(source string, data []byte, filename string, opt Options) (res *Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			res, err = nil, invalidf("the file could not be read (%v)", p)
		}
	}()
	src, ok := ValidSource(source)
	if !ok {
		return nil, invalidf("unknown source %q (want %s)", source, strings.Join(Sources, ", "))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, invalidf("the file is empty")
	}
	format, err := DetectFormat(src, opt.Format, filename, data)
	if err != nil {
		return nil, err
	}
	res = &Result{Source: src, Format: format, Cases: []Case{}, Warnings: []string{}, Errors: []string{}}
	switch src {
	case SourceTestRail:
		if format == FormatXML {
			err = parseTestRailXML(data, res)
		} else {
			err = parseTestRailCSV(data, res)
		}
	case SourceXray:
		if format == FormatJSON {
			err = parseXrayJSON(data, res)
		} else {
			err = parseXrayCSV(data, res)
		}
	case SourceZephyr:
		if format == FormatXML {
			err = parseZephyrXML(data, res)
		} else {
			err = parseZephyrCSV(data, res)
		}
	case SourceQTest:
		var t *table
		if format == FormatXLSX {
			t, err = readXLSX(data, qtestHeader)
		} else {
			t, err = readCSV(data)
		}
		if err == nil {
			err = parseQTest(t, res)
		}
	case SourceAzure:
		err = parseAzureCSV(data, res)
	}
	if err != nil {
		return nil, err
	}
	max := opt.MaxCases
	if max <= 0 {
		max = DefaultMaxCases
	}
	if len(res.Cases) > max {
		return nil, invalidf("the file contains %d test cases; at most %d can be imported at once", len(res.Cases), max)
	}
	finish(res, opt)
	if len(res.Cases) == 0 && len(res.Errors) == 0 {
		res.warn("no test cases were found in the file")
	}
	return res, nil
}

// finish cleans the cases and applies the mappings.
func finish(res *Result, opt Options) {
	prios, stats := map[string]*Value{}, map[string]*Value{}
	fields := map[string]*Field{}
	var porder, sorder, forder []string
	for i := range res.Cases {
		c := &res.Cases[i]
		c.Title = oneLine(c.Title)
		c.Description = strings.TrimSpace(c.Description)
		c.Preconditions = strings.TrimSpace(c.Preconditions)
		c.ExpectedResult = strings.TrimSpace(c.ExpectedResult)
		c.ExternalID = strings.TrimSpace(c.ExternalID)
		c.Folder = cleanFolder(c.Folder)
		c.Tags = uniq(c.Tags)
		c.Refs = uniq(c.Refs)
		if c.Warnings == nil {
			c.Warnings = []string{}
		}
		if c.Tags == nil {
			c.Tags = []string{}
		}
		if c.Refs == nil {
			c.Refs = []string{}
		}
		var steps []Step
		for _, s := range c.Steps {
			s.Action, s.Data, s.Expected = strings.TrimSpace(s.Action), strings.TrimSpace(s.Data), strings.TrimSpace(s.Expected)
			if s.Action != "" || s.Data != "" || s.Expected != "" {
				steps = append(steps, s)
			}
		}
		if len(steps) > maxSteps {
			c.warn("only the first %d of %d steps are imported", maxSteps, len(steps))
			steps = steps[:maxSteps]
		}
		if steps == nil {
			steps = []Step{}
		}
		c.Steps = steps
		if c.Title == "" {
			c.Title = firstLine(c.Description)
			if c.Title == "" {
				c.Title = c.ExternalID
			}
			if c.Title != "" {
				c.warn("no title: %q is used", c.Title)
			}
		}
		if raw := strings.TrimSpace(c.RawPriority); raw != "" {
			k := strings.ToLower(raw)
			if prios[k] == nil {
				target, guessed := mapped(opt.Priorities, raw, guessPriority, model.Priorities)
				prios[k] = &Value{Value: raw, Target: target, Guessed: guessed}
				porder = append(porder, k)
			}
			prios[k].Count++
			c.Priority = prios[k].Target
		}
		if raw := strings.TrimSpace(c.RawStatus); raw != "" {
			k := strings.ToLower(raw)
			if stats[k] == nil {
				target, guessed := mapped(opt.Statuses, raw, guessStatus, model.TestStatuses)
				stats[k] = &Value{Value: raw, Target: target, Guessed: guessed}
				sorder = append(sorder, k)
			}
			stats[k].Count++
			c.Status = stats[k].Target
		}
		for _, name := range sortedKeys(c.Custom) {
			v := strings.TrimSpace(c.Custom[name])
			if v == "" {
				delete(c.Custom, name)
				continue
			}
			k := strings.ToLower(name)
			if fields[k] == nil {
				target, guessed := mappedField(opt.Fields, name, opt.CustomFields)
				fields[k] = &Field{Name: name, Sample: clip(v, 80), Target: target, Guessed: guessed}
				forder = append(forder, k)
			}
			fields[k].Count++
		}
		for j := range c.Executions {
			if c.Executions[j].RunID == "" {
				c.Executions[j].RunID = c.Executions[j].Run
			}
		}
		res.Executions += len(c.Executions)
	}
	res.Priorities, res.Statuses, res.Fields = []Value{}, []Value{}, []Field{}
	for _, k := range porder {
		res.Priorities = append(res.Priorities, *prios[k])
	}
	for _, k := range sorder {
		res.Statuses = append(res.Statuses, *stats[k])
	}
	for _, k := range forder {
		res.Fields = append(res.Fields, *fields[k])
	}
	sort.SliceStable(res.Fields, func(i, j int) bool { return strings.ToLower(res.Fields[i].Name) < strings.ToLower(res.Fields[j].Name) })
}

// mapped returns the target of raw: the user's choice, else the guess.
func mapped(choice map[string]string, raw string, guess func(string) string, valid []string) (string, bool) {
	for k, v := range choice {
		if strings.EqualFold(strings.TrimSpace(k), raw) {
			v = strings.TrimSpace(v)
			if v == "" || model.Valid(valid, v) {
				return v, false
			}
		}
	}
	return guess(raw), true
}

// DetectFormat returns the format named by explicit, else by the file
// extension, else by sniffing data.
func DetectFormat(source, explicit, filename string, data []byte) (string, error) {
	format := ""
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(explicit), ".")) {
	case "":
	case "csv", "txt", "tsv":
		format = FormatCSV
	case "xml":
		format = FormatXML
	case "json":
		format = FormatJSON
	case "xlsx", "excel":
		format = FormatXLSX
	default:
		return "", invalidf("unsupported format %q (want csv, xml, json or xlsx)", explicit)
	}
	if format == "" {
		switch strings.ToLower(path.Ext(filename)) {
		case ".csv", ".txt", ".tsv":
			format = FormatCSV
		case ".xml":
			format = FormatXML
		case ".json":
			format = FormatJSON
		case ".xlsx", ".xlsm":
			format = FormatXLSX
		case ".xls":
			return "", invalidf("old binary Excel files (.xls) are not supported; save the file as .xlsx")
		}
	}
	if format == "" {
		head := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
		switch {
		case bytes.HasPrefix(data, []byte("PK\x03\x04")):
			format = FormatXLSX
		case bytes.HasPrefix(head, []byte("<")):
			format = FormatXML
		case bytes.HasPrefix(head, []byte("{")) || bytes.HasPrefix(head, []byte("[")):
			format = FormatJSON
		default:
			format = FormatCSV
		}
	}
	allowed := SourceFormats[source]
	for _, f := range allowed {
		if f == format {
			return format, nil
		}
	}
	return "", invalidf("%s files of %s are not supported (use %s)", format, SourceNames[source], strings.Join(allowed, " or "))
}

// FolderParts splits "A/B" into its names.
func FolderParts(folder string) []string {
	var out []string
	for _, p := range strings.Split(folder, "/") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cleanFolder(f string) string { return strings.Join(FolderParts(f), "/") }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniq(list []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}
