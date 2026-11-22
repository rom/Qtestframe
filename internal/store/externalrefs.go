package store

import (
	"context"
	"time"
)

// External references (migration 0035): the identifiers that items of a
// project have in another tool, written by the ReqIF import.

// ExternalRef links an item of a project to its identifier elsewhere.
type ExternalRef struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	System     string    `json:"system"`      // "reqif"
	EntityType string    `json:"entity_type"` // requirement | relation
	EntityID   string    `json:"entity_id"`
	ExternalID string    `json:"external_id"`
	Source     string    `json:"source"`
	Version    int       `json:"version"`
	ImportedAt time.Time `json:"imported_at"`
}

// ListExternalRefs returns the references of a project in system.
func (s *Store) ListExternalRefs(ctx context.Context, tenantID, projectID, system string) ([]ExternalRef, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, project_id, system, entity_type, entity_id, external_id, source, version, imported_at
		FROM external_refs WHERE tenant_id = ? AND project_id = ? AND system = ? ORDER BY imported_at, external_id`, tenantID, projectID, system)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExternalRef{}
	for rows.Next() {
		var r ExternalRef
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.System, &r.EntityType, &r.EntityID, &r.ExternalID, &r.Source, &r.Version, &r.ImportedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveExternalRef records (or moves) the reference of an external id.
func (s *Store) SaveExternalRef(ctx context.Context, tenantID string, r *ExternalRef) error {
	if r.ImportedAt.IsZero() {
		r.ImportedAt = s.now()
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM external_refs WHERE project_id = ? AND system = ? AND entity_type = ? AND external_id = ?`,
			r.ProjectID, r.System, r.EntityType, r.ExternalID); err != nil {
			return err
		}
		r.ID = newID()
		_, err := c.exec(ctx, `INSERT INTO external_refs (id, tenant_id, project_id, system, entity_type, entity_id, external_id, source, version, imported_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, tenantID, r.ProjectID, r.System, r.EntityType, r.EntityID, r.ExternalID, r.Source, r.Version, r.ImportedAt)
		return err
	})
}

// DeleteExternalRef removes a reference.
func (s *Store) DeleteExternalRef(ctx context.Context, tenantID, id string) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM external_refs WHERE id = ? AND tenant_id = ?`, id, tenantID)
	return err
}
