package store

import (
	"context"
	"fmt"

	"github.com/rom/qtestframe/internal/model"
)

const depSelect = `SELECT d.id, d.project_id, d.test_id, d.depends_on_id, d.created_by, d.created_at,
	t.key, t.title, o.key, o.title
	FROM test_dependencies d JOIN test_cases t ON t.id = d.test_id JOIN test_cases o ON o.id = d.depends_on_id`

// ListTestDependencies returns the test dependencies of a project, or those
// involving testID (both directions, with Direction set) when it is given.
func (s *Store) ListTestDependencies(ctx context.Context, tenantID, projectID, testID string) ([]model.TestDependency, error) {
	q := depSelect + ` WHERE d.tenant_id = ?`
	args := []any{tenantID}
	if projectID != "" {
		q += ` AND d.project_id = ?`
		args = append(args, projectID)
	}
	if testID != "" {
		q += ` AND (d.test_id = ? OR d.depends_on_id = ?)`
		args = append(args, testID, testID)
	}
	rows, err := s.c().query(ctx, q+` ORDER BY t.seq, o.seq`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TestDependency{}
	for rows.Next() {
		var d model.TestDependency
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.TestID, &d.DependsOnID, &d.CreatedBy, &d.CreatedAt,
			&d.TestKey, &d.TestTitle, &d.DependsOnKey, &d.DependsOnTitle); err != nil {
			return nil, err
		}
		if testID != "" {
			d.Direction = "outgoing"
			if d.DependsOnID == testID {
				d.Direction = "incoming"
			}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddTestDependency records that d.TestID depends on d.DependsOnID. Both
// tests must be in the same project; self-dependencies and cycles are
// rejected, duplicates are a conflict.
func (s *Store) AddTestDependency(ctx context.Context, tenantID string, d *model.TestDependency) error {
	if d.TestID == d.DependsOnID {
		return Invalidf("a test cannot depend on itself")
	}
	a, err := s.GetTest(ctx, tenantID, d.TestID)
	if err != nil {
		return err
	}
	b, err := s.GetTest(ctx, tenantID, d.DependsOnID)
	if err != nil {
		return err
	}
	if a.ProjectID != b.ProjectID {
		return Invalidf("both tests must be in the same project")
	}
	existing, err := s.ListTestDependencies(ctx, tenantID, a.ProjectID, "")
	if err != nil {
		return err
	}
	next := map[string][]string{}
	for _, e := range existing {
		next[e.TestID] = append(next[e.TestID], e.DependsOnID)
	}
	// adding test → dependsOn closes a cycle if dependsOn already (indirectly) depends on test
	seen := map[string]bool{}
	stack := []string{d.DependsOnID}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == d.TestID {
			return Invalidf("%s already depends on %s (directly or indirectly); the dependency would form a cycle", b.Key, a.Key)
		}
		if !seen[cur] {
			seen[cur] = true
			stack = append(stack, next[cur]...)
		}
	}
	d.ID, d.ProjectID, d.CreatedAt = newID(), a.ProjectID, s.now()
	d.TestKey, d.TestTitle, d.DependsOnKey, d.DependsOnTitle = a.Key, a.Title, b.Key, b.Title
	_, err = s.c().exec(ctx, `INSERT INTO test_dependencies (id, tenant_id, project_id, test_id, depends_on_id, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, tenantID, d.ProjectID, d.TestID, d.DependsOnID, d.CreatedBy, d.CreatedAt)
	return mapErr(err, "dependency")
}

// GetTestDependency returns one dependency.
func (s *Store) GetTestDependency(ctx context.Context, tenantID, id string) (*model.TestDependency, error) {
	var d model.TestDependency
	err := s.c().row(ctx, depSelect+` WHERE d.tenant_id = ? AND d.id = ?`, tenantID, id).Scan(&d.ID, &d.ProjectID, &d.TestID, &d.DependsOnID,
		&d.CreatedBy, &d.CreatedAt, &d.TestKey, &d.TestTitle, &d.DependsOnKey, &d.DependsOnTitle)
	if err != nil {
		return nil, mapErr(err, "dependency")
	}
	return &d, nil
}

// DeleteTestDependency removes a dependency.
func (s *Store) DeleteTestDependency(ctx context.Context, tenantID, id string) error {
	res, err := s.c().exec(ctx, `DELETE FROM test_dependencies WHERE tenant_id = ? AND id = ?`, tenantID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("dependency: %w", ErrNotFound)
	}
	return nil
}
