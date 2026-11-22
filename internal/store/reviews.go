package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- review requests

const reviewCols = `r.id, r.project_id, r.entity_type, r.entity_id, r.entity_version, r.reviewer_id, COALESCE(u.email, ''), COALESCE(u.display_name, ''),
	r.requested_by, r.message, r.checklist, r.status, r.comment, r.due_date, r.decided_at, r.created_at, r.updated_at`

const reviewFrom = ` FROM review_requests r LEFT JOIN users u ON u.id = r.reviewer_id`

func scanReview(row interface{ Scan(...any) error }) (*model.ReviewRequest, error) {
	var v model.ReviewRequest
	var list string
	var at sql.NullTime
	if err := row.Scan(&v.ID, &v.ProjectID, &v.EntityType, &v.EntityID, &v.EntityVersion, &v.ReviewerID, &v.ReviewerEmail, &v.ReviewerName,
		&v.RequestedBy, &v.Message, &list, &v.Status, &v.Comment, &v.DueDate, &at, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(list), &v.Checklist)
	if v.Checklist == nil {
		v.Checklist = []model.ChecklistItem{}
	}
	v.DecidedAt = timePtr(at)
	return &v, nil
}

func cleanChecklist(in []model.ChecklistItem) ([]model.ChecklistItem, error) {
	out := []model.ChecklistItem{}
	for _, c := range in {
		c.Text = strings.TrimSpace(c.Text)
		if c.Text == "" {
			continue
		}
		if len([]rune(c.Text)) > 300 || len([]rune(c.Note)) > 2000 {
			return nil, Invalidf("checklist items are at most 300 characters (notes 2000)")
		}
		out = append(out, c)
	}
	if len(out) > 40 {
		return nil, Invalidf("at most 40 checklist items")
	}
	return out, nil
}

// entityInfo returns project, key, title and version of a requirement or test.
func (s *Store) entityInfo(ctx context.Context, tenantID, kind, id string) (project, key, title string, version int, err error) {
	switch kind {
	case model.KindRequirement:
		r, e := s.GetRequirement(ctx, tenantID, id)
		if e != nil {
			return "", "", "", 0, e
		}
		return r.ProjectID, r.Key, r.Title, r.Version, nil
	case model.KindTest:
		t, e := s.GetTest(ctx, tenantID, id)
		if e != nil {
			return "", "", "", 0, e
		}
		return t.ProjectID, t.Key, t.Title, t.Version, nil
	}
	return "", "", "", 0, Invalidf("reviews are for requirements and tests")
}

// CreateReviewRequests asks each reviewer to review an item; reviewers that
// already have a pending request for it are skipped.
func (s *Store) CreateReviewRequests(ctx context.Context, tenantID string, tmpl model.ReviewRequest, reviewers []string) ([]model.ReviewRequest, error) {
	project, _, _, version, err := s.entityInfo(ctx, tenantID, tmpl.EntityType, tmpl.EntityID)
	if err != nil {
		return nil, err
	}
	if len(reviewers) == 0 {
		return nil, Invalidf("choose at least one reviewer")
	}
	if tmpl.Checklist, err = cleanChecklist(tmpl.Checklist); err != nil {
		return nil, err
	}
	tmpl.DueDate = strings.TrimSpace(tmpl.DueDate)
	if tmpl.DueDate != "" && !dateRe.MatchString(tmpl.DueDate) {
		return nil, Invalidf("due_date must be YYYY-MM-DD")
	}
	if len([]rune(tmpl.Message)) > 5000 {
		return nil, Invalidf("the message is at most 5000 characters")
	}
	list, _ := json.Marshal(tmpl.Checklist)
	var ids []string
	err = s.tx(ctx, func(c conn) error {
		for _, uid := range reviewers {
			var n int
			if err := c.row(ctx, `SELECT COUNT(*) FROM memberships WHERE tenant_id = ? AND user_id = ?`, tenantID, uid).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return Invalidf("reviewer %s is not a member of the organisation", uid)
			}
			if err := c.row(ctx, `SELECT COUNT(*) FROM review_requests WHERE entity_type = ? AND entity_id = ? AND reviewer_id = ? AND status = 'pending'`,
				tmpl.EntityType, tmpl.EntityID, uid).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			id, now := newID(), s.now()
			if _, err := c.exec(ctx, `INSERT INTO review_requests (id, tenant_id, project_id, entity_type, entity_id, entity_version, reviewer_id, requested_by, message, checklist, status, due_date, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?)`, id, tenantID, project, tmpl.EntityType, tmpl.EntityID, version, uid, tmpl.RequestedBy,
				tmpl.Message, string(list), tmpl.DueDate, now, now); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []model.ReviewRequest{}
	for _, id := range ids {
		v, err := s.GetReviewRequest(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// decorateReviews fills key, title and whether the item changed since.
func (s *Store) decorateReviews(ctx context.Context, tenantID string, list []model.ReviewRequest) {
	type info struct {
		key, title string
		version    int
	}
	cache := map[string]info{}
	for i := range list {
		v := &list[i]
		k := v.EntityType + ":" + v.EntityID
		in, ok := cache[k]
		if !ok {
			_, key, title, version, err := s.entityInfo(ctx, tenantID, v.EntityType, v.EntityID)
			if err == nil {
				in = info{key, title, version}
			}
			cache[k] = in
		}
		v.EntityKey, v.EntityTitle = in.key, in.title
		v.Outdated = in.version > v.EntityVersion
	}
}

// GetReviewRequest returns one review request.
func (s *Store) GetReviewRequest(ctx context.Context, tenantID, id string) (*model.ReviewRequest, error) {
	v, err := scanReview(s.c(ctx).row(ctx, `SELECT `+reviewCols+reviewFrom+` WHERE r.id = ? AND r.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "review request")
	}
	list := []model.ReviewRequest{*v}
	s.decorateReviews(ctx, tenantID, list)
	return &list[0], nil
}

// ReviewFilter selects review requests; empty fields match everything.
type ReviewFilter struct {
	ProjectID, EntityType, EntityID, ReviewerID, Status, RequestedBy string
}

// ListReviewRequests lists review requests, newest first.
func (s *Store) ListReviewRequests(ctx context.Context, tenantID string, f ReviewFilter) ([]model.ReviewRequest, error) {
	where := []string{"r.tenant_id = ?"}
	args := []any{tenantID}
	for _, x := range []struct{ col, v string }{{"r.project_id", f.ProjectID}, {"r.entity_type", f.EntityType}, {"r.entity_id", f.EntityID}, {"r.reviewer_id", f.ReviewerID}, {"r.status", f.Status}, {"r.requested_by", f.RequestedBy}} {
		if x.v != "" {
			where = append(where, x.col+" = ?")
			args = append(args, x.v)
		}
	}
	rows, err := s.c(ctx).query(ctx, `SELECT `+reviewCols+reviewFrom+` WHERE `+strings.Join(where, " AND ")+` ORDER BY r.created_at DESC, r.id LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	out := []model.ReviewRequest{}
	for rows.Next() {
		v, err := scanReview(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.decorateReviews(ctx, tenantID, out)
	return out, nil
}

// UpdateReviewRequest saves checklist, comment and status (a decision sets decided_at).
func (s *Store) UpdateReviewRequest(ctx context.Context, tenantID string, v *model.ReviewRequest) error {
	var err error
	if v.Checklist, err = cleanChecklist(v.Checklist); err != nil {
		return err
	}
	if !slices.Contains(model.ReviewStatuses, v.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.ReviewStatuses, ", "))
	}
	if len([]rune(v.Comment)) > 5000 {
		return Invalidf("the comment is at most 5000 characters")
	}
	v.UpdatedAt = s.now()
	if v.Status != "pending" && v.DecidedAt == nil {
		t := v.UpdatedAt
		v.DecidedAt = &t
	}
	list, _ := json.Marshal(v.Checklist)
	res, err := s.c(ctx).exec(ctx, `UPDATE review_requests SET checklist = ?, comment = ?, status = ?, decided_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		string(list), v.Comment, v.Status, nullTime(v.DecidedAt), v.UpdatedAt, v.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "review request")
	}
	return nil
}

// ---------------------------------------------------------------- sign-offs

// BaselineHash is the SHA-256 of a baseline's items (sorted).
func BaselineHash(b *model.Baseline) string {
	items := append([]model.BaselineItem(nil), b.Items...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].EntityType != items[j].EntityType {
			return items[i].EntityType < items[j].EntityType
		}
		return items[i].EntityID < items[j].EntityID
	})
	h := sha256.New()
	h.Write([]byte(b.ID + "\n" + b.Name + "\n"))
	for _, it := range items {
		h.Write([]byte(it.EntityType + "\x00" + it.EntityID + "\x00"))
		h.Write([]byte(strings.TrimSpace(string(it.Snapshot))))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SignBaseline records a signature on the baseline's current content.
func (s *Store) SignBaseline(ctx context.Context, tenantID string, so *model.Signoff) error {
	if !slices.Contains(model.SignoffMeanings, so.Meaning) {
		return Invalidf("meaning must be one of %s", strings.Join(model.SignoffMeanings, ", "))
	}
	if len([]rune(so.Comment)) > 2000 {
		return Invalidf("the comment is at most 2000 characters")
	}
	b, err := s.GetBaseline(ctx, tenantID, so.BaselineID)
	if err != nil {
		return err
	}
	so.ID = newID()
	so.ContentHash = BaselineHash(b)
	so.SignedAt = s.now()
	so.Valid = true
	_, err = s.c(ctx).exec(ctx, `INSERT INTO baseline_signoffs (id, tenant_id, baseline_id, user_id, user_email, user_name, meaning, comment, content_hash, signed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		so.ID, tenantID, so.BaselineID, so.UserID, so.UserEmail, so.UserName, so.Meaning, so.Comment, so.ContentHash, so.SignedAt)
	if err != nil && db.IsUniqueViolation(err) {
		return Conflictf("you have already signed this baseline as %q", so.Meaning)
	}
	return err
}

// ListSignoffs lists the signatures of a baseline and checks them against
// its content.
func (s *Store) ListSignoffs(ctx context.Context, tenantID, baselineID string) ([]model.Signoff, error) {
	b, err := s.GetBaseline(ctx, tenantID, baselineID)
	if err != nil {
		return nil, err
	}
	hash := BaselineHash(b)
	rows, err := s.c(ctx).query(ctx, `SELECT id, baseline_id, user_id, user_email, user_name, meaning, comment, content_hash, signed_at FROM baseline_signoffs WHERE tenant_id = ? AND baseline_id = ? ORDER BY signed_at, id`, tenantID, baselineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Signoff{}
	for rows.Next() {
		var so model.Signoff
		if err := rows.Scan(&so.ID, &so.BaselineID, &so.UserID, &so.UserEmail, &so.UserName, &so.Meaning, &so.Comment, &so.ContentHash, &so.SignedAt); err != nil {
			return nil, err
		}
		so.Valid = so.ContentHash == hash
		out = append(out, so)
	}
	return out, rows.Err()
}

// SignoffCount returns the number of signatures of a baseline.
func (s *Store) SignoffCount(ctx context.Context, tenantID, baselineID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM baseline_signoffs WHERE tenant_id = ? AND baseline_id = ?`, tenantID, baselineID).Scan(&n)
	return n, err
}

// SetReviewDueDate changes the due date of a review request ("" removes it).
func (s *Store) SetReviewDueDate(ctx context.Context, tenantID, id, due string) error {
	due = strings.TrimSpace(due)
	if due != "" && !dateRe.MatchString(due) {
		return Invalidf("due_date must be YYYY-MM-DD")
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE review_requests SET due_date = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, due, s.now(), id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "review request")
	}
	return nil
}

// InReviewItem is a requirement or test whose status is "review".
type InReviewItem struct {
	EntityType  string    `json:"entity_type"`
	EntityID    string    `json:"entity_id"`
	Key         string    `json:"key"`
	Title       string    `json:"title"`
	ProjectID   string    `json:"project_id"`
	ProjectKey  string    `json:"project_key"`
	ProjectName string    `json:"project_name"`
	Owner       string    `json:"owner"`
	CreatedBy   string    `json:"created_by"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListInReview returns the requirements and tests of all projects of a
// tenant whose status is "review", oldest change first (they waited longest).
func (s *Store) ListInReview(ctx context.Context, tenantID string) ([]InReviewItem, error) {
	rows, err := s.c(ctx).query(ctx, `
		SELECT 'requirement', r.id, r.key, r.title, r.project_id, p.key, p.name, r.owner, r.created_by, r.version, r.updated_at
		FROM requirements r JOIN projects p ON p.id = r.project_id
		WHERE r.tenant_id = ? AND r.status = 'review'
		UNION ALL
		SELECT 'test', t.id, t.key, t.title, t.project_id, p.key, p.name, t.owner, t.created_by, t.version, t.updated_at
		FROM test_cases t JOIN projects p ON p.id = t.project_id
		WHERE t.tenant_id = ? AND t.status = 'review'`, tenantID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InReviewItem{}
	for rows.Next() {
		var it InReviewItem
		if err := rows.Scan(&it.EntityType, &it.EntityID, &it.Key, &it.Title, &it.ProjectID, &it.ProjectKey, &it.ProjectName, &it.Owner, &it.CreatedBy, &it.Version, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out, nil
}
