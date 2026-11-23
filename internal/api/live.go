package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/live"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Live updates and presence (internal/live).
//
// The single choke point of the events is Server.audit: every change a
// handler (or a background job) makes is audited, and audit hands the
// entity to publishChange, which looks up its project and version inside
// the request transaction and publishes the event once it is committed
// (store.AfterCommit). New notifications come from the store
// (Store.OnNotifications). An event carries the entity type, id, project,
// version, action and actor, never a title or content: the clients reload
// what they show through the regular API, with its permission checks.
//
// GET /api/v1/events/stream is a Server-Sent Events stream of the events of
// the caller's organisation that the caller may see: every event is checked
// against the caller's access to its project and the read permission of its
// entity type (liveEntities) before it is written; events of other types or
// without a project are never sent. Notification events reach only their
// recipients. Access changes (project sharing, roles, members) make the
// streams re-read the access, and every minute the stream re-authenticates
// its request (a revoked session or token ends it).

// liveEntity is an entity type sent to the clients.
type liveEntity struct {
	perm rbac.Permission
	// query returns project_id, version and parent of an id (? id, ? tenant)
	query string
}

var liveEntities = map[string]liveEntity{
	model.KindRequirement: {rbac.RequirementRd, `SELECT project_id, version, '' FROM requirements WHERE id = ? AND tenant_id = ?`},
	model.KindTest:        {rbac.TestRead, `SELECT project_id, version, '' FROM test_cases WHERE id = ? AND tenant_id = ?`},
	"run":                 {rbac.RunRead, `SELECT project_id, 0, '' FROM test_runs WHERE id = ? AND tenant_id = ?`},
	"execution":           {rbac.RunRead, `SELECT r.project_id, 0, e.run_id FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE e.id = ? AND r.tenant_id = ?`},
	"cycle":               {rbac.RunRead, `SELECT project_id, 0, '' FROM test_cycles WHERE id = ? AND tenant_id = ?`},
	model.KindExploratory: {rbac.RunRead, `SELECT project_id, 0, '' FROM exploratory_sessions WHERE id = ? AND tenant_id = ?`},
	"defect":              {rbac.ProjectRead, `SELECT project_id, 0, '' FROM defects WHERE id = ? AND tenant_id = ?`},
	"risk":                {rbac.RequirementRd, `SELECT project_id, 0, '' FROM risks WHERE id = ? AND tenant_id = ?`},
	"release":             {rbac.ProjectRead, `SELECT project_id, 0, '' FROM releases WHERE id = ? AND tenant_id = ?`},
	"project":             {rbac.ProjectRead, `SELECT id, 0, '' FROM projects WHERE id = ? AND tenant_id = ?`},
}

// accessEntities are audited entities whose changes can change what
// someone may see (sharing, roles, memberships).
var accessEntities = map[string]bool{"project": true, "user": true, "role": true, "group": true, "tenant": true, "membership": true, "member": true, "scim_group": true}

// liveOn reports whether live updates are turned on (live.enabled).
func (s *Server) liveOn() bool { return s.conf().Live.Enabled }

// publishChange turns an audited action into a change event (see the
// package comment).
func (s *Server) publishChange(ctx context.Context, p *Principal, action, entity, id string) {
	if s.live == nil || p == nil || p.Tenant == nil || !s.liveOn() {
		return
	}
	tenant := p.TenantID()
	if accessEntities[entity] {
		store.AfterCommit(ctx, func() {
			s.live.Publish(context.Background(), live.Event{Kind: live.KindAccess, TenantID: tenant})
		})
	}
	le, ok := liveEntities[entity]
	if !ok || id == "" {
		return
	}
	if !s.live.Wanted(tenant) {
		// nobody listens: skip the lookup, but a client resuming later reloads
		store.AfterCommit(ctx, func() { s.live.Gap(tenant) })
		return
	}
	actor := p.User.DisplayName
	if actor == "" {
		actor = p.User.Email
	}
	var events []live.Event
	// restoring from the trash audits several ids at once
	for _, one := range strings.Split(id, ",") {
		ev := live.Event{Kind: live.KindChange, TenantID: tenant, Type: entity, EntityID: one, Action: action, ActorID: p.User.ID, ActorName: actor}
		pid, version, parent, err := s.Store.LiveTarget(ctx, le.query, one, tenant)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			continue
		}
		ev.Version, ev.ParentID = version, parent
		// a deleted item: the project of the request (resolved before the
		// handler ran); a moved one: both projects hear about it
		if pid != "" {
			ev.ProjectID = pid
			events = append(events, ev)
		}
		if p.projectID != "" && p.projectID != pid {
			ev.ProjectID = p.projectID
			events = append(events, ev)
		}
	}
	store.AfterCommit(ctx, func() {
		for _, e := range events {
			s.live.Publish(context.Background(), e)
		}
	})
}

// liveNotified publishes the recipients of new notifications.
func (s *Server) liveNotified(tenantID string, users []string) {
	if s.live == nil || !s.liveOn() || len(users) == 0 {
		return
	}
	s.live.Publish(context.Background(), live.Event{Kind: live.KindNotification, TenantID: tenantID, Users: users})
}

// StartLive starts the cross-instance bus (PostgreSQL) and the presence
// sweeper until ctx ends. On SQLite the hub is in-process only (one
// instance per database file).
func (s *Server) StartLive(ctx context.Context) {
	if !s.liveOn() {
		return
	}
	if s.Store.DB.Dialect == db.Postgres {
		bus, err := live.NewPGBus(ctx, s.Store.DB.DB, s.Log)
		if err != nil {
			s.Log.Warn("live: no cross-instance bus", "err", err)
		} else {
			s.live.SetBus(bus)
			go bus.Run(ctx, s.live)
		}
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.live.SweepPresence()
			}
		}
	}()
}

// StopLive ends the open event streams (graceful shutdown: register with
// http.Server.RegisterOnShutdown, streams never become idle).
func (s *Server) StopLive() { s.live.Close() }

// ---------------------------------------------------------------- stream

// liveWire is an event as the clients receive it.
type liveWire struct {
	ID        string        `json:"id,omitempty"`
	Kind      string        `json:"kind"`
	Type      string        `json:"type,omitempty"`
	EntityID  string        `json:"entity_id,omitempty"`
	ParentID  string        `json:"parent_id,omitempty"`
	ProjectID string        `json:"project_id,omitempty"`
	Version   int           `json:"version,omitempty"`
	Action    string        `json:"action,omitempty"`
	ActorID   string        `json:"actor_id,omitempty"`
	ActorName string        `json:"actor_name,omitempty"`
	Presence  *presenceWire `json:"presence,omitempty"`
	Time      *time.Time    `json:"time,omitempty"`
}

type presenceWire struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	State  string `json:"state"`
}

func wireOf(ev live.Event) liveWire {
	w := liveWire{ID: ev.ID, Kind: ev.Kind, Type: ev.Type, EntityID: ev.EntityID, ParentID: ev.ParentID, ProjectID: ev.ProjectID,
		Version: ev.Version, Action: ev.Action, ActorID: ev.ActorID, ActorName: ev.ActorName}
	if !ev.Time.IsZero() {
		t := ev.Time
		w.Time = &t
	}
	if ev.Kind == live.KindPresence {
		w.ID = "" // presence is not replayed
		if ev.Presence != nil {
			w.Presence = &presenceWire{UserID: ev.Presence.UserID, Name: ev.Presence.Name, Avatar: ev.Presence.Avatar, State: ev.Presence.State}
		}
	}
	if ev.Kind == live.KindNotification {
		w = liveWire{ID: ev.ID, Kind: ev.Kind}
	}
	return w
}

// liveViewer decides which events the user of a stream may see.
type liveViewer struct {
	s     *Server
	p     *Principal
	vis   map[string]*model.ProjectAccessInfo
	visAt time.Time
	now   func() time.Time
}

// visTTL bounds how long a stream trusts what it read about project access.
const visTTL = 30 * time.Second

func (v *liveViewer) refresh(ctx context.Context) {
	vis, err := v.s.visibleProjects(ctx, v.p)
	if err != nil {
		vis = map[string]*model.ProjectAccessInfo{}
	}
	v.vis, v.visAt = vis, v.now()
}

// allowed reports whether the event may be written to this stream.
func (v *liveViewer) allowed(ctx context.Context, ev live.Event) bool {
	switch ev.Kind {
	case live.KindNotification:
		for _, u := range ev.Users {
			if u == v.p.User.ID {
				return true
			}
		}
		return false
	case live.KindPresence:
		if ev.ActorID == v.p.User.ID {
			return false // one's own presence is no news
		}
	case live.KindChange:
	default:
		return false
	}
	le, ok := liveEntities[ev.Type]
	if !ok || ev.ProjectID == "" {
		return false
	}
	if v.vis == nil || v.now().Sub(v.visAt) > visTTL {
		v.refresh(ctx)
	}
	return visibleIn(v.vis, v.p, ev.ProjectID, le.perm)
}

// handleEventStream: GET /api/v1/events/stream (Server-Sent Events).
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.liveOn() {
		writeError(w, http.StatusNotFound, "live_disabled", "live updates are turned off on this server")
		return
	}
	if !requireOrg(w, p) {
		return
	}
	cfg := s.conf().Live
	sub, err := s.live.Subscribe(p.TenantID(), p.User.ID)
	if err != nil {
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusServiceUnavailable, "live_unavailable", err.Error())
		return
	}
	defer s.live.Unsubscribe(sub)

	rc := http.NewResponseController(w)
	// a stream outlives server.read_timeout and server.write_timeout: no
	// read deadline (it would cancel the request), the write deadline is
	// extended before every write
	_ = rc.SetReadDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store, no-transform")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	v := &liveViewer{s: s, p: p, now: time.Now}
	v.refresh(ctx)
	write := func(event string, data any) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(cfg.Heartbeat + 30*time.Second))
		var b strings.Builder
		if wv, ok := data.(liveWire); ok && wv.ID != "" {
			fmt.Fprintf(&b, "id: %s\n", wv.ID)
		}
		if event != "" {
			body, _ := json.Marshal(data)
			fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", event, body)
		} else {
			b.WriteString(": ping\n\n")
		}
		if _, err := w.Write([]byte(b.String())); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("hello", map[string]any{"heartbeat_ms": cfg.Heartbeat.Milliseconds(), "presence_ttl_ms": cfg.PresenceTTL.Milliseconds(), "user_id": p.User.ID}) {
		return
	}
	_, _ = w.Write([]byte("retry: 5000\n\n"))

	// resume: what this client missed, or a reset when it is no longer kept
	last := r.Header.Get("Last-Event-ID")
	if last == "" {
		last = r.URL.Query().Get("last_event_id")
	}
	replayed := map[string]bool{}
	if last != "" {
		evs, ok := s.live.Since(p.TenantID(), last)
		if !ok {
			if !write(live.KindReset, liveWire{Kind: live.KindReset}) {
				return
			}
		}
		for _, ev := range evs {
			replayed[ev.ID] = true
			if v.allowed(ctx, ev) && !write(ev.Kind, wireOf(ev)) {
				return
			}
		}
	}

	beat := time.NewTicker(cfg.Heartbeat)
	defer beat.Stop()
	lastAuth := time.Now()
	reauth := s.liveReauth
	if reauth <= 0 {
		reauth = time.Minute
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done:
			_ = write("bye", map[string]string{"reason": "shutdown"})
			return
		case <-sub.Lost:
			if !write(live.KindReset, liveWire{Kind: live.KindReset}) {
				return
			}
		case ev := <-sub.C:
			if replayed[ev.ID] {
				delete(replayed, ev.ID)
				continue
			}
			if ev.Kind == live.KindAccess {
				v.vis = nil
				continue
			}
			if v.allowed(ctx, ev) && !write(ev.Kind, wireOf(ev)) {
				return
			}
		case <-beat.C:
			if time.Since(lastAuth) >= reauth {
				// a revoked session or token, a disabled user: end the stream
				np, err := s.principal(r)
				if err != nil || np.TenantID() != p.TenantID() {
					_ = write("bye", map[string]string{"reason": "unauthorized"})
					return
				}
				v.p, v.vis, lastAuth = np, nil, time.Now()
			}
			if !write("", nil) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------- presence

type presenceRequest struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	// State: viewing | editing | left
	State string `json:"state"`
}

// presenceTarget checks that the caller may see the item and returns its
// project.
func (s *Server) presenceTarget(ctx context.Context, p *Principal, typ, id string) (string, error) {
	le, ok := liveEntities[typ]
	if !ok || id == "" || typ == "project" {
		return "", badRequest("entity_type must be one of requirement, test, run, defect, risk, cycle, exploratory_session")
	}
	pid, _, _, err := s.Store.LiveTarget(ctx, le.query, id, p.TenantID())
	if err != nil {
		return "", err
	}
	if err := s.canIn(ctx, p, pid, le.perm); err != nil {
		if errors.Is(err, errForbidden) {
			return "", fmt.Errorf("%s: %w", typ, store.ErrNotFound)
		}
		return "", err
	}
	return pid, nil
}

// presenceList is who else is on an item.
func (s *Server) presenceList(p *Principal, typ, id string) []presenceWire {
	out := []presenceWire{}
	for _, x := range s.live.Presence(p.TenantID(), typ, id) {
		if x.UserID == p.User.ID {
			continue
		}
		out = append(out, presenceWire{UserID: x.UserID, Name: x.Name, Avatar: x.Avatar, State: x.State})
	}
	return out
}

// handlePresence: POST /api/v1/presence records that the caller views or
// edits an item (a heartbeat, every 20 s while the item is open; "left"
// when it is closed) and answers who else is on it.
func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.liveOn() {
		writeError(w, http.StatusNotFound, "live_disabled", "live updates are turned off on this server")
		return
	}
	if !requireOrg(w, p) {
		return
	}
	var req presenceRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	switch req.State {
	case live.StateViewing, live.StateEditing, live.StateLeft:
	case "":
		req.State = live.StateViewing
	default:
		s.fail(w, r, badRequest("state must be viewing, editing or left"))
		return
	}
	pid, err := s.presenceTarget(r.Context(), p, req.EntityType, req.EntityID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	name := p.User.DisplayName
	if name == "" {
		name = p.User.Email
	}
	pr := live.Presence{TenantID: p.TenantID(), Type: req.EntityType, EntityID: req.EntityID, ProjectID: pid, UserID: p.User.ID, Name: name, Avatar: p.User.Avatar, State: req.State}
	s.live.Publish(r.Context(), live.Event{Kind: live.KindPresence, TenantID: p.TenantID(), Type: req.EntityType, EntityID: req.EntityID, ProjectID: pid,
		ActorID: p.User.ID, ActorName: name, Presence: &pr})
	writeJSON(w, http.StatusOK, map[string]any{"others": s.presenceList(p, req.EntityType, req.EntityID), "ttl_ms": s.conf().Live.PresenceTTL.Milliseconds()})
}

// handleGetPresence: GET /api/v1/presence?entity_type=&entity_id= lists
// who else is viewing or editing an item.
func (s *Server) handleGetPresence(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.liveOn() {
		writeError(w, http.StatusNotFound, "live_disabled", "live updates are turned off on this server")
		return
	}
	if !requireOrg(w, p) {
		return
	}
	typ, id := r.URL.Query().Get("entity_type"), r.URL.Query().Get("entity_id")
	if _, err := s.presenceTarget(r.Context(), p, typ, id); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"others": s.presenceList(p, typ, id)})
}
