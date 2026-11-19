package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request, p *Principal) {
	writeJSON(w, http.StatusOK, map[string]any{
		"obligations":          model.Obligations,
		"requirement_statuses": model.RequirementStatuses,
		"requirement_types":    model.RequirementTypes,
		"priorities":           model.Priorities,
		"test_statuses":        model.TestStatuses,
		"script_languages":     model.ScriptLanguages,
		"evaluation_methods":   model.EvaluationMethods,
		"run_statuses":         model.RunStatuses,
		"result_statuses":      model.ResultStatuses,
		"roles":                rbac.Roles,
		"runner_enabled":       s.Runner.Enabled(),
		"auto_create_test":     s.Cfg.Requirements.AutoCreateTest,
		"themes":               model.Themes,
		"languages":            model.Languages,
		"version":              Version,
	})
}

func (s *Server) handleRBAC(w http.ResponseWriter, r *http.Request, p *Principal) {
	writeJSON(w, http.StatusOK, map[string]any{"roles": rbac.Roles, "matrix": rbac.Matrix()})
}

func requireSuper(w http.ResponseWriter, p *Principal) bool {
	if !p.User.SuperAdmin || p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "super administrator required")
		return false
	}
	return true
}

// ---------------------------------------------------------------- tenants

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.User.SuperAdmin && p.Token == nil {
		ts, err := s.Store.ListTenants(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(ts))
		return
	}
	ms, err := s.Store.ListUserMemberships(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.Tenant{}
	for _, m := range ms {
		if t, err := s.Store.GetTenant(r.Context(), m.TenantID); err == nil {
			out = append(out, *t)
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct{ Slug, Name string }
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.Store.CreateTenant(r.Context(), req.Slug, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "tenant.create", "tenant", t.ID, t.Slug)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if !(p.User.SuperAdmin && p.Token == nil) && !(p.TenantID() == id && p.Can(rbac.TenantManage)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission tenant.manage required")
		return
	}
	var req struct{ Name string }
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.Store.UpdateTenant(r.Context(), id, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "tenant.update", "tenant", id, req.Name)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	id := r.PathValue("id")
	if err := s.Store.DeleteTenant(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "tenant.delete", "tenant", id, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- users

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	us, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(us))
}

type userReq struct {
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name"`
	Password    *string `json:"password"`
	SuperAdmin  *bool   `json:"super_admin"`
	Disabled    *bool   `json:"disabled"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req userReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	u := &model.User{Email: req.Email}
	if err := applyUserReq(u, req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateUser(r.Context(), u); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "user.create", "user", u.ID, u.Email)
	writeJSON(w, http.StatusCreated, u)
}

func applyUserReq(u *model.User, req userReq) error {
	if req.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.SuperAdmin != nil {
		u.SuperAdmin = *req.SuperAdmin
	}
	if req.Disabled != nil {
		u.Disabled = *req.Disabled
	}
	if req.Password != nil && *req.Password != "" {
		if err := auth.ValidatePassword(*req.Password); err != nil {
			return err
		}
		h, err := auth.HashPassword(*req.Password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	return nil
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUser(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req userReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if u.ID == p.User.ID && ((req.SuperAdmin != nil && !*req.SuperAdmin) || (req.Disabled != nil && *req.Disabled)) {
		writeError(w, http.StatusConflict, "conflict", "you cannot demote or disable yourself")
		return
	}
	if err := applyUserReq(u, req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateUser(ctx, u); err != nil {
		s.fail(w, r, err)
		return
	}
	if u.Disabled || req.Password != nil {
		_ = s.Store.DeleteUserSessions(ctx, u.ID)
	}
	s.audit(ctx, p, "user.update", "user", u.ID, u.Email)
	writeJSON(w, http.StatusOK, u)
}

// ---------------------------------------------------------------- members

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	ms, err := s.Store.ListTenantMembers(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(ms))
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Email       string `json:"email"`
		Role        string `json:"role"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !rbac.ValidRole(req.Role) {
		s.fail(w, r, badRequest("invalid role %q", req.Role))
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByEmail(ctx, req.Email)
	if errors.Is(err, store.ErrNotFound) {
		u = &model.User{Email: req.Email, DisplayName: req.DisplayName}
		if req.Password != "" {
			pw := req.Password
			if err := applyUserReq(u, userReq{Password: &pw}); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		err = s.Store.CreateUser(ctx, u)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetMembership(ctx, p.TenantID(), u.ID, req.Role); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "member.add", "user", u.ID, u.Email+" as "+req.Role)
	m, err := s.Store.GetMembership(ctx, p.TenantID(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m.Email, m.Name = u.Email, u.DisplayName
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) lastAdminGuard(w http.ResponseWriter, r *http.Request, p *Principal, uid string) bool {
	m, err := s.Store.GetMembership(r.Context(), p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if m.Role == rbac.RoleAdmin {
		n, err := s.Store.CountTenantAdmins(r.Context(), p.TenantID())
		if err != nil {
			s.fail(w, r, err)
			return false
		}
		if n <= 1 {
			writeError(w, http.StatusConflict, "conflict", "a tenant needs at least one admin")
			return false
		}
	}
	return true
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct{ Role string }
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !rbac.ValidRole(req.Role) {
		s.fail(w, r, badRequest("invalid role %q", req.Role))
		return
	}
	uid := r.PathValue("uid")
	if req.Role != rbac.RoleAdmin && !s.lastAdminGuard(w, r, p, uid) {
		return
	}
	if err := s.Store.SetMembership(r.Context(), p.TenantID(), uid, req.Role); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "member.role", "user", uid, req.Role)
	m, err := s.Store.GetMembership(r.Context(), p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	uid := r.PathValue("uid")
	if !s.lastAdminGuard(w, r, p, uid) {
		return
	}
	if err := s.Store.RemoveMembership(r.Context(), p.TenantID(), uid); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "member.remove", "user", uid, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- tokens

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request, p *Principal) {
	ts, err := s.Store.ListTokens(r.Context(), p.TenantID(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(ts))
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "tokens cannot create tokens")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		ExpiresDays int    `json:"expires_days"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		s.fail(w, r, badRequest("token name is required"))
		return
	}
	if req.Role == "" {
		req.Role = p.Role
	}
	if !rbac.ValidRole(req.Role) || rbac.Rank(req.Role) > rbac.Rank(p.Role) {
		s.fail(w, r, badRequest("role must be valid and not exceed your own role"))
		return
	}
	tok, prefix := auth.NewAPIToken()
	t := &model.APIToken{TenantID: p.TenantID(), UserID: p.User.ID, Name: strings.TrimSpace(req.Name), Role: req.Role,
		Hash: auth.HashToken(tok), Prefix: prefix, ExpiresAt: s.Store.ExpiryIn(req.ExpiresDays)}
	if err := s.Store.CreateToken(r.Context(), t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "token.create", "token", t.ID, t.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "item": t})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.DeleteToken(r.Context(), p.TenantID(), p.User.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "token.delete", "token", id, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- audit

var auditSchema = &query.Schema{Fields: []string{"action", "entity", "entity_id", "user", "details", "created_at"}}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, p *Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q, err := query.Parse(r.URL.Query().Get("q"), auditSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	es, err := s.Store.ListAudit(r.Context(), p.TenantID(), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.AuditEntry{}
	for _, e := range es {
		rec := query.MapRecord{"action": {e.Action}, "entity": {e.EntityType}, "entity_id": {e.EntityID},
			"user": {e.UserEmail}, "details": {e.Details}, "created_at": {e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}}
		if q.Match(rec) {
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}
