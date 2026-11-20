package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- notification mails

// MailNotification is a notification with its tenant, for e-mails.
type MailNotification struct {
	model.Notification
	TenantID string
}

// MarkNotificationsEmailed records that notifications were e-mailed.
func (s *Store) MarkNotificationsEmailed(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.c(ctx).exec(ctx, `UPDATE notifications SET emailed_at = ? WHERE id IN (`+placeholders(len(ids))+`)`, append([]any{s.now()}, anySlice(ids)...)...)
	return err
}

// DigestNotifications returns a user's unread notifications (all tenants)
// that were not e-mailed yet, oldest first (at most limit).
func (s *Store) DigestNotifications(ctx context.Context, userID string, limit int) ([]MailNotification, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, tenant_id, user_id, project_id, event, title, body, entity_type, entity_id, actor, created_at FROM notifications
		WHERE user_id = ? AND read_at IS NULL AND emailed_at IS NULL ORDER BY created_at, id LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MailNotification
	for rows.Next() {
		var n MailNotification
		if err := rows.Scan(&n.ID, &n.TenantID, &n.UserID, &n.ProjectID, &n.Event, &n.Title, &n.Body, &n.EntityType, &n.EntityID, &n.Actor, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UsersWithEmailMode returns the enabled users whose e-mail notification
// preference is mode.
func (s *Store) UsersWithEmailMode(ctx context.Context, mode string) ([]model.User, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+userCols+` FROM users WHERE disabled = ? AND preferences LIKE ? ORDER BY email`, false, `%"email_notifications":"`+mode+`"%`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		if u.Preferences.EmailNotifications == mode {
			out = append(out, *u)
		}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- jobs

// JobLastRun returns when the job last ran (nil = never).
func (s *Store) JobLastRun(ctx context.Context, name string) (*time.Time, error) {
	var t time.Time
	err := s.c(ctx).row(ctx, `SELECT last_run FROM job_runs WHERE name = ?`, name).Scan(&t)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}

// SetJobLastRun records when the job ran.
func (s *Store) SetJobLastRun(ctx context.Context, name string, at time.Time) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE job_runs SET last_run = ? WHERE name = ?`, at.UTC(), name)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
		_, err = c.exec(ctx, `INSERT INTO job_runs (name, last_run) VALUES (?, ?)`, name, at.UTC())
		return err
	})
}

// ---------------------------------------------------------------- invitations

const invitationCols = `id, tenant_id, user_id, email, role, token_hash, sso, invited_by, created_at, sent_at, expires_at, accepted_at, revoked_at`

func scanInvitation(r interface{ Scan(...any) error }) (*model.Invitation, error) {
	var i model.Invitation
	var acc, rev sql.NullTime
	if err := r.Scan(&i.ID, &i.TenantID, &i.UserID, &i.Email, &i.Role, &i.TokenHash, &i.SSO, &i.InvitedBy, &i.CreatedAt, &i.SentAt, &i.ExpiresAt, &acc, &rev); err != nil {
		return nil, err
	}
	i.CreatedAt, i.SentAt, i.ExpiresAt = i.CreatedAt.UTC(), i.SentAt.UTC(), i.ExpiresAt.UTC()
	i.AcceptedAt, i.RevokedAt = timePtr(acc), timePtr(rev)
	return &i, nil
}

// Now returns the store's current time (tests can set the clock).
func (s *Store) Now() time.Time { return s.now() }

// CreateInvitation stores an invitation; earlier open invitations of the
// same user in the tenant are revoked.
func (s *Store) CreateInvitation(ctx context.Context, i *model.Invitation) error {
	return s.tx(ctx, func(c conn) error {
		now := s.now()
		if _, err := c.exec(ctx, `UPDATE invitations SET revoked_at = ?, token_hash = '' WHERE tenant_id = ? AND user_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, now, i.TenantID, i.UserID); err != nil {
			return err
		}
		i.ID, i.CreatedAt, i.SentAt = newID(), now, now
		i.Email = NormalizeEmail(i.Email)
		_, err := c.exec(ctx, `INSERT INTO invitations (`+invitationCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			i.ID, i.TenantID, i.UserID, i.Email, i.Role, i.TokenHash, i.SSO, i.InvitedBy, i.CreatedAt, i.SentAt, i.ExpiresAt.UTC(), nil, nil)
		return mapErr(err, "invitation")
	})
}

// GetInvitationByToken returns the invitation with the token hash.
func (s *Store) GetInvitationByToken(ctx context.Context, hash string) (*model.Invitation, error) {
	if hash == "" {
		return nil, mapErr(sql.ErrNoRows, "invitation")
	}
	i, err := scanInvitation(s.c(ctx).row(ctx, `SELECT `+invitationCols+` FROM invitations WHERE token_hash = ?`, hash))
	return i, mapErr(err, "invitation")
}

// OpenInvitation returns the newest invitation of a user in a tenant that
// was neither accepted nor revoked (it may have expired).
func (s *Store) OpenInvitation(ctx context.Context, tenantID, userID string) (*model.Invitation, error) {
	i, err := scanInvitation(s.c(ctx).row(ctx, `SELECT `+invitationCols+` FROM invitations WHERE tenant_id = ? AND user_id = ? AND accepted_at IS NULL AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, tenantID, userID))
	return i, mapErr(err, "invitation")
}

// OpenInvitations returns the open invitations of a tenant by user ID.
func (s *Store) OpenInvitations(ctx context.Context, tenantID string) (map[string]model.Invitation, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+invitationCols+` FROM invitations WHERE tenant_id = ? AND accepted_at IS NULL AND revoked_at IS NULL ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.Invitation{}
	for rows.Next() {
		i, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out[i.UserID] = *i
	}
	return out, rows.Err()
}

// RenewInvitation stores a new token and expiry (resend).
func (s *Store) RenewInvitation(ctx context.Context, id, hash string, expires time.Time) error {
	res, err := s.c(ctx).exec(ctx, `UPDATE invitations SET token_hash = ?, sent_at = ?, expires_at = ? WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, hash, s.now(), expires.UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "invitation")
	}
	return nil
}

// CloseInvitation marks an open invitation accepted or revoked and clears
// its token, so the link cannot be used again.
func (s *Store) CloseInvitation(ctx context.Context, id string, accepted bool) error {
	col := "revoked_at"
	if accepted {
		col = "accepted_at"
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE invitations SET `+col+` = ?, token_hash = '' WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, s.now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Conflictf("the invitation was already used or revoked")
	}
	return nil
}

// AcceptUserInvitations marks all open invitations of a user accepted
// (single sign-on users: on their first sign-in) and returns how many.
func (s *Store) AcceptUserInvitations(ctx context.Context, userID string) (int, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE invitations SET accepted_at = ?, token_hash = '' WHERE user_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, s.now(), userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------- report subscriptions

const subscriptionCols = `s.id, s.project_id, s.definition_id, d.name, s.owner_id, u.email, s.frequency, s.weekday, s.day_of_month, s.time_of_day, s.timezone, s.format,
	s.recipients, s.addresses, s.message, s.enabled, s.next_run_at, s.last_run_at, s.last_sent_at, s.last_error, s.created_at, s.updated_at`

const subscriptionFrom = ` FROM report_subscriptions s JOIN report_definitions d ON d.id = s.definition_id JOIN users u ON u.id = s.owner_id `

func scanSubscription(r interface{ Scan(...any) error }) (*model.ReportSubscription, error) {
	var x model.ReportSubscription
	var rec, addr string
	var next, last, sent sql.NullTime
	if err := r.Scan(&x.ID, &x.ProjectID, &x.DefinitionID, &x.DefinitionName, &x.OwnerID, &x.OwnerEmail, &x.Frequency, &x.Weekday, &x.DayOfMonth, &x.TimeOfDay, &x.Timezone, &x.Format,
		&rec, &addr, &x.Message, &x.Enabled, &next, &last, &sent, &x.LastError, &x.CreatedAt, &x.UpdatedAt); err != nil {
		return nil, err
	}
	x.Recipients, x.Addresses = decodeTags(rec), decodeTags(addr)
	x.NextRunAt, x.LastRunAt, x.LastSentAt = timePtr(next), timePtr(last), timePtr(sent)
	x.CreatedAt, x.UpdatedAt = x.CreatedAt.UTC(), x.UpdatedAt.UTC()
	return &x, nil
}

func encodeList(l []string) string {
	if l == nil {
		l = []string{}
	}
	b, _ := json.Marshal(l)
	return string(b)
}

// validateSubscription checks the schedule and computes the next run
// (from now, unless the caller set x.NextRunAt).
func (s *Store) validateSubscription(x *model.ReportSubscription) error {
	if !slices.Contains(model.Frequencies, x.Frequency) {
		return Invalidf("frequency must be daily, weekly or monthly")
	}
	if x.Timezone == "" {
		x.Timezone = "UTC"
	}
	if len(x.Message) > 4000 {
		return Invalidf("message is too long (at most 4000 characters)")
	}
	x.Message = strings.TrimSpace(x.Message)
	if len(x.Recipients)+len(x.Addresses) == 0 {
		return Invalidf("choose at least one recipient")
	}
	if len(x.Recipients)+len(x.Addresses) > 100 {
		return Invalidf("at most 100 recipients")
	}
	next, err := x.NextRun(s.now())
	if err != nil {
		return Invalidf("%v", err)
	}
	switch {
	case !x.Enabled:
		x.NextRunAt = nil
	case x.NextRunAt == nil:
		x.NextRunAt = &next
	}
	return nil
}

// CreateReportSubscription stores a subscription of a saved report.
func (s *Store) CreateReportSubscription(ctx context.Context, tenantID string, x *model.ReportSubscription) error {
	d, err := s.GetReportDefinition(ctx, tenantID, x.DefinitionID)
	if err != nil {
		return err
	}
	if err := s.validateSubscription(x); err != nil {
		return err
	}
	x.ProjectID, x.DefinitionName = d.ProjectID, d.Name
	x.ID, x.CreatedAt = newID(), s.now()
	x.UpdatedAt = x.CreatedAt
	_, err = s.c(ctx).exec(ctx, `INSERT INTO report_subscriptions (id, tenant_id, project_id, definition_id, owner_id, frequency, weekday, day_of_month, time_of_day, timezone, format,
		recipients, addresses, message, enabled, next_run_at, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		x.ID, tenantID, x.ProjectID, x.DefinitionID, x.OwnerID, x.Frequency, x.Weekday, x.DayOfMonth, x.TimeOfDay, x.Timezone, x.Format,
		encodeList(x.Recipients), encodeList(x.Addresses), x.Message, x.Enabled, nullTime(x.NextRunAt), x.CreatedAt, x.UpdatedAt)
	return mapErr(err, "report subscription")
}

// GetReportSubscription returns a subscription.
func (s *Store) GetReportSubscription(ctx context.Context, tenantID, id string) (*model.ReportSubscription, error) {
	x, err := scanSubscription(s.c(ctx).row(ctx, `SELECT `+subscriptionCols+subscriptionFrom+`WHERE s.id = ? AND s.tenant_id = ?`, id, tenantID))
	return x, mapErr(err, "report subscription")
}

// ListReportSubscriptions returns the subscriptions of a project.
func (s *Store) ListReportSubscriptions(ctx context.Context, tenantID, projectID string) ([]model.ReportSubscription, error) {
	return s.listSubscriptions(ctx, `WHERE s.tenant_id = ? AND s.project_id = ? ORDER BY d.name, s.created_at`, tenantID, projectID)
}

func (s *Store) listSubscriptions(ctx context.Context, where string, args ...any) ([]model.ReportSubscription, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+subscriptionCols+`, s.tenant_id`+subscriptionFrom+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ReportSubscription{}
	for rows.Next() {
		var tid string
		x, err := scanSubscription(scanAppend{rows, &tid})
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// scanAppend scans extra trailing columns after the ones a scan function reads.
type scanAppend struct {
	r     interface{ Scan(...any) error }
	extra *string
}

func (a scanAppend) Scan(dest ...any) error { return a.r.Scan(append(dest, a.extra)...) }

// DueReportSubscription is a subscription whose run is due, with its tenant.
type DueReportSubscription struct {
	model.ReportSubscription
	TenantID string
}

// DueReportSubscriptions returns enabled subscriptions whose next run is
// not after now.
func (s *Store) DueReportSubscriptions(ctx context.Context, now time.Time) ([]DueReportSubscription, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+subscriptionCols+`, s.tenant_id`+subscriptionFrom+`WHERE s.enabled = ? AND s.next_run_at IS NOT NULL AND s.next_run_at <= ? ORDER BY s.next_run_at`, true, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueReportSubscription
	for rows.Next() {
		var d DueReportSubscription
		x, err := scanSubscription(scanAppend{rows, &d.TenantID})
		if err != nil {
			return nil, err
		}
		d.ReportSubscription = *x
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateReportSubscription stores the schedule, recipients and message
// (the next run is computed again).
func (s *Store) UpdateReportSubscription(ctx context.Context, tenantID string, x *model.ReportSubscription) error {
	if err := s.validateSubscription(x); err != nil {
		return err
	}
	x.UpdatedAt = s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE report_subscriptions SET frequency = ?, weekday = ?, day_of_month = ?, time_of_day = ?, timezone = ?, format = ?, recipients = ?, addresses = ?,
		message = ?, enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		x.Frequency, x.Weekday, x.DayOfMonth, x.TimeOfDay, x.Timezone, x.Format, encodeList(x.Recipients), encodeList(x.Addresses),
		x.Message, x.Enabled, nullTime(x.NextRunAt), x.UpdatedAt, x.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "report subscription")
	}
	return nil
}

// RecordReportRun stores the outcome of a run: when it ran, whether it was
// sent (errMsg empty) and the next run (nil = none).
func (s *Store) RecordReportRun(ctx context.Context, id string, at time.Time, errMsg string, next *time.Time) error {
	q := `UPDATE report_subscriptions SET last_run_at = ?, last_error = ?, next_run_at = ?`
	args := []any{at.UTC(), truncate(errMsg, 1000), nullTime(next)}
	if errMsg == "" {
		q += `, last_sent_at = ?`
		args = append(args, at.UTC())
	}
	_, err := s.c(ctx).exec(ctx, q+` WHERE id = ?`, append(args, id)...)
	return err
}

// StartReportRun records that a scheduled run started at and sets the next
// run (before the report is sent, so that a crash does not send it twice).
func (s *Store) StartReportRun(ctx context.Context, id string, at time.Time, next *time.Time) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE report_subscriptions SET last_run_at = ?, next_run_at = ? WHERE id = ?`, at.UTC(), nullTime(next), id)
	return err
}

// DeleteReportSubscription removes a subscription.
func (s *Store) DeleteReportSubscription(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM report_subscriptions WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "report subscription")
	}
	return nil
}
