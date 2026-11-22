package store

import (
	"context"
	"sort"
	"strings"
)

// ProjectTags returns the tags used by the requirements and tests of a
// project, with the number of items carrying each one.
func (s *Store) ProjectTags(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT tags FROM requirements WHERE tenant_id = ? AND project_id = ?
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
	rows, err := s.c(ctx).query(ctx, `SELECT owner FROM requirements WHERE tenant_id = ? AND project_id = ? AND owner <> ''
		UNION ALL SELECT owner FROM test_cases WHERE tenant_id = ? AND project_id = ? AND owner <> ''
		UNION ALL SELECT owner FROM risks WHERE tenant_id = ? AND project_id = ? AND owner <> ''
		UNION ALL SELECT assignee FROM defects WHERE tenant_id = ? AND project_id = ? AND assignee <> ''
		UNION ALL SELECT e.assignee FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE e.tenant_id = ? AND r.project_id = ? AND e.assignee IS NOT NULL AND e.assignee <> ''`,
		tenantID, projectID, tenantID, projectID, tenantID, projectID, tenantID, projectID, tenantID, projectID)
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

// TagCount is how many items of each kind carry a tag.
type TagCount struct {
	Tag          string `json:"tag"`
	Requirements int    `json:"requirements"`
	Tests        int    `json:"tests"`
	Documents    int    `json:"documents"`
	Total        int    `json:"total"`
}

// ProjectTagCounts counts the tags of a project's requirements, tests and
// files (documents, reports and attachments), most used first.
func (s *Store) ProjectTagCounts(ctx context.Context, tenantID, projectID string) ([]TagCount, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT 'r', tags FROM requirements WHERE tenant_id = ? AND project_id = ?
		UNION ALL SELECT 't', tags FROM test_cases WHERE tenant_id = ? AND project_id = ?
		UNION ALL SELECT 'd', tags FROM files WHERE tenant_id = ? AND project_id = ?`,
		tenantID, projectID, tenantID, projectID, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := map[string]*TagCount{}
	for rows.Next() {
		var kind, raw string
		if err := rows.Scan(&kind, &raw); err != nil {
			return nil, err
		}
		for _, t := range decodeTags(raw) {
			c := by[t]
			if c == nil {
				c = &TagCount{Tag: t}
				by[t] = c
			}
			switch kind {
			case "r":
				c.Requirements++
			case "t":
				c.Tests++
			default:
				c.Documents++
			}
			c.Total++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]TagCount, 0, len(by))
	for _, c := range by {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return strings.ToLower(out[i].Tag) < strings.ToLower(out[j].Tag)
	})
	return out, nil
}
