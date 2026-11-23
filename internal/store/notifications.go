package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- notifications

// AddNotifications stores notifications (IDs and times are filled in).
func (s *Store) AddNotifications(ctx context.Context, tenantID string, list []model.Notification) error {
	if len(list) == 0 {
		return nil
	}
	err := s.tx(ctx, func(c conn) error {
		for i := range list {
			n := &list[i]
			n.ID = newID()
			n.CreatedAt = s.now()
			if len([]rune(n.Title)) > 300 {
				n.Title = string([]rune(n.Title)[:300])
			}
			if len([]rune(n.Body)) > 2000 {
				n.Body = string([]rune(n.Body)[:2000])
			}
			if _, err := c.exec(ctx, `INSERT INTO notifications (id, tenant_id, user_id, project_id, event, title, body, entity_type, entity_id, actor, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				n.ID, tenantID, n.UserID, n.ProjectID, n.Event, n.Title, n.Body, n.EntityType, n.EntityID, n.Actor, n.CreatedAt); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && s.notified != nil {
		users := make([]string, 0, len(list))
		seen := map[string]bool{}
		for _, n := range list {
			if !seen[n.UserID] {
				seen[n.UserID] = true
				users = append(users, n.UserID)
			}
		}
		AfterCommit(ctx, func() { s.notified(tenantID, users) })
	}
	return err
}

// ListNotifications returns a user's newest notifications and the number
// that are unread.
// Notifications of the hidden projects are left out (and not counted).
func (s *Store) ListNotifications(ctx context.Context, tenantID, userID string, unreadOnly bool, limit int, hidden ...string) ([]model.Notification, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	base := `WHERE tenant_id = ? AND user_id = ?`
	args := []any{tenantID, userID}
	if len(hidden) > 0 {
		base += ` AND project_id NOT IN (` + placeholders(len(hidden)) + `)`
		args = append(args, anySlice(hidden)...)
	}
	where := base
	if unreadOnly {
		where += ` AND read_at IS NULL`
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id, user_id, project_id, event, title, body, entity_type, entity_id, actor, read_at, created_at FROM notifications `+where+` ORDER BY created_at DESC, id LIMIT ?`, append(append([]any{}, args...), limit)...)
	if err != nil {
		return nil, 0, err
	}
	out := []model.Notification{}
	for rows.Next() {
		var n model.Notification
		var at sql.NullTime
		if err := rows.Scan(&n.ID, &n.UserID, &n.ProjectID, &n.Event, &n.Title, &n.Body, &n.EntityType, &n.EntityID, &n.Actor, &at, &n.CreatedAt); err != nil {
			rows.Close()
			return nil, 0, err
		}
		n.ReadAt = timePtr(at)
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var unread int
	err = s.c(ctx).row(ctx, `SELECT COUNT(*) FROM notifications `+base+` AND read_at IS NULL`, args...).Scan(&unread)
	return out, unread, err
}

// MarkNotificationsRead marks a user's notifications read (nil ids = all).
func (s *Store) MarkNotificationsRead(ctx context.Context, tenantID, userID string, ids []string) (int, error) {
	q := `UPDATE notifications SET read_at = ? WHERE tenant_id = ? AND user_id = ? AND read_at IS NULL`
	args := []any{s.now(), tenantID, userID}
	if ids != nil {
		if len(ids) == 0 {
			return 0, nil
		}
		q += ` AND id IN (` + placeholders(len(ids)) + `)`
		args = append(args, anySlice(ids)...)
	}
	res, err := s.c(ctx).exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneNotifications deletes read notifications older than the given age.
func (s *Store) PruneNotifications(ctx context.Context, age time.Duration) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM notifications WHERE read_at IS NOT NULL AND created_at < ?`, s.now().Add(-age))
	return err
}

// ---------------------------------------------------------------- webhooks

const webhookCols = `id, project_id, name, url, secret, events, enabled, created_by, created_at, updated_at, last_delivery_at, last_status, last_error`

func scanWebhook(r interface{ Scan(...any) error }) (*model.Webhook, error) {
	var w model.Webhook
	var events string
	var at sql.NullTime
	if err := r.Scan(&w.ID, &w.ProjectID, &w.Name, &w.URL, &w.Secret, &events, &w.Enabled, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt, &at, &w.LastStatus, &w.LastError); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(events), &w.Events)
	if w.Events == nil {
		w.Events = []string{}
	}
	w.LastDeliveryAt = timePtr(at)
	w.HasSecret = w.Secret != ""
	return &w, nil
}

// openWebhook decrypts the signing secret of a scanned webhook.
func (s *Store) openWebhook(ctx context.Context, w *model.Webhook) error {
	var err error
	if w.Secret, err = s.unseal(ctx, w.Secret, adWebhook(w.ID)); err != nil {
		return fmt.Errorf("webhook %s: %w", w.ID, err)
	}
	return nil
}

// NewWebhookSecret returns a random signing secret.
func NewWebhookSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + hex.EncodeToString(b)
}

func validateWebhook(w *model.Webhook) error {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" || len(w.Name) > 200 {
		return Invalidf("webhook name is required (at most 200 characters)")
	}
	w.URL = strings.TrimSpace(w.URL)
	u, err := url.Parse(w.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(w.URL) > 2000 {
		return Invalidf("webhook url must be an http or https URL")
	}
	if u.User != nil {
		return Invalidf("webhook url must not contain a user name or password")
	}
	if w.Events == nil {
		w.Events = []string{}
	}
	if len(w.Events) == 0 {
		return Invalidf("choose at least one event (or \"*\" for all)")
	}
	for _, e := range w.Events {
		if e != "*" && !slices.Contains(model.WebhookEvents, e) {
			return Invalidf("unknown event %q", e)
		}
	}
	return nil
}

// CreateWebhook adds a webhook; an empty secret gets a random one.
func (s *Store) CreateWebhook(ctx context.Context, tenantID string, w *model.Webhook) error {
	if err := validateWebhook(w); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, w.ProjectID); err != nil {
		return err
	}
	if w.Secret == "" {
		w.Secret = NewWebhookSecret()
	}
	w.HasSecret = true
	w.ID = newID()
	w.CreatedAt = s.now()
	w.UpdatedAt = w.CreatedAt
	ev, _ := json.Marshal(w.Events)
	secret, err := s.seal(w.Secret, adWebhook(w.ID))
	if err != nil {
		return err
	}
	_, err = s.c(ctx).exec(ctx, `INSERT INTO webhooks (id, tenant_id, project_id, name, url, secret, events, enabled, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, tenantID, w.ProjectID, w.Name, w.URL, secret, string(ev), w.Enabled, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	return mapErr(err, "webhook")
}

// UpdateWebhook saves name, URL, events, enabled and (when not empty) the secret.
func (s *Store) UpdateWebhook(ctx context.Context, tenantID string, w *model.Webhook) error {
	if err := validateWebhook(w); err != nil {
		return err
	}
	w.UpdatedAt = s.now()
	ev, _ := json.Marshal(w.Events)
	secret, err := s.seal(w.Secret, adWebhook(w.ID))
	if err != nil {
		return err
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE webhooks SET name = ?, url = ?, events = ?, enabled = ?, secret = CASE WHEN ? = '' THEN secret ELSE ? END, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		w.Name, w.URL, string(ev), w.Enabled, secret, secret, w.UpdatedAt, w.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "webhook")
	}
	return nil
}

// GetWebhook returns a webhook including its secret.
func (s *Store) GetWebhook(ctx context.Context, tenantID, id string) (*model.Webhook, error) {
	w, err := scanWebhook(s.c(ctx).row(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "webhook")
	}
	return w, s.openWebhook(ctx, w)
}

// ListWebhooks lists a project's webhooks including secrets.
func (s *Store) ListWebhooks(ctx context.Context, tenantID, projectID string) ([]model.Webhook, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE tenant_id = ? AND project_id = ? ORDER BY name, created_at`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Webhook{}
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		if err := s.openWebhook(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WebhooksFor returns the enabled webhooks of a project subscribed to event.
func (s *Store) WebhooksFor(ctx context.Context, tenantID, projectID, event string) ([]model.Webhook, error) {
	list, err := s.ListWebhooks(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, w := range list {
		if w.Enabled && (slices.Contains(w.Events, "*") || slices.Contains(w.Events, event)) {
			out = append(out, w)
		}
	}
	return out, nil
}

// DeleteWebhook removes a webhook.
func (s *Store) DeleteWebhook(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM webhooks WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "webhook")
	}
	return nil
}

// RecordWebhookDelivery stores the outcome of the latest delivery.
func (s *Store) RecordWebhookDelivery(ctx context.Context, id string, status int, errText string) error {
	if len(errText) > 500 {
		errText = errText[:500]
	}
	_, err := s.c(ctx).exec(ctx, `UPDATE webhooks SET last_delivery_at = ?, last_status = ?, last_error = ? WHERE id = ?`, s.now(), status, errText, id)
	return err
}

// MarkEntityNotificationsRead marks a user's unread notifications of one
// event about the given items read (e.g. review requests that are done).
func (s *Store) MarkEntityNotificationsRead(ctx context.Context, tenantID, userID, event string, entityIDs []string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	args := append([]any{s.now(), tenantID, userID, event}, anySlice(entityIDs)...)
	_, err := s.c(ctx).exec(ctx, `UPDATE notifications SET read_at = ? WHERE tenant_id = ? AND user_id = ? AND event = ? AND read_at IS NULL AND entity_id IN (`+placeholders(len(entityIDs))+`)`, args...)
	return err
}

// MarkNotificationsUnread marks a user's notifications unread again.
func (s *Store) MarkNotificationsUnread(ctx context.Context, tenantID, userID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := append([]any{tenantID, userID}, anySlice(ids)...)
	res, err := s.c(ctx).exec(ctx, `UPDATE notifications SET read_at = NULL WHERE tenant_id = ? AND user_id = ? AND read_at IS NOT NULL AND id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteNotifications deletes a user's notifications: the given ones, or
// with readOnly (and no ids) all that have been read.
func (s *Store) DeleteNotifications(ctx context.Context, tenantID, userID string, ids []string, readOnly bool) (int, error) {
	q := `DELETE FROM notifications WHERE tenant_id = ? AND user_id = ?`
	args := []any{tenantID, userID}
	switch {
	case len(ids) > 0:
		q += ` AND id IN (` + placeholders(len(ids)) + `)`
		args = append(args, anySlice(ids)...)
	case readOnly:
		q += ` AND read_at IS NOT NULL`
	default:
		return 0, Invalidf("give ids or read: true")
	}
	res, err := s.c(ctx).exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
