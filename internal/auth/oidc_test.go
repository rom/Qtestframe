package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
	otherKey    *rsa.PrivateKey
)

func rsaKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	testKeyOnce.Do(func() {
		var err error
		if testKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
		if otherKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
	})
	return testKey, otherKey
}

var b64 = base64.RawURLEncoding

func signJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	c, _ := json.Marshal(claims)
	in := b64.EncodeToString(h) + "." + b64.EncodeToString(c)
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + b64.EncodeToString(sig)
}

// fakeIdP is a minimal OpenID Provider.
type fakeIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu sync.Mutex
	// claims returns the id_token claims for the given nonce; nil -> no id_token.
	claims      func(nonce string) map[string]any
	signKey     *rsa.PrivateKey
	tokenStatus int
	nonce       string // nonce seen on the authorize URL (set by the test)
	challenge   string
	gotVerifier string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, _ := rsaKeys(t)
	f := &fakeIdP{t: t, key: key, signKey: key, tokenStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		u := f.srv.URL
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                u,
			"authorization_endpoint":                u + "/authorize",
			"token_endpoint":                        u + "/token",
			"jwks_uri":                              u + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
			"n": b64.EncodeToString(f.key.N.Bytes()),
			"e": b64.EncodeToString(big.NewInt(int64(f.key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		f.gotVerifier = r.PostForm.Get("code_verifier")
		if f.tokenStatus != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.tokenStatus)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		if r.PostForm.Get("code") != "good-code" || r.PostForm.Get("grant_type") != "authorization_code" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		resp := map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600}
		if f.claims != nil {
			if c := f.claims(f.nonce); c != nil {
				resp["id_token"] = signJWT(f.t, f.signKey, c)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIdP) baseClaims(nonce string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": f.srv.URL, "aud": "client-1", "sub": "sub-123",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": nonce,
		"email": " alice@example.com ", "name": "Alice", "email_verified": true,
		"groups": []any{"qa", 42, "admins"},
	}
}

func newProvider(f *fakeIdP, scopes ...string) *OIDCProvider {
	p := NewOIDCProvider(config.OIDC{Name: "corp", Issuer: f.srv.URL, ClientID: "client-1", ClientSecret: "s3cret", Scopes: scopes}, "https://qtf.example/cb")
	p.HTTPClient = f.srv.Client()
	return p
}

// login runs AuthURL and records nonce/challenge at the fake IdP.
func login(t *testing.T, f *fakeIdP, p *OIDCProvider) OIDCState {
	t.Helper()
	raw, st, err := p.AuthURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(raw, f.srv.URL+"/authorize?") {
		t.Errorf("auth url %q", raw)
	}
	if q.Get("state") != st.State || q.Get("nonce") != st.Nonce || q.Get("client_id") != "client-1" ||
		q.Get("redirect_uri") != "https://qtf.example/cb" || q.Get("response_type") != "code" ||
		q.Get("code_challenge_method") != "S256" {
		t.Errorf("auth url query: %v", q)
	}
	sum := sha256.Sum256([]byte(st.Verifier))
	if q.Get("code_challenge") != b64.EncodeToString(sum[:]) {
		t.Errorf("code_challenge mismatch")
	}
	if !strings.HasPrefix(q.Get("scope"), "openid") {
		t.Errorf("scope %q", q.Get("scope"))
	}
	f.mu.Lock()
	f.nonce, f.challenge = q.Get("nonce"), q.Get("code_challenge")
	f.mu.Unlock()
	return st
}

func TestNewOIDCProviderDefaults(t *testing.T) {
	p := NewOIDCProvider(config.OIDC{Name: "x"}, "cb")
	if p.Cfg.EmailClaim != "email" || p.Cfg.NameClaim != "name" || p.Cfg.GroupsClaim != "groups" || len(p.Cfg.Scopes) != 3 {
		t.Errorf("defaults: %+v", p.Cfg)
	}
	p = NewOIDCProvider(config.OIDC{EmailClaim: "mail", NameClaim: "cn", GroupsClaim: "roles", Scopes: []string{"openid"}}, "cb")
	if p.Cfg.EmailClaim != "mail" || p.Cfg.NameClaim != "cn" || p.Cfg.GroupsClaim != "roles" || len(p.Cfg.Scopes) != 1 {
		t.Errorf("custom: %+v", p.Cfg)
	}
}

func TestOIDCSuccess(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = f.baseClaims
	p := newProvider(f, "email", "profile") // openid is added automatically
	st := login(t, f, p)
	id, err := p.Exchange(context.Background(), st, st.State, "good-code")
	if err != nil {
		t.Fatal(err)
	}
	if id.Provider != "oidc:corp" || id.Subject != "sub-123" || id.Email != "alice@example.com" || id.Name != "Alice" {
		t.Errorf("identity %+v", id)
	}
	if len(id.Groups) != 2 || id.Groups[0] != "qa" || id.Groups[1] != "admins" {
		t.Errorf("groups %v", id.Groups)
	}
	if f.gotVerifier != st.Verifier {
		t.Errorf("PKCE verifier not sent")
	}
	// Second AuthURL reuses the discovered provider.
	if _, st2, err := p.AuthURL(context.Background()); err != nil || st2.State == st.State {
		t.Errorf("second AuthURL: %v", err)
	}
}

func TestOIDCClaimVariants(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = func(n string) map[string]any {
		c := f.baseClaims(n)
		c["groups"] = "a,b"
		delete(c, "email_verified")
		return c
	}
	p := newProvider(f)
	st := login(t, f, p)
	id, err := p.Exchange(context.Background(), st, st.State, "good-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Groups) != 2 || id.Groups[1] != "b" {
		t.Errorf("groups %v", id.Groups)
	}
}

func TestOIDCExchangeFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		setup  func(f *fakeIdP)
		state  func(st OIDCState) (OIDCState, string, string)
		errSub string
	}{
		{"state mismatch", nil, func(st OIDCState) (OIDCState, string, string) { return st, "other", "good-code" }, "state mismatch"},
		{"empty state", nil, func(st OIDCState) (OIDCState, string, string) { st.State = ""; return st, "", "good-code" }, "state mismatch"},
		{"missing code", nil, func(st OIDCState) (OIDCState, string, string) { return st, st.State, "" }, "missing code"},
		{"bad code", nil, func(st OIDCState) (OIDCState, string, string) { return st, st.State, "bad" }, "token exchange"},
		{"token endpoint error", func(f *fakeIdP) { f.tokenStatus = http.StatusInternalServerError }, nil, "token exchange"},
		{"no id_token", func(f *fakeIdP) { f.claims = func(string) map[string]any { return nil } }, nil, "no id_token"},
		{"bad signature", func(f *fakeIdP) { _, f.signKey = rsaKeys(f.t) }, nil, "verify id_token"},
		{"wrong audience", func(f *fakeIdP) {
			f.claims = func(n string) map[string]any { c := f.baseClaims(n); c["aud"] = "someone-else"; return c }
		}, nil, "verify id_token"},
		{"nonce mismatch", nil, func(st OIDCState) (OIDCState, string, string) {
			s := st.State
			st.Nonce = "different"
			return st, s, "good-code"
		}, "nonce mismatch"},
		{"email not verified", func(f *fakeIdP) {
			f.claims = func(n string) map[string]any { c := f.baseClaims(n); c["email_verified"] = false; return c }
		}, nil, "not verified"},
		{"missing email", func(f *fakeIdP) {
			f.claims = func(n string) map[string]any { c := f.baseClaims(n); delete(c, "email"); return c }
		}, nil, "no e-mail claim"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeIdP(t)
			f.claims = f.baseClaims
			if tc.setup != nil {
				tc.setup(f)
			}
			p := newProvider(f)
			st := login(t, f, p)
			state, code := st.State, "good-code"
			if tc.state != nil {
				st, state, code = tc.state(st)
			}
			_, err := p.Exchange(ctx, st, state, code)
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("err = %v, want %q", err, tc.errSub)
			}
		})
	}
}

func TestOIDCDiscoveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p := NewOIDCProvider(config.OIDC{Name: "broken", Issuer: srv.URL, ClientID: "c"}, "cb")
	if _, _, err := p.AuthURL(context.Background()); err == nil || !strings.Contains(err.Error(), "discovery") {
		t.Errorf("AuthURL: %v", err)
	}
	if _, err := p.Exchange(context.Background(), OIDCState{State: "s"}, "s", "c"); err == nil || !strings.Contains(err.Error(), "discovery") {
		t.Errorf("Exchange: %v", err)
	}
}

func TestClaimHelpers(t *testing.T) {
	c := map[string]any{"s": "  v ", "n": 3, "list": []any{"a", 1, "b"}, "csv": "x,y", "empty": ""}
	if claimString(c, "s") != "v" || claimString(c, "n") != "" || claimString(c, "missing") != "" {
		t.Error("claimString")
	}
	if got := claimStrings(c, "list"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("list: %v", got)
	}
	if got := claimStrings(c, "csv"); len(got) != 2 || got[0] != "x" {
		t.Errorf("csv: %v", got)
	}
	if claimStrings(c, "empty") != nil || claimStrings(c, "missing") != nil || claimStrings(c, "n") != nil {
		t.Error("claimStrings nil cases")
	}
}
