package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Test data sets (migration 0057): named, versioned data shared by tests,
// configurations and runs. A table set (parameters and rows) can be the
// source of a data-driven test: the test's executions then get one
// iteration per row of the set instead of its own rows, and the run
// records which version of the set it used.

const dataSetCols = `d.id, d.project_id, d.name, d.description, d.kind, d.parameters, d.data_rows, d.classification, d.owner_id, d.expires_on, d.tags, d.status,
	d.version, d.created_by, d.created_at, d.updated_at,
	(SELECT COUNT(*) FROM test_data_usage u WHERE u.data_set_id = d.id), (SELECT MAX(u.used_at) FROM test_data_usage u WHERE u.data_set_id = d.id),
	(SELECT COUNT(*) FROM test_data_refresh_requests q WHERE q.data_set_id = d.id AND q.status = 'open'),
	(SELECT COUNT(*) FROM attachments a WHERE a.entity_type = 'test_data_set' AND a.entity_id = d.id)`

func scanDataSet(r interface{ Scan(...any) error }, today string) (*model.DataSet, error) {
	var d model.DataSet
	var params, rows, tags string
	var last sql.NullString
	if err := r.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Description, &d.Kind, &params, &rows, &d.Classification, &d.OwnerID, &d.ExpiresOn, &tags, &d.Status,
		&d.Version, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.UsageCount, &last, &d.OpenRefreshes, &d.FileCount); err != nil {
		return nil, err
	}
	d.Parameters, d.DataRows, d.Tags = decodeParams(params), decodeRows(rows), decodeTags(tags)
	d.CreatedAt, d.UpdatedAt = d.CreatedAt.UTC(), d.UpdatedAt.UTC()
	if last.Valid {
		// MAX() loses the column type on SQLite: parse the stored text
		if t, ok := parseDBTime(last.String); ok {
			d.LastUsedAt = &t
		}
	}
	d.Links = []model.DataLink{}
	d.Expired, d.Expiring = DataSetExpiry(d.ExpiresOn, today)
	return &d, nil
}

// parseDBTime parses a time as a database returns it as text.
func parseDBTime(s string) (time.Time, bool) {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// DataSetExpiry reports whether a set with the expiry date has expired
// (before today) or expires within model.DataSetExpiryDays.
func DataSetExpiry(expiresOn, today string) (expired, expiring bool) {
	if expiresOn == "" || today == "" {
		return false, false
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return false, false
	}
	if expiresOn < today {
		return true, false
	}
	return false, expiresOn <= t.AddDate(0, 0, model.DataSetExpiryDays).Format("2006-01-02")
}

// validateDataSet checks and normalises a data set.
func validateDataSet(d *model.DataSet) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > 200 {
		return Invalidf("data set name is required (at most 200 characters)")
	}
	if len(d.Description) > 20000 {
		return Invalidf("description is too long")
	}
	if d.Kind == "" {
		d.Kind = "table"
	}
	if !slices.Contains(model.DataSetKinds, d.Kind) {
		return Invalidf("kind must be one of %s", strings.Join(model.DataSetKinds, ", "))
	}
	if d.Classification == "" {
		d.Classification = "synthetic"
	}
	if !slices.Contains(model.DataClassifications, d.Classification) {
		return Invalidf("classification must be one of %s", strings.Join(model.DataClassifications, ", "))
	}
	if d.Status == "" {
		d.Status = "active"
	}
	if !slices.Contains(model.DataSetStatuses, d.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.DataSetStatuses, ", "))
	}
	d.ExpiresOn = strings.TrimSpace(d.ExpiresOn)
	if d.ExpiresOn != "" && !dateRe.MatchString(d.ExpiresOn) {
		return Invalidf("expires_on must be YYYY-MM-DD")
	}
	d.Tags = cleanTags(d.Tags)
	// the same rules as the inline data of a test
	t := model.TestCase{Parameters: d.Parameters, DataRows: d.DataRows}
	if d.Kind == "files" {
		t.Parameters, t.DataRows = nil, nil
	}
	if err := ValidateTestData(&t); err != nil {
		return err
	}
	d.Parameters, d.DataRows = t.Parameters, t.DataRows
	return nil
}

// ListDataSets returns the data sets of a project, by name, with their links.
func (s *Store) ListDataSets(ctx context.Context, tenantID, projectID string) ([]model.DataSet, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+dataSetCols+` FROM test_data_sets d WHERE d.tenant_id = ? AND d.project_id = ? ORDER BY LOWER(d.name), d.id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DataSet{}
	idx := map[string]int{}
	today := s.today()
	for rows.Next() {
		d, err := scanDataSet(rows, today)
		if err != nil {
			return nil, err
		}
		idx[d.ID] = len(out)
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	links, err := s.listDataLinks(ctx, `d.tenant_id = ? AND d.project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if i, ok := idx[l.DataSetID]; ok {
			out[i].Links = append(out[i].Links, l)
		}
	}
	return out, nil
}

// GetDataSet returns a data set with its links.
func (s *Store) GetDataSet(ctx context.Context, tenantID, id string) (*model.DataSet, error) {
	d, err := scanDataSet(s.c(ctx).row(ctx, `SELECT `+dataSetCols+` FROM test_data_sets d WHERE d.id = ? AND d.tenant_id = ?`, id, tenantID), s.today())
	if err != nil {
		return nil, mapErr(err, "data set")
	}
	if d.Links, err = s.listDataLinks(ctx, `l.data_set_id = ?`, id); err != nil {
		return nil, err
	}
	return d, nil
}

func checkDataSetName(ctx context.Context, c conn, d *model.DataSet) error {
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM test_data_sets WHERE project_id = ? AND LOWER(name) = LOWER(?) AND id <> ?`, d.ProjectID, d.Name, d.ID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("a data set named %q already exists", d.Name)
	}
	return nil
}

// CreateDataSet adds a data set.
func (s *Store) CreateDataSet(ctx context.Context, tenantID string, d *model.DataSet) error {
	if err := validateDataSet(d); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, d.ProjectID); err != nil {
		return err
	}
	d.ID, d.Version = newID(), 1
	d.CreatedAt = s.now()
	d.UpdatedAt = d.CreatedAt
	d.Links = []model.DataLink{}
	return s.tx(ctx, func(c conn) error {
		if err := checkDataSetName(ctx, c, d); err != nil {
			return err
		}
		if err := checkMember(ctx, c, tenantID, d.OwnerID, "owner"); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO test_data_sets (id, tenant_id, project_id, name, description, kind, parameters, data_rows, classification, owner_id, expires_on, tags, status,
			version, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, tenantID, d.ProjectID, d.Name, d.Description, d.Kind, encodeJSON(d.Parameters), encodeJSON(d.DataRows), d.Classification, d.OwnerID, d.ExpiresOn,
			encodeList(d.Tags), d.Status, d.Version, d.CreatedBy, d.CreatedAt, d.UpdatedAt)
		return mapErr(err, "data set name")
	})
}

// UpdateDataSet saves a data set (a new version). expectVersion > 0 fails
// with ErrStale when someone saved it in the meantime.
func (s *Store) UpdateDataSet(ctx context.Context, tenantID string, d *model.DataSet, expectVersion int) error {
	if err := validateDataSet(d); err != nil {
		return err
	}
	d.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if err := checkDataSetName(ctx, c, d); err != nil {
			return err
		}
		if err := checkMember(ctx, c, tenantID, d.OwnerID, "owner"); err != nil {
			return err
		}
		q := `UPDATE test_data_sets SET name = ?, description = ?, kind = ?, parameters = ?, data_rows = ?, classification = ?, owner_id = ?, expires_on = ?, tags = ?, status = ?,
			version = version + 1, updated_at = ? WHERE id = ? AND tenant_id = ?`
		args := []any{d.Name, d.Description, d.Kind, encodeJSON(d.Parameters), encodeJSON(d.DataRows), d.Classification, d.OwnerID, d.ExpiresOn, encodeList(d.Tags), d.Status,
			d.UpdatedAt, d.ID, tenantID}
		if expectVersion > 0 {
			q += ` AND version = ?`
			args = append(args, expectVersion)
		}
		res, err := c.exec(ctx, q, args...)
		if err != nil {
			return mapErr(err, "data set name")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if expectVersion > 0 {
				return ErrStale
			}
			return mapErr(sql.ErrNoRows, "data set")
		}
		return c.row(ctx, `SELECT version FROM test_data_sets WHERE id = ?`, d.ID).Scan(&d.Version)
	})
}

// DeleteDataSet removes a data set with its links, usage and refresh
// requests (its files stay in the project's documents).
func (s *Store) DeleteDataSet(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM test_data_sets WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "data set")
		}
		_, err = c.exec(ctx, `DELETE FROM attachments WHERE entity_type = 'test_data_set' AND entity_id = ?`, id)
		return err
	})
}

// SnapshotDataSet returns the JSON snapshot of a data set for its history.
func SnapshotDataSet(d *model.DataSet) json.RawMessage {
	c := *d
	c.UsageCount, c.LastUsedAt, c.OpenRefreshes, c.FileCount, c.Expired, c.Expiring = 0, nil, 0, 0, false, false
	links := make([]model.DataLink, 0, len(d.Links))
	for _, l := range d.Links {
		links = append(links, model.DataLink{ItemType: l.ItemType, ItemID: l.ItemID, Usage: l.Usage, ItemKey: l.ItemKey})
	}
	c.Links = links
	b, _ := json.Marshal(c)
	return b
}

// ---------------------------------------------------------------- links

const dataLinkCols = `l.id, l.data_set_id, l.item_type, l.item_id, l.usage, l.created_by, l.created_at, d.name,
	COALESCE(t.key, cf.name, r.name, ''), COALESCE(t.title, cf.description, r.description, '')`

const dataLinkFrom = ` FROM test_data_links l JOIN test_data_sets d ON d.id = l.data_set_id
	LEFT JOIN test_cases t ON l.item_type = 'test' AND t.id = l.item_id
	LEFT JOIN configurations cf ON l.item_type = 'configuration' AND cf.id = l.item_id
	LEFT JOIN test_runs r ON l.item_type = 'run' AND r.id = l.item_id`

// listDataLinks lists links whose item still exists.
func (s *Store) listDataLinks(ctx context.Context, where string, args ...any) ([]model.DataLink, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+dataLinkCols+dataLinkFrom+` WHERE `+where+` AND (t.id IS NOT NULL OR cf.id IS NOT NULL OR r.id IS NOT NULL)
		ORDER BY l.item_type, l.created_at, l.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DataLink{}
	for rows.Next() {
		var l model.DataLink
		if err := rows.Scan(&l.ID, &l.DataSetID, &l.ItemType, &l.ItemID, &l.Usage, &l.CreatedBy, &l.CreatedAt, &l.DataSetName, &l.ItemKey, &l.ItemTitle); err != nil {
			return nil, err
		}
		l.CreatedAt = l.CreatedAt.UTC()
		out = append(out, l)
	}
	return out, rows.Err()
}

// ItemDataLinks returns the data sets linked to a test, configuration or run.
func (s *Store) ItemDataLinks(ctx context.Context, tenantID, itemType, itemID string) ([]model.DataLink, error) {
	return s.listDataLinks(ctx, `l.tenant_id = ? AND l.item_type = ? AND l.item_id = ?`, tenantID, itemType, itemID)
}

// SetDataLink links a data set to an item (or changes the usage of the
// link). Only table sets can be the source of a test, and a test has at
// most one source: linking another source makes the previous one a
// reference.
func (s *Store) SetDataLink(ctx context.Context, tenantID string, l *model.DataLink) error {
	if !slices.Contains(model.DataLinkTypes, l.ItemType) {
		return Invalidf("item_type must be one of %s", strings.Join(model.DataLinkTypes, ", "))
	}
	if l.Usage == "" {
		l.Usage = "reference"
	}
	if !slices.Contains(model.DataLinkUsages, l.Usage) {
		return Invalidf("usage must be source or reference")
	}
	if l.Usage == "source" && l.ItemType != model.KindTest {
		return Invalidf("only a test can take its data from a set (usage source)")
	}
	return s.tx(ctx, func(c conn) error {
		var pid, kind string
		if err := c.row(ctx, `SELECT project_id, kind FROM test_data_sets WHERE id = ? AND tenant_id = ?`, l.DataSetID, tenantID).Scan(&pid, &kind); err != nil {
			return mapErr(err, "data set")
		}
		if l.Usage == "source" && kind != "table" {
			return Invalidf("only a table data set can be the source of a data-driven test")
		}
		table := map[string]string{model.KindTest: "test_cases", "configuration": "configurations", "run": "test_runs"}[l.ItemType]
		var ipid string
		if err := c.row(ctx, `SELECT project_id FROM `+table+` WHERE id = ? AND tenant_id = ?`, l.ItemID, tenantID).Scan(&ipid); err != nil || ipid != pid {
			return Invalidf("%s %q not found in the project of the data set", l.ItemType, l.ItemID)
		}
		if l.Usage == "source" {
			if _, err := c.exec(ctx, `UPDATE test_data_links SET usage = 'reference' WHERE item_type = 'test' AND item_id = ? AND usage = 'source' AND data_set_id <> ?`,
				l.ItemID, l.DataSetID); err != nil {
				return err
			}
		}
		res, err := c.exec(ctx, `UPDATE test_data_links SET usage = ? WHERE data_set_id = ? AND item_type = ? AND item_id = ?`, l.Usage, l.DataSetID, l.ItemType, l.ItemID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return c.row(ctx, `SELECT id, created_by, created_at FROM test_data_links WHERE data_set_id = ? AND item_type = ? AND item_id = ?`, l.DataSetID, l.ItemType, l.ItemID).
				Scan(&l.ID, &l.CreatedBy, &l.CreatedAt)
		}
		l.ID, l.CreatedAt = newID(), s.now()
		_, err = c.exec(ctx, `INSERT INTO test_data_links (id, tenant_id, data_set_id, item_type, item_id, usage, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			l.ID, tenantID, l.DataSetID, l.ItemType, l.ItemID, l.Usage, l.CreatedBy, l.CreatedAt)
		return err
	})
}

// DeleteDataLink removes a link.
func (s *Store) DeleteDataLink(ctx context.Context, tenantID, setID, linkID string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM test_data_links WHERE id = ? AND data_set_id = ? AND tenant_id = ?`, linkID, setID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "link")
	}
	return nil
}

// testDataSource returns the parameters and rows of the active table set
// a test takes its data from (ok false: the test uses its own data).
func testDataSource(ctx context.Context, c conn, testID string) (setID string, version int, params, rows string, ok bool, err error) {
	err = c.row(ctx, `SELECT d.id, d.version, d.parameters, d.data_rows FROM test_data_links l JOIN test_data_sets d ON d.id = l.data_set_id
		WHERE l.item_type = 'test' AND l.item_id = ? AND l.usage = 'source' AND d.kind = 'table' AND d.status = 'active'`, testID).Scan(&setID, &version, &params, &rows)
	if err == sql.ErrNoRows {
		return "", 0, "", "", false, nil
	}
	return setID, version, params, rows, err == nil, err
}

// recordDataUsage notes that a run used a version of a set for a test.
func recordDataUsage(ctx context.Context, c conn, tenantID, setID string, version int, runID, testID, user string, at time.Time) error {
	_, err := c.exec(ctx, `INSERT INTO test_data_usage (id, tenant_id, data_set_id, data_version, run_id, test_case_id, used_by, used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), tenantID, setID, version, runID, testID, user, at)
	return err
}

// DataSetUsage returns the uses of a set, newest first.
func (s *Store) DataSetUsage(ctx context.Context, tenantID, setID string, limit int) ([]model.DataUsage, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.c(ctx).query(ctx, `SELECT u.id, u.data_set_id, u.data_version, u.run_id, u.test_case_id, u.used_by, u.used_at, COALESCE(r.name, ''), COALESCE(t.key, '')
		FROM test_data_usage u LEFT JOIN test_runs r ON r.id = u.run_id LEFT JOIN test_cases t ON t.id = u.test_case_id
		WHERE u.data_set_id = ? AND u.tenant_id = ? ORDER BY u.used_at DESC, u.id LIMIT ?`, setID, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DataUsage{}
	for rows.Next() {
		var u model.DataUsage
		var run, test sql.NullString
		if err := rows.Scan(&u.ID, &u.DataSetID, &u.DataVersion, &run, &test, &u.UsedBy, &u.UsedAt, &u.RunName, &u.TestKey); err != nil {
			return nil, err
		}
		u.RunID, u.TestCaseID = strPtr(run), strPtr(test)
		u.UsedAt = u.UsedAt.UTC()
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- refresh requests

const refreshCols = `q.id, q.data_set_id, q.requester_id, q.requested_by, q.reason, q.needed_by, q.status, q.resolved_by, q.resolved_at, q.resolution_note,
	q.created_at, q.updated_at, d.name, d.project_id`

func scanRefresh(r interface{ Scan(...any) error }) (*model.RefreshRequest, error) {
	var q model.RefreshRequest
	var at sql.NullTime
	if err := r.Scan(&q.ID, &q.DataSetID, &q.RequesterID, &q.RequestedBy, &q.Reason, &q.NeededBy, &q.Status, &q.ResolvedBy, &at, &q.ResolutionNote,
		&q.CreatedAt, &q.UpdatedAt, &q.DataSetName, &q.ProjectID); err != nil {
		return nil, err
	}
	q.ResolvedAt = timePtr(at)
	q.CreatedAt, q.UpdatedAt = q.CreatedAt.UTC(), q.UpdatedAt.UTC()
	return &q, nil
}

func (s *Store) listRefreshes(ctx context.Context, where string, args ...any) ([]model.RefreshRequest, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+refreshCols+` FROM test_data_refresh_requests q JOIN test_data_sets d ON d.id = q.data_set_id WHERE `+where+
		` ORDER BY q.created_at DESC, q.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RefreshRequest{}
	for rows.Next() {
		q, err := scanRefresh(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

// ListRefreshRequests returns the refresh requests of a set, newest first.
func (s *Store) ListRefreshRequests(ctx context.Context, tenantID, setID string) ([]model.RefreshRequest, error) {
	return s.listRefreshes(ctx, `q.data_set_id = ? AND q.tenant_id = ?`, setID, tenantID)
}

// OpenRefreshRequests returns the open refresh requests of a tenant.
func (s *Store) OpenRefreshRequests(ctx context.Context, tenantID string) ([]model.RefreshRequest, error) {
	return s.listRefreshes(ctx, `q.tenant_id = ? AND q.status = 'open'`, tenantID)
}

// GetRefreshRequest returns a refresh request.
func (s *Store) GetRefreshRequest(ctx context.Context, tenantID, id string) (*model.RefreshRequest, error) {
	q, err := scanRefresh(s.c(ctx).row(ctx, `SELECT `+refreshCols+` FROM test_data_refresh_requests q JOIN test_data_sets d ON d.id = q.data_set_id WHERE q.id = ? AND q.tenant_id = ?`, id, tenantID))
	return q, mapErr(err, "refresh request")
}

// CreateRefreshRequest asks for a set to be refreshed.
func (s *Store) CreateRefreshRequest(ctx context.Context, tenantID string, q *model.RefreshRequest) error {
	q.Reason = strings.TrimSpace(q.Reason)
	if q.Reason == "" || utf8.RuneCountInString(q.Reason) > 5000 {
		return Invalidf("a reason is required (at most 5000 characters)")
	}
	q.NeededBy = strings.TrimSpace(q.NeededBy)
	if q.NeededBy != "" && !dateRe.MatchString(q.NeededBy) {
		return Invalidf("needed_by must be YYYY-MM-DD")
	}
	if _, err := s.GetDataSet(ctx, tenantID, q.DataSetID); err != nil {
		return err
	}
	q.ID, q.Status = newID(), "open"
	q.CreatedAt = s.now()
	q.UpdatedAt = q.CreatedAt
	_, err := s.c(ctx).exec(ctx, `INSERT INTO test_data_refresh_requests (id, tenant_id, data_set_id, requester_id, requested_by, reason, needed_by, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, q.ID, tenantID, q.DataSetID, q.RequesterID, q.RequestedBy, q.Reason, q.NeededBy, q.Status, q.CreatedAt, q.UpdatedAt)
	return err
}

// ResolveRefreshRequest closes an open refresh request as done or rejected.
func (s *Store) ResolveRefreshRequest(ctx context.Context, tenantID, id, status, user, note string) (*model.RefreshRequest, error) {
	if status != "done" && status != "rejected" {
		return nil, Invalidf("status must be done or rejected")
	}
	if utf8.RuneCountInString(note) > 5000 {
		return nil, Invalidf("the note is too long")
	}
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE test_data_refresh_requests SET status = ?, resolved_by = ?, resolved_at = ?, resolution_note = ?, updated_at = ?
		WHERE id = ? AND tenant_id = ? AND status = 'open'`, status, user, now, strings.TrimSpace(note), now, id, tenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetRefreshRequest(ctx, tenantID, id); err != nil {
			return nil, err
		}
		return nil, Conflictf("the refresh request is already closed")
	}
	return s.GetRefreshRequest(ctx, tenantID, id)
}

// ExpiringDataSets returns the active sets of a tenant that expired or
// expire within model.DataSetExpiryDays (for the owners' work lists).
func (s *Store) ExpiringDataSets(ctx context.Context, tenantID string) ([]model.DataSet, error) {
	limit := s.now().AddDate(0, 0, model.DataSetExpiryDays).Format("2006-01-02")
	rows, err := s.c(ctx).query(ctx, `SELECT `+dataSetCols+` FROM test_data_sets d WHERE d.tenant_id = ? AND d.status = 'active' AND d.expires_on <> '' AND d.expires_on <= ?
		ORDER BY d.expires_on, d.id`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DataSet{}
	today := s.today()
	for rows.Next() {
		d, err := scanDataSet(rows, today)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
