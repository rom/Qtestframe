package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/gherkin"
	"github.com/rom/qtestframe/internal/model"
)

// BDD authoring (migration 0070): tests in the Gherkin format and the
// step library of a project.

// Limits of the step library.
const (
	MaxStepDefinitions   = 2000
	MaxStepPattern       = 500
	MaxStepDescription   = 2000
	gherkinStepIDPrefix  = "gherkin:"
	maxReportedGherkinEr = 3
)

// GherkinError is a test whose Gherkin does not parse: ErrInvalid with the
// errors (line and message) of the document.
type GherkinError struct {
	Errors gherkin.Errors
}

func (e *GherkinError) Error() string {
	es := e.Errors
	more := ""
	if len(es) > maxReportedGherkinEr {
		more = " (" + strconv.Itoa(len(es)-maxReportedGherkinEr) + " more)"
		es = es[:maxReportedGherkinEr]
	}
	return "invalid: gherkin: " + es.Error() + more
}

func (e *GherkinError) Unwrap() error { return ErrInvalid }

func gherkinErr(err error) error {
	var es gherkin.Errors
	if errors.As(err, &es) {
		return &GherkinError{Errors: es}
	}
	return Invalidf("gherkin: %v", err)
}

// validateFormat checks the format of a test and, for a Gherkin test,
// that its feature parses (an empty one is allowed while drafting).
func validateFormat(t *model.TestCase) error {
	switch t.Format {
	case "":
		t.Format = model.FormatSteps
	case model.FormatSteps, model.FormatGherkin:
	default:
		return Invalidf("invalid format %q (steps or gherkin)", t.Format)
	}
	t.Gherkin = strings.ReplaceAll(t.Gherkin, "\r\n", "\n")
	if len(t.Gherkin) > gherkin.MaxSource {
		return Invalidf("the Gherkin is larger than %d bytes", gherkin.MaxSource)
	}
	if t.Format == model.FormatGherkin && strings.TrimSpace(t.Gherkin) != "" {
		if _, err := gherkin.Parse(t.Gherkin); err != nil {
			return gherkinErr(err)
		}
	}
	return nil
}

// gherkinInstances returns the executable scenarios of a Gherkin test.
func gherkinInstances(src string) ([]gherkin.Instance, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	doc, err := gherkin.Parse(src)
	if err != nil {
		return nil, gherkinErr(err)
	}
	return gherkin.Expand(doc), nil
}

// GherkinStepID is the step id of the step results of a Gherkin step:
// "gherkin:<line>".
func GherkinStepID(line int) string { return gherkinStepIDPrefix + strconv.Itoa(line) }

// instanceSteps turns the steps of a scenario into steps of a run: the
// keyword and text are the action, the doc string or table follows it.
func instanceSteps(in gherkin.Instance) []model.TestStep {
	out := make([]model.TestStep, 0, len(in.Steps))
	for _, st := range in.Steps {
		action := st.Keyword + " " + st.Text
		if st.Argument != "" {
			action += "\n" + st.Argument
		}
		out = append(out, model.TestStep{ID: GherkinStepID(st.Line), Action: action})
	}
	return out
}

// ---------------------------------------------------------------- step library

const stepDefCols = `id, project_id, keyword, pattern, kind, description, created_by, created_at, updated_at`

func scanStepDef(r interface{ Scan(...any) error }) (*model.StepDefinition, error) {
	var d model.StepDefinition
	if err := r.Scan(&d.ID, &d.ProjectID, &d.Keyword, &d.Pattern, &d.Kind, &d.Description, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.CreatedAt, d.UpdatedAt = d.CreatedAt.UTC(), d.UpdatedAt.UTC()
	return &d, nil
}

// ValidateStepDefinition normalises and checks a step definition.
func ValidateStepDefinition(d *model.StepDefinition) error {
	d.Pattern = strings.TrimSpace(d.Pattern)
	d.Description = strings.TrimSpace(d.Description)
	d.Keyword = strings.ToLower(strings.TrimSpace(d.Keyword))
	if d.Kind == "" {
		d.Kind = gherkin.KindCucumber
	}
	switch d.Keyword {
	case "", "given", "when", "then":
	default:
		return Invalidf("invalid keyword %q (given, when, then or empty)", d.Keyword)
	}
	if d.Pattern == "" {
		return Invalidf("the pattern is required")
	}
	if utf8.RuneCountInString(d.Pattern) > MaxStepPattern {
		return Invalidf("the pattern is longer than %d characters", MaxStepPattern)
	}
	if utf8.RuneCountInString(d.Description) > MaxStepDescription {
		return Invalidf("the description is longer than %d characters", MaxStepDescription)
	}
	if strings.ContainsAny(d.Pattern, "\n\r") {
		return Invalidf("the pattern must be on one line")
	}
	if _, err := gherkin.Compile(d.Pattern, d.Kind); err != nil {
		return Invalidf("pattern: %v", err)
	}
	return nil
}

// ListStepDefinitions returns the step library of a project, by pattern.
func (s *Store) ListStepDefinitions(ctx context.Context, tenantID, projectID string) ([]model.StepDefinition, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+stepDefCols+` FROM step_definitions WHERE tenant_id = ? AND project_id = ? ORDER BY pattern`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StepDefinition{}
	for rows.Next() {
		d, err := scanStepDef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetStepDefinition returns one step definition.
func (s *Store) GetStepDefinition(ctx context.Context, tenantID, id string) (*model.StepDefinition, error) {
	d, err := scanStepDef(s.c(ctx).row(ctx, `SELECT `+stepDefCols+` FROM step_definitions WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return d, mapErr(err, "step definition")
}

// CreateStepDefinition adds a step definition to a project's library.
func (s *Store) CreateStepDefinition(ctx context.Context, tenantID string, d *model.StepDefinition) error {
	if err := ValidateStepDefinition(d); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, d.ProjectID); err != nil {
		return err
	}
	var n int
	if err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM step_definitions WHERE project_id = ?`, d.ProjectID).Scan(&n); err != nil {
		return err
	}
	if n >= MaxStepDefinitions {
		return Invalidf("a step library has at most %d steps", MaxStepDefinitions)
	}
	d.ID = newID()
	d.CreatedAt = s.now().UTC()
	d.UpdatedAt = d.CreatedAt
	_, err := s.c(ctx).exec(ctx, `INSERT INTO step_definitions (id, tenant_id, project_id, keyword, pattern, kind, description, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, tenantID, d.ProjectID, d.Keyword, d.Pattern, d.Kind, d.Description, d.CreatedBy, d.CreatedAt, d.UpdatedAt)
	return mapErr(err, "a step with this pattern")
}

// UpdateStepDefinition stores the keyword, pattern, kind and description
// (the tests are not changed: see RenameStepDefinition).
func (s *Store) UpdateStepDefinition(ctx context.Context, tenantID string, d *model.StepDefinition) error {
	if err := ValidateStepDefinition(d); err != nil {
		return err
	}
	d.UpdatedAt = s.now().UTC()
	res, err := s.c(ctx).exec(ctx, `UPDATE step_definitions SET keyword = ?, pattern = ?, kind = ?, description = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		d.Keyword, d.Pattern, d.Kind, d.Description, d.UpdatedAt, d.ID, tenantID)
	if err != nil {
		return mapErr(err, "a step with this pattern")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "step definition")
	}
	return nil
}

// DeleteStepDefinition removes a step definition.
func (s *Store) DeleteStepDefinition(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM step_definitions WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "step definition")
	}
	return nil
}

// GherkinSources returns the Gherkin of the project's Gherkin tests, by key.
func (s *Store) GherkinSources(ctx context.Context, tenantID, projectID string) ([]gherkin.Source, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, key, title, gherkin FROM test_cases WHERE tenant_id = ? AND project_id = ? AND format = ? AND gherkin <> '' ORDER BY seq`,
		tenantID, projectID, model.FormatGherkin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gherkin.Source
	for rows.Next() {
		var src gherkin.Source
		if err := rows.Scan(&src.TestID, &src.TestKey, &src.TestTitle, &src.Text); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func libraryDefs(list []model.StepDefinition) []gherkin.Definition {
	defs := make([]gherkin.Definition, len(list))
	for i, d := range list {
		defs[i] = gherkin.Definition{ID: d.ID, Pattern: d.Pattern, Kind: d.Kind}
	}
	return defs
}

// StepLibrary is the step library of a project with where each step is
// used and the steps no definition matches.
type StepLibrary struct {
	Steps []model.StepDefinition `json:"items"`
	// Uses by definition id.
	Uses      map[string][]gherkin.Use `json:"uses"`
	Undefined []gherkin.Undefined      `json:"undefined"`
	// Unparsed: Gherkin tests with errors (test id → first error); their
	// steps up to the error are counted.
	Unparsed map[string]string `json:"unparsed"`
}

// StepLibraryUsage analyses the project's Gherkin tests against its library.
func (s *Store) StepLibraryUsage(ctx context.Context, tenantID, projectID string) (*StepLibrary, error) {
	list, err := s.ListStepDefinitions(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	srcs, err := s.GherkinSources(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	a := gherkin.Analyse(libraryDefs(list), srcs)
	for i := range list {
		list[i].UsageCount = len(a.Uses[list[i].ID])
	}
	und := a.Undefined
	if und == nil {
		und = []gherkin.Undefined{}
	}
	return &StepLibrary{Steps: list, Uses: a.Uses, Undefined: und, Unparsed: a.Unparsed}, nil
}

// RenamedTest is a test whose steps a rename rewrote.
type RenamedTest struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Steps   int    `json:"steps"`
	Version int    `json:"version"`
}

// RenameStepDefinition changes the pattern of a step definition to a new
// Cucumber expression and rewrites every step of the project's Gherkin
// tests that matched the old pattern, keeping their parameters. Each
// changed test gets a new version.
func (s *Store) RenameStepDefinition(ctx context.Context, tenantID, id, pattern string) (*model.StepDefinition, []RenamedTest, error) {
	d, err := s.GetStepDefinition(ctx, tenantID, id)
	if err != nil {
		return nil, nil, err
	}
	from, err := gherkin.Compile(d.Pattern, d.Kind)
	if err != nil {
		return nil, nil, Invalidf("the current pattern: %v", err)
	}
	nd := *d
	nd.Pattern, nd.Kind = pattern, gherkin.KindCucumber
	if err := ValidateStepDefinition(&nd); err != nil {
		return nil, nil, err
	}
	to, _ := gherkin.Compile(nd.Pattern, nd.Kind)
	if from.Params() != to.Params() {
		return nil, nil, Invalidf("the new pattern has %d parameters, the current one %d", to.Params(), from.Params())
	}
	srcs, err := s.GherkinSources(ctx, tenantID, d.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	changed := []RenamedTest{}
	err = s.tx(ctx, func(c conn) error {
		for _, src := range srcs {
			if _, perr := gherkin.Parse(src.Text); perr != nil {
				continue // a test with errors is left alone
			}
			text, n, err := gherkin.Rename(src.Text, from, to)
			if err != nil {
				return Invalidf("%s: %v", src.TestKey, err)
			}
			if n == 0 {
				continue
			}
			var version int
			if err := c.row(ctx, `SELECT version FROM test_cases WHERE id = ?`, src.TestID).Scan(&version); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `UPDATE test_cases SET gherkin = ?, version = ?, updated_at = ? WHERE id = ? AND version = ?`, text, version+1, s.now(), src.TestID, version); err != nil {
				return err
			}
			changed = append(changed, RenamedTest{ID: src.TestID, Key: src.TestKey, Steps: n, Version: version + 1})
		}
		nd.UpdatedAt = s.now().UTC()
		if _, err := c.exec(ctx, `UPDATE step_definitions SET pattern = ?, kind = ?, updated_at = ? WHERE id = ?`, nd.Pattern, nd.Kind, nd.UpdatedAt, nd.ID); err != nil {
			return mapErr(err, "a step with this pattern")
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &nd, changed, nil
}

// GherkinStepResult is the latest result of a step of a Gherkin test.
type GherkinStepResult struct {
	Line   int    `json:"line"`
	Status string `json:"status"`
	// Results: how many scenario instances (outline rows) ran the step.
	Results int `json:"results"`
}

// LastGherkinResults maps the step results of the latest execution of a
// Gherkin test (that has any result) back to the lines of its steps; a
// step run in several scenarios (a Background, Examples rows) combines
// their results (failed first).
func (s *Store) LastGherkinResults(ctx context.Context, tenantID, testID string) (string, []GherkinStepResult, error) {
	var execID string
	err := s.c(ctx).row(ctx, `SELECT e.id FROM test_executions e WHERE e.tenant_id = ? AND e.test_case_id = ?
		AND EXISTS (SELECT 1 FROM step_results sr WHERE sr.execution_id = e.id AND sr.step_id LIKE 'gherkin:%' AND sr.status <> 'not_run')
		ORDER BY COALESCE(e.finished_at, e.started_at) DESC, e.position DESC LIMIT 1`, tenantID, testID).Scan(&execID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", []GherkinStepResult{}, nil
	}
	if err != nil {
		return "", nil, err
	}
	rows, err := s.c(ctx).query(ctx, `SELECT step_id, status FROM step_results WHERE execution_id = ? AND step_id LIKE 'gherkin:%'`, execID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	byLine := map[int][]string{}
	var lines []int
	for rows.Next() {
		var sid, st string
		if err := rows.Scan(&sid, &st); err != nil {
			return "", nil, err
		}
		n, err := strconv.Atoi(strings.TrimPrefix(sid, gherkinStepIDPrefix))
		if err != nil {
			continue
		}
		if _, ok := byLine[n]; !ok {
			lines = append(lines, n)
		}
		byLine[n] = append(byLine[n], st)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	slices.Sort(lines)
	out := make([]GherkinStepResult, 0, len(lines))
	for _, l := range lines {
		out = append(out, GherkinStepResult{Line: l, Status: DeriveResult(byLine[l]), Results: len(byLine[l])})
	}
	return execID, out, nil
}
