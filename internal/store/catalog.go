package store

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
)

// CatalogItem is a requirement or test of a project that was copied from
// the catalogue.
type CatalogItem struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	Ref   string `json:"-"`
}

// CatalogItems maps the catalogue ids used by a project's requirements
// (kind "requirement") or tests to those items, in key order.
func (s *Store) CatalogItems(ctx context.Context, tenantID, projectID, kind string) (map[string][]CatalogItem, error) {
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id, key, title, catalog_ref FROM `+table+` WHERE tenant_id = ? AND project_id = ? AND catalog_ref <> '' ORDER BY seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]CatalogItem{}
	for rows.Next() {
		var it CatalogItem
		if err := rows.Scan(&it.ID, &it.Key, &it.Title, &it.Ref); err != nil {
			return nil, err
		}
		out[it.Ref] = append(out[it.Ref], it)
	}
	return out, rows.Err()
}
