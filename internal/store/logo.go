package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rom/qtestframe/internal/model"
)

// Logotypes of reports and generated documents: the organisation's in its
// settings (TenantSettings.Logo), a project's own in projects.logo. The
// images are files of kind logo (the organisation's without a project).

// ProjectLogo returns a project's own logo settings (defaults filled in;
// FileID "" when it uses the organisation's logo).
func (s *Store) ProjectLogo(ctx context.Context, tenantID, projectID string) (*model.LogoSettings, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT logo FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	var l model.LogoSettings
	_ = json.Unmarshal([]byte(raw), &l)
	l.Normalize()
	return &l, nil
}

// SaveProjectLogo stores a project's own logo settings.
func (s *Store) SaveProjectLogo(ctx context.Context, tenantID, projectID string, l *model.LogoSettings) error {
	l.Normalize()
	raw, _ := json.Marshal(l)
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET logo = ? WHERE id = ? AND tenant_id = ?`, string(raw), projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project: %w", ErrNotFound)
	}
	return nil
}

// SaveTenantLogo stores the organisation's logo settings (the other
// settings stay as they are).
func (s *Store) SaveTenantLogo(ctx context.Context, tenantID string, l *model.LogoSettings) error {
	st, err := s.TenantSettings(ctx, tenantID)
	if err != nil {
		return err
	}
	l.Normalize()
	l.Off = false
	st.Logo = *l
	return s.SaveTenantSettings(ctx, tenantID, st)
}

// LogoFile returns the content of a logo image of a tenant (nil when the
// file is gone or not a logo).
func (s *Store) LogoFile(ctx context.Context, tenantID, fileID string) ([]byte, error) {
	if fileID == "" {
		return nil, nil
	}
	f, err := s.GetFile(ctx, tenantID, fileID)
	if err != nil || f.Kind != model.FileLogo {
		return nil, nil
	}
	return s.FileData(ctx, fileID)
}
