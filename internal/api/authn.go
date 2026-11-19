package api

import (
	"context"
	"crypto/subtle"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
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
	User     *model.User
	Tenant   *model.Tenant
	Role     string
	Session  *model.Session
	Token    *model.APIToken
}

// Can reports whether the principal holds permission p in its tenant.
func (p *Principal) Can(perm rbac.Permission) bool {
	if p.User.SuperAdmin && p.Token == nil {
		return true
	}
	if p.Tenant == nil {
		return false
	}
	return rbac.Can(p.Role, perm)
}

// Permissions lists the effective permissions.
func (p *Principal) Permissions() []string {
	if p.User.SuperAdmin && p.Token == nil {
		return rbac.AllPermissions()
	}
	if p.Tenant == nil {
		return []string{}
	}
	return rbac.PermissionsOf(p.Role)
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
		role := t.Role
		m, err := s.Store.GetMembership(ctx, t.TenantID, u.ID)
		if err != nil && !u.SuperAdmin {
			return nil, err
		}
		if m != nil && rbac.Rank(m.Role) < rbac.Rank(role) {
			role = m.Role
		}
		return &Principal{User: u, Tenant: ten, Role: role, Token: t}, nil
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
	if sess.TenantID != "" {
		if t, err := s.Store.GetTenant(ctx, sess.TenantID); err == nil {
			if m, err := s.Store.GetMembership(ctx, t.ID, u.ID); err == nil {
				p.Tenant, p.Role = t, m.Role
			} else if u.SuperAdmin {
				p.Tenant, p.Role = t, rbac.RoleAdmin
			}
		}
	}
	return p, nil
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

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *model.User, tenantID string) (*model.Session, error) {
	ctx := r.Context()
	if tenantID == "" {
		ms, err := s.Store.ListUserMemberships(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		if len(ms) > 0 {
			tenantID = ms[0].TenantID
		}
	}
	tok := auth.RandomToken(32)
	sess := &model.Session{
		ID: auth.HashToken(tok), UserID: u.ID, TenantID: tenantID,
		CSRF: auth.RandomToken(24), ExpiresAt: time.Now().Add(s.Cfg.Auth.SessionTTL),
	}
	if err := s.Store.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode,
		Expires: sess.ExpiresAt,
	})
	return sess, nil
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
}

type meResponse struct {
	User        *model.User        `json:"user"`
	Tenant      *model.Tenant      `json:"tenant"`
	Role        string             `json:"role"`
	Permissions []string           `json:"permissions"`
	Memberships []model.Membership `json:"memberships"`
	CSRFToken   string             `json:"csrf_token"`
}

func (s *Server) me(ctx context.Context, p *Principal) (*meResponse, error) {
	ms, err := s.Store.ListUserMemberships(ctx, p.User.ID)
	if err != nil {
		return nil, err
	}
	res := &meResponse{User: p.User, Tenant: p.Tenant, Role: p.Role, Permissions: p.Permissions(), Memberships: ms}
	if p.Session != nil {
		res.CSRFToken = p.Session.CSRF
	}
	return res, nil
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
	res := struct {
		Local bool           `json:"local"`
		OIDC  []providerInfo `json:"oidc"`
		SAML  []providerInfo `json:"saml"`
	}{Local: s.Cfg.Auth.Local, OIDC: []providerInfo{}, SAML: []providerInfo{}}
	for _, o := range s.Cfg.Auth.OIDC {
		res.OIDC = append(res.OIDC, providerInfo{o.Name, displayName(o.DisplayName, o.Name), "/api/v1/auth/oidc/" + url.PathEscape(o.Name) + "/login"})
	}
	for _, sc := range s.Cfg.Auth.SAML {
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
	if !s.Cfg.Auth.Local {
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
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	s.limiter.reset(key)
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
	s.completeLogin(w, r, u, tenantID, "local")
}

func (s *Server) completeLogin(w http.ResponseWriter, r *http.Request, u *model.User, tenantID, method string) {
	sess, err := s.startSession(w, r, u, tenantID)
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
	s.audit(r.Context(), p, "login", "user", u.ID, method)
	if method != "local" {
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
		p.Role = m.Role
	} else if u.SuperAdmin {
		p.Role = rbac.RoleAdmin
	}
	return p, nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.Store.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	s.clearCookie(w, sessionCookie)
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
	role := ""
	if m, err := s.Store.GetMembership(ctx, t.ID, p.User.ID); err == nil {
		role = m.Role
	} else if p.User.SuperAdmin {
		role = rbac.RoleAdmin
	} else {
		writeError(w, http.StatusForbidden, "forbidden", "not a member of tenant")
		return
	}
	if err := s.Store.SetSessionTenant(ctx, p.Session.ID, t.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	p.Tenant, p.Role = t, role
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
	if err := auth.ValidatePassword(req.New); err != nil {
		s.fail(w, r, err)
		return
	}
	hash, err := auth.HashPassword(req.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p.User.PasswordHash = hash
	if err := s.Store.UpdateUser(r.Context(), p.User); err != nil {
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
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	return nil
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request, v any) error {
	c, err := r.Cookie(stateCookie)
	if err != nil {
		return auth.ErrBadState
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/api/v1/auth/", MaxAge: -1, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
	return s.signer.Verify(c.Value, v)
}

func (s *Server) loginError(w http.ResponseWriter, r *http.Request, provider string, err error) {
	s.Log.Warn("external login failed", "provider", provider, "err", err, "remote", s.clientIP(r))
	http.Redirect(w, r, "/login?error="+url.QueryEscape("Sign-in failed"), http.StatusFound)
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.OIDC[r.PathValue("name")]
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
	p, ok := s.OIDC[r.PathValue("name")]
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
	s.externalLogin(w, r, id, p.Cfg.Provisioning)
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
	u, tenantID, err := s.provision(r.Context(), id, prov)
	if err != nil {
		s.loginError(w, r, id.Provider, err)
		return
	}
	s.completeLogin(w, r, u, tenantID, id.Provider)
}

type provisioningCfg = config.Provisioning

func (s *Server) provision(ctx context.Context, id *auth.Identity, prov provisioningCfg) (*model.User, string, error) {
	u, err := s.Store.GetUserByExternal(ctx, id.Provider, id.Subject)
	if errors.Is(err, store.ErrNotFound) {
		u, err = s.Store.GetUserByEmail(ctx, id.Email)
		if err == nil {
			// Link an existing account only if it has no other external binding.
			if u.AuthProvider != "local" && u.AuthProvider != id.Provider {
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
			if err := s.Store.CreateUser(ctx, u); err != nil {
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
		def := prov.DefaultRole
		if !rbac.ValidRole(def) {
			def = rbac.RoleViewer
		}
		role := auth.MapRole(id.Groups, prov.RoleMapping, def, rbac.Rank)
		existing, err := s.Store.GetMembership(ctx, t.ID, u.ID)
		switch {
		case err != nil && prov.AutoProvision:
			err = s.Store.SetMembership(ctx, t.ID, u.ID, role)
		case err == nil && len(prov.RoleMapping) > 0 && existing.Role != role:
			// group mapping is authoritative when configured
			err = s.Store.SetMembership(ctx, t.ID, u.ID, role)
		default:
			err = nil
		}
		if err != nil {
			return nil, "", err
		}
	}
	return u, tenantID, nil
}
