package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Test environments and bookings (see docs/api.md "Test environments").
// Reading needs run.read; creating and changing environments, approving
// bookings and revealing an environment's secret need environment.manage
// (in the project, or in the organisation for a shared environment);
// booking needs run.execute in the project the booking is for. The owner
// of an environment may approve its bookings too.

// environmentSchema filters environments (GET /projects/{pid}/environments?q=).
var environmentSchema = &query.Schema{
	Fields: []string{"id", "name", "type", "status", "description", "owner", "tag", "component", "url", "shared", "capacity", "approval",
		"health", "booked", "created_by", "created_at", "updated_at"},
	Aliases: map[string]string{"tags": "tag", "components": "component", "version": "component", "versions": "component", "urls": "url",
		"booked_now": "booked"},
}

func (s *Server) routesEnvironments(h routeFunc) {
	h("GET /api/v1/projects/{pid}/environments", s.handleListEnvironments, "run.read")
	h("POST /api/v1/projects/{pid}/environments", s.handleCreateEnvironment, "environment.manage")
	h("GET /api/v1/projects/{pid}/bookings", s.handleProjectBookings, "run.read")
	h("GET /api/v1/environments", s.handleListSharedEnvironments, "run.read")
	h("GET /api/v1/environments/{id}", s.handleGetEnvironment, "run.read")
	h("PATCH /api/v1/environments/{id}", s.handleUpdateEnvironment, "environment.manage")
	h("DELETE /api/v1/environments/{id}", s.handleDeleteEnvironment, "environment.manage")
	h("POST /api/v1/environments/{id}/secret", s.handleRevealEnvironmentSecret, "environment.manage")
	h("POST /api/v1/environments/{id}/check", s.handleCheckEnvironment, "environment.manage")
	h("GET /api/v1/environments/{id}/history", s.handleEnvironmentHistory, "run.read")
	h("GET /api/v1/environments/{id}/bookings", s.handleListBookings, "run.read")
	h("POST /api/v1/environments/{id}/bookings", s.handleCreateBooking, "run.read")
	h("GET /api/v1/bookings/{id}", s.handleGetBooking, "run.read")
	h("PATCH /api/v1/bookings/{id}", s.handleUpdateBooking, "run.read")
	h("POST /api/v1/bookings/{id}/decision", s.handleDecideBooking, "run.read")
	h("POST /api/v1/bookings/{id}/cancel", s.handleCancelBooking, "run.read")
	h("GET /api/v1/runs/{id}/bookings", s.handleItemBookings("run"), "run.read")
	h("GET /api/v1/cycles/{id}/bookings", s.handleItemBookings("cycle"), "run.read")
	h("GET /api/v1/exploratory-sessions/{id}/bookings", s.handleItemBookings(model.KindExploratory), "run.read")
	h("GET /api/v1/releases/{id}/environments", s.handleReleaseEnvironments, "run.read")
}

// handleReleaseEnvironments lists the environments and component versions
// the results of a release's runs were obtained in.
func (s *Server) handleReleaseEnvironments(w http.ResponseWriter, r *http.Request, p *Principal) {
	rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ReleaseEnvironments(r.Context(), p.TenantID(), rel.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// envReq is the body of POST /projects/{pid}/environments and PATCH
// /environments/{id}.
type envReq struct {
	Name             *string            `json:"name"`
	Type             *string            `json:"type"`
	URLs             *[]string          `json:"urls"`
	Description      *string            `json:"description"`
	Components       *[]model.Component `json:"components"`
	OwnerID          *string            `json:"owner_id"`
	Status           *string            `json:"status"`
	StatusNote       *string            `json:"status_note"`
	Tags             *[]string          `json:"tags"`
	ConfigurationIDs *[]string          `json:"configuration_ids"`
	VariantIDs       *[]string          `json:"variant_ids"`
	Capacity         *int               `json:"capacity"`
	RequiresApproval *bool              `json:"requires_approval"`
	CredentialsRef   *string            `json:"credentials_ref"`
	// Secret: write-only ("" keeps the stored one); ClearSecret removes it.
	Secret                *string `json:"secret"`
	ClearSecret           bool    `json:"clear_secret"`
	HealthURL             *string `json:"health_url"`
	HealthIntervalMinutes *int    `json:"health_interval_minutes"`
	// Shared (create only): the environment is shared by the organisation
	// (needs environment.manage in the organisation).
	Shared bool `json:"shared"`
}

func (in *envReq) apply(e *model.Environment) {
	for _, f := range []struct{ src, dst *string }{{in.Name, &e.Name}, {in.Type, &e.Type}, {in.Description, &e.Description}, {in.OwnerID, &e.OwnerID},
		{in.Status, &e.Status}, {in.StatusNote, &e.StatusNote}, {in.CredentialsRef, &e.CredentialsRef}, {in.HealthURL, &e.HealthURL}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	for _, f := range []struct{ src, dst *[]string }{{in.URLs, &e.URLs}, {in.Tags, &e.Tags}, {in.ConfigurationIDs, &e.ConfigurationIDs}, {in.VariantIDs, &e.VariantIDs}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.Components != nil {
		e.Components = *in.Components
	}
	if in.Capacity != nil {
		e.Capacity = *in.Capacity
	}
	if in.RequiresApproval != nil {
		e.RequiresApproval = *in.RequiresApproval
	}
	if in.HealthIntervalMinutes != nil {
		e.HealthIntervalMinutes = *in.HealthIntervalMinutes
	}
	if in.Secret != nil {
		e.Secret = *in.Secret
	}
	e.ClearSecret = in.ClearSecret
}

// orgCan reports whether the caller holds perm in the organisation (not a
// project role): shared environments are governed by the organisation roles.
func (p *Principal) orgCan(perm rbac.Permission) bool {
	if p.User.SuperAdmin && p.Token == nil {
		return true
	}
	return slices.Contains(p.orgPermissions(), string(perm))
}

// canManageEnv reports whether the caller may manage an environment.
func (s *Server) canManageEnv(ctx context.Context, p *Principal, e *model.Environment) bool {
	if e.Shared {
		return p.orgCan(rbac.EnvironmentManage)
	}
	return s.canIn(ctx, p, e.ProjectID, rbac.EnvironmentManage) == nil
}

// environmentRecord is an environment for the query language.
func environmentRecord(e *model.Environment, people func(string) []string) query.MapRecord {
	var comps []string
	for _, c := range e.Components {
		comps = append(comps, c.Name, strings.TrimSpace(c.Name+" "+c.Version))
	}
	booked := "false"
	if e.BookedNow > 0 {
		booked = "true"
	}
	return query.MapRecord{"id": {e.ID}, "name": {e.Name}, "type": {e.Type}, "status": {e.Status}, "description": {e.Description},
		"owner": people(e.OwnerID), "tag": e.Tags, "component": comps, "url": e.URLs, "shared": {strconv.FormatBool(e.Shared)},
		"capacity": {strconv.Itoa(e.Capacity)}, "approval": {strconv.FormatBool(e.RequiresApproval)}, "health": {e.HealthStatus},
		"booked": {booked}, "created_by": {e.CreatedBy}, "created_at": {ts(e.CreatedAt)}, "updated_at": {ts(e.UpdatedAt)}}
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), environmentSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListEnvironments(r.Context(), p.TenantID(), pr.ID, r.URL.Query().Get("shared") != "false")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	people := s.peopleLookup(r.Context())
	out := []model.Environment{}
	var recs []query.MapRecord
	for i := range list {
		if rec := environmentRecord(&list[i], people); q.Match(rec) {
			out = append(out, list[i])
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleListSharedEnvironments(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	list, err := s.Store.ListEnvironments(r.Context(), p.TenantID(), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// recordEnvironment stores a revision of an environment for its history;
// failures are only logged.
func (s *Server) recordEnvironment(ctx context.Context, tenantID, user string, e *model.Environment, action string) {
	if err := s.Store.AddRevision(ctx, tenantID, model.KindEnvironment, e.ID, e.Version, action, user, store.SnapshotEnvironment(e)); err != nil {
		s.Log.Warn("revision not recorded", "entity", model.KindEnvironment, "id", e.ID, "err", err)
	}
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	var in envReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	e := &model.Environment{ProjectID: pr.ID, OwnerID: p.User.ID, CreatedBy: p.User.Email}
	if in.Shared {
		if !p.orgCan(rbac.EnvironmentManage) {
			writeError(w, http.StatusForbidden, "forbidden", "permission environment.manage in the organisation required for a shared environment")
			return
		}
		e.ProjectID = ""
	}
	in.apply(e)
	if in.Status != nil && *in.Status != "available" && *in.Status != "" {
		// a new environment may start in any state but needs one that exists
		e.Status = *in.Status
	}
	if e.OwnerID != "" && e.OwnerID != p.User.ID && !s.userCanSee(ctx, p.TenantID(), e.ProjectID, e.OwnerID, rbac.RunRead) {
		s.fail(w, r, badRequest("the owner may not see the runs of this project"))
		return
	}
	if err := s.Store.CreateEnvironment(ctx, p.TenantID(), e); err != nil {
		s.fail(w, r, err)
		return
	}
	scope := pr.Key
	if e.Shared {
		scope = "shared"
	}
	s.audit(ctx, p, "environment.create", model.KindEnvironment, e.ID, e.Name+" ("+scope+")")
	out, err := s.Store.GetEnvironment(ctx, p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.recordEnvironment(ctx, p.TenantID(), p.User.Email, out, "create")
	writeJSON(w, http.StatusCreated, out)
}

// loadEnvironment loads the {id} environment and checks that the caller
// may see it (a project environment: the project; a shared one: any
// member).
func (s *Server) loadEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Environment, bool) {
	e, err := s.Store.GetEnvironment(r.Context(), p.TenantID(), r.PathValue("id"))
	if err == nil && !e.Shared {
		err = s.canIn(r.Context(), p, e.ProjectID, "")
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return e, true
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	if !s.canManageEnv(ctx, p, e) {
		writeError(w, http.StatusForbidden, "forbidden", "permission environment.manage required")
		return
	}
	var in envReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	before, owner := e.Status, e.OwnerID
	in.apply(e)
	if e.OwnerID != owner && e.OwnerID != "" && !s.userCanSee(ctx, p.TenantID(), e.ProjectID, e.OwnerID, rbac.RunRead) {
		s.fail(w, r, badRequest("the owner may not see the runs of this project"))
		return
	}
	secretChanged := in.ClearSecret || (in.Secret != nil && *in.Secret != "")
	if err := s.Store.UpdateEnvironment(ctx, p.TenantID(), e); err != nil {
		s.fail(w, r, err)
		return
	}
	detail := e.Name + " " + e.Status
	if secretChanged {
		detail += " (secret changed)"
	}
	s.audit(ctx, p, "environment.update", model.KindEnvironment, e.ID, detail)
	out, err := s.Store.GetEnvironment(ctx, p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.recordEnvironment(ctx, p.TenantID(), p.User.Email, out, "update")
	if before != out.Status {
		s.environmentStatusChanged(r, p, out, before)
	}
	writeJSON(w, http.StatusOK, out)
}

// environmentStatusChanged tells the owner and the event subscribers.
func (s *Server) environmentStatusChanged(r *http.Request, p *Principal, e *model.Environment, before string) {
	ctx := r.Context()
	if e.OwnerID != "" && e.OwnerID != p.User.ID {
		s.notifyUsers(ctx, p, []string{e.OwnerID}, model.Notification{ProjectID: e.ProjectID, Event: "environment.status_changed",
			Title: e.Name, Body: before + " → " + e.Status + nonBlank(" · ", e.StatusNote), EntityType: model.KindEnvironment, EntityID: e.ID})
	}
	if e.ProjectID != "" {
		s.notifyEvent(r, p, e.ProjectID, "environment.status_changed", map[string]any{"environment": e, "previous_status": before})
	}
}

func nonBlank(sep, v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return sep + v
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	if !s.canManageEnv(r.Context(), p, e) {
		writeError(w, http.StatusForbidden, "forbidden", "permission environment.manage required")
		return
	}
	if err := s.Store.DeleteEnvironment(r.Context(), p.TenantID(), e.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "environment.delete", model.KindEnvironment, e.ID, e.Name)
	w.WriteHeader(http.StatusNoContent)
}

// handleRevealEnvironmentSecret returns the decrypted secret of an
// environment to an environment manager and writes an audit entry.
func (s *Server) handleRevealEnvironmentSecret(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	if !s.canManageEnv(r.Context(), p, e) {
		writeError(w, http.StatusForbidden, "forbidden", "permission environment.manage required")
		return
	}
	if !e.HasSecret {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	secret, err := s.Store.EnvironmentSecret(r.Context(), p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "environment.secret_reveal", model.KindEnvironment, e.ID, e.Name)
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
}

func (s *Server) handleEnvironmentHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	revs, err := s.Store.ListRevisions(r.Context(), p.TenantID(), model.KindEnvironment, e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(revs))
}

// ---------------------------------------------------------------- health checks

// healthTimeout bounds one health check request.
const healthTimeout = 10 * time.Second

// checkHealthURL fetches a health check URL: a 2xx or 3xx answer is up.
func (s *Server) checkHealthURL(ctx context.Context, u string) error {
	c := http.Client{Timeout: healthTimeout}
	if s.HTTPClient != nil {
		c = *s.HTTPClient
		if c.Timeout == 0 || c.Timeout > healthTimeout {
			c.Timeout = healthTimeout
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "qtestframe-health-check")
	res, err := c.Do(req)
	if err != nil {
		// the error may hold the URL: keep only what went wrong
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) && ue.Unwrap() != nil {
			return ue.Unwrap()
		}
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return nil
}

// runHealthCheck checks one environment and records the outcome; a status
// change is audited and told to the owner and the event subscribers.
func (s *Server) runHealthCheck(ctx context.Context, h store.HealthCheck) (string, error) {
	err := s.checkHealthURL(ctx, h.URL)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	before, after, rerr := s.Store.RecordHealth(ctx, h.TenantID, h.EnvironmentID, err == nil, msg, s.now())
	if rerr != nil {
		return "", rerr
	}
	if before != after {
		_ = s.Store.Audit(ctx, model.AuditEntry{TenantID: h.TenantID, Action: "environment.health", EntityType: model.KindEnvironment, EntityID: h.EnvironmentID,
			Details: fmt.Sprintf("%s: %s → %s%s", h.Name, before, after, nonBlank(" (", msg)+map[bool]string{true: ")", false: ""}[msg != ""])})
		if e, gerr := s.Store.GetEnvironment(ctx, h.TenantID, h.EnvironmentID); gerr == nil {
			p := &Principal{User: &model.User{Email: "qtestframe", DisplayName: "Qtestframe"}, Tenant: &model.Tenant{ID: h.TenantID}}
			s.environmentStatusChanged(jobRequest(ctx), p, e, before)
		}
	}
	if err != nil {
		return "down", nil
	}
	return "up", nil
}

// RunEnvironmentHealthChecks checks the environments whose health check is
// due (called every minute by RunJobs), a few at a time. It returns how
// many it checked.
func (s *Server) RunEnvironmentHealthChecks(ctx context.Context) (int, error) {
	due, err := s.Store.DueHealthChecks(ctx, s.now(), 100)
	if err != nil {
		return 0, err
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, h := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func(h store.HealthCheck) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := s.runHealthCheck(ctx, h); err != nil {
				s.Log.Warn("environment health check", "environment", h.EnvironmentID, "err", err)
			}
		}(h)
	}
	wg.Wait()
	return len(due), nil
}

// handleCheckEnvironment runs the health check now.
func (s *Server) handleCheckEnvironment(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	if !s.canManageEnv(r.Context(), p, e) {
		writeError(w, http.StatusForbidden, "forbidden", "permission environment.manage required")
		return
	}
	if e.HealthURL == "" {
		s.fail(w, r, badRequest("the environment has no health check URL"))
		return
	}
	if _, err := s.runHealthCheck(r.Context(), store.HealthCheck{TenantID: p.TenantID(), EnvironmentID: e.ID, ProjectID: e.ProjectID, Name: e.Name, URL: e.HealthURL}); err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.Store.GetEnvironment(r.Context(), p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- bookings

// bookingReq is the body of POST /environments/{id}/bookings and PATCH
// /bookings/{id}.
type bookingReq struct {
	StartsAt  *time.Time `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	Purpose   *string    `json:"purpose"`
	ProjectID *string    `json:"project_id"`
	RunID     *string    `json:"run_id"`
	CycleID   *string    `json:"cycle_id"`
	SessionID *string    `json:"session_id"`
}

func (in *bookingReq) apply(b *model.Booking) {
	if in.StartsAt != nil {
		b.StartsAt = *in.StartsAt
	}
	if in.EndsAt != nil {
		b.EndsAt = *in.EndsAt
	}
	if in.Purpose != nil {
		b.Purpose = *in.Purpose
	}
	for _, f := range []struct {
		src *string
		dst **string
	}{{in.RunID, &b.RunID}, {in.CycleID, &b.CycleID}, {in.SessionID, &b.SessionID}} {
		if f.src != nil {
			*f.dst = optID(f.src)
		}
	}
}

// writeBookingConflict answers 409 with the overlapping bookings.
func (s *Server) writeBookingConflict(w http.ResponseWriter, r *http.Request, p *Principal, err error) bool {
	var ce *store.BookingConflictError
	if !errors.As(err, &ce) {
		return false
	}
	over := s.redactBookings(r.Context(), p, ce.Conflict.Overlapping)
	writeJSON(w, http.StatusConflict, map[string]any{"error": ce.Error(), "code": "booking_conflict", "capacity": ce.Conflict.Capacity, "overlapping": over})
	return true
}

// redactBookings hides what a booking is for when it was made for a
// project the caller may not see.
func (s *Server) redactBookings(ctx context.Context, p *Principal, list []model.Booking) []model.Booking {
	vis, err := s.visibleProjects(ctx, p)
	out := make([]model.Booking, len(list))
	for i, b := range list {
		if b.ProjectID != "" && (err != nil || vis[b.ProjectID] == nil) {
			b = model.Booking{ID: b.ID, EnvironmentID: b.EnvironmentID, EnvironmentName: b.EnvironmentName, StartsAt: b.StartsAt, EndsAt: b.EndsAt,
				Status: b.Status, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, Hidden: true}
		}
		out[i] = b
	}
	return out
}

// bookingWindow reads ?from= and ?to= (RFC 3339 or YYYY-MM-DD).
func bookingWindow(r *http.Request) (store.BookingFilter, error) {
	var f store.BookingFilter
	parse := func(v string) (time.Time, error) {
		if v == "" {
			return time.Time{}, nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t, nil
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return time.Time{}, badRequest("from and to must be RFC 3339 times or YYYY-MM-DD dates")
		}
		return t, nil
	}
	var err error
	if f.From, err = parse(r.URL.Query().Get("from")); err != nil {
		return f, err
	}
	if f.To, err = parse(r.URL.Query().Get("to")); err != nil {
		return f, err
	}
	if st := r.URL.Query().Get("status"); st != "" {
		f.Statuses = strings.Split(st, ",")
	}
	return f, nil
}

func (s *Server) handleListBookings(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	f, err := bookingWindow(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListBookings(r.Context(), p.TenantID(), e.ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(s.redactBookings(r.Context(), p, list)))
}

func (s *Server) handleProjectBookings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	f, err := bookingWindow(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListProjectBookings(r.Context(), p.TenantID(), pr.ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(s.redactBookings(r.Context(), p, list)))
}

func (s *Server) handleCreateBooking(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, ok := s.loadEnvironment(w, r, p)
	if !ok {
		return
	}
	var in bookingReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := s.bookEnvironment(r, p, e, &in)
	if err != nil {
		if !s.writeBookingConflict(w, r, p, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// bookEnvironment books an environment for the caller (in the project of
// a project environment, or the given project for a shared one) and tells
// the approvers when it needs approval. Environment managers book without
// approval.
func (s *Server) bookEnvironment(r *http.Request, p *Principal, e *model.Environment, in *bookingReq) (*model.Booking, error) {
	ctx := r.Context()
	b := &model.Booking{EnvironmentID: e.ID, ProjectID: e.ProjectID, UserID: p.User.ID, BookedBy: p.User.Email}
	if e.Shared && in.ProjectID != nil {
		b.ProjectID = strings.TrimSpace(*in.ProjectID)
	}
	in.apply(b)
	if b.ProjectID != "" {
		if err := s.canIn(ctx, p, b.ProjectID, rbac.RunExecute); err != nil {
			return nil, err
		}
	} else if !p.orgCan(rbac.RunExecute) {
		return nil, fmt.Errorf("%w: permission run.execute required", errForbidden)
	}
	manager := s.canManageEnv(ctx, p, e) || (e.OwnerID != "" && e.OwnerID == p.User.ID)
	if err := s.Store.CreateBooking(ctx, p.TenantID(), b, manager); err != nil {
		return nil, err
	}
	out, err := s.Store.GetBooking(ctx, p.TenantID(), b.ID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, p, "booking.create", model.KindBooking, out.ID, fmt.Sprintf("%s %s – %s (%s)", e.Name, ts(out.StartsAt), ts(out.EndsAt), out.Status))
	if out.Status == "pending" {
		s.bookingRequested(r, p, e, out)
	}
	return out, nil
}

// bookingApprovers are the users who may decide on the bookings of an
// environment: its owner and the environment managers.
func (s *Server) bookingApprovers(ctx context.Context, tenantID string, e *model.Environment) []string {
	var out []string
	if e.OwnerID != "" {
		out = append(out, e.OwnerID)
	}
	if e.Shared {
		ms, err := s.Store.ListTenantMembers(ctx, tenantID)
		if err != nil {
			return out
		}
		set, err := s.Store.RoleSet(ctx, tenantID)
		if err != nil {
			return out
		}
		for _, m := range ms {
			if slices.Contains(set.PermissionsOfAll(m.Roles), string(rbac.EnvironmentManage)) && !slices.Contains(out, m.UserID) {
				out = append(out, m.UserID)
			}
		}
		return out
	}
	acc, _, err := s.membersIn(ctx, tenantID, e.ProjectID)
	if err != nil {
		return out
	}
	for id, a := range acc {
		if slices.Contains(a.Permissions, string(rbac.EnvironmentManage)) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func bookingTitle(b *model.Booking) string {
	return b.EnvironmentName + " " + b.StartsAt.Format("2006-01-02 15:04") + " – " + b.EndsAt.Format("2006-01-02 15:04") + " UTC"
}

func (s *Server) bookingRequested(r *http.Request, p *Principal, e *model.Environment, b *model.Booking) {
	ctx := r.Context()
	var to []string
	for _, id := range s.bookingApprovers(ctx, p.TenantID(), e) {
		if id != p.User.ID {
			to = append(to, id)
		}
	}
	s.notifyUsers(ctx, p, to, model.Notification{ProjectID: b.ProjectID, Event: "environment.booking_requested", Title: bookingTitle(b),
		Body: excerpt(b.BookedBy+nonBlank(": ", b.Purpose), 300), EntityType: model.KindEnvironment, EntityID: e.ID})
	if b.ProjectID != "" {
		s.notifyEvent(r, p, b.ProjectID, "environment.booking_requested", map[string]any{"booking": b, "environment": e})
	}
}

// loadBooking loads the {id} booking with its environment.
func (s *Server) loadBooking(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Booking, *model.Environment, bool) {
	ctx := r.Context()
	b, err := s.Store.GetBooking(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	e, err := s.Store.GetEnvironment(ctx, p.TenantID(), b.EnvironmentID)
	if err == nil && !e.Shared {
		err = s.canIn(ctx, p, e.ProjectID, "")
	}
	if err == nil && b.ProjectID != "" && b.UserID != p.User.ID {
		if s.canIn(ctx, p, b.ProjectID, "") != nil {
			// a booking for a project the caller does not see
			if s.canManageEnv(ctx, p, e) {
				red := s.redactBookings(ctx, p, []model.Booking{*b})
				b = &red[0]
			} else {
				err = errNoProject
			}
		}
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	return b, e, true
}

func (s *Server) handleGetBooking(w http.ResponseWriter, r *http.Request, p *Principal) {
	b, _, ok := s.loadBooking(w, r, p)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// mayChangeBooking: the booker, the environment's owner and managers.
func (s *Server) mayChangeBooking(ctx context.Context, p *Principal, b *model.Booking, e *model.Environment) bool {
	return b.UserID == p.User.ID || (e.OwnerID != "" && e.OwnerID == p.User.ID) || s.canManageEnv(ctx, p, e)
}

func (s *Server) handleUpdateBooking(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	b, e, ok := s.loadBooking(w, r, p)
	if !ok {
		return
	}
	if b.Hidden || !s.mayChangeBooking(ctx, p, b, e) {
		writeError(w, http.StatusForbidden, "forbidden", "only the booker or an environment manager can change a booking")
		return
	}
	var in bookingReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.ProjectID = nil
	in.apply(b)
	manager := s.canManageEnv(ctx, p, e) || (e.OwnerID != "" && e.OwnerID == p.User.ID)
	if err := s.Store.UpdateBooking(ctx, p.TenantID(), b, manager); err != nil {
		if !s.writeBookingConflict(w, r, p, err) {
			s.fail(w, r, err)
		}
		return
	}
	out, err := s.Store.GetBooking(ctx, p.TenantID(), b.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "booking.update", model.KindBooking, out.ID, fmt.Sprintf("%s %s – %s (%s)", e.Name, ts(out.StartsAt), ts(out.EndsAt), out.Status))
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDecideBooking(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	b, e, ok := s.loadBooking(w, r, p)
	if !ok {
		return
	}
	if !s.canManageEnv(ctx, p, e) && (e.OwnerID == "" || e.OwnerID != p.User.ID) {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner of the environment or an environment manager can approve bookings")
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Decision != "approved" && in.Decision != "rejected" {
		s.fail(w, r, badRequest("decision must be approved or rejected"))
		return
	}
	out, err := s.Store.DecideBooking(ctx, p.TenantID(), b.ID, in.Decision, p.User.Email, in.Note)
	if err != nil {
		if !s.writeBookingConflict(w, r, p, err) {
			s.fail(w, r, err)
		}
		return
	}
	s.audit(ctx, p, "booking.decide", model.KindBooking, out.ID, e.Name+": "+in.Decision+nonBlank(" · ", in.Note))
	event := "environment.booking_" + in.Decision
	s.notifyUsers(ctx, p, []string{out.UserID}, model.Notification{ProjectID: out.ProjectID, Event: event, Title: bookingTitle(out),
		Body: excerpt(in.Note, 300), EntityType: model.KindEnvironment, EntityID: e.ID})
	if out.ProjectID != "" {
		s.notifyEvent(r, p, out.ProjectID, event, map[string]any{"booking": out, "environment": e})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCancelBooking(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	b, e, ok := s.loadBooking(w, r, p)
	if !ok {
		return
	}
	if !s.mayChangeBooking(ctx, p, b, e) {
		writeError(w, http.StatusForbidden, "forbidden", "only the booker or an environment manager can cancel a booking")
		return
	}
	out, err := s.Store.DecideBooking(ctx, p.TenantID(), b.ID, "cancelled", p.User.Email, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "booking.cancel", model.KindBooking, out.ID, e.Name)
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- runs, cycles and sessions

// itemEnvReq is the environment part of the run, cycle and exploratory
// session bodies: environment_id selects an environment (id or name; ""
// none) and booking books it for the item at once.
type itemEnvReq struct {
	EnvironmentID *string     `json:"environment_id"`
	Booking       *bookingReq `json:"booking"`
}

// resolveEnvironmentRef turns an environment id or name of a project into
// its id (nil for "").
func (s *Server) resolveEnvironmentRef(ctx context.Context, tenantID, projectID string, ref *string) (*string, error) {
	if ref == nil || strings.TrimSpace(*ref) == "" {
		return nil, nil
	}
	e, err := s.Store.FindEnvironment(ctx, tenantID, projectID, *ref)
	if err != nil {
		return nil, err
	}
	return &e.ID, nil
}

// applyItemEnvironment sets the environment of a cycle or session and
// books it when asked; for a run (kind "run") the environment is set by
// the run handlers and only the booking is made here.
func (s *Server) applyItemEnvironment(r *http.Request, p *Principal, kind, id, projectID string, in itemEnvReq) error {
	ctx := r.Context()
	envID, err := s.resolveEnvironmentRef(ctx, p.TenantID(), projectID, in.EnvironmentID)
	if err != nil {
		return err
	}
	if kind != "run" && in.EnvironmentID != nil {
		if err := s.Store.SetItemEnvironment(ctx, p.TenantID(), kind, id, envID); err != nil {
			return err
		}
	}
	if in.Booking == nil {
		return nil
	}
	if envID == nil {
		if envID, err = s.Store.ItemEnvironment(ctx, p.TenantID(), kind, id); err != nil {
			return err
		}
		if envID == nil {
			return badRequest("select an environment to book")
		}
	}
	e, err := s.Store.GetEnvironment(ctx, p.TenantID(), *envID)
	if err != nil {
		return err
	}
	b := *in.Booking
	pid := projectID
	b.ProjectID = &pid
	link := id
	switch kind {
	case "run":
		b.RunID = &link
	case "cycle":
		b.CycleID = &link
	default:
		b.SessionID = &link
	}
	_, err = s.bookEnvironment(r, p, e, &b)
	return err
}

// itemBookings lists the bookings of a run, cycle or session.
func (s *Server) handleItemBookings(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		list, err := s.Store.ItemBookings(r.Context(), p.TenantID(), kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(s.redactBookings(r.Context(), p, list)))
	}
}
