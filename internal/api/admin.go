package api

import (
	"context"
	"errors"
	"github.com/rom/qtestframe/internal/export"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/prompt"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqlang"
	"github.com/rom/qtestframe/internal/store"
)

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request, p *Principal) {
	roles := rbac.Roles
	if p.Tenant != nil {
		if set, err := s.Store.RoleSet(r.Context(), p.TenantID()); err == nil {
			roles = set.Names()
		}
	}
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
		"roles":                roles,
		"runner_enabled":       s.Runner.Enabled(),
		"web_tests_enabled":    s.Runner.WebEnabled(),
		"auto_create_test":     s.conf().Requirements.AutoCreateTest,
		"max_upload_mb":        s.conf().Files.MaxUploadMB,
		"analysis_languages":   reqlang.Languages(),
		"themes":               model.Themes,
		"project_types":        model.ProjectTypes,
		"prompt_kinds":         prompt.Kinds,
		"test_types":           prompt.TestTypes,
		"languages":            model.Languages,
		"email_enabled":        s.emailOn(),
		"live_updates":         s.liveOn(),
		"email_digest_hour":    s.conf().Email.DigestHour,
		"token_max_days":       s.conf().Auth.TokenMaxDays,
		"token_require_expiry": s.conf().Auth.TokenRequireExpiry,
		"version":              Version,
	})
}

func (s *Server) handleRBAC(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Tenant == nil {
		writeJSON(w, http.StatusOK, map[string]any{"roles": rbac.Roles, "matrix": rbac.Matrix(), "permissions": rbac.AllPermissionList})
		return
	}
	set, err := s.Store.RoleSet(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": set.Names(), "matrix": set.Matrix(), "permissions": rbac.AllPermissionList})
}

// validRole reports whether role exists in the tenant.
func (s *Server) validRole(ctx context.Context, tenantID, role string) (bool, error) {
	set, err := s.Store.RoleSet(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return set.Valid(role), nil
}

// errOr returns err when set, else fallback.
func errOr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
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
	on, err := s.Store.MFAEnabledUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range us {
		us[i].MFAEnabled = on[us[i].ID]
	}
	writeJSON(w, http.StatusOK, listOf(us))
}

type userReq struct {
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name"`
	Password    *string `json:"password"`
	SuperAdmin  *bool   `json:"super_admin"`
	Disabled    *bool   `json:"disabled"`
	// MustChangePassword forces a password change at next sign-in.
	MustChangePassword *bool `json:"must_change_password"`
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
	if err := applyUserReq(u, req, pwPolicy(s.conf())); err != nil {
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

func applyUserReq(u *model.User, req userReq, pol auth.PasswordPolicy) error {
	if req.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.SuperAdmin != nil {
		u.SuperAdmin = *req.SuperAdmin
	}
	if req.Disabled != nil {
		u.Disabled = *req.Disabled
	}
	if req.MustChangePassword != nil {
		u.MustChangePassword = *req.MustChangePassword
	}
	if req.Password != nil && *req.Password != "" {
		if err := pol.Validate(*req.Password); err != nil {
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
	demote := (req.SuperAdmin != nil && !*req.SuperAdmin) || (req.Disabled != nil && *req.Disabled)
	if u.ID == p.User.ID && demote {
		writeError(w, http.StatusConflict, "conflict", "you cannot demote or disable yourself")
		return
	}
	// there is always an enabled super administrator
	if u.SuperAdmin && !u.Disabled && demote {
		n, err := s.Store.CountActiveSuperAdmins(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if n <= 1 {
			writeError(w, http.StatusConflict, "last_super_admin", "this is the last enabled super administrator: make another user super administrator first")
			return
		}
	}
	if err := applyUserReq(u, req, pwPolicy(s.conf())); err != nil {
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

// memberView is a member with their open invitation, if any.
type memberView struct {
	model.Membership
	Invitation *invitationView `json:"invitation,omitempty"`
	// MFARequired: the server or tenant MFA policy requires a second factor
	// for the member's role in this tenant (or for everyone).
	MFARequired bool `json:"mfa_required"`
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	ms, err := s.Store.ListTenantMembers(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	invs, err := s.Store.OpenInvitations(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pols, err := s.Store.MFAPolicies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	server, tenant := pols[""], pols[p.TenantID()]
	out := make([]memberView, len(ms))
	for i, m := range ms {
		out[i].Membership = m
		if inv, ok := invs[m.UserID]; ok {
			out[i].Invitation = s.viewInvitation(&inv)
		}
		out[i].MFARequired = (server != nil && (server.Requires("") || server.RequiresAny(m.Roles))) || (tenant != nil && tenant.RequiresAny(m.Roles))
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

// handleAddMember: POST /members {"email", "roles" (or "role"),
// "display_name"?, "password"?, "invite"?, "sso"?}. With "invite", a
// person without an account (or who never used it) gets an invitation
// e-mail instead of an initial password; an existing user is told by
// e-mail that they were added. Adding someone who is already a member
// gives them the requested roles.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Email       string   `json:"email"`
		Role        string   `json:"role"`
		Roles       []string `json:"roles"`
		DisplayName string   `json:"display_name"`
		Password    string   `json:"password"`
		Invite      bool     `json:"invite"`
		SSO         bool     `json:"sso"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	roles := requestedRoles(req.Role, req.Roles)
	if err := s.validRoles(r.Context(), p.TenantID(), roles); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Invite && !s.emailOn() {
		s.fail(w, r, errEmailOff())
		return
	}
	if req.Invite && req.Password != "" {
		s.fail(w, r, badRequest("an invitation lets the person choose the password: leave the password empty"))
		return
	}
	if req.Invite && mail.CheckAddress(store.NormalizeEmail(req.Email)) != nil {
		s.fail(w, r, badRequest("a valid e-mail address is required"))
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByEmail(ctx, req.Email)
	if errors.Is(err, store.ErrNotFound) {
		u = &model.User{Email: req.Email, DisplayName: req.DisplayName}
		if req.Password != "" {
			pw := req.Password
			if err := applyUserReq(u, userReq{Password: &pw}, pwPolicy(s.conf())); err != nil {
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
	if s.scimManaged(w, r, p, u.ID) {
		return
	}
	if _, err := s.Store.GetMembership(ctx, p.TenantID(), u.ID); err == nil && !s.lastAdminGuard(w, r, p, u.ID, roles) {
		return
	}
	if err := s.Store.SetMembershipRoles(ctx, p.TenantID(), u.ID, roles); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "member.add", "user", u.ID, u.Email+" as "+rolesText(roles))
	m, err := s.Store.GetMembership(ctx, p.TenantID(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m.Email, m.Name = u.Email, u.DisplayName
	out := memberView{Membership: *m}
	if req.Invite {
		if activated(u) {
			s.sendAddedNotice(ctx, p, u, roles)
		} else {
			inv, err := s.invite(ctx, p, u, roles, req.SSO)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "invitation.send", "user", u.ID, u.Email+" as "+rolesText(roles))
			out.Invitation = s.viewInvitation(inv)
		}
	}
	writeJSON(w, http.StatusCreated, out)
}

// lastAdminGuard answers 409 and returns false when giving the member uid
// the roles next (none: removing the member) would leave the tenant
// without an admin: the member holds admin, next does not, and nobody else
// holds admin.
func (s *Server) lastAdminGuard(w http.ResponseWriter, r *http.Request, p *Principal, uid string, next []string) bool {
	m, err := s.Store.GetMembership(r.Context(), p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if rbac.HasRole(m.Roles, rbac.RoleAdmin) && !rbac.HasRole(next, rbac.RoleAdmin) {
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

// scimManaged answers 409 and returns true when SCIM provisioning manages
// the user's membership: changes by hand would be overwritten at the next
// synchronisation, so they are made in the identity provider.
func (s *Server) scimManaged(w http.ResponseWriter, r *http.Request, p *Principal, uid string) bool {
	managed, err := s.Store.IsSCIMUser(r.Context(), p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return true
	}
	if managed {
		writeError(w, http.StatusConflict, "scim_managed", "this member is provisioned by SCIM: change the role or remove the user in the identity provider (group assignment), or map the groups under Administration → Provisioning (SCIM)")
	}
	return managed
}

// handleUpdateMember: PATCH /members/{uid} {"roles"} (or {"role"}) gives a
// member exactly these roles.
func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Role  string   `json:"role"`
		Roles []string `json:"roles"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	roles := requestedRoles(req.Role, req.Roles)
	if err := s.validRoles(ctx, p.TenantID(), roles); err != nil {
		s.fail(w, r, err)
		return
	}
	uid := r.PathValue("uid")
	if s.scimManaged(w, r, p, uid) {
		return
	}
	before, err := s.Store.GetMembership(ctx, p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.lastAdminGuard(w, r, p, uid, roles) {
		return
	}
	if err := s.Store.SetMembershipRoles(ctx, p.TenantID(), uid, roles); err != nil {
		s.fail(w, r, err)
		return
	}
	if change := rbac.DescribeChange(before.Roles, roles); change != "" {
		s.audit(ctx, p, "member.role", "user", uid, change+" → "+rolesText(roles))
	}
	m, err := s.Store.GetMembership(ctx, p.TenantID(), uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	uid := r.PathValue("uid")
	if s.scimManaged(w, r, p, uid) {
		return
	}
	if !s.lastAdminGuard(w, r, p, uid, nil) {
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
		Name        string   `json:"name"`
		Role        string   `json:"role"`
		Roles       []string `json:"roles"`
		ExpiresDays int      `json:"expires_days"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		s.fail(w, r, badRequest("token name is required"))
		return
	}
	// a token without roles gets all of the caller's roles
	roles := requestedRoles(req.Role, req.Roles)
	if len(roles) == 0 {
		roles = p.roleList()
	}
	set, err := s.Store.RoleSet(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ok, _ := set.ValidAll(roles); !ok || !rbac.Subset(set.PermissionsOfAll(roles), p.Permissions()) {
		s.fail(w, r, badRequest("roles must be valid and not exceed your own permissions"))
		return
	}
	if err := tokenPolicy(s.conf(), req.ExpiresDays); err != nil {
		s.fail(w, r, err)
		return
	}
	tok, prefix := auth.NewAPIToken()
	t := &model.APIToken{TenantID: p.TenantID(), UserID: p.User.ID, Name: strings.TrimSpace(req.Name), Roles: roles,
		Hash: auth.HashToken(tok), Prefix: prefix, ExpiresAt: s.Store.ExpiryIn(req.ExpiresDays)}
	if err := s.Store.CreateToken(r.Context(), t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "token.create", "token", t.ID, t.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "item": t})
}

// tokenPolicy checks the lifetime of a new API token against
// auth.token_require_expiry and auth.token_max_days (days <= 0: no expiry).
func tokenPolicy(c *config.Config, days int) error {
	switch {
	case days <= 0 && (c.Auth.TokenRequireExpiry || c.Auth.TokenMaxDays > 0):
		if c.Auth.TokenMaxDays > 0 {
			return badRequest("tokens must have an expiry date on this server, at most %d days ahead", c.Auth.TokenMaxDays)
		}
		return badRequest("tokens must have an expiry date on this server")
	case c.Auth.TokenMaxDays > 0 && days > c.Auth.TokenMaxDays:
		return badRequest("tokens may be valid for at most %d days on this server", c.Auth.TokenMaxDays)
	}
	return nil
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

// parseDay accepts RFC 3339 or YYYY-MM-DD; endOfDay moves a bare date to
// the end of that day (for "to").
func parseDay(v string, endOfDay bool) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, badRequest("invalid date %q (use YYYY-MM-DD or RFC 3339)", v)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return &t, nil
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, p *Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q, err := query.Parse(r.URL.Query().Get("q"), auditSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := parseDay(r.URL.Query().Get("from"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to, err := parseDay(r.URL.Query().Get("to"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tenantID := p.TenantID()
	// super administrators can read the audit trail of any organisation
	if t := r.URL.Query().Get("tenant"); t != "" && t != tenantID {
		if !p.User.SuperAdmin || p.Token != nil {
			writeError(w, http.StatusForbidden, "forbidden", "super administrator required for another organisation's audit trail")
			return
		}
		if _, err := s.Store.GetTenant(r.Context(), t); err != nil {
			s.fail(w, r, err)
			return
		}
		tenantID = t
	}
	es, err := s.Store.ListAuditRange(r.Context(), tenantID, from, to, limit)
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
	if format := r.URL.Query().Get("format"); format != "" && format != "json" {
		f, err := export.ParseFormat(format)
		if err != nil || (f != export.CSV && f != export.MD) {
			s.fail(w, r, badRequest("unsupported audit export format %q (want csv or md)", format))
			return
		}
		ds := export.Dataset{Title: "Audit trail", GeneratedAt: time.Now().UTC(), Sections: []export.Section{{Name: "audit", Title: "Entries",
			Columns: []string{"Time (UTC)", "User", "Action", "Entity", "Entity ID", "Details"}}}}
		day := func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.UTC().Format("2006-01-02 15:04:05")
		}
		ds.Meta = []export.Item{{Label: "From", Value: day(from)}, {Label: "To", Value: day(to)}, {Label: "Filter", Value: r.URL.Query().Get("q")},
			{Label: "Entries", Value: itoa(len(out))}}
		for _, e := range out {
			ds.Sections[0].Rows = append(ds.Sections[0].Rows, []string{ts(e.CreatedAt), e.UserEmail, e.Action, e.EntityType, e.EntityID, e.Details})
		}
		w.Header().Set("Content-Type", f.ContentType())
		w.Header().Set("Content-Disposition", `attachment; filename="audit-`+time.Now().UTC().Format("20060102-150405")+"."+f.Extension(false)+`"`)
		if err := export.Write(w, f, ds); err != nil {
			s.Log.Error("audit export failed", "err", err)
		}
		s.audit(r.Context(), p, "audit.export", "audit", "", itoa(len(out)))
		return
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

// handleResetPassword sets a random temporary password for a tenant member.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	uid := r.PathValue("uid")
	if _, err := s.Store.GetMembership(ctx, p.TenantID(), uid); err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.Store.GetUser(ctx, uid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if u.AuthProvider != "local" {
		s.fail(w, r, badRequest("the password of %s is managed by the identity provider", u.Email))
		return
	}
	isSuper := p.User.SuperAdmin && p.Token == nil
	if !isSuper {
		// a tenant admin must not take over accounts that reach beyond the tenant
		ms, err := s.Store.ListUserMemberships(ctx, uid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if u.SuperAdmin || len(ms) != 1 {
			writeError(w, http.StatusForbidden, "forbidden", "only a super administrator can reset the password of this user")
			return
		}
	}
	if u.ID == p.User.ID {
		s.fail(w, r, badRequest("use your profile to change your own password"))
		return
	}
	temp := auth.RandomToken(12) + "-9"
	if u.PasswordHash, err = auth.HashPassword(temp); err != nil {
		s.fail(w, r, err)
		return
	}
	u.MustChangePassword = true
	if err := s.Store.UpdateUser(ctx, u); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.Store.DeleteUserSessions(ctx, u.ID)
	s.audit(ctx, p, "member.reset_password", "user", u.ID, u.Email)
	writeJSON(w, http.StatusOK, map[string]string{"temporary_password": temp})
}
