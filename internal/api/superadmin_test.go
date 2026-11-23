package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/settings"
)

func TestSuperAdminTenantsAccess(t *testing.T) {
	e := newEnv(t)
	e.member("lead@example.com", rbac.RoleAdmin)
	lead := e.login("lead@example.com")
	id := e.tenant.ID
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/tenants"}, {"GET", "/api/v1/admin/tenants/" + id}, {"PUT", "/api/v1/admin/tenants/" + id + "/policy"},
		{"POST", "/api/v1/admin/tenants/" + id + "/suspend"}, {"POST", "/api/v1/admin/tenants/" + id + "/reactivate"},
		{"GET", "/api/v1/users/" + e.admin.ID + "/memberships"}, {"POST", "/api/v1/users/" + e.admin.ID + "/sessions/revoke"},
	} {
		lead.call(r.method, r.path, map[string]any{}, 403, nil)
	}
	// another organisation's audit trail is for super administrators only
	other, _ := e.st.CreateTenant(context.Background(), "other", "Other")
	lead.call("GET", "/api/v1/audit?tenant="+other.ID, nil, 403, nil)
}

func TestSuperAdminTenantUsageAndPolicy(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	a.project("USE")
	e.member("m1@example.com", rbac.RoleTester)

	var list struct {
		Items []adminTenant `json:"items"`
	}
	a.call("GET", "/api/v1/admin/tenants", nil, 200, &list)
	if len(list.Items) != 1 || list.Items[0].Usage.Members != 2 || list.Items[0].Usage.Projects != 1 || list.Items[0].Policy.AllowedProviders == nil {
		t.Fatalf("%+v", list.Items)
	}
	if list.Items[0].Usage.LastActivity == nil {
		t.Error("no last activity")
	}

	// invalid policies
	for _, bad := range []map[string]any{
		{"max_members": -1}, {"allowed_providers": []string{"ldap"}}, {"email_domains": []string{"not a domain"}}, {"email_domains": []string{"*.wild.example"}},
		{"note": strings.Repeat("x", 2001)},
	} {
		a.call("PUT", "/api/v1/admin/tenants/"+e.tenant.ID+"/policy", bad, 400, nil)
	}
	var v adminTenant
	a.call("PUT", "/api/v1/admin/tenants/"+e.tenant.ID+"/policy", map[string]any{
		"max_members": 2, "max_projects": 1, "max_storage_mb": 1, "ai_disabled": true,
		"allowed_providers": []string{"local", " local", "oidc:corp"}, "email_domains": []string{"@Example.COM", "example.com"}, "note": " contract 42 ",
	}, 200, &v)
	if v.Policy.MaxMembers != 2 || strings.Join(v.Policy.AllowedProviders, ",") != "local,oidc:corp" || strings.Join(v.Policy.EmailDomains, ",") != "example.com" || v.Policy.Note != "contract 42" || v.AIEnabled {
		t.Fatalf("%+v", v.Policy)
	}
	if d := auditDetails(t, e, "tenant.policy"); len(d) != 1 || !strings.Contains(d[0], "max_members: no limit → 2") || !strings.Contains(d[0], "email_domains: [] → [example.com]") {
		t.Fatal(d)
	}
	// one domain belongs to one organisation
	other, _ := e.st.CreateTenant(ctx, "other", "Other")
	a.call("PUT", "/api/v1/admin/tenants/"+other.ID+"/policy", map[string]any{"email_domains": []string{"example.com"}}, 409, nil)
	a.call("PUT", "/api/v1/admin/tenants/nope/policy", map[string]any{}, 404, nil)
	a.call("GET", "/api/v1/admin/tenants/nope", nil, 404, nil)

	// quotas: members, projects, storage
	u := mkUser(t, e.st, "third@example.com", false)
	a.call("POST", "/api/v1/members", map[string]any{"email": u.Email, "roles": []string{"viewer"}}, 409, nil)
	// changing the roles of an existing member is no new member
	a.call("POST", "/api/v1/members", map[string]any{"email": "m1@example.com", "roles": []string{"viewer"}}, 201, nil)
	a.call("POST", "/api/v1/projects", map[string]any{"key": "TWO", "name": "Two"}, 409, nil)
	if _, err := e.st.TenantUsage(ctx, "nope"); err == nil {
		t.Error("usage of a missing tenant")
	}
	if err := e.st.CreateFile(ctx, e.tenant.ID, &model.FileInfo{Name: "big.bin"}, make([]byte, 2<<20)); err == nil || !strings.Contains(err.Error(), "storage limit") {
		t.Fatal(err)
	}
	if err := e.st.CreateFile(ctx, e.tenant.ID, &model.FileInfo{Name: "small.bin"}, make([]byte, 1024)); err != nil {
		t.Fatal(err)
	}
	// AI assistance forbidden by the policy
	st := a.m("GET", "/api/v1/ai", nil, 200)
	if st["reason"] != "off" || st["policy_disabled"] != true {
		t.Fatal(st)
	}
}

func TestSuperAdminSuspendTenant(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	m := e.member("member@example.com", rbac.RoleTester)
	mc := e.login("member@example.com")
	mc.call("GET", "/api/v1/projects", nil, 200, nil)
	a := e.login("admin@example.com")
	// a token of the member
	var tok struct {
		Token string `json:"token"`
	}
	mc.call("POST", "/api/v1/tokens", map[string]any{"name": "ci", "expires_days": 30}, 201, &tok)
	tc := e.anon()
	tc.bearer = tok.Token
	tc.call("GET", "/api/v1/projects", nil, 200, nil)

	var v adminTenant
	a.call("POST", "/api/v1/admin/tenants/"+e.tenant.ID+"/suspend", nil, 200, &v)
	if v.SuspendedAt == nil {
		t.Fatal(v)
	}
	// the member's session has no organisation any more, the token is refused
	mc.call("GET", "/api/v1/projects", nil, 403, nil)
	if me := mc.m("GET", "/api/v1/auth/me", nil, 200); me["tenant"] != nil {
		t.Fatal(me["tenant"])
	}
	tc.call("GET", "/api/v1/projects", nil, 401, nil)
	mc.call("POST", "/api/v1/auth/switch-tenant", map[string]any{"tenant_id": e.tenant.ID}, 403, nil)
	// signing in again does not open it either
	c := e.anon()
	c.call("POST", "/api/v1/auth/login", map[string]string{"email": m.Email, "password": testPassword, "tenant": "acme"}, 403, nil)
	if me := e.login(m.Email).m("GET", "/api/v1/auth/me", nil, 200); me["tenant"] != nil {
		t.Fatal(me["tenant"])
	}
	// the super administrator still works in it
	a.call("GET", "/api/v1/projects", nil, 200, nil)
	if d := auditDetails(t, e, "tenant.suspend"); len(d) != 1 || d[0] != "acme" {
		t.Fatal(d)
	}
	var back adminTenant
	a.call("POST", "/api/v1/admin/tenants/"+e.tenant.ID+"/reactivate", nil, 200, &back)
	if back.SuspendedAt != nil {
		t.Fatal(back)
	}
	tc.call("GET", "/api/v1/projects", nil, 200, nil)
	mc.call("POST", "/api/v1/auth/switch-tenant", map[string]any{"tenant_id": e.tenant.ID}, 200, nil)
	a.call("POST", "/api/v1/admin/tenants/nope/suspend", nil, 404, nil)
	if _, err := e.st.SetTenantSuspended(ctx, "nope", false); err == nil {
		t.Error("suspended a missing tenant")
	}
}

func TestSuperAdminAllowedSignInMethods(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.member("pw@example.com", rbac.RoleTester)
	sso := mkUser(t, e.st, "sso@example.com", false)
	sso.AuthProvider = "oidc:corp"
	_ = e.st.UpdateUser(ctx, sso)
	_ = e.st.SetMembership(ctx, e.tenant.ID, sso.ID, rbac.RoleTester)
	if _, err := e.st.SetTenantPolicy(ctx, e.tenant.ID, model.TenantPolicy{AllowedProviders: []string{"oidc:corp"}}); err != nil {
		t.Fatal(err)
	}
	// password accounts cannot open the organisation, super administrators can
	c := e.anon()
	c.call("POST", "/api/v1/auth/login", map[string]string{"email": "pw@example.com", "password": testPassword, "tenant": "acme"}, 403, nil)
	if me := e.login("pw@example.com").m("GET", "/api/v1/auth/me", nil, 200); me["tenant"] != nil {
		t.Fatal(me["tenant"])
	}
	e.login("admin@example.com").call("GET", "/api/v1/projects", nil, 200, nil)
	if err := e.srv.tenantClosed(ctx, e.tenant, sso); err != nil {
		t.Fatal(err)
	}
	if !methodAllowed(model.TenantPolicy{AllowedProviders: []string{"local"}}, "") || !methodAllowed(model.TenantPolicy{AllowedProviders: []string{"local"}}, scimAuthProvider) {
		t.Error("local and SCIM accounts")
	}
	if methodName("") != "a password" || methodName("saml:x") != "saml:x" {
		t.Error(methodName(""))
	}
}

func TestProvisionDomainsAndPolicy(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other, _ := e.st.CreateTenant(ctx, "other", "Other")
	if _, err := e.st.SetTenantPolicy(ctx, other.ID, model.TenantPolicy{EmailDomains: []string{"other.example"}}); err != nil {
		t.Fatal(err)
	}
	prov := config.Provisioning{AutoProvision: true, DefaultRole: "tester, reviewer, bogus", AllowedDomains: []string{"example.com", "*.example.com", "other.example"}}
	// not an allowed domain
	if _, _, err := e.srv.provision(ctx, &auth.Identity{Provider: "oidc:x", Subject: "d0", Email: "a@evil.test"}, prov); err == nil {
		t.Fatal("expected domain refusal")
	}
	// no default organisation: the one of the e-mail domain, with every valid default role
	u, tid, err := e.srv.provision(ctx, &auth.Identity{Provider: "oidc:x", Subject: "d1", Email: "b@other.example"}, prov)
	if err != nil || tid != other.ID {
		t.Fatal(tid, err)
	}
	m, _ := e.st.GetMembership(ctx, other.ID, u.ID)
	if strings.Join(m.Roles, ",") != "tester,reviewer" && strings.Join(m.Roles, ",") != "reviewer,tester" {
		t.Fatal(m.Roles)
	}
	// group mappings with several roles
	prov.DefaultTenant = "acme"
	prov.RoleMapping = map[string]string{"qa": "tester, reviewer", "boss": "manager"}
	u, _, err = e.srv.provision(ctx, &auth.Identity{Provider: "oidc:x", Subject: "d2", Email: "c@sub.example.com", Groups: []string{"qa", "boss"}}, prov)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = e.st.GetMembership(ctx, e.tenant.ID, u.ID)
	if len(m.Roles) != 3 {
		t.Fatal(m.Roles)
	}
	// a suspended organisation is not joined
	if _, err := e.st.SetTenantSuspended(ctx, other.ID, true); err != nil {
		t.Fatal(err)
	}
	prov.DefaultTenant = ""
	u, tid, err = e.srv.provision(ctx, &auth.Identity{Provider: "oidc:x", Subject: "d3", Email: "d@other.example"}, prov)
	if err != nil || tid != "" {
		t.Fatal(tid, err)
	}
	if _, err := e.st.GetMembership(ctx, other.ID, u.ID); err == nil {
		t.Fatal("joined a suspended organisation")
	}
}

func TestSuperAdminUsers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	u := e.member("user@example.com", rbac.RoleTester)
	uc := e.login(u.Email)
	e.login(u.Email)
	var ms struct {
		Items    []model.Membership `json:"items"`
		Sessions int                `json:"sessions"`
	}
	a.call("GET", "/api/v1/users/"+u.ID+"/memberships", nil, 200, &ms)
	if len(ms.Items) != 1 || ms.Items[0].TenantSlug != "acme" || ms.Sessions != 2 {
		t.Fatalf("%+v", ms)
	}
	a.call("GET", "/api/v1/users/nope/memberships", nil, 404, nil)
	var rv struct {
		Revoked int `json:"revoked"`
	}
	a.call("POST", "/api/v1/users/"+u.ID+"/sessions/revoke", nil, 200, &rv)
	if rv.Revoked != 2 {
		t.Fatal(rv)
	}
	uc.call("GET", "/api/v1/projects", nil, 401, nil)
	a.call("POST", "/api/v1/users/"+e.admin.ID+"/sessions/revoke", nil, 400, nil)
	a.call("POST", "/api/v1/users/nope/sessions/revoke", nil, 404, nil)
	if d := auditDetails(t, e, "user.revoke_sessions"); len(d) != 1 || !strings.Contains(d[0], "2 sessions") {
		t.Fatal(d)
	}

	// the last enabled super administrator cannot be demoted or disabled
	second := mkUser(t, e.st, "second@example.com", true)
	sc := e.login(second.Email)
	sc.call("PATCH", "/api/v1/users/"+e.admin.ID, map[string]any{"super_admin": false}, 200, nil)
	e.login("admin@example.com").call("GET", "/api/v1/users", nil, 403, nil)
	var r map[string]any
	sc.call("PATCH", "/api/v1/users/"+second.ID, map[string]any{"disabled": true}, 409, &r)
	// a disabled super administrator does not count and may be demoted
	third := mkUser(t, e.st, "third@example.com", true)
	third.Disabled = true
	_ = e.st.UpdateUser(ctx, third)
	if n, _ := e.st.CountActiveSuperAdmins(ctx); n != 1 {
		t.Fatal(n)
	}
	sc.call("PATCH", "/api/v1/users/"+third.ID, map[string]any{"super_admin": false}, 200, nil)
	// the guard itself (requests by another super administrator cannot
	// reach it while there are two; it protects against races and other paths)
	last, _ := e.st.GetUser(ctx, second.ID)
	if err := e.srv.keepSuperAdmin(ctx, last, true); err == nil {
		t.Fatal("the last super administrator can be demoted")
	}
	if err := e.srv.keepSuperAdmin(ctx, last, false); err != nil {
		t.Fatal(err)
	}
	mkUser(t, e.st, "fourth@example.com", true)
	if err := e.srv.keepSuperAdmin(ctx, last, true); err != nil {
		t.Fatal(err)
	}
	plain, _ := e.st.GetUser(ctx, u.ID)
	if err := e.srv.keepSuperAdmin(ctx, plain, true); err != nil {
		t.Fatal(err)
	}
}

func TestSuperAdminAuditOfAnotherTenant(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other, _ := e.st.CreateTenant(ctx, "other", "Other")
	if err := e.st.Audit(ctx, model.AuditEntry{TenantID: other.ID, UserEmail: "x@example.com", Action: "other.thing"}); err != nil {
		t.Fatal(err)
	}
	a := e.login("admin@example.com")
	var r struct {
		Items []model.AuditEntry `json:"items"`
	}
	a.call("GET", "/api/v1/audit?tenant="+other.ID, nil, 200, &r)
	if len(r.Items) != 1 || r.Items[0].Action != "other.thing" {
		t.Fatal(r.Items)
	}
	a.call("GET", "/api/v1/audit?tenant=nope", nil, 404, nil)
	a.call("GET", "/api/v1/audit?tenant="+e.tenant.ID, nil, 200, &r)
	for _, x := range r.Items {
		if x.Action == "other.thing" {
			t.Fatal("other tenant's entry in the own trail")
		}
	}
}

func TestPasswordPolicySettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"auth.password_min_classes": 4, "auth.password_history": 2}, 200)
	u := e.member("pw@example.com", rbac.RoleTester)
	c := e.login(u.Email)
	change := func(cur, next string, want int) {
		t.Helper()
		c.call("POST", "/api/v1/auth/password", map[string]string{"current_password": cur, "new_password": next}, want, nil)
	}
	change(testPassword, "lowercase-only-42", 400) // three classes
	change(testPassword, testPassword, 400)        // the current one
	change(testPassword, "Second-Pass-42", 204)
	change("Second-Pass-42", testPassword, 400) // in the history
	change("Second-Pass-42", "Third-Pass-42", 204)
	change("Third-Pass-42", testPassword, 204) // two back: forgotten
	if h, _ := e.st.PasswordHistory(ctx, u.ID); len(h) != 3 {
		t.Fatal(len(h))
	}
	// the policy of /auth/providers and of new accounts
	if p := e.anon().m("GET", "/api/v1/auth/providers", nil, 200); p["password_min_classes"] != float64(4) {
		t.Fatal(p)
	}
	a.call("POST", "/api/v1/users", map[string]any{"email": "weak@example.com", "password": "weak-password-1"}, 400, nil)

	// expiry: a password older than the limit must be changed at the next sign-in
	putSettings(a, map[string]any{"auth.password_max_age_days": 30}, 200)
	if me := e.login(u.Email).m("GET", "/api/v1/auth/me", nil, 200); me["user"].(map[string]any)["must_change_password"] == true {
		t.Fatal("fresh password expired")
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	if _, err := e.st.DB.ExecContext(ctx, e.st.DB.Rebind(`UPDATE users SET password_changed_at = ? WHERE id = ?`), old, u.ID); err != nil {
		t.Fatal(err)
	}
	if me := e.login(u.Email).m("GET", "/api/v1/auth/me", nil, 200); me["user"].(map[string]any)["must_change_password"] != true {
		t.Fatal("old password not expired")
	}
	// accounts without a recorded change start counting at their next sign-in
	if _, err := e.st.DB.ExecContext(ctx, e.st.DB.Rebind(`UPDATE users SET password_changed_at = NULL WHERE id = ?`), e.admin.ID); err != nil {
		t.Fatal(err)
	}
	e.login("admin@example.com")
	if x, _ := e.st.GetUser(ctx, e.admin.ID); x.PasswordChangedAt == nil {
		t.Fatal("no start of the password age")
	}
}

func TestSessionIdleTimeoutSetting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"auth.session_idle_timeout": "30s"}, 400)
	putSettings(a, map[string]any{"auth.session_idle_timeout": "10m"}, 200)
	u := e.member("idle@example.com", rbac.RoleTester)
	c := e.login(u.Email)
	c.call("GET", "/api/v1/projects", nil, 200, nil)
	if _, err := e.st.DB.ExecContext(ctx, e.st.DB.Rebind(`UPDATE sessions SET last_seen = ? WHERE user_id = ?`), time.Now().Add(-11*time.Minute), u.ID); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/v1/projects", nil, 401, nil)
	putSettings(a, map[string]any{"auth.session_idle_timeout": "0s"}, 200)
	c = e.login(u.Email)
	if _, err := e.st.DB.ExecContext(ctx, e.st.DB.Rebind(`UPDATE sessions SET last_seen = ? WHERE user_id = ?`), time.Now().Add(-48*time.Hour), u.ID); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/v1/projects", nil, 200, nil)
}

func TestTokenPolicySettings(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "forever"}, 201, nil)
	putSettings(a, map[string]any{"auth.token_require_expiry": true}, 200)
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "forever"}, 400, nil)
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "year", "expires_days": 365}, 201, nil)
	putSettings(a, map[string]any{"auth.token_max_days": 90, "auth.token_require_expiry": false}, 200)
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "year", "expires_days": 365}, 400, nil)
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "none"}, 400, nil)
	a.call("POST", "/api/v1/tokens", map[string]any{"name": "q", "expires_days": 90}, 201, nil)
	if m := a.m("GET", "/api/v1/meta", nil, 200); m["token_max_days"] != float64(90) || m["token_require_expiry"] != false {
		t.Fatal(m["token_max_days"], m["token_require_expiry"])
	}
}

func TestOutboundSettings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer ts.Close()
	e := newEnv(t)
	a := e.login("admin@example.com")
	// invalid values
	putSettings(a, map[string]any{"outbound.allowed_hosts": []string{"bad host"}}, 400)
	putSettings(a, map[string]any{"outbound.proxy": "ftp://proxy"}, 400)
	putSettings(a, map[string]any{"outbound.allowed_hosts": "not-a-list"}, 400)
	test := func(values map[string]any) testResult {
		t.Helper()
		var r testResult
		a.call("POST", "/api/v1/admin/settings/test/outbound", map[string]any{"url": ts.URL + "/hook", "values": values}, 200, &r)
		return r
	}
	if r := test(nil); !r.OK || r.Details["status"] != float64(204) {
		t.Fatalf("%+v", r)
	}
	if r := test(map[string]any{"outbound.block_private": true}); r.OK || r.Details["blocked"] != true {
		t.Fatalf("%+v", r)
	}
	if r := test(map[string]any{"outbound.block_private": true, "outbound.allowed_hosts": []string{"127.0.0.1"}}); !r.OK {
		t.Fatalf("%+v", r)
	}
	if r := test(map[string]any{"outbound.denied_hosts": []string{"127.0.0.1"}}); r.OK {
		t.Fatalf("%+v", r)
	}
	if r := test(map[string]any{"outbound.allowed_hosts": []string{"*.example.com"}}); r.OK || !strings.Contains(r.Error, "allowed_hosts") {
		t.Fatalf("%+v", r)
	}
	a.call("POST", "/api/v1/admin/settings/test/outbound", map[string]any{"url": "file:///etc/passwd"}, 400, nil)
	// saved: the server's own client follows the settings at once
	r := putSettings(a, map[string]any{"outbound.block_private": true, "outbound.denied_hosts": []string{"metadata.google.internal", "169.254.169.254"}}, 200)
	if v := r.get(t, "outbound.denied_hosts").Value.([]any); len(v) != 2 {
		t.Fatal(v)
	}
	req, _ := http.NewRequest("GET", ts.URL, nil)
	if _, err := e.srv.outboundClient().Do(req); err == nil || !strings.Contains(err.Error(), "outbound call blocked") {
		t.Fatal(err)
	}
	putSettings(a, map[string]any{"outbound.block_private": false}, 200)
	res, err := e.srv.outboundClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if d := auditDetails(t, e, "settings.test_outbound"); len(d) < 5 {
		t.Fatal(d)
	}
}

func TestOutboundTransportRules(t *testing.T) {
	ctx := context.Background()
	u := func(s string) *url.URL { x, _ := url.Parse(s); return x }
	for _, tc := range []struct {
		c   config.Outbound
		u   string
		bad bool
	}{
		{config.Outbound{}, "https://jira.example.com", false},
		{config.Outbound{BlockPrivate: true}, "http://10.1.2.3/x", true},
		{config.Outbound{BlockPrivate: true}, "http://[::1]/x", true},
		{config.Outbound{BlockPrivate: true}, "http://100.64.0.1/x", true},
		{config.Outbound{BlockPrivate: true}, "http://8.8.8.8/x", false},
		{config.Outbound{BlockPrivate: true, AllowedHosts: []string{"10.1.2.3"}}, "http://10.1.2.3/x", false},
		{config.Outbound{DeniedHosts: []string{"*.internal"}}, "http://metadata.internal/x", true},
		{config.Outbound{AllowedHosts: []string{"*.example.com"}}, "https://a.example.com", false},
		{config.Outbound{AllowedHosts: []string{"*.example.com"}}, "https://example.com", true},
		{config.Outbound{}, "http:///nohost", true},
		{config.Outbound{BlockPrivate: true, Proxy: "http://proxy.test:3128"}, "http://localhost/x", true},
	} {
		err := checkOutbound(ctx, tc.c, u(tc.u))
		if (err != nil) != tc.bad {
			t.Errorf("%+v %s: %v", tc.c, tc.u, err)
		}
	}
	// the proxy: configured, bypassed for no_proxy hosts
	tr := newOutboundTransport(func() config.Outbound {
		return config.Outbound{Proxy: "http://proxy.test:3128", NoProxy: []string{"*.local.test"}}
	})
	r, _ := http.NewRequest("GET", "https://jira.example.com", nil)
	if p, _ := tr.proxy(r); p == nil || p.Host != "proxy.test:3128" {
		t.Fatal(p)
	}
	r, _ = http.NewRequest("GET", "https://a.local.test", nil)
	if p, _ := tr.proxy(r); p != nil {
		t.Fatal(p)
	}
	if settings.RedactURL("http://user:pw@proxy:3128") != "http://user:REDACTED@proxy:3128" || settings.RedactURL("http://proxy:1") != "http://proxy:1" || settings.RedactURL("::") != "::" {
		t.Error(settings.RedactURL("http://user:pw@proxy:3128"))
	}
}

func TestSystemSettingsNewKeys(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Auth.SAML = []config.SAML{{Name: "adfs", DisplayName: "ADFS", IDPMetadataURL: "https://idp.example.test/md",
			Provisioning: config.Provisioning{AllowedDomains: []string{"example.com"}}}}
	})
	a := e.login("admin@example.com")
	var v struct {
		Settings  []settingView `json:"settings"`
		SAML      []samlView    `json:"saml"`
		OIDCIcons []string      `json:"oidc_icons"`
		Sections  []string      `json:"sections"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &v)
	if len(v.SAML) != 1 || v.SAML[0].ACSURL != "http://example.test/api/v1/auth/saml/adfs/acs" || v.SAML[0].EntityID != "http://example.test/api/v1/auth/saml/adfs/metadata" || len(v.OIDCIcons) < 2 {
		t.Fatalf("%+v", v.SAML)
	}
	byKey := map[string]settingView{}
	for _, s := range v.Settings {
		byKey[s.Key] = s
	}
	// read-only keys show their value and source; the DSN never
	if s := byKey["database.dsn"]; !s.Locked || s.Value != nil || s.HasSecret == nil || !*s.HasSecret {
		t.Fatalf("%+v", s)
	}
	if s := byKey["server.listen"]; s.Value != ":8080" || s.Source != "default" || s.LockReason != "readonly" {
		t.Fatalf("%+v", s)
	}
	if s := byKey["runner.interpreters"]; len(s.Value.([]any)) != 4 {
		t.Fatalf("%+v", s)
	}
	// restart settings: stored, listed as pending until the restart
	r := putSettings(a, map[string]any{"live.max_connections": 10, "server.write_timeout": "5m", "live.heartbeat": "10s", "email.reply_to": "help@example.test",
		"server.frame_ancestors": []string{"https://doors.example.com"}}, 200)
	if s := r.get(t, "live.max_connections"); !s.PendingRestart || s.RunningValue != float64(2000) {
		t.Fatalf("%+v", s)
	}
	if s := r.get(t, "live.heartbeat"); s.PendingRestart || e.srv.conf().Live.Heartbeat != 10*time.Second {
		t.Fatalf("%+v", s)
	}
	if !r.RestartPending || e.srv.conf().Server.WriteTimeout != e.cfg.Server.WriteTimeout {
		t.Fatal("restart setting applied at once")
	}
	if c := e.srv.conf(); c.Email.ReplyTo != "help@example.test" || len(c.Server.FrameAncestors) != 1 {
		t.Fatal(c.Email.ReplyTo)
	}
	putSettings(a, map[string]any{"email.reply_to": "Help <help@example.test>"}, 400)
	putSettings(a, map[string]any{"server.frame_ancestors": []string{"https://doors.example.com/path"}}, 400)
	putSettings(a, map[string]any{"server.listen": ":9"}, 409)
	// the SAML test reads the metadata
	a.call("POST", "/api/v1/admin/settings/test/saml", map[string]any{"name": "nope"}, 404, nil)
}

func TestSystemSettingsSAMLTest(t *testing.T) {
	good := `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.test/md"><IDPSSODescriptor><KeyDescriptor use="signing"/><SingleSignOnService Binding="b" Location="https://idp.example.test/sso"/></IDPSSODescriptor></EntityDescriptor>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			_, _ = w.Write([]byte(good))
		case "/nokey":
			_, _ = w.Write([]byte(idpMetadata))
		case "/notxml":
			_, _ = w.Write([]byte("<html"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "md.xml")
	_ = os.WriteFile(file, []byte(good), 0o600)
	e := newEnv(t, func(c *config.Config) {
		c.Auth.SAML = []config.SAML{{Name: "good", IDPMetadataURL: srv.URL + "/good"}, {Name: "nokey", IDPMetadataURL: srv.URL + "/nokey"},
			{Name: "notxml", IDPMetadataURL: srv.URL + "/notxml"}, {Name: "missing", IDPMetadataURL: srv.URL + "/missing"},
			{Name: "file", IDPMetadataFile: file}, {Name: "nofile", IDPMetadataFile: filepath.Join(dir, "none.xml")},
			{Name: "other", IDPMetadataFile: file}}
	})
	// the providers are built from the URLs at start; the test endpoint fetches again
	a := e.login("admin@example.com")
	for name, ok := range map[string]bool{"good": true, "nokey": false, "notxml": false, "missing": false, "file": true, "nofile": false} {
		var r testResult
		a.call("POST", "/api/v1/admin/settings/test/saml", map[string]any{"name": name}, 200, &r)
		if r.OK != ok {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if d := auditDetails(t, e, "settings.test_saml"); len(d) != 6 {
		t.Fatal(d)
	}
}

func TestEmailReplyTo(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Email.Host, c.Email.From, c.Email.ReplyTo = "smtp.example.test", "qtf@example.test", "support@example.test"
	})
	fake := &mail.Fake{}
	e.srv.Mail = fake
	m := &mail.Message{From: e.srv.from(), To: []mail.Address{{Email: "x@example.test"}}, Subject: "s", Text: "t"}
	if err := e.srv.deliverMail(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if m.ReplyTo == nil || m.ReplyTo.Email != "support@example.test" {
		t.Fatal(m.ReplyTo)
	}
	own := &mail.Message{From: e.srv.from(), To: []mail.Address{{Email: "x@example.test"}}, Subject: "s", Text: "t", ReplyTo: &mail.Address{Email: "me@example.test"}}
	_ = e.srv.deliverMail(context.Background(), own)
	if own.ReplyTo.Email != "me@example.test" {
		t.Fatal(own.ReplyTo)
	}
}
