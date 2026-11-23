package api

// Administration of organisations and accounts by super administrators
// (Administration → Organisations / Users): usage, the policy of an
// organisation (quotas, AI, sign-in methods, e-mail domains), suspension,
// the memberships of an account and signing it out everywhere. Super
// administrators cannot impersonate a user: they act under their own name
// and every action is in the audit trail.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

func (s *Server) routesSuperAdmin(h routeFunc) {
	h("GET /api/v1/admin/tenants", s.handleAdminTenants)
	h("GET /api/v1/admin/tenants/{id}", s.handleAdminTenant)
	h("PUT /api/v1/admin/tenants/{id}/policy", s.handleTenantPolicy)
	h("POST /api/v1/admin/tenants/{id}/suspend", s.handleSuspendTenant(true))
	h("POST /api/v1/admin/tenants/{id}/reactivate", s.handleSuspendTenant(false))
	h("GET /api/v1/users/{id}/memberships", s.handleUserMemberships)
	h("POST /api/v1/users/{id}/sessions/revoke", s.handleRevokeUserSessions)
}

// adminTenant is an organisation as super administrators see it.
type adminTenant struct {
	model.Tenant
	Policy model.TenantPolicy `json:"policy"`
	Usage  model.TenantUsage  `json:"usage"`
	// Settings the organisation's administrators control themselves
	// (shown for information; changed inside the organisation).
	AIEnabled       bool `json:"ai_enabled"`
	ERecordsEnabled bool `json:"erecords_enabled"`
	MFARequired     bool `json:"mfa_required"`
}

func (s *Server) adminTenantView(r *http.Request, t model.Tenant, mfa map[string]*model.MFAPolicy) (*adminTenant, error) {
	ctx := r.Context()
	v := &adminTenant{Tenant: t}
	var err error
	if v.Policy, err = s.Store.TenantPolicy(ctx, t.ID); err != nil {
		return nil, err
	}
	if v.Usage, err = s.Store.TenantUsage(ctx, t.ID); err != nil {
		return nil, err
	}
	ts, err := s.Store.TenantSettings(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	v.AIEnabled = ts.AIEnabled && !v.Policy.AIDisabled
	er, err := s.Store.ERecordsSettings(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	v.ERecordsEnabled = er.Enabled
	if pol := mfa[t.ID]; pol != nil {
		v.MFARequired = pol.Mode == model.MFAAll || pol.Mode == model.MFARoles
	}
	return v, nil
}

func (s *Server) handleAdminTenants(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	ts, err := s.Store.ListTenants(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	mfa, err := s.Store.MFAPolicies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []adminTenant{}
	for _, t := range ts {
		v, err := s.adminTenantView(r, t, mfa)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, *v)
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleAdminTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	t, err := s.Store.GetTenant(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	mfa, err := s.Store.MFAPolicies(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.adminTenantView(r, *t, mfa)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// handleTenantPolicy: PUT /admin/tenants/{id}/policy replaces the policy.
func (s *Server) handleTenantPolicy(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	old, err := s.Store.TenantPolicy(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req model.TenantPolicy
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	pol, err := s.Store.SetTenantPolicy(ctx, id, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "tenant.policy", "tenant", id, policyDiff(old, pol))
	s.handleAdminTenant(w, r, p)
}

// policyDiff is the audit text of a policy change.
func policyDiff(a, b model.TenantPolicy) string {
	var out []string
	num := func(name string, x, y int) {
		if x != y {
			out = append(out, fmt.Sprintf("%s: %s → %s", name, limitText(x), limitText(y)))
		}
	}
	num("max_members", a.MaxMembers, b.MaxMembers)
	num("max_projects", a.MaxProjects, b.MaxProjects)
	num("max_storage_mb", a.MaxStorageMB, b.MaxStorageMB)
	if a.AIDisabled != b.AIDisabled {
		out = append(out, fmt.Sprintf("ai_disabled: %t → %t", a.AIDisabled, b.AIDisabled))
	}
	list := func(name string, x, y []string) {
		if strings.Join(x, ",") != strings.Join(y, ",") {
			out = append(out, fmt.Sprintf("%s: [%s] → [%s]", name, strings.Join(x, ", "), strings.Join(y, ", ")))
		}
	}
	list("allowed_providers", a.AllowedProviders, b.AllowedProviders)
	list("email_domains", a.EmailDomains, b.EmailDomains)
	if a.Note != b.Note {
		out = append(out, "note changed")
	}
	if len(out) == 0 {
		return "no changes"
	}
	return strings.Join(out, "; ")
}

func limitText(n int) string {
	if n == 0 {
		return "no limit"
	}
	return itoa(n)
}

// handleSuspendTenant: POST /admin/tenants/{id}/suspend and /reactivate.
// The super administrator's own session stays in the organisation.
func (s *Server) handleSuspendTenant(on bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		if !requireSuper(w, p) {
			return
		}
		ctx := r.Context()
		t, err := s.Store.SetTenantSuspended(ctx, r.PathValue("id"), on)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		action := "tenant.reactivate"
		if on {
			action = "tenant.suspend"
		}
		s.audit(ctx, p, action, "tenant", t.ID, t.Slug)
		s.handleAdminTenant(w, r, p)
	}
}

// handleUserMemberships: GET /users/{id}/memberships — the organisations
// of an account with its roles.
func (s *Server) handleUserMemberships(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	u, err := s.Store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ms, err := s.Store.ListUserMemberships(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sessions, err := s.Store.ListUserSessions(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ms, "sessions": len(sessions)})
}

// handleRevokeUserSessions: POST /users/{id}/sessions/revoke signs an
// account out everywhere (its API tokens stay valid; they are revoked by
// their owner or with the account's organisation).
func (s *Server) handleRevokeUserSessions(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUser(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if u.ID == p.User.ID {
		s.fail(w, r, badRequest("use your profile to end your own sessions"))
		return
	}
	n, err := s.Store.DeleteOtherSessions(ctx, u.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "user.revoke_sessions", "user", u.ID, fmt.Sprintf("%s: %d sessions", u.Email, n))
	writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
}
