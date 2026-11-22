package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path"
	"strings"
	"unicode"

	"github.com/rom/qtestframe/internal/model"
)

// Attachable entity types.
var AttachableTypes = []string{model.KindRequirement, model.KindTest, "run", "execution", model.KindExploratory}

// CleanFileName strips paths and control characters from an uploaded name.
func CleanFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
}

const fileCols = `id, project_id, kind, name, content_type, size, sha256, description, tags, uploaded_by, created_at`

func scanFile(r interface{ Scan(...any) error }) (*model.FileInfo, error) {
	var f model.FileInfo
	var project sql.NullString
	var tags string
	if err := r.Scan(&f.ID, &project, &f.Kind, &f.Name, &f.ContentType, &f.Size, &f.SHA256, &f.Description, &tags, &f.UploadedBy, &f.CreatedAt); err != nil {
		return nil, err
	}
	f.ProjectID = strPtr(project)
	f.Tags = decodeTags(tags)
	f.Links = []model.FileLink{}
	f.URL = "/api/v1/files/" + f.ID + "/content"
	if f.Kind == model.FileAvatar {
		f.URL = "/api/v1/avatars/" + f.ID
	}
	return &f, nil
}

// CreateFile stores a file. tenantID is "" for global files (avatars).
func (s *Store) CreateFile(ctx context.Context, tenantID string, f *model.FileInfo, data []byte) error {
	f.Name = CleanFileName(f.Name)
	if f.ContentType == "" {
		f.ContentType = "application/octet-stream"
	}
	sum := sha256.Sum256(data)
	f.ID, f.Size, f.SHA256, f.CreatedAt = newID(), int64(len(data)), hex.EncodeToString(sum[:]), s.now()
	f.Tags = cleanTags(f.Tags)
	f.Links = []model.FileLink{}
	f.URL = "/api/v1/files/" + f.ID + "/content"
	if f.Kind == model.FileAvatar {
		f.URL = "/api/v1/avatars/" + f.ID
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO files (id, tenant_id, project_id, kind, name, content_type, size, sha256, description, tags, uploaded_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, tenantID, nullStr(f.ProjectID), f.Kind, f.Name, f.ContentType, f.Size, f.SHA256, f.Description, encodeTags(f.Tags), f.UploadedBy, f.CreatedAt); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO file_blobs (file_id, data) VALUES (?, ?)`, f.ID, data)
		return err
	})
}

// GetFile returns file metadata (with links) of a tenant. Pass tenantID ""
// to look up global files (avatars).
func (s *Store) GetFile(ctx context.Context, tenantID, id string) (*model.FileInfo, error) {
	f, err := scanFile(s.c(ctx).row(ctx, `SELECT `+fileCols+` FROM files WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "file")
	}
	links, err := s.fileLinks(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if l := links[id]; l != nil {
		f.Links = l
	}
	return f, nil
}

// FileData returns the content of a file.
func (s *Store) FileData(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := s.c(ctx).row(ctx, `SELECT data FROM file_blobs WHERE file_id = ?`, id).Scan(&data)
	return data, mapErr(err, "file")
}

// ListFiles returns the files of a project (newest first), optionally of
// one kind, with their links.
func (s *Store) ListFiles(ctx context.Context, tenantID, projectID, kind string) ([]model.FileInfo, error) {
	q := `SELECT ` + fileCols + ` FROM files WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	} else {
		q += ` AND kind IN ('attachment', 'document', 'report')`
	}
	return s.listFiles(ctx, q+` ORDER BY created_at DESC`, args...)
}

func (s *Store) listFiles(ctx context.Context, q string, args ...any) ([]model.FileInfo, error) {
	rows, err := s.c(ctx).query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []model.FileInfo{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	links, err := s.fileLinks(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if l := links[out[i].ID]; l != nil {
			out[i].Links = l
		}
	}
	return out, nil
}

// fileLinks resolves the items files are attached to, with display keys.
func (s *Store) fileLinks(ctx context.Context, ids []string) (map[string][]model.FileLink, error) {
	out := map[string][]model.FileLink{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.c(ctx).query(ctx, `SELECT a.file_id, a.entity_type, a.entity_id, COALESCE(r.key, t.key, ru.name, et.key, xs.key, ''), COALESCE(e.run_id, ''), COALESCE(sr.execution_id, '')
		FROM attachments a
		LEFT JOIN requirements r ON a.entity_type = 'requirement' AND r.id = a.entity_id
		LEFT JOIN test_cases t ON a.entity_type = 'test' AND t.id = a.entity_id
		LEFT JOIN test_runs ru ON a.entity_type = 'run' AND ru.id = a.entity_id
		LEFT JOIN step_results sr ON a.entity_type = 'step' AND sr.id = a.entity_id
		LEFT JOIN test_executions e ON (a.entity_type = 'execution' AND e.id = a.entity_id) OR (a.entity_type = 'step' AND e.id = sr.execution_id)
		LEFT JOIN test_cases et ON et.id = e.test_case_id
		LEFT JOIN exploratory_sessions xs ON a.entity_type = 'exploratory_session' AND xs.id = a.entity_id
		WHERE a.file_id IN (`+placeholders(len(ids))+`) ORDER BY a.entity_type`, anySlice(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fid string
		var l model.FileLink
		if err := rows.Scan(&fid, &l.EntityType, &l.EntityID, &l.Key, &l.RunID, &l.ExecutionID); err != nil {
			return nil, err
		}
		out[fid] = append(out[fid], l)
	}
	return out, rows.Err()
}

// UpdateFile stores name, description and tags.
func (s *Store) UpdateFile(ctx context.Context, tenantID string, f *model.FileInfo) error {
	f.Name = CleanFileName(f.Name)
	f.Tags = cleanTags(f.Tags)
	res, err := s.c(ctx).exec(ctx, `UPDATE files SET name = ?, description = ?, tags = ? WHERE id = ? AND tenant_id = ?`, f.Name, f.Description, encodeTags(f.Tags), f.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "file")
	}
	return nil
}

// DeleteFile removes a file, its content and its links.
func (s *Store) DeleteFile(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		for _, q := range []string{`DELETE FROM attachments WHERE file_id = ?`, `DELETE FROM file_blobs WHERE file_id = ?`} {
			if _, err := c.exec(ctx, q, id); err != nil {
				return err
			}
		}
		if err := deleteItemLinks(ctx, c, "file", []string{id}); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM files WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "file")
		}
		return nil
	})
}

// Attach links a file to an item.
func (s *Store) Attach(ctx context.Context, fileID, entityType, entityID string) error {
	_, err := s.c(ctx).exec(ctx, `INSERT INTO attachments (file_id, entity_type, entity_id) VALUES (?, ?, ?)`, fileID, entityType, entityID)
	return mapErr(err, "attachment")
}

// Detach unlinks a file from an item; attachment files without remaining
// links are deleted.
func (s *Store) Detach(ctx context.Context, tenantID, fileID, entityType, entityID string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM attachments WHERE file_id = ? AND entity_type = ? AND entity_id = ?`, fileID, entityType, entityID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "attachment")
		}
		return deleteOrphanAttachments(ctx, c, tenantID, []string{fileID})
	})
}

// ListAttachments returns the files attached to an item.
func (s *Store) ListAttachments(ctx context.Context, tenantID, entityType, entityID string) ([]model.FileInfo, error) {
	return s.listFiles(ctx, `SELECT `+fileCols+` FROM files WHERE tenant_id = ? AND id IN (SELECT file_id FROM attachments WHERE entity_type = ? AND entity_id = ?) ORDER BY created_at`,
		tenantID, entityType, entityID)
}

// deleteOrphanAttachments deletes attachment-kind files (of candidates)
// that are no longer linked to anything.
func deleteOrphanAttachments(ctx context.Context, c conn, tenantID string, candidates []string) error {
	if len(candidates) == 0 {
		return nil
	}
	args := append([]any{tenantID}, anySlice(candidates)...)
	rows, err := c.query(ctx, `SELECT id FROM files WHERE tenant_id = ? AND kind = 'attachment' AND id IN (`+placeholders(len(candidates))+`)
		AND id NOT IN (SELECT file_id FROM attachments)`, args...)
	if err != nil {
		return err
	}
	orphans, err := scanStrings(rows)
	if err != nil || len(orphans) == 0 {
		return err
	}
	in := placeholders(len(orphans))
	if _, err := c.exec(ctx, `DELETE FROM file_blobs WHERE file_id IN (`+in+`)`, anySlice(orphans)...); err != nil {
		return err
	}
	_, err = c.exec(ctx, `DELETE FROM files WHERE id IN (`+in+`)`, anySlice(orphans)...)
	return err
}

// removeExecutionAttachments removes the files of deleted executions and of
// their step results (step evidence).
func removeExecutionAttachments(ctx context.Context, c conn, execs []string) error {
	if len(execs) == 0 {
		return nil
	}
	rows, err := c.query(ctx, `SELECT id FROM step_results WHERE execution_id IN (`+placeholders(len(execs))+`)`, anySlice(execs)...)
	if err != nil {
		return err
	}
	steps, err := scanStrings(rows)
	if err != nil {
		return err
	}
	if err := removeAttachments(ctx, c, "step", steps); err != nil {
		return err
	}
	return removeAttachments(ctx, c, "execution", execs)
}

// removeAttachments unlinks all files from deleted items and deletes
// attachment files that become orphaned.
func removeAttachments(ctx context.Context, c conn, entityType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	in := placeholders(len(ids))
	args := append([]any{entityType}, anySlice(ids)...)
	rows, err := c.query(ctx, `SELECT DISTINCT a.file_id, f.tenant_id FROM attachments a JOIN files f ON f.id = a.file_id WHERE a.entity_type = ? AND a.entity_id IN (`+in+`)`, args...)
	if err != nil {
		return err
	}
	byTenant := map[string][]string{}
	for rows.Next() {
		var fid, tid string
		if err := rows.Scan(&fid, &tid); err != nil {
			rows.Close()
			return err
		}
		byTenant[tid] = append(byTenant[tid], fid)
	}
	rows.Close()
	if _, err := c.exec(ctx, `DELETE FROM attachments WHERE entity_type = ? AND entity_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	for tid, files := range byTenant {
		if err := deleteOrphanAttachments(ctx, c, tid, files); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- report definitions

// CreateReportDefinition stores a saved custom report.
func (s *Store) CreateReportDefinition(ctx context.Context, tenantID string, d *model.ReportDefinition) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || len(d.Name) > 200 {
		return Invalidf("report name is required (max 200 characters)")
	}
	if _, err := s.GetProject(ctx, tenantID, d.ProjectID); err != nil {
		return err
	}
	d.ID, d.CreatedAt = newID(), s.now()
	d.UpdatedAt = d.CreatedAt
	_, err := s.c(ctx).exec(ctx, `INSERT INTO report_definitions (id, tenant_id, project_id, name, description, config, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, tenantID, d.ProjectID, d.Name, d.Description, string(d.Config), d.CreatedBy, d.CreatedAt, d.UpdatedAt)
	return mapErr(err, "report name")
}

const reportDefCols = `id, project_id, name, description, config, created_by, created_at, updated_at`

func scanReportDef(r interface{ Scan(...any) error }) (*model.ReportDefinition, error) {
	var d model.ReportDefinition
	var cfg string
	if err := r.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Description, &cfg, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.Config = []byte(cfg)
	return &d, nil
}

// GetReportDefinition returns a saved report.
func (s *Store) GetReportDefinition(ctx context.Context, tenantID, id string) (*model.ReportDefinition, error) {
	d, err := scanReportDef(s.c(ctx).row(ctx, `SELECT `+reportDefCols+` FROM report_definitions WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return d, mapErr(err, "report definition")
}

// ListReportDefinitions returns the saved reports of a project.
func (s *Store) ListReportDefinitions(ctx context.Context, tenantID, projectID string) ([]model.ReportDefinition, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+reportDefCols+` FROM report_definitions WHERE tenant_id = ? AND project_id = ? ORDER BY name`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ReportDefinition{}
	for rows.Next() {
		d, err := scanReportDef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// UpdateReportDefinition stores name, description and config.
func (s *Store) UpdateReportDefinition(ctx context.Context, tenantID string, d *model.ReportDefinition) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || len(d.Name) > 200 {
		return Invalidf("report name is required (max 200 characters)")
	}
	d.UpdatedAt = s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE report_definitions SET name = ?, description = ?, config = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		d.Name, d.Description, string(d.Config), d.UpdatedAt, d.ID, tenantID)
	if err != nil {
		return mapErr(err, "report name")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "report definition")
	}
	return nil
}

// DeleteReportDefinition removes a saved report.
func (s *Store) DeleteReportDefinition(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM report_definitions WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "report definition")
		}
		return deleteItemLinks(ctx, c, "report", []string{id})
	})
}

// ---------------------------------------------------------------- login events

// RecordLogin stores a sign-in attempt.
func (s *Store) RecordLogin(ctx context.Context, e *model.LoginEvent) error {
	e.ID, e.CreatedAt = newID(), s.now()
	_, err := s.c(ctx).exec(ctx, `INSERT INTO login_events (id, user_id, success, method, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.Success, e.Method, e.IP, truncate(e.UserAgent, 512), e.CreatedAt)
	return err
}

// ListLogins returns the newest sign-in events of a user.
func (s *Store) ListLogins(ctx context.Context, userID string, limit int) ([]model.LoginEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id, user_id, success, method, ip, user_agent, created_at FROM login_events WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LoginEvent{}
	for rows.Next() {
		var e model.LoginEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.Success, &e.Method, &e.IP, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
