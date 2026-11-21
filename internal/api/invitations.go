package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Invitations by e-mail: an administrator adds a member whose account does
// not exist yet; the person gets a single-use link (valid InvitationTTL) to
// a public page where they choose their name and password and are signed
// in. Single sign-on users are invited to just sign in. Only the hash of a
// token is stored.

// InvitationTTL is how long an invitation link can be used.
const InvitationTTL = 7 * 24 * time.Hour

func (s *Server) routesInvitations(h routeFunc) {
	h("POST /api/v1/members/{uid}/invitation", s.atomic(s.handleResendInvitation), "member.manage")
	h("DELETE /api/v1/members/{uid}/invitation", s.atomic(s.handleRevokeInvitation), "member.manage")
	// the public pages GET /invitations/{token} and POST …/accept are
	// registered with the other public routes (server.go)
}

// invitationView is what the members list shows of an open invitation.
type invitationView struct {
	ID        string    `json:"id"`
	SentAt    time.Time `json:"sent_at"`
	ExpiresAt time.Time `json:"expires_at"`
	SSO       bool      `json:"sso"`
	Expired   bool      `json:"expired"`
	InvitedBy string    `json:"invited_by"`
}

func (s *Server) viewInvitation(i *model.Invitation) *invitationView {
	return &invitationView{ID: i.ID, SentAt: i.SentAt, ExpiresAt: i.ExpiresAt, SSO: i.SSO, Expired: !s.Store.Now().Before(i.ExpiresAt), InvitedBy: i.InvitedBy}
}

// activated reports whether a person already uses their account: a
// password or an external identity. Invitations go to accounts that are
// not activated yet.
func activated(u *model.User) bool {
	return u.PasswordHash != "" || u.ExternalID != "" || u.AuthProvider != "local"
}

// invite creates an invitation of user u to the tenant of p and e-mails
// it after the commit.
func (s *Server) invite(ctx context.Context, p *Principal, u *model.User, role string, sso bool) (*model.Invitation, error) {
	tok := auth.RandomToken(32)
	inv := &model.Invitation{TenantID: p.TenantID(), UserID: u.ID, Email: u.Email, Role: role, TokenHash: auth.HashToken(tok), SSO: sso,
		InvitedBy: p.User.Email, ExpiresAt: s.Store.Now().Add(InvitationTTL)}
	if err := s.Store.CreateInvitation(ctx, inv); err != nil {
		return nil, err
	}
	s.sendInvitation(ctx, p, u, inv, tok)
	return inv, nil
}

// inviter is how an invitation names the person who sent it.
func inviter(p *Principal) string {
	if p.User.DisplayName != "" {
		return p.User.DisplayName
	}
	return p.User.Email
}

func (s *Server) sendInvitation(ctx context.Context, p *Principal, u *model.User, inv *model.Invitation, tok string) {
	lang := p.User.Preferences.Language // the recipient has no preferences yet
	tenant := p.Tenant.Name
	how := tr(lang, "invite.local", mailDate(lang, inv.ExpiresAt))
	if inv.SSO {
		how = tr(lang, "invite.sso", mailDate(lang, inv.ExpiresAt))
	}
	c := mailContent{Lang: mailLang(lang), Title: tr(lang, "invite.subject", tenant),
		Intro:  []string{tr(lang, "invite.intro", inviter(p), tenant, roleLabel(lang, inv.Role)), how},
		Action: &mailLink{Text: tr(lang, "invite.action"), URL: s.baseURL() + "/invite/" + url.PathEscape(tok)},
		After:  []string{tr(lang, "invite.ignore")}}
	m := s.message([]mail.Address{{Email: u.Email}}, c.Title, c)
	m.ReplyTo = &mail.Address{Name: oneLineText(inviter(p), 100), Email: p.User.Email}
	if mail.CheckAddress(p.User.Email) != nil {
		m.ReplyTo = nil
	}
	s.sendMail(ctx, m, nil)
}

// sendAddedNotice tells an existing user that they were added to a tenant.
func (s *Server) sendAddedNotice(ctx context.Context, p *Principal, u *model.User, role string) {
	lang := u.Preferences.Language
	tenant := p.Tenant.Name
	c := mailContent{Lang: mailLang(lang), Title: tr(lang, "added.subject", tenant),
		Intro:  []string{tr(lang, "added.intro", inviter(p), tenant, roleLabel(lang, role))},
		Action: &mailLink{Text: tr(lang, "added.action"), URL: s.baseURL() + "/"}}
	s.sendMail(ctx, s.message([]mail.Address{userAddress(u)}, c.Title, c), nil)
}

// roleLabel names a built-in role in an e-mail (custom roles by name).
func roleLabel(lang, role string) string {
	labels := map[string][2]string{
		"admin": {"administrator", "administratör"}, "manager": {"manager", "förvaltare"}, "test_lead": {"test lead", "testledare"},
		"tester": {"tester", "testare"}, "reviewer": {"reviewer", "granskare"}, "viewer": {"viewer", "läsare"},
	}
	if l, ok := labels[role]; ok {
		if mailLang(lang) == "sv" {
			return l[1]
		}
		return l[0]
	}
	return role
}

// memberUser loads the member {uid} of the caller's tenant.
func (s *Server) memberUser(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Membership, *model.User, bool) {
	ctx := r.Context()
	m, err := s.Store.GetMembership(ctx, p.TenantID(), r.PathValue("uid"))
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	u, err := s.Store.GetUser(ctx, m.UserID)
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	return m, u, true
}

// handleResendInvitation: POST /members/{uid}/invitation sends a new link
// (the old one stops working) to a member who has not accepted yet.
func (s *Server) handleResendInvitation(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.emailOn() {
		s.fail(w, r, errEmailOff())
		return
	}
	m, u, ok := s.memberUser(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	old, err := s.Store.OpenInvitation(ctx, p.TenantID(), u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if old == nil && activated(u) {
		s.fail(w, r, store.Conflictf("%s already uses their account", u.Email))
		return
	}
	sso := old != nil && old.SSO
	inv, err := s.invite(ctx, p, u, m.Role, sso)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "invitation.resend", "user", u.ID, u.Email)
	writeJSON(w, http.StatusOK, s.viewInvitation(inv))
}

// handleRevokeInvitation: DELETE /members/{uid}/invitation makes the link
// unusable; a person who never used their account also leaves the tenant.
func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request, p *Principal) {
	_, u, ok := s.memberUser(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	inv, err := s.Store.OpenInvitation(ctx, p.TenantID(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CloseInvitation(ctx, inv.ID, false); err != nil {
		s.fail(w, r, err)
		return
	}
	removed := false
	if !activated(u) {
		if err := s.Store.RemoveMembership(ctx, p.TenantID(), u.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		removed = true
	}
	s.audit(ctx, p, "invitation.revoke", "user", u.ID, u.Email)
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true, "removed": removed})
}

// openInvitation resolves the token of a public invitation request.
func (s *Server) openInvitationByToken(w http.ResponseWriter, r *http.Request) (*model.Invitation, *model.User, *model.Tenant, bool) {
	ctx := r.Context()
	tok := r.PathValue("token")
	inv, err := s.Store.GetInvitationByToken(ctx, auth.HashToken(tok))
	if err != nil || tok == "" || len(tok) > 200 || inv.AcceptedAt != nil || inv.RevokedAt != nil {
		writeError(w, http.StatusNotFound, "not_found", "this invitation link is not valid (it may have been used, replaced or revoked)")
		return nil, nil, nil, false
	}
	if !inv.Pending(s.Store.Now()) {
		writeError(w, http.StatusGone, "expired", "this invitation has expired; ask the administrator to send a new one")
		return nil, nil, nil, false
	}
	u, err := s.Store.GetUser(ctx, inv.UserID)
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, nil, false
	}
	t, err := s.Store.GetTenant(ctx, inv.TenantID)
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, nil, false
	}
	if _, err := s.Store.GetMembership(ctx, t.ID, u.ID); err != nil || u.Disabled {
		writeError(w, http.StatusNotFound, "not_found", "this invitation link is not valid (it may have been used, replaced or revoked)")
		return nil, nil, nil, false
	}
	return inv, u, t, true
}

// handleGetInvitation: GET /invitations/{token} (public) describes an open
// invitation for the accept page.
func (s *Server) handleGetInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow("invite|" + s.clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	inv, u, t, ok := s.openInvitationByToken(w, r)
	if !ok {
		return
	}
	s.limiter.reset("invite|" + s.clientIP(r))
	name := u.DisplayName
	if name == strings.Split(u.Email, "@")[0] {
		name = "" // the default; let the person choose
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email": u.Email, "display_name": name, "tenant_name": t.Name, "role": inv.Role, "invited_by": inv.InvitedBy,
		"expires_at": inv.ExpiresAt, "sso": inv.SSO, "local": s.conf().Auth.Local, "password_min_length": minPassword(s.conf()),
	})
}

// handleAcceptInvitation: POST /invitations/{token}/accept (public)
// {"display_name", "password"} sets the name and password (password
// policy as everywhere), closes the invitation and signs the person in.
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	key := "invite|" + s.clientIP(r)
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	inv, u, _, ok := s.openInvitationByToken(w, r)
	if !ok {
		return
	}
	if inv.SSO || !s.conf().Auth.Local {
		s.fail(w, r, badRequest("sign in with your organisation's account to accept this invitation"))
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" || len([]rune(name)) > 100 {
		s.fail(w, r, badRequest("display name must be 1-100 characters"))
		return
	}
	pw := req.Password
	if err := applyUserReq(u, userReq{Password: &pw}, minPassword(s.conf())); err != nil {
		s.fail(w, r, err)
		return
	}
	if pw == "" {
		s.fail(w, r, auth.ValidatePasswordMin("", minPassword(s.conf())))
		return
	}
	u.DisplayName = name
	u.MustChangePassword = false
	ctx := r.Context()
	err := s.Store.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CloseInvitation(ctx, inv.ID, true); err != nil {
			return err
		}
		return s.Store.UpdateUser(ctx, u)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.limiter.reset(key)
	sess, err := s.startSession(w, r, u, inv.TenantID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.principalFromSession(r, u, sess)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "invitation.accept", "user", u.ID, u.Email)
	s.recordLogin(r, u.ID, "invitation", true)
	res, err := s.me(ctx, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// acceptOnSignIn closes the open invitations of a user who signed in with
// single sign-on.
func (s *Server) acceptOnSignIn(ctx context.Context, p *Principal) {
	n, err := s.Store.AcceptUserInvitations(ctx, p.User.ID)
	if err != nil {
		s.Log.Warn("invitation accept", "err", err)
		return
	}
	if n > 0 {
		s.audit(ctx, p, "invitation.accept", "user", p.User.ID, p.User.Email+" (single sign-on)")
	}
}
