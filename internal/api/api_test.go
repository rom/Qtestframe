package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/secrets"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

const testPassword = "Correct-horse-42"

func init() { auth.BcryptCost = bcrypt.MinCost }

type env struct {
	t      *testing.T
	st     *store.Store
	srv    *Server
	hs     *httptest.Server
	tenant *model.Tenant
	admin  *model.User
	cfg    config.Config
}

func newEnv(t *testing.T, mut ...func(*config.Config)) *env {
	t.Helper()
	d := testutil.OpenDB(t)
	st := store.New(d)
	// stored secrets are encrypted in every API test
	if err := st.SetupSecrets(context.Background(), secrets.NewEphemeral()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.CookieSecure = false
	cfg.Server.BaseURL = "http://example.test"
	cfg.Runner.Enabled = true
	cfg.Runner.Sandbox = false
	cfg.Runner.WorkDir = t.TempDir()
	cfg.Runner.Timeout = 10 * time.Second
	// Most tests assert exact item counts; the automatic linked test is
	// covered separately in TestAutoCreateLinkedTest.
	cfg.Requirements.AutoCreateTest = false
	for _, m := range mut {
		m(&cfg)
	}
	ctx := context.Background()
	ten, err := st.CreateTenant(ctx, "acme", "Acme Corp")
	if err != nil {
		t.Fatal(err)
	}
	// Most tests execute draft tests; the strict default execution policy is
	// covered in readiness_test.go (strictPolicy).
	if err := st.SaveTenantSettings(ctx, ten.ID, &model.TenantSettings{AllowUnreadyTests: true, AllowUnreadyRequirements: true, AllowUnmetDependencies: true}); err != nil {
		t.Fatal(err)
	}
	admin := mkUser(t, st, "admin@example.com", true)
	if err := st.SetMembership(ctx, ten.ID, admin.ID, rbac.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(st, cfg, log, runner.New(cfg.Runner), fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return &env{t: t, st: st, srv: srv, hs: hs, tenant: ten, admin: admin, cfg: cfg}
}

func mkUser(t *testing.T, st *store.Store, email string, super bool) *model.User {
	t.Helper()
	h, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	u := &model.User{Email: email, PasswordHash: h, SuperAdmin: super}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

// member creates a user with role in the env tenant.
func (e *env) member(email, role string) *model.User {
	u := mkUser(e.t, e.st, email, false)
	if err := e.st.SetMembership(context.Background(), e.tenant.ID, u.ID, role); err != nil {
		e.t.Fatal(err)
	}
	return u
}

type client struct {
	e      *env
	hc     *http.Client
	csrf   string
	bearer string
	// headers are added to every request
	headers map[string]string
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, hc: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (e *env) login(email string) *client {
	c := e.anon()
	var me meResponse
	c.call("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": testPassword}, 200, &me)
	c.csrf = me.CSRFToken
	return c
}

func (c *client) raw(method, path string, body any) *http.Response {
	c.e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		buf, _ := json.Marshal(b)
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.e.hs.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	return res
}

// call performs a request, asserts the status and decodes into out.
func (c *client) call(method, path string, body any, want int, out any) {
	c.e.t.Helper()
	res := c.raw(method, path, body)
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			c.e.t.Fatalf("%s %s: decode: %v: %s", method, path, err, data)
		}
	}
}

func (c *client) m(method, path string, body any, want int) map[string]any {
	c.e.t.Helper()
	out := map[string]any{}
	c.call(method, path, body, want, &out)
	return out
}

func items(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, x := range m["items"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func (c *client) project(key string) string {
	p := c.m("POST", "/api/v1/projects", map[string]any{"key": key, "name": key + " project"}, 201)
	return p["id"].(string)
}

// ---------------------------------------------------------------- tests

func TestHealthUIAndHeaders(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	if m := c.m("GET", "/healthz", nil, 200); m["status"] != "ok" {
		t.Fatal(m)
	}
	c.m("GET", "/readyz", nil, 200)
	res := c.raw("GET", "/some/spa/route", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "app") || res.Header.Get("Content-Security-Policy") == "" ||
		res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("spa fallback: %s %v", body, res.Header)
	}
	res = c.raw("GET", "/assets/app.js", nil)
	res.Body.Close()
	if !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatal(res.Header)
	}
	res = c.raw("POST", "/", nil)
	res.Body.Close()
	if res.StatusCode != 405 {
		t.Fatal(res.StatusCode)
	}
	c.m("GET", "/api/v1/nope", nil, 404)
	c.m("GET", "/api/v1/projects", nil, 401)

	e.srv.UI = nil
	res = c.raw("GET", "/", nil)
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal(res.StatusCode)
	}
	e.st.DB.Close()
	c.m("GET", "/readyz", nil, 503)
}

func TestAuthFlow(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	pr := c.m("GET", "/api/v1/auth/providers", nil, 200)
	if pr["local"] != true {
		t.Fatal(pr)
	}
	c.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong-password1"}, 401)
	c.m("POST", "/api/v1/auth/login", map[string]string{"email": "nobody@example.com", "password": "wrong-password1"}, 401)
	c.m("POST", "/api/v1/auth/login", "{bad", 400)
	c.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": testPassword, "tenant": "missing"}, 401)

	a := e.login("admin@example.com")
	me := a.m("GET", "/api/v1/auth/me", nil, 200)
	if me["role"] != "admin" || me["csrf_token"] == "" {
		t.Fatal(me)
	}
	// CSRF required for session mutations
	noCSRF := *a
	noCSRF.csrf = ""
	noCSRF.m("POST", "/api/v1/projects", map[string]any{"key": "X1", "name": "x"}, 403)

	// change password
	a.m("POST", "/api/v1/auth/password", map[string]string{"current_password": "nope", "new_password": "Another-pass-77"}, 403)
	a.m("POST", "/api/v1/auth/password", map[string]string{"current_password": testPassword, "new_password": "short"}, 400)
	a.call("POST", "/api/v1/auth/password", map[string]string{"current_password": testPassword, "new_password": "Another-pass-77"}, 204, nil)
	me2 := a.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "Another-pass-77", "tenant": "acme"}, 200)
	a.csrf = me2["csrf_token"].(string)

	// tenant switching
	other, _ := e.st.CreateTenant(context.Background(), "other", "Other")
	a.m("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": other.ID}, 200) // super admin
	a.m("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": "missing"}, 404)
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": other.ID}, 403)
	v.m("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": e.tenant.ID}, 200)
	v2 := e.anon()
	v2.m("POST", "/api/v1/auth/login", map[string]string{"email": "viewer@example.com", "password": testPassword, "tenant": "other"}, 403)

	// logout
	a.call("POST", "/api/v1/auth/logout", nil, 204, nil)
	a.m("GET", "/api/v1/auth/me", nil, 401)

	// local login disabled
	e.srv.Cfg.Auth.Local = false
	e.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "x@y.z", "password": "x"}, 403)
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	for i := 0; i < 10; i++ {
		c.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "bad-password-1"}, 401)
	}
	c.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": testPassword}, 429)
	l := newRateLimiter(1, time.Millisecond)
	if !l.allow("a") || l.allow("a") {
		t.Fatal("limiter")
	}
	time.Sleep(2 * time.Millisecond)
	if !l.allow("a") {
		t.Fatal("window")
	}
}

func TestAdminTenantsUsersMembers(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	if n := len(items(a.m("GET", "/api/v1/tenants", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	tn := a.m("POST", "/api/v1/tenants", map[string]string{"slug": "beta", "name": "Beta"}, 201)
	a.m("POST", "/api/v1/tenants", map[string]string{"slug": "beta", "name": "Beta"}, 409)
	a.m("POST", "/api/v1/tenants", map[string]string{"slug": "B!", "name": "x"}, 400)
	a.m("PATCH", "/api/v1/tenants/"+tn["id"].(string), map[string]string{"name": "Beta 2"}, 200)
	a.m("PATCH", "/api/v1/tenants/"+tn["id"].(string), map[string]string{"name": ""}, 400)
	a.call("DELETE", "/api/v1/tenants/"+tn["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/tenants/"+tn["id"].(string), nil, 404)

	u := a.m("POST", "/api/v1/users", map[string]any{"email": "New@Example.com", "display_name": "New", "password": testPassword}, 201)
	if u["email"] != "new@example.com" {
		t.Fatal(u)
	}
	a.m("POST", "/api/v1/users", map[string]any{"email": "weak@example.com", "password": "weak"}, 400)
	a.m("POST", "/api/v1/users", map[string]any{"email": "invalid"}, 400)
	a.m("PATCH", "/api/v1/users/"+u["id"].(string), map[string]any{"disabled": true}, 200)
	a.m("PATCH", "/api/v1/users/"+e.admin.ID, map[string]any{"super_admin": false}, 409)
	a.m("PATCH", "/api/v1/users/missing", map[string]any{}, 404)
	if n := len(items(a.m("GET", "/api/v1/users", nil, 200))); n != 2 {
		t.Fatal(n)
	}
	// disabled user cannot login
	e.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "new@example.com", "password": testPassword}, 401)

	// members
	m := a.m("POST", "/api/v1/members", map[string]any{"email": "tester@example.com", "role": "tester", "password": testPassword}, 201)
	uid := m["user_id"].(string)
	a.m("POST", "/api/v1/members", map[string]any{"email": "x@example.com", "role": "boss"}, 400)
	a.m("POST", "/api/v1/members", map[string]any{"email": "y@example.com", "role": "viewer", "password": "weak"}, 400)
	a.m("PATCH", "/api/v1/members/"+uid, map[string]any{"role": "manager"}, 200)
	a.m("PATCH", "/api/v1/members/"+uid, map[string]any{"role": "god"}, 400)
	a.m("PATCH", "/api/v1/members/"+e.admin.ID, map[string]any{"role": "viewer"}, 409) // last admin
	if n := len(items(a.m("GET", "/api/v1/members", nil, 200))); n != 2 {
		t.Fatal(n)
	}
	tc := e.login("tester@example.com")
	tc.m("GET", "/api/v1/users", nil, 403)
	tc.m("POST", "/api/v1/tenants", map[string]string{"slug": "zz", "name": "z"}, 403)
	tc.m("DELETE", "/api/v1/tenants/"+e.tenant.ID, nil, 403)
	tc.m("PATCH", "/api/v1/tenants/"+e.tenant.ID, map[string]string{"name": "x"}, 403)
	if n := len(items(tc.m("GET", "/api/v1/tenants", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	a.m("DELETE", "/api/v1/members/"+e.admin.ID, nil, 409)
	a.call("DELETE", "/api/v1/members/"+uid, nil, 204, nil)
	a.m("DELETE", "/api/v1/members/"+uid, nil, 404)
	tc.m("GET", "/api/v1/auth/me", nil, 401) // session removed with membership

	meta := a.m("GET", "/api/v1/meta", nil, 200)
	if meta["runner_enabled"] != true {
		t.Fatal(meta)
	}
	if r := a.m("GET", "/api/v1/rbac", nil, 200); r["matrix"] == nil {
		t.Fatal(r)
	}
	audit := a.m("GET", "/api/v1/audit?q=action:member.*", nil, 200)
	if len(items(audit)) < 2 {
		t.Fatal(audit)
	}
	a.m("GET", "/api/v1/audit?q=bogus:1", nil, 400)
}

func TestTokens(t *testing.T) {
	e := newEnv(t)
	e.member("tester@example.com", rbac.RoleTester)
	c := e.login("tester@example.com")
	c.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "role": "admin"}, 400)
	c.m("POST", "/api/v1/tokens", map[string]any{"name": ""}, 400)
	tok := c.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "expires_days": 30}, 201)
	secret := tok["token"].(string)
	if !strings.HasPrefix(secret, "qtf_") {
		t.Fatal(secret)
	}
	b := e.anon()
	b.bearer = secret
	if me := b.m("GET", "/api/v1/auth/me", nil, 200); me["role"] != "tester" {
		t.Fatal(me)
	}
	b.m("POST", "/api/v1/tokens", map[string]any{"name": "x"}, 403)
	b.m("POST", "/api/v1/auth/switch-tenant", map[string]any{"tenant_id": e.tenant.ID}, 400)
	b.m("POST", "/api/v1/projects", map[string]any{"key": "NOPE", "name": "x"}, 403)
	bad := e.anon()
	bad.bearer = "qtf_invalid"
	bad.m("GET", "/api/v1/auth/me", nil, 401)
	bad.bearer = "something"
	bad.m("GET", "/api/v1/auth/me", nil, 401)
	list := items(c.m("GET", "/api/v1/tokens", nil, 200))
	if len(list) != 1 || list[0]["last_used"] == nil {
		t.Fatal(list)
	}
	c.call("DELETE", "/api/v1/tokens/"+list[0]["id"].(string), nil, 204, nil)
	c.m("DELETE", "/api/v1/tokens/"+list[0]["id"].(string), nil, 404)
	b.m("GET", "/api/v1/auth/me", nil, 401)

	// token role is capped by current membership role
	tok2 := c.m("POST", "/api/v1/tokens", map[string]any{"name": "ci2"}, 201)
	u, _ := e.st.GetUserByEmail(context.Background(), "tester@example.com")
	_ = e.st.SetMembership(context.Background(), e.tenant.ID, u.ID, rbac.RoleViewer)
	b.bearer = tok2["token"].(string)
	if me := b.m("GET", "/api/v1/auth/me", nil, 200); me["role"] != "viewer" {
		t.Fatal(me)
	}
}

func TestProjectsFoldersCategories(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ALPHA")
	a.m("POST", "/api/v1/projects", map[string]any{"key": "ALPHA", "name": "dup"}, 409)
	a.m("POST", "/api/v1/projects", map[string]any{"key": "1bad", "name": "x"}, 400)
	a.m("POST", "/api/v1/projects", map[string]any{"key": "OK", "name": ""}, 400)
	bare := a.m("POST", "/api/v1/projects", map[string]any{"key": "BARE", "name": "bare", "seed_categories": false}, 201)
	if n := len(items(a.m("GET", "/api/v1/projects/"+bare["id"].(string)+"/categories", nil, 200))); n != 0 {
		t.Fatal(n)
	}
	a.m("GET", "/api/v1/projects/"+pid, nil, 200)
	a.m("GET", "/api/v1/projects/missing", nil, 404)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"name": "Alpha!", "description": "d"}, 200)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"name": ""}, 400)
	if n := len(items(a.m("GET", "/api/v1/projects", nil, 200))); n != 2 {
		t.Fatal(n)
	}

	// folders
	root := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "Root"}, 201)
	child := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "Child", "parent_id": root["id"]}, 201)
	grand := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "Grand", "parent_id": child["id"]}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "X", "parent_id": root["id"]}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "bogus", "name": "X"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": " "}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "x", "parent_id": "missing"}, 400)
	a.m("PATCH", "/api/v1/folders/"+child["id"].(string), map[string]any{"name": "Child2", "description": "d"}, 200)
	a.m("PATCH", "/api/v1/folders/"+child["id"].(string), map[string]any{"name": ""}, 400)
	a.m("PATCH", "/api/v1/folders/missing", map[string]any{"name": "x"}, 404)
	a.m("POST", "/api/v1/folders/"+root["id"].(string)+"/move", map[string]any{"parent_id": grand["id"]}, 409)
	a.m("POST", "/api/v1/folders/"+grand["id"].(string)+"/move", map[string]any{"parent_id": nil, "position": 0}, 200)
	a.m("POST", "/api/v1/folders/"+grand["id"].(string)+"/move", map[string]any{"parent_id": root["id"], "position": 5}, 200)
	a.m("POST", "/api/v1/folders/"+grand["id"].(string)+"/move", map[string]any{"parent_id": "missing"}, 400)
	fl := items(a.m("GET", "/api/v1/projects/"+pid+"/folders?kind=test", nil, 200))
	if len(fl) != 3 {
		t.Fatal(fl)
	}
	// test placed in grand, then move subtree to another project
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "in grand", "folder_id": grand["id"]}, 201)
	pid2 := bare["id"].(string)
	a.m("POST", "/api/v1/folders/"+root["id"].(string)+"/move", map[string]any{"parent_id": nil, "project_id": pid2}, 200)
	moved := a.m("GET", "/api/v1/tests/"+tc["id"].(string), nil, 200)
	if moved["project_id"] != pid2 || !strings.HasPrefix(moved["key"].(string), "BARE-TC-") || moved["folder_path"] != "Root / Grand" {
		t.Fatal(moved)
	}
	// delete without cascade lifts children
	a.call("DELETE", "/api/v1/folders/"+root["id"].(string), nil, 204, nil)
	fl = items(a.m("GET", "/api/v1/projects/"+pid2+"/folders?kind=test", nil, 200))
	if len(fl) != 2 {
		t.Fatal(fl)
	}
	// cascade delete removes items
	a.call("DELETE", "/api/v1/folders/"+grand["id"].(string)+"?cascade=true", nil, 204, nil)
	a.m("GET", "/api/v1/tests/"+tc["id"].(string), nil, 404)
	a.m("DELETE", "/api/v1/folders/missing", nil, 404)

	// categories
	cats := items(a.m("GET", "/api/v1/projects/"+pid+"/categories?scope=test", nil, 200))
	if len(cats) < 10 {
		t.Fatal(len(cats))
	}
	c := a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "Speed", "name": "Slow", "color": "#000"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "Speed", "name": "Slow"}, 409)
	a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "x", "dimension": "Speed", "name": "Slow"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "A/B", "name": "Slow"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "", "name": "Slow"}, 400)
	a.m("PATCH", "/api/v1/categories/"+c["id"].(string), map[string]any{"name": "Very slow"}, 200)
	a.m("PATCH", "/api/v1/categories/"+c["id"].(string), map[string]any{"name": ""}, 400)
	a.m("PATCH", "/api/v1/categories/missing", map[string]any{"name": "x"}, 404)
	a.m("PATCH", "/api/v1/categories/"+c["id"].(string), "", 400)
	// merge dimension: Execution/Manual exists in Execution; create Polarity2/Manual and move into Execution
	dup := a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "Mode", "name": "Manual"}, 201)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "cat test", "category_ids": []string{dup["id"].(string)}}, 201)
	mv := a.m("POST", "/api/v1/projects/"+pid+"/categories/move", map[string]any{"scope": "test", "dimension": "Mode", "new_dimension": "Execution"}, 200)
	if mv["moved"].(float64) != 1 {
		t.Fatal(mv)
	}
	got := a.m("GET", "/api/v1/tests/"+t1["id"].(string), nil, 200)
	if len(got["category_ids"].([]any)) != 1 || got["category_ids"].([]any)[0] == dup["id"] {
		t.Fatal(got)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/categories/move", map[string]any{"scope": "test", "dimension": "Speed", "new_dimension": "Velocity"}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/categories/move", map[string]any{"scope": "test", "dimension": "Nope", "new_dimension": "X"}, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/categories/move", map[string]any{"scope": "test", "dimension": "Nope", "new_dimension": ""}, 400)
	a.call("DELETE", "/api/v1/categories/"+c["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/categories/"+c["id"].(string), nil, 404)

	a.call("DELETE", "/api/v1/projects/"+pid, nil, 204, nil)
	a.m("DELETE", "/api/v1/projects/"+pid, nil, 404)
}

func TestRequirements(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("REQ")
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Security"}, 201)
	sub := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Auth", "parent_id": f["id"]}, 201)
	cats := items(a.m("GET", "/api/v1/projects/"+pid+"/categories?scope=requirement", nil, 200))
	catID := cats[0]["id"].(string)

	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{
		"title": "Login", "description": "The system MUST authenticate users.", "folder_id": sub["id"],
		"tags": []string{"auth", "auth", " "}, "category_ids": []string{catID},
		"criteria": []map[string]any{{"description": "valid login works"}, {"description": "audit", "method": "inspection", "threshold": "100%"}},
	}, 201)
	if r1["obligation"] != "must" || r1["key"] != "REQ-REQ-1" || len(r1["criteria"].([]any)) != 2 || len(r1["tags"].([]any)) != 1 {
		t.Fatal(r1)
	}
	id := r1["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": ""}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "obligation": "maybe"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "type": "x"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "priority": "x"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "status": "x"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "criteria": []map[string]any{{"description": ""}}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "criteria": []map[string]any{{"description": "a", "method": "magic"}}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "folder_id": "missing"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x", "category_ids": []string{"missing"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": strings.Repeat("x", 501)}, 400)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Export", "description": "Users MAY export data.", "status": "approved", "priority": "low"}, 201)
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Overview", "obligation": "describe"}, 201)

	got := a.m("GET", "/api/v1/requirements/"+id, nil, 200)
	if got["folder_path"] != "Security / Auth" || got["coverage"] != "none" {
		t.Fatal(got)
	}
	a.m("GET", "/api/v1/requirements/missing", nil, 404)

	// patch: keep first criterion id, drop second, add new
	crit := got["criteria"].([]any)
	first := crit[0].(map[string]any)
	first["description"] = "valid login works (updated)"
	up := a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"status": "approved", "criteria": []any{first, map[string]any{"description": "new"}}}, 200)
	nc := up["criteria"].([]any)
	if up["version"].(float64) != 2 || len(nc) != 2 || nc[0].(map[string]any)["id"] != first["id"] || up["key"] != "REQ-REQ-1" {
		t.Fatal(up)
	}
	up = a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"title": "Login v3", "id": "hijack", "key": "HACK"}, 200)
	if up["id"] != id || up["key"] != "REQ-REQ-1" || len(up["criteria"].([]any)) != 2 {
		t.Fatal(up)
	}
	a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"status": "bogus"}, 400)
	a.m("PATCH", "/api/v1/requirements/"+id, "[1]", 400)
	a.m("PATCH", "/api/v1/requirements/"+id, "", 400)
	a.m("PATCH", "/api/v1/requirements/missing", map[string]any{}, 404)

	// list + filters
	list := func(q string) []map[string]any {
		return items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?"+q, nil, 200))
	}
	if n := len(list("")); n != 3 {
		t.Fatal(n)
	}
	if l := list("q=" + url.QueryEscape("obligation:must OR obligation=may")); len(l) != 2 {
		t.Fatal(l)
	}
	if l := list("q=" + url.QueryEscape("title~/^log/i tag:auth criteria:inspection")); len(l) != 0 && len(l) != 1 {
		t.Fatal(l)
	}
	if l := list("q=" + url.QueryEscape("dimension:\""+cats[0]["dimension"].(string)+"/"+cats[0]["name"].(string)+"\"")); len(l) != 1 {
		t.Fatal(l)
	}
	if l := list("folder_id=" + f["id"].(string)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := list("folder_id=" + f["id"].(string) + "&recursive=false"); len(l) != 0 {
		t.Fatal(l)
	}
	if l := list("folder_id=root"); len(l) != 2 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/requirements?q="+url.QueryEscape("(status:"), nil, 400)

	// analyze
	an := a.m("POST", "/api/v1/reqlang/analyze", map[string]string{"text": "The system should be fast and/or easy."}, 200)
	if an["obligation"] != "should" || len(an["issues"].([]any)) == 0 {
		t.Fatal(an)
	}
	a.m("POST", "/api/v1/reqlang/analyze", "", 400)
	sv := a.m("POST", "/api/v1/reqlang/analyze", map[string]string{"text": "Systemet ska vara snabbt och/eller enkelt."}, 200)
	if sv["obligation"] != "shall" || sv["language"] != "sv" || len(sv["issues"].([]any)) < 2 {
		t.Fatal(sv)
	}
	if forced := a.m("POST", "/api/v1/reqlang/analyze", map[string]string{"text": "The system shall work.", "language": "sv"}, 200); forced["language"] != "sv" {
		t.Fatal(forced)
	}
	a.m("POST", "/api/v1/reqlang/analyze", map[string]string{"text": "x", "language": "xx"}, 400)

	// bulk
	ids := []string{id, r2["id"].(string)}
	b := a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": ids, "set": map[string]any{"priority": "high", "status": "review", "obligation": "shall", "add_category_ids": []string{catID}, "remove_category_ids": []string{catID}}}, 200)
	if b["updated"].(float64) != 2 {
		t.Fatal(b)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": ids, "set": map[string]any{"priority": "x"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": ids, "set": map[string]any{"execution_type": "manual"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{"missing"}, "set": map[string]any{"priority": "low"}}, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": ids, "set": map[string]any{"add_category_ids": []string{"nope"}}}, 400)

	// move within project then across projects
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{r3["id"].(string)}, "folder_id": f["id"], "position": 0}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{}, "folder_id": nil}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{"missing"}}, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{id}, "folder_id": "missing"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{id}, "project_id": "missing"}, 404)
	pid2 := a.project("REQTWO")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/move", map[string]any{"ids": []string{id}, "folder_id": nil, "project_id": pid2}, 200)
	mv := a.m("GET", "/api/v1/requirements/"+id, nil, 200)
	if mv["key"] != "REQTWO-REQ-1" || len(mv["category_ids"].([]any)) != 0 && mv["category_ids"] == nil {
		t.Fatal(mv)
	}
	// delete
	d := a.m("DELETE", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{r2["id"].(string)}}, 200)
	if d["deleted"].(float64) != 1 {
		t.Fatal(d)
	}
	a.call("DELETE", "/api/v1/requirements/"+r3["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/requirements/"+r3["id"].(string), nil, 404)
}

func TestTestsLinksRunsAndExecution(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TST")
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "Suite"}, 201)
	steps := []map[string]any{{"action": "open", "expected": "page"}, {"action": "click", "expected": "done", "data": "x"}}
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Manual login", "steps": steps, "folder_id": f["id"], "tags": []string{"smoke"}}, 201)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Auto pass", "execution_type": "automated", "script": "echo hello $QTF_TEST_KEY"}, 201)
	_ = a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Auto fail", "execution_type": "automated", "script_language": "sh", "script": "echo boom; exit 2"}, 201)
	if t2["script_language"] != "sh" || len(t1["steps"].([]any)) != 2 {
		t.Fatal(t2, t1)
	}
	for _, bad := range []map[string]any{{"title": ""}, {"title": "x", "execution_type": "robot"}, {"title": "x", "script_language": "cobol"},
		{"title": "x", "priority": "x"}, {"title": "x", "status": "x"}, {"title": "x", "estimated_minutes": -1}, {"title": strings.Repeat("y", 600)}} {
		a.m("POST", "/api/v1/projects/"+pid+"/tests", bad, 400)
	}
	id1 := t1["id"].(string)
	up := a.m("PATCH", "/api/v1/tests/"+id1, map[string]any{"status": "ready", "steps": []map[string]any{{"action": "only step", "expected": "ok"}}}, 200)
	if len(up["steps"].([]any)) != 1 {
		t.Fatal(up)
	}
	up = a.m("PATCH", "/api/v1/tests/"+id1, map[string]any{"description": "d"}, 200)
	if len(up["steps"].([]any)) != 1 || up["last_result"] != "not_run" {
		t.Fatal(up)
	}
	a.m("PATCH", "/api/v1/tests/"+id1, map[string]any{"priority": "x"}, 400)
	a.m("PATCH", "/api/v1/tests/"+id1, "{", 400)
	a.m("PATCH", "/api/v1/tests/missing", map[string]any{}, 404)
	cp := a.m("POST", "/api/v1/tests/"+id1+"/copy", nil, 201)
	if !strings.HasPrefix(cp["title"].(string), "Copy of") || cp["key"] == t1["key"] {
		t.Fatal(cp)
	}
	a.m("POST", "/api/v1/tests/missing/copy", nil, 404)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q="+url.QueryEscape("type:automated script~/exit/"), nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q="+url.QueryEscape("step:\"only step\""), nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?folder_id="+f["id"].(string), nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/tests?q=nope:x", nil, 400)
	a.m("GET", "/api/v1/tests/missing", nil, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{cp["id"].(string)}, "set": map[string]any{"execution_type": "automated", "status": "deprecated"}}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{cp["id"].(string)}, "set": map[string]any{"obligation": "must"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/tests/move", map[string]any{"ids": []string{cp["id"].(string)}, "folder_id": f["id"]}, 200)
	a.m("DELETE", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{cp["id"].(string)}}, 200)
	a.m("DELETE", "/api/v1/tests/missing", nil, 404)

	// links
	r := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "MUST login", "criteria": []map[string]any{{"description": "c1"}}}, 201)
	rid := r["id"].(string)
	critID := r["criteria"].([]any)[0].(map[string]any)["id"]
	l1 := a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": id1, "criterion_id": critID}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": id1}, 409)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": t2["id"], "criterion_id": "bogus"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": "missing", "test_case_id": id1}, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": "missing"}, 404)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": t2["id"]}, 201)
	other := a.project("OTH")
	ot := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "o"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": ot["id"]}, 400)
	or := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "o"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": or["id"], "test_case_id": ot["id"]}, 400)
	if l := items(a.m("GET", "/api/v1/requirements/"+rid+"/links", nil, 200)); len(l) != 2 || l[0]["test_case_key"] == "" {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/tests/"+id1+"/links", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/tests/missing/links", nil, 404)
	a.m("GET", "/api/v1/requirements/missing/links", nil, 404)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/links", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q=linked:"+t1["key"].(string), nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// dropping the criterion detaches it from the link
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"criteria": []any{}}, 200)

	// runs
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R1", "environment": "staging", "build": "1.0",
		"test_ids": []string{id1}, "folder_ids": []string{f["id"].(string)}, "query": "type:automated"}, 201)
	runID := run["id"].(string)
	if run["summary"].(map[string]any)["total"].(float64) != 3 {
		t.Fatal(run)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": ""}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "x", "query": "bad:"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "x", "test_ids": []string{"missing"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "x", "folder_ids": []string{"missing"}}, 201)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/runs?q=env:staging", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/runs?q=x:y", nil, 400)
	execs := items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200))
	if len(execs) != 3 {
		t.Fatal(execs)
	}
	a.m("GET", "/api/v1/runs/"+runID+"/executions?q=zz:1", nil, 400)
	a.m("GET", "/api/v1/runs/missing/executions", nil, 404)
	if l := items(a.m("GET", "/api/v1/runs/"+runID+"/executions?q=status:not_run", nil, 200)); len(l) != 3 {
		t.Fatal(l)
	}
	byTitle := map[string]string{}
	for _, x := range execs {
		byTitle[x["test_title"].(string)] = x["id"].(string)
	}
	// manual execution with step results
	ex := a.m("GET", "/api/v1/executions/"+byTitle["Manual login"], nil, 200)
	srs := ex["step_results"].([]any)
	if len(srs) != 1 || ex["test"] == nil {
		t.Fatal(ex)
	}
	sr := srs[0].(map[string]any)
	ex = a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []map[string]any{{"id": sr["id"], "status": "failed", "actual": "error"}}, "notes": "bug 1", "assignee": "bob"}, 200)
	if ex["status"] != "failed" || ex["executed_by"] != "admin@example.com" || ex["finished_at"] == nil {
		t.Fatal(ex)
	}
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []map[string]any{{"id": "nope", "status": "passed"}}}, 400)
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []map[string]any{{"id": sr["id"], "status": "meh"}}}, 400)
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "meh"}, 400)
	a.m("PATCH", "/api/v1/executions/missing", map[string]any{}, 404)
	a.m("GET", "/api/v1/executions/missing", nil, 404)
	if g := a.m("GET", "/api/v1/runs/"+runID, nil, 200); g["status"] != "in_progress" || g["started_at"] == nil {
		t.Fatal(g)
	}
	// scripts
	ex = a.m("POST", "/api/v1/executions/"+byTitle["Auto pass"]+"/run-script", nil, 200)
	if ex["status"] != "passed" || !strings.Contains(ex["output"].(string), "hello TST-TC-2") {
		t.Fatal(ex)
	}
	ex = a.m("POST", "/api/v1/executions/"+byTitle["Auto fail"]+"/run-script", nil, 200)
	if ex["status"] != "failed" || !strings.Contains(ex["output"].(string), "boom") {
		t.Fatal(ex)
	}
	a.m("POST", "/api/v1/executions/"+byTitle["Manual login"]+"/run-script", nil, 400)
	a.m("POST", "/api/v1/executions/missing/run-script", nil, 404)
	e.srv.Runner.Cfg.Enabled = false
	a.m("POST", "/api/v1/executions/"+byTitle["Auto pass"]+"/run-script", nil, 409)
	e.srv.Runner.Cfg.Enabled = true

	// coverage now: req linked to manual(failed) + auto pass => failed
	rq := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if rq["coverage"] != "failed" || rq["linked_tests"].(float64) != 2 {
		t.Fatal(rq)
	}
	// CI import
	imp := a.m("POST", "/api/v1/runs/"+runID+"/results", map[string]any{"results": []map[string]any{
		{"test_key": t1["key"], "status": "passed", "duration_ms": 1200, "output": "ok"},
		{"test_key": "tst-tc-99", "status": "passed"},
		{"test_key": ot["key"], "status": "passed"},
	}}, 200)
	if imp["updated"].(float64) != 1 || len(imp["unknown"].([]any)) != 2 {
		t.Fatal(imp)
	}
	a.m("POST", "/api/v1/runs/"+runID+"/results", map[string]any{"results": []map[string]any{{"test_key": t1["key"], "status": "great"}}}, 400)
	a.m("POST", "/api/v1/runs/missing/results", map[string]any{"results": []any{}}, 404)
	if rq := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); rq["coverage"] != "passed" {
		t.Fatal(rq)
	}
	// add executions
	t4 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "late"}, 201)
	if r := a.m("POST", "/api/v1/runs/"+runID+"/executions", map[string]any{"test_ids": []string{t4["id"].(string), id1}}, 200); r["added"].(float64) != 1 {
		t.Fatal(r)
	}
	a.m("POST", "/api/v1/runs/missing/executions", map[string]any{"test_ids": []string{}}, 404)
	// reset an execution
	a.m("PATCH", "/api/v1/executions/"+byTitle["Auto fail"], map[string]any{"status": "not_run"}, 200)
	// update run
	g := a.m("PATCH", "/api/v1/runs/"+runID, map[string]any{"status": "completed", "name": "R1 final"}, 200)
	if g["completed_at"] == nil {
		t.Fatal(g)
	}
	g = a.m("PATCH", "/api/v1/runs/"+runID, map[string]any{"status": "in_progress"}, 200)
	if g["completed_at"] != nil {
		t.Fatal(g)
	}
	a.m("PATCH", "/api/v1/runs/"+runID, map[string]any{"status": "weird"}, 400)
	a.m("PATCH", "/api/v1/runs/"+runID, map[string]any{"name": " "}, 400)
	a.m("PATCH", "/api/v1/runs/missing", map[string]any{}, 404)
	a.m("GET", "/api/v1/runs/missing", nil, 404)

	// traceability & dashboard
	tr := a.m("GET", "/api/v1/projects/"+pid+"/traceability", nil, 200)
	if len(tr["requirements"].([]any)) != 1 || len(tr["links"].([]any)) != 2 {
		t.Fatal(tr)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/traceability?q=bad:", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/traceability?tq=bad:", nil, 400)
	dash := a.m("GET", "/api/v1/projects/"+pid+"/dashboard", nil, 200)
	if dash["requirements"].(map[string]any)["total"].(float64) != 1 || len(dash["recent_runs"].([]any)) == 0 {
		t.Fatal(dash)
	}
	a.m("GET", "/api/v1/projects/missing/dashboard", nil, 404)

	// search
	s := a.m("GET", "/api/v1/search?q=login", nil, 200)
	if s["total"].(float64) < 2 {
		t.Fatal(s)
	}
	s = a.m("GET", "/api/v1/search?q="+url.QueryEscape("kind:run R1")+"&types=run&limit=5", nil, 200)
	if s["total"].(float64) != 1 {
		t.Fatal(s)
	}
	s = a.m("GET", "/api/v1/search?q="+url.QueryEscape("project:OTH")+"&project_id="+other, nil, 200)
	if s["total"].(float64) != 2 {
		t.Fatal(s)
	}
	if s := a.m("GET", "/api/v1/search?q=", nil, 200); s["total"].(float64) != 0 {
		t.Fatal(s)
	}
	a.m("GET", "/api/v1/search?q=unknownfield:1", nil, 400)

	// export
	for _, f := range []string{"xlsx", "csv", "pdf", "docx"} {
		res := a.raw("GET", "/api/v1/projects/"+pid+"/export?format="+f, nil)
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || len(data) < 100 || !strings.Contains(res.Header.Get("Content-Disposition"), "TST-") {
			t.Fatalf("%s: %d %d %v", f, res.StatusCode, len(data), res.Header)
		}
	}
	res := a.raw("GET", "/api/v1/projects/"+pid+"/export?format=csv&aspects=requirements&q="+url.QueryEscape("execution_type:automated"), nil)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.Header.Get("Content-Type") != "text/csv; charset=utf-8" && !strings.HasPrefix(res.Header.Get("Content-Type"), "text/csv") || !strings.Contains(string(data), "TST-REQ-1") {
		t.Fatalf("%v %s", res.Header, data)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/export?format=rtf", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/export?format=csv&aspects=secrets", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/export?format=csv&q=bad%3A%3A", nil, 400)

	a.call("DELETE", "/api/v1/runs/"+runID, nil, 204, nil)
	a.m("DELETE", "/api/v1/runs/"+runID, nil, 404)
	a.call("DELETE", "/api/v1/links/"+l1["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/links/"+l1["id"].(string), nil, 404)
}

func TestRBACEnforcement(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RB")
	e.member("viewer@example.com", rbac.RoleViewer)
	e.member("tester@example.com", rbac.RoleTester)
	v := e.login("viewer@example.com")
	ts := e.login("tester@example.com")
	v.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200)
	v.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x"}, 403)
	v.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "x"}, 403)
	v.m("GET", "/api/v1/audit", nil, 403)
	ts.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "x"}, 403)
	tc := ts.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "x"}, 201)
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "R"}, 201)
	ts.m("PATCH", "/api/v1/folders/"+f["id"].(string), map[string]any{"name": "y"}, 403)
	ts.m("DELETE", "/api/v1/projects/"+pid, nil, 403)
	run := ts.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "r", "test_ids": []string{tc["id"].(string)}}, 201)
	ex := items(ts.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	ts.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "passed"}, 200)
	ts.m("POST", "/api/v1/executions/"+ex["id"].(string)+"/run-script", nil, 403)
	// tenant isolation: admin of another tenant cannot see project
	ctx := context.Background()
	other, _ := e.st.CreateTenant(ctx, "evil", "Evil")
	u := mkUser(t, e.st, "evil@example.com", false)
	_ = e.st.SetMembership(ctx, other.ID, u.ID, rbac.RoleAdmin)
	ev := e.login("evil@example.com")
	ev.m("GET", "/api/v1/projects/"+pid, nil, 404)
	ev.m("GET", "/api/v1/tests/"+tc["id"].(string), nil, 404)
	ev.m("GET", "/api/v1/runs/"+run["id"].(string), nil, 404)
	if n := len(items(ev.m("GET", "/api/v1/projects", nil, 200))); n != 0 {
		t.Fatal(n)
	}
	// user without tenant
	mkUser(t, e.st, "lonely@example.com", false)
	lo := e.login("lonely@example.com")
	if me := lo.m("GET", "/api/v1/auth/me", nil, 200); me["tenant"] != nil {
		t.Fatal(me)
	}
	lo.m("GET", "/api/v1/projects", nil, 403)
}

func TestHelpers(t *testing.T) {
	if s := snippet("short text", []string{"x"}); s != "short text" {
		t.Fatal(s)
	}
	long := strings.Repeat("lorem ipsum ", 40) + "NEEDLE " + strings.Repeat("dolor ", 40)
	if s := snippet(long, []string{"needle"}); !strings.Contains(s, "NEEDLE") || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Fatal(s)
	}
	if s := snippet(long, nil); !strings.HasSuffix(s, "…") {
		t.Fatal(s)
	}
	if truncateRunes("äöü", 2) != "äö" {
		t.Fatal("runes")
	}
	srv := &Server{Cfg: config.Config{Server: config.Server{TrustedProxies: []string{"127.0.0.0/8", "10.1.1.1"}}}}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	if ip := srv.clientIP(r); ip != "2.2.2.2" {
		t.Fatal(ip)
	}
	r.RemoteAddr = "10.1.1.1:1"
	if ip := srv.clientIP(r); ip != "2.2.2.2" {
		t.Fatal(ip)
	}
	r.RemoteAddr = "8.8.8.8:1"
	if ip := srv.clientIP(r); ip != "8.8.8.8" {
		t.Fatal(ip)
	}
	r.RemoteAddr = "weird"
	if ip := srv.clientIP(r); ip != "weird" {
		t.Fatal(ip)
	}
	if reqExpr("type:automated") != "type:automated" || testExpr("obligation:must") != "" || reqExpr("execution_type:manual") != "" {
		t.Fatal("expr routing")
	}
	if displayName("", "n") != "n" || displayName("d", "n") != "d" {
		t.Fatal("display")
	}
	r = httptest.NewRequest("GET", "/?b=yes&c=0&d=zz", nil)
	if !boolParam(r, "b", false) || boolParam(r, "c", true) || !boolParam(r, "d", true) {
		t.Fatal("boolParam")
	}
	// panic recovery
	srv.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := httptest.NewRecorder()
	srv.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 {
		t.Fatal(rec.Code)
	}
	// large body
	big := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("a", MaxBodyBytes+10)))
	if _, err := readBody(big); err == nil {
		t.Fatal("expected too large")
	}
	if _, err := patchKeys([]byte("[")); err == nil {
		t.Fatal("patchKeys")
	}
	_ = fmt.Sprint(listOf[int](nil))
}

func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	other := e.login("admin@example.com")
	a.call("POST", "/api/v1/auth/password", map[string]string{"current_password": testPassword, "new_password": "Brand-new-pass-9"}, 204, nil)
	a.m("GET", "/api/v1/auth/me", nil, 200)
	other.m("GET", "/api/v1/auth/me", nil, 401)
}

func TestAutoCreateLinkedTest(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Requirements.AutoCreateTest = true })
	a := e.login("admin@example.com")
	pid := a.project("AUTO")
	if m := a.m("GET", "/api/v1/meta", nil, 200); m["auto_create_test"] != true {
		t.Fatal(m)
	}
	top := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Security"}, 201)
	sub := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Auth", "parent_id": top["id"]}, 201)

	// default: a linked test is created in a mirrored folder, one step per criterion
	r := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{
		"title": "Login", "description": "The system MUST authenticate users.", "folder_id": sub["id"], "priority": "high",
		"criteria": []map[string]any{{"description": "valid login works"}, {"description": "p95 latency", "method": "analysis", "threshold": "< 1 s"}},
	}, 201)
	lt, ok := r["linked_test"].(map[string]any)
	if !ok || r["linked_tests"].(float64) != 1 || r["coverage"] != "not_run" {
		t.Fatal(r)
	}
	tc := a.m("GET", "/api/v1/tests/"+lt["id"].(string), nil, 200)
	steps := tc["steps"].([]any)
	if tc["folder_path"] != "Security / Auth" || tc["priority"] != "high" || tc["status"] != "draft" || len(steps) != 2 ||
		!strings.HasPrefix(tc["title"].(string), "Verify AUTO-REQ-1: Login") || steps[1].(map[string]any)["expected"] != "< 1 s" {
		t.Fatal(tc)
	}
	links := items(a.m("GET", "/api/v1/requirements/"+r["id"].(string)+"/links", nil, 200))
	if len(links) != 1 || links[0]["test_case_id"] != lt["id"] {
		t.Fatal(links)
	}
	// a second requirement in the same folder reuses the mirrored test folders
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout", "folder_id": sub["id"]}, 201)
	tc2 := a.m("GET", "/api/v1/tests/"+r2["linked_test"].(map[string]any)["id"].(string), nil, 200)
	if tc2["folder_id"] != tc["folder_id"] || len(tc2["steps"].([]any)) != 1 {
		t.Fatal(tc2)
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/folders?kind=test", nil, 200))); n != 2 {
		t.Fatal(n)
	}
	// top-level requirement -> top-level test
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Top"}, 201)
	if r3["linked_test"].(map[string]any)["folder_id"] != nil {
		t.Fatal(r3)
	}
	// opt out per request
	r4 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "No test", "create_test": false}, 201)
	if r4["linked_test"] != nil || r4["linked_tests"].(float64) != 0 {
		t.Fatal(r4)
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/tests", nil, 200))); n != 3 {
		t.Fatal(n)
	}

	// a second writer role also gets the linked test
	e.st.SetMembership(context.Background(), e.tenant.ID, mkUser(t, e.st, "w@example.com", false).ID, rbac.RoleManager)
	m := e.login("w@example.com")
	if r := m.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "M"}, 201); r["linked_test"] == nil {
		t.Fatal("manager has test.write and should get a linked test")
	}

	// opt-in when the server default is off
	e.srv.Cfg.Requirements.AutoCreateTest = false
	if r := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Off"}, 201); r["linked_test"] != nil {
		t.Fatal(r)
	}
	if r := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Opt in", "create_test": true}, 201); r["linked_test"] == nil {
		t.Fatal(r)
	}
}
