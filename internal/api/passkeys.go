package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Passkeys (WebAuthn; docs/admin-guide "Authentication", "Passkeys"):
// a passwordless sign-in with discoverable credentials (local accounts,
// user verification required), a second factor next to TOTP, and a way to
// confirm one's identity again for sensitive changes. The ceremonies are
// verified by internal/auth (webauthn.go); their challenges are stored
// server-side, single-use and short-lived, and bound to the session, the
// pending sign-in or a cookie of the browser.

const (
	// passkeyChallengeTTL: how long a ceremony may take.
	passkeyChallengeTTL = 5 * time.Minute
	// passkeyCookie binds a passwordless sign-in to the browser that asked
	// for its challenge (no sign-in with an assertion obtained elsewhere).
	passkeyCookie = "qtf_passkey"
	// reauthWindow: how recent a sign-in or re-authentication must be for
	// adding and deleting passkeys.
	reauthWindow = 10 * time.Minute
	// maxPasskeyName is the longest passkey name.
	maxPasskeyName = 100
)

func (s *Server) routesPasskeys(h routeFunc) {
	h("GET /api/v1/auth/passkeys", s.handleListPasskeys)
	h("POST /api/v1/auth/passkeys/register/options", s.handlePasskeyRegisterOptions)
	h("POST /api/v1/auth/passkeys/register", s.handlePasskeyRegister)
	h("PATCH /api/v1/auth/passkeys/{id}", s.handleRenamePasskey)
	h("DELETE /api/v1/auth/passkeys/{id}", s.handleDeletePasskey)
	h("POST /api/v1/auth/reauth/options", s.handleReauthOptions)
	h("POST /api/v1/auth/reauth", s.handleReauth)
	// administrators: revoke all passkeys of a user
	h("POST /api/v1/members/{uid}/passkeys/revoke", s.handleRevokeMemberPasskeys, "member.manage")
	h("POST /api/v1/users/{id}/passkeys/revoke", s.handleRevokeUserPasskeys)
}

// routesPasskeysPublic registers the sign-in endpoints (no session yet).
func (s *Server) routesPasskeysPublic(pub func(string, http.HandlerFunc)) {
	pub("POST /api/v1/auth/passkeys/login/options", s.handlePasskeyLoginOptions)
	pub("POST /api/v1/auth/passkeys/login", s.handlePasskeyLogin)
	pub("POST /api/v1/auth/mfa/passkey/options", s.handleMFAPasskeyOptions)
	pub("POST /api/v1/auth/mfa/passkey", s.handleMFAPasskey)
}

// ---------------------------------------------------------------- policy

// relyingParty returns the server's WebAuthn relying party, derived from
// server.base_url unless auth.passkeys.rp_id and origins say otherwise.
func (s *Server) relyingParty() (*auth.RelyingParty, error) {
	c := s.conf()
	return auth.NewRelyingParty(c.Server.BaseURL, c.Auth.Passkeys.RPID, c.Auth.Passkeys.Origins)
}

// passkeysUsable reports whether passkeys can be used (turned on and a
// valid relying party).
func (s *Server) passkeysUsable() bool {
	if !s.conf().Auth.Passkeys.Enabled {
		return false
	}
	_, err := s.relyingParty()
	return err == nil
}

// passwordlessOn reports whether the passwordless sign-in is offered.
func (s *Server) passwordlessOn() bool {
	c := s.conf()
	return c.Auth.Local && c.Auth.Passkeys.Passwordless && s.passkeysUsable()
}

// passkeysRP answers itself when passkeys cannot be used.
func (s *Server) passkeysRP(w http.ResponseWriter) (*auth.RelyingParty, bool) {
	if !s.conf().Auth.Passkeys.Enabled {
		writeError(w, http.StatusForbidden, "passkeys_disabled", "passkeys are turned off on this server")
		return nil, false
	}
	rp, err := s.relyingParty()
	if err != nil {
		s.Log.Warn("passkeys unavailable", "err", err)
		writeError(w, http.StatusServiceUnavailable, "passkeys_unavailable", "passkeys are not set up on this server: "+err.Error())
		return nil, false
	}
	return rp, true
}

// hasPasskeys reports whether the user has a passkey that can be used.
func (s *Server) hasPasskeys(ctx context.Context, userID string) (bool, error) {
	if !s.passkeysUsable() {
		return false, nil
	}
	n, err := s.Store.CountPasskeys(ctx, userID)
	return n > 0, err
}

// isAdminAnywhere reports whether the user is a super administrator or
// holds the admin role in an organisation (auth.passkeys.require_for_admins).
func isAdminAnywhere(u *model.User, ms []model.Membership) bool {
	if u.SuperAdmin {
		return true
	}
	for _, m := range ms {
		for _, r := range m.Roles {
			if r == rbac.RoleAdmin {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------- re-authentication

// recentAuth reports whether the session's user proved their identity
// within reauthWindow.
func (s *Server) recentAuth(p *Principal) bool {
	return p.Session != nil && s.Store.Now().Sub(p.Session.AuthenticatedAt()) <= reauthWindow
}

// requireRecentAuth answers itself when the session's last sign-in or
// re-authentication is too old.
func (s *Server) requireRecentAuth(w http.ResponseWriter, p *Principal) bool {
	if p.Session == nil {
		writeError(w, http.StatusForbidden, "forbidden", "passkeys are managed in a signed-in browser session, not with an API token")
		return false
	}
	if !s.recentAuth(p) {
		writeError(w, http.StatusForbidden, "reauth_required", fmt.Sprintf("confirm your identity first (sign-in or confirmation less than %d minutes ago)", int(reauthWindow/time.Minute)))
		return false
	}
	return true
}

// handleReauthOptions: POST /auth/reauth/options returns the request
// options of a passkey re-authentication.
func (s *Server) handleReauthOptions(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok {
		return
	}
	s.requestOptions(w, r, rp, p.User.ID, model.PasskeyReauth, p.Session.ID, "preferred")
}

// handleReauth: POST /auth/reauth {"password"} | {"code"} | {"credential"}
// confirms the user's identity in this session: the local password, a
// code of the authenticator app (or a recovery code) or a passkey.
func (s *Server) handleReauth(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	var req struct {
		Password   string          `json:"password"`
		Code       string          `json:"code"`
		Credential *credentialJSON `json:"credential"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	key := "reauth|" + p.User.ID
	if !s.mfaLimiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	method := ""
	switch {
	case req.Credential != nil:
		rp, ok := s.passkeysRP(w)
		if !ok {
			return
		}
		pk, err := s.verifyPasskey(ctx, rp, req.Credential, model.PasskeyReauth, p.Session.ID, p.User.ID, false)
		if err != nil {
			s.passkeyFailed(w, r, p.User.ID, err)
			return
		}
		method = "passkey " + pk.Name
	case req.Password != "":
		if p.User.AuthProvider != "local" || !auth.CheckPassword(p.User.PasswordHash, req.Password) {
			s.recordLogin(r, p.User.ID, "reauth", false)
			writeError(w, http.StatusForbidden, "reauth_failed", "the password is not correct")
			return
		}
		method = "password"
	case req.Code != "":
		ok, recovery, err := s.checkSecondFactor(ctx, p.User.ID, req.Code)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !ok {
			s.recordLogin(r, p.User.ID, "reauth", false)
			writeError(w, http.StatusForbidden, "reauth_failed", "the code is wrong or was already used")
			return
		}
		if recovery {
			s.audit(ctx, p, "mfa.recovery_code_used", "user", p.User.ID, p.User.Email)
		}
		method = "totp"
	default:
		s.fail(w, r, badRequest("give your password, an authenticator code or a passkey"))
		return
	}
	now := s.Store.Now()
	if err := s.Store.SetSessionReauth(ctx, p.Session.ID, now); err != nil {
		s.fail(w, r, err)
		return
	}
	s.mfaLimiter.reset(key)
	s.audit(ctx, p, "user.reauth", "user", p.User.ID, method)
	writeJSON(w, http.StatusOK, map[string]any{"reauth_until": now.Add(reauthWindow)})
}

// ---------------------------------------------------------------- ceremonies

// credentialJSON is a PublicKeyCredential as the browser serialises it
// (PublicKeyCredential.toJSON(): binary values in base64url).
type credentialJSON struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string   `json:"clientDataJSON"`
		AttestationObject string   `json:"attestationObject"`
		AuthenticatorData string   `json:"authenticatorData"`
		Signature         string   `json:"signature"`
		UserHandle        string   `json:"userHandle"`
		Transports        []string `json:"transports"`
	} `json:"response"`
	AuthenticatorAttachment string `json:"authenticatorAttachment"`
}

// errPasskeyExpired is the answer for an unknown, used or expired challenge.
var errPasskeyExpired = errors.New("the passkey request has expired or was already used; try again")

// errPasskeyUnknown is the answer for a credential that is not registered
// (or not of the expected user).
var errPasskeyUnknown = errors.New("this passkey is not registered for your account")

// rawID returns the decoded credential id.
func (c *credentialJSON) rawID() ([]byte, error) {
	id := c.RawID
	if id == "" {
		id = c.ID
	}
	b, err := auth.UnB64(id)
	if err != nil || len(b) == 0 || len(b) > auth.MaxCredentialIDLength {
		return nil, fmt.Errorf("%w: bad credential id", auth.ErrWebAuthn)
	}
	if c.Type != "" && c.Type != "public-key" {
		return nil, fmt.Errorf("%w: credential type %q", auth.ErrWebAuthn, c.Type)
	}
	return b, nil
}

func unb64(name, s string) ([]byte, error) {
	b, err := auth.UnB64(s)
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("%w: %s is missing or not base64url", auth.ErrWebAuthn, name)
	}
	return b, nil
}

// issueChallenge stores a new challenge and returns it.
func (s *Server) issueChallenge(ctx context.Context, purpose, userID, binding, uv string) ([]byte, error) {
	ch := auth.NewChallenge()
	err := s.Store.CreateWebAuthnChallenge(ctx, &model.WebAuthnChallenge{ID: auth.HashToken(auth.B64(ch)), Purpose: purpose,
		UserID: userID, Binding: binding, UserVerification: uv, ExpiresAt: s.Store.Now().Add(passkeyChallengeTTL)})
	return ch, err
}

// takeChallenge finds and consumes the challenge of the clientDataJSON
// (each challenge is answered once).
func (s *Server) takeChallenge(ctx context.Context, clientDataJSON []byte, purpose, binding string) (*model.WebAuthnChallenge, []byte, error) {
	cd, err := auth.ParseClientData(clientDataJSON)
	if err != nil {
		return nil, nil, err
	}
	raw, err := cd.ChallengeBytes()
	if err != nil {
		return nil, nil, err
	}
	ch, err := s.Store.TakeWebAuthnChallenge(ctx, auth.HashToken(auth.B64(raw)), purpose, binding)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, errPasskeyExpired
	}
	if err != nil {
		return nil, nil, err
	}
	return ch, raw, nil
}

type credDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

func descriptors(pks []model.Passkey) []credDescriptor {
	out := make([]credDescriptor, 0, len(pks))
	for _, pk := range pks {
		out = append(out, credDescriptor{Type: "public-key", ID: pk.CredentialID, Transports: pk.Transports})
	}
	return out
}

// requestOptions issues a challenge and answers with the options of
// navigator.credentials.get() (allowCredentials: the user's passkeys, or
// none for a discoverable sign-in).
func (s *Server) requestOptions(w http.ResponseWriter, r *http.Request, rp *auth.RelyingParty, userID, purpose, binding, uv string) {
	ctx := r.Context()
	allow := []credDescriptor{}
	if userID != "" {
		pks, err := s.Store.ListPasskeys(ctx, userID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if len(pks) == 0 {
			s.fail(w, r, store.Conflictf("you have no passkey"))
			return
		}
		allow = descriptors(pks)
	}
	ch, err := s.issueChallenge(ctx, purpose, userID, binding, uv)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"publicKey": map[string]any{
		"challenge": auth.B64(ch), "rpId": rp.ID, "timeout": passkeyChallengeTTL.Milliseconds(),
		"userVerification": uv, "allowCredentials": allow,
	}})
}

// verifyPasskey checks an assertion: its challenge (purpose, binding),
// the credential (of userID when given; otherwise its user handle must
// name its owner), the signature, the flags and the signature counter.
// It records the use and returns the passkey.
func (s *Server) verifyPasskey(ctx context.Context, rp *auth.RelyingParty, c *credentialJSON, purpose, binding, userID string, requireUV bool) (*model.Passkey, error) {
	id, err := c.rawID()
	if err != nil {
		return nil, err
	}
	cd, err := unb64("clientDataJSON", c.Response.ClientDataJSON)
	if err != nil {
		return nil, err
	}
	ad, err := unb64("authenticatorData", c.Response.AuthenticatorData)
	if err != nil {
		return nil, err
	}
	sig, err := unb64("signature", c.Response.Signature)
	if err != nil {
		return nil, err
	}
	ch, raw, err := s.takeChallenge(ctx, cd, purpose, binding)
	if err != nil {
		return nil, err
	}
	pk, err := s.Store.GetPasskeyByCredentialID(ctx, auth.B64(id))
	if errors.Is(err, store.ErrNotFound) {
		return nil, errPasskeyUnknown
	}
	if err != nil {
		return nil, err
	}
	if userID != "" && pk.UserID != userID || ch.UserID != "" && pk.UserID != ch.UserID {
		return nil, errPasskeyUnknown
	}
	if userID == "" {
		// a discoverable credential: the user handle names its owner
		uh, err := auth.UnB64(c.Response.UserHandle)
		if err != nil || string(uh) != pk.UserID {
			return nil, fmt.Errorf("%w: the user handle does not match the passkey", auth.ErrWebAuthn)
		}
	}
	key, err := auth.UnB64(pk.PublicKey)
	if err != nil {
		return nil, err
	}
	res, err := rp.VerifyAssertion(auth.AssertionResponse{ClientDataJSON: cd, AuthenticatorData: ad, Signature: sig}, raw, key, pk.SignCount, requireUV || ch.UserVerification == "required")
	if err != nil {
		return nil, err
	}
	if res.CounterRegressed {
		// a cloned authenticator, or a replay of an old assertion; the
		// signature is valid, so the sign-in goes ahead with a warning
		s.Log.Warn("passkey signature counter did not increase (possibly a cloned authenticator)",
			"user", pk.UserID, "passkey", pk.ID, "stored", pk.SignCount, "received", res.SignCount)
		u, uerr := s.Store.GetUser(ctx, pk.UserID)
		if uerr == nil {
			s.audit(ctx, &Principal{User: u}, "passkey.counter_regressed", "passkey", pk.ID,
				fmt.Sprintf("%s stored=%d received=%d", pk.Name, pk.SignCount, res.SignCount))
		}
	}
	if err := s.Store.UsePasskey(ctx, pk.ID, res.SignCount, res.BackedUp); err != nil {
		return nil, err
	}
	return pk, nil
}

// passkeyFailed answers a failed passkey ceremony.
func (s *Server) passkeyFailed(w http.ResponseWriter, r *http.Request, userID string, err error) {
	switch {
	case errors.Is(err, errPasskeyExpired):
		writeError(w, http.StatusBadRequest, "passkey_expired", err.Error())
	case errors.Is(err, errPasskeyUnknown):
		if userID != "" {
			s.recordLogin(r, userID, "passkey", false)
		}
		writeError(w, http.StatusBadRequest, "passkey_unknown", err.Error())
	case errors.Is(err, auth.ErrWebAuthn):
		s.Log.Warn("passkey verification failed", "err", err, "remote", s.clientIP(r))
		if userID != "" {
			s.recordLogin(r, userID, "passkey", false)
		}
		writeError(w, http.StatusBadRequest, "passkey_invalid", err.Error())
	default:
		s.fail(w, r, err)
	}
}

// ---------------------------------------------------------------- registration

// handlePasskeyRegisterOptions: POST /auth/passkeys/register/options
// returns the options of navigator.credentials.create(). Adding a passkey
// needs a recent sign-in or re-authentication.
func (s *Server) handlePasskeyRegisterOptions(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok || !s.requireRecentAuth(w, p) {
		return
	}
	ctx := r.Context()
	pks, err := s.Store.ListPasskeys(ctx, p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(pks) >= store.MaxPasskeysPerUser {
		s.fail(w, r, store.Conflictf("you can register at most %d passkeys; delete one first", store.MaxPasskeysPerUser))
		return
	}
	ch, err := s.issueChallenge(ctx, model.PasskeyRegister, p.User.ID, p.Session.ID, "preferred")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// discoverable credentials for the passwordless sign-in
	resident := "preferred"
	if s.conf().Auth.Passkeys.Passwordless {
		resident = "required"
	}
	params := []map[string]any{}
	for _, alg := range auth.WebAuthnAlgorithms {
		params = append(params, map[string]any{"type": "public-key", "alg": alg})
	}
	display := p.User.DisplayName
	if display == "" {
		display = p.User.Email
	}
	writeJSON(w, http.StatusOK, map[string]any{"publicKey": map[string]any{
		"rp":        map[string]string{"id": rp.ID, "name": rp.Name},
		"user":      map[string]string{"id": auth.B64([]byte(p.User.ID)), "name": p.User.Email, "displayName": display},
		"challenge": auth.B64(ch), "pubKeyCredParams": params, "timeout": passkeyChallengeTTL.Milliseconds(),
		"excludeCredentials": descriptors(pks),
		"authenticatorSelection": map[string]any{"residentKey": resident, "requireResidentKey": resident == "required",
			"userVerification": "preferred"},
		"attestation": "none",
	}})
}

// handlePasskeyRegister: POST /auth/passkeys/register {"name",
// "credential"} verifies the new credential and stores it.
func (s *Server) handlePasskeyRegister(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok || !s.requireRecentAuth(w, p) {
		return
	}
	var req struct {
		Name       string          `json:"name"`
		Credential *credentialJSON `json:"credential"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Credential == nil {
		s.fail(w, r, badRequest("credential is required"))
		return
	}
	name, err := passkeyName(req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	c := req.Credential
	cd, err := unb64("clientDataJSON", c.Response.ClientDataJSON)
	if err != nil {
		s.passkeyFailed(w, r, "", err)
		return
	}
	att, err := unb64("attestationObject", c.Response.AttestationObject)
	if err != nil {
		s.passkeyFailed(w, r, "", err)
		return
	}
	ch, raw, err := s.takeChallenge(ctx, cd, model.PasskeyRegister, p.Session.ID)
	if err == nil && ch.UserID != p.User.ID {
		err = errPasskeyExpired
	}
	if err != nil {
		s.passkeyFailed(w, r, "", err)
		return
	}
	nc, err := rp.VerifyRegistration(auth.RegistrationResponse{ClientDataJSON: cd, AttestationObject: att}, raw, false)
	if err != nil {
		s.passkeyFailed(w, r, "", err)
		return
	}
	if id, err := c.rawID(); err != nil || auth.B64(id) != auth.B64(nc.ID) {
		s.passkeyFailed(w, r, "", fmt.Errorf("%w: the credential id differs from the authenticator data", auth.ErrWebAuthn))
		return
	}
	if name == "" {
		n, err := s.Store.CountPasskeys(ctx, p.User.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		name = fmt.Sprintf("Passkey %d", n+1)
	}
	pk := &model.Passkey{UserID: p.User.ID, CredentialID: auth.B64(nc.ID), PublicKey: auth.B64(nc.PublicKey), Alg: nc.Alg,
		SignCount: nc.SignCount, AAGUID: hex.EncodeToString(nc.AAGUID), Transports: cleanTransports(c.Response.Transports),
		Attestation: nc.Format, Name: name, UserVerified: nc.UserVerified, BackupEligible: nc.BackupEligible, BackedUp: nc.BackedUp}
	if err := s.Store.AddPasskey(ctx, pk); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "passkey.register", "passkey", pk.ID, pk.Name)
	writeJSON(w, http.StatusCreated, pk)
}

// passkeyName validates a passkey name ("" = a default name).
func passkeyName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if len([]rune(n)) > maxPasskeyName {
		return "", badRequest("the name may have at most %d characters", maxPasskeyName)
	}
	if strings.ContainsAny(n, "\r\n\t") {
		return "", badRequest("the name must be one line")
	}
	return n, nil
}

// cleanTransports keeps the known transport hints the browser reported.
func cleanTransports(ts []string) []string {
	known := map[string]bool{"usb": true, "nfc": true, "ble": true, "internal": true, "hybrid": true, "smart-card": true}
	out := []string{}
	for _, t := range ts {
		if known[t] && len(out) < 6 {
			out = append(out, t)
		}
	}
	return out
}

// ---------------------------------------------------------------- self-service

// passkeyList is GET /auth/passkeys.
type passkeyList struct {
	Items []model.Passkey `json:"items"`
	// Enabled: passkeys are turned on; Available: and usable (the relying
	// party is configured: Reason says why not).
	Enabled      bool   `json:"enabled"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`
	Passwordless bool   `json:"passwordless"`
	RPID         string `json:"rp_id,omitempty"`
	// RecentAuthUntil: until then passkeys can be added and deleted
	// without confirming one's identity again.
	RecentAuthUntil *time.Time `json:"recent_auth_until"`
	ReauthMinutes   int        `json:"reauth_minutes"`
}

// handleListPasskeys: GET /auth/passkeys lists the caller's passkeys.
func (s *Server) handleListPasskeys(w http.ResponseWriter, r *http.Request, p *Principal) {
	pks, err := s.Store.ListPasskeys(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := s.conf()
	res := passkeyList{Items: pks, Enabled: c.Auth.Passkeys.Enabled, Passwordless: s.passwordlessOn(), ReauthMinutes: int(reauthWindow / time.Minute)}
	if rp, err := s.relyingParty(); err != nil {
		res.Reason = err.Error()
	} else {
		res.Available, res.RPID = res.Enabled, rp.ID
	}
	if p.Session != nil && s.recentAuth(p) {
		t := p.Session.AuthenticatedAt().Add(reauthWindow)
		res.RecentAuthUntil = &t
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRenamePasskey: PATCH /auth/passkeys/{id} {"name"}.
func (s *Server) handleRenamePasskey(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !mfaSelf(w, p) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	name, err := passkeyName(req.Name)
	if err == nil && name == "" {
		err = badRequest("the name is required")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	if err := s.Store.RenamePasskey(ctx, p.User.ID, id, name); err != nil {
		s.fail(w, r, err)
		return
	}
	pk, err := s.Store.GetPasskey(ctx, p.User.ID, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "passkey.rename", "passkey", pk.ID, pk.Name)
	writeJSON(w, http.StatusOK, pk)
}

// handleDeletePasskey: DELETE /auth/passkeys/{id} needs a recent sign-in
// or re-authentication. The last second factor of an account that a
// policy requires MFA of cannot be deleted.
func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !s.requireRecentAuth(w, p) {
		return
	}
	ctx := r.Context()
	pk, err := s.Store.GetPasskey(ctx, p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Store.CountPasskeys(ctx, p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n == 1 {
		m, err := s.userMFA(ctx, p.User.ID)
		if err != nil {
			s.fail(w, r, err)
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
		if need.Required && m == nil {
			writeError(w, http.StatusForbidden, "mfa_required", "multi-factor authentication is required for your account: set up an authenticator app or another passkey before deleting your last passkey")
			return
		}
	}
	if err := s.Store.DeletePasskey(ctx, p.User.ID, pk.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "passkey.delete", "passkey", pk.ID, pk.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- passwordless sign-in

// handlePasskeyLoginOptions: POST /auth/passkeys/login/options (public)
// issues the challenge of a passwordless sign-in with a discoverable
// credential (empty allowCredentials; also for conditional mediation,
// the passkey suggestions of the e-mail field). The challenge is bound to
// a short-lived cookie of this browser.
func (s *Server) handlePasskeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	if !s.passwordlessOn() {
		writeError(w, http.StatusForbidden, "passkeys_disabled", "signing in with a passkey is turned off on this server")
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok {
		return
	}
	if !s.passkeyLimiter.allow(s.clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	tok := auth.RandomToken(32)
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: tok, Path: mfaCookiePath, HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: int(passkeyChallengeTTL / time.Second)})
	s.requestOptions(w, r, rp, "", model.PasskeyLogin, auth.HashToken(tok), "required")
}

// handlePasskeyLogin: POST /auth/passkeys/login {"credential", "tenant"}
// (public) signs in with a passkey alone. Only local accounts can (an
// account of an identity provider signs in there); the authenticator must
// have verified the user, so no second factor is asked for.
func (s *Server) handlePasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.passwordlessOn() {
		writeError(w, http.StatusForbidden, "passkeys_disabled", "signing in with a passkey is turned off on this server")
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok {
		return
	}
	if !s.sameOrigin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "cross-origin request rejected")
		return
	}
	var req struct {
		Credential *credentialJSON `json:"credential"`
		Tenant     string          `json:"tenant"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Credential == nil {
		s.fail(w, r, badRequest("credential is required"))
		return
	}
	key := s.clientIP(r) + "|passkey"
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts, try again later")
		return
	}
	c, err := r.Cookie(passkeyCookie)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusBadRequest, "passkey_expired", errPasskeyExpired.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: "", Path: mfaCookiePath, MaxAge: -1, HttpOnly: true,
		Secure: s.conf().Auth.CookieSecure, SameSite: http.SameSiteStrictMode})
	ctx := r.Context()
	pk, err := s.verifyPasskey(ctx, rp, req.Credential, model.PasskeyLogin, auth.HashToken(c.Value), "", true)
	if err != nil {
		s.passkeyFailed(w, r, "", err)
		return
	}
	u, err := s.Store.GetUser(ctx, pk.UserID)
	if err != nil || u.Disabled {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	if u.AuthProvider != "local" {
		writeError(w, http.StatusForbidden, "passkey_sso", "your account signs in with "+methodName(u.AuthProvider)+": use that button; the passkey is your second factor there")
		return
	}
	tenantID, ok := s.loginTenant(w, r, u, req.Tenant)
	if !ok {
		return
	}
	s.limiter.reset(key)
	s.completeLoginOpts(w, r, u, tenantID, "passkey", loginOpts{JSON: true, Details: "credential=" + pk.Name})
}

// loginTenant resolves the organisation (slug) a sign-in asks for; it
// answers itself when the user cannot open it.
func (s *Server) loginTenant(w http.ResponseWriter, r *http.Request, u *model.User, slug string) (string, bool) {
	if slug == "" {
		return "", true
	}
	ctx := r.Context()
	t, err := s.Store.GetTenantBySlug(ctx, slug)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return "", false
	}
	if _, err := s.Store.GetMembership(ctx, t.ID, u.ID); err != nil && !u.SuperAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "not a member of tenant")
		return "", false
	}
	if err := s.tenantClosed(ctx, t, u); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
		return "", false
	}
	return t.ID, true
}

// ---------------------------------------------------------------- second factor

// handleMFAPasskeyOptions: POST /auth/mfa/passkey/options (public; the
// qtf_mfa cookie is the credential) issues the challenge of a passkey as
// the second factor of the pending sign-in.
func (s *Server) handleMFAPasskeyOptions(w http.ResponseWriter, r *http.Request) {
	ch, err := s.pendingChallenge(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "mfa_expired", "no sign-in is waiting for a second factor; sign in again")
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok {
		return
	}
	s.requestOptions(w, r, rp, ch.UserID, model.PasskeyMFA, ch.ID, "preferred")
}

// handleMFAPasskey: POST /auth/mfa/passkey {"credential", "remember"}
// (public) completes the pending sign-in with a passkey. Like codes,
// attempts count against the limits of the account and the sign-in.
func (s *Server) handleMFAPasskey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Credential *credentialJSON `json:"credential"`
		Remember   bool            `json:"remember"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Credential == nil {
		s.fail(w, r, badRequest("credential is required"))
		return
	}
	ctx := r.Context()
	ch, err := s.pendingChallenge(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "mfa_expired", "the sign-in has expired; sign in again")
		return
	}
	rp, ok := s.passkeysRP(w)
	if !ok {
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
		writeError(w, http.StatusUnauthorized, "mfa_expired", "too many failed attempts; sign in again")
		return
	}
	pk, err := s.verifyPasskey(ctx, rp, req.Credential, model.PasskeyMFA, ch.ID, u.ID, false)
	if err != nil {
		s.passkeyFailed(w, r, u.ID, err)
		return
	}
	_ = s.Store.DeleteMFAChallenge(ctx, ch.ID)
	s.clearMFACookie(w)
	s.limiter.reset(key)
	s.mfaLimiter.reset(u.ID)
	if req.Remember {
		if pol, err := s.Store.MFAPolicy(ctx, ""); err == nil && pol.RememberDays > 0 {
			s.rememberDevice(w, r, u.ID, pol.RememberDays)
		}
	}
	opts := loginOpts{JSON: true, Details: strings.TrimSpace(ch.Details + " mfa=passkey credential=" + pk.Name)}
	s.completeLoginOpts(w, r, u, ch.TenantID, ch.Method, opts)
}

// ---------------------------------------------------------------- administration

// handleRevokeMemberPasskeys: POST /members/{uid}/passkeys/revoke removes
// all passkeys of a member (a lost or compromised device). The same
// limits as resetting a member's MFA apply.
func (s *Server) handleRevokeMemberPasskeys(w http.ResponseWriter, r *http.Request, p *Principal) {
	u, ok := s.memberToReset(w, r, p, "only a super administrator can revoke the passkeys of this user")
	if !ok {
		return
	}
	s.revokePasskeys(w, r, p, u, "member.revoke_passkeys")
}

// handleRevokeUserPasskeys: POST /users/{id}/passkeys/revoke (super administrators).
func (s *Server) handleRevokeUserPasskeys(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	u, err := s.Store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.revokePasskeys(w, r, p, u, "user.revoke_passkeys")
}

// revokePasskeys deletes all passkeys of u and signs u out everywhere.
func (s *Server) revokePasskeys(w http.ResponseWriter, r *http.Request, p *Principal, u *model.User, action string) {
	ctx := r.Context()
	if u.ID == p.User.ID {
		s.fail(w, r, badRequest("use your profile to delete your own passkeys"))
		return
	}
	n, err := s.Store.DeleteUserPasskeys(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n == 0 {
		s.fail(w, r, store.Conflictf("%s has no passkeys", u.Email))
		return
	}
	_ = s.Store.DeleteUserSessions(ctx, u.ID)
	s.audit(ctx, p, action, "user", u.ID, fmt.Sprintf("%s passkeys=%d", u.Email, n))
	writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
}
