package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/rbac"
)

// fakeIdP is a minimal OpenID Connect provider.
type fakeIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	nonce  string
	claims map[string]any
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/auth", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "good" || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, 400)
			return
		}
		f.mu.Lock()
		claims := map[string]any{"iss": f.srv.URL, "aud": "qtf", "sub": "sub-1", "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "nonce": f.nonce}
		for k, v := range f.claims {
			claims[k] = v
		}
		f.mu.Unlock()
		sig, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		payload, _ := json.Marshal(claims)
		obj, _ := sig.Sign(payload)
		tok, _ := obj.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": tok, "expires_in": 3600})
	})
	return f
}

func TestOIDCLoginFlow(t *testing.T) {
	idp := newFakeIdP(t)
	e := newEnv(t, func(c *config.Config) {
		c.Auth.OIDC = []config.OIDC{{Name: "corp", DisplayName: "Corp", Issuer: idp.srv.URL, ClientID: "qtf", ClientSecret: "s",
			Provisioning: config.Provisioning{AutoProvision: true, DefaultTenant: "acme", DefaultRole: "viewer", RoleMapping: map[string]string{"qa": "tester"}}}}
	})
	c := e.anon()
	prov := c.m("GET", "/api/v1/auth/providers", nil, 200)
	if len(prov["oidc"].([]any)) != 1 {
		t.Fatal(prov)
	}
	login := func(claims map[string]any, code string) *http.Response {
		res := c.raw("GET", "/api/v1/auth/oidc/corp/login", nil)
		res.Body.Close()
		if res.StatusCode != 302 {
			t.Fatal(res.StatusCode)
		}
		loc, _ := url.Parse(res.Header.Get("Location"))
		q := loc.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
			t.Fatal(loc)
		}
		idp.mu.Lock()
		idp.nonce, idp.claims = q.Get("nonce"), claims
		idp.mu.Unlock()
		res = c.raw("GET", "/api/v1/auth/oidc/corp/callback?state="+url.QueryEscape(q.Get("state"))+"&code="+code, nil)
		res.Body.Close()
		return res
	}
	res := login(map[string]any{"email": "sso@example.com", "name": "SSO User", "groups": []any{"qa"}}, "good")
	if res.StatusCode != 302 || res.Header.Get("Location") != "/" {
		t.Fatal(res.StatusCode, res.Header.Get("Location"))
	}
	me := c.m("GET", "/api/v1/auth/me", nil, 200)
	if me["role"] != "tester" || me["user"].(map[string]any)["auth_provider"] != "oidc:corp" {
		t.Fatal(me)
	}
	// second login of the same subject maps to the same user
	res = login(map[string]any{"email": "sso@example.com", "groups": []any{}}, "good")
	if res.StatusCode != 302 || res.Header.Get("Location") != "/" {
		t.Fatal(res.Header.Get("Location"))
	}
	if me := c.m("GET", "/api/v1/auth/me", nil, 200); me["role"] != "viewer" { // mapping authoritative
		t.Fatal(me)
	}
	// failures redirect to /login?error
	for _, tc := range []struct {
		claims map[string]any
		code   string
	}{{map[string]any{"email": "x@example.com"}, "bad"}, {map[string]any{}, "good"}, {map[string]any{"email": "v@example.com", "email_verified": false}, "good"}} {
		res = login(tc.claims, tc.code)
		if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
			t.Fatal(tc, res.Header.Get("Location"))
		}
	}
	// callback without state cookie / with IdP error / unknown provider
	c2 := e.anon()
	res = c2.raw("GET", "/api/v1/auth/oidc/corp/callback?state=x&code=good", nil)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
	res = c.raw("GET", "/api/v1/auth/oidc/corp/login", nil)
	res.Body.Close()
	res = c.raw("GET", "/api/v1/auth/oidc/corp/callback?error=access_denied", nil)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
	c.m("GET", "/api/v1/auth/oidc/nope/login", nil, 404)
	c.m("GET", "/api/v1/auth/oidc/nope/callback", nil, 404)
	// discovery failure
	idp.srv.Close()
	e.srv.OIDC["corp"] = auth.NewOIDCProvider(e.srv.OIDC["corp"].Cfg, "http://x/cb")
	res = c.raw("GET", "/api/v1/auth/oidc/corp/login", nil)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
}

func TestProvision(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	prov := config.Provisioning{AutoProvision: false, DefaultTenant: "acme", DefaultRole: "bogus"}
	id := &auth.Identity{Provider: "saml:x", Subject: "s1", Email: "new@example.com", Groups: []string{"g"}}
	if _, _, err := e.srv.provision(ctx, id, prov); err == nil {
		t.Fatal("expected not provisioned")
	}
	// existing local account gets linked; default role falls back to viewer
	mkUser(t, e.st, "local@example.com", false)
	prov.AutoProvision = true
	u, tid, err := e.srv.provision(ctx, &auth.Identity{Provider: "saml:x", Subject: "s2", Email: "local@example.com"}, prov)
	if err != nil || tid != e.tenant.ID || u.AuthProvider != "saml:x" || u.PasswordHash != "" {
		t.Fatal(u, err)
	}
	m, _ := e.st.GetMembership(ctx, e.tenant.ID, u.ID)
	if m.Role != rbac.RoleViewer {
		t.Fatal(m)
	}
	// bound to another provider
	if _, _, err := e.srv.provision(ctx, &auth.Identity{Provider: "oidc:y", Subject: "s3", Email: "local@example.com"}, prov); err == nil {
		t.Fatal("expected conflict")
	}
	// disabled
	u.Disabled = true
	_ = e.st.UpdateUser(ctx, u)
	if _, _, err := e.srv.provision(ctx, &auth.Identity{Provider: "saml:x", Subject: "s2", Email: "local@example.com"}, prov); err == nil {
		t.Fatal("expected disabled")
	}
	// unknown tenant
	prov.DefaultTenant = "missing"
	if _, _, err := e.srv.provision(ctx, &auth.Identity{Provider: "saml:x", Subject: "s9", Email: "z@example.com"}, prov); err == nil {
		t.Fatal("expected tenant error")
	}
	// no default tenant: user only
	prov.DefaultTenant = ""
	if _, tid, err := e.srv.provision(ctx, &auth.Identity{Provider: "saml:x", Subject: "s10", Email: "q@example.com"}, prov); err != nil || tid != "" {
		t.Fatal(err)
	}
}

const idpMetadata = `<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.test/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`

func TestSAMLHandlers(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "idp.xml")
	if err := os.WriteFile(md, []byte(idpMetadata), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, func(c *config.Config) {
		c.Auth.SAML = []config.SAML{{Name: "adfs", IDPMetadataFile: md}, {Name: "broken", IDPMetadataFile: filepath.Join(dir, "missing.xml")}}
	})
	c := e.anon()
	res := c.raw("GET", "/api/v1/auth/saml/adfs/metadata", nil)
	buf := make([]byte, 4096)
	n, _ := res.Body.Read(buf)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(buf[:n]), "AssertionConsumerService") {
		t.Fatal(res.StatusCode, string(buf[:n]))
	}
	res = c.raw("GET", "/api/v1/auth/saml/adfs/login", nil)
	res.Body.Close()
	if res.StatusCode != 302 || !strings.HasPrefix(res.Header.Get("Location"), "https://idp.example.test/sso?SAMLRequest=") {
		t.Fatal(res.Header)
	}
	// ACS with garbage response (the state cookie is Secure so plain-http test clients do not resend it)
	res = c.raw("POST", "/api/v1/auth/saml/adfs/acs", "SAMLResponse=bogus")
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
	// valid state cookie but invalid response
	st, _ := e.srv.signer.Sign(auth.SAMLState{RequestID: "id-1"}, time.Minute)
	req, _ := http.NewRequest("POST", e.hs.URL+"/api/v1/auth/saml/adfs/acs", strings.NewReader("SAMLResponse=bogus"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: st})
	res, _ = c.hc.Do(req)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
	c.m("GET", "/api/v1/auth/saml/broken/metadata", nil, 502)
	res = c.raw("GET", "/api/v1/auth/saml/broken/login", nil)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Location"), "/login?error=") {
		t.Fatal(res.Header)
	}
	for _, p := range []string{"metadata", "login"} {
		c.m("GET", "/api/v1/auth/saml/nope/"+p, nil, 404)
	}
	c.m("POST", "/api/v1/auth/saml/nope/acs", nil, 404)
	if _, err := New(e.st, config.Config{Server: config.Server{BaseURL: "::bad"}, Auth: config.Auth{SAML: []config.SAML{{Name: "x"}}}}, e.srv.Log, nil, nil); err == nil {
		t.Fatal("expected bad base url error")
	}
}

func TestOriginAndCategoryPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ORG")
	e.member("tester@example.com", rbac.RoleTester)
	e.member("viewer@example.com", rbac.RoleViewer)
	ts := e.login("tester@example.com")
	v := e.login("viewer@example.com")
	c := ts.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "test", "dimension": "D", "name": "N"}, 201)
	ts.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "requirement", "dimension": "D", "name": "N"}, 403)
	v.m("PATCH", "/api/v1/categories/"+c["id"].(string), map[string]any{"name": "M"}, 403)
	v.m("DELETE", "/api/v1/categories/"+c["id"].(string), nil, 403)
	v.m("POST", "/api/v1/projects/"+pid+"/categories/move", map[string]any{"scope": "test", "dimension": "D", "new_dimension": "E"}, 403)
	ts.m("DELETE", "/api/v1/categories/missing", nil, 404)
	ts.call("DELETE", "/api/v1/categories/"+c["id"].(string), nil, 204, nil)

	for origin, want := range map[string]int{"https://evil.example": 403, "http://example.test": 201, e.hs.URL: 201, "%zz": 403} {
		req, _ := http.NewRequest("POST", e.hs.URL+"/api/v1/projects/"+pid+"/categories", strings.NewReader(`{"scope":"test","dimension":"O","name":"`+url.QueryEscape(origin)+`"}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", a.csrf)
		res, err := a.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatal(origin, res.StatusCode)
		}
	}
}
