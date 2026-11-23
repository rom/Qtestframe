package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// External references (migration 0035): the identifiers that items of a
// project have in another tool, written by the ReqIF import and the
// imports from Jira and Azure DevOps. The two-way sync (migration 0060)
// keeps with each reference what both sides looked like after the last
// sync.

// ExternalRef links an item of a project to its identifier elsewhere.
type ExternalRef struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	System     string    `json:"system"`      // "reqif", "jira", "azure-devops"
	EntityType string    `json:"entity_type"` // requirement | relation | comment
	EntityID   string    `json:"entity_id"`
	ExternalID string    `json:"external_id"`
	Source     string    `json:"source"`
	Version    int       `json:"version"`
	ImportedAt time.Time `json:"imported_at"`
	// Two-way sync: the tracker's version of the item, the hashes of the
	// mapped fields on each side, their values (Sync) and when the item was
	// last synced; Origin "qtestframe" for an item the sync created.
	RemoteVersion string     `json:"remote_version,omitempty"`
	LocalHash     string     `json:"local_hash,omitempty"`
	RemoteHash    string     `json:"remote_hash,omitempty"`
	Sync          *SyncState `json:"sync_state,omitempty"`
	Origin        string     `json:"origin,omitempty"`
	SyncedAt      *time.Time `json:"synced_at,omitempty"`
}

// SyncState holds the mapped values of both sides after the last sync
// (field → value): the base of the three-way comparison.
type SyncState struct {
	Local  map[string]string `json:"l"`
	Remote map[string]string `json:"r"`
}

const refCols = `id, project_id, system, entity_type, entity_id, external_id, source, version, imported_at,
	remote_version, local_hash, remote_hash, sync_state, origin, synced_at`

func scanRef(r interface{ Scan(...any) error }) (*ExternalRef, error) {
	var x ExternalRef
	var state string
	var synced sql.NullTime
	if err := r.Scan(&x.ID, &x.ProjectID, &x.System, &x.EntityType, &x.EntityID, &x.ExternalID, &x.Source, &x.Version, &x.ImportedAt,
		&x.RemoteVersion, &x.LocalHash, &x.RemoteHash, &state, &x.Origin, &synced); err != nil {
		return nil, err
	}
	if state != "" {
		var st SyncState
		if json.Unmarshal([]byte(state), &st) == nil {
			x.Sync = &st
		}
	}
	x.SyncedAt = timePtr(synced)
	return &x, nil
}

func syncStateText(st *SyncState) string {
	if st == nil {
		return ""
	}
	b, _ := json.Marshal(st)
	return string(b)
}

// ListExternalRefs returns the references of a project in system.
func (s *Store) ListExternalRefs(ctx context.Context, tenantID, projectID, system string) ([]ExternalRef, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+refCols+`
		FROM external_refs WHERE tenant_id = ? AND project_id = ? AND system = ? ORDER BY imported_at, external_id`, tenantID, projectID, system)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExternalRef{}
	for rows.Next() {
		r, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// SaveExternalRef records (or moves) the reference of an external id. The
// sync state of a reference to the same item is kept (an import between
// two syncs).
func (s *Store) SaveExternalRef(ctx context.Context, tenantID string, r *ExternalRef) error {
	if r.ImportedAt.IsZero() {
		r.ImportedAt = s.now()
	}
	return s.tx(ctx, func(c conn) error {
		old, err := scanRef(c.row(ctx, `SELECT `+refCols+` FROM external_refs WHERE project_id = ? AND system = ? AND entity_type = ? AND external_id = ?`,
			r.ProjectID, r.System, r.EntityType, r.ExternalID))
		if err == nil && old.EntityID == r.EntityID && r.Sync == nil {
			r.RemoteVersion, r.LocalHash, r.RemoteHash, r.Sync, r.SyncedAt = old.RemoteVersion, old.LocalHash, old.RemoteHash, old.Sync, old.SyncedAt
			if r.Origin == "" {
				r.Origin = old.Origin
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM external_refs WHERE project_id = ? AND system = ? AND entity_type = ? AND external_id = ?`,
			r.ProjectID, r.System, r.EntityType, r.ExternalID); err != nil {
			return err
		}
		r.ID = newID()
		_, err = c.exec(ctx, `INSERT INTO external_refs (id, tenant_id, project_id, system, entity_type, entity_id, external_id, source, version, imported_at,
			remote_version, local_hash, remote_hash, sync_state, origin, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, tenantID, r.ProjectID, r.System, r.EntityType, r.EntityID, r.ExternalID, r.Source, r.Version, r.ImportedAt,
			r.RemoteVersion, r.LocalHash, r.RemoteHash, syncStateText(r.Sync), r.Origin, nullTime(r.SyncedAt))
		return err
	})
}

// UpdateSyncRef records the state of a reference after a sync.
func (s *Store) UpdateSyncRef(ctx context.Context, tenantID string, r *ExternalRef) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE external_refs SET version = ?, remote_version = ?, local_hash = ?, remote_hash = ?, sync_state = ?, synced_at = ?
		WHERE id = ? AND tenant_id = ?`, r.Version, r.RemoteVersion, r.LocalHash, r.RemoteHash, syncStateText(r.Sync), nullTime(r.SyncedAt), r.ID, tenantID)
	return err
}

// DeleteExternalRef removes a reference.
func (s *Store) DeleteExternalRef(ctx context.Context, tenantID, id string) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM external_refs WHERE id = ? AND tenant_id = ?`, id, tenantID)
	return err
}
