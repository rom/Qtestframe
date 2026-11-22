package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

// Features of the AI assistance.
const (
	DraftTests         = "draft_tests"
	ImproveRequirement = "improve_requirement"
	SummarizeRun       = "summarize_run"
	SuggestMappings    = "suggest_mappings"
	SuggestCriteria    = "suggest_criteria"
)

// Features lists the features.
var Features = []string{DraftTests, ImproveRequirement, SummarizeRun, SuggestMappings, SuggestCriteria}

// Limits of the answers.
const (
	MaxTests    = 8
	MaxSteps    = 25
	MaxDefects  = 8
	MaxMappings = 12
	MaxCriteria = 10
	// maxText bounds a single text value of an answer (characters).
	maxText = 8000
)

// ErrInvalidAnswer: the answer did not match the schema, also after the retry.
var ErrInvalidAnswer = errors.New("ai: the answer did not match the expected form")

// ---------------------------------------------------------------- input data

// Criterion is an acceptance (evaluation) criterion.
type Criterion struct {
	Number      int    `json:"number,omitempty"`
	Description string `json:"description"`
	Method      string `json:"method"`
	Threshold   string `json:"threshold,omitempty"`
}

// RequirementData is a requirement as sent to the service.
type RequirementData struct {
	Key         string      `json:"key"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Type        string      `json:"type,omitempty"`
	Obligation  string      `json:"obligation,omitempty"`
	Rationale   string      `json:"rationale,omitempty"`
	Criteria    []Criterion `json:"acceptance_criteria"`
}

// Finding is a finding of the requirement checks (internal/reqlang).
type Finding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ImproveData is the input of ImproveRequirement.
type ImproveData struct {
	Requirement RequirementData `json:"requirement"`
	Findings    []Finding       `json:"findings"`
}

// FailedStep is a step of an execution that did not pass.
type FailedStep struct {
	Number   int    `json:"number"`
	Action   string `json:"action"`
	Expected string `json:"expected"`
	Status   string `json:"status"`
	Actual   string `json:"actual,omitempty"`
}

// ExecutionData is a failed or blocked execution.
type ExecutionData struct {
	Test          string       `json:"test"`
	Title         string       `json:"title"`
	Status        string       `json:"status"`
	Configuration string       `json:"configuration,omitempty"`
	Notes         string       `json:"notes,omitempty"`
	Output        string       `json:"error_output,omitempty"`
	Steps         []FailedStep `json:"failed_steps,omitempty"`
	Defects       []string     `json:"linked_defects,omitempty"`
}

// DefectData is a defect linked to the run.
type DefectData struct {
	Key      string   `json:"key"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Severity string   `json:"severity"`
	Tests    []string `json:"tests,omitempty"`
}

// RunData is the input of SummarizeRun.
type RunData struct {
	Run struct {
		Name        string         `json:"name"`
		Status      string         `json:"status"`
		Environment string         `json:"environment,omitempty"`
		Build       string         `json:"build,omitempty"`
		Results     map[string]int `json:"results"`
	} `json:"run"`
	Executions []ExecutionData `json:"failed_executions"`
	Defects    []DefectData    `json:"linked_defects"`
}

// MappingData is the input of SuggestMappings.
type MappingData struct {
	Kind        string              `json:"kind"`
	Key         string              `json:"key"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Extra       string              `json:"details,omitempty"`
	Mapped      map[string][]string `json:"already_mapped"`
}

// ---------------------------------------------------------------- answers

// Answer is the result type of a feature.
type Answer interface {
	validate() error
}

// TestStep is a step of a drafted test.
type TestStep struct {
	Action   string `json:"action"`
	Data     string `json:"data"`
	Expected string `json:"expected"`
}

// DraftTest is a drafted test.
type DraftTest struct {
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Preconditions  string     `json:"preconditions"`
	TestData       string     `json:"test_data"`
	ExpectedResult string     `json:"expected_result"`
	Steps          []TestStep `json:"steps"`
	Criteria       []int      `json:"criteria"`
}

// DraftTestsAnswer is the answer of DraftTests.
type DraftTestsAnswer struct {
	Tests []DraftTest `json:"tests"`
}

// ImproveAnswer is the answer of ImproveRequirement.
type ImproveAnswer struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Explanation string   `json:"explanation"`
	Addresses   []string `json:"addresses"`
}

// Cause is a likely cause of failures.
type Cause struct {
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Tests       []string `json:"tests"`
}

// DefectDraft is a suggested defect.
type DefectDraft struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Tests       []string `json:"tests"`
}

// RunSummaryAnswer is the answer of SummarizeRun.
type RunSummaryAnswer struct {
	Summary string        `json:"summary"`
	Causes  []Cause       `json:"causes"`
	Defects []DefectDraft `json:"defects"`
}

// Mapping is a suggested framework mapping.
type Mapping struct {
	Framework string `json:"framework"`
	ID        string `json:"id"`
	Reason    string `json:"reason"`
}

// MappingsAnswer is the answer of SuggestMappings.
type MappingsAnswer struct {
	Mappings []Mapping `json:"mappings"`
}

// CriteriaAnswer is the answer of SuggestCriteria.
type CriteriaAnswer struct {
	Criteria []Criterion `json:"criteria"`
}

func checkText(field, s string, required bool) error {
	if required && strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is empty", field)
	}
	if utf8.RuneCountInString(s) > maxText {
		return fmt.Errorf("%s is too long", field)
	}
	return nil
}

func (a *DraftTestsAnswer) validate() error {
	if len(a.Tests) == 0 {
		return errors.New("tests is empty")
	}
	if len(a.Tests) > MaxTests {
		return fmt.Errorf("more than %d tests", MaxTests)
	}
	for i, t := range a.Tests {
		if err := errors.Join(checkText("title", t.Title, true), checkText("description", t.Description, false),
			checkText("preconditions", t.Preconditions, false), checkText("test_data", t.TestData, false),
			checkText("expected_result", t.ExpectedResult, false)); err != nil {
			return fmt.Errorf("test %d: %w", i+1, err)
		}
		if len(t.Steps) == 0 || len(t.Steps) > MaxSteps {
			return fmt.Errorf("test %d: 1 to %d steps", i+1, MaxSteps)
		}
		for j, s := range t.Steps {
			if err := errors.Join(checkText("action", s.Action, true), checkText("data", s.Data, false), checkText("expected", s.Expected, false)); err != nil {
				return fmt.Errorf("test %d step %d: %w", i+1, j+1, err)
			}
		}
	}
	return nil
}

func (a *ImproveAnswer) validate() error {
	return errors.Join(checkText("title", a.Title, true), checkText("description", a.Description, true), checkText("explanation", a.Explanation, false))
}

// Severities of drafted defects.
var Severities = []string{"critical", "major", "minor", "trivial"}

func (a *RunSummaryAnswer) validate() error {
	if err := checkText("summary", a.Summary, true); err != nil {
		return err
	}
	if len(a.Defects) > MaxDefects {
		return fmt.Errorf("more than %d defects", MaxDefects)
	}
	for i, c := range a.Causes {
		if err := errors.Join(checkText("title", c.Title, true), checkText("explanation", c.Explanation, false)); err != nil {
			return fmt.Errorf("cause %d: %w", i+1, err)
		}
	}
	for i, d := range a.Defects {
		if err := errors.Join(checkText("title", d.Title, true), checkText("description", d.Description, false)); err != nil {
			return fmt.Errorf("defect %d: %w", i+1, err)
		}
		if !slices.Contains(Severities, d.Severity) {
			return fmt.Errorf("defect %d: severity must be one of %s", i+1, strings.Join(Severities, ", "))
		}
	}
	return nil
}

func (a *MappingsAnswer) validate() error {
	if len(a.Mappings) > 2*MaxMappings {
		return fmt.Errorf("more than %d mappings", MaxMappings)
	}
	for i, m := range a.Mappings {
		if err := errors.Join(checkText("framework", m.Framework, true), checkText("id", m.ID, true), checkText("reason", m.Reason, false)); err != nil {
			return fmt.Errorf("mapping %d: %w", i+1, err)
		}
	}
	return nil
}

// Methods are the evaluation methods of criteria.
var Methods = []string{"test", "inspection", "analysis", "demonstration", "review"}

func (a *CriteriaAnswer) validate() error {
	if len(a.Criteria) == 0 {
		return errors.New("criteria is empty")
	}
	if len(a.Criteria) > MaxCriteria {
		return fmt.Errorf("more than %d criteria", MaxCriteria)
	}
	for i, c := range a.Criteria {
		if err := errors.Join(checkText("description", c.Description, true), checkText("threshold", c.Threshold, false)); err != nil {
			return fmt.Errorf("criterion %d: %w", i+1, err)
		}
		if !slices.Contains(Methods, c.Method) {
			return fmt.Errorf("criterion %d: method must be one of %s", i+1, strings.Join(Methods, ", "))
		}
		if c.Number != 0 {
			return fmt.Errorf("criterion %d: unknown field number", i+1)
		}
	}
	return nil
}

// ---------------------------------------------------------------- calls

// Call is one request of a feature.
type Call struct {
	Feature string
	// Lang is the language of the prompt and the answer ("en", "sv").
	Lang string
	// Data is the item data (RequirementData, ImproveData, RunData,
	// MappingData).
	Data any
	// Mapping: what the item is ("requirement", "risk") and the allowed
	// frameworks as "id: meaning" lines and their ids.
	Kind         string
	Frameworks   []string
	FrameworkIDs []string
	MaxTokens    int
	Temperature  *float64
	// OnDelta receives the text of the answer as it is written (may be nil).
	OnDelta func(string)
}

// Result is the outcome of a call.
type Result struct {
	Feature       string `json:"feature"`
	PromptVersion string `json:"prompt_version"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Usage         Usage  `json:"usage"`
	// Retried: the first answer was invalid and the service was asked again.
	Retried bool `json:"retried"`
}

func maxOf(feature string) int {
	switch feature {
	case DraftTests:
		return MaxTests
	case SummarizeRun:
		return MaxDefects
	case SuggestMappings:
		return MaxMappings
	case SuggestCriteria:
		return MaxCriteria
	}
	return 0
}

// Prompt renders the system and user prompts of a call.
func Prompt(c Call) (system, user string, err error) {
	if !slices.Contains(Features, c.Feature) {
		return "", "", fmt.Errorf("ai: unknown feature %q", c.Feature)
	}
	data, err := encodeData(c.Data)
	if err != nil {
		return "", "", err
	}
	v := promptVars{Data: data, Max: maxOf(c.Feature), Kind: c.Kind, Frameworks: c.Frameworks, FrameworkIDs: strings.Join(c.FrameworkIDs, "|")}
	if system, err = render(c.Lang, c.Feature+".system", v); err != nil {
		return "", "", err
	}
	if user, err = render(c.Lang, c.Feature+".user", v); err != nil {
		return "", "", err
	}
	return system, user, nil
}

// extractJSON returns the JSON object of an answer (without Markdown
// fences or text around it).
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}

// parse decodes an answer strictly (no unknown fields) and validates it.
func parse(text string, out Answer) error {
	// decode into a fresh value: an invalid first answer leaves nothing behind
	fresh := reflect.New(reflect.TypeOf(out).Elem())
	dec := json.NewDecoder(bytes.NewReader([]byte(extractJSON(text))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(fresh.Interface()); err != nil {
		return fmt.Errorf("not a JSON object of the schema (%v)", err)
	}
	if dec.More() {
		return errors.New("more than one JSON value")
	}
	if err := fresh.Interface().(Answer).validate(); err != nil {
		return err
	}
	reflect.ValueOf(out).Elem().Set(fresh.Elem())
	return nil
}

// Run asks the provider and decodes the answer into out (a pointer to the
// feature's answer type). An answer that does not match the schema is
// retried once, with the error; the result counts the tokens of both.
func Run(ctx context.Context, p Provider, c Call, out Answer) (*Result, error) {
	system, user, err := Prompt(c)
	if err != nil {
		return nil, err
	}
	res := &Result{Feature: c.Feature, PromptVersion: PromptVersion, Provider: p.Name(), Model: p.ModelName()}
	req := Request{System: system, Messages: []Message{{Role: "user", Text: user}}, MaxTokens: c.MaxTokens, Temperature: c.Temperature}
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := p.Complete(ctx, req, c.OnDelta)
		if err != nil {
			return res, err
		}
		res.Usage.Add(resp.Usage)
		if resp.Model != "" {
			res.Model = resp.Model
		}
		perr := parse(resp.Text, out)
		if perr == nil {
			return res, nil
		}
		if resp.Truncated {
			perr = fmt.Errorf("the answer was cut off at the token limit (%v)", perr)
		}
		if attempt == 1 {
			return res, fmt.Errorf("%w: %v", ErrInvalidAnswer, perr)
		}
		res.Retried = true
		retry, err := render(c.Lang, "retry", promptVars{Error: perr.Error()})
		if err != nil {
			return res, err
		}
		req.Messages = append(req.Messages, Message{Role: "assistant", Text: resp.Text}, Message{Role: "user", Text: retry})
		if c.OnDelta != nil {
			c.OnDelta("\n")
		}
	}
	return res, ErrInvalidAnswer
}
