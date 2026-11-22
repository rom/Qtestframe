package store

import (
	"context"
	"database/sql"
	"slices"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// System elements: a project's product breakdown (systems, subsystems,
// components, products, suppliers) and the allocation of requirements and
// tests to them.

const elementCols = `id, project_id, parent_id, name, kind, description, position, created_by, created_at, updated_at`

func scanElement(r interface{ Scan(...any) error }) (*model.SystemElement, error) {
	var e model.SystemElement
	var parent sql.NullString
	if err := r.Scan(&e.ID, &e.ProjectID, &parent, &e.Name, &e.Kind, &e.Description, &e.Position, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.ParentID = strPtr(parent)
	e.RequirementIDs, e.TestIDs = []string{}, []string{}
	return &e, nil
}

// ElementPaths returns element id → "Root / Child / Element".
func ElementPaths(list []model.SystemElement) map[string]string {
	byID := map[string]*model.SystemElement{}
	for i := range list {
		byID[list[i].ID] = &list[i]
	}
	out := map[string]string{}
	var path func(id string, depth int) string
	path = func(id string, depth int) string {
		if p, ok := out[id]; ok {
			return p
		}
		e := byID[id]
		if e == nil || depth > 64 {
			return ""
		}
		p := e.Name
		if e.ParentID != nil && byID[*e.ParentID] != nil {
			p = path(*e.ParentID, depth+1) + " / " + e.Name
		}
		out[id] = p
		return p
	}
	for _, e := range list {
		path(e.ID, 0)
	}
	return out
}

// ElementAncestors returns element id → ids of the element and all
// elements above it.
func ElementAncestors(list []model.SystemElement) map[string][]string {
	parent := map[string]string{}
	for _, e := range list {
		if e.ParentID != nil {
			parent[e.ID] = *e.ParentID
		}
	}
	out := map[string][]string{}
	for _, e := range list {
		ids := []string{e.ID}
		for cur, n := e.ID, 0; n < 64; n++ {
			p, ok := parent[cur]
			if !ok || slices.Contains(ids, p) {
				break
			}
			ids = append(ids, p)
			cur = p
		}
		out[e.ID] = ids
	}
	return out
}

// ListElements returns the elements of a project in tree order (parents
// first, then by position and name) with their paths and allocations.
func (s *Store) ListElements(ctx context.Context, tenantID, projectID string) ([]model.SystemElement, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+elementCols+` FROM system_elements WHERE project_id = ? AND tenant_id = ? ORDER BY position, name`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	flat := []model.SystemElement{}
	for rows.Next() {
		e, err := scanElement(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		flat = append(flat, *e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reqs, tests, err := s.ElementAllocations(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	byElemR, byElemT := invert(reqs), invert(tests)
	paths := ElementPaths(flat)
	// tree order
	kids := map[string][]int{}
	ids := map[string]bool{}
	for _, e := range flat {
		ids[e.ID] = true
	}
	for i, e := range flat {
		p := ""
		if e.ParentID != nil && ids[*e.ParentID] {
			p = *e.ParentID
		}
		kids[p] = append(kids[p], i)
	}
	out := make([]model.SystemElement, 0, len(flat))
	done := map[string]bool{}
	var walk func(parent string)
	walk = func(parent string) {
		for _, i := range kids[parent] {
			e := flat[i]
			if done[e.ID] {
				continue
			}
			done[e.ID] = true
			e.Path = paths[e.ID]
			if v := byElemR[e.ID]; v != nil {
				e.RequirementIDs = v
			}
			if v := byElemT[e.ID]; v != nil {
				e.TestIDs = v
			}
			out = append(out, e)
			walk(e.ID)
		}
	}
	walk("")
	return out, nil
}

func invert(m map[string][]string) map[string][]string {
	out := map[string][]string{}
	for item, elems := range m {
		for _, e := range elems {
			out[e] = append(out[e], item)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// ElementAllocations returns requirement id → element ids and test id →
// element ids of a project.
func (s *Store) ElementAllocations(ctx context.Context, tenantID, projectID string) (map[string][]string, map[string][]string, error) {
	load := func(q string) (map[string][]string, error) {
		rows, err := s.c(ctx).query(ctx, q, projectID, tenantID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[string][]string{}
		for rows.Next() {
			var item, elem string
			if err := rows.Scan(&item, &elem); err != nil {
				return nil, err
			}
			out[item] = append(out[item], elem)
		}
		return out, rows.Err()
	}
	reqs, err := load(`SELECT a.requirement_id, a.element_id FROM requirement_elements a JOIN system_elements e ON e.id = a.element_id
		WHERE e.project_id = ? AND e.tenant_id = ? ORDER BY e.position, e.name`)
	if err != nil {
		return nil, nil, err
	}
	tests, err := load(`SELECT a.test_case_id, a.element_id FROM test_elements a JOIN system_elements e ON e.id = a.element_id
		WHERE e.project_id = ? AND e.tenant_id = ? ORDER BY e.position, e.name`)
	return reqs, tests, err
}

// GetElement returns an element with its direct allocations.
func (s *Store) GetElement(ctx context.Context, tenantID, id string) (*model.SystemElement, error) {
	e, err := scanElement(s.c(ctx).row(ctx, `SELECT `+elementCols+` FROM system_elements WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "system element")
	}
	list, err := s.ListElements(ctx, tenantID, e.ProjectID)
	if err != nil {
		return nil, err
	}
	for _, x := range list {
		if x.ID == id {
			return &x, nil
		}
	}
	return e, nil
}

func (s *Store) validateElement(ctx context.Context, c conn, tenantID string, e *model.SystemElement) error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len([]rune(e.Name)) > 200 {
		return Invalidf("element name is required (at most 200 characters)")
	}
	if strings.Contains(e.Name, "/") {
		return Invalidf("element name may not contain /")
	}
	if e.Kind == "" {
		e.Kind = "component"
	}
	if !model.Valid(model.ElementKinds, e.Kind) {
		return Invalidf("kind must be one of %s", strings.Join(model.ElementKinds, ", "))
	}
	if len(e.Description) > 10000 {
		return Invalidf("description may have at most 10000 characters")
	}
	if e.ParentID != nil && *e.ParentID == "" {
		e.ParentID = nil
	}
	if e.ParentID != nil {
		// the parent: same project, and not the element or one below it
		cur := *e.ParentID
		for n := 0; cur != ""; n++ {
			if cur == e.ID || n > 64 {
				return Invalidf("an element cannot be placed below itself")
			}
			var pid string
			var next sql.NullString
			if err := c.row(ctx, `SELECT project_id, parent_id FROM system_elements WHERE id = ? AND tenant_id = ?`, cur, tenantID).Scan(&pid, &next); err != nil {
				return mapErr(err, "parent element")
			}
			if pid != e.ProjectID {
				return Invalidf("the parent element belongs to another project")
			}
			cur = next.String
		}
	}
	// names are unique among siblings
	var n int
	q := `SELECT COUNT(*) FROM system_elements WHERE project_id = ? AND tenant_id = ? AND LOWER(name) = LOWER(?) AND id <> ? AND `
	args := []any{e.ProjectID, tenantID, e.Name, e.ID}
	if e.ParentID == nil {
		q += `parent_id IS NULL`
	} else {
		q += `parent_id = ?`
		args = append(args, *e.ParentID)
	}
	if err := c.row(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("an element named %q exists already at this place", e.Name)
	}
	return nil
}

// CreateElement adds a system element (at the end of its siblings).
func (s *Store) CreateElement(ctx context.Context, tenantID string, e *model.SystemElement) error {
	if _, err := s.GetProject(ctx, tenantID, e.ProjectID); err != nil {
		return err
	}
	e.ID = newID()
	e.CreatedAt = s.now()
	e.UpdatedAt = e.CreatedAt
	return s.tx(ctx, func(c conn) error {
		if err := s.validateElement(ctx, c, tenantID, e); err != nil {
			return err
		}
		if err := c.row(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM system_elements WHERE project_id = ?`, e.ProjectID).Scan(&e.Position); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO system_elements (id, tenant_id, project_id, parent_id, name, kind, description, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, tenantID, e.ProjectID, nullStr(e.ParentID), e.Name, e.Kind, e.Description, e.Position, e.CreatedBy, e.CreatedAt, e.UpdatedAt)
		return mapErr(err, "system element")
	})
}

// UpdateElement changes name, kind, description, parent and position.
func (s *Store) UpdateElement(ctx context.Context, tenantID string, e *model.SystemElement) error {
	old, err := s.GetElement(ctx, tenantID, e.ID)
	if err != nil {
		return err
	}
	e.ProjectID = old.ProjectID
	e.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if err := s.validateElement(ctx, c, tenantID, e); err != nil {
			return err
		}
		_, err := c.exec(ctx, `UPDATE system_elements SET parent_id = ?, name = ?, kind = ?, description = ?, position = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			nullStr(e.ParentID), e.Name, e.Kind, e.Description, e.Position, e.UpdatedAt, e.ID, tenantID)
		return mapErr(err, "system element")
	})
}

// DeleteElement removes an element, the elements below it and their
// allocations.
func (s *Store) DeleteElement(ctx context.Context, tenantID, id string) error {
	e, err := s.GetElement(ctx, tenantID, id)
	if err != nil {
		return err
	}
	list, err := s.ListElements(ctx, tenantID, e.ProjectID)
	if err != nil {
		return err
	}
	anc := ElementAncestors(list)
	var ids []string
	for _, x := range list {
		if slices.Contains(anc[x.ID], id) {
			ids = append(ids, x.ID)
		}
	}
	return s.tx(ctx, func(c conn) error {
		// children first (the foreign key cascades too; explicit for clarity)
		for i := len(ids) - 1; i >= 0; i-- {
			if _, err := c.exec(ctx, `DELETE FROM system_elements WHERE id = ? AND tenant_id = ?`, ids[i], tenantID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Allocate adds and removes allocations of requirements or tests (kind)
// of the element's project to an element.
func (s *Store) Allocate(ctx context.Context, tenantID, elementID, kind string, add, remove []string) error {
	e, err := s.GetElement(ctx, tenantID, elementID)
	if err != nil {
		return err
	}
	table, col, items := "requirement_elements", "requirement_id", "requirements"
	switch kind {
	case model.KindRequirement:
	case model.KindTest:
		table, col, items = "test_elements", "test_case_id", "test_cases"
	default:
		return Invalidf("kind must be requirement or test")
	}
	return s.tx(ctx, func(c conn) error {
		for _, id := range add {
			var pid string
			if err := c.row(ctx, `SELECT project_id FROM `+items+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
				return mapErr(err, kind)
			}
			if pid != e.ProjectID {
				return Invalidf("%s belongs to another project", kind)
			}
			if _, err := c.exec(ctx, `INSERT INTO `+table+` (`+col+`, element_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, elementID); err != nil {
				return err
			}
		}
		for _, id := range remove {
			if _, err := c.exec(ctx, `DELETE FROM `+table+` WHERE `+col+` = ? AND element_id = ?`, id, elementID); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetItemElements replaces the elements a requirement or test is
// allocated to.
func (s *Store) SetItemElements(ctx context.Context, tenantID, kind, itemID string, elementIDs []string) error {
	table, col, items := "requirement_elements", "requirement_id", "requirements"
	if kind == model.KindTest {
		table, col, items = "test_elements", "test_case_id", "test_cases"
	}
	var pid string
	if err := s.c(ctx).row(ctx, `SELECT project_id FROM `+items+` WHERE id = ? AND tenant_id = ?`, itemID, tenantID).Scan(&pid); err != nil {
		return mapErr(err, kind)
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM `+table+` WHERE `+col+` = ?`, itemID); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, eid := range elementIDs {
			if eid == "" || seen[eid] {
				continue
			}
			seen[eid] = true
			var epid string
			if err := c.row(ctx, `SELECT project_id FROM system_elements WHERE id = ? AND tenant_id = ?`, eid, tenantID).Scan(&epid); err != nil {
				return mapErr(err, "system element")
			}
			if epid != pid {
				return Invalidf("the element belongs to another project")
			}
			if _, err := c.exec(ctx, `INSERT INTO `+table+` (`+col+`, element_id) VALUES (?, ?)`, itemID, eid); err != nil {
				return err
			}
		}
		return nil
	})
}

// ItemElements returns the element ids a requirement or test is
// allocated to.
func (s *Store) ItemElements(ctx context.Context, kind, itemID string) ([]string, error) {
	table, col := "requirement_elements", "requirement_id"
	if kind == model.KindTest {
		table, col = "test_elements", "test_case_id"
	}
	rows, err := s.c(ctx).query(ctx, `SELECT a.element_id FROM `+table+` a JOIN system_elements e ON e.id = a.element_id WHERE a.`+col+` = ? ORDER BY e.position, e.name`, itemID)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}
