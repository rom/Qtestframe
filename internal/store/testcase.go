package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

const testCols = `id, project_id, folder_id, key, title, description, preconditions, execution_type, script_language, script, expected_result, priority, status, estimated_min, tags, position, created_by, created_at, updated_at`

func scanTest(r interface{ Scan(...any) error }) (*model.TestCase, error) {
	var t model.TestCase
	var folder sql.NullString
	var tags string
	if err := r.Scan(&t.ID, &t.ProjectID, &folder, &t.Key, &t.Title, &t.Description, &t.Preconditions, &t.ExecutionType, &t.ScriptLanguage, &t.Script,
		&t.ExpectedResult, &t.Priority, &t.Status, &t.EstimatedMin, &tags, &t.Position, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.FolderID = strPtr(folder)
	t.Tags = decodeTags(tags)
	t.CategoryIDs = []string{}
	t.Steps = []model.TestStep{}
	return &t, nil
}

// ValidateTest normalises and checks test case fields.
func ValidateTest(t *model.TestCase) error {
	t.Tags = cleanTags(t.Tags)
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return Invalidf("test title is required")
	}
	if len(t.Title) > 500 {
		return Invalidf("test title is too long")
	}
	if t.ExecutionType == "" {
		t.ExecutionType = model.ExecManual
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.Status == "" {
		t.Status = "draft"
	}
	if t.ExecutionType != model.ExecManual && t.ExecutionType != model.ExecAutomated {
		return Invalidf("invalid execution type %q", t.ExecutionType)
	}
	if t.ScriptLanguage != "" && !model.Valid(model.ScriptLanguages, t.ScriptLanguage) {
		return Invalidf("invalid script language %q", t.ScriptLanguage)
	}
	if t.ExecutionType == model.ExecAutomated && t.ScriptLanguage == "" && t.Script != "" {
		t.ScriptLanguage = "sh"
	}
	if !model.Valid(model.Priorities, t.Priority) {
		return Invalidf("invalid priority %q", t.Priority)
	}
	if !model.Valid(model.TestStatuses, t.Status) {
		return Invalidf("invalid status %q", t.Status)
	}
	if t.EstimatedMin < 0 {
		return Invalidf("estimated minutes must not be negative")
	}
	if len(t.Script) > 1<<20 {
		return Invalidf("script is too large")
	}
	return nil
}

// CreateTest inserts a test case including steps and categories.
func (s *Store) CreateTest(ctx context.Context, tenantID string, t *model.TestCase) error {
	if err := ValidateTest(t); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, t.ProjectID); err != nil {
		return err
	}
	t.ID = newID()
	t.CreatedAt = s.now()
	t.UpdatedAt = t.CreatedAt
	if t.FolderID != nil && *t.FolderID == "" {
		t.FolderID = nil
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.checkFolder(ctx, c, tenantID, t.ProjectID, model.KindTest, t.FolderID); err != nil {
			return err
		}
		seq, key, err := nextKey(ctx, c, t.ProjectID, model.KindTest)
		if err != nil {
			return err
		}
		t.Key, t.Position = key, seq
		if _, err := c.exec(ctx, `INSERT INTO test_cases (id, tenant_id, project_id, folder_id, seq, key, title, description, preconditions, execution_type, script_language, script, expected_result, priority, status, estimated_min, tags, position, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, tenantID, t.ProjectID, nullStr(t.FolderID), seq, t.Key, t.Title, t.Description, t.Preconditions, t.ExecutionType, t.ScriptLanguage, t.Script,
			t.ExpectedResult, t.Priority, t.Status, t.EstimatedMin, encodeTags(t.Tags), t.Position, t.CreatedBy, t.CreatedAt, t.UpdatedAt); err != nil {
			return mapErr(err, "test")
		}
		if err := setCategories(ctx, c, model.KindTest, t.ProjectID, t.ID, t.CategoryIDs); err != nil {
			return err
		}
		return replaceSteps(ctx, c, t)
	})
}

func replaceSteps(ctx context.Context, c conn, t *model.TestCase) error {
	if _, err := c.exec(ctx, `DELETE FROM test_steps WHERE test_case_id = ?`, t.ID); err != nil {
		return err
	}
	if t.Steps == nil {
		t.Steps = []model.TestStep{}
	}
	for i := range t.Steps {
		st := &t.Steps[i]
		if st.ID == "" {
			st.ID = newID()
		}
		st.Position = i
		if _, err := c.exec(ctx, `INSERT INTO test_steps (id, test_case_id, position, action, expected, data) VALUES (?, ?, ?, ?, ?, ?)`, st.ID, t.ID, i, st.Action, st.Expected, st.Data); err != nil {
			return mapErr(err, "step")
		}
	}
	return nil
}

func (s *Store) listSteps(ctx context.Context, where string, args ...any) (map[string][]model.TestStep, error) {
	rows, err := s.c().query(ctx, `SELECT id, test_case_id, position, action, expected, data FROM test_steps `+where+` ORDER BY position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]model.TestStep{}
	for rows.Next() {
		var st model.TestStep
		var tc string
		if err := rows.Scan(&st.ID, &tc, &st.Position, &st.Action, &st.Expected, &st.Data); err != nil {
			return nil, err
		}
		out[tc] = append(out[tc], st)
	}
	return out, rows.Err()
}

// GetTest returns a test case with steps and categories.
func (s *Store) GetTest(ctx context.Context, tenantID, id string) (*model.TestCase, error) {
	t, err := scanTest(s.c().row(ctx, `SELECT `+testCols+` FROM test_cases WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "test")
	}
	rows, err := s.c().query(ctx, `SELECT category_id FROM test_case_categories WHERE test_case_id = ? ORDER BY category_id`, id)
	if err != nil {
		return nil, err
	}
	if t.CategoryIDs, err = scanStrings(rows); err != nil {
		return nil, err
	}
	if t.CategoryIDs == nil {
		t.CategoryIDs = []string{}
	}
	steps, err := s.listSteps(ctx, `WHERE test_case_id = ?`, id)
	if err != nil {
		return nil, err
	}
	if st := steps[id]; st != nil {
		t.Steps = st
	}
	return t, nil
}

// ListTests returns all tests of a project with categories and steps.
func (s *Store) ListTests(ctx context.Context, tenantID, projectID string) ([]model.TestCase, error) {
	rows, err := s.c().query(ctx, `SELECT `+testCols+` FROM test_cases WHERE tenant_id = ? AND project_id = ? ORDER BY position, seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.TestCase{}
	for rows.Next() {
		t, err := scanTest(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cats, err := categoryAssignments(ctx, s.c(), model.KindTest, projectID)
	if err != nil {
		return nil, err
	}
	steps, err := s.listSteps(ctx, `WHERE test_case_id IN (SELECT id FROM test_cases WHERE project_id = ?)`, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if c := cats[out[i].ID]; c != nil {
			out[i].CategoryIDs = c
		}
		if st := steps[out[i].ID]; st != nil {
			out[i].Steps = st
		}
	}
	return out, nil
}

// UpdateTest stores all mutable fields; steps replaced when withSteps.
func (s *Store) UpdateTest(ctx context.Context, tenantID string, t *model.TestCase, withSteps bool) error {
	if err := ValidateTest(t); err != nil {
		return err
	}
	old, err := s.GetTest(ctx, tenantID, t.ID)
	if err != nil {
		return err
	}
	t.ProjectID, t.Key = old.ProjectID, old.Key
	t.CreatedAt, t.CreatedBy = old.CreatedAt, old.CreatedBy
	t.UpdatedAt = s.now()
	if t.FolderID != nil && *t.FolderID == "" {
		t.FolderID = nil
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.checkFolder(ctx, c, tenantID, t.ProjectID, model.KindTest, t.FolderID); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `UPDATE test_cases SET folder_id = ?, title = ?, description = ?, preconditions = ?, execution_type = ?, script_language = ?, script = ?, expected_result = ?, priority = ?, status = ?, estimated_min = ?, tags = ?, updated_at = ? WHERE id = ?`,
			nullStr(t.FolderID), t.Title, t.Description, t.Preconditions, t.ExecutionType, t.ScriptLanguage, t.Script, t.ExpectedResult, t.Priority, t.Status, t.EstimatedMin, encodeTags(t.Tags), t.UpdatedAt, t.ID); err != nil {
			return err
		}
		if err := setCategories(ctx, c, model.KindTest, t.ProjectID, t.ID, t.CategoryIDs); err != nil {
			return err
		}
		if withSteps {
			return replaceSteps(ctx, c, t)
		}
		t.Steps = old.Steps
		return nil
	})
}

// CopyTest duplicates a test case (new key, steps get new ids).
func (s *Store) CopyTest(ctx context.Context, tenantID, id, userID string) (*model.TestCase, error) {
	t, err := s.GetTest(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	t.Title = "Copy of " + t.Title
	if len(t.Title) > 500 {
		t.Title = t.Title[:500]
	}
	t.CreatedBy = userID
	for i := range t.Steps {
		t.Steps[i].ID = ""
	}
	if err := s.CreateTest(ctx, tenantID, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTests deletes tests of a project.
func (s *Store) DeleteTests(ctx context.Context, tenantID, projectID string, ids []string) (int, error) {
	return s.deleteItems(ctx, "test_cases", tenantID, projectID, ids)
}

// ---------------------------------------------------------------- links

// CreateLink connects a requirement and a test of the same project.
func (s *Store) CreateLink(ctx context.Context, tenantID string, l *model.TraceLink) error {
	r, err := s.GetRequirement(ctx, tenantID, l.RequirementID)
	if err != nil {
		return err
	}
	t, err := s.GetTest(ctx, tenantID, l.TestCaseID)
	if err != nil {
		return err
	}
	if r.ProjectID != t.ProjectID {
		return Invalidf("requirement and test belong to different projects")
	}
	if l.CriterionID != nil && *l.CriterionID != "" {
		ok := false
		for _, c := range r.Criteria {
			if c.ID == *l.CriterionID {
				ok = true
			}
		}
		if !ok {
			return Invalidf("criterion does not belong to requirement")
		}
	} else {
		l.CriterionID = nil
	}
	l.ID = newID()
	l.ProjectID = r.ProjectID
	l.CreatedAt = s.now()
	l.RequirementKey, l.TestCaseKey = r.Key, t.Key
	_, err = s.c().exec(ctx, `INSERT INTO trace_links (id, tenant_id, project_id, requirement_id, test_case_id, criterion_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, tenantID, l.ProjectID, l.RequirementID, l.TestCaseID, nullStr(l.CriterionID), l.CreatedAt)
	return mapErr(err, "link")
}

// ListLinks returns links of a project, optionally restricted to one
// requirement or test.
func (s *Store) ListLinks(ctx context.Context, tenantID, projectID, requirementID, testID string) ([]model.TraceLink, error) {
	q := `SELECT l.id, l.project_id, l.requirement_id, l.test_case_id, l.criterion_id, l.created_at, r.key, t.key
		FROM trace_links l JOIN requirements r ON r.id = l.requirement_id JOIN test_cases t ON t.id = l.test_case_id
		WHERE l.tenant_id = ?`
	args := []any{tenantID}
	if projectID != "" {
		q += ` AND l.project_id = ?`
		args = append(args, projectID)
	}
	if requirementID != "" {
		q += ` AND l.requirement_id = ?`
		args = append(args, requirementID)
	}
	if testID != "" {
		q += ` AND l.test_case_id = ?`
		args = append(args, testID)
	}
	rows, err := s.c().query(ctx, q+` ORDER BY r.seq, t.seq`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TraceLink{}
	for rows.Next() {
		var l model.TraceLink
		var crit sql.NullString
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.RequirementID, &l.TestCaseID, &crit, &l.CreatedAt, &l.RequirementKey, &l.TestCaseKey); err != nil {
			return nil, err
		}
		l.CriterionID = strPtr(crit)
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetLink returns one link.
func (s *Store) GetLink(ctx context.Context, tenantID, id string) (*model.TraceLink, error) {
	var l model.TraceLink
	var crit sql.NullString
	err := s.c().row(ctx, `SELECT id, project_id, requirement_id, test_case_id, criterion_id, created_at FROM trace_links WHERE id = ? AND tenant_id = ?`, id, tenantID).
		Scan(&l.ID, &l.ProjectID, &l.RequirementID, &l.TestCaseID, &crit, &l.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "link")
	}
	l.CriterionID = strPtr(crit)
	return &l, nil
}

// DeleteLink removes a link.
func (s *Store) DeleteLink(ctx context.Context, tenantID, id string) error {
	res, err := s.c().exec(ctx, `DELETE FROM trace_links WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "link")
	}
	return nil
}
