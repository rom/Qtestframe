package api

import (
	"context"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Cookie names.
const (
	sessionCookie = "qtf_session"
	stateCookie   = "qtf_login_state"
)

// Principal is the authenticated caller.
type Principal struct {
	User   *model.User
	Tenant *model.Tenant
	// Roles are the caller's roles in Tenant (an API token's: its roles
	// that the user still holds the permissions of) and Role the primary
	// one (the first).
	Role    string
	Roles   []string
	Session *model.Session
	Token   *model.APIToken
	// perms are the effective permissions in Tenant, resolved from the
	// tenant's role set (see resolvePerms); nil means the built-in defaults
	// of Roles. Inside a project they are the permissions there (see
	// access.go: scope).
	perms []string
	// roles is the tenant's role set; memberRoles the user's organisation
	// roles (also for API tokens, whose Roles are the token's).
	roles       rbac.Set
	memberRoles []string
	// orgPerms are the organisation-level permissions (perms before a
	// project scoped them); tokenPerms the permissions of an API token's role.
	orgPerms   []string
	tokenPerms []string
	// projectID and access: the project the request is about (see scope).
	projectID string
	access    *model.ProjectAccessInfo
}

// Can reports whether the principal holds permission p in its tenant.
func (p *Principal) Can(perm rbac.Permission) bool {
	if p.User.SuperAdmin && p.Token == nil {
		return true
	}
	if p.Tenant == nil {
		return false
	}
	if p.perms == nil {
		return slices.Contains(rbac.PermissionsOfAll(p.roleList()), string(perm))
	}
	for _, q := range p.perms {
		if q == string(perm) {
			return true
		}
	}
	return false
}

// Permissions lists the effective permissions.
func (p *Principal) Permissions() []string {
	if p.User.SuperAdmin && p.Token == nil {
		return rbac.AllPermissions()
	}
	if p.Tenant == nil {
		return []string{}
	}
	if p.perms == nil {
		return rbac.PermissionsOfAll(p.roleList())
	}
	return append([]string{}, p.perms...)
}

// roleList returns Roles, or [Role] for a principal built with Role only.
func (p *Principal) roleList() []string {
	if len(p.Roles) == 0 && p.Role != "" {
		return []string{p.Role}
	}
	return p.Roles
}

// setRoles sets the principal's roles and primary role.
func (p *Principal) setRoles(roles []string) {
	p.Roles = rbac.NormalizeRoles(roles)
	p.Role = rbac.Primary(p.Roles)
}

// resolvePerms loads the tenant's roles and sets the principal's effective
// permissions: the union of the permissions of its roles. An API token
// gets the permissions of its roles that the user's current roles also
// have.
func (s *Server) resolvePerms(ctx context.Context, p *Principal, memberRoles []string) error {
	if p.Tenant == nil {
		return nil
	}
	set, err := s.Store.RoleSet(ctx, p.Tenant.ID)
	if err != nil {
		return err
	}
	p.setRoles(p.roleList())
	perms := set.PermissionsOfAll(p.Roles)
	p.roles = set
	if p.Token == nil {
		p.memberRoles = p.Roles
	} else {
		p.memberRoles = rbac.NormalizeRoles(memberRoles)
		p.tokenPerms = append([]string{}, perms...)
	}
	if p.Token != nil && len(memberRoles) > 0 {
		mine := set.PermissionsOfAll(memberRoles)
		if !rbac.Subset(perms, mine) {
			p.setRoles(memberRoles) // the token's roles exceed the user's current ones
		}
		perms = rbac.Intersect(perms, mine)
	}
	p.perms = perms
	p.orgPerms = perms
	return nil
}

// TenantID returns the current tenant id or "".
func (p *Principal) TenantID() string {
	if p.Tenant == nil {
		return ""
	}
	return p.Tenant.ID
}

type handlerFunc func(w http.ResponseWriter, r *http.Request, p *Principal)

// authed resolves the principal, enforces CSRF and the given permissions.
func (s *Server) authed(fn handlerFunc, perms ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principal(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if p.Session != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get("X-CSRF-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.Session.CSRF)) != 1 {
				writeError(w, http.StatusForbidden, "forbidden", "missing or invalid CSRF token")
				return
			}
			if !s.sameOrigin(r) {
				writeError(w, http.StatusForbidden, "forbidden", "cross-origin request rejected")
				return
			}
		}
		if p.User.MustChangePassword && p.Token == nil && !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			writeError(w, http.StatusForbidden, "password_change_required", "you must change your password first")
			return
		}
		// the MFA policy requires a second factor this session has to set up first
		if p.Session != nil && p.Session.MFAEnrolRequired && !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			writeError(w, http.StatusForbidden, "mfa_enrolment_required", "you must set up multi-factor authentication first")
			return
		}
		// per-project access: the request's project scopes the permissions
		if !s.scopeRequest(w, r, p) {
			return
		}
		for _, perm := range perms {
			if !p.Can(rbac.Permission(perm)) {
				writeError(w, http.StatusForbidden, "forbidden", "permission "+perm+" required")
				return
			}
		}
		fn(w, r, p)
	})
}

func (s *Server) principal(r *http.Request) (*Principal, error) {
	ctx := r.Context()
	basicTokenAuth(r) // OSLC clients: an API token as the Basic password (oslc.go)
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if !auth.IsAPIToken(tok) {
			return nil, errors.New("bad token")
		}
		t, err := s.Store.GetTokenByHash(ctx, auth.HashToken(tok))
		if err != nil {
			return nil, err
		}
		u, err := s.Store.GetUser(ctx, t.UserID)
		if err != nil || u.Disabled {
			return nil, errors.New("user disabled")
		}
		ten, err := s.Store.GetTenant(ctx, t.TenantID)
		if err != nil {
			return nil, err
		}
		// a token never exceeds the user's current role
		m, err := s.Store.GetMembership(ctx, t.TenantID, u.ID)
		if err != nil && !u.SuperAdmin {
			return nil, err
		}
		p := &Principal{User: u, Tenant: ten, Role: t.Role, Roles: t.Roles, Token: t}
		var memberRoles []string
		if m != nil {
			memberRoles = m.Roles
		}
		if err := s.resolvePerms(ctx, p, memberRoles); err != nil {
			return nil, err
		}
		return p, nil
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, errors.New("no session")
	}
	sess, err := s.Store.GetSession(ctx, auth.HashToken(c.Value))
	if err != nil {
		return nil, err
	}
	u, err := s.Store.GetUser(ctx, sess.UserID)
	if err != nil || u.Disabled {
		return nil, errors.New("user disabled")
	}
	p := &Principal{User: u, Session: sess}
	if err := s.Store.TouchSession(ctx, sess, s.clientIP(r)); err != nil {
		s.Log.Warn("session touch failed", "err", err)
	}
	if sess.TenantID != "" {
		if t, err := s.Store.GetTenant(ctx, sess.TenantID); err == nil {
			if m, err := s.Store.GetMembership(ctx, t.ID, u.ID); err == nil {
				p.Tenant = t
				p.setRoles(m.Roles)
			} else if u.SuperAdmin {
				p.Tenant = t
				p.setRoles([]string{rbac.RoleAdmin})
			}
		}
	}
	if err := s.resolvePerms(ctx, p, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// sameOrigin rejects browser requests whose Origin header names another
// site (defence in depth on top of the CSRF token). Requests without an
// Origin header (non-browser clients) are accepted.
func (s *Server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	ou, err := url.Parse(o)
	if err != nil {
		return false
	}
	if ou.Host == r.Host {
		return true
	}
	if b, err := url.Parse(s.conf().Server.BaseURL); err == nil && b.Host != "" && strings.EqualFold(b.Scheme+"://"+b.Host, ou.Scheme+"://"+ou.Host) {
		return true
	}
	return false
}

// audit records an action of the principal (errors are only logged).
func (s *Server) audit(ctx context.Context, p *Principal, action, entity, id, details string) {
	e := model.AuditEntry{TenantID: p.TenantID(), UserID: p.User.ID, UserEmail: p.User.Email,
		Action: action, EntityType: entity, EntityID: id, Details: details}
	if err := s.Store.Audit(ctx, e); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
	s.Log.Info("audit", "tenant", e.TenantID, "user", e.UserEmail, "action", action, "entity", entity, "id", id)
}

// ---------------------------------------------------------------- sessions

// startSession creates a session and its cookie. enrolMFA restricts the
// session to /api/v1/auth/ until the user has set up a second factor.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *model.User, tenantID string, enrolMFA bool) (*model.Session, error) {
	ctx := r.Context()
	if tenantID == "" {
		ms, err := s.Store.ListUserMemberships(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		if len(ms) > 0 {
			tenantID = ms[0].TenantID
		}
		// the user's preferred tenant, if still valid
		if pref := u.Preferences.DefaultTenant; pref != "" {
			for _, m := range ms {
				if m.TenantID == pref {
					tenantID = pref
				}
			}
			if u.SuperAdmin {
				if _, err := s.Store.GetTenant(ctx, pref); err == nil {
					tenantID = pref
				}
			}
		}
	}
	tok := auth.RandomToken(32)
	sess := &model.Session{
		ID: auth.HashToken(tok), UserID: u.ID, TenantID: tenantID,
		CSRF: auth.RandomToken(24), ExpiresAt: time.Now().Add(s.conf().Auth.SessionTTL),
		IP: s.clientIP(r), UserAgent: r.UserAgent(), MFAEnrolRequired: enrolMFA,
	}
	if err := s.Store.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteLaxMode,
		Expires: sess.ExpiresAt,
	})
	return sess, nil
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
}

type meResponse struct {
	User        *model.User        `json:"user"`
	Tenant      *model.Tenant      `json:"tenant"`
	Role        string             `json:"role"`
	Roles       []string           `json:"roles"`
	Permissions []string           `json:"permissions"`
	Memberships []model.Membership `json:"memberships"`
	CSRFToken   string             `json:"csrf_token"`
	// Sign-in information relative to the current session.
	PreviousLogin *model.LoginEvent `json:"previous_login"`
	FailedSince   int               `json:"failed_logins_since_previous"`
	FirstLogin    bool              `json:"first_login"`
	// MFA is the caller's multi-factor state (sessions only).
	MFA *mfaStatus `json:"mfa,omitempty"`
}

func (s *Server) me(ctx context.Context, p *Principal) (*meResponse, error) {
	ms, err := s.Store.ListUserMemberships(ctx, p.User.ID)
	if err != nil {
		return nil, err
	}
	res := &meResponse{User: p.User, Tenant: p.Tenant, Role: p.Role, Roles: append([]string{}, p.roleList()...), Permissions: p.Permissions(), Memberships: ms}
	if p.Session != nil {
		res.CSRFToken = p.Session.CSRF
		if err := s.loginInfo(ctx, p, res); err != nil {
			return nil, err
		}
		if res.MFA, err = s.mfaStatusOf(ctx, p, ms); err != nil {
			return nil, err
		}
		u := *p.User
		u.MFAEnabled = res.MFA.Enabled
		res.User = &u
	}
	return res, nil
}

// loginInfo fills the previous successful sign-in before the current
// session, failed attempts since then and whether this is the first one.
func (s *Server) loginInfo(ctx context.Context, p *Principal, res *meResponse) error {
	events, err := s.Store.ListLogins(ctx, p.User.ID, 200)
	if err != nil {
		return err
	}
	start := p.Session.CreatedAt
	for i := range events {
		e := &events[i]
		if !e.CreatedAt.Before(start) {
			continue // this session's own sign-in and later ones
		}
		if e.Success {
			res.PreviousLogin = e
			break
		}
		res.FailedSince++
	}
	res.FirstLogin = res.PreviousLogin == nil
	return nil
}

// recordLogin stores a sign-in attempt; failures are only logged.
func (s *Server) recordLogin(r *http.Request, userID, method string, ok bool) {
	e := &model.LoginEvent{UserID: userID, Success: ok, Method: method, IP: s.clientIP(r), UserAgent: r.UserAgent()}
	if err := s.Store.RecordLogin(r.Context(), e); err != nil {
		s.Log.Warn("login event not recorded", "err", err)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, p *Principal) {
	res, err := s.me(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type providerInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	LoginURL    string `json:"login_url"`
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	c := s.conf()
	res := struct {
		Local bool           `json:"local"`
		OIDC  []providerInfo `json:"oidc"`
		SAML  []providerInfo `json:"saml"`
		// PasswordMinLength is the minimum length of local passwords.
		PasswordMinLength int `json:"password_min_length"`
	}{Local: c.Auth.Local, OIDC: []providerInfo{}, SAML: []providerInfo{}, PasswordMinLength: minPassword(c)}
	for _, o := range c.Auth.OIDC {
		if !o.On() {
			continue
		}
		res.OIDC = append(res.OIDC, providerInfo{o.Name, displayName(o.DisplayName, o.Name), "/api/v1/auth/oidc/" + url.PathEscape(o.Name) + "/login"})
	}
	for _, sc := range s.conf().Auth.SAML {
		res.SAML = append(res.SAML, providerInfo{sc.Name, displayName(sc.DisplayName, sc.Name), "/api/v1/auth/saml/" + url.PathEscape(sc.Name) + "/login"})
	}
	writeJSON(w, http.StatusOK, res)
}

func displayName(d, n string) string {
	if d != "" {
		return d
	}
	return n
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.conf().Auth.Local {
		writeError(w, http.StatusForbidden, "forbidden", "local login is disabled")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Tenant   string `json:"tenant"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	key := s.clientIP(r) + "|" + store.NormalizeEmail(req.Email)
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts, try again later")
		return
	}
	u, err := s.Store.GetUserByEmail(ctx, req.Email)
	hash := ""
	if err == nil && u.AuthProvider == "local" {
		hash = u.PasswordHash
	}
	if !auth.CheckPassword(hash, req.Password) || u == nil || u.Disabled {
		s.Log.Warn("login failed", "email", store.NormalizeEmail(req.Email), "remote", s.clientIP(r))
		if u != nil {
			s.recordLogin(r, u.ID, "local", false)
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	tenantID := ""
	if req.Tenant != "" {
		t, err := s.Store.GetTenantBySlug(ctx, req.Tenant)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
			return
		}
		if _, err := s.Store.GetMembership(ctx, t.ID, u.ID); err != nil && !u.SuperAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "not a member of tenant")
			return
		}
		tenantID = t.ID
	}
	// the rate limit of this address and account is reset once the second
	// factor (if any) has been given too
	s.afterFirstFactor(w, r, u, tenantID, firstFactor{Method: "local", LocalMFA: true, JSON: true, LimiterKey: key})
}

// loginOpts are the options of completeLoginOpts.
type loginOpts struct {
	// JSON answers with /auth/me (local and second-factor sign-ins) instead
	// of redirecting to the start page (identity provider callbacks).
	JSON bool
	// Details are added to the audit entry (e.g. "mfa=totp").
	Details string
	// EnrolRequired: the session must set up a second factor first.
	EnrolRequired bool
}

// completeLoginOpts starts the session of a complete sign-in (all factors
// given), records and audits it.
func (s *Server) completeLoginOpts(w http.ResponseWriter, r *http.Request, u *model.User, tenantID, method string, o loginOpts) {
	sess, err := s.startSession(w, r, u, tenantID, o.EnrolRequired)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	r2 := r.WithContext(r.Context())
	p, err := s.principalFromSession(r2, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "login", "user", u.ID, strings.TrimSpace(method+" "+o.Details))
	s.recordLogin(r, u.ID, method, true) // after the session exists: see loginInfo
	s.acceptOnSignIn(r.Context(), p)
	if !o.JSON {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	res, err := s.me(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) principalFromSession(r *http.Request, u *model.User, sess *model.Session) (*Principal, error) {
	p := &Principal{User: u, Session: sess}
	if sess.TenantID == "" {
		return p, nil
	}
	t, err := s.Store.GetTenant(r.Context(), sess.TenantID)
	if err != nil {
		return nil, err
	}
	p.Tenant = t
	if m, err := s.Store.GetMembership(r.Context(), t.ID, u.ID); err == nil {
		p.setRoles(m.Roles)
	} else if u.SuperAdmin {
		p.setRoles([]string{rbac.RoleAdmin})
	}
	if err := s.resolvePerms(r.Context(), p, nil); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.Store.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	s.clearCookie(w, sessionCookie)
	// also cancels a sign-in waiting for its second factor
	if ch, err := s.pendingChallenge(r); err == nil {
		_ = s.Store.DeleteMFAChallenge(r.Context(), ch.ID)
	}
	s.clearMFACookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSwitchTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Session == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "tokens are bound to a tenant")
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	t, err := s.Store.GetTenant(ctx, req.TenantID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var roles []string
	if m, err := s.Store.GetMembership(ctx, t.ID, p.User.ID); err == nil {
		roles = m.Roles
	} else if p.User.SuperAdmin {
		roles = []string{rbac.RoleAdmin}
	} else {
		writeError(w, http.StatusForbidden, "forbidden", "not a member of tenant")
		return
	}
	if err := s.Store.SetSessionTenant(ctx, p.Session.ID, t.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	p.Tenant = t
	p.setRoles(roles)
	if err := s.resolvePerms(ctx, p, nil); err != nil {
		s.fail(w, r, err)
		return
	}
	s.handleMe(w, r, p)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if p.User.AuthProvider != "local" {
		writeError(w, http.StatusBadRequest, "bad_request", "password is managed by the identity provider")
		return
	}
	if !auth.CheckPassword(p.User.PasswordHash, req.Current) {
		writeError(w, http.StatusForbidden, "forbidden", "current password is wrong")
		return
	}
	if err := auth.ValidatePasswordMin(req.New, minPassword(s.conf())); err != nil {
		s.fail(w, r, err)
		return
	}
	hash, err := auth.HashPassword(req.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p.User.PasswordHash = hash
	p.User.MustChangePassword = false
	if err := s.Store.UpdateUser(r.Context(), p.User); err != nil {
		s.fail(w, r, err)
		return
	}
	keep := ""
	if p.Session != nil {
		keep = p.Session.ID
	}
	if _, err := s.Store.DeleteOtherSessions(r.Context(), p.User.ID, keep); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "password.change", "user", p.User.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- external IdPs

func (s *Server) setState(w http.ResponseWriter, v any) error {
	val, err := s.signer.Sign(v, 10*time.Minute)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: val, Path: "/api/v1/auth/", HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	return nil
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request, v any) error {
	c, err := r.Cookie(stateCookie)
	if err != nil {
		return auth.ErrBadState
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/api/v1/auth/", MaxAge: -1, HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
	return s.signer.Verify(c.Value, v)
}

func (s *Server) loginError(w http.ResponseWriter, r *http.Request, provider string, err error) {
	s.Log.Warn("external login failed", "provider", provider, "err", err, "remote", s.clientIP(r))
	http.Redirect(w, r, "/login?error="+url.QueryEscape("Sign-in failed"), http.StatusFound)
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.oidcProvider(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	u, st, err := p.AuthURL(r.Context())
	if err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	if err := s.setState(w, st); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	p, ok := s.oidcProvider(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	var st auth.OIDCState
	if err := s.getState(w, r, &st); err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		s.loginError(w, r, p.Cfg.Name, errors.New("idp error: "+e))
		return
	}
	id, err := p.Exchange(r.Context(), st, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	// the identity provider's MFA counts unless local_mfa says otherwise
	idpMFA := p.IdPMFA(id)
	local := false
	switch p.LocalMFA() {
	case config.LocalMFAAlways:
		local = true
	case config.LocalMFAUnlessIdPMFA:
		local = !idpMFA
	}
	details := fmt.Sprintf("idp_mfa=%t", idpMFA)
	if len(id.AMR) > 0 {
		details += " amr=" + strings.Join(id.AMR, ",")
	}
	if id.ACR != "" {
		details += " acr=" + id.ACR
	}
	s.externalLoginMFA(w, r, id, p.Cfg.Provisioning, local, details)
}

func (s *Server) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	p, ok := s.SAML[r.PathValue("name")]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	md, err := p.Metadata(r.Context())
	if err != nil {
		s.Log.Error("saml metadata", "err", err)
		writeError(w, http.StatusBadGateway, "internal", "identity provider metadata unavailable")
		return
	}
	b, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write(b)
}

func (s *Server) handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.SAML[r.PathValue("name")]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	u, st, err := p.AuthURL(r.Context(), "")
	if err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	// The ACS POST is cross-site; SameSite=Lax cookies are not sent on
	// cross-site POSTs, so the SAML state cookie uses SameSite=None.
	val, err := s.signer.Sign(st, 10*time.Minute)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: val, Path: "/api/v1/auth/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteNoneMode, MaxAge: 600})
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	p, ok := s.SAML[r.PathValue("name")]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	var st auth.SAMLState
	if err := s.getState(w, r, &st); err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	id, err := p.ParseResponse(r, st)
	if err != nil {
		s.loginError(w, r, p.Cfg.Name, err)
		return
	}
	s.externalLogin(w, r, id, p.Cfg.Provisioning)
}

// externalLogin maps an external identity to a local user (provisioning
// it when allowed) and starts a session.
func (s *Server) externalLogin(w http.ResponseWriter, r *http.Request, id *auth.Identity, prov provisioningCfg) {
	s.externalLoginMFA(w, r, id, prov, false, "")
}

// externalLoginMFA is externalLogin where Qtestframe's own second factor
// applies when localMFA is set (OIDC local_mfa).
func (s *Server) externalLoginMFA(w http.ResponseWriter, r *http.Request, id *auth.Identity, prov provisioningCfg, localMFA bool, details string) {
	u, tenantID, err := s.provision(r.Context(), id, prov)
	if err != nil {
		s.loginError(w, r, id.Provider, err)
		return
	}
	s.afterFirstFactor(w, r, u, tenantID, firstFactor{Method: id.Provider, Details: details, LocalMFA: localMFA})
}

type provisioningCfg = config.Provisioning

func (s *Server) provision(ctx context.Context, id *auth.Identity, prov provisioningCfg) (*model.User, string, error) {
	u, err := s.Store.GetUserByExternal(ctx, id.Provider, id.Subject)
	if errors.Is(err, store.ErrNotFound) {
		u, err = s.Store.GetUserByEmail(ctx, id.Email)
		if err == nil {
			// Link an existing account only if it has no other external binding.
			// (accounts created by SCIM provisioning wait for this first sign-in)
			if u.AuthProvider != "local" && u.AuthProvider != scimAuthProvider && u.AuthProvider != id.Provider {
				return nil, "", errors.New("e-mail already bound to another identity provider")
			}
			u.AuthProvider, u.ExternalID = id.Provider, id.Subject
			u.PasswordHash = ""
			if err := s.Store.UpdateUser(ctx, u); err != nil {
				return nil, "", err
			}
		} else if errors.Is(err, store.ErrNotFound) {
			if !prov.AutoProvision {
				return nil, "", errors.New("user not provisioned")
			}
			u = &model.User{Email: id.Email, DisplayName: id.Name, AuthProvider: id.Provider, ExternalID: id.Subject}
			if err = s.Store.CreateUser(ctx, u); err != nil {
				return nil, "", err
			}
		}
	}
	if err != nil {
		return nil, "", err
	}
	if u.Disabled {
		return nil, "", errors.New("user disabled")
	}
	tenantID := ""
	if prov.DefaultTenant != "" {
		t, err := s.Store.GetTenantBySlug(ctx, prov.DefaultTenant)
		if err != nil {
			return nil, "", err
		}
		tenantID = t.ID
		set, err := s.Store.RoleSet(ctx, t.ID)
		if err != nil {
			return nil, "", err
		}
		def := prov.DefaultRole
		if !set.Valid(def) {
			def = rbac.RoleViewer
		}
		// every mapped role the tenant has (a deleted custom role is ignored)
		roles := rbac.NormalizeRoles(auth.MapRoles(id.Groups, prov.RoleMapping, def, set.Valid))
		existing, err := s.Store.GetMembership(ctx, t.ID, u.ID)
		managed, serr := s.Store.IsSCIMUser(ctx, t.ID, u.ID)
		if serr != nil {
			return nil, "", serr
		}
		switch {
		case managed:
			// SCIM provisioning owns this membership (and its absence)
			err = nil
		case err != nil && prov.AutoProvision:
			err = s.Store.SetMembershipRoles(ctx, t.ID, u.ID, roles)
		case err == nil && len(prov.RoleMapping) > 0 && !slices.Equal(existing.Roles, roles):
			// group mapping is authoritative when configured
			err = s.Store.SetMembershipRoles(ctx, t.ID, u.ID, roles)
		default:
			err = nil
		}
		if err != nil {
			return nil, "", err
		}
	}
	return u, tenantID, nil
}
