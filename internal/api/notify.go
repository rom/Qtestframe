package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rom/qtestframe/internal/mention"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Notifications (in-app, per user) and webhooks (per project, signed).

func (s *Server) routesNotifications(h routeFunc) {
	h("GET /api/v1/notifications", s.handleListNotifications)
	h("POST /api/v1/notifications/read", s.handleReadNotifications)
	h("POST /api/v1/notifications/unread", s.handleUnreadNotifications)
	h("DELETE /api/v1/notifications", s.handleDeleteNotifications)
	h("GET /api/v1/webhook-events", s.handleWebhookEvents)
	h("GET /api/v1/projects/{pid}/webhooks", s.handleListWebhooks, "project.update")
	h("POST /api/v1/projects/{pid}/webhooks", s.handleCreateWebhook, "project.update")
	h("POST /api/v1/projects/{pid}/webhooks/bulk", s.handleBulkWebhooks, "project.update")
	h("DELETE /api/v1/projects/{pid}/webhooks/bulk", s.handleBulkDeleteWebhooks, "project.update")
	h("POST /api/v1/projects/{pid}/webhooks/test", s.handleBulkTestWebhooks, "project.update")
	h("PATCH /api/v1/webhooks/{id}", s.handleUpdateWebhook, "project.update")
	h("DELETE /api/v1/webhooks/{id}", s.handleDeleteWebhook, "project.update")
	h("POST /api/v1/webhooks/{id}/test", s.handleTestWebhook, "project.update")
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request, p *Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, unread, err := s.Store.ListNotifications(r.Context(), p.TenantID(), p.User.ID, r.URL.Query().Get("unread") != "", limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "unread": unread})
}

// handleReadNotifications marks notifications read: {"ids": [...]} or {} for all.
func (s *Server) handleReadNotifications(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Store.MarkNotificationsRead(r.Context(), p.TenantID(), p.User.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"marked": n})
}

// handleUnreadNotifications marks notifications unread: {"ids": [...]}.
func (s *Server) handleUnreadNotifications(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.IDs) == 0 {
		s.fail(w, r, badRequest("choose at least one notification"))
		return
	}
	n, err := s.Store.MarkNotificationsUnread(r.Context(), p.TenantID(), p.User.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"marked": n})
}

// handleDeleteNotifications deletes the caller's notifications:
// {"ids": [...]} or {"read": true} for all read ones.
func (s *Server) handleDeleteNotifications(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		IDs  []string `json:"ids"`
		Read bool     `json:"read"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Store.DeleteNotifications(r.Context(), p.TenantID(), p.User.ID, req.IDs, req.Read)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

// ---------------------------------------------------------------- recipients

// recipients resolves people (user IDs, e-mail addresses, display names or
// @handles) to members of the tenant, leaving out the actor.
func (s *Server) recipients(ctx context.Context, p *Principal, people ...string) []string {
	ms, err := s.Store.ListTenantMembers(ctx, p.TenantID())
	if err != nil {
		return nil
	}
	users := make([]mention.User, 0, len(ms))
	for _, m := range ms {
		users = append(users, mention.User{ID: m.UserID, Email: m.Email, Name: m.Name})
	}
	index := map[string]string{}
	for _, d := range mention.Directory(users, nil) {
		if d.UserID == "" {
			continue
		}
		for _, k := range []string{d.UserID, d.Email, d.Name, d.Handle, "@" + d.Handle} {
			if k != "" {
				index[strings.ToLower(k)] = d.UserID
			}
		}
	}
	seen := map[string]bool{p.User.ID: true}
	var out []string
	for _, v := range people {
		id, ok := index[strings.ToLower(strings.TrimSpace(v))]
		if ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// notifyUsers stores an in-app notification for each recipient.
func (s *Server) notifyUsers(ctx context.Context, p *Principal, users []string, n model.Notification) {
	if len(users) == 0 {
		return
	}
	n.Actor = p.User.DisplayName
	if n.Actor == "" {
		n.Actor = p.User.Email
	}
	list := make([]model.Notification, 0, len(users))
	for _, u := range users {
		c := n
		c.UserID = u
		list = append(list, c)
	}
	if err := s.Store.AddNotifications(ctx, p.TenantID(), list); err != nil {
		s.Log.Warn("notifications", "event", n.Event, "err", err)
	}
}

// notifyMentions notifies the people @mentioned in text.
func (s *Server) notifyMentions(ctx context.Context, p *Principal, projectID, entityType, entityID, title, text string) {
	hs := mention.Extract(text)
	if len(hs) == 0 {
		return
	}
	s.notifyUsers(ctx, p, s.recipients(ctx, p, hs...), model.Notification{ProjectID: projectID, Event: "mention",
		Title: title, Body: excerpt(text, 300), EntityType: entityType, EntityID: entityID})
}

func excerpt(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// notifyDefect sends webhooks for a defect event and tells the assignee.
func (s *Server) notifyDefect(r *http.Request, p *Principal, d *model.Defect, event string) {
	s.notifyEvent(r, p, d.ProjectID, event, map[string]any{"defect": d})
	if d.Assignee != "" && (event == "defect.created" || event == "defect.assigned" || event == "defect.status") {
		title := d.Key + " " + d.Title
		if event == "defect.status" {
			title += " → " + d.Status
		}
		s.notifyUsers(r.Context(), p, s.recipients(r.Context(), p, d.Assignee), model.Notification{ProjectID: d.ProjectID,
			Event: event, Title: title, Body: excerpt(d.Description, 300), EntityType: model.KindDefect, EntityID: d.ID})
	}
}

// ---------------------------------------------------------------- webhooks

// webhookPayload is the body of a delivery.
type webhookPayload struct {
	ID        string    `json:"id"`
	Event     string    `json:"event"`
	ProjectID string    `json:"project_id"`
	Actor     string    `json:"actor"`
	Time      time.Time `json:"time"`
	Data      any       `json:"data"`
}

// Sign returns the signature header value of a webhook body.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// notifyEvent posts an event to the project's webhooks in the background.
func (s *Server) notifyEvent(r *http.Request, p *Principal, projectID, event string, data any) {
	hooks, err := s.Store.WebhooksFor(r.Context(), p.TenantID(), projectID, event)
	if err != nil || len(hooks) == 0 {
		return
	}
	body, err := json.Marshal(webhookPayload{ID: uuid.NewString(), Event: event, ProjectID: projectID, Actor: p.User.Email, Time: time.Now().UTC(), Data: data})
	if err != nil {
		return
	}
	for _, hk := range hooks {
		s.hooks.Add(1)
		go func(hk model.Webhook) {
			defer s.hooks.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			s.deliver(ctx, &hk, event, body)
		}(hk)
	}
}

// WaitWebhooks waits for deliveries in flight (tests, shutdown).
func (s *Server) WaitWebhooks() { s.hooks.Wait() }

func (s *Server) webhookClient() *http.Client {
	c := http.Client{Timeout: 10 * time.Second}
	if s.HTTPClient != nil {
		c = *s.HTTPClient
	}
	// Never follow redirects: the URL is what the admin configured.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// deliver posts one body and records the outcome on the webhook.
func (s *Server) deliver(ctx context.Context, hk *model.Webhook, event string, body []byte) (int, error) {
	status, err := func() (int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, hk.URL, bytes.NewReader(body))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Qtestframe-Webhook/1")
		req.Header.Set("X-Qtestframe-Event", event)
		req.Header.Set("X-Qtestframe-Webhook", hk.ID)
		req.Header.Set("X-Qtestframe-Signature", Sign(hk.Secret, body))
		res, err := s.webhookClient().Do(req)
		if err != nil {
			return 0, err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		if res.StatusCode >= 300 {
			return res.StatusCode, fmt.Errorf("HTTP %d", res.StatusCode)
		}
		return res.StatusCode, nil
	}()
	msg := ""
	if err != nil {
		msg = err.Error()
		s.Log.Warn("webhook delivery", "webhook", hk.ID, "event", event, "err", msg)
	}
	if rerr := s.Store.RecordWebhookDelivery(context.Background(), hk.ID, status, msg); rerr != nil {
		s.Log.Warn("webhook record", "err", rerr)
	}
	return status, err
}

func maskWebhook(w model.Webhook) model.Webhook {
	w.Secret = ""
	return w
}

func (s *Server) handleWebhookEvents(w http.ResponseWriter, r *http.Request, p *Principal) {
	writeJSON(w, http.StatusOK, listOf(model.WebhookEvents))
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListWebhooks(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range list {
		list[i] = maskWebhook(list[i])
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleCreateWebhook returns the secret once; it is never shown again.
func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.Webhook
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	hk := &model.Webhook{ProjectID: pr.ID, Name: req.Name, URL: req.URL, Secret: req.Secret, Events: req.Events, Enabled: req.Enabled, CreatedBy: p.User.Email}
	if err := s.Store.CreateWebhook(r.Context(), p.TenantID(), hk); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "webhook.create", "webhook", hk.ID, hk.Name+" "+hk.URL)
	writeJSON(w, http.StatusCreated, hk)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Webhook, bool) {
	hk, err := s.Store.GetWebhook(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return hk, true
}

// handleUpdateWebhook changes a webhook; "rotate_secret": true makes a new
// secret and returns it.
func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request, p *Principal) {
	hk, ok := s.webhook(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name         *string   `json:"name"`
		URL          *string   `json:"url"`
		Events       *[]string `json:"events"`
		Enabled      *bool     `json:"enabled"`
		Secret       string    `json:"secret"`
		RotateSecret bool      `json:"rotate_secret"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		hk.Name = *req.Name
	}
	if req.URL != nil {
		hk.URL = *req.URL
	}
	if req.Events != nil {
		hk.Events = *req.Events
	}
	if req.Enabled != nil {
		hk.Enabled = *req.Enabled
	}
	hk.Secret = req.Secret
	if req.RotateSecret {
		hk.Secret = store.NewWebhookSecret()
	}
	if err := s.Store.UpdateWebhook(r.Context(), p.TenantID(), hk); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "webhook.update", "webhook", hk.ID, hk.Name)
	out := *hk
	if !req.RotateSecret {
		out.Secret = ""
	}
	out.HasSecret = true
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request, p *Principal) {
	hk, ok := s.webhook(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteWebhook(r.Context(), p.TenantID(), hk.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "webhook.delete", "webhook", hk.ID, hk.Name)
	w.WriteHeader(http.StatusNoContent)
}

// handleTestWebhook sends a "ping" event now and reports the result.
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request, p *Principal) {
	hk, ok := s.webhook(w, r, p)
	if !ok {
		return
	}
	body, _ := json.Marshal(webhookPayload{ID: uuid.NewString(), Event: "ping", ProjectID: hk.ProjectID, Actor: p.User.Email, Time: time.Now().UTC(),
		Data: map[string]string{"webhook": hk.Name}})
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, err := s.deliver(ctx, hk, "ping", body)
	res := map[string]any{"ok": err == nil, "status": status}
	if err != nil {
		res["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------- hooks

// newText returns the part of after that was not in before (whole text
// when it changed in other ways), so mentions are only notified once.
func newText(before, after string) string {
	if before == after {
		return ""
	}
	if strings.HasPrefix(after, before) {
		return after[len(before):]
	}
	old := map[string]bool{}
	for _, h := range mention.Extract(before) {
		old[h] = true
	}
	var add []string
	for _, h := range mention.Extract(after) {
		if !old[h] {
			add = append(add, "@"+h)
		}
	}
	if len(add) == 0 {
		return ""
	}
	return after
}

// afterItemChange sends the created/updated webhook, tells a new owner and
// the people newly mentioned in text.
func (s *Server) afterItemChange(r *http.Request, p *Principal, kind, id, projectID, key, title, oldOwner, owner, text string, created bool) {
	ev := kind + ".updated"
	if created {
		ev = kind + ".created"
	}
	s.notifyEvent(r, p, projectID, ev, map[string]any{"kind": kind, "id": id, "key": key, "title": title})
	ctx := r.Context()
	if owner != "" && owner != oldOwner {
		s.notifyUsers(ctx, p, s.recipients(ctx, p, owner), model.Notification{ProjectID: projectID, Event: "assigned",
			Title: key + " " + title, EntityType: kind, EntityID: id})
	}
	if text != "" {
		s.notifyMentions(ctx, p, projectID, kind, id, key+" "+title, text)
	}
}

// afterComment sends the webhook and notifies the people mentioned.
func (s *Server) afterComment(r *http.Request, p *Principal, kind, id string, c *model.Comment) {
	ctx := r.Context()
	var projectID, key, title string
	if kind == model.KindTest {
		if t, err := s.Store.GetTest(ctx, p.TenantID(), id); err == nil {
			projectID, key, title = t.ProjectID, t.Key, t.Title
		}
	} else if q, err := s.Store.GetRequirement(ctx, p.TenantID(), id); err == nil {
		projectID, key, title = q.ProjectID, q.Key, q.Title
	}
	if projectID == "" {
		return
	}
	s.notifyEvent(r, p, projectID, "comment.created", map[string]any{"kind": kind, "id": id, "key": key, "comment": c})
	s.notifyMentions(ctx, p, projectID, kind, id, key+" "+title, c.Body)
}

// afterExecutionChange sends webhooks, tells a new assignee and the people
// mentioned in new notes, and reports a run that just completed.
func (s *Server) afterExecutionChange(r *http.Request, p *Principal, before, after *model.Execution, runBefore string) {
	ctx := r.Context()
	run, err := s.Store.GetRun(ctx, p.TenantID(), after.RunID)
	if err != nil {
		return
	}
	s.notifyEvent(r, p, run.ProjectID, "execution.updated", map[string]any{"execution": after, "run_id": run.ID})
	title := after.TestKey + " " + after.TestTitle + " (" + run.Name + ")"
	if after.Assignee != "" && after.Assignee != before.Assignee {
		s.notifyUsers(ctx, p, s.recipients(ctx, p, after.Assignee), model.Notification{ProjectID: run.ProjectID, Event: "assigned",
			Title: title, EntityType: "execution", EntityID: after.ID})
	}
	if t := newText(before.Notes, after.Notes); t != "" {
		s.notifyMentions(ctx, p, run.ProjectID, "execution", after.ID, title, t)
	}
	if run.Status == "completed" && runBefore != "completed" {
		s.notifyEvent(r, p, run.ProjectID, "run.completed", map[string]any{"run": run})
	}
}

// ---------------------------------------------------------------- bulk

func (s *Server) bulkWebhooksOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.Webhook, error) {
	ids, err := bulkIDs(ids)
	if err != nil {
		return nil, err
	}
	var out []*model.Webhook
	for _, id := range ids {
		hk, err := s.Store.GetWebhook(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if hk.ProjectID != projectID {
			return nil, fmt.Errorf("webhook %s: %w", id, store.ErrNotFound)
		}
		out = append(out, hk)
	}
	return out, nil
}

// handleBulkWebhooks: POST /projects/{pid}/webhooks/bulk
// {"ids": [...], "enabled"?, "add_events"?, "remove_events"?}. Every
// webhook must keep at least one event; all are checked before any is saved.
func (s *Server) handleBulkWebhooks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs          []string `json:"ids"`
		Enabled      *bool    `json:"enabled"`
		AddEvents    []string `json:"add_events"`
		RemoveEvents []string `json:"remove_events"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Enabled == nil && len(req.AddEvents) == 0 && len(req.RemoveEvents) == 0 {
		s.fail(w, r, badRequest("nothing to change: enabled, add_events or remove_events"))
		return
	}
	for _, e := range append(append([]string{}, req.AddEvents...), req.RemoveEvents...) {
		if e != "*" && !slices.Contains(model.WebhookEvents, e) {
			s.fail(w, r, badRequest("unknown event %q", e))
			return
		}
	}
	list, err := s.bulkWebhooksOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, hk := range list {
		events := slices.DeleteFunc(append([]string(nil), hk.Events...), func(e string) bool { return slices.Contains(req.RemoveEvents, e) })
		for _, e := range req.AddEvents {
			if !slices.Contains(events, e) {
				events = append(events, e)
			}
		}
		if len(events) == 0 {
			s.fail(w, r, badRequest("%s would have no events left", hk.Name))
			return
		}
		hk.Events = events
		if req.Enabled != nil {
			hk.Enabled = *req.Enabled
		}
		hk.Secret = "" // keep the stored secret
	}
	for _, hk := range list {
		if err := s.Store.UpdateWebhook(r.Context(), p.TenantID(), hk); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", hk.Name, err))
			return
		}
		s.audit(r.Context(), p, "webhook.update", "webhook", hk.ID, hk.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(list)})
}

// handleBulkDeleteWebhooks: DELETE /projects/{pid}/webhooks/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteWebhooks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.bulkWebhooksOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, hk := range list {
		if err := s.Store.DeleteWebhook(r.Context(), p.TenantID(), hk.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", hk.Name, err))
			return
		}
		s.audit(r.Context(), p, "webhook.delete", "webhook", hk.ID, hk.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(list)})
}

// handleBulkTestWebhooks: POST /projects/{pid}/webhooks/test {"ids": [...]}
// sends a ping to each webhook (one after another) →
// {"results": [{id, name, ok, status, error}]}.
func (s *Server) handleBulkTestWebhooks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.bulkWebhooksOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(list) > 50 {
		s.fail(w, r, badRequest("test at most 50 webhooks at a time"))
		return
	}
	results := []map[string]any{}
	for _, hk := range list {
		body, _ := json.Marshal(webhookPayload{ID: uuid.NewString(), Event: "ping", ProjectID: hk.ProjectID, Actor: p.User.Email, Time: time.Now().UTC(),
			Data: map[string]string{"webhook": hk.Name}})
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		status, err := s.deliver(ctx, hk, "ping", body)
		cancel()
		res := map[string]any{"id": hk.ID, "name": hk.Name, "ok": err == nil, "status": status}
		if err != nil {
			res["error"] = err.Error()
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
