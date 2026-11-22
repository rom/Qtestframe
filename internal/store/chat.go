package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/chat"
	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- chat channels

func adChat(id string) string { return "chat_channels.secret|" + id }

const chatCols = `id, project_id, name, provider, secret, homeserver, room, target, events, filters, language, digest_minutes, enabled,
	restricted_content, failures, disabled_reason, created_by, created_at, updated_at, last_delivery_at, last_status, last_error, next_digest_at`

func scanChat(r interface{ Scan(...any) error }) (*model.ChatChannel, error) {
	var c model.ChatChannel
	var events, filters string
	var last, next sql.NullTime
	if err := r.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Provider, &c.URL, &c.Homeserver, &c.Room, &c.Target, &events, &filters, &c.Language, &c.DigestMinutes, &c.Enabled,
		&c.RestrictedContent, &c.Failures, &c.DisabledReason, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &last, &c.LastStatus, &c.LastError, &next); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(events), &c.Events)
	if c.Events == nil {
		c.Events = []string{}
	}
	_ = json.Unmarshal([]byte(filters), &c.Filters)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	c.LastDeliveryAt, c.NextDigestAt = timePtr(last), timePtr(next)
	c.HasSecret = c.URL != ""
	return &c, nil
}

// openChat decrypts the secret of a scanned channel into URL or Token.
func (s *Store) openChat(ctx context.Context, c *model.ChatChannel) error {
	v, err := s.unseal(ctx, c.URL, adChat(c.ID))
	if err != nil {
		return err
	}
	c.URL = ""
	if c.Provider == chat.Matrix {
		c.Token = v
	} else {
		c.URL = v
	}
	return nil
}

var matrixRoom = regexp.MustCompile(`^[!#][^:\s]+:[^\s]+$`)

func chatURL(v, what string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(v) > 2000 {
		return nil, Invalidf("%s must be an http or https URL", what)
	}
	if u.User != nil {
		return nil, Invalidf("%s must not contain a user name or password", what)
	}
	return u, nil
}

// validateChat checks a channel and works out its target. The secret
// (URL or Token) is required when create is true; otherwise an empty one
// keeps the stored secret.
func validateChat(c *model.ChatChannel, create bool) (secret string, err error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 200 {
		return "", Invalidf("channel name is required (at most 200 characters)")
	}
	if !slices.Contains(chat.Providers, c.Provider) {
		return "", Invalidf("provider must be one of %s", strings.Join(chat.Providers, ", "))
	}
	if c.Language == "" {
		c.Language = "en"
	}
	if c.Language != "en" && c.Language != "sv" {
		return "", Invalidf("language must be en or sv")
	}
	if c.DigestMinutes < 0 || c.DigestMinutes > model.ChatMaxDigestMinutes {
		return "", Invalidf("digest_minutes must be between 0 (off) and %d", model.ChatMaxDigestMinutes)
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	if len(c.Events) == 0 {
		return "", Invalidf("choose at least one event (or \"*\" for all)")
	}
	for _, e := range c.Events {
		if e != "*" && !slices.Contains(model.WebhookEvents, e) {
			return "", Invalidf("unknown event %q", e)
		}
	}
	if p := c.Filters.MinPriority; p != "" && !slices.Contains(model.Priorities, p) {
		return "", Invalidf("min_priority must be one of %s", strings.Join(model.Priorities, ", "))
	}
	if c.Provider == chat.Matrix {
		c.URL = ""
		c.Homeserver = strings.TrimSuffix(strings.TrimSpace(c.Homeserver), "/")
		u, err := chatURL(c.Homeserver, "homeserver")
		if err != nil {
			return "", err
		}
		c.Room = strings.TrimSpace(c.Room)
		if !matrixRoom.MatchString(c.Room) || len(c.Room) > 255 {
			return "", Invalidf("room must be a room ID (!id:server) or alias (#name:server)")
		}
		c.Token = strings.TrimSpace(c.Token)
		if create && c.Token == "" {
			return "", Invalidf("the access token is required")
		}
		if len(c.Token) > 2000 {
			return "", Invalidf("the access token is too long")
		}
		c.Target = c.Room + " @ " + u.Host
		return c.Token, nil
	}
	c.Homeserver, c.Room, c.Token = "", "", ""
	c.URL = strings.TrimSpace(c.URL)
	if c.URL == "" {
		if create {
			return "", Invalidf("the webhook URL is required")
		}
		return "", nil
	}
	u, err := chatURL(c.URL, "webhook url")
	if err != nil {
		return "", err
	}
	c.Target = u.Host
	return c.URL, nil
}

// CreateChatChannel adds a channel of a project.
func (s *Store) CreateChatChannel(ctx context.Context, tenantID string, c *model.ChatChannel) error {
	secret, err := validateChat(c, true)
	if err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, c.ProjectID); err != nil {
		return err
	}
	c.ID = newID()
	c.CreatedAt = s.now()
	c.UpdatedAt = c.CreatedAt
	sealed, err := s.seal(secret, adChat(c.ID))
	if err != nil {
		return err
	}
	ev, _ := json.Marshal(c.Events)
	fl, _ := json.Marshal(c.Filters)
	_, err = s.c(ctx).exec(ctx, `INSERT INTO chat_channels (id, tenant_id, project_id, name, provider, secret, homeserver, room, target, events, filters, language, digest_minutes,
		enabled, restricted_content, failures, disabled_reason, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?, ?)`,
		c.ID, tenantID, c.ProjectID, c.Name, c.Provider, sealed, c.Homeserver, c.Room, c.Target, string(ev), string(fl), c.Language, c.DigestMinutes,
		c.Enabled, c.RestrictedContent, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
	c.HasSecret = true
	return mapErr(err, "chat channel")
}

// UpdateChatChannel saves a channel; an empty URL / Token keeps the stored
// secret (and target). The provider cannot change.
func (s *Store) UpdateChatChannel(ctx context.Context, tenantID string, c *model.ChatChannel) error {
	secret, err := validateChat(c, false)
	if err != nil {
		return err
	}
	c.UpdatedAt = s.now()
	sealed, err := s.seal(secret, adChat(c.ID))
	if err != nil {
		return err
	}
	ev, _ := json.Marshal(c.Events)
	fl, _ := json.Marshal(c.Filters)
	res, err := s.c(ctx).exec(ctx, `UPDATE chat_channels SET name = ?, homeserver = ?, room = ?, events = ?, filters = ?, language = ?, digest_minutes = ?, enabled = ?,
		restricted_content = ?, failures = ?, disabled_reason = ?, updated_at = ?,
		secret = CASE WHEN ? = '' THEN secret ELSE ? END, target = CASE WHEN ? = '' THEN target ELSE ? END
		WHERE id = ? AND tenant_id = ?`,
		c.Name, c.Homeserver, c.Room, string(ev), string(fl), c.Language, c.DigestMinutes, c.Enabled,
		c.RestrictedContent, c.Failures, c.DisabledReason, c.UpdatedAt, sealed, sealed, c.Target, c.Target, c.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "chat channel")
	}
	return nil
}

// GetChatChannel returns a channel with its secret.
func (s *Store) GetChatChannel(ctx context.Context, tenantID, id string) (*model.ChatChannel, error) {
	c, err := scanChat(s.c(ctx).row(ctx, `SELECT `+chatCols+` FROM chat_channels WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "chat channel")
	}
	return c, s.openChat(ctx, c)
}

// ListChatChannels lists a project's channels with their secrets and the
// number of events waiting for a digest.
func (s *Store) ListChatChannels(ctx context.Context, tenantID, projectID string) ([]model.ChatChannel, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+chatCols+` FROM chat_channels WHERE tenant_id = ? AND project_id = ? ORDER BY name, created_at`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.ChatChannel{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	pending := map[string]int{}
	rows, err = s.c(ctx).query(ctx, `SELECT d.channel_id, COUNT(*) FROM chat_deliveries d JOIN chat_channels c ON c.id = d.channel_id
		WHERE c.tenant_id = ? AND c.project_id = ? AND d.status = ? GROUP BY d.channel_id`, tenantID, projectID, model.ChatBatched)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return nil, err
		}
		pending[id] = n
	}
	rows.Close()
	for i := range out {
		out[i].Pending = pending[out[i].ID]
		if err := s.openChat(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}

// ChatChannelsFor returns the enabled channels of a project subscribed to event.
func (s *Store) ChatChannelsFor(ctx context.Context, tenantID, projectID, event string) ([]model.ChatChannel, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+chatCols+` FROM chat_channels WHERE tenant_id = ? AND project_id = ? AND enabled = ? ORDER BY name`, tenantID, projectID, true)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ChatChannel
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		if slices.Contains(c.Events, "*") || slices.Contains(c.Events, event) {
			c.URL = "" // the secret is read when posting
			out = append(out, *c)
		}
	}
	return out, rows.Err()
}

// DeleteChatChannel removes a channel and its deliveries.
func (s *Store) DeleteChatChannel(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM chat_deliveries WHERE channel_id = ? AND tenant_id = ?`, id, tenantID); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM chat_channels WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "chat channel")
		}
		return nil
	})
}

// DisableChatChannel turns a channel off and records why.
func (s *Store) DisableChatChannel(ctx context.Context, id, reason string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE chat_channels SET enabled = ?, disabled_reason = ?, updated_at = ? WHERE id = ?`, false, shortText(reason, 500), s.now(), id)
	return err
}

func shortText(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return strings.ToValidUTF8(s, "")
}

// ---------------------------------------------------------------- deliveries

// ChatLogSize is the number of finished deliveries kept per channel.
const ChatLogSize = 50

// chatDigestCap bounds the events kept for one digest.
const chatDigestCap = 1000

// AddChatDelivery stores a post of a channel: queued (due now) or, for a
// digest channel, batched until the digest (whose time is set by the
// first batched event).
func (s *Store) AddChatDelivery(ctx context.Context, tenantID string, ch *model.ChatChannel, event string, messages []byte, count int, batched bool) (string, error) {
	id := newID()
	now := s.now()
	err := s.tx(ctx, func(c conn) error {
		status := model.ChatQueued
		var next any = now
		if batched {
			status, next = model.ChatBatched, nil
			if _, err := c.exec(ctx, `UPDATE chat_channels SET next_digest_at = ? WHERE id = ? AND next_digest_at IS NULL`, now.Add(time.Duration(ch.DigestMinutes)*time.Minute), ch.ID); err != nil {
				return err
			}
		}
		_, err := c.exec(ctx, `INSERT INTO chat_deliveries (id, tenant_id, channel_id, event, messages, message_count, status, attempts, status_code, error, created_at, next_attempt_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)`, id, tenantID, ch.ID, event, string(messages), count, status, now, next)
		return err
	})
	return id, err
}

// ChatJob is a claimed delivery with its channel (including the secret).
type ChatJob struct {
	TenantID string
	Delivery model.ChatDelivery
	Messages []byte
	Channel  *model.ChatChannel
}

// ClaimChatDelivery takes a due delivery for lease (another server, or a
// later job run, takes it again once the lease ends); nil when it is not
// due or claimed elsewhere.
func (s *Store) ClaimChatDelivery(ctx context.Context, id string, lease time.Duration) (*ChatJob, error) {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE chat_deliveries SET status = ?, next_attempt_at = ? WHERE id = ? AND status IN (?, ?, ?) AND next_attempt_at <= ?`,
		model.ChatSending, now.Add(lease), id, model.ChatQueued, model.ChatRetry, model.ChatSending, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	var j ChatJob
	var msgs string
	err = s.c(ctx).row(ctx, `SELECT tenant_id, channel_id, event, messages, message_count, attempts, created_at FROM chat_deliveries WHERE id = ?`, id).
		Scan(&j.TenantID, &j.Delivery.ChannelID, &j.Delivery.Event, &msgs, &j.Delivery.Count, &j.Delivery.Attempts, &j.Delivery.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "chat delivery")
	}
	j.Delivery.ID, j.Delivery.Status, j.Messages = id, model.ChatSending, []byte(msgs)
	if j.Channel, err = s.GetChatChannel(ctx, j.TenantID, j.Delivery.ChannelID); err != nil {
		return nil, err
	}
	return &j, nil
}

// ChatOutcome is the result of one attempt.
type ChatOutcome struct {
	OK         bool
	StatusCode int
	Error      string
	// RetryAt: when to try again (nil: the delivery is finished).
	RetryAt *time.Time
	// CountFailure: a final failure counts against the channel (not for
	// test messages).
	CountFailure bool
}

// FinishChatDelivery records an attempt on the delivery and its channel and
// returns the channel's failures in a row.
func (s *Store) FinishChatDelivery(ctx context.Context, id, channelID string, o ChatOutcome) (int, error) {
	now := s.now()
	o.Error = shortText(o.Error, 500)
	failures := 0
	err := s.tx(ctx, func(c conn) error {
		status := model.ChatSent
		var sent, next any = now, nil
		switch {
		case o.OK:
		case o.RetryAt != nil:
			status, sent, next = model.ChatRetry, nil, o.RetryAt.UTC()
		default:
			status, sent = model.ChatFailed, nil
		}
		if _, err := c.exec(ctx, `UPDATE chat_deliveries SET status = ?, attempts = attempts + 1, status_code = ?, error = ?, sent_at = ?, next_attempt_at = ? WHERE id = ?`,
			status, o.StatusCode, o.Error, sent, next, id); err != nil {
			return err
		}
		upd := `UPDATE chat_channels SET last_delivery_at = ?, last_status = ?, last_error = ?`
		switch {
		case o.OK:
			upd += `, failures = 0`
		case status == model.ChatFailed && o.CountFailure:
			upd += `, failures = failures + 1`
		}
		if _, err := c.exec(ctx, upd+` WHERE id = ?`, now, o.StatusCode, o.Error, channelID); err != nil {
			return err
		}
		if err := c.row(ctx, `SELECT failures FROM chat_channels WHERE id = ?`, channelID).Scan(&failures); err != nil {
			return mapErr(err, "chat channel")
		}
		if status == model.ChatRetry {
			return nil
		}
		// keep the newest ChatLogSize finished deliveries
		_, err := c.exec(ctx, `DELETE FROM chat_deliveries WHERE channel_id = ? AND status IN (?, ?) AND id NOT IN (
			SELECT id FROM chat_deliveries WHERE channel_id = ? AND status IN (?, ?) ORDER BY created_at DESC, id DESC LIMIT ?)`,
			channelID, model.ChatSent, model.ChatFailed, channelID, model.ChatSent, model.ChatFailed, ChatLogSize)
		return err
	})
	return failures, err
}

// DueChatDeliveries returns the IDs of the deliveries due now, oldest first.
func (s *Store) DueChatDeliveries(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM chat_deliveries WHERE status IN (?, ?, ?) AND next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?`,
		model.ChatQueued, model.ChatRetry, model.ChatSending, s.now(), limit)
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

// DueChatDigests returns the IDs of the channels whose digest is due.
func (s *Store) DueChatDigests(ctx context.Context) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM chat_channels WHERE next_digest_at IS NOT NULL AND next_digest_at <= ? ORDER BY next_digest_at`, s.now())
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

// CollectChatDigest turns the batched events of a channel whose digest is
// due into one queued delivery and returns its ID ("" when the digest is
// not due, was collected elsewhere or has no events).
func (s *Store) CollectChatDigest(ctx context.Context, channelID string) (string, error) {
	id := ""
	err := s.tx(ctx, func(c conn) error {
		now := s.now()
		res, err := c.exec(ctx, `UPDATE chat_channels SET next_digest_at = NULL WHERE id = ? AND next_digest_at IS NOT NULL AND next_digest_at <= ?`, channelID, now)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		rows, err := c.query(ctx, `SELECT id, tenant_id, event, messages FROM chat_deliveries WHERE channel_id = ? AND status = ? ORDER BY created_at, id`, channelID, model.ChatBatched)
		if err != nil {
			return err
		}
		var ids []string
		var tenant, event string
		all := []json.RawMessage{}
		for rows.Next() {
			var rid, ev, msgs string
			if err := rows.Scan(&rid, &tenant, &ev, &msgs); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, rid)
			event = ev
			var list []json.RawMessage
			if json.Unmarshal([]byte(msgs), &list) == nil && len(all) < chatDigestCap {
				all = append(all, list...)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if _, err := c.exec(ctx, `DELETE FROM chat_deliveries WHERE id IN (`+placeholders(len(ids))+`)`, anySlice(ids)...); err != nil {
			return err
		}
		if len(all) > chatDigestCap {
			all = all[:chatDigestCap]
		}
		if len(all) > 1 {
			event = "digest"
		}
		b, _ := json.Marshal(all)
		id = newID()
		_, err = c.exec(ctx, `INSERT INTO chat_deliveries (id, tenant_id, channel_id, event, messages, message_count, status, attempts, status_code, error, created_at, next_attempt_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)`, id, tenant, channelID, event, string(b), len(all), model.ChatQueued, now, now)
		return err
	})
	return id, err
}

// ListChatDeliveries returns a channel's deliveries, newest first (not the
// events waiting for a digest).
func (s *Store) ListChatDeliveries(ctx context.Context, tenantID, channelID string) ([]model.ChatDelivery, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, channel_id, event, message_count, status, attempts, status_code, error, created_at, next_attempt_at, sent_at
		FROM chat_deliveries WHERE tenant_id = ? AND channel_id = ? AND status <> ? ORDER BY created_at DESC, id DESC LIMIT ?`, tenantID, channelID, model.ChatBatched, ChatLogSize+10)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ChatDelivery{}
	for rows.Next() {
		var d model.ChatDelivery
		var next, sent sql.NullTime
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.Event, &d.Count, &d.Status, &d.Attempts, &d.StatusCode, &d.Error, &d.CreatedAt, &next, &sent); err != nil {
			return nil, err
		}
		d.CreatedAt = d.CreatedAt.UTC()
		d.NextAttemptAt, d.SentAt = timePtr(next), timePtr(sent)
		out = append(out, d)
	}
	return out, rows.Err()
}
