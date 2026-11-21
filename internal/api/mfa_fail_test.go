package api

import (
	"context"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// A super administrator outside any tenant manages only the server policy
// and resets users of no tenant.
func TestMFAWithoutTenant(t *testing.T) {
	e := newEnv(t)
	clk := withClock(e)
	mkUser(t, e.st, "root@example.com", true)
	r := e.login("root@example.com")
	v := r.m("GET", "/api/v1/admin/mfa", nil, 200)
	if v["tenant"] != nil || v["can_edit_server"] != true {
		t.Fatal(v)
	}
	r.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "all"}, 400)
	lone := mkUser(t, e.st, "lone@example.com", false)
	enrol(t, e.login("lone@example.com"), clk)
	r.m("POST", "/api/v1/users/"+lone.ID+"/mfa/reset", nil, 204)
	r.m("POST", "/api/v1/auth/mfa/disable", map[string]string{"code": ""}, 204)
	r.m("POST", "/api/v1/auth/mfa/recovery-codes", nil, 400)
	r.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{}, 409)
	r.m("POST", "/api/v1/auth/mfa/disable", "{", 400)
	// remember this device, then sign in again without a code
	r.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "optional", "remember_days": 3}, 200)
	secret, _ := enrol(t, e.login("lone@example.com"), clk)
	clk.add(30 * time.Second)
	p := passwordStep(t, e, "lone@example.com")
	verify(t, p, totpAt(t, secret, clk.Now()), true)
	res := p.m("POST", "/api/v1/auth/login", map[string]string{"email": "lone@example.com", "password": testPassword}, 200)
	if res["csrf_token"] == nil {
		t.Fatal(res)
	}
	p.csrf = res["csrf_token"].(string)
	// own code checks are rate limited per account
	for i := 0; i < 10; i++ {
		p.m("POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": "000000"}, 400)
	}
	p.m("POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": "000000"}, 429)
	// so are set-up confirmations
	n := e.login("root@example.com")
	n.m("POST", "/api/v1/auth/mfa/totp", nil, 200)
	for i := 0; i < 9; i++ { // (one attempt above)
		n.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": "000000"}, 400)
	}
	n.m("POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": "000000"}, 429)
	// a user disabled while the second factor is pending is not signed in
	resetLimits(e)
	q := passwordStep(t, e, "lone@example.com")
	lone.Disabled = true
	if err := e.st.UpdateUser(context.Background(), lone); err != nil {
		t.Fatal(err)
	}
	q.m("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": "000000"}, 401)
	q.m("GET", "/api/v1/auth/mfa/challenge", nil, 401)
}

func TestMFAPolicyGraceStart(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	save := func(roles ...string) time.Time {
		t.Helper()
		p := &model.MFAPolicy{Scope: e.tenant.ID, Mode: model.MFARoles, Roles: roles}
		if err := e.st.SaveMFAPolicy(ctx, p); err != nil {
			t.Fatal(err)
		}
		return *p.RequiredSince
	}
	first := save("admin")
	time.Sleep(2 * time.Millisecond)
	if again := save("admin"); !again.Equal(first) {
		t.Fatal("same roles restarted the grace period")
	}
	if more := save("admin", "tester"); !more.After(first) {
		t.Fatal("a new role did not start a grace period")
	}
	fewer := save("tester")
	if p, _ := e.st.MFAPolicy(ctx, e.tenant.ID); !p.RequiredSince.Equal(fewer) || len(p.Roles) != 1 {
		t.Fatal(p)
	}
}

// TestMFADatabaseFailures drops the MFA tables one after another: the
// endpoints answer 500 instead of acting on missing data.
func TestMFADatabaseFailures(t *testing.T) {
	e := newEnv(t)
	clk := withClock(e)
	a := e.login("admin@example.com")
	anna := e.member("anna@example.com", rbac.RoleTester)
	c := e.login("anna@example.com")
	secret, codes := enrol(t, c, clk)
	a.m("PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "optional", "remember_days": 5}, 200)
	pending := passwordStep(t, e, "anna@example.com")
	drop := func(table string) {
		t.Helper()
		if _, err := e.st.DB.ExecContext(context.Background(), `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	status := func(cl *client, method, path string, body any) int {
		t.Helper()
		res := cl.raw(method, path, body)
		res.Body.Close()
		return res.StatusCode
	}
	want := func(got int, ok ...int) {
		t.Helper()
		for _, o := range ok {
			if got == o {
				return
			}
		}
		t.Fatalf("status %d, want %v", got, ok)
	}

	// remembered devices (the answer, /auth/me, counts them)
	drop("mfa_trusted_devices")
	clk.add(30 * time.Second)
	want(status(pending, "POST", "/api/v1/auth/mfa/verify", map[string]any{"code": totpAt(t, secret, clk.Now()), "remember": true}), 500)
	want(status(c, "GET", "/api/v1/auth/mfa", nil), 500)
	want(status(c, "GET", "/api/v1/auth/me", nil), 500)
	want(status(c, "DELETE", "/api/v1/auth/mfa/trusted-devices", nil), 500)

	// pending sign-ins
	drop("mfa_challenges")
	want(status(e.anon(), "POST", "/api/v1/auth/login", map[string]string{"email": "anna@example.com", "password": testPassword}), 500)
	want(status(e.anon(), "GET", "/api/v1/auth/mfa/challenge", nil), 401)

	// policies
	drop("mfa_policies")
	want(status(a, "GET", "/api/v1/admin/mfa", nil), 500)
	want(status(a, "GET", "/api/v1/members", nil), 500)
	want(status(a, "PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "all"}), 500)
	want(status(a, "PUT", "/api/v1/admin/mfa/server", map[string]any{"mode": "all"}), 500)
	want(status(c, "POST", "/api/v1/auth/mfa/disable", map[string]string{"code": codes[0]}), 500)
	want(status(e.anon(), "POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": testPassword}), 500)
	if due, err := e.srv.mfaEnrolDue(context.Background(), anna); err == nil || due {
		t.Fatal(due, err)
	}

	// recovery codes and enrolments
	drop("mfa_recovery_codes")
	want(status(c, "POST", "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": codes[1]}), 409, 500)
	want(status(c, "POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": codes[1]}), 500)
	want(status(a, "POST", "/api/v1/members/"+anna.ID+"/mfa/reset", nil), 500)
	drop("user_mfa")
	want(status(a, "GET", "/api/v1/users", nil), 500)
	want(status(c, "POST", "/api/v1/auth/mfa/totp", nil), 500)
	want(status(c, "POST", "/api/v1/auth/mfa/disable", map[string]string{"code": "123456"}), 500)
	want(status(c, "POST", "/api/v1/auth/mfa/recovery-codes", map[string]string{"code": "123456"}), 500)
}
