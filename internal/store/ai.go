package store

import (
	"context"
	"fmt"
)

// ProjectAIDisabled reports whether a project opted out of the AI
// assistance (projects.ai_disabled, migration 0061).
func (s *Store) ProjectAIDisabled(ctx context.Context, tenantID, projectID string) (bool, error) {
	var off bool
	if err := s.c(ctx).row(ctx, `SELECT ai_disabled FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&off); err != nil {
		return false, mapErr(err, "project")
	}
	return off, nil
}

// SetProjectAIDisabled turns the AI assistance of a project off (true) or
// back on.
func (s *Store) SetProjectAIDisabled(ctx context.Context, tenantID, projectID string, disabled bool) error {
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET ai_disabled = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, disabled, s.now(), projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project: %w", ErrNotFound)
	}
	return nil
}
