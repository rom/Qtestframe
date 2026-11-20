package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

const reqCols = `id, project_id, folder_id, key, title, description, obligation, type, priority, status, source, rationale, owner, tags, version, position, created_by, created_at, updated_at`

func scanRequirement(r interface{ Scan(...any) error }) (*model.Requirement, error) {
	var q model.Requirement
	var folder sql.NullString
	var tags string
	if err := r.Scan(&q.ID, &q.ProjectID, &folder, &q.Key, &q.Title, &q.Description, &q.Obligation, &q.Type, &q.Priority, &q.Status,
		&q.Source, &q.Rationale, &q.Owner, &tags, &q.Version, &q.Position, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
		return nil, err
	}
	q.FolderID = strPtr(folder)
	q.Tags = decodeTags(tags)
	q.CategoryIDs = []string{}
	return &q, nil
}

// ValidateRequirement normalises and checks requirement fields.
func ValidateRequirement(r *model.Requirement) error {
	r.Tags = cleanTags(r.Tags)
	r.Title = strings.TrimSpace(r.Title)
	r.Owner = strings.TrimSpace(r.Owner)
	if r.Title == "" {
		return Invalidf("requirement title is required")
	}
	if len(r.Title) > 500 {
		return Invalidf("requirement title is too long")
	}
	if r.Type == "" {
		r.Type = "functional"
	}
	if r.Priority == "" {
		r.Priority = "medium"
	}
	if r.Status == "" {
		r.Status = "draft"
	}
	if r.Obligation != "" && !model.Valid(model.Obligations, r.Obligation) {
		return Invalidf("invalid obligation %q", r.Obligation)
	}
	if !model.Valid(model.RequirementTypes, r.Type) {
		return Invalidf("invalid requirement type %q", r.Type)
	}
	if !model.Valid(model.Priorities, r.Priority) {
		return Invalidf("invalid priority %q", r.Priority)
	}
	if !model.Valid(model.RequirementStatuses, r.Status) {
		return Invalidf("invalid status %q", r.Status)
	}
	for i := range r.Criteria {
		c := &r.Criteria[i]
		c.Description = strings.TrimSpace(c.Description)
		if c.Description == "" {
			return Invalidf("evaluation criterion %d: description is required", i+1)
		}
		if c.Method == "" {
			c.Method = "test"
		}
		if !model.Valid(model.EvaluationMethods, c.Method) {
			return Invalidf("evaluation criterion %d: invalid method %q", i+1, c.Method)
		}
	}
	return nil
}

func (s *Store) checkFolder(ctx context.Context, c conn, tenantID, projectID, kind string, folderID *string) error {
	if folderID == nil || *folderID == "" {
		return nil
	}
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM folders WHERE id = ? AND tenant_id = ? AND project_id = ? AND kind = ?`, *folderID, tenantID, projectID, kind).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return Invalidf("folder not found in project")
	}
	return nil
}

// CreateRequirement inserts a requirement with criteria and categories.
func (s *Store) CreateRequirement(ctx context.Context, tenantID string, r *model.Requirement) error {
	if err := ValidateRequirement(r); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, r.ProjectID); err != nil {
		return err
	}
	r.ID = newID()
	r.Version = 1
	r.CreatedAt = s.now()
	r.UpdatedAt = r.CreatedAt
	if r.FolderID != nil && *r.FolderID == "" {
		r.FolderID = nil
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.checkFolder(ctx, c, tenantID, r.ProjectID, model.KindRequirement, r.FolderID); err != nil {
			return err
		}
		seq, key, err := nextKey(ctx, c, r.ProjectID, model.KindRequirement)
		if err != nil {
			return err
		}
		r.Key = key
		r.Position = seq
		if _, err := c.exec(ctx, `INSERT INTO requirements (id, tenant_id, project_id, folder_id, seq, key, title, description, obligation, type, priority, status, source, rationale, owner, tags, version, position, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, tenantID, r.ProjectID, nullStr(r.FolderID), seq, r.Key, r.Title, r.Description, r.Obligation, r.Type, r.Priority, r.Status,
			r.Source, r.Rationale, r.Owner, encodeTags(r.Tags), r.Version, r.Position, r.CreatedBy, r.CreatedAt, r.UpdatedAt); err != nil {
			return mapErr(err, "requirement")
		}
		if err := setCategories(ctx, c, model.KindRequirement, r.ProjectID, r.ID, r.CategoryIDs); err != nil {
			return err
		}
		return replaceCriteria(ctx, c, r)
	})
}

func replaceCriteria(ctx context.Context, c conn, r *model.Requirement) error {
	// Keep IDs of criteria that are re-submitted so trace links survive.
	rows, err := c.query(ctx, `SELECT id FROM evaluation_criteria WHERE requirement_id = ?`, r.ID)
	if err != nil {
		return err
	}
	existing, err := scanStrings(rows)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for i := range r.Criteria {
		cr := &r.Criteria[i]
		cr.RequirementID = r.ID
		cr.Position = i
		isOld := false
		for _, e := range existing {
			if e == cr.ID {
				isOld = true
				break
			}
		}
		if isOld {
			keep[cr.ID] = true
			if _, err := c.exec(ctx, `UPDATE evaluation_criteria SET description = ?, method = ?, threshold = ?, position = ? WHERE id = ?`, cr.Description, cr.Method, cr.Threshold, cr.Position, cr.ID); err != nil {
				return err
			}
			continue
		}
		cr.ID = newID()
		if _, err := c.exec(ctx, `INSERT INTO evaluation_criteria (id, requirement_id, description, method, threshold, position) VALUES (?, ?, ?, ?, ?, ?)`, cr.ID, r.ID, cr.Description, cr.Method, cr.Threshold, cr.Position); err != nil {
			return err
		}
	}
	for _, e := range existing {
		if !keep[e] {
			if _, err := c.exec(ctx, `UPDATE trace_links SET criterion_id = NULL WHERE criterion_id = ?`, e); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `DELETE FROM evaluation_criteria WHERE id = ?`, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetRequirement returns a requirement with criteria and categories.
func (s *Store) GetRequirement(ctx context.Context, tenantID, id string) (*model.Requirement, error) {
	r, err := scanRequirement(s.c().row(ctx, `SELECT `+reqCols+` FROM requirements WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "requirement")
	}
	rows, err := s.c().query(ctx, `SELECT category_id FROM requirement_categories WHERE requirement_id = ? ORDER BY category_id`, id)
	if err != nil {
		return nil, err
	}
	if r.CategoryIDs, err = scanStrings(rows); err != nil {
		return nil, err
	}
	if r.CategoryIDs == nil {
		r.CategoryIDs = []string{}
	}
	crit, err := s.listCriteria(ctx, s.c(), `WHERE requirement_id = ?`, id)
	if err != nil {
		return nil, err
	}
	r.Criteria = crit
	return r, nil
}

func (s *Store) listCriteria(ctx context.Context, c conn, where string, args ...any) ([]model.EvaluationCriterion, error) {
	rows, err := c.query(ctx, `SELECT id, requirement_id, description, method, threshold, position FROM evaluation_criteria `+where+` ORDER BY position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.EvaluationCriterion{}
	for rows.Next() {
		var e model.EvaluationCriterion
		if err := rows.Scan(&e.ID, &e.RequirementID, &e.Description, &e.Method, &e.Threshold, &e.Position); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListRequirements returns all requirements of a project including
// category assignments and criteria.
func (s *Store) ListRequirements(ctx context.Context, tenantID, projectID string) ([]model.Requirement, error) {
	rows, err := s.c().query(ctx, `SELECT `+reqCols+` FROM requirements WHERE tenant_id = ? AND project_id = ? ORDER BY position, seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.Requirement{}
	for rows.Next() {
		r, err := scanRequirement(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cats, err := categoryAssignments(ctx, s.c(), model.KindRequirement, projectID)
	if err != nil {
		return nil, err
	}
	crit, err := s.listCriteria(ctx, s.c(), `WHERE requirement_id IN (SELECT id FROM requirements WHERE project_id = ?)`, projectID)
	if err != nil {
		return nil, err
	}
	byReq := map[string][]model.EvaluationCriterion{}
	for _, c := range crit {
		byReq[c.RequirementID] = append(byReq[c.RequirementID], c)
	}
	for i := range out {
		if c := cats[out[i].ID]; c != nil {
			out[i].CategoryIDs = c
		}
		out[i].Criteria = byReq[out[i].ID]
	}
	return out, nil
}

// UpdateRequirement stores all mutable fields; criteria are replaced
// when replaceCriteria is true. The version is incremented.
func (s *Store) UpdateRequirement(ctx context.Context, tenantID string, r *model.Requirement, withCriteria bool) error {
	if err := ValidateRequirement(r); err != nil {
		return err
	}
	old, err := s.GetRequirement(ctx, tenantID, r.ID)
	if err != nil {
		return err
	}
	r.ProjectID = old.ProjectID
	r.Key = old.Key
	r.CreatedAt, r.CreatedBy = old.CreatedAt, old.CreatedBy
	r.Version = old.Version + 1
	r.UpdatedAt = s.now()
	if r.FolderID != nil && *r.FolderID == "" {
		r.FolderID = nil
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.checkFolder(ctx, c, tenantID, r.ProjectID, model.KindRequirement, r.FolderID); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `UPDATE requirements SET folder_id = ?, title = ?, description = ?, obligation = ?, type = ?, priority = ?, status = ?, source = ?, rationale = ?, owner = ?, tags = ?, version = ?, updated_at = ? WHERE id = ?`,
			nullStr(r.FolderID), r.Title, r.Description, r.Obligation, r.Type, r.Priority, r.Status, r.Source, r.Rationale, r.Owner, encodeTags(r.Tags), r.Version, r.UpdatedAt, r.ID); err != nil {
			return err
		}
		if err := setCategories(ctx, c, model.KindRequirement, r.ProjectID, r.ID, r.CategoryIDs); err != nil {
			return err
		}
		if withCriteria {
			return replaceCriteria(ctx, c, r)
		}
		r.Criteria = old.Criteria
		return nil
	})
}

// DeleteRequirements deletes requirements of a project.
func (s *Store) DeleteRequirements(ctx context.Context, tenantID, projectID string, ids []string) (int, error) {
	return s.deleteItems(ctx, "requirements", tenantID, projectID, ids)
}

func (s *Store) deleteItems(ctx context.Context, table, tenantID, projectID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	where := ` WHERE tenant_id = ? AND id IN (` + placeholders(len(ids)) + `)`
	args := append([]any{tenantID}, anySlice(ids)...)
	if projectID != "" {
		where += ` AND project_id = ?`
		args = append(args, projectID)
	}
	kind := model.KindRequirement
	if table == "test_cases" {
		kind = model.KindTest
	}
	n := 0
	err := s.tx(ctx, func(c conn) error {
		rows, err := c.query(ctx, `SELECT id FROM `+table+where, args...)
		if err != nil {
			return err
		}
		found, err := scanStrings(rows)
		if err != nil {
			return err
		}
		if err := deleteEntityData(ctx, c, kind, found); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM `+table+where, args...)
		if err != nil {
			return err
		}
		m, _ := res.RowsAffected()
		n = int(m)
		return nil
	})
	return n, err
}

// MoveItems moves requirements or tests into a folder (or the root) and
// optionally another project, placing them at position in the target
// folder ordering.
func (s *Store) MoveItems(ctx context.Context, tenantID, kind, projectID string, ids []string, folderID *string, position int, targetProject string) error {
	if !validKind(kind) {
		return Invalidf("invalid kind")
	}
	if len(ids) == 0 {
		return Invalidf("no items given")
	}
	if targetProject == "" {
		targetProject = projectID
	}
	if _, err := s.GetProject(ctx, tenantID, targetProject); err != nil {
		return err
	}
	if folderID != nil && *folderID == "" {
		folderID = nil
	}
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	}
	return s.tx(ctx, func(c conn) error {
		var n int
		args := append([]any{tenantID, projectID}, anySlice(ids)...)
		if err := c.row(ctx, `SELECT COUNT(*) FROM `+table+` WHERE tenant_id = ? AND project_id = ? AND id IN (`+placeholders(len(ids))+`)`, args...).Scan(&n); err != nil {
			return err
		}
		if n != len(uniq(ids)) {
			return mapErr(sql.ErrNoRows, kind)
		}
		if err := s.checkFolder(ctx, c, tenantID, targetProject, kind, folderID); err != nil {
			return err
		}
		if targetProject != projectID {
			if err := s.moveItemsToProject(ctx, c, tenantID, kind, ids, projectID, targetProject); err != nil {
				return err
			}
		}
		if _, err := c.exec(ctx, `UPDATE `+table+` SET folder_id = ? WHERE id IN (`+placeholders(len(ids))+`)`, append([]any{nullStr(folderID)}, anySlice(ids)...)...); err != nil {
			return err
		}
		// re-number positions inside the target folder
		var rows *sql.Rows
		var err error
		if folderID == nil {
			rows, err = c.query(ctx, `SELECT id FROM `+table+` WHERE project_id = ? AND folder_id IS NULL ORDER BY position, seq`, targetProject)
		} else {
			rows, err = c.query(ctx, `SELECT id FROM `+table+` WHERE project_id = ? AND folder_id = ? ORDER BY position, seq`, targetProject, *folderID)
		}
		if err != nil {
			return err
		}
		all, err := scanStrings(rows)
		if err != nil {
			return err
		}
		moving := map[string]bool{}
		for _, id := range ids {
			moving[id] = true
		}
		rest := make([]string, 0, len(all))
		for _, id := range all {
			if !moving[id] {
				rest = append(rest, id)
			}
		}
		if position < 0 || position > len(rest) {
			position = len(rest)
		}
		order := append(append(append([]string{}, rest[:position]...), uniq(ids)...), rest[position:]...)
		for i, id := range order {
			if _, err := c.exec(ctx, `UPDATE `+table+` SET position = ? WHERE id = ?`, i, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func uniq(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// moveItemsToProject re-keys items, remaps categories by dimension/name and
// drops trace links (links never cross projects).
func (s *Store) moveItemsToProject(ctx context.Context, c conn, tenantID, kind string, ids []string, from, to string) error {
	if len(ids) == 0 {
		return nil
	}
	table, catTbl, col, linkCol := "requirements", "requirement_categories", "requirement_id", "requirement_id"
	if kind == model.KindTest {
		table, catTbl, col, linkCol = "test_cases", "test_case_categories", "test_case_id", "test_case_id"
	}
	// category mapping source id -> target id
	rows, err := c.query(ctx, `SELECT s.id, t.id FROM categories s JOIN categories t ON t.project_id = ? AND t.scope = s.scope AND t.dimension = s.dimension AND t.name = s.name WHERE s.project_id = ? AND s.scope = ?`, to, from, kind)
	if err != nil {
		return err
	}
	mapping := map[string]string{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			rows.Close()
			return err
		}
		mapping[a] = b
	}
	rows.Close()
	for _, id := range ids {
		seq, key, err := nextKey(ctx, c, to, kind)
		if err != nil {
			return err
		}
		if _, err := c.exec(ctx, `UPDATE `+table+` SET project_id = ?, seq = ?, key = ? WHERE id = ? AND tenant_id = ?`, to, seq, key, id, tenantID); err != nil {
			return err
		}
		crow, err := c.query(ctx, `SELECT category_id FROM `+catTbl+` WHERE `+col+` = ?`, id)
		if err != nil {
			return err
		}
		cats, err := scanStrings(crow)
		if err != nil {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM `+catTbl+` WHERE `+col+` = ?`, id); err != nil {
			return err
		}
		for _, cat := range cats {
			if t, ok := mapping[cat]; ok {
				if _, err := c.exec(ctx, `INSERT INTO `+catTbl+` (`+col+`, category_id) VALUES (?, ?)`, id, t); err != nil {
					return err
				}
			}
		}
		if _, err := c.exec(ctx, `DELETE FROM trace_links WHERE `+linkCol+` = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// BulkUpdate describes a bulk field change.
type BulkUpdate struct {
	Status            *string  `json:"status"`
	Priority          *string  `json:"priority"`
	Obligation        *string  `json:"obligation"`
	ExecutionType     *string  `json:"execution_type"`
	AddCategoryIDs    []string `json:"add_category_ids"`
	RemoveCategoryIDs []string `json:"remove_category_ids"`
}

// BulkUpdateItems applies field changes to many requirements or tests.
func (s *Store) BulkUpdateItems(ctx context.Context, tenantID, kind, projectID string, ids []string, u BulkUpdate) (int, error) {
	if !validKind(kind) {
		return 0, Invalidf("invalid kind")
	}
	table := "requirements"
	sets := []string{}
	args := []any{}
	check := func(set []string, v *string, col, what string) error {
		if v == nil {
			return nil
		}
		if !model.Valid(set, *v) {
			return Invalidf("invalid %s %q", what, *v)
		}
		sets = append(sets, col+" = ?")
		args = append(args, *v)
		return nil
	}
	var err error
	if kind == model.KindRequirement {
		if err = check(model.RequirementStatuses, u.Status, "status", "status"); err == nil {
			err = check(model.Obligations, u.Obligation, "obligation", "obligation")
		}
		if u.ExecutionType != nil {
			err = Invalidf("execution_type not applicable to requirements")
		}
	} else {
		table = "test_cases"
		if err = check(model.TestStatuses, u.Status, "status", "status"); err == nil {
			err = check([]string{model.ExecManual, model.ExecAutomated}, u.ExecutionType, "execution_type", "execution type")
		}
		if u.Obligation != nil {
			err = Invalidf("obligation not applicable to tests")
		}
	}
	if err == nil {
		err = check(model.Priorities, u.Priority, "priority", "priority")
	}
	if err != nil {
		return 0, err
	}
	ids = uniq(ids)
	n := 0
	err = s.tx(ctx, func(c conn) error {
		for _, id := range ids {
			var exists int
			if err := c.row(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ? AND tenant_id = ? AND project_id = ?`, id, tenantID, projectID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return mapErr(sql.ErrNoRows, kind+" "+id)
			}
			if len(sets) > 0 {
				q := `UPDATE ` + table + ` SET ` + strings.Join(sets, ", ") + `, updated_at = ?`
				q += `, version = version + 1`
				if _, err := c.exec(ctx, q+` WHERE id = ?`, append(append(append([]any{}, args...), s.now()), id)...); err != nil {
					return err
				}
			}
			if err := addCategories(ctx, c, kind, projectID, id, u.AddCategoryIDs); err != nil {
				return err
			}
			if err := removeCategories(ctx, c, kind, id, u.RemoveCategoryIDs); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
