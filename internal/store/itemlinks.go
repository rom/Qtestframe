package store

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Limits of item links.
const (
	MaxLinkURL   = 2000
	MaxLinkLabel = 200
)

// linkTables maps a link end type to the table, key column and title
// column used to resolve it.
var linkTables = map[string]struct{ table, key, title, extra string }{
	model.KindRequirement: {"requirements", "key", "title", "''"},
	model.KindTest:        {"test_cases", "key", "title", "''"},
	"run":                 {"test_runs", "name", "description", "status"},
	"file":                {"files", "name", "description", "kind"},
	"report":              {"report_definitions", "name", "description", "''"},
}

type linkEnd struct{ key, title, extra, project string }

// resolveLinkEnds looks up keys and titles of items of one type.
func resolveLinkEnds(ctx context.Context, c conn, tenantID, typ string, ids []string) (map[string]linkEnd, error) {
	out := map[string]linkEnd{}
	t, ok := linkTables[typ]
	if !ok || len(ids) == 0 {
		return out, nil
	}
	rows, err := c.query(ctx, `SELECT id, `+t.key+`, `+t.title+`, `+t.extra+`, COALESCE(project_id, '') FROM `+t.table+` WHERE tenant_id = ? AND id IN (`+placeholders(len(ids))+`)`,
		append([]any{tenantID}, anySlice(ids)...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var e linkEnd
		if err := rows.Scan(&id, &e.key, &e.title, &e.extra, &e.project); err != nil {
			return nil, err
		}
		out[id] = e
	}
	return out, rows.Err()
}

// validLinkURL accepts absolute http(s) and mailto URLs.
func validLinkURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxLinkURL {
		return "", Invalidf("url is required (at most %d characters)", MaxLinkURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", Invalidf("invalid url")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", Invalidf("url must include a host")
		}
	case "mailto":
		if u.Opaque == "" {
			return "", Invalidf("mailto url must include an address")
		}
	default:
		return "", Invalidf("url must start with http://, https:// or mailto:")
	}
	return raw, nil
}

// CreateItemLink adds a link. The source and an internal target must
// belong to the same project; files must be documents or saved reports of
// the project's archive.
func (s *Store) CreateItemLink(ctx context.Context, tenantID string, l *model.ItemLink) error {
	if !slices.Contains(model.ItemLinkSourceTypes, l.SourceType) {
		return Invalidf("source_type must be one of %s", strings.Join(model.ItemLinkSourceTypes, ", "))
	}
	if !slices.Contains(model.ItemLinkTargetTypes, l.TargetType) {
		return Invalidf("target_type must be one of %s", strings.Join(model.ItemLinkTargetTypes, ", "))
	}
	l.Label = strings.TrimSpace(l.Label)
	if utf8.RuneCountInString(l.Label) > MaxLinkLabel {
		return Invalidf("label may have at most %d characters", MaxLinkLabel)
	}
	c := s.c(ctx)
	src, err := resolveLinkEnds(ctx, c, tenantID, l.SourceType, []string{l.SourceID})
	if err != nil {
		return err
	}
	se, ok := src[l.SourceID]
	if !ok {
		return mapErr(sql.ErrNoRows, l.SourceType)
	}
	l.ProjectID = se.project
	if l.TargetType == "url" {
		if l.URL, err = validLinkURL(l.URL); err != nil {
			return err
		}
		l.TargetID = nil
	} else {
		l.URL = ""
		if l.TargetID == nil || *l.TargetID == "" {
			return Invalidf("target_id is required")
		}
		if l.TargetType == l.SourceType && *l.TargetID == l.SourceID {
			return Invalidf("an item cannot link to itself")
		}
		tgt, err := resolveLinkEnds(ctx, c, tenantID, l.TargetType, []string{*l.TargetID})
		if err != nil {
			return err
		}
		te, ok := tgt[*l.TargetID]
		if !ok {
			return mapErr(sql.ErrNoRows, l.TargetType)
		}
		if te.project != l.ProjectID {
			return Invalidf("links can only point to items of the same project")
		}
		if l.TargetType == "file" && te.extra != "document" && te.extra != "report" {
			return Invalidf("only documents and saved reports of the archive can be linked")
		}
	}
	// no duplicates
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM item_links WHERE tenant_id = ? AND source_type = ? AND source_id = ? AND target_type = ? AND COALESCE(target_id, '') = ? AND url = ?`,
		tenantID, l.SourceType, l.SourceID, l.TargetType, deref(l.TargetID), l.URL).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("this link already exists")
	}
	l.ID = newID()
	l.CreatedAt = s.now()
	is := l.Issue
	if is == nil || l.TargetType != "url" {
		is, l.Issue = &model.LinkedIssue{}, nil
	}
	_, err = c.exec(ctx, `INSERT INTO item_links (id, tenant_id, project_id, source_type, source_id, target_type, target_id, url, label, created_by, created_at,
		issue_tracker, issue_key, issue_status, issue_state, issue_synced_at, issue_passed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, tenantID, l.ProjectID, l.SourceType, l.SourceID, l.TargetType, nullStr(l.TargetID), l.URL, l.Label, l.CreatedBy, l.CreatedAt,
		is.Tracker, is.Key, is.Status, is.State, nullTime(is.SyncedAt), nullTime(is.PassedAt))
	if err != nil {
		return err
	}
	return s.decorateItemLinks(ctx, tenantID, []*model.ItemLink{l})
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

const itemLinkCols = `id, project_id, source_type, source_id, target_type, target_id, url, label, created_by, created_at,
	issue_tracker, issue_key, issue_status, issue_state, issue_synced_at, issue_passed_at`

func scanItemLink(r interface{ Scan(...any) error }) (*model.ItemLink, error) {
	var l model.ItemLink
	var target sql.NullString
	var is model.LinkedIssue
	var synced, passed sql.NullTime
	if err := r.Scan(&l.ID, &l.ProjectID, &l.SourceType, &l.SourceID, &l.TargetType, &target, &l.URL, &l.Label, &l.CreatedBy, &l.CreatedAt,
		&is.Tracker, &is.Key, &is.Status, &is.State, &synced, &passed); err != nil {
		return nil, err
	}
	l.TargetID = strPtr(target)
	if is.Key != "" {
		is.SyncedAt, is.PassedAt = timePtr(synced), timePtr(passed)
		l.Issue = &is
	}
	return &l, nil
}

// SetItemLinkIssue records what was read of a link's issue (and the URL
// and label, which follow the issue).
func (s *Store) SetItemLinkIssue(ctx context.Context, tenantID string, l *model.ItemLink) error {
	is := l.Issue
	if is == nil {
		is = &model.LinkedIssue{}
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE item_links SET url = ?, label = ?, issue_tracker = ?, issue_key = ?, issue_status = ?, issue_state = ?, issue_synced_at = ?, issue_passed_at = ?
		WHERE tenant_id = ? AND id = ? AND target_type = 'url'`,
		l.URL, l.Label, is.Tracker, is.Key, is.Status, is.State, nullTime(is.SyncedAt), nullTime(is.PassedAt), tenantID, l.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "link")
	}
	return nil
}

// IssueLinks returns the links to tracker issues carried by requirements
// of a project (all of them when requirementIDs is nil), oldest first.
func (s *Store) IssueLinks(ctx context.Context, tenantID, projectID string, requirementIDs []string) ([]model.ItemLink, error) {
	q := `SELECT ` + itemLinkCols + ` FROM item_links WHERE tenant_id = ? AND project_id = ? AND source_type = ? AND target_type = 'url' AND issue_key <> ''`
	args := []any{tenantID, projectID, model.KindRequirement}
	if requirementIDs != nil {
		if len(requirementIDs) == 0 {
			return []model.ItemLink{}, nil
		}
		q += ` AND source_id IN (` + placeholders(len(requirementIDs)) + `)`
		args = append(args, anySlice(requirementIDs)...)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	var ptrs []*model.ItemLink
	for rows.Next() {
		l, err := scanItemLink(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		l.Direction = "outgoing"
		ptrs = append(ptrs, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.decorateItemLinks(ctx, tenantID, ptrs); err != nil {
		return nil, err
	}
	out := make([]model.ItemLink, 0, len(ptrs))
	for _, l := range ptrs {
		out = append(out, *l)
	}
	return out, nil
}

// GetItemLink returns a link.
func (s *Store) GetItemLink(ctx context.Context, tenantID, id string) (*model.ItemLink, error) {
	l, err := scanItemLink(s.c(ctx).row(ctx, `SELECT `+itemLinkCols+` FROM item_links WHERE tenant_id = ? AND id = ?`, tenantID, id))
	if err != nil {
		return nil, mapErr(err, "link")
	}
	return l, s.decorateItemLinks(ctx, tenantID, []*model.ItemLink{l})
}

// ListItemLinks lists the links of an item: those it carries (outgoing)
// and those pointing at it (incoming), outgoing first.
func (s *Store) ListItemLinks(ctx context.Context, tenantID, itemType, itemID string) ([]model.ItemLink, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+itemLinkCols+` FROM item_links WHERE tenant_id = ? AND ((source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)) ORDER BY created_at, id`,
		tenantID, itemType, itemID, itemType, itemID)
	if err != nil {
		return nil, err
	}
	var ptrs []*model.ItemLink
	for rows.Next() {
		l, err := scanItemLink(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		l.Direction = "outgoing"
		if !(l.SourceType == itemType && l.SourceID == itemID) {
			l.Direction = "incoming"
		}
		ptrs = append(ptrs, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.decorateItemLinks(ctx, tenantID, ptrs); err != nil {
		return nil, err
	}
	out := make([]model.ItemLink, 0, len(ptrs))
	for _, dir := range []string{"outgoing", "incoming"} {
		for _, l := range ptrs {
			if l.Direction == dir {
				out = append(out, *l)
			}
		}
	}
	return out, nil
}

// LinkSourceProjects returns the project of each existing source item of
// one type (requirement, test or run); unknown ids are left out.
func (s *Store) LinkSourceProjects(ctx context.Context, tenantID, sourceType string, ids []string) (map[string]string, error) {
	ends, err := resolveLinkEnds(ctx, s.c(ctx), tenantID, sourceType, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(ends))
	for id, e := range ends {
		out[id] = e.project
	}
	return out, nil
}

// ListProjectItemLinks returns the links carried by items of one source
// type in a project (all source types when sourceType is empty), oldest
// first.
func (s *Store) ListProjectItemLinks(ctx context.Context, tenantID, projectID, sourceType string) ([]model.ItemLink, error) {
	q := `SELECT ` + itemLinkCols + ` FROM item_links WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if sourceType != "" {
		q += ` AND source_type = ?`
		args = append(args, sourceType)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	var ptrs []*model.ItemLink
	for rows.Next() {
		l, err := scanItemLink(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		l.Direction = "outgoing"
		ptrs = append(ptrs, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.decorateItemLinks(ctx, tenantID, ptrs); err != nil {
		return nil, err
	}
	out := make([]model.ItemLink, 0, len(ptrs))
	for _, l := range ptrs {
		out = append(out, *l)
	}
	return out, nil
}

// decorateItemLinks fills keys and titles of both ends.
func (s *Store) decorateItemLinks(ctx context.Context, tenantID string, links []*model.ItemLink) error {
	ids := map[string][]string{}
	for _, l := range links {
		ids[l.SourceType] = append(ids[l.SourceType], l.SourceID)
		if l.TargetID != nil {
			ids[l.TargetType] = append(ids[l.TargetType], *l.TargetID)
		}
	}
	ends := map[string]map[string]linkEnd{}
	for typ, list := range ids {
		m, err := resolveLinkEnds(ctx, s.c(ctx), tenantID, typ, list)
		if err != nil {
			return err
		}
		ends[typ] = m
	}
	for _, l := range links {
		if e, ok := ends[l.SourceType][l.SourceID]; ok {
			l.SourceKey, l.SourceTitle = e.key, e.title
		}
		if l.TargetID != nil {
			if e, ok := ends[l.TargetType][*l.TargetID]; ok {
				l.TargetKey, l.TargetTitle = e.key, e.title
				if l.TargetType == "file" {
					l.TargetKind = e.extra
				}
			}
		}
	}
	return nil
}

// UpdateItemLinkLabel changes the label of a link.
func (s *Store) UpdateItemLinkLabel(ctx context.Context, tenantID, id, label string) error {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > MaxLinkLabel {
		return Invalidf("label may have at most %d characters", MaxLinkLabel)
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE item_links SET label = ? WHERE tenant_id = ? AND id = ?`, label, tenantID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "link")
	}
	return nil
}

// DeleteItemLink removes a link.
func (s *Store) DeleteItemLink(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM item_links WHERE tenant_id = ? AND id = ?`, tenantID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "link")
	}
	return nil
}

// deleteItemLinks removes the links from and to deleted items.
func deleteItemLinks(ctx context.Context, c conn, typ string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	in := placeholders(len(ids))
	args := append([]any{typ}, anySlice(ids)...)
	args = append(args, typ)
	args = append(args, anySlice(ids)...)
	_, err := c.exec(ctx, `DELETE FROM item_links WHERE (source_type = ? AND source_id IN (`+in+`)) OR (target_type = ? AND target_id IN (`+in+`))`, args...)
	return err
}

// CountItemLinks returns the number of links (both directions) per item of a type.
func (s *Store) CountItemLinks(ctx context.Context, tenantID, projectID, typ string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT source_id, target_id, source_type, target_type FROM item_links WHERE tenant_id = ? AND project_id = ? AND (source_type = ? OR target_type = ?)`, tenantID, projectID, typ, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var src, st, tt string
		var tgt sql.NullString
		if err := rows.Scan(&src, &tgt, &st, &tt); err != nil {
			return nil, err
		}
		if st == typ {
			out[src]++
		}
		if tt == typ && tgt.Valid {
			out[tgt.String]++
		}
	}
	return out, rows.Err()
}
