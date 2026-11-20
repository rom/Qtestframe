package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
)

// Administration of SCIM provisioning for tenant admins (tenant.manage):
// SCIM tokens, the default role, the group → role mapping and the recent
// provisioning events.

func (s *Server) routesSCIMAdmin(h func(string, handlerFunc, ...string)) {
	h("GET /api/v1/scim", s.handleSCIMOverview, "tenant.manage")
	h("PATCH /api/v1/scim/settings", s.atomic(s.handleSCIMSettings), "tenant.manage")
	h("POST /api/v1/scim/tokens", s.handleCreateSCIMToken, "tenant.manage")
	h("DELETE /api/v1/scim/tokens/{id}", s.handleDeleteSCIMToken, "tenant.manage")
	h("PATCH /api/v1/scim/groups/{id}", s.atomic(s.handleSCIMGroupRole), "tenant.manage")
	h("GET /api/v1/scim/events", s.handleSCIMEvents, "tenant.manage")
}

type scimGroupInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	ExternalID  string `json:"external_id"`
	Role        string `json:"role"`
	Members     int    `json:"members"`
}

func (s *Server) handleSCIMOverview(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cfg, err := s.Store.SCIMSettings(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tokens, err := s.Store.ListSCIMTokens(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	groups, err := s.Store.ListSCIMGroups(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	users, err := s.Store.ListSCIMUsers(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	active := 0
	for _, u := range users {
		if u.Active {
			active++
		}
	}
	gi := []scimGroupInfo{}
	for _, g := range groups {
		gi = append(gi, scimGroupInfo{ID: g.ID, DisplayName: g.DisplayName, ExternalID: g.ExternalID, Role: g.Role, Members: len(g.Members)})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url": s.scimBase(r), "default_role": cfg.DefaultRole,
		"tokens": tokens, "groups": gi, "users": len(users), "active_users": active,
	})
}

// failSCIMAdmin answers errors of the admin endpoints (the last-admin rule
// as 409).
func (s *Server) failSCIMAdmin(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errLastAdmin) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	s.fail(w, r, err)
}

// resyncSCIM brings every provisioned user of the tenant in line with the
// current mapping.
func (s *Server) resyncSCIM(r *http.Request, tenantID string) error {
	users, err := s.Store.ListSCIMUsers(r.Context(), tenantID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.UserID)
	}
	return s.scimSyncAll(r.Context(), tenantID, ids)
}

func (s *Server) handleSCIMSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req model.SCIMSettings
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if req.DefaultRole != "" {
		if ok, err := s.validRole(ctx, p.TenantID(), req.DefaultRole); err != nil || !ok {
			s.fail(w, r, errOr(err, badRequest("invalid role %q", req.DefaultRole)))
			return
		}
	}
	if err := s.Store.SaveSCIMSettings(ctx, p.TenantID(), &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.resyncSCIM(r, p.TenantID()); err != nil {
		s.failSCIMAdmin(w, r, err)
		return
	}
	s.audit(ctx, p, "scim.settings", "tenant", p.TenantID(), "default role "+orNone(req.DefaultRole))
	writeJSON(w, http.StatusOK, req)
}

func orNone(role string) string {
	if role == "" {
		return "(no access)"
	}
	return role
}

func (s *Server) handleSCIMGroupRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Role string `json:"role"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if req.Role != "" {
		if ok, err := s.validRole(ctx, p.TenantID(), req.Role); err != nil || !ok {
			s.fail(w, r, errOr(err, badRequest("invalid role %q", req.Role)))
			return
		}
	}
	g, err := s.Store.GetSCIMGroup(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	g.Role = req.Role
	if err := s.Store.UpdateSCIMGroup(ctx, g, nil); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.scimSyncAll(ctx, p.TenantID(), g.Members); err != nil {
		s.failSCIMAdmin(w, r, err)
		return
	}
	s.audit(ctx, p, "scim.group.role", "group", g.ID, g.DisplayName+" → "+orNone(req.Role))
	writeJSON(w, http.StatusOK, scimGroupInfo{ID: g.ID, DisplayName: g.DisplayName, ExternalID: g.ExternalID, Role: g.Role, Members: len(g.Members)})
}

// NewSCIMToken returns a new SCIM token and its display prefix.
func NewSCIMToken() (token, prefix string) {
	token = SCIMTokenPrefix + auth.RandomToken(32)
	return token, token[:len(SCIMTokenPrefix)+6]
}

func (s *Server) handleCreateSCIMToken(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "API tokens cannot create SCIM tokens")
		return
	}
	var req struct {
		Name        string `json:"name"`
		ExpiresDays int    `json:"expires_days"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.ExpiresDays < 0 || req.ExpiresDays > 3650 {
		s.fail(w, r, badRequest("expires_days must be between 1 and 3650 (0 = never)"))
		return
	}
	tok, prefix := NewSCIMToken()
	t := &model.SCIMToken{TenantID: p.TenantID(), Name: strings.TrimSpace(req.Name), Hash: auth.HashToken(tok), Prefix: prefix,
		CreatedBy: p.User.Email, ExpiresAt: s.Store.ExpiryIn(req.ExpiresDays)}
	if err := s.Store.CreateSCIMToken(r.Context(), t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "scim.token.create", "scim_token", t.ID, t.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "item": t})
}

func (s *Server) handleDeleteSCIMToken(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.DeleteSCIMToken(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "scim.token.delete", "scim_token", t.ID, t.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSCIMEvents(w http.ResponseWriter, r *http.Request, p *Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	es, err := s.Store.ListAuditByPrefix(r.Context(), p.TenantID(), "scim.", limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(es))
}
