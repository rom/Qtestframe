package auth

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"

	"github.com/rom/qtestframe/internal/config"
)

// SAMLProvider is a SAML 2.0 service provider bound to one IdP.
type SAMLProvider struct {
	Cfg        config.SAML
	BaseURL    *url.URL // e.g. https://qtf.example.com/api/v1/auth/saml/<name>
	HTTPClient *http.Client

	mu sync.Mutex
	sp *saml.ServiceProvider
}

// NewSAMLProvider returns a provider. base is the provider URL prefix
// under which /metadata and /acs are served.
func NewSAMLProvider(cfg config.SAML, base string) (*SAMLProvider, error) {
	u, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("saml %s: base url: %w", cfg.Name, err)
	}
	if cfg.EmailAttr == "" {
		cfg.EmailAttr = "email"
	}
	if cfg.NameAttr == "" {
		cfg.NameAttr = "displayName"
	}
	if cfg.GroupsAttr == "" {
		cfg.GroupsAttr = "groups"
	}
	return &SAMLProvider{Cfg: cfg, BaseURL: u}, nil
}

// SP returns the (lazily initialised) service provider.
func (p *SAMLProvider) SP(ctx context.Context) (*saml.ServiceProvider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sp != nil {
		return p.sp, nil
	}
	var md *saml.EntityDescriptor
	var err error
	switch {
	case p.Cfg.IDPMetadataFile != "":
		var b []byte
		if b, err = os.ReadFile(p.Cfg.IDPMetadataFile); err == nil {
			md, err = samlsp.ParseMetadata(b)
		}
	default:
		var mu *url.URL
		if mu, err = url.Parse(p.Cfg.IDPMetadataURL); err == nil {
			client := p.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: 15 * time.Second}
			}
			md, err = samlsp.FetchMetadata(ctx, client, *mu)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("saml %s: idp metadata: %w", p.Cfg.Name, err)
	}
	sp := &saml.ServiceProvider{
		EntityID:          p.Cfg.EntityID,
		MetadataURL:       *p.BaseURL.JoinPath("metadata"),
		AcsURL:            *p.BaseURL.JoinPath("acs"),
		IDPMetadata:       md,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
	}
	if sp.EntityID == "" {
		sp.EntityID = sp.MetadataURL.String()
	}
	if p.Cfg.SPCertFile != "" && p.Cfg.SPKeyFile != "" {
		kp, err := tls.LoadX509KeyPair(p.Cfg.SPCertFile, p.Cfg.SPKeyFile)
		if err != nil {
			return nil, fmt.Errorf("saml %s: sp keypair: %w", p.Cfg.Name, err)
		}
		cert, err := x509.ParseCertificate(kp.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("saml %s: sp cert: %w", p.Cfg.Name, err)
		}
		signer, ok := kp.PrivateKey.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("saml %s: sp key is not a signer", p.Cfg.Name)
		}
		sp.Key, sp.Certificate = signer, cert
	}
	p.sp = sp
	return sp, nil
}

// SAMLState is kept in a signed cookie between login and ACS.
type SAMLState struct {
	RequestID string `json:"r"`
}

// AuthURL builds a redirect-binding AuthnRequest.
func (p *SAMLProvider) AuthURL(ctx context.Context, relay string) (string, SAMLState, error) {
	sp, err := p.SP(ctx)
	if err != nil {
		return "", SAMLState{}, err
	}
	loc := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
	if loc == "" {
		return "", SAMLState{}, errors.New("saml: idp has no redirect binding")
	}
	req, err := sp.MakeAuthenticationRequest(loc, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return "", SAMLState{}, err
	}
	u, err := req.Redirect(relay, sp)
	if err != nil {
		return "", SAMLState{}, err
	}
	return u.String(), SAMLState{RequestID: req.ID}, nil
}

// Metadata returns the SP metadata document.
func (p *SAMLProvider) Metadata(ctx context.Context) (*saml.EntityDescriptor, error) {
	sp, err := p.SP(ctx)
	if err != nil {
		return nil, err
	}
	return sp.Metadata(), nil
}

// ParseResponse validates the POSTed assertion and returns the identity.
func (p *SAMLProvider) ParseResponse(r *http.Request, st SAMLState) (*Identity, error) {
	sp, err := p.SP(r.Context())
	if err != nil {
		return nil, err
	}
	a, err := sp.ParseResponse(r, []string{st.RequestID})
	if err != nil {
		var ie *saml.InvalidResponseError
		if errors.As(err, &ie) {
			return nil, fmt.Errorf("saml: invalid response: %w", ie.PrivateErr)
		}
		return nil, fmt.Errorf("saml: %w", err)
	}
	return p.identity(a)
}

func (p *SAMLProvider) identity(a *saml.Assertion) (*Identity, error) {
	id := &Identity{Provider: "saml:" + p.Cfg.Name}
	if a.Subject != nil && a.Subject.NameID != nil {
		id.Subject = a.Subject.NameID.Value
	}
	for _, st := range a.AttributeStatements {
		for _, attr := range st.Attributes {
			name := attr.Name
			match := func(want string) bool { return name == want || attr.FriendlyName == want }
			var vals []string
			for _, v := range attr.Values {
				vals = append(vals, v.Value)
			}
			switch {
			case match(p.Cfg.EmailAttr) && len(vals) > 0:
				id.Email = vals[0]
			case match(p.Cfg.NameAttr) && len(vals) > 0:
				id.Name = vals[0]
			case match(p.Cfg.GroupsAttr):
				id.Groups = append(id.Groups, vals...)
			}
		}
	}
	if id.Email == "" && strings.Contains(id.Subject, "@") {
		id.Email = id.Subject
	}
	if id.Subject == "" {
		id.Subject = id.Email
	}
	if id.Email == "" {
		return nil, errors.New("saml: assertion has no e-mail")
	}
	return id, nil
}
