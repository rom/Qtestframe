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

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Waivers and deviations (see model.Waiver): the formal acceptance of
// requirements that are not, or differently, met, with approvals signed
// electronically over a hash of the waiver's content.

const waiverCols = `id, project_id, key, type, title, release_id, justification, compensating, conditions, risk_id, requested_by, approvers,
	status, expires_on, decided_at, decided_by, decision_comment, created_by, created_at, updated_at`

func scanWaiver(r interface{ Scan(...any) error }) (*model.Waiver, error) {
	var w model.Waiver
	var rel, risk sql.NullString
	var approvers string
	var dec sql.NullTime
	if err := r.Scan(&w.ID, &w.ProjectID, &w.Key, &w.Type, &w.Title, &rel, &w.Justification, &w.Compensating, &w.Conditions, &risk, &w.RequestedBy, &approvers,
		&w.Status, &w.ExpiresOn, &dec, &w.DecidedBy, &w.DecisionComment, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	if rel.Valid {
		w.ReleaseID = &rel.String
	}
	if risk.Valid {
		w.RiskID = &risk.String
	}
	_ = json.Unmarshal([]byte(approvers), &w.Approvers)
	if w.Approvers == nil {
		w.Approvers = []string{}
	}
	w.DecidedAt = timePtr(dec)
	return &w, nil
}

// WaiverEditable reports whether a waiver in status may still be edited.
func WaiverEditable(status string) bool { return status == "draft" || status == "requested" }

// WaiverActive reports whether a waiver is in force on day today (YYYY-MM-DD).
func WaiverActive(w *model.Waiver, today string) bool {
	return w.Status == "approved" && (w.ExpiresOn == "" || w.ExpiresOn >= today)
}

// WaiverHash is the hash of the content an approval signs.
func WaiverHash(w *model.Waiver) string {
	reqs := append([]string{}, w.RequirementIDs...)
	sort.Strings(reqs)
	rel, risk := "", ""
	if w.ReleaseID != nil {
		rel = *w.ReleaseID
	}
	if w.RiskID != nil {
		risk = *w.RiskID
	}
	b, _ := json.Marshal([]any{w.ID, w.Type, w.Title, rel, reqs, w.Justification, w.Compensating, w.Conditions, risk, w.ExpiresOn, w.Approvers})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func normalizeEmails(in []string) []string {
	out := []string{}
	for _, e := range in {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

// validateWaiver checks and normalises the editable fields.
func validateWaiver(w *model.Waiver) error {
	if w.Type == "" {
		w.Type = "waiver"
	}
	if !slices.Contains(model.WaiverTypes, w.Type) {
		return Invalidf("type must be one of %s", strings.Join(model.WaiverTypes, ", "))
	}
	w.Title = strings.TrimSpace(w.Title)
	if w.Title == "" || len([]rune(w.Title)) > 300 {
		return Invalidf("waiver title is required (at most 300 characters)")
	}
	w.Justification, w.Compensating, w.Conditions = strings.TrimSpace(w.Justification), strings.TrimSpace(w.Compensating), strings.TrimSpace(w.Conditions)
	for _, f := range []string{w.Justification, w.Compensating, w.Conditions} {
		if len([]rune(f)) > 8000 {
			return Invalidf("justification, compensating measures and conditions are at most 8000 characters each")
		}
	}
	w.ExpiresOn = strings.TrimSpace(w.ExpiresOn)
	if w.ExpiresOn != "" && !dateRe.MatchString(w.ExpiresOn) {
		return Invalidf("expires_on must be YYYY-MM-DD")
	}
	w.Approvers = normalizeEmails(w.Approvers)
	if len(w.Approvers) > 20 {
		return Invalidf("at most 20 approvers")
	}
	for _, e := range w.Approvers {
		if !strings.Contains(e, "@") || len(e) > 320 {
			return Invalidf("approver %q is not an e-mail address", e)
		}
	}
	if w.ReleaseID != nil && *w.ReleaseID == "" {
		w.ReleaseID = nil
	}
	if w.RiskID != nil && *w.RiskID == "" {
		w.RiskID = nil
	}
	ids := []string{}
	for _, id := range w.RequirementIDs {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	w.RequirementIDs = ids
	return nil
}

// ValidateWaiverForRequest checks what a waiver needs before it is
// requested: requirements, a justification and, for a deviation, the
// solution chosen instead (in the compensating measures).
func ValidateWaiverForRequest(w *model.Waiver) error {
	if len(w.RequirementIDs) == 0 {
		return Invalidf("a waiver needs at least one requirement")
	}
	if len([]rune(w.Justification)) < 3 {
		return Invalidf("a waiver needs a justification")
	}
	if w.Type == "deviation" && len([]rune(w.Compensating)) < 3 {
		return Invalidf("a deviation needs the solution chosen instead (compensating measures)")
	}
	return nil
}

// checkWaiverRefs verifies that the release, risk and requirements belong
// to the project.
func checkWaiverRefs(ctx context.Context, c conn, tenantID string, w *model.Waiver) error {
	if err := checkRelease(ctx, c, tenantID, w.ProjectID, w.ReleaseID); err != nil {
		return err
	}
	if w.RiskID != nil {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM risks WHERE id = ? AND tenant_id = ?`, *w.RiskID, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "risk")
		}
		if pid != w.ProjectID {
			return Invalidf("the risk belongs to another project")
		}
	}
	for _, id := range w.RequirementIDs {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "requirement")
		}
		if pid != w.ProjectID {
			return Invalidf("requirement %s belongs to another project", id)
		}
	}
	return nil
}

func setWaiverRequirements(ctx context.Context, c conn, w *model.Waiver) error {
	if _, err := c.exec(ctx, `DELETE FROM waiver_requirements WHERE waiver_id = ?`, w.ID); err != nil {
		return err
	}
	for _, id := range w.RequirementIDs {
		if _, err := c.exec(ctx, `INSERT INTO waiver_requirements (waiver_id, requirement_id) VALUES (?, ?)`, w.ID, id); err != nil {
			return err
		}
	}
	return nil
}

// CreateWaiver adds a waiver with a new key (PROJ-WVR-n).
func (s *Store) CreateWaiver(ctx context.Context, tenantID string, w *model.Waiver) error {
	if err := validateWaiver(w); err != nil {
		return err
	}
	if w.Status == "" {
		w.Status = "draft"
	}
	if w.Status != "draft" && w.Status != "requested" {
		return Invalidf("a new waiver is a draft or requested")
	}
	if w.Status == "requested" {
		if err := ValidateWaiverForRequest(w); err != nil {
			return err
		}
	}
	if _, err := s.GetProject(ctx, tenantID, w.ProjectID); err != nil {
		return err
	}
	w.ID = newID()
	w.CreatedAt = s.now()
	w.UpdatedAt = w.CreatedAt
	approvers, _ := json.Marshal(w.Approvers)
	return s.tx(ctx, func(c conn) error {
		if err := checkWaiverRefs(ctx, c, tenantID, w); err != nil {
			return err
		}
		seq, key, err := nextKey(ctx, c, w.ProjectID, model.KindWaiver)
		if err != nil {
			return err
		}
		w.Key = key
		if _, err := c.exec(ctx, `INSERT INTO waivers (id, tenant_id, project_id, seq, key, type, title, release_id, justification, compensating, conditions, risk_id, requested_by, approvers,
			status, expires_on, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			w.ID, tenantID, w.ProjectID, seq, w.Key, w.Type, w.Title, w.ReleaseID, w.Justification, w.Compensating, w.Conditions, w.RiskID, w.RequestedBy, string(approvers),
			w.Status, w.ExpiresOn, w.CreatedBy, w.CreatedAt, w.UpdatedAt); err != nil {
			return mapErr(err, "waiver")
		}
		return setWaiverRequirements(ctx, c, w)
	})
}

// UpdateWaiver saves the editable fields of a draft or requested waiver.
// Approvals of earlier content stop counting (their hash no longer
// matches).
func (s *Store) UpdateWaiver(ctx context.Context, tenantID string, w *model.Waiver) error {
	if err := validateWaiver(w); err != nil {
		return err
	}
	if !WaiverEditable(w.Status) {
		return Conflictf("only a draft or requested waiver can be changed (it is %s)", w.Status)
	}
	if w.Status == "requested" {
		if err := ValidateWaiverForRequest(w); err != nil {
			return err
		}
	}
	w.UpdatedAt = s.now()
	approvers, _ := json.Marshal(w.Approvers)
	return s.tx(ctx, func(c conn) error {
		if err := checkWaiverRefs(ctx, c, tenantID, w); err != nil {
			return err
		}
		res, err := c.exec(ctx, `UPDATE waivers SET type = ?, title = ?, release_id = ?, justification = ?, compensating = ?, conditions = ?, risk_id = ?, approvers = ?,
			expires_on = ?, updated_at = ? WHERE id = ? AND tenant_id = ? AND status IN ('draft', 'requested')`,
			w.Type, w.Title, w.ReleaseID, w.Justification, w.Compensating, w.Conditions, w.RiskID, string(approvers), w.ExpiresOn, w.UpdatedAt, w.ID, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Conflictf("the waiver was decided in the meantime")
		}
		return setWaiverRequirements(ctx, c, w)
	})
}

// SetWaiverStatus moves a waiver from one of the from states to to,
// recording who decided and why for approved, rejected and revoked.
// ErrConflict when its state is not one of from.
func (s *Store) SetWaiverStatus(ctx context.Context, tenantID, id string, from []string, to, by, comment string) error {
	if !slices.Contains(model.WaiverStatuses, to) {
		return Invalidf("unknown status %q", to)
	}
	now := s.now()
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(from)), ", ")
	args := []any{to, now}
	q := `UPDATE waivers SET status = ?, updated_at = ?`
	switch to {
	case "approved", "rejected", "revoked":
		q += `, decided_at = ?, decided_by = ?, decision_comment = ?`
		args = append(args, now, by, strings.TrimSpace(comment))
	case "draft", "requested":
		q += `, decided_at = NULL, decided_by = '', decision_comment = ''`
	}
	q += ` WHERE id = ? AND tenant_id = ? AND status IN (` + ph + `)`
	args = append(args, id, tenantID)
	for _, f := range from {
		args = append(args, f)
	}
	res, err := s.c(ctx).exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetWaiver(ctx, tenantID, id); err != nil {
			return err
		}
		return Conflictf("the waiver is not %s", strings.Join(from, " or "))
	}
	return nil
}

// SignWaiver records an approver's decision on the waiver's current content.
func (s *Store) SignWaiver(ctx context.Context, tenantID string, a *model.WaiverApproval) error {
	if !slices.Contains(model.WaiverDecisions, a.Decision) {
		return Invalidf("decision must be one of %s", strings.Join(model.WaiverDecisions, ", "))
	}
	a.Comment = strings.TrimSpace(a.Comment)
	if len([]rune(a.Comment)) > 2000 {
		return Invalidf("the comment is at most 2000 characters")
	}
	if a.Decision == "reject" && len([]rune(a.Comment)) < 3 {
		return Invalidf("a rejection needs a comment")
	}
	w, err := s.GetWaiver(ctx, tenantID, a.WaiverID)
	if err != nil {
		return err
	}
	a.ID = newID()
	a.ContentHash = w.ContentHash
	a.SignedAt = s.now()
	a.Valid = true
	_, err = s.c(ctx).exec(ctx, `INSERT INTO waiver_signoffs (id, tenant_id, waiver_id, user_id, user_email, user_name, decision, comment, content_hash, signed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, tenantID, a.WaiverID, a.UserID, a.UserEmail, a.UserName, a.Decision, a.Comment, a.ContentHash, a.SignedAt)
	if err != nil && db.IsUniqueViolation(err) {
		return Conflictf("you have already signed this version of the waiver")
	}
	return err
}

// GetWaiver returns a waiver with its requirements and approvals.
func (s *Store) GetWaiver(ctx context.Context, tenantID, id string) (*model.Waiver, error) {
	w, err := scanWaiver(s.c(ctx).row(ctx, `SELECT `+waiverCols+` FROM waivers WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "waiver")
	}
	list := []model.Waiver{*w}
	if err := s.decorateWaivers(ctx, tenantID, w.ProjectID, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// WaiverFilter narrows ListWaivers ("" = any).
type WaiverFilter struct {
	Status, Type, ReleaseID, RequirementID string
	// ActiveOnly: approved waivers that have not expired.
	ActiveOnly bool
}

// ListWaivers lists the waivers of a project, newest first.
func (s *Store) ListWaivers(ctx context.Context, tenantID, projectID string, f WaiverFilter) ([]model.Waiver, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+waiverCols+` FROM waivers WHERE tenant_id = ? AND project_id = ? ORDER BY seq DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	all := []model.Waiver{}
	for rows.Next() {
		w, err := scanWaiver(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, *w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.decorateWaivers(ctx, tenantID, projectID, all); err != nil {
		return nil, err
	}
	out := []model.Waiver{}
	for _, w := range all {
		switch {
		case f.Status != "" && w.Status != f.Status,
			f.Type != "" && w.Type != f.Type,
			f.ReleaseID == "project" && w.ReleaseID != nil,
			f.ReleaseID != "" && f.ReleaseID != "project" && (w.ReleaseID == nil || *w.ReleaseID != f.ReleaseID),
			f.RequirementID != "" && !slices.Contains(w.RequirementIDs, f.RequirementID),
			f.ActiveOnly && !w.Active:
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

// decorateWaivers fills requirements, approvals, names and the hash.
func (s *Store) decorateWaivers(ctx context.Context, tenantID, projectID string, list []model.Waiver) error {
	if len(list) == 0 {
		return nil
	}
	c := s.c(ctx)
	rows, err := c.query(ctx, `SELECT wr.waiver_id, q.id, q.key, q.title FROM waiver_requirements wr JOIN waivers w ON w.id = wr.waiver_id JOIN requirements q ON q.id = wr.requirement_id
		WHERE w.project_id = ? AND w.tenant_id = ? ORDER BY q.seq`, projectID, tenantID)
	if err != nil {
		return err
	}
	reqs := map[string][]model.ItemRefLite{}
	for rows.Next() {
		var wid string
		var it model.ItemRefLite
		if err := rows.Scan(&wid, &it.ID, &it.Key, &it.Title); err != nil {
			rows.Close()
			return err
		}
		reqs[wid] = append(reqs[wid], it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = c.query(ctx, `SELECT a.id, a.waiver_id, a.user_id, a.user_email, a.user_name, a.decision, a.comment, a.content_hash, a.signed_at FROM waiver_signoffs a
		JOIN waivers w ON w.id = a.waiver_id WHERE w.project_id = ? AND w.tenant_id = ? ORDER BY a.signed_at, a.id`, projectID, tenantID)
	if err != nil {
		return err
	}
	approvals := map[string][]model.WaiverApproval{}
	for rows.Next() {
		var a model.WaiverApproval
		if err := rows.Scan(&a.ID, &a.WaiverID, &a.UserID, &a.UserEmail, &a.UserName, &a.Decision, &a.Comment, &a.ContentHash, &a.SignedAt); err != nil {
			rows.Close()
			return err
		}
		approvals[a.WaiverID] = append(approvals[a.WaiverID], a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	names, err := s.ReleaseNames(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	riskKeys := map[string]string{}
	rows, err = c.query(ctx, `SELECT id, key FROM risks WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return err
		}
		riskKeys[id] = key
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	today := s.today()
	for i := range list {
		w := &list[i]
		w.Requirements = reqs[w.ID]
		if w.Requirements == nil {
			w.Requirements = []model.ItemRefLite{}
		}
		w.RequirementIDs = make([]string, len(w.Requirements))
		for j, r := range w.Requirements {
			w.RequirementIDs[j] = r.ID
		}
		w.ContentHash = WaiverHash(w)
		w.Approvals = approvals[w.ID]
		if w.Approvals == nil {
			w.Approvals = []model.WaiverApproval{}
		}
		for j := range w.Approvals {
			w.Approvals[j].Valid = w.Approvals[j].ContentHash == w.ContentHash
		}
		if w.ReleaseID != nil {
			w.ReleaseName = names[*w.ReleaseID]
		}
		if w.RiskID != nil {
			w.RiskKey = riskKeys[*w.RiskID]
		}
		w.Active = WaiverActive(w, today)
	}
	return nil
}

// ApprovalOutcome decides a requested waiver from its valid approvals:
// "rejected" when a counted approver rejected it, "approved" when every
// designated approver (or, without designated approvers, anyone) approved
// it, else "".
func ApprovalOutcome(w *model.Waiver) string {
	approved := map[string]bool{}
	for _, a := range w.Approvals {
		if !a.Valid {
			continue
		}
		email := strings.ToLower(a.UserEmail)
		if len(w.Approvers) > 0 && !slices.Contains(w.Approvers, email) {
			continue
		}
		if a.Decision == "reject" {
			return "rejected"
		}
		approved[email] = true
	}
	if len(w.Approvers) == 0 {
		if len(approved) > 0 {
			return "approved"
		}
		return ""
	}
	for _, e := range w.Approvers {
		if !approved[e] {
			return ""
		}
	}
	return "approved"
}

// DeleteWaiver removes a waiver that is not in force (approved waivers are
// revoked first, so the decision stays on record).
func (s *Store) DeleteWaiver(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		var status string
		if err := c.row(ctx, `SELECT status FROM waivers WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&status); err != nil {
			return mapErr(err, "waiver")
		}
		if status == "approved" {
			return Conflictf("revoke an approved waiver before deleting it")
		}
		if _, err := c.exec(ctx, `DELETE FROM waiver_signoffs WHERE waiver_id = ?`, id); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM waiver_requirements WHERE waiver_id = ?`, id); err != nil {
			return err
		}
		_, err := c.exec(ctx, `DELETE FROM waivers WHERE id = ? AND tenant_id = ?`, id, tenantID)
		return err
	})
}

// ExpiredWaiver is a waiver ExpireWaivers marked expired.
type ExpiredWaiver struct {
	TenantID string
	model.Waiver
}

// ExpireWaivers marks the approved waivers of every tenant whose expiry date
// has passed (before today) as expired and returns them. Each is returned by
// exactly one call, also with several servers.
func (s *Store) ExpireWaivers(ctx context.Context) ([]ExpiredWaiver, error) {
	today := s.today()
	rows, err := s.c(ctx).query(ctx, `SELECT tenant_id, id FROM waivers WHERE status = 'approved' AND expires_on <> '' AND expires_on < ?`, today)
	if err != nil {
		return nil, err
	}
	type ref struct{ tenant, id string }
	var due []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.tenant, &r.id); err != nil {
			rows.Close()
			return nil, err
		}
		due = append(due, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ExpiredWaiver
	for _, r := range due {
		res, err := s.c(ctx).exec(ctx, `UPDATE waivers SET status = 'expired', updated_at = ? WHERE id = ? AND status = 'approved'`, s.now(), r.id)
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		if w, err := s.GetWaiver(ctx, r.tenant, r.id); err == nil {
			out = append(out, ExpiredWaiver{TenantID: r.tenant, Waiver: *w})
		}
	}
	return out, nil
}

// ActiveWaivers returns the waivers in force today for the requirements of
// a project, by requirement id: approved, not expired, for the release
// (releaseID "" = only project-wide waivers) or for the whole project.
func (s *Store) ActiveWaivers(ctx context.Context, tenantID, projectID, releaseID string) (map[string][]model.Waiver, error) {
	list, err := s.ListWaivers(ctx, tenantID, projectID, WaiverFilter{ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	out := map[string][]model.Waiver{}
	for _, w := range list {
		if w.ReleaseID != nil && *w.ReleaseID != releaseID {
			continue
		}
		for _, id := range w.RequirementIDs {
			out[id] = append(out[id], w)
		}
	}
	return out, nil
}
