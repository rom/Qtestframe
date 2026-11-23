package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/chat"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Chat notifications: channels of a project post its events to Slack,
// Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix or
// Discord (rendering: internal/chat).
//
// The events are those of the central registry (model.WebhookEvents)
// that notifyEvent sends to the webhooks; a new event there is offered by
// GET /chat-events and can be chosen at once (grouped by chat.Group).
//
// Delivery: every post is a row of chat_deliveries (an outbox) written
// once the request's transaction is committed and tried at once in the
// background. A failed attempt is retried by the minute job (RunJobs →
// RunChatJobs) with exponential backoff (1, 2, 4, 8, 16 minutes, or the
// Retry-After of HTTP 429) up to chatMaxAttempts; a refused URL or token
// (HTTP 4xx) is not retried. After model.ChatMaxFailures failed deliveries
// in a row the channel turns itself off and the project's administrators
// are notified. A digest channel collects its events (status batched)
// and the job posts them as one message every DigestMinutes.
//
// Content: a message names the event, the item's key and title as a link
// (server.base_url; no link without it), its status and priority, who did
// it and a short summary. A channel of a restricted project shows only the
// event, the key, the status and the link, unless an organisation
// administrator allowed it the content (RestrictedContent). Secrets never
// reach a message, and the webhook URL / token is never returned by the
// API nor written to a log or error.

func (s *Server) routesChat(h routeFunc) {
	h("GET /api/v1/chat-events", s.handleChatEvents)
	h("GET /api/v1/projects/{pid}/chat-channels", s.handleListChatChannels, "project.update")
	h("POST /api/v1/projects/{pid}/chat-channels", s.handleCreateChatChannel, "project.update")
	h("PATCH /api/v1/chat-channels/{id}", s.handleUpdateChatChannel, "project.update")
	h("DELETE /api/v1/chat-channels/{id}", s.handleDeleteChatChannel, "project.update")
	h("POST /api/v1/chat-channels/{id}/test", s.handleTestChatChannel, "project.update")
	h("GET /api/v1/chat-channels/{id}/deliveries", s.handleChatDeliveries, "project.update")
}

const (
	// chatMaxAttempts: attempts of one delivery before it fails.
	chatMaxAttempts = 6
	// chatLease: how long a server owns a delivery it is posting.
	chatLease = 2 * time.Minute
)

// chatBackoff is the wait after the n-th failed attempt (1, 2, 4, … min).
func chatBackoff(n int, retryAfter time.Duration) time.Duration {
	d := time.Minute << max(0, min(n-1, 6))
	d = min(max(d, retryAfter), time.Hour)
	return d
}

// ---------------------------------------------------------------- events

type chatEvent struct {
	Event  string            `json:"event"`
	Group  string            `json:"group"`
	Labels map[string]string `json:"labels"`
}

type chatGroup struct {
	ID     string            `json:"id"`
	Labels map[string]string `json:"labels"`
}

// handleChatEvents: GET /chat-events — the events a channel can choose,
// from the central registry, with their groups and labels (en, sv).
func (s *Server) handleChatEvents(w http.ResponseWriter, r *http.Request, p *Principal) {
	events := make([]chatEvent, 0, len(model.WebhookEvents))
	used := map[string]bool{}
	for _, e := range model.WebhookEvents {
		g := chat.Group(e)
		used[g] = true
		events = append(events, chatEvent{Event: e, Group: g, Labels: map[string]string{"en": chat.EventLabel("en", e), "sv": chat.EventLabel("sv", e)}})
	}
	groups := []chatGroup{}
	for _, g := range chat.Groups {
		if used[g] {
			groups = append(groups, chatGroup{ID: g, Labels: map[string]string{"en": chat.GroupLabel("en", g), "sv": chat.GroupLabel("sv", g)}})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events, "groups": groups, "providers": chat.Providers})
}

// ---------------------------------------------------------------- channels

func maskChat(c model.ChatChannel) model.ChatChannel {
	c.URL, c.Token = "", ""
	return c
}

func (s *Server) handleListChatChannels(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListChatChannels(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range list {
		list[i] = maskChat(list[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "restricted": pr.Access == model.ProjectAccessRestricted,
		"can_allow_restricted": p.Can(rbac.TenantManage), "base_url": s.baseURL() != ""})
}

// checkChatRelease: the release of a filter must be one of the project's.
func (s *Server) checkChatRelease(ctx context.Context, p *Principal, projectID, id string) error {
	if id == "" {
		return nil
	}
	rel, err := s.Store.GetRelease(ctx, p.TenantID(), id)
	if err != nil || rel.ProjectID != projectID {
		return badRequest("the release of the filter is not one of this project's")
	}
	return nil
}

func (s *Server) handleCreateChatChannel(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.ChatChannel
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.RestrictedContent && !p.Can(rbac.TenantManage) {
		s.fail(w, r, forbidden("only organisation administrators may let a channel show restricted content"))
		return
	}
	if err := s.checkChatRelease(r.Context(), p, pr.ID, req.Filters.ReleaseID); err != nil {
		s.fail(w, r, err)
		return
	}
	c := &model.ChatChannel{ProjectID: pr.ID, Name: req.Name, Provider: req.Provider, URL: req.URL, Token: req.Token, Homeserver: req.Homeserver, Room: req.Room,
		Events: req.Events, Filters: req.Filters, Language: req.Language, DigestMinutes: req.DigestMinutes, Enabled: req.Enabled,
		RestrictedContent: req.RestrictedContent, CreatedBy: p.User.Email}
	if err := s.Store.CreateChatChannel(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "chat.create", "chat_channel", c.ID, c.Name+" ("+c.Provider+", "+c.Target+")")
	writeJSON(w, http.StatusCreated, maskChat(*c))
}

func (s *Server) chatChannel(w http.ResponseWriter, r *http.Request, p *Principal) (*model.ChatChannel, bool) {
	c, err := s.Store.GetChatChannel(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return c, true
}

// handleUpdateChatChannel changes a channel; a URL or token replaces the
// stored one (leave it out to keep it). Turning a channel on again clears
// its failures.
func (s *Server) handleUpdateChatChannel(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.chatChannel(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name              *string            `json:"name"`
		URL               *string            `json:"url"`
		Token             *string            `json:"token"`
		Homeserver        *string            `json:"homeserver"`
		Room              *string            `json:"room"`
		Events            *[]string          `json:"events"`
		Filters           *model.ChatFilters `json:"filters"`
		Language          *string            `json:"language"`
		DigestMinutes     *int               `json:"digest_minutes"`
		Enabled           *bool              `json:"enabled"`
		RestrictedContent *bool              `json:"restricted_content"`
		Provider          *string            `json:"provider"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Provider != nil && *req.Provider != c.Provider {
		s.fail(w, r, badRequest("the provider of a channel cannot change; add a new channel"))
		return
	}
	if req.RestrictedContent != nil && *req.RestrictedContent && !c.RestrictedContent && !p.Can(rbac.TenantManage) {
		s.fail(w, r, forbidden("only organisation administrators may let a channel show restricted content"))
		return
	}
	secretChanged := false
	c.URL, c.Token = "", "" // keep the stored secret unless a new one is given
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
		c.URL, secretChanged = *req.URL, true
	}
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" {
		c.Token, secretChanged = *req.Token, true
	}
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.Homeserver != nil {
		c.Homeserver = *req.Homeserver
	}
	if req.Room != nil {
		c.Room = *req.Room
	}
	if req.Events != nil {
		c.Events = *req.Events
	}
	if req.Filters != nil {
		if err := s.checkChatRelease(r.Context(), p, c.ProjectID, req.Filters.ReleaseID); err != nil {
			s.fail(w, r, err)
			return
		}
		c.Filters = *req.Filters
	}
	if req.Language != nil {
		c.Language = *req.Language
	}
	if req.DigestMinutes != nil {
		c.DigestMinutes = *req.DigestMinutes
	}
	if req.RestrictedContent != nil {
		c.RestrictedContent = *req.RestrictedContent
	}
	if req.Enabled != nil {
		if *req.Enabled && !c.Enabled {
			c.Failures, c.DisabledReason = 0, ""
		}
		c.Enabled = *req.Enabled
	}
	if err := s.Store.UpdateChatChannel(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	details := c.Name
	if secretChanged {
		details += " (new URL or token)"
	}
	s.audit(r.Context(), p, "chat.update", "chat_channel", c.ID, details)
	out, err := s.Store.GetChatChannel(r.Context(), p.TenantID(), c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, maskChat(*out))
}

func (s *Server) handleDeleteChatChannel(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.chatChannel(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteChatChannel(r.Context(), p.TenantID(), c.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "chat.delete", "chat_channel", c.ID, c.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChatDeliveries(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.chatChannel(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListChatDeliveries(r.Context(), p.TenantID(), c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleTestChatChannel posts a test message now (also to a channel that
// is off, to check a new URL before turning it on) and reports the result.
// It is logged as a delivery but never retried nor counted as a failure.
func (s *Server) handleTestChatChannel(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.chatChannel(w, r, p)
	if !ok {
		return
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), c.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	title := map[string]string{"en": "Qtestframe can post to this channel", "sv": "Qtestframe kan skriva i den här kanalen"}[c.Language]
	m := chat.Message{Event: chat.EventTest, Title: title, Actor: actorName(p), Project: pr.Key + " " + pr.Name, Time: s.Store.Now().UTC(),
		Summary: map[string]string{"en": "Channel: ", "sv": "Kanal: "}[c.Language] + c.Name}
	if base := s.baseURL(); base != "" {
		m.URL = base + "/p/" + url.PathEscape(pr.ID) + "/settings"
	}
	b, _ := json.Marshal([]chat.Message{m})
	id, err := s.Store.AddChatDelivery(r.Context(), p.TenantID(), c, chat.EventTest, b, 1, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, err := s.deliverChat(ctx, id)
	out := map[string]any{"ok": err == nil, "status": res.Status}
	if err != nil {
		out["error"] = err.Error()
	}
	s.audit(r.Context(), p, "chat.test", "chat_channel", c.ID, c.Name)
	writeJSON(w, http.StatusOK, out)
}

func actorName(p *Principal) string {
	if p == nil || p.User == nil {
		return ""
	}
	if p.User.DisplayName != "" {
		return p.User.DisplayName
	}
	return p.User.Email
}

// ---------------------------------------------------------------- events → messages

// chatFacts are what the filters look at.
type chatFacts struct {
	run      bool // a run or result event
	failed   bool // with failed results / a failed result
	priority string
	releases []string
}

func (f chatFacts) pass(flt model.ChatFilters) bool {
	if flt.FailedOnly && f.run && !f.failed {
		return false
	}
	if flt.MinPriority != "" && f.priority != "" && slices.Index(model.Priorities, f.priority) > slices.Index(model.Priorities, flt.MinPriority) {
		return false
	}
	if flt.ReleaseID != "" && !slices.Contains(f.releases, flt.ReleaseID) {
		return false
	}
	return true
}

// severityPriority puts defect severities on the priority scale.
var severityPriority = map[string]string{"critical": "critical", "major": "high", "minor": "medium", "trivial": "low"}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func obj(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	return v
}

// chatItemURL links to an item of a project ("" without server.base_url).
func (s *Server) chatItemURL(projectID, kind, id string, extra ...string) string {
	base := s.baseURL()
	if base == "" {
		return ""
	}
	p := base + "/p/" + url.PathEscape(projectID)
	q := url.QueryEscape(id)
	switch kind {
	case model.KindRequirement:
		return p + "/requirements?item=" + q
	case model.KindTest:
		return p + "/tests?item=" + q
	case model.KindDefect:
		return p + "/defects?item=" + q
	case model.KindRisk:
		return p + "/risks?risk=" + q
	case "waiver":
		return p + "/waivers?waiver=" + q
	case model.KindChangeRequest:
		return p + "/change-requests?cr=" + q
	case "release":
		return p + "/releases?r=" + q
	case "baseline":
		return p + "/baselines?b=" + q
	case "run":
		return p + "/runs/" + url.PathEscape(id)
	case "execution":
		if len(extra) > 0 && extra[0] != "" {
			return p + "/runs/" + url.PathEscape(extra[0]) + "/exec/" + url.PathEscape(id)
		}
		return p + "/runs"
	}
	return p + "/dashboard"
}

// runSummary is "12 passed, 2 failed" of a run's result counts.
func runSummary(sum map[string]any) (string, bool) {
	var parts []string
	for _, st := range model.ResultStatuses {
		if n, ok := sum[st].(float64); ok && n > 0 {
			parts = append(parts, strconv.Itoa(int(n))+" "+strings.ReplaceAll(st, "_", " "))
		}
	}
	failed, _ := sum[model.ResultFailed].(float64)
	return strings.Join(parts, ", "), failed > 0
}

// chatMessage builds the message of an event from its webhook data. It
// knows the data of the registered events and reads any other event's
// data by its common fields (key, title or name, status, id).
func (s *Server) chatMessage(ctx context.Context, p *Principal, pr *model.Project, event string, data any) (chat.Message, chatFacts) {
	m := chat.Message{Event: event, Actor: actorName(p), Project: pr.Key + " " + pr.Name, Time: s.Store.Now().UTC()}
	var f chatFacts
	b, _ := json.Marshal(data)
	var d map[string]any
	_ = json.Unmarshal(b, &d)
	if d == nil {
		d = map[string]any{}
	}
	prefix, _, _ := strings.Cut(event, ".")
	tenant := p.TenantID()
	switch {
	case obj(d, "defect") != nil:
		o := obj(d, "defect")
		m.Kind, m.Key, m.Title, m.Status = model.KindDefect, str(o, "key"), str(o, "title"), str(o, "status")
		m.Priority = str(o, "priority")
		if m.Priority == "" {
			m.Priority = severityPriority[str(o, "severity")]
		}
		m.URL = s.chatItemURL(pr.ID, model.KindDefect, str(o, "id"))
		switch event {
		case "defect.created":
			m.Summary = excerpt(str(o, "description"), 300)
		case "defect.assigned":
			m.Summary = "→ " + str(o, "assignee")
		case "defect.regression":
			if rg := obj(d, "regression"); rg != nil {
				m.Summary = strings.TrimSpace(str(rg, "test_key") + " " + str(rg, "run_name") + ": " + str(rg, "action"))
			}
		}
	case obj(d, "run") != nil:
		o := obj(d, "run")
		m.Kind, m.Title, m.Status = "run", str(o, "name"), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, "run", str(o, "id"))
		sum := obj(o, "summary")
		if sum == nil {
			if run, err := s.Store.GetRun(ctx, tenant, str(o, "id")); err == nil {
				sb, _ := json.Marshal(run.Summary)
				_ = json.Unmarshal(sb, &sum)
			}
		}
		m.Summary, f.failed = runSummary(sum)
		f.run = true
		if m.Status == "completed" {
			m.Tone = map[bool]string{true: chat.ToneDanger, false: chat.ToneGood}[f.failed]
		}
		if rel := str(o, "release_id"); rel != "" {
			f.releases = append(f.releases, rel)
		}
	case obj(d, "execution") != nil:
		o := obj(d, "execution")
		runID := str(d, "run_id")
		m.Kind, m.Key, m.Title, m.Status = "execution", str(o, "test_key"), str(o, "test_title"), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, "execution", str(o, "id"), runID)
		f.run, f.failed = true, m.Status == model.ResultFailed
		if run, err := s.Store.GetRun(ctx, tenant, runID); err == nil {
			m.Summary = run.Name
			if run.ReleaseID != nil {
				f.releases = append(f.releases, *run.ReleaseID)
			}
		}
	case obj(d, "release") != nil:
		o := obj(d, "release")
		m.Kind, m.Title, m.Status = "release", strings.TrimSpace(str(o, "name")+" "+str(o, "version")), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, "release", str(o, "id"))
		m.Summary = excerpt(str(o, "release_note", "description"), 300)
		f.releases = append(f.releases, str(o, "id"))
	case obj(d, "risk") != nil:
		o := obj(d, "risk")
		m.Kind, m.Key, m.Title, m.Status = model.KindRisk, str(o, "key"), str(o, "title"), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, model.KindRisk, str(o, "id"))
		l, _ := o["likelihood"].(float64)
		i, _ := o["impact"].(float64)
		if l > 0 && i > 0 {
			m.Priority = model.RiskLevel(int(l), int(i))
			m.Summary = fmt.Sprintf("%d × %d = %d", int(l), int(i), int(l*i))
		}
	case obj(d, "review") != nil:
		o := obj(d, "review")
		m.Kind, m.Key, m.Title, m.Status = str(o, "entity_type"), str(o, "entity_key"), str(o, "entity_title"), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, m.Kind, str(o, "entity_id"))
		who := str(o, "reviewer_name", "reviewer_email")
		if c := str(o, "comment", "message"); c != "" {
			m.Summary = who + ": " + excerpt(c, 300)
		} else {
			m.Summary = who
		}
	case obj(d, "waiver") != nil:
		o := obj(d, "waiver")
		m.Kind, m.Key, m.Title, m.Status = "waiver", str(o, "key"), str(o, "title"), str(o, "status")
		m.URL = s.chatItemURL(pr.ID, "waiver", str(o, "id"))
		m.Summary = excerpt(str(o, "decision_comment", "justification"), 300)
		if rel := str(o, "release_id"); rel != "" {
			f.releases = append(f.releases, rel)
		}
	case obj(d, "change_request") != nil:
		o := obj(d, "change_request")
		m.Kind, m.Key, m.Title, m.Status, m.Priority = model.KindChangeRequest, str(o, "key"), str(o, "title"), str(o, "status"), str(o, "priority")
		m.URL = s.chatItemURL(pr.ID, model.KindChangeRequest, str(o, "id"))
		m.Summary = excerpt(str(o, "decision_comment", "reason"), 300)
	case obj(d, "signoff") != nil:
		o := obj(d, "signoff")
		m.Kind, m.Title, m.Status = "baseline", str(d, "baseline"), str(o, "meaning")
		m.URL = s.chatItemURL(pr.ID, "baseline", str(d, "baseline_id"))
		m.Summary = excerpt(str(o, "comment"), 300)
	case event == "comment.created":
		m.Kind, m.Key = str(d, "kind"), str(d, "key")
		m.URL = s.chatItemURL(pr.ID, m.Kind, str(d, "id"))
		if c := obj(d, "comment"); c != nil {
			m.Summary = excerpt(str(c, "body"), 300)
		}
		s.chatItem(ctx, tenant, &m, &f, str(d, "id"), false)
	case str(d, "kind") == model.KindRequirement || str(d, "kind") == model.KindTest:
		m.Kind, m.Key, m.Title, m.Status = str(d, "kind"), str(d, "key"), str(d, "title"), str(d, "status")
		m.URL = s.chatItemURL(pr.ID, m.Kind, str(d, "id"))
		s.chatItem(ctx, tenant, &m, &f, str(d, "id"), strings.HasSuffix(event, ".created"))
	default:
		// an event added to the registry later: its main object, or the data
		o := d
		for _, v := range d {
			if mm, ok := v.(map[string]any); ok && (mm["key"] != nil || mm["title"] != nil || mm["name"] != nil) {
				o = mm
				break
			}
		}
		m.Kind = str(d, "kind")
		if m.Kind == "" {
			m.Kind = prefix
		}
		m.Key, m.Title, m.Status, m.Priority = str(o, "key"), str(o, "title", "name"), str(o, "status"), str(o, "priority")
		m.Summary = excerpt(str(o, "summary", "body", "description"), 300)
		m.URL = s.chatItemURL(pr.ID, m.Kind, str(o, "id"), str(o, "run_id"))
		if rel := str(o, "release_id"); rel != "" {
			f.releases = append(f.releases, rel)
		}
	}
	if m.Priority != "" && !slices.Contains(model.Priorities, m.Priority) {
		m.Priority = ""
	}
	f.priority = m.Priority
	return m, f
}

// chatItem fills in a requirement's or test's title, status, priority and
// (when created) description.
func (s *Server) chatItem(ctx context.Context, tenant string, m *chat.Message, f *chatFacts, id string, created bool) {
	var title, status, priority, desc string
	switch m.Kind {
	case model.KindRequirement:
		q, err := s.Store.GetRequirement(ctx, tenant, id)
		if err != nil {
			return
		}
		title, status, priority, desc = q.Title, q.Status, q.Priority, q.Description
	case model.KindTest:
		t, err := s.Store.GetTest(ctx, tenant, id)
		if err != nil {
			return
		}
		title, status, priority, desc = t.Title, t.Status, t.Priority, t.Description
	default:
		return
	}
	if m.Title == "" {
		m.Title = title
	}
	if m.Status == "" {
		m.Status = status
	}
	m.Priority = priority
	if created && m.Summary == "" {
		m.Summary = excerpt(desc, 300)
	}
}

// minimal keeps what a channel without RestrictedContent may show of a
// restricted project.
func minimal(m chat.Message, pr *model.Project) chat.Message {
	return chat.Message{Event: m.Event, Kind: m.Kind, Key: m.Key, URL: m.URL, Status: m.Status, Priority: m.Priority, Actor: m.Actor, Project: pr.Key, Time: m.Time}
}

// notifyChat queues the event for the project's chat channels once the
// request's transaction is committed.
func (s *Server) notifyChat(r *http.Request, p *Principal, projectID, event string, data any) {
	ctx := r.Context()
	channels, err := s.Store.ChatChannelsFor(ctx, p.TenantID(), projectID, event)
	if err != nil || len(channels) == 0 {
		return
	}
	pr, err := s.Store.GetProject(ctx, p.TenantID(), projectID)
	if err != nil {
		return
	}
	m, facts := s.chatMessage(ctx, p, pr, event, data)
	type post struct {
		ch   model.ChatChannel
		body []byte
	}
	var posts []post
	for _, ch := range channels {
		if !facts.pass(ch.Filters) {
			continue
		}
		msg := m
		if pr.Access == model.ProjectAccessRestricted && !ch.RestrictedContent {
			msg = minimal(m, pr)
		}
		b, _ := json.Marshal([]chat.Message{msg})
		posts = append(posts, post{ch, b})
	}
	if len(posts) == 0 {
		return
	}
	tenant := p.TenantID()
	store.AfterCommit(ctx, func() {
		for _, x := range posts {
			batched := x.ch.DigestMinutes > 0
			id, err := s.Store.AddChatDelivery(context.Background(), tenant, &x.ch, event, x.body, 1, batched)
			if err != nil {
				s.Log.Warn("chat queue", "channel", x.ch.ID, "event", event, "err", err)
				continue
			}
			if batched {
				continue
			}
			s.hooks.Add(1)
			go func() {
				defer s.hooks.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_, _ = s.deliverChat(ctx, id)
			}()
		}
	})
}

// ---------------------------------------------------------------- delivery

// deliverChat makes one attempt at a due delivery (nothing when another
// server or goroutine has it) and records the outcome.
func (s *Server) deliverChat(ctx context.Context, id string) (chat.Result, error) {
	job, err := s.Store.ClaimChatDelivery(ctx, id, chatLease)
	if err != nil {
		s.Log.Warn("chat claim", "delivery", id, "err", err)
		return chat.Result{}, err
	}
	if job == nil {
		return chat.Result{}, nil
	}
	ch := job.Channel
	test := job.Delivery.Event == chat.EventTest
	var msgs []chat.Message
	_ = json.Unmarshal(job.Messages, &msgs)
	var res chat.Result
	switch {
	case !ch.Enabled && !test:
		err = fmt.Errorf("the channel is turned off")
		res.Permanent = true
	case len(msgs) == 0:
		err = fmt.Errorf("nothing to post")
		res.Permanent = true
	default:
		res, err = chat.Send(ctx, s.webhookClient(), chat.Target{Provider: ch.Provider, URL: ch.URL, Homeserver: ch.Homeserver, Room: ch.Room, Token: ch.Token}, ch.Language, msgs)
	}
	o := store.ChatOutcome{OK: err == nil, StatusCode: res.Status, CountFailure: !test && ch.Enabled}
	attempt := job.Delivery.Attempts + 1
	if err != nil {
		o.Error = err.Error()
		if !res.Permanent && !test && attempt < chatMaxAttempts {
			at := s.Store.Now().Add(chatBackoff(attempt, res.RetryAfter))
			o.RetryAt = &at
		}
		s.Log.Warn("chat delivery", "channel", ch.ID, "event", job.Delivery.Event, "attempt", attempt, "err", o.Error)
	}
	failures, ferr := s.Store.FinishChatDelivery(context.Background(), id, ch.ID, o)
	if ferr != nil {
		s.Log.Warn("chat record", "err", ferr)
		return res, err
	}
	if err != nil && o.RetryAt == nil && o.CountFailure && failures >= model.ChatMaxFailures {
		s.disableChat(job.TenantID, ch, o.Error)
	}
	return res, err
}

// disableChat turns a failing channel off and tells the project's
// administrators (those who may change its settings).
func (s *Server) disableChat(tenantID string, ch *model.ChatChannel, lastErr string) {
	ctx := context.Background()
	reason := fmt.Sprintf("turned off after %d failed deliveries in a row; last error: %s", model.ChatMaxFailures, lastErr)
	if err := s.Store.DisableChatChannel(ctx, ch.ID, reason); err != nil {
		s.Log.Warn("chat disable", "err", err)
		return
	}
	s.Log.Warn("chat channel turned off", "channel", ch.ID, "project", ch.ProjectID, "err", lastErr)
	_ = s.Store.Audit(ctx, model.AuditEntry{TenantID: tenantID, UserEmail: "system", Action: "chat.disabled", EntityType: "chat_channel", EntityID: ch.ID, Details: ch.Name + ": " + reason})
	access, _, err := s.membersIn(ctx, tenantID, ch.ProjectID)
	if err != nil {
		return
	}
	var list []model.Notification
	for uid, acc := range access {
		if slices.Contains(acc.Permissions, string(rbac.ProjectUpdate)) {
			list = append(list, model.Notification{UserID: uid, ProjectID: ch.ProjectID, Event: "chat.disabled",
				Title: fmt.Sprintf("Chat channel %q was turned off", ch.Name), Body: reason, EntityType: "chat_channel", EntityID: ch.ID, Actor: "Qtestframe"})
		}
	}
	if len(list) == 0 {
		return
	}
	slices.SortFunc(list, func(a, b model.Notification) int { return strings.Compare(a.UserID, b.UserID) })
	if err := s.Store.AddNotifications(ctx, tenantID, list); err != nil {
		s.Log.Warn("chat disable notification", "err", err)
		return
	}
	s.mailNotifications(ctx, &Principal{User: &model.User{}}, list)
}

// RunChatJobs posts the digests that are due and retries the deliveries
// whose backoff has passed (called every minute by RunJobs).
func (s *Server) RunChatJobs(ctx context.Context) (int, error) {
	digests, err := s.Store.DueChatDigests(ctx)
	if err != nil {
		return 0, err
	}
	for _, chID := range digests {
		if _, err := s.Store.CollectChatDigest(ctx, chID); err != nil {
			s.Log.Warn("chat digest", "channel", chID, "err", err)
		}
	}
	due, err := s.Store.DueChatDeliveries(ctx, 200)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range due {
		if ctx.Err() != nil {
			break
		}
		actx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := s.deliverChat(actx, id); err == nil {
			n++
		}
		cancel()
	}
	return n, nil
}
