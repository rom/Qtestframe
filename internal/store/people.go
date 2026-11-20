package store

import "context"

// ProjectTags returns the tags used by the requirements and tests of a
// project, with the number of items carrying each one.
func (s *Store) ProjectTags(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c().query(ctx, `SELECT tags FROM requirements WHERE tenant_id = ? AND project_id = ?
		UNION ALL SELECT tags FROM test_cases WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, t := range decodeTags(raw) {
			out[t]++
		}
	}
	return out, rows.Err()
}

// ProjectPeopleNames returns the non-empty owner values of a project's
// requirements and the assignees of its executions, one entry per use.
func (s *Store) ProjectPeopleNames(ctx context.Context, tenantID, projectID string) ([]string, error) {
	rows, err := s.c().query(ctx, `SELECT owner FROM requirements WHERE tenant_id = ? AND project_id = ? AND owner <> ''
		UNION ALL SELECT e.assignee FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE e.tenant_id = ? AND r.project_id = ? AND e.assignee IS NOT NULL AND e.assignee <> ''`, tenantID, projectID, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
