package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/model"
)

// Item templates: of a project (item_templates), of the organisation per
// project type (org_item_templates) and built in (customfield.Builtins);
// project_template_defaults names the template a project preselects.

// MaxTemplates bounds the templates of a project or project type per kind.
const MaxTemplates = 100

// ValidateTemplate normalises and checks a template.
func ValidateTemplate(t *model.ItemTemplate) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || utf8.RuneCountInString(t.Name) > 200 {
		return Invalidf("template name is required (at most 200 characters)")
	}
	t.Description = strings.TrimSpace(t.Description)
	if utf8.RuneCountInString(t.Description) > 2000 {
		return Invalidf("description may have at most 2000 characters")
	}
	if t.Kind != model.KindRequirement && t.Kind != model.KindTest {
		return Invalidf("kind must be requirement or test")
	}
	c := &t.Content
	c.Tags = cleanTags(c.Tags)
	if utf8.RuneCountInString(c.Title) > 500 {
		return Invalidf("title is too long")
	}
	check := func(name, v string, set []string) error {
		if v != "" && !slices.Contains(set, v) {
			return Invalidf("invalid %s %q", name, v)
		}
		return nil
	}
	if err := check("priority", c.Priority, model.Priorities); err != nil {
		return err
	}
	if t.Kind == model.KindRequirement {
		c.Preconditions, c.ExpectedResult, c.ExecutionType, c.EstimatedMin, c.Steps = "", "", "", 0, nil
		for _, e := range []error{check("obligation", c.Obligation, model.Obligations), check("type", c.Type, model.RequirementTypes),
			check("status", c.Status, model.RequirementStatuses)} {
			if e != nil {
				return e
			}
		}
		if len(c.Criteria) > 200 {
			return Invalidf("at most 200 criteria")
		}
		for i := range c.Criteria {
			cr := &c.Criteria[i]
			cr.ID, cr.RequirementID, cr.Position = "", "", i
			if cr.Method == "" {
				cr.Method = "test"
			}
			if !slices.Contains(model.EvaluationMethods, cr.Method) {
				return Invalidf("criterion %d: invalid method %q", i+1, cr.Method)
			}
		}
	} else {
		c.Obligation, c.Type, c.Source, c.Rationale, c.Criteria = "", "", "", "", nil
		for _, e := range []error{check("status", c.Status, model.TestStatuses),
			check("execution type", c.ExecutionType, []string{model.ExecManual, model.ExecAutomated})} {
			if e != nil {
				return e
			}
		}
		if c.EstimatedMin < 0 || c.EstimatedMin > 100000 {
			return Invalidf("estimated_minutes must be between 0 and 100000")
		}
		if len(c.Steps) > 500 {
			return Invalidf("at most 500 steps")
		}
		for i := range c.Steps {
			st := &c.Steps[i]
			st.ID, st.Position, st.CallTestID, st.CallTestKey, st.CallTestTitle = "", i, nil, "", ""
		}
	}
	for k, v := range c.CustomFields {
		if strings.TrimSpace(k) == "" || len(k) > 40 {
			return Invalidf("invalid custom field key %q", k)
		}
		switch v.(type) {
		case nil, string, float64, bool, []any, []string:
		default:
			return Invalidf("custom field %s: invalid value", k)
		}
		if v == nil {
			delete(c.CustomFields, k)
		}
	}
	if b, _ := json.Marshal(c); len(b) > 256<<10 {
		return Invalidf("the template content is too large (at most 256 KiB)")
	}
	return nil
}

const templateCols = `id, kind, name, description, content, position, created_by, created_at, updated_at`

func scanTemplate(r interface{ Scan(...any) error }, scope *string, t *model.ItemTemplate) error {
	var content string
	if err := r.Scan(&t.ID, scope, &t.Kind, &t.Name, &t.Description, &content, &t.Position, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return err
	}
	_ = json.Unmarshal([]byte(content), &t.Content)
	for k, v := range t.Content.CustomFields {
		t.Content.CustomFields[k] = normalizeDecoded(v)
	}
	return nil
}

func scanTemplates(rows *sql.Rows, org bool) ([]model.ItemTemplate, error) {
	defer rows.Close()
	out := []model.ItemTemplate{}
	for rows.Next() {
		var t model.ItemTemplate
		scope, src := &t.ProjectID, model.TemplateProject
		if org {
			scope, src = &t.ProjectType, model.TemplateOrganisation
		}
		if err := scanTemplate(rows, scope, &t); err != nil {
			return nil, err
		}
		t.Source = src
		out = append(out, t)
	}
	return out, rows.Err()
}

func tplTable(org bool) (table, scopeCol string) {
	if org {
		return "org_item_templates", "project_type"
	}
	return "item_templates", "project_id"
}

// ListTemplates returns the templates of a project (org false: scope is the
// project id) or of an organisation project type (org true: scope is the
// type), of one kind ("" for both).
func (s *Store) ListTemplates(ctx context.Context, tenantID string, org bool, scope, kind string) ([]model.ItemTemplate, error) {
	table, col := tplTable(org)
	q := `SELECT id, ` + col + `, kind, name, description, content, position, created_by, created_at, updated_at FROM ` + table + ` WHERE tenant_id = ?`
	args := []any{tenantID}
	if scope != "" {
		q += ` AND ` + col + ` = ?`
		args = append(args, scope)
	}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY kind, position, name`, args...)
	if err != nil {
		return nil, err
	}
	return scanTemplates(rows, org)
}

// GetTemplate returns a project (org false) or organisation template.
func (s *Store) GetTemplate(ctx context.Context, tenantID string, org bool, id string) (*model.ItemTemplate, error) {
	table, col := tplTable(org)
	rows, err := s.c(ctx).query(ctx, `SELECT id, `+col+`, kind, name, description, content, position, created_by, created_at, updated_at FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return nil, err
	}
	list, err := scanTemplates(rows, org)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// CreateTemplate adds a project template (ProjectID) or an organisation
// template (ProjectType).
func (s *Store) CreateTemplate(ctx context.Context, tenantID string, t *model.ItemTemplate) error {
	if err := ValidateTemplate(t); err != nil {
		return err
	}
	org := t.ProjectID == ""
	scope := t.ProjectID
	if org {
		if !slices.Contains(model.ProjectTypes, t.ProjectType) {
			return Invalidf("project_type must be one of %s", strings.Join(model.ProjectTypes, ", "))
		}
		scope = t.ProjectType
	} else if _, err := s.GetProject(ctx, tenantID, t.ProjectID); err != nil {
		return err
	}
	table, col := tplTable(org)
	return s.tx(ctx, func(c conn) error {
		var n int
		var max sql.NullInt64
		if err := c.row(ctx, `SELECT COUNT(*), MAX(position) FROM `+table+` WHERE tenant_id = ? AND `+col+` = ? AND kind = ?`, tenantID, scope, t.Kind).Scan(&n, &max); err != nil {
			return err
		}
		if n >= MaxTemplates {
			return Invalidf("at most %d templates per kind", MaxTemplates)
		}
		if t.Position <= 0 && max.Valid {
			t.Position = int(max.Int64) + 1
		}
		t.ID, t.CreatedAt = newID(), s.now()
		t.UpdatedAt = t.CreatedAt
		t.Source = model.TemplateProject
		if org {
			t.Source = model.TemplateOrganisation
		}
		content, _ := json.Marshal(t.Content)
		_, err := c.exec(ctx, `INSERT INTO `+table+` (id, tenant_id, `+col+`, kind, name, description, content, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, tenantID, scope, t.Kind, t.Name, t.Description, string(content), t.Position, t.CreatedBy, t.CreatedAt, t.UpdatedAt)
		return mapErr(err, "template name")
	})
}

// UpdateTemplate stores a template's name, description, content and
// position (its scope and kind never change).
func (s *Store) UpdateTemplate(ctx context.Context, tenantID string, org bool, t *model.ItemTemplate) error {
	old, err := s.GetTemplate(ctx, tenantID, org, t.ID)
	if err != nil {
		return err
	}
	t.ProjectID, t.ProjectType, t.Kind, t.Source, t.CreatedAt, t.CreatedBy = old.ProjectID, old.ProjectType, old.Kind, old.Source, old.CreatedAt, old.CreatedBy
	if err := ValidateTemplate(t); err != nil {
		return err
	}
	t.UpdatedAt = s.now()
	table, _ := tplTable(org)
	content, _ := json.Marshal(t.Content)
	_, err = s.c(ctx).exec(ctx, `UPDATE `+table+` SET name = ?, description = ?, content = ?, position = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		t.Name, t.Description, string(content), t.Position, t.UpdatedAt, t.ID, tenantID)
	return mapErr(err, "template name")
}

// DeleteTemplate deletes a template; a project default that names it is
// cleared.
func (s *Store) DeleteTemplate(ctx context.Context, tenantID string, org bool, id string) error {
	table, _ := tplTable(org)
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = c.exec(ctx, `DELETE FROM project_template_defaults WHERE template_id = ? AND tenant_id = ?`, id, tenantID)
		return err
	})
}

// TemplateDefaults returns a project's default template per kind.
func (s *Store) TemplateDefaults(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT kind, template_id FROM project_template_defaults WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, id string
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = id
	}
	return out, rows.Err()
}

// SetTemplateDefault sets (templateID "" clears) the default template of a
// project for a kind. The template must be one the project can use: its
// own, one of the organisation for the project's type, or a built-in one
// of that type.
func (s *Store) SetTemplateDefault(ctx context.Context, tenantID, projectID, kind, templateID string) error {
	if kind != model.KindRequirement && kind != model.KindTest {
		return Invalidf("kind must be requirement or test")
	}
	pr, err := s.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if templateID != "" {
		var ok bool
		if customfield.IsBuiltin(templateID) {
			b, found := customfield.Builtin(templateID, "en")
			ok = found && b.Kind == kind && b.ProjectType == pr.Type
		} else if t, err := s.GetTemplate(ctx, tenantID, false, templateID); err == nil {
			ok = t.ProjectID == projectID && t.Kind == kind
		} else if t, err := s.GetTemplate(ctx, tenantID, true, templateID); err == nil {
			ok = t.ProjectType == pr.Type && t.Kind == kind
		}
		if !ok {
			return Invalidf("template %q is not a %s template of this project", templateID, kind)
		}
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM project_template_defaults WHERE tenant_id = ? AND project_id = ? AND kind = ?`, tenantID, projectID, kind); err != nil {
			return err
		}
		if templateID == "" {
			return nil
		}
		_, err := c.exec(ctx, `INSERT INTO project_template_defaults (id, tenant_id, project_id, kind, template_id) VALUES (?, ?, ?, ?, ?)`, newID(), tenantID, projectID, kind, templateID)
		return err
	})
}
