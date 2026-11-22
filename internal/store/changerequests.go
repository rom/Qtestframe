package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Change requests and the change control settings of a project (see
// model.ChangeRequest).

// MaxCRItems caps the items of one change request.
const MaxCRItems = 500

// ---------------------------------------------------------------- settings

// DefaultChangeControl returns the settings of a project without its own.
func DefaultChangeControl() *model.ChangeControlSettings {
	return &model.ChangeControlSettings{Mode: "off", ProtectedStatuses: []string{}, LockSignedBaselines: true,
		Board: model.ChangeBoard{Users: []string{}, Roles: []string{}, Quorum: "all"}}
}

// EffectiveProtectedStatuses returns the statuses under change control.
func EffectiveProtectedStatuses(c *model.ChangeControlSettings) []string {
	if len(c.ProtectedStatuses) == 0 {
		return model.DefaultProtectedStatuses
	}
	return c.ProtectedStatuses
}

// ChangeControl returns the change control settings of a project.
func (s *Store) ChangeControl(ctx context.Context, tenantID, projectID string) (*model.ChangeControlSettings, error) {
	out := DefaultChangeControl()
	var statuses, board string
	var at time.Time
	err := s.c(ctx).row(ctx, `SELECT mode, protected_statuses, lock_signed_baselines, board, quorum, signature_meaning, baseline_on_close, updated_by, updated_at
		FROM change_control_settings WHERE project_id = ? AND tenant_id = ?`, projectID, tenantID).
		Scan(&out.Mode, &statuses, &out.LockSignedBaselines, &board, &out.Board.Quorum, &out.SignatureMeaning, &out.BaselineOnClose, &out.UpdatedBy, &at)
	if db.IsNoRows(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.ProtectedStatuses = decodeTags(statuses)
	_ = json.Unmarshal([]byte(board), &out.Board)
	if out.Board.Users == nil {
		out.Board.Users = []string{}
	}
	if out.Board.Roles == nil {
		out.Board.Roles = []string{}
	}
	if out.Board.Quorum == "" {
		out.Board.Quorum = "all"
	}
	at = at.UTC()
	out.UpdatedAt = &at
	return out, nil
}

// ValidQuorum reports whether q is all, any or a positive number.
func ValidQuorum(q string) bool {
	if q == "all" || q == "any" {
		return true
	}
	n, err := strconv.Atoi(q)
	return err == nil && n >= 1 && n <= 50
}

// SetChangeControl saves the change control settings of a project.
func (s *Store) SetChangeControl(ctx context.Context, tenantID, projectID string, c *model.ChangeControlSettings) error {
	if c.Mode == "" {
		c.Mode = "off"
	}
	if !slices.Contains(model.ChangeControlModes, c.Mode) {
		return Invalidf("mode must be one of %s", strings.Join(model.ChangeControlModes, ", "))
	}
	c.ProtectedStatuses = cleanTags(c.ProtectedStatuses)
	for _, st := range c.ProtectedStatuses {
		if !slices.Contains(model.RequirementStatuses, st) && !slices.Contains(model.TestStatuses, st) {
			return Invalidf("unknown status %q", st)
		}
	}
	c.Board.Users = normalizeEmails(c.Board.Users)
	for _, e := range c.Board.Users {
		if !strings.Contains(e, "@") || len(e) > 320 {
			return Invalidf("board member %q is not an e-mail address", e)
		}
	}
	c.Board.Roles = cleanTags(c.Board.Roles)
	if len(c.Board.Users)+len(c.Board.Roles) > 30 {
		return Invalidf("at most 30 board members and roles")
	}
	c.Board.Quorum = strings.TrimSpace(c.Board.Quorum)
	if c.Board.Quorum == "" {
		c.Board.Quorum = "all"
	}
	if !ValidQuorum(c.Board.Quorum) {
		return Invalidf("quorum must be all, any or a number")
	}
	c.SignatureMeaning = strings.TrimSpace(c.SignatureMeaning)
	if len([]rune(c.SignatureMeaning)) > 500 {
		return Invalidf("the signature meaning is at most 500 characters")
	}
	if _, err := s.GetProject(ctx, tenantID, projectID); err != nil {
		return err
	}
	now := s.now()
	c.UpdatedAt = &now
	board, _ := json.Marshal(c.Board)
	return s.tx(ctx, func(cn conn) error {
		if _, err := cn.exec(ctx, `DELETE FROM change_control_settings WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		_, err := cn.exec(ctx, `INSERT INTO change_control_settings (project_id, tenant_id, mode, protected_statuses, lock_signed_baselines, board, quorum, signature_meaning, baseline_on_close, updated_by, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, projectID, tenantID, c.Mode, encodeTags(c.ProtectedStatuses), c.LockSignedBaselines, string(board), c.Board.Quorum,
			c.SignatureMeaning, c.BaselineOnClose, c.UpdatedBy, now)
		return err
	})
}

// SignedBaselinesOf returns the names of the signed baselines that hold an
// item (a baseline with at least one sign-off).
func (s *Store) SignedBaselinesOf(ctx context.Context, tenantID, kind, id string) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT b.name FROM baseline_items bi JOIN baselines b ON b.id = bi.baseline_id
		WHERE b.tenant_id = ? AND bi.entity_type = ? AND bi.entity_id = ? AND EXISTS (SELECT 1 FROM baseline_signoffs so WHERE so.baseline_id = b.id) ORDER BY b.name`, tenantID, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- change requests

const crCols = `id, project_id, key, title, reason, category, priority, status, impact_action, impact, impact_notes, effort, assessed_by, assessed_at, links,
	baseline_on_close, baseline_id, decided_by, decided_at, decision_comment, applied_by, applied_at, verified_by, verified_at, closed_by, closed_at, created_by, created_at, updated_at`

func scanCR(r interface{ Scan(...any) error }) (*model.ChangeRequest, error) {
	var c model.ChangeRequest
	var impact, links string
	var assessed, decided, applied, verified, closed sql.NullTime
	if err := r.Scan(&c.ID, &c.ProjectID, &c.Key, &c.Title, &c.Reason, &c.Category, &c.Priority, &c.Status, &c.ImpactAction, &impact, &c.ImpactNotes, &c.Effort,
		&c.AssessedBy, &assessed, &links, &c.BaselineOnClose, &c.BaselineID, &c.DecidedBy, &decided, &c.DecisionComment, &c.AppliedBy, &applied,
		&c.VerifiedBy, &verified, &c.ClosedBy, &closed, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if impact != "" {
		var imp model.ChangeImpact
		if json.Unmarshal([]byte(impact), &imp) == nil {
			c.Impact = &imp
		}
	}
	_ = json.Unmarshal([]byte(links), &c.Links)
	normLinks(&c.Links)
	c.AssessedAt, c.DecidedAt, c.AppliedAt, c.VerifiedAt, c.ClosedAt = timePtr(assessed), timePtr(decided), timePtr(applied), timePtr(verified), timePtr(closed)
	return &c, nil
}

func normLinks(l *model.ChangeRequestLinks) {
	l.DefectIDs, l.RiskIDs, l.Issues = cleanTags(l.DefectIDs), cleanTags(l.RiskIDs), cleanTags(l.Issues)
}

// CREditable reports whether a change request in status may be edited
// (its fields and items).
func CREditable(status string) bool { return status == "draft" }

// validateCR checks and normalises the editable fields.
func validateCR(c *model.ChangeRequest) error {
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" || len([]rune(c.Title)) > 300 {
		return Invalidf("change request title is required (at most 300 characters)")
	}
	c.Reason = strings.TrimSpace(c.Reason)
	if len([]rune(c.Reason)) > 8000 {
		return Invalidf("the reason is at most 8000 characters")
	}
	if c.Category == "" {
		c.Category = "correction"
	}
	if !slices.Contains(model.CRCategories, c.Category) {
		return Invalidf("category must be one of %s", strings.Join(model.CRCategories, ", "))
	}
	if c.Priority == "" {
		c.Priority = "medium"
	}
	if !slices.Contains(model.Priorities, c.Priority) {
		return Invalidf("priority must be one of %s", strings.Join(model.Priorities, ", "))
	}
	if c.ImpactAction == "" {
		c.ImpactAction = model.ImpactApplySuspect
	}
	if !slices.Contains(model.ImpactChoices, c.ImpactAction) {
		return Invalidf("impact_action must be one of %s", strings.Join(model.ImpactChoices, ", "))
	}
	normLinks(&c.Links)
	if len(c.Links.DefectIDs)+len(c.Links.RiskIDs)+len(c.Links.Issues) > 100 {
		return Invalidf("at most 100 linked defects, risks and issues")
	}
	for _, is := range c.Links.Issues {
		if len(is) > 500 {
			return Invalidf("an issue reference is at most 500 characters")
		}
	}
	return nil
}

// checkCRLinks verifies that the linked defects and risks belong to the
// project.
func checkCRLinks(ctx context.Context, c conn, tenantID string, cr *model.ChangeRequest) error {
	for _, l := range []struct {
		table, what string
		ids         []string
	}{{"defects", "defect", cr.Links.DefectIDs}, {"risks", "risk", cr.Links.RiskIDs}} {
		for _, id := range l.ids {
			var pid string
			if err := c.row(ctx, `SELECT project_id FROM `+l.table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
				return mapErr(err, l.what)
			}
			if pid != cr.ProjectID {
				return Invalidf("the %s belongs to another project", l.what)
			}
		}
	}
	return nil
}

// CreateChangeRequest adds a draft change request with a new key
// (PROJ-CR-n) and its items.
func (s *Store) CreateChangeRequest(ctx context.Context, tenantID string, cr *model.ChangeRequest) error {
	if err := validateCR(cr); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, cr.ProjectID); err != nil {
		return err
	}
	cr.ID = newID()
	cr.Status = "draft"
	cr.CreatedAt = s.now()
	cr.UpdatedAt = cr.CreatedAt
	links, _ := json.Marshal(cr.Links)
	return s.tx(ctx, func(c conn) error {
		if err := checkCRLinks(ctx, c, tenantID, cr); err != nil {
			return err
		}
		seq, key, err := nextKey(ctx, c, cr.ProjectID, model.KindChangeRequest)
		if err != nil {
			return err
		}
		cr.Key = key
		if _, err := c.exec(ctx, `INSERT INTO change_requests (id, tenant_id, project_id, seq, key, title, reason, category, priority, status, impact_action, links, baseline_on_close, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, cr.ID, tenantID, cr.ProjectID, seq, cr.Key, cr.Title, cr.Reason, cr.Category, cr.Priority, cr.Status,
			cr.ImpactAction, string(links), cr.BaselineOnClose, cr.CreatedBy, cr.CreatedAt, cr.UpdatedAt); err != nil {
			return mapErr(err, "change request")
		}
		for i := range cr.Items {
			if err := s.putCRItem(ctx, c, tenantID, cr, &cr.Items[i]); err != nil {
				return err
			}
		}
		return addCREvent(ctx, c, cr.ID, "created", cr.CreatedBy, "", cr.CreatedAt)
	})
}

// UpdateChangeRequest saves the editable fields of a draft change request.
func (s *Store) UpdateChangeRequest(ctx context.Context, tenantID string, cr *model.ChangeRequest) error {
	if err := validateCR(cr); err != nil {
		return err
	}
	cr.UpdatedAt = s.now()
	links, _ := json.Marshal(cr.Links)
	return s.tx(ctx, func(c conn) error {
		if err := checkCRLinks(ctx, c, tenantID, cr); err != nil {
			return err
		}
		res, err := c.exec(ctx, `UPDATE change_requests SET title = ?, reason = ?, category = ?, priority = ?, impact_action = ?, links = ?, baseline_on_close = ?, updated_at = ?
			WHERE id = ? AND tenant_id = ? AND status = 'draft'`, cr.Title, cr.Reason, cr.Category, cr.Priority, cr.ImpactAction, string(links), cr.BaselineOnClose, cr.UpdatedAt, cr.ID, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Conflictf("only a draft change request can be changed")
		}
		return nil
	})
}

// itemVersionOf returns the project, version and status of a requirement or test.
func itemVersionOf(ctx context.Context, c conn, tenantID, kind, id string) (project string, version int, status string, err error) {
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	} else if kind != model.KindRequirement {
		return "", 0, "", Invalidf("entity_type must be requirement or test")
	}
	err = c.row(ctx, `SELECT project_id, version, status FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&project, &version, &status)
	return project, version, status, mapErr(err, kind)
}

// CleanProposal checks the proposed fields of an item of kind: known
// fields only, valid JSON, at least one.
func CleanProposal(kind string, in map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	allowed := model.CRProposalFields[kind]
	if allowed == nil {
		return nil, Invalidf("entity_type must be requirement or test")
	}
	out := map[string]json.RawMessage{}
	for k, v := range in {
		if !slices.Contains(allowed, k) {
			return nil, Invalidf("field %q cannot be proposed for a %s (allowed: %s)", k, kind, strings.Join(allowed, ", "))
		}
		v = bytes.TrimSpace(v)
		if !json.Valid(v) {
			return nil, Invalidf("field %q is not valid JSON", k)
		}
		var compact bytes.Buffer
		_ = json.Compact(&compact, v)
		out[k] = compact.Bytes()
	}
	if len(out) == 0 {
		return nil, Invalidf("propose at least one field")
	}
	return out, nil
}

func (s *Store) putCRItem(ctx context.Context, c conn, tenantID string, cr *model.ChangeRequest, it *model.ChangeRequestItem) error {
	prop, err := CleanProposal(it.EntityType, it.Proposed)
	if err != nil {
		return err
	}
	it.Proposed = prop
	pid, version, _, err := itemVersionOf(ctx, c, tenantID, it.EntityType, it.EntityID)
	if err != nil {
		return err
	}
	if pid != cr.ProjectID {
		return Invalidf("%s %s belongs to another project", it.EntityType, it.EntityID)
	}
	if it.BaseVersion <= 0 || it.BaseVersion > version {
		it.BaseVersion = version
	}
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM change_request_items WHERE cr_id = ?`, cr.ID).Scan(&n); err != nil {
		return err
	}
	b, _ := json.Marshal(it.Proposed)
	var existing string
	err = c.row(ctx, `SELECT id FROM change_request_items WHERE cr_id = ? AND entity_type = ? AND entity_id = ?`, cr.ID, it.EntityType, it.EntityID).Scan(&existing)
	if err == nil {
		it.ID = existing
		_, err = c.exec(ctx, `UPDATE change_request_items SET proposed = ?, base_version = ? WHERE id = ?`, string(b), it.BaseVersion, existing)
		return err
	}
	if !db.IsNoRows(err) {
		return err
	}
	if n >= MaxCRItems {
		return Invalidf("a change request has at most %d items", MaxCRItems)
	}
	it.ID = newID()
	_, err = c.exec(ctx, `INSERT INTO change_request_items (id, cr_id, entity_type, entity_id, base_version, proposed, position) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		it.ID, cr.ID, it.EntityType, it.EntityID, it.BaseVersion, string(b), n+1)
	return err
}

// PutChangeRequestItems adds the items to a draft change request, or
// replaces the proposal of an item it already has.
func (s *Store) PutChangeRequestItems(ctx context.Context, tenantID, crID string, items []model.ChangeRequestItem) error {
	cr, err := scanCR(s.c(ctx).row(ctx, `SELECT `+crCols+` FROM change_requests WHERE id = ? AND tenant_id = ?`, crID, tenantID))
	if err != nil {
		return mapErr(err, "change request")
	}
	if !CREditable(cr.Status) {
		return Conflictf("only the items of a draft change request can be changed (it is %s)", cr.Status)
	}
	return s.tx(ctx, func(c conn) error {
		for i := range items {
			if err := s.putCRItem(ctx, c, tenantID, cr, &items[i]); err != nil {
				return err
			}
		}
		_, err := c.exec(ctx, `UPDATE change_requests SET updated_at = ? WHERE id = ?`, s.now(), crID)
		return err
	})
}

// RemoveChangeRequestItem removes an item from a draft change request.
func (s *Store) RemoveChangeRequestItem(ctx context.Context, tenantID, crID, itemID string) error {
	return s.tx(ctx, func(c conn) error {
		var status string
		if err := c.row(ctx, `SELECT status FROM change_requests WHERE id = ? AND tenant_id = ?`, crID, tenantID).Scan(&status); err != nil {
			return mapErr(err, "change request")
		}
		if !CREditable(status) {
			return Conflictf("only the items of a draft change request can be changed (it is %s)", status)
		}
		res, err := c.exec(ctx, `DELETE FROM change_request_items WHERE id = ? AND cr_id = ?`, itemID, crID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "change request item")
		}
		return nil
	})
}

func addCREvent(ctx context.Context, c conn, crID, action, actor, comment string, at time.Time) error {
	_, err := c.exec(ctx, `INSERT INTO change_request_events (id, cr_id, action, actor, comment, at) VALUES (?, ?, ?, ?, ?, ?)`, newID(), crID, action, actor, strings.TrimSpace(comment), at)
	return err
}

// AddChangeRequestEvent records a step of a change request's timeline.
func (s *Store) AddChangeRequestEvent(ctx context.Context, crID, action, actor, comment string) error {
	return addCREvent(ctx, s.c(ctx), crID, action, actor, comment, s.now())
}

// CRTransition is a status change of a change request.
type CRTransition struct {
	From []string
	To   string
	By   string
	// Comment is recorded on the timeline (and as the decision comment of
	// a decision).
	Comment string
	// Event names the timeline step ("" = the new status).
	Event string
	// Impact (with ImpactNotes and Effort) is stored by an assessment.
	Impact      *model.ChangeImpact
	ImpactNotes *string
	Effort      *string
	// BaselineID is stored when closing created a baseline.
	BaselineID string
}

// SetChangeRequestStatus moves a change request from one of t.From to
// t.To and records who and when. ErrConflict when its state is not one of
// From.
func (s *Store) SetChangeRequestStatus(ctx context.Context, tenantID, id string, t CRTransition) error {
	if !slices.Contains(model.CRStatuses, t.To) {
		return Invalidf("unknown status %q", t.To)
	}
	now := s.now()
	args := []any{t.To, now}
	q := `UPDATE change_requests SET status = ?, updated_at = ?`
	switch t.To {
	case "approved", "rejected", "deferred":
		q += `, decided_by = ?, decided_at = ?, decision_comment = ?`
		args = append(args, t.By, now, strings.TrimSpace(t.Comment))
	case "draft":
		q += `, decided_by = '', decided_at = NULL, decision_comment = ''`
	case "implemented":
		q += `, applied_by = ?, applied_at = ?`
		args = append(args, t.By, now)
	case "verified":
		q += `, verified_by = ?, verified_at = ?`
		args = append(args, t.By, now)
	case "closed":
		q += `, closed_by = ?, closed_at = ?, baseline_id = ?`
		args = append(args, t.By, now, t.BaselineID)
	}
	if t.Impact != nil {
		b, _ := json.Marshal(t.Impact)
		q += `, impact = ?, assessed_by = ?, assessed_at = ?`
		args = append(args, string(b), t.By, now)
	}
	if t.ImpactNotes != nil {
		q += `, impact_notes = ?`
		args = append(args, strings.TrimSpace(*t.ImpactNotes))
	}
	if t.Effort != nil {
		q += `, effort = ?`
		args = append(args, strings.TrimSpace(*t.Effort))
	}
	q += ` WHERE id = ? AND tenant_id = ? AND status IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(t.From)), ", ") + `)`
	args = append(args, id, tenantID)
	for _, f := range t.From {
		args = append(args, f)
	}
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, q, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var st string
			if err := c.row(ctx, `SELECT status FROM change_requests WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&st); err != nil {
				return mapErr(err, "change request")
			}
			return Conflictf("the change request is %s, not %s", st, strings.Join(t.From, " or "))
		}
		ev := t.Event
		if ev == "" {
			ev = t.To
		}
		return addCREvent(ctx, c, id, ev, t.By, t.Comment, now)
	})
}

// SetChangeRequestItemApplied records the version applying created.
func (s *Store) SetChangeRequestItemApplied(ctx context.Context, itemID string, version int) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE change_request_items SET applied_version = ? WHERE id = ?`, version, itemID)
	return err
}

// SignChangeRequest records a board member's decision on the change
// request's current content.
func (s *Store) SignChangeRequest(ctx context.Context, tenantID string, cr *model.ChangeRequest, a *model.ChangeRequestSignoff) error {
	if !slices.Contains(model.CRDecisions, a.Decision) {
		return Invalidf("decision must be one of %s", strings.Join(model.CRDecisions, ", "))
	}
	a.Comment = strings.TrimSpace(a.Comment)
	if len([]rune(a.Comment)) > 2000 {
		return Invalidf("the comment is at most 2000 characters")
	}
	if a.Decision != "approve" && len([]rune(a.Comment)) < 3 {
		return Invalidf("a rejection or deferral needs a comment")
	}
	a.ID = newID()
	a.ContentHash = cr.ContentHash
	a.SignedAt = s.now()
	a.Valid = true
	return s.tx(ctx, func(c conn) error {
		_, err := c.exec(ctx, `INSERT INTO change_request_signoffs (id, tenant_id, cr_id, user_id, user_email, user_name, decision, meaning, comment, content_hash, signed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, tenantID, cr.ID, a.UserID, a.UserEmail, a.UserName, a.Decision, a.Meaning, a.Comment, a.ContentHash, a.SignedAt)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return Conflictf("you have already signed this version of the change request")
			}
			return err
		}
		return addCREvent(ctx, c, cr.ID, "signed."+a.Decision, a.UserEmail, a.Comment, a.SignedAt)
	})
}

// CRHash is the hash of the content the board signs.
func CRHash(cr *model.ChangeRequest) string {
	type item struct {
		Kind, ID string
		Base     int
		Proposed map[string]json.RawMessage
	}
	items := make([]item, 0, len(cr.Items))
	for _, it := range cr.Items {
		items = append(items, item{it.EntityType, it.EntityID, it.BaseVersion, it.Proposed})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Kind+items[i].ID < items[j].Kind+items[j].ID })
	b, _ := json.Marshal([]any{cr.ID, cr.Title, cr.Reason, cr.Category, cr.ImpactAction, items})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// GetChangeRequest returns a change request with its items (and their
// current values), signatures and timeline.
func (s *Store) GetChangeRequest(ctx context.Context, tenantID, id string) (*model.ChangeRequest, error) {
	cr, err := scanCR(s.c(ctx).row(ctx, `SELECT `+crCols+` FROM change_requests WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "change request")
	}
	list := []model.ChangeRequest{*cr}
	if err := s.decorateCRs(ctx, tenantID, cr.ProjectID, list, true); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// GetChangeRequestByKey returns a change request of a project by key.
func (s *Store) GetChangeRequestByKey(ctx context.Context, tenantID, projectID, key string) (*model.ChangeRequest, error) {
	var id string
	if err := s.c(ctx).row(ctx, `SELECT id FROM change_requests WHERE tenant_id = ? AND project_id = ? AND (key = ? OR id = ?)`, tenantID, projectID, key, key).Scan(&id); err != nil {
		return nil, mapErr(err, "change request")
	}
	return s.GetChangeRequest(ctx, tenantID, id)
}

// CRFilter narrows ListChangeRequests ("" = any).
type CRFilter struct {
	Status, Category string
	// Open: only open change requests (model.CROpenStatuses).
	Open bool
	// EntityType / EntityID: only those that change the item.
	EntityType, EntityID string
}

// ListChangeRequests lists the change requests of a project, newest first.
func (s *Store) ListChangeRequests(ctx context.Context, tenantID, projectID string, f CRFilter) ([]model.ChangeRequest, error) {
	q := `SELECT ` + crCols + ` FROM change_requests WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.Category != "" {
		q += ` AND category = ?`
		args = append(args, f.Category)
	}
	if f.EntityID != "" {
		q += ` AND id IN (SELECT cr_id FROM change_request_items WHERE entity_type = ? AND entity_id = ?)`
		args = append(args, f.EntityType, f.EntityID)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY seq DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []model.ChangeRequest{}
	for rows.Next() {
		cr, err := scanCR(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if f.Open && !model.CROpen(cr.Status) {
			continue
		}
		out = append(out, *cr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.decorateCRs(ctx, tenantID, projectID, out, false)
}

// itemState is the current state of a requirement or test as JSON fields.
type itemState struct {
	key, title, status string
	version            int
	fields             map[string]json.RawMessage
}

func (s *Store) loadItemState(ctx context.Context, tenantID, kind, id string) (*itemState, error) {
	var v any
	st := &itemState{}
	if kind == model.KindTest {
		t, err := s.GetTest(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		st.key, st.title, st.status, st.version, v = t.Key, t.Title, t.Status, t.Version, t
	} else {
		r, err := s.GetRequirement(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		st.key, st.title, st.status, st.version, v = r.Key, r.Title, r.Status, r.Version, r
	}
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &st.fields)
	return st, nil
}

// decorateCRs fills items, signatures, events, references and the hash.
// full: also the current values of the proposed fields (one item load
// each); otherwise only keys, titles and versions.
func (s *Store) decorateCRs(ctx context.Context, tenantID, projectID string, list []model.ChangeRequest, full bool) error {
	if len(list) == 0 {
		return nil
	}
	c := s.c(ctx)
	idx := map[string]int{}
	for i := range list {
		idx[list[i].ID] = i
		list[i].Items, list[i].Approvals, list[i].Events = []model.ChangeRequestItem{}, []model.ChangeRequestSignoff{}, []model.ChangeRequestEvent{}
		list[i].Defects, list[i].Risks = []model.ItemRefLite{}, []model.ItemRefLite{}
	}
	scope := `SELECT id FROM change_requests WHERE tenant_id = ? AND project_id = ?`
	rows, err := c.query(ctx, `SELECT i.id, i.cr_id, i.entity_type, i.entity_id, i.base_version, i.proposed, i.applied_version,
		COALESCE(r.key, t.key, ''), COALESCE(r.title, t.title, ''), COALESCE(r.status, t.status, ''), COALESCE(r.version, t.version, 0)
		FROM change_request_items i
		LEFT JOIN requirements r ON i.entity_type = 'requirement' AND r.id = i.entity_id
		LEFT JOIN test_cases t ON i.entity_type = 'test' AND t.id = i.entity_id
		WHERE i.cr_id IN (`+scope+`) ORDER BY i.position, i.id`, tenantID, projectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var it model.ChangeRequestItem
		var crID, prop string
		if err := rows.Scan(&it.ID, &crID, &it.EntityType, &it.EntityID, &it.BaseVersion, &prop, &it.AppliedVersion, &it.Key, &it.Title, &it.Status, &it.CurrentVersion); err != nil {
			rows.Close()
			return err
		}
		_ = json.Unmarshal([]byte(prop), &it.Proposed)
		it.Missing = it.Key == ""
		if i, ok := idx[crID]; ok {
			list[i].Items = append(list[i].Items, it)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = c.query(ctx, `SELECT id, cr_id, user_id, user_email, user_name, decision, meaning, comment, content_hash, signed_at FROM change_request_signoffs
		WHERE cr_id IN (`+scope+`) ORDER BY signed_at, id`, tenantID, projectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a model.ChangeRequestSignoff
		var crID string
		if err := rows.Scan(&a.ID, &crID, &a.UserID, &a.UserEmail, &a.UserName, &a.Decision, &a.Meaning, &a.Comment, &a.ContentHash, &a.SignedAt); err != nil {
			rows.Close()
			return err
		}
		if i, ok := idx[crID]; ok {
			list[i].Approvals = append(list[i].Approvals, a)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if full {
		rows, err = c.query(ctx, `SELECT id, cr_id, action, actor, comment, at FROM change_request_events WHERE cr_id IN (`+scope+`) ORDER BY at, id`, tenantID, projectID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var e model.ChangeRequestEvent
			var crID string
			if err := rows.Scan(&e.ID, &crID, &e.Action, &e.Actor, &e.Comment, &e.At); err != nil {
				rows.Close()
				return err
			}
			if i, ok := idx[crID]; ok {
				list[i].Events = append(list[i].Events, e)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	refs := func(q string) (map[string]model.ItemRefLite, error) {
		out := map[string]model.ItemRefLite{}
		rows, err := c.query(ctx, q, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.ItemRefLite
			if err := rows.Scan(&r.ID, &r.Key, &r.Title); err != nil {
				return nil, err
			}
			out[r.ID] = r
		}
		return out, rows.Err()
	}
	var defects, risks map[string]model.ItemRefLite
	needRefs := slices.ContainsFunc(list, func(cr model.ChangeRequest) bool { return len(cr.Links.DefectIDs)+len(cr.Links.RiskIDs) > 0 })
	if needRefs {
		if defects, err = refs(`SELECT id, key, title FROM defects WHERE tenant_id = ? AND project_id = ?`); err != nil {
			return err
		}
		if risks, err = refs(`SELECT id, key, title FROM risks WHERE tenant_id = ? AND project_id = ?`); err != nil {
			return err
		}
	}
	for i := range list {
		cr := &list[i]
		for _, id := range cr.Links.DefectIDs {
			if r, ok := defects[id]; ok {
				cr.Defects = append(cr.Defects, r)
			}
		}
		for _, id := range cr.Links.RiskIDs {
			if r, ok := risks[id]; ok {
				cr.Risks = append(cr.Risks, r)
			}
		}
		cr.ContentHash = CRHash(cr)
		for j := range cr.Approvals {
			cr.Approvals[j].Valid = cr.Approvals[j].ContentHash == cr.ContentHash
		}
		for j := range cr.Items {
			it := &cr.Items[j]
			// applied items moved on by applying, which is not outdated
			it.Outdated = !it.Missing && it.AppliedVersion == 0 && it.CurrentVersion != it.BaseVersion
			if it.Outdated || it.Missing {
				cr.Outdated = cr.Outdated || (model.CROpen(cr.Status) && cr.Status != "draft")
			}
			if !full || it.Missing {
				continue
			}
			st, err := s.loadItemState(ctx, tenantID, it.EntityType, it.EntityID)
			if err != nil {
				continue
			}
			it.Current = map[string]json.RawMessage{}
			for k := range it.Proposed {
				if v, ok := st.fields[k]; ok {
					it.Current[k] = v
				} else {
					it.Current[k] = json.RawMessage("null")
				}
			}
		}
	}
	return nil
}

// DeleteChangeRequest removes a draft, rejected or withdrawn change request.
func (s *Store) DeleteChangeRequest(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		var status string
		if err := c.row(ctx, `SELECT status FROM change_requests WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&status); err != nil {
			return mapErr(err, "change request")
		}
		if status != "draft" && status != "rejected" {
			return Conflictf("only a draft or rejected change request can be deleted (it is %s)", status)
		}
		for _, t := range []string{"change_request_events", "change_request_signoffs", "change_request_items"} {
			if _, err := c.exec(ctx, `DELETE FROM `+t+` WHERE cr_id = ?`, id); err != nil {
				return err
			}
		}
		_, err := c.exec(ctx, `DELETE FROM change_requests WHERE id = ? AND tenant_id = ?`, id, tenantID)
		return err
	})
}

// OpenChangeRequestsByItem returns, for the items of a project with open
// change requests, the keys of those change requests ("kind:id" → keys,
// in key order).
func (s *Store) OpenChangeRequestsByItem(ctx context.Context, tenantID, projectID string) (map[string][]string, error) {
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(model.CROpenStatuses)), ", ")
	args := []any{tenantID, projectID}
	for _, st := range model.CROpenStatuses {
		args = append(args, st)
	}
	rows, err := s.c(ctx).query(ctx, `SELECT i.entity_type, i.entity_id, c.key FROM change_request_items i JOIN change_requests c ON c.id = i.cr_id
		WHERE c.tenant_id = ? AND c.project_id = ? AND c.status IN (`+ph+`) ORDER BY c.seq`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var kind, id, key string
		if err := rows.Scan(&kind, &id, &key); err != nil {
			return nil, err
		}
		k := kind + ":" + id
		if !slices.Contains(out[k], key) {
			out[k] = append(out[k], key)
		}
	}
	return out, rows.Err()
}

// ItemState returns the project, version, status and key of a requirement
// or test.
func (s *Store) ItemState(ctx context.Context, tenantID, kind, id string) (project string, version int, status, key string, err error) {
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	} else if kind != model.KindRequirement {
		return "", 0, "", "", Invalidf("kind must be requirement or test")
	}
	err = s.c(ctx).row(ctx, `SELECT project_id, version, status, key FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&project, &version, &status, &key)
	return project, version, status, key, mapErr(err, kind)
}

// ChangeRequestIDsInStatus returns the change requests of a tenant in
// one of the statuses, oldest update first.
func (s *Store) ChangeRequestIDsInStatus(ctx context.Context, tenantID string, statuses ...string) ([]string, error) {
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(statuses)), ", ")
	args := []any{tenantID}
	for _, st := range statuses {
		args = append(args, st)
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM change_requests WHERE tenant_id = ? AND status IN (`+ph+`) ORDER BY updated_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
