package api

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/testutil/softauthn"
)

const pkBase = "https://qtf.example.test"

// passkeyEnv is an env whose base URL can serve passkeys.
func passkeyEnv(t *testing.T, mut ...func(*config.Config)) (*env, *softauthn.Authenticator) {
	t.Helper()
	e := newEnv(t, append([]func(*config.Config){func(c *config.Config) { c.Server.BaseURL = pkBase }}, mut...)...)
	return e, softauthn.New("qtf.example.test", pkBase)
}

func challengeOf(t *testing.T, opts map[string]any) string {
	t.Helper()
	pk, ok := opts["publicKey"].(map[string]any)
	if !ok {
		t.Fatalf("no publicKey: %v", opts)
	}
	return pk["challenge"].(string)
}

// addPasskey registers a new ES256 passkey for the signed-in client.
func addPasskey(t *testing.T, c *client, a *softauthn.Authenticator, userID, name string) (*softauthn.Credential, map[string]any) {
	t.Helper()
	opts := c.m("POST", "/api/v1/auth/passkeys/register/options", nil, 200)
	cred := softauthn.NewCredential(softauthn.ES256, []byte(userID))
	pk := c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"name": name, "credential": a.CreateJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 201)
	return cred, pk
}

// passkeyLogin signs in without a password and returns the client.
func passkeyLogin(t *testing.T, e *env, a *softauthn.Authenticator, cred *softauthn.Credential, o softauthn.Options, want int) (*client, map[string]any) {
	t.Helper()
	c := e.anon()
	opts := c.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	res := c.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": a.GetJSON(cred, challengeOf(t, opts), o)}, want)
	if want == 200 {
		c.csrf, _ = res["csrf_token"].(string)
	}
	return c, res
}

func pkAudit(t *testing.T, e *env, tenantID string) map[string]string {
	t.Helper()
	list, err := e.st.ListAudit(context.Background(), tenantID, 500)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, a := range list {
		if _, ok := out[a.Action]; !ok {
			out[a.Action] = a.Details
		}
	}
	return out
}

func TestPasskeyRegisterAndPasswordlessLogin(t *testing.T) {
	e, a := passkeyEnv(t)
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	if prov := e.anon().m("GET", "/api/v1/auth/providers", nil, 200); prov["passkeys"] != true {
		t.Fatal(prov)
	}
	list := c.m("GET", "/api/v1/auth/passkeys", nil, 200)
	if list["available"] != true || list["passwordless"] != true || list["rp_id"] != "qtf.example.test" || list["recent_auth_until"] == nil || len(list["items"].([]any)) != 0 {
		t.Fatal(list)
	}
	// the creation options
	opts := c.m("POST", "/api/v1/auth/passkeys/register/options", nil, 200)["publicKey"].(map[string]any)
	sel := opts["authenticatorSelection"].(map[string]any)
	if opts["rp"].(map[string]any)["id"] != "qtf.example.test" || opts["attestation"] != "none" || sel["residentKey"] != "required" ||
		opts["user"].(map[string]any)["name"] != "anna@example.com" || len(opts["pubKeyCredParams"].([]any)) != 3 {
		t.Fatal(opts)
	}
	cred := softauthn.NewCredential(softauthn.ES256, []byte(u.ID))
	body := map[string]any{"name": "  Laptop  ", "credential": a.CreateJSON(cred, opts["challenge"].(string), softauthn.Options{Format: "packed"})}
	pk := c.m("POST", "/api/v1/auth/passkeys/register", body, 201)
	if pk["name"] != "Laptop" || pk["attestation"] != "packed" || pk["last_used_at"] != nil || pk["public_key"] != nil {
		t.Fatal(pk)
	}
	// the challenge was used: the same answer again fails
	if res := c.m("POST", "/api/v1/auth/passkeys/register", body, 400); res["code"] != "passkey_expired" {
		t.Fatal(res)
	}
	// a second passkey gets a default name; the first is excluded from new registrations
	_, pk2 := addPasskey(t, c, a, u.ID, "")
	if pk2["name"] != "Passkey 2" {
		t.Fatal(pk2)
	}
	opts = c.m("POST", "/api/v1/auth/passkeys/register/options", nil, 200)["publicKey"].(map[string]any)
	if len(opts["excludeCredentials"].([]any)) != 2 {
		t.Fatal(opts)
	}
	// the same credential twice is a conflict
	if res := c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"credential": a.CreateJSON(cred, opts["challenge"].(string), softauthn.Options{})}, 409); !strings.Contains(res["error"].(string), "already registered") {
		t.Fatal(res)
	}

	// sign in with the passkey alone
	cred.Counting = true
	pc, me := passkeyLogin(t, e, a, cred, softauthn.Options{}, 200)
	if me["user"].(map[string]any)["email"] != "anna@example.com" || pc.csrf == "" {
		t.Fatal(me)
	}
	if mfa := me["mfa"].(map[string]any); mfa["passkeys"] != float64(2) {
		t.Fatal(mfa)
	}
	pc.m("GET", "/api/v1/projects", nil, 200)
	items := items(pc.m("GET", "/api/v1/auth/passkeys", nil, 200))
	if items[0]["last_used_at"] == nil || items[1]["last_used_at"] != nil {
		t.Fatal(items)
	}
	logins, _ := e.st.ListLogins(context.Background(), u.ID, 10)
	if len(logins) == 0 || logins[0].Method != "passkey" || !logins[0].Success {
		t.Fatal(logins)
	}
	acts := pkAudit(t, e, e.tenant.ID)
	if acts["passkey.register"] == "" || !strings.HasPrefix(acts["login"], "passkey credential=Laptop") {
		t.Fatal(acts)
	}

	// the counter must increase: a replayed counter is let in with a warning
	cred.Counting = false
	passkeyLogin(t, e, a, cred, softauthn.Options{}, 200)
	if _, ok := pkAudit(t, e, "")["passkey.counter_regressed"]; !ok {
		t.Error("no counter warning")
	}
}

func TestPasskeyLoginRejected(t *testing.T) {
	e, a := passkeyEnv(t)
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	cred, _ := addPasskey(t, c, a, u.ID, "Key")
	for name, o := range map[string]softauthn.Options{
		"origin": {Origin: "https://evil.example.net"},
		"uv":     {NoUV: true},
		"rp":     {RPID: "example.test"},
		"sig":    {BadSignature: true},
	} {
		if _, res := passkeyLogin(t, e, a, cred, o, 400); res["code"] != "passkey_invalid" {
			t.Errorf("%s: %v", name, res)
		}
		resetLimits(e)
	}
	// an unknown passkey, a wrong user handle
	other := softauthn.NewCredential(softauthn.ES256, []byte(u.ID))
	if _, res := passkeyLogin(t, e, a, other, softauthn.Options{}, 400); res["code"] != "passkey_unknown" {
		t.Fatal(res)
	}
	saved := cred.UserHandle
	cred.UserHandle = []byte("someone-else")
	if _, res := passkeyLogin(t, e, a, cred, softauthn.Options{}, 400); res["code"] != "passkey_invalid" {
		t.Fatal(res)
	}
	cred.UserHandle = saved
	// the challenge belongs to the browser that asked for it (cookie)
	c1, c2 := e.anon(), e.anon()
	opts := c1.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	assertion := a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})
	if res := c2.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": assertion}, 400); res["code"] != "passkey_expired" {
		t.Fatal(res)
	}
	c2.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	if res := c2.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": assertion}, 400); res["code"] != "passkey_expired" {
		t.Fatal(res)
	}
	// (a challenge is gone once someone answered it, rightly or not)
	c1.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": assertion}, 400)
	// ... and is answered once
	opts = c1.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	assertion = a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})
	c1.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": assertion}, 200)
	if res := c1.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": assertion}, 400); res["code"] != "passkey_expired" {
		t.Fatal(res)
	}
	resetLimits(e)
	// bad requests
	c3 := e.anon()
	c3.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "***"}}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "AAAA", "type": "password"}}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "AAAA", "response": map[string]any{"clientDataJSON": "e30"}}}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "AAAA", "response": map[string]any{"clientDataJSON": "e30", "authenticatorData": "AAAA"}}}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "AAAA", "response": map[string]any{"clientDataJSON": "e30", "authenticatorData": "AAAA", "signature": "AAAA"}}}, 400)
	c3.m("POST", "/api/v1/auth/passkeys/login", "{", 400)
	// cross-origin requests are refused
	x := e.anon().with(map[string]string{"Origin": "https://evil.example.net"})
	x.m("POST", "/api/v1/auth/passkeys/login", map[string]any{}, 403)
	// a disabled account, an organisation, an account of an identity provider
	passkeyLogin(t, e, a, cred, softauthn.Options{}, 200)
	ctx := context.Background()
	if _, err := e.st.CreateTenant(ctx, "other", "Other"); err != nil {
		t.Fatal(err)
	}
	tc := e.anon()
	opts = tc.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	tc.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"tenant": "other", "credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 403)
	tc = e.anon()
	opts = tc.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	tc.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"tenant": "nope", "credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 401)
	tc = e.anon()
	opts = tc.m("POST", "/api/v1/auth/passkeys/login/options", nil, 200)
	me := tc.m("POST", "/api/v1/auth/passkeys/login", map[string]any{"tenant": "acme", "credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 200)
	if me["tenant"].(map[string]any)["slug"] != "acme" {
		t.Fatal(me)
	}
	resetLimits(e)
	uu, _ := e.st.GetUser(ctx, u.ID)
	uu.AuthProvider, uu.ExternalID = "oidc:corp", "sub-1"
	if err := e.st.UpdateUser(ctx, uu); err != nil {
		t.Fatal(err)
	}
	if _, res := passkeyLogin(t, e, a, cred, softauthn.Options{}, 403); res["code"] != "passkey_sso" {
		t.Fatal(res)
	}
	uu.Disabled = true
	if err := e.st.UpdateUser(ctx, uu); err != nil {
		t.Fatal(err)
	}
	passkeyLogin(t, e, a, cred, softauthn.Options{}, 401)
	// no cookie at all
	e.anon().m("POST", "/api/v1/auth/passkeys/login", map[string]any{"credential": map[string]any{"id": "AAAA"}}, 400)
}

func TestPasskeySecondFactor(t *testing.T) {
	e, a := passkeyEnv(t)
	clk := withClock(e)
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	cred, _ := addPasskey(t, c, a, u.ID, "Key")
	// the password sign-in now asks for the passkey
	pc := e.anon()
	res := pc.m("POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}, 200)
	if res["mfa_required"] != true || len(res["methods"].([]any)) != 1 || res["methods"].([]any)[0] != "passkey" {
		t.Fatal(res)
	}
	if ch := pc.m("GET", "/api/v1/auth/mfa/challenge", nil, 200); ch["methods"].([]any)[0] != "passkey" {
		t.Fatal(ch)
	}
	opts := pc.m("POST", "/api/v1/auth/mfa/passkey/options", nil, 200)
	allow := opts["publicKey"].(map[string]any)["allowCredentials"].([]any)
	if len(allow) != 1 || opts["publicKey"].(map[string]any)["userVerification"] != "preferred" {
		t.Fatal(opts)
	}
	// a wrong passkey counts as a failed attempt
	other := softauthn.NewCredential(softauthn.ES256, nil)
	if r := pc.m("POST", "/api/v1/auth/mfa/passkey", map[string]any{"credential": a.GetJSON(other, challengeOf(t, opts), softauthn.Options{})}, 400); r["code"] != "passkey_unknown" {
		t.Fatal(r)
	}
	opts = pc.m("POST", "/api/v1/auth/mfa/passkey/options", nil, 200)
	// without user verification: fine for a second factor
	var me meResponse
	pc.call("POST", "/api/v1/auth/mfa/passkey", map[string]any{"credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{NoUV: true})}, 200, &me)
	if me.CSRFToken == "" {
		t.Fatal(me)
	}
	if acts := pkAudit(t, e, e.tenant.ID); !strings.Contains(acts["login"], "mfa=passkey") {
		t.Fatal(acts)
	}
	// with an authenticator app too, both are offered
	enrol(t, c, clk)
	res = e.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}, 200)
	if got := res["methods"].([]any); len(got) != 3 || got[2] != "passkey" {
		t.Fatal(got)
	}
	// no pending sign-in
	e.anon().m("POST", "/api/v1/auth/mfa/passkey/options", nil, 401)
	e.anon().m("POST", "/api/v1/auth/mfa/passkey", map[string]any{"credential": map[string]any{}}, 401)
	e.anon().m("POST", "/api/v1/auth/mfa/passkey", map[string]any{}, 400)
	e.anon().m("POST", "/api/v1/auth/mfa/passkey", "{", 400)
	// remember the device
	resetLimits(e)
	if err := e.st.SaveMFAPolicy(context.Background(), &model.MFAPolicy{Scope: "", Mode: "optional", RememberDays: 7}); err != nil {
		t.Fatal(err)
	}
	rc := e.anon()
	rc.m("POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}, 200)
	opts = rc.m("POST", "/api/v1/auth/mfa/passkey/options", nil, 200)
	rc.m("POST", "/api/v1/auth/mfa/passkey", map[string]any{"remember": true, "credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 200)
	u2, _ := url.Parse(e.hs.URL + "/api/v1/auth/")
	found := false
	for _, ck := range rc.hc.Jar.Cookies(u2) {
		found = found || ck.Name == mfaDeviceCookie
	}
	if !found {
		t.Error("device not remembered")
	}
}

func TestPasskeyManageAndReauth(t *testing.T) {
	e, a := passkeyEnv(t)
	clk := withClock(e)
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	cred, pk := addPasskey(t, c, a, u.ID, "Key")
	id := pk["id"].(string)
	// rename
	if r := c.m("PATCH", "/api/v1/auth/passkeys/"+id, map[string]string{"name": "Phone"}, 200); r["name"] != "Phone" {
		t.Fatal(r)
	}
	c.m("PATCH", "/api/v1/auth/passkeys/"+id, map[string]string{"name": " "}, 400)
	c.m("PATCH", "/api/v1/auth/passkeys/"+id, map[string]string{"name": "a\nb"}, 400)
	c.m("PATCH", "/api/v1/auth/passkeys/"+id, map[string]string{"name": strings.Repeat("x", 101)}, 400)
	c.m("PATCH", "/api/v1/auth/passkeys/nope", map[string]string{"name": "x"}, 404)
	c.m("PATCH", "/api/v1/auth/passkeys/"+id, "{", 400)
	c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"name": strings.Repeat("x", 101), "credential": map[string]any{}}, 400)
	c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"name": "x"}, 400)
	c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"credential": map[string]any{}}, 400)
	c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"credential": map[string]any{"response": map[string]any{"clientDataJSON": "e30"}}}, 400)
	c.m("POST", "/api/v1/auth/passkeys/register", map[string]any{"credential": map[string]any{"response": map[string]any{"clientDataJSON": "e30", "attestationObject": "oA"}}}, 400)
	c.m("POST", "/api/v1/auth/passkeys/register", "{", 400)

	// an hour later the sign-in is no longer recent
	e.st.SetClock(func() time.Time { return time.Now().UTC().Add(time.Hour) })
	clk.add(time.Hour)
	if r := c.m("DELETE", "/api/v1/auth/passkeys/"+id, nil, 403); r["code"] != "reauth_required" {
		t.Fatal(r)
	}
	if r := c.m("POST", "/api/v1/auth/passkeys/register/options", nil, 403); r["code"] != "reauth_required" {
		t.Fatal(r)
	}
	if l := c.m("GET", "/api/v1/auth/passkeys", nil, 200); l["recent_auth_until"] != nil {
		t.Fatal(l)
	}
	// confirm with the password (a wrong one first)
	if r := c.m("POST", "/api/v1/auth/reauth", map[string]string{"password": "wrong-password-1"}, 403); r["code"] != "reauth_failed" {
		t.Fatal(r)
	}
	c.m("POST", "/api/v1/auth/reauth", map[string]string{}, 400)
	c.m("POST", "/api/v1/auth/reauth", "{", 400)
	if r := c.m("POST", "/api/v1/auth/reauth", map[string]string{"password": testPassword}, 200); r["reauth_until"] == nil {
		t.Fatal(r)
	}
	c.m("DELETE", "/api/v1/auth/passkeys/"+id, nil, 204)
	c.m("DELETE", "/api/v1/auth/passkeys/"+id, nil, 404)
	if acts := pkAudit(t, e, e.tenant.ID); acts["passkey.delete"] != "Phone" || acts["user.reauth"] != "password" || acts["passkey.rename"] != "Phone" {
		t.Fatal(acts)
	}

	// without an authenticator app a code cannot confirm
	if r := c.m("POST", "/api/v1/auth/reauth", map[string]string{"code": "123456"}, 403); r["code"] != "reauth_failed" {
		t.Fatal(r)
	}
	_ = cred
	e.st.SetClock(nil)
}

func TestPasskeyReauthWithPasskeyAndCode(t *testing.T) {
	e, a := passkeyEnv(t)
	clk := withClock(e)
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	cred, pk := addPasskey(t, c, a, u.ID, "Key")
	secret, codes := enrol(t, c, clk)
	later := func(d time.Duration) {
		e.st.SetClock(func() time.Time { return time.Now().UTC().Add(d) })
		clk.add(d)
	}
	later(time.Hour)
	c.m("DELETE", "/api/v1/auth/passkeys/"+pk["id"].(string), nil, 403)
	opts := c.m("POST", "/api/v1/auth/reauth/options", nil, 200)
	// a passkey of another account is refused
	other := softauthn.NewCredential(softauthn.ES256, nil)
	c.m("POST", "/api/v1/auth/reauth", map[string]any{"credential": a.GetJSON(other, challengeOf(t, opts), softauthn.Options{})}, 400)
	opts = c.m("POST", "/api/v1/auth/reauth/options", nil, 200)
	c.m("POST", "/api/v1/auth/reauth", map[string]any{"credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 200)
	if acts := pkAudit(t, e, e.tenant.ID); acts["user.reauth"] != "passkey Key" {
		t.Fatal(acts)
	}
	// with a code (and a recovery code) later
	later(time.Hour)
	c.m("POST", "/api/v1/auth/reauth", map[string]string{"code": totpAt(t, secret, clk.Now())}, 200)
	later(time.Hour)
	c.m("POST", "/api/v1/auth/reauth", map[string]string{"code": codes[0]}, 200)
	c.m("DELETE", "/api/v1/auth/passkeys/"+pk["id"].(string), nil, 204)
	// without passkeys there are no passkey options
	c.m("POST", "/api/v1/auth/reauth/options", nil, 409)
	// API tokens cannot manage passkeys
	tok := e.login("admin@example.com").m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)
	tc := e.anon()
	tc.bearer = tok["token"].(string)
	tc.m("POST", "/api/v1/auth/reauth", map[string]string{"password": testPassword}, 403)
	tc.m("POST", "/api/v1/auth/reauth/options", nil, 403)
	tc.m("POST", "/api/v1/auth/passkeys/register/options", nil, 403)
	tc.m("POST", "/api/v1/auth/passkeys/register", map[string]any{}, 403)
	tc.m("PATCH", "/api/v1/auth/passkeys/x", map[string]any{"name": "x"}, 403)
	tc.m("DELETE", "/api/v1/auth/passkeys/x", nil, 403)
	e.st.SetClock(nil)
}

func TestPasskeyPolicyForAdmins(t *testing.T) {
	e, a := passkeyEnv(t, func(c *config.Config) { c.Auth.Passkeys.RequireForAdmins = true })
	lead := e.member("lead@example.com", rbac.RoleAdmin)
	// the administrator must set up a second factor first
	c := e.login("lead@example.com")
	me := c.m("GET", "/api/v1/auth/me", nil, 200)
	if mfa := me["mfa"].(map[string]any); mfa["required"] != true || mfa["enrolment_required"] != true {
		t.Fatal(mfa)
	}
	if r := c.m("GET", "/api/v1/projects", nil, 403); r["code"] != "mfa_enrolment_required" {
		t.Fatal(r)
	}
	// a passkey is enough
	cred, pk := addPasskey(t, c, a, lead.ID, "Key")
	c.m("GET", "/api/v1/projects", nil, 200)
	if mfa := c.m("GET", "/api/v1/auth/me", nil, 200)["mfa"].(map[string]any); mfa["enrolment_required"] != false || mfa["enrolment_deadline"] != nil {
		t.Fatal(mfa)
	}
	// the last second factor cannot be deleted
	if r := c.m("DELETE", "/api/v1/auth/passkeys/"+pk["id"].(string), nil, 403); r["code"] != "mfa_required" {
		t.Fatal(r)
	}
	// testers are not affected
	e.member("tess@example.com", rbac.RoleTester)
	tc := e.login("tess@example.com")
	if mfa := tc.m("GET", "/api/v1/auth/me", nil, 200)["mfa"].(map[string]any); mfa["required"] != false {
		t.Fatal(mfa)
	}
	// the password sign-in of the administrator asks for the passkey
	pc := e.anon()
	if r := pc.m("POST", "/api/v1/auth/login", map[string]string{"email": "lead@example.com", "password": testPassword}, 200); r["mfa_required"] != true {
		t.Fatal(r)
	}
	opts := pc.m("POST", "/api/v1/auth/mfa/passkey/options", nil, 200)
	var me2 meResponse
	pc.call("POST", "/api/v1/auth/mfa/passkey", map[string]any{"credential": a.GetJSON(cred, challengeOf(t, opts), softauthn.Options{})}, 200, &me2)
	pc.csrf = me2.CSRFToken
	// the MFA policy view counts passkeys as a second factor
	v := pc.m("GET", "/api/v1/admin/mfa", nil, 200)
	if v["members_enrolled"] != float64(1) {
		t.Fatal(v)
	}
	// the super administrator is an administrator too
	if r := e.login("admin@example.com").m("GET", "/api/v1/projects", nil, 403); r["code"] != "mfa_enrolment_required" {
		t.Fatal(r)
	}
}

func TestPasskeyAdminRevoke(t *testing.T) {
	e, a := passkeyEnv(t)
	anna := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	addPasskey(t, c, a, anna.ID, "One")
	addPasskey(t, c, a, anna.ID, "Two")
	ac := e.login("admin@example.com")
	// counts in the member and user lists
	var annaRow map[string]any
	for _, m := range items(ac.m("GET", "/api/v1/members", nil, 200)) {
		if m["user_id"] == anna.ID {
			annaRow = m
		}
	}
	if annaRow == nil || annaRow["passkeys"] != float64(2) {
		t.Fatal(annaRow)
	}
	for _, u := range items(ac.m("GET", "/api/v1/users", nil, 200)) {
		if u["id"] == anna.ID && u["passkeys"] != float64(2) {
			t.Fatal(u)
		}
	}
	// a tenant administrator revokes them: anna is signed out
	e.member("lead@example.com", rbac.RoleAdmin)
	lc := e.login("lead@example.com")
	if r := lc.m("POST", "/api/v1/members/"+anna.ID+"/passkeys/revoke", nil, 200); r["revoked"] != float64(2) {
		t.Fatal(r)
	}
	c.m("GET", "/api/v1/auth/me", nil, 401)
	lc.m("POST", "/api/v1/members/"+anna.ID+"/passkeys/revoke", nil, 409)
	lc.m("POST", "/api/v1/members/nope/passkeys/revoke", nil, 404)
	// not a super administrator's (only super administrators can)
	lc.m("POST", "/api/v1/members/"+e.admin.ID+"/passkeys/revoke", nil, 403)
	// testers cannot
	e.member("tess@example.com", rbac.RoleTester)
	e.login("tess@example.com").m("POST", "/api/v1/members/"+anna.ID+"/passkeys/revoke", nil, 403)
	// super administrators everywhere; not their own
	c = e.login("anna@example.com")
	addPasskey(t, c, a, anna.ID, "Three")
	ac.m("POST", "/api/v1/users/"+anna.ID+"/passkeys/revoke", nil, 200)
	ac.m("POST", "/api/v1/users/nope/passkeys/revoke", nil, 404)
	ac.m("POST", "/api/v1/users/"+e.admin.ID+"/passkeys/revoke", nil, 400)
	lc.m("POST", "/api/v1/users/"+anna.ID+"/passkeys/revoke", nil, 403)
	acts := pkAudit(t, e, e.tenant.ID)
	if !strings.Contains(acts["member.revoke_passkeys"], "passkeys=2") || acts["user.revoke_passkeys"] == "" {
		t.Fatal(acts)
	}
}

func TestPasskeysOffOrUnavailable(t *testing.T) {
	// the default test base URL is http (not a secure context): unavailable
	e := newEnv(t)
	e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	if prov := e.anon().m("GET", "/api/v1/auth/providers", nil, 200); prov["passkeys"] != false {
		t.Fatal(prov)
	}
	l := c.m("GET", "/api/v1/auth/passkeys", nil, 200)
	if l["available"] != false || !strings.Contains(l["reason"].(string), "https") {
		t.Fatal(l)
	}
	if r := c.m("POST", "/api/v1/auth/passkeys/register/options", nil, 503); r["code"] != "passkeys_unavailable" {
		t.Fatal(r)
	}
	e.anon().m("POST", "/api/v1/auth/passkeys/login/options", nil, 403)
	// turned off
	e2, a := passkeyEnv(t)
	u := e2.member("anna@example.com", rbac.RoleTester)
	c2 := e2.login("anna@example.com")
	cred, _ := addPasskey(t, c2, a, u.ID, "Key")
	e2.admin2().m("PUT", "/api/v1/admin/settings", map[string]any{"values": map[string]any{"auth.passkeys.enabled": false}}, 200)
	if r := c2.m("POST", "/api/v1/auth/passkeys/register/options", nil, 403); r["code"] != "passkeys_disabled" {
		t.Fatal(r)
	}
	e2.anon().m("POST", "/api/v1/auth/passkeys/login", map[string]any{}, 403)
	// the password sign-in no longer asks for the (unusable) passkey
	me := e2.login("anna@example.com").me(t)
	if me["csrf_token"] == "" {
		t.Fatal(me)
	}
	_ = cred
	// passwordless off: passkeys are a second factor only
	e2.admin2().m("PUT", "/api/v1/admin/settings", map[string]any{"values": map[string]any{"auth.passkeys.enabled": true, "auth.passkeys.passwordless": false}}, 200)
	e2.anon().m("POST", "/api/v1/auth/passkeys/login/options", nil, 403)
	opts := c2.m("POST", "/api/v1/auth/passkeys/register/options", nil, 200)["publicKey"].(map[string]any)
	if opts["authenticatorSelection"].(map[string]any)["residentKey"] != "preferred" {
		t.Fatal(opts)
	}
	if r := e2.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}, 200); r["mfa_required"] != true {
		t.Fatal(r)
	}
}

// admin2 signs in the env's super administrator.
func (e *env) admin2() *client { return e.login("admin@example.com") }

func TestPasskeyRPConfig(t *testing.T) {
	e, _ := passkeyEnv(t, func(c *config.Config) {
		c.Auth.Passkeys.RPID = "example.test"
		c.Auth.Passkeys.Origins = []string{pkBase, "https://other.example.test"}
	})
	u := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	a := softauthn.New("example.test", "https://other.example.test")
	addPasskey(t, c, a, u.ID, "Other origin")
	if l := c.m("GET", "/api/v1/auth/passkeys", nil, 200); l["rp_id"] != "example.test" {
		t.Fatal(l)
	}
}
