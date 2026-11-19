package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/rom/qtestframe/internal/config"
)

// OIDCProvider performs the authorization code flow with PKCE and nonce.
type OIDCProvider struct {
	Cfg         config.OIDC
	RedirectURL string
	HTTPClient  *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewOIDCProvider creates a provider; discovery happens lazily on first use.
func NewOIDCProvider(cfg config.OIDC, redirectURL string) *OIDCProvider {
	if cfg.EmailClaim == "" {
		cfg.EmailClaim = "email"
	}
	if cfg.NameClaim == "" {
		cfg.NameClaim = "name"
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return &OIDCProvider{Cfg: cfg, RedirectURL: redirectURL}
}

func (p *OIDCProvider) ctx(ctx context.Context) context.Context {
	if p.HTTPClient != nil {
		return oidc.ClientContext(ctx, p.HTTPClient)
	}
	return ctx
}

func (p *OIDCProvider) init(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.provider != nil {
		return nil
	}
	prov, err := oidc.NewProvider(p.ctx(ctx), p.Cfg.Issuer)
	if err != nil {
		return fmt.Errorf("oidc %s: discovery: %w", p.Cfg.Name, err)
	}
	scopes := p.Cfg.Scopes
	hasOpenID := false
	for _, s := range scopes {
		if s == oidc.ScopeOpenID {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	p.provider = prov
	p.oauth = &oauth2.Config{
		ClientID:     p.Cfg.ClientID,
		ClientSecret: p.Cfg.ClientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  p.RedirectURL,
		Scopes:       scopes,
	}
	p.verifier = prov.Verifier(&oidc.Config{ClientID: p.Cfg.ClientID})
	return nil
}

// OIDCState is kept in a signed cookie between login and callback.
type OIDCState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
}

// AuthURL returns the IdP redirect URL and the state to remember.
func (p *OIDCProvider) AuthURL(ctx context.Context) (string, OIDCState, error) {
	if err := p.init(ctx); err != nil {
		return "", OIDCState{}, err
	}
	st := OIDCState{State: RandomToken(24), Nonce: RandomToken(24), Verifier: oauth2.GenerateVerifier()}
	u := p.oauth.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier))
	return u, st, nil
}

// Exchange validates the callback and returns the asserted identity.
func (p *OIDCProvider) Exchange(ctx context.Context, st OIDCState, state, code string) (*Identity, error) {
	if err := p.init(ctx); err != nil {
		return nil, err
	}
	if state == "" || state != st.State {
		return nil, errors.New("oidc: state mismatch")
	}
	if code == "" {
		return nil, errors.New("oidc: missing code")
	}
	tok, err := p.oauth.Exchange(p.ctx(ctx), code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return nil, fmt.Errorf("oidc: token exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("oidc: no id_token in response")
	}
	idt, err := p.verifier.Verify(p.ctx(ctx), raw)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify id_token: %w", err)
	}
	if idt.Nonce != st.Nonce {
		return nil, errors.New("oidc: nonce mismatch")
	}
	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: claims: %w", err)
	}
	id := &Identity{
		Provider: "oidc:" + p.Cfg.Name,
		Subject:  idt.Subject,
		Email:    claimString(claims, p.Cfg.EmailClaim),
		Name:     claimString(claims, p.Cfg.NameClaim),
		Groups:   claimStrings(claims, p.Cfg.GroupsClaim),
	}
	if v, ok := claims["email_verified"].(bool); ok && !v {
		return nil, errors.New("oidc: e-mail address not verified")
	}
	if id.Email == "" {
		return nil, errors.New("oidc: no e-mail claim")
	}
	return id, nil
}

func claimString(c map[string]any, k string) string {
	if s, ok := c[k].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func claimStrings(c map[string]any, k string) []string {
	switch v := c[k].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return strings.Split(v, ",")
	}
	return nil
}
