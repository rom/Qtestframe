package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Multi-factor authentication (docs/security.md, "Multi-factor
// authentication"): TOTP authenticator apps with single-use recovery
// codes as the second factor of local (and, if configured, OIDC) sign-ins;
// the MFA policies of the server and of each tenant; administrator resets.
// API tokens are not affected: they are a credential of their own.

const (
	// mfaCookie holds the token of a sign-in that waits for its second factor.
	mfaCookie = "qtf_mfa"
	// mfaDeviceCookie holds the token of a remembered device.
	mfaDeviceCookie = "qtf_mfa_device"
	// mfaChallengeTTL: how long the second factor may take.
	mfaChallengeTTL = 5 * time.Minute
	// mfaMaxAttempts: wrong codes per pending sign-in before it is dropped.
	mfaMaxAttempts = 5
	// mfaIssuer names the account in authenticator apps.
	mfaIssuer = "Qtestframe"
	// mfaCookiePath: the cookies are only sent to the sign-in endpoints.
	mfaCookiePath = "/api/v1/auth/"
)

func (s *Server) routesMFA(h routeFunc) {
	h("GET /api/v1/auth/mfa", s.handleMFAStatus)
	h("POST /api/v1/auth/mfa/totp", s.handleMFASetup)
	h("POST /api/v1/auth/mfa/totp/confirm", s.handleMFAConfirm)
	h("POST /api/v1/auth/mfa/disable", s.handleMFADisable)
	h("POST /api/v1/auth/mfa/recovery-codes", s.handleMFARecoveryCodes)
	h("DELETE /api/v1/auth/mfa/trusted-devices", s.handleMFAForgetDevices)
	// policy (super administrators: the server; tenant.manage: the tenant)
	h("GET /api/v1/admin/mfa", s.handleMFAPolicyGet, "tenant.manage")
	h("PUT /api/v1/admin/mfa/server", s.handleMFAPolicyServer)
	h("PUT /api/v1/admin/mfa/tenant", s.handleMFAPolicyTenant, "tenant.manage")
	// administrator resets
	h("POST /api/v1/members/{uid}/mfa/reset", s.handleResetMemberMFA, "member.manage")
	h("POST /api/v1/users/{id}/mfa/reset", s.handleResetUserMFA)
}

// ---------------------------------------------------------------- policy evaluation

// mfaRequirement is what the policies demand of one user.
type mfaRequirement struct {
	Required bool
	// Deadline: from then on the user must enrol at sign-in (the end of the
	// shortest grace period of the policies that require MFA).
	Deadline *time.Time
	// RememberDays of the server policy (0 = no "remember this device").
	RememberDays int
}

// mfaRequirementFor evaluates the server policy (for every user, for the
// super administrator pseudo-role and for the user's roles in all tenants)
// and the policy of each tenant the user belongs to (for their role there).
// A tenant can only add requirements, never lift the server's.
func (s *Server) mfaRequirementFor(ctx context.Context, u *model.User, ms []model.Membership) (mfaRequirement, error) {
	var req mfaRequirement
	pols, err := s.Store.MFAPolicies(ctx)
	if err != nil {
		return req, err
	}
	server := pols[""]
	if server == nil {
		server = store.DefaultMFAPolicy("")
	}
	req.RememberDays = server.RememberDays
	consider := func(p *model.MFAPolicy, role string) {
		if p == nil || !p.Requires(role) {
			return
		}
		since := u.CreatedAt
		if p.RequiredSince != nil && p.RequiredSince.After(since) {
			since = *p.RequiredSince
		}
		d := since.AddDate(0, 0, p.GraceDays)
		if req.Deadline == nil || d.Before(*req.Deadline) {
			req.Deadline = &d
		}
		req.Required = true
	}
	consider(server, "") // mode "all"
	if u.SuperAdmin {
		consider(server, model.MFASuperAdminRole)
	}
	for _, m := range ms {
		consider(server, m.Role)
		consider(pols[m.TenantID], m.Role)
	}
	return req, nil
}

// enrolDue reports whether the user must enrol now (required, no second
// factor, grace period over).
func (r mfaRequirement) enrolDue(now time.Time, enrolled bool) bool {
	return r.Required && !enrolled && r.Deadline != nil && !now.Before(*r.Deadline)
}

// userMFA returns the user's confirmed enrolment or nil.
func (s *Server) userMFA(ctx context.Context, userID string) (*model.UserMFA, error) {
	m, err := s.Store.GetUserMFA(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if m.EnabledAt == nil {
		return nil, nil
	}
	return m, nil
}

// mfaEnrolDue reports whether a new session of u must enrol a second factor first.
func (s *Server) mfaEnrolDue(ctx context.Context, u *model.User) (bool, error) {
	m, err := s.userMFA(ctx, u.ID)
	if err != nil {
		return false, err
	}
	ms, err := s.Store.ListUserMemberships(ctx, u.ID)
	if err != nil {
		return false, err
	}
	req, err := s.mfaRequirementFor(ctx, u, ms)
	if err != nil {
		return false, err
	}
	return req.enrolDue(s.now(), m != nil), nil
}

// ---------------------------------------------------------------- sign-in

// firstFactor describes a sign-in whose first factor succeeded.
type firstFactor struct {
	Method  string // login method: "local", "oidc:<name>"
	Details string // audit details (e.g. the IdP's amr)
	// LocalMFA: Qtestframe's own second factor applies to this sign-in.
	LocalMFA bool
	// JSON: answer with JSON (local sign-in) rather than a redirect (IdP callback).
	JSON bool
	// LimiterKey is reset once the sign-in is complete.
	LimiterKey string
}

// afterFirstFactor completes a sign-in, asks for the second factor or
// starts a session that must enrol one first.
func (s *Server) afterFirstFactor(w http.ResponseWriter, r *http.Request, u *model.User, tenantID string, f firstFactor) {
	opts := loginOpts{JSON: f.JSON, Details: f.Details}
	done := func() {
		if f.LimiterKey != "" {
			s.limiter.reset(f.LimiterKey)
		}
		s.completeLoginOpts(w, r, u, tenantID, f.Method, opts)
	}
	if !f.LocalMFA {
		done()
		return
	}
	ctx := r.Context()
	m, err := s.userMFA(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ms, err := s.Store.ListUserMemberships(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	req, err := s.mfaRequirementFor(ctx, u, ms)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if m == nil {
		opts.EnrolRequired = req.enrolDue(s.now(), false)
		done()
		return
	}
	if req.RememberDays > 0 && s.trustedDevice(r, u.ID, req.RememberDays) {
		opts.Details = strings.TrimSpace(opts.Details + " mfa=remembered-device")
		done()
		return
	}
	tok := auth.RandomToken(32)
	ch := &model.MFAChallenge{ID: auth.HashToken(tok), UserID: u.ID, TenantID: tenantID, Method: f.Method, Details: f.Details,
		ExpiresAt: s.Store.Now().Add(mfaChallengeTTL)}
	if err := s.Store.CreateMFAChallenge(ctx, ch); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: mfaCookie, Value: tok, Path: mfaCookiePath, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(mfaChallengeTTL / time.Second)})
	if !f.JSON {
		http.Redirect(w, r, "/login?mfa=1", http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, s.challengeView(ch, req.RememberDays))
}

// challengeView answers a sign-in that waits for its second factor.
func (s *Server) challengeView(ch *model.MFAChallenge, rememberDays int) map[string]any {
	return map[string]any{"mfa_required": true, "methods": []string{"totp", "recovery_code"},
		"remember_days": rememberDays, "expires_at": ch.ExpiresAt}
}

// pendingChallenge returns the sign-in of the qtf_mfa cookie.
func (s *Server) pendingChallenge(r *http.Request) (*model.MFAChallenge, error) {
	c, err := r.Cookie(mfaCookie)
	if err != nil || c.Value == "" {
		return nil, store.ErrNotFound
	}
	return s.Store.GetMFAChallenge(r.Context(), auth.HashToken(c.Value))
}

func (s *Server) clearMFACookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: mfaCookie, Value: "", Path: mfaCookiePath, MaxAge: -1, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
}

// trustedDevice reports whether the request carries a remembered device of the user.
func (s *Server) trustedDevice(r *http.Request, userID string, days int) bool {
	c, err := r.Cookie(mfaDeviceCookie)
	if err != nil || c.Value == "" {
		return false
	}
	ok, err := s.Store.TrustedDevice(r.Context(), auth.HashToken(c.Value), userID, time.Duration(days)*24*time.Hour)
	return err == nil && ok
}

// handleMFAChallenge: GET /auth/mfa/challenge (public) describes the
// pending sign-in (after an identity provider's redirect to /login?mfa=1).
func (s *Server) handleMFAChallenge(w http.ResponseWriter, r *http.Request) {
	ch, err := s.pendingChallenge(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "mfa_expired", "no sign-in is waiting for a second factor; sign in again")
		return
	}
	pol, err := s.Store.MFAPolicy(r.Context(), "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.challengeView(ch, pol.RememberDays))
}

// handleMFAVerify: POST /auth/mfa/verify (public) {"code", "remember"}
// checks the second factor of the pending sign-in and starts the session.
// Attempts count against the sign-in rate limit of the address and
// account, against a per-account limit and against the pending sign-in
// (mfaMaxAttempts); failures are recorded as failed sign-ins (method "mfa").
func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Remember bool   `json:"remember"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	ch, err := s.pendingChallenge(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "mfa_expired", "the sign-in has expired; sign in again")
		return
	}
	u, err := s.Store.GetUser(ctx, ch.UserID)
	if err != nil || u.Disabled {
		_ = s.Store.DeleteMFAChallenge(ctx, ch.ID)
		s.clearMFACookie(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	key := s.clientIP(r) + "|" + store.NormalizeEmail(u.Email)
	if !s.limiter.allow(key) || !s.mfaLimiter.allow(u.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	n, err := s.Store.CountMFAAttempt(ctx, ch.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n > mfaMaxAttempts {
		_ = s.Store.DeleteMFAChallenge(ctx, ch.ID)
		s.clearMFACookie(w)
		writeError(w, http.StatusUnauthorized, "mfa_expired", "too many wrong codes; sign in again")
		return
	}
	ok, recovery, err := s.checkSecondFactor(ctx, u.ID, req.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		s.Log.Warn("second factor failed", "email", u.Email, "remote", s.clientIP(r))
		s.recordLogin(r, u.ID, "mfa", false)
		writeError(w, http.StatusUnauthorized, "mfa_invalid", "the code is wrong or was already used")
		return
	}
	_ = s.Store.DeleteMFAChallenge(ctx, ch.ID)
	s.clearMFACookie(w)
	s.limiter.reset(key)
	s.mfaLimiter.reset(u.ID)
	factor := "totp"
	if recovery {
		factor = "recovery-code"
	}
	opts := loginOpts{JSON: true, Details: strings.TrimSpace(ch.Details + " mfa=" + factor)}
	if req.Remember {
		if pol, err := s.Store.MFAPolicy(ctx, ""); err == nil && pol.RememberDays > 0 {
			s.rememberDevice(w, r, u.ID, pol.RememberDays)
		}
	}
	if recovery {
		s.audit(ctx, &Principal{User: u}, "mfa.recovery_code_used", "user", u.ID, u.Email)
	}
	s.completeLoginOpts(w, r, u, ch.TenantID, ch.Method, opts)
}

func (s *Server) rememberDevice(w http.ResponseWriter, r *http.Request, userID string, days int) {
	tok := auth.RandomToken(32)
	exp := s.Store.Now().AddDate(0, 0, days)
	if err := s.Store.AddTrustedDevice(r.Context(), auth.HashToken(tok), userID, s.clientIP(r), r.UserAgent(), exp); err != nil {
		s.Log.Warn("remembered device not stored", "err", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: mfaDeviceCookie, Value: tok, Path: mfaCookiePath, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: exp, MaxAge: days * 24 * 3600})
}

// checkSecondFactor verifies a TOTP code (each step once) or a recovery
// code (each code once) of the user's confirmed enrolment.
func (s *Server) checkSecondFactor(ctx context.Context, userID, code string) (ok, recovery bool, err error) {
	m, err := s.userMFA(ctx, userID)
	if err != nil || m == nil {
		return false, false, err
	}
	if auth.IsRecoveryCode(code) {
		ok, err := s.Store.UseRecoveryCode(ctx, userID, auth.HashRecoveryCode(code))
		return ok, true, err
	}
	step, ok := auth.VerifyTOTP(m.Secret, code, s.now(), m.LastStep)
	if !ok {
		return false, false, nil
	}
	// conditional update: of two requests with the same code only one wins
	ok, err = s.Store.UseTOTPStep(ctx, userID, step)
	return ok, false, err
}

// ---------------------------------------------------------------- self-service

type mfaStatus struct {
	Enabled            bool       `json:"enabled"`
	EnabledAt          *time.Time `json:"enabled_at"`
	Pending            bool       `json:"pending"`
	RecoveryCodesLeft  int        `json:"recovery_codes_left"`
	RecoveryCodesTotal int        `json:"recovery_codes_total"`
	TrustedDevices     int        `json:"trusted_devices"`
	Required           bool       `json:"required"`
	EnrolmentDeadline  *time.Time `json:"enrolment_deadline"`
	EnrolmentRequired  bool       `json:"enrolment_required"`
	RememberDays       int        `json:"remember_days"`
}

func (s *Server) mfaStatusOf(ctx context.Context, p *Principal, ms []model.Membership) (*mfaStatus, error) {
	st := &mfaStatus{}
	m, err := s.Store.GetUserMFA(ctx, p.User.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return nil, err
	default:
		st.Enabled, st.EnabledAt, st.Pending = m.EnabledAt != nil, m.EnabledAt, m.EnabledAt == nil
		if st.Enabled {
			st.RecoveryCodesLeft, st.RecoveryCodesTotal = m.CodesLeft, m.CodesTotal
		}
	}
	if st.TrustedDevices, err = s.Store.CountTrustedDevices(ctx, p.User.ID); err != nil {
		return nil, err
	}
	req, err := s.mfaRequirementFor(ctx, p.User, ms)
	if err != nil {
		return nil, err
	}
	st.Required, st.RememberDays = req.Required, req.RememberDays
	if req.Required && !st.Enabled {
		st.EnrolmentDeadline = req.Deadline
	}
	st.EnrolmentRequired = p.Session != nil && p.Session.MFAEnrolRequired && !st.Enabled
	return st, nil
}

func (s *Server) handleMFAStatus(w http.ResponseWriter, r *http.Request, p *Principal) {
	ms, err := s.Store.ListUserMemberships(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.mfaStatusOf(r.Context(), p, ms)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// mfaSelf allows MFA changes only in a browser session (not with an API token).
func mfaSelf(w http.ResponseWriter, p *Principal) bool {
	if p.Session == nil {
		writeError(w, http.StatusForbidden, "forbidden", "multi-factor authentication is managed in a signed-in browser session, not with an API token")
		return false
	}
	return true
}

// handleMFASetup: POST /auth/mfa/totp starts setting up an authenticator
// app. The secret is shown this once (QR code and manual key); it becomes
// active with POST /auth/mfa/totp/confirm.
func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	secret := auth.NewTOTPSecret()
	if err := s.Store.StartTOTP(r.Context(), p.User.ID, secret); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret": secret, "otpauth_uri": auth.OTPAuthURI(mfaIssuer, p.User.Email, secret),
		"issuer": mfaIssuer, "account": p.User.Email,
		"digits": auth.TOTPDigits, "period": auth.TOTPPeriod, "algorithm": "SHA1",
	})
}

// handleMFAConfirm: POST /auth/mfa/totp/confirm {"code"} activates the
// authenticator and returns the recovery codes (shown once).
func (s *Server) handleMFAConfirm(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if !s.mfaLimiter.allow("confirm|" + p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	m, err := s.Store.GetUserMFA(ctx, p.User.ID)
	if err != nil || m.EnabledAt != nil {
		s.fail(w, r, store.Conflictf("no authenticator set-up is waiting for confirmation; start again"))
		return
	}
	step, ok := auth.VerifyTOTP(m.Secret, req.Code, s.now(), 0)
	if !ok {
		writeError(w, http.StatusBadRequest, "mfa_invalid", "the code is wrong; check the time on your device and try the next code")
		return
	}
	codes := auth.NewRecoveryCodes(auth.RecoveryCodeCount)
	if err := s.Store.ConfirmTOTP(ctx, p.User.ID, step, hashCodes(codes)); err != nil {
		s.fail(w, r, err)
		return
	}
	s.mfaLimiter.reset("confirm|" + p.User.ID)
	s.audit(ctx, p, "mfa.enable", "user", p.User.ID, "totp")
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func hashCodes(codes []string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = auth.HashRecoveryCode(c)
	}
	return out
}

// verifyOwnCode checks a current code of the caller (re-authentication for
// turning MFA off and new recovery codes); it answers itself on failure.
func (s *Server) verifyOwnCode(w http.ResponseWriter, r *http.Request, p *Principal, code string) bool {
	if !s.mfaLimiter.allow(p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return false
	}
	ok, recovery, err := s.checkSecondFactor(r.Context(), p.User.ID, code)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if !ok {
		s.recordLogin(r, p.User.ID, "mfa", false)
		writeError(w, http.StatusBadRequest, "mfa_invalid", "the code is wrong or was already used")
		return false
	}
	if recovery {
		s.audit(r.Context(), p, "mfa.recovery_code_used", "user", p.User.ID, p.User.Email)
	}
	s.mfaLimiter.reset(p.User.ID)
	return true
}

// handleMFADisable: POST /auth/mfa/disable {"code"} removes the caller's
// second factor, unless a policy requires one.
func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	m, err := s.userMFA(ctx, p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if m == nil {
		// nothing confirmed: drop a pending set-up
		if _, err := s.Store.DeleteUserMFA(ctx, p.User.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ms, err := s.Store.ListUserMemberships(ctx, p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	need, err := s.mfaRequirementFor(ctx, p.User, ms)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if need.Required {
		writeError(w, http.StatusForbidden, "mfa_required", "multi-factor authentication is required for your account and cannot be turned off")
		return
	}
	if !s.verifyOwnCode(w, r, p, req.Code) {
		return
	}
	if _, err := s.Store.DeleteUserMFA(ctx, p.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "mfa.disable", "user", p.User.ID, "")
	s.sendMFANotice(ctx, p.User, "mfa.disabled", "")
	w.WriteHeader(http.StatusNoContent)
}

// handleMFARecoveryCodes: POST /auth/mfa/recovery-codes {"code"} replaces
// the recovery codes (the old ones stop working) and returns the new ones.
func (s *Server) handleMFARecoveryCodes(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	m, err := s.userMFA(ctx, p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if m == nil {
		s.fail(w, r, store.Conflictf("set up an authenticator app first"))
		return
	}
	if !s.verifyOwnCode(w, r, p, req.Code) {
		return
	}
	codes := auth.NewRecoveryCodes(auth.RecoveryCodeCount)
	if err := s.Store.ReplaceRecoveryCodes(ctx, p.User.ID, hashCodes(codes)); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "mfa.recovery_codes", "user", p.User.ID, "")
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// handleMFAForgetDevices: DELETE /auth/mfa/trusted-devices forgets the
// caller's remembered devices.
func (s *Server) handleMFAForgetDevices(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	n, err := s.Store.DeleteTrustedDevices(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: mfaDeviceCookie, Value: "", Path: mfaCookiePath, MaxAge: -1, HttpOnly: true,
		Secure: s.Cfg.Auth.CookieSecure, SameSite: http.SameSiteLaxMode})
	s.audit(r.Context(), p, "mfa.forget_devices", "user", p.User.ID, fmt.Sprint(n))
	writeJSON(w, http.StatusOK, map[string]int{"forgotten": n})
}

// ---------------------------------------------------------------- policy administration

type mfaPolicyView struct {
	Server       *model.MFAPolicy `json:"server"`
	Tenant       *model.MFAPolicy `json:"tenant"`
	CanEditSrv   bool             `json:"can_edit_server"`
	Members      int              `json:"members"`
	MembersMFA   int              `json:"members_enrolled"`
	ServerRoles  []string         `json:"server_roles"`
	UsersTotal   int              `json:"users,omitempty"`
	UsersEnabled int              `json:"users_enrolled,omitempty"`
}

// handleMFAPolicyGet: GET /admin/mfa returns the server policy, the
// tenant's policy and how many members have a second factor.
func (s *Server) handleMFAPolicyGet(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	srv, err := s.Store.MFAPolicy(ctx, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := mfaPolicyView{Server: srv, CanEditSrv: p.User.SuperAdmin && p.Token == nil,
		ServerRoles: append(append([]string{}, rbac.Roles...), model.MFASuperAdminRole)}
	if p.Tenant != nil {
		if v.Tenant, err = s.Store.MFAPolicy(ctx, p.TenantID()); err != nil {
			s.fail(w, r, err)
			return
		}
		ms, err := s.Store.ListTenantMembers(ctx, p.TenantID())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v.Members = len(ms)
		for _, m := range ms {
			if m.MFAEnabled {
				v.MembersMFA++
			}
		}
	}
	if v.CanEditSrv {
		us, err := s.Store.ListUsers(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		on, err := s.Store.MFAEnabledUsers(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v.UsersTotal, v.UsersEnabled = len(us), len(on)
	}
	writeJSON(w, http.StatusOK, v)
}

type mfaPolicyReq struct {
	Mode         string   `json:"mode"`
	Roles        []string `json:"roles"`
	GraceDays    int      `json:"grace_days"`
	RememberDays int      `json:"remember_days"`
}

// handleMFAPolicyServer: PUT /admin/mfa/server (super administrators).
func (s *Server) handleMFAPolicyServer(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req mfaPolicyReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	valid := map[string]bool{model.MFASuperAdminRole: true}
	for _, ro := range rbac.Roles {
		valid[ro] = true
	}
	for _, ro := range req.Roles {
		if !valid[ro] {
			s.fail(w, r, badRequest("unknown role %q (the server policy knows the built-in roles and super_admin)", ro))
			return
		}
	}
	s.saveMFAPolicy(w, r, p, "", req)
}

// handleMFAPolicyTenant: PUT /admin/mfa/tenant (tenant.manage). A tenant
// cannot remember devices (remember_days is a server setting).
func (s *Server) handleMFAPolicyTenant(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Tenant == nil {
		s.fail(w, r, badRequest("choose a tenant first"))
		return
	}
	var req mfaPolicyReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, ro := range req.Roles {
		ok, err := s.validRole(r.Context(), p.TenantID(), ro)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !ok {
			s.fail(w, r, badRequest("unknown role %q", ro))
			return
		}
	}
	req.RememberDays = 0
	s.saveMFAPolicy(w, r, p, p.TenantID(), req)
}

func (s *Server) saveMFAPolicy(w http.ResponseWriter, r *http.Request, p *Principal, scope string, req mfaPolicyReq) {
	pol := &model.MFAPolicy{Scope: scope, Mode: req.Mode, Roles: req.Roles, GraceDays: req.GraceDays,
		RememberDays: req.RememberDays, UpdatedBy: p.User.Email}
	if err := s.Store.SaveMFAPolicy(r.Context(), pol); err != nil {
		s.fail(w, r, err)
		return
	}
	what := "tenant"
	if scope == "" {
		what = "server"
	}
	s.audit(r.Context(), p, "mfa.policy", "mfa_policy", what,
		fmt.Sprintf("mode=%s roles=%s grace_days=%d remember_days=%d", pol.Mode, strings.Join(pol.Roles, ","), pol.GraceDays, pol.RememberDays))
	writeJSON(w, http.StatusOK, pol)
}

// ---------------------------------------------------------------- administrator reset

// handleResetMemberMFA: POST /members/{uid}/mfa/reset removes a member's
// second factor (lost phone). Like a password reset, a tenant
// administrator can only reset members who belong to no other tenant and
// are not super administrators. The user is signed out everywhere, told by
// e-mail and in-app notification, and must enrol again at the next sign-in
// if a policy requires MFA.
func (s *Server) handleResetMemberMFA(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	if !(p.User.SuperAdmin && p.Token == nil) {
		ms, err := s.Store.ListUserMemberships(ctx, uid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if u.SuperAdmin || len(ms) != 1 {
			writeError(w, http.StatusForbidden, "forbidden", "only a super administrator can reset the multi-factor authentication of this user")
			return
		}
	}
	s.resetMFA(w, r, p, u, "member.reset_mfa", p.TenantID())
}

// handleResetUserMFA: POST /users/{id}/mfa/reset (super administrators).
func (s *Server) handleResetUserMFA(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	u, err := s.Store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tenant := ""
	if ms, err := s.Store.ListUserMemberships(r.Context(), u.ID); err == nil && len(ms) > 0 {
		tenant = ms[0].TenantID
		for _, m := range ms {
			if m.TenantID == p.TenantID() {
				tenant = m.TenantID
			}
		}
	}
	s.resetMFA(w, r, p, u, "user.reset_mfa", tenant)
}

func (s *Server) resetMFA(w http.ResponseWriter, r *http.Request, p *Principal, u *model.User, action, tenantID string) {
	ctx := r.Context()
	if u.ID == p.User.ID {
		s.fail(w, r, badRequest("use your profile to change your own multi-factor authentication"))
		return
	}
	had, err := s.Store.DeleteUserMFA(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !had {
		s.fail(w, r, store.Conflictf("%s has no second factor", u.Email))
		return
	}
	_ = s.Store.DeleteUserSessions(ctx, u.ID)
	s.audit(ctx, p, action, "user", u.ID, u.Email)
	actor := p.User.DisplayName
	if actor == "" {
		actor = p.User.Email
	}
	if tenantID != "" {
		n := model.Notification{UserID: u.ID, Event: "mfa.reset", Title: mfaResetTitle(u.Preferences.Language),
			EntityType: "user", EntityID: u.ID, Actor: actor}
		if err := s.Store.AddNotifications(ctx, tenantID, []model.Notification{n}); err != nil {
			s.Log.Warn("notification", "event", n.Event, "err", err)
		}
	}
	s.sendMFANotice(ctx, u, "mfa.reset", actor)
	w.WriteHeader(http.StatusNoContent)
}

func mfaResetTitle(lang string) string { return tr(lang, "mfa.reset.subject") }

// sendMFANotice e-mails a security notice about the user's second factor
// (whatever their notification settings).
func (s *Server) sendMFANotice(ctx context.Context, u *model.User, kind, actor string) {
	if !s.emailOn() {
		return
	}
	lang := mailLang(u.Preferences.Language)
	intro := tr(lang, kind+".intro")
	if actor != "" {
		intro = tr(lang, kind+".intro", actor)
	}
	c := mailContent{Lang: lang, Title: tr(lang, kind+".subject"),
		Intro:  []string{intro, tr(lang, "mfa.notYou")},
		Action: &mailLink{Text: tr(lang, "mfa.action"), URL: s.baseURL() + "/profile#mfa"}}
	s.sendMail(ctx, s.message([]mail.Address{userAddress(u)}, c.Title, c), nil)
}
