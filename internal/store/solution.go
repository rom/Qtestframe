package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rom/qtestframe/internal/model"
)

// SolutionPreamble returns a project's solution preamble (empty when none
// was saved).
func (s *Store) SolutionPreamble(ctx context.Context, tenantID, projectID string) (*model.SolutionPreamble, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT solution_preamble FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	var p model.SolutionPreamble
	_ = json.Unmarshal([]byte(raw), &p)
	return &p, nil
}

// SaveSolutionPreamble stores a project's solution preamble with the next
// version, the time and who saved it, and records the revision for the
// history. p gets the stored bookkeeping.
func (s *Store) SaveSolutionPreamble(ctx context.Context, tenantID, projectID string, p *model.SolutionPreamble, user string) error {
	prev, err := s.SolutionPreamble(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	p.Normalize()
	now := s.now()
	p.Version, p.UpdatedAt, p.UpdatedBy = prev.Version+1, &now, user
	b, _ := json.Marshal(p)
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET solution_preamble = ? WHERE id = ? AND tenant_id = ?`, string(b), projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project: %w", ErrNotFound)
	}
	action := "update"
	if prev.Version == 0 {
		action = "create"
	}
	return s.AddRevision(ctx, tenantID, model.KindSolutionPreamble, projectID, p.Version, action, user, SnapshotSolutionPreamble(p))
}

// SnapshotSolutionPreamble returns the JSON snapshot stored for a preamble
// (without the bookkeeping that is not content).
func SnapshotSolutionPreamble(p *model.SolutionPreamble) json.RawMessage {
	c := *p
	c.UpdatedAt, c.UpdatedBy = nil, ""
	b, _ := json.Marshal(c)
	return b
}
