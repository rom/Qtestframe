package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// mfaClock is the real time plus an offset the tests move forward (TOTP
// time steps, grace periods).
type mfaClock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *mfaClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().UTC().Add(c.offset)
}

func (c *mfaClock) add(d time.Duration) {
	c.mu.Lock()
	c.offset += d
	c.mu.Unlock()
}

func withClock(e *env) *mfaClock {
	c := &mfaClock{}
	e.srv.Clock = c.Now
	return c
}

// resetLimits forgets all rate-limited attempts (tests that sign in often).
func resetLimits(e *env) {
	for _, l := range []*rateLimiter{e.srv.limiter, e.srv.mfaLimiter} {
		l.mu.Lock()
		l.hits = map[string][]time.Time{}
		l.mu.Unlock()
	}
}

func totpAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := auth.TOTPCode(secret, auth.TOTPStep(at))
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// enrol sets up TOTP for the signed-in client and returns the secret and
// the recovery codes.
func enrol(t *testing.T, c *client, clk *mfaClock) (string, []string) {
	t.Helper()
	setup := c.m("POST", "/api/v1/auth/mfa/totp", nil, 200)
	secret := setup["secret"].(string)
	if !strings.HasPrefix(setup["otpauth_uri"].(string), "otpauth://totp/Qtestframe:") {
		t.Fatal(setup)
	}
	res := c.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": totpAt(t, secret, clk.Now())}, 200)
	var codes []string
	for _, x := range res["recovery_codes"].([]any) {
		codes = append(codes, x.(string))
	}
	if len(codes) != 10 {
		t.Fatal(res)
	}
	return secret, codes
}

// passwordStep signs in with the password and expects the MFA step.
func passwordStep(t *testing.T, e *env, email string) *client {
	t.Helper()
	c := e.anon()
	res := c.m("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": testPassword}, 200)
	if res["mfa_required"] != true || res["csrf_token"] != nil {
		t.Fatal(res)
	}
	return c
}

// verify gives the second factor and returns the signed-in client.
func verify(t *testing.T, c *client, code string, remember bool) {
	t.Helper()
	var me meResponse
	c.call("POST", "/api/v1/auth/mfa/verify", map[string]any{"code": code, "remember": remember}, 200, &me)
	c.csrf = me.CSRFToken
	if me.CSRFToken == "" || me.MFA == nil || !me.MFA.Enabled || !me.User.MFAEnabled {
		t.Fatalf("%+v", me)
	}
}

func TestMFAEnrolAndSignIn(t *testing.T) {
	e := newEnv(t)
	clk := withClock(e)
	e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	if st := c.m("GET", "/api/v1/auth/mfa", nil, 200); st["enabled"] != false || st["required"] != false {
		t.Fatal(st)
	}
	// a wrong confirmation code keeps the set-up pending
	c.m("POST", "/api/v1/auth/mfa/totp", nil, 200)
	if c.m("GET", "/api/v1/auth/mfa", nil, 200)["pending"] != true {
		t.Fatal("not pending")
	}
	if res := c.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": "000000"}, 400); res["code"] != "mfa_invalid" {
		t.Fatal(res)
	}
	secret, codes := enrol(t, c, clk)
	st := c.m("GET", "/api/v1/auth/mfa", nil, 200)
	if st["enabled"] != true || st["recovery_codes_left"] != 10.0 || st["secret"] != nil {
		t.Fatal(st)
	}
	if me := c.m("GET", "/api/v1/auth/me", nil, 200); me["user"].(map[string]any)["mfa_enabled"] != true {
		t.Fatal(me)
	}
	c.m("POST", "/api/v1/auth/mfa/totp", nil, 409) // already active

	// sign in: the password alone gives no session
	p := passwordStep(t, e, "anna@example.com")
	p.m("GET", "/api/v1/auth/me", nil, 401)
	if ch := p.m("GET", "/api/v1/auth/mfa/challenge", nil, 200); ch["mfa_required"] != true {
		t.Fatal(ch)
	}
	// the code used to confirm cannot be used again (replay)
	if res := p.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": totpAt(t, secret, clk.Now())}, 401); res["code"] != "mfa_invalid" {
		t.Fatal(res)
	}
	clk.add(30 * time.Second)
	code := totpAt(t, secret, clk.Now())
	verify(t, p, code, false)
	p.m("GET", "/api/v1/projects", nil, 200)
	// the pending sign-in is gone; the same code does not work twice
	p2 := passwordStep(t, e, "anna@example.com")
	p2.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": code}, 401)
	// a recovery code works once
	verify(t, p2, strings.ToUpper(codes[0]), false)
	p3 := passwordStep(t, e, "anna@example.com")
	p3.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": codes[0]}, 401)
	if st := p2.m("GET", "/api/v1/auth/mfa", nil, 200); st["recovery_codes_left"] != 9.0 {
		t.Fatal(st)
	}
	// five wrong codes end the pending sign-in
	for i := 0; i < 4; i++ {
		p3.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": "123456"}, 401)
	}
	if res := p3.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": codes[1]}, 401); res["code"] != "mfa_expired" {
		t.Fatal(res)
	}
	p3.m("GET", "/api/v1/auth/mfa/challenge", nil, 401)
	// the attempts count against the sign-in limit of address and account
	for i := 0; i < 3; i++ {
		passwordStep(t, e, "anna@example.com")
	}
	if res := e.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}, 429); res["code"] != "rate_limited" {
		t.Fatal(res)
	}
	resetLimits(e)
	// failed second factors are failed sign-ins
	failed := 0
	for _, l := range items(p2.m("GET", "/api/v1/auth/logins", nil, 200)) {
		if l["method"] == "mfa" && l["success"] == false {
			failed++
		}
	}
	if failed < 5 {
		t.Fatal(failed)
	}
	// logout cancels a pending sign-in
	p4 := passwordStep(t, e, "anna@example.com")
	p4.raw("POST", "/api/v1/auth/logout", nil).Body.Close()
	p4.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": codes[2]}, 401)

	// new recovery codes need a current code; the old ones stop working
	c.m("POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": "000000"}, 400)
	clk.add(30 * time.Second)
	fresh := c.m("POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": totpAt(t, secret, clk.Now())}, 200)["recovery_codes"].([]any)
	p5 := passwordStep(t, e, "anna@example.com")
	p5.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": codes[3]}, 401)
	verify(t, p5, fresh[0].(string), false)

	// API tokens are not affected, and cannot change MFA
	tok := c.m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)["token"].(string)
	b := e.anon()
	b.bearer = tok
	b.m("GET", "/api/v1/projects", nil, 200)
	b.m("POST", "/api/v1/auth/mfa/totp", nil, 403)

	// turning MFA off needs a code
	c.m("POST", "/api/v1/auth/mfa/disable", map[string]string{"code": "000000"}, 400)
	c.m("POST", "/api/v1/auth/mfa/disable", map[string]string{"code": fresh[1].(string)}, 204)
	e.login("anna@example.com").m("GET", "/api/v1/projects", nil, 200)
	// turning off a pending set-up (or nothing) is fine
	c.m("POST", "/api/v1/auth/mfa/totp", nil, 200)
	c.m("POST", "/api/v1/auth/mfa/disable", map[string]string{"code": ""}, 204)
	c.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": "123456"}, 409)
	c.m("POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": "123456"}, 409)
	e.anon().m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": "123456"}, 401)
}

func TestMFAPolicyAndEnforcement(t *testing.T) {
	e := newEnv(t)
	clk := withClock(e)
	a := e.login("admin@example.com")
	e.member("tess@example.com", rbac.RoleTester)
	e.member("vera@example.com", rbac.RoleViewer)
	e.member("lead@example.com", rbac.RoleAdmin)

	pol := a.m("GET", "/api/v1/admin/mfa", nil, 200)
	if pol["server"].(map[string]any)["mode"] != "optional" || pol["tenant"].(map[string]any)["mode"] != "optional" || pol["can_edit_server"] != true {
		t.Fatal(pol)
	}
	// validation
	a.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "sometimes"}, 400)
	a.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "roles", "roles": []string{}}, 400)
	a.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "roles", "roles": []string{"nope"}}, 400)
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "roles", "roles": []string{"nope"}}, 400)
	a.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "all", "grace_days": -1}, 400)
	// a tenant administrator manages the tenant policy, not the server's
	l := e.login("lead@example.com")
	if v := l.m("GET", "/api/v1/admin/mfa", nil, 200); v["can_edit_server"] != false {
		t.Fatal(v)
	}
	l.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "all"}, 403)
	e.login("vera@example.com").m("GET", "/api/v1/admin/mfa", nil, 403)

	// required for testers, at once
	saved := l.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "roles", "roles": []string{"tester"}, "grace_days": 0, "remember_days": 9}, 200)
	if saved["required_since"] == nil || saved["remember_days"] != 0.0 || saved["updated_by"] != "lead@example.com" {
		t.Fatal(saved)
	}
	members := items(a.m("GET", "/api/v1/members", nil, 200))
	for _, m := range members {
		if (m["role"] == "tester") != (m["mfa_required"] == true) || m["mfa_enabled"] != false {
			t.Fatal(m)
		}
	}
	tc := e.login("tess@example.com")
	if res := tc.m("GET", "/api/v1/projects", nil, 403); res["code"] != "mfa_enrolment_required" {
		t.Fatal(res)
	}
	me := tc.m("GET", "/api/v1/auth/me", nil, 200)
	if mfa := me["mfa"].(map[string]any); mfa["enrolment_required"] != true || mfa["required"] != true {
		t.Fatal(me)
	}
	enrol(t, tc, clk)
	tc.m("GET", "/api/v1/projects", nil, 200)
	if res := tc.m("POST", "/api/v1/auth/mfa/disable", map[string]string{"code": "123456"}, 403); res["code"] != "mfa_required" {
		t.Fatal(res)
	}
	// the viewer is not concerned
	e.login("vera@example.com").m("GET", "/api/v1/projects", nil, 200)

	// the server requires MFA for everyone with a grace period of 7 days
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "all", "grace_days": 7, "remember_days": 30}, 200)
	v := e.login("vera@example.com")
	v.m("GET", "/api/v1/projects", nil, 200)
	mfa := v.m("GET", "/api/v1/auth/me", nil, 200)["mfa"].(map[string]any)
	if mfa["required"] != true || mfa["enrolment_deadline"] == nil || mfa["enrolment_required"] != false {
		t.Fatal(mfa)
	}
	clk.add(8 * 24 * time.Hour)
	e.login("vera@example.com").m("GET", "/api/v1/projects", nil, 403)
	// saving the same policy again does not restart the grace period
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "all", "grace_days": 7, "remember_days": 30}, 200)
	e.login("vera@example.com").m("GET", "/api/v1/projects", nil, 403)

	// remember this device (server remember_days)
	vc := e.login("vera@example.com")
	secret, _ := enrol(t, vc, clk)
	clk.add(30 * time.Second)
	p := passwordStep(t, e, "vera@example.com")
	if ch := p.m("GET", "/api/v1/auth/mfa/challenge", nil, 200); ch["remember_days"] != 30.0 {
		t.Fatal(ch)
	}
	verify(t, p, totpAt(t, secret, clk.Now()), true)
	p.raw("POST", "/api/v1/auth/logout", nil).Body.Close()
	var me2 meResponse
	p.call("POST", "/api/v1/auth/login", map[string]string{"email": "vera@example.com", "password": testPassword}, 200, &me2)
	if me2.CSRFToken == "" {
		t.Fatal("remembered device still asked for a code")
	}
	p.csrf = me2.CSRFToken
	if st := p.m("GET", "/api/v1/auth/mfa", nil, 200); st["trusted_devices"] != 1.0 {
		t.Fatal(st)
	}
	if n := p.m("DELETE", "/api/v1/auth/mfa/trusted-devices", nil, 200); n["forgotten"] != 1.0 {
		t.Fatal(n)
	}
	passwordStep(t, e, "vera@example.com")
	// optional again
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "optional", "remember_days": 0}, 200)
	if v := a.m("GET", "/api/v1/admin/mfa", nil, 200); v["server"].(map[string]any)["required_since"] != nil || v["members_enrolled"] != 2.0 || v["users_enrolled"] != 2.0 {
		t.Fatal(v)
	}
	// user list shows the state
	for _, u := range items(a.m("GET", "/api/v1/users", nil, 200)) {
		want := u["email"] == "vera@example.com" || u["email"] == "tess@example.com"
		if (u["mfa_enabled"] == true) != want {
			t.Fatal(u)
		}
	}
	// a super administrator required by role
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "roles", "roles": []string{"super_admin"}}, 200)
	e.login("admin@example.com").m("GET", "/api/v1/projects", nil, 403)
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "optional"}, 200)
	// deleting the tenant deletes its policy
	if err := e.st.DeleteTenant(context.Background(), e.tenant.ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.st.MFAPolicy(context.Background(), e.tenant.ID); p.Mode != model.MFAOptional {
		t.Fatal(p)
	}
}

func TestMFAAdminReset(t *testing.T) {
	e, f := mailEnv(t)
	clk := withClock(e)
	a := e.login("admin@example.com")
	anna := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	enrol(t, c, clk)
	// a tenant administrator who is not a super administrator
	e.member("lead@example.com", rbac.RoleAdmin)
	l := e.login("lead@example.com")
	// someone in another tenant too: only a super administrator may reset
	other, err := e.st.CreateTenant(context.Background(), "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	bob := e.member("bob@example.com", rbac.RoleViewer)
	if err := e.st.SetMembership(context.Background(), other.ID, bob.ID, rbac.RoleViewer); err != nil {
		t.Fatal(err)
	}
	bc := e.login("bob@example.com")
	enrol(t, bc, clk)
	l.m("POST", "/api/v1/members/"+bob.ID+"/mfa/reset", nil, 403)
	l.m("POST", "/api/v1/members/"+e.admin.ID+"/mfa/reset", nil, 403)
	l.m("POST", "/api/v1/users/"+anna.ID+"/mfa/reset", nil, 403)
	for _, m := range items(a.m("GET", "/api/v1/members", nil, 200)) {
		if (m["mfa_enabled"] == true) != (m["email"] == "anna@example.com" || m["email"] == "bob@example.com") {
			t.Fatal(m)
		}
	}
	l.m("POST", "/api/v1/members/"+anna.ID+"/mfa/reset", nil, 204)
	l.m("POST", "/api/v1/members/"+anna.ID+"/mfa/reset", nil, 409) // nothing left
	l.m("POST", "/api/v1/members/nope/mfa/reset", nil, 404)
	c.m("GET", "/api/v1/auth/me", nil, 401) // signed out
	ac := e.login("anna@example.com")
	ac.m("GET", "/api/v1/projects", nil, 200)
	ns := items(ac.m("GET", "/api/v1/notifications", nil, 200))
	if len(ns) != 1 || ns[0]["event"] != "mfa.reset" || ns[0]["actor"] != "lead" {
		t.Fatal(ns)
	}
	// the super administrator resets anyone
	a.m("POST", "/api/v1/users/"+bob.ID+"/mfa/reset", nil, 204)
	a.m("POST", "/api/v1/users/"+e.admin.ID+"/mfa/reset", nil, 400) // own: use the profile
	a.m("POST", "/api/v1/users/nope/mfa/reset", nil, 404)
	audit := items(a.m("GET", "/api/v1/audit", nil, 200))
	seen := map[string]bool{}
	for _, x := range audit {
		seen[x["action"].(string)] = true
	}
	for _, act := range []string{"member.reset_mfa", "user.reset_mfa", "mfa.enable"} {
		if !seen[act] {
			t.Error("audit", act)
		}
	}
	msgs := sent(e, f)
	n := 0
	for _, m := range msgs {
		if strings.Contains(m.Subject, "multi-factor authentication was reset") {
			n++
		}
	}
	if n != 2 {
		t.Fatal(len(msgs), n)
	}
}

func TestMFAWithOIDC(t *testing.T) {
	idp := newFakeIdP(t)
	e := newEnv(t, func(c *config.Config) {
		c.Auth.OIDC = []config.OIDC{{Name: "corp", Issuer: idp.srv.URL, ClientID: "qtf", ClientSecret: "s", LocalMFA: config.LocalMFAUnlessIdPMFA,
			MFAACRValues: []string{"strong"},
			Provisioning: config.Provisioning{AutoProvision: true, DefaultTenant: "acme", DefaultRole: "viewer"}}}
	})
	clk := withClock(e)
	c := e.anon()
	login := func(claims map[string]any) string {
		res := c.raw("GET", "/api/v1/auth/oidc/corp/login", nil)
		res.Body.Close()
		loc, _ := url.Parse(res.Header.Get("Location"))
		q := loc.Query()
		idp.mu.Lock()
		idp.nonce, idp.claims = q.Get("nonce"), claims
		idp.mu.Unlock()
		res = c.raw("GET", "/api/v1/auth/oidc/corp/callback?state="+url.QueryEscape(q.Get("state"))+"&code=good", nil)
		res.Body.Close()
		if res.StatusCode != http.StatusFound {
			t.Fatal(res.StatusCode)
		}
		return res.Header.Get("Location")
	}
	claims := map[string]any{"email": "sso@example.com", "amr": []any{"pwd"}}
	if loc := login(claims); loc != "/" { // no second factor yet
		t.Fatal(loc)
	}
	me := c.m("GET", "/api/v1/auth/me", nil, 200)
	c.csrf = me["csrf_token"].(string)
	secret, _ := enrol(t, c, clk)
	c.raw("POST", "/api/v1/auth/logout", nil).Body.Close()
	c.csrf = ""
	if loc := login(claims); loc != "/login?mfa=1" {
		t.Fatal(loc)
	}
	c.m("GET", "/api/v1/auth/mfa/challenge", nil, 200)
	clk.add(30 * time.Second)
	verify(t, c, totpAt(t, secret, clk.Now()), false)
	c.raw("POST", "/api/v1/auth/logout", nil).Body.Close()
	// the provider's MFA (amr mfa, or an acr of mfa_acr_values) is enough
	if loc := login(map[string]any{"email": "sso@example.com", "amr": []any{"pwd", "mfa"}}); loc != "/" {
		t.Fatal(loc)
	}
	if loc := login(map[string]any{"email": "sso@example.com", "acr": "strong"}); loc != "/" {
		t.Fatal(loc)
	}
	found := false
	a := e.login("admin@example.com")
	for _, x := range items(a.m("GET", "/api/v1/audit", nil, 200)) {
		if x["action"] == "login" && strings.Contains(x["details"].(string), "mfa=totp") && strings.Contains(x["details"].(string), "amr=pwd") {
			found = true
		}
	}
	if !found {
		t.Error("audit details")
	}
}

func TestMFAInvitationEnrolment(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.st.SaveMFAPolicy(ctx, &model.MFAPolicy{Scope: "", Mode: model.MFAAll}); err != nil {
		t.Fatal(err)
	}
	u := mkUser(t, e.st, "new@example.com", false)
	if err := e.st.SetMembership(ctx, e.tenant.ID, u.ID, rbac.RoleViewer); err != nil {
		t.Fatal(err)
	}
	due, err := e.srv.mfaEnrolDue(ctx, u)
	if err != nil || !due {
		t.Fatal(due, err)
	}
	// the store refuses nonsense
	if err := e.st.SaveMFAPolicy(ctx, &model.MFAPolicy{Mode: model.MFAAll, RememberDays: 400}); err == nil {
		t.Fatal("remember_days accepted")
	}
	if ok, err := e.st.UseTOTPStep(ctx, u.ID, 5); ok || err != nil {
		t.Fatal(ok, err)
	}
	if n, err := e.st.PurgeExpiredMFA(ctx); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
